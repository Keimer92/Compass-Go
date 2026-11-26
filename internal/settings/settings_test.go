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
	if got := Load(); got.CloseTabOnBuildEnd {
		t.Fatalf("CloseTabOnBuildEnd por defecto deberia ser false (la pestana no se cierra)")
	}
}

func TestLoadKeepsCloseTabWhenNotInFile(t *testing.T) {
	base := t.TempDir()
	t.Setenv("LOCALAPPDATA", base)

	dir := filepath.Join(base, "compass")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// settings.json antiguo (sin close_tab_on_build_end): debe seguir false.
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"open_output_dir":true}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := Load(); got.CloseTabOnBuildEnd {
		t.Fatalf("un fichero sin close_tab_on_build_end deberia mantenerlo en false")
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

func TestAutoCommitIdleMinClamped(t *testing.T) {
	base := t.TempDir()
	t.Setenv("LOCALAPPDATA", base)
	dir := filepath.Join(base, "compass")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 0 (fichero antiguo) y valores fuera de rango se normalizan.
	for _, tc := range []struct {
		in   string
		want int
	}{
		{`{"auto_commit_idle_min":0}`, 2},
		{`{"auto_commit_idle_min":-3}`, 2},
		{`{"auto_commit_idle_min":999}`, 240},
		{`{"auto_commit_idle_min":12}`, 12},
	} {
		if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(tc.in), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := Load().AutoCommitIdleMin; got != tc.want {
			t.Fatalf("%s => %d, queria %d", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeVibeDisabled(t *testing.T) {
	got := NormalizeVibeDisabled([]string{" 01OpenCode ", "", "01opencode", "02CodeBuddy"})
	if len(got) != 2 || got[0] != "01OpenCode" || got[1] != "02CodeBuddy" {
		t.Fatalf("NormalizeVibeDisabled = %#v, queria [01OpenCode 02CodeBuddy]", got)
	}
}

func TestSameVibeDisabledIgnoresOrderAndCase(t *testing.T) {
	a := []string{"01OpenCode", "02CodeBuddy"}
	b := []string{"02codebuddy", "01OPENCODE"}
	if !SameVibeDisabled(a, b) {
		t.Error("las listas equivalentes deberian comparar iguales")
	}
	if SameVibeDisabled(a, []string{"01OpenCode"}) {
		t.Error("una lista con menos CLIs no es igual")
	}
	if !SameVibeDisabled(nil, []string{}) {
		t.Error("nil y lista vacia son equivalentes")
	}
}
