package container

import (
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

func TestScrollbackCapturesHistory(t *testing.T) {
	b := newScrollback(20, 4)

	// Doce lineas en una pantalla de 4: solo caben 4, el resto es historia.
	var buf strings.Builder
	for i := 0; i < 12; i++ {
		buf.WriteString("L" + string(rune('A'+i)) + "\n")
	}
	b.Feed([]byte(buf.String()))

	if got := b.DocRows(); got != 12 {
		t.Errorf("DocRows = %d, queria 12", got)
	}
	// Sin subir se ven las ultimas 4 (L8..L11): la primera visible es L8.
	if got := lineText(b, b.visibleTop(0)); got != "L8" {
		t.Errorf("ventana viva = %q, queria L8", got)
	}

	// Subir 3 lineas: la primera visible pasa a ser L5.
	if !b.Scroll(-3) {
		t.Fatal("Scroll(-3) no cambio el desplazamiento")
	}
	if got := b.Offset(); got != 3 {
		t.Errorf("Offset = %d, queria 3", got)
	}
	if got := lineText(b, b.visibleTop(b.Offset())); got != "L5" {
		t.Errorf("ventana subida = %q, queria L5", got)
	}

	// Bajar hasta el fondo.
	for b.Scroll(1) {
	}
	if got := b.Offset(); got != 0 {
		t.Errorf("Offset al bajar = %d, queria 0", got)
	}
	if got := lineText(b, b.visibleTop(0)); got != "L8" {
		t.Errorf("al volver abajo = %q, queria L8", got)
	}
}

func TestScrollbackAnchorOnNewOutput(t *testing.T) {
	b := newScrollback(20, 4)
	var buf strings.Builder
	for i := 0; i < 12; i++ {
		buf.WriteString("L" + string(rune('A'+i)) + "\n")
	}
	b.Feed([]byte(buf.String()))

	b.Scroll(-3) // subimos 3
	before := b.visibleTop(b.Offset())
	if before != b.contentTop()-3 {
		t.Fatalf("visibleTop = %d, queria contentTop-3 = %d", before, b.contentTop()-3)
	}

	// Llega texto nuevo mientras se esta subido: la vista se queda anclada a
	// las mismas filas del documento (el contenido rueda por debajo).
	b.Feed([]byte("L12\n"))
	if got := b.visibleTop(b.Offset()); got != before {
		t.Errorf("visibleTop tras llegar texto = %d, queria seguir en %d", got, before)
	}
	if got := lineText(b, before); got != "L6" {
		t.Errorf("la fila anclada ahora es %q, queria L6", got)
	}
}

func TestScrollbackClampShortContent(t *testing.T) {
	b := newScrollback(20, 10)
	b.Feed([]byte("una\nlinea\ncorta\n"))
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
	var buf strings.Builder
	for i := 0; i < 12; i++ {
		buf.WriteString("L" + string(rune('A'+i)) + "\n")
	}
	b.Feed([]byte(buf.String()))
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
	var buf strings.Builder
	for i := 0; i < 12; i++ {
		buf.WriteString("L" + string(rune('A'+i)) + "\n")
	}
	b.Feed([]byte(buf.String()))
	if b.DocRows() != 12 {
		t.Fatalf("DocRows = %d, queria 12", b.DocRows())
	}
	// Pantalla alterna (pager/editor): no debe mezclarse con la historia.
	b.Feed([]byte("\x1b[?1049hcontenido alt\x1b[?1049l"))
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
