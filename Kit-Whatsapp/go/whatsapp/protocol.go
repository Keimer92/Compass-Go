package whatsapp

import (
	"fmt"
	"strings"
)

// statusFromLine traduce una línea STATUS:<code>:<extra> del motor al estado de
// interfaz, replicando la tabla de commands/whatsapp.rs. El `extra` se conserva
// tal cual (Rust solo recorta la línea completa, no el resto tras el primer dos puntos).
func statusFromLine(content string) (uiStatus string, uiMessage string, clientInfo *string) {
	code := content
	var extra *string
	if idx := strings.Index(content, ":"); idx >= 0 {
		code = content[:idx]
		rest := content[idx+1:]
		extra = &rest
	}

	switch code {
	case "init":
		return "connecting", "⚙️ Iniciando servicios...", nil
	case "starting":
		return "connecting", "🌐 Cargando módulos...", nil
	case "auth_ready":
		return "connecting", "🔐 Preparando sesión...", nil
	case "socket_creating":
		return "connecting", "📡 Creando túnel seguro...", nil
	case "connecting_socket":
		return "connecting", "🛰️ Conectando con servidores...", nil
	case "restarting":
		return "connecting", "🔄 Reintentando en 5s...", nil
	case "session_invalid":
		return "disconnected", "❌ Sesión expirada o cerrada. Limpiando...", nil
	case "account_conflict":
		return "disconnected", "⚠️ La cuenta de WhatsApp se usó en otro dispositivo. Úsala en un solo equipo o con otro número.", nil
	case "session_cleared":
		return "disconnected", "🧹 Sesión limpiada. Escanea el código QR de nuevo.", nil
	case "connected":
		return "connected", "✓ WhatsApp Conectado", extra
	case "disconnected":
		reason := "Sesión interrumpida"
		if extra != nil {
			reason = *extra
		}
		return "disconnected", reason, nil
	case "error":
		msg := "❌ Error crítico"
		if extra != nil {
			msg = *extra
		}
		return "disconnected", msg, nil
	default:
		return "connecting", "⚙️ Cargando...", nil
	}
}

func strPtr(s string) *string { return &s }

// outboundPayload construye el objeto del evento whatsapp-outbound-message con las
// mismas claves que el json! de Rust (status, type, number, message).
func outboundPayload(fields map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range fields {
		if s, ok := v.(string); ok {
			out[k] = s
			continue
		}
		out[k] = v
	}
	return out
}

// handleMotorLine procesa una línea de stdout del bundle. Devuelve true si la línea
// produjo un evento de estado (para el log de trazabilidad).
func (s *Supervisor) handleMotorLine(line string) {
	switch {
	case strings.HasPrefix(line, "QR_DATA_URL:"):
		waDiag("MOTOR", fmt.Sprintf("QR recibido como imagen (%d caracteres)", len(line)))
		s.emitStatus(&Status{
			Status:  "qr",
			QR:      strPtr(strings.TrimSpace(strings.ReplaceAll(line, "QR_DATA_URL:", ""))),
			Message: strPtr("✓ ¡Listo! Escanea el código QR"),
		})
	case strings.HasPrefix(line, "QR:"):
		waDiag("MOTOR", fmt.Sprintf("QR recibido como texto (%d caracteres)", len(line)))
		s.emitStatus(&Status{
			Status:  "qr",
			QR:      strPtr(strings.TrimSpace(strings.ReplaceAll(line, "QR:", ""))),
			Message: strPtr("✓ ¡Listo! Escanea el código QR"),
		})
	case strings.HasPrefix(line, "QR_ERROR:"):
		errMsg := strings.TrimSpace(strings.ReplaceAll(line, "QR_ERROR:", ""))
		waDiag("MOTOR", "QR_ERROR: "+errMsg)
		s.emitStatus(&Status{
			Status:  "disconnected",
			Message: strPtr("❌ Error generando QR: " + errMsg),
		})
	case strings.HasPrefix(line, "STATUS:"):
		content := strings.TrimSpace(strings.ReplaceAll(line, "STATUS:", ""))
		uiStatus, uiMessage, clientInfo := statusFromLine(content)
		waLog("STATUS: " + content + " (" + uiMessage + ")")
		// Conexión abierta = socket nuevo con claves nuevas: los Bad MAC de la
		// sesión anterior ya no describen el estado del canal. Lo mismo cuando
		// el motor limpia la sesión (revinculación).
		if uiStatus == "connected" || strings.HasPrefix(content, "session_cleared") || strings.HasPrefix(content, "session_invalid") {
			s.resetSessionCorruption()
		}
		s.emitStatus(&Status{
			Status:     uiStatus,
			Message:    strPtr(uiMessage),
			ClientInfo: clientInfo,
		})
		// Conflicto de cuenta (misma cuenta en otro dispositivo): avisamos con
		// el banner rojo además del cambio de estado.
		if strings.HasPrefix(content, "account_conflict") {
			s.emitSessionCorrupt("La cuenta de WhatsApp se está usando en otro dispositivo. Usa la cuenta en un solo equipo (o con otro número). Vuelve a escanear el QR para reenlazarla aquí.")
		}
	case strings.HasPrefix(line, "SENDING:"):
		num := strings.TrimSpace(strings.ReplaceAll(line, "SENDING:", ""))
		waLog("MOTOR: SENDING (enviando texto)")
		s.emitOutbound(outboundPayload(map[string]any{"status": "sending", "type": "text", "number": num}))
	case strings.HasPrefix(line, "SENT_CONFIRM:"), strings.HasPrefix(line, "SENT_RETRY_OK:"):
		num := fieldAfterColons(line, 1)
		waLog("MOTOR: CONFIRMADO -> " + line)
		s.resetSessionCorruption()
		s.emitOutbound(outboundPayload(map[string]any{"status": "sent", "type": "text", "number": num}))
	case strings.HasPrefix(line, "IMAGE_SENT_CONFIRM:"):
		num := strings.TrimSpace(strings.ReplaceAll(line, "IMAGE_SENT_CONFIRM:", ""))
		waLog("MOTOR: IMAGEN CONFIRMADA -> " + line)
		s.resetSessionCorruption()
		s.emitOutbound(outboundPayload(map[string]any{"status": "sent", "type": "image", "number": num}))
	case strings.HasPrefix(line, "IMAGE_ERROR:"):
		waLog("MOTOR: ERROR IMAGEN -> " + line)
		s.emitOutbound(outboundPayload(map[string]any{
			"status":  "error",
			"type":    "image",
			"message": strings.TrimSpace(strings.ReplaceAll(line, "IMAGE_ERROR:", "")),
		}))
	case strings.HasPrefix(line, "IMAGE_SILENT_BLOCK:"):
		num := strings.TrimSpace(strings.ReplaceAll(line, "IMAGE_SILENT_BLOCK:", ""))
		waLog("MOTOR: POSIBLE BLOQUEO SILENCIOSO (imagen) -> " + num)
		s.emitOutbound(outboundPayload(map[string]any{
			"status":  "error",
			"type":    "image",
			"message": "Posible bloqueo silencioso: el servidor no confirmó la entrega al número " + num,
		}))
	case strings.HasPrefix(line, "SILENT_BLOCK:"):
		num := strings.TrimSpace(strings.ReplaceAll(line, "SILENT_BLOCK:", ""))
		waLog("MOTOR: POSIBLE BLOQUEO SILENCIOSO (texto) -> " + num)
		s.emitOutbound(outboundPayload(map[string]any{
			"status":  "error",
			"type":    "text",
			"message": "Posible bloqueo silencioso: el servidor no confirmó la entrega al número " + num,
		}))
	case strings.HasPrefix(line, "ACK_PENDING:"):
		num := strings.TrimSpace(strings.ReplaceAll(line, "ACK_PENDING:", ""))
		waLog("MOTOR: ACK PENDIENTE (posible demora de entrega) -> " + num)
		s.emitOutbound(outboundPayload(map[string]any{
			"status":  "pending_ack",
			"type":    "text",
			"number":  num,
			"message": "El mensaje a " + num + " fue enviado pero WhatsApp aún no confirma la entrega. Puede tardar unos segundos más.",
		}))
	case strings.HasPrefix(line, "SEND_ERROR:"):
		waLog("MOTOR: ERROR ENVÍO -> " + line)
		s.emitOutbound(outboundPayload(map[string]any{
			"status":  "error",
			"type":    "text",
			"message": strings.TrimSpace(strings.ReplaceAll(line, "SEND_ERROR:", "")),
		}))
	case strings.HasPrefix(line, "SEND_RETRY_FAILED:"):
		waLog("MOTOR: RETRY FALLÓ -> " + line)
		s.emitOutbound(outboundPayload(map[string]any{
			"status":  "error",
			"type":    "text",
			"message": strings.TrimSpace(strings.ReplaceAll(line, "SEND_RETRY_FAILED:", "")),
		}))
	case strings.HasPrefix(line, "ENGINE_ERROR:"):
		waLog("MOTOR: ERROR MOTOR -> " + line)
		s.emitOutbound(outboundPayload(map[string]any{
			"status":  "error",
			"type":    "engine",
			"message": strings.TrimSpace(strings.ReplaceAll(line, "ENGINE_ERROR:", "")),
		}))
	case strings.HasPrefix(line, "CONFIG_RECEIVED:"):
		waLog("MOTOR: Config recibida correctamente")
	default:
		if strings.TrimSpace(line) != "" {
			waLog("MOTOR (no manejado): " + line)
		}
	}
}

// fieldAfterColons equivale a l.split(':').nth(1) de Rust: el segundo campo.
func fieldAfterColons(line string, index int) string {
	parts := strings.Split(line, ":")
	if index < len(parts) {
		return strings.TrimSpace(parts[index])
	}
	return ""
}
