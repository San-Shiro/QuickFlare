//go:build !windows

package autostart

// Autostart is Windows-only for now. macOS wants a LaunchAgent plist and
// Linux an XDG .desktop entry in ~/.config/autostart; both are straightforward
// but neither is written yet. Supported() reports false so the UI can hide
// the setting rather than offer a switch that does nothing.

func Supported() bool { return false }

func Enabled() (bool, error) { return false, nil }

func Enable() error { return nil }

func Disable() error { return nil }

func Refresh() error { return nil }
