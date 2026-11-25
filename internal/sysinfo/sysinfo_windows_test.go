//go:build windows

package sysinfo

import (
	"testing"
	"time"
)

func TestSnapshotWindows(t *testing.T) {
	info := Snapshot()
	// La GPU se consulta en segundo plano: el primer snapshot puede no traerla.
	deadline := time.Now().Add(6 * time.Second)
	for len(info.GPU) == 0 && time.Now().Before(deadline) {
		time.Sleep(300 * time.Millisecond)
		info = Snapshot()
	}
	t.Logf("cpu=%q cores=%d usage=%.1f%% ram=%d/%d gpu=%v disks=%d",
		info.CPU.Name, info.CPU.Cores, info.CPU.Usage, info.RAM.Used, info.RAM.Total, info.GPU, len(info.Disks))
	for _, d := range info.Disks {
		t.Logf("disk %s %q free=%d total=%d", d.Letter, d.Label, d.Free, d.Total)
	}

	if info.CPU.Cores == 0 {
		t.Error("cores = 0")
	}
	if info.CPU.Usage < 0 || info.CPU.Usage > 100 {
		t.Errorf("usage fuera de rango: %v", info.CPU.Usage)
	}
	if info.RAM.Total == 0 {
		t.Error("RAM total = 0")
	}
	if info.RAM.Used > info.RAM.Total {
		t.Errorf("RAM usada (%d) > total (%d)", info.RAM.Used, info.RAM.Total)
	}
	if len(info.Disks) == 0 {
		t.Error("sin discos fijos/extraibles")
	}
	for _, d := range info.Disks {
		if d.Total == 0 || d.Free > d.Total {
			t.Errorf("disco %s invalido: free=%d total=%d", d.Letter, d.Free, d.Total)
		}
	}
}
