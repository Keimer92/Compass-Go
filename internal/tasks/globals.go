package tasks

// globals.go despacha las tareas globales (no dependen de un proyecto): menú
// contextual de Windows, OpenCode y las herramientas de vibe coding. Equivalente
// de src/main.rs::run_global_task.

import "strings"

// RunGlobalTask ejecuta una tarea global por su nombre. Devuelve error si la
// tarea no es global o no está implementada.
func RunGlobalTask(task string) error {
	switch task {
	// ── Menú contextual de Windows ──
	case "context-menu":
		return contextMenuToggle()
	case "context-menu-install":
		return contextMenuInstall()
	case "context-menu-uninstall":
		return contextMenuUninstall()

	// ── OpenCode ──
	// `opencode` (a secas) abre el menu interactivo, igual que en Rust; el
	// alias `opencode-configure` configura el proveedor/plugin directamente.
	case "opencode":
		return contextMenuToggle()
	case "opencode-configure":
		return ConfigureOpencode(true)
	case "opencode-install":
		return installOpencode()
	case "opencode-uninstall":
		return uninstallOpencode()

	// ── Herramientas de vibe coding (instalan/desinstalan la cascada) ──
	case "cline-install":
		return installCline()
	case "cline-uninstall":
		return uninstallCline()
	case "kilo-install":
		return installKilo()
	case "kilo-uninstall":
		return uninstallKilo()
	case "antigravity-install":
		return installAntigravity()
	case "antigravity-uninstall":
		return uninstallAntigravity()
	case "vibe-setup":
		// Flujo completo del instalador: detectar/instalar CLIs, configurar
		// OpenCode y montar la cascada (equivale a context_menu::setup).
		return ContextMenuSetup()
	}
	return &globalTaskError{task: task}
}

// IsGlobalTask indica si la tarea es global (no necesita un proyecto).
func IsGlobalTask(task string) bool {
	if task == "context-gitignore" {
		return false // opera sobre un directorio concreto, no global
	}
	switch {
	case strings.HasPrefix(task, "context-menu"),
		strings.HasPrefix(task, "opencode"),
		strings.HasPrefix(task, "cline"),
		strings.HasPrefix(task, "kilo"),
		strings.HasPrefix(task, "antigravity"),
		strings.HasPrefix(task, "vibe"):
		return true
	}
	return false
}

type globalTaskError struct{ task string }

func (e *globalTaskError) Error() string {
	return "tarea global desconocida: '" + e.task + "'"
}
