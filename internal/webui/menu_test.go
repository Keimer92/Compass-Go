package webui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"compass/internal/settings"
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
		"Clean":       "general",
	}
	for label, platform := range want {
		if got[label] != platform {
			t.Errorf("card %q: platform = %q, queria %q", label, got[label], platform)
		}
	}
}

// El catalogo de Opciones sigue incluyendo todas las CLIs (instaladas o no)
// con su estado, para poder activarlas aunque no esten en el menu.
func TestVibeToolsCatalogKeepsDisabled(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	dir := initRepo(t)
	b := NewBridge(dir)

	tools := b.GetVibeTools()
	if len(tools) < 2 {
		t.Fatalf("catalogo de CLIs = %d, queria al menos 2", len(tools))
	}
	hidden := tools[0].Key
	// La comparacion debe ignorar mayusculas, como la lista guardada.
	settings.Save(settings.Settings{VibeDisabled: []string{strings.ToLower(hidden)}})

	for _, opt := range b.GetVibeTools() {
		want := opt.Key != hidden
		if opt.Enabled != want {
			t.Errorf("%s: enabled = %v, queria %v", opt.Key, opt.Enabled, want)
		}
	}
	for _, it := range b.GetMenu() {
		if it.ID == "vibe-run-"+hidden {
			t.Errorf("la CLI deshabilitada %q sigue en el menu", hidden)
		}
	}
}

// Un proyecto Go+Wails sin build/android no debe ofrecer las cards de Android:
// al pulsarlas la tarea aborta pidiendo `wails3 update build-assets`. Tampoco
// Signing, que solo tiene sentido con un modulo Android.
func TestWailsWithoutAndroidModuleHidesAndroidCards(t *testing.T) {
	b := newWailsProject(t, false, true)

	android := []string{"And. Dev", "APK Release", "AAB", "Install", "Debug Andr", "Signing"}
	for _, it := range b.GetMenu() {
		for _, label := range android {
			if it.Label == label {
				t.Errorf("la card %q no deberia aparecer sin build/android", label)
			}
		}
	}
	// Las de escritorio siguen estando.
	var sawWindows, sawClean bool
	for _, it := range b.GetMenu() {
		switch it.Label {
		case "Windows":
			sawWindows = true
		case "Clean":
			sawClean = true
		}
	}
	if !sawWindows || !sawClean {
		t.Errorf("faltan cards de escritorio (windows=%v clean=%v)", sawWindows, sawClean)
	}
}

// Con el modulo Android generado (build/android/gradlew.bat) vuelven a salir.
func TestWailsWithAndroidModuleShowsAndroidCards(t *testing.T) {
	b := newWailsProject(t, true, true)

	got := map[string]bool{}
	for _, it := range b.GetMenu() {
		got[it.Label] = true
	}
	for _, label := range []string{"And. Dev", "APK Release", "AAB", "Install", "Debug Andr", "Signing"} {
		if !got[label] {
			t.Errorf("falta la card %q con build/android presente", label)
		}
	}
}

// Sin Taskfile.yml, `wails3 task dev|build|package` aborta con "No Taskfile
// found": las cards de escritorio de Wails no deben ofrecerse. El resto
// (Clean, Version, Entorno) no depende del CLI de Wails. Git ya
// no es una card: vive en la pestana Git del panel derecho.
func TestWailsWithoutTaskfileHidesDesktopCards(t *testing.T) {
	b := newWailsProject(t, false, false)

	got := map[string]bool{}
	for _, it := range b.GetMenu() {
		got[it.Label] = true
	}
	for _, label := range []string{"Windows", "Win Release"} {
		if got[label] {
			t.Errorf("la card %q no deberia aparecer sin Taskfile.yml", label)
		}
	}
	for _, label := range []string{"Clean", "Version", "Entorno"} {
		if !got[label] {
			t.Errorf("falta la card %q, que no depende del CLI de Wails", label)
		}
	}
	if got["Git"] {
		t.Error("la card Git ya no deberia aparecer: se gestiona en la pestana Git")
	}
}

// newWailsProject crea un proyecto Go+Wails minimo (go.mod con la dependencia
// y frontend/package.json). withAndroid anade build/android/gradlew.bat y
// withTaskfile un Taskfile.yml, que es lo que exige `wails3 task`.
func newWailsProject(t *testing.T, withAndroid, withTaskfile bool) *Bridge {
	t.Helper()
	t.Setenv("LOCALAPPDATA", t.TempDir())
	dir := t.TempDir()
	gomod := "module demo\n\ngo 1.22\n\nrequire github.com/wailsapp/wails/v3 v3.0.0\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "frontend"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "frontend", "package.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if withTaskfile {
		taskfile := "version: '3'\n\ntasks:\n  dev:\n    cmds:\n      - go run .\n"
		if err := os.WriteFile(filepath.Join(dir, "Taskfile.yml"), []byte(taskfile), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if withAndroid {
		android := filepath.Join(dir, "build", "android")
		if err := os.MkdirAll(android, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(android, "gradlew.bat"), []byte("@echo off\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// NewBridge hace chdir: se registra aqui para que el cleanup devuelva el
	// cwd original antes de que TempDir intente borrar la carpeta (Windows no
	// deja eliminar el directorio de trabajo actual).
	t.Chdir(dir)
	return NewBridge(dir)
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
