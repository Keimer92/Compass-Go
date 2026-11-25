//go:build !windows

package sysinfo

import "runtime"

// Snapshot devuelve lo poco que se puede leer sin APIs de Windows. Compass Go
// se distribuye para Windows; esto solo mantiene `go build ./...` en otros SO.
func Snapshot() Info {
	return Info{CPU: CPU{Cores: runtime.NumCPU()}}
}
