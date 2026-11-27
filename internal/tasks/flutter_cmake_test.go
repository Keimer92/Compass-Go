package tasks

import (
	"os"
	"path/filepath"
	"testing"

	"compass/internal/project"
)

// writeCMakeCache deja un CMakeCache.txt en <root>/<rel>/CMakeCache.txt con las
// rutas indicadas (home y cachefile).
func writeCMakeCache(t *testing.T, root, rel, home, cacheFileDir string) string {
	t.Helper()
	dir := filepath.Join(root, rel)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(dir, "CMakeCache.txt")
	body := "# This is the CMakeCache file.\n"
	if home != "" {
		body += "CMAKE_HOME_DIRECTORY:INTERNAL=" + home + "\n"
	}
	if cacheFileDir != "" {
		body += "CMAKE_CACHEFILE_DIR:INTERNAL=" + cacheFileDir + "\n"
	}
	if err := os.WriteFile(cache, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestEnsureFlutterCMakeCacheBorraRutaVieja: si el proyecto se movio, la cache
// de Windows apunta a la ruta anterior y CMake aborta. Debe borrarse sola.
func TestEnsureFlutterCMakeCacheBorraRutaVieja(t *testing.T) {
	root := t.TempDir()
	writeCMakeCache(t, root, filepath.Join("build", "windows", "x64"),
		`D:/Mis Proyectos/Flutter/ELA Group/windows`,
		`d:/Mis Proyectos/Flutter/ELA Group/build/windows/x64`)

	ctx := &project.ProjectContext{ScriptDir: root, ProjectType: project.TypeFlutter}
	ensureFlutterCMakeCache(ctx)

	if dirExists(filepath.Join(root, "build", "windows")) {
		t.Error("la cache de otra ruta debe eliminarse")
	}
}

// TestEnsureFlutterCMakeCacheBorraCxxAndroid: la cache nativa de Android
// (.cxx) tambien guarda rutas absolutas y debe purgarse tras un movimiento.
func TestEnsureFlutterCMakeCacheBorraCxxAndroid(t *testing.T) {
	root := t.TempDir()
	writeCMakeCache(t, root, filepath.Join("build", ".cxx", "debug", "abc123", "arm64-v8a"),
		"E:/SDKs/Flutter/packages/flutter_tools/gradle/src/main/scripts",
		`d:/Mis Proyectos/Flutter/School Tracker Pro ELA/build/.cxx/debug/abc123/arm64-v8a`)

	ctx := &project.ProjectContext{ScriptDir: root, ProjectType: project.TypeFlutter}
	ensureFlutterCMakeCache(ctx)

	if dirExists(filepath.Join(root, "build", ".cxx")) {
		t.Error("la cache .cxx de otra ruta debe eliminarse")
	}
}

// TestEnsureFlutterCMakeCacheConservaRutaActual: con la ruta correcta no se
// toca nada (no se invalida el build incremental).
func TestEnsureFlutterCMakeCacheConservaRutaActual(t *testing.T) {
	root := t.TempDir()
	rel := filepath.Join("build", "windows", "x64")
	dir := filepath.Join(root, rel)
	writeCMakeCache(t, root, rel,
		filepath.Join(root, "windows"),
		dir)

	ctx := &project.ProjectContext{ScriptDir: root, ProjectType: project.TypeFlutter}
	ensureFlutterCMakeCache(ctx)

	if !fileExists(filepath.Join(dir, "CMakeCache.txt")) {
		t.Error("no debe borrarse una cache que corresponde a la ruta actual")
	}
}

// TestEnsureFlutterCMakeCacheSinCache no falla cuando aun no hay build.
func TestEnsureFlutterCMakeCacheSinCache(t *testing.T) {
	root := t.TempDir()
	ctx := &project.ProjectContext{ScriptDir: root, ProjectType: project.TypeFlutter}
	ensureFlutterCMakeCache(ctx)
}

func TestSamePathIgnoraMayusculasYSeparadores(t *testing.T) {
	a := `D:\Mis Proyectos\Go + Wails v3\ELA Group\windows`
	b := `d:/Mis Proyectos/Go + Wails v3/ELA Group/windows`
	if !samePath(a, b) {
		t.Errorf("samePath(%q, %q) = false", a, b)
	}
	if samePath(a, `D:/Mis Proyectos/Flutter/ELA Group/windows`) {
		t.Error("rutas distintas no deben considerarse iguales")
	}
}
