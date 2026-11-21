package webui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"compass/internal/settings"
)

func TestIsConflict(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{" M", false},
		{"??", false},
		{"UU", true},
		{"AA", true},
		{"UD", true},
		{" M", false},
	}
	for _, tc := range cases {
		if got := isConflict(tc.in); got != tc.want {
			t.Fatalf("isConflict(%q) = %v, queria %v", tc.in, got, tc.want)
		}
	}
}

// Volver a guardar un archivo que YA estaba modificado debe reiniciar el reloj:
// si la firma solo mirase los codigos de git status, la segunda edicion pasaba
// desapercibida y el commit llegaba a mitad de la escritura.
func TestAutoSigDetectsRepeatedEdits(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	dir := initRepo(t)
	path := filepath.Join(dir, "nota.txt")
	if err := os.WriteFile(path, []byte("uno\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	first, _, err := autoSig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if first == "" {
		t.Fatal("la firma no deberia estar vacia con un archivo modificado")
	}

	// Mismo estado git ("??" / "M"), contenido distinto: la firma cambia.
	if err := os.WriteFile(path, []byte("dos, mas largo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, _, err := autoSig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Fatal("editar otra vez el mismo archivo debe cambiar la firma")
	}

	// Sin tocar nada, la firma es estable.
	third, _, err := autoSig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if third != second {
		t.Fatal("la firma debe ser estable cuando no hay actividad")
	}
}

func TestFirstLine(t *testing.T) {
	if got := firstLine("feat: algo\n\n- detalle"); got != "feat: algo" {
		t.Fatalf("firstLine = %q", got)
	}
	if got := firstLine("  solo una  "); got != "solo una" {
		t.Fatalf("firstLine = %q", got)
	}
}

func TestSetAutoCommitRequiresRepo(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module demo\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := NewBridge(dir)
	if err := b.SetAutoCommit(true); err == nil {
		t.Fatal("activar Auto sin repositorio debe fallar")
	}
	if b.AutoCommitStatus().Enabled {
		t.Fatal("el vigilante no debe quedar activo")
	}
}

func TestSetAutoCommitToggle(t *testing.T) {
	dir := initRepo(t)
	b := NewBridge(dir)

	if b.AutoCommitStatus().Enabled {
		t.Fatal("Auto empieza apagado")
	}
	if err := b.SetAutoCommit(true); err != nil {
		t.Fatalf("activar Auto: %v", err)
	}
	st := b.AutoCommitStatus()
	if !st.Enabled || st.IdleMin != 5 {
		t.Fatalf("estado inesperado: %+v", st)
	}
	if err := b.SetAutoCommit(false); err != nil {
		t.Fatalf("desactivar Auto: %v", err)
	}
	if b.AutoCommitStatus().Enabled {
		t.Fatal("Auto debe quedar apagado")
	}
}

// Reiniciar Compass no debe apagar el boton "Auto": el estado se persiste por
// proyecto y el vigilante se rearma al arrancar.
func TestAutoCommitSurvivesRestart(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	dir := initRepo(t)

	b := NewBridge(dir)
	if err := b.SetAutoCommit(true); err != nil {
		t.Fatalf("activar Auto: %v", err)
	}

	// Un Bridge nuevo sobre la misma configuracion equivale a reiniciar la app.
	b2 := NewBridge(dir)
	if !b2.AutoCommitStatus().Enabled {
		t.Fatal("Auto debe seguir activo tras reiniciar")
	}

	if err := b2.SetAutoCommit(false); err != nil {
		t.Fatalf("desactivar Auto: %v", err)
	}
	b3 := NewBridge(dir)
	if b3.AutoCommitStatus().Enabled {
		t.Fatal("un Auto desactivado no debe rearmarse")
	}
}

// Un arbol con cambios que no se mueven acaba commiteado solo. Se baja el
// umbral a 1 minuto en settings para no esperar los 5 reales.
func TestAutoCommitCommitsAfterIdle(t *testing.T) {
	dir := initRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "nota.txt"), []byte("hola\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := settings.Default()
	cfg.AutoCommitIdleMin = 1
	settings.Save(cfg)

	b := NewBridge(dir)
	if err := b.SetAutoCommit(true); err != nil {
		t.Fatalf("activar Auto: %v", err)
	}

	b.autoMu.Lock()
	r := b.autoRuns[commitKey(dir)]
	r.changedAt = r.changedAt.Add(-2 * time.Minute)
	b.autoMu.Unlock()

	b.autoStep(r)

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if out, err := runGit(dir, "log", "--oneline"); err == nil && strings.TrimSpace(out) != "" {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("no se creo el commit automatico")
}
