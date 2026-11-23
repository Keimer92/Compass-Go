package webui

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassifyRemoteFailure(t *testing.T) {
	err := errors.New("exit status 128")
	cases := []struct {
		name string
		out  string
		want string
	}{
		{"borrado", "remote: Repository not found.\nfatal: repository 'https://github.com/u/r.git/' not found", "missing"},
		{"credenciales", "fatal: could not read Username for 'https://github.com': terminal prompts disabled", "auth"},
		{"sin red", "fatal: unable to access 'https://github.com/u/r.git/': Could not resolve host: github.com", "unreachable"},
		{"desconocido", "fatal: algo raro", "error"},
	}
	for _, tc := range cases {
		if got, _ := classifyRemoteFailure(tc.out, err); got != tc.want {
			t.Errorf("%s: obtuve %q, queria %q", tc.name, got, tc.want)
		}
	}
}

func TestGitRemoteStatusLocal(t *testing.T) {
	dir := initRepo(t)
	b := NewBridge(dir)

	if st := b.GitRemoteStatus(""); st.State != "none" {
		t.Fatalf("sin remoto esperaba none: %+v", st)
	}

	bare := t.TempDir()
	cmd := exec.Command("git", "init", "--bare", "-q", bare)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v %s", err, out)
	}
	// Un remoto vacio existe pero no tiene HEAD: estado "empty", no "missing".
	if st := b.GitRemoteStatus(bare); st.State != "empty" {
		t.Fatalf("repo vacio esperaba empty: %+v", st)
	}

	missing := filepath.Join(t.TempDir(), "no-existe")
	if st := b.GitRemoteStatus(missing); st.State != "missing" {
		t.Fatalf("ruta inexistente esperaba missing: %+v", st)
	}

	if st := b.GitRemoteStatus("-oProxyCommand=calc"); st.State != "error" {
		t.Fatalf("URL con guion esperaba error: %+v", st)
	}
}

func TestGhRepoFromURL(t *testing.T) {
	cases := map[string][2]string{
		"https://github.com/Keimer92/School-Tracker":     {"Keimer92", "School-Tracker"},
		"https://github.com/Keimer92/School-Tracker.git": {"Keimer92", "School-Tracker"},
		"https://github.com/Keimer92/School-Tracker/":    {"Keimer92", "School-Tracker"},
		"git@github.com:Keimer92/School-Tracker.git":     {"Keimer92", "School-Tracker"},
		"https://github.com/u/repo/tree/main/src":        {"u", "repo"},
	}
	for url, want := range cases {
		m := ghRepoFromURLRe.FindStringSubmatch(url)
		if m == nil {
			t.Errorf("%s: no se extrajo owner/repo", url)
			continue
		}
		if m[1] != want[0] || m[2] != want[1] {
			t.Errorf("%s: obtuve %s/%s, queria %s/%s", url, m[1], m[2], want[0], want[1])
		}
	}
	if m := ghRepoFromURLRe.FindStringSubmatch("https://gitlab.com/u/r"); m != nil {
		t.Errorf("un URL que no es de GitHub no deberia coincidir: %v", m)
	}
}

// Un 404 de GitHub no puede confirmar que el repo este borrado: sin gh
// autenticado el estado debe ser "unverifiable", nunca "missing".
func TestRemote404IsNotReportedAsDeleted(t *testing.T) {
	state, detail := classifyRemoteFailure("remote: Repository not found.", errors.New("exit status 128"))
	if state != "missing" {
		t.Fatalf("clasificacion cruda esperaba missing: %q", state)
	}
	if strings.Contains(detail, "ya no existe") {
		t.Errorf("el detalle no debe afirmar que fue borrado: %q", detail)
	}
}

func TestGitCreateRemoteRejectsBadName(t *testing.T) {
	dir := initRepo(t)
	b := NewBridge(dir)
	// El nombre se valida antes de tocar `gh`, asi que no hace falta el CLI.
	if _, err := b.GitCreateRemote("-malo", true); err == nil {
		t.Fatal("un nombre con guion inicial no deberia aceptarse")
	}
	if _, err := b.GitCreateRemote("nombre con espacios", true); err == nil {
		t.Fatal("un nombre con espacios no deberia aceptarse")
	}
}
