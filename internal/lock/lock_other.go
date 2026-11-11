//go:build !windows

package lock

import "fmt"

// Lock es el candado de instancia (no implementado fuera de Windows).
type Lock struct{}

// TryAcquire siempre falla fuera de Windows.
func TryAcquire(key string) (*Lock, bool, error) {
	return nil, false, fmt.Errorf("Compass solo tiene soporte para Windows")
}

// Close es no-op.
func (l *Lock) Close() error { return nil }
