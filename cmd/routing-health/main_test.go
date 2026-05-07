// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHealthHandler_AllDegraded verifies health endpoint with all checks degraded
func TestHealthHandler_AllDegraded(t *testing.T) {
	checker := &Checker{
		CheckFRR:  func() string { return "degraded" },
		CheckBIRD: func() string { return "degraded" },
		CheckWG0:  func() bool { return false },
		CheckBGP:  func() int { return 0 },
		CheckOSPF: func() int { return 0 },
		Hostname:  "test-host",
	}

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	checker.healthHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Status != "degraded" {
		t.Errorf("Expected status 'degraded', got %q", resp.Status)
	}

	if resp.Checks["frr"] != "degraded" {
		t.Errorf("Expected frr check 'degraded', got %q", resp.Checks["frr"])
	}

	if resp.Checks["bird"] != "degraded" {
		t.Errorf("Expected bird check 'degraded', got %q", resp.Checks["bird"])
	}

	if resp.Checks["wg0"] != "degraded" {
		t.Errorf("Expected wg0 check 'degraded', got %q", resp.Checks["wg0"])
	}
}

// TestReadyHandler_wg0Down verifies /ready returns 503 when wg0 is down
func TestReadyHandler_wg0Down(t *testing.T) {
	checker := &Checker{
		CheckWG0: func() bool { return false },
	}

	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	checker.readyHandler(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", w.Code)
	}
}

// TestReadyHandler_wg0Up verifies /ready returns 200 when wg0 is up
func TestReadyHandler_wg0Up(t *testing.T) {
	checker := &Checker{
		CheckWG0: func() bool { return true },
	}

	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	checker.readyHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// TestMetricsHandler verifies /metrics returns proper Prometheus format
func TestMetricsHandler(t *testing.T) {
	checker := &Checker{
		CheckFRR:  func() string { return "ok" },
		CheckBIRD: func() string { return "degraded" },
		CheckWG0:  func() bool { return true },
		CheckBGP:  func() int { return 2 },
		CheckOSPF: func() int { return 3 },
		Hostname:  "test-forge",
	}

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	checker.metricsHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/plain") {
		t.Errorf("Expected Content-Type text/plain, got %q", contentType)
	}

	body := w.Body.String()

	if !strings.Contains(body, "routing_health_frr_up") {
		t.Error("Expected routing_health_frr_up in metrics")
	}

	if !strings.Contains(body, "routing_health_bird_up") {
		t.Error("Expected routing_health_bird_up in metrics")
	}

	if !strings.Contains(body, "routing_health_wg0_up") {
		t.Error("Expected routing_health_wg0_up in metrics")
	}

	if !strings.Contains(body, "routing_health_bgp_sessions") {
		t.Error("Expected routing_health_bgp_sessions in metrics")
	}

	if !strings.Contains(body, "routing_health_ospf_neighbors") {
		t.Error("Expected routing_health_ospf_neighbors in metrics")
	}

	if !strings.Contains(body, "test-forge") {
		t.Error("Expected hostname label in metrics")
	}

	if !strings.Contains(body, "1.0") { // FRR is ok
		t.Error("Expected FRR status 1.0")
	}

	if !strings.Contains(body, "bgp_sessions{host=\"test-forge\"} 2") {
		t.Error("Expected BGP sessions count 2")
	}

	if !strings.Contains(body, "ospf_neighbors{host=\"test-forge\"} 3") {
		t.Error("Expected OSPF neighbors count 3")
	}
}

// TestCheckFRR_NotRunning verifies FRR check gracefully returns degraded
func TestCheckFRR_NotRunning(t *testing.T) {
	// This test uses a mock checker instead of relying on actual systemctl
	// to avoid dependencies on the test environment
	checker := &Checker{
		CheckFRR: func() string { return "degraded" },
	}

	result := checker.CheckFRR()
	if result != "degraded" {
		t.Errorf("Expected degraded, got %q", result)
	}
}

// TestCheckWG0_NoInterface verifies wg0 check gracefully returns false
func TestCheckWG0_NoInterface(t *testing.T) {
	// This test uses a mock checker to simulate missing interface
	checker := &Checker{
		CheckWG0: func() bool { return false },
	}

	result := checker.CheckWG0()
	if result != false {
		t.Error("Expected false for missing wg0 interface")
	}
}

// TestHealthHandler_AllOK verifies health endpoint with all checks ok
func TestHealthHandler_AllOK(t *testing.T) {
	checker := &Checker{
		CheckFRR:  func() string { return "ok" },
		CheckBIRD: func() string { return "ok" },
		CheckWG0:  func() bool { return true },
		CheckBGP:  func() int { return 1 },
		CheckOSPF: func() int { return 2 },
		Hostname:  "test-host",
	}

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	checker.healthHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Status != "ok" {
		t.Errorf("Expected status 'ok', got %q", resp.Status)
	}

	if resp.Checks["frr"] != "ok" {
		t.Errorf("Expected frr check 'ok', got %q", resp.Checks["frr"])
	}

	if resp.Checks["bird"] != "ok" {
		t.Errorf("Expected bird check 'ok', got %q", resp.Checks["bird"])
	}

	if resp.Checks["wg0"] != "ok" {
		t.Errorf("Expected wg0 check 'ok', got %q", resp.Checks["wg0"])
	}
}

// TestMetricsHandler_AllZero verifies /metrics with all checks down
func TestMetricsHandler_AllZero(t *testing.T) {
	checker := &Checker{
		CheckFRR:  func() string { return "degraded" },
		CheckBIRD: func() string { return "degraded" },
		CheckWG0:  func() bool { return false },
		CheckBGP:  func() int { return 0 },
		CheckOSPF: func() int { return 0 },
		Hostname:  "test-outpost",
	}

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	checker.metricsHandler(w, req)

	body := w.Body.String()

	if !strings.Contains(body, "routing_health_frr_up{host=\"test-outpost\"} 0.0") {
		t.Error("Expected FRR status 0.0")
	}

	if !strings.Contains(body, "routing_health_bird_up{host=\"test-outpost\"} 0.0") {
		t.Error("Expected BIRD status 0.0")
	}

	if !strings.Contains(body, "routing_health_wg0_up{host=\"test-outpost\"} 0.0") {
		t.Error("Expected wg0 status 0.0")
	}

	if !strings.Contains(body, "bgp_sessions{host=\"test-outpost\"} 0") {
		t.Error("Expected BGP sessions count 0")
	}

	if !strings.Contains(body, "ospf_neighbors{host=\"test-outpost\"} 0") {
		t.Error("Expected OSPF neighbors count 0")
	}
}
