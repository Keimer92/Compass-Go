package webui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"compass/internal/settings"
	"compass/internal/windialog"
)

// Change es una linea de `git status --porcelain`.
type Change struct {
	Code   string `json:"code"`
	Path   string `json:"path"`
	Status string `json:"status"`
}

// FileNode es un nodo del arbol de archivos del explorador.
type FileNode struct {
	Name     string      `json:"name"`
	Path     string      `json:"path"`
	IsDir    bool        `json:"isDir"`
	Children []*FileNode `json:"children,omitempty"`
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

// FileTree devuelve el arbol del proyecto (dos niveles) para el explorador.
func (b *Bridge) FileTree() []*FileNode {
	dir := b.projectDir()
	if dir == "" {
		return []*FileNode{}
	}
	return buildTree(dir, 0, 2)
}

func buildTree(dir string, depth, maxDepth int) []*FileNode {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})
	out := []*FileNode{}
	for _, ent := range entries {
		name := ent.Name()
		if ent.IsDir() && skipDirs[name] {
			continue
		}
		if strings.HasPrefix(name, ".") && name != ".gitignore" {
			continue
		}
		node := &FileNode{
			Name:  name,
			Path:  filepath.Join(dir, name),
			IsDir: ent.IsDir(),
		}
		if node.IsDir && depth+1 < maxDepth {
			node.Children = buildTree(node.Path, depth+1, maxDepth)
		}
		out = append(out, node)
	}
	return out
}

// gitSummary resume los cambios para el prompt de opencode.
func gitSummary(root string) string {
	var parts []string
	for _, args := range [][]string{
		{"diff", "--stat", "--no-color"},
		{"diff", "--cached", "--stat", "--no-color"},
		{"status", "--porcelain=v1", "-uall"},
	} {
		if out, err := runGit(root, args...); err == nil && out != "" {
			parts = append(parts, out)
		}
	}
	s := strings.Join(parts, "\n")
	if len(s) > 4000 {
		s = s[:4000]
	}
	return s
}

// GenerateCommit pide a opencode un mensaje de commit para los cambios actuales.
func (b *Bridge) GenerateCommit() (string, error) {
	dir := b.projectDir()
	if dir == "" {
		return "", errors.New("no hay proyecto activo")
	}
	summary := gitSummary(dir)
	if strings.TrimSpace(summary) == "" {
		return "", errors.New("no hay cambios para commitear")
	}
	if _, err := exec.LookPath("opencode"); err != nil {
		return "", errors.New("opencode no está en el PATH")
	}
	model := strings.TrimSpace(settings.Load().CommitModel)
	if model == "" {
		model = settings.Default().CommitModel
	}
	prompt := "Write a git commit message in ENGLISH. Do NOT explore the repository.\n" +
		"Use ONLY the change list below. Output exactly this format:\n" +
		"Line 1: type(scope): imperative summary (max 72 chars).\n" +
		"Then 3 or 4 short bullet lines starting with '- ' (max 90 chars each),\n" +
		"each describing a concrete change from the list (mention filenames when useful).\n" +
		"No blank lines, no markdown, no quotes, no explanation.\n\n" +
		"Change list:\n" + summary

	cmd := exec.Command("opencode", "run", "-m", model, prompt)
	windialog.NoWindow(cmd)
	cmd.Dir = dir

	done := make(chan struct{})
	var out []byte
	var err error
	go func() {
		out, err = cmd.CombinedOutput()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(90 * time.Second):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return "", errors.New("OpenCode tardó demasiado en responder")
	}
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if len(detail) > 200 {
			detail = detail[len(detail)-200:]
		}
		return "", fmt.Errorf("opencode falló: %v\n%s", err, detail)
	}
	msg := cleanCommitMessage(string(out))
	if strings.TrimSpace(msg) == "" {
		return "", errors.New("opencode no devolvió un mensaje utilizable")
	}
	return msg, nil
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
	if len(out) > 5 {
		out = out[:5]
	}
	for i, line := range out {
		if len(line) > 95 {
			out[i] = line[:95]
		}
	}
	return strings.Join(out, "\n")
}

// Commit stagea todo y hace el commit con el mensaje dado.
func (b *Bridge) Commit(message string) (string, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return "", errors.New("el mensaje del commit está vacío")
	}
	dir := b.projectDir()
	if dir == "" {
		return "", errors.New("no hay proyecto activo")
	}
	if out, err := runGit(dir, "add", "-A"); err != nil {
		return "", fmt.Errorf("git add: %s", out)
	}
	out, err := runGit(dir, "commit", "-m", message)
	if err != nil {
		return "", fmt.Errorf("git commit: %s", out)
	}
	return out, nil
}
