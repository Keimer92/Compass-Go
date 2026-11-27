package whatsapp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Emit publica un evento backend → frontend (se resuelve en tiempo de llamada
// porque el servicio se construye antes de que exista la aplicación Wails).
type Emit func(name string, data any)

// Supervisor replica el ciclo de vida del sidecar Node de commands/whatsapp.rs:
// generación + mutex de arranque, lector de stdout que traduce el protocolo de
// prefijos a eventos, escritura de comandos por stdin y limpieza de sesión.
type Supervisor struct {
	emit Emit

	mu         sync.Mutex
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	starting   bool
	closed     bool
	generation uint64
	runDone    chan struct{}

	writeMu sync.Mutex

	lastMu sync.Mutex
	last   *Status

	// lastSessionErr limita la frecuencia del aviso de sesión corrupta para no
	// inundar el frontend con un evento por cada línea de "Bad MAC".
	lastSessionErrMu sync.Mutex
	lastSessionErr   time.Time

	// Historial reciente de "Bad MAC": si se repite, la sesión está rota y
	// enviar sólo haría esperar 40 s por un timeout seguro.
	corruptMu    sync.Mutex
	corruptTimes []time.Time

	// engineLocked: este proceso es el único que puede lanzar el motor de
	// WhatsApp (dos instancias sobre la misma sesión la corrompen).
	engineLocked bool
}

func New(emit Emit) *Supervisor {
	return &Supervisor{
		emit: emit,
		last: &Status{
			Status:  "disconnected",
			Message: strPtr("Motor no iniciado"),
		},
	}
}

// cloneStatus copia también las cadenas apuntadas: Clone de Rust era profundo y
// las getters del servicio no deben exponer punteros compartidos con el estado
// interno (un mutate en el llamador corrompería el último estado conocido).
func cloneStatus(status *Status) *Status {
	clone := *status
	if status.QR != nil {
		value := *status.QR
		clone.QR = &value
	}
	if status.Message != nil {
		value := *status.Message
		clone.Message = &value
	}
	if status.ClientInfo != nil {
		value := *status.ClientInfo
		clone.ClientInfo = &value
	}
	return &clone
}

func (s *Supervisor) currentStatus() *Status {
	s.lastMu.Lock()
	defer s.lastMu.Unlock()
	return cloneStatus(s.last)
}

func (s *Supervisor) emitStatus(status *Status) {
	clone := cloneStatus(status)
	s.lastMu.Lock()
	s.last = clone
	s.lastMu.Unlock()
	if s.emit != nil {
		s.emit("whatsapp-status", clone)
	}
}

func (s *Supervisor) sendStatus(status, message string) {
	waLog("STATUS emit: " + status + " | " + message)
	s.emitStatus(&Status{Status: status, Message: strPtr(message)})
}

func (s *Supervisor) emitOutbound(payload map[string]any) {
	if s.emit != nil {
		s.emit("whatsapp-outbound-message", payload)
	}
}

// isSessionCorruption reconoce los errores que indican una sesión de WhatsApp
// corrupta/inválida: fallos de descifrado Signal (Bad MAC) por claves que ya no
// coinciden con el servidor. Suelen venir por STDERR del motor (Baileys).
func isSessionCorruption(line string) bool {
	lower := strings.ToLower(line)
	return strings.Contains(lower, "bad mac") ||
		strings.Contains(lower, "failed to decrypt") ||
		strings.Contains(lower, "session error") ||
		strings.Contains(lower, "no sessions found to decrypt") ||
		strings.Contains(lower, "invalid signal")
}

// emitSessionCorrupt avisa al frontend (máx. una vez cada 20 s) de que la
// sesión está corrupta y debe re-escanearse el QR. No cambia el estado de
// conexión: el motor puede seguir "connected" pero inservible para enviar.
// Con `message` se puede sobreescribir el texto (p. ej. conflicto de cuenta).
func (s *Supervisor) emitSessionCorrupt(message ...string) {
	s.lastSessionErrMu.Lock()
	if time.Since(s.lastSessionErr) < 20*time.Second {
		s.lastSessionErrMu.Unlock()
		return
	}
	s.lastSessionErr = time.Now()
	s.lastSessionErrMu.Unlock()

	msg := "Sesión de WhatsApp inválida o corrupta. Vuelve a escanear el código QR (Configuración → WhatsApp → limpiar sesión) para restablecer el envío."
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	if s.emit != nil {
		s.emit("whatsapp-session-error", map[string]string{"message": msg})
	}
}

// noteSessionCorruption acumula las señales de "Bad MAC" de los últimos 60 s.
// Varias seguidas indican que la sesión está rota de verdad y no un fallo
// puntual de un contacto concreto (cada evento emite dos líneas).
func (s *Supervisor) noteSessionCorruption() {
	now := time.Now()
	s.corruptMu.Lock()
	keep := s.corruptTimes[:0]
	for _, t := range s.corruptTimes {
		if now.Sub(t) < corruptionWindow {
			keep = append(keep, t)
		}
	}
	s.corruptTimes = append(keep, now)
	s.corruptMu.Unlock()
}

// corruptionWindow limita la ventana en la que un "Bad MAC" cuenta como prueba
// de que la sesión sigue rota. Los Bad MAC llegan en ráfaga al sincronizar; un
// canal realmente corrupto los repite de forma continua. 15 s distingue ambos
// casos y evita bloquear envíos sanos durante un minuto entero tras revincular.
const corruptionWindow = 15 * time.Second

// sessionCorrupt informa si la sesión viene fallando de forma sostenida.
func (s *Supervisor) sessionCorrupt() bool {
	cut := time.Now().Add(-corruptionWindow)
	s.corruptMu.Lock()
	defer s.corruptMu.Unlock()
	n := 0
	for _, t := range s.corruptTimes {
		if t.After(cut) {
			n++
		}
	}
	return n >= 6
}

// resetSessionCorruption descarta el historial de "Bad MAC". Se llama cuando el
// motor abre una conexión nueva, cuando se revincula/limpia la sesión y cuando
// un envío se confirma: en esos momentos el canal vuelve a estar sano y el
// bloqueo de envíos ya no tiene sentido.
func (s *Supervisor) resetSessionCorruption() {
	s.corruptMu.Lock()
	s.corruptTimes = nil
	s.corruptMu.Unlock()
}

// Start lanza el motor. Es inmediato: el arranque real corre en una goroutine,
// como el `tauri::async_runtime::spawn` de Rust.
func (s *Supervisor) Start(force *bool) {
	isForce := force != nil && *force
	waLog(fmt.Sprintf("===== INICIO start_whatsapp (force=%v) =====", isForce))
	waLog("Log file: " + logFilePath())
	waTrace("Iniciando motor de WhatsApp (Modo Sidecar)...")

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		waLog("Start ignored: WhatsApp supervisor is shutting down")
		return
	}
	if isForce {
		if s.cmd != nil && s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
			waTrace("Force restart - proceso anterior terminado")
			waLog("Force restart: proceso anterior terminado (kill)")
		}
		s.cmd = nil
		s.stdin = nil
		s.starting = false
		// Invalida la generación anterior: el Terminated del motor viejo no debe
		// borrar el handle del nuevo proceso.
		s.generation++
	}

	if s.starting {
		status := s.currentStatus()
		s.mu.Unlock()
		waTrace("Motor ya se está iniciando, reenviando último estado...")
		waLog("Motor ya se está iniciando, reenviando último estado")
		if s.emit != nil {
			s.emit("whatsapp-status", status)
		}
		return
	}
	if s.cmd != nil {
		status := s.currentStatus()
		s.mu.Unlock()
		waTrace("Motor ya activo, reenviando estado actual...")
		waLog("Motor ya activo, reenviando estado actual")
		if s.emit != nil {
			s.emit("whatsapp-status", status)
		}
		return
	}
	if !s.engineLocked {
		ok, err := acquireEngineInstance()
		if err != nil {
			waLog("⚠️ No se pudo crear el candado de instancia del motor: " + err.Error())
		} else if !ok {
			s.mu.Unlock()
			msg := fmt.Sprintf("Otra ventana de %s ya está usando WhatsApp. Ciérrala y vuelve a intentar.", cfg.AppName)
			waCritical("%s", msg)
			s.sendStatus("disconnected", msg)
			return
		} else {
			s.engineLocked = true
			waLog("Candado de instancia del motor adquirido")
		}
	}
	s.starting = true
	generation := s.generation
	runDone := make(chan struct{})
	s.runDone = runDone
	s.mu.Unlock()

	go s.run(generation, runDone)
}

func (s *Supervisor) run(generation uint64, runDone chan struct{}) {
	defer func() {
		s.mu.Lock()
		if s.generation == generation {
			s.starting = false
		}
		if s.runDone == runDone {
			s.runDone = nil
		}
		s.mu.Unlock()
		close(runDone)
	}()

	if !s.isCurrent(generation) {
		return
	}
	s.sendStatus("connecting", "🚀 Iniciando motor (Bundle)...")

	bundle := findFirst(
		"whatsapp-engine-bundle/index.js",
		"resources/whatsapp-engine-bundle/index.js",
	)
	if bundle == "" {
		errMsg := "❌ No se encontró el bundle del motor"
		waCritical("%s", errMsg)
		waLog("ERROR: " + errMsg)
		s.failStart(generation, errMsg)
		return
	}
	node, err := resolveNode()
	if err != nil {
		errMsg := "❌ " + err.Error()
		waCritical("%s", errMsg)
		waLog("ERROR: " + errMsg)
		s.failStart(generation, errMsg)
		return
	}
	if err := checkNodeVersion(node); err != nil {
		errMsg := "❌ " + err.Error()
		waCritical("%s", errMsg)
		waLog("ERROR: " + errMsg)
		s.failStart(generation, errMsg)
		return
	}

	waTrace("Ruta del bundle: %s", bundle)
	waLog("Bundle resuelto: " + bundle)

	appData := cfg.DataDir
	waLog("app_data_dir: " + appData)

	authDir := filepath.Join(appData, "whatsapp_auth")
	DiagHeader(
		"node: "+node,
		"bundle: "+bundle,
		"work_dir: "+appData,
		"auth_dir: "+authDir+" ("+describeAuthDir(authDir)+")",
		"engine_env: "+strings.TrimSpace(os.Getenv("COMPASS_WHATSAPP_ENGINE")),
	)

	cmd, stdin, stdout, stderr, err := startEngine(node, bundle, appData)
	if err != nil {
		s.failStart(generation, "❌ Error al lanzar el sidecar de Node: "+err.Error())
		return
	}

	s.mu.Lock()
	if s.closed || s.generation != generation {
		s.mu.Unlock()
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return
	}
	s.cmd = cmd
	s.stdin = stdin
	s.starting = false
	s.mu.Unlock()

	waLog("Sidecar de Node lanzado correctamente")

	var streams sync.WaitGroup
	streams.Add(2)
	go func() {
		defer streams.Done()
		readLines(stdout, func(line string) {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				return
			}
			s.handleMotorLine(trimmed)
		})
	}()
	go func() {
		defer streams.Done()
		readLines(stderr, func(line string) {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				waLog("STDERR: " + trimmed)
			}
			fmt.Fprintf(os.Stderr, "WA_SIDECAR_ERR: %s\n", trimmed)
			// Sesión de WhatsApp corrupta: el canal cifrado no se puede
			// establecer y los envíos (incluidos los recibos) expiran a los
			// 40 s. Lo avisamos al frontend para que pida re-escanear el QR.
			if isSessionCorruption(trimmed) {
				s.noteSessionCorruption()
				s.emitSessionCorrupt()
			}
		})
	}()

	streams.Wait()
	err = cmd.Wait()
	code := -1
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	fmt.Printf("WA_TRACE: Motor terminado con código %d\n", code)
	waLog(fmt.Sprintf("Motor terminado con código %d", code))
	if err != nil && !isExpectedTermination(err) {
		waLog("Wait error: " + err.Error())
	}

	s.mu.Lock()
	if s.generation == generation {
		s.cmd = nil
		s.stdin = nil
		shouldEmitDisconnected := !s.closed
		s.mu.Unlock()
		if shouldEmitDisconnected {
			s.sendStatus("disconnected", "Motor apagado")
		}
		return
	}
	s.mu.Unlock()
}

func (s *Supervisor) isCurrent(generation uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.closed && s.generation == generation
}

func isExpectedTermination(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr)
}

func (s *Supervisor) failStart(generation uint64, message string) {
	waCritical("%s", message)
	waLog("ERROR: " + message)
	if s.isCurrent(generation) {
		s.sendStatus("error", message)
	}
}

// Shutdown solicita el cierre graceful del sidecar y termina el proceso si no
// responde dentro del plazo. Se invoca al cerrar la aplicación Wails.
func (s *Supervisor) Shutdown() error {
	s.mu.Lock()
	if s.closed {
		done := s.runDone
		s.mu.Unlock()
		if done != nil {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				return errors.New("timeout esperando el cierre del motor de WhatsApp")
			}
		}
		return nil
	}
	s.closed = true
	s.generation++
	cmd, stdin, done := s.cmd, s.stdin, s.runDone
	s.cmd = nil
	s.stdin = nil
	s.starting = false
	s.mu.Unlock()

	if stdin != nil {
		s.writeMu.Lock()
		_, writeErr := io.WriteString(stdin, "{\"type\":\"shutdown\"}\n")
		closeErr := stdin.Close()
		s.writeMu.Unlock()
		if writeErr != nil {
			waLog("No se pudo enviar shutdown al motor: " + writeErr.Error())
		}
		if closeErr != nil {
			waLog("No se pudo cerrar stdin del motor: " + closeErr.Error())
		}
	}

	if done == nil {
		return nil
	}
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-done:
		return nil
	case <-timer.C:
		if cmd != nil && cmd.Process != nil {
			if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				return fmt.Errorf("no se pudo terminar el motor de WhatsApp: %w", err)
			}
		}
		select {
		case <-done:
			return nil
		case <-time.After(2 * time.Second):
			return errors.New("timeout esperando el cierre del motor de WhatsApp")
		}
	}
}

// readLines escanea un stream de lado del motor. El búfer se amplía porque las
// líneas QR_DATA_URL superan con holgura el límite por defecto de bufio.
func readLines(r io.Reader, handler func(string)) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		handler(scanner.Text())
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		waLog("stream cerrado: " + err.Error())
	}
}

func (s *Supervisor) Logout() error {
	// Invalida la generación actual para que el Terminated del motor viejo no
	// sobrescriba el estado "Sesión cerrada" ni toque el handle.
	s.mu.Lock()
	s.generation++
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	s.cmd = nil
	s.stdin = nil
	s.starting = false
	s.mu.Unlock()

	authPath := filepath.Join(cfg.DataDir, "whatsapp_auth")
	if _, err := os.Stat(authPath); err == nil {
		_ = os.RemoveAll(authPath)
	}

	// Sesión nueva: el historial de Bad MAC de la sesión anterior ya no aplica.
	s.resetSessionCorruption()

	s.sendStatus("disconnected", "Sesión cerrada")
	return nil
}

func (s *Supervisor) SendDirect(number, message string) (string, error) {
	logFilePath()
	cleaned := cleanNumber(number)

	s.mu.Lock()
	stdin := s.stdin
	s.mu.Unlock()

	if stdin == nil {
		waLog("send_whatsapp_direct: motor NO iniciado")
		return "", errors.New("El motor de WhatsApp no está iniciado")
	}

	payload, _ := json.Marshal(map[string]any{"type": "send", "number": cleaned, "text": message})
	waLog("ENVIAR TEXTO -> number=" + cleaned)
	waDiag("ENVÍO", fmt.Sprintf("texto a %s (%d caracteres)", cleaned, len(message)))
	waLog("Payload: " + string(payload))

	s.writeMu.Lock()
	_, err := stdin.Write(append(payload, '\n'))
	s.writeMu.Unlock()
	if err != nil {
		waLog("ERROR escribiendo en engine: " + err.Error())
		return "", errors.New("Error escribiendo en engine: " + err.Error())
	}

	waLog("Texto escrito en engine OK, esperando confirmación del motor")
	if s.emit != nil {
		s.emit("whatsapp-message-sent", map[string]any{"number": cleaned, "status": "sending"})
	}
	return "SENDING:" + cleaned, nil
}

func (s *Supervisor) SendImage(number, imagePath, caption string) error {
	logFilePath()

	// Con la sesión corrupta (Bad MAC repetido) el motor no puede cifrar ni
	// subir nada: esperar 40 s sólo alarga el fallo.
	if s.sessionCorrupt() {
		msg := "La sesión de WhatsApp está corrupta (Bad MAC): los envíos no se entregan. Vuelve a escanear el QR en Configuración → WhatsApp."
		waLog("ENVIAR IMAGEN bloqueado: " + msg)
		if s.emit != nil {
			s.emit("whatsapp-outbound-message", map[string]any{
				"status":  "error",
				"type":    "image",
				"message": msg,
			})
		}
		return errors.New(msg)
	}

	s.mu.Lock()
	stdin := s.stdin
	s.mu.Unlock()

	if stdin == nil {
		waLog("send_whatsapp_image: motor NO iniciado")
		return errors.New("El motor de WhatsApp no está iniciado")
	}

	payload, _ := json.Marshal(map[string]any{
		"type":      "send_image",
		"number":    number,
		"imagePath": imagePath,
		"caption":   caption,
	})
	waLog(fmt.Sprintf("ENVIAR IMAGEN -> number=%s imagePath=%s caption=%s", number, imagePath, caption))
	waDiag("ENVÍO", fmt.Sprintf("imagen a %s | %s", number, describeFile(imagePath)))
	waLog("Payload: " + string(payload))

	s.writeMu.Lock()
	_, err := stdin.Write(append(payload, '\n'))
	s.writeMu.Unlock()
	if err != nil {
		waLog("ERROR escribiendo en engine: " + err.Error())
		return errors.New("Error escribiendo en engine: " + err.Error())
	}

	waLog("Imagen escrita en engine OK, esperando confirmación del motor")
	return nil
}

func (s *Supervisor) Status() *Status {
	return s.currentStatus()
}

func (s *Supervisor) IsLinked() bool {
	return s.currentStatus().Status == "connected"
}
