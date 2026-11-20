//go:build windows

package webui

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"compass/internal/container"
)

// TestTaskOutputReachesTapE2E comprueba el camino completo del terminal web:
// Compass se reinvoca con --task dentro de una ConPTY, engancha la consola
// padre y su salida llega al tap que alimenta pty:data.
//
//	COMPASS_E2E=1 go test ./internal/webui -run TestTaskOutputReachesTapE2E -v
func TestTaskOutputReachesTapE2E(t *testing.T) {
	if os.Getenv("COMPASS_E2E") != "1" {
		t.Skip("define COMPASS_E2E=1 para ejecutar la prueba de extremo a extremo")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	exe := buildWebBinary(t)
	// Igual que dist.bat: subsistema GUI, para probar AttachParentConsole.
	build := exec.Command("go", "build", "-ldflags=-H=windowsgui", "-o", exe, "./cmd/compass-web")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	proj := t.TempDir()
	if err := os.WriteFile(filepath.Join(proj, "go.mod"), []byte("module demo\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var got strings.Builder
	script := fmt.Sprintf("& '%s' --task version --path '%s'; exit $LASTEXITCODE", exe, proj)
	sess, err := container.SpawnTap(script, proj, 30, 100, nil, func(b []byte) {
		mu.Lock()
		got.Write(b)
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("SpawnTap: %v", err)
	}
	defer sess.Close()

	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		text := got.String()
		mu.Unlock()
		if strings.Contains(text, "VERSIONADOR") {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	t.Fatalf("la salida de la tarea no llego al tap; recibido:\n%s", got.String())
}

// TestOpenTerminalUsesClientID comprueba que la sesion usa el id que elige el
// frontend desde el primer byte: es lo que permite casar `pty:data` antes de
// que la promesa de OpenTerminal resuelva.
//
//	COMPASS_E2E=1 go test ./internal/webui -run TestOpenTerminalUsesClientID -v
func TestOpenTerminalUsesClientID(t *testing.T) {
	if os.Getenv("COMPASS_E2E") != "1" {
		t.Skip("define COMPASS_E2E=1 para ejecutar la prueba de extremo a extremo")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	exe := buildWebBinary(t)
	build := exec.Command("go", "build", "-ldflags=-H=windowsgui", "-o", exe, "./cmd/compass-web")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	// Aislar el workspace persistido (no apilar proyectos de prueba en el
	// %LOCALAPPDATA% real).
	t.Setenv("LOCALAPPDATA", t.TempDir())
	// La carpeta del proyecto no puede ser t.TempDir: la ConPTY mantiene el cwd
	// y el cleanup fallaria al borrarla.
	proj, err := os.MkdirTemp("", "compass-e2e-proj")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(proj) })
	t.Chdir(proj) // NewBridge hace os.Chdir
	if err := os.WriteFile(filepath.Join(proj, "go.mod"), []byte("module demo\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var events []map[string]any
	emitHook = func(name string, data any) {
		if name != "pty:data" && name != "pty:exit" {
			return
		}
		m, _ := data.(map[string]any)
		mu.Lock()
		events = append(events, m)
		mu.Unlock()
	}
	defer func() { emitHook = nil }()

	b := NewBridge(proj)
	script := fmt.Sprintf("& '%s' --task version --path '%s'; exit $LASTEXITCODE", exe, proj)
	id := b.OpenTerminal("tab-7", "Version", script, 30, 100)
	if id != "tab-7" {
		t.Fatalf("OpenTerminal devolvio %q, esperaba el id del cliente", id)
	}
	defer b.CloseTerminal(id)

	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		var acc strings.Builder
		for _, e := range events {
			if s, ok := e["data"].(string); ok {
				raw, _ := base64.StdEncoding.DecodeString(s)
				acc.Write(raw)
			}
		}
		done := strings.Contains(acc.String(), "VERSIONADOR")
		mu.Unlock()
		if done {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) < 2 {
		t.Fatalf("no llegaron eventos (recibidos %d)", len(events))
	}
	var text strings.Builder
	for _, e := range events {
		if e["id"] != "tab-7" {
			t.Fatalf("evento con id %v, esperaba tab-7", e["id"])
		}
		if s, ok := e["data"].(string); ok {
			raw, err := base64.StdEncoding.DecodeString(s)
			if err != nil {
				t.Fatalf("base64 invalido: %v", err)
			}
			text.Write(raw)
		}
	}
	if !strings.Contains(text.String(), "VERSIONADOR") {
		t.Fatalf("la salida no llego con el id del cliente:\n%s", text.String())
	}
}

// buildWebBinary compila el binario real con subsistema GUI en una carpeta
// temporal que no sea la del test (Windows tarda en soltar el .exe y el
// cleanup de t.TempDir fallaria).
func buildWebBinary(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp("", "compass-e2e")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		time.Sleep(500 * time.Millisecond)
		_ = os.RemoveAll(dir)
	})
	exe := filepath.Join(dir, "compass-web.exe")
	build := exec.Command("go", "build", "-ldflags=-H=windowsgui", "-o", exe, "./cmd/compass-web")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return exe
}
