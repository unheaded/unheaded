//! WebSocket Connection Handler
//!
//! Handles individual WebSocket connections, message parsing,
//! and subscription management.

use std::collections::HashMap;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

use parking_lot::RwLock;
use tracing::debug;

use super::protocol::{
    ClientMessage, ServerMessage, SpanEventInfo, SpanInfo, Subscription, TraceDetailsResponse,
    TraceQueryRequest, TraceUpdate,
};
use crate::correlation::{CorrelationEngine, Span, TraceStore, TraceSummary};

/// Handler state for a single WebSocket connection
pub struct WebSocketHandler {
    /// Connection ID
    pub connection_id: String,
    /// Active subscriptions
    subscriptions: RwLock<HashMap<String, Subscription>>,
    /// Client info
    client_name: RwLock<Option<String>>,
    client_version: RwLock<Option<String>>,
    /// Connection time
    connected_at: Instant,
    /// Messages sent
    messages_sent: AtomicU64,
    /// Messages received
    messages_received: AtomicU64,
    /// Reference to trace store (for queries)
    trace_store: Option<Arc<TraceStore>>,
    /// Reference to correlation engine
    correlation_engine: Option<Arc<CorrelationEngine>>,
}

impl WebSocketHandler {
    /// Create a new handler
    pub fn new(connection_id: impl Into<String>) -> Self {
        Self {
            connection_id: connection_id.into(),
            subscriptions: RwLock::new(HashMap::new()),
            client_name: RwLock::new(None),
            client_version: RwLock::new(None),
            connected_at: Instant::now(),
            messages_sent: AtomicU64::new(0),
            messages_received: AtomicU64::new(0),
            trace_store: None,
            correlation_engine: None,
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

    /// Get connection ID
    pub fn connection_id(&self) -> &str {
        &self.connection_id
    }

    /// Get connection duration
    pub fn connection_duration(&self) -> Duration {
        self.connected_at.elapsed()
    }

    /// Get subscription count
    pub fn subscription_count(&self) -> usize {
        self.subscriptions.read().len()
    }

    /// Generate the connection ack message
    pub fn connection_message(&self) -> ServerMessage {
        ServerMessage::Connected {
            version: env!("CARGO_PKG_VERSION").to_string(),
            connection_id: self.connection_id.clone(),
            capabilities: vec![
                "streaming".to_string(),
                "query".to_string(),
                "subscription".to_string(),
                "batching".to_string(),
            ],
        }
    }

    /// Handle an incoming client message
    pub fn handle_message(&self, message: ClientMessage) -> Option<ServerMessage> {
        self.messages_received.fetch_add(1, Ordering::Relaxed);

        match message {
            ClientMessage::Subscribe { id, subscription } => {
                debug!(
                    connection = %self.connection_id,
                    subscription_id = %id,
                    "New subscription"
                );

                self.subscriptions.write().insert(id.clone(), subscription);

                Some(ServerMessage::Subscribed {
                    id,
                    success: true,
                    error: None,
                })
            }

            ClientMessage::Unsubscribe { id } => {
                debug!(
                    connection = %self.connection_id,
                    subscription_id = %id,
                    "Unsubscribe"
                );

                self.subscriptions.write().remove(&id);

                Some(ServerMessage::Unsubscribed { id })
            }

            ClientMessage::UpdateSubscription { id, subscription } => {
                debug!(
                    connection = %self.connection_id,
                    subscription_id = %id,
                    "Update subscription"
                );

                let success = {
                    let mut subs = self.subscriptions.write();
                    if subs.contains_key(&id) {
                        subs.insert(id.clone(), subscription);
                        true
                    } else {
                        false
                    }
                };

                Some(ServerMessage::Subscribed {
                    id,
                    success,
                    error: if success {
                        None
                    } else {
                        Some("Subscription not found".to_string())
                    },
                })
            }

            ClientMessage::GetTrace {
                request_id,
                trace_id,
            } => {
                debug!(
                    connection = %self.connection_id,
                    trace_id = %trace_id,
                    "Get trace request"
                );

                let trace = self.get_trace_details(&trace_id);

                Some(ServerMessage::TraceDetails {
                    request_id,
                    trace,
                    error: None,
                })
            }

            ClientMessage::QueryTraces { request_id, query } => {
                debug!(
                    connection = %self.connection_id,
                    "Query traces request"
                );

                let result = self.query_traces(&query);

                Some(ServerMessage::QueryResult {
                    request_id,
                    traces: result.0,
                    total_count: result.1,
                    has_more: result.2,
                    query_time_ms: result.3,
                })
            }

            ClientMessage::Ping { timestamp } => {
                let server_timestamp = SystemTime::now()
                    .duration_since(UNIX_EPOCH)
                    .unwrap_or_default()
                    .as_millis() as u64;

                Some(ServerMessage::Pong {
                    client_timestamp: timestamp,
                    server_timestamp,
                })
            }

            ClientMessage::ClientInfo { name, version } => {
                *self.client_name.write() = Some(name);
                *self.client_version.write() = Some(version);
                None // No response needed
            }
        }
    }

    /// Check if an update matches any subscription
    pub fn should_send_update(&self, update: &TraceUpdate) -> Option<String> {
        let subs = self.subscriptions.read();

        for (id, sub) in subs.iter() {
            if sub.matches(update) {
                return Some(id.clone());
            }
        }

        None
    }

    /// Get all subscription IDs that match an update
    pub fn matching_subscriptions(&self, update: &TraceUpdate) -> Vec<String> {
        let subs = self.subscriptions.read();

        subs.iter()
            .filter(|(_, sub)| sub.matches(update))
            .map(|(id, _)| id.clone())
            .collect()
    }

    /// Record a sent message
    pub fn record_sent(&self) {
        self.messages_sent.fetch_add(1, Ordering::Relaxed);
    }

    /// Get trace details (for GetTrace request)
    fn get_trace_details(&self, trace_id: &str) -> Option<TraceDetailsResponse> {
        let store = self.trace_store.as_ref()?;

        let summary = store.get(trace_id)?;
        let spans = store.get_spans(trace_id).unwrap_or_default();

        let span_infos: Vec<SpanInfo> = spans.iter().map(|s| self.span_to_info(s)).collect();

        Some(TraceDetailsResponse {
            summary,
            spans: span_infos,
            flows: Vec::new(), // TODO: Get flow info from correlation engine
        })
    }

    /// Convert Span to SpanInfo for response
    fn span_to_info(&self, span: &Span) -> SpanInfo {
        let attributes = serde_json::to_value(&span.attributes).unwrap_or_default();

        let events: Vec<SpanEventInfo> = span
            .events
            .iter()
            .map(|e| SpanEventInfo {
                name: e.name.clone(),
                timestamp_ns: e.timestamp_ns,
                attributes: serde_json::to_value(&e.attributes).unwrap_or_default(),
            })
            .collect();

        SpanInfo {
            span_id: span.span_id.to_hex(),
            parent_span_id: span.parent_span_id.map(|id| id.to_hex()),
            name: span.name.clone(),
            kind: format!("{:?}", span.kind),
            start_time_ns: span.start_time_ns,
            end_time_ns: span.end_time_ns,
            duration_ns: span.duration_ns(),
            status: format!("{:?}", span.status),
            status_message: span.status_message.clone(),
            attributes,
            events,
        }
    }

    /// Query traces (for QueryTraces request)
    fn query_traces(&self, query: &TraceQueryRequest) -> (Vec<TraceSummary>, usize, bool, u64) {
        let store = match &self.trace_store {
            Some(s) => s,
            None => return (Vec::new(), 0, false, 0),
        };

        let start = Instant::now();

        // Build query
        let mut store_query = crate::correlation::TraceQuery::new()
            .limit(query.limit)
            .offset(query.offset);

        if let Some(ref service) = query.service {
            store_query = store_query.service(service);
        }

        if let Some(min_dur) = query.min_duration_ns {
            store_query = store_query.min_duration(min_dur);
        }

        if let Some(max_dur) = query.max_duration_ns {
            store_query = store_query.max_duration(max_dur);
        }

        if let Some(ref status) = query.status {
            store_query = store_query.status(status);
        }

        if let (Some(start_ns), Some(end_ns)) = (query.start_time_ns, query.end_time_ns) {
            store_query = store_query.time_range(start_ns, end_ns);
        }

        let result = store.query(&store_query);

        let query_time_ms = start.elapsed().as_millis() as u64;

        (
            result.traces,
            result.total_count,
            result.has_more,
            query_time_ms,
        )
    }
}

/// Connection statistics
#[derive(Debug, Clone)]
pub struct HandlerStats {
    pub connection_id: String,
    pub connected_duration_secs: u64,
    pub messages_sent: u64,
    pub messages_received: u64,
    pub subscription_count: usize,
    pub client_name: Option<String>,
    pub client_version: Option<String>,
}

impl From<&WebSocketHandler> for HandlerStats {
    fn from(handler: &WebSocketHandler) -> Self {
        Self {
            connection_id: handler.connection_id.clone(),
            connected_duration_secs: handler.connection_duration().as_secs(),
            messages_sent: handler.messages_sent.load(Ordering::Relaxed),
            messages_received: handler.messages_received.load(Ordering::Relaxed),
            subscription_count: handler.subscription_count(),
            client_name: handler.client_name.read().clone(),
            client_version: handler.client_version.read().clone(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_handler_creation() {
        let handler = WebSocketHandler::new("test-conn");
        assert_eq!(handler.connection_id(), "test-conn");
        assert_eq!(handler.subscription_count(), 0);
    }

    #[test]
    fn test_subscription_handling() {
        let handler = WebSocketHandler::new("test-conn");

        let subscribe_msg = ClientMessage::Subscribe {
            id: "sub-1".to_string(),
            subscription: Subscription::default(),
        };

        let response = handler.handle_message(subscribe_msg);
        assert!(matches!(
            response,
            Some(ServerMessage::Subscribed { success: true, .. })
        ));
        assert_eq!(handler.subscription_count(), 1);

        let unsubscribe_msg = ClientMessage::Unsubscribe {
            id: "sub-1".to_string(),
        };

        let response = handler.handle_message(unsubscribe_msg);
        assert!(matches!(response, Some(ServerMessage::Unsubscribed { .. })));
        assert_eq!(handler.subscription_count(), 0);
    }

    #[test]
    fn test_ping_pong() {
        let handler = WebSocketHandler::new("test-conn");

        let ping_msg = ClientMessage::Ping { timestamp: 12345 };

        let response = handler.handle_message(ping_msg);
        if let Some(ServerMessage::Pong {
            client_timestamp,
            server_timestamp,
        }) = response
        {
            assert_eq!(client_timestamp, 12345);
            assert!(server_timestamp > 0);
        } else {
            panic!("Expected Pong response");
        }
    }

    #[test]
    fn test_matching_subscriptions() {
        let handler = WebSocketHandler::new("test-conn");

        // Subscribe to service-a
        handler.handle_message(ClientMessage::Subscribe {
            id: "sub-1".to_string(),
            subscription: Subscription::for_service("service-a"),
        });

        // Subscribe to all
        handler.handle_message(ClientMessage::Subscribe {
            id: "sub-2".to_string(),
            subscription: Subscription::all_events(),
        });

        let update = TraceUpdate {
            trace_id: "test".to_string(),
            services: vec!["service-a".to_string()],
            status: "ok".to_string(),
            ..TraceUpdate::new("test", "packet")
        };

        let matching = handler.matching_subscriptions(&update);
        assert_eq!(matching.len(), 2);

        let other_update = TraceUpdate {
            trace_id: "test".to_string(),
            services: vec!["service-b".to_string()],
            status: "ok".to_string(),
            ..TraceUpdate::new("test", "packet")
        };

        let matching = handler.matching_subscriptions(&other_update);
        assert_eq!(matching.len(), 1); // Only sub-2 (all) should match
    }
}
