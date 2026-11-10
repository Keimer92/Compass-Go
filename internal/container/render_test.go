package container

import (
	"strings"
	"testing"

	"compass/internal/theme"

	"github.com/gdamore/tcell/v2"
)

// screenText vuelca el contenido de una pantalla simulada.
func screenText(s tcell.SimulationScreen) string {
	cells, w, h := s.GetContents()
	var b strings.Builder
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			if i >= len(cells) {
				break
			}
			if cells[i].Runes == nil {
				b.WriteByte(' ')
				continue
			}
			b.WriteString(string(cells[i].Runes))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func newTestScreen(w, h int) tcell.SimulationScreen {
	s := tcell.NewSimulationScreen("UTF-8")
	_ = s.Init()
	s.SetSize(w, h)
	return s
}

func TestDrawBarShowsTheCompassTab(t *testing.T) {
	s := newTestScreen(80, 10)
	defer s.Fini()

	c := New()
	c.DrawBar(s, Rect{0, 0, 80, BarHeight}, theme.Default())
	s.Show()

	text := screenText(s)
	if !strings.Contains(text, "Compass Go") {
		t.Fatalf("la barra no muestra la pestana del menu:\n%s", text)
	}
	if !strings.Contains(text, "╭") || !strings.Contains(text, "╮") {
		t.Fatalf("la pestana no tiene caja redondeada:\n%s", text)
	}
}

func TestClickOnTheBarSelectsTheMenu(t *testing.T) {
	c := New()
	// Sin pestanas abiertas solo existe la pestana Compass.
	c.DrawBar(newTestScreen(80, 10), Rect{0, 0, 80, BarHeight}, theme.Default())
	if len(c.bar) != 1 || c.bar[0].target != -1 {
		t.Fatalf("zonas de la barra: %+v", c.bar)
	}
	if !c.Click(c.bar[0].rect.X+2, 1) {
		t.Fatal("el clic deberia caer en la pestana Compass")
	}
	if c.Active() != -1 {
		t.Fatalf("active = %d", c.Active())
	}
}
