package webui

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"compass/internal/container"
	"compass/internal/sound"
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

// maxReplayBytes es la salida que se guarda por sesion para repintar la
// pestana cuando la UI se recarga (HMR en desarrollo). xterm tiene su propio
// scrollback de 5000 lineas; aqui basta con lo ultimo que se vio.
const maxReplayBytes = 256 << 10

// TerminalService gestiona las ConPTYs abiertas desde el frontend.
//
// Las sesiones sobreviven a una recarga de la pagina: en modo desarrollo Vite
// recarga el WebView en cada cambio de .svelte, y matar los procesos en cada
// guardado haria inservible el ciclo. El frontend, al montar, pide List() y
// reconstruye sus pestanas con el mismo id.
type TerminalService struct {
	mu       sync.Mutex
	sessions map[string]*termSession
	order    []string
	next     int
}

type termSession struct {
	sess    *container.Session
	label   string
	project string
	script  string
	rows    int
	cols    int
	exited  bool
	buf     []byte
}

// TerminalInfo describe una sesion viva para que la UI pueda reengancharse.
type TerminalInfo struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Project string `json:"project"`
	Exited  bool   `json:"exited"`
	Output  string `json:"output"` // base64 de la salida reciente
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
func (t *TerminalService) OpenSession(id, label, project, script, dir string, rows, cols int) string {
	if rows < 4 {
		rows = 24
	}
	if cols < 10 {
		cols = 80
	}
	back := container.NewScrollback(cols, rows)

	t.mu.Lock()
	if id == "" {
		t.next++
		id = fmt.Sprintf("pty-%d", t.next)
	}
	if prev := t.sessions[id]; prev != nil {
		// Un id repetido (p. ej. tras recargar con la misma etiqueta) se
		// reemplaza: la sesion vieja se cierra para no dejar dos ConPTY
		// escribiendo en la misma pestana.
		delete(t.sessions, id)
		go prev.sess.Close()
	}
	s := &termSession{label: label, project: project, script: script, rows: rows, cols: cols}
	t.sessions[id] = s
	t.order = append(t.order, id)
	t.mu.Unlock()

	sess, err := container.SpawnTap(script, dir, rows, cols, back, func(chunk []byte) {
		t.mu.Lock()
		s.buf = appendReplay(s.buf, chunk)
		t.mu.Unlock()
		Emit("pty:data", map[string]any{"id": id, "data": Encode(chunk)})
	})
	tlog("OpenSession id=%s script=%q dir=%q rows=%d cols=%d err=%v", id, script, dir, rows, cols, err)
	if err != nil {
		t.mu.Lock()
		delete(t.sessions, id)
		t.mu.Unlock()
		return ""
	}

	t.mu.Lock()
	s.sess = sess
	targetCols, targetRows := s.cols, s.rows
	t.mu.Unlock()
	if targetCols != cols || targetRows != rows {
		sess.Resize(targetCols, targetRows)
	}

	go func() {
		code := sess.WaitExit()
		tlog("pty %s exit code=%d", id, code)
		// La sesion NO se borra: si la UI se recarga despues de que el proceso
		// termine, la pestana se reconstruye con su salida y el aviso de fin.
		t.mu.Lock()
		if cur := t.sessions[id]; cur == s {
			s.exited = true
		}
		t.mu.Unlock()
		Emit("pty:exit", map[string]any{"id": id, "code": code})
		// Aviso de fin de tarea. Un codigo negativo es un cierre forzado
		// (cerrar la pestana) y no debe sonar como fallo.
		switch {
		case code == 0:
			sound.Play(sound.EventTaskOK)
		case code > 0:
			sound.Play(sound.EventTaskError)
		}
	}()
	return id
}

// appendReplay acumula salida recortando por el principio. El corte se alinea
// con un salto de linea para no dejar una secuencia de escape partida.
func appendReplay(buf, chunk []byte) []byte {
	if len(chunk) >= maxReplayBytes {
		return append(buf[:0], chunk[len(chunk)-maxReplayBytes:]...)
	}
	buf = append(buf, chunk...)
	if len(buf) <= maxReplayBytes {
		return buf
	}
	cut := len(buf) - maxReplayBytes
	if i := indexByteFrom(buf, '\n', cut); i >= 0 && i+1 < len(buf) {
		cut = i + 1
	}
	return append(buf[:0], buf[cut:]...)
}

func indexByteFrom(b []byte, c byte, from int) int {
	for i := from; i < len(b); i++ {
		if b[i] == c {
			return i
		}
	}
	return -1
}

// List devuelve las sesiones en orden de creacion, con su salida reciente.
func (t *TerminalService) List() []TerminalInfo {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]TerminalInfo, 0, len(t.order))
	for _, id := range t.order {
		s := t.sessions[id]
		if s == nil {
			continue
		}
		out = append(out, TerminalInfo{
			ID:      id,
			Label:   s.label,
			Project: s.project,
			Exited:  s.exited,
			Output:  base64.StdEncoding.EncodeToString(s.buf),
		})
	}
	return out
}

// Write envia bytes del frontend a la ConPTY.
func (t *TerminalService) Write(id string, data []byte) {
	t.mu.Lock()
	s := t.sessions[id]
	var sess *container.Session
	if s != nil {
		sess = s.sess
	}
	t.mu.Unlock()
	if sess != nil {
		sess.Write(data)
	}
}

// Resize cambia el tamano de la ConPTY.
func (t *TerminalService) Resize(id string, cols, rows int) {
	t.mu.Lock()
	s := t.sessions[id]
	var sess *container.Session
	if s != nil {
		s.rows, s.cols = rows, cols
		sess = s.sess
	}
	t.mu.Unlock()
	if sess != nil {
		sess.Resize(cols, rows)
	}
}

// Close mata la sesion y la olvida.
func (t *TerminalService) Close(id string) {
	t.mu.Lock()
	s := t.sessions[id]
	delete(t.sessions, id)
	for i, v := range t.order {
		if v == id {
			t.order = append(t.order[:i], t.order[i+1:]...)
			break
		}
	}
	var sess *container.Session
	if s != nil {
		sess = s.sess
	}
	t.mu.Unlock()
	if sess != nil {
		sess.Close()
	}
}
