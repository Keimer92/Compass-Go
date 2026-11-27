package whatsapp

import (
	"os"
	"path/filepath"
	"testing"
)

// TestResolveHostBundlePriorizaElKitCanonico: el host de Flutter debe resolver
// el bundle del kit antes que cualquier copia junto al exe, igual que los
// proyectos Go.
func TestResolveHostBundlePriorizaElKitCanonico(t *testing.T) {
	kit := t.TempDir()
	if err := os.MkdirAll(filepath.Join(kit, "single"), 0o755); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(kit, "single", "index.js")
	if err := os.WriteFile(bundle, []byte("//"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COMPASS_WHATSAPP_ENGINE", kit)

	got, err := resolveHostBundle()
	if err != nil {
		t.Fatalf("resolveHostBundle: %v", err)
	}
	if got != bundle {
		t.Fatalf("bundle = %q, queria %q", got, bundle)
	}
}

// TestResolveHostBundleSinKit informa del error cuando no hay bundle.
func TestResolveHostBundleSinKit(t *testing.T) {
	t.Setenv("COMPASS_WHATSAPP_ENGINE", t.TempDir())
	if _, err := resolveHostBundle(); err == nil {
		t.Fatal("sin bundle deberia devolver error")
	}
}

// TestResolveHostBundleEnCwd cubre el layout de desarrollo: la carpeta
// whatsapp_engine/ del proyecto junto al cwd.
func TestResolveHostBundleEnCwd(t *testing.T) {
	t.Setenv("COMPASS_WHATSAPP_ENGINE", "")
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "whatsapp_engine"), 0o755); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(dir, "whatsapp_engine", "index.js")
	if err := os.WriteFile(bundle, []byte("//"), 0o644); err != nil {
		t.Fatal(err)
	}
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	got, err := resolveHostBundle()
	if err != nil {
		t.Fatalf("resolveHostBundle: %v", err)
	}
	if got != bundle {
		t.Fatalf("bundle = %q, queria %q", got, bundle)
	}
}
