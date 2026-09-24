<div align="center">

<img src="assets/logo-256.png" alt="QuickFlare" width="112">

# QuickFlare

**Publish localhost to the internet through Cloudflare Tunnel with cloudflared.**

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Release](https://img.shields.io/badge/release-v0.3.3-F6821F.svg)](https://github.com/San-Shiro/QuickFlare/releases)

</div>

---

You have something running on `localhost:3000`. You want it reachable from the
internet, on your own domain. QuickFlare does that in a few clicks instead of a
config file and three dashboard pages.

No port forwarding, no firewall rules — `cloudflared` makes an outbound
connection to Cloudflare.

## Install

From [Releases](https://github.com/San-Shiro/QuickFlare/releases):

**Windows** — tray app and CLI, with the `cloudflared` engine bundled inside it, so
there is nothing else to install.

| File | |
|---|---|
| `QuickFlare-0.3.3-Setup.exe` | **Modern GUI Installer** (Recommended): branded setup wizard, choose CLI+Tray or CLI-only, auto-configures PATH and Start Menu |
| `QuickFlare-0.3.3-x64.msi` | Windows Installer (MSI): enterprise / silent installs |
| `QuickFlare-0.3.3-windows-portable.zip` | Portable bundle (CLI + Tray) |
| `quickflare-0.3.3-windows-amd64.exe` | Standalone CLI only |

**Linux** — command line only, and currently a preview. See
[Linux](#linux-preview) below.

| File | |
|---|---|
| `quickflare_0.3.3_amd64.deb` | Debian, Ubuntu, Mint |
| `quickflare-0.3.3.x86_64.rpm` | Fedora, RHEL, openSUSE |
| `quickflare-0.3.3-linux-amd64.tar.gz` | Any distribution |

`arm64` builds of each are published alongside.

## Quick Start

Left-click the QuickFlare tray icon to open the panel:

- **Quick Tunnels (No Account)**: Switch to the **Quick** tab, click **+**, and enter a local port to get an instant `https://*.trycloudflare.com` public link. Ephemeral and closes when stopped or on app exit.
- **Custom Domains**: Paste a scoped Cloudflare API token, choose your domain, and add a subdomain and local port (`dev` + `3000` → `dev.example.com`). QuickFlare automatically provisions the tunnel, creates the DNS CNAME record, and routes traffic.

See the dedicated [Cloudflare API Token Guide](docs/CLOUDFLARE_API_TOKEN.md) for step-by-step instructions on generating your token.

## Linux (preview)

The Linux build is a **command-line tool**, not the tray app, and it is
published as a pre-release. It does what the tray does - quick tunnels, routes
on your own domain, reconciliation - from a terminal:

```bash
quickflare login
quickflare route add app --port 3000
quickflare service install
```

`quickflare service install` writes a systemd user unit so routes survive
logout and restart on failure, rather than needing a terminal held open.

Two things to know before using it:

- **`cloudflared` is not bundled on Linux.** Distribution packages should not
  ship a second copy of a binary the package manager can supply. Install it
  from Cloudflare's repository; the packages recommend it.
- **Your API token is stored in plain text**, in a file readable only by you.
  Windows encrypts it with DPAPI; the Linux keystore integration is not
  written yet, and `quickflare status` says so plainly. That is the main
  reason this is a preview.

## Documentation

- [Usage Guide](docs/USAGE.md) — Comprehensive walkthrough of features, route management, and autostart.
- [Cloudflare API Token Guide](docs/CLOUDFLARE_API_TOKEN.md) — Step-by-step token creation with required permissions.
- [Architecture](docs/ARCHITECTURE.md) — Win32 windowing, Gio immediate-mode UI, and Job Object process supervisor.
- [Codebase Guide](docs/CODEBASE.md) — Package layout, state synchronization, and testing patterns.

## Notes

- **Routes are public.** QuickFlare publishes the hostname and nothing else —
  it adds no login. Anything you put behind a route should do its own
  authentication, or not be published at all.
- **The tray app is Windows only.** Linux has the command line tool above; macOS has neither yet.
- On Windows your API token is encrypted with Windows DPAPI, tied to your
  Windows account, and stored in `%APPDATA%\QuickFlare\config.json`. On Linux
  it is **not** encrypted yet - see the preview note above.
- Subdomains are one level deep (`app.example.com`, not `app.dev.example.com`)
  — the free Cloudflare certificate doesn't cover deeper names.
- One DNS record per route. QuickFlare doesn't touch `*.yourdomain`, so a
  domain you're already using stays fine.
- On launch it checks your routes against Cloudflare, restores the ones still
  there, and drops the ones that aren't.
- The binary isn't signed, so Windows will warn you the first time.
- `cloudflared` is bundled inside the Windows build. The Linux packages recommend it instead of shipping their own copy.

## Build

```bash
# Windows tray app
go build -ldflags "-H windowsgui -s -w" -o build/QuickFlare-0.3.3.exe ./cmd/quickflare

# Linux CLI - cross-compiles from anywhere, no cgo
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o build/quickflare ./cmd/quickflare-cli

go test ./...
```

Go 1.26+. The CLI needs no cgo; the tray app needs it only on Linux, which is
why the two are separate binaries.

## Licence

[Apache 2.0](LICENSE).
