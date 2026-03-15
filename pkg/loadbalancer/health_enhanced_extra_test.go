// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package loadbalancer

import (
	"testing"
	"time"
)

func TestDefaultHealthCheckLogger(t *testing.T) {
	logger := &DefaultHealthCheckLogger{}
	// Verify no-op methods don't panic
	logger.Debug("test %s", "arg")
	logger.Info("test %s", "arg")
	logger.Warn("test %s", "arg")
	logger.Error("test %s", "arg")
}

func TestLatencyWindow_Empty(t *testing.T) {
	lw := NewLatencyWindow(10)
	avg, min, max, p50, p95, p99 := lw.Stats()
	if avg != 0 || min != 0 || max != 0 || p50 != 0 || p95 != 0 || p99 != 0 {
		t.Error("expected all zeros for empty window")
	}
}

func TestLatencyWindow_Partial(t *testing.T) {
	lw := NewLatencyWindow(100)

	lw.Add(10 * time.Millisecond)
	lw.Add(20 * time.Millisecond)
	lw.Add(30 * time.Millisecond)

	avg, min, max, _, _, _ := lw.Stats()
	if avg == 0 {
		t.Error("expected non-zero avg")
	}
	if min != 10*time.Millisecond {
		t.Errorf("expected min 10ms, got %v", min)
	}
	if max != 30*time.Millisecond {
		t.Errorf("expected max 30ms, got %v", max)
	}
}

func TestLatencyWindow_Filled(t *testing.T) {
	lw := NewLatencyWindow(5)

	for i := 0; i < 10; i++ {
		lw.Add(time.Duration(i+1) * time.Millisecond)
	}

	avg, _, _, _, _, _ := lw.Stats()
	if avg == 0 {
		t.Error("expected non-zero avg for filled window")
	}
}

func TestHealthCheckMetrics_RecordCheck(t *testing.T) {
	m := NewHealthCheckMetrics()

	m.RecordCheck("b1", true, 10*time.Millisecond)
	m.RecordCheck("b1", false, 100*time.Millisecond)
	m.RecordCheck("b1", true, 5*time.Millisecond)

	stats := m.GetStats("b1")
	if stats.TotalChecks != 3 {
		t.Errorf("expected 3 total checks, got %d", stats.TotalChecks)
	}
	if stats.FailedChecks != 1 {
		t.Errorf("expected 1 failed check, got %d", stats.FailedChecks)
	}
}

func TestHealthCheckMetrics_RecordStateTransition(t *testing.T) {
	m := NewHealthCheckMetrics()

	m.RecordStateTransition("b1", BackendStateUnknown, BackendStateHealthy, "health check passed")
	m.RecordStateTransition("b1", BackendStateHealthy, BackendStateUnhealthy, "health check failed")

	stats := m.GetStats("b1")
	_ = stats // Just verify no panic
}

func TestHealthCheckMetrics_RecordStateTransition_Overflow(t *testing.T) {
	m := NewHealthCheckMetrics()

	// Add more than 100 transitions to test truncation
	for i := 0; i < 110; i++ {
		m.RecordStateTransition("b1", BackendStateHealthy, BackendStateUnhealthy, "test")
	}

	// Should still work without panic
	_ = m.GetStats("b1")
}

func TestHealthCheckMetrics_RecordCircuitBreakerTrip(t *testing.T) {
	m := NewHealthCheckMetrics()

	m.RecordCircuitBreakerTrip("b1")
	m.RecordCircuitBreakerTrip("b1")
	m.RecordCircuitBreakerTrip("b1")

	stats := m.GetStats("b1")
	if stats.CircuitBreakerTrips != 3 {
		t.Errorf("expected 3 circuit breaker trips, got %d", stats.CircuitBreakerTrips)
	}
}

func TestHealthCheckMetrics_GetStats_Unknown(t *testing.T) {
	m := NewHealthCheckMetrics()

	stats := m.GetStats("nonexistent")
	if stats.TotalChecks != 0 {
		t.Errorf("expected 0 for unknown backend, got %d", stats.TotalChecks)
	}
}
