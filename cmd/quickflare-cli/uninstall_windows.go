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

	"golang.org/x/sys/windows/registry"

	"github.com/San-Shiro/QuickFlare/internal/config"
	"github.com/San-Shiro/QuickFlare/internal/installer"
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

// findInstaller searches common locations for QuickFlare-Setup.exe or MSI.
func findInstaller() string {
	candidates := []string{
		"QuickFlare-Setup.exe",
		"build/QuickFlare-Setup.exe",
		"../QuickFlare-Setup.exe",
		"QuickFlare-*.msi",
		"build/QuickFlare-*.msi",
		"../QuickFlare-*.msi",
	}
	if userProfile := os.Getenv("USERPROFILE"); userProfile != "" {
		candidates = append(candidates,
			filepath.Join(userProfile, "Downloads", "QuickFlare-Setup.exe"),
			filepath.Join(userProfile, "Downloads", "QuickFlare-*.msi"),
		)
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

// launchInstaller launches QuickFlare-Setup.exe or falls back to msiexec for MSI.
func launchInstaller(targetPath string) error {
	absPath, err := filepath.Abs(targetPath)
	if err != nil {
		absPath = targetPath
	}

	if strings.HasSuffix(strings.ToLower(absPath), ".msi") {
		fmt.Printf("Launching Windows Installer (%s)...\n", absPath)
		cmd := exec.Command("msiexec.exe", "/i", absPath)
		return cmd.Start()
	}

	fmt.Printf("Launching QuickFlare Setup (%s)...\n", absPath)
	cmd := exec.Command(absPath)
	return cmd.Start()
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

	_, installDir := getInstalledProductInfo()
	if installDir == "" {
		installDir = installer.DefaultInstallDir()
	}

	opt := installer.Options{
		InstallDir:   installDir,
		RemoveRoutes: removeRoutes,
		RemoveConfig: removeConfig,
	}

	fmt.Println("\nStarting QuickFlare uninstallation...")
	err := installer.Uninstall(ctx, opt, func(step string, prog float32) {
		fmt.Printf("[%3.0f%%] %s\n", prog*100, step)
	})
	if err != nil {
		return fmt.Errorf("uninstallation failed: %w", err)
	}

	fmt.Println("\nQuickFlare uninstalled successfully.")
	return nil
}


// cmdReinstall handles 'quickflare reinstall'.
func cmdReinstall(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("reinstall", flag.ExitOnError)
	yes := fs.Bool("y", false, "confirm uninstallation and reinstallation without interactive prompt")
	fs.BoolVar(yes, "yes", false, "confirm without interactive prompt")
	keepRoutes := fs.Bool("keep-routes", false, "do not remove active Cloudflare routes")
	keepConfig := fs.Bool("keep-config", false, "preserve configuration and saved credentials")
	installerPath := fs.String("installer", "", "path to QuickFlare-Setup.exe or MSI installer")
	fs.StringVar(installerPath, "msi", "", "path to QuickFlare installer")
	pos := parseFlags(fs, args)

	targetInstaller := *installerPath
	if targetInstaller == "" && len(pos) > 0 {
		targetInstaller = pos[0]
	}
	if targetInstaller == "" {
		targetInstaller = findInstaller()
	}

	_, installDir := getInstalledProductInfo()
	reader := bufio.NewReader(os.Stdin)

	if installDir != "" || installer.IsInstalled() {
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

		if installDir == "" {
			installDir = installer.DefaultInstallDir()
		}

		opt := installer.Options{
			InstallDir:   installDir,
			RemoveRoutes: removeRoutes,
			RemoveConfig: *keepConfig == false && !*yes, // keep config on reinstall by default
		}

		fmt.Println("\nUninstalling current version...")
		_ = installer.Uninstall(ctx, opt, func(step string, prog float32) {
			fmt.Printf("[%3.0f%%] %s\n", prog*100, step)
		})
	} else {
		fmt.Println("No existing QuickFlare installation detected. Performing fresh install...")
	}

	if targetInstaller == "" {
		return fmt.Errorf("no QuickFlare installer found; please download QuickFlare-Setup.exe or specify --installer <path>")
	}

	return launchInstaller(targetInstaller)
}

// cmdInstall handles 'quickflare install'.
// If QuickFlare is currently installed, it uninstalls the current version first, then reinstalls.
func cmdInstall(ctx context.Context, args []string) error {
	_, installDir := getInstalledProductInfo()
	if installDir != "" || installer.IsInstalled() {
		fmt.Println("QuickFlare is currently installed. Running uninstaller for current version, then reinstalling...")
		return cmdReinstall(ctx, args)
	}

	// Fresh install
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	installerPath := fs.String("installer", "", "path to QuickFlare-Setup.exe or MSI installer")
	fs.StringVar(installerPath, "msi", "", "path to QuickFlare installer")
	pos := parseFlags(fs, args)

	targetInstaller := *installerPath
	if targetInstaller == "" && len(pos) > 0 {
		targetInstaller = pos[0]
	}
	if targetInstaller == "" {
		targetInstaller = findInstaller()
	}
	if targetInstaller == "" {
		return fmt.Errorf("no QuickFlare installer found; please download QuickFlare-Setup.exe or specify --installer <path>")
	}

	return launchInstaller(targetInstaller)
}
