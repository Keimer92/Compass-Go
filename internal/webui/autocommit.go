package webui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"compass/internal/settings"
)

// Auto-commit por inactividad.
//
// El boton "Auto" del panel Git activa, para el proyecto activo, un vigilante
// que mira `git status --porcelain` cada autoTickEvery. Mientras la firma del
// estado no cambie, corre un reloj; cuando pasan los minutos configurados en
// Opciones (settings.AutoCommitIdleMin, 5 por defecto) sin ningun cambio nuevo,
// genera el mensaje (OpenRouter si hay API key; si no, un mensaje generico) y
// commitea. Nunca hace push: subir sigue siendo manual.
const autoTickEvery = 15 * time.Second

// autoIdle lee el umbral actual. Se consulta en cada pasada para que cambiar
// los minutos en Opciones surta efecto sin apagar y encender el boton.
func autoIdle() time.Duration {
	m := settings.Load().AutoCommitIdleMin
	if m < 1 {
		m = 5
	}
	return time.Duration(m) * time.Minute
}

// AutoCommitState describe el vigilante del proyecto activo para la UI.
type AutoCommitState struct {
	Enabled   bool   `json:"enabled"`
	IdleMin   int    `json:"idleMin"`
	Pending   bool   `json:"pending"` // hay cambios esperando el commit
	IdleSec   int    `json:"idleSec"` // segundos desde el ultimo cambio detectado
	Busy      bool   `json:"busy"`    // generando mensaje o commiteando
	LastMsg   string `json:"lastMsg"` // asunto del ultimo commit automatico
	LastError string `json:"lastError"`
}

// autoTrack es el estado del vigilante de UN proyecto.
type autoTrack struct {
	dir       string
	idle      time.Duration
	sig       string // ultima firma de `git status --porcelain`
	changedAt time.Time
	pending   bool
	busy      bool
	lastMsg   string
	lastErr   string
}

// SetAutoCommit activa o desactiva el commit automatico del proyecto activo.
//
// Al activarlo se siembra la firma actual, para que el reloj empiece a contar
// desde ese momento: cambios viejos no se commitean de golpe al pulsar Auto.
// El estado se persiste por proyecto: al reiniciar Compass el vigilante se
// rearma solo (resumeAuto), que es lo que el usuario espera del boton.
func (b *Bridge) SetAutoCommit(enabled bool) error {
	dir := b.projectDir()
	if dir == "" {
		return errors.New("no hay proyecto activo")
	}
	if enabled && !b.GitInfo().IsRepo {
		return errors.New("esta carpeta no es un repositorio git")
	}

	persistAuto(dir, enabled)
	if !enabled {
		b.autoMu.Lock()
		delete(b.autoRuns, commitKey(dir))
		b.autoMu.Unlock()
		return nil
	}
	b.armAuto(dir)
	return nil
}

// armAuto arranca el vigilante de dir si no estaba ya corriendo, sin tocar la
// persistencia. Lo usan SetAutoCommit y resumeAuto.
func (b *Bridge) armAuto(dir string) {
	if dir == "" {
		return
	}
	key := commitKey(dir)
	b.autoMu.Lock()
	if b.autoRuns == nil {
		b.autoRuns = map[string]*autoTrack{}
	}
	if _, ok := b.autoRuns[key]; ok {
		b.autoMu.Unlock()
		return
	}
	sig, _, _ := autoSig(dir)
	b.autoRuns[key] = &autoTrack{
		dir:       dir,
		idle:      autoIdle(),
		sig:       sig,
		changedAt: time.Now(),
		pending:   strings.TrimSpace(sig) != "",
	}
	b.autoMu.Unlock()

	b.autoOnce.Do(func() { go b.autoLoop() })
}

// resumeAuto rearma los vigilantes guardados en settings. Se llama al arrancar
// y al cambiar de proyecto: sin esto, reiniciar Compass apagaba el modo Auto
// aunque el usuario lo hubiera dejado activado.
//
// Solo se rearman proyectos que siguen en el workspace: quitar un proyecto de
// la barra lateral no debe dejar un commit automatico corriendo en segundo
// plano para siempre. El llamador debe tener tomado b.mu.
func (b *Bridge) resumeAuto() {
	want := map[string]bool{}
	for _, dir := range settings.Load().AutoCommitProjects {
		if strings.TrimSpace(dir) != "" {
			want[commitKey(dir)] = true
		}
	}
	if len(want) == 0 {
		return
	}
	for _, p := range b.ws.Projects {
		if p == nil || !want[commitKey(p.Path)] {
			continue
		}
		if st, err := os.Stat(p.Path); err != nil || !st.IsDir() {
			continue
		}
		if out, err := runGit(p.Path, "rev-parse", "--is-inside-work-tree"); err != nil || strings.TrimSpace(out) != "true" {
			continue
		}
		b.armAuto(p.Path)
	}
}

// persistAuto guarda o quita dir de la lista de proyectos con Auto activado.
func persistAuto(dir string, enabled bool) {
	key := commitKey(dir)
	s := settings.Load()
	next := make([]string, 0, len(s.AutoCommitProjects)+1)
	for _, p := range s.AutoCommitProjects {
		if commitKey(p) != key {
			next = append(next, p)
		}
	}
	if enabled {
		next = append(next, dir)
	}
	s.AutoCommitProjects = next
	settings.Save(s)
}

// stopAuto apaga el vigilante de dir y lo olvida en settings. Se usa cuando el
// proyecto sale del workspace: nadie debe commitear en segundo plano.
func (b *Bridge) stopAuto(dir string) {
	b.autoMu.Lock()
	delete(b.autoRuns, commitKey(dir))
	b.autoMu.Unlock()
	persistAuto(dir, false)
}

// AutoCommitStatus devuelve el estado del vigilante del proyecto activo.
func (b *Bridge) AutoCommitStatus() AutoCommitState {
	dir := b.projectDir()
	st := AutoCommitState{IdleMin: int(autoIdle() / time.Minute)}
	if dir == "" {
		return st
	}
	b.autoMu.Lock()
	defer b.autoMu.Unlock()
	r := b.autoRuns[commitKey(dir)]
	if r == nil {
		return st
	}
	st.Enabled = true
	st.IdleMin = int(autoIdle() / time.Minute)
	st.Pending = r.pending
	st.Busy = r.busy
	st.LastMsg = r.lastMsg
	st.LastError = r.lastErr
	if r.pending && !r.changedAt.IsZero() {
		st.IdleSec = int(time.Since(r.changedAt).Seconds())
	}
	return st
}

func (b *Bridge) autoLoop() {
	t := time.NewTicker(autoTickEvery)
	defer t.Stop()
	for range t.C {
		b.autoTick()
	}
}

func (b *Bridge) autoTick() {
	b.autoMu.Lock()
	runs := make([]*autoTrack, 0, len(b.autoRuns))
	for _, r := range b.autoRuns {
		runs = append(runs, r)
	}
	b.autoMu.Unlock()
	for _, r := range runs {
		b.autoStep(r)
	}
}

func (b *Bridge) autoStep(r *autoTrack) {
	sig, conflict, err := autoSig(r.dir)
	idle := autoIdle()

	b.autoMu.Lock()
	defer b.autoMu.Unlock()
	// El umbral se refresca en cada pasada: cambiarlo en Opciones aplica al
	// siguiente tick sin reiniciar el vigilante.
	r.idle = idle
	if r.busy {
		return
	}
	if err != nil && strings.TrimSpace(sig) == "" {
		r.lastErr = "git status: " + err.Error()
		return
	}
	if strings.TrimSpace(sig) == "" {
		r.sig = ""
		r.changedAt = time.Time{}
		r.pending = false
		r.lastErr = ""
		return
	}
	// Cualquier cambio nuevo reinicia el reloj: el commit solo ocurre cuando
	// el arbol lleva un rato quieto, no a mitad de una edicion. La firma
	// incluye tamano y mtime de cada archivo, asi que volver a guardar un
	// archivo ya modificado tambien cuenta como actividad.
	if sig != r.sig {
		r.sig = sig
		r.changedAt = time.Now()
		r.pending = true
		r.lastErr = ""
		return
	}
	if r.changedAt.IsZero() {
		r.changedAt = time.Now()
		return
	}
	if time.Since(r.changedAt) < r.idle {
		return
	}
	if conflict {
		// Un arbol en conflicto no se commitea solo: se espera a que lo
		// resuelva el usuario y se reinicia el reloj para no repetir el aviso.
		r.lastErr = "hay conflictos sin resolver"
		r.changedAt = time.Now()
		return
	}
	r.busy = true
	go b.autoCommit(r)
}

func (b *Bridge) autoCommit(r *autoTrack) {
	defer func() {
		b.autoMu.Lock()
		r.busy = false
		b.autoMu.Unlock()
	}()

	msg, err := b.GenerateCommit(r.dir)
	if err != nil || strings.TrimSpace(msg) == "" {
		// Sin OpenRouter configurado el commit sigue siendo util: se usa un
		// asunto generico con la fecha.
		msg = "chore: cambios automaticos " + time.Now().Format("2006-01-02 15:04")
	}
	out, cerr := b.Commit(r.dir, msg)

	b.autoMu.Lock()
	if cerr != nil {
		r.lastErr = cerr.Error()
		r.changedAt = time.Now()
	} else {
		r.lastErr = ""
		r.lastMsg = firstLine(msg)
		r.sig = ""
		r.changedAt = time.Time{}
		r.pending = false
	}
	b.autoMu.Unlock()

	if cerr == nil {
		Emit("git:auto", map[string]any{"dir": r.dir, "message": firstLine(msg), "out": out})
	}
}

// autoSig resume el estado del arbol para detectar actividad: codigos de
// `git status` MAS tamano y mtime de cada archivo cambiado. Solo los codigos no
// bastan: volver a editar un archivo ya modificado no cambia su "M" y el reloj
// seguia corriendo aunque el usuario estuviera escribiendo.
//
// Se usa `-z` para que git no entrecomille rutas con acentos o espacios; en
// ese formato un renombrado ocupa dos campos (ruta nueva y ruta original).
func autoSig(dir string) (sig string, conflict bool, err error) {
	raw, err := runGit(dir, "status", "--porcelain=v1", "-z", "-uall")
	if err != nil && strings.TrimSpace(raw) == "" {
		return "", false, err
	}
	var sb strings.Builder
	fields := strings.Split(raw, "\x00")
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if len(f) < 4 {
			continue
		}
		code, path := f[:2], f[3:]
		if code[0] == 'R' || code[0] == 'C' {
			i++ // el siguiente campo es la ruta original
		}
		if isConflict(code) {
			conflict = true
		}
		sb.WriteString(code)
		sb.WriteByte(' ')
		sb.WriteString(path)
		if fi, statErr := os.Stat(filepath.Join(dir, path)); statErr == nil {
			fmt.Fprintf(&sb, "|%d|%d", fi.Size(), fi.ModTime().UnixNano())
		} else {
			// Borrados (o rutas que ya no existen): la firma es estable.
			sb.WriteString("|gone")
		}
		sb.WriteByte('\n')
	}
	return sb.String(), conflict, nil
}

// isConflict detecta los codigos de conflicto de `git status --porcelain`.
func isConflict(code string) bool {
	switch code {
	case "UU", "AA", "DD", "AU", "UA", "DU", "UD":
		return true
	}
	return false
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}
