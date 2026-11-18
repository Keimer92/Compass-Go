//go:build windows

package window

import (
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procEnumWindows              = user32.NewProc("EnumWindows")
	procGetWindowTextW           = user32.NewProc("GetWindowTextW")
	procGetClassNameW            = user32.NewProc("GetClassNameW")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procIsIconic                 = user32.NewProc("IsIconic")
	procShowWindow               = user32.NewProc("ShowWindow")
	procBringWindowToTop         = user32.NewProc("BringWindowToTop")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procSetFocus                 = user32.NewProc("SetFocus")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	procAttachThreadInput        = user32.NewProc("AttachThreadInput")
	procSetWindowPos             = user32.NewProc("SetWindowPos")
	procAllowSetForegroundWindow = user32.NewProc("AllowSetForegroundWindow")
	procGetCurrentThreadID       = kernel32.NewProc("GetCurrentThreadId")
)

// swRestore es el SW_* de ShowWindow que saca una ventana minimizada.
const swRestore = 9

// asfwAny permite que cualquier proceso ponga su ventana en primer plano.
const asfwAny = 0xFFFFFFFF

// SetWindowPos: -1 = HWND_TOPMOST, -2 = HWND_NOTOPMOST.
const (
	hwndTopmost   = ^uintptr(0)
	hwndNoTopmost = ^uintptr(1)
	swpNoSize     = 0x0001
	swpNoMove     = 0x0002
	swpShowWindow = 0x0040
)

// AllowForeground deja que el proceso pid (la instancia de Compass ya abierta)
// pueda traer su ventana al frente. Lo llama la segunda instancia, que si
// tiene permiso de primer plano por haber sido lanzada por el usuario.
func AllowForeground(pid int) {
	if pid > 0 {
		if ok, _, _ := procAllowSetForegroundWindow.Call(uintptr(pid)); ok != 0 {
			return
		}
	}
	_, _, _ = procAllowSetForegroundWindow.Call(asfwAny)
}

// FocusHandle trae al frente y activa la ventana hwnd, aunque otra aplicacion
// tenga el foco. Devuelve false si no habia ventana.
func FocusHandle(hwnd uintptr) bool {
	if hwnd == 0 {
		return false
	}
	if iconic, _, _ := procIsIconic.Call(hwnd); iconic != 0 {
		_, _, _ = procShowWindow.Call(hwnd, swRestore)
	}

	// Truco clasico: subir a TOPMOST y bajarla devuelve la ventana al orden Z
	// superior incluso si Windows bloquea SetForegroundWindow.
	_, _, _ = procSetWindowPos.Call(hwnd, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpShowWindow)
	_, _, _ = procSetWindowPos.Call(hwnd, hwndNoTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpShowWindow)

	// Engancharse al hilo en primer plano evita que Windows ignore el cambio
	// de foco por no ser el proceso activo.
	cur, _, _ := procGetCurrentThreadID.Call()
	fg, _, _ := procGetForegroundWindow.Call()
	if fg != 0 {
		fgThread, _, _ := procGetWindowThreadProcessID.Call(fg, 0)
		if fgThread != 0 && fgThread != cur {
			_, _, _ = procAttachThreadInput.Call(cur, fgThread, 1)
			defer procAttachThreadInput.Call(cur, fgThread, 0)
		}
	}

	_, _, _ = procBringWindowToTop.Call(hwnd)
	_, _, _ = procSetForegroundWindow.Call(hwnd)
	_, _, _ = procSetFocus.Call(hwnd)
	return true
}

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
