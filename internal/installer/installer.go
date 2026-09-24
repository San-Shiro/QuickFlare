package installer

import (
	"os"
	"path/filepath"
)

// Version is the current application version.
const Version = "0.4.5"

// Options specifies what the installer or uninstaller should do.
type Options struct {
	InstallDir      string // Target folder (default: %LOCALAPPDATA%\QuickFlare)
	InstallTray     bool   // Whether to install quickflare-tray.exe (CLI is always installed)
	AddToPath       bool   // Register InstallDir into user PATH
	CreateShortcuts bool   // Create Start Menu shortcuts (if InstallTray is true)
	LaunchAfter     bool   // Launch QuickFlare tray app after installation completes
	RemoveRoutes    bool   // Unpublish active Cloudflare routes on uninstall
	RemoveConfig    bool   // Remove %APPDATA%\QuickFlare config directory on uninstall
}

// ProgressFunc reports installation steps and progress (0.0 to 1.0).
type ProgressFunc func(step string, progress float32)

// DefaultInstallDir returns the default per-user installation directory.
func DefaultInstallDir() string {
	if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
		return filepath.Join(localAppData, "QuickFlare")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "AppData", "Local", "QuickFlare")
}

// Payload contains the binary bytes to be written to disk.
type Payload struct {
	CliExe  []byte
	TrayExe []byte
}

// GlobalPayload can be populated by the entry point.
var GlobalPayload Payload
