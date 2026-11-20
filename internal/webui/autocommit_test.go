package webui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"compass/internal/settings"
)

func TestHasConflict(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{" M archivo.go", false},
		{"?? nuevo.txt", false},
		{"UU conflicto.go", true},
		{" M ok.go\nAA ambos.go\n", true},
		{"UD borrado.go", true},
	}
	for _, tc := range cases {
		if got := hasConflict(tc.in); got != tc.want {
			t.Fatalf("hasConflict(%q) = %v, queria %v", tc.in, got, tc.want)
		}
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
