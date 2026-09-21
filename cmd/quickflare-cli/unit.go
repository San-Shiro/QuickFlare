package main

import "fmt"

// unitFile renders the systemd user unit.
//
// Deliberately not behind a build tag, so it can be tested from any machine.
// The content is the risky part of service install - a wrong directive here
// produces a unit that starts, fails and restarts forever - and that risk
// should not be confined to a platform the author may not be able to run.
//
// configDir is passed in rather than written as %h/.config/QuickFlare.
// XDG_CONFIG_HOME can move the config somewhere else entirely, and a
// ReadWritePaths that does not name the real directory produces the worst
// kind of failure: the service starts, then cannot save anything, with a
// permission error that looks nothing like a misconfigured unit.
func unitFile(execPath, configDir string) string {
	return fmt.Sprintf(`[Unit]
Description=QuickFlare tunnel connector
Documentation=https://github.com/San-Shiro/QuickFlare
# network-online, not network: the connector dials out immediately, and one
# started before DNS is up just spends its first attempts failing.
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=%s run
Restart=on-failure
RestartSec=5

# The connector is a network client. It reads a token, runs cloudflared and
# talks to Cloudflare; it has no reason to reach anything else on the disk.
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=read-only
# ...except its own config, which is where the API token lives.
ReadWritePaths=%s

[Install]
WantedBy=default.target
`, execPath, configDir)
}
