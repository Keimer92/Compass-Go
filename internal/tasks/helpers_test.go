package tasks

import (
	"os"
	"path/filepath"
	"testing"

	"compass/internal/project"
)

func writePubspec(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "pubspec.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadPubspecMetaPrefersDisplayName(t *testing.T) {
	root := t.TempDir()
	writePubspec(t, root, `name: ela_academy
display_name: ELA Academy
version: 1.0.0+1
`)
	ctx := &project.ProjectContext{ScriptDir: root, ProjectType: project.TypeFlutter}

	name, version := readPubspecMeta(ctx)
	if name != "ELA Academy" {
		t.Fatalf("name = %q, want %q", name, "ELA Academy")
	}
	if version != "1.0.0" {
		t.Fatalf("version = %q, want %q", version, "1.0.0")
	}
}

func TestReadPubspecMetaFallsBackToName(t *testing.T) {
	root := t.TempDir()
	writePubspec(t, root, `name: temp_app
version: 2.3.4+7
`)
	ctx := &project.ProjectContext{ScriptDir: root, ProjectType: project.TypeFlutter}

	name, version := readPubspecMeta(ctx)
	if name != "temp_app" {
		t.Fatalf("name = %q, want %q", name, "temp_app")
	}
	if version != "2.3.4" {
		t.Fatalf("version = %q, want %q", version, "2.3.4")
	}
}

func TestReadPubspecMetaTrimsSpacesAndQuotes(t *testing.T) {
	root := t.TempDir()
	writePubspec(t, root, `name:   'technical_name'
display_name:   "Mi Nombre Bonito"
version:   '3.1.0+42'
`)
	ctx := &project.ProjectContext{ScriptDir: root, ProjectType: project.TypeFlutter}

	name, version := readPubspecMeta(ctx)
	if name != "Mi Nombre Bonito" {
		t.Fatalf("name = %q, want %q", name, "Mi Nombre Bonito")
	}
	if version != "3.1.0" {
		t.Fatalf("version = %q, want %q", version, "3.1.0")
	}
}

func TestReadPubspecMetaMissingFileUsesFolderName(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Mi Proyecto Flutter")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx := &project.ProjectContext{ScriptDir: root, ProjectType: project.TypeFlutter}

	name, version := readPubspecMeta(ctx)
	if name != "Mi_Proyecto_Flutter" {
		t.Fatalf("name = %q, want %q", name, "Mi_Proyecto_Flutter")
	}
	if version != "" {
		t.Fatalf("version = %q, want empty", version)
	}
}

func TestArtifactNameUsesDisplayName(t *testing.T) {
	root := t.TempDir()
	writePubspec(t, root, `name: ela_academy
display_name: ELA Academy
version: 1.0.0+1
`)
	ctx := &project.ProjectContext{ScriptDir: root, ProjectType: project.TypeFlutter}

	name, version := flutterAppMeta(ctx)
	got := artifactBase(name, version, "x64") + " Setup.exe"
	want := "ELA Academy v1.0.0 x64 Setup.exe"
	if got != want {
		t.Fatalf("artifact = %q, want %q", got, want)
	}
}

func TestReadPubspecMetaDisplayNameWithEmptyValueFallsBack(t *testing.T) {
	root := t.TempDir()
	writePubspec(t, root, `name: gimnasio
display_name:
version: 4.0.0+1
`)
	ctx := &project.ProjectContext{ScriptDir: root, ProjectType: project.TypeFlutter}

	name, _ := readPubspecMeta(ctx)
	if name != "gimnasio" {
		t.Fatalf("name = %q, want %q", name, "gimnasio")
	}
}
