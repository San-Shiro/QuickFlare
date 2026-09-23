# QuickFlare — Codebase Structure & Developer Guide

This document outlines the package organization, build process, and core invariants for QuickFlare developers.

---

## 1. Directory Structure

```
QuickFlare/
├── cmd/
│   └── quickflare/       # Application entry point, Windows icon resource
├── internal/
│   ├── autostart/        # Per-user HKCU Run key registry autostart manager
│   ├── cloudflare/       # Cloudflare API v4 client (tunnels, ingress, DNS)
│   ├── config/           # DPAPI-encrypted token store & route cache
│   ├── supervisor/       # Inbuilt engine extraction, child process lifecycle, job objects
│   └── ui/               # Gio UI widgets, panels, views, tray integration, Win32 placement
├── packaging/
│   ├── quickflare.wxs    # WiX Toolset v5 MSI installer definition
│   └── release.sh        # Automated build and GitHub release publisher
├── assets/               # Application icons and logos (PNG, ICO)
├── docs/                 # Documentation (USAGE.md, ARCHITECTURE.md, CODEBASE.md)
├── go.mod / go.sum       # Go dependencies (Go 1.26+, CGO_ENABLED=0)
└── README.md             # Project overview
```

---

## 2. Key Packages

### `internal/supervisor`
Manages the `cloudflared` engine:
- `embedded_windows.go`: Handles decompression and extraction of the embedded `cloudflared.exe.gz` to `%LOCALAPPDATA%\QuickFlare\bin\cloudflared.exe`.
- `cloudflared.go`: Implements `Tunnel` supervisor, stderr monitoring, health check probing via `/ready` endpoint, and binary location via `FindBinary()`.
- `quick.go`: Spawns and manages anonymous TryCloudflare quick tunnels (`cloudflared tunnel --url ...`).
- `job_windows.go`: Binds child processes to a Windows Kernel Job Object (`JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`).

### `internal/cloudflare`
Minimal, stdlib-only Cloudflare API v4 client:
- `client.go`: Authenticated HTTP client with exponential backoff and rate-limit handling.
- `tunnel.go`: Manages Cloudflare Tunnels (create, delete, ingress configuration, token retrieval).
- `dns.go`: Manages CNAME DNS records, verifying ownership via `OwnedByUs()`.
- `account.go`: Discovers zones and account metadata.

### `internal/config`
Persists user settings and route caches:
- Stored in `%APPDATA%\QuickFlare\config.json`.
- Uses Windows Data Protection API (`CryptProtectData`) to encrypt API tokens at rest.
- Prevents cleartext leaks via `json:"-"` struct tags.
- `guard.go` prevents test binaries from writing to production config paths.

### `internal/ui`
User interface built with [Gio](https://gioui.org):
- `panel.go`: Primary panel state machine, event dispatcher, and route mutation handlers.
- `views.go`: Screen implementations (Main list, Quick tunnel form, Named route form, Settings).
- `place_windows.go`: Win32 asynchronous positioning, taskbar hiding, DWM styling, and multi-monitor detection.
- `tokens.go`: Design system tokens (typography, spacing, colors, geometry).

---

## 3. Building from Source

### Prerequisites
- Windows 10 or 11 (amd64)
- Go 1.26 or higher
- (Optional, for MSI) WiX Toolset v5.0.2

### Building the Portable Binary
```powershell
go build -ldflags "-H windowsgui -s -w" -o build/QuickFlare-0.3.1.exe ./cmd/quickflare
```

### Running Tests
```powershell
go vet ./...
go test -v ./...
```

### Building the MSI Installer
```powershell
wix build packaging/quickflare.wxs -b . -d Version=0.3.1 -o build/QuickFlare-0.3.1-x64.msi
```
