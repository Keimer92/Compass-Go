package webui

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"compass/internal/windialog"
)

// La cuenta de GitHub la guarda el credential helper del sistema (Git
// Credential Manager). Estas acciones son las mismas que
// `git credential-manager github ...`, pero desde la UI. Compass no guarda
// tokens ni decide con que cuenta se autentica git.

// Los usuarios de GitHub solo llevan letras, digitos y guiones (hasta 39).
var ghAccountRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,38}$`)

// runGCM ejecuta `git credential-manager <args>` sin consola visible.
func runGCM(timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	full := append([]string{"credential-manager"}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	windialog.NoWindow(cmd)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))

	if ctx.Err() == context.DeadlineExceeded {
		return text, errors.New("tiempo de espera agotado esperando a Git Credential Manager")
	}
	if err != nil {
		low := strings.ToLower(text)
		if strings.Contains(low, "is not a git command") || strings.Contains(low, "no es un comando de git") {
			return text, errors.New("Git Credential Manager no está instalado")
		}
		if text == "" {
			return "", fmt.Errorf("git credential-manager falló: %w", err)
		}
		return text, errors.New(text)
	}
	return text, nil
}

// GitHubAccounts lista las cuentas que conoce Git Credential Manager.
func (b *Bridge) GitHubAccounts() ([]string, error) {
	out, err := runGCM(20*time.Second, "github", "list")
	if err != nil {
		return nil, err
	}
	accounts := []string{}
	for _, line := range strings.Split(out, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			accounts = append(accounts, s)
		}
	}
	return accounts, nil
}

// GitHubLogin abre el flujo de Git Credential Manager (navegador) para la
// cuenta indicada. Puede tardar: el usuario autoriza en el navegador, por eso
// el plazo es amplio.
func (b *Bridge) GitHubLogin(username string) (string, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return "", errors.New("escribe tu usuario de GitHub")
	}
	if !ghAccountRe.MatchString(username) {
		return "", errors.New("usuario de GitHub no válido (letras, números y guiones)")
	}
	if _, err := runGCM(180*time.Second, "github", "login", "--username", username); err != nil {
		return "", err
	}
	return fmt.Sprintf("Sesión iniciada como %s.", username), nil
}

// GitHubLogout olvida la cuenta en Git Credential Manager. No borra nada en
// GitHub ni en Compass.
func (b *Bridge) GitHubLogout(account string) (string, error) {
	account = strings.TrimSpace(account)
	if !ghAccountRe.MatchString(account) {
		return "", errors.New("cuenta de GitHub no válida")
	}
	if _, err := runGCM(30*time.Second, "github", "logout", account); err != nil {
		return "", err
	}
	return fmt.Sprintf("Cuenta %s eliminada del credential helper.", account), nil
}
