# QuickFlare — Usage Guide

QuickFlare is a lightweight Windows tray application that exposes local network services to the internet through Cloudflare Tunnel with zero firewall configuration, no port forwarding, and no complex configuration files.

---

## 1. Installation

### Native Setup Installer (Recommended)
Download **`QuickFlare-0.4.5-Setup.exe`** from the [Releases](https://github.com/San-Shiro/QuickFlare/releases) page.
- Ultra-compact native Windows setup installer (~24 MB, reduced by 65% from 71 MB).
- Experience selection: **CLI with System Tray (Recommended)** or **CLI Only** (strict invariant: no tray-only install).
- Per-user installation to `%LOCALAPPDATA%\QuickFlare` (no administrator privileges required).
- Automatically registers the install directory into user `PATH` environment variable with instant shell broadcast so `quickflare` is immediately available in CMD and PowerShell.
- Creates Start Menu shortcuts for **QuickFlare** and **Uninstall QuickFlare**.
- Registers `quickflare` in Windows Run (`Win + R -> quickflare`) and Windows Apps & Features (`Settings -> Apps -> Installed apps`).
- Built-in clean uninstallation that prompts to gracefully unpublish active Cloudflare routes via API (default: enabled) and remove local credentials.

### Windows Installer (MSI)
Download **`QuickFlare-0.4.5-x64.msi`** from the [Releases](https://github.com/San-Shiro/QuickFlare/releases) page for enterprise or automated GPO deployments.

### Portable Package (CLI + Tray)
Download **`QuickFlare-0.4.5-windows-portable.zip`** from the [Releases](https://github.com/San-Shiro/QuickFlare/releases) page.
- Portable bundle containing `quickflare.exe`, `quickflare-tray.exe`, and `uninstall.cmd`.
- Extract to any folder. Run `quickflare path install` to register into user `PATH`.

### Standalone CLI Only
Download **`quickflare-0.4.5-windows-amd64.exe`** from the [Releases](https://github.com/San-Shiro/QuickFlare/releases) page.
- Lightweight standalone terminal binary for headless or script-driven environments.

---

## 2. Uninstallation & Reinstallation

QuickFlare provides clean ways to uninstall or reinstall:
1. **Uninstaller File**: Open `%LOCALAPPDATA%\QuickFlare` and double-click `uninstall.cmd`.
2. **Command Line**: Run `quickflare uninstall` (or `quickflare uninstall --yes`) in `cmd.exe` or PowerShell.
3. **Start Menu / Windows Settings**: Click **Uninstall QuickFlare** in the Start Menu or use **Windows Settings** -> **Installed Apps**.

### Uninstallation Options:
- **Active Route Teardown**: Prompts to remove active routes from Cloudflare (`[Y/n]`, default: enabled). When confirmed, gracefully deletes DNS CNAME records and tunnel ingress rules.
- **Config & Secret Cleanup**: Prompts to remove configuration files and stored API tokens from `%APPDATA%\QuickFlare` (`[Y/n]`, default: enabled).
- **Flags**:
  - `quickflare uninstall -y`: Non-interactive uninstall (defaults to tearing down active routes and deleting configs).
  - `quickflare uninstall --keep-routes`: Uninstall while preserving Cloudflare DNS records and tunnel routes.
  - `quickflare uninstall --keep-config`: Uninstall while preserving `%APPDATA%\QuickFlare` settings and tokens.

### Reinstallation:
- `quickflare reinstall`: Detects existing QuickFlare installation, uninstalls the current version cleanly, and reinstalls from the latest installer.
- `quickflare install`: Installs QuickFlare; if already present, automatically uninstalls the current version first then reinstalls.

---

## 2. Modes of Operation

QuickFlare provides two ways to publish your local ports:

### Quick Tunnels (No Account Needed)
Use this when you want to quickly share a local server (e.g. Next.js, Flask, Vite, Django, local webhook listener) with a friend, coworker, or client.

1. Click the QuickFlare tray icon to open the panel.
2. Select the **Quick** tab.
3. Click the **+** button.
4. Enter your local port number (e.g. `3000` or `8080`).
5. Click **Start**.
6. Within a few seconds, a secure `https://<random>.trycloudflare.com` URL is generated.
7. Click the **Copy** icon next to the link to copy the URL to your clipboard.
8. Click **Stop** whenever you are done to tear down the tunnel immediately.

> [!NOTE]
> Quick tunnels are ephemeral. Closing QuickFlare terminates all active quick tunnels.

---

### Named Routes (Your Own Custom Domain)
Use this when you want persistent, memorable URLs (e.g. `dashboard.yourdomain.com -> localhost:3000`) on a domain you manage in Cloudflare.

#### One-Time Setup:
1. Log into the [Cloudflare Dashboard](https://dash.cloudflare.com/profile/api-tokens).
2. Go to **My Profile** → **API Tokens** → **Create Token**.
3. Choose **Create Custom Token** with the following two permissions:
   - **Account** → **Cloudflare Tunnel** → **Edit**
   - **Zone** → **DNS** → **Edit**
4. Set Zone Resources to **Include** → **All zones** (or specific zones you want to use).
5. Click **Continue to summary** and **Create Token**.
6. In QuickFlare, paste the token into the API Token field and click **Verify token**.
7. QuickFlare will verify the token and discover your domains automatically.

> [!TIP]
> For a full step-by-step walkthrough with security and troubleshooting tips, see the [Cloudflare API Token Guide](CLOUDFLARE_API_TOKEN.md).

#### Adding a Named Route:
1. Select your target domain from the domain picker.
2. Click the **+** button.
3. Enter your desired subdomain (e.g. `api`, `app`, `dev`) and local port (e.g. `8000`).
4. Click **Publish**.
5. QuickFlare will automatically create or reuse the `quickflare` tunnel, register the CNAME DNS record, and route incoming traffic directly to your local port.

#### Stopping a Named Route:
- Click **Stop** on any route row.
- QuickFlare will ask for confirmation and remove the CNAME DNS record and ingress route on Cloudflare. Only DNS records created by QuickFlare (tagged with `Managed by QuickFlare`) are touched.

---

## 3. Settings & Options

Click the gear icon in the top right to open **Settings**:
- **API Token**: Update or verify your Cloudflare API token at any time.
- **Start with Windows**: Toggle automatic launch on Windows login (per-user, no admin prompt).
- **Engine & Version**: View the active Cloudflare tunnel engine version, QuickFlare version, and binary status.

---

## 4. Command-Line Interface (CLI)

QuickFlare includes a full command-line interface (`quickflare`) automatically registered in your user `PATH`. You can control QuickFlare directly from `cmd.exe`, PowerShell, or Windows Terminal:

```cmd
:: Start or stop the QuickFlare tray application
quickflare start
quickflare stop

:: Pause or resume route forwarding (client-side toggle)
quickflare pause
quickflare resume

:: Inspect runtime status, token, and routes
quickflare status
quickflare route ls

:: Publish or remove routes directly from the terminal
quickflare route add app --port 3000
quickflare route rm app.yourdomain.com

:: Ephemeral trycloudflare tunnel (no account needed)
quickflare quick --port 8080

:: Check or install PATH registration
quickflare path status
quickflare path install

:: Install or reinstall (uninstalls current first if present)
quickflare install
quickflare reinstall

:: Clean uninstallation (prompts to remove Cloudflare routes and configs)
quickflare uninstall
```
Changes made via the CLI automatically synchronize in real time with the running tray UI.
