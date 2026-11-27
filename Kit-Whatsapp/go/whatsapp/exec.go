package whatsapp

import (
	"io"
	"os/exec"
)

// startEngine lanza el bundle Node con sus tres streams conectados y el cwd en el
// directorio de datos (donde el motor crea `whatsapp_auth`).
func startEngine(node, bundle, workDir string) (*exec.Cmd, io.WriteCloser, io.ReadCloser, io.ReadCloser, error) {
	cmd := exec.Command(node, bundle)
	cmd.Dir = workDir
	hideConsoleWindow(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, nil, nil, err
	}
	return cmd, stdin, stdout, stderr, nil
}
