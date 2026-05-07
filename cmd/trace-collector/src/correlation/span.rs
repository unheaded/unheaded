//! Span representation for distributed tracing.
//!
//! A span represents a single operation within a trace.
//! Spans can be nested to form a tree structure.

use std::collections::HashMap;
use std::time::Duration;

use serde::{Deserialize, Serialize};

use super::{SpanId, TraceId};

/// Span kind (role in the trace)
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum SpanKind {
    /// Internal operation (default)
    Internal,
    /// Server receiving a request
    Server,
    /// Client making a request
    Client,
    /// Producer sending a message
    Producer,
    /// Consumer receiving a message
    Consumer,
}

impl Default for SpanKind {
    fn default() -> Self {
        Self::Internal
    }
}

/// Span status
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum SpanStatus {
    /// Unset (default)
    Unset,
    /// Operation completed successfully
    Ok,
    /// Operation failed
    Error,
}

impl Default for SpanStatus {
    fn default() -> Self {
        Self::Unset
    }
}

/// A span event (point-in-time occurrence within a span)
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SpanEvent {
    /// Event name
    pub name: String,
    /// Timestamp (nanoseconds since Unix epoch)
    pub timestamp_ns: u64,
    /// Event attributes
    pub attributes: HashMap<String, AttributeValue>,
}

impl SpanEvent {
    pub fn new(name: impl Into<String>, timestamp_ns: u64) -> Self {
        Self {
            name: name.into(),
            timestamp_ns,
            attributes: HashMap::new(),
        }
    }

    pub fn with_attribute(
        mut self,
        key: impl Into<String>,
        value: impl Into<AttributeValue>,
    ) -> Self {
        self.attributes.insert(key.into(), value.into());
        self
    }
}

/// A span link (reference to another span)
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SpanLink {
    /// Trace ID of the linked span
    pub trace_id: TraceId,
    /// Span ID of the linked span
    pub span_id: SpanId,
    /// Link attributes
    pub attributes: HashMap<String, AttributeValue>,
}

impl SpanLink {
    pub fn new(trace_id: TraceId, span_id: SpanId) -> Self {
        Self {
            trace_id,
            span_id,
            attributes: HashMap::new(),
        }
    }
}

/// Attribute value (typed)
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(untagged)]
pub enum AttributeValue {
    String(String),
    Int(i64),
    Float(f64),
    Bool(bool),
    StringArray(Vec<String>),
    IntArray(Vec<i64>),
}

impl From<String> for AttributeValue {
    fn from(s: String) -> Self {
        Self::String(s)
    }
}

impl From<&str> for AttributeValue {
    fn from(s: &str) -> Self {
        Self::String(s.to_string())
    }
}

impl From<i64> for AttributeValue {
    fn from(i: i64) -> Self {
        Self::Int(i)
    }
}

impl From<i32> for AttributeValue {
    fn from(i: i32) -> Self {
        Self::Int(i as i64)
    }
}

impl From<u32> for AttributeValue {
    fn from(i: u32) -> Self {
        Self::Int(i as i64)
    }
}

impl From<u64> for AttributeValue {
    fn from(i: u64) -> Self {
        Self::Int(i as i64)
    }
}

impl From<f64> for AttributeValue {
    fn from(f: f64) -> Self {
        Self::Float(f)
    }
}

impl From<bool> for AttributeValue {
    fn from(b: bool) -> Self {
        Self::Bool(b)
    }
}

/// Resource attributes (service info)
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Resource {
    /// Service name
    pub service_name: String,
    /// Service namespace
    pub service_namespace: Option<String>,
    /// Service version
    pub service_version: Option<String>,
    /// Host name
    pub host_name: Option<String>,
    /// Process ID
    pub pid: Option<u32>,
    /// Additional attributes
    pub attributes: HashMap<String, AttributeValue>,
}

impl Resource {
    pub fn new(service_name: impl Into<String>) -> Self {
        Self {
            service_name: service_name.into(),
            service_namespace: None,
            service_version: None,
            host_name: None,
            pid: None,
            attributes: HashMap::new(),
        }
    }

    pub fn with_attribute(
        mut self,
        key: impl Into<String>,
        value: impl Into<AttributeValue>,
    ) -> Self {
        self.attributes.insert(key.into(), value.into());
        self
    }
}

/// A span in a distributed trace
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Span {
    /// Trace ID this span belongs to
    pub trace_id: TraceId,
    /// Unique span ID
    pub span_id: SpanId,
    /// Parent span ID (if not root)
    pub parent_span_id: Option<SpanId>,
    /// Span name (operation name)
    pub name: String,
    /// Span kind
    pub kind: SpanKind,
    /// Start timestamp (nanoseconds since Unix epoch)
    pub start_time_ns: u64,
    /// End timestamp (nanoseconds since Unix epoch)
    pub end_time_ns: Option<u64>,
    /// Span status
    pub status: SpanStatus,
    /// Status message (for errors)
    pub status_message: Option<String>,
    /// Span attributes
    pub attributes: HashMap<String, AttributeValue>,
    /// Span events
    pub events: Vec<SpanEvent>,
    /// Span links
    pub links: Vec<SpanLink>,
    /// Resource (service info)
    pub resource: Option<Resource>,
    /// Instrumentation scope (library info)
    pub scope_name: Option<String>,
    /// Scope version
    pub scope_version: Option<String>,
}

impl Span {
    /// Create a new span
    pub fn new(
        trace_id: TraceId,
        span_id: SpanId,
        name: impl Into<String>,
        start_time_ns: u64,
    ) -> Self {
        Self {
            trace_id,
            span_id,
            parent_span_id: None,
            name: name.into(),
            kind: SpanKind::Internal,
            start_time_ns,
            end_time_ns: None,
            status: SpanStatus::Unset,
            status_message: None,
            attributes: HashMap::new(),
            events: Vec::new(),
            links: Vec::new(),
            resource: None,
            scope_name: None,
            scope_version: None,
        }
    }

    /// Create a new root span
    pub fn root(name: impl Into<String>, start_time_ns: u64) -> Self {
        Self::new(TraceId::generate(), SpanId::generate(), name, start_time_ns)
    }

    /// Create a child span
    pub fn child(&self, name: impl Into<String>, start_time_ns: u64) -> Self {
        let mut span = Self::new(self.trace_id, SpanId::generate(), name, start_time_ns);
        span.parent_span_id = Some(self.span_id);
        span
    }

    /// Set the parent span
    pub fn with_parent(mut self, parent_span_id: SpanId) -> Self {
        self.parent_span_id = Some(parent_span_id);
        self
    }

    /// Set the span kind
    pub fn with_kind(mut self, kind: SpanKind) -> Self {
        self.kind = kind;
        self
    }

    /// Add an attribute
    pub fn with_attribute(
        mut self,
        key: impl Into<String>,
        value: impl Into<AttributeValue>,
    ) -> Self {
        self.attributes.insert(key.into(), value.into());
        self
    }

    /// Set the resource
    pub fn with_resource(mut self, resource: Resource) -> Self {
        self.resource = Some(resource);
        self
    }

    /// Add an event
    pub fn add_event(&mut self, event: SpanEvent) {
        self.events.push(event);
    }

    /// Add a link
    pub fn add_link(&mut self, link: SpanLink) {
        self.links.push(link);
    }

    /// End the span
    pub fn end(&mut self, end_time_ns: u64) {
        self.end_time_ns = Some(end_time_ns);
    }

    /// Set status to OK
    pub fn set_ok(&mut self) {
        self.status = SpanStatus::Ok;
    }

    /// Set status to Error with message
    pub fn set_error(&mut self, message: impl Into<String>) {
        self.status = SpanStatus::Error;
        self.status_message = Some(message.into());
    }

    /// Check if this is a root span
    pub fn is_root(&self) -> bool {
        self.parent_span_id.is_none()
    }

    /// Check if this span has ended
    pub fn is_ended(&self) -> bool {
        self.end_time_ns.is_some()
    }

    /// Get the duration (if ended)
    pub fn duration(&self) -> Option<Duration> {
        self.end_time_ns
            .map(|end| Duration::from_nanos(end.saturating_sub(self.start_time_ns)))
    }

    /// Get duration in nanoseconds
    pub fn duration_ns(&self) -> Option<u64> {
        self.end_time_ns
            .map(|end| end.saturating_sub(self.start_time_ns))
    }
}

/// Builder for creating spans with network context
pub struct SpanBuilder {
    trace_id: TraceId,
    span_id: SpanId,
    parent_span_id: Option<SpanId>,
    name: String,
    kind: SpanKind,
    start_time_ns: u64,
    attributes: HashMap<String, AttributeValue>,
    resource: Option<Resource>,
}

impl SpanBuilder {
    pub fn new(name: impl Into<String>, start_time_ns: u64) -> Self {
        Self {
            trace_id: TraceId::generate(),
            span_id: SpanId::generate(),
            parent_span_id: None,
            name: name.into(),
            kind: SpanKind::Internal,
            start_time_ns,
            attributes: HashMap::new(),
            resource: None,
        }
    }

    pub fn trace_id(mut self, trace_id: TraceId) -> Self {
        self.trace_id = trace_id;
        self
    }

    pub fn span_id(mut self, span_id: SpanId) -> Self {
        self.span_id = span_id;
        self
    }

    pub fn parent(mut self, parent_span_id: SpanId) -> Self {
        self.parent_span_id = Some(parent_span_id);
        self
    }

    pub fn kind(mut self, kind: SpanKind) -> Self {
        self.kind = kind;
        self
    }

    pub fn attribute(mut self, key: impl Into<String>, value: impl Into<AttributeValue>) -> Self {
        self.attributes.insert(key.into(), value.into());
        self
    }

    pub fn resource(mut self, resource: Resource) -> Self {
        self.resource = Some(resource);
        self
    }

    /// Add network-specific attributes
    pub fn network(
        self,
        src_ip: &str,
        dst_ip: &str,
        src_port: u16,
        dst_port: u16,
        protocol: &str,
    ) -> Self {
        self.attribute("net.peer.ip", src_ip)
            .attribute("net.host.ip", dst_ip)
            .attribute("net.peer.port", src_port as i64)
            .attribute("net.host.port", dst_port as i64)
            .attribute("net.transport", protocol)
    }

    /// Add HTTP-specific attributes
    pub fn http(self, method: &str, url: &str, status_code: i32) -> Self {
        self.attribute("http.method", method)
            .attribute("http.url", url)
            .attribute("http.status_code", status_code as i64)
    }

    pub fn build(self) -> Span {
        let mut span = Span::new(self.trace_id, self.span_id, self.name, self.start_time_ns);
        span.parent_span_id = self.parent_span_id;
        span.kind = self.kind;
        span.attributes = self.attributes;
        span.resource = self.resource;
        span
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_span_creation() {
        let span = Span::root("test-operation", 1000);
        assert!(!span.trace_id.is_zero());
        assert!(!span.span_id.is_zero());
        assert!(span.parent_span_id.is_none());
        assert_eq!(span.name, "test-operation");
        assert_eq!(span.start_time_ns, 1000);
    }

    #[test]
    fn test_child_span() {
        let parent = Span::root("parent", 1000);
        let child = parent.child("child", 2000);

        assert_eq!(parent.trace_id, child.trace_id);
        assert_eq!(child.parent_span_id, Some(parent.span_id));
    }

    #[test]
    fn test_span_duration() {
        let mut span = Span::root("test", 1000);
        assert!(span.duration().is_none());

        span.end(5000);
        assert_eq!(span.duration(), Some(Duration::from_nanos(4000)));
    }

    #[test]
    fn test_span_builder() {
        let span = SpanBuilder::new("http-request", 1000)
            .kind(SpanKind::Server)
            .attribute("http.method", "GET")
            .attribute("http.status_code", 200i64)
            .build();

        assert_eq!(span.kind, SpanKind::Server);
        assert!(span.attributes.contains_key("http.method"));
    }

    #[test]
    fn test_span_events() {
        let mut span = Span::root("test", 1000);
        span.add_event(SpanEvent::new("processing-started", 2000).with_attribute("items", 10i64));

        assert_eq!(span.events.len(), 1);
        assert_eq!(span.events[0].name, "processing-started");
    }

    #[test]
    fn test_attribute_value() {
        let s: AttributeValue = "test".into();
        assert!(matches!(s, AttributeValue::String(_)));

        let i: AttributeValue = 42i64.into();
        assert!(matches!(i, AttributeValue::Int(42)));

        let f: AttributeValue = 3.14f64.into();
        assert!(matches!(f, AttributeValue::Float(_)));

        let b: AttributeValue = true.into();
        assert!(matches!(b, AttributeValue::Bool(true)));
    }
}
