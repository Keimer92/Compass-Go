package tui

import (
	"math"
	"time"

	"compass/internal/theme"

	"github.com/gdamore/tcell/v2"
)

// Duracion de una vuelta completa del halo de hover.
const hoverCycle = 2600 * time.Millisecond

// Longitud de la cola luminosa del halo, en celdas de borde.
const shimmerTrail = 9.0

// Radio del resplandor interior alrededor de la cabeza del halo.
const glowRadius = 11.0

// Intervalo entre fotogramas mientras hay una animacion en curso (~30 fps).
const animFrame = 33 * time.Millisecond

// applyHoverGlow pinta el halo luminoso que recorre el borde de `r` en bucle
// (sentido horario) y derrama un resplandor interior alrededor de su cabeza,
// al estilo del orbe del icono de Google.
//
// Se aplica sobre el lienzo de la card ya pintada: en el borde cambia el `fg`;
// en el interior tine el `bg` hacia el tinte oscuro del acento, sin tocar el
// texto.
func applyHoverGlow(b *cellBuf, r Rect, phase float32, accent, tint, bright tcell.Color) {
	if r.Width < 2 || r.Height < 2 {
		return
	}
	w := float32(r.Width)
	h := float32(r.Height)
	// Perimetro del borde: arriba + abajo + laterales (las esquinas ya cuentan
	// en arriba/abajo).
	total := 2*w + 2*(h-2)
	phase -= float32(math.Floor(float64(phase)))
	head := phase * total
	// Respiracion: dos pulsos de intensidad por vuelta.
	breath := float32(0.72 + 0.28*math.Sin(float64(phase)*math.Pi*2*2))
	glow := theme.Mix(accent, bright, 0.55)

	// 1. Cola luminosa sobre el borde, con distancia circular para que la
	//    cabeza y su estela crucen las esquinas sin saltos.
	pos := 0
	paint := func(x, y int) {
		raw := float32(pos) - head
		if raw < 0 {
			raw = -raw
		}
		dist := raw
		if total-raw < dist {
			dist = total - raw
		}
		k := 1 - dist/shimmerTrail
		if k < 0 {
			k = 0
		}
		k *= breath
		if k > 0.02 {
			if c := b.at(x, y); c != nil {
				fg, _, _ := c.st.Decompose()
				c.st = c.st.Foreground(theme.Mix(fg, glow, minf(k, 1)))
			}
		}
		pos++
	}

	for x := r.X; x < r.Right(); x++ {
		paint(x, r.Y)
	}
	for y := r.Y + 1; y < r.Bottom(); y++ {
		paint(r.Right()-1, y)
	}
	for x := r.Right() - 2; x >= r.X; x-- {
		paint(x, r.Bottom()-1)
	}
	for y := r.Bottom() - 2; y >= r.Y+1; y-- {
		paint(r.X, y)
	}

	// 2. Resplandor interior: el fondo se oscurece hacia el tinte del acento
	//    cerca de la cabeza. Radio eliptico porque la card es ancha y baja.
	cx, cy := perimeterPoint(r, head)
	for y := r.Y + 1; y < r.Bottom()-1; y++ {
		for x := r.X + 1; x < r.Right()-1; x++ {
			dx := float32(x - cx)
			dy := float32(y-cy) * 2
			d := float32(math.Sqrt(float64(dx*dx + dy*dy)))
			k := 1 - d/glowRadius
			if k < 0 {
				k = 0
			}
			k *= 0.8 * breath
			if k > 0.02 {
				if c := b.at(x, y); c != nil {
					_, bg, _ := c.st.Decompose()
					c.st = c.st.Background(theme.Mix(bg, tint, minf(k, 1)))
				}
			}
		}
	}
}

// perimeterPoint es la coordenada del punto `pos` del perimetro de `r`, en el
// mismo recorrido que hace applyHoverGlow (arriba → derecha → abajo →
// izquierda).
func perimeterPoint(r Rect, pos float32) (int, int) {
	w := float32(r.Width)
	h := float32(r.Height)
	total := 2*w + 2*(h-2)
	p := pos - float32(math.Floor(float64(pos/total)))*total
	if p < 0 {
		p += total
	}
	// Arriba (incluye las dos esquinas superiores).
	if p < w {
		return r.X + int(p), r.Y
	}
	p -= w
	// Derecha (sin la esquina superior, con la inferior).
	if p < h-1 {
		return r.Right() - 1, r.Y + int(p) + 1
	}
	p -= h - 1
	// Abajo (de derecha a izquierda).
	if p < w-1 {
		return r.Right() - 2 - int(p), r.Bottom() - 1
	}
	p -= w - 1
	// Izquierda (sin esquinas).
	return r.X, r.Bottom() - 2 - int(p)
}

// minf es el minimo de dos float32.
func minf(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}
