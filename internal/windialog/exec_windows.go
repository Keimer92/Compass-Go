//go:build windows

package windialog

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// NoWindow marca un proceso hijo para que no abra ventana de consola. Necesario
// porque el exe es una app GUI: sin esto, cada `git status` del explorador
// parpadea una consola al ejecutarse.
func NoWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
}
