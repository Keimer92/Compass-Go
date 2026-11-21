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

// RemoteStatus es el resultado de comprobar si el remoto existe de verdad.
//
// El estado lo usa el panel Git para avisar cuando el repositorio de GitHub
// fue borrado: en ese caso ofrece el asistente para vincular otro.
type RemoteStatus struct {
	// State: ok | empty | missing | auth | unreachable | none | error.
	State  string `json:"state"`
	Detail string `json:"detail"`
	URL    string `json:"url"`
}

// GhInfo dice si el CLI de GitHub esta disponible y con que cuenta.
type GhInfo struct {
	Available bool   `json:"available"`
	Account   string `json:"account"`
}

var (
	ghRepoNameRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
	ghURLRe      = regexp.MustCompile(`https://[^\s]+`)
)

// runGh ejecuta el CLI de GitHub sin abrir consola y con plazo.
func runGh(dir string, d time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", args...)
	windialog.NoWindow(cmd)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return strings.TrimSpace(string(out)), fmt.Errorf("gh %s: tiempo de espera agotado", args[0])
	}
	return strings.TrimSpace(string(out)), err
}

// GitRemoteStatus comprueba el remoto con `git ls-remote` (no modifica nada).
//
// Con remoteURL vacio se comprueba 'origin'. La clasificacion de errores cubre
// lo que devuelven GitHub y SSH: repositorio borrado (404), credenciales,
// host inalcanzable y repositorio vacio (existe pero sin HEAD).
func (b *Bridge) GitRemoteStatus(remoteURL string) RemoteStatus {
	dir := b.projectDir()
	st := RemoteStatus{State: "none"}
	if dir == "" {
		st.Detail = "no hay proyecto activo"
		return st
	}

	target := strings.TrimSpace(remoteURL)
	if target == "" {
		out, err := runGit(dir, "remote", "get-url", "origin")
		if err != nil || strings.TrimSpace(out) == "" {
			return st
		}
		target = strings.TrimSpace(out)
	} else if strings.HasPrefix(target, "-") || strings.ContainsAny(target, "\r\n") {
		st.State = "error"
		st.Detail = "URL no valida"
		return st
	}
	st.URL = target

	out, err := runGitTimeout(dir, 25*time.Second, "ls-remote", "--exit-code", target, "HEAD")
	if err == nil {
		st.State = "ok"
		return st
	}
	// Exit code 2 con --exit-code: el repo existe pero no tiene HEAD (vacio).
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 2 {
		st.State = "empty"
		return st
	}
	st.State, st.Detail = classifyRemoteFailure(out, err)
	return st
}

// classifyRemoteFailure traduce el error de `git ls-remote` a un estado.
func classifyRemoteFailure(out string, err error) (string, string) {
	low := strings.ToLower(out + " " + err.Error())
	switch {
	case strings.Contains(low, "authentication failed"),
		strings.Contains(low, "could not read username"),
		strings.Contains(low, "permission denied"),
		strings.Contains(low, "terminal prompts disabled"),
		strings.Contains(low, "access denied"),
		strings.Contains(low, "returned error: 403"):
		return "auth", "GitHub no permitio el acceso: revisa tus credenciales o permisos."
	case strings.Contains(low, "not found"),
		strings.Contains(low, "does not appear to be a git repository"),
		strings.Contains(low, "repository does not exist"),
		strings.Contains(low, "returned error: 404"):
		return "missing", "El repositorio remoto ya no existe (404)."
	case strings.Contains(low, "could not resolve host"),
		strings.Contains(low, "unable to access"),
		strings.Contains(low, "connection refused"),
		strings.Contains(low, "timed out"),
		strings.Contains(low, "tiempo de espera"):
		return "unreachable", "No se pudo contactar con el remoto (sin conexion o DNS)."
	}
	detail := strings.TrimSpace(out)
	if detail == "" {
		detail = err.Error()
	}
	return "error", detail
}

// GitGhInfo informa si `gh` esta instalado y con que cuenta ha iniciado sesion.
func (b *Bridge) GitGhInfo() GhInfo {
	info := GhInfo{}
	if _, err := exec.LookPath("gh"); err != nil {
		return info
	}
	info.Available = true
	dir := b.projectDir()
	if dir == "" {
		dir = "."
	}
	if out, err := runGh(dir, 10*time.Second, "api", "user", "--jq", ".login"); err == nil {
		info.Account = strings.TrimSpace(out)
	}
	return info
}

// GitCreateRemote crea un repositorio nuevo en la cuenta de GitHub del CLI `gh`
// y apunta 'origin' a el. No hace push: el frontend lo lanza despues para
// mostrar el progreso por separado.
func (b *Bridge) GitCreateRemote(name string, private bool) (string, error) {
	dir := b.projectDir()
	if dir == "" {
		return "", errors.New("no hay proyecto activo")
	}
	if out, err := runGit(dir, "rev-parse", "--is-inside-work-tree"); err != nil || strings.TrimSpace(out) != "true" {
		return "", errors.New("no es un repositorio git: inicializalo primero")
	}
	name = strings.TrimSpace(name)
	if !ghRepoNameRe.MatchString(name) {
		return "", errors.New("nombre no valido: usa letras, numeros, punto, guion o guion bajo")
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return "", errors.New("no se encontro el CLI de GitHub (gh)")
	}

	args := []string{"repo", "create", name}
	if private {
		args = append(args, "--private")
	} else {
		args = append(args, "--public")
	}
	out, err := runGh(dir, 120*time.Second, args...)
	if err != nil {
		detail := strings.TrimSpace(out)
		if detail == "" {
			detail = err.Error()
		}
		return "", fmt.Errorf("gh repo create: %s", detail)
	}

	created := ""
	if m := ghURLRe.FindString(out); m != "" {
		created = strings.TrimSuffix(m, ".git")
	}
	if created == "" {
		if acc := b.GitGhInfo().Account; acc != "" {
			created = "https://github.com/" + acc + "/" + name
		}
	}
	if created == "" {
		return "", errors.New("el repositorio se creo pero no se pudo leer su URL")
	}
	msg, err := b.GitSetRemote(created)
	if err != nil {
		return "", fmt.Errorf("repositorio creado (%s) pero no se pudo enlazar: %w", created, err)
	}
	return "Repositorio creado: " + created + "\n" + msg, nil
}
