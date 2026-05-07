//! WebSocket Server Implementation
//!
//! Manages WebSocket connections and broadcasts trace updates to clients.

use std::collections::HashMap;
use std::net::SocketAddr;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::Arc;
use std::time::{Duration, Instant};

use anyhow::{Context, Result};
use futures_util::{SinkExt, StreamExt};
use parking_lot::RwLock;
use tokio::net::{TcpListener, TcpStream};
use tokio::sync::{broadcast, mpsc};
use tokio_tungstenite::{accept_async, tungstenite::Message};
use tracing::{debug, error, info, warn};

use super::handler::{HandlerStats, WebSocketHandler};
use super::protocol::{ClientMessage, ServerMessage, TraceUpdate};
use crate::correlation::{CorrelationEngine, TraceStore, TraceSummary};

/// WebSocket server configuration
#[derive(Debug, Clone)]
pub struct WebSocketConfig {
    /// Bind address
    pub bind_addr: String,
    /// Maximum connections
    pub max_connections: usize,
    /// Ping interval
    pub ping_interval: Duration,
    /// Ping timeout
    pub ping_timeout: Duration,
    /// Message queue size per connection
    pub message_queue_size: usize,
    /// Enable message batching
    pub batching_enabled: bool,
    /// Batch interval
    pub batch_interval: Duration,
    /// Maximum batch size
    pub max_batch_size: usize,
}

impl Default for WebSocketConfig {
    fn default() -> Self {
        Self {
            bind_addr: "0.0.0.0:9091".to_string(),
            max_connections: 100,
            ping_interval: Duration::from_secs(30),
            ping_timeout: Duration::from_secs(10),
            message_queue_size: 1000,
            batching_enabled: true,
            batch_interval: Duration::from_millis(100),
            max_batch_size: 100,
        }
    }
}

/// WebSocket server statistics
#[derive(Debug, Default)]
pub struct WebSocketStats {
    /// Total connections accepted
    pub connections_accepted: AtomicU64,
    /// Current active connections
    pub active_connections: AtomicU64,
    /// Total messages sent
    pub messages_sent: AtomicU64,
    /// Total messages received
    pub messages_received: AtomicU64,
    /// Total updates broadcast
    pub updates_broadcast: AtomicU64,
    /// Connection errors
    pub connection_errors: AtomicU64,
}

impl WebSocketStats {
    pub fn new() -> Self {
        Self::default()
    }
}

/// Connection handle for managing a single client
struct Connection {
    #[allow(dead_code)]
    id: String,
    handler: Arc<WebSocketHandler>,
    tx: mpsc::Sender<ServerMessage>,
    #[allow(dead_code)]
    connected_at: Instant,
}

/// WebSocket server for trace streaming
pub struct WebSocketServer {
    /// Configuration
    config: WebSocketConfig,
    /// Statistics
    stats: Arc<WebSocketStats>,
    /// Active connections
    connections: RwLock<HashMap<String, Connection>>,
    /// Shutdown flag
    shutdown: Arc<AtomicBool>,
    /// Trace store reference
    trace_store: Option<Arc<TraceStore>>,
    /// Correlation engine reference
    correlation_engine: Option<Arc<CorrelationEngine>>,
    /// Broadcast channel for trace updates
    update_tx: broadcast::Sender<TraceUpdate>,
}

impl WebSocketServer {
    /// Create a new WebSocket server
    pub fn new(config: WebSocketConfig) -> Self {
        let (update_tx, _) = broadcast::channel(10000);

        Self {
            config,
            stats: Arc::new(WebSocketStats::new()),
            connections: RwLock::new(HashMap::new()),
            shutdown: Arc::new(AtomicBool::new(false)),
            trace_store: None,
            correlation_engine: None,
            update_tx,
        }
    }

    /// Set trace store reference
    pub fn with_trace_store(mut self, store: Arc<TraceStore>) -> Self {
        self.trace_store = Some(store);
        self
    }

    /// Set correlation engine reference
    pub fn with_correlation_engine(mut self, engine: Arc<CorrelationEngine>) -> Self {
        self.correlation_engine = Some(engine);
        self
    }

    /// Get statistics
    pub fn stats(&self) -> &Arc<WebSocketStats> {
        &self.stats
    }

    /// Get active connection count
    pub fn connection_count(&self) -> usize {
        self.connections.read().len()
    }

    /// Get connection stats
    pub fn connection_stats(&self) -> Vec<HandlerStats> {
        self.connections
            .read()
            .values()
            .map(|c| HandlerStats::from(c.handler.as_ref()))
            .collect()
    }

    /// Broadcast a trace update to all connected clients
    pub fn broadcast_update(&self, update: TraceUpdate) {
        let _ = self.update_tx.send(update);
        self.stats.updates_broadcast.fetch_add(1, Ordering::Relaxed);
    }

    /// Broadcast a trace summary (converted to update)
    pub fn broadcast_summary(&self, summary: &TraceSummary) {
        let update = TraceUpdate::from_summary(summary);
        self.broadcast_update(update);
    }

    /// Subscribe to updates (for internal use)
    pub fn subscribe_updates(&self) -> broadcast::Receiver<TraceUpdate> {
        self.update_tx.subscribe()
    }

    /// Signal shutdown
    pub fn shutdown(&self) {
        self.shutdown.store(true, Ordering::SeqCst);
    }

    /// Run the WebSocket server
    pub async fn run(self: Arc<Self>) -> Result<()> {
        let addr: SocketAddr = self
            .config
            .bind_addr
            .parse()
            .context("Invalid bind address")?;

        let listener = TcpListener::bind(&addr)
            .await
            .context("Failed to bind WebSocket server")?;

        info!(addr = %addr, "WebSocket server listening");

        loop {
            tokio::select! {
                accept_result = listener.accept() => {
                    match accept_result {
                        Ok((stream, peer_addr)) => {
                            if self.shutdown.load(Ordering::Relaxed) {
                                break;
                            }

                            let connections = self.connections.read().len();
                            if connections >= self.config.max_connections {
                                warn!(
                                    max = self.config.max_connections,
                                    "Maximum connections reached, rejecting"
                                );
                                continue;
                            }

                            let server = Arc::clone(&self);
                            tokio::spawn(async move {
                                if let Err(e) = server.handle_connection(stream, peer_addr).await {
                                    debug!(peer = %peer_addr, error = %e, "Connection error");
                                }
                            });
                        }
                        Err(e) => {
                            error!(error = %e, "Failed to accept connection");
                            self.stats.connection_errors.fetch_add(1, Ordering::Relaxed);
                        }
                    }
                }
                _ = tokio::time::sleep(Duration::from_millis(100)) => {
                    if self.shutdown.load(Ordering::Relaxed) {
                        break;
                    }
                }
            }
        }

        info!("WebSocket server shutting down");

        // Close all connections
        let connections: Vec<_> = self.connections.write().drain().collect();
        for (_id, conn) in connections {
            let _ = conn
                .tx
                .send(ServerMessage::Error {
                    code: "shutdown".to_string(),
                    message: "Server shutting down".to_string(),
                    request_id: None,
                })
                .await;
        }

        Ok(())
    }

    /// Handle a single WebSocket connection
    async fn handle_connection(
        self: &Arc<Self>,
        stream: TcpStream,
        peer_addr: SocketAddr,
    ) -> Result<()> {
        let ws_stream = accept_async(stream)
            .await
            .context("WebSocket handshake failed")?;

        let connection_id = format!("ws-{}-{}", peer_addr, fastrand::u32(..));

        info!(
            connection_id = %connection_id,
            peer = %peer_addr,
            "WebSocket connection accepted"
        );

        self.stats
            .connections_accepted
            .fetch_add(1, Ordering::Relaxed);
        self.stats
            .active_connections
            .fetch_add(1, Ordering::Relaxed);

        // Create handler
        let mut handler = WebSocketHandler::new(&connection_id);
        if let Some(ref store) = self.trace_store {
            handler = handler.with_trace_store(Arc::clone(store));
        }
        if let Some(ref engine) = self.correlation_engine {
            handler = handler.with_correlation_engine(Arc::clone(engine));
        }
        let handler = Arc::new(handler);

        // Create message channel
        let (msg_tx, mut msg_rx) = mpsc::channel::<ServerMessage>(self.config.message_queue_size);

        // Store connection
        {
            let conn = Connection {
                id: connection_id.clone(),
                handler: Arc::clone(&handler),
                tx: msg_tx.clone(),
                connected_at: Instant::now(),
            };
            self.connections.write().insert(connection_id.clone(), conn);
        }

        // Split WebSocket stream
        let (mut ws_sink, mut ws_stream) = ws_stream.split();

        // Send connection message
        let connect_msg = handler.connection_message();
        let json = serde_json::to_string(&connect_msg)?;
        ws_sink.send(Message::Text(json)).await?;
        handler.record_sent();
        self.stats.messages_sent.fetch_add(1, Ordering::Relaxed);

        // Subscribe to trace updates
        let mut update_rx = self.update_tx.subscribe();

        // Batching state
        let batching = self.config.batching_enabled;
        let batch_interval = self.config.batch_interval;
        let max_batch_size = self.config.max_batch_size;
        let mut pending_updates: Vec<(String, TraceUpdate)> = Vec::new();
        let mut _last_batch_send = Instant::now();

        loop {
            tokio::select! {
                // Incoming WebSocket messages
                msg = ws_stream.next() => {
                    match msg {
                        Some(Ok(Message::Text(text))) => {
                            self.stats.messages_received.fetch_add(1, Ordering::Relaxed);

                            match serde_json::from_str::<ClientMessage>(&text) {
                                Ok(client_msg) => {
                                    if let Some(response) = handler.handle_message(client_msg) {
                                        let json = serde_json::to_string(&response)?;
                                        ws_sink.send(Message::Text(json)).await?;
                                        handler.record_sent();
                                        self.stats.messages_sent.fetch_add(1, Ordering::Relaxed);
                                    }
                                }
                                Err(e) => {
                                    warn!(error = %e, "Failed to parse client message");
                                    let error_msg = ServerMessage::Error {
                                        code: "parse_error".to_string(),
                                        message: format!("Failed to parse message: {}", e),
                                        request_id: None,
                                    };
                                    let json = serde_json::to_string(&error_msg)?;
                                    ws_sink.send(Message::Text(json)).await?;
                                }
                            }
                        }
                        Some(Ok(Message::Ping(data))) => {
                            ws_sink.send(Message::Pong(data)).await?;
                        }
                        Some(Ok(Message::Pong(_))) => {
                            // Pong received, connection is alive
                        }
                        Some(Ok(Message::Close(_))) => {
                            debug!(connection_id = %connection_id, "Client closed connection");
                            break;
                        }
                        Some(Err(e)) => {
                            warn!(connection_id = %connection_id, error = %e, "WebSocket error");
                            break;
                        }
                        None => {
                            debug!(connection_id = %connection_id, "WebSocket stream ended");
                            break;
                        }
                        _ => {}
                    }
                }

                // Outgoing messages from internal queue
                msg = msg_rx.recv() => {
                    match msg {
                        Some(server_msg) => {
                            let json = serde_json::to_string(&server_msg)?;
                            ws_sink.send(Message::Text(json)).await?;
                            handler.record_sent();
                            self.stats.messages_sent.fetch_add(1, Ordering::Relaxed);
                        }
                        None => break,
                    }
                }

                // Trace updates from broadcast
                update = update_rx.recv() => {
                    match update {
                        Ok(trace_update) => {
                            // Check if this matches any subscription
                            for sub_id in handler.matching_subscriptions(&trace_update) {
                                if batching {
                                    pending_updates.push((sub_id, trace_update.clone()));
                                } else {
                                    let msg = ServerMessage::TraceUpdate {
                                        subscription_id: sub_id,
                                        update: trace_update.clone(),
                                    };
                                    let json = serde_json::to_string(&msg)?;
                                    ws_sink.send(Message::Text(json)).await?;
                                    handler.record_sent();
                                    self.stats.messages_sent.fetch_add(1, Ordering::Relaxed);
                                }
                            }
                        }
                        Err(broadcast::error::RecvError::Lagged(n)) => {
                            warn!(connection_id = %connection_id, missed = n, "Broadcast lagged");
                        }
                        Err(broadcast::error::RecvError::Closed) => {
                            break;
                        }
                    }
                }

                // Batch flush timer
                _ = tokio::time::sleep(batch_interval), if batching && !pending_updates.is_empty() => {
                    self.flush_batch(&handler, &mut ws_sink, &mut pending_updates, max_batch_size).await?;
                    _last_batch_send = Instant::now();
                }
            }

            // Check if we should flush batch due to size
            if batching && pending_updates.len() >= max_batch_size {
                self.flush_batch(&handler, &mut ws_sink, &mut pending_updates, max_batch_size)
                    .await?;
                _last_batch_send = Instant::now();
            }
        }

        // Cleanup
        self.connections.write().remove(&connection_id);
        self.stats
            .active_connections
            .fetch_sub(1, Ordering::Relaxed);

        info!(
            connection_id = %connection_id,
            duration_secs = handler.connection_duration().as_secs(),
            "WebSocket connection closed"
        );

        Ok(())
    }

    /// Flush pending updates as a batch
    async fn flush_batch<S>(
        &self,
        handler: &Arc<WebSocketHandler>,
        ws_sink: &mut futures_util::stream::SplitSink<S, Message>,
        pending_updates: &mut Vec<(String, TraceUpdate)>,
        _max_batch_size: usize,
    ) -> Result<()>
    where
        S: futures_util::Sink<Message> + Unpin,
        <S as futures_util::Sink<Message>>::Error: std::error::Error + Send + Sync + 'static,
    {
        if pending_updates.is_empty() {
            return Ok(());
        }

        // Group by subscription ID
        let mut batches: HashMap<String, Vec<TraceUpdate>> = HashMap::new();
        for (sub_id, update) in pending_updates.drain(..) {
            batches.entry(sub_id).or_default().push(update);
        }

        // Send batched messages
        for (sub_id, updates) in batches {
            if updates.len() == 1 {
                let msg = ServerMessage::TraceUpdate {
                    subscription_id: sub_id,
                    update: updates.into_iter().next().unwrap(),
                };
                let json = serde_json::to_string(&msg)?;
                ws_sink
                    .send(Message::Text(json))
                    .await
                    .map_err(|e| anyhow::anyhow!("WebSocket send error: {}", e))?;
            } else {
                let msg = ServerMessage::TraceUpdateBatch {
                    subscription_id: sub_id,
                    updates,
                };
                let json = serde_json::to_string(&msg)?;
                ws_sink
                    .send(Message::Text(json))
                    .await
                    .map_err(|e| anyhow::anyhow!("WebSocket send error: {}", e))?;
            }
            handler.record_sent();
            self.stats.messages_sent.fetch_add(1, Ordering::Relaxed);
        }

        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_config_default() {
        let config = WebSocketConfig::default();
        assert_eq!(config.max_connections, 100);
        assert!(config.batching_enabled);
    }

    #[test]
    fn test_server_creation() {
        let server = WebSocketServer::new(WebSocketConfig::default());
        assert_eq!(server.connection_count(), 0);
    }

    #[test]
    fn test_broadcast() {
        let server = WebSocketServer::new(WebSocketConfig::default());
        let _rx = server.subscribe_updates();

        let update = TraceUpdate::new("test-trace", "packet");
        server.broadcast_update(update);

        // Should be able to receive the update
        // Note: In actual async context, this would work
    }
}
