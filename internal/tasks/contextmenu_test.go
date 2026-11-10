package tasks

import (
	"os"
	"path/filepath"
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
