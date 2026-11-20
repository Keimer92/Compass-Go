// Package menu define las cards del menu de Compass: que existe, como se
// llama y como se describe segun el tipo de proyecto.
//
// Es solo el modelo de datos: la pinta la GUI web (internal/webui). Aqui no
// hay nada de terminal ni de tcell.
package menu

import (
	"compass/internal/project"
)

// MenuItem identifica cada card del menu.
type MenuItem int

const (
	// DEV
	WindowsDev MenuItem = iota
	AndroidDev
	AndroidHmr
	WindowsBuild
	AndroidBuildArm64
	AndroidBuildArmv7
	AndroidBuildAll
	InstallDebug
	FlutterProfile
	Tests
	// MNT
	Clean
	DebugAndroid
	Signing
	VersionItem
	// CFG
	Git
	ConfigureTerminal
	Environment
	// VibeCoding se anade al final a proposito: los indices de las cards ya
	// existentes no se mueven (los usan los temas y los IDs de pestana).
	VibeCoding
)

// Icon devuelve el emoji de la card.
func (m MenuItem) Icon() string {
	switch m {
	case WindowsDev:
		return "\U0001F5A5"
	case AndroidDev:
		return "\U0001F4F1"
	case AndroidHmr:
		return "\U0001F525"
	case WindowsBuild:
		return "\U0001F4E6"
	case AndroidBuildArm64, AndroidBuildArmv7, AndroidBuildAll:
		return "\U0001F680"
	case InstallDebug:
		return "\U0001F4E6"
	case FlutterProfile:
		return "\U0001F3AF"
	case Tests:
		return "\U0001F9EA"
	case Clean:
		return "\U0001F9F9"
	case DebugAndroid:
		return "\U0001F50D"
	case Signing:
		return "\U0001F511"
	case VersionItem:
		return "\U0001F516"
	case Git:
		return "\U0001F527"
	case ConfigureTerminal:
		return "\u2699"
	case Environment:
		return "\U0001F4BE"
	case VibeCoding:
		return "\U0001F916"
	}
	return ""
}

// Section devuelve la seccion ("DEV" | "MNT" | "CFG") de la card.
func (m MenuItem) Section() string {
	switch m {
	case WindowsDev, AndroidDev, AndroidHmr, WindowsBuild, AndroidBuildArm64,
		AndroidBuildArmv7, AndroidBuildAll, InstallDebug, FlutterProfile, Tests:
		return "DEV"
	// Vibe Coding es una herramienta de desarrollo, no de configuracion: va en
	// la pestana principal para que se vea sin tener que entrar en Config.
	case VibeCoding:
		return "DEV"
	case Clean, DebugAndroid, Signing, VersionItem:
		return "MNT"
	default:
		return "CFG"
	}
}

// Platform clasifica la card por destino para pintar un distintivo extra en la
// GUI: "windows", "android", "go" (tareas de un proyecto Go puro) o "general"
// (utilidades y tareas del proyecto).
func (m MenuItem) Platform(pt project.ProjectType) string {
	// En Go puro, Run / Win Release / HMR / Test son tareas del propio Go
	// (go run, go build, air, go test), no tareas de Windows ni de Android.
	if pt == project.TypeGo {
		switch m {
		case WindowsDev, WindowsBuild, AndroidHmr, Tests:
			return "go"
		}
	}
	switch m {
	case WindowsDev, WindowsBuild:
		return "windows"
	case AndroidDev, AndroidBuildArm64, AndroidBuildArmv7, AndroidBuildAll,
		InstallDebug, DebugAndroid:
		return "android"
	default:
		return "general"
	}
}

// IsAndroid indica si la card necesita el modulo Android del proyecto
// (build/android en Wails, android/ en Flutter, la raiz en Kotlin). Se usa
// para no ofrecer builds de Android a proyectos que no lo tienen generado.
//
// AndroidHmr solo cuenta como Android en Flutter/Kotlin/Wails: en Go puro y
// Python esa misma card es el reload del servidor de desarrollo.
func (m MenuItem) IsAndroid(pt project.ProjectType) bool {
	switch m {
	case AndroidDev, AndroidBuildArm64, AndroidBuildArmv7, AndroidBuildAll,
		InstallDebug, DebugAndroid, Signing:
		return true
	case AndroidHmr:
		return pt == project.TypeFlutter || pt == project.TypeKotlin ||
			pt == project.TypeGoWails
	}
	return false
}

// ColorKey es la clave del acento en theme.CardKeys.
func (m MenuItem) ColorKey() string {
	switch m {
	case WindowsDev:
		return "run"
	case AndroidDev:
		return "android"
	case AndroidHmr:
		return "reload"
	case WindowsBuild:
		return "build"
	case AndroidBuildArm64:
		return "apk"
	case AndroidBuildArmv7:
		return "apk32"
	case AndroidBuildAll:
		return "aab"
	case InstallDebug:
		return "install"
	case FlutterProfile:
		return "profile"
	case Tests:
		return "test"
	case Clean:
		return "clean"
	case DebugAndroid:
		return "debug"
	case Signing:
		return "sign"
	case VersionItem:
		return "version"
	case Git:
		return "git"
	case ConfigureTerminal:
		return "terminal"
	case Environment:
		return "environment"
	case VibeCoding:
		// "vibe" no esta en theme.CardKeys (es un arreglo fijo de 18 posiciones
		// que casa con los 20 temas y anadir una clave obligaria a tocar todos).
		// El tema cae en su acento global y el icono lo aporta icons.ts.
		return "vibe"
	}
	return "terminal"
}

// Label devuelve la etiqueta corta de la card segun el tipo de proyecto.
func (m MenuItem) Label(pt project.ProjectType) string {
	switch m {
	case WindowsDev:
		switch pt {
		case project.TypePython, project.TypeGo:
			return "Run"
		}
		return "Windows"
	case AndroidDev:
		if pt == project.TypeGoWails {
			return "And. Dev"
		}
		return "Android"
	case AndroidHmr:
		// Go puro (air -> go-hmr) y Python (recarga Uvicorn) tienen una tarea
		// real detras. Flutter/Kotlin/Wails no: la card se oculta (GetMenu
		// descarta las cards sin label).
		switch pt {
		case project.TypePython:
			return "Reload"
		case project.TypeGo:
			return "HMR"
		}
		return ""
	case WindowsBuild:
		switch pt {
		case project.TypeGo, project.TypeGoWails:
			return "Win Release"
		case project.TypePython:
			return "Package"
		}
		return "Win Build"
	case AndroidBuildArm64:
		switch pt {
		case project.TypeFlutter, project.TypeKotlin, project.TypeGoWails:
			return "APK Release"
		}
		return ""
	case AndroidBuildArmv7:
		return ""
	case AndroidBuildAll:
		switch pt {
		case project.TypeFlutter, project.TypeKotlin, project.TypeGoWails:
			return "AAB"
		}
		return ""
	case InstallDebug:
		return "Install"
	case FlutterProfile:
		return "Profile"
	case Tests:
		if pt == project.TypePython || pt == project.TypeGo {
			return "Test"
		}
		return ""
	case Clean:
		return "Clean"
	case DebugAndroid:
		return "Debug Andr"
	case Signing:
		// Solo hay firma Android en Flutter/Kotlin/Go+Wails; en Go puro y
		// Python la tarea solo respondia "No aplicable".
		if pt == project.TypeGo || pt == project.TypePython {
			return ""
		}
		return "Signing"
	case VersionItem:
		return "Version"
	case Git:
		return "Git"
	case ConfigureTerminal:
		return "Terminal"
	case Environment:
		return "Entorno"
	case VibeCoding:
		// Disponible en todos los tipos de proyecto: las CLIs de IA trabajan
		// sobre cualquier carpeta.
		return "Vibe Coding"
	}
	return ""
}

// Description devuelve la descripcion de la card segun el tipo de proyecto.
func (m MenuItem) Description(pt project.ProjectType) string {
	// Vibe Coding no depende del tipo de proyecto: el texto es el mismo en
	// todos, asi que se resuelve antes del switch por plataforma.
	if m == VibeCoding {
		return "CLIs de IA en una pestaña"
	}
	switch pt {
	case project.TypeFlutter:
		switch m {
		case WindowsDev:
			return "flutter run windows"
		case AndroidDev:
			return "flutter run android"
		case WindowsBuild:
			return "Solo host (x64)"
		case AndroidBuildArm64:
			return "arm64 · armv7 · universal"
		case AndroidBuildAll:
			return "Bundle para Play Store"
		case Clean:
			return "Limpia builds/deps"
		case DebugAndroid:
			return "Depurar Android"
		case Signing:
			return "Firmas y huellas"
		case Git:
			return "Init · remoto · estado"
		case ConfigureTerminal:
			return "Autocompletado"
		case Environment:
			return "Backup y restauracion"
		case FlutterProfile:
			return "flutter run --profile"
		}
	case project.TypeKotlin:
		switch m {
		case WindowsDev:
			return "gradlew :composeApp:run"
		case AndroidDev:
			return "gradlew :installDebug"
		case WindowsBuild:
			return "Solo host (x64)"
		case AndroidBuildArm64:
			return "arm64 · armv7 · universal"
		case AndroidBuildAll:
			return "Bundle para Play Store"
		case InstallDebug:
			return "gradlew :installDebug"
		case Clean:
			return "Limpia builds/deps"
		case DebugAndroid:
			return "Depurar Android"
		case Signing:
			return "Firmas y huellas"
		case Git:
			return "Init · remoto · estado"
		case ConfigureTerminal:
			return "Autocompletado"
		case Environment:
			return "Backup y restauracion"
		}
	case project.TypeGoWails:
		switch m {
		case WindowsDev:
			return "dev escritorio"
		case AndroidDev:
			return "APK dev"
		case AndroidHmr:
			return "HMR (Vite)"
		case WindowsBuild:
			return "x86 · x64 · ARM64"
		case AndroidBuildArm64:
			return "arm64 · armv7 · universal"
		case AndroidBuildAll:
			return "Bundle para Play Store"
		case InstallDebug:
			return "APK dev"
		case Clean:
			return "Limpia builds/deps"
		case DebugAndroid:
			return "logcat de la app"
		case Signing:
			return "Firmas y huellas"
		case Git:
			return "Init · remoto · estado"
		case VersionItem:
			return "main.go + config.yml + metadatos"
		case ConfigureTerminal:
			return "Autocompletado"
		case Environment:
			return "Backup y restauracion"
		}
	case project.TypePython:
		switch m {
		case WindowsDev:
			return "python <script/module>"
		case AndroidHmr:
			return "Uvicorn ASGI --reload"
		case WindowsBuild:
			return "python -m build"
		case Clean:
			return "Limpia caches y builds Python"
		case VersionItem:
			return "Actualiza pyproject.toml"
		case Git:
			return "Init · remoto · estado"
		case ConfigureTerminal:
			return "Autocompletado"
		case Environment:
			return "Backup y restauracion"
		case Tests:
			return "python -m pytest"
		}
	case project.TypeGo:
		switch m {
		case WindowsDev:
			return "go run ."
		case AndroidHmr:
			return "air (rebuild + restart)"
		case WindowsBuild:
			return "x86 · x64 · ARM64"
		case Clean:
			return "Limpia binarios"
		case Git:
			return "Init · remoto · estado"
		case ConfigureTerminal:
			return "Autocompletado"
		case Environment:
			return "Backup y restauracion"
		case Tests:
			return "go test ./..."
		}
	}
	return ""
}

