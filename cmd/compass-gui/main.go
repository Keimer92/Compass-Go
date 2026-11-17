// Command compass-gui es la GUI nativa (Gio) de Compass Go. Durante la fase 1
// convive con cmd/compass (la TUI de tcell), que sigue siendo la version
// estable; se lanza con -p o, sin argumentos, sobre el directorio actual.
package main

import (
	"flag"
	"fmt"
	"os"

	"compass/internal/gui"
	"compass/internal/project"
	"compass/internal/tasks"
	"compass/internal/workspace"
)

func main() {
	var task, shortTask, path string
	flag.StringVar(&task, "task", "", "ejecutar una tarea directamente")
	flag.StringVar(&shortTask, "t", "", "alias de --task")
	flag.StringVar(&path, "path", "", "ruta del proyecto")
	flag.StringVar(&path, "p", "", "alias de --path")
	flag.Parse()

	dir := path
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if err := os.Chdir(dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Las cards abren el propio binario con --task dentro de una ConPTY.
	t := task
	if t == "" {
		t = shortTask
	}
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

	// Si ya hay una ventana abierta, le pasamos la carpeta y salimos: asi el
	// menu contextual apila proyectos en la misma ventana.
	if workspace.TryForward(dir) {
		return
	}

	if err := gui.Run(dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
