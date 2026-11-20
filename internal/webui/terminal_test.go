package webui

import (
	"encoding/base64"
	"strings"
	"testing"
)

// TestListReportsSessionsInOrder comprueba que la UI puede reconstruir sus
// pestanas tras una recarga: mismo id, etiqueta, proyecto y salida.
func TestListReportsSessionsInOrder(t *testing.T) {
	svc := NewTerminalService()
	svc.mu.Lock()
	svc.sessions["tab-1"] = &termSession{label: "Windows Build", project: `C:\proj`, buf: []byte("hola")}
	svc.sessions["tab-2"] = &termSession{label: "Git", project: `C:\proj`, exited: true, buf: []byte("adios")}
	svc.order = []string{"tab-1", "tab-2"}
	svc.mu.Unlock()

	got := svc.List()
	if len(got) != 2 {
		t.Fatalf("List devolvio %d sesiones, esperaba 2", len(got))
	}
	if got[0].ID != "tab-1" || got[0].Label != "Windows Build" || got[0].Project != `C:\proj` {
		t.Fatalf("cabecera inesperada: %+v", got[0])
	}
	if got[0].Exited {
		t.Fatal("la primera sesion no esta terminada")
	}
	raw, err := base64.StdEncoding.DecodeString(got[0].Output)
	if err != nil {
		t.Fatalf("output no es base64: %v", err)
	}
	if string(raw) != "hola" {
		t.Fatalf("output = %q, esperaba %q", raw, "hola")
	}
	if !got[1].Exited {
		t.Fatal("la segunda sesion deberia estar terminada")
	}
}

// TestListSkipsForgottenSessions protege el caso de un id en `order` que ya no
// existe en el mapa (cerrado mientras la UI estaba recargando).
func TestListSkipsForgottenSessions(t *testing.T) {
	svc := NewTerminalService()
	svc.mu.Lock()
	svc.order = []string{"fantasma"}
	svc.mu.Unlock()

	if got := svc.List(); len(got) != 0 {
		t.Fatalf("List devolvio %d sesiones, esperaba 0", len(got))
	}
}

// TestAppendReplayKeepsTailAndAligns comprueba que el bufer de repintado se
// recorta por el principio sin partir lineas ni secuencias de escape.
func TestAppendReplayKeepsTailAndAligns(t *testing.T) {
	line := strings.Repeat("x", 99) + "\n"
	var buf []byte
	for i := 0; i < 5000; i++ {
		buf = appendReplay(buf, []byte(line))
	}
	if len(buf) > maxReplayBytes {
		t.Fatalf("el bufer crecio a %d bytes (maximo %d)", len(buf), maxReplayBytes)
	}
	if len(buf)%len(line) != 0 {
		t.Fatalf("el recorte partio una linea: %d bytes no es multiplo de %d", len(buf), len(line))
	}
	if !strings.HasPrefix(string(buf), "x") || !strings.HasSuffix(string(buf), "\n") {
		t.Fatal("el bufer no empieza en una linea completa o no termina en salto")
	}
}

// TestAppendReplayAcceptsHugeChunk evita que un unico chunk mayor que el tope
// haga crecer el bufer sin limite.
func TestAppendReplayAcceptsHugeChunk(t *testing.T) {
	huge := make([]byte, maxReplayBytes*2)
	buf := appendReplay(nil, huge)
	if len(buf) != maxReplayBytes {
		t.Fatalf("bufer = %d bytes, esperaba %d", len(buf), maxReplayBytes)
	}
}
