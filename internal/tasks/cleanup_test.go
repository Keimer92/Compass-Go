package tasks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"compass/internal/project"
)

func TestCleanupOptionsAreRegistered(t *testing.T) {
	opts := CleanupOptions()
	if len(opts) == 0 {
		t.Fatal("CleanupOptions no devolvio ninguna accion")
	}
	for _, o := range opts {
		if !Available("clean-do-"+o.Key, project.TypeGo) {
			t.Errorf("la accion %q no esta registrada para Go", o.Key)
		}
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("contenido de prueba suficiente"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestListCleanupTargetsFiltersAndFormats(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "bin", "app.exe"))
	writeFile(t, filepath.Join(root, "frontend", "dist", "index.html"))
	// node_modules NO existe: no debe aparecer aunque sea un target de GoWails.
	ctx := &project.ProjectContext{ScriptDir: root, ProjectType: project.TypeGoWails}

	targets := ListCleanupTargets(ctx)
	got := map[string]CleanupTarget{}
	for _, tg := range targets {
		got[tg.Label] = tg
	}
	for _, label := range []string{"bin/", "frontend/dist"} {
		tg, ok := got[label]
		if !ok {
			t.Errorf("falta el target %q", label)
			continue
		}
		if tg.Size == "" {
			t.Errorf("el target %q no tiene tamano formateado", label)
		}
		if tg.Deps {
			t.Errorf("el target %q no deberia marcarse como deps", label)
		}
	}
	if _, ok := got["frontend/node_modules/"]; ok {
		t.Error("node_modules no existe y no deberia aparecer")
	}

	// Al crear node_modules, aparece marcado como deps.
	writeFile(t, filepath.Join(root, "frontend", "node_modules", "x", "y.js"))
	for _, tg := range ListCleanupTargets(ctx) {
		if tg.Label == "frontend/node_modules/" && !tg.Deps {
			t.Error("node_modules deberia marcarse como deps")
		}
	}
}

func TestPurgeCleanupTargetsDeletesAndReports(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	writeFile(t, filepath.Join(bin, "sub", "a.txt"))
	ctx := &project.ProjectContext{ScriptDir: root, ProjectType: project.TypeGo}

	res, err := PurgeCleanupTargets(ctx, []string{bin})
	if err != nil {
		t.Fatal(err)
	}
	if res.Freed == 0 {
		t.Error("Freed deberia ser > 0")
	}
	if res.FreedLabel == "" {
		t.Error("FreedLabel no deberia estar vacio")
	}
	if len(res.Items) != 1 || res.Items[0].Error != "" {
		t.Errorf("Items inesperados: %+v", res.Items)
	}
	if len(res.Errors) != 0 {
		t.Errorf("errores inesperados: %v", res.Errors)
	}
	if _, err := os.Stat(bin); !os.IsNotExist(err) {
		t.Error("bin/ deberia haberse borrado")
	}
	// Tras purgar, el target ya no se lista.
	for _, tg := range ListCleanupTargets(ctx) {
		if tg.Path == bin {
			t.Error("bin/ ya no deberia aparecer tras borrarlo")
		}
	}
}

func TestPurgeCleanupTargetsRejectsUnknownPathAllOrNothing(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	secret := filepath.Join(root, "secret")
	writeFile(t, filepath.Join(bin, "a.txt"))
	writeFile(t, filepath.Join(secret, "b.txt"))
	ctx := &project.ProjectContext{ScriptDir: root, ProjectType: project.TypeGo}

	// bin es permitido, secret no: no se debe borrar NADA.
	if _, err := PurgeCleanupTargets(ctx, []string{bin, secret}); err == nil {
		t.Fatal("esperaba error al incluir un path no permitido")
	}
	if _, err := os.Stat(bin); err != nil {
		t.Error("bin/ no deberia haberse borrado (validacion todo-o-nada)")
	}
	if _, err := os.Stat(secret); err != nil {
		t.Error("secret/ no deberia haberse borrado")
	}
}

func TestPurgeCleanupTargetsRejectsTraversalAndRelative(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "bin", "a.txt"))
	ctx := &project.ProjectContext{ScriptDir: root, ProjectType: project.TypeGo}

	for _, bad := range []string{
		"bin",
		"..",
		filepath.Join(root, "bin", "..", "..", "afuera"),
	} {
		if _, err := ResolveCleanupTargets(ctx, []string{bad}); err == nil {
			t.Errorf("esperaba rechazo de %q", bad)
		}
	}
	// La capitalizacion distinta es valida en Windows.
	if _, err := ResolveCleanupTargets(ctx, []string{strings.ToUpper(filepath.Join(root, "bin"))}); err != nil {
		t.Errorf("la comparacion insensible a mayusculas deberia aceptar BIN: %v", err)
	}
}

func TestPurgeCleanupTargetsWithoutContext(t *testing.T) {
	if _, err := PurgeCleanupTargets(nil, []string{"x"}); err == nil {
		t.Error("sin contexto esperaba error")
	}
	if got := ListCleanupTargets(nil); got == nil || len(got) != 0 {
		t.Errorf("ListCleanupTargets(nil) = %v, queria slice vacio", got)
	}
}
