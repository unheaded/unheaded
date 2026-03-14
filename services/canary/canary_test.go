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

// ---------------------------------------------------------------------------
// TestDefaultSLOThreshold
// ---------------------------------------------------------------------------

func TestDefaultSLOThreshold(t *testing.T) {
	slo := DefaultSLOThreshold()
	if slo == nil {
		t.Fatal("DefaultSLOThreshold returned nil")
	}
	if slo.MaxErrorRatePPM != 5000 {
		t.Errorf("MaxErrorRatePPM = %d, want 5000", slo.MaxErrorRatePPM)
	}
	if slo.MaxLatencyP99MS != 500.0 {
		t.Errorf("MaxLatencyP99MS = %f, want 500.0", slo.MaxLatencyP99MS)
	}
	if slo.MinThroughputPPS != 10.0 {
		t.Errorf("MinThroughputPPS = %f, want 10.0", slo.MinThroughputPPS)
	}
}

// ---------------------------------------------------------------------------
// TestDefaultConfig
// ---------------------------------------------------------------------------

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig returned nil")
	}
	if cfg.WotanTopic != "monad.canary.metrics" {
		t.Errorf("WotanTopic = %q, want %q", cfg.WotanTopic, "monad.canary.metrics")
	}
}

// ---------------------------------------------------------------------------
// TestGetDeployment
// ---------------------------------------------------------------------------

func TestGetDeployment(t *testing.T) {
	svc := newTestService(t)
	addTestDeployment(t, svc, "svc-get", "v2", "v1", StateProbing, 5)

	t.Run("existing deployment", func(t *testing.T) {
		d, ok := svc.GetDeployment("svc-get")
		if !ok {
			t.Fatal("expected to find deployment")
		}
		if d.ServiceID != "svc-get" {
			t.Errorf("ServiceID = %q, want %q", d.ServiceID, "svc-get")
		}
	})

	t.Run("nonexistent deployment", func(t *testing.T) {
		_, ok := svc.GetDeployment("nonexistent")
		if ok {
			t.Error("expected not to find nonexistent deployment")
		}
	})
}

// ---------------------------------------------------------------------------
// TestListDeployments
// ---------------------------------------------------------------------------

func TestListDeployments(t *testing.T) {
	svc := newTestService(t)

	t.Run("empty list", func(t *testing.T) {
		list := svc.ListDeployments()
		if len(list) != 0 {
			t.Errorf("expected 0 deployments, got %d", len(list))
		}
	})

	t.Run("populated list", func(t *testing.T) {
		addTestDeployment(t, svc, "svc-1", "v2", "v1", StateProbing, 5)
		addTestDeployment(t, svc, "svc-2", "v3", "v2", StateRamp, 40)
		list := svc.ListDeployments()
		if len(list) != 2 {
			t.Errorf("expected 2 deployments, got %d", len(list))
		}
	})
}

// ---------------------------------------------------------------------------
// TestStats
// ---------------------------------------------------------------------------

func TestStats(t *testing.T) {
	svc := newTestService(t)
	addTestDeployment(t, svc, "s1", "v2", "v1", StateProbing, 5)
	addTestDeployment(t, svc, "s2", "v3", "v2", StateRamp, 40)
	addTestDeployment(t, svc, "s3", "v4", "v3", StateStable, 100)
	addTestDeployment(t, svc, "s4", "v5", "v4", StateRollback, 0)

	stats := svc.Stats()
	if stats["total_deployments"] != 4 {
		t.Errorf("total_deployments = %v, want 4", stats["total_deployments"])
	}
	if stats["probing_deployments"] != 1 {
		t.Errorf("probing_deployments = %v, want 1", stats["probing_deployments"])
	}
	if stats["ramp_deployments"] != 1 {
		t.Errorf("ramp_deployments = %v, want 1", stats["ramp_deployments"])
	}
	if stats["stable_deployments"] != 1 {
		t.Errorf("stable_deployments = %v, want 1", stats["stable_deployments"])
	}
	if stats["rollback_deployments"] != 1 {
		t.Errorf("rollback_deployments = %v, want 1", stats["rollback_deployments"])
	}
}

// ---------------------------------------------------------------------------
// TestWotanDrops
// ---------------------------------------------------------------------------

func TestWotanDrops(t *testing.T) {
	svc := newTestService(t)

	if drops := svc.WotanDrops(); drops != 0 {
		t.Errorf("initial WotanDrops = %d, want 0", drops)
	}

	// publishEvent with nil wotan should increment drops
	svc.publishEvent(context.Background(), "test.topic", map[string]interface{}{"key": "value"})
	if drops := svc.WotanDrops(); drops != 1 {
		t.Errorf("after publish WotanDrops = %d, want 1", drops)
	}

	svc.publishEvent(context.Background(), "test.topic", map[string]interface{}{"key": "value2"})
	if drops := svc.WotanDrops(); drops != 2 {
		t.Errorf("after second publish WotanDrops = %d, want 2", drops)
	}
}

// ---------------------------------------------------------------------------
// TestHTTPCreateCanaryValidation
// ---------------------------------------------------------------------------

func TestHTTPCreateCanaryValidation(t *testing.T) {
	svc := newTestService(t)
	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "missing service_id",
			body:       `{"canary_version":"v2","stable_version":"v1"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing canary_version",
			body:       `{"service_id":"svc","stable_version":"v1"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing stable_version",
			body:       `{"service_id":"svc","canary_version":"v2"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid JSON",
			body:       `not json`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "valid with default split",
			body:       `{"service_id":"svc-default","canary_version":"v2","stable_version":"v1"}`,
			wantStatus: http.StatusCreated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/canaries", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}

	// Test duplicate creation (conflict)
	t.Run("duplicate service_id returns conflict", func(t *testing.T) {
		body := `{"service_id":"svc-default","canary_version":"v3","stable_version":"v2"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/canaries", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusConflict {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
		}
	})
}

// ---------------------------------------------------------------------------
// TestHTTPCanariesMethodNotAllowed
// ---------------------------------------------------------------------------

func TestHTTPCanariesMethodNotAllowed(t *testing.T) {
	svc := newTestService(t)
	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/canaries", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT /api/v1/canaries status = %d, want 405", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// TestHTTPGetCanaryByID
// ---------------------------------------------------------------------------

func TestHTTPGetCanaryByID(t *testing.T) {
	svc := newTestService(t)
	addTestDeployment(t, svc, "svc-get-http", "v2", "v1", StateProbing, 5)
	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	t.Run("existing canary", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/canaries/svc-get-http", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		var d CanaryDeployment
		if err := json.NewDecoder(rec.Body).Decode(&d); err != nil {
			t.Fatalf("decode error: %v", err)
		}
		if d.ServiceID != "svc-get-http" {
			t.Errorf("ServiceID = %q, want %q", d.ServiceID, "svc-get-http")
		}
	})

	t.Run("nonexistent canary", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/canaries/nonexistent", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})

	t.Run("unknown action", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/canaries/svc-get-http/unknown", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// TestHTTPGetCanaryMetrics
// ---------------------------------------------------------------------------

func TestHTTPGetCanaryMetrics(t *testing.T) {
	svc := newTestService(t)
	addTestDeployment(t, svc, "svc-metrics", "v2", "v1", StateProbing, 5)
	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	t.Run("metrics for existing canary (no stored comparison)", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/canaries/svc-metrics/metrics", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		var mc MetricsComparison
		if err := json.NewDecoder(rec.Body).Decode(&mc); err != nil {
			t.Fatalf("decode error: %v", err)
		}
		if mc.ServiceID != "svc-metrics" {
			t.Errorf("ServiceID = %q, want %q", mc.ServiceID, "svc-metrics")
		}
		if !mc.Healthy {
			t.Error("expected Healthy=true for default metrics")
		}
	})

	t.Run("metrics for nonexistent canary", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/canaries/nonexistent/metrics", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// TestHTTPPromoteCanary
// ---------------------------------------------------------------------------

func TestHTTPPromoteCanary(t *testing.T) {
	svc := newTestService(t)
	addTestDeployment(t, svc, "svc-promote", "v2", "v1", StateRamp, 50)
	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	t.Run("promote existing canary", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/canaries/svc-promote/promote", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		d, ok := svc.GetDeployment("svc-promote")
		if !ok {
			t.Fatal("deployment not found after promote")
		}
		if d.State != StateStable {
			t.Errorf("State = %s, want STABLE", d.State)
		}
		if d.TrafficSplitPct != 100 {
			t.Errorf("TrafficSplitPct = %d, want 100", d.TrafficSplitPct)
		}
		if d.CompletedAt == nil {
			t.Error("CompletedAt should be set after promote")
		}
	})

	t.Run("promote nonexistent canary", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/canaries/nonexistent/promote", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// TestHTTPRollbackCanary
// ---------------------------------------------------------------------------

func TestHTTPRollbackCanary(t *testing.T) {
	svc := newTestService(t)
	addTestDeployment(t, svc, "svc-rollback", "v2", "v1", StateRamp, 50)
	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	t.Run("rollback existing canary", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/canaries/svc-rollback/rollback", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		d, ok := svc.GetDeployment("svc-rollback")
		if !ok {
			t.Fatal("deployment not found after rollback")
		}
		if d.State != StateRollback {
			t.Errorf("State = %s, want ROLLBACK", d.State)
		}
		if d.TrafficSplitPct != 0 {
			t.Errorf("TrafficSplitPct = %d, want 0", d.TrafficSplitPct)
		}
		if d.RollbackReason != "manual_rollback" {
			t.Errorf("RollbackReason = %q, want %q", d.RollbackReason, "manual_rollback")
		}
	})

	t.Run("rollback nonexistent canary", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/canaries/nonexistent/rollback", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// TestHTTPAbortCanary
// ---------------------------------------------------------------------------

func TestHTTPAbortCanary(t *testing.T) {
	svc := newTestService(t)
	addTestDeployment(t, svc, "svc-abort", "v2", "v1", StateRamp, 50)
	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	t.Run("abort existing canary", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/canaries/svc-abort", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Errorf("status = %d, want 204", rec.Code)
		}

		_, ok := svc.GetDeployment("svc-abort")
		if ok {
			t.Error("deployment should have been removed after abort")
		}
	})

	t.Run("abort nonexistent canary", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/canaries/nonexistent", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// TestEvaluateAll
// ---------------------------------------------------------------------------

func TestEvaluateAll(t *testing.T) {
	cfg := &Config{
		EvalInterval:    1 * time.Millisecond,
		ProbeDuration:   1 * time.Millisecond,
		RampInterval:    1 * time.Hour, // long ramp so we stay in RAMP
		RampStepPct:     10,
		InitialSplitPct: 5,
	}
	svc := newTestServiceWithConfig(t, cfg)

	// Add deployments in various states
	d1 := addTestDeployment(t, svc, "eval-probing", "v2", "v1", StateProbing, 5)
	d1.ProbeStartTime = time.Now().Add(-1 * time.Hour)

	addTestDeployment(t, svc, "eval-stable", "v2", "v1", StateStable, 100)
	addTestDeployment(t, svc, "eval-rollback", "v2", "v1", StateRollback, 0)

	svc.evaluateAll(context.Background())

	// Probing should have transitioned to RAMP
	d1After, _ := svc.GetDeployment("eval-probing")
	if d1After.State != StateRamp {
		t.Errorf("eval-probing State = %s, want RAMP", d1After.State)
	}

	// Stable and Rollback should remain unchanged (skipped)
	dStable, _ := svc.GetDeployment("eval-stable")
	if dStable.State != StateStable {
		t.Errorf("eval-stable State = %s, want STABLE (should be skipped)", dStable.State)
	}
	dRollback, _ := svc.GetDeployment("eval-rollback")
	if dRollback.State != StateRollback {
		t.Errorf("eval-rollback State = %s, want ROLLBACK (should be skipped)", dRollback.State)
	}
}

// ---------------------------------------------------------------------------
// TestHTTPMetricsContent
// ---------------------------------------------------------------------------

func TestHTTPMetricsContent(t *testing.T) {
	svc := newTestService(t)
	addTestDeployment(t, svc, "m1", "v2", "v1", StateProbing, 5)
	addTestDeployment(t, svc, "m2", "v3", "v2", StateRamp, 50)
	addTestDeployment(t, svc, "m3", "v4", "v3", StateStable, 100)
	addTestDeployment(t, svc, "m4", "v5", "v4", StateRollback, 0)

	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	body := rec.Body.String()
	for _, expected := range []string{
		`canary_deployments_active 4`,
		`canary_deployments_by_state{state="PROBING"} 1`,
		`canary_deployments_by_state{state="RAMP"} 1`,
		`canary_deployments_by_state{state="STABLE"} 1`,
		`canary_deployments_by_state{state="ROLLBACK"} 1`,
		`canary_wotan_drops_total 0`,
	} {
		if !contains(body, expected) {
			t.Errorf("metrics missing %q", expected)
		}
	}
}

// ---------------------------------------------------------------------------
// TestHandleCriticalAlert
// ---------------------------------------------------------------------------

func TestHandleCriticalAlert(t *testing.T) {
	svc := newTestService(t)

	// nil message should not panic
	svc.handleCriticalAlert(context.Background(), nil)

	// non-nil message should not panic
	msg := &wotanClient.Message{
		MessageID: "test-123",
		Topic:     "alerts.critical",
	}
	svc.handleCriticalAlert(context.Background(), msg)
}

// ---------------------------------------------------------------------------
// TestPublishEventNilWotan
// ---------------------------------------------------------------------------

func TestPublishEventNilWotan(t *testing.T) {
	svc := newTestService(t)

	// With nil wotan, publishEvent should increment drops and not panic
	for i := 0; i < 5; i++ {
		svc.publishEvent(context.Background(), "test.topic", map[string]interface{}{
			"iteration": i,
		})
	}

	if drops := svc.WotanDrops(); drops != 5 {
		t.Errorf("WotanDrops = %d, want 5", drops)
	}
}

// ---------------------------------------------------------------------------
// TestHTTPEmptyServiceIDInPath
// ---------------------------------------------------------------------------

func TestHTTPEmptyServiceIDInPath(t *testing.T) {
	svc := newTestService(t)
	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	// The path /api/v1/canaries/ with no service ID should return 400
	req := httptest.NewRequest(http.MethodGet, "/api/v1/canaries/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}
