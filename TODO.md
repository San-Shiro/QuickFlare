# QuickFlare Feature Roadmap & Backlog (TODO)

This document tracks upcoming feature proposals and architectural initiatives beyond the active **P1** scope.

---

## 🛡️ Phase 0 (P0) — Security & Zero Trust Integration

### 1. Cloudflare Access / Zero Trust Application Integration
- **Objective**: Protect public localhost routes with Cloudflare Access without running a local authentication proxy.
- **Architecture**:
  - Integrate with Cloudflare Access API endpoints (`/accounts/<id>/access/apps` and `/accounts/<id>/access/apps/<app_id>/policies`).
  - Wire into existing two-way reconciler: When a route is marked "Protected", verify its corresponding Access Application and Policy exist.
  - Support common policy rules: Email PIN / One-Time Pin, Google/GitHub IdP, or IP range whitelists.
- **Blast Radius**: Medium (requires additional API token permissions: `Access: Apps and Policies: Edit`).
- **Status**: Backlog / Post-P1.

---

## 🚀 Deferred Phase 1 (P1) Features

### 2. Raw TCP / SSH / RDP Tunneling
- **Objective**: Publish non-HTTP local services (SSH on port 22, RDP on port 3389, raw TCP databases on port 5432/3306).
- **Architecture**:
  - `cloudflared` natively accepts `ssh://localhost:22`, `rdp://localhost:3389`, and `tcp://localhost:5432` in ingress rules.
  - Non-HTTP services require client-side proxying via `cloudflared access`.
  - Add client command generator to QuickFlare UI:
    ```bash
    cloudflared access ssh --hostname ssh.yourdomain.com
    ```
- **Status**: Deferred from initial P1.

### 3. Access Service Tokens
- **Objective**: Machine-to-machine authentication for CI/CD pipelines, automated webhooks, and background daemons.
- **Architecture**:
  - Provision Cloudflare Access Service Tokens via API (`/accounts/<id>/access/service_tokens`).
  - Store generated `CF-Access-Client-Secret` in QuickFlare's Windows DPAPI encrypted vault (`internal/config`).
  - Copyable header credentials for API testing (`CF-Access-Client-Id` and `CF-Access-Client-Secret`).
- **Status**: Deferred from initial P1.

---

## 🧭 Phase 2 (P2) — Advanced & Specialized Workflows

### 4. Outbound Webhook Dispatcher
- **Objective**: Push real-time JSON notifications to Slack, Discord, or automation scripts on tunnel lifecycle events.
- **Architecture**:
  - Lightweight stdlib `net/http` worker consuming supervisor events (`StateConnected`, `StateReconnecting`, `StateFailed`).
  - Configurable webhook URL and secret header.

### 5. Multi-Account & Multi-Profile Support
- **Objective**: Enable switching between distinct Cloudflare accounts or client workspaces without re-authenticating.
- **Architecture**:
  - Profile switcher dropdown in UI header.
  - Multi-tenant config model (`profiles: map[string]ProfileConfig`) backed by isolated DPAPI encrypted tokens.

### 6. Local Basic-Auth Proxy for Ephemeral Quick Tunnels
- **Objective**: Protect free `trycloudflare.com` tunnels where Cloudflare Access is unavailable.
- **Architecture**:
  - Optional, opt-in local `net/http.ReverseProxy` binding on a loopback port in front of the local service.
  - Enforces HTTP Basic Authentication (`Authorization: Basic <base64>`).
  - Strict isolation: Only enabled when explicitly toggled on a Quick Tunnel route.

### 7. Global Windows System Hotkey
- **Objective**: Toggle flyout panel visibility or pause/resume routes instantly via keyboard shortcut.
- **Architecture**:
  - Register global hotkey via Win32 `RegisterHotKey` (e.g. `Win + Alt + Q`).
  - Handle `WM_HOTKEY` in main Win32 message loop.

### 8. Cross-Platform Abstraction Layer (`platform` package)
- **Objective**: Pave the way for future macOS and Linux native UI builds without rewriting core domain logic.
- **Architecture**:
  - Define unified Go interfaces: `CredentialStore`, `AutostartManager`, `SingleInstanceLock`, `TrayProvider`.
  - Move Windows-specific implementations (`internal/ui/place_windows.go`, `job_windows.go`, `instance_windows.go`) behind build-tagged interface implementations.
