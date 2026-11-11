package container

import (
	"strings"
	"testing"

	"github.com/hinshun/vt10x"
)

// feed escribe content en un terminal vt10x y actualiza el scrollback.
func feed(b *Scrollback, term vt10x.Terminal, content string) {
	_, _ = term.Write([]byte(content))
	b.Update(term)
}

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

func TestScrollbackCapturesScrolledLines(t *testing.T) {
	term := vt10x.New(vt10x.WithSize(20, 4))
	b := newScrollback()

	// Cuatro lineas en una pantalla de 4: la cuarta hace rodar la primera.
	feed(b, term, "L0\nL1\nL2\nL3\n")

	if got := b.DocRows(); got != 5 {
		t.Errorf("DocRows = %d, queria 5", got)
	}
	if got := lineText(b, 0); got != "L0" {
		t.Errorf("fila 0 (historia) = %q, queria L0", got)
	}
	if got := b.Offset(); got != 0 {
		t.Errorf("Offset = %d, queria 0 (al fondo)", got)
	}
}

func TestScrollbackScrollAndAnchor(t *testing.T) {
	term := vt10x.New(vt10x.WithSize(20, 5))
	b := newScrollback()

	// Doce lineas en pantalla de 5: 7 deben ir a la historia.
	var buf strings.Builder
	for i := 0; i < 12; i++ {
		buf.WriteString("L" + string(rune('A'+i)) + "\n")
	}
	feed(b, term, buf.String())

	if got := b.DocRows(); got != 12 {
		t.Errorf("DocRows = %d, queria 12", got)
	}
	if back := b.DocRows() - 5; back != 7 {
		t.Errorf("lineas en historia = %d, queria 7", back)
	}

	// Subir 3 lineas: el tope del documento visible retrocede.
	if !b.Scroll(-3) {
		t.Fatal("Scroll(-3) no cambio el desplazamiento")
	}
	if got := b.Offset(); got != 3 {
		t.Errorf("Offset = %d, queria 3", got)
	}
	// Tras subir 3, la primera fila visible es la linea 4 del documento
	// (0-based: DocRows 12, filas 5, offset 3 -> top = 12-5-3 = 4).
	if got := b.TopRow(); got != 4 {
		t.Errorf("TopRow = %d, queria 4", got)
	}
	if got := lineText(b, b.TopRow()); got != "L4" {
		t.Errorf("fila superior = %q, queria L4", got)
	}

	// Nuevo texto mientras se esta subido: la vista se queda anclada.
	before := b.TopRow()
	feed(b, term, "L12\nL13\n")
	if got := b.TopRow(); got != before {
		t.Errorf("TopRow tras llegar texto = %d, queria seguir en %d (anclado)", got, before)
	}
	if got := b.Offset(); got != 5 {
		t.Errorf("Offset tras anclaje = %d, queria 5", got)
	}

	// Bajar hasta el fondo.
	for b.Scroll(1) {
	}
	if got := b.Offset(); got != 0 {
		t.Errorf("Offset al bajar = %d, queria 0", got)
	}
}

func TestScrollbackResetsOnResize(t *testing.T) {
	term := vt10x.New(vt10x.WithSize(20, 4))
	b := newScrollback()
	feed(b, term, "L0\nL1\nL2\nL3\n")
	if b.DocRows() != 5 {
		t.Fatalf("DocRows = %d, queria 5 antes del resize", b.DocRows())
	}
	term.Resize(20, 10)
	b.Update(term)
	if b.DocRows() != 10 {
		t.Errorf("tras resize DocRows = %d, queria 10 (historia reiniciada)", b.DocRows())
	}
}

func TestScrollbackIgnoresAltScreen(t *testing.T) {
	term := vt10x.New(vt10x.WithSize(20, 4))
	b := newScrollback()
	// Historia normal.
	feed(b, term, "L0\nL1\nL2\nL3\n")
	if b.DocRows() != 5 {
		t.Fatalf("DocRows = %d, queria 5", b.DocRows())
	}
	// Pantalla alterna (pager/editor): no debe mezclarse con la historia.
	feed(b, term, "\x1b[?1049hcontenido alt\x1b[?1049l")
	if b.DocRows() != 4 {
		t.Errorf("en pantalla alterna DocRows = %d, queria 4 (sin historia)", b.DocRows())
	}
}
