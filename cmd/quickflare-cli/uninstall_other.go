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

func cmdReinstall(ctx context.Context, args []string) error {
	fmt.Println("To reinstall QuickFlare on Linux:")
	fmt.Println("  Debian/Ubuntu: sudo apt install --reinstall quickflare")
	fmt.Println("  Fedora/RHEL:   sudo dnf reinstall quickflare")
	return nil
}

func cmdInstall(ctx context.Context, args []string) error {
	fmt.Println("To install QuickFlare on Linux:")
	fmt.Println("  Debian/Ubuntu: sudo apt install ./quickflare_*.deb")
	fmt.Println("  Fedora/RHEL:   sudo dnf install ./quickflare-*.rpm")
	return nil
}
