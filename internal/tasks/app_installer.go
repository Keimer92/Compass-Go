package tasks

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"compass/internal/project"
	"compass/internal/shell"
)

// Plantilla del instalador NSIS generico para proyectos. Compass la embebe en el
// binario y sustituye los marcadores @...@ antes de invocar makensis.
//
//go:embed assets/app-installer.nsi
var appInstallerTemplate string

// nsisValue escapa un valor para incrustarlo en una cadena NSIS entre comillas.
func nsisValue(s string) string {
	s = strings.ReplaceAll(s, "$", "$$")
	s = strings.ReplaceAll(s, `"`, "''")
	return s
}

// buildAppInstaller genera "<Nombre> v<ver> [arch] Setup.exe" junto al ejecutable
// del proyecto, con la pagina final que ofrece crear acceso directo y ejecutar
// ahora.
func buildAppInstaller(appName, version, label, exePath, iconPath string) (string, error) {
	if appName == "" {
		appName = sanitizeAppName(strings.TrimSuffix(filepath.Base(exePath), filepath.Ext(exePath)))
	}
	if appName == "" {
		appName = "App"
	}
	regKey := sanitizeAppName(appName)
	if regKey == "" {
		regKey = "App"
	}

	makensis := locateMakensis()
	if makensis == "" {
		return "", fmt.Errorf("NSIS (`makensis`) no encontrado; instala NSIS para generar el instalador")
	}

	dir := filepath.Dir(exePath)
	outFile := filepath.Join(dir, artifactBase(appName, version, label)+" Setup.exe")

	script := strings.NewReplacer(
		"@APPNAME@", nsisValue(appName),
		"@APPEXE@", nsisValue(filepath.Base(exePath)),
		"@APPVER@", nsisValue(version),
		"@APPICON@", nsisValue(iconPath),
		"@APPSRC@", nsisValue(exePath),
		"@OUTFILE@", nsisValue(outFile),
		"@REGKEY@", nsisValue(regKey),
	).Replace(appInstallerTemplate)

	tmp, err := os.CreateTemp("", "compass-app-*.nsi")
	if err != nil {
		return "", err
	}
	scriptPath := tmp.Name()
	defer os.Remove(scriptPath)
	if _, err := tmp.WriteString(script); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}

	env := map[string]string{}
	if makensis != "makensis" {
		env = nsisEnv(filepath.Dir(makensis))
	}
	fmt.Printf("\n[INFO] Generando instalador: %s\n", filepath.Base(outFile))
	ok, err := shell.RunProgram(makensis, []string{"/V2", scriptPath}, dir, env)
	if err != nil {
		return "", err
	}
	if !ok || !fileExists(outFile) {
		return "", fmt.Errorf("makensis no pudo generar el instalador de %s", appName)
	}
	return outFile, nil
}

// findProjectIcon busca un .ico del propio proyecto para la marca del instalador.
func findProjectIcon(ctx *project.ProjectContext) string {
	for _, rel := range []string{
		"build/windows/icon.ico",
		"assets/icon/Icon.ico",
		"assets/icon.ico",
		"resources/icon.ico",
		"icon.ico",
	} {
		p := filepath.Join(ctx.ScriptDir, filepath.FromSlash(rel))
		if fileExists(p) {
			return p
		}
	}
	return ""
}

// withAppInstaller devuelve [exe] o [exe, instalador]. Si NSIS falta o falla,
// avisa y sigue con el portable: el release no se aborta por no poder empaquetar.
func withAppInstaller(ctx *project.ProjectContext, appName, version, label, exePath string) []string {
	installer, err := buildAppInstaller(appName, version, label, exePath, findProjectIcon(ctx))
	if err != nil {
		fmt.Printf("\n[WARN] No se genero el instalador: %v\n", err)
		return []string{exePath}
	}
	return []string{exePath, installer}
}
