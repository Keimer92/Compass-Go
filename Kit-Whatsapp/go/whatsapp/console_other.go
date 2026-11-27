//go:build !windows

package whatsapp

import "os/exec"

func hideConsoleWindow(*exec.Cmd) {}
