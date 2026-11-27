//go:build !windows

package whatsapp

// acquireEngineInstance no aplica fuera de Windows: GymPro es una aplicación
// de escritorio Windows y este candado existe para su modelo de proceso.
func acquireEngineInstance() (bool, error) { return true, nil }
