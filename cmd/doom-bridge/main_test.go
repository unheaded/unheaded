// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestBridge creates a bridge in dry-run mode for testing.
func newTestBridge() *bridge {
	return &bridge{
		port:    0,
		mapPath: "",
		dryRun:  true,
		static:  ".",
		clients: make(map[*client]struct{}),
	}
}

func TestHandleHealth(t *testing.T) {
	b := newTestBridge()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	b.handleHealth(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Health status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body := w.Body.String()
	if !strings.Contains(body, `"status":"ok"`) {
		t.Errorf("Health body = %q, want status ok", body)
	}
	if !strings.Contains(body, `"dry_run":true`) {
		t.Errorf("Health body = %q, want dry_run true", body)
	}
}

func TestHandleReady_DryRun(t *testing.T) {
	b := newTestBridge()

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	b.handleReady(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Ready status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body := w.Body.String()
	if !strings.Contains(body, `"status":"ready"`) {
		t.Errorf("Ready body = %q, want status ready", body)
	}
	if !strings.Contains(body, `"version":"0.1.0-alpha"`) {
		t.Errorf("Ready body = %q, want version", body)
	}
}

func TestHandleReady_NoMaps(t *testing.T) {
	b := newTestBridge()
	b.dryRun = false // Not dry-run, but no maps opened

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	b.handleReady(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Ready status = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}

	body := w.Body.String()
	if !strings.Contains(body, `"status":"not_ready"`) {
		t.Errorf("Ready body = %q, want status not_ready", body)
	}
}

func TestHandleMetrics(t *testing.T) {
	b := newTestBridge()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	b.handleMetrics(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Metrics status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/plain") {
		t.Errorf("Metrics Content-Type = %q, want text/plain", contentType)
	}

	body := w.Body.String()
	expectedMetrics := []string{
		"unheaded_doom_bridge_clients",
		"unheaded_doom_bridge_frames_total",
		"unheaded_doom_bridge_bytes_sent_total",
		"unheaded_doom_bridge_errors_total",
	}
	for _, m := range expectedMetrics {
		if !strings.Contains(body, m) {
			t.Errorf("Metrics body missing %q", m)
		}
	}
}

func TestStatsMessage_JSON(t *testing.T) {
	msg := statsMessage{
		Type:    "stats",
		Packets: 1200,
		Ticks:   60,
		Insns:   85000,
		Halted:  0,
		PC:      0x1234,
		Flags:   0x42,
		Regs:    make([]uint32, 16),
	}
	msg.Regs[0] = 42
	msg.Regs[15] = 0xFFFF0000

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded statsMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if decoded.Type != "stats" {
		t.Errorf("Type = %q, want stats", decoded.Type)
	}
	if decoded.Packets != 1200 {
		t.Errorf("Packets = %d, want 1200", decoded.Packets)
	}
	if decoded.PC != 0x1234 {
		t.Errorf("PC = 0x%X, want 0x1234", decoded.PC)
	}
	if decoded.Regs[0] != 42 {
		t.Errorf("Regs[0] = %d, want 42", decoded.Regs[0])
	}
	if decoded.Regs[15] != 0xFFFF0000 {
		t.Errorf("Regs[15] = 0x%X, want 0xFFFF0000", decoded.Regs[15])
	}
}

func TestLogf(t *testing.T) {
	// Just verify logf doesn't panic with various inputs
	logf("INFO", "test message")
	logf("WARN", "test with pairs", "key", "value")
	logf("ERROR", "test with multiple", "a", 1, "b", "two", "c", 3.14)
	logf("DEBUG", "test with odd pairs", "key") // odd number of kv pairs
}
