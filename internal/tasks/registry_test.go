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
	// Los corekits no tienen build, release ni version propio.
	apps := appTypes()
	required := map[string][]project.ProjectType{
		"windows-dev":     apps,
		"windows-build":   apps,
		"windows-release": apps,
		"android-release": apps,
		"clean":           all,
		"signing":         all,
		"version":         apps,
		"git":             all,
		"terminal":        all,
		"environment":     all,

		"android-dev":           {project.TypeFlutter, project.TypeKotlin, project.TypeGoWails},
		"android-hmr":           {project.TypeGoWails, project.TypePython},
		"android-aab":           {project.TypeFlutter, project.TypeKotlin, project.TypeGoWails},
		"android-install-debug": {project.TypeKotlin, project.TypeGoWails},
		"debug-android":         {project.TypeFlutter, project.TypeKotlin, project.TypeGoWails},
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

// TestKitTypesOnlyHaveGenericTasks fija el contrato de los corekits: se abren
// para versionarlos (git, terminal, entorno, limpieza), pero no deben ofrecer
// build, release, firma ni version, que no aplican a una libreria.
func TestKitTypesOnlyHaveGenericTasks(t *testing.T) {
	for _, pt := range []project.ProjectType{project.TypeWhatsappKit, project.TypeKitD1} {
		for _, task := range []string{"windows-dev", "windows-build", "windows-release", "version", "go-test"} {
			if _, ok := registry[fmt.Sprintf("%s|%d", task, pt)]; ok {
				t.Errorf("%s no deberia tener la tarea %q", pt.Label(), task)
			}
		}
		for _, task := range []string{"clean", "git", "terminal", "environment"} {
			if _, ok := registry[fmt.Sprintf("%s|%d", task, pt)]; !ok {
				t.Errorf("%s deberia tener la tarea %q", pt.Label(), task)
			}
		}
	}
}
