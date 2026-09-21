<div align="center">

<img src="assets/logo-256.png" alt="QuickFlare" width="112">

# QuickFlare

**Publish localhost to the internet through Cloudflare Tunnel with cloudflared.**

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Release](https://img.shields.io/badge/release-v0.3.0-F6821F.svg)](https://github.com/San-Shiro/QuickFlare/releases)

</div>

---

You have something running on `localhost:3000`. You want it reachable from the
internet, on your own domain. QuickFlare does that in a few clicks instead of a
config file and three dashboard pages.

No port forwarding, no firewall rules — `cloudflared` makes an outbound
connection to Cloudflare.

## Install

Grab either file from [Releases](https://github.com/San-Shiro/QuickFlare/releases):

- **`QuickFlare-0.3.0-x64.msi`** — Windows Installer, per-user, no admin
- **`QuickFlare-0.3.0.exe`** — Portable executable, zero setup

QuickFlare bundles the `cloudflared` tunnel engine directly inside the app, so zero external installation or dependencies are required.

## Quick Start

Left-click the QuickFlare tray icon to open the panel:

- **Quick Tunnels (No Account)**: Switch to the **Quick** tab, click **+**, and enter a local port to get an instant `https://*.trycloudflare.com` public link. Ephemeral and closes when stopped or on app exit.
- **Custom Domains**: Paste a scoped Cloudflare API token, choose your domain, and add a subdomain and local port (`dev` + `3000` → `dev.example.com`). QuickFlare automatically provisions the tunnel, creates the DNS CNAME record, and routes traffic.

See the dedicated [Cloudflare API Token Guide](docs/CLOUDFLARE_API_TOKEN.md) for step-by-step instructions on generating your token.

## Documentation

- [Usage Guide](docs/USAGE.md) — Comprehensive walkthrough of features, route management, and autostart.
- [Cloudflare API Token Guide](docs/CLOUDFLARE_API_TOKEN.md) — Step-by-step token creation with required permissions.
- [Architecture](docs/ARCHITECTURE.md) — Win32 windowing, Gio immediate-mode UI, and Job Object process supervisor.
- [Codebase Guide](docs/CODEBASE.md) — Package layout, state synchronization, and testing patterns.

## Notes

- **Routes are public.** QuickFlare publishes the hostname and nothing else —
  it adds no login. Anything you put behind a route should do its own
  authentication, or not be published at all.
- **Windows only** right now.
- Your API token is encrypted with Windows DPAPI, tied to your Windows account.
  It's stored in `%APPDATA%\QuickFlare\config.json` and never in plaintext.
- Subdomains are one level deep (`app.example.com`, not `app.dev.example.com`)
  — the free Cloudflare certificate doesn't cover deeper names.
- One DNS record per route. QuickFlare doesn't touch `*.yourdomain`, so a
  domain you're already using stays fine.
- On launch it checks your routes against Cloudflare, restores the ones still
  there, and drops the ones that aren't.
- The binary isn't signed, so Windows will warn you the first time.
- `cloudflared` is bundled directly inside QuickFlare — no separate installation required.

## Build

```bash
go build -ldflags "-H windowsgui -s -w" -o build/QuickFlare-0.3.0.exe ./cmd/quickflare
go test ./...
```

Go 1.26+, no cgo.

## Licence

[Apache 2.0](LICENSE).
