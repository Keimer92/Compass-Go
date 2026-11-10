package tasks

// Tareas de Android para proyectos Tauri (equivalente de src/tasks/android.rs).
// Mantiene las funciones compartidas de android.go (Device, adbCommand, etc.) y
// añade dev/build/debug de Android para TypeTauri.

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"compass/internal/project"
	"compass/internal/prompts"
	"compass/internal/shell"
	"compass/internal/ui"
)

// ndkVersion que Compass instala/usa (debe coincidir con project.ndkVersion).
const tauriNdkVersion = "27.0.12077973"

const npmDevAndroidScript = "dev:android"

func init() {
	Register("android-dev", project.TypeTauri, TauriAndroidDev)
	Register("android-arm64", project.TypeTauri, TauriBuildArm64)
	Register("android-armv7", project.TypeTauri, TauriBuildArmv7)
	Register("android-all", project.TypeTauri, TauriBuildAll)
	Register("android-aab", project.TypeTauri, TauriBuildAab)
	Register("debug-android", project.TypeTauri, TauriDebugAndroid)
}

// ───────────────────────────────────────────────────────────────────────────
// Precondiciones
// ───────────────────────────────────────────────────────────────────────────

// tauriAndroidInit inicializa el proyecto Android de Tauri.
func tauriAndroidInit(ctx *project.ProjectContext) error {
	if dirExists(filepath.Join(ctx.ScriptDir, "node_modules", "@tauri-apps", "cli")) {
		return shell.RunInDirStatus("npx", ctx.ScriptDir, "tauri", "android", "init")
	}
	return shell.RunInDirStatus("cargo", ctx.ScriptDir, "tauri", "android", "init")
}

// tauriEnsureAndroidProject asegura que exista el proyecto Android generado.
func tauriEnsureAndroidProject(ctx *project.ProjectContext) error {
	if ctx.HasAndroidProject {
		return nil
	}
	fmt.Println("[WARNING] No se encontró el proyecto Android.")
	fmt.Println("[INFO] Inicializando proyecto Android con Tauri...")
	if err := tauriAndroidInit(ctx); err != nil {
		return err
	}
	gradlew := filepath.Join(ctx.ScriptDir, "src-tauri", "gen", "android", "gradlew.bat")
	if !fileExists(gradlew) {
		return fmt.Errorf("[ERROR] No se pudo inicializar Gradle en el proyecto Android.")
	}
	return nil
}

// readTauriIdentifier lee el identifier de tauri.conf.json.
func readTauriIdentifier(ctx *project.ProjectContext) string {
	data, err := os.ReadFile(filepath.Join(ctx.ScriptDir, "src-tauri", "tauri.conf.json"))
	if err != nil {
		return ""
	}
	return extractJSONField(string(data), "identifier")
}

// findKotlinPackage busca el primer directorio hoja que contenga .kt bajo root.
func findKotlinPackage(root string) string {
	var rec func(string, int) string
	rec = func(p string, depth int) string {
		if depth > 8 {
			return ""
		}
		entries, err := os.ReadDir(p)
		if err != nil {
			return ""
		}
		var dirs []string
		hasKt := false
		for _, e := range entries {
			if e.IsDir() {
				if strings.HasPrefix(e.Name(), ".") {
					continue
				}
				dirs = append(dirs, filepath.Join(p, e.Name()))
			} else if strings.EqualFold(filepath.Ext(e.Name()), "kt") {
				if !strings.HasPrefix(e.Name(), ".") {
					hasKt = true
				}
			}
		}
		if hasKt {
			return p
		}
		for _, d := range dirs {
			if r := rec(d, depth+1); r != "" {
				return r
			}
		}
		return ""
	}
	return rec(root, 0)
}

// packageOfDir reconstruye el nombre de paquete a partir de la ruta relativa.
func packageOfDir(root, dir string) string {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return ""
	}
	parts := strings.Split(rel, string(filepath.Separator))
	for i, p := range parts {
		parts[i] = p
	}
	return strings.Join(parts, ".")
}

// movePackage mueve el paquete viejo al nuevo y reescribe los `package`.
func movePackage(oldDir, newDir, oldPkg, newPkg string) error {
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(oldDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		src := filepath.Join(oldDir, e.Name())
		dst := filepath.Join(newDir, e.Name())
		if strings.HasSuffix(e.Name(), ".kt") {
			data, err := os.ReadFile(src)
			if err != nil {
				return err
			}
			if err := os.WriteFile(dst, []byte(strings.ReplaceAll(string(data), oldPkg, newPkg)), 0o644); err != nil {
				return err
			}
		} else {
			if err := copyFile(src, dst); err != nil {
				return err
			}
		}
	}
	return nil
}

// ensurePackageConsistent comprueba que el paquete Java coincida con el
// identifier y ofrece repararlo.
func tauriEnsurePackageConsistent(ctx *project.ProjectContext) error {
	identifier := readTauriIdentifier(ctx)
	if identifier == "" {
		return nil
	}
	pkgPath := strings.ReplaceAll(identifier, ".", "/")
	expectedDir := filepath.Join(ctx.ScriptDir, "src-tauri", "gen", "android", "app", "src", "main", "java", pkgPath)
	if dirExists(expectedDir) {
		return nil
	}
	fmt.Println("\n[WARN] El directorio del paquete Android no coincide con el identifier de tauri.conf.json.")
	fmt.Printf("       Paquete esperado: %s\n", identifier)
	fmt.Printf("       Directorio faltante: %s\n", expectedDir)

	javaRoot := filepath.Join(ctx.ScriptDir, "src-tauri", "gen", "android", "app", "src", "main", "java")
	oldLeaf := findKotlinPackage(javaRoot)
	if oldLeaf == "" {
		fmt.Println("\nNo se encontró ningún paquete Java anterior bajo:", javaRoot)
		ans, err := prompts.Select("¿Qué hacer?",
			[]string{"Ejecutar 'tauri android init' (genera proyecto default)", "Saltar (el build puede fallar)"})
		if err == nil && ans == 0 {
			if err := tauriAndroidInit(ctx); err != nil {
				fmt.Printf("[ERROR] %v\n", err)
			} else {
				fmt.Println("[OK] Proyecto Android regenerado.")
			}
		}
		ui.Pause()
		return nil
	}
	oldPackage := packageOfDir(javaRoot, oldLeaf)
	fmt.Printf("\nSe detectó el paquete anterior: %s\n", oldPackage)
	ans, err := prompts.Select("Opciones",
		[]string{fmt.Sprintf("Mover '%s' a '%s' y reescribir los 'package'", oldPackage, identifier),
			"Ejecutar 'tauri android init' (sobreescribe archivos default)",
			"Saltar (el build puede fallar)"})
	if err == nil {
		switch ans {
		case 0:
			if err := movePackage(oldLeaf, expectedDir, oldPackage, identifier); err != nil {
				fmt.Printf("[ERROR] %v\n", err)
			} else {
				fmt.Printf("[OK] Archivos movidos de '%s' a '%s'\n", oldPackage, identifier)
				fmt.Println("[INFO] Puedes eliminar el directorio viejo manualmente si lo deseas.")
			}
		case 1:
			if err := tauriAndroidInit(ctx); err != nil {
				fmt.Printf("[ERROR] %v\n", err)
			} else {
				fmt.Println("[OK] Proyecto Android regenerado.")
			}
		}
	}
	ui.Pause()
	return nil
}

// ensureRustTargets instala los targets de Rust para Android si faltan.
func ensureRustTargets() error {
	out, err := exec.Command("rustup", "target", "list", "--installed").CombinedOutput()
	if err != nil {
		return fmt.Errorf("no se pudo ejecutar rustup: %w", err)
	}
	installed := string(out)
	required := []string{"aarch64-linux-android", "armv7-linux-androideabi", "i686-linux-android", "x86_64-linux-android"}
	var missing []string
	for _, t := range required {
		if !strings.Contains(installed, t) {
			missing = append(missing, t)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	fmt.Println("[INFO] Instalando targets de Rust para Android:", strings.Join(missing, ", "))
	if err := shell.RunStatus("rustup", append([]string{"target", "add"}, missing...)...); err != nil {
		return err
	}
	fmt.Print("[OK] Targets de Rust instalados.\n\n")
	return nil
}

// acceptLicenses acepta las licencias del SDK alimentando 'y' por stdin.
func acceptLicenses(sdkManager string) error {
	cmd := exec.Command(sdkManager, "--licenses")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	for i := 0; i < 30; i++ {
		_, _ = stdin.Write([]byte("y\n"))
	}
	_ = stdin.Close()
	return cmd.Wait()
}

// ensureNdk asegura la NDK (desde ctx.NdkHome o instalándola con sdkmanager).
func ensureNdk(ctx *project.ProjectContext) error {
	if ctx.NdkHome != "" && fileExists(filepath.Join(ctx.NdkHome, "source.properties")) {
		os.Setenv("ANDROID_NDK_HOME", ctx.NdkHome)
		return nil
	}
	if ctx.SdkManager == "" {
		return fmt.Errorf("[ERROR] No se encontró sdkmanager. Verifica ANDROID_HOME.")
	}
	fmt.Printf("[INFO] Instalando NDK %s...\n", tauriNdkVersion)
	if err := acceptLicenses(ctx.SdkManager); err != nil {
		return err
	}
	if err := shell.RunStatus(ctx.SdkManager, "ndk;"+tauriNdkVersion); err != nil {
		return fmt.Errorf("No se pudo instalar el NDK %s: %w", tauriNdkVersion, err)
	}
	ndkPath := filepath.Join(ctx.AndroidHome, "ndk", tauriNdkVersion)
	os.Setenv("ANDROID_NDK_HOME", ndkPath)
	fmt.Print("[OK] NDK instalado.\n\n")
	return nil
}

// cleanupBuildLocks elimina locks de build y detiene daemons de Gradle.
func cleanupBuildLocks(ctx *project.ProjectContext) {
	cargoLock := filepath.Join(ctx.ScriptDir, "src-tauri", "target", ".cargo-lock")
	_ = os.Remove(cargoLock)
	gradleCache := filepath.Join(ctx.ScriptDir, "src-tauri", "gen", "android", "app", ".gradle")
	_ = shell.RemoveDir(gradleCache)
	_ = exec.Command("taskkill", "/F", "/IM", "java.exe", "/T").Run()
}

// prepareAndroidDev reúne las precondiciones de dev/debug en Android.
func prepareAndroidDev(ctx *project.ProjectContext) error {
	if err := tauriEnsure(ctx); err != nil {
		return err
	}
	if err := ctx.AssertAndroidSdk(); err != nil {
		return err
	}
	if err := tauriEnsureAndroidProject(ctx); err != nil {
		return err
	}
	if err := tauriEnsurePackageConsistent(ctx); err != nil {
		return err
	}
	// arm64 por defecto (el dev server de Tauri compila así).
	os.Setenv("ORG_GRADLE_PROJECT_abiList", "arm64-v8a")
	os.Setenv("ORG_GRADLE_PROJECT_archList", "arm64")
	os.Setenv("ORG_GRADLE_PROJECT_targetList", "aarch64")
	cleanupBuildLocks(ctx)
	if err := ensureNdk(ctx); err != nil {
		return err
	}
	if err := ensureRustTargets(); err != nil {
		return err
	}
	return nil
}

// prepareRelease reúne las precondiciones de build release.
func prepareRelease(ctx *project.ProjectContext) error {
	if err := tauriEnsure(ctx); err != nil {
		return err
	}
	if err := ctx.AssertAndroidSdk(); err != nil {
		return err
	}
	if err := tauriEnsureAndroidProject(ctx); err != nil {
		return err
	}
	return tauriEnsurePackageConsistent(ctx)
}

// ───────────────────────────────────────────────────────────────────────────
// Detección del lanzamiento dev
// ───────────────────────────────────────────────────────────────────────────

// declaresNpmDevAndroid indica si package.json declara `dev:android`.
func declaresNpmDevAndroid(ctx *project.ProjectContext) bool {
	data, err := os.ReadFile(filepath.Join(ctx.ScriptDir, "package.json"))
	if err != nil {
		return false
	}
	// parseo tolerante: busca "dev:android"
	idx := strings.Index(string(data), npmDevAndroidScript)
	if idx < 0 {
		return false
	}
	// que esté dentro de "scripts"
	return strings.Contains(string(data), "\"scripts\"")
}

// devCommand devuelve el comando de arranque dev (npm script o CLI de Tauri).
func tauriDevCommand(ctx *project.ProjectContext) string {
	if declaresNpmDevAndroid(ctx) {
		return "npm.cmd run " + npmDevAndroidScript
	}
	return tauriCommand(ctx, "android dev")
}

// ───────────────────────────────────────────────────────────────────────────
// Builds de Android
// ───────────────────────────────────────────────────────────────────────────

func archEnvs(target string) map[string]string {
	switch target {
	case "aarch64":
		return map[string]string{
			"ORG_GRADLE_PROJECT_abiList":    "arm64-v8a",
			"ORG_GRADLE_PROJECT_archList":   "arm",
			"ORG_GRADLE_PROJECT_targetList": "aarch64",
		}
	case "armv7":
		return map[string]string{
			"ORG_GRADLE_PROJECT_abiList":    "armeabi-v7a",
			"ORG_GRADLE_PROJECT_archList":   "arm",
			"ORG_GRADLE_PROJECT_targetList": "armv7",
		}
	}
	return nil
}

// apkOutputDir del módulo Android Tauri.
func tauriApkOutputDir(ctx *project.ProjectContext) string {
	return filepath.Join(ctx.ScriptDir, "src-tauri", "gen", "android", "app", "build", "outputs", "apk")
}

// apkArtifactBase construye el nombre del APK con productName/version.
func tauriApkArtifactBase(ctx *project.ProjectContext, label string) string {
	name, version := readTauriMeta(ctx)
	if name == "" {
		name = sanitizeAppName(filepath.Base(ctx.ScriptDir))
	}
	switch {
	case version == "" && label == "":
		return name
	case version == "":
		return fmt.Sprintf("%s-%s", name, label)
	case label == "":
		return fmt.Sprintf("%s-v%s", name, version)
	default:
		return fmt.Sprintf("%s-v%s-%s", name, version, label)
	}
}

// tauriNewestApk busca el APK más reciente en la carpeta de salida.
func tauriNewestApk(ctx *project.ProjectContext) (string, error) {
	dir := tauriApkOutputDir(ctx)
	best, ok := wailsNewestMatching(dir, "apk", "")
	if !ok {
		return "", fmt.Errorf("[ERROR] El build terminó bien pero no se encontró la APK en: %s", dir)
	}
	return best, nil
}

// buildArch compila la APK release de una ABI.
func tauriBuildArch(ctx *project.ProjectContext, target, label string) (string, error) {
	if err := prepareRelease(ctx); err != nil {
		return "", err
	}
	command := tauriCommand(ctx, fmt.Sprintf("android build --target %s --apk", target))
	fmt.Println()
	fmt.Printf("[INFO] Compilando Android APK %s (Release)...\n", label)
	fmt.Printf("[INFO] Usando: %s\n\n", command)
	ok, err := shell.RunShellStatus(command, ctx.ScriptDir, archEnvs(target))
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("[ERROR] Falló el build Android %s (`tauri android build --target %s`).", label, target)
	}
	apk, err := tauriNewestApk(ctx)
	if err != nil {
		return "", err
	}
	base := tauriApkArtifactBase(ctx, label)
	if renamed := shell.RenameArtifact(apk, base); renamed != "" {
		return renamed, nil
	}
	return "", fmt.Errorf("[ERROR] No se pudo renombrar %s", apk)
}

// buildUniversal compila la APK universal.
func tauriBuildUniversal(ctx *project.ProjectContext) (string, error) {
	if err := prepareRelease(ctx); err != nil {
		return "", err
	}
	command := tauriCommand(ctx, "android build --apk")
	fmt.Println()
	fmt.Println("[INFO] Compilando Android APK universal (Release)...")
	fmt.Printf("[INFO] Usando: %s\n\n", command)
	ok, err := shell.RunShellStatus(command, ctx.ScriptDir, nil)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("[ERROR] Falló el build Android universal (`tauri android build --apk`).")
	}
	apk, err := tauriNewestApk(ctx)
	if err != nil {
		return "", err
	}
	base := tauriApkArtifactBase(ctx, "")
	if renamed := shell.RenameArtifact(apk, base); renamed != "" {
		return renamed, nil
	}
	return "", fmt.Errorf("[ERROR] No se pudo renombrar %s", apk)
}

// buildAabArtifact compila el AAB release.
func tauriBuildAabArtifact(ctx *project.ProjectContext) (string, error) {
	if err := prepareRelease(ctx); err != nil {
		return "", err
	}
	command := tauriCommand(ctx, "android build --aab")
	fmt.Println()
	fmt.Println("[INFO] Compilando Android AAB (Release)...")
	fmt.Printf("[INFO] Usando: %s\n\n", command)
	ok, err := shell.RunShellStatus(command, ctx.ScriptDir, nil)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("[ERROR] Falló el build Android AAB (`tauri android build --aab`).")
	}
	dir := filepath.Join(ctx.ScriptDir, "src-tauri", "gen", "android", "app", "build", "outputs", "bundle")
	best, ok := wailsNewestMatching(dir, "aab", "")
	if !ok {
		return "", fmt.Errorf("[ERROR] El build terminó bien pero no se encontró el AAB en: %s", dir)
	}
	base := tauriApkArtifactBase(ctx, "")
	if renamed := shell.RenameArtifact(best, base); renamed != "" {
		return renamed, nil
	}
	return "", fmt.Errorf("[ERROR] No se pudo renombrar %s", best)
}

// TauriBuildArm64: APK release arm64.
func TauriBuildArm64(ctx *project.ProjectContext) error {
	apk, err := tauriBuildArch(ctx, "aarch64", "arm64")
	if err != nil {
		fmt.Println()
		fmt.Println(err)
		fmt.Println()
		ui.Pause()
		return err
	}
	shell.ShowArtifact(apk)
	ui.PauseFor(10)
	return nil
}

// TauriBuildArmv7: APK release armv7.
func TauriBuildArmv7(ctx *project.ProjectContext) error {
	apk, err := tauriBuildArch(ctx, "armv7", "armv7")
	if err != nil {
		fmt.Println()
		fmt.Println(err)
		fmt.Println()
		ui.Pause()
		return err
	}
	shell.ShowArtifact(apk)
	ui.PauseFor(10)
	return nil
}

// TauriBuildAll: APK universal.
func TauriBuildAll(ctx *project.ProjectContext) error {
	apk, err := tauriBuildUniversal(ctx)
	if err != nil {
		fmt.Println()
		fmt.Println(err)
		fmt.Println()
		ui.Pause()
		return err
	}
	shell.ShowArtifact(apk)
	ui.PauseFor(10)
	return nil
}

// TauriBuildAab: AAB release.
func TauriBuildAab(ctx *project.ProjectContext) error {
	aab, err := tauriBuildAabArtifact(ctx)
	if err != nil {
		fmt.Println()
		fmt.Println(err)
		fmt.Println()
		ui.Pause()
		return err
	}
	shell.ShowArtifact(aab)
	ui.PauseFor(10)
	return nil
}

// ───────────────────────────────────────────────────────────────────────────
// Dev y Debug
// ───────────────────────────────────────────────────────────────────────────

// TauriAndroidDev arranca el desarrollo Android en una ventana nueva.
func TauriAndroidDev(ctx *project.ProjectContext) error {
	if err := prepareAndroidDev(ctx); err != nil {
		return err
	}
	_, err := selectAndroidDevice(ctx)
	if err != nil {
		return err
	}
	ui.InfoBanner("Iniciando desarrollo Android")
	fmt.Println("  Tauri se conectará automáticamente al dispositivo disponible.")
	fmt.Println()
	launch := tauriDevCommand(ctx)
	desc := "CLI de Tauri (package.json no declara `dev:android`)"
	if declaresNpmDevAndroid(ctx) {
		desc = "script `dev:android` de package.json"
	}
	fmt.Printf("[INFO] Arrancando con el %s.\n", desc)
	return shell.SpawnNewWindowInteractive("Tauri Android Dev", ctx.ScriptDir, launch)
}

// TauriDebugAndroid abre el depurador de Chrome (edge://inspect) para Android.
func TauriDebugAndroid(ctx *project.ProjectContext) error {
	shell.ClearScreen()
	ui.InfoBanner("Depurador Chrome (Android)")

	// Preparar el entorno ANTES de tocar adb.
	if err := prepareAndroidDev(ctx); err != nil {
		return err
	}

	adb := ctx.AdbPath
	if adb == "" {
		adb = "adb"
	}

	fmt.Println("[1/5] Reiniciando servidor ADB...")
	_, _ = exec.Command(adb, "kill-server").Output()
	time.Sleep(1 * time.Second)
	if _, err := exec.Command(adb, "start-server").Output(); err != nil {
		return fmt.Errorf("no se pudo arrancar adb: %w", err)
	}
	time.Sleep(2 * time.Second)

	fmt.Println("[2/5] Buscando dispositivos...")
	out, _ := exec.Command(adb, "devices", "-l").CombinedOutput()
	stdout := string(out)
	fmt.Println(stdout)

	scanner := bufio.NewScanner(strings.NewReader(stdout))
	hasDevice := false
	scanner.Scan() // saltar cabecera
	for scanner.Scan() {
		t := strings.TrimSpace(scanner.Text())
		if t != "" && strings.Contains(t, "device") && !strings.Contains(t, "offline") {
			hasDevice = true
			break
		}
	}

	if !hasDevice {
		fmt.Println("\n[WARNING] Ningún dispositivo Android detectado.")
		fmt.Println("  Conecta el teléfono y desbloquéalo, con la depuración USB activada")
		fmt.Println("  y la huella RSA aceptada. Verifica con: adb devices -l")
		fmt.Print("  Luego abre chrome://inspect/#devices manualmente.\n\n")
	} else {
		fmt.Println("[3/5] Abriendo puerto de depuración...")
		for _, line := range strings.Split(stdout, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.Contains(trimmed, "offline") || !strings.Contains(trimmed, "device") {
				continue
			}
			fields := strings.Fields(trimmed)
			serial := fields[0]
			_, _ = exec.Command(adb, "-s", serial, "forward", "tcp:9222", "localabstract:chrome_devtools_remote").Output()
			_, _ = exec.Command(adb, "-s", serial, "forward", "tcp:9229", "localabstract:firefox_devtools_remote").Output()
		}
	}

	fmt.Println("[4/5] Lanzando la app en modo debug...")
	launch := tauriDevCommand(ctx)
	if err := shell.SpawnNewWindowInteractive("Tauri Android Dev", ctx.ScriptDir, launch); err != nil {
		fmt.Printf("[WARN] No se pudo lanzar la ventana de dev: %v\n", err)
	}
	fmt.Println()

	fmt.Println("[5/5] Abriendo Chrome DevTools...")
	url := "edge://inspect/#devices"
	fmt.Println()
	fmt.Println("[INFO] Abriendo edge://inspect/#devices ...")
	ps := fmt.Sprintf("Set-Clipboard -Value %s; Start-Process -FilePath msedge -ArgumentList %s", shell.PsQuote(url), shell.PsQuote(url))
	_, _ = shell.RunPS(ps)

	if hasDevice {
		fmt.Println("[INFO] Si no ves el dispositivo en Edge, verifica:")
		fmt.Println("  1. En edge://inspect, haz clic en 'Discover USB devices'")
		fmt.Println("  2. O usa: adb forward tcp:9222 localabstract:chrome_devtools_remote")
		fmt.Println("  3. Abre http://localhost:9222/json en Edge")
	}

	ui.Pause()
	return nil
}
