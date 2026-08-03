// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

//! THE SHIELD - command-line entry point.
//!
//! This binary is deliberately thin: argument parsing, configuration loading,
//! and the wiring in [`run`] that assembles the subsystems. Everything it
//! assembles lives in the `shield` library crate (`src/lib.rs`), which is
//! where the WAF itself is implemented and documented.

use shield::circuit::{CircuitBreakerConfig, CircuitBreakerManager};
use shield::config::Config;
use shield::metrics::MetricsRegistry;
use shield::proxy::{Backend, HealthChecker, ReverseProxy};
use shield::ratelimit::RateLimitManager;
use shield::router::Router;
use shield::rules::RuleEngine;
use shield::server::Server;
use shield::tls::TlsManager;

use std::env;
use std::process;
use std::sync::Arc;
use std::time::Duration;

const VERSION: &str = env!("CARGO_PKG_VERSION");
const NAME: &str = "THE SHIELD";

fn print_banner() {
    eprintln!(
        r#"
╔═══════════════════════════════════════════════════════════════╗
║                                                               ║
║   ████████╗██╗  ██╗███████╗    ███████╗██╗  ██╗██╗███████╗██║ ║
║   ╚══██╔══╝██║  ██║██╔════╝    ██╔════╝██║  ██║██║██╔════╝██║ ║
║      ██║   ███████║█████╗      ███████╗███████║██║█████╗  ██║ ║
║      ██║   ██╔══██║██╔══╝      ╚════██║██╔══██║██║██╔══╝  ██║ ║
║      ██║   ██║  ██║███████╗    ███████║██║  ██║██║███████╗██║ ║
║      ╚═╝   ╚═╝  ╚═╝╚══════╝    ╚══════╝╚═╝  ╚═╝╚═╝╚══════╝╚═╝ ║
║                                                               ║
║         WAF/Gateway for the Unheaded Kingdom v{}          ║
║                                                               ║
╚═══════════════════════════════════════════════════════════════╝
"#,
        VERSION
    );
}

fn print_usage() {
    eprintln!(
        r#"
Usage: shield [OPTIONS] <CONFIG_FILE>

Options:
    -h, --help       Print this help message
    -v, --version    Print version information
    -t, --test       Test configuration and exit
    -d, --debug      Enable debug logging

Examples:
    shield config.toml
    shield --test config.toml
    shield -d /etc/shield/config.toml

Configuration File Format (TOML):

    [server]
    listen = "0.0.0.0"
    http_port = 8080
    https_port = 8443
    max_connections = 10000

    [tls]  # Optional
    cert_path = "/path/to/cert.pem"
    key_path = "/path/to/key.pem"
    min_version = "1.2"

    [[backends]]
    name = "api"
    address = "127.0.0.1:3000"
    weight = 1
    pool_size = 100

    [[rules]]
    id = "block-sqli"
    pattern = "(?i)(union|select)"
    target = "query"
    action = "block"
    log = true

    [[rules]]
    id = "rate-limit-api"
    path = "/api/*"
    rate = 100
    per = "1m"
    action = "limit"
"#
    );
}

fn print_version() {
    eprintln!("{} v{}", NAME, VERSION);
    eprintln!("WAF/Gateway for the Unheaded Kingdom");
}

#[tokio::main]
async fn main() {
    // Parse command line arguments
    let args: Vec<String> = env::args().collect();

    if args.len() < 2 {
        print_banner();
        print_usage();
        process::exit(1);
    }

    let mut config_path = None;
    let mut test_mode = false;
    let mut debug_mode = false;

    let mut i = 1;
    while i < args.len() {
        match args[i].as_str() {
            "-h" | "--help" => {
                print_usage();
                process::exit(0);
            }
            "-v" | "--version" => {
                print_version();
                process::exit(0);
            }
            "-t" | "--test" => {
                test_mode = true;
            }
            "-d" | "--debug" => {
                debug_mode = true;
            }
            arg if !arg.starts_with('-') => {
                config_path = Some(arg.to_string());
            }
            other => {
                eprintln!("Unknown option: {}", other);
                print_usage();
                process::exit(1);
            }
        }
        i += 1;
    }

    let config_path = match config_path {
        Some(p) => p,
        None => {
            eprintln!("Error: Configuration file is required");
            print_usage();
            process::exit(1);
        }
    };

    // Load configuration
    let config = match Config::load(&config_path) {
        Ok(c) => c,
        Err(e) => {
            eprintln!("Error loading configuration: {}", e);
            process::exit(1);
        }
    };

    if test_mode {
        eprintln!("Configuration OK");
        eprintln!("  Backends: {}", config.backends.len());
        eprintln!("  Rules: {}", config.rules.len());
        eprintln!(
            "  TLS: {}",
            if config.tls.is_some() {
                "enabled"
            } else {
                "disabled"
            }
        );
        process::exit(0);
    }

    print_banner();

    // Initialize components
    if let Err(e) = run(config, debug_mode).await {
        eprintln!("Fatal error: {}", e);
        process::exit(1);
    }
}

async fn run(config: Config, debug_mode: bool) -> Result<(), Box<dyn std::error::Error>> {
    // Create metrics registry
    let metrics = Arc::new(MetricsRegistry::new());

    // Create rule engine
    let rule_engine = Arc::new(
        RuleEngine::new(&config.rules, &config.ip_allowlist, &config.ip_blocklist)
            .map_err(|e| format!("Failed to create rule engine: {}", e))?,
    );

    eprintln!(
        "Loaded {} WAF rules, {} IP allowlist, {} IP blocklist",
        rule_engine.rule_count(),
        config.ip_allowlist.len(),
        config.ip_blocklist.len()
    );

    // Create rate limiter
    let rate_limiter = Arc::new(RateLimitManager::new(
        config.rate_limit.default_rate,
        config.rate_limit.default_burst,
        Duration::from_secs(config.rate_limit.cleanup_interval_secs),
    ));

    // Start rate limiter cleanup task
    let rate_limiter_clone = rate_limiter.clone();
    tokio::spawn(async move {
        let mut interval =
            tokio::time::interval(Duration::from_secs(config.rate_limit.cleanup_interval_secs));
        loop {
            interval.tick().await;
            rate_limiter_clone.cleanup_all().await;
        }
    });

    // Create circuit breaker manager
    let circuit_breakers = Arc::new(CircuitBreakerManager::new(CircuitBreakerConfig {
        enabled: config.circuit_breaker.enabled,
        failure_threshold: config.circuit_breaker.failure_threshold,
        success_threshold: config.circuit_breaker.success_threshold,
        reset_timeout: Duration::from_secs(config.circuit_breaker.reset_timeout_secs),
        failure_window: Duration::from_secs(config.circuit_breaker.half_open_timeout_secs),
        half_open_max_requests: 3,
    }));

    // Create backends
    let backends: Result<Vec<Arc<Backend>>, _> = config
        .backends
        .iter()
        .map(|cfg| Backend::from_config(cfg).map(Arc::new))
        .collect();
    let backends = backends.map_err(|e| format!("Failed to create backend: {}", e))?;

    eprintln!("Configured {} backend(s):", backends.len());
    for backend in &backends {
        eprintln!(
            "  - {} @ {} (weight: {})",
            backend.name, backend.address, backend.weight
        );
    }

    // Start health checker
    let health_checker = HealthChecker::new(
        backends.clone(),
        Duration::from_secs(10),
        Duration::from_secs(5),
    );
    let _health_handle = health_checker.start();

    // Create reverse proxy
    let proxy = Arc::new(ReverseProxy::new(
        backends,
        circuit_breakers,
        metrics.clone(),
        config.request_timeout(),
    ));

    // Create router
    let mut router = Router::new(
        rule_engine,
        rate_limiter,
        proxy,
        metrics.clone(),
        config.server.max_body_size,
    );
    router.set_rate_limit_enabled(config.rate_limit.enabled);

    if debug_mode || config.logging.log_allowed {
        router.set_log_all(true);
    }

    let router = Arc::new(router);

    // Create TLS manager if configured
    let tls_manager = if let Some(ref tls_config) = config.tls {
        match TlsManager::from_config(tls_config) {
            Ok(tls) => {
                eprintln!("TLS enabled (min version: {})", tls_config.min_version);
                Some(Arc::new(tls))
            }
            Err(e) => {
                eprintln!("Warning: TLS configuration failed: {}", e);
                eprintln!("Continuing with HTTP only");
                None
            }
        }
    } else {
        None
    };

    // Create and run server
    let server = Server::new(&config, router, metrics, tls_manager);
    server.run().await?;

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_config_parsing() {
        let config_str = r#"
[server]
listen = "127.0.0.1"
http_port = 8080

[[backends]]
name = "test"
address = "127.0.0.1:3000"

[[rules]]
id = "test-rule"
pattern = "test"
target = "query"
action = "log"
"#;

        let config: Config = config_str.parse().expect("Failed to parse config");
        assert_eq!(config.server.listen, "127.0.0.1");
        assert_eq!(config.backends.len(), 1);
        assert_eq!(config.rules.len(), 1);
    }
}
