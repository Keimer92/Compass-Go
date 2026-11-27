package whatsapp

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// minNodeMajor es la version minima de Node que exige Baileys (>= 20.0.0).
// Con versiones anteriores el motor falla con errores cripticos; mejor
// detectarlo antes de arrancar.
const minNodeMajor = 20

var nodeVersionRe = regexp.MustCompile(`^v(\d+)\.`)

// resolveNode localiza el ejecutable de Node en cascada:
//  1. un sidecar node.exe dentro del propio proyecto (staging/release anterior);
//  2. el Node instalado en la maquina (PATH).
//
// No usa el node.exe del engine canónico (COMPASS_WHATSAPP_ENGINE): el runtime
// depende del Node del sistema aunque el corekit de Compass conserve el binario.
// Devuelve una ruta utilizable por exec.Command o un error con la accion a
// seguir. No valida la version: eso lo hace checkNodeVersion, para poder
// distinguir "no hay Node" de "hay una version vieja".
func resolveNode() (string, error) {
	if p := findFirstProject(
		"node.exe",
		"resources/node.exe",
		"bin/node.exe",
		"node-x86_64-pc-windows-msvc.exe",
		"resources/node-x86_64-pc-windows-msvc.exe",
		"bin/node-x86_64-pc-windows-msvc.exe",
	); p != "" {
		return p, nil
	}
	if p, err := exec.LookPath("node"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf(
		"no se encontro Node.js: instala Node %d+ desde https://nodejs.org o vuelve a etapar el motor con su node.exe",
		minNodeMajor)
}

// checkNodeVersion rechaza un Node anterior a minNodeMajor.
func checkNodeVersion(node string) error {
	cmd := exec.Command(node, "--version")
	// Sin esto, node.exe hereda/crea una consola al comprobarlo desde la GUI:
	// se ve un parpadeo de cmd y la ventana de GymPro se minimiza.
	hideConsoleWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("no se pudo ejecutar %q --version: %w", node, err)
	}
	version := strings.TrimSpace(string(out))
	m := nodeVersionRe.FindStringSubmatch(version)
	if m == nil {
		return fmt.Errorf("version de Node no reconocida: %q", version)
	}
	major, err := strconv.Atoi(m[1])
	if err != nil {
		return fmt.Errorf("version de Node no reconocida: %q", version)
	}
	if major < minNodeMajor {
		return fmt.Errorf("Node %d+ requerido (encontrado %s)", minNodeMajor, version)
	}
	return nil
}
