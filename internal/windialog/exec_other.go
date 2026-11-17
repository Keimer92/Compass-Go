//go:build !windows

package windialog

import "os/exec"

// NoWindow no hace nada fuera de Windows.
func NoWindow(cmd *exec.Cmd) {}
