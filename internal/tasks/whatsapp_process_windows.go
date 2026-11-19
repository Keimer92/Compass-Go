//go:build windows

package tasks

import (
	"fmt"
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// closeProjectWhatsappEngines cierra los motores de WhatsApp (node.exe) de este
// proyecto que estén corriendo antes de un build de Windows. Windows no deja
// sobrescribir un .exe en uso, y CMake copia node.exe al Release en cada build;
// si quedó un motor vivo de una prueba anterior, el install falla con
// "Permission denied" (envuelto en el críptico MSB3073 de MSBuild).
//
// Solo termina node.exe cuya ruta completa está bajo la carpeta del proyecto
// (ver isProjectWhatsappEnginePath). Nunca toca los node.exe del sistema ni los
// de otros proyectos. Devuelve cuántos cerró; si no puede cerrar alguno avisa.
func closeProjectWhatsappEngines(projectRoot string) int {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		fmt.Printf("[WARN] No se pudieron enumerar procesos: %v\n", err)
		return 0
	}
	defer windows.CloseHandle(snap)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snap, &entry); err != nil {
		return 0
	}

	self := uint32(os.Getpid())
	closed := 0
	failed := 0
	for {
		// ProcessEntry32.ExeFile es un [260]uint16 terminado en NUL.
		exe := strings.ToLower(windows.UTF16ToString(entry.ExeFile[:]))
		pid := entry.ProcessID
		if exe == "node.exe" && pid != self {
			path := processImagePath(pid)
			if path != "" && isProjectWhatsappEnginePath(path, projectRoot) {
				if err := terminateProcess(pid); err != nil {
					fmt.Printf("[WARN] No se pudo cerrar el motor de WhatsApp (PID %d): %v.\n", pid, err)
					fmt.Println("[WARN] Cierra la app que lo usa antes de recompilar.")
					failed++
				} else {
					fmt.Printf("[OK] Motor de WhatsApp cerrado (PID %d).\n", pid)
					closed++
				}
			}
		}
		if err := windows.Process32Next(snap, &entry); err != nil {
			break
		}
	}
	if closed > 0 {
		fmt.Printf("[OK] %d motor(es) de WhatsApp cerrados antes del build.\n", closed)
	}
	if failed > 0 {
		fmt.Printf("[WARN] %d motor(es) siguen en uso; el build podria fallar con Permission denied.\n", failed)
	}
	return closed
}

// processImagePath devuelve la ruta completa del ejecutable de un PID, o "" si
// no se puede consultar (el proceso ya salió o no hay permisos).
func processImagePath(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)

	buf := make([]uint16, windows.MAX_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:size])
}

// terminateProcess pide el cierre de un PID y espera (hasta 5 s) a que salga,
// para que Windows libere el archivo antes de que CMake intente copiarlo.
func terminateProcess(pid uint32) error {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	if err := windows.TerminateProcess(h, 1); err != nil {
		return err
	}
	// WaitForSingleObject devuelve WAIT_OBJECT_0 (0) cuando el proceso salió;
	// WAIT_TIMEOUT (258) significa que sigue vivo, pero el handle queda cerrado.
	_, _ = windows.WaitForSingleObject(h, 5000)
	return nil
}
