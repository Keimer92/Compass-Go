package container

import (
	"github.com/gdamore/tcell/v2"
	"github.com/hinshun/vt10x"
)

// ansi16 es la paleta xterm clasica.
var ansi16 = [16]tcell.Color{
	tcell.NewHexColor(0x000000),
	tcell.NewHexColor(0x800000),
	tcell.NewHexColor(0x008000),
	tcell.NewHexColor(0x808000),
	tcell.NewHexColor(0x000080),
	tcell.NewHexColor(0x800080),
	tcell.NewHexColor(0x008080),
	tcell.NewHexColor(0xc0c0c0),
	tcell.NewHexColor(0x808080),
	tcell.NewHexColor(0xff0000),
	tcell.NewHexColor(0x00ff00),
	tcell.NewHexColor(0xffff00),
	tcell.NewHexColor(0x0000ff),
	tcell.NewHexColor(0xff00ff),
	tcell.NewHexColor(0x00ffff),
	tcell.NewHexColor(0xffffff),
}

// xterm256 son los 256 colores del terminal: 16 ANSI, el cubo 6x6x6 y la rampa
// de grises.
var xterm256 [256]tcell.Color

func init() {
	copy(xterm256[:16], ansi16[:])
	levels := [6]int{0, 95, 135, 175, 215, 255}
	for i := 16; i < 232; i++ {
		n := i - 16
		r := levels[n/36]
		g := levels[(n%36)/6]
		b := levels[n%6]
		xterm256[i] = tcell.NewHexColor(int32(r<<16 | g<<8 | b))
	}
	for i := 232; i < 256; i++ {
		v := 8 + (i-232)*10
		xterm256[i] = tcell.NewHexColor(int32(v<<16 | v<<8 | v))
	}
}

// colorOf traduce un color de vt10x a tcell; los "default" usan el del tema.
func colorOf(c vt10x.Color, def tcell.Color) tcell.Color {
	switch {
	case c == vt10x.DefaultFG || c == vt10x.DefaultBG || c == vt10x.DefaultCursor:
		return def
	case c < 16:
		return ansi16[c]
	case c < 256:
		return xterm256[c]
	default:
		return def
	}
}

// Atributos del glifo de vt10x (bits de Glyph.Mode).
const (
	attrReverse   = 1 << 0
	attrUnderline = 1 << 1
	attrBold      = 1 << 2
	attrGfx       = 1 << 3
	attrItalic    = 1 << 4
	attrBlink     = 1 << 5
	attrWrap      = 1 << 6
)

// styleOf convierte un glifo en un estilo de tcell.
func styleOf(g vt10x.Glyph, defFG, defBG tcell.Color) tcell.Style {
	fg := colorOf(g.FG, defFG)
	bg := colorOf(g.BG, defBG)
	if g.Mode&attrReverse != 0 {
		fg, bg = bg, fg
	}
	st := tcell.StyleDefault.Foreground(fg).Background(bg)
	if g.Mode&attrBold != 0 {
		st = st.Bold(true)
	}
	if g.Mode&attrUnderline != 0 {
		st = st.Underline(true)
	}
	if g.Mode&attrItalic != 0 {
		st = st.Italic(true)
	}
	if g.Mode&attrBlink != 0 {
		st = st.Blink(true)
	}
	return st
}
