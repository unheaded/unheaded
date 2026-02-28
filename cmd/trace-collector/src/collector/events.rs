//! Event processing and filtering for the collector.
//!
//! Provides filtering capabilities to reduce event volume and
//! processing utilities for event transformation.

use regex::Regex;
use std::collections::HashSet;

use crate::events::{Event, EventType};

/// Event filter configuration
#[derive(Debug, Clone)]
pub struct EventFilter {
    /// Allowed event types (empty = all allowed)
    pub event_types: HashSet<EventType>,
    /// Process name filter (regex)
    pub process_filter: Option<Regex>,
    /// PID filter (specific PIDs to include)
    pub pid_filter: HashSet<u32>,
    /// Minimum latency threshold (nanoseconds)
    pub min_latency_ns: u64,
    /// Maximum events per second (rate limiting)
    pub max_rate: Option<u64>,
    /// Invert the filter (exclude instead of include)
    pub invert: bool,
}

impl EventFilter {
    /// Create a new filter that allows all events
    pub fn allow_all() -> Self {
        Self {
            event_types: HashSet::new(),
            process_filter: None,
            pid_filter: HashSet::new(),
            min_latency_ns: 0,
            max_rate: None,
            invert: false,
        }
    }

    /// Create a filter for specific event types
    pub fn event_types(types: &[EventType]) -> Self {
        Self {
            event_types: types.iter().cloned().collect(),
            process_filter: None,
            pid_filter: HashSet::new(),
            min_latency_ns: 0,
            max_rate: None,
            invert: false,
        }
    }

    /// Add a process name filter
    pub fn with_process_filter(mut self, pattern: &str) -> Self {
        self.process_filter = Regex::new(pattern).ok();
        self
    }

    /// Add specific PIDs to filter
    pub fn with_pids(mut self, pids: &[u32]) -> Self {
        self.pid_filter = pids.iter().cloned().collect();
        self
    }

    /// Set minimum latency threshold
    pub fn with_min_latency_ns(mut self, ns: u64) -> Self {
        self.min_latency_ns = ns;
        self
    }

    /// Set rate limit
    pub fn with_max_rate(mut self, rate: u64) -> Self {
        self.max_rate = Some(rate);
        self
    }

    /// Invert the filter
    pub fn inverted(mut self) -> Self {
        self.invert = true;
        self
    }

    /// Check if an event matches this filter
    pub fn matches(&self, event: &Event) -> bool {
        let mut matches = true;

        // Check event type
        if !self.event_types.is_empty() && !self.event_types.contains(&event.event_type) {
            matches = false;
        }

        // Check process filter
        if let Some(ref regex) = self.process_filter {
            if !regex.is_match(&event.comm) {
                matches = false;
            }
        }

        // Check PID filter
        if !self.pid_filter.is_empty() && !self.pid_filter.contains(&event.pid) {
            matches = false;
        }

        // Check latency threshold for latency events
        if self.min_latency_ns > 0 {
            if let crate::events::EventData::Latency(ref latency) = event.data {
                if latency.latency_ns < self.min_latency_ns {
                    matches = false;
                }
            }
        }

        // Apply inversion
        if self.invert {
            !matches
        } else {
            matches
        }
    }
}

impl Default for EventFilter {
    fn default() -> Self {
        Self::allow_all()
    }
}

/// Event processor for transforming events
pub struct EventProcessor {
    /// Hostname to attach to events
    #[allow(dead_code)]
    hostname: String,
    /// Enable enrichment
    enrichment_enabled: bool,
}

impl EventProcessor {
    /// Create a new event processor
    pub fn new() -> Self {
        Self {
            hostname: hostname::get()
                .map(|h| h.to_string_lossy().to_string())
                .unwrap_or_else(|_| "unknown".to_string()),
            enrichment_enabled: true,
        }
    }

    /// Process an event, optionally enriching it
    pub fn process(&self, event: Event) -> Event {
        if self.enrichment_enabled {
            // Add hostname if not present
            // Events don't have a hostname field currently, but we could add metadata
        }
        event
    }

    /// Enable or disable enrichment
    pub fn set_enrichment(&mut self, enabled: bool) {
        self.enrichment_enabled = enabled;
    }
}

impl Default for EventProcessor {
    fn default() -> Self {
        Self::new()
    }
}

/// Rate limiter for event filtering
pub struct RateLimiter {
    /// Maximum events per second
    max_rate: u64,
    /// Current count in this window
    count: u64,
    /// Window start time
    window_start: std::time::Instant,
    /// Window duration
    window_duration: std::time::Duration,
}

impl RateLimiter {
    /// Create a new rate limiter
    pub fn new(max_rate: u64) -> Self {
        Self {
            max_rate,
            count: 0,
            window_start: std::time::Instant::now(),
            window_duration: std::time::Duration::from_secs(1),
        }
    }

    /// Check if an event should be allowed
    pub fn allow(&mut self) -> bool {
        let now = std::time::Instant::now();

        // Reset window if expired
        if now.duration_since(self.window_start) >= self.window_duration {
            self.count = 0;
            self.window_start = now;
        }

        // Check rate
        if self.count < self.max_rate {
            self.count += 1;
            true
        } else {
            false
        }
    }

    /// Get current rate
    pub fn current_rate(&self) -> f64 {
        let elapsed = self.window_start.elapsed().as_secs_f64();
        if elapsed > 0.0 {
            self.count as f64 / elapsed
        } else {
            0.0
        }
    }
}

/// Event aggregator for reducing event volume
pub struct EventAggregator {
    /// Aggregation window
    window: std::time::Duration,
    /// Last flush time
    last_flush: std::time::Instant,
    /// Pending events by key
    pending: std::collections::HashMap<String, AggregatedEvent>,
}

/// Aggregated event
#[derive(Debug, Clone)]
pub struct AggregatedEvent {
    /// First event timestamp
    pub first_ts: u64,
    /// Last event timestamp
    pub last_ts: u64,
    /// Event count
    pub count: u64,
    /// Sum of latencies (for averaging)
    pub latency_sum_ns: u64,
    /// Min latency
    pub min_latency_ns: u64,
    /// Max latency
    pub max_latency_ns: u64,
}

impl EventAggregator {
    /// Create a new aggregator with the given window
    pub fn new(window: std::time::Duration) -> Self {
        Self {
            window,
            last_flush: std::time::Instant::now(),
            pending: std::collections::HashMap::new(),
        }
    }

    /// Add an event to the aggregator
    pub fn add(&mut self, event: &Event) -> Option<AggregatedEvent> {
        // Create aggregation key
        let key = format!(
            "{}:{}:{}:{:?}",
            event.comm, event.pid, event.cpu, event.event_type
        );

        // Update or create aggregated event
        let entry = self.pending.entry(key).or_insert_with(|| AggregatedEvent {
            first_ts: event.timestamp_ns,
            last_ts: event.timestamp_ns,
            count: 0,
            latency_sum_ns: 0,
            min_latency_ns: u64::MAX,
            max_latency_ns: 0,
        });

        entry.last_ts = event.timestamp_ns;
        entry.count += 1;

        // Add latency if present
        if let crate::events::EventData::Latency(ref latency) = event.data {
            entry.latency_sum_ns += latency.latency_ns;
            entry.min_latency_ns = entry.min_latency_ns.min(latency.latency_ns);
            entry.max_latency_ns = entry.max_latency_ns.max(latency.latency_ns);
        }

        // Check if we should flush
        if self.last_flush.elapsed() >= self.window {
            self.last_flush = std::time::Instant::now();
            // Return aggregated events would require more complex handling
            None
        } else {
            None
        }
    }

    /// Flush all pending aggregated events
    pub fn flush(&mut self) -> Vec<AggregatedEvent> {
        let events: Vec<_> = self.pending.values().cloned().collect();
        self.pending.clear();
        self.last_flush = std::time::Instant::now();
        events
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::events::{EventData, EventType};

    fn make_test_event(event_type: EventType, comm: &str, pid: u32) -> Event {
        Event::new(
            event_type,
            0,
            1000000,
            pid,
            pid,
            comm.to_string(),
            EventData::Raw(vec![]),
        )
    }

    #[test]
    fn test_filter_allow_all() {
        let filter = EventFilter::allow_all();
        let event = make_test_event(EventType::Packet, "nginx", 1234);
        assert!(filter.matches(&event));
    }

    #[test]
    fn test_filter_event_types() {
        let filter = EventFilter::event_types(&[EventType::Packet]);

        let packet_event = make_test_event(EventType::Packet, "nginx", 1234);
        let syscall_event = make_test_event(EventType::Syscall, "nginx", 1234);

        assert!(filter.matches(&packet_event));
        assert!(!filter.matches(&syscall_event));
    }

    #[test]
    fn test_filter_process() {
        let filter = EventFilter::allow_all().with_process_filter("nginx|envoy");

        let nginx_event = make_test_event(EventType::Packet, "nginx", 1234);
        let curl_event = make_test_event(EventType::Packet, "curl", 5678);

        assert!(filter.matches(&nginx_event));
        assert!(!filter.matches(&curl_event));
    }

    #[test]
    fn test_filter_pids() {
        let filter = EventFilter::allow_all().with_pids(&[1234, 5678]);

        let event1 = make_test_event(EventType::Packet, "nginx", 1234);
        let event2 = make_test_event(EventType::Packet, "curl", 9999);

        assert!(filter.matches(&event1));
        assert!(!filter.matches(&event2));
    }

    #[test]
    fn test_filter_inverted() {
        let filter = EventFilter::event_types(&[EventType::Syscall]).inverted();

        let packet_event = make_test_event(EventType::Packet, "nginx", 1234);
        let syscall_event = make_test_event(EventType::Syscall, "nginx", 1234);

        // Inverted: syscall events are excluded, others included
        assert!(filter.matches(&packet_event));
        assert!(!filter.matches(&syscall_event));
    }

    #[test]
    fn test_rate_limiter() {
        let mut limiter = RateLimiter::new(10);

        // Should allow first 10
        for _ in 0..10 {
            assert!(limiter.allow());
        }

        // Should deny 11th
        assert!(!limiter.allow());
    }

    #[test]
    fn test_event_aggregator() {
        let mut aggregator = EventAggregator::new(std::time::Duration::from_secs(1));

        let event = make_test_event(EventType::Packet, "nginx", 1234);
        aggregator.add(&event);
        aggregator.add(&event);

        let flushed = aggregator.flush();
        assert_eq!(flushed.len(), 1);
        assert_eq!(flushed[0].count, 2);
    }
}
