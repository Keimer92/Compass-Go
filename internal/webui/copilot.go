package webui

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"compass/internal/windialog"
)

// Proveedor de mensajes de commit basado en el CLI local de GitHub Copilot.
//
// No usa ninguna API: ejecuta `copilot -s -p <prompt>` con el usuario ya
// autenticado por `copilot login`, asi que consume la suscripcion del usuario
// (premium requests) en vez de una API key. Es mas lento que OpenRouter (~18 s
// medidos), por eso es una opcion y no el proveedor por defecto.
const copilotTimeout = 120 * time.Second

// copilotBinary localiza el CLI. Prefiere el binario nativo (se ejecuta
// directo); el shim de npm es un .cmd y necesita pasar por cmd.exe.
func copilotBinary() (bin string, prefix []string, err error) {
	if base := os.Getenv("LOCALAPPDATA"); base != "" {
		p := filepath.Join(base, "GitHubCopilotCLI", "copilot.exe")
		if st, statErr := os.Stat(p); statErr == nil && !st.IsDir() {
			return p, nil, nil
		}
	}
	if p, lookErr := exec.LookPath("copilot.exe"); lookErr == nil {
		return p, nil, nil
	}
	if p, lookErr := exec.LookPath("copilot"); lookErr == nil {
		low := strings.ToLower(p)
		if strings.HasSuffix(low, ".cmd") || strings.HasSuffix(low, ".bat") {
			return "cmd", []string{"/c", p}, nil
		}
		return p, nil, nil
	}
	return "", nil, errors.New("no se encontró el CLI de Copilot (instálalo con: npm i -g @github/copilot)")
}

// CopilotAvailable permite a Opciones avisar si el CLI esta instalado.
func (b *Bridge) CopilotAvailable() bool {
	_, _, err := copilotBinary()
	return err == nil
}

// copilotCommitMessage pide el mensaje al CLI. Se niegan las herramientas de
// shell y escritura: el mensaje se genera solo con el prompt (que ya lleva el
// resumen de git), sin que el agente toque el repositorio.
func copilotCommitMessage(parent context.Context, prompt string) (string, error) {
	bin, prefix, err := copilotBinary()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(parent, copilotTimeout)
	defer cancel()

	args := append(prefix, "-s", "--deny-tool=shell", "--deny-tool=write", "-p", prompt)
	cmd := exec.CommandContext(ctx, bin, args...)
	windialog.NoWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		if parent.Err() != nil {
			return "", errCommitCancelled
		}
		if ctx.Err() != nil {
			return "", errors.New("copilot: tiempo de espera agotado (120 s)")
		}
		var exitErr *exec.ExitError
		msg := ""
		if errors.As(err, &exitErr) {
			msg = strings.TrimSpace(string(exitErr.Stderr))
		}
		if msg == "" {
			msg = err.Error()
		}
		return "", errors.New("copilot: " + msg)
	}
	return strings.TrimSpace(string(out)), nil
}
