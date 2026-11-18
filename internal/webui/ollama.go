package webui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Cliente minimo para la API de chat de Ollama Cloud (o un Ollama local si se
// configura otro endpoint). Solo usa la stdlib.
//
// POST <endpoint>/api/chat
//   Authorization: Bearer <apiKey>
//   { "model": "...", "messages": [...], "stream": false }
// Respuesta: { "message": { "role", "content" }, "error"?: "..." }

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type ollamaChatResponse struct {
	Message ollamaMessage `json:"message"`
	Error   string        `json:"error"`
}

// chatCompletion envia un prompt a Ollama y devuelve el texto de la respuesta.
func chatCompletion(endpoint, apiKey, model, prompt string) (string, error) {
	body, err := json.Marshal(ollamaChatRequest{
		Model:    model,
		Messages: []ollamaMessage{{Role: "user", Content: prompt}},
		Stream:   false,
	})
	if err != nil {
		return "", fmt.Errorf("no se pudo serializar la peticion: %w", err)
	}

	url := strings.TrimRight(strings.TrimSpace(endpoint), "/") + "/api/chat"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("URL de Ollama invalida: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("no se pudo contactar con Ollama: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("no se pudo leer la respuesta de Ollama: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		detail := strings.TrimSpace(string(raw))
		if len(detail) > 300 {
			detail = detail[:300]
		}
		return "", fmt.Errorf("Ollama devolvio HTTP %d: %s", resp.StatusCode, detail)
	}

	var parsed ollamaChatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("respuesta de Ollama no valida: %w", err)
	}
	if strings.TrimSpace(parsed.Error) != "" {
		return "", fmt.Errorf("Ollama: %s", strings.TrimSpace(parsed.Error))
	}
	content := strings.TrimSpace(parsed.Message.Content)
	if content == "" {
		return "", fmt.Errorf("Ollama no devolvio contenido")
	}
	return content, nil
}
