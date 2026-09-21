//go:build linux

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/San-Shiro/QuickFlare/internal/config"
)

// Connector persistence on Linux is systemd's job, not ours.
//
// The Windows tray keeps cloudflared alive by staying open. There is no tray
// to rely on here, and the alternative - writing a daemon - would mean
// reimplementing restart-on-failure, logging, start-at-login and
// survive-logout, all of which a twelve-line unit file already gets right.
//
// The unit runs "quickflare run" rather than cloudflared directly. That
// matters for one specific reason: running cloudflared would need its tunnel
// token in ExecStart, and unit files are world-readable. Going through
// quickflare keeps the credential in the 0600 config, fetches a fresh tunnel
// token each start, and survives the tunnel being recreated.

const unitName = "quickflare.service"

// systemdPresent reports whether this is a systemd system. Alpine, Void,
// Artix and Devuan are not, and on those the connector has to be run in the
// foreground or supervised by whatever the distro does use.
func systemdPresent() bool {
	st, err := os.Stat("/run/systemd/system")
	return err == nil && st.IsDir()
}

func unitPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "systemd", "user", unitName), nil
}

func cmdService(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("service needs a subcommand: install, uninstall, status, start, stop")
	}
	switch args[0] {
	case "install":
		return serviceInstall(ctx)
	case "uninstall", "remove":
		return serviceUninstall(ctx)
	case "status":
		return serviceStatus(ctx)
	case "start", "stop", "restart":
		return systemctl(ctx, args[0], unitName)
	default:
		return fmt.Errorf("unknown service subcommand %q", args[0])
	}
}

func serviceInstall(ctx context.Context) error {
	if !systemdPresent() {
		return fmt.Errorf("this system does not run systemd - use 'quickflare run' " +
			"under whatever supervisor the distribution provides")
	}

	// Refuse to install a service that cannot work. A unit that starts, fails
	// to find a token and restarts forever is worse than a clear error now.
	if _, err := open(); err != nil {
		return err
	}

	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate this binary: %w", err)
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return fmt.Errorf("resolve this binary: %w", err)
	}

	path, err := unitPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}

	// The config directory is resolved now, not written as a %h specifier:
	// XDG_CONFIG_HOME can move it, and a ReadWritePaths naming the wrong
	// place fails at write time with an error that looks nothing like a
	// misconfigured unit. See unit.go.
	cfgPath, err := config.Path()
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}
	unit := unitFile(self, filepath.Dir(cfgPath))

	if err := os.WriteFile(path, []byte(unit), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	fmt.Printf("Wrote %s\n", path)

	if err := systemctl(ctx, "daemon-reload"); err != nil {
		return err
	}
	if err := systemctl(ctx, "enable", "--now", unitName); err != nil {
		return err
	}

	fmt.Println("Service installed and started.")

	// Without lingering, a user unit stops at logout - which for a machine
	// you are exposing a port from is almost never what is wanted, and is
	// confusing when it happens.
	if !lingering() {
		who := "$USER"
		if u, err := user.Current(); err == nil {
			who = u.Username
		}
		fmt.Printf("\nRoutes will stop when you log out. To keep them up:\n"+
			"    sudo loginctl enable-linger %s\n", who)
	}
	return nil
}

func serviceUninstall(ctx context.Context) error {
	path, err := unitPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("no service installed")
	}

	// Best-effort: a unit that is already stopped, masked or broken should
	// still be removable rather than leaving the file behind.
	_ = systemctl(ctx, "disable", "--now", unitName)

	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	_ = systemctl(ctx, "daemon-reload")

	fmt.Printf("Removed %s\n", path)
	fmt.Println("Routes stay published on Cloudflare - remove them with 'quickflare route rm'.")
	return nil
}

func serviceStatus(ctx context.Context) error {
	if !systemdPresent() {
		fmt.Println("systemd: not present on this system")
		return nil
	}
	path, err := unitPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Println("service: not installed  (quickflare service install)")
		return nil
	}

	fmt.Printf("unit:    %s\n", path)
	fmt.Printf("active:  %s\n", systemctlProperty(ctx, "ActiveState"))
	fmt.Printf("enabled: %s\n", systemctlProperty(ctx, "UnitFileState"))
	fmt.Printf("linger:  %s\n", yesNo(lingering()))
	return nil
}

// systemctl runs a --user systemctl command, surfacing its stderr rather than
// just an exit code - "Failed to connect to bus" is the common failure and it
// means something quite specific.
func systemctl(ctx context.Context, args ...string) error {
	full := append([]string{"--user"}, args...)
	cmd := exec.CommandContext(ctx, "systemctl", full...)
	outBytes, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(outBytes))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("systemctl %s: %s", strings.Join(args, " "), msg)
	}
	return nil
}

func systemctlProperty(ctx context.Context, name string) string {
	cmd := exec.CommandContext(ctx, "systemctl", "--user", "show", unitName, "-p", name, "--value")
	b, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	if v := strings.TrimSpace(string(b)); v != "" {
		return v
	}
	return "unknown"
}

// lingering reports whether this user's services survive logout.
func lingering() bool {
	u, err := user.Current()
	if err != nil {
		return false
	}
	b, err := exec.Command("loginctl", "show-user", u.Username, "-p", "Linger", "--value").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(b)) == "yes"
}
