//go:build windows

package shell

import (
	"syscall"
	"unsafe"
)

var procGetConsoleTitleW = syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleTitleW")

// ConsoleTitle devuelve el título real de la consola del proceso.
//
// ok=false si no hay consola (stdout redirigido, tests) o la API falla. Sirve
// para detectar que otro proceso lo sobrescribió: una pestaña ConPTY interna
// propaga su título a la consola del host y el OpenConsole elevado le antepone
// "Administrator: ".
func ConsoleTitle() (string, bool) {
	buf := make([]uint16, 256)
	n, _, _ := procGetConsoleTitleW.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)
	if n == 0 || n > uintptr(len(buf)) {
		return "", false
	}
	return syscall.UTF16ToString(buf[:n]), true
}
