//go:build windows

package ipc

import (
	"syscall"

	"golang.org/x/sys/windows"
)

func isWindowsProcessAlive(pid int) bool {
	const PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
	h, err := windows.OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)

	var exitCode uint32
	if err := windows.GetExitCodeProcess(h, &exitCode); err != nil {
		return false
	}
	const STILL_ACTIVE = 259
	return exitCode == STILL_ACTIVE
}

func syscallSignalZero() syscall.Signal {
	return syscall.Signal(0)
}
