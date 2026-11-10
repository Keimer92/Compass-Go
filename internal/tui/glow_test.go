package tui

import (
	"testing"
	"time"

	"compass/internal/project"
	"compass/internal/theme"

	"github.com/gdamore/tcell/v2"
)

// El halo arranca al entrar en una card: moverse dentro de la misma no lo
// reinicia, y salir lo cancela.
func TestHoverStartsTheGlowOnlyWhenEnteringACard(t *testing.T) {
	st := newMenuState(project.TypeGoWails, theme.Default())
	idx := st.tabRange().start + 3

	st.setHover(idx)
	if !st.hoverAnimating() {
		t.Fatal("entrar en una card inicia el halo")
	}
	started := st.hoverStarted

	st.setHover(idx)
	if !st.hoverStarted.Equal(started) {
		t.Fatal("seguir en la misma card no reinicia la animacion")
	}

	st.setHover(-1)
	if st.hoverAnimating() {
		t.Fatal("sin hover no hay animacion")
	}
	if !st.hoverStarted.IsZero() {
		t.Fatal("sin hover el instante de entrada queda limpio")
	}
}

// El halo gira en bucle: la fase vuelve a 0 cada hoverCycle, asi que sigue
// animandose mientras el puntero permanezca sobre la card.
func TestHoverPhaseCyclesWhileHovering(t *testing.T) {
	st := newMenuState(project.TypeGoWails, theme.Default())
	st.setHover(st.tabRange().start)

	st.hoverStarted = time.Now().Add(-hoverCycle / 2)
	half, ok := st.hoverPhase()
	if !ok || absf(half-0.5) > 0.05 {
		t.Fatalf("media vuelta ≈ fase 0.5: %v (%v)", half, ok)
	}

	st.hoverStarted = time.Now().Add(-hoverCycle * 3 / 2)
	wrapped, ok := st.hoverPhase()
	if !ok || absf(wrapped-0.5) > 0.05 {
		t.Fatalf("vuelta y media ≈ fase 0.5: %v", wrapped)
	}
	if !st.hoverAnimating() {
		t.Fatal("mientras haya hover el halo sigue")
	}
}

// El recorrido del halo por el perimetro no se sale del borde.
func TestPerimeterPointWalksTheBorder(t *testing.T) {
	r := Rect{3, 2, 10, 3}
	for _, pos := range []float32{0, 4.9, 9.9, 10, 15.5, 21.9, 22.5} {
		x, y := perimeterPoint(r, pos)
		if x != r.X && x != r.Right()-1 && y != r.Y && y != r.Bottom()-1 {
			t.Fatalf("pos=%v cayo fuera del borde: (%d, %d)", pos, x, y)
		}
	}
}

// El halo pinta sobre la card hovereada: el borde cambia de fg y el interior
// de bg (resplandor), respecto del mismo hover sin animacion.
func TestHoverGlowBrightensTheBorderAndTheInterior(t *testing.T) {
	settled, rect := renderTestCard(t, 0, false)
	glowing, _ := renderTestCard(t, 0.5, true)

	borderChanges := 0
	for x := rect.X; x < rect.Right(); x++ {
		for _, y := range []int{rect.Y, rect.Bottom() - 1} {
			if fgOf(settled, x, y) != fgOf(glowing, x, y) {
				borderChanges++
			}
		}
	}
	for y := rect.Y; y < rect.Bottom(); y++ {
		for _, x := range []int{rect.X, rect.Right() - 1} {
			if fgOf(settled, x, y) != fgOf(glowing, x, y) {
				borderChanges++
			}
		}
	}
	if borderChanges == 0 {
		t.Fatal("el halo debe iluminar celdas del borde de la card hovereada")
	}

	interiorChanges := 0
	for x := rect.X + 1; x < rect.Right()-1; x++ {
		if bgOf(settled, x, rect.Y+1) != bgOf(glowing, x, rect.Y+1) {
			interiorChanges++
		}
	}
	if interiorChanges == 0 {
		t.Fatal("el resplandor debe tenir el fondo interior de la card")
	}
}

// renderTestCard pinta una card hovereada en una pantalla simulada y devuelve
// la pantalla y su rectangulo.
func renderTestCard(t *testing.T, phase float32, glow bool) (tcell.SimulationScreen, Rect) {
	t.Helper()
	s := newTestScreen(t)
	rect := Rect{2, 1, 30, CardH}
	th := theme.Default()
	buf := newCellBuf(rect)
	drawCard(buf, rect, WindowsDev, false, true, project.TypeGoWails, th)
	if glow {
		key := WindowsDev.ColorKey()
		applyHoverGlow(buf, rect, phase, th.CardAccent(key), th.CardBg(key), th.Text)
	}
	buf.blit(s)
	s.Show()
	return s, rect
}

func fgOf(s tcell.SimulationScreen, x, y int) tcell.Color {
	cells, w, _ := s.GetContents()
	fg, _, _ := cells[y*w+x].Style.Decompose()
	return fg
}

func bgOf(s tcell.SimulationScreen, x, y int) tcell.Color {
	cells, w, _ := s.GetContents()
	_, bg, _ := cells[y*w+x].Style.Decompose()
	return bg
}

func absf(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
