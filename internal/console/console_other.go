//go:build !windows

// Package console protege el cierre de la ventana anfitriona mientras hay
// pestanas internas abiertas. En el resto de sistemas no hace falta.
package console

// Install no hace nada fuera de Windows.
func Install() {}

// SetOpenTabs no hace nada fuera de Windows.
func SetOpenTabs(int) {}

// RegisterPane no hace nada fuera de Windows.
func RegisterPane(int) {}

// UnregisterPane no hace nada fuera de Windows.
func UnregisterPane(int) {}
