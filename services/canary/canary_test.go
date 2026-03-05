// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package canary

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

// newTestServiceWithConfig returns a service with a custom config.
func newTestServiceWithConfig(t *testing.T, cfg *Config) *Service {
	t.Helper()
	return NewService(newTestLogger(), nil, cfg)
}

// addTestDeployment creates and inserts a canary deployment directly.
func addTestDeployment(t *testing.T, svc *Service, serviceID, canaryVer, stableVer string, state CanaryState, splitPct int) *CanaryDeployment {
	t.Helper()
	now := time.Now()
	d := &CanaryDeployment{
		ServiceID:       serviceID,
		CanaryVersion:   canaryVer,
		StableVersion:   stableVer,
		TrafficSplitPct: splitPct,
		State:           state,
		ProbeStartTime:  now,
		LastEvaluation:  now,
		LastRampTime:    now,
		SLO:             DefaultSLOThreshold(),
		CreatedAt:       now,
	}
	svc.mu.Lock()
	svc.deployments[serviceID] = d
	svc.mu.Unlock()
	return d
}

// ---------------------------------------------------------------------------
// TestNewService
// ---------------------------------------------------------------------------

func TestNewService(t *testing.T) {
	t.Run("nil config uses defaults", func(t *testing.T) {
		svc := NewService(newTestLogger(), nil, nil)
		if svc == nil {
			t.Fatal("NewService returned nil")
		}
		if svc.config.EvalInterval != 5*time.Second {
			t.Errorf("EvalInterval = %v, want 5s", svc.config.EvalInterval)
		}
		if svc.config.ProbeDuration != 5*time.Minute {
			t.Errorf("ProbeDuration = %v, want 5m", svc.config.ProbeDuration)
		}
		if svc.config.RampInterval != 2*time.Minute {
			t.Errorf("RampInterval = %v, want 2m", svc.config.RampInterval)
		}
		if svc.config.RampStepPct != 10 {
			t.Errorf("RampStepPct = %d, want 10", svc.config.RampStepPct)
		}
		if svc.config.InitialSplitPct != 5 {
			t.Errorf("InitialSplitPct = %d, want 5", svc.config.InitialSplitPct)
		}
	})

	t.Run("custom config", func(t *testing.T) {
		cfg := &Config{
			EvalInterval:    10 * time.Second,
			ProbeDuration:   1 * time.Minute,
			RampInterval:    30 * time.Second,
			RampStepPct:     20,
			InitialSplitPct: 10,
			WotanTopic:      "custom.topic",
		}
		svc := NewService(newTestLogger(), nil, cfg)
		if svc.config.EvalInterval != 10*time.Second {
			t.Errorf("EvalInterval = %v, want 10s", svc.config.EvalInterval)
		}
		if svc.config.RampStepPct != 20 {
			t.Errorf("RampStepPct = %d, want 20", svc.config.RampStepPct)
		}
	})

	t.Run("deployments map initialized", func(t *testing.T) {
		svc := newTestService(t)
		if svc.deployments == nil {
			t.Fatal("deployments map is nil")
		}
		if len(svc.deployments) != 0 {
			t.Errorf("expected empty deployments, got %d", len(svc.deployments))
		}
	})
}

// ---------------------------------------------------------------------------
// TestCanaryStateMachine
// ---------------------------------------------------------------------------

func TestCanaryStateMachine(t *testing.T) {
	t.Run("initial state is PROBING", func(t *testing.T) {
		svc := newTestService(t)
		d := addTestDeployment(t, svc, "svc-a", "v2", "v1", StateProbing, 5)
		if d.State != StateProbing {
			t.Errorf("State = %s, want PROBING", d.State)
		}
	})

	t.Run("PROBING stays until probe duration elapses", func(t *testing.T) {
		cfg := &Config{
			EvalInterval:    1 * time.Second,
			ProbeDuration:   10 * time.Minute, // long probe
			RampInterval:    1 * time.Minute,
			RampStepPct:     10,
			InitialSplitPct: 5,
		}
		svc := newTestServiceWithConfig(t, cfg)
		d := addTestDeployment(t, svc, "svc-b", "v2", "v1", StateProbing, 5)
		d.ProbeStartTime = time.Now() // just started

		svc.evaluateCanary(context.Background(), d)

		if d.State != StateProbing {
			t.Errorf("State = %s, want PROBING (probe not yet complete)", d.State)
		}
	})

	t.Run("PROBING transitions to RAMP after probe duration", func(t *testing.T) {
		cfg := &Config{
			EvalInterval:    1 * time.Second,
			ProbeDuration:   1 * time.Millisecond, // near instant probe
			RampInterval:    1 * time.Minute,
			RampStepPct:     10,
			InitialSplitPct: 5,
		}
		svc := newTestServiceWithConfig(t, cfg)
		d := addTestDeployment(t, svc, "svc-c", "v2", "v1", StateProbing, 5)
		d.ProbeStartTime = time.Now().Add(-1 * time.Minute) // probe started 1 min ago

		svc.evaluateCanary(context.Background(), d)

		if d.State != StateRamp {
			t.Errorf("State = %s, want RAMP after probe expired", d.State)
		}
	})

	t.Run("RAMP increases traffic split", func(t *testing.T) {
		cfg := &Config{
			EvalInterval:    1 * time.Second,
			ProbeDuration:   1 * time.Millisecond,
			RampInterval:    1 * time.Millisecond, // near instant ramp
			RampStepPct:     10,
			InitialSplitPct: 5,
		}
		svc := newTestServiceWithConfig(t, cfg)
		d := addTestDeployment(t, svc, "svc-d", "v2", "v1", StateRamp, 20)
		d.LastRampTime = time.Now().Add(-1 * time.Minute) // ramp interval elapsed

		svc.evaluateCanary(context.Background(), d)

		if d.TrafficSplitPct != 30 {
			t.Errorf("TrafficSplitPct = %d, want 30", d.TrafficSplitPct)
		}
	})

	t.Run("RAMP transitions to STABLE at 100%", func(t *testing.T) {
		cfg := &Config{
			EvalInterval:    1 * time.Second,
			ProbeDuration:   1 * time.Millisecond,
			RampInterval:    1 * time.Millisecond,
			RampStepPct:     10,
			InitialSplitPct: 5,
		}
		svc := newTestServiceWithConfig(t, cfg)
		d := addTestDeployment(t, svc, "svc-e", "v2", "v1", StateRamp, 95)
		d.LastRampTime = time.Now().Add(-1 * time.Minute)

		svc.evaluateCanary(context.Background(), d)

		if d.State != StateStable {
			t.Errorf("State = %s, want STABLE at 100%%", d.State)
		}
		if d.TrafficSplitPct != 100 {
			t.Errorf("TrafficSplitPct = %d, want 100", d.TrafficSplitPct)
		}
		if d.CompletedAt == nil {
			t.Error("CompletedAt should be set when stable")
		}
	})
}

// ---------------------------------------------------------------------------
// TestSLOEvaluation
// ---------------------------------------------------------------------------

func TestSLOEvaluation(t *testing.T) {
	svc := newTestService(t)

	t.Run("healthy metrics pass SLO", func(t *testing.T) {
		metrics := &CanaryMetrics{
			ServiceID:   "svc-a",
			Version:     "v2",
			ErrorRate:   100.0,  // 100 ppm < 5000 ppm
			LatencyP99:  45.0,   // 45ms < 500ms
			Throughput:  250.0,  // 250 rps > 10 rps
			SampleCount: 1000,
		}
		slo := DefaultSLOThreshold()

		if !svc.checkSLO(metrics, slo) {
			t.Error("healthy metrics should pass SLO check")
		}
	})

	t.Run("high error rate fails SLO", func(t *testing.T) {
		metrics := &CanaryMetrics{
			ServiceID:   "svc-a",
			Version:     "v2",
			ErrorRate:   10000.0, // 10000 ppm > 5000 ppm
			LatencyP99:  45.0,
			Throughput:  250.0,
			SampleCount: 1000,
		}
		slo := DefaultSLOThreshold()

		if svc.checkSLO(metrics, slo) {
			t.Error("high error rate should fail SLO check")
		}
	})

	t.Run("high latency fails SLO", func(t *testing.T) {
		metrics := &CanaryMetrics{
			ServiceID:   "svc-a",
			Version:     "v2",
			ErrorRate:   100.0,
			LatencyP99:  800.0, // 800ms > 500ms
			Throughput:  250.0,
			SampleCount: 1000,
		}
		slo := DefaultSLOThreshold()

		if svc.checkSLO(metrics, slo) {
			t.Error("high latency should fail SLO check")
		}
	})

	t.Run("low throughput fails SLO", func(t *testing.T) {
		metrics := &CanaryMetrics{
			ServiceID:   "svc-a",
			Version:     "v2",
			ErrorRate:   100.0,
			LatencyP99:  45.0,
			Throughput:  5.0, // 5 rps < 10 rps
			SampleCount: 1000,
		}
		slo := DefaultSLOThreshold()

		if svc.checkSLO(metrics, slo) {
			t.Error("low throughput should fail SLO check")
		}
	})

	t.Run("nil SLO passes all", func(t *testing.T) {
		metrics := &CanaryMetrics{
			ServiceID:   "svc-a",
			ErrorRate:   999999.0,
			LatencyP99:  999999.0,
			Throughput:  0.0,
			SampleCount: 1000,
		}

		if !svc.checkSLO(metrics, nil) {
			t.Error("nil SLO should pass all metrics")
		}
	})
}

// ---------------------------------------------------------------------------
// TestAutoRamp
// ---------------------------------------------------------------------------

func TestAutoRamp(t *testing.T) {
	t.Run("ramp through full lifecycle", func(t *testing.T) {
		cfg := &Config{
			EvalInterval:    1 * time.Millisecond,
			ProbeDuration:   1 * time.Millisecond,
			RampInterval:    1 * time.Millisecond,
			RampStepPct:     25,
			InitialSplitPct: 5,
		}
		svc := newTestServiceWithConfig(t, cfg)
		d := addTestDeployment(t, svc, "svc-ramp", "v2", "v1", StateProbing, 5)
		d.ProbeStartTime = time.Now().Add(-1 * time.Hour)
		d.LastRampTime = time.Now().Add(-1 * time.Hour)

		// First eval: PROBING -> RAMP
		svc.evaluateCanary(context.Background(), d)
		if d.State != StateRamp {
			t.Fatalf("State = %s, want RAMP", d.State)
		}

		// Subsequent evals: ramp up by 25% each
		d.LastRampTime = time.Now().Add(-1 * time.Hour)
		svc.evaluateCanary(context.Background(), d)
		if d.TrafficSplitPct != 30 {
			t.Errorf("after first ramp: TrafficSplitPct = %d, want 30", d.TrafficSplitPct)
		}

		d.LastRampTime = time.Now().Add(-1 * time.Hour)
		svc.evaluateCanary(context.Background(), d)
		if d.TrafficSplitPct != 55 {
			t.Errorf("after second ramp: TrafficSplitPct = %d, want 55", d.TrafficSplitPct)
		}

		d.LastRampTime = time.Now().Add(-1 * time.Hour)
		svc.evaluateCanary(context.Background(), d)
		if d.TrafficSplitPct != 80 {
			t.Errorf("after third ramp: TrafficSplitPct = %d, want 80", d.TrafficSplitPct)
		}

		d.LastRampTime = time.Now().Add(-1 * time.Hour)
		svc.evaluateCanary(context.Background(), d)
		if d.State != StateStable {
			t.Errorf("State = %s, want STABLE", d.State)
		}
		if d.TrafficSplitPct != 100 {
			t.Errorf("TrafficSplitPct = %d, want 100", d.TrafficSplitPct)
		}
	})
}

// ---------------------------------------------------------------------------
// TestRollbackOnSLOBreach
// ---------------------------------------------------------------------------

func TestRollbackOnSLOBreach(t *testing.T) {
	t.Run("SLO breach during PROBING triggers rollback", func(t *testing.T) {
		svc := newTestService(t)
		d := addTestDeployment(t, svc, "svc-fail", "v2", "v1", StateProbing, 5)

		// Set SLO with impossibly low error tolerance
		d.SLO = &SLOThreshold{
			MaxErrorRatePPM:  0, // zero tolerance
			MaxLatencyP99MS:  500.0,
			MinThroughputPPS: 10.0,
		}

		svc.evaluateCanary(context.Background(), d)

		// The default simulated metrics have ErrorRate=100 which exceeds 0 tolerance
		if d.State != StateRollback {
			t.Errorf("State = %s, want ROLLBACK after SLO breach", d.State)
		}
		if d.TrafficSplitPct != 0 {
			t.Errorf("TrafficSplitPct = %d, want 0 after rollback", d.TrafficSplitPct)
		}
		if d.RollbackReason != "slo_breach" {
			t.Errorf("RollbackReason = %q, want %q", d.RollbackReason, "slo_breach")
		}
		if d.CompletedAt == nil {
			t.Error("CompletedAt should be set after rollback")
		}
	})

	t.Run("SLO breach during RAMP triggers rollback", func(t *testing.T) {
		svc := newTestService(t)
		d := addTestDeployment(t, svc, "svc-fail-ramp", "v2", "v1", StateRamp, 50)

		// Impossibly tight latency threshold
		d.SLO = &SLOThreshold{
			MaxErrorRatePPM:  5000,
			MaxLatencyP99MS:  1.0, // 1ms max — will breach at 45ms
			MinThroughputPPS: 10.0,
		}

		svc.evaluateCanary(context.Background(), d)

		if d.State != StateRollback {
			t.Errorf("State = %s, want ROLLBACK", d.State)
		}
	})
}

// ---------------------------------------------------------------------------
// HTTP API tests
// ---------------------------------------------------------------------------

func TestHTTPListCanaries(t *testing.T) {
	svc := newTestService(t)
	addTestDeployment(t, svc, "svc-a", "v2", "v1", StateProbing, 5)
	addTestDeployment(t, svc, "svc-b", "v3", "v2", StateRamp, 40)

	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/canaries", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/canaries status = %d, want 200", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	count, ok := resp["count"].(float64)
	if !ok || int(count) != 2 {
		t.Errorf("count = %v, want 2", resp["count"])
	}
}

func TestHTTPCreateCanary(t *testing.T) {
	svc := newTestService(t)

	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	body := `{"service_id":"svc-new","canary_version":"v2","stable_version":"v1","traffic_split_pct":10}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/canaries", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/canaries status = %d, want 201", rec.Code)
	}

	var deployment CanaryDeployment
	if err := json.NewDecoder(rec.Body).Decode(&deployment); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if deployment.ServiceID != "svc-new" {
		t.Errorf("ServiceID = %q, want %q", deployment.ServiceID, "svc-new")
	}
	if deployment.State != StateProbing {
		t.Errorf("State = %s, want PROBING", deployment.State)
	}
	if deployment.TrafficSplitPct != 10 {
		t.Errorf("TrafficSplitPct = %d, want 10", deployment.TrafficSplitPct)
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

		var resp map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&resp)
		if resp["service"] != "canary" {
			t.Errorf("service = %v, want canary", resp["service"])
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
}

func TestHTTPMetrics(t *testing.T) {
	svc := newTestService(t)
	addTestDeployment(t, svc, "svc-a", "v2", "v1", StateProbing, 5)

	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /metrics status = %d, want 200", rec.Code)
	}

	body := rec.Body.String()
	if !contains(body, "canary_deployments_active") {
		t.Error("metrics missing canary_deployments_active")
	}
	if !contains(body, "canary_deployments_by_state") {
		t.Error("metrics missing canary_deployments_by_state")
	}
}

// contains is a helper that checks if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
