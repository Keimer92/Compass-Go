package webui

import (
	"strings"
	"testing"
)

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

// La card Vibe Coding esta en todos los proyectos y abre el modal de subcards
// con una entrada por CLI mas las dos acciones de mantenimiento.
func TestVibeCardAndSubcards(t *testing.T) {
	dir := initRepo(t)
	b := NewBridge(dir)

	id := ""
	section := ""
	for _, it := range b.GetMenu() {
		if it.Label == "Vibe Coding" {
			id = it.ID
			section = it.Section
		}
	}
	if id == "" {
		t.Fatal("falta la card 'Vibe Coding'")
	}
	// Debe verse en la pestana principal: si cae en CFG solo aparece en Config.
	if section == "CFG" {
		t.Error("la card 'Vibe Coding' no deberia quedar solo en la pestana Config")
	}

	subs := b.Subcards(id)
	if len(subs) < 3 {
		t.Fatalf("subcards = %d, queria al menos 3", len(subs))
	}
	for _, s := range subs {
		if s.Label == "" || s.Task == "" || s.Icon == "" {
			t.Errorf("subcard incompleta: %+v", s)
		}
	}
	if subs[len(subs)-2].Task != "vibe-setup" {
		t.Errorf("penultima subcard = %q, queria vibe-setup", subs[len(subs)-2].Task)
	}
	if subs[len(subs)-1].Task != "opencode-configure" {
		t.Errorf("ultima subcard = %q, queria opencode-configure", subs[len(subs)-1].Task)
	}
}

// Una CLI no instalada no debe romper la pestana: el script avisa en vez de
// intentar lanzar una ruta vacia.
func TestVibeRunScriptFallsBackWhenMissing(t *testing.T) {
	dir := initRepo(t)
	b := NewBridge(dir)

	script := b.TaskScript("vibe-run-noexiste")
	if !strings.Contains(script, "no esta instalada") {
		t.Errorf("script = %q, queria el aviso de CLI no instalada", script)
	}
}
