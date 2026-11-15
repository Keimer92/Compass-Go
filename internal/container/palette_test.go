package container

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/hinshun/vt10x"
)

// TestColorOfTrueColor conserva el RGB que emite un hijo con truecolor: es el
// caso de los submenus, que pintan dentro de una pestana ConPTY. Si el RGB se
// perdiera, sus cards saldrian sin acentos ni fondos frente al menu principal.
func TestColorOfTrueColor(t *testing.T) {
	def := tcell.NewHexColor(0x16161e)
	for _, rgb := range []int32{0x1e1e24, 0x00ff88, 0xffffff, 0x102030} {
		got := colorOf(vt10x.Color(rgb), def)
		if got != tcell.NewHexColor(rgb) {
			t.Errorf("colorOf(0x%06x) = %v, queria %v", rgb, got, tcell.NewHexColor(rgb))
		}
	}
}

// TestStyleOfRoundTrip comprueba el camino completo ConPTY: un hijo tcell con
// truecolor emite 38;2/48;2, vt10x los parsea y styleOf los devuelve intactos.
func TestStyleOfRoundTrip(t *testing.T) {
	term := vt10x.New(vt10x.WithSize(10, 3))
	if _, err := term.Write([]byte("\x1b[38;2;255;0;128m\x1b[48;2;1;2;3mX")); err != nil {
		t.Fatalf("write: %v", err)
	}
	g := term.Cell(0, 0)
	if g.Char != 'X' {
		t.Fatalf("char = %q, queria X", g.Char)
	}
	st := styleOf(g, tcell.ColorWhite, tcell.ColorBlack)
	fg, bg, _ := st.Decompose()
	if fg != tcell.NewHexColor(0xff0080) {
		t.Errorf("fg = %v, queria 0xff0080", fg)
	}
	if bg != tcell.NewHexColor(0x010203) {
		t.Errorf("bg = %v, queria 0x010203", bg)
	}
}

// TestColorOfDefaults mantiene los casos base: default del tema, ANSI y xterm.
func TestColorOfDefaults(t *testing.T) {
	def := tcell.NewHexColor(0x16161e)
	if got := colorOf(vt10x.DefaultFG, def); got != def {
		t.Errorf("DefaultFG = %v, queria el del tema", got)
	}
	if got := colorOf(vt10x.DefaultBG, def); got != def {
		t.Errorf("DefaultBG = %v, queria el del tema", got)
	}
	if got := colorOf(vt10x.Color(1), def); got != ansi16[1] {
		t.Errorf("ANSI 1 = %v, queria %v", got, ansi16[1])
	}
	if got := colorOf(vt10x.Color(200), def); got != xterm256[200] {
		t.Errorf("xterm 200 = %v, queria %v", got, xterm256[200])
	}
}
