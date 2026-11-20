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
	if _, err := copilotCommitMessage(context.Background(), "hola", ""); err == nil {
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

func TestCopilotModelArgs(t *testing.T) {
	cases := []struct {
		model string
		want  string
	}{
		{"", "--model auto"},
		{"auto-intelligence", "--model auto --auto-tier intelligence"},
		{"claude-haiku-4.5", "--model claude-haiku-4.5"},
		{"gemini-3.8-flash", "--model gemini-3.8-flash"},
		// Un id desconocido (o intento de inyeccion) cae en Auto.
		{"claude-inventado", "--model auto"},
		{"--yolo", "--model auto"},
	}
	for _, tc := range cases {
		got := strings.Join(copilotModelArgs(tc.model), " ")
		if got != tc.want {
			t.Fatalf("copilotModelArgs(%q) = %q, queria %q", tc.model, got, tc.want)
		}
	}
}

func TestCopilotModelsCatalog(t *testing.T) {
	opts := (&Bridge{}).CopilotModels()
	if len(opts) < 3 {
		t.Fatalf("el catalogo deberia ofrecer varias opciones, got %d", len(opts))
	}
	if opts[0].ID != "" {
		t.Fatalf("la primera opcion debe ser Auto (id vacio), got %q", opts[0].ID)
	}
	for _, o := range opts {
		if strings.TrimSpace(o.Label) == "" {
			t.Fatalf("opcion sin label: %+v", o)
		}
	}
}
