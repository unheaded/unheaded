// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"unheaded/cmd/dashboard-backend/internal/events"
	"unheaded/pkg/auth"
	"unheaded/pkg/logger"
)

// testLogger creates a discard logger for testing.
func testLogger() *logger.Logger {
	return logger.New(io.Discard)
}

// newTestServer creates a Server with default config for testing.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	config := DefaultConfig()
	config.WotanAddr = "localhost:19999"
	log := testLogger()
	srv, err := NewServer(config, log)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	return srv
}

// newStartedTestServer creates and starts a Server, registering cleanup.
func newStartedTestServer(t *testing.T) *Server {
	t.Helper()
	srv := newTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutCancel()
		srv.Shutdown(shutCtx)
	})
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	return srv
}

// ---- Config Tests ----

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.ListenAddr == "" {
		t.Error("expected non-empty ListenAddr")
	}
	if cfg.ReadTimeout != 15*time.Second {
		t.Errorf("ReadTimeout = %v, want 15s", cfg.ReadTimeout)
	}
	if cfg.WriteTimeout != 15*time.Second {
		t.Errorf("WriteTimeout = %v, want 15s", cfg.WriteTimeout)
	}
	if cfg.WotanAddr != "localhost:18001" {
		t.Errorf("WotanAddr = %q, want localhost:18001", cfg.WotanAddr)
	}
	if cfg.ServiceName != "dashboard-backend" {
		t.Errorf("ServiceName = %q, want dashboard-backend", cfg.ServiceName)
	}
}

func TestConfigValidate_NilConfig(t *testing.T) {
	var cfg *Config
	err := cfg.Validate()
	if err != ErrNilConfig {
		t.Errorf("expected ErrNilConfig, got %v", err)
	}
}

func TestConfigValidate_MissingWotanAddr(t *testing.T) {
	cfg := &Config{
		ListenAddr: ":8080",
		WotanAddr:  "",
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "wotan address required") {
		t.Errorf("expected wotan address error, got %v", err)
	}
}

func TestConfigValidate_DefaultsFilled(t *testing.T) {
	cfg := &Config{
		WotanAddr: "localhost:18001",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if cfg.ListenAddr == "" {
		t.Error("expected ListenAddr to be set")
	}
	if cfg.ReadTimeout == 0 {
		t.Error("expected ReadTimeout to be set")
	}
	if cfg.WriteTimeout == 0 {
		t.Error("expected WriteTimeout to be set")
	}
	if cfg.ServiceName == "" {
		t.Error("expected ServiceName to be set")
	}
	if cfg.WebSocketConfig == nil {
		t.Error("expected WebSocketConfig to be set")
	}
	if cfg.ScraperConfig == nil {
		t.Error("expected ScraperConfig to be set")
	}
	if cfg.HealthConfig == nil {
		t.Error("expected HealthConfig to be set")
	}
	if cfg.EventsConfig == nil {
		t.Error("expected EventsConfig to be set")
	}
}

// ---- NewServer Tests ----

func TestNewServer_NilLogger(t *testing.T) {
	config := DefaultConfig()
	config.WotanAddr = "localhost:19999"
	srv, err := NewServer(config, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestNewServer_InvalidConfig(t *testing.T) {
	cfg := &Config{WotanAddr: ""}
	_, err := NewServer(cfg, testLogger())
	if err == nil {
		t.Fatal("expected error for empty wotan addr")
	}
}

// ---- Health / Ready Endpoints ----

func TestHandleHealth(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "healthy" {
		t.Errorf("status = %q, want healthy", resp["status"])
	}
}

func TestHandleReady_NotStarted(t *testing.T) {
	srv := newTestServer(t)
	// Server is not started yet, started = false
	srv.started = false
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	srv.handleReady(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestHandleReady_Started(t *testing.T) {
	srv := newStartedTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	srv.handleReady(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "ready" {
		t.Errorf("status = %q, want ready", resp["status"])
	}
}

// ---- API Metrics Endpoint ----

func TestHandleAPIMetrics_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/metrics", nil)
	w := httptest.NewRecorder()
	srv.handleAPIMetrics(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleAPIMetrics_GET(t *testing.T) {
	srv := newStartedTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	w := httptest.NewRecorder()
	srv.handleAPIMetrics(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

// ---- MetricsQuery Endpoint ----

func TestHandleMetricsQuery_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/query", nil)
	w := httptest.NewRecorder()
	srv.handleMetricsQuery(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleMetricsQuery_InvalidJSON(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/metrics/query", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	srv.handleMetricsQuery(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleMetricsQuery_WithDuration(t *testing.T) {
	srv := newStartedTestServer(t)
	body := `{"name":"test_metric","since_duration":"5m"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/metrics/query", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleMetricsQuery(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestHandleMetricsQuery_WithSince(t *testing.T) {
	srv := newStartedTestServer(t)
	body := `{"name":"test_metric","since":"` + time.Now().Add(-1*time.Hour).Format(time.RFC3339) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/metrics/query", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleMetricsQuery(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestHandleMetricsQuery_InvalidDuration(t *testing.T) {
	srv := newTestServer(t)
	body := `{"since_duration":"bad"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/metrics/query", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleMetricsQuery(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleMetricsQuery_InvalidSinceTime(t *testing.T) {
	srv := newTestServer(t)
	body := `{"since":"not-a-time"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/metrics/query", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleMetricsQuery(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleMetricsQuery_DefaultSince(t *testing.T) {
	srv := newStartedTestServer(t)
	body := `{"name":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/metrics/query", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleMetricsQuery(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// ---- Events Endpoints ----

func TestHandleEvents_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", nil)
	w := httptest.NewRecorder()
	srv.handleEvents(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleEvents_GET(t *testing.T) {
	srv := newStartedTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?limit=10&type=system&severity=info&source=test", nil)
	w := httptest.NewRecorder()
	srv.handleEvents(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := resp["events"]; !ok {
		t.Error("expected 'events' key in response")
	}
}

func TestHandleEventsSummary_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events/summary", nil)
	w := httptest.NewRecorder()
	srv.handleEventsSummary(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleEventsSummary_GET(t *testing.T) {
	srv := newStartedTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/summary", nil)
	w := httptest.NewRecorder()
	srv.handleEventsSummary(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// ---- System Health Endpoint ----

func TestHandleSystemHealth_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	srv.handleSystemHealth(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleSystemHealth_GET(t *testing.T) {
	srv := newStartedTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	srv.handleSystemHealth(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// ---- Service Health Endpoint ----

func TestHandleServiceHealth_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/health/wotan", nil)
	w := httptest.NewRecorder()
	srv.handleServiceHealth(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleServiceHealth_EmptyName(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/", nil)
	w := httptest.NewRecorder()
	srv.handleServiceHealth(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleServiceHealth_NotFound(t *testing.T) {
	srv := newStartedTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.handleServiceHealth(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// ---- Flows Endpoint ----

func TestHandleFlows_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/flows", nil)
	w := httptest.NewRecorder()
	srv.handleFlows(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleFlows_NoEBPF(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/flows", nil)
	w := httptest.NewRecorder()
	srv.handleFlows(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

// ---- Stats Endpoint ----

func TestHandleStats_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/stats", nil)
	w := httptest.NewRecorder()
	srv.handleStats(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleStats_GET(t *testing.T) {
	srv := newStartedTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	w := httptest.NewRecorder()
	srv.handleStats(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := resp["server"]; !ok {
		t.Error("expected 'server' key")
	}
	if _, ok := resp["scraper"]; !ok {
		t.Error("expected 'scraper' key")
	}
	if _, ok := resp["health"]; !ok {
		t.Error("expected 'health' key")
	}
}

// ---- Traces Endpoints ----

func TestHandleTraces_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/traces", nil)
	w := httptest.NewRecorder()
	srv.handleTraces(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleTraces_EmptyBuffer(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces", nil)
	w := httptest.NewRecorder()
	srv.handleTraces(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["source"] != "pipeline_buffer" {
		t.Errorf("source = %v, want pipeline_buffer", resp["source"])
	}
}

func TestHandleTraces_WithLimit(t *testing.T) {
	srv := newTestServer(t)
	// Add some trace events
	for i := 0; i < 5; i++ {
		srv.traceBuffer.Add(TraceEvent{TraceID: "t" + string(rune('0'+i))})
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces?limit=3", nil)
	w := httptest.NewRecorder()
	srv.handleTraces(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	traces := resp["traces"].([]interface{})
	if len(traces) != 3 {
		t.Errorf("got %d traces, want 3", len(traces))
	}
	if resp["has_more"] != true {
		t.Error("expected has_more=true")
	}
}

func TestHandleTraces_WithQueryParams(t *testing.T) {
	srv := newTestServer(t)
	now := time.Now()
	url := "/api/v1/traces?service=test&start_time=" + now.Add(-1*time.Hour).Format(time.RFC3339) +
		"&end_time=" + now.Format(time.RFC3339) +
		"&min_duration=100ms&has_error=true"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	srv.handleTraces(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestHandleTraceByID_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/traces/abc", nil)
	w := httptest.NewRecorder()
	srv.handleTraceByID(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleTraceByID_EmptyID(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces/", nil)
	w := httptest.NewRecorder()
	srv.handleTraceByID(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleTraceByID_NoCollector(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces/some-id", nil)
	w := httptest.NewRecorder()
	srv.handleTraceByID(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

// ---- Aggregated Endpoints ----

func TestHandleAggregated_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/aggregated", nil)
	w := httptest.NewRecorder()
	srv.handleAggregated(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleAggregated_GET(t *testing.T) {
	srv := newStartedTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aggregated", nil)
	w := httptest.NewRecorder()
	srv.handleAggregated(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if _, ok := resp["metrics"]; !ok {
		t.Error("expected 'metrics' key")
	}
	if _, ok := resp["health"]; !ok {
		t.Error("expected 'health' key")
	}
	if _, ok := resp["events"]; !ok {
		t.Error("expected 'events' key")
	}
	if _, ok := resp["stream"]; !ok {
		t.Error("expected 'stream' key")
	}
}

func TestHandleAggregatedMetrics_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/aggregated/metrics", nil)
	w := httptest.NewRecorder()
	srv.handleAggregatedMetrics(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleAggregatedMetrics_WithDuration(t *testing.T) {
	srv := newStartedTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aggregated/metrics?since=5m", nil)
	w := httptest.NewRecorder()
	srv.handleAggregatedMetrics(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestHandleAggregatedMetrics_WithRFC3339(t *testing.T) {
	srv := newStartedTestServer(t)
	since := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aggregated/metrics?since="+since, nil)
	w := httptest.NewRecorder()
	srv.handleAggregatedMetrics(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestHandleAggregatedMetrics_DefaultSince(t *testing.T) {
	srv := newStartedTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aggregated/metrics", nil)
	w := httptest.NewRecorder()
	srv.handleAggregatedMetrics(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestHandleAggregatedHealth_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/aggregated/health", nil)
	w := httptest.NewRecorder()
	srv.handleAggregatedHealth(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleAggregatedHealth_GET(t *testing.T) {
	srv := newStartedTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aggregated/health", nil)
	w := httptest.NewRecorder()
	srv.handleAggregatedHealth(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if _, ok := resp["overall"]; !ok {
		t.Error("expected 'overall' key")
	}
	if _, ok := resp["internal"]; !ok {
		t.Error("expected 'internal' key")
	}
}

// ---- Services Endpoints ----

func TestHandleServices_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/services", nil)
	w := httptest.NewRecorder()
	srv.handleServices(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleServices_GET(t *testing.T) {
	srv := newStartedTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/services", nil)
	w := httptest.NewRecorder()
	srv.handleServices(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if _, ok := resp["services"]; !ok {
		t.Error("expected 'services' key")
	}
}

// ---- ServiceByName Endpoints ----

func TestHandleServiceByName_EmptyName(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/", nil)
	w := httptest.NewRecorder()
	srv.handleServiceByName(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleServiceByName_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/services/test-svc", nil)
	w := httptest.NewRecorder()
	srv.handleServiceByName(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleServiceByNameGet_NotFound(t *testing.T) {
	srv := newStartedTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.handleServiceByName(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestHandleServiceByNameDelete_NotFound(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/services/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.handleServiceByName(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// ---- eBPF Endpoints (no ingestor) ----

func TestHandleLatency_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/latency", nil)
	w := httptest.NewRecorder()
	srv.handleLatency(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleLatency_NoIngestor(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/latency", nil)
	w := httptest.NewRecorder()
	srv.handleLatency(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["data"] != nil {
		t.Errorf("expected data=nil, got %v", resp["data"])
	}
}

func TestHandleEBPFStats_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ebpf/stats", nil)
	w := httptest.NewRecorder()
	srv.handleEBPFStats(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleEBPFStats_NoIngestor(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ebpf/stats", nil)
	w := httptest.NewRecorder()
	srv.handleEBPFStats(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["active"] != false {
		t.Errorf("expected active=false, got %v", resp["active"])
	}
}

func TestHandleEBPFEvents_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ebpf/events", nil)
	w := httptest.NewRecorder()
	srv.handleEBPFEvents(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleEBPFEvents_NoIngestor(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ebpf/events", nil)
	w := httptest.NewRecorder()
	srv.handleEBPFEvents(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	count := resp["count"].(float64)
	if count != 0 {
		t.Errorf("expected count=0, got %v", count)
	}
}

// ---- Hosts Endpoint ----

func TestHandleHosts_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/hosts", nil)
	w := httptest.NewRecorder()
	srv.handleHosts(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleHosts_GET(t *testing.T) {
	srv := newTestServer(t)
	// Override hosts to avoid real network calls
	srv.hosts = []KingdomHost{
		{ID: "local", Addr: "127.0.0.1", Type: "test", MetricsURL: ""},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts", nil)
	w := httptest.NewRecorder()
	srv.handleHosts(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	hosts := resp["hosts"].([]interface{})
	if len(hosts) != 1 {
		t.Errorf("expected 1 host, got %d", len(hosts))
	}
}

func TestHandleHosts_RemoteHostOffline(t *testing.T) {
	srv := newTestServer(t)
	// Point to a non-existent server to test offline path
	srv.hosts = []KingdomHost{
		{ID: "remote", Addr: "192.168.99.99", Type: "bare-metal", MetricsURL: "http://192.168.99.99:99999/metrics"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts", nil)
	w := httptest.NewRecorder()
	srv.handleHosts(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	hosts := resp["hosts"].([]interface{})
	host := hosts[0].(map[string]interface{})
	if host["online"] != false {
		t.Errorf("expected offline=false for unreachable host")
	}
}

// ---- Infrastructure Endpoints ----

func TestHandleInfrastructure_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/infrastructure", nil)
	w := httptest.NewRecorder()
	srv.handleInfrastructure(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleInfrastructure_GET(t *testing.T) {
	srv := newStartedTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/infrastructure", nil)
	w := httptest.NewRecorder()
	srv.handleInfrastructure(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["runtime"] != "docker" {
		t.Errorf("expected runtime=docker, got %v", resp["runtime"])
	}
}

func TestHandleInfraContainers_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/infrastructure/containers", nil)
	w := httptest.NewRecorder()
	srv.handleInfraContainers(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleInfraContainers_GET(t *testing.T) {
	srv := newStartedTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/infrastructure/containers", nil)
	w := httptest.NewRecorder()
	srv.handleInfraContainers(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if _, ok := resp["containers"]; !ok {
		t.Error("expected 'containers' key")
	}
}

// ---- Service Config Endpoint ----

func TestHandleServiceConfig_EmptyName(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/config/", nil)
	w := httptest.NewRecorder()
	srv.handleServiceConfig(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleServiceConfig_NotFound(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/config/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.handleServiceConfig(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestHandleServiceConfig_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/services/config/test", nil)
	w := httptest.NewRecorder()
	srv.handleServiceConfig(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

// ---- Service Restart Endpoint ----

func TestHandleServiceRestart_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/restart/test", nil)
	w := httptest.NewRecorder()
	srv.handleServiceRestart(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleServiceRestart_EmptyName(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/services/restart/", nil)
	w := httptest.NewRecorder()
	srv.handleServiceRestart(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleServiceRestart_NotFound(t *testing.T) {
	srv := newStartedTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/services/restart/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.handleServiceRestart(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// ---- Doom Screen MethodNotAllowed ----

func TestHandleDoomScreen_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/doom/screen", nil)
	w := httptest.NewRecorder()
	srv.handleDoomScreen(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

// ---- Doom Input MethodNotAllowed ----

func TestHandleDoomInput_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/doom/input", nil)
	w := httptest.NewRecorder()
	srv.handleDoomInput(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

// ---- Stream Filter ----

func TestMatchesStreamFilter_TypeFilter(t *testing.T) {
	srv := newTestServer(t)

	msg := &StreamMessage{Type: "health", Service: "test"}
	filter := StreamFilter{Types: []string{"metrics", "trace"}}
	if srv.matchesStreamFilter(msg, filter) {
		t.Error("expected no match for type 'health' when filter is [metrics, trace]")
	}

	filter2 := StreamFilter{Types: []string{"health"}}
	if !srv.matchesStreamFilter(msg, filter2) {
		t.Error("expected match for type 'health'")
	}
}

func TestMatchesStreamFilter_ServiceFilter(t *testing.T) {
	srv := newTestServer(t)

	msg := &StreamMessage{Type: "health", Service: "wotan"}
	filter := StreamFilter{Services: []string{"timeguru", "captain"}}
	if srv.matchesStreamFilter(msg, filter) {
		t.Error("expected no match for service 'wotan'")
	}

	filter2 := StreamFilter{Services: []string{"wotan"}}
	if !srv.matchesStreamFilter(msg, filter2) {
		t.Error("expected match for service 'wotan'")
	}
}

func TestMatchesStreamFilter_EmptyFilter(t *testing.T) {
	srv := newTestServer(t)
	msg := &StreamMessage{Type: "anything", Service: "any"}
	filter := StreamFilter{}
	if !srv.matchesStreamFilter(msg, filter) {
		t.Error("expected match for empty filter (pass all)")
	}
}

func TestMatchesStreamFilter_EmptyService(t *testing.T) {
	srv := newTestServer(t)
	// When msg.Service is empty, service filter should not reject
	msg := &StreamMessage{Type: "health", Service: ""}
	filter := StreamFilter{Services: []string{"wotan"}}
	if !srv.matchesStreamFilter(msg, filter) {
		t.Error("expected match when msg.Service is empty (service filter skipped)")
	}
}

// ---- BroadcastToStream ----

func TestBroadcastToStream(t *testing.T) {
	srv := newTestServer(t)

	ch := make(chan *StreamMessage, 10)
	srv.streamSubsMu.Lock()
	srv.streamSubs[ch] = StreamFilter{} // accept all
	srv.streamSubsMu.Unlock()

	msg := &StreamMessage{
		Type:      "health",
		Service:   "test",
		Timestamp: time.Now(),
		Data:      map[string]string{"status": "ok"},
	}
	srv.broadcastToStream(msg)

	select {
	case got := <-ch:
		if got.Type != "health" {
			t.Errorf("got type %q, want health", got.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for stream message")
	}

	// Clean up
	srv.streamSubsMu.Lock()
	delete(srv.streamSubs, ch)
	srv.streamSubsMu.Unlock()
}

// ---- ResolveServiceName ----

func TestResolveServiceName(t *testing.T) {
	srv := newTestServer(t)
	srv.config.ServiceEndpoints = map[string]string{
		"wotan":    "10.10.10.10:18001",
		"timeguru": "10.10.10.20:19000",
	}

	if got := srv.resolveServiceName("10.10.10.10"); got != "wotan" {
		t.Errorf("resolveServiceName(10.10.10.10) = %q, want wotan", got)
	}
	if got := srv.resolveServiceName("10.10.10.99"); got != "10.10.10.99" {
		t.Errorf("resolveServiceName(10.10.10.99) = %q, want 10.10.10.99", got)
	}
}

func TestResolveServiceName_NilEndpoints(t *testing.T) {
	srv := newTestServer(t)
	srv.config.ServiceEndpoints = nil
	if got := srv.resolveServiceName("10.10.10.10"); got != "10.10.10.10" {
		t.Errorf("got %q, want 10.10.10.10", got)
	}
}

func TestResolveServiceByID(t *testing.T) {
	srv := newTestServer(t)
	srv.config.ServiceEndpoints = nil
	if got := srv.resolveServiceByID(0); got != "" {
		t.Errorf("resolveServiceByID(0) = %q, want empty", got)
	}
	if got := srv.resolveServiceByID(1); got != "" {
		t.Errorf("resolveServiceByID(1) with nil endpoints = %q, want empty", got)
	}
}

func TestResolveServiceByID_WithEndpoints(t *testing.T) {
	srv := newTestServer(t)
	srv.config.ServiceEndpoints = map[string]string{
		"svc-a": "10.10.10.10:8000",
	}
	// id=0 is always unknown
	if got := srv.resolveServiceByID(0); got != "" {
		t.Errorf("id=0 should be empty, got %q", got)
	}
	// id=1 should resolve to some service (map iteration order is not guaranteed)
	got := srv.resolveServiceByID(1)
	if got != "svc-a" {
		t.Errorf("id=1 = %q, want svc-a", got)
	}
	// Out of range
	if got := srv.resolveServiceByID(255); got != "" {
		t.Errorf("id=255 should be empty, got %q", got)
	}
}

// ---- TraceBuffer Additional Tests ----

func TestTraceBuffer_RecentEmpty(t *testing.T) {
	buf := NewTraceBuffer(10)
	recent := buf.Recent(5)
	if len(recent) != 0 {
		t.Errorf("expected 0 recent from empty buffer, got %d", len(recent))
	}
}

func TestTraceBuffer_RecentRequestZero(t *testing.T) {
	buf := NewTraceBuffer(10)
	buf.Add(TraceEvent{TraceID: "t1"})
	recent := buf.Recent(0)
	if len(recent) != 0 {
		t.Errorf("expected 0 from Recent(0), got %d", len(recent))
	}
}

// ---- parseTraceTopicEvent Tests ----

func TestParseTraceTopicEvent_PacketTopic(t *testing.T) {
	srv := newTestServer(t)
	now := time.Now()

	te := srv.parseTraceTopicEvent(events.Event{
		Topic:     "traces.packet",
		Timestamp: now,
		Data: map[string]interface{}{
			"trace_id":   "pkt-001",
			"src_ip":     "10.0.0.1",
			"dst_ip":     "10.0.0.2",
			"protocol":   "TCP",
			"packet_len": float64(128),
			"hop_count":  float64(3),
		},
	})
	if te.EventType != "packet" {
		t.Errorf("EventType = %q, want packet", te.EventType)
	}
	if te.TraceID != "pkt-001" {
		t.Errorf("TraceID = %q, want pkt-001", te.TraceID)
	}
	if te.Src != "10.0.0.1" {
		t.Errorf("Src = %q, want 10.0.0.1", te.Src)
	}
	if te.Dst != "10.0.0.2" {
		t.Errorf("Dst = %q, want 10.0.0.2", te.Dst)
	}
	if te.Protocol != "TCP" {
		t.Errorf("Protocol = %q, want TCP", te.Protocol)
	}
	if te.PacketLen != 128 {
		t.Errorf("PacketLen = %d, want 128", te.PacketLen)
	}
	if te.HopCount != 3 {
		t.Errorf("HopCount = %d, want 3", te.HopCount)
	}
}

func TestParseTraceTopicEvent_FlowTopic(t *testing.T) {
	srv := newTestServer(t)
	te := srv.parseTraceTopicEvent(events.Event{
		Topic:     "traces.flow",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"trace_id": "flow-001",
			"state":    "ESTABLISHED",
			"rtt_ms":   float64(1.5),
		},
	})
	if te.EventType != "flow" {
		t.Errorf("EventType = %q, want flow", te.EventType)
	}
	if te.FlowState != "ESTABLISHED" {
		t.Errorf("FlowState = %q, want ESTABLISHED", te.FlowState)
	}
	if te.RTTMs != 1.5 {
		t.Errorf("RTTMs = %f, want 1.5", te.RTTMs)
	}
}

func TestParseTraceTopicEvent_LatencyTopic(t *testing.T) {
	srv := newTestServer(t)
	te := srv.parseTraceTopicEvent(events.Event{
		Topic:     "traces.latency",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"rtt_ns": float64(500000), // 0.5ms in ns
		},
	})
	if te.EventType != "latency" {
		t.Errorf("EventType = %q, want latency", te.EventType)
	}
	if te.RTTMs < 0.49 || te.RTTMs > 0.51 {
		t.Errorf("RTTMs = %f, want ~0.5", te.RTTMs)
	}
}

func TestParseTraceTopicEvent_CorrelatedWithFlowKey(t *testing.T) {
	srv := newTestServer(t)
	te := srv.parseTraceTopicEvent(events.Event{
		Topic:     "traces.correlated",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"trace_id":   "corr-001",
			"flow_state": "SYN_SENT",
			"flow_key": map[string]interface{}{
				"src_addr": "10.0.0.1",
				"dst_addr": "10.0.0.2",
				"src_port": float64(8080),
				"dst_port": float64(9090),
				"protocol": float64(6), // TCP
			},
		},
	})
	if te.EventType != "correlated" {
		t.Errorf("EventType = %q, want correlated", te.EventType)
	}
	if te.Src != "10.0.0.1:8080" {
		t.Errorf("Src = %q, want 10.0.0.1:8080", te.Src)
	}
	if te.Dst != "10.0.0.2:9090" {
		t.Errorf("Dst = %q, want 10.0.0.2:9090", te.Dst)
	}
	if te.Protocol != "TCP" {
		t.Errorf("Protocol = %q, want TCP", te.Protocol)
	}
	if te.FlowState != "SYN_SENT" {
		t.Errorf("FlowState = %q, want SYN_SENT", te.FlowState)
	}
}

func TestParseTraceTopicEvent_UDPProtocol(t *testing.T) {
	srv := newTestServer(t)
	te := srv.parseTraceTopicEvent(events.Event{
		Topic:     "traces.packet",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"flow_key": map[string]interface{}{
				"src_addr": "10.0.0.1",
				"dst_addr": "10.0.0.2",
				"protocol": float64(17), // UDP
			},
		},
	})
	if te.Protocol != "UDP" {
		t.Errorf("Protocol = %q, want UDP", te.Protocol)
	}
}

func TestParseTraceTopicEvent_UnknownProtocol(t *testing.T) {
	srv := newTestServer(t)
	te := srv.parseTraceTopicEvent(events.Event{
		Topic:     "traces.packet",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"flow_key": map[string]interface{}{
				"src_addr": "10.0.0.1",
				"dst_addr": "10.0.0.2",
				"protocol": float64(132), // SCTP
			},
		},
	})
	if !strings.Contains(te.Protocol, "proto(132)") {
		t.Errorf("Protocol = %q, want proto(132)", te.Protocol)
	}
}

func TestParseTraceTopicEvent_UnknownTopic(t *testing.T) {
	srv := newTestServer(t)
	te := srv.parseTraceTopicEvent(events.Event{
		Topic:     "traces.unknown",
		Timestamp: time.Now(),
		Data:      nil,
	})
	if te.EventType != "unknown" {
		t.Errorf("EventType = %q, want unknown", te.EventType)
	}
	// Should generate a trace ID
	if te.TraceID == "" {
		t.Error("expected generated TraceID")
	}
}

func TestParseTraceTopicEvent_AltSrcDstFields(t *testing.T) {
	srv := newTestServer(t)
	te := srv.parseTraceTopicEvent(events.Event{
		Topic:     "traces.packet",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"src": "1.2.3.4",
			"dst": "5.6.7.8",
		},
	})
	if te.Src != "1.2.3.4" {
		t.Errorf("Src = %q, want 1.2.3.4", te.Src)
	}
	if te.Dst != "5.6.7.8" {
		t.Errorf("Dst = %q, want 5.6.7.8", te.Dst)
	}
}

// ---- parsePrometheusText Tests ----

func TestParsePrometheusText(t *testing.T) {
	text := `# HELP go_goroutines Number of goroutines
# TYPE go_goroutines gauge
go_goroutines 42
process_start_time_seconds 1.7e9
go_memstats_sys_bytes{instance="a"} 12345678
`
	result := parsePrometheusText(text)
	if result["go_goroutines"] != 42 {
		t.Errorf("go_goroutines = %v, want 42", result["go_goroutines"])
	}
	if result["process_start_time_seconds"] != 1.7e9 {
		t.Errorf("process_start_time_seconds = %v, want 1.7e9", result["process_start_time_seconds"])
	}
	if result["go_memstats_sys_bytes"] != 12345678 {
		t.Errorf("go_memstats_sys_bytes = %v, want 12345678", result["go_memstats_sys_bytes"])
	}
}

func TestParsePrometheusText_EmptyAndComments(t *testing.T) {
	text := `# Just a comment

# Another comment
`
	result := parsePrometheusText(text)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d entries", len(result))
	}
}

func TestParsePrometheusText_DuplicateMetric(t *testing.T) {
	text := `my_metric 100
my_metric 200
`
	result := parsePrometheusText(text)
	if result["my_metric"] != 100 {
		t.Errorf("expected first value 100, got %v", result["my_metric"])
	}
}

func TestParsePrometheusText_MalformedLabel(t *testing.T) {
	// Missing closing brace
	text := `bad_metric{label="value" 42
`
	result := parsePrometheusText(text)
	if _, ok := result["bad_metric"]; ok {
		t.Error("expected bad_metric to be skipped due to missing '}'")
	}
}

func TestParsePrometheusText_BadValue(t *testing.T) {
	text := `bad_value_metric not_a_number
`
	result := parsePrometheusText(text)
	if _, ok := result["bad_value_metric"]; ok {
		t.Error("expected bad value to be skipped")
	}
}

// ---- Getter Methods ----

func TestGetterMethods(t *testing.T) {
	srv := newTestServer(t)

	if srv.GetWebSocketServer() == nil {
		t.Error("GetWebSocketServer returned nil")
	}
	if srv.GetScraper() == nil {
		t.Error("GetScraper returned nil")
	}
	if srv.GetHealthMonitor() == nil {
		t.Error("GetHealthMonitor returned nil")
	}
	if srv.GetEventStreamer() == nil {
		t.Error("GetEventStreamer returned nil")
	}
	if srv.GetMetricsAggregator() == nil {
		t.Error("GetMetricsAggregator returned nil")
	}
	if srv.GetTraceBuffer() == nil {
		t.Error("GetTraceBuffer returned nil")
	}
	if srv.GetConfigLoader() == nil {
		t.Error("GetConfigLoader returned nil")
	}
	// Optional fields can be nil
	if srv.GetTraceCollector() != nil {
		t.Error("expected nil TraceCollector")
	}
	if srv.GetHealthAggregator() != nil {
		t.Error("expected nil HealthAggregator")
	}
}

// ---- Setter Methods ----

func TestSetTraceCollector(t *testing.T) {
	srv := newTestServer(t)
	srv.SetTraceCollector(nil)
	if srv.GetTraceCollector() != nil {
		t.Error("expected nil after setting nil")
	}
}

func TestSetHealthAggregator(t *testing.T) {
	srv := newTestServer(t)
	srv.SetHealthAggregator(nil)
	if srv.GetHealthAggregator() != nil {
		t.Error("expected nil after setting nil")
	}
}

func TestRegisterHealthCheck_NoAggregator(t *testing.T) {
	srv := newTestServer(t)
	err := srv.RegisterHealthCheck("dummy")
	if err == nil {
		t.Error("expected error when aggregator is nil")
	}
	if !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---- RecordMetric ----

func TestRecordMetric(t *testing.T) {
	srv := newTestServer(t)
	err := srv.RecordMetric("test_metric", 42.0, map[string]string{"env": "test"})
	if err != nil {
		t.Errorf("RecordMetric: %v", err)
	}
}

// ---- Server Lifecycle ----

func TestServerStartTwice(t *testing.T) {
	srv := newStartedTestServer(t)
	err := srv.Start(context.Background())
	if err == nil {
		t.Error("expected error on second Start")
	}
	if !strings.Contains(err.Error(), "already started") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestServerShutdown(t *testing.T) {
	config := DefaultConfig()
	config.WotanAddr = "localhost:19999"
	srv, err := NewServer(config, testLogger())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	cancel()

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		t.Errorf("Shutdown: %v", err)
	}

	// Double shutdown should be safe (shutdownOnce)
	if err := srv.Shutdown(shutCtx); err != nil {
		t.Errorf("second Shutdown: %v", err)
	}
}

// ---- DoomState snapshot concurrency ----

func TestDoomState_SnapshotConcurrency(t *testing.T) {
	ds := &DoomState{}

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			regs := [16]uint32{}
			regs[0] = uint32(i)
			ds.UpdateCPU(uint32(i), regs, 0, false, false, uint64(i), 0, 0)
		}
		close(done)
	}()

	for i := 0; i < 100; i++ {
		snap := ds.snapshot()
		_ = snap.PC // read
	}
	<-done
}

// ---- buildDashboardAuthenticators ----

func TestBuildDashboardAuthenticators_Empty(t *testing.T) {
	// No keys configured => empty authenticators
	auths := buildDashboardAuthenticators(auth.ServiceAuthConfig{})
	if len(auths) != 0 {
		t.Errorf("expected 0 authenticators, got %d", len(auths))
	}
}

func TestBuildDashboardAuthenticators_WithJWT(t *testing.T) {
	cfg := auth.ServiceAuthConfig{
		JWTSigningKey: []byte("test-secret-key-for-jwt"),
		Issuer:        "test-issuer",
		Audience:      "test-audience",
		ServiceName:   "dashboard-backend",
	}
	auths := buildDashboardAuthenticators(cfg)
	// Should have 2 JWT authenticators (one for user, one for service)
	if len(auths) != 2 {
		t.Errorf("expected 2 authenticators with JWT, got %d", len(auths))
	}
}

func TestBuildDashboardAuthenticators_WithAPIKeys(t *testing.T) {
	cfg := auth.ServiceAuthConfig{
		APIKeys: []string{"key1", "key2"},
	}
	auths := buildDashboardAuthenticators(cfg)
	if len(auths) != 1 {
		t.Errorf("expected 1 authenticator with API keys, got %d", len(auths))
	}
}

// ---- Static file handling ----

func TestHandleStaticIndex_NotFound(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/nonexistent-page", nil)
	w := httptest.NewRecorder()
	srv.handleStaticIndex(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}
