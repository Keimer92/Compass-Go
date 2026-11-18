//go:build windows

package container

import (
	"os/exec"
	"strconv"
	"syscall"
)

// createNoWindow evita que taskkill abra una consola visible (parpadeo) al
// cerrar una pestaña desde la app GUI.
const createNoWindow = 0x08000000

// killTree mata el proceso y todos sus descendientes.
func killTree(pid int) {
	if pid == 0 {
		return
	}
	cmd := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F")
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
	_ = cmd.Run()
}
