// Package settings guarda las opciones globales de Compass (las que no
// dependen de un proyecto concreto) en %LOCALAPPDATA%\compass\settings.json.
//
// Es el equivalente ligero de theme.Save: un fichero pequeno, sin dependencias
// y tolerante a errores (si falta o esta corrupto se usan los valores por
// defecto).
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Settings son las opciones globales del usuario.
type Settings struct {
	// OpenOutputDir abre el Explorer en la carpeta/artefacto de salida al
	// terminar un build. Por defecto si (comportamiento historico).
	OpenOutputDir bool `json:"open_output_dir"`

	// OpenRouterEndpoint es la base de la API de OpenRouter. Por defecto
	// https://openrouter.ai/api/v1 (se le anade /chat/completions).
	OpenRouterEndpoint string `json:"openrouter_endpoint"`

	// OpenRouterModel es el modelo usado para generar mensajes de commit.
	OpenRouterModel string `json:"openrouter_model"`

	// OpenRouterAPIKey es el bearer token de https://openrouter.ai/keys.
	OpenRouterAPIKey string `json:"openrouter_api_key"`
}

// Default devuelve los valores de fabrica.
func Default() Settings {
	return Settings{
		OpenOutputDir:      true,
		OpenRouterEndpoint: "https://openrouter.ai/api/v1",
		// Vacio: GenerateCommit elige el gratuito mas rapido disponible si el
		// usuario no ha seleccionado uno en Opciones.
		OpenRouterModel: "",
	}
}

func configPath() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return ""
	}
	return filepath.Join(base, "compass", "settings.json")
}

// Load lee las opciones guardadas. Un fichero ausente o invalido devuelve los
// valores por defecto (nunca es un error).
func Load() Settings {
	s := Default()
	path := configPath()
	if path == "" {
		return s
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return Default()
	}
	def := Default()
	// Los campos antiguos de Ollama/opencode que pueda haber en el fichero se
	// ignoran (no estan en el struct) y desaparecen al siguiente Save.
	if strings.TrimSpace(s.OpenRouterEndpoint) == "" {
		s.OpenRouterEndpoint = def.OpenRouterEndpoint
	}
	if strings.TrimSpace(s.OpenRouterModel) == "" {
		s.OpenRouterModel = def.OpenRouterModel
	}
	return s
}

// Save persiste las opciones. Un fallo de escritura no es fatal.
func Save(s Settings) {
	path := configPath()
	if path == "" {
		return
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, append(data, '\n'), 0o644)
}
