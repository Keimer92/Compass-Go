package webui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"compass/internal/settings"
)

// El panel Git debe enterarse de un guardado sin esperar al ticker de 15 s:
// el vigilante de fsnotify adelanta la firma del auto-commit al momento.
func TestWatchDetectsChangeInstantly(t *testing.T) {
	dir := initRepo(t)
	cfg := settings.Default()
	cfg.AutoCommitIdleMin = 60 // que no commitee durante el test
	settings.Save(cfg)

	b := NewBridge(dir)
	if err := b.SetAutoCommit(true); err != nil {
		t.Fatalf("activar Auto: %v", err)
	}
	defer b.stopWatch(dir)

	b.autoMu.Lock()
	r := b.autoRuns[commitKey(dir)]
	b.autoMu.Unlock()
	if r == nil {
		t.Fatal("el vigilante de Auto no quedo armado")
	}

	b.autoMu.Lock()
	before := r.changedAt
	b.autoMu.Unlock()

	// Un guardado normal: crear el archivo y escribir.
	if err := os.WriteFile(filepath.Join(dir, "nuevo.txt"), []byte("hola\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		b.autoMu.Lock()
		pending, changedAt, sig := r.pending, r.changedAt, r.sig
		b.autoMu.Unlock()
		if pending && sig != "" && changedAt.After(before) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("el vigilante no detecto el cambio a tiempo")
}

// Escribir en una carpeta excluida (node_modules, dist...) no debe generar
// actividad: son miles de eventos de build que no son cambios de git.
func TestWatchIgnoresExcludedDirs(t *testing.T) {
	if !watchIgnored(`C:\proj`, `C:\proj\node_modules\pkg\index.js`) {
		t.Fatal("node_modules deberia ignorarse")
	}
	if !watchIgnored(`C:\proj`, `C:\proj\dist\app.exe`) {
		t.Fatal("dist deberia ignorarse")
	}
	if !watchIgnored(`C:\proj`, `C:\proj\.git\index`) {
		t.Fatal(".git deberia ignorarse")
	}
	if watchIgnored(`C:\proj`, `C:\proj\src\main.go`) {
		t.Fatal("un archivo normal no deberia ignorarse")
	}
}
