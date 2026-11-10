package tasks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"compass/internal/project"
	"compass/internal/theme"
)

func releaseCtx(t *testing.T, kind project.ProjectType) *project.ProjectContext {
	t.Helper()
	return &project.ProjectContext{
		ScriptDir:   filepath.Join("C:", "tmp", "proyecto"),
		ProjectType: kind,
		AndroidHome: filepath.Join("C:", "Android"),
	}
}

func releaseLabels(entries []releaseEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.card.Label
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestWindowsEntriesAreOfferedOnlyByDesktopArchTypes(t *testing.T) {
	want := []string{
		"Windows x86",
		"Windows x64",
		"Windows ARM64",
		"Todos",
		"Volver al menu principal",
	}
	for _, kind := range []project.ProjectType{
		project.TypeTauri, project.TypeRust, project.TypeGo, project.TypeGoWails,
	} {
		entries := windowsEntries(releaseCtx(t, kind))
		if got := releaseLabels(entries); !equalStrings(got, want) {
			t.Errorf("%s: %v, quiero %v", kind.Label(), got, want)
		}
		if entries[3].key != archKeyAll {
			t.Errorf("%s: la card Todos debe ser 'all'", kind.Label())
		}
		if entries[len(entries)-1].key != archKeyBack {
			t.Errorf("%s: la última card debe ser back", kind.Label())
		}
	}
	for _, kind := range []project.ProjectType{
		project.TypeFlutter, project.TypeKotlin, project.TypePython,
	} {
		if entries := windowsEntries(releaseCtx(t, kind)); entries != nil {
			t.Errorf("%s no debería ofrecer release de escritorio por arch", kind.Label())
		}
	}
}

func TestAndroidEntriesDependOnTheToolchain(t *testing.T) {
	full := []string{
		"Android arm64",
		"Android armv7",
		"Android universal",
		"Todos",
		"Volver al menu principal",
	}
	for _, kind := range []project.ProjectType{
		project.TypeTauri, project.TypeFlutter, project.TypeGoWails,
	} {
		if got := releaseLabels(androidEntries(releaseCtx(t, kind))); !equalStrings(got, full) {
			t.Errorf("%s: %v, quiero %v", kind.Label(), got, full)
		}
	}

	kotlin := androidEntries(releaseCtx(t, project.TypeKotlin))
	wantKotlin := []string{"Android universal", "Volver al menu principal"}
	if got := releaseLabels(kotlin); !equalStrings(got, wantKotlin) {
		t.Errorf("Kotlin sin jniLibs: %v, quiero %v", got, wantKotlin)
	}
	if kotlin[0].card.Description != "Sin libs nativas · sirve para todas las ABIs" {
		t.Errorf("descripción = %q", kotlin[0].card.Description)
	}

	for _, kind := range []project.ProjectType{
		project.TypeRust, project.TypeGo, project.TypePython,
	} {
		if entries := androidEntries(releaseCtx(t, kind)); entries != nil {
			t.Errorf("%s no debería ofrecer release de Android", kind.Label())
		}
	}
}

func TestAndroidEntriesIncludeAbisWhenKotlinHasJniLibs(t *testing.T) {
	root := t.TempDir()
	jni := filepath.Join(root, "composeApp", "src", "main", "jniLibs", "arm64-v8a")
	if err := os.MkdirAll(jni, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jni, "libdemo.so"), []byte("elf"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &project.ProjectContext{ScriptDir: root, ProjectType: project.TypeKotlin}
	entries := androidEntries(ctx)
	if len(entries) != 5 {
		t.Fatalf("entradas = %d, quiero 5", len(entries))
	}
	if entries[3].key != archKeyAll {
		t.Fatalf("la card Todos debe ser 'all', es %q", entries[3].key)
	}
	if !hasNativeLibs(kotlinAndroidDir(ctx)) {
		t.Fatal("hasNativeLibs debe detectar el .so")
	}
}

func TestReleaseCardColorKeysAreValidAndDistinct(t *testing.T) {
	known := map[string]bool{}
	for _, k := range theme.CardKeys {
		known[k] = true
	}

	ctx := releaseCtx(t, project.TypeTauri)
	for name, entries := range map[string][]releaseEntry{
		"windows": windowsEntries(ctx),
		"android": androidEntries(ctx),
	} {
		seen := map[string]bool{}
		for _, e := range entries {
			key := e.card.ColorKey
			if seen[key] {
				t.Errorf("%s: clave de paleta repetida %q", name, key)
			}
			seen[key] = true
			if !known[key] {
				t.Errorf("%s: clave de paleta desconocida %q", name, key)
			}
		}
	}
}

func TestWindowsTriplesAndGoarchsMatchRealTargets(t *testing.T) {
	cases := map[string]string{
		archKeyX86:   "i686-pc-windows-msvc",
		archKeyX64:   "x86_64-pc-windows-msvc",
		archKeyArm64: "aarch64-pc-windows-msvc",
	}
	for key, want := range cases {
		if got, ok := windowsTriple(key); !ok || got != want {
			t.Errorf("triple(%s) = %q,%v; quiero %q", key, got, ok, want)
		}
	}
	if _, ok := windowsTriple(archKeyAll); ok {
		t.Error("'all' no tiene triple")
	}

	if windowsLabel(archKeyX64) != "" || windowsLabel(archKeyX86) != "x86" || windowsLabel(archKeyArm64) != "arm64" {
		t.Error("etiquetas de artefacto mal")
	}

	if a, ok := goArchOf(archKeyX86); !ok || a.goarch() != "386" {
		t.Errorf("GOARCH x86 = %q", a.goarch())
	}
	if a, ok := goArchOf(archKeyX64); !ok || a.goarch() != "amd64" {
		t.Errorf("GOARCH x64 = %q", a.goarch())
	}
	if a, ok := goArchOf(archKeyArm64); !ok || a.goarch() != "arm64" {
		t.Errorf("GOARCH arm64 = %q", a.goarch())
	}
	if _, ok := goArchOf(archKeyAll); ok {
		t.Error("'all' no tiene GOARCH")
	}
}

func TestReleaseBackIsTheLastEntryInBothSubmenus(t *testing.T) {
	ctx := releaseCtx(t, project.TypeTauri)
	for name, entries := range map[string][]releaseEntry{
		"windows": windowsEntries(ctx),
		"android": androidEntries(ctx),
	} {
		last := entries[len(entries)-1]
		if last.key != archKeyBack {
			t.Errorf("%s: última entrada = %q", name, last.key)
		}
	}
}

func TestArtifactBaseOmitsEmptyParts(t *testing.T) {
	cases := []struct {
		name, version, label, want string
	}{
		{"App", "1.2.3", "", "App v1.2.3"},
		{"App", "1.2.3", "arm64", "App v1.2.3 arm64"},
		{"App", "", "", "App"},
		{"App", "", "x86", "App x86"},
	}
	for _, c := range cases {
		if got := artifactBase(c.name, c.version, c.label); got != c.want {
			t.Errorf("artifactBase(%q,%q,%q) = %q, quiero %q", c.name, c.version, c.label, got, c.want)
		}
	}
}

func TestWriteAbiInitScriptPinsOneAbiOrEnablesSplits(t *testing.T) {
	single, err := writeAbiInitScript("arm64-v8a")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(single)
	data, _ := os.ReadFile(single)
	if !containsAll(string(data), "abiFilters.add('arm64-v8a')", "splits.abi.enable = false") {
		t.Errorf("init script de una ABI inesperado:\n%s", data)
	}

	splits, err := writeAbiInitScript("")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(splits)
	data, _ = os.ReadFile(splits)
	if !containsAll(string(data), "splits.abi.enable = true", "universalApk = true", "'arm64-v8a', 'armeabi-v7a'") {
		t.Errorf("init script de splits inesperado:\n%s", data)
	}
}

func containsAll(haystack string, needles ...string) bool {
	for _, n := range needles {
		if !strings.Contains(haystack, n) {
			return false
		}
	}
	return true
}
