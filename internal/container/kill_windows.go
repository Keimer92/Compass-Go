//go:build windows

package container

import (
	"os/exec"
	"strconv"
)

// killTree mata el proceso y todos sus descendientes.
func killTree(pid int) {
	if pid == 0 {
		return
	}
	cmd := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F")
	cmd.Stdout = nil
	cmd.Stderr = nil
	_ = cmd.Run()
}
