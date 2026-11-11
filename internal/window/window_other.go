//go:build !windows

package window

// Focus no hace nada fuera de Windows.
func Focus(title string) bool { return false }
