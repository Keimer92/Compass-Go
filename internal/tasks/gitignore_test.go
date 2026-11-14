package tasks

import (
	"strings"
	"testing"
)

func TestGitignoreContentIgnoresTaskChecksums(t *testing.T) {
	for _, pt := range allTypes() {
		if !strings.Contains(gitignoreContent(pt), "\n.task/\n") {
			t.Errorf("gitignoreContent(%v) no ignora .task/", pt)
		}
	}
}
