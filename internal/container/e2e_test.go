//go:build windows

package container

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"compass/internal/clipboard"
)

// TestCompassSubmenusE2E arranca el compass-go.exe real dentro de una ConPTY y
// comprueba que los submenús nuevos (Firmas y Release) se dibujan y que Esc
// vuelve a cerrar la tarea.
//
// Es una prueba de sistema (compila el binario y tarda unos segundos), así que
// sólo corre con COMPASS_E2E=1:
//
//	COMPASS_E2E=1 go test ./internal/container -run TestCompassSubmenusE2E -v
func TestCompassSubmenusE2E(t *testing.T) {
	if os.Getenv("COMPASS_E2E") != "1" {
		t.Skip("define COMPASS_E2E=1 para ejecutar la prueba de extremo a extremo")
	}

	exe := compassExe(t)

	project := t.TempDir()
	// Un proyecto Flutter da acceso a las tareas de firmas y release Android.
	pubspec := "name: fixture\nversion: 1.0.0+1\n"
	if err := os.WriteFile(filepath.Join(project, "pubspec.yaml"), []byte(pubspec), 0644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		task string
		want []string
	}{
		{"menu", "", []string{"pestañas", "navegar", "ejecutar"}},
		{"signing", "signing", []string{"FIRMAS", "Crear keystore", "Verificar APK"}},
		{"git", "git", []string{"GIT", "Inicializar repo"}},
		{"release-android", "android-release", []string{"RELEASE", "arm64", "universal"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var script string
			if tc.task == "" {
				// Menú principal: sin --task, el binario detecta el proyecto y
				// abre el TUI.
				script = fmt.Sprintf("& '%s' --path '%s'", exe, project)
			} else {
				script = PaneScript(exe, tc.task, project)
			}
			s, err := Spawn(script, project, 32, 120, nil)
			if err != nil {
				t.Fatalf("spawn: %v", err)
			}
			defer s.Close()

			// El menú principal no tiene "Volver"; su pie lleva "pestañas".
			gate := "Volver"
			if tc.task == "" {
				gate = "pestañas"
			}
			if !waitForText(s, gate, 25*time.Second) {
				t.Fatalf("no se dibujó. Pantalla:\n%s", s.Text())
			}
			screen := s.Text()
			for _, want := range tc.want {
				if !strings.Contains(screen, want) {
					t.Errorf("falta %q en la pantalla:\n%s", want, screen)
				}
			}

			// Esc vuelve al menú principal / termina la tarea.
			s.Write([]byte("\x1b"))
			deadline := time.Now().Add(10 * time.Second)
			for !s.Exited() && time.Now().Before(deadline) {
				time.Sleep(100 * time.Millisecond)
			}
			if !s.Exited() {
				t.Fatalf("Esc no cerró el submenú. Pantalla:\n%s", s.Text())
			}
		})
	}
}

func waitForText(s *Session, want string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(s.Text(), want) {
			return true
		}
		if s.Exited() {
			return false
		}
		time.Sleep(150 * time.Millisecond)
	}
	return false
}

// compassExe compila el binario real y devuelve su ruta. Se compila siempre
// (una vez por ejecución) para que las pruebas no corran contra un binario
// viejo de una sesión anterior.
var (
	compassOnce sync.Once
	compassPath string
	compassErr  error
)

func compassExe(t *testing.T) string {
	t.Helper()
	compassOnce.Do(func() {
		root, err := filepath.Abs(filepath.Join("..", ".."))
		if err != nil {
			compassErr = err
			return
		}
		exe := filepath.Join(root, "compass-go.exe")
		build := exec.Command("go", "build", "-o", exe, "./cmd/compass-gui")
		build.Dir = root
		if out, err := build.CombinedOutput(); err != nil {
			compassErr = fmt.Errorf("no se pudo compilar compass-go.exe: %v\n%s", err, out)
			return
		}
		compassPath = exe
	})
	if compassErr != nil {
		t.Fatal(compassErr)
	}
	return compassPath
}

// TestTabSwitchE2E reproduce el fallo de abrir una pestana, volver a la pestana
// principal y quedarse sin poder abrir otra: el menu resolvia el clic al pulsar
// con un estado del raton que seguia "pulsado" al cruzar de pestana, asi que la
// primera pulsacion al volver se tomaba por un arrastre.
//
//	COMPASS_E2E=1 go test ./internal/container -run TestTabSwitchE2E -v
func TestTabSwitchE2E(t *testing.T) {
	if os.Getenv("COMPASS_E2E") != "1" {
		t.Skip("define COMPASS_E2E=1 para ejecutar la prueba de extremo a extremo")
	}

	exe := compassExe(t)

	// Proyecto Go minimo: la card "Run" (go run .) termina sola y deja la
	// pestana abierta con su marca ✓.
	project := t.TempDir()
	files := map[string]string{
		"go.mod":  "module fixture\n\ngo 1.21\n",
		"main.go": "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"HOLA MUNDO\") }\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(project, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}

	s, err := Spawn(fmt.Sprintf("& '%s' --path '%s'", exe, project), project, 40, 120, nil)
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer s.Close()

	if !waitForText(s, "DESARROLLO", 25*time.Second) {
		t.Fatalf("el menu no se dibujo. Pantalla:\n%s", s.Text())
	}

	// Primera pestana: card "Run" de la rejilla.
	mouseClick(s, 10, 8)
	if !waitForText(s, "1 Run", 20*time.Second) {
		t.Fatalf("no se abrio la pestana 1. Pantalla:\n%s", s.Text())
	}

	// Volver a Inicio desde la barra.
	mouseClick(s, 5, 1)
	if !waitForText(s, "DESARROLLO", 10*time.Second) {
		t.Fatalf("el clic en la pestana Compass no volvio al menu. Pantalla:\n%s", s.Text())
	}

	// La misma card debe abrir una segunda pestana.
	mouseClick(s, 10, 8)
	if !waitForText(s, "2 Run", 20*time.Second) {
		t.Fatalf("no se abrio la pestana 2 tras volver al menu. Pantalla:\n%s", s.Text())
	}

	// Y desde el menu, la barra sigue sirviendo para volver a una pestana.
	mouseClick(s, 5, 1)
	if !waitForText(s, "DESARROLLO", 10*time.Second) {
		t.Fatalf("no volvio al menu. Pantalla:\n%s", s.Text())
	}
	mouseClick(s, 20, 1)
	if !waitForText(s, "HOLA MUNDO", 10*time.Second) {
		t.Fatalf("el clic en la pestana 1 no la mostro. Pantalla:\n%s", s.Text())
	}
}

// TestSelectionCopyE2E comprueba que arrastrar sobre el terminal de una pestana
// selecciona el texto visible y lo deja en el portapapeles.
//
//	COMPASS_E2E=1 go test ./internal/container -run TestSelectionCopyE2E -v
func TestSelectionCopyE2E(t *testing.T) {
	if os.Getenv("COMPASS_E2E") != "1" {
		t.Skip("define COMPASS_E2E=1 para ejecutar la prueba de extremo a extremo")
	}

	exe := compassExe(t)

	project := t.TempDir()
	files := map[string]string{
		"go.mod":  "module fixture\n\ngo 1.21\n",
		"main.go": "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"HOLA MUNDO\") }\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(project, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}

	s, err := Spawn(fmt.Sprintf("& '%s' --path '%s'", exe, project), project, 40, 120, nil)
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer s.Close()

	if !waitForText(s, "DESARROLLO", 25*time.Second) {
		t.Fatalf("el menu no se dibujo. Pantalla:\n%s", s.Text())
	}
	mouseClick(s, 10, 8) // card Run: pestana con la salida de `go run .`
	if !waitForText(s, "HOLA MUNDO", 25*time.Second) {
		t.Fatalf("la tarea no imprimio nada. Pantalla:\n%s", s.Text())
	}

	row, _, ok := findText(s, "HOLA MUNDO")
	if !ok {
		t.Fatalf("no se encontro el texto. Pantalla:\n%s", s.Text())
	}

	// Marca previa para no dar por bueno lo que ya estaba en el portapapeles.
	if err := clipboard.Write("compass-e2e-sentinel"); err != nil {
		t.Fatalf("no se pudo preparar el portapapeles: %v", err)
	}

	// Shift+arrastrar sobre la linea copiada. s.Text() es la pantalla completa
	// del contenedor (titulo, cuerpo de la pestana y pie), asi que la fila que
	// devuelve findText ya es una coordenada de pantalla del contenedor: el
	// arrastre la usa directamente como la Y del raton (el term.Y del pane resta
	// esa misma altura al convertirla a fila del cuerpo). Se abarca tambien una
	// fila arriba y abajo por si la linea se desplaza un poco, y una franja ancha
	// (hasta la columna 30) porque al terminar el proceso el pane reindenta la
	// salida y el texto se desplaza a la derecha. Asi se captura "HOLA MUNDO".
	y0 := row - 1
	if y0 < BarHeight {
		y0 = BarHeight
	}
	mouseDrag(s, 0, y0, 30, row+1)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if got := clipboardText(t); strings.Contains(got, "HOLA MUNDO") {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("el texto seleccionado no llego al portapapeles (portapapeles: %q). Pantalla:\n%s",
		clipboardText(t), s.Text())
}

// mouseClick manda una pulsacion y su suelta en (col,row).
func mouseClick(s *Session, col, row int) {
	s.Write([]byte(fmt.Sprintf("\x1b[<0;%d;%dM", col+1, row+1)))
	time.Sleep(120 * time.Millisecond)
	s.Write([]byte(fmt.Sprintf("\x1b[<0;%d;%dm", col+1, row+1)))
	time.Sleep(500 * time.Millisecond)
}

// mouseDrag arrastra con Shift (boton 1 + modificador 4) de (x0,y0) a (x1,y1).
func mouseDrag(s *Session, x0, y0, x1, y1 int) {
	s.Write([]byte(fmt.Sprintf("\x1b[<4;%d;%dM", x0+1, y0+1)))
	time.Sleep(120 * time.Millisecond)
	s.Write([]byte(fmt.Sprintf("\x1b[<36;%d;%dM", x1+1, y1+1)))
	time.Sleep(120 * time.Millisecond)
	s.Write([]byte(fmt.Sprintf("\x1b[<4;%d;%dm", x1+1, y1+1)))
	time.Sleep(300 * time.Millisecond)
}

// findText busca want en la pantalla y devuelve su fila y columna.
func findText(s *Session, want string) (row, col int, ok bool) {
	for y, line := range strings.Split(s.Text(), "\n") {
		if i := strings.Index(line, want); i >= 0 {
			return y, utf8.RuneCountInString(line[:i]), true
		}
	}
	return 0, 0, false
}

// clipboardText lee el portapapeles con PowerShell (solo para las pruebas).
func clipboardText(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("powershell", "-NoProfile", "-Command", "Get-Clipboard -Raw").Output()
	if err != nil {
		t.Fatalf("Get-Clipboard: %v", err)
	}
	return string(out)
}
