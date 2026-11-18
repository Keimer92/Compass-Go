//go:build !windows

package window

// Focus no hace nada fuera de Windows.
func Focus(title string) bool { return false }

// AllowForeground no hace nada fuera de Windows.
func AllowForeground(pid int) {}

// FocusHandle no hace nada fuera de Windows.
func FocusHandle(hwnd uintptr) bool { return false }
