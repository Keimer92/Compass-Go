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
	ctx := &project.ProjectContext{ScriptDir: `C:\proj\Demo`, ProjectType: project.TypeFlutter}
	st := newMenuState(ctx.ProjectType, theme.Default())

	renderMenu(sim, st, ctx, Rect{0, 0, 100, 30})
	sim.Show()
	txt := screenText(sim)

	for _, want := range []string{
		"Compass", "DESARROLLO", "CONFIGURACION", "Proyecto", "FLUTTER",
		"Windows", "Android", "Clean", "Signing",
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("falta %q en el render:\n%s", want, txt)
		}
	}

	// La pestana CFG debe mostrar sus cards (Git, Terminal, Entorno).
	st.setTab(TabCfg)
	renderMenu(sim, st, ctx, Rect{0, 0, 100, 30})
	sim.Show()
	txt = screenText(sim)
	for _, want := range []string{"Git", "Terminal", "Entorno"} {
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
	ctx := &project.ProjectContext{ScriptDir: `C:\proj\Demo`, ProjectType: project.TypeFlutter}
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

// labelX devuelve la columna donde empieza `label` en el screen.
func labelX(sim tcell.SimulationScreen, label string) (int, bool) {
	cells, w, h := sim.GetContents()
	want := []rune(label)
	for y := 0; y < h; y++ {
		row := make([]rune, w)
		for x := 0; x < w; x++ {
			c := cells[y*w+x]
			if len(c.Runes) > 0 {
				row[x] = c.Runes[0]
			} else {
				row[x] = ' '
			}
		}
		for x := 0; x+len(want) <= w; x++ {
			match := true
			for i, r := range want {
				if row[x+i] != r {
					match = false
					break
				}
			}
			if match {
				return x, true
			}
		}
	}
	return 0, false
}

func TestMenuGroupsByPlatformInColumns(t *testing.T) {
	sim := newTestScreen(t) // 100x30: cabe el layout de 3 columnas
	ctx := &project.ProjectContext{ScriptDir: `C:\p`, ProjectType: project.TypeGoWails}
	st := newMenuState(ctx.ProjectType, theme.Default())
	renderMenu(sim, st, ctx, Rect{0, 0, 100, 30})
	sim.Show()

	if groups, cols, ok := st.grouped(); !ok || cols != 3 || len(groups) != 3 {
		t.Fatalf("esperaba 3 columnas agrupadas, got ok=%v cols=%d groups=%d", ok, cols, len(groups))
	}

	winX, ok1 := labelX(sim, "Windows")
	andX, ok2 := labelX(sim, "And. Dev")
	comX, ok3 := labelX(sim, "Clean")
	if !ok1 || !ok2 || !ok3 {
		t.Fatalf("faltan etiquetas en el render (win=%v and=%v com=%v)", ok1, ok2, ok3)
	}
	if !(winX < andX && andX < comX) {
		t.Errorf("columnas mal ordenadas: Windows=%d Android=%d Comun=%d", winX, andX, comX)
	}

	// Navegacion: ↓ dentro de la columna, → entre columnas.
	if st.items[st.selected] != WindowsDev {
		t.Fatalf("seleccion inicial = %v, queria WindowsDev", st.items[st.selected])
	}
	st.moveRow(1)
	if st.items[st.selected] != WindowsBuild {
		t.Errorf("↓ desde WindowsDev = %v, queria WindowsBuild", st.items[st.selected])
	}
	st.moveRow(-1)
	if st.items[st.selected] != WindowsDev {
		t.Errorf("↑ desde WindowsBuild = %v, queria WindowsDev", st.items[st.selected])
	}
	st.moveSelection(1)
	if st.items[st.selected] != AndroidDev {
		t.Errorf("→ desde WindowsDev = %v, queria AndroidDev", st.items[st.selected])
	}
	st.moveSelection(1)
	if st.items[st.selected] != Clean {
		t.Errorf("→ desde AndroidDev = %v, queria Clean", st.items[st.selected])
	}
}
