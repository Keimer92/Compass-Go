package container

import (
	"fmt"
	"strings"
	"time"

	"compass/internal/console"

	"github.com/gdamore/tcell/v2"
)

// BarHeight es el alto de la barra de pestanas: borde superior, etiqueta y
// borde inferior.
const BarHeight = 3

// tabGap son las columnas vacias entre pestanas.
const tabGap = 1

// Rect es un rectangulo de celdas de pantalla.
type Rect struct {
	X, Y, Width, Height int
}

// Right es la primera columna fuera del rectangulo.
func (r Rect) Right() int { return r.X + r.Width }

// Bottom es la primera fila fuera del rectangulo.
func (r Rect) Bottom() int { return r.Y + r.Height }

// Contains indica si (x,y) cae dentro.
func (r Rect) Contains(x, y int) bool {
	return x >= r.X && x < r.Right() && y >= r.Y && y < r.Bottom()
}

// Pane es una pestana de tarea.
type Pane struct {
	Title   string
	Session *Session
	// Dir es el directorio del proyecto de la pestana; alimenta el acento de
	// color determinista de la pestana (tab_accent, paridad con Rust).
	Dir string
}

// barHit es una zona clicable de la barra. Target -1 es la pestana Compass.
type barHit struct {
	target int
	rect   Rect
}

// closeHit es el boton de cierre de una pestana de tarea.
type closeHit struct {
	index int
	rect  Rect
}

// Container son las pestanas del contenedor. Active < 0 es el menu (Inicio).
type Container struct {
	panes  []Pane
	active int
	// last es la ultima pestana de terminal visitada, para que F12 vuelva.
	last   int
	bar    []barHit
	closes []closeHit

	// prevBtn es el estado del raton en el evento anterior: sin el no se
	// distingue una pulsacion de un arrastre.
	prevBtn tcell.ButtonMask

	// sel es la seleccion de texto de la pestana activa y dragging indica que
	// se esta arrastrando con el boton pulsado.
	sel      Selection
	dragging bool
	// term es el rectangulo del terminal de la pestana activa en coordenadas
	// de pantalla: lo fija DrawBody y lo usa el raton para pasar a celdas.
	term Rect
	// notice es el aviso efimero del pie ("copiado al portapapeles").
	notice   string
	noticeAt time.Time
}

// noticeFor es lo que dura el aviso del pie.
const noticeFor = 2 * time.Second

// New crea un contenedor sin pestanas.
func New() *Container {
	return &Container{active: -1, last: -1}
}

// Active devuelve el indice de la pestana activa, o -1 si se ve el menu.
func (c *Container) Active() int { return c.active }

// TabCount son las pestanas de tarea abiertas (el menu no cuenta).
func (c *Container) TabCount() int { return len(c.panes) }

// RunningCount son las pestanas cuyo proceso sigue vivo.
func (c *Container) RunningCount() int {
	n := 0
	for _, p := range c.panes {
		if !p.Session.Exited() {
			n++
		}
	}
	return n
}

// Open abre una pestana nueva y le da el foco.
func (c *Container) Open(title, script, dir string, rows, cols int) error {
	s, err := Spawn(script, dir, rows, cols)
	if err != nil {
		return err
	}
	console.RegisterPane(s.Pid())
	c.panes = append(c.panes, Pane{Title: title, Session: s, Dir: dir})
	c.active = len(c.panes) - 1
	c.last = c.active
	c.resetSelection()
	// Con pestanas abiertas, conhost no deja cerrar la ventana anfitriona de
	// golpe: la salida pasa por el aviso del menu.
	console.SetOpenTabs(len(c.panes))
	return nil
}

// ShowMenu vuelve al menu sin cerrar ninguna sesion.
func (c *Container) ShowMenu() {
	if c.active >= 0 {
		c.last = c.active
	}
	c.active = -1
	c.resetSelection()
}

// ShowLast vuelve a la ultima pestana de terminal. Devuelve false si no hay.
func (c *Container) ShowLast() bool {
	if c.last >= 0 && c.last < len(c.panes) {
		c.active = c.last
		c.resetSelection()
		return true
	}
	if len(c.panes) == 0 {
		return false
	}
	c.active = len(c.panes) - 1
	c.last = c.active
	c.resetSelection()
	return true
}

// Select cambia de pestana; -1 vuelve al menu.
func (c *Container) Select(target int) {
	if target < 0 {
		c.ShowMenu()
		return
	}
	if target < len(c.panes) {
		c.active = target
		c.last = target
		c.resetSelection()
	}
}

// SelectNumber salta a la pestana n (1-based, como los numeros de la barra).
func (c *Container) SelectNumber(n int) {
	if n >= 1 && n <= len(c.panes) {
		c.Select(n - 1)
	}
}

// Next cicla entre pestanas.
func (c *Container) Next(delta int) {
	if len(c.panes) == 0 {
		c.active = -1
		return
	}
	n := len(c.panes)
	cur := c.active
	if cur < 0 {
		cur = 0
	}
	i := ((cur+delta)%n + n) % n
	c.active = i
	c.last = i
	c.resetSelection()
}

// CloseActive cierra la pestana activa.
func (c *Container) CloseActive() {
	if c.active < 0 {
		return
	}
	c.closePane(c.active)
}

func (c *Container) closePane(i int) {
	if i < 0 || i >= len(c.panes) {
		return
	}
	pane := c.panes[i]
	c.panes = append(c.panes[:i], c.panes[i+1:]...)
	console.UnregisterPane(pane.Session.Pid())
	console.SetOpenTabs(len(c.panes))
	pane.Session.Close()
	c.resetSelection()

	if len(c.panes) == 0 {
		c.active = -1
		c.last = -1
		return
	}
	// Cerrar una pestana que no es la activa no mueve el foco; solo se
	// corrigen los indices cuando quedan por encima de la borrada.
	if c.active == i {
		if i > len(c.panes)-1 {
			c.active = len(c.panes) - 1
		}
	} else if c.active > i {
		c.active--
	}
	if c.last == i {
		c.last = c.active
	} else if c.last > i {
		c.last--
	}
}

// ReapFinished cierra las pestanas cuya tarea termino pidiendo cerrarse.
//
// Se recorre de atras hacia adelante porque closePane reindexa los indices
// mayores. Las pestanas que salen con otro codigo se quedan con su ultima
// pantalla y la marca ✓: son el historial de la tarea, no basura.
func (c *Container) ReapFinished() {
	for i := len(c.panes) - 1; i >= 0; i-- {
		if c.panes[i].Session.CloseRequested() {
			c.closePane(i)
		}
	}
}

// CloseAll cierra todas las pestanas (al salir de Compass).
func (c *Container) CloseAll() {
	for _, p := range c.panes {
		console.UnregisterPane(p.Session.Pid())
		p.Session.Close()
	}
	c.panes = nil
	console.SetOpenTabs(0)
	c.active = -1
	c.last = -1
	c.resetSelection()
}

// Click procesa un clic en la barra. Devuelve true si cayo en una pestana o en
// un boton de cierre.
func (c *Container) Click(col, row int) bool {
	for _, cl := range c.closes {
		if cl.rect.Contains(col, row) {
			c.closePane(cl.index)
			return true
		}
	}
	for _, hit := range c.bar {
		if hit.rect.Contains(col, row) {
			c.Select(hit.target)
			return true
		}
	}
	return false
}

// Pane devuelve la pestana activa, o nil si se ve el menu.
func (c *Container) Pane() *Pane {
	if c.active < 0 || c.active >= len(c.panes) {
		return nil
	}
	return &c.panes[c.active]
}

// Title es el titulo corto de la pestana i (1-based en la barra).
func (c *Container) Title(i int) string {
	if i < 0 || i >= len(c.panes) {
		return ""
	}
	return c.panes[i].Title
}

// shortTitle recorta un titulo a `max` caracteres con elipsis.
func shortTitle(title string, max int) string {
	r := []rune(title)
	if len(r) <= max {
		return title
	}
	if max <= 1 {
		return "…"
	}
	return string(r[:max-1]) + "…"
}

// PaneScript es el guion de la pestana: la misma tarea que ejecutaba la
// ventana externa, mas la propagacion de su codigo de salida.
//
// `& '<exe>' --task …` devuelve 1 o 0, no el codigo del proceso: PowerShell
// solo propaga el codigo de un comando nativo con `exit $LASTEXITCODE`. Sin
// ese cierre, el ExitCloseTab de una cuenta atras se quedaria dentro del
// compass hijo y la pestana nunca se cerraria.
func PaneScript(exe, task, project string) string {
	return fmt.Sprintf("& %s --task %s --path %s; exit $LASTEXITCODE",
		psQuote(exe), task, psQuote(project))
}

// psQuote escapa una cadena para PowerShell duplicando las comillas simples.
func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
