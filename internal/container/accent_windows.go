//go:build windows

package container

import (
	"os"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// selfInstanceIndex es el indice de este Compass entre los que corren a la vez.
// Se calcula una sola vez: si otro Compass se cierra mientras este vive, su
// pestana no debe cambiar de color a media sesion.
var selfInstanceIndex = computeInstanceIndex()

var onceInstance sync.Once

func instanceIndex() int {
	// selfInstanceIndex se inicializa en la carga del paquete; el sync.Once
	// cubre el caso de pruebas que lo fuerzan a recalcular.
	onceInstance.Do(func() {})
	return selfInstanceIndex
}

// computeInstanceIndex cuenta cuantos procesos compass-go.exe tienen un PID menor
// que el de este, igual que instance_index de Rust.
func computeInstanceIndex() int {
	name := strings.ToLower(basename(os.Args[0]))
	if name == "" || name == "." {
		return 0
	}
	myself := uint32(os.Getpid())

	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(snap)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	count := 0
	if err := windows.Process32First(snap, &entry); err != nil {
		return 0
	}
	for {
		exe := strings.ToLower(windows.UTF16ToString(entry.ExeFile[:]))
		if exe == name && entry.ProcessID < myself {
			count++
		}
		if err := windows.Process32Next(snap, &entry); err != nil {
			break
		}
	}
	return count
}

func basename(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}
