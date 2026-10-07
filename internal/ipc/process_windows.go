//go:build windows

package ipc

import (
	"syscall"

	"golang.org/x/sys/windows"
)

func isWindowsProcessAlive(pid int) bool {
	const desiredAccess = windows.PROCESS_QUERY_LIMITED_INFORMATION | windows.SYNCHRONIZE
	h, err := windows.OpenProcess(desiredAccess, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)

	ev, err := windows.WaitForSingleObject(h, 0)
	if err != nil {
		return false
	}
	return ev == uint32(windows.WAIT_TIMEOUT)
}

func syscallSignalZero() syscall.Signal {
	return syscall.Signal(0)
}
