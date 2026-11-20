package webui

import (
	"context"
	"strings"
	"testing"
)

// Sin CLI instalado, el proveedor Copilot debe fallar con un mensaje util (y no
// colgarse intentando ejecutar algo inexistente).
func TestCopilotBinaryMissing(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	t.Setenv("PATHEXT", ".COM;.EXE;.BAT;.CMD")

	if _, _, err := copilotBinary(); err == nil {
		t.Fatal("sin CLI en PATH ni en LOCALAPPDATA debe devolver error")
	}
	if _, err := copilotCommitMessage(context.Background(), "hola"); err == nil {
		t.Fatal("copilotCommitMessage debe fallar si no hay CLI")
	} else if !strings.Contains(strings.ToLower(err.Error()), "copilot") {
		t.Fatalf("el error deberia nombrar a Copilot: %v", err)
	}
}

func TestCopilotAvailableWithoutCLI(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	t.Setenv("PATHEXT", ".COM;.EXE;.BAT;.CMD")

	b := &Bridge{}
	if b.CopilotAvailable() {
		t.Fatal("CopilotAvailable debe ser false sin CLI")
	}
}
