package webui

import (
	"fmt"

	"compass/internal/project"
	"compass/internal/tasks"
	"compass/internal/menu"
)

// menuItems es el orden canónico de cards (mismo que la TUI/GUI anterior).
var menuItems = []menu.MenuItem{
	menu.WindowsDev, menu.AndroidDev, menu.AndroidHmr, menu.WindowsBuild,
	menu.AndroidBuildArm64, menu.AndroidBuildArmv7, menu.AndroidBuildAll,
	menu.InstallDebug, menu.FlutterProfile, menu.Tests,
	menu.Clean, menu.DebugAndroid, menu.Signing, menu.VersionItem,
	menu.Git, menu.ConfigureTerminal, menu.Environment,
	menu.VibeCoding,
}

var allMenuItems = menuItems

// hiddenMenuItems son cards retiradas del menu. Se deja el item en menuItems
// (y en el enum de menu) para no desplazar los indices que usan los temas y los
// IDs de pestaña, pero no se muestra nunca.
var hiddenMenuItems = map[menu.MenuItem]bool{
	menu.FlutterProfile: true, // modo --profile de Flutter retirado
}

// menuItem devuelve la card del indice. WindowsDev vale 0, asi que el valor
// cero NO puede usarse como centinela de "no existe".
func menuItem(i int) (menu.MenuItem, bool) {
	if i < 0 || i >= len(menuItems) {
		return 0, false
	}
	return menuItems[i], true
}

// Subcard es una opcion de un submenu nativo (Release / Git / Entorno).
type Subcard struct {
	Task        string `json:"task"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
}

// Subcards devuelve las subcards de una card, o nil si la card ejecuta su
// tarea directamente. Replica el comportamiento de la GUI Gio, que abria un
// dialogo nativo para estos cuatro casos.
func (b *Bridge) Subcards(taskID string) []Subcard {
	b.mu.Lock()
	ctx := b.ctx
	b.mu.Unlock()
	if ctx == nil {
		return nil
	}
	idx := 0
	if _, err := fmt.Sscanf(taskID, "%d", &idx); err != nil {
		return nil
	}
	item, ok := menuItem(idx)
	if !ok {
		return nil
	}
	switch item {
	case menu.WindowsBuild:
		if ctx.ProjectType == project.TypeGo || ctx.ProjectType == project.TypeGoWails {
			return []Subcard{
				{"windows-release-x64", "Windows x64", "64 bits · uso general", "run"},
				{"windows-release-all", "Todos", "Compila x86 + x64 + ARM64", "test"},
				{"windows-release-x86", "Windows x86", "32 bits · PCs y VM antiguas", "build"},
				{"windows-release-arm64", "Windows ARM64", "Snapdragon X · bajo consumo", "install"},
			}
		}
		if ctx.ProjectType == project.TypeFlutter {
			return []Subcard{
				{"windows-build", "Windows " + tasks.HostArchLabel(), "Build del release · equipo actual", "run"},
				{"flutter-portable", "Portable (un exe)", "Todo dentro · extrae al ejecutar", "install"},
				{"flutter-installer", "Instalador", "Setup.exe con todo dentro", "build"},
			}
		}
	case menu.AndroidBuildArm64, menu.AndroidBuildAll:
		return []Subcard{
			{"android-release-arm64", "Android arm64", "arm64-v8a · móviles modernos", "android"},
			{"android-release-armv7", "Android armv7", "armeabi-v7a · equipos antiguos", "apk32"},
			{"android-release-universal", "Android universal", "Una APK con todas las ABIs", "apk"},
			{"android-release-all", "Todos", "Compila las tres APK en serie", "aab"},
		}
	case menu.Git:
		return []Subcard{
			{"git-do-init", "Inicializar repositorio", "git init + rama main + primer commit", "build"},
			{"git-do-remote", "Conectar con remoto", "Enlazar con GitHub (URL o gh)", "git"},
			{"git-do-status", "Ver estado", "Rama, remoto y cambios", "debug"},
		}
	case menu.Clean:
		// Mismas acciones que el submenu de la TUI, pero como cards.
		opts := tasks.CleanupOptions()
		out := make([]Subcard, 0, len(opts))
		for _, o := range opts {
			out = append(out, Subcard{
				Task:        "clean-do-" + o.Key,
				Label:       o.Label,
				Description: o.Description,
				Icon:        o.ColorKey,
			})
		}
		return out
	case menu.Signing:
		// Mismas acciones que el submenu de la TUI, pero como cards.
		opts := tasks.SigningOptions()
		out := make([]Subcard, 0, len(opts))
		for _, o := range opts {
			out = append(out, Subcard{
				Task:        "signing-do-" + o.Key,
				Label:       o.Label,
				Description: o.Description,
				Icon:        o.ColorKey,
			})
		}
		return out
	case menu.Environment:
		return []Subcard{
			{"env-do-backup", "Backup completo", "Variables, inventario y archivos", "environment"},
			{"env-do-inventory", "Solo inventario", "Herramientas y versiones, sin copiar", "version"},
			{"env-do-restore", "Restaurar entorno", "Variables, rutas y rollback", "clean"},
			{"env-do-validate", "Validar backup", "Diff sin modificar nada", "debug"},
			{"env-do-open", "Abrir respaldos", "Carpeta de respaldos de Compass", "git"},
		}
	case menu.VibeCoding:
		return vibeSubcards()
	}
	return nil
}

// vibeSubcards lista las CLIs de Vibe Coding detectadas en la maquina, una
// subcard por herramienta, mas las acciones de mantenimiento. Las que no estan
// instaladas se muestran igual (para que se vea que existen) pero su tarea
// avisa en la pestana en vez de lanzar nada.
func vibeSubcards() []Subcard {
	tools := tasks.VibeTools()
	out := make([]Subcard, 0, len(tools)+2)
	for _, t := range tools {
		desc := "No instalada · usa 'Instalar faltantes'"
		if t.Found {
			desc = t.Path
		}
		out = append(out, Subcard{
			Task:        "vibe-run-" + t.Key,
			Label:       t.Title,
			Description: desc,
			Icon:        "vibe",
		})
	}
	return append(out,
		Subcard{"vibe-setup", "Instalar faltantes", "Detecta e instala las CLIs que falten", "install"},
		Subcard{"opencode-configure", "Configurar OpenCode", "Proveedor InferX y plugin de la TUI", "environment"},
	)
}

// taskFor mapea una card a su tarea CLI segun el tipo de proyecto.
func taskFor(item menu.MenuItem, pt project.ProjectType) string {
	switch item {
	case menu.WindowsDev:
		return "windows-dev"
	case menu.AndroidDev:
		return "android-dev"
	case menu.AndroidHmr:
		// Python recarga Uvicorn (android-hmr) y Go puro usa Air (go-hmr).
		// En el resto de tipos la card no se muestra (Label devuelve "").
		switch pt {
		case project.TypePython:
			return "android-hmr"
		case project.TypeGo:
			return "go-hmr"
		}
		return ""
	case menu.WindowsBuild:
		switch pt {
		case project.TypeGo, project.TypeGoWails:
			return "windows-release"
		default:
			return "windows-build"
		}
	case menu.AndroidBuildArm64:
		return "android-release"
	case menu.AndroidBuildAll:
		return "android-aab"
	case menu.InstallDebug:
		return "android-install-debug"
	case menu.Tests:
		switch pt {
		case project.TypePython:
			return "python-test"
		case project.TypeGo:
			return "go-test"
		}
	case menu.Clean:
		return "clean"
	case menu.DebugAndroid:
		return "debug-android"
	case menu.Signing:
		return "signing"
	case menu.VersionItem:
		return "version"
	case menu.Git:
		return "git"
	case menu.ConfigureTerminal:
		return "terminal"
	case menu.Environment:
		return "environment"
	case menu.VibeCoding:
		return "vibe"
	}
	return ""
}
