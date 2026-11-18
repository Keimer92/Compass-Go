//go:build windows

// Package console protege el cierre de la ventana anfitriona mientras hay
// pestanas internas abiertas. Es el equivalente de src/utils/console_guard.rs.
//
//   - conhost: mientras hay pestanas, el elemento SC_CLOSE del menu del sistema
//     se deshabilita, asi que la × no cierra la ventana; la salida pasa por el
//     aviso del menu (Esc → aviso → Enter).
//   - Windows Terminal: no hay forma de vetar el cierre de una pestana desde la
//     app; aqui el manejador de consola mata los arboles de las pestanas antes de
//     que el sistema termine el proceso.
//
// Todo es best-effort: sin ventana de consola no se hace nada raro.
package console

import (
	"os/exec"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

const (
	ctrlCloseEvent    = 2
	ctrlLogoffEvent   = 5
	ctrlShutdownEvent = 6

	scClose     = 0xF060
	mfByCommand = 0x00000000
	mfEnabled   = 0x00000000
	mfGrayed    = 0x00000001
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	user32   = syscall.NewLazyDLL("user32.dll")

	procSetConsoleCtrlHandler = kernel32.NewProc("SetConsoleCtrlHandler")
	procGetConsoleWindow      = kernel32.NewProc("GetConsoleWindow")

	procGetSystemMenu  = user32.NewProc("GetSystemMenu")
	procEnableMenuItem = user32.NewProc("EnableMenuItem")
	procDrawMenuBar    = user32.NewProc("DrawMenuBar")
)

var (
	installed atomic.Bool
	openTabs  atomic.Int64

	pidsMu sync.Mutex
	pids   []int
)

// Install instala el manejador de cierre de consola. Idempotente.
func Install() {
	if installed.Swap(true) {
		return
	}
	procSetConsoleCtrlHandler.Call(handlerCallback, 1)
}

// SetOpenTabs sincroniza el numero de pestanas internas abiertas.
//
// Solo toca la ventana cuando se cruza de 0 a >0 (o al reves): llamar a
// EnableMenuItem en cada fotograma seria desperdicio.
func SetOpenTabs(n int) {
	prev := openTabs.Swap(int64(n))
	if !installed.Load() {
		return
	}
	if (prev == 0) != (n == 0) {
		setHostCloseEnabled(n == 0)
	}
}

// RegisterPane recuerda el PID del shell de una pestana recien abierta.
func RegisterPane(pid int) {
	if pid == 0 {
		return
	}
	pidsMu.Lock()
	defer pidsMu.Unlock()
	for _, p := range pids {
		if p == pid {
			return
		}
	}
	pids = append(pids, pid)
}

// UnregisterPane olvida una pestana cerrada por el usuario.
func UnregisterPane(pid int) {
	pidsMu.Lock()
	defer pidsMu.Unlock()
	out := pids[:0]
	for _, p := range pids {
		if p != pid {
			out = append(out, p)
		}
	}
	pids = out
}

var handlerCallback = syscall.NewCallback(func(ctrlType uint32) uintptr {
	closing := ctrlType == ctrlCloseEvent
	sessionEnd := ctrlType == ctrlLogoffEvent || ctrlType == ctrlShutdownEvent
	if !closing && !sessionEnd {
		return 0
	}
	if openTabs.Load() == 0 {
		return 0
	}
	// El host va a terminar este proceso: sin esto, cargo/gradlew/flutter
	// quedarian vivos escribiendo en una ConPTY que ya no existe.
	killPaneTrees()
	return 1
})

func killPaneTrees() {
	pidsMu.Lock()
	snapshot := append([]int(nil), pids...)
	pidsMu.Unlock()
	for _, pid := range snapshot {
		cmd := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F")
		// CREATE_NO_WINDOW: taskkill no debe abrir una consola visible.
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
		_ = cmd.Run()
	}
}

func setHostCloseEnabled(enabled bool) {
	hwnd, _, _ := procGetConsoleWindow.Call()
	if hwnd == 0 {
		return
	}
	menu, _, _ := procGetSystemMenu.Call(hwnd, 0)
	if menu == 0 {
		return
	}
	state := uintptr(mfEnabled)
	if !enabled {
		state = mfGrayed
	}
	procEnableMenuItem.Call(menu, scClose, uintptr(mfByCommand)|state)
	// El boton × solo refleja el cambio al repintar la barra de menus.
	procDrawMenuBar.Call(hwnd)
}

// Evita que el compilador se queje de unsafe si algun dia se usa un puntero.
var _ = unsafe.Pointer(nil)
