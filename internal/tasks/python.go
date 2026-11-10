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
	Register("windows-dev", project.TypePython, PythonRun)
	Register("android-hmr", project.TypePython, PythonReload)
	Register("python-test", project.TypePython, PythonTest)
	Register("windows-build", project.TypePython, PythonBuild)
	Register("clean", project.TypePython, PythonClean)
}

// pythonInterpreter devuelve el ejecutable de Python: el del venv del proyecto
// si existe, o python.exe del PATH. Equivalente de python.rs::interpreter.
func pythonInterpreter(ctx *project.ProjectContext) string {
	venv := filepath.Join(ctx.ScriptDir, ".venv", "Scripts", "python.exe")
	if fileExists(venv) {
		return venv
	}
	return "python.exe"
}

// inferredScript deduce main.py o app.py si solo hay uno de los dos.
func inferredScript(ctx *project.ProjectContext) string {
	hasMain := fileExists(filepath.Join(ctx.ScriptDir, "main.py"))
	hasApp := fileExists(filepath.Join(ctx.ScriptDir, "app.py"))
	switch {
	case hasMain && !hasApp:
		return "main.py"
	case !hasMain && hasApp:
		return "app.py"
	default:
		return ""
	}
}

// pythonRunProcess ejecuta el interprete con los argumentos dados y pausa.
func pythonRunProcess(ctx *project.ProjectContext, args []string, purpose string) error {
	py := pythonInterpreter(ctx)
	fmt.Printf("[INFO] %s: %s %s\n", purpose, py, strings.Join(args, " "))
	ok, err := shell.RunProgram(py, args, ctx.ScriptDir, nil)
	if err != nil {
		return fmt.Errorf("no se pudo ejecutar Python desde '%s'. Instala Python o crea el entorno .venv del proyecto: %w", py, err)
	}
	fmt.Println()
	ui.Pause()
	if !ok {
		return fmt.Errorf("el proceso Python terminó con código de error")
	}
	return nil
}

// PythonRun ejecuta el script principal (main.py/app.py) o uno pedido.
func PythonRun(ctx *project.ProjectContext) error {
	entry := inferredScript(ctx)
	if entry == "" {
		target := ui.AskText("Script .py o módulo Python (por ejemplo src/main.py o paquete.cli)", "")
		target = strings.TrimSpace(target)
		if target == "" {
			return fmt.Errorf("no se indicó un script o módulo Python")
		}
		entry = target
	}

	var args []string
	if strings.HasSuffix(entry, ".py") {
		if !fileExists(filepath.Join(ctx.ScriptDir, entry)) {
			return fmt.Errorf("no existe el script Python: %s", filepath.Join(ctx.ScriptDir, entry))
		}
		args = []string{entry}
	} else {
		args = []string{"-m", entry}
	}
	return pythonRunProcess(ctx, args, "Ejecutando Python")
}

// PythonReload lanza Uvicorn con autorecarga para una app ASGI.
func PythonReload(ctx *project.ProjectContext) error {
	target := ui.AskText("Aplicación ASGI para Uvicorn (formato módulo:variable)", "main:app")
	target = strings.TrimSpace(target)
	if target == "" || !strings.Contains(target, ":") {
		return fmt.Errorf("el destino ASGI debe tener el formato módulo:variable")
	}
	args := []string{"-m", "uvicorn", target, "--reload", "--host", "127.0.0.1"}
	return pythonRunProcess(ctx, args, "Iniciando Uvicorn con autorecarga")
}

// PythonTest ejecuta 'python -m pytest'.
func PythonTest(ctx *project.ProjectContext) error {
	return pythonRunProcess(ctx, []string{"-m", "pytest"}, "Ejecutando pruebas con pytest")
}

// PythonBuild construye wheel y sdist con 'python -m build'.
func PythonBuild(ctx *project.ProjectContext) error {
	return pythonRunProcess(ctx, []string{"-m", "build"}, "Construyendo wheel y sdist")
}

// PythonClean elimina los artefactos y cachés de un proyecto Python.
func PythonClean(ctx *project.ProjectContext) error {
	root := ctx.ScriptDir
	type entry struct {
		label string
		path  string
		isDir bool
	}
	top := []entry{
		{"build/", filepath.Join(root, "build"), true},
		{"dist/", filepath.Join(root, "dist"), true},
		{"__pycache__/", filepath.Join(root, "__pycache__"), true},
		{".pytest_cache/", filepath.Join(root, ".pytest_cache"), true},
		{".mypy_cache/", filepath.Join(root, ".mypy_cache"), true},
		{".ruff_cache/", filepath.Join(root, ".ruff_cache"), true},
		{".venv/", filepath.Join(root, ".venv"), true},
		{"venv/", filepath.Join(root, "venv"), true},
	}
	for _, e := range top {
		if e.isDir {
			if dirExists(e.path) {
				_ = shell.RemoveDir(e.path)
				fmt.Printf("[OK] Eliminado %s\n", e.label)
			}
		} else if fileExists(e.path) {
			_ = os.Remove(e.path)
			fmt.Printf("[OK] Eliminado %s\n", e.label)
		}
	}
	for _, name := range []string{"__pycache__", ".pytest_cache", ".mypy_cache", ".ruff_cache"} {
		for _, p := range findDirsNamed(root, name) {
			_ = shell.RemoveDir(p)
			fmt.Printf("[OK] Eliminado %s\n", p)
		}
	}
	fmt.Println("[OK] Limpieza Python completada.")
	return nil
}
