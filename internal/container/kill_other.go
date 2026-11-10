//go:build !windows

package container

import (
	"os"
	"syscall"
)

// killTree mata el grupo de procesos del shell de la pestana.
func killTree(pid int) {
	if pid <= 0 {
		return
	}
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Signal(syscall.SIGKILL)
	}
}
