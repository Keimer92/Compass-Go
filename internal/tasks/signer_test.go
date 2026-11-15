package tasks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"compass/internal/theme"
)

func TestPropsRoundtrip(t *testing.T) {
	p := Props{
		StoreFile:     "release.keystore",
		StorePassword: "s3cret",
		KeyAlias:      "upload",
		KeyPassword:   "keypass",
	}
	if got := parseProps(p.Render()); got != p {
		t.Fatalf("roundtrip = %+v, quiero %+v", got, p)
	}
}

func TestPropsAcceptFlutterAndTauriKeyAliases(t *testing.T) {
	flutter := parseProps("storePassword=a\nkeyPassword=b\nkeyAlias=c\nstoreFile=d.jks")
	if flutter.StorePassword != "a" || flutter.KeyPassword != "b" ||
		flutter.KeyAlias != "c" || flutter.StoreFile != "d.jks" {
		t.Fatalf("aliases Flutter mal parseados: %+v", flutter)
	}

	tauri := parseProps("storepass=a\nkeypass=b\nalias=c\nstoreFile=d")
	if tauri.StorePassword != "a" || tauri.KeyPassword != "b" || tauri.KeyAlias != "c" {
		t.Fatalf("aliases Tauri mal parseados: %+v", tauri)
	}
}

func TestPropsStripQuotes(t *testing.T) {
	p := parseProps("storeFile=\"a b.keystore\"\nkeyAlias = \"x\"")
	if p.StoreFile != "a b.keystore" {
		t.Fatalf("storeFile = %q", p.StoreFile)
	}
	if p.KeyAlias != "x" {
		t.Fatalf("keyAlias = %q", p.KeyAlias)
	}
}

func TestResolveKeystoreHandlesAbsoluteAndRelative(t *testing.T) {
	dir := filepath.Join("tmp", "mod")
	if got, want := resolveKeystore(dir, "release.keystore"), filepath.Join(dir, "release.keystore"); got != want {
		t.Fatalf("relativo = %q, quiero %q", got, want)
	}
	abs := filepath.Join(t.TempDir(), "other.jks")
	if got := resolveKeystore(dir, abs); got != abs {
		t.Fatalf("absoluto = %q, quiero %q", got, abs)
	}
}

func TestBase64MatchesKnownVectors(t *testing.T) {
	cases := map[string]string{
		"":       "",
		"f":      "Zg==",
		"fo":     "Zm8=",
		"foo":    "Zm9v",
		"foob":   "Zm9vYg==",
		"fooba":  "Zm9vYmE=",
		"foobar": "Zm9vYmFy",
	}
	for in, want := range cases {
		if got := base64Encode([]byte(in)); got != want {
			t.Errorf("base64(%q) = %q, quiero %q", in, got, want)
		}
	}
}

func sampleSigningInfo() SigningInfo {
	return SigningInfo{
		Layout: AndroidLayout{
			ModuleDir:   filepath.Join("tmp", "app"),
			PropsName:   "keystore.properties",
			SigningDir:  filepath.Join("tmp", "app"),
			PatchGradle: true,
		},
		Props: &Props{
			StoreFile:     "release.keystore",
			StorePassword: "storepw",
			KeyAlias:      "upload",
			KeyPassword:   "keypw",
		},
		Keystore: filepath.Join("tmp", "app", "release.keystore"),
	}
}

// El caso que rompe la heurística ingenua: KEYSTORE_PASSWORD contiene KEYSTORE
// pero debe resolver a la contraseña, no al base64 del archivo.
func TestSecretGuessingPrefersPasswordOverKeystore(t *testing.T) {
	info := sampleSigningInfo()
	const b64 = "BASE64DATA"

	cases := []struct{ name, value, source string }{
		{"ANDROID_KEYSTORE_BASE64", b64, "keystore (base64)"},
		{"ANDROID_KEYSTORE_PASSWORD", "storepw", "storePassword"},
		{"ANDROID_KEY_ALIAS", "upload", "keyAlias"},
		{"ANDROID_KEY_PASSWORD", "keypw", "keyPassword"},
		{"STORE_FILE", b64, "keystore (base64)"},
		{"OTRA_COSA", "", "sin datos"},
	}
	for _, c := range cases {
		g := guessSecret(c.name, info, b64, true)
		if g.Value != c.value || g.Source != c.source {
			t.Errorf("%s = (%q,%q), quiero (%q,%q)", c.name, g.Value, g.Source, c.value, c.source)
		}
	}

	if g := guessSecret("ANDROID_KEYSTORE_BASE64", info, "", false); g.Source != "keystore no encontrado" {
		t.Fatalf("sin keystore, source = %q", g.Source)
	}
}

func TestWorkflowSecretNamesScansYamlRecursively(t *testing.T) {
	root := t.TempDir()
	wf := filepath.Join(root, ".github", "workflows", "nested")
	if err := os.MkdirAll(wf, 0755); err != nil {
		t.Fatal(err)
	}
	content := "env:\n  K: ${{ secrets.ANDROID_KEYSTORE_BASE64 }}\n  P: ${{ secrets.MY_PASS }}\n"
	if err := os.WriteFile(filepath.Join(wf, "release.yml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wf, "notes.txt"), []byte("secrets.IGNORED\n"), 0644); err != nil {
		t.Fatal(err)
	}

	names := workflowSecretNames(root)
	if len(names) != 2 || names[0] != "ANDROID_KEYSTORE_BASE64" || names[1] != "MY_PASS" {
		t.Fatalf("nombres = %v", names)
	}
}

func TestGradleSigningStatusDetectsThePatch(t *testing.T) {
	dir := t.TempDir()
	if got := gradleSigningStatus(dir); got != gradleNoFile {
		t.Fatalf("sin archivo = %v", got)
	}

	kts := filepath.Join(dir, "build.gradle.kts")
	if err := os.WriteFile(kts, []byte("android {\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := gradleSigningStatus(dir); got != gradleUnpatched {
		t.Fatalf("sin parche = %v", got)
	}

	patched := "val keystorePropertiesFile = rootProject.file(\"app/keystore.properties\")\nandroid {\n signingConfigs {\n }\n}\n"
	if err := os.WriteFile(kts, []byte(patched), 0644); err != nil {
		t.Fatal(err)
	}
	if got := gradleSigningStatus(dir); got != gradlePatched {
		t.Fatalf("parcheado = %v", got)
	}
}

func TestPatchBuildGradleInjectsTheLoader(t *testing.T) {
	dir := t.TempDir()
	layout := AndroidLayout{
		ModuleDir:   dir,
		PropsName:   "keystore.properties",
		SigningDir:  dir,
		PatchGradle: true,
	}
	original := "plugins {\n}\n\nandroid {\n    defaultConfig {\n    }\n    buildTypes {\n        getByName(\"release\") {\n        }\n    }\n}\n"
	path := filepath.Join(dir, "build.gradle.kts")
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	if err := patchBuildGradle(layout); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{
		"import java.util.Properties",
		"val keystorePropertiesFile",
		"signingConfigs {",
		"signingConfig = signingConfigs.getByName(\"release\")",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("falta %q en el gradle parcheado:\n%s", want, got)
		}
	}

	// Una segunda pasada no debe duplicar nada.
	before := strings.Count(got, "signingConfigs {")
	if err := patchBuildGradle(layout); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(path)
	if after := strings.Count(string(again), "signingConfigs {"); after != before {
		t.Fatalf("signingConfigs duplicado: %d -> %d", before, after)
	}
}

func TestPatchBuildGradleIsANoopForWails(t *testing.T) {
	dir := t.TempDir()
	layout := AndroidLayout{ModuleDir: dir, PropsName: "keystore.properties", SigningDir: dir, PatchGradle: false}
	if err := patchBuildGradle(layout); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("Wails no debe escribir en el módulo: %v", entries)
	}
}

func TestApksignerCertParsing(t *testing.T) {
	text := "Verifies\nSigner #1 certificate DN: CN=x\n" +
		"Signer #1 certificate SHA-256 digest: AABB:CCDD\n" +
		"Signer #1 certificate SHA-1 digest: 11:22\n"
	certs := parseApksignerCerts(text)
	if len(certs) != 2 {
		t.Fatalf("certs = %v", certs)
	}
	if certs[0] != [2]string{"SHA-256", "AABB:CCDD"} || certs[1] != [2]string{"SHA-1", "11:22"} {
		t.Fatalf("certs = %v", certs)
	}
}

func TestFingerprintParsingAndNormalization(t *testing.T) {
	// Formato real de `keytool -list -v`.
	text := "Alias name: upload\n" +
		"Certificate fingerprints:\n" +
		"\t SHA1: AB:CD:EF\n" +
		"\t SHA256: 01:23:45\n" +
		"\t MD5: 99:88\n"
	fp := parseFingerprints(text)
	if fp.SHA1 != "AB:CD:EF" || fp.SHA256 != "01:23:45" || fp.MD5 != "99:88" {
		t.Fatalf("huellas = %+v", fp)
	}
	if normalizeFingerprint("AB:CD:ef") != normalizeFingerprint("abcdef") {
		t.Fatal("la normalización debe ignorar ':' y mayúsculas")
	}
	if !(Fingerprints{}).IsEmpty() {
		t.Fatal("un Fingerprints vacío debe ser IsEmpty")
	}
	if (Fingerprints{SHA1: "x"}).IsEmpty() {
		t.Fatal("un Fingerprints con SHA1 no debe ser IsEmpty")
	}
}

func TestSigningEntriesCoverEveryItem(t *testing.T) {
	entries := signingEntries()
	if len(entries) != 10 {
		t.Fatalf("entradas = %d, quiero 10", len(entries))
	}
	seen := map[string]bool{}
	for _, e := range entries {
		if seen[e.key] {
			t.Errorf("clave duplicada %q", e.key)
		}
		seen[e.key] = true
		if e.run == nil {
			t.Errorf("%q no tiene acción", e.key)
		}
	}
}

func TestSigningCardsUseDistinctPaletteKeys(t *testing.T) {
	known := map[string]bool{}
	for _, k := range theme.CardKeys {
		known[k] = true
	}
	seen := map[string]bool{}
	for _, e := range signingEntries() {
		key := e.card.ColorKey
		if seen[key] {
			t.Errorf("dos cards comparten acento %q", key)
		}
		seen[key] = true
		if !known[key] {
			t.Errorf("%q no es una clave de CardKeys", key)
		}
	}
}
