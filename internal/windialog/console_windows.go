//go:build windows

package windialog

import "os"

var (
	procAttachConsole = kernel32.NewProc("AttachConsole")
	procGetStdHandle  = kernel32.NewProc("GetStdHandle")
)

const attachParentProcess = uintptr(0xffffffff)

// AttachParentConsole se usa en las invocaciones --task: el exe se compila como
// app GUI (sin consola propia), pero al correr dentro de una ConPTY hay que
// enganchar la consola del padre y reapuntar los streams para que los builds
// sigan escribiendo su salida en el terminal de Compass.
func AttachParentConsole() {
	_, _, _ = procAttachConsole.Call(attachParentProcess)
	setStream(^uintptr(9), &os.Stdin)  // STD_INPUT_HANDLE  = -10
	setStream(^uintptr(10), &os.Stdout) // STD_OUTPUT_HANDLE = -11
	setStream(^uintptr(11), &os.Stderr) // STD_ERROR_HANDLE  = -12
}

func setStream(id uintptr, dst **os.File) {
	h, _, _ := procGetStdHandle.Call(id)
	if h == 0 || h == ^uintptr(0) {
		return
	}
	*dst = os.NewFile(h, "console")
}
