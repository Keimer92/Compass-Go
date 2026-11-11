//go:build windows

// Package clipboard escribe texto en el portapapeles de Windows.
package clipboard

import (
	"fmt"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procOpenClipboard    = user32.NewProc("OpenClipboard")
	procCloseClipboard   = user32.NewProc("CloseClipboard")
	procEmptyClipboard   = user32.NewProc("EmptyClipboard")
	procSetClipboardData = user32.NewProc("SetClipboardData")
	procGlobalAlloc      = kernel32.NewProc("GlobalAlloc")
	procGlobalLock       = kernel32.NewProc("GlobalLock")
	procGlobalUnlock     = kernel32.NewProc("GlobalUnlock")
	procGlobalFree       = kernel32.NewProc("GlobalFree")
	procRtlMoveMemory    = kernel32.NewProc("RtlMoveMemory")
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

// Write copia text al portapapeles como CF_UNICODETEXT.
//
// La memoria se le asigna al portapapeles, asi que sobrevive al proceso: no se
// libera si SetClipboardData la acepta.
func Write(text string) error {
	buf := append(utf16.Encode([]rune(text)), 0)

	h, _, err := procGlobalAlloc.Call(gmemMoveable, uintptr(len(buf))*2)
	if h == 0 {
		return fmt.Errorf("no se pudo reservar memoria para el portapapeles: %v", err)
	}
	ptr, _, _ := procGlobalLock.Call(h)
	if ptr == 0 {
		procGlobalFree.Call(h)
		return fmt.Errorf("no se pudo bloquear la memoria del portapapeles")
	}
	// Se copia con RtlMoveMemory: la memoria del portapapeles no es de Go.
	procRtlMoveMemory.Call(ptr, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))*2)
	procGlobalUnlock.Call(h)

	if err := open(); err != nil {
		procGlobalFree.Call(h)
		return err
	}
	defer procCloseClipboard.Call()

	procEmptyClipboard.Call()
	if ok, _, err := procSetClipboardData.Call(cfUnicodeText, h); ok == 0 {
		procGlobalFree.Call(h)
		return fmt.Errorf("no se pudo dejar el texto en el portapapeles: %v", err)
	}
	return nil
}

// open abre el portapapeles reintentando: otro proceso puede tenerlo tomado un
// instante.
func open() error {
	var err error
	for i := 0; i < 5; i++ {
		var ok uintptr
		ok, _, err = procOpenClipboard.Call(0)
		if ok != 0 {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("el portapapeles esta ocupado: %v", err)
}
