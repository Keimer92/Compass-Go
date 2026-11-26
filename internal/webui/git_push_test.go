package webui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestParsePushLine(t *testing.T) {
	cases := []struct {
		line     string
		ok       bool
		stage    string
		percent  int
		uploaded int64
		speed    int64
	}{
		{"Enumerating objects: 5, done.", false, "", 0, 0, 0},
		{"Counting objects: 100% (5/5), done.", true, "counting", 10, 0, 0},
		{"Compressing objects:  50% (2/4)", true, "compressing", 20, 0, 0},
		{"Writing objects:  45% (5/11), 512.00 KiB | 1.00 MiB/s", true, "writing", 59, 524288, 1048576},
		{"Writing objects: 100% (3/3), 291 bytes, done.", true, "writing", 95, 291, 0},
		{"remote: Resolving deltas: 100% (2/2), completed with 2 local objects.", true, "resolving", 100, 0, 0},
		{"Everything up-to-date", false, "", 0, 0, 0},
		{"fatal: unable to access 'https://github.com/x/y/': 403", false, "", 0, 0, 0},
	}
	for _, c := range cases {
		p, ok := parsePushLine(c.line)
		if ok != c.ok {
			t.Errorf("%q: ok=%v, queria %v", c.line, ok, c.ok)
			continue
		}
		if !ok {
			continue
		}
		if p.Stage != c.stage || p.Percent != c.percent || p.Uploaded != c.uploaded || p.Speed != c.speed {
			t.Errorf("%q: %+v, queria stage=%s pct=%d up=%d speed=%d",
				c.line, p, c.stage, c.percent, c.uploaded, c.speed)
		}
	}
}

func TestSplitProgressLines(t *testing.T) {
	data := []byte("Writing objects:  10% (1/10)\rWriting objects:  20% (2/10)\nDone\n")
	var got []string
	for len(data) > 0 {
		n, tok, err := splitProgressLines(data, false)
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			break
		}
		if len(tok) > 0 {
			got = append(got, string(tok))
		}
		data = data[n:]
	}
	want := []string{
		"Writing objects:  10% (1/10)",
		"Writing objects:  20% (2/10)",
		"Done",
	}
	if len(got) != len(want) {
		t.Fatalf("partio %d lineas, queria %d: %q", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("linea %d = %q, queria %q", i, got[i], want[i])
		}
	}
}

func TestPushTransferSizeCountsNewObjects(t *testing.T) {
	dir := initRepo(t)
	blob := make([]byte, 32*1024)
	for i := range blob {
		blob[i] = byte('a' + i%26)
	}
	if err := os.WriteFile(filepath.Join(dir, "big.txt"), blob, 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommit(t, dir, "add", "-A")
	gitCommit(t, dir, "commit", "-q", "-m", "big")

	// Sin upstream se envian todos los objetos de HEAD.
	if n := pushTransferSize(dir, ""); n <= 0 {
		t.Fatalf("tamano estimado %d, queria > 0", n)
	}
	// Un blob de 32 KiB comprimido no baja de unos cientos de bytes; el limite
	// bajo solo comprueba que se recorren los objetos.
	if n := pushTransferSize(dir, "HEAD"); n != 0 {
		t.Fatalf("sin objetos nuevos el tamano deberia ser 0: %d", n)
	}
}

// TestGitPushEmitsProgress empuja a un remoto bare local (sin red) y comprueba
// que el evento de progreso llega y termina en 100%.
func TestGitPushEmitsProgress(t *testing.T) {
	dir := initRepo(t)
	remote := filepath.Join(t.TempDir(), "remoto.git")
	if out, err := exec.Command("git", "init", "--bare", "-q", remote).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v %s", err, out)
	}
	gitCommit(t, dir, "remote", "add", "origin", remote)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hola"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommit(t, dir, "add", "-A")
	gitCommit(t, dir, "commit", "-q", "-m", "init")

	var events []PushProgress
	prev := emitHook
	emitHook = func(name string, data any) {
		if name != "git:pushprogress" {
			return
		}
		if p, ok := data.(PushProgress); ok {
			events = append(events, p)
		}
	}
	defer func() { emitHook = prev }()

	b := NewBridge(dir)
	if _, err := b.GitPush(); err != nil {
		t.Fatalf("push: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("no se emitio ningun evento de progreso")
	}
	// Las fases reales de git tienen que haberse reconocido, no solo el
	// "done" sintetico del final.
	stages := map[string]bool{}
	for _, e := range events {
		stages[e.Stage] = true
	}
	if !stages["counting"] || !stages["writing"] {
		t.Fatalf("faltan fases reales en %v (eventos: %+v)", stages, events)
	}
	last := events[len(events)-1]
	if last.Stage != "done" || last.Percent != 100 {
		t.Fatalf("ultimo evento %+v, queria done/100", last)
	}
	for _, e := range events {
		if e.Percent < 0 || e.Percent > 100 {
			t.Fatalf("porcentaje fuera de rango: %+v", e)
		}
		if e.Uploaded < 0 || e.Total < 0 {
			t.Fatalf("bytes negativos: %+v", e)
		}
	}
}
