package tasks

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"compass/internal/project"
)

func TestRenderAppInstallerScriptWhatsappMarkers(t *testing.T) {
	const (
		app  = "Demo"
		ver  = "1.2.3"
		exe  = `C:\apps\Demo\Demo.exe`
		icon = `C:\apps\Demo\icon.ico`
		out  = `C:\apps\Demo\Demo Setup.exe`
		reg  = "Demo"
	)

	// Sin kit: los marcadores desaparecen y no se mete nada del motor.
	plain := renderAppInstallerScript(app, ver, exe, icon, out, reg, nil)
	if strings.Contains(plain, "@WHATSAPP_") {
		t.Fatal("quedaron marcadores de WhatsApp sin sustituir")
	}
	if strings.Contains(plain, "File /r") || strings.Contains(plain, `resources\node.exe`) {
		t.Fatal("el instalador sin kit incluyo archivos del motor")
	}

	// Con kit: se incluyen node.exe, el bundle y su borrado al desinstalar.
	wa := &whatsappInstallerPayload{
		NodeExe:   `C:\kit\node.exe`,
		BundleDir: `C:\kit\whatsapp-engine-bundle`,
	}
	withKit := renderAppInstallerScript(app, ver, exe, icon, out, reg, wa)
	for _, want := range []string{
		`SetOutPath "$INSTDIR\resources"`,
		`File "C:\kit\node.exe"`,
		`File /r "C:\kit\whatsapp-engine-bundle\*.*"`,
		`Delete "$INSTDIR\resources\node.exe"`,
		`RMDir /r "$INSTDIR\resources\whatsapp-engine-bundle"`,
	} {
		if !strings.Contains(withKit, want) {
			t.Errorf("falta en el NSIS: %s", want)
		}
	}
	if strings.Contains(withKit, "@WHATSAPP_") {
		t.Fatal("quedaron marcadores de WhatsApp sin sustituir")
	}

	// Con bundle pero sin sidecar: el bundle viaja y no se referencia node.exe.
	bundleOnly := &whatsappInstallerPayload{BundleDir: `C:\kit\whatsapp-engine-bundle`}
	noNode := renderAppInstallerScript(app, ver, exe, icon, out, reg, bundleOnly)
	if !strings.Contains(noNode, `File /r "C:\kit\whatsapp-engine-bundle\*.*"`) {
		t.Error("falta el bundle en el NSIS sin node.exe")
	}
	if strings.Contains(noNode, `File "C:\kit\node.exe"`) {
		t.Error("se incluyo node.exe aunque no existe")
	}
	if !strings.Contains(noNode, `Delete "$INSTDIR\resources\node.exe"`) {
		t.Error("el desinstalador deberia limpiar un node.exe de versiones anteriores")
	}
}

func TestStripWhatsappNsi(t *testing.T) {
	original := "Section \"Instalar\"\r\n  File \"app.exe\"\r\nSectionEnd\r\n"
	injected := original + "\r\n" + waNsiBegin + "\r\nalgo\r\n" + waNsiEnd + "\r\n"
	if got := stripWhatsappNsi(injected); got != original {
		t.Fatalf("stripWhatsappNsi no restauro el original:\n%q", got)
	}
	if got := stripWhatsappNsi(original); got != original {
		t.Fatalf("stripWhatsappNsi modifico un NSIS sin inyeccion:\n%q", got)
	}
}

func TestBuildWhatsappPortableSkipsWithoutKit(t *testing.T) {
	ctx := &project.ProjectContext{ScriptDir: t.TempDir(), ProjectType: project.TypeGo}
	got, err := buildWhatsappPortable(ctx, filepath.Join(t.TempDir(), "app.exe"), "Demo", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("se genero un portable para un proyecto sin kit: %s", got)
	}
}

// TestBuildWhatsappPortableSpacedExeName reproduce el caso de GymPro: el exe
// portable de Wails se llama "gympro portable.exe". El nombre con espacio no
// debe filtrarse en los -ldflags (el linker interpretaría "portable.exe" como
// una opción y volcaría su ayuda).
func TestBuildWhatsappPortableSpacedExeName(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("el portable de WhatsApp es especifico de Windows")
	}
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	resources := filepath.Join(bin, "resources")
	bundle := filepath.Join(resources, "whatsapp-engine-bundle")
	for _, dir := range []string{bin, resources, bundle} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(resources, "node.exe"), []byte("fake node"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "index.js"), []byte("// fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	appDir := filepath.Join(root, "app-src")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	spacedExe := filepath.Join(bin, "gympro portable.exe")
	buildTestGoExe(t, appDir, "package main\nfunc main(){}\n", spacedExe)

	ctx := &project.ProjectContext{
		ScriptDir:         root,
		ProjectType:       project.TypeGo,
		HasWhatsAppEngine: true,
	}
	// En el release el portable con el kit reemplaza al exe original en su
	// mismo nombre ("gympro portable.exe"), sin dejar un segundo exe suelto.
	out, err := buildWhatsappPortable(ctx, spacedExe, "gympro", "", "amd64", spacedExe)
	if err != nil {
		t.Fatalf("fallo el portable con un exe con espacios: %v", err)
	}
	if !fileExists(out) {
		t.Fatalf("no se genero el portable: %s", out)
	}
	if got, want := filepath.Base(out), "gympro portable.exe"; got != want {
		t.Fatalf("nombre inesperado: %q, quiero %q", got, want)
	}
	// El portable generado ya no es el exe original de 20 MB: lo reemplazo.
	info, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("el portable reemplazo quedo vacio")
	}
	// No debe quedar ningun exe con el sufijo "(WhatsApp)".
	leftovers, _ := filepath.Glob(filepath.Join(bin, "*WhatsApp*.exe"))
	if len(leftovers) != 0 {
		t.Fatalf("quedaron exes sueltos: %v", leftovers)
	}
}

// TestWriteStubIconResource comprueba que, con go-winres disponible y un icono
// del proyecto, se genera el .syso que go build embebe en el stub portable.
func TestWriteStubIconResource(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("los recursos de icono son especificos de Windows")
	}
	if _, err := exec.LookPath("go-winres"); err != nil {
		t.Skip("go-winres no instalado")
	}
	// Icono real del repositorio (ruta relativa al directorio del paquete).
	ico, err := filepath.Abs(filepath.Join("..", "..", "assets", "icon", "Default.ico"))
	if err != nil || !fileExists(ico) {
		t.Skipf("icono de prueba no disponible: %v", err)
	}
	tmp := t.TempDir()
	writeStubIconResource(tmp, ico, "amd64")

	matches, _ := filepath.Glob(filepath.Join(tmp, "rsrc_windows_*.syso"))
	if len(matches) == 0 {
		t.Fatal("writeStubIconResource no genero el .syso del icono")
	}
}

func TestWhatsappPayloadHashStable(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "node.exe")
	if err := os.WriteFile(f, []byte("node"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := whatsappPayloadHash(f)
	b := whatsappPayloadHash(f)
	if a != b || a == "" {
		t.Fatalf("hash inestable: %q vs %q", a, b)
	}
	if err := os.WriteFile(f, []byte("node actualizado"), 0o644); err != nil {
		t.Fatal(err)
	}
	if c := whatsappPayloadHash(f); c == a {
		t.Fatal("el hash no cambio al cambiar el contenido")
	}
}

// buildTestGoExe compila un main.go de prueba con el Go del sistema.
func buildTestGoExe(t *testing.T, dir, mainGo, out string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testfakeapp\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainGo), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = dir
	if outBytes, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build fallo: %v\n%s", err, outBytes)
	}
}

// TestLauncherTemplateExtractAndRun comprueba el ciclo completo del stub:
// extrae el payload a LOCALAPPDATA, lanza el exe real con el cwd y el entorno
// correctos, pasa los argumentos y propaga el codigo de salida.
func TestLauncherTemplateExtractAndRun(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("el portable de WhatsApp es especifico de Windows")
	}
	root := t.TempDir()
	tmpModule := filepath.Join(root, "stub")
	payload := filepath.Join(tmpModule, "payload")
	resources := filepath.Join(payload, "resources")
	bundle := filepath.Join(resources, "whatsapp-engine-bundle")
	for _, dir := range []string{tmpModule, payload, resources, bundle} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// App falsa: escribe cwd, argumentos y COMPASS_WHATSAPP_ENGINE en TEST_OUT.
	fakeApp := `package main
import ("fmt"; "os")
func main() {
	cwd, _ := os.Getwd()
	out := os.Getenv("TEST_OUT")
	if out != "" {
		_ = os.WriteFile(out, []byte(fmt.Sprintf("cwd=%s\nargs=%q\nengine=%s\n", cwd, os.Args[1:], os.Getenv("COMPASS_WHATSAPP_ENGINE"))), 0o644)
	}
	if os.Getenv("TEST_EXIT") == "7" { os.Exit(7) }
}
`
	appDir := filepath.Join(root, "fakeapp")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	buildTestGoExe(t, appDir, fakeApp, filepath.Join(payload, "app.exe"))
	if err := os.WriteFile(filepath.Join(resources, "node.exe"), []byte("fake node"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "index.js"), []byte("// fake bundle"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(tmpModule, "go.mod"), []byte("module stubsample\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpModule, "main.go"), []byte(whatsappLauncherTemplate), 0o644); err != nil {
		t.Fatal(err)
	}
	stub := filepath.Join(root, "stub.exe")
	cmd := exec.Command("go", "build", "-ldflags",
		"-X main.appName=TestApp -X main.appExe=app.exe -X main.buildID=abc123",
		"-o", stub, ".")
	cmd.Dir = tmpModule
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("no se pudo compilar el stub: %v\n%s", err, out)
	}

	local := filepath.Join(root, "LocalAppData")
	if err := os.MkdirAll(local, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOCALAPPDATA", local)
	markerPath := filepath.Join(local, "TestApp", "abc123", ".compass-ready")
	outPath := filepath.Join(root, "out.txt")
	t.Setenv("TEST_OUT", outPath)
	t.Setenv("TEST_EXIT", "")

	runStub := func(args ...string) (string, error) {
		c := exec.Command(stub, args...)
		out, err := c.CombinedOutput()
		return string(out), err
	}

	out, err := runStub("hola", "mundo")
	if err != nil {
		t.Fatalf("el stub fallo: %v\n%s", err, out)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("la app no escribio la salida: %v\n%s", err, out)
	}
	info := string(data)
	wantDir := filepath.Join(local, "TestApp", "abc123")
	if !strings.Contains(info, "cwd="+wantDir) {
		t.Errorf("cwd incorrecto. Salida:\n%s", info)
	}
	if !strings.Contains(info, `args=["hola" "mundo"]`) {
		t.Errorf("argumentos no propagados. Salida:\n%s", info)
	}
	if !strings.Contains(info, "engine="+filepath.Join(wantDir, "resources")) {
		t.Errorf("COMPASS_WHATSAPP_ENGINE incorrecto. Salida:\n%s", info)
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Errorf("no se escribio el marcador de cache: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wantDir, "resources", "whatsapp-engine-bundle", "index.js")); err != nil {
		t.Errorf("no se extrajo el bundle: %v", err)
	}

	// Segundo arranque: ya extraido, no debe volver a imprimir el aviso.
	out2, err := runStub("segundo")
	if err != nil {
		t.Fatalf("el segundo arranque fallo: %v\n%s", err, out2)
	}
	if strings.Contains(out2, "Extrayendo payload") {
		t.Errorf("re-extrajo el payload pese a tener el marcador:\n%s", out2)
	}

	// El codigo de salida de la app se propaga.
	t.Setenv("TEST_EXIT", "7")
	_, err = runStub()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
		t.Fatalf("no se propago el codigo de salida: %v", err)
	}
}
