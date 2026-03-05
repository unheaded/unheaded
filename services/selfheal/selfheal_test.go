// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package selfheal

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"unheaded/pkg/logger"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newTestLogger returns a logger that discards output (safe for concurrent use).
func newTestLogger() *logger.Logger {
	return logger.New(io.Discard)
}

// newTestService returns a ready-to-use *Service with defaults.
func newTestService(t *testing.T) *Service {
	t.Helper()
	return NewService(newTestLogger(), nil, nil)
}

// addTestEndpoint registers an endpoint and returns its health struct.
func addTestEndpoint(t *testing.T, svc *Service, id string, healthScore int, failCount int, circuit CircuitState) *EndpointHealth {
	t.Helper()
	svc.mu.Lock()
	defer svc.mu.Unlock()

	ep := &EndpointHealth{
		EndpointID:   id,
		HealthScore:  healthScore,
		FailCount:    failCount,
		CircuitState: circuit,
		LastSeen:     time.Now(),
	}
	svc.endpoints[id] = ep
	svc.circuits[id] = circuit
	return ep
}

// addTestBackupRoster adds a backup roster for testing.
func addTestBackupRoster(t *testing.T, svc *Service, primaryID string, backups []string) {
	t.Helper()
	svc.mu.Lock()
	defer svc.mu.Unlock()
	svc.backups[primaryID] = &BackupRoster{
		PrimaryID: primaryID,
		Backups:   backups,
		Count:     len(backups),
	}
}

// ---------------------------------------------------------------------------
// TestHealthScoreCalculation
// ---------------------------------------------------------------------------

func TestHealthScoreCalculation(t *testing.T) {
	svc := newTestService(t)

	t.Run("perfect health — all fast, no errors, circuit closed", func(t *testing.T) {
		ep := &EndpointHealth{
			EndpointID:     "ep-1",
			FailCount:      0,
			LatencyBuckets: [5]uint64{900, 100, 0, 0, 0}, // 100% < 10ms
			CircuitState:   CircuitClosed,
		}

		score := svc.calculateHealthScore(ep)
		if score != 100 {
			t.Errorf("score = %d, want 100 for perfect health", score)
		}
	})

	t.Run("degraded — some slow requests and errors", func(t *testing.T) {
		ep := &EndpointHealth{
			EndpointID:     "ep-2",
			FailCount:      3,
			LatencyBuckets: [5]uint64{300, 200, 200, 200, 100}, // mixed latency
			CircuitState:   CircuitClosed,
		}

		score := svc.calculateHealthScore(ep)
		if score >= 100 || score <= 0 {
			t.Errorf("score = %d, expected degraded score between 1-99", score)
		}
	})

	t.Run("zero samples defaults to full score", func(t *testing.T) {
		ep := &EndpointHealth{
			EndpointID:     "ep-3",
			FailCount:      0,
			LatencyBuckets: [5]uint64{0, 0, 0, 0, 0},
			CircuitState:   CircuitClosed,
		}

		score := svc.calculateHealthScore(ep)
		if score != 100 {
			t.Errorf("score = %d, want 100 for zero samples (default healthy)", score)
		}
	})

	t.Run("open circuit penalizes score", func(t *testing.T) {
		epClosed := &EndpointHealth{
			EndpointID:     "ep-4a",
			FailCount:      0,
			LatencyBuckets: [5]uint64{1000, 0, 0, 0, 0},
			CircuitState:   CircuitClosed,
		}
		epOpen := &EndpointHealth{
			EndpointID:     "ep-4b",
			FailCount:      0,
			LatencyBuckets: [5]uint64{1000, 0, 0, 0, 0},
			CircuitState:   CircuitOpen,
		}

		scoreClosed := svc.calculateHealthScore(epClosed)
		scoreOpen := svc.calculateHealthScore(epOpen)

		if scoreOpen >= scoreClosed {
			t.Errorf("open circuit score (%d) should be less than closed (%d)", scoreOpen, scoreClosed)
		}
	})

	t.Run("half-open circuit partially penalizes", func(t *testing.T) {
		ep := &EndpointHealth{
			EndpointID:     "ep-5",
			FailCount:      0,
			LatencyBuckets: [5]uint64{1000, 0, 0, 0, 0},
			CircuitState:   CircuitHalfOpen,
		}

		score := svc.calculateHealthScore(ep)
		// Should be less than 100 due to half-open penalty (circuit weight 10%, half-open = 50)
		if score >= 100 {
			t.Errorf("score = %d, want < 100 with half-open circuit", score)
		}
		// But more than open circuit
		if score < 90 {
			t.Errorf("score = %d, expected >= 90 (only circuit penalty)", score)
		}
	})

	t.Run("high fail count reduces score", func(t *testing.T) {
		ep := &EndpointHealth{
			EndpointID:     "ep-6",
			FailCount:      10,
			LatencyBuckets: [5]uint64{1000, 0, 0, 0, 0},
			CircuitState:   CircuitClosed,
		}

		score := svc.calculateHealthScore(ep)
		if score >= 100 {
			t.Errorf("score = %d, want < 100 with high fail count", score)
		}
	})
}

// ---------------------------------------------------------------------------
// TestCircuitBreakerStateMachine
// ---------------------------------------------------------------------------

func TestCircuitBreakerStateMachine(t *testing.T) {
	t.Run("CLOSED stays CLOSED when healthy", func(t *testing.T) {
		svc := newTestService(t)
		ep := addTestEndpoint(t, svc, "ep-healthy", 100, 0, CircuitClosed)
		ep.LatencyBuckets = [5]uint64{1000, 0, 0, 0, 0}

		svc.mu.Lock()
		svc.evaluateEndpoint(context.Background(), "ep-healthy", ep)
		svc.mu.Unlock()

		if ep.CircuitState != CircuitClosed {
			t.Errorf("CircuitState = %s, want CLOSED", ep.CircuitState)
		}
	})

	t.Run("CLOSED to OPEN when health drops and fails exceed threshold", func(t *testing.T) {
		svc := newTestService(t)
		ep := addTestEndpoint(t, svc, "ep-failing", 30, 5, CircuitClosed)
		ep.LatencyBuckets = [5]uint64{0, 0, 0, 0, 1000} // all gt50ms
		addTestBackupRoster(t, svc, "ep-failing", []string{"backup-1"})

		svc.mu.Lock()
		svc.evaluateEndpoint(context.Background(), "ep-failing", ep)
		svc.mu.Unlock()

		if ep.CircuitState != CircuitOpen {
			t.Errorf("CircuitState = %s, want OPEN after health drop + fails", ep.CircuitState)
		}
	})

	t.Run("OPEN to HALF_OPEN when health improves", func(t *testing.T) {
		svc := newTestService(t)
		ep := addTestEndpoint(t, svc, "ep-recovering", 60, 5, CircuitOpen)
		ep.LatencyBuckets = [5]uint64{800, 100, 50, 50, 0}

		svc.mu.Lock()
		svc.evaluateEndpoint(context.Background(), "ep-recovering", ep)
		svc.mu.Unlock()

		if ep.CircuitState != CircuitHalfOpen {
			t.Errorf("CircuitState = %s, want HALF_OPEN after score improvement", ep.CircuitState)
		}
	})

	t.Run("HALF_OPEN to CLOSED after consecutive healthy checks", func(t *testing.T) {
		svc := newTestService(t)
		ep := addTestEndpoint(t, svc, "ep-healing", 90, 1, CircuitHalfOpen)
		ep.LatencyBuckets = [5]uint64{950, 30, 20, 0, 0}
		ep.SuccessStreak = circuitCloseConsecutive - 1 // one more success needed

		svc.mu.Lock()
		svc.evaluateEndpoint(context.Background(), "ep-healing", ep)
		svc.mu.Unlock()

		if ep.CircuitState != CircuitClosed {
			t.Errorf("CircuitState = %s, want CLOSED after consecutive healthy checks", ep.CircuitState)
		}
		if ep.FailCount != 0 {
			t.Errorf("FailCount = %d, want 0 after recovery", ep.FailCount)
		}
	})

	t.Run("HALF_OPEN reverts to OPEN on failure", func(t *testing.T) {
		svc := newTestService(t)
		ep := addTestEndpoint(t, svc, "ep-relapse", 40, 5, CircuitHalfOpen)
		ep.LatencyBuckets = [5]uint64{0, 0, 0, 100, 900} // bad latency
		ep.SuccessStreak = 2

		svc.mu.Lock()
		svc.evaluateEndpoint(context.Background(), "ep-relapse", ep)
		svc.mu.Unlock()

		if ep.CircuitState != CircuitOpen {
			t.Errorf("CircuitState = %s, want OPEN after relapse", ep.CircuitState)
		}
		if ep.SuccessStreak != 0 {
			t.Errorf("SuccessStreak = %d, want 0 after relapse", ep.SuccessStreak)
		}
	})
}

// ---------------------------------------------------------------------------
// TestFailoverTrigger
// ---------------------------------------------------------------------------

func TestFailoverTrigger(t *testing.T) {
	t.Run("auto failover on circuit open with backup", func(t *testing.T) {
		svc := newTestService(t)
		ep := addTestEndpoint(t, svc, "ep-auto", 30, 5, CircuitClosed)
		ep.LatencyBuckets = [5]uint64{0, 0, 0, 0, 1000}
		addTestBackupRoster(t, svc, "ep-auto", []string{"backup-a", "backup-b"})

		svc.mu.Lock()
		svc.evaluateEndpoint(context.Background(), "ep-auto", ep)
		svc.mu.Unlock()

		if ep.CircuitState != CircuitOpen {
			t.Errorf("CircuitState = %s, want OPEN", ep.CircuitState)
		}
		if ep.LastFailover == nil {
			t.Error("LastFailover should be set after auto failover")
		}

		events := svc.GetEvents()
		if len(events) == 0 {
			t.Fatal("expected at least one failover event")
		}
		if events[0].BackupID != "backup-a" {
			t.Errorf("BackupID = %q, want %q", events[0].BackupID, "backup-a")
		}
	})

	t.Run("manual failover via API", func(t *testing.T) {
		svc := newTestService(t)
		addTestEndpoint(t, svc, "ep-manual", 80, 0, CircuitClosed)
		addTestBackupRoster(t, svc, "ep-manual", []string{"backup-c"})

		mux := http.NewServeMux()
		svc.registerRoutes(mux)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/endpoints/ep-manual/failover", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("POST failover status = %d, want 200", rec.Code)
		}

		var event FailoverEvent
		if err := json.NewDecoder(rec.Body).Decode(&event); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if event.BackupID != "backup-c" {
			t.Errorf("BackupID = %q, want %q", event.BackupID, "backup-c")
		}
	})

	t.Run("failover without backup returns conflict", func(t *testing.T) {
		svc := newTestService(t)
		addTestEndpoint(t, svc, "ep-no-backup", 80, 0, CircuitClosed)

		mux := http.NewServeMux()
		svc.registerRoutes(mux)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/endpoints/ep-no-backup/failover", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusConflict {
			t.Errorf("POST failover (no backup) status = %d, want 409", rec.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// TestRecoveryDetection
// ---------------------------------------------------------------------------

func TestRecoveryDetection(t *testing.T) {
	t.Run("manual recovery restores health", func(t *testing.T) {
		svc := newTestService(t)
		addTestEndpoint(t, svc, "ep-recover", 30, 5, CircuitOpen)

		mux := http.NewServeMux()
		svc.registerRoutes(mux)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/endpoints/ep-recover/recover", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("POST recover status = %d, want 200", rec.Code)
		}

		ep, ok := svc.GetEndpoint("ep-recover")
		if !ok {
			t.Fatal("endpoint not found after recovery")
		}
		if ep.CircuitState != CircuitClosed {
			t.Errorf("CircuitState = %s, want CLOSED after recovery", ep.CircuitState)
		}
		if ep.HealthScore != svc.config.DefaultHealthScore {
			t.Errorf("HealthScore = %d, want %d", ep.HealthScore, svc.config.DefaultHealthScore)
		}
		if ep.FailCount != 0 {
			t.Errorf("FailCount = %d, want 0", ep.FailCount)
		}
		if ep.LastRecovery == nil {
			t.Error("LastRecovery should be set")
		}
	})

	t.Run("recovery of unknown endpoint returns 404", func(t *testing.T) {
		svc := newTestService(t)

		mux := http.NewServeMux()
		svc.registerRoutes(mux)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/endpoints/unknown/recover", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("POST recover (unknown) status = %d, want 404", rec.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// TestBackupSelection
// ---------------------------------------------------------------------------

func TestBackupSelection(t *testing.T) {
	t.Run("selects first non-open backup", func(t *testing.T) {
		svc := newTestService(t)
		svc.mu.Lock()
		svc.circuits["backup-1"] = CircuitOpen
		svc.circuits["backup-2"] = CircuitClosed
		svc.circuits["backup-3"] = CircuitClosed
		svc.mu.Unlock()

		roster := &BackupRoster{
			PrimaryID: "primary",
			Backups:   []string{"backup-1", "backup-2", "backup-3"},
			Count:     3,
		}

		selected := svc.selectBackup(roster)
		if selected != "backup-2" {
			t.Errorf("selected = %q, want %q (skip open backup-1)", selected, "backup-2")
		}
	})

	t.Run("falls back to first backup when all open", func(t *testing.T) {
		svc := newTestService(t)
		svc.mu.Lock()
		svc.circuits["backup-1"] = CircuitOpen
		svc.circuits["backup-2"] = CircuitOpen
		svc.mu.Unlock()

		roster := &BackupRoster{
			PrimaryID: "primary",
			Backups:   []string{"backup-1", "backup-2"},
			Count:     2,
		}

		selected := svc.selectBackup(roster)
		if selected != "backup-1" {
			t.Errorf("selected = %q, want %q (fallback to first)", selected, "backup-1")
		}
	})

	t.Run("empty roster returns empty string", func(t *testing.T) {
		svc := newTestService(t)
		roster := &BackupRoster{
			PrimaryID: "primary",
			Backups:   []string{},
			Count:     0,
		}

		selected := svc.selectBackup(roster)
		if selected != "" {
			t.Errorf("selected = %q, want empty string", selected)
		}
	})
}

// ---------------------------------------------------------------------------
// HTTP API tests
// ---------------------------------------------------------------------------

func TestHTTPEndpoints(t *testing.T) {
	svc := newTestService(t)
	addTestEndpoint(t, svc, "ep-a", 95, 0, CircuitClosed)
	addTestEndpoint(t, svc, "ep-b", 60, 3, CircuitHalfOpen)

	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	t.Run("list endpoints", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/endpoints", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/v1/endpoints status = %d, want 200", rec.Code)
		}

		var resp map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&resp)
		if count, ok := resp["count"].(float64); !ok || int(count) != 2 {
			t.Errorf("count = %v, want 2", resp["count"])
		}
	})

	t.Run("get endpoint health", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/endpoints/ep-a/health", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("GET endpoint health status = %d, want 200", rec.Code)
		}

		var ep EndpointHealth
		json.NewDecoder(rec.Body).Decode(&ep)
		if ep.EndpointID != "ep-a" {
			t.Errorf("EndpointID = %q, want %q", ep.EndpointID, "ep-a")
		}
	})

	t.Run("unknown endpoint returns 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/endpoints/unknown/health", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("GET unknown endpoint status = %d, want 404", rec.Code)
		}
	})
}

func TestHTTPBackups(t *testing.T) {
	svc := newTestService(t)

	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	t.Run("create backup roster", func(t *testing.T) {
		body := `{"primary_id":"ep-primary","backups":["backup-1","backup-2"]}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/backups", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("POST /api/v1/backups status = %d, want 201", rec.Code)
		}

		var roster BackupRoster
		json.NewDecoder(rec.Body).Decode(&roster)
		if roster.PrimaryID != "ep-primary" {
			t.Errorf("PrimaryID = %q, want %q", roster.PrimaryID, "ep-primary")
		}
		if roster.Count != 2 {
			t.Errorf("Count = %d, want 2", roster.Count)
		}
	})

	t.Run("list backups", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/backups", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/v1/backups status = %d, want 200", rec.Code)
		}
	})
}

func TestHTTPCircuitBreakers(t *testing.T) {
	svc := newTestService(t)
	addTestEndpoint(t, svc, "ep-x", 90, 0, CircuitClosed)
	addTestEndpoint(t, svc, "ep-y", 30, 5, CircuitOpen)

	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/circuit-breakers", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/circuit-breakers status = %d, want 200", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	if count, ok := resp["count"].(float64); !ok || int(count) != 2 {
		t.Errorf("count = %v, want 2", resp["count"])
	}
}

func TestHTTPHealthAndReady(t *testing.T) {
	svc := newTestService(t)

	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	t.Run("health", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("GET /health status = %d, want 200", rec.Code)
		}
	})

	t.Run("ready", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ready", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("GET /ready status = %d, want 200", rec.Code)
		}
	})

	t.Run("metrics", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("GET /metrics status = %d, want 200", rec.Code)
		}
	})
}
