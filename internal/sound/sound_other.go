//go:build !windows

package sound

// Event identifica un aviso sonoro de Compass.
type Event string

const (
	EventBuildOK     Event = "build_ok"
	EventBuildError  Event = "build_error"
	EventCommitOK    Event = "commit_ok"
	EventCommitError Event = "commit_error"
	EventTaskOK      Event = "task_ok"
	EventTaskError   Event = "task_error"
)

// EventInfo describe un evento para la UI de Opciones.
type EventInfo struct {
	ID            string `json:"id"`
	Label         string `json:"label"`
	Description   string `json:"description"`
	DefaultPreset string `json:"default_preset"`
}

// PresetInfo describe un conjunto de sonidos para la UI de Opciones.
type PresetInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

var eventList = []EventInfo{
	{string(EventBuildOK), "Build completado", "Al terminar un build sin errores", "classic"},
	{string(EventBuildError), "Build fallido", "Cuando un build termina con error", "classic"},
	{string(EventCommitOK), "Commit creado", "Al crear un commit, manual o automatico", "classic"},
	{string(EventCommitError), "Commit fallido", "Cuando un commit no se puede crear", "classic"},
	{string(EventTaskOK), "Tarea completada", "Cuando un proceso del terminal termina bien", "none"},
	{string(EventTaskError), "Tarea fallida", "Cuando un proceso del terminal termina con error", "none"},
}

var presetList = []PresetInfo{
	{"classic", "Clasico", "Los tonos historicos de Compass"},
	{"soft", "Suave", "Tonos graves y relajados"},
	{"bright", "Brillante", "Tonos agudos y rapidos"},
	{"deep", "Grave", "Tonos bajos y secos"},
	{"none", "Silencio", "Sin sonido"},
}

// Events devuelve los eventos disponibles en el orden de la UI.
func Events() []EventInfo {
	out := make([]EventInfo, len(eventList))
	copy(out, eventList)
	return out
}

// Presets devuelve los presets disponibles en el orden de la UI.
func Presets() []PresetInfo {
	out := make([]PresetInfo, len(presetList))
	copy(out, presetList)
	return out
}

// Normalize completa un mapa evento->preset con los valores por defecto.
func Normalize(m map[string]string) map[string]string {
	out := make(map[string]string, len(eventList))
	for _, ev := range eventList {
		out[ev.ID] = ev.DefaultPreset
	}
	return out
}

// Play es un no-op fuera de Windows (Compass solo se distribuye en Windows).
func Play(Event) {}

// Preview es un no-op fuera de Windows.
func Preview(Event, string) {}

// Success es un no-op fuera de Windows.
func Success() {}

// Failure es un no-op fuera de Windows.
func Failure() {}
