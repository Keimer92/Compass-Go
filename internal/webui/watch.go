package webui

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Vigilancia del arbol de trabajo.
//
// El panel Git necesita enterarse de un cambio EN CUANTO se guarda el archivo:
// el sondeo cada 15 s de autoStep llega tarde (el contador de inactividad
// arranca hasta 15 s despues del guardado y el panel muestra un estado viejo).
// Aqui se escucha el sistema de archivos con fsnotify, se agrupan las rafagas
// de eventos (un guardado suele producir varios) y se calcula la misma firma
// que usa el auto-commit.
//
// El ticker de autoStep se conserva como red de seguridad: si un evento se
// pierde (limite de handles, unidad de red), el commit automatico sigue
// funcionando, solo con algo de retraso.

const (
	// watchDebounce agrupa la rafaga de eventos de un mismo guardado.
	watchDebounce = 200 * time.Millisecond
	// watchMaxDirs acota los handles abiertos: un repo con node_modules puede
	// tener decenas de miles de carpetas.
	watchMaxDirs = 3000
)

// watchSkip son carpetas que no producen cambios de git utiles y si mucho
// ruido (o miles de handles). Se excluyen del arbol vigilado.
var watchSkip = map[string]bool{
	".git":         true,
	"node_modules": true,
	"dist":         true,
	"build":        true,
	"vendor":       true,
	"target":       true,
	".next":        true,
	".nuxt":        true,
	".svelte-kit":  true,
	".cache":       true,
	"__pycache__":  true,
	".venv":        true,
	"venv":         true,
	"bin":          true,
	"obj":          true,
	"coverage":     true,
	".gradle":      true,
	".dart_tool":   true,
}

// dirWatch es el vigilante de UN proyecto.
type dirWatch struct {
	dir  string
	fsw  *fsnotify.Watcher
	stop chan struct{}

	// lastSig evita avisar al frontend cuando el evento no cambia nada que
	// git vea (p.ej. escribir un archivo ignorado).
	mu      sync.Mutex
	lastSig string
}

// ensureWatch deja un vigilante activo para dir. Es idempotente y silencioso:
// si la carpeta no es un repo git o el sistema no deja vigilar, no hace nada.
func (b *Bridge) ensureWatch(dir string) {
	if strings.TrimSpace(dir) == "" {
		return
	}
	key := commitKey(dir)

	b.watchMu.Lock()
	if _, ok := b.watches[key]; ok {
		b.watchMu.Unlock()
		return
	}
	if b.watches == nil {
		b.watches = map[string]*dirWatch{}
	}
	b.watchMu.Unlock()

	// Solo tiene sentido vigilar arboles de trabajo de git.
	if out, err := runGit(dir, "rev-parse", "--is-inside-work-tree"); err != nil || strings.TrimSpace(out) != "true" {
		return
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	dw := &dirWatch{dir: dir, fsw: w, stop: make(chan struct{})}
	if addWatchTree(w, dir) == 0 {
		_ = w.Close()
		return
	}
	sig, _, _ := autoSig(dir)
	dw.lastSig = sig

	b.watchMu.Lock()
	if _, ok := b.watches[key]; ok { // otra llamada gano la carrera
		b.watchMu.Unlock()
		_ = w.Close()
		return
	}
	b.watches[key] = dw
	b.watchMu.Unlock()

	go b.watchLoop(dw)
}

// stopWatch cierra el vigilante de dir (proyecto retirado del workspace).
func (b *Bridge) stopWatch(dir string) {
	key := commitKey(dir)
	b.watchMu.Lock()
	dw := b.watches[key]
	delete(b.watches, key)
	b.watchMu.Unlock()
	if dw == nil {
		return
	}
	close(dw.stop)
	_ = dw.fsw.Close()
}

// watchForget olvida la ultima firma de dir. Se llama tras un commit: git solo
// toco .git (excluido del vigilante), asi que la firma guardada quedaria vieja
// y el siguiente guardado no avisaria al panel.
func (b *Bridge) watchForget(dir string) {
	b.watchMu.Lock()
	dw := b.watches[commitKey(dir)]
	b.watchMu.Unlock()
	if dw == nil {
		return
	}
	dw.mu.Lock()
	dw.lastSig = ""
	dw.mu.Unlock()
}

// addWatchTree registra dir y todas sus subcarpetas utiles.
func addWatchTree(w *fsnotify.Watcher, root string) int {
	n := 0
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if p != root && watchSkip[d.Name()] {
			return filepath.SkipDir
		}
		if n >= watchMaxDirs {
			return filepath.SkipAll
		}
		if w.Add(p) == nil {
			n++
		}
		return nil
	})
	return n
}

func (b *Bridge) watchLoop(dw *dirWatch) {
	var timer *time.Timer
	var timerC <-chan time.Time
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	for {
		select {
		case <-dw.stop:
			return
		case ev, ok := <-dw.fsw.Events:
			if !ok {
				return
			}
			// Las carpetas nuevas hay que vigilarlas aparte: fsnotify no es
			// recursivo por si solo.
			if ev.Op&fsnotify.Create != 0 {
				if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() {
					addWatchTree(dw.fsw, ev.Name)
				}
			}
			if watchIgnored(dw.dir, ev.Name) {
				continue
			}
			if timer == nil {
				timer = time.NewTimer(watchDebounce)
			} else {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(watchDebounce)
			}
			timerC = timer.C
		case <-dw.fsw.Errors:
			// Errores del vigilante (buffer desbordado, ruta borrada): el
			// ticker de autoStep sigue cubriendo el caso.
			continue
		case <-timerC:
			timerC = nil
			b.watchFlush(dw)
		}
	}
}

// watchFlush recalcula la firma tras una rafaga y avisa a la UI solo si el
// arbol cambio de verdad.
func (b *Bridge) watchFlush(dw *dirWatch) {
	sig, conflict, err := autoSig(dw.dir)
	if err != nil && strings.TrimSpace(sig) == "" {
		return
	}
	dw.mu.Lock()
	changed := sig != dw.lastSig
	dw.lastSig = sig
	dw.mu.Unlock()
	if !changed {
		return
	}
	b.applySig(dw.dir, sig)
	Emit("git:changed", map[string]any{
		"dir":      dw.dir,
		"changes":  countSig(sig),
		"conflict": conflict,
	})
}

// applySig adelanta el estado del auto-commit con lo que acaba de ver el
// vigilante: el reloj de inactividad empieza a contar en el momento del
// guardado, no en el siguiente tick.
func (b *Bridge) applySig(dir, sig string) {
	b.autoMu.Lock()
	defer b.autoMu.Unlock()
	r := b.autoRuns[commitKey(dir)]
	if r == nil || r.busy {
		return
	}
	if strings.TrimSpace(sig) == "" {
		r.sig = ""
		r.changedAt = time.Time{}
		r.pending = false
		r.lastErr = ""
		return
	}
	if sig != r.sig {
		r.sig = sig
		r.changedAt = time.Now()
		r.pending = true
		r.lastErr = ""
	}
}

// countSig cuenta los archivos de la firma de autoSig.
func countSig(sig string) int {
	if strings.TrimSpace(sig) == "" {
		return 0
	}
	return strings.Count(sig, "\n")
}

// watchIgnored filtra rutas que git no veria o que son ruido de editores.
func watchIgnored(root, name string) bool {
	rel, err := filepath.Rel(root, name)
	if err != nil {
		return true
	}
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if watchSkip[part] {
			return true
		}
	}
	base := filepath.Base(name)
	switch {
	case strings.HasSuffix(base, ".swp"),
		strings.HasSuffix(base, ".tmp"),
		strings.HasPrefix(base, ".~"),
		base == "4913":
		return true
	}
	return false
}
