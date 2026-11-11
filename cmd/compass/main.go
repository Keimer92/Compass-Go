// Command compass es la entrada principal de la herramienta TUI para Windows.
// Con --task ejecuta una tarea headless; sin --task abre el menu interactivo.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"compass/internal/lock"
	"compass/internal/project"
	"compass/internal/prompts"
	"compass/internal/shell"
	"compass/internal/tasks"
	"compass/internal/tui"
	"compass/internal/window"
)

func main() {
	var (
		task string
		path string
	)
	// Alias cortos como en el CLI Rust: -t/--task y -p/--path. El menú
	// contextual invoca `compass -p '<carpeta>'`, así que -p es obligatorio.
	flag.StringVar(&task, "task", "", "Ejecutar tarea directamente (ej: windows-dev, clean)")
	flag.StringVar(&task, "t", "", "Alias de --task")
	flag.StringVar(&path, "path", "", "Ruta del proyecto (se autodetecta por defecto)")
	flag.StringVar(&path, "p", "", "Alias de --path")
	pick := flag.Bool("pick", false, "Elegir la carpeta del proyecto con un navegador")
	trace := flag.Bool("trace", false, "Registrar eventos del menú TUI en <tmpdir>/compass-tui.log")
	flag.Parse()

	// Tareas globales (no dependen de un proyecto): menu contextual de Windows.
	if task != "" && tasks.IsGlobalTask(task) {
		if err := tasks.RunGlobalTask(task); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	// ── Resolucion de la carpeta del proyecto ──
	dir, err := resolveDir(*pick, path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.Chdir(dir); err != nil {
		fmt.Fprintln(os.Stderr, "No se pudo cambiar al directorio:", err)
		os.Exit(1)
	}

	// context-gitignore opera sobre el directorio elegido (sin detectar proyecto).
	if task == "context-gitignore" {
		if err := tasks.AddContextGitignoreFromDir(dir); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	// La instancia unica solo aplica a la TUI (sin --task).
	if task == "" {
		// Sin --task, un fallo de deteccion ofrece elegir la carpeta.
		ctx, err := project.Detect()
		if err != nil {
			fmt.Fprintf(os.Stderr, "\n[WARN] %v\n\n", err)
			chosen, perr := prompts.PickProjectRoot(dir)
			if perr != nil || chosen == "" {
				fmt.Println("[INFO] No se eligio ningun proyecto.")
				return
			}
			if err := os.Chdir(chosen); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			ctx, err = project.Detect()
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}

		// Un solo Compass por proyecto: si ya hay uno abierto para esta
		// carpeta, se trae su ventana al frente en vez de abrir otra.
		guard, free, err := lock.TryAcquire(ctx.ScriptDir)
		switch {
		case err != nil:
			fmt.Fprintln(os.Stderr, "[WARN] No se pudo verificar instancia unica:", err)
		case !free:
			reuseOpenInstance(ctx)
			return
		default:
			defer guard.Close()
		}

		tui.SetTrace(*trace)
		if err := tui.Run(ctx); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	ctx, err := project.Detect()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	shell.SetConsoleTitle(ctx.WindowTitle())
	shell.SetTabColor(shell.TabAccent(ctx.ScriptDir))

	if err := tasks.RunTaskDirect(ctx, task); err != nil {
		fmt.Fprintln(os.Stderr, err)
		// Un fallo conserva su pestana para poder leer el mensaje (Rust sale
		// con codigo 1 al propagar el Err de run_task_direct).
		os.Exit(1)
	}

	if shell.CloseTabRequested() {
		os.Exit(shell.ExitCloseTab)
	}
}

// reuseOpenInstance evita una segunda ventana para el mismo proyecto: lleva al
// frente la que ya esta abierta y, si no la encuentra, deja el aviso a la vista
// un momento antes de cerrar esta (la lanzada desde el menu contextual abre su
// propia consola y desaparece al salir).
func reuseOpenInstance(ctx *project.ProjectContext) {
	if window.Focus(ctx.WindowTitle()) {
		return
	}
	fmt.Printf("[INFO] Ya hay un Compass Go abierto para %s.\n", ctx.ScriptDir)
	time.Sleep(2 * time.Second)
}

// resolveDir aplica la misma prioridad que Rust: --pick > --path > autodeteccion.
func resolveDir(pick bool, p string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if pick {
		chosen, err := prompts.PickProjectRoot(cwd)
		if err != nil {
			return "", err
		}
		if chosen == "" {
			return "", fmt.Errorf("[INFO] No se eligio ningun proyecto")
		}
		return chosen, nil
	}
	if p != "" {
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("la ruta especificada no existe: %s", p)
		}
		return p, nil
	}
	return cwd, nil
}
