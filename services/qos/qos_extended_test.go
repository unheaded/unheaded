// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package qos

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/logger"
	wotanClient "unheaded/pkg/wotan-client"
)

// ---------------------------------------------------------------------------
// TestAddr
// ---------------------------------------------------------------------------

func TestAddr(t *testing.T) {
	log := logger.New(io.Discard)
	svc := NewService(log, nil)

	// Before Start, listener is nil so Addr returns "".
	if got := svc.Addr(); got != "" {
		t.Errorf("Addr before Start = %q, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// TestStartStop — exercises Start (with nil wotan) and Stop.
// ---------------------------------------------------------------------------

func TestStartStop(t *testing.T) {
	log := logger.New(io.Discard)
	svc := NewService(log, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Addr should now be non-empty.
	addr := svc.Addr()
	if addr == "" {
		t.Fatal("expected non-empty Addr after Start")
	}

	// Quick smoke test: hit /health on the real listener.
	resp, err := http.Get("http://" + addr + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /health status = %d, want 200", resp.StatusCode)
	}

	// Stop should succeed.
	cancel() // cancel ctx so aggregateStats goroutine exits
	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestHandleCriticalAlert
// ---------------------------------------------------------------------------

func TestHandleCriticalAlert(t *testing.T) {
	log := logger.New(io.Discard)
	svc := NewService(log, nil)

	// nil message — should not panic.
	svc.handleCriticalAlert(nil)

	// Non-nil message — should log and not panic.
	svc.handleCriticalAlert(&wotanClient.Message{
		MessageID: "alert-001",
		Topic:     "alerts.critical",
		Payload:   `{"level":"critical","msg":"test"}`,
	})
}

// ---------------------------------------------------------------------------
// TestListenForAlertsNilWotan
// ---------------------------------------------------------------------------

func TestListenForAlertsNilWotan(t *testing.T) {
	log := logger.New(io.Discard)
	svc := NewService(log, nil) // wotan == nil

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Should return immediately (wotan == nil path).
	done := make(chan struct{})
	go func() {
		svc.listenForAlerts(ctx)
		close(done)
	}()

	select {
	case <-done:
		// success — returned immediately
	case <-time.After(2 * time.Second):
		t.Fatal("listenForAlerts did not return for nil wotan")
	}
}

// ---------------------------------------------------------------------------
// TestAggregateStatsContextCancel
// ---------------------------------------------------------------------------

func TestAggregateStatsContextCancel(t *testing.T) {
	log := logger.New(io.Discard)
	svc := NewService(log, nil)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		svc.aggregateStats(ctx)
		close(done)
	}()

	// Cancel immediately to exercise the ctx.Done() branch.
	cancel()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("aggregateStats did not exit on context cancel")
	}
}

// ---------------------------------------------------------------------------
// TestAggregateStatsPublishesSnapshot
// ---------------------------------------------------------------------------

func TestAggregateStatsPublishesSnapshot(t *testing.T) {
	log := logger.New(io.Discard)
	svc := NewService(log, nil) // nil wotan → drops incremented

	// Add stats so publishStatsSnapshot does work.
	svc.UpdateStats(&QueueStats{ServiceID: "agg-svc", Packets: 42})

	before := svc.WotanDrops()

	ctx, cancel := context.WithCancel(context.Background())

	go svc.aggregateStats(ctx)

	// Wait long enough for at least one tick (100ms).
	time.Sleep(250 * time.Millisecond)
	cancel()

	after := svc.WotanDrops()
	if after <= before {
		t.Errorf("expected WotanDrops to increase after aggregation tick; before=%d after=%d", before, after)
	}
}

// ---------------------------------------------------------------------------
// TestPublishPolicyUpdateNilWotan
// ---------------------------------------------------------------------------

func TestPublishPolicyUpdateNilWotan(t *testing.T) {
	log := logger.New(io.Discard)
	svc := NewService(log, nil)

	before := svc.WotanDrops()
	svc.publishPolicyUpdate("svc-1", &QoSPolicy{
		ServiceID: "svc-1",
		Weight:    2,
	})
	after := svc.WotanDrops()
	if after != before+1 {
		t.Errorf("WotanDrops = %d, want %d", after, before+1)
	}
}

// ---------------------------------------------------------------------------
// TestDetectCongestionDropProbOnOpenCircuit
// ---------------------------------------------------------------------------

// Covers the branch where DropProbability > 0.1 AND circuit is already OPEN.
// The suggestion should contain the drop probability suffix appended to the
// latency-based message.
func TestDetectCongestionDropProbOnOpenCircuit(t *testing.T) {
	log := logger.New(io.Discard)
	svc := NewService(log, nil)

	_ = svc.SetPolicy(&QoSPolicy{
		ServiceID:       "dp-open",
		Weight:          1,
		TargetLatencyMS: 5,
	})

	svc.UpdateStats(&QueueStats{
		ServiceID:       "dp-open",
		LatencyP99:      30.0, // > 5x target → OPEN
		DropProbability: 0.2,  // > 0.1 → appends drop probability text
	})

	info := svc.DetectCongestionExported("dp-open")
	if info.CircuitState != CircuitOpen {
		t.Errorf("expected OPEN, got %s", info.CircuitState)
	}
	if !strings.Contains(info.Suggestion, "drop probability") {
		t.Errorf("suggestion should mention drop probability: %s", info.Suggestion)
	}
	if !strings.Contains(info.Suggestion, "circuit OPEN") {
		t.Errorf("suggestion should mention circuit OPEN: %s", info.Suggestion)
	}
}

// ---------------------------------------------------------------------------
// TestDetectCongestionDropProbOnHalfOpenCircuit
// ---------------------------------------------------------------------------

// DropProbability > 0.1 when circuit is already HALF_OPEN (latency-based).
// The circuit stays HALF_OPEN but the suggestion gains the drop probability text.
func TestDetectCongestionDropProbOnHalfOpenCircuit(t *testing.T) {
	log := logger.New(io.Discard)
	svc := NewService(log, nil)

	_ = svc.SetPolicy(&QoSPolicy{
		ServiceID:       "dp-half",
		Weight:          1,
		TargetLatencyMS: 5,
	})

	svc.UpdateStats(&QueueStats{
		ServiceID:       "dp-half",
		LatencyP99:      12.0, // > 2x target → HALF_OPEN
		DropProbability: 0.15, // > 0.1 → appends drop probability text
	})

	info := svc.DetectCongestionExported("dp-half")
	if info.CircuitState != CircuitHalfOpen {
		t.Errorf("expected HALF_OPEN, got %s", info.CircuitState)
	}
	if !strings.Contains(info.Suggestion, "drop probability") {
		t.Errorf("suggestion should mention drop probability: %s", info.Suggestion)
	}
}

// ---------------------------------------------------------------------------
// TestDetectCongestionNoPolicy
// ---------------------------------------------------------------------------

// Service has stats but no policy → returns CLOSED (early return).
func TestDetectCongestionNoPolicy(t *testing.T) {
	log := logger.New(io.Discard)
	svc := NewService(log, nil)

	// Add stats without a matching policy.
	svc.UpdateStats(&QueueStats{
		ServiceID:  "no-policy",
		LatencyP99: 100.0,
	})

	info := svc.DetectCongestionExported("no-policy")
	if info.CircuitState != CircuitClosed {
		t.Errorf("expected CLOSED for service with no policy, got %s", info.CircuitState)
	}
	if info.Suggestion != "traffic normal" {
		t.Errorf("unexpected suggestion: %s", info.Suggestion)
	}
}

// ---------------------------------------------------------------------------
// TestPublishStatsSnapshotEmptyStats
// ---------------------------------------------------------------------------

// Empty stats → early return, no wotan drop increment.
func TestPublishStatsSnapshotEmptyStats(t *testing.T) {
	log := logger.New(io.Discard)
	svc := NewService(log, nil)

	before := svc.WotanDrops()
	svc.publishStatsSnapshot(context.Background())
	after := svc.WotanDrops()

	if after != before {
		t.Errorf("WotanDrops changed (%d → %d) with empty stats", before, after)
	}
}

// ---------------------------------------------------------------------------
// TestDetectCongestionNegativeTarget — policy with negative TargetLatencyMS
// ---------------------------------------------------------------------------

func TestDetectCongestionNegativeTarget(t *testing.T) {
	log := logger.New(io.Discard)
	svc := NewService(log, nil)

	// Bypass Validate() by writing directly into the map to set a negative
	// target (Validate would fix it). This exercises the targetMS <= 0 fallback
	// inside detectCongestion.
	svc.mu.Lock()
	svc.policies["neg-target"] = &QoSPolicy{
		ServiceID:       "neg-target",
		Weight:          1,
		TargetLatencyMS: -1,
	}
	svc.mu.Unlock()

	svc.UpdateStats(&QueueStats{
		ServiceID:  "neg-target",
		LatencyP99: 30.0, // > 5x default (5ms)
	})

	info := svc.DetectCongestionExported("neg-target")
	if info.CircuitState != CircuitOpen {
		t.Errorf("expected OPEN with negative target fallback, got %s", info.CircuitState)
	}
}
