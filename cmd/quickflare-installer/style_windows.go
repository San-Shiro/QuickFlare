//go:build windows

package main

import (
	"sync"
	"unsafe"

	"gioui.org/app"
	"gioui.org/io/event"
	"golang.org/x/sys/windows"
)

var (
	dwmapi                    = windows.NewLazySystemDLL("dwmapi.dll")
	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")

	styleOnce sync.Once
)

// ViewHandle extracts the native window handle from Gio's Win32ViewEvent.
func ViewHandle(e event.Event) (uintptr, bool) {
	if ve, ok := e.(app.Win32ViewEvent); ok {
		return ve.HWND, true
	}
	return 0, false
}

// StyleInstallerWindow enables Windows 10/11 immersive dark mode title bar
// and rounded window corners.
func StyleInstallerWindow(hwnd uintptr) {
	if hwnd == 0 {
		return
	}

	styleOnce.Do(func() {
		set := func(attr uintptr, val uint32) {
			_, _, _ = procDwmSetWindowAttribute.Call(
				hwnd, attr,
				uintptr(unsafe.Pointer(&val)),
				unsafe.Sizeof(val),
			)
		}

		// DWMWA_USE_IMMERSIVE_DARK_MODE: 20 on Windows 11 / Windows 10 20H1+, 19 on older Win10
		dark := uint32(1)
		set(20, dark)
		set(19, dark)

		// DWMWA_WINDOW_CORNER_PREFERENCE (33) -> DWMWCP_ROUND (2)
		round := uint32(2)
		set(33, round)
	})
}
