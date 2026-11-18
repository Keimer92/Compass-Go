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

// noWindowAttrs evita que los procesos de tareas abran una consola visible
// (parpadeo) cuando Compass corre como app GUI sin consola propia. Se aplica
// SIEMPRE: los hijos heredan los handles de stdio, asi que su salida sigue
// llegando a la ConPTY aunque no tengan ventana.
func noWindowAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: createNoWindow}
}

// WindowScript construye el script de una ventana nueva: titulo y comando.
func WindowScript(title, command string, interactive bool) string {
	script := fmt.Sprintf("[Console]::Title = %s; & %s", PsQuote(title), command)
	if interactive {
		script += "; Read-Host 'Presiona Enter para cerrar' | Out-Null"
	}
	return script
}

// SpawnNewWindowAutoexit ejecuta el comando en la consola actual (la ConPTY
// embebida) y retorna al terminar. Ya NO abre una ventana externa: Compass es
// el propio terminal y todo debe responder dentro de la app.
func SpawnNewWindowAutoexit(title, dir, command string) error {
	_ = title
	RunShellStatus(command, dir, nil)
	return nil
}

// SpawnNewWindowInteractive ejecuta el comando interactivo en la consola actual
// (la ConPTY embebida), con stdin funcional. No abre ventana externa.
func SpawnNewWindowInteractive(title, dir, command string) error {
	_ = title
	RunShellStatus(command, dir, nil)
	return nil
}

// RunPSFileStatus ejecuta un .ps1 con -File en dir, heredando stdio.
func RunPSFileStatus(script string, args []string, dir string, envs map[string]string) (bool, error) {
	if !dirExists(dir) {
		return false, fmt.Errorf("el directorio no existe: %s", dir)
	}
	// Hereda la consola: la salida debe verse en la ConPTY.
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
