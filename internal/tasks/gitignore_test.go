package tasks

import (
	"strings"
	"testing"

	"compass/internal/project"
)

func TestGitignoreContentIgnoresTaskChecksums(t *testing.T) {
	for _, pt := range allTypes() {
		if !strings.Contains(gitignoreContent(pt), "\n.task/\n") {
			t.Errorf("gitignoreContent(%v) no ignora .task/", pt)
		}
	}
}

func TestGitignoreContentIgnoresNodeModulesAndDist(t *testing.T) {
	for _, pt := range allTypes() {
		content := gitignoreContent(pt)
		for _, rule := range []string{"\nnode_modules/\n", "\ndist/\n"} {
			if !strings.Contains(content, rule) {
				t.Errorf("gitignoreContent(%v) no incluye %q", pt, strings.TrimSpace(rule))
			}
		}
	}
}

func TestGitignoreGoIgnoresFrontendArtifacts(t *testing.T) {
	content := gitignoreContent(project.TypeGo)
	for _, rule := range []string{"frontend/node_modules/", "frontend/dist/", "*.syso", "coverage.out"} {
		if !strings.Contains(content, rule) {
			t.Errorf("el .gitignore de Go no incluye %q", rule)
		}
	}
}

func TestGitignoreContentIgnoresWhatsappStagedEngine(t *testing.T) {
	for _, pt := range allTypes() {
		content := gitignoreContent(pt)
		for _, rule := range whatsappStagedRules {
			if !strings.Contains(content, "\n"+rule+"\n") {
				t.Errorf("gitignoreContent(%v) no ignora %q", pt, rule)
			}
		}
	}
}

func TestGitignoreFlutterIgnoresGeneratedPlatformDirs(t *testing.T) {
	content := gitignoreContent(project.TypeFlutter)
	for _, rule := range []string{
		"windows/flutter/ephemeral/",
		"linux/flutter/ephemeral/",
		"macos/Flutter/ephemeral/",
		"ios/Flutter/ephemeral/",
		"*.iml",
		"android/app/release/",
		"bin/",
	} {
		if !strings.Contains(content, "\n"+rule+"\n") {
			t.Errorf("el .gitignore de Flutter no incluye %q", rule)
		}
	}
}

func TestGitignoreKeepsEnvExample(t *testing.T) {
	for _, pt := range allTypes() {
		content := gitignoreContent(pt)
		if strings.Contains(content, "\n.env.example\n") {
			t.Errorf("gitignoreContent(%v) ignora .env.example, que debe versionarse", pt)
		}
		if !strings.Contains(content, "\n!.env.example\n") {
			t.Errorf("gitignoreContent(%v) no re-incluye .env.example", pt)
		}
	}
}

func TestMergeGitignoreAddsOnlyMissingRules(t *testing.T) {
	existing := "# mio\nnode_modules/\n*.log\n"
	merged := mergeGitignore(existing, gitignoreContentForPlatform(project.TypeGo))
	if strings.Count(merged, "\nnode_modules/\n") != 1 {
		t.Fatalf("node_modules/ se duplico:\n%s", merged)
	}
	if !strings.Contains(merged, "\ndist/\n") {
		t.Fatalf("no se anadio dist/:\n%s", merged)
	}
	if !strings.HasPrefix(merged, "# mio\n") {
		t.Fatalf("se perdio el contenido original:\n%s", merged)
	}
}
