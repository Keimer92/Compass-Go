package webui

import (
	"os"
	"path/filepath"
	"strings"

	"compass/internal/tasks"
)

// ProjectKitInfo dice qué core kits consume un proyecto del workspace. El
// sidebar lo pinta como insignia en su tarjeta, junto a "Auto" y "GitHub".
type ProjectKitInfo struct {
	Path     string `json:"path"`
	Whatsapp bool   `json:"whatsapp"`
	D1       bool   `json:"d1"`
}

// ProjectKits detecta, para cada proyecto abierto, si consume el motor de
// WhatsApp y/o el Kit D1. La detección de WhatsApp recorre las fuentes (imports
// Go o referencias Flutter), así que el frontend lo pide una vez al arrancar y
// al cambiar el workspace, no en cada repintado.
func (b *Bridge) ProjectKits() []ProjectKitInfo {
	b.mu.Lock()
	paths := make([]string, 0, len(b.ws.Projects))
	for _, p := range b.ws.Projects {
		if p != nil && strings.TrimSpace(p.Path) != "" {
			paths = append(paths, p.Path)
		}
	}
	b.mu.Unlock()

	out := make([]ProjectKitInfo, 0, len(paths))
	for _, dir := range paths {
		out = append(out, ProjectKitInfo{
			Path:     dir,
			Whatsapp: tasks.ProjectUsesWhatsappEngineAt(dir),
			D1:       projectUsesKitD1(dir),
		})
	}
	return out
}

// projectUsesKitD1 detecta un consumidor del kit: el módulo Go syncengine en
// go.mod (GymPro, School Tracker) o el servicio D1 de Flutter (ELA Group).
func projectUsesKitD1(dir string) bool {
	if data, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
		if strings.Contains(string(data), kitD1Module) {
			return true
		}
	}
	return fileExists(filepath.Join(dir, "lib", "services", "d1_sync.dart"))
}
