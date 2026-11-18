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
	// la enmascara).
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

func TestGetSettingsMasksAPIKey(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())

	settings.Save(settings.Settings{OpenRouterAPIKey: "sk-or-v1-secreto"})

	b := &Bridge{}
	if got := b.GetSettings().OpenRouterAPIKey; got != apiKeyMask {
		t.Fatalf("la key deberia llegar enmascarada, got %q", got)
	}
	// Guardar de nuevo con la mascara (p.ej. al cambiar solo el modelo) no la
	// borra ni la sustituye por asteriscos.
	b.SetSettings(b.GetSettings())
	if got := settings.Load().OpenRouterAPIKey; got != "sk-or-v1-secreto" {
		t.Fatalf("la key se corrompio al reguardar la mascara: %q", got)
	}
}
