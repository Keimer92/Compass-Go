//go:build windows

package shell

import (
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// countEarlierInstances cuenta los procesos vivos con el mismo nombre que este
// ejecutable cuyo PID es menor.
func countEarlierInstances() int {
	name := ownExeName()
	if name == "" {
		return 0
	}

	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(snap)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snap, &entry); err != nil {
		return 0
	}

	myself := uint32(os.Getpid())
	count := 0
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
