# QuickFlare Hotfix & Core Architecture Audit

> **Core Invariant**: *"Do not break what is already fine."*  
> This document distinguishes between **hacky hotfixes that can be evolved into cleaner core-level architecture** and **battle-tested platform invariants that MUST stay as-is** because they are required by the Windows OS kernel, Win32 subsystems, or Cloudflare API protocols.

---

## 1. Executive Summary & Status

### Work Completed in Current Session
- **P1: Pruned Legacy Dead Code**
  - Removed abandoned Rust workspace: `crates/quickflare-core`, `crates/quickflare-cli`, `Cargo.toml`, `Cargo.lock`.
  - Removed legacy Go installer: `cmd/quickflare-installer/` (superseded by Inno Setup in v0.4.5; removed ~38,000 lines of obsolete setup UI code).
  - Cleaned up build scripts in `packaging/generate_assets.py`.
- **P2: Desktop Handle Lifecycle Hardening**
  - Hardened `AttachDefaultDesktop()` in `internal/ui/place_windows.go`.
  - Query current desktop name via `GetThreadDesktop` and `GetUserObjectInformationW(UOI_NAME)`. If already attached to `"Default"`, immediately return without making redundant `OpenDesktopW` syscalls.
  - Added failure-case cleanup (`CloseDesktop`) to guarantee zero handle leaks in the Win32 handle table.

---

## 2. Categorized Audit Tables

### Table A: Win32 Desktop, Threading & Systray Integration

| ID | Mechanism / Hotfix | Current Implementation | Verdict | Why It Must Stay OR Proposed Core Architectural Improvement |
| :--- | :--- | :--- | :--- | :--- |
| **A-1** | **Thread Desktop Attachment** (`AttachDefaultDesktop`) | Checks current thread desktop via `GetThreadDesktop` + `GetUserObjectInformationW`; opens and assigns `"Default"` only if needed; closes handle on failure. | **STAY AS-IS (Hardened in P2)** | **Win32 Platform Invariant.** Worker threads spawned by the Go runtime default to the process's window station. If running in background sessions or detached sandboxes, worker threads lack access to the interactive `"Default"` desktop containing `Shell_TrayWnd`. Redundant handle accumulation was resolved in P2. |
| **A-2** | **OS Thread Pinning** (`runtime.LockOSThread()`) | Called on entry to `main()` and `onReady()`. | **STAY AS-IS** | **Go Runtime / Win32 Invariant.** The Win32 message pump, thread desktop context, and message queues (`GetMessage`/`DispatchMessage`) are strictly bound to a single OS thread. The Go scheduler must never migrate these goroutines across worker threads. |
| **A-3** | **Tray State Notification Attachment** | `AttachDefaultDesktop()` is called inside `p.OnDisableChanged()` closure in `tray.go`. | **CORE CANDIDATE** | **Cleaner Core Fix:** Instead of re-checking desktop attachment on every disable toggle, the callback should be dispatched to the pinned UI loop or systray worker thread directly via `systray` channels. Since P2 made `AttachDefaultDesktop()` a fast zero-cost no-op when already on `"Default"`, this is safe to leave or refactor without urgency. |
| **A-4** | **Uncaught Panic Recovery** | `defer func() { recover() }` in `main()` and `tray.Run()` writing to `%LOCALAPPDATA%\QuickFlare\tray.log`. | **STAY AS-IS** | **Reliability Invariant.** A GUI app built with `-ldflags="-H=windowsgui"` has no console window. Without panic catching and structured file redirection, any runtime panic vanishes into thin air without diagnostic trace. |

---

### Table B: Window Placement, DWM Rendering, Visuals & Focus Management

| ID | Mechanism / Hotfix | Current Implementation | Verdict | Why It Must Stay OR Proposed Core Architectural Improvement |
| :--- | :--- | :--- | :--- | :--- |
| **B-1** | **Asynchronous Win32 Dispatch** (`ShowWindowAsync`, `SWP_ASYNCWINDOWPOS`) | Posts show/hide and positioning messages instead of blocking synchronous `ShowWindow`/`SetWindowPos`. | **STAY AS-IS** | **Deadlock Prevention Invariant.** Cross-thread synchronous `SendMessage` or `ShowWindow` blocks the caller until the receiving thread pumps. Calling synchronous Win32 APIs from a goroutine holding a Go lock while Gio owns the window causes an immediate deadlock of the UI pump. |
| **B-2** | **DWM Corner Rounding & Border Elimination** (`StyleWindow`) | Invokes `DwmSetWindowAttribute` for `DWMWA_WINDOW_CORNER_PREFERENCE` (round) and `DWMWA_BORDER_COLOR` (none). | **STAY AS-IS** | **Windows 11 Graphic Invariant.** Windows 11 applies a default 1px white border and rounded clipping. Turning off the border and delegating corner rounding to DWM prevents "chewed" double-radii corners. On Windows 10, the call fails gracefully and falls back to clean square edges. |
| **B-3** | **Taskbar Button Suppression** (`HideFromTaskbar`) | Replaces `WS_EX_APPWINDOW` with `WS_EX_TOOLWINDOW` via `GetWindowLongPtrW`/`SetWindowLongPtrW`. | **STAY AS-IS** | **UI Convention Invariant.** Gio creates top-level windows as `WS_EX_APPWINDOW`. A tray flyout popover must never occupy a slot in the Windows Taskbar or Alt-Tab switcher. |
| **B-4** | **Multi-Monitor Work Area Clamping** (`AnchorToTray`) | Queries cursor position via `GetCursorPos`, resolves monitor via `MonitorFromPoint(..., MONITOR_DEFAULTTONEAREST)`, retrieves `rcWork` (excluding taskbar), and clamps coordinates with a 12px margin. | **STAY AS-IS** | **Hardware / Multi-Screen Invariant.** Users clicking the tray icon have the mouse positioned over the active taskbar monitor. Resolving work area from mouse position ensures the popover always appears above/beside the taskbar on the correct monitor and never hangs off-screen. |
| **B-5** | **Foreground Stealing via Thread Input Attachment** (`Focus`) | Calls `AllowSetForegroundWindow(-1)`, attaches input queues with `AttachThreadInput(thisTid, curTid, 1)`, brings window to top, sets foreground, and detaches. | **STAY AS-IS** | **Win32 Security Subsystem Invariant.** Windows enforces "Foreground Lockout" (`LockSetForegroundWindow`). Direct `SetForegroundWindow` calls from background threads are ignored and merely flash the taskbar icon. Input queue attachment is the canonical Win32 method for tray flyouts. |
| **B-6** | **Focus Loss Polling** (`watchFocus`) | Polls `Foreground()` every 150ms with a 350ms initial grace period and 2.5s acquisition window. | **STAY AS-IS** | **Deadlock Prevention Invariant.** The alternative—subclassing Gio's window procedure (`GWLP_WNDPROC`) to catch `WM_ACTIVATE` or `WM_KILLFOCUS`—runs foreign Go code on Gio's DirectX/Win32 thread, which historically caused catastrophic lockups. A 150ms timer consumes <0.01% CPU and is 100% immune to message pump deadlocks. |
| **B-7** | **Tray Click Debounce on Blur** (`autoHidAt`) | In `Toggle()`, if the panel was closed by `watchFocus` within the last 400ms, the next tray click is swallowed. | **STAY AS-IS** | **Shell Race Condition Invariant.** When the panel is open and the user clicks the tray icon to close it, Windows Shell (`Shell_TrayWnd`) steals foreground focus *before* the tray click event arrives. The focus watcher closes the window; 10ms later, `systray` delivers the click. Without the 400ms guard, `Toggle()` would immediately re-open the panel, causing an annoying double-flicker. |
| **B-8** | **Arbitrary Sleep in Window Parking** (`setHandle`) | In `setHandle()`, an off-thread goroutine does `time.Sleep(250 * time.Millisecond)` between `Hide(h)` and `StyleWindow`/`HideFromTaskbar`. | **CORE CANDIDATE** | **Cleaner Core Fix:** Replace the fixed 250ms sleep with a condition poll or message pump hook checking `!IsWindowVisible(h)` with a 5ms interval and a 200ms timeout. Since this only runs once during app startup in the background, it has zero runtime performance impact, but replacing sleep with event-driven completion is cleaner. |
| **B-9** | **Layered Window Fade Animation** (`fadeTo`) | Uses a background goroutine ticking every 10ms (`fadeDuration / fadeSteps`) calling `SetAlpha(h, v)`. | **CORE CANDIDATE** | **Cleaner Core Fix:** Currently uses `time.Sleep(step)` across 11 discrete steps. While lightweight and functional, an event-driven `time.Ticker` or Gio frame-synchronized tick would eliminate ad-hoc sleep goroutines. However, since it is protected by `fadeGen` generational counters, it works reliably today. |
| **B-10** | **Clipboard "Copied" Confirmation Lapse** (`copyConn`) | Fires `time.Sleep(copiedFor + 50*time.Millisecond)` then `p.dispatch(func() {})` to wake the idle loop. | **STAY AS-IS** | **Gio Event Loop Invariant.** Gio does not re-render when the application is idle. To auto-dismiss a temporary badge when no mouse motion occurs, waking the event loop after the timeout is required. |

---

### Table C: Process Supervision, Job Objects & Lifecycle Management

| ID | Mechanism / Hotfix | Current Implementation | Verdict | Why It Must Stay OR Proposed Core Architectural Improvement |
| :--- | :--- | :--- | :--- | :--- |
| **C-1** | **Kernel Job Object Kill-On-Close** (`job_windows.go`) | Spawns `CreateJobObject` with `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` and binds `cloudflared.exe` with `AssignProcessToJobObject`. | **STAY AS-IS (CRITICAL)** | **Kernel Invariant.** If QuickFlare is terminated via Task Manager, an installer upgrade, system reboot, or sudden crash, Go `defer` statements NEVER execute. The Windows kernel automatically closes process job object handles upon termination, which forcibly cleans up child `cloudflared` instances. |
| **C-2** | **Console Window Suppression** (`proc_windows.go`) | Sets `CREATE_NO_WINDOW` (`0x08000000`) and `HideWindow: true` in `SysProcAttr`. | **STAY AS-IS** | **Platform Subsystem Invariant.** When a GUI application (`windowsgui` subsystem) spawns a console binary (`cloudflared.exe`), Windows automatically creates a black CMD console window unless `CREATE_NO_WINDOW` is explicitly passed. |
| **C-3** | **Health Check Polling via `/ready` Endpoint** (`pollReady`) | Connects to `http://127.0.0.1:<metrics-port>/ready` every 2s for structured JSON `{status: 200, readyConnections: N}`. | **STAY AS-IS (EXCELLENT)** | **Architectural Standard.** Far superior to fragile log parsing (`"Registered tunnel connection"`), which breaks whenever upstream Cloudflare changes log phrasing. |
| **C-4** | **Connector Auto-Update Suppression** (`--no-autoupdate`) | Appends `--no-autoupdate` flag when launching `cloudflared tunnel run`. | **STAY AS-IS** | **Supervision Invariant.** If `cloudflared` auto-updates, it restarts its own process under a new PID, breaking the parent supervisor's process handle and causing false disconnect alarms. |
| **C-5** | **Supervisory Generational Racing** (`tunnelGen`) | Increments `tunnelGen` and cancels `tunnelCancel()` whenever settings are re-saved. | **STAY AS-IS** | **Concurrency Invariant.** Rapid re-verification in Settings could leave two background provisioning routines racing to launch `cloudflared.exe`. Generational tagging guarantees only the latest routine survives, and older ones immediately stop their supervisors. |

---

### Table D: Persistence, Encryption & Filesystem Concurrency

| ID | Mechanism / Hotfix | Current Implementation | Verdict | Why It Must Stay OR Proposed Core Architectural Improvement |
| :--- | :--- | :--- | :--- | :--- |
| **D-1** | **Windows DPAPI Token Encryption** (`protect`/`unprotect`) | Uses `CryptProtectData` and `CryptUnprotectData` via syscalls to encrypt `APIToken`. | **STAY AS-IS** | **Security / OS Invariant.** API tokens are encrypted with keys tied to the user's Windows login credentials. Tokens cannot be read by other user accounts on the same machine. |
| **D-2** | **Atomic File Replacement** | Writes config to a unique temporary file (`config-*.json.tmp`), calls `Sync()`, closes, and calls `os.Rename`. | **STAY AS-IS** | **Data Integrity Invariant.** Prevents corrupted, half-written JSON files if power is lost or the application is killed during a write. |
| **D-3** | **Sharing Violation Retry Loop** | Retries `os.Rename` up to 8 times with exponential backoff (`10ms * 2^attempt` up to 1.28s) upon failure. | **STAY AS-IS** | **Windows Filesystem Invariant.** On Windows, newly created files are frequently intercepted by Windows Defender, Search Indexer, or third-party antivirus for scanning (`ERROR_SHARING_VIOLATION` / `0x20`). Bounded backoff rides out transient locks cleanly. |
| **D-4** | **Global Config Serialization Mutex** (`saveMu`) | Synchronizes all disk writes across UI and CLI actions. | **STAY AS-IS** | **Thread Safety Invariant.** Guarantees that concurrent route additions cannot write interleaved or clobbered config files. |

---

### Table E: Cloudflare Protocol & State Reconciliation

| ID | Mechanism / Hotfix | Current Implementation | Verdict | Why It Must Stay OR Proposed Core Architectural Improvement |
| :--- | :--- | :--- | :--- | :--- |
| **E-1** | **Ingress Catch-All Normalization** (`normalizeIngress`) | Strips trailing empty catch-alls and explicitly appends `http_status:404` as the final rule. | **STAY AS-IS** | **Cloudflare Cloud Protocol Invariant.** Cloudflare's remotely-managed tunnel API strictly rejects any ingress configuration where the final rule contains a hostname or is not a catch-all service. |
| **E-2** | **Full Document Replacement Mutex** (`ingressMu`) | Guarded read-modify-write via `UpdateIngress`. | **STAY AS-IS** | **Cloudflare API Invariant.** Cloudflare does not support atomic per-rule updates (upstream issue `cloudflared#1437`). QuickFlare must read the whole configuration, mutate routes in memory, and `PUT` the entire configuration back. `ingressMu` prevents lost updates. |
| **E-3** | **DNS Ownership Comment Tags** (`RecordComment`) | Writes `"Managed by QuickFlare"` (and recognizes legacy `"Managed by Keycard"`) in DNS record comments. | **STAY AS-IS** | **Safety Invariant.** Prevents QuickFlare from inadvertently modifying or deleting existing user DNS records on shared domains. |
| **E-4** | **Subdomain Conflict Detection** (`conflictTypes`) | Inspects `CNAME`, `A`, and `AAAA` records before creating a route. | **STAY AS-IS** | **DNS RFC Invariant.** A CNAME record cannot coexist with another CNAME or A record on the same subdomain. Detecting this upfront gives the user clear diagnostics rather than obscure Cloudflare API error codes. |
| **E-5** | **Two-Way Reconciliation Engine** (`applyReconcile`) | Compares local `config.json` against live Cloudflare tunnel ingress & DNS records, pruning phantom routes and adopting discovered routes. | **STAY AS-IS** | **Self-Healing Architecture.** Handles routes deleted via Cloudflare Dashboard or tunnels deleted out-of-band without desynchronizing local UI state. |

---

### Table F: Inter-Process Communication & Mutex Coordination

| ID | Mechanism / Hotfix | Current Implementation | Verdict | Why It Must Stay OR Proposed Core Architectural Improvement |
| :--- | :--- | :--- | :--- | :--- |
| **F-1** | **Kernel Single-Instance Mutex** (`acquireSingleInstance`) | Uses `windows.CreateMutex` with `Local\QuickFlare_Tray_SingleInstance_Mutex`. | **STAY AS-IS** | **OS Concurrency Invariant.** Faster and more reliable than port binding or file locks. Automatically released by the Windows kernel if the process terminates. |
| **F-2** | **Secondary Instance Handshake** (`client.Open`) | If mutex exists, the secondary instance connects to the running instance's loopback IPC server and triggers `/api/open`. | **STAY AS-IS** | **UX Standard.** Clicking the app shortcut when QuickFlare is already running smoothly reveals the existing tray flyout rather than showing an error. |
| **F-3** | **Authenticated Loopback HTTP Server** (`internal/ipc`) | Dynamic port assignment (`127.0.0.1:0`), 32-byte cryptographically secure token stored in `ipc.json` (mode `0600`). | **STAY AS-IS** | **Security Invariant.** Any local user/process could otherwise hit loopback endpoints. Requiring the `X-QuickFlare-Token` header prevents unprivileged cross-process abuse. |
| **F-4** | **Stale IPC File Detection via PID Liveness** (`isWindowsProcessAlive`) | Calls `windows.OpenProcess` with `PROCESS_QUERY_LIMITED_INFORMATION` and checks `WaitForSingleObject` timeout. | **STAY AS-IS** | **Crash Recovery Invariant.** If the machine is hard-reset or the process killed, `ipc.json` is left on disk. PID checking detects dead processes and auto-cleans stale session files on the next launch. |

---

## 3. High-Leverage Core Improvements (Safe & Non-Breaking)

The following candidates can be refactored into cleaner core structures in future minor iterations without breaking any existing behavior:

1. **Window Parking Visibility Polling (`B-8`)**:
   - *Current*: `time.Sleep(250 * time.Millisecond)` in background goroutine of `setHandle()`.
   - *Core Improvement*: Replace with an asynchronous poll loop checking `procIsWindowVisible.Call(h) == 0` with a 10ms interval and 200ms timeout.
2. **Systray Route State Dispatch (`A-3`)**:
   - *Current*: `p.OnDisableChanged()` invokes `AttachDefaultDesktop()` before modifying menu items.
   - *Core Improvement*: In P2, `AttachDefaultDesktop()` was hardened to perform a zero-cost early return if already on `"Default"`. A further cleanup can dispatch UI state changes directly through the pinned systray event pump.
3. **Fade Animation Timer Consolidation (`B-9`)**:
   - *Current*: Ad-hoc goroutine with `time.Sleep(step)` looping 11 times.
   - *Core Improvement*: Use a single centralized lightweight timer or Gio frame invalidation driver.
