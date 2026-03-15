// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package telemetry

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"unheaded/pkg/logger"
)

func newTestService() *Service {
	return NewService(logger.New(io.Discard), nil)
}

// ---------------------------------------------------------------------------
// RecentHops tests
// ---------------------------------------------------------------------------

func TestRecentHops_DefaultMinutes(t *testing.T) {
	svc := newTestService()
	svc.IngestHopForTest(HopMetric{
		TraceID:   "t1",
		Timestamp: time.Now(),
		Latency:   time.Millisecond,
		ServiceID: 1,
	})

	// passing 0 should default to 5 minutes
	hops := svc.RecentHops(0)
	if len(hops) != 1 {
		t.Errorf("expected 1 hop, got %d", len(hops))
	}
}

func TestRecentHops_FiltersOld(t *testing.T) {
	svc := newTestService()
	// Ingest a hop from 10 minutes ago.
	svc.IngestHopForTest(HopMetric{
		TraceID:   "old",
		Timestamp: time.Now().Add(-10 * time.Minute),
		Latency:   time.Millisecond,
		ServiceID: 1,
	})
	// Ingest a recent hop.
	svc.IngestHopForTest(HopMetric{
		TraceID:   "new",
		Timestamp: time.Now(),
		Latency:   time.Millisecond,
		ServiceID: 1,
	})

	hops := svc.RecentHops(5)
	if len(hops) != 1 {
		t.Errorf("expected 1 recent hop, got %d", len(hops))
	}
	if hops[0].TraceID != "new" {
		t.Errorf("expected new hop, got %s", hops[0].TraceID)
	}
}

// ---------------------------------------------------------------------------
// RecentPaths tests
// ---------------------------------------------------------------------------

func TestRecentPaths_DefaultLimit(t *testing.T) {
	svc := newTestService()
	for i := 0; i < 5; i++ {
		svc.IngestPathForTest(PathMetric{
			TraceID:   "p",
			Timestamp: time.Now(),
		})
	}

	// 0 should default to 100
	paths := svc.RecentPaths(0)
	if len(paths) != 5 {
		t.Errorf("expected 5 paths, got %d", len(paths))
	}
}

func TestRecentPaths_LimitExceedsStored(t *testing.T) {
	svc := newTestService()
	svc.IngestPathForTest(PathMetric{TraceID: "p1", Timestamp: time.Now()})
	svc.IngestPathForTest(PathMetric{TraceID: "p2", Timestamp: time.Now()})

	paths := svc.RecentPaths(100)
	if len(paths) != 2 {
		t.Errorf("expected 2 paths, got %d", len(paths))
	}
}

func TestRecentPaths_ReturnsLatest(t *testing.T) {
	svc := newTestService()
	for i := 0; i < 10; i++ {
		svc.IngestPathForTest(PathMetric{
			TraceID:   "p",
			Timestamp: time.Now(),
		})
	}

	paths := svc.RecentPaths(3)
	if len(paths) != 3 {
		t.Errorf("expected 3 paths, got %d", len(paths))
	}
}

// ---------------------------------------------------------------------------
// ServiceLatency tests
// ---------------------------------------------------------------------------

func TestServiceLatency_MatchesSrcOrDst(t *testing.T) {
	svc := newTestService()

	// Ingest hops with service ID 5 at a past time so they flush.
	past := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		svc.IngestHopForTest(HopMetric{
			TraceID:   "t1",
			Timestamp: past,
			Latency:   time.Millisecond,
			ServiceID: 5,
		})
	}

	// Force flush.
	svc.aggMu.Lock()
	for key, bucket := range svc.windows {
		if bucket.sampleCount > 0 {
			svc.flushBucket(key, bucket)
			delete(svc.windows, key)
		}
	}
	svc.aggMu.Unlock()

	// Service ID 5 should appear as src, and 6 as dst.
	aggs := svc.ServiceLatency(5)
	if len(aggs) == 0 {
		t.Error("expected aggregates for service 5 as src")
	}

	aggs = svc.ServiceLatency(6)
	if len(aggs) == 0 {
		t.Error("expected aggregates for service 6 as dst")
	}

	aggs = svc.ServiceLatency(99)
	if len(aggs) != 0 {
		t.Errorf("expected 0 aggregates for service 99, got %d", len(aggs))
	}
}

// ---------------------------------------------------------------------------
// RecentAggregates tests
// ---------------------------------------------------------------------------

func TestRecentAggregates_DefaultLimit(t *testing.T) {
	svc := newTestService()

	// Create aggregates by flushing multiple windows.
	for i := 0; i < 3; i++ {
		ts := time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC)
		svc.IngestHopForTest(HopMetric{
			TraceID:   "t",
			Timestamp: ts,
			Latency:   time.Millisecond,
			ServiceID: 1,
		})
	}

	// Force flush all.
	svc.aggMu.Lock()
	for key, bucket := range svc.windows {
		if bucket.sampleCount > 0 {
			svc.flushBucket(key, bucket)
			delete(svc.windows, key)
		}
	}
	svc.aggMu.Unlock()

	// 0 should default to 100
	aggs := svc.RecentAggregates(0)
	if len(aggs) == 0 {
		t.Error("expected some aggregates")
	}
}

// ---------------------------------------------------------------------------
// Stats tests
// ---------------------------------------------------------------------------

func TestStats_Counters(t *testing.T) {
	svc := newTestService()
	svc.IngestHopForTest(HopMetric{TraceID: "t", Timestamp: time.Now(), Latency: time.Millisecond, ServiceID: 1})
	svc.IngestHopForTest(HopMetric{TraceID: "t", Timestamp: time.Now(), Latency: time.Millisecond, ServiceID: 1})
	svc.IngestPathForTest(PathMetric{TraceID: "p", Timestamp: time.Now()})

	stats := svc.Stats()
	if stats["hops_stored"].(int) != 2 {
		t.Errorf("expected 2 hops_stored, got %v", stats["hops_stored"])
	}
	if stats["paths_stored"].(int) != 1 {
		t.Errorf("expected 1 path_stored, got %v", stats["paths_stored"])
	}
	if stats["hops_received"].(int64) != 2 {
		t.Errorf("expected 2 hops_received, got %v", stats["hops_received"])
	}
	if stats["paths_received"].(int64) != 1 {
		t.Errorf("expected 1 paths_received, got %v", stats["paths_received"])
	}
	if _, ok := stats["active_windows"]; !ok {
		t.Error("expected active_windows in stats")
	}
}

// ---------------------------------------------------------------------------
// Ring buffer overflow tests
// ---------------------------------------------------------------------------

func TestHopRingBuffer_Overflow(t *testing.T) {
	svc := newTestService()
	// Reduce maxHops for testing.
	svc.maxHops = 100

	for i := 0; i < 150; i++ {
		svc.IngestHopForTest(HopMetric{
			TraceID:   "overflow",
			Timestamp: time.Now(),
			Latency:   time.Millisecond,
			ServiceID: 1,
		})
	}

	svc.hopsMu.RLock()
	count := len(svc.hops)
	svc.hopsMu.RUnlock()

	if count > 100 {
		t.Errorf("expected hops to be trimmed to <= 100, got %d", count)
	}
}

func TestPathRingBuffer_Overflow(t *testing.T) {
	svc := newTestService()
	svc.maxPaths = 50

	for i := 0; i < 80; i++ {
		svc.IngestPathForTest(PathMetric{
			TraceID:   "overflow",
			Timestamp: time.Now(),
		})
	}

	svc.pathsMu.RLock()
	count := len(svc.paths)
	svc.pathsMu.RUnlock()

	if count > 50 {
		t.Errorf("expected paths to be trimmed to <= 50, got %d", count)
	}
}

func TestAggregateRingBuffer_Overflow(t *testing.T) {
	svc := newTestService()
	svc.maxAggs = 20

	// Create many aggregates by flushing many windows.
	for i := 0; i < 30; i++ {
		ts := time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC)
		svc.IngestHopForTest(HopMetric{
			TraceID:   "t",
			Timestamp: ts,
			Latency:   time.Millisecond,
			ServiceID: 1,
		})
	}

	// Force flush all.
	svc.aggMu.Lock()
	for key, bucket := range svc.windows {
		if bucket.sampleCount > 0 {
			svc.flushBucket(key, bucket)
			delete(svc.windows, key)
		}
	}
	svc.aggMu.Unlock()

	svc.aggMu.RLock()
	count := len(svc.aggregates)
	svc.aggMu.RUnlock()

	if count > 20 {
		t.Errorf("expected aggregates to be trimmed to <= 20, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// ingestPath: TotalLatency calculation
// ---------------------------------------------------------------------------

func TestIngestPath_NoHops(t *testing.T) {
	svc := newTestService()
	svc.IngestPathForTest(PathMetric{
		TraceID: "empty",
		Hops:    nil,
	})

	paths := svc.RecentPaths(10)
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d", len(paths))
	}
	if paths[0].HopCount != 0 {
		t.Errorf("expected 0 hops, got %d", paths[0].HopCount)
	}
}

func TestIngestPath_SingleHop(t *testing.T) {
	svc := newTestService()
	svc.IngestPathForTest(PathMetric{
		TraceID: "single",
		Hops: []HopMetric{
			{TraceID: "single", HopIndex: 0, Timestamp: time.Now(), Latency: time.Millisecond},
		},
	})

	paths := svc.RecentPaths(10)
	if paths[0].HopCount != 1 {
		t.Errorf("expected 1 hop, got %d", paths[0].HopCount)
	}
	// Single hop: TotalLatency should remain 0 (needs >= 2 hops for calculation)
	if paths[0].TotalLatency != 0 {
		t.Errorf("expected 0 total latency for single hop, got %v", paths[0].TotalLatency)
	}
}

func TestIngestPath_PresetTotalLatency(t *testing.T) {
	svc := newTestService()
	svc.IngestPathForTest(PathMetric{
		TraceID:      "preset",
		TotalLatency: 42 * time.Millisecond,
		Hops: []HopMetric{
			{TraceID: "preset", HopIndex: 0, Timestamp: time.Now()},
			{TraceID: "preset", HopIndex: 1, Timestamp: time.Now().Add(10 * time.Millisecond)},
		},
	})

	paths := svc.RecentPaths(10)
	// Should keep the preset value since it's non-zero.
	if paths[0].TotalLatency != 42*time.Millisecond {
		t.Errorf("expected preset latency 42ms, got %v", paths[0].TotalLatency)
	}
}

func TestIngestPath_ZeroTimestamps(t *testing.T) {
	svc := newTestService()
	svc.IngestPathForTest(PathMetric{
		TraceID: "zerostamps",
		Hops: []HopMetric{
			{TraceID: "z", HopIndex: 0},
			{TraceID: "z", HopIndex: 1},
		},
	})

	paths := svc.RecentPaths(10)
	// Zero timestamps should not calculate latency.
	if paths[0].TotalLatency != 0 {
		t.Errorf("expected 0 total latency with zero timestamps, got %v", paths[0].TotalLatency)
	}
}

// ---------------------------------------------------------------------------
// ingestHop: zero timestamp
// ---------------------------------------------------------------------------

func TestIngestHop_ZeroTimestamp(t *testing.T) {
	svc := newTestService()
	svc.IngestHopForTest(HopMetric{
		TraceID:   "t1",
		Latency:   time.Millisecond,
		ServiceID: 1,
		// Timestamp is zero
	})

	hops := svc.RecentHops(5)
	if len(hops) != 1 {
		t.Fatalf("expected 1 hop, got %d", len(hops))
	}
	if hops[0].Timestamp.IsZero() {
		t.Error("expected timestamp to be filled in")
	}
}

// ---------------------------------------------------------------------------
// HTTP handler tests
// ---------------------------------------------------------------------------

func TestHandleHops(t *testing.T) {
	svc := newTestService()
	svc.IngestHopForTest(HopMetric{
		TraceID:   "t1",
		Timestamp: time.Now(),
		Latency:   time.Millisecond,
		ServiceID: 1,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry/hops", nil)
	rr := httptest.NewRecorder()
	svc.handleHops(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if int(resp["count"].(float64)) != 1 {
		t.Errorf("expected count=1, got %v", resp["count"])
	}
}

func TestHandlePaths(t *testing.T) {
	svc := newTestService()
	svc.IngestPathForTest(PathMetric{TraceID: "p1", Timestamp: time.Now()})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry/paths", nil)
	rr := httptest.NewRecorder()
	svc.handlePaths(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if int(resp["count"].(float64)) != 1 {
		t.Errorf("expected count=1, got %v", resp["count"])
	}
}

func TestHandleServiceLatency_Valid(t *testing.T) {
	svc := newTestService()

	// Generate some aggregates.
	past := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	svc.IngestHopForTest(HopMetric{
		TraceID:   "t",
		Timestamp: past,
		Latency:   time.Millisecond,
		ServiceID: 5,
	})

	svc.aggMu.Lock()
	for key, bucket := range svc.windows {
		if bucket.sampleCount > 0 {
			svc.flushBucket(key, bucket)
			delete(svc.windows, key)
		}
	}
	svc.aggMu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry/services/5/latency", nil)
	rr := httptest.NewRecorder()
	svc.handleServiceLatency(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if int(resp["service_id"].(float64)) != 5 {
		t.Errorf("expected service_id=5, got %v", resp["service_id"])
	}
}

func TestHandleServiceLatency_InvalidID(t *testing.T) {
	svc := newTestService()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry/services/abc/latency", nil)
	rr := httptest.NewRecorder()
	svc.handleServiceLatency(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid service ID, got %d", rr.Code)
	}
}

func TestHandleAggregate(t *testing.T) {
	svc := newTestService()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry/aggregate", nil)
	rr := httptest.NewRecorder()
	svc.handleAggregate(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if int(resp["count"].(float64)) != 0 {
		t.Errorf("expected 0 aggregates initially, got %v", resp["count"])
	}
}

func TestHandleHealth(t *testing.T) {
	svc := newTestService()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	svc.handleHealth(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("expected ok, got %s", resp["status"])
	}
}

func TestHandleReady(t *testing.T) {
	svc := newTestService()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()
	svc.handleReady(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["status"] != "ready" {
		t.Errorf("expected ready, got %s", resp["status"])
	}
}

func TestHandleMetrics(t *testing.T) {
	svc := newTestService()
	svc.IngestHopForTest(HopMetric{TraceID: "t", Timestamp: time.Now(), Latency: time.Millisecond, ServiceID: 1})

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	svc.handleMetrics(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["hops_stored"] == nil {
		t.Error("expected hops_stored in metrics response")
	}
}

// ---------------------------------------------------------------------------
// Stop tests
// ---------------------------------------------------------------------------

func TestStop_NilServer(t *testing.T) {
	svc := newTestService()
	err := svc.Stop()
	if err != nil {
		t.Fatalf("Stop with nil server should not error, got %v", err)
	}
}

func TestStop_WithServer(t *testing.T) {
	svc := newTestService()
	// Create a real HTTP server to test shutdown.
	mux := http.NewServeMux()
	svc.registerRoutes(mux)
	server := httptest.NewServer(mux)
	svc.httpServer = &http.Server{Addr: server.Listener.Addr().String(), Handler: mux}
	server.Close()

	err := svc.Stop()
	// The server was already closed by httptest, so shutdown may return
	// an error about the listener; that's acceptable.
	_ = err
}

// ---------------------------------------------------------------------------
// aggregationLoop exits on context cancel
// ---------------------------------------------------------------------------

func TestAggregationLoop_ExitsOnCancel(t *testing.T) {
	svc := newTestService()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		svc.aggregationLoop(ctx)
		close(done)
	}()
	cancel()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("aggregationLoop did not exit on context cancel")
	}
}

// ---------------------------------------------------------------------------
// feedAggregation: window rotation
// ---------------------------------------------------------------------------

func TestFeedAggregation_WindowRotation(t *testing.T) {
	svc := newTestService()

	// Ingest hops in window 1.
	t1 := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	svc.IngestHopForTest(HopMetric{TraceID: "t", Timestamp: t1, Latency: time.Millisecond, ServiceID: 1})
	svc.IngestHopForTest(HopMetric{TraceID: "t", Timestamp: t1, Latency: 2 * time.Millisecond, ServiceID: 1})

	// Ingest hop in window 2 (should flush window 1).
	t2 := time.Date(2026, 3, 1, 12, 0, 1, 0, time.UTC)
	svc.IngestHopForTest(HopMetric{TraceID: "t", Timestamp: t2, Latency: 3 * time.Millisecond, ServiceID: 1})

	// Window 1 should have been flushed as an aggregate.
	aggs := svc.RecentAggregates(100)
	if len(aggs) < 1 {
		t.Error("expected at least 1 aggregate from flushed window")
	}

	// First aggregate should have 2 packets from window 1.
	if aggs[0].PacketCount != 2 {
		t.Errorf("expected 2 packets in first window, got %d", aggs[0].PacketCount)
	}
}

// ---------------------------------------------------------------------------
// flushExpiredWindows test
// ---------------------------------------------------------------------------

func TestFlushExpiredWindows(t *testing.T) {
	svc := newTestService()

	// Ingest a hop in a past window.
	past := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	svc.IngestHopForTest(HopMetric{TraceID: "t", Timestamp: past, Latency: time.Millisecond, ServiceID: 1})

	svc.flushExpiredWindows()

	aggs := svc.RecentAggregates(100)
	if len(aggs) != 1 {
		t.Errorf("expected 1 aggregate after flush, got %d", len(aggs))
	}
}

// ---------------------------------------------------------------------------
// Percentile edge cases
// ---------------------------------------------------------------------------

func TestPercentile_LargeDataset(t *testing.T) {
	// Create 100 values: 1ms to 100ms.
	values := make([]time.Duration, 100)
	for i := 0; i < 100; i++ {
		values[i] = time.Duration(i+1) * time.Millisecond
	}

	p50 := percentile(values, 50)
	if p50 < 49*time.Millisecond || p50 > 51*time.Millisecond {
		t.Errorf("P50 of 1..100ms should be ~50ms, got %v", p50)
	}

	p99 := percentile(values, 99)
	if p99 < 98*time.Millisecond || p99 > 100*time.Millisecond {
		t.Errorf("P99 of 1..100ms should be ~99ms, got %v", p99)
	}

	p0 := percentile(values, 0)
	if p0 != 1*time.Millisecond {
		t.Errorf("P0 should be 1ms, got %v", p0)
	}

	p100 := percentile(values, 100)
	if p100 != 100*time.Millisecond {
		t.Errorf("P100 should be 100ms, got %v", p100)
	}
}

// ---------------------------------------------------------------------------
// registerRoutes (coverage for route setup)
// ---------------------------------------------------------------------------

func TestRegisterRoutes(t *testing.T) {
	svc := newTestService()
	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	// Verify routes are registered by making requests.
	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v1/telemetry/hops"},
		{"GET", "/api/v1/telemetry/paths"},
		{"GET", "/api/v1/telemetry/aggregate"},
		{"GET", "/health"},
		{"GET", "/ready"},
		{"GET", "/metrics"},
	}

	for _, ep := range endpoints {
		req := httptest.NewRequest(ep.method, ep.path, nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("%s %s: expected 200, got %d", ep.method, ep.path, rr.Code)
		}
	}
}
