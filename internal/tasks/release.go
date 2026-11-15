package tasks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"compass/internal/project"
	"compass/internal/shell"
	"compass/internal/theme"
	"compass/internal/tui"
	"compass/internal/ui"
)

func init() {
	for _, pt := range allTypes() {
		Register("windows-release", pt, ReleaseWindows)
		Register("android-release", pt, ReleaseAndroid)
	}
}

const (
	releaseTitleWindows = "RELEASE · WINDOWS"
	releaseTitleAndroid = "RELEASE · ANDROID"

	iconDesktop   = "\U0001f5a5"
	iconAndroid   = "\U0001f4f1"
	iconUniversal = "\U0001f4e6"
	iconAll       = "\u2705"
)

// Claves de arquitectura compartidas por ambos submenús.
const (
	archKeyX86       = "x86"
	archKeyX64       = "x64"
	archKeyArm64     = "arm64"
	archKeyArmv7     = "armv7"
	archKeyUniversal = "universal"
	archKeyAll       = "all"
)

// revealMode decide qué se resalta al terminar un lote.
type revealMode int

const (
	revealLastArtifact revealMode = iota
	revealBinFolder
)

func windowsSupported(pt project.ProjectType) bool {
	switch pt {
	case project.TypeTauri, project.TypeRust, project.TypeGo, project.TypeGoWails:
		return true
	}
	return false
}

func androidSupported(pt project.ProjectType) bool {
	switch pt {
	case project.TypeTauri, project.TypeFlutter, project.TypeKotlin, project.TypeGoWails:
		return true
	}
	return false
}

// windowsTriple devuelve el triple de Rust para una arquitectura de Windows.
func windowsTriple(key string) (string, bool) {
	switch key {
	case archKeyX86:
		return "i686-pc-windows-msvc", true
	case archKeyX64:
		return "x86_64-pc-windows-msvc", true
	case archKeyArm64:
		return "aarch64-pc-windows-msvc", true
	}
	return "", false
}

// windowsLabel es el sufijo del artefacto (x64 es la entrega normal: sin sufijo).
func windowsLabel(key string) string {
	switch key {
	case archKeyX86:
		return "x86"
	case archKeyArm64:
		return "arm64"
	default:
		return ""
	}
}

func goArchOf(key string) (GoArch, bool) {
	switch key {
	case archKeyX86:
		return GoX86, true
	case archKeyX64:
		return GoX64, true
	case archKeyArm64:
		return GoArm64, true
	}
	return 0, false
}

func desktopArchOf(key string) (desktopArch, bool) {
	switch key {
	case archKeyX86:
		return archX86, true
	case archKeyX64:
		return archX64, true
	case archKeyArm64:
		return archArm64, true
	}
	return 0, false
}

// ── Entradas de los submenús ────────────────────────────────────────────────

type releaseEntry struct {
	key  string
	card tui.SubItem
}

// windowsEntries son las cards del release de escritorio.
func windowsEntries(ctx *project.ProjectContext) []releaseEntry {
	if !windowsSupported(ctx.ProjectType) {
		return nil
	}
	return []releaseEntry{
		{archKeyX64, tui.SubItem{
			Icon: iconDesktop, Label: "Windows x64",
			Description: "64 bits · uso general", ColorKey: "run",
		}},
		{archKeyAll, tui.SubItem{
			Icon: iconAll, Label: "Todos",
			Description: "Compila x86 + x64 + ARM64", ColorKey: "test",
		}},
		{archKeyX86, tui.SubItem{
			Icon: iconDesktop, Label: "Windows x86",
			Description: "32 bits · PCs y VMs antiguas", ColorKey: "build",
		}},
		{archKeyArm64, tui.SubItem{
			Icon: iconDesktop, Label: "Windows ARM64",
			Description: "Snapdragon X · bajo consumo", ColorKey: "install",
		}},
	}
}

// androidEntries son las cards del release de Android.
//
// Kotlin sin jniLibs no tiene APK por ABI: las tres serían idénticas, así que
// sólo se ofrece la universal.
func androidEntries(ctx *project.ProjectContext) []releaseEntry {
	if !androidSupported(ctx.ProjectType) {
		return nil
	}

	if ctx.ProjectType == project.TypeKotlin && !hasNativeLibs(kotlinAndroidDir(ctx)) {
		return []releaseEntry{
			{archKeyUniversal, tui.SubItem{
				Icon: iconUniversal, Label: "Android universal",
				Description: "Sin libs nativas · sirve para todas las ABIs", ColorKey: "apk",
			}},
		}
	}

	return []releaseEntry{
		{archKeyArm64, tui.SubItem{
			Icon: iconAndroid, Label: "Android arm64",
			Description: "arm64-v8a · moviles modernos", ColorKey: "android",
		}},
		{archKeyArmv7, tui.SubItem{
			Icon: iconAndroid, Label: "Android armv7",
			Description: "armeabi-v7a · equipos antiguos", ColorKey: "apk32",
		}},
		{archKeyUniversal, tui.SubItem{
			Icon: iconUniversal, Label: "Android universal",
			Description: "Una APK con todas las ABIs", ColorKey: "apk",
		}},
		{archKeyAll, tui.SubItem{
			Icon: iconAll, Label: "Todos",
			Description: "Compila las tres APK en serie", ColorKey: "aab",
		}},
	}
}

// ── Puntos de entrada ───────────────────────────────────────────────────────

// ReleaseWindows abre el submenú de release de escritorio. Flutter/Kotlin/
// Python delegan en su build de escritorio de siempre (sólo host).
func ReleaseWindows(ctx *project.ProjectContext) error {
	switch ctx.ProjectType {
	case project.TypeTauri, project.TypeRust, project.TypeGo, project.TypeGoWails:
		reveal := revealLastArtifact
		if ctx.ProjectType == project.TypeGoWails {
			reveal = revealBinFolder
		}
		return runReleaseMenu(ctx, releaseTitleWindows, windowsEntries(ctx), reveal,
			func(key string) ([]string, error) { return windowsArtifacts(ctx, key) })
	case project.TypeFlutter:
		return FlutterBuildWindows(ctx)
	case project.TypeKotlin:
		return KotlinBuildWindows(ctx)
	case project.TypePython:
		return PythonBuild(ctx)
	}
	return nil
}

// ReleaseAndroid abre el submenú de release de Android (APK por ABI).
func ReleaseAndroid(ctx *project.ProjectContext) error {
	if !androidSupported(ctx.ProjectType) {
		shell.ClearScreen()
		fmt.Println("[INFO] Los builds de Android no aplican a este tipo de proyecto.")
		ui.Pause()
		return nil
	}

	reveal := revealLastArtifact
	if ctx.ProjectType == project.TypeGoWails {
		reveal = revealBinFolder
	}

	return runReleaseMenu(ctx, releaseTitleAndroid, androidEntries(ctx), reveal,
		func(key string) ([]string, error) { return androidArtifacts(ctx, key) })
}

// runReleaseMenu es el bucle canónico de los submenús: ejecuta la acción con la
// consola restaurada y vuelve al TUI sin cerrar el submenú si el build falla.
func runReleaseMenu(
	ctx *project.ProjectContext,
	title string,
	entries []releaseEntry,
	reveal revealMode,
	exec func(string) ([]string, error),
) error {
	cards := make([]tui.SubEntry[string], len(entries))
	for i, e := range entries {
		cards[i] = tui.SubEntry[string]{Value: e.key, Item: e.card}
	}
	menu := tui.NewSubmenu(title, "DEV", cards)
	th := theme.Load()

	for {
		choice, ok, err := menu.Run(ctx, th)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}

		artifacts, err := exec(choice)
		if err != nil {
			fmt.Println()
			fmt.Println(err)
			fmt.Println()
			ui.Pause()
		} else {
			finishRelease(ctx, artifacts, reveal)
		}

		// `finishRelease` agotó su cuenta atrás: pide cerrar la pestaña, así que
		// el submenú no espera otra elección y la tarea termina.
		if shell.CloseTabRequested() {
			return nil
		}
	}
}

// finishRelease lista lo generado, revela el resultado y espera una sola vez.
func finishRelease(ctx *project.ProjectContext, artifacts []string, reveal revealMode) {
	if len(artifacts) > 0 {
		fmt.Println()
		fmt.Println("[OK] Artefactos generados:")
		for _, a := range artifacts {
			fmt.Printf("  - %s\n", a)
		}
		switch reveal {
		case revealLastArtifact:
			shell.ShowArtifact(artifacts[len(artifacts)-1])
		case revealBinFolder:
			shell.OpenFolder(releaseBinDir(ctx))
		}
	}
	ui.PauseFor(10)
}

func releaseBinDir(ctx *project.ProjectContext) string {
	if ctx.ProjectType == project.TypeGoWails {
		return wailsBinDir(ctx)
	}
	return filepath.Join(ctx.ScriptDir, "bin")
}

// ── Despacho por arquitectura ───────────────────────────────────────────────

func singleArtifact(artifact string, err error) ([]string, error) {
	if err != nil {
		return nil, err
	}
	return []string{artifact}, nil
}

func windowsArtifacts(ctx *project.ProjectContext, key string) ([]string, error) {
	// Etapa el motor de WhatsApp canónico (no-op si el proyecto no lo usa).
	stageWhatsappEngine(ctx)

	if key == archKeyAll {
		var artifacts []string
		for _, a := range []string{archKeyX86, archKeyX64, archKeyArm64} {
			got, err := windowsArtifacts(ctx, a)
			if err != nil {
				return nil, err
			}
			artifacts = append(artifacts, got...)
		}
		return artifacts, nil
	}

	switch ctx.ProjectType {
	case project.TypeTauri:
		triple, ok := windowsTriple(key)
		if !ok {
			return nil, fmt.Errorf("[ERROR] Arquitectura sin triple.")
		}
		return singleArtifact(windowsBuildArch(ctx, triple, windowsLabel(key)))
	case project.TypeRust:
		triple, ok := windowsTriple(key)
		if !ok {
			return nil, fmt.Errorf("[ERROR] Arquitectura sin triple.")
		}
		exe, err := rustBuildArch(ctx, triple, windowsLabel(key))
		if err != nil {
			return nil, err
		}
		name, version := readCargoMeta(ctx)
		return withAppInstaller(ctx, name, version, windowsLabel(key), exe), nil
	case project.TypeGo:
		target, ok := goArchOf(key)
		if !ok {
			return nil, fmt.Errorf("[ERROR] Arquitectura sin GOARCH.")
		}
		exe, err := GoBuildArch(ctx, target)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSuffix(executableName(ctx.ScriptDir), ".exe")
		return withAppInstaller(ctx, name, "", target.label(), exe), nil
	case project.TypeGoWails:
		target, ok := desktopArchOf(key)
		if !ok {
			return nil, fmt.Errorf("[ERROR] Arquitectura sin equivalente.")
		}
		return wailsBuildDesktopArch(ctx, target)
	default:
		return nil, fmt.Errorf("[ERROR] %s no compila escritorio por arquitectura.", ctx.ProjectType.Label())
	}
}

func androidArtifacts(ctx *project.ProjectContext, key string) ([]string, error) {
	if key == archKeyAll {
		// Los lotes nativos hacen una sola pasada; Tauri no tiene splits, así
		// que compila arquitectura a arquitectura.
		switch ctx.ProjectType {
		case project.TypeTauri:
			var artifacts []string
			for _, a := range []string{archKeyArm64, archKeyArmv7, archKeyUniversal} {
				got, err := androidArtifacts(ctx, a)
				if err != nil {
					return nil, err
				}
				artifacts = append(artifacts, got...)
			}
			return artifacts, nil
		case project.TypeFlutter:
			return flutterBuildApksAll(ctx)
		case project.TypeKotlin:
			if hasNativeLibs(kotlinAndroidDir(ctx)) {
				return kotlinBuildApksAll(ctx)
			}
			return singleArtifact(kotlinBuildApkUniversal(ctx))
		case project.TypeGoWails:
			apks, err := wailsBuildReleaseApks(ctx)
			if err != nil {
				return nil, err
			}
			if len(apks) > 0 {
				installApkIfDevice(ctx, apks[len(apks)-1], wailsApplicationID(ctx), wailsMainActivity(ctx))
			}
			return apks, nil
		default:
			return nil, fmt.Errorf("[ERROR] %s no compila APK.", ctx.ProjectType.Label())
		}
	}

	switch ctx.ProjectType {
	case project.TypeTauri:
		switch key {
		case archKeyArm64:
			return singleArtifact(tauriBuildArch(ctx, "aarch64", "arm64"))
		case archKeyArmv7:
			return singleArtifact(tauriBuildArch(ctx, "armv7", "armv7"))
		case archKeyUniversal:
			return singleArtifact(tauriBuildUniversal(ctx))
		}
	case project.TypeFlutter:
		switch key {
		case archKeyArm64:
			return singleArtifact(flutterBuildApkArch(ctx, "android-arm64", "arm64"))
		case archKeyArmv7:
			return singleArtifact(flutterBuildApkArch(ctx, "android-arm", "armv7"))
		case archKeyUniversal:
			return singleArtifact(flutterBuildApkUniversal(ctx))
		}
	case project.TypeKotlin:
		switch key {
		case archKeyArm64:
			return singleArtifact(kotlinBuildApkArch(ctx, "arm64-v8a", "arm64"))
		case archKeyArmv7:
			return singleArtifact(kotlinBuildApkArch(ctx, "armeabi-v7a", "armv7"))
		case archKeyUniversal:
			return singleArtifact(kotlinBuildApkUniversal(ctx))
		}
	case project.TypeGoWails:
		switch key {
		case archKeyArm64:
			return singleArtifact(wailsBuildApkArch(ctx, abiArm64))
		case archKeyArmv7:
			return singleArtifact(wailsBuildApkArch(ctx, abiArmv7))
		case archKeyUniversal:
			apk, err := wailsBuildApkUniversal(ctx)
			if err != nil {
				return nil, err
			}
			// El universal instala en cualquier teléfono (arm64 o armv7).
			installApkIfDevice(ctx, apk, wailsApplicationID(ctx), wailsMainActivity(ctx))
			return []string{apk}, nil
		}
	}

	return nil, fmt.Errorf("[ERROR] %s no compila APK %s.", ctx.ProjectType.Label(), key)
}

// ── Builds por arquitectura que faltaban en los módulos ─────────────────────

// artifactBase compone "<name>[ v<version>][ <label>]".
func artifactBase(name, version, label string) string {
	base := name
	if version != "" {
		base += " v" + version
	}
	if label != "" {
		base += " " + label
	}
	return base
}

// rustBuildArch compila un release de Rust para una arquitectura de Windows.
//
// El artefacto queda en target/<triple>/release y se renombra a
// "<name> v<version> [label].exe"; label vacío equivale a x64.
func rustBuildArch(ctx *project.ProjectContext, triple, label string) (string, error) {
	_, _ = shell.RunProgram("rustup", []string{"target", "add", triple}, ctx.ScriptDir, nil)

	display := label
	if display == "" {
		display = "x64"
	}
	fmt.Printf("\n[INFO] Compilando Rust para %s (%s)...\n\n", display, triple)
	command := fmt.Sprintf("cargo build --release --target %s", triple)
	ok, err := shell.RunShellStatus(command, ctx.ScriptDir, nil)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("[ERROR] `%s` fallo. En ARM64 suele faltar el enlazador de Visual Studio para ARM64.", command)
	}

	dir := filepath.Join(ctx.ScriptDir, "target", triple, "release")
	artifact := shell.NewestFileIn(dir, []string{"exe"})
	if artifact == "" {
		return "", fmt.Errorf("[ERROR] No se encontro el .exe en %s", dir)
	}
	name, version := readCargoMeta(ctx)
	return shell.RenameArtifact(artifact, artifactBase(name, version, label)), nil
}

// windowsBuildArch compila un release de Tauri para una arquitectura de Windows.
//
// Si NSIS no genera instalador (x86 o falta makensis) se entrega el .exe
// portable, que ya está compilado.
func windowsBuildArch(ctx *project.ProjectContext, triple, label string) (string, error) {
	if err := tauriEnsure(ctx); err != nil {
		return "", err
	}
	_, _ = shell.RunProgram("rustup", []string{"target", "add", triple}, ctx.ScriptDir, nil)

	display := label
	if display == "" {
		display = "x64"
	}
	fmt.Printf("\n[INFO] Compilando escritorio Tauri para %s (%s)...\n\n", display, triple)
	command := tauriCommand(ctx, fmt.Sprintf("build --target %s --bundles nsis", triple))
	ok, _ := shell.RunShellStatus(command, ctx.ScriptDir, nil)

	name, version := readTauriMeta(ctx)
	if name == "" {
		name = sanitizeAppName(filepath.Base(ctx.ScriptDir))
	}
	base := artifactBase(name, version, label)
	targetDir := filepath.Join(ctx.ScriptDir, "src-tauri", "target", triple, "release")

	if ok {
		bundle := filepath.Join(targetDir, "bundle", "nsis")
		if p := shell.NewestFileIn(bundle, []string{"exe", "msi"}); p != "" {
			return shell.RenameArtifact(p, base), nil
		}
	}

	if portable := shell.NewestFileIn(targetDir, []string{"exe"}); portable != "" {
		fmt.Println()
		fmt.Println("[WARN] No se genero el instalador NSIS; se entrega el ejecutable portable.")
		target := filepath.Join(targetDir, base+".exe")
		if err := copyFile(portable, target); err != nil {
			return "", err
		}
		return target, nil
	}

	return "", fmt.Errorf("[ERROR] Fallo el build de Tauri para %s (%s). En ARM64 suele faltar el enlazador de Visual Studio para ARM64.", label, triple)
}

// runFlutterApk ejecuta `flutter build apk --release <args>`.
func runFlutterApk(ctx *project.ProjectContext, args string) error {
	command := strings.TrimSpace("flutter build apk --release " + args)
	fmt.Printf("\n[INFO] Ejecutando: %s\n\n", command)
	ok, err := shell.RunShellStatus(command, ctx.ScriptDir, nil)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("[ERROR] `%s` fallo.", command)
	}
	return nil
}

// flutterApkFromOutput resuelve la APK esperada o la más reciente de la carpeta.
func flutterApkFromOutput(ctx *project.ProjectContext, file string) (string, error) {
	dir := flutterApkOutputDir(ctx)
	path := filepath.Join(dir, file)
	if fileExists(path) {
		return path, nil
	}
	best := shell.NewestArtifact(dir, []string{"apk"})
	if best == "" {
		return "", fmt.Errorf("[ERROR] No se encontro la APK en %s", dir)
	}
	return best, nil
}

// flutterBuildApkArch compila el APK release de una arquitectura.
func flutterBuildApkArch(ctx *project.ProjectContext, platform, label string) (string, error) {
	if err := ensurePubget(ctx); err != nil {
		return "", err
	}
	if err := runFlutterApk(ctx, "--target-platform "+platform); err != nil {
		return "", err
	}
	apk, err := flutterApkFromOutput(ctx, "app-release.apk")
	if err != nil {
		return "", err
	}
	return copyApkToBin(ctx, apk, flutterApkBase(ctx, label))
}

// flutterBuildApkUniversal compila una APK con todas las ABIs.
func flutterBuildApkUniversal(ctx *project.ProjectContext) (string, error) {
	if err := ensurePubget(ctx); err != nil {
		return "", err
	}
	if err := runFlutterApk(ctx, ""); err != nil {
		return "", err
	}
	apk, err := flutterApkFromOutput(ctx, "app-release.apk")
	if err != nil {
		return "", err
	}
	return copyApkToBin(ctx, apk, flutterApkBase(ctx, ""))
}

// flutterBuildApksAll genera tres APK: universal primero (se copia **antes** de
// los splits, porque el build siguiente reescribe `app-release.apk`) y luego
// arm64/armv7.
func flutterBuildApksAll(ctx *project.ProjectContext) ([]string, error) {
	if err := ensurePubget(ctx); err != nil {
		return nil, err
	}

	var artifacts []string

	if err := runFlutterApk(ctx, ""); err != nil {
		return nil, err
	}
	universal, err := flutterApkFromOutput(ctx, "app-release.apk")
	if err != nil {
		return nil, err
	}
	copied, err := copyApkToBin(ctx, universal, flutterApkBase(ctx, ""))
	if err != nil {
		return nil, err
	}
	artifacts = append(artifacts, copied)

	if err := runFlutterApk(ctx, "--split-per-abi"); err != nil {
		return nil, err
	}
	for _, split := range []struct{ file, label string }{
		{"app-arm64-v8a-release.apk", "arm64"},
		{"app-armeabi-v7a-release.apk", "armv7"},
	} {
		apk, err := flutterApkFromOutput(ctx, split.file)
		if err != nil {
			return nil, err
		}
		copied, err := copyApkToBin(ctx, apk, flutterApkBase(ctx, split.label))
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, copied)
	}

	return artifacts, nil
}

// hasNativeLibs indica si el módulo Kotlin trae librerías nativas (.so).
func hasNativeLibs(moduleDir string) bool {
	var walk func(dir string) bool
	walk = func(dir string) bool {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return false
		}
		for _, e := range entries {
			p := filepath.Join(dir, e.Name())
			if e.IsDir() {
				if walk(p) {
					return true
				}
			} else if strings.EqualFold(filepath.Ext(p), ".so") {
				return true
			}
		}
		return false
	}
	return walk(filepath.Join(moduleDir, "src", "main", "jniLibs"))
}

// writeAbiInitScript fija la ABI del build antes de que AGP lea el DSL.
//
// El `afterEvaluate` se registra desde `beforeProject` para quedar delante del
// de AGP; con `projectsEvaluated` ya sería tarde. abi vacío = splits.
func writeAbiInitScript(abi string) (string, error) {
	var b strings.Builder
	b.WriteString("// Generado por Compass: ABIs del build release.\n")
	b.WriteString("gradle.beforeProject { p ->\n")
	b.WriteString("    p.afterEvaluate {\n")
	b.WriteString("        def android = p.extensions.findByName('android')\n")
	b.WriteString("        if (android == null) { return }\n")
	if abi != "" {
		b.WriteString("        android.splits.abi.enable = false\n")
		b.WriteString("        android.defaultConfig.ndk.abiFilters.clear()\n")
		fmt.Fprintf(&b, "        android.defaultConfig.ndk.abiFilters.add('%s')\n", abi)
	} else {
		b.WriteString("        android.splits.abi.enable = true\n")
		b.WriteString("        android.splits.abi.reset()\n")
		b.WriteString("        android.splits.abi.include 'arm64-v8a', 'armeabi-v7a'\n")
		b.WriteString("        android.splits.abi.universalApk = true\n")
	}
	b.WriteString("    }\n}\n")

	f, err := os.CreateTemp("", "compass-kotlin-abis-*.gradle")
	if err != nil {
		return "", fmt.Errorf("no se pudo escribir el init script: %w", err)
	}
	path := f.Name()
	if _, err := f.WriteString(b.String()); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return path, nil
}

// kotlinAssembleRelease ejecuta :assembleRelease con el init script opcional.
func kotlinAssembleRelease(ctx *project.ProjectContext, init string) (bool, error) {
	module := gradleModule(ctx)
	var command string
	if init != "" {
		command = gradlewCmd(fmt.Sprintf("--init-script %s %s:assembleRelease", shell.PsQuote(init), module))
	} else {
		command = gradlewCmd(fmt.Sprintf("%s:assembleRelease", module))
	}
	fmt.Println()
	fmt.Println("[INFO] Ejecutando: " + command)
	fmt.Println()
	return shell.RunShellStatus(command, ctx.ScriptDir, kotlinSigningEnv(ctx))
}

// kotlinNewestMatching busca la APK más reciente cuyo nombre contenga fragment.
func kotlinNewestMatching(dir, fragment string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	best := ""
	var bestTime time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".apk") {
			continue
		}
		if !strings.Contains(strings.ToLower(e.Name()), fragment) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if best == "" || info.ModTime().After(bestTime) {
			best = filepath.Join(dir, e.Name())
			bestTime = info.ModTime()
		}
	}
	return best
}

// kotlinBuildApkArch compila el APK release de una ABI.
func kotlinBuildApkArch(ctx *project.ProjectContext, abi, label string) (string, error) {
	fmt.Printf("\n[INFO] Compilando Android APK %s (Release)...\n\n", abi)
	if err := ensureGradle(ctx); err != nil {
		return "", err
	}

	init, err := writeAbiInitScript(abi)
	if err != nil {
		return "", err
	}
	built, err := kotlinAssembleRelease(ctx, init)
	_ = os.Remove(init)
	if err != nil {
		return "", err
	}
	if !built {
		return "", fmt.Errorf("[ERROR] Gradle fallo en `assembleRelease` (%s).", abi)
	}

	dir := kotlinApkOutputDir(ctx)
	src := filepath.Join(dir, "app-release.apk")
	if !fileExists(src) {
		src = shell.NewestArtifact(dir, []string{"apk"})
	}
	if src == "" {
		return "", fmt.Errorf("[ERROR] No se encontro la APK en %s", dir)
	}
	return shell.RenameArtifact(src, kotlinApkBase(ctx, label)), nil
}

// kotlinBuildApkUniversal compila una APK sin splits; se instala si hay teléfono.
func kotlinBuildApkUniversal(ctx *project.ProjectContext) (string, error) {
	fmt.Println("\n[INFO] Compilando Android APK universal (Release)...")
	if err := ensureGradle(ctx); err != nil {
		return "", err
	}
	built, err := kotlinAssembleRelease(ctx, "")
	if err != nil {
		return "", err
	}
	if !built {
		return "", fmt.Errorf("[ERROR] Gradle fallo en `assembleRelease`.")
	}

	dir := kotlinApkOutputDir(ctx)
	src := filepath.Join(dir, "app-release.apk")
	if !fileExists(src) {
		src = shell.NewestArtifact(dir, []string{"apk"})
	}
	if src == "" {
		return "", fmt.Errorf("[ERROR] No se encontro la APK en %s", dir)
	}
	apk := shell.RenameArtifact(src, kotlinApkBase(ctx, ""))
	installReleaseApk(ctx, apk)
	return apk, nil
}

// kotlinBuildApksAll genera arm64 + armv7 + universal en una sola pasada.
func kotlinBuildApksAll(ctx *project.ProjectContext) ([]string, error) {
	fmt.Println("\n[INFO] Compilando Android APK arm64 + armv7 + universal (Release)...")
	if err := ensureGradle(ctx); err != nil {
		return nil, err
	}

	init, err := writeAbiInitScript("")
	if err != nil {
		return nil, err
	}
	built, err := kotlinAssembleRelease(ctx, init)
	_ = os.Remove(init)
	if err != nil {
		return nil, err
	}
	if !built {
		return nil, fmt.Errorf("[ERROR] Gradle fallo en `assembleRelease` (splits).")
	}

	dir := kotlinApkOutputDir(ctx)
	var artifacts []string
	for _, split := range []struct{ fragment, label string }{
		{"arm64-v8a", "arm64"},
		{"armeabi-v7a", "armv7"},
	} {
		src := kotlinNewestMatching(dir, split.fragment)
		if src == "" {
			return nil, fmt.Errorf("[ERROR] Gradle no dejo la APK %s en %s", split.fragment, dir)
		}
		artifacts = append(artifacts, shell.RenameArtifact(src, kotlinApkBase(ctx, split.label)))
	}

	universal := kotlinNewestMatching(dir, "universal")
	if universal == "" {
		return nil, fmt.Errorf("[ERROR] Gradle no dejo la APK universal en %s", dir)
	}
	universal = shell.RenameArtifact(universal, kotlinApkBase(ctx, ""))
	installReleaseApk(ctx, universal)
	artifacts = append(artifacts, universal)

	return artifacts, nil
}
