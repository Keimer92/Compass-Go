package webui

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"compass/internal/container"
)

func tlog(format string, args ...any) {
	f, err := os.OpenFile(filepath.Join(os.TempDir(), "compass-web-terminal.log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, format+"\n", args...)
}

// TerminalService gestiona las ConPTYs abiertas desde el frontend.
type TerminalService struct {
	mu       sync.Mutex
	sessions map[string]*termSession
	next     int
}

type termSession struct {
	sess *container.Session
}

// NewTerminalService crea el servicio.
func NewTerminalService() *TerminalService {
	return &TerminalService{sessions: make(map[string]*termSession)}
}

// OpenSession arranca una ConPTY con el script dado y devuelve su ID.
//
// El ID lo elige el frontend (el de su pestana) para que los eventos
// `pty:data` se puedan casar desde el primer byte: si el id se asignara aqui,
// la salida que llega antes de que el JS reciba la respuesta se perderia.
func (t *TerminalService) OpenSession(id, script, dir string, rows, cols int) string {
	if rows < 4 {
		rows = 24
	}
	if cols < 10 {
		cols = 80
	}
	back := container.NewScrollback(cols, rows)

	if id == "" {
		t.mu.Lock()
		t.next++
		id = fmt.Sprintf("pty-%d", t.next)
		t.mu.Unlock()
	}

	sess, err := container.SpawnTap(script, dir, rows, cols, back, func(chunk []byte) {
		Emit("pty:data", map[string]any{"id": id, "data": Encode(chunk)})
	})
	tlog("OpenSession id=%s script=%q dir=%q rows=%d cols=%d err=%v", id, script, dir, rows, cols, err)
	if err != nil {
		return ""
	}

	t.mu.Lock()
	t.sessions[id] = &termSession{sess: sess}
	t.mu.Unlock()

	go func() {
		code := sess.WaitExit()
		tlog("pty %s exit code=%d", id, code)
		Emit("pty:exit", map[string]any{"id": id, "code": code})
		t.mu.Lock()
		delete(t.sessions, id)
		t.mu.Unlock()
	}()
	return id
}

// Write envia bytes del frontend a la ConPTY.
func (t *TerminalService) Write(id string, data []byte) {
	t.mu.Lock()
	s := t.sessions[id]
	t.mu.Unlock()
	if s != nil {
		s.sess.Write(data)
	}
}

// Resize cambia el tamano de la ConPTY.
func (t *TerminalService) Resize(id string, cols, rows int) {
	t.mu.Lock()
	s := t.sessions[id]
	t.mu.Unlock()
	if s != nil {
		s.sess.Resize(cols, rows)
	}
}

// Close mata la sesion y la olvida.
func (t *TerminalService) Close(id string) {
	t.mu.Lock()
	s := t.sessions[id]
	delete(t.sessions, id)
	t.mu.Unlock()
	if s != nil {
		s.sess.Close()
	}
}
