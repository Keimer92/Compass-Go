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

// generateCommitMessage pide a opencode una unica linea de commit.
func generateCommitMessage(root, model string) (string, error) {
	summary := gitSummary(root)
	if strings.TrimSpace(summary) == "" {
		return "", errors.New("no hay cambios para commitear")
	}
	prompt := "Genera UN SOLO mensaje de commit en español, formato Conventional Commits " +
		"(tipo: descripcion corta), maximo 72 caracteres, sin comillas ni markdown. " +
		"Responde unicamente con esa linea, sin explicaciones. Cambios:\n" + summary

	if _, err := exec.LookPath("opencode"); err != nil {
		return "", errors.New("opencode no está en el PATH")
	}
	if strings.TrimSpace(model) == "" {
		model = "opencode/hy4-flash-free"
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
	return cleanCommitLine(string(out)), nil
}

// cleanCommitLine se queda con la ultima linea util de la salida de opencode,
// ignorando las lineas de estado ("> build ...", herramientas, etc.).
func cleanCommitLine(raw string) string {
	var candidate string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ">") ||
			strings.HasPrefix(line, "✱") || strings.HasPrefix(line, "→") ||
			strings.HasPrefix(line, "←") || strings.HasPrefix(line, "▸") {
			continue
		}
		candidate = line
	}
	candidate = strings.Trim(candidate, "`\"' ")
	if len(candidate) > 100 {
		candidate = candidate[:100]
	}
	return candidate
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
		return "chore: actualizar proyecto"
	}
	return fmt.Sprintf("chore: actualizar %d archivos", n)
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
						return a.commit.genBtn.Layout(gtx, pillText(a.th, txt, a.palette.Accent))
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
						return a.commit.commitBtn.Layout(gtx, pillText(a.th, "Commit", a.palette.Success))
					})
				}),
			)
		})
}
