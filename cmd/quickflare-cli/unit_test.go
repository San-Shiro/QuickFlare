package main

import (
	"strings"
	"testing"
)

// A systemd unit fails at runtime, on a machine the author may not have. The
// cheap checks are worth making here instead.
func TestUnitFile(t *testing.T) {
	u := unitFile("/home/sam/.local/bin/quickflare", "/home/sam/.config/QuickFlare")

	must := map[string]string{
		"[Unit]":                      "systemd needs the section header",
		"[Service]":                   "systemd needs the section header",
		"[Install]":                   "without this, enable has nothing to link",
		"WantedBy=default.target":     "a user unit starts from default.target, not multi-user.target",
		"Wants=network-online.target": "After alone does not pull the target in",
		"Restart=on-failure":          "the point of using systemd at all",
	}
	for frag, why := range must {
		if !strings.Contains(u, frag) {
			t.Errorf("unit is missing %q - %s", frag, why)
		}
	}

	// ExecStart must be absolute: systemd does not search PATH.
	if !strings.Contains(u, "ExecStart=/home/sam/.local/bin/quickflare run") {
		t.Error("ExecStart must be the absolute binary path plus the run subcommand")
	}

	// The whole reason configDir is a parameter. A unit that hardcodes
	// %h/.config/QuickFlare breaks for anyone with XDG_CONFIG_HOME set, and
	// breaks at write time rather than at install time.
	if !strings.Contains(u, "ReadWritePaths=/home/sam/.config/QuickFlare") {
		t.Error("ReadWritePaths must name the resolved config directory")
	}
	if strings.Contains(u, "%h") {
		t.Error("no %h specifiers: the config directory is resolved at install time")
	}

	// ProtectSystem=strict makes the whole filesystem read-only, so without a
	// matching ReadWritePaths the service cannot save its own route cache.
	if strings.Contains(u, "ProtectSystem=strict") && !strings.Contains(u, "ReadWritePaths=") {
		t.Error("ProtectSystem=strict without ReadWritePaths leaves the config unwritable")
	}
}

// Every directive must be Key=Value under some section. A stray line is not a
// warning in systemd - it refuses to load the unit.
func TestUnitFileIsWellFormed(t *testing.T) {
	u := unitFile("/usr/bin/quickflare", "/root/.config/QuickFlare")

	var section string
	for i, line := range strings.Split(u, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = line
			continue
		}
		if section == "" {
			t.Errorf("line %d is outside any section: %q", i+1, line)
		}
		if !strings.Contains(line, "=") {
			t.Errorf("line %d is not Key=Value: %q", i+1, line)
		}
	}
}
