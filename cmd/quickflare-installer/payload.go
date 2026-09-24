package main

import (
	"embed"
	"os"
	"path/filepath"

	"github.com/San-Shiro/QuickFlare/internal/installer"
)

//go:embed payload/*
var payloadFS embed.FS

// loadPayload populates installer.GlobalPayload with the CLI and Tray binaries.
// It checks the embedded filesystem first. If the embedded binaries are absent
// or empty (e.g. during development/testing), it inspects the local filesystem.
func loadPayload() error {
	// 1. Try embedded binaries
	if data, err := payloadFS.ReadFile("payload/quickflare.exe"); err == nil && len(data) > 0 {
		installer.GlobalPayload.CliExe = data
	}
	if data, err := payloadFS.ReadFile("payload/quickflare-tray.exe"); err == nil && len(data) > 0 {
		installer.GlobalPayload.TrayExe = data
	}

	// 2. Fallbacks for local dev / unbundled runs
	selfPath, _ := os.Executable()
	selfDir := filepath.Dir(selfPath)

	candidateDirs := []string{
		selfDir,
		filepath.Join(selfDir, "bin"),
		filepath.Join(selfDir, "build"),
		"build",
		filepath.Join("..", "build"),
		filepath.Join("..", "..", "build"),
	}

	if len(installer.GlobalPayload.CliExe) == 0 {
		for _, d := range candidateDirs {
			p := filepath.Join(d, "quickflare.exe")
			if data, err := os.ReadFile(p); err == nil && len(data) > 0 {
				installer.GlobalPayload.CliExe = data
				break
			}
		}
	}

	if len(installer.GlobalPayload.TrayExe) == 0 {
		for _, d := range candidateDirs {
			p := filepath.Join(d, "quickflare-tray.exe")
			if data, err := os.ReadFile(p); err == nil && len(data) > 0 {
				installer.GlobalPayload.TrayExe = data
				break
			}
		}
	}

	return nil
}
