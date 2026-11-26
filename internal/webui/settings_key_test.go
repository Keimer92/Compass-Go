package webui

import (
	"testing"

	"compass/internal/settings"
)

func TestSetSettingsKeepsAPIKeyWhenModelChanges(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())

	settings.Save(settings.Settings{
		OpenOutputDir:      true,
		OpenRouterEndpoint: "https://openrouter.ai/api/v1",
		OpenRouterModel:    "modelo-a",
		OpenRouterAPIKey:   "sk-or-v1-secreto",
	})

	b := &Bridge{}
	// Lo que envia el frontend al cambiar de modelo: la key va vacia (GetSettings
	// nunca la devuelve).
	b.SetSettings(settings.Settings{
		OpenOutputDir:      true,
		OpenRouterEndpoint: "https://openrouter.ai/api/v1",
		OpenRouterModel:    "modelo-b",
		OpenRouterAPIKey:   "",
	})

	got := settings.Load()
	if got.OpenRouterAPIKey != "sk-or-v1-secreto" {
		t.Fatalf("la key se perdio al cambiar de modelo: %q", got.OpenRouterAPIKey)
	}
	if got.OpenRouterModel != "modelo-b" {
		t.Fatalf("el modelo no se guardo: %q", got.OpenRouterModel)
	}
}

func TestGetSettingsNeverExposesAPIKey(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())

	settings.Save(settings.Settings{OpenRouterAPIKey: "sk-or-v1-secreto"})

	b := &Bridge{}
	if got := b.GetSettings().OpenRouterAPIKey; got != "" {
		t.Fatalf("la key no debe llegar al frontend, got %q", got)
	}
	if !b.OpenRouterKeyConfigured() {
		t.Fatal("OpenRouterKeyConfigured debe ser true con key guardada")
	}
	// Guardar de nuevo sin key (p.ej. al cambiar solo el modelo) no la borra.
	b.SetSettings(b.GetSettings())
	if got := settings.Load().OpenRouterAPIKey; got != "sk-or-v1-secreto" {
		t.Fatalf("la key se perdio al reguardar los ajustes: %q", got)
	}
}
