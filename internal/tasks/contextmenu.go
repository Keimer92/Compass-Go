package tasks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"compass/internal/shell"
	"compass/internal/ui"
)

const (
	ctxCompassKey   = "zz_CompassGo"
	ctxGitignoreKey = "zz_CompassGitignore"
	ctxGitignoreOld = "zz_VibeGitignore"
	ctxGitignoreLbl = "Compass Go: Add .gitignore"
	ctxVibeKey      = "zz_VibeCoding"
	ctxVibePath     = `Directory\ContextMenus\VibeCoding`
	ctxMenuStamp    = "CompassStamp"
)

func init() {
	// registrado como tareas globales en RunGlobalTask (no dependen del proyecto)
}

func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	if d := filepath.Dir(exe); d != "" {
		return d
	}
	return "."
}

func resolveIcon(name string) string {
	dir := exeDir()
	custom := filepath.Join(dir, name)
	fallback := filepath.Join(dir, "Default.ico")
	if fileExists(custom) {
		return custom
	}
	if fileExists(fallback) {
		return fallback
	}
	if home := homeDir(); home != "" {
		return filepath.Join(home, ".cargo", "bin", "Default.ico")
	}
	return "Default.ico"
}

func iconPath() string     { return resolveIcon("icon.ico") }
func opencodeIcon() string { return resolveIcon("opencode.ico") }

func exePath() string {
	name := "compass-go.exe"
	if d := exeDir(); d != "" {
		cand := filepath.Join(d, name)
		return cand
	}
	return name
}

// ── Compass (entrada principal) ──────────────────────────────────────────────

func contextMenuInstall() error {
	exe := exePath()
	shellName := shell.ShellPath()
	folderCommand := compassContextCommand(shellName, exe, "%1")
	backgroundCommand := compassContextCommand(shellName, exe, "%W")

	if commandIsCurrent(ctxCompassKey, []string{shellName, exe}) && labelIs(ctxCompassKey, "Compass Go") {
		fmt.Println("[SKIP] Menu contextual Compass Go ya esta instalado.")
		fmt.Printf("  Ruta: %s\n", exe)
		if err := installGitignoreActionWindows(); err != nil {
			return err
		}
		removeOldCodeRunnerEntry()
		return nil
	}

	removeOldCodeRunnerEntry()

	icon := iconPath()
	fmt.Printf("[INFO] Icon path: %s\n", icon)
	fmt.Printf("[INFO] Icon exists: %v\n", fileExists(icon))

	ps := fmt.Sprintf(`
New-Item -Path 'HKCU:\Software\Classes\Directory\shell\%s' -Force | Out-Null; `+
		`Set-ItemProperty -Path 'HKCU:\Software\Classes\Directory\shell\%s' -Name '(default)' -Value 'Compass Go'; `+
		`Set-ItemProperty -Path 'HKCU:\Software\Classes\Directory\shell\%s' -Name 'Icon' -Type String -Value %s -Force; `+
		`New-Item -Path 'HKCU:\Software\Classes\Directory\shell\%s\command' -Force | Out-Null; `+
		`Set-ItemProperty -Path 'HKCU:\Software\Classes\Directory\shell\%s\command' -Name '(default)' -Value %s -Force; `+
		`New-Item -Path 'HKCU:\Software\Classes\Directory\Background\shell\%s' -Force | Out-Null; `+
		`Set-ItemProperty -Path 'HKCU:\Software\Classes\Directory\Background\shell\%s' -Name '(default)' -Value 'Compass Go'; `+
		`Set-ItemProperty -Path 'HKCU:\Software\Classes\Directory\Background\shell\%s' -Name 'Icon' -Type String -Value %s -Force; `+
		`New-Item -Path 'HKCU:\Software\Classes\Directory\Background\shell\%s\command' -Force | Out-Null; `+
		`Set-ItemProperty -Path 'HKCU:\Software\Classes\Directory\Background\shell\%s\command' -Name '(default)' -Value %s -Force`,
		ctxCompassKey, ctxCompassKey, ctxCompassKey, shell.PsQuote(icon),
		ctxCompassKey, ctxCompassKey, shell.PsQuote(folderCommand),
		ctxCompassKey, ctxCompassKey, ctxCompassKey, shell.PsQuote(icon),
		ctxCompassKey, ctxCompassKey, shell.PsQuote(backgroundCommand))

	if out, err := shell.RunPS(ps); err != nil {
		return fmt.Errorf("error al instalar menu contextual:\n%s", string(out))
	}

	if err := installGitignoreActionWindows(); err != nil {
		return err
	}

	fmt.Println("[OK] Menu contextual Compass Go instalado.")
	fmt.Printf("  Ruta: %s\n", exe)
	fmt.Printf("  Icono: %s\n", icon)
	fmt.Println("  Haz clic derecho en una carpeta -> 'Compass Go'")
	fmt.Println()
	fmt.Println("[INFO] Si el icono no aparece, intenta:")
	fmt.Println("  1. Cerrar sesion y volver a iniciar")
	fmt.Println("  2. O ejecutar en PowerShell: ie4uinit.exe -show")
	return nil
}

func removeOldCodeRunnerEntry() {
	ps := `Remove-Item -Path 'HKCU:\Software\Classes\Directory\shell\zz_CodeRunner' -Recurse -Force -ErrorAction SilentlyContinue; ` +
		`Remove-Item -Path 'HKCU:\Software\Classes\Directory\Background\shell\zz_CodeRunner' -Recurse -Force -ErrorAction SilentlyContinue`
	_, _ = shell.RunPS(ps)
}

func installGitignoreActionWindows() error {
	exe := exePath()
	icon := iconPath()
	shellName := shell.ShellPath()
	folderCommand := gitignoreContextCommand(shellName, exe, "%1")
	backgroundCommand := gitignoreContextCommand(shellName, exe, "%V")
	ps := fmt.Sprintf(`
Remove-Item -Path 'HKCU:\Software\Classes\Directory\shell\%s' -Recurse -Force -ErrorAction SilentlyContinue; `+
		`Remove-Item -Path 'HKCU:\Software\Classes\Directory\Background\shell\%s' -Recurse -Force -ErrorAction SilentlyContinue; `+
		`New-Item -Path 'HKCU:\Software\Classes\Directory\shell\%s' -Force | Out-Null; `+
		`Set-ItemProperty -Path 'HKCU:\Software\Classes\Directory\shell\%s' -Name '(default)' -Value %s -Force; `+
		`Set-ItemProperty -Path 'HKCU:\Software\Classes\Directory\shell\%s' -Name 'Icon' -Type String -Value %s -Force; `+
		`New-Item -Path 'HKCU:\Software\Classes\Directory\shell\%s\command' -Force | Out-Null; `+
		`Set-ItemProperty -Path 'HKCU:\Software\Classes\Directory\shell\%s\command' -Name '(default)' -Value %s -Force; `+
		`New-Item -Path 'HKCU:\Software\Classes\Directory\Background\shell\%s' -Force | Out-Null; `+
		`Set-ItemProperty -Path 'HKCU:\Software\Classes\Directory\Background\shell\%s' -Name '(default)' -Value %s -Force; `+
		`Set-ItemProperty -Path 'HKCU:\Software\Classes\Directory\Background\shell\%s' -Name 'Icon' -Type String -Value %s -Force; `+
		`New-Item -Path 'HKCU:\Software\Classes\Directory\Background\shell\%s\command' -Force | Out-Null; `+
		`Set-ItemProperty -Path 'HKCU:\Software\Classes\Directory\Background\shell\%s\command' -Name '(default)' -Value %s -Force`,
		ctxGitignoreOld, ctxGitignoreOld,
		ctxGitignoreKey, ctxGitignoreKey, shell.PsQuote(ctxGitignoreLbl), ctxGitignoreKey, shell.PsQuote(icon),
		ctxGitignoreKey, ctxGitignoreKey, shell.PsQuote(folderCommand),
		ctxGitignoreKey, ctxGitignoreKey, shell.PsQuote(ctxGitignoreLbl), ctxGitignoreKey, shell.PsQuote(icon),
		ctxGitignoreKey, ctxGitignoreKey, shell.PsQuote(backgroundCommand))
	if out, err := shell.RunPS(ps); err != nil {
		return fmt.Errorf("error al instalar la accion contextual .gitignore:\n%s", string(out))
	}
	return nil
}

func compassContextCommand(shellName, exe, placeholder string) string {
	return fmt.Sprintf(`"%s" -NoLogo -NoProfile -Command "& %s -p '%s'"`,
		shellName, shell.PsQuote(exe), placeholder)
}

func gitignoreContextCommand(shellName, exe, placeholder string) string {
	// El binario es de consola, asi que lanzarlo directamente desde Explorer
	// hace parpadear una ventana cmd. Lo ejecutamos dentro de un pwsh oculto:
	// el hijo hereda esa consola oculta, asi que no aparece ninguna ventana.
	//
	// Ojo: NO usar Start-Process -ArgumentList para pasar la ruta. Start-Process
	// une los argumentos sin comillas, y una ruta con espacios (o con barra
	// final) llega partida al binario, que falla. La llamada directa `& $exe`
	// pasa la ruta como un unico argumento y funciona en todos los casos.
	script := fmt.Sprintf(`
$exe = %s
$path = %s
& $exe --task context-gitignore --path $path
$code = $LASTEXITCODE
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
$n = New-Object System.Windows.Forms.NotifyIcon
$n.Icon = [System.Drawing.SystemIcons]::Information
$n.Visible = $true
if ($code -eq 0) {
  $n.ShowBalloonTip(4000, 'Compass Go', '.gitignore creado o actualizado', 'Info')
} else {
  $n.ShowBalloonTip(5000, 'Compass Go', 'No se pudo crear el .gitignore', 'Warning')
}
Start-Sleep -Seconds 4
$n.Dispose()`, shell.PsQuote(exe), shell.PsQuote(placeholder))
	return fmt.Sprintf(`"%s" -NoLogo -NoProfile -WindowStyle Hidden -Command "%s"`, shellName, script)
}

func contextMenuUninstall() error {
	removeOldCodeRunnerEntry()
	ps := fmt.Sprintf(`
Remove-Item -Path 'HKCU:\Software\Classes\Directory\shell\%s' -Recurse -Force -ErrorAction SilentlyContinue; `+
		`Remove-Item -Path 'HKCU:\Software\Classes\Directory\Background\shell\%s' -Recurse -Force -ErrorAction SilentlyContinue; `+
		`Remove-Item -Path 'HKCU:\Software\Classes\Directory\shell\%s' -Recurse -Force -ErrorAction SilentlyContinue; `+
		`Remove-Item -Path 'HKCU:\Software\Classes\Directory\Background\shell\%s' -Recurse -Force -ErrorAction SilentlyContinue; `+
		`Remove-Item -Path 'HKCU:\Software\Classes\Directory\shell\%s' -Recurse -Force -ErrorAction SilentlyContinue; `+
		`Remove-Item -Path 'HKCU:\Software\Classes\Directory\Background\shell\%s' -Recurse -Force -ErrorAction SilentlyContinue; `+
		`Remove-Item -Path 'HKCU:\Software\Classes\Directory\shell\zz_Compass' -Recurse -Force -ErrorAction SilentlyContinue; `+
		`Remove-Item -Path 'HKCU:\Software\Classes\Directory\Background\shell\zz_Compass' -Recurse -Force -ErrorAction SilentlyContinue`,
		ctxCompassKey, ctxCompassKey, ctxGitignoreKey, ctxGitignoreKey, ctxGitignoreOld, ctxGitignoreOld)
	_, _ = shell.RunPS(ps)
	fmt.Println("[OK] Menu contextual Compass Go eliminado.")
	return nil
}

// ── Vibe Coding (menu cascada) ──────────────────────────────────────────────

type vibeTool struct {
	key      string
	title    string
	cli      string
	install  string
	fallback []string
	via      installVia
}

type installVia int

const (
	viaCmd installVia = iota
	viaPowerShell
)

var vibeTools = []vibeTool{
	{"01OpenCode", "OpenCode", "opencode --auto", "npm install -g @opencode/cli", nil, viaCmd},
	{"02CodeBuddy", "CodeBuddy", "codebuddy --dangerously-skip-permissions", "npm install -g @tencent-ai/codebuddy-code", nil, viaCmd},
	{"03KiloCode", "Kilo Code", "kilo --auto", "npm install -g @kilocode/cli", nil, viaCmd},
	{"04Cline", "Cline", "cline --auto-approve true", "npm install -g cline", nil, viaCmd},
	{"05Antigravity", "Antigravity", "agy --dangerously-skip-permissions", "irm https://antigravity.google/cli/install.ps1 | iex", []string{"LOCALAPPDATA/agy/bin/agy.exe"}, viaPowerShell},
	{"06Copilot", "Copilot CLI", "copilot --yolo", "winget install --id GitHub.Copilot --exact --accept-source-agreements --accept-package-agreements", []string{"LOCALAPPDATA/Microsoft/WinGet/Links/copilot.exe"}, viaCmd},
	{"07KiroCli", "Kiro CLI", "kiro-cli chat --trust-all-tools", "", []string{"LOCALAPPDATA/Kiro-Cli/kiro-cli.exe"}, viaCmd},
	{"08Freebuff", "Freebuff", "freebuff", "npm install -g freebuff", nil, viaCmd},
	{"09CommandCode", "Command Code", "cmdc --yolo", "npm install -g command-code", nil, viaCmd},
	{"10QoderCli", "Qoder CLI", "qodercli --yolo", "irm https://qoder.com/install.ps1 | iex", []string{"HOME/.qoder/bin/qodercli/qodercli.exe", "HOME/.qoder/bin/qodercli.exe"}, viaPowerShell},
	{"11Hermes", "Hermes", "hermes", "iex (irm https://hermes-agent.nousresearch.com/install.ps1)", []string{"LOCALAPPDATA/hermes/bin/hermes.exe", "LOCALAPPDATA/hermes/bin/hermes.cmd"}, viaPowerShell},
}

func resolveToolCommand(t vibeTool) (string, string) {
	bin := t.cli
	args := ""
	if b, a, ok := strings.Cut(t.cli, " "); ok {
		bin = b
		args = a
	}
	if path := commandPath(bin); path != "" {
		path = preferCmdShim(path)
		if args != "" {
			return fmt.Sprintf("%s %s", shell.PsQuote(path), args), path
		}
		return shell.PsQuote(path), path
	}
	for _, cand := range fallbackCandidates(t) {
		if fileExists(cand) {
			p := cand
			var cmdLine string
			if args != "" {
				cmdLine = fmt.Sprintf("%s %s", shell.PsQuote(p), args)
			} else {
				cmdLine = shell.PsQuote(p)
			}
			return cmdLine, p
		}
	}
	return "", ""
}

func fallbackCandidates(t vibeTool) []string {
	var out []string
	for _, hint := range t.fallback {
		switch {
		case strings.HasPrefix(hint, "LOCALAPPDATA/"):
			if local := os.Getenv("LOCALAPPDATA"); local != "" {
				out = append(out, joinRelative(local, strings.TrimPrefix(hint, "LOCALAPPDATA/")))
			}
		case strings.HasPrefix(hint, "HOME/"):
			if home := homeDir(); home != "" {
				out = append(out, joinRelative(home, strings.TrimPrefix(hint, "HOME/")))
			}
		default:
			out = append(out, hint)
		}
	}
	if strings.HasPrefix(t.install, "npm ") {
		appData := os.Getenv("APPDATA")
		if appData == "" {
			if home := homeDir(); home != "" {
				appData = filepath.Join(home, "AppData", "Roaming")
			}
		}
		if appData != "" {
			bin, _, _ := strings.Cut(t.cli, " ")
			base := filepath.Join(appData, "npm", bin)
			out = append(out, base+".cmd", base+".exe", base+".ps1", base)
		}
	}
	return out
}

func preferCmdShim(path string) string {
	if strings.EqualFold(filepath.Ext(path), ".ps1") {
		cmdShim := strings.TrimSuffix(path, filepath.Ext(path)) + ".cmd"
		if fileExists(cmdShim) {
			return cmdShim
		}
	}
	return path
}

func joinRelative(base, relative string) string {
	parts := strings.Split(relative, "/")
	result := base
	for _, p := range parts {
		if p == "" {
			continue
		}
		result = filepath.Join(result, p)
	}
	return result
}

func menuStamp() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	info, err := os.Stat(exe)
	if err != nil {
		return ""
	}
	mod := info.ModTime()
	secs := mod.Unix()
	if secs < 0 {
		return ""
	}
	return fmt.Sprintf("%d", secs)
}

func vibeContextCommand(shellName, title, cmdline string) string {
	return fmt.Sprintf(`"%s" -NoLogo -NoProfile -Command "Set-Location -LiteralPath '%%V'; [Console]::Title = %s; & %s"`,
		shellName, shell.PsQuote(title), cmdline)
}

func installVibeCoding() error {
	icon := opencodeIcon()
	removeVibeOldKeys()
	removeVibeCtxTree()

	type resolved struct {
		tool vibeTool
		cmd  string
		note string
	}
	var resolvedTools []resolved
	for _, t := range vibeTools {
		if cmd, note := resolveToolCommand(t); cmd != "" {
			resolvedTools = append(resolvedTools, resolved{tool: t, cmd: cmd, note: note})
		}
	}

	if len(resolvedTools) == 0 {
		fmt.Println("[WARN] No se encontro ninguna CLI de Vibe Coding (ni en el PATH ni en sus rutas de instalacion).")
		fmt.Println("[WARN] Se instalara el menu Vibe Coding sin sub-entradas.")
		fmt.Println("[WARN] Instala las CLIs y vuelve a ejecutar el instalador.")
	} else {
		fmt.Printf("[INFO] Detectadas %d herramienta(s) de Vibe Coding en el PATH.\n", len(resolvedTools))
	}
	fmt.Println("[INFO] Detectando herramientas Vibe Coding...")
	for _, t := range vibeTools {
		found := false
		var note string
		for _, r := range resolvedTools {
			if r.tool.key == t.key {
				found = true
				note = r.note
			}
		}
		switch {
		case found && note != "":
			fmt.Printf("  [OK] %-14s (PATH no lo expone, usando: %s)\n", t.title, note)
		case found:
			fmt.Printf("  [OK] %s\n", t.title)
		default:
			fmt.Printf("  [X] %s no encontrado\n", t.title)
		}
	}

	var ps strings.Builder
	iconQuoted := shell.PsQuote(icon)
	ps.WriteString(fmt.Sprintf(`
New-Item -Path 'HKCU:\Software\Classes\Directory\shell\%s' -Force | Out-Null; `+
		`Set-ItemProperty -Path 'HKCU:\Software\Classes\Directory\shell\%s' -Name 'MUIVerb' -Value 'Vibe Coding' -Force; `+
		`Set-ItemProperty -Path 'HKCU:\Software\Classes\Directory\shell\%s' -Name 'Icon' -Type String -Value %s -Force; `+
		`Set-ItemProperty -Path 'HKCU:\Software\Classes\Directory\shell\%s' -Name 'ExtendedSubCommandsKey' -Value '%s' -Force; `+
		`New-Item -Path 'HKCU:\Software\Classes\Directory\Background\shell\%s' -Force | Out-Null; `+
		`Set-ItemProperty -Path 'HKCU:\Software\Classes\Directory\Background\shell\%s' -Name 'MUIVerb' -Value 'Vibe Coding' -Force; `+
		`Set-ItemProperty -Path 'HKCU:\Software\Classes\Directory\Background\shell\%s' -Name 'Icon' -Type String -Value %s -Force; `+
		`Set-ItemProperty -Path 'HKCU:\Software\Classes\Directory\Background\shell\%s' -Name 'ExtendedSubCommandsKey' -Value '%s' -Force; `,
		ctxVibeKey, ctxVibeKey, ctxVibeKey, iconQuoted, ctxVibeKey, ctxVibePath,
		ctxVibeKey, ctxVibeKey, ctxVibeKey, iconQuoted, ctxVibeKey, ctxVibePath))

	shellName := shell.ShellPath()
	for _, r := range resolvedTools {
		command := vibeContextCommand(shellName, r.tool.title, r.cmd)
		ps.WriteString(fmt.Sprintf(`
New-Item -Path 'HKCU:\Software\Classes\%s\shell\%s' -Force | Out-Null; `+
			`Set-ItemProperty -Path 'HKCU:\Software\Classes\%s\shell\%s' -Name 'MUIVerb' -Value %s -Force; `+
			`Set-ItemProperty -Path 'HKCU:\Software\Classes\%s\shell\%s' -Name 'Icon' -Type String -Value %s -Force; `+
			`New-Item -Path 'HKCU:\Software\Classes\%s\shell\%s\command' -Force | Out-Null; `+
			`Set-ItemProperty -Path 'HKCU:\Software\Classes\%s\shell\%s\command' -Name '(default)' -Value %s -Force; `,
			ctxVibePath, r.tool.key,
			ctxVibePath, r.tool.key, shell.PsQuote(r.tool.title),
			ctxVibePath, r.tool.key, iconQuoted,
			ctxVibePath, r.tool.key,
			ctxVibePath, r.tool.key, shell.PsQuote(command)))
	}

	if stamp := menuStamp(); stamp != "" {
		ps.WriteString(fmt.Sprintf(`
Set-ItemProperty -Path 'HKCU:\Software\Classes\%s' -Name '%s' -Value '%s' -Force; `,
			ctxVibePath, ctxMenuStamp, stamp))
	}

	if out, err := shell.RunPS(ps.String()); err != nil {
		return fmt.Errorf("error al instalar menu Vibe Coding:\n%s", string(out))
	}

	fmt.Println()
	fmt.Println("[OK] Menu contextual 'Vibe Coding' instalado.")
	fmt.Printf("  Icono: %s\n", icon)
	if len(resolvedTools) == 0 {
		fmt.Println("  [WARN] Sin sub-entradas: ninguna CLI encontrada.")
	}
	for _, r := range resolvedTools {
		if r.note != "" {
			fmt.Printf("  [OK] %-14s -> %s\n", r.tool.title, r.note)
		}
	}
	fmt.Println("  Clic derecho en una carpeta -> 'Vibe Coding'")
	fmt.Println()
	fmt.Println("[INFO] Si el menu no aparece, reinicia el Explorador de Windows:")
	fmt.Println("  taskkill /f /im explorer.exe & start explorer.exe")
	return nil
}

func uninstallVibeCoding() error {
	ps := fmt.Sprintf(`
Remove-Item -Path 'HKCU:\Software\Classes\Directory\shell\%s' -Recurse -Force -ErrorAction SilentlyContinue; `+
		`Remove-Item -Path 'HKCU:\Software\Classes\Directory\Background\shell\%s' -Recurse -Force -ErrorAction SilentlyContinue; `+
		`Remove-Item -Path 'HKCU:\Software\Classes\%s' -Recurse -Force -ErrorAction SilentlyContinue`,
		ctxVibeKey, ctxVibeKey, ctxVibePath)
	_, _ = shell.RunPS(ps)
	fmt.Println("[OK] Menu contextual 'Vibe Coding' eliminado.")
	return nil
}

func installOpencode() error {
	if err := ConfigureOpencode(true); err != nil {
		return err
	}
	return installVibeCoding()
}

func uninstallOpencode() error { return uninstallVibeCoding() }

func installCline() error         { return installVibeCoding() }
func uninstallCline() error       { return uninstallVibeCoding() }
func installKilo() error          { return installVibeCoding() }
func uninstallKilo() error        { return uninstallVibeCoding() }
func installAntigravity() error   { return installVibeCoding() }
func uninstallAntigravity() error { return uninstallVibeCoding() }

// ── Deteccion e instalacion de herramientas ─────────────────────────────────

func ensureToolsInstalled() error {
	var faltantes []vibeTool
	for _, t := range vibeTools {
		if cmd, _ := resolveToolCommand(t); cmd == "" {
			faltantes = append(faltantes, t)
		}
	}
	var instalables []vibeTool
	for _, t := range faltantes {
		if t.install != "" {
			instalables = append(instalables, t)
		}
	}

	if len(faltantes) == 0 {
		fmt.Printf("[OK] Las %d herramientas de Vibe Coding estan instaladas.\n", len(vibeTools))
		return nil
	}

	fmt.Println()
	fmt.Printf("[INFO] Herramientas de Vibe Coding no encontradas: %d\n", len(faltantes))
	for _, t := range faltantes {
		detalle := t.install
		if detalle == "" {
			detalle = "instalador grafico, no se instala desde la terminal"
		}
		fmt.Printf("       - %s  (%s)\n", t.title, detalle)
	}
	if len(instalables) > 0 {
		fmt.Println()
		fmt.Printf("[INFO] Se preguntara por %d de ellas.\n", len(instalables))
	}
	fmt.Println()

	for _, t := range faltantes {
		fmt.Printf("[?] %s no esta instalado.\n", t.title)
		if t.install == "" {
			fmt.Println("    No se instala desde la terminal: usa su instalador.")
			continue
		}
		fmt.Printf("    Comando oficial: %s\n", t.install)
		fmt.Println("    (npm global, instalador nativo o winget, segun la herramienta)")
		respuesta := ui.AskText("    ¿Instalarlo ahora?", "n")
		if !esSi(respuesta) {
			fmt.Printf("    [SKIP] No se instala %s.\n", t.title)
			continue
		}
		fmt.Printf("    Instalando %s...\n", t.title)
		if err := instalar(t); err != nil {
			fmt.Printf("    [ERROR] No se pudo instalar %s: %v\n", t.title, err)
			continue
		}
		if cmd, _ := resolveToolCommand(t); cmd != "" {
			fmt.Printf("    [OK] %s instalado.\n", t.title)
		} else {
			fmt.Printf("    [WARN] %s se instalo pero su ejecutable no aparece.\n", t.title)
			fmt.Println("         Si es Antigravity, abre una terminal nueva: el PATH se")
			fmt.Println("         actualiza al iniciar sesion.")
		}
	}
	return nil
}

func instalar(t vibeTool) error {
	switch t.via {
	case viaCmd:
		script := strings.Replace(t.install, "npm ", "npm.cmd ", 1)
		return shell.RunPSInDirStatus(script, ".")
	default:
		return shell.RunPSInDirStatus(t.install, ".")
	}
}

func esSi(respuesta string) bool {
	switch strings.ToLower(strings.TrimSpace(respuesta)) {
	case "s", "si", "sí", "y", "yes":
		return true
	default:
		return false
	}
}

// ── Toggle interactivo ──────────────────────────────────────────────────────

type menuChoice int

const (
	mcToggleCompass menuChoice = iota
	mcToggleVibe
	mcInstallAll
	mcUninstallAll
	mcBack
)

func contextMenuToggle() error {
	for {
		shell.ClearScreen()
		title := "MENU CONTEXTUAL DE WINDOWS"
		fmt.Println("╔══════════════════════════════════════════════════╗")
		fmt.Printf("║     %s\n", title)
		fmt.Println("╚══════════════════════════════════════════════════╝")
		fmt.Println()

		crInstalled := isInstalled(ctxCompassKey)
		vcInstalled := isInstalled(ctxVibeKey)

		options := []struct {
			label string
			act   menuChoice
		}{}
		crLabel := "  Compass"
		if crInstalled {
			crLabel = "✔ Compass"
		}
		vcLabel := "  Vibe Coding"
		if vcInstalled {
			vcLabel = "✔ Vibe Coding"
		}
		options = append(options, struct {
			label string
			act   menuChoice
		}{fmt.Sprintf("1. %s  [%s]", crLabel, toggleWord(crInstalled)), mcToggleCompass})
		options = append(options, struct {
			label string
			act   menuChoice
		}{fmt.Sprintf("2. %s  [%s]", vcLabel, toggleWord(vcInstalled)), mcToggleVibe})
		options = append(options, struct {
			label string
			act   menuChoice
		}{"3. Instalar todos", mcInstallAll})
		options = append(options, struct {
			label string
			act   menuChoice
		}{"4. Desinstalar todos", mcUninstallAll})
		back := len(options) + 1
		options = append(options, struct {
			label string
			act   menuChoice
		}{fmt.Sprintf("%d. Volver", back), mcBack})

		opts := make([]string, len(options))
		for i, o := range options {
			opts[i] = o.label
		}
		idx, err := selectIdx("SELECCIONA UNA OPCION", opts)
		if err != nil {
			continue
		}
		act := options[idx].act
		switch act {
		case mcToggleCompass:
			if crInstalled {
				_ = contextMenuUninstall()
			} else {
				_ = contextMenuInstall()
			}
			ui.Pause()
		case mcToggleVibe:
			if vcInstalled {
				_ = uninstallVibeCoding()
			} else {
				_ = installVibeCoding()
			}
			ui.Pause()
		case mcInstallAll:
			_ = contextMenuInstall()
			_ = installVibeCoding()
			ui.Pause()
		case mcUninstallAll:
			_ = contextMenuUninstall()
			_ = uninstallVibeCoding()
			ui.Pause()
		case mcBack:
			return nil
		}
	}
}

func toggleWord(installed bool) string {
	if installed {
		return "Desinstalar"
	}
	return "Instalar"
}

// ── Helpers de registro ─────────────────────────────────────────────────────

func isInstalled(name string) bool {
	var ps string
	if name == ctxVibeKey {
		ps = fmt.Sprintf("Test-Path 'HKCU:\\Software\\Classes\\Directory\\shell\\%s'", name)
	} else {
		ps = fmt.Sprintf("Test-Path 'HKCU:\\Software\\Classes\\Directory\\shell\\%s\\command'", name)
	}
	out, err := shell.RunPS(ps)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "True"
}

func commandIsCurrent(name string, needles []string) bool {
	var checks []string
	for _, n := range needles {
		checks = append(checks, fmt.Sprintf("$v.Contains(%s)", shell.PsQuote(n)))
	}
	ps := fmt.Sprintf(`
$v = (Get-ItemProperty -Path 'HKCU:\Software\Classes\Directory\shell\%s\command' -ErrorAction SilentlyContinue).'(default)'; `+
		`if ($null -eq $v) { 'false' } else { %s }`, name, strings.Join(checks, " -and "))
	out, err := shell.RunPS(ps)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "True"
}

func labelIs(name, expected string) bool {
	ps := fmt.Sprintf(`
$v = (Get-ItemProperty -Path 'HKCU:\Software\Classes\Directory\shell\%s' -ErrorAction SilentlyContinue); `+
		`if ($null -eq $v) { 'false' } else { if ($v.MUIVerb) { $v.MUIVerb } else { $v.'(default)' } }`, name)
	out, err := shell.RunPS(ps)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == expected
}

func removeVibeOldKeys() {
	old := []string{"zz_OpenCode", "zz_pCline", "zz_pKilo"}
	var ps strings.Builder
	for _, k := range old {
		ps.WriteString(fmt.Sprintf(`
Remove-Item -Path 'HKCU:\Software\Classes\Directory\shell\%s' -Recurse -Force -ErrorAction SilentlyContinue; `+
			`Remove-Item -Path 'HKCU:\Software\Classes\Directory\Background\shell\%s' -Recurse -Force -ErrorAction SilentlyContinue; `, k, k))
	}
	if ps.Len() > 0 {
		_, _ = shell.RunPS(ps.String())
	}
}

func removeVibeCtxTree() {
	ps := fmt.Sprintf(`
Remove-Item -Path 'HKCU:\Software\Classes\%s' -Recurse -Force -ErrorAction SilentlyContinue`, ctxVibePath)
	_, _ = shell.RunPS(ps)
}

// ContextMenuSetup flujo completo del instalador: pregunta por las que falten y
// monta el menu.
func ContextMenuSetup() error {
	if err := ensureToolsInstalled(); err != nil {
		return err
	}
	fmt.Println()
	if err := ConfigureOpencode(true); err != nil {
		return err
	}
	fmt.Println()
	return installVibeCoding()
}
