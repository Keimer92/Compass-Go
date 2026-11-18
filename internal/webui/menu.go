package webui

import (
	"fmt"

	"compass/internal/project"
	"compass/internal/tui"
)

// menuItems es el orden canónico de cards (mismo que la TUI/GUI anterior).
var menuItems = []tui.MenuItem{
	tui.WindowsDev, tui.AndroidDev, tui.AndroidHmr, tui.WindowsBuild,
	tui.AndroidBuildArm64, tui.AndroidBuildArmv7, tui.AndroidBuildAll,
	tui.InstallDebug, tui.FlutterProfile, tui.Tests,
	tui.Clean, tui.DebugAndroid, tui.Signing, tui.VersionItem,
	tui.Git, tui.BuildWhatsApp, tui.ConfigureTerminal, tui.Environment,
}

var allMenuItems = menuItems

// hiddenMenuItems son cards retiradas del menu. Se deja el item en menuItems
// (y en el enum de tui) para no desplazar los indices que usan los temas y los
// IDs de pestaña, pero no se muestra nunca.
var hiddenMenuItems = map[tui.MenuItem]bool{
	tui.FlutterProfile: true, // modo --profile de Flutter retirado
}

// menuItem devuelve la card del indice. WindowsDev vale 0, asi que el valor
// cero NO puede usarse como centinela de "no existe".
func menuItem(i int) (tui.MenuItem, bool) {
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
	case tui.WindowsBuild:
		if ctx.ProjectType == project.TypeGo || ctx.ProjectType == project.TypeGoWails {
			return []Subcard{
				{"windows-release-x64", "Windows x64", "64 bits · uso general", "run"},
				{"windows-release-all", "Todos", "Compila x86 + x64 + ARM64", "test"},
				{"windows-release-x86", "Windows x86", "32 bits · PCs y VM antiguas", "build"},
				{"windows-release-arm64", "Windows ARM64", "Snapdragon X · bajo consumo", "install"},
			}
		}
	case tui.AndroidBuildArm64, tui.AndroidBuildAll:
		return []Subcard{
			{"android-release-arm64", "Android arm64", "arm64-v8a · móviles modernos", "android"},
			{"android-release-armv7", "Android armv7", "armeabi-v7a · equipos antiguos", "apk32"},
			{"android-release-universal", "Android universal", "Una APK con todas las ABIs", "apk"},
			{"android-release-all", "Todos", "Compila las tres APK en serie", "aab"},
		}
	case tui.Git:
		return []Subcard{
			{"git-do-init", "Inicializar repositorio", "git init + rama main + primer commit", "build"},
			{"git-do-gitignore", "Crear/actualizar .gitignore", "Plantilla por tipo de proyecto", "clean"},
			{"git-do-remote", "Conectar con remoto", "Enlazar con GitHub (URL o gh)", "git"},
			{"git-do-status", "Ver estado", "Rama, remoto y cambios", "debug"},
		}
	case tui.Environment:
		return []Subcard{
			{"env-do-backup", "Backup completo", "Variables, inventario y archivos", "environment"},
			{"env-do-inventory", "Solo inventario", "Herramientas y versiones, sin copiar", "version"},
			{"env-do-restore", "Restaurar entorno", "Variables, rutas y rollback", "clean"},
			{"env-do-validate", "Validar backup", "Diff sin modificar nada", "debug"},
			{"env-do-open", "Abrir respaldos", "Carpeta de respaldos de Compass", "git"},
		}
	}
	return nil
}

// taskFor mapea una card a su tarea CLI segun el tipo de proyecto.
func taskFor(item tui.MenuItem, pt project.ProjectType) string {
	switch item {
	case tui.WindowsDev:
		return "windows-dev"
	case tui.AndroidDev:
		return "android-dev"
	case tui.AndroidHmr:
		switch pt {
		case project.TypeGoWails, project.TypePython:
			return "android-hmr"
		case project.TypeGo:
			return "go-hmr"
		}
	case tui.WindowsBuild:
		switch pt {
		case project.TypeGo, project.TypeGoWails:
			return "windows-release"
		default:
			return "windows-build"
		}
	case tui.AndroidBuildArm64:
		return "android-release"
	case tui.AndroidBuildAll:
		return "android-aab"
	case tui.InstallDebug:
		return "android-install-debug"
	case tui.Tests:
		switch pt {
		case project.TypePython:
			return "python-test"
		case project.TypeGo:
			return "go-test"
		}
	case tui.Clean:
		return "clean"
	case tui.DebugAndroid:
		return "debug-android"
	case tui.Signing:
		return "signing"
	case tui.VersionItem:
		return "version"
	case tui.Git:
		return "git"
	case tui.BuildWhatsApp:
		return "whatsapp"
	case tui.ConfigureTerminal:
		return "terminal"
	case tui.Environment:
		return "environment"
	}
	return ""
}
