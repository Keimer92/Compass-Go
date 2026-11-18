// Package container implementa las pestanas de terminal incrustadas: cada card
// del menu abre una sesion ConPTY que corre el mismo CLI que antes abria una
// ventana externa, y su salida se pinta dentro de esta ventana.
//
// Es el equivalente de src/container.rs. La diferencia de motor es el parser:
// Rust usa vt100 (con scrollback) y aqui vt10x, que emula la pantalla visible.
package container

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"

	"compass/internal/shell"

	"github.com/aymanbagabas/go-pty"
	"github.com/hinshun/vt10x"
)

// ExitCloseTab es el codigo con el que una tarea pide cerrar su pestana.
const ExitCloseTab = shell.ExitCloseTab

// Session es una sesion ConPTY con su parser de salida.
type Session struct {
	term vt10x.Terminal
	pty  pty.Pty
	pid  int

	// sb es el scrollback de la pestana; su terminal "alto" se alimenta con
	// la misma salida que el parser vivo para conservar la historia.
	sb *Scrollback

	exited atomic.Bool
	// code es el codigo de salida del shell; -1 mientras sigue vivo.
	code atomic.Int32

	// done se cierra cuando el proceso termina; permite esperar su salida.
	done chan struct{}

	mu         sync.Mutex
	rows, cols int
}

// Spawn arranca `script` con el shell activo dentro de una ConPTY nueva.
//
// La sesion hereda COMPASS_EMBEDDED=1: con esa marca las tareas que lanzan
// procesos interactivos los ejecutan en esta misma consola en vez de abrir
// otra ventana. sb es el scrollback de la pestana (su terminal "alto" se
// alimenta con la misma salida para conservar la historia).
func Spawn(script, dir string, rows, cols int, sb *Scrollback) (*Session, error) {
	return SpawnTap(script, dir, rows, cols, sb, nil)
}

// SpawnTap es Spawn con un consumidor adicional de la salida cruda.
//
// tap recibe cada trozo leido de la ConPTY (para reenviarlo a un frontend
// web). Es imprescindible que la salida se reparta desde el mismo lector que
// alimenta el parser: dos lectores sobre la misma ConPTY se robarian bytes.
func SpawnTap(script, dir string, rows, cols int, sb *Scrollback, tap func([]byte)) (*Session, error) {
	if rows < 2 {
		rows = 2
	}
	if cols < 10 {
		cols = 10
	}

	p, err := pty.New()
	if err != nil {
		return nil, fmt.Errorf("no se pudo crear la sesion de terminal: %w", err)
	}
	// go-pty crea la ConPTY a 80x25: hay que fijar el tamano antes de arrancar
	// o el hijo pinta su primer fotograma con la geometria equivocada.
	_ = p.Resize(cols, rows)

	cmd := p.Command(shellExe(), "-NoLogo", "-NoProfile", "-Command", script)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "COMPASS_EMBEDDED=1")

	if err := cmd.Start(); err != nil {
		_ = p.Close()
		return nil, fmt.Errorf("no se pudo iniciar el shell de la pestana: %w", err)
	}

	s := &Session{
		term: vt10x.New(vt10x.WithSize(cols, rows)),
		pty:  p,
		rows: rows,
		cols: cols,
		sb:   sb,
		done: make(chan struct{}),
	}
	s.code.Store(-1)
	if cmd.Process != nil {
		s.pid = cmd.Process.Pid
	}

	// La salida se parsea en segundo plano; la pantalla queda protegida por el
	// candado del propio emulador. El mismo flujo alimenta el terminal "alto"
	// del scrollback para conservar la historia.
	go func() {
		br := bufio.NewReaderSize(p, 8192)
		buf := make([]byte, 8192)
		for {
			n, err := br.Read(buf)
			if n > 0 {
				s.term.Write(buf[:n])
				if s.sb != nil {
					s.sb.Feed(buf[:n])
				}
				if tap != nil {
					chunk := make([]byte, n)
					copy(chunk, buf[:n])
					tap(chunk)
				}
			}
			if err != nil {
				s.exited.Store(true)
				return
			}
		}
	}()

	// La ConPTY no cierra el pipe de salida mientras el master siga abierto,
	// asi que el lector no ve EOF al terminar la tarea: el codigo se pregunta
	// esperando al proceso, que es lo que decide si la pestana se cierra sola.
	go func() {
		_ = cmd.Wait()
		if cmd.ProcessState != nil {
			s.code.Store(int32(cmd.ProcessState.ExitCode()))
		}
		s.exited.Store(true)
		close(s.done)
	}()

	return s, nil
}

// shellExe resuelve la ruta absoluta del shell activo.
//
// go-pty resuelve el ejecutable relativo al directorio de trabajo cuando el
// nombre no trae ruta, asi que con `Dir` puesto un "pwsh" pelado no se
// encontraria: hay que pasarle la ruta completa.
func shellExe() string {
	if p, err := exec.LookPath(shell.Shell()); err == nil {
		return p
	}
	return shell.ShellPath()
}

// WithScreen da acceso a la pantalla parseada con el candado tomado.
func (s *Session) WithScreen(f func(vt10x.View)) {
	s.term.Lock()
	defer s.term.Unlock()
	f(s.term)
}

// ExitCode devuelve el codigo de salida del proceso (o -1 si sigue vivo).
func (s *Session) ExitCode() int { return int(s.code.Load()) }

// WaitExit espera a que el proceso termine y devuelve su codigo de salida.
func (s *Session) WaitExit() int {
	<-s.done
	return s.ExitCode()
}

// Write reenvia bytes al proceso de la pestana.
func (s *Session) Write(b []byte) {
	if s.exited.Load() {
		return
	}
	_, _ = s.pty.Write(b)
}

// Text devuelve el contenido visible de la pantalla (para pruebas y trazas).
//
// Se arma celda a celda en vez de usar View.String(): String toma el candado
// del emulador por dentro y dentro de WithScreen ya esta tomado.
func (s *Session) Text() string {
	var b strings.Builder
	s.WithScreen(func(v vt10x.View) {
		cols, rows := v.Size()
		for y := 0; y < rows; y++ {
			for x := 0; x < cols; x++ {
				ch := v.Cell(x, y).Char
				if ch == 0 {
					ch = ' '
				}
				b.WriteRune(ch)
			}
			b.WriteByte('\n')
		}
	})
	return b.String()
}

// Resize propaga el tamano al parser y a la ConPTY. Los dos tienen que ir
// juntos: el parser decide donde corta cada linea y la ConPTY hasta donde
// puede escribir el hijo.
func (s *Session) Resize(cols, rows int) {
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	s.mu.Lock()
	if s.cols == cols && s.rows == rows {
		s.mu.Unlock()
		return
	}
	s.cols, s.rows = cols, rows
	s.mu.Unlock()

	s.term.Resize(cols, rows)
	if s.sb != nil {
		s.sb.Resize(cols, rows)
	}
	_ = s.pty.Resize(cols, rows)
}

// Exited indica si el shell de la pestana ya termino.
func (s *Session) Exited() bool { return s.exited.Load() }

// CloseRequested indica que la tarea termino pidiendo cerrar su pestana (el
// codigo ExitCloseTab de ui.Pause tras agotar la cuenta atras).
func (s *Session) CloseRequested() bool {
	return s.exited.Load() && s.code.Load() == int32(ExitCloseTab)
}

// Pid es el PID del shell de la pestana (0 si el sistema no lo dio).
func (s *Session) Pid() int { return s.pid }

// Close mata el arbol de procesos de la pestana y libera la ConPTY.
//
// taskkill /T hace falta porque matar solo al shell deja vivos a sus nietos
// (compass --task, cargo, gradlew, flutter...), que seguirian escribiendo en
// una ConPTY que ya no existe.
func (s *Session) Close() {
	killTree(s.pid)
	_ = s.pty.Close()
}
