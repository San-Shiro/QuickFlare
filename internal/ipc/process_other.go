//go:build !windows

package ipc

import (
	"syscall"
)

func isWindowsProcessAlive(pid int) bool {
	return false
}

func syscallSignalZero() syscall.Signal {
	return syscall.Signal(0)
}
