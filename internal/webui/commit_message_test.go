package webui

import (
	"strings"
	"testing"
)

// Un modelo que devuelve todo en una linea no debe acabar como asunto
// interminable en el historial.
func TestCleanCommitMessageSplitsInlineBullets(t *testing.T) {
	raw := "fix(receipt): tolerate missing emblem asset when rendering receipt - " +
		"Wrap precacheImage of assets/receipt/ela_emblem.png in try/catch inside ReceiptRenderer - " +
		"Swallow image load errors so OverlayEntry receipt capture proceeds without the emblem - " +
		"Remove unused logoStr lookup of the logo setting in _PaymentsPageState._showReceiptDialog"

	msg := cleanCommitMessage(raw)
	lines := strings.Split(msg, "\n")

	if got, want := lines[0], "fix(receipt): tolerate missing emblem asset when rendering receipt"; got != want {
		t.Fatalf("asunto = %q, quiero %q", got, want)
	}
	if lines[1] != "" {
		t.Fatalf("falta la linea en blanco entre asunto y cuerpo: %q", msg)
	}
	if len(lines) != 5 {
		t.Fatalf("lineas = %d, quiero 5:\n%s", len(lines), msg)
	}
	for _, line := range lines[2:] {
		if !strings.HasPrefix(line, "- ") {
			t.Fatalf("viñeta sin prefijo: %q", line)
		}
	}
	if strings.Contains(lines[0], " - ") {
		t.Fatalf("el asunto conserva viñetas: %q", lines[0])
	}
}

func TestCleanCommitMessageKeepsWellFormedOutput(t *testing.T) {
	raw := "feat(webui): add kit badges to project cards\n" +
		"\n" +
		"- Expose Bridge.ProjectKits with whatsapp and d1 flags\n" +
		"- Paint .project-kit chips under the project name\n"

	msg := cleanCommitMessage(raw)
	want := "feat(webui): add kit badges to project cards\n\n" +
		"- Expose Bridge.ProjectKits with whatsapp and d1 flags\n" +
		"- Paint .project-kit chips under the project name"
	if msg != want {
		t.Fatalf("mensaje =\n%s\nquiero\n%s", msg, want)
	}
}

func TestCleanCommitMessageNormalizesBulletMarkers(t *testing.T) {
	raw := "chore: tidy installer\n\n* drop legacy Kit Whatsapp path\n1. purge corekit before install"
	msg := cleanCommitMessage(raw)
	want := "chore: tidy installer\n\n" +
		"- drop legacy Kit Whatsapp path\n" +
		"- purge corekit before install"
	if msg != want {
		t.Fatalf("mensaje =\n%s\nquiero\n%s", msg, want)
	}
}

func TestCleanCommitMessageCapsBulletsAndHeader(t *testing.T) {
	raw := "fix: " + strings.Repeat("palabra ", 20) + "\n\n" +
		"- uno\n- dos\n- tres\n- cuatro\n- cinco\n- seis\n- siete"
	msg := cleanCommitMessage(raw)
	lines := strings.Split(msg, "\n")
	if n := len([]rune(lines[0])); n > commitHeaderMax {
		t.Fatalf("asunto de %d caracteres: %q", n, lines[0])
	}
	if bullets := (len(lines) - 2); bullets != commitMaxBullets {
		t.Fatalf("viñetas = %d, quiero %d:\n%s", bullets, commitMaxBullets, msg)
	}
}

func TestCleanCommitMessageOnlyHeader(t *testing.T) {
	if got, want := cleanCommitMessage("chore: bump deps\n\n"), "chore: bump deps"; got != want {
		t.Fatalf("mensaje = %q, quiero %q", got, want)
	}
	if got := cleanCommitMessage("   \n\n"); got != "" {
		t.Fatalf("mensaje vacio esperado, obtuve %q", got)
	}
}
