package tasks

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"compass/internal/ui"
)

//go:embed assets/opencode/tui.tsx
var pluginSource string

const (
	opProvider     = "inferx"
	opModel        = "deepseek-v4.1-flash-1017-basic"
	opModelName    = "DeepSeek V4.1 Flash (InferX)"
	opBaseURL      = "https://model.inferx.net/endpoints/v1"
	opPackage      = "@opencode/ai/providers/openai-compatible"
	opAPIEnv       = "INFERX_API_KEY"
	opPlaceholder  = "PEGA_TU_API_AQUI"
	opReminderFile = "opencode-inferx-reminder"
	opPluginDir    = "inferx-reminder"
	opConfigSchema = "https://opencode.ai/config.json"
)

// ConfigDir devuelve el directorio global de configuracion de OpenCode.
func opencodeConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "opencode")
	}
	if home := homeDir(); home != "" {
		return filepath.Join(home, ".config", "opencode")
	}
	return ""
}

// ConfigFile devuelve el archivo de configuracion a modificar (jsonc preferente).
func opencodeConfigFile() string {
	dir := opencodeConfigDir()
	if dir == "" {
		return ""
	}
	jsonc := filepath.Join(dir, "opencode.jsonc")
	json := filepath.Join(dir, "opencode.json")
	if fileExists(jsonc) {
		return jsonc
	}
	if fileExists(json) {
		return json
	}
	return jsonc
}

// OpencodeAvailable indica si OpenCode esta instalado (PATH o npm global).
func opencodeAvailable() bool {
	if commandExists("opencode") {
		return true
	}
	if appdata := os.Getenv("APPDATA"); appdata != "" {
		npm := filepath.Join(appdata, "npm")
		if fileExists(filepath.Join(npm, "opencode.cmd")) ||
			fileExists(filepath.Join(npm, "opencode.ps1")) {
			return true
		}
	}
	return false
}

// Configure anade (o completa) el proveedor inferx en opencode.json(c).
func ConfigureOpencode(interactive bool) error {
	if !opencodeAvailable() {
		fmt.Println("[SKIP] OpenCode no esta instalado: no se configura InferX.")
		return nil
	}
	path := opencodeConfigFile()
	if path == "" {
		fmt.Println("[WARN] No se pudo determinar la carpeta de configuracion de OpenCode.")
		return nil
	}

	var value map[string]interface{}
	if data, err := os.ReadFile(path); err == nil {
		parsed, err := parseJSONC(string(data))
		if err != nil {
			fmt.Printf("[WARN] No se pudo interpretar %s: %v\n", path, err)
			fmt.Println("[WARN] No se modifico la configuracion de OpenCode.")
			return nil
		}
		if m, ok := parsed.(map[string]interface{}); ok {
			value = m
		} else {
			value = map[string]interface{}{}
		}
	} else {
		value = map[string]interface{}{}
	}

	yaConfigurado := inferxConfiguredIn(value) || envSet(opAPIEnv)

	var apiKey string
	if !yaConfigurado && interactive {
		fmt.Println()
		fmt.Println("[?] InferX esta disponible en OpenCode, pero falta su API key.")
		fmt.Printf("    Endpoint: %s\n", opBaseURL)
		fmt.Println("    (Enter para dejarla pendiente; tambien vale " + opAPIEnv + ")")
		key := ui.AskPassword("    API key de InferX")
		if strings.TrimSpace(key) != "" {
			apiKey = key
		}
	}

	mergeInferx(value, apiKey)

	if parent := filepath.Dir(path); parent != "" {
		_ = os.MkdirAll(parent, 0755)
	}
	if fileExists(path) {
		backupOpencode(path)
	}

	rendered, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("no se pudo serializar la config: %w", err)
	}
	if err := os.WriteFile(path, append(rendered, '\n'), 0644); err != nil {
		return fmt.Errorf("no se pudo escribir %s: %w", path, err)
	}

	fmt.Println()
	fmt.Printf("[OK] Proveedor InferX configurado en %s\n", path)
	if apiKey != "" {
		fmt.Printf("[OK] API key guardada (%s…).\n", shortKey(apiKey))
	} else if yaConfigurado {
		fmt.Println("[SKIP] La API key de InferX ya estaba configurada.")
	} else {
		fmt.Println("[INFO] Sin API key todavia. Se puede anadir despues:")
		fmt.Printf("       - variable de entorno %s, o\n", opAPIEnv)
		fmt.Println("       - `compass --task opencode-configure` para volver a pedirla.")
	}

	if err := installPlugin(); err != nil {
		fmt.Printf("[WARN] No se pudo instalar el plugin: %v\n", err)
	}

	if yaConfigurado || apiKey != "" || inferxConfiguredIn(value) {
		clearReminderDismissal()
	}
	return nil
}

func installPlugin() error {
	dir := opencodeConfigDir()
	if dir == "" {
		return nil
	}
	pluginDir := filepath.Join(dir, "plugins", opPluginDir)
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		return fmt.Errorf("no se pudo crear %s: %w", pluginDir, err)
	}
	file := filepath.Join(pluginDir, "tui.tsx")
	if err := os.WriteFile(file, []byte(pluginSource), 0644); err != nil {
		return fmt.Errorf("no se pudo escribir %s: %w", file, err)
	}
	fmt.Println("[OK] Recordatorio de InferX instalado en la TUI de OpenCode.")
	fmt.Printf("     %s\n", file)
	return nil
}

func backupOpencode(path string) {
	name := filepath.Base(path)
	backup := filepath.Join(filepath.Dir(path), name+".compass.bak")
	_ = copyFile(path, backup)
}

func shortKey(key string) string {
	runes := []rune(strings.TrimSpace(key))
	if len(runes) <= 8 {
		return strings.Repeat("*", len(runes))
	}
	return string(runes[:6])
}

// inferxConfiguredIn indica si InferX esta listo (clave literal, ref env o env).
func inferxConfiguredIn(value map[string]interface{}) bool {
	providers, ok := value["providers"].(map[string]interface{})
	if !ok {
		return false
	}
	provider, ok := providers[opProvider].(map[string]interface{})
	if !ok {
		return false
	}

	if settings, ok := provider["settings"].(map[string]interface{}); ok {
		if key, ok := settings["apiKey"].(string); ok {
			key = strings.TrimSpace(key)
			if key != "" && key != opPlaceholder {
				if ref := envReference(key); ref != "" {
					if envSet(ref) {
						return true
					}
				} else {
					return true
				}
			}
		}
	}

	if env, ok := provider["env"].([]interface{}); ok {
		for _, v := range env {
			if s, ok := v.(string); ok && envSet(s) {
				return true
			}
		}
	}
	return false
}

func envSet(name string) bool {
	v := os.Getenv(name)
	return v != ""
}

func envReference(value string) string {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "{env:") {
		return ""
	}
	rest := strings.TrimPrefix(value, "{env:")
	rest = strings.TrimSuffix(rest, "}")
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return ""
	}
	return rest
}

func mergeInferx(value map[string]interface{}, apiKey string) {
	providers, ok := value["providers"].(map[string]interface{})
	if !ok {
		providers = map[string]interface{}{}
		value["providers"] = providers
	}
	provider, ok := providers[opProvider].(map[string]interface{})
	if !ok {
		provider = map[string]interface{}{}
		providers[opProvider] = provider
	}

	provider["name"] = "InferX"
	provider["env"] = []interface{}{opAPIEnv}
	provider["package"] = opPackage

	settings, ok := provider["settings"].(map[string]interface{})
	if !ok {
		settings = map[string]interface{}{}
		provider["settings"] = settings
	}
	settings["baseURL"] = opBaseURL
	if key := strings.TrimSpace(apiKey); key != "" {
		settings["apiKey"] = key
	}

	models, ok := provider["models"].(map[string]interface{})
	if !ok {
		models = map[string]interface{}{}
		provider["models"] = models
	}
	if _, ok := models[opModel].(map[string]interface{}); !ok {
		models[opModel] = map[string]interface{}{"name": opModelName}
	}

	if _, ok := value["model"]; !ok {
		value["model"] = fmt.Sprintf("%s/%s", opProvider, opModel)
	}
}

// parseJSONC acepta JSON o JSONC (comentarios y comas finales) tolerante.
func parseJSONC(input string) (interface{}, error) {
	clean := stripJSONCComments(input)
	clean = stripTrailingCommas(clean)
	dec := json.NewDecoder(strings.NewReader(clean))
	dec.UseNumber()
	var out interface{}
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func stripJSONCComments(input string) string {
	var b strings.Builder
	inString := false
	escape := false
	chars := []rune(input)
	for i := 0; i < len(chars); i++ {
		c := chars[i]
		if inString {
			b.WriteRune(c)
			if escape {
				escape = false
			} else if c == '\\' {
				escape = true
			} else if c == '"' {
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
			b.WriteRune(c)
			continue
		}
		if c == '/' && i+1 < len(chars) {
			next := chars[i+1]
			if next == '/' {
				for i+1 < len(chars) && chars[i+1] != '\n' {
					i++
				}
				continue
			}
			if next == '*' {
				i += 2
				for i < len(chars) && !(chars[i] == '*' && i+1 < len(chars) && chars[i+1] == '/') {
					i++
				}
				i++
				continue
			}
		}
		b.WriteRune(c)
	}
	return b.String()
}

func stripTrailingCommas(input string) string {
	chars := []rune(input)
	var b strings.Builder
	inString := false
	escape := false
	for i := 0; i < len(chars); i++ {
		c := chars[i]
		if inString {
			b.WriteRune(c)
			if escape {
				escape = false
			} else if c == '\\' {
				escape = true
			} else if c == '"' {
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
			b.WriteRune(c)
			continue
		}
		if c == ',' {
			j := i + 1
			for j < len(chars) && (chars[j] == ' ' || chars[j] == '\n' || chars[j] == '\t' || chars[j] == '\r') {
				j++
			}
			if j < len(chars) && (chars[j] == '}' || chars[j] == ']') {
				continue
			}
		}
		b.WriteRune(c)
	}
	return b.String()
}

func reminderPath() string {
	if d := dataDir(); d != "" {
		return filepath.Join(d, "compass", opReminderFile)
	}
	return ""
}

func reminderDismissed() bool {
	p := reminderPath()
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}

func dismissReminder() {
	p := reminderPath()
	if p == "" {
		return
	}
	if dir := filepath.Dir(p); dir != "" {
		_ = os.MkdirAll(dir, 0755)
	}
	_ = os.WriteFile(p, []byte("dismissed\n"), 0644)
}

func clearReminderDismissal() {
	p := reminderPath()
	if p != "" {
		_ = os.Remove(p)
	}
}

// reminderNeeded indica si debe mostrarse el aviso de "falta la API de InferX".
func OpencodeReminderNeeded() bool {
	if reminderDismissed() || !opencodeAvailable() {
		return false
	}
	return keyIsMissing()
}

func keyIsMissing() bool {
	path := opencodeConfigFile()
	if path == "" || !fileExists(path) {
		return true
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	parsed, err := parseJSONC(string(data))
	if err != nil {
		return true
	}
	m, ok := parsed.(map[string]interface{})
	if !ok {
		return true
	}
	return !inferxConfiguredIn(m)
}

// ConfigureOpencodeEntry expone el punto de entrada global.
func ConfigureOpencodeEntry() error {
	return ConfigureOpencode(true)
}
