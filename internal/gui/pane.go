package gui

import (
	"image"

	"compass/internal/container"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// selRange es una seleccion de celdas (inclusive) en la rejilla del terminal.
type selRange struct {
	active           bool
	fromX, fromY     int
	toX, toY         int
}

func (s selRange) contains(x, y int) bool {
	if !s.active {
		return false
	}
	c0, r0, c1, r1 := s.bounds()
	if y < r0 || y > r1 {
		return false
	}
	switch {
	case r0 == r1:
		return x >= c0 && x <= c1
	case y == r0:
		return x >= c0
	case y == r1:
		return x <= c1
	default:
		return true
	}
}

func (s selRange) bounds() (c0, r0, c1, r1 int) {
	c0, r0, c1, r1 = s.fromX, s.fromY, s.toX, s.toY
	if r0 > r1 || (r0 == r1 && c0 > c1) {
		c0, r0, c1, r1 = c1, r1, c0, r0
	}
	return
}

// Pane es una pestana de tarea: su ConPTY, su historia y su estado de seleccion.
type Pane struct {
	Title   string
	Task    string
	Session *container.Session
	Back    *container.Scrollback
	Term    *Terminal
	Sel     selRange
	Done    bool

	tag      byte
	tabBtn   widget.Clickable
	closeBtn widget.Clickable
}

var paneCounter byte

// newPane crea una pestana con su ConPTY y su scrollback.
func (a *Application) newPane(title, task, script, dir string) (*Pane, error) {
	cols, rows := 120, 36
	back := container.NewScrollback(cols, rows)
	sess, err := container.Spawn(script, dir, rows, cols, back)
	if err != nil {
		return nil, err
	}
	paneCounter++
	return &Pane{
		Title:   title,
		Task:    task,
		Session: sess,
		Back:    back,
		Term:    NewTerminal(&a.palette),
		tag:     paneCounter + 100,
	}, nil
}

func (a *Application) closePane(i int) {
	if i < 0 || i >= len(a.panes) {
		return
	}
	p := a.panes[i]
	p.Session.Close()
	a.panes = append(a.panes[:i], a.panes[i+1:]...)
	if len(a.panes) == 0 {
		a.activePane = -1
		return
	}
	if a.activePane >= len(a.panes) {
		a.activePane = len(a.panes) - 1
	}
}

// reapFinished marca las pestanas terminadas (✓) y conserva su ultima pantalla.
// No las cierra automaticamente: asi el usuario puede leer la salida de un
// dev server que murio rapido y cerrarlas con la X cuando quiera.
func (a *Application) reapFinished() {
	for _, p := range a.panes {
		if p.Session.Exited() {
			p.Done = true
		}
	}
}

// layoutTabBar dibuja la barra de pestanas de tareas sobre el terminal.
func (a *Application) layoutTabBar(gtx layout.Context, p Palette, width, height int) {
	stack := clip.Rect(image.Rect(0, 0, width, height)).Push(gtx.Ops)
	paint.Fill(gtx.Ops, blend(p.Background, p.Border, 0.5))
	stack.Pop()

	bar := gtx
	bar.Constraints = layout.Exact(image.Pt(width, height))
	layout.Inset{Left: unit.Dp(8), Top: unit.Dp(4)}.Layout(bar, func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, 0, len(a.panes)*2)
		for i := range a.panes {
			i := i
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.tabButtonWidget(gtx, p, i)
			}))
		}
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
	})
}

func (a *Application) tabButtonWidget(gtx layout.Context, p Palette, i int) layout.Dimensions {
	pane := a.panes[i]
	active := i == a.activePane

	lbl := material.Caption(a.th, pane.Title)
	if active {
		lbl.Color = p.Text
	} else {
		lbl.Color = p.Muted
	}

	return layout.Inset{Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return pane.tabBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			m := op.Record(gtx.Ops)
			dims := layout.Inset{
				Left: unit.Dp(12), Right: unit.Dp(8),
				Top: unit.Dp(5), Bottom: unit.Dp(5),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Rigid(lbl.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return tabCloseButton(gtx, a.th, p, &pane.closeBtn)
						})
					}),
				)
			})
			call := m.Stop()

			bg := blend(p.Background, p.Border, 0.25)
			if active {
				bg = blend(p.Background, p.Accent, 0.28)
			} else if pane.tabBtn.Hovered() {
				bg = blend(p.Background, p.Accent, 0.15)
			}
			rad := gtx.Dp(unit.Dp(9))
			stack := clip.RRect{
				Rect: image.Rectangle{Max: dims.Size},
				SE: rad, SW: rad, NE: rad, NW: rad,
			}.Push(gtx.Ops)
			paint.Fill(gtx.Ops, bg)
			stack.Pop()
			call.Add(gtx.Ops)
			return dims
		})
	})
}

