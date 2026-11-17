//go:build windows

// Package windialog muestra cuadros de mensaje nativos de Windows, para que
// los errores se vean aunque el ejecutable no tenga consola.
package windialog

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32         = windows.NewLazySystemDLL("user32.dll")
	kernel32       = windows.NewLazySystemDLL("kernel32.dll")
	procMessageBox = user32.NewProc("MessageBoxW")
	procGetConsole = kernel32.NewProc("GetConsoleWindow")
	procShowWindow = user32.NewProc("ShowWindow")
)

const (
	mbOK      = 0x00000000
	mbIconErr = 0x00000010
	mbTopmost = 0x00040000
)

// HideConsole oculta la ventana de consola del proceso actual. La consola
// sigue existiendo (stdout funciona para las tareas), solo no se ve.
func HideConsole() {
	hwnd, _, _ := procGetConsole.Call()
	if hwnd == 0 {
		return
	}
	// SW_HIDE = 0
	_, _, _ = procShowWindow.Call(hwnd, 0)
}

// Error muestra un cuadro de error modal.
func Error(title, text string) {
	t, errT := windows.UTF16PtrFromString(title)
	m, errM := windows.UTF16PtrFromString(text)
	if errT != nil || errM != nil {
		return
	}
	_, _, _ = procMessageBox.Call(
		0,
		uintptr(unsafe.Pointer(m)),
		uintptr(unsafe.Pointer(t)),
		uintptr(mbOK|mbIconErr|mbTopmost),
	)
}
