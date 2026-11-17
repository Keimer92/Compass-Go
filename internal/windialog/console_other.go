//go:build !windows

package windialog

// AttachParentConsole no hace nada fuera de Windows.
func AttachParentConsole() {}
