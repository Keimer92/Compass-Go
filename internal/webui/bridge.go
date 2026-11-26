// Package webui es el puente entre el frontend Svelte y el motor de Compass.
// Expone servicios enlazados por Wails y events hacia el navegador.
package webui

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"compass/internal/project"
	"compass/internal/settings"
	"compass/internal/sound"
	"compass/internal/tasks"
	"compass/internal/window"
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

	// commitMu protege commitCancels: una generacion de commit en curso POR
	// PROYECTO, para poder abortarla desde el boton Cancelar sin tocar las de
	// los demas proyectos (generar en A y luego en B no cancela A).
	commitMu      sync.Mutex
	commitCancels map[string]*commitRun
	commitSeq     uint64

	// Auto-commit por inactividad (boton "Auto" de cada tarjeta de proyecto). Vive en el
	// backend y no en la pestana para que siga vigilando aunque el usuario
	// cambie a Explorador o Historial.
	autoMu   sync.Mutex
	autoRuns map[string]*autoTrack
	autoOnce sync.Once

	// Vigilancia del arbol de trabajo (fsnotify) para detectar cambios al
	// instante. Se activa para el proyecto activo y para los que tienen Auto.
	watchMu sync.Mutex
	watches map[string]*dirWatch
	// watchDead marca carpetas retiradas mientras un ensureWatch en vuelo
	// todavia recorre el arbol: al terminar no debe dejar un vigilante vivo.
	watchDead map[string]bool

	// releasePortable es el toggle "Incluir portable" del modal Win Release.
	// Se guarda aqui porque TaskScript (que arma el comando) y el modal viven
	// en procesos distintos del de la tarea.
	releasePortable bool
}

// commitRun es una generacion de mensaje de commit en curso. El id permite
// saber si la entrada del mapa sigue siendo esta y no una posterior.
type commitRun struct {
	id     uint64
	cancel context.CancelFunc
}

// commitKey normaliza la ruta del proyecto para usarla como clave.
func commitKey(dir string) string {
	if abs, err := filepath.Abs(dir); err == nil {
		return strings.ToLower(abs)
	}
	return strings.ToLower(dir)
}

// NewBridge crea el servicio para la carpeta inicial.
//
// La carpeta de lanzamiento manda: si es un proyecto detectable se activa esa,
// aunque el workspace guardado tenga otras. Si no lo es, se cae al proyecto
// activo persistido para que el menu no salga vacio.
func NewBridge(dir string) *Bridge {
	b := &Bridge{
		dir:           dir,
		ws:            workspace.Load(),
		term:          NewTerminalService(),
		commitCancels: map[string]*commitRun{},
	}
	b.ws.AddAny(dir)
	b.detect(b.dir)
	if b.ctx == nil {
		if p := b.ws.ActiveProject(); p != nil {
			b.detect(p.Path)
		}
	}
	// El boton "Auto" del panel Git sobrevive al reinicio: se rearman los
	// proyectos que lo tenian activado.
	b.mu.Lock()
	b.resumeAuto()
	b.mu.Unlock()
	// Cambios de archivo al instante en el panel Git del proyecto activo.
	b.ensureWatch(b.dir)
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
	b.resumeAuto()
	// Igual que en SelectProject: el recorrido del arbol no bloquea el alta.
	activeDir := b.dir
	b.allowWatch(activeDir)
	go b.ensureWatch(activeDir)
	Emit("workspace:changed", b.workspaceInfo())
	// Si el proyecto llega desde el menu contextual (otra instancia), trae
	// esta ventana al frente.
	b.FocusWindow()
}

// PickProject abre el selector nativo de carpetas y apila el proyecto elegido.
// Lo usa el boton "+" de la barra lateral.
func (b *Bridge) PickProject() map[string]any {
	if app == nil {
		return b.GetWorkspace()
	}
	dir, err := app.Dialog.OpenFile().
		SetTitle("Abrir proyecto en Compass Go").
		CanChooseFiles(false).
		CanChooseDirectories(true).
		PromptForSingleSelection()
	if err != nil || strings.TrimSpace(dir) == "" {
		return b.GetWorkspace()
	}
	b.AddProject(dir)
	return b.GetWorkspace()
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
	// Platform es "windows", "android", "go" o "general": el frontend lo pinta
	// como distintivo adicional sobre el icono propio de la card.
	Platform string `json:"platform"`
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
	b.resumeAuto()
	// El vigilante recorre el arbol para registrar sus carpetas y puede tardar
	// en proyectos grandes: se arma fuera del lock y despues de responder a la
	// UI, para que el cambio de proyecto no espere por el.
	dir := b.dir
	info := b.workspaceInfo()
	b.mu.Unlock()
	b.allowWatch(dir)
	go b.ensureWatch(dir)
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
	removed := b.ws.Projects[index]
	b.ws.Remove(index) // ajusta Active y hace Save
	if removed != nil {
		// Un proyecto que sale del workspace no debe seguir commiteando solo
		// ni mantener un vigilante de archivos abierto.
		b.stopAuto(removed.Path)
		b.stopWatch(removed.Path)
	}
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
		// Las cards de Android solo tienen sentido si el proyecto trae el
		// modulo Android generado (build/android, android/ o gradlew). Sin el,
		// las tareas abortan pidiendo `wails3 update build-assets`.
		if item.IsAndroid(ctx.ProjectType) && !ctx.HasAndroidProject {
			continue
		}
		// En Go+Wails, Win Dev / Win Release lanzan `wails3 task ...`: sin
		// Taskfile.yml el CLI aborta con "No Taskfile found".
		if item.NeedsWailsTasks() && ctx.ProjectType == project.TypeGoWails && !ctx.HasTaskfile {
			continue
		}
		// Solo se muestran cards con una tarea real para este tipo de proyecto:
		// asi no aparecen (p.ej.) HMR o Android en un proyecto Go puro.
		if !tasks.Available(taskFor(item, ctx.ProjectType), ctx.ProjectType) {
			continue
		}
		out = append(out, MenuItemInfo{
			ID:          fmt.Sprintf("%d", i),
			Label:       label,
			Description: item.Description(ctx.ProjectType),
			Icon:        item.Icon(),
			Color:       item.ColorKey(),
			Section:     item.Section(),
			Platform:    item.Platform(ctx.ProjectType),
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
	// Las cards de Vibe Coding no tienen indice de menu: su id ya es la tarea
	// (vibe-run-<cli>). Las acciones de mantenimiento se lanzan igual desde
	// Opciones con su nombre de tarea.
	if strings.HasPrefix(taskID, "vibe-run-") || taskID == "vibe-setup" ||
		taskID == "opencode-configure" {
		return taskID
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
	// Vibe Coding: la CLI se lanza directamente en la ConPTY, sin el binario de
	// Compass de por medio. Son TUIs interactivas (opencode, cline...) y
	// necesitan ser el proceso en primer plano para recibir teclado y Ctrl-C.
	if key, ok := strings.CutPrefix(task, "vibe-run-"); ok {
		cmd := tasks.VibeCommand(key)
		if cmd == "" {
			return "Write-Host 'Esa CLI no esta instalada. Usa \"Instalar faltantes\".' -ForegroundColor Yellow"
		}
		return "& " + cmd
	}
	dir := b.projectDir()
	exe := b.ExePath()
	// ArgumentList se une con espacios sin entrecomillar: la ruta (que puede
	// llevar espacios) tiene que ir con comillas dobles dentro del literal.
	argPath := psQuote(`"` + dir + `"`)
	args := fmt.Sprintf("'--task',%s,'--path',%s", psQuote(task), argPath)
	// El release de escritorio lleva el toggle "Incluir portable" del modal.
	// El proceso de la tarea es otra instancia del binario, asi que la opcion
	// viaja como flag; sin ella el release deja solo el instalador.
	if strings.HasPrefix(task, "windows-release") && b.releasePortableOn() {
		args += ",'--portable'"
	}
	return fmt.Sprintf(
		"$p = Start-Process -FilePath %s -ArgumentList %s -NoNewWindow -Wait -PassThru; exit $p.ExitCode",
		psQuote(exe), args)
}

// SetReleasePortable guarda el toggle "Incluir portable" del modal Win Release.
// Se reinicia a false cada vez que se abre el modal (por defecto, sin portable).
func (b *Bridge) SetReleasePortable(on bool) {
	b.mu.Lock()
	b.releasePortable = on
	b.mu.Unlock()
}

func (b *Bridge) releasePortableOn() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.releasePortable
}

// psQuote envuelve en comillas simples de PowerShell (escapando las internas).
func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// OpenTerminal abre una ConPTY con el script dado. El id lo aporta el
// frontend (el de su pestana) para no perder la salida temprana.
func (b *Bridge) OpenTerminal(id, label, script string, rows, cols int) string {
	b.mu.Lock()
	dir := b.dir
	b.mu.Unlock()
	return b.term.OpenSession(id, label, dir, script, dir, rows, cols)
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

// ListTerminals devuelve las ConPTY vivas con su salida reciente. El frontend
// lo llama al montar para reconstruir las pestanas que ya existian: en modo
// desarrollo el WebView se recarga en cada cambio de la UI y los procesos
// deben seguir vivos.
func (b *Bridge) ListTerminals() []TerminalInfo { return b.term.List() }

// FocusWindow trae al frente la ventana principal de Compass. El frontend lo
// llama tras lanzar una tarea para recuperar el foco si otra ventana (p. ej.
// la app en modo dev) se lo roba al abrirse.
func (b *Bridge) FocusWindow() {
	if app == nil {
		return
	}
	w, ok := app.Window.Get("main")
	if !ok || w == nil {
		return
	}
	// SetForegroundWindow desde Wails suele ser ignorado por Windows cuando la
	// ventana no es la activa; FocusHandle restaura y se engancha al hilo en
	// primer plano para forzarlo.
	if hwnd := w.NativeWindow(); hwnd != nil {
		window.FocusHandle(uintptr(hwnd))
	}
	w.Focus()
}

// GetSettings devuelve las opciones globales. La API key NUNCA se envia al
// frontend: OpenRouterKeyConfigured dice si ya hay una guardada, sin revelar
// ningun caracter (ni siquiera enmascarado).
func (b *Bridge) GetSettings() settings.Settings {
	s := settings.Load()
	s.SoundPresets = sound.Normalize(s.SoundPresets)
	s.OpenRouterAPIKey = ""
	return s
}

// OpenRouterKeyConfigured indica si ya hay una API key guardada.
func (b *Bridge) OpenRouterKeyConfigured() bool {
	return strings.TrimSpace(settings.Load().OpenRouterAPIKey) != ""
}

// SetSettings guarda las opciones globales partiendo de lo que ya habia
// guardado: la key solo se reemplaza si el frontend envia una nueva (no vacia).
// Asi cambiar el modelo no puede perder la key.
func (b *Bridge) SetSettings(s settings.Settings) {
	cur := settings.Load()

	cur.OpenOutputDir = s.OpenOutputDir
	cur.CloseTabOnBuildEnd = s.CloseTabOnBuildEnd
	cur.SoundPresets = sound.Normalize(s.SoundPresets)
	// 0 lo descarta Load al normalizar; el rango util lo acota settings.Load.
	cur.TerminalPanelPct = s.TerminalPanelPct
	vibeChanged := !settings.SameVibeDisabled(cur.VibeDisabled, s.VibeDisabled)
	cur.VibeDisabled = settings.NormalizeVibeDisabled(s.VibeDisabled)
	if s.AutoCommitIdleMin > 0 {
		cur.AutoCommitIdleMin = s.AutoCommitIdleMin
	}
	if v := strings.TrimSpace(s.OpenRouterEndpoint); v != "" {
		cur.OpenRouterEndpoint = v
	}
	if v := strings.TrimSpace(s.OpenRouterModel); v != "" {
		cur.OpenRouterModel = v
	}
	if k := strings.TrimSpace(s.OpenRouterAPIKey); k != "" {
		cur.OpenRouterAPIKey = k
	}

	settings.Save(cur)
	// La cascada del Explorador se escribe en el registro al instalarse: si
	// cambian los interruptores hay que reescribirla para que muestre las
	// mismas CLIs que la card. Se hace en segundo plano (PowerShell tarda) y
	// solo si el menu ya estaba instalado.
	if vibeChanged {
		go func() { _ = tasks.SyncVibeContextMenu() }()
	}
}

// SoundCatalog describe los eventos y presets que ofrece el modal de Opciones.
type SoundCatalog struct {
	Events  []sound.EventInfo  `json:"events"`
	Presets []sound.PresetInfo `json:"presets"`
}

// GetSoundCatalog devuelve la lista de eventos y presets de sonido.
func (b *Bridge) GetSoundCatalog() SoundCatalog {
	return SoundCatalog{Events: sound.Events(), Presets: sound.Presets()}
}

// PreviewSound reproduce un evento con el preset pedido, sin guardar nada.
// El frontend lo usa en el boton de escucha de cada evento.
func (b *Bridge) PreviewSound(event, preset string) {
	sound.Preview(sound.Event(event), preset)
}

// ListCleanupCandidates devuelve los destinos que existen ahora mismo en el
// proyecto activo, para el modal "Elegir que borrar" de la GUI.
func (b *Bridge) ListCleanupCandidates() []tasks.CleanupTarget {
	b.mu.Lock()
	ctx := b.ctx
	b.mu.Unlock()
	return tasks.ListCleanupTargets(ctx)
}

// PurgeCleanupCandidates valida los paths contra la lista dinamica del
// proyecto activo antes de borrar. Si cualquier path no es un destino
// permitido, devuelve error y no borra nada.
func (b *Bridge) PurgeCleanupCandidates(paths []string) (tasks.CleanupPurgeResult, error) {
	b.mu.Lock()
	ctx := b.ctx
	b.mu.Unlock()
	if ctx == nil {
		return tasks.CleanupPurgeResult{}, fmt.Errorf("no hay proyecto activo")
	}
	return tasks.PurgeCleanupTargets(ctx, paths)
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
