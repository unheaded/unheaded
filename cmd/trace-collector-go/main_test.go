// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/ebpf"
)

// ── FlowCorrelator tests ───────────────────────────────────────────────────

func makeEvent(evType ebpf.EventType, hopID uint8, flowLabel uint16, ts uint64) *ebpf.AnamnesisEvent {
	return &ebpf.AnamnesisEvent{
		TimestampNs: ts,
		EventType:   evType,
		HopID:       hopID,
		FlowLabelLo: flowLabel,
		Monad: ebpf.MonadState{
			Version:      ebpf.MonadVersion,
			SrcServiceID: 0x0A,
			DstServiceID: 0x0B,
			HopCount:     hopID,
			FlowAction:   0x01,
			CircuitState: 0x01,
		},
	}
}

func TestFlowCorrelator_CompleteFlow(t *testing.T) {
	fc := NewFlowCorrelator(60 * time.Second)

	// Send BIRTH → HOP → HOP → DEATH for flow 0x1234
	fc.Add(makeEvent(ebpf.EventBirth, 0, 0x1234, 1000))
	fc.Add(makeEvent(ebpf.EventHop, 1, 0x1234, 2000))
	fc.Add(makeEvent(ebpf.EventHop, 2, 0x1234, 3000))
	fc.Add(makeEvent(ebpf.EventDeath, 3, 0x1234, 4000))

	// Should get a completed flow
	select {
	case flow := <-fc.CompletedFlows():
		if flow.FlowLabel != 0x1234 {
			t.Errorf("FlowLabel = 0x%04x, want 0x1234", flow.FlowLabel)
		}
		if flow.HopCount != 2 {
			t.Errorf("HopCount = %d, want 2", flow.HopCount)
		}
		if !flow.Complete {
			t.Error("expected Complete = true")
		}
		if flow.Birth == nil {
			t.Error("expected Birth event")
		}
		if flow.Death == nil {
			t.Error("expected Death event")
		}
		if flow.StartNs != 1000 {
			t.Errorf("StartNs = %d, want 1000", flow.StartNs)
		}
		if flow.EndNs != 4000 {
			t.Errorf("EndNs = %d, want 4000", flow.EndNs)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for completed flow")
	}

	// No in-flight flows
	if fc.Len() != 0 {
		t.Errorf("Len() = %d, want 0", fc.Len())
	}
}

func TestFlowCorrelator_IncompleteFlow(t *testing.T) {
	fc := NewFlowCorrelator(60 * time.Second)

	// Only BIRTH and HOP — no DEATH
	fc.Add(makeEvent(ebpf.EventBirth, 0, 0x5678, 1000))
	fc.Add(makeEvent(ebpf.EventHop, 1, 0x5678, 2000))

	// No completed flow should be emitted
	select {
	case <-fc.CompletedFlows():
		t.Fatal("should not emit incomplete flow")
	case <-time.After(50 * time.Millisecond):
		// Expected
	}

	// One in-flight flow
	if fc.Len() != 1 {
		t.Errorf("Len() = %d, want 1", fc.Len())
	}
}

func TestFlowCorrelator_MultipleFlows(t *testing.T) {
	fc := NewFlowCorrelator(60 * time.Second)

	// Two concurrent flows
	fc.Add(makeEvent(ebpf.EventBirth, 0, 0x0001, 1000))
	fc.Add(makeEvent(ebpf.EventBirth, 0, 0x0002, 1100))
	fc.Add(makeEvent(ebpf.EventHop, 1, 0x0001, 2000))
	fc.Add(makeEvent(ebpf.EventDeath, 2, 0x0002, 2100)) // flow 2 completes first
	fc.Add(makeEvent(ebpf.EventDeath, 2, 0x0001, 3000)) // flow 1 completes second

	// Read both completed flows
	flows := make(map[uint16]*FlowRecord)
	for i := 0; i < 2; i++ {
		select {
		case flow := <-fc.CompletedFlows():
			flows[flow.FlowLabel] = flow
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for flow %d", i)
		}
	}

	if _, ok := flows[0x0001]; !ok {
		t.Error("missing flow 0x0001")
	}
	if _, ok := flows[0x0002]; !ok {
		t.Error("missing flow 0x0002")
	}
}

func TestFlowCorrelator_GC(t *testing.T) {
	fc := NewFlowCorrelator(100 * time.Millisecond)

	// Add stale flow
	fc.Add(makeEvent(ebpf.EventBirth, 0, 0x9999, 1000))

	// GC with "now" far in the future
	removed := fc.GC(1000 + uint64(time.Second.Nanoseconds()))
	if removed != 1 {
		t.Errorf("GC removed = %d, want 1", removed)
	}
	if fc.Len() != 0 {
		t.Errorf("Len() = %d after GC, want 0", fc.Len())
	}
}

func TestFlowCorrelator_AnomalyEvents(t *testing.T) {
	fc := NewFlowCorrelator(60 * time.Second)

	fc.Add(makeEvent(ebpf.EventBirth, 0, 0xAAAA, 1000))
	fc.Add(makeEvent(ebpf.EventAnomaly, 1, 0xAAAA, 1500))
	fc.Add(makeEvent(ebpf.EventChaos, 1, 0xAAAA, 1700))
	fc.Add(makeEvent(ebpf.EventDeath, 2, 0xAAAA, 2000))

	select {
	case flow := <-fc.CompletedFlows():
		if len(flow.Anomalies) != 2 {
			t.Errorf("Anomalies count = %d, want 2", len(flow.Anomalies))
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

// ── RateLimiter tests ───────────────────────────────────────────────────────

func TestRateLimiter_Unlimited(t *testing.T) {
	rl := NewRateLimiter(0)
	for i := 0; i < 100; i++ {
		if !rl.Allow() {
			t.Fatalf("unlimited rate limiter rejected event %d", i)
		}
	}
}

func TestRateLimiter_DropsExcess(t *testing.T) {
	rl := NewRateLimiter(10) // 10/sec

	// Exhaust tokens quickly
	allowed := 0
	for i := 0; i < 100; i++ {
		if rl.Allow() {
			allowed++
		}
	}

	if allowed > 20 {
		t.Errorf("allowed %d events, expected ~10 (some slack for refill)", allowed)
	}
	if rl.Dropped() == 0 {
		t.Error("expected some dropped events")
	}
}

// ── Topic mapping tests ─────────────────────────────────────────────────────

func TestTopicForEvent(t *testing.T) {
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
		if got := topicForEvent(tt.et); got != tt.want {
			t.Errorf("topicForEvent(%v) = %q, want %q", tt.et, got, tt.want)
		}
	}
}

// ── TransformEvent tests ────────────────────────────────────────────────────

func TestTransformEvent(t *testing.T) {
	ev := makeEvent(ebpf.EventBirth, 1, 0x1234, 1000)

	data, err := TransformEvent(ev)
	if err != nil {
		t.Fatalf("TransformEvent: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if decoded["event_type"] != "birth" {
		t.Errorf("event_type = %v, want \"birth\"", decoded["event_type"])
	}
}

// ── WotanPublisher tests ────────────────────────────────────────────────────

func TestWotanPublisher_PublishBatch(t *testing.T) {
	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	addr := strings.TrimPrefix(srv.URL, "http://")
	pub := NewWotanPublisher(addr, 100, 50*time.Millisecond)

	events := []*ebpf.AnamnesisEvent{
		makeEvent(ebpf.EventBirth, 0, 0x1234, 1000),
		makeEvent(ebpf.EventHop, 1, 0x1234, 2000),
	}

	ctx := context.Background()
	err := pub.PublishBatch(ctx, TopicBirth, events)
	if err != nil {
		t.Fatalf("PublishBatch: %v", err)
	}

	if !pub.IsConnected() {
		t.Error("expected connected after publish")
	}
	if len(received) == 0 {
		t.Error("server received no payload")
	}
}

func TestWotanPublisher_EmptyBatch(t *testing.T) {
	pub := NewWotanPublisher("localhost:9090", 100, 50*time.Millisecond)
	err := pub.PublishBatch(context.Background(), TopicBirth, nil)
	if err != nil {
		t.Fatalf("empty batch should not error: %v", err)
	}
}

// ── Health handler tests ────────────────────────────────────────────────────

func TestHealthHandler(t *testing.T) {
	pub := NewWotanPublisher("localhost:9090", 100, 50*time.Millisecond)
	fc := NewFlowCorrelator(60 * time.Second)

	handler := &healthHandler{
		publisher:  pub,
		correlator: fc,
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("status = %v, want \"ok\"", body["status"])
	}
}

// ── Demo event generator test ───────────────────────────────────────────────

func TestDemoEventGenerator(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := demoEventGenerator(ctx)

	// Should produce events
	count := 0
	timeout := time.After(200 * time.Millisecond)
	for {
		select {
		case data, ok := <-ch:
			if !ok {
				goto done
			}
			if len(data) != ebpf.AnamnesisEventSize {
				t.Fatalf("event size = %d, want %d", len(data), ebpf.AnamnesisEventSize)
			}
			ev, err := ebpf.DecodeAnamnesisEvent(data)
			if err != nil {
				t.Fatalf("decode error: %v", err)
			}
			if !ev.EventType.Valid() {
				t.Fatalf("invalid event type: %d", ev.EventType)
			}
			count++
			if count >= 20 {
				goto done
			}
		case <-timeout:
			goto done
		}
	}
done:
	cancel()
	if count < 10 {
		t.Errorf("generated only %d events, expected ≥10 in 200ms", count)
	}
}
