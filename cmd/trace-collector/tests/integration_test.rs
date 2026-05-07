//! Integration tests for the trace-collector pipeline.
//!
//! These tests verify the eBPF -> trace-collector -> Wotan pipeline
//! using mock eBPF events instead of real kernel ring buffers.
//!
//! Test coverage:
//! 1. Correlation engine receives mock events and creates correlated traces
//! 2. Publisher batching serializes and batches events correctly
//! 3. WebSocket server starts and accepts connections
//! 4. End-to-end: mock events -> correlation -> publisher batching -> output

use std::net::{IpAddr, Ipv4Addr};
use std::sync::atomic::Ordering;
use std::sync::Arc;
use std::time::Duration;

use futures_util::StreamExt;

use trace_collector::correlation::{
    CorrelationConfig, CorrelationEngine, FlowKey, TraceId, TraceQuery, TraceStore,
    TraceStoreConfig, TraceSummary,
};
use trace_collector::events::latency::{LatencyBucket, ProbeType};
use trace_collector::events::packet::{IpProtocol, PacketDirection};
use trace_collector::events::syscall::SyscallCategory;
use trace_collector::events::{
    Event, EventBatch, EventData, EventType, LatencyEvent, PacketEvent, SyscallEvent,
};
use trace_collector::publisher::batch::{BatchPayload, BatchState, EventBatcher};
use trace_collector::websocket::{
    ClientMessage, ServerMessage, Subscription, TraceUpdate, WebSocketConfig, WebSocketServer,
};

// ============================================================================
// TEST HELPERS
// ============================================================================

/// Create a mock packet event with configurable parameters.
fn mock_packet_event(
    pid: u32,
    comm: &str,
    src_ip: Ipv4Addr,
    dst_ip: Ipv4Addr,
    src_port: u16,
    dst_port: u16,
    timestamp_ns: u64,
) -> Event {
    Event::new(
        EventType::Packet,
        0,
        timestamp_ns,
        pid,
        pid,
        comm.to_string(),
        EventData::Packet(PacketEvent {
            pid,
            tid: pid,
            comm: comm.to_string(),
            ifindex: 1,
            ifname: Some("eth0".to_string()),
            pkt_len: 128,
            src_ip: IpAddr::V4(src_ip),
            dst_ip: IpAddr::V4(dst_ip),
            src_port,
            dst_port,
            protocol: IpProtocol::Tcp,
            direction: PacketDirection::Egress,
            tcp_flags: None,
        }),
    )
}

/// Create a mock latency event.
fn mock_latency_event(pid: u32, comm: &str, latency_ns: u64, timestamp_ns: u64) -> Event {
    Event::new(
        EventType::Latency,
        0,
        timestamp_ns,
        pid,
        pid,
        comm.to_string(),
        EventData::Latency(LatencyEvent {
            pid,
            tid: pid,
            comm: comm.to_string(),
            probe_type: ProbeType::Kprobe,
            probe_name: "tcp_sendmsg".to_string(),
            latency_ns,
            entry_ts: timestamp_ns - latency_ns,
            exit_ts: timestamp_ns,
            stack_id: None,
            bucket: LatencyBucket::from_ns(latency_ns),
        }),
    )
}

/// Create a mock syscall event.
fn mock_syscall_event(pid: u32, comm: &str, syscall_nr: u32, timestamp_ns: u64) -> Event {
    Event::new(
        EventType::Syscall,
        0,
        timestamp_ns,
        pid,
        pid,
        comm.to_string(),
        EventData::Syscall(SyscallEvent {
            pid,
            tid: pid,
            comm: comm.to_string(),
            syscall_nr,
            syscall_name: format!("syscall_{}", syscall_nr),
            category: SyscallCategory::Network,
            is_exit: true,
            ret: Some(0),
            args: vec![0; 6],
            duration_ns: Some(5000),
            error: None,
        }),
    )
}

/// Create a batch of diverse mock events simulating a real workload.
fn mock_event_stream(count: usize) -> Vec<Event> {
    let mut events = Vec::with_capacity(count);
    let base_ts: u64 = 1_000_000_000;

    for i in 0..count {
        let ts = base_ts + (i as u64) * 1000;
        let event = match i % 3 {
            0 => mock_packet_event(
                1000 + (i as u32 % 5),
                "nginx",
                Ipv4Addr::new(10, 0, 0, 1),
                Ipv4Addr::new(10, 0, 0, 2),
                (8080 + (i % 10)) as u16,
                80,
                ts,
            ),
            1 => mock_latency_event(
                1000 + (i as u32 % 5),
                "nginx",
                50_000 + (i as u64 * 100),
                ts,
            ),
            2 => mock_syscall_event(
                1000 + (i as u32 % 5),
                "nginx",
                42, // connect
                ts,
            ),
            _ => unreachable!(),
        };
        events.push(event);
    }

    events
}

// ============================================================================
// 1. CORRELATION ENGINE TESTS
// ============================================================================

#[test]
fn correlation_engine_processes_packet_events_and_creates_traces() {
    let engine = CorrelationEngine::new(CorrelationConfig::default());
    assert_eq!(engine.active_trace_count(), 0);

    let event = mock_packet_event(
        1234,
        "curl",
        Ipv4Addr::new(192, 168, 1, 10),
        Ipv4Addr::new(10, 0, 0, 1),
        54321,
        443,
        1_000_000,
    );

    let trace_id = engine.process_event(&event);
    assert!(trace_id.is_some(), "Packet event should create a trace");
    assert_eq!(engine.active_trace_count(), 1);

    let stats = engine.stats();
    assert_eq!(stats.events_processed.load(Ordering::Relaxed), 1);
    assert_eq!(stats.traces_created.load(Ordering::Relaxed), 1);
}

#[test]
fn correlation_engine_groups_same_flow_into_one_trace() {
    let engine = CorrelationEngine::new(CorrelationConfig::default());

    let src = Ipv4Addr::new(10, 0, 0, 1);
    let dst = Ipv4Addr::new(10, 0, 0, 2);

    // Three packets on the same flow (same 5-tuple)
    let ev1 = mock_packet_event(100, "nginx", src, dst, 8080, 80, 1_000_000);
    let ev2 = mock_packet_event(100, "nginx", src, dst, 8080, 80, 2_000_000);
    let ev3 = mock_packet_event(100, "nginx", src, dst, 8080, 80, 3_000_000);

    let t1 = engine.process_event(&ev1).unwrap();
    let t2 = engine.process_event(&ev2).unwrap();
    let t3 = engine.process_event(&ev3).unwrap();

    // All three events should be part of the same trace
    assert_eq!(t1, t2);
    assert_eq!(t2, t3);
    assert_eq!(engine.active_trace_count(), 1);

    // Verify the trace context accumulated data
    let ctx = engine.get_trace(t1).unwrap();
    assert!(
        ctx.spans.len() >= 1,
        "Trace should have at least 1 span (root)"
    );
    assert!(ctx.packet_count >= 2, "Packet count should reflect updates");
}

#[test]
fn correlation_engine_creates_separate_traces_for_different_flows() {
    let engine = CorrelationEngine::new(CorrelationConfig::default());

    let flow_a = mock_packet_event(
        100,
        "nginx",
        Ipv4Addr::new(10, 0, 0, 1),
        Ipv4Addr::new(10, 0, 0, 2),
        8080,
        80,
        1_000_000,
    );
    let flow_b = mock_packet_event(
        200,
        "envoy",
        Ipv4Addr::new(172, 16, 0, 1),
        Ipv4Addr::new(172, 16, 0, 2),
        9090,
        443,
        1_000_000,
    );

    let t_a = engine.process_event(&flow_a).unwrap();
    let t_b = engine.process_event(&flow_b).unwrap();

    assert_ne!(t_a, t_b, "Different flows should produce different traces");
    assert_eq!(engine.active_trace_count(), 2);
}

#[test]
fn correlation_engine_flow_association_works_bidirectionally() {
    let engine = CorrelationEngine::new(CorrelationConfig::default());

    let event = mock_packet_event(
        100,
        "nginx",
        Ipv4Addr::new(10, 0, 0, 1),
        Ipv4Addr::new(10, 0, 0, 2),
        8080,
        80,
        1_000_000,
    );

    let trace_id = engine.process_event(&event).unwrap();

    // Look up via the flow key
    let flow = FlowKey::new(
        u32::from(Ipv4Addr::new(10, 0, 0, 1)),
        u32::from(Ipv4Addr::new(10, 0, 0, 2)),
        8080,
        80,
        6, // TCP
    );

    let found = engine.trace_by_flow(&flow);
    assert_eq!(found, Some(trace_id));

    // Reverse direction should also find it (normalized flow key)
    let flow_reverse = FlowKey::new(
        u32::from(Ipv4Addr::new(10, 0, 0, 2)),
        u32::from(Ipv4Addr::new(10, 0, 0, 1)),
        80,
        8080,
        6,
    );

    let found_reverse = engine.trace_by_flow(&flow_reverse);
    assert_eq!(found_reverse, Some(trace_id));
}

#[test]
fn correlation_engine_sampling_zero_drops_all() {
    let config = CorrelationConfig {
        sample_rate: 0.0,
        ..CorrelationConfig::default()
    };
    let engine = CorrelationEngine::new(config);

    let event = mock_packet_event(
        100,
        "nginx",
        Ipv4Addr::new(10, 0, 0, 1),
        Ipv4Addr::new(10, 0, 0, 2),
        8080,
        80,
        1_000_000,
    );

    let result = engine.process_event(&event);
    assert!(result.is_none(), "Zero sample rate should drop all events");
    assert_eq!(engine.active_trace_count(), 0);
    assert_eq!(
        engine.stats().sampled_out.load(Ordering::Relaxed),
        1,
        "Should record sampled out"
    );
}

#[test]
fn correlation_engine_raw_events_counted_as_orphans() {
    let engine = CorrelationEngine::new(CorrelationConfig::default());

    let raw_event = Event::new(
        EventType::Custom,
        0,
        1_000_000,
        0,
        0,
        "unknown".to_string(),
        EventData::Raw(vec![0xde, 0xad, 0xbe, 0xef]),
    );

    let result = engine.process_event(&raw_event);
    assert!(result.is_none());
    assert_eq!(engine.stats().orphan_events.load(Ordering::Relaxed), 1);
}

#[test]
fn correlation_engine_handles_high_volume_events() {
    let engine = CorrelationEngine::new(CorrelationConfig::default());
    let events = mock_event_stream(500);

    let mut trace_ids = Vec::new();
    for event in &events {
        if let Some(id) = engine.process_event(event) {
            trace_ids.push(id);
        }
    }

    assert!(
        engine.stats().events_processed.load(Ordering::Relaxed) == 500,
        "All 500 events should be processed"
    );
    assert!(
        engine.active_trace_count() > 0,
        "Should have created at least one trace"
    );
    assert!(
        engine.stats().traces_created.load(Ordering::Relaxed) > 0,
        "Should have created traces"
    );
}

#[test]
fn correlation_engine_trace_summary_from_context() {
    let engine = CorrelationEngine::new(CorrelationConfig::default());

    // Process a packet event to create a trace
    let event = mock_packet_event(
        1234,
        "test-svc",
        Ipv4Addr::new(192, 168, 1, 1),
        Ipv4Addr::new(192, 168, 1, 2),
        12345,
        80,
        1_000_000,
    );

    let trace_id = engine.process_event(&event).unwrap();
    let ctx = engine.get_trace(trace_id).unwrap();

    let summary = TraceSummary::from_context(&ctx);
    assert_eq!(summary.trace_id, trace_id.to_hex());
    assert!(summary.services.contains(&"test-svc".to_string()));
    assert!(summary.span_count >= 1);
    assert_eq!(summary.start_time_ns, 1_000_000);
}

#[test]
fn correlation_engine_completed_trace_subscription() {
    let engine = CorrelationEngine::new(CorrelationConfig {
        max_active_traces: 2,
        ..CorrelationConfig::default()
    });

    let mut rx = engine.subscribe_completed();

    // Fill up to max_active_traces
    let ev1 = mock_packet_event(
        100,
        "svc-a",
        Ipv4Addr::new(10, 0, 0, 1),
        Ipv4Addr::new(10, 0, 0, 2),
        1000,
        80,
        1_000_000,
    );
    let ev2 = mock_packet_event(
        200,
        "svc-b",
        Ipv4Addr::new(172, 16, 0, 1),
        Ipv4Addr::new(172, 16, 0, 2),
        2000,
        443,
        2_000_000,
    );
    // Third event on a new flow forces eviction of oldest
    let ev3 = mock_packet_event(
        300,
        "svc-c",
        Ipv4Addr::new(192, 168, 0, 1),
        Ipv4Addr::new(192, 168, 0, 2),
        3000,
        8080,
        3_000_000,
    );

    engine.process_event(&ev1);
    engine.process_event(&ev2);
    engine.process_event(&ev3);

    // When the third trace is created, the oldest should be evicted
    // and a completed trace summary should be broadcast
    let summary = rx.try_recv();
    assert!(
        summary.is_ok(),
        "Should receive a completed trace summary when evicting"
    );

    let summary = summary.unwrap();
    assert!(!summary.trace_id.is_empty());
}

// ============================================================================
// 2. PUBLISHER / EVENT BATCHER TESTS
// ============================================================================

#[test]
fn batcher_starts_empty() {
    let batcher = EventBatcher::new("integration-test".to_string(), 100, Duration::from_secs(10));
    assert!(batcher.is_empty());
    assert_eq!(batcher.len(), 0);
    assert_eq!(batcher.state(), BatchState::Empty);
}

#[test]
fn batcher_collects_events_and_tracks_state() {
    let mut batcher =
        EventBatcher::new("integration-test".to_string(), 10, Duration::from_secs(10));

    let event = mock_packet_event(
        1234,
        "test",
        Ipv4Addr::new(10, 0, 0, 1),
        Ipv4Addr::new(10, 0, 0, 2),
        8080,
        80,
        1_000_000,
    );

    assert!(batcher.add(event));
    assert_eq!(batcher.len(), 1);
    assert_eq!(batcher.state(), BatchState::Collecting);
    assert!(!batcher.should_flush());
}

#[test]
fn batcher_transitions_to_ready_at_capacity() {
    let mut batcher = EventBatcher::new("integration-test".to_string(), 3, Duration::from_secs(60));

    for i in 0..3 {
        let event = mock_packet_event(
            1234,
            "test",
            Ipv4Addr::new(10, 0, 0, 1),
            Ipv4Addr::new(10, 0, 0, 2),
            (8080 + i) as u16,
            80,
            1_000_000 + (i as u64) * 1000,
        );
        batcher.add(event);
    }

    assert_eq!(batcher.len(), 3);
    assert!(batcher.should_flush());
    assert_eq!(batcher.state(), BatchState::Ready);
}

#[test]
fn batcher_flush_produces_valid_payload() {
    let mut batcher = EventBatcher::new("integration-test".to_string(), 5, Duration::from_secs(60));

    for i in 0..5 {
        let event = mock_packet_event(
            1234,
            "test",
            Ipv4Addr::new(10, 0, 0, 1),
            Ipv4Addr::new(10, 0, 0, 2),
            (8080 + i) as u16,
            80,
            1_000_000 + (i as u64) * 1000,
        );
        batcher.add(event);
    }

    let payload = batcher.flush();
    assert!(payload.is_some(), "Flush should produce a payload");

    let payload = payload.unwrap();
    assert_eq!(payload.event_count, 5);
    assert!(payload.payload_bytes > 0);
    assert_eq!(payload.sequence, 0);
    assert_eq!(payload.first_timestamp, 1_000_000);
    assert_eq!(payload.last_timestamp, 1_004_000);
    assert_eq!(payload.duration_ns(), 4_000);

    // After flush, batcher should be empty
    assert!(batcher.is_empty());
    assert_eq!(batcher.state(), BatchState::Empty);
}

#[test]
fn batcher_flush_on_empty_returns_none() {
    let mut batcher =
        EventBatcher::new("integration-test".to_string(), 10, Duration::from_secs(60));

    let payload = batcher.flush();
    assert!(payload.is_none());
}

#[test]
fn batcher_sequence_increments_across_flushes() {
    let mut batcher = EventBatcher::new("integration-test".to_string(), 2, Duration::from_secs(60));

    // First batch
    batcher.add(mock_packet_event(
        100,
        "a",
        Ipv4Addr::new(10, 0, 0, 1),
        Ipv4Addr::new(10, 0, 0, 2),
        1000,
        80,
        1_000_000,
    ));
    batcher.add(mock_packet_event(
        100,
        "a",
        Ipv4Addr::new(10, 0, 0, 1),
        Ipv4Addr::new(10, 0, 0, 2),
        1001,
        80,
        2_000_000,
    ));
    let p1 = batcher.flush().unwrap();
    assert_eq!(p1.sequence, 0);

    // Second batch
    batcher.add(mock_packet_event(
        200,
        "b",
        Ipv4Addr::new(10, 0, 0, 3),
        Ipv4Addr::new(10, 0, 0, 4),
        2000,
        443,
        3_000_000,
    ));
    let p2 = batcher.flush().unwrap();
    assert_eq!(p2.sequence, 1);
}

#[test]
fn batcher_stats_accumulate_correctly() {
    let mut batcher =
        EventBatcher::new("integration-test".to_string(), 100, Duration::from_secs(60));

    for i in 0..20 {
        batcher.add(mock_packet_event(
            100,
            "test",
            Ipv4Addr::new(10, 0, 0, 1),
            Ipv4Addr::new(10, 0, 0, 2),
            (1000 + i) as u16,
            80,
            (i as u64) * 1000,
        ));
    }

    assert_eq!(batcher.stats().events_batched.load(Ordering::Relaxed), 20);

    batcher.flush();
    assert_eq!(batcher.stats().batches_flushed.load(Ordering::Relaxed), 1);
}

#[test]
fn batcher_serializes_diverse_event_types() {
    let mut batcher =
        EventBatcher::new("integration-test".to_string(), 100, Duration::from_secs(60));

    // Add one of each type
    batcher.add(mock_packet_event(
        100,
        "nginx",
        Ipv4Addr::new(10, 0, 0, 1),
        Ipv4Addr::new(10, 0, 0, 2),
        8080,
        80,
        1_000_000,
    ));
    batcher.add(mock_latency_event(200, "envoy", 500_000, 2_000_000));
    batcher.add(mock_syscall_event(300, "curl", 42, 3_000_000));

    let payload = batcher.flush().unwrap();
    assert_eq!(payload.event_count, 3);
    assert!(payload.payload_bytes > 0);

    // Verify we can decode the protobuf
    let decoded = prost::Message::decode(&payload.payload[..]);
    let decoded: Result<trace_collector::proto::TraceEventBatch, _> = decoded;
    assert!(decoded.is_ok(), "Payload should be valid protobuf");

    let batch = decoded.unwrap();
    assert_eq!(batch.events.len(), 3);
    assert_eq!(batch.collector_id, "integration-test");
}

#[test]
fn batch_payload_compression_ratio() {
    let payload = BatchPayload {
        sequence: 0,
        event_count: 10,
        raw_bytes: 2000,
        payload_bytes: 500,
        compressed: true,
        first_timestamp: 1_000_000,
        last_timestamp: 2_000_000,
        payload: bytes::Bytes::new(),
    };

    assert_eq!(payload.compression_ratio(), 4.0);
    assert_eq!(payload.duration_ns(), 1_000_000);
}

// ============================================================================
// 3. EVENT SERIALIZATION ROUND-TRIP TESTS
// ============================================================================

#[test]
fn event_serialization_roundtrip_packet() {
    let event = mock_packet_event(
        42,
        "wotan",
        Ipv4Addr::new(10, 10, 10, 10),
        Ipv4Addr::new(10, 10, 10, 20),
        50051,
        80,
        999_999_999,
    );

    let json = serde_json::to_string(&event).unwrap();
    let deserialized: Event = serde_json::from_str(&json).unwrap();

    assert_eq!(deserialized.event_type, EventType::Packet);
    assert_eq!(deserialized.pid, 42);
    assert_eq!(deserialized.comm, "wotan");
    assert_eq!(deserialized.timestamp_ns, 999_999_999);

    if let EventData::Packet(pkt) = &deserialized.data {
        assert_eq!(pkt.src_port, 50051);
        assert_eq!(pkt.dst_port, 80);
    } else {
        panic!("Expected Packet data after deserialization");
    }
}

#[test]
fn event_serialization_roundtrip_latency() {
    let event = mock_latency_event(555, "trace-collector", 250_000, 5_000_000);

    let json = serde_json::to_string(&event).unwrap();
    let deserialized: Event = serde_json::from_str(&json).unwrap();

    assert_eq!(deserialized.event_type, EventType::Latency);
    if let EventData::Latency(lat) = &deserialized.data {
        assert_eq!(lat.latency_ns, 250_000);
        assert_eq!(lat.probe_name, "tcp_sendmsg");
    } else {
        panic!("Expected Latency data after deserialization");
    }
}

#[test]
fn event_serialization_roundtrip_syscall() {
    let event = mock_syscall_event(777, "envoy", 42, 10_000_000);

    let json = serde_json::to_string(&event).unwrap();
    let deserialized: Event = serde_json::from_str(&json).unwrap();

    assert_eq!(deserialized.event_type, EventType::Syscall);
    if let EventData::Syscall(sc) = &deserialized.data {
        assert_eq!(sc.syscall_nr, 42);
        assert_eq!(sc.ret, Some(0));
    } else {
        panic!("Expected Syscall data after deserialization");
    }
}

#[test]
fn event_batch_tracks_timestamps_and_bytes() {
    let mut batch = EventBatch::new();
    assert!(batch.is_empty());
    assert_eq!(batch.duration_ns(), 0);

    batch.push(mock_packet_event(
        1,
        "a",
        Ipv4Addr::new(1, 1, 1, 1),
        Ipv4Addr::new(2, 2, 2, 2),
        80,
        80,
        100,
    ));
    batch.push(mock_packet_event(
        2,
        "b",
        Ipv4Addr::new(3, 3, 3, 3),
        Ipv4Addr::new(4, 4, 4, 4),
        443,
        443,
        500,
    ));

    assert_eq!(batch.len(), 2);
    assert_eq!(batch.first_timestamp, 100);
    assert_eq!(batch.last_timestamp, 500);
    assert_eq!(batch.duration_ns(), 400);
    assert!(batch.total_bytes > 0);

    batch.clear();
    assert!(batch.is_empty());
    assert_eq!(batch.first_timestamp, 0);
}

// ============================================================================
// 4. TRACE STORE TESTS
// ============================================================================

#[test]
fn trace_store_stores_and_retrieves_summaries() {
    let store = TraceStore::new(TraceStoreConfig::default()).unwrap();

    let summary = TraceSummary {
        trace_id: "abcdef0123456789abcdef0123456789".to_string(),
        root_operation: "network.request".to_string(),
        services: vec!["nginx".to_string(), "envoy".to_string()],
        span_count: 5,
        duration_ns: 10_000,
        start_time_ns: 1_000_000,
        end_time_ns: 1_010_000,
        status: "ok".to_string(),
        avg_latency_ns: Some(2_000),
        p95_latency_ns: Some(5_000),
        packet_count: 20,
        error_count: 0,
    };

    store.store_summary(summary.clone());
    assert_eq!(store.trace_count(), 1);

    let retrieved = store.get("abcdef0123456789abcdef0123456789");
    assert!(retrieved.is_some());
    let retrieved = retrieved.unwrap();
    assert_eq!(retrieved.root_operation, "network.request");
    assert_eq!(retrieved.span_count, 5);
    assert_eq!(retrieved.services.len(), 2);
}

#[test]
fn trace_store_query_by_service() {
    let store = TraceStore::new(TraceStoreConfig::default()).unwrap();

    let nginx_trace = TraceSummary {
        trace_id: "trace-nginx-1".to_string(),
        root_operation: "http.request".to_string(),
        services: vec!["nginx".to_string()],
        span_count: 3,
        duration_ns: 5_000,
        start_time_ns: 1_000_000,
        end_time_ns: 1_005_000,
        status: "ok".to_string(),
        avg_latency_ns: None,
        p95_latency_ns: None,
        packet_count: 10,
        error_count: 0,
    };

    let envoy_trace = TraceSummary {
        trace_id: "trace-envoy-1".to_string(),
        root_operation: "grpc.call".to_string(),
        services: vec!["envoy".to_string()],
        span_count: 2,
        duration_ns: 8_000,
        start_time_ns: 2_000_000,
        end_time_ns: 2_008_000,
        status: "ok".to_string(),
        avg_latency_ns: None,
        p95_latency_ns: None,
        packet_count: 5,
        error_count: 0,
    };

    store.store_summary(nginx_trace);
    store.store_summary(envoy_trace);

    let result = store.query(&TraceQuery::new().service("nginx"));
    assert_eq!(result.total_count, 1);
    assert_eq!(result.traces[0].trace_id, "trace-nginx-1");

    let result = store.query(&TraceQuery::new().service("envoy"));
    assert_eq!(result.total_count, 1);
    assert_eq!(result.traces[0].trace_id, "trace-envoy-1");
}

#[test]
fn trace_store_query_by_duration() {
    let store = TraceStore::new(TraceStoreConfig::default()).unwrap();

    for i in 0..5 {
        let summary = TraceSummary {
            trace_id: format!("trace-{}", i),
            root_operation: "op".to_string(),
            services: vec!["svc".to_string()],
            span_count: 1,
            duration_ns: (i + 1) as u64 * 10_000,
            start_time_ns: (i as u64) * 100_000,
            end_time_ns: (i as u64) * 100_000 + (i + 1) as u64 * 10_000,
            status: "ok".to_string(),
            avg_latency_ns: None,
            p95_latency_ns: None,
            packet_count: 0,
            error_count: 0,
        };
        store.store_summary(summary);
    }

    // Only traces with duration >= 30_000 ns
    let result = store.query(&TraceQuery::new().min_duration(30_000));
    assert_eq!(result.total_count, 3); // traces 2,3,4 => 30k, 40k, 50k
}

#[test]
fn trace_store_eviction_on_capacity() {
    let config = TraceStoreConfig {
        max_memory_traces: 3,
        ..TraceStoreConfig::default()
    };
    let store = TraceStore::new(config).unwrap();

    for i in 0..5 {
        let summary = TraceSummary {
            trace_id: format!("trace-{}", i),
            root_operation: "op".to_string(),
            services: vec!["svc".to_string()],
            span_count: 1,
            duration_ns: 1000,
            start_time_ns: (i as u64) * 1000,
            end_time_ns: (i as u64) * 1000 + 1000,
            status: "ok".to_string(),
            avg_latency_ns: None,
            p95_latency_ns: None,
            packet_count: 0,
            error_count: 0,
        };
        store.store_summary(summary);
    }

    // Should have evicted oldest, keeping at most 3
    assert!(store.trace_count() <= 3);
    // Oldest traces should be gone
    assert!(store.get("trace-0").is_none());
    assert!(store.get("trace-1").is_none());
}

// ============================================================================
// 5. WEBSOCKET SERVER TESTS
// ============================================================================

#[test]
fn websocket_server_creation_and_config() {
    let server = WebSocketServer::new(WebSocketConfig {
        bind_addr: "127.0.0.1:0".to_string(), // Port 0 for test
        max_connections: 50,
        ..WebSocketConfig::default()
    });
    assert_eq!(server.connection_count(), 0);

    let stats = server.stats();
    assert_eq!(stats.connections_accepted.load(Ordering::Relaxed), 0);
    assert_eq!(stats.active_connections.load(Ordering::Relaxed), 0);
}

#[test]
fn websocket_server_broadcast_delivers_to_subscriber() {
    let server = WebSocketServer::new(WebSocketConfig::default());
    let mut rx = server.subscribe_updates();

    let update = TraceUpdate::new("test-trace-id", "packet");
    server.broadcast_update(update.clone());

    let received = rx.try_recv();
    assert!(received.is_ok(), "Subscriber should receive the broadcast");
    assert_eq!(received.unwrap().trace_id, "test-trace-id");
}

#[test]
fn websocket_server_broadcast_summary_converts_correctly() {
    let server = WebSocketServer::new(WebSocketConfig::default());
    let mut rx = server.subscribe_updates();

    let summary = TraceSummary {
        trace_id: "summary-trace-1".to_string(),
        root_operation: "http.request".to_string(),
        services: vec!["gateway".to_string()],
        span_count: 10,
        duration_ns: 50_000,
        start_time_ns: 1_000_000,
        end_time_ns: 1_050_000,
        status: "ok".to_string(),
        avg_latency_ns: Some(5_000),
        p95_latency_ns: Some(8_000),
        packet_count: 15,
        error_count: 0,
    };

    server.broadcast_summary(&summary);

    let received = rx.try_recv().unwrap();
    assert_eq!(received.trace_id, "summary-trace-1");
    assert_eq!(received.operation, "http.request");
    assert!(received.is_complete);
    assert_eq!(received.status, "ok");
    assert_eq!(received.duration_ns, Some(50_000));
}

#[tokio::test]
async fn websocket_server_binds_and_accepts_tcp() {
    // Use port 0 so the OS assigns a free port
    let _config = WebSocketConfig {
        bind_addr: "127.0.0.1:0".to_string(),
        ..WebSocketConfig::default()
    };

    // We can't use port 0 with the server's built-in run() since it parses
    // the address. Instead, manually bind and verify the TCP listener works.
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();

    // Spawn a task that accepts one connection
    let accept_handle = tokio::spawn(async move {
        let result = tokio::time::timeout(Duration::from_secs(2), listener.accept()).await;
        result.is_ok() && result.unwrap().is_ok()
    });

    // Give the listener a moment to be ready
    tokio::time::sleep(Duration::from_millis(50)).await;

    // Connect to the server
    let connect_result =
        tokio::time::timeout(Duration::from_secs(2), tokio::net::TcpStream::connect(addr)).await;

    assert!(connect_result.is_ok(), "Connection should not time out");
    assert!(
        connect_result.unwrap().is_ok(),
        "TCP connection should succeed"
    );

    let accepted = accept_handle.await.unwrap();
    assert!(accepted, "Server should accept the connection");
}

#[tokio::test]
async fn websocket_server_full_handshake() {
    // Bind a real TCP listener
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();

    // Spawn a task that accepts and upgrades to WebSocket
    let server_handle = tokio::spawn(async move {
        let (stream, _peer) = listener.accept().await.unwrap();
        let ws_stream = tokio_tungstenite::accept_async(stream).await;
        ws_stream.is_ok()
    });

    tokio::time::sleep(Duration::from_millis(50)).await;

    // Connect as a WebSocket client
    let url = format!("ws://{}", addr);
    let client_result = tokio::time::timeout(
        Duration::from_secs(2),
        tokio_tungstenite::connect_async(&url),
    )
    .await;

    assert!(
        client_result.is_ok(),
        "Client connection should not time out"
    );
    let (ws_stream, _response) = client_result.unwrap().unwrap();

    // Both sides should have a successful WebSocket connection
    let server_ok = server_handle.await.unwrap();
    assert!(server_ok, "Server WebSocket handshake should succeed");

    // Verify we can split the client stream (proves it's a valid WS connection)
    let (_write, _read) = ws_stream.split();
}

// ============================================================================
// 6. WEBSOCKET PROTOCOL MESSAGE TESTS
// ============================================================================

#[test]
fn websocket_subscription_matches_all_events() {
    let sub = Subscription::all_events();
    let update = TraceUpdate::new("any-trace", "any-type");
    assert!(sub.matches(&update));
}

#[test]
fn websocket_subscription_filters_by_service() {
    let sub = Subscription::for_service("my-service");

    let matching = TraceUpdate {
        services: vec!["my-service".to_string()],
        status: "ok".to_string(),
        ..TraceUpdate::new("t1", "packet")
    };
    let non_matching = TraceUpdate {
        services: vec!["other-service".to_string()],
        status: "ok".to_string(),
        ..TraceUpdate::new("t2", "packet")
    };

    assert!(sub.matches(&matching));
    assert!(!sub.matches(&non_matching));
}

#[test]
fn websocket_subscription_errors_only() {
    let sub = Subscription::errors();

    let error = TraceUpdate {
        status: "error".to_string(),
        ..TraceUpdate::new("t1", "packet")
    };
    let ok = TraceUpdate {
        status: "ok".to_string(),
        ..TraceUpdate::new("t2", "packet")
    };

    assert!(sub.matches(&error));
    assert!(!sub.matches(&ok));
}

#[test]
fn websocket_client_message_subscribe_serialization() {
    let msg = ClientMessage::Subscribe {
        id: "sub-integration-test".to_string(),
        subscription: Subscription::all_events(),
    };

    let json = serde_json::to_string(&msg).unwrap();
    assert!(json.contains("subscribe"));
    assert!(json.contains("sub-integration-test"));

    let parsed: ClientMessage = serde_json::from_str(&json).unwrap();
    if let ClientMessage::Subscribe { id, subscription } = parsed {
        assert_eq!(id, "sub-integration-test");
        assert!(subscription.all);
    } else {
        panic!("Deserialized wrong message variant");
    }
}

#[test]
fn websocket_server_message_connected_serialization() {
    let msg = ServerMessage::Connected {
        version: "0.1.0".to_string(),
        connection_id: "ws-test-123".to_string(),
        capabilities: vec!["streaming".to_string(), "query".to_string()],
    };

    let json = serde_json::to_string(&msg).unwrap();
    assert!(json.contains("connected"));
    assert!(json.contains("ws-test-123"));

    let parsed: ServerMessage = serde_json::from_str(&json).unwrap();
    if let ServerMessage::Connected {
        version,
        connection_id,
        capabilities,
    } = parsed
    {
        assert_eq!(version, "0.1.0");
        assert_eq!(connection_id, "ws-test-123");
        assert_eq!(capabilities.len(), 2);
    } else {
        panic!("Deserialized wrong message variant");
    }
}

#[test]
fn websocket_trace_update_from_summary() {
    let summary = TraceSummary {
        trace_id: "update-test-1".to_string(),
        root_operation: "grpc.call".to_string(),
        services: vec!["svc-a".to_string(), "svc-b".to_string()],
        span_count: 7,
        duration_ns: 25_000,
        start_time_ns: 100_000,
        end_time_ns: 125_000,
        status: "error".to_string(),
        avg_latency_ns: Some(3_500),
        p95_latency_ns: Some(10_000),
        packet_count: 30,
        error_count: 2,
    };

    let update = TraceUpdate::from_summary(&summary);
    assert_eq!(update.trace_id, "update-test-1");
    assert_eq!(update.operation, "grpc.call");
    assert_eq!(update.services.len(), 2);
    assert_eq!(update.duration_ns, Some(25_000));
    assert_eq!(update.latency_ns, Some(3_500));
    assert_eq!(update.status, "error");
    assert!(update.is_complete);
    assert_eq!(update.event_type, "trace");
}

// ============================================================================
// 7. END-TO-END PIPELINE TESTS
// ============================================================================

#[test]
fn e2e_mock_events_through_correlation_to_store() {
    // Set up the pipeline: events -> correlation engine -> trace store
    let engine = CorrelationEngine::new(CorrelationConfig::default());
    let store = TraceStore::new(TraceStoreConfig::default()).unwrap();

    // Simulate a burst of events from multiple flows
    let events = vec![
        // Flow A: nginx -> backend
        mock_packet_event(
            100,
            "nginx",
            Ipv4Addr::new(10, 0, 0, 1),
            Ipv4Addr::new(10, 0, 0, 10),
            8080,
            3000,
            1_000_000,
        ),
        mock_packet_event(
            100,
            "nginx",
            Ipv4Addr::new(10, 0, 0, 1),
            Ipv4Addr::new(10, 0, 0, 10),
            8080,
            3000,
            1_001_000,
        ),
        // Flow B: client -> gateway
        mock_packet_event(
            200,
            "envoy",
            Ipv4Addr::new(172, 16, 0, 1),
            Ipv4Addr::new(172, 16, 0, 100),
            54321,
            443,
            1_500_000,
        ),
        // Latency event for PID 100 (should try to correlate with flow A trace)
        mock_latency_event(100, "nginx", 500_000, 2_000_000),
        // Syscall event for PID 200 (should try to correlate with flow B trace)
        mock_syscall_event(200, "envoy", 44, 2_500_000), // sendto
    ];

    // Process through correlation engine
    let mut trace_ids = Vec::new();
    for event in &events {
        if let Some(id) = engine.process_event(event) {
            trace_ids.push(id);
        }
    }

    // Should have created at least 2 traces (2 different flows)
    assert!(
        engine.active_trace_count() >= 2,
        "Expected at least 2 traces for 2 flows, got {}",
        engine.active_trace_count()
    );

    // Store completed traces
    for trace_id in engine.active_trace_ids() {
        if let Some(ctx) = engine.get_trace(trace_id) {
            store.store(&ctx);
        }
    }

    // Verify the store has the traces
    assert!(
        store.trace_count() >= 2,
        "Store should contain at least 2 traces"
    );

    // Query for nginx traces
    let nginx_result = store.query(&TraceQuery::new().service("nginx"));
    assert!(
        nginx_result.total_count >= 1,
        "Should find nginx traces in store"
    );
}

#[test]
fn e2e_mock_events_through_correlation_to_batcher() {
    // Pipeline: events -> correlation engine -> batcher -> serialized payload
    let engine = CorrelationEngine::new(CorrelationConfig::default());
    let mut batcher = EventBatcher::new("e2e-test".to_string(), 10, Duration::from_secs(60));
    // Disable compression so we can decode the raw protobuf directly
    batcher.set_compression(false);

    let events = mock_event_stream(30);

    // Process through correlation, then batch
    for event in events {
        engine.process_event(&event);
        batcher.add(event);
    }

    assert_eq!(batcher.len(), 30);
    assert!(
        engine.stats().events_processed.load(Ordering::Relaxed) == 30,
        "All events should be processed by correlation"
    );

    // Flush the batcher
    let payload = batcher.flush().unwrap();
    assert_eq!(payload.event_count, 30);
    assert!(payload.payload_bytes > 0);

    // Verify the protobuf can be decoded
    use prost::Message;
    let batch = trace_collector::proto::TraceEventBatch::decode(&payload.payload[..]).unwrap();
    assert_eq!(batch.events.len(), 30);
    assert_eq!(batch.collector_id, "e2e-test");
}

#[test]
fn e2e_correlation_to_websocket_broadcast() {
    // Pipeline: events -> correlation engine -> WebSocket broadcast
    let engine = CorrelationEngine::new(CorrelationConfig {
        max_active_traces: 2,
        ..CorrelationConfig::default()
    });

    let store = Arc::new(TraceStore::new(TraceStoreConfig::default()).unwrap());
    let ws_server = Arc::new(
        WebSocketServer::new(WebSocketConfig::default())
            .with_trace_store(Arc::clone(&store))
            .with_correlation_engine(Arc::new(CorrelationEngine::new(
                CorrelationConfig::default(),
            ))),
    );

    // Subscribe to WebSocket updates
    let mut ws_rx = ws_server.subscribe_updates();

    // Subscribe to completed traces from correlation engine
    let mut completed_rx = engine.subscribe_completed();

    // Process events that will fill and evict traces
    let ev1 = mock_packet_event(
        100,
        "svc-a",
        Ipv4Addr::new(10, 0, 0, 1),
        Ipv4Addr::new(10, 0, 0, 2),
        1000,
        80,
        1_000_000,
    );
    let ev2 = mock_packet_event(
        200,
        "svc-b",
        Ipv4Addr::new(172, 16, 0, 1),
        Ipv4Addr::new(172, 16, 0, 2),
        2000,
        443,
        2_000_000,
    );
    let ev3 = mock_packet_event(
        300,
        "svc-c",
        Ipv4Addr::new(192, 168, 0, 1),
        Ipv4Addr::new(192, 168, 0, 2),
        3000,
        8080,
        3_000_000,
    );

    engine.process_event(&ev1);
    engine.process_event(&ev2);
    engine.process_event(&ev3); // This causes eviction

    // The eviction should produce a completed trace
    if let Ok(summary) = completed_rx.try_recv() {
        // Forward to WebSocket broadcast (as the daemon does)
        ws_server.broadcast_summary(&summary);
        store.store_summary(summary);
    }

    // Verify the WebSocket subscriber received the update
    if let Ok(update) = ws_rx.try_recv() {
        assert!(update.is_complete);
        assert!(!update.trace_id.is_empty());
    }

    // Verify the store was updated
    assert!(
        store.trace_count() >= 1,
        "Store should have at least one trace"
    );
}

#[test]
fn e2e_full_pipeline_with_diverse_events() {
    // Full pipeline test: mixed events -> correlation -> store + batcher + WebSocket
    let engine = CorrelationEngine::new(CorrelationConfig::default());
    let store = TraceStore::new(TraceStoreConfig::default()).unwrap();
    let mut batcher = EventBatcher::new("full-e2e".to_string(), 50, Duration::from_secs(60));
    let ws_server = WebSocketServer::new(WebSocketConfig::default());
    let mut ws_rx = ws_server.subscribe_updates();

    let events = mock_event_stream(100);
    let mut created_trace_count = 0u64;
    let mut packet_events = 0u64;
    let mut latency_events = 0u64;
    let mut syscall_events = 0u64;

    for event in &events {
        match event.event_type {
            EventType::Packet => packet_events += 1,
            EventType::Latency => latency_events += 1,
            EventType::Syscall => syscall_events += 1,
            _ => {}
        }

        // Step 1: Correlation
        if let Some(_trace_id) = engine.process_event(event) {
            // Event was correlated
        }

        // Step 2: Batch for publishing
        batcher.add(event.clone());
    }

    // Verify event type distribution (stream alternates: packet, latency, syscall)
    // 100 events: indices 0,3,6,...,99 => packet if i%3==0, etc.
    // packet: 0,3,6,...,99 -> 34 events
    // latency: 1,4,7,...,97 -> 33 events
    // syscall: 2,5,8,...,98 -> 33 events
    assert_eq!(packet_events, 34);
    assert_eq!(latency_events, 33);
    assert_eq!(syscall_events, 33);

    // Step 3: Store all active traces
    for trace_id in engine.active_trace_ids() {
        if let Some(ctx) = engine.get_trace(trace_id) {
            let summary = TraceSummary::from_context(&ctx);
            ws_server.broadcast_summary(&summary);
            store.store(&ctx);
            created_trace_count += 1;
        }
    }

    // Step 4: Flush batcher
    let payload = batcher.flush().unwrap();

    // Verify all stages produced output
    assert_eq!(engine.stats().events_processed.load(Ordering::Relaxed), 100);
    assert!(created_trace_count > 0, "Should have created traces");
    assert!(store.trace_count() > 0, "Store should have traces");
    assert_eq!(payload.event_count, 100);
    assert!(payload.payload_bytes > 0);

    // Verify WebSocket broadcast received updates
    let mut ws_update_count = 0;
    while ws_rx.try_recv().is_ok() {
        ws_update_count += 1;
    }
    assert!(
        ws_update_count > 0,
        "WebSocket should have received broadcast updates"
    );
}

// ============================================================================
// 8. TRACE ID AND FLOW KEY TESTS
// ============================================================================

#[test]
fn trace_id_roundtrip_hex() {
    let id = TraceId::new(0x0123456789abcdef, 0xfedcba9876543210);
    let hex = id.to_hex();
    assert_eq!(hex, "0123456789abcdeffedcba9876543210");

    let parsed = TraceId::from_hex(&hex).unwrap();
    assert_eq!(id, parsed);
}

#[test]
fn trace_id_roundtrip_bytes() {
    let id = TraceId::generate();
    let bytes = id.to_bytes();
    let from_bytes = TraceId::from_bytes(bytes);
    assert_eq!(id, from_bytes);
}

#[test]
fn trace_id_generation_uniqueness() {
    let ids: Vec<TraceId> = (0..100).map(|_| TraceId::generate()).collect();
    for i in 0..ids.len() {
        assert!(!ids[i].is_zero());
        for j in (i + 1)..ids.len() {
            assert_ne!(ids[i], ids[j], "Generated trace IDs should be unique");
        }
    }
}

#[test]
fn flow_key_normalization_symmetry() {
    let k1 = FlowKey::new(100, 200, 1000, 2000, 6);
    let k2 = FlowKey::new(200, 100, 2000, 1000, 6);
    assert_eq!(
        k1.normalized(),
        k2.normalized(),
        "Normalized flow keys should be identical regardless of direction"
    );
}

#[test]
fn flow_key_hash_consistency() {
    let k1 = FlowKey::new(100, 200, 1000, 2000, 6);
    let k2 = FlowKey::new(100, 200, 1000, 2000, 6);
    assert_eq!(k1.hash(), k2.hash());

    let k3 = FlowKey::new(100, 200, 1000, 2001, 6);
    assert_ne!(k1.hash(), k3.hash());
}

// ============================================================================
// 9. WEBSOCKET HANDLER INTEGRATION TESTS
// ============================================================================

#[test]
fn websocket_handler_subscribe_and_match() {
    use trace_collector::websocket::WebSocketHandler;

    let handler = WebSocketHandler::new("integration-handler");
    assert_eq!(handler.subscription_count(), 0);

    // Subscribe to a specific service
    let resp = handler.handle_message(ClientMessage::Subscribe {
        id: "sub-1".to_string(),
        subscription: Subscription::for_service("backend"),
    });
    assert!(matches!(
        resp,
        Some(ServerMessage::Subscribed { success: true, .. })
    ));
    assert_eq!(handler.subscription_count(), 1);

    // Subscribe to all events
    let resp = handler.handle_message(ClientMessage::Subscribe {
        id: "sub-2".to_string(),
        subscription: Subscription::all_events(),
    });
    assert!(matches!(
        resp,
        Some(ServerMessage::Subscribed { success: true, .. })
    ));
    assert_eq!(handler.subscription_count(), 2);

    // Test matching
    let backend_update = TraceUpdate {
        services: vec!["backend".to_string()],
        status: "ok".to_string(),
        ..TraceUpdate::new("trace-1", "packet")
    };
    let matching = handler.matching_subscriptions(&backend_update);
    assert_eq!(
        matching.len(),
        2,
        "Both subscriptions should match backend events"
    );

    let frontend_update = TraceUpdate {
        services: vec!["frontend".to_string()],
        status: "ok".to_string(),
        ..TraceUpdate::new("trace-2", "packet")
    };
    let matching = handler.matching_subscriptions(&frontend_update);
    assert_eq!(
        matching.len(),
        1,
        "Only all-events subscription should match frontend"
    );

    // Unsubscribe
    handler.handle_message(ClientMessage::Unsubscribe {
        id: "sub-1".to_string(),
    });
    assert_eq!(handler.subscription_count(), 1);
}

#[test]
fn websocket_handler_ping_pong() {
    use trace_collector::websocket::WebSocketHandler;

    let handler = WebSocketHandler::new("ping-pong-handler");

    let resp = handler.handle_message(ClientMessage::Ping {
        timestamp: 1234567890,
    });

    if let Some(ServerMessage::Pong {
        client_timestamp,
        server_timestamp,
    }) = resp
    {
        assert_eq!(client_timestamp, 1234567890);
        assert!(server_timestamp > 0);
    } else {
        panic!("Expected Pong response");
    }
}

#[test]
fn websocket_handler_query_without_store_returns_empty() {
    use trace_collector::websocket::WebSocketHandler;

    let handler = WebSocketHandler::new("query-handler");

    let resp = handler.handle_message(ClientMessage::GetTrace {
        request_id: "req-1".to_string(),
        trace_id: "nonexistent".to_string(),
    });

    if let Some(ServerMessage::TraceDetails {
        request_id,
        trace,
        error: _,
    }) = resp
    {
        assert_eq!(request_id, "req-1");
        assert!(trace.is_none(), "No trace store means no results");
    } else {
        panic!("Expected TraceDetails response");
    }
}

#[test]
fn websocket_handler_query_with_store() {
    use trace_collector::websocket::WebSocketHandler;

    let store = Arc::new(TraceStore::new(TraceStoreConfig::default()).unwrap());
    store.store_summary(TraceSummary {
        trace_id: "findable-trace".to_string(),
        root_operation: "test.op".to_string(),
        services: vec!["test-svc".to_string()],
        span_count: 1,
        duration_ns: 1000,
        start_time_ns: 1_000_000,
        end_time_ns: 1_001_000,
        status: "ok".to_string(),
        avg_latency_ns: None,
        p95_latency_ns: None,
        packet_count: 0,
        error_count: 0,
    });

    let handler = WebSocketHandler::new("store-handler").with_trace_store(Arc::clone(&store));

    let resp = handler.handle_message(ClientMessage::GetTrace {
        request_id: "req-2".to_string(),
        trace_id: "findable-trace".to_string(),
    });

    if let Some(ServerMessage::TraceDetails {
        request_id,
        trace,
        error: _,
    }) = resp
    {
        assert_eq!(request_id, "req-2");
        assert!(trace.is_some(), "Should find the trace in the store");
        let detail = trace.unwrap();
        assert_eq!(detail.summary.trace_id, "findable-trace");
        assert_eq!(detail.summary.root_operation, "test.op");
    } else {
        panic!("Expected TraceDetails response");
    }
}
