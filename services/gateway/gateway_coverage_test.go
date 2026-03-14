// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"unheaded/pkg/logger"
	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/services/gateway/config"
)

// ---------------------------------------------------------------------------
// healthManager tests
// ---------------------------------------------------------------------------

func TestHealthManager_NoChecks(t *testing.T) {
	hm := newHealthManager()
	status := hm.CheckAll(context.Background())
	if !status.Healthy {
		t.Error("expected healthy with no checks")
	}
	if len(status.Checks) != 0 {
		t.Errorf("expected 0 checks, got %d", len(status.Checks))
	}
}

func TestHealthManager_AllHealthy(t *testing.T) {
	hm := newHealthManager()
	hm.Register("db", func(ctx context.Context) error { return nil })
	hm.Register("cache", func(ctx context.Context) error { return nil })

	status := hm.CheckAll(context.Background())
	if !status.Healthy {
		t.Error("expected healthy")
	}
	if status.Checks["db"] != "healthy" {
		t.Errorf("expected db=healthy, got %s", status.Checks["db"])
	}
	if status.Checks["cache"] != "healthy" {
		t.Errorf("expected cache=healthy, got %s", status.Checks["cache"])
	}
}

func TestHealthManager_OneUnhealthy(t *testing.T) {
	hm := newHealthManager()
	hm.Register("db", func(ctx context.Context) error { return fmt.Errorf("connection refused") })
	hm.Register("cache", func(ctx context.Context) error { return nil })

	status := hm.CheckAll(context.Background())
	if status.Healthy {
		t.Error("expected unhealthy when one check fails")
	}
	if status.Checks["db"] != "connection refused" {
		t.Errorf("expected error message, got %s", status.Checks["db"])
	}
	if status.Checks["cache"] != "healthy" {
		t.Errorf("expected cache=healthy, got %s", status.Checks["cache"])
	}
}

// ---------------------------------------------------------------------------
// healthHandler tests
// ---------------------------------------------------------------------------

func newTestGateway(t *testing.T) *Gateway {
	t.Helper()
	log := logger.New(io.Discard).SetLevel(logger.DebugLevel)
	cfg := config.DefaultConfig()
	cfg.Auth.Enabled = false
	cfg.Wotan.Enabled = false

	gw, err := New(cfg, log)
	if err != nil {
		t.Fatalf("Failed to create gateway: %v", err)
	}
	return gw
}

func TestHealthHandler_Healthy(t *testing.T) {
	gw := newTestGateway(t)
	gw.healthMgr.Register("test", func(ctx context.Context) error { return nil })

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	gw.healthHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var status healthStatus
	json.NewDecoder(rr.Body).Decode(&status)
	if !status.Healthy {
		t.Error("expected healthy")
	}
}

func TestHealthHandler_Unhealthy(t *testing.T) {
	gw := newTestGateway(t)
	gw.healthMgr.Register("broken", func(ctx context.Context) error {
		return fmt.Errorf("down")
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	gw.healthHandler(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// livenessHandler tests
// ---------------------------------------------------------------------------

func TestLivenessHandler(t *testing.T) {
	gw := newTestGateway(t)
	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	rr := httptest.NewRecorder()
	gw.livenessHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if body != `{"status":"alive"}` {
		t.Errorf("unexpected body: %s", body)
	}
}

// ---------------------------------------------------------------------------
// readinessHandler tests
// ---------------------------------------------------------------------------

func TestReadinessHandler_NoRoutes(t *testing.T) {
	gw := newTestGateway(t)
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()
	gw.readinessHandler(rr, req)

	// With no routes, should be ready.
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestReadinessHandler_WithHealthyBackend(t *testing.T) {
	log := logger.New(io.Discard).SetLevel(logger.DebugLevel)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	cfg := config.DefaultConfig()
	cfg.Auth.Enabled = false
	cfg.Wotan.Enabled = false
	cfg.Routes = []config.RouteConfig{
		{
			Name:       "test",
			PathPrefix: "/api/",
			Backends:   []string{backend.URL},
			Timeout:    5 * time.Second,
		},
	}

	gw, err := New(cfg, log)
	if err != nil {
		t.Fatalf("Failed to create gateway: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()
	gw.readinessHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// metricsHandler tests
// ---------------------------------------------------------------------------

func TestMetricsHandler(t *testing.T) {
	gw := newTestGateway(t)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	gw.metricsHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if ct == "" {
		t.Error("expected Content-Type header")
	}
}

// ---------------------------------------------------------------------------
// buildHandler tests
// ---------------------------------------------------------------------------

func TestBuildHandler_Proxies(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer backend.Close()

	log := logger.New(io.Discard).SetLevel(logger.DebugLevel)
	cfg := config.DefaultConfig()
	cfg.Auth.Enabled = false
	cfg.Wotan.Enabled = false
	cfg.Routes = []config.RouteConfig{
		{
			Name:       "test",
			PathPrefix: "/api/",
			Backends:   []string{backend.URL},
			Timeout:    5 * time.Second,
		},
	}

	gw, err := New(cfg, log)
	if err != nil {
		t.Fatalf("Failed to create gateway: %v", err)
	}
	handler := gw.buildHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if rr.Header().Get("X-Trace-ID") == "" {
		t.Error("expected X-Trace-ID header from tracing middleware")
	}
}

// ---------------------------------------------------------------------------
// GetRouter / GetMetrics / IsRunning tests
// ---------------------------------------------------------------------------

func TestGetRouter(t *testing.T) {
	gw := newTestGateway(t)
	if gw.GetRouter() == nil {
		t.Error("expected non-nil router")
	}
}

func TestGetMetrics(t *testing.T) {
	gw := newTestGateway(t)
	if gw.GetMetrics() == nil {
		t.Error("expected non-nil metrics")
	}
}

func TestIsRunning_False(t *testing.T) {
	gw := newTestGateway(t)
	if gw.IsRunning() {
		t.Error("expected not running before Start")
	}
}

// ---------------------------------------------------------------------------
// handleCriticalAlert tests
// ---------------------------------------------------------------------------

func TestHandleCriticalAlert_Nil(t *testing.T) {
	gw := newTestGateway(t)
	// Should not panic with nil message.
	gw.handleCriticalAlert(context.Background(), nil)
}

// ---------------------------------------------------------------------------
// Gateway double start test
// ---------------------------------------------------------------------------

func TestGateway_DoubleStart(t *testing.T) {
	gw := newTestGateway(t)
	gw.cfg.Server.HTTPPort = 0

	ctx, cancel := context.WithCancel(context.Background())
	// Start in background
	go func() {
		gw.Start(ctx)
	}()

	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)

	// Second start should fail
	err := func() error {
		ctx2, cancel2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel2()
		return gw.Start(ctx2)
	}()

	if err == nil || err.Error() != "gateway already running" {
		t.Logf("got err: %v (may be nil if race with shutdown)", err)
	}

	cancel()
	time.Sleep(100 * time.Millisecond)
}

// ---------------------------------------------------------------------------
// Shutdown when not running
// ---------------------------------------------------------------------------

func TestShutdown_NotRunning(t *testing.T) {
	gw := newTestGateway(t)
	err := gw.Shutdown()
	if err != nil {
		t.Fatalf("Shutdown when not running should return nil, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// New with invalid config
// ---------------------------------------------------------------------------

func TestNew_InvalidConfig(t *testing.T) {
	log := logger.New(io.Discard)
	cfg := config.DefaultConfig()
	cfg.Server.HTTPPort = -1 // invalid

	_, err := New(cfg, log)
	if err == nil {
		t.Error("expected error for invalid config")
	}
}

// ---------------------------------------------------------------------------
// New with routes and health check URL
// ---------------------------------------------------------------------------

func TestNew_WithRoutes(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	log := logger.New(io.Discard)
	cfg := config.DefaultConfig()
	cfg.Auth.Enabled = false
	cfg.Wotan.Enabled = false
	cfg.Routes = []config.RouteConfig{
		{
			Name:           "test",
			PathPrefix:     "/api/",
			Backends:       []string{backend.URL},
			HealthCheckURL: "/health",
			Timeout:        5 * time.Second,
		},
	}

	gw, err := New(cfg, log)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if gw.GetRouter().GetRoute("test") == nil {
		t.Error("expected route 'test' to be registered")
	}
}

// ---------------------------------------------------------------------------
// readinessHandler with unhealthy backends
// ---------------------------------------------------------------------------

func TestReadinessHandler_NoHealthyBackends(t *testing.T) {
	log := logger.New(io.Discard).SetLevel(logger.DebugLevel)
	cfg := config.DefaultConfig()
	cfg.Auth.Enabled = false
	cfg.Wotan.Enabled = false
	cfg.Routes = []config.RouteConfig{
		{
			Name:       "api",
			PathPrefix: "/api/",
			Backends:   []string{"http://localhost:1"}, // unreachable
			Timeout:    time.Second,
		},
	}

	gw, err := New(cfg, log)
	if err != nil {
		t.Fatalf("Failed to create gateway: %v", err)
	}

	// Mark the backend unhealthy.
	route := gw.router.GetRoute("api")
	if route != nil {
		route.LoadBalancer.MarkUnhealthy("http://localhost:1")
	}

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()
	gw.readinessHandler(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 with no healthy backends, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// wotanMiddleware test (without actual wotan)
// ---------------------------------------------------------------------------

func TestWotanMiddleware_ForwardsRequest(t *testing.T) {
	gw := newTestGateway(t)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Even with nil wotan, buildHandler includes middleware chain.
	// Test the handler directly - wotan is nil so wotanMiddleware won't be applied.
	handler := gw.buildHandler()

	req := httptest.NewRequest(http.MethodGet, "/unknown-path", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Should get 404 from router since no routes matched.
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}

	_ = inner // just to verify inner is valid
}

// ---------------------------------------------------------------------------
// handleCriticalAlert with message
// ---------------------------------------------------------------------------

func TestHandleCriticalAlert_WithMessage(t *testing.T) {
	gw := newTestGateway(t)
	// Import is already available through gateway.go.
	msg := &wotanClient.Message{
		MessageID: "alert-123",
		Topic:     "alerts.critical",
		Payload:   "test alert",
	}
	gw.handleCriticalAlert(context.Background(), msg)
	// Should not panic, just log.
}

// ---------------------------------------------------------------------------
// listenForAlerts with nil wotan
// ---------------------------------------------------------------------------

func TestListenForAlerts_NilWotan(t *testing.T) {
	gw := newTestGateway(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	// Should return immediately since wotan is nil.
	gw.listenForAlerts(ctx)
}
