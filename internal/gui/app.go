package gui

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"compass/internal/container"
	"compass/internal/project"
	"compass/internal/settings"
	"compass/internal/theme"
	"compass/internal/tui"
	"compass/internal/workspace"

	"gioui.org/app"
	"gioui.org/io/clipboard"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// frame es el ritmo de repintado mientras hay una pestana activa (≈30 fps).
const frame = 33 * time.Millisecond

// Application es la ventana de la GUI nativa de Compass.
type Application struct {
	w   *app.Window
	th  *material.Theme
	ops op.Ops

	ctx     *project.ProjectContext
	palette Palette
	themeIx int

	grid       *CardGrid
	panes      []*Pane
	activePane int // -1 = sin terminal activa

	ws           *workspace.Workspace
	ctxByProject map[string]*project.ProjectContext
	grids        map[string]*CardGrid
	panesByProj  map[string][]*Pane
	activeByProj map[string]int
	projBtns     []widget.Clickable
	termToggle   widget.Clickable
	termVisible  bool
	settings     settings.Settings
	optionsOpen  bool
	optionCheck  widget.Clickable
	optionsClose widget.Clickable
	optionsBtn   widget.Clickable
	explorer     *Explorer
	explorers    map[string]*Explorer
	ipcCh        chan string
	closeIPC     func()

	inputTag   byte
	termX      int
	termY      int
	picker     bool
	pickerIx   int
	pickerTmp  Palette
	themeBtn   widget.Clickable
	homeTab    widget.Clickable
	cfgTab     widget.Clickable
	pickerRows []widget.Clickable
}

// Run abre la ventana sobre projectDir y bloquea hasta que se cierra. El CLI
// headless y la TUI antigua (COMPASS_LEGACY_TUI) no se tocan.
func Run(projectDir string) error {
	a := &Application{
		w:            new(app.Window),
		th:           material.NewTheme(),
		themeIx:      theme.IndexOf(theme.Load().Name),
		ws:           workspace.Load(),
		ctxByProject: make(map[string]*project.ProjectContext),
		grids:        make(map[string]*CardGrid),
		panesByProj:  make(map[string][]*Pane),
		activeByProj: make(map[string]int),
		ipcCh:        make(chan string, 16),
		activePane:   -1,
		termVisible:  true,
		explorers:    make(map[string]*Explorer),
		settings:     settings.Load(),
	}
	a.palette = paletteFrom(theme.Themes[a.themeIx])
	a.th.Shaper = text.NewShaper()
	a.applyPalette()

	// La carpeta con la que se abrio entra en la lista (o se activa si ya
	// estaba) y luego se detecta el tipo de cada proyecto guardado.
	a.ws.AddAny(projectDir)
	for _, p := range a.ws.Projects {
		if err := os.Chdir(p.Path); err != nil {
			continue
		}
		ctx, err := project.Detect()
		if err != nil {
			continue
		}
		a.ctxByProject[p.Path] = ctx
		a.grids[p.Path] = NewCardGrid(ctx.ProjectType, a.openTask)
		a.explorers[p.Path] = NewExplorer(p.Path)
		a.activeByProj[p.Path] = -1
	}
	if len(a.ctxByProject) == 0 {
		return fmt.Errorf("no se pudo detectar ningun proyecto valido")
	}
	active := a.ws.ActiveProject()
	if active == nil || a.ctxByProject[active.Path] == nil {
		for path, ctx := range a.ctxByProject {
			a.ws.Active = indexOfPath(a.ws, path)
			active = &workspace.Project{Path: path, Type: ctx.ProjectType.Label()}
			break
		}
	}
	_ = os.Chdir(active.Path)
	a.ctx = a.ctxByProject[active.Path]
	a.grid = a.grids[active.Path]
	a.explorer = a.explorers[active.Path]
	a.panes = a.panesByProj[active.Path]
	a.activePane = a.activeByProj[active.Path]

	// El CLI de instancia unica nos entrega carpetas nuevas por IPC.
	cleanup, err := workspace.Listen(func(path string) { a.ipcCh <- path })
	if err != nil {
		return err
	}
	a.closeIPC = cleanup
	defer cleanup()

	// Barra nativa de Windows, maximizada al arrancar. app.Size pondria el
	// modo Windowed, asi que no se usa.
	a.w.Option(app.Title("Compass Go - "+a.ctx.WindowTitle()),
		app.Maximized.Option())
	// Tipografia mas cercana a San Francisco que Consolas en la interfaz
	// (el terminal sigue usando Consolas por ser monoespaciada).
	a.th.Face = "Segoe UI"
	a.th.TextSize = 14

	go a.pulse()
	go a.loop()
	app.Main()
	return nil
}

func indexOfPath(ws *workspace.Workspace, path string) int {
	for i, p := range ws.Projects {
		if p.Path == path {
			return i
		}
	}
	return 0
}

// saveCurrent guarda las pestañas del proyecto activo antes de cambiar.
func (a *Application) saveCurrent() {
	if cur := a.ws.ActiveProject(); cur != nil {
		a.panesByProj[cur.Path] = a.panes
		a.activeByProj[cur.Path] = a.activePane
	}
}

// addProject incorpora una carpeta recibida por IPC y la activa.
func (a *Application) addProject(dir string) {
	// Hay que guardar ANTES: AddAny cambia ws.Active al nuevo proyecto.
	a.saveCurrent()
	i, isNew := a.ws.AddAny(dir)
	if i < 0 {
		return
	}
	p := a.ws.Projects[i]
	if isNew || a.ctxByProject[p.Path] == nil {
		cwd, _ := os.Getwd()
		defer os.Chdir(cwd)
		if err := os.Chdir(p.Path); err != nil {
			return
		}
		ctx, err := project.Detect()
		if err != nil {
			return
		}
		a.ctxByProject[p.Path] = ctx
		a.grids[p.Path] = NewCardGrid(ctx.ProjectType, a.openTask)
		a.explorers[p.Path] = NewExplorer(p.Path)
	}
	a.loadProject(i)
	for len(a.projBtns) < len(a.ws.Projects) {
		a.projBtns = append(a.projBtns, widget.Clickable{})
	}
}

// switchProject guarda el estado actual y carga otro proyecto.
func (a *Application) switchProject(i int) {
	a.saveCurrent()
	a.loadProject(i)
	a.ws.Save()
}

// loadProject activa un proyecto ya detectado, sin guardar el anterior.
func (a *Application) loadProject(i int) {
	if i < 0 || i >= len(a.ws.Projects) {
		return
	}
	a.ws.Active = i
	p := a.ws.Projects[i]
	_ = os.Chdir(p.Path)
	a.ctx = a.ctxByProject[p.Path]
	a.grid = a.grids[p.Path]
	a.explorer = a.explorers[p.Path]
	if a.ctx == nil || a.grid == nil {
		return
	}
	a.panes = a.panesByProj[p.Path]
	a.activePane = a.activeByProj[p.Path]
	a.termVisible = len(a.panes) > 0
}

// applyPalette sincroniza la paleta con los material labels de Gio.
func (a *Application) applyPalette() {
	a.th.Palette.Fg = a.palette.Text
	a.th.Palette.Bg = a.palette.Background
	a.th.Palette.ContrastBg = a.palette.Accent
	a.th.Palette.ContrastFg = a.palette.Background
}

func (a *Application) togglePicker() {
	a.picker = !a.picker
	if a.picker {
		a.pickerIx = a.themeIx
		a.pickerTmp = a.palette
		a.pickerRows = make([]widget.Clickable, len(theme.Themes))
	}
}

func (a *Application) applyTheme(i int) {
	if i < 0 || i >= len(theme.Themes) {
		return
	}
	a.themeIx = i
	a.palette = paletteFrom(theme.Themes[i])
	a.applyPalette()
	theme.Save(theme.Themes[i])
	a.picker = false
}

func (a *Application) pulse() {
	ticker := time.NewTicker(frame)
	defer ticker.Stop()
	n := 0
	for range ticker.C {
		n++
		// El estado de Git y el arbol se refrescan cada ~5 s en segundo plano.
		if n%150 == 0 {
			if ex := a.explorer; ex != nil {
				go ex.Refresh()
			}
		}
		a.w.Invalidate()
	}
}

func (a *Application) loop() {
	for {
		e := a.w.Event()
		switch e := e.(type) {
		case app.DestroyEvent:
			for _, p := range a.panes {
				p.Session.Close()
			}
			return
		case app.FrameEvent:
			// Carpetas entregadas por el menu contextual (otra instancia).
		ipcLoop:
			for {
				select {
				case path := <-a.ipcCh:
					a.addProject(path)
				default:
					break ipcLoop
				}
			}
			a.reapFinished()
			gtx := app.NewContext(&a.ops, e)
			a.handleInput(gtx)
			a.layout(gtx)
			// Registrar el tag al final y SOLO sobre el area del terminal:
			// asi no intercepta los clics de cards, pestanas o temas.
			hit := clip.Rect(image.Rect(a.termX, a.termY,
				gtx.Constraints.Max.X, gtx.Constraints.Max.Y)).Push(gtx.Ops)
			event.Op(gtx.Ops, &a.inputTag)
			hit.Pop()
			e.Frame(gtx.Ops)
		}
	}
}

// handleInput registra el filtro de teclado y reparte las teclas segun el
// estado (selector de temas, menu o terminal).
func (a *Application) handleInput(gtx layout.Context) {
	// Gio solo encola EventKey si alguien registro un key.Filter en el frame.
	gtx.Execute(key.FocusCmd{Tag: &a.inputTag})
	for {
		ev, ok := gtx.Event(
			key.FocusFilter{Target: &a.inputTag},
			key.Filter{Focus: &a.inputTag, Name: ""},
		)
		if !ok {
			break
		}
		if e, ok := ev.(key.Event); ok && e.State == key.Press {
			a.onKey(e)
		}
	}

	// El raton (rueda y arrastre) usa el mismo tag registrado arriba.
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: &a.inputTag,
			Kinds:  pointer.Scroll | pointer.Press | pointer.Drag | pointer.Release,
		})
		if !ok {
			return
		}
		a.onPointer(ev.(pointer.Event), gtx)
	}
}

func (a *Application) onPointer(e pointer.Event, gtx layout.Context) {
	if !a.termVisible || a.activePane < 0 || a.activePane >= len(a.panes) {
		return
	}
	p := a.panes[a.activePane]
	cw, ch, cols, rows := p.Term.Metrics()
	if cw < 1 {
		cw, ch = 8, 16
	}
	if cols < 1 {
		cols, rows = 80, 25
	}
	x := int(e.Position.X) - a.termX
	y := int(e.Position.Y) - a.termY

	switch e.Kind {
	case pointer.Scroll:
		if p.Back != nil && !p.Back.Alt() {
			p.Back.Scroll(int(e.Scroll.Y) * 3)
		}
	case pointer.Press:
		p.Sel = selRange{
			active: true,
			fromX:  clamp(x/cw, 0, cols-1),
			fromY:  clamp(y/ch, 0, rows-1),
		}
		p.Sel.toX, p.Sel.toY = p.Sel.fromX, p.Sel.fromY
	case pointer.Drag:
		if p.Sel.active {
			p.Sel.toX = clamp(x/cw, 0, cols-1)
			p.Sel.toY = clamp(y/ch, 0, rows-1)
		}
	case pointer.Release:
		if p.Sel.active {
			p.Sel.toX = clamp(x/cw, 0, cols-1)
			p.Sel.toY = clamp(y/ch, 0, rows-1)
			if txt := selectionText(p, cols, rows); txt != "" {
				gtx.Execute(clipboard.WriteCmd{
					Type: "application/text",
					Data: io.NopCloser(strings.NewReader(txt)),
				})
			}
		}
	}
}

// ctrlLetter detecta Ctrl+letra aunque Gio entregue el nombre como caracter
// de control en vez de como letra (paso bajo SendKeys/IME).
func ctrlLetter(e key.Event, letter byte) bool {
	if e.Modifiers&key.ModCtrl == 0 {
		return false
	}
	if len(e.Name) == 1 {
		r := []rune(e.Name)[0]
		if r == rune(letter) || r == rune(letter)+('a'-'A') {
			return true
		}
		if r > 0 && r < 32 && byte(r) == letter-'A'+1 {
			return true
		}
	}
	return false
}

func (a *Application) onKey(e key.Event) {
	// Ctrl+O abre las opciones globales (mismo atajo que la TUI antigua).
	if ctrlLetter(e, 'O') {
		a.optionsOpen = !a.optionsOpen
		return
	}
	// Ctrl+T abre/cierra el selector de temas desde cualquier vista.
	if ctrlLetter(e, 'T') {
		a.togglePicker()
		return
	}

	if a.optionsOpen {
		if e.Name == key.NameEscape || e.Name == key.NameReturn ||
			e.Name == key.NameEnter {
			a.optionsOpen = false
		}
		return
	}

	if a.picker {
		switch e.Name {
		case key.NameUpArrow:
			a.pickerIx = (a.pickerIx + len(theme.Themes) - 1) % len(theme.Themes)
			a.pickerTmp = paletteFrom(theme.Themes[a.pickerIx])
		case key.NameDownArrow:
			a.pickerIx = (a.pickerIx + 1) % len(theme.Themes)
			a.pickerTmp = paletteFrom(theme.Themes[a.pickerIx])
		case key.NameReturn, key.NameEnter:
			a.applyTheme(a.pickerIx)
		case key.NameEscape:
			a.picker = false
		}
		return
	}

	// Escape oculta el panel de terminal (las cards siguen a la vista).
	if e.Name == key.NameEscape && a.termVisible && len(a.panes) > 0 {
		a.termVisible = false
		return
	}

	if !a.termVisible || len(a.panes) == 0 || a.activePane < 0 {
		switch e.Name {
		case key.NameUpArrow:
			a.grid.Move(0, -1)
		case key.NameDownArrow:
			a.grid.Move(0, 1)
		case key.NameLeftArrow:
			a.grid.Move(-1, 0)
		case key.NameRightArrow:
			a.grid.Move(1, 0)
		case key.NameReturn, key.NameEnter:
			a.grid.Activate()
		case key.NameTab:
			a.grid.SetTab(tui.TabCfg)
		}
		return
	}

	// Terminal: reenviar bytes a la ConPTY de la pestana activa.
	if a.activePane >= 0 && a.activePane < len(a.panes) {
		if b := encodeKey(e); len(b) > 0 {
			a.panes[a.activePane].Session.Write(b)
		}
	}
}

// openTask abre la tarea de una card en una pestana ConPTY.
func (a *Application) openTask(item tui.MenuItem) {
	task := taskFor(item, a.ctx.ProjectType)
	if task == "" {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	script := container.PaneScript(exe, task, a.ctx.ScriptDir)
	label := item.Label(a.ctx.ProjectType)
	p, err := a.newPane(label, task, script, a.ctx.ScriptDir)
	if err != nil {
		return
	}
	a.panes = append(a.panes, p)
	a.activePane = len(a.panes) - 1
	a.termVisible = true
}

// taskFor mapea una card a su tarea CLI segun el tipo de proyecto (misma
// logica que menuItemToTask de la TUI antigua).
func taskFor(item tui.MenuItem, pt project.ProjectType) string {
	switch item {
	case tui.WindowsDev:
		return "windows-dev"
	case tui.AndroidDev:
		return "android-dev"
	case tui.AndroidHmr:
		switch pt {
		case project.TypeGoWails, project.TypePython:
			return "android-hmr"
		case project.TypeGo:
			return "go-hmr"
		}
	case tui.WindowsBuild:
		switch pt {
		case project.TypeGo, project.TypeGoWails:
			return "windows-release"
		default:
			return "windows-build"
		}
	case tui.AndroidBuildArm64:
		return "android-release"
	case tui.AndroidBuildAll:
		return "android-aab"
	case tui.InstallDebug:
		return "android-install-debug"
	case tui.FlutterProfile:
		return "flutter-profile"
	case tui.Tests:
		switch pt {
		case project.TypePython:
			return "python-test"
		case project.TypeGo:
			return "go-test"
		}
	case tui.Clean:
		return "clean"
	case tui.DebugAndroid:
		return "debug-android"
	case tui.Signing:
		return "signing"
	case tui.VersionItem:
		return "version"
	case tui.Git:
		return "git"
	case tui.BuildWhatsApp:
		return "whatsapp"
	case tui.ConfigureTerminal:
		return "terminal"
	case tui.Environment:
		return "environment"
	}
	return ""
}

func (a *Application) layout(gtx layout.Context) {
	// Botones del raton (la GUI se puede usar sin atajos de teclado).
	for len(a.projBtns) < len(a.ws.Projects) {
		a.projBtns = append(a.projBtns, widget.Clickable{})
	}
	for i := range a.ws.Projects {
		if a.projBtns[i].Clicked(gtx) {
			a.switchProject(i)
		}
	}
	if a.termToggle.Clicked(gtx) {
		a.termVisible = !a.termVisible
	}
	if a.optionsBtn.Clicked(gtx) {
		a.optionsOpen = !a.optionsOpen
	}
	if a.optionCheck.Clicked(gtx) {
		a.settings.OpenOutputDir = !a.settings.OpenOutputDir
		settings.Save(a.settings)
	}
	if a.optionsClose.Clicked(gtx) {
		a.optionsOpen = false
	}
	if a.themeBtn.Clicked(gtx) {
		a.togglePicker()
	}
	if a.homeTab.Clicked(gtx) {
		a.activePane = -1
		a.grid.SetTab(tui.TabDevMnt)
	}
	if a.cfgTab.Clicked(gtx) {
		a.activePane = -1
		a.grid.SetTab(tui.TabCfg)
	}
	// Pestanas de tareas: clic para activar, × para cerrar.
	for i := range a.panes {
		if a.panes[i].closeBtn.Clicked(gtx) {
			a.closePane(i)
			break
		}
		if a.panes[i].tabBtn.Clicked(gtx) {
			a.activePane = i
		}
	}
	if a.picker {
		for i := range a.pickerRows {
			if a.pickerRows[i].Clicked(gtx) {
				a.applyTheme(i)
				break
			}
		}
	}

	w, h := gtx.Constraints.Max.X, gtx.Constraints.Max.Y
	sideW := gtx.Dp(unit.Dp(240))
	rightW := gtx.Dp(unit.Dp(300))
	headH := gtx.Dp(unit.Dp(44))

	p := &a.palette
	if a.picker {
		p = &a.pickerTmp
	}

	white := color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	paintRect(gtx, 0, 0, w, h, p.Background)
	// Sidebars con tono un poco mas claro (material traslucido estilo macOS).
	paintRect(gtx, 0, 0, sideW, h, blend(p.Background, white, 0.05))
	paintRect(gtx, w-rightW, 0, rightW, h, blend(p.Background, white, 0.03))
	paintRect(gtx, sideW, 0, w-sideW, headH, blend(p.Background, p.Border, 0.35))
	paintRect(gtx, sideW, 0, 1, h, p.Border)
	paintRect(gtx, w-rightW, 0, 1, h, p.Border)

	a.layoutSidebar(gtx, p, sideW, h)
	a.layoutHeader(gtx, p, sideW, headH, w-rightW)

	contentW, contentH := w-sideW-rightW, h-headH
	cg := gtx
	ct := op.Offset(image.Pt(sideW, headH)).Push(cg.Ops)

	if len(a.panes) > 0 && a.termVisible {
		// Panel de terminal abajo, estilo IDE: las cards siguen visibles arriba.
		panelH := contentH * 42 / 100
		minPanel := gtx.Dp(unit.Dp(190))
		minGrid := gtx.Dp(unit.Dp(200))
		if panelH < minPanel {
			panelH = minPanel
		}
		if contentH-panelH < minGrid {
			panelH = contentH - minGrid
		}
		if panelH < 80 {
			panelH = 80
		}
		gridH := contentH - panelH

		gg := cg
		gg.Constraints = layout.Exact(image.Pt(contentW, gridH))
		gridClip := clip.Rect(image.Rect(0, 0, contentW, gridH)).Push(gg.Ops)
		a.grid.Layout(gg, *p, a.th)
		gridClip.Pop()

		sep := image.Rect(0, gridH, contentW, gridH+2)
		s := clip.Rect(sep).Push(cg.Ops)
		paint.Fill(cg.Ops, p.Border)
		s.Pop()

		pg := cg
		pg.Constraints = layout.Exact(image.Pt(contentW, panelH))
		pt := op.Offset(image.Pt(0, gridH)).Push(pg.Ops)
		barH := gtx.Dp(unit.Dp(30))
		a.layoutTabBar(pg, *p, contentW, barH)

		tg := pg
		tg.Constraints = layout.Exact(image.Pt(contentW, panelH-barH))
		ttrans := op.Offset(image.Pt(0, barH)).Push(tg.Ops)
		if a.activePane >= 0 && a.activePane < len(a.panes) {
			a.panes[a.activePane].Term.Layout(tg, a.panes[a.activePane], p)
		}
		ttrans.Pop()
		pt.Pop()

		a.termX, a.termY = sideW, headH+gridH+barH
	} else {
		// Sin panel, area de impacto vacia para no tapar las cards.
		a.termX, a.termY = w, h
		cg.Constraints = layout.Exact(image.Pt(contentW, contentH))
		a.grid.Layout(cg, *p, a.th)
	}
	ct.Pop()

	// Panel derecho: arbol de archivos + cambios de Git.
	eg := gtx
	eg.Constraints = layout.Exact(image.Pt(rightW, h-headH))
	et := op.Offset(image.Pt(w-rightW, headH)).Push(eg.Ops)
	if a.explorer != nil {
		a.explorer.Layout(eg, *p, a.th)
	}
	et.Pop()

	if a.picker {
		a.layoutPicker(gtx, w, h)
	}
	if a.optionsOpen {
		a.layoutOptions(gtx, w, h)
	}
}

// layoutOptions es el modal de opciones globales (equivalente al Ctrl+O de la
// TUI). La casilla se persiste en %LOCALAPPDATA%\compass\settings.json.
func (a *Application) layoutOptions(gtx layout.Context, w, h int) {
	pw, ph := gtx.Dp(unit.Dp(420)), gtx.Dp(unit.Dp(180))
	panel := image.Rect((w-pw)/2, (h-ph)/2, (w+pw)/2, (h+ph)/2)

	dim := clip.Rect(image.Rect(0, 0, w, h)).Push(gtx.Ops)
	paint.Fill(gtx.Ops, color.NRGBA{R: 0, G: 0, B: 0, A: 0xaa})
	dim.Pop()
	stack := clip.Rect(panel).Push(gtx.Ops)
	paint.Fill(gtx.Ops, a.palette.Background)
	stack.Pop()

	bg := gtx
	bg.Constraints = layout.Exact(panel.Size())
	tr := op.Offset(panel.Min).Push(bg.Ops)
	layout.Inset{Top: unit.Dp(18), Left: unit.Dp(20), Right: unit.Dp(20)}.Layout(bg,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Body1(a.th, "Opciones")
					lbl.Color = a.palette.Brand
					return lbl.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return a.optionCheck.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							mark := "[ ]"
							if a.settings.OpenOutputDir {
								mark = "[x]"
							}
							lbl := material.Body2(a.th, mark+"  Abrir la carpeta de salida al terminar un build")
							lbl.Color = a.palette.Text
							return lbl.Layout(gtx)
						})
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Caption(a.th, "Cerrar")
						lbl.Color = a.palette.Muted
						return a.optionsClose.Layout(gtx, lbl.Layout)
					})
				}),
			)
		})
	tr.Pop()
}

func (a *Application) layoutSidebar(gtx layout.Context, p *Palette, sideW, h int) {
	sg := gtx
	sg.Constraints = layout.Exact(image.Pt(sideW, h))
	layout.Inset{Top: unit.Dp(18), Left: unit.Dp(16)}.Layout(sg, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body2(a.th, "COMPASS GO")
				lbl.Color = p.Brand
				return lbl.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Body1(a.th, a.ctx.WindowTitle())
					lbl.Color = p.Text
					return lbl.Layout(gtx)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Caption(a.th, a.ctx.ProjectType.Label())
					lbl.Color = p.Muted
					return lbl.Layout(gtx)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(28), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Caption(a.th, "PROYECTOS")
					lbl.Color = p.Muted
					return lbl.Layout(gtx)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				children := make([]layout.FlexChild, 0, len(a.ws.Projects))
				for i, proj := range a.ws.Projects {
					i, proj := i, proj
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.projectRow(gtx, p, i, proj)
					}))
				}
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
			}),
		)
	})
}

func (a *Application) projectRow(gtx layout.Context, p *Palette, i int,
	proj *workspace.Project) layout.Dimensions {

	active := i == a.ws.Active
	row := image.Rect(0, 0, gtx.Constraints.Max.X, gtx.Dp(unit.Dp(34)))
	if active {
		s := clip.Rect(row).Push(gtx.Ops)
		paint.Fill(gtx.Ops, blend(p.Background, p.Accent, 0.22))
		s.Pop()
		bar := image.Rect(0, 0, gtx.Dp(unit.Dp(3)), row.Max.Y)
		s = clip.Rect(bar).Push(gtx.Ops)
		paint.Fill(gtx.Ops, p.Accent)
		s.Pop()
	}
	return a.projBtns[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(12), Top: unit.Dp(7)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Body2(a.th, filepath.Base(proj.Path))
					if active {
						lbl.Color = p.Text
					} else {
						lbl.Color = p.Muted
					}
					return lbl.Layout(gtx)
				}),
			)
		})
	})
}

func (a *Application) layoutHeader(gtx layout.Context, p *Palette, sideW, headH, w int) {
	hg := gtx
	hg.Constraints = layout.Exact(image.Pt(w-sideW, headH))
	trans := op.Offset(image.Pt(sideW, 0)).Push(hg.Ops)
	layout.Inset{Left: unit.Dp(16), Top: unit.Dp(9)}.Layout(hg, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.tabButton(gtx, p, "Inicio", &a.homeTab,
					a.grid.tab == tui.TabDevMnt)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.tabButton(gtx, p, "Config", &a.cfgTab,
					a.grid.tab == tui.TabCfg)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				txt := "Terminal ▾"
				if a.termVisible && len(a.panes) > 0 {
					txt = "Terminal ▴"
				}
				lbl := material.Caption(a.th, txt)
				lbl.Color = p.Muted
				return layout.Inset{Left: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return a.termToggle.Layout(gtx, lbl.Layout)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Caption(a.th, "Opciones")
				lbl.Color = p.Muted
				return layout.Inset{Left: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return a.optionsBtn.Layout(gtx, lbl.Layout)
				})
			}),
		)
	})
	trans.Pop()

	// Boton de temas a la derecha.
	rg := gtx
	tw := gtx.Dp(unit.Dp(190))
	rg.Constraints = layout.Exact(image.Pt(tw, headH))
	tr := op.Offset(image.Pt(w-sideW-tw-gtx.Dp(unit.Dp(12)), 0)).Push(rg.Ops)
	layout.Inset{Top: unit.Dp(9)}.Layout(rg, func(gtx layout.Context) layout.Dimensions {
		lbl := material.Body2(a.th, "◈ "+p.Name+"  ▾")
		lbl.Color = p.Accent
		return a.themeBtn.Layout(gtx, lbl.Layout)
	})
	tr.Pop()
}

func (a *Application) tabButton(gtx layout.Context, p *Palette, name string,
	btn *widget.Clickable, active bool) layout.Dimensions {

	lbl := material.Body2(a.th, name)
	if active {
		lbl.Color = p.Brand
	} else {
		lbl.Color = p.Muted
	}
	dims := btn.Layout(gtx, lbl.Layout)
	if active {
		bar := image.Rect(0, dims.Size.Y-2, dims.Size.X, dims.Size.Y)
		s := clip.Rect(bar).Push(gtx.Ops)
		paint.Fill(gtx.Ops, p.Brand)
		s.Pop()
	}
	return dims
}

func (a *Application) layoutPicker(gtx layout.Context, w, h int) {
	pw, ph := gtx.Dp(unit.Dp(360)), gtx.Dp(unit.Dp(560))
	x, y := (w-pw)/2, (h-ph)/2
	panel := image.Rect(x, y, x+pw, y+ph)

	dimStack := clip.Rect(image.Rect(0, 0, w, h)).Push(gtx.Ops)
	paint.Fill(gtx.Ops, color.NRGBA{R: 0, G: 0, B: 0, A: 0xaa})
	dimStack.Pop()

	stack := clip.Rect(panel).Push(gtx.Ops)
	paint.Fill(gtx.Ops, a.palette.Background)
	stack.Pop()

	pg := gtx
	pg.Constraints = layout.Exact(panel.Size())
	pt := op.Offset(panel.Min).Push(pg.Ops)
	layout.Inset{Top: unit.Dp(12), Left: unit.Dp(16)}.Layout(pg, func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body1(a.th, "Temas  ↑↓  Enter  Esc")
				lbl.Color = a.palette.Brand
				return lbl.Layout(gtx)
			}),
		}
		for i, t := range theme.Themes {
			i, t := i, t
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if i == a.pickerIx {
					row := image.Rect(0, 0, gtx.Constraints.Max.X, gtx.Dp(unit.Dp(24)))
					s := clip.Rect(row).Push(gtx.Ops)
					paint.Fill(gtx.Ops, a.palette.HoverBg)
					s.Pop()
				}
				lbl := material.Body2(a.th, t.Name)
				if i == a.pickerIx {
					lbl.Color = a.palette.Accent
				} else {
					lbl.Color = a.palette.Text
				}
				return a.pickerRows[i].Layout(gtx, lbl.Layout)
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
	pt.Pop()
}

func paintRect(gtx layout.Context, x, y, w, h int, c color.NRGBA) {
	if w <= 0 || h <= 0 {
		return
	}
	stack := clip.Rect(image.Rect(x, y, x+w, y+h)).Push(gtx.Ops)
	paint.Fill(gtx.Ops, c)
	stack.Pop()
}
