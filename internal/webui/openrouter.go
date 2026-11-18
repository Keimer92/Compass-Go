package webui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"compass/internal/settings"
)

// Cliente minimo para la API de chat de OpenRouter (compatible con OpenAI).
// Solo usa la stdlib.
//
// POST <endpoint>/chat/completions
//   Authorization: Bearer <apiKey>
//   { "model": "...", "messages": [...], "stream": false }
// Respuesta: { "choices": [ { "message": { "content": "..." } } ], "error"?: {...} }

type orMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type orChatRequest struct {
	Model    string      `json:"model"`
	Messages []orMessage `json:"messages"`
	Stream   bool        `json:"stream"`
}

type orChatResponse struct {
	Choices []struct {
		Message orMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Code    any    `json:"code"`
	} `json:"error"`
}

// openRouterChat envia un prompt a OpenRouter y devuelve el texto de la
// respuesta.
func openRouterChat(endpoint, apiKey, model, prompt string) (string, error) {
	body, err := json.Marshal(orChatRequest{
		Model:    model,
		Messages: []orMessage{{Role: "user", Content: prompt}},
		Stream:   false,
	})
	if err != nil {
		return "", fmt.Errorf("no se pudo serializar la peticion: %w", err)
	}

	url := strings.TrimRight(strings.TrimSpace(endpoint), "/") + "/chat/completions"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("URL de OpenRouter invalida: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	// OpenRouter recomienda identificar la aplicacion que consume la API.
	req.Header.Set("HTTP-Referer", "https://github.com/compass-go")
	req.Header.Set("X-Title", "Compass Go")

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("no se pudo contactar con OpenRouter: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("no se pudo leer la respuesta de OpenRouter: %w", err)
	}

	var parsed orChatResponse
	_ = json.Unmarshal(raw, &parsed)

	if resp.StatusCode != http.StatusOK {
		if parsed.Error != nil && strings.TrimSpace(parsed.Error.Message) != "" {
			return "", fmt.Errorf("OpenRouter devolvio HTTP %d: %s", resp.StatusCode, strings.TrimSpace(parsed.Error.Message))
		}
		detail := strings.TrimSpace(string(raw))
		if len(detail) > 300 {
			detail = detail[:300]
		}
		return "", fmt.Errorf("OpenRouter devolvio HTTP %d: %s", resp.StatusCode, detail)
	}

	if parsed.Error != nil && strings.TrimSpace(parsed.Error.Message) != "" {
		return "", fmt.Errorf("OpenRouter: %s", strings.TrimSpace(parsed.Error.Message))
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("OpenRouter no devolvio contenido")
	}
	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("OpenRouter no devolvio contenido")
	}
	return content, nil
}

// ModelOption es un modelo gratuito de OpenRouter listo para el selector.
type ModelOption struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Context int    `json:"context"`
}

type orModelsResponse struct {
	Data []struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		ContextLength int    `json:"context_length"`
		Pricing       struct {
			Prompt     string `json:"prompt"`
			Completion string `json:"completion"`
		} `json:"pricing"`
	} `json:"data"`
}

// modelSpeedRank ordena por familias que suelen ser rapidas. La API de
// OpenRouter no expone throughput, asi que usamos este heuristico.
func modelSpeedRank(id, name string) int {
	s := strings.ToLower(id + " " + name)
	switch {
	case strings.Contains(s, "flash"):
		return 0
	case strings.Contains(s, "mini"):
		return 1
	case strings.Contains(s, "small"):
		return 2
	case strings.Contains(s, "nano"):
		return 3
	case strings.Contains(s, "lite"):
		return 4
	case strings.Contains(s, "turbo"):
		return 5
	case strings.Contains(s, "instant"):
		return 6
	case strings.Contains(s, "haiku"):
		return 7
	}
	return 20
}

// ListModels devuelve los modelos GRATUITOS de OpenRouter ordenados por un
// heuristico de velocidad (familias flash/mini/small/lite/turbo primero) y,
// a igualdad, por contexto mas pequeno. No requiere API key.
func (b *Bridge) ListModels() []ModelOption {
	endpoint := strings.TrimRight(strings.TrimSpace(settings.Load().OpenRouterEndpoint), "/")
	if endpoint == "" {
		endpoint = "https://openrouter.ai/api/v1"
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(endpoint + "/models")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}
	var parsed orModelsResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil
	}

	out := make([]ModelOption, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		if strings.HasPrefix(m.ID, "~") { // alias internos de OpenRouter
			continue
		}
		free := strings.HasSuffix(m.ID, ":free") ||
			(strings.TrimSpace(m.Pricing.Prompt) == "0" && strings.TrimSpace(m.Pricing.Completion) == "0")
		if !free {
			continue
		}
		out = append(out, ModelOption{ID: m.ID, Name: m.Name, Context: m.ContextLength})
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := modelSpeedRank(out[i].ID, out[i].Name), modelSpeedRank(out[j].ID, out[j].Name)
		if ri != rj {
			return ri < rj
		}
		if out[i].Context != out[j].Context {
			return out[i].Context < out[j].Context
		}
		return out[i].ID < out[j].ID
	})
	return out
}
