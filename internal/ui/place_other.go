//go:build !windows

package ui

import "gioui.org/io/event"

// Placement is Windows-only for now. On other platforms the panel falls back to
// wherever the window manager puts it; see AnchorToTray in place_windows.go for
// the contract these need to satisfy when macOS/Linux are targeted.

func AnchorToTray(hwnd uintptr) {}

func Focus(hwnd uintptr) {}

func Show(hwnd uintptr) {}

func Hide(hwnd uintptr) {}

func ViewHandle(e event.Event) (uintptr, bool) { return 0, false }

func rectString(hwnd uintptr) string { return "<n/a>" }

func HideFromTaskbar(hwnd uintptr) {}

func StyleWindow(hwnd uintptr) {}

func EnableFade(hwnd uintptr) {}

func SetAlpha(hwnd uintptr, a uint8) {}

// Foreground returning 0 means "unknown", which the focus watcher reads as
// "do not act" - so the panel simply never auto-hides off Windows.
func Foreground() uintptr { return 0 }
