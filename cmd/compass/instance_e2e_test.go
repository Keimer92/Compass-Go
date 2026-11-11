//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"compass/internal/container"
	"compass/internal/lock"
)

// TestSingleInstancePerProjectE2E abre el TUI real de un proyecto y comprueba
// que una segunda instancia para la misma carpeta no arranca: avisa y sale.
//
//	COMPASS_E2E=1 go test ./cmd/compass -run TestSingleInstancePerProjectE2E -v
func TestSingleInstancePerProjectE2E(t *testing.T) {
	if os.Getenv("COMPASS_E2E") != "1" {
		t.Skip("define COMPASS_E2E=1 para ejecutar la prueba de extremo a extremo")
	}

	exe := buildCompass(t)

	project := t.TempDir()
	files := map[string]string{
		"go.mod":  "module fixture\n\ngo 1.21\n",
		"main.go": "package main\n\nfunc main() {}\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(project, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Primera instancia: el TUI de verdad, dentro de una ConPTY.
	first, err := container.Spawn(fmt.Sprintf("& '%s' --path '%s'", exe, project), project, 40, 120, nil)
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer first.Close()
	if !waitForScreen(first, "DESARROLLO", 25*time.Second) {
		t.Fatalf("el menu no se dibujo. Pantalla:\n%s", first.Text())
	}

	// Segunda instancia: debe salir sola con el aviso, sin abrir ventana.
	second := exec.Command(exe, "--path", project)
	second.Dir = project
	out, err := second.CombinedOutput()
	if err != nil {
		t.Fatalf("la segunda instancia fallo: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "Ya hay un Compass Go abierto") {
		t.Fatalf("la segunda instancia no aviso de que ya hay una abierta:\n%s", out)
	}

	// Al cerrar la primera, el candado del proyecto queda libre.
	first.Write([]byte{0x1b})
	deadline := time.Now().Add(15 * time.Second)
	for !first.Exited() && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if !first.Exited() {
		t.Fatalf("la primera instancia no se cerro con Esc. Pantalla:\n%s", first.Text())
	}

	guard, free, err := lock.TryAcquire(project)
	if err != nil {
		t.Fatalf("no se pudo comprobar el candado: %v", err)
	}
	if !free {
		guard.Close()
		t.Fatal("el candado del proyecto sigue tomado tras cerrar la instancia")
	}
	guard.Close()
}

// buildCompass compila el binario real y devuelve su ruta.
func buildCompass(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(root, "compass-go.exe")
	build := exec.Command("go", "build", "-o", exe, "./cmd/compass")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("no se pudo compilar compass-go.exe: %v\n%s", err, out)
	}
	return exe
}

// waitForScreen espera a que la pantalla de la sesion contenga want.
func waitForScreen(s *container.Session, want string, timeout time.Duration) bool {
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
