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
	// Solo las apps consumen el motor etajado; los corekits no lo necesitan.
	for _, pt := range appTypes() {
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

// El kit D1 guarda credenciales y estado de Wrangler que nunca deben subirse.
func TestGitignoreKitD1IgnoresCloudflareSecrets(t *testing.T) {
	content := gitignoreContent(project.TypeKitD1)
	for _, rule := range []string{"\n.wrangler/\n", "\n.dev.vars\n", "\nsync-keys.json\n", "\n*.key\n", "\n*.db\n"} {
		if !strings.Contains(content, rule) {
			t.Errorf("el .gitignore del Kit D1 no incluye %q", strings.TrimSpace(rule))
		}
	}
	// Las reglas del motor WhatsApp etajado solo aplican a proyectos que lo
	// consumen; en el kit serian ruido.
	if strings.Contains(content, "resources/whatsapp-engine-bundle/") {
		t.Error("el .gitignore del Kit D1 no debe incluir el staging de WhatsApp")
	}
}

func TestGitignoreWhatsappKitIgnoresBundleAndAuth(t *testing.T) {
	content := gitignoreContent(project.TypeWhatsappKit)
	for _, rule := range []string{"\nwhatsapp-engine-bundle/\n", "\nsingle/\n", "\nnode.exe\n", "\nwhatsapp_auth/\n"} {
		if !strings.Contains(content, rule) {
			t.Errorf("el .gitignore del kit WhatsApp no incluye %q", strings.TrimSpace(rule))
		}
	}
}
