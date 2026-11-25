package webui

import "testing"

// Los nombres de cuenta se usan como argumento de `git credential-manager`:
// la validacion debe cortar cualquier cosa rara antes de ejecutar nada.
func TestGitHubAuthRejectsInvalidNames(t *testing.T) {
	b := NewBridge(t.TempDir())

	// Nombres invalidos: la validacion falla antes de invocar a GCM.
	for _, name := range []string{"", "   ", "user name", "user;calc", "a/b", "-flag", "user@host"} {
		if _, err := b.GitHubLogin(name); err == nil {
			t.Errorf("GitHubLogin(%q) no fallo", name)
		}
		if _, err := b.GitHubLogout(name); err == nil {
			t.Errorf("GitHubLogout(%q) no fallo", name)
		}
	}
}

func TestGitHubAccountPattern(t *testing.T) {
	valid := []string{"Keimer92", "keimer-92", "a", "ABC123"}
	for _, name := range valid {
		if !ghAccountRe.MatchString(name) {
			t.Errorf("%q deberia ser valido", name)
		}
	}
	invalid := []string{"", "a b", "user;rm", "a/b", "-x", "x_", "toolongusernamethatisover39chars0123456789"}
	for _, name := range invalid {
		if ghAccountRe.MatchString(name) {
			t.Errorf("%q no deberia ser valido", name)
		}
	}
}
