package tasks

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
