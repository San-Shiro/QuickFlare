use anyhow::{Context, Result};
use reqwest::header::{HeaderMap, HeaderValue, AUTHORIZATION};
use reqwest::Client;
use serde_json::json;

use crate::models::{
    Account, ApiResponse, DnsRecord, IngressRule, Tunnel, TunnelConfig, TunnelConfigWrapper, Zone,
};

const BASE_URL: &str = "https://api.cloudflare.com/client/v4";

#[derive(Clone)]
pub struct CloudflareClient {
    client: Client,
    token: String,
}

impl CloudflareClient {
    pub fn new(token: String) -> Result<Self> {
        let mut headers = HeaderMap::new();
        let mut auth_val = HeaderValue::from_str(&format!("Bearer {}", token.trim()))
            .context("Invalid characters in Cloudflare API token")?;
        auth_val.set_sensitive(true);
        headers.insert(AUTHORIZATION, auth_val);

        let client = Client::builder()
            .default_headers(headers)
            .build()
            .context("Failed to construct HTTP client")?;

        Ok(Self { client, token })
    }

    pub fn token(&self) -> &str {
        &self.token
    }

    /// Lists accounts accessible by the current API token.
    pub async fn list_accounts(&self) -> Result<Vec<Account>> {
        let url = format!("{}/accounts", BASE_URL);
        let resp: ApiResponse<Vec<Account>> = self
            .client
            .get(&url)
            .send()
            .await
            .context("Failed to request accounts")?
            .json()
            .await
            .context("Failed to parse accounts response")?;

        if !resp.success {
            let msg = resp.errors.first().map(|e| e.message.as_str()).unwrap_or("Unknown API error");
            anyhow::bail!("Cloudflare API error: {}", msg);
        }

        Ok(resp.result.unwrap_or_default())
    }

    /// Lists active DNS zones (domains) for the account.
    pub async fn list_zones(&self) -> Result<Vec<Zone>> {
        let url = format!("{}/zones?status=active&per_page=50", BASE_URL);
        let resp: ApiResponse<Vec<Zone>> = self
            .client
            .get(&url)
            .send()
            .await
            .context("Failed to request zones")?
            .json()
            .await
            .context("Failed to parse zones response")?;

        if !resp.success {
            let msg = resp.errors.first().map(|e| e.message.as_str()).unwrap_or("Unknown API error");
            anyhow::bail!("Cloudflare API error: {}", msg);
        }

        Ok(resp.result.unwrap_or_default())
    }

    /// Finds an existing named tunnel or creates a new one.
    pub async fn get_or_create_tunnel(&self, account_id: &str, name: &str) -> Result<Tunnel> {
        let list_url = format!(
            "{}/accounts/{}/cfd_tunnel?is_deleted=false&name={}",
            BASE_URL, account_id, name
        );

        let resp: ApiResponse<Vec<Tunnel>> = self
            .client
            .get(&list_url)
            .send()
            .await
            .context("Failed to search tunnels")?
            .json()
            .await
            .context("Failed to parse tunnel list response")?;

        if let Some(tunnels) = resp.result {
            if let Some(tunnel) = tunnels.into_iter().find(|t| t.name == name) {
                return Ok(tunnel);
            }
        }

        // Generate a 32-byte secret for tunnel creation
        let secret: String = (0..32)
            .map(|_| format!("{:02x}", rand_u8()))
            .collect();

        let create_url = format!("{}/accounts/{}/cfd_tunnel", BASE_URL, account_id);
        let create_body = json!({
            "name": name,
            "tunnel_secret": secret
        });

        let create_resp: ApiResponse<Tunnel> = self
            .client
            .post(&create_url)
            .json(&create_body)
            .send()
            .await
            .context("Failed to create tunnel")?
            .json()
            .await
            .context("Failed to parse tunnel creation response")?;

        if !create_resp.success {
            let msg = create_resp.errors.first().map(|e| e.message.as_str()).unwrap_or("Unknown API error");
            anyhow::bail!("Failed to create Cloudflare Tunnel: {}", msg);
        }

        create_resp.result.context("Empty result in tunnel creation response")
    }

    /// Retrieves the run token for a specific tunnel.
    pub async fn get_tunnel_token(&self, account_id: &str, tunnel_id: &str) -> Result<String> {
        let url = format!("{}/accounts/{}/cfd_tunnel/{}/token", BASE_URL, account_id, tunnel_id);
        let resp: ApiResponse<String> = self
            .client
            .get(&url)
            .send()
            .await
            .context("Failed to request tunnel token")?
            .json()
            .await
            .context("Failed to parse tunnel token response")?;

        if !resp.success {
            let msg = resp.errors.first().map(|e| e.message.as_str()).unwrap_or("Unknown API error");
            anyhow::bail!("Failed to get tunnel token: {}", msg);
        }

        resp.result.context("Empty tunnel token result")
    }

    /// Updates the remote ingress rules for a tunnel.
    pub async fn update_ingress_rules(
        &self,
        account_id: &str,
        tunnel_id: &str,
        mut rules: Vec<IngressRule>,
    ) -> Result<()> {
        // Cloudflare requires a catch-all 404 rule at the end of the ingress list
        rules.push(IngressRule {
            hostname: None,
            service: "http_status:404".to_string(),
        });

        let url = format!(
            "{}/accounts/{}/cfd_tunnel/{}/configurations",
            BASE_URL, account_id, tunnel_id
        );
        let body = TunnelConfigWrapper {
            config: TunnelConfig { ingress: rules },
        };

        let resp: ApiResponse<serde_json::Value> = self
            .client
            .put(&url)
            .json(&body)
            .send()
            .await
            .context("Failed to send ingress rules update")?
            .json()
            .await
            .context("Failed to parse ingress update response")?;

        if !resp.success {
            let msg = resp.errors.first().map(|e| e.message.as_str()).unwrap_or("Unknown API error");
            anyhow::bail!("Failed to update tunnel ingress rules: {}", msg);
        }

        Ok(())
    }

    /// Creates or updates a DNS CNAME record pointing to <tunnel_id>.cfargotunnel.com.
    pub async fn upsert_dns_cname(
        &self,
        zone_id: &str,
        hostname: &str,
        tunnel_id: &str,
    ) -> Result<DnsRecord> {
        let target = format!("{}.cfargotunnel.com", tunnel_id);
        let list_url = format!(
            "{}/zones/{}/dns_records?type=CNAME&name={}",
            BASE_URL, zone_id, hostname
        );

        let list_resp: ApiResponse<Vec<DnsRecord>> = self
            .client
            .get(&list_url)
            .send()
            .await
            .context("Failed to query existing DNS records")?
            .json()
            .await
            .context("Failed to parse DNS query response")?;

        if let Some(records) = list_resp.result {
            if let Some(existing) = records.into_iter().find(|r| r.name == hostname) {
                if existing.content == target {
                    return Ok(existing);
                }
                // Update target if needed
                let update_url = format!("{}/zones/{}/dns_records/{}", BASE_URL, zone_id, existing.id);
                let update_body = json!({
                    "type": "CNAME",
                    "name": hostname,
                    "content": target,
                    "proxied": true
                });
                let update_resp: ApiResponse<DnsRecord> = self
                    .client
                    .put(&update_url)
                    .json(&update_body)
                    .send()
                    .await
                    .context("Failed to update DNS CNAME record")?
                    .json()
                    .await
                    .context("Failed to parse DNS update response")?;

                return update_resp.result.context("Empty DNS update result");
            }
        }

        // Create new record
        let create_url = format!("{}/zones/{}/dns_records", BASE_URL, zone_id);
        let create_body = json!({
            "type": "CNAME",
            "name": hostname,
            "content": target,
            "proxied": true
        });

        let create_resp: ApiResponse<DnsRecord> = self
            .client
            .post(&create_url)
            .json(&create_body)
            .send()
            .await
            .context("Failed to create DNS CNAME record")?
            .json()
            .await
            .context("Failed to parse DNS creation response")?;

        if !create_resp.success {
            let msg = create_resp.errors.first().map(|e| e.message.as_str()).unwrap_or("Unknown API error");
            anyhow::bail!("Failed to create DNS CNAME record: {}", msg);
        }

        create_resp.result.context("Empty DNS creation result")
    }

    /// Deletes a DNS CNAME record for a hostname if present.
    pub async fn delete_dns_cname(&self, zone_id: &str, hostname: &str) -> Result<()> {
        let list_url = format!(
            "{}/zones/{}/dns_records?type=CNAME&name={}",
            BASE_URL, zone_id, hostname
        );

        let list_resp: ApiResponse<Vec<DnsRecord>> = self
            .client
            .get(&list_url)
            .send()
            .await
            .context("Failed to query DNS record for deletion")?
            .json()
            .await
            .context("Failed to parse DNS query response")?;

        if let Some(records) = list_resp.result {
            for record in records {
                if record.name == hostname {
                    let del_url = format!("{}/zones/{}/dns_records/{}", BASE_URL, zone_id, record.id);
                    let _ = self.client.delete(&del_url).send().await;
                }
            }
        }

        Ok(())
    }
}

fn rand_u8() -> u8 {
    use std::time::SystemTime;
    let nanos = SystemTime::now()
        .duration_since(SystemTime::UNIX_EPOCH)
        .unwrap_or_default()
        .subsec_nanos();
    (nanos % 256) as u8
}
