//go:build !windows

package ui

// notify is a no-op on non-Windows platforms.
func notify(title, message string) {}
