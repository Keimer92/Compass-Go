// Package webui — panel de administración de Kit D1.
//
// Expone el estado del kit (módulo Go, instancias del worker, consumidores y
// sesión de Wrangler) y lanza sus acciones en la ConPTY de Compass. El kit es
// global (no por proyecto), así que el panel vive en la barra lateral como
// "Kit D1", igual que "Ajustes de Git".
package webui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"compass/internal/settings"
	"compass/internal/windialog"
)

const (
	kitD1Module = "github.com/Keimer92/kit-d1/go/syncengine"
	kitD1Env    = "COMPASS_KIT_D1"
)

// KitD1Instance describe una instancia del worker (worker/instances/<nombre>).
type KitD1Instance struct {
	Name       string   `json:"name"`
	WorkerName string   `json:"workerName"`
	WorkerURL  string   `json:"workerUrl"`
	Namespace  string   `json:"namespace"`
	KeyPrefix  string   `json:"keyPrefix"`
	HasPackage bool     `json:"hasPackage"`
	HasSchema  bool     `json:"hasSchema"`
	HasKeys    bool     `json:"hasKeys"`
	Scripts    []string `json:"scripts"`
}

// KitD1Consumer es un proyecto que consume el módulo o el worker del kit.
type KitD1Consumer struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Version string `json:"version"`
	// Local es true cuando un go.work redirige el módulo al checkout local.
	Local bool `json:"local"`
	// Engine es "go" (módulo syncengine) o "flutter" (D1SyncEngine propio).
	Engine string `json:"engine"`
}

// KitD1Info es el estado completo que pinta el panel.
type KitD1Info struct {
	Path      string          `json:"path"`
	Exists    bool            `json:"exists"`
	GoVersion string          `json:"goVersion"`
	GitBranch string          `json:"gitBranch"`
	GitDirty  bool            `json:"gitDirty"`
	LastTag   string          `json:"lastTag"`
	Instances []KitD1Instance `json:"instances"`
	Consumers []KitD1Consumer `json:"consumers"`
	Error     string          `json:"error"`
}

// KitD1ActionDef describe una acción del catálogo para el frontend.
type KitD1ActionDef struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	// NeedsInstance indica que la acción requiere elegir una instancia.
	NeedsInstance bool `json:"needsInstance"`
	// Dangerous marca acciones que piden confirmación de dos pasos en la UI.
	Dangerous bool `json:"dangerous"`
	// Group agrupa los botones en el panel ("worker", "nube", "claves", "kit").
	Group string `json:"group"`
}

var kitD1Actions = []KitD1ActionDef{
	{ID: "assemble", Label: "Ensamblar worker", Description: "Copia template/src al directorio de la instancia.", NeedsInstance: true, Group: "worker"},
	{ID: "typecheck", Label: "Typecheck", Description: "npm run typecheck en la instancia.", NeedsInstance: true, Group: "worker"},
	{ID: "dev", Label: "Worker local (dev)", Description: "wrangler dev en 127.0.0.1.", NeedsInstance: true, Group: "worker"},
	{ID: "deploy", Label: "Deploy a producción", Description: "Ensambla, typecheck y despliega el worker.", NeedsInstance: true, Dangerous: true, Group: "nube"},
	{ID: "provision-dry", Label: "Provision (dry-run)", Description: "Simula el aprovisionamiento sin tocar la nube.", NeedsInstance: true, Group: "nube"},
	{ID: "provision", Label: "Provision real", Description: "Aplica infraestructura D1/KV en Cloudflare.", NeedsInstance: true, Dangerous: true, Group: "nube"},
	{ID: "whoami", Label: "Wrangler whoami", Description: "Muestra la cuenta de Cloudflare con sesión activa.", NeedsInstance: true, Group: "nube"},
	{ID: "login", Label: "Wrangler login", Description: "Abre el navegador para iniciar sesión en Cloudflare.", NeedsInstance: true, Group: "nube"},
	{ID: "admin-list", Label: "Listar cuentas", Description: "Lista tenants registrados (requiere SYNC_ADMIN_ID_TOKEN).", NeedsInstance: true, Group: "claves"},
	{ID: "key-export", Label: "Exportar clave", Description: "Re-emite el archivo .key de un tenant existente.", NeedsInstance: true, Group: "claves"},
	{ID: "key-rotate", Label: "Rotar clave", Description: "Crea y registra una clave nueva (la vieja sigue activa).", NeedsInstance: true, Dangerous: true, Group: "claves"},
	{ID: "tag", Label: "Crear tag del módulo", Description: "Etiqueta el módulo Go con una versión semver.", Dangerous: true, Group: "kit"},
}

// kitD1Root resuelve la raíz del kit. Orden: setting explícito, variable de
// entorno, hermano "Core Station/Kit D1" del proyecto activo y del exe.
func (b *Bridge) kitD1Root() string {
	s := settings.Load()
	cands := []string{}
	if strings.TrimSpace(s.KitD1Path) != "" {
		cands = append(cands, s.KitD1Path)
	}
	if v := strings.TrimSpace(os.Getenv(kitD1Env)); v != "" {
		cands = append(cands, v)
	}
	base := b.projectDir()
	if base != "" {
		cands = append(cands, filepath.Join(filepath.Dir(base), "Core Station", "Kit D1"))
		cands = append(cands, filepath.Join(base, "Core Station", "Kit D1"))
	}
	if exe, err := os.Executable(); err == nil {
		cands = append(cands, filepath.Join(filepath.Dir(filepath.Dir(exe)), "Core Station", "Kit D1"))
		cands = append(cands, filepath.Join(filepath.Dir(exe), "Core Station", "Kit D1"))
	}
	for _, c := range cands {
		if c == "" {
			continue
		}
		if abs, err := filepath.Abs(c); err == nil {
			c = abs
		}
		if fi, err := os.Stat(filepath.Join(c, "worker", "instances")); err == nil && fi.IsDir() {
			return c
		}
	}
	if strings.TrimSpace(s.KitD1Path) != "" {
		return s.KitD1Path
	}
	return ""
}

// KitD1Info devuelve el estado del kit: módulo, git, instancias y consumidores.
// No consulta Wrangler (puede tardar); el frontend lo pide aparte con Whoami.
func (b *Bridge) KitD1Info() KitD1Info {
	root := b.kitD1Root()
	info := KitD1Info{
		Path:      root,
		Exists:    root != "" && dirExists(filepath.Join(root, "worker", "instances")),
		Instances: []KitD1Instance{},
		Consumers: []KitD1Consumer{},
	}
	if !info.Exists {
		info.Error = "No se encontró Kit D1. Fija la ruta con 'Elegir ruta'."
		return info
	}
	if v, err := os.ReadFile(filepath.Join(root, "go", "syncengine", "go.mod")); err == nil {
		if m := regexp.MustCompile(`(?m)^go\s+(\S+)`).FindStringSubmatch(string(v)); len(m) == 2 {
			info.GoVersion = m[1]
		}
	}
	if out, err := runGit(root, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		info.GitBranch = out
	}
	if out, err := runGit(root, "status", "--porcelain"); err == nil && strings.TrimSpace(out) != "" {
		info.GitDirty = true
	}
	if out, err := runGit(root, "describe", "--tags", "--abbrev=0"); err == nil {
		info.LastTag = out
	}
	info.Instances = kitD1Instances(root)
	info.Consumers = b.kitD1Consumers(root)
	return info
}

// KitD1Whoami consulta la sesión de Wrangler con un plazo corto. Se llama
// aparte al abrir el modal para no retrasar el resto del estado.
func (b *Bridge) KitD1Whoami() string {
	root := b.kitD1Root()
	if root == "" {
		return "Kit D1 no encontrado"
	}
	dir := ""
	for _, ins := range kitD1Instances(root) {
		if ins.HasPackage {
			dir = filepath.Join(root, "worker", "instances", ins.Name)
			break
		}
	}
	if dir == "" {
		dir = root
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "cmd", "/c", "npx wrangler whoami")
	windialog.NoWindow(cmd)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if ctx.Err() != nil {
		return "Wrangler tardó demasiado en responder"
	}
	if err != nil && text == "" {
		return "Sin sesión de Wrangler (ejecuta Wrangler login)"
	}
	if text == "" {
		return "Sin sesión de Wrangler"
	}
	return text
}

// KitD1Actions devuelve el catálogo de acciones para dibujar los botones.
func (b *Bridge) KitD1Actions() []KitD1ActionDef { return kitD1Actions }

// KitD1SetPath guarda la ruta del kit en settings y devuelve el estado nuevo.
func (b *Bridge) KitD1SetPath(path string) KitD1Info {
	s := settings.Load()
	s.KitD1Path = strings.TrimSpace(path)
	settings.Save(s)
	return b.KitD1Info()
}

// KitD1PickPath abre el selector nativo de carpetas para elegir la raíz del kit.
func (b *Bridge) KitD1PickPath() KitD1Info {
	if app == nil {
		return b.KitD1Info()
	}
	dir, err := app.Dialog.OpenFile().
		SetTitle("Elegir raíz de Kit D1").
		CanChooseFiles(false).
		CanChooseDirectories(true).
		PromptForSingleSelection()
	if err != nil || strings.TrimSpace(dir) == "" {
		return b.KitD1Info()
	}
	return b.KitD1SetPath(dir)
}

// KitD1Script arma el script PowerShell que corre una acción en la ConPTY.
// La acción y la instancia se validan contra el catálogo y el disco: nunca se
// intercala texto libre del usuario en el comando.
func (b *Bridge) KitD1Script(action, instance, arg string) string {
	valid := false
	for _, a := range kitD1Actions {
		if a.ID == action {
			valid = a.NeedsInstance
			break
		}
	}
	if action == "tag" {
		valid = true
	}
	if !valid {
		return "Write-Host 'Acción no válida' -ForegroundColor Red"
	}
	root := b.kitD1Root()
	if root == "" {
		return "Write-Host 'Kit D1 no encontrado' -ForegroundColor Red"
	}
	if action != "tag" {
		if !kitD1ValidInstance(root, instance) {
			return "Write-Host 'Instancia no válida' -ForegroundColor Red"
		}
	}
	kit := psQuote(root)
	instDir := filepath.Join(root, "worker", "instances", instance)
	dir := psQuote(instDir)

	var cmd string
	switch action {
	case "assemble":
		cmd = fmt.Sprintf("Set-Location -LiteralPath %s; node scripts/build-instance.mjs %s", kit, psQuote(instance))
	case "typecheck":
		cmd = fmt.Sprintf("Set-Location -LiteralPath %s; npm run typecheck", dir)
	case "dev":
		cmd = fmt.Sprintf("Set-Location -LiteralPath %s; npm run dev", dir)
	case "deploy":
		cmd = fmt.Sprintf("Set-Location -LiteralPath %s; node scripts/build-instance.mjs %s; if ($LASTEXITCODE -ne 0) { exit 1 }; Set-Location -LiteralPath %s; npm run typecheck; if ($LASTEXITCODE -ne 0) { exit 1 }; npm run deploy", kit, psQuote(instance), dir)
	case "provision-dry":
		cmd = fmt.Sprintf("Set-Location -LiteralPath %s; node scripts/provision.mjs --dir %s --dry-run", kit, dir)
	case "provision":
		cmd = fmt.Sprintf("Set-Location -LiteralPath %s; node scripts/provision.mjs --dir %s", kit, dir)
	case "whoami":
		cmd = fmt.Sprintf("Set-Location -LiteralPath %s; npx wrangler whoami", dir)
	case "login":
		cmd = fmt.Sprintf("Set-Location -LiteralPath %s; npx wrangler login", dir)
	case "admin-list":
		cmd = fmt.Sprintf("Set-Location -LiteralPath %s; node scripts/admin.mjs --dir %s list", kit, dir)
	case "key-export":
		cmd = fmt.Sprintf("Set-Location -LiteralPath %s; node scripts/generate-key.mjs --dir %s", kit, dir)
	case "key-rotate":
		cmd = fmt.Sprintf("Set-Location -LiteralPath %s; node scripts/generate-key.mjs --dir %s --nueva", kit, dir)
	case "tag":
		ver := strings.TrimSpace(arg)
		if !regexp.MustCompile(`^v?\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`).MatchString(ver) {
			return "Write-Host 'Versión no válida (usa v0.1.0)' -ForegroundColor Red"
		}
		if !strings.HasPrefix(ver, "v") {
			ver = "v" + ver
		}
		cmd = fmt.Sprintf("git -C %s tag -a %s -m %s; git -C %s push origin %s", kit, psQuote(ver), psQuote("Release "+ver), kit, psQuote(ver))
	}
	return cmd + fmt.Sprintf("; Write-Host '--- fin (%s) ---' -ForegroundColor Cyan", action)
}

// --- helpers ---------------------------------------------------------------

func dirExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// kitD1ValidInstance confirma que el nombre existe en worker/instances y no
// contiene separadores (evita path traversal desde el frontend).
func kitD1ValidInstance(root, name string) bool {
	if name == "" || strings.ContainsAny(name, `/\..`) {
		return false
	}
	return dirExists(filepath.Join(root, "worker", "instances", name))
}

func kitD1Instances(root string) []KitD1Instance {
	base := filepath.Join(root, "worker", "instances")
	entries, err := os.ReadDir(base)
	if err != nil {
		return []KitD1Instance{}
	}
	out := make([]KitD1Instance, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(base, e.Name())
		ins := KitD1Instance{
			Name:       e.Name(),
			HasPackage: fileExists(filepath.Join(dir, "package.json")),
			HasSchema:  fileExists(filepath.Join(dir, "sync.schema.json")),
			HasKeys:    fileExists(filepath.Join(dir, "sync-keys.json")),
			Scripts:    []string{},
		}
		if b, err := os.ReadFile(filepath.Join(dir, "wrangler.toml")); err == nil {
			if m := regexp.MustCompile(`(?m)^name\s*=\s*"([^"]+)"`).FindSubmatch(b); len(m) == 2 {
				ins.WorkerName = string(m[1])
			}
		}
		if b, err := os.ReadFile(filepath.Join(dir, "sync-project.json")); err == nil {
			var cfg struct {
				WorkerURL string `json:"workerUrl"`
				Namespace string `json:"namespace"`
				KeyPrefix string `json:"keyPrefix"`
			}
			if json.Unmarshal(b, &cfg) == nil {
				ins.WorkerURL = cfg.WorkerURL
				ins.Namespace = cfg.Namespace
				ins.KeyPrefix = cfg.KeyPrefix
			}
		}
		if b, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil {
			var pkg struct {
				Scripts map[string]string `json:"scripts"`
			}
			if json.Unmarshal(b, &pkg) == nil {
				for k := range pkg.Scripts {
					ins.Scripts = append(ins.Scripts, k)
				}
				sort.Strings(ins.Scripts)
			}
		}
		out = append(out, ins)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// kitD1Consumers busca proyectos que usan el módulo Go (go.mod/go.work) o el
// worker (Flutter) entre los proyectos del workspace y los hermanos del kit.
func (b *Bridge) kitD1Consumers(root string) []KitD1Consumer {
	seen := map[string]bool{}
	out := []KitD1Consumer{}
	add := func(dir string) {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return
		}
		key := strings.ToLower(abs)
		if seen[key] {
			return
		}
		seen[key] = true
		c := KitD1Consumer{Name: filepath.Base(abs), Path: abs}
		if b, err := os.ReadFile(filepath.Join(abs, "go.mod")); err == nil {
			mod := string(b)
			if strings.Contains(mod, kitD1Module) {
				if m := regexp.MustCompile(`kit-d1/go/syncengine\s+(v\S+)`).FindStringSubmatch(mod); len(m) == 2 {
					c.Version = m[1]
				}
				c.Engine = "go"
				if w, err := os.ReadFile(filepath.Join(abs, "go.work")); err == nil && strings.Contains(string(w), kitD1Module) {
					c.Local = true
				}
			}
		}
		if c.Engine == "" && fileExists(filepath.Join(abs, "lib", "services", "d1_sync.dart")) {
			c.Engine = "flutter"
		}
		if c.Engine != "" {
			out = append(out, c)
		}
	}
	for _, p := range b.ws.Projects {
		add(p.Path)
	}
	// Los proyectos suelen vivir como hermanos de "Core Station".
	parent := filepath.Dir(filepath.Dir(root))
	if entries, err := os.ReadDir(parent); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				add(filepath.Join(parent, e.Name()))
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
