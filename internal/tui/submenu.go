package tui

import (
	"fmt"
	"math"
	"time"

	"compass/internal/project"
	"compass/internal/theme"

	"github.com/gdamore/tcell/v2"
)

// SubItem es una entrada de submenu: lo que la card necesita para pintarse.
type SubItem struct {
	Icon        string
	Label       string
	Description string
	// ColorKey es la clave de theme.CardKeys que da el acento de esta card.
	ColorKey string
}

// SubEntry asocia un valor del llamador con su card.
type SubEntry[T comparable] struct {
	Value T
	Item  SubItem
}

// Submenu es el navegador de cards de un submenu (Firmas, Git, Limpieza,
// Entorno). Reusa la rejilla, las cards y el halo del menu principal, y corre
// dentro de la pestana ConPTY de la tarea.
//
// Equivale a tasks/submenu.rs de la version Rust.
type Submenu[T comparable] struct {
	// title es la etiqueta de la fila de seccion, p. ej. "GIT".
	title string
	// section es la seccion del tema ("DEV" | "MNT" | "CFG") que la colorea.
	section string

	entries      []SubEntry[T]
	selected     int
	hover        int
	hoverStarted time.Time
	scroll       int
	vh           int
	cols         int
	ui           *UiLayout
	prevBtn      tcell.ButtonMask

	// warnAndroid reserva la fila de aviso de Android si el SDK no esta
	// configurado. Lo apagan los submenus que no compilan Android.
	warnAndroid bool
}

// NewSubmenu crea un submenu con sus entradas.
func NewSubmenu[T comparable](title, section string, entries []SubEntry[T]) *Submenu[T] {
	return &Submenu[T]{
		title:       title,
		section:     section,
		entries:     entries,
		hover:       -1,
		warnAndroid: true,
	}
}

// WithAndroidWarning activa o desactiva la fila de aviso de Android.
func (s *Submenu[T]) WithAndroidWarning(on bool) *Submenu[T] {
	s.warnAndroid = on
	return s
}

// Len es el numero de entradas.
func (s *Submenu[T]) Len() int { return len(s.entries) }

func (s *Submenu[T]) item(idx int) SubItem { return s.entries[idx].Item }

// setHover actualiza la card bajo el puntero. El halo arranca al entrar en una
// card nueva: moverse dentro de la misma no lo reinicia.
func (s *Submenu[T]) setHover(h int) {
	if s.hover == h {
		return
	}
	s.hover = h
	if h >= 0 {
		s.hoverStarted = time.Now()
	} else {
		s.hoverStarted = time.Time{}
	}
}

func (s *Submenu[T]) hoverPhase() (float32, bool) {
	if s.hover < 0 || s.hoverStarted.IsZero() {
		return 0, false
	}
	p := float32(time.Since(s.hoverStarted).Seconds()) / float32(hoverCycle.Seconds())
	return p - float32(math.Floor(float64(p))), true
}

func (s *Submenu[T]) hoverAnimating() bool {
	return s.hover >= 0 && !s.hoverStarted.IsZero()
}

// ensureVisible ajusta el scroll (en filas) para que la card seleccionada
// quede entera dentro del viewport.
func (s *Submenu[T]) ensureVisible() {
	n := len(s.entries)
	if n == 0 || s.vh < CardH {
		s.scroll = 0
		return
	}
	cols := s.cols
	if cols < 1 {
		cols = 1
	}
	rows := visibleRows(s.vh)
	if rows < 1 {
		rows = 1
	}
	i := s.selected
	if i > n-1 {
		i = n - 1
	}
	row := i / cols
	totalRows := (n + cols - 1) / cols
	max := totalRows - rows
	if max < 0 {
		max = 0
	}
	sc := s.scroll
	if row < sc {
		sc = row
	} else if row >= sc+rows {
		sc = row + 1 - rows
	}
	if sc > max {
		sc = max
	}
	s.scroll = sc
}

// moveSelection da un paso horizontal: una card dentro de la fila.
func (s *Submenu[T]) moveSelection(delta int) {
	max := len(s.entries) - 1
	s.selected += delta
	if s.selected < 0 {
		s.selected = 0
	}
	if s.selected > max {
		s.selected = max
	}
	s.ensureVisible()
}

// moveRow da un paso vertical: una fila completa de la rejilla.
func (s *Submenu[T]) moveRow(delta int) {
	step := s.cols
	if step < 1 {
		step = 1
	}
	max := len(s.entries) - 1
	s.selected += delta * step
	if s.selected < 0 {
		s.selected = 0
	}
	if s.selected > max {
		s.selected = max
	}
	s.ensureVisible()
}

// cardAt traduce una posicion de raton a un indice de entrada.
func (s *Submenu[T]) cardAt(x, y int) (int, bool) {
	if s.ui == nil {
		return 0, false
	}
	return gridIndexAt(*s.ui, s.scroll, len(s.entries), x, y)
}

// Run dibuja el submenu y devuelve el valor elegido. ok=false si el usuario
// pulso Esc (volver al menu principal).
func (s *Submenu[T]) Run(ctx *project.ProjectContext, th *theme.Theme) (T, bool, error) {
	var zero T

	screen, err := tcell.NewScreen()
	if err != nil {
		return zero, false, err
	}
	if err := screen.Init(); err != nil {
		return zero, false, err
	}
	defer screen.Fini()

	screen.EnableMouse()
	screen.SetStyle(tcell.StyleDefault.Background(th.Background).Foreground(th.Text))

	events := make(chan tcell.Event, 32)
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

	for {
		s.draw(screen, ctx, th)
		screen.Show()

		var ev tcell.Event
		if s.hoverAnimating() {
			select {
			case ev = <-events:
			case <-time.After(animFrame):
				continue
			}
		} else {
			ev = <-events
		}
		if ev == nil {
			return zero, false, nil
		}

		switch e := ev.(type) {
		case *tcell.EventResize:
			screen.Sync()
		case *tcell.EventKey:
			s.setHover(-1)
			switch e.Key() {
			case tcell.KeyEsc:
				return zero, false, nil
			case tcell.KeyEnter:
				if s.selected >= 0 && s.selected < len(s.entries) {
					return s.entries[s.selected].Value, true, nil
				}
			case tcell.KeyUp:
				s.moveRow(-1)
			case tcell.KeyDown:
				s.moveRow(1)
			case tcell.KeyLeft:
				s.moveSelection(-1)
			case tcell.KeyRight:
				s.moveSelection(1)
			default:
				switch e.Rune() {
				case 'q', 'Q':
					return zero, false, nil
				case 'k', 'K':
					s.moveRow(-1)
				case 'j', 'J':
					s.moveRow(1)
				case 'h', 'H':
					s.moveSelection(-1)
				case 'l', 'L':
					s.moveSelection(1)
				}
			}
		case *tcell.EventMouse:
			prev := s.prevBtn
			s.handleMouse(e)
			// El clic se resuelve al soltar, como en el menu principal.
			if e.Buttons()&tcell.Button1 == 0 && prev&tcell.Button1 != 0 {
				if idx, ok := s.cardAt(e.Position()); ok {
					s.selected = idx
					return s.entries[idx].Value, true, nil
				}
			}
		}
	}
}

// handleMouse procesa la rueda y el hover.
func (s *Submenu[T]) handleMouse(e *tcell.EventMouse) {
	btn := e.Buttons()
	if btn&tcell.WheelUp != 0 {
		s.moveRow(-1)
	}
	if btn&tcell.WheelDown != 0 {
		s.moveRow(1)
	}
	x, y := e.Position()
	if idx, ok := s.cardAt(x, y); ok {
		s.setHover(idx)
	} else {
		s.setHover(-1)
	}
	s.prevBtn = btn
}

// draw pinta cabecera, seccion, rejilla con halo, scroll y pie.
func (s *Submenu[T]) draw(screen tcell.Screen, ctx *project.ProjectContext, th *theme.Theme) {
	w, h := screen.Size()
	area := Rect{0, 0, w, h}
	fillRect(screen, area, ' ', bgStyle(th))

	ui := layout(area, s.warnAndroid && showAndroidWarning(ctx), false)
	s.vh = ui.List.Height
	s.cols = ui.Cols

	if ui.List.Height < CardH {
		minH := ui.Header.Height + TabsH + FooterH + CardH
		cx := area.X + area.Width/2
		putCenter(screen, cx, area.Y+1, "Terminal demasiado pequena.", bgStyle(th).Foreground(th.Warning))
		putCenter(screen, cx, area.Y+2, fmt.Sprintf("Necesita al menos %d lineas.", minH), bgStyle(th).Foreground(th.Muted))
		s.ui = nil
		return
	}

	renderHeader(screen, ui.Header, ctx, th, false, nil)
	s.renderSection(screen, ui.Tabs, th)

	s.ensureVisible()
	s.renderCards(screen, ui, th)
	cols := ui.Cols
	if cols < 1 {
		cols = 1
	}
	renderScrollbar(screen, ui, (len(s.entries)+cols-1)/cols, s.scroll, th)
	s.renderFooter(screen, ui.Footer, ctx, th)

	s.ui = &ui
}

// renderSection pinta la fila de seccion con la misma forma que las pestanas
// del menu principal: " <titulo> <n> " en el acento de su seccion.
func (s *Submenu[T]) renderSection(screen tcell.Screen, area Rect, th *theme.Theme) {
	text := fmt.Sprintf(" %s %d ", s.title, len(s.entries))
	st := bgStyle(th).Foreground(th.Section(s.section)).Bold(true)
	put(screen, area.X, area.Y, text, st)
}

// renderCards pinta la rejilla de cards con su halo.
func (s *Submenu[T]) renderCards(screen tcell.Screen, ui UiLayout, th *theme.Theme) {
	n := len(s.entries)
	cols := ui.Cols
	cellW := gridCellW(ui.List.Width, cols)
	cardW := gridCardW(ui.List.Width, cols)
	listY := ui.List.Y
	listBottom := ui.List.Bottom()
	totalRows := (n + cols - 1) / cols

	for row := s.scroll; row < totalRows; row++ {
		y := listY + (row-s.scroll)*RowH
		if y+CardH > listBottom {
			break
		}
		for col := 0; col < cols; col++ {
			idx := row*cols + col
			if idx >= n {
				break
			}
			rect := Rect{ui.List.X + col*cellW, y, cardW, CardH}
			item := s.item(idx)
			selected := s.selected == idx
			hovered := s.hover == idx

			buf := newCellBuf(rect)
			drawSubCard(buf, rect, item, selected, hovered, th)
			if hovered {
				if phase, ok := s.hoverPhase(); ok {
					applyHoverGlow(buf, rect, phase, th.CardAccent(item.ColorKey), th.CardBg(item.ColorKey), th.Text)
				}
			}
			buf.blit(screen)
		}
	}
}

// renderFooter pinta los atajos a la izquierda y la ruta a la derecha.
func (s *Submenu[T]) renderFooter(screen tcell.Screen, area Rect, ctx *project.ProjectContext, th *theme.Theme) {
	base := bgStyle(th)
	x := area.X
	seg := func(text string, st tcell.Style) {
		put(screen, x, area.Y, text, st)
		x += unisegWidth(text)
	}
	seg("  ←→↑↓", base.Foreground(th.Text))
	seg(" navegar   ", base.Foreground(th.Muted))
	seg("Enter", base.Foreground(th.Text))
	seg(" ejecutar   ", base.Foreground(th.Muted))
	seg("Esc", base.Foreground(th.Text))
	seg(" volver   ", base.Foreground(th.Muted))

	leftW := x - area.X
	if leftW > area.Width {
		leftW = area.Width
	}

	full := ctx.ScriptDir
	rightW := area.Width - leftW
	shown := full
	if unisegWidth(full) > rightW && rightW >= 2 {
		runes := []rune(full)
		keep := rightW - 1
		if keep < len(runes) {
			shown = "…" + string(runes[len(runes)-keep:])
		}
	}
	shownW := unisegWidth(shown)
	if shownW > area.Width {
		shownW = area.Width
	}
	put(screen, area.Right()-shownW, area.Y, shown, base.Foreground(th.Muted))
}
