package project

import (
	"os"
	"path/filepath"
	"testing"
)

// Compass etaja el motor canónico en whatsapp_engine/ (index.js + node.exe).
// Eso NO debe contarse como "motor propio del proyecto": si se cuenta, la tarea
// whatsapp intenta compilar un motor local que no existe (sin engine.js).
// Un proyecto Wails v3 "hecho a mano" (sin Taskfile ni build/config.yml),
// pero con go-winres.json o frontend/package.json, debe clasificarse como
// Go+Wails para que reciba el versionador y las tareas correctas.
func TestClassifyWailsV3WithoutTaskfile(t *testing.T) {
	for _, marker := range []string{"go-winres.json", filepath.Join("frontend", "package.json")} {
		root := t.TempDir()
		gomod := "module example.com/app\n\ngo 1.24\n\nrequire github.com/wailsapp/wails/v3 v3.1.0\n"
		if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(gomod), 0o644); err != nil {
			t.Fatal(err)
		}
		markerPath := filepath.Join(root, marker)
		if err := os.MkdirAll(filepath.Dir(markerPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(markerPath, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		pt, ok := ClassifyDir(root)
		if !ok || pt != TypeGoWails {
			t.Errorf("con %s: ClassifyDir = %v, %v; queria Go+Wails", marker, pt, ok)
		}
	}

	// Sin la dependencia wails, aunque haya frontend, es Go puro.
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "frontend"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "frontend", "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if pt, _ := ClassifyDir(root); pt != TypeGo {
		t.Errorf("sin dependencia wails: ClassifyDir = %v, queria Go", pt)
	}
}

func TestHasWhatsAppEngineRequiresLocalSource(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "pubspec.yaml"), []byte("name: demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	engine := filepath.Join(root, "whatsapp_engine")
	if err := os.MkdirAll(engine, 0o755); err != nil {
		t.Fatal(err)
	}

	// Solo archivos etajados por Compass.
	for _, f := range []string{"index.js", "node.exe"} {
		if err := os.WriteFile(filepath.Join(engine, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if hasLocalWhatsappEngine(TypeFlutter, root) {
		t.Fatal("whatsapp_engine/ con solo index.js + node.exe no es un motor local")
	}

	// Con la fuente del motor sí lo es.
	if err := os.WriteFile(filepath.Join(engine, "engine.js"), []byte("//"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !hasLocalWhatsappEngine(TypeFlutter, root) {
		t.Fatal("whatsapp_engine/engine.js debería marcar un motor local")
	}
}
