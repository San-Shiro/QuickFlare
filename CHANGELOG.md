# Changelog

## v0.4.8 - 2026-10-07

- Protected tunnel ingress configuration updates with a mutex to prevent concurrent route clobbering.
- Added generational cancellation to eliminate orphaned `cloudflared` background processes on tunnel restarts.
- Reset reconnect backoff to 1s after sustained uptime, eliminating prolonged 30s delays after temporary network drops.
- Added tri-state DNS verification so API or network errors no longer report routes as falsely connected.
- Implemented atomic config writes with Windows file-sharing retry logic to prevent file truncation under concurrent access.
- Fixed target scheme normalization to preserve explicit schemes (`https://`, `tcp://`) without prepending duplicate `http://`.
- Fixed Windows process liveness checks using signaled handle synchronization instead of unreliable exit code 259.

## v0.4.7 - 2026-10-07

- **Windows Tray Thread Affinity & Message Queue Stabilization**: Fixed an issue on Windows where Go scheduler OS thread migration starved the Win32 message pump of `systray`, causing the tray icon to ignore clicks and right-clicks.
- **Auto-Hide Race & Focus Acquisition**: Re-architected panel focus management to prevent premature auto-hiding on Windows 11; integrated `AttachThreadInput`, `BringWindowToTop`, and `AllowSetForegroundWindow` for reliable foreground focus acquisition.
- **Single-Instance Enforcement & Process Decoupling**: Added Win32 named mutex `Local\QuickFlare_Tray_SingleInstance_Mutex` with IPC forwarding so secondary launches bring up the existing window cleanly. Replaced CLI process spawning with `ShellExecuteW` to avoid termination from console job objects.
- **Clean Process Lifecycle**: Ensured explicit teardown on Gio `DestroyEvent` to prevent zombie tray processes from lingering in the background.
- **Integrated Bug Reporting & Diagnostics**: Added one-click GitHub issue reporting and instant log directory access in both the desktop Settings view and the system tray menu.
- **CLI Diagnostics & Version Commands**: Added `quickflare version` (`-v`) and `quickflare issue` commands to quickly inspect versions and report bugs from the terminal.
- **Deep Uninstallation & State Cleanup**: Enhanced Inno Setup uninstaller to completely wipe runtime artifacts (`bin\`, `logs\`, `config.json`, `tray.log`), leaving zero residual files upon uninstallation.
- **Pure Native Go Gio Desktop Architecture**: Retained and polished the Gio UI desktop app with dark mode, fluid transitions, and seamless Windows DWM integration.

## v0.4.6 - 2026-09-26

- Fixed tray icon click not opening window due to duplicate mouse-down/up event triggers.
- Added named mutex and IPC detection to prevent duplicate tray instances on relaunch.
- Fixed installer lingering in memory by launching tray process detached via ShellExecute.

## v0.4.5 - 2026-09-24

- **Native Setup Engine** — Replaced bloated Go setup wizard with native Inno Setup 6 compiler using ultra64 solid LZMA2 compression, reducing installer size by 65% from 71 MB to 24 MB.
- **Zero Go Runtime Overhead in Setup** — Native C/C++ installer engine runs instantly, deduplicates cross-binary data blocks, and eliminates redundant GPU/GUI framework overhead.
- **Graceful Uninstallation Prompt** — Native uninstaller prompts to gracefully unpublish active Cloudflare routes via API before removing local files.
- **Environment & PATH** — Automatically registers QuickFlare in User `PATH` with instant `WM_SETTINGCHANGE` broadcast for immediate terminal availability.
- **Flexible Experience Tasks** — Choose between "CLI with System Tray (Recommended)" and "CLI Only" (strictly enforcing no tray-only mode).

## v0.4.0 - 2026-09-24

- **Modern Setup GUI** — Added branded Go + Gio standalone installer (`QuickFlare-Setup.exe`) with dark theme and Windows DWM immersive title bar.
- **Experience Selection** — Choose between "CLI with System Tray (Recommended)" and "CLI Only" (strict invariant: no tray-only option).
- **Graceful Route Teardown** — Uninstaller prompts to gracefully unpublish active Cloudflare routes via API (default: enabled) and remove local credentials.
- **Environment & Shortcuts** — Automated user PATH registration with instant shell broadcast, Start Menu shortcuts, and Apps & Features integration.
- **CLI Command Integration** — Modernized `quickflare install` and `quickflare reinstall` to auto-detect and run the modern setup wizard.

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
