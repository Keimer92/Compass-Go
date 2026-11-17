//go:build !windows

package shell

import (
	"fmt"
	"syscall"
)

func noWindowAttrs() *syscall.SysProcAttr { return nil }

func SpawnNewWindowAutoexit(title, dir, command string) error {
	return fmt.Errorf("Compass solo tiene soporte para Windows")
}

func SpawnNewWindowInteractive(title, dir, command string) error {
	return fmt.Errorf("Compass solo tiene soporte para Windows")
}

func RunPSFileStatus(script string, args []string, dir string, envs map[string]string) (bool, error) {
	return false, fmt.Errorf("Compass solo tiene soporte para Windows")
}
