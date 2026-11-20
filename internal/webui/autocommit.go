package webui

import (
	"errors"
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
func (b *Bridge) SetAutoCommit(enabled bool) error {
	dir := b.projectDir()
	if dir == "" {
		return errors.New("no hay proyecto activo")
	}
	if enabled && !b.GitInfo().IsRepo {
		return errors.New("esta carpeta no es un repositorio git")
	}

	key := commitKey(dir)
	b.autoMu.Lock()
	if b.autoRuns == nil {
		b.autoRuns = map[string]*autoTrack{}
	}
	if !enabled {
		delete(b.autoRuns, key)
		b.autoMu.Unlock()
		return nil
	}
	sig, _ := runGit(dir, "status", "--porcelain=v1", "-uall")
	b.autoRuns[key] = &autoTrack{
		dir:       dir,
		idle:      autoIdle(),
		sig:       sig,
		changedAt: time.Now(),
		pending:   strings.TrimSpace(sig) != "",
	}
	b.autoMu.Unlock()

	b.autoOnce.Do(func() { go b.autoLoop() })
	return nil
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
	raw, err := runGit(r.dir, "status", "--porcelain=v1", "-uall")
	idle := autoIdle()

	b.autoMu.Lock()
	defer b.autoMu.Unlock()
	// El umbral se refresca en cada pasada: cambiarlo en Opciones aplica al
	// siguiente tick sin reiniciar el vigilante.
	r.idle = idle
	if r.busy {
		return
	}
	if err != nil && strings.TrimSpace(raw) == "" {
		r.lastErr = "git status: " + err.Error()
		return
	}
	if strings.TrimSpace(raw) == "" {
		r.sig = ""
		r.changedAt = time.Time{}
		r.pending = false
		r.lastErr = ""
		return
	}
	// Cualquier cambio nuevo reinicia el reloj: el commit solo ocurre cuando
	// el arbol lleva un rato quieto, no a mitad de una edicion.
	if raw != r.sig {
		r.sig = raw
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
	if hasConflict(raw) {
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

// hasConflict detecta codigos de conflicto de `git status --porcelain`.
func hasConflict(porcelain string) bool {
	for _, line := range strings.Split(porcelain, "\n") {
		if len(line) < 2 {
			continue
		}
		switch line[:2] {
		case "UU", "AA", "DD", "AU", "UA", "DU", "UD":
			return true
		}
	}
	return false
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}
