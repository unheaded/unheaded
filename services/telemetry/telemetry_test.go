// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package telemetry

import (
	"os"
	"testing"
	"time"

	"unheaded/pkg/logger"
)

// ── TestNewService ───────────────────────────────────────────────────────

func TestNewService(t *testing.T) {
	log := logger.New(os.Stderr)
	svc := NewService(log, nil)

	if svc == nil {
		t.Fatal("NewService returned nil")
	}
	if svc.log != log {
		t.Error("logger not set correctly")
	}
	if svc.wotan != nil {
		t.Error("expected nil wotan client")
	}
	if svc.maxHops != 100000 {
		t.Errorf("expected maxHops=100000, got %d", svc.maxHops)
	}
	if svc.maxPaths != 10000 {
		t.Errorf("expected maxPaths=10000, got %d", svc.maxPaths)
	}
	if svc.windows == nil {
		t.Error("windows map not initialized")
	}

	stats := svc.Stats()
	if stats["hops_stored"].(int) != 0 {
		t.Error("expected 0 hops stored initially")
	}
}

// ── TestHopMetricAggregation ─────────────────────────────────────────────

func TestHopMetricAggregation(t *testing.T) {
	log := logger.New(os.Stderr)
	svc := NewService(log, nil)

	// Ingest a set of hop metrics with known latencies for a single
	// service pair so we can verify percentile calculation.
	now := time.Now()
	windowStart := now.Truncate(time.Second)

	latencies := []time.Duration{
		1 * time.Millisecond,
		2 * time.Millisecond,
		3 * time.Millisecond,
		4 * time.Millisecond,
		5 * time.Millisecond,
		6 * time.Millisecond,
		7 * time.Millisecond,
		8 * time.Millisecond,
		9 * time.Millisecond,
		10 * time.Millisecond,
	}

	for i, lat := range latencies {
		svc.IngestHopForTest(HopMetric{
			TraceID:    "trace-001",
			HopIndex:   uint8(i),
			Timestamp:  windowStart,
			Latency:    lat,
			QueueDepth: uint32(i + 1),
			LinkUtil:   float64(i) * 0.1,
			ServiceID:  0x0A,
		})
	}

	// Verify hops were stored.
	hops := svc.RecentHops(5)
	if len(hops) != len(latencies) {
		t.Errorf("expected %d hops, got %d", len(latencies), len(hops))
	}

	// Flush the window to compute aggregates.
	svc.flushExpiredWindows()

	// The aggregation window may not be flushed yet if windowStart equals
	// the current truncated second. Force a flush by manipulating the
	// window bucket directly.
	svc.aggMu.Lock()
	for key, bucket := range svc.windows {
		if bucket.sampleCount > 0 {
			svc.flushBucket(key, bucket)
			delete(svc.windows, key)
		}
	}
	svc.aggMu.Unlock()

	aggs := svc.RecentAggregates(100)
	if len(aggs) == 0 {
		t.Fatal("expected at least one aggregate after flush")
	}

	agg := aggs[len(aggs)-1]
	if agg.PacketCount != int64(len(latencies)) {
		t.Errorf("expected packet_count=%d, got %d", len(latencies), agg.PacketCount)
	}

	// P50 of [1..10]ms should be approximately 5.5ms.
	p50Expected := 5500 * time.Microsecond
	p50Tolerance := 1 * time.Millisecond
	if abs(agg.LatencyP50-p50Expected) > p50Tolerance {
		t.Errorf("P50 latency: expected ~%v, got %v", p50Expected, agg.LatencyP50)
	}

	// P99 should be close to 10ms.
	if agg.LatencyP99 < 9*time.Millisecond {
		t.Errorf("P99 latency: expected >= 9ms, got %v", agg.LatencyP99)
	}

	// Queue depth average: (1+2+...+10)/10 = 5.5
	if agg.QueueDepthAvg < 5.0 || agg.QueueDepthAvg > 6.0 {
		t.Errorf("QueueDepthAvg: expected ~5.5, got %f", agg.QueueDepthAvg)
	}
}

// ── TestServicePairWindowing ─────────────────────────────────────────────

func TestServicePairWindowing(t *testing.T) {
	log := logger.New(os.Stderr)
	svc := NewService(log, nil)

	// Ingest hops into two different 1-second windows.
	t1 := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 3, 1, 12, 0, 1, 0, time.UTC)

	// Window 1 samples.
	for i := 0; i < 5; i++ {
		svc.IngestHopForTest(HopMetric{
			TraceID:   "w1-trace",
			HopIndex:  uint8(i),
			Timestamp: t1,
			Latency:   time.Duration(i+1) * time.Millisecond,
			ServiceID: 0x01,
		})
	}

	// Window 2 samples — trigger flush of window 1 by using a later timestamp.
	for i := 0; i < 3; i++ {
		svc.IngestHopForTest(HopMetric{
			TraceID:   "w2-trace",
			HopIndex:  uint8(i),
			Timestamp: t2,
			Latency:   time.Duration(i+10) * time.Millisecond,
			ServiceID: 0x01,
		})
	}

	// Force flush all windows.
	svc.aggMu.Lock()
	for key, bucket := range svc.windows {
		if bucket.sampleCount > 0 {
			svc.flushBucket(key, bucket)
			delete(svc.windows, key)
		}
	}
	svc.aggMu.Unlock()

	aggs := svc.RecentAggregates(100)
	if len(aggs) < 2 {
		t.Fatalf("expected at least 2 aggregates (one per window), got %d", len(aggs))
	}

	// First aggregate should have 5 samples, second should have 3.
	if aggs[0].PacketCount != 5 {
		t.Errorf("window 1: expected 5 packets, got %d", aggs[0].PacketCount)
	}
	if aggs[1].PacketCount != 3 {
		t.Errorf("window 2: expected 3 packets, got %d", aggs[1].PacketCount)
	}

	// Windows should be different timestamps.
	if aggs[0].Window.Equal(aggs[1].Window) {
		t.Error("expected different window timestamps for different seconds")
	}
}

// ── TestPathReconstruction ───────────────────────────────────────────────

func TestPathReconstruction(t *testing.T) {
	log := logger.New(os.Stderr)
	svc := NewService(log, nil)

	// Build a path from individual hops.
	now := time.Now()
	hops := []HopMetric{
		{TraceID: "path-001", HopIndex: 0, Timestamp: now, Latency: 1 * time.Millisecond, ServiceID: 0x01},
		{TraceID: "path-001", HopIndex: 1, Timestamp: now.Add(2 * time.Millisecond), Latency: 2 * time.Millisecond, ServiceID: 0x02},
		{TraceID: "path-001", HopIndex: 2, Timestamp: now.Add(5 * time.Millisecond), Latency: 3 * time.Millisecond, ServiceID: 0x03},
		{TraceID: "path-001", HopIndex: 3, Timestamp: now.Add(9 * time.Millisecond), Latency: 4 * time.Millisecond, ServiceID: 0x04},
	}

	// Ingest hops individually.
	for _, hop := range hops {
		svc.IngestHopForTest(hop)
	}

	// Construct a path from the hops and ingest it.
	path := PathMetric{
		TraceID: "path-001",
		Hops:    hops,
	}
	svc.IngestPathForTest(path)

	// Verify path was stored with correct total latency.
	paths := svc.RecentPaths(10)
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d", len(paths))
	}

	p := paths[0]
	if p.TraceID != "path-001" {
		t.Errorf("expected trace_id=path-001, got %s", p.TraceID)
	}
	if p.HopCount != 4 {
		t.Errorf("expected hop_count=4, got %d", p.HopCount)
	}

	// TotalLatency should be calculated from first to last hop timestamps:
	// 9ms - 0ms = 9ms.
	expectedLatency := 9 * time.Millisecond
	if p.TotalLatency != expectedLatency {
		t.Errorf("expected total_latency=%v, got %v", expectedLatency, p.TotalLatency)
	}

	// Verify hops are intact.
	if len(p.Hops) != 4 {
		t.Fatalf("expected 4 hops in path, got %d", len(p.Hops))
	}
	for i, hop := range p.Hops {
		if hop.HopIndex != uint8(i) {
			t.Errorf("hop %d: expected index=%d, got %d", i, i, hop.HopIndex)
		}
	}

	// Verify stats reflect ingestion.
	stats := svc.Stats()
	hopsReceived := stats["hops_received"].(int64)
	pathsReceived := stats["paths_received"].(int64)
	if hopsReceived != 4 {
		t.Errorf("expected hops_received=4, got %d", hopsReceived)
	}
	if pathsReceived != 1 {
		t.Errorf("expected paths_received=1, got %d", pathsReceived)
	}
}

// ── Test helpers ─────────────────────────────────────────────────────────

// abs returns the absolute value of a time.Duration.
func abs(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// ── TestPercentile ───────────────────────────────────────────────────────

func TestPercentile(t *testing.T) {
	tests := []struct {
		name   string
		values []time.Duration
		p      float64
		expect time.Duration
		tol    time.Duration
	}{
		{
			name:   "empty",
			values: nil,
			p:      50,
			expect: 0,
		},
		{
			name:   "single",
			values: []time.Duration{5 * time.Millisecond},
			p:      99,
			expect: 5 * time.Millisecond,
		},
		{
			name:   "two values p50",
			values: []time.Duration{1 * time.Millisecond, 3 * time.Millisecond},
			p:      50,
			expect: 2 * time.Millisecond,
			tol:    100 * time.Microsecond,
		},
		{
			name:   "ten values p95",
			values: []time.Duration{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			p:      95,
			expect: 9,
			tol:    1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := percentile(tc.values, tc.p)
			if tc.tol == 0 {
				if got != tc.expect {
					t.Errorf("percentile(%v, %v) = %v, want %v", tc.values, tc.p, got, tc.expect)
				}
			} else {
				if abs(got-tc.expect) > tc.tol {
					t.Errorf("percentile(%v, %v) = %v, want ~%v (tol %v)", tc.values, tc.p, got, tc.expect, tc.tol)
				}
			}
		})
	}
}
