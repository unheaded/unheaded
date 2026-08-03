// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

//! HTTP/HTTPS Server for THE SHIELD
//!
//! High-performance HTTP server built on hyper.
//! Supports both HTTP and HTTPS with TLS termination.

use crate::config::Config;
use crate::metrics::MetricsRegistry;
use crate::proxy::TokioIo;
use crate::router::Router;
use crate::tls::TlsManager;
use http_body_util::Full;
use hyper::body::{Bytes, Incoming};
use hyper::server::conn::http1;
use hyper::service::service_fn;
use hyper::{Request, Response};
use std::net::SocketAddr;
use std::sync::Arc;
use std::time::Duration;
use tokio::net::{TcpListener, TcpStream};
use tokio::sync::Semaphore;

/// Server state and configuration
pub struct Server {
    /// HTTP listener address
    http_addr: SocketAddr,
    /// HTTPS listener address (optional)
    https_addr: Option<SocketAddr>,
    /// Metrics endpoint address
    metrics_addr: SocketAddr,
    /// Whether to expose the metrics endpoint at all (`[metrics] enabled`).
    /// Previously parsed and never read, so the endpoint could not be turned
    /// off — it listened regardless of config.
    metrics_enabled: bool,
    /// TLS manager (optional)
    tls_manager: Option<Arc<TlsManager>>,
    /// Request router
    router: Arc<Router>,
    /// Metrics registry
    metrics: Arc<MetricsRegistry>,
    /// Maximum concurrent connections
    max_connections: usize,
    /// Request timeout
    request_timeout: Duration,
}

impl Server {
    /// Create a new server from components
    pub fn new(
        config: &Config,
        router: Arc<Router>,
        metrics: Arc<MetricsRegistry>,
        tls_manager: Option<Arc<TlsManager>>,
    ) -> Self {
        Self {
            http_addr: config.http_addr(),
            https_addr: if tls_manager.is_some() {
                Some(config.https_addr())
            } else {
                None
            },
            metrics_addr: config.metrics_addr(),
            metrics_enabled: config.metrics.enabled,
            tls_manager,
            router,
            metrics,
            max_connections: config.server.max_connections,
            request_timeout: config.request_timeout(),
        }
    }

    /// Run the server
    pub async fn run(self) -> Result<(), ServerError> {
        // Create connection semaphore
        let connection_semaphore = Arc::new(Semaphore::new(self.max_connections));

        // Start metrics server only when enabled — this endpoint exposes
        // operational detail, so an operator turning it off must actually
        // close the listener rather than just hide it from docs.
        let _metrics_handle = if self.metrics_enabled {
            Some(self.start_metrics_server().await?)
        } else {
            None
        };

        // Start HTTP server
        let http_handle = self.start_http_server(connection_semaphore.clone()).await?;

        // Start HTTPS server if configured
        let https_handle = if self.https_addr.is_some() {
            Some(
                self.start_https_server(connection_semaphore.clone())
                    .await?,
            )
        } else {
            None
        };

        eprintln!("THE SHIELD is protecting the Kingdom!");
        eprintln!("  HTTP:    {}", self.http_addr);
        if let Some(addr) = self.https_addr {
            eprintln!("  HTTPS:   {}", addr);
        }
        if self.metrics_enabled {
            eprintln!("  Metrics: {}", self.metrics_addr);
        } else {
            eprintln!("  Metrics: disabled");
        }

        // Wait for shutdown signal
        tokio::select! {
            _ = tokio::signal::ctrl_c() => {
                eprintln!("\nShutting down THE SHIELD...");
            }
            result = http_handle => {
                if let Err(e) = result {
                    eprintln!("HTTP server error: {:?}", e);
                }
            }
            result = async {
                if let Some(handle) = https_handle {
                    handle.await
                } else {
                    std::future::pending().await
                }
            } => {
                if let Err(e) = result {
                    eprintln!("HTTPS server error: {:?}", e);
                }
            }
        }

        Ok(())
    }

    /// Start the HTTP server
    async fn start_http_server(
        &self,
        semaphore: Arc<Semaphore>,
    ) -> Result<tokio::task::JoinHandle<()>, ServerError> {
        let listener =
            TcpListener::bind(self.http_addr)
                .await
                .map_err(|e| ServerError::BindError {
                    addr: self.http_addr,
                    source: e,
                })?;

        let router = self.router.clone();
        let metrics = self.metrics.clone();
        let timeout = self.request_timeout;

        let handle = tokio::spawn(async move {
            loop {
                match listener.accept().await {
                    Ok((stream, addr)) => {
                        let permit = match semaphore.clone().try_acquire_owned() {
                            Ok(permit) => permit,
                            Err(_) => {
                                eprintln!("Connection limit reached, rejecting {}", addr);
                                continue;
                            }
                        };

                        let router = router.clone();
                        let metrics = metrics.clone();

                        metrics.active_connections.inc();

                        tokio::spawn(async move {
                            if let Err(e) =
                                handle_http_connection(stream, addr, router, timeout).await
                            {
                                eprintln!("Connection error from {}: {}", addr, e);
                            }
                            metrics.active_connections.dec();
                            drop(permit);
                        });
                    }
                    Err(e) => {
                        eprintln!("Accept error: {}", e);
                    }
                }
            }
        });

        Ok(handle)
    }

    /// Start the HTTPS server
    async fn start_https_server(
        &self,
        semaphore: Arc<Semaphore>,
    ) -> Result<tokio::task::JoinHandle<()>, ServerError> {
        let https_addr = self.https_addr.ok_or(ServerError::NoTlsConfig)?;
        let tls_manager = self.tls_manager.clone().ok_or(ServerError::NoTlsConfig)?;

        let listener = TcpListener::bind(https_addr)
            .await
            .map_err(|e| ServerError::BindError {
                addr: https_addr,
                source: e,
            })?;

        let router = self.router.clone();
        let metrics = self.metrics.clone();
        let timeout = self.request_timeout;

        let handle = tokio::spawn(async move {
            loop {
                match listener.accept().await {
                    Ok((stream, addr)) => {
                        let permit = match semaphore.clone().try_acquire_owned() {
                            Ok(permit) => permit,
                            Err(_) => {
                                eprintln!("Connection limit reached, rejecting {}", addr);
                                continue;
                            }
                        };

                        let router = router.clone();
                        let metrics = metrics.clone();
                        let tls = tls_manager.clone();

                        metrics.active_connections.inc();

                        tokio::spawn(async move {
                            // Perform TLS handshake
                            match tls.accept(stream).await {
                                Ok(tls_stream) => {
                                    if let Err(e) =
                                        handle_https_connection(tls_stream, addr, router, timeout)
                                            .await
                                    {
                                        eprintln!("TLS connection error from {}: {}", addr, e);
                                    }
                                }
                                Err(e) => {
                                    eprintln!("TLS handshake error from {}: {}", addr, e);
                                }
                            }
                            metrics.active_connections.dec();
                            drop(permit);
                        });
                    }
                    Err(e) => {
                        eprintln!("Accept error: {}", e);
                    }
                }
            }
        });

        Ok(handle)
    }

    /// Start the metrics server
    async fn start_metrics_server(&self) -> Result<tokio::task::JoinHandle<()>, ServerError> {
        let listener =
            TcpListener::bind(self.metrics_addr)
                .await
                .map_err(|e| ServerError::BindError {
                    addr: self.metrics_addr,
                    source: e,
                })?;

        let metrics = self.metrics.clone();

        let handle = tokio::spawn(async move {
            loop {
                match listener.accept().await {
                    Ok((stream, _addr)) => {
                        let metrics = metrics.clone();

                        tokio::spawn(async move {
                            if let Err(e) = handle_metrics_connection(stream, metrics).await {
                                eprintln!("Metrics connection error: {}", e);
                            }
                        });
                    }
                    Err(e) => {
                        eprintln!("Metrics accept error: {}", e);
                    }
                }
            }
        });

        Ok(handle)
    }
}

/// Handle an HTTP connection
async fn handle_http_connection(
    stream: TcpStream,
    addr: SocketAddr,
    router: Arc<Router>,
    _timeout: Duration,
) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    let io = TokioIo::new(stream);
    let client_ip = Some(addr.ip());

    let service = service_fn(move |req: Request<Incoming>| {
        let router = router.clone();
        async move {
            // Check for internal endpoints first
            let path = req.uri().path();
            if let Some(response) = router.handle_internal(path).await {
                return Ok::<_, hyper::Error>(response);
            }

            // Route through WAF
            Ok(router.route(req, client_ip).await)
        }
    });

    http1::Builder::new()
        .keep_alive(true)
        .serve_connection(io, service)
        .await?;

    Ok(())
}

/// Handle an HTTPS connection
async fn handle_https_connection<S>(
    stream: tokio_rustls::server::TlsStream<S>,
    addr: SocketAddr,
    router: Arc<Router>,
    _timeout: Duration,
) -> Result<(), Box<dyn std::error::Error + Send + Sync>>
where
    S: tokio::io::AsyncRead + tokio::io::AsyncWrite + Unpin + Send + 'static,
{
    let io = TokioIo::new(stream);
    let client_ip = Some(addr.ip());

    let service = service_fn(move |req: Request<Incoming>| {
        let router = router.clone();
        async move {
            // Check for internal endpoints first
            let path = req.uri().path();
            if let Some(response) = router.handle_internal(path).await {
                return Ok::<_, hyper::Error>(response);
            }

            // Route through WAF
            Ok(router.route(req, client_ip).await)
        }
    });

    http1::Builder::new()
        .keep_alive(true)
        .serve_connection(io, service)
        .await?;

    Ok(())
}

/// Handle a metrics connection
async fn handle_metrics_connection(
    stream: TcpStream,
    metrics: Arc<MetricsRegistry>,
) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    let io = TokioIo::new(stream);

    let service = service_fn(move |req: Request<Incoming>| {
        let metrics = metrics.clone();
        async move {
            let path = req.uri().path();

            let response = match path {
                "/metrics" => {
                    let body = metrics.export_prometheus().await;
                    Response::builder()
                        .status(200)
                        .header("Content-Type", "text/plain; charset=utf-8")
                        .body(Full::new(Bytes::from(body)))
                        .unwrap()
                }
                "/health" | "/healthz" => Response::builder()
                    .status(200)
                    .body(Full::new(Bytes::from(r#"{"status":"healthy"}"#)))
                    .unwrap(),
                _ => Response::builder()
                    .status(404)
                    .body(Full::new(Bytes::from("Not Found")))
                    .unwrap(),
            };

            Ok::<_, hyper::Error>(response)
        }
    });

    http1::Builder::new().serve_connection(io, service).await?;

    Ok(())
}

/// Server errors
#[derive(Debug)]
pub enum ServerError {
    /// Failed to bind to address
    BindError {
        addr: SocketAddr,
        source: std::io::Error,
    },
    /// TLS not configured but HTTPS requested
    NoTlsConfig,
    /// Connection error
    ConnectionError(String),
}

impl std::fmt::Display for ServerError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            ServerError::BindError { addr, source } => {
                write!(f, "Failed to bind to {}: {}", addr, source)
            }
            ServerError::NoTlsConfig => write!(f, "TLS not configured"),
            ServerError::ConnectionError(msg) => write!(f, "Connection error: {}", msg),
        }
    }
}

impl std::error::Error for ServerError {}

/// Server builder for easier configuration
pub struct ServerBuilder {
    router: Option<Arc<Router>>,
    metrics: Option<Arc<MetricsRegistry>>,
    tls_manager: Option<Arc<TlsManager>>,
    http_addr: Option<SocketAddr>,
    https_addr: Option<SocketAddr>,
    metrics_addr: Option<SocketAddr>,
    /// Expose the metrics endpoint. Defaults to true, matching
    /// `[metrics] enabled`'s serde default.
    metrics_enabled: bool,
    max_connections: usize,
    request_timeout: Duration,
}

impl ServerBuilder {
    /// Create a new server builder
    pub fn new() -> Self {
        Self {
            router: None,
            metrics: None,
            tls_manager: None,
            http_addr: None,
            https_addr: None,
            metrics_addr: None,
            metrics_enabled: true,
            max_connections: 10000,
            request_timeout: Duration::from_secs(30),
        }
    }

    /// Set the router
    pub fn router(mut self, router: Arc<Router>) -> Self {
        self.router = Some(router);
        self
    }

    /// Set the metrics registry
    pub fn metrics(mut self, metrics: Arc<MetricsRegistry>) -> Self {
        self.metrics = Some(metrics);
        self
    }

    /// Set the TLS manager
    pub fn tls(mut self, tls: Arc<TlsManager>) -> Self {
        self.tls_manager = Some(tls);
        self
    }

    /// Set the HTTP listen address
    pub fn http_addr(mut self, addr: SocketAddr) -> Self {
        self.http_addr = Some(addr);
        self
    }

    /// Set the HTTPS listen address
    pub fn https_addr(mut self, addr: SocketAddr) -> Self {
        self.https_addr = Some(addr);
        self
    }

    /// Set the metrics listen address
    pub fn metrics_addr(mut self, addr: SocketAddr) -> Self {
        self.metrics_addr = Some(addr);
        self
    }

    /// Set maximum connections
    pub fn max_connections(mut self, max: usize) -> Self {
        self.max_connections = max;
        self
    }

    /// Set request timeout
    pub fn request_timeout(mut self, timeout: Duration) -> Self {
        self.request_timeout = timeout;
        self
    }

    /// Build the server
    pub fn build(self) -> Result<Server, &'static str> {
        let router = self.router.ok_or("Router is required")?;
        let metrics = self.metrics.ok_or("Metrics registry is required")?;
        let http_addr = self.http_addr.ok_or("HTTP address is required")?;
        let metrics_addr = self.metrics_addr.ok_or("Metrics address is required")?;

        Ok(Server {
            http_addr,
            https_addr: self.https_addr,
            metrics_addr,
            metrics_enabled: self.metrics_enabled,
            tls_manager: self.tls_manager,
            router,
            metrics,
            max_connections: self.max_connections,
            request_timeout: self.request_timeout,
        })
    }
}

impl Default for ServerBuilder {
    fn default() -> Self {
        Self::new()
    }
}
