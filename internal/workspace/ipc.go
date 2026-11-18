package workspace

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"time"

	"compass/internal/window"
)

// Message es el mensaje que el CLI envia a la ventana abierta.
type Message struct {
	Op   string `json:"op"`
	Path string `json:"path"`
}

type instanceToken struct {
	Port int `json:"port"`
	PID  int `json:"pid"`
}

func instancePath() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return ""
	}
	return filepath.Join(base, "compass", "gui-instance.json")
}

// TryForward intenta entregar la carpeta a una GUI ya abierta. Devuelve true
// si la entrega tuvo exito (el CLI debe salir sin abrir otra ventana).
func TryForward(dir string) bool {
	path := instancePath()
	if path == "" {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var tok instanceToken
	if err := json.Unmarshal(data, &tok); err != nil || tok.Port <= 0 {
		return false
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", itoa(tok.Port)), 400*time.Millisecond)
	if err != nil {
		_ = os.Remove(path) // instancia muerta: token obsoleto
		return false
	}
	defer conn.Close()
	// Esta instancia la lanzo el usuario (menu contextual / acceso directo), asi
	// que puede ceder el permiso de primer plano a la ventana ya abierta para
	// que se traiga al frente en vez de quedarse detras.
	window.AllowForeground(tok.PID)
	if err := json.NewEncoder(conn).Encode(Message{Op: "addProject", Path: dir}); err != nil {
		return false
	}
	return true
}

// Listen abre el servidor IPC de la GUI. onAdd se llama desde una goroutine:
// quien lo recibe debe pasar la carpeta al hilo de la UI.
func Listen(onAdd func(string)) (func(), error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	port := ln.Addr().(*net.TCPAddr).Port

	if path := instancePath(); path != "" {
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		if data, err := json.Marshal(instanceToken{Port: port, PID: os.Getpid()}); err == nil {
			_ = os.WriteFile(path, data, 0o644)
		}
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				var msg Message
				if err := json.NewDecoder(c).Decode(&msg); err == nil &&
					msg.Op == "addProject" && msg.Path != "" {
					onAdd(msg.Path)
				}
			}(conn)
		}
	}()

	cleanup := func() {
		_ = ln.Close()
		if path := instancePath(); path != "" {
			_ = os.Remove(path)
		}
	}
	return cleanup, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
