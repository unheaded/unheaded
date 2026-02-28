//! Trace Correlation Engine
//!
//! Correlates events from multiple eBPF sources into coherent distributed traces.
//! Maintains active trace state and cross-service associations.

use std::collections::{HashMap, VecDeque};
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use std::time::{Duration, Instant};

use parking_lot::RwLock;
use serde::{Deserialize, Serialize};
use tokio::sync::broadcast;
use tracing::{debug, info, warn};

use super::{FlowKey, Span, SpanBuilder, SpanId, SpanKind, SpanStatus, TraceId};
use crate::events::{Event, EventData, EventType};

/// Configuration for the correlation engine
#[derive(Debug, Clone)]
pub struct CorrelationConfig {
    /// Maximum number of active traces to track
    pub max_active_traces: usize,
    /// Trace expiration timeout (no events for this long = expired)
    pub trace_timeout: Duration,
    /// Flow-to-trace association timeout
    pub flow_timeout: Duration,
    /// Enable cross-service correlation
    pub cross_service_correlation: bool,
    /// Sample rate (1.0 = all traces, 0.1 = 10%)
    pub sample_rate: f64,
    /// Maximum spans per trace
    pub max_spans_per_trace: usize,
    /// Cleanup interval
    pub cleanup_interval: Duration,
}

impl Default for CorrelationConfig {
    fn default() -> Self {
        Self {
            max_active_traces: 10000,
            trace_timeout: Duration::from_secs(60),
            flow_timeout: Duration::from_secs(30),
            cross_service_correlation: true,
            sample_rate: 1.0,
            max_spans_per_trace: 100,
            cleanup_interval: Duration::from_secs(10),
        }
    }
}

/// Statistics for the correlation engine
#[derive(Debug, Default)]
pub struct CorrelationStats {
    /// Total events processed
    pub events_processed: AtomicU64,
    /// Traces created
    pub traces_created: AtomicU64,
    /// Traces completed
    pub traces_completed: AtomicU64,
    /// Traces expired
    pub traces_expired: AtomicU64,
    /// Spans created
    pub spans_created: AtomicU64,
    /// Flow associations created
    pub flow_associations: AtomicU64,
    /// Cross-service correlations
    pub cross_service_correlations: AtomicU64,
    /// Events without trace context
    pub orphan_events: AtomicU64,
    /// Events sampled out
    pub sampled_out: AtomicU64,
}

/// Context for an active trace
#[derive(Debug, Clone)]
pub struct TraceContext {
    /// Trace ID
    pub trace_id: TraceId,
    /// Root span ID
    pub root_span_id: SpanId,
    /// All spans in this trace
    pub spans: Vec<Span>,
    /// Services involved in this trace
    pub services: Vec<String>,
    /// Start timestamp
    pub start_time_ns: u64,
    /// Last activity timestamp
    pub last_activity_ns: u64,
    /// Creation instant (for timeout tracking)
    pub created_at: Instant,
    /// Last activity instant
    pub last_activity_at: Instant,
    /// Total packet count
    pub packet_count: u64,
    /// Total latency samples
    pub latency_samples: Vec<u64>,
    /// Associated flow keys
    pub flows: Vec<FlowKey>,
}

impl TraceContext {
    /// Create a new trace context
    pub fn new(trace_id: TraceId, root_span_id: SpanId, start_time_ns: u64) -> Self {
        let now = Instant::now();
        Self {
            trace_id,
            root_span_id,
            spans: Vec::new(),
            services: Vec::new(),
            start_time_ns,
            last_activity_ns: start_time_ns,
            created_at: now,
            last_activity_at: now,
            packet_count: 0,
            latency_samples: Vec::new(),
            flows: Vec::new(),
        }
    }

    /// Update last activity
    pub fn touch(&mut self, timestamp_ns: u64) {
        self.last_activity_ns = timestamp_ns;
        self.last_activity_at = Instant::now();
    }

    /// Add a span to this trace
    pub fn add_span(&mut self, span: Span) {
        self.touch(span.start_time_ns);
        self.spans.push(span);
    }

    /// Add a service if not already present
    pub fn add_service(&mut self, service: &str) {
        if !self.services.contains(&service.to_string()) {
            self.services.push(service.to_string());
        }
    }

    /// Add a flow association
    pub fn add_flow(&mut self, flow: FlowKey) {
        if !self.flows.contains(&flow) {
            self.flows.push(flow);
        }
    }

    /// Record a latency sample
    pub fn record_latency(&mut self, latency_ns: u64) {
        self.latency_samples.push(latency_ns);
    }

    /// Get trace duration so far
    pub fn duration_ns(&self) -> u64 {
        self.last_activity_ns.saturating_sub(self.start_time_ns)
    }

    /// Check if trace has timed out
    pub fn is_expired(&self, timeout: Duration) -> bool {
        self.last_activity_at.elapsed() > timeout
    }

    /// Get average latency (if samples exist)
    pub fn avg_latency_ns(&self) -> Option<u64> {
        if self.latency_samples.is_empty() {
            None
        } else {
            Some(self.latency_samples.iter().sum::<u64>() / self.latency_samples.len() as u64)
        }
    }

    /// Get P95 latency
    pub fn p95_latency_ns(&self) -> Option<u64> {
        if self.latency_samples.is_empty() {
            return None;
        }
        let mut sorted = self.latency_samples.clone();
        sorted.sort_unstable();
        let idx = (sorted.len() as f64 * 0.95) as usize;
        sorted.get(idx.min(sorted.len() - 1)).copied()
    }
}

/// Summary of a completed trace
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TraceSummary {
    /// Trace ID
    pub trace_id: String,
    /// Root span name
    pub root_operation: String,
    /// Services involved
    pub services: Vec<String>,
    /// Total span count
    pub span_count: usize,
    /// Duration in nanoseconds
    pub duration_ns: u64,
    /// Start timestamp
    pub start_time_ns: u64,
    /// End timestamp
    pub end_time_ns: u64,
    /// Status (derived from root span)
    pub status: String,
    /// Average latency
    pub avg_latency_ns: Option<u64>,
    /// P95 latency
    pub p95_latency_ns: Option<u64>,
    /// Packet count
    pub packet_count: u64,
    /// Error count
    pub error_count: usize,
}

impl TraceSummary {
    pub fn from_context(ctx: &TraceContext) -> Self {
        let root_span = ctx.spans.first();
        let root_operation = root_span.map(|s| s.name.clone()).unwrap_or_default();
        let status = root_span
            .map(|s| match s.status {
                SpanStatus::Ok => "ok",
                SpanStatus::Error => "error",
                SpanStatus::Unset => "unset",
            })
            .unwrap_or("unknown")
            .to_string();

        let error_count = ctx
            .spans
            .iter()
            .filter(|s| s.status == SpanStatus::Error)
            .count();

        Self {
            trace_id: ctx.trace_id.to_hex(),
            root_operation,
            services: ctx.services.clone(),
            span_count: ctx.spans.len(),
            duration_ns: ctx.duration_ns(),
            start_time_ns: ctx.start_time_ns,
            end_time_ns: ctx.last_activity_ns,
            status,
            avg_latency_ns: ctx.avg_latency_ns(),
            p95_latency_ns: ctx.p95_latency_ns(),
            packet_count: ctx.packet_count,
            error_count,
        }
    }
}

/// The correlation engine
pub struct CorrelationEngine {
    /// Configuration
    config: CorrelationConfig,
    /// Active traces by trace ID
    traces: RwLock<HashMap<TraceId, TraceContext>>,
    /// Flow-to-trace associations
    flow_to_trace: RwLock<HashMap<FlowKey, TraceId>>,
    /// Statistics
    stats: Arc<CorrelationStats>,
    /// Broadcast channel for completed traces
    completed_tx: broadcast::Sender<TraceSummary>,
    /// Recent completed traces (for query)
    recent_completed: RwLock<VecDeque<TraceSummary>>,
}

impl CorrelationEngine {
    /// Create a new correlation engine
    pub fn new(config: CorrelationConfig) -> Self {
        let (completed_tx, _) = broadcast::channel(1000);

        Self {
            config,
            traces: RwLock::new(HashMap::new()),
            flow_to_trace: RwLock::new(HashMap::new()),
            stats: Arc::new(CorrelationStats::default()),
            completed_tx,
            recent_completed: RwLock::new(VecDeque::with_capacity(100)),
        }
    }

    /// Get statistics
    pub fn stats(&self) -> &Arc<CorrelationStats> {
        &self.stats
    }

    /// Get active trace count
    pub fn active_trace_count(&self) -> usize {
        self.traces.read().len()
    }

    /// Subscribe to completed traces
    pub fn subscribe_completed(&self) -> broadcast::Receiver<TraceSummary> {
        self.completed_tx.subscribe()
    }

    /// Process an event and update correlation state
    pub fn process_event(&self, event: &Event) -> Option<TraceId> {
        self.stats.events_processed.fetch_add(1, Ordering::Relaxed);

        // Apply sampling
        if self.config.sample_rate < 1.0 && fastrand::f64() > self.config.sample_rate {
            self.stats.sampled_out.fetch_add(1, Ordering::Relaxed);
            return None;
        }

        match &event.data {
            EventData::Packet(packet) => self.process_packet_event(event, packet),
            EventData::Latency(latency) => self.process_latency_event(event, latency),
            EventData::Syscall(syscall) => self.process_syscall_event(event, syscall),
            EventData::Raw(_) => {
                self.stats.orphan_events.fetch_add(1, Ordering::Relaxed);
                None
            }
        }
    }

    /// Process a packet event
    fn process_packet_event(
        &self,
        event: &Event,
        packet: &crate::events::PacketEvent,
    ) -> Option<TraceId> {
        // Extract flow key
        let flow_key = self.flow_key_from_packet(packet);

        // Check for existing trace association via flow
        let existing_trace = {
            let flow_map = self.flow_to_trace.read();
            flow_map.get(&flow_key.normalized()).copied()
        };

        let trace_id = if let Some(trace_id) = existing_trace {
            // Update existing trace
            self.update_trace(trace_id, event, Some(flow_key));
            trace_id
        } else {
            // Create new trace for this flow
            let trace_id = TraceId::generate();
            self.create_trace(trace_id, event, Some(flow_key));

            // Associate flow with trace
            {
                let mut flow_map = self.flow_to_trace.write();
                flow_map.insert(flow_key.normalized(), trace_id);
                self.stats.flow_associations.fetch_add(1, Ordering::Relaxed);
            }

            trace_id
        };

        Some(trace_id)
    }

    /// Process a latency event
    fn process_latency_event(
        &self,
        event: &Event,
        latency: &crate::events::LatencyEvent,
    ) -> Option<TraceId> {
        // Try to find trace by process context
        // For now, record latency for the most recent trace from this PID
        let matching_trace_id = {
            let traces = self.traces.read();
            traces.iter().find_map(|(trace_id, ctx)| {
                if ctx.spans.iter().any(|s| {
                    s.attributes
                        .get("process.pid")
                        .map(|v| matches!(v, crate::correlation::span::AttributeValue::Int(pid) if *pid as u32 == event.pid))
                        .unwrap_or(false)
                }) {
                    Some(*trace_id)
                } else {
                    None
                }
            })
        };

        if let Some(trace_id) = matching_trace_id {
            // Update trace with latency
            if let Some(ctx) = self.traces.write().get_mut(&trace_id) {
                ctx.record_latency(latency.latency_ns);
                ctx.touch(event.timestamp_ns);
            }
            return Some(trace_id);
        }

        // No matching trace found
        self.stats.orphan_events.fetch_add(1, Ordering::Relaxed);
        None
    }

    /// Process a syscall event
    fn process_syscall_event(
        &self,
        event: &Event,
        syscall: &crate::events::SyscallEvent,
    ) -> Option<TraceId> {
        // Syscall events are correlated by PID
        let matching_trace_id = {
            let traces = self.traces.read();
            traces.iter().find_map(|(trace_id, ctx)| {
                if ctx.spans.iter().any(|s| {
                    s.attributes
                        .get("process.pid")
                        .map(|v| matches!(v, crate::correlation::span::AttributeValue::Int(pid) if *pid as u32 == event.pid))
                        .unwrap_or(false)
                }) {
                    Some(*trace_id)
                } else {
                    None
                }
            })
        };

        if let Some(trace_id) = matching_trace_id {
            // Add syscall event to trace
            if let Some(ctx) = self.traces.write().get_mut(&trace_id) {
                let span = SpanBuilder::new(
                    format!("syscall:{}", syscall.syscall_nr),
                    event.timestamp_ns,
                )
                .trace_id(trace_id)
                .kind(SpanKind::Internal)
                .attribute("syscall.nr", syscall.syscall_nr)
                .attribute("process.pid", event.pid as i64)
                .attribute("process.command", event.comm.clone())
                .build();

                ctx.add_span(span);
                self.stats.spans_created.fetch_add(1, Ordering::Relaxed);
            }
            return Some(trace_id);
        }

        self.stats.orphan_events.fetch_add(1, Ordering::Relaxed);
        None
    }

    /// Create a new trace
    fn create_trace(&self, trace_id: TraceId, event: &Event, flow: Option<FlowKey>) {
        // Check capacity
        {
            let traces = self.traces.read();
            if traces.len() >= self.config.max_active_traces {
                warn!("Maximum active traces reached, dropping oldest");
                drop(traces);
                self.evict_oldest_trace();
            }
        }

        let root_span_id = SpanId::generate();
        let mut ctx = TraceContext::new(trace_id, root_span_id, event.timestamp_ns);

        // Create root span based on event type
        let span_name = match event.event_type {
            EventType::Packet => "network.request".to_string(),
            EventType::Syscall => format!("syscall:{}", event.comm),
            EventType::Latency => "latency.probe".to_string(),
            _ => "unknown".to_string(),
        };

        let span = SpanBuilder::new(&span_name, event.timestamp_ns)
            .trace_id(trace_id)
            .span_id(root_span_id)
            .kind(SpanKind::Server)
            .attribute("process.pid", event.pid as i64)
            .attribute("process.tid", event.tid as i64)
            .attribute("process.command", event.comm.clone())
            .build();

        ctx.add_span(span);
        ctx.add_service(&event.comm);

        if let Some(flow) = flow {
            ctx.add_flow(flow);
        }

        {
            let mut traces = self.traces.write();
            traces.insert(trace_id, ctx);
        }

        self.stats.traces_created.fetch_add(1, Ordering::Relaxed);
        self.stats.spans_created.fetch_add(1, Ordering::Relaxed);

        debug!(trace_id = %trace_id, "Created new trace");
    }

    /// Update an existing trace
    fn update_trace(&self, trace_id: TraceId, event: &Event, flow: Option<FlowKey>) {
        let mut traces = self.traces.write();

        if let Some(ctx) = traces.get_mut(&trace_id) {
            ctx.touch(event.timestamp_ns);
            ctx.packet_count += 1;
            ctx.add_service(&event.comm);

            if let Some(flow) = flow {
                ctx.add_flow(flow);
            }

            // Check span limit
            if ctx.spans.len() < self.config.max_spans_per_trace {
                // Add a span for this event
                let span = SpanBuilder::new(
                    format!("{}:{}", event.event_type as u32, event.comm),
                    event.timestamp_ns,
                )
                .trace_id(trace_id)
                .parent(ctx.root_span_id)
                .kind(SpanKind::Internal)
                .attribute("process.pid", event.pid as i64)
                .attribute("cpu", event.cpu as i64)
                .build();

                ctx.add_span(span);
                self.stats.spans_created.fetch_add(1, Ordering::Relaxed);
            }
        }
    }

    /// Evict the oldest trace
    fn evict_oldest_trace(&self) {
        let mut traces = self.traces.write();

        // Find oldest by created_at
        let oldest = traces
            .iter()
            .min_by_key(|(_, ctx)| ctx.created_at)
            .map(|(id, _)| *id);

        if let Some(trace_id) = oldest {
            if let Some(ctx) = traces.remove(&trace_id) {
                self.stats.traces_expired.fetch_add(1, Ordering::Relaxed);

                // Send completion notification
                let summary = TraceSummary::from_context(&ctx);
                let _ = self.completed_tx.send(summary.clone());

                // Add to recent completed
                let mut recent = self.recent_completed.write();
                if recent.len() >= 100 {
                    recent.pop_front();
                }
                recent.push_back(summary);
            }
        }
    }

    /// Clean up expired traces
    pub fn cleanup_expired(&self) {
        let mut expired = Vec::new();

        {
            let traces = self.traces.read();
            for (id, ctx) in traces.iter() {
                if ctx.is_expired(self.config.trace_timeout) {
                    expired.push((*id, ctx.clone()));
                }
            }
        }

        if !expired.is_empty() {
            let mut traces = self.traces.write();
            let mut flow_map = self.flow_to_trace.write();

            for (trace_id, ctx) in expired {
                traces.remove(&trace_id);

                // Remove flow associations
                for flow in &ctx.flows {
                    flow_map.remove(&flow.normalized());
                }

                self.stats.traces_expired.fetch_add(1, Ordering::Relaxed);

                // Send completion notification
                let summary = TraceSummary::from_context(&ctx);
                let _ = self.completed_tx.send(summary.clone());

                // Add to recent completed
                let mut recent = self.recent_completed.write();
                if recent.len() >= 100 {
                    recent.pop_front();
                }
                recent.push_back(summary);

                debug!(trace_id = %trace_id, "Trace expired");
            }
        }

        // Also clean up expired flow associations
        let mut expired_flows = Vec::new();
        {
            let traces = self.traces.read();
            let flow_map = self.flow_to_trace.read();

            for (flow, trace_id) in flow_map.iter() {
                if !traces.contains_key(trace_id) {
                    expired_flows.push(*flow);
                }
            }
        }

        if !expired_flows.is_empty() {
            let mut flow_map = self.flow_to_trace.write();
            for flow in expired_flows {
                flow_map.remove(&flow);
            }
        }
    }

    /// Get a trace by ID
    pub fn get_trace(&self, trace_id: TraceId) -> Option<TraceContext> {
        self.traces.read().get(&trace_id).cloned()
    }

    /// Get recent completed trace summaries
    pub fn recent_traces(&self) -> Vec<TraceSummary> {
        self.recent_completed.read().iter().cloned().collect()
    }

    /// Get all active trace IDs
    pub fn active_trace_ids(&self) -> Vec<TraceId> {
        self.traces.read().keys().copied().collect()
    }

    /// Associate a trace ID with a flow
    pub fn associate_flow(&self, flow: FlowKey, trace_id: TraceId) {
        let mut flow_map = self.flow_to_trace.write();
        flow_map.insert(flow.normalized(), trace_id);
        self.stats.flow_associations.fetch_add(1, Ordering::Relaxed);

        if let Some(ctx) = self.traces.write().get_mut(&trace_id) {
            ctx.add_flow(flow);
        }
    }

    /// Look up trace by flow
    pub fn trace_by_flow(&self, flow: &FlowKey) -> Option<TraceId> {
        self.flow_to_trace.read().get(&flow.normalized()).copied()
    }

    /// Helper to create flow key from packet event
    fn flow_key_from_packet(&self, packet: &crate::events::PacketEvent) -> FlowKey {
        let src_addr = match packet.src_ip {
            std::net::IpAddr::V4(ip) => u32::from(ip),
            std::net::IpAddr::V6(_) => 0, // TODO: Handle IPv6
        };

        let dst_addr = match packet.dst_ip {
            std::net::IpAddr::V4(ip) => u32::from(ip),
            std::net::IpAddr::V6(_) => 0,
        };

        let protocol = match packet.protocol {
            crate::events::packet::IpProtocol::Tcp => 6,
            crate::events::packet::IpProtocol::Udp => 17,
            crate::events::packet::IpProtocol::Icmp => 1,
            crate::events::packet::IpProtocol::Icmpv6 => 58,
            crate::events::packet::IpProtocol::Other(p) => p,
        };

        FlowKey::new(src_addr, dst_addr, packet.src_port, packet.dst_port, protocol)
    }

    /// Run the correlation engine cleanup task
    pub async fn run_cleanup(&self, mut shutdown: broadcast::Receiver<()>) {
        info!("Correlation engine cleanup task started");

        loop {
            tokio::select! {
                _ = tokio::time::sleep(self.config.cleanup_interval) => {
                    self.cleanup_expired();
                }
                _ = shutdown.recv() => {
                    info!("Correlation engine cleanup task stopping");
                    break;
                }
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::events::{EventData, PacketEvent};
    use std::net::{IpAddr, Ipv4Addr};

    fn create_test_packet_event() -> Event {
        Event {
            event_type: EventType::Packet,
            cpu: 0,
            timestamp_ns: 1000,
            pid: 1234,
            tid: 1234,
            comm: "test-process".to_string(),
            data: EventData::Packet(PacketEvent {
                pid: 1234,
                tid: 1234,
                comm: "test-process".to_string(),
                ifindex: 1,
                ifname: Some("eth0".to_string()),
                pkt_len: 100,
                src_ip: IpAddr::V4(Ipv4Addr::new(192, 168, 1, 1)),
                dst_ip: IpAddr::V4(Ipv4Addr::new(192, 168, 1, 2)),
                src_port: 12345,
                dst_port: 80,
                protocol: crate::events::packet::IpProtocol::Tcp,
                direction: crate::events::packet::PacketDirection::Egress,
                tcp_flags: None,
            }),
            raw: None,
        }
    }

    #[test]
    fn test_correlation_engine_creation() {
        let engine = CorrelationEngine::new(CorrelationConfig::default());
        assert_eq!(engine.active_trace_count(), 0);
    }

    #[test]
    fn test_process_event_creates_trace() {
        let engine = CorrelationEngine::new(CorrelationConfig::default());
        let event = create_test_packet_event();

        let trace_id = engine.process_event(&event);
        assert!(trace_id.is_some());
        assert_eq!(engine.active_trace_count(), 1);
    }

    #[test]
    fn test_flow_association() {
        let engine = CorrelationEngine::new(CorrelationConfig::default());
        let event = create_test_packet_event();

        let trace_id = engine.process_event(&event).unwrap();

        let flow = FlowKey::new(
            u32::from(Ipv4Addr::new(192, 168, 1, 1)),
            u32::from(Ipv4Addr::new(192, 168, 1, 2)),
            12345,
            80,
            6,
        );

        let found_trace = engine.trace_by_flow(&flow);
        assert_eq!(found_trace, Some(trace_id));
    }

    #[test]
    fn test_trace_summary() {
        let trace_id = TraceId::generate();
        let ctx = TraceContext::new(trace_id, SpanId::generate(), 1000);

        let summary = TraceSummary::from_context(&ctx);
        assert_eq!(summary.trace_id, trace_id.to_hex());
        assert_eq!(summary.span_count, 0);
    }

    #[test]
    fn test_sampling() {
        let mut config = CorrelationConfig::default();
        config.sample_rate = 0.0; // Sample nothing

        let engine = CorrelationEngine::new(config);
        let event = create_test_packet_event();

        let trace_id = engine.process_event(&event);
        assert!(trace_id.is_none());
    }
}
