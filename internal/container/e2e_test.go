//go:build windows

package container

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(root, "compass-go.exe")
	if _, err := os.Stat(exe); err != nil {
		build := exec.Command("go", "build", "-o", exe, "./cmd/compass")
		build.Dir = root
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("no se pudo compilar compass-go.exe: %v\n%s", err, out)
		}
	}

	project := t.TempDir()
	conf := `{"productName":"Fixture","version":"1.0.0"}`
	if err := os.WriteFile(filepath.Join(project, "tauri.conf.json"), []byte(conf), 0644); err != nil {
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
			s, err := Spawn(script, project, 32, 120)
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
