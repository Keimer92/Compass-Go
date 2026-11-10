package tasks

import (
	"fmt"
	"path/filepath"

	"compass/internal/project"
	"compass/internal/shell"
)

func init() {
	Register("windows-dev", project.TypeTauri, WindowsDev)
	Register("windows-build", project.TypeTauri, WindowsBuild)
}

// WindowsDev inicia el desarrollo de escritorio Tauri en una ventana nueva.
func WindowsDev(ctx *project.ProjectContext) error {
	if err := tauriEnsure(ctx); err != nil {
		return err
	}
	fmt.Println("\n[INFO] Iniciando desarrollo de escritorio...")
	fmt.Println("[INFO] Servidor: http://localhost:1420")
	return shell.SpawnNewWindowInteractive("Tauri Desktop (Dev)", ctx.ScriptDir, tauriCommand(ctx, "dev"))
}

// WindowsBuild compila el instalador de escritorio Tauri (Release).
func WindowsBuild(ctx *project.ProjectContext) error {
	if err := tauriEnsure(ctx); err != nil {
		return err
	}
	fmt.Println("\n[INFO] Compilando aplicación de escritorio (Release)...")
	bundleDir := filepath.Join(ctx.ScriptDir, "src-tauri", "target", "release", "bundle")
	name, version := readTauriMeta(ctx)
	var rename string
	if name != "" {
		if version != "" {
			rename = fmt.Sprintf("%s v%s", name, version)
		} else {
			rename = name
		}
	}
	_, err := shell.RunBuildAndReveal(tauriCommand(ctx, "build"), ctx.ScriptDir, bundleDir, []string{"exe", "msi"}, rename)
	return err
}
