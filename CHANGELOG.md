# Changelog

## v0.3.1-dev - 2026-09-21

Linux support. Windows unchanged apart from installer fixes.

### Linux

- `quickflare` command line tool: `login`, `domains`, `route add|ls|rm`,
  `quick`, `run`, `status`, `reconcile`.
- `quickflare service install` writes a systemd user unit, so routes survive
  logout and restart on failure.
- Packages for amd64 and arm64: `.deb`, `.rpm`, tarball, bare binary.
- `cloudflared` is not bundled on Linux; the packages recommend it.
- **The API token is stored in plain text**, readable only by you (0600).
  Windows encrypts it with DPAPI. `login` and `status` both say so.
- No tray app. The command line is the whole interface.

### Windows

- The installer shipped `cloudflared` twice and never used the embedded copy.
  MSI: 39.1 MB to 23.4 MB.
- Uninstall left 52 MB behind.
- The release MSI was built as x86 while shipping a 64-bit binary.

### Tested

`.deb` on Ubuntu and `.rpm` on Fedora: install, run, remove. 33 behavioural
checks in a container, including `systemd-analyze verify` on the generated
unit. Not yet run on a physical Linux desktop.

## v0.3.0 - 2026-09-21

First release.

QuickFlare publishes a localhost port to the internet through Cloudflare
Tunnel with cloudflared. No port forwarding, no config files.

### Features

- **Custom Domains** — Automatically provisions tunnels, creates DNS records, and routes traffic for your Cloudflare domains.
- **Temporary Domains** — Instant `trycloudflare.com` tunnels for any local port with no account required.

