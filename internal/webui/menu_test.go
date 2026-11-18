package webui

import "testing"

// Un proyecto Go puro no tiene tareas de Android: esas cards no deben
// aparecer porque al pulsarlas no se ejecutaria nada. HMR si aplica (Air).
func TestMenuForPureGoHidesAndroidCards(t *testing.T) {
	dir := initRepo(t)
	b := NewBridge(dir)

	for _, it := range b.GetMenu() {
		switch it.Label {
		case "Reload", "Android", "Install", "Debug Andr", "APK Release", "AAB":
			t.Errorf("la card %q no deberia aparecer en un proyecto Go puro", it.Label)
		}
	}
}

// Go puro es una plataforma de primera clase: sus tareas propias llevan el
// badge "go" y la card HMR (Air) vuelve a estar presente y es funcional.
func TestMenuForPureGoPlatformAndHmr(t *testing.T) {
	dir := initRepo(t)
	b := NewBridge(dir)

	got := map[string]string{}
	for _, it := range b.GetMenu() {
		got[it.Label] = it.Platform
	}

	for _, label := range []string{"Run", "Win Release", "HMR", "Test"} {
		if _, ok := got[label]; !ok {
			t.Errorf("falta la card %q en un proyecto Go puro", label)
		}
		if got[label] != "go" {
			t.Errorf("card %q: platform = %q, queria %q", label, got[label], "go")
		}
	}
	if got["Version"] != "general" {
		t.Errorf("card Version: platform = %q, queria %q", got["Version"], "general")
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
		"Run":         "go",
		"Win Release": "go",
		"HMR":         "go",
		"Test":        "go",
		"Version":     "general",
		"Git":         "general",
		"Clean":       "general",
	}
	for label, platform := range want {
		if got[label] != platform {
			t.Errorf("card %q: platform = %q, queria %q", label, got[label], platform)
		}
	}
}
