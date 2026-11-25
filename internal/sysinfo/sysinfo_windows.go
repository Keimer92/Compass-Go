//go:build windows

package sysinfo

import (
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"compass/internal/shell"
)

var (
	kernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
	procGetSystemTimes       = kernel32.NewProc("GetSystemTimes")
)

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

// cpuTimes son los contadores acumulados de GetSystemTimes (en unidades de
// 100 ns). kernel ya incluye el tiempo idle.
type cpuTimes struct {
	idle   uint64
	kernel uint64
	user   uint64
	at     time.Time
}

var (
	cpuMu     sync.Mutex
	cpuLast   cpuTimes
	cpuHas    bool
	cpuCached struct {
		usage float64
		at    time.Time
	}

	nameOnce sync.Once
	cpuName  string

	gpuMu      sync.Mutex
	gpuLoaded  bool
	gpuLoading bool
	gpuNames   []string
)

// Snapshot toma una lectura barata del sistema. La GPU se consulta una sola
// vez (es estatica) porque PowerShell tarda cientos de ms.
func Snapshot() Info {
	info := Info{
		CPU:   readCPU(),
		RAM:   readRAM(),
		GPU:   readGPU(),
		Disks: readDisks(),
	}
	return info
}

func readCPU() CPU {
	nameOnce.Do(func() {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE,
			`HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE)
		if err != nil {
			return
		}
		defer k.Close()
		if v, _, err := k.GetStringValue("ProcessorNameString"); err == nil {
			cpuName = strings.TrimSpace(v)
		}
	})
	return CPU{
		Name:  cpuName,
		Cores: runtime.NumCPU(),
		Usage: cpuUsage(),
	}
}

func readRAM() RAM {
	var m memoryStatusEx
	m.Length = uint32(unsafe.Sizeof(m))
	if r, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m))); r == 0 {
		return RAM{}
	}
	return RAM{Total: m.TotalPhys, Used: m.TotalPhys - m.AvailPhys}
}

func systemTimes() (cpuTimes, bool) {
	var idle, kernel, user windows.Filetime
	if r, _, _ := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idle)),
		uintptr(unsafe.Pointer(&kernel)),
		uintptr(unsafe.Pointer(&user)),
	); r == 0 {
		return cpuTimes{}, false
	}
	return cpuTimes{
		idle:   uint64(idle.HighDateTime)<<32 | uint64(idle.LowDateTime),
		kernel: uint64(kernel.HighDateTime)<<32 | uint64(kernel.LowDateTime),
		user:   uint64(user.HighDateTime)<<32 | uint64(user.LowDateTime),
		at:     time.Now(),
	}, true
}

// cpuUsage devuelve el uso desde la lectura anterior. La primera llamada solo
// toma la base y devuelve 0: dormir 200 ms aqui retrasaba el arranque de la
// app, y el siguiente poll (2 s) ya da el uso real.
func cpuUsage() float64 {
	cpuMu.Lock()
	defer cpuMu.Unlock()

	// Se reutiliza el valor 1 s para que varios clientes no deformen el delta.
	if cpuHas && time.Since(cpuCached.at) < time.Second {
		return cpuCached.usage
	}

	if !cpuHas {
		cur, ok := systemTimes()
		if !ok {
			return 0
		}
		cpuLast = cur
		cpuHas = true
		cpuCached.usage = 0
		cpuCached.at = time.Now()
		return 0
	}

	prev := cpuLast
	cur, ok := systemTimes()
	if !ok {
		return 0
	}
	cpuLast = cur

	dIdle := float64(cur.idle - prev.idle)
	dKernel := float64(cur.kernel - prev.kernel)
	dUser := float64(cur.user - prev.user)
	total := dKernel + dUser
	if total <= 0 || !ok {
		return 0
	}
	usage := (total - dIdle) / total * 100
	if usage < 0 {
		usage = 0
	}
	if usage > 100 {
		usage = 100
	}
	cpuCached.usage = usage
	cpuCached.at = time.Now()
	return usage
}

func readGPU() []string {
	gpuMu.Lock()
	if gpuLoaded {
		names := gpuNames
		gpuMu.Unlock()
		return names
	}
	if gpuLoading {
		gpuMu.Unlock()
		return nil
	}
	gpuLoading = true
	gpuMu.Unlock()

	// Consultar PowerShell tarda cientos de ms y arrancar una consola cuesta
	// CPU: se espera a que la app termine de abrir y se hace en segundo plano.
	// El primer snapshot sale sin GPU y un poll posterior ya la trae.
	go func() {
		time.Sleep(2 * time.Second)
		names, ok := queryGPU()
		gpuMu.Lock()
		gpuNames = names
		gpuLoaded = ok
		gpuLoading = false
		gpuMu.Unlock()
	}()
	return nil
}

func queryGPU() ([]string, bool) {
	out, err := shell.RunPS(
		"Get-CimInstance Win32_VideoController | Where-Object { $_.Name } | Select-Object -ExpandProperty Name")
	if err != nil {
		return nil, false
	}
	names := []string{}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		name := strings.TrimSpace(line)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names, true
}

func readDisks() []Disk {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil
	}
	out := make([]Disk, 0, 4)
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		root := string(rune('A'+i)) + `:\`
		ptr, err := windows.UTF16PtrFromString(root)
		if err != nil {
			continue
		}
		// 2 = extraible, 3 = fijo. Se omiten CD-ROM, red y ramdisk.
		kind := windows.GetDriveType(ptr)
		if kind != windows.DRIVE_FIXED && kind != windows.DRIVE_REMOVABLE {
			continue
		}
		var freeAvail, total, free uint64
		if err := windows.GetDiskFreeSpaceEx(ptr, &freeAvail, &total, &free); err != nil || total == 0 {
			continue
		}
		out = append(out, Disk{
			Letter: root[:2],
			Label:  volumeLabel(ptr),
			Total:  total,
			Free:   free,
		})
	}
	return out
}

func volumeLabel(root *uint16) string {
	buf := make([]uint16, windows.MAX_PATH+1)
	if err := windows.GetVolumeInformation(root, &buf[0], uint32(len(buf)),
		nil, nil, nil, nil, 0); err != nil {
		return ""
	}
	return strings.TrimSpace(windows.UTF16ToString(buf))
}
