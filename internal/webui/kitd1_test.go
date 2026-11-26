package webui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	_ = os.WriteFile(filepath.Join(consumer, "go.mod"), []byte("require github.com/Keimer92/kit-d1/go/syncengine v0.1.0\n"), 0o644)
	_ = os.WriteFile(filepath.Join(consumer, "go.work"), []byte("replace github.com/Keimer92/kit-d1/go/syncengine => \"D:/kit\"\n"), 0o644)
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
	// Instancia inexistente o con traversal: se rechaza.
	if s := b.KitD1Script("dev", "no-existe", ""); !strings.Contains(s, "no válida") {
		t.Fatalf("instancia inexistente aceptada: %s", s)
	}
	if s := b.KitD1Script("dev", "../alpha", ""); !strings.Contains(s, "no válida") {
		t.Fatalf("traversal aceptado: %s", s)
	}
	// Accion desconocida: se rechaza.
	if s := b.KitD1Script("borrar-todo", "alpha", ""); !strings.Contains(s, "no válida") {
		t.Fatalf("accion desconocida aceptada: %s", s)
	}
	// Tag exige semver.
	if s := b.KitD1Script("tag", "", "drop-table"); !strings.Contains(s, "no válida") {
		t.Fatalf("version no semver aceptada: %s", s)
	}
	if s := b.KitD1Script("tag", "", "1.2.3"); !strings.Contains(s, "v1.2.3") {
		t.Fatalf("tag sin v: %s", s)
	}
}

func TestKitD1AccionesCatalogo(t *testing.T) {
	ids := map[string]bool{}
	for _, a := range kitD1Actions {
		if ids[a.ID] {
			t.Fatalf("accion duplicada: %s", a.ID)
		}
		ids[a.ID] = true
		if a.ID != "tag" && !a.NeedsInstance {
			t.Fatalf("accion %s deberia needing instance", a.ID)
		}
	}
	for _, want := range []string{"assemble", "typecheck", "dev", "deploy", "provision-dry", "provision", "whoami", "login", "admin-list", "key-export", "key-rotate", "tag"} {
		if !ids[want] {
			t.Fatalf("falta accion %s", want)
		}
	}
}
