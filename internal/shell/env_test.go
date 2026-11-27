package shell

import (
	"os"
	"path/filepath"
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

// Un JAVA_HOME que apunta a un JDK borrado o movido rompe Gradle; debe
// descartarse del entorno de los hijos para que usen el java del PATH.
func TestMergeEnvDropsBrokenJavaHome(t *testing.T) {
	t.Setenv("JAVA_HOME", filepath.Join(t.TempDir(), "jdk-borrado"))

	env := mergeEnv(nil)
	for _, kv := range env {
		if strings.HasPrefix(strings.ToUpper(kv), "JAVA_HOME=") {
			t.Fatalf("mergeEnv conservo un JAVA_HOME invalido: %q", kv)
		}
	}
}

func TestMergeEnvKeepsValidJavaHome(t *testing.T) {
	jdk := t.TempDir()
	if err := os.MkdirAll(filepath.Join(jdk, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jdk, "bin", "java.exe"), []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JAVA_HOME", jdk)

	found := false
	for _, kv := range mergeEnv(nil) {
		if kv == "JAVA_HOME="+jdk {
			found = true
		}
	}
	if !found {
		t.Fatal("mergeEnv descarto un JAVA_HOME valido")
	}
}

func TestEnvironDropsBrokenJavaHome(t *testing.T) {
	t.Setenv("JAVA_HOME", filepath.Join(t.TempDir(), "no-existe"))
	for _, kv := range Environ() {
		if strings.HasPrefix(strings.ToUpper(kv), "JAVA_HOME=") {
			t.Fatalf("Environ conservo un JAVA_HOME invalido: %q", kv)
		}
	}
}
