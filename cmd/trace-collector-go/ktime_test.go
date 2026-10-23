// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"net"
	"testing"
	"time"
)

// A live flow must be announced once and stay known. On east (2026-10-23)
// flow_tracker's last_seen_ns was ~8.6e14 (ktime, ~10 days of uptime) and the
// reader compared it with time.Now().UnixNano(): every flow expired in the
// poll that found it, so each 1 s poll re-published all 1025 flows as "new"
// (3 times each in a 2 s sample) with an "expired" right behind.
func TestFlowReader_LiveFlowNotExpiredEachPoll(t *testing.T) {
	loader := NewMockBPFLoader()
	now := ktimeNow()
	fk := makeFlowKey([4]byte{127, 0, 0, 1}, [4]byte{127, 0, 0, 1}, 19005, 51738, 6)
	insertFlow(loader.flowMap, fk, makeFlowState(ConnStateEstablished, now-3e9, now, 3, 3, 222, 162))

	reader := NewFlowReader(loader, nil, DefaultFlowReaderConfig())
	reader.pollOnce(context.Background())
	reader.pollOnce(context.Background())

	var got []string
	for len(reader.Events()) > 0 {
		got = append(got, (<-reader.Events()).EventTypeName())
	}
	if len(got) != 1 || got[0] != "new" {
		t.Errorf("events over two polls = %v, want [new]", got)
	}
	if s := reader.Stats(); s.FlowsExpired != 0 {
		t.Errorf("FlowsExpired = %d, want 0", s.FlowsExpired)
	}
	if reader.ActiveFlows() != 1 {
		t.Errorf("ActiveFlows = %d, want 1", reader.ActiveFlows())
	}
}

// RunGC must age flows on the packets' clock: against wall clock every flow
// was ~1.8e18 ns old and was dropped at the first GC tick, not after FlowTTL.
func TestPacketCorrelator_RunGCKeepsFreshFlow(t *testing.T) {
	config := DefaultPacketCorrelatorConfig()
	config.GCInterval = 2 * time.Millisecond
	pc := NewPacketCorrelator(config)
	pc.Add(makeTraceEntry(net.ParseIP("127.0.0.1"), net.ParseIP("127.0.0.1"), 56636, 18001, 6, ktimeNow()))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	pc.RunGC(ctx)

	if pc.Stats().GCCycles == 0 {
		t.Fatal("GC never ran")
	}
	if pc.Len() != 1 {
		t.Errorf("Len() = %d after GC ticks, want 1 (flow ~30ms old, TTL %v)", pc.Len(), config.FlowTTL)
	}
}

func TestOlderThan(t *testing.T) {
	for _, c := range []struct {
		now, ts, ttl uint64
		want         bool
	}{
		{100, 50, 10, true},
		{100, 95, 10, false},
		{100, 90, 10, false},
		{100, 150, 10, false}, // ts read after now: not ancient
	} {
		if got := olderThan(c.now, c.ts, c.ttl); got != c.want {
			t.Errorf("olderThan(%d, %d, %d) = %v, want %v", c.now, c.ts, c.ttl, got, c.want)
		}
	}
}
