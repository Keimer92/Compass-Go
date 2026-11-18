package tasks

import (
	"testing"

	"compass/internal/project"
)

func TestCleanupOptionsAreRegistered(t *testing.T) {
	opts := CleanupOptions()
	if len(opts) == 0 {
		t.Fatal("CleanupOptions no devolvio ninguna accion")
	}
	for _, o := range opts {
		if !Available("clean-do-"+o.Key, project.TypeGo) {
			t.Errorf("la accion %q no esta registrada para Go", o.Key)
		}
	}
}
