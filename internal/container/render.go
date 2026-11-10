package container

import (
	"fmt"

	"compass/internal/shell"
	"compass/internal/theme"

	"github.com/gdamore/tcell/v2"
	"github.com/hinshun/vt10x"
	"github.com/rivo/uniseg"
)

// put dibuja `text` en (x,y) con `style`, avanzando por ancho de celda.
func put(s tcell.Screen, x, y int, text string, style tcell.Style) {
	for _, r := range text {
		w := uniseg.StringWidth(string(r))
		if w == 0 {
			continue
		}
		s.SetContent(x, y, r, nil, style)
		x += w
	}
}

// fillRect rellena un rectangulo con un caracter y estilo.
func fillRect(s tcell.Screen, r Rect, ch rune, style tcell.Style) {
	for y := r.Y; y < r.Bottom(); y++ {
		for x := r.X; x < r.Right(); x++ {
			s.SetContent(x, y, ch, nil, style)
		}
	}
}

// DrawBar pinta la barra superior: una caja redondeada por pestana, con un
// boton × en las de tarea. La pestana del menu se llama "Compass".
func (c *Container) DrawBar(s tcell.Screen, area Rect, th *theme.Theme) {
	c.bar = c.bar[:0]
	c.closes = c.closes[:0]
	if area.Height <= 0 || area.Width <= 0 {
		return
	}

	type entry struct {
		target   int
		label    string
		exited   bool
		closable bool
		// accent es el color de la pestana: th.Brand para el menu, el acento
		// determinista del proyecto para las de tarea (tab_accent).
		accent tcell.Color
	}

	entries := []entry{{-1, "Compass Go", false, false, th.Brand}}
	for i, p := range c.panes {
		label := fmt.Sprintf("%d %s", i+1, shortTitle(p.Title, 20))
		exited := p.Session.Exited()
		if exited {
			label += " ✓"
		}
		entries = append(entries, entry{i, label, exited, true, TabAccent(p.Dir)})
	}

	x := area.X
	for n, e := range entries {
		labelW := uniseg.StringWidth(e.label)
		innerW := labelW + 2
		if e.closable {
			innerW += 2
		}
		w := innerW + 2

		if n > 0 {
			if x+tabGap > area.Right() {
				break
			}
			x += tabGap
		}
		if x+w > area.Right() {
			break
		}

		h := BarHeight
		if h > area.Height {
			h = area.Height
		}
		tab := Rect{x, area.Y, w, h}
		active := e.target == c.active

		borderStyle := tcell.StyleDefault.Foreground(th.Border)
		textStyle := tcell.StyleDefault.Foreground(th.Text)
		switch {
		case active:
			borderStyle = tcell.StyleDefault.Foreground(e.accent).Bold(true)
			textStyle = tcell.StyleDefault.Foreground(e.accent).Bold(true)
		case e.exited:
			borderStyle = tcell.StyleDefault.Foreground(th.Muted)
			textStyle = tcell.StyleDefault.Foreground(th.Muted)
		default:
			// Pestana de tarea inactiva: acento atenuado sobre el fondo para
			// distinguirla de la activa sin perder la identidad del proyecto.
			dim := theme.Mix(e.accent, th.Background, 0.4)
			borderStyle = tcell.StyleDefault.Foreground(dim)
			textStyle = tcell.StyleDefault.Foreground(dim)
		}

		if active {
			fillRect(s, tab, ' ', tcell.StyleDefault.Background(th.HoverBg))
		}

		// Caja redondeada.
		if h >= 1 {
			for bx := tab.X + 1; bx < tab.Right()-1; bx++ {
				s.SetContent(bx, tab.Y, '─', nil, borderStyle)
			}
			s.SetContent(tab.X, tab.Y, '╭', nil, borderStyle)
			s.SetContent(tab.Right()-1, tab.Y, '╮', nil, borderStyle)
		}
		if h >= 3 {
			for bx := tab.X + 1; bx < tab.Right()-1; bx++ {
				s.SetContent(bx, tab.Bottom()-1, '─', nil, borderStyle)
			}
			s.SetContent(tab.X, tab.Bottom()-1, '╰', nil, borderStyle)
			s.SetContent(tab.Right()-1, tab.Bottom()-1, '╯', nil, borderStyle)
		}
		if h >= 2 {
			mid := tab.Y + 1
			s.SetContent(tab.X, mid, '│', nil, borderStyle)
			s.SetContent(tab.Right()-1, mid, '│', nil, borderStyle)
			put(s, tab.X+1, mid, " "+e.label+" ", textStyle)
			if e.closable {
				closeStyle := textStyle
				if !active {
					closeStyle = tcell.StyleDefault.Foreground(th.Muted)
				}
				put(s, tab.X+3+labelW, mid, "× ", closeStyle)
				// El × va tras " etiqueta ": borde + hueco + etiqueta + hueco.
				// La zona clicable abarca los dos huecos y el glifo.
				c.closes = append(c.closes, closeHit{
					index: e.target,
					rect:  Rect{tab.X + 2 + labelW, tab.Y, 3, tab.Height},
				})
			}
		}

		c.bar = append(c.bar, barHit{target: e.target, rect: tab})
		x += w
	}
}

// DrawBody pinta la sesion activa y su pie.
func (c *Container) DrawBody(s tcell.Screen, body Rect, th *theme.Theme) {
	p := c.Pane()
	if p == nil || body.Height <= 0 || body.Width <= 0 {
		return
	}

	termH := body.Height - 1
	if termH > 0 {
		term := Rect{body.X, body.Y, body.Width, termH}
		p.Session.Resize(term.Width, term.Height)
		p.Session.WithScreen(func(v vt10x.View) {
			renderView(s, term, v, th)
		})
	}

	if termH+1 <= body.Height {
		footer := Rect{body.X, body.Y + termH, body.Width, 1}
		renderPaneFooter(s, footer, th, p.Session.Exited())
	}
}

// renderView vuelca la pantalla parseada en el area del terminal.
func renderView(s tcell.Screen, area Rect, v vt10x.View, th *theme.Theme) {
	cols, rows := v.Size()
	defFG, defBG := th.Text, th.Background

	for y := 0; y < area.Height && y < rows; y++ {
		for x := 0; x < area.Width && x < cols; x++ {
			g := v.Cell(x, y)
			ch := g.Char
			if ch == 0 {
				ch = ' '
			}
			s.SetContent(area.X+x, area.Y+y, ch, nil, styleOf(g, defFG, defBG))
			// Una runa ancha ocupa dos celdas: la siguiente la cubre tcell.
			if uniseg.StringWidth(string(ch)) == 2 {
				x++
			}
		}
	}

	if !v.CursorVisible() {
		return
	}
	cur := v.Cursor()
	if cur.X < 0 || cur.X >= area.Width || cur.Y < 0 || cur.Y >= area.Height {
		return
	}
	g := v.Cell(cur.X, cur.Y)
	ch := g.Char
	if ch == 0 {
		ch = ' '
	}
	st := tcell.StyleDefault.
		Foreground(colorOf(g.BG, defBG)).
		Background(colorOf(g.FG, defFG))
	s.SetContent(area.X+cur.X, area.Y+cur.Y, ch, nil, st)
}

// renderPaneFooter pinta la ayuda de teclas y el estado del proceso.
func renderPaneFooter(s tcell.Screen, r Rect, th *theme.Theme, exited bool) {
	base := tcell.StyleDefault.Background(th.Background)
	text := base.Foreground(th.Text)
	muted := base.Foreground(th.Muted)

	x := r.X
	seg := func(t string, st tcell.Style) {
		put(s, x, r.Y, t, st)
		x += uniseg.StringWidth(t)
	}

	seg("  F12", text)
	seg(" menu  ", muted)
	seg("Ctrl+W", text)
	seg(" cerrar  ", muted)
	seg("Ctrl+Tab", text)
	seg(" siguiente  ", muted)
	seg("Alt+1..9", text)
	seg(" pestana  ", muted)

	if exited {
		seg("proceso finalizado", base.Foreground(th.Warning))
	} else {
		seg("en ejecucion", base.Foreground(th.Success))
	}

	right := shell.Shell() + " "
	rw := uniseg.StringWidth(right)
	if rw > 0 && r.X+r.Width-rw > x {
		put(s, r.X+r.Width-rw, r.Y, right, muted)
	}
}
