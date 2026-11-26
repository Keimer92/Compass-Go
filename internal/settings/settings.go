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

	// CloseTabOnBuildEnd cierra la pestana del panel cuando termina una tarea
	// de compilacion (tras la cuenta atras de 10 s). Por defecto NO: la
	// pestana se queda abierta para poder leer la salida con calma.
	CloseTabOnBuildEnd bool `json:"close_tab_on_build_end"`

	// OpenRouterEndpoint es la base de la API de OpenRouter. Por defecto
	// https://openrouter.ai/api/v1 (se le anade /chat/completions).
	OpenRouterEndpoint string `json:"openrouter_endpoint"`

	// OpenRouterModel es el modelo usado para generar mensajes de commit.
	OpenRouterModel string `json:"openrouter_model"`

	// OpenRouterAPIKey es el bearer token de https://openrouter.ai/keys.
	OpenRouterAPIKey string `json:"openrouter_api_key"`
	// AutoCommitIdleMin son los minutos que el arbol debe quedarse quieto
	// antes de que el boton "Auto" del panel Git commitee. 0 o negativo se
	// normaliza al valor por defecto (5).
	AutoCommitIdleMin int `json:"auto_commit_idle_min"`
	// AutoCommitProjects guarda las carpetas con el boton "Auto" activado,
	// para que al reiniciar Compass el vigilante se rearme solo en ellas.
	AutoCommitProjects []string `json:"auto_commit_projects"`
	// CommitProvider elige como se generan los mensajes de commit:
	// "openrouter" (API con key) o "copilot" (GitHub Copilot CLI local).
	CommitProvider string `json:"commit_provider"`
	// CopilotModel es el modelo del CLI de Copilot. Vacio = Auto (equilibrado).
	// "auto-intelligence" pide el perfil de maxima calidad; el resto son ids
	// concretos del catalogo (haiku, flash, luna...).
	CopilotModel string `json:"copilot_model"`
	// SoundPresets elige el preset de sonido de cada evento (build, commit,
	// terminal). Las claves son ids de evento y los valores ids de preset; el
	// paquete sound normaliza ambos y rellena los que falten.
	SoundPresets map[string]string `json:"sound_presets"`

	// TerminalPanelPct es el porcentaje del area central que ocupa el panel
	// de terminales. Lo ajusta el separador arrastrable entre las cards y el
	// terminal; se acota a un rango util para que ninguno de los dos quede
	// inutilizable.
	TerminalPanelPct int `json:"terminal_panel_pct"`

	// VibeDisabled son las CLIs de Vibe Coding que el usuario oculto de la
	// card. Se guarda la lista de deshabilitadas (y no la de habilitadas)
	// para que cualquier CLI nueva aparezca activa sin migrar el fichero.
	VibeDisabled []string `json:"vibe_disabled"`

	// KitD1Path es la raiz de Kit D1 (contiene worker/instances y scripts/).
	// El kit vive dentro de Compass como "Kit D1/" (subtree); vacio = autodetectar
	// junto al proyecto activo o al exe, con fallback al checkout antiguo
	// "Core Station/Kit D1". El panel permite fijarlo explícitamente.
	KitD1Path string `json:"kit_d1_path"`
}

// Default devuelve los valores de fabrica.
func Default() Settings {
	return Settings{
		OpenOutputDir: true,
		// Desactivado: al terminar un build la pestana se queda abierta.
		CloseTabOnBuildEnd: false,
		OpenRouterEndpoint: "https://openrouter.ai/api/v1",
		// Vacio: GenerateCommit elige el gratuito mas rapido disponible si el
		// usuario no ha seleccionado uno en Opciones.
		OpenRouterModel:   "",
		AutoCommitIdleMin: 5,
		CommitProvider:    ProviderOpenRouter,
		TerminalPanelPct:  60,
		// Los avisos de terminal llegan apagados: un build ya suena desde su
		// propio proceso y encender ambos duplicaria el aviso.
		SoundPresets: map[string]string{
			"build_ok":     "classic",
			"build_error":  "classic",
			"commit_ok":    "classic",
			"commit_error": "classic",
			"task_ok":      "none",
			"task_error":   "none",
		},
	}
}

// Proveedores de mensajes de commit.
const (
	ProviderOpenRouter = "openrouter"
	ProviderCopilot    = "copilot"
)

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
	// El umbral del auto-commit se acota a un rango util: un valor 0 (fichero
	// antiguo o entrada vacia) no debe commitear al instante.
	if s.AutoCommitIdleMin < 1 {
		s.AutoCommitIdleMin = def.AutoCommitIdleMin
	}
	if s.AutoCommitIdleMin > 240 {
		s.AutoCommitIdleMin = 240
	}
	// El reparto cards/terminal se acota: 0 (fichero antiguo) cae en el valor
	// por defecto y los extremos no pueden dejar un panel sin espacio.
	if s.TerminalPanelPct < 25 {
		s.TerminalPanelPct = def.TerminalPanelPct
	}
	if s.TerminalPanelPct > 80 {
		s.TerminalPanelPct = 80
	}
	// Un proveedor desconocido (o vacio en un fichero antiguo) cae en el
	// historico: OpenRouter.
	if s.CommitProvider != ProviderCopilot {
		s.CommitProvider = ProviderOpenRouter
	}
	// Rutas vacias o repetidas en la lista de proyectos con Auto: se limpian
	// para que el arranque no intente vigilar entradas invalidas.
	if len(s.AutoCommitProjects) > 0 {
		seen := map[string]bool{}
		clean := make([]string, 0, len(s.AutoCommitProjects))
		for _, p := range s.AutoCommitProjects {
			p = strings.TrimSpace(p)
			if p == "" || seen[strings.ToLower(p)] {
				continue
			}
			seen[strings.ToLower(p)] = true
			clean = append(clean, p)
		}
		s.AutoCommitProjects = clean
	}
	s.VibeDisabled = NormalizeVibeDisabled(s.VibeDisabled)
	return s
}

// NormalizeVibeDisabled limpia la lista de CLIs de Vibe Coding deshabilitadas:
// recorta espacios, descarta vacios y duplicados (sin distinguir mayusculas).
func NormalizeVibeDisabled(keys []string) []string {
	if len(keys) == 0 {
		return nil
	}
	seen := map[string]bool{}
	clean := make([]string, 0, len(keys))
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" || seen[strings.ToLower(k)] {
			continue
		}
		seen[strings.ToLower(k)] = true
		clean = append(clean, k)
	}
	return clean
}

// SameVibeDisabled compara dos listas de CLIs deshabilitadas sin importar el
// orden ni las mayusculas. Se usa para reescribir el menu del Explorador solo
// cuando los interruptores cambian de verdad.
func SameVibeDisabled(a, b []string) bool {
	x, y := NormalizeVibeDisabled(a), NormalizeVibeDisabled(b)
	if len(x) != len(y) {
		return false
	}
	seen := make(map[string]bool, len(x))
	for _, k := range x {
		seen[strings.ToLower(k)] = true
	}
	for _, k := range y {
		if !seen[strings.ToLower(k)] {
			return false
		}
	}
	return true
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
