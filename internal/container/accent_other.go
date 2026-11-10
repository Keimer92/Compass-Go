//go:build !windows

package container

// instanceIndex es 0 fuera de Windows: no hay snapshot de procesos del host.
func instanceIndex() int { return 0 }
