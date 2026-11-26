package webui

import (
	"testing"

	"compass/internal/workspace"
)

// Las insignias de las cards se calculan con comandos git locales, sin mirar
// el proyecto activo ni consultar la red.
func TestProjectRepos(t *testing.T) {
	dir := initRepo(t)
	gitCommit(t, dir, "remote", "add", "origin", "https://github.com/Keimer92/Compass-Go.git")
	plain := t.TempDir()

	b := NewBridge(dir)
	b.mu.Lock()
	b.ws.Projects = []*workspace.Project{
		{Path: dir},
		{Path: plain},
	}
	b.mu.Unlock()

	got := map[string]ProjectRepo{}
	for _, r := range b.ProjectRepos() {
		got[r.Path] = r
	}
	if len(got) != 2 {
		t.Fatalf("esperaba 2 proyectos, got %d: %+v", len(got), got)
	}
	repo := got[dir]
	if !repo.IsRepo || !repo.IsGitHub {
		t.Fatalf("repo GitHub mal detectado: %+v", repo)
	}
	if repo.WebURL != "https://github.com/Keimer92/Compass-Go" {
		t.Fatalf("webUrl inesperada: %q", repo.WebURL)
	}
	if repo.RemoteURL == "" {
		t.Fatal("falta la URL de origin")
	}
	local := got[plain]
	if local.IsRepo || local.IsGitHub || local.WebURL != "" {
		t.Fatalf("una carpeta comun debe ser solo local: %+v", local)
	}
}

// Un repo sin remoto tambien es "solo local" desde el punto de vista de la
// insignia, aunque git diga que es repositorio.
func TestProjectReposWithoutRemote(t *testing.T) {
	dir := initRepo(t)
	b := NewBridge(dir)
	b.mu.Lock()
	b.ws.Projects = []*workspace.Project{{Path: dir}}
	b.mu.Unlock()

	got := b.ProjectRepos()
	if len(got) != 1 {
		t.Fatalf("esperaba 1 proyecto, got %d", len(got))
	}
	if !got[0].IsRepo || got[0].IsGitHub || got[0].WebURL != "" {
		t.Fatalf("repo sin remoto mal detectado: %+v", got[0])
	}
}
