//go:build !windows

package whatsapp

import "os/exec"

// assignToJob no aplica fuera de Windows: el hijo muere con el grupo de
// procesos del host.
func assignToJob(cmd *exec.Cmd) {}
