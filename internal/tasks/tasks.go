// Package tasks contiene el despachador de tareas (equivalente de
// src/main.rs::run_task_direct) y, en subarchivos, cada modulo de tarea.
//
// El despachador es fiel a la estructura Rust: cada combinacion (tarea, tipo de
// proyecto) se resuelve a una funcion registrada por su subarchivo.
package tasks

import (
	"fmt"

	"compass/internal/project"
)

// TaskFn es una tarea que opera sobre el contexto del proyecto.
type TaskFn func(ctx *project.ProjectContext) error

// registry mapea "tarea|tipo" -> funcion.
var registry = map[string]TaskFn{}

// Register da de alta una tarea para un tipo de proyecto concreto.
func Register(task string, pt project.ProjectType, fn TaskFn) {
	registry[fmt.Sprintf("%s|%d", task, pt)] = fn
}

// RunTaskDirect ejecuta una tarea por su nombre, ya resuelta la carpeta y el
// contexto. Devuelve error si la tarea no aplica o no esta implementada.
//
// Las tareas globales (context-menu*, opencode*, cline*, kilo*, antigravity*,
// vibe-setup) y context-gitignore se resuelven antes en cmd/compass-gui/main.go, así
// que aquí sólo llegan tareas de proyecto.
func RunTaskDirect(ctx *project.ProjectContext, task string) error {
	if task == "context-gitignore" {
		return fmt.Errorf("context-gitignore se gestiona antes de detectar el proyecto (usa --path)")
	}

	key := fmt.Sprintf("%s|%d", task, ctx.ProjectType)
	if fn, ok := registry[key]; ok {
		return fn(ctx)
	}
	return fmt.Errorf("tarea '%s' no implementada para proyecto %s", task, ctx.ProjectType.Label())
}
