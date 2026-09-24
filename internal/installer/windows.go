//go:build windows

package installer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/San-Shiro/QuickFlare/internal/cloudflare"
	"github.com/San-Shiro/QuickFlare/internal/config"
	"github.com/San-Shiro/QuickFlare/internal/core"
	"github.com/San-Shiro/QuickFlare/internal/ipc"
)

const (
	regUninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\QuickFlare`
	regAppKey       = `Software\QuickFlare`
)

// IsInstalled returns true if QuickFlare is currently registered or installed.
func IsInstalled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, regUninstallKey, registry.QUERY_VALUE)
	if err == nil {
		k.Close()
		return true
	}
	k2, err2 := registry.OpenKey(registry.CURRENT_USER, regAppKey, registry.QUERY_VALUE)
	if err2 == nil {
		k2.Close()
		return true
	}
	cliPath := filepath.Join(DefaultInstallDir(), "quickflare.exe")
	if _, err := os.Stat(cliPath); err == nil {
		return true
	}
	return false
}

// broadcastSettingChange notifies Windows shells that user environment variables changed.
func broadcastSettingChange() {
	user32 := windows.NewLazySystemDLL("user32.dll")
	sendMessageTimeout := user32.NewProc("SendMessageTimeoutW")
	envPtr, _ := windows.UTF16PtrFromString("Environment")

	const (
		HWND_BROADCAST   = 0xFFFF
		WM_SETTINGCHANGE = 0x001A
		SMTO_ABORTIFHUNG = 0x0002
	)

	var result uintptr
	_, _, _ = sendMessageTimeout.Call(
		uintptr(HWND_BROADCAST),
		uintptr(WM_SETTINGCHANGE),
		0,
		uintptr(unsafe.Pointer(envPtr)),
		uintptr(SMTO_ABORTIFHUNG),
		5000,
		uintptr(unsafe.Pointer(&result)),
	)
}

// addPathEntry adds targetDir to HKCU\Environment\Path if not already present.
func addPathEntry(targetDir string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, "Environment", registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open user environment registry: %w", err)
	}
	defer k.Close()

	curPath, _, err := k.GetStringValue("Path")
	if err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("read user PATH: %w", err)
	}

	parts := strings.Split(curPath, ";")
	for _, p := range parts {
		if strings.EqualFold(strings.TrimSpace(p), targetDir) {
			return nil // already present
		}
	}

	var newParts []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			newParts = append(newParts, trimmed)
		}
	}
	newParts = append(newParts, targetDir)
	newVal := strings.Join(newParts, ";")

	if err := k.SetStringValue("Path", newVal); err != nil {
		return fmt.Errorf("write user PATH: %w", err)
	}
	broadcastSettingChange()
	return nil
}

// removePathEntries removes specified dirs from HKCU\Environment\Path.
func removePathEntries(dirs ...string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, "Environment", registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()

	curPath, _, err := k.GetStringValue("Path")
	if err != nil {
		return nil
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
	return nil
}

// createShortcut creates a Windows .lnk shortcut file.
func createShortcut(shortcutPath, targetPath, workDir, iconPath, description string) error {
	_ = os.MkdirAll(filepath.Dir(shortcutPath), 0755)
	psCmd := fmt.Sprintf(`$s = (New-Object -ComObject WScript.Shell).CreateShortcut('%s'); $s.TargetPath = '%s'; $s.WorkingDirectory = '%s'; $s.Description = '%s'; if ('%s' -ne '') { $s.IconLocation = '%s' }; $s.Save()`,
		shortcutPath, targetPath, workDir, description, iconPath, iconPath)
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", psCmd)
	return cmd.Run()
}

// Install executes the full installation process.
func Install(ctx context.Context, opt Options, p ProgressFunc) error {
	if p == nil {
		p = func(string, float32) {}
	}
	if opt.InstallDir == "" {
		opt.InstallDir = DefaultInstallDir()
	}

	p("Preparing installation environment...", 0.10)
	// 1. Quiesce any running instances
	client, err := ipc.Discover()
	if err == nil && client != nil {
		_ = client.Quit(ctx)
		time.Sleep(400 * time.Millisecond)
	}
	_ = exec.Command("taskkill", "/f", "/im", "quickflare.exe").Run()
	_ = exec.Command("taskkill", "/f", "/im", "quickflare-tray.exe").Run()

	p("Creating installation directory...", 0.25)
	if err := os.MkdirAll(opt.InstallDir, 0755); err != nil {
		return fmt.Errorf("create install directory: %w", err)
	}

	p("Extracting QuickFlare binaries...", 0.40)
	cliDest := filepath.Join(opt.InstallDir, "quickflare.exe")
	if len(GlobalPayload.CliExe) > 0 {
		if err := os.WriteFile(cliDest, GlobalPayload.CliExe, 0755); err != nil {
			return fmt.Errorf("write quickflare.exe: %w", err)
		}
	} else {
		// Fallback: check build directory if running during development
		if src, err := os.ReadFile("build/quickflare.exe"); err == nil {
			_ = os.WriteFile(cliDest, src, 0755)
		}
	}

	trayDest := filepath.Join(opt.InstallDir, "quickflare-tray.exe")
	if opt.InstallTray {
		if len(GlobalPayload.TrayExe) > 0 {
			if err := os.WriteFile(trayDest, GlobalPayload.TrayExe, 0755); err != nil {
				return fmt.Errorf("write quickflare-tray.exe: %w", err)
			}
		} else {
			if src, err := os.ReadFile("build/quickflare-tray.exe"); err == nil {
				_ = os.WriteFile(trayDest, src, 0755)
			}
		}
	} else {
		// If user chose CLI-only, ensure tray binary is not present
		_ = os.Remove(trayDest)
	}

	p("Configuring uninstaller script...", 0.55)
	uninstallCmdPath := filepath.Join(opt.InstallDir, "uninstall.cmd")
	uninstallCmdContent := `@echo off
title QuickFlare Uninstaller
if exist "%~dp0quickflare.exe" (
    "%~dp0quickflare.exe" uninstall %*
    exit /b %ERRORLEVEL%
)
echo QuickFlare uninstallation initiated...
pause
`
	_ = os.WriteFile(uninstallCmdPath, []byte(uninstallCmdContent), 0755)

	// 2. PATH Registration
	if opt.AddToPath {
		p("Configuring user PATH environment...", 0.70)
		if err := addPathEntry(opt.InstallDir); err != nil {
			return fmt.Errorf("register user PATH: %w", err)
		}
	}

	// 3. App Paths
	p("Registering application shell commands...", 0.80)
	kCli, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\App Paths\quickflare.exe`, registry.SET_VALUE)
	if err == nil {
		_ = kCli.SetStringValue("", cliDest)
		_ = kCli.SetStringValue("Path", opt.InstallDir)
		kCli.Close()
	}
	if opt.InstallTray {
		kTray, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\App Paths\quickflare-tray.exe`, registry.SET_VALUE)
		if err == nil {
			_ = kTray.SetStringValue("", trayDest)
			_ = kTray.SetStringValue("Path", opt.InstallDir)
			kTray.Close()
		}
	}

	// 4. Start Menu Shortcuts
	programsDir := filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", "QuickFlare")
	if opt.InstallTray && opt.CreateShortcuts {
		p("Creating Start Menu shortcuts...", 0.85)
		_ = os.MkdirAll(programsDir, 0755)
		appLnk := filepath.Join(programsDir, "QuickFlare.lnk")
		_ = createShortcut(appLnk, trayDest, opt.InstallDir, trayDest, "Publish localhost through Cloudflare Tunnel")

		uninstLnk := filepath.Join(programsDir, "Uninstall QuickFlare.lnk")
		_ = createShortcut(uninstLnk, cliDest, opt.InstallDir, trayDest, "Uninstall QuickFlare")
	} else {
		_ = os.RemoveAll(programsDir)
	}

	// 5. Windows Apps & Features Registration
	p("Registering Windows Apps & Features...", 0.92)
	kUninst, _, err := registry.CreateKey(registry.CURRENT_USER, regUninstallKey, registry.SET_VALUE)
	if err == nil {
		_ = kUninst.SetStringValue("DisplayName", "QuickFlare")
		_ = kUninst.SetStringValue("DisplayVersion", Version)
		_ = kUninst.SetStringValue("Publisher", "QuickFlare contributors")
		_ = kUninst.SetStringValue("InstallLocation", opt.InstallDir)
		_ = kUninst.SetStringValue("UninstallString", fmt.Sprintf(`"%s" uninstall`, cliDest))
		if opt.InstallTray {
			_ = kUninst.SetStringValue("DisplayIcon", trayDest)
		} else {
			_ = kUninst.SetStringValue("DisplayIcon", cliDest)
		}
		_ = kUninst.SetStringValue("URLInfoAbout", "https://github.com/San-Shiro/QuickFlare")
		_ = kUninst.SetDWordValue("NoModify", 1)
		_ = kUninst.SetDWordValue("NoRepair", 1)
		_ = kUninst.SetDWordValue("EstimatedSize", 60000)
		kUninst.Close()
	}

	// 6. Registry Metadata
	kApp, _, err := registry.CreateKey(registry.CURRENT_USER, regAppKey, registry.SET_VALUE)
	if err == nil {
		_ = kApp.SetDWordValue("installed", 1)
		_ = kApp.SetStringValue("InstallDir", opt.InstallDir)
		_ = kApp.SetStringValue("Version", Version)
		if opt.InstallTray {
			_ = kApp.SetDWordValue("tray_installed", 1)
		} else {
			_ = kApp.SetDWordValue("tray_installed", 0)
		}
		kApp.Close()
	}

	// 7. Optional launch after install
	if opt.LaunchAfter && opt.InstallTray {
		p("Launching QuickFlare...", 0.98)
		cmd := exec.Command(trayDest)
		_ = cmd.Start()
	}

	p("Installation completed successfully!", 1.0)
	return nil
}

// Uninstall executes the full uninstallation process.
func Uninstall(ctx context.Context, opt Options, p ProgressFunc) error {
	if p == nil {
		p = func(string, float32) {}
	}
	if opt.InstallDir == "" {
		opt.InstallDir = DefaultInstallDir()
	}

	// 1. Unpublish active Cloudflare routes if requested
	if opt.RemoveRoutes {
		p("Unpublishing active routes from Cloudflare...", 0.20)
		cfg, err := config.Load()
		if err == nil && cfg != nil && cfg.APIToken != "" && len(cfg.Routes) > 0 {
			s, err := openSession(cfg)
			if err == nil {
				if err := s.withTunnel(ctx); err == nil {
					for _, r := range s.routes() {
						_ = core.Unpublish(ctx, s.client, r.ZoneID, s.tunnelID, r.Hostname)
					}
					cfg.Routes = nil
					_ = cfg.Save()
				}
			}
		}
	}

	p("Stopping QuickFlare processes...", 0.40)
	client, err := ipc.Discover()
	if err == nil && client != nil {
		_ = client.Quit(ctx)
		time.Sleep(300 * time.Millisecond)
	}
	_ = exec.Command("taskkill", "/f", "/im", "quickflare.exe").Run()
	_ = exec.Command("taskkill", "/f", "/im", "quickflare-tray.exe").Run()
	_ = exec.Command("taskkill", "/f", "/im", "cloudflared.exe").Run()

	// 2. Remove configuration if requested
	if opt.RemoveConfig {
		p("Removing configuration files and credentials...", 0.55)
		configDir, err := config.Dir()
		if err == nil && configDir != "" {
			_ = os.RemoveAll(configDir)
		}
	}

	// 3. Remove user PATH
	p("Cleaning user PATH environment...", 0.70)
	selfPath, _ := os.Executable()
	selfDir := filepath.Dir(selfPath)
	_ = removePathEntries(opt.InstallDir, selfDir, DefaultInstallDir())

	// 4. Remove Start Menu shortcuts
	p("Removing Start Menu shortcuts...", 0.80)
	programsDir := filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", "QuickFlare")
	_ = os.RemoveAll(programsDir)

	// 5. Remove registry keys
	p("Removing registry associations...", 0.90)
	_ = registry.DeleteKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\App Paths\quickflare.exe`)
	_ = registry.DeleteKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\App Paths\quickflare-tray.exe`)
	_ = registry.DeleteKey(registry.CURRENT_USER, regUninstallKey)
	_ = registry.DeleteKey(registry.CURRENT_USER, regAppKey)

	// 6. Remove files and install folder
	p("Removing application files...", 0.95)
	_ = os.Remove(filepath.Join(opt.InstallDir, "quickflare.exe"))
	_ = os.Remove(filepath.Join(opt.InstallDir, "quickflare-tray.exe"))
	_ = os.Remove(filepath.Join(opt.InstallDir, "uninstall.cmd"))
	_ = os.RemoveAll(filepath.Join(opt.InstallDir, "bin"))
	_ = os.Remove(opt.InstallDir) // only removes if empty

	p("Uninstallation completed successfully!", 1.0)
	return nil
}

// session helper for route teardown
type instSession struct {
	cfg      *config.Config
	client   *cloudflare.Client
	tunnelID string
}

func openSession(cfg *config.Config) (*instSession, error) {
	if cfg == nil || cfg.APIToken == "" {
		return nil, fmt.Errorf("no API token")
	}
	return &instSession{cfg: cfg, client: cloudflare.New(cfg.APIToken, "")}, nil
}

func (s *instSession) withTunnel(ctx context.Context) error {
	tun, err := core.FindOrCreateTunnel(ctx, s.client)
	if err != nil {
		return err
	}
	s.tunnelID = tun.ID
	return nil
}

func (s *instSession) routes() []core.Route {
	out := make([]core.Route, 0, len(s.cfg.Routes))
	for _, r := range s.cfg.Routes {
		out = append(out, core.Route{
			Hostname: r.Hostname,
			Target:   r.Target,
			ZoneID:   r.ZoneID,
		})
	}
	return out
}
