// Package shell agrupa todo lo relativo a la ejecucion de comandos en Windows
// mediante PowerShell, la gestion de la consola (titulo, color de pestana) y la
// busqueda/revelado de artefactos. Es el equivalente de src/utils/cmd.rs.
package shell

import (
	"compass/internal/stdin"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"compass/internal/settings"
	"compass/internal/sound"
)

// ExitCloseTab es el codigo de salida con el que una tarea pide que su pestana
// del contenedor se cierre. Alto a proposito para no confundirse con 0/1/2.
const ExitCloseTab = 95

var (
	shellOnce sync.Once
	shellName string

	shellPathOnce sync.Once
	shellPath     string
)

// Shell devuelve el shell activo ("pwsh" si esta disponible, si no
// "powershell"). Se prefiere pwsh porque aporta el icono nuevo.
func Shell() string {
	shellOnce.Do(func() {
		if available("pwsh") {
			shellName = "pwsh"
		} else {
			shellName = "powershell"
		}
	})
	return shellName
}

// ShellPath devuelve la ruta completa del shell activo (para entradas del
// registro, donde Explorer no hereda el PATH de la terminal).
func ShellPath() string {
	shellPathOnce.Do(func() {
		if Shell() == "pwsh" {
			if pf := os.Getenv("ProgramFiles"); pf != "" {
				cand := filepath.Join(pf, "PowerShell", "7", "pwsh.exe")
				if fileExists(cand) {
					shellPath = cand
					return
				}
			}
			if out, err := hideConsole(exec.Command("where.exe", "pwsh")).Output(); err == nil {
				if line := firstNonEmptyLine(string(out)); line != "" {
					shellPath = line
					return
				}
			}
			shellPath = "pwsh.exe"
			return
		}
		if root := os.Getenv("SystemRoot"); root != "" {
			shellPath = filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
		} else {
			shellPath = "powershell.exe"
		}
	})
	return shellPath
}

// Embedded indica si este proceso corre dentro de una pestana del contenedor
// (la marca la pone el contenedor al abrir la sesion ConPTY).
func Embedded() bool {
	_, ok := os.LookupEnv("COMPASS_EMBEDDED")
	return ok
}

// PsQuote escapa una cadena para PowerShell duplicando las comillas simples.
func PsQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// hideConsole aplica los atributos que impiden que un proceso hijo abra una
// consola visible (parpadeo) cuando Compass corre como app GUI. Los handles de
// stdio se heredan igual, asi que la salida sigue llegando a la ConPTY.
func hideConsole(c *exec.Cmd) *exec.Cmd {
	c.SysProcAttr = noWindowAttrs()
	return c
}

// HideConsole expone hideConsole para que otros paquetes (git, adb, keytool)
// ejecuten procesos externos sin que aparezca una consola visible.
func HideConsole(c *exec.Cmd) *exec.Cmd { return hideConsole(c) }

// RunPS ejecuta un script de PowerShell y captura su salida.
func RunPS(script string) ([]byte, error) {
	cmd := hideConsole(exec.Command(Shell(), "-NoLogo", "-NoProfile", "-Command", script))
	out, err := cmd.CombinedOutput()
	return out, err
}

// RunPSInDirStatus ejecuta un script en dir; devuelve error si falla.
func RunPSInDirStatus(script string, dir string) error {
	if !dirExists(dir) {
		return fmt.Errorf("el directorio no existe: %s", dir)
	}
	// Sin CREATE_NO_WINDOW: el hijo hereda la consola (la ConPTY) y su salida
	// llega al panel. CREATE_NO_WINDOW le daria una consola nueva y la perderia.
	cmd := exec.Command(Shell(), "-NoLogo", "-NoProfile", "-Command", script)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("no se pudo ejecutar PowerShell: %w", err)
	}
	return nil
}

// mergeEnv devuelve el entorno del proceso con las variables de envs
// aplicadas. Hay que partir de os.Environ(): un Cmd con Env no nulo deja de
// heredar PATH, SystemRoot, TEMP y todo lo demas.
func mergeEnv(envs map[string]string) []string {
	env := os.Environ()
	for k, v := range envs {
		prefix := k + "="
		filtered := env[:0]
		for _, kv := range env {
			if len(kv) >= len(prefix) && strings.EqualFold(kv[:len(prefix)], prefix) {
				continue
			}
			filtered = append(filtered, kv)
		}
		env = append(filtered, prefix+v)
	}
	return env
}

// RunPSStatus ejecuta un script en dir con entorno extra; devuelve si tuvo exito.
func RunPSStatus(script string, dir string, envs map[string]string) (bool, error) {
	if !dirExists(dir) {
		return false, fmt.Errorf("el directorio no existe: %s", dir)
	}
	// Hereda la consola: la salida debe verse en la ConPTY.
	cmd := exec.Command(Shell(), "-NoLogo", "-NoProfile", "-Command", script)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = mergeEnv(envs)
	err := cmd.Run()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// RunPSFile ejecuta un script de PowerShell (archivo .ps1) con argumentos y
// entorno extra; devuelve si tuvo exito. Equivale a cmd::run_ps_file_status.
func RunPSFile(path string, args []string, dir string, envs map[string]string) (bool, error) {
	if !fileExists(path) {
		return false, fmt.Errorf("el script no existe: %s", path)
	}
	if !dirExists(dir) {
		return false, fmt.Errorf("el directorio no existe: %s", dir)
	}
	full := []string{Shell(), "-NoLogo", "-NoProfile", "-File", path}
	full = append(full, args...)
	// Hereda la consola: la salida debe verse en la ConPTY.
	c := exec.Command(full[0], full[1:]...)
	c.Dir = dir
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Env = mergeEnv(envs)
	err := c.Run()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Run ejecuta un comando y captura su salida.
func Run(cmd string, args ...string) ([]byte, error) {
	c := hideConsole(exec.Command(cmd, args...))
	return c.CombinedOutput()
}

// RunInDir ejecuta un comando en dir y captura su salida.
func RunInDir(cmd string, dir string, args ...string) ([]byte, error) {
	if !dirExists(dir) {
		return nil, fmt.Errorf("el directorio no existe: %s", dir)
	}
	c := hideConsole(exec.Command(cmd, args...))
	c.Dir = dir
	return c.CombinedOutput()
}

// RunStatus ejecuta un comando; error si falla.
func RunStatus(cmd string, args ...string) error {
	// Hereda la consola para que la salida se vea en la ConPTY.
	c := exec.Command(cmd, args...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("comando falló: %w", err)
	}
	return nil
}

// RunInDirStatus ejecuta un comando en dir; error si falla.
func RunInDirStatus(cmd string, dir string, args ...string) error {
	if !dirExists(dir) {
		return fmt.Errorf("el directorio no existe: %s", dir)
	}
	// Hereda la consola para que la salida se vea en la ConPTY.
	c := exec.Command(cmd, args...)
	c.Dir = dir
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("falló al ejecutar '%s': %w", cmd, err)
	}
	return nil
}

// RunTimeout ejecuta cmd esperando hasta timeout secs con stdin cerrado.
// Devuelve (false, nil) si se agoto el tiempo; (_, err) si no arranco.
func RunTimeout(cmd string, args []string, timeoutSecs uint) (bool, error) {
	c := hideConsole(exec.Command(cmd, args...))
	c.Stdin = nil
	if err := c.Start(); err != nil {
		return false, fmt.Errorf("falló al ejecutar '%s': %w", cmd, err)
	}
	limit := time.Now().Add(time.Duration(timeoutSecs) * time.Second)
	for {
		err := c.Wait()
		if err == nil {
			return true, nil
		}
		if time.Now().After(limit) {
			_ = c.Process.Kill()
			_ = c.Wait()
			return false, nil
		}
		time.Sleep(150 * time.Millisecond)
	}
}

// RunProgram ejecuta un programa sin shell, con args, env y dir; devuelve exito.
// Hereda la consola (ConPTY) para que su salida se vea en el panel: asi las
// cards que lanzan un proceso (p. ej. `python app.py`) muestran su progreso.
func RunProgram(program string, args []string, dir string, envs map[string]string) (bool, error) {
	c := exec.Command(program, args...)
	c.Dir = dir
	c.Env = mergeEnv(envs)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	err := c.Run()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// RunProgramCapture ejecuta un programa capturando la salida.
func RunProgramCapture(program string, args []string, dir string, envs map[string]string) ([]byte, error) {
	c := hideConsole(exec.Command(program, args...))
	c.Dir = dir
	c.Env = mergeEnv(envs)
	out, err := c.CombinedOutput()
	return out, err
}

// RunInCurrentWindow ejecuta un comando en el shell activo, heredando stdio.
func RunInCurrentWindow(command string, dir string) error {
	fmt.Println()
	fmt.Println("[INFO] Ejecutando: " + command)
	fmt.Println()
	if !runShellStatus(command, dir, nil) {
		fmt.Println()
		fmt.Println("[ERROR] El comando fallo.")
	}
	return nil
}

// RunShellStatus ejecuta command en el shell activo y devuelve si tuvo exito.
func RunShellStatus(command string, dir string, envs map[string]string) (bool, error) {
	return runShellStatus(command, dir, envs), nil
}

func runShellStatus(command string, dir string, envs map[string]string) bool {
	script := "& " + command
	c := exec.Command(Shell(), "-NoLogo", "-NoProfile", "-Command", script)
	c.Dir = dir
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	// Sin CREATE_NO_WINDOW: hereda la consola (ConPTY) para que la salida se
	// vea en el panel. Con la bandera, el hijo recibiria una consola nueva y su
	// salida se perderia.
	c.Env = mergeEnv(envs)
	err := c.Run()
	return err == nil
}

// BuildOutcome es el resultado de un build: exito y ruta del artefacto.
type BuildOutcome struct {
	Success  bool
	Artifact string
}

// RunBuildAndReveal ejecuta command en dir, revela el artefacto mas reciente en
// folder con extensiones exts y (opcionalmente) lo renombra a renameTo.
func RunBuildAndReveal(command, dir, folder string, exts []string, renameTo string) (BuildOutcome, error) {
	return RunBuildAndRevealWithEnv(command, dir, folder, exts, renameTo, nil)
}

// RunBuildAndRevealWithEnv es como RunBuildAndReveal pero con variables extra.
func RunBuildAndRevealWithEnv(command, dir, folder string, exts []string, renameTo string, envs map[string]string) (BuildOutcome, error) {
	fmt.Println()
	fmt.Println("[INFO] Ejecutando: " + command)
	fmt.Println()
	success := runShellStatus(command, dir, envs)
	var artifact string
	if success {
		artifact = revealArtifact(folder, exts, renameTo)
	} else {
		sound.Failure()
		fmt.Println()
		fmt.Println("[ERROR] El build fallo.")
	}
	fmt.Println()
	if success {
		// El usuario ve la cuenta atras; dentro del contenedor ademas cierra
		// la pestana.
		PauseFor(10)
	} else {
		// Con error se espera Enter para poder leer los mensajes antes de que
		// se cierre la ventana.
		Pause()
	}
	return BuildOutcome{Success: success, Artifact: artifact}, nil
}

// NewestArtifact busca el archivo mas reciente (recursivo) en folder cuya
// extension este en exts (vacio = cualquiera).
func NewestArtifact(folder string, exts []string) string {
	bestTime := time.Time{}
	bestPath := ""
	walk(folder, exts, &bestTime, &bestPath)
	return bestPath
}

// sidecarDirs son carpetas que Compass etaja junto al binario (motor de
// WhatsApp, recursos). Su contenido (node.exe) puede ser mas reciente que el
// ejecutable de la app y no es nunca el artefacto del build.
var sidecarDirs = map[string]bool{
	"whatsapp_engine": true,
	"resources":       true,
}

func walk(dir string, exts []string, bestTime *time.Time, bestPath *string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		if e.IsDir() {
			if sidecarDirs[e.Name()] {
				continue
			}
			walk(p, exts, bestTime, bestPath)
			continue
		}
		if !matchExt(p, exts) {
			continue
		}
		// node.exe es el sidecar de Node del motor de WhatsApp: nunca es el
		// artefacto del proyecto, aunque su fecha sea la mas reciente.
		if strings.EqualFold(e.Name(), "node.exe") {
			continue
		}
		if info, err := e.Info(); err == nil {
			mt := info.ModTime()
			if mt.After(*bestTime) {
				*bestTime = mt
				*bestPath = p
			}
		}
	}
}

// NewestFileIn busca el archivo mas reciente directamente en folder (sin
// recursion) cuya extension este en exts.
func NewestFileIn(folder string, exts []string) string {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return ""
	}
	bestTime := time.Time{}
	bestPath := ""
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		p := filepath.Join(folder, e.Name())
		if !matchExt(p, exts) {
			continue
		}
		if info, err := e.Info(); err == nil {
			mt := info.ModTime()
			if mt.After(bestTime) {
				bestTime = mt
				bestPath = p
			}
		}
	}
	return bestPath
}

func matchExt(p string, exts []string) bool {
	if len(exts) == 0 {
		return true
	}
	ext := strings.ToLower(filepath.Ext(p))
	for _, e := range exts {
		if ext == "."+strings.ToLower(e) {
			return true
		}
	}
	return false
}

// RenameArtifact renombra path a <base>.<ext> conservando la extension.
func RenameArtifact(path, base string) string {
	ext := filepath.Ext(path)
	if ext == "" {
		return path
	}
	target := filepath.Join(filepath.Dir(path), base+ext)
	if target == path {
		return path
	}
	_ = os.Remove(target)
	if err := os.Rename(path, target); err == nil {
		return target
	}
	if err := copyFile(path, target); err == nil {
		return target
	}
	return path
}

func revealArtifact(folder string, exts []string, renameTo string) string {
	artifact := NewestArtifact(folder, exts)
	if artifact == "" {
		fmt.Println()
		fmt.Println("[OK] Build exitoso (artefacto no encontrado en la carpeta)")
		if settings.Load().OpenOutputDir {
			OpenFolder(folder)
		}
		return ""
	}
	if renameTo != "" {
		artifact = RenameArtifact(artifact, renameTo)
	}
	ShowArtifact(artifact)
	return artifact
}

// ShowArtifact muestra el artefacto construido y, si el usuario lo tiene
// activado en las opciones globales, lo resalta en el Explorer.
func ShowArtifact(path string) {
	fmt.Println()
	fmt.Printf("[OK] Build exitoso: %s\n", filepath.Base(path))
	fmt.Printf("[INFO] Artefacto: %s\n", path)
	if settings.Load().OpenOutputDir {
		OpenInExplorer(path)
	}
}

func windowsPathForExplorer(path string) string {
	return strings.ReplaceAll(path, "/", "\\")
}

// OpenInExplorer abre el Explorer resaltando path.
//
// Ojo: explorer.exe exige el modificador "/select," y la ruta como DOS
// argumentos separados. Si se concatenan en uno ("/select,\"C:\\...\\x.exe\""),
// el runtime de Go re-escapa las comillas y Explorer no reconoce la ruta: cae
// en la carpeta Documents en vez de abrir la carpeta del archivo.
func OpenInExplorer(path string) {
	p := windowsPathForExplorer(path)
	_ = exec.Command("explorer", "/select,", p).Start()
}

// OpenFolder abre una carpeta en el Explorer.
func OpenFolder(folder string) {
	p := windowsPathForExplorer(folder)
	_ = exec.Command("explorer", p).Start()
}

// SetConsoleTitle fija el titulo de la consola (OSC 0). Dentro de una pestana
// incrustada no se escribe (lo gestiona el host).
func SetConsoleTitle(title string) {
	if Embedded() {
		return
	}
	fmt.Printf("\x1b]0;%s\x07", title)
}

// SetTabColor tiñe la pestana de Windows Terminal con rgb (DECAC + OSC 4).
func SetTabColor(rgb [3]byte) {
	if Embedded() {
		return
	}
	fmt.Print(tabColorSequence(rgb))
}

func tabColorSequence(rgb [3]byte) string {
	return fmt.Sprintf("\x1b[2;263;264,|\x1b]4;264;rgb:%02x/%02x/%02x\x1b\\",
		rgb[0], rgb[1], rgb[2])
}

// ReclaimConsoleTitle reescribe el titulo solo si otro proceso lo cambio.
//
// El titulo viaja desde una pestana ConPTY a la consola real, asi que el pwsh
// del contenedor lo pisa al abrir una tarea y conhost le antepone su prefijo de
// elevacion. Se compara con el titulo real en vez de reescribir a ciegas.
func ReclaimConsoleTitle(wanted string) {
	if Embedded() {
		return
	}
	if actual, ok := ConsoleTitle(); ok && actual != wanted {
		SetConsoleTitle(wanted)
	}
}

// ClearScreen limpia la pantalla de la consola.
func ClearScreen() {
	fmt.Print("\x1b[2J\x1b[H")
}

// Pause espera a que el usuario pulse Enter.
//
// Vive en shell (y no en ui) para que RunBuildAndReveal pueda pausar sin crear
// un ciclo de imports: ui ya depende de shell.
func Pause() {
	fmt.Print("  Presiona Enter para continuar...")
	_ = stdin.ReadLine()
}

// PauseFor hace una cuenta regresiva y, dentro del contenedor, pide cerrar la
// pestana (el proceso sale con ExitCloseTab).
//
// La cuenta atras y el cierre solo ocurren si el usuario activo
// "cerrar la pestana al terminar un build" en Opciones (por defecto esta
// desactivado y la pestana se queda abierta).
func PauseFor(seconds uint) {
	// Todas las tareas de build terminan aqui: el aviso sonoro suena al
	// instante, antes de la cuenta atras y del cierre de la pestana.
	sound.Success()
	if !settings.Load().CloseTabOnBuildEnd {
		fmt.Println()
		fmt.Println("  Build completado. La pestana permanece abierta.")
		return
	}
	for remaining := int(seconds); remaining >= 1; remaining-- {
		fmt.Printf("\r  Cerrando en %d s...  ", remaining)
		time.Sleep(time.Second)
	}
	fmt.Println()
	if Embedded() {
		RequestCloseTab()
	}
}

// RunNpmInstall ejecuta `npm.cmd install` en dir (npm.cmd evita el shim .ps1
// bloqueado por ExecutionPolicy).
func RunNpmInstall(dir string) error {
	if !dirExists(dir) {
		return fmt.Errorf("el directorio no existe: %s", dir)
	}
	if !fileExists(filepath.Join(dir, "package.json")) {
		return fmt.Errorf("no se encontro package.json en: %s", dir)
	}
	ok, err := RunPSStatus("npm.cmd install", dir, nil)
	if err != nil {
		return fmt.Errorf("no se pudo ejecutar npm. Verifica que Node.js este instalado: %w", err)
	}
	if !ok {
		return fmt.Errorf("npm install fallo en: %s", dir)
	}
	return nil
}

// RemoveDir elimina un directorio recursivamente (fallback a PowerShell si
// falla, p.ej. rutas largas en Windows).
func RemoveDir(path string) error {
	if !dirExists(path) {
		return nil
	}
	if err := os.RemoveAll(path); err == nil {
		return nil
	}
	ps := fmt.Sprintf("Remove-Item -LiteralPath %s -Recurse -Force", PsQuote(path))
	out, err := RunPS(ps)
	if err != nil {
		return fmt.Errorf("error al eliminar %s: %s", path, string(out))
	}
	return nil
}

// AddUserPathDir anade dir al PATH persistente del usuario (HKCU\Environment).
// Devuelve true si lo escribio; false si ya estaba.
func AddUserPathDir(dir string) (bool, error) {
	script := userPathAppendScript(dir)
	out, err := RunPS(script)
	if err != nil {
		return false, fmt.Errorf("no se pudo escribir el PATH del usuario: %s", string(out))
	}
	return strings.Contains(string(out), "COMPASS_PATH_ADDED"), nil
}

// RefreshEnvironment notifica a Explorer el cambio de entorno.
func RefreshEnvironment() {
	_ = hideConsole(exec.Command("ie4uinit.exe", "-show")).Start()
}

func userPathAppendScript(dir string) string {
	q := PsQuote(dir)
	return fmt.Sprintf(`$ErrorActionPreference = 'Stop'
$dir = %s
$key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $true)
$raw = $key.GetValue('Path', '', 'DoNotExpandEnvironmentNames')
$lista = @($raw -split ';')
if ($lista -notcontains $dir) {
  $nuevo = if ($raw) { "$raw;$dir" } else { $dir }
  $key.SetValue('Path', $nuevo, [Microsoft.Win32.RegistryValueKind]::ExpandString)
  'COMPASS_PATH_ADDED'
} else { 'COMPASS_PATH_PRESENT' }
$key.Close()`, q)
}

// ---- cierre de pestana solicitado ----

var closeTab atomic.Bool

// RequestCloseTab marca que este proceso (tarea) pide cerrar su pestana.
func RequestCloseTab() {
	closeTab.Store(true)
}

// CloseTabRequested indica si se pidio cerrar la pestana.
func CloseTabRequested() bool {
	return closeTab.Load()
}

// ---- helpers ----

func available(name string) bool {
	// Ocultar la consola: esta deteccion corre la primera vez que se usa el
	// shell y, sin la bandera, abriria una terminal que aparece y desaparece.
	c := hideConsole(exec.Command(name, "-NoLogo", "-NoProfile", "-Command", "exit 0"))
	c.Stdin = nil
	c.Stdout = nil
	c.Stderr = nil
	return c.Run() == nil
}

func firstNonEmptyLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		t := strings.TrimSpace(l)
		if t != "" {
			return t
		}
	}
	return ""
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
