// Package gui es la capa visual nativa (Gio) de Compass Go.
package gui

import (
	"image"
	"image/color"
	"os"
	"strings"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/font/opentype"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"github.com/hinshun/vt10x"
	"golang.org/x/image/math/fixed"
)

// Atributos de vt10x (mismos bits que container/palette.go).
const (
	attrReverse   = 1 << 0
	attrUnderline = 1 << 1
	attrBold      = 1 << 2
	attrItalic    = 1 << 4
)

// ansi16 es la paleta xterm clasica (duplicada aqui para no arrastrar tcell).
var ansi16 = [16]color.NRGBA{
	{0x00, 0x00, 0x00, 0xff}, {0x80, 0x00, 0x00, 0xff},
	{0x00, 0x80, 0x00, 0xff}, {0x80, 0x80, 0x00, 0xff},
	{0x00, 0x00, 0x80, 0xff}, {0x80, 0x00, 0x80, 0xff},
	{0x00, 0x80, 0x80, 0xff}, {0xc0, 0xc0, 0xc0, 0xff},
	{0x80, 0x80, 0x80, 0xff}, {0xff, 0x00, 0x00, 0xff},
	{0x00, 0xff, 0x00, 0xff}, {0xff, 0xff, 0x00, 0xff},
	{0x00, 0x00, 0xff, 0xff}, {0xff, 0x00, 0xff, 0xff},
	{0x00, 0xff, 0xff, 0xff}, {0xff, 0xff, 0xff, 0xff},
}

// colorOf traduce un color de vt10x a Gio con la misma logica que
// container/palette.go (los "default" usan el color del tema).
func colorOf(c vt10x.Color, def color.NRGBA) color.NRGBA {
	switch {
	case c == vt10x.DefaultFG || c == vt10x.DefaultBG || c == vt10x.DefaultCursor:
		return def
	case c < 16:
		return ansi16[c]
	case c < 256:
		levels := [6]int{0, 95, 135, 175, 215, 255}
		if c < 232 {
			n := int(c) - 16
			return color.NRGBA{
				R: uint8(levels[n/36]), G: uint8(levels[(n%36)/6]),
				B: uint8(levels[n%6]), A: 0xff,
			}
		}
		v := uint8(8 + (int(c) - 232) * 10)
		return color.NRGBA{R: v, G: v, B: v, A: 0xff}
	case c < 1<<24:
		return color.NRGBA{
			R: uint8(c >> 16), G: uint8(c >> 8), B: uint8(c), A: 0xff,
		}
	default:
		return def
	}
}

// cell es una copia de un glifo de vt10x para poder soltar su candado antes de
// pintar (las operaciones de Gio no deben hacerse con el terminal bloqueado).
type cell struct {
	ch     rune
	fg, bg vt10x.Color
	mode   int16
}

// Terminal pinta la pantalla visible o la historia de una pestana.
type Terminal struct {
	shaper  *text.Shaper
	font    font.Font
	size    unit.Sp
	palette *Palette

	cw, ch int
	cols   int
	rows   int

	glyphCache map[rune][]text.Glyph
}

// NewTerminal crea el widget con fuente monoespaciada y fallback de emoji.
func NewTerminal(p *Palette) *Terminal {
	var emoji []font.FontFace
	if data, err := os.ReadFile(`C:\Windows\Fonts\seguiemj.ttf`); err == nil {
		if faces, err := opentype.ParseCollection(data); err == nil {
			emoji = faces
		}
	}
	return &Terminal{
		shaper:     text.NewShaper(text.WithCollection(emoji)),
		font:       font.Font{Typeface: "Consolas, Segoe UI Emoji"},
		size:       14,
		glyphCache: make(map[rune][]text.Glyph),
		palette:    p,
	}
}

// Metrics devuelve el tamano de celda y la geometria de la ultima pestana.
func (t *Terminal) Metrics() (cw, ch, cols, rows int) {
	return t.cw, t.ch, t.cols, t.rows
}

func (t *Terminal) metrics(gtx layout.Context) (int, int) {
	if t.cw > 0 {
		return t.cw, t.ch
	}
	px := fixed.I(gtx.Sp(t.size))
	t.shaper.LayoutString(text.Parameters{
		Font: t.font, PxPerEm: px, MaxWidth: 1 << 20,
	}, "M")
	if g, ok := t.shaper.NextGlyph(); ok {
		t.cw = g.Advance.Ceil()
		t.ch = (g.Ascent + g.Descent).Ceil()
	}
	if t.cw < 1 {
		t.cw = 8
	}
	if t.ch < 1 {
		t.ch = 16
	}
	return t.cw, t.ch
}

func (t *Terminal) glyphsFor(px fixed.Int26_6, r rune) []text.Glyph {
	if g, ok := t.glyphCache[r]; ok {
		return g
	}
	t.shaper.LayoutString(text.Parameters{
		Font: t.font, PxPerEm: px, MaxWidth: 1 << 20, DisableSpaceTrim: true,
	}, string(r))
	var out []text.Glyph
	for g, ok := t.shaper.NextGlyph(); ok; g, ok = t.shaper.NextGlyph() {
		out = append(out, g)
	}
	t.glyphCache[r] = out
	return out
}

// Layout dibuja el terminal de p dentro de gtx.Constraints.Max.
func (t *Terminal) Layout(gtx layout.Context, p *Pane, pal *Palette) layout.Dimensions {
	dims := layout.Dimensions{Size: gtx.Constraints.Max}
	area := image.Rectangle{Max: dims.Size}
	defFG, defBG := pal.Text, pal.Background

	// El clip va antes del fill: si no, PaintOp pinta toda la ventana.
	clipArea := clip.Rect(area).Push(gtx.Ops)
	defer clipArea.Pop()
	paint.Fill(gtx.Ops, defBG)

	// Registrar el tag para que Gio nos entregue rueda y arrastre.
	event.Op(gtx.Ops, &p.tag)

	cw, ch := t.metrics(gtx)
	cols, rows := area.Max.X/cw, area.Max.Y/ch
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	// Llamar siempre (Session.Resize ya descarta tamaños iguales): así la
	// ConPTY acaba convergiendo aunque el primer frame llegue tarde y el TUI
	// hijo no pierde filas superiores bajo la barra de pestañas.
	t.cols, t.rows = cols, rows
	p.Session.Resize(cols, rows)
	if p.Back != nil {
		p.Back.Resize(cols, rows)
	}

	lines, curX, curY, curVisible := t.snapshot(p, cols, rows)
	px := fixed.I(gtx.Sp(t.size))

	for y, line := range lines {
		if y >= rows {
			break
		}
		for x, c0 := range line {
			if x >= cols || c0.ch == 0 || c0.ch < 32 {
				continue
			}
			fg := colorOf(c0.fg, defFG)
			bg := colorOf(c0.bg, defBG)
			if c0.mode&attrReverse != 0 {
				fg, bg = bg, fg
			}
			if p.Sel.contains(x, y) {
				bg = blend(pal.Accent, defBG, 0.55)
			}
			if bg != defBG {
				paintCell(gtx, x*cw, y*ch, cw, ch, bg)
			}
			glyphs := t.glyphsFor(px, c0.ch)
			if len(glyphs) == 0 {
				continue
			}
			first := glyphs[0]
			off := f32.Pt(
				float32(x*cw)-fixedToFloat(first.X),
				float32(y*ch+first.Ascent.Ceil())-float32(first.Y),
			)
			flushGlyphs(gtx, t.shaper, glyphs, fg, off)
		}
	}

	if curVisible && curX < cols && curY < rows && !p.Sel.active {
		paintCell(gtx, curX*cw, curY*ch, cw, ch,
			color.NRGBA{R: 0x9a, G: 0xa8, B: 0xc0, A: 0x99})
	}
	return dims
}

// snapshot devuelve la rejilla a pintar: la pantalla viva o la historia.
func (t *Terminal) snapshot(p *Pane, cols, rows int) ([][]cell, int, int, bool) {
	if p.Back != nil && !p.Back.AtBottom() && !p.Back.Alt() {
		top := p.Back.ViewTop()
		lines := make([][]cell, rows)
		for y := 0; y < rows; y++ {
			lines[y] = make([]cell, cols)
			for x := 0; x < cols; x++ {
				g := p.Back.Cell(x, top+y)
				lines[y][x] = cell{ch: g.Char, fg: g.FG, bg: g.BG, mode: g.Mode}
			}
		}
		return lines, -1, -1, false
	}

	var lines [][]cell
	var cx, cy int
	var vis bool
	p.Session.WithScreen(func(v vt10x.View) {
		vcols, vrows := v.Size()
		lines = make([][]cell, vrows)
		for y := 0; y < vrows; y++ {
			lines[y] = make([]cell, vcols)
			for x := 0; x < vcols; x++ {
				g := v.Cell(x, y)
				lines[y][x] = cell{ch: g.Char, fg: g.FG, bg: g.BG, mode: g.Mode}
			}
		}
		c := v.Cursor()
		cx, cy, vis = c.X, c.Y, v.CursorVisible()
	})
	return lines, cx, cy, vis
}

// selectionText extrae el texto seleccionado de la historia (o pantalla viva).
func selectionText(p *Pane, cols, rows int) string {
	c0, r0, c1, r1 := p.Sel.bounds()
	top := 0
	if p.Back != nil {
		top = p.Back.ViewTop()
	}
	var out strings.Builder
	for row := r0; row <= r1; row++ {
		var line strings.Builder
		from, to := 0, cols-1
		if row == r0 {
			from = c0
		}
		if row == r1 {
			to = c1
		}
		for col := from; col <= to; col++ {
			var g vt10x.Glyph
			if p.Back != nil && !p.Back.AtBottom() {
				g = p.Back.Cell(col, top+row)
			} else {
				p.Session.WithScreen(func(v vt10x.View) {
					if col < cols && row < rows {
						g = v.Cell(col, row)
					}
				})
			}
			ch := g.Char
			if ch == 0 {
				ch = ' '
			}
			line.WriteRune(ch)
		}
		out.WriteString(strings.TrimRight(line.String(), " "))
		if row < r1 {
			out.WriteString("\r\n")
		}
	}
	return out.String()
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func paintCell(gtx layout.Context, x, y, w, h int, c color.NRGBA) {
	stack := clip.Rect(image.Rect(x, y, x+w, y+h)).Push(gtx.Ops)
	paint.Fill(gtx.Ops, c)
	stack.Pop()
}

func flushGlyphs(gtx layout.Context, sh *text.Shaper, glyphs []text.Glyph, col color.NRGBA, off f32.Point) {
	trans := op.Affine(f32.AffineId().Offset(off)).Push(gtx.Ops)
	outline := clip.Outline{Path: sh.Shape(glyphs)}.Op().Push(gtx.Ops)
	paint.ColorOp{Color: col}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	outline.Pop()
	if call := sh.Bitmaps(glyphs); call != (op.CallOp{}) {
		call.Add(gtx.Ops)
	}
	trans.Pop()
}

func fixedToFloat(i fixed.Int26_6) float32 { return float32(i) / 64.0 }

// encodeKey traduce un EventKey de Gio a bytes de terminal, con las mismas
// secuencias xterm que container/encode.go usa para tcell.
func encodeKey(e key.Event) []byte {
	if e.State != key.Press {
		return nil
	}
	switch e.Name {
	case key.NameReturn, key.NameEnter:
		return []byte{'\r'}
	case key.NameDeleteBackward:
		return []byte{0x7f}
	case key.NameTab:
		return []byte{'\t'}
	case key.NameSpace:
		return []byte{' '}
	case key.NameEscape:
		return []byte{0x1b}
	case key.NameUpArrow:
		return []byte("\x1b[A")
	case key.NameDownArrow:
		return []byte("\x1b[B")
	case key.NameRightArrow:
		return []byte("\x1b[C")
	case key.NameLeftArrow:
		return []byte("\x1b[D")
	case key.NameHome:
		return []byte("\x1b[H")
	case key.NameEnd:
		return []byte("\x1b[F")
	case key.NameDeleteForward:
		return []byte("\x1b[3~")
	case key.NamePageUp:
		return []byte("\x1b[5~")
	case key.NamePageDown:
		return []byte("\x1b[6~")
	}
	if len(e.Name) == 1 {
		r := []rune(e.Name)[0]
		if r >= 'A' && r <= 'Z' && e.Modifiers&key.ModShift == 0 && e.Modifiers&key.ModCtrl == 0 {
			r += 'a' - 'A'
		}
		if e.Modifiers&key.ModCtrl != 0 && r >= 'A' && r <= 'Z' {
			return []byte{byte(r - 'A' + 1)}
		}
		if e.Modifiers&key.ModCtrl != 0 && r >= 'a' && r <= 'z' {
			return []byte{byte(r - 'a' + 1)}
		}
		return []byte(string(r))
	}
	return nil
}
