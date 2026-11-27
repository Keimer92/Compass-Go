// Package webui — panel de administración de Kit D1.
//
// Expone el estado del kit (módulo Go, instancias del worker, consumidores y
// sesión de Wrangler) y lanza sus acciones en la ConPTY de Compass. El kit es
// global (no por proyecto), así que el panel vive en la barra lateral como
// "Kit D1", igual que "Ajustes de Git".
package webui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
	kitD1Module = "github.com/Keimer92/Compass-Go/Kit-D1/go/syncengine"
	kitD1Env    = "COMPASS_KIT_D1"
)

// Regexes para leer el bloque [[d1_databases]] del wrangler.toml de una
// instancia. reD1Block captura el contenido hasta el siguiente encabezado de
// tabla, porque el marcador [[d1_databases]] puede venir pegado al valor previo.
var (
	reD1Block = regexp.MustCompile(`(?s)\[\[d1_databases\]\](.*?)(?:\[\[|\z)`)
	reD1Name  = regexp.MustCompile(`(?m)^\s*database_name\s*=\s*"([^"]+)"`)
	reD1ID    = regexp.MustCompile(`(?m)^\s*database_id\s*=\s*"([^"]+)"`)
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
	// Base D1 declarada en el wrangler.toml de la instancia.
	DatabaseName string `json:"databaseName"`
	DatabaseID   string `json:"databaseId"`
}

// KitD1Database es una base D1 de Cloudflare con sus métricas de las últimas
// 24 h (las mismas que muestra el dashboard) y la instancia local que la usa.
type KitD1Database struct {
	Name      string `json:"name"`
	ID        string `json:"id"`
	CreatedAt string `json:"createdAt"`
	FileSize  int64  `json:"fileSize"`
	// Instance/Worker identifican el worker local que apunta a esta base.
	Instance string `json:"instance"`
	Worker   string `json:"worker"`
	// Missing marca una base declarada por una instancia pero inexistente en la
	// cuenta de Cloudflare (tipico de un worker ya retirado).
	Missing bool `json:"missing"`
	// Analytics indica si las metricas de 24 h llegaron de Cloudflare.
	Analytics    bool  `json:"analytics"`
	ReadQueries  int64 `json:"readQueries"`
	WriteQueries int64 `json:"writeQueries"`
	RowsRead     int64 `json:"rowsRead"`
	RowsWritten  int64 `json:"rowsWritten"`
}

// KitD1Databases es el estado del panel de bases D1.
type KitD1Databases struct {
	Account   string          `json:"account"`
	Databases []KitD1Database `json:"databases"`
	Error     string          `json:"error"`
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
	{ID: "bootstrap", Label: "Instalar dependencias", Description: "npm install en la instancia (necesario en la copia del instalador).", NeedsInstance: true, Group: "kit"},
}

// kitD1Root resuelve la raíz del kit. Prioriza la copia que viaja con el
// binario (junto al exe): el instalador empaqueta "Kit-D1/" dentro de
// %LOCALAPPDATA%\Programs\Keimer, asi que el binario instalado usa su propia
// copia y no la del repo. Si hay cambios en el kit basta con reinstalar
// Compass Go. En desarrollo (go run/dev.bat) el exe vive en un temporal sin
// Kit-D1, por lo que se cae a las rutas junto al proyecto activo. Se conservan
// los nombres viejos "Kit D1" / "Core Station/Kit D1" como fallback heredado.
func (b *Bridge) kitD1Root() string {
	s := settings.Load()
	exeDir := ""
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}
	for _, c := range kitD1Candidates(s, exeDir, b.projectDir()) {
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

// kitD1Candidates arma la lista ordenada de rutas candidatas para el kit.
// Primero la copia empaquetada junto al binario (exeDir), luego las rutas junto
// al proyecto activo (desarrollo). exeDir vacio o base vacio se omiten.
func kitD1Candidates(s settings.Settings, exeDir, base string) []string {
	cands := []string{}
	if strings.TrimSpace(s.KitD1Path) != "" {
		cands = append(cands, s.KitD1Path)
	}
	if v := strings.TrimSpace(os.Getenv(kitD1Env)); v != "" {
		cands = append(cands, v)
	}
	// 1) Copia empaquetada con el binario (instalacion).
	if exeDir != "" {
		cands = append(cands, filepath.Join(exeDir, "Kit-D1"))
		cands = append(cands, filepath.Join(filepath.Dir(exeDir), "Kit-D1"))
		cands = append(cands, filepath.Join(exeDir, "Kit D1"))
		cands = append(cands, filepath.Join(filepath.Dir(exeDir), "Kit D1"))
		cands = append(cands, filepath.Join(filepath.Dir(exeDir), "Core Station", "Kit D1"))
		cands = append(cands, filepath.Join(exeDir, "Core Station", "Kit D1"))
	}
	// 2) Fallback de desarrollo: junto al proyecto activo.
	if base != "" {
		parent := filepath.Dir(base)
		cands = append(cands, filepath.Join(base, "Kit-D1"))
		cands = append(cands, filepath.Join(parent, "Kit-D1"))
		// Si el proyecto activo es un hermano de Compass (GymPro, School
		// Tracker), el kit esta en <padre>/Compass Go UX/Kit-D1.
		cands = append(cands, filepath.Join(parent, "Compass Go UX", "Kit-D1"))
		cands = append(cands, filepath.Join(filepath.Dir(parent), "Kit-D1"))
		// Legacy: nombres anteriores, conservados como fallback.
		cands = append(cands, filepath.Join(base, "Kit D1"))
		cands = append(cands, filepath.Join(parent, "Kit D1"))
		cands = append(cands, filepath.Join(parent, "Compass Go UX", "Kit D1"))
		cands = append(cands, filepath.Join(parent, "Core Station", "Kit D1"))
		cands = append(cands, filepath.Join(base, "Core Station", "Kit D1"))
	}
	return cands
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
	// El kit vive dentro del repo de Compass: sin --match, git describe tomaria
	// un tag de Compass como si fuera la version del módulo syncengine.
	if out, err := runGit(root, "describe", "--tags", "--match", "Kit-D1/go/syncengine/v*", "--abbrev=0"); err == nil {
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
	return wranglerWhoami(dir, 12*time.Second)
}

// wranglerWhoami corre `npx wrangler whoami` con la salida en un archivo.
//
// Con CombinedOutput en Windows se cuelga: al vencer el plazo Go mata cmd.exe,
// pero el node.exe nieto hereda el pipe y el lector nunca ve EOF, así que el
// modal se queda en "consultando…" para siempre. Con un archivo, Run vuelve
// en cuanto termina el proceso. --yes, CI=1 y el stdin nulo evitan que npx
// espere una confirmación interactiva sin consola.
func wranglerWhoami(dir string, timeout time.Duration) string {
	return whoamiWithCommand(dir, "npx --yes wrangler whoami", timeout)
}

// whoamiWithCommand separa el comando para poder probar el plazo sin red.
func whoamiWithCommand(dir, command string, timeout time.Duration) string {
	text, runErr, timedOut := runWranglerCapture(dir, command, timeout)
	if timedOut {
		return "Wrangler tardó demasiado en responder"
	}
	if runErr != nil && text == "" {
		return "Sin sesión de Wrangler (ejecuta Wrangler login)"
	}
	if text == "" {
		return "Sin sesión de Wrangler"
	}
	return text
}

// runWranglerCapture ejecuta un comando en dir con la salida (stdout y stderr)
// en un archivo temporal. Con CombinedOutput en Windows se cuelga: al vencer el
// plazo Go mata cmd.exe, pero el node.exe nieto hereda el pipe y el lector
// nunca ve EOF. Con un archivo, Run vuelve en cuanto termina el proceso.
// --yes, CI=1 y el stdin nulo evitan que npx espere una confirmación.
func runWranglerCapture(dir, command string, timeout time.Duration) (string, error, bool) {
	f, err := os.CreateTemp("", "compass-wrangler-*.txt")
	if err != nil {
		return "", err, false
	}
	name := f.Name()
	defer os.Remove(name)
	defer f.Close()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "cmd", "/c", command)
	windialog.NoWindow(cmd)
	cmd.Dir = dir
	cmd.Stdout = f
	cmd.Stderr = f
	if devNull, err := os.OpenFile(os.DevNull, os.O_RDONLY, 0); err == nil {
		defer devNull.Close()
		cmd.Stdin = devNull
	}
	cmd.Env = append(os.Environ(), "CI=1")
	runErr := cmd.Run()
	timedOut := ctx.Err() != nil

	out, _ := os.ReadFile(name)
	return strings.TrimSpace(string(out)), runErr, timedOut
}

// kitD1WranglerDir elige la carpeta desde la que correr Wrangler: una instancia
// con package.json (ahi estan las dependencias) o la raiz del kit.
func kitD1WranglerDir(root string) string {
	for _, ins := range kitD1Instances(root) {
		if ins.HasPackage {
			return filepath.Join(root, "worker", "instances", ins.Name)
		}
	}
	return root
}

// KitD1Databases lista las bases D1 de la cuenta con el tamaño, las tablas y
// las métricas de 24 h que muestra el dashboard de Cloudflare, cruzadas con las
// instancias locales que las usan. Es una llamada de red: el panel la pide
// aparte para no bloquear el resto del estado.
func (b *Bridge) KitD1Databases() KitD1Databases {
	root := b.kitD1Root()
	out := KitD1Databases{Databases: []KitD1Database{}}
	if root == "" {
		out.Error = "Kit D1 no encontrado"
		return out
	}
	dir := kitD1WranglerDir(root)

	// 1) Cuenta activa (la misma que usa Wrangler) para las métricas.
	account := strings.TrimSpace(os.Getenv("CLOUDFLARE_ACCOUNT_ID"))
	if account == "" {
		if text, err, timedOut := runWranglerCapture(dir, "npx --yes wrangler whoami --json", 25*time.Second); err == nil && !timedOut {
			var who struct {
				Accounts []struct {
					ID string `json:"id"`
				} `json:"accounts"`
			}
			if decodeFirstJSON(text, &who) == nil && len(who.Accounts) > 0 {
				account = who.Accounts[0].ID
			}
		}
	}
	out.Account = account

	// 2) Bases de la cuenta.
	cloud := []KitD1Database{}
	text, err, timedOut := runWranglerCapture(dir, "npx --yes wrangler d1 list --json", 30*time.Second)
	switch {
	case timedOut:
		out.Error = "Wrangler tardó demasiado al listar las bases"
	case err != nil && strings.TrimSpace(text) == "":
		out.Error = "Sin sesión de Wrangler (ejecuta Wrangler login)"
	default:
		var rows []struct {
			UUID      string `json:"uuid"`
			Name      string `json:"name"`
			CreatedAt string `json:"created_at"`
			FileSize  int64  `json:"file_size"`
		}
		if e := decodeFirstJSON(text, &rows); e != nil {
			out.Error = "No se pudo leer la lista de bases de Wrangler"
		} else {
			for _, r := range rows {
				cloud = append(cloud, KitD1Database{
					Name: r.Name, ID: r.UUID, CreatedAt: r.CreatedAt, FileSize: r.FileSize,
				})
			}
		}
	}

	// 3) Métricas de 24 h (GraphQL). Si falla, se muestran solo tamaño/tablas.
	stats := d1Analytics(cloudflareToken(), account)
	out.Databases = mergeKitD1Databases(cloud, kitD1Instances(root), stats)
	return out
}

// mergeKitD1Databases cruza las bases de la cuenta con las instancias locales
// (por database_id) y con las métricas de 24 h. Las bases que una instancia
// declara pero que ya no existen en la cuenta se listan con Missing=true, para
// poder ver y retirar el worker huérfano que las apunta.
func mergeKitD1Databases(cloud []KitD1Database, instances []KitD1Instance, stats map[string]d1Stats) []KitD1Database {
	byID := map[string]KitD1Instance{}
	for _, ins := range instances {
		if ins.DatabaseID != "" {
			byID[strings.ToLower(ins.DatabaseID)] = ins
		}
	}
	out := make([]KitD1Database, 0, len(cloud)+len(instances))
	seen := map[string]bool{}
	for _, db := range cloud {
		key := strings.ToLower(db.ID)
		seen[key] = true
		if ins, ok := byID[key]; ok {
			db.Instance, db.Worker = ins.Name, ins.WorkerName
		}
		if st, ok := stats[key]; ok {
			db.Analytics = true
			db.ReadQueries, db.WriteQueries = st.ReadQueries, st.WriteQueries
			db.RowsRead, db.RowsWritten = st.RowsRead, st.RowsWritten
		}
		out = append(out, db)
	}
	for _, ins := range instances {
		key := strings.ToLower(ins.DatabaseID)
		if ins.DatabaseID == "" || seen[key] {
			continue
		}
		out = append(out, KitD1Database{
			Name: ins.DatabaseName, ID: ins.DatabaseID,
			Instance: ins.Name, Worker: ins.WorkerName, Missing: true,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// d1Stats son los contadores de 24 h de una base.
type d1Stats struct {
	ReadQueries  int64
	WriteQueries int64
	RowsRead     int64
	RowsWritten  int64
}

// d1Analytics consulta la API GraphQL de Cloudflare con el token de Wrangler.
// Es la misma fuente que usa el dashboard para "Queries", "Rows read" y
// "Rows written" de las últimas 24 h.
func d1Analytics(token, account string) map[string]d1Stats {
	if strings.TrimSpace(token) == "" || strings.TrimSpace(account) == "" {
		return nil
	}
	const query = `query($acc:String!,$since:Time!){viewer{accounts(filter:{accountTag:$acc}){` +
		`d1AnalyticsAdaptiveGroups(limit:100,filter:{datetime_geq:$since}){` +
		`dimensions{databaseId}sum{readQueries writeQueries rowsRead rowsWritten}}}}}`
	body, err := json.Marshal(map[string]any{
		"query": query,
		"variables": map[string]string{
			"acc":   account,
			"since": time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339),
		},
	})
	if err != nil {
		return nil
	}
	req, err := http.NewRequest(http.MethodPost,
		"https://api.cloudflare.com/client/v4/graphql", bytes.NewReader(body))
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var parsed struct {
		Data struct {
			Viewer struct {
				Accounts []struct {
					Groups []struct {
						Dimensions struct {
							DatabaseID string `json:"databaseId"`
						} `json:"dimensions"`
						Sum struct {
							ReadQueries  float64 `json:"readQueries"`
							WriteQueries float64 `json:"writeQueries"`
							RowsRead     float64 `json:"rowsRead"`
							RowsWritten  float64 `json:"rowsWritten"`
						} `json:"sum"`
					} `json:"d1AnalyticsAdaptiveGroups"`
				} `json:"accounts"`
			} `json:"viewer"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil
	}
	out := map[string]d1Stats{}
	for _, acc := range parsed.Data.Viewer.Accounts {
		for _, g := range acc.Groups {
			if g.Dimensions.DatabaseID == "" {
				continue
			}
			out[strings.ToLower(g.Dimensions.DatabaseID)] = d1Stats{
				ReadQueries:  int64(g.Sum.ReadQueries),
				WriteQueries: int64(g.Sum.WriteQueries),
				RowsRead:     int64(g.Sum.RowsRead),
				RowsWritten:  int64(g.Sum.RowsWritten),
			}
		}
	}
	return out
}

// cloudflareToken devuelve el token para la API: primero CLOUDFLARE_API_TOKEN
// (CI/automatizacion) y si no el que guarda Wrangler al hacer login. Nunca sale
// del proceso: solo se usa para la consulta GraphQL.
func cloudflareToken() string {
	if t := strings.TrimSpace(os.Getenv("CLOUDFLARE_API_TOKEN")); t != "" {
		return t
	}
	var paths []string
	if a := strings.TrimSpace(os.Getenv("APPDATA")); a != "" {
		// Wrangler mapea %APPDATA% a XDG_CONFIG_HOME en Windows.
		paths = append(paths, filepath.Join(a, "xdg.config", ".wrangler", "config", "default.toml"))
	}
	if x := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); x != "" {
		paths = append(paths, filepath.Join(x, ".wrangler", "config", "default.toml"))
	}
	if h, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(h, ".config", ".wrangler", "config", "default.toml"))
	}
	re := regexp.MustCompile(`(?m)^oauth_token\s*=\s*"([^"]+)"`)
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if m := re.FindSubmatch(data); len(m) == 2 {
			return string(m[1])
		}
	}
	return ""
}

// decodeFirstJSON decodifica el primer valor JSON del texto. Wrangler mezcla
// avisos en el mismo flujo que el --json, asi que no se puede decodificar el
// archivo entero.
func decodeFirstJSON(text string, v any) error {
	for i := 0; i < len(text); i++ {
		if text[i] != '{' && text[i] != '[' {
			continue
		}
		dec := json.NewDecoder(strings.NewReader(text[i:]))
		if err := dec.Decode(v); err == nil {
			return nil
		}
	}
	return fmt.Errorf("no se encontro JSON en la salida")
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
	if !valid {
		return "Write-Host 'Acción no válida' -ForegroundColor Red"
	}
	root := b.kitD1Root()
	if root == "" {
		return "Write-Host 'Kit D1 no encontrado' -ForegroundColor Red"
	}
	if !kitD1ValidInstance(root, instance) {
		return "Write-Host 'Instancia no válida' -ForegroundColor Red"
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
	case "bootstrap":
		cmd = fmt.Sprintf("Set-Location -LiteralPath %s; npm install", dir)
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
			// Base D1 declarada por la instancia. El encabezado [[d1_databases]]
			// no siempre empieza linea: algunos wrangler.toml lo pegan al valor
			// anterior, asi que se busca el bloque y se lee dentro de el.
			if blk := reD1Block.FindSubmatch(b); len(blk) == 2 {
				if m := reD1Name.FindSubmatch(blk[1]); len(m) == 2 {
					ins.DatabaseName = string(m[1])
				}
				if m := reD1ID.FindSubmatch(blk[1]); len(m) == 2 {
					ins.DatabaseID = string(m[1])
				}
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
				if m := regexp.MustCompile(`Kit-D1/go/syncengine\s+(v\S+)`).FindStringSubmatch(mod); len(m) == 2 {
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
