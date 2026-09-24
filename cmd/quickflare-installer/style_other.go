//go:build !windows

package main

import "gioui.org/io/event"

func ViewHandle(e event.Event) (uintptr, bool) {
	return 0, false
}

func StyleInstallerWindow(hwnd uintptr) {}
