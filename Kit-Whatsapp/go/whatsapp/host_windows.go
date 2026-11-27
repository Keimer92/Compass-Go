//go:build windows

package whatsapp

import (
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

// hostJob se mantiene abierto durante toda la vida del host: cerrar el handle
// (o morir el proceso) mata al motor por JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE.
var hostJob windows.Handle

// assignToJob mete el motor en un job object para que no sobreviva al host.
// Se llama justo despues de cmd.Start (exec.Cmd no ofrece un hook previo).
func assignToJob(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return
	}
	proc, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		windows.CloseHandle(job)
		return
	}
	defer windows.CloseHandle(proc)
	if err := windows.AssignProcessToJobObject(job, proc); err != nil {
		windows.CloseHandle(job)
		return
	}
	hostJob = job
}
