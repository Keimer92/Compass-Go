package shell

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// TabAccent devuelve el color de acento de la pestaña para un proyecto.
//
// Sale del path canónico y en minúsculas (Windows no distingue mayúsculas), así
// que el mismo proyecto conserva su color al reabrirse. La saturación y la
// luminosidad son fijas para que el texto de la pestaña siga legible en
// cualquier tema.
//
// El tono se cuantiza a 12 posiciones de la rueda y se gira según la instancia
// de Compass que lo pide: dos proyectos cuyos hashes caían cerca (dos verdes
// casi iguales) y dos instancias del *mismo* proyecto se ven siempre distintas.
func TabAccent(project string) [3]byte {
	hue := accentHue(fnv1a64([]byte(projectKey(project))), instanceIndex())
	return hslToRGB(hue/360.0, 0.65, 0.45)
}

// projectKey es la clave cromática del proyecto: su path canónico, en
// minúsculas.
func projectKey(project string) string {
	if resolved, err := filepath.EvalSymlinks(project); err == nil {
		if abs, err := filepath.Abs(resolved); err == nil {
			return strings.ToLower(abs)
		}
	}
	return strings.ToLower(project)
}

const (
	accentTones = 12
	// Giro entre instancias: el ángulo áureo de la rueda. Como 360/137,508 no
	// es entero, ninguna cantidad razonable de instancias repite tono.
	accentGolden = 137.508
)

// accentHue devuelve el tono (grados) del acento de una instancia.
func accentHue(hash uint64, instance int) float64 {
	base := float64(hash%accentTones) * (360.0 / accentTones)
	return math.Mod(base+float64(instance)*accentGolden, 360.0)
}

var (
	instanceOnce  sync.Once
	instanceCount int
)

// instanceIndex es el índice de este proceso entre los Compass que corren a la
// vez. Se ordena por PID y se cuenta cuántos son anteriores: el primer Compass
// de la sesión devuelve 0. Se calcula una sola vez por proceso —si otro Compass
// se cierra mientras este vive, su pestaña no debe cambiar a media sesión.
func instanceIndex() int {
	instanceOnce.Do(func() {
		instanceCount = countEarlierInstances()
	})
	return instanceCount
}

// fnv1a64 es el hash FNV-1a de 64 bits.
func fnv1a64(bytes []byte) uint64 {
	hash := uint64(0xcbf29ce484222325)
	for _, b := range bytes {
		hash ^= uint64(b)
		hash *= 0x100000001b3
	}
	return hash
}

// hslToRGB convierte HSL (h en [0,1)) a RGB de 8 bits.
func hslToRGB(h, s, l float64) [3]byte {
	c := (1.0 - math.Abs(2.0*l-1.0)) * s
	x := c * (1.0 - math.Abs(math.Mod(h*6.0, 2.0)-1.0))
	m := l - c/2.0

	var r, g, b float64
	switch int(h * 6.0) {
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

	return [3]byte{
		byte(math.Round((r + m) * 255.0)),
		byte(math.Round((g + m) * 255.0)),
		byte(math.Round((b + m) * 255.0)),
	}
}

// ownExeName es el nombre en minúsculas de este ejecutable.
func ownExeName() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return strings.ToLower(filepath.Base(exe))
}
