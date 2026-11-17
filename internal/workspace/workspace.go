// Package workspace gestiona la lista de proyectos abiertos en una sola
// ventana de Compass y la persistencia de esa lista. Es la base del modelo
// multi-proyecto de la GUI (fase 3 del plan).
package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"compass/internal/project"
)

// Project es un proyecto apilado en la sidebar.
type Project struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

// Workspace es la lista persistida de proyectos y cual esta activo.
type Workspace struct {
	Projects []*Project `json:"projects"`
	Active   int        `json:"active"`
}

func storePath() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return ""
	}
	return filepath.Join(base, "compass", "projects.json")
}

// Load lee la lista guardada. Un fichero ausente o corrupto no es un error.
func Load() *Workspace {
	ws := &Workspace{Active: -1}
	path := storePath()
	if path == "" {
		return ws
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ws
	}
	var stored Workspace
	if err := json.Unmarshal(data, &stored); err != nil {
		return ws
	}
	for _, p := range stored.Projects {
		if p == nil || p.Path == "" {
			continue
		}
		if _, err := os.Stat(p.Path); err != nil {
			continue // la carpeta ya no existe
		}
		ws.Projects = append(ws.Projects, p)
	}
	ws.Active = stored.Active
	if ws.Active < 0 || ws.Active >= len(ws.Projects) {
		ws.Active = 0
	}
	if len(ws.Projects) == 0 {
		ws.Active = -1
	}
	return ws
}

// Save persiste la lista. Un fallo de escritura no es fatal.
func (ws *Workspace) Save() {
	path := storePath()
	if path == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	if data, err := json.MarshalIndent(ws, "", "  "); err == nil {
		_ = os.WriteFile(path, data, 0o644)
	}
}

// AddAny añade una carpeta si no esta ya. Devuelve el indice y si es nueva.
func (ws *Workspace) AddAny(dir string) (int, bool) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	for i, p := range ws.Projects {
		if samePath(p.Path, abs) {
			ws.Active = i
			return i, false
		}
	}
	if err := os.Chdir(abs); err != nil {
		return -1, false
	}
	ctx, err := project.Detect()
	if err != nil {
		return -1, false
	}
	ws.Projects = append(ws.Projects, &Project{
		Path: abs,
		Type: ctx.ProjectType.Label(),
	})
	ws.Active = len(ws.Projects) - 1
	ws.Save()
	return ws.Active, true
}

// Remove elimina un proyecto de la lista.
func (ws *Workspace) Remove(i int) {
	if i < 0 || i >= len(ws.Projects) {
		return
	}
	ws.Projects = append(ws.Projects[:i], ws.Projects[i+1:]...)
	if len(ws.Projects) == 0 {
		ws.Active = -1
	} else if ws.Active >= len(ws.Projects) {
		ws.Active = len(ws.Projects) - 1
	}
	ws.Save()
}

// ActiveProject devuelve el proyecto activo o nil.
func (ws *Workspace) ActiveProject() *Project {
	if ws.Active < 0 || ws.Active >= len(ws.Projects) {
		return nil
	}
	return ws.Projects[ws.Active]
}

func samePath(a, b string) bool {
	a, errA := filepath.Abs(a)
	b, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return a == b
	}
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}
