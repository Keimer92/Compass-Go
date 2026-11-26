package webui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// speedServer responde a /chat/completions con un retardo distinto por modelo.
func speedServer(t *testing.T, failModel string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			http.NotFound(w, r)
			return
		}
		var req struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Model == failModel {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"error":{"message":"modelo no disponible"}}`)
			return
		}
		if strings.Contains(req.Model, "slow") {
			time.Sleep(120 * time.Millisecond)
		} else {
			time.Sleep(5 * time.Millisecond)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"OK"}}]}`)
	}))
}

func TestTestModelSpeedsOrdenaPorTiempo(t *testing.T) {
	srv := speedServer(t, "")
	defer srv.Close()

	models := []ModelOption{
		{ID: "slow/model", Name: "Lento"},
		{ID: "fast/model", Name: "Rapido"},
	}
	got := (&Bridge{}).TestModelSpeeds(models, srv.URL, "sk-test")
	if len(got) != 2 {
		t.Fatalf("esperaba 2 resultados, got %d", len(got))
	}
	if got[0].ID != "fast/model" || !got[0].OK {
		t.Fatalf("el mas rapido debe ir primero: %+v", got[0])
	}
	if got[0].MS >= got[1].MS {
		t.Fatalf("tiempos no ordenados: %+v", got)
	}
	if got[0].Name != "Rapido" {
		t.Fatalf("nombre perdido: %+v", got[0])
	}
}

func TestTestModelSpeedsFallosAlFinal(t *testing.T) {
	srv := speedServer(t, "roto/model")
	defer srv.Close()

	models := []ModelOption{{ID: "roto/model", Name: "Roto"}, {ID: "fast/model", Name: "Rapido"}}
	got := (&Bridge{}).TestModelSpeeds(models, srv.URL, "sk-test")
	if len(got) != 2 || got[0].ID != "fast/model" {
		t.Fatalf("el fallo debe ir al final: %+v", got)
	}
	if got[1].OK || !strings.Contains(got[1].Error, "modelo no disponible") {
		t.Fatalf("el fallo debe traer el motivo: %+v", got[1])
	}
}

func TestTestModelSpeedsSinKey(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	got := (&Bridge{}).TestModelSpeeds([]ModelOption{{ID: "fast/model"}}, "", "")
	if len(got) != 0 {
		t.Fatalf("sin API key no debe consultar: %+v", got)
	}
}
