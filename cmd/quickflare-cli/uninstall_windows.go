//go:build windows

package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/sys/windows/registry"

	"github.com/San-Shiro/QuickFlare/internal/ipc"
)

func cmdUninstall(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("uninstall", flag.ExitOnError)
	yes := fs.Bool("y", false, "confirm uninstallation without interactive prompt")
	fs.BoolVar(yes, "yes", false, "confirm uninstallation without interactive prompt")
	fs.Parse(args)

	if !*yes {
		fmt.Print("Are you sure you want to uninstall QuickFlare? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read confirmation: %w", err)
		}
		input = strings.TrimSpace(input)
		if !strings.EqualFold(input, "y") && !strings.EqualFold(input, "yes") {
			fmt.Println("Uninstallation cancelled.")
			return nil
		}
	}

	// 1. Stop running tray application if active
	client, err := ipc.Discover()
	if err == nil && client != nil {
		fmt.Printf("Stopping running QuickFlare tray (PID %d)...\n", client.PID())
		_ = client.Quit(ctx)
		time.Sleep(500 * time.Millisecond)
	}

	// 2. Check for Windows Installer ProductCode in registry
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\QuickFlare`, registry.QUERY_VALUE)
	var prodCode string
	if err == nil {
		prodCode, _, _ = k.GetStringValue("ProductCode")
		k.Close()
	}

	if prodCode != "" {
		fmt.Printf("Launching Windows uninstaller (%s)...\n", prodCode)
		cmd := exec.Command("msiexec.exe", "/x", prodCode)
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("launch msiexec: %w", err)
		}
		fmt.Println("Windows uninstaller launched. Exiting QuickFlare...")
		return nil
	}

	// 3. Fallback for portable / non-MSI installs: clean PATH and registry
	fmt.Println("Windows Installer product code not registered. Performing direct cleanup...")
	_ = cmdPath(ctx, []string{"remove"})

	_ = registry.DeleteKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\App Paths\quickflare.exe`)
	_ = registry.DeleteKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\App Paths\quickflare-tray.exe`)
	_ = registry.DeleteKey(registry.CURRENT_USER, `Software\QuickFlare`)

	selfPath, _ := os.Executable()
	fmt.Printf("QuickFlare PATH and registry associations removed.\nYou may now delete the binary at: %s\n", selfPath)
	return nil
}
