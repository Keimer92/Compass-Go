package whatsapp

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// HostOptions configura el sidecar `whatsapp-host`. Lo usan las apps que no
// pueden enlazar Go (Flutter): el host resuelve Node y el bundle igual que los
// proyectos Go, lanza el motor y reenvía stdio tal cual, sin interpretar el
// protocolo.
type HostOptions struct {
	// Bundle es la ruta explícita de index.js. Vacío = se resuelve.
	Bundle string
	// AuthDir es la carpeta de sesión; se pasa al motor como WHATSAPP_AUTH_DIR.
	AuthDir string
	// WorkDir es el cwd del motor. Vacío = cfg.DataDir.
	WorkDir string
}

// RunHost arranca el motor y reenvía stdin/stdout/stderr hasta que termina.
// Devuelve el código de salida del motor (1 si no se pudo arrancar).
func RunHost(o HostOptions) int {
	bundle := strings.TrimSpace(o.Bundle)
	if bundle == "" {
		var err error
		if bundle, err = resolveHostBundle(); err != nil {
			fmt.Fprintln(os.Stderr, "[ERROR] "+err.Error())
			return 1
		}
	}
	node, err := resolveNode()
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR] "+err.Error())
		return 1
	}
	if err := checkNodeVersion(node); err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR] "+err.Error())
		return 1
	}

	workDir := strings.TrimSpace(o.WorkDir)
	if workDir == "" {
		workDir = cfg.DataDir
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] no se pudo crear %s: %v\n", workDir, err)
		return 1
	}
	authDir := strings.TrimSpace(o.AuthDir)
	if authDir == "" {
		authDir = filepath.Join(workDir, "whatsapp_auth")
	}
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] no se pudo crear %s: %v\n", authDir, err)
		return 1
	}

	waLog(fmt.Sprintf("Host: arrancando motor (node=%s, bundle=%s, cwd=%s)", node, bundle, workDir))

	cmd := exec.Command(node, bundle)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"WHATSAPP_AUTH_DIR="+authDir,
		"NODE_OPTIONS=--no-deprecation",
	)
	hideConsoleWindow(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR] stdin del motor: "+err.Error())
		return 1
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR] stdout del motor: "+err.Error())
		return 1
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR] stderr del motor: "+err.Error())
		return 1
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR] no se pudo lanzar el motor: "+err.Error())
		return 1
	}
	// Si el host muere, el motor no debe quedar huerfano (job object en Windows).
	assignToJob(cmd)

	var wg sync.WaitGroup
	wg.Add(3)
	// La app escribe comandos en el stdin del host; se reenvían al motor. Al
	// cerrarse (la app terminó) se cierra el del motor para que salga solo.
	go func() {
		defer wg.Done()
		_, _ = io.Copy(stdin, os.Stdin)
		_ = stdin.Close()
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(os.Stdout, stdout)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(os.Stderr, stderr)
	}()

	err = cmd.Wait()
	wg.Wait()

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			waLog(fmt.Sprintf("Host: el motor termino con codigo %d", exitErr.ExitCode()))
			return exitErr.ExitCode()
		}
		waLog("Host: el motor termino con error: " + err.Error())
		return 1
	}
	waLog("Host: el motor termino correctamente")
	return 0
}

// resolveHostBundle busca index.js del motor. Mismo orden que los proyectos
// Go: kit canónico → junto al host (release) → cwd.
func resolveHostBundle() (string, error) {
	if v := strings.TrimSpace(os.Getenv("COMPASS_WHATSAPP_ENGINE")); v != "" {
		for _, rel := range []string{
			filepath.Join("single", "index.js"),
			filepath.Join("whatsapp-engine-bundle", "index.js"),
		} {
			if p := filepath.Join(v, rel); fileExists(p) {
				return p, nil
			}
		}
	}
	exe := exeDir()
	for _, rel := range []string{
		"index.js",
		filepath.Join("..", "single", "index.js"),
		filepath.Join("resources", "whatsapp-engine-bundle", "index.js"),
		filepath.Join("whatsapp_engine", "index.js"),
	} {
		if p := filepath.Join(exe, rel); fileExists(p) {
			return p, nil
		}
	}
	if wd, err := os.Getwd(); err == nil {
		for _, rel := range []string{
			filepath.Join("whatsapp_engine", "index.js"),
			filepath.Join("single", "index.js"),
			filepath.Join("resources", "whatsapp-engine-bundle", "index.js"),
		} {
			if p := filepath.Join(wd, rel); fileExists(p) {
				return p, nil
			}
		}
	}
	return "", errors.New("no se encontro el bundle del motor (index.js)")
}
