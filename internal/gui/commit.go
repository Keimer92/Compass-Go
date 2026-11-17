package gui

import (
	"errors"
	"fmt"
	"image/color"
	"os/exec"
	"strings"
	"time"

	"compass/internal/windialog"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func pillText(th *material.Theme, txt string, col color.NRGBA) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		lbl := material.Body2(th, txt)
		lbl.Color = col
		return layout.Inset{
			Left: unit.Dp(12), Right: unit.Dp(12),
			Top: unit.Dp(6), Bottom: unit.Dp(6),
		}.Layout(gtx, lbl.Layout)
	}
}

// CommitDraft es el estado del panel de commit asistido por opencode.
type CommitDraft struct {
	Open    bool
	Loading bool
	Error   string

	Editor     widget.Editor
	genBtn     widget.Clickable
	commitBtn  widget.Clickable
	cancelBtn  widget.Clickable
}

type commitResult struct {
	message string
	err     error
}

// gitSummary devuelve un resumen corto de los cambios para el prompt.
func gitSummary(root string) string {
	var out []string
	for _, args := range [][]string{
		{"diff", "--stat", "--no-color"},
		{"diff", "--cached", "--stat", "--no-color"},
		{"status", "--porcelain=v1", "-uall"},
	} {
		cmd := exec.Command("git", args...)
		windialog.NoWindow(cmd)
		cmd.Dir = root
		b, err := cmd.Output()
		if err == nil {
			out = append(out, strings.TrimSpace(string(b)))
		}
	}
	s := strings.Join(out, "\n")
	if len(s) > 4000 {
		s = s[:4000]
	}
	return s
}

// generateCommitMessage pide a opencode un mensaje de commit detallado en ingles.
func generateCommitMessage(root, model string) (string, error) {
	summary := gitSummary(root)
	if strings.TrimSpace(summary) == "" {
		return "", errors.New("no hay cambios para commitear")
	}
	prompt := "Write a git commit message in ENGLISH. Do NOT explore the repository.\n" +
		"Use ONLY the change list below. Output exactly this format:\n" +
		"Line 1: type(scope): imperative summary (max 72 chars).\n" +
		"Then 3 or 4 short bullet lines starting with '- ' (max 90 chars each),\n" +
		"each describing a concrete change from the list (mention filenames when useful).\n" +
		"No blank lines, no markdown, no quotes, no explanation.\n\n" +
		"Change list:\n" + summary

	if _, err := exec.LookPath("opencode"); err != nil {
		return "", errors.New("opencode no está en el PATH")
	}
	if strings.TrimSpace(model) == "" {
		model = "opencode/nemotron-3.5-lightning-free"
	}
	cmd := exec.Command("opencode", "run", "-m", model, prompt)
	windialog.NoWindow(cmd)
	cmd.Dir = root
	done := make(chan struct{})
	var out []byte
	var err error
	go func() {
		out, err = cmd.CombinedOutput()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(90 * time.Second):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return "", errors.New("OpenCode tardo demasiado en responder")
	}
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if len(detail) > 200 {
			detail = detail[len(detail)-200:]
		}
		return "", fmt.Errorf("opencode falló: %v\n%s", err, detail)
	}
	return cleanCommitMessage(string(out)), nil
}

// cleanCommitMessage conserva el mensaje multilinea y descarta las lineas de
// estado del CLI ("> build ...", herramientas, bloques de codigo, etc.).
func cleanCommitMessage(raw string) string {
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		line = strings.TrimRight(line, " \t")
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, ">") || strings.HasPrefix(t, "✱") ||
			strings.HasPrefix(t, "→") || strings.HasPrefix(t, "←") ||
			strings.HasPrefix(t, "▸") || strings.HasPrefix(t, "```") ||
			strings.HasPrefix(strings.ToLower(t), "here is") {
			continue
		}
		if t == "" {
			if len(out) > 0 && out[len(out)-1] != "" {
				out = append(out, "")
			}
			continue
		}
		out = append(out, strings.Trim(line, "`\"' "))
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	// Cabecera + 3 o 4 viñetas cortas, recortadas.
	if len(out) > 5 {
		out = out[:5]
	}
	for i, line := range out {
		if len(line) > 95 {
			out[i] = line[:95]
		}
	}
	return strings.Join(out, "\n")
}

// fallbackCommitMessage genera un mensaje util sin agente, a partir del resumen
// de cambios. Se usa cuando opencode falla (p.ej. token caducado).
func fallbackCommitMessage(root string) string {
	summary := gitSummary(root)
	n := 0
	for _, line := range strings.Split(summary, "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	if n == 0 {
		return "chore: update project"
	}
	return fmt.Sprintf("chore: update %d files", n)
}

// doCommit stagea todos los cambios y hace el commit con el mensaje dado.
func doCommit(root, message string) (string, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return "", errors.New("el mensaje del commit esta vacio")
	}
	add := exec.Command("git", "add", "-A")
	windialog.NoWindow(add)
	add.Dir = root
	if err := add.Run(); err != nil {
		return "", fmt.Errorf("git add: %w", err)
	}
	cmd := exec.Command("git", "commit", "-m", message)
	windialog.NoWindow(cmd)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git commit: %s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// layoutCommit dibuja el modal de commit sobre toda la ventana.
// layoutCommitPanel dibuja el formulario de commit dentro del panel derecho.
func (a *Application) layoutCommitPanel(gtx layout.Context) {
	layout.Inset{Top: unit.Dp(14), Left: unit.Dp(14), Right: unit.Dp(14)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return dialogTitle(gtx, a, a.palette, "Commit con OpenCode", &a.commit.cancelBtn)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						ed := material.Editor(a.th, &a.commit.Editor, "Mensaje del commit...")
						ed.Color = a.palette.Text
						return ed.Layout(gtx)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					txt := "Generar con OpenCode"
					if a.commit.Loading {
						txt = "Generando..."
					}
					return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return a.pillButton(gtx, &a.palette, txt, &a.commit.genBtn, false)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Caption(a.th, "Se añadirán todos los cambios (git add -A)")
						lbl.Color = a.palette.Muted
						return lbl.Layout(gtx)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if a.commit.Error == "" {
						return layout.Dimensions{}
					}
					return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Caption(a.th, a.commit.Error)
						lbl.Color = a.palette.Warning
						return lbl.Layout(gtx)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return a.pillButton(gtx, &a.palette, "Commit", &a.commit.commitBtn, false)
					})
				}),
			)
		})
}
