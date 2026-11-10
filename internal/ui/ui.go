// Package ui agrupa la presentacion en consola: banners, pausas y la cuenta
// regresiva que cierra la pestana. Equivalente de src/utils/ui.rs.
package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"compass/internal/shell"
	"github.com/rivo/uniseg"
)

// IsYes indica si una respuesta de [S/n] es afirmativa.
func IsYes(answer string) bool {
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "s", "si", "sí", "y", "yes":
		return true
	default:
		return false
	}
}

// AskText pregunta y devuelve el texto introducido (default si vacio).
func AskText(prompt string, def string) string {
	msg := prompt + ": "
	if def != "" {
		msg = fmt.Sprintf("%s [%s]: ", prompt, def)
	}
	fmt.Print(msg)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.TrimRight(line, "\r\n")
	if strings.TrimSpace(line) == "" {
		return def
	}
	return line
}

// AskPassword lee una contrasena. Por ahora lee linea (sin ocultar); el modo raw
// lo aporta tcell en una fase posterior.
func AskPassword(prompt string) string {
	fmt.Print(prompt + ": ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimRight(line, "\r\n")
}

// Banner imprime un banner con borde y el texto centrado (cuenta runes).
func Banner(icon, title string) {
	inner := fmt.Sprintf("  %s  %s  ", icon, title)
	innerW := displayWidth(inner)
	width := innerW
	if width < 48 {
		width = 48
	}
	hBorder := strings.Repeat("─", width)
	left := (width - innerW) / 2
	right := width - innerW - left
	fmt.Println()
	fmt.Printf("  ┌%s┐\n", hBorder)
	fmt.Printf("  │%s%s%s│\n", strings.Repeat(" ", left), inner, strings.Repeat(" ", right))
	fmt.Printf("  └%s┘\n", hBorder)
	fmt.Println()
}

// SuccessBanner banner de exito.
func SuccessBanner(title string) { Banner("✅", title) }

// InfoBanner banner informativo.
func InfoBanner(title string) { Banner("ℹ\uFE0E", title) }

// BoxBanner banner de caja ┌═┐ con el titulo centrado.
func BoxBanner(title string) {
	width := displayWidth(title)
	if width < 48 {
		width = 48
	}
	pad := width - displayWidth(title)
	left := pad / 2
	right := pad - left
	fmt.Printf("╔%s╗\n", strings.Repeat("═", width))
	fmt.Printf("║%s%s%s║\n", strings.Repeat(" ", left), title, strings.Repeat(" ", right))
	fmt.Printf("╚%s╝\n", strings.Repeat("═", width))
}

// Pause espera a que el usuario pulse Enter.
func Pause() { shell.Pause() }

// PauseFor cuenta regresiva y, dentro del contenedor, pide cerrar la pestana.
func PauseFor(seconds uint) { shell.PauseFor(seconds) }

func displayWidth(s string) int {
	return uniseg.StringWidth(s)
}
