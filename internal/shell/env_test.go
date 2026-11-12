package shell

import (
	"os"
	"strings"
	"testing"
)

func TestMergeEnvKeepsProcessEnvAndOverrides(t *testing.T) {
	t.Setenv("COMPASS_MERGE_TEST", "original")

	env := mergeEnv(map[string]string{
		"COMPASS_MERGE_TEST": "override",
		"GOOS":               "android",
	})

	var found, goos bool
	for _, kv := range env {
		switch {
		case kv == "COMPASS_MERGE_TEST=override":
			found = true
		case strings.HasPrefix(kv, "COMPASS_MERGE_TEST="):
			t.Errorf("quedo la variable sin sobrescribir: %q", kv)
		case kv == "GOOS=android":
			goos = true
		}
	}
	if !found {
		t.Error("mergeEnv no aplico la variable extra")
	}
	if !goos {
		t.Error("mergeEnv no aplico GOOS")
	}
	if os.Getenv("PATH") != "" {
		pathFound := false
		for _, kv := range env {
			if strings.HasPrefix(strings.ToUpper(kv), "PATH=") {
				pathFound = true
			}
		}
		if !pathFound {
			t.Error("mergeEnv perdio PATH del proceso")
		}
	}
}
