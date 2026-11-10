package tasks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"compass/internal/project"
	"compass/internal/shell"
	"compass/internal/ui"
)

func init() {
	Register("windows-dev", project.TypeGo, GoRun)
	Register("windows-build", project.TypeGo, GoBuild)
	Register("go-hmr", project.TypeGo, GoReload)
	Register("go-test", project.TypeGo, GoTest)
	Register("clean", project.TypeGo, GoClean)
}

// GoArch es una arquitectura de Windows para un build cruzado de Go.
type GoArch int

const (
	GoX86 GoArch = iota
	GoX64
	GoArm64
)

func (a GoArch) goarch() string {
	switch a {
	case GoX86:
		return "386"
	case GoX64:
		return "amd64"
	case GoArm64:
		return "arm64"
	}
	return ""
}

func (a GoArch) label() string {
	switch a {
	case GoX86:
		return "x86"
	case GoX64:
		return "x64"
	case GoArm64:
		return "arm64"
	}
	return ""
}

// GoRun ejecuta 'go run .' en la ventana actual.
func GoRun(ctx *project.ProjectContext) error {
	return shell.RunInCurrentWindow("go run .", ctx.ScriptDir)
}

// GoReload ejecuta Air para reconstruir y reiniciar al cambiar archivos.
func GoReload(ctx *project.ProjectContext) error {
	fmt.Println("[INFO] Iniciando Air para reconstruir y reiniciar al cambiar archivos Go...")
	if err := shell.RunInDirStatus("air", ctx.ScriptDir); err != nil {
		return fmt.Errorf("no se pudo ejecutar Air. Instálalo con `go install github.com/air-verse/air@latest` y asegúrate de que GOPATH\\bin esté en PATH")
	}
	return nil
}

// GoBuild compila el proyecto a bin/<nombre>.exe y revela el artefacto.
func GoBuild(ctx *project.ProjectContext) error {
	outputDir := filepath.Join(ctx.ScriptDir, "bin")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("no se pudo crear %s: %w", outputDir, err)
	}
	outputName := executableName(ctx.ScriptDir)
	command := fmt.Sprintf("go build -o bin/%s .", outputName)
	_, err := shell.RunBuildAndReveal(command, ctx.ScriptDir, outputDir, []string{"exe"}, "")
	return err
}

// GoBuildArch compila un release de Go para una arquitectura de Windows.
func GoBuildArch(ctx *project.ProjectContext, arch GoArch) (string, error) {
	outputDir := filepath.Join(ctx.ScriptDir, "bin")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("no se pudo crear %s: %w", outputDir, err)
	}
	exe := executableName(ctx.ScriptDir)
	stem := strings.TrimSuffix(exe, ".exe")
	output := filepath.Join(outputDir, fmt.Sprintf("%s-%s.exe", stem, arch.label()))

	ok, err := shell.RunProgram("go", []string{"build", "-trimpath", "-o", output, "."},
		ctx.ScriptDir, map[string]string{
			"GOOS":        "windows",
			"GOARCH":      arch.goarch(),
			"CGO_ENABLED": "0",
		})
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("[ERROR] `go build` fallo para %s (GOARCH=%s). "+
			"Si el proyecto usa cgo, no puede compilarse cruzado sin el toolchain de C de esa arquitectura.",
			arch.label(), arch.goarch())
	}
	return output, nil
}

// GoTest ejecuta 'go test ./...' en la ventana actual.
func GoTest(ctx *project.ProjectContext) error {
	fmt.Println("[INFO] Ejecutando pruebas Go: go test ./...")
	return shell.RunInCurrentWindow("go test ./...", ctx.ScriptDir)
}

// GoClean ejecuta 'go clean' y elimina el directorio bin.
func GoClean(ctx *project.ProjectContext) error {
	fmt.Println("[INFO] Ejecutando go clean...")
	if err := shell.RunInDirStatus("go", ctx.ScriptDir, "clean"); err != nil {
		return err
	}
	bin := filepath.Join(ctx.ScriptDir, "bin")
	if shell.RemoveDir(bin) != nil {
		return fmt.Errorf("no se pudo eliminar %s", bin)
	}
	fmt.Println("[OK] Limpieza Go completada.")
	ui.Pause()
	return nil
}

func executableName(root string) string {
	name := filepath.Base(root)
	var b strings.Builder
	for _, ch := range name {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') ||
			ch == '-' || ch == '_' || ch == '.' {
			b.WriteRune(ch)
		} else {
			b.WriteRune('_')
		}
	}
	safe := b.String()
	if safe == "" {
		safe = "app"
	}
	return safe + ".exe"
}
