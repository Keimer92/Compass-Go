package container

import (
	"testing"

	"compass/internal/theme"

	"github.com/hinshun/vt10x"
)

// newScreenSession crea una sesion con una pantalla ya rellena, sin ConPTY.
func newScreenSession(cols, rows int, content string) *Session {
	term := vt10x.New(vt10x.WithSize(cols, rows))
	_, _ = term.Write([]byte(content))
	return &Session{term: term}
}

func TestSelectionText(t *testing.T) {
	sess := newScreenSession(20, 5, "hola mundo\r\nsegunda linea\r\ntercera\r\n")

	cases := []struct {
		name string
		sel  Selection
		want string
	}{
		{
			name: "una palabra",
			sel:  Selection{fromCol: 0, fromRow: 0, toCol: 3, toRow: 0, active: true},
			want: "hola",
		},
		{
			name: "varias lineas",
			sel:  Selection{fromCol: 0, fromRow: 0, toCol: 6, toRow: 1, active: true},
			want: "hola mundo\r\nsegunda",
		},
		{
			name: "arrastre al reves",
			sel:  Selection{fromCol: 6, fromRow: 1, toCol: 0, toRow: 0, active: true},
			want: "hola mundo\r\nsegunda",
		},
		{
			name: "linea entera hasta el final",
			sel:  Selection{fromCol: 0, fromRow: 2, toCol: 19, toRow: 2, active: true},
			want: "tercera",
		},
		{
			name: "sin seleccion",
			sel:  Selection{},
			want: "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.sel.Text(sess); got != c.want {
				t.Errorf("Text() = %q, queria %q", got, c.want)
			}
		})
	}
}

func TestSelectionContains(t *testing.T) {
	// De (2,0) a (1,2): primer renglon desde la columna 2, ultimo hasta la 1.
	sel := Selection{fromCol: 2, fromRow: 0, toCol: 1, toRow: 2, active: true}

	cases := []struct {
		col, row int
		want     bool
	}{
		{0, 0, false},
		{2, 0, true},
		{15, 0, true},
		{0, 1, true},
		{15, 1, true},
		{0, 2, true},
		{1, 2, true},
		{2, 2, false},
		{5, 3, false},
	}
	for _, c := range cases {
		if got := sel.Contains(c.col, c.row); got != c.want {
			t.Errorf("Contains(%d,%d) = %v, queria %v", c.col, c.row, got, c.want)
		}
	}

	if (Selection{}).Contains(0, 0) {
		t.Error("una seleccion vacia no contiene nada")
	}
}

// TestRenderViewHighlightsSelection comprueba que las celdas seleccionadas se
// pintan con el acento del tema y las demas no.
func TestRenderViewHighlightsSelection(t *testing.T) {
	th := theme.Default()
	term := vt10x.New(vt10x.WithSize(20, 4))
	_, _ = term.Write([]byte("hola mundo\r\n"))

	sel := Selection{fromCol: 0, fromRow: 0, toCol: 3, toRow: 0, active: true}
	s := newTestScreen(20, 4)
	defer s.Fini()
	renderView(s, Rect{0, 0, 20, 4}, term, th, sel)
	s.Show()

	cells, w, _ := s.GetContents()
	if _, bg, _ := cells[0].Style.Decompose(); bg != th.Brand {
		t.Errorf("la celda seleccionada no lleva el acento: %v", bg)
	}
	if _, bg, _ := cells[5].Style.Decompose(); bg == th.Brand {
		t.Errorf("una celda fuera de la seleccion no deberia ir resaltada")
	}
	if len(cells) < w {
		t.Fatalf("pantalla simulada incompleta")
	}
}
