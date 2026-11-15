//go:build !windows

package sound

// Success es un no-op fuera de Windows (Compass solo se distribuye en Windows).
func Success() {}

// Failure es un no-op fuera de Windows.
func Failure() {}
