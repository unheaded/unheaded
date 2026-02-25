// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ── HealthHandler tests ───────────────────────────────────────────────────

func TestHealthHandler_AllLoaded(t *testing.T) {
	state := NewCollectorState()
	state.Programs = []ProgramStatus{
		{Name: "packet_marker", Enabled: true, Loaded: true},
		{Name: "flow_tracker", Enabled: true, Loaded: true},
		{Name: "latency_probe", Enabled: true, Loaded: true},
	}
	pubConfig := DefaultTracePublisherConfig()
	state.Publisher = NewTracePublisher(pubConfig)

	handler := &HealthHandler{State: state}
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if body["status"] != "healthy" {
		t.Errorf("status = %v, want \"healthy\"", body["status"])
	}
}

func TestHealthHandler_Unhealthy(t *testing.T) {
	state := NewCollectorState()
	state.Programs = []ProgramStatus{
		{Name: "packet_marker", Enabled: true, Loaded: true},
		{Name: "flow_tracker", Enabled: true, Loaded: false}, // Not loaded!
		{Name: "latency_probe", Enabled: false, Loaded: false}, // Disabled, OK
	}

	handler := &HealthHandler{State: state}
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if body["status"] != "unhealthy" {
		t.Errorf("status = %v, want \"unhealthy\"", body["status"])
	}
}

func TestHealthHandler_DisabledProgramsOK(t *testing.T) {
	state := NewCollectorState()
	state.Programs = []ProgramStatus{
		{Name: "packet_marker", Enabled: false, Loaded: false},
		{Name: "flow_tracker", Enabled: false, Loaded: false},
		{Name: "latency_probe", Enabled: false, Loaded: false},
	}

	handler := &HealthHandler{State: state}
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (all disabled = healthy)", w.Code)
	}
}

func TestHealthHandler_ProgramsInResponse(t *testing.T) {
	state := NewCollectorState()
	state.Programs = []ProgramStatus{
		{Name: "packet_marker", Enabled: true, Loaded: true},
	}

	handler := &HealthHandler{State: state}
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var body struct {
		Programs []ProgramStatus `json:"programs"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(body.Programs) != 1 {
		t.Fatalf("expected 1 program, got %d", len(body.Programs))
	}
	if body.Programs[0].Name != "packet_marker" {
		t.Errorf("program name = %q, want \"packet_marker\"", body.Programs[0].Name)
	}
}

// ── ReadyHandler tests ────────────────────────────────────────────────────

func TestReadyHandler_NotReady(t *testing.T) {
	state := NewCollectorState()

	handler := &ReadyHandler{State: state}
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}

	var body map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["status"] != "not_ready" {
		t.Errorf("status = %v, want \"not_ready\"", body["status"])
	}
}

func TestReadyHandler_Ready(t *testing.T) {
	state := NewCollectorState()
	// Simulate processing an event
	te := makeTraceEntry(
		net.ParseIP("10.0.0.1"),
		net.ParseIP("10.0.0.2"),
		8080, 443, 6, uint64(time.Now().UnixNano()),
	)
	state.RecordTrace(te)

	handler := &ReadyHandler{State: state}
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var body map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["status"] != "ready" {
		t.Errorf("status = %v, want \"ready\"", body["status"])
	}
}

// ── TracesHandler tests ───────────────────────────────────────────────────

func TestTracesHandler_Empty(t *testing.T) {
	state := NewCollectorState()

	handler := &TracesHandler{State: state}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var body struct {
		Count  int                 `json:"count"`
		Traces []*recentTraceEntry `json:"traces"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if body.Count != 0 {
		t.Errorf("count = %d, want 0", body.Count)
	}
}

func TestTracesHandler_WithEntries(t *testing.T) {
	state := NewCollectorState()

	// Add some trace entries
	for i := 0; i < 5; i++ {
		te := makeTraceEntry(
			net.ParseIP("10.0.0.1"),
			net.ParseIP("10.0.0.2"),
			uint16(8080+i), 443, 6, uint64(i*1000),
		)
		state.RecordTrace(te)
	}

	handler := &TracesHandler{State: state}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var body struct {
		Count  int                 `json:"count"`
		Traces []*recentTraceEntry `json:"traces"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)

	if body.Count != 5 {
		t.Errorf("count = %d, want 5", body.Count)
	}
}

// ── StatsHandler tests ────────────────────────────────────────────────────

func TestStatsHandler_Basic(t *testing.T) {
	state := NewCollectorState()
	pubConfig := DefaultTracePublisherConfig()
	state.Publisher = NewTracePublisher(pubConfig)
	state.PacketCorrelator = NewPacketCorrelator(DefaultPacketCorrelatorConfig())

	handler := &StatsHandler{State: state}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if _, ok := body["events_processed"]; !ok {
		t.Error("missing events_processed in stats response")
	}
	if _, ok := body["publisher"]; !ok {
		t.Error("missing publisher in stats response")
	}
}

func TestStatsHandler_NilComponents(t *testing.T) {
	state := NewCollectorState()

	handler := &StatsHandler{State: state}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// ── CollectorState tests ──────────────────────────────────────────────────

func TestCollectorState_RecordAndRetrieve(t *testing.T) {
	state := NewCollectorState()

	// Record 10 entries
	for i := 0; i < 10; i++ {
		te := makeTraceEntry(
			net.ParseIP("10.0.0.1"),
			net.ParseIP("10.0.0.2"),
			uint16(1000+i), 80, 6, uint64(i*1000),
		)
		state.RecordTrace(te)
	}

	traces := state.GetRecentTraces(5)
	if len(traces) != 5 {
		t.Errorf("GetRecentTraces(5) returned %d, want 5", len(traces))
	}

	// Should be newest first (descending SrcPort)
	if traces[0].SrcPort != 1009 {
		t.Errorf("newest trace SrcPort = %d, want 1009", traces[0].SrcPort)
	}
}

func TestCollectorState_RingBufferWrap(t *testing.T) {
	state := NewCollectorState()

	// Fill more than maxRecentTraces
	for i := 0; i < maxRecentTraces+50; i++ {
		te := makeTraceEntry(
			net.ParseIP("10.0.0.1"),
			net.ParseIP("10.0.0.2"),
			uint16(i%65536), 80, 6, uint64(i*1000),
		)
		state.RecordTrace(te)
	}

	traces := state.GetRecentTraces(maxRecentTraces)
	if len(traces) != maxRecentTraces {
		t.Errorf("GetRecentTraces(%d) returned %d", maxRecentTraces, len(traces))
	}
}

func TestCollectorState_RecordAnamnesisEvent(t *testing.T) {
	state := NewCollectorState()
	state.RecordAnamnesisEvent()
	state.RecordAnamnesisEvent()
	state.RecordAnamnesisEvent()

	if state.EventsProcessed != 3 {
		t.Errorf("EventsProcessed = %d, want 3", state.EventsProcessed)
	}
}
