// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package qos

import (
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
			ServiceID:      "web-frontend",
			Class:          10,
			Weight:         4,
			RateLimitMbps:  100.0,
			BurstBytes:     65536,
			TargetLatencyMS: 5,
			IntervalMS:     100,
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
