//go:build windows

package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/windows/registry"

	"github.com/San-Shiro/QuickFlare/internal/config"
	"github.com/San-Shiro/QuickFlare/internal/core"
	"github.com/San-Shiro/QuickFlare/internal/ipc"
)

// getInstalledProductInfo reads the MSI ProductCode and InstallDir from HKCU\Software\QuickFlare.
func getInstalledProductInfo() (string, string) {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\QuickFlare`, registry.QUERY_VALUE)
	if err != nil {
		return "", ""
	}
	defer k.Close()
	prodCode, _, _ := k.GetStringValue("ProductCode")
	installDir, _, _ := k.GetStringValue("InstallDir")
	return prodCode, installDir
}

// unpublishAllActiveRoutes connects to Cloudflare and unpublishes each active route.
func unpublishAllActiveRoutes(ctx context.Context) error {
	s, err := open()
	if err != nil {
		return err
	}
	routes := s.routes()
	if len(routes) == 0 {
		return nil
	}
	if err := s.withTunnel(ctx); err != nil {
		return fmt.Errorf("connect to tunnel: %w", err)
	}

	for _, r := range routes {
		fmt.Printf("Unpublishing route %s from Cloudflare...\n", r.Hostname)
		problems := core.Unpublish(ctx, s.client, r.ZoneID, s.tunnelID, r.Hostname)
		for _, p := range problems {
			fmt.Fprintf(os.Stderr, "  %s: %s\n", r.Hostname, p)
		}
	}
	_ = s.saveRoutes(nil)
	notifyTrayReload(ctx)
	return nil
}

// findInstallerMSI searches common locations for the QuickFlare MSI installer.
func findInstallerMSI() string {
	candidates := []string{
		"QuickFlare-*.msi",
		"build/QuickFlare-*.msi",
		"../QuickFlare-*.msi",
	}
	if userProfile := os.Getenv("USERPROFILE"); userProfile != "" {
		candidates = append(candidates, filepath.Join(userProfile, "Downloads", "QuickFlare-*.msi"))
	}

	for _, pattern := range candidates {
		matches, err := filepath.Glob(pattern)
		if err == nil && len(matches) > 0 {
			sort.Slice(matches, func(i, j int) bool {
				fi, err1 := os.Stat(matches[i])
				fj, err2 := os.Stat(matches[j])
				if err1 == nil && err2 == nil {
					return fi.ModTime().After(fj.ModTime())
				}
				return matches[i] > matches[j]
			})
			return matches[0]
		}
	}
	return ""
}

// launchDetachedReinstall writes a temporary script in %TEMP% and runs it detached.
func launchDetachedReinstall(prodCode, msiPath string) error {
	tempDir := os.TempDir()
	batchPath := filepath.Join(tempDir, "quickflare-reinstall.cmd")

	script := fmt.Sprintf(`@echo off
setlocal
title QuickFlare Reinstaller
echo ======================================================
echo              QuickFlare Reinstall
echo ======================================================
echo Waiting for QuickFlare processes to close...
timeout /t 1 /nobreak >nul
taskkill /f /im quickflare.exe >nul 2>&1
taskkill /f /im quickflare-tray.exe >nul 2>&1
taskkill /f /im cloudflared.exe >nul 2>&1

if not "%[1]s"=="" (
    echo Uninstalling current version of QuickFlare...
    msiexec.exe /x %[1]s /qb
)

echo Reinstalling QuickFlare from %[2]s...
msiexec.exe /i "%[2]s" /qb

echo Reinstallation complete.
del "%%~f0" 2>nul
exit /b 0
`, prodCode, msiPath)

	if err := os.WriteFile(batchPath, []byte(script), 0700); err != nil {
		return fmt.Errorf("create reinstaller script: %w", err)
	}

	cmd := exec.Command("cmd.exe", "/c", "start", "", batchPath)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launch reinstaller: %w", err)
	}

	fmt.Println("Reinstallation helper launched in background. Exiting QuickFlare...")
	return nil
}

// cmdUninstall handles 'quickflare uninstall'.
func cmdUninstall(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("uninstall", flag.ExitOnError)
	yes := fs.Bool("y", false, "confirm uninstallation without interactive prompt")
	fs.BoolVar(yes, "yes", false, "confirm uninstallation without interactive prompt")
	keepRoutes := fs.Bool("keep-routes", false, "do not remove active Cloudflare routes")
	keepConfig := fs.Bool("keep-config", false, "preserve configuration and saved tokens")
	reinstall := fs.Bool("reinstall", false, "uninstall current version then reinstall")
	msiPath := fs.String("msi", "", "path to installer MSI for reinstall")
	fs.Parse(args)

	if *reinstall {
		var reinstallArgs []string
		if *yes {
			reinstallArgs = append(reinstallArgs, "-y")
		}
		if *keepRoutes {
			reinstallArgs = append(reinstallArgs, "--keep-routes")
		}
		if *keepConfig {
			reinstallArgs = append(reinstallArgs, "--keep-config")
		}
		if *msiPath != "" {
			reinstallArgs = append(reinstallArgs, "--msi", *msiPath)
		}
		return cmdReinstall(ctx, reinstallArgs)
	}

	reader := bufio.NewReader(os.Stdin)

	if !*yes {
		fmt.Print("Are you sure you want to uninstall QuickFlare? [y/N]: ")
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

	// 1. Ask about removing active Cloudflare routes (default: ENABLED)
	removeRoutes := !*keepRoutes
	cfg, _ := config.Load()
	if !*yes && !*keepRoutes {
		if cfg != nil && cfg.APIToken != "" && len(cfg.Routes) > 0 {
			fmt.Printf("\nFound %d active Cloudflare route(s).\n", len(cfg.Routes))
			fmt.Print("Remove active routes from Cloudflare? [Y/n]: ")
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)
			// Defaults to enabled (Y) unless user explicitly enters 'n' or 'no'
			if strings.EqualFold(input, "n") || strings.EqualFold(input, "no") {
				removeRoutes = false
			}
		} else {
			removeRoutes = false
		}
	}

	// 2. Ask about removing configuration files and saved tokens (default: ENABLED)
	removeConfig := !*keepConfig
	if !*yes && !*keepConfig {
		fmt.Print("Remove configuration files and saved tokens? [Y/n]: ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if strings.EqualFold(input, "n") || strings.EqualFold(input, "no") {
			removeConfig = false
		}
	}

	// 3. Stop running tray application if active
	client, err := ipc.Discover()
	if err == nil && client != nil {
		fmt.Printf("Stopping running QuickFlare tray (PID %d)...\n", client.PID())
		_ = client.Quit(ctx)
		time.Sleep(500 * time.Millisecond)
	}

	// 4. Gracefully remove active routes from Cloudflare if enabled
	if removeRoutes {
		fmt.Println("Gracefully removing active routes from Cloudflare...")
		if err := unpublishAllActiveRoutes(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "warning: unpublish routes from Cloudflare: %v\n", err)
		} else {
			fmt.Println("Active Cloudflare routes removed.")
		}
	}

	// 5. Remove configuration directory if enabled
	if removeConfig {
		configDir, err := config.Dir()
		if err == nil && configDir != "" {
			_ = os.RemoveAll(configDir)
			fmt.Println("Configuration and saved credentials removed.")
		}
	}

	// 6. Clean any QuickFlare entries from user PATH
	selfPath, _ := os.Executable()
	selfDir := filepath.Dir(selfPath)
	prodCode, installDir := getInstalledProductInfo()
	cleanPathEntries(selfDir, installDir, filepath.Join(os.Getenv("LOCALAPPDATA"), "QuickFlare"))

	// 7. Check for Windows Installer ProductCode in registry
	if prodCode != "" {
		fmt.Printf("Launching Windows uninstaller (%s)...\n", prodCode)
		cmd := exec.Command("msiexec.exe", "/x", prodCode, "/qb")
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("launch msiexec: %w", err)
		}
		fmt.Println("Windows uninstaller launched. Exiting QuickFlare...")
		return nil
	}

	// Fallback for portable / non-MSI installs: clean registry
	fmt.Println("Windows Installer product code not registered. Performing direct cleanup...")
	_ = registry.DeleteKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\App Paths\quickflare.exe`)
	_ = registry.DeleteKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\App Paths\quickflare-tray.exe`)
	_ = registry.DeleteKey(registry.CURRENT_USER, `Software\QuickFlare`)

	fmt.Printf("QuickFlare PATH and registry associations removed.\nYou may now delete the binary at: %s\n", selfPath)
	return nil
}

func cleanPathEntries(dirs ...string) {
	k, err := registry.OpenKey(registry.CURRENT_USER, "Environment", registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()

	curPath, _, err := k.GetStringValue("Path")
	if err != nil {
		return
	}

	parts := strings.Split(curPath, ";")
	var remaining []string
	changed := false
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		remove := false
		for _, d := range dirs {
			if d != "" && strings.EqualFold(trimmed, strings.TrimRight(d, `\/`)) {
				remove = true
				changed = true
				break
			}
		}
		if !remove {
			remaining = append(remaining, trimmed)
		}
	}

	if changed {
		newVal := strings.Join(remaining, ";")
		_ = k.SetStringValue("Path", newVal)
		broadcastSettingChange()
	}
}

// cmdReinstall handles 'quickflare reinstall'.
func cmdReinstall(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("reinstall", flag.ExitOnError)
	yes := fs.Bool("y", false, "confirm uninstallation and reinstallation without interactive prompt")
	fs.BoolVar(yes, "yes", false, "confirm without interactive prompt")
	keepRoutes := fs.Bool("keep-routes", false, "do not remove active Cloudflare routes")
	keepConfig := fs.Bool("keep-config", false, "preserve configuration and saved credentials")
	msiPath := fs.String("msi", "", "path to QuickFlare MSI installer")
	pos := parseFlags(fs, args)

	targetMSI := *msiPath
	if targetMSI == "" && len(pos) > 0 {
		targetMSI = pos[0]
	}
	if targetMSI == "" {
		targetMSI = findInstallerMSI()
	}

	prodCode, installDir := getInstalledProductInfo()
	reader := bufio.NewReader(os.Stdin)

	if prodCode != "" || installDir != "" {
		fmt.Println("QuickFlare installation detected.")
		if !*yes {
			fmt.Print("Uninstall current version and reinstall? [Y/n]: ")
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)
			if strings.EqualFold(input, "n") || strings.EqualFold(input, "no") {
				fmt.Println("Reinstallation cancelled.")
				return nil
			}
		}

		// Active routes prompt (default: enabled)
		removeRoutes := !*keepRoutes
		cfg, _ := config.Load()
		if !*yes && !*keepRoutes {
			if cfg != nil && cfg.APIToken != "" && len(cfg.Routes) > 0 {
				fmt.Printf("\nFound %d active Cloudflare route(s).\n", len(cfg.Routes))
				fmt.Print("Remove active routes from Cloudflare? [Y/n]: ")
				input, _ := reader.ReadString('\n')
				input = strings.TrimSpace(input)
				if strings.EqualFold(input, "n") || strings.EqualFold(input, "no") {
					removeRoutes = false
				}
			} else {
				removeRoutes = false
			}
		}

		// Stop running tray application
		client, err := ipc.Discover()
		if err == nil && client != nil {
			fmt.Printf("Stopping running QuickFlare tray (PID %d)...\n", client.PID())
			_ = client.Quit(ctx)
			time.Sleep(500 * time.Millisecond)
		}

		// Gracefully remove routes if enabled
		if removeRoutes {
			fmt.Println("Gracefully removing active routes from Cloudflare...")
			if err := unpublishAllActiveRoutes(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "warning: remove routes: %v\n", err)
			} else {
				fmt.Println("Active Cloudflare routes removed.")
			}
		}

		// Config removal if requested
		if !*keepConfig && *yes {
			// keep configs by default during interactive reinstall so credentials aren't lost
		}
	} else {
		fmt.Println("No existing QuickFlare installation detected. Performing fresh install...")
	}

	if targetMSI == "" {
		return fmt.Errorf("no QuickFlare MSI installer found; please specify --msi <path>")
	}

	absMSI, err := filepath.Abs(targetMSI)
	if err != nil {
		absMSI = targetMSI
	}

	fmt.Printf("Using installer: %s\n", absMSI)
	return launchDetachedReinstall(prodCode, absMSI)
}

// cmdInstall handles 'quickflare install'.
// If QuickFlare is currently installed, it uninstalls the current version first, then reinstalls.
func cmdInstall(ctx context.Context, args []string) error {
	prodCode, installDir := getInstalledProductInfo()
	if prodCode != "" || installDir != "" {
		fmt.Println("QuickFlare is currently installed. Running uninstaller for current version, then reinstalling...")
		return cmdReinstall(ctx, args)
	}

	// Fresh install
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	msiPath := fs.String("msi", "", "path to QuickFlare MSI installer")
	pos := parseFlags(fs, args)

	targetMSI := *msiPath
	if targetMSI == "" && len(pos) > 0 {
		targetMSI = pos[0]
	}
	if targetMSI == "" {
		targetMSI = findInstallerMSI()
	}
	if targetMSI == "" {
		return fmt.Errorf("no QuickFlare MSI installer found; please specify --msi <path>")
	}

	absMSI, err := filepath.Abs(targetMSI)
	if err != nil {
		absMSI = targetMSI
	}

	fmt.Printf("Launching QuickFlare installer (%s)...\n", absMSI)
	cmd := exec.Command("msiexec.exe", "/i", absMSI)
	return cmd.Start()
}
