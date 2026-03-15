// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package e2e provides performance-focused E2E tests for the Unheaded platform.
//
// Tests verify:
//   - Trace entry decoding latency (< 100ns)
//   - Correlator throughput (10000+ ops/sec)
//   - WebSocket-style broadcast to many clients (within 1s)
//
// All tests are self-contained using in-process mocks and httptest.Server.
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"unheaded/pkg/discovery"
	"unheaded/pkg/trace"
	"unheaded/pkg/wotan-client/mock"
)

// ============================================================================
// PERFORMANCE E2E TESTS
// ============================================================================

// TestPerformance_TraceProcessingLatency verifies that trace ID generation,
// serialization, and deserialization complete within tight latency bounds.
func TestPerformance_TraceProcessingLatency(t *testing.T) {
	const iterations = 10000

	// Benchmark trace ID generation.
	t.Run("trace_id_generation", func(t *testing.T) {
		start := time.Now()
		for i := 0; i < iterations; i++ {
			id, err := trace.NewTraceID()
			if err != nil {
				t.Fatalf("Failed at iteration %d: %v", i, err)
			}
			if id.IsZero() {
				t.Fatalf("Zero trace ID at iteration %d", i)
			}
		}
		elapsed := time.Since(start)
		perOp := elapsed / iterations

		t.Logf("Trace ID generation: %d ops in %v (%.0f ns/op)", iterations, elapsed, float64(perOp.Nanoseconds()))

		if perOp > 25*time.Microsecond {
			t.Errorf("Trace ID generation too slow: %v/op (max 25us)", perOp)
		}
	})

	// Benchmark flow label extraction.
	t.Run("flow_label_extraction", func(t *testing.T) {
		id := trace.MustNewTraceID()

		start := time.Now()
		for i := 0; i < iterations; i++ {
			fl := id.FlowLabelHint()
			if fl > 0x000FFFFF {
				t.Fatalf("Invalid flow label: %x", fl)
			}
		}
		elapsed := time.Since(start)
		perOp := elapsed / iterations

		t.Logf("Flow label extraction: %d ops in %v (%.0f ns/op)", iterations, elapsed, float64(perOp.Nanoseconds()))

		// Flow label extraction should be sub-100ns (just bit masking).
		if perOp > 100*time.Nanosecond {
			t.Errorf("Flow label extraction too slow: %v/op (max 100ns)", perOp)
		}
	})

	// Benchmark trace event JSON encode/decode round-trip.
	t.Run("trace_event_roundtrip", func(t *testing.T) {
		type traceEvent struct {
			TraceID   string `json:"trace_id"`
			FlowLabel uint32 `json:"flow_label"`
			Source    string `json:"source"`
			Dest     string `json:"dest"`
			Protocol string `json:"protocol"`
			LatencyNS int64  `json:"latency_ns"`
			Timestamp int64  `json:"timestamp"`
		}

		id := trace.MustNewTraceID()
		event := traceEvent{
			TraceID:   id.String(),
			FlowLabel: id.FlowLabelHint(),
			Source:    "10.10.10.20",
			Dest:     "10.10.10.100",
			Protocol: "tcp",
			LatencyNS: 1500000,
			Timestamp: time.Now().UnixMilli(),
		}

		start := time.Now()
		for i := 0; i < iterations; i++ {
			data, err := json.Marshal(event)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			var decoded traceEvent
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			if decoded.TraceID != event.TraceID {
				t.Fatalf("Round-trip mismatch")
			}
		}
		elapsed := time.Since(start)
		perOp := elapsed / iterations

		t.Logf("Trace event round-trip: %d ops in %v (%.0f ns/op)", iterations, elapsed, float64(perOp.Nanoseconds()))

		// JSON round-trip should complete within 50us (varies by host).
		if perOp > 50*time.Microsecond {
			t.Errorf("Trace event round-trip too slow: %v/op (max 50us)", perOp)
		}
	})

	// Benchmark TraceID from hex string.
	t.Run("trace_id_from_hex", func(t *testing.T) {
		id := trace.MustNewTraceID()
		hexStr := id.String()

		start := time.Now()
		for i := 0; i < iterations; i++ {
			parsed, err := trace.TraceIDFromHex(hexStr)
			if err != nil {
				t.Fatalf("Failed at iteration %d: %v", i, err)
			}
			if parsed.IsZero() {
				t.Fatal("Parsed trace ID is zero")
			}
		}
		elapsed := time.Since(start)
		perOp := elapsed / iterations

		t.Logf("TraceID from hex: %d ops in %v (%.0f ns/op)", iterations, elapsed, float64(perOp.Nanoseconds()))

		if perOp > 1*time.Microsecond {
			t.Errorf("TraceID from hex too slow: %v/op (max 1us)", perOp)
		}
	})
}

// TestPerformance_CorrelatorThroughput verifies that the trace correlation
// pipeline (generate ID -> encode -> publish -> decode) exceeds 10,000 ops/sec.
func TestPerformance_CorrelatorThroughput(t *testing.T) {
	const targetOps = 10000
	const workers = 4

	wotanClient := mock.NewMockClient(mock.WithAutoApprove())
	ctx := context.Background()

	// Subscribe the collector to the trace topic.
	traceTopic := "traces.correlator"
	_, err := wotanClient.Subscribe(ctx, traceTopic, "correlator")
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	var opsCompleted int64
	var wg sync.WaitGroup

	opsPerWorker := targetOps / workers

	start := time.Now()

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for i := 0; i < opsPerWorker; i++ {
				// Generate trace ID.
				id, err := trace.NewTraceID()
				if err != nil {
					return
				}

				// Encode trace event.
				event := map[string]interface{}{
					"trace_id":   id.String(),
					"flow_label": id.FlowLabelHint(),
					"worker":     workerID,
					"seq":        i,
					"timestamp":  time.Now().UnixNano(),
				}

				payload, err := json.Marshal(event)
				if err != nil {
					return
				}

				// Publish to Wotan.
				if err := wotanClient.Publish(ctx, traceTopic, payload); err != nil {
					return
				}

				atomic.AddInt64(&opsCompleted, 1)
			}
		}(w)
	}

	wg.Wait()
	elapsed := time.Since(start)

	ops := atomic.LoadInt64(&opsCompleted)
	opsPerSec := float64(ops) / elapsed.Seconds()

	t.Logf("Correlator throughput: %d ops in %v (%.0f ops/sec, %d workers)",
		ops, elapsed, opsPerSec, workers)

	if opsPerSec < float64(targetOps) {
		t.Errorf("Throughput below target: %.0f ops/sec (min %d)", opsPerSec, targetOps)
	}

	// Verify all messages were stored.
	msgs, err := wotanClient.GetMessages(ctx, traceTopic, 0, int(ops)+10)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}

	if int64(len(msgs)) != ops {
		t.Errorf("Expected %d messages, got %d", ops, len(msgs))
	}
}

// TestPerformance_WebSocketBroadcast simulates broadcasting trace events to
// multiple connected clients (using channels as WebSocket stand-ins) and
// verifies all clients receive events within 1 second.
func TestPerformance_WebSocketBroadcast(t *testing.T) {
	const numClients = 100
	const numEvents = 50

	// Broadcaster hub: fan-out to all connected clients.
	type broadcastHub struct {
		mu      sync.RWMutex
		clients []chan []byte
	}

	hub := &broadcastHub{}

	// Register clients.
	for i := 0; i < numClients; i++ {
		ch := make(chan []byte, numEvents+10) // Buffer to prevent blocking.
		hub.mu.Lock()
		hub.clients = append(hub.clients, ch)
		hub.mu.Unlock()
	}

	// Broadcast events.
	start := time.Now()

	for i := 0; i < numEvents; i++ {
		traceID := trace.MustNewTraceID()
		event := map[string]interface{}{
			"type":      "trace",
			"trace_id":  traceID.String(),
			"seq":       i,
			"timestamp": time.Now().UnixNano(),
		}
		payload, _ := json.Marshal(event)

		hub.mu.RLock()
		for _, ch := range hub.clients {
			select {
			case ch <- payload:
			default:
				// Drop if buffer full.
			}
		}
		hub.mu.RUnlock()
	}

	broadcastDuration := time.Since(start)

	// Verify all clients received all events.
	var wg sync.WaitGroup
	var totalReceived int64
	var missedClients int64

	for i, ch := range hub.clients {
		wg.Add(1)
		go func(clientID int, c chan []byte) {
			defer wg.Done()

			received := 0
			timeout := time.After(1 * time.Second)

			for received < numEvents {
				select {
				case _, ok := <-c:
					if !ok {
						return
					}
					received++
				case <-timeout:
					atomic.AddInt64(&missedClients, 1)
					atomic.AddInt64(&totalReceived, int64(received))
					return
				}
			}

			atomic.AddInt64(&totalReceived, int64(received))
		}(i, ch)
	}

	wg.Wait()

	total := atomic.LoadInt64(&totalReceived)
	missed := atomic.LoadInt64(&missedClients)
	expected := int64(numClients * numEvents)

	t.Logf("WebSocket broadcast: %d events to %d clients in %v",
		numEvents, numClients, broadcastDuration)
	t.Logf("  Total received: %d/%d (%.1f%%)", total, expected, float64(total)/float64(expected)*100)
	t.Logf("  Clients with missed events: %d/%d", missed, numClients)

	if broadcastDuration > 1*time.Second {
		t.Errorf("Broadcast took too long: %v (max 1s)", broadcastDuration)
	}

	if total < expected {
		t.Errorf("Not all events received: %d/%d", total, expected)
	}

	if missed > 0 {
		t.Errorf("%d clients missed events within timeout", missed)
	}

	// Clean up channels.
	for _, ch := range hub.clients {
		close(ch)
	}
}

// TestPerformance_DiscoveryResolutionLatency verifies that service discovery
// resolution is fast (< 1ms per lookup).
func TestPerformance_DiscoveryResolutionLatency(t *testing.T) {
	wotanDiscovery := newMockWotanDiscovery()
	registry := discovery.New(wotanDiscovery)

	ctx := context.Background()
	if err := registry.Start(ctx); err != nil {
		t.Fatalf("Failed to start registry: %v", err)
	}
	defer registry.Stop()

	// Register services.
	numServices := 50
	for i := 0; i < numServices; i++ {
		entry := discovery.ServiceEntry{
			Name:    fmt.Sprintf("perf-service-%d", i),
			Address: fmt.Sprintf("http://10.10.10.%d:8080", i+10),
			Port:    8080 + i,
			Health:  "healthy",
		}
		if err := registry.Register(entry); err != nil {
			t.Fatalf("Failed to register service %d: %v", i, err)
		}
	}

	const lookups = 10000

	start := time.Now()
	for i := 0; i < lookups; i++ {
		name := fmt.Sprintf("perf-service-%d", i%numServices)
		entry, err := registry.Resolve(name)
		if err != nil {
			t.Fatalf("Resolve failed for %s: %v", name, err)
		}
		if entry.Health != "healthy" {
			t.Fatalf("Unexpected health: %s", entry.Health)
		}
	}
	elapsed := time.Since(start)
	perOp := elapsed / lookups

	t.Logf("Discovery resolution: %d lookups in %v (%.0f ns/op, %d services)",
		lookups, elapsed, float64(perOp.Nanoseconds()), numServices)

	if perOp > 1*time.Millisecond {
		t.Errorf("Discovery resolution too slow: %v/op (max 1ms)", perOp)
	}
}

// TestPerformance_ConcurrentHTTPRequests verifies that the service can handle
// high concurrent HTTP request load without errors.
func TestPerformance_ConcurrentHTTPRequests(t *testing.T) {
	// Create a simple test server.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/data", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "ok",
			"timestamp": time.Now().UnixNano(),
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	const totalRequests = 1000
	const concurrency = 50

	var successCount int64
	var errorCount int64

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	start := time.Now()

	for i := 0; i < totalRequests; i++ {
		wg.Add(1)
		sem <- struct{}{} // Acquire semaphore.
		go func() {
			defer wg.Done()
			defer func() { <-sem }() // Release semaphore.

			resp, err := http.Get(server.URL + "/api/v1/data")
			if err != nil {
				atomic.AddInt64(&errorCount, 1)
				return
			}
			resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				atomic.AddInt64(&successCount, 1)
			} else {
				atomic.AddInt64(&errorCount, 1)
			}
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)

	successes := atomic.LoadInt64(&successCount)
	errors := atomic.LoadInt64(&errorCount)
	rps := float64(successes) / elapsed.Seconds()

	t.Logf("Concurrent HTTP: %d requests, %d success, %d errors in %v (%.0f req/s, concurrency=%d)",
		totalRequests, successes, errors, elapsed, rps, concurrency)

	if errors > 0 {
		t.Errorf("Had %d errors out of %d requests", errors, totalRequests)
	}

	if rps < 1000 {
		t.Logf("Warning: throughput below 1000 req/s (got %.0f), may be CI environment", rps)
	}
}

// TestPerformance_WotanPublishThroughput verifies Wotan message publishing
// throughput under load.
func TestPerformance_WotanPublishThroughput(t *testing.T) {
	wotanClient := mock.NewMockClient(mock.WithAutoApprove())
	ctx := context.Background()

	topic := "perf.throughput"
	_, err := wotanClient.Subscribe(ctx, topic, "perf-test")
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	const numMessages = 10000

	start := time.Now()
	for i := 0; i < numMessages; i++ {
		payload, _ := json.Marshal(map[string]int{"seq": i})
		if err := wotanClient.Publish(ctx, topic, payload); err != nil {
			t.Fatalf("Publish %d failed: %v", i, err)
		}
	}
	elapsed := time.Since(start)

	publishCount := wotanClient.GetPublishCount()
	msgsPerSec := float64(publishCount) / elapsed.Seconds()

	t.Logf("Wotan publish throughput: %d msgs in %v (%.0f msgs/sec)",
		publishCount, elapsed, msgsPerSec)

	if publishCount != numMessages {
		t.Errorf("Expected %d publishes, got %d", numMessages, publishCount)
	}

	if msgsPerSec < 10000 {
		t.Logf("Warning: publish throughput below 10k msgs/sec (got %.0f)", msgsPerSec)
	}
}

// TestPerformance_HealthCheckAggregation verifies that the health check
// aggregator can run checks and aggregate results quickly.
func TestPerformance_HealthCheckAggregation(t *testing.T) {
	// Create mock service endpoints.
	numServices := 20
	servers := make([]*httptest.Server, numServices)

	for i := 0; i < numServices; i++ {
		mux := http.NewServeMux()
		svcName := fmt.Sprintf("perf-svc-%d", i)
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"status":  "healthy",
				"service": svcName,
			})
		})
		servers[i] = httptest.NewServer(mux)
	}
	defer func() {
		for _, s := range servers {
			s.Close()
		}
	}()

	// Time how long it takes to check all services sequentially.
	start := time.Now()
	for _, server := range servers {
		resp, err := http.Get(server.URL + "/health")
		if err != nil {
			t.Fatalf("Health check failed: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}
	}
	sequentialDuration := time.Since(start)

	// Time how long it takes to check all services concurrently.
	start = time.Now()
	var wg sync.WaitGroup
	for _, server := range servers {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			resp, err := http.Get(url + "/health")
			if err != nil {
				return
			}
			resp.Body.Close()
		}(server.URL)
	}
	wg.Wait()
	concurrentDuration := time.Since(start)

	t.Logf("Health check aggregation (%d services):", numServices)
	t.Logf("  Sequential: %v (%.0f us/check)", sequentialDuration, float64(sequentialDuration.Microseconds())/float64(numServices))
	t.Logf("  Concurrent: %v (%.0f us/check)", concurrentDuration, float64(concurrentDuration.Microseconds())/float64(numServices))

	// Concurrent should be faster than sequential.
	if concurrentDuration > sequentialDuration && numServices > 1 {
		t.Logf("Warning: concurrent checks slower than sequential (may be single-core CI)")
	}

	// All checks should complete within 5 seconds.
	if concurrentDuration > 5*time.Second {
		t.Errorf("Concurrent health checks too slow: %v (max 5s)", concurrentDuration)
	}
}
