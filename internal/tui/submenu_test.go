package tui

import (
	"testing"
	"time"
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
