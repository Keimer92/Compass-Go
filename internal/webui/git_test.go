package webui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func initRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git no esta en el PATH")
	}
	// Aislar el workspace persistido: los tests no deben apilar proyectos en
	// el %LOCALAPPDATA% real del usuario.
	t.Setenv("LOCALAPPDATA", t.TempDir())
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module demo\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return dir
}

func TestGitStatusParsesPorcelain(t *testing.T) {
	dir := initRepo(t)
	b := NewBridge(dir)
	if b.ctx == nil {
		t.Fatal("no se detecto el proyecto")
	}

	if err := os.WriteFile(filepath.Join(dir, "nuevo.txt"), []byte("hola"), 0o644); err != nil {
		t.Fatal(err)
	}
	changes := b.GitStatus()
	if len(changes) == 0 {
		t.Fatal("esperaba cambios en el repo recien creado")
	}
	var found bool
	for _, c := range changes {
		if c.Path == "nuevo.txt" {
			found = true
			if c.Status != "Untracked" {
				t.Fatalf("estado inesperado: %+v", c)
			}
		}
	}
	if !found {
		t.Fatalf("nuevo.txt no aparece en los cambios: %+v", changes)
	}
}

func TestGitSummaryIncludesDiffsAndNewFiles(t *testing.T) {
	dir := initRepo(t)
	main := filepath.Join(dir, "main.go")
	if err := os.WriteFile(main, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", "init"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}

	// Un archivo modificado (va en el diff) y otro nuevo (solo en el extracto).
	if err := os.WriteFile(main, []byte("package main\n\nfunc NuevaFuncion() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nuevo.go"), []byte("package main\n\nfunc HelperNuevo() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := gitSummary(dir)
	if !strings.Contains(got, "NuevaFuncion") {
		t.Fatalf("el resumen deberia incluir el diff del archivo modificado:\n%s", got)
	}
	if !strings.Contains(got, "HelperNuevo") {
		t.Fatalf("el resumen deberia incluir un extracto del archivo nuevo:\n%s", got)
	}
}

func TestListDirSkipsHeavyDirs(t *testing.T) {
	dir := initRepo(t)
	b := NewBridge(dir)
	if err := os.MkdirAll(filepath.Join(dir, "node_modules", "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	nodes := b.ListDir("")
	for _, n := range nodes {
		if n.Name == "node_modules" {
			t.Fatal("node_modules no deberia aparecer en el arbol")
		}
	}
	found := false
	for _, n := range nodes {
		if n.Name == "src" && n.IsDir {
			found = true
		}
	}
	if !found {
		t.Fatalf("arbol inesperado: %+v", nodes)
	}
	// El arbol es perezoso: los hijos se piden al desplegar la carpeta.
	children := b.ListDir(filepath.Join(dir, "src"))
	if len(children) != 1 || children[0].Name != "main.go" {
		t.Fatalf("hijos inesperados de src: %+v", children)
	}
}

func TestSubcards(t *testing.T) {
	dir := initRepo(t)
	b := NewBridge(dir)
	if got := b.Subcards("0"); len(got) != 0 {
		t.Fatalf("la card Run no deberia tener subcards: %+v", got)
	}
	for _, id := range []string{"14", "17"} { // Git, Environment
		if got := b.Subcards(id); len(got) == 0 {
			t.Fatalf("la card %s deberia tener subcards", id)
		}
	}
}
