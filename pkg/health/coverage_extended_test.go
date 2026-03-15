// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// --- Close ---

func TestCloseStopsMonitoring(t *testing.T) {
	agg := NewAggregator(nil, nil)
	ctx := context.Background()
	if err := agg.StartMonitoring(ctx, 100*time.Millisecond); err != nil {
		t.Fatalf("StartMonitoring: %v", err)
	}
	if !agg.IsRunning() {
		t.Fatal("expected running after start")
	}

	if err := agg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if agg.IsRunning() {
		t.Fatal("expected not running after Close")
	}
}

func TestCloseWhenNotRunning(t *testing.T) {
	agg := NewAggregator(nil, nil)
	// Should not panic.
	if err := agg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// --- applyThresholds ---

func TestApplyThresholdsRecovering(t *testing.T) {
	agg := NewAggregator(nil, nil)
	check := &HealthCheck{
		Name:             "thresh-test",
		SuccessThreshold: 3,
		FailureThreshold: 3,
	}
	result := &HealthResult{
		Status:               StatusHealthy,
		ConsecutiveSuccesses: 1, // below threshold of 3
	}
	agg.applyThresholds(check, result)
	if result.Status != StatusDegraded {
		t.Fatalf("expected degraded during recovery, got %s", result.Status)
	}
	if !strings.Contains(result.Message, "Recovering") {
		t.Fatalf("expected Recovering message, got %s", result.Message)
	}
}

func TestApplyThresholdsDegradedFailures(t *testing.T) {
	agg := NewAggregator(nil, nil)
	check := &HealthCheck{
		Name:             "thresh-fail",
		SuccessThreshold: 1,
		FailureThreshold: 5,
	}
	result := &HealthResult{
		Status:              StatusUnhealthy,
		ConsecutiveFailures: 2, // below threshold of 5
	}
	agg.applyThresholds(check, result)
	if result.Status != StatusDegraded {
		t.Fatalf("expected degraded for failures below threshold, got %s", result.Status)
	}
	if !strings.Contains(result.Message, "Degraded") {
		t.Fatalf("expected Degraded message, got %s", result.Message)
	}
}

func TestApplyThresholdsHealthyAboveThreshold(t *testing.T) {
	agg := NewAggregator(nil, nil)
	check := &HealthCheck{
		Name:             "thresh-ok",
		SuccessThreshold: 2,
		FailureThreshold: 3,
	}
	result := &HealthResult{
		Status:               StatusHealthy,
		ConsecutiveSuccesses: 5,
		Message:              "original",
	}
	agg.applyThresholds(check, result)
	if result.Status != StatusHealthy {
		t.Fatalf("expected healthy, got %s", result.Status)
	}
	if result.Message != "original" {
		t.Fatalf("expected unchanged message, got %s", result.Message)
	}
}

// --- updateConsecutiveCounts ---

func TestUpdateConsecutiveCountsNewCheckHealthy(t *testing.T) {
	agg := NewAggregator(nil, nil)
	check := &HealthCheck{Name: "new-svc"}
	result := &HealthResult{Status: StatusHealthy}

	agg.updateConsecutiveCounts(check, result)
	if result.ConsecutiveSuccesses != 1 {
		t.Fatalf("expected 1 success, got %d", result.ConsecutiveSuccesses)
	}
	if result.ConsecutiveFailures != 0 {
		t.Fatalf("expected 0 failures, got %d", result.ConsecutiveFailures)
	}
}

func TestUpdateConsecutiveCountsNewCheckUnhealthy(t *testing.T) {
	agg := NewAggregator(nil, nil)
	check := &HealthCheck{Name: "new-fail"}
	result := &HealthResult{Status: StatusUnhealthy}

	agg.updateConsecutiveCounts(check, result)
	if result.ConsecutiveFailures != 1 {
		t.Fatalf("expected 1 failure, got %d", result.ConsecutiveFailures)
	}
	if result.ConsecutiveSuccesses != 0 {
		t.Fatalf("expected 0 successes, got %d", result.ConsecutiveSuccesses)
	}
}

func TestUpdateConsecutiveCountsAccumulate(t *testing.T) {
	agg := NewAggregator(nil, nil)
	check := &HealthCheck{Name: "accum"}
	// Seed previous result
	agg.resultsMu.Lock()
	agg.results["accum"] = &HealthResult{
		ConsecutiveSuccesses: 3,
		ConsecutiveFailures:  0,
	}
	agg.resultsMu.Unlock()

	result := &HealthResult{Status: StatusHealthy}
	agg.updateConsecutiveCounts(check, result)
	if result.ConsecutiveSuccesses != 4 {
		t.Fatalf("expected 4, got %d", result.ConsecutiveSuccesses)
	}
	if result.ConsecutiveFailures != 0 {
		t.Fatalf("expected 0, got %d", result.ConsecutiveFailures)
	}
}

func TestUpdateConsecutiveCountsResetOnFailure(t *testing.T) {
	agg := NewAggregator(nil, nil)
	check := &HealthCheck{Name: "reset"}
	agg.resultsMu.Lock()
	agg.results["reset"] = &HealthResult{
		ConsecutiveSuccesses: 5,
		ConsecutiveFailures:  0,
	}
	agg.resultsMu.Unlock()

	result := &HealthResult{Status: StatusUnhealthy}
	agg.updateConsecutiveCounts(check, result)
	if result.ConsecutiveFailures != 1 {
		t.Fatalf("expected 1, got %d", result.ConsecutiveFailures)
	}
	if result.ConsecutiveSuccesses != 0 {
		t.Fatalf("expected 0, got %d", result.ConsecutiveSuccesses)
	}
}

// --- recordMetrics with degraded status ---

func TestRecordMetricsDegraded(t *testing.T) {
	agg := NewAggregator(nil, nil)
	check := &HealthCheck{Name: "metric-deg", Type: CheckTypeHTTP}
	result := &HealthResult{
		Status:   StatusDegraded,
		Duration: 10 * time.Millisecond,
	}
	// Should not panic.
	agg.recordMetrics(check, result)
}

func TestRecordMetricsUnhealthy(t *testing.T) {
	agg := NewAggregator(nil, nil)
	check := &HealthCheck{Name: "metric-unhealthy", Type: CheckTypeTCP}
	result := &HealthResult{
		Status:   StatusUnhealthy,
		Duration: 50 * time.Millisecond,
	}
	// Should not panic.
	agg.recordMetrics(check, result)
}

// --- updateCircuitBreakerMetrics ---

func TestUpdateCircuitBreakerMetricsHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker("test-cb", 1, 1, 1*time.Millisecond)
	cb.RecordFailure() // open
	time.Sleep(5 * time.Millisecond)
	cb.Allow() // half-open

	agg := NewAggregator(nil, nil)
	// Should not panic.
	agg.updateCircuitBreakerMetrics("test-cb", cb)
}

func TestUpdateCircuitBreakerMetricsOpen(t *testing.T) {
	cb := NewCircuitBreaker("test-cb-open", 1, 1, 30*time.Second)
	cb.RecordFailure()

	agg := NewAggregator(nil, nil)
	agg.updateCircuitBreakerMetrics("test-cb-open", cb)
}

// --- Circuit breaker Allow default branch ---

func TestCircuitBreakerAllowUnknownState(t *testing.T) {
	cb := &CircuitBreaker{
		Name:  "unknown-state",
		State: "unexpected",
	}
	if !cb.Allow() {
		t.Fatal("expected Allow() to return true for unknown state")
	}
}

// --- Wotan check via httptest ---

func TestRunChecksWotanHealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	addr := srv.Listener.Addr().String()
	agg := NewAggregator(nil, nil)
	agg.RegisterCheck(HealthCheck{
		Name:   "wotan-healthy",
		Type:   CheckTypeWotan,
		Target: addr,
	})

	results := agg.RunChecks(context.Background())
	r := results["wotan-healthy"]
	if r.Status != StatusHealthy {
		t.Fatalf("expected healthy, got %s: %s", r.Status, r.Message)
	}
}

func TestRunChecksWotanDegradedHTTPFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	addr := srv.Listener.Addr().String()
	agg := NewAggregator(nil, nil)
	agg.RegisterCheck(HealthCheck{
		Name:   "wotan-degraded",
		Type:   CheckTypeWotan,
		Target: addr,
	})

	results := agg.RunChecks(context.Background())
	r := results["wotan-degraded"]
	if r.Status != StatusDegraded {
		t.Fatalf("expected degraded, got %s: %s", r.Status, r.Message)
	}
}

func TestRunChecksWotanTCPFail(t *testing.T) {
	agg := NewAggregator(&AggregatorConfig{
		DefaultTimeout:  200 * time.Millisecond,
		DefaultInterval: 30 * time.Second,
		HistorySize:     100,
	}, nil)
	agg.RegisterCheck(HealthCheck{
		Name:   "wotan-tcp-fail",
		Type:   CheckTypeWotan,
		Target: "127.0.0.1:1",
	})

	results := agg.RunChecks(context.Background())
	r := results["wotan-tcp-fail"]
	if r.Status == StatusHealthy {
		t.Fatalf("expected non-healthy status for TCP failure, got %s: %s", r.Status, r.Message)
	}
}

// --- HTTP Handler: /checks endpoint ---

func TestHTTPHandlerChecks(t *testing.T) {
	agg := NewAggregator(nil, nil)
	agg.RegisterCheck(HealthCheck{Name: "svc-a", Type: CheckTypeHTTP, Target: "http://a"})
	agg.RegisterCheck(HealthCheck{Name: "svc-b", Type: CheckTypeTCP, Target: "localhost:80"})

	handler := agg.HTTPHandler()
	req := httptest.NewRequest(http.MethodGet, "/checks", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	count, ok := body["count"].(float64)
	if !ok || int(count) != 2 {
		t.Fatalf("expected count 2, got %v", body["count"])
	}
}

func TestHTTPHandlerChecksMethodNotAllowed(t *testing.T) {
	agg := NewAggregator(nil, nil)
	handler := agg.HTTPHandler()
	req := httptest.NewRequest(http.MethodPost, "/checks", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// --- HTTP Handler: /history/{name} ---

func TestHTTPHandlerHistory(t *testing.T) {
	agg := NewAggregator(nil, nil)
	agg.RegisterCheck(HealthCheck{Name: "hist-svc", Type: CheckTypeHTTP, Target: "http://a"})

	agg.historyMu.Lock()
	h := agg.history["hist-svc"]
	h.Add(HealthResult{Name: "hist-svc", Status: StatusHealthy, Timestamp: time.Now()})
	agg.historyMu.Unlock()

	handler := agg.HTTPHandler()
	req := httptest.NewRequest(http.MethodGet, "/history/hist-svc?duration=1h", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["name"] != "hist-svc" {
		t.Fatalf("expected hist-svc, got %v", body["name"])
	}
}

func TestHTTPHandlerHistoryDefaultDuration(t *testing.T) {
	agg := NewAggregator(nil, nil)
	agg.RegisterCheck(HealthCheck{Name: "hist-def", Type: CheckTypeHTTP, Target: "http://a"})

	handler := agg.HTTPHandler()
	req := httptest.NewRequest(http.MethodGet, "/history/hist-def", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHTTPHandlerHistoryInvalidDuration(t *testing.T) {
	agg := NewAggregator(nil, nil)
	agg.RegisterCheck(HealthCheck{Name: "hist-bad", Type: CheckTypeHTTP, Target: "http://a"})

	handler := agg.HTTPHandler()
	req := httptest.NewRequest(http.MethodGet, "/history/hist-bad?duration=invalid", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHTTPHandlerHistoryNotFound(t *testing.T) {
	agg := NewAggregator(nil, nil)
	handler := agg.HTTPHandler()
	req := httptest.NewRequest(http.MethodGet, "/history/nonexistent", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHTTPHandlerHistoryEmptyName(t *testing.T) {
	agg := NewAggregator(nil, nil)
	handler := agg.HTTPHandler()
	req := httptest.NewRequest(http.MethodGet, "/history/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHTTPHandlerHistoryMethodNotAllowed(t *testing.T) {
	agg := NewAggregator(nil, nil)
	handler := agg.HTTPHandler()
	req := httptest.NewRequest(http.MethodPost, "/history/some", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// --- HTTP Handler: /checks/register (POST) ---

func TestHTTPHandlerRegisterCheck(t *testing.T) {
	agg := NewAggregator(nil, nil)
	handler := agg.HTTPHandler()

	body := `{"name":"new-check","type":"http","target":"http://localhost/health"}`
	req := httptest.NewRequest(http.MethodPost, "/checks/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// Verify the check was registered.
	checks := agg.GetChecks()
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(checks))
	}
}

func TestHTTPHandlerRegisterCheckDuplicate(t *testing.T) {
	agg := NewAggregator(nil, nil)
	agg.RegisterCheck(HealthCheck{Name: "dup-reg", Type: CheckTypeHTTP, Target: "http://a"})

	handler := agg.HTTPHandler()
	body := `{"name":"dup-reg","type":"http","target":"http://b"}`
	req := httptest.NewRequest(http.MethodPost, "/checks/register", strings.NewReader(body))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func TestHTTPHandlerRegisterCheckBadJSON(t *testing.T) {
	agg := NewAggregator(nil, nil)
	handler := agg.HTTPHandler()

	req := httptest.NewRequest(http.MethodPost, "/checks/register", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHTTPHandlerRegisterCheckEmptyName(t *testing.T) {
	agg := NewAggregator(nil, nil)
	handler := agg.HTTPHandler()

	body := `{"name":"","type":"http","target":"http://a"}`
	req := httptest.NewRequest(http.MethodPost, "/checks/register", strings.NewReader(body))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHTTPHandlerRegisterMethodNotAllowed(t *testing.T) {
	agg := NewAggregator(nil, nil)
	handler := agg.HTTPHandler()

	req := httptest.NewRequest(http.MethodGet, "/checks/register", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// --- HTTP Handler: DELETE /checks/{name} ---

func TestHTTPHandlerDeleteCheck(t *testing.T) {
	agg := NewAggregator(nil, nil)
	agg.RegisterCheck(HealthCheck{Name: "del-me", Type: CheckTypeHTTP, Target: "http://a"})

	handler := agg.HTTPHandler()
	req := httptest.NewRequest(http.MethodDelete, "/checks/del-me", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	checks := agg.GetChecks()
	if len(checks) != 0 {
		t.Fatalf("expected 0 checks after delete, got %d", len(checks))
	}
}

func TestHTTPHandlerDeleteCheckNotFound(t *testing.T) {
	agg := NewAggregator(nil, nil)
	handler := agg.HTTPHandler()
	req := httptest.NewRequest(http.MethodDelete, "/checks/nonexistent", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHTTPHandlerDeleteCheckEmptyName(t *testing.T) {
	agg := NewAggregator(nil, nil)
	handler := agg.HTTPHandler()
	req := httptest.NewRequest(http.MethodDelete, "/checks/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHTTPHandlerDeleteCheckMethodNotAllowed(t *testing.T) {
	agg := NewAggregator(nil, nil)
	handler := agg.HTTPHandler()
	req := httptest.NewRequest(http.MethodGet, "/checks/some-check", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// --- HTTP Handler: POST /run ---

func TestHTTPHandlerRunChecks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	agg := NewAggregator(nil, nil)
	agg.RegisterCheck(HealthCheck{
		Name:   "run-svc",
		Type:   CheckTypeHTTP,
		Target: srv.URL,
		HTTPConfig: &HTTPCheckConfig{
			Method:         "GET",
			ExpectedStatus: 200,
		},
	})

	handler := agg.HTTPHandler()
	req := httptest.NewRequest(http.MethodPost, "/run", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	count, ok := body["count"].(float64)
	if !ok || int(count) != 1 {
		t.Fatalf("expected count 1, got %v", body["count"])
	}
}

func TestHTTPHandlerRunMethodNotAllowed(t *testing.T) {
	agg := NewAggregator(nil, nil)
	handler := agg.HTTPHandler()
	req := httptest.NewRequest(http.MethodGet, "/run", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// --- HTTP Handler: /ready ---

func TestHTTPHandlerReadyHealthy(t *testing.T) {
	agg := NewAggregator(nil, nil)
	agg.RegisterCheck(HealthCheck{Name: "rdy-svc", Type: CheckTypeHTTP, Target: "http://a"})
	agg.resultsMu.Lock()
	agg.results["rdy-svc"] = &HealthResult{Name: "rdy-svc", Status: StatusHealthy}
	agg.resultsMu.Unlock()

	handler := agg.HTTPHandler()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHTTPHandlerReadyUnhealthy(t *testing.T) {
	agg := NewAggregator(nil, nil)
	agg.RegisterCheck(HealthCheck{Name: "rdy-bad", Type: CheckTypeHTTP, Target: "http://a"})
	agg.resultsMu.Lock()
	agg.results["rdy-bad"] = &HealthResult{Name: "rdy-bad", Status: StatusUnhealthy}
	agg.resultsMu.Unlock()

	handler := agg.HTTPHandler()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestHTTPHandlerReadyMethodNotAllowed(t *testing.T) {
	agg := NewAggregator(nil, nil)
	handler := agg.HTTPHandler()
	req := httptest.NewRequest(http.MethodPost, "/ready", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// --- HTTP Handler: /live ---

func TestHTTPHandlerLive(t *testing.T) {
	agg := NewAggregator(nil, nil)
	handler := agg.HTTPHandler()
	req := httptest.NewRequest(http.MethodGet, "/live", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "alive" {
		t.Fatalf("expected alive, got %s", body["status"])
	}
}

func TestHTTPHandlerLiveMethodNotAllowed(t *testing.T) {
	agg := NewAggregator(nil, nil)
	handler := agg.HTTPHandler()
	req := httptest.NewRequest(http.MethodPost, "/live", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// --- HTTP Handler: /health with degraded status ---

func TestHTTPHandlerHealthDegraded(t *testing.T) {
	agg := NewAggregator(nil, nil)
	agg.RegisterCheck(HealthCheck{Name: "deg-svc", Type: CheckTypeHTTP, Target: "http://a"})
	agg.resultsMu.Lock()
	agg.results["deg-svc"] = &HealthResult{Name: "deg-svc", Status: StatusDegraded}
	agg.resultsMu.Unlock()

	handler := agg.HTTPHandler()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for degraded, got %d", w.Code)
	}
}

// --- HTTP Handler: /health/ empty name ---

func TestHTTPHandlerHealthSlashEmptyName(t *testing.T) {
	agg := NewAggregator(nil, nil)
	handler := agg.HTTPHandler()
	req := httptest.NewRequest(http.MethodGet, "/health/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHTTPHandlerHealthSlashMethodNotAllowed(t *testing.T) {
	agg := NewAggregator(nil, nil)
	handler := agg.HTTPHandler()
	req := httptest.NewRequest(http.MethodPost, "/health/some-check", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// --- HTTP check with FollowRedirects ---

func TestRunChecksHTTPFollowRedirects(t *testing.T) {
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer final.Close()

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusFound)
	}))
	defer redirect.Close()

	agg := NewAggregator(nil, nil)
	agg.RegisterCheck(HealthCheck{
		Name:   "redirect-follow",
		Type:   CheckTypeHTTP,
		Target: redirect.URL,
		HTTPConfig: &HTTPCheckConfig{
			Method:          "GET",
			ExpectedStatus:  200,
			FollowRedirects: true,
		},
	})

	results := agg.RunChecks(context.Background())
	r := results["redirect-follow"]
	if r.Status != StatusHealthy {
		t.Fatalf("expected healthy after following redirect, got %s: %s", r.Status, r.Message)
	}
}

// --- HTTP check with custom headers ---

func TestRunChecksHTTPWithHeaders(t *testing.T) {
	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Custom")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	agg := NewAggregator(nil, nil)
	agg.RegisterCheck(HealthCheck{
		Name:   "header-check",
		Type:   CheckTypeHTTP,
		Target: srv.URL,
		HTTPConfig: &HTTPCheckConfig{
			Method:         "GET",
			ExpectedStatus: 200,
			Headers:        map[string]string{"X-Custom": "test-value"},
		},
	})

	agg.RunChecks(context.Background())
	if gotHeader != "test-value" {
		t.Fatalf("expected X-Custom header 'test-value', got '%s'", gotHeader)
	}
}

// --- runSingleCheck circuit breaker open path ---

func TestRunSingleCheckCircuitBreakerOpen(t *testing.T) {
	agg := NewAggregator(nil, nil)
	agg.RegisterCheck(HealthCheck{
		Name:   "cb-open-svc",
		Type:   CheckTypeHTTP,
		Target: "http://localhost:99999/health",
	})

	// Trip the circuit breaker.
	agg.cbMu.Lock()
	cb := agg.circuitBreakers["cb-open-svc"]
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()
	agg.cbMu.Unlock()

	results := agg.RunChecks(context.Background())
	r := results["cb-open-svc"]
	if r.Status != StatusUnhealthy {
		t.Fatalf("expected unhealthy, got %s", r.Status)
	}
	if !strings.Contains(r.Message, "Circuit breaker") {
		t.Fatalf("expected circuit breaker message, got %s", r.Message)
	}
}

// --- StartMonitoring context cancellation ---

func TestStartMonitoringContextCancel(t *testing.T) {
	agg := NewAggregator(nil, nil)
	ctx, cancel := context.WithCancel(context.Background())

	err := agg.StartMonitoring(ctx, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("StartMonitoring: %v", err)
	}

	// Cancel the context.
	cancel()
	// Give goroutine time to exit.
	time.Sleep(100 * time.Millisecond)

	// Monitoring goroutine should have exited, but running flag is set by StopMonitoring.
	// The goroutine exits but the flag is still true until StopMonitoring is called.
	// Call StopMonitoring to clean up.
	agg.StopMonitoring()
	if agg.IsRunning() {
		t.Fatal("expected not running after context cancel + StopMonitoring")
	}
}
