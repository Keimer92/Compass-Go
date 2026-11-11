package container

import "github.com/gdamore/tcell/v2"

// HandleKey procesa una tecla con una pestana activa.
//
// Las teclas reservadas del contenedor no llegan al hijo: F12 vuelve al menu,
// Ctrl+W cierra la pestana, Ctrl+Tab / Ctrl+Shift+Tab ciclan, Alt+1..9 salta
// a una pestana y Ctrl+Shift+C copia la seleccion. Todo lo demas se codifica y
// se reenvia a la sesion activa.
func (c *Container) HandleKey(e *tcell.EventKey) bool {
	if e.Key() == tcell.KeyF12 {
		c.ShowMenu()
		return true
	}

	// Ctrl+C sigue siendo el interrupt del hijo: solo copia con Shift. Ojo:
	// Windows Terminal se queda con este atajo si lo tiene asignado, por eso
	// la seleccion con el raton ya copia sola al soltar.
	if e.Key() == tcell.KeyCtrlC && e.Modifiers()&tcell.ModShift != 0 {
		c.copySelection()
		return true
	}

	if e.Modifiers()&tcell.ModCtrl != 0 {
		switch e.Key() {
		case tcell.KeyTab:
			c.Next(1)
			return true
		case tcell.KeyBacktab:
			c.Next(-1)
			return true
		case tcell.KeyCtrlW:
			c.CloseActive()
			return true
		}
	}

	if e.Modifiers()&tcell.ModAlt != 0 && e.Key() == tcell.KeyRune {
		r := e.Rune()
		if r >= '1' && r <= '9' {
			c.SelectNumber(int(r - '0'))
			return true
		}
		if r == 'm' || r == 'M' {
			c.ShowMenu()
			return true
		}
	}

	// Lo que se escribe cambia el contenido bajo la seleccion: se descarta.
	c.resetSelection()
	if b := EncodeKey(e); b != nil {
		if p := c.Pane(); p != nil {
			p.Session.Write(b)
		}
	}
	return true
}

// HandleMouse procesa el raton con una pestana activa. La barra superior
// pertenece al contenedor; el resto se reenvia al hijo activo como SGR 1006.
// ConPTY lo traduce a MOUSE_EVENT_RECORDs, asi que los TUIs hijos (firmas,
// Git) tienen el mismo hover y clic que el menu. Un shell que no pidio el modo
// raton los ignora.
//
// Con Shift el raton selecciona texto (y no va al hijo); sin Shift solo
// selecciona cuando el hijo no pidio el raton, que es el caso de una shell
// normal. Al soltar, lo seleccionado se copia al portapapeles.
func (c *Container) HandleMouse(e *tcell.EventMouse) bool {
	x, y := e.Position()
	btn := e.Buttons()
	prev := c.prevBtn
	c.prevBtn = btn

	// El clic se resuelve al soltar, como en la version Rust.
	if btn&tcell.Button1 == 0 && prev&tcell.Button1 != 0 {
		if c.Click(x, y) {
			return true
		}
	}

	p := c.Pane()
	if p == nil {
		return true
	}

	// El boton izquierdo selecciona texto: siempre con Shift y, sin Shift,
	// cuando el hijo no pidio el raton (una shell normal). Lo seleccionado no
	// llega al hijo.
	left := btn&tcell.Button1 != 0 || prev&tcell.Button1 != 0
	if left && (e.Modifiers()&tcell.ModShift != 0 || !c.mouseTracking()) {
		if c.selectMouse(e, prev) {
			return true
		}
	}

	if seq, _ := EncodeMouseSGR(e, prev, BarHeight); seq != nil {
		p.Session.Write(seq)
	}
	return true
}
