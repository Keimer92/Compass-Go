//go:build windows

// Package lock implementa la instancia unica por directorio de proyecto
// (equivalente de src/utils/instance_lock.rs).
//
// El candado es un LockFileEx sobre un fichero por proyecto dentro de los datos
// de la aplicacion: no se deja nada en la carpeta del proyecto, que puede ser de
// solo lectura o estar dentro de un repositorio.
package lock

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// Lock representa el candado de instancia; al cerrarse libera el archivo.
type Lock struct {
	file *os.File
}

// TryAcquire intenta bloquear la clave key (la ruta del proyecto). Devuelve:
//   - (lock, true, nil)  si se obtuvo el bloqueo (somos la instancia unica)
//   - (nil, false, nil)  si ya hay otra instancia corriendo
//   - (nil, false, err)  si hubo un error al intentarlo
func TryAcquire(key string) (*Lock, bool, error) {
	path, err := lockPath(key)
	if err != nil {
		return nil, false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, false, err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
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

// Close libera el bloqueo. El fichero se queda en disco: borrarlo abre una
// carrera en la que otra instancia puede bloquear un fichero que ya no esta
// ligado al nombre (y una tercera crearia uno nuevo, con dos candados vivos
// para el mismo proyecto). Un fichero vacio por proyecto no molesta.
func (l *Lock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	ol := new(windows.Overlapped)
	_ = windows.UnlockFileEx(windows.Handle(l.file.Fd()), 0, 1, 0, ol)
	err := l.file.Close()
	l.file = nil
	return err
}

// lockPath devuelve el fichero de candado de un proyecto:
// %LOCALAPPDATA%\compass\instances\<hash>.lock. El nombre sale de la ruta
// normalizada para que dos formas de escribirla ("C:\P", "c:\p\") den el mismo
// candado.
func lockPath(key string) (string, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = os.TempDir()
	}
	abs, err := filepath.Abs(key)
	if err != nil {
		abs = key
	}
	norm := strings.ToLower(filepath.Clean(abs))
	sum := sha256.Sum256([]byte(norm))
	name := hex.EncodeToString(sum[:8]) + ".lock"
	return filepath.Join(base, "compass", "instances", name), nil
}
