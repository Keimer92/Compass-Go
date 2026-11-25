package tasks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"compass/internal/settings"
)

func TestFallbackCandidatesIncludesNpmGlobalShims(t *testing.T) {
	appData := t.TempDir()
	t.Setenv("APPDATA", appData)

	candidates := fallbackCandidates(vibeTools[0])
	want := filepath.Join(appData, "npm", "opencode.cmd")
	for _, candidate := range candidates {
		if candidate == want {
			return
		}
	}
	t.Fatalf("fallbackCandidates() no incluyó %q: %v", want, candidates)
}

func TestGitignoreNotifyScriptHasNoInnerQuotes(t *testing.T) {
	// El script se inyecta en `-Command "..."` y cmd.exe no escapa comillas
	// internas: cualquier `"` literal rompe la tarea (no creaba el .gitignore).
	script := gitignoreNotifyScript(`C:\apps\compass-go.exe`, "%1")
	if strings.Contains(script, `"`) {
		t.Fatalf("el script contiene comillas dobles literales:\n%s", script)
	}
	if !strings.Contains(script, "[char]34") {
		t.Fatalf("el script no construye la comilla con [char]34:\n%s", script)
	}
}

func TestGitignoreContextCommandQuotesPath(t *testing.T) {
	cmd := gitignoreContextCommand("pwsh.exe", `C:\apps\compass-go.exe`, "%V")
	if !strings.Contains(cmd, `--task context-gitignore`) {
		t.Fatalf("el comando no invoca la tarea:\n%s", cmd)
	}
	// Un solo par de comillas externas (-Command "..."); el resto va en el script.
	if strings.Count(cmd, `-Command "`) != 1 {
		t.Fatalf("comando mal formado:\n%s", cmd)
	}
}

func TestPreferCmdShimForPowerShellScript(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "opencode.ps1")
	cmdShim := filepath.Join(dir, "opencode.cmd")
	for _, path := range []string{script, cmdShim} {
		if err := os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
	}

	if got := preferCmdShim(script); got != cmdShim {
		t.Fatalf("preferCmdShim(%q) = %q, want %q", script, got, cmdShim)
	}
}

// Las CLIs deshabilitadas en Opciones no deben entrar en la cascada del
// Explorador. Con todas deshabilitadas la resolucion no debe buscar nada.
func TestResolveVibeToolsSkipsDisabled(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	disabled := make([]string, 0, len(vibeTools))
	for _, tool := range vibeTools {
		disabled = append(disabled, tool.key)
	}
	settings.Save(settings.Settings{VibeDisabled: disabled})

	if got := resolveVibeTools(); len(got) != 0 {
		t.Fatalf("resolveVibeTools = %d herramientas, queria 0", len(got))
	}
}

// El script de la cascada solo debe crear las sub-entradas recibidas: asi el
// filtrado de resolveVibeTools se traslada al registro.
func TestVibeCtxScriptOnlyWritesGivenTools(t *testing.T) {
	got := vibeCtxScript([]resolvedVibeTool{
		{tool: vibeTools[0], cmd: "'opencode.cmd' --auto"},
	}, "C:\\icons\\VibeCoding.ico")

	if !strings.Contains(got, vibeTools[0].key) {
		t.Errorf("el script no menciona la CLI incluida %q", vibeTools[0].key)
	}
	if strings.Contains(got, vibeTools[1].key) {
		t.Errorf("el script menciona la CLI excluida %q", vibeTools[1].key)
	}
}
