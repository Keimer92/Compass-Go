//go:build !windows

package clipboard

import "fmt"

// Write no hace nada fuera de Windows.
func Write(text string) error {
	return fmt.Errorf("el portapapeles solo esta implementado en Windows")
}
