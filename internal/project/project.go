// Package project detecta el tipo de proyecto y construye el ProjectContext.
//
// Es el equivalente de src/utils/context.rs en Rust: clasifica un directorio
// como Flutter / Kotlin / Python / Go / Go+Wails, localiza el Android SDK y
// rellena los campos que las tareas necesitan.
package project

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ProjectType es el tipo de proyecto reconocido por Compass.
type ProjectType int

const (
	TypeFlutter ProjectType = iota
	TypeKotlin
	TypePython
	TypeGo
	// TypeGoWails es Go + Wails v3 (go.mod con wailsapp/wails + Taskfile.yml).
	TypeGoWails
)

func (t ProjectType) Label() string {
	switch t {
	case TypeFlutter:
		return "Flutter"
	case TypeKotlin:
		return "Kotlin"
	case TypePython:
		return "Python"
	case TypeGo:
		return "Go"
	case TypeGoWails:
		return "Go + Wails"
	default:
		return "Desconocido"
	}
}

// WhatsappPath devuelve la ruta del motor de WhatsApp segun el tipo de proyecto.
func (t ProjectType) WhatsappPath(root string) string {
	return filepath.Join(root, "whatsapp_engine")
}

// WhatsappBundlePath devuelve la ruta del bundle compilado del motor WhatsApp.
func (t ProjectType) WhatsappBundlePath(root string) string {
	switch t {
	case TypeFlutter:
		// Flutter usa un unico index.js autocontenido (modo --single) dentro de
		// whatsapp_engine/, que windows/CMakeLists.txt empaqueta junto al exe.
		return filepath.Join(root, "whatsapp_engine")
	case TypeKotlin:
		return filepath.Join(root, "composeApp", "resources", "whatsapp-engine-bundle")
	default:
		return filepath.Join(root, "resources", "whatsapp-engine-bundle")
	}
}

// ProjectContext es la informacion de un proyecto ya detectado.
type ProjectContext struct {
	ScriptDir         string
	ProjectType       ProjectType
	AndroidHome       string // "" si no esta configurado
	NdkHome           string
	AdbPath           string
	SdkManager        string
	HasNodeModules    bool
	HasAndroidProject bool
	HasWhatsAppEngine bool
}

// WindowTitle devuelve solo el nombre del directorio del proyecto (como titulo
// de ventana/pestana). El tipo va en el badge del TUI.
func (c *ProjectContext) WindowTitle() string {
	return filepath.Base(c.ScriptDir)
}

// AssertAndroidSdk devuelve error si ANDROID_HOME no esta configurado.
func (c *ProjectContext) AssertAndroidSdk() error {
	if c.AndroidHome == "" {
		return &DetectError{Msg: "[ERROR] ANDROID_HOME no está configurado.\n" +
			"   Asegúrate de tener instalado Android SDK, o define ANDROID_HOME."}
	}
	return nil
}

// DetectError es el error de deteccion de proyecto.
type DetectError struct{ Msg string }

func (e *DetectError) Error() string { return e.Msg }

const ndkVersion = "27.0.12077973"

// Detect localiza la raiz del proyecto (subiendo desde el CWD) y construye el
// ProjectContext completo.
func Detect() (*ProjectContext, error) {
	start, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	root, pt, err := detectProjectRoot(start)
	if err != nil {
		return nil, err
	}

	androidHome := detectAndroidHome()
	var ndkHome, adbPath, sdkManager string
	if androidHome != "" {
		ndkHome = filepath.Join(androidHome, "ndk", ndkVersion)
		adbPath = filepath.Join(androidHome, "platform-tools", "adb.exe")
		sdkManager = findSdkmanager(androidHome)
	}

	hasNodeModules := dirExists(filepath.Join(root, "node_modules"))
	hasAndroid := androidProjectPresent(pt, root)
	// Motor propio del proyecto: solo cuenta si trae la fuente (engine.js). La
	// carpeta whatsapp_engine/ tambien la crea Compass al etajar el motor
	// canónico (index.js + node.exe), y eso no es un motor local.
	hasWhatsApp := hasLocalWhatsappEngine(pt, root)

	return &ProjectContext{
		ScriptDir:         root,
		ProjectType:       pt,
		AndroidHome:       androidHome,
		NdkHome:           ndkHome,
		AdbPath:           adbPath,
		SdkManager:        sdkManager,
		HasNodeModules:    hasNodeModules,
		HasAndroidProject: hasAndroid,
		HasWhatsAppEngine: hasWhatsApp,
	}, nil
}

// hasLocalWhatsappEngine indica si el proyecto trae la fuente del motor
// (whatsapp_engine/engine.js), es decir, un motor propio que hay que compilar.
// La carpeta whatsapp_engine/ también la crea Compass al etajar el motor
// canónico (index.js + node.exe), y eso NO es un motor local.
func hasLocalWhatsappEngine(pt ProjectType, root string) bool {
	return fileExists(filepath.Join(pt.WhatsappPath(root), "engine.js"))
}

// androidProjectPresent indica si el proyecto tiene un modulo Android ya
// generado, segun su tipo.
func androidProjectPresent(pt ProjectType, root string) bool {
	switch pt {
	case TypeFlutter:
		return gradlewAt(filepath.Join(root, "android"))
	case TypeKotlin:
		return gradlewAt(root)
	case TypeGoWails:
		return gradlewAt(filepath.Join(root, "build", "android"))
	default:
		return false
	}
}

func gradlewAt(dir string) bool {
	return fileExists(filepath.Join(dir, "gradlew.bat"))
}

// ClassifyDir decide si `d` es la raiz de un proyecto y de que tipo.
func ClassifyDir(d string) (ProjectType, bool) {
	if fileExists(filepath.Join(d, "pubspec.yaml")) {
		return TypeFlutter, true
	}
	if isWailsV3(d) {
		return TypeGoWails, true
	}
	// Kotlin requiere gradlew.bat y settings.gradle.kts.
	if gradlewAt(d) && fileExists(filepath.Join(d, "settings.gradle.kts")) {
		return TypeKotlin, true
	}
	if isPythonProject(d) {
		return TypePython, true
	}
	if fileExists(filepath.Join(d, "go.mod")) {
		return TypeGo, true
	}
	return 0, false
}

// isWailsV3 reconoce Go + Wails v3 (dependencia wailsapp/wails en go.mod y
// Taskfile.yml o build/config.yml).
func isWailsV3(d string) bool {
	data, err := os.ReadFile(filepath.Join(d, "go.mod"))
	if err != nil {
		return false
	}
	if !strings.Contains(string(data), "wailsapp/wails") {
		return false
	}
	return fileExists(filepath.Join(d, "Taskfile.yml")) ||
		fileExists(filepath.Join(d, "build", "config.yml"))
}

func isPythonProject(d string) bool {
	data, err := os.ReadFile(filepath.Join(d, "pyproject.toml"))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		t := strings.TrimSpace(line)
		if t == "[project]" || t == "[tool.poetry]" {
			return true
		}
	}
	return false
}

func detectProjectRoot(start string) (string, ProjectType, error) {
	dir := start
	for {
		if pt, ok := ClassifyDir(dir); ok {
			return dir, pt, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", 0, &DetectError{Msg: "No se encontró un proyecto Flutter/Dart, " +
		"Kotlin Multiplatform, Go, Python o Go + Wails en este directorio ni en sus padres.\n" +
		"   Usa el selector con 'compass --pick' para elegir la carpeta del proyecto."}
}

func detectAndroidHome() string {
	for _, key := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
		if v := os.Getenv(key); v != "" {
			if dirExists(v) {
				return v
			}
		}
	}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		cand := filepath.Join(local, "Android", "Sdk")
		if dirExists(cand) {
			return cand
		}
	}
	if up := os.Getenv("USERPROFILE"); up != "" {
		cand := filepath.Join(up, "AppData", "Local", "Android", "Sdk")
		if dirExists(cand) {
			return cand
		}
	}
	return ""
}

// findSdkmanager localiza sdkmanager dentro de $ANDROID_HOME/cmdline-tools,
// probando primero "latest" y luego la carpeta versionada mas alta.
func findSdkmanager(androidHome string) string {
	binName := "sdkmanager.exe"
	root := filepath.Join(androidHome, "cmdline-tools")

	latest := filepath.Join(root, "latest", "bin", binName)
	if fileExists(latest) {
		return latest
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	type ver struct {
		key []int
		bin string
	}
	var versions []ver
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		var key []int
		for _, p := range strings.Split(name, ".") {
			n, err := strconvAtoi(p)
			if err != nil {
				key = nil
				break
			}
			key = append(key, n)
		}
		if len(key) == 0 {
			continue
		}
		bin := filepath.Join(root, name, "bin", binName)
		if fileExists(bin) {
			versions = append(versions, ver{key: key, bin: bin})
		}
	}
	if len(versions) == 0 {
		return ""
	}
	sort.Slice(versions, func(i, j int) bool {
		a, b := versions[i].key, versions[j].key
		n := len(a)
		if len(b) < n {
			n = len(b)
		}
		for k := 0; k < n; k++ {
			if a[k] != b[k] {
				return a[k] > b[k]
			}
		}
		return len(a) > len(b)
	})
	return versions[0].bin
}

func strconvAtoi(s string) (int, error) {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, &DetectError{Msg: "no es digito"}
		}
		n = n*10 + int(r-'0')
	}
	if s == "" {
		return 0, &DetectError{Msg: "vacio"}
	}
	return n, nil
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
