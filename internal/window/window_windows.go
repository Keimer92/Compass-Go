//go:build windows

package window

import (
	"syscall"
	"unsafe"
)

var (
	user32 = syscall.NewLazyDLL("user32.dll")

	procEnumWindows         = user32.NewProc("EnumWindows")
	procGetWindowTextW      = user32.NewProc("GetWindowTextW")
	procGetClassNameW       = user32.NewProc("GetClassNameW")
	procIsWindowVisible     = user32.NewProc("IsWindowVisible")
	procIsIconic            = user32.NewProc("IsIconic")
	procShowWindow          = user32.NewProc("ShowWindow")
	procBringWindowToTop    = user32.NewProc("BringWindowToTop")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
)

// swRestore es el SW_* de ShowWindow que saca una ventana minimizada.
const swRestore = 9

// consoleClasses son las clases de ventana que hospedan una consola: conhost /
// OpenConsole y Windows Terminal. Asi un titulo igual en otra aplicacion no nos
// hace traer al frente la ventana equivocada.
var consoleClasses = []string{"ConsoleWindowClass", "CASCADIA_HOSTING_WINDOW_CLASS"}

// focusTitle es el titulo buscado por el callback de enumeracion.
var focusTitle string

// focusFound es el HWND encontrado (0 = ninguno).
var focusFound uintptr

// enumCallback se registra una sola vez: NewCallback tiene un numero limitado
// de huecos y no se liberan.
var enumCallback = syscall.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
	if visible, _, _ := procIsWindowVisible.Call(hwnd); visible == 0 {
		return 1
	}
	if !isConsoleWindow(hwnd) {
		return 1
	}
	if !titleMatches(windowText(hwnd), focusTitle) {
		return 1
	}
	focusFound = hwnd
	return 0
})

// Focus trae al frente la ventana de consola cuyo titulo corresponde a title
// (el nombre del directorio del proyecto). Devuelve false si no hay ninguna.
func Focus(title string) bool {
	if title == "" {
		return false
	}
	focusTitle = title
	focusFound = 0
	_, _, _ = procEnumWindows.Call(enumCallback, 0)
	if focusFound == 0 {
		return false
	}

	if iconic, _, _ := procIsIconic.Call(focusFound); iconic != 0 {
		_, _, _ = procShowWindow.Call(focusFound, swRestore)
	}
	_, _, _ = procBringWindowToTop.Call(focusFound)
	_, _, _ = procSetForegroundWindow.Call(focusFound)
	return true
}

// isConsoleWindow indica si la ventana pertenece a una consola.
func isConsoleWindow(hwnd uintptr) bool {
	name := windowClass(hwnd)
	for _, c := range consoleClasses {
		if name == c {
			return true
		}
	}
	return false
}

// windowText devuelve el titulo de la ventana.
func windowText(hwnd uintptr) string { return windowString(hwnd, procGetWindowTextW) }

// windowClass devuelve la clase de la ventana.
func windowClass(hwnd uintptr) string { return windowString(hwnd, procGetClassNameW) }

func windowString(hwnd uintptr, proc *syscall.LazyProc) string {
	buf := make([]uint16, 512)
	n, _, _ := proc.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}
