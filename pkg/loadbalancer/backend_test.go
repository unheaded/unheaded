// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package loadbalancer

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestBackendState_String(t *testing.T) {
	tests := []struct {
		state BackendState
		want  string
	}{
		{BackendStateUnknown, "unknown"},
		{BackendStateHealthy, "healthy"},
		{BackendStateUnhealthy, "unhealthy"},
		{BackendStateDraining, "draining"},
		{BackendStateDisabled, "disabled"},
		{BackendState(99), "invalid"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("BackendState(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestNewBackend_Enabled(t *testing.T) {
	b, err := NewBackend(BackendConfig{
		Name:    "b1",
		Address: "127.0.0.1:8080",
		Weight:  10,
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("NewBackend failed: %v", err)
	}
	if b.State() != BackendStateUnknown {
		t.Errorf("expected state unknown, got %s", b.State())
	}
	if b.Config.Name != "b1" {
		t.Errorf("expected name b1, got %s", b.Config.Name)
	}
}

func TestNewBackend_Disabled(t *testing.T) {
	b, err := NewBackend(BackendConfig{
		Name:    "b1",
		Address: "127.0.0.1:8080",
		Enabled: false,
	})
	if err != nil {
		t.Fatalf("NewBackend failed: %v", err)
	}
	if b.State() != BackendStateDisabled {
		t.Errorf("expected state disabled, got %s", b.State())
	}
}

func TestBackend_IsHealthy(t *testing.T) {
	b, _ := NewBackend(BackendConfig{Name: "b1", Address: "127.0.0.1:8080", Enabled: true})

	// Unknown is considered healthy
	if !b.IsHealthy() {
		t.Error("unknown state should be considered healthy")
	}

	b.SetState(BackendStateHealthy)
	if !b.IsHealthy() {
		t.Error("healthy state should be healthy")
	}

	b.SetState(BackendStateUnhealthy)
	if b.IsHealthy() {
		t.Error("unhealthy state should not be healthy")
	}

	b.SetState(BackendStateDraining)
	if b.IsHealthy() {
		t.Error("draining state should not be healthy")
	}

	b.SetState(BackendStateDisabled)
	if b.IsHealthy() {
		t.Error("disabled state should not be healthy")
	}
}

func TestBackend_IsAvailable(t *testing.T) {
	b, _ := NewBackend(BackendConfig{Name: "b1", Address: "127.0.0.1:8080", Enabled: true, MaxConnections: 2})

	if !b.IsAvailable() {
		t.Error("should be available initially")
	}

	b.IncrementConnections()
	b.IncrementConnections()
	if b.IsAvailable() {
		t.Error("should not be available at max connections")
	}

	b.DecrementConnections()
	if !b.IsAvailable() {
		t.Error("should be available after decrement")
	}
}

func TestBackend_IsAvailable_NoLimit(t *testing.T) {
	b, _ := NewBackend(BackendConfig{Name: "b1", Address: "127.0.0.1:8080", Enabled: true, MaxConnections: 0})

	for i := 0; i < 100; i++ {
		b.IncrementConnections()
	}
	if !b.IsAvailable() {
		t.Error("should be available with no connection limit")
	}
}

func TestBackend_ConnectionCounting(t *testing.T) {
	b, _ := NewBackend(BackendConfig{Name: "b1", Address: "127.0.0.1:8080", Enabled: true})

	b.IncrementConnections()
	b.IncrementConnections()
	b.IncrementConnections()

	if b.ActiveConnections() != 3 {
		t.Errorf("expected 3 active connections, got %d", b.ActiveConnections())
	}
	if b.TotalConnections() != 3 {
		t.Errorf("expected 3 total connections, got %d", b.TotalConnections())
	}

	b.DecrementConnections()
	if b.ActiveConnections() != 2 {
		t.Errorf("expected 2 active connections, got %d", b.ActiveConnections())
	}
	// Total should not decrease
	if b.TotalConnections() != 3 {
		t.Errorf("expected 3 total connections, got %d", b.TotalConnections())
	}
}

func TestBackend_RequestCounting(t *testing.T) {
	b, _ := NewBackend(BackendConfig{Name: "b1", Address: "127.0.0.1:8080", Enabled: true})

	b.IncrementRequests()
	b.IncrementRequests()

	if b.TotalRequests() != 2 {
		t.Errorf("expected 2 total requests, got %d", b.TotalRequests())
	}
}

func TestBackend_ErrorRecording(t *testing.T) {
	b, _ := NewBackend(BackendConfig{Name: "b1", Address: "127.0.0.1:8080", Enabled: true})

	testErr := errors.New("test error")
	b.RecordError(testErr)
	b.RecordError(testErr)

	if b.TotalErrors() != 2 {
		t.Errorf("expected 2 total errors, got %d", b.TotalErrors())
	}

	if b.LastError() == nil {
		t.Error("expected last error to be set")
	}

	count := b.ErrorCount(time.Minute)
	if count != 2 {
		t.Errorf("expected 2 errors in window, got %d", count)
	}
}

func TestBackend_ErrorCount_WindowExpiry(t *testing.T) {
	b, _ := NewBackend(BackendConfig{Name: "b1", Address: "127.0.0.1:8080", Enabled: true})

	b.RecordError(errors.New("err"))

	// Window of 0 should catch nothing
	count := b.ErrorCount(0)
	if count != 0 {
		t.Errorf("expected 0 errors in zero window, got %d", count)
	}
}

func TestBackend_BytesTracking(t *testing.T) {
	b, _ := NewBackend(BackendConfig{Name: "b1", Address: "127.0.0.1:8080", Enabled: true})

	b.AddBytes(100)
	b.AddBytes(200)

	if b.TotalBytes() != 300 {
		t.Errorf("expected 300 total bytes, got %d", b.TotalBytes())
	}
}

func TestBackend_ResponseTimeRecording(t *testing.T) {
	b, _ := NewBackend(BackendConfig{Name: "b1", Address: "127.0.0.1:8080", Enabled: true})

	b.RecordResponseTime(10 * time.Millisecond)
	b.RecordResponseTime(20 * time.Millisecond)

	stats := b.Stats()
	if stats.AvgResponseTime == 0 {
		t.Error("expected non-zero avg response time")
	}
}

func TestBackend_SetHealthCheckResult_Healthy(t *testing.T) {
	b, _ := NewBackend(BackendConfig{Name: "b1", Address: "127.0.0.1:8080", Enabled: true})

	hc := HealthCheckConfig{Retries: 3, SuccessThreshold: 2}

	b.SetHealthCheckResult(true, hc)
	if b.ConsecutiveSuccesses() != 1 {
		t.Errorf("expected 1 consecutive success, got %d", b.ConsecutiveSuccesses())
	}

	b.SetHealthCheckResult(true, hc)
	if b.State() != BackendStateHealthy {
		t.Errorf("expected healthy after meeting threshold, got %s", b.State())
	}
}

func TestBackend_SetHealthCheckResult_Unhealthy(t *testing.T) {
	b, _ := NewBackend(BackendConfig{Name: "b1", Address: "127.0.0.1:8080", Enabled: true})
	b.SetState(BackendStateHealthy)

	hc := HealthCheckConfig{Retries: 2, SuccessThreshold: 1}

	b.SetHealthCheckResult(false, hc)
	if b.ConsecutiveFailures() != 1 {
		t.Errorf("expected 1 consecutive failure, got %d", b.ConsecutiveFailures())
	}

	b.SetHealthCheckResult(false, hc)
	if b.State() != BackendStateUnhealthy {
		t.Errorf("expected unhealthy after meeting threshold, got %s", b.State())
	}
}

func TestBackend_SetHealthCheckResult_DrainingNotOverridden(t *testing.T) {
	b, _ := NewBackend(BackendConfig{Name: "b1", Address: "127.0.0.1:8080", Enabled: true})
	b.SetState(BackendStateDraining)

	hc := HealthCheckConfig{Retries: 1, SuccessThreshold: 1}

	// Healthy result should not change draining state
	b.SetHealthCheckResult(true, hc)
	if b.State() != BackendStateDraining {
		t.Errorf("expected draining state preserved, got %s", b.State())
	}
}

func TestBackend_SetHealthCheckResult_DisabledNotOverridden(t *testing.T) {
	b, _ := NewBackend(BackendConfig{Name: "b1", Address: "127.0.0.1:8080", Enabled: false})

	hc := HealthCheckConfig{Retries: 1, SuccessThreshold: 1}

	// Failure should not override disabled
	b.SetHealthCheckResult(false, hc)
	if b.State() != BackendStateDisabled {
		t.Errorf("expected disabled state preserved, got %s", b.State())
	}
}

func TestBackend_LastHealthCheck(t *testing.T) {
	b, _ := NewBackend(BackendConfig{Name: "b1", Address: "127.0.0.1:8080", Enabled: true})

	// Initially zero
	if !b.LastHealthCheck().IsZero() {
		t.Error("expected zero time initially")
	}

	hc := HealthCheckConfig{Retries: 1, SuccessThreshold: 1}
	b.SetHealthCheckResult(true, hc)

	if b.LastHealthCheck().IsZero() {
		t.Error("expected non-zero time after health check")
	}
}

func TestBackend_LastError_Initially(t *testing.T) {
	b, _ := NewBackend(BackendConfig{Name: "b1", Address: "127.0.0.1:8080", Enabled: true})
	if b.LastError() != nil {
		t.Error("expected nil last error initially")
	}
}

func TestBackend_DrainAndIsDrained(t *testing.T) {
	b, _ := NewBackend(BackendConfig{Name: "b1", Address: "127.0.0.1:8080", Enabled: true})

	b.IncrementConnections()
	b.Drain()

	if b.State() != BackendStateDraining {
		t.Errorf("expected draining, got %s", b.State())
	}
	if b.IsDrained() {
		t.Error("should not be drained with active connections")
	}

	b.DecrementConnections()
	if !b.IsDrained() {
		t.Error("should be drained with no active connections")
	}
}

func TestBackend_EnableDisable(t *testing.T) {
	b, _ := NewBackend(BackendConfig{Name: "b1", Address: "127.0.0.1:8080", Enabled: true})

	b.Disable()
	if b.State() != BackendStateDisabled {
		t.Errorf("expected disabled, got %s", b.State())
	}

	b.Enable()
	if b.State() != BackendStateUnknown {
		t.Errorf("expected unknown after enable, got %s", b.State())
	}
}

func TestBackend_EnableFromDraining(t *testing.T) {
	b, _ := NewBackend(BackendConfig{Name: "b1", Address: "127.0.0.1:8080", Enabled: true})
	b.Drain()

	b.Enable()
	if b.State() != BackendStateUnknown {
		t.Errorf("expected unknown after enable from draining, got %s", b.State())
	}
}

func TestBackend_EnableFromHealthy_NoChange(t *testing.T) {
	b, _ := NewBackend(BackendConfig{Name: "b1", Address: "127.0.0.1:8080", Enabled: true})
	b.SetState(BackendStateHealthy)

	b.Enable()
	// Should not change state when already healthy
	if b.State() != BackendStateHealthy {
		t.Errorf("expected healthy state preserved, got %s", b.State())
	}
}

func TestBackend_Stats(t *testing.T) {
	b, _ := NewBackend(BackendConfig{Name: "b1", Address: "127.0.0.1:8080", Enabled: true})

	b.SetState(BackendStateHealthy)
	b.IncrementConnections()
	b.IncrementRequests()
	b.AddBytes(1024)
	b.RecordResponseTime(50 * time.Millisecond)

	stats := b.Stats()
	if stats.Name != "b1" {
		t.Errorf("expected name b1, got %s", stats.Name)
	}
	if stats.Address != "127.0.0.1:8080" {
		t.Errorf("expected address, got %s", stats.Address)
	}
	if stats.State != BackendStateHealthy {
		t.Errorf("expected healthy, got %s", stats.State)
	}
	if stats.ActiveConnections != 1 {
		t.Errorf("expected 1 active conn, got %d", stats.ActiveConnections)
	}
	if stats.TotalRequests != 1 {
		t.Errorf("expected 1 total request, got %d", stats.TotalRequests)
	}
	if stats.TotalBytes != 1024 {
		t.Errorf("expected 1024 bytes, got %d", stats.TotalBytes)
	}
}

func TestBackend_CleanupErrors(t *testing.T) {
	b, _ := NewBackend(BackendConfig{Name: "b1", Address: "127.0.0.1:8080", Enabled: true})

	b.RecordError(errors.New("err1"))
	b.RecordError(errors.New("err2"))

	// Cleanup with a very large window should keep all errors
	b.CleanupErrors(time.Hour)
	if b.ErrorCount(time.Hour) != 2 {
		t.Errorf("expected 2 errors after cleanup with large window, got %d", b.ErrorCount(time.Hour))
	}
}

func TestBackend_ConcurrentAccess(t *testing.T) {
	b, _ := NewBackend(BackendConfig{Name: "b1", Address: "127.0.0.1:8080", Enabled: true})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.IncrementConnections()
			b.IncrementRequests()
			b.AddBytes(100)
			b.RecordResponseTime(time.Millisecond)
			_ = b.ActiveConnections()
			_ = b.TotalConnections()
			_ = b.TotalRequests()
			_ = b.TotalBytes()
			_ = b.IsHealthy()
			_ = b.IsAvailable()
			_ = b.Stats()
			b.DecrementConnections()
		}()
	}
	wg.Wait()

	if b.ActiveConnections() != 0 {
		t.Errorf("expected 0 active connections, got %d", b.ActiveConnections())
	}
	if b.TotalConnections() != 100 {
		t.Errorf("expected 100 total connections, got %d", b.TotalConnections())
	}
}

// --- ResponseTimeHistogram tests ---

func TestResponseTimeHistogram_Empty(t *testing.T) {
	h := NewResponseTimeHistogram()
	avg, p50, p95, p99 := h.Stats()
	if avg != 0 || p50 != 0 || p95 != 0 || p99 != 0 {
		t.Error("expected all zeros for empty histogram")
	}
}

func TestResponseTimeHistogram_Observe(t *testing.T) {
	h := NewResponseTimeHistogram()

	for i := 0; i < 100; i++ {
		h.Observe(10 * time.Millisecond)
	}

	avg, p50, p95, p99 := h.Stats()
	if avg != 10*time.Millisecond {
		t.Errorf("expected avg 10ms, got %v", avg)
	}
	if p50 == 0 {
		t.Error("expected non-zero p50")
	}
	_ = p95
	_ = p99
}

func TestResponseTimeHistogram_LargeValues(t *testing.T) {
	h := NewResponseTimeHistogram()
	h.Observe(10 * time.Second)

	_, _, _, _ = h.Stats()
	// Just verify no panic
}

// --- RateCounter tests ---

func TestRateCounter_Basic(t *testing.T) {
	rc := NewRateCounter(time.Minute)

	rc.Add(10)
	rc.Add(20)

	rate := rc.Rate()
	if rate <= 0 {
		t.Errorf("expected positive rate, got %f", rate)
	}
}

func TestRateCounter_ZeroWindow(t *testing.T) {
	rc := NewRateCounter(0)
	rc.Add(10)

	rate := rc.Rate()
	if rate != 0 {
		t.Errorf("expected 0 rate for zero window, got %f", rate)
	}
}

// --- BackendPool tests ---

func makePoolConfig(n int) *Config {
	c := DefaultConfig()
	for i := 0; i < n; i++ {
		c.Backends = append(c.Backends, BackendConfig{
			Name:    fmt.Sprintf("b%d", i),
			Address: fmt.Sprintf("127.0.0.1:%d", 8080+i),
			Weight:  1,
			Enabled: true,
		})
	}
	return c
}

func TestNewBackendPool(t *testing.T) {
	c := makePoolConfig(3)
	pool, err := NewBackendPool(c)
	if err != nil {
		t.Fatalf("NewBackendPool failed: %v", err)
	}

	all := pool.All()
	if len(all) != 3 {
		t.Errorf("expected 3 backends, got %d", len(all))
	}
}

func TestBackendPool_Get(t *testing.T) {
	c := makePoolConfig(2)
	pool, _ := NewBackendPool(c)

	b, ok := pool.Get("b0")
	if !ok {
		t.Error("expected to find b0")
	}
	if b.Config.Name != "b0" {
		t.Errorf("expected name b0, got %s", b.Config.Name)
	}

	_, ok = pool.Get("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent")
	}
}

func TestBackendPool_Healthy(t *testing.T) {
	c := makePoolConfig(3)
	pool, _ := NewBackendPool(c)

	// All start as unknown (healthy)
	healthy := pool.Healthy()
	if len(healthy) != 3 {
		t.Errorf("expected 3 healthy, got %d", len(healthy))
	}

	// Mark one unhealthy
	b, _ := pool.Get("b1")
	b.SetState(BackendStateUnhealthy)

	healthy = pool.Healthy()
	if len(healthy) != 2 {
		t.Errorf("expected 2 healthy, got %d", len(healthy))
	}
}

func TestBackendPool_Select(t *testing.T) {
	c := makePoolConfig(3)
	pool, _ := NewBackendPool(c)

	b, err := pool.Select(context.Background(), "key")
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}
	if b == nil {
		t.Error("expected non-nil backend")
	}
}

func TestBackendPool_Select_NoHealthy(t *testing.T) {
	c := makePoolConfig(2)
	pool, _ := NewBackendPool(c)

	// Mark all unhealthy
	for _, b := range pool.All() {
		b.SetState(BackendStateUnhealthy)
	}

	_, err := pool.Select(context.Background(), "key")
	if !errors.Is(err, ErrNoHealthyBackends) {
		t.Errorf("expected ErrNoHealthyBackends, got: %v", err)
	}
}

func TestBackendPool_Add(t *testing.T) {
	c := makePoolConfig(1)
	pool, _ := NewBackendPool(c)

	err := pool.Add(BackendConfig{Name: "new", Address: "127.0.0.1:9090", Enabled: true})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	if len(pool.All()) != 2 {
		t.Errorf("expected 2 backends, got %d", len(pool.All()))
	}
}

func TestBackendPool_Add_Duplicate(t *testing.T) {
	c := makePoolConfig(1)
	pool, _ := NewBackendPool(c)

	err := pool.Add(BackendConfig{Name: "b0", Address: "127.0.0.1:9090", Enabled: true})
	if err == nil {
		t.Error("expected error for duplicate name")
	}
}

func TestBackendPool_Remove(t *testing.T) {
	c := makePoolConfig(2)
	pool, _ := NewBackendPool(c)

	err := pool.Remove("b0")
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	if len(pool.All()) != 1 {
		t.Errorf("expected 1 backend, got %d", len(pool.All()))
	}

	_, ok := pool.Get("b0")
	if ok {
		t.Error("expected b0 to be removed")
	}
}

func TestBackendPool_Remove_NotFound(t *testing.T) {
	c := makePoolConfig(1)
	pool, _ := NewBackendPool(c)

	err := pool.Remove("nonexistent")
	if err == nil {
		t.Error("expected error for not found")
	}
}

func TestBackendPool_UpdateWeight(t *testing.T) {
	c := makePoolConfig(1)
	pool, _ := NewBackendPool(c)

	err := pool.UpdateWeight("b0", 50)
	if err != nil {
		t.Fatalf("UpdateWeight failed: %v", err)
	}

	b, _ := pool.Get("b0")
	if b.Config.Weight != 50 {
		t.Errorf("expected weight 50, got %d", b.Config.Weight)
	}
}

func TestBackendPool_UpdateWeight_NotFound(t *testing.T) {
	c := makePoolConfig(1)
	pool, _ := NewBackendPool(c)

	err := pool.UpdateWeight("nonexistent", 50)
	if err == nil {
		t.Error("expected error for not found")
	}
}

func TestBackendPool_UpdateWeight_Invalid(t *testing.T) {
	c := makePoolConfig(1)
	pool, _ := NewBackendPool(c)

	if err := pool.UpdateWeight("b0", -1); err == nil {
		t.Error("expected error for negative weight")
	}
	if err := pool.UpdateWeight("b0", 101); err == nil {
		t.Error("expected error for weight > 100")
	}
}

func TestBackendPool_Stats(t *testing.T) {
	c := makePoolConfig(3)
	pool, _ := NewBackendPool(c)

	// Set various states
	b0, _ := pool.Get("b0")
	b0.SetState(BackendStateHealthy)
	b0.IncrementRequests()
	b0.AddBytes(100)

	b1, _ := pool.Get("b1")
	b1.SetState(BackendStateUnhealthy)

	b2, _ := pool.Get("b2")
	b2.SetState(BackendStateDraining)

	stats := pool.Stats()
	if stats.TotalBackends != 3 {
		t.Errorf("expected 3 total, got %d", stats.TotalBackends)
	}
	if stats.HealthyBackends != 1 {
		t.Errorf("expected 1 healthy, got %d", stats.HealthyBackends)
	}
	if stats.UnhealthyBackends != 1 {
		t.Errorf("expected 1 unhealthy, got %d", stats.UnhealthyBackends)
	}
	if stats.DrainingBackends != 1 {
		t.Errorf("expected 1 draining, got %d", stats.DrainingBackends)
	}
	if stats.TotalRequests != 1 {
		t.Errorf("expected 1 request, got %d", stats.TotalRequests)
	}
	if stats.TotalBytes != 100 {
		t.Errorf("expected 100 bytes, got %d", stats.TotalBytes)
	}
}

// --- ConnectionPool tests ---

func TestNewConnectionPool(t *testing.T) {
	b, _ := NewBackend(BackendConfig{Name: "b1", Address: "127.0.0.1:8080", Enabled: true})
	pool := NewConnectionPool(b, 10, time.Minute)

	if pool.Size() != 0 {
		t.Errorf("expected empty pool, got size %d", pool.Size())
	}
}

func TestConnectionPool_Close(t *testing.T) {
	b, _ := NewBackend(BackendConfig{Name: "b1", Address: "127.0.0.1:8080", Enabled: true})
	pool := NewConnectionPool(b, 10, time.Minute)

	pool.Close()
	if pool.Size() != 0 {
		t.Errorf("expected empty pool after close, got %d", pool.Size())
	}
}
