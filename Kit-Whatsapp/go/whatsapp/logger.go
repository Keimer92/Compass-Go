package whatsapp

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Paridad con commands/whatsapp.rs: un único whatsapp.log en AppData con
// rotación de 5 MB y 3 archivos históricos.
const (
	logMaxBytes  = 5 * 1024 * 1024
	logMaxFiles  = 3
	diagMaxBytes = 10 * 1024 * 1024
)

var (
	logPathOnce sync.Once
	logPath     string

	diagPathOnce sync.Once
	diagPath     string

	diagMu       sync.Mutex
	diagLastLine string
	diagRepeat   int
	diagStarted  bool
)

func logFilePath() string {
	logPathOnce.Do(func() {
		dir := cfg.DataDir
		_ = os.MkdirAll(dir, 0o755)
		logPath = filepath.Join(dir, "whatsapp.log")
	})
	return logPath
}

// desktopDir resuelve el Escritorio real del usuario (incluye el caso en que
// OneDrive lo redirige). Si no existe ninguno, cae en la carpeta del usuario.
func desktopDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	candidates := make([]string, 0, 5)
	if od := strings.TrimSpace(os.Getenv("OneDrive")); od != "" {
		candidates = append(candidates, filepath.Join(od, "Desktop"))
	}
	candidates = append(candidates,
		filepath.Join(home, "Desktop"),
		filepath.Join(home, "OneDrive", "Desktop"),
		filepath.Join(home, "Escritorio"),
	)
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}
	return home
}

// diagFilePath es el log pensado para que el usuario lo envíe: en el Escritorio
// y con todo lo que hace el motor (conexión, QR, envíos y errores).
func diagFilePath() string {
	diagPathOnce.Do(func() {
		dir := desktopDir()
		if dir == "" {
			dir = cfg.DataDir
		}
		diagPath = filepath.Join(dir, cfg.AppName+"-WhatsApp.log")
	})
	return diagPath
}

func rotateLog(path string) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < logMaxBytes {
		return
	}
	_ = os.Remove(fmt.Sprintf("%s.%d", path, logMaxFiles))
	for i := logMaxFiles - 1; i >= 1; i-- {
		_ = os.Rename(fmt.Sprintf("%s.%d", path, i), fmt.Sprintf("%s.%d", path, i+1))
	}
	_ = os.Rename(path, path+".1")
}

func rotateDiag(path string) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < diagMaxBytes {
		return
	}
	_ = os.Remove(path + ".1")
	_ = os.Rename(path, path+".1")
}

// writeDiag escribe en el log del Escritorio. Las líneas idénticas consecutivas
// se agrupan para que un fallo en bucle (p. ej. "Bad MAC" cada 5 s) no llene el
// archivo y siga siendo legible.
func writeDiag(line string) {
	diagMu.Lock()
	defer diagMu.Unlock()

	if !diagStarted {
		diagStarted = true
		diagLastLine = ""
		diagRepeat = 0
	}

	if line == diagLastLine {
		diagRepeat++
		if diagRepeat != 10 && diagRepeat != 100 && diagRepeat != 1000 && diagRepeat%10000 != 0 {
			return
		}
		line = fmt.Sprintf("%s  [repetido x%d]", line, diagRepeat)
	} else {
		diagLastLine = line
		diagRepeat = 1
	}

	path := diagFilePath()
	rotateDiag(path)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "[%s] %s\n", time.Now().Format("2006-01-02 15:04:05"), line)
}

// waDiag registra una línea sólo en el log de diagnóstico del Escritorio.
func waDiag(tag, msg string) {
	if tag != "" {
		writeDiag("[" + tag + "] " + msg)
		return
	}
	writeDiag(msg)
}

// DiagFromFrontend vuelca al log de diagnóstico un mensaje generado en la UI
// (por ejemplo, el fallo al generar o enviar el recibo). Así el archivo del
// Escritorio reúne el lado Go y el lado interfaz en una sola traza.
func DiagFromFrontend(msg string) {
	if strings.TrimSpace(msg) == "" {
		return
	}
	waDiag("UI", msg)
}

// InitDiagnostics crea el log del Escritorio al abrir la aplicación, sin
// esperar a que arranque el motor de WhatsApp. Si el Escritorio no se puede
// escribir (antivirus, acceso a carpetas controlado, Escritorio redirigido a
// una ruta inexistente), cae en AppData y deja constancia en app.log.
func InitDiagnostics() {
	path := diagFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fallbackDiag(fmt.Sprintf("no se pudo preparar %s: %v", path, err))
	} else if f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err != nil {
		fallbackDiag(fmt.Sprintf("no se pudo escribir %s: %v", path, err))
	} else {
		f.Close()
	}
	log.Printf("log de diagnóstico de WhatsApp: %s", diagFilePath())
	writeDiag("========== INICIO DE APLICACIÓN ==========")
	writeDiag(fmt.Sprintf("%s %s | %s | perfil=%s", cfg.AppName, cfg.AppVersion, runtime.GOOS, profileName()))
	writeDiag("usuario: " + os.Getenv("USERNAME"))
	writeDiag("archivo: " + diagFilePath())
}

// fallbackDiag mueve el log a AppData cuando el Escritorio no es escribible.
func fallbackDiag(motivo string) {
	diagPath = filepath.Join(cfg.DataDir, cfg.AppName+"-WhatsApp.log")
	log.Printf("⚠️ log de diagnóstico movido a %s (%s)", diagPath, motivo)
}

// DiagHeader deja una cabecera con el entorno del motor al abrir una sesión.
// Se llama una vez por arranque del sidecar.
func DiagHeader(lines ...string) {
	writeDiag("========== NUEVA SESIÓN ==========")
	writeDiag(fmt.Sprintf("%s %s | %s | perfil=%s", cfg.AppName, cfg.AppVersion, runtime.GOOS, profileName()))
	writeDiag("log de diagnóstico: " + diagFilePath())
	for _, l := range lines {
		writeDiag(l)
	}
	writeDiag("==================================")
}

func profileName() string {
	if p := strings.TrimSpace(cfg.Profile); p != "" {
		return p
	}
	return "default"
}

func waLog(msg string) {
	path := logFilePath()
	rotateLog(path)
	if f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		fmt.Fprintf(f, "[%s] %s\n", time.Now().Format("2006-01-02 15:04:05"), msg)
		f.Close()
	}
	// El log del Escritorio recibe todo lo mismo: así un reporte de error trae
	// también lo que escribió el motor.
	writeDiag(msg)
}

func waTrace(format string, a ...any) {
	line := "WA_TRACE: " + fmt.Sprintf(format, a...)
	fmt.Println(line)
	waLog(line)
}

func waCritical(format string, a ...any) {
	line := "WA_CRITICAL: " + fmt.Sprintf(format, a...)
	fmt.Fprintln(os.Stderr, line)
	waLog(line)
}

// engineFile localiza recursos del motor incluyendo el engine canónico. En
// producción viven junto al exe y en desarrollo en el árbol del proyecto
// (cwd = raíz del repo al correr `wails3 dev`).
//
// Si COMPASS_WHATSAPP_ENGINE apunta al engine canónico (la misma variable que usa
// Compass Go en internal/tasks/whatsapp_engine.go), se reutiliza directamente sin
// copiar nada a GymPro: el bundle y, opcionalmente, node.exe viven ahí. Esto
// cubre el modo debug (`wails3 dev`), que no pasa por el staging de Compass.
func engineFile(rel string) string {
	return findEngineFile(rel, true)
}

// projectFile localiza recursos solo en el proyecto/ejecutable, sin mirar el
// engine canónico. Lo usa resolveNode: el runtime debe preferir el Node del
// PATH y no el sidecar que pueda quedar en el corekit de Compass.
func projectFile(rel string) string {
	return findEngineFile(rel, false)
}

func findEngineFile(rel string, includeEngine bool) string {
	exe := exeDir()
	bases := make([]string, 0, 11)
	var engineDir string
	if includeEngine {
		if eng := strings.TrimSpace(os.Getenv("COMPASS_WHATSAPP_ENGINE")); eng != "" {
			engineDir = eng
			bases = append(bases, eng)
		}
	}
	bases = append(bases,
		exe,
		filepath.Join(exe, "bin"),
		filepath.Join(exe, "resources"),
		filepath.Join(exe, ".."),
		filepath.Join(exe, "..", "resources"),
		filepath.Join(exe, "..", ".."),
		filepath.Join(exe, "..", "..", "resources"),
		".",
		"resources",
	)

	// En debug el bundle puede no estar generado todavía; lo construimos bajo
	// demanda con el node.exe del propio engine canónico para no depender de un
	// node del sistema. Solo aplica al bundle, no a node.exe.
	if engineDir != "" && rel == "whatsapp-engine-bundle/index.js" {
		ensureCanonicalBundle(engineDir)
	}
	for _, b := range bases {
		p := filepath.Join(b, rel)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			// Debe ser absoluta: el motor se lanza con cwd = AppData, así que una
			// ruta relativa dejaría de resolverse al cambiar de directorio.
			if abs, err := filepath.Abs(p); err == nil {
				return abs
			}
			return p
		}
	}
	return ""
}

// bundleBuildMu serializa la generación del bundle para no disparar dos builds en
// paralelo si varias rutinas lo pidieran a la vez.
var bundleBuildMu sync.Mutex

// ensureCanonicalBundle genera whatsapp-engine-bundle con `node bundle.mjs` dentro
// del engine canónico si todavía no existe. Usa el node.exe que vive en el propio
// engine (no requiere node en el PATH). Es un no-op si el bundle ya está presente
// o si no hay node.exe disponible para generarlo.
func ensureCanonicalBundle(engineDir string) {
	bundle := filepath.Join(engineDir, "whatsapp-engine-bundle", "index.js")
	if _, err := os.Stat(bundle); err == nil {
		return
	}
	bundleBuildMu.Lock()
	defer bundleBuildMu.Unlock()
	// Re-chequeo tras tomar el lock: otro llamante pudo generarlo mientras tanto.
	if _, err := os.Stat(bundle); err == nil {
		return
	}
	node := filepath.Join(engineDir, "node.exe")
	if _, err := os.Stat(node); err != nil {
		waLog(fmt.Sprintf("auto-build del bundle omitido: no hay node.exe en el engine canónico %s", engineDir))
		return
	}
	waLog(fmt.Sprintf("bundle del motor ausente en %s; generándolo con `node bundle.mjs`", engineDir))
	cmd := exec.Command(node, "bundle.mjs")
	cmd.Dir = engineDir
	hideConsoleWindow(cmd)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		waCritical("falló el auto-build del bundle del motor: %v", err)
	}
}
func findFirst(candidates ...string) string {
	for _, rel := range candidates {
		if p := engineFile(rel); p != "" {
			return p
		}
	}
	return ""
}

// findFirstProject es como findFirst pero ignora el engine canónico: solo mira
// el proyecto/ejecutable. Se usa para resolver Node en runtime.
func findFirstProject(candidates ...string) string {
	for _, rel := range candidates {
		if p := projectFile(rel); p != "" {
			return p
		}
	}
	return ""
}

func cleanNumber(number string) string {
	return strings.NewReplacer("+", "", "-", "", " ", "").Replace(number)
}

// describeFile indica si el archivo que se va a enviar existe y su tamaño: un
// recibo sin imagen en disco es la causa más común de "no se envió".
func describeFile(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return path + " (NO EXISTE)"
	}
	return fmt.Sprintf("%s (%d bytes)", path, info.Size())
}

// describeAuthDir resume el estado de la sesión guardada: si existe, cuántos
// archivos tiene y cuándo se escribió creds.json. Sirve para distinguir "no hay
// sesión" de "la sesión se guardó pero el motor no la lee".
func describeAuthDir(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "no existe"
	}
	if len(entries) == 0 {
		return "vacía"
	}
	out := fmt.Sprintf("%d archivos", len(entries))
	if info, err := os.Stat(filepath.Join(dir, "creds.json")); err == nil {
		out += fmt.Sprintf(", creds.json %d bytes (%s)", info.Size(), info.ModTime().Format("2006-01-02 15:04:05"))
	} else {
		out += ", SIN creds.json"
	}
	return out
}
