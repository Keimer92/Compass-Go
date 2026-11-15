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

	// theme es el tema con el que se pinta; el selector Ctrl+T lo cambia en
	// vivo igual que en el menu principal. th es el puntero del llamador: se
	// actualiza para que el siguiente Run conserve el tema elegido.
	theme  *theme.Theme
	th     *theme.Theme
	picker *ThemePicker
}

// NewSubmenu crea un submenu con sus entradas.
func NewSubmenu[T comparable](title, section string, entries []SubEntry[T]) *Submenu[T] {
	return &Submenu[T]{
		title:   title,
		section: section,
		entries: entries,
		hover:   -1,
	}
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

// selectPhase devuelve una fase continua basada en el reloj para que la card
// seleccionada del submenu mantenga el mismo halo giratorio que la TUI
// principal (ver MenuState.selectPhase).
func (s *Submenu[T]) selectPhase() (float32, bool) {
	if s.selected < 0 {
		return 0, false
	}
	p := float64(time.Now().UnixNano()) / float64(hoverCycle.Nanoseconds())
	return float32(p - math.Floor(p)), true
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
	s.theme = th
	s.th = th

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
		s.draw(screen, ctx)
		screen.Show()

		var ev tcell.Event
		if s.picker != nil || s.hoverAnimating() || s.selected >= 0 {
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
			// El modal de temas captura el teclado, como en el menu principal.
			if s.picker != nil {
				s.handlePickerKey(e)
				continue
			}
			s.setHover(-1)
			switch e.Key() {
			case tcell.KeyEsc:
				return zero, false, nil
			case tcell.KeyCtrlT:
				orig := *s.theme
				s.picker = &ThemePicker{index: theme.IndexOf(s.theme.Name), original: &orig}
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
			if s.picker != nil {
				continue
			}
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

// handlePickerKey procesa el modal de temas: flechas previsualizan, Enter
// guarda y Esc/Ctrl+T restauran. Igual que en el menu principal.
func (s *Submenu[T]) handlePickerKey(e *tcell.EventKey) {
	switch e.Key() {
	case tcell.KeyEsc, tcell.KeyCtrlT:
		s.applyTheme(s.picker.original)
		s.picker = nil
	case tcell.KeyEnter:
		theme.Save(s.theme)
		s.picker = nil
	case tcell.KeyUp, tcell.KeyLeft:
		s.previewTheme(theme.Prev(s.theme.Name))
	case tcell.KeyDown, tcell.KeyRight:
		s.previewTheme(theme.Next(s.theme.Name))
	}
}

// previewTheme aplica un tema sin guardarlo.
func (s *Submenu[T]) previewTheme(t *theme.Theme) {
	if s.picker != nil {
		s.picker.index = theme.IndexOf(t.Name)
	}
	s.applyTheme(t)
}

// applyTheme cambia el tema activo y lo refleja en el puntero del llamador.
func (s *Submenu[T]) applyTheme(t *theme.Theme) {
	s.theme = t
	if s.th != nil {
		*s.th = *t
	}
}

// draw pinta cabecera, seccion, rejilla con halo, scroll y pie.
func (s *Submenu[T]) draw(screen tcell.Screen, ctx *project.ProjectContext) {
	th := s.theme
	w, h := screen.Size()
	area := Rect{0, 0, w, h}
	fillRect(screen, area, ' ', bgStyle(th))

	ui := layout(area, showAndroidWarning(ctx), false)
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
	renderSubmenuFooter(screen, ui.Footer, th, ctx)

	if s.picker != nil {
		renderThemePicker(screen, area, th, *s.picker)
	}

	s.ui = &ui
}

// renderSection pinta la fila de seccion con la misma forma que la pestana
// activa del menu principal: " <titulo> " en el acento de su seccion.
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
			} else if selected {
				if phase, ok := s.selectPhase(); ok {
					applyHoverGlow(buf, rect, phase, th.CardAccent(item.ColorKey), th.CardBg(item.ColorKey), th.Text)
				}
			}
			buf.blit(screen)
		}
	}
}
