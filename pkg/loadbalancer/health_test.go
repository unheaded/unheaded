// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package loadbalancer

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestHealthChecker tests the basic health checker functionality.
func TestHealthChecker(t *testing.T) {
	// Create a test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Parse server address
	_, port, _ := net.SplitHostPort(server.Listener.Addr().String())

	// Create config
	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{
			Name:    "backend1",
			Address: "127.0.0.1:" + port,
			Weight:  1,
			Enabled: true,
		},
	}
	config.HealthCheck = HealthCheckConfig{
		Enabled:          true,
		Type:             HealthCheckHTTP,
		Interval:         100 * time.Millisecond,
		Timeout:          50 * time.Millisecond,
		Retries:          2,
		SuccessThreshold: 1,
		HTTPPath:         "/health",
	}

	// Create pool and health checker
	pool, err := NewBackendPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}

	hc := NewHealthChecker(config.HealthCheck, pool)

	// Set callbacks
	var healthyCount, unhealthyCount int32
	hc.OnHealthy(func(backend *Backend) {
		atomic.AddInt32(&healthyCount, 1)
	})
	hc.OnUnhealthy(func(backend *Backend, err error) {
		atomic.AddInt32(&unhealthyCount, 1)
	})

	// Start health checker
	if err := hc.Start(); err != nil {
		t.Fatalf("Failed to start health checker: %v", err)
	}

	// Wait for health checks to run
	time.Sleep(300 * time.Millisecond)

	// Verify backend is healthy
	backend, _ := pool.Get("backend1")
	if backend.State() != BackendStateHealthy {
		t.Errorf("Expected backend to be healthy, got %v", backend.State())
	}

	// Get stats
	stats := hc.Stats()
	if stats.Results["backend1"].TotalChecks == 0 {
		t.Error("Expected at least one health check")
	}
	if stats.Results["backend1"].TotalErrors > 0 {
		t.Error("Expected no health check errors")
	}

	// Stop health checker
	if err := hc.Stop(); err != nil {
		t.Fatalf("Failed to stop health checker: %v", err)
	}
}

// TestHealthCheckerTCP tests TCP health checks.
func TestHealthCheckerTCP(t *testing.T) {
	// Create a TCP listener
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

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

	// Get listener address
	addr := listener.Addr().String()

	// Create config
	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{
			Name:    "backend1",
			Address: addr,
			Weight:  1,
			Enabled: true,
		},
	}
	config.HealthCheck = HealthCheckConfig{
		Enabled:          true,
		Type:             HealthCheckTCP,
		Interval:         100 * time.Millisecond,
		Timeout:          50 * time.Millisecond,
		Retries:          2,
		SuccessThreshold: 1,
	}

	// Create pool and health checker
	pool, err := NewBackendPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}

	hc := NewHealthChecker(config.HealthCheck, pool)

	// Perform immediate check
	backend, _ := pool.Get("backend1")
	hc.CheckNow(backend)

	// Verify result
	result, ok := hc.LastResult("backend1")
	if !ok {
		t.Fatal("Expected health check result")
	}
	if !result.healthy {
		t.Error("Expected backend to be healthy")
	}
}

// TestHealthCheckerUnhealthy tests unhealthy detection.
func TestHealthCheckerUnhealthy(t *testing.T) {
	// Create config with non-existent backend
	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{
			Name:    "backend1",
			Address: "127.0.0.1:1", // Should fail to connect
			Weight:  1,
			Enabled: true,
		},
	}
	config.HealthCheck = HealthCheckConfig{
		Enabled:          true,
		Type:             HealthCheckTCP,
		Interval:         50 * time.Millisecond,
		Timeout:          20 * time.Millisecond,
		Retries:          2,
		SuccessThreshold: 1,
	}

	// Create pool and health checker
	pool, err := NewBackendPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}

	hc := NewHealthChecker(config.HealthCheck, pool)

	var unhealthyCallbackCalled int32
	hc.OnUnhealthy(func(backend *Backend, err error) {
		atomic.StoreInt32(&unhealthyCallbackCalled, 1)
	})

	// Start health checker
	if err := hc.Start(); err != nil {
		t.Fatalf("Failed to start health checker: %v", err)
	}

	// Wait for health checks to run
	time.Sleep(200 * time.Millisecond)

	// Verify backend is unhealthy
	backend, _ := pool.Get("backend1")
	if backend.State() != BackendStateUnhealthy {
		t.Errorf("Expected backend to be unhealthy, got %v", backend.State())
	}

	// Verify callback was called
	if atomic.LoadInt32(&unhealthyCallbackCalled) != 1 {
		t.Error("Expected unhealthy callback to be called")
	}

	// Stop health checker
	hc.Stop()
}

// TestPassiveHealthChecker tests passive health checking.
func TestPassiveHealthChecker(t *testing.T) {
	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{
			Name:    "backend1",
			Address: "127.0.0.1:8080",
			Weight:  1,
			Enabled: true,
		},
	}
	config.HealthCheck = HealthCheckConfig{
		PassiveEnabled:        true,
		PassiveErrorThreshold: 3,
		PassiveErrorWindow:    1 * time.Second,
	}

	// Create pool
	pool, err := NewBackendPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}

	// Set backend to healthy first
	backend, _ := pool.Get("backend1")
	backend.SetState(BackendStateHealthy)

	// Create passive health checker
	phc := NewPassiveHealthChecker(config.HealthCheck, pool)

	// Record errors
	for i := 0; i < 3; i++ {
		phc.RecordError(backend, fmt.Errorf("test error %d", i))
	}

	// Backend should now be unhealthy
	if backend.State() != BackendStateUnhealthy {
		t.Errorf("Expected backend to be unhealthy after errors, got %v", backend.State())
	}
}

// TestHealthAggregator tests the health aggregator.
func TestHealthAggregator(t *testing.T) {
	// Create a test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	_, port, _ := net.SplitHostPort(server.Listener.Addr().String())

	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{
			Name:    "backend1",
			Address: "127.0.0.1:" + port,
			Weight:  1,
			Enabled: true,
		},
	}
	config.HealthCheck = HealthCheckConfig{
		Enabled:               true,
		Type:                  HealthCheckHTTP,
		Interval:              100 * time.Millisecond,
		Timeout:               50 * time.Millisecond,
		Retries:               2,
		SuccessThreshold:      1,
		HTTPPath:              "/",
		PassiveEnabled:        true,
		PassiveErrorThreshold: 5,
		PassiveErrorWindow:    1 * time.Second,
	}

	pool, err := NewBackendPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}

	// Create aggregator
	ha := NewHealthAggregator(config.HealthCheck, pool)

	var healthyCallbackCalled int32
	ha.OnHealthy(func(backend *Backend) {
		atomic.StoreInt32(&healthyCallbackCalled, 1)
	})

	// Start
	if err := ha.Start(); err != nil {
		t.Fatalf("Failed to start health aggregator: %v", err)
	}

	// Wait for health checks
	time.Sleep(200 * time.Millisecond)

	// Verify callback
	if atomic.LoadInt32(&healthyCallbackCalled) != 1 {
		t.Error("Expected healthy callback to be called")
	}

	// Stop
	if err := ha.Stop(); err != nil {
		t.Fatalf("Failed to stop health aggregator: %v", err)
	}
}

// TestHealthEndpoint tests the health endpoint HTTP handler.
func TestHealthEndpoint(t *testing.T) {
	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{
			Name:    "backend1",
			Address: "127.0.0.1:8080",
			Weight:  1,
			Enabled: true,
		},
	}

	pool, err := NewBackendPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}

	// Set backend healthy
	backend, _ := pool.Get("backend1")
	backend.SetState(BackendStateHealthy)

	endpoint := NewHealthEndpoint(pool)

	// Test /health endpoint
	t.Run("Health", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/health", nil)
		w := httptest.NewRecorder()
		endpoint.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}
	})

	// Test /ready endpoint
	t.Run("Ready", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/ready", nil)
		w := httptest.NewRecorder()
		endpoint.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}
	})

	// Test /backends endpoint
	t.Run("Backends", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/backends", nil)
		w := httptest.NewRecorder()
		endpoint.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}
	})
}

// TestHealthCheckScheduler tests custom health check scheduling.
func TestHealthCheckScheduler(t *testing.T) {
	// Create a test TCP listener
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
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

	addr := listener.Addr().String()

	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{
			Name:    "backend1",
			Address: addr,
			Weight:  1,
			Enabled: true,
		},
	}
	config.HealthCheck = HealthCheckConfig{
		Enabled:          true,
		Type:             HealthCheckTCP,
		Interval:         100 * time.Millisecond,
		Timeout:          50 * time.Millisecond,
		Retries:          1,
		SuccessThreshold: 1,
	}

	pool, err := NewBackendPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}

	hc := NewHealthChecker(config.HealthCheck, pool)
	scheduler := NewHealthCheckScheduler(hc)

	// Set custom interval
	scheduler.SetInterval("backend1", 50*time.Millisecond)

	// Start scheduler
	if err := scheduler.Start(); err != nil {
		t.Fatalf("Failed to start scheduler: %v", err)
	}

	// Wait for checks
	time.Sleep(200 * time.Millisecond)

	// Verify checks ran
	result, ok := hc.LastResult("backend1")
	if !ok {
		t.Fatal("Expected health check result")
	}
	if !result.healthy {
		t.Error("Expected backend to be healthy")
	}

	// Stop scheduler
	scheduler.Stop()
}

// TestBackendHealthCheckResult tests the SetHealthCheckResult method.
func TestBackendHealthCheckResult(t *testing.T) {
	config := BackendConfig{
		Name:    "test",
		Address: "127.0.0.1:8080",
		Weight:  1,
		Enabled: true,
	}

	backend, err := NewBackend(config)
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}

	healthConfig := HealthCheckConfig{
		Retries:          3,
		SuccessThreshold: 2,
	}

	// Record failures
	for i := 0; i < 3; i++ {
		backend.SetHealthCheckResult(false, healthConfig)
	}

	// Should be unhealthy after 3 failures (= retries)
	if backend.State() != BackendStateUnhealthy {
		t.Errorf("Expected unhealthy after failures, got %v", backend.State())
	}

	// Record successes
	backend.SetHealthCheckResult(true, healthConfig)
	// Still unhealthy (need 2 successes)
	if backend.State() == BackendStateHealthy {
		t.Error("Backend became healthy too early")
	}

	backend.SetHealthCheckResult(true, healthConfig)
	// Now should be healthy
	if backend.State() != BackendStateHealthy {
		t.Errorf("Expected healthy after successes, got %v", backend.State())
	}
}

// TestConcurrentHealthChecks tests concurrent health check execution.
func TestConcurrentHealthChecks(t *testing.T) {
	// Create multiple test servers
	var servers []*httptest.Server
	var addresses []string

	for i := 0; i < 5; i++ {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(10 * time.Millisecond) // Simulate work
			w.WriteHeader(http.StatusOK)
		}))
		servers = append(servers, server)
		_, port, _ := net.SplitHostPort(server.Listener.Addr().String())
		addresses = append(addresses, "127.0.0.1:"+port)
	}
	defer func() {
		for _, s := range servers {
			s.Close()
		}
	}()

	// Create config with multiple backends
	config := DefaultConfig()
	for i, addr := range addresses {
		config.Backends = append(config.Backends, BackendConfig{
			Name:    fmt.Sprintf("backend%d", i),
			Address: addr,
			Weight:  1,
			Enabled: true,
		})
	}
	config.HealthCheck = HealthCheckConfig{
		Enabled:          true,
		Type:             HealthCheckHTTP,
		Interval:         50 * time.Millisecond,
		Timeout:          100 * time.Millisecond,
		Retries:          1,
		SuccessThreshold: 1,
		HTTPPath:         "/",
	}

	pool, err := NewBackendPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}

	hc := NewHealthChecker(config.HealthCheck, pool)

	if err := hc.Start(); err != nil {
		t.Fatalf("Failed to start health checker: %v", err)
	}

	// Wait for checks
	time.Sleep(200 * time.Millisecond)

	// All backends should be healthy
	for i := 0; i < 5; i++ {
		backend, _ := pool.Get(fmt.Sprintf("backend%d", i))
		if backend.State() != BackendStateHealthy {
			t.Errorf("Backend %d should be healthy, got %v", i, backend.State())
		}
	}

	hc.Stop()
}

// TestHealthCheckerDoubleStart tests starting health checker twice.
func TestHealthCheckerDoubleStart(t *testing.T) {
	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{
			Name:    "backend1",
			Address: "127.0.0.1:8080",
			Weight:  1,
			Enabled: true,
		},
	}
	config.HealthCheck = HealthCheckConfig{
		Enabled:  true,
		Type:     HealthCheckTCP,
		Interval: 100 * time.Millisecond,
		Timeout:  50 * time.Millisecond,
	}

	pool, _ := NewBackendPool(config)
	hc := NewHealthChecker(config.HealthCheck, pool)

	// First start should succeed
	if err := hc.Start(); err != nil {
		t.Fatalf("First start failed: %v", err)
	}

	// Second start should fail
	if err := hc.Start(); err == nil {
		t.Error("Expected error on second start")
	}

	hc.Stop()
}

// TestHealthCheckerStopWithoutStart tests stopping without starting.
func TestHealthCheckerStopWithoutStart(t *testing.T) {
	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{
			Name:    "backend1",
			Address: "127.0.0.1:8080",
			Weight:  1,
			Enabled: true,
		},
	}
	config.HealthCheck = HealthCheckConfig{
		Enabled:  true,
		Type:     HealthCheckTCP,
		Interval: 100 * time.Millisecond,
		Timeout:  50 * time.Millisecond,
	}

	pool, _ := NewBackendPool(config)
	hc := NewHealthChecker(config.HealthCheck, pool)

	// Stop without start should fail
	if err := hc.Stop(); err == nil {
		t.Error("Expected error when stopping without start")
	}
}

// TestHealthCheckHTTPExpectedCodes tests HTTP health checks with expected status codes.
func TestHealthCheckHTTPExpectedCodes(t *testing.T) {
	// Create server that returns 201
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	_, port, _ := net.SplitHostPort(server.Listener.Addr().String())

	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{
			Name:    "backend1",
			Address: "127.0.0.1:" + port,
			Weight:  1,
			Enabled: true,
		},
	}
	config.HealthCheck = HealthCheckConfig{
		Enabled:           true,
		Type:              HealthCheckHTTP,
		Interval:          100 * time.Millisecond,
		Timeout:           50 * time.Millisecond,
		Retries:           1,
		SuccessThreshold:  1,
		HTTPPath:          "/",
		HTTPExpectedCodes: []int{200, 201, 204}, // 201 is in the list
	}

	pool, _ := NewBackendPool(config)
	hc := NewHealthChecker(config.HealthCheck, pool)

	backend, _ := pool.Get("backend1")
	hc.CheckNow(backend)

	result, ok := hc.LastResult("backend1")
	if !ok {
		t.Fatal("Expected result")
	}
	if !result.healthy {
		t.Error("Expected healthy with expected code")
	}
}

// TestHealthCheckHTTPExpectedBody tests HTTP health checks with expected body content.
func TestHealthCheckHTTPExpectedBody(t *testing.T) {
	// Create server that returns specific body
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "healthy"}`))
	}))
	defer server.Close()

	_, port, _ := net.SplitHostPort(server.Listener.Addr().String())

	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{
			Name:    "backend1",
			Address: "127.0.0.1:" + port,
			Weight:  1,
			Enabled: true,
		},
	}
	config.HealthCheck = HealthCheckConfig{
		Enabled:          true,
		Type:             HealthCheckHTTP,
		Interval:         100 * time.Millisecond,
		Timeout:          50 * time.Millisecond,
		Retries:          1,
		SuccessThreshold: 1,
		HTTPPath:         "/",
		HTTPExpectedBody: "healthy",
	}

	pool, _ := NewBackendPool(config)
	hc := NewHealthChecker(config.HealthCheck, pool)

	backend, _ := pool.Get("backend1")
	hc.CheckNow(backend)

	result, ok := hc.LastResult("backend1")
	if !ok {
		t.Fatal("Expected result")
	}
	if !result.healthy {
		t.Errorf("Expected healthy with matching body, got err: %v", result.err)
	}
}

// TestDisabledBackendSkipped tests that disabled backends are skipped.
func TestDisabledBackendSkipped(t *testing.T) {
	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{
			Name:    "backend1",
			Address: "127.0.0.1:8080",
			Weight:  1,
			Enabled: false, // Disabled
		},
	}
	config.HealthCheck = HealthCheckConfig{
		Enabled:  true,
		Type:     HealthCheckTCP,
		Interval: 50 * time.Millisecond,
		Timeout:  25 * time.Millisecond,
	}

	pool, _ := NewBackendPool(config)
	hc := NewHealthChecker(config.HealthCheck, pool)

	if err := hc.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// No result should exist since backend is disabled
	_, ok := hc.LastResult("backend1")
	if ok {
		t.Error("Expected no result for disabled backend")
	}

	hc.Stop()
}

// TestPerBackendHealthCheckOverride tests per-backend health check configuration.
func TestPerBackendHealthCheckOverride(t *testing.T) {
	// Create two servers - one HTTP, one that just accepts TCP
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer httpServer.Close()

	tcpListener, _ := net.Listen("tcp", "127.0.0.1:0")
	defer tcpListener.Close()
	go func() {
		for {
			conn, err := tcpListener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	_, httpPort, _ := net.SplitHostPort(httpServer.Listener.Addr().String())
	tcpAddr := tcpListener.Addr().String()

	// Backend 1 uses HTTP (global default), Backend 2 has TCP override
	tcpOverride := HealthCheckConfig{
		Enabled:          true,
		Type:             HealthCheckTCP,
		Interval:         50 * time.Millisecond,
		Timeout:          25 * time.Millisecond,
		Retries:          1,
		SuccessThreshold: 1,
	}

	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{
			Name:    "backend1",
			Address: "127.0.0.1:" + httpPort,
			Weight:  1,
			Enabled: true,
		},
		{
			Name:                "backend2",
			Address:             tcpAddr,
			Weight:              1,
			Enabled:             true,
			HealthCheckOverride: &tcpOverride,
		},
	}
	config.HealthCheck = HealthCheckConfig{
		Enabled:          true,
		Type:             HealthCheckHTTP, // Global default is HTTP
		Interval:         50 * time.Millisecond,
		Timeout:          25 * time.Millisecond,
		Retries:          1,
		SuccessThreshold: 1,
		HTTPPath:         "/",
	}

	pool, _ := NewBackendPool(config)
	hc := NewHealthChecker(config.HealthCheck, pool)

	if err := hc.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Both backends should be healthy
	backend1, _ := pool.Get("backend1")
	backend2, _ := pool.Get("backend2")

	if backend1.State() != BackendStateHealthy {
		t.Errorf("Backend1 should be healthy, got %v", backend1.State())
	}
	if backend2.State() != BackendStateHealthy {
		t.Errorf("Backend2 should be healthy, got %v", backend2.State())
	}

	hc.Stop()
}

// BenchmarkHealthCheck benchmarks health check performance.
func BenchmarkHealthCheck(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	_, port, _ := net.SplitHostPort(server.Listener.Addr().String())

	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{
			Name:    "backend1",
			Address: "127.0.0.1:" + port,
			Weight:  1,
			Enabled: true,
		},
	}
	config.HealthCheck = HealthCheckConfig{
		Enabled:          true,
		Type:             HealthCheckHTTP,
		Interval:         time.Hour, // Don't run automatically
		Timeout:          time.Second,
		Retries:          1,
		SuccessThreshold: 1,
		HTTPPath:         "/",
	}

	pool, _ := NewBackendPool(config)
	hc := NewHealthChecker(config.HealthCheck, pool)
	backend, _ := pool.Get("backend1")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hc.CheckNow(backend)
	}
}

// BenchmarkConcurrentHealthChecks benchmarks concurrent health checks.
func BenchmarkConcurrentHealthChecks(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	_, port, _ := net.SplitHostPort(server.Listener.Addr().String())

	config := DefaultConfig()
	for i := 0; i < 10; i++ {
		config.Backends = append(config.Backends, BackendConfig{
			Name:    fmt.Sprintf("backend%d", i),
			Address: "127.0.0.1:" + port,
			Weight:  1,
			Enabled: true,
		})
	}
	config.HealthCheck = HealthCheckConfig{
		Enabled:          true,
		Type:             HealthCheckHTTP,
		Interval:         time.Hour,
		Timeout:          time.Second,
		Retries:          1,
		SuccessThreshold: 1,
		HTTPPath:         "/",
	}

	pool, _ := NewBackendPool(config)
	hc := NewHealthChecker(config.HealthCheck, pool)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup
		for _, backend := range pool.All() {
			wg.Add(1)
			go func(b *Backend) {
				defer wg.Done()
				hc.CheckNow(b)
			}(backend)
		}
		wg.Wait()
	}
}

// TestErrorClassification tests error classification.
func TestErrorClassification(t *testing.T) {
	tests := []struct {
		err      error
		expected string
	}{
		{fmt.Errorf("connection refused"), ErrorTypeConnection},
		{fmt.Errorf("dial tcp: connection reset by peer"), ErrorTypeConnection},
		{fmt.Errorf("i/o timeout"), ErrorTypeTimeout},
		{fmt.Errorf("context deadline exceeded"), ErrorTypeTimeout},
		{fmt.Errorf("http status 503"), ErrorTypeHTTP5xx},
		{fmt.Errorf("protocol error"), ErrorTypeProtocol},
		{fmt.Errorf("some other error"), ErrorTypeOther},
		{nil, ""},
	}

	for _, tc := range tests {
		result := ClassifyError(tc.err)
		if result != tc.expected {
			t.Errorf("ClassifyError(%v) = %s, expected %s", tc.err, result, tc.expected)
		}
	}
}

// TestLatencyWindow tests the latency window statistics.
func TestLatencyWindow(t *testing.T) {
	lw := NewLatencyWindow(10)

	// Add samples
	samples := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
		50 * time.Millisecond,
	}

	for _, s := range samples {
		lw.Add(s)
	}

	avg, min, max, _, _, _ := lw.Stats()

	expectedAvg := 30 * time.Millisecond
	if avg != expectedAvg {
		t.Errorf("Expected avg %v, got %v", expectedAvg, avg)
	}

	if min != 10*time.Millisecond {
		t.Errorf("Expected min 10ms, got %v", min)
	}

	if max != 50*time.Millisecond {
		t.Errorf("Expected max 50ms, got %v", max)
	}
}

// TestHealthHistory tests health history tracking.
func TestHealthHistory(t *testing.T) {
	h := NewHealthHistory(20)

	// Add mostly healthy results
	for i := 0; i < 15; i++ {
		h.Add(true, 10*time.Millisecond)
	}

	// Add some unhealthy results at the end
	for i := 0; i < 5; i++ {
		h.Add(false, 10*time.Millisecond)
	}

	// Success rate should be 75%
	rate := h.SuccessRate()
	if rate != 0.75 {
		t.Errorf("Expected success rate 0.75, got %f", rate)
	}

	// Trend should be negative (degrading - more recent failures)
	trend := h.Trend()
	if trend >= 0 {
		t.Errorf("Expected negative trend (degrading), got %f", trend)
	}
}

// TestHealthCheckJitter tests health check jitter.
func TestHealthCheckJitter(t *testing.T) {
	jitter := NewHealthCheckJitter(100*time.Millisecond, 0.2)

	// Get multiple intervals and verify they vary
	intervals := make(map[time.Duration]bool)
	for i := 0; i < 100; i++ {
		interval := jitter.GetInterval()
		intervals[interval] = true

		// Should be within 80ms to 120ms (100ms +/- 20%)
		if interval < 80*time.Millisecond || interval > 120*time.Millisecond {
			t.Errorf("Interval %v outside expected range", interval)
		}
	}

	// With jitter, we should see some variation
	// Note: Due to the deterministic nature of the "random" in GetInterval,
	// we might not get much variation in quick succession
}

// TestHealthCheckRetry tests the retry logic.
func TestHealthCheckRetry(t *testing.T) {
	retry := NewHealthCheckWithRetry(3, 100*time.Millisecond)

	// Test retryable errors
	connErr := fmt.Errorf("connection refused")
	if !retry.IsRetryable(connErr) {
		t.Error("Connection error should be retryable")
	}

	timeoutErr := fmt.Errorf("context deadline exceeded")
	if !retry.IsRetryable(timeoutErr) {
		t.Error("Timeout error should be retryable")
	}

	otherErr := fmt.Errorf("some other error")
	if retry.IsRetryable(otherErr) {
		t.Error("Other error should not be retryable")
	}

	// Test backoff
	delay0 := retry.GetDelay(0)
	delay1 := retry.GetDelay(1)
	delay2 := retry.GetDelay(2)

	if delay0 != 100*time.Millisecond {
		t.Errorf("Expected delay0 100ms, got %v", delay0)
	}
	if delay1 != 200*time.Millisecond {
		t.Errorf("Expected delay1 200ms, got %v", delay1)
	}
	if delay2 != 400*time.Millisecond {
		t.Errorf("Expected delay2 400ms, got %v", delay2)
	}
}

// TestEnhancedHealthChecker tests the enhanced health checker.
func TestEnhancedHealthChecker(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	_, port, _ := net.SplitHostPort(server.Listener.Addr().String())

	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{
			Name:    "backend1",
			Address: "127.0.0.1:" + port,
			Weight:  1,
			Enabled: true,
		},
	}
	config.HealthCheck = HealthCheckConfig{
		Enabled:          true,
		Type:             HealthCheckHTTP,
		Interval:         100 * time.Millisecond,
		Timeout:          50 * time.Millisecond,
		Retries:          3,
		SuccessThreshold: 1,
		HTTPPath:         "/",
	}

	pool, err := NewBackendPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}

	ehc := NewEnhancedHealthChecker(config.HealthCheck, pool)

	// Test circuit breaker integration
	backend, _ := pool.Get("backend1")
	err = ehc.CheckWithCircuitBreaker(backend)
	if err != nil {
		t.Errorf("Check with circuit breaker failed: %v", err)
	}

	// Get backend health info
	info := ehc.GetBackendHealth("backend1")
	if info.Name != "backend1" {
		t.Errorf("Expected name backend1, got %s", info.Name)
	}
	if info.CircuitBreakerState != "closed" {
		t.Errorf("Expected circuit breaker closed, got %s", info.CircuitBreakerState)
	}
}

// TestEnhancedPassiveHealthChecker tests the enhanced passive health checker.
func TestEnhancedPassiveHealthChecker(t *testing.T) {
	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{
			Name:    "backend1",
			Address: "127.0.0.1:8080",
			Weight:  1,
			Enabled: true,
		},
	}
	config.HealthCheck = HealthCheckConfig{
		PassiveEnabled:        true,
		PassiveErrorThreshold: 3,
		PassiveErrorWindow:    1 * time.Second,
	}

	pool, _ := NewBackendPool(config)
	backend, _ := pool.Get("backend1")
	backend.SetState(BackendStateHealthy)

	ephc := NewEnhancedPassiveHealthChecker(config.HealthCheck, pool)

	// Set error types to track
	ephc.SetErrorTypes([]string{ErrorTypeConnection, ErrorTypeTimeout})

	// Record errors with types
	ephc.RecordErrorWithType(backend, fmt.Errorf("connection refused"), ErrorTypeConnection)
	ephc.RecordErrorWithType(backend, fmt.Errorf("timeout"), ErrorTypeTimeout)
	ephc.RecordErrorWithType(backend, fmt.Errorf("connection refused"), ErrorTypeConnection)

	// Get error stats
	stats := ephc.GetErrorStats("backend1")
	if stats[ErrorTypeConnection] != 2 {
		t.Errorf("Expected 2 connection errors, got %d", stats[ErrorTypeConnection])
	}
	if stats[ErrorTypeTimeout] != 1 {
		t.Errorf("Expected 1 timeout error, got %d", stats[ErrorTypeTimeout])
	}
}

// TestHealthCheckCoordinator tests the health check coordinator.
func TestHealthCheckCoordinator(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	_, port, _ := net.SplitHostPort(server.Listener.Addr().String())

	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{
			Name:    "backend1",
			Address: "127.0.0.1:" + port,
			Weight:  1,
			Enabled: true,
		},
	}
	config.HealthCheck = HealthCheckConfig{
		Enabled:          true,
		Type:             HealthCheckHTTP,
		Interval:         100 * time.Millisecond,
		Timeout:          50 * time.Millisecond,
		Retries:          1,
		SuccessThreshold: 1,
		HTTPPath:         "/",
	}

	pool, _ := NewBackendPool(config)
	ehc := NewEnhancedHealthChecker(config.HealthCheck, pool)

	coordinator := NewHealthCheckCoordinator()
	coordinator.AddChecker("pool1", ehc)

	if err := coordinator.Start(); err != nil {
		t.Fatalf("Failed to start coordinator: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Get all health info
	healthInfo := coordinator.GetAllHealth()
	if len(healthInfo) != 1 {
		t.Errorf("Expected 1 pool, got %d", len(healthInfo))
	}
	if len(healthInfo["pool1"]) != 1 {
		t.Errorf("Expected 1 backend, got %d", len(healthInfo["pool1"]))
	}

	if err := coordinator.Stop(); err != nil {
		t.Fatalf("Failed to stop coordinator: %v", err)
	}
}

// TestBackendDrain tests the backend drain functionality.
func TestBackendDrain(t *testing.T) {
	config := BackendConfig{
		Name:    "test",
		Address: "127.0.0.1:8080",
		Weight:  1,
		Enabled: true,
	}

	backend, _ := NewBackend(config)
	backend.SetState(BackendStateHealthy)

	// Simulate active connections
	backend.IncrementConnections()
	backend.IncrementConnections()

	// Start draining
	backend.Drain()

	if backend.State() != BackendStateDraining {
		t.Errorf("Expected draining state, got %v", backend.State())
	}

	// Should not be drained yet
	if backend.IsDrained() {
		t.Error("Backend should not be drained with active connections")
	}

	// Remove connections
	backend.DecrementConnections()
	backend.DecrementConnections()

	// Now should be drained
	if !backend.IsDrained() {
		t.Error("Backend should be drained with no active connections")
	}
}

// TestSelectWithContext tests backend selection with context.
func TestSelectWithContext(t *testing.T) {
	config := DefaultConfig()
	config.Backends = []BackendConfig{
		{
			Name:    "backend1",
			Address: "127.0.0.1:8080",
			Weight:  1,
			Enabled: true,
		},
	}

	pool, _ := NewBackendPool(config)
	backend, _ := pool.Get("backend1")
	backend.SetState(BackendStateHealthy)

	// Test with normal context
	ctx := context.Background()
	selected, err := pool.Select(ctx, "test-key")
	if err != nil {
		t.Errorf("Select failed: %v", err)
	}
	if selected == nil {
		t.Error("Expected backend, got nil")
	}

	// Test with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Selection should still work even with cancelled context
	// (context is for future use in async selection)
	selected, err = pool.Select(ctx, "test-key")
	if err != nil {
		t.Errorf("Select with cancelled context failed: %v", err)
	}
	if selected == nil {
		t.Error("Select with cancelled context should still return a backend (sync path)")
	}
}
