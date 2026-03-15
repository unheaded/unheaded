// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"testing"
	"time"
)

// ── Helper: create a FlowKey with known IPv4 addresses ───────────────────────

func makeFlowKey(srcIP [4]byte, dstIP [4]byte, srcPort, dstPort uint16, proto uint8) FlowKey {
	return FlowKey{
		SrcAddr:  binary.BigEndian.Uint32(srcIP[:]),
		DstAddr:  binary.BigEndian.Uint32(dstIP[:]),
		SrcPort:  srcPort,
		DstPort:  dstPort,
		Protocol: proto,
	}
}

// makeFlowState creates a FlowStateEntry with known values.
func makeFlowState(state uint8, startNS, lastSeenNS, pktsIn, pktsOut, bytesIn, bytesOut uint64) FlowStateEntry {
	return FlowStateEntry{
		StartNS:    startNS,
		LastSeenNS: lastSeenNS,
		PacketsIn:  pktsIn,
		PacketsOut: pktsOut,
		BytesIn:    bytesIn,
		BytesOut:   bytesOut,
		State:      state,
	}
}

// insertFlow encodes a FlowKey+FlowState and inserts into a MockMapReader.
func insertFlow(m *MockMapReader, fk FlowKey, fs FlowStateEntry) {
	keyBytes := fk.Encode()
	valBytes := fs.Encode()
	m.Insert(keyBytes[:], valBytes[:])
}

// ── FlowKey encode/decode roundtrip ──────────────────────────────────────────

func TestFlowKey_EncodeDecodeRoundtrip(t *testing.T) {
	original := makeFlowKey(
		[4]byte{10, 10, 10, 1},
		[4]byte{10, 10, 10, 2},
		8080, 443, 6,
	)

	encoded := original.Encode()
	decoded, err := DecodeFlowKey(encoded[:])
	if err != nil {
		t.Fatalf("DecodeFlowKey: %v", err)
	}

	if decoded.SrcAddr != original.SrcAddr {
		t.Errorf("SrcAddr = 0x%08x, want 0x%08x", decoded.SrcAddr, original.SrcAddr)
	}
	if decoded.DstAddr != original.DstAddr {
		t.Errorf("DstAddr = 0x%08x, want 0x%08x", decoded.DstAddr, original.DstAddr)
	}
	if decoded.SrcPort != original.SrcPort {
		t.Errorf("SrcPort = %d, want %d", decoded.SrcPort, original.SrcPort)
	}
	if decoded.DstPort != original.DstPort {
		t.Errorf("DstPort = %d, want %d", decoded.DstPort, original.DstPort)
	}
	if decoded.Protocol != original.Protocol {
		t.Errorf("Protocol = %d, want %d", decoded.Protocol, original.Protocol)
	}
}

func TestDecodeFlowKey_TooShort(t *testing.T) {
	_, err := DecodeFlowKey(make([]byte, 5))
	if err == nil {
		t.Error("expected error for short input")
	}
}

func TestFlowKey_SrcIP(t *testing.T) {
	fk := makeFlowKey([4]byte{192, 168, 1, 100}, [4]byte{10, 0, 0, 1}, 80, 443, 6)
	srcIP := fk.SrcIP()
	if srcIP.String() != "192.168.1.100" {
		t.Errorf("SrcIP = %s, want 192.168.1.100", srcIP)
	}
}

func TestFlowKey_DstIP(t *testing.T) {
	fk := makeFlowKey([4]byte{192, 168, 1, 100}, [4]byte{10, 0, 0, 1}, 80, 443, 6)
	dstIP := fk.DstIP()
	if dstIP.String() != "10.0.0.1" {
		t.Errorf("DstIP = %s, want 10.0.0.1", dstIP)
	}
}

func TestFlowKey_String(t *testing.T) {
	fk := makeFlowKey([4]byte{10, 10, 10, 1}, [4]byte{10, 10, 10, 2}, 8080, 443, 6)
	s := fk.String()
	if s == "" {
		t.Error("String() returned empty")
	}
	// Should contain the protocol
	if len(s) < 10 {
		t.Errorf("String() too short: %q", s)
	}
}

// ── FlowStateEntry encode/decode roundtrip ───────────────────────────────────

func TestFlowState_EncodeDecodeRoundtrip(t *testing.T) {
	original := makeFlowState(ConnStateEstablished, 1000, 5000, 100, 200, 50000, 100000)
	original.TraceID[0] = 0xAB
	original.TraceID[15] = 0xCD

	encoded := original.Encode()
	decoded, err := DecodeFlowState(encoded[:])
	if err != nil {
		t.Fatalf("DecodeFlowState: %v", err)
	}

	if decoded.StartNS != original.StartNS {
		t.Errorf("StartNS = %d, want %d", decoded.StartNS, original.StartNS)
	}
	if decoded.LastSeenNS != original.LastSeenNS {
		t.Errorf("LastSeenNS = %d, want %d", decoded.LastSeenNS, original.LastSeenNS)
	}
	if decoded.PacketsIn != original.PacketsIn {
		t.Errorf("PacketsIn = %d, want %d", decoded.PacketsIn, original.PacketsIn)
	}
	if decoded.PacketsOut != original.PacketsOut {
		t.Errorf("PacketsOut = %d, want %d", decoded.PacketsOut, original.PacketsOut)
	}
	if decoded.BytesIn != original.BytesIn {
		t.Errorf("BytesIn = %d, want %d", decoded.BytesIn, original.BytesIn)
	}
	if decoded.BytesOut != original.BytesOut {
		t.Errorf("BytesOut = %d, want %d", decoded.BytesOut, original.BytesOut)
	}
	if decoded.State != original.State {
		t.Errorf("State = %d, want %d", decoded.State, original.State)
	}
	if decoded.TraceID != original.TraceID {
		t.Error("TraceID mismatch")
	}
}

func TestDecodeFlowState_TooShort(t *testing.T) {
	_, err := DecodeFlowState(make([]byte, 10))
	if err == nil {
		t.Error("expected error for short input")
	}
}

func TestFlowState_TotalPackets(t *testing.T) {
	fs := makeFlowState(ConnStateEstablished, 0, 0, 100, 200, 0, 0)
	if fs.TotalPackets() != 300 {
		t.Errorf("TotalPackets = %d, want 300", fs.TotalPackets())
	}
}

func TestFlowState_TotalBytes(t *testing.T) {
	fs := makeFlowState(ConnStateEstablished, 0, 0, 0, 0, 50000, 100000)
	if fs.TotalBytes() != 150000 {
		t.Errorf("TotalBytes = %d, want 150000", fs.TotalBytes())
	}
}

func TestFlowState_StateName(t *testing.T) {
	tests := []struct {
		state uint8
		want  string
	}{
		{ConnStateUnknown, "UNKNOWN"},
		{ConnStateSynSent, "SYN_SENT"},
		{ConnStateSynReceived, "SYN_RECEIVED"},
		{ConnStateEstablished, "ESTABLISHED"},
		{ConnStateFinWait1, "FIN_WAIT1"},
		{ConnStateFinWait2, "FIN_WAIT2"},
		{ConnStateCloseWait, "CLOSE_WAIT"},
		{ConnStateClosing, "CLOSING"},
		{ConnStateLastAck, "LAST_ACK"},
		{ConnStateTimeWait, "TIME_WAIT"},
		{ConnStateClosed, "CLOSED"},
		{99, "state(99)"},
	}
	for _, tt := range tests {
		fs := FlowStateEntry{State: tt.state}
		if got := fs.StateName(); got != tt.want {
			t.Errorf("StateName(%d) = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestFlowState_DurationNS(t *testing.T) {
	fs := makeFlowState(ConnStateEstablished, 1000, 5000, 0, 0, 0, 0)
	if fs.DurationNS() != 4000 {
		t.Errorf("DurationNS = %d, want 4000", fs.DurationNS())
	}

	// Edge case: LastSeenNS < StartNS (should return 0)
	fs2 := makeFlowState(ConnStateEstablished, 5000, 1000, 0, 0, 0, 0)
	if fs2.DurationNS() != 0 {
		t.Errorf("DurationNS should be 0 when LastSeenNS < StartNS, got %d", fs2.DurationNS())
	}
}

func TestFlowState_TraceIDHex(t *testing.T) {
	fs := FlowStateEntry{}
	fs.TraceID[0] = 0xDE
	fs.TraceID[15] = 0xAD
	hex := fs.TraceIDHex()
	if len(hex) != 32 {
		t.Errorf("TraceIDHex length = %d, want 32", len(hex))
	}
	if hex[:2] != "de" {
		t.Errorf("TraceIDHex starts with %q, want \"de\"", hex[:2])
	}
	if hex[30:32] != "ad" {
		t.Errorf("TraceIDHex ends with %q, want \"ad\"", hex[30:32])
	}
}

// ── FlowEventEntry decode ────────────────────────────────────────────────────

func TestDecodeFlowEvent(t *testing.T) {
	// Build a raw 104-byte buffer
	var buf [FlowEventSize]byte

	// timestamp_ns at offset 0
	binary.LittleEndian.PutUint64(buf[0:8], 9999)

	// flow_key at offset 8 (16 bytes)
	fk := makeFlowKey([4]byte{10, 0, 0, 1}, [4]byte{10, 0, 0, 2}, 80, 443, 6)
	keyBytes := fk.Encode()
	copy(buf[8:24], keyBytes[:])

	// flow_state at offset 24 (72 bytes)
	fs := makeFlowState(ConnStateEstablished, 1000, 5000, 10, 20, 5000, 10000)
	stateBytes := fs.Encode()
	copy(buf[24:96], stateBytes[:])

	// event_type at offset 96
	buf[96] = FlowEventStateChange

	event, err := DecodeFlowEvent(buf[:])
	if err != nil {
		t.Fatalf("DecodeFlowEvent: %v", err)
	}

	if event.TimestampNS != 9999 {
		t.Errorf("TimestampNS = %d, want 9999", event.TimestampNS)
	}
	if event.Key.Protocol != 6 {
		t.Errorf("Protocol = %d, want 6", event.Key.Protocol)
	}
	if event.State.State != ConnStateEstablished {
		t.Errorf("State = %d, want %d", event.State.State, ConnStateEstablished)
	}
	if event.EventType != FlowEventStateChange {
		t.Errorf("EventType = %d, want %d", event.EventType, FlowEventStateChange)
	}
	if event.EventTypeName() != "state_change" {
		t.Errorf("EventTypeName = %q, want \"state_change\"", event.EventTypeName())
	}
}

func TestDecodeFlowEvent_TooShort(t *testing.T) {
	_, err := DecodeFlowEvent(make([]byte, 50))
	if err == nil {
		t.Error("expected error for short input")
	}
}

// ── FlowEventTypeName ────────────────────────────────────────────────────────

func TestFlowEventTypeName(t *testing.T) {
	tests := []struct {
		et   uint8
		want string
	}{
		{FlowEventNew, "new"},
		{FlowEventStateChange, "state_change"},
		{FlowEventExpired, "expired"},
		{FlowEventUpdate, "update"},
		{FlowEventAnomaly, "anomaly"},
		{99, "event(99)"},
	}
	for _, tt := range tests {
		if got := FlowEventTypeName(tt.et); got != tt.want {
			t.Errorf("FlowEventTypeName(%d) = %q, want %q", tt.et, got, tt.want)
		}
	}
}

// ── ConnStateName ────────────────────────────────────────────────────────────

func TestConnStateName(t *testing.T) {
	if ConnStateName(ConnStateEstablished) != "ESTABLISHED" {
		t.Errorf("ConnStateName(3) = %q, want ESTABLISHED", ConnStateName(ConnStateEstablished))
	}
	if ConnStateName(255) != "state(255)" {
		t.Errorf("ConnStateName(255) = %q, want state(255)", ConnStateName(255))
	}
}

// ── FlowReader tests ─────────────────────────────────────────────────────────

func TestFlowReader_NewFlowDetection(t *testing.T) {
	loader := NewMockBPFLoader()

	// Insert a flow into the mock flow state map
	fk := makeFlowKey([4]byte{10, 10, 10, 1}, [4]byte{10, 10, 10, 2}, 8080, 443, 6)
	fs := makeFlowState(ConnStateEstablished, 1000, uint64(time.Now().UnixNano()), 10, 20, 5000, 10000)
	insertFlow(loader.flowMap, fk, fs)

	publisher := NewWotanPublisher("localhost:9090", 100, 50*time.Millisecond)
	config := DefaultFlowReaderConfig()
	reader := NewFlowReader(loader, publisher, config)

	// Run a single poll
	reader.pollOnce(context.Background())

	stats := reader.Stats()
	if stats.FlowsRead != 1 {
		t.Errorf("FlowsRead = %d, want 1", stats.FlowsRead)
	}
	if stats.PollCycles != 1 {
		t.Errorf("PollCycles = %d, want 1", stats.PollCycles)
	}
	if stats.EventsSent != 1 {
		t.Errorf("EventsSent = %d, want 1 (new flow event)", stats.EventsSent)
	}

	// Should have emitted a new flow event
	select {
	case ev := <-reader.Events():
		if ev.EventType != FlowEventNew {
			t.Errorf("EventType = %d, want %d (new)", ev.EventType, FlowEventNew)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for flow event")
	}

	if reader.ActiveFlows() != 1 {
		t.Errorf("ActiveFlows = %d, want 1", reader.ActiveFlows())
	}
}

func TestFlowReader_StateChangeDetection(t *testing.T) {
	loader := NewMockBPFLoader()

	fk := makeFlowKey([4]byte{10, 10, 10, 1}, [4]byte{10, 10, 10, 2}, 8080, 443, 6)
	fs := makeFlowState(ConnStateEstablished, 1000, uint64(time.Now().UnixNano()), 10, 20, 5000, 10000)
	insertFlow(loader.flowMap, fk, fs)

	publisher := NewWotanPublisher("localhost:9090", 100, 50*time.Millisecond)
	config := DefaultFlowReaderConfig()
	reader := NewFlowReader(loader, publisher, config)

	// First poll: detect new flow
	reader.pollOnce(context.Background())
	// Drain the new event
	<-reader.Events()

	// Change state to FinWait1
	fs.State = ConnStateFinWait1
	fs.PacketsIn = 50
	insertFlow(loader.flowMap, fk, fs)

	// Second poll: detect state change
	reader.pollOnce(context.Background())

	stats := reader.Stats()
	if stats.EventsSent != 2 {
		t.Errorf("EventsSent = %d, want 2 (new + state_change)", stats.EventsSent)
	}

	select {
	case ev := <-reader.Events():
		if ev.EventType != FlowEventStateChange {
			t.Errorf("EventType = %d, want %d (state_change)", ev.EventType, FlowEventStateChange)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for state change event")
	}
}

func TestFlowReader_StaleFlowExpiration(t *testing.T) {
	loader := NewMockBPFLoader()

	// Insert a flow with a recent last_seen so it survives the first poll
	nowNS := uint64(time.Now().UnixNano())
	fk := makeFlowKey([4]byte{10, 10, 10, 1}, [4]byte{10, 10, 10, 2}, 8080, 443, 6)
	fs := makeFlowState(ConnStateEstablished, nowNS-1000, nowNS, 10, 20, 5000, 10000)
	insertFlow(loader.flowMap, fk, fs)

	publisher := NewWotanPublisher("localhost:9090", 100, 50*time.Millisecond)
	config := DefaultFlowReaderConfig()
	config.StaleTTL = 10 * time.Millisecond // Short TTL for testing
	reader := NewFlowReader(loader, publisher, config)

	// First poll: detects new flow (last_seen is fresh, so it survives stale check)
	reader.pollOnce(context.Background())
	<-reader.Events() // drain new event

	if reader.ActiveFlows() != 1 {
		t.Fatalf("ActiveFlows = %d after first poll, want 1", reader.ActiveFlows())
	}

	// Wait for the flow to become stale
	time.Sleep(20 * time.Millisecond)

	// Second poll: flow is still in the BPF map but last_seen is now stale
	reader.pollOnce(context.Background())

	stats := reader.Stats()
	if stats.FlowsExpired != 1 {
		t.Errorf("FlowsExpired = %d, want 1", stats.FlowsExpired)
	}

	// Should get an expired event
	select {
	case ev := <-reader.Events():
		if ev.EventType != FlowEventExpired {
			t.Errorf("EventType = %d, want %d (expired)", ev.EventType, FlowEventExpired)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for expired event")
	}

	if reader.ActiveFlows() != 0 {
		t.Errorf("ActiveFlows = %d after expiration, want 0", reader.ActiveFlows())
	}
}

func TestFlowReader_DisappearedFlowExpiration(t *testing.T) {
	loader := NewMockBPFLoader()

	fk := makeFlowKey([4]byte{10, 10, 10, 1}, [4]byte{10, 10, 10, 2}, 8080, 443, 6)
	fs := makeFlowState(ConnStateEstablished, 1000, uint64(time.Now().UnixNano()), 10, 20, 5000, 10000)
	insertFlow(loader.flowMap, fk, fs)

	publisher := NewWotanPublisher("localhost:9090", 100, 50*time.Millisecond)
	config := DefaultFlowReaderConfig()
	reader := NewFlowReader(loader, publisher, config)

	// First poll: detect new flow
	reader.pollOnce(context.Background())
	<-reader.Events() // drain new event

	// Remove the flow from the BPF map (simulating LRU eviction)
	keyBytes := fk.Encode()
	loader.flowMap.Delete(keyBytes[:])

	// Second poll: flow disappeared from map
	reader.pollOnce(context.Background())

	stats := reader.Stats()
	if stats.FlowsExpired != 1 {
		t.Errorf("FlowsExpired = %d, want 1", stats.FlowsExpired)
	}

	select {
	case ev := <-reader.Events():
		if ev.EventType != FlowEventExpired {
			t.Errorf("EventType = %d, want %d (expired)", ev.EventType, FlowEventExpired)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for expired event")
	}
}

func TestFlowReader_EmptyMap(t *testing.T) {
	loader := NewMockBPFLoader()
	publisher := NewWotanPublisher("localhost:9090", 100, 50*time.Millisecond)
	reader := NewFlowReader(loader, publisher, DefaultFlowReaderConfig())

	reader.pollOnce(context.Background())

	stats := reader.Stats()
	if stats.FlowsRead != 0 {
		t.Errorf("FlowsRead = %d, want 0", stats.FlowsRead)
	}
	if stats.EventsSent != 0 {
		t.Errorf("EventsSent = %d, want 0", stats.EventsSent)
	}
}

func TestFlowReader_MultipleFlows(t *testing.T) {
	loader := NewMockBPFLoader()

	// Insert 5 distinct flows
	nowNS := uint64(time.Now().UnixNano())
	for i := 0; i < 5; i++ {
		fk := makeFlowKey(
			[4]byte{10, 10, 10, 1},
			[4]byte{10, 10, 10, byte(2 + i)},
			uint16(8080+i), 443, 6,
		)
		fs := makeFlowState(ConnStateEstablished, nowNS-1000, nowNS, uint64(i*10), uint64(i*20), 0, 0)
		insertFlow(loader.flowMap, fk, fs)
	}

	publisher := NewWotanPublisher("localhost:9090", 100, 50*time.Millisecond)
	reader := NewFlowReader(loader, publisher, DefaultFlowReaderConfig())

	reader.pollOnce(context.Background())

	stats := reader.Stats()
	if stats.FlowsRead != 5 {
		t.Errorf("FlowsRead = %d, want 5", stats.FlowsRead)
	}
	if stats.EventsSent != 5 {
		t.Errorf("EventsSent = %d, want 5", stats.EventsSent)
	}
	if reader.ActiveFlows() != 5 {
		t.Errorf("ActiveFlows = %d, want 5", reader.ActiveFlows())
	}

	// Drain events
	for i := 0; i < 5; i++ {
		select {
		case ev := <-reader.Events():
			if ev.EventType != FlowEventNew {
				t.Errorf("event %d: EventType = %d, want %d (new)", i, ev.EventType, FlowEventNew)
			}
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for event %d", i)
		}
	}
}

func TestFlowReader_EmptyFlowMap(t *testing.T) {
	// A loader with an empty (but non-nil) flow map
	loader := NewMockBPFLoader()
	// flowMap is empty by default

	publisher := NewWotanPublisher("localhost:9090", 100, 50*time.Millisecond)
	reader := NewFlowReader(loader, publisher, DefaultFlowReaderConfig())

	reader.pollOnce(context.Background())

	stats := reader.Stats()
	if stats.PollCycles != 1 {
		t.Errorf("PollCycles = %d, want 1", stats.PollCycles)
	}
	if stats.FlowsRead != 0 {
		t.Errorf("FlowsRead = %d, want 0", stats.FlowsRead)
	}
	if stats.EventsSent != 0 {
		t.Errorf("EventsSent = %d, want 0", stats.EventsSent)
	}
}

func TestFlowReader_UnchangedFlowNoEvent(t *testing.T) {
	loader := NewMockBPFLoader()

	fk := makeFlowKey([4]byte{10, 10, 10, 1}, [4]byte{10, 10, 10, 2}, 8080, 443, 6)
	fs := makeFlowState(ConnStateEstablished, 1000, uint64(time.Now().UnixNano()), 10, 20, 5000, 10000)
	insertFlow(loader.flowMap, fk, fs)

	publisher := NewWotanPublisher("localhost:9090", 100, 50*time.Millisecond)
	reader := NewFlowReader(loader, publisher, DefaultFlowReaderConfig())

	// First poll: new flow event
	reader.pollOnce(context.Background())
	<-reader.Events()

	// Second poll: same state, no event expected
	reader.pollOnce(context.Background())

	stats := reader.Stats()
	if stats.EventsSent != 1 {
		t.Errorf("EventsSent = %d, want 1 (only the initial new event)", stats.EventsSent)
	}

	// Channel should be empty
	select {
	case ev := <-reader.Events():
		t.Errorf("unexpected event: type=%d", ev.EventType)
	case <-time.After(50 * time.Millisecond):
		// Expected: no event
	}
}

func TestFlowReader_BadKeyDecode(t *testing.T) {
	loader := NewMockBPFLoader()

	// Insert entry with a too-short key (< 16 bytes)
	loader.flowMap.Insert([]byte("short"), make([]byte, FlowStateSize))

	publisher := NewWotanPublisher("localhost:9090", 100, 50*time.Millisecond)
	reader := NewFlowReader(loader, publisher, DefaultFlowReaderConfig())

	reader.pollOnce(context.Background())

	stats := reader.Stats()
	if stats.DecodeErrors != 1 {
		t.Errorf("DecodeErrors = %d, want 1", stats.DecodeErrors)
	}
}

func TestFlowReader_BadValueDecode(t *testing.T) {
	loader := NewMockBPFLoader()

	// Insert entry with valid key but too-short value
	fk := makeFlowKey([4]byte{10, 0, 0, 1}, [4]byte{10, 0, 0, 2}, 80, 443, 6)
	keyBytes := fk.Encode()
	loader.flowMap.Insert(keyBytes[:], []byte("tooshort"))

	publisher := NewWotanPublisher("localhost:9090", 100, 50*time.Millisecond)
	reader := NewFlowReader(loader, publisher, DefaultFlowReaderConfig())

	reader.pollOnce(context.Background())

	stats := reader.Stats()
	if stats.DecodeErrors != 1 {
		t.Errorf("DecodeErrors = %d, want 1", stats.DecodeErrors)
	}
}

func TestFlowReader_RunContext(t *testing.T) {
	loader := NewMockBPFLoader()
	publisher := NewWotanPublisher("localhost:9090", 100, 50*time.Millisecond)
	config := DefaultFlowReaderConfig()
	config.PollInterval = 10 * time.Millisecond
	reader := NewFlowReader(loader, publisher, config)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		reader.Run(ctx)
		close(done)
	}()

	// Let it run a few cycles
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Good, Run returned
	case <-time.After(time.Second):
		t.Fatal("Run did not return after context cancellation")
	}

	stats := reader.Stats()
	if stats.PollCycles == 0 {
		t.Error("expected at least 1 poll cycle")
	}
}

// ── FlowReader stats accuracy ────────────────────────────────────────────────

func TestFlowReader_StatsAccuracy(t *testing.T) {
	loader := NewMockBPFLoader()
	publisher := NewWotanPublisher("localhost:9090", 100, 50*time.Millisecond)
	reader := NewFlowReader(loader, publisher, DefaultFlowReaderConfig())

	// Insert 3 flows
	nowNS := uint64(time.Now().UnixNano())
	for i := 0; i < 3; i++ {
		fk := makeFlowKey([4]byte{10, 0, 0, 1}, [4]byte{10, 0, 0, byte(2 + i)}, uint16(1000+i), 80, 6)
		fs := makeFlowState(ConnStateEstablished, nowNS-1000, nowNS, 10, 20, 1000, 2000)
		insertFlow(loader.flowMap, fk, fs)
	}

	// First poll: 3 new flows
	reader.pollOnce(context.Background())

	stats := reader.Stats()
	if stats.PollCycles != 1 {
		t.Errorf("PollCycles = %d, want 1", stats.PollCycles)
	}
	if stats.FlowsRead != 3 {
		t.Errorf("FlowsRead = %d, want 3", stats.FlowsRead)
	}
	if stats.EventsSent != 3 {
		t.Errorf("EventsSent = %d, want 3", stats.EventsSent)
	}
	if stats.DecodeErrors != 0 {
		t.Errorf("DecodeErrors = %d, want 0", stats.DecodeErrors)
	}

	// Second poll: no changes
	reader.pollOnce(context.Background())

	stats = reader.Stats()
	if stats.PollCycles != 2 {
		t.Errorf("PollCycles = %d, want 2", stats.PollCycles)
	}
	if stats.EventsSent != 3 {
		t.Errorf("EventsSent = %d, want 3 (no new events)", stats.EventsSent)
	}

	// Drain all events
	for i := 0; i < 3; i++ {
		select {
		case <-reader.Events():
		case <-time.After(time.Second):
			t.Fatalf("timeout draining event %d", i)
		}
	}
}

// ── DefaultFlowReaderConfig ──────────────────────────────────────────────────

func TestDefaultFlowReaderConfig(t *testing.T) {
	config := DefaultFlowReaderConfig()

	if config.PollInterval != 1*time.Second {
		t.Errorf("PollInterval = %v, want 1s", config.PollInterval)
	}
	if config.StaleTTL != 60*time.Second {
		t.Errorf("StaleTTL = %v, want 60s", config.StaleTTL)
	}
	if config.FlowTopic != "traces.flow" {
		t.Errorf("FlowTopic = %q, want \"traces.flow\"", config.FlowTopic)
	}
	if config.BatchSize != 256 {
		t.Errorf("BatchSize = %d, want 256", config.BatchSize)
	}
}

// ── Struct size assertions ───────────────────────────────────────────────────

func TestStructSizes(t *testing.T) {
	// Verify our Go types match the expected BPF struct sizes
	if FlowKeySize != 16 {
		t.Errorf("FlowKeySize = %d, want 16", FlowKeySize)
	}
	if FlowStateSize != 72 {
		t.Errorf("FlowStateSize = %d, want 72", FlowStateSize)
	}
	if FlowEventSize != 104 {
		t.Errorf("FlowEventSize = %d, want 104", FlowEventSize)
	}

	// Verify encode produces correct sizes
	fk := FlowKey{}
	fkBytes := fk.Encode()
	if len(fkBytes) != FlowKeySize {
		t.Errorf("FlowKey.Encode() len = %d, want %d", len(fkBytes), FlowKeySize)
	}

	fs := FlowStateEntry{}
	fsBytes := fs.Encode()
	if len(fsBytes) != FlowStateSize {
		t.Errorf("FlowStateEntry.Encode() len = %d, want %d", len(fsBytes), FlowStateSize)
	}
}

// ── TCP state transition names ───────────────────────────────────────────────

func TestConnStateName_AllStates(t *testing.T) {
	states := []struct {
		val  uint8
		name string
	}{
		{0, "UNKNOWN"},
		{1, "SYN_SENT"},
		{2, "SYN_RECEIVED"},
		{3, "ESTABLISHED"},
		{4, "FIN_WAIT1"},
		{5, "FIN_WAIT2"},
		{6, "CLOSE_WAIT"},
		{7, "CLOSING"},
		{8, "LAST_ACK"},
		{9, "TIME_WAIT"},
		{10, "CLOSED"},
	}
	for _, s := range states {
		got := ConnStateName(s.val)
		if got != s.name {
			t.Errorf("ConnStateName(%d) = %q, want %q", s.val, got, s.name)
		}
	}
}

// ── FlowKey normalization sanity ─────────────────────────────────────────────

func TestFlowKey_EncodeIsDeterministic(t *testing.T) {
	fk := makeFlowKey([4]byte{192, 168, 1, 1}, [4]byte{10, 0, 0, 1}, 443, 80, 6)

	enc1 := fk.Encode()
	enc2 := fk.Encode()

	if enc1 != enc2 {
		t.Error("FlowKey.Encode() is not deterministic")
	}
}

// ── Multiple state transitions ───────────────────────────────────────────────

func TestFlowReader_MultipleStateTransitions(t *testing.T) {
	loader := NewMockBPFLoader()

	fk := makeFlowKey([4]byte{10, 10, 10, 1}, [4]byte{10, 10, 10, 2}, 8080, 443, 6)
	nowNS := uint64(time.Now().UnixNano())

	states := []uint8{
		ConnStateSynSent,
		ConnStateSynReceived,
		ConnStateEstablished,
		ConnStateFinWait1,
		ConnStateClosed,
	}

	publisher := NewWotanPublisher("localhost:9090", 100, 50*time.Millisecond)
	reader := NewFlowReader(loader, publisher, DefaultFlowReaderConfig())

	var expectedEvents uint64
	for _, state := range states {
		fs := makeFlowState(state, 1000, nowNS, 10, 20, 5000, 10000)
		insertFlow(loader.flowMap, fk, fs)

		reader.pollOnce(context.Background())
		expectedEvents++ // Each poll should produce an event (new or state_change)

		// Drain the event
		select {
		case <-reader.Events():
		case <-time.After(time.Second):
			t.Fatalf("timeout at state %d", state)
		}
	}

	stats := reader.Stats()
	if stats.EventsSent != expectedEvents {
		t.Errorf("EventsSent = %d, want %d", stats.EventsSent, expectedEvents)
	}

	_ = fmt.Sprintf("traced %d state transitions", len(states))
}
