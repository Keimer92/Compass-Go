// Command compass-gui es la GUI nativa (Gio) de Compass Go. Es el binario que
// debe sustituir a cmd/compass: abre la ventana con -p y, con --task, ejecuta
// tareas (globales o de proyecto) dentro de una ConPTY.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"compass/internal/gui"
	"compass/internal/project"
	"compass/internal/prompts"
	"compass/internal/tasks"
	"compass/internal/windialog"
	"compass/internal/workspace"
)

func main() {
	// Al estar compilado como app GUI no hay consola propia. Las tareas --task
	// corren dentro de una ConPTY: enganchamos la consola padre para que su
	// salida siga llegando al terminal embebido.
	windialog.AttachParentConsole()

	var task, shortTask, path string
	flag.StringVar(&task, "task", "", "ejecutar una tarea directamente")
	flag.StringVar(&shortTask, "t", "", "alias de --task")
	flag.StringVar(&path, "path", "", "ruta del proyecto")
	flag.StringVar(&path, "p", "", "alias de --path")
	pick := flag.Bool("pick", false, "elegir la carpeta con un navegador")
	flag.Parse()

	// Las tareas globales no necesitan proyecto (instalador, menu contextual,
	// vibe coding...). Se despachan antes de resolver la carpeta.
	t := task
	if t == "" {
		t = shortTask
	}
	if t != "" && tasks.IsGlobalTask(t) {
		if err := tasks.RunGlobalTask(t); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	// Explorer/pwsh pueden dejar comillas literal en la ruta (p.ej.
	// `C:\proy\"` cuando %1 termina en backslash). Las quitamos antes de
	// usarla; un backslash final normal lo acepta os.Chdir.
	path = strings.Trim(strings.TrimSpace(path), `"`)
	dir := path
	if dir == "" || *pick {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if *pick {
			chosen, err := prompts.PickProjectRoot(cwd)
			if err != nil || chosen == "" {
				return
			}
			dir = chosen
		} else {
			dir = cwd
		}
	}
	if err := os.Chdir(dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if t == "context-gitignore" {
		if err := tasks.AddContextGitignoreFromDir(dir); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	// Las cards abren el propio binario con --task dentro de una ConPTY.
	if t != "" {
		ctx, err := project.Detect()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := tasks.RunTaskDirect(ctx, t); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	// Si ya hay una ventana abierta, le pasamos la carpeta y salimos: así el
	// menú contextual apila proyectos en la misma ventana.
	if workspace.TryForward(dir) {
		return
	}

	if err := gui.Run(dir); err != nil {
		var detectErr *project.DetectError
		if errors.As(err, &detectErr) {
			windialog.Error("Compass Go · Directorio no admitido", fmt.Sprintf(
				"La carpeta no contiene un proyecto reconocido:\n\n%s\n\n"+
					"Tipos admitidos: Flutter, Kotlin, Python, Go y Go + Wails.\n\n"+
					"%s", dir, err))
		} else {
			windialog.Error("Compass Go", err.Error())
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
