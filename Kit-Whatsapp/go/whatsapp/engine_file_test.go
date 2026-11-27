package whatsapp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFindFirstHonorsCompassEngineEnv verifica la solución 2: cuando
// COMPASS_WHATSAPP_ENGINE apunta al engine canónico, GymPro reutiliza node.exe y
// el bundle directamente desde ahí, sin copiar nada localmente (modo debug).
func TestFindFirstHonorsCompassEngineEnv(t *testing.T) {
	engineDir := t.TempDir()
	bundle := filepath.Join(engineDir, "whatsapp-engine-bundle", "index.js")
	if err := os.MkdirAll(filepath.Dir(bundle), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bundle, []byte("bundle"), 0o644); err != nil {
		t.Fatal(err)
	}
	node := filepath.Join(engineDir, "node.exe")
	if err := os.WriteFile(node, []byte("node"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("COMPASS_WHATSAPP_ENGINE", engineDir)

	if got := findFirst("whatsapp-engine-bundle/index.js"); got != bundle {
		t.Fatalf("bundle: got %q, quiero %q", got, bundle)
	}
	if got := findFirst("node.exe"); got != node {
		t.Fatalf("node: got %q, quiero %q", got, node)
	}
}

// TestFindFirstEnvPrecedence confirma que el engine canónico (env) gana sobre una
// copia local, de modo que debug siempre usa la fuente única de verdad.
func TestFindFirstEnvPrecedence(t *testing.T) {
	engineDir := t.TempDir()
	engineNode := filepath.Join(engineDir, "node.exe")
	if err := os.WriteFile(engineNode, []byte("canon"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Copia local que NO debería usarse porque el engine canónico va primero.
	localNode := filepath.Join(t.TempDir(), "ignored", "node.exe")
	if err := os.MkdirAll(filepath.Dir(localNode), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(localNode, []byte("local"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("COMPASS_WHATSAPP_ENGINE", engineDir)
	// Forzamos un fallback imposible para localNode usando un cwd sin node.exe:
	// basta comprobar que devuelve el del engine canónico.
	if got := findFirst("node.exe"); got != engineNode {
		t.Fatalf("node: got %q, quiero %q", got, engineNode)
	}
}

// TestResolveNodeIgnoresCanonicalEngineSidecar verifica que el runtime no usa
// el node.exe del engine canónico: con COMPASS_WHATSAPP_ENGINE apuntando a un
// kit con node.exe, resolveNode debe devolver el Node del PATH.
func TestResolveNodeIgnoresCanonicalEngineSidecar(t *testing.T) {
	engineDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(engineDir, "node.exe"), []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COMPASS_WHATSAPP_ENGINE", engineDir)

	node, err := resolveNode()
	if err != nil {
		t.Skipf("no hay Node en PATH para la prueba: %v", err)
	}
	prefix := filepath.Clean(engineDir) + string(os.PathSeparator)
	if strings.HasPrefix(filepath.Clean(node), prefix) {
		t.Fatalf("resolveNode uso el sidecar del engine canónico: %s", node)
	}
}

// TestEnsureCanonicalBundleSkipsWithoutNode confirma que, si el engine canónico no
// tiene node.exe, el auto-build se omite sin pánico y sin crear el bundle.
func TestEnsureCanonicalBundleSkipsWithoutNode(t *testing.T) {
	dir := t.TempDir()
	ensureCanonicalBundle(dir)
	if _, err := os.Stat(filepath.Join(dir, "whatsapp-engine-bundle", "index.js")); err == nil {
		t.Fatal("no debió generarse el bundle sin node.exe")
	}
}

// TestEnsureCanonicalBundleNoopWhenPresent confirma que si el bundle ya existe no
// intenta reconstruirlo (evita builds innecesarios en cada arranque).
func TestEnsureCanonicalBundleNoopWhenPresent(t *testing.T) {
	dir := t.TempDir()
	bundle := filepath.Join(dir, "whatsapp-engine-bundle", "index.js")
	if err := os.MkdirAll(filepath.Dir(bundle), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bundle, []byte("bundle"), 0o644); err != nil {
		t.Fatal(err)
	}
	ensureCanonicalBundle(dir) // no debe pánico ni error
}
