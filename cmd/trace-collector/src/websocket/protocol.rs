//! WebSocket Protocol Messages
//!
//! Defines the message format for WebSocket communication between
//! the trace collector and dashboard clients.

use serde::{Deserialize, Serialize};

use crate::correlation::TraceSummary;

/// Subscription parameters for filtering events
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Subscription {
    /// Subscribe to all events
    #[serde(default)]
    pub all: bool,
    /// Filter by event types
    #[serde(default)]
    pub event_types: Vec<String>,
    /// Filter by services
    #[serde(default)]
    pub services: Vec<String>,
    /// Filter by trace ID (for following a specific trace)
    #[serde(default)]
    pub trace_ids: Vec<String>,
    /// Minimum latency threshold (nanoseconds)
    pub min_latency_ns: Option<u64>,
    /// Only include errors
    #[serde(default)]
    pub errors_only: bool,
    /// Sample rate (0.0-1.0)
    #[serde(default = "default_sample_rate")]
    pub sample_rate: f64,
}

fn default_sample_rate() -> f64 {
    1.0
}

impl Default for Subscription {
    fn default() -> Self {
        Self {
            all: true,
            event_types: Vec::new(),
            services: Vec::new(),
            trace_ids: Vec::new(),
            min_latency_ns: None,
            errors_only: false,
            sample_rate: 1.0,
        }
    }
}

impl Subscription {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn all_events() -> Self {
        Self {
            all: true,
            ..Default::default()
        }
    }

    pub fn for_service(service: impl Into<String>) -> Self {
        Self {
            all: false,
            services: vec![service.into()],
            ..Default::default()
        }
    }

    pub fn for_trace(trace_id: impl Into<String>) -> Self {
        Self {
            all: false,
            trace_ids: vec![trace_id.into()],
            ..Default::default()
        }
    }

    pub fn errors() -> Self {
        Self {
            all: false,
            errors_only: true,
            ..Default::default()
        }
    }

    /// Check if an event matches this subscription
    pub fn matches(&self, update: &TraceUpdate) -> bool {
        if self.all && !self.errors_only {
            return true;
        }

        // Check errors_only
        if self.errors_only && update.status != "error" {
            return false;
        }

        // Check event types
        if !self.event_types.is_empty()
            && !self.event_types.contains(&update.event_type) {
                return false;
            }

        // Check services
        if !self.services.is_empty()
            && !update.services.iter().any(|s| self.services.contains(s)) {
                return false;
            }

        // Check trace IDs
        if !self.trace_ids.is_empty()
            && !self.trace_ids.contains(&update.trace_id) {
                return false;
            }

        // Check minimum latency
        if let Some(min_lat) = self.min_latency_ns {
            if let Some(lat) = update.latency_ns {
                if lat < min_lat {
                    return false;
                }
            }
        }

        // Apply sampling
        if self.sample_rate < 1.0 && fastrand::f64() > self.sample_rate {
            return false;
        }

        true
    }
}

/// Message from client to server
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum ClientMessage {
    /// Subscribe to events
    Subscribe {
        /// Subscription ID (for multiple subscriptions)
        id: String,
        /// Subscription parameters
        subscription: Subscription,
    },
    /// Unsubscribe from events
    Unsubscribe {
        /// Subscription ID
        id: String,
    },
    /// Modify existing subscription
    UpdateSubscription {
        /// Subscription ID
        id: String,
        /// New subscription parameters
        subscription: Subscription,
    },
    /// Request trace details
    GetTrace {
        /// Request ID for correlation
        request_id: String,
        /// Trace ID to fetch
        trace_id: String,
    },
    /// Query historical traces
    QueryTraces {
        /// Request ID
        request_id: String,
        /// Query parameters
        query: TraceQueryRequest,
    },
    /// Ping (keepalive)
    Ping {
        /// Timestamp for RTT measurement
        timestamp: u64,
    },
    /// Client info (for identification)
    ClientInfo {
        /// Client name/identifier
        name: String,
        /// Client version
        version: String,
    },
}

/// Query request parameters
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TraceQueryRequest {
    /// Filter by service
    pub service: Option<String>,
    /// Minimum duration (nanoseconds)
    pub min_duration_ns: Option<u64>,
    /// Maximum duration (nanoseconds)
    pub max_duration_ns: Option<u64>,
    /// Status filter
    pub status: Option<String>,
    /// Start time (Unix nanoseconds)
    pub start_time_ns: Option<u64>,
    /// End time
    pub end_time_ns: Option<u64>,
    /// Maximum results
    #[serde(default = "default_limit")]
    pub limit: usize,
    /// Offset for pagination
    #[serde(default)]
    pub offset: usize,
}

fn default_limit() -> usize {
    100
}

/// Message from server to client
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum ServerMessage {
    /// Connection acknowledged
    Connected {
        /// Server version
        version: String,
        /// Connection ID
        connection_id: String,
        /// Server capabilities
        capabilities: Vec<String>,
    },
    /// Subscription confirmed
    Subscribed {
        /// Subscription ID
        id: String,
        /// Success flag
        success: bool,
        /// Error message (if failed)
        error: Option<String>,
    },
    /// Unsubscription confirmed
    Unsubscribed {
        /// Subscription ID
        id: String,
    },
    /// Real-time trace update
    TraceUpdate {
        /// Subscription ID this relates to
        subscription_id: String,
        /// The trace update
        update: TraceUpdate,
    },
    /// Batch of trace updates (for efficiency)
    TraceUpdateBatch {
        /// Subscription ID
        subscription_id: String,
        /// Updates in this batch
        updates: Vec<TraceUpdate>,
    },
    /// Response to GetTrace
    TraceDetails {
        /// Request ID
        request_id: String,
        /// Trace data (if found)
        trace: Option<TraceDetailsResponse>,
        /// Error message (if failed)
        error: Option<String>,
    },
    /// Response to QueryTraces
    QueryResult {
        /// Request ID
        request_id: String,
        /// Matching traces
        traces: Vec<TraceSummary>,
        /// Total count
        total_count: usize,
        /// Whether more results exist
        has_more: bool,
        /// Query time (milliseconds)
        query_time_ms: u64,
    },
    /// Pong (keepalive response)
    Pong {
        /// Echo of client timestamp
        client_timestamp: u64,
        /// Server timestamp
        server_timestamp: u64,
    },
    /// Error message
    Error {
        /// Error code
        code: String,
        /// Error message
        message: String,
        /// Related request ID (if applicable)
        request_id: Option<String>,
    },
    /// Server statistics (periodic)
    Stats {
        /// Active trace count
        active_traces: usize,
        /// Events per second
        events_per_second: f64,
        /// Connected clients
        connected_clients: usize,
        /// Uptime (seconds)
        uptime_secs: u64,
    },
}

/// Real-time trace update
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TraceUpdate {
    /// Trace ID
    pub trace_id: String,
    /// Span ID (if applicable)
    pub span_id: Option<String>,
    /// Parent span ID (if applicable)
    pub parent_span_id: Option<String>,
    /// Event type
    pub event_type: String,
    /// Operation/span name
    pub operation: String,
    /// Services involved
    pub services: Vec<String>,
    /// Timestamp (nanoseconds since Unix epoch)
    pub timestamp_ns: u64,
    /// Duration (if span ended)
    pub duration_ns: Option<u64>,
    /// Latency measurement (if available)
    pub latency_ns: Option<u64>,
    /// Status (ok, error, unset)
    pub status: String,
    /// Error message (if error)
    pub error_message: Option<String>,
    /// Key attributes
    pub attributes: serde_json::Value,
    /// Source (packet-marker, flow-tracker, etc.)
    pub source: String,
    /// Is this a new trace?
    pub is_new_trace: bool,
    /// Is this the final update for the trace?
    pub is_complete: bool,
}

impl TraceUpdate {
    pub fn new(trace_id: impl Into<String>, event_type: impl Into<String>) -> Self {
        Self {
            trace_id: trace_id.into(),
            span_id: None,
            parent_span_id: None,
            event_type: event_type.into(),
            operation: String::new(),
            services: Vec::new(),
            timestamp_ns: std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap_or_default()
                .as_nanos() as u64,
            duration_ns: None,
            latency_ns: None,
            status: "unset".to_string(),
            error_message: None,
            attributes: serde_json::Value::Object(serde_json::Map::new()),
            source: "trace-collector".to_string(),
            is_new_trace: false,
            is_complete: false,
        }
    }

    pub fn from_summary(summary: &TraceSummary) -> Self {
        Self {
            trace_id: summary.trace_id.clone(),
            span_id: None,
            parent_span_id: None,
            event_type: "trace".to_string(),
            operation: summary.root_operation.clone(),
            services: summary.services.clone(),
            timestamp_ns: summary.start_time_ns,
            duration_ns: Some(summary.duration_ns),
            latency_ns: summary.avg_latency_ns,
            status: summary.status.clone(),
            error_message: None,
            attributes: serde_json::json!({
                "span_count": summary.span_count,
                "packet_count": summary.packet_count,
                "error_count": summary.error_count,
            }),
            source: "trace-collector".to_string(),
            is_new_trace: false,
            is_complete: true,
        }
    }
}

/// Detailed trace response (for GetTrace)
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TraceDetailsResponse {
    /// Trace summary
    pub summary: TraceSummary,
    /// All spans in the trace
    pub spans: Vec<SpanInfo>,
    /// Flow information
    pub flows: Vec<FlowInfo>,
}

/// Span information for trace details
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SpanInfo {
    /// Span ID
    pub span_id: String,
    /// Parent span ID
    pub parent_span_id: Option<String>,
    /// Operation name
    pub name: String,
    /// Kind (internal, server, client)
    pub kind: String,
    /// Start timestamp
    pub start_time_ns: u64,
    /// End timestamp
    pub end_time_ns: Option<u64>,
    /// Duration
    pub duration_ns: Option<u64>,
    /// Status
    pub status: String,
    /// Status message
    pub status_message: Option<String>,
    /// Attributes
    pub attributes: serde_json::Value,
    /// Events
    pub events: Vec<SpanEventInfo>,
}

/// Span event information
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SpanEventInfo {
    pub name: String,
    pub timestamp_ns: u64,
    pub attributes: serde_json::Value,
}

/// Flow information
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FlowInfo {
    pub src_ip: String,
    pub dst_ip: String,
    pub src_port: u16,
    pub dst_port: u16,
    pub protocol: String,
    pub packets: u64,
    pub bytes: u64,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_subscription_default() {
        let sub = Subscription::default();
        assert!(sub.all);
        assert!(!sub.errors_only);
        assert_eq!(sub.sample_rate, 1.0);
    }

    #[test]
    fn test_subscription_matching() {
        let sub = Subscription::for_service("my-service");

        let matching_update = TraceUpdate {
            trace_id: "test".to_string(),
            services: vec!["my-service".to_string()],
            status: "ok".to_string(),
            ..TraceUpdate::new("test", "packet")
        };

        let non_matching_update = TraceUpdate {
            trace_id: "test".to_string(),
            services: vec!["other-service".to_string()],
            status: "ok".to_string(),
            ..TraceUpdate::new("test", "packet")
        };

        assert!(sub.matches(&matching_update));
        assert!(!sub.matches(&non_matching_update));
    }

    #[test]
    fn test_subscription_errors_only() {
        let sub = Subscription::errors();

        let error_update = TraceUpdate {
            status: "error".to_string(),
            ..TraceUpdate::new("test", "packet")
        };

        let ok_update = TraceUpdate {
            status: "ok".to_string(),
            ..TraceUpdate::new("test", "packet")
        };

        assert!(sub.matches(&error_update));
        assert!(!sub.matches(&ok_update));
    }

    #[test]
    fn test_client_message_serialization() {
        let msg = ClientMessage::Subscribe {
            id: "sub-1".to_string(),
            subscription: Subscription::default(),
        };

        let json = serde_json::to_string(&msg).unwrap();
        assert!(json.contains("subscribe"));

        let parsed: ClientMessage = serde_json::from_str(&json).unwrap();
        if let ClientMessage::Subscribe { id, .. } = parsed {
            assert_eq!(id, "sub-1");
        } else {
            panic!("Wrong message type");
        }
    }

    #[test]
    fn test_server_message_serialization() {
        let msg = ServerMessage::Connected {
            version: "0.1.0".to_string(),
            connection_id: "conn-123".to_string(),
            capabilities: vec!["streaming".to_string()],
        };

        let json = serde_json::to_string(&msg).unwrap();
        assert!(json.contains("connected"));
    }
}
