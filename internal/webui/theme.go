package webui

import (
	"fmt"

	"compass/internal/theme"

	"github.com/gdamore/tcell/v2"
)

// ThemeInfo es un tema listo para pintar en el navegador (colores CSS).
type ThemeInfo struct {
	Name    string            `json:"name"`
	Bg      string            `json:"bg"`
	Panel   string            `json:"panel"`
	Sidebar string            `json:"sidebar"`
	Text    string            `json:"text"`
	Muted   string            `json:"muted"`
	Border  string            `json:"border"`
	Brand   string            `json:"brand"`
	Accent  string            `json:"accent"`
	Cards   map[string]string `json:"cards"`
}

func cssColor(c tcell.Color) string {
	v := c.Hex()
	if v < 0 {
		return ""
	}
	return fmt.Sprintf("#%06x", v)
}

func themeInfo(t *theme.Theme) ThemeInfo {
	cards := make(map[string]string, len(theme.CardKeys))
	for i, k := range theme.CardKeys {
		cards[k] = fmt.Sprintf("#%06x", t.CardHexes[i])
	}
	return ThemeInfo{
		Name:    t.Name,
		Bg:      cssColor(t.Background),
		Panel:   cssColor(theme.Mix(t.Background, t.Border, 0.12)),
		Sidebar: cssColor(theme.Mix(t.Background, t.Border, 0.06)),
		Text:    cssColor(t.Text),
		Muted:   cssColor(t.Muted),
		Border:  cssColor(t.Border),
		Brand:   cssColor(t.Brand),
		Accent:  cssColor(t.Accent),
		Cards:   cards,
	}
}

// GetThemes devuelve los 20 temas disponibles.
func (b *Bridge) GetThemes() []ThemeInfo {
	out := make([]ThemeInfo, 0, len(theme.Themes))
	for _, t := range theme.Themes {
		out = append(out, themeInfo(t))
	}
	return out
}

// GetTheme devuelve el tema activo (persistido).
func (b *Bridge) GetTheme() ThemeInfo { return themeInfo(theme.Load()) }

// SetTheme activa y persiste un tema por nombre. Devuelve el tema aplicado.
func (b *Bridge) SetTheme(name string) ThemeInfo {
	t, ok := theme.ByName(name)
	if !ok {
		return themeInfo(theme.Load())
	}
	theme.Save(t)
	return themeInfo(t)
}
