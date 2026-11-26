package webui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNewBridgeDetectsProject comprueba que el arranque sobre una carpeta con
// go.mod deja ctx listo y el menu con cards (regresion del menu vacio).
func TestNewBridgeDetectsProject(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir) // NewBridge hace os.Chdir; restaura al terminar el test
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module demo\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOCALAPPDATA", t.TempDir())

	b := NewBridge(dir)
	if b.ctx == nil {
		t.Fatalf("ctx nil para %s (debug=%v)", dir, b.GetDebug())
	}
	menu := b.GetMenu()
	if len(menu) == 0 {
		t.Fatalf("menu vacio (debug=%v)", b.GetDebug())
	}
	for _, m := range menu {
		if m.ID == "" || m.Label == "" {
			t.Fatalf("card incompleta: %+v", m)
		}
		if task := b.RunTask(m.ID); task == "" && m.ID != "" {
			t.Logf("card %s (%s) sin tarea CLI", m.Label, m.ID)
		}
	}
}

// TestNewBridgeFallsBackToWorkspace comprueba que una carpeta no-proyecto no
// deja el menu vacio si hay un proyecto persistido.
func TestNewBridgeFallsBackToWorkspace(t *testing.T) {
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)

	proj := filepath.Join(t.TempDir(), "proj")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(proj) // NewBridge hace os.Chdir; restaura al terminar el test
	if err := os.WriteFile(filepath.Join(proj, "go.mod"), []byte("module demo\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	first := NewBridge(proj)
	if first.ctx == nil {
		t.Fatal("no se detecto el proyecto inicial")
	}

	other := t.TempDir() // sin marcadores de proyecto
	b := NewBridge(other)
	if b.ctx == nil || len(b.GetMenu()) == 0 {
		t.Fatalf("menu vacio tras fallback (debug=%v)", b.GetDebug())
	}
}

// TestTaskScriptQuotesPaths comprueba que una ruta con espacios viaja
// entrecomillada a Start-Process -ArgumentList (si no, el hijo recibe
// "--path D:\Mis" y falla con chdir).
func TestTaskScriptQuotesPaths(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Chdir(dir)
	b := NewBridge(dir)

	script := b.TaskScript("env-do-inventory")
	if !strings.Contains(script, "Start-Process") || !strings.Contains(script, "-Wait") {
		t.Fatalf("el script no espera al hijo: %s", script)
	}
	want := `'"` + dir + `"'`
	if !strings.Contains(script, want) {
		t.Fatalf("la ruta no va entrecomillada.\nscript: %s\nesperaba: %s", script, want)
	}
	if strings.Count(script, "'")%2 != 0 {
		t.Fatalf("comillas simples desbalanceadas: %s", script)
	}
}

// TestTaskScriptPortableFlag comprueba que el toggle del modal Win Release
// llega al proceso de la tarea como --portable, y solo a los releases.
func TestTaskScriptPortableFlag(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Chdir(dir)
	b := NewBridge(dir)

	if s := b.TaskScript("windows-release-x64"); strings.Contains(s, "--portable") {
		t.Fatalf("sin activar el toggle no debe llevar --portable: %s", s)
	}
	b.SetReleasePortable(true)
	if s := b.TaskScript("windows-release-x64"); !strings.Contains(s, "'--portable'") {
		t.Fatalf("con el toggle activo falta --portable: %s", s)
	}
	// Una tarea que no es de release no debe recibir el flag.
	if s := b.TaskScript("env-do-inventory"); strings.Contains(s, "--portable") {
		t.Fatalf("una tarea normal no lleva --portable: %s", s)
	}
}
