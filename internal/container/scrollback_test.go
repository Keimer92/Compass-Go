package container

import (
	"strconv"
	"strings"
	"testing"
)

// lineText devuelve el texto de una fila del documento, sin espacios finales.
func lineText(b *Scrollback, row int) string {
	var sb strings.Builder
	cols := b.Cols()
	for x := 0; x < cols; x++ {
		if ch := b.Cell(x, row).Char; ch != 0 {
			sb.WriteRune(ch)
		}
	}
	return strings.TrimRight(sb.String(), " ")
}

// feedLines rellena el scrollback con `n` lineas etiquetadas L0..L{n-1} usando
// retorno de carro+salto (como hace el pty real), no solo \n.
func feedLines(b *Scrollback, n int) {
	var buf strings.Builder
	for i := 0; i < n; i++ {
		buf.WriteString("L" + strconv.Itoa(i) + "\r\n")
	}
	b.Feed([]byte(buf.String()))
}

func TestScrollbackCapturesHistory(t *testing.T) {
	b := newScrollback(20, 4)

	// Doce lineas en una pantalla de 4: solo caben 4, el resto es historia.
	feedLines(b, 12)

	if got := b.DocRows(); got != 12 {
		t.Errorf("DocRows = %d, queria 12", got)
	}
	// Sin subir se ven las ultimas 4 (L8..L11): la primera visible es L8.
	if got := lineText(b, b.ViewTop()); got != "L8" {
		t.Errorf("ventana viva = %q, queria L8", got)
	}

	// Subir 3 lineas: la primera visible pasa a ser L5.
	if !b.Scroll(-3) {
		t.Fatal("Scroll(-3) no cambio el desplazamiento")
	}
	if got := b.Offset(); got != 3 {
		t.Errorf("Offset = %d, queria 3", got)
	}
	if got := lineText(b, b.ViewTop()); got != "L5" {
		t.Errorf("ventana subida = %q, queria L5", got)
	}

	// Bajar hasta el fondo.
	for b.Scroll(1) {
	}
	if got := b.Offset(); got != 0 {
		t.Errorf("Offset al bajar = %d, queria 0", got)
	}
	if got := lineText(b, b.ViewTop()); got != "L8" {
		t.Errorf("al volver abajo = %q, queria L8", got)
	}
}

func TestScrollbackAnchorOnNewOutput(t *testing.T) {
	b := newScrollback(20, 4)
	feedLines(b, 12)

	b.Scroll(-3) // subimos 3
	before := b.ViewTop()
	if before != b.contentTop()-3 {
		t.Fatalf("ViewTop = %d, queria contentTop-3 = %d", before, b.contentTop()-3)
	}

	// Llega texto nuevo mientras se esta subido: la vista se queda anclada a
	// las mismas filas del documento (el contenido rueda por debajo).
	b.Feed([]byte("L12\r\n"))
	if got := b.ViewTop(); got != before {
		t.Errorf("ViewTop tras llegar texto = %d, queria seguir en %d", got, before)
	}
	// La fila anclada sigue mostrando L5; L12 cae por debajo del viewport.
	if got := lineText(b, before); got != "L5" {
		t.Errorf("la fila anclada ahora es %q, queria L5", got)
	}
}

func TestScrollbackAnchorWhenHistoryFull(t *testing.T) {
	b := newScrollback(20, 4)
	feedLines(b, maxScrollback)
	b.Scroll(-3)
	before := lineText(b, b.ViewTop())

	// Con la historia llena, cada fila nueva desplaza el contenido una fila
	// dentro del terminal alto: el ancla tiene que bajar lo mismo para que la
	// vista siga mostrando las mismas lineas.
	b.Feed([]byte("X\r\n"))
	if got := lineText(b, b.ViewTop()); got != before {
		t.Errorf("la vista derivo de %q a %q al llenarse la historia", before, got)
	}
}

func TestScrollbackClampShortContent(t *testing.T) {
	b := newScrollback(20, 10)
	b.Feed([]byte("una\r\nlinea\r\ncorta\r\n"))
	// Contenido corto: no hay historia por encima, el scroll queda en 0.
	if got := b.Offset(); got != 0 {
		t.Errorf("Offset = %d, queria 0 (sin historia)", got)
	}
	if b.Scroll(-5) {
		t.Error("Scroll no deberia cambiar con contenido corto")
	}
}

func TestScrollbackResetsOnResize(t *testing.T) {
	b := newScrollback(20, 4)
	feedLines(b, 12)
	if b.DocRows() != 12 {
		t.Fatalf("DocRows = %d, queria 12", b.DocRows())
	}
	// Redimensionar el ancho no borra la historia.
	b.Resize(20, 10)
	if b.DocRows() != 12 {
		t.Errorf("tras resize DocRows = %d, queria 12 (historia conservada)", b.DocRows())
	}
}

func TestScrollbackIgnoresAltScreen(t *testing.T) {
	b := newScrollback(20, 4)
	feedLines(b, 12)
	if b.DocRows() != 12 {
		t.Fatalf("DocRows = %d, queria 12", b.DocRows())
	}
	// Pantalla alterna (pager/editor): no debe mezclarse con la historia.
	b.Feed([]byte("\x1b[?1049h"))
	if !b.Alt() {
		t.Fatal("Alt() deberia ser true en pantalla alterna")
	}
	b.Feed([]byte("\x1b[?1049l"))
	if b.Alt() {
		t.Fatal("Alt() deberia ser false al salir de pantalla alterna")
	}
	if b.DocRows() != 12 {
		t.Errorf("DocRows tras alt = %d, queria 12 (historia intacta)", b.DocRows())
	}
}
