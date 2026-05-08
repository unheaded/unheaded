//! Reverse Proxy for THE SHIELD
//!
//! High-performance reverse proxy with connection pooling.
//! Designed for minimal latency and maximum throughput.

use crate::circuit::{CircuitBreaker, CircuitBreakerManager, CircuitState};
use crate::config::BackendConfig;
use crate::metrics::MetricsRegistry;
use http_body_util::{BodyExt, Full};
use hyper::body::{Bytes, Incoming};
use hyper::client::conn::http1;
use hyper::{Request, Response, StatusCode};
use std::collections::HashMap;
use std::net::SocketAddr;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::Arc;
use std::time::Duration;
use tokio::io::{AsyncRead, AsyncWrite};
use tokio::net::TcpStream;
use tokio::sync::{Mutex, Semaphore};
use tokio::time::timeout;

/// Error types for proxy operations
#[derive(Debug)]
pub enum ProxyError {
    /// Backend connection failed
    ConnectionFailed(String),
    /// Backend timed out
    Timeout,
    /// Circuit breaker is open
    CircuitOpen,
    /// No healthy backends available
    NoBackends,
    /// Request forwarding failed
    ForwardFailed(String),
    /// Invalid backend configuration
    InvalidConfig(String),
}

impl std::fmt::Display for ProxyError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            ProxyError::ConnectionFailed(msg) => write!(f, "Connection failed: {}", msg),
            ProxyError::Timeout => write!(f, "Backend timeout"),
            ProxyError::CircuitOpen => write!(f, "Circuit breaker is open"),
            ProxyError::NoBackends => write!(f, "No healthy backends available"),
            ProxyError::ForwardFailed(msg) => write!(f, "Forward failed: {}", msg),
            ProxyError::InvalidConfig(msg) => write!(f, "Invalid configuration: {}", msg),
        }
    }
}

impl std::error::Error for ProxyError {}

/// A pooled connection to a backend
struct PooledConnection {
    /// The TCP stream
    stream: TcpStream,
    /// When this connection was created
    #[allow(dead_code)]
    created_at: std::time::Instant,
}

/// Connection pool for a single backend
struct ConnectionPool {
    /// Available connections
    connections: Mutex<Vec<PooledConnection>>,
    /// Maximum pool size
    max_size: usize,
    /// Semaphore to limit concurrent connections
    semaphore: Semaphore,
    /// Backend address
    address: SocketAddr,
    /// Connection timeout
    connect_timeout: Duration,
}

impl ConnectionPool {
    /// Create a new connection pool
    fn new(address: SocketAddr, max_size: usize, connect_timeout: Duration) -> Self {
        Self {
            connections: Mutex::new(Vec::with_capacity(max_size)),
            max_size,
            semaphore: Semaphore::new(max_size),
            address,
            connect_timeout,
        }
    }

    /// Get a connection from the pool or create a new one
    async fn get(&self) -> Result<TcpStream, ProxyError> {
        // Try to acquire semaphore permit
        let _permit = self
            .semaphore
            .acquire()
            .await
            .map_err(|_| ProxyError::ConnectionFailed("Pool closed".to_string()))?;

        // Try to get an existing connection
        {
            let mut connections = self.connections.lock().await;
            if let Some(pooled) = connections.pop() {
                // Check if connection is still valid
                // Note: We release the permit when the connection is returned
                return Ok(pooled.stream);
            }
        }

        // Create a new connection
        let stream =
            timeout(self.connect_timeout, TcpStream::connect(self.address))
                .await
                .map_err(|_| ProxyError::Timeout)?
                .map_err(|e| ProxyError::ConnectionFailed(e.to_string()))?;

        // Set TCP options for performance
        stream.set_nodelay(true).ok();

        Ok(stream)
    }

    /// Return a connection to the pool
    async fn put(&self, stream: TcpStream) {
        let mut connections = self.connections.lock().await;
        if connections.len() < self.max_size {
            connections.push(PooledConnection {
                stream,
                created_at: std::time::Instant::now(),
            });
        }
        // If pool is full, connection will be dropped
    }

    /// Get pool statistics
    #[allow(dead_code)]
    async fn stats(&self) -> (usize, usize) {
        let connections = self.connections.lock().await;
        (connections.len(), self.max_size)
    }
}

/// Backend server representation
pub struct Backend {
    /// Backend name
    pub name: String,
    /// Backend address
    pub address: SocketAddr,
    /// Weight for load balancing
    pub weight: u32,
    /// Connection pool
    pool: ConnectionPool,
    /// Health check path
    pub health_check_path: Option<String>,
    /// Is backend healthy
    healthy: std::sync::atomic::AtomicBool,
}

impl Backend {
    /// Create a new backend from configuration
    pub fn from_config(config: &BackendConfig) -> Result<Self, ProxyError> {
        let address: SocketAddr = config
            .address
            .parse()
            .map_err(|e| ProxyError::InvalidConfig(format!("Invalid address: {}", e)))?;

        Ok(Self {
            name: config.name.clone(),
            address,
            weight: config.weight,
            pool: ConnectionPool::new(address, config.pool_size, Duration::from_secs(5)),
            health_check_path: config.health_check_path.clone(),
            healthy: std::sync::atomic::AtomicBool::new(true),
        })
    }

    /// Check if backend is healthy
    pub fn is_healthy(&self) -> bool {
        self.healthy.load(Ordering::Relaxed)
    }

    /// Set backend health status
    pub fn set_healthy(&self, healthy: bool) {
        self.healthy.store(healthy, Ordering::Relaxed);
    }

    /// Get a connection from the pool
    pub async fn get_connection(&self) -> Result<TcpStream, ProxyError> {
        self.pool.get().await
    }

    /// Return a connection to the pool
    pub async fn return_connection(&self, stream: TcpStream) {
        self.pool.put(stream).await;
    }
}

/// Load balancing strategy
#[derive(Debug, Clone, Copy)]
pub enum LoadBalanceStrategy {
    /// Round-robin selection
    RoundRobin,
    /// Weighted round-robin
    WeightedRoundRobin,
    /// Least connections
    LeastConnections,
    /// Random selection
    Random,
}

/// Load balancer for backend selection
pub struct LoadBalancer {
    /// Available backends
    backends: Vec<Arc<Backend>>,
    /// Current index for round-robin
    current_index: AtomicUsize,
    /// Load balancing strategy
    strategy: LoadBalanceStrategy,
    /// Weighted indices (for weighted round-robin)
    weighted_indices: Vec<usize>,
    /// Current weight index
    weight_index: AtomicUsize,
}

impl LoadBalancer {
    /// Create a new load balancer
    pub fn new(backends: Vec<Arc<Backend>>, strategy: LoadBalanceStrategy) -> Self {
        // Build weighted indices
        let mut weighted_indices = Vec::new();
        for (i, backend) in backends.iter().enumerate() {
            for _ in 0..backend.weight {
                weighted_indices.push(i);
            }
        }

        Self {
            backends,
            current_index: AtomicUsize::new(0),
            strategy,
            weighted_indices,
            weight_index: AtomicUsize::new(0),
        }
    }

    /// Select a backend
    pub fn select(&self) -> Option<Arc<Backend>> {
        let healthy_backends: Vec<_> = self
            .backends
            .iter()
            .filter(|b| b.is_healthy())
            .cloned()
            .collect();

        if healthy_backends.is_empty() {
            return None;
        }

        match self.strategy {
            LoadBalanceStrategy::RoundRobin => {
                let index = self.current_index.fetch_add(1, Ordering::Relaxed);
                Some(healthy_backends[index % healthy_backends.len()].clone())
            }
            LoadBalanceStrategy::WeightedRoundRobin => {
                if self.weighted_indices.is_empty() {
                    return None;
                }
                let index = self.weight_index.fetch_add(1, Ordering::Relaxed);
                let backend_index = self.weighted_indices[index % self.weighted_indices.len()];
                if self.backends[backend_index].is_healthy() {
                    Some(self.backends[backend_index].clone())
                } else {
                    // Fallback to round-robin for healthy backends
                    let fallback_index = self.current_index.fetch_add(1, Ordering::Relaxed);
                    Some(healthy_backends[fallback_index % healthy_backends.len()].clone())
                }
            }
            LoadBalanceStrategy::LeastConnections => {
                // For simplicity, just use round-robin
                // A proper implementation would track active connections
                let index = self.current_index.fetch_add(1, Ordering::Relaxed);
                Some(healthy_backends[index % healthy_backends.len()].clone())
            }
            LoadBalanceStrategy::Random => {
                use std::collections::hash_map::RandomState;
                use std::hash::{BuildHasher, Hasher};
                let state = RandomState::new();
                let mut hasher = state.build_hasher();
                hasher.write_usize(self.current_index.fetch_add(1, Ordering::Relaxed));
                let index = hasher.finish() as usize;
                Some(healthy_backends[index % healthy_backends.len()].clone())
            }
        }
    }

    /// Get all backends
    pub fn backends(&self) -> &[Arc<Backend>] {
        &self.backends
    }
}

/// The reverse proxy
pub struct ReverseProxy {
    /// Load balancer
    load_balancer: Arc<LoadBalancer>,
    /// Circuit breaker manager
    circuit_breakers: Arc<CircuitBreakerManager>,
    /// Request timeout
    request_timeout: Duration,
    /// Metrics registry
    metrics: Arc<MetricsRegistry>,
}

impl ReverseProxy {
    /// Create a new reverse proxy
    pub fn new(
        backends: Vec<Arc<Backend>>,
        circuit_breakers: Arc<CircuitBreakerManager>,
        metrics: Arc<MetricsRegistry>,
        request_timeout: Duration,
    ) -> Self {
        Self {
            load_balancer: Arc::new(LoadBalancer::new(backends, LoadBalanceStrategy::RoundRobin)),
            circuit_breakers,
            request_timeout,
            metrics,
        }
    }

    /// Forward a request to a backend.
    /// Body type is `Full<Bytes>` because the router has already buffered
    /// the request body before calling here (router.rs constructs the
    /// forward Request<Full<Bytes>>); hyper 1.x doesn't allow user code
    /// to construct Request<Incoming>, only the server side does.
    pub async fn forward(
        &self,
        mut request: Request<Full<Bytes>>,
    ) -> Result<Response<Full<Bytes>>, ProxyError> {
        // Select a backend
        let backend = self
            .load_balancer
            .select()
            .ok_or(ProxyError::NoBackends)?;

        // Check circuit breaker
        let circuit_breaker = self.circuit_breakers.get(&backend.name).await;
        if circuit_breaker.state().await == CircuitState::Open {
            self.metrics
                .backend_errors
                .with_labels(&[("backend", &backend.name), ("error_type", "circuit_open")])
                .await
                .inc();
            return Err(ProxyError::CircuitOpen);
        }

        if !circuit_breaker.allow_request().await {
            return Err(ProxyError::CircuitOpen);
        }

        // Get connection from pool
        let stream = match backend.get_connection().await {
            Ok(s) => s,
            Err(e) => {
                circuit_breaker.record_failure().await;
                self.metrics
                    .backend_errors
                    .with_labels(&[("backend", &backend.name), ("error_type", "connection")])
                    .await
                    .inc();
                return Err(e);
            }
        };

        // Update request headers
        let host = backend.address.to_string();
        request.headers_mut().insert(
            hyper::header::HOST,
            host.parse().unwrap(),
        );

        // Add X-Forwarded headers
        // Note: In production, you'd want to preserve existing values
        // Copy the SocketAddr out so the immutable extensions() borrow is dropped
        // before headers_mut() mutates the same request.
        let client_addr = request.extensions().get::<SocketAddr>().copied();
        if let Some(addr) = client_addr {
            request.headers_mut().insert(
                "X-Forwarded-For",
                addr.ip().to_string().parse().unwrap(),
            );
        }

        request.headers_mut().insert(
            "X-Forwarded-Proto",
            "http".parse().unwrap(),
        );

        // Create HTTP connection
        let (mut sender, conn) = match http1::handshake(TokioIo::new(stream)).await {
            Ok(c) => c,
            Err(e) => {
                circuit_breaker.record_failure().await;
                self.metrics
                    .backend_errors
                    .with_labels(&[("backend", &backend.name), ("error_type", "handshake")])
                    .await
                    .inc();
                return Err(ProxyError::ForwardFailed(e.to_string()));
            }
        };

        // Spawn connection handler
        tokio::spawn(async move {
            if let Err(e) = conn.await {
                eprintln!("Connection error: {}", e);
            }
        });

        // Send request with timeout
        let response = match timeout(self.request_timeout, sender.send_request(request)).await {
            Ok(Ok(resp)) => resp,
            Ok(Err(e)) => {
                circuit_breaker.record_failure().await;
                self.metrics
                    .backend_errors
                    .with_labels(&[("backend", &backend.name), ("error_type", "request")])
                    .await
                    .inc();
                return Err(ProxyError::ForwardFailed(e.to_string()));
            }
            Err(_) => {
                circuit_breaker.record_failure().await;
                self.metrics
                    .backend_errors
                    .with_labels(&[("backend", &backend.name), ("error_type", "timeout")])
                    .await
                    .inc();
                return Err(ProxyError::Timeout);
            }
        };

        // Record success
        circuit_breaker.record_success().await;

        // Convert response body
        let (parts, body) = response.into_parts();
        let body_bytes = body
            .collect()
            .await
            .map_err(|e| ProxyError::ForwardFailed(e.to_string()))?
            .to_bytes();

        Ok(Response::from_parts(parts, Full::new(body_bytes)))
    }

    /// Get load balancer for health checks
    pub fn load_balancer(&self) -> &Arc<LoadBalancer> {
        &self.load_balancer
    }

    /// Create an error response
    pub fn error_response(error: &ProxyError) -> Response<Full<Bytes>> {
        let (status, message) = match error {
            ProxyError::CircuitOpen => (
                StatusCode::SERVICE_UNAVAILABLE,
                "Service temporarily unavailable",
            ),
            ProxyError::NoBackends => (
                StatusCode::SERVICE_UNAVAILABLE,
                "No healthy backends available",
            ),
            ProxyError::Timeout => (StatusCode::GATEWAY_TIMEOUT, "Backend timeout"),
            ProxyError::ConnectionFailed(_) => (StatusCode::BAD_GATEWAY, "Backend connection failed"),
            ProxyError::ForwardFailed(_) => (StatusCode::BAD_GATEWAY, "Request forwarding failed"),
            ProxyError::InvalidConfig(_) => {
                (StatusCode::INTERNAL_SERVER_ERROR, "Configuration error")
            }
        };

        let body = format!(
            r#"{{"error":"{}","message":"{}"}}"#,
            status.as_u16(),
            message
        );

        Response::builder()
            .status(status)
            .header("Content-Type", "application/json")
            .body(Full::new(Bytes::from(body)))
            .unwrap()
    }
}

/// Health checker for backends
pub struct HealthChecker {
    /// Backends to check
    backends: Vec<Arc<Backend>>,
    /// Check interval
    interval: Duration,
    /// Check timeout
    timeout: Duration,
}

impl HealthChecker {
    /// Create a new health checker
    pub fn new(backends: Vec<Arc<Backend>>, interval: Duration, check_timeout: Duration) -> Self {
        Self {
            backends,
            interval,
            timeout: check_timeout,
        }
    }

    /// Start the health check loop
    pub fn start(self) -> tokio::task::JoinHandle<()> {
        tokio::spawn(async move {
            let mut ticker = tokio::time::interval(self.interval);

            loop {
                ticker.tick().await;

                for backend in &self.backends {
                    let healthy = self.check_backend(backend).await;
                    backend.set_healthy(healthy);
                }
            }
        })
    }

    /// Check a single backend
    async fn check_backend(&self, backend: &Backend) -> bool {
        let path = backend
            .health_check_path
            .as_deref()
            .unwrap_or("/health");

        match timeout(self.timeout, TcpStream::connect(backend.address)).await {
            Ok(Ok(stream)) => {
                // Try to send a simple HTTP request
                stream.set_nodelay(true).ok();

                let (mut sender, conn) = match http1::handshake(TokioIo::new(stream)).await {
                    Ok(c) => c,
                    Err(_) => return false,
                };

                tokio::spawn(async move {
                    let _ = conn.await;
                });

                let request = Request::builder()
                    .method("GET")
                    .uri(path)
                    .header("Host", backend.address.to_string())
                    .body(Full::<Bytes>::new(Bytes::new()))
                    .unwrap();

                match timeout(self.timeout, sender.send_request(request)).await {
                    Ok(Ok(response)) => response.status().is_success(),
                    _ => false,
                }
            }
            _ => false,
        }
    }
}

/// Tokio IO adapter for hyper
pub struct TokioIo<T>(T);

impl<T> TokioIo<T> {
    pub fn new(inner: T) -> Self {
        Self(inner)
    }

    pub fn inner(&self) -> &T {
        &self.0
    }
}

impl<T: AsyncRead + Unpin> hyper::rt::Read for TokioIo<T> {
    fn poll_read(
        mut self: std::pin::Pin<&mut Self>,
        cx: &mut std::task::Context<'_>,
        mut buf: hyper::rt::ReadBufCursor<'_>,
    ) -> std::task::Poll<std::io::Result<()>> {
        let n = unsafe {
            let mut tbuf = tokio::io::ReadBuf::uninit(buf.as_mut());
            match std::pin::Pin::new(&mut self.0).poll_read(cx, &mut tbuf) {
                std::task::Poll::Ready(Ok(())) => tbuf.filled().len(),
                other => return other,
            }
        };
        unsafe {
            buf.advance(n);
        }
        std::task::Poll::Ready(Ok(()))
    }
}

impl<T: AsyncWrite + Unpin> hyper::rt::Write for TokioIo<T> {
    fn poll_write(
        mut self: std::pin::Pin<&mut Self>,
        cx: &mut std::task::Context<'_>,
        buf: &[u8],
    ) -> std::task::Poll<std::io::Result<usize>> {
        std::pin::Pin::new(&mut self.0).poll_write(cx, buf)
    }

    fn poll_flush(
        mut self: std::pin::Pin<&mut Self>,
        cx: &mut std::task::Context<'_>,
    ) -> std::task::Poll<std::io::Result<()>> {
        std::pin::Pin::new(&mut self.0).poll_flush(cx)
    }

    fn poll_shutdown(
        mut self: std::pin::Pin<&mut Self>,
        cx: &mut std::task::Context<'_>,
    ) -> std::task::Poll<std::io::Result<()>> {
        std::pin::Pin::new(&mut self.0).poll_shutdown(cx)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_backend_from_config() {
        let config = BackendConfig {
            name: "test".to_string(),
            address: "127.0.0.1:8080".to_string(),
            weight: 1,
            health_check_path: Some("/health".to_string()),
            pool_size: 10,
        };

        let backend = Backend::from_config(&config).unwrap();
        assert_eq!(backend.name, "test");
        assert!(backend.is_healthy());
    }

    #[test]
    fn test_load_balancer_round_robin() {
        let backends = vec![
            Arc::new(Backend::from_config(&BackendConfig {
                name: "b1".to_string(),
                address: "127.0.0.1:8001".to_string(),
                weight: 1,
                health_check_path: None,
                pool_size: 10,
            }).unwrap()),
            Arc::new(Backend::from_config(&BackendConfig {
                name: "b2".to_string(),
                address: "127.0.0.1:8002".to_string(),
                weight: 1,
                health_check_path: None,
                pool_size: 10,
            }).unwrap()),
        ];

        let lb = LoadBalancer::new(backends, LoadBalanceStrategy::RoundRobin);

        let b1 = lb.select().unwrap();
        let b2 = lb.select().unwrap();
        let b3 = lb.select().unwrap();

        // Should cycle through backends
        assert_ne!(b1.name, b2.name);
        assert_eq!(b1.name, b3.name);
    }

    #[test]
    fn test_load_balancer_no_healthy() {
        let backend = Arc::new(Backend::from_config(&BackendConfig {
            name: "b1".to_string(),
            address: "127.0.0.1:8001".to_string(),
            weight: 1,
            health_check_path: None,
            pool_size: 10,
        }).unwrap());

        backend.set_healthy(false);

        let lb = LoadBalancer::new(vec![backend], LoadBalanceStrategy::RoundRobin);
        assert!(lb.select().is_none());
    }
}
