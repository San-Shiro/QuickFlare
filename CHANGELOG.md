# Changelog

## v0.3.3 - 2026-09-23

- **Typography** — Embedded Inter and JetBrains Mono fonts for enhanced readability across all views.
- **Route Controls** — One-click pause/resume toggle with tray icon indicator and automatic route self-healing.
- **UI Enhancements** — Confirmation modal overlay for route deletion, active configuration spinner, and page fade transitions.
- **Linux Packages (Untested)** — Added CLI and packages for Linux (.deb, .rpm, .tar.gz, standalone binaries) (untested).

## v0.3.2-dev - 2026-09-23

Pre-release adding Linux support alongside Windows binaries.

### Linux CLI
- **Command Line Tool** — Added `quickflare` CLI: `login`, `domains`, `route add|ls|rm`, `quick`, `run`, `status`, `reconcile`.
- **Process Supervision** — `quickflare service install` sets up a systemd user unit so routes survive logout and restart on failure.
- **Packages** — Native packages for `amd64` and `arm64`: `.deb` (Debian/Ubuntu), `.rpm` (Fedora/RHEL), `.tar.gz` archive, and standalone binaries.
- **Credential Storage** — Linux stores API tokens with 0600 file permissions (Windows uses DPAPI encryption).

### Windows
- Includes latest Windows portable binary (`.exe`) and installer (`.msi`).

## v0.3.1 - 2026-09-23

- **UI Updates** — Red trash button for removing routes; live status dot (green when local service is listening, yellow when port is idle).
- **Windows Installer** — Clean uninstall of runtime engine; 64-bit MSI registration.

## v0.3.0 - 2026-09-21

First release.

- **Custom Domains** — Automatically provisions tunnels, creates DNS records, and routes traffic for your Cloudflare domains.
- **Temporary Domains** — Instant `trycloudflare.com` tunnels for any local port with no account required.
