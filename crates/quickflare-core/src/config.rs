use std::path::{Path, PathBuf};
use anyhow::{Context, Result};
use base64::prelude::*;
use serde::{Deserialize, Serialize};

use crate::models::StoredRoute;

const CONFIG_DIR_NAME: &str = "QuickFlare";
const CONFIG_FILE_NAME: &str = "config.json";
const DPAPI_ENTROPY: &[u8] = b"quickflare.cloudflare.api-token.v1";

/// Persistent application configuration.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize, Default)]
pub struct Config {
    /// In-memory plaintext API token (not serialized directly to JSON).
    #[serde(skip)]
    pub api_token: String,

    /// DPAPI-protected base64 token written to disk.
    #[serde(rename = "api_token_enc", skip_serializing_if = "Option::is_none")]
    pub token_enc: Option<String>,

    /// Last selected domain / zone name.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub domain: Option<String>,

    /// Stored route list.
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub routes: Vec<StoredRoute>,

    /// True if routes were paused/disabled.
    #[serde(default)]
    pub routes_disabled: bool,

    /// If true, restores last active route state on launch.
    #[serde(default)]
    pub restore_last_state: bool,
}

impl Config {
    /// Returns the default path to config.json (%LOCALAPPDATA%\QuickFlare\config.json),
    /// falling back to %APPDATA%\QuickFlare\config.json if already present.
    pub fn file_path() -> Result<PathBuf> {
        let local_base = dirs::data_local_dir().or_else(dirs::config_dir)
            .context("Failed to locate user local data directory")?;
        let local_path = local_base.join(CONFIG_DIR_NAME).join(CONFIG_FILE_NAME);

        if !local_path.exists() {
            if let Some(roaming_base) = dirs::config_dir() {
                let roaming_path = roaming_base.join(CONFIG_DIR_NAME).join(CONFIG_FILE_NAME);
                if roaming_path.exists() {
                    return Ok(roaming_path);
                }
            }
        }

        Ok(local_path)
    }

    /// Loads the configuration from disk, decrypting the stored API token if present.
    pub fn load() -> Result<Self> {
        let path = Self::file_path()?;
        Self::load_from_path(&path)
    }

    /// Loads the configuration from a specific file path.
    pub fn load_from_path(path: &Path) -> Result<Self> {
        if !path.exists() {
            return Ok(Config::default());
        }

        let content = std::fs::read_to_string(path)
            .with_context(|| format!("Failed to read config file at {:?}", path))?;

        let mut config: Config = serde_json::from_str(&content)
            .with_context(|| format!("Failed to parse config JSON from {:?}", path))?;

        if let Some(enc_token) = &config.token_enc {
            if !enc_token.is_empty() {
                match unprotect_token(enc_token) {
                    Ok(plain) => config.api_token = plain,
                    Err(e) => tracing::warn!("Failed to decrypt API token: {:#}", e),
                }
            }
        }

        Ok(config)
    }

    /// Saves the configuration to disk atomically, encrypting the API token.
    pub fn save(&mut self) -> Result<()> {
        let local_base = dirs::data_local_dir().or_else(dirs::config_dir)
            .context("Failed to locate user local data directory")?;
        let target_path = local_base.join(CONFIG_DIR_NAME).join(CONFIG_FILE_NAME);
        self.save_to_path(&target_path)
    }

    /// Saves the configuration to a specific file path atomically.
    pub fn save_to_path(&mut self, path: &Path) -> Result<()> {
        if let Some(parent) = path.parent() {
            std::fs::create_dir_all(parent)
                .with_context(|| format!("Failed to create config directory {:?}", parent))?;
        }

        if !self.api_token.is_empty() {
            let enc = protect_token(&self.api_token)?;
            self.token_enc = Some(enc);
        } else {
            self.token_enc = None;
        }

        let json_data = serde_json::to_string_pretty(self)
            .context("Failed to serialize configuration to JSON")?;

        let tmp_path = path.with_extension("tmp");
        std::fs::write(&tmp_path, json_data)
            .with_context(|| format!("Failed to write temporary config {:?}", tmp_path))?;

        if let Err(_) = std::fs::rename(&tmp_path, path) {
            let _ = std::fs::copy(&tmp_path, path);
            let _ = std::fs::remove_file(&tmp_path);
        }

        Ok(())
    }
}

/// Protects a plaintext token using Windows DPAPI with the hardcoded QuickFlare entropy.
pub fn protect_token(plain: &str) -> Result<String> {
    #[cfg(windows)]
    {
        use std::ptr::null_mut;
        use windows_sys::Win32::Security::Cryptography::{
            CryptProtectData, CRYPTPROTECT_UI_FORBIDDEN, CRYPT_INTEGER_BLOB,
        };
        extern "system" {
            fn LocalFree(hmem: *mut std::ffi::c_void) -> *mut std::ffi::c_void;
        }

        let plain_bytes = plain.as_bytes();
        let mut in_blob = CRYPT_INTEGER_BLOB {
            cbData: plain_bytes.len() as u32,
            pbData: plain_bytes.as_ptr() as *mut u8,
        };

        let mut entropy_blob = CRYPT_INTEGER_BLOB {
            cbData: DPAPI_ENTROPY.len() as u32,
            pbData: DPAPI_ENTROPY.as_ptr() as *mut u8,
        };

        let mut out_blob = CRYPT_INTEGER_BLOB {
            cbData: 0,
            pbData: null_mut(),
        };

        let success = unsafe {
            CryptProtectData(
                &mut in_blob,
                null_mut(),
                &mut entropy_blob,
                null_mut(),
                null_mut(),
                CRYPTPROTECT_UI_FORBIDDEN,
                &mut out_blob,
            )
        };

        if success == 0 {
            anyhow::bail!("CryptProtectData failed with error {}", unsafe {
                windows_sys::Win32::Foundation::GetLastError()
            });
        }

        let encrypted_bytes = unsafe {
            std::slice::from_raw_parts(out_blob.pbData, out_blob.cbData as usize).to_vec()
        };

        unsafe {
            LocalFree(out_blob.pbData as _);
        }

        Ok(BASE64_STANDARD.encode(encrypted_bytes))
    }

    #[cfg(not(windows))]
    {
        Ok(BASE64_STANDARD.encode(plain.as_bytes()))
    }
}

/// Unprotects a base64 ciphertext token using Windows DPAPI with the hardcoded QuickFlare entropy.
pub fn unprotect_token(encoded: &str) -> Result<String> {
    let encrypted_bytes = BASE64_STANDARD
        .decode(encoded.trim())
        .context("Failed to base64-decode encrypted token")?;

    #[cfg(windows)]
    {
        use std::ptr::null_mut;
        use windows_sys::Win32::Security::Cryptography::{
            CryptUnprotectData, CRYPTPROTECT_UI_FORBIDDEN, CRYPT_INTEGER_BLOB,
        };
        extern "system" {
            fn LocalFree(hmem: *mut std::ffi::c_void) -> *mut std::ffi::c_void;
        }

        let mut in_blob = CRYPT_INTEGER_BLOB {
            cbData: encrypted_bytes.len() as u32,
            pbData: encrypted_bytes.as_ptr() as *mut u8,
        };

        let mut entropy_blob = CRYPT_INTEGER_BLOB {
            cbData: DPAPI_ENTROPY.len() as u32,
            pbData: DPAPI_ENTROPY.as_ptr() as *mut u8,
        };

        let mut out_blob = CRYPT_INTEGER_BLOB {
            cbData: 0,
            pbData: null_mut(),
        };

        let success = unsafe {
            CryptUnprotectData(
                &mut in_blob,
                null_mut(),
                &mut entropy_blob,
                null_mut(),
                null_mut(),
                CRYPTPROTECT_UI_FORBIDDEN,
                &mut out_blob,
            )
        };

        if success == 0 {
            anyhow::bail!("CryptUnprotectData failed with error {}", unsafe {
                windows_sys::Win32::Foundation::GetLastError()
            });
        }

        let decrypted_bytes = unsafe {
            std::slice::from_raw_parts(out_blob.pbData, out_blob.cbData as usize).to_vec()
        };

        unsafe {
            LocalFree(out_blob.pbData as _);
        }

        let plain = String::from_utf8(decrypted_bytes)
            .context("Decrypted token is not valid UTF-8 string")?;

        Ok(plain)
    }

    #[cfg(not(windows))]
    {
        let plain = String::from_utf8(encrypted_bytes)
            .context("Decoded token is not valid UTF-8 string")?;
        Ok(plain)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_protect_unprotect_roundtrip() {
        let token = "test_cloudflare_token_12345!@#";
        let encrypted = protect_token(token).expect("protect should succeed");
        assert_ne!(token, encrypted);

        let decrypted = unprotect_token(&encrypted).expect("unprotect should succeed");
        assert_eq!(token, decrypted);
    }

    #[test]
    fn test_file_path_location() {
        let path = Config::file_path().expect("should find config path");
        assert!(path.ends_with(std::path::Path::new("QuickFlare").join("config.json")));
    }

    #[test]
    fn test_save_load_roundtrip() {
        let test_dir = std::env::temp_dir().join(format!("qf_test_{}_{}", std::process::id(), std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH).unwrap().as_nanos()));
        let path = test_dir.join("subdir").join("config.json");
        let mut cfg = Config {
            api_token: "secret_123".to_string(),
            domain: Some("test.com".to_string()),
            routes_disabled: true,
            ..Default::default()
        };
        cfg.save_to_path(&path).expect("save should succeed");
        assert!(path.exists());

        let loaded = Config::load_from_path(&path).expect("load should succeed");
        assert_eq!(loaded.api_token, "secret_123");
        assert_eq!(loaded.domain, Some("test.com".to_string()));
        assert!(loaded.routes_disabled);

        let _ = std::fs::remove_dir_all(&test_dir);
    }
}
