package webui

import "testing"

// Un proyecto Go puro no tiene tareas de Android/HMR: esas cards no deben
// aparecer porque al pulsarlas no se ejecutaria nada.
func TestMenuForPureGoHidesAndroidCards(t *testing.T) {
	dir := initRepo(t)
	b := NewBridge(dir)

	for _, it := range b.GetMenu() {
		switch it.Label {
		case "HMR", "Reload", "Android", "Install", "Debug Andr":
			t.Errorf("la card %q no deberia aparecer en un proyecto Go puro", it.Label)
		}
	}
}

func TestMenuSetsPlatformBadge(t *testing.T) {
	dir := initRepo(t)
	b := NewBridge(dir)

	got := map[string]string{}
	for _, it := range b.GetMenu() {
		got[it.Label] = it.Platform
	}

	want := map[string]string{
		"Run":         "windows",
		"Win Release": "windows",
		"Git":         "general",
		"Clean":       "general",
	}
	for label, platform := range want {
		if got[label] != platform {
			t.Errorf("card %q: platform = %q, queria %q", label, got[label], platform)
		}
	}
}
