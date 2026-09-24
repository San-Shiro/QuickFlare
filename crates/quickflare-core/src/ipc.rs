use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};

pub const IPC_PIPE_NAME: &str = r"\\.\pipe\quickflare-ipc";

#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum IpcRequest {
    Ping,
    Status,
    Reload,
    Open,
    Quit,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum IpcResponse {
    Pong,
    Status {
        connected: bool,
        routes_count: usize,
    },
    Ok,
    Error(String),
}

#[cfg(windows)]
pub use windows::{IpcClient, IpcServer};

#[cfg(windows)]
pub mod windows {
    use super::*;
    use tokio::net::windows::named_pipe::{ClientOptions, ServerOptions};
    use tokio::io::{AsyncReadExt, AsyncWriteExt};

    pub struct IpcServer {
        pipe_name: String,
    }

    impl IpcServer {
        pub fn new() -> Self {
            Self {
                pipe_name: IPC_PIPE_NAME.to_string(),
            }
        }

        pub async fn run<F, Fut>(self, handler: F) -> Result<()>
        where
            F: Fn(IpcRequest) -> Fut + Send + Sync + 'static,
            Fut: std::future::Future<Output = IpcResponse> + Send + 'static,
        {
            let handler = std::sync::Arc::new(handler);
            loop {
                let server = match ServerOptions::new()
                    .first_pipe_instance(false)
                    .create(&self.pipe_name)
                {
                    Ok(s) => s,
                    Err(e) => {
                        tracing::warn!("Failed to create named pipe instance: {:?}, retrying in 500ms", e);
                        tokio::time::sleep(tokio::time::Duration::from_millis(500)).await;
                        continue;
                    }
                };

                if let Err(e) = server.connect().await {
                    tracing::debug!("Client connection failed or disconnected: {:?}", e);
                    continue;
                }

                let handler_clone = handler.clone();
                tokio::spawn(async move {
                    let mut server = server;
                    let mut buf = vec![0u8; 4096];
                    if let Ok(n) = server.read(&mut buf).await {
                        if n > 0 {
                            if let Ok(req) = serde_json::from_slice::<IpcRequest>(&buf[..n]) {
                                let resp = handler_clone(req).await;
                                if let Ok(resp_bytes) = serde_json::to_vec(&resp) {
                                    let _ = server.write_all(&resp_bytes).await;
                                }
                            }
                        }
                    }
                });
            }
        }
    }

    pub struct IpcClient;

    impl IpcClient {
        pub async fn send(req: &IpcRequest) -> Result<IpcResponse> {
            use tokio::io::{AsyncReadExt, AsyncWriteExt};

            let mut client = None;
            for _ in 0..3 {
                match ClientOptions::new().open(IPC_PIPE_NAME) {
                    Ok(c) => {
                        client = Some(c);
                        break;
                    }
                    Err(e) if e.raw_os_error() == Some(231) => {
                        tokio::time::sleep(tokio::time::Duration::from_millis(50)).await;
                    }
                    Err(e) => return Err(e).context("Failed to connect to QuickFlare background instance"),
                }
            }
            let mut client = client.context("QuickFlare background instance is busy")?;

            let req_bytes = serde_json::to_vec(req).context("Failed to serialize IPC request")?;
            client.write_all(&req_bytes).await.context("Failed to write request to pipe")?;

            let mut buf = vec![0u8; 4096];
            let n = client.read(&mut buf).await.context("Failed to read response from pipe")?;

            let resp: IpcResponse = serde_json::from_slice(&buf[..n])
                .context("Failed to deserialize IPC response")?;

            Ok(resp)
        }
    }
}
