package tasks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultBackupRootUsesDocuments(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", "")

	want := filepath.Join(home, "Documents", "Compass Go", "Entorno de Desarrollo")
	if got := defaultBackupRoot(filepath.Join(home, "appdata")); got != want {
		t.Errorf("defaultBackupRoot = %q, queria %q", got, want)
	}
}

func TestDefaultBackupRootFallsBackToHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", "")
	t.Setenv("HOME", home)

	want := filepath.Join(home, "Documents", "Compass Go", "Entorno de Desarrollo")
	if got := defaultBackupRoot(filepath.Join(home, "appdata")); got != want {
		t.Errorf("defaultBackupRoot = %q, queria %q", got, want)
	}
}

func TestDefaultBackupRootFallsBackToWork(t *testing.T) {
	t.Setenv("USERPROFILE", "")
	t.Setenv("HOME", "")

	work := filepath.Join(t.TempDir(), "env-backup")
	want := filepath.Join(work, "Backup")
	if got := defaultBackupRoot(work); got != want {
		t.Errorf("defaultBackupRoot = %q, queria %q", got, want)
	}
}

func TestLatestBackupReadsDocumentsRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", "")

	root := filepath.Join(home, "Documents", "Compass Go", "Entorno de Desarrollo")
	newest := filepath.Join(root, "EnvBackup_2026-02-01_000000")
	if err := os.MkdirAll(newest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "EnvBackup_2026-01-01_000000"), 0o755); err != nil {
		t.Fatal(err)
	}

	if got := latestBackup(filepath.Join(home, "appdata")); got != newest {
		t.Errorf("latestBackup = %q, queria %q", got, newest)
	}
}
