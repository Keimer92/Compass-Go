package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFillsDefaultSoundPresets(t *testing.T) {
	base := t.TempDir()
	t.Setenv("LOCALAPPDATA", base)
	dir := filepath.Join(base, "compass")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Fichero antiguo sin sound_presets: deben quedar los valores de fabrica.
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"open_output_dir":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Load()
	if got.SoundPresets["build_ok"] != "classic" {
		t.Fatalf("build_ok deberia ser classic por defecto, got %q", got.SoundPresets["build_ok"])
	}
	if got.SoundPresets["task_ok"] != "none" || got.SoundPresets["task_error"] != "none" {
		t.Fatalf("los eventos de terminal deberian quedar en none por defecto")
	}
}

func TestLoadKeepsPartialSoundPresets(t *testing.T) {
	base := t.TempDir()
	t.Setenv("LOCALAPPDATA", base)
	dir := filepath.Join(base, "compass")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"open_output_dir":true,"sound_presets":{"build_ok":"deep"}}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Load()
	if got.SoundPresets["build_ok"] != "deep" {
		t.Fatalf("build_ok guardado deberia respetarse, got %q", got.SoundPresets["build_ok"])
	}
	if got.SoundPresets["commit_ok"] != "classic" {
		t.Fatalf("las claves ausentes deberian conservar el valor de fabrica, got %q", got.SoundPresets["commit_ok"])
	}
}
