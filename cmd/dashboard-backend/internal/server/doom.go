// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package server — Doom API endpoints for the dashboard backend.
//
// These endpoints provide a REST-based alternative to doom-bridge's
// WebSocket streaming, enabling doom-viewport.js to poll for framebuffer
// and CPU state data without requiring a running doom-bridge instance.
//
// Endpoints:
//   GET  /api/v1/doom/screen — Returns screen framebuffer (16000 bytes base64 or raw)
//   GET  /api/v1/doom/cpu    — Returns CPU state as JSON
//   POST /api/v1/doom/input  — Accepts keyboard input
package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// DoomState holds the latest Doom compute state for API serving.
// Updated by the eBPF ingestor when compute.* events arrive.
type DoomState struct {
	mu sync.RWMutex

	// CPU state
	PC         uint32   `json:"pc"`
	Regs       [16]uint32 `json:"regs"`
	Flags      uint8    `json:"flags"`
	Halted     bool     `json:"halted"`
	Stalled    bool     `json:"stalled"`
	InsnCount  uint64   `json:"insns"`
	CacheHits  uint64   `json:"cache_hits"`
	CacheMisses uint64  `json:"cache_misses"`
	Packets    uint64   `json:"packets"`
	Ticks      uint64   `json:"ticks"`
	LastUpdate time.Time `json:"last_update"`

	// Screen buffer (160x100 = 16000 bytes, palette-indexed)
	Screen [16000]byte `json:"-"`

	// Keyboard state
	LastKey     uint16 `json:"last_key"`
	KeyPressed  bool   `json:"key_pressed"`
}

// UpdateCPU updates the CPU state fields.
func (ds *DoomState) UpdateCPU(pc uint32, regs [16]uint32, flags uint8, halted, stalled bool, insns, hits, misses uint64) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.PC = pc
	ds.Regs = regs
	ds.Flags = flags
	ds.Halted = halted
	ds.Stalled = stalled
	ds.InsnCount = insns
	ds.CacheHits = hits
	ds.CacheMisses = misses
	ds.LastUpdate = time.Now()
}

// UpdateScreen updates the screen buffer from raw bytes.
func (ds *DoomState) UpdateScreen(data []byte) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	n := len(data)
	if n > 16000 {
		n = 16000
	}
	copy(ds.Screen[:n], data[:n])
	ds.LastUpdate = time.Now()
}

// snapshot returns a copy of the current state under read lock.
// Returns a new DoomState with all fields copied (excluding the mutex).
func (ds *DoomState) snapshot() DoomState {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	var s DoomState
	s.PC = ds.PC
	s.Regs = ds.Regs
	s.Flags = ds.Flags
	s.Halted = ds.Halted
	s.Stalled = ds.Stalled
	s.InsnCount = ds.InsnCount
	s.CacheHits = ds.CacheHits
	s.CacheMisses = ds.CacheMisses
	s.Packets = ds.Packets
	s.Ticks = ds.Ticks
	s.LastUpdate = ds.LastUpdate
	s.Screen = ds.Screen
	s.LastKey = ds.LastKey
	s.KeyPressed = ds.KeyPressed
	return s
}

// handleDoomScreen serves the screen framebuffer.
// GET /api/v1/doom/screen
//   ?format=base64 (default) — returns JSON { "screen": "<base64>" }
//   ?format=raw — returns raw 16000 bytes as application/octet-stream
func (s *Server) handleDoomScreen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	snap := s.doomState.snapshot()
	format := r.URL.Query().Get("format")

	if format == "raw" {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", "16000")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(snap.Screen[:])
		return
	}

	// Default: base64 JSON
	w.Header().Set("Content-Type", "application/json")
	resp := struct {
		Screen     string `json:"screen"`
		NonZero    int    `json:"non_zero_pixels"`
		LastUpdate string `json:"last_update"`
	}{
		Screen:     base64.StdEncoding.EncodeToString(snap.Screen[:]),
		LastUpdate: snap.LastUpdate.Format(time.RFC3339Nano),
	}
	for _, b := range snap.Screen {
		if b != 0 {
			resp.NonZero++
		}
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// handleDoomCPU serves the CPU state.
// GET /api/v1/doom/cpu
func (s *Server) handleDoomCPU(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	snap := s.doomState.snapshot()
	w.Header().Set("Content-Type", "application/json")

	resp := struct {
		Type        string     `json:"type"`
		PC          uint32     `json:"pc"`
		Regs        [16]uint32 `json:"regs"`
		Flags       uint8      `json:"flags"`
		Halted      bool       `json:"halted"`
		Stalled     bool       `json:"stalled"`
		Insns       uint64     `json:"insns"`
		CacheHits   uint64     `json:"cache_hits"`
		CacheMisses uint64     `json:"cache_misses"`
		Packets     uint64     `json:"packets"`
		Ticks       uint64     `json:"ticks"`
		LastUpdate  string     `json:"last_update"`
	}{
		Type:        "stats",
		PC:          snap.PC,
		Regs:        snap.Regs,
		Flags:       snap.Flags,
		Halted:      snap.Halted,
		Stalled:     snap.Stalled,
		Insns:       snap.InsnCount,
		CacheHits:   snap.CacheHits,
		CacheMisses: snap.CacheMisses,
		Packets:     snap.Packets,
		Ticks:       snap.Ticks,
		LastUpdate:  snap.LastUpdate.Format(time.RFC3339Nano),
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// doomInputRequest is the JSON body for keyboard input.
type doomInputRequest struct {
	Key     uint16 `json:"key"`
	Pressed bool   `json:"pressed"`
}

// handleDoomInput accepts keyboard input for the Doom compute engine.
// POST /api/v1/doom/input
func (s *Server) handleDoomInput(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req doomInputRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	s.doomState.mu.Lock()
	s.doomState.LastKey = req.Key
	s.doomState.KeyPressed = req.Pressed
	s.doomState.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
