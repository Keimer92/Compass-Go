//go:build !windows

package shell

// ConsoleTitle no aplica fuera de Windows.
func ConsoleTitle() (string, bool) { return "", false }
