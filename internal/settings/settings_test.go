package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsWhenFileIsMissing(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())

	if got := Load(); !got.OpenOutputDir {
		t.Fatalf("OpenOutputDir por defecto deberia ser true, got %v", got)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	base := t.TempDir()
	t.Setenv("LOCALAPPDATA", base)

	Save(Settings{OpenOutputDir: false})

	if got := Load(); got.OpenOutputDir {
		t.Fatalf("OpenOutputDir deberia persistir como false")
	}
	if _, err := os.Stat(filepath.Join(base, "compass", "settings.json")); err != nil {
		t.Fatalf("no se escribio settings.json: %v", err)
	}
}

func TestLoadFallsBackOnCorruptFile(t *testing.T) {
	base := t.TempDir()
	t.Setenv("LOCALAPPDATA", base)

	dir := filepath.Join(base, "compass")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := Load(); !got.OpenOutputDir {
		t.Fatalf("un fichero corrupto deberia caer en los valores por defecto")
	}
}
