package webui

import "testing"

// Cancelar la generacion de un proyecto no debe tocar la de otro: cada
// proyecto tiene su propia entrada en commitCancels.
func TestCancelCommitIsPerProject(t *testing.T) {
	b := &Bridge{commitCancels: map[string]*commitRun{}}

	var cancelled []string
	b.commitCancels[commitKey(`C:\proj\a`)] = &commitRun{id: 1, cancel: func() {
		cancelled = append(cancelled, "a")
	}}
	b.commitCancels[commitKey(`C:\proj\b`)] = &commitRun{id: 2, cancel: func() {
		cancelled = append(cancelled, "b")
	}}

	b.CancelCommit(`C:\proj\b`)

	if len(cancelled) != 1 || cancelled[0] != "b" {
		t.Fatalf("CancelCommit cancelo %v, queria solo el proyecto b", cancelled)
	}
}

// commitKey normaliza mayusculas y rutas relativas, para que dos formas de
// escribir el mismo proyecto compartan entrada.
func TestCommitKeyNormalizes(t *testing.T) {
	a := commitKey(`C:\Proj\App`)
	b := commitKey(`c:\proj\app`)
	if a != b {
		t.Errorf("commitKey no normaliza mayusculas: %q != %q", a, b)
	}
}
