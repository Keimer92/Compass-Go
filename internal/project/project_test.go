package project

import (
	"os"
	"path/filepath"
	"testing"
)

// Compass etaja el motor canónico en whatsapp_engine/ (index.js + node.exe).
// Eso NO debe contarse como "motor propio del proyecto": si se cuenta, la tarea
// whatsapp intenta compilar un motor local que no existe (sin engine.js).
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
