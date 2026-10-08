# Changelog

## v0.4.9 - 2026-10-08

- Icon updated.

## v0.4.8 - 2026-10-07

- Protected tunnel ingress updates with a mutex to eliminate concurrent route clobbering.
- Added generational process cancellation to eliminate orphaned `cloudflared` background instances on tunnel restart.
- Reset reconnect backoff to 1s after sustained uptime, removing 30s delays after temporary network drops.
- Added tri-state DNS verification to prevent failed network calls from reporting false connected states.
- Implemented atomic config writes with Windows file-sharing retry logic to prevent truncation.

## v0.4.7 - 2026-10-07

- Fixed Windows tray thread affinity and message pump starvation causing unresponsive click events.
- Re-architected panel focus and Win32 foreground acquisition to prevent premature auto-hiding.
- Enforced single-instance via named mutex and IPC forwarding so secondary launches bring up the existing window.
- Added one-click GitHub issue reporting and instant log directory access in Settings and tray menu.
- Deep uninstaller cleanup completely removes runtime artifacts, logs, and configuration files.

## v0.4.6 - 2026-09-26

- Fixed tray icon click not opening window due to duplicate mouse-down/up event triggers.
- Added named mutex and IPC detection to prevent duplicate tray instances on relaunch.
- Fixed installer lingering in memory by launching tray process detached via ShellExecute.

## v0.4.5 - 2026-09-24

- Replaced bloated Go setup wizard with native Inno Setup 6 compiler using ultra64 solid LZMA2 compression, reducing installer size by 65% from 71 MB to 24 MB.
- Native uninstaller prompts to unpublish active Cloudflare routes via API before removing local files.
- Registers QuickFlare in User `PATH` with instant `WM_SETTINGCHANGE` broadcast.

## v0.4.0 - 2026-09-24

- Native standalone Windows setup installer (`QuickFlare-Setup.exe`).
- Clean uninstallation gracefully unpublishes active Cloudflare routes via API and removes credentials.
- Automatic PATH registration with shell broadcast, Start Menu shortcuts, and Apps & Features integration.

## v0.3.3 - 2026-09-23

- Embedded Inter and JetBrains Mono typography across all views.
- One-click route pause/resume toggle with tray status indicator and auto-reconciliation.
- Confirmation modal overlay for route deletion and active configuration spinner.
- Cross-compiled Linux CLI packages (.deb, .rpm, .tar.gz, standalone binaries).

## v0.3.2-dev - 2026-09-23

- Added headless Linux CLI: `login`, `domains`, `route add|ls|rm`, `quick`, `run`, `status`, and `reconcile`.
- Systemd user unit generation (`quickflare service install`) for persistent background supervision.
- Native Linux packages (.deb, .rpm, .tar.gz) for amd64 and arm64 architectures.
- Plaintext token storage with 0600 file permissions on Linux.

## v0.3.1 - 2026-09-23

- In-app route deletion button and live status indicators (active listener vs idle port).
- Automated cleanup of runtime `cloudflared` engine on uninstallation.
- 64-bit MSI installer registration.

## v0.3.0 - 2026-09-21

- Expose local ports to the internet through Cloudflare Tunnel without port forwarding or config files.
- Custom Domains: automatic tunnel provisioning, DNS CNAME configuration, and route traffic management.
- Quick Tunnels: instant ephemeral `trycloudflare.com` public URLs without an account.
- Native Windows system tray application with embedded `cloudflared` runtime.
