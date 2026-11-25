package webui

import "compass/internal/sysinfo"

// GetSystemInfo devuelve CPU, RAM, GPU y discos para el panel derecho. La
// lectura es barata salvo la primera (la GPU se consulta una sola vez), asi
// que el frontend puede refrescarla cada pocos segundos.
func (b *Bridge) GetSystemInfo() sysinfo.Info {
	return sysinfo.Snapshot()
}
