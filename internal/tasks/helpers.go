package tasks

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"compass/internal/project"
	"compass/internal/shell"
)

// runNpmInstall instala dependencias npm en dir (si package.json existe).
func runNpmInstall(dir string) {
	if err := shell.RunNpmInstall(dir); err != nil {
		fmt.Printf("[WARN] %v\n", err)
	}
}

// ensureWhatsappNpm instala las dependencias del motor WhatsApp si el proyecto
// lo tiene.
func ensureWhatsappNpm(ctx *project.ProjectContext) {
	if ctx.HasWhatsAppEngine {
		engine := ctx.ProjectType.WhatsappPath(ctx.ScriptDir)
		fmt.Println("[INFO] Instalando dependencias del motor WhatsApp...")
		runNpmInstall(engine)
	}
}

// fmtWarn imprime un aviso en stderr (consistente con el resto de tareas).
func fmtWarn(format string, args ...any) {
	fmt.Printf("[WARN] "+format+"\n", args...)
}

// gradlewCmd devuelve el comando del wrapper Gradle de Windows.
func gradlewCmd(args string) string {
	return ".\\gradlew.bat " + args
}

// kotlinAndroidDir devuelve el directorio Android del proyecto Kotlin
// (composeApp por defecto, app si existe).
func kotlinAndroidDir(ctx *project.ProjectContext) string {
	app := filepath.Join(ctx.ScriptDir, "app")
	compose := filepath.Join(ctx.ScriptDir, "composeApp")
	if dirExists(app) && !dirExists(compose) {
		return app
	}
	return compose
}

// gradleModule devuelve ":app" o ":composeApp" segun el directorio Android.
func gradleModule(ctx *project.ProjectContext) string {
	if filepath.Base(kotlinAndroidDir(ctx)) == "app" {
		return ":app"
	}
	return ":composeApp"
}

func sanitizeAppName(name string) string {
	var b strings.Builder
	for _, ch := range name {
		switch {
		case ch >= 'a' && ch <= 'z', ch >= 'A' && ch <= 'Z', ch >= '0' && ch <= '9', ch == '-', ch == '_':
			b.WriteRune(ch)
		case ch == ' ':
			b.WriteRune('_')
		default:
			b.WriteRune('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

// readCargoMeta lee name/version de [package] en Cargo.toml.
func readCargoMeta(ctx *project.ProjectContext) (string, string) {
	data, err := os.ReadFile(filepath.Join(ctx.ScriptDir, "Cargo.toml"))
	if err != nil {
		return "app", ""
	}
	var name, version string
	inPackage := false
	for _, line := range strings.Split(string(data), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "[") {
			inPackage = t == "[package]"
			continue
		}
		if !inPackage {
			continue
		}
		if k, v, ok := strings.Cut(t, "="); ok {
			k = strings.TrimSpace(k)
			v = strings.TrimSpace(v)
			v = strings.Trim(v, "\"'")
			switch k {
			case "name":
				name = v
			case "version":
				version = v
			}
		}
	}
	if name == "" {
		name = "app"
	}
	return name, version
}

// readPubspecMeta lee name/version de pubspec.yaml (Flutter).
func readPubspecMeta(ctx *project.ProjectContext) (string, string) {
	data, err := os.ReadFile(filepath.Join(ctx.ScriptDir, "pubspec.yaml"))
	if err != nil {
		return sanitizeAppName(filepath.Base(ctx.ScriptDir)), ""
	}
	var name, version string
	for _, line := range strings.Split(string(data), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "name:") && name == "" {
			name = strings.Trim(strings.TrimPrefix(t, "name:"), "'\"")
		}
		if strings.HasPrefix(t, "version:") && version == "" {
			v := strings.Trim(strings.TrimPrefix(t, "version:"), "'\"")
			version = strings.Split(v, "+")[0]
		}
		if name != "" && version != "" {
			break
		}
	}
	if name == "" {
		name = sanitizeAppName(filepath.Base(ctx.ScriptDir))
	}
	return name, version
}

// readTauriMeta lee productName/version de src-tauri/tauri.conf.json.
func readTauriMeta(ctx *project.ProjectContext) (string, string) {
	path := filepath.Join(ctx.ScriptDir, "src-tauri", "tauri.conf.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return filepath.Base(ctx.ScriptDir), ""
	}
	name := extractJSONField(string(data), "productName")
	version := extractJSONField(string(data), "version")
	return name, version
}

// extractJSONField extrae un valor de cadena de un JSON plano (sin estructuras).
func extractJSONField(jsonStr, key string) string {
	// Busca "key": "value" de forma tolerante.
	idx := strings.Index(jsonStr, "\""+key+"\"")
	if idx < 0 {
		return ""
	}
	rest := jsonStr[idx+len(key)+3:] // salta "key":
	if i := strings.Index(rest, "\""); i >= 0 {
		end := strings.Index(rest[i+1:], "\"")
		if end >= 0 {
			return rest[i+1 : i+1+end]
		}
	}
	return ""
}

// tauriCommand construye el comando del CLI de Tauri (npx para v2, cargo como
// fallback).
func tauriCommand(ctx *project.ProjectContext, sub string) string {
	if dirExists(filepath.Join(ctx.ScriptDir, "node_modules", "@tauri-apps", "cli")) {
		return "npx tauri " + sub
	}
	return "cargo tauri " + sub
}

// tauriEnsure verifica que el CLI de Tauri esta disponible.
func tauriEnsure(ctx *project.ProjectContext) error {
	if dirExists(filepath.Join(ctx.ScriptDir, "node_modules", "@tauri-apps", "cli")) {
		return nil
	}
	if _, err := exec.LookPath("cargo"); err == nil {
		return nil
	}
	return fmt.Errorf("no se encontro el CLI de Tauri (node_modules/@tauri-apps/cli ni cargo tauri)")
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// copyFile copia un archivo de forma binaria (para APKs y artefactos).
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

// dataDir devuelve %LOCALAPPDATA% (carpeta de datos de la aplicacion).
func dataDir() string {
	if v := os.Getenv("LOCALAPPDATA"); v != "" {
		return v
	}
	if v := os.Getenv("USERPROFILE"); v != "" {
		return filepath.Join(v, "AppData", "Local")
	}
	return ""
}

// homeDir devuelve %USERPROFILE%.
func homeDir() string {
	if v := os.Getenv("USERPROFILE"); v != "" {
		return v
	}
	if v := os.Getenv("HOME"); v != "" {
		return v
	}
	return ""
}

// findGradlew devuelve la ruta del wrapper de Gradle (gradlew.bat) en dir, o "".
func findGradlew(dir string) string {
	cand := filepath.Join(dir, "gradlew.bat")
	if fileExists(cand) {
		return cand
	}
	return ""
}

// commandExists indica si 'name' esta disponible como comando (via PowerShell
// Get-Command).
func commandExists(name string) bool {
	ps := fmt.Sprintf("$null -ne (Get-Command -Name '%s' -ErrorAction SilentlyContinue)", name)
	out, err := shell.RunPS(ps)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "True"
}

// allTypes devuelve los 7 tipos de proyecto reconocidos (para registrar tareas
// globales por tipo).
func allTypes() []project.ProjectType {
	return []project.ProjectType{
		project.TypeTauri,
		project.TypeFlutter,
		project.TypeKotlin,
		project.TypePython,
		project.TypeRust,
		project.TypeGo,
		project.TypeGoWails,
	}
}
