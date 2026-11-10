package container

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
)

// EncodeKey traduce una tecla de tcell a los bytes que espera la consola.
//
// Cubre lo que usan las herramientas del menu (CLIs, flutter run, gradle y una
// shell de PowerShell): texto, control, flechas, navegacion y F1–F12. Las
// secuencias son las estandar de xterm, que es lo que ConPTY entrega.
func EncodeKey(e *tcell.EventKey) []byte {
	alt := e.Modifiers()&tcell.ModAlt != 0
	k := e.Key()

	var b []byte

	switch {
	// Ctrl+A..Ctrl+Z, Ctrl+Space, Ctrl+[ \ ] ^ _.
	case k >= tcell.KeyCtrlSpace && k <= tcell.KeyCtrlUnderscore:
		b = []byte{byte(k - tcell.KeyCtrlSpace)}
	case k == tcell.KeyRune:
		r := e.Rune()
		if r == 0 {
			return nil
		}
		b = []byte(string(r))
	default:
		switch k {
		case tcell.KeyEnter:
			b = []byte{'\r'}
		case tcell.KeyBackspace, tcell.KeyBackspace2:
			b = []byte{0x7f}
		case tcell.KeyTab:
			b = []byte{'\t'}
		case tcell.KeyBacktab:
			b = []byte("\x1b[Z")
		case tcell.KeyEsc:
			b = []byte{0x1b}
		case tcell.KeyUp:
			b = []byte("\x1b[A")
		case tcell.KeyDown:
			b = []byte("\x1b[B")
		case tcell.KeyRight:
			b = []byte("\x1b[C")
		case tcell.KeyLeft:
			b = []byte("\x1b[D")
		case tcell.KeyHome:
			b = []byte("\x1b[H")
		case tcell.KeyEnd:
			b = []byte("\x1b[F")
		case tcell.KeyInsert:
			b = []byte("\x1b[2~")
		case tcell.KeyDelete:
			b = []byte("\x1b[3~")
		case tcell.KeyPgUp:
			b = []byte("\x1b[5~")
		case tcell.KeyPgDn:
			b = []byte("\x1b[6~")
		case tcell.KeyF1:
			b = []byte("\x1bOP")
		case tcell.KeyF2:
			b = []byte("\x1bOQ")
		case tcell.KeyF3:
			b = []byte("\x1bOR")
		case tcell.KeyF4:
			b = []byte("\x1bOS")
		case tcell.KeyF5:
			b = []byte("\x1b[15~")
		case tcell.KeyF6:
			b = []byte("\x1b[17~")
		case tcell.KeyF7:
			b = []byte("\x1b[18~")
		case tcell.KeyF8:
			b = []byte("\x1b[19~")
		case tcell.KeyF9:
			b = []byte("\x1b[20~")
		case tcell.KeyF10:
			b = []byte("\x1b[21~")
		case tcell.KeyF11:
			b = []byte("\x1b[23~")
		case tcell.KeyF12:
			b = []byte("\x1b[24~")
		default:
			return nil
		}
	}

	if alt && (len(b) == 0 || b[0] != 0x1b) {
		b = append([]byte{0x1b}, b...)
	}
	return b
}

// mouseButtonCode traduce un boton de tcell al codigo SGR (0/1/2).
func mouseButtonCode(b tcell.ButtonMask) (int, bool) {
	switch b {
	case tcell.Button1:
		return 0, true
	case tcell.Button2:
		return 1, true
	case tcell.Button3:
		return 2, true
	}
	return 0, false
}

// EncodeMouseSGR codifica un evento de raton como SGR 1006 (CSI < b ; x ; y M|m),
// el formato que ConPTY traduce a MOUSE_EVENT_RECORDs para el hijo.
//
// `prev` es la mascara de botones del evento anterior: sin ella no se puede
// distinguir una pulsacion de un arrastre (tcell informa el estado, no la
// transicion). `top` es la primera fila del area del hijo dentro de la ventana
// (la barra de pestanas ocupa las de arriba). Devuelve nil si el puntero esta
// sobre la barra.
func EncodeMouseSGR(e *tcell.EventMouse, prev tcell.ButtonMask, top int) ([]byte, tcell.ButtonMask) {
	col, row := e.Position()
	btn := e.Buttons()

	if row < top {
		return nil, btn
	}

	code := 0
	release := false
	valid := true

	switch {
	case btn&tcell.WheelUp != 0:
		code = 64
	case btn&tcell.WheelDown != 0:
		code = 65
	case btn&tcell.WheelLeft != 0:
		code = 66
	case btn&tcell.WheelRight != 0:
		code = 67
	case btn != tcell.ButtonNone && btn != prev:
		// Pulsacion (o cambio de boton).
		if c, ok := mouseButtonCode(btn); ok {
			code = c
		} else {
			valid = false
		}
	case btn == tcell.ButtonNone && prev != tcell.ButtonNone:
		// Suelta: el codigo es el del boton que se suelta.
		if c, ok := mouseButtonCode(prev); ok {
			code = c
			release = true
		} else {
			valid = false
		}
	case btn != tcell.ButtonNone:
		// Arrastre con el boton pulsado.
		if c, ok := mouseButtonCode(btn); ok {
			code = c + 32
		} else {
			valid = false
		}
	default:
		// Movimiento sin boton.
		code = 35
	}

	if !valid {
		return nil, btn
	}

	mods := 0
	if e.Modifiers()&tcell.ModShift != 0 {
		mods |= 4
	}
	if e.Modifiers()&tcell.ModAlt != 0 {
		mods |= 8
	}
	if e.Modifiers()&tcell.ModCtrl != 0 {
		mods |= 16
	}

	end := 'M'
	if release {
		end = 'm'
	}
	seq := fmt.Sprintf("\x1b[<%d;%d;%d%c", code+mods, col+1, row-top+1, end)
	return []byte(seq), btn
}
