package container

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

func key(k tcell.Key, r rune, mod tcell.ModMask) []byte {
	return EncodeKey(tcell.NewEventKey(k, r, mod))
}

func eq(t *testing.T, got []byte, want string) {
	t.Helper()
	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestEncodeKeyPlainTextAndEnter(t *testing.T) {
	eq(t, key(tcell.KeyRune, 'a', tcell.ModNone), "a")
	eq(t, key(tcell.KeyRune, 'ñ', tcell.ModNone), "ñ")
	eq(t, key(tcell.KeyEnter, 0, tcell.ModNone), "\r")
	eq(t, key(tcell.KeyTab, 0, tcell.ModNone), "\t")
	eq(t, key(tcell.KeyBackspace, 0, tcell.ModNone), "\x7f")
	eq(t, key(tcell.KeyEsc, 0, tcell.ModNone), "\x1b")
}

func TestEncodeKeyControlLetters(t *testing.T) {
	eq(t, key(tcell.KeyCtrlC, 0, tcell.ModCtrl), "\x03")
	eq(t, key(tcell.KeyCtrlA, 0, tcell.ModCtrl), "\x01")
	eq(t, key(tcell.KeyCtrlZ, 0, tcell.ModCtrl), "\x1a")
	eq(t, key(tcell.KeyCtrlSpace, 0, tcell.ModCtrl), "\x00")
}

func TestEncodeKeyArrowsAndNavigation(t *testing.T) {
	eq(t, key(tcell.KeyUp, 0, tcell.ModNone), "\x1b[A")
	eq(t, key(tcell.KeyDown, 0, tcell.ModNone), "\x1b[B")
	eq(t, key(tcell.KeyRight, 0, tcell.ModNone), "\x1b[C")
	eq(t, key(tcell.KeyLeft, 0, tcell.ModNone), "\x1b[D")
	eq(t, key(tcell.KeyHome, 0, tcell.ModNone), "\x1b[H")
	eq(t, key(tcell.KeyEnd, 0, tcell.ModNone), "\x1b[F")
	eq(t, key(tcell.KeyInsert, 0, tcell.ModNone), "\x1b[2~")
	eq(t, key(tcell.KeyDelete, 0, tcell.ModNone), "\x1b[3~")
	eq(t, key(tcell.KeyPgUp, 0, tcell.ModNone), "\x1b[5~")
	eq(t, key(tcell.KeyPgDn, 0, tcell.ModNone), "\x1b[6~")
	eq(t, key(tcell.KeyBacktab, 0, tcell.ModNone), "\x1b[Z")
}

func TestEncodeKeyFunctionKeys(t *testing.T) {
	eq(t, key(tcell.KeyF1, 0, tcell.ModNone), "\x1bOP")
	eq(t, key(tcell.KeyF4, 0, tcell.ModNone), "\x1bOS")
	eq(t, key(tcell.KeyF5, 0, tcell.ModNone), "\x1b[15~")
	eq(t, key(tcell.KeyF12, 0, tcell.ModNone), "\x1b[24~")
}

func TestEncodeKeyAltPrefixesEscape(t *testing.T) {
	eq(t, key(tcell.KeyRune, 'x', tcell.ModAlt), "\x1bx")
	// Las secuencias que ya empiezan por ESC no se prefijan dos veces.
	eq(t, key(tcell.KeyUp, 0, tcell.ModAlt), "\x1b[A")
}

func TestEncodeMouseIsRelativeToTheBody(t *testing.T) {
	// Fila 4 con barra de 3: el hijo cuenta desde 1 en su propio origen.
	down := tcell.NewEventMouse(5, 4, tcell.Button1, tcell.ModNone)
	seq, _ := EncodeMouseSGR(down, tcell.ButtonNone, BarHeight)
	if string(seq) != "\x1b[<0;6;2M" {
		t.Fatalf("down: %q", seq)
	}

	up := tcell.NewEventMouse(5, 4, tcell.ButtonNone, tcell.ModNone)
	seq, _ = EncodeMouseSGR(up, tcell.Button1, BarHeight)
	if string(seq) != "\x1b[<0;6;2m" {
		t.Fatalf("up: %q", seq)
	}
}

func TestEncodeMouseDragAndWheel(t *testing.T) {
	drag := tcell.NewEventMouse(2, 5, tcell.Button1, tcell.ModNone)
	seq, _ := EncodeMouseSGR(drag, tcell.Button1, BarHeight)
	if string(seq) != "\x1b[<32;3;3M" {
		t.Fatalf("drag: %q", seq)
	}

	wheel := tcell.NewEventMouse(2, 5, tcell.WheelUp, tcell.ModNone)
	seq, _ = EncodeMouseSGR(wheel, tcell.ButtonNone, BarHeight)
	if string(seq) != "\x1b[<64;3;3M" {
		t.Fatalf("wheel: %q", seq)
	}
}

func TestEncodeMouseOnTheBarIsNotForwarded(t *testing.T) {
	e := tcell.NewEventMouse(2, 1, tcell.Button1, tcell.ModNone)
	if seq, _ := EncodeMouseSGR(e, tcell.ButtonNone, BarHeight); seq != nil {
		t.Fatalf("la barra no se reenvia: %q", seq)
	}
}

func TestShortTitleTruncates(t *testing.T) {
	if got := shortTitle("corto", 20); got != "corto" {
		t.Fatalf("got %q", got)
	}
	long := "una etiqueta de tarea bastante larga"
	got := shortTitle(long, 20)
	if len([]rune(got)) != 20 || []rune(got)[19] != '…' {
		t.Fatalf("got %q", got)
	}
}

func TestPaneScriptQuotesAndPropagatesExitCode(t *testing.T) {
	s := PaneScript(`C:\Users\Un Usuario\bin\compass-go.exe`, "android-install-debug", `D:\Mis Proyectos\Go + Wails v3`)
	want := `& 'C:\Users\Un Usuario\bin\compass-go.exe' --task android-install-debug --path 'D:\Mis Proyectos\Go + Wails v3'; exit $LASTEXITCODE`
	if s != want {
		t.Fatalf("got %q", s)
	}
}
