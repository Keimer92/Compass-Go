package webui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func initRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git no esta en el PATH")
	}
	// Aislar el workspace persistido: los tests no deben apilar proyectos en
	// el %LOCALAPPDATA% real del usuario.
	t.Setenv("LOCALAPPDATA", t.TempDir())
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module demo\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return dir
}

func TestGitStatusParsesPorcelain(t *testing.T) {
	dir := initRepo(t)
	b := NewBridge(dir)
	if b.ctx == nil {
		t.Fatal("no se detecto el proyecto")
	}

	if err := os.WriteFile(filepath.Join(dir, "nuevo.txt"), []byte("hola"), 0o644); err != nil {
		t.Fatal(err)
	}
	changes := b.GitStatus()
	if len(changes) == 0 {
		t.Fatal("esperaba cambios en el repo recien creado")
	}
	var found bool
	for _, c := range changes {
		if c.Path == "nuevo.txt" {
			found = true
			if c.Status != "Untracked" {
				t.Fatalf("estado inesperado: %+v", c)
			}
		}
	}
	if !found {
		t.Fatalf("nuevo.txt no aparece en los cambios: %+v", changes)
	}
}

func TestGitSummaryIncludesDiffsAndNewFiles(t *testing.T) {
	dir := initRepo(t)
	main := filepath.Join(dir, "main.go")
	if err := os.WriteFile(main, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", "init"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}

	// Un archivo modificado (va en el diff) y otro nuevo (solo en el extracto).
	if err := os.WriteFile(main, []byte("package main\n\nfunc NuevaFuncion() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nuevo.go"), []byte("package main\n\nfunc HelperNuevo() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := gitSummary(dir)
	if !strings.Contains(got, "NuevaFuncion") {
		t.Fatalf("el resumen deberia incluir el diff del archivo modificado:\n%s", got)
	}
	if !strings.Contains(got, "HelperNuevo") {
		t.Fatalf("el resumen deberia incluir un extracto del archivo nuevo:\n%s", got)
	}
}

func TestListDirSkipsHeavyDirs(t *testing.T) {
	dir := initRepo(t)
	b := NewBridge(dir)
	if err := os.MkdirAll(filepath.Join(dir, "node_modules", "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	nodes := b.ListDir("")
	for _, n := range nodes {
		if n.Name == "node_modules" {
			t.Fatal("node_modules no deberia aparecer en el arbol")
		}
	}
	found := false
	for _, n := range nodes {
		if n.Name == "src" && n.IsDir {
			found = true
		}
	}
	if !found {
		t.Fatalf("arbol inesperado: %+v", nodes)
	}
	// El arbol es perezoso: los hijos se piden al desplegar la carpeta.
	children := b.ListDir(filepath.Join(dir, "src"))
	if len(children) != 1 || children[0].Name != "main.go" {
		t.Fatalf("hijos inesperados de src: %+v", children)
	}
}

func TestSubcards(t *testing.T) {
	dir := initRepo(t)
	b := NewBridge(dir)
	if got := b.Subcards("0"); len(got) != 0 {
		t.Fatalf("la card Run no deberia tener subcards: %+v", got)
	}
	// Git ya no tiene card ni subcards: se gestiona en la pestana Git.
	if got := b.Subcards("14"); len(got) != 0 {
		t.Fatalf("la card Git ya no deberia tener subcards: %+v", got)
	}
	if got := b.Subcards("16"); len(got) == 0 { // Environment
		t.Fatal("la card Environment deberia tener subcards")
	}
	// Vibe Coding ya no tiene subcards: cada CLI es una card propia.
	if got := b.Subcards("17"); len(got) != 0 {
		t.Fatalf("la card Vibe Coding ya no deberia tener subcards: %+v", got)
	}
}

func gitCommit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
}

func TestGitBranchReportsCurrentBranch(t *testing.T) {
	dir := initRepo(t)
	b := NewBridge(dir)
	// Un repo recien creado puede no tener rama hasta el primer commit.
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommit(t, dir, "add", "-A")
	gitCommit(t, dir, "commit", "-q", "-m", "init")
	if got := b.GitBranch(); got == "" || got == "HEAD" || strings.HasPrefix(got, "detached") {
		t.Fatalf("rama inesperada: %q", got)
	}
}

func TestCommitDetailListsFilesAndCounts(t *testing.T) {
	dir := initRepo(t)
	b := NewBridge(dir)
	if err := os.WriteFile(filepath.Join(dir, "uno.txt"), []byte("uno\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommit(t, dir, "add", "-A")
	gitCommit(t, dir, "commit", "-q", "-m", "feat: primer archivo", "-m", "cuerpo del commit")
	// Segundo commit que modifica el archivo y añade otro.
	if err := os.WriteFile(filepath.Join(dir, "uno.txt"), []byte("uno\ndos\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dos.txt"), []byte("nuevo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommit(t, dir, "add", "-A")
	gitCommit(t, dir, "commit", "-q", "-m", "feat: segundo commit")

	log := b.GitLog()
	if len(log) < 2 {
		t.Fatalf("esperaba al menos dos commits: %+v", log)
	}
	detail, err := b.CommitDetail(log[0].Hash)
	if err != nil {
		t.Fatalf("CommitDetail: %v", err)
	}
	if !strings.Contains(detail.Subject, "segundo commit") {
		t.Fatalf("asunto inesperado: %q", detail.Subject)
	}
	if len(detail.Files) != 2 {
		t.Fatalf("esperaba 2 archivos, hay %d: %+v", len(detail.Files), detail.Files)
	}
	byPath := map[string]CommitFile{}
	for _, f := range detail.Files {
		byPath[f.Path] = f
	}
	if f, ok := byPath["dos.txt"]; !ok || f.Status != "Added" || f.Added != 1 {
		t.Fatalf("dos.txt inesperado: %+v", byPath)
	}
	if f, ok := byPath["uno.txt"]; !ok || f.Status != "Modified" || f.Added != 1 {
		t.Fatalf("uno.txt inesperado: %+v", byPath)
	}
}

func TestCommitDetailRejectsBadHash(t *testing.T) {
	dir := initRepo(t)
	b := NewBridge(dir)
	if _, err := b.CommitDetail("--upload-pack=evil"); err == nil {
		t.Fatal("un hash que empieza por guion debe rechazarse")
	}
}

func TestGitInfoNoRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git no esta en el PATH")
	}
	t.Setenv("LOCALAPPDATA", t.TempDir())
	dir := t.TempDir()
	t.Chdir(dir)
	b := NewBridge(dir)
	info := b.GitInfo()
	if info.IsRepo || info.HasCommits || info.Changes != 0 {
		t.Fatalf("info inesperada sin repo: %+v", info)
	}
}

func TestGitInfoWithCommitNoRemote(t *testing.T) {
	dir := initRepo(t)
	b := NewBridge(dir)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommit(t, dir, "add", "-A")
	gitCommit(t, dir, "commit", "-q", "-m", "init")

	info := b.GitInfo()
	if !info.IsRepo || !info.HasCommits {
		t.Fatalf("deberia ser repo con commits: %+v", info)
	}
	if info.Branch == "" {
		t.Fatalf("rama vacia: %+v", info)
	}
	if info.RemoteURL != "" || info.WebURL != "" || info.Upstream != "" || info.IsGitHub {
		t.Fatalf("sin remoto deberia estar todo vacio: %+v", info)
	}
	if info.Ahead != 0 || info.Behind != 0 {
		t.Fatalf("sin upstream ahead/behind deberian ser 0: %+v", info)
	}
}

func TestGitInfoGitHubRemote(t *testing.T) {
	dir := initRepo(t)
	b := NewBridge(dir)

	for _, tc := range []struct {
		remote   string
		web      string
		isGitHub bool
	}{
		{"git@github.com:user/repo.git", "https://github.com/user/repo", true},
		{"https://github.com/u/r.git", "https://github.com/u/r", true},
		{"https://gitlab.com/u/r.git", "https://gitlab.com/u/r", false},
	} {
		// remove puede fallar si aun no existe origin (1a iteracion): se tolera.
		rm := exec.Command("git", "remote", "remove", "origin")
		rm.Dir = dir
		_ = rm.Run()
		gitCommit(t, dir, "remote", "add", "origin", tc.remote)
		info := b.GitInfo()
		if info.WebURL != tc.web {
			t.Errorf("remote %q: WebURL=%q, esperaba %q", tc.remote, info.WebURL, tc.web)
		}
		if info.IsGitHub != tc.isGitHub {
			t.Errorf("remote %q: IsGitHub=%v, esperaba %v", tc.remote, info.IsGitHub, tc.isGitHub)
		}
	}
}

func TestParseAheadBehind(t *testing.T) {
	ahead, behind := parseAheadBehind("1\t2")
	if ahead != 1 || behind != 2 {
		t.Fatalf("parseo ahead/behind: %d/%d", ahead, behind)
	}
	ahead, behind = parseAheadBehind("basura")
	if ahead != 0 || behind != 0 {
		t.Fatalf("entrada invalida deberia dar 0/0: %d/%d", ahead, behind)
	}
}

func TestGitPushNoRemote(t *testing.T) {
	dir := initRepo(t)
	b := NewBridge(dir)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommit(t, dir, "add", "-A")
	gitCommit(t, dir, "commit", "-q", "-m", "init")

	_, err := b.GitPush()
	if err == nil || !strings.Contains(err.Error(), "remoto") {
		t.Fatalf("sin remoto deberia fallar menciona 'remoto': %v", err)
	}
}

func TestGitSyncRequiresRemoteAndUpstream(t *testing.T) {
	dir := initRepo(t)
	b := NewBridge(dir)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommit(t, dir, "add", "-A")
	gitCommit(t, dir, "commit", "-q", "-m", "init")

	if _, err := b.GitSync(); err == nil || !strings.Contains(err.Error(), "remoto") {
		t.Fatalf("sin remoto deberia fallar mencionando 'remoto': %v", err)
	}

	// Con remoto pero sin upstream, el pull --rebase no tiene contra que
	// comparar: se rechaza antes de tocar la red.
	gitCommit(t, dir, "remote", "add", "origin", "https://github.com/user/repo.git")
	if _, err := b.GitSync(); err == nil || !strings.Contains(err.Error(), "upstream") {
		t.Fatalf("sin upstream deberia fallar mencionando 'upstream': %v", err)
	}
}

func TestGitSetRemoteRejectsBadURL(t *testing.T) {
	dir := initRepo(t)
	b := NewBridge(dir)

	for _, bad := range []string{"", "--upload-pack=evil", "https://x\nmal", "ftp://ejemplo.com/repo.git"} {
		if _, err := b.GitSetRemote(bad); err == nil {
			t.Errorf("URL invalida %q deberia rechazarse", bad)
		}
	}

	const good = "https://github.com/user/repo.git"
	if _, err := b.GitSetRemote(good); err != nil {
		t.Fatalf("URL valida rechazada: %v", err)
	}
	if got, _ := runGit(dir, "remote", "get-url", "origin"); got != good {
		t.Fatalf("remoto no persistio: %q", got)
	}
	const good2 = "https://github.com/user/otro.git"
	if _, err := b.GitSetRemote(good2); err != nil {
		t.Fatalf("actualizacion de URL fallo: %v", err)
	}
	if got, _ := runGit(dir, "remote", "get-url", "origin"); got != good2 {
		t.Fatalf("remoto no se actualizo: %q", got)
	}
}

func TestGitInitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git no esta en el PATH")
	}
	t.Setenv("LOCALAPPDATA", t.TempDir())
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module demo\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := NewBridge(dir)
	if _, err := b.GitInitRepo(); err != nil {
		t.Fatalf("GitInitRepo fallo: %v", err)
	}
	info := b.GitInfo()
	if !info.IsRepo || !info.HasCommits {
		t.Fatalf("deberia tener repo con commits: %+v", info)
	}
	if info.Branch != "main" {
		t.Fatalf("rama deberia ser main, es %q", info.Branch)
	}
	// Segunda llamada: idempotente, sin error.
	if _, err := b.GitInitRepo(); err != nil {
		t.Fatalf("GitInitRepo 2a llamada fallo: %v", err)
	}
}

func TestOpenExternalRejectsNonHTTP(t *testing.T) {
	b := &Bridge{}
	for _, bad := range []string{"file:///C:/x", "javascript:alert(1)", "", "https://x y"} {
		if err := b.OpenExternal(bad); err == nil {
			t.Errorf("OpenExternal deberia rechazar %q", bad)
		}
	}
}
