# QuickFlare — Architecture & Design

QuickFlare is built as a self-contained, native Windows tray application with immediate-mode graphics, process lifecycle supervision, and direct integration with Cloudflare Tunnel.

---

## 1. High-Level Architecture

```
+-------------------------------------------------------------+
|                     QuickFlare Process                      |
|                                                             |
|  +---------------------+        +------------------------+  |
|  |   Gio GUI Thread    | <====> |     Tray Goroutine     |  |
|  |  (Immediate Mode)   |  MPSC  |  (fyne.io/systray)     |  |
|  +---------------------+        +------------------------+  |
|             ^                                |              |
|             | AppState                       | Win32        |
|             v                                v              |
|  +---------------------+        +------------------------+  |
|  | Cloudflare Client   |        | Windows Window Manager |  |
|  | (API v4 / HTTP std) |        | (DWM / Async Placement)|  |
|  +---------------------+        +------------------------+  |
|             |                                |              |
|             v                                v              |
|  +-------------------------------------------------------+  |
|  |               Process Supervisor & Engine             |  |
|  |  - Windows Kernel Job Object                          |  |
|  |  - Inbuilt cloudflared engine extraction & lifecycle  |  |
|  +-------------------------------------------------------+  |
+-------------------------------------------------------------+
                               |
                               | Spawns child processes
                               v
               +-------------------------------+
               |       cloudflared.exe         |
               |  (Outbound QUIC/TLS Tunnel)   |
               +-------------------------------+
                               |
                               v
                   Cloudflare Edge Anycast
```

---

## 2. Inbuilt Engine & Supervisor Lifecycle

### Inbuilt Binary Extraction
QuickFlare embeds a compressed copy of the official `cloudflared` executable (`v2026.9.1`).
- Portable runs: On first execution, the embedded binary is extracted to `%LOCALAPPDATA%\QuickFlare\bin\cloudflared.exe`.
- MSI installs: The binary is pre-installed directly into the application folder next to `QuickFlare.exe`.
- Runtime Resolution: `FindBinary()` prioritizes the application directory and the local app data bin directory, guaranteeing that a verified, compatible engine is used without requiring any user intervention or external tools.

### Coexistence with Existing `cloudflared`
QuickFlare is explicitly designed not to interfere with any pre-existing `cloudflared` installations:
- Configuration isolation: QuickFlare passes all arguments via CLI flags (`--url` or `--token` with `--metrics 127.0.0.1:<port>`). It never touches `%USERPROFILE%\.cloudflared\config.yml` or global certificates.
- Port isolation: All tunnel traffic is outbound over TLS/QUIC. For metrics, QuickFlare binds an ephemeral loopback port (`freePort()`), preventing collisions with standard ports.
- Service isolation: QuickFlare processes run as independent user-space children, coexisting seamlessly with any background Windows Services.

### Process Cleanup with Windows Job Objects
To prevent orphaned background processes:
- Every `cloudflared` child process is attached to a dedicated Windows **Job Object** with `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`.
- If QuickFlare is closed cleanly, terminated via Task Manager, crashes, or is replaced by an installer upgrade, the Windows kernel guarantees all child processes are killed immediately.

---

## 3. UI & Native Windows Integration

### Immediate-Mode GUI (Gio)
QuickFlare uses [Gio](https://gioui.org) (`gioui.org v0.10.2`) for high-performance immediate-mode rendering without webviews, Electron, or Chromium overhead.

### Win32 Async Window Placement
The flyout panel is created as a Gio window and positioned above the system tray:
- Asynchronous positioning: All window position and visibility calls from the tray thread utilize `SetWindowPos` with `SWP_ASYNCWINDOWPOS (0x4000)` and `ShowWindowAsync` to prevent deadlocks with the Gio rendering loop.
- Monitor detection: `AnchorToTray` resolves the target display from the cursor position at click time, properly supporting multi-monitor setups with mixed DPI scaling.
- DWM integration: QuickFlare uses `DWMWA_WINDOW_CORNER_PREFERENCE = DWMWCP_ROUND` on Windows 11 to leverage hardware-native rounded window corners, avoiding double-rounded edge artifacts.

---

## 4. Cloudflare State & Reconciliation

### Cloudflare as Source of Truth
Local configuration in `%APPDATA%\QuickFlare\config.json` serves only as a cache:
- On launch, `reconcileList` queries Cloudflare DNS and ingress rules to verify the actual state.
- Routes confirmed in both DNS and ingress are marked connected.
- Stale routes missing from Cloudflare are cleaned up automatically.
- Out-of-band routes found in ingress are adopted seamlessly.

### DNS Ownership Protection
Every DNS record created by QuickFlare carries a marker comment: `Managed by QuickFlare`.
QuickFlare will **never** overwrite, repoint, or delete a DNS record that does not carry this ownership comment. This ensures existing infrastructure or other tunnels on the domain are never disrupted.
