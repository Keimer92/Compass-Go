package tasks

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"compass/internal/menu"
	"compass/internal/prompts"
	"compass/internal/project"
	"compass/internal/shell"
	"compass/internal/ui"
)

//go:embed assets/env-backup/Backup-Variables.ps1
var backupScript string

//go:embed assets/env-backup/Restore-Variables.ps1
var restoreScript string

const (
	envBackupFile  = "Backup-Variables.ps1"
	envRestoreFile = "Restore-Variables.ps1"
)

func init() {
	for _, pt := range allTypes() {
		Register("environment", pt, EnvManage)
		Register("env", pt, EnvManage)
		Register("env-backup", pt, EnvManage)
		Register("env-restore", pt, EnvManage)
		// Acciones individuales para las subcards nativas de la GUI.
		Register("env-do-backup", pt, func(ctx *project.ProjectContext) error {
			return EnvAction("backup")
		})
		Register("env-do-inventory", pt, func(ctx *project.ProjectContext) error {
			return EnvAction("inventory")
		})
		Register("env-do-restore", pt, func(ctx *project.ProjectContext) error {
			return EnvAction("restore")
		})
		Register("env-do-validate", pt, func(ctx *project.ProjectContext) error {
			return EnvAction("validate")
		})
		Register("env-do-open", pt, func(ctx *project.ProjectContext) error {
			return EnvAction("open")
		})
	}
}

// EnvAction ejecuta una opción del submenú de entorno sin dibujar el submenú de
// tcell. Prepara el directorio de trabajo igual que EnvManage.
func EnvAction(action string) error {
	work, err := envWorkDir()
	if err != nil {
		return err
	}
	if err := envMaterialize(work); err != nil {
		return err
	}
	for _, e := range envEntries() {
		if e.key == action && e.run != nil {
			return e.run(work)
		}
	}
	return nil
}

type envAction struct {
	key  string
	card menu.SubItem
	run  func(string) error
}

func envWorkDir() (string, error) {
	if dataDir() == "" {
		return "", fmt.Errorf("no se pudo determinar LOCALAPPDATA para guardar el entorno")
	}
	dir := filepath.Join(dataDir(), "compass", "env-backup")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("no se pudo crear %s: %w", dir, err)
	}
	return dir, nil
}

func envMaterialize(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	for name, content := range map[string]string{envBackupFile: backupScript, envRestoreFile: restoreScript} {
		path := filepath.Join(dir, name)
		current, err := os.ReadFile(path)
		stale := err != nil || string(current) != content
		if stale {
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				return fmt.Errorf("no se pudo escribir %s: %w", path, err)
			}
		}
	}
	return nil
}

func envEntries() []envAction {
	return []envAction{
		{"backup", menu.SubItem{
			Icon: "\U0001f4be", Label: "Backup completo",
			Description: "Variables, inventario y archivos", ColorKey: "environment",
		}, func(work string) error { return runBackup(work, false) }},
		{"inventory", menu.SubItem{
			Icon: "\U0001f4cb", Label: "Solo inventario",
			Description: "Herramientas y versiones, sin copiar", ColorKey: "version",
		}, func(work string) error { return runBackup(work, true) }},
		{"restore", menu.SubItem{
			Icon: "\u267b", Label: "Restaurar entorno",
			Description: "Variables, rutas y rollback", ColorKey: "clean",
		}, func(work string) error { return runRestore(work, false) }},
		{"validate", menu.SubItem{
			Icon: "\U0001f50d", Label: "Validar backup",
			Description: "Diff sin modificar nada", ColorKey: "debug",
		}, func(work string) error { return runRestore(work, true) }},
		{"open", menu.SubItem{
			Icon: "\U0001f4c2", Label: "Abrir respaldos",
			Description: "Carpeta de respaldos de Compass", ColorKey: "git",
		}, runOpenBackups},
	}
}

// EnvManage es el punto de entrada del submenu de entorno.
//
// Tras ejecutar una accion se vuelve al submenu, como en Rust.
func EnvManage(ctx *project.ProjectContext) error {
	work, err := envWorkDir()
	if err != nil {
		shell.ClearScreen()
		fmt.Printf("[ERROR] No se pudieron preparar los scripts de respaldo: %v\n", err)
		ui.Pause()
		return nil
	}
	if err := envMaterialize(work); err != nil {
		shell.ClearScreen()
		fmt.Printf("[ERROR] No se pudieron preparar los scripts de respaldo: %v\n", err)
		ui.Pause()
		return nil
	}

	entries := envEntries()
	labels := make([]string, len(entries))
	for i, e := range entries {
		labels[i] = e.card.Label + " · " + e.card.Description
	}
	idx, err := prompts.Select("Entorno · respaldo", labels)
	if err != nil || idx < 0 || idx >= len(entries) {
		return err
	}
	return entries[idx].run(work)
}

func defaultBackupRoot(work string) string {
	return filepath.Join(work, "Backup")
}

func backupScriptPath(work string) string  { return filepath.Join(work, envBackupFile) }
func restoreScriptPath(work string) string { return filepath.Join(work, envRestoreFile) }

func runBackup(work string, inventoryOnly bool) error {
	shell.ClearScreen()
	if inventoryOnly {
		ui.BoxBanner("INVENTARIO DEL ENTORNO")
	} else {
		ui.BoxBanner("BACKUP DEL ENTORNO DE DESARROLLO")
	}
	fmt.Println()
	if inventoryOnly {
		fmt.Println("  Guarda herramientas y versiones sin copiar archivos.")
	} else {
		fmt.Println("  Variables, inventario y archivos de configuracion")
		fmt.Println("  (.gitconfig, SSH, VS Code, keystores...).")
	}
	fmt.Println("  Ejecuta Compass como Administrador para incluir las variables")
	fmt.Println("  de SISTEMA.")
	fmt.Println()

	defaultDest := defaultBackupRoot(work)
	dest := ui.AskText("Destino de los respaldos", defaultDest)
	if strings.TrimSpace(dest) == "" {
		dest = defaultDest
	}

	keepStr := ui.AskText("Respaldos a conservar", "1")
	keep, err := strconv.Atoi(strings.TrimSpace(keepStr))
	if err != nil || keep < 1 {
		keep = 1
	}
	if keep > 99 {
		keep = 99
	}

	script := backupScriptPath(work)
	if !fileExists(script) {
		if err := envMaterialize(work); err != nil {
			return err
		}
		script = backupScriptPath(work)
	}

	args := []string{"-Destino", dest}
	if inventoryOnly {
		args = append(args, "-SoloInventario")
	}
	args = append(args, "-ConservarN", strconv.Itoa(keep))

	fmt.Println()
	ok, err := shell.RunPSFile(script, args, work, nil)
	if err != nil {
		fmt.Printf("[ERROR] No se pudo ejecutar el respaldo: %v\n", err)
	}
	if !ok {
		fmt.Println()
		fmt.Println("[AVISO] El respaldo termino con fallos; revisa el resumen de arriba.")
	}
	fmt.Println()
	ui.Pause()
	return nil
}

func runRestore(work string, validateOnly bool) error {
	shell.ClearScreen()
	if validateOnly {
		ui.BoxBanner("VALIDAR BACKUP DEL ENTORNO")
	} else {
		ui.BoxBanner("RESTAURAR ENTORNO DE DESARROLLO")
	}
	fmt.Println()
	if validateOnly {
		fmt.Println("  Solo informa: no modifica variables ni archivos.")
	} else {
		fmt.Println("  Restaura variables, adapta rutas de usuario y guarda un")
		fmt.Println("  respaldo del estado actual antes de tocar nada.")
		fmt.Println("  Ejecuta Compass como Administrador para restaurar las")
		fmt.Println("  variables de SISTEMA.")
	}
	fmt.Println()

	latest := latestBackup(work)
	var def string
	if latest != "" {
		def = latest
	}
	folder := ui.AskText("Carpeta del backup", def)

	script := restoreScriptPath(work)
	if !fileExists(script) {
		if err := envMaterialize(work); err != nil {
			return err
		}
		script = restoreScriptPath(work)
	}

	var args []string
	if strings.TrimSpace(folder) != "" {
		args = append(args, "-CarpetaBackup", folder)
	}
	if validateOnly {
		args = append(args, "-SoloValidar")
	} else {
		files := ui.AskText("Restaurar tambien archivos (git/ssh/VS Code)? (s/N)", "n")
		if ui.IsYes(files) {
			args = append(args, "-RestaurarArchivos")
		}
	}

	fmt.Println()
	ok, err := shell.RunPSFile(script, args, work, nil)
	if err != nil {
		fmt.Printf("[ERROR] No se pudo ejecutar la restauracion: %v\n", err)
	}
	if !ok {
		fmt.Println()
		fmt.Println("[AVISO] La restauracion termino con errores; revisa el resumen de arriba.")
	}
	fmt.Println()
	ui.Pause()
	return nil
}

func latestBackup(work string) string {
	root := defaultBackupRoot(work)
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	var dirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if !strings.HasPrefix(e.Name(), "EnvBackup_") {
			continue
		}
		dirs = append(dirs, filepath.Join(root, e.Name()))
	}
	if len(dirs) == 0 {
		return ""
	}
	// Orden alfabetico == cronologico (AAAA-MM-DD_HHMMSS).
	var best string
	for _, d := range dirs {
		if best == "" || d > best {
			best = d
		}
	}
	return best
}

func runOpenBackups(work string) error {
	root := defaultBackupRoot(work)
	if err := os.MkdirAll(root, 0755); err != nil {
		return err
	}
	cmd := exec.Command("explorer.exe", root)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("no se pudo abrir %s: %w", root, err)
	}
	return nil
}

// selectIdx muestra un menu de seleccion (texto) y devuelve el indice.
func selectIdx(title string, options []string) (int, error) {
	fmt.Println()
	fmt.Printf("╔══════════════════════════════════════════════════╗\n")
	fmt.Printf("║  %s\n", title)
	fmt.Println("╚══════════════════════════════════════════════════╝")
	for i, o := range options {
		fmt.Printf("  %2d. %s\n", i+1, o)
	}
	fmt.Print("\nOpcion: ")
	var line string
	fmt.Scanln(&line)
	n, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || n < 1 || n > len(options) {
		return -1, fmt.Errorf("opcion invalida")
	}
	return n - 1, nil
}
