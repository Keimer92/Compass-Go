package whatsapp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type captured struct {
	name string
	data any
}

func newRecorder(t *testing.T) (*Supervisor, *[]captured) {
	t.Helper()
	// Evita escribir whatsapp.log en el AppData real durante las pruebas.
	useTestData(t, t.TempDir())

	var mu sync.Mutex
	events := []captured{}
	sup := New(func(name string, data any) {
		mu.Lock()
		events = append(events, captured{name: name, data: data})
		mu.Unlock()
	})
	return sup, &events
}

func lastStatus(t *testing.T, events *[]captured) *Status {
	t.Helper()
	for i := len(*events) - 1; i >= 0; i-- {
		if (*events)[i].name == "whatsapp-status" {
			status, ok := (*events)[i].data.(*Status)
			if !ok {
				t.Fatalf("payload de whatsapp-status no es *WhatsappStatus: %#v", (*events)[i].data)
			}
			return status
		}
	}
	t.Fatal("no se emitió whatsapp-status")
	return nil
}

func lastOutbound(t *testing.T, events *[]captured) map[string]any {
	t.Helper()
	for i := len(*events) - 1; i >= 0; i-- {
		if (*events)[i].name == "whatsapp-outbound-message" {
			payload, ok := (*events)[i].data.(map[string]any)
			if !ok {
				t.Fatalf("payload de outbound no es map: %#v", (*events)[i].data)
			}
			return payload
		}
	}
	t.Fatal("no se emitió whatsapp-outbound-message")
	return nil
}

func TestStatusFromLineCodes(t *testing.T) {
	cases := []struct {
		content    string
		wantStatus string
		wantMsg    string
		wantInfo   *string
	}{
		{"init", "connecting", "⚙️ Iniciando servicios...", nil},
		{"starting", "connecting", "🌐 Cargando módulos...", nil},
		{"auth_ready", "connecting", "🔐 Preparando sesión...", nil},
		{"socket_creating", "connecting", "📡 Creando túnel seguro...", nil},
		{"connecting_socket", "connecting", "🛰️ Conectando con servidores...", nil},
		{"restarting", "connecting", "🔄 Reintentando en 5s...", nil},
		{"session_invalid", "disconnected", "❌ Sesión expirada o cerrada. Limpiando...", nil},
		{"session_cleared", "disconnected", "🧹 Sesión limpiada. Escanea el código QR de nuevo.", nil},
		{"connected:+50588888888@s.whatsapp.net", "connected", "✓ WhatsApp Conectado", strPtr("+50588888888@s.whatsapp.net")},
		{"disconnected", "disconnected", "Sesión interrumpida", nil},
		{"disconnected:Replaced by other login", "disconnected", "Replaced by other login", nil},
		{"error:Boom", "disconnected", "Boom", nil},
		{"desconocido", "connecting", "⚙️ Cargando...", nil},
	}
	for _, c := range cases {
		status, msg, info := statusFromLine(c.content)
		if status != c.wantStatus || msg != c.wantMsg {
			t.Errorf("statusFromLine(%q) = (%q, %q), queremos (%q, %q)", c.content, status, msg, c.wantStatus, c.wantMsg)
		}
		switch {
		case c.wantInfo == nil && info != nil:
			t.Errorf("statusFromLine(%q) clientInfo = %q, queremos nil", c.content, *info)
		case c.wantInfo != nil && info == nil:
			t.Errorf("statusFromLine(%q) clientInfo = nil, queremos %q", c.content, *c.wantInfo)
		case c.wantInfo != nil && *info != *c.wantInfo:
			t.Errorf("statusFromLine(%q) clientInfo = %q, queremos %q", c.content, *info, *c.wantInfo)
		}
	}
}

func TestHandleMotorLineStatuses(t *testing.T) {
	sup, events := newRecorder(t)

	sup.handleMotorLine("QR_DATA_URL:data:image/png;base64,AAAA")
	status := lastStatus(t, events)
	if status.Status != "qr" || status.QR == nil || *status.QR != "data:image/png;base64,AAAA" {
		t.Fatalf("QR_DATA_URL mal traducido: %+v", status)
	}
	if status.Message == nil || *status.Message != "✓ ¡Listo! Escanea el código QR" {
		t.Fatalf("mensaje de QR inesperado: %+v", status.Message)
	}

	sup.handleMotorLine("QR:https://example.com/qr")
	status = lastStatus(t, events)
	if status.Status != "qr" || status.QR == nil || *status.QR != "https://example.com/qr" {
		t.Fatalf("QR mal traducido: %+v", status)
	}

	sup.handleMotorLine("QR_ERROR:socket closed")
	status = lastStatus(t, events)
	if status.Status != "disconnected" || status.QR != nil || status.Message == nil ||
		*status.Message != "❌ Error generando QR: socket closed" {
		t.Fatalf("QR_ERROR mal traducido: %+v", status)
	}

	sup.handleMotorLine("STATUS:connected:+50588888888@s.whatsapp.net")
	status = lastStatus(t, events)
	if status.Status != "connected" || status.ClientInfo == nil ||
		*status.ClientInfo != "+50588888888@s.whatsapp.net" {
		t.Fatalf("STATUS connected mal traducido: %+v", status)
	}

	sup.handleMotorLine("STATUS:disconnected")
	status = lastStatus(t, events)
	if status.Status != "disconnected" || *status.Message != "Sesión interrumpida" {
		t.Fatalf("STATUS disconnected mal traducido: %+v", status)
	}
}

func TestHandleMotorLineOutboundEvents(t *testing.T) {
	sup, events := newRecorder(t)

	sup.handleMotorLine("SENDING:50588888888")
	payload := lastOutbound(t, events)
	if payload["status"] != "sending" || payload["type"] != "text" || payload["number"] != "50588888888" {
		t.Fatalf("SENDING -> %+v", payload)
	}

	sup.handleMotorLine("SENT_CONFIRM:50588888888")
	payload = lastOutbound(t, events)
	if payload["status"] != "sent" || payload["type"] != "text" || payload["number"] != "50588888888" {
		t.Fatalf("SENT_CONFIRM -> %+v", payload)
	}

	// Equivalente a split(':').nth(1): se conserva solo el segundo campo.
	sup.handleMotorLine("SENT_RETRY_OK:50588888888:extra:noise")
	payload = lastOutbound(t, events)
	if payload["number"] != "50588888888" {
		t.Fatalf("SENT_RETRY_OK number = %v", payload["number"])
	}

	sup.handleMotorLine("IMAGE_SENT_CONFIRM:50588888888")
	payload = lastOutbound(t, events)
	if payload["status"] != "sent" || payload["type"] != "image" {
		t.Fatalf("IMAGE_SENT_CONFIRM -> %+v", payload)
	}

	sup.handleMotorLine("IMAGE_ERROR:Sin confirmación")
	payload = lastOutbound(t, events)
	if payload["status"] != "error" || payload["type"] != "image" || payload["message"] != "Sin confirmación" {
		t.Fatalf("IMAGE_ERROR -> %+v", payload)
	}

	sup.handleMotorLine("IMAGE_SILENT_BLOCK:50588888888")
	payload = lastOutbound(t, events)
	if payload["status"] != "error" || payload["type"] != "image" ||
		!strings.Contains(payload["message"].(string), "50588888888") {
		t.Fatalf("IMAGE_SILENT_BLOCK -> %+v", payload)
	}

	sup.handleMotorLine("SILENT_BLOCK:50588888888")
	payload = lastOutbound(t, events)
	if payload["type"] != "text" || payload["status"] != "error" {
		t.Fatalf("SILENT_BLOCK -> %+v", payload)
	}

	sup.handleMotorLine("ACK_PENDING:50588888888")
	payload = lastOutbound(t, events)
	if payload["status"] != "pending_ack" || payload["number"] != "50588888888" {
		t.Fatalf("ACK_PENDING -> %+v", payload)
	}

	sup.handleMotorLine("SEND_ERROR:Socket not ready")
	payload = lastOutbound(t, events)
	if payload["status"] != "error" || payload["type"] != "text" || payload["message"] != "Socket not ready" {
		t.Fatalf("SEND_ERROR -> %+v", payload)
	}

	sup.handleMotorLine("SEND_RETRY_FAILED:retry exhausted")
	payload = lastOutbound(t, events)
	if payload["message"] != "retry exhausted" {
		t.Fatalf("SEND_RETRY_FAILED -> %+v", payload)
	}

	sup.handleMotorLine("ENGINE_ERROR:boom")
	payload = lastOutbound(t, events)
	if payload["type"] != "engine" || payload["message"] != "boom" {
		t.Fatalf("ENGINE_ERROR -> %+v", payload)
	}
}

func TestHandleMotorLineIgnoresConfigAndNoise(t *testing.T) {
	sup, events := newRecorder(t)
	sup.handleMotorLine("CONFIG_RECEIVED:ok")
	sup.handleMotorLine("   ")
	sup.handleMotorLine("alguna salida suelta del motor")
	if len(*events) != 0 {
		t.Fatalf("líneas sin contrato no deben emitir eventos: %+v", *events)
	}
}

func TestStatusIsStoredAndCloned(t *testing.T) {
	sup, _ := newRecorder(t)
	if got := sup.Status(); got == nil || got.Status != "disconnected" {
		t.Fatalf("estado inicial = %+v", got)
	}
	if got := sup.Status(); got == nil || got.Message == nil || *got.Message != "Motor no iniciado" {
		t.Fatalf("mensaje inicial = %+v", got)
	}

	sup.sendStatus("connected", "✓ WhatsApp Conectado")
	supplied := sup.Status()
	if supplied.Status != "connected" {
		t.Fatalf("status tras sendStatus = %+v", supplied)
	}
	if sup.IsLinked() != true {
		t.Fatal("IsLinked debería ser true con status connected")
	}
	// La copia no debe compartir el puntero con el estado interno.
	*supplied.Message = "mutado"
	if again := sup.Status(); *again.Message != "✓ WhatsApp Conectado" {
		t.Fatalf("currentStatus no clonó: %+v", again)
	}
}

func TestSendWithoutEngineFails(t *testing.T) {
	sup, _ := newRecorder(t)
	if _, err := sup.SendDirect("+505 8888-8888", "hola"); err == nil {
		t.Fatal("enviar sin motor debe fallar")
	} else if err.Error() != "El motor de WhatsApp no está iniciado" {
		t.Fatalf("texto de error inesperado: %q", err.Error())
	}
	if err := sup.SendImage("50588888888", "x.png", "cap"); err == nil {
		t.Fatal("enviar imagen sin motor debe fallar")
	}
}

func TestSendDirectWritesJSONLine(t *testing.T) {
	sup, events := newRecorder(t)
	sup.currentStatus()

	pipe := &capturePipe{}
	sup.mu.Lock()
	sup.stdin = pipe
	sup.mu.Unlock()

	result, err := sup.SendDirect("+505 8888-8888", "Hola {member}")
	if err != nil {
		t.Fatalf("SendDirect: %v", err)
	}
	if result != "SENDING:50588888888" {
		t.Fatalf("retorno = %q, queremos SENDING:50588888888", result)
	}

	var payload map[string]any
	if err := json.Unmarshal(bytesLastLine(pipe.buf), &payload); err != nil {
		t.Fatalf("stdin no recibió JSON: %q", pipe.buf)
	}
	if payload["type"] != "send" || payload["number"] != "50588888888" || payload["text"] != "Hola {member}" {
		t.Fatalf("payload enviado al motor = %+v", payload)
	}
	if !strings.HasSuffix(pipe.buf, "\n") {
		t.Fatal("cada comando debe terminar en salto de línea")
	}
	if len(*events) != 1 || (*events)[0].name != "whatsapp-message-sent" {
		t.Fatalf("eventos esperados: whatsapp-message-sent, got %+v", *events)
	}
}

func TestSendImageWritesJSONLine(t *testing.T) {
	sup, events := newRecorder(t)
	pipe := &capturePipe{}
	sup.mu.Lock()
	sup.stdin = pipe
	sup.mu.Unlock()

	if err := sup.SendImage("50588888888", `C:\temp\recibo.png`, "Recibo"); err != nil {
		t.Fatalf("SendImage: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(bytesLastLine(pipe.buf), &payload); err != nil {
		t.Fatalf("stdin no recibió JSON: %q", pipe.buf)
	}
	// Rust NO limpia el número en send_image: se conserva tal cual llega.
	if payload["type"] != "send_image" || payload["number"] != "50588888888" ||
		payload["imagePath"] != `C:\temp\recibo.png` || payload["caption"] != "Recibo" {
		t.Fatalf("payload = %+v", payload)
	}
	if len(*events) != 0 {
		t.Fatalf("SendImage no debe emitir whatsapp-message-sent: %+v", *events)
	}
}

func TestLogoutKillsAndClearsAuth(t *testing.T) {
	dir := t.TempDir()
	useTestData(t, dir)

	sup := New(func(string, any) {})
	authDir := filepath.Join(dir, "whatsapp_auth")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authDir, "creds.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := sup.Logout(); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := os.Stat(authDir); !os.IsNotExist(err) {
		t.Fatalf("whatsapp_auth debería haberse eliminado: %v", err)
	}
	status := sup.Status()
	if status.Status != "disconnected" || status.Message == nil || *status.Message != "Sesión cerrada" {
		t.Fatalf("estado tras logout = %+v", status)
	}
	if sup.generation != 1 {
		t.Fatalf("generation = %d, se esperaba 1 (invalida el Terminated del motor viejo)", sup.generation)
	}
}

func TestShutdownPreventsFurtherStarts(t *testing.T) {
	sup, _ := newRecorder(t)
	if err := sup.Shutdown(); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	force := true
	sup.Start(nil)
	sup.Start(&force)
	if !sup.closed || sup.starting || sup.cmd != nil {
		t.Fatalf("estado tras cierre y reinicio: closed=%v starting=%v cmd=%v", sup.closed, sup.starting, sup.cmd)
	}
	if err := sup.Shutdown(); err != nil {
		t.Fatalf("segundo Shutdown: %v", err)
	}
}

func TestRotateLogKeepsThreeHistorical(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	path := filepath.Join(dir, "whatsapp.log")

	if err := os.WriteFile(path, make([]byte, logMaxBytes+1), 0o644); err != nil {
		t.Fatal(err)
	}
	rotateLog(path)
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("no se creó el archivo rotado .1: %v", err)
	}
	if info, err := os.Stat(path); err == nil && info.Size() > 0 {
		t.Fatalf("el log activo debería estar recreatedo vacío, tiene %d bytes", info.Size())
	}
}

func TestEngineFileFindsBundleInResources(t *testing.T) {
	dir := t.TempDir()
	useTestData(t, dir)
	// El kit canónico del equipo (COMPASS_WHATSAPP_ENGINE) ganaría a la copia
	// local; el test comprueba la resolución por cwd, así que se anula.
	t.Setenv("COMPASS_WHATSAPP_ENGINE", "")
	bundle := filepath.Join(dir, "resources", "whatsapp-engine-bundle", "index.js")
	if err := os.MkdirAll(filepath.Dir(bundle), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bundle, []byte("//"), 0o644); err != nil {
		t.Fatal(err)
	}

	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	if got := findFirst("whatsapp-engine-bundle/index.js", "resources/whatsapp-engine-bundle/index.js"); got != bundle {
		t.Fatalf("findFirst = %q, queremos %q", got, bundle)
	}
	if got := findFirst("no-existe.exe"); got != "" {
		t.Fatalf("findFirst de un archivo inexistente = %q", got)
	}
}

func TestStatusFromLineKeepsExtraAfterFirstColon(t *testing.T) {
	// Rust divide solo por el PRIMER colon: el resto puede contener más dos puntos.
	_, _, info := statusFromLine("connected:a:b:c")
	if info == nil || *info != "a:b:c" {
		t.Fatalf("clientInfo = %v", info)
	}
}

type capturePipe struct {
	buf string
}

func (c *capturePipe) Write(p []byte) (int, error) {
	c.buf += string(p)
	return len(p), nil
}

func (c *capturePipe) Close() error { return nil }

func bytesLastLine(s string) []byte {
	trimmed := strings.TrimRight(s, "\n")
	if idx := strings.LastIndex(trimmed, "\n"); idx >= 0 {
		return []byte(trimmed[idx+1:])
	}
	return []byte(trimmed)
}

func TestIsSessionCorruption(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{"Session error:Error: Bad MAC Error: Bad MAC", true},
		{"Failed to decrypt message with any known session...", true},
		{"[engine] onWhatsApp: número registrado -> 50587265635", false},
		{"Imagen escrita en engine OK, esperando confirmación", false},
		{"STDERR: something unrelated", false},
		{"no sessions found to decrypt message", true},
		{"invalid signal key", true},
	}
	for _, c := range cases {
		if got := isSessionCorruption(c.line); got != c.want {
			t.Errorf("isSessionCorruption(%q) = %v, want %v", c.line, got, c.want)
		}
	}
}

// useTestData apunta el paquete a una carpeta de datos temporal y restaura la
// configuración previa al terminar el test.
func useTestData(t *testing.T, dir string) {
	t.Helper()
	prev := options()
	Configure(Options{AppName: "TestApp", AppVersion: "test", DataDir: dir})
	t.Cleanup(func() { Configure(prev) })
}
