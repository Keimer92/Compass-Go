package tasks

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"compass/internal/project"
)

// assertValidGo comprueba que el resultado sigue siendo codigo Go compilable.
func assertValidGo(t *testing.T, src string) {
	t.Helper()
	if _, err := parser.ParseFile(token.NewFileSet(), "x.go", src, parser.AllErrors); err != nil {
		t.Errorf("codigo Go invalido: %v\n---\n%s", err, src)
	}
}

func TestApplyGoVersionEndToEnd(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		hasCur bool
	}{
		{"replace existing const", "package main\n\nconst version = \"1.0.0\"\n\nfunc main() {}\n", true},
		{"insert after single import", "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"x\") }\n", false},
		{"insert after import block", "package main\n\nimport (\n\t\"fmt\"\n\t\"os\"\n)\n\nfunc main() { fmt.Println(os.Args) }\n", false},
		{"insert into named package", "package miapp\n\ntype T struct{}\n", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, has := detectVersion(fmtGoConst, c.input)
			if has != c.hasCur {
				t.Fatalf("deteccion: hasCur=%v, queria %v", has, c.hasCur)
			}
			out := applyGoVersion(c.input, has, "3.2.1")
			assertValidGo(t, out)
			if !strings.Contains(out, `const version = "3.2.1"`) {
				t.Errorf("version no actualizada: %q", out)
			}
			if has && strings.Contains(out, "1.0.0") {
				t.Errorf("quedo la version vieja: %q", out)
			}
		})
	}
}

func TestPubspecVersionDetectAndApply(t *testing.T) {
	src := "name: demo\ndescription: x\nversion: 1.0.0+1\nenvironment:\n  sdk: '>=3.0.0'\n"
	got, ok := detectVersion(fmtPubspec, src)
	if !ok || got != "1.0.0+1" {
		t.Fatalf("deteccion pubspec = %q, %v; queria 1.0.0+1", got, ok)
	}
	out, err := applyFormat(&versionFile{format: fmtPubspec, current: got, text: src}, "2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "version: 2.3.4+2") {
		t.Errorf("build number no se incremento:\n%s", out)
	}
	if strings.Contains(out, "1.0.0+1") {
		t.Errorf("quedo la version vieja:\n%s", out)
	}

	// Build explicito + sufijo no numerico se rechaza.
	if _, err := applyFormat(&versionFile{format: fmtPubspec, current: got, text: src}, "2.3.4-beta.1"); err == nil {
		t.Errorf("pubspec acepto un sufijo -pre, deberia fallar")
	}
	explicit, err := applyFormat(&versionFile{format: fmtPubspec, current: got, text: src}, "2.3.4+9")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(explicit, "version: 2.3.4+9") {
		t.Errorf("build explicito no aplicado:\n%s", explicit)
	}
}

func TestKotlinGradleDSLDetectAndApply(t *testing.T) {
	src := "android {\n    defaultConfig {\n        versionCode = 433\n        versionName = \"4.3.3\"\n    }\n}\n"
	name, ok := gradleName(src)
	if !ok || name != "4.3.3" {
		t.Fatalf("versionName DSL = %q, %v", name, ok)
	}
	code, ok := gradleCode(src)
	if !ok || code != 433 {
		t.Fatalf("versionCode DSL = %d, %v", code, ok)
	}
	out, err := applyFormat(&versionFile{format: fmtGradle, current: name, currentCode: code, hasCode: true, text: src}, "4.4.0")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `versionName = "4.4.0"`) {
		t.Errorf("versionName no actualizado:\n%s", out)
	}
	if !strings.Contains(out, "versionCode = 434") {
		t.Errorf("versionCode no incrementado:\n%s", out)
	}
}

func TestWinresVersionDetectAndApply(t *testing.T) {
	src := `{
  "RT_MANIFEST": { "#1": { "0409": { "identity": { "name": "App", "version": "1.0.0.0" } } } },
  "RT_VERSION": { "#1": { "0000": {
    "fixed": { "file_version": "1.0.0.0", "product_version": "1.0.0.0" },
    "info": { "0409": { "FileVersion": "1.0.0.0", "ProductVersion": "1.0.0.0" } }
  } } }
}`
	got, ok := detectVersion(fmtWinres, src)
	if !ok || got != "1.0.0" {
		t.Fatalf("deteccion winres = %q, %v; queria 1.0.0", got, ok)
	}
	out, err := applyFormat(&versionFile{format: fmtWinres, current: got, text: src}, "2.5.1")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, "2.5.1.0") != 5 {
		t.Errorf("winres: se esperaban 5 campos a 2.5.1.0:\n%s", out)
	}
	if !strings.Contains(out, `"product_version": "2.5.1.0"`) {
		t.Errorf("product_version se perdio:\n%s", out)
	}
	if !strings.Contains(out, `"ProductVersion": "2.5.1.0"`) {
		t.Errorf("ProductVersion se perdio:\n%s", out)
	}
}

func TestRunnerRCApply(t *testing.T) {
	src := "#define VERSION_AS_NUMBER 1,0,0,0\n#define VERSION_AS_STRING \"1.0.0\"\n"
	out, err := applyFormat(&versionFile{format: fmtRunnerRC, current: "1.0.0", text: src}, "3.1.4")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "#define VERSION_AS_NUMBER 3,1,4,0") {
		t.Errorf("numero RC no actualizado:\n%s", out)
	}
	if !strings.Contains(out, `#define VERSION_AS_STRING "3.1.4"`) {
		t.Errorf("cadena RC no actualizada:\n%s", out)
	}
}

func TestVersionConfigsByType(t *testing.T) {
	root := t.TempDir()
	ctx := &project.ProjectContext{ScriptDir: root}
	for _, tc := range []struct {
		pt    project.ProjectType
		want  string
		avoid string
	}{
		{project.TypeFlutter, "pubspec.yaml", "go.mod"},
		{project.TypeKotlin, "build.gradle.kts", "pubspec.yaml"},
		{project.TypeGoWails, "go-winres.json", "build/config.yml"},
		{project.TypePython, "pyproject.toml", "pubspec.yaml"},
		{project.TypeGo, "go.mod", "pubspec.yaml"},
	} {
		ctx.ProjectType = tc.pt
		cfgs := versionConfigs(ctx)
		foundWant := false
		for _, c := range cfgs {
			if strings.HasSuffix(c.path, tc.want) {
				foundWant = true
			}
			if strings.HasSuffix(c.path, tc.avoid) && c.source == srcRequired {
				t.Errorf("%v: %s no deberia ser obligatoria", tc.pt, tc.avoid)
			}
		}
		if !foundWant {
			t.Errorf("%v: versionConfigs no incluye %s", tc.pt, tc.want)
		}
	}
}

func TestGoVersionFileAndPackage(t *testing.T) {
	root := t.TempDir()
	// Sin main.go -> version.go, package deducido del module.
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module github.com/acme/mi-app\n\ngo 1.22\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := goVersionFile(root); filepath.Base(got) != "version.go" {
		t.Errorf("sin main.go queria version.go, got %s", filepath.Base(got))
	}
	if got := goPackageFromModule(root); got != "mi_app" {
		t.Errorf("package deducido = %q, queria %q", got, "mi_app")
	}
	seed := "package " + goPackageFromModule(root) + "\n"
	assertValidGo(t, applyGoVersion(seed, false, "1.0.0"))

	// Con main.go -> elige main.go.
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := goVersionFile(root); filepath.Base(got) != "main.go" {
		t.Errorf("con main.go queria main.go, got %s", filepath.Base(got))
	}
}

// La GUI pide el plan sin escribir nada y luego aplica: ambos pasos deben
// describir y tocar los mismos archivos que la tarea de terminal.
func TestVersionInfoAndApplyFlutter(t *testing.T) {
	root := t.TempDir()
	pubspec := "name: demo\nversion: 1.0.0+3\n"
	if err := os.WriteFile(filepath.Join(root, "pubspec.yaml"), []byte(pubspec), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &project.ProjectContext{ScriptDir: root, ProjectType: project.TypeFlutter}

	info, err := VersionInfo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Files) != 1 || info.Files[0].Path != "pubspec.yaml" || info.Files[0].Current != "1.0.0+3" {
		t.Fatalf("plan inesperado: %+v", info)
	}
	if info.Files[0].Missing {
		t.Fatal("pubspec no deberia marcarse como faltante")
	}
	// VersionInfo no escribe.
	if data, _ := os.ReadFile(filepath.Join(root, "pubspec.yaml")); string(data) != pubspec {
		t.Fatalf("VersionInfo modifico el archivo: %q", data)
	}

	res, err := VersionApply(ctx, "2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if res.Version != "2.0.0" || res.Files != 1 {
		t.Fatalf("resultado inesperado: %+v", res)
	}
	data, _ := os.ReadFile(filepath.Join(root, "pubspec.yaml"))
	if !strings.Contains(string(data), "version: 2.0.0+4") {
		t.Fatalf("pubspec no actualizado con build incrementado: %q", data)
	}
}

func TestVersionApplyRejectsBadFormat(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "pubspec.yaml"), []byte("version: 1.0.0+1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &project.ProjectContext{ScriptDir: root, ProjectType: project.TypeFlutter}
	if _, err := VersionApply(ctx, "2.0"); err == nil {
		t.Fatal("se acepto una version con formato invalido")
	}
	if _, err := VersionApply(ctx, "  "); err == nil {
		t.Fatal("se acepto una version vacia")
	}
}

// En Go puro el plan apunta a main.go y la GUI puede crear el const cuando no
// existe, sin dejar codigo invalido.
func TestVersionApplyGoInsertsConst(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module demo/app\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainGo := "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"x\") }\n"
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(mainGo), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &project.ProjectContext{ScriptDir: root, ProjectType: project.TypeGo}

	info, err := VersionInfo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Files) != 1 || !info.Files[0].Missing {
		t.Fatalf("plan inesperado: %+v", info)
	}
	if _, err := VersionApply(ctx, "0.4.0"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(root, "main.go"))
	assertValidGo(t, string(data))
	if !strings.Contains(string(data), `const version = "0.4.0"`) {
		t.Fatalf("no se inserto el const: %q", data)
	}
}
