//go:build !windows

package tasks

// closeProjectWhatsappEngines no hace nada fuera de Windows (la shell, CMake y
// los motores node.exe son específicos de Windows).
func closeProjectWhatsappEngines(string) int { return 0 }
