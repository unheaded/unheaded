package main

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"unheaded/pkg/ebpf"
)

// ── TracePublisher creation tests ─────────────────────────────────────────

func TestNewTracePublisher(t *testing.T) {
	config := DefaultTracePublisherConfig()
	pub := NewTracePublisher(config)

	if pub == nil {
		t.Fatal("NewTracePublisher returned nil")
	}
	if pub.IsConnected() {
		t.Error("new publisher should not be connected")
	}
}

func TestDefaultTracePublisherConfig(t *testing.T) {
	config := DefaultTracePublisherConfig()

	if config.WotanAddr != "localhost:9090" {
		t.Errorf("WotanAddr = %q, want \"localhost:9090\"", config.WotanAddr)
	}
	if config.BatchSize != 100 {
		t.Errorf("BatchSize = %d, want 100", config.BatchSize)
	}
	if config.FlushInterval != 50*time.Millisecond {
		t.Errorf("FlushInterval = %v, want 50ms", config.FlushInterval)
	}
}

// ── PublishPacketEvent tests ──────────────────────────────────────────────

func TestTracePublisher_PublishPacketEvent(t *testing.T) {
	config := DefaultTracePublisherConfig()
	config.BatchSize = 1000 // Large batch to prevent auto-flush
	pub := NewTracePublisher(config)

	te := makeTraceEntry(
		net.ParseIP("10.0.0.1"),
		net.ParseIP("10.0.0.2"),
		8080, 443, 6, uint64(time.Now().UnixNano()),
	)

	err := pub.PublishPacketEvent(te)
	if err != nil {
		t.Fatalf("PublishPacketEvent: %v", err)
	}

	stats := pub.Stats()
	if stats.PacketEvents != 1 {
		t.Errorf("PacketEvents = %d, want 1", stats.PacketEvents)
	}
}

func TestTracePublisher_PublishMultiplePacketEvents(t *testing.T) {
	config := DefaultTracePublisherConfig()
	config.BatchSize = 1000
	pub := NewTracePublisher(config)

	for i := 0; i < 10; i++ {
		te := makeTraceEntry(
			net.ParseIP("10.0.0.1"),
			net.ParseIP("10.0.0.2"),
			uint16(8080+i), 443, 6, uint64(i*1000),
		)
		if err := pub.PublishPacketEvent(te); err != nil {
			t.Fatalf("PublishPacketEvent[%d]: %v", i, err)
		}
	}

	stats := pub.Stats()
	if stats.PacketEvents != 10 {
		t.Errorf("PacketEvents = %d, want 10", stats.PacketEvents)
	}
}

// ── PublishAnamnesisEvent tests ───────────────────────────────────────────

func TestTracePublisher_PublishAnamnesisEvent(t *testing.T) {
	config := DefaultTracePublisherConfig()
	config.BatchSize = 1000
	pub := NewTracePublisher(config)

	ev := &ebpf.AnamnesisEvent{
		TimestampNs: uint64(time.Now().UnixNano()),
		EventType:   ebpf.EventBirth,
		HopID:       1,
		FlowLabelLo: 0x1234,
		Monad: ebpf.MonadState{
			Version: ebpf.MonadVersion,
		},
	}

	err := pub.PublishAnamnesisEvent(ev)
	if err != nil {
		t.Fatalf("PublishAnamnesisEvent: %v", err)
	}
}

// ── PublishFlowEvent tests ───────────────────────────────────────────────

func TestTracePublisher_PublishFlowEvent(t *testing.T) {
	config := DefaultTracePublisherConfig()
	config.BatchSize = 1000
	pub := NewTracePublisher(config)

	payload, _ := json.Marshal(map[string]string{"type": "flow", "action": "new"})
	pub.PublishFlowEvent(payload)

	stats := pub.Stats()
	if stats.FlowEvents != 1 {
		t.Errorf("FlowEvents = %d, want 1", stats.FlowEvents)
	}
}

// ── PublishLatencyEvent tests ────────────────────────────────────────────

func TestTracePublisher_PublishLatencyEvent(t *testing.T) {
	config := DefaultTracePublisherConfig()
	config.BatchSize = 1000
	pub := NewTracePublisher(config)

	payload, _ := json.Marshal(map[string]interface{}{"rtt_ns": 5000000})
	pub.PublishLatencyEvent(payload)

	stats := pub.Stats()
	if stats.LatencyEvents != 1 {
		t.Errorf("LatencyEvents = %d, want 1", stats.LatencyEvents)
	}
}

// ── PublishCorrelatedFlow tests ──────────────────────────────────────────

func TestTracePublisher_PublishCorrelatedFlow(t *testing.T) {
	config := DefaultTracePublisherConfig()
	config.BatchSize = 1000
	pub := NewTracePublisher(config)

	flow := &CorrelatedFlow{
		Forward: &TrackedFlow{
			PacketCount: 5,
			ByteCount:   640,
		},
		Reverse: &TrackedFlow{
			PacketCount: 3,
			ByteCount:   384,
		},
		RTTNS:   5000000,
		Matched: true,
	}

	err := pub.PublishCorrelatedFlow(flow)
	if err != nil {
		t.Fatalf("PublishCorrelatedFlow: %v", err)
	}

	stats := pub.Stats()
	if stats.CorrelatedSent != 1 {
		t.Errorf("CorrelatedSent = %d, want 1", stats.CorrelatedSent)
	}
}

// ── Flush tests ─────────────────────────────────────────────────────────

func TestTracePublisher_Flush(t *testing.T) {
	config := DefaultTracePublisherConfig()
	config.BatchSize = 1000 // Large batch, won't auto-flush
	pub := NewTracePublisher(config)

	// Enqueue some events
	for i := 0; i < 5; i++ {
		te := makeTraceEntry(
			net.ParseIP("10.0.0.1"),
			net.ParseIP("10.0.0.2"),
			uint16(8080+i), 443, 6, uint64(i*1000),
		)
		pub.PublishPacketEvent(te)
	}

	// Flush manually
	pub.Flush(context.Background())

	stats := pub.Stats()
	if stats.BatchesSent != 1 {
		t.Errorf("BatchesSent = %d, want 1", stats.BatchesSent)
	}
	if stats.EventsSent != 5 {
		t.Errorf("EventsSent = %d, want 5", stats.EventsSent)
	}
	if stats.FlushCycles != 1 {
		t.Errorf("FlushCycles = %d, want 1", stats.FlushCycles)
	}
}

func TestTracePublisher_AutoFlushOnBatchSize(t *testing.T) {
	config := DefaultTracePublisherConfig()
	config.BatchSize = 3 // Small batch for auto-flush testing
	pub := NewTracePublisher(config)

	// Enqueue exactly BatchSize events to trigger auto-flush
	for i := 0; i < 3; i++ {
		te := makeTraceEntry(
			net.ParseIP("10.0.0.1"),
			net.ParseIP("10.0.0.2"),
			uint16(8080+i), 443, 6, uint64(i*1000),
		)
		pub.PublishPacketEvent(te)
	}

	stats := pub.Stats()
	// The batch should have been flushed automatically
	if stats.BatchesSent < 1 {
		t.Errorf("BatchesSent = %d, want >= 1 (auto-flush)", stats.BatchesSent)
	}
}

func TestTracePublisher_FlushEmpty(t *testing.T) {
	config := DefaultTracePublisherConfig()
	pub := NewTracePublisher(config)

	// Flush with nothing enqueued
	pub.Flush(context.Background())

	stats := pub.Stats()
	if stats.BatchesSent != 0 {
		t.Errorf("BatchesSent = %d, want 0 (nothing to flush)", stats.BatchesSent)
	}
	if stats.FlushCycles != 1 {
		t.Errorf("FlushCycles = %d, want 1", stats.FlushCycles)
	}
}

// ── Multi-topic batching tests ───────────────────────────────────────────

func TestTracePublisher_MultiTopicFlush(t *testing.T) {
	config := DefaultTracePublisherConfig()
	config.BatchSize = 1000
	pub := NewTracePublisher(config)

	// Publish to different topics
	te := makeTraceEntry(
		net.ParseIP("10.0.0.1"),
		net.ParseIP("10.0.0.2"),
		8080, 443, 6, 1000,
	)
	pub.PublishPacketEvent(te)

	flowPayload, _ := json.Marshal(map[string]string{"type": "flow"})
	pub.PublishFlowEvent(flowPayload)

	latencyPayload, _ := json.Marshal(map[string]interface{}{"rtt_ns": 1000})
	pub.PublishLatencyEvent(latencyPayload)

	// Flush all
	pub.Flush(context.Background())

	stats := pub.Stats()
	// Should have flushed 3 batches (one per topic)
	if stats.BatchesSent != 3 {
		t.Errorf("BatchesSent = %d, want 3 (one per topic)", stats.BatchesSent)
	}
	if stats.EventsSent != 3 {
		t.Errorf("EventsSent = %d, want 3", stats.EventsSent)
	}
}

// ── Connected state tests ────────────────────────────────────────────────

func TestTracePublisher_ConnectedAfterFlush(t *testing.T) {
	config := DefaultTracePublisherConfig()
	config.BatchSize = 1000
	pub := NewTracePublisher(config)

	if pub.IsConnected() {
		t.Error("should not be connected before any flush")
	}

	te := makeTraceEntry(
		net.ParseIP("10.0.0.1"),
		net.ParseIP("10.0.0.2"),
		8080, 443, 6, 1000,
	)
	pub.PublishPacketEvent(te)
	pub.Flush(context.Background())

	if !pub.IsConnected() {
		t.Error("should be connected after successful flush")
	}
}

// ── marshalTraceEntry tests ─────────────────────────────────────────────

func TestMarshalTraceEntry(t *testing.T) {
	te := makeTraceEntry(
		net.ParseIP("10.10.10.1"),
		net.ParseIP("10.10.10.2"),
		8080, 443, 6, 1234567890,
	)

	data, err := marshalTraceEntry(te)
	if err != nil {
		t.Fatalf("marshalTraceEntry: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if decoded["protocol"] != "TCP" {
		t.Errorf("protocol = %v, want \"TCP\"", decoded["protocol"])
	}
	if decoded["src_port"] != float64(8080) {
		t.Errorf("src_port = %v, want 8080", decoded["src_port"])
	}
	if decoded["dst_port"] != float64(443) {
		t.Errorf("dst_port = %v, want 443", decoded["dst_port"])
	}
}

// ── anamnesisTopicForEvent tests ─────────────────────────────────────────

func TestAnamnesisTopicForEvent(t *testing.T) {
	tests := []struct {
		et   ebpf.EventType
		want string
	}{
		{ebpf.EventBirth, TopicBirth},
		{ebpf.EventHop, TopicHop},
		{ebpf.EventDeath, TopicDeath},
		{ebpf.EventAnomaly, TopicAnomaly},
		{ebpf.EventChaos, TopicChaos},
	}
	for _, tt := range tests {
		if got := anamnesisTopicForEvent(tt.et); got != tt.want {
			t.Errorf("anamnesisTopicForEvent(%v) = %q, want %q", tt.et, got, tt.want)
		}
	}
}

// ── TracePublisher.Run tests ─────────────────────────────────────────────

func TestTracePublisher_RunAndShutdown(t *testing.T) {
	config := DefaultTracePublisherConfig()
	config.FlushInterval = 10 * time.Millisecond
	config.BatchSize = 1000
	pub := NewTracePublisher(config)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		pub.Run(ctx)
		close(done)
	}()

	// Enqueue some events
	te := makeTraceEntry(
		net.ParseIP("10.0.0.1"),
		net.ParseIP("10.0.0.2"),
		8080, 443, 6, 1000,
	)
	pub.PublishPacketEvent(te)

	// Wait for at least one flush cycle
	time.Sleep(50 * time.Millisecond)

	// Shutdown
	cancel()
	select {
	case <-done:
		// Good
	case <-time.After(2 * time.Second):
		t.Fatal("publisher.Run did not return after cancel")
	}

	stats := pub.Stats()
	if stats.FlushCycles == 0 {
		t.Error("expected at least 1 flush cycle")
	}
}

// ── Topic constant tests ────────────────────────────────────────────────

func TestTopicConstants(t *testing.T) {
	if TopicTracesPacket != "traces.packet" {
		t.Errorf("TopicTracesPacket = %q", TopicTracesPacket)
	}
	if TopicTracesFlow != "traces.flow" {
		t.Errorf("TopicTracesFlow = %q", TopicTracesFlow)
	}
	if TopicTracesLatency != "traces.latency" {
		t.Errorf("TopicTracesLatency = %q", TopicTracesLatency)
	}
	if TopicTracesCorrelated != "traces.correlated" {
		t.Errorf("TopicTracesCorrelated = %q", TopicTracesCorrelated)
	}
}
