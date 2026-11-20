package tasks

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"compass/internal/project"
	"compass/internal/prompts"
	"compass/internal/shell"
	"compass/internal/menu"
	"compass/internal/ui"
)

func init() {
	// El despachador manda "sign"/"signing"/"sha1"/"keystore" a SigningManage
	// para *cualquier* tipo de proyecto; los tipos sin Android reciben el aviso
	// "No aplicable".
	for _, pt := range allTypes() {
		Register("sign", pt, SigningManage)
		Register("signing", pt, SigningManage)
		Register("sha1", pt, SigningManage)
		Register("keystore", pt, SigningManage)

		// Acciones individuales para las subcards nativas de la GUI (mismo
		// estilo que las de Windows release): cada una ejecuta su accion una
		// sola vez en la pestana, sin dibujar el submenu de la TUI.
		for _, e := range signingEntries() {
			Register("signing-do-"+e.key, pt, e.run)
		}
	}
}

// SigningOption es una accion del submenu de firmas expuesta a la GUI.
type SigningOption struct {
	Key         string
	Label       string
	Description string
	ColorKey    string
}

// SigningOptions devuelve las acciones del submenu en el mismo orden que la
// TUI, para que la GUI pinte las mismas cards.
func SigningOptions() []SigningOption {
	entries := signingEntries()
	out := make([]SigningOption, 0, len(entries))
	for _, e := range entries {
		out = append(out, SigningOption{
			Key:         e.key,
			Label:       e.card.Label,
			Description: e.card.Description,
			ColorKey:    e.card.ColorKey,
		})
	}
	return out
}

// Props representa los campos de keystore.properties / key.properties.
//
// Es el equivalente de src/tasks/signer.rs::Props: la fuente de verdad del
// parseo de la firma release, reutilizada por los builds de Kotlin/Flutter.
type Props struct {
	StoreFile     string
	StorePassword string
	KeyAlias      string
	KeyPassword   string
}

// parseProps tolera los alias de clave de Flutter (storePassword/keyPassword/
// keyAlias/storeFile) y los de KMP (storepass/keypass/alias).
func parseProps(text string) Props {
	var p Props
	for _, line := range strings.Split(text, "\n") {
		eq := strings.Index(line, "=")
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		val = strings.Trim(val, "\"'")
		switch key {
		case "storeFile":
			p.StoreFile = val
		case "storePassword", "storepass":
			p.StorePassword = val
		case "keyAlias", "alias":
			p.KeyAlias = val
		case "keyPassword", "keypass":
			p.KeyPassword = val
		}
	}
	return p
}

// Render devuelve el contenido del .properties en el orden canónico.
func (p Props) Render() string {
	return fmt.Sprintf(
		"storePassword=%s\nkeyPassword=%s\nkeyAlias=%s\nstoreFile=%s\n",
		p.StorePassword, p.KeyPassword, p.KeyAlias, p.StoreFile,
	)
}

// readProps lee y parsea el .properties; devuelve ok=false si no existe.
func readProps(path string) (Props, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Props{}, false
	}
	return parseProps(string(data)), true
}

// resolveKeystore resuelve el storeFile a una ruta real (absoluta o relativa a
// signingDir). Es fiel a src/tasks/signer.rs::resolve_keystore.
func resolveKeystore(signingDir, storeFile string) string {
	p := filepath.FromSlash(storeFile)
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(signingDir, p)
}

// ── Layout y autodetección ──────────────────────────────────────────────────

// AndroidLayout indica dónde vive la firma de un proyecto Android.
type AndroidLayout struct {
	// ModuleDir es el directorio del módulo Android (android/app,
	// composeApp, app o build/android/app).
	ModuleDir string
	// PropsName es key.properties (Flutter) o keystore.properties.
	PropsName string
	// SigningDir es donde viven el keystore y el .properties. En Wails v3 es
	// android-signing/, fuera de build/ (que se regenera).
	SigningDir string
	// PatchGradle indica si hay que parchear build.gradle. En Wails v3 no:
	// su Gradle ya lee ANDROID_KEYSTORE_* del entorno.
	PatchGradle bool
}

// PropsPath devuelve la ruta del archivo de propiedades.
func (l AndroidLayout) PropsPath() string {
	return filepath.Join(l.SigningDir, l.PropsName)
}

// androidLayout devuelve el layout del proyecto; ok=false para Python/Go.
func androidLayout(ctx *project.ProjectContext) (AndroidLayout, bool) {
	switch ctx.ProjectType {
	case project.TypeFlutter:
		dir := filepath.Join(ctx.ScriptDir, "android", "app")
		return AndroidLayout{ModuleDir: dir, PropsName: "key.properties", SigningDir: dir, PatchGradle: true}, true
	case project.TypeKotlin:
		dir := kotlinAndroidDir(ctx)
		return AndroidLayout{ModuleDir: dir, PropsName: "keystore.properties", SigningDir: dir, PatchGradle: true}, true
	case project.TypeGoWails:
		return AndroidLayout{
			ModuleDir:   filepath.Join(ctx.ScriptDir, "build", "android", "app"),
			PropsName:   "keystore.properties",
			SigningDir:  filepath.Join(ctx.ScriptDir, "android-signing"),
			PatchGradle: false,
		}, true
	default:
		return AndroidLayout{}, false
	}
}

// SigningInfo es el estado de la firma release ya resuelto.
type SigningInfo struct {
	Layout AndroidLayout
	// Props es nil si el .properties no existe.
	Props *Props
	// Keystore es la ruta absoluta, sólo si el archivo existe.
	Keystore string
	// KeystoreMissing es el storeFile declarado pero inexistente en disco.
	KeystoreMissing string
}

// Alias devuelve el alias de la llave ("" si no hay properties).
func (i SigningInfo) Alias() string {
	if i.Props == nil {
		return ""
	}
	return i.Props.KeyAlias
}

// StorePassword devuelve la contraseña del almacén.
func (i SigningInfo) StorePassword() string {
	if i.Props == nil {
		return ""
	}
	return i.Props.StorePassword
}

// KeyPassword devuelve la contraseña de la llave; si falta, la del almacén.
func (i SigningInfo) KeyPassword() string {
	if i.Props == nil {
		return ""
	}
	if i.Props.KeyPassword != "" {
		return i.Props.KeyPassword
	}
	return i.Props.StorePassword
}

// detectSigning reúne el estado de la firma a partir de lo que hay en disco.
func detectSigning(ctx *project.ProjectContext) (SigningInfo, bool) {
	layout, ok := androidLayout(ctx)
	if !ok {
		return SigningInfo{}, false
	}
	info := SigningInfo{Layout: layout}
	if p, ok := readProps(layout.PropsPath()); ok {
		info.Props = &p
	}
	if info.Props != nil && info.Props.StoreFile != "" {
		path := resolveKeystore(layout.SigningDir, info.Props.StoreFile)
		if fileExists(path) {
			info.Keystore = path
		} else {
			info.KeystoreMissing = info.Props.StoreFile
		}
	}
	return info, true
}

// gradleSigning es el grado de parcheo del build.gradle(.kts) del módulo.
type gradleSigning int

const (
	gradleNoFile gradleSigning = iota
	gradleUnpatched
	gradlePatched
)

// gradleSigningStatus comprueba si el Gradle del módulo ya tiene el bloque de
// firma inyectado.
func gradleSigningStatus(moduleDir string) gradleSigning {
	kts := filepath.Join(moduleDir, "build.gradle.kts")
	groovy := filepath.Join(moduleDir, "build.gradle")
	path := kts
	if !fileExists(kts) {
		if !fileExists(groovy) {
			return gradleNoFile
		}
		path = groovy
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return gradleNoFile
	}
	content := string(data)
	hasLoader := strings.Contains(content, "keystorePropertiesFile")
	hasConfigs := strings.Contains(content, "signingConfigs {") || strings.Contains(content, "signingConfigs{")
	if hasLoader && hasConfigs {
		return gradlePatched
	}
	return gradleUnpatched
}

// ── keytool y huellas ───────────────────────────────────────────────────────

// findKeytool localiza el keytool.exe de un JDK o de Android Studio.
func findKeytool() (string, error) {
	const exe = "keytool.exe"
	paths := []string{"keytool"}

	if jh := os.Getenv("JAVA_HOME"); jh != "" {
		paths = append(paths, filepath.Join(jh, "bin", exe))
	}
	paths = append(paths,
		`C:\Program Files\Android\Android Studio\jbr\bin\`+exe,
		`C:\Program Files\Android\Android Studio\jre\bin\`+exe,
	)
	for _, base := range []string{`C:\Program Files\Java`, `C:\Program Files (x86)\Java`} {
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		for _, e := range entries {
			k := filepath.Join(base, e.Name(), "bin", exe)
			if fileExists(k) {
				paths = append(paths, k)
			}
		}
	}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		for _, jbr := range []string{"jbr", "jre"} {
			c := filepath.Join(local, "Programs", "Android Studio", jbr, "bin", exe)
			if fileExists(c) {
				paths = append(paths, c)
			}
		}
	}

	for _, p := range paths {
		if p == "" {
			continue
		}
		err := shell.HideConsole(exec.Command(p, "-help")).Run()
		if err == nil {
			return p, nil
		}
		// Existe pero devolvió un código distinto de cero: también sirve.
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return p, nil
		}
	}

	return "", fmt.Errorf("no se pudo encontrar '%s'.\n"+
		"Asegúrate de tener un JDK (o Android Studio) instalado,\n"+
		"o agrega Java a tus variables de entorno (PATH/JAVA_HOME)", exe)
}

// Fingerprints son las huellas de un keystore ya resuelto.
type Fingerprints struct {
	SHA1   string
	SHA256 string
	MD5    string
}

// IsEmpty indica si no se reconoció ninguna huella.
func (f Fingerprints) IsEmpty() bool {
	return f.SHA1 == "" && f.SHA256 == "" && f.MD5 == ""
}

// normalizeFingerprint quita ':' y pasa a minúsculas para comparar.
func normalizeFingerprint(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			b.WriteRune(r | 0x20)
		}
	}
	return b.String()
}

// keystoreFingerprints ejecuta `keytool -list -v` y extrae las huellas.
func keystoreFingerprints(keytool, keystore, storePass string) (Fingerprints, error) {
	out, err := shell.HideConsole(exec.Command(keytool, "-list", "-v", "-keystore", keystore, "-storepass", storePass)).CombinedOutput()
	if err != nil {
		msg := firstNonEmptyLine(string(out))
		if msg == "" {
			msg = "error desconocido"
		}
		return Fingerprints{}, fmt.Errorf("no se pudo leer el keystore: %s", msg)
	}
	return parseFingerprints(string(out)), nil
}

// firstNonEmptyLine devuelve la primera línea no vacía de un texto.
func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return ""
}

// parseFingerprints extrae SHA-1/SHA-256/MD5 del texto de `keytool -list -v`.
func parseFingerprints(text string) Fingerprints {
	var fp Fingerprints
	prefixes := []string{"SHA1:", "SHA256:", "MD5:", "SHA-1:", "SHA-256:"}
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		for _, prefix := range prefixes {
			if !strings.HasPrefix(t, prefix) {
				continue
			}
			name := strings.ReplaceAll(strings.TrimSuffix(prefix, ":"), "-", "")
			value := strings.TrimSpace(strings.TrimPrefix(t, prefix))
			switch name {
			case "SHA1":
				fp.SHA1 = value
			case "SHA256":
				fp.SHA256 = value
			default:
				fp.MD5 = value
			}
		}
	}
	return fp
}

// debugKeystore devuelve el keystore de debug por defecto (~/.android).
func debugKeystore() (path, alias, pass string, ok bool) {
	home := homeDir()
	if home == "" {
		return "", "", "", false
	}
	p := filepath.Join(home, ".android", "debug.keystore")
	if !fileExists(p) {
		return "", "", "", false
	}
	return p, "androiddebugkey", "android", true
}

// ── base64 ──────────────────────────────────────────────────────────────────

// base64Encode codifica en base64 estándar (con relleno '=').
//
// Se usa para el secreto ANDROID_KEYSTORE_BASE64 que esperan los workflows
// típicos: el keystore es binario y GitHub Secrets guarda texto.
func base64Encode(input []byte) string {
	return base64.StdEncoding.EncodeToString(input)
}

// ── Secretos para GitHub Actions ────────────────────────────────────────────

// SecretGuess es un secreto con su valor propuesto y de dónde sale.
type SecretGuess struct {
	Name   string
	Value  string
	Source string
}

var secretRefRe = regexp.MustCompile(`secrets\.([A-Za-z_][A-Za-z0-9_]*)`)

// workflowSecretNames devuelve los `secrets.NOMBRE` referenciados en
// .github/workflows (.yml/.yaml), únicos y ordenados.
func workflowSecretNames(root string) []string {
	dir := filepath.Join(root, ".github", "workflows")
	files := collectFiles(dir, "yml", "yaml")
	var names []string
	seen := map[string]bool{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, m := range secretRefRe.FindAllStringSubmatch(string(data), -1) {
			n := m[1]
			if !seen[n] {
				seen[n] = true
				names = append(names, n)
			}
		}
	}
	sort.Strings(names)
	return names
}

// collectFiles recorre dir recursivamente y devuelve los archivos con las
// extensiones dadas.
func collectFiles(dir string, exts ...string) []string {
	var out []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		if e.IsDir() {
			out = append(out, collectFiles(p, exts...)...)
			continue
		}
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(p)), ".")
		for _, want := range exts {
			if ext == want {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

// guessSecret empareja un nombre de secreto con el valor local que le toca.
//
// El orden importa: ANDROID_KEYSTORE_PASSWORD contiene KEYSTORE, así que las
// contraseñas se comprueban antes que el keystore/base64.
func guessSecret(name string, info SigningInfo, keystoreB64 string, hasB64 bool) SecretGuess {
	u := strings.ToUpper(name)
	store := info.StorePassword()
	keyp := info.KeyPassword()
	alias := info.Alias()

	var value, source string
	b64Value := func() (string, string) {
		if hasB64 {
			return keystoreB64, "keystore (base64)"
		}
		return "", "keystore no encontrado"
	}

	switch {
	case strings.Contains(u, "BASE64") || strings.HasSuffix(u, "B64"):
		value, source = b64Value()
	case strings.Contains(u, "ALIAS"):
		value, source = alias, "keyAlias"
	case strings.Contains(u, "KEYSTORE_PASSWORD") || strings.Contains(u, "STORE_PASS") || strings.Contains(u, "STOREPASS"):
		value, source = store, "storePassword"
	case strings.Contains(u, "KEY_PASSWORD") || strings.Contains(u, "KEYPASS"):
		value, source = keyp, "keyPassword"
	case strings.Contains(u, "PASSWORD"):
		value, source = store, "storePassword"
	case strings.Contains(u, "KEYSTORE") || strings.Contains(u, "STORE_FILE"):
		value, source = b64Value()
	default:
		value, source = "", "sin datos"
	}

	return SecretGuess{Name: name, Value: value, Source: source}
}

// ── Submenú de firmas ───────────────────────────────────────────────────────

type signingItem struct {
	key  string
	card menu.SubItem
	run  func(*project.ProjectContext) error
}

// signingEntries son las 11 entradas del submenú, en orden de pantalla.
func signingEntries() []signingItem {
	return []signingItem{
		{"status", menu.SubItem{
			Icon: "\U0001f4ca", Label: "Estado de la firma",
			Description: "Autodeteccion de keystore y Gradle", ColorKey: "debug",
		}, showSigningStatus},
		{"create", menu.SubItem{
			Icon: "\U0001f511", Label: "Crear keystore",
			Description: "Generar un keystore nuevo", ColorKey: "sign",
		}, createKeystore},
		{"import", menu.SubItem{
			Icon: "\U0001f4e4", Label: "Importar keystore",
			Description: "Importar keystore existente", ColorKey: "install",
		}, importKeystore},
		{"manual", menu.SubItem{
			Icon: "\u270f", Label: "Configuracion manual",
			Description: "Escribir keystore.properties a mano", ColorKey: "terminal",
		}, manualConfigure},
		{"fingerprints", menu.SubItem{
			Icon: "\U0001f532", Label: "Huellas (SHA-1/SHA-256)",
			Description: "Huellas SHA-1, SHA-256 y MD5", ColorKey: "apk",
		}, showFingerprints},
		{"secrets", menu.SubItem{
			Icon: "\U0001f510", Label: "Secretos de GitHub Actions",
			Description: "Valores para secretos de CI", ColorKey: "version",
		}, showSecrets},
		{"backup", menu.SubItem{
			Icon: "\U0001f4be", Label: "Respaldar keystore",
			Description: "Copia de keystore + properties", ColorKey: "build",
		}, backupKeystore},
		{"restore", menu.SubItem{
			Icon: "\U0001f5d1", Label: "Restaurar respaldo",
			Description: "Restaurar desde un respaldo", ColorKey: "reload",
		}, restoreKeystore},
		{"list", menu.SubItem{
			Icon: "\U0001f4cb", Label: "Listar respaldos",
			Description: "Ver respaldos disponibles", ColorKey: "clean",
		}, listBackups},
		{"verify", menu.SubItem{
			Icon: "\u2705", Label: "Verificar APK",
			Description: "apksigner vs keystore configurado", ColorKey: "test",
		}, verifyApk},
	}
}

// SigningManage es el punto de entrada de las tareas sign/signing/sha1/keystore
// cuando se invocan por CLI. La GUI usa las subcards (signing-do-<key>).
func SigningManage(ctx *project.ProjectContext) error {
	if ctx.ProjectType == project.TypePython ||
		ctx.ProjectType == project.TypeGo {
		shell.ClearScreen()
		fmt.Println("[INFO] No aplicable a este tipo de proyecto.")
		ui.Pause()
		return nil
	}

	entries := signingEntries()
	labels := make([]string, len(entries))
	for i, e := range entries {
		labels[i] = e.card.Label + " · " + e.card.Description
	}
	idx, err := prompts.Select("Firmas · keystore", labels)
	if err != nil || idx < 0 || idx >= len(entries) {
		return err
	}
	return entries[idx].run(ctx)
}

// ── Utilidades de layout ────────────────────────────────────────────────────

// requireLayout devuelve el layout o un error tras avisar al usuario.
func requireLayout(ctx *project.ProjectContext) (AndroidLayout, error) {
	layout, ok := androidLayout(ctx)
	if !ok {
		fmt.Fprintln(os.Stderr, "[ERROR] No aplicable a este tipo de proyecto.")
		ui.Pause()
		return AndroidLayout{}, fmt.Errorf("proyecto sin Android")
	}
	return layout, nil
}

// ensureModuleExists comprueba el módulo y crea la carpeta de firma si falta.
func ensureModuleExists(layout AndroidLayout) bool {
	if !dirExists(layout.ModuleDir) {
		fmt.Fprintln(os.Stderr, "[ERROR] No se encontró el directorio Android del proyecto.")
		fmt.Fprintf(os.Stderr, "  Ruta esperada: %s\n", layout.ModuleDir)
		if !layout.PatchGradle {
			fmt.Fprintln(os.Stderr, "  Genéralos con: wails3 update build-assets")
		}
		return false
	}
	if !dirExists(layout.SigningDir) {
		if err := os.MkdirAll(layout.SigningDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] No se pudo crear: %s\n", layout.SigningDir)
			fmt.Fprintf(os.Stderr, "  %v\n", err)
			return false
		}
	}
	return true
}

func signingTypeLabel(pt project.ProjectType) string {
	if pt == project.TypeKotlin {
		return "Kotlin/KMP"
	}
	return pt.Label()
}

// ── Estado / autodetección ──────────────────────────────────────────────────

func showSigningStatus(ctx *project.ProjectContext) error {
	shell.ClearScreen()
	ui.BoxBanner("ESTADO DE LA FIRMA")
	fmt.Println()

	info, ok := detectSigning(ctx)
	if !ok {
		if ctx.ProjectType == project.TypeGoWails {
			fmt.Println("[INFO] En Go + Wails v3 la firma la hace el propio Gradle de Wails")
			fmt.Println("       con variables de entorno, no con un keystore.properties:")
			fmt.Println("         ANDROID_KEYSTORE_FILE, ANDROID_KEYSTORE_PASSWORD,")
			fmt.Println("         ANDROID_KEY_ALIAS, ANDROID_KEY_PASSWORD")
		} else {
			fmt.Println("[INFO] No aplicable a este tipo de proyecto.")
		}
		ui.Pause()
		return nil
	}

	fmt.Printf("  Proyecto : %s\n", signingTypeLabel(ctx.ProjectType))
	fmt.Printf("  Módulo   : %s\n", info.Layout.ModuleDir)
	if info.Layout.SigningDir != info.Layout.ModuleDir {
		fmt.Printf("  Firma en : %s\n", info.Layout.SigningDir)
	}

	if info.Props != nil {
		fmt.Printf("  Properties: %s  [OK]\n", info.Layout.PropsName)
	} else {
		fmt.Printf("  Properties: %s  [FALTA]\n", info.Layout.PropsName)
	}

	switch {
	case info.Keystore != "":
		var size int64
		if st, err := os.Stat(info.Keystore); err == nil {
			size = st.Size()
		}
		fmt.Printf("  Keystore : %s  (%d KB)  [OK]\n", info.Keystore, size/1024)
	case info.KeystoreMissing != "":
		fmt.Printf("  Keystore : %s  [NO EXISTE]\n", info.KeystoreMissing)
	default:
		fmt.Println("  Keystore : (sin configurar)")
	}

	if alias := info.Alias(); alias != "" {
		fmt.Printf("  Alias    : %s\n", alias)
	}

	var gradle string
	if !info.Layout.PatchGradle {
		gradle = "firma por variables de entorno (ANDROID_KEYSTORE_*) [OK]"
	} else {
		switch gradleSigningStatus(info.Layout.ModuleDir) {
		case gradlePatched:
			gradle = "parcheado (signingConfigs inyectado) [OK]"
		case gradleUnpatched:
			gradle = "sin parchear — usa la opción 2/3/4 [WARN]"
		default:
			gradle = "no se encontró build.gradle(.kts) [WARN]"
		}
	}
	fmt.Printf("  Gradle   : %s\n", gradle)

	keytool, ktErr := findKeytool()
	fmt.Println()
	fmt.Println("── Huellas release ────────────────────────────────")
	switch {
	case info.Keystore != "" && info.StorePassword() != "":
		if ktErr != nil {
			fmt.Println("  [WARN] Sin 'keytool' (JDK) para leer las huellas.")
		} else if fp, err := keystoreFingerprints(keytool, info.Keystore, info.StorePassword()); err != nil {
			fmt.Printf("  [WARN] %v\n", err)
		} else {
			printFingerprints(fp)
		}
	case info.Keystore != "":
		fmt.Println("  [INFO] Falta storePassword en el .properties.")
	default:
		fmt.Println("  [INFO] Sin keystore release configurado.")
	}

	fmt.Println()
	fmt.Println("── Huellas debug ──────────────────────────────────")
	if path, alias, pass, ok := debugKeystore(); ok {
		fmt.Printf("  Keystore: %s\n", path)
		fmt.Printf("  Alias   : %s\n", alias)
		if ktErr != nil {
			fmt.Println("  [WARN] Sin 'keytool' (JDK) para leer las huellas.")
		} else if fp, err := keystoreFingerprints(keytool, path, pass); err != nil {
			fmt.Printf("  [WARN] %v\n", err)
		} else {
			printFingerprints(fp)
		}
	} else {
		fmt.Println("  [INFO] No hay debug.keystore en ~/.android (compila una vez en debug).")
	}

	if info.Keystore != "" || info.KeystoreMissing != "" {
		fmt.Println()
		fmt.Println("[TIP] Opción 6: genera los secretos para GitHub Actions.")
	}
	ui.Pause()
	return nil
}

func printFingerprints(fp Fingerprints) {
	if fp.SHA1 != "" {
		fmt.Printf("  SHA-1   : %s\n", fp.SHA1)
	}
	if fp.SHA256 != "" {
		fmt.Printf("  SHA-256 : %s\n", fp.SHA256)
	}
	if fp.MD5 != "" {
		fmt.Printf("  MD5     : %s\n", fp.MD5)
	}
	if fp.IsEmpty() {
		fmt.Println("  [WARN] keytool no devolvió ninguna huella reconocible.")
	}
}

func showFingerprints(ctx *project.ProjectContext) error {
	shell.ClearScreen()
	ui.BoxBanner("HUELLAS DIGITALES ANDROID")
	fmt.Println()

	keytool, err := findKeytool()
	if err != nil {
		return err
	}

	fmt.Println("── DEBUG ──────────────────────────────────────────")
	if path, alias, pass, ok := debugKeystore(); ok {
		fmt.Printf("  Keystore: %s\n", path)
		fmt.Printf("  Alias   : %s\n", alias)
		if fp, err := keystoreFingerprints(keytool, path, pass); err != nil {
			fmt.Printf("  [WARN] %v\n", err)
		} else {
			printFingerprints(fp)
		}
	} else {
		dir := filepath.Join(homeDir(), ".android")
		fmt.Printf("  [WARN] No se encontró debug.keystore en %s\n", dir)
		fmt.Println("  Compila una vez en debug o créalo con keytool.")
	}
	fmt.Println()

	fmt.Println("── RELEASE ────────────────────────────────────────")
	if info, ok := detectSigning(ctx); ok {
		if info.Keystore != "" && info.StorePassword() != "" {
			fmt.Printf("  Keystore: %s\n", info.Keystore)
			if alias := info.Alias(); alias != "" {
				fmt.Printf("  Alias   : %s\n", alias)
			}
			if fp, err := keystoreFingerprints(keytool, info.Keystore, info.StorePassword()); err != nil {
				fmt.Printf("  [WARN] %v\n", err)
			} else {
				printFingerprints(fp)
			}
		} else {
			fmt.Printf("  [INFO] Sin firma release configurada (falta %s).\n", info.Layout.PropsName)
			fmt.Println("  Usa la opción 2 o 3 para crearla.")
		}
	} else {
		fmt.Println("  [INFO] No aplicable a este tipo de proyecto.")
	}

	fmt.Println()
	fmt.Println("[INFO] Pega estas huellas en Firebase / Google Cloud Console según lo necesites.")
	ui.Pause()
	return nil
}

// ── Crear / importar / manual ──────────────────────────────────────────────

func createKeystore(ctx *project.ProjectContext) error {
	layout, err := requireLayout(ctx)
	if err != nil {
		return err
	}
	if !ensureModuleExists(layout) {
		ui.Pause()
		return nil
	}
	fmt.Println()
	ui.InfoBanner("Crear nueva llave (Keystore)")

	name := strings.TrimSpace(ui.AskText("Nombre del keystore (sin extensión)", "release"))
	if !strings.HasSuffix(name, ".keystore") {
		name += ".keystore"
	}
	alias := ui.AskText("Alias de la llave", "upload")
	pass := ui.AskPassword("Contraseña para el Keystore (mín 6 caracteres)")

	dest := filepath.Join(layout.SigningDir, name)
	keytool, err := findKeytool()
	if err != nil {
		return err
	}
	fmt.Printf("  keytool encontrado en: %s\n", keytool)
	fmt.Println()

	fmt.Println("  Limpiando keystores existentes...")
	if entries, err := os.ReadDir(layout.SigningDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if ext != ".keystore" && ext != ".jks" {
				continue
			}
			if os.Remove(filepath.Join(layout.SigningDir, e.Name())) == nil {
				fmt.Printf("    Eliminado: %s\n", e.Name())
			}
		}
	}

	fmt.Println("\n  Generando keystore (esto puede tardar unos segundos)...")
	// El CN sólo afecta a los metadatos del certificado, no a la firma.
	const dname = "CN=Compass App, OU=Development, O=Developer, L=City, ST=State, C=US"
	genkey := func(a string) *exec.Cmd {
		return shell.HideConsole(exec.Command(keytool,
			"-genkey", "-v",
			"-keystore", dest,
			"-alias", a,
			"-keyalg", "RSA",
			"-keysize", "2048",
			"-validity", "10000",
			"-storepass", pass,
			"-keypass", pass,
			"-dname", dname,
		))
	}

	first := genkey(alias)
	first.Stdin, first.Stdout, first.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := first.Run(); err != nil {
		fmt.Printf("  Falló con alias '%s'. Probando con alias temporal...\n", alias)
		tempAlias := "temp_" + alias
		_ = shell.HideConsole(exec.Command(keytool,
			"-delete", "-alias", tempAlias,
			"-keystore", dest, "-storepass", pass,
		)).Run()

		second := genkey(tempAlias)
		second.Stdin, second.Stdout, second.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := second.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "[ERROR] Falló la creación del keystore.")
			fmt.Fprintln(os.Stderr, "Sugerencia: prueba con otro nombre de archivo o alias diferente.")
			ui.Pause()
			return nil
		}

		fmt.Printf("  Renombrando alias '%s' a '%s'...\n", tempAlias, alias)
		_ = shell.HideConsole(exec.Command(keytool,
			"-changealias", "-alias", tempAlias, "-destalias", alias,
			"-keystore", dest, "-storepass", pass, "-keypass", pass,
		)).Run()
	}

	fmt.Printf("  [OK] Keystore generado: %s\n", name)
	fmt.Println()

	if err := writeProperties(layout, name, pass, pass, alias); err != nil {
		return err
	}
	if err := patchBuildGradle(layout); err != nil {
		return err
	}

	ui.SuccessBanner("Firmas configuradas exitosamente")
	ui.Pause()
	return nil
}

func importKeystore(ctx *project.ProjectContext) error {
	layout, err := requireLayout(ctx)
	if err != nil {
		return err
	}
	if !ensureModuleExists(layout) {
		ui.Pause()
		return nil
	}
	fmt.Println()
	ui.InfoBanner("Usando llave existente")

	pathStr := strings.TrimSpace(ui.AskText("Ruta a tu archivo .keystore o .jks", ""))
	keystoreFile := filepath.FromSlash(pathStr)

	if !fileExists(keystoreFile) && !dirExists(keystoreFile) {
		fmt.Fprintf(os.Stderr, "[ERROR] No se encontró el archivo: %s\n", pathStr)
		ui.Pause()
		return nil
	}

	if dirExists(keystoreFile) {
		fmt.Println("  Buscando archivo .keystore o .jks en la carpeta...")
		found := ""
		if entries, err := os.ReadDir(keystoreFile); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				ext := strings.ToLower(filepath.Ext(e.Name()))
				if ext == ".keystore" || ext == ".jks" {
					found = filepath.Join(keystoreFile, e.Name())
					break
				}
			}
		}
		if found == "" {
			fmt.Fprintln(os.Stderr, "[ERROR] No se encontró ningún .keystore ni .jks en la carpeta.")
			ui.Pause()
			return nil
		}
		keystoreFile = found
		fmt.Printf("  Llave encontrada: %s\n", filepath.Base(keystoreFile))
	}

	alias := ui.AskText("Alias de la llave (Key Alias)", "")
	storePass := ui.AskPassword("Contraseña del Keystore (Store Password)")
	keyPassInput := ui.AskPassword("Contraseña de la Llave (Key Password)")
	keyPass := storePass
	if keyPassInput != "" {
		keyPass = keyPassInput
	}

	kName := filepath.Base(keystoreFile)
	dest := filepath.Join(layout.SigningDir, kName)
	if err := copyFile(keystoreFile, dest); err != nil {
		return fmt.Errorf("error al copiar el archivo: %w", err)
	}
	fmt.Println("  [OK] Archivo copiado correctamente.")

	if err := writeProperties(layout, kName, storePass, keyPass, alias); err != nil {
		return err
	}
	if err := patchBuildGradle(layout); err != nil {
		return err
	}

	ui.SuccessBanner("Firmas configuradas exitosamente")
	ui.Pause()
	return nil
}

func manualConfigure(ctx *project.ProjectContext) error {
	layout, err := requireLayout(ctx)
	if err != nil {
		return err
	}
	if !ensureModuleExists(layout) {
		ui.Pause()
		return nil
	}
	fmt.Println()
	ui.InfoBanner("Configuración manual")

	name := ui.AskText("Nombre del archivo .keystore (ej: release.keystore)", "release.keystore")
	alias := ui.AskText("Alias de la llave", "upload")
	storePass := ui.AskPassword("Contraseña del Keystore (Store Password)")
	keyPassInput := ui.AskPassword("Contraseña de la Llave (Key Password)")
	keyPass := storePass
	if keyPassInput != "" {
		keyPass = keyPassInput
	}

	if err := writeProperties(layout, name, storePass, keyPass, alias); err != nil {
		return err
	}
	if err := patchBuildGradle(layout); err != nil {
		return err
	}

	ui.SuccessBanner("Configuración manual completada")
	fmt.Printf("  Asegúrate de que '%s' exista en %s.\n", name, layout.SigningDir)
	ui.Pause()
	return nil
}

func writeProperties(layout AndroidLayout, keystoreName, storePass, keyPass, keyAlias string) error {
	props := Props{
		StoreFile:     keystoreName,
		StorePassword: storePass,
		KeyAlias:      keyAlias,
		KeyPassword:   keyPass,
	}
	path := layout.PropsPath()
	if err := os.WriteFile(path, []byte(props.Render()), 0644); err != nil {
		return fmt.Errorf("error al escribir %s: %w", path, err)
	}
	fmt.Printf("  [OK] %s generado.\n", layout.PropsName)
	return nil
}

// patchBuildGradle inyecta el loader, signingConfigs y el signingConfig de
// release en el build.gradle(.kts) del módulo. Soporta Kotlin Script y Groovy.
func patchBuildGradle(layout AndroidLayout) error {
	if !layout.PatchGradle {
		fmt.Println("  [OK] El Gradle de Wails ya lee la firma del entorno; no hay que parchearlo.")
		fmt.Println("       Compass exporta ANDROID_KEYSTORE_* al compilar la APK/AAB release.")
		return nil
	}

	androidApp := layout.ModuleDir
	propsName := layout.PropsName

	kts := filepath.Join(androidApp, "build.gradle.kts")
	useKts := fileExists(kts)
	gradlePath := kts
	if !useKts {
		groovy := filepath.Join(androidApp, "build.gradle")
		if !fileExists(groovy) {
			fmt.Fprintf(os.Stderr, "[WARNING] No se encontró build.gradle(.kts) en: %s\n", androidApp)
			fmt.Fprintln(os.Stderr, "  Puedes agregar la configuración manualmente.")
			return nil
		}
		gradlePath = groovy
	}

	data, err := os.ReadFile(gradlePath)
	if err != nil {
		return fmt.Errorf("error al leer %s: %w", gradlePath, err)
	}
	content := string(data)
	modified := false

	moduleName := filepath.Base(androidApp)
	propertiesRef := moduleName + "/" + propsName

	if useKts {
		if !strings.Contains(content, "import java.util.Properties") &&
			!strings.Contains(content, "import java.util.*") {
			content = "import java.util.Properties\n\n" + content
			modified = true
		}

		if !strings.Contains(content, "val keystorePropertiesFile") {
			loader := fmt.Sprintf("\nval keystorePropertiesFile = rootProject.file(\"%s\")\nval keystoreProperties = Properties()\nif (keystorePropertiesFile.exists()) {\n    keystorePropertiesFile.inputStream().use { keystoreProperties.load(it) }\n}\n", propertiesRef)
			content = strings.ReplaceAll(content, "android {", loader+"\nandroid {")
			modified = true
		}

		if !strings.Contains(content, "signingConfigs {") {
			signing := "\n    signingConfigs {\n        if (keystorePropertiesFile.exists()) {\n            create(\"release\") {\n                keyAlias = keystoreProperties.getProperty(\"keyAlias\") ?: \"\"\n                keyPassword = keystoreProperties.getProperty(\"keyPassword\") ?: \"\"\n                storeFile = file(keystoreProperties.getProperty(\"storeFile\") ?: \"\")\n                storePassword = keystoreProperties.getProperty(\"storePassword\") ?: \"\"\n            }\n        }\n    }\n"
			content = strings.ReplaceAll(content, "    defaultConfig {", signing+"\n    defaultConfig {")
			modified = true
		}

		if !strings.Contains(content, "signingConfig = signingConfigs.getByName(\"release\")") {
			apply := "\n            if (keystorePropertiesFile.exists()) {\n                signingConfig = signingConfigs.getByName(\"release\")\n            }"
			content = strings.ReplaceAll(content, "getByName(\"release\") {", "getByName(\"release\") {"+apply)
			modified = true
		}
	} else {
		if !strings.Contains(content, "def keystorePropertiesFile") {
			loader := fmt.Sprintf("\ndef keystorePropertiesFile = rootProject.file('%s')\ndef keystoreProperties = new Properties()\nif (keystorePropertiesFile.exists()) {\n    keystoreProperties.load(new FileInputStream(keystorePropertiesFile))\n}\n", propertiesRef)
			content = strings.ReplaceAll(content, "android {", loader+"\nandroid {")
			modified = true
		}

		if !strings.Contains(content, "signingConfigs {") {
			signing := "\n    signingConfigs {\n        if (keystorePropertiesFile.exists()) {\n            release {\n                keyAlias keystoreProperties['keyAlias']\n                keyPassword keystoreProperties['keyPassword']\n                storeFile file(keystoreProperties['storeFile'])\n                storePassword keystoreProperties['storePassword']\n            }\n        }\n    }\n"
			content = strings.ReplaceAll(content, "    defaultConfig {", signing+"\n    defaultConfig {")
			modified = true
		}

		if !strings.Contains(content, "signingConfig signingConfigs.release") {
			apply := "\n            if (keystorePropertiesFile.exists()) {\n                signingConfig = signingConfigs.release\n            }"
			content = strings.ReplaceAll(content, "release {", "release {"+apply)
			modified = true
		}
	}

	if modified {
		if err := os.WriteFile(gradlePath, []byte(content), 0644); err != nil {
			return fmt.Errorf("error al escribir %s: %w", gradlePath, err)
		}
		fmt.Printf("  [OK] %s actualizado con configuración de firma.\n", filepath.Base(gradlePath))
	} else {
		fmt.Printf("  %s ya contenía la configuración.\n", filepath.Base(gradlePath))
	}
	return nil
}

// ── Respaldos ──────────────────────────────────────────────────────────────

func backupDir(ctx *project.ProjectContext) string {
	return filepath.Join(ctx.ScriptDir, "keystore-backups")
}

func backupKeystore(ctx *project.ProjectContext) error {
	shell.ClearScreen()
	ui.BoxBanner("RESPALDAR KEYSTORE")
	layout, err := requireLayout(ctx)
	if err != nil {
		return err
	}
	if !ensureModuleExists(layout) {
		ui.Pause()
		return nil
	}

	// Keystores de la carpeta de firma + el declarado en las properties (puede
	// vivir fuera).
	var files []string
	seen := map[string]bool{}
	if entries, err := os.ReadDir(layout.SigningDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := strings.ToLower(e.Name())
			if strings.HasSuffix(name, ".keystore") || strings.HasSuffix(name, ".jks") {
				p := filepath.Join(layout.SigningDir, e.Name())
				files = append(files, p)
				seen[strings.ToLower(p)] = true
			}
		}
	}
	if info, ok := detectSigning(ctx); ok && info.Keystore != "" {
		if !seen[strings.ToLower(info.Keystore)] {
			files = append(files, info.Keystore)
		}
	}

	if len(files) == 0 {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "[ERROR] No se encontraron archivos .keystore o .jks.")
		fmt.Fprintf(os.Stderr, "La ruta buscada es: %s\n", layout.SigningDir)
		fmt.Fprintln(os.Stderr, "Primero crea uno con la opción 2.")
		ui.Pause()
		return nil
	}

	timestamp := time.Now().Format("20060102_150405")
	destDir := filepath.Join(backupDir(ctx), timestamp)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	for _, src := range files {
		name := filepath.Base(src)
		if err := copyFile(src, filepath.Join(destDir, name)); err != nil {
			return err
		}
		fmt.Printf("  [OK] %s respaldado.\n", name)
	}

	// Las properties van con el keystore: sin ellas el respaldo no sirve.
	for _, propsName := range []string{"keystore.properties", "key.properties"} {
		props := filepath.Join(layout.SigningDir, propsName)
		if !fileExists(props) {
			continue
		}
		if err := copyFile(props, filepath.Join(destDir, propsName)); err != nil {
			return err
		}
		fmt.Printf("  [OK] %s respaldado.\n", propsName)
	}

	fmt.Println()
	fmt.Println("[OK] Respaldo completado en:")
	fmt.Printf("     %s\n", destDir)
	fmt.Println()
	fmt.Println("IMPORTANTE: Guarda esta carpeta en un lugar seguro (USB, nube).")
	ui.Pause()
	return nil
}

func restoreKeystore(ctx *project.ProjectContext) error {
	shell.ClearScreen()
	ui.BoxBanner("RESTAURAR KEYSTORE")
	layout, err := requireLayout(ctx)
	if err != nil {
		return err
	}

	dir := backupDir(ctx)
	if !dirExists(dir) {
		fmt.Fprintln(os.Stderr, "[ERROR] No existe la carpeta de respaldos.")
		fmt.Fprintln(os.Stderr, "Primero crea un respaldo con la opción 7.")
		ui.Pause()
		return nil
	}

	backups := backupDirs(dir)
	if len(backups) == 0 {
		fmt.Fprintln(os.Stderr, "[ERROR] No hay respaldos disponibles.")
		ui.Pause()
		return nil
	}

	labels := make([]string, len(backups))
	for i, p := range backups {
		count := 0
		if entries, err := os.ReadDir(p); err == nil {
			count = len(entries)
		}
		labels[i] = fmt.Sprintf("%s (%d archivo(s))", filepath.Base(p), count)
	}

	idx, err := prompts.Select("Selecciona respaldo a restaurar:", labels)
	if err != nil {
		return nil
	}
	selected := backups[idx]

	count := 0
	entries, err := os.ReadDir(selected)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		src := filepath.Join(selected, e.Name())
		if err := copyFile(src, filepath.Join(layout.SigningDir, e.Name())); err != nil {
			return err
		}
		fmt.Printf("  [OK] %s restaurado.\n", e.Name())
		count++
	}

	fmt.Println()
	fmt.Printf("[OK] Restauración completada (%d archivo(s)).\n", count)
	fmt.Println("Ya puedes compilar tu Android en modo release con firma.")
	ui.Pause()
	return nil
}

// backupDirs devuelve las subcarpetas de respaldo ordenadas por nombre.
func backupDirs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out
}

func listBackups(ctx *project.ProjectContext) error {
	shell.ClearScreen()
	ui.BoxBanner("RESPALDOS DISPONIBLES")

	dir := backupDir(ctx)
	if !dirExists(dir) {
		fmt.Println("No existe la carpeta de respaldos.")
		fmt.Println("Usa la opción 7 para crear tu primer respaldo.")
		ui.Pause()
		return nil
	}

	backups := backupDirs(dir)
	if len(backups) == 0 {
		fmt.Println("No hay respaldos aún.")
		fmt.Println("Usa la opción 7 para crear tu primer respaldo.")
	} else {
		fmt.Println()
		fmt.Printf("Total de respaldos: %d\n", len(backups))
		fmt.Println()
		for _, entry := range backups {
			var files int
			var totalSize int64
			if entries, err := os.ReadDir(entry); err == nil {
				for _, f := range entries {
					files++
					if info, err := f.Info(); err == nil {
						totalSize += info.Size()
					}
				}
			}
			fmt.Printf("  %s  |  %d archivo(s)  |  %.2f KB\n",
				filepath.Base(entry), files, float64(totalSize)/1024.0)
		}
	}
	ui.Pause()
	return nil
}

// ── Secretos para GitHub Actions ───────────────────────────────────────────

func showSecrets(ctx *project.ProjectContext) error {
	shell.ClearScreen()
	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Println("║        SECRETOS PARA GITHUB ACTIONS              ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")
	fmt.Println()

	info, ok := detectSigning(ctx)
	if !ok {
		fmt.Println("[INFO] No aplicable a este tipo de proyecto.")
		ui.Pause()
		return nil
	}

	keystoreB64 := ""
	hasB64 := false
	if info.Keystore != "" {
		if data, err := os.ReadFile(info.Keystore); err == nil {
			keystoreB64 = base64Encode(data)
			hasB64 = true
		}
	}
	if !hasB64 {
		fmt.Println("[WARN] No se encontró el keystore; el base64 irá vacío.")
		fmt.Println("       Configura la firma primero (opción 2 o 3).")
		fmt.Println()
	}

	// 1. Nombres detectados en los workflows del proyecto.
	names := workflowSecretNames(ctx.ScriptDir)
	workflows := collectFiles(filepath.Join(ctx.ScriptDir, ".github", "workflows"), "yml", "yaml")

	if len(names) > 0 {
		fmt.Printf("── Detectados en tus workflows (%d archivo(s)) ────\n", len(workflows))
		fmt.Println()
		for _, n := range names {
			printSecret(guessSecret(n, info, keystoreB64, hasB64))
		}
		fmt.Println()
	} else {
		fmt.Println("[INFO] No se encontraron secretos (`secrets.NOMBRE`) en .github/workflows.")
		fmt.Println("       Abajo van los valores canónicos que suelen usar las apps Android.")
		fmt.Println()
	}

	// 2. Valores canónicos, siempre (aunque no haya workflows).
	fmt.Println("── Valores canónicos ──────────────────────────────")
	fmt.Println()
	canonical := []SecretGuess{
		{Name: "ANDROID_KEYSTORE_BASE64", Value: keystoreB64, Source: "keystore (base64)"},
		{Name: "ANDROID_KEYSTORE_PASSWORD", Value: info.StorePassword(), Source: "storePassword"},
		{Name: "ANDROID_KEY_ALIAS", Value: info.Alias(), Source: "keyAlias"},
		{Name: "ANDROID_KEY_PASSWORD", Value: info.KeyPassword(), Source: "keyPassword"},
	}
	var report strings.Builder
	report.WriteString("# Secretos para GitHub Actions (generado por compass)\n")
	report.WriteString("# NO COMMITEAR: contiene la contraseña del keystore.\n\n")
	for _, g := range canonical {
		printSecret(g)
		fmt.Fprintf(&report, "%s\n%s\n\n", g.Name, g.Value)
	}

	if len(names) > 0 {
		report.WriteString("# Detectados en tus workflows\n")
		for _, n := range names {
			g := guessSecret(n, info, keystoreB64, hasB64)
			fmt.Fprintf(&report, "%s # %s\n", g.Name, g.Source)
		}
	}

	fmt.Println()
	fmt.Println("[WARN] Estos valores son secretos: la contraseña aparece en claro para")
	fmt.Println("       que la copies a GitHub ▸ Settings ▸ Secrets and variables ▸ Actions.")
	fmt.Println()

	save := ui.AskText("¿Guardar el reporte en un archivo? (s/N)", "N")
	if ui.IsYes(save) {
		path := filepath.Join(ctx.ScriptDir, "github-secrets.txt")
		if err := os.WriteFile(path, []byte(report.String()), 0644); err != nil {
			return fmt.Errorf("no se pudo escribir %s: %w", path, err)
		}
		fmt.Printf("  [OK] Reporte guardado en: %s\n", path)

		// Girar el .gitignore del proyecto para no filtrar el archivo.
		gi := filepath.Join(ctx.ScriptDir, ".gitignore")
		if fileExists(gi) {
			content, _ := os.ReadFile(gi)
			if !strings.Contains(string(content), "github-secrets.txt") {
				newContent := string(content)
				if !strings.HasSuffix(newContent, "\n") {
					newContent += "\n"
				}
				newContent += "\n# Secretos generados por compass\ngithub-secrets.txt\n"
				_ = os.WriteFile(gi, []byte(newContent), 0644)
				fmt.Println("  [OK] Añadido a .gitignore (no se subirá por accidente).")
			}
		} else {
			fmt.Println("  [WARN] No hay .gitignore: asegúrate de no commitear este archivo.")
		}
	}

	ui.Pause()
	return nil
}

func printSecret(g SecretGuess) {
	shown := g.Value
	switch {
	case g.Value == "":
		shown = "(sin datos)"
	case len(g.Value) > 72:
		shown = fmt.Sprintf("%s… (%d bytes)", g.Value[:72], len(g.Value))
	}
	fmt.Printf("  %-28s = %-80s (%s)\n", g.Name, shown, g.Source)
}

// ── Verificación de APK ────────────────────────────────────────────────────

// findApksigner localiza apksigner en la build-tools más reciente del SDK.
func findApksigner(ctx *project.ProjectContext) string {
	if ctx.AndroidHome == "" {
		return ""
	}
	root := filepath.Join(ctx.AndroidHome, "build-tools")
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}

	type candidate struct {
		key  []int
		path string
	}
	var versions []candidate
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		key := versionKey(e.Name())
		if len(key) == 0 {
			continue
		}
		for _, bin := range []string{"apksigner.exe", "apksigner.bat"} {
			p := filepath.Join(root, e.Name(), bin)
			if fileExists(p) {
				versions = append(versions, candidate{key, p})
				break
			}
		}
	}
	sort.SliceStable(versions, func(i, j int) bool {
		return compareVersion(versions[i].key, versions[j].key) > 0
	})
	if len(versions) == 0 {
		return ""
	}
	return versions[0].path
}

func versionKey(name string) []int {
	var key []int
	for _, part := range strings.Split(name, ".") {
		if n, err := strconv.Atoi(part); err == nil {
			key = append(key, n)
		}
	}
	return key
}

func compareVersion(a, b []int) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	default:
		return 0
	}
}

// findNewestApk busca el APK más reciente entre las carpetas de salida típicas.
func findNewestApk(ctx *project.ProjectContext) string {
	roots := []string{filepath.Join(ctx.ScriptDir, "build", "app", "outputs")}
	if layout, ok := androidLayout(ctx); ok {
		roots = append(roots, filepath.Join(layout.ModuleDir, "build", "outputs", "apk"))
	}

	newest := ""
	var newestTime time.Time
	for _, root := range roots {
		for _, apk := range collectApks(root) {
			st, err := os.Stat(apk)
			if err != nil {
				continue
			}
			if newest == "" || st.ModTime().After(newestTime) {
				newest = apk
				newestTime = st.ModTime()
			}
		}
	}
	return newest
}

func collectApks(dir string) []string {
	var out []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		if e.IsDir() {
			out = append(out, collectApks(p)...)
		} else if strings.EqualFold(filepath.Ext(p), ".apk") {
			out = append(out, p)
		}
	}
	return out
}

// runToolCapture ejecuta una herramienta (admite .bat vía cmd.exe) y devuelve
// stdout+stderr combinados.
func runToolCapture(program string, args ...string) ([]byte, error) {
	ext := strings.ToLower(filepath.Ext(program))
	var cmd *exec.Cmd
	if ext == ".bat" || ext == ".cmd" {
		cmd = shell.HideConsole(exec.Command("cmd", append([]string{"/c", program}, args...)...))
	} else {
		cmd = shell.HideConsole(exec.Command(program, args...))
	}
	return cmd.CombinedOutput()
}

func verifyApk(ctx *project.ProjectContext) error {
	shell.ClearScreen()
	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Println("║           VERIFICAR FIRMA DE UN APK              ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")
	fmt.Println()

	apk := findNewestApk(ctx)
	if apk != "" {
		fmt.Printf("[INFO] APK detectado: %s\n", apk)
	} else {
		input := strings.TrimSpace(ui.AskText("Ruta del APK a verificar", ""))
		if input == "" {
			fmt.Println("[INFO] Cancelado.")
			ui.Pause()
			return nil
		}
		apk = filepath.FromSlash(input)
	}

	if !fileExists(apk) {
		fmt.Fprintf(os.Stderr, "[ERROR] No existe: %s\n", apk)
		ui.Pause()
		return nil
	}

	apksigner := findApksigner(ctx)
	if apksigner == "" {
		fmt.Fprintln(os.Stderr, "[ERROR] No se encontró 'apksigner' en $ANDROID_HOME/build-tools.")
		fmt.Fprintln(os.Stderr, `Instala build-tools con: sdkmanager "build-tools;34.0.0"`)
		ui.Pause()
		return nil
	}

	out, err := runToolCapture(apksigner, "verify", "--print-certs", apk)
	if err != nil && len(out) == 0 {
		return fmt.Errorf("error al ejecutar apksigner: %w", err)
	}
	text := string(out)

	fmt.Println()
	certs := parseApksignerCerts(text)
	if len(certs) == 0 {
		fmt.Println("[ERROR] apksigner no devolvió certificados.")
		fmt.Println("        ¿Está el APK firmado? Salida:")
		lines := strings.Split(text, "\n")
		for i, line := range lines {
			if i >= 10 {
				break
			}
			fmt.Printf("        %s\n", line)
		}
		ui.Pause()
		return nil
	}

	fmt.Println("── Firmante del APK ───────────────────────────────")
	apkSHA1, apkSHA256 := "", ""
	for _, c := range certs {
		fmt.Printf("  %-9s: %s\n", c[0], c[1])
		switch c[0] {
		case "SHA-1":
			apkSHA1 = normalizeFingerprint(c[1])
		case "SHA-256":
			apkSHA256 = normalizeFingerprint(c[1])
		}
	}

	// Comparar contra el keystore release configurado.
	fmt.Println()
	fmt.Println("── Comparación con el keystore configurado ────────")
	info, ok := detectSigning(ctx)
	if ok && info.Keystore != "" && info.StorePassword() != "" {
		keytool, err := findKeytool()
		if err != nil {
			fmt.Printf("  [WARN] %v\n", err)
			ui.Pause()
			return nil
		}
		fp, err := keystoreFingerprints(keytool, info.Keystore, info.StorePassword())
		if err != nil {
			fmt.Printf("  [WARN] No se pudieron leer las huellas del keystore: %v\n", err)
		} else {
			wantSHA256 := normalizeFingerprint(fp.SHA256)
			wantSHA1 := normalizeFingerprint(fp.SHA1)
			sha256OK := wantSHA256 != "" && wantSHA256 == apkSHA256
			sha1OK := wantSHA1 != "" && wantSHA1 == apkSHA1
			if sha256OK || sha1OK {
				ui.SuccessBanner("El APK coincide con el keystore configurado")
			} else {
				fmt.Println("  [WARN] El APK NO coincide con el keystore release configurado.")
				fmt.Println("         El APK se firmó con otras llaves (¿debug? ¿keystore viejo?).")
			}
		}
	} else {
		fmt.Println("  [INFO] No hay keystore release configurado para comparar.")
	}

	ui.Pause()
	return nil
}

// parseApksignerCerts extrae (label, valor) de `apksigner verify --print-certs`.
func parseApksignerCerts(text string) [][2]string {
	var out [][2]string
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		if _, v, ok := strings.Cut(t, "certificate SHA-256 digest:"); ok {
			out = append(out, [2]string{"SHA-256", strings.TrimSpace(v)})
		} else if _, v, ok := strings.Cut(t, "certificate SHA-1 digest:"); ok {
			out = append(out, [2]string{"SHA-1", strings.TrimSpace(v)})
		}
	}
	return out
}

// kotlinSigningEnv devuelve las variables de entorno de firma release para un
// proyecto Kotlin, leyendo keystore.properties del directorio del módulo
// Android. Devuelve un mapa vacío si no está configurado.
//
// Equivalente de src/tasks/kotlin.rs::release_signing_env / signing_envs.
func kotlinSigningEnv(ctx *project.ProjectContext) map[string]string {
	props, ok := readProps(filepath.Join(kotlinAndroidDir(ctx), "keystore.properties"))
	if !ok || props.StoreFile == "" {
		return map[string]string{}
	}
	storePath := resolveKeystore(kotlinAndroidDir(ctx), props.StoreFile)
	if !fileExists(storePath) {
		fmtWarn("No se encontro keystore.properties de release; el build release podria fallar. " +
			"Usa la tarjeta Keystore para configurar la firma.")
		return map[string]string{}
	}
	keyPass := props.KeyPassword
	if keyPass == "" {
		keyPass = props.StorePassword
	}
	return map[string]string{
		"RELEASE_STORE_FILE":     storePath,
		"RELEASE_STORE_PASSWORD": props.StorePassword,
		"RELEASE_KEY_ALIAS":      props.KeyAlias,
		"RELEASE_KEY_PASSWORD":   keyPass,
	}
}
