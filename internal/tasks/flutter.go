package tasks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"compass/internal/project"
	"compass/internal/prompts"
	"compass/internal/shell"
	"compass/internal/ui"
)

func init() {
	Register("windows-dev", project.TypeFlutter, FlutterRunWindows)
	Register("android-dev", project.TypeFlutter, FlutterRunAndroid)
	Register("windows-build", project.TypeFlutter, FlutterBuildWindows)
	Register("android-aab", project.TypeFlutter, FlutterBuildAab)
	Register("android-all", project.TypeFlutter, FlutterBuildAab)
	Register("android-arm64", project.TypeFlutter, FlutterBuildApk)
	Register("clean", project.TypeFlutter, FlutterClean)
	Register("flutter-profile", project.TypeFlutter, flutterProfile)
	Register("flutter-profile-windows", project.TypeFlutter, FlutterProfileWindows)
	Register("flutter-profile-android", project.TypeFlutter, FlutterProfileAndroid)
	Register("debug-android", project.TypeFlutter, flutterDebugAndroid)
}

// flutterAppMeta lee name/version de pubspec.yaml (saneados).
func flutterAppMeta(ctx *project.ProjectContext) (string, string) {
	name, version := readPubspecMeta(ctx)
	if name == "" {
		name = sanitizeAppName(filepath.Base(ctx.ScriptDir))
	}
	return name, version
}

// ensurePubget ejecuta 'flutter pub get' si no existe el directorio .dart_tool.
func ensurePubget(ctx *project.ProjectContext) error {
	if dirExists(filepath.Join(ctx.ScriptDir, ".dart_tool")) {
		return nil
	}
	fmt.Println("[INFO] Ejecutando flutter pub get...")
	if err := shell.RunInDirStatus("flutter", ctx.ScriptDir, "pub", "get"); err != nil {
		return err
	}
	fmt.Println("[OK] flutter pub get completado.")
	return nil
}

// FlutterRunWindows lanza 'flutter run -d windows' en una ventana nueva.
func FlutterRunWindows(ctx *project.ProjectContext) error {
	if err := ensurePubget(ctx); err != nil {
		return err
	}
	ensureWhatsappNpm(ctx)
	return shell.SpawnNewWindowInteractive("Flutter Desktop (Dev)", ctx.ScriptDir, "flutter run -d windows")
}

// FlutterRunAndroid lanza 'flutter run -d <serial>' en una ventana nueva.
func FlutterRunAndroid(ctx *project.ProjectContext) error {
	if err := ensurePubget(ctx); err != nil {
		return err
	}
	serial, err := selectAndroidDevice(ctx)
	if err != nil {
		return err
	}
	return shell.SpawnNewWindowInteractive("Flutter Android (Dev)", ctx.ScriptDir,
		fmt.Sprintf("flutter run -d %s", serial))
}

// FlutterProfileWindows lanza 'flutter run --profile -d windows'.
func FlutterProfileWindows(ctx *project.ProjectContext) error {
	if err := ensurePubget(ctx); err != nil {
		return err
	}
	ensureWhatsappNpm(ctx)
	return shell.SpawnNewWindowInteractive("Flutter Desktop (Profile)", ctx.ScriptDir,
		"flutter run --profile -d windows")
}

// FlutterProfileAndroid lanza 'flutter run --profile -d <serial>'.
func FlutterProfileAndroid(ctx *project.ProjectContext) error {
	if err := ensurePubget(ctx); err != nil {
		return err
	}
	serial, err := selectAndroidDevice(ctx)
	if err != nil {
		return err
	}
	return shell.SpawnNewWindowInteractive("Flutter Android (Profile)", ctx.ScriptDir,
		fmt.Sprintf("flutter run --profile -d %s", serial))
}

// flutterProfile pregunta la plataforma (Windows/Android) y lanza el run en
// modo profile. Es el despacho de la tarea "flutter-profile" (card Flutter).
func flutterProfile(ctx *project.ProjectContext) error {
	fmt.Println("\n[INFO] Selecciona la plataforma para Profile:")
	fmt.Println()
	fmt.Println("  1. Windows")
	fmt.Println("  2. Android")
	option := strings.TrimSpace(ui.AskText("Opción", ""))
	switch option {
	case "1":
		return FlutterProfileWindows(ctx)
	case "2":
		return FlutterProfileAndroid(ctx)
	}
	fmt.Println("[ERROR] Opción no válida.")
	return nil
}

// flutterDebugAndroid: Flutter depura con su propio run (no hay logcat propio).
func flutterDebugAndroid(*project.ProjectContext) error {
	fmt.Println("[INFO] Para depurar Flutter: flutter run --debug")
	return nil
}

// FlutterBuildWindows compila el release de escritorio y revela el artefacto.
func FlutterBuildWindows(ctx *project.ProjectContext) error {
	fmt.Println("\n[INFO] Compilando aplicación de escritorio (Release)...")
	if err := ensurePubget(ctx); err != nil {
		return err
	}
	ensureWhatsappNpm(ctx)

	name, version := flutterAppMeta(ctx)
	rename := ""
	if name != "" {
		if version != "" {
			rename = fmt.Sprintf("%s v%s", name, version)
		} else {
			rename = name
		}
	}
	releaseDir := filepath.Join(ctx.ScriptDir, "build")
	_, err := shell.RunBuildAndReveal("flutter build windows --release", ctx.ScriptDir,
		releaseDir, []string{"exe"}, rename)
	return err
}

// apkOutputDir es la carpeta de salida de APKs de Flutter.
func flutterApkOutputDir(ctx *project.ProjectContext) string {
	return filepath.Join(ctx.ScriptDir, "build", "app", "outputs", "flutter-apk")
}

// apkBase construye el nombre base "<name>-v<ver>[-<suffix>]" del APK.
func flutterApkBase(ctx *project.ProjectContext, suffix string) string {
	name, version := flutterAppMeta(ctx)
	base := name
	if version != "" {
		base = fmt.Sprintf("%s-v%s", name, version)
	}
	if suffix != "" {
		base = fmt.Sprintf("%s-%s", base, suffix)
	}
	return base
}

// copyApkToBin copia (no renombra) el APK a bin/ con el nombre final.
func copyApkToBin(ctx *project.ProjectContext, src, base string) (string, error) {
	bin := filepath.Join(ctx.ScriptDir, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		return "", err
	}
	dest := filepath.Join(bin, base+".apk")
	if fileExists(dest) {
		_ = os.Remove(dest)
	}
	if err := copyFile(src, dest); err != nil {
		return "", fmt.Errorf("no se pudo copiar %s a %s: %w", src, dest, err)
	}
	return dest, nil
}

// FlutterBuildApk compila el APK universal release, lo copia a bin/ y lo muestra.
func FlutterBuildApk(ctx *project.ProjectContext) error {
	fmt.Println("\n[INFO] Compilando Android APK universal (Release)...")
	if err := ensurePubget(ctx); err != nil {
		return err
	}
	command := "flutter build apk --release"
	fmt.Printf("\n[INFO] Ejecutando: %s\n\n", command)
	ok, err := shell.RunShellStatus(command, ctx.ScriptDir, nil)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("[ERROR] `%s` fallo.", command)
	}
	src := filepath.Join(flutterApkOutputDir(ctx), "app-release.apk")
	if !fileExists(src) {
		src = shell.NewestArtifact(flutterApkOutputDir(ctx), []string{"apk"})
	}
	if src == "" {
		return fmt.Errorf("[ERROR] No se encontro la APK en %s", flutterApkOutputDir(ctx))
	}
	dest, err := copyApkToBin(ctx, src, flutterApkBase(ctx, ""))
	if err != nil {
		return err
	}
	shell.ShowArtifact(dest)
	ui.PauseFor(10)
	return nil
}

// FlutterBuildAab compila el AAB release y lo revela.
func FlutterBuildAab(ctx *project.ProjectContext) error {
	fmt.Println("\n[INFO] Compilando Android AAB (Release)...")
	if err := ensurePubget(ctx); err != nil {
		return err
	}
	name, version := flutterAppMeta(ctx)
	rename := ""
	if name != "" {
		if version != "" {
			rename = fmt.Sprintf("%s-v%s", name, version)
		} else {
			rename = name
		}
	}
	aabDir := filepath.Join(ctx.ScriptDir, "build", "app", "outputs", "bundle", "release")
	_, err := shell.RunBuildAndReveal("flutter build appbundle --release", ctx.ScriptDir,
		aabDir, []string{"aab"}, rename)
	return err
}

// FlutterClean elimina los artefactos de build de un proyecto Flutter.
func FlutterClean(ctx *project.ProjectContext) error {
	shell.ClearScreen()
	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Println("║      LIMPIAR PROYECTO FLUTTER                    ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("Se eliminará:")
	fmt.Println("  - build/")
	fmt.Println("  - .dart_tool/")
	fmt.Println("  - .packages, .flutter-plugins*")
	fmt.Println("  - node_modules/")
	fmt.Println("  - dist/")
	fmt.Println("  - android/.gradle/")
	fmt.Println("  - android/build/")
	fmt.Println("  - android/app/build/")
	fmt.Println("  - *.log")
	fmt.Println()
	fmt.Println("Se conservará:")
	fmt.Println("  - lib/ (código fuente Dart)")
	fmt.Println("  - android/ (configuración, key.properties, assets, jniLibs)")
	fmt.Println("  - ios/, web/, test/")
	fmt.Println("  - pubspec.yaml + pubspec.lock")
	fmt.Println("  - whatsapp_engine/ (motor WhatsApp)")
	fmt.Println("  - resources/whatsapp-engine-bundle/ (bundle WhatsApp compilado)")
	fmt.Println("  - Firmas: key.properties, *.keystore, *.jks")
	fmt.Println()

	confirm, err := prompts.Confirm("¿Estás seguro?", false)
	if err != nil {
		return err
	}
	if !confirm {
		fmt.Println("[INFO] Operación cancelada.")
		ui.Pause()
		return nil
	}

	fmt.Println("[1/6] Ejecutando flutter clean")
	_, _ = shell.RunInDir("flutter", ctx.ScriptDir, "clean")
	fmt.Println("  [OK] flutter clean")

	type rm struct {
		label string
		path  string
	}
	dirs := []rm{
		{"build/", filepath.Join(ctx.ScriptDir, "build")},
		{".dart_tool/", filepath.Join(ctx.ScriptDir, ".dart_tool")},
		{"android/.gradle/", filepath.Join(ctx.ScriptDir, "android", ".gradle")},
		{"android/build/", filepath.Join(ctx.ScriptDir, "android", "build")},
		{"android/app/build/", filepath.Join(ctx.ScriptDir, "android", "app", "build")},
		{"node_modules/", filepath.Join(ctx.ScriptDir, "node_modules")},
	}
	for _, d := range dirs {
		if dirExists(d.path) {
			_ = shell.RemoveDir(d.path)
			fmt.Printf("  [OK] %s eliminado\n", d.label)
		}
	}
	for _, f := range []string{".flutter-plugins", ".flutter-plugins-dependencies", ".packages"} {
		p := filepath.Join(ctx.ScriptDir, f)
		if fileExists(p) {
			_ = os.Remove(p)
		}
	}
	fmt.Println()
	fmt.Println("[OK] Proyecto Flutter limpiado!")
	fmt.Println("Para restaurar dependencias: flutter pub get")
	ui.Pause()
	return nil
}
