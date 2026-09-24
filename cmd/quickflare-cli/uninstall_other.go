//go:build !windows

package main

import (
	"context"
	"fmt"
)

func cmdUninstall(ctx context.Context, args []string) error {
	fmt.Println("To uninstall QuickFlare on Linux:")
	fmt.Println("  Debian/Ubuntu: sudo apt remove quickflare")
	fmt.Println("  Fedora/RHEL:   sudo dnf remove quickflare")
	fmt.Println("  Arch:          sudo pacman -R quickflare")
	fmt.Println("  Standalone:    rm /usr/local/bin/quickflare")
	return nil
}
