package tasks

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"compass/internal/gitmsg"
	"compass/internal/menu"
	"compass/internal/project"
	"compass/internal/prompts"
	"compass/internal/shell"
	"compass/internal/ui"
)

func init() {
	for _, pt := range allTypes() {
		Register("git", pt, GitManage)
		Register("git-init", pt, GitManage)
		Register("context-gitignore", pt, addContextGitignoreTask)
		// Acciones individuales para las subcards nativas de la GUI.
		Register("git-do-init", pt, func(ctx *project.ProjectContext) error {
			return GitAction(ctx, "init")
		})
		Register("git-do-remote", pt, func(ctx *project.ProjectContext) error {
			return GitAction(ctx, "remote")
		})
		Register("git-do-status", pt, func(ctx *project.ProjectContext) error {
			return GitAction(ctx, "status")
		})
	}
}

// GitAction ejecuta una opción del submenú de Git sin dibujar el submenú de
// tcell: la GUI la ofrece como subcard clicable.
func GitAction(ctx *project.ProjectContext, action string) error {
	for _, e := range gitEntries() {
		if e.key == action && e.run != nil {
			return e.run(ctx)
		}
	}
	return nil
}

// ── helpers de ejecución ────────────────────────────────────────────────────

func runCapture(program string, args []string, dir string) error {
	cmd := shell.HideConsole(exec.Command(program, args...))
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return nil
}

func gitCapture(ctx *project.ProjectContext, args ...string) error {
	return runCapture("git", args, ctx.ScriptDir)
}

func gitOut(ctx *project.ProjectContext, args ...string) string {
	cmd := shell.HideConsole(exec.Command("git", args...))
	cmd.Dir = ctx.ScriptDir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func isRepo(ctx *project.ProjectContext) bool {
	_, err := os.Stat(filepath.Join(ctx.ScriptDir, ".git"))
	return err == nil
}

func hasCommits(ctx *project.ProjectContext) bool {
	return gitOut(ctx, "rev-parse", "--verify", "HEAD") != ""
}

// ── Inicialización ──────────────────────────────────────────────────────────

func createRepo(ctx *project.ProjectContext) bool {
	if isRepo(ctx) && hasCommits(ctx) {
		return true
	}
	if !isRepo(ctx) {
		fmt.Println("[INFO] Inicializando repositorio git...")
		if err := gitCapture(ctx, "init"); err != nil {
			fmt.Printf("[ERROR] git init fallo: %v\n", err)
			return false
		}
		fmt.Println("[OK] Repositorio inicializado.")
	}

	branch := gitOut(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if branch == "" {
		branch = "main"
	}
	if branch != "main" {
		fmt.Println("[INFO] Renombrando rama a 'main'...")
		if gitCapture(ctx, "branch", "-M", "main") == nil {
			fmt.Println("[OK] Rama renombrada a 'main'.")
		} else {
			fmt.Println("[WARN] No se pudo renombrar la rama a 'main'.")
		}
	} else {
		fmt.Println("[OK] La rama ya se llama 'main'.")
	}

	fmt.Println("[INFO] Agregando archivos al staging...")
	_ = gitCapture(ctx, "add", ".")
	fmt.Println("[OK] Archivos agregados.")

	fmt.Println("[INFO] Creando primer commit...")
	if err := gitCapture(ctx, "commit", "-m", "First Commit"); err != nil {
		msg := err.Error()
		if strings.Contains(msg, "nothing to commit") ||
			strings.Contains(msg, "no changes") ||
			strings.Contains(msg, "nothing added") {
			fmt.Println("[INFO] No habia cambios que commitear.")
		} else {
			fmt.Printf("[WARN] git commit: %s\n", msg)
			if strings.Contains(msg, "who you are") ||
				strings.Contains(msg, "user.email") ||
				strings.Contains(msg, "user.name") {
				fmt.Println("       Configura tu identidad de git:")
				fmt.Println("         git config --global user.name \"Tu Nombre\"")
				fmt.Println("         git config --global user.email \"tu@email.com\"")
			}
		}
	} else {
		fmt.Println("[OK] Primer commit creado: 'First Commit'.")
	}

	return hasCommits(ctx)
}

func gitInit(ctx *project.ProjectContext) error {
	shell.ClearScreen()
	ui.BoxBanner("INICIALIZAR REPOSITORIO GIT (main)")
	fmt.Println()

	if isRepo(ctx) && hasCommits(ctx) {
		fmt.Println("[INFO] El repositorio ya existe y tiene commits.")
		fmt.Println("  No se realizo ningun cambio.")
		ui.Pause()
		return nil
	}

	if !createRepo(ctx) {
		fmt.Println("[WARN] El repositorio quedo sin commits; revisa la configuracion de git.")
	}
	fmt.Println()
	ui.Pause()
	return nil
}

// ── Conexión con remoto ─────────────────────────────────────────────────────

func looksLikeGitURL(url string) bool {
	u := strings.TrimSpace(url)
	ok := strings.HasPrefix(u, "https://") ||
		strings.HasPrefix(u, "http://") ||
		strings.HasPrefix(u, "git@") ||
		strings.HasPrefix(u, "ssh://")
	return ok && (strings.Contains(u, "/") || strings.Contains(u, ":"))
}

func webURL(remote string) string {
	r := strings.TrimSpace(remote)
	if rest := strings.TrimPrefix(r, "git@"); rest != r {
		if host, path, ok := strings.Cut(rest, ":"); ok {
			return fmt.Sprintf("https://%s/%s", host, strings.TrimSuffix(path, ".git"))
		}
	}
	if strings.HasPrefix(r, "http://") || strings.HasPrefix(r, "https://") {
		return strings.TrimSuffix(r, ".git")
	}
	return ""
}

func pushCurrentBranch(ctx *project.ProjectContext) {
	branch := gitOut(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if branch == "" || branch == "HEAD" {
		branch = "main"
	}

	fmt.Printf("[INFO] Subiendo la rama '%s' (git push -u origin %s)...\n", branch, branch)
	if err := gitCapture(ctx, "push", "-u", "origin", branch); err != nil {
		fmt.Printf("[ERROR] git push fallo:\n%s\n", err)
		fmt.Println()
		if hint := gitmsg.ExplainPush(err.Error()); hint != "" {
			fmt.Println(hint)
			return
		}
		fmt.Println("Pistas:")
		fmt.Println("  - ¿Estas autenticado? (Personal Access Token en HTTPS, o `gh auth login`).")
		fmt.Printf("  - Si el remoto ya tenia contenido: git pull --rebase origin %s\n", branch)
		fmt.Println("  - El repo debe existir en GitHub y tener permiso de escritura.")
		return
	}
	fmt.Printf("[OK] Rama subida. 'origin/%s' queda como upstream.\n", branch)
	if web := webURL(gitOut(ctx, "remote", "get-url", "origin")); web != "" {
		fmt.Printf("     Repositorio: %s\n", web)
	}
}

func createGithubRepo(ctx *project.ProjectContext) {
	fmt.Println()
	if !commandExists("gh") {
		fmt.Println("[WARN] `gh` (GitHub CLI) no esta instalado.")
		fmt.Println("       Usa la opcion 1 y pega la URL, o instala gh: https://cli.github.com")
		ui.Pause()
		return
	}
	if gitOut(ctx, "remote", "get-url", "origin") != "" {
		fmt.Println("[WARN] Ya existe un remoto 'origin'.")
		fmt.Println("       Borralo primero si quieres recrearlo: git remote remove origin")
		ui.Pause()
		return
	}

	defName := filepath.Base(ctx.ScriptDir)
	defName = strings.ReplaceAll(defName, " ", "-")
	name := ui.AskText("Nombre del repositorio en GitHub", defName)
	if strings.TrimSpace(name) == "" {
		fmt.Println("[INFO] Operacion cancelada.")
		ui.Pause()
		return
	}

	idx, err := prompts.Select("Visibilidad:", []string{"Privado", "Publico"})
	if err != nil {
		ui.Pause()
		return
	}
	vis := "--private"
	if idx == 1 {
		vis = "--public"
	}

	fmt.Println()
	fmt.Println("[INFO] Creando el repositorio y subiendo el codigo (gh repo create)...")
	args := []string{"repo", "create", name, "--source=.", "--remote=origin", vis, "--push"}
	if err := runCapture("gh", args, ctx.ScriptDir); err != nil {
		fmt.Printf("[ERROR] gh repo create fallo:\n%s\n", err)
		fmt.Println()
		fmt.Println("Pistas:")
		fmt.Println("  - ¿Sesion iniciada? Ejecuta: gh auth login")
		fmt.Println("  - ¿El nombre ya existe en tu cuenta? Prueba otro.")
	} else {
		fmt.Printf("[OK] Repositorio '%s' creado y codigo subido.\n", name)
	}
	ui.Pause()
}

func connectRemote(ctx *project.ProjectContext) error {
	shell.ClearScreen()
	ui.BoxBanner("CONECTAR CON REPOSITORIO REMOTO")
	fmt.Println()

	if !isRepo(ctx) || !hasCommits(ctx) {
		fmt.Println("[INFO] Este proyecto todavia no es un repositorio git con commits.")
		ok, _ := prompts.Confirm("¿Inicializarlo ahora (git init + primer commit)?", true)
		if !ok {
			fmt.Println("[INFO] Operacion cancelada.")
			ui.Pause()
			return nil
		}
		if !createRepo(ctx) {
			fmt.Println("[WARN] No se pudo preparar el repositorio local.")
			ui.Pause()
			return nil
		}
		fmt.Println()
	}

	existing := gitOut(ctx, "remote", "get-url", "origin")
	if existing != "" {
		fmt.Printf("[INFO] Ya hay un remoto 'origin': %s\n", existing)
	}

	var modes []string
	modes = append(modes, "1. Ya tengo el repositorio en GitHub (pego la URL)")
	if commandExists("gh") {
		modes = append(modes, "2. Crear un repositorio nuevo en GitHub (gh)")
	}

	idx, err := prompts.Select("¿Como quieres conectar?", modes)
	if err != nil {
		ui.Pause()
		return nil
	}
	if idx == 1 && len(modes) > 1 {
		createGithubRepo(ctx)
		return nil
	}

	url := ui.AskText("URL del repositorio (https://github.com/usuario/repo.git o git@github.com:usuario/repo.git)", existing)
	if strings.TrimSpace(url) == "" {
		fmt.Println("[INFO] No especificaste una URL.")
		ui.Pause()
		return nil
	}
	if !looksLikeGitURL(url) {
		fmt.Println("[WARN] La URL no parece de un repositorio git; se usara tal cual.")
	}

	switch {
	case existing == url:
		fmt.Println("[INFO] El remoto 'origin' ya apunta a esa URL.")
	case existing != "":
		fmt.Println("[INFO] Reemplazando la URL del remoto 'origin'...")
		if err := gitCapture(ctx, "remote", "set-url", "origin", url); err != nil {
			fmt.Printf("[ERROR] No se pudo actualizar el remoto: %v\n", err)
			ui.Pause()
			return nil
		}
		fmt.Println("[OK] Remoto 'origin' actualizado.")
	default:
		fmt.Println("[INFO] Agregando remoto 'origin'...")
		if err := gitCapture(ctx, "remote", "add", "origin", url); err != nil {
			fmt.Printf("[ERROR] No se pudo agregar el remoto: %v\n", err)
			ui.Pause()
			return nil
		}
		fmt.Println("[OK] Remoto 'origin' agregado.")
	}

	fmt.Println()
	pushCurrentBranch(ctx)

	fmt.Println()
	ui.Pause()
	return nil
}

// ── Estado ──────────────────────────────────────────────────────────────────

func showGitStatus(ctx *project.ProjectContext) error {
	shell.ClearScreen()
	ui.BoxBanner("ESTADO DEL REPOSITORIO")
	fmt.Println()

	if !isRepo(ctx) {
		fmt.Println("[INFO] No hay repositorio git en este proyecto.")
		ui.Pause()
		return nil
	}

	branch := gitOut(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if branch == "" {
		branch = "(sin commits)"
	}
	fmt.Printf("  Rama:    %s\n", branch)

	if remote := gitOut(ctx, "remote", "get-url", "origin"); remote != "" {
		fmt.Printf("  Remoto:  %s\n", remote)
	} else {
		fmt.Println("  Remoto:  (ninguno)")
	}

	commits := gitOut(ctx, "rev-list", "--count", "HEAD")
	if commits == "" {
		commits = "0"
	}
	fmt.Printf("  Commits: %s\n", commits)
	fmt.Println()
	fmt.Println("Cambios sin commitear:")

	status := gitOut(ctx, "status", "--short")
	if strings.TrimSpace(status) == "" {
		fmt.Println("  (arbol de trabajo limpio)")
	} else {
		for _, line := range strings.Split(status, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				fmt.Printf("  %s\n", line)
			}
		}
	}

	fmt.Println()
	ui.Pause()
	return nil
}

// ── .gitignore ──────────────────────────────────────────────────────────────

const (
	gitignoreFlutter = `# ============================================
# .gitignore para Flutter/Dart
# ============================================

# --- Flutter/Dart ---
build/
.dart_tool/
.packages
.flutter-plugins
.flutter-plugins-dependencies
coverage/
*.iml

# --- Salidas generadas por plataforma ---
windows/flutter/ephemeral/
linux/flutter/ephemeral/
macos/Flutter/ephemeral/
ios/Flutter/ephemeral/

# --- Artefactos de release generados por Compass (portable + instalador) ---
bin/

# --- Android (Gradle) ---
android/build/
android/.gradle/
android/.cxx/
android/app/build/
android/app/debug/
android/app/profile/
android/app/release/
android/local.properties
android/key.properties
*.jks
*.keystore

# --- Node.js ---
node_modules/

# --- ios ---
ios/.symlinks/
ios/Pods/
ios/Runner.xcworkspace/
ios/Flutter/Flutter.framework
ios/Flutter/Flutter.podspec

# --- Sistema operativo ---
.DS_Store
Thumbs.db
Desktop.ini
*.swp
*.swo
*~

# --- IDE ---
.vscode/
.idea/
*.sublime-project
*.sublime-workspace

# --- Variables de entorno ---
.env
.env.local
.env.*.local
!.env.example

# --- Logs ---
*.log
`

	gitignoreKotlin = `# ============================================
# .gitignore para Kotlin Multiplatform
# ============================================

# --- Gradle Build Artifacts ---
build/
.gradle/
local.properties
*.apk
*.aab
*.ap_
*.dex
composeApp/build/
composeApp/out/
composeApp/release/

# --- Kotlin ---
.kotlin/
.externalNativeBuild/
.cxx/
captures/
*.hprof

# --- Node / frontends embebidos ---
node_modules/

# --- Signing / Secrets (NO subir: contienen la clave privada) ---
composeApp/keystore.properties
*.keystore
*.jks
*.p12
*.pfx

# --- IDE ---
.idea/
*.iml
.vscode/
*.sublime-project
*.sublime-workspace

# --- OS ---
.DS_Store
Thumbs.db
Desktop.ini
*.swp
*.swo
*~

# --- Environment ---
.env
.env.local
.env.*.local
!.env.example

# --- Logs ---
*.log
`

	gitignoreGoWails = `# ============================================
# .gitignore para Go + Wails v3
# ============================================

# --- Binarios generados ---
bin/

# --- Firma de Android (keystore + contrasenas: NO subir) ---
android-signing/
keystore-backups/

# --- Node.js / frontend ---
node_modules/
frontend/node_modules/
**/frontend/dist/
frontend/bindings/
build/bin/

# --- Android (generado por 'wails3 update build-assets') ---
build/android/app/build/
build/android/.gradle/
build/android/app/src/main/jniLibs/
build/android/app/src/main/assets/
build/android/overlay.json
build/android/local.properties
build/android/gen/
*.apk
*.aab

# --- Go ---
*.syso
*.test
*.out
*.exe
*.dll
*.so
*.dylib
coverage.out

# --- IDE ---
.idea/
*.iml
.vscode/
*.sublime-project
*.sublime-workspace

# --- OS ---
.DS_Store
Thumbs.db
Desktop.ini
*.swp
*.swo
*~

# --- Environment ---
.env
.env.local
.env.*.local
!.env.example

# --- Logs ---
*.log
`

	gitignoreGo = `# ============================================
# .gitignore para Go
# ============================================

# --- Binarios y pruebas ---
bin/
*.exe
*.exe~
*.dll
*.so
*.dylib
*.test
*.out
*.syso
coverage.out
*.coverprofile

# --- Node / frontends embebidos (Wails, web, tooling) ---
node_modules/
frontend/node_modules/
**/frontend/dist/
frontend/bindings/

# --- Editor / entorno local ---
.vscode/
.idea/
.env
.env.*
!.env.example
*.log
`

	gitignorePython = `# ============================================
# .gitignore para Python
# ============================================

# --- Python bytecode y caches ---
__pycache__/
*.py[cod]
*$py.class
.pytest_cache/
.mypy_cache/
.ruff_cache/
.tox/
.ipynb_checkpoints/
.coverage
htmlcov/

# --- Entornos virtuales ---
.venv/
venv/
ENV/

# --- Build / packaging ---
build/
dist/
*.egg-info/
*.mo

# --- Node / frontends embebidos ---
node_modules/

# --- Secrets / local config ---
.env
.env.*
!.env.example

# --- IDE / OS / logs ---
.vscode/
.idea/
.DS_Store
Thumbs.db
*.log
`

	gitignoreWhatsappKit = `# ============================================
# .gitignore para el kit del motor de WhatsApp
# ============================================

# --- Dependencias Node ---
node_modules/
dist/
npm-debug.log*
yarn-error.log*

# --- Bundle compilado por ncc (bundle.mjs lo regenera) ---
whatsapp-engine-bundle/
single/
out/

# --- Runtime ---
# El motor usa el Node 20+ del sistema: nunca se versiona un node.exe.
node.exe

# --- Sesiones y credenciales de WhatsApp ---
whatsapp_auth/
.auth/
auth_info*/
*.session

# --- Config local ---
.env
.env.*
!.env.example

# --- Logs ---
*.log

# --- IDE / OS ---
.vscode/
.idea/
.DS_Store
Thumbs.db
`

	gitignoreKitD1 = `# ============================================
# .gitignore para el kit Cloudflare D1
# ============================================

# --- Dependencias Node (worker y scripts) ---
node_modules/
**/node_modules/
dist/

# --- Wrangler / Cloudflare ---
.wrangler/
**/.wrangler/
.dev.vars
.env
.env.*
!.env.example

# --- Credenciales del kit (NUNCA subir) ---
sync-keys.json
*-keys.json
*.key

# --- Bases de datos locales ---
*.db
*.sqlite
*.sqlite3

# --- Go (motor de sync) ---
*.exe
*.test
*.out
coverage.out
go.work
go.work.sum

# --- Logs / IDE / OS ---
*.log
.vscode/
.idea/
.DS_Store
Thumbs.db
`
)

func gitignoreContent(pt project.ProjectType) string {
	var content string
	switch pt {
	case project.TypeFlutter:
		content = gitignoreFlutter
	case project.TypeKotlin:
		content = gitignoreKotlin
	case project.TypeGoWails:
		content = gitignoreGoWails
	case project.TypeGo:
		content = gitignoreGo
	case project.TypePython:
		content = gitignorePython
	case project.TypeWhatsappKit:
		content = gitignoreWhatsappKit
	case project.TypeKitD1:
		content = gitignoreKitD1
	default:
		content = gitignoreGo
	}
	// Los corekits no etajan el motor de WhatsApp de otros proyectos: esas
	// reglas solo generarian ruido (resources/whatsapp-engine-bundle/).
	if pt.IsKit() {
		return withCommonRules(withTaskCacheRules(content))
	}
	return withWhatsappRules(withCommonRules(withTaskCacheRules(content)))
}

// whatsappStagedRules cubren el motor de WhatsApp que Compass etaja en cada
// build (node.exe + bundle). Se regenera desde el corekit, asi que nunca debe
// versionarse: son ~74 MB por proyecto.
var whatsappStagedRules = []string{
	"whatsapp_engine/",
	"resources/whatsapp-engine-bundle/",
	"composeApp/resources/whatsapp-engine-bundle/",
}

// withWhatsappRules anade las reglas del motor etajado que falten.
func withWhatsappRules(content string) string {
	var missing []string
	for _, rule := range whatsappStagedRules {
		if !hasIgnoreRule(content, rule) {
			missing = append(missing, rule)
		}
	}
	if len(missing) == 0 {
		return content
	}
	content = strings.TrimRight(content, "\n \t")
	content += "\n\n# --- Motor WhatsApp etajado por Compass (se regenera en cada build) ---\n"
	for _, rule := range missing {
		content += rule + "\n"
	}
	return content
}

// commonIgnoreRules ensucian el arbol en cualquier proyecto: dependencias Node
// (aunque el proyecto sea Go, si lleva UI web) y salidas de empaquetado.
var commonIgnoreRules = []string{"node_modules/", "dist/"}

// withCommonRules anade las reglas comunes que falten, sin duplicarlas.
func withCommonRules(content string) string {
	var missing []string
	for _, rule := range commonIgnoreRules {
		if !hasIgnoreRule(content, rule) {
			missing = append(missing, rule)
		}
	}
	if len(missing) == 0 {
		return content
	}
	content = strings.TrimRight(content, "\n \t")
	content += "\n\n# --- Dependencias Node y salidas de empaquetado ---\n"
	for _, rule := range missing {
		content += rule + "\n"
	}
	return content
}

// hasIgnoreRule indica si content ya contiene ese patron como regla propia.
func hasIgnoreRule(content, rule string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == rule {
			return true
		}
	}
	return false
}

// withTaskCacheRules anade el directorio de trabajo de go-task (Taskfile).
// Task guarda ahi los checksums de cada build (.task/checksum/...) y
// temporales; nunca contiene fuentes, asi que es seguro ignorarlo en
// cualquier tipo de proyecto (Wails v3 lo usa por defecto).
func withTaskCacheRules(content string) string {
	content = strings.TrimRight(content, "\n \t")
	content += "\n\n# --- Task (go-task): cache de checksums ---\n.task/\n"
	return content
}

func withWindowsRules(content string) string {
	content = strings.TrimRight(content, "\n \t")
	content += "\n\n# --- Windows ---\n$RECYCLE.BIN/\nehthumbs.db\n*.lnk\n"
	return content
}

func gitignoreContentForPlatform(pt project.ProjectType) string {
	return withWindowsRules(gitignoreContent(pt))
}

// mergeGitignore conserva el contenido existente y anade solo las reglas que
// faltan como patrones de ignore exactos.
func mergeGitignore(existing, required string) string {
	present := map[string]bool{}
	for _, line := range strings.Split(existing, "\n") {
		l := strings.TrimSpace(line)
		if l != "" && !strings.HasPrefix(l, "#") {
			present[l] = true
		}
	}
	var missing []string
	for _, line := range strings.Split(required, "\n") {
		l := strings.TrimSpace(line)
		if l != "" && !strings.HasPrefix(l, "#") && !present[l] {
			missing = append(missing, l)
		}
	}
	if len(missing) == 0 {
		return existing
	}
	merged := existing
	if merged != "" && !strings.HasSuffix(merged, "\n") {
		merged += "\n"
	}
	// La cabecera se escribe una sola vez; en actualizaciones posteriores solo
	// se anaden las reglas nuevas.
	if merged != "" && !strings.Contains(merged, mergeMarker) {
		merged += "\n" + mergeMarker + "\n"
	}
	for _, rule := range missing {
		merged += rule + "\n"
	}
	return merged
}

// mergeMarker separa las reglas que anade Compass de las que ya tenia el repo.
const mergeMarker = "# Compass project ignore rules"

func writeContextGitignore(root, required string) error {
	path := filepath.Join(root, ".gitignore")
	var existing string
	if data, err := os.ReadFile(path); err == nil {
		existing = string(data)
	}
	content := required
	if existing != "" {
		content = mergeGitignore(existing, required)
	}
	if existing == content {
		fmt.Printf("[OK] .gitignore ya contiene las reglas necesarias: %s\n", path)
		return nil
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("no se pudo escribir %s: %w", path, err)
	}
	fmt.Printf("[OK] .gitignore actualizado: %s\n", path)
	return nil
}

// addContextGitignore (tarea context-gitignore ya resuelta la carpeta).
func addContextGitignoreTask(ctx *project.ProjectContext) error {
	return writeContextGitignore(ctx.ScriptDir, gitignoreContentForPlatform(ctx.ProjectType))
}

// AddContextGitignoreFromDir escribe un .gitignore sensato para la carpeta dada
// subiendo hasta encontrar un proyecto reconocido.
//
// Si no hay ningun proyecto reconocido en la carpeta ni en sus padres, no falla:
// crea un .gitignore generico en la carpeta seleccionada (así el menu contextual
// siempre hace algo util, en lugar de mostrar un error).
func AddContextGitignoreFromDir(start string) error {
	info, err := os.Stat(start)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("la ruta no es una carpeta: %s", start)
	}
	dir := start
	for {
		if pt, ok := project.ClassifyDir(dir); ok {
			return writeContextGitignore(dir, gitignoreContentForPlatform(pt))
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	// No se encontro un proyecto reconocido: .gitignore generico en la carpeta.
	return writeContextGitignore(start, gitignoreContentForPlatform(project.TypeGo))
}

// ── Menú unificado de Git (submenu de cards) ───────────────────────────────

type gitItem struct {
	key  string
	card menu.SubItem
	run  func(*project.ProjectContext) error
}

func gitEntries() []gitItem {
	return []gitItem{
		{"init", menu.SubItem{
			Icon: "\U0001f4c1", Label: "Inicializar repositorio",
			Description: "git init + main + primer commit", ColorKey: "build",
		}, gitInit},
		{"remote", menu.SubItem{
			Icon: "\U0001f517", Label: "Conectar con remoto",
			Description: "Enlazar con GitHub (URL o gh)", ColorKey: "git",
		}, connectRemote},
		{"status", menu.SubItem{
			Icon: "\U0001f4ca", Label: "Ver estado",
			Description: "Rama, remoto y cambios", ColorKey: "debug",
		}, showGitStatus},
	}
}

// GitManage es el punto de entrada de las tareas git/git-init.
//
// Tras ejecutar una accion se vuelve al submenu (como en Rust): la accion
// imprime y pide Enter con la consola normal, y despues se repinta.
func GitManage(ctx *project.ProjectContext) error {
	entries := gitEntries()
	labels := make([]string, len(entries))
	for i, e := range entries {
		labels[i] = e.card.Label + " · " + e.card.Description
	}
	idx, err := prompts.Select("Git", labels)
	if err != nil || idx < 0 || idx >= len(entries) {
		return err
	}
	return entries[idx].run(ctx)
}
