package window

import "testing"

func TestTitleMatches(t *testing.T) {
	cases := []struct {
		have, want string
		ok         bool
	}{
		{"MiProyecto", "MiProyecto", true},
		{"miproyecto", "MiProyecto", true},
		{" MiProyecto ", "MiProyecto", true},
		{"Administrator: MiProyecto", "MiProyecto", true},
		{"Administrador: MiProyecto", "MiProyecto", true},
		{"MiProyecto - pwsh", "MiProyecto", false},
		{"OtroProyecto", "MiProyecto", false},
		{"", "MiProyecto", false},
		{"MiProyecto", "", false},
	}
	for _, c := range cases {
		if got := titleMatches(c.have, c.want); got != c.ok {
			t.Errorf("titleMatches(%q, %q) = %v, queria %v", c.have, c.want, got, c.ok)
		}
	}
}
