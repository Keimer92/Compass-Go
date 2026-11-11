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
//
// El viewport se ancla por fila absoluta del documento (top): al recibir
// salida nueva mientras se esta subido, la vista no se mueve y el nuevo
// contenido rueda por debajo; al estar abajo, la vista sigue el fondo.
type Scrollback struct {
	mu sync.Mutex

	cols, rows int // tamano de la pantalla viva (area visible)
	H          int // altura del terminal alto (maxScrollback)
	back       vt10x.Terminal

	used int // numero aproximado de filas de contenido (limita el scroll)
	top  int // fila absoluta del documento en la primera linea visible
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
	b.mu.Lock()
	oldTop := b.contentTopLocked()
	b.back.Write(chunk)
	b.used += bytes.Count(chunk, []byte{'\n'})
	if b.used > b.H {
		b.used = b.H
	}
	newTop := b.contentTopLocked()
	// Si estabamos abajo (o mas abajo aun), seguimos el fondo; si estabamos
	// subidos, la vista queda anclada a las mismas filas.
	if b.top >= oldTop {
		b.top = newTop
	}
	b.mu.Unlock()
}

// Resize ajusta el ancho del terminal alto; la altura se mantiene para no
// perder la historia.
func (b *Scrollback) Resize(cols, rows int) {
	b.mu.Lock()
	oldTop := b.contentTopLocked()
	b.cols, b.rows = cols, rows
	b.back.Resize(cols, b.H)
	newTop := b.contentTopLocked()
	if b.top >= oldTop {
		b.top = newTop
	}
	b.mu.Unlock()
}

// docRowsLocked es el numero de filas de contenido (hasta maxScrollback).
// Requiere poseer b.mu.
func (b *Scrollback) docRowsLocked() int {
	d := b.used
	if d > b.H {
		d = b.H
	}
	return d
}

// docRows es el numero de filas de contenido conservadas (hasta maxScrollback).
func (b *Scrollback) docRows() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.docRowsLocked()
}

// DocRows expuesto para los tests.
func (b *Scrollback) DocRows() int { return b.docRows() }

// contentTopLocked es la fila (en el terminal alto) donde empieza el contenido
// visible: 0 mientras el contenido cabe sin rellenar la pantalla y, al rellenar
// y rodar, la fila desde la que se ven las ultimas `rows` lineas. Requiere
// poseer b.mu.
func (b *Scrollback) contentTopLocked() int {
	ct := b.docRowsLocked() - b.rows
	if ct < 0 {
		ct = 0
	}
	return ct
}

// contentTop expuesto (adquiere el cerrojo).
func (b *Scrollback) contentTop() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.contentTopLocked()
}

// ViewTop es la fila absoluta (en el terminal alto) de la primera linea
// visible del viewport.
func (b *Scrollback) ViewTop() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.top
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

// Offset es el numero de lineas subidas desde el fondo (0 = abajo, en vivo).
func (b *Scrollback) Offset() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	off := b.contentTopLocked() - b.top
	if off < 0 {
		off = 0
	}
	return off
}

// AtBottom indica si se ve la pantalla viva (sin subir).
func (b *Scrollback) AtBottom() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.top >= b.contentTopLocked()
}

// Alt indica si el hijo esta en pantalla alterna (less, vim, ...): no se debe
// scrollbar ni mezclar con la historia normal.
func (b *Scrollback) Alt() bool {
	return b.back.Mode()&vt10x.ModeAltScreen != 0
}

// Scroll ajusta el desplazamiento: delta negativo sube (contenido mas antiguo,
// hacia la fila 0) y positivo baja. Devuelve true si el desplazamiento cambio.
func (b *Scrollback) Scroll(delta int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	next := b.top + delta
	if next < 0 {
		next = 0
	}
	max := b.contentTopLocked()
	if next > max {
		next = max
	}
	if next == b.top {
		return false
	}
	b.top = next
	return true
}
