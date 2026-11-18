package tasks

// GoWails port de src/tasks/gowails.rs: tareas de Go + Wails v3 (escritorio y
// Android). El escritorio delega en el Taskfile (`wails3 task build/package`);
// el Android lo construye Compass a mano porque el Taskfile de la plantilla usa
// scripts POSIX que no corren en Windows (ver doc del módulo Rust).

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"compass/internal/project"
	"compass/internal/prompts"
	"compass/internal/settings"
	"compass/internal/shell"
	"compass/internal/ui"
)

// minSdk de la plantilla de Wails v3 (build/android/app/build.gradle).
const wailsMinSdk = 21

// abi describe una ABI de Android que Compass compila con la NDK.
type abi struct {
	jniDir      string // carpeta jniLibs que recibe libwails.so
	goarch      string // GOARCH de Go
	goarm       string // GOARM (sólo ARM de 32 bits); "" si no aplica
	clangTriple string // prefijo del target de la NDK (<triple><minSdk>)
	artifact    string // sufijo del APK generado (arm64, armv7)
}

var (
	abiArm64 = abi{
		jniDir:      "arm64-v8a",
		goarch:      "arm64",
		goarm:       "",
		clangTriple: "aarch64-linux-android",
		artifact:    "arm64",
	}
	abiArmv7 = abi{
		jniDir:      "armeabi-v7a",
		goarch:      "arm",
		goarm:       "7",
		clangTriple: "armv7a-linux-androideabi",
		artifact:    "armv7",
	}
	releaseAbis = []abi{abiArm64, abiArmv7}
)

func init() {
	Register("windows-dev", project.TypeGoWails, WailsDev)
	Register("windows-build", project.TypeGoWails, WailsBuildDesktop)
	Register("android-dev", project.TypeGoWails, WailsInstallDebug)
	Register("android-install-debug", project.TypeGoWails, WailsInstallDebug)
	Register("android-hmr", project.TypeGoWails, WailsDevAndroid)
	Register("android-arm64", project.TypeGoWails, WailsBuildApk)
	Register("android-armv7", project.TypeGoWails, WailsBuildApkArmv7)
	Register("android-aab", project.TypeGoWails, WailsBuildAab)
	Register("android-all", project.TypeGoWails, WailsBuildAab)
	Register("debug-android", project.TypeGoWails, WailsDebugAndroid)
	Register("package", project.TypeGoWails, WailsPackageDesktop)
	Register("windows-package", project.TypeGoWails, WailsPackageDesktop)
}

// ───────────────────────────────────────────────────────────────────────────
// Configuración del proyecto
// ───────────────────────────────────────────────────────────────────────────

// taskfileVar lee una variable del bloque vars: del Taskfile.yml (sólo literales).
func wailsTaskfileVar(ctx *project.ProjectContext, name string) string {
	data, err := os.ReadFile(filepath.Join(ctx.ScriptDir, "Taskfile.yml"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		// Las vars de la raíz van indentadas con 2 espacios.
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent != 2 {
			continue
		}
		trimmed := strings.TrimSpace(line)
		prefix := name + ":"
		if !strings.HasPrefix(trimmed, prefix) {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
		if strings.HasPrefix(value, "\"") {
			if end := strings.Index(value[1:], "\""); end >= 0 {
				return value[1 : end+1]
			}
		}
		if strings.HasPrefix(value, "'") {
			if end := strings.Index(value[1:], "'"); end >= 0 {
				return value[1 : end+1]
			}
		}
		if comment := strings.Index(value, " #"); comment >= 0 {
			value = strings.TrimSpace(value[:comment])
		}
		return value
	}
	return ""
}

// appName del proyecto (APP_NAME del Taskfile; si no, la carpeta).
func wailsAppName(ctx *project.ProjectContext) string {
	if v := wailsTaskfileVar(ctx, "APP_NAME"); v != "" {
		return v
	}
	return filepath.Base(ctx.ScriptDir)
}

// binDir donde el Taskfile deja los binarios (BIN_DIR; por defecto bin/).
func wailsBinDir(ctx *project.ProjectContext) string {
	dir := wailsTaskfileVar(ctx, "BIN_DIR")
	if dir == "" {
		dir = "bin"
	}
	if filepath.IsAbs(dir) {
		return filepath.Clean(dir)
	}
	return filepath.Join(ctx.ScriptDir, dir)
}

// GoWailsClean corre 'go clean' y elimina los directorios generados por Wails
// (bin/, build/bin, build/android).
func GoWailsClean(ctx *project.ProjectContext) error {
	fmt.Println("[INFO] Ejecutando go clean (GoWails)...")
	if err := shell.RunInDirStatus("go", ctx.ScriptDir, "clean"); err != nil {
		return err
	}
	dirs := []string{
		wailsBinDir(ctx),
		filepath.Join(ctx.ScriptDir, "build", "bin"),
		wailsAndroidDir(ctx),
	}
	for _, d := range dirs {
		if shell.RemoveDir(d) != nil {
			fmt.Printf("[WARN] No se pudo eliminar %s\n", d)
		}
	}
	fmt.Println("[OK] Limpieza GoWails completada.")
	ui.Pause()
	return nil
}

// readWailsAppMeta lee productName y version de build/config.yml (Wails v3).
func readWailsAppMeta(ctx *project.ProjectContext) (string, string) {
	confPath := filepath.Join(ctx.ScriptDir, "build", "config.yml")
	data, err := os.ReadFile(confPath)
	if err != nil {
		return "", ""
	}
	var name, version string
	inInfo := false
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "info:" {
			inInfo = true
			continue
		}
		if inInfo && trimmed != "" && !strings.HasPrefix(trimmed, " ") && !strings.HasPrefix(trimmed, "\t") {
			inInfo = false
		}
		if !inInfo {
			continue
		}
		k, v, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		key := strings.TrimSpace(k)
		val := strings.TrimSpace(v)
		// quita comentarios y comillas
		if i := strings.Index(val, "#"); i >= 0 {
			val = val[:i]
		}
		val = strings.Trim(val, "\"' ")
		switch key {
		case "productName":
			name = val
		case "version":
			version = val
		}
	}
	return name, version
}

// releaseMeta: nombre y versión de los artefactos (build/config.yml).
func wailsReleaseMeta(ctx *project.ProjectContext) (string, string) {
	if name, version := readWailsAppMeta(ctx); name != "" && version != "" {
		return name, version
	}
	if name, version := readWailsAppMeta(ctx); name != "" {
		return name, version
	}
	return wailsAppName(ctx), ""
}

// versionCode derivado de X.Y.Z (Play Store exige entero monótono).
func wailsVersionCode(version string) (int64, bool) {
	core := version
	if i := strings.IndexAny(core, "-+"); i >= 0 {
		core = core[:i]
	}
	core = strings.TrimSpace(core)
	parts := strings.Split(core, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return 0, false
	}
	var major, minor, patch int64
	major, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, false
	}
	if len(parts) > 1 {
		minor, _ = strconv.ParseInt(parts[1], 10, 64)
	}
	if len(parts) > 2 {
		patch, _ = strconv.ParseInt(parts[2], 10, 64)
	}
	if len(parts) > 3 || minor > 999 || patch > 999 {
		return 0, false
	}
	code := major*1_000_000 + minor*1_000 + patch
	if code < 0 || code > 2_100_000_000 {
		return 0, false
	}
	return code, true
}

// versionedArtifact: <Nombre>-v<ver>-<sufijo>.<ext> (sin versión: <Nombre>-<sufijo>.<ext>).
func wailsVersionedArtifact(name, version, suffix, ext string) string {
	name = sanitizeAppName(name)
	if suffix != "" {
		suffix = "-" + suffix
	}
	if version != "" {
		return fmt.Sprintf("%s-v%s%s.%s", name, version, suffix, ext)
	}
	return fmt.Sprintf("%s%s.%s", name, suffix, ext)
}

// desktopArtifact: <Nombre> v<ver> <sufijo>.<ext> (con espacios).
func wailsDesktopArtifact(name, version, suffix, ext string) string {
	base := name
	if version != "" {
		base = fmt.Sprintf("%s v%s", name, version)
	}
	if suffix != "" {
		return fmt.Sprintf("%s %s.%s", base, suffix, ext)
	}
	return fmt.Sprintf("%s.%s", base, ext)
}

// androidDir del proyecto Wails (build/android).
func wailsAndroidDir(ctx *project.ProjectContext) string {
	return filepath.Join(ctx.ScriptDir, "build", "android")
}

// applicationId del módulo Android (build/android/app/build.gradle).
func wailsApplicationID(ctx *project.ProjectContext) string {
	gradle := filepath.Join(wailsAndroidDir(ctx), "app", "build.gradle")
	candidates := []string{gradle, filepath.Join(wailsAndroidDir(ctx), "app", "build.gradle.kts")}
	for _, p := range candidates {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			t := strings.TrimSpace(line)
			if strings.HasPrefix(t, "applicationId") {
				v := strings.TrimSpace(t[len("applicationId"):])
				v = strings.Trim(v, "\"' ")
				if v != "" {
					return v
				}
			}
		}
	}
	return "com.wails.app"
}

// mainActivity: <namespace>.MainActivity de build/android/app/build.gradle.
func wailsMainActivity(ctx *project.ProjectContext) string {
	gradle := filepath.Join(wailsAndroidDir(ctx), "app", "build.gradle")
	candidates := []string{gradle, filepath.Join(wailsAndroidDir(ctx), "app", "build.gradle.kts")}
	for _, p := range candidates {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			t := strings.TrimSpace(line)
			if strings.HasPrefix(t, "namespace") {
				v := strings.TrimSpace(t[len("namespace"):])
				v = strings.Trim(v, "\"' ")
				if v != "" {
					return v + ".MainActivity"
				}
			}
		}
	}
	return "com.wails.app.MainActivity"
}

// ───────────────────────────────────────────────────────────────────────────
// Utilidades
// ───────────────────────────────────────────────────────────────────────────

// ensureWailsCli comprueba wails3 y go.
func ensureWailsCli() error {
	if !commandExists("wails3") {
		return fmt.Errorf("[ERROR] No se encontró el CLI `wails3`.\n" +
			"   Instálalo con:\n  go install github.com/wailsapp/wails/v3/cmd/wails3@latest\n" +
			"   (el binario queda en $(go env GOPATH)/bin, que debe estar en el PATH).")
	}
	if !commandExists("go") {
		return fmt.Errorf("[ERROR] No se encontró Go en el PATH.\n" +
			"   Instálalo desde https://go.dev/dl/ y reinstala el CLI de Wails.")
	}
	return nil
}

// ── NSIS ──────────────────────────────────────────────────────────────────

const nsisInstallHint = "Instálalo con:\n" +
	"  winget install NSIS.NSIS --exact --accept-source-agreements --accept-package-agreements\n" +
	"o compila sólo el portable con `wails3 task build ARCH=amd64`."

// nsisCandidates: carpetas donde NSIS deja makensis.exe por defecto.
func nsisCandidates() []string {
	var dirs []string
	for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles", "ProgramW6432"} {
		root := os.Getenv(env)
		if root == "" {
			continue
		}
		d := filepath.Join(root, "NSIS")
		found := false
		for _, e := range dirs {
			if e == d {
				found = true
				break
			}
		}
		if !found {
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// locateMakensis devuelve la ruta si está en el PATH o en una carpeta candidata,
// "" si no aparece en ninguna parte.
func locateMakensis() string {
	if commandExists("makensis") {
		return "makensis"
	}
	for _, d := range nsisCandidates() {
		cand := filepath.Join(d, "makensis.exe")
		if fileExists(cand) {
			return cand
		}
	}
	return ""
}

// nsisEnv devuelve el PATH inyectado (carpeta delante) para el hijo.
func nsisEnv(dir string) map[string]string {
	current := os.Getenv("PATH")
	return map[string]string{"PATH": dir + ";" + current}
}

// ensureNsis asegura que el package de Wails encontrará makensis.
func ensureNsis() (map[string]string, error) {
	found := locateMakensis()
	if found == "" {
		return installNsis()
	}
	if found == "makensis" {
		return nil, nil
	}
	// instalado fuera del PATH de este proceso: inyectamos la carpeta.
	fmt.Printf("[INFO] NSIS: %s (fuera del PATH). Se le pasa a esta compilación sin tocar el registro.\n", found)
	dir := filepath.Dir(found)
	return nsisEnv(dir), nil
}

// installNsis: instalación interactiva de NSIS cuando no está en ninguna parte.
func installNsis() (map[string]string, error) {
	fmt.Println()
	fmt.Println("[ERROR] Falta NSIS (`makensis`): es lo que compila el instalador de `wails3 task package`.")
	fmt.Println("        El `.exe` portable se genera igual; lo que falla es el paso `windows:create:nsis:installer`.")
	ok, err := prompts.Confirm("¿Instalar NSIS ahora con winget?", true)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("[ERROR] Sin NSIS no se puede empaquetar. " + nsisInstallHint)
	}
	if err := shell.RunPSInDirStatus(
		"winget install NSIS.NSIS --exact --accept-source-agreements --accept-package-agreements",
		".",
	); err != nil {
		fmt.Println("[AVISO] winget terminó con fallo (o se canceló).")
	}
	found := locateMakensis()
	if found == "" || found == "makensis" {
		if found == "makensis" {
			return nil, nil
		}
		return nil, fmt.Errorf("[ERROR] NSIS sigue sin aparecer tras la intentona de instalación.\n" + nsisInstallHint)
	}
	dir := filepath.Dir(found)
	fmt.Printf("[OK] NSIS instalado en %s\n", dir)
	ok, err = prompts.Confirm("¿Añadir esa carpeta al PATH del usuario para que también funcione desde una consola normal?", true)
	if err != nil {
		return nil, err
	}
	if ok {
		added, err := shell.AddUserPathDir(dir)
		if err != nil {
			fmt.Printf("[WARN] No se pudo añadir al PATH: %v\n", err)
		} else if added {
			shell.RefreshEnvironment()
			fmt.Println("[OK] Añadido al PATH del usuario (HKCU\\Environment).")
			fmt.Println("[AVISO] Las ventanas de terminal abiertas conservan el PATH viejo: hay que reiniciarlas.")
		} else {
			fmt.Println("[INFO] Esa carpeta ya estaba en el PATH del usuario.")
		}
	}
	return nsisEnv(dir), nil
}

// ── NDK ─────────────────────────────────────────────────────────────────────

// ndkRoot: ANDROID_NDK_HOME → ctx.NdkHome → la más reciente bajo $ANDROID_HOME/ndk.
func wailsNdkRoot(ctx *project.ProjectContext) (string, error) {
	if v := os.Getenv("ANDROID_NDK_HOME"); v != "" {
		if dirExists(v) {
			return v, nil
		}
	}
	if ctx.NdkHome != "" && dirExists(ctx.NdkHome) {
		return ctx.NdkHome, nil
	}
	sdk := ctx.AndroidHome
	if sdk == "" {
		return "", fmt.Errorf("[ERROR] No se encontró la NDK de Android.\n" +
			"   Instala una con: sdkmanager \"ndk;27.0.12077973\"\n" +
			"   o define ANDROID_NDK_HOME.")
	}
	entries, err := os.ReadDir(filepath.Join(sdk, "ndk"))
	if err != nil {
		return "", fmt.Errorf("[ERROR] No se encontró la NDK de Android.\n" +
			"   Instala una con: sdkmanager \"ndk;27.0.12077973\"\n" +
			"   o define ANDROID_NDK_HOME.")
	}
	type ver struct {
		key []int
		dir string
	}
	var versions []ver
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		var key []int
		for _, p := range strings.Split(e.Name(), ".") {
			n, err := strconv.Atoi(p)
			if err != nil {
				key = nil
				break
			}
			key = append(key, n)
		}
		if len(key) == 0 {
			continue
		}
		versions = append(versions, ver{key: key, dir: filepath.Join(sdk, "ndk", e.Name())})
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("[ERROR] No se encontró la NDK de Android.\n" +
			"   Instala una con: sdkmanager \"ndk;27.0.12077973\"\n" +
			"   o define ANDROID_NDK_HOME.")
	}
	// la más reciente por número de versión.
	best := versions[0]
	for _, v := range versions[1:] {
		greater := false
		n := len(v.key)
		if len(best.key) < n {
			n = len(best.key)
		}
		for i := 0; i < n; i++ {
			if v.key[i] != best.key[i] {
				greater = v.key[i] > best.key[i]
				break
			}
		}
		if greater {
			best = v
		}
	}
	return best.dir, nil
}

// ndkToolchain: carpeta bin de la toolchain LLVM de la NDK para Windows.
func wailsNdkToolchain(ndk string) (string, error) {
	bin := filepath.Join(ndk, "toolchains", "llvm", "prebuilt", "windows-x86_64", "bin")
	if dirExists(bin) {
		return bin, nil
	}
	return "", fmt.Errorf("[ERROR] La NDK de %s no tiene la toolchain LLVM para este host.", ndk)
}

// ndkCompilers devuelve los compiladores cc/cxx con --target explícito.
func wailsNdkCompilers(bin, triple string) (string, string, error) {
	clang := filepath.Join(bin, "clang.exe")
	if !fileExists(clang) {
		return "", "", fmt.Errorf("no se encontró %s", clang)
	}
	target := fmt.Sprintf("%s%d", triple, wailsMinSdk)
	cc := fmt.Sprintf("%s --target=%s", clang, target)
	cxx := fmt.Sprintf("%s --target=%s", filepath.Join(bin, "clang++.exe"), target)
	return cc, cxx, nil
}

// npmRun ejecuta `npm run <script>` en el frontend (npm.cmd evita el shim .ps1).
func wailsNpmRun(dir, script string) error {
	ok, err := shell.RunPSStatus("npm.cmd run "+script, dir, nil)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("`npm run %s` falló en: %s", script, dir)
	}
	return nil
}

// ───────────────────────────────────────────────────────────────────────────
// Escritorio
// ───────────────────────────────────────────────────────────────────────────

// WailsDev: modo desarrollo de escritorio (`wails3 task dev`) en ventana nueva.
func WailsDev(ctx *project.ProjectContext) error {
	if err := ensureWailsCli(); err != nil {
		return err
	}
	// Best-effort: genera el .syso de icono/version para que el .exe no lleva
	// el icono genérico de Go.
	_, _ = shell.RunProgram("wails3", []string{
		"generate", "syso",
		"-arch", "amd64",
		"-icon", "build/windows/icon.ico",
		"-manifest", "build/windows/wails.exe.manifest",
		"-info", "build/windows/info.json",
		"-out", "wails_windows_amd64.syso",
	}, ctx.ScriptDir, nil)

	shell.ClearScreen()
	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Println("║          WAILS v3 - DEV (ESCRITORIO)             ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("[INFO] Comando: wails3 task dev")
	fmt.Println("[INFO] El servidor de Vite y la app se ejecutan en esta pestaña.")
	fmt.Println()
	return shell.SpawnNewWindowInteractive("Wails Dev", ctx.ScriptDir, "wails3 task dev")
}

// desktopArch lista las arquitecturas de escritorio de Wails.
type desktopArch int

const (
	archX86 desktopArch = iota
	archX64
	archArm64
)

func (a desktopArch) label() string {
	switch a {
	case archX86:
		return "x86"
	case archX64:
		return "x64"
	case archArm64:
		return "ARM64"
	}
	return ""
}

// WailsBuildDesktop compila x86, x64 y ARM64 y deja los paquetes en bin/.
func WailsBuildDesktop(ctx *project.ProjectContext) error {
	fmt.Print("\n[INFO] Compilando escritorio para x86, x64 y ARM64...\n\n")
	var artifacts []string
	for i, a := range []desktopArch{archX86, archX64, archArm64} {
		fmt.Printf("[%d/3] Windows %s...\n", i+1, a.label())
		arts, err := wailsBuildDesktopArch(ctx, a)
		if err != nil {
			return err
		}
		artifacts = append(artifacts, arts...)
	}
	fmt.Println()
	fmt.Println("[OK] Paquetes generados:")
	for _, a := range artifacts {
		fmt.Printf("  - %s\n", a)
	}
	if settings.Load().OpenOutputDir {
		shell.OpenFolder(wailsBinDir(ctx))
	}
	ui.PauseFor(10)
	return nil
}

// WailsPackageDesktop es alias de build_desktop.
func WailsPackageDesktop(ctx *project.ProjectContext) error {
	return WailsBuildDesktop(ctx)
}

// wailsBuildDesktopArch compila/empaqueta una arquitectura y nombra los artefactos en BIN_DIR.
func wailsBuildDesktopArch(ctx *project.ProjectContext, a desktopArch) ([]string, error) {
	if err := ensureWailsCli(); err != nil {
		return nil, err
	}
	bin := wailsBinDir(ctx)
	if err := os.MkdirAll(bin, 0o755); err != nil {
		return nil, fmt.Errorf("no se pudo crear el directorio de salida %s: %w", bin, err)
	}

	name := wailsAppName(ctx)
	_, version := wailsReleaseMeta(ctx)
	portable := filepath.Join(bin, wailsAppName(ctx)+".exe")

	switch a {
	case archX86:
		fmt.Println("[INFO] Windows x86: binario portable (`wails3 task build ARCH=386`).")
		ok, err := shell.RunShellStatus("wails3 task build ARCH=386", ctx.ScriptDir, nil)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("[ERROR] Falló el build x86 (`wails3 task build ARCH=386`).")
		}
		dest := filepath.Join(bin, wailsDesktopArtifact(name, version, "x86 portable", "exe"))
		if err := moveWailsArtifact(portable, dest); err != nil {
			return nil, err
		}
		return []string{dest}, nil
	default:
		var arg, archLabel string
		if a == archArm64 {
			arg, archLabel = "arm64", "ARM64"
		} else {
			arg, archLabel = "amd64", "x64"
		}
		portableLabel := "portable"
		if a == archArm64 {
			portableLabel = archLabel + " portable"
		}
		label := a.label()
		fmt.Printf("[INFO] Windows %s: portable + instalador NSIS (`wails3 task package ARCH=%s`).\n", label, arg)
		envs, err := ensureNsis()
		if err != nil {
			return nil, err
		}
		ok, err := shell.RunShellStatus(fmt.Sprintf("wails3 task package ARCH=%s", arg), ctx.ScriptDir, envs)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("[ERROR] Falló el paquete %s (`wails3 task package ARCH=%s`).", label, arg)
		}
		portableDest := filepath.Join(bin, wailsDesktopArtifact(name, version, portableLabel, "exe"))
		installer := filepath.Join(bin, fmt.Sprintf("%s-%s-installer.exe", wailsAppName(ctx), arg))
		installerDest := filepath.Join(bin, wailsDesktopArtifact(name, version, archLabel+" installer", "exe"))
		if err := moveWailsArtifact(portable, portableDest); err != nil {
			return nil, err
		}
		if err := moveWailsArtifact(installer, installerDest); err != nil {
			return nil, err
		}
		return []string{portableDest, installerDest}, nil
	}
}

// ───────────────────────────────────────────────────────────────────────────
// Android
// ───────────────────────────────────────────────────────────────────────────

// prepareAndroid deja el proyecto Android listo para Gradle (overlay, bindings,
// frontend y libwails.so por ABI). Devuelve el entorno de firma o nil.
func wailsPrepareAndroid(ctx *project.ProjectContext, release bool, abis []abi) (map[string]string, error) {
	if err := ensureWailsCli(); err != nil {
		return nil, err
	}
	android := wailsAndroidDir(ctx)
	if !fileExists(filepath.Join(android, "settings.gradle")) &&
		!fileExists(filepath.Join(android, "settings.gradle.kts")) {
		return nil, fmt.Errorf("[ERROR] No se encontró el proyecto Android de Wails en: %s\n"+
			"   Regenera los assets de build con: wails3 update build-assets", android)
	}
	ndk, err := wailsNdkRoot(ctx)
	if err != nil {
		return nil, err
	}
	toolchain, err := wailsNdkToolchain(ndk)
	if err != nil {
		return nil, err
	}
	root := ctx.ScriptDir

	// 1. Overlay.
	fmt.Println("[1/5] Generando el overlay de Android...")
	overlay := filepath.Join(android, "overlay.json")
	if ok, err := shell.RunProgram("wails3", []string{
		"android", "overlay:gen",
		"-out", overlay,
		"-config", "build/config.yml",
	}, root, nil); err != nil {
		return nil, err
	} else if !ok {
		return nil, fmt.Errorf("[ERROR] Falló `wails3 android overlay:gen`.")
	}

	// 2. Bindings. Wails v3 las genera para Android con CGO desactivado (la
	// ruta sin cgo existe justo para que el generador pueda cargar el paquete
	// sin la NDK). GOARCH=arm64 evita android/amd64, que exige enlace externo
	// con cgo y hace fallar la carga de paquetes en el host. Los tags de
	// release/debug son solo para compilar libwails.so; el generador usa
	// siempre `android`.
	fmt.Println("[2/5] Generando los bindings...")
	bo, err := shell.RunProgramCapture("wails3", []string{
		"generate", "bindings",
		"-f", "-tags android",
		"-clean=true", "-ts", "-i",
	}, root, map[string]string{
		"GOOS":        "android",
		"GOARCH":      "arm64",
		"CGO_ENABLED": "0",
	})
	if err != nil || strings.Contains(string(bo), "could not import") {
		out := strings.TrimSpace(string(bo))
		return nil, fmt.Errorf("[ERROR] El generador de bindings no pudo cargar los paquetes de Go.\n"+
			"   Se genera con GOOS=android y CGO_ENABLED=0 (sin NDK), solo con el tag android.\n%s", out)
	}

	// 3. Frontend.
	fmt.Println("[3/5] Compilando el frontend...")
	frontend := filepath.Join(root, "frontend")
	if fileExists(filepath.Join(frontend, "package.json")) {
		if !dirExists(filepath.Join(frontend, "node_modules")) {
			runNpmInstall(frontend)
		}
		if err := wailsNpmRun(frontend, "build"); err != nil {
			return nil, err
		}
	} else {
		fmt.Println("[WARN] No hay frontend/ con package.json; se sigue sin frontend.")
	}

	// 4. libwails.so por ABI.
	fmt.Println("[4/5] Compilando libwails.so con Go + NDK...")
	goArgs := []string{"build", "-buildmode=c-shared", "-overlay", overlay}
	if release {
		goArgs = append(goArgs, "-tags", "production,android", "-trimpath", "-buildvcs=false", "-ldflags=-w -s")
	} else {
		goArgs = append(goArgs, "-tags", "android,debug", "-buildvcs=false", "-gcflags=all=-l")
	}
	for _, a := range abis {
		cc, cxx, err := wailsNdkCompilers(toolchain, a.clangTriple)
		if err != nil {
			return nil, fmt.Errorf("[ERROR] No se encontró el compilador %s%d en: %s", a.clangTriple, wailsMinSdk, toolchain)
		}
		jniDir := filepath.Join(android, "app", "src", "main", "jniLibs", a.jniDir)
		os.MkdirAll(jniDir, 0o755)
		soPath := filepath.Join(jniDir, "libwails.so")

		args := append([]string{}, goArgs...)
		args = append(args, "-o", soPath)

		envs := map[string]string{
			"GOOS":        "android",
			"GOARCH":      a.goarch,
			"CGO_ENABLED": "1",
			"CC":          cc,
			"CXX":         cxx,
		}
		if a.goarm != "" {
			envs["GOARM"] = a.goarm
		}
		fmt.Printf("      → %s (GOARCH=%s)\n", a.jniDir, a.goarch)
		ok, err := shell.RunProgram("go", args, root, envs)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("[ERROR] Falló la compilación de libwails.so para %s.\n"+
				"   Revisa que la NDK sea compatible y que GOOS/GOARCH/CGO_ENABLED sean correctos.", a.jniDir)
		}
	}

	wailsEnsureSdkProperties(ctx, android)

	signing := wailsSigningEnv(ctx)
	if release && signing == nil {
		fmt.Println("[WARN] No hay keystore de release configurado; la APK se firmará con la llave de dev.")
		fmt.Println("       Configúralo en la tarjeta Signing (o con --task sign).")
	}
	return signing, nil
}

// writeInitScript escribe un init script de Gradle que inyecta versión, ABI y
// splits. Se aplica con --init-script sin editar build/android/.
func wailsWriteInitScript(version string, abis []abi, splits bool) (string, error) {
	var abiList []string
	for _, a := range abis {
		abiList = append(abiList, "'"+a.jniDir+"'")
	}
	joined := strings.Join(abiList, ", ")

	var sb strings.Builder
	sb.WriteString("// Generado por Compass: version y ABI del build (build/android/ se regenera).\n")
	sb.WriteString("gradle.beforeProject { p ->\n")
	sb.WriteString("    p.afterEvaluate {\n")
	sb.WriteString("        def android = p.extensions.findByName('android')\n")
	sb.WriteString("        if (android == null) { return }\n")
	if version != "" {
		escaped := strings.ReplaceAll(version, "\\", "\\\\")
		escaped = strings.ReplaceAll(escaped, "'", "\\'")
		fmt.Fprintf(&sb, "        android.defaultConfig.versionName = '%s'\n", escaped)
		if code, ok := wailsVersionCode(version); ok {
			fmt.Fprintf(&sb, "        android.defaultConfig.versionCode = %d\n", code)
		}
	}
	sb.WriteString("        android.defaultConfig.ndk.abiFilters.clear()\n")
	fmt.Fprintf(&sb, "        android.defaultConfig.ndk.abiFilters.addAll([%s])\n", joined)
	hasArmv7 := false
	for _, a := range abis {
		if a.jniDir == "armeabi-v7a" {
			hasArmv7 = true
		}
	}
	if hasArmv7 {
		sb.WriteString("        android.packagingOptions.doNotStrip '*/armeabi-v7a/libwails.so'\n")
	}
	if splits {
		sb.WriteString("        android.splits.abi.enable = true\n")
		sb.WriteString("        android.splits.abi.reset()\n")
		fmt.Fprintf(&sb, "        android.splits.abi.include %s\n", joined)
		sb.WriteString("        android.splits.abi.universalApk = true\n")
	}
	sb.WriteString("    }\n}\n")

	f, err := os.CreateTemp("", fmt.Sprintf("compass-wails-%d-*.gradle", os.Getpid()))
	if err != nil {
		return "", err
	}
	if _, err := f.WriteString(sb.String()); err != nil {
		f.Close()
		return "", err
	}
	f.Close()
	return f.Name(), nil
}

// wailsGradle ejecuta una tarea de Gradle en build/android.
func wailsGradle(android, task string, envs map[string]string) (bool, error) {
	if findGradlew(android) == "" {
		return false, fmt.Errorf("[ERROR] No se encontró el wrapper de Gradle en: %s\n"+
			"   Regenera los assets de build con: wails3 update build-assets", android)
	}
	script := fmt.Sprintf(".\\gradlew.bat %s", task)
	return shell.RunPSStatus(script, android, envs)
}

// wailsEnsureSdkProperties escribe local.properties con sdk.dir si hace falta.
func wailsEnsureSdkProperties(ctx *project.ProjectContext, android string) error {
	if os.Getenv("ANDROID_HOME") != "" || os.Getenv("ANDROID_SDK_ROOT") != "" {
		return nil
	}
	if ctx.AndroidHome == "" {
		return nil
	}
	props := filepath.Join(android, "local.properties")
	content := fmt.Sprintf("sdk.dir=%s\n", ctx.AndroidHome)
	if data, err := os.ReadFile(props); err == nil {
		if strings.Contains(string(data), content) {
			return nil
		}
	}
	return os.WriteFile(props, []byte(content), 0o644)
}

// newestIn devuelve el archivo más reciente (recursivo) con la extensión dada.
func wailsNewestIn(dir, ext string) (string, bool) {
	return wailsNewestMatching(dir, ext, "")
}

// newestMatching devuelve el archivo más reciente cuya extensión coincide y cuyo
// nombre contiene needle (vacío = cualquiera).
func wailsNewestMatching(dir, ext, needle string) (string, bool) {
	var bestTime time.Time
	best := ""
	var rec func(string)
	rec = func(p string) {
		entries, err := os.ReadDir(p)
		if err != nil {
			return
		}
		for _, e := range entries {
			path := filepath.Join(p, e.Name())
			if e.IsDir() {
				rec(path)
				continue
			}
			if strings.EqualFold(filepath.Ext(path), "."+ext) {
				if needle != "" && !strings.Contains(filepath.Base(path), needle) {
					continue
				}
				if info, err := e.Info(); err == nil {
					if info.ModTime().After(bestTime) {
						bestTime = info.ModTime()
						best = path
					}
				}
			}
		}
	}
	if dirExists(dir) {
		rec(dir)
	}
	if best == "" {
		return "", false
	}
	return best, true
}

// copyArtifact copia un artefacto a su destino y lo anuncia.
func copyArtifact(src, dest string) string {
	if err := copyFile(src, dest); err != nil {
		fmt.Printf("[WARN] No se pudo copiar a %s: %v\n", dest, err)
		return src
	}
	fmt.Printf("[OK] %s\n", dest)
	return dest
}

func moveWailsArtifact(src, dest string) error {
	if filepath.Clean(src) == filepath.Clean(dest) {
		return nil
	}
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("no se encontró el artefacto generado %s: %w", src, err)
	}
	if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("no se pudo reemplazar %s: %w", dest, err)
	}
	if err := os.Rename(src, dest); err != nil {
		return fmt.Errorf("no se pudo renombrar %s a %s: %w", src, dest, err)
	}
	fmt.Printf("[OK] %s\n", dest)
	return nil
}

// wailsSigningEnv devuelve las variables de firma que espera el Gradle de Wails
// (ANDROID_KEYSTORE_FILE, ...), leyendo android-signing/keystore.properties.
func wailsSigningEnv(ctx *project.ProjectContext) map[string]string {
	signingDir := filepath.Join(ctx.ScriptDir, "android-signing")
	propsPath := filepath.Join(signingDir, "keystore.properties")
	props, ok := readProps(propsPath)
	if !ok || props.StoreFile == "" {
		return nil
	}
	storePath := resolveKeystore(signingDir, props.StoreFile)
	if !fileExists(storePath) {
		return nil
	}
	storePass := props.StorePassword
	if storePass == "" {
		storePass = props.KeyPassword
	}
	keyPass := props.KeyPassword
	if keyPass == "" {
		keyPass = storePass
	}
	keyAlias := props.KeyAlias
	return map[string]string{
		"ANDROID_KEYSTORE_FILE":     storePath,
		"ANDROID_KEYSTORE_PASSWORD": storePass,
		"ANDROID_KEY_ALIAS":         keyAlias,
		"ANDROID_KEY_PASSWORD":      keyPass,
	}
}

// androidApkOutputDir: carpeta de salida de APKs del módulo Android.
func wailsApkReleaseDir(android string) string {
	return filepath.Join(android, "app", "build", "outputs", "apk", "release")
}

// buildReleaseApks compila las 3 APK release (arm64, armv7, universal).
func wailsBuildReleaseApks(ctx *project.ProjectContext) ([]string, error) {
	signing, err := wailsPrepareAndroid(ctx, true, releaseAbis)
	if err != nil {
		return nil, err
	}
	android := wailsAndroidDir(ctx)
	name, version := wailsReleaseMeta(ctx)
	fmt.Println("[5/5] Ensamblando las APK con Gradle (splits por ABI + universal)...")
	initScript, err := wailsWriteInitScript(version, releaseAbis, true)
	if err != nil {
		return nil, err
	}
	task := fmt.Sprintf("--init-script %s assembleRelease", shell.PsQuote(initScript))
	built, err := wailsGradle(android, task, signing)
	if err != nil {
		os.Remove(initScript)
		return nil, err
	}
	os.Remove(initScript)
	if !built {
		return nil, fmt.Errorf("[ERROR] Gradle falló en `assembleRelease`.")
	}
	bin := wailsBinDir(ctx)
	os.MkdirAll(bin, 0o755)
	out := wailsApkReleaseDir(android)
	var artifacts []string
	for _, a := range releaseAbis {
		src, ok := wailsNewestMatching(out, "apk", a.jniDir)
		if !ok {
			return nil, fmt.Errorf("[ERROR] Gradle terminó bien pero no se encontró la APK '%s' en: %s", a.jniDir, out)
		}
		dest := filepath.Join(bin, wailsVersionedArtifact(name, version, a.artifact, "apk"))
		artifacts = append(artifacts, copyArtifact(src, dest))
	}
	universal, ok := wailsNewestMatching(out, "apk", "universal")
	if !ok {
		return nil, fmt.Errorf("[ERROR] Gradle terminó bien pero no se encontró la APK 'universal' en: %s", out)
	}
	dest := filepath.Join(bin, wailsVersionedArtifact(name, version, "universal", "apk"))
	artifacts = append(artifacts, copyArtifact(universal, dest))
	_ = copyFile(universal, filepath.Join(bin, wailsAppName(ctx)+".apk"))
	return artifacts, nil
}

// buildApkArch compila la APK release de una sola ABI (sin splits).
func wailsBuildApkArch(ctx *project.ProjectContext, a abi) (string, error) {
	abis := []abi{a}
	signing, err := wailsPrepareAndroid(ctx, true, abis)
	if err != nil {
		return "", err
	}
	android := wailsAndroidDir(ctx)
	name, version := wailsReleaseMeta(ctx)
	fmt.Printf("[5/5] Ensamblando la APK %s con Gradle...\n", a.artifact)
	initScript, err := wailsWriteInitScript(version, abis, false)
	if err != nil {
		return "", err
	}
	task := fmt.Sprintf("--init-script %s assembleRelease", shell.PsQuote(initScript))
	built, err := wailsGradle(android, task, signing)
	if err != nil {
		os.Remove(initScript)
		return "", err
	}
	os.Remove(initScript)
	if !built {
		return "", fmt.Errorf("[ERROR] Gradle falló en `assembleRelease`.")
	}
	out := wailsApkReleaseDir(android)
	needle := "app-release"
	if a.artifact == "armv7" {
		needle = "app-armeabi-v7a-release"
	} else {
		needle = "app-arm64-v8a-release"
	}
	apk, ok := wailsNewestMatching(out, "apk", needle)
	if !ok {
		apk, ok = wailsNewestIn(out, "apk")
	}
	if !ok {
		return "", fmt.Errorf("[ERROR] Gradle terminó bien pero no se encontró la APK en: %s", out)
	}
	bin := wailsBinDir(ctx)
	os.MkdirAll(bin, 0o755)
	return copyArtifact(apk, filepath.Join(bin, wailsVersionedArtifact(name, version, a.artifact, "apk"))), nil
}

// buildApkUniversal compila la APK universal release (sin splits).
func wailsBuildApkUniversal(ctx *project.ProjectContext) (string, error) {
	signing, err := wailsPrepareAndroid(ctx, true, releaseAbis)
	if err != nil {
		return "", err
	}
	android := wailsAndroidDir(ctx)
	name, version := wailsReleaseMeta(ctx)
	fmt.Println("[5/5] Ensamblando la APK universal con Gradle...")
	initScript, err := wailsWriteInitScript(version, releaseAbis, false)
	if err != nil {
		return "", err
	}
	task := fmt.Sprintf("--init-script %s assembleRelease", shell.PsQuote(initScript))
	built, err := wailsGradle(android, task, signing)
	if err != nil {
		os.Remove(initScript)
		return "", err
	}
	os.Remove(initScript)
	if !built {
		return "", fmt.Errorf("[ERROR] Gradle falló en `assembleRelease`.")
	}
	out := wailsApkReleaseDir(android)
	apk, ok := wailsNewestMatching(out, "apk", "app-release")
	if !ok {
		apk, ok = wailsNewestIn(out, "apk")
	}
	if !ok {
		return "", fmt.Errorf("[ERROR] Gradle terminó bien pero no se encontró la APK en: %s", out)
	}
	bin := wailsBinDir(ctx)
	os.MkdirAll(bin, 0o755)
	dest := filepath.Join(bin, wailsVersionedArtifact(name, version, "universal", "apk"))
	copied := copyArtifact(apk, dest)
	_ = copyFile(apk, filepath.Join(bin, wailsAppName(ctx)+".apk"))
	return copied, nil
}

// buildDebugApk compila la APK de debug (arm64) para instalar.
func wailsBuildDebugApk(ctx *project.ProjectContext) (string, error) {
	if _, err := wailsPrepareAndroid(ctx, false, []abi{abiArm64}); err != nil {
		return "", err
	}
	android := wailsAndroidDir(ctx)
	fmt.Println("[5/5] Ensamblando la APK de debug con Gradle...")
	built, err := wailsGradle(android, "assembleDebug", nil)
	if err != nil {
		return "", err
	}
	if !built {
		return "", fmt.Errorf("[ERROR] Gradle falló en `assembleDebug`.")
	}
	out := filepath.Join(android, "app", "build", "outputs", "apk", "debug")
	apk, ok := wailsNewestIn(out, "apk")
	if !ok {
		return "", fmt.Errorf("[ERROR] Gradle terminó bien pero no se encontró la APK en: %s", out)
	}
	return apk, nil
}

// WailsBuildApk: APK release (arm64, armv7 y universal) + instalación si hay device.
func WailsBuildApk(ctx *project.ProjectContext) error {
	fmt.Print("\n[INFO] Compilando Android (Release) con Wails...\n\n")
	fmt.Print("[INFO] Se generan tres APK: arm64, armv7 y universal.\n\n")
	result := func() error {
		apks, err := wailsBuildReleaseApks(ctx)
		if err != nil {
			return err
		}
		if len(apks) > 0 {
			installApkIfDevice(ctx, apks[len(apks)-1], wailsApplicationID(ctx), wailsMainActivity(ctx))
		}
		return nil
	}()
	if result != nil {
		fmt.Println()
		fmt.Println(result)
		fmt.Println()
		ui.Pause()
		return result
	}
	ui.PauseFor(10)
	return nil
}

// WailsBuildApkArmv7: APK release sólo armv7.
func WailsBuildApkArmv7(ctx *project.ProjectContext) error {
	fmt.Print("\n[INFO] Compilando Android APK armv7 (Release) con Wails...\n\n")
	if _, err := wailsBuildApkArch(ctx, abiArmv7); err != nil {
		fmt.Println()
		fmt.Println(err)
		fmt.Println()
		ui.Pause()
		return err
	}
	ui.PauseFor(10)
	return nil
}

// WailsInstallDebug: APK dev + instalación y arranque en el teléfono.
func WailsInstallDebug(ctx *project.ProjectContext) error {
	fmt.Print("\n[INFO] Compilando Android APK (Dev) con Wails...\n\n")
	result := func() error {
		apk, err := wailsBuildDebugApk(ctx)
		if err != nil {
			return err
		}
		installApkIfDevice(ctx, apk, wailsApplicationID(ctx), wailsMainActivity(ctx))
		return nil
	}()
	if result != nil {
		fmt.Println()
		fmt.Println(result)
		fmt.Println()
		ui.Pause()
		return result
	}
	ui.PauseFor(10)
	return nil
}

// WailsBuildAab: AAB release para Play Store.
func WailsBuildAab(ctx *project.ProjectContext) error {
	fmt.Print("\n[INFO] Compilando Android AAB (Release) con Wails...\n\n")
	aab, err := func() (string, error) {
		signing, err := wailsPrepareAndroid(ctx, true, releaseAbis)
		if err != nil {
			return "", err
		}
		android := wailsAndroidDir(ctx)
		name, version := wailsReleaseMeta(ctx)
		fmt.Println("[5/5] Empaquetando el AAB con Gradle...")
		initScript, err := wailsWriteInitScript(version, releaseAbis, false)
		if err != nil {
			return "", err
		}
		task := fmt.Sprintf("--init-script %s bundleRelease", shell.PsQuote(initScript))
		built, err := wailsGradle(android, task, signing)
		os.Remove(initScript)
		if err != nil {
			return "", err
		}
		if !built {
			return "", fmt.Errorf("[ERROR] Gradle falló en `bundleRelease`.")
		}
		aabDir := filepath.Join(android, "app", "build", "outputs", "bundle", "release")
		aab, ok := wailsNewestIn(aabDir, "aab")
		if !ok {
			return "", fmt.Errorf("[ERROR] Gradle terminó bien pero no se encontró el AAB en: %s", aabDir)
		}
		bin := wailsBinDir(ctx)
		os.MkdirAll(bin, 0o755)
		dest := filepath.Join(bin, wailsVersionedArtifact(name, version, "", "aab"))
		copied := copyArtifact(aab, dest)
		_ = copyFile(aab, filepath.Join(bin, wailsAppName(ctx)+".aab"))
		return copied, nil
	}()
	if err != nil {
		fmt.Println()
		fmt.Println(err)
		fmt.Println()
		ui.Pause()
		return err
	}
	fmt.Println()
	fmt.Println("[OK] AAB generado correctamente en:", aab)
	fmt.Println("[INFO] Un AAB no se instala directamente en el teléfono; es para Play Store.")
	ui.PauseFor(10)
	return nil
}

// WailsDebugAndroid: logcat de la app en el dispositivo.
func WailsDebugAndroid(ctx *project.ProjectContext) error {
	pkg := wailsApplicationID(ctx)
	fmt.Printf("\n[INFO] Depurando %s en el dispositivo...\n", pkg)
	device, err := selectAndroidDevice(ctx)
	if err != nil {
		return err
	}
	adb := ctx.AdbPath
	if adb == "" {
		adb = "adb"
	}
	pid, err := shell.HideConsole(exec.Command(adb, "-s", device, "shell", "pidof", pkg)).CombinedOutput()
	pidStr := strings.TrimSpace(string(pid))
	var command string
	if pidStr == "" {
		command = fmt.Sprintf("\"%s\" -s %s logcat -v time AndroidRuntime:E", adb, device)
	} else {
		command = fmt.Sprintf("\"%s\" -s %s logcat -v time --pid=%s", adb, device, pidStr)
	}
	return shell.SpawnNewWindowInteractive("Wails - logcat", ctx.ScriptDir, command)
}

// ───────────────────────────────────────────────────────────────────────────
// HMR en Android
// ───────────────────────────────────────────────────────────────────────────

// WailsDevAndroid: HMR en Android (`wails3 task android:dev`).
func WailsDevAndroid(ctx *project.ProjectContext) error {
	if err := ensureWailsCli(); err != nil {
		return err
	}
	port := 9245
	if v := os.Getenv("WAILS_VITE_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			port = n
		}
	}
	// Comprueba que Vite esté escuchando en el puerto.
	viteOk := false
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 3*time.Second)
	if err == nil {
		conn.Close()
		viteOk = true
	}

	shell.ClearScreen()
	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Println("║       WAILS v3 - HMR EN ANDROID                  ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("[INFO] Puerto de Vite esperado: http://localhost:%d\n", port)
	fmt.Println()

	if !viteOk {
		fmt.Printf("[WARN] Vite no está corriendo en el puerto %d.\n", port)
		fmt.Println()
		fmt.Println("  Abre otra terminal, ve a frontend/ y ejecuta:")
		fmt.Println("    npm run dev")
		fmt.Println()
		fmt.Println("  Cuando Vite esté corriendo, vuelve a lanzar esta tarjeta.")
		fmt.Println("  (La tarea android:dev también lo comprueba, pero así lo ves antes.)")
		fmt.Println()
		ui.Pause()
		return nil
	}
	fmt.Printf("[OK] Vite detectado en localhost:%d.\n", port)
	fmt.Println()

	// Parche del MIME type para HMR (idempotente).
	wailsPatchWailsBridgeMime(ctx)

	fmt.Println("[WARN] HMR en Android es experimental en Wails v3: la pantalla puede")
	fmt.Println("       quedar en blanco si el asset server no sirve algún módulo con")
	fmt.Println("       el MIME correcto. Si pasa, usa chrome://inspect para ver el")
	fmt.Println("       error exacto en la consola del WebView.")
	fmt.Println()

	fmt.Println("[INFO] Desinstalando la versión anterior (evita el choque de firma")
	fmt.Println("[INFO] debug ↔ dev que da INSTALL_FAILED_UPDATE_INCOMPATIBLE)...")
	pkg := wailsApplicationID(ctx)
	if ctx.AdbPath != "" {
		_, _ = adbCommand(ctx, "uninstall", pkg)
	}
	fmt.Println()
	fmt.Println("[INFO] Lanzando: wails3 task android:dev")
	fmt.Println("[INFO] La tarea compila el .so, publica el puerto con `adb reverse`,")
	fmt.Println("[INFO] instala la APK de dev y arranca la app.")
	fmt.Println()

	return shell.SpawnNewWindowInteractive("Wails Android HMR", ctx.ScriptDir, "wails3 task android:dev")
}

// wailsPatchWailsBridgeMime parchea WailsBridge.java para servir los módulos de
// Vite (HMR) con el MIME correcto. Idempotente.
func wailsPatchWailsBridgeMime(ctx *project.ProjectContext) {
	path := filepath.Join(ctx.ScriptDir, "build", "android", "app", "src", "main",
		"java", "com", "wails", "app", "WailsBridge.java")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	content := string(data)
	if strings.Contains(content, "MimeTypeMap.getSingleton()") {
		return
	}
	old := `        try {
            String mimeType = nativeGetAssetMimeType(path);
            return mimeType != null ? mimeType : "application/octet-stream";
        } catch (Exception e) {
            return "application/octet-stream";
        }`
	new := `        // El asset server de Wails devuelve "application/octet-stream" para los
        // modulos de Vite (@vite/client, src/main.ts) -> el WebView los rechaza.
        // Por eso se resuelve PRIMERO el MIME por extension, antes de confiar
        // en nativeGetAssetMimeType. Se usa MimeTypeMap de Android para cubrir
        // todos los tipos (.ts, .svelte, .css, .js, .json, ...) sin listarlos
        // uno a uno.
        String lower = path.toLowerCase();
        int dot = lower.lastIndexOf('.');
        if (dot >= 0 && dot < lower.length() - 1) {
            String ext = lower.substring(dot + 1);
            String mime = android.webkit.MimeTypeMap.getSingleton()
                    .getMimeTypeFromExtension(ext);
            if (mime != null && !mime.isEmpty()) {
                return mime;
            }
        }
        if (lower.contains("@vite")) {
            return "application/javascript";
        }
        try {
            String mimeType = nativeGetAssetMimeType(path);
            if (mimeType != null && !mimeType.equals("application/octet-stream")) {
                return mimeType;
            }
        } catch (Exception e) {
            // fall through to the default below
        }
        return "application/octet-stream";`
	if i := strings.Index(content, old); i >= 0 {
		patched := content[:i] + new + content[i+len(old):]
		if err := os.WriteFile(path, []byte(patched), 0o644); err == nil {
			fmt.Println("[INFO] Parche MIME aplicado a WailsBridge.java (HMR).")
		}
	}
}

// ───────────────────────────────────────────────────────────────────────────
// Utilidades de red (HMR)
// ───────────────────────────────────────────────────────────────────────────
