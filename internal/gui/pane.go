package gui

import (
	"image"

	"compass/internal/container"

	"gioui.org/layout"
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

// reapFinished cierra las pestanas que pidieron cerrarse y marca el resto como
// terminadas (✓) para conservar su ultima pantalla como historial.
func (a *Application) reapFinished() {
	for i := len(a.panes) - 1; i >= 0; i-- {
		p := a.panes[i]
		if p.Session.CloseRequested() {
			a.closePane(i)
			continue
		}
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

	lblText := pane.Title
	if pane.Done {
		lblText += " ✓"
	}
	lbl := material.Caption(a.th, lblText)
	if active {
		lbl.Color = p.Text
	} else {
		lbl.Color = p.Muted
	}

	return layout.Inset{Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return pane.tabBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					dims := lbl.Layout(gtx)
					if active {
						bar := image.Rect(0, dims.Size.Y-1, dims.Size.X, dims.Size.Y)
						s := clip.Rect(bar).Push(gtx.Ops)
						paint.Fill(gtx.Ops, p.Brand)
						s.Pop()
					}
					return dims
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				x := material.Caption(a.th, "×")
				x.Color = p.Muted
				return pane.closeBtn.Layout(gtx, x.Layout)
			}),
		)
	})
}

