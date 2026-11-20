// Command compass-web es la GUI web (Wails v3 + Svelte) de Compass Go.
package main

import (
	"embed"
	"flag"
	"fmt"
	"os"
	"strings"

	"compass/assets"
	"compass/internal/prompts"
	"compass/internal/tasks"
	"compass/internal/webui"
	"compass/internal/windialog"
	"compass/internal/workspace"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var frontendFS embed.FS

func main() {
	// El binario es de subsistema GUI: sin esto, cuando Compass se reinvoca con
	// --task dentro de la ConPTY del terminal embebido no hay consola padre y la
	// salida de la tarea se perderia.
	windialog.AttachParentConsole()

	var task, shortTask, path string
	flag.StringVar(&task, "task", "", "tarea directa")
	flag.StringVar(&shortTask, "t", "", "alias de --task")
	flag.StringVar(&path, "path", "", "ruta del proyecto")
	flag.StringVar(&path, "p", "", "alias de --path")
	pick := flag.Bool("pick", false, "elegir la carpeta con un navegador")
	flag.Parse()

	t := task
	if t == "" {
		t = shortTask
	}

	// Las tareas globales (menu contextual, vibe coding, opencode...) no
	// dependen de un proyecto y se despachan antes de detectar nada.
	if t != "" && tasks.IsGlobalTask(t) {
		if err := tasks.RunGlobalTask(t); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	// Explorer/pwsh pueden dejar comillas literales en la ruta.
	path = strings.Trim(strings.TrimSpace(path), `"`)

	dir := path
	if dir == "" || *pick {
		cwd, err := os.Getwd()
		if err != nil {
			fail(err)
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

	if t == "context-gitignore" {
		if err := tasks.AddContextGitignoreFromDir(dir); err != nil {
			fail(err)
		}
		return
	}

	// Las cards abren este mismo binario con --task dentro de una ConPTY.
	if t != "" {
		if err := webui.RunTaskDirect(t, dir); err != nil {
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

	runApp(dir)
}

func runApp(dir string) {
	bridge := webui.NewBridge(dir)
	app := application.New(application.Options{
		Name:        "Compass Go",
		Description: "Compass Go - GUI web",
		// La barra de titulo usa el icono del exe; como go-winres lo embebe con
		// un ID que Wails no busca, se le pasa explicitamente.
		Icon: assets.CompassICO,
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(frontendFS),
		},
		Services: []application.Service{
			application.NewService(bridge),
		},
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: false,
		},
	})

	// Sin esto, los eventos pty:data / pty:exit no llegan al frontend.
	webui.SetApp(app)

	// IPC: otra instancia (menu contextual / segunda invocacion) puede apilar
	// una carpeta en esta ventana en vez de abrir una nueva.
	if stop, err := workspace.Listen(bridge.AddProject); err == nil {
		app.OnShutdown(stop)
	}

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:       "main",
		Title:      "Compass Go",
		Width:      1280,
		Height:     800,
		MinWidth:   900,
		MinHeight:  600,
		StartState: application.WindowStateMaximised,
	})

	if err := app.Run(); err != nil {
		fail(err)
	}
}

// fail muestra el error en un dialogo nativo (no hay consola propia).
func fail(err error) {
	windialog.Error("Compass Go", err.Error())
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
