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
	Register("windows-dev", project.TypeKotlin, KotlinRunWindows)
	Register("android-dev", project.TypeKotlin, KotlinRunAndroid)
	Register("android-install-debug", project.TypeKotlin, KotlinInstallDebug)
	Register("windows-build", project.TypeKotlin, KotlinBuildWindows)
	Register("android-aab", project.TypeKotlin, KotlinBuildAab)
	Register("android-all", project.TypeKotlin, KotlinBuildAab)
	Register("android-arm64", project.TypeKotlin, KotlinBuildApk)
	Register("debug-android", project.TypeKotlin, kotlinDebugAndroid)
	Register("clean", project.TypeKotlin, KotlinClean)
}

// readKotlinAppMeta lee (name, version) del módulo Android Kotlin.
func readKotlinAppMeta(ctx *project.ProjectContext) (string, string) {
	androidDir := kotlinAndroidDir(ctx)

	var version string
	for _, gf := range []string{
		filepath.Join(androidDir, "build.gradle.kts"),
		filepath.Join(androidDir, "build.gradle"),
	} {
		data, err := os.ReadFile(gf)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "") {
			t := strings.TrimSpace(line)
			if !strings.Contains(t, "versionName") {
				continue
			}
			// toma lo que hay tras "versionName", sin paréntesis ni comillas.
			rest := strings.SplitN(t, "versionName", 2)[1]
			rest = strings.TrimSpace(rest)
			rest = strings.Trim(rest, "\"'")
			rest = strings.TrimRight(rest, ")")
			rest = strings.TrimSpace(rest)
			if rest != "" && rest != "null" {
				version = rest
				break
			}
		}
		if version != "" {
			break
		}
	}

	var name string
	data, err := os.ReadFile(filepath.Join(androidDir, "src", "main", "res", "values", "strings.xml"))
	if err == nil {
		for _, line := range strings.Split(string(data), "") {
			t := strings.TrimSpace(line)
			if !strings.Contains(t, "app_name") {
				continue
			}
			gt := strings.Index(t, ">")
			if gt < 0 {
				continue
			}
			end := strings.Index(t[gt:], "</")
			if end < 0 {
				continue
			}
			val := strings.TrimSpace(t[gt+1 : gt+end])
			if val != "" {
				name = val
				break
			}
		}
	}

	if name == "" {
		name = "App"
	}
	if version == "" {
		version = "0.0.0"
	}
	return sanitizeAppName(name), version
}

// extractNamespace extrae 'namespace = "..."' del Gradle del módulo.
func extractNamespace(ctx *project.ProjectContext) string {
	moduleDir := kotlinAndroidDir(ctx)
	for _, p := range []string{
		filepath.Join(moduleDir, "build.gradle.kts"),
		filepath.Join(moduleDir, "build.gradle"),
	} {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "") {
			t := strings.TrimSpace(line)
			if !strings.HasPrefix(t, "namespace = ") {
				continue
			}
			ns := strings.TrimPrefix(t, "namespace = ")
			ns = strings.Trim(ns, "\"'")
			ns = strings.TrimRight(ns, ";")
			ns = strings.TrimSpace(ns)
			if ns != "" {
				return ns
			}
		}
	}
	return ""
}

// extractMainActivityPackage busca 'package x' en MainActivity.kt.
func extractMainActivityPackage(ctx *project.ProjectContext) string {
	root := filepath.Join(kotlinAndroidDir(ctx), "src")
	var result string
	var walk func(dir string)
	walk = func(dir string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			p := filepath.Join(dir, e.Name())
			if e.IsDir() {
				walk(p)
			} else if e.Name() == "MainActivity.kt" {
				data, err := os.ReadFile(p)
				if err != nil {
					return
				}
				for _, line := range strings.Split(string(data), "") {
					t := strings.TrimSpace(line)
					if strings.HasPrefix(t, "package ") {
						result = strings.TrimRight(strings.TrimSpace(t[len("package "):]), ";")
						return
					}
				}
			}
		}
	}
	walk(root)
	return result
}

// installAndLaunch compila :installDebug y abre la app en el dispositivo.
func installAndLaunch(ctx *project.ProjectContext, serial string) error {
	adb := ctx.AdbPath
	if adb == "" {
		adb = "adb"
	}
	namespace := extractNamespace(ctx)
	if namespace == "" {
		namespace = "com.example.app"
	}
	activity := extractMainActivityPackage(ctx)
	if activity != "" {
		activity = activity + ".MainActivity"
	} else {
		activity = namespace + ".MainActivity"
	}

	gradle := gradlewCmd(fmt.Sprintf("%s:installDebug", gradleModule(ctx)))
	envs := map[string]string{"ANDROID_SERIAL": serial}

	fmt.Printf("\n[INFO] Ejecutando: %s\n\n", gradle)
	ok, err := shell.RunShellStatus(gradle, ctx.ScriptDir, envs)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("[ERROR] Fallo la instalación (gradle installDebug)")
	}
	fmt.Println()
	fmt.Println("[OK] Abriendo aplicacion...")
	_, _ = shell.RunShellStatus(fmt.Sprintf("\"%s\" shell am start -n %s/%s", adb, namespace, activity),
		ctx.ScriptDir, nil)

	fmt.Println()
	ui.Pause()
	return nil
}

// KotlinRunWindows lanza el run de escritorio (Hot Reload) en ventana nueva.
func KotlinRunWindows(ctx *project.ProjectContext) error {
	if err := ensureGradle(ctx); err != nil {
		return err
	}
	ensureWhatsappNpm(ctx)

	shell.ClearScreen()
	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Println("║     KMP DESKTOP                                  ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")
	fmt.Println("[INFO] Iniciando con Hot Reload...")
	fmt.Println()
	return shell.SpawnNewWindowInteractive("KMP Desktop (Run)", ctx.ScriptDir,
		gradlewCmd(fmt.Sprintf("%s:run", gradleModule(ctx))))
}

// KotlinRunAndroid instala y abre la app debug en el dispositivo.
func KotlinRunAndroid(ctx *project.ProjectContext) error {
	if err := ensureGradle(ctx); err != nil {
		return err
	}
	serial, err := selectAndroidDevice(ctx)
	if err != nil {
		return err
	}
	return installAndLaunch(ctx, serial)
}

// KotlinInstallDebug es alias de run_android (instala y abre debug).
func KotlinInstallDebug(ctx *project.ProjectContext) error {
	if err := ensureGradle(ctx); err != nil {
		return err
	}
	serial, err := selectAndroidDevice(ctx)
	if err != nil {
		return err
	}
	return installAndLaunch(ctx, serial)
}

// kotlinDebugAndroid: KMP depura con Gradle installDebug (no hay logcat propio).
func kotlinDebugAndroid(*project.ProjectContext) error {
	fmt.Println("[INFO] Para depurar KMP: ./gradlew :composeApp:installDebug")
	return nil
}

// KotlinBuildWindows compila la distribución de escritorio y la revela.
func KotlinBuildWindows(ctx *project.ProjectContext) error {
	fmt.Println("\n[INFO] Compilando aplicación de escritorio con Gradle...")
	if err := ensureGradle(ctx); err != nil {
		return err
	}
	ensureWhatsappNpm(ctx)

	module := kotlinAndroidDir(ctx)
	distDir := filepath.Join(module, "build", "compose", "binaries", "main")
	name, version := readKotlinAppMeta(ctx)
	rename := ""
	if name != "" {
		if version != "" {
			rename = fmt.Sprintf("%s v%s", name, version)
		} else {
			rename = name
		}
	}
	command := gradlewCmd(fmt.Sprintf("%s:packageDistributionForCurrentOS", gradleModule(ctx)))
	_, err := shell.RunBuildAndReveal(command, ctx.ScriptDir, distDir, []string{"exe"}, rename)
	return err
}

// kotlinApkOutputDir es la carpeta de salida de APKs release de Kotlin.
func kotlinApkOutputDir(ctx *project.ProjectContext) string {
	return filepath.Join(kotlinAndroidDir(ctx), "build", "outputs", "apk", "release")
}

// kotlinApkBase construye "<name>-v<ver>[-<suffix>]" para el APK.
func kotlinApkBase(ctx *project.ProjectContext, suffix string) string {
	name, version := readKotlinAppMeta(ctx)
	base := name
	if version != "" {
		base = fmt.Sprintf("%s-v%s", name, version)
	}
	if suffix != "" {
		base = fmt.Sprintf("%s-%s", base, suffix)
	}
	return base
}

// KotlinBuildApk compila el APK release universal, lo renombra e instala.
func KotlinBuildApk(ctx *project.ProjectContext) error {
	fmt.Println("\n[INFO] Compilando Android APK universal (Release)...")
	if err := ensureGradle(ctx); err != nil {
		return err
	}
	module := gradleModule(ctx)
	command := gradlewCmd(fmt.Sprintf("%s:assembleRelease", module))
	envs := kotlinSigningEnv(ctx)
	if len(envs) == 0 {
		fmt.Println("[WARN] No se encontro keystore.properties de release; el build release podria fallar.")
	}
	fmt.Printf("\n[INFO] Ejecutando: %s\n\n", command)
	ok, err := shell.RunShellStatus(command, ctx.ScriptDir, envs)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("[ERROR] Gradle fallo en `assembleRelease`.")
	}

	dir := kotlinApkOutputDir(ctx)
	src := filepath.Join(dir, "app-release.apk")
	if !fileExists(src) {
		src = shell.NewestArtifact(dir, []string{"apk"})
	}
	if src == "" {
		return fmt.Errorf("[ERROR] No se encontro la APK en %s", dir)
	}
	dest := shell.RenameArtifact(src, kotlinApkBase(ctx, ""))
	installReleaseApk(ctx, dest)
	return nil
}

// installReleaseApk instala el APK release si hay un dispositivo conectado.
func installReleaseApk(ctx *project.ProjectContext, apk string) {
	if apk == "" {
		return
	}
	namespace := extractNamespace(ctx)
	activity := extractMainActivityPackage(ctx)
	if activity != "" {
		activity = activity + ".MainActivity"
	} else {
		activity = namespace + ".MainActivity"
	}
	installApkIfDevice(ctx, apk, namespace, activity)
}

// KotlinBuildAab compila el AAB release y lo revela (con envs de firma).
func KotlinBuildAab(ctx *project.ProjectContext) error {
	fmt.Println("\n[INFO] Compilando Android AAB (Release)...")
	if err := ensureGradle(ctx); err != nil {
		return err
	}
	module := kotlinAndroidDir(ctx)
	gradle := gradlewCmd(fmt.Sprintf("%s:bundleRelease", gradleModule(ctx)))
	envs := kotlinSigningEnv(ctx)
	if len(envs) == 0 {
		fmt.Println("[WARN] No se encontro keystore.properties de release; el build release podria fallar.")
	}
	name, version := readKotlinAppMeta(ctx)
	rename := ""
	if name != "" {
		if version != "" {
			rename = fmt.Sprintf("%s-v%s", name, version)
		} else {
			rename = name
		}
	}
	aabDir := filepath.Join(module, "build", "outputs", "bundle", "release")
	_, err := shell.RunBuildAndRevealWithEnv(gradle, ctx.ScriptDir, aabDir, []string{"aab"}, rename, envs)
	if err == nil {
		fmt.Println("[INFO] Un AAB no se instala directamente en el telefono.")
		fmt.Println("       Usa la tarjeta de APK release para instalarlo, o sube el bundle a Play Store.")
	}
	return err
}

// KotlinClean elimina los artefactos de build de un proyecto KMP.
func KotlinClean(ctx *project.ProjectContext) error {
	shell.ClearScreen()
	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Println("║      LIMPIAR PROYECTO KMP                       ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")
	fmt.Println()
	moduleDir := kotlinAndroidDir(ctx)
	moduleName := filepath.Base(moduleDir)

	fmt.Println("Se eliminará:")
	fmt.Printf("  - build/")
	fmt.Printf("  - %s/build/\n", moduleName)
	fmt.Println("  - .gradle/")
	fmt.Println("  - local.properties")
	fmt.Println("  - node_modules/")
	fmt.Println("  - *.log")
	fmt.Println("  - .kotlin/ (caché Kotlin)")
	fmt.Println()
	fmt.Println("Se conservará:")
	fmt.Printf("  - %s/src/ (código fuente Kotlin)\n", moduleName)
	fmt.Printf("  - %s/build.gradle.kts\n", moduleName)
	fmt.Println("  - settings.gradle.kts, gradle.properties")
	fmt.Println("  - gradle/wrapper/ (wrapper no se borra)")
	fmt.Println("  - gradlew.bat (wrapper de Windows)")
	fmt.Println("  - whatsapp_engine/ (motor WhatsApp)")
	fmt.Printf("  - %s/resources/whatsapp-engine-bundle/ (bundle WhatsApp compilado)\n", moduleName)
	fmt.Println("  - Firmas: keystore.properties, *.keystore, *.jks")
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

	fmt.Println("[1/6] Ejecutando gradle clean")
	_, _ = shell.RunInDir("gradlew.bat", ctx.ScriptDir, "clean")
	fmt.Println("  [OK] gradle clean")

	rootBuild := filepath.Join(ctx.ScriptDir, "build")
	if dirExists(rootBuild) {
		_ = shell.RemoveDir(rootBuild)
		fmt.Println("  [OK] build/ raíz eliminado")
	}
	moduleBuild := filepath.Join(moduleDir, "build")
	if dirExists(moduleBuild) {
		_ = shell.RemoveDir(moduleBuild)
		fmt.Printf("  [OK] %s/build/ eliminado\n", moduleName)
	}
	gradleCache := filepath.Join(ctx.ScriptDir, ".gradle")
	if dirExists(gradleCache) {
		_ = shell.RemoveDir(gradleCache)
		fmt.Println("  [OK] .gradle/ eliminado")
	}
	nm := filepath.Join(ctx.ScriptDir, "node_modules")
	if dirExists(nm) {
		_ = shell.RemoveDir(nm)
		fmt.Println("  [OK] node_modules/ eliminado")
	}
	kotlinCache := filepath.Join(ctx.ScriptDir, ".kotlin")
	if dirExists(kotlinCache) {
		_ = shell.RemoveDir(kotlinCache)
		fmt.Println("  [OK] .kotlin/ eliminado")
	}
	localProps := filepath.Join(ctx.ScriptDir, "local.properties")
	if fileExists(localProps) {
		_ = os.Remove(localProps)
		fmt.Println("  [OK] local.properties eliminado")
	}
	fmt.Println()
	fmt.Println("[OK] Proyecto KMP limpiado!")
	fmt.Println("Para restaurar dependencias: gradlew.bat build")
	ui.Pause()
	return nil
}
