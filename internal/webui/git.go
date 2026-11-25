package webui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"compass/internal/gitmsg"
	"compass/internal/settings"
	"compass/internal/sound"
	"compass/internal/windialog"
)

// errCommitCancelled es el error que recibe el frontend cuando el usuario pulsa
// Cancelar mientras se genera el mensaje de commit.
var errCommitCancelled = errors.New("generacion cancelada")

// Change es una linea de `git status --porcelain`.
type Change struct {
	Code   string `json:"code"`
	Path   string `json:"path"`
	Status string `json:"status"`
}

// GitInfo es el estado completo del repositorio que pinta el panel Git lateral.
type GitInfo struct {
	IsRepo     bool   `json:"isRepo"`
	HasCommits bool   `json:"hasCommits"`
	Branch     string `json:"branch"`    // "detached@<short>" si HEAD suelto
	Upstream   string `json:"upstream"`  // "origin/main"; "" si no hay
	RemoteURL  string `json:"remoteUrl"` // URL cruda de origin
	WebURL     string `json:"webUrl"`    // https://github.com/user/repo
	IsGitHub   bool   `json:"isGitHub"`
	Ahead      int    `json:"ahead"`
	Behind     int    `json:"behind"`
	Changes    int    `json:"changes"` // len(GitStatus())
}

// FileNode es un nodo del arbol de archivos del explorador.
type FileNode struct {
	Name string `json:"name"`
	Path string `json:"path"`
	// Rel es la ruta relativa a la raiz del proyecto, en formato "/" para
	// casarla con las rutas que devuelve `git status`.
	Rel   string `json:"rel"`
	IsDir bool   `json:"isDir"`
}

var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "bin": true, "obj": true,
	"dist": true, "vendor": true, "target": true, ".idea": true,
	".vs": true, "corekit": true, "out": true,
}

func runGit(root string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	windialog.NoWindow(cmd)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// runGitTimeout es runGit con plazo. Se usa solo para push/pull: un prompt de
// credenciales o SSH colgado no debe congelar para siempre la promesa Wails.
func runGitTimeout(root string, d time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	windialog.NoWindow(cmd)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return strings.TrimSpace(string(out)), fmt.Errorf("git %s: tiempo de espera agotado", args[0])
	}
	return strings.TrimSpace(string(out)), err
}

func statusLabel(code string) string {
	if len(code) < 2 {
		return "Changed"
	}
	switch {
	case strings.Contains(code, "?"):
		return "Untracked"
	case strings.Contains(code, "D"):
		return "Deleted"
	case strings.Contains(code, "A"):
		return "Added"
	case strings.Contains(code, "R"):
		return "Renamed"
	case strings.Contains(code, "M"):
		return "Modified"
	}
	return "Changed"
}

// projectDir devuelve la raiz del proyecto activo con el candado tomado.
func (b *Bridge) projectDir() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.dir
}

// GitStatus devuelve los cambios pendientes del proyecto activo.
func (b *Bridge) GitStatus() []Change {
	out := []Change{}
	dir := b.projectDir()
	if dir == "" {
		return out
	}
	raw, err := runGit(dir, "status", "--porcelain=v1", "-uall")
	if err != nil && raw == "" {
		return out
	}
	for _, line := range strings.Split(raw, "\n") {
		if len(line) < 4 {
			continue
		}
		code := line[:2]
		path := strings.TrimSpace(line[3:])
		// Los renombrados vienen como "old -> new".
		if i := strings.LastIndex(path, " -> "); i >= 0 {
			path = path[i+4:]
		}
		out = append(out, Change{Code: code, Path: path, Status: statusLabel(code)})
	}
	return out
}

// Limites del material que se envia al modelo: contexto suficiente para que el
// mensaje sea preciso sin desbordar la ventana de los modelos gratuitos.
const (
	maxStatusLines    = 120
	maxDiffChars      = 9000
	maxFileDiffChars  = 4000 // un solo archivo no puede acaparar el prompt
	maxUntrackedFiles = 4
	maxUntrackedLines = 50
)

// gitSummary arma el material del prompt: estado, diffs reales (con contexto) y
// un extracto de los archivos nuevos. Antes solo enviaba --stat y nombres de
// archivo, asi que el modelo no podia saber que cambio y escribia bullets
// genericos.
func gitSummary(root string) string {
	var parts []string

	if out, err := runGit(root, "status", "--porcelain=v1", "-uall"); err == nil && out != "" {
		if filtered := filterStatus(out); filtered != "" {
			parts = append(parts, "STATUS (porcelain):\n"+clipLines(filtered, maxStatusLines))
		}
	}

	diff := strings.TrimSpace(diffSection(root, false) + "\n" + diffSection(root, true))
	if diff != "" {
		parts = append(parts, "DIFF:\n"+clip(diff, maxDiffChars))
	}

	if news := newFileExcerpts(root, maxUntrackedFiles, maxUntrackedLines); news != "" {
		parts = append(parts, "NEW FILES (excerpt):\n"+news)
	}

	return strings.Join(parts, "\n\n")
}

// diffSection devuelve el diff de trabajo (o del index) archivo por archivo,
// omitiendo el contenido de artefactos generados (bundles, dist, minificados):
// ocupan todo el presupuesto y no dicen nada util sobre el cambio.
func diffSection(root string, staged bool) string {
	numstat := []string{"diff", "--numstat", "--no-color"}
	if staged {
		numstat = append(numstat, "--cached")
	}
	out, err := runGit(root, numstat...)
	if err != nil || strings.TrimSpace(out) == "" {
		return ""
	}

	var b strings.Builder
	for _, line := range strings.Split(out, "\n") {
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) != 3 {
			continue
		}
		added, deleted, path := fields[0], fields[1], strings.TrimSpace(fields[2])
		if path == "" {
			continue
		}
		if generatedPath(path) {
			b.WriteString("--- " + path + " (generado, contenido omitido)\n")
			continue
		}
		if added == "-" || deleted == "-" { // binario
			b.WriteString("--- " + path + " (binario)\n")
			continue
		}
		args := []string{"diff", "--no-color", "-U2"}
		if staged {
			args = append(args, "--cached")
		}
		args = append(args, "--", path)
		d, err := runGit(root, args...)
		if err != nil || d == "" {
			continue
		}
		b.WriteString(clip(stripGitWarnings(d), maxFileDiffChars))
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

// stripGitWarnings quita avisos de git (p.ej. CRLF) que no describen cambios.
func stripGitWarnings(s string) string {
	var keep []string
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "warning:") {
			continue
		}
		keep = append(keep, line)
	}
	return strings.Join(keep, "\n")
}

// filterStatus quita del porcelain los archivos generados (node_modules, dist,
// binarios...): si no, ocupan todo el listado y esconden los cambios reales.
func filterStatus(raw string) string {
	var keep []string
	skipped := 0
	for _, line := range strings.Split(raw, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if len(line) > 3 && generatedPath(strings.TrimSpace(line[3:])) {
			skipped++
			continue
		}
		keep = append(keep, line)
	}
	if skipped > 0 {
		keep = append(keep, fmt.Sprintf("... (%d archivos generados omitidos)", skipped))
	}
	return strings.Join(keep, "\n")
}

// generatedPath marca rutas cuyo diff es ruido (artefactos de build).
func generatedPath(p string) bool {
	lp := strings.ToLower(filepath.ToSlash(p))
	if strings.Contains(lp, "node_modules/") ||
		strings.Contains(lp, "/dist/") || strings.HasPrefix(lp, "dist/") ||
		strings.Contains(lp, "/build/") || strings.HasPrefix(lp, "build/") {
		return true
	}
	for _, ext := range []string{".min.js", ".min.css", ".map", ".lock", ".ico", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".woff", ".woff2", ".ttf", ".pdf"} {
		if strings.HasSuffix(lp, ext) {
			return true
		}
	}
	return strings.HasSuffix(lp, "package-lock.json")
}

// newFileExcerpts devuelve las primeras lineas de los archivos sin seguimiento
// (git diff no los muestra, asi que sin esto el modelo los ve solo por nombre).
func newFileExcerpts(root string, maxFiles, maxLines int) string {
	raw, err := runGit(root, "status", "--porcelain=v1", "-uall")
	if err != nil || raw == "" {
		return ""
	}
	var b strings.Builder
	found := 0
	for _, line := range strings.Split(raw, "\n") {
		if !strings.HasPrefix(line, "?? ") {
			continue
		}
		rel := strings.Trim(strings.TrimSpace(line[3:]), `"`)
		if rel == "" || strings.HasSuffix(rel, "/") || generatedPath(rel) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil || len(data) == 0 {
			continue
		}
		probe := data
		if len(probe) > 8000 {
			probe = probe[:8000]
		}
		if bytes.IndexByte(probe, 0) >= 0 || !utf8.ValidString(string(probe)) {
			continue // binario: no aporta al mensaje
		}
		b.WriteString("--- " + rel + "\n")
		b.WriteString(clipLines(string(data), maxLines))
		b.WriteString("\n")
		found++
		if found >= maxFiles {
			break
		}
	}
	return strings.TrimSpace(b.String())
}

// clip recorta s a max caracteres cortando por linea y sin romper UTF-8.
func clip(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := s[:max]
	if i := strings.LastIndex(cut, "\n"); i > max/2 {
		cut = cut[:i]
	}
	return strings.ToValidUTF8(cut, "") + "\n... (truncado)"
}

// clipLines deja las primeras max lineas de s.
func clipLines(s string, max int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= max {
		return s
	}
	return strings.Join(lines[:max], "\n") + "\n... (truncado)"
}

// GenerateCommit pide a OpenRouter un mensaje de commit para los cambios
// actuales. La key, el modelo y el endpoint se leen de las opciones globales.
//
// dir identifica el proyecto: cada generacion vive en su propio contexto
// cancelable, de modo que dos proyectos pueden generar a la vez y Cancelar solo
// afecta al proyecto indicado. Con dir vacio se usa el proyecto activo.
func (b *Bridge) GenerateCommit(dir string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		dir = b.projectDir()
	}
	if dir == "" {
		return "", errors.New("no hay proyecto activo")
	}
	summary := gitSummary(dir)
	if strings.TrimSpace(summary) == "" {
		return "", errors.New("no hay cambios para commitear")
	}
	cfg := settings.Load()
	provider := cfg.CommitProvider
	model := ""
	if provider != settings.ProviderCopilot {
		if strings.TrimSpace(cfg.OpenRouterAPIKey) == "" {
			return "", errors.New("falta la API key de OpenRouter: añádela en Opciones (openrouter.ai/keys)")
		}
		model = strings.TrimSpace(cfg.OpenRouterModel)
		if model == "" {
			// Sin modelo elegido: usa el gratuito mas rapido disponible.
			if opts := b.ListModels(); len(opts) > 0 {
				model = opts[0].ID
			}
		}
		if model == "" {
			return "", errors.New("falta el modelo de OpenRouter: elige uno en Opciones")
		}
	}
	prompt := "You write precise Conventional Commit messages from the git evidence below.\n" +
		"You cannot explore the repository: base EVERY line strictly on the diff, status and new-file excerpts.\n\n" +
		"Output format (exactly this, nothing else):\n" +
		"Line 1: <type>(<scope>): <imperative summary, max 72 chars, no trailing period>\n" +
		"Then 3 to 5 lines, each starting with \"- \", max 90 chars, describing ONE concrete change.\n\n" +
		"Rules:\n" +
		"- type is one of: feat, fix, refactor, perf, docs, test, build, ci, chore, style.\n" +
		"- scope is the affected area taken from the paths (e.g. webui, shell, tasks, frontend, installer).\n" +
		"- Each bullet must name what actually changed: function, type, struct field, flag, CSS class,\n" +
		"  UI text, endpoint or file. Say what it does now, not that it was \"updated\".\n" +
		"- BANNED generic phrasing: \"improve code\", \"update files\", \"various changes\",\n" +
		"  \"minor fixes\", \"refactor code\", \"enhance functionality\", \"make improvements\".\n" +
		"- If several files change for one reason, group them in a single bullet that names the reason.\n" +
		"- No blank lines, no markdown fences, no quotes, no preamble, no explanation.\n\n" +
		"Git evidence:\n" + summary

	// La peticion vive en un contexto cancelable para que el boton Cancelar
	// corte la espera al instante en vez de dejar el modal bloqueado. El
	// contexto es por proyecto: relanzar en A no cancela la generacion de B.
	ctx, cancel := context.WithCancel(context.Background())
	key := commitKey(dir)
	b.commitMu.Lock()
	if b.commitCancels == nil {
		b.commitCancels = map[string]*commitRun{}
	}
	if prev := b.commitCancels[key]; prev != nil {
		prev.cancel() // una generacion anterior del MISMO proyecto que quedo colgada
	}
	b.commitSeq++
	run := &commitRun{id: b.commitSeq, cancel: cancel}
	b.commitCancels[key] = run
	b.commitMu.Unlock()
	defer func() {
		cancel()
		b.commitMu.Lock()
		// Solo se borra la entrada si sigue siendo esta generacion: si el
		// usuario relanzo, la nueva entrada no se debe tocar.
		if cur := b.commitCancels[key]; cur != nil && cur.id == run.id {
			delete(b.commitCancels, key)
		}
		b.commitMu.Unlock()
	}()

	var out string
	var err error
	if provider == settings.ProviderCopilot {
		out, err = copilotCommitMessage(ctx, prompt, cfg.CopilotModel)
	} else {
		out, err = openRouterChat(ctx, cfg.OpenRouterEndpoint, cfg.OpenRouterAPIKey, model, prompt)
	}
	if err != nil {
		if ctx.Err() != nil {
			return "", errCommitCancelled
		}
		return "", err
	}
	msg := cleanCommitMessage(out)
	if strings.TrimSpace(msg) == "" {
		return "", errors.New("OpenRouter no devolvió un mensaje utilizable")
	}
	return msg, nil
}

// CancelCommit aborta la generacion de commit en curso del proyecto indicado,
// si la hay. La llamada a GenerateCommit de ESE proyecto termina con
// errCommitCancelled; las generaciones de otros proyectos siguen intactas.
// Con dir vacio se usa el proyecto activo.
func (b *Bridge) CancelCommit(dir string) {
	if strings.TrimSpace(dir) == "" {
		dir = b.projectDir()
	}
	b.commitMu.Lock()
	run := b.commitCancels[commitKey(dir)]
	b.commitMu.Unlock()
	if run != nil {
		run.cancel()
	}
}

// cleanCommitMessage descarta el ruido del CLI y deja cabecera + viñetas.
func cleanCommitMessage(raw string) string {
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		line = strings.TrimRight(line, " \t")
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, ">") || strings.HasPrefix(t, "✱") ||
			strings.HasPrefix(t, "→") || strings.HasPrefix(t, "←") ||
			strings.HasPrefix(t, "▸") || strings.HasPrefix(t, "```") ||
			strings.HasPrefix(strings.ToLower(t), "here is") {
			continue
		}
		if t == "" {
			if len(out) > 0 && out[len(out)-1] != "" {
				out = append(out, "")
			}
			continue
		}
		out = append(out, strings.Trim(line, "`\"' "))
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	if len(out) > 6 {
		out = out[:6] // cabecera + hasta 5 vinietas
	}
	for i, line := range out {
		if len(line) > 95 {
			out[i] = strings.ToValidUTF8(line[:95], "")
		}
	}
	return strings.Join(out, "\n")
}

// Commit stagea todo y hace el commit con el mensaje dado en el proyecto
// indicado (con dir vacio, el activo).
func (b *Bridge) Commit(dir, message string) (string, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return "", errors.New("el mensaje del commit está vacío")
	}
	if strings.TrimSpace(dir) == "" {
		dir = b.projectDir()
	}
	if dir == "" {
		return "", errors.New("no hay proyecto activo")
	}
	if out, err := runGit(dir, "add", "-A"); err != nil {
		sound.Play(sound.EventCommitError)
		return "", fmt.Errorf("git add: %s", out)
	}
	out, err := runGit(dir, "commit", "-m", message)
	if err != nil {
		sound.Play(sound.EventCommitError)
		return "", fmt.Errorf("git commit: %s", out)
	}
	sound.Play(sound.EventCommitOK)
	// El commit toca solo .git (excluido del vigilante): se olvida la firma
	// para que el siguiente guardado vuelva a avisar al panel.
	b.watchForget(dir)
	return out, nil
}

// ansiRe elimina secuencias de escape ANSI (algunos mensajes de commit las
// traen embebidas y se verian como basura en el historial).
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

// CommitEntry es una fila del historial de la rama actual.
type CommitEntry struct {
	Hash    string `json:"hash"`
	Short   string `json:"short"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	Subject string `json:"subject"`
}

// GitLog devuelve los ultimos commits de la rama actual (hasta 100).
// Usa separadores de control (%x1f / %x1e) para no romperse con asuntos que
// contengan comas o tabuladores.
func (b *Bridge) GitLog() []CommitEntry {
	dir := b.projectDir()
	if dir == "" {
		return nil
	}
	// Fecha y hora locales del commit, con el mismo formato que el historial.
	format := "%h%x1f%H%x1f%an%x1f%ad%x1f%s%x1e"
	out, err := runGit(dir, "log", "-n", "100", "--date=format:%Y-%m-%d %H:%M", "--pretty=format:"+format)
	if err != nil {
		return nil
	}
	var commits []CommitEntry
	for _, rec := range strings.Split(out, "\x1e") {
		rec = strings.TrimLeft(rec, "\r\n")
		if strings.TrimSpace(rec) == "" {
			continue
		}
		parts := strings.Split(rec, "\x1f")
		if len(parts) < 5 {
			continue
		}
		commits = append(commits, CommitEntry{
			Short:   parts[0],
			Hash:    parts[1],
			Author:  ansiRe.ReplaceAllString(parts[2], ""),
			Date:    parts[3],
			Subject: strings.TrimSpace(ansiRe.ReplaceAllString(parts[4], "")),
		})
	}
	return commits
}

// GitBranch devuelve la rama en la que esta el proyecto activo. Con HEAD
// desacoplado devuelve "detached@<short>" para que el explorador no muestre un
// "HEAD" ambiguo; sin repositorio (o sin proyecto) devuelve "".
func (b *Bridge) GitBranch() string {
	dir := b.projectDir()
	if dir == "" {
		return ""
	}
	return branchName(dir)
}

// branchName devuelve la rama de dir, con "detached@<short>" para HEAD suelto
// y "" si no hay repositorio. Lo comparten GitBranch y GitInfo.
func branchName(dir string) string {
	out, err := runGit(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return ""
	}
	if out == "HEAD" {
		if short, err := runGit(dir, "rev-parse", "--short", "HEAD"); err == nil && short != "" {
			return "detached@" + short
		}
		return "detached"
	}
	return out
}

// CommitFile es un archivo tocado por un commit, con su recuento de lineas.
type CommitFile struct {
	Path    string `json:"path"`
	Status  string `json:"status"`
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
	Binary  bool   `json:"binary"`
}

// CommitDetail es el contenido de un commit: cabecera, cuerpo y archivos.
type CommitDetail struct {
	Hash    string       `json:"hash"`
	Short   string       `json:"short"`
	Author  string       `json:"author"`
	Date    string       `json:"date"`
	Subject string       `json:"subject"`
	Body    string       `json:"body"`
	Files   []CommitFile `json:"files"`
}

// hashRe valida el hash que llega del frontend: se pasa como argumento de git,
// asi que no puede empezar por "-" ni traer nada que no sea hexadecimal.
var hashRe = regexp.MustCompile(`^[0-9a-fA-F]{4,40}$`)

// commitStatus traduce la letra de `--name-status` a la etiqueta del frontend.
func commitStatus(code string) string {
	if code == "" {
		return "Modified"
	}
	switch code[0] {
	case 'A':
		return "Added"
	case 'D':
		return "Deleted"
	case 'R':
		return "Renamed"
	case 'C':
		return "Copied"
	case 'T':
		return "Type"
	}
	return "Modified"
}

// CommitDetail devuelve que se commiteo en hash: mensaje completo y la lista de
// archivos con lineas añadidas/borradas. Se usan dos lecturas (name-status y
// numstat) porque git no da ambas en un formato estable de una sola pasada.
func (b *Bridge) CommitDetail(hash string) (CommitDetail, error) {
	dir := b.projectDir()
	if dir == "" {
		return CommitDetail{}, errors.New("no hay proyecto activo")
	}
	hash = strings.TrimSpace(hash)
	if !hashRe.MatchString(hash) {
		return CommitDetail{}, errors.New("hash de commit invalido")
	}

	header, err := runGit(dir, "log", "-1", "--no-color", "--date=format:%Y-%m-%d %H:%M",
		"--format=%H%x1f%h%x1f%an%x1f%ad%x1f%s%x1f%b%x1e", hash)
	if err != nil {
		return CommitDetail{}, fmt.Errorf("git log: %s", header)
	}
	fields := strings.SplitN(strings.TrimRight(header, "\x1e\r\n"), "\x1f", 6)
	if len(fields) < 5 {
		return CommitDetail{}, errors.New("no se pudo leer el commit")
	}
	detail := CommitDetail{
		Hash:    fields[0],
		Short:   fields[1],
		Author:  ansiRe.ReplaceAllString(fields[2], ""),
		Date:    fields[3],
		Subject: strings.TrimSpace(ansiRe.ReplaceAllString(fields[4], "")),
		Files:   []CommitFile{},
	}
	if len(fields) == 6 {
		detail.Body = strings.TrimSpace(ansiRe.ReplaceAllString(fields[5], ""))
	}

	// Estado por archivo (A/M/D/R...). En renombrados la ruta buena es la
	// ultima columna.
	statuses := map[string]string{}
	if out, err := runGit(dir, "log", "-1", "--name-status", "--format=", hash); err == nil {
		for _, line := range strings.Split(out, "\n") {
			cols := strings.Split(strings.TrimRight(line, "\r"), "\t")
			if len(cols) < 2 || strings.TrimSpace(cols[0]) == "" {
				continue
			}
			path := strings.TrimSpace(cols[len(cols)-1])
			if path != "" {
				statuses[path] = commitStatus(strings.TrimSpace(cols[0]))
			}
		}
	}

	if out, err := runGit(dir, "log", "-1", "--numstat", "--format=", hash); err == nil {
		for _, line := range strings.Split(out, "\n") {
			cols := strings.SplitN(strings.TrimRight(line, "\r"), "\t", 3)
			if len(cols) != 3 {
				continue
			}
			path := strings.TrimSpace(cols[2])
			if path == "" {
				continue
			}
			f := CommitFile{Path: path, Status: statuses[path]}
			if f.Status == "" {
				if strings.Contains(path, " => ") {
					f.Status = "Renamed"
				} else {
					f.Status = "Modified"
				}
			}
			if cols[0] == "-" || cols[1] == "-" {
				f.Binary = true
			} else {
				f.Added, _ = strconv.Atoi(cols[0])
				f.Removed, _ = strconv.Atoi(cols[1])
			}
			detail.Files = append(detail.Files, f)
		}
	}
	return detail, nil
}

// ── Panel Git lateral: estado y acciones ────────────────────────────────────

// GitInfo devuelve el estado completo del repo del proyecto activo para pintar
// el panel Git. Nunca falla: los campos que no se pueden leer quedan vacios.
func (b *Bridge) GitInfo() GitInfo {
	info := GitInfo{}
	dir := b.projectDir()
	if dir == "" {
		return info
	}
	if out, err := runGit(dir, "rev-parse", "--is-inside-work-tree"); err == nil && strings.TrimSpace(out) == "true" {
		info.IsRepo = true
	}
	if !info.IsRepo {
		return info
	}
	if _, err := runGit(dir, "rev-parse", "--verify", "HEAD"); err == nil {
		info.HasCommits = true
	}
	info.Branch = branchName(dir)
	if remote, err := runGit(dir, "remote", "get-url", "origin"); err == nil {
		info.RemoteURL = remote
	}
	info.WebURL = gitWebURL(info.RemoteURL)
	info.IsGitHub = gitHostIsGitHub(gitHost(info.RemoteURL))
	// @{u} es una revision fija de git, no texto del usuario.
	if out, err := runGit(dir, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"); err == nil {
		info.Upstream = strings.TrimSpace(out)
	}
	if info.Upstream != "" {
		if out, err := runGit(dir, "rev-list", "--left-right", "--count", "HEAD...@{u}"); err == nil {
			info.Ahead, info.Behind = parseAheadBehind(out)
		}
	}
	info.Changes = len(b.GitStatus())
	return info
}

// GitPush sube la rama actual. Con upstream, hace `git push` (como VSCode); sin
// el, fija el upstream con `git push -u origin HEAD`. No hace nada sin remoto.
func (b *Bridge) GitPush() (string, error) {
	dir := b.projectDir()
	if dir == "" {
		return "", errors.New("no hay proyecto activo")
	}
	info := b.GitInfo()
	if !info.IsRepo {
		return "", errors.New("no es un repositorio git")
	}
	if !info.HasCommits {
		return "", errors.New("el repo no tiene commits")
	}
	if info.RemoteURL == "" {
		return "", errors.New("sin remoto 'origin': configúralo antes de hacer push")
	}
	if strings.HasPrefix(info.Branch, "detached") {
		return "", errors.New("HEAD desacoplado: no se puede hacer push")
	}

	var out string
	var err error
	if info.Upstream == "" {
		out, err = runGitTimeout(dir, 90*time.Second, "push", "-u", "origin", "HEAD")
	} else {
		out, err = runGitTimeout(dir, 90*time.Second, "push")
	}
	if err != nil {
		return "", pushError(out)
	}
	return out, nil
}

// pushError traduce un push fallido. Un 403 suele ser otra cuenta guardada en
// el credential helper: se explica y se remite al modal que tiene el
// login/logout como botones.
func pushError(out string) error {
	if d, ok := gitmsg.ParseDenied(out); ok {
		return fmt.Errorf("GitHub autenticó como %q, que no tiene permiso de escritura sobre %q.\n\nCámbiala en Ajustes de Git → Cuenta de GitHub.", d.Account, d.Repo)
	}
	if gitmsg.IsForbidden(out) {
		return errors.New("GitHub rechazó el push (403): la credencial guardada no tiene permiso de escritura sobre este repositorio.\n\nRevisa la cuenta en Ajustes de Git → Cuenta de GitHub.")
	}
	return fmt.Errorf("git push: %s", out)
}

// GitSync hace pull --rebase y despues push en una sola llamada. Reutiliza una
// unica deteccion de GitInfo (siete procesos git) en vez de repetirla para el
// pull y para el push, que es lo que hacia lenta la sincronizacion.
func (b *Bridge) GitSync() (string, error) {
	dir := b.projectDir()
	if dir == "" {
		return "", errors.New("no hay proyecto activo")
	}
	info := b.GitInfo()
	if !info.IsRepo {
		return "", errors.New("no es un repositorio git")
	}
	if !info.HasCommits {
		return "", errors.New("el repo no tiene commits")
	}
	if info.RemoteURL == "" {
		return "", errors.New("sin remoto 'origin': configúralo antes de sincronizar")
	}
	if strings.HasPrefix(info.Branch, "detached") {
		return "", errors.New("HEAD desacoplado: no se puede sincronizar")
	}
	if info.Upstream == "" {
		return "", errors.New("la rama no tiene upstream: no se puede sincronizar")
	}

	pulled, err := runGitTimeout(dir, 90*time.Second, "pull", "--rebase")
	if err != nil {
		return "", fmt.Errorf("git pull: %s", pulled)
	}
	pushed, err := runGitTimeout(dir, 90*time.Second, "push")
	if err != nil {
		return "", pushError(pushed)
	}
	return strings.TrimSpace(strings.TrimSpace(pulled) + "\n" + strings.TrimSpace(pushed)), nil
}

// GitPull trae cambios del upstream con rebase. Lo compone el frontend con
// GitPush para la accion "Sincronizar".
func (b *Bridge) GitPull() (string, error) {
	dir := b.projectDir()
	if dir == "" {
		return "", errors.New("no hay proyecto activo")
	}
	info := b.GitInfo()
	if !info.IsRepo {
		return "", errors.New("no es un repositorio git")
	}
	if info.Upstream == "" {
		return "", errors.New("la rama no tiene upstream: no se puede sincronizar")
	}
	out, err := runGitTimeout(dir, 90*time.Second, "pull", "--rebase")
	if err != nil {
		return "", fmt.Errorf("git pull: %s", out)
	}
	return out, nil
}

// GitSetRemote agrega o actualiza el remoto 'origin'. Valida la URL antes de
// pasarla a git: sin opciones que empiecen por '-' ni saltos de linea.
func (b *Bridge) GitSetRemote(remoteURL string) (string, error) {
	dir := b.projectDir()
	if dir == "" {
		return "", errors.New("no hay proyecto activo")
	}
	if out, err := runGit(dir, "rev-parse", "--is-inside-work-tree"); err != nil || strings.TrimSpace(out) != "true" {
		return "", errors.New("no es un repositorio git: inicialízalo primero")
	}
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return "", errors.New("la URL del remoto está vacía")
	}
	if strings.HasPrefix(remoteURL, "-") {
		return "", errors.New("URL no válida: no puede empezar por guion")
	}
	if strings.ContainsAny(remoteURL, "\r\n") {
		return "", errors.New("URL no válida: contiene saltos de línea")
	}
	if !gitLooksLikeURL(remoteURL) {
		return "", errors.New("usa https://... o git@github.com:usuario/repo.git")
	}

	existing := ""
	if remote, err := runGit(dir, "remote", "get-url", "origin"); err == nil {
		existing = remote
	}
	args := []string{"remote", "add", "origin", remoteURL}
	verb := "agregado"
	if strings.TrimSpace(existing) != "" {
		args = []string{"remote", "set-url", "origin", remoteURL}
		verb = "actualizado"
	}
	out, err := runGit(dir, args...)
	if err != nil {
		return "", fmt.Errorf("git remote: %s", out)
	}
	return "Remoto 'origin' " + verb + ": " + remoteURL, nil
}

// GitInitRepo inicializa el repo, crea la rama main y hace el primer commit.
// Idempotente: si ya hay commits no toca nada.
func (b *Bridge) GitInitRepo() (string, error) {
	dir := b.projectDir()
	if dir == "" {
		return "", errors.New("no hay proyecto activo")
	}
	isRepo := false
	if out, err := runGit(dir, "rev-parse", "--is-inside-work-tree"); err == nil && strings.TrimSpace(out) == "true" {
		isRepo = true
	}
	hasCommits := false
	if isRepo {
		if _, err := runGit(dir, "rev-parse", "--verify", "HEAD"); err == nil {
			hasCommits = true
		}
	}
	if isRepo && hasCommits {
		return "El repositorio ya tiene commits; no se hizo ningún cambio.", nil
	}
	if !isRepo {
		if out, err := runGit(dir, "init"); err != nil {
			return "", fmt.Errorf("git init: %s", out)
		}
	}
	// Fijar la rama main. Sin commits, HEAD es una rama "unborn" y la forma
	// fiable de nombrarla es symbolic-ref; con commits, branch -M renombra.
	if !hasCommits {
		_, _ = runGit(dir, "symbolic-ref", "HEAD", "refs/heads/main")
	} else if b := branchName(dir); b != "main" && b != "" && !strings.HasPrefix(b, "detached") {
		_, _ = runGit(dir, "branch", "-M", "main")
	}
	if out, err := runGit(dir, "add", "-A"); err != nil {
		return "", fmt.Errorf("git add: %s", out)
	}
	out, err := runGit(dir, "commit", "-m", "First Commit")
	if err != nil {
		low := strings.ToLower(out)
		if strings.Contains(low, "nothing to commit") || strings.Contains(low, "no changes") {
			return "No había cambios que commitear.", nil
		}
		if strings.Contains(low, "who you are") || strings.Contains(low, "user.email") || strings.Contains(low, "user.name") {
			return "", errors.New("configura tu identidad de git: git config --global user.name \"Tu Nombre\" y user.email \"tu@email.com\"")
		}
		return "", fmt.Errorf("git commit: %s", out)
	}
	// Ya es repo: a partir de ahora el panel Git detecta cambios al instante.
	b.ensureWatch(dir)
	return out, nil
}

// OpenExternal abre un enlace en el navegador del sistema. Solo http/https.
func (b *Bridge) OpenExternal(rawURL string) error {
	u := strings.TrimSpace(rawURL)
	if u == "" || strings.ContainsAny(u, " \t\r\n") {
		return errors.New("URL no válida")
	}
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return errors.New("solo se pueden abrir URLs http/https")
	}
	if _, err := url.Parse(u); err != nil {
		return err
	}
	if app == nil {
		return nil
	}
	return app.Browser.OpenURL(u)
}

// ── helpers de URL de remoto ────────────────────────────────────────────────

func gitLooksLikeURL(s string) bool {
	u := strings.TrimSpace(s)
	ok := strings.HasPrefix(u, "https://") ||
		strings.HasPrefix(u, "http://") ||
		strings.HasPrefix(u, "git@") ||
		strings.HasPrefix(u, "ssh://")
	return ok && (strings.Contains(u, "/") || strings.Contains(u, ":"))
}

// gitHost extrae el host de un remoto git (git@host:path o scheme://host/path).
func gitHost(remote string) string {
	r := strings.TrimSpace(remote)
	if r == "" {
		return ""
	}
	if rest := strings.TrimPrefix(r, "git@"); rest != r {
		if host, _, ok := strings.Cut(rest, ":"); ok {
			return strings.ToLower(host)
		}
	}
	if u, err := url.Parse(r); err == nil && u.Hostname() != "" {
		return strings.ToLower(u.Hostname())
	}
	return ""
}

// gitWebURL convierte un remoto git en su URL web (sin el sufijo .git).
func gitWebURL(remote string) string {
	r := strings.TrimSpace(remote)
	if r == "" {
		return ""
	}
	if rest := strings.TrimPrefix(r, "git@"); rest != r {
		if host, path, ok := strings.Cut(rest, ":"); ok {
			return fmt.Sprintf("https://%s/%s", host, strings.TrimSuffix(path, ".git"))
		}
	}
	if strings.HasPrefix(r, "http://") || strings.HasPrefix(r, "https://") {
		return strings.TrimSuffix(r, ".git")
	}
	if strings.HasPrefix(r, "ssh://") {
		if u, err := url.Parse(r); err == nil && u.Hostname() != "" {
			return fmt.Sprintf("https://%s%s", u.Hostname(), strings.TrimSuffix(u.Path, ".git"))
		}
	}
	return ""
}

func gitHostIsGitHub(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	return h == "github.com" || h == "www.github.com"
}

// parseAheadBehind lee "1\t2" de rev-list --left-right --count (left=HEAD).
func parseAheadBehind(out string) (int, int) {
	fields := strings.Fields(out)
	if len(fields) < 2 {
		return 0, 0
	}
	ahead, errA := strconv.Atoi(fields[0])
	behind, errB := strconv.Atoi(fields[1])
	if errA != nil || errB != nil {
		return 0, 0
	}
	return ahead, behind
}
