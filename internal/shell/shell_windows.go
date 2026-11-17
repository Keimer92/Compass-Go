//go:build windows

package shell

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// const CREATE_NEW_CONSOLE = 0x00000010
const createNewConsole = 0x00000010
const createNoWindow = 0x08000000

// noWindowAttrs evita que los procesos de tareas abran una consola nueva cuando
// Corren dentro de la ConPTY de Compass (el exe es una app GUI sin consola).
func noWindowAttrs() *syscall.SysProcAttr {
	if Embedded() {
		return &syscall.SysProcAttr{CreationFlags: createNoWindow}
	}
	return nil
}

// WindowScript construye el script de una ventana nueva: titulo y comando.
func WindowScript(title, command string, interactive bool) string {
	script := fmt.Sprintf("[Console]::Title = %s; & %s", PsQuote(title), command)
	if interactive {
		script += "; Read-Host 'Presiona Enter para cerrar' | Out-Null"
	}
	return script
}

// SpawnNewWindowAutoexit abre una ventana que se cierra al terminar el comando.
func SpawnNewWindowAutoexit(title, dir, command string) error {
	if Embedded() {
		RunShellStatus(command, dir, nil)
		return nil
	}
	script := WindowScript(title, command, false)
	cmd := exec.Command(Shell(), "-NoLogo", "-NoProfile", "-Command", script)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewConsole}
	return cmd.Start()
}

// SpawnNewWindowInteractive abre una terminal con stdin funcional para comandos
// interactivos; queda abierta tras el comando.
func SpawnNewWindowInteractive(title, dir, command string) error {
	if Embedded() {
		RunShellStatus(command, dir, nil)
		return nil
	}
	script := WindowScript(title, command, true)
	cmd := exec.Command(Shell(), "-NoLogo", "-NoProfile", "-Command", script)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewConsole}
	return cmd.Start()
}

// RunPSFileStatus ejecuta un .ps1 con -File en dir, heredando stdio.
func RunPSFileStatus(script string, args []string, dir string, envs map[string]string) (bool, error) {
	if !dirExists(dir) {
		return false, fmt.Errorf("el directorio no existe: %s", dir)
	}
	cmd := exec.Command(Shell(), "-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script)
	cmd.Dir = dir
	cmd.Args = append(cmd.Args, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	for k, v := range envs {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	err := cmd.Run()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
