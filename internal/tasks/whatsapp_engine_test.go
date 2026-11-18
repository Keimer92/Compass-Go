package tasks

import (
	"os"
	"path/filepath"
	"testing"

	"compass/internal/project"
)

func TestWhatsappDetection(t *testing.T) {
	// El propio Compass no debe detectarse como consumidor: el skip de corekit
	// y el parseo de imports (no de strings) evitan falsos positivos.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot := filepath.Dir(filepath.Dir(wd))
	self := &project.ProjectContext{ScriptDir: repoRoot, ProjectType: project.TypeGo}
	if projectUsesWhatsappEngine(self) {
		t.Error("Compass se detecto a si mismo como consumidor del motor de WhatsApp")
	}

	base := t.TempDir()

	// Un proyecto que importa un paquete whatsapp si se detecta.
	withImport := filepath.Join(base, "importador")
	if err := os.MkdirAll(filepath.Join(withImport, "internal"), 0o755); err != nil {
		t.Fatal(err)
	}
	mainGo := "package main\nimport _ \"demo/internal/whatsapp\"\nfunc main(){}\n"
	if err := os.WriteFile(filepath.Join(withImport, "main.go"), []byte(mainGo), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &project.ProjectContext{ScriptDir: withImport, ProjectType: project.TypeGo}
	if !projectUsesWhatsappEngine(ctx) {
		t.Error("no se detectó el proyecto que importa el paquete whatsapp")
	}

	// Un mensaje de texto con "whatsapp" NO debe contar como import.
	withText := filepath.Join(base, "texto")
	if err := os.MkdirAll(withText, 0o755); err != nil {
		t.Fatal(err)
	}
	textGo := "package main\nfunc main(){ println(\"resources/whatsapp-engine-bundle/\") }\n"
	if err := os.WriteFile(filepath.Join(withText, "main.go"), []byte(textGo), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx = &project.ProjectContext{ScriptDir: withText, ProjectType: project.TypeGo}
	if projectUsesWhatsappEngine(ctx) {
		t.Error("un literal de texto con 'whatsapp' se conto como import")
	}

	// El marcador explicito tambien activa el motor.
	withMarker := filepath.Join(base, "marcador")
	if err := os.MkdirAll(withMarker, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(withMarker, "use-whatsapp-engine"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	ctx = &project.ProjectContext{ScriptDir: withMarker, ProjectType: project.TypeGo}
	if !projectUsesWhatsappEngine(ctx) {
		t.Error("no se detectó el marcador use-whatsapp-engine")
	}
}

func TestCleanupStagedWhatsappResources(t *testing.T) {
	root := t.TempDir()

	// Go puro / Go+Wails: se va el resources/ entero porque Compass lo genera.
	goRes := filepath.Join(root, "bin", "resources")
	if err := os.MkdirAll(filepath.Join(goRes, "whatsapp-engine-bundle"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(goRes, "node.exe"), []byte("node"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(goRes, "whatsapp-engine-bundle", "index.js"), []byte("//"), 0o644); err != nil {
		t.Fatal(err)
	}
	goCtx := &project.ProjectContext{ScriptDir: root, ProjectType: project.TypeGo, HasWhatsAppEngine: true}
	cleanupStagedWhatsappResources(goCtx)
	if _, err := os.Stat(goRes); !os.IsNotExist(err) {
		t.Fatalf("quedo el resources/ etapado en Go: %v", err)
	}

	// Otras plataformas: solo se van los archivos del kit; se conserva lo demás.
	flutterRes := filepath.Join(root, "resources")
	if err := os.MkdirAll(filepath.Join(flutterRes, "whatsapp-engine-bundle"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"node.exe", "icon.ico"} {
		if err := os.WriteFile(filepath.Join(flutterRes, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(flutterRes, "whatsapp-engine-bundle", "index.js"), []byte("//"), 0o644); err != nil {
		t.Fatal(err)
	}
	flutterCtx := &project.ProjectContext{ScriptDir: root, ProjectType: project.TypeFlutter, HasWhatsAppEngine: true}
	cleanupStagedWhatsappResources(flutterCtx)
	if !fileExists(filepath.Join(flutterRes, "icon.ico")) {
		t.Error("se borro un recurso del proyecto que no es del kit")
	}
	if fileExists(filepath.Join(flutterRes, "node.exe")) || dirExists(filepath.Join(flutterRes, "whatsapp-engine-bundle")) {
		t.Error("quedaron archivos del kit en la plataforma no-Go")
	}

	// Sin kit no se toca nada.
	noKit := &project.ProjectContext{ScriptDir: root, ProjectType: project.TypeGo}
	cleanupStagedWhatsappResources(noKit)
}
