package shell

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// El motor de WhatsApp etajado (whatsapp_engine/node.exe) suele tener fecha
// mas reciente que el exe de la app, porque Compass lo copia en cada build.
// NewestArtifact no debe devolverlo: el release acababa renombrando node.exe
// como si fuera el binario del proyecto.
func TestNewestArtifactIgnoresWhatsappSidecar(t *testing.T) {
	root := t.TempDir()
	release := filepath.Join(root, "Release")
	engine := filepath.Join(release, "whatsapp_engine")
	if err := os.MkdirAll(engine, 0o755); err != nil {
		t.Fatal(err)
	}

	app := filepath.Join(release, "miapp.exe")
	if err := os.WriteFile(app, []byte("app"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(app, old, old); err != nil {
		t.Fatal(err)
	}

	node := filepath.Join(engine, "node.exe")
	if err := os.WriteFile(node, []byte("node"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := NewestArtifact(root, []string{"exe"}); got != app {
		t.Fatalf("NewestArtifact() = %q, quiero el exe de la app %q", got, app)
	}
}

func TestNewestArtifactIgnoresNodeExeOutsideSidecarDir(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "miapp.exe")
	if err := os.WriteFile(app, []byte("app"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(app, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "node.exe"), []byte("node"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := NewestArtifact(root, []string{"exe"}); got != app {
		t.Fatalf("NewestArtifact() = %q, quiero %q", got, app)
	}
}
