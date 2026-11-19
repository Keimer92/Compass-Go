package tasks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
