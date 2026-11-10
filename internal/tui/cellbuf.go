package tui

import "github.com/gdamore/tcell/v2"

// cellSink acepta celdas de pantalla. Lo implementan tcell.Screen y cellBuf.
type cellSink interface {
	SetContent(x, y int, mainc rune, combc []rune, style tcell.Style)
}

// cell es una celda ya pintada.
type cell struct {
	ch   rune
	comb []rune
	st   tcell.Style
}

// cellBuf es un lienzo de celdas con coordenadas absolutas.
//
// tcell no deja leer una celda ya pintada, y el halo necesita mezclar su brillo
// con el color que ya tenia cada celda. La card se pinta primero aqui y se
// vuelca a la pantalla despues de aplicar el halo, igual que el buffer de
// ratatui en la version Rust.
type cellBuf struct {
	x, y, w, h int
	cells      []cell
}

// newCellBuf crea un lienzo para `r` relleno de espacios.
func newCellBuf(r Rect) *cellBuf {
	b := &cellBuf{x: r.X, y: r.Y, w: r.Width, h: r.Height}
	b.cells = make([]cell, r.Width*r.Height)
	for i := range b.cells {
		b.cells[i] = cell{ch: ' ', st: tcell.StyleDefault}
	}
	return b
}

// SetContent pinta una celda; las coordenadas fuera del lienzo se ignoran.
func (b *cellBuf) SetContent(x, y int, ch rune, comb []rune, st tcell.Style) {
	dx, dy := x-b.x, y-b.y
	if dx < 0 || dx >= b.w || dy < 0 || dy >= b.h {
		return
	}
	b.cells[dy*b.w+dx] = cell{ch: ch, comb: comb, st: st}
}

// at devuelve la celda en coordenadas absolutas, o nil si esta fuera.
func (b *cellBuf) at(x, y int) *cell {
	dx, dy := x-b.x, y-b.y
	if dx < 0 || dx >= b.w || dy < 0 || dy >= b.h {
		return nil
	}
	return &b.cells[dy*b.w+dx]
}

// blit vuelca el lienzo a la pantalla.
func (b *cellBuf) blit(s tcell.Screen) {
	for dy := 0; dy < b.h; dy++ {
		for dx := 0; dx < b.w; dx++ {
			c := b.cells[dy*b.w+dx]
			s.SetContent(b.x+dx, b.y+dy, c.ch, c.comb, c.st)
			// Una runa ancha ocupa dos celdas y tcell rellena la segunda.
			if unisegWidth(string(c.ch)) == 2 {
				dx++
			}
		}
	}
}
