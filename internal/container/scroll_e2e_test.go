//go:build windows

package container

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestScrollWheelE2E comprueba que la rueda sube la historia de una pestana de
// terminal y que el pie lo refleja.
//
//	COMPASS_E2E=1 go test ./internal/container -run TestScrollWheelE2E -v
func TestScrollWheelE2E(t *testing.T) {
	if os.Getenv("COMPASS_E2E") != "1" {
		t.Skip("define COMPASS_E2E=1 para ejecutar la prueba de extremo a extremo")
	}

	exe := compassExe(t)

	project := t.TempDir()
	main := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfor i := 0; i < 200; i++ {\n\t\tfmt.Printf(\"LINEA %03d\\n\", i)\n\t}\n}\n"
	files := map[string]string{
		"go.mod":  "module fixture\n\ngo 1.21\n",
		"main.go": main,
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
	mouseClick(s, 10, 8) // card Run
	if !waitForText(s, "LINEA 199", 25*time.Second) {
		t.Fatalf("la tarea no imprimio toda la salida. Pantalla:\n%s", s.Text())
	}

	// Rueda arriba sobre el cuerpo del terminal (SGR 1006, boton 64).
	s.Write([]byte("\x1b[<64;21;11M"))
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && !screenHas(s, "↑ 3 lineas") {
		time.Sleep(100 * time.Millisecond)
	}
	if !screenHas(s, "↑ 3 lineas") {
		t.Fatalf("la rueda no subio la historia. Pantalla:\n%s", s.Text())
	}

	// Tres pasos de rueda abajo vuelven al fondo.
	for i := 0; i < 3; i++ {
		s.Write([]byte("\x1b[<65;21;11M"))
		time.Sleep(150 * time.Millisecond)
	}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && screenHas(s, "lineas") {
		time.Sleep(100 * time.Millisecond)
	}
	if screenHas(s, "lineas") {
		t.Fatalf("la rueda no volvio al fondo. Pantalla:\n%s", s.Text())
	}
}

func screenHas(s *Session, want string) bool {
	return strings.Contains(s.Text(), want)
}
