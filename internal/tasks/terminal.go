package tasks

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"compass/internal/project"
	"compass/internal/shell"
	"compass/internal/ui"
)

func init() {
	Register("terminal", project.TypeTauri, TerminalConfigure)
	Register("terminal", project.TypeFlutter, TerminalConfigure)
	Register("terminal", project.TypeKotlin, TerminalConfigure)
	Register("terminal", project.TypePython, TerminalConfigure)
	Register("terminal", project.TypeRust, TerminalConfigure)
	Register("terminal", project.TypeGo, TerminalConfigure)
	Register("terminal", project.TypeGoWails, TerminalConfigure)
}

const profilePredictive = `# --- compass: autocompletado (PSReadLine >= 2.2.0) ---
try { Set-PSReadLineOption -PredictionSource History } catch { }

Set-PSReadLineKeyHandler -Key Tab -Function AcceptSuggestion
Set-PSReadLineKeyHandler -Key RightArrow -Function AcceptNextSuggestionWord

Set-PSReadLineOption -MaximumHistoryCount 32767 -HistorySaveStyle SaveIncrementally
Set-PSReadLineOption -BellStyle None -ShowToolTips
`

const profileLegacy = `# --- compass: autocompletado (PSReadLine anterior a 2.2.0) ---
Set-PSReadLineKeyHandler -Key Tab -Function MenuComplete

Set-PSReadLineKeyHandler -Key UpArrow -Function HistorySearchBackward
Set-PSReadLineKeyHandler -Key DownArrow -Function HistorySearchForward

Set-PSReadLineOption -HistorySearchCursorMovesToEnd

Set-PSReadLineOption -MaximumHistoryCount 32767 -HistorySaveStyle SaveIncrementally
Set-PSReadLineOption -BellStyle None -ShowToolTips
`

const profileSinModulo = `# --- compass: autocompletado (no configurado) ---
# No se encontró PSReadLine instalado para el shell configurado, así que no hay
# nada que configurar. Para instalarlo (Windows PowerShell 5.1):
#
#   Install-Module PSReadLine -Scope CurrentUser -Force
`

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// psreadlineVersion devuelve la versión más reciente de PSReadLine disponible.
func psreadlineVersion() string {
	script := "(Get-Module -ListAvailable PSReadLine | Sort-Object Version -Descending | Select-Object -First 1).Version.ToString()"
	out, err := shell.Run(shell.Shell(), "-NoProfile", "-Command", script)
	if err != nil {
		return ""
	}
	v := strings.TrimSpace(string(out))
	if v == "" {
		return ""
	}
	return v
}

// upgradePsreadline instala la última PSReadLine en 5.1 y devuelve la versión.
func upgradePsreadline() string {
	const timeoutSecs uint = 300
	script := "[Net.ServicePointManager]::SecurityProtocol = " +
		"[Net.SecurityProtocolType]::Tls12 -bor [Net.SecurityProtocolType]::Tls11; " +
		"Install-Module PSReadLine -Scope CurrentUser -Force -AllowClobber " +
		"-Confirm:$false -ErrorAction Stop; " +
		"(Get-Module -ListAvailable PSReadLine | Sort-Object Version -Descending | " +
		"Select-Object -First 1).Version.ToString()"
	fmt.Print("  Instalando (descarga desde PSGallery, la primera vez tarda)...\n\n")
	ok, _ := shell.RunTimeout(shell.Shell(), []string{"-NoProfile", "-Command", script}, timeoutSecs)
	if !ok {
		fmt.Println("[WARN] La instalación tardó demasiado y se ha cancelado.")
	}
	return psreadlineVersion()
}

// versionCode convierte "2.11.4" -> 211.
func versionCode(version string) (uint, bool) {
	parts := strings.Split(version, ".")
	if len(parts) == 0 {
		return 0, false
	}
	major, err := strconv.ParseUint(strings.TrimSpace(parts[0]), 10, 32)
	if err != nil {
		return 0, false
	}
	var minor uint64
	if len(parts) > 1 {
		minor, _ = strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 32)
	}
	return uint(major)*100 + uint(minor), true
}

func isPredictive(version string) bool {
	code, ok := versionCode(version)
	return ok && code >= 202
}

// profileFor elige el perfil según la versión instalada.
func profileFor(version string) string {
	code, ok := versionCode(version)
	if ok && code >= 202 {
		return profilePredictive
	}
	if version != "" {
		return profileLegacy
	}
	return profileSinModulo
}

// profileSubdir devuelve la subcarpeta de Documentos del shell.
func profileSubdir(sh string) string {
	if sh == "pwsh" {
		return "PowerShell"
	}
	return "WindowsPowerShell"
}

// profilePath devuelve la ruta del perfil del shell activo.
func profilePath() string {
	out, err := shell.Run(shell.Shell(), "-NoProfile", "-Command", "$PROFILE")
	if err == nil {
		p := strings.TrimSpace(string(out))
		if p != "" {
			return p
		}
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	return filepath.Join(home, "Documents", profileSubdir(shell.Shell()), "Microsoft.PowerShell_profile.ps1")
}

// TerminalConfigure escribe el perfil de PowerShell con el autocompletado.
// Equivalente de tasks/terminal.rs::configure.
func TerminalConfigure(ctx *project.ProjectContext) error {
	sh := shell.Shell()
	esPwsh := sh == "pwsh"

	version := psreadlineVersion()
	actualizada := false

	shell.ClearScreen()
	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Println("║     CONFIGURAR TERMINAL - AUTOCOMPLETADO         ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("Shell: %s\n", sh)
	fmt.Printf("  %s\n", shell.ShellPath())
	fmt.Println()

	switch {
	case version != "" && isPredictive(version):
		fmt.Printf("[OK] PSReadLine %s ya soporta autocompletado predictivo.\n", version)
	case esPwsh:
		fmt.Printf("[WARN] %s no expone una PSReadLine con predictivas.\n", sh)
		fmt.Println("       PSReadLine viene incluido en PowerShell 7; no se instala")
		fmt.Println("       otra copia para no sombrear la bundled. Actualiza PowerShell.")
	default:
		fmt.Println("[INFO] El completado predictivo necesita PSReadLine 2.2.0+.")
		actual := " no hay ninguna instalada."
		if version != "" {
			actual = fmt.Sprintf(" ahora está la %s que trae %s.", version, sh)
		}
		fmt.Printf("[INFO] Se va a actualizar a la última versión disponible%s\n\n", actual)
		version = upgradePsreadline()
		actualizada = true
	}

	profile := profileFor(version)

	if actualizada && !isPredictive(version) {
		if version != "" {
			fmt.Printf("[WARN] La actualización no dio una versión con predictivas (sigue %s).\n", version)
		} else {
			fmt.Println("[WARN] La actualización no surtió efecto. Revisa la conexión a PSGallery.")
		}
		fmt.Println("       Se configurará igualmente el autocompletado que esa versión permita.")
	}
	fmt.Println()

	code, _ := versionCode(version)
	if code >= 202 {
		fmt.Printf("PSReadLine detectado: %s\n", version)
		fmt.Println("Se activará el autocompletado predictivo:")
		fmt.Println("  1. Sugerencias del historial mientras escribes")
		fmt.Println("  2. TAB acepta la sugerencia completa")
		fmt.Println("  3. Flecha Derecha acepta palabra por palabra")
	} else if version != "" {
		fmt.Printf("PSReadLine detectado: %s\n\n", version)
		fmt.Println("[WARN] Esta versión no tiene sugerencias predictivas.")
		fmt.Println("       Son una añadido de PSReadLine 2.2.0, y esta máquina tiene la")
		fmt.Printf("       %s que trae %s de serie.\n\n", version, sh)
		fmt.Println("Se configurará el autocompletado disponible en ella:")
		fmt.Println("  1. TAB abre el menú de completados")
		fmt.Println("  2. Flechas arriba/abajo buscan en el historial")
		fmt.Println("  3. Historial grande, sin campana, con ToolTips")
		if !esPwsh {
			fmt.Println()
			fmt.Println("Para tener el completado predictivo, actualiza PSReadLine:")
			fmt.Println("  Install-Module PSReadLine -Scope CurrentUser -Force")
		}
	} else {
		fmt.Println("[WARN] No se encontró PSReadLine instalado.")
		fmt.Println("       Se escribirá un perfil que sólo explica cómo instalarlo.")
		if !esPwsh {
			fmt.Println()
			fmt.Println("  Install-Module PSReadLine -Scope CurrentUser -Force")
		}
	}
	fmt.Println()

	profileFile := profilePath()
	profileDir := filepath.Dir(profileFile)
	if !dirExists(profileDir) {
		_ = os.MkdirAll(profileDir, 0755)
	}

	if fileExists(profileFile) {
		data, _ := os.ReadFile(profileFile)
		previous := strings.TrimPrefix(string(data), string(utf8BOM))
		if strings.HasPrefix(string(data), string(utf8BOM)) && previous == profile {
			fmt.Println("[INFO] El perfil ya estaba actualizado. No se toca nada.")
			fmt.Println()
			ui.Pause()
			return nil
		}
		backup := filepath.Join(profileDir, "Microsoft.PowerShell_profile.ps1.compass-bak")
		_ = writeUtf8Bom(backup, previous)
		fmt.Println("[INFO] Copia del perfil anterior en:")
		fmt.Printf("  %s\n\n", backup)
	}

	writeUtf8Bom(profileFile, profile)

	fmt.Println("[LISTO] La configuración se ha guardado en:")
	fmt.Printf("  %s\n\n", profileFile)
	fmt.Println("Abre una nueva terminal para disfrutar del autocompletado.")
	fmt.Println()
	ui.Pause()
	return nil
}

// writeUtf8Bom escribe el contenido como UTF-8 con BOM (PowerShell 5.1 lee sin
// BOM como ANSI y descuadra los acentos).
func writeUtf8Bom(path, contents string) error {
	var b []byte
	b = append(b, utf8BOM...)
	b = append(b, []byte(contents)...)
	return os.WriteFile(path, b, 0644)
}
