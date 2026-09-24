use serde::{Deserialize, Serialize};

/// A stored route remembered between runs.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct StoredRoute {
    pub hostname: String,
    pub target: String,
    pub zone_id: String,
}

/// Cloudflare account response.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize, Default)]
pub struct Account {
    pub id: String,
    pub name: String,
}

/// Cloudflare DNS zone (domain).
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize, Default)]
pub struct Zone {
    pub id: String,
    pub name: String,
}

/// Cloudflare Tunnel object.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize, Default)]
pub struct Tunnel {
    pub id: String,
    pub name: String,
}

/// Cloudflare DNS record.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize, Default)]
pub struct DnsRecord {
    pub id: String,
    #[serde(rename = "type")]
    pub record_type: String,
    pub name: String,
    pub content: String,
}

/// Ingress rule in Cloudflare tunnel remote configuration.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize, Default)]
pub struct IngressRule {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub hostname: Option<String>,
    pub service: String,
}

/// Configuration structure for Cloudflare Tunnel configuration API.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct TunnelConfig {
    pub ingress: Vec<IngressRule>,
}

/// Tunnel configuration wrapper for PUT /accounts/{id}/cfd_tunnel/{tunnel_id}/configurations
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct TunnelConfigWrapper {
    pub config: TunnelConfig,
}

/// Generic Cloudflare API response envelope.
#[derive(Debug, Clone, Deserialize)]
pub struct ApiResponse<T> {
    pub success: bool,
    #[serde(default)]
    pub errors: Vec<ApiError>,
    #[serde(default)]
    pub result: Option<T>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct ApiError {
    pub code: i64,
    pub message: String,
}
