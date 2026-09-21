//go:build windows

// Package ui - Windows window placement.
//
// Gio has no window-positioning API, so we take the HWND it publishes via
// app.Win32ViewEvent and place the panel with user32!SetWindowPos directly.
// All of this is plain syscall, so the build stays CGO-free on Windows.
package ui

import (
	"fmt"
	"unsafe"

	"gioui.org/app"
	"gioui.org/io/event"
	"golang.org/x/sys/windows"
)

var (
	dwmapi                    = windows.NewLazySystemDLL("dwmapi.dll")
	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")

	user32              = windows.NewLazySystemDLL("user32.dll")
	procGetCursorPos    = user32.NewProc("GetCursorPos")
	procMonitorFromPt   = user32.NewProc("MonitorFromPoint")
	procGetMonitorInfo  = user32.NewProc("GetMonitorInfoW")
	procSetWindowPos    = user32.NewProc("SetWindowPos")
	procGetWindowRect   = user32.NewProc("GetWindowRect")
	procSetForeground   = user32.NewProc("SetForegroundWindow")
	procShowWindowAsync = user32.NewProc("ShowWindowAsync")
	procGetWindowLong   = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLong   = user32.NewProc("SetWindowLongPtrW")
	procSetLayeredAttrs = user32.NewProc("SetLayeredWindowAttributes")
	procGetForeground   = user32.NewProc("GetForegroundWindow")
)

type point struct{ X, Y int32 }

type rect struct{ Left, Top, Right, Bottom int32 }

func (r rect) width() int32  { return r.Right - r.Left }
func (r rect) height() int32 { return r.Bottom - r.Top }

type monitorInfo struct {
	cbSize    uint32
	rcMonitor rect
	rcWork    rect
	dwFlags   uint32
}

const (
	monitorDefaultToNearest = 2

	swpNoSize     = 0x0001
	swpNoZOrder   = 0x0004
	swpNoActivate = 0x0010
	swpShowWindow = 0x0040
	// swpAsyncWindowPos posts the request to the owning thread instead of
	// sending it. Without this, a cross-thread SetWindowPos blocks until that
	// thread pumps - which deadlocks when we move the panel from the tray
	// goroutine while Gio owns the window.
	swpAsyncWindowPos = 0x4000

	swHide           = 0
	swShow           = 5
	swShowNoActivate = 4

	// Ex-style bits. Gio creates its window WS_EX_APPWINDOW, which forces a
	// taskbar button; a tray popover must not have one. WS_EX_TOOLWINDOW
	// removes it and also keeps the panel out of Alt-Tab.
	gwlExStyle     = ^uintptr(19) // GWL_EXSTYLE (-20)
	wsExAppWindow  = 0x00040000
	wsExToolWindow = 0x00000080

	// WS_EX_LAYERED lets SetLayeredWindowAttributes set whole-window opacity,
	// which is how the panel fades. This is safe with Gio's renderer only
	// because its D3D11 swap chain is the bitblt model
	// (DXGI_SWAP_EFFECT_DISCARD, BufferCount 1). A flip-model swap chain
	// cannot present to a layered window and would render black - so if Gio
	// ever switches, the fade has to go, not the check.
	wsExLayered = 0x00080000
	lwaAlpha    = 0x00000002

	// Gap between the panel and the screen edges, in physical pixels.
	edgeMargin = 12

	// DWM attributes, Windows 11 (build 22000+). Older Windows rejects both
	// with E_INVALIDARG, which is why StyleWindow ignores the result.
	dwmwaWindowCornerPreference = 33
	dwmwaBorderColor            = 34

	dwmwcpRound = 2 // DWMWCP_ROUND

	// DWMWA_COLOR_NONE. Windows 11 draws a 1px border on every top-level
	// window; this is the documented way to say "no border at all".
	dwmwaColorNone = 0xFFFFFFFE
)

// workAreaAtCursor returns the usable desktop rect (excluding the taskbar) of
// the monitor under the mouse. Because the user reaches the panel by clicking
// the tray icon, the cursor is over the tray - so this resolves to the monitor
// that actually owns the tray, which is what makes multi-monitor behave.
func workAreaAtCursor() (rect, bool) {
	var pt point
	if r, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt))); r == 0 {
		return rect{}, false
	}
	// MonitorFromPoint takes POINT by value, which on amd64 packs into one word.
	mon, _, _ := procMonitorFromPt.Call(
		uintptr(uint32(pt.X))|uintptr(uint32(pt.Y))<<32,
		monitorDefaultToNearest,
	)
	if mon == 0 {
		return rect{}, false
	}
	mi := monitorInfo{cbSize: uint32(unsafe.Sizeof(monitorInfo{}))}
	if r, _, _ := procGetMonitorInfo.Call(mon, uintptr(unsafe.Pointer(&mi))); r == 0 {
		return rect{}, false
	}
	return mi.rcWork, true
}

// windowSize reads the panel's real pixel size. We cannot compute this from the
// Dp we asked for, because the DPI scale is decided by the monitor.
func windowSize(hwnd uintptr) (int32, int32, bool) {
	r, ok := windowRect(hwnd)
	if !ok {
		return 0, 0, false
	}
	return r.width(), r.height(), true
}

func windowRect(hwnd uintptr) (rect, bool) {
	var r rect
	if ret, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r))); ret == 0 {
		return rect{}, false
	}
	return r, true
}

// AnchorToTray places the panel near the tray, clamped so it can never hang off
// the monitor edge. A borderless window pushed off-screen cannot be dragged
// back, so the clamp is load-bearing, not cosmetic.
func AnchorToTray(hwnd uintptr) {
	if hwnd == 0 {
		dbg("AnchorToTray called with nil hwnd")
		return
	}
	work, ok := workAreaAtCursor()
	if !ok {
		dbg("workAreaAtCursor failed")
		return
	}
	w, h, ok := windowSize(hwnd)
	if !ok {
		dbg("windowSize failed for hwnd=%#x", hwnd)
		return
	}
	dbg("hwnd=%#x work=%+v winsize=%dx%d", hwnd, work, w, h)

	// Anchor bottom-right of the work area; the taskbar is already excluded, so
	// this sits just above it regardless of which edge the taskbar lives on.
	x := work.Right - w - edgeMargin
	y := work.Bottom - h - edgeMargin

	if x < work.Left+edgeMargin {
		x = work.Left + edgeMargin
	}
	if y < work.Top+edgeMargin {
		y = work.Top + edgeMargin
	}

	dbg("calling SetWindowPos x=%d y=%d", x, y)
	ret, _, lastErr := procSetWindowPos.Call(hwnd, 0, uintptr(x), uintptr(y), 0, 0,
		swpNoSize|swpNoZOrder|swpNoActivate|swpShowWindow|swpAsyncWindowPos)
	dbg("SetWindowPos -> (%d,%d) ret=%d err=%v", x, y, ret, lastErr)
}

// Focus brings the panel forward so keystrokes land in it.
func Focus(hwnd uintptr) {
	if hwnd != 0 {
		procSetForeground.Call(hwnd)
	}
}

// Show makes the panel visible without stealing focus; Hide removes it. Gio
// has no show/hide of its own, so like placement this goes through the HWND.
//
// ShowWindowAsync, not ShowWindow: the plain call SENDS WM_SHOWWINDOW to the
// window's owning thread and blocks until that thread handles it. Every call
// here is cross-thread, and blocking one of our goroutines on Gio's message
// pump deadlocks the pump itself - the window then never moves, never hides,
// and never repaints. The Async form posts instead, so nothing blocks.
func Show(hwnd uintptr) {
	if hwnd != 0 {
		// SW_SHOW, not SW_SHOWNOACTIVATE: an unactivated window does not get
		// keyboard focus, so text fields in the panel would be dead.
		ret, _, err := procShowWindowAsync.Call(hwnd, swShow)
		dbg("ShowWindowAsync(show) ret=%d err=%v", ret, err)
	}
}

func Hide(hwnd uintptr) {
	if hwnd != 0 {
		ret, _, err := procShowWindowAsync.Call(hwnd, swHide)
		dbg("ShowWindowAsync(hide) ret=%d err=%v", ret, err)
	}
}

// ViewHandle extracts the native window handle from the platform's view event.
// Gio publishes it once the window exists; everything above needs it.
func ViewHandle(e event.Event) (uintptr, bool) {
	if ve, ok := e.(app.Win32ViewEvent); ok {
		return ve.HWND, true
	}
	return 0, false
}

// rectString renders the window's current rect, for tracing.
func rectString(hwnd uintptr) string {
	r, ok := windowRect(hwnd)
	if !ok {
		return "<unavailable>"
	}
	return fmt.Sprintf("(%d,%d)-(%d,%d)", r.Left, r.Top, r.Right, r.Bottom)
}

// StyleWindow removes the window border and hands corner rounding to DWM.
//
// Both halves of this fix the same mismatch. The window is a plain rectangle
// as far as Win32 is concerned, so two separate things were rounding it:
//
//   - DWM rounds every top-level window on Windows 11, at its own radius,
//     and outlines it with a 1px border.
//   - The panel painted its background as a 12dp rounded rect of its own.
//
// The two radii do not agree, so the sliver between the app's rounded fill
// and DWM's rounded clip was never painted by anything - which is what made
// the corners look chewed - and DWM's border traced the window's shape rather
// than the one the panel had drawn.
//
// So: turn the border off, keep DWM's rounding (it clips properly, including
// the shadow), and let the panel paint a flat rectangle underneath it. See
// Panel.layout. On Windows 10 neither attribute exists, both calls fail
// harmlessly, and the panel is a square-cornered rectangle - which is what a
// Windows 10 flyout looks like anyway.
func StyleWindow(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	set := func(attr uintptr, val uint32) {
		ret, _, err := procDwmSetWindowAttribute.Call(
			hwnd, attr,
			uintptr(unsafe.Pointer(&val)),
			unsafe.Sizeof(val),
		)
		dbg("DwmSetWindowAttribute(%d, %#x) hr=%#x err=%v", attr, val, ret, err)
	}
	set(dwmwaBorderColor, dwmwaColorNone)
	set(dwmwaWindowCornerPreference, dwmwcpRound)
}

// EnableFade makes the window layered so SetAlpha can drive its opacity.
//
// Like HideFromTaskbar this writes the ex-style, so it carries the same rule:
// never call it from the goroutine running Gio's event loop.
func EnableFade(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	cur, _, _ := procGetWindowLong.Call(hwnd, gwlExStyle)
	ret, _, err := procSetWindowLong.Call(hwnd, gwlExStyle, cur|wsExLayered)
	dbg("ex-style layered %#x -> %#x ret=%d err=%v", cur, cur|wsExLayered, ret, err)
	// Opaque to begin with: the first show sets this to 0 before revealing
	// the window, and a window that is layered but never given an alpha is
	// fully transparent on some drivers.
	SetAlpha(hwnd, 255)
}

// SetAlpha sets whole-window opacity, 0 transparent to 255 opaque. It is a
// no-op until EnableFade has run.
func SetAlpha(hwnd uintptr, a uint8) {
	if hwnd == 0 {
		return
	}
	procSetLayeredAttrs.Call(hwnd, 0, uintptr(a), lwaAlpha)
}

// Foreground is the window the user is currently working in, or 0 when
// nothing holds the foreground - a real transient state while windows are
// being switched, and not the same thing as "the panel lost focus".
func Foreground() uintptr {
	h, _, _ := procGetForeground.Call()
	return h
}

// HideFromTaskbar swaps the window from an app window to a tool window, which
// removes its taskbar button and takes it out of Alt-Tab.
//
// This only takes effect while the window is hidden, so it must be called
// after Hide and before the first Show.
func HideFromTaskbar(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	cur, _, _ := procGetWindowLong.Call(hwnd, gwlExStyle)
	next := (cur &^ wsExAppWindow) | wsExToolWindow
	ret, _, err := procSetWindowLong.Call(hwnd, gwlExStyle, next)
	dbg("ex-style %#x -> %#x ret=%d err=%v", cur, next, ret, err)
}
