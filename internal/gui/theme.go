package gui

import (
	"image/color"

	"compass/internal/theme"

	"github.com/gdamore/tcell/v2"
)

// Palette es el tema de Compass traducido a colores de Gio. Los 20 temas de
// internal/theme se reutilizan tal cual: solo cambia el tipo de color.
type Palette struct {
	Name                                                                        string
	Background, Text, Muted, Border, Brand                                      color.NRGBA
	Dev, Mnt, Cfg, HoverBg, Success, Info, Accent, Warning                      color.NRGBA
	Cards                                                                       [18]color.NRGBA
}

// nrgba convierte un tcell.Color (que es lo que usa internal/theme) a NRGBA.
func nrgba(c tcell.Color) color.NRGBA {
	r, g, b := c.RGB()
	return color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 0xff}
}

func rgbHex(v int32) color.NRGBA {
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff}
}

// paletteFrom traduce un *theme.Theme a la paleta que consume la GUI.
func paletteFrom(t *theme.Theme) Palette {
	p := Palette{
		Name:       t.Name,
		Background: nrgba(t.Background),
		Text:       nrgba(t.Text),
		Muted:      nrgba(t.Muted),
		Border:     nrgba(t.Border),
		Brand:      nrgba(t.Brand),
		Dev:        nrgba(t.Dev),
		Mnt:        nrgba(t.Mnt),
		Cfg:        nrgba(t.Cfg),
		HoverBg:    nrgba(t.HoverBg),
		Success:    nrgba(t.Success),
		Info:       nrgba(t.Info),
		Accent:     nrgba(t.Accent),
		Warning:    nrgba(t.Warning),
	}
	for i, hex := range t.CardHexes {
		p.Cards[i] = rgbHex(hex)
	}
	return p
}

// dim oscurece un color hacia el fondo, para el fondo de la card activa.
func dim(c color.NRGBA, pct int) color.NRGBA {
	return color.NRGBA{
		R: uint8(int(c.R) * pct / 100),
		G: uint8(int(c.G) * pct / 100),
		B: uint8(int(c.B) * pct / 100),
		A: 0xff,
	}
}

// blend mezcla a y b con t en [0,1].
func blend(a, b color.NRGBA, t float32) color.NRGBA {
	mix := func(x, y uint8) uint8 {
		return uint8(float32(x) + (float32(y)-float32(x))*t)
	}
	return color.NRGBA{R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B), A: 0xff}
}
