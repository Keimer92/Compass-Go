package container

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

// TestTabAccentStablePerProject comprueba que el mismo proyecto conserva su
// color y que dos rutas equivalentes (mayusculas/distintas) colisionan, como
// tab_accent_is_stable_per_project de Rust.
func TestTabAccentStablePerProject(t *testing.T) {
	a := TabAccent("C:\\Proyectos\\foo")
	b := TabAccent("c:\\proyectos\\foo")
	c := TabAccent("C:\\Proyectos\\bar")
	if a != b {
		t.Errorf("rutas equivalentes dan colores distintos: %v != %v", a, b)
	}
	if a == c {
		t.Errorf("proyectos distintos no deberian colisionar siempre: %v", a)
	}
}

// TestTabAccentVisibleSaturation replica tab_accent_has_visible_saturation:
// ningun tono de la rueda debe salir gris (r==g==b).
func TestTabAccentVisibleSaturation(t *testing.T) {
	for i := 0; i < int(tabTones); i++ {
		project := string(rune('a' + i)) // hash distinto por nombre
		c := TabAccent(project)
		r, g, bl := c.RGB()
		if r == g && g == bl {
			t.Errorf("el proyecto %q da gris (%d,%d,%d)", project, r, g, bl)
		}
	}
}

// TestTabAccentTonesSpread verifica que los 12 tonos cubren el espectro: al
// menos hay varios colores distintos entre los proyectos de una letra.
func TestTabAccentTonesSpread(t *testing.T) {
	seen := map[tcell.Color]bool{}
	for i := 0; i < 26; i++ {
		seen[TabAccent(string(rune('a' + i)))] = true
	}
	if len(seen) < 8 {
		t.Errorf("se esperaban muchos tonos distintos, hubo %d", len(seen))
	}
}
