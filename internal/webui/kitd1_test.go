package webui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"compass/internal/settings"
	"compass/internal/workspace"
)

// kitFixture arma un checkout minimo de Kit D1 con dos instancias y un
// consumidor Go, para probar el panel sin depender del kit real.
func kitFixture(t *testing.T) (root string, b *Bridge) {
	t.Helper()
	root = t.TempDir()
	for _, name := range []string{"alpha", "beta"} {
		dir := filepath.Join(root, "worker", "instances", name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		_ = os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"scripts":{"dev":"wrangler dev"}}`), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "sync.schema.json"), []byte(`{"version":1,"tables":[]}`), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "wrangler.toml"), []byte("name = \"worker-"+name+"\"\n"), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "sync-project.json"), []byte(`{"workerUrl":"https://x.workers.dev","namespace":"ns-`+name+`","keyPrefix":"kp_"}`), 0o644)
	}
	t.Setenv(kitD1Env, root)
	b = &Bridge{dir: root, ws: &workspace.Workspace{Projects: []*workspace.Project{}, Active: -1}}
	return root, b
}

func TestKitD1InfoLeeInstancias(t *testing.T) {
	root, b := kitFixture(t)
	b.ws.Projects = append(b.ws.Projects, &workspace.Project{Path: root})
	info := b.KitD1Info()
	if !info.Exists {
		t.Fatalf("Exists=false para %s", root)
	}
	if len(info.Instances) != 2 {
		t.Fatalf("esperaba 2 instancias, got %d", len(info.Instances))
	}
	if info.Instances[0].Name != "alpha" || info.Instances[1].Name != "beta" {
		t.Fatalf("instancias desordenadas: %+v", info.Instances)
	}
	if info.Instances[0].WorkerName != "worker-alpha" {
		t.Fatalf("workerName=%q", info.Instances[0].WorkerName)
	}
	if info.Instances[0].WorkerURL != "https://x.workers.dev" {
		t.Fatalf("workerUrl=%q", info.Instances[0].WorkerURL)
	}
	if !info.Instances[0].HasPackage || !info.Instances[0].HasSchema {
		t.Fatalf("flags de archivos a false: %+v", info.Instances[0])
	}
}

func TestKitD1InfoConsumidores(t *testing.T) {
	root, b := kitFixture(t)
	// Un proyecto hermano del kit usa el modulo y tiene go.work local.
	parent := filepath.Dir(filepath.Dir(root))
	// El fixture crea el kit directamente en un temp, asi que forzamos el
	// consumidor anadiendolo al workspace.
	consumer := filepath.Join(t.TempDir(), "mi-app")
	_ = os.MkdirAll(consumer, 0o755)
	_ = os.WriteFile(filepath.Join(consumer, "go.mod"), []byte("require github.com/Keimer92/Compass-Go/Kit-D1/go/syncengine v0.1.0\n"), 0o644)
	_ = os.WriteFile(filepath.Join(consumer, "go.work"), []byte("replace github.com/Keimer92/Compass-Go/Kit-D1/go/syncengine => \"D:/kit\"\n"), 0o644)
	b.ws.Projects = append(b.ws.Projects, &workspace.Project{Path: consumer})
	info := b.KitD1Info()
	var found *KitD1Consumer
	for i := range info.Consumers {
		if info.Consumers[i].Name == "mi-app" {
			found = &info.Consumers[i]
		}
	}
	if found == nil {
		t.Fatalf("consumidor mi-app no detectado: %+v", info.Consumers)
	}
	if found.Version != "v0.1.0" || !found.Local || found.Engine != "go" {
		t.Fatalf("consumidor mal leido: %+v", found)
	}
	_ = parent
}

// TestKitD1InstancesLeeBaseD1 comprueba el cruce con wrangler.toml, incluido el
// encabezado [[d1_databases]] pegado al valor anterior (formato real de
// ela-academy-flutter) y varios bloques de tabla en el mismo archivo.
func TestKitD1InstancesLeeBaseD1(t *testing.T) {
	root := t.TempDir()
	pega := filepath.Join(root, "worker", "instances", "pegada")
	if err := os.MkdirAll(pega, 0o755); err != nil {
		t.Fatal(err)
	}
	toml := "name = \"pegada-d1-sync\"\ncompatibility_date = \"2026-11-04\"[[d1_databases]]\n" +
		"binding = \"DB\"\ndatabase_name = \"pegada\"\ndatabase_id = \"AAA-111\"[[kv_namespaces]]\n" +
		"binding = \"ROUTING\"\nid = \"kv1\"\n"
	if err := os.WriteFile(filepath.Join(pega, "wrangler.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	otra := filepath.Join(root, "worker", "instances", "otra")
	if err := os.MkdirAll(otra, 0o755); err != nil {
		t.Fatal(err)
	}
	toml2 := "name = \"otra\"\n\n[[kv_namespaces]]\nbinding = \"ROUTING\"\nid = \"kv2\"\n\n" +
		"[[d1_databases]]\nbinding = \"DB\"\ndatabase_name = \"otra-base\"\ndatabase_id = \"BBB-222\"\n"
	if err := os.WriteFile(filepath.Join(otra, "wrangler.toml"), []byte(toml2), 0o644); err != nil {
		t.Fatal(err)
	}

	got := map[string]KitD1Instance{}
	for _, ins := range kitD1Instances(root) {
		got[ins.Name] = ins
	}
	if ins := got["pegada"]; ins.WorkerName != "pegada-d1-sync" || ins.DatabaseName != "pegada" || ins.DatabaseID != "AAA-111" {
		t.Fatalf("pegada mal leida: %+v", ins)
	}
	if ins := got["otra"]; ins.DatabaseName != "otra-base" || ins.DatabaseID != "BBB-222" {
		t.Fatalf("otra mal leida: %+v", ins)
	}
}

// TestMergeKitD1Databases cubre el cruce bases↔instancias↔métricas y el caso de
// la base declarada por una instancia que ya no existe en la cuenta.
func TestMergeKitD1Databases(t *testing.T) {
	cloud := []KitD1Database{
		{Name: "zeta", ID: "id-z", FileSize: 2048},
		{Name: "alfa", ID: "ID-A", FileSize: 512},
	}
	instances := []KitD1Instance{
		{Name: "alfa-inst", WorkerName: "alfa-worker", DatabaseName: "alfa", DatabaseID: "id-a"},
		{Name: "huerfana", WorkerName: "school-tracker-flutter", DatabaseName: "school-tracker-flutter", DatabaseID: "id-muerta"},
		{Name: "sin-base"},
	}
	stats := map[string]d1Stats{"id-z": {ReadQueries: 7, WriteQueries: 3, RowsRead: 100, RowsWritten: 9}}

	got := mergeKitD1Databases(cloud, instances, stats)
	if len(got) != 3 {
		t.Fatalf("esperaba 3 bases, got %d: %+v", len(got), got)
	}
	if got[0].Name != "alfa" || got[1].Name != "school-tracker-flutter" || got[2].Name != "zeta" {
		t.Fatalf("orden inesperado: %+v", got)
	}
	// El cruce por id ignora mayusculas.
	if got[0].Instance != "alfa-inst" || got[0].Worker != "alfa-worker" || got[0].Missing {
		t.Fatalf("alfa mal cruzada: %+v", got[0])
	}
	// La base muerta se marca para poder limpiar el worker que la apunta.
	if !got[1].Missing || got[1].Instance != "huerfana" {
		t.Fatalf("base huerfana mal marcada: %+v", got[1])
	}
	// Métricas de 24 h en la base que las tiene.
	if !got[2].Analytics || got[2].RowsRead != 100 || got[2].WriteQueries != 3 {
		t.Fatalf("metricas no aplicadas: %+v", got[2])
	}
}

func TestKitD1ScriptValidaAcciones(t *testing.T) {
	_, b := kitFixture(t)

	if s := b.KitD1Script("assemble", "alpha", ""); !strings.Contains(s, "build-instance.mjs") {
		t.Fatalf("assemble sin build-instance: %s", s)
	}
	if s := b.KitD1Script("deploy", "alpha", ""); !strings.Contains(s, "npm run deploy") {
		t.Fatalf("deploy sin deploy: %s", s)
	}
	if s := b.KitD1Script("key-rotate", "alpha", ""); !strings.Contains(s, "--nueva") {
		t.Fatalf("rotate sin --nueva: %s", s)
	}
	if s := b.KitD1Script("bootstrap", "alpha", ""); !strings.Contains(s, "npm install") {
		t.Fatalf("bootstrap sin npm install: %s", s)
	}
	// Instancia inexistente o con traversal: se rechaza.
	if s := b.KitD1Script("dev", "no-existe", ""); !strings.Contains(s, "no válida") {
		t.Fatalf("instancia inexistente aceptada: %s", s)
	}
	if s := b.KitD1Script("dev", "../alpha", ""); !strings.Contains(s, "no válida") {
		t.Fatalf("traversal aceptado: %s", s)
	}
	// Accion desconocida (incluida la antigua 'tag'): se rechaza.
	if s := b.KitD1Script("borrar-todo", "alpha", ""); !strings.Contains(s, "no válida") {
		t.Fatalf("accion desconocida aceptada: %s", s)
	}
	if s := b.KitD1Script("tag", "alpha", "v0.1.0"); !strings.Contains(s, "no válida") {
		t.Fatalf("accion tag retirada todavia aceptada: %s", s)
	}
}

func TestWranglerWhoamiLeeSalida(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("cmd.exe requiere Windows")
	}
	if got := whoamiWithCommand(t.TempDir(), "echo sesion-falsa", 5*time.Second); got != "sesion-falsa" {
		t.Fatalf("whoami = %q, queria sesion-falsa", got)
	}
}

// El hijo que hereda el pipe es lo que colgaba el modal: con salida a archivo,
// el plazo tiene que devolver aunque el comando siga corriendo.
func TestWranglerWhoamiCaducaSinColgar(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("cmd.exe requiere Windows")
	}
	start := time.Now()
	got := whoamiWithCommand(t.TempDir(), "for /l %i in (1,1,999999) do @rem", 400*time.Millisecond)
	if got != "Wrangler tardó demasiado en responder" {
		t.Fatalf("whoami = %q, queria aviso de timeout", got)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("whoami tardó %s pese al plazo de 400ms", time.Since(start))
	}
}

func TestKitD1AccionesCatalogo(t *testing.T) {
	ids := map[string]bool{}
	for _, a := range kitD1Actions {
		if ids[a.ID] {
			t.Fatalf("accion duplicada: %s", a.ID)
		}
		ids[a.ID] = true
		if !a.NeedsInstance {
			t.Fatalf("accion %s deberia needing instance", a.ID)
		}
	}
	for _, want := range []string{"assemble", "typecheck", "dev", "deploy", "provision-dry", "provision", "whoami", "login", "admin-list", "key-export", "key-rotate", "bootstrap"} {
		if !ids[want] {
			t.Fatalf("falta accion %s", want)
		}
	}
	if ids["tag"] {
		t.Fatal("la accion tag no debe estar en el catalogo (repo remoto archivado)")
	}
}

// TestKitD1RootDentroDeCompass comprueba que el kit se autodetecta cuando vive
// como subcarpeta "Kit-D1/" del proyecto activo (layout del subtree).
func TestKitD1RootDentroDeCompass(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv(kitD1Env, "")
	compass := filepath.Join(t.TempDir(), "Compass Go UX")
	kit := filepath.Join(compass, "Kit-D1")
	if err := os.MkdirAll(filepath.Join(kit, "worker", "instances"), 0o755); err != nil {
		t.Fatal(err)
	}
	b := &Bridge{dir: compass, ws: &workspace.Workspace{Projects: []*workspace.Project{}, Active: -1}}
	if got := b.kitD1Root(); got != kit {
		t.Fatalf("kitD1Root = %q, queria %q", got, kit)
	}
	// La deteccion del proyecto activo tambien debe encontrarlo subiendo un nivel.
	sub := filepath.Join(compass, "internal", "webui")
	b.dir = sub
	if got := b.kitD1Root(); got != kit {
		t.Fatalf("desde subdirectorio kitD1Root = %q, queria %q", got, kit)
	}
}

// La copia junto al exe (instalacion) debe tener prioridad sobre la del
// proyecto: asi el binario instalado usa sus propios kits y basta con
// reinstalar Compass Go cuando cambie el kit.
func TestKitD1CandidatesExePrimero(t *testing.T) {
	t.Setenv(kitD1Env, "")
	exeDir := filepath.Join(t.TempDir(), "Programs", "Keimer")
	base := filepath.Join(t.TempDir(), "Compass Go UX")
	cands := kitD1Candidates(settings.Settings{}, exeDir, base)

	wantExe := filepath.Join(exeDir, "Kit-D1")
	wantProj := filepath.Join(base, "Kit-D1")
	idxExe, idxProj := -1, -1
	for i, c := range cands {
		switch c {
		case wantExe:
			idxExe = i
		case wantProj:
			idxProj = i
		}
	}
	if idxExe < 0 {
		t.Fatalf("no se propuso la copia del exe %q: %v", wantExe, cands)
	}
	if idxProj < 0 {
		t.Fatalf("no se propuso la copia del proyecto %q: %v", wantProj, cands)
	}
	if idxExe > idxProj {
		t.Fatalf("la copia del exe (%d) debe ir antes que la del proyecto (%d)", idxExe, idxProj)
	}
}

// TestProjectKitsInsignias comprueba la deteccion que alimenta las insignias de
// la tarjeta: modulo Go del kit o servicio D1 de Flutter, y marcador del motor
// de WhatsApp.
func TestProjectKitsInsignias(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv(kitD1Env, "")

	goConsumer := t.TempDir()
	if err := os.WriteFile(filepath.Join(goConsumer, "go.mod"),
		[]byte("module demo\n\nrequire github.com/Keimer92/Compass-Go/Kit-D1/go/syncengine v0.1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(goConsumer, "use-whatsapp-engine"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	flutterConsumer := t.TempDir()
	if err := os.WriteFile(filepath.Join(flutterConsumer, "pubspec.yaml"), []byte("name: demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(flutterConsumer, "lib", "services"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(flutterConsumer, "lib", "services", "d1_sync.dart"), []byte("// d1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	plain := t.TempDir()
	if err := os.WriteFile(filepath.Join(plain, "go.mod"), []byte("module plano\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	b := &Bridge{ws: &workspace.Workspace{Projects: []*workspace.Project{
		{Path: goConsumer}, {Path: flutterConsumer}, {Path: plain}, nil,
	}}}
	got := map[string]ProjectKitInfo{}
	for _, k := range b.ProjectKits() {
		got[k.Path] = k
	}
	if k := got[goConsumer]; !k.Whatsapp || !k.D1 {
		t.Fatalf("consumidor Go mal detectado: %+v", k)
	}
	if k := got[flutterConsumer]; k.Whatsapp || !k.D1 {
		t.Fatalf("consumidor Flutter mal detectado: %+v", k)
	}
	if k := got[plain]; k.Whatsapp || k.D1 {
		t.Fatalf("proyecto sin kits marcado: %+v", k)
	}
	if len(got) != 3 {
		t.Fatalf("se esperaban 3 proyectos, hubo %d", len(got))
	}
}

// El .key se emite con la clave que ya está en sync-keys.json, sin tocar la
// red: reemitir no debe rotar, porque cada clave abre su propio espacio de
// datos (sk = sha256(clave)) y los equipos ya vinculados dejarían de ver lo
// nuevo.
func TestKitD1GenerateKeyReusesSavedKey(t *testing.T) {
	root, b := kitFixture(t)
	dir := filepath.Join(root, "worker", "instances", "alpha")
	proyecto := `{
		"namespace": "ns-alpha",
		"keyPrefix": "kp_",
		"routingKv": "SCHOOL_ROUTING",
		"keysFile": "sync-keys.json",
		"defaultTenant": "default",
		"workerUrl": "https://worker-alpha.example.workers.dev"
	}`
	if err := os.WriteFile(filepath.Join(dir, "sync-project.json"), []byte(proyecto), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tenants.json"), []byte(`{"tenants":[{"slug":"nube"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	guardadas := `{"nube":{"name":"nube","schoolKey":"kp_abc123","databaseId":"11111111-2222-3333-4444-555555555555"}}`
	if err := os.WriteFile(filepath.Join(dir, "sync-keys.json"), []byte(guardadas), 0o644); err != nil {
		t.Fatal(err)
	}

	k := b.KitD1GenerateKey("alpha")
	if k.Error != "" {
		t.Fatalf("error inesperado: %s", k.Error)
	}
	if k.Key != "kp_abc123" || k.Source != "keys-file" {
		t.Fatalf("clave=%q source=%q, quería la guardada en sync-keys.json", k.Key, k.Source)
	}
	if k.KeyFile == "" {
		t.Fatal("no se escribió el archivo .key")
	}
	if filepath.Base(k.KeyFile) != "ns-alpha-nube.key" {
		t.Fatalf("nombre del .key = %q", filepath.Base(k.KeyFile))
	}
	blob, err := os.ReadFile(k.KeyFile)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		WorkerURL string `json:"workerUrl"`
		SchoolKey string `json:"schoolKey"`
	}
	if err := json.Unmarshal(blob, &got); err != nil {
		t.Fatalf(".key no es JSON válido: %v", err)
	}
	if got.SchoolKey != "kp_abc123" {
		t.Fatalf("schoolKey del .key = %q", got.SchoolKey)
	}
	if got.WorkerURL != "https://worker-alpha.example.workers.dev" {
		t.Fatalf("workerUrl del .key = %q", got.WorkerURL)
	}
}

func TestKitD1RoutingKVIDReadsGluedBlock(t *testing.T) {
	dir := t.TempDir()
	toml := "name = \"w\"\n" +
		"[[kv_namespaces]]\nbinding = \"OTHER\"\nid = \"aaaaaaaa\"\n" +
		"[[kv_namespaces]]\nbinding = \"SCHOOL_ROUTING\"\nid = \"bbbbbbbb\"\n" +
		"[[d1_databases]]\nbinding = \"DB\"\ndatabase_name = \"x\"\n"
	if err := os.WriteFile(filepath.Join(dir, "wrangler.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := kitD1RoutingKVID(dir, "SCHOOL_ROUTING"); got != "bbbbbbbb" {
		t.Fatalf("id del KV de ruteo = %q, quería bbbbbbbb", got)
	}
	if got := kitD1RoutingKVID(dir, "MISSING"); got != "" {
		t.Fatalf("binding inexistente devolvió %q", got)
	}
}

// Sin KV en wrangler.toml no se puede registrar la clave: fallar antes de
// ejecutar Wrangler evita crear una clave que el worker no sabría enrutar.
func TestKitD1MintKeyRequiresRoutingKV(t *testing.T) {
	dir := t.TempDir()
	cfg := loadKitD1ProjectConfig(dir)
	if _, err := kitD1MintKey(dir, cfg, "11111111-2222-3333-4444-555555555555"); err == nil {
		t.Fatal("se esperaba error sin [[kv_namespaces]] en wrangler.toml")
	}
}

func TestKitD1TenantSlugUsesFirstTenant(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tenants.json"), []byte(`{"tenants":[{"slug":"nube"},{"slug":"otro"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := loadKitD1ProjectConfig(dir)
	if got := kitD1TenantSlug(dir, cfg); got != "nube" {
		t.Fatalf("slug = %q", got)
	}
	if got := kitD1TenantSlug(t.TempDir(), cfg); got != "default" {
		t.Fatalf("sin tenants.json el slug debería ser default, fue %q", got)
	}
}
