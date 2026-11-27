//go:build windows

package whatsapp

import "golang.org/x/sys/windows"

// engineInstanceMutex se mantiene vivo durante todo el proceso: Windows libera
// el mutex con nombre cuando el proceso termina, así que no quedan candados
// huérfanos tras un cierre inesperado.
var engineInstanceMutex windows.Handle

// acquireEngineInstance evita que dos procesos de GymPro lancen el motor de
// WhatsApp a la vez sobre la misma carpeta de sesión. Dos motores con las
// mismas credenciales se pisan las claves Signal y provocan "Bad MAC" y envíos
// que se quedan colgados.
func acquireEngineInstance() (bool, error) {
	name, err := windows.UTF16PtrFromString(`Local\` + cfg.AppName + `-WhatsApp-Engine`)
	if err != nil {
		return false, err
	}
	handle, err := windows.CreateMutex(nil, false, name)
	if err != nil {
		return false, err
	}
	if err := windows.GetLastError(); err == windows.ERROR_ALREADY_EXISTS {
		windows.CloseHandle(handle)
		return false, nil
	}
	engineInstanceMutex = handle
	return true, nil
}
