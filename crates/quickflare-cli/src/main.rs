use anyhow::{Context, Result};
use clap::{Parser, Subcommand};
use quickflare_core::client::CloudflareClient;
use quickflare_core::config::Config;
use quickflare_core::ipc::{IpcClient, IpcRequest, IpcResponse};
use quickflare_core::models::{IngressRule, StoredRoute};

#[derive(Parser, Debug)]
#[command(name = "quickflare", version = "0.4.6", about = "Publish localhost to the internet through Cloudflare Tunnel")]
struct Cli {
    #[command(subcommand)]
    command: Option<Commands>,
}

#[derive(Subcommand, Debug)]
enum Commands {
    /// Save your Cloudflare API token
    Login {
        #[arg(long, help = "Cloudflare API Token")]
        token: Option<String>,
    },
    /// Show current tunnel status, domain, and active routes
    Status,
    /// Manage tunnel routes
    Route {
        #[command(subcommand)]
        command: RouteCommands,
    },
    /// Enable and start all routes
    Up,
    /// Pause all routes
    Down,
    /// Launch the background system tray application
    Tray,
    /// Stop the background system tray application
    Stop,
}

#[derive(Subcommand, Debug)]
enum RouteCommands {
    /// Add and publish a new route (e.g. `quickflare route add app --port 3000`)
    Add {
        /// Subdomain or full hostname
        hostname: String,
        /// Local port (e.g. 3000)
        #[arg(long, default_value_t = 3000)]
        port: u16,
    },
    /// List all configured routes
    Ls,
    /// Remove an existing route
    Rm {
        /// Hostname to delete
        hostname: String,
    },
}

#[tokio::main]
async fn main() -> Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(tracing_subscriber::EnvFilter::from_default_env())
        .init();

    let cli = Cli::parse();

    match cli.command {
        Some(Commands::Login { token }) => cmd_login(token).await,
        Some(Commands::Status) | None => cmd_status().await,
        Some(Commands::Route { command }) => match command {
            RouteCommands::Add { hostname, port } => cmd_route_add(&hostname, port).await,
            RouteCommands::Ls => cmd_route_ls().await,
            RouteCommands::Rm { hostname } => cmd_route_rm(&hostname).await,
        },
        Some(Commands::Up) => cmd_up().await,
        Some(Commands::Down) => cmd_down().await,
        Some(Commands::Tray) => cmd_tray().await,
        Some(Commands::Stop) => cmd_stop().await,
    }
}

async fn cmd_login(token_opt: Option<String>) -> Result<()> {
    let token = match token_opt {
        Some(t) => t.trim().to_string(),
        None => {
            println!("Please enter your Cloudflare API Token:");
            let mut line = String::new();
            std::io::stdin().read_line(&mut line)?;
            line.trim().to_string()
        }
    };

    if token.is_empty() {
        anyhow::bail!("API token cannot be empty");
    }

    println!("Validating Cloudflare API token...");
    let client = CloudflareClient::new(token.clone())?;
    let accounts = client.list_accounts().await
        .context("Failed to authenticate with Cloudflare API. Check token permissions.")?;

    if accounts.is_empty() {
        anyhow::bail!("Token has no accessible Cloudflare accounts");
    }

    let zones = client.list_zones().await.unwrap_or_default();
    println!("✓ Authenticated as account: {} ({})", accounts[0].name, accounts[0].id);
    if !zones.is_empty() {
        println!("✓ Accessible domains: {}", zones.iter().map(|z| z.name.as_str()).collect::<Vec<_>>().join(", "));
    }

    let mut cfg = Config::load().unwrap_or_default();
    cfg.api_token = token;
    if let Some(first_zone) = zones.first() {
        if cfg.domain.is_none() {
            cfg.domain = Some(first_zone.name.clone());
        }
    }
    cfg.save()?;

    println!("✓ Token successfully encrypted with Windows DPAPI and saved to config.");
    let _ = notify_tray(IpcRequest::Reload).await;
    Ok(())
}

async fn cmd_status() -> Result<()> {
    let cfg = Config::load().unwrap_or_default();
    println!("QuickFlare v0.4.6 (Rust Native)");
    println!("===============================");

    if cfg.api_token.is_empty() {
        println!("Account: Not logged in. Run 'quickflare login' to authenticate.");
    } else {
        println!("Account Token: Configured (DPAPI Encrypted)");
        if let Some(domain) = &cfg.domain {
            println!("Active Domain: {}", domain);
        }
        println!("Routes Status: {}", if cfg.routes_disabled { "PAUSED" } else { "ACTIVE" });
        println!("Total Routes:  {}", cfg.routes.len());

        if !cfg.routes.is_empty() {
            println!("\nConfigured Routes:");
            println!("{:<30} {:<20}", "HOSTNAME", "TARGET");
            println!("{:-<30} {:-<20}", "", "");
            for r in &cfg.routes {
                println!("{:<30} {:<20}", r.hostname, r.target);
            }
        }
    }

    print!("\nBackground Tray Daemon: ");
    match notify_tray(IpcRequest::Status).await {
        Ok(IpcResponse::Status { connected, routes_count }) => {
            println!("RUNNING (Tunnel: {}, Active: {})", if connected { "Connected" } else { "Idle" }, routes_count);
        }
        Ok(_) => {
            println!("RUNNING");
        }
        Err(e) => {
            println!("STOPPED ({})", e);
        }
    }

    Ok(())
}

async fn cmd_route_add(hostname_input: &str, port: u16) -> Result<()> {
    let mut cfg = Config::load().unwrap_or_default();
    if cfg.api_token.is_empty() {
        anyhow::bail!("Not logged in. Run 'quickflare login' first.");
    }

    let client = CloudflareClient::new(cfg.api_token.clone())?;
    let accounts = client.list_accounts().await?;
    let account = accounts.first().context("No Cloudflare account found")?;

    let zones = client.list_zones().await?;
    let domain = cfg.domain.as_ref()
        .or_else(|| zones.first().map(|z| &z.name))
        .context("No domains available on account")?;

    let full_hostname = if hostname_input.contains('.') {
        hostname_input.to_string()
    } else {
        format!("{}.{}", hostname_input, domain)
    };

    let zone = zones.iter().find(|z| full_hostname.ends_with(&z.name))
        .context("Hostname does not match any zone on this account")?;

    println!("Setting up tunnel and DNS for https://{} -> localhost:{}...", full_hostname, port);

    let tunnel = client.get_or_create_tunnel(&account.id, "quickflare").await?;
    client.upsert_dns_cname(&zone.id, &full_hostname, &tunnel.id).await?;

    let target = format!("http://localhost:{}", port);
    cfg.routes.retain(|r| r.hostname != full_hostname);
    cfg.routes.push(StoredRoute {
        hostname: full_hostname.clone(),
        target: target.clone(),
        zone_id: zone.id.clone(),
    });

    let ingress_rules: Vec<IngressRule> = cfg.routes.iter().map(|r| IngressRule {
        hostname: Some(r.hostname.clone()),
        service: r.target.clone(),
    }).collect();

    client.update_ingress_rules(&account.id, &tunnel.id, ingress_rules).await?;
    cfg.save()?;

    println!("✓ Successfully published https://{} -> {}", full_hostname, target);
    let _ = notify_tray(IpcRequest::Reload).await;
    Ok(())
}

async fn cmd_route_ls() -> Result<()> {
    let cfg = Config::load().unwrap_or_default();
    if cfg.routes.is_empty() {
        println!("No routes configured. Add one with 'quickflare route add <name> --port <port>'.");
        return Ok(());
    }

    println!("{:<32} {:<22} {:<24}", "HOSTNAME", "TARGET", "ZONE ID");
    println!("{:-<32} {:-<22} {:-<24}", "", "", "");
    for r in &cfg.routes {
        println!("{:<32} {:<22} {:<24}", r.hostname, r.target, r.zone_id);
    }
    Ok(())
}

async fn cmd_route_rm(hostname: &str) -> Result<()> {
    let mut cfg = Config::load().unwrap_or_default();
    let index = cfg.routes.iter().position(|r| r.hostname == hostname)
        .context(format!("Route '{}' not found in configuration", hostname))?;

    let route = cfg.routes.remove(index);

    if !cfg.api_token.is_empty() {
        println!("Removing Cloudflare DNS record for {}...", route.hostname);
        if let Ok(client) = CloudflareClient::new(cfg.api_token.clone()) {
            let _ = client.delete_dns_cname(&route.zone_id, &route.hostname).await;
            if let Ok(accounts) = client.list_accounts().await {
                if let Some(acc) = accounts.first() {
                    if let Ok(tunnel) = client.get_or_create_tunnel(&acc.id, "quickflare").await {
                        let ingress_rules: Vec<IngressRule> = cfg.routes.iter().map(|r| IngressRule {
                            hostname: Some(r.hostname.clone()),
                            service: r.target.clone(),
                        }).collect();
                        let _ = client.update_ingress_rules(&acc.id, &tunnel.id, ingress_rules).await;
                    }
                }
            }
        }
    }

    cfg.save()?;
    println!("✓ Route '{}' removed successfully.", hostname);
    let _ = notify_tray(IpcRequest::Reload).await;
    Ok(())
}

async fn cmd_up() -> Result<()> {
    let mut cfg = Config::load().unwrap_or_default();
    cfg.routes_disabled = false;
    cfg.save()?;
    println!("✓ QuickFlare routes enabled.");
    let _ = notify_tray(IpcRequest::Reload).await;
    Ok(())
}

async fn cmd_down() -> Result<()> {
    let mut cfg = Config::load().unwrap_or_default();
    cfg.routes_disabled = true;
    cfg.save()?;
    println!("✓ QuickFlare routes paused.");
    let _ = notify_tray(IpcRequest::Reload).await;
    Ok(())
}

async fn cmd_tray() -> Result<()> {
    #[cfg(windows)]
    {
        // First check if an existing instance is already running
        if let Ok(_) = notify_tray(IpcRequest::Open).await {
            println!("✓ QuickFlare system tray is already running (opened window).");
            return Ok(());
        }

        let exe_path = std::env::current_exe()?
            .parent()
            .unwrap()
            .join("quickflare-tray.exe");

        if !exe_path.exists() {
            anyhow::bail!("quickflare-tray.exe not found alongside quickflare.exe at {:?}", exe_path);
        }

        let mut cmd = std::process::Command::new(&exe_path);
        cmd.spawn().with_context(|| format!("Failed to launch tray executable at {:?}", exe_path))?;
        println!("✓ QuickFlare system tray popover launched.");
        Ok(())
    }

    #[cfg(not(windows))]
    {
        println!("Tray application is supported on Windows.");
        Ok(())
    }
}

async fn cmd_stop() -> Result<()> {
    match notify_tray(IpcRequest::Quit).await {
        Ok(_) => {
            println!("✓ QuickFlare background instance stopped.");
        }
        Err(_) => {
            println!("QuickFlare background instance is not running.");
        }
    }
    Ok(())
}

async fn notify_tray(req: IpcRequest) -> Result<IpcResponse> {
    #[cfg(windows)]
    {
        IpcClient::send(&req).await
    }
    #[cfg(not(windows))]
    {
        Ok(IpcResponse::Ok)
    }
}
