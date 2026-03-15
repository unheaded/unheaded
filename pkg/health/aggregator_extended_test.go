// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// --- DefaultConfig tests ---

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig returned nil")
	}
	if cfg.DefaultTimeout != 5*time.Second {
		t.Fatalf("expected 5s timeout, got %s", cfg.DefaultTimeout)
	}
	if cfg.DefaultInterval != 30*time.Second {
		t.Fatalf("expected 30s interval, got %s", cfg.DefaultInterval)
	}
	if cfg.HistorySize != 1000 {
		t.Fatalf("expected 1000 history size, got %d", cfg.HistorySize)
	}
	if cfg.CircuitBreakerFailures != 3 {
		t.Fatalf("expected 3 CB failures, got %d", cfg.CircuitBreakerFailures)
	}
	if cfg.CircuitBreakerSuccesses != 2 {
		t.Fatalf("expected 2 CB successes, got %d", cfg.CircuitBreakerSuccesses)
	}
	if cfg.CircuitBreakerResetTimeout != 30*time.Second {
		t.Fatalf("expected 30s CB reset timeout, got %s", cfg.CircuitBreakerResetTimeout)
	}
	if cfg.WotanTopic != "health.events" {
		t.Fatalf("expected health.events topic, got %s", cfg.WotanTopic)
	}
	if cfg.ServiceName != "health-aggregator" {
		t.Fatalf("expected health-aggregator, got %s", cfg.ServiceName)
	}
}

// --- NewAggregator tests ---

func TestNewAggregatorNilConfig(t *testing.T) {
	agg := NewAggregator(nil, nil)
	if agg == nil {
		t.Fatal("NewAggregator returned nil")
	}
	if agg.config.DefaultTimeout != 5*time.Second {
		t.Fatal("expected default config to be applied")
	}
}

func TestNewAggregatorCustomConfig(t *testing.T) {
	cfg := &AggregatorConfig{
		DefaultTimeout:  10 * time.Second,
		DefaultInterval: 60 * time.Second,
		HistorySize:     500,
		ServiceName:     "test-agg",
	}
	agg := NewAggregator(cfg, nil)
	if agg.config.DefaultTimeout != 10*time.Second {
		t.Fatal("custom config not applied")
	}
	if agg.config.ServiceName != "test-agg" {
		t.Fatal("expected test-agg service name")
	}
}

// --- RegisterCheck tests ---

func TestRegisterCheckSuccess(t *testing.T) {
	agg := NewAggregator(nil, nil)
	err := agg.RegisterCheck(HealthCheck{
		Name:   "svc-1",
		Type:   CheckTypeHTTP,
		Target: "http://localhost:8080/health",
	})
	if err != nil {
		t.Fatalf("RegisterCheck: %v", err)
	}

	checks := agg.GetChecks()
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(checks))
	}
	if checks[0].Name != "svc-1" {
		t.Fatalf("expected svc-1, got %s", checks[0].Name)
	}
}

func TestRegisterCheckAppliesDefaults(t *testing.T) {
	agg := NewAggregator(nil, nil)
	err := agg.RegisterCheck(HealthCheck{
		Name:   "svc-defaults",
		Type:   CheckTypeHTTP,
		Target: "http://localhost/health",
	})
	if err != nil {
		t.Fatalf("RegisterCheck: %v", err)
	}

	checks := agg.GetChecks()
	c := checks[0]
	if c.Timeout != 5*time.Second {
		t.Fatalf("expected default timeout 5s, got %s", c.Timeout)
	}
	if c.Interval != 30*time.Second {
		t.Fatalf("expected default interval 30s, got %s", c.Interval)
	}
	if c.SuccessThreshold != 1 {
		t.Fatalf("expected default success threshold 1, got %d", c.SuccessThreshold)
	}
	if c.FailureThreshold != 3 {
		t.Fatalf("expected default failure threshold 3, got %d", c.FailureThreshold)
	}
	if c.HTTPConfig == nil {
		t.Fatal("expected HTTP config to be set")
	}
	if c.HTTPConfig.Method != "GET" {
		t.Fatalf("expected GET method, got %s", c.HTTPConfig.Method)
	}
}

func TestRegisterCheckGRPCDefaults(t *testing.T) {
	agg := NewAggregator(nil, nil)
	err := agg.RegisterCheck(HealthCheck{
		Name:   "grpc-svc",
		Type:   CheckTypeGRPC,
		Target: "localhost:9090",
	})
	if err != nil {
		t.Fatalf("RegisterCheck: %v", err)
	}
	checks := agg.GetChecks()
	if checks[0].GRPCConfig == nil {
		t.Fatal("expected gRPC config to be set")
	}
}

func TestRegisterCheckExecDefaults(t *testing.T) {
	agg := NewAggregator(nil, nil)
	err := agg.RegisterCheck(HealthCheck{
		Name:   "exec-svc",
		Type:   CheckTypeExec,
		Target: "/usr/bin/check",
	})
	if err != nil {
		t.Fatalf("RegisterCheck: %v", err)
	}
	checks := agg.GetChecks()
	if checks[0].ExecConfig == nil {
		t.Fatal("expected exec config to be set")
	}
	if checks[0].ExecConfig.ExpectedExitCode != 0 {
		t.Fatal("expected default exit code 0")
	}
}

func TestRegisterCheckEmptyName(t *testing.T) {
	agg := NewAggregator(nil, nil)
	err := agg.RegisterCheck(HealthCheck{
		Name: "",
		Type: CheckTypeHTTP,
	})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestRegisterCheckDuplicate(t *testing.T) {
	agg := NewAggregator(nil, nil)
	agg.RegisterCheck(HealthCheck{Name: "dup", Type: CheckTypeHTTP, Target: "http://a"})
	err := agg.RegisterCheck(HealthCheck{Name: "dup", Type: CheckTypeHTTP, Target: "http://b"})
	if !errors.Is(err, ErrCheckAlreadyExists) {
		t.Fatalf("expected ErrCheckAlreadyExists, got %v", err)
	}
}

func TestRegisterCheckInitializesResult(t *testing.T) {
	agg := NewAggregator(nil, nil)
	agg.RegisterCheck(HealthCheck{Name: "init-svc", Type: CheckTypeTCP, Target: "localhost:80"})

	result, err := agg.GetHealth("init-svc")
	if err != nil {
		t.Fatalf("GetHealth: %v", err)
	}
	if result.Status != StatusUnknown {
		t.Fatalf("expected unknown status, got %s", result.Status)
	}
}

// --- UnregisterCheck tests ---

func TestUnregisterCheck(t *testing.T) {
	agg := NewAggregator(nil, nil)
	agg.RegisterCheck(HealthCheck{Name: "unreg", Type: CheckTypeHTTP, Target: "http://a"})

	err := agg.UnregisterCheck("unreg")
	if err != nil {
		t.Fatalf("UnregisterCheck: %v", err)
	}

	_, err = agg.GetHealth("unreg")
	if !errors.Is(err, ErrCheckNotFound) {
		t.Fatalf("expected ErrCheckNotFound after unregister, got %v", err)
	}

	checks := agg.GetChecks()
	if len(checks) != 0 {
		t.Fatalf("expected 0 checks after unregister, got %d", len(checks))
	}
}

func TestUnregisterCheckNotFound(t *testing.T) {
	agg := NewAggregator(nil, nil)
	err := agg.UnregisterCheck("nonexistent")
	if !errors.Is(err, ErrCheckNotFound) {
		t.Fatalf("expected ErrCheckNotFound, got %v", err)
	}
}

// --- GetHealth tests ---

func TestGetHealthNotFound(t *testing.T) {
	agg := NewAggregator(nil, nil)
	_, err := agg.GetHealth("missing")
	if !errors.Is(err, ErrCheckNotFound) {
		t.Fatalf("expected ErrCheckNotFound, got %v", err)
	}
}

// --- GetSystemHealth tests ---

func TestGetSystemHealthNoChecks(t *testing.T) {
	agg := NewAggregator(nil, nil)
	sh := agg.GetSystemHealth()
	if sh.Status != StatusUnknown {
		t.Fatalf("expected unknown status with no checks, got %s", sh.Status)
	}
	if sh.TotalCount != 0 {
		t.Fatalf("expected 0 total, got %d", sh.TotalCount)
	}
}

func TestGetSystemHealthAllHealthy(t *testing.T) {
	agg := NewAggregator(nil, nil)
	agg.RegisterCheck(HealthCheck{Name: "a", Type: CheckTypeHTTP, Target: "http://a"})
	agg.RegisterCheck(HealthCheck{Name: "b", Type: CheckTypeHTTP, Target: "http://b"})

	// Manually set results to healthy.
	agg.resultsMu.Lock()
	agg.results["a"] = &HealthResult{Name: "a", Status: StatusHealthy}
	agg.results["b"] = &HealthResult{Name: "b", Status: StatusHealthy}
	agg.resultsMu.Unlock()

	sh := agg.GetSystemHealth()
	if sh.Status != StatusHealthy {
		t.Fatalf("expected healthy, got %s", sh.Status)
	}
	if sh.HealthyCount != 2 {
		t.Fatalf("expected 2 healthy, got %d", sh.HealthyCount)
	}
	if sh.TotalCount != 2 {
		t.Fatalf("expected 2 total, got %d", sh.TotalCount)
	}
}

func TestGetSystemHealthWithUnhealthy(t *testing.T) {
	agg := NewAggregator(nil, nil)
	agg.RegisterCheck(HealthCheck{Name: "a", Type: CheckTypeHTTP, Target: "http://a"})
	agg.RegisterCheck(HealthCheck{Name: "b", Type: CheckTypeHTTP, Target: "http://b"})

	agg.resultsMu.Lock()
	agg.results["a"] = &HealthResult{Name: "a", Status: StatusHealthy}
	agg.results["b"] = &HealthResult{Name: "b", Status: StatusUnhealthy}
	agg.resultsMu.Unlock()

	sh := agg.GetSystemHealth()
	if sh.Status != StatusUnhealthy {
		t.Fatalf("expected unhealthy, got %s", sh.Status)
	}
	if sh.UnhealthyCount != 1 {
		t.Fatalf("expected 1 unhealthy, got %d", sh.UnhealthyCount)
	}
}

func TestGetSystemHealthWithDegraded(t *testing.T) {
	agg := NewAggregator(nil, nil)
	agg.RegisterCheck(HealthCheck{Name: "a", Type: CheckTypeHTTP, Target: "http://a"})
	agg.RegisterCheck(HealthCheck{Name: "b", Type: CheckTypeHTTP, Target: "http://b"})

	agg.resultsMu.Lock()
	agg.results["a"] = &HealthResult{Name: "a", Status: StatusHealthy}
	agg.results["b"] = &HealthResult{Name: "b", Status: StatusDegraded}
	agg.resultsMu.Unlock()

	sh := agg.GetSystemHealth()
	if sh.Status != StatusDegraded {
		t.Fatalf("expected degraded, got %s", sh.Status)
	}
	if sh.DegradedCount != 1 {
		t.Fatalf("expected 1 degraded, got %d", sh.DegradedCount)
	}
}

// --- GetHealthHistory tests ---

func TestGetHealthHistoryNotFound(t *testing.T) {
	agg := NewAggregator(nil, nil)
	_, err := agg.GetHealthHistory("missing", time.Hour)
	if !errors.Is(err, ErrCheckNotFound) {
		t.Fatalf("expected ErrCheckNotFound, got %v", err)
	}
}

func TestGetHealthHistoryReturnsResults(t *testing.T) {
	agg := NewAggregator(nil, nil)
	agg.RegisterCheck(HealthCheck{Name: "hist-svc", Type: CheckTypeHTTP, Target: "http://a"})

	// Add history entries.
	agg.historyMu.Lock()
	hist := agg.history["hist-svc"]
	hist.Add(HealthResult{Name: "hist-svc", Status: StatusHealthy, Timestamp: time.Now()})
	hist.Add(HealthResult{Name: "hist-svc", Status: StatusDegraded, Timestamp: time.Now()})
	agg.historyMu.Unlock()

	results, err := agg.GetHealthHistory("hist-svc", time.Hour)
	if err != nil {
		t.Fatalf("GetHealthHistory: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

// --- GetCircuitBreakerState tests ---

func TestGetCircuitBreakerState(t *testing.T) {
	agg := NewAggregator(nil, nil)
	agg.RegisterCheck(HealthCheck{Name: "cb-svc", Type: CheckTypeHTTP, Target: "http://a"})

	state, err := agg.GetCircuitBreakerState("cb-svc")
	if err != nil {
		t.Fatalf("GetCircuitBreakerState: %v", err)
	}
	if state != "closed" {
		t.Fatalf("expected closed, got %s", state)
	}
}

func TestGetCircuitBreakerStateNotFound(t *testing.T) {
	agg := NewAggregator(nil, nil)
	_, err := agg.GetCircuitBreakerState("missing")
	if !errors.Is(err, ErrCheckNotFound) {
		t.Fatalf("expected ErrCheckNotFound, got %v", err)
	}
}

// --- AddListener tests ---

func TestAddListenerReceivesEvents(t *testing.T) {
	agg := NewAggregator(nil, nil)

	var mu sync.Mutex
	var events []HealthEvent
	agg.AddListener(func(e HealthEvent) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})

	agg.RegisterCheck(HealthCheck{Name: "listener-svc", Type: CheckTypeHTTP, Target: "http://a"})

	// Give the goroutine time to execute.
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(events) == 0 {
		t.Fatal("expected at least one event from listener")
	}
	if events[0].Type != "check_registered" {
		t.Fatalf("expected check_registered event, got %s", events[0].Type)
	}
	if events[0].CheckName != "listener-svc" {
		t.Fatalf("expected listener-svc, got %s", events[0].CheckName)
	}
}

// --- IsRunning / StartMonitoring / StopMonitoring tests ---

func TestIsRunningDefault(t *testing.T) {
	agg := NewAggregator(nil, nil)
	if agg.IsRunning() {
		t.Fatal("expected not running initially")
	}
}

func TestStartStopMonitoring(t *testing.T) {
	agg := NewAggregator(nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := agg.StartMonitoring(ctx, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("StartMonitoring: %v", err)
	}
	if !agg.IsRunning() {
		t.Fatal("expected running after start")
	}

	// Starting again should fail.
	err = agg.StartMonitoring(ctx, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected error starting monitoring twice")
	}

	agg.StopMonitoring()
	if agg.IsRunning() {
		t.Fatal("expected not running after stop")
	}
}

func TestStopMonitoringNoop(t *testing.T) {
	agg := NewAggregator(nil, nil)
	// Should not panic.
	agg.StopMonitoring()
}

// --- GetChecks tests ---

func TestGetChecksSorted(t *testing.T) {
	agg := NewAggregator(nil, nil)
	agg.RegisterCheck(HealthCheck{Name: "zebra", Type: CheckTypeHTTP, Target: "http://z"})
	agg.RegisterCheck(HealthCheck{Name: "alpha", Type: CheckTypeHTTP, Target: "http://a"})
	agg.RegisterCheck(HealthCheck{Name: "middle", Type: CheckTypeHTTP, Target: "http://m"})

	checks := agg.GetChecks()
	if len(checks) != 3 {
		t.Fatalf("expected 3 checks, got %d", len(checks))
	}
	if checks[0].Name != "alpha" || checks[1].Name != "middle" || checks[2].Name != "zebra" {
		t.Fatalf("checks not sorted: %s, %s, %s", checks[0].Name, checks[1].Name, checks[2].Name)
	}
}

// --- HTTPHandler tests ---

func TestHTTPHandlerHealth(t *testing.T) {
	agg := NewAggregator(nil, nil)
	handler := agg.HTTPHandler()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		// No checks registered = unknown = unhealthy response
		t.Logf("status: %d (expected 503 for unknown with no checks)", w.Code)
	}

	var sh SystemHealth
	if err := json.NewDecoder(w.Body).Decode(&sh); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

func TestHTTPHandlerHealthWithChecks(t *testing.T) {
	agg := NewAggregator(nil, nil)
	agg.RegisterCheck(HealthCheck{Name: "test-svc", Type: CheckTypeHTTP, Target: "http://a"})

	// Mark as healthy.
	agg.resultsMu.Lock()
	agg.results["test-svc"] = &HealthResult{Name: "test-svc", Status: StatusHealthy}
	agg.resultsMu.Unlock()

	handler := agg.HTTPHandler()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHTTPHandlerHealthSpecificCheck(t *testing.T) {
	agg := NewAggregator(nil, nil)
	agg.RegisterCheck(HealthCheck{Name: "my-svc", Type: CheckTypeHTTP, Target: "http://a"})

	handler := agg.HTTPHandler()

	// Valid check.
	req := httptest.NewRequest(http.MethodGet, "/health/my-svc", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Missing check.
	req2 := httptest.NewRequest(http.MethodGet, "/health/nonexistent", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w2.Code)
	}
}

func TestHTTPHandlerMethodNotAllowed(t *testing.T) {
	agg := NewAggregator(nil, nil)
	handler := agg.HTTPHandler()

	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// --- RunChecks with HTTP test server ---

func TestRunChecksHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	agg := NewAggregator(nil, nil)
	agg.RegisterCheck(HealthCheck{
		Name:   "http-test",
		Type:   CheckTypeHTTP,
		Target: srv.URL,
		HTTPConfig: &HTTPCheckConfig{
			Method:         "GET",
			ExpectedStatus: 200,
		},
	})

	results := agg.RunChecks(context.Background())
	r, ok := results["http-test"]
	if !ok {
		t.Fatal("expected result for http-test")
	}
	if r.Status != StatusHealthy {
		t.Fatalf("expected healthy, got %s: %s", r.Status, r.Message)
	}
}

func TestRunChecksHTTPUnhealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	agg := NewAggregator(nil, nil)
	agg.RegisterCheck(HealthCheck{
		Name:   "http-fail",
		Type:   CheckTypeHTTP,
		Target: srv.URL,
	})

	results := agg.RunChecks(context.Background())
	r := results["http-fail"]
	if r.Status == StatusHealthy {
		t.Fatal("expected unhealthy for 500 response")
	}
}

func TestRunChecksHTTPWithExpectedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","version":"1.0"}`))
	}))
	defer srv.Close()

	agg := NewAggregator(nil, nil)
	agg.RegisterCheck(HealthCheck{
		Name:   "body-check",
		Type:   CheckTypeHTTP,
		Target: srv.URL,
		HTTPConfig: &HTTPCheckConfig{
			Method:         "GET",
			ExpectedStatus: 200,
			ExpectedBody:   `"status":"ok"`,
		},
	})

	results := agg.RunChecks(context.Background())
	r := results["body-check"]
	if r.Status != StatusHealthy {
		t.Fatalf("expected healthy, got %s: %s", r.Status, r.Message)
	}
}

func TestRunChecksHTTPBodyMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"error"}`))
	}))
	defer srv.Close()

	agg := NewAggregator(nil, nil)
	agg.RegisterCheck(HealthCheck{
		Name:   "body-mismatch",
		Type:   CheckTypeHTTP,
		Target: srv.URL,
		HTTPConfig: &HTTPCheckConfig{
			Method:         "GET",
			ExpectedStatus: 200,
			ExpectedBody:   "all_good",
		},
	})

	results := agg.RunChecks(context.Background())
	r := results["body-mismatch"]
	if r.Status == StatusHealthy {
		t.Fatal("expected unhealthy for body mismatch")
	}
}

func TestRunChecksUnknownType(t *testing.T) {
	agg := NewAggregator(nil, nil)
	agg.RegisterCheck(HealthCheck{
		Name:   "unknown",
		Type:   CheckType("imaginary"),
		Target: "nowhere",
	})

	results := agg.RunChecks(context.Background())
	r := results["unknown"]
	if r.Status == StatusHealthy {
		t.Fatal("expected unhealthy for unknown check type")
	}
}

// --- Concurrent access tests ---

func TestAggregatorConcurrentRegister(t *testing.T) {
	agg := NewAggregator(nil, nil)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := string(rune('A'+idx)) + "-svc"
			agg.RegisterCheck(HealthCheck{
				Name:   name,
				Type:   CheckTypeHTTP,
				Target: "http://localhost",
			})
		}(i)
	}
	wg.Wait()

	checks := agg.GetChecks()
	if len(checks) != 20 {
		t.Fatalf("expected 20 checks, got %d", len(checks))
	}
}

// --- emitEvent with nil wotan ---

func TestEmitEventNilWotan(t *testing.T) {
	// Should not panic.
	agg := NewAggregator(nil, nil)
	agg.RegisterCheck(HealthCheck{Name: "nil-wotan", Type: CheckTypeHTTP, Target: "http://a"})
	// The RegisterCheck internally calls emitEvent; if we get here, nil wotan is handled.
}

// --- mockWotan for testing event publishing ---

type mockWotan struct {
	mu       sync.Mutex
	messages [][]byte
	err      error
}

func (m *mockWotan) Publish(_ context.Context, _ string, payload []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.messages = append(m.messages, payload)
	return nil
}

func TestEmitEventWithWotan(t *testing.T) {
	w := &mockWotan{}
	agg := NewAggregator(nil, w)
	agg.RegisterCheck(HealthCheck{Name: "wotan-test", Type: CheckTypeHTTP, Target: "http://a"})

	// Give async listeners time.
	time.Sleep(50 * time.Millisecond)

	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.messages) == 0 {
		t.Fatal("expected at least one wotan message")
	}
}

func TestEmitEventWotanError(t *testing.T) {
	w := &mockWotan{err: errors.New("publish failed")}
	agg := NewAggregator(nil, w)
	// Should not panic even when wotan returns an error.
	agg.RegisterCheck(HealthCheck{Name: "wotan-err", Type: CheckTypeHTTP, Target: "http://a"})
}

// --- TCP check with test server ---

func TestRunChecksTCP(t *testing.T) {
	// Start a TCP listener.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	// Extract host:port from the test server URL.
	addr := srv.Listener.Addr().String()

	agg := NewAggregator(nil, nil)
	agg.RegisterCheck(HealthCheck{
		Name:   "tcp-test",
		Type:   CheckTypeTCP,
		Target: addr,
	})

	results := agg.RunChecks(context.Background())
	r := results["tcp-test"]
	if r.Status != StatusHealthy {
		t.Fatalf("expected healthy TCP, got %s: %s", r.Status, r.Message)
	}
}

func TestRunChecksTCPFail(t *testing.T) {
	agg := NewAggregator(&AggregatorConfig{
		DefaultTimeout:  200 * time.Millisecond,
		DefaultInterval: 30 * time.Second,
		HistorySize:     100,
	}, nil)
	agg.RegisterCheck(HealthCheck{
		Name:   "tcp-fail",
		Type:   CheckTypeTCP,
		Target: "127.0.0.1:1", // Port 1 is unlikely to be listening.
	})

	results := agg.RunChecks(context.Background())
	r := results["tcp-fail"]
	if r.Status == StatusHealthy {
		t.Fatal("expected unhealthy for unreachable TCP")
	}
}
