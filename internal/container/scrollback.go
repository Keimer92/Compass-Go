package container

import (
	"sync"
	"time"

	"github.com/hinshun/vt10x"
)

// maxScrollback es el numero maximo de filas guardadas por pestana. Basta para
// varias pantallas de salida de un build/debug sin consumir memoria sin ton.
const maxScrollback = 5000

// Scrollback conserva las filas que vt10x ya no muestra. El emulador solo
// tiene pantalla visible, asi que las capturamos comparando la pantalla viva
// entre muestreos: cuando rueda hacia arriba, las filas que salen por arriba
// se guardan aqui y el usuario puede subir a verlas y seleccionarlas.
//
// Cada pestana tiene su propio Scrollback y una goroutine que lo actualiza.
type Scrollback struct {
	mu sync.Mutex

	cols, rows int
	live       [][]vt10x.Glyph // ultima pantalla viva (rows filas de cols glyphs)
	backlog    [][]vt10x.Glyph // filas que rodaron fuera de la parte visible
	offset     int             // lineas subidas desde el fondo (0 = abajo, en vivo)

	curX, curY int
	curVisible bool
	curGlyph   vt10x.Glyph
}

// newScrollback crea un scrollback sin geometria (la fija la primera captura).
func newScrollback() *Scrollback {
	return &Scrollback{}
}

// Cols es el ancho de la pantalla capturada.
func (b *Scrollback) Cols() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cols
}

// resize reinicia el scrollback porque la geometria cambio.
func (b *Scrollback) resize(cols, rows int) {
	b.cols, b.rows = cols, rows
	b.live = nil
	b.backlog = nil
	b.offset = 0
	b.curVisible = false
}

// Update toma una instantanea de la pantalla viva y, si ha rodado hacia
// arriba, guarda en el backlog las filas que salieron por arriba.
func (b *Scrollback) Update(v vt10x.View) {
	cols, rows := v.Size()
	if cols != b.cols || rows != b.rows {
		b.resize(cols, rows)
	}

	cur := make([][]vt10x.Glyph, rows)
	for y := 0; y < rows; y++ {
		row := make([]vt10x.Glyph, cols)
		for x := 0; x < cols; x++ {
			row[x] = v.Cell(x, y)
		}
		cur[y] = row
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// Pantalla alterna (less, vim, ...): no tiene sentido el scrollback ni
	// mezclarlo con la normal; se reinicia al volver.
	if v.Mode()&vt10x.ModeAltScreen != 0 {
		b.backlog = nil
		b.offset = 0
		b.live = cur
		b.captureCursor(v)
		return
	}

	if len(b.live) == 0 {
		b.live = cur
		b.captureCursor(v)
		return
	}

	// Numero de filas que rodaron hacia arriba: el mayor k tal que
	// cur[i] == live[i+k] para i en [0, rows-k).
	k := 0
	for cand := 1; cand <= rows; cand++ {
		ok := true
		for i := 0; i < rows-cand; i++ {
			if !rowEqual(cur[i], b.live[i+cand]) {
				ok = false
				break
			}
		}
		if ok {
			k = cand
		} else {
			break
		}
	}

	if k > 0 {
		for j := 0; j < k; j++ {
			b.backlog = append(b.backlog, b.live[j])
		}
		if len(b.backlog) > maxScrollback {
			b.backlog = b.backlog[len(b.backlog)-maxScrollback:]
		}
		b.live = cur
		// Si el usuario estaba subido, se queda viendo las mismas filas de
		// historia: el indice de arriba se desplaza igual que el texto.
		if b.offset > 0 {
			b.offset += k
			if b.offset > len(b.backlog) {
				b.offset = len(b.backlog)
			}
		}
	} else {
		b.live = cur
	}
	b.captureCursor(v)
}

// captureCursor guarda la posicion y el glyph del cursor para pintarlo.
func (b *Scrollback) captureCursor(v vt10x.View) {
	b.curVisible = v.CursorVisible()
	cur := v.Cursor()
	b.curX, b.curY = cur.X, cur.Y
	b.curGlyph = v.Cell(cur.X, cur.Y)
}

// rowEqual compara dos filas celda a celda.
func rowEqual(a, b []vt10x.Glyph) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// DocRows es el numero total de filas (historia + vivas).
func (b *Scrollback) DocRows() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.backlog) + b.rows
}

// TopRow es la primera fila del documento visible (coordenadas de doc).
func (b *Scrollback) TopRow() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.backlog) + b.rows - b.rows - b.offset
}

// Cell devuelve el glyph en coordenadas de documento (docX, docY).
func (b *Scrollback) Cell(docX, docY int) vt10x.Glyph {
	b.mu.Lock()
	defer b.mu.Unlock()
	if docY < 0 {
		return vt10x.Glyph{}
	}
	if docY < len(b.backlog) {
		row := b.backlog[docY]
		if docX >= 0 && docX < len(row) {
			return row[docX]
		}
		return vt10x.Glyph{}
	}
	liveY := docY - len(b.backlog)
	if liveY >= 0 && liveY < len(b.live) {
		row := b.live[liveY]
		if docX >= 0 && docX < len(row) {
			return row[docX]
		}
	}
	return vt10x.Glyph{}
}

// Scroll ajusta el desplazamiento (negativo sube, positivo baja). Devuelve
// true si el desplazamiento cambio.
func (b *Scrollback) Scroll(delta int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	max := len(b.backlog)
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

// AtBottom indica si se ve la pantalla viva (sin subir).
func (b *Scrollback) AtBottom() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.offset == 0
}

// Offset es el numero de lineas subidas desde el fondo.
func (b *Scrollback) Offset() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.offset
}

// Cursor devuelve la posicion del cursor en coordenadas de documento y si es
// visible (solo cuando se ve la pantalla viva).
func (b *Scrollback) Cursor() (x, y int, g vt10x.Glyph, visible bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.offset != 0 || !b.curVisible {
		return 0, 0, vt10x.Glyph{}, false
	}
	return b.curX, len(b.backlog) + b.curY, b.curGlyph, true
}

// Start arranca la goroutine que actualiza el scrollback hasta que se cierre
// done.
func (b *Scrollback) Start(s *Session, done <-chan struct{}) {
	go func() {
		t := time.NewTicker(40 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				s.WithScreen(b.Update)
			}
		}
	}()
}
