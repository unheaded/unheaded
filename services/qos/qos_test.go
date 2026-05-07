// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package qos

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"unheaded/pkg/logger"
)

func newTestLogger() *logger.Logger {
	return logger.New(io.Discard)
}

func TestNewService(t *testing.T) {
	log := newTestLogger()
	svc := NewService(log, nil)

	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	if svc.log != log {
		t.Error("expected logger to be set")
	}
	if svc.policies == nil {
		t.Error("expected policies map to be initialized")
	}
	if svc.stats == nil {
		t.Error("expected stats map to be initialized")
	}
	if len(svc.policies) != 0 {
		t.Error("expected empty policies map")
	}
	if len(svc.stats) != 0 {
		t.Error("expected empty stats map")
	}
}

func TestPolicyUpdate(t *testing.T) {
	log := newTestLogger()
	svc := NewService(log, nil)

	t.Run("set valid policy", func(t *testing.T) {
		p := &QoSPolicy{
			ServiceID:       "web-frontend",
			Class:           10,
			Weight:          4,
			RateLimitMbps:   100.0,
			BurstBytes:      65536,
			TargetLatencyMS: 5,
			IntervalMS:      100,
		}
		err := svc.SetPolicy(p)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got, ok := svc.GetPolicy("web-frontend")
		if !ok {
			t.Fatal("expected to find policy for web-frontend")
		}
		if got.Class != 10 {
			t.Errorf("expected class 10, got %d", got.Class)
		}
		if got.Weight != 4 {
			t.Errorf("expected weight 4, got %d", got.Weight)
		}
		if got.RateLimitMbps != 100.0 {
			t.Errorf("expected rate_limit 100.0, got %f", got.RateLimitMbps)
		}
	})

	t.Run("invalid weight rejected", func(t *testing.T) {
		p := &QoSPolicy{
			ServiceID: "bad-weight",
			Weight:    0,
		}
		err := svc.SetPolicy(p)
		if err == nil {
			t.Error("expected error for weight 0")
		}
	})

	t.Run("weight too high rejected", func(t *testing.T) {
		p := &QoSPolicy{
			ServiceID: "bad-weight-high",
			Weight:    17,
		}
		err := svc.SetPolicy(p)
		if err == nil {
			t.Error("expected error for weight 17")
		}
	})

	t.Run("nil policy rejected", func(t *testing.T) {
		err := svc.SetPolicy(nil)
		if err == nil {
			t.Error("expected error for nil policy")
		}
	})

	t.Run("empty service_id rejected", func(t *testing.T) {
		p := &QoSPolicy{
			ServiceID: "",
			Weight:    1,
		}
		err := svc.SetPolicy(p)
		if err == nil {
			t.Error("expected error for empty service_id")
		}
	})

	t.Run("defaults applied for zero target/interval", func(t *testing.T) {
		p := &QoSPolicy{
			ServiceID:       "default-test",
			Weight:          1,
			TargetLatencyMS: 0,
			IntervalMS:      0,
		}
		err := svc.SetPolicy(p)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got, _ := svc.GetPolicy("default-test")
		if got.TargetLatencyMS != DefaultTargetMS {
			t.Errorf("expected target %d, got %d", DefaultTargetMS, got.TargetLatencyMS)
		}
		if got.IntervalMS != DefaultIntervalMS {
			t.Errorf("expected interval %d, got %d", DefaultIntervalMS, got.IntervalMS)
		}
	})

	t.Run("HTTP POST updates policy", func(t *testing.T) {
		mux := http.NewServeMux()
		svc.registerRoutes(mux)

		body := `{"class": 20, "weight": 8, "rate_limit_mbps": 50.0, "burst_bytes": 32768}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/policy/http-test-svc", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		got, ok := svc.GetPolicy("http-test-svc")
		if !ok {
			t.Fatal("expected policy to be set via HTTP")
		}
		if got.Class != 20 {
			t.Errorf("expected class 20, got %d", got.Class)
		}
		if got.Weight != 8 {
			t.Errorf("expected weight 8, got %d", got.Weight)
		}
	})

	t.Run("GET all policies", func(t *testing.T) {
		mux := http.NewServeMux()
		svc.registerRoutes(mux)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/policy", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		var resp map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp["total"] == nil {
			t.Error("expected total field in response")
		}
	})
}

func TestQueueStatsAggregation(t *testing.T) {
	log := newTestLogger()
	svc := NewService(log, nil)

	// Add a policy and stats.
	_ = svc.SetPolicy(&QoSPolicy{
		ServiceID:       "svc-a",
		Weight:          1,
		TargetLatencyMS: 5,
		IntervalMS:      100,
	})

	svc.UpdateStats(&QueueStats{
		ServiceID:       "svc-a",
		Packets:         10000,
		Drops:           50,
		QueueDepth:      120,
		DropProbability: 0.005,
		LatencyP50:      2.1,
		LatencyP99:      8.5,
		LatencyP999:     15.0,
	})

	st, ok := svc.GetStats("svc-a")
	if !ok {
		t.Fatal("expected stats for svc-a")
	}
	if st.Packets != 10000 {
		t.Errorf("expected 10000 packets, got %d", st.Packets)
	}
	if st.Drops != 50 {
		t.Errorf("expected 50 drops, got %d", st.Drops)
	}
	if st.LatencyP99 != 8.5 {
		t.Errorf("expected p99 8.5, got %f", st.LatencyP99)
	}
	if st.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}

	// Stats for non-existent service.
	_, ok = svc.GetStats("nonexistent")
	if ok {
		t.Error("expected false for nonexistent service stats")
	}

	// Nil and empty stats rejected.
	svc.UpdateStats(nil)
	svc.UpdateStats(&QueueStats{ServiceID: ""})
	// Should not crash or add entries.
}

func TestCongestionDetection(t *testing.T) {
	log := newTestLogger()
	svc := NewService(log, nil)

	_ = svc.SetPolicy(&QoSPolicy{
		ServiceID:       "svc-congested",
		Weight:          1,
		TargetLatencyMS: 5,
		IntervalMS:      100,
	})

	t.Run("normal traffic - circuit closed", func(t *testing.T) {
		svc.UpdateStats(&QueueStats{
			ServiceID:       "svc-congested",
			LatencyP50:      1.0,
			LatencyP99:      4.0,
			LatencyP999:     8.0,
			DropProbability: 0.001,
		})

		info := svc.DetectCongestionExported("svc-congested")
		if info.CircuitState != CircuitClosed {
			t.Errorf("expected CLOSED, got %s", info.CircuitState)
		}
	})

	t.Run("moderate congestion - circuit half open", func(t *testing.T) {
		svc.UpdateStats(&QueueStats{
			ServiceID:       "svc-congested",
			LatencyP50:      5.0,
			LatencyP99:      12.0, // > 2x target (5ms)
			LatencyP999:     20.0,
			DropProbability: 0.05,
		})

		info := svc.DetectCongestionExported("svc-congested")
		if info.CircuitState != CircuitHalfOpen {
			t.Errorf("expected HALF_OPEN, got %s", info.CircuitState)
		}
	})

	t.Run("severe congestion - circuit open", func(t *testing.T) {
		svc.UpdateStats(&QueueStats{
			ServiceID:       "svc-congested",
			LatencyP50:      15.0,
			LatencyP99:      30.0, // > 5x target (5ms)
			LatencyP999:     50.0,
			DropProbability: 0.2,
		})

		info := svc.DetectCongestionExported("svc-congested")
		if info.CircuitState != CircuitOpen {
			t.Errorf("expected OPEN, got %s", info.CircuitState)
		}
	})

	t.Run("high drop probability alone triggers half open", func(t *testing.T) {
		svc.UpdateStats(&QueueStats{
			ServiceID:       "svc-congested",
			LatencyP50:      2.0,
			LatencyP99:      4.0, // Under threshold
			LatencyP999:     6.0,
			DropProbability: 0.15, // > 0.1
		})

		info := svc.DetectCongestionExported("svc-congested")
		if info.CircuitState != CircuitHalfOpen {
			t.Errorf("expected HALF_OPEN due to drop probability, got %s", info.CircuitState)
		}
	})

	t.Run("nonexistent service is closed", func(t *testing.T) {
		info := svc.DetectCongestionExported("nonexistent")
		if info.CircuitState != CircuitClosed {
			t.Errorf("expected CLOSED for nonexistent, got %s", info.CircuitState)
		}
	})
}

func TestHealthEndpoints(t *testing.T) {
	log := newTestLogger()
	svc := NewService(log, nil)

	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	for _, endpoint := range []string{"/health", "/ready"} {
		t.Run(endpoint, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, endpoint, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("expected status 200 for %s, got %d", endpoint, w.Code)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestWotanDrops
// ---------------------------------------------------------------------------

func TestWotanDrops(t *testing.T) {
	svc := NewService(newTestLogger(), nil)

	if drops := svc.WotanDrops(); drops != 0 {
		t.Errorf("initial WotanDrops = %d, want 0", drops)
	}

	// publishPolicyUpdate with nil wotan increments drops
	svc.publishPolicyUpdate("test-svc", &QoSPolicy{
		ServiceID: "test-svc",
		Weight:    1,
	})

	if drops := svc.WotanDrops(); drops != 1 {
		t.Errorf("WotanDrops = %d, want 1", drops)
	}
}

// ---------------------------------------------------------------------------
// TestPublishStatsSnapshot
// ---------------------------------------------------------------------------

func TestPublishStatsSnapshot(t *testing.T) {
	svc := NewService(newTestLogger(), nil)

	t.Run("no stats - no publish", func(t *testing.T) {
		before := svc.WotanDrops()
		svc.publishStatsSnapshot(context.Background())
		after := svc.WotanDrops()
		// With no stats, nothing should be published
		if after != before {
			t.Errorf("WotanDrops changed from %d to %d with no stats", before, after)
		}
	})

	t.Run("with stats and nil wotan - increments drops", func(t *testing.T) {
		svc.UpdateStats(&QueueStats{
			ServiceID: "svc-a",
			Packets:   100,
		})
		before := svc.WotanDrops()
		svc.publishStatsSnapshot(context.Background())
		after := svc.WotanDrops()
		if after != before+1 {
			t.Errorf("WotanDrops = %d, want %d", after, before+1)
		}
	})
}

// ---------------------------------------------------------------------------
// TestHTTPMetrics
// ---------------------------------------------------------------------------

func TestHTTPMetrics(t *testing.T) {
	svc := NewService(newTestLogger(), nil)
	_ = svc.SetPolicy(&QoSPolicy{
		ServiceID: "svc-a",
		Weight:    1,
	})
	svc.UpdateStats(&QueueStats{
		ServiceID: "svc-a",
		Packets:   5000,
		Drops:     100,
	})

	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	for _, expected := range []string{
		"unheaded_qos_policies_total 1",
		"unheaded_qos_services_monitored 1",
		"unheaded_qos_packets_total 5000",
		"unheaded_qos_drops_total 100",
		"unheaded_qos_wotan_drops_total",
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("metrics missing %q in:\n%s", expected, body)
		}
	}
}

// ---------------------------------------------------------------------------
// TestHTTPStatsByService
// ---------------------------------------------------------------------------

func TestHTTPStatsByService(t *testing.T) {
	svc := NewService(newTestLogger(), nil)
	svc.UpdateStats(&QueueStats{
		ServiceID:  "svc-x",
		Packets:    200,
		LatencyP99: 5.5,
	})

	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	t.Run("existing service stats", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/stats/svc-x", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", w.Code)
		}

		var st QueueStats
		if err := json.NewDecoder(w.Body).Decode(&st); err != nil {
			t.Fatalf("decode error: %v", err)
		}
		if st.Packets != 200 {
			t.Errorf("Packets = %d, want 200", st.Packets)
		}
	})

	t.Run("nonexistent service stats", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/stats/nonexistent", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", w.Code)
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/stats/svc-x", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("status = %d, want 405", w.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// TestHTTPStatsAll
// ---------------------------------------------------------------------------

func TestHTTPStatsAll(t *testing.T) {
	svc := NewService(newTestLogger(), nil)
	svc.UpdateStats(&QueueStats{ServiceID: "svc-1", Packets: 100})
	svc.UpdateStats(&QueueStats{ServiceID: "svc-2", Packets: 200})

	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	t.Run("GET returns all stats sorted", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}

		var resp map[string]interface{}
		json.NewDecoder(w.Body).Decode(&resp)
		total := int(resp["total"].(float64))
		if total != 2 {
			t.Errorf("total = %d, want 2", total)
		}
	})

	t.Run("POST not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/stats", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("status = %d, want 405", w.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// TestHTTPCongestion
// ---------------------------------------------------------------------------

func TestHTTPCongestion(t *testing.T) {
	svc := NewService(newTestLogger(), nil)
	_ = svc.SetPolicy(&QoSPolicy{
		ServiceID:       "svc-c1",
		Weight:          2,
		TargetLatencyMS: 5,
	})
	_ = svc.SetPolicy(&QoSPolicy{
		ServiceID:       "svc-c2",
		Weight:          3,
		TargetLatencyMS: 10,
	})
	svc.UpdateStats(&QueueStats{
		ServiceID:  "svc-c1",
		LatencyP99: 3.0,
	})
	svc.UpdateStats(&QueueStats{
		ServiceID:  "svc-c2",
		LatencyP99: 60.0, // > 5x target of 10ms
	})

	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	t.Run("GET returns congestion info", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/congestion", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}

		var resp map[string]interface{}
		json.NewDecoder(w.Body).Decode(&resp)
		total := int(resp["total"].(float64))
		if total != 2 {
			t.Errorf("total = %d, want 2", total)
		}
	})

	t.Run("POST not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/congestion", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("status = %d, want 405", w.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// TestHTTPPolicyByServiceGet
// ---------------------------------------------------------------------------

func TestHTTPPolicyByServiceGet(t *testing.T) {
	svc := NewService(newTestLogger(), nil)
	_ = svc.SetPolicy(&QoSPolicy{
		ServiceID: "get-svc",
		Weight:    4,
		Class:     10,
	})

	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	t.Run("GET existing policy", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/policy/get-svc", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		var p QoSPolicy
		json.NewDecoder(w.Body).Decode(&p)
		if p.Weight != 4 {
			t.Errorf("Weight = %d, want 4", p.Weight)
		}
	})

	t.Run("GET nonexistent policy", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/policy/nonexistent", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", w.Code)
		}
	})

	t.Run("DELETE not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/policy/get-svc", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("status = %d, want 405", w.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// TestHTTPPolicyPostValidation
// ---------------------------------------------------------------------------

func TestHTTPPolicyPostValidation(t *testing.T) {
	svc := NewService(newTestLogger(), nil)

	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "invalid JSON",
			body:       `not json`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid weight",
			body:       `{"weight": 0}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "negative rate limit",
			body:       `{"weight": 5, "rate_limit_mbps": -1.0}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "valid policy",
			body:       `{"weight": 5, "rate_limit_mbps": 100.0}`,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/policy/test-validate", strings.NewReader(tt.body))
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body: %s", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestHTTPPoliciesMethodNotAllowed
// ---------------------------------------------------------------------------

func TestHTTPPoliciesMethodNotAllowed(t *testing.T) {
	svc := NewService(newTestLogger(), nil)
	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/policy", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

// ---------------------------------------------------------------------------
// TestPolicyValidation
// ---------------------------------------------------------------------------

func TestPolicyValidation(t *testing.T) {
	tests := []struct {
		name    string
		policy  QoSPolicy
		wantErr bool
	}{
		{
			name:    "valid policy",
			policy:  QoSPolicy{ServiceID: "svc", Weight: 1},
			wantErr: false,
		},
		{
			name:    "empty service_id",
			policy:  QoSPolicy{ServiceID: "", Weight: 1},
			wantErr: true,
		},
		{
			name:    "weight zero",
			policy:  QoSPolicy{ServiceID: "svc", Weight: 0},
			wantErr: true,
		},
		{
			name:    "weight 17",
			policy:  QoSPolicy{ServiceID: "svc", Weight: 17},
			wantErr: true,
		},
		{
			name:    "negative rate limit",
			policy:  QoSPolicy{ServiceID: "svc", Weight: 1, RateLimitMbps: -1.0},
			wantErr: true,
		},
		{
			name:    "weight 16 (max valid)",
			policy:  QoSPolicy{ServiceID: "svc", Weight: 16},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.policy.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestCongestionDetectionEdgeCases
// ---------------------------------------------------------------------------

func TestCongestionDetectionEdgeCases(t *testing.T) {
	svc := NewService(newTestLogger(), nil)

	t.Run("no stats for service", func(t *testing.T) {
		_ = svc.SetPolicy(&QoSPolicy{
			ServiceID:       "no-stats",
			Weight:          1,
			TargetLatencyMS: 5,
		})
		info := svc.DetectCongestionExported("no-stats")
		if info.CircuitState != CircuitClosed {
			t.Errorf("expected CLOSED, got %s", info.CircuitState)
		}
	})

	t.Run("zero target latency uses default", func(t *testing.T) {
		_ = svc.SetPolicy(&QoSPolicy{
			ServiceID:       "zero-target",
			Weight:          1,
			TargetLatencyMS: 0,
		})
		// After validate, TargetLatencyMS should be DefaultTargetMS
		svc.UpdateStats(&QueueStats{
			ServiceID:  "zero-target",
			LatencyP99: 30.0, // > 5x default 5ms = 25ms
		})
		info := svc.DetectCongestionExported("zero-target")
		if info.CircuitState != CircuitOpen {
			t.Errorf("expected OPEN, got %s", info.CircuitState)
		}
	})
}

// ---------------------------------------------------------------------------
// TestHTTPStatsByServiceEmptyID
// ---------------------------------------------------------------------------

func TestHTTPStatsByServiceEmptyID(t *testing.T) {
	svc := NewService(newTestLogger(), nil)
	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// ---------------------------------------------------------------------------
// TestHTTPPolicyByServiceEmptyID
// ---------------------------------------------------------------------------

func TestHTTPPolicyByServiceEmptyID(t *testing.T) {
	svc := NewService(newTestLogger(), nil)
	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/policy/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}
