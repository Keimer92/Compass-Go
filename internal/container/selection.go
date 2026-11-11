package container

import (
	"strings"
	"time"

	"compass/internal/clipboard"

	"github.com/gdamore/tcell/v2"
	"github.com/hinshun/vt10x"
)

// Selection es el trozo de pantalla elegido con el raton en una pestana.
//
// vt10x solo emula la pantalla visible (no hay scrollback), asi que la
// seleccion nunca sale de lo que se ve.
type Selection struct {
	fromCol, fromRow int
	toCol, toRow     int
	active           bool
}

// Empty indica si no hay nada seleccionado.
func (s Selection) Empty() bool { return !s.active }

// Contains indica si la celda (col,row) esta dentro de la seleccion.
func (s Selection) Contains(col, row int) bool {
	c0, r0, c1, r1 := s.bounds()
	if !s.active || row < r0 || row > r1 {
		return false
	}
	if r0 == r1 {
		return col >= c0 && col <= c1
	}
	switch row {
	case r0:
		return col >= c0
	case r1:
		return col <= c1
	default:
		return true
	}
}

// bounds devuelve la seleccion en orden de lectura (inicio arriba-izquierda).
func (s Selection) bounds() (c0, r0, c1, r1 int) {
	c0, r0, c1, r1 = s.fromCol, s.fromRow, s.toCol, s.toRow
	if r0 > r1 || (r0 == r1 && c0 > c1) {
		c0, r0, c1, r1 = c1, r1, c0, r0
	}
	return c0, r0, c1, r1
}

// Text extrae el texto seleccionado del documento del terminal (historia +
// pantalla viva). Cada linea se recorta por la derecha y se separan con CRLF,
// como al copiar de una consola.
func (s Selection) Text(b *Scrollback) string {
	if !s.active || b == nil {
		return ""
	}
	c0, r0, c1, r1 := s.bounds()
	cols := b.Cols()
	if cols < 1 {
		cols = 1
	}

	var buf strings.Builder
	for row := r0; row <= r1; row++ {
		if row < 0 {
			continue
		}
		from, to := 0, cols-1
		if row == r0 {
			from = c0
		}
		if row == r1 {
			to = c1
		}
		var line strings.Builder
		for col := from; col <= to; col++ {
			if col < 0 {
				continue
			}
			ch := b.Cell(col, row).Char
			if ch == 0 {
				ch = ' '
			}
			line.WriteRune(ch)
		}
		buf.WriteString(strings.TrimRight(line.String(), " "))
		if row < r1 {
			buf.WriteString("\r\n")
		}
	}
	return buf.String()
}

// resetSelection descarta la seleccion y el aviso del pie.
func (c *Container) resetSelection() {
	c.sel = Selection{}
	c.dragging = false
	c.notice = ""
}

// Notice devuelve el aviso del pie mientras siga vigente.
func (c *Container) Notice() string {
	if c.notice == "" || time.Since(c.noticeAt) > noticeFor {
		return ""
	}
	return c.notice
}

// setNotice deja un aviso en el pie durante noticeFor.
func (c *Container) setNotice(msg string) {
	c.notice = msg
	c.noticeAt = time.Now()
}

// cellAt traduce coordenadas de pantalla a celdas del terminal alto, teniendo
// en cuenta el scroll del scrollback. Las filas inferiores son la pantalla
// viva y las superiores, la historia.
func (c *Container) cellAt(x, y int) (int, int, bool) {
	if !c.term.Contains(x, y) {
		return 0, 0, false
	}
	col := x - c.term.X
	localY := y - c.term.Y
	p := c.Pane()
	if p == nil || p.sb == nil {
		return col, localY, true
	}
	docY := p.sb.visibleTop(p.sb.Offset()) + localY
	return col, docY, true
}

// mouseTracking indica si el hijo pidio el raton (modos 1000/1002/1003). Si no
// lo pidio, arrastrar selecciona texto directamente, sin Shift.
func (c *Container) mouseTracking() bool {
	p := c.Pane()
	if p == nil {
		return false
	}
	track := false
	p.Session.WithScreen(func(v vt10x.View) {
		track = v.Mode()&vt10x.ModeMouseMask != 0
	})
	return track
}

// selectMouse actualiza la seleccion con el raton. Devuelve true si el evento
// era de seleccion y por tanto no debe llegar al hijo.
func (c *Container) selectMouse(e *tcell.EventMouse, prev tcell.ButtonMask) bool {
	btn := e.Buttons()
	pressed := btn&tcell.Button1 != 0
	wasPressed := prev&tcell.Button1 != 0

	switch {
	case pressed && !wasPressed:
		col, row, ok := c.cellAt(e.Position())
		if !ok {
			return false
		}
		c.sel = Selection{fromCol: col, fromRow: row, toCol: col, toRow: row, active: true}
		c.dragging = true
		c.notice = ""
		return true
	case pressed && c.dragging:
		if col, row, ok := c.cellAt(e.Position()); ok {
			c.sel.toCol, c.sel.toRow = col, row
		}
		return true
	case !pressed && wasPressed && c.dragging:
		c.dragging = false
		c.copySelection()
		return true
	}
	return false
}

// copySelection copia al portapapeles lo seleccionado. Devuelve cuantos
// caracteres se copiaron.
func (c *Container) copySelection() (int, bool) {
	p := c.Pane()
	if p == nil || p.sb == nil || c.sel.Empty() {
		return 0, false
	}
	text := c.sel.Text(p.sb)
	if strings.TrimSpace(text) == "" {
		c.resetSelection()
		return 0, false
	}
	if err := clipboard.Write(text); err != nil {
		c.setNotice("no se pudo copiar")
		return 0, false
	}
	c.setNotice("copiado al portapapeles")
	return len([]rune(text)), true
}
