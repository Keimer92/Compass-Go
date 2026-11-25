package tasks

import (
	"testing"

	"compass/internal/project"
)

// VibeTools es la fuente de las cards de Vibe Coding y de la cascada del
// Explorador: debe exponer exactamente el catalogo, con claves unicas y estables.
func TestVibeToolsCatalog(t *testing.T) {
	tools := VibeTools()
	if len(tools) != len(vibeTools) {
		t.Fatalf("VibeTools() devolvio %d herramientas, queria %d", len(tools), len(vibeTools))
	}

	seen := map[string]bool{}
	for _, tool := range tools {
		if tool.Key == "" || tool.Title == "" {
			t.Errorf("herramienta incompleta: %+v", tool)
		}
		if seen[tool.Key] {
			t.Errorf("clave repetida: %q", tool.Key)
		}
		seen[tool.Key] = true
		if tool.Found && tool.Path == "" {
			t.Errorf("%q marcada como instalada pero sin ruta", tool.Key)
		}
	}
}

// VibeCommand ignora claves desconocidas en vez de devolver una linea a medias.
func TestVibeCommandUnknownKey(t *testing.T) {
	if got := VibeCommand("no-existe"); got != "" {
		t.Errorf("VibeCommand(no-existe) = %q, queria vacio", got)
	}
}

// La card "vibe" debe existir para todos los tipos de proyecto o GetMenu la
// filtraria y no se veria en ninguno.
func TestVibeTaskRegisteredForAllTypes(t *testing.T) {
	for _, pt := range []project.ProjectType{
		project.TypeFlutter, project.TypeKotlin, project.TypePython,
		project.TypeGo, project.TypeGoWails,
	} {
		if !Available("vibe", pt) {
			t.Errorf("la tarea 'vibe' no esta registrada para %s", pt.Label())
		}
	}
}
