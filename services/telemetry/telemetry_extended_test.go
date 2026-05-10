// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"unheaded/pkg/logger"
	wotanClient "unheaded/pkg/wotan-client"
)

// ---------------------------------------------------------------------------
// fakeWotan: a minimal HTTP server mimicking the Wotan message bus API.
// It supports subscribe, publish, and message polling endpoints so that
// a real *wotanClient.Client can be created and used for integration-level
// tests of the telemetry service's Start, listener, and publish paths.
// ---------------------------------------------------------------------------

type fakeWotan struct {
	mu       sync.Mutex
	messages map[string][]*wotanClient.Message // topic -> messages
	seq      int64
	server   *httptest.Server
}

func newFakeWotan() *fakeWotan {
	fw := &fakeWotan{
		messages: make(map[string][]*wotanClient.Message),
	}

	mux := http.NewServeMux()

	// POST /api/v1/topics/{topic}/subscribe
	mux.HandleFunc("/api/v1/topics/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/subscribe") && r.Method == http.MethodPost:
			// Extract topic from path: /api/v1/topics/{topic}/subscribe
			parts := strings.Split(strings.TrimPrefix(path, "/api/v1/topics/"), "/")
			topic := parts[0]
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"subscriber": map[string]interface{}{
					"subscriber_id": "sub-" + topic,
					"topic":         topic,
					"display_name":  "telemetry-agg",
					"status":        "approved",
				},
			})

		case strings.HasSuffix(path, "/publish") && r.Method == http.MethodPost:
			// Extract topic from path: /api/v1/topics/{topic}/publish
			parts := strings.Split(strings.TrimPrefix(path, "/api/v1/topics/"), "/")
			topic := parts[0]

			var body struct {
				SubscriberID string `json:"subscriber_id"`
				Payload      string `json:"payload"`
			}
			json.NewDecoder(r.Body).Decode(&body)

			fw.mu.Lock()
			seq := atomic.AddInt64(&fw.seq, 1)
			fw.messages[topic] = append(fw.messages[topic], &wotanClient.Message{
				MessageID: fmt.Sprintf("msg-%d", seq),
				Topic:     topic,
				SenderID:  body.SubscriberID,
				Payload:   body.Payload,
				Seq:       seq,
				CreatedAt: time.Now(),
			})
			fw.mu.Unlock()

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

		case strings.Contains(path, "/messages"):
			// GET /api/v1/topics/{topic}/messages
			parts := strings.Split(strings.TrimPrefix(path, "/api/v1/topics/"), "/")
			topic := parts[0]

			fw.mu.Lock()
			msgs := fw.messages[topic]
			// Clear after retrieval to prevent infinite polling loops.
			fw.messages[topic] = nil
			fw.mu.Unlock()

			if msgs == nil {
				msgs = []*wotanClient.Message{}
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"messages": msgs,
			})

		default:
			http.NotFound(w, r)
		}
	})

	fw.server = httptest.NewServer(mux)
	return fw
}

func (fw *fakeWotan) close() {
	fw.server.Close()
}

// addr returns host:port for wotanClient.NewClient.
func (fw *fakeWotan) addr() string {
	return strings.TrimPrefix(fw.server.URL, "http://")
}

// ---------------------------------------------------------------------------
// Tests for Start (covers the nil-wotan and with-wotan branches).
// ---------------------------------------------------------------------------

func TestStart_NilWotan(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start with nil wotan should succeed, got %v", err)
	}

	// Verify HTTP server is running by calling Stop.
	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestStart_WithWotan(t *testing.T) {
	fw := newFakeWotan()
	defer fw.close()

	client, err := wotanClient.NewClient(fw.addr())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	svc := NewService(logger.New(io.Discard), client)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Give listeners time to start.
	time.Sleep(100 * time.Millisecond)

	// Verify service is running then stop it.
	cancel()
	time.Sleep(100 * time.Millisecond)

	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestStart_WotanSubscribeFailure(t *testing.T) {
	// Create a server that always returns 500 for subscribe.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "internal"})
	}))
	defer server.Close()

	client, err := wotanClient.NewClient(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	svc := NewService(logger.New(io.Discard), client)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start should succeed even if subscriptions fail (best-effort).
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start should succeed despite subscribe failures: %v", err)
	}

	cancel()
	svc.Stop()
}

// ---------------------------------------------------------------------------
// Tests for publishAggregate (covers the Wotan publish path).
// ---------------------------------------------------------------------------

func TestPublishAggregate(t *testing.T) {
	fw := newFakeWotan()
	defer fw.close()

	client, err := wotanClient.NewClient(fw.addr())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	// Subscribe to the aggregate topic so Publish works.
	ctx := context.Background()
	_, err = client.Subscribe(ctx, "telemetry.aggregate", "telemetry-agg")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	svc := NewService(logger.New(io.Discard), client)

	agg := ServicePairAggregate{
		SrcSvcID:      1,
		DstSvcID:      2,
		Window:        time.Now().Truncate(time.Second),
		LatencyP50:    5 * time.Millisecond,
		LatencyP95:    9 * time.Millisecond,
		LatencyP99:    10 * time.Millisecond,
		QueueDepthAvg: 3.5,
		PacketCount:   100,
	}

	// Call publishAggregate directly.
	svc.publishAggregate(agg)

	// Verify the message was published to the fake server.
	fw.mu.Lock()
	msgs := fw.messages["telemetry.aggregate"]
	fw.mu.Unlock()

	if len(msgs) == 0 {
		t.Fatal("expected at least 1 published message")
	}

	// Verify payload structure.
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(msgs[0].Payload), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if payload["event_type"] != "telemetry.aggregate" {
		t.Errorf("expected event_type=telemetry.aggregate, got %v", payload["event_type"])
	}
	if int(payload["src_svc_id"].(float64)) != 1 {
		t.Errorf("expected src_svc_id=1, got %v", payload["src_svc_id"])
	}
	if int(payload["dst_svc_id"].(float64)) != 2 {
		t.Errorf("expected dst_svc_id=2, got %v", payload["dst_svc_id"])
	}
	if int64(payload["packet_count"].(float64)) != 100 {
		t.Errorf("expected packet_count=100, got %v", payload["packet_count"])
	}
}

func TestPublishAggregate_NilWotan(t *testing.T) {
	// With nil wotan, flushBucket should not attempt to publish.
	svc := NewService(logger.New(io.Discard), nil)

	past := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	svc.IngestHopForTest(HopMetric{
		TraceID:   "t",
		Timestamp: past,
		Latency:   time.Millisecond,
		ServiceID: 1,
	})

	// Force flush — should not panic even though wotan is nil.
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
		t.Error("expected at least 1 aggregate")
	}
}

// ---------------------------------------------------------------------------
// Tests for flushBucket with Wotan publish (integration).
// ---------------------------------------------------------------------------

func TestFlushBucket_WithWotanPublish(t *testing.T) {
	fw := newFakeWotan()
	defer fw.close()

	client, err := wotanClient.NewClient(fw.addr())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	_, err = client.Subscribe(ctx, "telemetry.aggregate", "telemetry-agg")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	svc := NewService(logger.New(io.Discard), client)

	// Ingest multiple hops into a past window.
	past := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		svc.IngestHopForTest(HopMetric{
			TraceID:    "flush-test",
			Timestamp:  past,
			Latency:    time.Duration(i+1) * time.Millisecond,
			QueueDepth: uint32(i * 2),
			ServiceID:  3,
		})
	}

	// Force flush to trigger publish.
	svc.aggMu.Lock()
	for key, bucket := range svc.windows {
		if bucket.sampleCount > 0 {
			svc.flushBucket(key, bucket)
			delete(svc.windows, key)
		}
	}
	svc.aggMu.Unlock()

	// Wait for async publish goroutine.
	time.Sleep(200 * time.Millisecond)

	fw.mu.Lock()
	msgs := fw.messages["telemetry.aggregate"]
	fw.mu.Unlock()

	if len(msgs) == 0 {
		t.Fatal("expected published aggregate message")
	}

	// Verify aggs_published counter was incremented.
	stats := svc.Stats()
	if stats["aggs_published"].(int64) < 1 {
		t.Errorf("expected aggs_published >= 1, got %v", stats["aggs_published"])
	}
}

// ---------------------------------------------------------------------------
// Tests for listener goroutines (listenHopMetrics, listenPathMetrics, listenAlerts).
// ---------------------------------------------------------------------------

func TestListenHopMetrics_StreamError(t *testing.T) {
	// Test that listenHopMetrics handles StreamMessages error gracefully.
	// Create a client that's not subscribed to the topic, so StreamMessages fails.
	fw := newFakeWotan()
	defer fw.close()

	client, err := wotanClient.NewClient(fw.addr())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	svc := NewService(logger.New(io.Discard), client)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Should return without panicking.
	done := make(chan struct{})
	go func() {
		svc.listenHopMetrics(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Success: returned gracefully on error.
	case <-time.After(2 * time.Second):
		t.Fatal("listenHopMetrics did not return on StreamMessages error")
	}
}

func TestListenPathMetrics_StreamError(t *testing.T) {
	fw := newFakeWotan()
	defer fw.close()

	client, err := wotanClient.NewClient(fw.addr())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	svc := NewService(logger.New(io.Discard), client)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		svc.listenPathMetrics(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Success.
	case <-time.After(2 * time.Second):
		t.Fatal("listenPathMetrics did not return on StreamMessages error")
	}
}

func TestListenAlerts_StreamError(t *testing.T) {
	fw := newFakeWotan()
	defer fw.close()

	client, err := wotanClient.NewClient(fw.addr())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	svc := NewService(logger.New(io.Discard), client)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		svc.listenAlerts(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Success.
	case <-time.After(2 * time.Second):
		t.Fatal("listenAlerts did not return on StreamMessages error")
	}
}

func TestListenHopMetrics_ContextCancel(t *testing.T) {
	fw := newFakeWotan()
	defer fw.close()

	client, err := wotanClient.NewClient(fw.addr())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	// Subscribe so StreamMessages succeeds.
	ctx, cancel := context.WithCancel(context.Background())
	_, err = client.Subscribe(ctx, "telemetry.hop", "telemetry-agg")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	svc := NewService(logger.New(io.Discard), client)

	done := make(chan struct{})
	go func() {
		svc.listenHopMetrics(ctx)
		close(done)
	}()

	// Cancel context to trigger exit.
	cancel()

	select {
	case <-done:
		// Success.
	case <-time.After(3 * time.Second):
		t.Fatal("listenHopMetrics did not exit on context cancel")
	}
}

func TestListenPathMetrics_ContextCancel(t *testing.T) {
	fw := newFakeWotan()
	defer fw.close()

	client, err := wotanClient.NewClient(fw.addr())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	_, err = client.Subscribe(ctx, "telemetry.path", "telemetry-agg")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	svc := NewService(logger.New(io.Discard), client)

	done := make(chan struct{})
	go func() {
		svc.listenPathMetrics(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// Success.
	case <-time.After(3 * time.Second):
		t.Fatal("listenPathMetrics did not exit on context cancel")
	}
}

func TestListenAlerts_ContextCancel(t *testing.T) {
	fw := newFakeWotan()
	defer fw.close()

	client, err := wotanClient.NewClient(fw.addr())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	_, err = client.Subscribe(ctx, "alerts.critical", "telemetry-agg")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	svc := NewService(logger.New(io.Discard), client)

	done := make(chan struct{})
	go func() {
		svc.listenAlerts(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// Success.
	case <-time.After(3 * time.Second):
		t.Fatal("listenAlerts did not exit on context cancel")
	}
}

// ---------------------------------------------------------------------------
// Test Stop with a running HTTP server (full lifecycle).
// ---------------------------------------------------------------------------

func TestStartStop_FullLifecycle(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil)

	ctx, cancel := context.WithCancel(context.Background())

	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Give HTTP server time to start.
	time.Sleep(100 * time.Millisecond)

	// Cancel context to stop background goroutines.
	cancel()

	// Stop should shut down HTTP server cleanly.
	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Test handleServiceLatency with missing /latency suffix.
// ---------------------------------------------------------------------------

func TestHandleServiceLatency_MissingLatencyPath(t *testing.T) {
	svc := newTestService()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry/services/notanumber", nil)
	rr := httptest.NewRecorder()
	svc.handleServiceLatency(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for path without /latency, got %d", rr.Code)
	}
}

func TestHandleServiceLatency_EmptyPath(t *testing.T) {
	svc := newTestService()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry/services/", nil)
	rr := httptest.NewRecorder()
	svc.handleServiceLatency(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty service ID path, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Test ingestHop with zero timestamp sets current time.
// ---------------------------------------------------------------------------

func TestIngestHop_ZeroTimestamp_SetsNow(t *testing.T) {
	svc := newTestService()

	before := time.Now()
	svc.IngestHopForTest(HopMetric{
		TraceID:   "zero-ts",
		Latency:   time.Millisecond,
		ServiceID: 1,
	})
	after := time.Now()

	hops := svc.RecentHops(5)
	if len(hops) != 1 {
		t.Fatalf("expected 1 hop, got %d", len(hops))
	}

	ts := hops[0].Timestamp
	if ts.Before(before) || ts.After(after) {
		t.Errorf("hop timestamp %v not between %v and %v", ts, before, after)
	}
}

// ---------------------------------------------------------------------------
// Test ingestPath with zero timestamp sets current time.
// ---------------------------------------------------------------------------

func TestIngestPath_ZeroTimestamp_SetsNow(t *testing.T) {
	svc := newTestService()

	before := time.Now()
	svc.IngestPathForTest(PathMetric{
		TraceID: "zero-ts-path",
	})
	after := time.Now()

	paths := svc.RecentPaths(10)
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d", len(paths))
	}

	ts := paths[0].Timestamp
	if ts.Before(before) || ts.After(after) {
		t.Errorf("path timestamp %v not between %v and %v", ts, before, after)
	}
}

// ---------------------------------------------------------------------------
// Test the full route mux via httptest server (integration-style).
// ---------------------------------------------------------------------------

func TestRouteMux_Integration(t *testing.T) {
	svc := newTestService()

	// Ingest test data.
	now := time.Now()
	svc.IngestHopForTest(HopMetric{
		TraceID: "mux-t", Timestamp: now, Latency: time.Millisecond, ServiceID: 1,
	})
	svc.IngestPathForTest(PathMetric{
		TraceID: "mux-p", Timestamp: now,
		Hops: []HopMetric{
			{TraceID: "mux-p", HopIndex: 0, Timestamp: now},
			{TraceID: "mux-p", HopIndex: 1, Timestamp: now.Add(5 * time.Millisecond)},
		},
	})

	mux := http.NewServeMux()
	svc.registerRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	tests := []struct {
		path       string
		wantStatus int
		wantKey    string
	}{
		{"/api/v1/telemetry/hops", 200, "hops"},
		{"/api/v1/telemetry/paths", 200, "paths"},
		{"/api/v1/telemetry/aggregate", 200, "aggregates"},
		{"/health", 200, "status"},
		{"/ready", 200, "status"},
		{"/metrics", 200, "hops_stored"},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			resp, err := http.Get(server.URL + tc.path)
			if err != nil {
				t.Fatalf("GET %s: %v", tc.path, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.wantStatus {
				t.Errorf("GET %s: expected %d, got %d", tc.path, tc.wantStatus, resp.StatusCode)
			}

			var body map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&body)
			if _, ok := body[tc.wantKey]; !ok {
				t.Errorf("GET %s: missing key %q in response", tc.path, tc.wantKey)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Test JSON response content types.
// ---------------------------------------------------------------------------

func TestHandlers_ContentType(t *testing.T) {
	svc := newTestService()

	handlers := []struct {
		name    string
		handler http.HandlerFunc
		path    string
	}{
		{"hops", svc.handleHops, "/api/v1/telemetry/hops"},
		{"paths", svc.handlePaths, "/api/v1/telemetry/paths"},
		{"aggregate", svc.handleAggregate, "/api/v1/telemetry/aggregate"},
		{"health", svc.handleHealth, "/health"},
		{"ready", svc.handleReady, "/ready"},
		{"metrics", svc.handleMetrics, "/metrics"},
	}

	for _, h := range handlers {
		t.Run(h.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, h.path, nil)
			rr := httptest.NewRecorder()
			h.handler(rr, req)

			ct := rr.Header().Get("Content-Type")
			if ct != "application/json" {
				t.Errorf("expected Content-Type application/json, got %q", ct)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Test multiple service pairs in aggregation.
// ---------------------------------------------------------------------------

func TestFeedAggregation_MultipleServicePairs(t *testing.T) {
	svc := newTestService()

	past := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Ingest hops from two different service IDs.
	svc.IngestHopForTest(HopMetric{TraceID: "t1", Timestamp: past, Latency: time.Millisecond, ServiceID: 1})
	svc.IngestHopForTest(HopMetric{TraceID: "t2", Timestamp: past, Latency: 2 * time.Millisecond, ServiceID: 5})

	// Flush all.
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
		t.Fatalf("expected at least 2 aggregates (one per pair), got %d", len(aggs))
	}

	// Verify different service pairs.
	pairsSeen := make(map[pairKey]bool)
	for _, agg := range aggs {
		pairsSeen[pairKey{Src: agg.SrcSvcID, Dst: agg.DstSvcID}] = true
	}

	if !pairsSeen[pairKey{Src: 1, Dst: 2}] {
		t.Error("expected aggregate for pair (1, 2)")
	}
	if !pairsSeen[pairKey{Src: 5, Dst: 6}] {
		t.Error("expected aggregate for pair (5, 6)")
	}
}

// ---------------------------------------------------------------------------
// Test flushExpiredWindows with no data (empty state).
// ---------------------------------------------------------------------------

func TestFlushExpiredWindows_NoData(t *testing.T) {
	svc := newTestService()

	// Should not panic on empty state.
	svc.flushExpiredWindows()

	aggs := svc.RecentAggregates(100)
	if len(aggs) != 0 {
		t.Errorf("expected 0 aggregates, got %d", len(aggs))
	}
}

// ---------------------------------------------------------------------------
// Test RecentHops with negative minutes defaults to 5.
// ---------------------------------------------------------------------------

func TestRecentHops_NegativeMinutes(t *testing.T) {
	svc := newTestService()
	svc.IngestHopForTest(HopMetric{
		TraceID:   "neg",
		Timestamp: time.Now(),
		Latency:   time.Millisecond,
		ServiceID: 1,
	})

	hops := svc.RecentHops(-1)
	if len(hops) != 1 {
		t.Errorf("expected 1 hop with negative minutes, got %d", len(hops))
	}
}

// ---------------------------------------------------------------------------
// Test RecentPaths with negative limit defaults to 100.
// ---------------------------------------------------------------------------

func TestRecentPaths_NegativeLimit(t *testing.T) {
	svc := newTestService()
	svc.IngestPathForTest(PathMetric{TraceID: "neg", Timestamp: time.Now()})

	paths := svc.RecentPaths(-5)
	if len(paths) != 1 {
		t.Errorf("expected 1 path with negative limit, got %d", len(paths))
	}
}

// ---------------------------------------------------------------------------
// Test RecentAggregates with negative limit defaults to 100.
// ---------------------------------------------------------------------------

func TestRecentAggregates_NegativeLimit(t *testing.T) {
	svc := newTestService()
	aggs := svc.RecentAggregates(-1)
	if len(aggs) != 0 {
		t.Errorf("expected 0 aggregates initially, got %d", len(aggs))
	}
}

// ---------------------------------------------------------------------------
// Test Stats with aggregates and windows present.
// ---------------------------------------------------------------------------

func TestStats_WithAggregatesAndWindows(t *testing.T) {
	svc := newTestService()

	// Create current-window data (won't be flushed yet).
	svc.IngestHopForTest(HopMetric{
		TraceID:   "stats",
		Timestamp: time.Now(),
		Latency:   time.Millisecond,
		ServiceID: 1,
	})

	stats := svc.Stats()
	if stats["active_windows"].(int) < 1 {
		t.Errorf("expected at least 1 active window, got %v", stats["active_windows"])
	}

	// Also flush a past window so we have aggregates.
	past := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	svc.IngestHopForTest(HopMetric{
		TraceID:   "stats2",
		Timestamp: past,
		Latency:   time.Millisecond,
		ServiceID: 2,
	})

	svc.flushExpiredWindows()

	stats = svc.Stats()
	if stats["aggregates"].(int) < 1 {
		t.Errorf("expected at least 1 aggregate, got %v", stats["aggregates"])
	}
}
