package tasks

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDropPortableRemovesExeAndKeepsInstaller: con el toggle apagado el release
// deja solo el instalador y borra el .exe suelto del directorio de salida.
func TestDropPortableRemovesExeAndKeepsInstaller(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "App-x64.exe")
	installer := filepath.Join(dir, "App-x64-installer.exe")
	for _, p := range []string{exe, installer} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got := dropPortable([]string{exe, installer}, exe)
	if len(got) != 1 || got[0] != installer {
		t.Fatalf("quedaron %v, queria solo el instalador", got)
	}
	if _, err := os.Stat(exe); !os.IsNotExist(err) {
		t.Fatalf("el portable deberia haberse borrado (err=%v)", err)
	}
}

// TestDropPortableKeepsOnlyArtifact: si no hay instalador (NSIS fallo) el exe
// se conserva: quedarse sin nada seria peor que ignorar el toggle.
func TestDropPortableKeepsOnlyArtifact(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "App-x64.exe")
	if err := os.WriteFile(exe, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := dropPortable([]string{exe}, exe)
	if len(got) != 1 || got[0] != exe {
		t.Fatalf("quedaron %v, queria el exe", got)
	}
	if _, err := os.Stat(exe); err != nil {
		t.Fatalf("no debia borrarse: %v", err)
	}
}

func TestSetIncludePortable(t *testing.T) {
	prev := includePortable
	defer SetIncludePortable(prev)

	SetIncludePortable(false)
	if portableEnabled() {
		t.Fatal("por defecto el portable debe estar desactivado")
	}
	SetIncludePortable(true)
	if !portableEnabled() {
		t.Fatal("SetIncludePortable(true) no surtio efecto")
	}
}
