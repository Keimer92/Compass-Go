package tui

import (
	"strings"
	"testing"

	"compass/internal/project"
	"compass/internal/theme"

	"github.com/gdamore/tcell/v2"
)

// screenText extrae el texto visible de un screen (una runa por celda).
func screenText(s tcell.SimulationScreen) string {
	cells, w, h := s.GetContents()
	var b strings.Builder
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := cells[y*w+x]
			if len(c.Runes) > 0 {
				b.WriteRune(c.Runes[0])
			} else {
				b.WriteRune(' ')
			}
		}
		b.WriteRune('\n')
	}
	return b.String()
}

func newTestScreen(t *testing.T) tcell.SimulationScreen {
	t.Helper()
	sim := tcell.NewSimulationScreen("UTF-8")
	if err := sim.Init(); err != nil {
		t.Fatalf("init sim screen: %v", err)
	}
	sim.SetSize(100, 30)
	t.Cleanup(sim.Fini)
	return sim
}

func TestRenderMenuShowsCoreElements(t *testing.T) {
	sim := newTestScreen(t)
	ctx := &project.ProjectContext{ScriptDir: `C:\proj\Demo`, ProjectType: project.TypeTauri}
	st := newMenuState(ctx.ProjectType, theme.Default())

	renderMenu(sim, st, ctx, Rect{0, 0, 100, 30})
	sim.Show()
	txt := screenText(sim)

	for _, want := range []string{
		"Compass", "DESARROLLO", "CONFIGURACION", "Proyecto", "TAURI",
		"Windows", "Android", "Clean", "Signing",
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("falta %q en el render:\n%s", want, txt)
		}
	}

	// La pestana CFG debe mostrar sus cards (Git, WhatsApp, Terminal, Entorno).
	st.setTab(TabCfg)
	renderMenu(sim, st, ctx, Rect{0, 0, 100, 30})
	sim.Show()
	txt = screenText(sim)
	for _, want := range []string{"Git", "WhatsApp", "Terminal", "Entorno"} {
		if !strings.Contains(txt, want) {
			t.Errorf("falta %q en la pestana CFG:\n%s", want, txt)
		}
	}
}

func TestRenderMenuNarrowDoesNotPanic(t *testing.T) {
	sim := newTestScreen(t)
	sim.SetSize(20, 6) // demasiado pequena: debe mostrar el aviso, no paniquear.
	ctx := &project.ProjectContext{ScriptDir: `C:\p`, ProjectType: project.TypeGo}
	st := newMenuState(ctx.ProjectType, theme.Default())
	renderMenu(sim, st, ctx, Rect{0, 0, 20, 6})
	sim.Show()
	if !strings.Contains(screenText(sim), "demasiado") {
		t.Errorf("esperaba el aviso de terminal pequena")
	}
	if st.ui != nil {
		t.Errorf("ui deberia ser nil cuando no cabe ninguna card")
	}
}

func TestGridHitTesting(t *testing.T) {
	sim := newTestScreen(t)
	ctx := &project.ProjectContext{ScriptDir: `C:\proj\Demo`, ProjectType: project.TypeTauri}
	st := newMenuState(ctx.ProjectType, theme.Default())
	renderMenu(sim, st, ctx, Rect{0, 0, 100, 30})

	ui := st.ui
	if ui == nil {
		t.Fatal("ui no calculada")
	}
	// El centro de la primera card debe mapear al primer item de la pestana.
	idx := st.hoverAt(ui.List.X+1, ui.List.Y+1)
	if idx != st.tabRange().start {
		t.Errorf("hoverAt primera card = %d, queria %d", idx, st.tabRange().start)
	}
}
