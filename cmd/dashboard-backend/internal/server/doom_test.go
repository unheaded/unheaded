// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDoomState_UpdateCPU(t *testing.T) {
	ds := &DoomState{}
	regs := [16]uint32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 0xFFFF0000}
	ds.UpdateCPU(42, regs, 0x03, false, false, 1000, 50, 10)

	snap := ds.snapshot()
	if snap.PC != 42 {
		t.Errorf("PC = %d, want 42", snap.PC)
	}
	if snap.Regs[0] != 1 {
		t.Errorf("Regs[0] = %d, want 1", snap.Regs[0])
	}
	if snap.Flags != 0x03 {
		t.Errorf("Flags = %d, want 3", snap.Flags)
	}
	if snap.InsnCount != 1000 {
		t.Errorf("InsnCount = %d, want 1000", snap.InsnCount)
	}
	if snap.LastUpdate.IsZero() {
		t.Error("LastUpdate should be set")
	}
}

func TestDoomState_UpdateScreen(t *testing.T) {
	ds := &DoomState{}

	// Fill first 100 bytes with test pattern
	data := make([]byte, 100)
	for i := range data {
		data[i] = byte(i)
	}
	ds.UpdateScreen(data)

	snap := ds.snapshot()
	for i := 0; i < 100; i++ {
		if snap.Screen[i] != byte(i) {
			t.Errorf("Screen[%d] = %d, want %d", i, snap.Screen[i], i)
			break
		}
	}
	// Remaining should be zero
	if snap.Screen[100] != 0 {
		t.Errorf("Screen[100] = %d, want 0", snap.Screen[100])
	}
}

func TestDoomState_UpdateScreen_Oversized(t *testing.T) {
	ds := &DoomState{}
	// Should not panic with oversized data
	data := make([]byte, 70000)
	data[63999] = 0xFF
	ds.UpdateScreen(data)

	snap := ds.snapshot()
	if snap.Screen[63999] != 0xFF {
		t.Errorf("Screen[63999] = %d, want 0xFF", snap.Screen[63999])
	}
}

func TestHandleDoomCPU(t *testing.T) {
	s := &Server{
		doomState: &DoomState{},
	}

	regs := [16]uint32{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xFFFF0000}
	s.doomState.UpdateCPU(123, regs, 0x01, false, false, 5000, 100, 20)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/doom/cpu", nil)
	w := httptest.NewRecorder()
	s.handleDoomCPU(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp struct {
		Type   string     `json:"type"`
		PC     uint32     `json:"pc"`
		Regs   [16]uint32 `json:"regs"`
		Insns  uint64     `json:"insns"`
		Halted bool       `json:"halted"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.PC != 123 {
		t.Errorf("PC = %d, want 123", resp.PC)
	}
	if resp.Insns != 5000 {
		t.Errorf("Insns = %d, want 5000", resp.Insns)
	}
	if resp.Type != "stats" {
		t.Errorf("Type = %q, want 'stats'", resp.Type)
	}
}

func TestHandleDoomScreen_Base64(t *testing.T) {
	s := &Server{
		doomState: &DoomState{},
	}

	// Put test pattern in screen
	data := make([]byte, 64000)
	data[0] = 0xAA
	data[100] = 0xBB
	s.doomState.UpdateScreen(data)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/doom/screen", nil)
	w := httptest.NewRecorder()
	s.handleDoomScreen(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp struct {
		Screen  string `json:"screen"`
		NonZero int    `json:"non_zero_pixels"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.NonZero != 2 {
		t.Errorf("NonZero = %d, want 2", resp.NonZero)
	}

	decoded, err := base64.StdEncoding.DecodeString(resp.Screen)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	if len(decoded) != 64000 {
		t.Errorf("decoded len = %d, want 64000", len(decoded))
	}
	if decoded[0] != 0xAA {
		t.Errorf("decoded[0] = 0x%02X, want 0xAA", decoded[0])
	}
}

func TestHandleDoomScreen_Raw(t *testing.T) {
	s := &Server{
		doomState: &DoomState{},
	}

	data := make([]byte, 64000)
	data[0] = 0xCC
	s.doomState.UpdateScreen(data)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/doom/screen?format=raw", nil)
	w := httptest.NewRecorder()
	s.handleDoomScreen(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if w.Body.Len() != 64000 {
		t.Errorf("body len = %d, want 64000", w.Body.Len())
	}
	if w.Body.Bytes()[0] != 0xCC {
		t.Errorf("body[0] = 0x%02X, want 0xCC", w.Body.Bytes()[0])
	}
}

func TestHandleDoomInput(t *testing.T) {
	s := &Server{
		doomState: &DoomState{},
	}

	body := `{"key":72,"pressed":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/doom/input", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleDoomInput(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	snap := s.doomState.snapshot()
	if snap.LastKey != 72 {
		t.Errorf("LastKey = %d, want 72", snap.LastKey)
	}
	if !snap.KeyPressed {
		t.Error("KeyPressed should be true")
	}
}

func TestHandleDoomInput_InvalidJSON(t *testing.T) {
	s := &Server{
		doomState: &DoomState{},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/doom/input", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	s.handleDoomInput(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleDoomCPU_MethodNotAllowed(t *testing.T) {
	s := &Server{
		doomState: &DoomState{},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/doom/cpu", nil)
	w := httptest.NewRecorder()
	s.handleDoomCPU(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}
