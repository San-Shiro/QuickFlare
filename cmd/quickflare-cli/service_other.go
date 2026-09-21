//go:build !linux

package main

import (
	"context"
	"fmt"
	"runtime"
)

// Service management is systemd, and systemd is Linux.
//
// This stub exists so the CLI compiles and can be developed from the Windows
// machine that builds the tray app. On Windows the tray is the only supported
// interface and it keeps the connector alive by staying open; macOS would
// want a launchd agent, which is the same shape as the systemd unit but is
// not written yet.
func cmdService(ctx context.Context, args []string) error {
	return fmt.Errorf("service management is Linux-only (this is %s); "+
		"use 'quickflare run' to keep the connector in the foreground", runtime.GOOS)
}
