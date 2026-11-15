package tui

import (
	"strings"
	"testing"

	"compass/internal/project"
	"compass/internal/settings"
	"compass/internal/theme"
)

func TestSettingsModalShowsCheckbox(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())

	sim := newTestScreen(t)
	ctx := &project.ProjectContext{ScriptDir: `C:\proj\Demo`, ProjectType: project.TypeGo}
	st := newMenuState(ctx.ProjectType, theme.Default())
	st.settings = newSettingsModal()

	renderMenu(sim, st, ctx, Rect{0, 0, 100, 30})
	sim.Show()
	txt := screenText(sim)

	for _, want := range []string{"Opciones", "[x]", "carpeta de salida", "siempre esta activo"} {
		if !strings.Contains(txt, want) {
			t.Errorf("falta %q en el modal:\n%s", want, txt)
		}
	}
}

func TestSettingsTogglePersists(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())

	m := newSettingsModal()
	if !m.openOutputDir {
		t.Fatal("la casilla deberia empezar activa (valor por defecto)")
	}
	m.toggle()
	if m.openOutputDir {
		t.Fatal("toggle deberia desactivar la casilla")
	}
	if settings.Load().OpenOutputDir {
		t.Fatal("el cambio deberia persistir en settings.json")
	}
}
