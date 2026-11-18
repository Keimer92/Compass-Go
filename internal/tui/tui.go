package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"compass/internal/console"
	"compass/internal/container"
	"compass/internal/project"
	"compass/internal/shell"
	"compass/internal/theme"

	"github.com/gdamore/tcell/v2"
)

// frame es la espera del bucle con una pestana activa (≈30 fps): asi la salida
// del hijo y el cursor se ven fluir, y la pulsacion de una tecla no espera al
// siguiente fotograma.
const frame = 33 * time.Millisecond

var traceEnabled bool

// SetTrace activa/desactiva el registro de eventos en <tmpdir>/compass-tui.log.
func SetTrace(on bool) { traceEnabled = on }

func traceEvent(msg string) {
	if !traceEnabled {
		return
	}
	path := filepath.Join(os.TempDir(), "compass-tui.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, msg)
}

// Run arranca el menu interactivo. Equivale a menu::run de Rust.
func Run(ctx *project.ProjectContext) error {
	if traceEnabled {
		_ = os.WriteFile(filepath.Join(os.TempDir(), "compass-tui.log"), nil, 0o644)
	}

	th := theme.Load()

	screen, err := tcell.NewScreen()
	if err != nil {
		return err
	}
	if err := screen.Init(); err != nil {
		return err
	}
	defer screen.Fini()

	screen.EnableMouse()
	screen.SetStyle(tcell.StyleDefault.Background(th.Background).Foreground(th.Text))

	st := newMenuState(ctx.ProjectType, th)
	panes := container.New()
	console.Install()

	// Cada proyecto abre su propia pestana: el titulo es solo el directorio y el
	// acento sale del path.
	shell.SetConsoleTitle(ctx.WindowTitle())
	shell.SetTabColor(shell.TabAccent(ctx.ScriptDir))

	traceEvent(fmt.Sprintf("TERMINAL SIZE: %dx%d | project: %s", screenW(screen), screenH(screen), ctx.ScriptDir))

	// El teclado se lee en una goroutine para poder repintar el terminal a
	// ~30 fps aunque no llegue ningun evento.
	events := make(chan tcell.Event, 64)
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			ev := screen.PollEvent()
			if ev == nil {
				return
			}
			select {
			case events <- ev:
			case <-done:
				return
			}
		}
	}()

	lastTitle := time.Now()
	var prevBtn tcell.ButtonMask

	for {
		// Las tareas que terminan con cuenta atras salen pidiendo que se cierre
		// su pestana: se recolecta antes de dibujar.
		panes.ReapFinished()

		w, h := screen.Size()
		// st.theme cambia en vivo con el selector (preview); el fondo, las
		// pestanas y el terminal abierto deben seguirlo al instante.
		cur := st.theme
		fillRect(screen, Rect{0, 0, w, h}, ' ', bgStyle(cur))

		barH := container.BarHeight
		if barH > h {
			barH = h
		}
		bar := Rect{0, 0, w, barH}
		body := Rect{0, barH, w, h - barH}

		if panes.Active() >= 0 {
			// El terminal no tiene cards: el hit-testing del menu queda apagado
			// para que un clic no ejecute nada de la pantalla oculta.
			st.ui = nil
			panes.DrawBody(screen, container.Rect{X: body.X, Y: body.Y, Width: body.Width, Height: body.Height}, cur)
		} else {
			renderMenu(screen, st, ctx, body)
		}
		panes.DrawBar(screen, container.Rect{X: bar.X, Y: bar.Y, Width: bar.Width, Height: bar.Height}, cur)
		screen.Show()

		// Recuperar el titulo de la consola una vez por segundo.
		if time.Since(lastTitle) >= time.Second {
			lastTitle = time.Now()
			shell.ReclaimConsoleTitle(ctx.WindowTitle())
		}

		var ev tcell.Event
		switch {
		case panes.Active() >= 0:
			// Terminal incrustada: se repinta a ~30 fps aunque no lleguen
			// eventos, para ver fluir la salida del hijo.
			select {
			case ev = <-events:
			case <-time.After(frame):
				continue
			}
		case st.hoverAnimating() || st.selectAnimating():
			// Halo de hover en curso: mismo ritmo de fotogramas.
			select {
			case ev = <-events:
			case <-time.After(animFrame):
				continue
			}
		default:
			ev = <-events
		}
		if ev == nil {
			panes.CloseAll()
			return nil
		}

		switch e := ev.(type) {
		case *tcell.EventResize:
			screen.Sync()
		case *tcell.EventKey:
			if panes.Active() >= 0 {
				panes.HandleKey(e)
				continue
			}
			quit, err := handleKey(st, ctx, panes, screen, e)
			if err != nil {
				panes.CloseAll()
				return err
			}
			if quit {
				// Salir con pestanas abiertas mata sus procesos: se avisa una
				// vez y se pide confirmacion.
				if st.quit == nil && panes.TabCount() > 0 {
					st.quit = &QuitConfirm{tabs: panes.TabCount(), running: panes.RunningCount()}
					continue
				}
				panes.CloseAll()
				return nil
			}
		case *tcell.EventMouse:
			if panes.Active() >= 0 {
				panes.HandleMouse(e)
				// El menu no vio este evento: su estado del raton hay que
				// ponerlo al dia igualmente o, al volver a Inicio, la primera
				// pulsacion se tomaria por un arrastre y no abriria nada.
				prevBtn = e.Buttons()
				continue
			}
			handleMouse(st, ctx, panes, screen, e, &prevBtn)
		}
	}
}

// handleKey procesa una pulsacion del menu. Devuelve quit=true para salir.
func handleKey(st *MenuState, ctx *project.ProjectContext, panes *container.Container, screen tcell.Screen, e *tcell.EventKey) (bool, error) {
	// Modal de temas: el teclado le pertenece por completo.
	if st.picker != nil {
		switch e.Key() {
		case tcell.KeyEsc:
			st.theme = st.picker.original
			st.picker = nil
		case tcell.KeyEnter:
			theme.Save(st.theme)
			st.picker = nil
		case tcell.KeyUp, tcell.KeyLeft:
			st.previewTheme(theme.Prev(st.theme.Name))
		case tcell.KeyDown, tcell.KeyRight:
			st.previewTheme(theme.Next(st.theme.Name))
		case tcell.KeyCtrlT:
			st.theme = st.picker.original
			st.picker = nil
		}
		return false, nil
	}

	// Aviso de salida: Enter confirma, Esc/n cancelan.
	if st.quit != nil {
		switch e.Key() {
		case tcell.KeyEnter:
			return true, nil
		case tcell.KeyEsc:
			st.quit = nil
		default:
			if r := e.Rune(); r == 'n' || r == 'N' {
				st.quit = nil
			}
		}
		return false, nil
	}

	// Modal de opciones globales: Enter/Espacio alterna la casilla, Esc cierra.
	if st.settings != nil {
		switch e.Key() {
		case tcell.KeyEsc:
			st.settings = nil
		case tcell.KeyEnter:
			st.settings.toggle()
		case tcell.KeyRune:
			if e.Rune() == ' ' {
				st.settings.toggle()
			}
		}
		return false, nil
	}

	st.setHover(-1)

	// Ctrl+O abre las opciones globales.
	if e.Key() == tcell.KeyCtrlO {
		st.settings = newSettingsModal()
		return false, nil
	}

	// Ctrl+T abre el modal de temas.
	if e.Key() == tcell.KeyCtrlT {
		st.openPicker()
		return false, nil
	}

	switch e.Key() {
	case tcell.KeyEsc:
		return true, nil
	case tcell.KeyF12:
		panes.ShowLast()
	case tcell.KeyEnter:
		if st.selected >= 0 && st.selected < len(st.items) {
			openSelected(ctx, st.items[st.selected], panes, screen)
		}
	case tcell.KeyUp:
		st.moveRow(-1)
	case tcell.KeyDown:
		st.moveRow(1)
	case tcell.KeyLeft:
		st.moveSelection(-1)
	case tcell.KeyRight:
		st.moveSelection(1)
	case tcell.KeyTab:
		st.setTab(st.tab.next())
	case tcell.KeyBacktab:
		st.setTab(st.tab.prev())
	}
	return false, nil
}

// handleMouse procesa raton: rueda mueve filas; clic en pestana cambia; clic en
// card ejecuta. El hover sigue al puntero.
//
// El clic se resuelve al soltar, como en el contenedor y en los submenus. Si se
// resolviera al pulsar, un arrastre ejecutaria la card y el estado del raton
// quedaria "pulsado" al cruzar de una pestana al menu.
func handleMouse(st *MenuState, ctx *project.ProjectContext, panes *container.Container, screen tcell.Screen, e *tcell.EventMouse, prevBtn *tcell.ButtonMask) {
	x, y := e.Position()
	btn := e.Buttons()
	prev := *prevBtn
	*prevBtn = btn

	if btn&tcell.WheelUp != 0 {
		st.moveRow(-1)
	}
	if btn&tcell.WheelDown != 0 {
		st.moveRow(1)
	}

	if btn&tcell.Button1 == 0 && prev&tcell.Button1 != 0 {
		// Barra del contenedor: pestanas de tarea y sus botones ×.
		if panes.Click(x, y) {
			st.setHover(-1)
			return
		}
		// Barra de pestanas del menu.
		if t, ok := st.tabAt(x, y); ok {
			st.setTab(t)
			st.setHover(-1)
			return
		}
		// Boton [x] del recordatorio de InferX.
		if st.opencodeReminder && st.ui != nil && st.ui.ReminderButton != nil &&
			st.ui.ReminderButton.Contains(x, y) {
			st.opencodeReminder = false
			st.setHover(-1)
			return
		}
		// Card.
		if idx := st.hoverAt(x, y); idx >= 0 {
			st.jumpTo(idx)
			openSelected(ctx, st.items[idx], panes, screen)
			st.setHover(-1)
			return
		}
	}

	// El hover se actualiza con cualquier actividad del raton.
	st.setHover(st.hoverAt(x, y))
}

// openSelected abre la tarea de la card en una pestana incrustada. Si ConPTY
// no arranca, cae a la ventana externa de siempre.
func openSelected(ctx *project.ProjectContext, item MenuItem, panes *container.Container, screen tcell.Screen) {
	task := menuItemToTask(item, ctx.ProjectType)
	if task == "" {
		return
	}

	// COMPASS_EXTERNAL_WINDOWS=1 recupera el comportamiento antiguo (una
	// ventana por tarjeta) sin recompilar.
	if os.Getenv("COMPASS_EXTERNAL_WINDOWS") != "" {
		spawnInWindow(ctx, task)
		return
	}

	exe, err := os.Executable()
	if err != nil {
		spawnInWindow(ctx, task)
		return
	}

	script := container.PaneScript(exe, task, ctx.ScriptDir)
	w, h := screen.Size()
	rows := h - container.BarHeight - 1
	if rows < 2 {
		rows = 2
	}
	title := item.Label(ctx.ProjectType)

	traceEvent("OPEN TAB: " + task)
	if err := panes.Open(title, script, ctx.ScriptDir, rows, w); err != nil {
		// Sin ConPTY la tarjeta no puede quedarse sin hacer nada.
		traceEvent(fmt.Sprintf("SESSION ERROR: %v", err))
		spawnInWindow(ctx, task)
	}
}

// spawnInWindow abre la tarea en una ventana externa (respaldo).
func spawnInWindow(ctx *project.ProjectContext, task string) {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	command := fmt.Sprintf("\"%s\" --task %s --path \"%s\"", exe, task, ctx.ScriptDir)
	title := "Compass - " + filepath.Base(ctx.ScriptDir)
	_ = shell.SpawnNewWindowAutoexit(title, ctx.ScriptDir, command)
}

// menuItemToTask mapea una card a su tarea CLI segun el tipo de proyecto.
func menuItemToTask(item MenuItem, pt project.ProjectType) string {
	switch item {
	case WindowsDev:
		return "windows-dev"
	case AndroidDev:
		return "android-dev"
	case AndroidHmr:
		switch pt {
		case project.TypeGoWails, project.TypePython:
			return "android-hmr"
		case project.TypeGo:
			return "go-hmr"
		}
	case WindowsBuild:
		switch pt {
		case project.TypeGo, project.TypeGoWails:
			return "windows-release"
		default:
			return "windows-build"
		}
	case AndroidBuildArm64:
		switch pt {
		case project.TypeFlutter, project.TypeKotlin, project.TypeGoWails:
			return "android-release"
		}
	case AndroidBuildAll:
		switch pt {
		case project.TypeFlutter, project.TypeKotlin, project.TypeGoWails:
			return "android-aab"
		}
	case InstallDebug:
		return "android-install-debug"
	case Tests:
		switch pt {
		case project.TypePython:
			return "python-test"
		case project.TypeGo:
			return "go-test"
		}
	case Clean:
		return "clean"
	case DebugAndroid:
		return "debug-android"
	case Signing:
		return "signing"
	case VersionItem:
		return "version"
	case Git:
		return "git"
	case BuildWhatsApp:
		return "whatsapp"
	case ConfigureTerminal:
		return "terminal"
	case Environment:
		return "environment"
	}
	return ""
}
