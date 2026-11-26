package webui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"compass/internal/settings"
	"compass/internal/workspace"
)

func TestIsConflict(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{" M", false},
		{"??", false},
		{"UU", true},
		{"AA", true},
		{"UD", true},
		{" M", false},
	}
	for _, tc := range cases {
		if got := isConflict(tc.in); got != tc.want {
			t.Fatalf("isConflict(%q) = %v, queria %v", tc.in, got, tc.want)
		}
	}
}

// Volver a guardar un archivo que YA estaba modificado debe reiniciar el reloj:
// si la firma solo mirase los codigos de git status, la segunda edicion pasaba
// desapercibida y el commit llegaba a mitad de la escritura.
func TestAutoSigDetectsRepeatedEdits(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	dir := initRepo(t)
	path := filepath.Join(dir, "nota.txt")
	if err := os.WriteFile(path, []byte("uno\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	first, _, err := autoSig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if first == "" {
		t.Fatal("la firma no deberia estar vacia con un archivo modificado")
	}

	// Mismo estado git ("??" / "M"), contenido distinto: la firma cambia.
	if err := os.WriteFile(path, []byte("dos, mas largo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, _, err := autoSig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Fatal("editar otra vez el mismo archivo debe cambiar la firma")
	}

	// Sin tocar nada, la firma es estable.
	third, _, err := autoSig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if third != second {
		t.Fatal("la firma debe ser estable cuando no hay actividad")
	}
}

func TestFirstLine(t *testing.T) {
	if got := firstLine("feat: algo\n\n- detalle"); got != "feat: algo" {
		t.Fatalf("firstLine = %q", got)
	}
	if got := firstLine("  solo una  "); got != "solo una" {
		t.Fatalf("firstLine = %q", got)
	}
}

func TestSetAutoCommitRequiresRepo(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module demo\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := NewBridge(dir)
	if err := b.SetAutoCommit(true); err == nil {
		t.Fatal("activar Auto sin repositorio debe fallar")
	}
	if b.AutoCommitStatus().Enabled {
		t.Fatal("el vigilante no debe quedar activo")
	}
}

func TestSetAutoCommitToggle(t *testing.T) {
	dir := initRepo(t)
	b := NewBridge(dir)

	if b.AutoCommitStatus().Enabled {
		t.Fatal("Auto empieza apagado")
	}
	if err := b.SetAutoCommit(true); err != nil {
		t.Fatalf("activar Auto: %v", err)
	}
	st := b.AutoCommitStatus()
	if !st.Enabled || st.IdleMin != 5 {
		t.Fatalf("estado inesperado: %+v", st)
	}
	if err := b.SetAutoCommit(false); err != nil {
		t.Fatalf("desactivar Auto: %v", err)
	}
	if b.AutoCommitStatus().Enabled {
		t.Fatal("Auto debe quedar apagado")
	}
}

// Reiniciar Compass no debe apagar el boton "Auto": el estado se persiste por
// proyecto y el vigilante se rearma al arrancar.
func TestAutoCommitSurvivesRestart(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	dir := initRepo(t)

	b := NewBridge(dir)
	if err := b.SetAutoCommit(true); err != nil {
		t.Fatalf("activar Auto: %v", err)
	}

	// Un Bridge nuevo sobre la misma configuracion equivale a reiniciar la app.
	b2 := NewBridge(dir)
	if !b2.AutoCommitStatus().Enabled {
		t.Fatal("Auto debe seguir activo tras reiniciar")
	}

	if err := b2.SetAutoCommit(false); err != nil {
		t.Fatalf("desactivar Auto: %v", err)
	}
	b3 := NewBridge(dir)
	if b3.AutoCommitStatus().Enabled {
		t.Fatal("un Auto desactivado no debe rearmarse")
	}
}

// Un arbol con cambios que no se mueven acaba commiteado solo. Se baja el
// umbral a 1 minuto en settings para no esperar los 5 reales.
func TestAutoCommitCommitsAfterIdle(t *testing.T) {
	dir := initRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "nota.txt"), []byte("hola\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := settings.Default()
	cfg.AutoCommitIdleMin = 1
	settings.Save(cfg)

	b := NewBridge(dir)
	if err := b.SetAutoCommit(true); err != nil {
		t.Fatalf("activar Auto: %v", err)
	}

	b.autoMu.Lock()
	r := b.autoRuns[commitKey(dir)]
	r.changedAt = r.changedAt.Add(-2 * time.Minute)
	b.autoMu.Unlock()

	b.autoStep(r)

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if out, err := runGit(dir, "log", "--oneline"); err == nil && strings.TrimSpace(out) != "" {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("no se creo el commit automatico")
}

// El toggle vive en la card de cada proyecto: activar A no debe activar B,
// y un directorio fuera del workspace no puede enrolarse.
func TestSetAutoCommitForIsPerProject(t *testing.T) {
	dirA := initRepo(t)
	dirB := initRepo(t)
	b := NewBridge(dirA)
	b.mu.Lock()
	b.ws.Projects = []*workspace.Project{
		{Path: dirA},
		{Path: dirB},
	}
	b.mu.Unlock()

	if err := b.SetAutoCommitFor(dirA, true); err != nil {
		t.Fatalf("activar A: %v", err)
	}
	if !b.AutoCommitStatusFor(dirA).Enabled || b.AutoCommitStatusFor(dirB).Enabled {
		t.Fatal("activar A no debe activar B")
	}
	if err := b.SetAutoCommitFor(dirB, true); err != nil {
		t.Fatalf("activar B: %v", err)
	}
	if !b.AutoCommitStatusFor(dirA).Enabled || !b.AutoCommitStatusFor(dirB).Enabled {
		t.Fatal("A y B deben quedar activos de forma independiente")
	}
	if err := b.SetAutoCommitFor(dirA, false); err != nil {
		t.Fatalf("desactivar A: %v", err)
	}
	if b.AutoCommitStatusFor(dirA).Enabled || !b.AutoCommitStatusFor(dirB).Enabled {
		t.Fatal("desactivar A no debe tocar B")
	}
	outside := filepath.Join(t.TempDir(), "ajeno")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := b.SetAutoCommitFor(outside, true); err == nil {
		t.Fatal("un proyecto fuera del workspace debe rechazarse")
	}
}

// git:auto debe avisar una vez por intento, con el resultado, aunque falle.
func TestAutoCommitEmitsResultEvent(t *testing.T) {
	dir := initRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "nota.txt"), []byte("hola\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := settings.Default()
	cfg.AutoCommitIdleMin = 1
	settings.Save(cfg)

	b := NewBridge(dir)
	if err := b.SetAutoCommit(true); err != nil {
		t.Fatalf("activar Auto: %v", err)
	}
	b.autoMu.Lock()
	r := b.autoRuns[commitKey(dir)]
	r.changedAt = r.changedAt.Add(-2 * time.Minute)
	b.autoMu.Unlock()

	events := make(chan map[string]any, 4)
	prev := emitHook
	emitHook = func(name string, data any) {
		if name != "git:auto" {
			return
		}
		if m, ok := data.(map[string]any); ok {
			events <- m
		}
	}
	defer func() { emitHook = prev }()

	b.autoStep(r)
	select {
	case e := <-events:
		if e["ok"] != true || e["dir"] != dir || e["error"] != "" || e["message"] == "" {
			t.Fatalf("evento de exito inesperado: %+v", e)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("no se emitio git:auto tras el commit")
	}
}

// Un commit que falla también debe emitir git:auto, con ok=false y el error,
// para que la UI pueda mostrar el toast rojo.
func TestAutoCommitEmitsFailureEvent(t *testing.T) {
	dir := initRepo(t)
	hook := filepath.Join(dir, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nota.txt"), []byte("hola\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := settings.Default()
	cfg.AutoCommitIdleMin = 1
	settings.Save(cfg)
	b := NewBridge(dir)
	if err := b.SetAutoCommit(true); err != nil {
		t.Fatalf("activar Auto: %v", err)
	}
	b.autoMu.Lock()
	r := b.autoRuns[commitKey(dir)]
	r.changedAt = r.changedAt.Add(-2 * time.Minute)
	b.autoMu.Unlock()

	events := make(chan map[string]any, 4)
	prev := emitHook
	emitHook = func(name string, data any) {
		if name != "git:auto" {
			return
		}
		if m, ok := data.(map[string]any); ok {
			events <- m
		}
	}
	defer func() { emitHook = prev }()

	b.autoStep(r)
	select {
	case e := <-events:
		if e["ok"] != false || e["dir"] != dir {
			t.Fatalf("evento de fallo inesperado: %+v", e)
		}
		if errText, ok := e["error"].(string); !ok || errText == "" {
			t.Fatalf("el evento de fallo debe traer el error: %+v", e)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("no se emitio git:auto tras el fallo del commit")
	}
}
