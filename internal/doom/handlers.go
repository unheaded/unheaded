// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package doom — HTTP handlers for Doom-over-IPv6 dashboard endpoints.
//
// These handlers expose the BPF map data (screen buffer, CPU state, keyboard)
// over HTTP so the dashboard can render the Doom framebuffer and interact
// with the running MBC CPU.
//
// Endpoints:
//   - GET  /doom/screen — read 160x100 framebuffer, returns raw binary or base64 JSON
//   - GET  /doom/status — read CPU state, returns JSON
//   - POST /doom/input  — accept keyboard state JSON, write to KBD map
package doom

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ScreenWidth is the Doom framebuffer width in pixels.
const ScreenWidth = 160

// ScreenHeight is the Doom framebuffer height in pixels.
const ScreenHeight = 100

// ScreenBytes is the total framebuffer size (160 * 100 = 16000 bytes).
const ScreenBytes = ScreenWidth * ScreenHeight

// DoomHandler provides HTTP handlers for the Doom-over-IPv6 dashboard.
// It reads/writes BPF maps through the MapAccessor interface.
type DoomHandler struct {
	// cpuMap accesses the CPU state BPF map.
	cpuMap MapAccessor
	// screenMap accesses the screen framebuffer BPF map.
	screenMap MapAccessor
	// kbdMap accesses the keyboard state BPF map.
	kbdMap MapAccessor
}

// NewDoomHandler creates a new DoomHandler with the given BPF map accessors.
// Any accessor may be nil; handlers will return 503 if the required map is unavailable.
func NewDoomHandler(cpuMap, screenMap, kbdMap MapAccessor) *DoomHandler {
	return &DoomHandler{
		cpuMap:    cpuMap,
		screenMap: screenMap,
		kbdMap:    kbdMap,
	}
}

// RegisterRoutes registers the Doom endpoints on the given ServeMux.
func (h *DoomHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/doom/screen", h.HandleScreen)
	mux.HandleFunc("/doom/status", h.HandleStatus)
	mux.HandleFunc("/doom/input", h.HandleInput)
}

// StatusResponse is the JSON response for GET /doom/status.
type StatusResponse struct {
	PC          uint32   `json:"pc"`
	Registers   []uint32 `json:"registers"`
	Flags       uint8    `json:"flags"`
	FlagStr     string   `json:"flag_str"`
	Halted      bool     `json:"halted"`
	Stalled     bool     `json:"stalled"`
	InsnCount   uint64   `json:"insn_count"`
	CacheHits   uint64   `json:"cache_hits"`
	CacheMisses uint64   `json:"cache_misses"`
	SleepUntil  uint64   `json:"sleep_until"`
	Status      string   `json:"status"`
}

// ScreenResponse is the JSON response for GET /doom/screen?format=base64.
type ScreenResponse struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Format string `json:"format"`
	Data   string `json:"data"`
}

// InputRequest is the JSON body for POST /doom/input.
type InputRequest struct {
	Key     uint32 `json:"key"`
	Pressed bool   `json:"pressed"`
}

// HandleScreen handles GET /doom/screen.
//
// Query parameters:
//   - format=raw (default): returns raw binary (application/octet-stream)
//   - format=base64: returns base64-encoded JSON with width/height metadata
//
// The screen buffer is read from screenMap as 16000 bytes starting at index 0.
func (h *DoomHandler) HandleScreen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.screenMap == nil {
		http.Error(w, `{"error":"screen map not available"}`, http.StatusServiceUnavailable)
		return
	}

	// Read screen buffer from map (indexed by 4-byte keys, each value is a chunk)
	// For simplicity, we read the entire buffer as a single large value at key 0.
	// The actual BPF map layout may use per-row or per-chunk keys.
	screenData, err := h.readScreenBuffer()
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"read screen: %s"}`, err), http.StatusInternalServerError)
		return
	}

	format := r.URL.Query().Get("format")
	if strings.EqualFold(format, "base64") {
		encoded := base64.StdEncoding.EncodeToString(screenData)
		resp := ScreenResponse{
			Width:  ScreenWidth,
			Height: ScreenHeight,
			Format: "base64",
			Data:   encoded,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	// Default: raw binary
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(screenData)))
	w.Header().Set("X-Screen-Width", fmt.Sprintf("%d", ScreenWidth))
	w.Header().Set("X-Screen-Height", fmt.Sprintf("%d", ScreenHeight))
	w.Write(screenData)
}

// HandleStatus handles GET /doom/status.
// Returns JSON with CPU state: PC, registers, flags, cycle count, etc.
func (h *DoomHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.cpuMap == nil {
		http.Error(w, `{"error":"cpu map not available"}`, http.StatusServiceUnavailable)
		return
	}

	cpu, err := ReadCpuState(h.cpuMap)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"read cpu state: %s"}`, err), http.StatusInternalServerError)
		return
	}

	// Build flag string
	flagStr := ""
	if cpu.HasFlag(FlagZero) {
		flagStr += "Z"
	}
	if cpu.HasFlag(FlagNegative) {
		flagStr += "N"
	}
	if cpu.HasFlag(FlagCarry) {
		flagStr += "C"
	}
	if flagStr == "" {
		flagStr = "-"
	}

	status := "running"
	if cpu.IsHalted() {
		status = "halted"
	} else if cpu.IsStalled() {
		status = "stalled"
	}

	regs := make([]uint32, RegCount)
	copy(regs, cpu.Regs[:])

	resp := StatusResponse{
		PC:          cpu.PC,
		Registers:   regs,
		Flags:       cpu.Flags,
		FlagStr:     flagStr,
		Halted:      cpu.IsHalted(),
		Stalled:     cpu.IsStalled(),
		InsnCount:   cpu.InsnCount,
		CacheHits:   cpu.CacheHits,
		CacheMisses: cpu.CacheMisses,
		SleepUntil:  cpu.SleepUntil,
		Status:      status,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// HandleInput handles POST /doom/input.
// Accepts JSON body with key scancode and pressed state.
// Writes the keyboard event to the KBD BPF map.
func (h *DoomHandler) HandleInput(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.kbdMap == nil {
		http.Error(w, `{"error":"keyboard map not available"}`, http.StatusServiceUnavailable)
		return
	}

	// SECURITY: Limit body size to prevent DoS
	var input InputRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&input); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"invalid JSON: %s"}`, err), http.StatusBadRequest)
		return
	}

	if err := InjectKey(h.kbdMap, input.Key, input.Pressed); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"inject key: %s"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":      true,
		"key":     input.Key,
		"pressed": input.Pressed,
	})
}

// readScreenBuffer reads the full 16000-byte screen buffer from the screenMap.
// The BPF map stores screen data as contiguous bytes. We read it in 4-byte chunks
// indexed by sequential u32 keys (0, 1, 2, ..., 3999).
func (h *DoomHandler) readScreenBuffer() ([]byte, error) {
	// Try reading the whole buffer as a single entry at key 0 first.
	// This works if the map is an array with a single large value.
	key := make([]byte, 4)
	binary.LittleEndian.PutUint32(key, 0)

	data, err := h.screenMap.LookupElem(key, ScreenBytes)
	if err == nil && len(data) >= ScreenBytes {
		return data[:ScreenBytes], nil
	}

	// Fallback: read 4 bytes at a time (4000 entries for 16000 bytes)
	buf := make([]byte, 0, ScreenBytes)
	chunkCount := ScreenBytes / 4
	for i := 0; i < chunkCount; i++ {
		binary.LittleEndian.PutUint32(key, uint32(i))
		chunk, err := h.screenMap.LookupElem(key, 4)
		if err != nil {
			return nil, fmt.Errorf("screen chunk %d: %w", i, err)
		}
		buf = append(buf, chunk...)
	}
	return buf, nil
}
