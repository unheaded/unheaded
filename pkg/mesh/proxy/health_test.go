// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package proxy

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// ==================== HealthStatus Tests ====================

func TestHealthStatus_String(t *testing.T) {
	tests := []struct {
		status   HealthStatus
		expected string
	}{
		{HealthUnknown, "unknown"},
		{HealthHealthy, "healthy"},
		{HealthUnhealthy, "unhealthy"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.status.String() != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, tt.status.String())
			}
		})
	}
}

// ==================== HealthCheckConfig Tests ====================

func TestDefaultHealthCheckConfig(t *testing.T) {
	config := DefaultHealthCheckConfig()

	if config.Interval != 10*time.Second {
		t.Errorf("expected Interval 10s, got %v", config.Interval)
	}
	if config.Timeout != 5*time.Second {
		t.Errorf("expected Timeout 5s, got %v", config.Timeout)
	}
	if config.HealthyThreshold != 2 {
		t.Errorf("expected HealthyThreshold 2, got %d", config.HealthyThreshold)
	}
	if config.UnhealthyThreshold != 3 {
		t.Errorf("expected UnhealthyThreshold 3, got %d", config.UnhealthyThreshold)
	}
	if config.Path != "/health" {
		t.Errorf("expected Path /health, got %s", config.Path)
	}
	if config.ExpectedStatus != 200 {
		t.Errorf("expected ExpectedStatus 200, got %d", config.ExpectedStatus)
	}
}

// ==================== HealthChecker Tests ====================

func TestNewHealthChecker(t *testing.T) {
	t.Run("WithNilConfig", func(t *testing.T) {
		hc := NewHealthChecker(nil)
		defer hc.Close()

		if hc == nil {
			t.Fatal("expected non-nil health checker")
		}
		if hc.config.Interval != 10*time.Second {
			t.Error("expected default config to be used")
		}
	})

	t.Run("WithCustomConfig", func(t *testing.T) {
		config := &HealthCheckConfig{
			Interval:    5 * time.Second,
			Timeout:     2 * time.Second,
			Path:        "/healthz",
			TCPOnly:     true,
		}

		hc := NewHealthChecker(config)
		defer hc.Close()

		if hc.config.Interval != 5*time.Second {
			t.Errorf("expected interval 5s, got %v", hc.config.Interval)
		}
		if !hc.config.TCPOnly {
			t.Error("expected TCPOnly to be true")
		}
	})
}

func TestHealthChecker_Start_and_Stop(t *testing.T) {
	config := &HealthCheckConfig{
		Interval:           100 * time.Millisecond,
		Timeout:            50 * time.Millisecond,
		HealthyThreshold:   1,
		UnhealthyThreshold: 1,
		TCPOnly:            true,
	}

	hc := NewHealthChecker(config)
	defer hc.Close()

	// Start a test server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer listener.Close()

	address := listener.Addr().String()

	// Accept connections in background
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	hc.Start(address)

	// Wait for health checks
	time.Sleep(200 * time.Millisecond)

	status := hc.GetStatus(address)
	if status != HealthHealthy {
		t.Errorf("expected healthy status, got %v", status)
	}

	hc.Stop(address)

	// Status should return unknown for stopped check
	time.Sleep(50 * time.Millisecond)
	status = hc.GetStatus(address)
	if status != HealthUnknown {
		t.Errorf("expected unknown status after stop, got %v", status)
	}
}

func TestHealthChecker_GetStatus_Unknown(t *testing.T) {
	hc := NewHealthChecker(nil)
	defer hc.Close()

	status := hc.GetStatus("unknown-address:8080")
	if status != HealthUnknown {
		t.Errorf("expected unknown status for untracked address, got %v", status)
	}
}

func TestHealthChecker_IsHealthy(t *testing.T) {
	config := &HealthCheckConfig{
		Interval:           100 * time.Millisecond,
		Timeout:            50 * time.Millisecond,
		HealthyThreshold:   1,
		UnhealthyThreshold: 1,
		TCPOnly:            true,
	}

	hc := NewHealthChecker(config)
	defer hc.Close()

	// Start a test server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer listener.Close()

	address := listener.Addr().String()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	hc.Start(address)
	time.Sleep(200 * time.Millisecond)

	if !hc.IsHealthy(address) {
		t.Error("expected endpoint to be healthy")
	}
}

func TestHealthChecker_OnChange(t *testing.T) {
	config := &HealthCheckConfig{
		Interval:           100 * time.Millisecond,
		Timeout:            50 * time.Millisecond,
		HealthyThreshold:   1,
		UnhealthyThreshold: 1,
		TCPOnly:            true,
	}

	hc := NewHealthChecker(config)
	defer hc.Close()

	var mu sync.Mutex
	var changedAddress string
	var changedStatus HealthStatus

	hc.OnChange(func(address string, status HealthStatus) {
		mu.Lock()
		changedAddress = address
		changedStatus = status
		mu.Unlock()
	})

	// Start server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}

	address := listener.Addr().String()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	hc.Start(address)
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	if changedAddress != address {
		t.Errorf("expected address %s in callback, got %s", address, changedAddress)
	}
	if changedStatus != HealthHealthy {
		t.Errorf("expected healthy status in callback, got %v", changedStatus)
	}
	mu.Unlock()

	listener.Close()
}

func TestHealthChecker_GetAllStatus(t *testing.T) {
	config := &HealthCheckConfig{
		Interval:           100 * time.Millisecond,
		Timeout:            50 * time.Millisecond,
		HealthyThreshold:   1,
		UnhealthyThreshold: 1,
		TCPOnly:            true,
	}

	hc := NewHealthChecker(config)
	defer hc.Close()

	// Start two test servers
	listener1, _ := net.Listen("tcp", "127.0.0.1:0")
	defer listener1.Close()
	listener2, _ := net.Listen("tcp", "127.0.0.1:0")
	defer listener2.Close()

	go func() {
		for {
			conn, err := listener1.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()
	go func() {
		for {
			conn, err := listener2.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	hc.Start(listener1.Addr().String())
	hc.Start(listener2.Addr().String())

	time.Sleep(200 * time.Millisecond)

	allStatus := hc.GetAllStatus()
	if len(allStatus) != 2 {
		t.Errorf("expected 2 status entries, got %d", len(allStatus))
	}
}

func TestHealthChecker_HTTPCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Extract host:port from server URL
	address := server.Listener.Addr().String()

	config := &HealthCheckConfig{
		Interval:           100 * time.Millisecond,
		Timeout:            50 * time.Millisecond,
		HealthyThreshold:   1,
		UnhealthyThreshold: 1,
		Path:               "/health",
		ExpectedStatus:     200,
		TCPOnly:            false,
	}

	hc := NewHealthChecker(config)
	defer hc.Close()

	hc.Start(address)
	time.Sleep(200 * time.Millisecond)

	status := hc.GetStatus(address)
	if status != HealthHealthy {
		t.Errorf("expected healthy status for HTTP check, got %v", status)
	}
}

func TestHealthChecker_HTTPCheck_BadStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	address := server.Listener.Addr().String()

	config := &HealthCheckConfig{
		Interval:           100 * time.Millisecond,
		Timeout:            50 * time.Millisecond,
		HealthyThreshold:   1,
		UnhealthyThreshold: 1,
		Path:               "/health",
		ExpectedStatus:     200,
		TCPOnly:            false,
	}

	hc := NewHealthChecker(config)
	defer hc.Close()

	hc.Start(address)
	time.Sleep(200 * time.Millisecond)

	status := hc.GetStatus(address)
	if status != HealthUnhealthy {
		t.Errorf("expected unhealthy status for bad HTTP status, got %v", status)
	}
}

func TestHealthChecker_TCPCheck_Unreachable(t *testing.T) {
	config := &HealthCheckConfig{
		Interval:           100 * time.Millisecond,
		Timeout:            50 * time.Millisecond,
		HealthyThreshold:   1,
		UnhealthyThreshold: 1,
		TCPOnly:            true,
	}

	hc := NewHealthChecker(config)
	defer hc.Close()

	// Unreachable address
	hc.Start("127.0.0.1:59999")
	time.Sleep(200 * time.Millisecond)

	status := hc.GetStatus("127.0.0.1:59999")
	if status != HealthUnhealthy {
		t.Errorf("expected unhealthy status for unreachable address, got %v", status)
	}
}

// ==================== HealthCheckError Tests ====================

func TestHealthCheckError(t *testing.T) {
	err := &HealthCheckError{
		StatusCode: 503,
		Expected:   200,
	}

	expectedMsg := "health check failed: got 503, expected 200"
	if err.Error() != expectedMsg {
		t.Errorf("expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

// ==================== ActiveHealthChecker Tests ====================

func TestActiveHealthChecker(t *testing.T) {
	t.Run("NewActiveHealthChecker", func(t *testing.T) {
		config := DefaultHealthCheckConfig()
		ahc := NewActiveHealthChecker(config)
		defer ahc.Close()

		if ahc == nil {
			t.Fatal("expected non-nil active health checker")
		}
	})

	t.Run("RegisterProbe", func(t *testing.T) {
		config := DefaultHealthCheckConfig()
		ahc := NewActiveHealthChecker(config)
		defer ahc.Close()

		probe := &mockHealthProbe{healthy: true}
		ahc.RegisterProbe("10.0.0.1:8080", probe)

		ctx := context.Background()
		err := ahc.Check(ctx, "10.0.0.1:8080")
		if err != nil {
			t.Errorf("expected no error from healthy probe, got %v", err)
		}
	})

	t.Run("Check_FallsBackToDefault", func(t *testing.T) {
		config := &HealthCheckConfig{
			Timeout: 50 * time.Millisecond,
			TCPOnly: true,
		}
		ahc := NewActiveHealthChecker(config)
		defer ahc.Close()

		// Start a test server
		listener, _ := net.Listen("tcp", "127.0.0.1:0")
		defer listener.Close()

		go func() {
			for {
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				conn.Close()
			}
		}()

		address := listener.Addr().String()
		ctx := context.Background()
		err := ahc.Check(ctx, address)
		if err != nil {
			t.Errorf("expected no error from TCP check, got %v", err)
		}
	})
}

type mockHealthProbe struct {
	healthy bool
	err     error
}

func (m *mockHealthProbe) Check(ctx context.Context, address string) error {
	if m.err != nil {
		return m.err
	}
	if !m.healthy {
		return &HealthCheckError{StatusCode: 503, Expected: 200}
	}
	return nil
}

// ==================== PassiveHealthConfig Tests ====================

func TestDefaultPassiveHealthConfig(t *testing.T) {
	config := DefaultPassiveHealthConfig()

	if config.FailureThreshold != 5 {
		t.Errorf("expected FailureThreshold 5, got %d", config.FailureThreshold)
	}
	if config.SuccessThreshold != 2 {
		t.Errorf("expected SuccessThreshold 2, got %d", config.SuccessThreshold)
	}
	if config.EjectionTime != 30*time.Second {
		t.Errorf("expected EjectionTime 30s, got %v", config.EjectionTime)
	}
	if config.MaxEjectionPercent != 50.0 {
		t.Errorf("expected MaxEjectionPercent 50.0, got %f", config.MaxEjectionPercent)
	}
}

// ==================== PassiveHealthChecker Tests ====================

func TestPassiveHealthChecker(t *testing.T) {
	t.Run("NewPassiveHealthChecker", func(t *testing.T) {
		phc := NewPassiveHealthChecker(nil)
		if phc == nil {
			t.Fatal("expected non-nil passive health checker")
		}
	})

	t.Run("NewUnknownAddress_IsHealthy", func(t *testing.T) {
		phc := NewPassiveHealthChecker(nil)

		status := phc.GetStatus("unknown-address:8080")
		if status != HealthHealthy {
			t.Errorf("expected unknown address to be healthy by default, got %v", status)
		}
	})

	t.Run("RecordSuccess", func(t *testing.T) {
		phc := NewPassiveHealthChecker(&PassiveHealthConfig{
			FailureThreshold:   3,
			SuccessThreshold:   1,
			EjectionTime:       100 * time.Millisecond,
			MaxEjectionPercent: 100,
		})

		phc.RecordSuccess("10.0.0.1:8080")

		status := phc.GetStatus("10.0.0.1:8080")
		if status != HealthHealthy {
			t.Errorf("expected healthy status after success, got %v", status)
		}
	})

	t.Run("RecordFailure_TriggersEjection", func(t *testing.T) {
		config := &PassiveHealthConfig{
			FailureThreshold:   3,
			SuccessThreshold:   2,
			EjectionTime:       100 * time.Millisecond,
			MaxEjectionPercent: 100,
		}
		phc := NewPassiveHealthChecker(config)

		address := "10.0.0.1:8080"

		// Record failures
		for i := 0; i < 3; i++ {
			phc.RecordFailure(address)
		}

		status := phc.GetStatus(address)
		if status != HealthUnhealthy {
			t.Errorf("expected unhealthy status after failures, got %v", status)
		}
	})

	t.Run("EjectionExpiry", func(t *testing.T) {
		config := &PassiveHealthConfig{
			FailureThreshold:   2,
			SuccessThreshold:   1,
			EjectionTime:       50 * time.Millisecond,
			MaxEjectionPercent: 100,
		}
		phc := NewPassiveHealthChecker(config)

		address := "10.0.0.1:8080"

		// Trigger ejection
		phc.RecordFailure(address)
		phc.RecordFailure(address)

		if phc.GetStatus(address) != HealthUnhealthy {
			t.Fatal("expected unhealthy after failures")
		}

		// Wait for ejection to expire
		time.Sleep(100 * time.Millisecond)

		// Should be healthy again
		status := phc.GetStatus(address)
		if status != HealthHealthy {
			t.Errorf("expected healthy status after ejection expiry, got %v", status)
		}
	})

	t.Run("RecoveryAfterSuccess", func(t *testing.T) {
		config := &PassiveHealthConfig{
			FailureThreshold:   2,
			SuccessThreshold:   2,
			EjectionTime:       1 * time.Hour,
			MaxEjectionPercent: 100,
		}
		phc := NewPassiveHealthChecker(config)

		address := "10.0.0.1:8080"

		// Trigger ejection
		phc.RecordFailure(address)
		phc.RecordFailure(address)

		// Record successes for recovery
		phc.RecordSuccess(address)
		phc.RecordSuccess(address)

		status := phc.GetStatus(address)
		if status != HealthHealthy {
			t.Errorf("expected healthy status after success threshold, got %v", status)
		}
	})

	t.Run("MaxEjectionPercent_LimitsEjections", func(t *testing.T) {
		config := &PassiveHealthConfig{
			FailureThreshold:   1,
			SuccessThreshold:   1,
			EjectionTime:       1 * time.Hour,
			MaxEjectionPercent: 50, // Only 50% can be ejected
		}
		phc := NewPassiveHealthChecker(config)

		// Create two endpoints
		addr1 := "10.0.0.1:8080"
		addr2 := "10.0.0.2:8080"

		// Initialize both as healthy
		phc.RecordSuccess(addr1)
		phc.RecordSuccess(addr2)

		// Try to eject both
		phc.RecordFailure(addr1)
		phc.RecordFailure(addr2)

		// Count unhealthy
		unhealthy := 0
		if phc.GetStatus(addr1) == HealthUnhealthy {
			unhealthy++
		}
		if phc.GetStatus(addr2) == HealthUnhealthy {
			unhealthy++
		}

		// Only one should be ejected (50% of 2)
		if unhealthy > 1 {
			t.Errorf("expected at most 1 ejected endpoint (50%%), got %d", unhealthy)
		}
	})

	t.Run("OnChange_Callback", func(t *testing.T) {
		config := &PassiveHealthConfig{
			FailureThreshold:   1,
			SuccessThreshold:   1,
			EjectionTime:       100 * time.Millisecond,
			MaxEjectionPercent: 100,
		}
		phc := NewPassiveHealthChecker(config)

		var mu sync.Mutex
		var callbackStatus HealthStatus
		var callbackCalled bool

		phc.OnChange(func(address string, status HealthStatus) {
			mu.Lock()
			callbackCalled = true
			callbackStatus = status
			mu.Unlock()
		})

		phc.RecordFailure("10.0.0.1:8080")

		// Allow callback goroutine to run
		time.Sleep(10 * time.Millisecond)

		mu.Lock()
		if !callbackCalled {
			t.Error("expected OnChange callback to be called")
		}
		if callbackStatus != HealthUnhealthy {
			t.Errorf("expected unhealthy status in callback, got %v", callbackStatus)
		}
		mu.Unlock()
	})

	t.Run("SuccessResetsFailureCount", func(t *testing.T) {
		config := &PassiveHealthConfig{
			FailureThreshold:   3,
			SuccessThreshold:   1,
			EjectionTime:       1 * time.Hour,
			MaxEjectionPercent: 100,
		}
		phc := NewPassiveHealthChecker(config)

		address := "10.0.0.1:8080"

		// Two failures
		phc.RecordFailure(address)
		phc.RecordFailure(address)

		// One success resets
		phc.RecordSuccess(address)

		// Two more failures shouldn't trigger ejection
		phc.RecordFailure(address)
		phc.RecordFailure(address)

		if phc.GetStatus(address) != HealthHealthy {
			t.Error("expected healthy - failure count should have reset")
		}

		// One more failure should trigger ejection now
		phc.RecordFailure(address)
		if phc.GetStatus(address) != HealthUnhealthy {
			t.Error("expected unhealthy after 3 consecutive failures")
		}
	})
}

// ==================== Concurrent Access Tests ====================

func TestHealthChecker_ConcurrentAccess(t *testing.T) {
	config := &HealthCheckConfig{
		Interval:           100 * time.Millisecond,
		Timeout:            50 * time.Millisecond,
		HealthyThreshold:   1,
		UnhealthyThreshold: 1,
		TCPOnly:            true,
	}

	hc := NewHealthChecker(config)
	defer hc.Close()

	listener, _ := net.Listen("tcp", "127.0.0.1:0")
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	address := listener.Addr().String()
	hc.Start(address)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hc.GetStatus(address)
			hc.IsHealthy(address)
		}()
	}
	wg.Wait()
}

func TestPassiveHealthChecker_ConcurrentAccess(t *testing.T) {
	config := &PassiveHealthConfig{
		FailureThreshold:   10,
		SuccessThreshold:   5,
		EjectionTime:       100 * time.Millisecond,
		MaxEjectionPercent: 100,
	}
	phc := NewPassiveHealthChecker(config)

	var wg sync.WaitGroup
	addresses := []string{
		"10.0.0.1:8080",
		"10.0.0.2:8080",
		"10.0.0.3:8080",
	}

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			addr := addresses[idx%len(addresses)]
			if idx%2 == 0 {
				phc.RecordSuccess(addr)
			} else {
				phc.RecordFailure(addr)
			}
			phc.GetStatus(addr)
		}(i)
	}
	wg.Wait()
}

// ==================== Helper Function Tests ====================

func TestItoa(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{10, "10"},
		{123, "123"},
		{-1, "-1"},
		{-123, "-123"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := itoa(tt.input)
			if result != tt.expected {
				t.Errorf("itoa(%d) = %s, expected %s", tt.input, result, tt.expected)
			}
		})
	}
}

// ==================== Benchmark Tests ====================

func BenchmarkPassiveHealthChecker_RecordSuccess(b *testing.B) {
	phc := NewPassiveHealthChecker(nil)
	address := "10.0.0.1:8080"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		phc.RecordSuccess(address)
	}
}

func BenchmarkPassiveHealthChecker_RecordFailure(b *testing.B) {
	config := &PassiveHealthConfig{
		FailureThreshold:   1000000, // High to avoid ejection
		MaxEjectionPercent: 0,       // Disable ejection
	}
	phc := NewPassiveHealthChecker(config)
	address := "10.0.0.1:8080"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		phc.RecordFailure(address)
	}
}

func BenchmarkPassiveHealthChecker_GetStatus(b *testing.B) {
	phc := NewPassiveHealthChecker(nil)
	address := "10.0.0.1:8080"
	phc.RecordSuccess(address)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		phc.GetStatus(address)
	}
}
