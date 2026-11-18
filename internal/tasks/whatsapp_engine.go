package tasks

import (
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"compass/internal/project"
	"compass/internal/shell"
)

// Relativa al ejecutable de Compass donde vive el motor de WhatsApp canónico
// dentro del repo/instalación (corekit/<kit>). Aquí el único kit es whatsapp.
const whatsappEngineRel = "corekit/whatsapp"

// whatsappEngineMarker es un archivo opcional que un proyecto puede crear en su
// raíz para optar explícitamente a incrustar el motor de WhatsApp canónico.
// La detección principal es automática (ver projectUsesWhatsappEngine).
const whatsappEngineMarker = "use-whatsapp-engine"

// errScanStop corta el WalkDir cuando ya se encontró la señal.
var errScanStop = errors.New("stop scan")

// whatsappEngineSource devuelve la ruta del motor de WhatsApp canónico.
// Orden de resolución:
//  1. COMPASS_WHATSAPP_ENGINE (override explícito).
//  2. <dir del exe>/corekit/whatsapp (instalación / binario compilado).
//  3. <cwd>/corekit/whatsapp (desarrollo con `go run`).
//  4. fallback a (2).
func whatsappEngineSource() string {
	if v := os.Getenv("COMPASS_WHATSAPP_ENGINE"); v != "" {
		return filepath.Clean(v)
	}
	rel := whatsappEngineRel
	if exe, err := os.Executable(); err == nil {
		if cand := filepath.Join(filepath.Dir(exe), rel); dirExists(cand) {
			return cand
		}
	}
	if wd, err := os.Getwd(); err == nil {
		if cand := filepath.Join(wd, rel); dirExists(cand) {
			return cand
		}
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), rel)
	}
	return rel
}

// projectUsesWhatsappEngine indica si el proyecto debe incrustar el motor.
// Criterios (cualquiera basta):
//  1. tiene una carpeta whatsapp_engine local (motor propio del proyecto);
//  2. tiene el marcador use-whatsapp-engine en su raíz (opt-in explícito);
//  3. sus fuentes Go importan un paquete cuya ruta referencia whatsapp
//     (detección automática).
func projectUsesWhatsappEngine(ctx *project.ProjectContext) bool {
	if ctx.HasWhatsAppEngine {
		return true
	}
	if fileExists(filepath.Join(ctx.ScriptDir, whatsappEngineMarker)) {
		return true
	}
	return projectImportsWhatsapp(ctx.ScriptDir)
}

// projectImportsWhatsapp escanea recursivamente las fuentes Go del proyecto en
// busca de una importación cuyo path referencie whatsapp (p.ej. internal/whatsapp
// del supervisor). Omite carpetas que no son código fuente (node_modules,
// vendor, .git, bin, resources, out, corekit) para no rallar ni dar falsos
// positivos dentro del propio Compass.
func projectImportsWhatsapp(root string) bool {
	skip := map[string]bool{
		"node_modules": true, "vendor": true, ".git": true,
		"bin": true, "resources": true, "out": true, "corekit": true,
	}
	found := false
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skip[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		// Parsear solo los imports evita falsos positivos con literales de
		// texto (p.ej. los propios mensajes de Compass que dicen
		// "resources/whatsapp-engine-bundle/").
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return nil
		}
		for _, imp := range file.Imports {
			importPath, err := strconv.Unquote(imp.Path.Value)
			if err == nil && strings.Contains(importPath, "whatsapp") {
				found = true
				return errScanStop
			}
		}
		return nil
	})
	return found
}

// engineOutputResources devuelve el directorio resources/ de salida donde el
// binario espera node.exe + whatsapp-engine-bundle.
func engineOutputResources(ctx *project.ProjectContext) string {
	switch ctx.ProjectType {
	case project.TypeGo, project.TypeGoWails:
		return filepath.Join(wailsBinDir(ctx), "resources")
	default:
		return filepath.Dir(ctx.ProjectType.WhatsappBundlePath(ctx.ScriptDir))
	}
}

// stageWhatsappEngine asegura que node.exe + el bundle del motor canónico estén
// en el resources/ de salida del proyecto. Es un no-op si el proyecto no usa el
// motor o el motor canónico no existe.
func stageWhatsappEngine(ctx *project.ProjectContext) {
	if !projectUsesWhatsappEngine(ctx) {
		return
	}
	src := whatsappEngineSource()
	if !dirExists(src) {
		fmt.Printf("[WARN] Motor de WhatsApp canónico no encontrado en %s; se omite el staging.\n", src)
		return
	}

	// ¿Hay que reconstruir el bundle? Solo entonces hacen falta las
	// dependencias (node_modules), que en la copia instalada no se envian:
	// el bundle ya viene compilado y es autocontenido.
	bundleSrc := filepath.Join(src, "whatsapp-engine-bundle")
	indexJs := filepath.Join(bundleSrc, "index.js")
	engineJs := filepath.Join(src, "engine.js")
	needRebuild := !fileExists(indexJs) || isNewer(engineJs, indexJs)

	if needRebuild && isNewer(filepath.Join(src, "package.json"), filepath.Join(src, "node_modules")) {
		fmt.Println("[INFO] Instalando dependencias del motor de WhatsApp canónico...")
		shell.RunNpmInstall(src)
	}

	if needRebuild {
		fmt.Println("[INFO] Compilando motor de WhatsApp canónico (ncc)...")
		if err := shell.RunInDirStatus("node", src, "bundle.mjs"); err != nil {
			if fileExists(indexJs) {
				fmt.Printf("[WARN] No se pudo recompilar el bundle; se usa el existente: %v\n", err)
			} else {
				fmt.Printf("[WARN] No se pudo compilar el motor canónico: %v\n", err)
				return
			}
		}
	}

	dest := engineOutputResources(ctx)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		fmt.Printf("[WARN] No se pudo crear %s: %v\n", dest, err)
		return
	}
	if err := copyDir(filepath.Join(src, "whatsapp-engine-bundle"), filepath.Join(dest, "whatsapp-engine-bundle")); err != nil {
		fmt.Printf("[WARN] No se pudo copiar el bundle de WhatsApp: %v\n", err)
		return
	}
	if fileExists(filepath.Join(src, "node.exe")) {
		_ = copyFile(filepath.Join(src, "node.exe"), filepath.Join(dest, "node.exe"))
	}
	fmt.Printf("[OK] Motor de WhatsApp canónico instalado en %s\n", dest)
}

// cleanupStagedWhatsappResources borra el resources/ temporario que
// stageWhatsappEngine deja junto al binario. Tras el release, el portable (un
// único exe) y el instalador ya llevan cada uno su propia copia embebida, así
// que no debe quedar el motor como archivo suelto en la carpeta de salida.
// En Go/Go+Wails ese resources/ es íntegramente generado por Compass (bin/
// resources); en el resto de plataformas solo se quitan los archivos del kit
// por si la carpeta contiene otros recursos del proyecto.
func cleanupStagedWhatsappResources(ctx *project.ProjectContext) {
	if !projectUsesWhatsappEngine(ctx) {
		return
	}
	dest := engineOutputResources(ctx)
	if ctx.ProjectType == project.TypeGo || ctx.ProjectType == project.TypeGoWails {
		if err := os.RemoveAll(dest); err != nil {
			fmt.Printf("[WARN] No se pudo limpiar %s: %v\n", dest, err)
		}
		return
	}
	if err := os.Remove(filepath.Join(dest, "node.exe")); err != nil && !os.IsNotExist(err) {
		fmt.Printf("[WARN] No se pudo borrar node.exe de %s: %v\n", dest, err)
	}
	if err := os.RemoveAll(filepath.Join(dest, "whatsapp-engine-bundle")); err != nil {
		fmt.Printf("[WARN] No se pudo limpiar el bundle en %s: %v\n", dest, err)
	}
}
