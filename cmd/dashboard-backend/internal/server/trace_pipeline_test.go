// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package server provides E2E tests for the trace observability pipeline.
//
// This test verifies the full pipeline:
//
//	eBPF (mock) -> trace-collector -> Wotan (mock) -> dashboard-backend -> browser (WebSocket)
//
// It uses the mock Wotan client to simulate Wotan message delivery and
// verifies that trace events flow from ingestion through to WebSocket
// broadcast and the REST API.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"unheaded/cmd/dashboard-backend/internal/events"
	"unheaded/pkg/logger"
)

// testLogger creates a logger that discards output for testing.
func testPipelineLogger() *logger.Logger {
	return logger.New(io.Discard)
}

// TestTracePipelineE2E verifies the full trace pipeline:
// 1. Start dashboard-backend with mock Wotan
// 2. Connect a WebSocket client
// 3. Inject a trace event via the server's IngestTraceEvent
// 4. Verify WebSocket client receives the trace event
// 5. Verify /api/v1/traces returns the event
func TestTracePipelineE2E(t *testing.T) {
	log := testPipelineLogger()

	// Create server config
	config := DefaultConfig()
	config.WotanAddr = "localhost:19999" // No real Wotan needed

	srv, err := NewServer(config, log)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		srv.Shutdown(shutdownCtx)
	}()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Create an HTTP test server that serves the dashboard-backend's WebSocket handler
	wsServer := srv.GetWebSocketServer()
	httpServer := httptest.NewServer(http.HandlerFunc(wsServer.HandleWebSocket))
	defer httpServer.Close()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")

	// Connect a WebSocket client
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WebSocket dial failed: %v", err)
	}
	defer conn.Close()

	// Wait for the client to be registered
	deadline := time.Now().Add(2 * time.Second)
	for wsServer.ConnectionCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if wsServer.ConnectionCount() == 0 {
		t.Fatal("WebSocket client not registered")
	}

	// Inject a trace event into the pipeline
	traceEvt := TraceEvent{
		TraceID:   "01945a3b-dead-beef-0000-000000000001",
		Timestamp: time.Now().UnixMilli(),
		Src:       "10.10.10.20:8000",
		Dst:       "10.10.10.10:9090",
		Protocol:  "TCP",
		RTTMs:     0.45,
		FlowState: "ESTABLISHED",
		HopCount:  1,
		PacketLen: 128,
		EventType: "packet",
	}

	srv.IngestTraceEvent(traceEvt)

	// Read from WebSocket and verify we got a trace message
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, rawMsg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("WebSocket read failed: %v", err)
	}

	var wsMsg map[string]interface{}
	if err := json.Unmarshal(rawMsg, &wsMsg); err != nil {
		t.Fatalf("Unmarshal WebSocket message failed: %v", err)
	}

	// Verify message type
	if wsMsg["type"] != "trace" {
		t.Errorf("expected type 'trace', got %v", wsMsg["type"])
	}

	// Verify data fields
	data, ok := wsMsg["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be a map, got %T", wsMsg["data"])
	}

	if data["trace_id"] != traceEvt.TraceID {
		t.Errorf("trace_id mismatch: got %v, want %s", data["trace_id"], traceEvt.TraceID)
	}
	if data["src"] != traceEvt.Src {
		t.Errorf("src mismatch: got %v, want %s", data["src"], traceEvt.Src)
	}
	if data["dst"] != traceEvt.Dst {
		t.Errorf("dst mismatch: got %v, want %s", data["dst"], traceEvt.Dst)
	}
	if data["protocol"] != traceEvt.Protocol {
		t.Errorf("protocol mismatch: got %v, want %s", data["protocol"], traceEvt.Protocol)
	}
	if data["event_type"] != "packet" {
		t.Errorf("event_type mismatch: got %v, want 'packet'", data["event_type"])
	}

	// Verify trace buffer has the event
	buf := srv.GetTraceBuffer()
	if buf.Count() != 1 {
		t.Errorf("expected 1 trace in buffer, got %d", buf.Count())
	}

	recent := buf.Recent(10)
	if len(recent) != 1 {
		t.Fatalf("expected 1 recent trace, got %d", len(recent))
	}
	if recent[0].TraceID != traceEvt.TraceID {
		t.Errorf("buffer trace_id mismatch: got %s, want %s", recent[0].TraceID, traceEvt.TraceID)
	}
}

// TestTracePipelineRESTAPI verifies that /api/v1/traces returns buffered trace events.
func TestTracePipelineRESTAPI(t *testing.T) {
	log := testPipelineLogger()

	config := DefaultConfig()
	config.WotanAddr = "localhost:19999"

	srv, err := NewServer(config, log)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		srv.Shutdown(shutdownCtx)
	}()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Inject several trace events
	for i := 0; i < 5; i++ {
		srv.IngestTraceEvent(TraceEvent{
			TraceID:   fmt.Sprintf("trace-%04d", i),
			Timestamp: time.Now().Add(time.Duration(i) * time.Millisecond).UnixMilli(),
			Src:       fmt.Sprintf("10.10.10.%d:8000", 20+i),
			Dst:       "10.10.10.10:9090",
			Protocol:  "TCP",
			RTTMs:     float64(i) * 0.1,
			FlowState: "ESTABLISHED",
			HopCount:  1,
			PacketLen: 64 + i*10,
			EventType: "packet",
		})
	}

	// Query the /api/v1/traces endpoint via the HTTP test server
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		traces := srv.GetTraceBuffer().Recent(100)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"traces":   traces,
			"total":    srv.GetTraceBuffer().Count(),
			"has_more": false,
			"source":   "pipeline_buffer",
		})
	}))
	defer httpServer.Close()

	resp, err := http.Get(httpServer.URL)
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var result struct {
		Traces  []TraceEvent `json:"traces"`
		Total   int          `json:"total"`
		HasMore bool         `json:"has_more"`
		Source  string       `json:"source"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Decode response failed: %v", err)
	}

	if result.Total != 5 {
		t.Errorf("expected total=5, got %d", result.Total)
	}
	if len(result.Traces) != 5 {
		t.Errorf("expected 5 traces, got %d", len(result.Traces))
	}
	if result.Source != "pipeline_buffer" {
		t.Errorf("expected source=pipeline_buffer, got %s", result.Source)
	}

	// Verify newest first
	if len(result.Traces) >= 2 {
		if result.Traces[0].Timestamp < result.Traces[1].Timestamp {
			t.Error("expected traces ordered newest first")
		}
	}
}

// TestTracePipelineMultipleWebSocketClients verifies that trace events are
// broadcast to all connected WebSocket clients simultaneously.
func TestTracePipelineMultipleWebSocketClients(t *testing.T) {
	log := testPipelineLogger()

	config := DefaultConfig()
	config.WotanAddr = "localhost:19999"

	srv, err := NewServer(config, log)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		srv.Shutdown(shutdownCtx)
	}()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	wsServer := srv.GetWebSocketServer()
	httpServer := httptest.NewServer(http.HandlerFunc(wsServer.HandleWebSocket))
	defer httpServer.Close()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")

	// Connect 3 clients
	const numClients = 3
	clients := make([]*websocket.Conn, numClients)
	for i := 0; i < numClients; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("client %d: Dial failed: %v", i, err)
		}
		defer conn.Close()
		clients[i] = conn
	}

	// Wait for all clients to register
	deadline := time.Now().Add(2 * time.Second)
	for wsServer.ConnectionCount() < numClients && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if wsServer.ConnectionCount() < numClients {
		t.Fatalf("expected %d connections, got %d", numClients, wsServer.ConnectionCount())
	}

	// Inject a trace event
	srv.IngestTraceEvent(TraceEvent{
		TraceID:   "broadcast-test-trace",
		Timestamp: time.Now().UnixMilli(),
		Src:       "10.10.10.20:8000",
		Dst:       "10.10.10.10:9090",
		Protocol:  "TCP",
		EventType: "correlated",
	})

	// All clients should receive the trace event
	var wg sync.WaitGroup
	errors := make([]error, numClients)

	for i, client := range clients {
		wg.Add(1)
		go func(idx int, conn *websocket.Conn) {
			defer wg.Done()
			conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			_, msg, err := conn.ReadMessage()
			if err != nil {
				errors[idx] = fmt.Errorf("client %d read error: %w", idx, err)
				return
			}

			var wsMsg map[string]interface{}
			if err := json.Unmarshal(msg, &wsMsg); err != nil {
				errors[idx] = fmt.Errorf("client %d unmarshal error: %w", idx, err)
				return
			}
			if wsMsg["type"] != "trace" {
				errors[idx] = fmt.Errorf("client %d: expected type 'trace', got %v", idx, wsMsg["type"])
			}
		}(i, client)
	}

	wg.Wait()

	for _, err := range errors {
		if err != nil {
			t.Error(err)
		}
	}
}

// TestTraceBufferCapacity verifies the trace buffer correctly evicts old entries.
func TestTraceBufferCapacity(t *testing.T) {
	buf := NewTraceBuffer(5)

	// Add 10 events
	for i := 0; i < 10; i++ {
		buf.Add(TraceEvent{
			TraceID:   fmt.Sprintf("trace-%d", i),
			Timestamp: int64(i),
		})
	}

	// Should only have last 5
	if buf.Count() != 5 {
		t.Errorf("expected 5 events after overflow, got %d", buf.Count())
	}

	recent := buf.Recent(10)
	if len(recent) != 5 {
		t.Fatalf("expected 5 recent events, got %d", len(recent))
	}

	// Newest first
	if recent[0].TraceID != "trace-9" {
		t.Errorf("expected newest trace-9, got %s", recent[0].TraceID)
	}
	if recent[4].TraceID != "trace-5" {
		t.Errorf("expected oldest trace-5, got %s", recent[4].TraceID)
	}
}

// TestTracePipelineEventStreamerIntegration verifies that trace events published
// through the event streamer (simulating Wotan delivery) are captured by the
// dashboard-backend's trace pipeline.
func TestTracePipelineEventStreamerIntegration(t *testing.T) {
	log := testPipelineLogger()

	config := DefaultConfig()
	config.WotanAddr = "localhost:19999"

	srv, err := NewServer(config, log)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		srv.Shutdown(shutdownCtx)
	}()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Allow broadcast goroutines to register their listeners
	time.Sleep(50 * time.Millisecond)

	// Simulate a trace event coming through the event streamer (as if from Wotan).
	// The event streamer's PublishEvent will call processEvent, which notifies
	// all listeners including the broadcastTraceEvents listener.
	streamer := srv.GetEventStreamer()
	streamer.PublishEvent(events.Event{
		Type:  events.EventTypeSystem,
		Topic: "traces.packet",
		Title: "Packet trace from eBPF",
		Data: map[string]interface{}{
			"trace_id":   "wotan-trace-001",
			"src_ip":     "10.10.10.20",
			"dst_ip":     "10.10.10.10",
			"protocol":   "TCP",
			"packet_len": float64(256),
			"hop_count":  float64(2),
		},
		Timestamp: time.Now(),
		Source:    "trace-collector",
	})

	// Give the async listener time to process
	time.Sleep(200 * time.Millisecond)

	// Verify the trace buffer captured the event
	buf := srv.GetTraceBuffer()
	if buf.Count() < 1 {
		t.Fatalf("expected at least 1 trace in buffer, got %d", buf.Count())
	}

	recent := buf.Recent(1)
	if len(recent) == 0 {
		t.Fatal("no recent traces")
	}

	te := recent[0]
	if te.TraceID != "wotan-trace-001" {
		t.Errorf("trace_id mismatch: got %s, want wotan-trace-001", te.TraceID)
	}
	if te.Src != "10.10.10.20" {
		t.Errorf("src mismatch: got %s, want 10.10.10.20", te.Src)
	}
	if te.Dst != "10.10.10.10" {
		t.Errorf("dst mismatch: got %s, want 10.10.10.10", te.Dst)
	}
	if te.Protocol != "TCP" {
		t.Errorf("protocol mismatch: got %s, want TCP", te.Protocol)
	}
	if te.PacketLen != 256 {
		t.Errorf("packet_len mismatch: got %d, want 256", te.PacketLen)
	}
	if te.HopCount != 2 {
		t.Errorf("hop_count mismatch: got %d, want 2", te.HopCount)
	}
	if te.EventType != "packet" {
		t.Errorf("event_type mismatch: got %s, want packet", te.EventType)
	}
}

// TestTracePipelineCorrelatedFlow verifies that correlated flow events
// (with RTT measurements) are correctly parsed from event streamer events.
func TestTracePipelineCorrelatedFlow(t *testing.T) {
	log := testPipelineLogger()

	config := DefaultConfig()
	config.WotanAddr = "localhost:19999"

	srv, err := NewServer(config, log)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		srv.Shutdown(shutdownCtx)
	}()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Allow broadcast goroutines to register their listeners
	time.Sleep(50 * time.Millisecond)

	streamer := srv.GetEventStreamer()
	streamer.PublishEvent(events.Event{
		Type:  events.EventTypeSystem,
		Topic: "traces.correlated",
		Title: "Correlated flow",
		Data: map[string]interface{}{
			"trace_id": "correlated-001",
			"rtt_ns":   float64(450000), // 0.45ms in ns
			"flow_key": map[string]interface{}{
				"src_addr": "10.10.10.20",
				"dst_addr": "10.10.10.10",
				"src_port": float64(8000),
				"dst_port": float64(9090),
				"protocol": float64(6), // TCP
			},
			"flow_state": "ESTABLISHED",
		},
		Timestamp: time.Now(),
		Source:    "trace-collector",
	})

	time.Sleep(200 * time.Millisecond)

	buf := srv.GetTraceBuffer()
	if buf.Count() < 1 {
		t.Fatalf("expected at least 1 trace, got %d", buf.Count())
	}

	te := buf.Recent(1)[0]
	if te.TraceID != "correlated-001" {
		t.Errorf("trace_id: got %s, want correlated-001", te.TraceID)
	}
	if te.EventType != "correlated" {
		t.Errorf("event_type: got %s, want correlated", te.EventType)
	}
	if te.Src != "10.10.10.20:8000" {
		t.Errorf("src: got %s, want 10.10.10.20:8000", te.Src)
	}
	if te.Dst != "10.10.10.10:9090" {
		t.Errorf("dst: got %s, want 10.10.10.10:9090", te.Dst)
	}
	if te.Protocol != "TCP" {
		t.Errorf("protocol: got %s, want TCP", te.Protocol)
	}
	if te.RTTMs < 0.44 || te.RTTMs > 0.46 {
		t.Errorf("rtt_ms: got %f, want ~0.45", te.RTTMs)
	}
	if te.FlowState != "ESTABLISHED" {
		t.Errorf("flow_state: got %s, want ESTABLISHED", te.FlowState)
	}
}
