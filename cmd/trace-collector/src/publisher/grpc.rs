//! gRPC client for Wotan integration.
//!
//! THE WHISPERING VOID speaks to Wotan, the realm's message courier,
//! delivering kernel whispers to eager listeners.
//!
//! This module provides the low-level gRPC client for communicating
//! with the Wotan message bus service.

use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::Arc;
use std::time::{Duration, Instant};

use anyhow::{Context, Result};
use parking_lot::RwLock;
use prost::Message;
use tonic::transport::{Channel, Endpoint};
use tracing::{debug, error, info, warn};

use crate::metrics;
use crate::proto::{HealthResponse, PublishResponse, TraceEventBatch};

/// Connection timeout
const CONNECT_TIMEOUT: Duration = Duration::from_secs(5);

/// Request timeout
const REQUEST_TIMEOUT: Duration = Duration::from_secs(10);

/// Maximum retry attempts
const MAX_RETRIES: u32 = 5;

/// Base retry backoff
const BASE_RETRY_BACKOFF: Duration = Duration::from_millis(100);

/// Maximum retry backoff
const MAX_RETRY_BACKOFF: Duration = Duration::from_secs(30);

/// gRPC client state
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ClientState {
    /// Not connected
    Disconnected,
    /// Attempting to connect
    Connecting,
    /// Connected and ready
    Connected,
    /// Connection failed
    Failed,
}

/// gRPC client statistics
#[derive(Debug, Default)]
pub struct ClientStats {
    /// Total requests sent
    pub requests_sent: AtomicU64,
    /// Total successful responses
    pub responses_ok: AtomicU64,
    /// Total failed requests
    pub requests_failed: AtomicU64,
    /// Total bytes sent
    pub bytes_sent: AtomicU64,
    /// Total reconnections
    pub reconnections: AtomicU64,
    /// Last request latency (microseconds)
    pub last_latency_us: AtomicU64,
}

impl ClientStats {
    pub fn new() -> Self {
        Self::default()
    }
}

/// gRPC client for Wotan
pub struct WotanClient {
    /// Endpoint URL
    endpoint: String,
    /// gRPC channel
    channel: RwLock<Option<Channel>>,
    /// Current state
    state: RwLock<ClientState>,
    /// Client statistics
    stats: Arc<ClientStats>,
    /// Sequence number for requests
    #[allow(dead_code)]
    sequence: AtomicU64,
}

impl WotanClient {
    /// Create a new Wotan client
    pub async fn new(endpoint: &str) -> Result<Self> {
        let client = Self {
            endpoint: endpoint.to_string(),
            channel: RwLock::new(None),
            state: RwLock::new(ClientState::Disconnected),
            stats: Arc::new(ClientStats::new()),
            sequence: AtomicU64::new(0),
        };

        // Try initial connection
        client.connect().await?;

        Ok(client)
    }

    /// Connect to Wotan
    pub async fn connect(&self) -> Result<()> {
        *self.state.write() = ClientState::Connecting;

        info!(endpoint = self.endpoint, "Connecting to Wotan");

        let endpoint = Endpoint::from_shared(self.endpoint.clone())
            .context("Invalid endpoint URL")?
            .connect_timeout(CONNECT_TIMEOUT)
            .timeout(REQUEST_TIMEOUT)
            .tcp_keepalive(Some(Duration::from_secs(30)))
            .tcp_nodelay(true);

        match tokio::time::timeout(CONNECT_TIMEOUT, endpoint.connect()).await {
            Ok(Ok(channel)) => {
                *self.channel.write() = Some(channel);
                *self.state.write() = ClientState::Connected;
                metrics::set_connection_state(&self.endpoint, true);
                info!("Connected to Wotan");
                Ok(())
            }
            Ok(Err(e)) => {
                *self.state.write() = ClientState::Failed;
                metrics::set_connection_state(&self.endpoint, false);
                Err(anyhow::anyhow!("Connection failed: {}", e))
            }
            Err(_) => {
                *self.state.write() = ClientState::Failed;
                metrics::set_connection_state(&self.endpoint, false);
                Err(anyhow::anyhow!("Connection timeout"))
            }
        }
    }

    /// Reconnect with exponential backoff
    pub async fn reconnect(&self) -> Result<()> {
        let mut backoff = BASE_RETRY_BACKOFF;
        let mut attempts = 0;

        while attempts < MAX_RETRIES {
            attempts += 1;
            warn!(
                attempt = attempts,
                backoff_ms = backoff.as_millis(),
                "Attempting to reconnect to Wotan"
            );

            tokio::time::sleep(backoff).await;

            match self.connect().await {
                Ok(()) => {
                    self.stats.reconnections.fetch_add(1, Ordering::Relaxed);
                    return Ok(());
                }
                Err(e) => {
                    error!(attempt = attempts, error = %e, "Reconnection failed");
                }
            }

            // Exponential backoff with jitter
            backoff = (backoff * 2).min(MAX_RETRY_BACKOFF);
            let jitter = Duration::from_millis(rand_jitter(100));
            backoff += jitter;
        }

        Err(anyhow::anyhow!(
            "Failed to reconnect after {} attempts",
            MAX_RETRIES
        ))
    }

    /// Get current state
    pub fn state(&self) -> ClientState {
        *self.state.read()
    }

    /// Check if connected
    pub fn is_connected(&self) -> bool {
        self.state() == ClientState::Connected && self.channel.read().is_some()
    }

    /// Get statistics
    pub fn stats(&self) -> &Arc<ClientStats> {
        &self.stats
    }

    /// Get next sequence number
    #[allow(dead_code)]
    fn next_sequence(&self) -> u64 {
        self.sequence.fetch_add(1, Ordering::SeqCst)
    }

    /// Publish a batch of events
    pub async fn publish(&self, batch: TraceEventBatch) -> Result<PublishResponse> {
        let start = Instant::now();
        let batch_size = batch.events.len();

        // Ensure connected
        let channel = {
            let guard = self.channel.read();
            guard.clone()
        };

        let channel = match channel {
            Some(c) => c,
            None => {
                self.reconnect().await?;
                self.channel
                    .read()
                    .clone()
                    .ok_or_else(|| anyhow::anyhow!("No channel after reconnect"))?
            }
        };

        // Encode the batch
        let mut buf = Vec::with_capacity(batch.encoded_len());
        batch.encode(&mut buf).context("Failed to encode batch")?;

        let bytes_sent = buf.len();
        self.stats
            .bytes_sent
            .fetch_add(bytes_sent as u64, Ordering::Relaxed);
        self.stats.requests_sent.fetch_add(1, Ordering::Relaxed);

        debug!(
            batch_size = batch_size,
            bytes = bytes_sent,
            sequence = batch.sequence,
            "Sending batch to Wotan"
        );

        // Make the gRPC call (simulated for now)
        let result = self.do_publish(channel, buf, batch.sequence).await;

        let latency_us = start.elapsed().as_micros() as u64;
        self.stats
            .last_latency_us
            .store(latency_us, Ordering::Relaxed);

        match result {
            Ok(response) => {
                self.stats.responses_ok.fetch_add(1, Ordering::Relaxed);
                metrics::record_publish_latency(latency_us as f64, true);
                Ok(response)
            }
            Err(e) => {
                self.stats.requests_failed.fetch_add(1, Ordering::Relaxed);
                metrics::record_publish_latency(latency_us as f64, false);

                // Check if we need to reconnect
                if Self::is_connection_error(&e) {
                    warn!("Connection error, scheduling reconnection");
                    *self.state.write() = ClientState::Disconnected;
                    *self.channel.write() = None;
                    metrics::set_connection_state(&self.endpoint, false);
                }

                Err(e)
            }
        }
    }

    /// Perform the actual publish (simulated gRPC call)
    async fn do_publish(
        &self,
        _channel: Channel,
        payload: Vec<u8>,
        sequence: u64,
    ) -> Result<PublishResponse> {
        // In a real implementation, this would use the generated gRPC client stub
        // For demonstration, we simulate the server response

        // Simulate network latency
        tokio::time::sleep(Duration::from_micros(100)).await;

        // Decode to get event count
        let batch = TraceEventBatch::decode(&payload[..]).context("Failed to decode batch")?;

        Ok(PublishResponse {
            success: true,
            accepted: batch.events.len() as u64,
            rejected: 0,
            error: String::new(),
            server_sequence: sequence,
        })
    }

    /// Check health of Wotan
    pub async fn health_check(&self) -> Result<HealthResponse> {
        let channel = {
            let guard = self.channel.read();
            guard.clone()
        };

        let _channel = match channel {
            Some(c) => c,
            None => return Err(anyhow::anyhow!("Not connected")),
        };

        // Simulated health response
        Ok(HealthResponse {
            healthy: true,
            version: "1.0.0".to_string(),
            uptime_seconds: 0,
            message_rate: 0.0,
        })
    }

    /// Check if error is a connection error
    fn is_connection_error(e: &anyhow::Error) -> bool {
        let error_str = e.to_string().to_lowercase();
        error_str.contains("connection")
            || error_str.contains("unavailable")
            || error_str.contains("refused")
            || error_str.contains("reset")
            || error_str.contains("timeout")
    }
}

/// Simple random jitter
fn rand_jitter(max_ms: u64) -> u64 {
    use std::time::SystemTime;
    let seed = SystemTime::now()
        .duration_since(SystemTime::UNIX_EPOCH)
        .unwrap_or_default()
        .as_nanos() as u64;
    seed % max_ms
}

/// gRPC streaming client for high-throughput scenarios
pub struct StreamingClient {
    /// Inner client
    client: Arc<WotanClient>,
    /// Streaming state
    streaming: AtomicBool,
}

impl StreamingClient {
    /// Create a new streaming client
    pub async fn new(endpoint: &str) -> Result<Self> {
        let client = WotanClient::new(endpoint).await?;
        Ok(Self {
            client: Arc::new(client),
            streaming: AtomicBool::new(false),
        })
    }

    /// Start streaming mode
    pub fn start_stream(&self) {
        self.streaming.store(true, Ordering::SeqCst);
    }

    /// Stop streaming mode
    pub fn stop_stream(&self) {
        self.streaming.store(false, Ordering::SeqCst);
    }

    /// Check if streaming
    pub fn is_streaming(&self) -> bool {
        self.streaming.load(Ordering::Relaxed)
    }

    /// Send a batch (in streaming mode or regular)
    pub async fn send(&self, batch: TraceEventBatch) -> Result<PublishResponse> {
        self.client.publish(batch).await
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_client_state() {
        assert_eq!(ClientState::Disconnected, ClientState::Disconnected);
        assert_ne!(ClientState::Connected, ClientState::Disconnected);
    }

    #[test]
    fn test_client_stats() {
        let stats = ClientStats::new();
        stats.requests_sent.store(100, Ordering::SeqCst);
        assert_eq!(stats.requests_sent.load(Ordering::SeqCst), 100);
    }

    #[test]
    fn test_rand_jitter() {
        let jitter = rand_jitter(100);
        assert!(jitter < 100);
    }

    #[test]
    fn test_is_connection_error() {
        let err = anyhow::anyhow!("connection refused");
        assert!(WotanClient::is_connection_error(&err));

        let err = anyhow::anyhow!("parse error");
        assert!(!WotanClient::is_connection_error(&err));
    }
}
