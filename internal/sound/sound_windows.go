//go:build windows

// Package sound emite los avisos sonoros de fin de build.
//
// Usa `Beep` de kernel32 (en Windows 7+ se redirige al dispositivo de audio
// predeterminado), asi que no depende de ficheros WAV ni de PowerShell y no
// anade latencia perceptible al cierre de la pestana.
package sound

import (
	"time"

	"golang.org/x/sys/windows"
)

var (
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	procBeep = kernel32.NewProc("Beep")
)

func beep(freq, ms uint32) {
	// Beep es sincrono: bloquea hasta que termina el tono.
	_, _, _ = procBeep.Call(uintptr(freq), uintptr(ms))
}

// Success suena una triada ascendente cuando el build termino bien.
func Success() {
	go func() {
		beep(880, 110)
		time.Sleep(20 * time.Millisecond)
		beep(1175, 110)
		time.Sleep(20 * time.Millisecond)
		beep(1568, 180)
	}()
}

// Failure suena dos tonos graves cuando el build fallo.
func Failure() {
	go func() {
		beep(440, 160)
		time.Sleep(30 * time.Millisecond)
		beep(311, 260)
	}()
}
