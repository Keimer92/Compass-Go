package shell

import (
	"math"
	"testing"
)

// hueOf es el tono que TabAccent usaría para p si fuera la primera instancia.
func hueOf(p string) float64 {
	return accentHue(fnv1a64([]byte(projectKey(p))), 0)
}

// arc es la distancia corta entre dos tonos de la rueda.
func arc(a, b float64) float64 {
	d := math.Mod(math.Abs(a-b), 360.0)
	return math.Min(d, 360.0-d)
}

func TestTabAccentIsStablePerProject(t *testing.T) {
	a := `D:\Proyectos\School Tracker`
	if TabAccent(a) != TabAccent(a) {
		t.Fatal("el mismo proyecto debe conservar el color")
	}
	// En Windows el path no distingue mayúsculas.
	b := `d:\proyectos\school tracker`
	if TabAccent(a) != TabAccent(b) {
		t.Fatalf("mayúsculas distintas: %v != %v", TabAccent(a), TabAccent(b))
	}
}

func TestProjectTonesLandOnTheTwelveToneGrid(t *testing.T) {
	for _, name := range []string{
		"School Tracker", "SongSync", "Compass", "Go + Wails v3",
		"GymPro", "Bingoku", "AirLyrics", "Activator",
	} {
		hue := hueOf(`D:\Proyectos\` + name)
		rem := math.Mod(hue, 30.0)
		if math.Abs(rem) >= 1e-3 && math.Abs(rem-30.0) >= 1e-3 {
			t.Errorf("%s se salió de la rejilla: %f", name, hue)
		}
	}
}

func TestTonesAreGridSpacedBetweenProjects(t *testing.T) {
	paths := []string{
		`D:\Proyectos\School Tracker`,
		`D:\Proyectos\SongSync`,
		`D:\Proyectos\Compass`,
		`D:\Proyectos\GymPro`,
		`D:\Proyectos\Bingoku`,
		`D:\Proyectos\AirLyrics`,
		`D:\Proyectos\Activator`,
	}
	for i, a := range paths {
		for _, b := range paths[i+1:] {
			d := arc(hueOf(a), hueOf(b))
			if d >= 1e-3 && d < 30.0-1e-3 {
				t.Errorf("%s y %s caen a %.1f° el uno del otro", a, b, d)
			}
		}
	}
}

// Dos instancias abiertas a la vez nunca heredan el mismo color, aunque el
// proyecto sea idéntico: el giro áureo las separa.
func TestLiveInstancesNeverShareAccent(t *testing.T) {
	key := fnv1a64([]byte(`d:\proyectos\compass`))
	tones := make([]float64, 7)
	for k := range tones {
		tones[k] = accentHue(key, k)
	}
	for i, a := range tones {
		for _, b := range tones[i+1:] {
			if arc(a, b) < 30.0 {
				t.Fatalf("instancias del mismo proyecto a %.1f°", arc(a, b))
			}
		}
	}

	names := []string{"School Tracker", "SongSync", "Compass", "GymPro", "Bingoku", "AirLyrics"}
	tones = tones[:0]
	for k, n := range names {
		tones = append(tones, accentHue(fnv1a64([]byte(projectKey(`D:\Proyectos\`+n))), k))
	}
	for i, a := range tones {
		for _, b := range tones[i+1:] {
			if arc(a, b) < 10.0 {
				t.Fatalf("dos instancias de proyectos distintos caen a %.1f°", arc(a, b))
			}
		}
	}
}

func TestTabAccentHasVisibleSaturation(t *testing.T) {
	for _, name := range []string{"School Tracker", "SongSync", "Compass", "Go + Wails v3"} {
		rgb := TabAccent(name)
		max, min := rgb[0], rgb[0]
		for _, c := range rgb[1:] {
			if c > max {
				max = c
			}
			if c < min {
				min = c
			}
		}
		// Un gris no se distinguiría de otra pestaña.
		if int(max)-int(min) <= 60 {
			t.Errorf("color plano para %s: %d,%d,%d", name, rgb[0], rgb[1], rgb[2])
		}
	}
}
