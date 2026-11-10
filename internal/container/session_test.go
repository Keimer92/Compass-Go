//go:build windows

package container

import (
	"strings"
	"testing"
	"time"
)

// waitFor sondea la condicion hasta 10 s (arrancar pwsh tarda).
func waitFor(t *testing.T, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func TestSessionRendersItsOutput(t *testing.T) {
	s, err := Spawn("Write-Output 'hola compass'; exit 0", t.TempDir(), 10, 60)
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer s.Close()

	if !waitFor(t, s.Exited) {
		t.Fatal("la sesion no termino")
	}

	dump := s.Text()
	if !strings.Contains(dump, "hola compass") {
		t.Fatalf("la pantalla no muestra la salida:\n%s", dump)
	}
}

func TestSessionPropagatesExitCode(t *testing.T) {
	s, err := Spawn("exit 95", t.TempDir(), 10, 60)
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer s.Close()

	if !waitFor(t, s.Exited) {
		t.Fatal("la sesion no termino")
	}
	if !s.CloseRequested() {
		t.Fatalf("se esperaba ExitCloseTab, code=%d", s.code.Load())
	}
}
