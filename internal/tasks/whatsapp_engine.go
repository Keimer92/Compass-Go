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
// dentro del repo/instalación (Kit Whatsapp).
const whatsappEngineRel = "Kit Whatsapp"

// whatsappEngineMarker es un archivo opcional que un proyecto puede crear en su
// raíz para optar explícitamente a incrustar el motor de WhatsApp canónico.
// La detección principal es automática (ver projectUsesWhatsappEngine).
const whatsappEngineMarker = "use-whatsapp-engine"

// errScanStop corta el WalkDir cuando ya se encontró la señal.
var errScanStop = errors.New("stop scan")

// whatsappEngineSource devuelve la ruta del motor de WhatsApp canónico.
// Orden de resolución:
//  1. COMPASS_WHATSAPP_ENGINE (override explícito).
//  2. <dir del exe>/Kit Whatsapp (instalación / binario compilado).
//  3. <cwd>/Kit Whatsapp (desarrollo con `go run`).
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
//  3. es Flutter y su build (windows/CMakeLists.txt) o su código Dart
//     referencian whatsapp_engine (aunque la carpeta ya no viva en el proyecto);
//  4. sus fuentes Go importan un paquete cuya ruta referencia whatsapp
//     (detección automática).
func projectUsesWhatsappEngine(ctx *project.ProjectContext) bool {
	if ctx.HasWhatsAppEngine {
		return true
	}
	if fileExists(filepath.Join(ctx.ScriptDir, whatsappEngineMarker)) {
		return true
	}
	if ctx.ProjectType == project.TypeFlutter && flutterUsesWhatsappEngine(ctx.ScriptDir) {
		return true
	}
	return projectImportsWhatsapp(ctx.ScriptDir)
}

// flutterUsesWhatsappEngine detecta que un proyecto Flutter depende del motor
// aunque la carpeta whatsapp_engine ya no esté en el proyecto (ahora vive en el
// corekit de Compass): lo delata la regla install() del CMakeLists de Windows
// o la resolución de rutas del servicio Dart.
func flutterUsesWhatsappEngine(root string) bool {
	if textFileContains(filepath.Join(root, "windows", "CMakeLists.txt"), "whatsapp_engine") {
		return true
	}
	found := false
	_ = filepath.WalkDir(filepath.Join(root, "lib"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".dart" {
			return nil
		}
		if textFileContains(path, "whatsapp_engine") {
			found = true
			return errScanStop
		}
		return nil
	})
	return found
}

// textFileContains indica si el archivo de texto contiene needle, sin
// distinguir mayúsculas.
func textFileContains(path, needle string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(data)), strings.ToLower(needle))
}

// projectImportsWhatsapp escanea recursivamente las fuentes Go del proyecto en
// busca de una importación cuyo path referencie whatsapp (p.ej. internal/whatsapp
// del supervisor). Omite carpetas que no son código fuente (node_modules,
// vendor, .git, bin, resources, out, Kit Whatsapp, corekit) para no rallar ni
// dar falsos positivos dentro del propio Compass.
func projectImportsWhatsapp(root string) bool {
	skip := map[string]bool{
		"node_modules": true, "vendor": true, ".git": true,
		"bin": true, "resources": true, "out": true, "corekit": true,
		"Kit Whatsapp": true,
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
// binario espera whatsapp-engine-bundle (y node.exe si se usa sidecar).
func engineOutputResources(ctx *project.ProjectContext) string {
	switch ctx.ProjectType {
	case project.TypeGo, project.TypeGoWails:
		return filepath.Join(wailsBinDir(ctx), "resources")
	case project.TypeFlutter:
		// El motor Flutter se etaja en whatsapp_engine/ (solo index.js).
		return ctx.ProjectType.WhatsappBundlePath(ctx.ScriptDir)
	default:
		return filepath.Dir(ctx.ProjectType.WhatsappBundlePath(ctx.ScriptDir))
	}
}

// prepareWhatsappEngine deja el motor listo antes de compilar o ejecutar:
// instala las dependencias de un motor local (si existe) y etaja el canónico
// cuando el proyecto lo consume.
func prepareWhatsappEngine(ctx *project.ProjectContext) {
	ensureWhatsappNpm(ctx)
	stageWhatsappEngine(ctx)
}

// stageWhatsappEngine asegura que el motor canónico esté en la salida del
// proyecto. Es un no-op si el proyecto no usa el motor o el motor canónico no
// existe.
func stageWhatsappEngine(ctx *project.ProjectContext) {
	if !projectUsesWhatsappEngine(ctx) {
		return
	}
	src := whatsappEngineSource()
	if !dirExists(src) {
		fmt.Printf("[WARN] Motor de WhatsApp canónico no encontrado en %s; se omite el staging.\n", src)
		return
	}

	// Flutter consume un único index.js autocontenido dentro de whatsapp_engine/.
	if ctx.ProjectType == project.TypeFlutter {
		stageFlutterWhatsappEngine(ctx, src)
		return
	}

	if !ensureWhatsappAssetBundle(src) {
		return
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
	// El sidecar node.exe ya no se etaja: el runtime usa el Node 20+ del
	// sistema (PATH). Si quedó uno de un staging anterior, se elimina para que
	// el proyecto no lo prefiera sobre el del PATH.
	if err := os.Remove(filepath.Join(dest, "node.exe")); err == nil {
		fmt.Println("[INFO] Eliminado node.exe del staging anterior; se usará Node 20+ del sistema.")
	}
	fmt.Printf("[OK] Motor de WhatsApp canónico instalado en %s\n", dest)
}

// ensureWhatsappAssetBundle asegura whatsapp-engine-bundle/index.js (modo
// assetBuilds, ideal para Go/Wails) en el motor canónico.
func ensureWhatsappAssetBundle(src string) bool {
	indexJs := filepath.Join(src, "whatsapp-engine-bundle", "index.js")
	needRebuild := !fileExists(indexJs) || isNewer(filepath.Join(src, "engine.js"), indexJs)
	if needRebuild && isNewer(filepath.Join(src, "package.json"), filepath.Join(src, "node_modules")) {
		fmt.Println("[INFO] Instalando dependencias del motor de WhatsApp canónico...")
		shell.RunNpmInstall(src)
	}
	if needRebuild {
		fmt.Println("[INFO] Compilando motor de WhatsApp canónico (ncc)...")
		if err := shell.RunInDirStatus("node", src, "bundle.mjs"); err != nil {
			if fileExists(indexJs) {
				fmt.Printf("[WARN] No se pudo recompilar el bundle; se usa el existente: %v\n", err)
				return true
			}
			fmt.Printf("[WARN] No se pudo compilar el motor canónico: %v\n", err)
			return false
		}
	}
	return fileExists(indexJs)
}

// ensureWhatsappSingleBundle asegura single/index.js (modo --single: un solo
// archivo autocontenido, ideal para Flutter).
func ensureWhatsappSingleBundle(src string) bool {
	indexJs := filepath.Join(src, "single", "index.js")
	needRebuild := !fileExists(indexJs) || isNewer(filepath.Join(src, "engine.js"), indexJs)
	if needRebuild && isNewer(filepath.Join(src, "package.json"), filepath.Join(src, "node_modules")) {
		fmt.Println("[INFO] Instalando dependencias del motor de WhatsApp canónico...")
		shell.RunNpmInstall(src)
	}
	if needRebuild {
		fmt.Println("[INFO] Compilando motor de WhatsApp canónico (single-file)...")
		if err := shell.RunInDirStatus("node", src, "bundle.mjs", "--single"); err != nil {
			if fileExists(indexJs) {
				fmt.Printf("[WARN] No se pudo recompilar el bundle; se usa el existente: %v\n", err)
				return true
			}
			fmt.Printf("[WARN] No se pudo compilar el motor canónico: %v\n", err)
			return false
		}
	}
	return fileExists(indexJs)
}

// stageFlutterWhatsappEngine deja whatsapp_engine/index.js en la raíz del
// proyecto: es lo que empaqueta windows/CMakeLists.txt junto al exe y lo que el
// servicio Dart resuelve en runtime (Node se toma del PATH).
func stageFlutterWhatsappEngine(ctx *project.ProjectContext, src string) {
	if !ensureWhatsappSingleBundle(src) {
		return
	}
	dest := ctx.ProjectType.WhatsappBundlePath(ctx.ScriptDir)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		fmt.Printf("[WARN] No se pudo crear %s: %v\n", dest, err)
		return
	}
	if err := copyFile(filepath.Join(src, "single", "index.js"), filepath.Join(dest, "index.js")); err != nil {
		fmt.Printf("[WARN] No se pudo copiar el bundle de WhatsApp: %v\n", err)
		return
	}
	// El sidecar node.exe ya no se etaja: el runtime usa el Node 20+ del
	// sistema (PATH). Si quedó uno de un staging anterior, se elimina para que
	// el proyecto no lo prefiera sobre el del PATH.
	if err := os.Remove(filepath.Join(dest, "node.exe")); err == nil {
		fmt.Println("[INFO] Eliminado node.exe del staging anterior; se usará Node 20+ del sistema.")
	}
	fmt.Printf("[OK] Motor de WhatsApp canónico instalado en %s\n", dest)

	// El motor se etaja, pero quien lo mete en el release es la regla install()
	// del CMakeLists. Si falta, se inyecta: sin ella el build sale sin motor y
	// el fallo solo se ve en runtime.
	if !textFileContains(filepath.Join(ctx.ScriptDir, "windows", "CMakeLists.txt"), "whatsapp_engine") {
		wrote, err := ensureFlutterWhatsappCmakeRule(ctx.ScriptDir)
		if err != nil {
			fmt.Printf("[WARN] No se pudo añadir la regla de empaquetado a windows/CMakeLists.txt: %v\n", err)
		} else if wrote {
			fmt.Println("[OK] Añadida la regla install() del motor en windows/CMakeLists.txt")
		}
	}
}

// whatsappCmakeRule instala whatsapp_engine/ junto al ejecutable del build de
// Windows. Va con saltos \n; se adaptan al estilo del archivo al inyectarla.
const whatsappCmakeRule = `# Compass Go: bundle del motor de WhatsApp junto al ejecutable.
# La carpeta la genera Compass en cada build; no editar a mano.
install(DIRECTORY "${CMAKE_SOURCE_DIR}/../whatsapp_engine"
  DESTINATION "${INSTALL_BUNDLE_LIB_DIR}"
  COMPONENT Runtime
  OPTIONAL
  PATTERN "node_modules" EXCLUDE
  PATTERN ".git" EXCLUDE)
`

// ensureFlutterWhatsappCmakeRule añade la regla install() del motor al final de
// windows/CMakeLists.txt si no está. Es idempotente: si el archivo ya menciona
// whatsapp_engine no lo toca. Devuelve true solo cuando escribe.
func ensureFlutterWhatsappCmakeRule(root string) (bool, error) {
	path := filepath.Join(root, "windows", "CMakeLists.txt")
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	content := string(data)
	if strings.Contains(strings.ToLower(content), "whatsapp_engine") {
		return false, nil
	}
	eol := "\n"
	if strings.Contains(content, "\r\n") {
		eol = "\r\n"
	}
	if !strings.HasSuffix(content, "\n") {
		content += eol
	}
	// Una sola linea en blanco de separacion con lo que ya hubiera.
	if !strings.HasSuffix(content, eol+eol) {
		content += eol
	}
	content += strings.ReplaceAll(whatsappCmakeRule, "\n", eol)
	return true, os.WriteFile(path, []byte(content), 0o644)
}

// cleanupReleaseWhatsappSidecar borra el node.exe que un build anterior pudo
// dejar en el Release de Flutter. CMake copia el contenido de whatsapp_engine/
// pero no elimina archivos que ya no existen en origen, así que sin esto el
// release seguiría llevando ~70 MB de sidecar. Solo toca node.exe; index.js se
// conserva.
func cleanupReleaseWhatsappSidecar(releaseDir string) {
	if releaseDir == "" {
		return
	}
	sidecar := filepath.Join(releaseDir, "whatsapp_engine", "node.exe")
	if err := os.Remove(sidecar); err == nil {
		fmt.Println("[INFO] Eliminado node.exe del Release; se usará Node 20+ del sistema.")
	}
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
	switch ctx.ProjectType {
	case project.TypeGo, project.TypeGoWails:
		if err := os.RemoveAll(dest); err != nil {
			fmt.Printf("[WARN] No se pudo limpiar %s: %v\n", dest, err)
		}
		return
	case project.TypeFlutter:
		// Solo se limpia si el release ya se llevo su propia copia (la regla
		// install() del CMakeLists). Sin ella, borrar dejaria el proyecto sin
		// ninguna copia del motor.
		if !textFileContains(filepath.Join(ctx.ScriptDir, "windows", "CMakeLists.txt"), "whatsapp_engine") {
			return
		}
	}
	if err := os.Remove(filepath.Join(dest, "node.exe")); err != nil && !os.IsNotExist(err) {
		fmt.Printf("[WARN] No se pudo borrar node.exe de %s: %v\n", dest, err)
	}
	if err := os.RemoveAll(filepath.Join(dest, "whatsapp-engine-bundle")); err != nil {
		fmt.Printf("[WARN] No se pudo limpiar el bundle en %s: %v\n", dest, err)
	}
	// Flutter: el index.js single-file también lo genera Compass.
	if ctx.ProjectType == project.TypeFlutter {
		if err := os.Remove(filepath.Join(dest, "index.js")); err != nil && !os.IsNotExist(err) {
			fmt.Printf("[WARN] No se pudo borrar index.js de %s: %v\n", dest, err)
		}
		// Solo elimina la carpeta si quedó vacía (no toca archivos del proyecto).
		_ = os.Remove(dest)
	}
}
