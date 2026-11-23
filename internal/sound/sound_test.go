package sound

import "testing"

func TestNormalizeFillsDefaultsAndDropsUnknown(t *testing.T) {
	got := Normalize(map[string]string{
		"build_ok":     "soft",
		"build_error":  "desconocido",
		"commit_ok":    "  bright  ",
		"evento_falso": "classic",
	})
	if got["build_ok"] != "soft" {
		t.Fatalf("build_ok deberia conservar soft, got %q", got["build_ok"])
	}
	if got["build_error"] != "classic" {
		t.Fatalf("un preset desconocido deberia caer en classic, got %q", got["build_error"])
	}
	if got["commit_ok"] != "bright" {
		t.Fatalf("commit_ok deberia recortar espacios, got %q", got["commit_ok"])
	}
	if _, ok := got["evento_falso"]; ok {
		t.Fatalf("Normalize no deberia conservar eventos desconocidos")
	}
	if got["task_ok"] != "none" || got["task_error"] != "none" {
		t.Fatalf("los eventos de terminal deberian quedar en none por defecto")
	}
	if len(got) != len(Events()) {
		t.Fatalf("Normalize deberia devolver un preset por evento, got %d", len(got))
	}
}

func TestCatalogHasPresetsForEveryEvent(t *testing.T) {
	valid := map[string]bool{}
	for _, p := range Presets() {
		valid[p.ID] = true
	}
	for _, ev := range Events() {
		if !valid[ev.DefaultPreset] {
			t.Fatalf("el evento %s apunta a un preset inexistente %q", ev.ID, ev.DefaultPreset)
		}
	}
}
