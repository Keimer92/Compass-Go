// Package tui implementa el menu interactivo de Compass sobre tcell.
//
// Es el equivalente de src/menu.rs. Cubre el menu base (layout en rejilla,
// cards, pestanas, scrollbar, cabecera y pie, con teclado y raton), los modales
// de temas y de salida, y el contenedor de terminales incrustadas: cada card
// abre su tarea en una pestana ConPTY dentro de esta misma ventana.
package tui

import (
	"math"
	"time"

	"compass/internal/project"
	"compass/internal/theme"
)

// Geometria de la rejilla (equivale a las constantes de menu.rs).
const (
	CardH       = 3
	CardGap     = 1
	RowH        = CardH + CardGap
	cardMinW    = 24
	gridMaxCols = 4
	FooterH     = 1
	TabsH       = 1
)

// Version es la version mostrada en la cabecera (CARGO_PKG_VERSION en Rust).
const Version = "1.0.0"

// MenuItem identifica cada card del menu.
type MenuItem int

const (
	// DEV
	WindowsDev MenuItem = iota
	AndroidDev
	AndroidHmr
	WindowsBuild
	AndroidBuildArm64
	AndroidBuildArmv7
	AndroidBuildAll
	InstallDebug
	FlutterProfile
	Tests
	// MNT
	Clean
	DebugAndroid
	Signing
	VersionItem
	// CFG
	Git
	BuildWhatsApp
	ConfigureTerminal
	Environment
)

// Tab es cada pestana del menu.
type Tab int

const (
	TabDevMnt Tab = iota
	TabCfg
)

func (t Tab) index() int { return int(t) }

func (t Tab) next() Tab {
	if t == TabDevMnt {
		return TabCfg
	}
	return TabDevMnt
}

func (t Tab) prev() Tab { return t.next() }

// TabRange es el rango de items que pertenece a una pestana (end exclusivo).
type TabRange struct{ start, end int }

func (r TabRange) len() int {
	if r.end <= r.start {
		return 0
	}
	return r.end - r.start
}

// Icon devuelve el emoji de la card.
func (m MenuItem) Icon() string {
	switch m {
	case WindowsDev:
		return "\U0001F5A5"
	case AndroidDev:
		return "\U0001F4F1"
	case AndroidHmr:
		return "\U0001F525"
	case WindowsBuild:
		return "\U0001F4E6"
	case AndroidBuildArm64, AndroidBuildArmv7, AndroidBuildAll:
		return "\U0001F680"
	case InstallDebug:
		return "\U0001F4E6"
	case FlutterProfile:
		return "\U0001F3AF"
	case Tests:
		return "\U0001F9EA"
	case Clean:
		return "\U0001F9F9"
	case DebugAndroid:
		return "\U0001F50D"
	case Signing:
		return "\U0001F511"
	case VersionItem:
		return "\U0001F516"
	case Git:
		return "\U0001F527"
	case BuildWhatsApp:
		return "\U0001F4AC"
	case ConfigureTerminal:
		return "\u2699"
	case Environment:
		return "\U0001F4BE"
	}
	return ""
}

// Section devuelve la seccion ("DEV" | "MNT" | "CFG") de la card.
func (m MenuItem) Section() string {
	switch m {
	case WindowsDev, AndroidDev, AndroidHmr, WindowsBuild, AndroidBuildArm64,
		AndroidBuildArmv7, AndroidBuildAll, InstallDebug, FlutterProfile, Tests:
		return "DEV"
	case Clean, DebugAndroid, Signing, VersionItem:
		return "MNT"
	default:
		return "CFG"
	}
}

// Tab devuelve la pestana a la que pertenece la card.
func (m MenuItem) Tab() Tab {
	if m.Section() == "CFG" {
		return TabCfg
	}
	return TabDevMnt
}

// ColorKey es la clave del acento en theme.CardKeys.
func (m MenuItem) ColorKey() string {
	switch m {
	case WindowsDev:
		return "run"
	case AndroidDev:
		return "android"
	case AndroidHmr:
		return "reload"
	case WindowsBuild:
		return "build"
	case AndroidBuildArm64:
		return "apk"
	case AndroidBuildArmv7:
		return "apk32"
	case AndroidBuildAll:
		return "aab"
	case InstallDebug:
		return "install"
	case FlutterProfile:
		return "profile"
	case Tests:
		return "test"
	case Clean:
		return "clean"
	case DebugAndroid:
		return "debug"
	case Signing:
		return "sign"
	case VersionItem:
		return "version"
	case Git:
		return "git"
	case BuildWhatsApp:
		return "whatsapp"
	case ConfigureTerminal:
		return "terminal"
	case Environment:
		return "environment"
	}
	return "terminal"
}

// Label devuelve la etiqueta corta de la card segun el tipo de proyecto.
func (m MenuItem) Label(pt project.ProjectType) string {
	switch m {
	case WindowsDev:
		switch pt {
		case project.TypeRust, project.TypePython, project.TypeGo:
			return "Run"
		}
		return "Windows"
	case AndroidDev:
		if pt == project.TypeGoWails {
			return "And. Dev"
		}
		return "Android"
	case AndroidHmr:
		if pt == project.TypePython {
			return "Reload"
		}
		return "HMR"
	case WindowsBuild:
		switch pt {
		case project.TypeTauri, project.TypeRust, project.TypeGo, project.TypeGoWails:
			return "Win Release"
		case project.TypePython:
			return "Package"
		}
		return "Win Build"
	case AndroidBuildArm64:
		switch pt {
		case project.TypeTauri, project.TypeFlutter, project.TypeKotlin, project.TypeGoWails:
			return "APK Release"
		}
		return ""
	case AndroidBuildArmv7:
		return ""
	case AndroidBuildAll:
		switch pt {
		case project.TypeTauri, project.TypeFlutter, project.TypeKotlin, project.TypeGoWails:
			return "AAB"
		}
		return ""
	case InstallDebug:
		return "Install"
	case FlutterProfile:
		return "Profile"
	case Tests:
		if pt == project.TypePython || pt == project.TypeGo {
			return "Test"
		}
		return ""
	case Clean:
		return "Clean"
	case DebugAndroid:
		return "Debug Andr"
	case Signing:
		return "Signing"
	case VersionItem:
		return "Version"
	case Git:
		return "Git"
	case BuildWhatsApp:
		return "WhatsApp"
	case ConfigureTerminal:
		return "Terminal"
	case Environment:
		return "Entorno"
	}
	return ""
}

// Description devuelve la descripcion de la card segun el tipo de proyecto.
func (m MenuItem) Description(pt project.ProjectType) string {
	switch pt {
	case project.TypeRust:
		switch m {
		case WindowsDev:
			return "cargo run"
		case WindowsBuild:
			return "x86 · x64 · ARM64"
		case Clean:
			return "Limpia target/ y cache"
		case VersionItem:
			return "Actualiza Cargo.toml"
		case Git:
			return "Init · gitignore · remoto"
		case BuildWhatsApp:
			return "Compila WhatsApp"
		case ConfigureTerminal:
			return "Autocompletado"
		case Environment:
			return "Backup y restauracion"
		}
	case project.TypeTauri:
		switch m {
		case WindowsDev:
			return "Inicia el dev server"
		case AndroidDev:
			return "Dev con target Android"
		case WindowsBuild:
			return "x86 · x64 · ARM64"
		case AndroidBuildArm64:
			return "arm64 · armv7 · universal"
		case AndroidBuildAll:
			return "Bundle para Play Store"
		case Clean:
			return "Limpia builds/deps"
		case DebugAndroid:
			return "Dev + Chrome DevTools"
		case Signing:
			return "Firmas y huellas"
		case Git:
			return "Init · gitignore · remoto"
		case BuildWhatsApp:
			return "Compila WhatsApp"
		case VersionItem:
			return "Bump pkg/cargo/tauri"
		case ConfigureTerminal:
			return "Autocompletado"
		case Environment:
			return "Backup y restauracion"
		}
	case project.TypeFlutter:
		switch m {
		case WindowsDev:
			return "flutter run windows"
		case AndroidDev:
			return "flutter run android"
		case WindowsBuild:
			return "Solo host (x64)"
		case AndroidBuildArm64:
			return "arm64 · armv7 · universal"
		case AndroidBuildAll:
			return "Bundle para Play Store"
		case Clean:
			return "Limpia builds/deps"
		case DebugAndroid:
			return "Depurar Android"
		case Signing:
			return "Firmas y huellas"
		case Git:
			return "Init · gitignore · remoto"
		case BuildWhatsApp:
			return "Compila WhatsApp"
		case ConfigureTerminal:
			return "Autocompletado"
		case Environment:
			return "Backup y restauracion"
		case FlutterProfile:
			return "flutter run --profile"
		}
	case project.TypeKotlin:
		switch m {
		case WindowsDev:
			return "gradlew :composeApp:run"
		case AndroidDev:
			return "gradlew :installDebug"
		case WindowsBuild:
			return "Solo host (x64)"
		case AndroidBuildArm64:
			return "arm64 · armv7 · universal"
		case AndroidBuildAll:
			return "Bundle para Play Store"
		case InstallDebug:
			return "gradlew :installDebug"
		case Clean:
			return "Limpia builds/deps"
		case DebugAndroid:
			return "Depurar Android"
		case Signing:
			return "Firmas y huellas"
		case Git:
			return "Init · gitignore · remoto"
		case BuildWhatsApp:
			return "Compila WhatsApp"
		case ConfigureTerminal:
			return "Autocompletado"
		case Environment:
			return "Backup y restauracion"
		}
	case project.TypeGoWails:
		switch m {
		case WindowsDev:
			return "dev escritorio"
		case AndroidDev:
			return "APK dev"
		case AndroidHmr:
			return "HMR (Vite)"
		case WindowsBuild:
			return "x86 · x64 · ARM64"
		case AndroidBuildArm64:
			return "arm64 · armv7 · universal"
		case AndroidBuildAll:
			return "Bundle para Play Store"
		case InstallDebug:
			return "APK dev"
		case Clean:
			return "Limpia builds/deps"
		case DebugAndroid:
			return "logcat de la app"
		case Signing:
			return "Firmas y huellas"
		case Git:
			return "Init · gitignore"
		case BuildWhatsApp:
			return "Compila WhatsApp"
		case VersionItem:
			return "main.go + config.yml + metadatos"
		case ConfigureTerminal:
			return "Autocompletado"
		case Environment:
			return "Backup y restauracion"
		}
	case project.TypePython:
		switch m {
		case WindowsDev:
			return "python <script/module>"
		case AndroidHmr:
			return "Uvicorn ASGI --reload"
		case WindowsBuild:
			return "python -m build"
		case Clean:
			return "Limpia caches y builds Python"
		case VersionItem:
			return "Actualiza pyproject.toml"
		case Git:
			return "Init · gitignore · remoto"
		case ConfigureTerminal:
			return "Autocompletado"
		case Environment:
			return "Backup y restauracion"
		case Tests:
			return "python -m pytest"
		}
	case project.TypeGo:
		switch m {
		case WindowsDev:
			return "go run ."
		case AndroidHmr:
			return "air (rebuild + restart)"
		case WindowsBuild:
			return "x86 · x64 · ARM64"
		case Clean:
			return "Limpia binarios"
		case Git:
			return "Init · gitignore · remoto"
		case ConfigureTerminal:
			return "Autocompletado"
		case Environment:
			return "Backup y restauracion"
		case Tests:
			return "go test ./..."
		}
	}
	return ""
}

// Rect es una region de la pantalla.
type Rect struct{ X, Y, Width, Height int }

func (r Rect) Right() int  { return r.X + r.Width }
func (r Rect) Bottom() int { return r.Y + r.Height }

func (r Rect) Contains(x, y int) bool {
	return x >= r.X && x < r.Right() && y >= r.Y && y < r.Bottom()
}

// UiLayout son las regiones calculadas por layout().
type UiLayout struct {
	Header         Rect
	Tabs           Rect
	List           Rect
	Cols           int
	Scrollbar      Rect
	Footer         Rect
	ReminderButton *Rect
}

func gridCols(width int) int {
	c := (width + CardGap) / (cardMinW + CardGap)
	if c < 1 {
		c = 1
	}
	if c > gridMaxCols {
		c = gridMaxCols
	}
	return c
}

func gridCellW(width, cols int) int {
	if cols < 1 {
		cols = 1
	}
	w := width / cols
	if w < 1 {
		w = 1
	}
	return w
}

func gridCardW(width, cols int) int {
	w := gridCellW(width, cols) - CardGap
	if w < 0 {
		return 0
	}
	return w
}

func visibleRows(height int) int {
	if height < 0 {
		return 0
	}
	return (height + CardGap) / RowH
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// layout calcula las regiones de la pantalla.
func layout(area Rect, warn, reminder bool) UiLayout {
	headerH := 2 + btoi(warn) + btoi(reminder)

	header := Rect{area.X, area.Y, area.Width, headerH}
	tabs := Rect{area.X, area.Y + headerH, area.Width, TabsH}
	listTop := area.Y + headerH + TabsH
	footer := Rect{area.X, area.Bottom() - FooterH, area.Width, FooterH}
	listH := footer.Y - listTop
	if listH < 0 {
		listH = 0
	}
	list := Rect{area.X, listTop, area.Width, listH}

	margin := 0
	if list.Width > 4 {
		margin = 1
	}
	barRoom := 0
	if list.Width >= 8 {
		barRoom = 2
	}
	listW := list.Width - (margin + barRoom)
	if listW < 0 {
		listW = 0
	}
	listRect := Rect{list.X + margin, list.Y, listW, list.Height}
	cols := gridCols(listW)

	var sb Rect
	if barRoom == 2 {
		sb = Rect{listRect.Right() + 1, list.Y, 1, list.Height}
	} else {
		sb = Rect{list.Right() - 1, list.Y, 0, list.Height}
	}

	var rb *Rect
	if reminder && header.Width >= 8 {
		r := Rect{header.Right() - 4, header.Y + 2 + btoi(warn), 3, 1}
		rb = &r
	}

	return UiLayout{
		Header:         header,
		Tabs:           tabs,
		List:           listRect,
		Cols:           cols,
		Scrollbar:      sb,
		Footer:         footer,
		ReminderButton: rb,
	}
}

// ThemePicker es el estado del modal de temas.
type ThemePicker struct {
	index    int
	original *theme.Theme
}

// QuitConfirm son los datos del aviso de salida.
type QuitConfirm struct {
	tabs    int
	running int
}

// menuGroup es una columna de la pestana DEV/MNT (indices globales).
type menuGroup struct {
	start, end int
}

// MenuState es el estado del menu.
type MenuState struct {
	items    []MenuItem
	groups   []menuGroup
	tabs     [2]TabRange
	tab      Tab
	selected int
	remember [2]int
	hover    int // -1 = ninguno
	// hoverStarted es el instante en que el puntero entro en la card
	// hovereada: arranca el barrido de brillo y se limpia al salir.
	hoverStarted time.Time
	scroll       int
	vh           int
	cols         int
	ui           *UiLayout

	theme            *theme.Theme
	picker           *ThemePicker
	quit             *QuitConfirm
	opencodeReminder bool
}

// newMenuState construye el estado para un tipo de proyecto.
//
// La pestana DEV/MNT se agrupa por plataforma de compilacion (Windows,
// Android y comun) para pintarla en columnas; CFG va en una sola lista.
func newMenuState(pt project.ProjectType, th *theme.Theme) *MenuState {
	base := []MenuItem{
		WindowsDev, AndroidDev, AndroidHmr, WindowsBuild, AndroidBuildArm64,
		AndroidBuildArmv7, AndroidBuildAll, InstallDebug, FlutterProfile, Tests,
		Clean, DebugAndroid, Signing, VersionItem, Git, BuildWhatsApp,
		ConfigureTerminal, Environment,
	}

	var dev [3][]MenuItem
	var cfg []MenuItem
	for _, it := range base {
		// La card suelta de armv7 sobra en todos los tipos.
		if !keepForType(it, pt) || it == AndroidBuildArmv7 {
			continue
		}
		if it.Tab() == TabCfg {
			cfg = append(cfg, it)
			continue
		}
		g := menuGroupOf(it, pt)
		dev[g] = append(dev[g], it)
	}

	var items []MenuItem
	var groups []menuGroup
	for _, list := range dev {
		if len(list) == 0 {
			continue
		}
		start := len(items)
		items = append(items, list...)
		groups = append(groups, menuGroup{start: start, end: len(items)})
	}
	devEnd := len(items)
	items = append(items, cfg...)

	tabs := [2]TabRange{
		{start: 0, end: devEnd},
		{start: devEnd, end: len(items)},
	}

	return &MenuState{
		items:    items,
		groups:   groups,
		tabs:     tabs,
		tab:      TabDevMnt,
		selected: tabs[0].start,
		remember: [2]int{tabs[0].start, tabs[1].start},
		hover:    -1,
		cols:     1,
		theme:    th,
	}
}

// Grupos de la pestana DEV/MNT (orden de las columnas).
const (
	groupWindows = iota
	groupAndroid
	groupCommon
)

// menuGroupOf decide a que columna pertenece una card.
func menuGroupOf(it MenuItem, pt project.ProjectType) int {
	switch it {
	case WindowsDev, WindowsBuild, FlutterProfile:
		return groupWindows
	case AndroidDev, AndroidBuildArm64, AndroidBuildArmv7, AndroidBuildAll,
		InstallDebug, DebugAndroid, Signing:
		return groupAndroid
	case AndroidHmr:
		// En Tauri/Flutter/Kotlin/Wails es HMR de Android; en Go/Python es
		// el reload del servidor de desarrollo (comun).
		switch pt {
		case project.TypeTauri, project.TypeFlutter, project.TypeKotlin, project.TypeGoWails:
			return groupAndroid
		}
	}
	return groupCommon
}

// keepForType replica el filtro por tipo de proyecto de MenuState::new.
func keepForType(item MenuItem, pt project.ProjectType) bool {
	switch pt {
	case project.TypeRust:
		switch item {
		case AndroidDev, AndroidBuildArm64, AndroidBuildArmv7, AndroidBuildAll,
			InstallDebug, DebugAndroid, Signing, FlutterProfile, AndroidHmr, Tests:
			return false
		}
	case project.TypeTauri:
		switch item {
		case InstallDebug, FlutterProfile, AndroidHmr, Tests:
			return false
		}
	case project.TypeFlutter:
		switch item {
		case AndroidBuildArmv7, VersionItem, InstallDebug, AndroidHmr, Tests:
			return false
		}
	case project.TypeKotlin:
		switch item {
		case AndroidBuildArmv7, VersionItem, FlutterProfile, AndroidHmr, Tests:
			return false
		}
	case project.TypeGoWails:
		switch item {
		case AndroidBuildArmv7, FlutterProfile, Tests:
			return false
		}
	case project.TypePython:
		switch item {
		case AndroidDev, AndroidBuildArm64, AndroidBuildArmv7, AndroidBuildAll,
			InstallDebug, FlutterProfile, DebugAndroid, Signing, BuildWhatsApp:
			return false
		}
	case project.TypeGo:
		switch item {
		case AndroidDev, AndroidBuildArm64, AndroidBuildArmv7, AndroidBuildAll,
			InstallDebug, FlutterProfile, DebugAndroid, Signing, VersionItem, BuildWhatsApp:
			return false
		}
	}
	return true
}

func (s *MenuState) tabRange() TabRange { return s.tabs[s.tab.index()] }
func (s *MenuState) tabLen() int        { return s.tabRange().len() }

func (s *MenuState) localSelected() int {
	v := s.selected - s.tabRange().start
	if v < 0 {
		return 0
	}
	return v
}

func (s *MenuState) openPicker() {
	s.picker = &ThemePicker{index: theme.IndexOf(s.theme.Name), original: s.theme}
}

func (s *MenuState) previewTheme(t *theme.Theme) {
	if s.picker != nil {
		s.picker.index = theme.IndexOf(t.Name)
	}
	s.theme = t
}

func (s *MenuState) setHover(h int) {
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

// hoverPhase es la fase (0..1) del halo mientras el puntero esta sobre una
// card. El halo gira en bucle: una vuelta completa por hoverCycle.
func (s *MenuState) hoverPhase() (float32, bool) {
	if s.hover < 0 || s.hoverStarted.IsZero() {
		return 0, false
	}
	p := float32(time.Since(s.hoverStarted).Seconds()) / float32(hoverCycle.Seconds())
	return p - float32(math.Floor(float64(p))), true
}

// hoverAnimating indica si hay que repintar a ~30 fps para animar el halo.
func (s *MenuState) hoverAnimating() bool {
	return s.hover >= 0 && !s.hoverStarted.IsZero()
}

// groupedLayoutFor indica si la pestana actual debe pintarse en columnas por
// plataforma y devuelve sus grupos. Necesita ancho suficiente para que cada
// columna mantenga el ancho minimo de card; si no, se usa la rejilla normal.
func groupedLayoutFor(ui UiLayout, tab Tab, groups []menuGroup) ([]menuGroup, int, bool) {
	if tab != TabDevMnt || len(groups) < 2 {
		return nil, 0, false
	}
	cols := len(groups)
	if gridCardW(ui.List.Width, cols) < cardMinW {
		return nil, 0, false
	}
	return groups, cols, true
}

// grouped devuelve el layout agrupado segun el ultimo render.
func (s *MenuState) grouped() ([]menuGroup, int, bool) {
	if s.ui == nil {
		return nil, 0, false
	}
	return groupedLayoutFor(*s.ui, s.tab, s.groups)
}

// totalRowsFor calcula las filas de contenido para la barra de scroll.
func (s *MenuState) totalRowsFor(ui UiLayout) int {
	n := s.tabLen()
	if n == 0 {
		return 0
	}
	if groups, _, ok := groupedLayoutFor(ui, s.tab, s.groups); ok {
		rows := 0
		for _, g := range groups {
			if l := g.end - g.start; l > rows {
				rows = l
			}
		}
		return rows
	}
	cols := ui.Cols
	if cols < 1 {
		cols = 1
	}
	return (n + cols - 1) / cols
}

func (s *MenuState) ensureVisible() {
	n := s.tabLen()
	if n == 0 || s.vh < CardH {
		s.scroll = 0
		return
	}
	rows := visibleRows(s.vh)
	if rows < 1 {
		rows = 1
	}

	var row, totalRows int
	if groups, _, ok := s.grouped(); ok {
		for _, g := range groups {
			if s.selected >= g.start && s.selected < g.end {
				row = s.selected - g.start
			}
			if l := g.end - g.start; l > totalRows {
				totalRows = l
			}
		}
	} else {
		cols := s.cols
		if cols < 1 {
			cols = 1
		}
		i := s.localSelected()
		if i > n-1 {
			i = n - 1
		}
		row = i / cols
		totalRows = (n + cols - 1) / cols
	}

	maxScroll := totalRows - rows
	if maxScroll < 0 {
		maxScroll = 0
	}

	sc := s.scroll
	if row < sc {
		sc = row
	} else if row >= sc+rows {
		sc = row + 1 - rows
	}
	if sc > maxScroll {
		sc = maxScroll
	}
	if sc < 0 {
		sc = 0
	}
	s.scroll = sc
}

func (s *MenuState) setTab(t Tab) {
	if s.tab == t {
		return
	}
	s.remember[s.tab.index()] = s.selected
	s.tab = t
	r := s.tabs[t.index()]
	if r.len() == 0 {
		return
	}
	sel := s.remember[t.index()]
	if sel < r.start {
		sel = r.start
	} else if sel > r.end-1 {
		sel = r.end - 1
	}
	s.selected = sel
	s.remember[t.index()] = s.selected
	s.ensureVisible()
}

func (s *MenuState) jumpTo(idx int) {
	if len(s.items) == 0 {
		return
	}
	if idx > len(s.items)-1 {
		idx = len(s.items) - 1
	}
	target := s.items[idx].Tab()
	if target != s.tab {
		s.remember[s.tab.index()] = s.selected
		s.tab = target
	}
	s.selected = idx
	s.ensureVisible()
}

func (s *MenuState) moveSelection(delta int) {
	if len(s.items) == 0 {
		return
	}
	// En columnas por plataforma, ←/→ cambia de columna manteniendo la fila.
	if groups, _, ok := s.grouped(); ok {
		for gi, g := range groups {
			if s.selected < g.start || s.selected >= g.end {
				continue
			}
			row := s.selected - g.start
			next := gi + delta
			if next < 0 {
				next = 0
			} else if next >= len(groups) {
				next = len(groups) - 1
			}
			target := groups[next]
			if last := target.end - target.start - 1; row > last {
				row = last
			}
			s.selected = target.start + row
			s.ensureVisible()
			return
		}
	}
	max := len(s.items) - 1
	next := s.selected + delta
	if next < 0 {
		next = 0
	} else if next > max {
		next = max
	}
	s.jumpTo(next)
}

func (s *MenuState) moveRow(delta int) {
	n := s.tabLen()
	if n == 0 {
		return
	}
	// En columnas por plataforma, ↑/↓ recorre solo la columna actual.
	if groups, _, ok := s.grouped(); ok {
		for _, g := range groups {
			if s.selected < g.start || s.selected >= g.end {
				continue
			}
			row := s.selected - g.start + delta
			if row < 0 {
				row = 0
			}
			if last := g.end - g.start - 1; row > last {
				row = last
			}
			s.selected = g.start + row
			s.ensureVisible()
			return
		}
	}
	step := s.cols
	if step < 1 {
		step = 1
	}
	local := s.localSelected()
	if local > n-1 {
		local = n - 1
	}
	next := local + delta*step
	if next < 0 {
		next = 0
	} else if next > n-1 {
		next = n - 1
	}
	s.jumpTo(s.tabRange().start + next)
}

// hoverAt devuelve el indice global de la card bajo (col,row), o -1.
func (s *MenuState) hoverAt(col, row int) int {
	ui := s.ui
	if ui == nil {
		return -1
	}
	if groups, cols, ok := s.grouped(); ok {
		if idx, ok := groupedIndexAt(*ui, s.scroll, groups, cols, col, row); ok {
			return idx
		}
		return -1
	}
	local, ok := gridIndexAt(*ui, s.scroll, s.tabLen(), col, row)
	if !ok {
		return -1
	}
	return s.tabRange().start + local
}

// groupedIndexAt traduce coordenadas a un indice global en el layout por
// columnas (una columna por grupo, de arriba abajo).
func groupedIndexAt(ui UiLayout, scroll int, groups []menuGroup, cols, col, row int) (int, bool) {
	if !ui.List.Contains(col, row) || cols < 1 {
		return 0, false
	}
	cellW := gridCellW(ui.List.Width, cols)
	cardW := gridCardW(ui.List.Width, cols)

	xOff := col - ui.List.X
	gi := xOff / cellW
	if gi >= cols || xOff%cellW >= cardW {
		return 0, false
	}

	yOff := (row - ui.List.Y) + scroll*RowH
	if yOff%RowH >= CardH {
		return 0, false
	}
	r := yOff / RowH
	g := groups[gi]
	if r >= g.end-g.start {
		return 0, false
	}
	return g.start + r, true
}

// gridIndexAt devuelve el indice local de card bajo unas coordenadas.
func gridIndexAt(ui UiLayout, scroll, length, col, row int) (int, bool) {
	if !ui.List.Contains(col, row) {
		return 0, false
	}
	cellW := gridCellW(ui.List.Width, ui.Cols)
	cardW := gridCardW(ui.List.Width, ui.Cols)

	xOff := col - ui.List.X
	gridCol := xOff / cellW
	if gridCol >= ui.Cols || xOff%cellW >= cardW {
		return 0, false
	}

	yOff := (row - ui.List.Y) + scroll*RowH
	if yOff%RowH >= CardH {
		return 0, false
	}
	local := (yOff/RowH)*ui.Cols + gridCol
	if local >= length {
		return 0, false
	}
	return local, true
}

// tabAt devuelve la pestana bajo (col,row).
func (s *MenuState) tabAt(col, row int) (Tab, bool) {
	ui := s.ui
	if ui == nil || !ui.Tabs.Contains(col, row) {
		return 0, false
	}
	titles := s.tabTitles()
	start := ui.Tabs.X
	devEnd := start + unisegWidth(titles[0])
	if col < devEnd {
		return TabDevMnt, true
	}
	cfgStart := devEnd + 3 // " │ "
	if col >= cfgStart && col < cfgStart+unisegWidth(titles[1]) {
		return TabCfg, true
	}
	return 0, false
}

// tabTitles devuelve las etiquetas de la barra de pestanas.
func (s *MenuState) tabTitles() [2]string {
	return [2]string{
		" DESARROLLO · MNT " + itoa(s.tabs[TabDevMnt.index()].len()) + " ",
		" CONFIGURACION " + itoa(s.tabs[TabCfg.index()].len()) + " ",
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
