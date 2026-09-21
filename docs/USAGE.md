# QuickFlare — Usage Guide

QuickFlare is a lightweight Windows tray application that exposes local network services to the internet through Cloudflare Tunnel with zero firewall configuration, no port forwarding, and no complex configuration files.

---

## 1. Installation

### Installer (Recommended)
Download **`QuickFlare-0.3.0-x64.msi`** from the [Releases](https://github.com/San-Shiro/QuickFlare/releases) page.
- Per-user installation (no administrator privileges required).
- Places a shortcut in your Start Menu.
- Registers `quickflare` in Windows Run (`Win + R -> quickflare`).
- Includes the inbuilt Cloudflare Tunnel engine directly.

### Portable Executable
Download **`QuickFlare-0.3.0.exe`** from the [Releases](https://github.com/San-Shiro/QuickFlare/releases) page.
- Standalone portable binary with zero external dependencies.
- Automatically initializes and manages the inbuilt Cloudflare Tunnel engine.

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
