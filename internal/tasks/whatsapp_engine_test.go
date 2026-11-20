package tasks

import (
	"os"
	"path/filepath"
	"strings"
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

// TestCleanupReleaseWhatsappSidecar verifica que se borra el node.exe obsoleto
// del Release de Flutter sin tocar el index.js del motor.
func TestCleanupReleaseWhatsappSidecar(t *testing.T) {
	dir := t.TempDir()
	engine := filepath.Join(dir, "whatsapp_engine")
	if err := os.MkdirAll(engine, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"node.exe", "index.js"} {
		if err := os.WriteFile(filepath.Join(engine, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cleanupReleaseWhatsappSidecar(dir)
	if fileExists(filepath.Join(engine, "node.exe")) {
		t.Error("no se borro node.exe del Release")
	}
	if !fileExists(filepath.Join(engine, "index.js")) {
		t.Error("se borro index.js del Release")
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

	// Flutter: el motor se etaja en whatsapp_engine/ (index.js + node.exe); se
	// conserva cualquier archivo propio del proyecto (p. ej. engine.js). Solo se
	// limpia si el CMakeLists instala la carpeta (el release ya tiene su copia).
	flutterDir := filepath.Join(root, "whatsapp_engine")
	if err := os.MkdirAll(flutterDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"node.exe", "index.js", "engine.js"} {
		if err := os.WriteFile(filepath.Join(flutterDir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	flutterCtx := &project.ProjectContext{ScriptDir: root, ProjectType: project.TypeFlutter, HasWhatsAppEngine: true}

	// Sin regla install() no se toca nada: el motor no viajo al release.
	cleanupStagedWhatsappResources(flutterCtx)
	if !fileExists(filepath.Join(flutterDir, "index.js")) {
		t.Fatal("se limpio el motor sin que el release lo hubiera copiado")
	}

	// Con la regla install() se quitan los archivos del kit y queda la carpeta
	// (tiene engine.js del proyecto).
	win := filepath.Join(root, "windows")
	if err := os.MkdirAll(win, 0o755); err != nil {
		t.Fatal(err)
	}
	cmake := "install(DIRECTORY \"${CMAKE_SOURCE_DIR}/../whatsapp_engine\" DESTINATION \"${INSTALL_BUNDLE_LIB_DIR}\")"
	if err := os.WriteFile(filepath.Join(win, "CMakeLists.txt"), []byte(cmake), 0o644); err != nil {
		t.Fatal(err)
	}
	cleanupStagedWhatsappResources(flutterCtx)
	if !fileExists(filepath.Join(flutterDir, "engine.js")) {
		t.Error("se borro un archivo del proyecto que no es del kit")
	}
	if fileExists(filepath.Join(flutterDir, "node.exe")) || fileExists(filepath.Join(flutterDir, "index.js")) {
		t.Error("quedaron archivos del kit en Flutter")
	}

	// Si la carpeta solo tenía archivos del kit, desaparece.
	onlyKit := filepath.Join(root, "solo_kit", "whatsapp_engine")
	if err := os.MkdirAll(onlyKit, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"node.exe", "index.js"} {
		if err := os.WriteFile(filepath.Join(onlyKit, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	onlyKitRoot := filepath.Join(root, "solo_kit")
	if err := os.MkdirAll(filepath.Join(onlyKitRoot, "windows"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(onlyKitRoot, "windows", "CMakeLists.txt"), []byte(cmake), 0o644); err != nil {
		t.Fatal(err)
	}
	cleanupStagedWhatsappResources(&project.ProjectContext{ScriptDir: onlyKitRoot, ProjectType: project.TypeFlutter, HasWhatsAppEngine: true})
	if dirExists(onlyKit) {
		t.Error("la carpeta del kit deberia desaparecer si queda vacia")
	}

	// Sin kit no se toca nada.
	noKit := &project.ProjectContext{ScriptDir: root, ProjectType: project.TypeGo}
	cleanupStagedWhatsappResources(noKit)
}

// TestFlutterMarkerEnablesEngine verifica que el marcador use-whatsapp-engine
// (el mismo de GymPro) habilite el motor en Flutter sin depender de que el
// CMakeLists o el código Dart mencionen whatsapp_engine.
func TestFlutterMarkerEnablesEngine(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "pubspec.yaml"), []byte("name: demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &project.ProjectContext{ScriptDir: root, ProjectType: project.TypeFlutter}
	if projectUsesWhatsappEngine(ctx) {
		t.Fatal("sin marcador ni referencias no debe usar el motor")
	}

	if err := os.WriteFile(filepath.Join(root, whatsappEngineMarker), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if !projectUsesWhatsappEngine(ctx) {
		t.Fatal("el marcador use-whatsapp-engine debe habilitar el motor en Flutter")
	}
}

// TestFlutterUsesWhatsappEngine verifica que un proyecto Flutter se detecte como
// consumidor del kit aunque la carpeta whatsapp_engine ya no viva en el
// proyecto: basta la regla del CMakeLists o la resolución del servicio Dart.
// TestEnsureFlutterWhatsappCmakeRule verifica la inyeccion de la regla
// install() del motor en windows/CMakeLists.txt, respetando el fin de linea del
// archivo y sin duplicarla en llamadas posteriores.
func TestEnsureFlutterWhatsappCmakeRule(t *testing.T) {
	for _, tc := range []struct {
		name string
		eol  string
	}{
		{"crlf", "\r\n"},
		{"lf", "\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			win := filepath.Join(root, "windows")
			if err := os.MkdirAll(win, 0o755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(win, "CMakeLists.txt")
			original := "set(INSTALL_BUNDLE_LIB_DIR \"${CMAKE_INSTALL_PREFIX}\")" + tc.eol +
				"install(TARGETS app RUNTIME DESTINATION \"${CMAKE_INSTALL_PREFIX}\")" + tc.eol
			if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
				t.Fatal(err)
			}

			wrote, err := ensureFlutterWhatsappCmakeRule(root)
			if err != nil {
				t.Fatal(err)
			}
			if !wrote {
				t.Fatal("no inyecto la regla")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			got := string(data)
			if !strings.Contains(got, `install(DIRECTORY "${CMAKE_SOURCE_DIR}/../whatsapp_engine"`) {
				t.Fatalf("falta la regla install():\n%s", got)
			}
			if !strings.Contains(got, `DESTINATION "${INSTALL_BUNDLE_LIB_DIR}"`) {
				t.Fatalf("falta el destino del bundle:\n%s", got)
			}
			if !strings.HasPrefix(got, original) {
				t.Fatalf("se altero el contenido original:\n%s", got)
			}
			if strings.Contains(got, tc.eol+tc.eol+tc.eol) {
				t.Fatalf("demasiadas lineas en blanco antes de la regla:\n%s", got)
			}
			if tc.eol == "\r\n" && strings.Count(got, "\n") != strings.Count(got, "\r\n") {
				t.Fatalf("mezcla de finales de linea en un archivo CRLF")
			}
			if tc.eol == "\n" && strings.Contains(got, "\r\n") {
				t.Fatalf("se colaron CRLF en un archivo LF")
			}

			wrote, err = ensureFlutterWhatsappCmakeRule(root)
			if err != nil {
				t.Fatal(err)
			}
			if wrote {
				t.Fatal("deberia ser idempotente")
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != got {
				t.Fatal("la segunda llamada modifico el archivo")
			}
		})
	}
}

func TestFlutterUsesWhatsappEngine(t *testing.T) {
	root := t.TempDir()
	if flutterUsesWhatsappEngine(root) {
		t.Fatal("un proyecto vacío no debe usar el motor")
	}

	win := filepath.Join(root, "windows")
	if err := os.MkdirAll(win, 0o755); err != nil {
		t.Fatal(err)
	}
	cmake := "install(DIRECTORY \"${CMAKE_SOURCE_DIR}/../whatsapp_engine\"\n  DESTINATION \"${INSTALL_BUNDLE_LIB_DIR}\")"
	if err := os.WriteFile(filepath.Join(win, "CMakeLists.txt"), []byte(cmake), 0o644); err != nil {
		t.Fatal(err)
	}
	if !flutterUsesWhatsappEngine(root) {
		t.Error("no se detectó whatsapp_engine en windows/CMakeLists.txt")
	}

	// Sin CMake, la detección también funciona por el código Dart.
	root2 := t.TempDir()
	lib := filepath.Join(root2, "lib", "services")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	dart := "final candidates = ['${Directory.current.path}/whatsapp_engine'];"
	if err := os.WriteFile(filepath.Join(lib, "whatsapp_service.dart"), []byte(dart), 0o644); err != nil {
		t.Fatal(err)
	}
	if !flutterUsesWhatsappEngine(root2) {
		t.Error("no se detectó whatsapp_engine en el código Dart")
	}

	ctx := &project.ProjectContext{ScriptDir: root, ProjectType: project.TypeFlutter}
	if !projectUsesWhatsappEngine(ctx) {
		t.Error("projectUsesWhatsappEngine no detectó el proyecto Flutter")
	}
}
