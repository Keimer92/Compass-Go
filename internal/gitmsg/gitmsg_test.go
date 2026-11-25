package gitmsg

import (
	"strings"
	"testing"
)

func TestExplainPushDeniedNamesAccountAndRepo(t *testing.T) {
	out := `remote: Permission to Keimer92/Compass-Go.git denied to Keimer92-C5.
fatal: unable to access 'https://github.com/Keimer92/Compass-Go/': The requested URL returned error: 403`
	got := ExplainPush(out)
	if got == "" {
		t.Fatal("no reconocio el error de permisos")
	}
	for _, want := range []string{`"Keimer92-C5"`, `"Keimer92/Compass-Go"`, "login --username Keimer92", "logout Keimer92-C5"} {
		if !strings.Contains(got, want) {
			t.Errorf("falta %q en el mensaje:\n%s", want, got)
		}
	}
}

func TestExplainPushForbiddenWithoutAccount(t *testing.T) {
	out := "fatal: unable to access 'https://github.com/org/repo/': The requested URL returned error: 403"
	got := ExplainPush(out)
	if got == "" {
		t.Fatal("no reconocio el 403")
	}
	if !strings.Contains(got, "github list") || !strings.Contains(got, "login --username") {
		t.Errorf("faltan los comandos de GCM:\n%s", got)
	}
}

func TestExplainPushIgnoresOtherErrors(t *testing.T) {
	for _, out := range []string{
		"",
		"fatal: no upstream configured for branch 'main'",
		"error: failed to push some refs (non-fast-forward)",
	} {
		if got := ExplainPush(out); got != "" {
			t.Errorf("ExplainPush(%q) = %q, queria vacio", out, got)
		}
	}
}
