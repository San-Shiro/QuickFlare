#![cfg_attr(windows, windows_subsystem = "windows")]

use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;
use anyhow::{Context, Result};
use slint::{ComponentHandle, ModelRc, VecModel};
use tokio::sync::Mutex;
use tray_icon::menu::{Menu, MenuEvent, MenuItem, PredefinedMenuItem};
use tray_icon::{MouseButton, MouseButtonState, TrayIcon, TrayIconBuilder, TrayIconEvent};

use quickflare_core::client::CloudflareClient;
use quickflare_core::config::Config;
use quickflare_core::ipc::{IpcRequest, IpcResponse, IpcServer};
use quickflare_core::models::{IngressRule, StoredRoute};
use quickflare_core::supervisor::Supervisor;

#[cfg(windows)]
mod win32 {
    use std::ffi::OsStr;
    use std::os::windows::ffi::OsStrExt;
    use windows_sys::Win32::Foundation::{CloseHandle, GetLastError, SetLastError, ERROR_ALREADY_EXISTS, HANDLE};
    use windows_sys::Win32::System::Threading::CreateMutexW;
    use windows_sys::Win32::UI::WindowsAndMessaging::{
        FindWindowW, GetForegroundWindow, SetForegroundWindow, ShowWindow, SW_RESTORE,
    };
    use quickflare_core::ipc::{IpcClient, IpcRequest};

    pub struct SingleInstanceMutex {
        handle: HANDLE,
    }

    impl Drop for SingleInstanceMutex {
        fn drop(&mut self) {
            unsafe {
                if !self.handle.is_null() {
                    CloseHandle(self.handle);
                }
            }
        }
    }

    fn try_acquire_mutex(name: &str) -> Result<Option<SingleInstanceMutex>, u32> {
        let wide: Vec<u16> = OsStr::new(name).encode_wide().chain(std::iter::once(0)).collect();
        unsafe {
            SetLastError(0);
            let handle = CreateMutexW(std::ptr::null(), 1, wide.as_ptr());
            let err = GetLastError();
            if handle.is_null() {
                return Err(err);
            }
            if err == ERROR_ALREADY_EXISTS {
                CloseHandle(handle);
                return Ok(None);
            }
            Ok(Some(SingleInstanceMutex { handle }))
        }
    }

    pub fn kill_zombie_instances() {
        let current_pid = std::process::id();
        let _ = std::process::Command::new("taskkill")
            .args([
                "/F",
                "/IM",
                "quickflare-tray.exe",
                "/FI",
                &format!("PID ne {}", current_pid),
            ])
            .output();
    }

    pub fn ensure_single_instance(rt: &tokio::runtime::Runtime) -> Result<SingleInstanceMutex, String> {
        const MUTEX_NAME: &str = r"Local\QuickFlare_Tray_Instance_Mutex";

        match try_acquire_mutex(MUTEX_NAME) {
            Ok(Some(guard)) => Ok(guard),
            Ok(None) => {
                // Mutex is already held. Check if existing instance is alive and responsive.
                tracing::info!("Existing QuickFlare tray instance mutex detected. Notifying existing instance...");
                let notified = rt.block_on(async {
                    tokio::time::timeout(
                        tokio::time::Duration::from_millis(1500),
                        IpcClient::send(&IpcRequest::Open),
                    ).await
                });

                match notified {
                    Ok(Ok(_)) => {
                        Err("Existing QuickFlare tray instance is already running. Opened existing window.".into())
                    }
                    _ => {
                        // Existing process is a hung/zombie process! Kill it and take over.
                        tracing::warn!("Existing instance is unresponsive. Terminating zombie process...");
                        kill_zombie_instances();
                        std::thread::sleep(std::time::Duration::from_millis(300));

                        match try_acquire_mutex(MUTEX_NAME) {
                            Ok(Some(guard)) => {
                                tracing::info!("Acquired single instance mutex after terminating zombie process.");
                                Ok(guard)
                            }
                            Ok(None) => Err("Could not acquire mutex even after zombie cleanup.".into()),
                            Err(code) => Err(format!("Failed to create mutex after cleanup: error code {}", code)),
                        }
                    }
                }
            }
            Err(code) => Err(format!("Failed to create mutex: error code {}", code)),
        }
    }

    pub fn focus_or_toggle(is_visible: bool, hide_fn: impl FnOnce(), show_fn: impl FnOnce()) {
        let title: Vec<u16> = OsStr::new("QuickFlare").encode_wide().chain(std::iter::once(0)).collect();
        unsafe {
            let hwnd = FindWindowW(std::ptr::null(), title.as_ptr());
            if !hwnd.is_null() {
                let foreground = GetForegroundWindow();
                if is_visible && foreground == hwnd {
                    hide_fn();
                } else {
                    show_fn();
                    ShowWindow(hwnd, SW_RESTORE);
                    SetForegroundWindow(hwnd);
                }
            } else {
                if is_visible {
                    hide_fn();
                } else {
                    show_fn();
                }
            }
        }
    }

    pub fn focus_window() {
        let title: Vec<u16> = OsStr::new("QuickFlare").encode_wide().chain(std::iter::once(0)).collect();
        unsafe {
            let hwnd = FindWindowW(std::ptr::null(), title.as_ptr());
            if !hwnd.is_null() {
                ShowWindow(hwnd, SW_RESTORE);
                SetForegroundWindow(hwnd);
            }
        }
    }
}

#[cfg(not(windows))]
mod win32 {
    pub struct SingleInstanceMutex;
    pub fn ensure_single_instance(_rt: &tokio::runtime::Runtime) -> Result<SingleInstanceMutex, String> {
        Ok(SingleInstanceMutex)
    }
    pub fn focus_or_toggle(is_visible: bool, hide_fn: impl FnOnce(), show_fn: impl FnOnce()) {
        if is_visible {
            hide_fn();
        } else {
            show_fn();
        }
    }
    pub fn focus_window() {}
}

slint::include_modules!();

fn main() -> Result<()> {
    std::panic::set_hook(Box::new(|info| {
        let msg = format!("PANIC: {:?}\nBacktrace:\n{:?}", info, std::backtrace::Backtrace::capture());
        if let Some(mut path) = dirs::data_local_dir() {
            path.push("QuickFlare");
            let _ = std::fs::create_dir_all(&path);
            path.push("panic.log");
            let _ = std::fs::write(&path, msg);
        }
    }));

    if let Some(mut path) = dirs::data_local_dir() {
        path.push("QuickFlare");
        let _ = std::fs::create_dir_all(&path);
        path.push("tray.log");
        if let Ok(file) = std::fs::OpenOptions::new().create(true).write(true).append(true).open(&path) {
            tracing_subscriber::fmt()
                .with_writer(std::sync::Mutex::new(file))
                .with_ansi(false)
                .init();
        }
    }

    tracing::info!("QuickFlare Tray initializing...");

    let rt = tokio::runtime::Builder::new_multi_thread()
        .enable_all()
        .build()
        .context("Failed to build tokio runtime")?;
    let _guard = rt.enter();

    let _single_instance_guard = match win32::ensure_single_instance(&rt) {
        Ok(guard) => {
            tracing::info!("Acquired single instance mutex successfully.");
            guard
        }
        Err(msg) => {
            tracing::info!("{}", msg);
            return Ok(());
        }
    };

    let main_window = MainWindow::new().context("Failed to initialize Slint MainWindow")?;
    let main_handle = main_window.as_weak();
    let is_window_visible = Arc::new(AtomicBool::new(true));

    let vis_close = is_window_visible.clone();
    main_window.window().on_close_requested(move || {
        vis_close.store(false, Ordering::SeqCst);
        slint::CloseRequestResponse::HideWindow
    });

    // 1. Setup System Tray & Menu
    let tray_menu = Menu::new();
    let item_open = MenuItem::new("Open QuickFlare", true, None);
    let item_pause = MenuItem::new("Pause All Routes", true, None);
    let item_resume = MenuItem::new("Resume All Routes", true, None);
    let item_quit = MenuItem::new("Quit", true, None);

    let _ = tray_menu.append(&item_open);
    let _ = tray_menu.append(&PredefinedMenuItem::separator());
    let _ = tray_menu.append(&item_pause);
    let _ = tray_menu.append(&item_resume);
    let _ = tray_menu.append(&PredefinedMenuItem::separator());
    let _ = tray_menu.append(&item_quit);

    // Create a 16x16 default icon buffer (RGBA)
    let icon_data = vec![0xF6, 0x82, 0x1F, 0xFF].repeat(16 * 16);
    let tray_icon = tray_icon::Icon::from_rgba(icon_data, 16, 16).unwrap();

    let _tray: TrayIcon = TrayIconBuilder::new()
        .with_menu(Box::new(tray_menu))
        .with_tooltip("QuickFlare - Cloudflare Reverse Proxy")
        .with_icon(tray_icon)
        .build()
        .context("Failed to build system tray icon")?;

    let supervisor = Arc::new(Mutex::new(Supervisor::new().0));
    let is_connected = Arc::new(AtomicBool::new(false));

    // 2. Initialize state from config
    let cfg = Config::load().unwrap_or_default();
    update_ui_state(&main_handle, &cfg, false);

    // 3. Start tunnel if logged in
    if !cfg.api_token.is_empty() && !cfg.routes_disabled {
        let sup = supervisor.clone();
        let conn = is_connected.clone();
        let token = cfg.api_token.clone();
        tokio::spawn(async move {
            if let Ok(client) = CloudflareClient::new(token) {
                if let Ok(accounts) = client.list_accounts().await {
                    if let Some(acc) = accounts.first() {
                        if let Ok(tunnel) = client.get_or_create_tunnel(&acc.id, "quickflare").await {
                            if let Ok(run_token) = client.get_tunnel_token(&acc.id, &tunnel.id).await {
                                let mut s = sup.lock().await;
                                if s.start(&run_token).await.is_ok() {
                                    conn.store(true, Ordering::SeqCst);
                                }
                            }
                        }
                    }
                }
            }
        });
    }

    // 4. Start local IPC server for CLI commands
    let sup_ipc = supervisor.clone();
    let conn_ipc = is_connected.clone();
    let handle_ipc = main_handle.clone();
    let is_visible_ipc = is_window_visible.clone();
    tokio::spawn(async move {
        let server = IpcServer::new();
        tracing::info!("Starting IPC server...");
        if let Err(e) = server.run(move |req| {
            let sup = sup_ipc.clone();
            let conn = conn_ipc.clone();
            let handle = handle_ipc.clone();
            let is_vis = is_visible_ipc.clone();
            async move {
                match req {
                    IpcRequest::Ping => IpcResponse::Pong,
                    IpcRequest::Status => {
                        let cfg = Config::load().unwrap_or_default();
                        IpcResponse::Status {
                            connected: conn.load(Ordering::SeqCst),
                            routes_count: cfg.routes.len(),
                        }
                    }
                    IpcRequest::Reload => {
                        let cfg = Config::load().unwrap_or_default();
                        let connected = conn.load(Ordering::SeqCst);
                        let _ = slint::invoke_from_event_loop(move || {
                            update_ui_state(&handle, &cfg, connected);
                        });
                        IpcResponse::Ok
                    }
                    IpcRequest::Open => {
                        let _ = slint::invoke_from_event_loop(move || {
                            if let Some(w) = handle.upgrade() {
                                let _ = w.show();
                                is_vis.store(true, Ordering::SeqCst);
                                win32::focus_window();
                            }
                        });
                        IpcResponse::Ok
                    }
                    IpcRequest::Quit => {
                        let mut s = sup.lock().await;
                        let _ = s.stop().await;
                        let _ = slint::invoke_from_event_loop(|| {
                            let _ = slint::quit_event_loop();
                        });
                        tokio::spawn(async {
                            tokio::time::sleep(tokio::time::Duration::from_millis(50)).await;
                            std::process::exit(0);
                        });
                        IpcResponse::Ok
                    }
                }
            }
        }).await {
            tracing::error!("IPC server error: {:?}", e);
        }
    });

    // 5. Connect UI Callbacks
    let handle_for_pause = main_handle.clone();
    let sup_pause = supervisor.clone();
    main_window.on_toggle_pause_all(move || {
        let handle = handle_for_pause.clone();
        let sup = sup_pause.clone();
        tokio::spawn(async move {
            let mut cfg = Config::load().unwrap_or_default();
            cfg.routes_disabled = !cfg.routes_disabled;
            let _ = cfg.save();

            let mut s = sup.lock().await;
            if cfg.routes_disabled {
                let _ = s.stop().await;
            }

            let _ = slint::invoke_from_event_loop(move || {
                update_ui_state(&handle, &cfg, !cfg.routes_disabled);
            });
        });
    });

    let handle_for_add = main_handle.clone();
    main_window.on_add_route(move |subdomain, port_str| {
        let handle = handle_for_add.clone();
        let subdomain = subdomain.to_string();
        let port: u16 = port_str.trim().parse().unwrap_or(8080);
        tokio::spawn(async move {
            let mut cfg = Config::load().unwrap_or_default();
            if cfg.api_token.is_empty() {
                return;
            }

            if let Ok(client) = CloudflareClient::new(cfg.api_token.clone()) {
                if let (Ok(accounts), Ok(zones)) = (client.list_accounts().await, client.list_zones().await) {
                    if let (Some(acc), Some(zone)) = (accounts.first(), zones.first()) {
                        let full_hostname = format!("{}.{}", subdomain, zone.name);
                        let target = format!("http://localhost:{}", port);

                        if let Ok(tunnel) = client.get_or_create_tunnel(&acc.id, "quickflare").await {
                            let _ = client.upsert_dns_cname(&zone.id, &full_hostname, &tunnel.id).await;

                            cfg.routes.retain(|r| r.hostname != full_hostname);
                            cfg.routes.push(StoredRoute {
                                hostname: full_hostname.clone(),
                                target,
                                zone_id: zone.id.clone(),
                            });

                            let rules: Vec<IngressRule> = cfg.routes.iter().map(|r| IngressRule {
                                hostname: Some(r.hostname.clone()),
                                service: r.target.clone(),
                            }).collect();

                            let _ = client.update_ingress_rules(&acc.id, &tunnel.id, rules).await;
                            let _ = cfg.save();

                            let _ = slint::invoke_from_event_loop(move || {
                                update_ui_state(&handle, &cfg, true);
                            });
                        }
                    }
                }
            }
        });
    });

    let handle_for_rm = main_handle.clone();
    main_window.on_remove_route(move |hostname| {
        let handle = handle_for_rm.clone();
        let hostname = hostname.to_string();
        tokio::spawn(async move {
            let mut cfg = Config::load().unwrap_or_default();
            if let Some(pos) = cfg.routes.iter().position(|r| r.hostname == hostname) {
                let route = cfg.routes.remove(pos);
                if !cfg.api_token.is_empty() {
                    if let Ok(client) = CloudflareClient::new(cfg.api_token.clone()) {
                        let _ = client.delete_dns_cname(&route.zone_id, &route.hostname).await;
                    }
                }
                let _ = cfg.save();
                let _ = slint::invoke_from_event_loop(move || {
                    update_ui_state(&handle, &cfg, true);
                });
            }
        });
    });

    main_window.on_copy_url(move |url| {
        let text = url.to_string();
        let _ = tokio::task::spawn_blocking(move || {
            if let Ok(mut clipboard) = arboard::Clipboard::new() {
                let _ = clipboard.set_text(text);
            }
        });
    });

    let sup_quit = supervisor.clone();
    main_window.on_quit(move || {
        let sup = sup_quit.clone();
        tokio::spawn(async move {
            let mut s = sup.lock().await;
            let _ = s.stop().await;
            let _ = slint::invoke_from_event_loop(|| {
                let _ = slint::quit_event_loop();
            });
            tokio::time::sleep(tokio::time::Duration::from_millis(50)).await;
            std::process::exit(0);
        });
    });

    // 6. Handle Tray Clicks & Events in background
    let handle_events = main_handle.clone();
    let is_visible_tray = is_window_visible.clone();
    tokio::task::spawn_blocking(move || {
        loop {
            if let Ok(event) = TrayIconEvent::receiver().recv() {
                if let TrayIconEvent::Click {
                    button: MouseButton::Left,
                    button_state: MouseButtonState::Up,
                    ..
                } = event {
                    let h = handle_events.clone();
                    let vis = is_visible_tray.clone();
                    let _ = slint::invoke_from_event_loop(move || {
                        if let Some(w) = h.upgrade() {
                            let current = vis.load(Ordering::SeqCst);
                            win32::focus_or_toggle(
                                current,
                                || {
                                    let _ = w.hide();
                                    vis.store(false, Ordering::SeqCst);
                                },
                                || {
                                    let _ = w.show();
                                    vis.store(true, Ordering::SeqCst);
                                },
                            );
                        }
                    });
                }
            }
        }
    });

    // Handle Tray Menu Events in background
    let id_open = item_open.id().clone();
    let id_pause = item_pause.id().clone();
    let id_resume = item_resume.id().clone();
    let id_quit = item_quit.id().clone();
    let handle_menu = main_handle.clone();
    let sup_menu = supervisor.clone();
    let is_visible_menu = is_window_visible.clone();
    tokio::task::spawn_blocking(move || {
        loop {
            if let Ok(event) = MenuEvent::receiver().recv() {
                if event.id == id_open {
                    let h = handle_menu.clone();
                    let vis = is_visible_menu.clone();
                    let _ = slint::invoke_from_event_loop(move || {
                        if let Some(w) = h.upgrade() {
                            let _ = w.show();
                            vis.store(true, Ordering::SeqCst);
                            win32::focus_window();
                        }
                    });
                } else if event.id == id_pause {
                    let sup = sup_menu.clone();
                    let h = handle_menu.clone();
                    tokio::spawn(async move {
                        let mut cfg = Config::load().unwrap_or_default();
                        cfg.routes_disabled = true;
                        let _ = cfg.save();
                        let mut s = sup.lock().await;
                        let _ = s.stop().await;
                        let _ = slint::invoke_from_event_loop(move || {
                            update_ui_state(&h, &cfg, false);
                        });
                    });
                } else if event.id == id_resume {
                    let sup = sup_menu.clone();
                    let h = handle_menu.clone();
                    tokio::spawn(async move {
                        let mut cfg = Config::load().unwrap_or_default();
                        cfg.routes_disabled = false;
                        let _ = cfg.save();
                        let mut connected = false;
                        if !cfg.api_token.is_empty() {
                            if let Ok(client) = CloudflareClient::new(cfg.api_token.clone()) {
                                if let Ok(accounts) = client.list_accounts().await {
                                    if let Some(acc) = accounts.first() {
                                        if let Ok(tunnel) = client.get_or_create_tunnel(&acc.id, "quickflare").await {
                                            if let Ok(run_token) = client.get_tunnel_token(&acc.id, &tunnel.id).await {
                                                let mut s = sup.lock().await;
                                                if s.start(&run_token).await.is_ok() {
                                                    connected = true;
                                                }
                                            }
                                        }
                                    }
                                }
                            }
                        }
                        let _ = slint::invoke_from_event_loop(move || {
                            update_ui_state(&h, &cfg, connected);
                        });
                    });
                } else if event.id == id_quit {
                    let sup = sup_menu.clone();
                    tokio::spawn(async move {
                        let mut s = sup.lock().await;
                        let _ = s.stop().await;
                        let _ = slint::invoke_from_event_loop(|| {
                            let _ = slint::quit_event_loop();
                        });
                        tokio::time::sleep(tokio::time::Duration::from_millis(50)).await;
                        std::process::exit(0);
                    });
                    break;
                }
            }
        }
    });

    tracing::info!("Showing MainWindow...");
    main_window.show().context("Failed to show MainWindow")?;
    tracing::info!("Entering event loop...");
    let res = slint::run_event_loop_until_quit();
    tracing::info!("Slint event loop exited: {:?}", res);
    std::process::exit(0);
}

fn update_ui_state(handle: &slint::Weak<MainWindow>, cfg: &Config, connected: bool) {
    if let Some(w) = handle.upgrade() {
        w.set_connected(connected);
        if let Some(dom) = &cfg.domain {
            w.set_domain_name(dom.clone().into());
        }
        w.set_routes_paused(cfg.routes_disabled);

        let items: Vec<RouteItem> = cfg
            .routes
            .iter()
            .map(|r| RouteItem {
                hostname: r.hostname.clone().into(),
                target: r.target.clone().into(),
                zone_id: r.zone_id.clone().into(),
            })
            .collect();

        w.set_routes(ModelRc::new(VecModel::from(items)));
    }
}
