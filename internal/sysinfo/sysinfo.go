// Package sysinfo expone un resumen del estado del equipo (CPU, RAM, GPU y
// discos) para el panel derecho de la GUI.
package sysinfo

// Info es el snapshot que consume el frontend.
type Info struct {
	CPU   CPU      `json:"cpu"`
	RAM   RAM      `json:"ram"`
	GPU   []string `json:"gpu"`
	Disks []Disk   `json:"disks"`
}

// CPU describe el procesador y su uso desde la ultima consulta.
type CPU struct {
	Name  string  `json:"name"`
	Cores int     `json:"cores"`
	Usage float64 `json:"usage"` // 0-100
}

// RAM va en bytes.
type RAM struct {
	Total uint64 `json:"total"`
	Used  uint64 `json:"used"`
}

// Disk es una unidad con letra visible.
type Disk struct {
	Letter string `json:"letter"` // "C:"
	Label  string `json:"label"`
	Total  uint64 `json:"total"`
	Free   uint64 `json:"free"`
}
