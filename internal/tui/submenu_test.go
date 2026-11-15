package tui

import (
	"testing"
	"time"

	"compass/internal/project"
	"compass/internal/theme"

	"github.com/gdamore/tcell/v2"
)

func testSubmenu(n int) *Submenu[int] {
	entries := make([]SubEntry[int], n)
	for i := range entries {
		entries[i] = SubEntry[int]{Value: i, Item: SubItem{Label: string(rune('A' + i))}}
	}
	s := NewSubmenu("TEST", "MNT", entries)
	s.cols = 3
	return s
}

func TestSubmenuMovesByRowAndColumn(t *testing.T) {
	s := testSubmenu(5)
	s.vh = 8 // dos filas de cards

	s.moveSelection(1)
	if s.selected != 1 {
		t.Fatalf("selected = %d", s.selected)
	}
	s.moveRow(1)
	if s.selected != 4 {
		t.Fatalf("una fila abajo desde 1 con 3 columnas = 4, no %d", s.selected)
	}
	s.moveRow(1)
	if s.selected != 4 {
		t.Fatalf("la seleccion no debe salirse del final: %d", s.selected)
	}
	s.moveSelection(1)
	if s.selected != 4 {
		t.Fatalf("la seleccion no debe salirse del final: %d", s.selected)
	}
	s.moveRow(-1)
	if s.selected != 1 {
		t.Fatalf("una fila arriba desde 4 = 1, no %d", s.selected)
	}
	s.moveSelection(-5)
	if s.selected != 0 {
		t.Fatalf("la seleccion no debe salirse del principio: %d", s.selected)
	}
}

func TestSubmenuScrollsWholeRows(t *testing.T) {
	s := testSubmenu(9)
	s.vh = 8 // dos filas visibles
	s.moveRow(2)
	// Fila 2 (indices 6..8) fuera del viewport de 2 filas: scroll = 1.
	if s.selected != 6 {
		t.Fatalf("selected = %d", s.selected)
	}
	if s.scroll != 1 {
		t.Fatalf("scroll = %d, se esperaba 1", s.scroll)
	}
	s.moveRow(-2)
	if s.scroll != 0 {
		t.Fatalf("scroll = %d, se esperaba 0", s.scroll)
	}
}

func TestSubmenuHoverFollowsThePointer(t *testing.T) {
	s := testSubmenu(4)
	s.ui = &UiLayout{List: Rect{0, 3, 90, 8}, Cols: 3}

	s.setHover(1)
	if !s.hoverAnimating() {
		t.Fatal("el hover inicia el halo")
	}
	started := s.hoverStarted
	time.Sleep(time.Millisecond)
	s.setHover(1)
	if !s.hoverStarted.Equal(started) {
		t.Fatal("seguir en la misma card no reinicia el halo")
	}
	s.setHover(-1)
	if s.hoverAnimating() {
		t.Fatal("sin hover no hay animacion")
	}
}

// TestSubmenuUsesThemedCards comprueba que las cards del submenu salen con el
// mismo estilo que las del menu principal: borde en el acento de la card y
// fondo del acento atenuado cuando esta seleccionada.
func TestSubmenuUsesThemedCards(t *testing.T) {
	th := theme.Themes[0]
	ctx := &project.ProjectContext{ProjectType: project.TypeGoWails, AndroidHome: "x", ScriptDir: "C:/tmp/proj"}
	entries := []SubEntry[int]{
		{Value: 0, Item: SubItem{Icon: "\U0001F5A5", Label: "Windows x64", Description: "amd64", ColorKey: "run"}},
		{Value: 1, Item: SubItem{Icon: "\U0001F5A5", Label: "Windows arm64", Description: "arm64", ColorKey: "build"}},
	}
	s := NewSubmenu("RELEASE WINDOWS", "DEV", entries)
	s.theme = th

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(100, 30)

	s.draw(screen, ctx)

	ui := layout(Rect{0, 0, 100, 30}, showAndroidWarning(ctx), false)
	cardW := gridCardW(ui.List.Width, ui.Cols)
	first := Rect{ui.List.X, ui.List.Y, cardW, CardH}

	// Esquina del borde de la card seleccionada: el halo gira sobre el acento,
	// asi que el color varia por fotograma; lo estable es el fondo del acento
	// atenuado y que el borde no sea el texto plano del tema.
	_, _, st, _ := screen.GetContent(first.X, first.Y)
	fg, bg, _ := st.Decompose()
	if bg != th.CardBg("run") {
		t.Errorf("fondo de la card seleccionada = %v, queria %v", bg, th.CardBg("run"))
	}
	if fg == th.Text {
		t.Errorf("borde de la card seleccionada = %v, queria el acento de la card", fg)
	}

	// La segunda card no esta seleccionada: su fondo es el del tema.
	second := Rect{first.X + gridCellW(ui.List.Width, ui.Cols), first.Y, cardW, CardH}
	_, _, st2, _ := screen.GetContent(second.X, second.Y)
	fg2, bg2, _ := st2.Decompose()
	if fg2 != th.CardAccent("build") {
		t.Errorf("borde 2 fg = %v, queria %v", fg2, th.CardAccent("build"))
	}
	if bg2 != th.Background {
		t.Errorf("fondo de la card sin seleccionar = %v, queria %v", bg2, th.Background)
	}
}
