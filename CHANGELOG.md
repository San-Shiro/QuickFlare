# Changelog

## v0.3.1-dev - 2026-09-21 (pre-release)

**Linux support, as a preview.** QuickFlare gains a command-line interface and
runs on Linux for the first time. Windows is unchanged apart from two
installer fixes.

This is published as a pre-release because one thing is genuinely not finished:
on Linux the API token is stored in plain text. See *Known limitations*.

### Linux

- **`quickflare` command line tool** — `login`, `domains`,
  `route add` / `ls` / `rm`, `quick`, `run`, `status`, `reconcile`. Same rules
  as the tray app, because both now call the same code.
- **systemd integration** — `quickflare service install` writes a user unit,
  so routes survive logout and restart on failure instead of needing a
  terminal held open. The unit runs `quickflare run` rather than `cloudflared`
  directly, which keeps the tunnel token out of a world-readable unit file.
- **Packages** — `.deb`, `.rpm` and a `.tar.gz`, for `amd64` and `arm64`.
- **Process supervision** — connectors are bound to the parent with
  `Pdeathsig`, so they die with QuickFlare even when it is killed rather than
  closed. The Windows equivalent is a Job Object.
- **No bundled `cloudflared`** — distribution packages recommend it rather
  than shipping a second copy of a binary the package manager can supply. The
  Windows build still bundles it.

### Internal

- **`internal/core`** — provisioning and reconciliation moved out of the UI
  package, where they had been methods on the Gio panel. Nothing could reach
  them without linking a GUI toolkit, which on Linux means cgo, which rules
  out cross-compiling. A test now fails the build if `internal/core` ever
  imports Gio again.
- **Linux test suite** — 33 behavioural checks run in a container against a
  real Linux userland, including `systemd-analyze verify` on the generated
  unit. Cross-compiling proves a binary links, not that it behaves.

### Fixed (Windows)

- **The installer carried `cloudflared` twice** — once embedded in
  `QuickFlare.exe` and again as a separate file, and the embedded copy was
  never used because the installed one always won. The MSI drops from 39.1 MB
  to 23.4 MB.
- **Uninstall left 52 MB behind** — the extracted engine was written by the
  app, so the installer had no record of it and neither it nor its directory
  was removed.
- **The release MSI was built as x86** while shipping a 64-bit binary, which
  registered the product in the 32-bit registry view.

### Known limitations

- **The Linux API token is stored in plain text**, in a file readable only by
  you (0600). Windows encrypts it with DPAPI. `quickflare login` and
  `quickflare status` both say so. libsecret integration is the next piece of
  work, and is why this is a pre-release.
- **No tray app on Linux.** The command line tool is the whole interface.
- The Linux build has been tested in containers and by cross-compilation, not
  on a physical desktop.
- `systemctl --user` behaviour is unverified; the generated unit is validated,
  the handshake with a running systemd is not.

## v0.3.0 - 2026-09-21

First release.

QuickFlare publishes a localhost port to the internet through Cloudflare
Tunnel with cloudflared. No port forwarding, no config files.

### Features

- **Custom Domains** — Automatically provisions tunnels, creates DNS records, and routes traffic for your Cloudflare domains.
- **Temporary Domains** — Instant `trycloudflare.com` tunnels for any local port with no account required.

