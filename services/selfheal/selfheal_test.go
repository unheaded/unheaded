// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package selfheal

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/logger"
	wotanClient "unheaded/pkg/wotan-client"
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

// ---------------------------------------------------------------------------
// RegisterEndpoint / ListEndpoints / Stats / WotanDrops
// ---------------------------------------------------------------------------

func TestRegisterEndpoint(t *testing.T) {
	svc := newTestService(t)

	t.Run("registers new endpoint with defaults", func(t *testing.T) {
		svc.RegisterEndpoint("ep-new")

		ep, ok := svc.GetEndpoint("ep-new")
		if !ok {
			t.Fatal("endpoint not found after registration")
		}
		if ep.HealthScore != svc.config.DefaultHealthScore {
			t.Errorf("HealthScore = %d, want %d", ep.HealthScore, svc.config.DefaultHealthScore)
		}
		if ep.CircuitState != CircuitClosed {
			t.Errorf("CircuitState = %s, want CLOSED", ep.CircuitState)
		}
	})

	t.Run("does not overwrite existing endpoint", func(t *testing.T) {
		addTestEndpoint(t, svc, "ep-existing", 42, 7, CircuitOpen)
		svc.RegisterEndpoint("ep-existing")

		ep, _ := svc.GetEndpoint("ep-existing")
		if ep.HealthScore != 42 {
			t.Errorf("HealthScore = %d, want 42 (should not overwrite)", ep.HealthScore)
		}
	})
}

func TestListEndpoints(t *testing.T) {
	svc := newTestService(t)

	t.Run("empty when no endpoints", func(t *testing.T) {
		eps := svc.ListEndpoints()
		if len(eps) != 0 {
			t.Errorf("ListEndpoints = %d, want 0", len(eps))
		}
	})

	t.Run("returns all registered endpoints", func(t *testing.T) {
		addTestEndpoint(t, svc, "ep-1", 90, 0, CircuitClosed)
		addTestEndpoint(t, svc, "ep-2", 60, 2, CircuitHalfOpen)

		eps := svc.ListEndpoints()
		if len(eps) != 2 {
			t.Errorf("ListEndpoints = %d, want 2", len(eps))
		}
	})
}

func TestStats(t *testing.T) {
	svc := newTestService(t)

	t.Run("empty stats", func(t *testing.T) {
		stats := svc.Stats()
		if stats["total_endpoints"].(int) != 0 {
			t.Errorf("total_endpoints = %v, want 0", stats["total_endpoints"])
		}
	})

	t.Run("categorizes endpoints correctly", func(t *testing.T) {
		addTestEndpoint(t, svc, "healthy-1", 90, 0, CircuitClosed)
		addTestEndpoint(t, svc, "healthy-2", 75, 0, CircuitClosed)
		addTestEndpoint(t, svc, "degraded-1", 60, 2, CircuitHalfOpen)
		addTestEndpoint(t, svc, "failed-1", 30, 5, CircuitOpen)

		stats := svc.Stats()
		if stats["total_endpoints"].(int) != 4 {
			t.Errorf("total_endpoints = %v, want 4", stats["total_endpoints"])
		}
		if stats["healthy_endpoints"].(int) != 2 {
			t.Errorf("healthy_endpoints = %v, want 2", stats["healthy_endpoints"])
		}
		if stats["degraded_endpoints"].(int) != 1 {
			t.Errorf("degraded_endpoints = %v, want 1", stats["degraded_endpoints"])
		}
		if stats["failed_endpoints"].(int) != 1 {
			t.Errorf("failed_endpoints = %v, want 1", stats["failed_endpoints"])
		}
		if stats["circuit_closed"].(int) != 2 {
			t.Errorf("circuit_closed = %v, want 2", stats["circuit_closed"])
		}
		if stats["circuit_open"].(int) != 1 {
			t.Errorf("circuit_open = %v, want 1", stats["circuit_open"])
		}
		if stats["circuit_half_open"].(int) != 1 {
			t.Errorf("circuit_half_open = %v, want 1", stats["circuit_half_open"])
		}
	})
}

func TestWotanDrops(t *testing.T) {
	svc := newTestService(t)

	if svc.WotanDrops() != 0 {
		t.Errorf("WotanDrops = %d, want 0", svc.WotanDrops())
	}

	// publishEvent with nil wotan should increment drops
	svc.publishEvent(context.Background(), "test.topic", map[string]interface{}{"key": "value"})

	if svc.WotanDrops() != 1 {
		t.Errorf("WotanDrops = %d, want 1 after dropped event", svc.WotanDrops())
	}
}

// ---------------------------------------------------------------------------
// publishEvent coverage
// ---------------------------------------------------------------------------

func TestPublishEventNilWotan(t *testing.T) {
	svc := newTestService(t)

	svc.publishEvent(context.Background(), "test.topic", map[string]interface{}{
		"endpoint_id": "ep-1",
	})

	if svc.WotanDrops() != 1 {
		t.Errorf("WotanDrops = %d, want 1", svc.WotanDrops())
	}
}

// ---------------------------------------------------------------------------
// healthMonitorLoop / runHealthChecks
// ---------------------------------------------------------------------------

func TestRunHealthChecks(t *testing.T) {
	svc := newTestService(t)
	addTestEndpoint(t, svc, "ep-check-1", 100, 0, CircuitClosed)
	addTestEndpoint(t, svc, "ep-check-2", 100, 0, CircuitClosed)

	// Set some latency data so scores get recalculated
	svc.mu.Lock()
	svc.endpoints["ep-check-1"].LatencyBuckets = [5]uint64{900, 50, 30, 10, 10}
	svc.endpoints["ep-check-2"].LatencyBuckets = [5]uint64{100, 50, 50, 200, 600}
	svc.mu.Unlock()

	svc.runHealthChecks(context.Background())

	ep1, _ := svc.GetEndpoint("ep-check-1")
	ep2, _ := svc.GetEndpoint("ep-check-2")

	if ep1.HealthScore == 0 {
		t.Error("ep-check-1 HealthScore should be recalculated to non-zero")
	}
	if ep2.HealthScore >= ep1.HealthScore {
		t.Errorf("ep-check-2 (%d) should have lower score than ep-check-1 (%d)", ep2.HealthScore, ep1.HealthScore)
	}
}

func TestHealthMonitorLoop(t *testing.T) {
	cfg := &Config{
		HealthCheckInterval: 20 * time.Millisecond,
		DefaultHealthScore:  100,
		WotanTopic:          "network.failover",
	}
	svc := NewService(newTestLogger(), nil, cfg)
	addTestEndpoint(t, svc, "ep-loop", 100, 0, CircuitClosed)

	svc.mu.Lock()
	svc.endpoints["ep-loop"].LatencyBuckets = [5]uint64{500, 200, 100, 100, 100}
	svc.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	go svc.healthMonitorLoop(ctx)

	// Let it run at least one tick
	time.Sleep(50 * time.Millisecond)
	cancel()

	ep, _ := svc.GetEndpoint("ep-loop")
	// After evaluation, health score should reflect actual latency data
	if ep.HealthScore == 0 {
		t.Error("HealthScore should be recalculated after monitor loop tick")
	}
}

// ---------------------------------------------------------------------------
// handleCriticalAlert
// ---------------------------------------------------------------------------

func TestHandleCriticalAlert(t *testing.T) {
	svc := newTestService(t)

	t.Run("nil message is no-op", func(t *testing.T) {
		// Should not panic
		svc.handleCriticalAlert(context.Background(), nil)
	})

	t.Run("non-nil message logs alert", func(t *testing.T) {
		// Should not panic; just exercises the logging path
		svc.handleCriticalAlert(context.Background(), &wotanClient.Message{
			MessageID: "msg-123",
			Topic:     "alerts.critical",
		})
	})
}

// ---------------------------------------------------------------------------
// HTTP handler edge cases (method not allowed, bad request body, etc.)
// ---------------------------------------------------------------------------

func TestHandleEndpointsMethodNotAllowed(t *testing.T) {
	svc := newTestService(t)
	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/endpoints", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/v1/endpoints status = %d, want 405", rec.Code)
	}
}

func TestHandleBackupsMethodNotAllowed(t *testing.T) {
	svc := newTestService(t)
	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/backups", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE /api/v1/backups status = %d, want 405", rec.Code)
	}
}

func TestHandleCircuitBreakersMethodNotAllowed(t *testing.T) {
	svc := newTestService(t)
	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/circuit-breakers", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/v1/circuit-breakers status = %d, want 405", rec.Code)
	}
}

func TestUpdateBackupInvalidBody(t *testing.T) {
	svc := newTestService(t)
	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	t.Run("malformed JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/backups", bytes.NewBufferString("{invalid"))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("POST bad JSON status = %d, want 400", rec.Code)
		}
	})

	t.Run("missing primary_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/backups", bytes.NewBufferString(`{"backups":["b1"]}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("POST missing primary_id status = %d, want 400", rec.Code)
		}
	})
}

func TestHandleEndpointByIDEdgeCases(t *testing.T) {
	svc := newTestService(t)
	addTestEndpoint(t, svc, "ep-byid", 90, 0, CircuitClosed)
	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	t.Run("GET endpoint by ID without action returns health", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/endpoints/ep-byid", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("GET /api/v1/endpoints/ep-byid status = %d, want 200", rec.Code)
		}
	})

	t.Run("unknown action returns 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/endpoints/ep-byid/nonexistent", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("GET unknown action status = %d, want 404", rec.Code)
		}
	})

	t.Run("POST to health returns 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/endpoints/ep-byid/health", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("POST to health action status = %d, want 404", rec.Code)
		}
	})

	t.Run("failover on unknown endpoint returns 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/endpoints/unknown/failover", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("POST failover unknown endpoint status = %d, want 404", rec.Code)
		}
	})
}

func TestHandleMetricsWithMixedEndpoints(t *testing.T) {
	svc := newTestService(t)
	addTestEndpoint(t, svc, "m-healthy", 90, 0, CircuitClosed)
	addTestEndpoint(t, svc, "m-degraded", 55, 2, CircuitHalfOpen)
	addTestEndpoint(t, svc, "m-failed", 20, 8, CircuitOpen)

	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /metrics status = %d, want 200", rec.Code)
	}

	body := rec.Body.String()
	// Check that metrics contain all categories
	for _, want := range []string{
		`selfheal_endpoints_total 3`,
		`category="healthy"`,
		`category="degraded"`,
		`category="failed"`,
		`state="CLOSED"`,
		`state="OPEN"`,
		`state="HALF_OPEN"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics missing expected string %q", want)
		}
	}
}

// ---------------------------------------------------------------------------
// autoFailover edge case: no backup roster
// ---------------------------------------------------------------------------

func TestAutoFailoverNoBackup(t *testing.T) {
	svc := newTestService(t)
	ep := addTestEndpoint(t, svc, "ep-no-roster", 30, 5, CircuitClosed)
	ep.LatencyBuckets = [5]uint64{0, 0, 0, 0, 1000}

	// Evaluate endpoint — should try autoFailover but find no backup
	svc.mu.Lock()
	svc.evaluateEndpoint(context.Background(), "ep-no-roster", ep)
	svc.mu.Unlock()

	if ep.CircuitState != CircuitOpen {
		t.Errorf("CircuitState = %s, want OPEN", ep.CircuitState)
	}
	if ep.LastFailover != nil {
		t.Error("LastFailover should be nil when no backup available")
	}
	events := svc.GetEvents()
	if len(events) != 0 {
		t.Errorf("expected 0 events when no backup, got %d", len(events))
	}
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

func TestCircuitStateConstants(t *testing.T) {
	states := []CircuitState{CircuitClosed, CircuitOpen, CircuitHalfOpen}
	for _, s := range states {
		if s == "" {
			t.Error("circuit state constant should not be empty")
		}
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
