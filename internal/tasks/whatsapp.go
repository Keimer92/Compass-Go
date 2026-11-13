package tasks

import (
	"fmt"
	"os"
	"path/filepath"

	"compass/internal/project"
	"compass/internal/shell"
	"compass/internal/ui"
)

func init() {
	Register("whatsapp", project.TypeTauri, WhatsappBuild)
	Register("whatsapp", project.TypeFlutter, WhatsappBuild)
	Register("whatsapp", project.TypeKotlin, WhatsappBuild)
	Register("whatsapp", project.TypeRust, WhatsappBuild)
	Register("whatsapp", project.TypePython, WhatsappBuild)
	Register("whatsapp", project.TypeGo, WhatsappBuild)
	Register("whatsapp", project.TypeGoWails, WhatsappBuild)
}

// WhatsappBuild compila el motor de WhatsApp. Si el proyecto tiene una carpeta
// whatsapp_engine local, la usa; si no, usa el motor canónico (definido por
// COMPASS_WHATSAPP_ENGINE, dentro de Compass en corekit/whatsapp) y luego lo
// etaja en el binario del proyecto.
func WhatsappBuild(ctx *project.ProjectContext) error {
	shell.ClearScreen()
	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Println("║         COMPILAR MOTOR DE WHATSAPP               ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")

	if ctx.HasWhatsAppEngine {
		return buildLocalWhatsapp(ctx)
	}

	src := whatsappEngineSource()
	if !dirExists(src) || !fileExists(filepath.Join(src, "engine.js")) {
		fmt.Println()
		fmt.Printf("[ERROR] No se encontró un motor de WhatsApp (ni local ni canónico).\n")
		fmt.Printf("  Motor canónico esperado en: %s\n", src)
		fmt.Println("  Define COMPASS_WHATSAPP_ENGINE o crea whatsapp_engine/ en el proyecto.")
		ui.Pause()
		return nil
	}

	fmt.Println()
	fmt.Println("[INFO] Usando motor de WhatsApp canónico en:")
	fmt.Printf("  %s\n", src)
	fmt.Println("[INFO] Instalando dependencias del motor...")
	if err := shell.RunNpmInstall(src); err != nil {
		fmt.Printf("[ERROR] %v\n", err)
		ui.Pause()
		return nil
	}

	fmt.Println("[INFO] Compilando motor de WhatsApp con ncc...")
	if err := shell.RunInDirStatus("node", src, "bundle.mjs"); err != nil {
		fmt.Printf("[ERROR] Falló la compilación del motor: %v\n", err)
		ui.Pause()
		return nil
	}

	fmt.Println("[OK] Motor de WhatsApp compilado exitosamente!")

	stageWhatsappEngine(ctx)

	bundleDir := filepath.Join(src, "whatsapp-engine-bundle")
	if dirExists(bundleDir) {
		var total int64
		_ = filepath.Walk(bundleDir, func(_ string, info os.FileInfo, _ error) error {
			if info != nil && !info.IsDir() {
				total += info.Size()
			}
			return nil
		})
		fmt.Printf("  Bundle generado en: %s\n", bundleDir)
		fmt.Printf("  Tamaño total del bundle: %.2f MB\n", float64(total)/1048576.0)
	}

	fmt.Println()
	ui.Pause()
	return nil
}

// buildLocalWhatsapp compila el motor de WhatsApp que vive dentro del propio
// proyecto (carpeta whatsapp_engine).
func buildLocalWhatsapp(ctx *project.ProjectContext) error {
	engineDir := ctx.ProjectType.WhatsappPath(ctx.ScriptDir)
	if !dirExists(engineDir) {
		fmt.Println()
		fmt.Printf("[ERROR] No se encontró el motor de WhatsApp en este proyecto.\n")
		fmt.Printf("  Ruta esperada: %s\n\n", engineDir)
		fmt.Println("  Este proyecto no tiene un motor de WhatsApp integrado.")
		ui.Pause()
		return nil
	}
	if !fileExists(filepath.Join(engineDir, "engine.js")) {
		fmt.Println()
		fmt.Printf("[ERROR] No se encontró el motor de WhatsApp en este proyecto.\n")
		fmt.Printf("  Ruta esperada: %s\n", filepath.Join(engineDir, "engine.js"))
		ui.Pause()
		return nil
	}
	bundleMjs := filepath.Join(engineDir, "bundle.mjs")
	if !fileExists(bundleMjs) {
		fmt.Println()
		fmt.Printf("[ERROR] No se encontró el script de compilación del motor.\n")
		fmt.Printf("  Ruta esperada: %s\n", bundleMjs)
		ui.Pause()
		return nil
	}
	if !fileExists(filepath.Join(engineDir, "package.json")) {
		fmt.Println()
		fmt.Printf("[ERROR] El directorio del motor no tiene package.json.\n")
		fmt.Printf("  Ruta: %s\n", engineDir)
		ui.Pause()
		return nil
	}

	fmt.Println()
	fmt.Println("[INFO] Instalando dependencias del motor...")
	if err := shell.RunNpmInstall(engineDir); err != nil {
		fmt.Printf("[ERROR] %v\n", err)
		ui.Pause()
		return nil
	}

	fmt.Println("[INFO] Compilando motor de WhatsApp con ncc...")
	if err := shell.RunInDirStatus("node", engineDir, bundleMjs); err != nil {
		fmt.Printf("[ERROR] Falló la compilación del motor: %v\n", err)
		ui.Pause()
		return nil
	}

	fmt.Println("[OK] Motor de WhatsApp compilado exitosamente!")

	bundleDir := ctx.ProjectType.WhatsappBundlePath(ctx.ScriptDir)
	if dirExists(bundleDir) {
		var total int64
		_ = filepath.Walk(bundleDir, func(_ string, info os.FileInfo, _ error) error {
			if info != nil && !info.IsDir() {
				total += info.Size()
			}
			return nil
		})
		fmt.Printf("  Bundle generado en: %s\n", bundleDir)
		fmt.Printf("  Tamaño total del bundle: %.2f MB\n", float64(total)/1048576.0)
	}

	fmt.Println()
	ui.Pause()
	return nil
}
