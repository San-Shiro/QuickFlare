use std::io::{BufReader, Read};
use std::path::PathBuf;
use std::sync::Mutex;
use anyhow::{Context, Result};
use flate2::read::GzDecoder;

const EMBEDDED_CLOUDFLARED_GZ: &[u8] = include_bytes!("../assets/cloudflared.exe.gz");

pub const EMBEDDED_CLOUDFLARED_VERSION: &str = "2026.9.1";
pub const EXPECTED_UNCOMPRESSED_SIZE: u64 = 54976432;

static EXTRACT_LOCK: Mutex<()> = Mutex::new(());

/// Returns the path to the extracted cloudflared binary in %LOCALAPPDATA%\QuickFlare\bin\cloudflared.exe,
/// extracting it if missing or invalid.
pub fn ensure_embedded_binary() -> Result<PathBuf> {
    let _guard = EXTRACT_LOCK.lock().unwrap();

    let local_app = dirs::data_local_dir()
        .or_else(dirs::data_dir)
        .or_else(dirs::config_dir)
        .context("Failed to locate local application data directory")?;

    let bin_dir = local_app.join("QuickFlare").join("bin");
    std::fs::create_dir_all(&bin_dir)
        .with_context(|| format!("Failed to create binary directory {:?}", bin_dir))?;

    #[cfg(windows)]
    let binary_name = "cloudflared.exe";
    #[cfg(not(windows))]
    let binary_name = "cloudflared";

    let target_path = bin_dir.join(binary_name);

    if target_path.exists() {
        if let Ok(metadata) = target_path.metadata() {
            if metadata.len() == EXPECTED_UNCOMPRESSED_SIZE {
                return Ok(target_path);
            }
        }
    }

    tracing::info!(
        "Extracting embedded cloudflared v{} ({} bytes compressed)...",
        EMBEDDED_CLOUDFLARED_VERSION,
        EMBEDDED_CLOUDFLARED_GZ.len()
    );

    let tmp_path = target_path.with_extension("tmp");
    let mut decoder = GzDecoder::new(BufReader::new(EMBEDDED_CLOUDFLARED_GZ));
    let mut output_file = std::fs::File::create(&tmp_path)
        .with_context(|| format!("Failed to create temporary output file {:?}", tmp_path))?;

    let mut buf = [0u8; 65536];
    let mut total_written = 0u64;

    loop {
        let n = decoder.read(&mut buf).context("Failed decompressing cloudflared payload")?;
        if n == 0 {
            break;
        }
        std::io::Write::write_all(&mut output_file, &buf[..n])
            .context("Failed writing decompressed payload to disk")?;
        total_written += n as u64;
    }

    drop(output_file);

    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        std::fs::set_permissions(&tmp_path, std::fs::Permissions::from_mode(0o755))?;
    }

    std::fs::rename(&tmp_path, &target_path)
        .with_context(|| format!("Failed to atomically rename {:?} to {:?}", tmp_path, target_path))?;

    tracing::info!(
        "Successfully extracted cloudflared to {:?} ({} bytes)",
        target_path,
        total_written
    );

    Ok(target_path)
}
