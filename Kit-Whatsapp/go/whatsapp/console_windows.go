//go:build windows

package whatsapp

import (
	"os/exec"
	"syscall"
)

// hideConsoleWindow evita que node.exe abra una consola encima de la ventana.
func hideConsoleWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	// Sin este flag, la máscara de STARTUPINFO (wShowWindow) se ignora y node.exe
	// hereda la consola del proceso padre.
	cmd.SysProcAttr.CreationFlags |= 0x08000000 // CREATE_NO_WINDOW
}
