// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWriteHealthy(t *testing.T) {
	w := httptest.NewRecorder()
	WriteHealthy(w, "timeguru", "1.0.0")

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %s", ct)
	}

	var body Response
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Service != "timeguru" {
		t.Fatalf("expected service timeguru, got %s", body.Service)
	}
	if body.Status != StatusHealthy {
		t.Fatalf("expected status healthy, got %s", body.Status)
	}
	if body.Version != "1.0.0" {
		t.Fatalf("expected version 1.0.0, got %s", body.Version)
	}
	if body.Timestamp.IsZero() {
		t.Fatal("expected non-zero timestamp")
	}
	if time.Since(body.Timestamp) > 5*time.Second {
		t.Fatal("timestamp too far in the past")
	}
}

func TestWriteUnhealthy(t *testing.T) {
	w := httptest.NewRecorder()
	WriteUnhealthy(w, "captain", "2.0.0")

	resp := w.Result()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %s", ct)
	}

	var body Response
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Service != "captain" {
		t.Fatalf("expected service captain, got %s", body.Service)
	}
	if body.Status != StatusUnhealthy {
		t.Fatalf("expected status unhealthy, got %s", body.Status)
	}
	if body.Version != "2.0.0" {
		t.Fatalf("expected version 2.0.0, got %s", body.Version)
	}
}

func TestWriteDegraded(t *testing.T) {
	w := httptest.NewRecorder()
	WriteDegraded(w, "architect", "3.0.0")

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var body Response
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Service != "architect" {
		t.Fatalf("expected service architect, got %s", body.Service)
	}
	if body.Status != StatusDegraded {
		t.Fatalf("expected status degraded, got %s", body.Status)
	}
	if body.Version != "3.0.0" {
		t.Fatalf("expected version 3.0.0, got %s", body.Version)
	}
}

func TestWriteHealthResponseJSON(t *testing.T) {
	// Verify the JSON shape matches the documented format.
	w := httptest.NewRecorder()
	WriteHealthy(w, "wotan", "0.1.0")

	var raw map[string]interface{}
	if err := json.NewDecoder(w.Result().Body).Decode(&raw); err != nil {
		t.Fatalf("decode raw: %v", err)
	}
	for _, key := range []string{"service", "status", "version", "timestamp"} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("expected key %q in JSON", key)
		}
	}
}
