//go:build windows

// Package lock implementa la instancia unica por directorio de proyecto
// (equivalente de src/utils/instance_lock.rs).
package lock

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// Lock representa el candado de instancia; al cerrarse libera el archivo.
type Lock struct {
	file *os.File
}

// TryAcquire intenta bloquear el directorio dir. Devuelve:
//   - (lock, true, nil)  si se obtuvo el bloqueo (somos la instancia unica)
//   - (nil, false, nil)  si ya hay otra instancia corriendo
//   - (nil, false, err)  si hubo un error al intentarlo
func TryAcquire(dir string) (*Lock, bool, error) {
	path := filepath.Join(dir, ".compass-go-instance.lock")
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, false, err
	}
	ol := new(windows.Overlapped)
	err = windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0, ol,
	)
	if err != nil {
		f.Close()
		// ERROR_LOCK_VIOLATION (33) o ERROR_IO_PENDING => ya bloqueado.
		return nil, false, nil
	}
	return &Lock{file: f}, true, nil
}

// Close libera el bloqueo y elimina el archivo de candado.
func (l *Lock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	ol := new(windows.Overlapped)
	_ = windows.UnlockFileEx(windows.Handle(l.file.Fd()), 0, 1, 0, ol)
	path := l.file.Name()
	err := l.file.Close()
	_ = os.Remove(path)
	return err
}
