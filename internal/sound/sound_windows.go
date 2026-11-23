//go:build windows

// Package sound emite los avisos sonoros de Compass.
//
// Usa `Beep` de kernel32 (en Windows 7+ se redirige al dispositivo de audio
// predeterminado), asi que no depende de ficheros WAV ni de PowerShell y no
// anade latencia perceptible al cierre de la pestana.
//
// Cada evento (build, commit, terminal) tiene un preset elegible desde
// Opciones. Los presets son secuencias de tonos: no hay ficheros de audio que
// instalar y el usuario puede escucharlos antes de elegirlos.
package sound

import (
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"

	"compass/internal/settings"
)

// Event identifica un aviso sonoro de Compass.
type Event string

const (
	// EventBuildOK suena cuando un build termina sin errores.
	EventBuildOK Event = "build_ok"
	// EventBuildError suena cuando un build falla.
	EventBuildError Event = "build_error"
	// EventCommitOK suena al crear un commit (manual o automatico).
	EventCommitOK Event = "commit_ok"
	// EventCommitError suena cuando un commit no se puede crear.
	EventCommitError Event = "commit_error"
	// EventTaskOK suena cuando un proceso del terminal termina bien.
	EventTaskOK Event = "task_ok"
	// EventTaskError suena cuando un proceso del terminal termina con error.
	EventTaskError Event = "task_error"
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

// eventList es el orden en el que la UI muestra los eventos.
var eventList = []EventInfo{
	{string(EventBuildOK), "Build completado", "Al terminar un build sin errores", "classic"},
	{string(EventBuildError), "Build fallido", "Cuando un build termina con error", "classic"},
	{string(EventCommitOK), "Commit creado", "Al crear un commit, manual o automatico", "classic"},
	{string(EventCommitError), "Commit fallido", "Cuando un commit no se puede crear", "classic"},
	{string(EventTaskOK), "Tarea completada", "Cuando un proceso del terminal termina bien", "none"},
	{string(EventTaskError), "Tarea fallida", "Cuando un proceso del terminal termina con error", "none"},
}

// presetList es el orden en el que la UI muestra los presets.
var presetList = []PresetInfo{
	{"classic", "Clasico", "Los tonos historicos de Compass"},
	{"soft", "Suave", "Tonos graves y relajados"},
	{"bright", "Brillante", "Tonos agudos y rapidos"},
	{"deep", "Grave", "Tonos bajos y secos"},
	{"none", "Silencio", "Sin sonido"},
}

// tone es una nota de la secuencia. gapMs es la pausa posterior.
type tone struct {
	freq  uint32
	ms    uint32
	gapMs uint32
}

// tonesByPreset guarda la secuencia de cada preset para cada evento.
var tonesByPreset = map[string]map[Event][]tone{
	"classic": {
		EventBuildOK:     {{880, 110, 20}, {1175, 110, 20}, {1568, 180, 0}},
		EventBuildError:  {{440, 160, 30}, {311, 260, 0}},
		EventCommitOK:    {{988, 90, 20}, {1319, 140, 0}},
		EventCommitError: {{392, 120, 25}, {294, 200, 0}},
		EventTaskOK:      {{1047, 80, 20}, {1319, 80, 20}, {1568, 120, 0}},
		EventTaskError:   {{330, 150, 25}, {262, 200, 0}},
	},
	"soft": {
		EventBuildOK:     {{523, 150, 40}, {659, 150, 40}, {784, 220, 0}},
		EventBuildError:  {{349, 180, 40}, {262, 280, 0}},
		EventCommitOK:    {{587, 120, 30}, {698, 160, 0}},
		EventCommitError: {{311, 150, 30}, {233, 240, 0}},
		EventTaskOK:      {{659, 120, 30}, {784, 160, 0}},
		EventTaskError:   {{294, 160, 30}, {220, 220, 0}},
	},
	"bright": {
		EventBuildOK:     {{1047, 90, 15}, {1319, 90, 15}, {1568, 90, 15}, {2093, 160, 0}},
		EventBuildError:  {{523, 100, 20}, {415, 100, 20}, {330, 180, 0}},
		EventCommitOK:    {{1175, 70, 15}, {1568, 110, 0}},
		EventCommitError: {{466, 100, 20}, {349, 160, 0}},
		EventTaskOK:      {{1319, 70, 15}, {1760, 120, 0}},
		EventTaskError:   {{415, 100, 20}, {311, 180, 0}},
	},
	"deep": {
		EventBuildOK:     {{262, 160, 40}, {330, 160, 40}, {392, 240, 0}},
		EventBuildError:  {{220, 200, 40}, {165, 300, 0}},
		EventCommitOK:    {{294, 130, 30}, {349, 170, 0}},
		EventCommitError: {{196, 180, 35}, {147, 280, 0}},
		EventTaskOK:      {{330, 120, 30}, {392, 180, 0}},
		EventTaskError:   {{185, 180, 35}, {139, 260, 0}},
	},
	"none": {},
}

var (
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	procBeep = kernel32.NewProc("Beep")
	playMu   sync.Mutex
)

func beep(freq, ms uint32) {
	// Beep es sincrono: bloquea hasta que termina el tono.
	_, _, _ = procBeep.Call(uintptr(freq), uintptr(ms))
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

func validPreset(id string) bool {
	_, ok := tonesByPreset[id]
	return ok
}

// Normalize completa un mapa evento->preset con los valores por defecto y
// descarta ids desconocidos. Se usa al leer y al guardar las opciones.
func Normalize(m map[string]string) map[string]string {
	out := make(map[string]string, len(eventList))
	for _, ev := range eventList {
		p := strings.TrimSpace(m[ev.ID])
		if !validPreset(p) {
			p = ev.DefaultPreset
		}
		out[ev.ID] = p
	}
	return out
}

// presetFor resuelve el preset guardado para un evento.
func presetFor(e Event) string {
	m := settings.Load().SoundPresets
	for _, ev := range eventList {
		if ev.ID != string(e) {
			continue
		}
		p := strings.TrimSpace(m[ev.ID])
		if validPreset(p) {
			return p
		}
		return ev.DefaultPreset
	}
	return "classic"
}

func playTones(tones []tone) {
	if len(tones) == 0 {
		return
	}
	playMu.Lock()
	defer playMu.Unlock()
	for _, t := range tones {
		beep(t.freq, t.ms)
		if t.gapMs > 0 {
			time.Sleep(time.Duration(t.gapMs) * time.Millisecond)
		}
	}
}

func playAsync(e Event, preset string) {
	tones := tonesByPreset[preset][e]
	go playTones(tones)
}

// Play reproduce el sonido configurado para el evento.
func Play(e Event) {
	playAsync(e, presetFor(e))
}

// Preview reproduce un evento con un preset concreto sin tocar lo guardado.
// Lo usa el boton de escucha de Opciones.
func Preview(e Event, preset string) {
	if !validPreset(preset) {
		preset = "classic"
	}
	playAsync(e, preset)
}

// Success suena una triada ascendente cuando el build termino bien.
func Success() { Play(EventBuildOK) }

// Failure suena dos tonos graves cuando el build fallo.
func Failure() { Play(EventBuildError) }
