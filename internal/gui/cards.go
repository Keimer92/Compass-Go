package gui

import (
	"image"
	"image/color"
	"unicode"

	"compass/internal/project"
	"compass/internal/tui"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// allMenuItems es el equivalente del arreglo de items del menu de la TUI.
// Se mantiene aqui para no depender del estado interno de internal/tui.
var allMenuItems = []tui.MenuItem{
	tui.WindowsDev, tui.AndroidDev, tui.AndroidHmr, tui.WindowsBuild,
	tui.AndroidBuildArm64, tui.AndroidBuildArmv7, tui.AndroidBuildAll,
	tui.InstallDebug, tui.FlutterProfile, tui.Tests,
	tui.Clean, tui.DebugAndroid, tui.Signing, tui.VersionItem,
	tui.Git, tui.BuildWhatsApp, tui.ConfigureTerminal, tui.Environment,
}

// CardGrid es la rejilla de cards de una pestana del menu.
type CardGrid struct {
	pt       project.ProjectType
	items    []tui.MenuItem
	clicks   []widget.Clickable
	selected int
	tab      tui.Tab
	cols     int

	onActivate func(tui.MenuItem)
}

// NewCardGrid crea la rejilla con los items visibles para el tipo de proyecto.
func NewCardGrid(pt project.ProjectType, onActivate func(tui.MenuItem)) *CardGrid {
	g := &CardGrid{pt: pt, onActivate: onActivate}
	g.SetTab(tui.TabDevMnt)
	return g
}

// SetTab cambia de pestana y recalcula los items visibles.
func (g *CardGrid) SetTab(tab tui.Tab) {
	g.tab = tab
	g.items = nil
	for _, item := range allMenuItems {
		if item.Tab() != tab {
			continue
		}
		if item.Label(g.pt) == "" {
			continue
		}
		g.items = append(g.items, item)
	}
	g.clicks = make([]widget.Clickable, len(g.items))
	g.selected = 0
}

// Move mueve la seleccion con flechas (dx/dy en celdas de la rejilla).
func (g *CardGrid) Move(dx, dy int) {
	n := len(g.items)
	if n == 0 {
		return
	}
	if g.cols < 1 {
		g.cols = 3
	}
	i := g.selected
	switch {
	case dx != 0:
		i += dx
	case dy < 0:
		i -= g.cols
	case dy > 0:
		i += g.cols
	}
	if i < 0 {
		i = 0
	}
	if i >= n {
		i = n - 1
	}
	g.selected = i
}

// Activate ejecuta la card seleccionada.
func (g *CardGrid) Activate() {
	if g.selected >= 0 && g.selected < len(g.items) && g.onActivate != nil {
		g.onActivate(g.items[g.selected])
	}
}

// SelectedItem devuelve la card seleccionada (o 0 si no hay).
func (g *CardGrid) SelectedItem() tui.MenuItem {
	if g.selected >= 0 && g.selected < len(g.items) {
		return g.items[g.selected]
	}
	return 0
}

// Layout dibuja la rejilla dentro de las constraints de gtx.
func (g *CardGrid) Layout(gtx layout.Context, p Palette, th *material.Theme) layout.Dimensions {
	avail := gtx.Constraints.Max
	cols := avail.X / gtx.Dp(unit.Dp(250))
	if cols < 2 {
		cols = 2
	}
	if cols > 4 {
		cols = 4
	}
	g.cols = cols

	gap := gtx.Dp(unit.Dp(14))
	cardW := (avail.X - gap*(cols-1)) / cols
	cardH := gtx.Dp(unit.Dp(84))

	for i := range g.items {
		if g.clicks[i].Clicked(gtx) {
			g.selected = i
			g.Activate()
		}
	}

	for i, item := range g.items {
		col, row := i%cols, i/cols
		x := col * (cardW + gap)
		y := row * (cardH + gap)
		g.layoutCard(gtx, p, th, item, i,
			image.Rect(x, y, x+cardW, y+cardH))
	}
	rows := (len(g.items) + cols - 1) / cols
	return layout.Dimensions{Size: image.Pt(avail.X, rows*(cardH+gap))}
}

func (g *CardGrid) layoutCard(gtx layout.Context, p Palette, th *material.Theme,
	item tui.MenuItem, idx int, r image.Rectangle) {

	key := item.ColorKey()
	accent := p.Cards[0]
	for i, k := range themeCardKeys {
		if k == key && i < len(p.Cards) {
			accent = p.Cards[i]
			break
		}
	}

	selected := idx == g.selected
	hovered := g.clicks[idx].Hovered()
	bg := blend(p.Background, p.Border, 0.28)
	if hovered && !selected {
		bg = blend(bg, accent, 0.22)
	}
	if selected {
		pct := 27
		if hovered {
			pct = 40
		}
		bg = dim(accent, pct)
	}
	rad := gtx.Dp(unit.Dp(9))
	stack := clip.RRect{
		Rect: r, SE: rad, SW: rad, NE: rad, NW: rad,
	}.Push(gtx.Ops)
	paint.Fill(gtx.Ops, bg)
	stack.Pop()

	// Barra de acento a la izquierda.
	bar := image.Rect(r.Min.X, r.Min.Y, r.Min.X+gtx.Dp(unit.Dp(4)), r.Max.Y)
	stack = clip.Rect(bar).Push(gtx.Ops)
	paint.Fill(gtx.Ops, accent)
	stack.Pop()

	// Cuadrado con la inicial de la card (los emoji de Icon() aun no se pintan
	// en Gio; se retomara cuando se resuelva el fallback de color glyphs).
	label := item.Label(g.pt)
	badge := gtx.Dp(unit.Dp(34))
	br := image.Rect(r.Min.X+gtx.Dp(unit.Dp(16)), r.Min.Y+gtx.Dp(unit.Dp(18)),
		r.Min.X+gtx.Dp(unit.Dp(16))+badge, r.Min.Y+gtx.Dp(unit.Dp(18))+badge)
	stack = clip.Rect(br).Push(gtx.Ops)
	paint.Fill(gtx.Ops, dim(accent, 55))
	stack.Pop()

	badgeGtx := gtx
	badgeGtx.Constraints = layout.Exact(br.Size())
	btrans := op.Offset(br.Min).Push(badgeGtx.Ops)
	drawCentered(badgeGtx, th, string(firstRune(label)), accent)
	btrans.Pop()

	// Texto: titulo + descripcion.
	txtGtx := gtx
	tx := r.Min.X + gtx.Dp(unit.Dp(62))
	txtGtx.Constraints = layout.Exact(image.Pt(r.Max.X - tx - gtx.Dp(unit.Dp(10)), r.Dy()))
	trans := op.Offset(image.Pt(tx, r.Min.Y+gtx.Dp(unit.Dp(12)))).Push(txtGtx.Ops)
	layout.Flex{Axis: layout.Vertical}.Layout(txtGtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Body1(th, label)
			lbl.Color = p.Text
			return lbl.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Caption(th, item.Description(g.pt))
			lbl.Color = p.Muted
			return lbl.Layout(gtx)
		}),
	)
	trans.Pop()

	//Area clicable por encima de todo.
	clickGtx := gtx
	clickGtx.Constraints = layout.Exact(r.Size())
	ct := op.Offset(r.Min).Push(clickGtx.Ops)
	g.clicks[idx].Layout(clickGtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: r.Size()}
	})
	ct.Pop()
}

// themeCardKeys es theme.CardKeys copiado aqui para no importar tcell desde el
// widget (la GUI solo necesita las claves, no los colores tcell).
var themeCardKeys = [18]string{
	"run", "android", "reload", "build", "apk", "apk32", "aab", "install", "profile", "test",
	"clean", "debug", "sign", "version", "git", "whatsapp", "terminal", "environment",
}

func firstRune(s string) string {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return string(r)
		}
	}
	return "?"
}

// drawCentered dibuja un texto centrado en las constraints actuales.
func drawCentered(gtx layout.Context, th *material.Theme, txt string, col color.NRGBA) {
	lbl := material.Body1(th, txt)
	lbl.Color = col
	lbl.Alignment = text.Middle
	lbl.Layout(gtx)
}
