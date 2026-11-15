// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package ebpf

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

// ADR-147 Phase 10: the collector publishes each trace event once, on
// traces.*; the ingestor reads those topics and none of the retired
// ebpf.{packet,flow,latency}.events mirrors.
func TestIngestorReadsTraces(t *testing.T) {
	has := map[string]bool{}
	for _, tp := range Topics {
		has[tp] = true
	}
	for _, want := range []string{"traces.packet", "traces.flow", "traces.latency"} {
		if !has[want] {
			t.Errorf("Topics lacks %s", want)
		}
	}
	for _, gone := range []string{"ebpf.packet.events", "ebpf.flow.events", "ebpf.latency.events"} {
		if has[gone] {
			t.Errorf("Topics still holds the mirror %s", gone)
		}
	}

	mock := newMockWotanClient()
	cfg := DefaultIngestorConfig()
	cfg.ExpireInterval = time.Hour
	ing := NewIngestor(cfg, mock, nil)
	var mu sync.Mutex
	var seen []string
	ing.OnMessage(func(topic, _ string, _ time.Time) {
		mu.Lock()
		seen = append(seen, topic)
		mu.Unlock()
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := ing.Start(ctx); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)

	fe := FlowEvent{
		TimestampNs: 1,
		FlowKey:     FlowKey{SrcAddr: "10.0.0.1", DstAddr: "10.0.0.2", SrcPort: 80, DstPort: 443, Protocol: 6},
		FlowState:   FlowState{State: StateEstablished},
		EventType:   FlowNew,
	}
	data, _ := json.Marshal([]FlowEvent{fe})
	mock.send("traces.flow", string(data))
	deadline := time.Now().Add(2 * time.Second)
	for ing.Stats().FlowsIngested == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	if n := ing.Stats().FlowsIngested; n != 1 {
		t.Errorf("flows_ingested = %d, want 1", n)
	}
	if n := ing.FlowGraph().ActiveFlowCount(); n != 1 {
		t.Errorf("active flows = %d, want 1", n)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 || seen[0] != "traces.flow" {
		t.Errorf("message listeners saw %v, want one traces.flow", seen)
	}
}
