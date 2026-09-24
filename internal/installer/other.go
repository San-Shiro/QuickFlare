//go:build !windows

package installer

import (
	"context"
	"fmt"
)

// IsInstalled returns false on non-windows platforms.
func IsInstalled() bool {
	return false
}

// Install returns an error on non-windows platforms as the installer is currently tailored for Windows.
func Install(ctx context.Context, opt Options, p ProgressFunc) error {
	return fmt.Errorf("installer is only supported on Windows")
}

// Uninstall returns an error on non-windows platforms.
func Uninstall(ctx context.Context, opt Options, p ProgressFunc) error {
	return fmt.Errorf("uninstaller is only supported on Windows")
}
