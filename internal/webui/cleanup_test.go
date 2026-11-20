package webui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBridgeListCleanupCandidates(t *testing.T) {
	b := newWailsProject(t, false, false)
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// bin/ existe; frontend/dist no (newWailsProject solo crea frontend/).
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "app.exe"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	targets := b.ListCleanupCandidates()
	var binPath, binSize string
	var binDeps bool
	found := false
	for _, tg := range targets {
		if tg.Label == "bin/" {
			found, binPath, binSize, binDeps = true, tg.Path, tg.Size, tg.Deps
		}
		if tg.Label == "frontend/dist" {
			t.Error("frontend/dist no existe y no deberia aparecer")
		}
	}
	if !found {
		t.Fatal("no aparecio el target bin/")
	}
	if binPath != filepath.Join(dir, "bin") {
		t.Errorf("path = %q, queria %q", binPath, filepath.Join(dir, "bin"))
	}
	if binSize == "" {
		t.Error("bin/ sin tamano")
	}
	if binDeps {
		t.Error("bin/ no es deps")
	}
}

func TestBridgePurgeValidatesAgainstActiveProject(t *testing.T) {
	b := newWailsProject(t, false, false)
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "b.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Path de fuera: rechazado, intacto.
	if _, err := b.PurgeCleanupCandidates([]string{outside}); err == nil {
		t.Fatal("esperaba error con path fuera del proyecto")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Error("outside/ no deberia borrarse")
	}
	// Mezcla valido + invalido: nada se borra.
	if _, err := b.PurgeCleanupCandidates([]string{bin, outside}); err == nil {
		t.Fatal("esperaba error con la mezcla")
	}
	if _, err := os.Stat(bin); err != nil {
		t.Error("bin/ no deberia borrarse en la mezcla")
	}
	// Valido: se borra.
	res, err := b.PurgeCleanupCandidates([]string{bin})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 || res.Freed == 0 || res.FreedLabel == "" {
		t.Errorf("resultado inesperado: %+v", res)
	}
	if _, err := os.Stat(bin); !os.IsNotExist(err) {
		t.Error("bin/ deberia haberse borrado")
	}
	for _, tg := range b.ListCleanupCandidates() {
		if tg.Path == bin {
			t.Error("bin/ ya no deberia listarse")
		}
	}
}

func TestBridgeCleanupWithoutProject(t *testing.T) {
	b := &Bridge{}
	if got := b.ListCleanupCandidates(); got == nil || len(got) != 0 {
		t.Errorf("ListCleanupCandidates = %v, queria slice vacio", got)
	}
	if _, err := b.PurgeCleanupCandidates([]string{"x"}); err == nil {
		t.Error("sin proyecto esperaba error")
	}
}
