// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package ebpf

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	pkgebpf "unheaded/pkg/ebpf"
	wotanClient "unheaded/pkg/wotan-client"
)

// ── Mock Wotan subscriber ───────────────────────────────────────────────────

type mockWotanSubscriber struct {
	channels map[string]chan *wotanClient.Message
}

func newMockWotanSubscriber() *mockWotanSubscriber {
	return &mockWotanSubscriber{
		channels: make(map[string]chan *wotanClient.Message),
	}
}

func (m *mockWotanSubscriber) Subscribe(_ context.Context, topic, _ string) (*wotanClient.Subscriber, error) {
	m.channels[topic] = make(chan *wotanClient.Message, 100)
	return &wotanClient.Subscriber{Topic: topic, Status: "approved"}, nil
}

func (m *mockWotanSubscriber) StreamMessages(_ context.Context, topic string) (<-chan *wotanClient.Message, error) {
	ch, ok := m.channels[topic]
	if !ok {
		ch = make(chan *wotanClient.Message, 100)
		m.channels[topic] = ch
	}
	return ch, nil
}

func (m *mockWotanSubscriber) Close() error { return nil }

func (m *mockWotanSubscriber) send(topic string, ev *pkgebpf.AnamnesisEvent) {
	data, _ := json.Marshal(ev)
	ch := m.channels[topic]
	if ch == nil {
		return
	}
	ch <- &wotanClient.Message{
		Topic:   topic,
		Payload: string(data),
	}
}

// ── FlowBuilder tests ──────────────────────────────────────────────────────

func newTestEvent(evType pkgebpf.EventType, hopID uint8, flowLabel uint16, ts uint64) *pkgebpf.AnamnesisEvent {
	return &pkgebpf.AnamnesisEvent{
		TimestampNs: ts,
		EventType:   evType,
		HopID:       hopID,
		FlowLabelLo: flowLabel,
		Monad: pkgebpf.MonadState{
			Version:      pkgebpf.MonadVersion,
			SrcServiceID: 0x0A,
			DstServiceID: 0x0B,
			HopCount:     hopID,
			FlowAction:   0x01,
			CircuitState: 0x01,
		},
	}
}

func TestFlowBuilder_CompleteFlow(t *testing.T) {
	fb := NewFlowBuilder(FlowBuilderConfig{TTL: 60 * time.Second})

	fb.Ingest(newTestEvent(pkgebpf.EventBirth, 0, 0x1234, 1000))
	fb.Ingest(newTestEvent(pkgebpf.EventHop, 1, 0x1234, 2000))
	fb.Ingest(newTestEvent(pkgebpf.EventHop, 2, 0x1234, 3000))
	fb.Ingest(newTestEvent(pkgebpf.EventDeath, 3, 0x1234, 4000))

	select {
	case flow := <-fb.CompletedFlows():
		if flow.FlowLabel != 0x1234 {
			t.Errorf("FlowLabel = 0x%04x, want 0x1234", flow.FlowLabel)
		}
		if flow.HopCount != 2 {
			t.Errorf("HopCount = %d, want 2", flow.HopCount)
		}
		if !flow.Complete {
			t.Error("expected Complete = true")
		}
		if flow.LatencyNs != 3000 {
			t.Errorf("LatencyNs = %d, want 3000", flow.LatencyNs)
		}
		if len(flow.HopLatency) != 3 {
			t.Errorf("HopLatency len = %d, want 3 (birth→hop, hop→hop, hop→death)", len(flow.HopLatency))
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for completed flow")
	}

	stats := fb.Stats()
	if stats.CompletedFlows != 1 {
		t.Errorf("CompletedFlows = %d, want 1", stats.CompletedFlows)
	}
}

func TestFlowBuilder_IncompleteFlow(t *testing.T) {
	fb := NewFlowBuilder(FlowBuilderConfig{TTL: 60 * time.Second})

	fb.Ingest(newTestEvent(pkgebpf.EventBirth, 0, 0x5678, 1000))
	fb.Ingest(newTestEvent(pkgebpf.EventHop, 1, 0x5678, 2000))

	select {
	case <-fb.CompletedFlows():
		t.Fatal("should not emit incomplete flow")
	case <-time.After(50 * time.Millisecond):
		// Expected
	}

	if fb.InFlightCount() != 1 {
		t.Errorf("InFlightCount = %d, want 1", fb.InFlightCount())
	}
}

func TestFlowBuilder_GC(t *testing.T) {
	fb := NewFlowBuilder(FlowBuilderConfig{TTL: 100 * time.Millisecond})

	fb.Ingest(newTestEvent(pkgebpf.EventBirth, 0, 0x9999, 1000))

	removed := fb.GC(1000 + uint64(time.Second.Nanoseconds()))
	if removed != 1 {
		t.Errorf("GC removed = %d, want 1", removed)
	}
}

func TestFlowBuilder_AnomalyTracking(t *testing.T) {
	fb := NewFlowBuilder(FlowBuilderConfig{})

	fb.Ingest(newTestEvent(pkgebpf.EventBirth, 0, 0xAAAA, 1000))
	fb.Ingest(newTestEvent(pkgebpf.EventAnomaly, 1, 0xAAAA, 1500))
	fb.Ingest(newTestEvent(pkgebpf.EventChaos, 1, 0xAAAA, 1700))
	fb.Ingest(newTestEvent(pkgebpf.EventDeath, 2, 0xAAAA, 2000))

	select {
	case flow := <-fb.CompletedFlows():
		if len(flow.Anomalies) != 2 {
			t.Errorf("Anomalies = %d, want 2", len(flow.Anomalies))
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

// ── HopLatency tests ────────────────────────────────────────────────────────

func TestAnamnesisFlow_ComputeHopLatency(t *testing.T) {
	flow := &AnamnesisFlow{
		Birth: newTestEvent(pkgebpf.EventBirth, 0, 0x1234, 1000),
		Hops: []*pkgebpf.AnamnesisEvent{
			newTestEvent(pkgebpf.EventHop, 1, 0x1234, 2000),
			newTestEvent(pkgebpf.EventHop, 2, 0x1234, 3500),
		},
		Death: newTestEvent(pkgebpf.EventDeath, 3, 0x1234, 4000),
	}

	flow.computeHopLatency()

	if len(flow.HopLatency) != 3 {
		t.Fatalf("HopLatency len = %d, want 3", len(flow.HopLatency))
	}

	// Birth → Hop1: 1000ns
	if flow.HopLatency[0].LatencyNs != 1000 {
		t.Errorf("hop 0→1 latency = %d, want 1000", flow.HopLatency[0].LatencyNs)
	}
	// Hop1 → Hop2: 1500ns
	if flow.HopLatency[1].LatencyNs != 1500 {
		t.Errorf("hop 1→2 latency = %d, want 1500", flow.HopLatency[1].LatencyNs)
	}
	// Hop2 → Death: 500ns
	if flow.HopLatency[2].LatencyNs != 500 {
		t.Errorf("hop 2→3 latency = %d, want 500", flow.HopLatency[2].LatencyNs)
	}
}

// ── Monad decode tests ──────────────────────────────────────────────────────

func TestDecodeMonadForDashboard(t *testing.T) {
	m := &pkgebpf.MonadState{
		Version:      pkgebpf.MonadVersion,
		SrcServiceID: 0x0A,
		DstServiceID: 0x0B,
		HopCount:     3,
		QosClass:     0x01,
		FlowAction:   0x01, // forward
		CircuitState: 0x01, // closed
		Flags:        pkgebpf.FlagTraced | pkgebpf.FlagChaos,
		LatencyHint:  250,
		DeployRing:   0x02, // canary
		Scratch:      [4]uint8{0xBE, 0xEF, 0xDE, 0xAD},
	}
	m.RecomputeChecksum()

	decoded := DecodeMonadForDashboard(m)

	if decoded.FlowAction != "forward" {
		t.Errorf("FlowAction = %q, want \"forward\"", decoded.FlowAction)
	}
	if decoded.CircuitState != "closed" {
		t.Errorf("CircuitState = %q, want \"closed\"", decoded.CircuitState)
	}
	if decoded.DeployRing != "canary" {
		t.Errorf("DeployRing = %q, want \"canary\"", decoded.DeployRing)
	}
	if len(decoded.Flags) != 2 {
		t.Errorf("Flags len = %d, want 2", len(decoded.Flags))
	}
	if decoded.ScratchR0 != 0xBEEF {
		t.Errorf("ScratchR0 = 0x%04x, want 0xBEEF", decoded.ScratchR0)
	}
	if decoded.ScratchR1 != 0xDEAD {
		t.Errorf("ScratchR1 = 0x%04x, want 0xDEAD", decoded.ScratchR1)
	}
	if !decoded.ChecksumOK {
		t.Error("expected ChecksumOK = true")
	}
}

func TestDecodeMonadForDashboard_AllActions(t *testing.T) {
	tests := []struct {
		action uint8
		want   string
	}{
		{0x01, "forward"},
		{0x02, "trace"},
		{0x03, "sample"},
		{0x04, "mirror"},
		{0x05, "drop"},
		{0xFF, "unknown(255)"},
	}
	for _, tt := range tests {
		m := &pkgebpf.MonadState{FlowAction: tt.action}
		d := DecodeMonadForDashboard(m)
		if d.FlowAction != tt.want {
			t.Errorf("action 0x%02x: got %q, want %q", tt.action, d.FlowAction, tt.want)
		}
	}
}

// ── WebSocket message tests ─────────────────────────────────────────────────

func TestNewFlowWSMessage(t *testing.T) {
	flow := &AnamnesisFlow{
		FlowLabel: 0x1234,
		Complete:  true,
		HopCount:  2,
	}

	data, err := NewFlowWSMessage(flow)
	if err != nil {
		t.Fatalf("NewFlowWSMessage: %v", err)
	}

	var msg AnamnesisWSMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if msg.Type != "flow" {
		t.Errorf("Type = %q, want \"flow\"", msg.Type)
	}
}

func TestNewHopWSMessage(t *testing.T) {
	ev := newTestEvent(pkgebpf.EventHop, 1, 0x1234, 1000)
	data, err := NewHopWSMessage(ev)
	if err != nil {
		t.Fatalf("NewHopWSMessage: %v", err)
	}

	var msg AnamnesisWSMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if msg.Type != "hop" {
		t.Errorf("Type = %q, want \"hop\"", msg.Type)
	}
}

func TestNewAnomalyWSMessage(t *testing.T) {
	ev := newTestEvent(pkgebpf.EventChaos, 1, 0x1234, 1000)
	data, err := NewAnomalyWSMessage(ev)
	if err != nil {
		t.Fatalf("NewAnomalyWSMessage: %v", err)
	}

	var msg AnamnesisWSMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if msg.Type != "chaos" {
		t.Errorf("Type = %q, want \"chaos\"", msg.Type)
	}
}

// ── AnamnesisIngestor integration test ──────────────────────────────────────

func TestAnamnesisIngestor_EndToEnd(t *testing.T) {
	mock := newMockWotanSubscriber()
	fb := NewFlowBuilder(FlowBuilderConfig{})

	var broadcasts [][]byte
	var broadcastMu sync.Mutex
	broadcaster := func(data []byte) {
		broadcastMu.Lock()
		broadcasts = append(broadcasts, data)
		broadcastMu.Unlock()
	}

	ing := NewAnamnesisIngestor(mock, fb, broadcaster, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := ing.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Give the goroutines time to start
	time.Sleep(50 * time.Millisecond)

	// Send a complete flow: BIRTH → HOP → DEATH
	mock.send(AnamnesisTopicBirth, newTestEvent(pkgebpf.EventBirth, 0, 0x1234, 1000))
	mock.send(AnamnesisTopicHop, newTestEvent(pkgebpf.EventHop, 1, 0x1234, 2000))
	mock.send(AnamnesisTopicDeath, newTestEvent(pkgebpf.EventDeath, 2, 0x1234, 3000))

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	stats := ing.Stats()
	if stats.EventsIngested != 3 {
		t.Errorf("EventsIngested = %d, want 3", stats.EventsIngested)
	}
	if stats.FlowBuilder.CompletedFlows != 1 {
		t.Errorf("CompletedFlows = %d, want 1", stats.FlowBuilder.CompletedFlows)
	}

	// Should have received WebSocket broadcasts (hop + flow at minimum)
	broadcastMu.Lock()
	bc := len(broadcasts)
	broadcastMu.Unlock()
	if bc < 2 {
		t.Errorf("broadcasts = %d, want ≥2 (hop + flow)", bc)
	}
}
