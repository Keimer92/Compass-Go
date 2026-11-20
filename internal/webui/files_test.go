package webui

import (
	"os"
	"path/filepath"
	"testing"
)

// newFileBridge arma un Bridge con una raiz temporal, sin deteccion de
// proyecto: las operaciones de archivos solo necesitan b.dir.
func newFileBridge(t *testing.T) (*Bridge, string) {
	t.Helper()
	root := t.TempDir()
	return &Bridge{dir: root}, root
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFileRename(t *testing.T) {
	b, root := newFileBridge(t)
	src := filepath.Join(root, "nota.txt")
	writeTestFile(t, src, "hola")

	dest, err := b.FileRename(src, "renombrada.txt")
	if err != nil {
		t.Fatalf("FileRename: %v", err)
	}
	if dest != filepath.Join(root, "renombrada.txt") {
		t.Errorf("destino = %q", dest)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Errorf("no se creo el destino: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("el origen deberia haber desaparecido")
	}

	if _, err := b.FileRename(dest, `sub\otro.txt`); err == nil {
		t.Error("un nombre con separadores debe rechazarse")
	}
	if _, err := b.FileRename(dest, ".."); err == nil {
		t.Error("'..' debe rechazarse")
	}
}

func TestFileDeleteRecursive(t *testing.T) {
	b, root := newFileBridge(t)
	dir := filepath.Join(root, "build")
	writeTestFile(t, filepath.Join(dir, "sub", "a.txt"), "x")

	if err := b.FileDelete(dir); err != nil {
		t.Fatalf("FileDelete: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("la carpeta deberia haberse borrado entera")
	}
	if err := b.FileDelete(""); err == nil {
		t.Error("no se debe poder borrar la raiz del proyecto")
	}
}

func TestFileCopyAndMove(t *testing.T) {
	b, root := newFileBridge(t)
	src := filepath.Join(root, "a", "dato.txt")
	writeTestFile(t, src, "contenido")
	dest := filepath.Join(root, "b")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := b.FileCopy([]string{src}, dest); err != nil {
		t.Fatalf("FileCopy: %v", err)
	}
	copied := filepath.Join(dest, "dato.txt")
	data, err := os.ReadFile(copied)
	if err != nil || string(data) != "contenido" {
		t.Fatalf("copia incorrecta: %q, %v", data, err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Error("copiar no debe borrar el origen")
	}

	// Copiar otra vez al mismo destino debe fallar, no sobrescribir.
	if err := b.FileCopy([]string{src}, dest); err == nil {
		t.Error("se esperaba error al copiar sobre un archivo existente")
	}

	if err := b.FileMove([]string{src}, dest); err == nil {
		t.Error("mover a un destino con el mismo nombre debe fallar")
	}
	other := filepath.Join(root, "c")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := b.FileMove([]string{src}, other); err != nil {
		t.Fatalf("FileMove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(other, "dato.txt")); err != nil {
		t.Error("el archivo deberia estar en el destino")
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("mover debe borrar el origen")
	}
}

// Ninguna operacion puede salirse de la raiz del proyecto.
func TestFileOpsRejectEscape(t *testing.T) {
	b, root := newFileBridge(t)
	outside := filepath.Join(filepath.Dir(root), "fuera.txt")
	writeTestFile(t, outside, "x")
	t.Cleanup(func() { _ = os.Remove(outside) })

	if err := b.FileDelete(outside); err == nil {
		t.Error("FileDelete acepto una ruta fuera del proyecto")
	}
	if _, err := b.FileRename(outside, "otro.txt"); err == nil {
		t.Error("FileRename acepto una ruta fuera del proyecto")
	}
	if err := b.FileCopy([]string{outside}, root); err == nil {
		t.Error("FileCopy acepto una ruta fuera del proyecto")
	}
	if err := b.FileCopy([]string{".."}, root); err == nil {
		t.Error("FileCopy acepto '..'")
	}
}
