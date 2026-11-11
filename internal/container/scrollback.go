package container

import (
	"bytes"
	"sync"

	"github.com/hinshun/vt10x"
)

// maxScrollback es el numero maximo de filas de historia conservadas por
// pestana. Cubre varias pantallas de salida de un build/debug sin consumir
// memoria sin ton.
const maxScrollback = 5000

// Scrollback conserva el texto que vt10x ya no muestra. El emulador solo
// tiene pantalla visible, asi que ademas de la pantalla viva (que el hijo ve
// con su altura real) mantenemos un terminal vt10x "alto" (maxScrollback
// filas) alimentado con la misma salida del hijo: sus filas inferiores son la
// pantalla viva y las superiores, la historia. El usuario sube a verlas y
// seleccionarlas.
//
// La pantalla viva se pinta desde el terminal real del hijo (su altura es la
// del area visible); la historia se pinta desde este terminal alto.
type Scrollback struct {
	mu sync.Mutex

	cols, rows int     // tamano de la pantalla viva (area visible)
	H          int     // altura del terminal alto (maxScrollback)
	back       vt10x.Terminal

	offset int // lineas subidas desde el fondo (0 = abajo, en vivo)
	used   int // numero aproximado de filas de contenido (limita el scroll)
}

// newScrollback crea un scrollback para una pantalla viva de rows x cols.
func newScrollback(cols, rows int) *Scrollback {
	H := maxScrollback
	return &Scrollback{
		cols: cols,
		rows: rows,
		H:    H,
		back: vt10x.New(vt10x.WithSize(cols, H)),
	}
}

// Feed entrega un trozo de salida al terminal alto (historia).
func (b *Scrollback) Feed(chunk []byte) {
	b.back.Write(chunk)
	b.mu.Lock()
	b.used += bytes.Count(chunk, []byte{'\n'})
	if b.used > b.H {
		b.used = b.H
	}
	b.mu.Unlock()
}

// Resize ajusta el ancho del terminal alto; la altura se mantiene para no
// perder la historia.
func (b *Scrollback) Resize(cols, rows int) {
	b.mu.Lock()
	b.cols, b.rows = cols, rows
	b.mu.Unlock()
	b.back.Resize(cols, b.H)
}

// docRows es el numero de filas con contenido que puede mostrar el scrollback.
func (b *Scrollback) docRows() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	d := b.rows
	if b.used > d {
		d = b.used
	}
	if d > b.H {
		d = b.H
	}
	return d
}

// DocRows expuesto para los tests.
func (b *Scrollback) DocRows() int { return b.docRows() }

// topBack es la primera fila (en el terminal alto) de la ventana visible.
func (b *Scrollback) topBack() int {
	b.mu.Lock()
	off := b.offset
	b.mu.Unlock()
	return b.H - b.docRows() + off
}

// visibleTop es la primera fila (en el terminal alto) de la ventana visible
// para un desplazamiento concreto: las filas inferiores son la pantalla viva y
// las superiores, la historia.
func (b *Scrollback) visibleTop(off int) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.H - b.rows - off
}

// Cell devuelve el glyph del terminal alto en (docX, backRow); vacio fuera.
func (b *Scrollback) Cell(docX, backRow int) vt10x.Glyph {
	if backRow < 0 || backRow >= b.H {
		return vt10x.Glyph{}
	}
	return b.back.Cell(docX, backRow)
}

// Cols es el ancho de la pantalla.
func (b *Scrollback) Cols() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cols
}

// Offset es el numero de lineas subidas desde el fondo.
func (b *Scrollback) Offset() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.offset
}

// AtBottom indica si se ve la pantalla viva (sin subir).
func (b *Scrollback) AtBottom() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.offset == 0
}

// Alt indica si el hijo esta en pantalla alterna (less, vim, ...): no se debe
// scrollbar ni mezclar con la historia normal.
func (b *Scrollback) Alt() bool {
	return b.back.Mode()&vt10x.ModeAltScreen != 0
}

// Scroll ajusta el desplazamiento (negativo sube, positivo baja). Devuelve
// true si el desplazamiento cambio.
func (b *Scrollback) Scroll(delta int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	max := b.docRows() - b.rows
	if max < 0 {
		max = 0
	}
	next := b.offset + delta
	if next < 0 {
		next = 0
	}
	if next > max {
		next = max
	}
	if next == b.offset {
		return false
	}
	b.offset = next
	return true
}
