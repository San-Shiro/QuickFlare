use std::process::Stdio;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;
use anyhow::{Context, Result};
use tokio::io::{AsyncBufReadExt, BufReader};
use tokio::process::{Child, Command};
use tokio::sync::broadcast;

use crate::embedded::ensure_embedded_binary;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum TunnelState {
    Stopped,
    Starting,
    Connected,
    Reconnecting,
    Failed,
}

pub struct Supervisor {
    child: Option<Child>,
    state_tx: broadcast::Sender<TunnelState>,
    running: Arc<AtomicBool>,
}

impl Supervisor {
    pub fn new() -> (Self, broadcast::Receiver<TunnelState>) {
        let (state_tx, state_rx) = broadcast::channel(16);
        let supervisor = Self {
            child: None,
            state_tx,
            running: Arc::new(AtomicBool::new(false)),
        };
        (supervisor, state_rx)
    }

    /// Spawns the cloudflared process running the specified tunnel token.
    pub async fn start(&mut self, token: &str) -> Result<()> {
        if self.running.load(Ordering::SeqCst) {
            self.stop().await?;
        }

        let binary_path = ensure_embedded_binary()
            .context("Failed to ensure cloudflared binary is extracted")?;

        let _ = self.state_tx.send(TunnelState::Starting);
        self.running.store(true, Ordering::SeqCst);

        let mut cmd = Command::new(&binary_path);
        cmd.args(["tunnel", "run", "--token", token]);
        cmd.stdout(Stdio::piped());
        cmd.stderr(Stdio::piped());

        #[cfg(windows)]
        {
            // CREATE_NO_WINDOW = 0x08000000
            const CREATE_NO_WINDOW: u32 = 0x08000000;
            cmd.creation_flags(CREATE_NO_WINDOW);
        }

        let mut child = cmd.spawn().with_context(|| {
            format!("Failed to spawn cloudflared process from {:?}", binary_path)
        })?;

        #[cfg(windows)]
        {
            if let Some(raw_handle) = child.raw_handle() {
                assign_to_job_object(raw_handle);
            }
        }

        let stderr = child.stderr.take();
        let stdout = child.stdout.take();
        let state_tx_err = self.state_tx.clone();
        let running_err = self.running.clone();
        if let Some(err) = stderr {
            tokio::spawn(async move {
                let mut lines = BufReader::new(err).lines();
                while let Ok(Some(line)) = lines.next_line().await {
                    tracing::debug!("[cloudflared stderr] {}", line);
                    if line.contains("Registered tunnel connection")
                        || (line.contains("Connection") && line.contains("registered"))
                    {
                        let _ = state_tx_err.send(TunnelState::Connected);
                    } else if line.contains("Retrying connection in") {
                        let _ = state_tx_err.send(TunnelState::Reconnecting);
                    }
                }
                if running_err.load(Ordering::SeqCst) {
                    let _ = state_tx_err.send(TunnelState::Stopped);
                }
            });
        }

        let state_tx_out = self.state_tx.clone();
        let running_out = self.running.clone();
        if let Some(out) = stdout {
            tokio::spawn(async move {
                let mut lines = BufReader::new(out).lines();
                while let Ok(Some(line)) = lines.next_line().await {
                    tracing::debug!("[cloudflared stdout] {}", line);
                    if line.contains("Registered tunnel connection")
                        || (line.contains("Connection") && line.contains("registered"))
                    {
                        let _ = state_tx_out.send(TunnelState::Connected);
                    } else if line.contains("Retrying connection in") {
                        let _ = state_tx_out.send(TunnelState::Reconnecting);
                    }
                }
                if running_out.load(Ordering::SeqCst) {
                    let _ = state_tx_out.send(TunnelState::Stopped);
                }
            });
        }

        self.child = Some(child);
        Ok(())
    }

    /// Stops the running cloudflared process.
    pub async fn stop(&mut self) -> Result<()> {
        self.running.store(false, Ordering::SeqCst);
        if let Some(mut child) = self.child.take() {
            let _ = child.kill().await;
            let _ = child.wait().await;
        }
        let _ = self.state_tx.send(TunnelState::Stopped);
        Ok(())
    }

    pub fn is_running(&self) -> bool {
        self.running.load(Ordering::SeqCst)
    }
}

impl Drop for Supervisor {
    fn drop(&mut self) {
        if let Some(mut child) = self.child.take() {
            let _ = child.start_kill();
        }
    }
}

#[cfg(windows)]
fn assign_to_job_object(process_handle: std::os::windows::io::RawHandle) {
    use std::sync::Once;
    use windows_sys::Win32::Foundation::{HANDLE, INVALID_HANDLE_VALUE};
    use windows_sys::Win32::System::JobObjects::{
        AssignProcessToJobObject, CreateJobObjectW, SetInformationJobObject,
        JobObjectExtendedLimitInformation, JOBOBJECT_EXTENDED_LIMIT_INFORMATION,
        JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
    };

    static INIT: Once = Once::new();
    static mut JOB_HANDLE: HANDLE = INVALID_HANDLE_VALUE;

    unsafe {
        INIT.call_once(|| {
            let handle = CreateJobObjectW(std::ptr::null_mut(), std::ptr::null());
            if handle != INVALID_HANDLE_VALUE && !handle.is_null() {
                let mut info: JOBOBJECT_EXTENDED_LIMIT_INFORMATION = std::mem::zeroed();
                info.BasicLimitInformation.LimitFlags = JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE;
                let res = SetInformationJobObject(
                    handle,
                    JobObjectExtendedLimitInformation,
                    &info as *const _ as *const std::ffi::c_void,
                    std::mem::size_of::<JOBOBJECT_EXTENDED_LIMIT_INFORMATION>() as u32,
                );
                if res != 0 {
                    JOB_HANDLE = handle;
                }
            }
        });

        if JOB_HANDLE != INVALID_HANDLE_VALUE && !JOB_HANDLE.is_null() {
            AssignProcessToJobObject(JOB_HANDLE, process_handle as HANDLE);
        }
    }
}
