package container

import (
	"math"
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"
)

// tabTones son las posiciones de la rueda de tono: 12 -> 30° de separación,
// discernible a un golpe de vista en la pestana.
const tabTones = 12

// tabGolden es el angulo aureo de la rueda (360/137,508 no es entero), de modo
// que ninguna cantidad razonable de instancias repite tono.
const tabGolden = 137.508

// fnv1a es el hash FNV-1a de 64 bits: misma semilla y mismo primo que Rust.
func fnv1a(data []byte) uint64 {
	var h uint64 = 0xcbf29ce484222325
	for _, b := range data {
		h ^= uint64(b)
		h *= 0x100000001b3
	}
	return h
}

// accentHue devuelve el tono (grados) del acento de una instancia: el hash se
// cuantiza a tabTones posiciones y se gira segun el indice de instancia con el
// angulo aureo, igual que en Rust.
func accentHue(hash uint64, instance int) float64 {
	base := float64(hash%tabTones) * (360.0 / float64(tabTones))
	return math.Mod(base+float64(instance)*tabGolden, 360.0)
}

// projectKey es el path canonico en minusculas (Windows no distingue
// mayusculas), asi el mismo proyecto conserva su color al reabrirse.
func projectKey(project string) string {
	clean := filepath.Clean(project)
	if abs, err := filepath.Abs(clean); err == nil {
		clean = abs
	}
	return strings.ToLower(clean)
}

// hslToRGB es la conversion HSL->RGB de Rust (h en [0,360), s y l en [0,1]).
func hslToRGB(h, s, l float64) (int32, int32, int32) {
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h*6, 2)-1))
	m := l - c/2
	var r, g, b float64
	switch int(h*6) % 6 {
	case 0:
		r, g, b = c, x, 0
	case 1:
		r, g, b = x, c, 0
	case 2:
		r, g, b = 0, c, x
	case 3:
		r, g, b = 0, x, c
	case 4:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return int32((r + m) * 255), int32((g + m) * 255), int32((b + m) * 255)
}

// TabAccent devuelve el color de acento determinista de un proyecto, igual que
// tab_accent de Rust (saturacion 0.65, luminosidad 0.45). El tono se deriva del
// path del proyecto y se gira segun el indice de esta instancia entre los
// Compass vivos.
func TabAccent(project string) tcell.Color {
	h := accentHue(fnv1a([]byte(projectKey(project))), instanceIndex())
	r, g, b := hslToRGB(h/360.0, 0.65, 0.45)
	return tcell.NewRGBColor(r, g, b)
}
