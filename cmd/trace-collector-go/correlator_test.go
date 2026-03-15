// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package main

import (
	"net"
	"testing"
	"time"
)

// ── PacketCorrelator tests ──────────────────────────────────────────────────

func TestPacketCorrelator_ForwardOnly(t *testing.T) {
	pc := NewPacketCorrelator(DefaultPacketCorrelatorConfig())

	entry := makeTraceEntry(
		net.ParseIP("10.0.0.1"),
		net.ParseIP("10.0.0.2"),
		12345, 80, 6, 1000000,
	)

	pc.Add(entry)

	// Only one direction — should not match
	select {
	case <-pc.CorrelatedFlows():
		t.Error("should not emit correlated flow with only forward direction")
	case <-time.After(50 * time.Millisecond):
		// Expected
	}

	if pc.Len() != 1 {
		t.Errorf("Len() = %d, want 1", pc.Len())
	}

	stats := pc.Stats()
	if stats.EntriesProcessed != 1 {
		t.Errorf("EntriesProcessed = %d, want 1", stats.EntriesProcessed)
	}
	if stats.FlowsCreated != 1 {
		t.Errorf("FlowsCreated = %d, want 1", stats.FlowsCreated)
	}
}

func TestPacketCorrelator_BidirectionalMatch(t *testing.T) {
	pc := NewPacketCorrelator(DefaultPacketCorrelatorConfig())

	// Forward: 10.0.0.1:12345 -> 10.0.0.2:80
	forward := makeTraceEntry(
		net.ParseIP("10.0.0.1"),
		net.ParseIP("10.0.0.2"),
		12345, 80, 6, 1000000,
	)

	// Reverse: 10.0.0.2:80 -> 10.0.0.1:12345
	reverse := makeTraceEntry(
		net.ParseIP("10.0.0.2"),
		net.ParseIP("10.0.0.1"),
		80, 12345, 6, 2000000,
	)

	pc.Add(forward)
	pc.Add(reverse)

	// Should get a correlated flow
	select {
	case flow := <-pc.CorrelatedFlows():
		if !flow.Matched {
			t.Error("expected Matched = true")
		}
		if flow.RTTNS == 0 {
			t.Error("expected non-zero RTT")
		}
		if flow.RTTNS != 1000000 {
			t.Errorf("RTTNS = %d, want 1000000", flow.RTTNS)
		}
		if flow.Forward == nil {
			t.Error("expected non-nil Forward")
		}
		if flow.Reverse == nil {
			t.Error("expected non-nil Reverse")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for correlated flow")
	}

	stats := pc.Stats()
	if stats.FlowsMatched != 1 {
		t.Errorf("FlowsMatched = %d, want 1", stats.FlowsMatched)
	}
}

func TestPacketCorrelator_RTTCalculation(t *testing.T) {
	pc := NewPacketCorrelator(DefaultPacketCorrelatorConfig())

	// Forward at t=100ms
	forward := makeTraceEntry(
		net.ParseIP("10.0.0.1"),
		net.ParseIP("10.0.0.2"),
		9999, 443, 6, 100_000_000, // 100ms in ns
	)
	// Reverse at t=105ms (5ms RTT)
	reverse := makeTraceEntry(
		net.ParseIP("10.0.0.2"),
		net.ParseIP("10.0.0.1"),
		443, 9999, 6, 105_000_000, // 105ms in ns
	)

	pc.Add(forward)
	pc.Add(reverse)

	select {
	case flow := <-pc.CorrelatedFlows():
		expectedRTT := 5 * time.Millisecond
		if flow.RTTDuration() != expectedRTT {
			t.Errorf("RTTDuration = %v, want %v", flow.RTTDuration(), expectedRTT)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestPacketCorrelator_MultipleUpdates(t *testing.T) {
	pc := NewPacketCorrelator(DefaultPacketCorrelatorConfig())

	// Multiple packets in the same direction
	for i := uint64(0); i < 5; i++ {
		entry := makeTraceEntry(
			net.ParseIP("10.0.0.1"),
			net.ParseIP("10.0.0.2"),
			8080, 80, 6, i*1000,
		)
		pc.Add(entry)
	}

	// Should have 1 flow with packet count 5
	if pc.Len() != 1 {
		t.Errorf("Len() = %d, want 1 (all same 5-tuple)", pc.Len())
	}

	stats := pc.Stats()
	if stats.EntriesProcessed != 5 {
		t.Errorf("EntriesProcessed = %d, want 5", stats.EntriesProcessed)
	}
	// Only 1 flow created (rest are updates)
	if stats.FlowsCreated != 1 {
		t.Errorf("FlowsCreated = %d, want 1", stats.FlowsCreated)
	}
}

func TestPacketCorrelator_DifferentProtocols(t *testing.T) {
	pc := NewPacketCorrelator(DefaultPacketCorrelatorConfig())

	// TCP flow
	tcpEntry := makeTraceEntry(
		net.ParseIP("10.0.0.1"),
		net.ParseIP("10.0.0.2"),
		8080, 80, 6, 1000,
	)
	// UDP flow (same IPs and ports, different protocol)
	udpEntry := makeTraceEntry(
		net.ParseIP("10.0.0.1"),
		net.ParseIP("10.0.0.2"),
		8080, 80, 17, 2000,
	)

	pc.Add(tcpEntry)
	pc.Add(udpEntry)

	// Should track as 2 separate flows
	if pc.Len() != 2 {
		t.Errorf("Len() = %d, want 2 (different protocols)", pc.Len())
	}
}

func TestPacketCorrelator_GC(t *testing.T) {
	config := DefaultPacketCorrelatorConfig()
	config.FlowTTL = 100 * time.Millisecond
	pc := NewPacketCorrelator(config)

	// Add a flow at t=0
	entry := makeTraceEntry(
		net.ParseIP("10.0.0.1"),
		net.ParseIP("10.0.0.2"),
		1234, 80, 6, 1000,
	)
	pc.Add(entry)

	// GC with "now" far in the future (1 second)
	removed := pc.GC(1000 + uint64(time.Second.Nanoseconds()))

	if removed != 1 {
		t.Errorf("GC removed = %d, want 1", removed)
	}
	if pc.Len() != 0 {
		t.Errorf("Len() = %d after GC, want 0", pc.Len())
	}

	stats := pc.Stats()
	if stats.FlowsExpired != 1 {
		t.Errorf("FlowsExpired = %d, want 1", stats.FlowsExpired)
	}
}

func TestPacketCorrelator_GCKeepsFresh(t *testing.T) {
	config := DefaultPacketCorrelatorConfig()
	config.FlowTTL = 1 * time.Second
	pc := NewPacketCorrelator(config)

	nowNS := uint64(time.Now().UnixNano())

	// Add a fresh flow
	entry := makeTraceEntry(
		net.ParseIP("10.0.0.1"),
		net.ParseIP("10.0.0.2"),
		1234, 80, 6, nowNS,
	)
	pc.Add(entry)

	// GC at "now" — flow should survive
	removed := pc.GC(nowNS)

	if removed != 0 {
		t.Errorf("GC removed = %d, want 0 (flow is fresh)", removed)
	}
	if pc.Len() != 1 {
		t.Errorf("Len() = %d after GC, want 1", pc.Len())
	}
}

func TestPacketCorrelator_StatsAccuracy(t *testing.T) {
	pc := NewPacketCorrelator(DefaultPacketCorrelatorConfig())

	// Add entries and correlate
	for i := 0; i < 5; i++ {
		forward := makeTraceEntry(
			net.ParseIP("10.0.0.1"),
			net.ParseIP("10.0.0.2"),
			uint16(1000+i), 80, 6, uint64(i*1000),
		)
		reverse := makeTraceEntry(
			net.ParseIP("10.0.0.2"),
			net.ParseIP("10.0.0.1"),
			80, uint16(1000+i), 6, uint64(i*1000+500),
		)
		pc.Add(forward)
		pc.Add(reverse)
	}

	stats := pc.Stats()
	if stats.EntriesProcessed != 10 {
		t.Errorf("EntriesProcessed = %d, want 10", stats.EntriesProcessed)
	}
	if stats.FlowsMatched != 5 {
		t.Errorf("FlowsMatched = %d, want 5", stats.FlowsMatched)
	}

	// Drain correlated flows
	for i := 0; i < 5; i++ {
		select {
		case <-pc.CorrelatedFlows():
		case <-time.After(time.Second):
			t.Fatalf("timeout draining flow %d", i)
		}
	}
}

// ── CorrelatedFlow tests ────────────────────────────────────────────────────

func TestCorrelatedFlow_RTTDuration(t *testing.T) {
	cf := &CorrelatedFlow{
		RTTNS: 5_000_000, // 5ms
	}
	if cf.RTTDuration() != 5*time.Millisecond {
		t.Errorf("RTTDuration = %v, want 5ms", cf.RTTDuration())
	}
}

func TestCorrelatedFlow_ZeroRTT(t *testing.T) {
	cf := &CorrelatedFlow{
		RTTNS: 0,
	}
	if cf.RTTDuration() != 0 {
		t.Errorf("RTTDuration = %v, want 0", cf.RTTDuration())
	}
}
