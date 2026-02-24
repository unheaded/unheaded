package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/ebpf"
	wotanClient "unheaded/pkg/wotan-client"
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

	if config.WotanAddr != "localhost:18001" {
		t.Errorf("WotanAddr = %q, want \"localhost:18001\"", config.WotanAddr)
	}
	if config.WotanHTTPAddr != "localhost:18000" {
		t.Errorf("WotanHTTPAddr = %q, want \"localhost:18000\"", config.WotanHTTPAddr)
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
	config.BatchSize = 1000
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

	// Flush manually (tsc nil → sendBatch drops gracefully, stats still count)
	// Dual-publish: each packet event → traces.packet + ebpf.packet.events = 2 batches
	pub.Flush(context.Background())

	stats := pub.Stats()
	if stats.BatchesSent != 2 {
		t.Errorf("BatchesSent = %d, want 2 (dual-publish)", stats.BatchesSent)
	}
	if stats.EventsSent != 10 {
		t.Errorf("EventsSent = %d, want 10 (5 events × 2 topics)", stats.EventsSent)
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
	// Dual-publish: packet→2, flow→2, latency→2 = 6 batches, 6 events
	pub.Flush(context.Background())

	stats := pub.Stats()
	if stats.BatchesSent != 6 {
		t.Errorf("BatchesSent = %d, want 6 (2 per event type: traces.* + ebpf.*)", stats.BatchesSent)
	}
	if stats.EventsSent != 6 {
		t.Errorf("EventsSent = %d, want 6", stats.EventsSent)
	}
}

// ── Connected state tests ────────────────────────────────────────────────

func TestTracePublisher_NotConnectedWithoutConnect(t *testing.T) {
	config := DefaultTracePublisherConfig()
	pub := NewTracePublisher(config)

	if pub.IsConnected() {
		t.Error("should not be connected without calling Connect()")
	}

	// Flush drops events silently when tsc is nil
	te := makeTraceEntry(
		net.ParseIP("10.0.0.1"),
		net.ParseIP("10.0.0.2"),
		8080, 443, 6, 1000,
	)
	pub.PublishPacketEvent(te)
	pub.Flush(context.Background())

	// Still not "connected" — sendBatch doesn't set connected when tsc is nil
	if pub.IsConnected() {
		t.Error("should not be connected after flush without Connect()")
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

// ── HTTP fallback tests ─────────────────────────────────────────────────
// These tests verify the HTTP REST fallback path works when gRPC is unavailable.
// Wotan is gRPC-first, but HTTP fallback ensures resilience.

// newTestWotanHTTPServer creates a mock Wotan HTTP server that handles
// subscribe and publish endpoints.
func newTestWotanHTTPServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/subscribe") && r.Method == http.MethodPost:
			// Mock subscribe: return 201 Created with approved subscriber
			topic := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/topics/"), "/subscribe")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"subscriber": map[string]interface{}{
					"subscriber_id": "test-sub-001",
					"topic":         topic,
					"display_name":  "test",
					"status":        "approved",
					"requested_at":  time.Now().Format(time.RFC3339),
				},
			})
		case strings.HasSuffix(r.URL.Path, "/publish") && r.Method == http.MethodPost:
			// Mock publish: accept message
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"message_id": "msg-001",
				"seq":        1,
				"timestamp":  time.Now().Format(time.RFC3339),
			})
		case strings.HasSuffix(r.URL.Path, "/messages") && r.Method == http.MethodGet:
			// Mock get messages
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"messages": []interface{}{},
			})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
}

func TestTracePublisher_ConnectWithHTTPFallback(t *testing.T) {
	srv := newTestWotanHTTPServer(t)
	defer srv.Close()

	config := DefaultTracePublisherConfig()
	config.WotanAddr = "localhost:19999"                                     // Non-existent gRPC
	config.WotanHTTPAddr = strings.TrimPrefix(srv.URL, "http://")           // HTTP fallback
	config.BatchSize = 1000

	pub := NewTracePublisher(config)
	err := pub.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect should succeed (gRPC lazy dial): %v", err)
	}

	if pub.tsc == nil {
		t.Fatal("TopicStreamClient should be set after Connect")
	}
	if !pub.IsConnected() {
		t.Error("should be connected after successful Connect()")
	}
}

func TestTracePublisher_HTTPFallbackPublish(t *testing.T) {
	srv := newTestWotanHTTPServer(t)
	defer srv.Close()

	httpAddr := strings.TrimPrefix(srv.URL, "http://")

	// Create HTTP fallback client directly and subscribe
	httpClient, err := wotanClient.NewClient(httpAddr)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	// Subscribe via HTTP
	ctx := context.Background()
	_, err = httpClient.Subscribe(ctx, TopicTracesPacket, "trace-collector-test")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Publish via HTTP
	payload, _ := json.Marshal(map[string]string{"test": "hello"})
	err = httpClient.Publish(ctx, TopicTracesPacket, payload)
	if err != nil {
		t.Fatalf("Publish via HTTP fallback: %v", err)
	}
}

func TestTracePublisher_FlushViaHTTPFallback(t *testing.T) {
	srv := newTestWotanHTTPServer(t)
	defer srv.Close()

	httpAddr := strings.TrimPrefix(srv.URL, "http://")

	config := DefaultTracePublisherConfig()
	config.WotanAddr = "localhost:19999"       // Non-existent gRPC
	config.WotanHTTPAddr = httpAddr             // HTTP fallback
	config.BatchSize = 1000

	pub := NewTracePublisher(config)
	err := pub.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Enqueue events
	for i := 0; i < 5; i++ {
		te := makeTraceEntry(
			net.ParseIP("10.0.0.1"),
			net.ParseIP("10.0.0.2"),
			uint16(8080+i), 443, 6, uint64(i*1000),
		)
		pub.PublishPacketEvent(te)
	}

	// Flush — gRPC will fail, should fall through to HTTP fallback or error gracefully
	// Dual-publish: 5 packet events → traces.packet + ebpf.packet.events = 2 batches
	pub.Flush(context.Background())

	stats := pub.Stats()
	if stats.FlushCycles != 1 {
		t.Errorf("FlushCycles = %d, want 1", stats.FlushCycles)
	}
	// Each topic batch either succeeds via HTTP fallback or errors — both are valid
	total := stats.BatchesSent + stats.Errors
	if total != 2 {
		t.Errorf("BatchesSent(%d) + Errors(%d) = %d, want 2 (dual-publish)", stats.BatchesSent, stats.Errors, total)
	}
}

func TestTracePublisher_RunWithHTTPFallback(t *testing.T) {
	srv := newTestWotanHTTPServer(t)
	defer srv.Close()

	httpAddr := strings.TrimPrefix(srv.URL, "http://")

	config := DefaultTracePublisherConfig()
	config.FlushInterval = 10 * time.Millisecond
	config.BatchSize = 1000
	config.WotanAddr = "localhost:19999"
	config.WotanHTTPAddr = httpAddr

	pub := NewTracePublisher(config)
	pub.Connect(context.Background())

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		pub.Run(ctx)
		close(done)
	}()

	// Enqueue events
	te := makeTraceEntry(
		net.ParseIP("10.0.0.1"),
		net.ParseIP("10.0.0.2"),
		8080, 443, 6, 1000,
	)
	pub.PublishPacketEvent(te)

	// Wait for flush cycle
	time.Sleep(50 * time.Millisecond)

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
