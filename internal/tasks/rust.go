package tasks

import (
	"fmt"
	"path/filepath"

	"compass/internal/project"
	"compass/internal/shell"
)

func init() {
	Register("windows-dev", project.TypeRust, RustRun)
	Register("windows-build", project.TypeRust, RustBuild)
	Register("clean", project.TypeRust, RustClean)
}

// RustRun ejecuta 'cargo run' en la ventana actual.
func RustRun(ctx *project.ProjectContext) error {
	return shell.RunInCurrentWindow("cargo run", ctx.ScriptDir)
}

// RustBuild compila el release y revela el artefacto con nombre+versión.
func RustBuild(ctx *project.ProjectContext) error {
	releaseDir := filepath.Join(ctx.ScriptDir, "target", "release")
	name, version := readCargoMeta(ctx)
	var rename string
	if name != "" {
		if version != "" {
			rename = fmt.Sprintf("%s v%s", name, version)
		} else {
			rename = name
		}
	}
	_, err := shell.RunBuildAndReveal("cargo build --release", ctx.ScriptDir, releaseDir, []string{"exe"}, rename)
	return err
}

// RustClean ejecuta 'cargo clean'.
func RustClean(ctx *project.ProjectContext) error {
	fmt.Println("[INFO] Ejecutando 'cargo clean'...")
	if err := shell.RunInDirStatus("cargo", ctx.ScriptDir, "clean"); err != nil {
		return err
	}
	fmt.Println("[OK] 'cargo clean' completado.")
	return nil
}
