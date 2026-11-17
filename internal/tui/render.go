package tui

import (
	"fmt"
	"strings"

	"compass/internal/project"
	"compass/internal/theme"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"
)

// unisegWidth mide el ancho en celdas de una cadena (emojis incluidos).
func unisegWidth(s string) int { return uniseg.StringWidth(s) }

// runeWidth mide el ancho en celdas de una runa.
func runeWidth(r rune) int { return uniseg.StringWidth(string(r)) }

// put dibuja `text` en (x,y) con `style`, avanzando por ancho de celda.
func put(s cellSink, x, y int, text string, style tcell.Style) {
	for _, r := range text {
		rw := runeWidth(r)
		if rw == 0 {
			continue
		}
		s.SetContent(x, y, r, nil, style)
		x += rw
	}
}

func screenW(s tcell.Screen) int { w, _ := s.Size(); return w }
func screenH(s tcell.Screen) int { _, h := s.Size(); return h }

// fillRect rellena un rectangulo con un caracter y estilo.
func fillRect(s cellSink, r Rect, ch rune, style tcell.Style) {
	for y := r.Y; y < r.Bottom(); y++ {
		for x := r.X; x < r.Right(); x++ {
			s.SetContent(x, y, ch, nil, style)
		}
	}
}

// drawBox dibuja un borde redondeado con `style`.
func drawBox(s cellSink, r Rect, style tcell.Style) {
	if r.Width < 2 || r.Height < 2 {
		return
	}
	// Horizontales.
	for x := r.X + 1; x < r.Right()-1; x++ {
		s.SetContent(x, r.Y, '─', nil, style)
		s.SetContent(x, r.Bottom()-1, '─', nil, style)
	}
	// Verticales.
	for y := r.Y + 1; y < r.Bottom()-1; y++ {
		s.SetContent(r.X, y, '│', nil, style)
		s.SetContent(r.Right()-1, y, '│', nil, style)
	}
	// Esquinas.
	s.SetContent(r.X, r.Y, '╭', nil, style)
	s.SetContent(r.Right()-1, r.Y, '╮', nil, style)
	s.SetContent(r.X, r.Bottom()-1, '╰', nil, style)
	s.SetContent(r.Right()-1, r.Bottom()-1, '╯', nil, style)
}

// bgStyle es el estilo base con el fondo del tema.
func bgStyle(th *theme.Theme) tcell.Style {
	return tcell.StyleDefault.Background(th.Background)
}

// renderMenu dibuja el menu completo dentro de `area`.
func renderMenu(s tcell.Screen, st *MenuState, ctx *project.ProjectContext, area Rect) {
	th := st.theme

	// Fondo de toda el area con el background del tema.
	fillRect(s, area, ' ', bgStyle(th))

	warn := showAndroidWarning(ctx)
	ui := layout(area, warn, st.opencodeReminder)
	st.vh = ui.List.Height
	st.cols = ui.Cols

	if ui.List.Height < CardH {
		minH := ui.Header.Height + TabsH + FooterH + CardH
		msgStyle := bgStyle(th).Foreground(th.Warning)
		subStyle := bgStyle(th).Foreground(th.Muted)
		cx := area.X + area.Width/2
		putCenter(s, cx, area.Y+1, "Terminal demasiado pequena.", msgStyle)
		putCenter(s, cx, area.Y+2, fmt.Sprintf("Necesita al menos %d lineas.", minH), subStyle)
		st.ui = nil
		return
	}

	renderHeader(s, ui.Header, ctx, th, st.opencodeReminder, ui.ReminderButton)
	renderTabs(s, ui.Tabs, st, th)

	st.ensureVisible()
	renderCards(s, ui, st, ctx.ProjectType, th)
	renderScrollbar(s, ui, st.totalRowsFor(ui), st.scroll, th)
	renderFooter(s, ui.Footer, th, ctx)

	// Modales por encima.
	if st.picker != nil {
		renderThemePicker(s, area, th, *st.picker)
	}
	if st.settings != nil {
		renderSettingsModal(s, area, th, *st.settings)
	}
	if st.quit != nil {
		renderQuitConfirm(s, area, th, *st.quit)
	}

	st.ui = &ui
}

func putCenter(s tcell.Screen, cx, y int, text string, style tcell.Style) {
	w := unisegWidth(text)
	put(s, cx-w/2, y, text, style)
}

// showAndroidWarning indica si se debe mostrar el aviso de ANDROID_HOME.
func showAndroidWarning(ctx *project.ProjectContext) bool {
	return ctx.AndroidHome == "" && ctx.ProjectType != project.TypePython
}

func renderTabs(s tcell.Screen, area Rect, st *MenuState, th *theme.Theme) {
	titles := st.tabTitles()
	var active tcell.Color
	if st.tab == TabDevMnt {
		active = th.Dev
	} else {
		active = th.Cfg
	}
	base := bgStyle(th)

	x := area.X
	put(s, x, area.Y, titles[0], tabStyle(base, th, st.tab == TabDevMnt, active))
	x += unisegWidth(titles[0])
	put(s, x, area.Y, " │ ", base.Foreground(th.Muted))
	x += 3
	put(s, x, area.Y, titles[1], tabStyle(base, th, st.tab == TabCfg, active))
}

func tabStyle(base tcell.Style, th *theme.Theme, active bool, accent tcell.Color) tcell.Style {
	if active {
		return base.Foreground(accent).Bold(true)
	}
	return base.Foreground(th.Muted)
}

func renderCards(s tcell.Screen, ui UiLayout, st *MenuState, pt project.ProjectType, th *theme.Theme) {
	if groups, cols, ok := groupedLayoutFor(ui, st.tab, st.groups); ok {
		renderGroupedCards(s, ui, st, pt, th, groups, cols)
		return
	}
	n := st.tabLen()
	start := st.tabRange().start
	cols := ui.Cols
	cellW := gridCellW(ui.List.Width, cols)
	cardW := gridCardW(ui.List.Width, cols)
	listY := ui.List.Y
	listBottom := ui.List.Bottom()
	totalRows := (n + cols - 1) / cols

	for row := st.scroll; row < totalRows; row++ {
		y := listY + (row-st.scroll)*RowH
		if y+CardH > listBottom {
			break
		}
		for col := 0; col < cols; col++ {
			local := row*cols + col
			if local >= n {
				break
			}
			idx := start + local
			rect := Rect{ui.List.X + col*cellW, y, cardW, CardH}
			selected := st.selected == idx
			hovered := st.hover == idx

			// La card se pinta en un lienzo aparte para poder aplicar el halo
			// encima: tcell no deja leer una celda ya pintada.
			buf := newCellBuf(rect)
			drawCard(buf, rect, st.items[idx], selected, hovered, pt, th)
			if hovered {
				if phase, ok := st.hoverPhase(); ok {
					key := st.items[idx].ColorKey()
					applyHoverGlow(buf, rect, phase, th.CardAccent(key), th.CardBg(key), th.Text)
				}
			} else if selected {
				if phase, ok := st.selectPhase(); ok {
					key := st.items[idx].ColorKey()
					applyHoverGlow(buf, rect, phase, th.CardAccent(key), th.CardBg(key), th.Text)
				}
			}
			buf.blit(s)
		}
	}
}

// renderGroupedCards pinta una columna por grupo (Windows, Android, comun),
// con las cards apiladas de arriba abajo dentro de su columna.
func renderGroupedCards(s tcell.Screen, ui UiLayout, st *MenuState, pt project.ProjectType, th *theme.Theme, groups []menuGroup, cols int) {
	cellW := gridCellW(ui.List.Width, cols)
	cardW := gridCardW(ui.List.Width, cols)
	listY := ui.List.Y
	listBottom := ui.List.Bottom()

	for gi, g := range groups {
		for row := st.scroll; row < g.end-g.start; row++ {
			y := listY + (row-st.scroll)*RowH
			if y+CardH > listBottom {
				break
			}
			idx := g.start + row
			rect := Rect{ui.List.X + gi*cellW, y, cardW, CardH}
			selected := st.selected == idx
			hovered := st.hover == idx

			buf := newCellBuf(rect)
			drawCard(buf, rect, st.items[idx], selected, hovered, pt, th)
			if hovered {
				if phase, ok := st.hoverPhase(); ok {
					key := st.items[idx].ColorKey()
					applyHoverGlow(buf, rect, phase, th.CardAccent(key), th.CardBg(key), th.Text)
				}
			} else if selected {
				if phase, ok := st.selectPhase(); ok {
					key := st.items[idx].ColorKey()
					applyHoverGlow(buf, rect, phase, th.CardAccent(key), th.CardBg(key), th.Text)
				}
			}
			buf.blit(s)
		}
	}
}

// drawCard dibuja una card de tres lineas con su acento propio.
func drawCard(s cellSink, r Rect, item MenuItem, selected, hovered bool, pt project.ProjectType, th *theme.Theme) {
	drawCardParts(s, r, item.Icon(), item.Label(pt), item.Description(pt), item.ColorKey(), selected, hovered, th)
}

// drawSubCard dibuja una card de submenu.
func drawSubCard(s cellSink, r Rect, item SubItem, selected, hovered bool, th *theme.Theme) {
	drawCardParts(s, r, item.Icon, item.Label, item.Description, item.ColorKey, selected, hovered, th)
}

// drawCardParts pinta una card: fondo, borde redondeado y contenido con la
// etiqueta a la izquierda y la descripcion pegada al borde derecho.
func drawCardParts(s cellSink, r Rect, icon, title, desc, key string, selected, hovered bool, th *theme.Theme) {
	if r.Width < 2 || r.Height < 2 {
		return
	}
	accent := th.CardAccent(key)

	// Fondo de la card segun estado.
	base := bgStyle(th)
	if selected {
		base = base.Background(th.CardBg(key))
	} else if hovered {
		base = base.Background(th.HoverBg)
	}
	fillRect(s, r, ' ', base)

	// Borde.
	var border tcell.Style
	if selected {
		border = base.Foreground(accent).Bold(true)
	} else if hovered {
		border = base.Foreground(th.Text).Bold(true)
	} else {
		border = base.Foreground(accent)
	}
	drawBox(s, r, border)

	// Contenido: etiqueta a la izquierda, descripcion pegada al borde derecho.
	label := " " + icon + "  " + title
	labelStyle := base.Foreground(accent)
	if selected || hovered {
		labelStyle = labelStyle.Bold(true)
	}
	innerW := r.Width - 2
	y := r.Y + 1

	if desc == "" {
		put(s, r.X+1, y, label, labelStyle)
		return
	}

	labelW := unisegWidth(label)
	available := innerW - labelW
	room := available - 2
	if room < 3 {
		put(s, r.X+1, y, label, labelStyle)
		return
	}

	shown := desc
	if unisegWidth(desc) > room {
		runes := []rune(desc)
		if len(runes) > room-1 {
			runes = runes[:room-1]
		}
		shown = string(runes) + "…"
	}
	shownW := unisegWidth(shown)
	pad := available - (shownW + 1)
	if pad < 1 {
		pad = 1
	}

	descStyle := base.Foreground(th.Muted)
	if hovered && !selected {
		descStyle = base.Foreground(th.Text)
	}

	x := r.X + 1
	put(s, x, y, label, labelStyle)
	x += labelW
	put(s, x, y, strings.Repeat(" ", pad), base)
	x += pad
	put(s, x, y, shown, descStyle)
	x += shownW
	put(s, x, y, " ", base)
}

func renderScrollbar(s tcell.Screen, ui UiLayout, rows, scroll int, th *theme.Theme) {
	if ui.Scrollbar.Width == 0 || rows < 1 {
		return
	}
	content := rows * RowH
	if content <= ui.List.Height {
		return
	}
	trackLen := ui.List.Height
	if trackLen <= 0 {
		return
	}
	viewport := ui.List.Height

	thumbLen := trackLen * viewport / content
	if thumbLen < 1 {
		thumbLen = 1
	}
	if thumbLen > trackLen {
		thumbLen = trackLen
	}
	denom := content - viewport
	thumbPos := 0
	if denom > 0 {
		thumbPos = (scroll * RowH) * (trackLen - thumbLen) / denom
	}
	if thumbPos < 0 {
		thumbPos = 0
	}

	base := bgStyle(th)
	trackStyle := base.Foreground(th.Border)
	thumbStyle := base.Foreground(th.Brand)
	for i := 0; i < trackLen; i++ {
		y := ui.List.Y + i
		if i >= thumbPos && i < thumbPos+thumbLen {
			s.SetContent(ui.Scrollbar.X, y, '█', nil, thumbStyle)
		} else {
			s.SetContent(ui.Scrollbar.X, y, '│', nil, trackStyle)
		}
	}
}

func renderHeader(s tcell.Screen, area Rect, ctx *project.ProjectContext, th *theme.Theme, reminder bool, reminderBtn *Rect) {
	base := bgStyle(th)

	name := "Proyecto"
	if ctx.ScriptDir != "" {
		name = baseName(ctx.ScriptDir)
	}

	var emoji string
	switch ctx.ProjectType {
	case project.TypeFlutter:
		emoji = "\U0001F3AF"
	case project.TypeKotlin:
		emoji = "\U0001F537"
	case project.TypeGo:
		emoji = "\U0001F4E6"
	case project.TypeGoWails:
		emoji = "\U0001F408"
	case project.TypePython:
		emoji = "\U0001F40D"
	}
	var badge string
	var badgeColor tcell.Color
	switch ctx.ProjectType {
	case project.TypeFlutter:
		badge, badgeColor = "FLUTTER", th.Info
	case project.TypeKotlin:
		badge, badgeColor = "KOTLIN", th.Accent
	case project.TypeGo:
		badge, badgeColor = "GO", th.Info
	case project.TypeGoWails:
		badge, badgeColor = "GO", th.Info
	case project.TypePython:
		badge, badgeColor = "PYTHON", th.Accent
	}

	// Linea 1: Compass + version.
	x := area.X
	put(s, x, area.Y, "  "+emoji+"  Compass  v", base.Foreground(th.Brand).Bold(true))
	x += unisegWidth("  " + emoji + "  Compass  v")
	put(s, x, area.Y, Version, base.Foreground(th.Muted))

	// Linea 2: Proyecto + badge.
	x = area.X
	put(s, x, area.Y+1, "     Proyecto: ", base.Foreground(th.Muted))
	x += unisegWidth("     Proyecto: ")
	put(s, x, area.Y+1, name, base.Foreground(th.Text))
	x += unisegWidth(name)
	put(s, x, area.Y+1, "  ", base)
	x += 2
	badgeStyle := base.Background(badgeColor).Foreground(tcell.ColorBlack).Bold(true)
	put(s, x, area.Y+1, " "+badge+" ", badgeStyle)

	line := area.Y + 2
	if showAndroidWarning(ctx) {
		put(s, area.X, line, "    \u26A0 ANDROID_HOME no configurada", base.Foreground(th.Warning))
		line++
	}
	if reminder {
		put(s, area.X, line, "    \u26A0 InferX: falta la API — compass --task opencode-configure", base.Foreground(th.Warning))
		if reminderBtn != nil {
			putCenter(s, reminderBtn.X+reminderBtn.Width/2, reminderBtn.Y, "[x]", base.Foreground(th.Warning).Bold(true))
		}
	}
}

// footerSeg es un tramo del pie: texto y color.
type footerSeg struct {
	text string
	col  tcell.Color
}

// renderFooter pinta el pie del menu principal.
func renderFooter(s tcell.Screen, area Rect, th *theme.Theme, ctx *project.ProjectContext) {
	renderFooterSegs(s, area, th, ctx, []footerSeg{
		{"  Tab", th.Text},
		{" pestañas   ", th.Muted},
		{"\u2190\u2192\u2191\u2193", th.Text},
		{" navegar   ", th.Muted},
		{"Enter", th.Text},
		{" ejecutar   ", th.Muted},
		{"Esc", th.Text},
		{" salir   ", th.Muted},
		{"Ctrl+O opciones  ", th.Brand},
		{fmt.Sprintf("Ctrl+T temas: %s  ", th.Name), th.Brand},
	})
}

// renderSubmenuFooter pinta el pie de un submenu con sus propias teclas, pero
// con el mismo estilo, posicion y ruta que el menu principal.
func renderSubmenuFooter(s tcell.Screen, area Rect, th *theme.Theme, ctx *project.ProjectContext) {
	renderFooterSegs(s, area, th, ctx, []footerSeg{
		{"  \u2190\u2192\u2191\u2193", th.Text},
		{" navegar   ", th.Muted},
		{"Enter", th.Text},
		{" ejecutar   ", th.Muted},
		{"Esc", th.Text},
		{" volver   ", th.Muted},
		{fmt.Sprintf("Ctrl+T temas: %s  ", th.Name), th.Brand},
	})
}

// renderFooterSegs pinta los atajos a la izquierda y la ruta a la derecha.
func renderFooterSegs(s tcell.Screen, area Rect, th *theme.Theme, ctx *project.ProjectContext, segs []footerSeg) {
	base := bgStyle(th)

	x := area.X
	for _, sg := range segs {
		put(s, x, area.Y, sg.text, base.Foreground(sg.col))
		x += unisegWidth(sg.text)
	}
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
	put(s, area.Right()-shownW, area.Y, shown, base.Foreground(th.Muted))
}

func renderSettingsModal(s tcell.Screen, area Rect, th *theme.Theme, m SettingsModal) {
	w := 66
	if w > area.Width {
		w = area.Width
	}
	h := 7
	if h > area.Height {
		h = area.Height
	}
	x := area.X + (area.Width-w)/2
	y := area.Y + (area.Height-h)/2
	rect := Rect{x, y, w, h}

	base := bgStyle(th)
	fillRect(s, rect, ' ', base)
	drawBox(s, rect, base.Foreground(th.Brand))
	put(s, rect.X+2, rect.Y, " Opciones ", base.Foreground(th.Text).Bold(true))

	check := "[ ]"
	if m.openOutputDir {
		check = "[x]"
	}
	markStyle := base.Foreground(th.Muted)
	if m.openOutputDir {
		markStyle = base.Foreground(th.Success).Bold(true)
	}
	put(s, rect.X+2, rect.Y+2, check, markStyle)
	put(s, rect.X+6, rect.Y+2, "Abrir la carpeta de salida al terminar un build", base.Foreground(th.Text))

	put(s, rect.X+2, rect.Y+4, "El aviso sonoro al terminar un build siempre esta activo.", base.Foreground(th.Muted))
	put(s, rect.X+1, rect.Bottom()-1, " Espacio/Enter alternar  Esc cerrar ", base.Foreground(th.Muted))
}

func renderThemePicker(s tcell.Screen, area Rect, th *theme.Theme, picker ThemePicker) {
	count := len(theme.Themes)
	w := 44
	if w > area.Width {
		w = area.Width
	}
	h := count + 2
	if h > area.Height {
		h = area.Height
	}
	x := area.X + (area.Width-w)/2
	y := area.Y + (area.Height-h)/2
	rect := Rect{x, y, w, h}

	base := bgStyle(th)
	fillRect(s, rect, ' ', base)
	drawBox(s, rect, base.Foreground(th.Brand))

	// Titulo " Temas ".
	put(s, rect.X+2, rect.Y, " Temas ", base.Foreground(th.Text).Bold(true))

	// La lista puede no caber en pantallas bajas: se desplaza para mantener
	// visible el tema seleccionado.
	visible := h - 2
	start := 0
	if picker.index >= visible {
		start = picker.index - visible + 1
	}

	for i := start; i < count; i++ {
		rowY := rect.Y + 1 + (i - start)
		if rowY >= rect.Bottom()-1 {
			break
		}
		selected := i == picker.index
		marker := "  "
		if selected {
			marker = "\u25B8 "
		}
		mStyle := base.Foreground(th.Muted)
		if selected {
			mStyle = base.Foreground(th.Brand)
		}
		put(s, rect.X+1, rowY, " "+marker, mStyle)

		nameStyle := base.Foreground(th.Muted)
		if selected {
			nameStyle = base.Foreground(th.Text)
		}
		t := theme.Themes[i]
		put(s, rect.X+4, rowY, padRight(t.Name, 12), nameStyle)

		// Muestras DEV/MNT/CFG.
		sx := rect.X + 4 + 12 + 2
		put(s, sx, rowY, "██", base.Foreground(t.Dev))
		put(s, sx+2, rowY, "██", base.Foreground(t.Mnt))
		put(s, sx+4, rowY, "██", base.Foreground(t.Cfg))
	}

	// Pie del modal.
	put(s, rect.X+1, rect.Bottom()-1, " \u2191\u2193 elegir  Enter aplicar  Esc cancelar ", base.Foreground(th.Muted))
}

func renderQuitConfirm(s tcell.Screen, area Rect, th *theme.Theme, confirm QuitConfirm) {
	w := 52
	if w > area.Width {
		w = area.Width
	}
	h := 6
	if h > area.Height {
		h = area.Height
	}
	x := area.X + (area.Width-w)/2
	y := area.Y + (area.Height-h)/2
	rect := Rect{x, y, w, h}

	base := bgStyle(th)
	fillRect(s, rect, ' ', base)
	drawBox(s, rect, base.Foreground(th.Warning))
	put(s, rect.X+2, rect.Y, " Salir de Compass ", base.Foreground(th.Warning).Bold(true))

	var tabsLine, runningLine string
	if confirm.tabs == 1 {
		tabsLine = " Hay 1 pestaña abierta."
	} else {
		tabsLine = fmt.Sprintf(" Hay %d pestañas abiertas.", confirm.tabs)
	}
	switch confirm.running {
	case 0:
		runningLine = " Sus procesos ya terminaron."
	case 1:
		runningLine = " 1 de ellas con un proceso en ejecución."
	default:
		runningLine = fmt.Sprintf(" %d de ellas con procesos en ejecución.", confirm.running)
	}

	put(s, rect.X+1, rect.Y+1, tabsLine, base.Foreground(th.Text).Bold(true))
	put(s, rect.X+1, rect.Y+2, runningLine, base.Foreground(th.Warning))
	put(s, rect.X+1, rect.Y+4, " Al salir se cerrarán sus pestañas y procesos.", base.Foreground(th.Muted))
	put(s, rect.X+1, rect.Bottom()-1, " Enter salir y cerrar  Esc cancelar ", base.Foreground(th.Muted))
}

// padRight rellena con espacios hasta `n` celdas.
func padRight(s string, n int) string {
	w := unisegWidth(s)
	if w >= n {
		return s
	}
	return s + strings.Repeat(" ", n-w)
}

// baseName devuelve el ultimo componente de una ruta.
func baseName(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimRight(p, "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}
