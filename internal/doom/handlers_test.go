// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package doom

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// screenMockMap is a mock that returns a full screen buffer.
type screenMockMap struct {
	data []byte
}

func newScreenMockMap(fill byte) *screenMockMap {
	data := make([]byte, ScreenBytes)
	for i := range data {
		data[i] = fill
	}
	return &screenMockMap{data: data}
}

func (m *screenMockMap) LookupElem(key []byte, valueSize int) ([]byte, error) {
	if valueSize >= ScreenBytes {
		result := make([]byte, valueSize)
		copy(result, m.data)
		return result, nil
	}
	// Chunk read
	idx := binary.LittleEndian.Uint32(key)
	offset := int(idx) * 4
	if offset+4 > len(m.data) {
		return nil, fmt.Errorf("out of range: %d", idx)
	}
	result := make([]byte, 4)
	copy(result, m.data[offset:offset+4])
	return result, nil
}

func (m *screenMockMap) UpdateElem(key, value []byte) error {
	return nil
}

// TestHandleStatusOK tests GET /doom/status with a valid CPU map.
func TestHandleStatusOK(t *testing.T) {
	cpuMap := newMockMap()
	cpu := DefaultCpuState()
	cpu.PC = 0x42
	cpu.Regs[0] = 0xDEAD
	cpu.InsnCount = 1234
	cpu.CacheHits = 100
	cpu.CacheMisses = 10
	_ = WriteCpuState(cpuMap, cpu)

	h := NewDoomHandler(cpuMap, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/doom/status", nil)
	rec := httptest.NewRecorder()
	h.HandleStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.PC != 0x42 {
		t.Errorf("PC = 0x%X, want 0x42", resp.PC)
	}
	if resp.InsnCount != 1234 {
		t.Errorf("InsnCount = %d, want 1234", resp.InsnCount)
	}
	if resp.Status != "running" {
		t.Errorf("Status = %q, want 'running'", resp.Status)
	}
	if resp.Registers[0] != 0xDEAD {
		t.Errorf("Registers[0] = 0x%X, want 0xDEAD", resp.Registers[0])
	}
}

// TestHandleStatusHalted tests GET /doom/status when CPU is halted.
func TestHandleStatusHalted(t *testing.T) {
	cpuMap := newMockMap()
	cpu := DefaultCpuState()
	cpu.Halted = 1
	_ = WriteCpuState(cpuMap, cpu)

	h := NewDoomHandler(cpuMap, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/doom/status", nil)
	rec := httptest.NewRecorder()
	h.HandleStatus(rec, req)

	var resp StatusResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)

	if resp.Status != "halted" {
		t.Errorf("Status = %q, want 'halted'", resp.Status)
	}
	if !resp.Halted {
		t.Error("Halted should be true")
	}
}

// TestHandleStatusNoCpuMap tests GET /doom/status without CPU map.
func TestHandleStatusNoCpuMap(t *testing.T) {
	h := NewDoomHandler(nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/doom/status", nil)
	rec := httptest.NewRecorder()
	h.HandleStatus(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

// TestHandleStatusMethodNotAllowed tests POST to /doom/status.
func TestHandleStatusMethodNotAllowed(t *testing.T) {
	h := NewDoomHandler(newMockMap(), nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/doom/status", nil)
	rec := httptest.NewRecorder()
	h.HandleStatus(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

// TestHandleScreenRaw tests GET /doom/screen with raw format.
func TestHandleScreenRaw(t *testing.T) {
	screen := newScreenMockMap(0xAA)
	h := NewDoomHandler(nil, screen, nil)

	req := httptest.NewRequest(http.MethodGet, "/doom/screen", nil)
	rec := httptest.NewRecorder()
	h.HandleScreen(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	if rec.Header().Get("Content-Type") != "application/octet-stream" {
		t.Errorf("Content-Type = %q, want application/octet-stream", rec.Header().Get("Content-Type"))
	}

	if rec.Body.Len() != ScreenBytes {
		t.Errorf("body len = %d, want %d", rec.Body.Len(), ScreenBytes)
	}

	// Verify content is 0xAA
	for i, b := range rec.Body.Bytes() {
		if b != 0xAA {
			t.Fatalf("byte[%d] = 0x%02X, want 0xAA", i, b)
		}
	}
}

// TestHandleScreenBase64 tests GET /doom/screen?format=base64.
func TestHandleScreenBase64(t *testing.T) {
	screen := newScreenMockMap(0x42)
	h := NewDoomHandler(nil, screen, nil)

	req := httptest.NewRequest(http.MethodGet, "/doom/screen?format=base64", nil)
	rec := httptest.NewRecorder()
	h.HandleScreen(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp ScreenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Width != ScreenWidth {
		t.Errorf("Width = %d, want %d", resp.Width, ScreenWidth)
	}
	if resp.Height != ScreenHeight {
		t.Errorf("Height = %d, want %d", resp.Height, ScreenHeight)
	}
	if resp.Format != "base64" {
		t.Errorf("Format = %q, want 'base64'", resp.Format)
	}

	decoded, err := base64.StdEncoding.DecodeString(resp.Data)
	if err != nil {
		t.Fatalf("decode base64: %v", err)
	}
	if len(decoded) != ScreenBytes {
		t.Errorf("decoded len = %d, want %d", len(decoded), ScreenBytes)
	}
}

// TestHandleScreenNoMap tests GET /doom/screen without screen map.
func TestHandleScreenNoMap(t *testing.T) {
	h := NewDoomHandler(nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/doom/screen", nil)
	rec := httptest.NewRecorder()
	h.HandleScreen(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

// TestHandleInputOK tests POST /doom/input with valid key event.
func TestHandleInputOK(t *testing.T) {
	ResetKeySequence()
	kbdMap := newMockMap()
	h := NewDoomHandler(nil, nil, kbdMap)

	body := `{"key": 42, "pressed": true}`
	req := httptest.NewRequest(http.MethodPost, "/doom/input", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleInput(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)

	if resp["ok"] != true {
		t.Errorf("ok = %v, want true", resp["ok"])
	}

	// Verify the key was written to the map
	ks, err := ReadKeyState(kbdMap)
	if err != nil {
		t.Fatalf("ReadKeyState: %v", err)
	}
	if ks.Key != 42 {
		t.Errorf("Key = %d, want 42", ks.Key)
	}
	if ks.Pressed != 1 {
		t.Errorf("Pressed = %d, want 1", ks.Pressed)
	}
}

// TestHandleInputRelease tests POST /doom/input with key release.
func TestHandleInputRelease(t *testing.T) {
	ResetKeySequence()
	kbdMap := newMockMap()
	h := NewDoomHandler(nil, nil, kbdMap)

	body := `{"key": 10, "pressed": false}`
	req := httptest.NewRequest(http.MethodPost, "/doom/input", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.HandleInput(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	ks, _ := ReadKeyState(kbdMap)
	if ks.Pressed != 0 {
		t.Errorf("Pressed = %d, want 0", ks.Pressed)
	}
}

// TestHandleInputNoMap tests POST /doom/input without keyboard map.
func TestHandleInputNoMap(t *testing.T) {
	h := NewDoomHandler(nil, nil, nil)

	body := `{"key": 42, "pressed": true}`
	req := httptest.NewRequest(http.MethodPost, "/doom/input", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.HandleInput(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

// TestHandleInputBadJSON tests POST /doom/input with invalid JSON.
func TestHandleInputBadJSON(t *testing.T) {
	kbdMap := newMockMap()
	h := NewDoomHandler(nil, nil, kbdMap)

	body := `not json at all`
	req := httptest.NewRequest(http.MethodPost, "/doom/input", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.HandleInput(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// TestHandleInputMethodNotAllowed tests GET to /doom/input.
func TestHandleInputMethodNotAllowed(t *testing.T) {
	h := NewDoomHandler(nil, nil, newMockMap())

	req := httptest.NewRequest(http.MethodGet, "/doom/input", nil)
	rec := httptest.NewRecorder()
	h.HandleInput(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

// TestRegisterRoutes tests that routes are registered correctly.
func TestRegisterRoutes(t *testing.T) {
	h := NewDoomHandler(nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Verify we can hit the endpoints (they will return 503 since maps are nil)
	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/doom/screen"},
		{http.MethodGet, "/doom/status"},
		{http.MethodPost, "/doom/input"},
	}

	for _, ep := range endpoints {
		req := httptest.NewRequest(ep.method, ep.path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		// Should not be 404 (routes are registered)
		if rec.Code == http.StatusNotFound {
			t.Errorf("%s %s returned 404, route not registered", ep.method, ep.path)
		}
	}
}

// TestStatusResponseFlags tests flag string formatting in status response.
func TestStatusResponseFlags(t *testing.T) {
	cpuMap := newMockMap()
	cpu := DefaultCpuState()
	cpu.Flags = FlagZero | FlagNegative | FlagCarry
	_ = WriteCpuState(cpuMap, cpu)

	h := NewDoomHandler(cpuMap, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/doom/status", nil)
	rec := httptest.NewRecorder()
	h.HandleStatus(rec, req)

	var resp StatusResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)

	if resp.FlagStr != "ZNC" {
		t.Errorf("FlagStr = %q, want 'ZNC'", resp.FlagStr)
	}
	if resp.Flags != FlagZero|FlagNegative|FlagCarry {
		t.Errorf("Flags = 0x%02X, want 0x%02X", resp.Flags, FlagZero|FlagNegative|FlagCarry)
	}
}
