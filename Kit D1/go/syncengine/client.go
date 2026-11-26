package syncengine

// Cliente HTTP hacia el Worker de Cloudflare.
//
// Transcripción de `http_post_json` en `src-tauri/src/d1.rs` (que usaba `ureq`).
// El contrato es el del worker (`cloudflare-sync/src/index.ts`):
// POST con JSON y header `X-School-Key`; el ruteo del school key a la base
// D1 lo hace el worker.

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Timeouts del cliente (los del Rust: `timeout_connect(10s)` y
// `timeout(60s)`).
const (
	TimeoutConexion = 10 * time.Second
	TimeoutTotal    = 60 * time.Second
)

// Client habla con un worker D1.
type Client struct {
	// BaseURL es la URL del worker, sin barra final.
	BaseURL string
	// SchoolKey identifica el namespace del usuario (header `X-School-Key`).
	SchoolKey string
	// HTTP es el cliente de red (timeouts incluidos).
	HTTP *http.Client
}

// NewClient arma un cliente con los timeouts del original.
func NewClient(baseURL, schoolKey string) *Client {
	return &Client{
		BaseURL:   strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		SchoolKey: schoolKey,
		HTTP: &http.Client{
			Timeout: TimeoutTotal,
			Transport: &http.Transport{
				DialContext:           (&net.Dialer{Timeout: TimeoutConexion}).DialContext,
				TLSHandshakeTimeout:   TimeoutConexion,
				ResponseHeaderTimeout: TimeoutTotal,
				MaxIdleConnsPerHost:   2,
				// Forzar HTTP/1.1: con Cloudflare el cliente HTTP/2 de Go sufre
				// RST inesperados (WSAECONNRESET) al reusar conexiones idle, lo
				// que abortaba el /push con "conexión interrumpida por el host
				// remoto". HTTP/1.1 con keep-alive corto es estable aquí.
				ForceAttemptHTTP2: false,
				TLSClientConfig:   &tls.Config{NextProtos: []string{"http/1.1"}},
				IdleConnTimeout:   30 * time.Second,
				DisableKeepAlives: false,
			},
		},
	}
}

// Post hace un POST JSON a `path` del worker y devuelve la respuesta parseada.
//
// Equivale a `http_post_json` del Rust:
//   - 4xx/5xx → error con el mensaje real del worker (campo `error`);
//   - `{"ok": false}` → error aunque el status sea 200;
//   - respuesta no-JSON → error con el texto crudo.
func (c *Client) Post(path string, body any) (map[string]any, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("no se pudo serializar el cuerpo de %s: %w", path, err)
	}

	url := c.BaseURL + path

	// Reintento ante errores de red transitorios (RST de Cloudflare, caída de
	// conexión idle, etc.). No reintenta errores de aplicación (4xx/5xx), que
	// vienen como respuesta HTTP y se procesan abajo.
	const reintentos = 3
	var lastErr error
	for intento := 0; intento < reintentos; intento++ {
		if intento > 0 {
			time.Sleep(time.Duration(intento) * 500 * time.Millisecond)
		}
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("no se pudo preparar la petición hacia %s: %w", url, err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-School-Key", c.SchoolKey)

		resp, err := c.HTTP.Do(req)
		if err != nil {
			lastErr = err
			// Error de red: reintentar.
			continue
		}
		defer resp.Body.Close()

		text, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = fmt.Errorf("no se pudo leer la respuesta de %s: %w", path, err)
			continue
		}

		var parsed map[string]any
		dec := json.NewDecoder(bytes.NewReader(text))
		dec.UseNumber()
		if err := dec.Decode(&parsed); err != nil {
			if resp.StatusCode >= 400 {
				return nil, fmt.Errorf("Worker error (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(text)))
			}
			return nil, fmt.Errorf("respuesta inválida del Worker en %s: %s", path, strings.TrimSpace(string(text)))
		}

		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("Worker error (HTTP %d): %s", resp.StatusCode, detalleError(parsed, string(text)))
		}
		if ok, presente := parsed["ok"].(bool); presente && !ok {
			return nil, fmt.Errorf("Worker error (HTTP %d): %s", resp.StatusCode, detalleError(parsed, string(text)))
		}
		return parsed, nil
	}
	return nil, fmt.Errorf("Error de red hacia el Worker (%s): %w", url, lastErr)
}

// detalleError saca el mensaje real del worker (campo `error`), o el cuerpo
// crudo si no está.
func detalleError(parsed map[string]any, fallback string) string {
	if parsed != nil {
		if e, ok := parsed["error"].(string); ok && e != "" {
			return e
		}
	}
	return strings.TrimSpace(fallback)
}
