//go:build windows

// Package autostart controls whether QuickFlare launches when the user logs in.
package autostart

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// runKey is the per-user startup list.
//
// HKEY_CURRENT_USER, not HKEY_LOCAL_MACHINE: this is one user choosing to
// start their own tray app, so it needs no administrator rights and cannot
// affect anyone else on the machine. The Startup folder would work too, but
// means generating a .lnk; a registry value is a string.
const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

// valueName is how the entry appears in Task Manager's Startup tab.
const valueName = "QuickFlare"

// Supported reports whether this platform can manage autostart.
func Supported() bool { return true }

// exePath is the command line written to the registry.
//
// Quoted deliberately: Windows splits an unquoted Run value on spaces, so an
// executable under "Program Files" - or any path with a space in it - would
// otherwise be read as a different program plus arguments, and silently fail
// to start.
func exePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return `"` + exe + `"`, nil
}

// Enabled reports whether QuickFlare is set to start at login.
func Enabled() (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("open startup key: %w", err)
	}
	defer k.Close()

	v, _, err := k.GetStringValue(valueName)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read startup entry: %w", err)
	}
	return strings.TrimSpace(v) != "", nil
}

// Enable adds or refreshes the startup entry.
func Enable() error {
	path, err := exePath()
	if err != nil {
		return err
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open startup key: %w", err)
	}
	defer k.Close()

	if err := k.SetStringValue(valueName, path); err != nil {
		return fmt.Errorf("write startup entry: %w", err)
	}
	return nil
}

// Disable removes the startup entry. Removing one that is not there is not an
// error - the caller asked for it to be off, and it is.
func Disable() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("open startup key: %w", err)
	}
	defer k.Close()

	if err := k.DeleteValue(valueName); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("remove startup entry: %w", err)
	}
	return nil
}

// Refresh repoints an existing entry at the current executable.
//
// A stale path is the usual way autostart quietly breaks: the app is moved,
// or replaced by a new build somewhere else, and the entry keeps naming a
// file that no longer exists. Nothing reports this - it simply stops starting
// - so it is checked on every launch while the setting is on.
func Refresh() error {
	on, err := Enabled()
	if err != nil || !on {
		return err
	}
	want, err := exePath()
	if err != nil {
		return err
	}

	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open startup key: %w", err)
	}
	defer k.Close()

	got, _, err := k.GetStringValue(valueName)
	if err != nil {
		return fmt.Errorf("read startup entry: %w", err)
	}
	if strings.EqualFold(strings.TrimSpace(got), want) {
		return nil
	}
	return k.SetStringValue(valueName, want)
}
