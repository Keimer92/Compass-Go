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

	// CommitModel es un campo legacy de la epoca de opencode. Ya no se usa
	// para generar commits, pero se conserva para migrar el modelo elegido
	// por el usuario a OllamaModel en Load().
	CommitModel string `json:"commit_model,omitempty"`

	// OllamaEndpoint es la base de la API de Ollama Cloud. Por defecto
	// https://ollama.com (se le anade /api/chat).
	OllamaEndpoint string `json:"ollama_endpoint"`

	// OllamaModel es el modelo cloud usado para generar mensajes de commit.
	OllamaModel string `json:"ollama_model"`

	// OllamaAPIKey es el bearer token de https://ollama.com/settings/keys.
	OllamaAPIKey string `json:"ollama_api_key"`
}

// Default devuelve los valores de fabrica.
func Default() Settings {
	return Settings{
		OpenOutputDir:  true,
		OllamaEndpoint: "https://ollama.com",
		OllamaModel:    "gemma4:31b",
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
	// Migracion: el antiguo commit_model (opencode/...) se convierte en el
	// modelo de Ollama siempre que no fuera uno reservado de opencode.
	if strings.TrimSpace(s.OllamaModel) == "" {
		legacy := strings.TrimSpace(s.CommitModel)
		if legacy != "" && !strings.HasPrefix(legacy, "opencode/") &&
			legacy != "hy4-flash-free" && legacy != "nemotron-3.5-lightning-free" {
			s.OllamaModel = legacy
		} else {
			s.OllamaModel = def.OllamaModel
		}
	}
	s.CommitModel = ""
	if strings.TrimSpace(s.OllamaEndpoint) == "" {
		s.OllamaEndpoint = def.OllamaEndpoint
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
