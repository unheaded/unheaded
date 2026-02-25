//! Configuration module for THE SHIELD
//!
//! Handles TOML configuration parsing for the WAF/Gateway.
//! All configuration is validated at load time to fail fast.

use serde::Deserialize;
use std::collections::HashSet;
use std::fs;
use std::net::SocketAddr;
use std::path::Path;
use std::time::Duration;

/// Main configuration structure for THE SHIELD
#[derive(Debug, Clone, Deserialize)]
pub struct Config {
    /// Server configuration
    pub server: ServerConfig,

    /// TLS configuration (optional)
    #[serde(default)]
    pub tls: Option<TlsConfig>,

    /// Backend configuration
    pub backends: Vec<BackendConfig>,

    /// Rate limiting configuration
    #[serde(default)]
    pub rate_limit: RateLimitConfig,

    /// Circuit breaker configuration
    #[serde(default)]
    pub circuit_breaker: CircuitBreakerConfig,

    /// WAF rules
    #[serde(default)]
    pub rules: Vec<RuleConfig>,

    /// IP allowlist
    #[serde(default)]
    pub ip_allowlist: Vec<String>,

    /// IP blocklist
    #[serde(default)]
    pub ip_blocklist: Vec<String>,

    /// Logging configuration
    #[serde(default)]
    pub logging: LoggingConfig,

    /// Metrics configuration
    #[serde(default)]
    pub metrics: MetricsConfig,
}

/// Server configuration
#[derive(Debug, Clone, Deserialize)]
pub struct ServerConfig {
    /// Listen address
    #[serde(default = "default_listen_addr")]
    pub listen: String,

    /// HTTP port
    #[serde(default = "default_http_port")]
    pub http_port: u16,

    /// HTTPS port
    #[serde(default = "default_https_port")]
    pub https_port: u16,

    /// Maximum concurrent connections
    #[serde(default = "default_max_connections")]
    pub max_connections: usize,

    /// Request timeout in seconds
    #[serde(default = "default_request_timeout")]
    pub request_timeout_secs: u64,

    /// Maximum request body size in bytes
    #[serde(default = "default_max_body_size")]
    pub max_body_size: usize,
}

/// TLS configuration
#[derive(Debug, Clone, Deserialize)]
pub struct TlsConfig {
    /// Path to certificate file (PEM format)
    pub cert_path: String,

    /// Path to private key file (PEM format)
    pub key_path: String,

    /// Minimum TLS version (1.2 or 1.3)
    #[serde(default = "default_min_tls_version")]
    pub min_version: String,
}

/// Backend server configuration
#[derive(Debug, Clone, Deserialize)]
pub struct BackendConfig {
    /// Backend name/identifier
    pub name: String,

    /// Backend address (host:port)
    pub address: String,

    /// Weight for load balancing (higher = more traffic)
    #[serde(default = "default_weight")]
    pub weight: u32,

    /// Health check path
    #[serde(default)]
    pub health_check_path: Option<String>,

    /// Connection pool size
    #[serde(default = "default_pool_size")]
    pub pool_size: usize,
}

/// Rate limiting configuration
#[derive(Debug, Clone, Deserialize)]
pub struct RateLimitConfig {
    /// Enable rate limiting
    #[serde(default = "default_true")]
    pub enabled: bool,

    /// Default requests per second per IP
    #[serde(default = "default_rate")]
    pub default_rate: u32,

    /// Burst size (token bucket capacity)
    #[serde(default = "default_burst")]
    pub default_burst: u32,

    /// Cleanup interval for expired entries (seconds)
    #[serde(default = "default_cleanup_interval")]
    pub cleanup_interval_secs: u64,
}

/// Circuit breaker configuration
#[derive(Debug, Clone, Deserialize)]
pub struct CircuitBreakerConfig {
    /// Enable circuit breaker
    #[serde(default = "default_true")]
    pub enabled: bool,

    /// Failure threshold before opening circuit
    #[serde(default = "default_failure_threshold")]
    pub failure_threshold: u32,

    /// Success threshold before closing circuit
    #[serde(default = "default_success_threshold")]
    pub success_threshold: u32,

    /// Time in half-open state (seconds)
    #[serde(default = "default_half_open_timeout")]
    pub half_open_timeout_secs: u64,

    /// Reset timeout after circuit opens (seconds)
    #[serde(default = "default_reset_timeout")]
    pub reset_timeout_secs: u64,
}

/// WAF rule configuration
#[derive(Debug, Clone, Deserialize)]
pub struct RuleConfig {
    /// Unique rule identifier
    pub id: String,

    /// Rule description
    #[serde(default)]
    pub description: Option<String>,

    /// Enable/disable rule
    #[serde(default = "default_true")]
    pub enabled: bool,

    /// Regex pattern to match
    #[serde(default)]
    pub pattern: Option<String>,

    /// Target for pattern matching: query, path, header, body, all
    #[serde(default = "default_target")]
    pub target: String,

    /// Specific header name (when target = "header")
    #[serde(default)]
    pub header_name: Option<String>,

    /// Path pattern (glob-style)
    #[serde(default)]
    pub path: Option<String>,

    /// HTTP methods this rule applies to
    #[serde(default)]
    pub methods: Option<Vec<String>>,

    /// Action: block, allow, log, limit
    pub action: String,

    /// Log matches
    #[serde(default)]
    pub log: bool,

    /// Rate limit (requests)
    #[serde(default)]
    pub rate: Option<u32>,

    /// Rate limit period (e.g., "1s", "1m", "1h")
    #[serde(default)]
    pub per: Option<String>,

    /// Custom response code when blocking
    #[serde(default = "default_block_status")]
    pub block_status: u16,

    /// Custom response message when blocking
    #[serde(default)]
    pub block_message: Option<String>,

    /// Priority (lower = higher priority)
    #[serde(default = "default_priority")]
    pub priority: i32,
}

/// Logging configuration
#[derive(Debug, Clone, Deserialize, Default)]
pub struct LoggingConfig {
    /// Log level: trace, debug, info, warn, error
    #[serde(default = "default_log_level")]
    pub level: String,

    /// Log format: json, text
    #[serde(default = "default_log_format")]
    pub format: String,

    /// Log file path (optional, defaults to stdout)
    #[serde(default)]
    pub file: Option<String>,

    /// Log blocked requests
    #[serde(default = "default_true")]
    pub log_blocked: bool,

    /// Log allowed requests
    #[serde(default)]
    pub log_allowed: bool,
}

/// Metrics configuration
#[derive(Debug, Clone, Deserialize)]
pub struct MetricsConfig {
    /// Enable metrics
    #[serde(default = "default_true")]
    pub enabled: bool,

    /// Metrics endpoint path
    #[serde(default = "default_metrics_path")]
    pub path: String,

    /// Metrics port (separate from main server)
    #[serde(default = "default_metrics_port")]
    pub port: u16,
}

// Default value functions
fn default_listen_addr() -> String {
    "0.0.0.0".to_string()
}

fn default_http_port() -> u16 {
    8080
}

fn default_https_port() -> u16 {
    8443
}

fn default_max_connections() -> usize {
    10000
}

fn default_request_timeout() -> u64 {
    30
}

fn default_max_body_size() -> usize {
    10 * 1024 * 1024 // 10 MB
}

fn default_min_tls_version() -> String {
    "1.2".to_string()
}

fn default_weight() -> u32 {
    1
}

fn default_pool_size() -> usize {
    100
}

fn default_true() -> bool {
    true
}

fn default_rate() -> u32 {
    100
}

fn default_burst() -> u32 {
    200
}

fn default_cleanup_interval() -> u64 {
    60
}

fn default_failure_threshold() -> u32 {
    5
}

fn default_success_threshold() -> u32 {
    3
}

fn default_half_open_timeout() -> u64 {
    30
}

fn default_reset_timeout() -> u64 {
    60
}

fn default_target() -> String {
    "all".to_string()
}

fn default_block_status() -> u16 {
    403
}

fn default_priority() -> i32 {
    100
}

fn default_log_level() -> String {
    "info".to_string()
}

fn default_log_format() -> String {
    "text".to_string()
}

fn default_metrics_path() -> String {
    "/metrics".to_string()
}

fn default_metrics_port() -> u16 {
    9090
}

impl Default for ServerConfig {
    fn default() -> Self {
        Self {
            listen: default_listen_addr(),
            http_port: default_http_port(),
            https_port: default_https_port(),
            max_connections: default_max_connections(),
            request_timeout_secs: default_request_timeout(),
            max_body_size: default_max_body_size(),
        }
    }
}

impl Default for RateLimitConfig {
    fn default() -> Self {
        Self {
            enabled: true,
            default_rate: default_rate(),
            default_burst: default_burst(),
            cleanup_interval_secs: default_cleanup_interval(),
        }
    }
}

impl Default for CircuitBreakerConfig {
    fn default() -> Self {
        Self {
            enabled: true,
            failure_threshold: default_failure_threshold(),
            success_threshold: default_success_threshold(),
            half_open_timeout_secs: default_half_open_timeout(),
            reset_timeout_secs: default_reset_timeout(),
        }
    }
}

impl Default for MetricsConfig {
    fn default() -> Self {
        Self {
            enabled: true,
            path: default_metrics_path(),
            port: default_metrics_port(),
        }
    }
}

impl Config {
    /// Load configuration from a TOML file
    pub fn load<P: AsRef<Path>>(path: P) -> Result<Self, ConfigError> {
        let content = fs::read_to_string(path.as_ref()).map_err(|e| ConfigError::IoError {
            path: path.as_ref().display().to_string(),
            source: e,
        })?;

        let config: Config = toml::from_str(&content).map_err(|e| ConfigError::ParseError {
            message: e.to_string(),
        })?;

        config.validate()?;
        Ok(config)
    }

    /// Load configuration from a string
    pub fn from_str(content: &str) -> Result<Self, ConfigError> {
        let config: Config =
            toml::from_str(content).map_err(|e| ConfigError::ParseError {
                message: e.to_string(),
            })?;

        config.validate()?;
        Ok(config)
    }

    /// Validate the configuration
    fn validate(&self) -> Result<(), ConfigError> {
        // Validate server config
        if self.server.http_port == 0 && self.tls.is_none() {
            return Err(ConfigError::ValidationError {
                message: "Either HTTP port or TLS must be configured".to_string(),
            });
        }

        // Validate backends
        if self.backends.is_empty() {
            return Err(ConfigError::ValidationError {
                message: "At least one backend must be configured".to_string(),
            });
        }

        for backend in &self.backends {
            if backend.address.is_empty() {
                return Err(ConfigError::ValidationError {
                    message: format!("Backend '{}' has empty address", backend.name),
                });
            }
        }

        // Validate TLS config if present
        if let Some(ref tls) = self.tls {
            if !Path::new(&tls.cert_path).exists() {
                return Err(ConfigError::ValidationError {
                    message: format!("TLS certificate not found: {}", tls.cert_path),
                });
            }
            if !Path::new(&tls.key_path).exists() {
                return Err(ConfigError::ValidationError {
                    message: format!("TLS key not found: {}", tls.key_path),
                });
            }
        }

        // Validate rules
        let mut rule_ids = HashSet::new();
        for rule in &self.rules {
            if !rule_ids.insert(&rule.id) {
                return Err(ConfigError::ValidationError {
                    message: format!("Duplicate rule ID: {}", rule.id),
                });
            }

            // Validate action
            match rule.action.as_str() {
                "block" | "allow" | "log" | "limit" => {}
                _ => {
                    return Err(ConfigError::ValidationError {
                        message: format!(
                            "Invalid action '{}' for rule '{}'. Must be: block, allow, log, limit",
                            rule.action, rule.id
                        ),
                    });
                }
            }

            // Validate target
            match rule.target.as_str() {
                "query" | "path" | "header" | "body" | "all" | "method" | "uri" => {}
                _ => {
                    return Err(ConfigError::ValidationError {
                        message: format!(
                            "Invalid target '{}' for rule '{}'. Must be: query, path, header, body, method, uri, all",
                            rule.target, rule.id
                        ),
                    });
                }
            }

            // Rate limit rules must have rate and per
            if rule.action == "limit" && (rule.rate.is_none() || rule.per.is_none()) {
                return Err(ConfigError::ValidationError {
                    message: format!(
                        "Rate limit rule '{}' must specify 'rate' and 'per'",
                        rule.id
                    ),
                });
            }
        }

        Ok(())
    }

    /// Get the HTTP listen address
    pub fn http_addr(&self) -> SocketAddr {
        format!("{}:{}", self.server.listen, self.server.http_port)
            .parse()
            .expect("Invalid HTTP listen address")
    }

    /// Get the HTTPS listen address
    pub fn https_addr(&self) -> SocketAddr {
        format!("{}:{}", self.server.listen, self.server.https_port)
            .parse()
            .expect("Invalid HTTPS listen address")
    }

    /// Get the metrics listen address
    pub fn metrics_addr(&self) -> SocketAddr {
        format!("{}:{}", self.server.listen, self.metrics.port)
            .parse()
            .expect("Invalid metrics listen address")
    }

    /// Get request timeout as Duration
    pub fn request_timeout(&self) -> Duration {
        Duration::from_secs(self.server.request_timeout_secs)
    }
}

/// Configuration errors
#[derive(Debug)]
pub enum ConfigError {
    IoError {
        path: String,
        source: std::io::Error,
    },
    ParseError {
        message: String,
    },
    ValidationError {
        message: String,
    },
}

impl std::fmt::Display for ConfigError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            ConfigError::IoError { path, source } => {
                write!(f, "Failed to read config file '{}': {}", path, source)
            }
            ConfigError::ParseError { message } => {
                write!(f, "Failed to parse config: {}", message)
            }
            ConfigError::ValidationError { message } => {
                write!(f, "Config validation failed: {}", message)
            }
        }
    }
}

impl std::error::Error for ConfigError {}

/// Parse duration string like "1s", "1m", "1h", "1d"
pub fn parse_duration(s: &str) -> Result<Duration, String> {
    let s = s.trim();
    if s.is_empty() {
        return Err("Empty duration string".to_string());
    }

    let (num_str, unit) = s.split_at(s.len() - 1);
    let num: u64 = num_str
        .parse()
        .map_err(|_| format!("Invalid duration number: {}", num_str))?;

    match unit {
        "s" => Ok(Duration::from_secs(num)),
        "m" => Ok(Duration::from_secs(num * 60)),
        "h" => Ok(Duration::from_secs(num * 3600)),
        "d" => Ok(Duration::from_secs(num * 86400)),
        _ => Err(format!("Invalid duration unit: {}. Use s, m, h, or d", unit)),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_duration() {
        assert_eq!(parse_duration("1s").unwrap(), Duration::from_secs(1));
        assert_eq!(parse_duration("5m").unwrap(), Duration::from_secs(300));
        assert_eq!(parse_duration("1h").unwrap(), Duration::from_secs(3600));
        assert_eq!(parse_duration("1d").unwrap(), Duration::from_secs(86400));
        assert!(parse_duration("invalid").is_err());
        assert!(parse_duration("").is_err());
    }

    #[test]
    fn test_config_from_str() {
        let config_str = r#"
[server]
listen = "127.0.0.1"
http_port = 8080

[[backends]]
name = "api"
address = "127.0.0.1:3000"

[[rules]]
id = "block-sqli"
pattern = "(?i)(union|select)"
target = "query"
action = "block"
log = true
"#;

        let config = Config::from_str(config_str).unwrap();
        assert_eq!(config.server.listen, "127.0.0.1");
        assert_eq!(config.backends.len(), 1);
        assert_eq!(config.rules.len(), 1);
    }
}
