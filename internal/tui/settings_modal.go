package tui

import "compass/internal/settings"

// SettingsModal es el estado del modal de opciones globales (Ctrl+O).
//
// Solo tiene una casilla hoy: abrir o no la carpeta de salida al terminar un
// build. El aviso sonoro de fin de build es permanente y no se puede apagar.
type SettingsModal struct {
	openOutputDir bool
}

func newSettingsModal() *SettingsModal {
	return &SettingsModal{openOutputDir: settings.Load().OpenOutputDir}
}

// toggle invierte la casilla activa y la persiste al instante.
func (m *SettingsModal) toggle() {
	m.openOutputDir = !m.openOutputDir
	s := settings.Load()
	s.OpenOutputDir = m.openOutputDir
	settings.Save(s)
}
