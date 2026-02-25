// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// health.go — Health, readiness, metrics, and API endpoints.
//
// Provides HTTP handlers for:
//   /health     — All enabled BPF programs loaded successfully
//   /ready      — At least one trace event has been processed
//   /metrics    — Prometheus metrics (via promhttp)
//   /api/v1/traces  — Recent trace events (JSON)
//   /api/v1/stats   — Current statistics (JSON)
//
// Per RFC 9669: we use "BPF" (not "eBPF") throughout.
package main

import (
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"unheaded/pkg/ebpf"
)

// ── ProgramStatus ─────────────────────────────────────────────────────────

// ProgramStatus tracks whether a BPF program is enabled and loaded.
type ProgramStatus struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Loaded  bool   `json:"loaded"`
}

// ── CollectorState ────────────────────────────────────────────────────────

// CollectorState holds the shared state used by health and API handlers.
type CollectorState struct {
	mu sync.RWMutex

	// Programs tracks BPF program load status.
	Programs []ProgramStatus

	// Publisher is the trace publisher.
	Publisher *TracePublisher

	// PacketCorrelator is the 5-tuple correlator (from correlator.go).
	PacketCorrelator *PacketCorrelator

	// FlowCorrelator is the Anamnesis flow correlator.
	FlowCorrelator *FlowCorrelator

	// TraceReader is the BPF map reader.
	TraceReader *TraceReader

	// AnamnesisReader is the BPF ring buffer reader (from pkg/ebpf).
	AnamnesisReader *ebpf.AnamnesisReader

	// EventsProcessed counts total events across all sources.
	EventsProcessed uint64

	// RecentTraces stores a ring buffer of recent trace entries for the API.
	recentTraces []*recentTraceEntry
	traceIndex   int
	traceMu      sync.Mutex
}

// recentTraceEntry wraps a trace event with a receive timestamp.
type recentTraceEntry struct {
	ReceivedAt time.Time   `json:"received_at"`
	TraceID    string      `json:"trace_id"`
	SrcIP      string      `json:"src_ip"`
	DstIP      string      `json:"dst_ip"`
	SrcPort    uint16      `json:"src_port"`
	DstPort    uint16      `json:"dst_port"`
	Protocol   string      `json:"protocol"`
	PacketLen  uint16      `json:"packet_len"`
	HopCount   uint8       `json:"hop_count"`
}

const maxRecentTraces = 256

// NewCollectorState creates a new state holder.
func NewCollectorState() *CollectorState {
	return &CollectorState{
		recentTraces: make([]*recentTraceEntry, maxRecentTraces),
	}
}

// RecordTrace records a trace entry for the recent traces API.
func (cs *CollectorState) RecordTrace(te *TraceEntry) {
	atomic.AddUint64(&cs.EventsProcessed, 1)

	entry := &recentTraceEntry{
		ReceivedAt: time.Now(),
		TraceID:    te.TraceIDHex(),
		SrcIP:      te.SrcIPAddr().String(),
		DstIP:      te.DstIPAddr().String(),
		SrcPort:    te.SrcPort,
		DstPort:    te.DstPort,
		Protocol:   te.ProtocolName(),
		PacketLen:  te.PacketLen,
		HopCount:   te.HopCount,
	}

	cs.traceMu.Lock()
	cs.recentTraces[cs.traceIndex%maxRecentTraces] = entry
	cs.traceIndex++
	cs.traceMu.Unlock()
}

// RecordAnamnesisEvent records an Anamnesis event for the counter.
func (cs *CollectorState) RecordAnamnesisEvent() {
	atomic.AddUint64(&cs.EventsProcessed, 1)
}

// GetRecentTraces returns up to the last N recent traces, newest first.
func (cs *CollectorState) GetRecentTraces(limit int) []*recentTraceEntry {
	cs.traceMu.Lock()
	defer cs.traceMu.Unlock()

	if limit <= 0 || limit > maxRecentTraces {
		limit = maxRecentTraces
	}

	total := cs.traceIndex
	if total > maxRecentTraces {
		total = maxRecentTraces
	}
	if limit > total {
		limit = total
	}

	result := make([]*recentTraceEntry, 0, limit)
	for i := 0; i < limit; i++ {
		idx := (cs.traceIndex - 1 - i) % maxRecentTraces
		if idx < 0 {
			idx += maxRecentTraces
		}
		entry := cs.recentTraces[idx]
		if entry != nil {
			result = append(result, entry)
		}
	}
	return result
}

// ── Health handler (/health) ──────────────────────────────────────────────

// HealthHandler reports whether all enabled BPF programs are loaded.
type HealthHandler struct {
	State *CollectorState
}

func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.State.mu.RLock()
	programs := make([]ProgramStatus, len(h.State.Programs))
	copy(programs, h.State.Programs)
	h.State.mu.RUnlock()

	allLoaded := true
	for _, p := range programs {
		if p.Enabled && !p.Loaded {
			allLoaded = false
			break
		}
	}

	status := "healthy"
	httpStatus := http.StatusOK
	if !allLoaded {
		status = "unhealthy"
		httpStatus = http.StatusServiceUnavailable
	}

	resp := struct {
		Status   string          `json:"status"`
		Programs []ProgramStatus `json:"programs"`
		Wotan    bool            `json:"wotan_connected"`
	}{
		Status:   status,
		Programs: programs,
	}

	if h.State.Publisher != nil {
		resp.Wotan = h.State.Publisher.IsConnected()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(resp)
}

// ── Ready handler (/ready) ────────────────────────────────────────────────

// ReadyHandler reports whether the collector has processed at least one event.
type ReadyHandler struct {
	State *CollectorState
}

func (h *ReadyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	processed := atomic.LoadUint64(&h.State.EventsProcessed)
	ready := processed > 0

	status := "ready"
	httpStatus := http.StatusOK
	if !ready {
		status = "not_ready"
		httpStatus = http.StatusServiceUnavailable
	}

	resp := struct {
		Status          string `json:"status"`
		EventsProcessed uint64 `json:"events_processed"`
	}{
		Status:          status,
		EventsProcessed: processed,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(resp)
}

// ── Traces API handler (/api/v1/traces) ───────────────────────────────────

// TracesHandler returns recent trace events as JSON.
type TracesHandler struct {
	State *CollectorState
}

func (h *TracesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	traces := h.State.GetRecentTraces(100)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		Count  int                  `json:"count"`
		Traces []*recentTraceEntry  `json:"traces"`
	}{
		Count:  len(traces),
		Traces: traces,
	})
}

// ── Stats API handler (/api/v1/stats) ─────────────────────────────────────

// StatsHandler returns current statistics as JSON.
type StatsHandler struct {
	State *CollectorState
}

func (h *StatsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	resp := struct {
		EventsProcessed     uint64                     `json:"events_processed"`
		Publisher            *TracePublisherStats       `json:"publisher,omitempty"`
		PacketCorrelator     *PacketCorrelatorStats     `json:"packet_correlator,omitempty"`
		TraceReader          *TraceReaderStats          `json:"trace_reader,omitempty"`
		AnamnesisReader      *ebpf.ReaderStats          `json:"anamnesis_reader,omitempty"`
	}{
		EventsProcessed: atomic.LoadUint64(&h.State.EventsProcessed),
	}

	if h.State.Publisher != nil {
		stats := h.State.Publisher.Stats()
		resp.Publisher = &stats
	}
	if h.State.PacketCorrelator != nil {
		stats := h.State.PacketCorrelator.Stats()
		resp.PacketCorrelator = &stats
	}
	if h.State.TraceReader != nil {
		stats := h.State.TraceReader.Stats()
		resp.TraceReader = &stats
	}
	if h.State.AnamnesisReader != nil {
		stats := h.State.AnamnesisReader.Stats()
		resp.AnamnesisReader = &stats
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
