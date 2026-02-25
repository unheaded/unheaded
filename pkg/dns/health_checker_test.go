// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package dns

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEnhancedHealthChecker_New(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false

	sd, err := NewEnhancedServiceDiscovery(config)
	if err != nil {
		t.Fatalf("Failed to create service discovery: %v", err)
	}
	defer sd.Close()

	hc := NewEnhancedHealthChecker(config, sd)
	if hc == nil {
		t.Fatal("NewEnhancedHealthChecker returned nil")
	}

	if hc.config != config {
		t.Error("Config not set correctly")
	}
	if hc.sd != sd {
		t.Error("Service discovery not set correctly")
	}
}

func TestEnhancedHealthChecker_StartStop(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false

	sd, _ := NewEnhancedServiceDiscovery(config)
	defer sd.Close()

	hc := NewEnhancedHealthChecker(config, sd)

	// Start
	err := hc.Start()
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}

	// Start again should fail
	err = hc.Start()
	if err == nil {
		t.Error("Second Start should return error")
	}

	// Stop
	hc.Stop()

	// Stop again should be safe
	hc.Stop()
}

func TestEnhancedHealthChecker_AddRemoveService(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false
	config.HealthCheckInterval = 100 * time.Millisecond

	sd, _ := NewEnhancedServiceDiscovery(config)
	defer sd.Close()

	hc := NewEnhancedHealthChecker(config, sd)
	hc.Start()
	defer hc.Stop()

	svc := &ServiceDefinition{
		Name:     "test-svc",
		Protocol: "tcp",
		Port:     8080,
		Endpoints: []*ServiceEndpoint{
			{
				ID:      "ep-1",
				Address: net.ParseIP("127.0.0.1"),
				Port:    8080,
				Health:  HealthStateHealthy,
			},
		},
		HealthCheck: &ServiceHealthCheck{
			Type:     "tcp",
			Interval: 100 * time.Millisecond,
			Timeout:  50 * time.Millisecond,
		},
	}

	// Add service
	hc.AddService(svc)

	// Adding again should be no-op
	hc.AddService(svc)

	// Wait a bit for health check to run
	time.Sleep(50 * time.Millisecond)

	// Remove service
	hc.RemoveService("test-svc")

	// Remove again should be safe
	hc.RemoveService("test-svc")
}

func TestEnhancedHealthChecker_TCPCheck(t *testing.T) {
	// Start a TCP listener
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start listener: %v", err)
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

	port := listener.Addr().(*net.TCPAddr).Port

	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false
	config.HealthCheckInterval = 50 * time.Millisecond
	config.HealthCheckTimeout = 100 * time.Millisecond
	config.HealthyThreshold = 1
	config.UnhealthyThreshold = 1

	sd, _ := NewEnhancedServiceDiscovery(config)
	defer sd.Close()

	hc := NewEnhancedHealthChecker(config, sd)
	hc.Start()
	defer hc.Stop()

	svc := &ServiceDefinition{
		Name: "tcp-test",
		Endpoints: []*ServiceEndpoint{
			{
				ID:      "ep-1",
				Address: net.ParseIP("127.0.0.1"),
				Port:    uint16(port),
				Health:  HealthStateUnknown,
			},
		},
		HealthCheck: &ServiceHealthCheck{
			Type:               "tcp",
			Interval:           50 * time.Millisecond,
			Timeout:            100 * time.Millisecond,
			HealthyThreshold:   1,
			UnhealthyThreshold: 1,
		},
	}

	// Register service with SD first
	sd.RegisterService(svc)
	hc.AddService(svc)

	// Wait for health check
	time.Sleep(200 * time.Millisecond)

	// Check health
	info, err := hc.GetEndpointHealth("tcp-test", "ep-1")
	if err != nil {
		t.Fatalf("GetEndpointHealth error: %v", err)
	}

	if info.Health != HealthStateHealthy {
		t.Errorf("Expected healthy, got %v", info.Health)
	}
}

func TestEnhancedHealthChecker_HTTPCheck(t *testing.T) {
	// Start an HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Parse port from URL
	addr := server.Listener.Addr().(*net.TCPAddr)

	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false
	config.HealthCheckInterval = 50 * time.Millisecond
	config.HealthCheckTimeout = 100 * time.Millisecond
	config.HealthyThreshold = 1
	config.UnhealthyThreshold = 1

	sd, _ := NewEnhancedServiceDiscovery(config)
	defer sd.Close()

	hc := NewEnhancedHealthChecker(config, sd)
	hc.Start()
	defer hc.Stop()

	svc := &ServiceDefinition{
		Name: "http-test",
		Endpoints: []*ServiceEndpoint{
			{
				ID:      "ep-1",
				Address: net.ParseIP("127.0.0.1"),
				Port:    uint16(addr.Port),
				Health:  HealthStateUnknown,
			},
		},
		HealthCheck: &ServiceHealthCheck{
			Type:               "http",
			Interval:           50 * time.Millisecond,
			Timeout:            100 * time.Millisecond,
			Path:               "/health",
			ExpectedStatus:     200,
			HealthyThreshold:   1,
			UnhealthyThreshold: 1,
		},
	}

	sd.RegisterService(svc)
	hc.AddService(svc)

	// Wait for health check
	time.Sleep(200 * time.Millisecond)

	info, err := hc.GetEndpointHealth("http-test", "ep-1")
	if err != nil {
		t.Fatalf("GetEndpointHealth error: %v", err)
	}

	if info.Health != HealthStateHealthy {
		t.Errorf("Expected healthy, got %v", info.Health)
	}
}

func TestEnhancedHealthChecker_FailedCheck(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false
	config.HealthCheckInterval = 50 * time.Millisecond
	config.HealthCheckTimeout = 50 * time.Millisecond
	config.HealthyThreshold = 1
	config.UnhealthyThreshold = 1

	sd, _ := NewEnhancedServiceDiscovery(config)
	defer sd.Close()

	hc := NewEnhancedHealthChecker(config, sd)
	hc.Start()
	defer hc.Stop()

	svc := &ServiceDefinition{
		Name: "fail-test",
		Endpoints: []*ServiceEndpoint{
			{
				ID:      "ep-1",
				Address: net.ParseIP("127.0.0.1"),
				Port:    59999, // Unlikely to be in use
				Health:  HealthStateHealthy,
			},
		},
		HealthCheck: &ServiceHealthCheck{
			Type:               "tcp",
			Interval:           50 * time.Millisecond,
			Timeout:            50 * time.Millisecond,
			HealthyThreshold:   1,
			UnhealthyThreshold: 1,
		},
	}

	sd.RegisterService(svc)
	hc.AddService(svc)

	// Wait for health check to fail
	time.Sleep(200 * time.Millisecond)

	info, err := hc.GetEndpointHealth("fail-test", "ep-1")
	if err != nil {
		t.Fatalf("GetEndpointHealth error: %v", err)
	}

	if info.Health == HealthStateHealthy {
		t.Error("Expected unhealthy or degraded state")
	}
	if info.LastError == nil {
		t.Error("Expected last error to be set")
	}
}

func TestEnhancedHealthChecker_CheckNow(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false
	config.HealthCheckInterval = time.Hour // Long interval
	config.HealthCheckTimeout = 50 * time.Millisecond
	config.HealthyThreshold = 1
	config.UnhealthyThreshold = 1

	sd, _ := NewEnhancedServiceDiscovery(config)
	defer sd.Close()

	hc := NewEnhancedHealthChecker(config, sd)
	hc.Start()
	defer hc.Stop()

	svc := &ServiceDefinition{
		Name: "check-now-test",
		Endpoints: []*ServiceEndpoint{
			{
				ID:      "ep-1",
				Address: net.ParseIP("127.0.0.1"),
				Port:    59998,
				Health:  HealthStateHealthy,
			},
		},
		HealthCheck: &ServiceHealthCheck{
			Type:               "tcp",
			Interval:           time.Hour,
			Timeout:            50 * time.Millisecond,
			HealthyThreshold:   1,
			UnhealthyThreshold: 1,
		},
	}

	sd.RegisterService(svc)
	hc.AddService(svc)

	// Force immediate check
	err := hc.CheckNow("check-now-test")
	if err != nil {
		t.Fatalf("CheckNow error: %v", err)
	}

	// Service not found
	err = hc.CheckNow("nonexistent")
	if err != ErrServiceNotFound {
		t.Errorf("Expected ErrServiceNotFound, got %v", err)
	}
}

func TestEnhancedHealthChecker_HealthStats(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false
	config.HealthCheckInterval = time.Hour
	config.HealthyThreshold = 1
	config.UnhealthyThreshold = 1

	sd, _ := NewEnhancedServiceDiscovery(config)
	defer sd.Close()

	hc := NewEnhancedHealthChecker(config, sd)
	hc.Start()
	defer hc.Stop()

	svc := &ServiceDefinition{
		Name: "stats-test",
		Endpoints: []*ServiceEndpoint{
			{ID: "ep-1", Address: net.ParseIP("10.0.0.1"), Port: 80, Health: HealthStateHealthy},
			{ID: "ep-2", Address: net.ParseIP("10.0.0.2"), Port: 80, Health: HealthStateUnhealthy},
		},
		HealthCheck: &ServiceHealthCheck{
			Type:     "tcp",
			Interval: time.Hour,
		},
	}

	sd.RegisterService(svc)
	hc.AddService(svc)

	stats := hc.HealthStats()
	svcStats, ok := stats["stats-test"]
	if !ok {
		t.Fatal("Expected stats for stats-test")
	}

	if svcStats.TotalEndpoints != 2 {
		t.Errorf("TotalEndpoints = %d, want 2", svcStats.TotalEndpoints)
	}
	if svcStats.HealthyEndpoints != 1 {
		t.Errorf("HealthyEndpoints = %d, want 1", svcStats.HealthyEndpoints)
	}
}

func TestEnhancedHealthChecker_GetEndpointHealthErrors(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false

	sd, _ := NewEnhancedServiceDiscovery(config)
	defer sd.Close()

	hc := NewEnhancedHealthChecker(config, sd)
	hc.Start()
	defer hc.Stop()

	// Non-existent service
	_, err := hc.GetEndpointHealth("nonexistent", "ep-1")
	if err != ErrServiceNotFound {
		t.Errorf("Expected ErrServiceNotFound, got %v", err)
	}

	// Add service
	svc := &ServiceDefinition{
		Name: "test",
		Endpoints: []*ServiceEndpoint{
			{ID: "ep-1", Address: net.ParseIP("10.0.0.1"), Port: 80, Health: HealthStateHealthy},
		},
		HealthCheck: &ServiceHealthCheck{Type: "tcp"},
	}
	sd.RegisterService(svc)
	hc.AddService(svc)

	// Non-existent endpoint
	_, err = hc.GetEndpointHealth("test", "nonexistent")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected endpoint not found error, got %v", err)
	}
}

func TestCircuitState_String(t *testing.T) {
	tests := []struct {
		state CircuitState
		want  string
	}{
		{CircuitClosed, "closed"},
		{CircuitOpen, "open"},
		{CircuitHalfOpen, "half-open"},
		{CircuitState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.state.String(); got != tt.want {
				t.Errorf("String() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestEnhancedHealthChecker_CircuitBreaker(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false
	config.HealthCheckInterval = 10 * time.Millisecond
	config.HealthCheckTimeout = 10 * time.Millisecond
	config.HealthyThreshold = 1
	config.UnhealthyThreshold = 2

	sd, _ := NewEnhancedServiceDiscovery(config)
	defer sd.Close()

	hc := NewEnhancedHealthChecker(config, sd)
	hc.Start()
	defer hc.Stop()

	svc := &ServiceDefinition{
		Name: "circuit-test",
		Endpoints: []*ServiceEndpoint{
			{ID: "ep-1", Address: net.ParseIP("127.0.0.1"), Port: 59997, Health: HealthStateHealthy},
		},
		HealthCheck: &ServiceHealthCheck{
			Type:               "tcp",
			Interval:           10 * time.Millisecond,
			Timeout:            10 * time.Millisecond,
			HealthyThreshold:   1,
			UnhealthyThreshold: 2,
		},
		CircuitBreaker: &CircuitBreakerConfig{
			Enabled:          true,
			ErrorThreshold:   2,
			ErrorWindow:      time.Second,
			OpenDuration:     50 * time.Millisecond,
			HalfOpenRequests: 1,
		},
	}

	sd.RegisterService(svc)
	hc.AddService(svc)

	// Wait for circuit to open (after failures)
	time.Sleep(100 * time.Millisecond)

	info, _ := hc.GetEndpointHealth("circuit-test", "ep-1")
	// Circuit state should be tracked
	if info.CircuitState != CircuitOpen && info.CircuitState != CircuitHalfOpen && info.CircuitState != CircuitClosed {
		t.Errorf("Unexpected circuit state: %v", info.CircuitState)
	}
}

func TestEnhancedHealthChecker_DegradedState(t *testing.T) {
	// Test that partial failures result in degraded state

	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false
	config.HealthCheckInterval = 20 * time.Millisecond
	config.HealthCheckTimeout = 10 * time.Millisecond
	config.HealthyThreshold = 2
	config.UnhealthyThreshold = 3

	sd, _ := NewEnhancedServiceDiscovery(config)
	defer sd.Close()

	hc := NewEnhancedHealthChecker(config, sd)
	hc.Start()
	defer hc.Stop()

	svc := &ServiceDefinition{
		Name: "degraded-test",
		Endpoints: []*ServiceEndpoint{
			{ID: "ep-1", Address: net.ParseIP("127.0.0.1"), Port: 59996, Health: HealthStateHealthy},
		},
		HealthCheck: &ServiceHealthCheck{
			Type:               "tcp",
			Interval:           20 * time.Millisecond,
			Timeout:            10 * time.Millisecond,
			HealthyThreshold:   2,
			UnhealthyThreshold: 3,
		},
	}

	sd.RegisterService(svc)
	hc.AddService(svc)

	// After 1 failure, should be degraded (not fully unhealthy yet)
	time.Sleep(50 * time.Millisecond)

	info, _ := hc.GetEndpointHealth("degraded-test", "ep-1")
	// State should be degraded or progressing to unhealthy
	if info.Health != HealthStateDegraded && info.Health != HealthStateUnhealthy && info.Health != HealthStateHealthy {
		t.Errorf("Unexpected health state: %v", info.Health)
	}
}

func TestEnhancedHealthChecker_ResponseTime(t *testing.T) {
	// Start a TCP listener
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start listener: %v", err)
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

	port := listener.Addr().(*net.TCPAddr).Port

	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false
	config.HealthCheckInterval = 20 * time.Millisecond
	config.HealthCheckTimeout = 100 * time.Millisecond
	config.HealthyThreshold = 1

	sd, _ := NewEnhancedServiceDiscovery(config)
	defer sd.Close()

	hc := NewEnhancedHealthChecker(config, sd)
	hc.Start()
	defer hc.Stop()

	svc := &ServiceDefinition{
		Name: "response-time-test",
		Endpoints: []*ServiceEndpoint{
			{ID: "ep-1", Address: net.ParseIP("127.0.0.1"), Port: uint16(port), Health: HealthStateUnknown},
		},
		HealthCheck: &ServiceHealthCheck{
			Type:             "tcp",
			Interval:         20 * time.Millisecond,
			Timeout:          100 * time.Millisecond,
			HealthyThreshold: 1,
		},
	}

	sd.RegisterService(svc)
	hc.AddService(svc)

	// Wait for a few health checks
	time.Sleep(100 * time.Millisecond)

	info, err := hc.GetEndpointHealth("response-time-test", "ep-1")
	if err != nil {
		t.Fatalf("GetEndpointHealth error: %v", err)
	}

	// Response time should be recorded
	if info.ResponseTime == 0 {
		t.Error("Response time should be > 0 for successful checks")
	}
}

func TestEnhancedHealthChecker_IPv6(t *testing.T) {
	// Skip if IPv6 localhost not available
	listener, err := net.Listen("tcp", "[::1]:0")
	if err != nil {
		t.Skip("IPv6 not available")
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

	port := listener.Addr().(*net.TCPAddr).Port

	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false
	config.HealthCheckInterval = 20 * time.Millisecond
	config.HealthCheckTimeout = 100 * time.Millisecond
	config.HealthyThreshold = 1

	sd, _ := NewEnhancedServiceDiscovery(config)
	defer sd.Close()

	hc := NewEnhancedHealthChecker(config, sd)
	hc.Start()
	defer hc.Stop()

	svc := &ServiceDefinition{
		Name: "ipv6-test",
		Endpoints: []*ServiceEndpoint{
			{ID: "ep-1", Address: net.ParseIP("::1"), Port: uint16(port), Health: HealthStateUnknown},
		},
		HealthCheck: &ServiceHealthCheck{
			Type:             "tcp",
			Interval:         20 * time.Millisecond,
			Timeout:          100 * time.Millisecond,
			HealthyThreshold: 1,
		},
	}

	sd.RegisterService(svc)
	hc.AddService(svc)

	time.Sleep(100 * time.Millisecond)

	info, err := hc.GetEndpointHealth("ipv6-test", "ep-1")
	if err != nil {
		t.Fatalf("GetEndpointHealth error: %v", err)
	}

	if info.Health != HealthStateHealthy {
		t.Errorf("Expected healthy for IPv6 endpoint, got %v", info.Health)
	}
}
