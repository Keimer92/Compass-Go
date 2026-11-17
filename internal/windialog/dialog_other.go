//go:build !windows

package windialog

// Error no hace nada fuera de Windows.
func Error(title, text string) {}

// HideConsole no hace nada fuera de Windows.
func HideConsole() {}
