use serde::{Deserialize, Serialize};

#[derive(Debug, Serialize, Deserialize)]
pub struct Route {
    pub table: u32,
    pub iface: String,
    pub address: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct Rule {
    pub table: u32,
    pub iif: String,
    pub priority: u32,
}

#[derive(Debug, Default, Serialize, Deserialize)]
pub struct HostInfo {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub mac: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub via: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub ip: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub hostname: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub registered: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub access: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub priority: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub active: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub rx_bytes: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tx_bytes: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub link: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub uptime: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub first_seen: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_seen: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub auto_negotiation: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub speed: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub duplex: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub port: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub system_mode: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub http_port: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub http_host: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub region: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub firmware: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub interface: Option<HostInterface>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub dhcp: Option<HostDHCP>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub mws: Option<HostMWS>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct HostInterface {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct HostDHCP {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub expires: Option<i64>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct HostMWS {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub cid: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub ap: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub psm: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub mld: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub authenticated: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tx_rate: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub uptime: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub rssi: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub mcs: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub security: Option<String>,
}

#[derive(Debug, Serialize)]
pub struct Error {
    pub error: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub exit_code: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub output: Option<String>,
}
