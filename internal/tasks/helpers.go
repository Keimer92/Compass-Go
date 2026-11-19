package tasks

import (
	"fmt"
	"os"
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

// ensureWhatsappNpm instala las dependencias de un motor WhatsApp que viva
// dentro del propio proyecto. Si no hay package.json no hay motor local que
// instalar: el canónico vive en el corekit y su staging gestiona las deps.
func ensureWhatsappNpm(ctx *project.ProjectContext) {
	engine := ctx.ProjectType.WhatsappPath(ctx.ScriptDir)
	if !fileExists(filepath.Join(engine, "package.json")) {
		return
	}
	fmt.Println("[INFO] Instalando dependencias del motor WhatsApp...")
	runNpmInstall(engine)
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

// moveFile deja src en dst sobrescribiendo el destino. Prueba rename primero;
// si falla (p.ej. distinta unidad: temp en C:, artefacto en D:) copia y borra.
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
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

func commandPath(name string) string {
	ps := fmt.Sprintf(
		`$c = Get-Command -Name %s -ErrorAction SilentlyContinue | Where-Object { $_.CommandType -in @('Application','ExternalScript') } | Select-Object -First 1; if ($c) { $c.Path }`,
		shell.PsQuote(name),
	)
	out, err := shell.RunPS(ps)
	if err != nil {
		return ""
	}
	path := strings.TrimSpace(string(out))
	if path == "" || !filepath.IsAbs(path) {
		return ""
	}
	return path
}

// allTypes devuelve los 5 tipos de proyecto reconocidos (para registrar tareas
// globales por tipo).
func allTypes() []project.ProjectType {
	return []project.ProjectType{
		project.TypeFlutter,
		project.TypeKotlin,
		project.TypePython,
		project.TypeGo,
		project.TypeGoWails,
	}
}

// copyDir copia recursivamente src en dst (sobrescribiendo).
func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := copyDir(s, d); err != nil {
				return err
			}
		} else if err := copyFile(s, d); err != nil {
			return err
		}
	}
	return nil
}

// isNewer indica si el archivo a es mas reciente que b (o b no existe).
func isNewer(a, b string) bool {
	ia, err := os.Stat(a)
	if err != nil {
		return false
	}
	ib, err := os.Stat(b)
	if err != nil {
		return true
	}
	return ia.ModTime().After(ib.ModTime())
}
