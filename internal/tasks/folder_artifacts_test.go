package tasks

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// extraerTarGz es la contraparte de test de writeFolderArchive: lee el .tar.gz
// y devuelve un mapa ruta relativa -> contenido.
func extraerTarGz(t *testing.T, archive string) map[string]string {
	t.Helper()
	f, err := os.Open(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()

	out := map[string]string{}
	tr := tar.NewReader(zr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		out[hdr.Name] = string(data)
	}
	return out
}

func TestWriteFolderArchiveRoundtrip(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "release")
	if err := os.MkdirAll(filepath.Join(src, "data", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"app.exe":             "BINARIO",
		"flutter_windows.dll": "DLL",
		"data/a.txt":          "A",
		"data/sub/b.txt":      "B",
	}
	for rel, content := range files {
		if err := os.WriteFile(filepath.Join(src, filepath.FromSlash(rel)), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	archive := filepath.Join(root, "payload.tar.gz")
	if err := writeFolderArchive(src, "", archive); err != nil {
		t.Fatal(err)
	}
	got := extraerTarGz(t, archive)
	if len(got) != len(files) {
		t.Fatalf("esperaba %d archivos, obtuve %d: %v", len(files), len(got), got)
	}
	for rel, content := range files {
		if got[rel] != content {
			t.Errorf("%s: esperaba %q, obtuve %q", rel, content, got[rel])
		}
	}
}

func TestWriteFolderArchiveComprime(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "release")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	// Contenido muy repetitivo: gzip debe comprimirlo claramente.
	if err := os.WriteFile(filepath.Join(src, "fill.bin"), []byte(strings.Repeat("A", 200000)), 0o644); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(root, "payload.tar.gz")
	if err := writeFolderArchive(src, "", archive); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(archive)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() >= 200000 {
		t.Errorf("el archivo no parece comprimido: %d bytes", info.Size())
	}
}

func TestWriteFolderArchiveRenombraExe(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "release")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	realExe := " school_tracker_pro_ela v 1.0.0.exe"
	if err := os.WriteFile(filepath.Join(src, realExe), []byte("BIN"), 0o644); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(root, "payload.tar.gz")
	if err := writeFolderArchive(src, realExe, archive); err != nil {
		t.Fatal(err)
	}
	got := extraerTarGz(t, archive)
	if _, ok := got[realExe]; ok {
		t.Error("el exe conservo su nombre real con espacios")
	}
	if got["app.exe"] != "BIN" {
		t.Errorf("app.exe no encontrado o incorrecto: %v", got)
	}
}

func TestFileHashEstable(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "x.bin")
	if err := os.WriteFile(p, []byte("contenido"), 0o644); err != nil {
		t.Fatal(err)
	}
	h1, err := fileHash(p)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := fileHash(p)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Errorf("hash inestable: %s != %s", h1, h2)
	}
	if len(h1) != 64 {
		t.Errorf("SHA-256 esperado de 64 hex, obtuve %d", len(h1))
	}
	if err := os.WriteFile(p, []byte("otro"), 0o644); err != nil {
		t.Fatal(err)
	}
	h3, err := fileHash(p)
	if err != nil {
		t.Fatal(err)
	}
	if h3 == h1 {
		t.Error("el hash no cambio al modificar el archivo")
	}
}

func TestFolderInstallerTemplateSinMarcadores(t *testing.T) {
	replacer := strings.NewReplacer(
		"@APPNAME@", nsisValue("Mi App"),
		"@APPEXE@", nsisValue("mi.exe"),
		"@APPVER@", nsisValue("1.2.3"),
		"@APPICON@", nsisValue("C:/icono.ico"),
		"@PAYLOADDIR@", nsisValue("C:/release"),
		"@OUTFILE@", nsisValue("C:/bin/Setup.exe"),
		"@REGKEY@", nsisValue("Mi_App"),
	)
	result := replacer.Replace(folderInstallerTemplate)
	if strings.Contains(result, "@") && strings.Contains(result, "@APP") {
		t.Error("quedaron marcadores @APP... sin sustituir")
	}
	for _, needle := range []string{`Name "Mi App"`, `mi.exe`, `1.2.3`, `C:/release\*.*`, `C:/bin/Setup.exe`} {
		if !strings.Contains(result, needle) {
			t.Errorf("falta %q en el script generado", needle)
		}
	}
}

func TestHostArchLabel(t *testing.T) {
	label := HostArchLabel()
	esperado := map[string]string{"amd64": "x64", "386": "x86", "arm64": "ARM64"}
	if want, ok := esperado[runtime.GOARCH]; ok {
		if label != want {
			t.Errorf("HostArchLabel() = %q, esperaba %q", label, want)
		}
	} else if label == "" {
		t.Error("HostArchLabel() esta vacio")
	}
}
