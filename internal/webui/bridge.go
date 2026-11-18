// Package webui es el puente entre el frontend Svelte y el motor de Compass.
// Expone servicios enlazados por Wails y events hacia el navegador.
package webui

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"compass/internal/project"
	"compass/internal/settings"
	"compass/internal/tasks"
	"compass/internal/workspace"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Bridge es el servicio enlazado al frontend.
type Bridge struct {
	ws   *workspace.Workspace
	dir  string
	ctx  *project.ProjectContext
	mu   sync.Mutex
	term *TerminalService
}

// NewBridge crea el servicio para la carpeta inicial.
//
// La carpeta de lanzamiento manda: si es un proyecto detectable se activa esa,
// aunque el workspace guardado tenga otras. Si no lo es, se cae al proyecto
// activo persistido para que el menu no salga vacio.
func NewBridge(dir string) *Bridge {
	b := &Bridge{dir: dir, ws: workspace.Load(), term: NewTerminalService()}
	b.ws.AddAny(dir)
	b.detect(b.dir)
	if b.ctx == nil {
		if p := b.ws.ActiveProject(); p != nil {
			b.detect(p.Path)
		}
	}
	return b
}

// detect intenta convertir dir en el proyecto activo.
func (b *Bridge) detect(dir string) bool {
	if dir == "" {
		return false
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	if err := os.Chdir(abs); err != nil {
		return false
	}
	ctx, err := project.Detect()
	if err != nil {
		return false
	}
	b.ctx = ctx
	b.dir = ctx.ScriptDir
	return true
}

// GetDebug expone el estado interno para diagnosticar el arranque.
func (b *Bridge) GetDebug() map[string]any {
	b.mu.Lock()
	defer b.mu.Unlock()
	ctxDir := ""
	pt := ""
	if b.ctx != nil {
		ctxDir = b.ctx.ScriptDir
		pt = b.ctx.ProjectType.Label()
	}
	return map[string]any{
		"dir":      b.dir,
		"ctxDir":   ctxDir,
		"type":     pt,
		"projects": len(b.ws.Projects),
		"active":   b.ws.Active,
	}
}

// AddProject apila una carpeta (p.ej. desde el menu contextual de otra
// instancia) y la activa. Emite workspace:changed para que el frontend repinte.
func (b *Bridge) AddProject(dir string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	i, _ := b.ws.AddAny(dir)
	if i < 0 {
		return
	}
	b.ws.Active = i
	b.ws.Save()
	if p := b.ws.Projects[i]; p != nil {
		b.detect(p.Path)
	}
	Emit("workspace:changed", b.workspaceInfo())
	// Si el proyecto llega desde el menu contextual (otra instancia), trae
	// esta ventana al frente.
	b.FocusWindow()
}

// ProjectInfo es la informacion de un proyecto para el frontend.
type ProjectInfo struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// MenuItemInfo es una card del menu.
type MenuItemInfo struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Color       string `json:"color"`
	Section     string `json:"section"`
}

// GetWorkspace devuelve los proyectos y el activo.
func (b *Bridge) GetWorkspace() map[string]any {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.workspaceInfo()
}

// workspaceInfo arma la respuesta de GetWorkspace (sin tomar el candado).
func (b *Bridge) workspaceInfo() map[string]any {
	projects := make([]ProjectInfo, 0, len(b.ws.Projects))
	for _, p := range b.ws.Projects {
		projects = append(projects, ProjectInfo{
			Path: p.Path, Name: filepath.Base(p.Path), Type: p.Type,
		})
	}
	return map[string]any{
		"projects": projects,
		"active":   b.ws.Active,
	}
}

// SelectProject cambia el proyecto activo.
func (b *Bridge) SelectProject(index int) map[string]any {
	b.mu.Lock()
	if index < 0 || index >= len(b.ws.Projects) {
		info := b.workspaceInfo()
		b.mu.Unlock()
		return info
	}
	b.ws.Active = index
	b.ws.Save()
	if p := b.ws.Projects[index]; p != nil {
		b.detect(p.Path)
	}
	info := b.workspaceInfo()
	b.mu.Unlock()
	return info
}

// RemoveProject cierra un proyecto de la lista (lo quita del workspace, no
// borra nada del disco). Si era el activo, re-detecta el nuevo activo; si no
// queda ninguno, deja el puente sin contexto. Emite workspace:changed.
func (b *Bridge) RemoveProject(index int) map[string]any {
	b.mu.Lock()
	defer b.mu.Unlock()
	if index < 0 || index >= len(b.ws.Projects) {
		return b.workspaceInfo()
	}
	wasActive := index == b.ws.Active
	b.ws.Remove(index) // ajusta Active y hace Save
	if len(b.ws.Projects) == 0 {
		b.ctx, b.dir = nil, ""
	} else if wasActive {
		if p := b.ws.ActiveProject(); p != nil && !b.detect(p.Path) {
			b.ctx, b.dir = nil, ""
		}
	}
	Emit("workspace:changed", b.workspaceInfo())
	return b.workspaceInfo()
}

// GetMenu devuelve las cards del menu del proyecto activo.
func (b *Bridge) GetMenu() []MenuItemInfo {
	b.mu.Lock()
	ctx := b.ctx
	b.mu.Unlock()
	out := []MenuItemInfo{}
	if ctx == nil {
		return out
	}
	for i, item := range allMenuItems {
		if hiddenMenuItems[item] {
			continue
		}
		label := item.Label(ctx.ProjectType)
		if label == "" {
			continue
		}
		out = append(out, MenuItemInfo{
			ID:          fmt.Sprintf("%d", i),
			Label:       label,
			Description: item.Description(ctx.ProjectType),
			Icon:        item.Icon(),
			Color:       item.ColorKey(),
			Section:     item.Section(),
		})
	}
	return out
}

// RunTask lanza una tarea en una pestana (el frontend abrira un terminal).
func (b *Bridge) RunTask(taskID string) string {
	b.mu.Lock()
	ctx := b.ctx
	b.mu.Unlock()
	if ctx == nil {
		return ""
	}
	idx := 0
	if _, err := fmt.Sscanf(taskID, "%d", &idx); err != nil {
		return ""
	}
	item, ok := menuItem(idx)
	if !ok {
		return ""
	}
	return taskFor(item, ctx.ProjectType)
}

// ExePath devuelve la ruta del ejecutable (para reinvocarlo dentro de la ConPTY).
func (b *Bridge) ExePath() string {
	exe, err := os.Executable()
	if err != nil {
		return "compass-go.exe"
	}
	return exe
}

// TaskScript arma el comando PowerShell que corre una tarea dentro de la
// ConPTY.
//
// Hace falta Start-Process -Wait: el binario es de subsistema GUI y PowerShell
// NO espera a los ejecutables GUI lanzados con `&`, asi que el shell terminaba
// enseguida, la ConPTY se cerraba y la tarea moria (justo al llegar a un
// prompt). -NoNewWindow comparte la consola, con lo que la salida sigue
// llegando al terminal.
func (b *Bridge) TaskScript(task string) string {
	dir := b.projectDir()
	exe := b.ExePath()
	// ArgumentList se une con espacios sin entrecomillar: la ruta (que puede
	// llevar espacios) tiene que ir con comillas dobles dentro del literal.
	argPath := psQuote(`"` + dir + `"`)
	return fmt.Sprintf(
		"$p = Start-Process -FilePath %s -ArgumentList '--task',%s,'--path',%s -NoNewWindow -Wait -PassThru; exit $p.ExitCode",
		psQuote(exe), psQuote(task), argPath)
}

// psQuote envuelve en comillas simples de PowerShell (escapando las internas).
func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// OpenTerminal abre una ConPTY con el script dado. El id lo aporta el
// frontend (el de su pestana) para no perder la salida temprana.
func (b *Bridge) OpenTerminal(id, script string, rows, cols int) string {
	b.mu.Lock()
	dir := b.dir
	b.mu.Unlock()
	return b.term.OpenSession(id, script, dir, rows, cols)
}

// WriteTerminal envia bytes (base64) a la ConPTY.
func (b *Bridge) WriteTerminal(id, dataB64 string) {
	data, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		return
	}
	b.term.Write(id, data)
}

// ResizeTerminal cambia el tamano de la ConPTY.
func (b *Bridge) ResizeTerminal(id string, cols, rows int) { b.term.Resize(id, cols, rows) }

// CloseTerminal mata la sesion de la pestana.
func (b *Bridge) CloseTerminal(id string) { b.term.Close(id) }

// FocusWindow trae al frente la ventana principal de Compass. El frontend lo
// llama tras lanzar una tarea para recuperar el foco si otra ventana (p. ej.
// la app en modo dev) se lo roba al abrirse.
func (b *Bridge) FocusWindow() {
	if app == nil {
		return
	}
	if w, ok := app.Window.Get("main"); ok && w != nil {
		w.Focus()
	}
}

// GetSettings devuelve las opciones globales. La API key nunca se envia al
// frontend: se devuelve vacia para que el campo del modal quede en blanco.
func (b *Bridge) GetSettings() settings.Settings {
	s := settings.Load()
	s.OpenRouterAPIKey = ""
	return s
}

// SetSettings guarda las opciones globales. Si el frontend no envio la key
// (vacio o enmascarado), se conserva la que ya habia guardada.
func (b *Bridge) SetSettings(s settings.Settings) {
	if k := strings.TrimSpace(s.OpenRouterAPIKey); k == "" || k == "********" {
		s.OpenRouterAPIKey = settings.Load().OpenRouterAPIKey
	}
	settings.Save(s)
}

// RunTask es el punto de entrada --task del propio binario.
func RunTask(task, dir string) {
	_ = RunTaskDirect(task, dir)
}

// RunTaskDirect ejecuta una tarea headless (usa el mismo despachador).
func RunTaskDirect(task, dir string) error {
	if err := os.Chdir(dir); err != nil {
		return err
	}
	ctx, err := project.Detect()
	if err != nil {
		return err
	}
	return tasks.RunTaskDirect(ctx, task)
}

// Encode sirve para enviar bytes al frontend como base64.
func Encode(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// app guarda la instancia para emitir eventos desde otras partes.
var app *application.App

// emitHook permite a los tests capturar los eventos sin ventana.
var emitHook func(name string, data any)

// SetApp registra la app (se llama desde el entrypoint).
func SetApp(a *application.App) { app = a }

// Emit envia un evento al frontend.
func Emit(name string, data any) {
	if emitHook != nil {
		emitHook(name, data)
		return
	}
	if app != nil {
		app.Event.Emit(name, data)
	}
}
