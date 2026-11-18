package tasks

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"compass/internal/project"
	"compass/internal/shell"
	"compass/internal/ui"
)

// Plantilla del stub portable. Se copia a un modulo Go temporal y se compila
// con go build; el resultado es un unico .exe que extrae el payload en runtime.
//
//go:embed assets/whatsapp-launcher.go.txt
var whatsappLauncherTemplate string

func init() {
	for _, pt := range allTypes() {
		Register("whatsapp-portable", pt, WhatsappPortableManage)
	}
}

// whatsappPayloadPaths localiza node.exe + el bundle que deben viajar dentro
// del portable. Prefiere lo que el release ya etapo en resources/ y, si no
// existe, cae al motor canonico de Compass.
func whatsappPayloadPaths(ctx *project.ProjectContext) (nodeExe, bundleDir string) {
	res := engineOutputResources(ctx)
	candNode := filepath.Join(res, "node.exe")
	candBundle := filepath.Join(res, "whatsapp-engine-bundle")
	if fileExists(candNode) && dirExists(candBundle) {
		return candNode, candBundle
	}
	src := whatsappEngineSource()
	return filepath.Join(src, "node.exe"), filepath.Join(src, "whatsapp-engine-bundle")
}

// whatsappPayloadValid comprueba que el payload este completo.
func whatsappPayloadValid(nodeExe, bundleDir string) bool {
	return fileExists(nodeExe) && dirExists(bundleDir) &&
		fileExists(filepath.Join(bundleDir, "index.js"))
}

// whatsappPayloadHash produce un ID corto y estable a partir del contenido de
// los archivos del payload. Se inyecta en el stub para que la carpeta de cache
// cambie cuando cambie cualquiera de los archivos embebidos.
func whatsappPayloadHash(paths ...string) string {
	h := sha256.New()
	addFile := func(path, rel string) {
		data, err := os.ReadFile(path)
		if err != nil {
			return
		}
		fmt.Fprintf(h, "F:%s:%d:", filepath.ToSlash(rel), len(data))
		_, _ = h.Write(data)
	}
	for _, root := range paths {
		info, err := os.Stat(root)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			addFile(root, filepath.Base(root))
			continue
		}
		var files []string
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			files = append(files, path)
			return nil
		})
		sort.Strings(files)
		for _, f := range files {
			rel, _ := filepath.Rel(root, f)
			addFile(f, filepath.ToSlash(rel))
		}
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if len(sum) > 12 {
		sum = sum[:12]
	}
	return sum
}

// writeStubIconResource intenta generar un .syso con el icono del proyecto
// dentro del modulo temporal para que go build lo embeba en el stub. Es
// best-effort: si no hay icono o go-winres no esta instalado, no hace nada y
// el portable sale con el icono generico de Go.
func writeStubIconResource(tmp, icoPath, goarch string) {
	if icoPath == "" || !fileExists(icoPath) {
		return
	}
	if _, err := exec.LookPath("go-winres"); err != nil {
		return
	}
	arch := goarch
	if arch == "" {
		arch = runtime.GOARCH
	}

	// go-winres resuelve las rutas del JSON relativas al directorio de trabajo
	// (tmp), asi que copiamos el icono ahi con un nombre fijo sin espacios.
	if err := copyFile(icoPath, filepath.Join(tmp, "icon.ico")); err != nil {
		return
	}
	cfg := `{
  "RT_GROUP_ICON": {
    "APP": {
      "0000": "icon.ico"
    }
  }
}`
	if err := os.WriteFile(filepath.Join(tmp, "winres.json"), []byte(cfg), 0o644); err != nil {
		return
	}
	ok, err := shell.RunProgram("go-winres", []string{
		"make", "--in", "winres.json", "--out", "rsrc", "--arch", arch,
	}, tmp, nil)
	if err != nil || !ok {
		fmt.Printf("[WARN] No se pudo embeber el icono en el portable: %v\n", err)
	}
}

// buildWhatsappPortable envuelve appExe en un unico .exe autocontenido.
// Devuelve "" (sin error) si el proyecto no usa el kit de WhatsApp, para que
// los llamadores puedan aplicarlo de forma incondicional en el release.
// goarch="" compila para el equipo actual; si no, se usa como GOARCH (release).
// outPath es el destino final: si se indica (release), el portable con el kit
// reemplaza a ese exe dejando un solo archivo; si esta vacio (tarea manual) se
// genera un exe nuevo con el sufijo "portable (WhatsApp)".
func buildWhatsappPortable(ctx *project.ProjectContext, appExe, appName, version, goarch, outPath string) (string, error) {
	if !projectUsesWhatsappEngine(ctx) {
		return "", nil
	}
	if appExe == "" || !fileExists(appExe) {
		return "", fmt.Errorf("no se encontro el ejecutable a envolver: %q", appExe)
	}
	nodeExe, bundleDir := whatsappPayloadPaths(ctx)
	if !whatsappPayloadValid(nodeExe, bundleDir) {
		return "", fmt.Errorf("payload del motor de WhatsApp incompleto (node.exe / whatsapp-engine-bundle/index.js)")
	}

	tmp, err := os.MkdirTemp("", "compass-wa-portable-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)

	// Dentro del payload el exe se llama siempre app.exe: los nombres de los
	// artefactos de release llevan espacios ("gympro portable.exe") y un -X
	// con espacios parte los ldflags y rompe el linker. Nombre fijo = cero
	// escapado, y el stub no depende del nombre original de todos modos.
	const launcherExeName = "app.exe"
	payloadDir := filepath.Join(tmp, "payload")
	resourcesDir := filepath.Join(payloadDir, "resources")
	bundleOut := filepath.Join(resourcesDir, "whatsapp-engine-bundle")
	if err := os.MkdirAll(bundleOut, 0o755); err != nil {
		return "", err
	}
	if err := copyFile(appExe, filepath.Join(payloadDir, launcherExeName)); err != nil {
		return "", fmt.Errorf("no se pudo copiar el exe al payload: %w", err)
	}
	if err := copyFile(nodeExe, filepath.Join(resourcesDir, "node.exe")); err != nil {
		return "", fmt.Errorf("no se pudo copiar node.exe al payload: %w", err)
	}
	if err := copyDir(bundleDir, bundleOut); err != nil {
		return "", fmt.Errorf("no se pudo copiar el bundle al payload: %w", err)
	}

	buildID := whatsappPayloadHash(appExe, nodeExe, bundleDir)
	safeName := sanitizeAppName(appName)
	if safeName == "" {
		safeName = "CompassApp"
	}

	writeTemp := func(name, content string) error {
		return os.WriteFile(filepath.Join(tmp, name), []byte(content), 0o644)
	}
	if err := writeTemp("go.mod", "module compasswhatsappstub\n\ngo 1.22\n"); err != nil {
		return "", err
	}
	if err := writeTemp("main.go", whatsappLauncherTemplate); err != nil {
		return "", err
	}
	// Intentamos embeber el icono del proyecto en el stub (best-effort).
	writeStubIconResource(tmp, findProjectIcon(ctx), goarch)

	// Compilamos siempre a un temporal: si el build falla, el exe original
	// sigue intacto. Solo al terminar se mueve al destino final.
	stubExe := filepath.Join(tmp, "stub.exe")
	ldflags := fmt.Sprintf("-s -w -X main.appName=%s -X main.appExe=%s -X main.buildID=%s",
		safeName, launcherExeName, buildID)
	env := map[string]string{"CGO_ENABLED": "0"}
	if goarch != "" {
		env["GOOS"] = "windows"
		env["GOARCH"] = goarch
	}

	fmt.Printf("\n[INFO] Generando portable de un único exe (WhatsApp): %s\n", filepath.Base(appExe))
	ok, err := shell.RunProgram("go", []string{
		"build", "-trimpath", "-ldflags", ldflags, "-o", stubExe, ".",
	}, tmp, env)
	if err != nil {
		return "", err
	}
	if !ok || !fileExists(stubExe) {
		return "", fmt.Errorf("`go build` del stub portable fallo")
	}

	if outPath == "" {
		outName := artifactBase(appName, version, "portable (WhatsApp)") + ".exe"
		outPath = filepath.Join(filepath.Dir(appExe), outName)
	}
	if err := moveFile(stubExe, outPath); err != nil {
		return "", fmt.Errorf("no se pudo colocar el portable en %s: %w", outPath, err)
	}
	fmt.Printf("[OK] Portable generado: %s\n", outPath)
	return outPath, nil
}

// findPortableCandidate busca un exe ya compilado en las carpetas de salida
// tipicas del proyecto. Devuelve el mas reciente, o "" si no hay ninguno.
func findPortableCandidate(ctx *project.ProjectContext) string {
	var candidates []string
	dirs := []string{
		filepath.Join(ctx.ScriptDir, "bin"),
		filepath.Join(ctx.ScriptDir, "build", "bin"),
		ctx.ScriptDir,
	}
	for _, dir := range dirs {
		matches, _ := filepath.Glob(filepath.Join(dir, "*.exe"))
		for _, m := range matches {
			base := strings.ToLower(filepath.Base(m))
			if strings.Contains(base, "uninstall") || strings.Contains(base, "setup") {
				continue
			}
			candidates = append(candidates, m)
		}
	}
	var best string
	var bestMod int64
	for _, c := range candidates {
		info, err := os.Stat(c)
		if err != nil {
			continue
		}
		if m := info.ModTime().Unix(); m > bestMod {
			best, bestMod = c, m
		}
	}
	return best
}

const (
	waNsiBegin = "; COMPASS_WHATSAPP_PAYLOAD_BEGIN"
	waNsiEnd   = "; COMPASS_WHATSAPP_PAYLOAD_END"
)

// stripWhatsappNsi quita un bloque inyectado en una pasada anterior (p.ej. un
// package interrumpido) para poder volver a inyectarlo con rutas actualizadas.
func stripWhatsappNsi(s string) string {
	i := strings.Index(s, waNsiBegin)
	if i < 0 {
		return s
	}
	j := strings.Index(s, waNsiEnd)
	if j < 0 {
		return s
	}
	j += len(waNsiEnd)
	for j < len(s) && (s[j] == '\r' || s[j] == '\n') {
		j++
	}
	// El bloque se anade precedido de una linea en blanco; quitarla tambien.
	if i >= 2 && s[i-2:i] == "\r\n" {
		i -= 2
	} else if i >= 1 && (s[i-1] == '\r' || s[i-1] == '\n') {
		i--
	}
	return s[:i] + s[j:]
}

// injectWhatsappIntoWailsNsi parchea el project.nsi de un proyecto Wails para
// que el instalador incluya node.exe + el bundle. Devuelve una funcion que
// restaura el archivo original y debe ejecutarse tras `wails3 task package`.
// Si el proyecto no usa el kit o no hay script NSIS, no hace nada.
func injectWhatsappIntoWailsNsi(ctx *project.ProjectContext) (func(), error) {
	noop := func() {}
	if !projectUsesWhatsappEngine(ctx) {
		return noop, nil
	}
	node, bundle := whatsappPayloadPaths(ctx)
	if !whatsappPayloadValid(node, bundle) {
		fmt.Println("[WARN] Falta el payload del motor; el instalador Wails no lo incluira.")
		return noop, nil
	}
	nsi := filepath.Join(ctx.ScriptDir, "build", "windows", "nsis", "project.nsi")
	if !fileExists(nsi) {
		fmt.Printf("[WARN] No se encontro %s; el instalador Wails no incluira el motor.\n", nsi)
		return noop, nil
	}
	raw, err := os.ReadFile(nsi)
	if err != nil {
		return noop, err
	}
	original := stripWhatsappNsi(string(raw))

	snippet := "\r\n" + waNsiBegin + "\r\n" +
		"Section \"Compass WhatsApp Engine\"\r\n" +
		"  SectionIn RO\r\n" +
		"  SetOutPath \"$INSTDIR\\resources\"\r\n" +
		"  File \"" + nsisValue(node) + "\"\r\n" +
		"  SetOutPath \"$INSTDIR\\resources\\whatsapp-engine-bundle\"\r\n" +
		"  File /r \"" + nsisValue(filepath.Join(bundle, "*.*")) + "\"\r\n" +
		"  SetOutPath \"$INSTDIR\"\r\n" +
		"SectionEnd\r\n" +
		"\r\n" +
		"Section \"Uninstall\"\r\n" +
		"  Delete \"$INSTDIR\\resources\\node.exe\"\r\n" +
		"  RMDir /r \"$INSTDIR\\resources\\whatsapp-engine-bundle\"\r\n" +
		"  RMDir \"$INSTDIR\\resources\"\r\n" +
		"SectionEnd\r\n" +
		waNsiEnd + "\r\n"

	if err := os.WriteFile(nsi, []byte(original+snippet), 0o644); err != nil {
		return noop, err
	}
	restore := func() {
		if err := os.WriteFile(nsi, []byte(original), 0o644); err != nil {
			fmt.Printf("[WARN] No se pudo restaurar %s: %v\n", nsi, err)
		}
	}
	return restore, nil
}

// WhatsappPortableManage es la tarea reutilizable: toma cualquier exe ya
// compilado y lo envuelve como portable de un unico archivo con el kit dentro.
func WhatsappPortableManage(ctx *project.ProjectContext) error {
	ui.BoxBanner("PORTABLE WHATSAPP · UN ÚNICO EXE")

	if !projectUsesWhatsappEngine(ctx) {
		fmt.Println("[INFO] Este proyecto no usa el motor de WhatsApp; no hay nada que envolver.")
		ui.Pause()
		return nil
	}
	nodeExe, bundleDir := whatsappPayloadPaths(ctx)
	if !whatsappPayloadValid(nodeExe, bundleDir) {
		fmt.Printf("[WARN] Falta el payload del motor (node.exe / bundle). Ejecuta primero la tarea 'whatsapp'.\n")
		ui.Pause()
		return nil
	}

	def := findPortableCandidate(ctx)
	prompt := "Ruta del .exe a envolver"
	if def != "" {
		prompt += " (Enter para usar " + def + ")"
	}
	answer := strings.Trim(strings.TrimSpace(ui.AskText(prompt+": ", def)), `"`)
	if answer == "" {
		fmt.Println("[WARN] No se indico ningun exe.")
		ui.Pause()
		return nil
	}
	if !filepath.IsAbs(answer) {
		answer = filepath.Join(ctx.ScriptDir, answer)
	}
	if !fileExists(answer) {
		fmt.Printf("[ERROR] No se encontro el exe: %s\n", answer)
		ui.Pause()
		return nil
	}

	name := strings.TrimSuffix(filepath.Base(answer), filepath.Ext(answer))
	out, err := buildWhatsappPortable(ctx, answer, name, "", "", "")
	if err != nil {
		fmt.Printf("[ERROR] %v\n", err)
		ui.Pause()
		return nil
	}
	if out == "" {
		ui.Pause()
		return nil
	}
	shell.ShowArtifact(out)
	ui.Pause()
	return nil
}
