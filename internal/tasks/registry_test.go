package tasks

import (
	"fmt"
	"testing"

	"compass/internal/project"
)

// TestMenuTasksAreRegisteredForEveryType evita que una card del TUI termine en
// "tarea no implementada": replica la matriz de src/main.rs para las tareas que
// las cards pueden disparar.
func TestMenuTasksAreRegisteredForEveryType(t *testing.T) {
	all := allTypes()
	required := map[string][]project.ProjectType{
		"windows-dev":     all,
		"windows-build":   all,
		"windows-release": all,
		"android-release": all,
		"clean":           all,
		"signing":         all,
		"version":         all,
		"git":             all,
		"whatsapp":        all,
		"terminal":        all,
		"environment":     all,

		"android-dev":           {project.TypeFlutter, project.TypeKotlin, project.TypeGoWails},
		"android-hmr":           {project.TypeGoWails, project.TypePython},
		"android-aab":           {project.TypeFlutter, project.TypeKotlin, project.TypeGoWails},
		"android-install-debug": {project.TypeKotlin, project.TypeGoWails},
		"debug-android":         {project.TypeFlutter, project.TypeKotlin, project.TypeGoWails},
		"flutter-profile":       {project.TypeFlutter},
		"python-test":           {project.TypePython},
		"go-test":               {project.TypeGo},
	}

	for task, types := range required {
		for _, pt := range types {
			key := fmt.Sprintf("%s|%d", task, pt)
			if _, ok := registry[key]; !ok {
				t.Errorf("falta registrar %q para %s", task, pt.Label())
			}
		}
	}
}
