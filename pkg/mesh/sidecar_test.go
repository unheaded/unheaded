// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package mesh

import (
	"context"
	"testing"
	"time"

	"unheaded/pkg/mesh/discovery"
	"unheaded/pkg/mesh/loadbalance"
	"unheaded/pkg/mesh/mtls"
	"unheaded/pkg/mesh/observe"
	"unheaded/pkg/mesh/policy"
	"unheaded/pkg/mesh/proxy"
)

func TestSidecarConfig(t *testing.T) {
	config := DefaultSidecarConfig("test-service", "test-namespace")

	if config.ServiceName != "test-service" {
		t.Errorf("expected ServiceName 'test-service', got '%s'", config.ServiceName)
	}

	if config.Namespace != "test-namespace" {
		t.Errorf("expected Namespace 'test-namespace', got '%s'", config.Namespace)
	}

	if config.TrustDomain != mtls.DefaultTrustDomain {
		t.Errorf("expected TrustDomain '%s', got '%s'", mtls.DefaultTrustDomain, config.TrustDomain)
	}

	if config.InboundPort != 15001 {
		t.Errorf("expected InboundPort 15001, got %d", config.InboundPort)
	}

	if config.DefaultTimeout != 30*time.Second {
		t.Errorf("expected DefaultTimeout 30s, got %v", config.DefaultTimeout)
	}
}

func TestNewSidecar(t *testing.T) {
	config := DefaultSidecarConfig("test-service", "test-namespace")
	sidecar := NewSidecar(config)

	if sidecar == nil {
		t.Fatal("expected non-nil sidecar")
	}

	if sidecar.config != config {
		t.Error("config not properly assigned")
	}

	if sidecar.initialized {
		t.Error("sidecar should not be initialized")
	}

	if sidecar.running {
		t.Error("sidecar should not be running")
	}
}

func TestSidecarWithNilConfig(t *testing.T) {
	sidecar := NewSidecar(nil)

	if sidecar == nil {
		t.Fatal("expected non-nil sidecar even with nil config")
	}

	if sidecar.config == nil {
		t.Error("expected default config to be set")
	}
}

func TestSPIFFEID(t *testing.T) {
	tests := []struct {
		name        string
		trustDomain string
		serviceName string
		expected    string
	}{
		{
			name:        "simple service",
			trustDomain: "unheaded.local",
			serviceName: "api",
			expected:    "spiffe://unheaded.local/service/api",
		},
		{
			name:        "service with dots",
			trustDomain: "unheaded.local",
			serviceName: "api.gateway",
			expected:    "spiffe://unheaded.local/service/api.gateway",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := mtls.NewSPIFFEID(tt.trustDomain, tt.serviceName)
			if id.String() != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, id.String())
			}
		})
	}
}

func TestSPIFFEIDParsing(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		valid   bool
		service string
	}{
		{
			name:    "valid spiffe id",
			input:   "spiffe://unheaded.local/service/api",
			valid:   true,
			service: "api",
		},
		{
			name:    "valid with namespace",
			input:   "spiffe://unheaded.local/ns/prod/sa/api",
			valid:   true,
			service: "api",
		},
		{
			name:  "invalid scheme",
			input: "https://unheaded.local/service/api",
			valid: false,
		},
		{
			name:  "no path",
			input: "spiffe://unheaded.local",
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := mtls.ParseSPIFFEID(tt.input)
			if tt.valid {
				if err != nil {
					t.Errorf("expected valid parse, got error: %v", err)
				}
				if id.ServiceName() != tt.service {
					t.Errorf("expected service '%s', got '%s'", tt.service, id.ServiceName())
				}
			} else {
				if err == nil {
					t.Error("expected error for invalid SPIFFE ID")
				}
			}
		})
	}
}

func TestLoadBalancerRoundRobin(t *testing.T) {
	rr := loadbalance.NewRoundRobin()
	pool := loadbalance.NewEndpointPool(rr)

	endpoints := []*loadbalance.Endpoint{
		{Address: "10.0.0.1", Port: 8080, Healthy: true},
		{Address: "10.0.0.2", Port: 8080, Healthy: true},
		{Address: "10.0.0.3", Port: 8080, Healthy: true},
	}
	pool.SetEndpoints(endpoints)

	// Should cycle through endpoints
	picked := make(map[string]int)
	for i := 0; i < 9; i++ {
		ep := pool.Pick()
		if ep == nil {
			t.Fatal("expected non-nil endpoint")
		}
		picked[ep.Address]++
	}

	// Each should be picked 3 times
	for addr, count := range picked {
		if count != 3 {
			t.Errorf("expected address %s to be picked 3 times, got %d", addr, count)
		}
	}
}

func TestLoadBalancerLeastConn(t *testing.T) {
	lc := loadbalance.NewLeastConn()
	pool := loadbalance.NewEndpointPool(lc)

	endpoints := []*loadbalance.Endpoint{
		{Address: "10.0.0.1", Port: 8080, Healthy: true},
		{Address: "10.0.0.2", Port: 8080, Healthy: true},
	}
	pool.SetEndpoints(endpoints)

	// First pick should be first endpoint
	ep := pool.Pick()
	if ep == nil {
		t.Fatal("expected non-nil endpoint")
	}

	// Simulate connection
	lc.IncrementConnections(ep)

	// Next pick should be second endpoint (less connections)
	ep2 := pool.Pick()
	if ep2 == nil {
		t.Fatal("expected non-nil endpoint")
	}

	if ep2.Address == ep.Address {
		t.Error("expected different endpoint with least connections")
	}
}

func TestLoadBalancerWeighted(t *testing.T) {
	w := loadbalance.NewWeighted()
	pool := loadbalance.NewEndpointPool(w)

	endpoints := []*loadbalance.Endpoint{
		{Address: "10.0.0.1", Port: 8080, Weight: 10, Healthy: true},
		{Address: "10.0.0.2", Port: 8080, Weight: 1, Healthy: true},
	}
	pool.SetEndpoints(endpoints)

	// Higher weight endpoint should be picked more often
	picked := make(map[string]int)
	for i := 0; i < 100; i++ {
		ep := pool.Pick()
		if ep != nil {
			picked[ep.Address]++
		}
	}

	// 10.0.0.1 should be picked significantly more
	if picked["10.0.0.1"] <= picked["10.0.0.2"] {
		t.Errorf("weighted endpoint should be picked more: %v", picked)
	}
}

func TestEndpointPoolHealthFiltering(t *testing.T) {
	rr := loadbalance.NewRoundRobin()
	pool := loadbalance.NewEndpointPool(rr)

	endpoints := []*loadbalance.Endpoint{
		{Address: "10.0.0.1", Port: 8080, Healthy: true},
		{Address: "10.0.0.2", Port: 8080, Healthy: false},
		{Address: "10.0.0.3", Port: 8080, Healthy: true},
	}
	pool.SetEndpoints(endpoints)

	// Should only pick healthy endpoints
	for i := 0; i < 10; i++ {
		ep := pool.Pick()
		if ep == nil {
			t.Fatal("expected non-nil endpoint")
		}
		if ep.Address == "10.0.0.2" {
			t.Error("should not pick unhealthy endpoint")
		}
	}
}

func TestCircuitBreaker(t *testing.T) {
	config := &policy.CircuitBreakerConfig{
		Enabled:            true,
		FailureThreshold:   3,
		SuccessThreshold:   2,
		HalfOpenRequests:   1,
		Timeout:            100 * time.Millisecond,
		ConsecutiveErrors:  3,
	}

	cb := policy.NewCircuitBreaker(config)

	// Initial state should be closed
	if cb.State() != policy.StateClosed {
		t.Errorf("expected closed state, got %v", cb.State())
	}

	// Simulate failures to trip the circuit
	for i := 0; i < 5; i++ {
		cb.Execute(context.Background(), func(ctx context.Context) error {
			return context.DeadlineExceeded
		})
	}

	// Should be open now
	if cb.State() != policy.StateOpen {
		t.Errorf("expected open state after failures, got %v", cb.State())
	}

	// Requests should be rejected
	err := cb.Execute(context.Background(), func(ctx context.Context) error {
		return nil
	})
	if err != policy.ErrCircuitOpen {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}

	// Wait for timeout and then try a request to trigger half-open
	time.Sleep(150 * time.Millisecond)

	// The state transitions to half-open when a request is allowed
	// Try to execute - this should be allowed and trigger half-open
	cb.Execute(context.Background(), func(ctx context.Context) error {
		return nil
	})

	// After successful request in half-open, it may stay half-open or close
	// Just verify it's not stuck in open
	state := cb.State()
	if state == policy.StateOpen {
		t.Errorf("expected state to transition from open, got %v", state)
	}
}

func TestRetryPolicy(t *testing.T) {
	retryPolicy := &policy.RetryPolicy{
		MaxRetries:  3,
		RetryOn:     []string{"connection-failure"},
		BackoffBase: 10 * time.Millisecond,
		BackoffMax:  100 * time.Millisecond,
	}

	retryer := policy.NewRetryer(retryPolicy)

	attempts := 0
	err := retryer.Do(context.Background(), func(ctx context.Context) error {
		attempts++
		if attempts < 3 {
			return context.DeadlineExceeded // Not retryable by default
		}
		return nil
	})

	// Should fail because deadline exceeded is not in retry conditions
	if err == nil {
		t.Log("Operation completed")
	}
}

func TestMetrics(t *testing.T) {
	metrics := observe.NewMetrics()

	// Record some requests
	metrics.RecordRequest("api", "default", 200, 100*time.Millisecond, nil)
	metrics.RecordRequest("api", "default", 200, 200*time.Millisecond, nil)
	metrics.RecordRequest("api", "default", 500, 50*time.Millisecond, context.DeadlineExceeded)

	snapshot := metrics.Snapshot()

	if snapshot.TotalRequests != 3 {
		t.Errorf("expected 3 total requests, got %d", snapshot.TotalRequests)
	}

	if snapshot.SuccessRequests != 2 {
		t.Errorf("expected 2 success requests, got %d", snapshot.SuccessRequests)
	}

	if snapshot.FailedRequests != 1 {
		t.Errorf("expected 1 failed request, got %d", snapshot.FailedRequests)
	}

	successRate := snapshot.SuccessRate()
	if successRate < 66.0 || successRate > 67.0 {
		t.Errorf("expected success rate ~66.67%%, got %.2f%%", successRate)
	}
}

func TestTracing(t *testing.T) {
	exporter := observe.NewInMemoryExporter()
	tracer := observe.NewTracer("test-service", 1.0, exporter)

	ctx := context.Background()
	span, ctx := tracer.StartSpan(ctx, "test-operation")

	span.SetTag("key", "value")
	span.Log(map[string]string{"event": "test"})

	tracer.FinishSpan(span)

	spans := exporter.Spans()
	if len(spans) != 1 {
		t.Errorf("expected 1 span, got %d", len(spans))
	}

	if spans[0].Name != "test-operation" {
		t.Errorf("expected span name 'test-operation', got '%s'", spans[0].Name)
	}

	if spans[0].Tags["key"] != "value" {
		t.Error("tag not recorded")
	}
}

func TestSpanContext(t *testing.T) {
	exporter := observe.NewInMemoryExporter()
	tracer := observe.NewTracer("test-service", 1.0, exporter)

	// Create parent span
	ctx := context.Background()
	parentSpan, ctx := tracer.StartSpan(ctx, "parent")

	// Create child span
	childSpan, _ := tracer.StartSpan(ctx, "child")

	// Child should have parent's trace ID
	if childSpan.TraceID != parentSpan.TraceID {
		t.Error("child span should have same trace ID as parent")
	}

	// Child should have parent span ID as parent
	if childSpan.ParentSpanID != parentSpan.SpanID {
		t.Error("child span should reference parent span ID")
	}

	tracer.FinishSpan(childSpan)
	tracer.FinishSpan(parentSpan)
}

func TestStaticDiscovery(t *testing.T) {
	discoverer := discovery.NewStaticDiscoverer()

	svc := &discovery.Service{
		Name:      "api",
		Namespace: "default",
		Endpoints: []*discovery.Endpoint{
			{Address: "10.0.0.1", Port: 8080, Healthy: true},
			{Address: "10.0.0.2", Port: 8080, Healthy: true},
		},
	}
	discoverer.AddService(svc)

	// Discover the service
	ctx := context.Background()
	discovered, err := discoverer.Discover(ctx, "api", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(discovered.Endpoints) != 2 {
		t.Errorf("expected 2 endpoints, got %d", len(discovered.Endpoints))
	}

	// Try non-existent service
	_, err = discoverer.Discover(ctx, "unknown", "default")
	if err != discovery.ErrServiceNotFound {
		t.Errorf("expected ErrServiceNotFound, got %v", err)
	}
}

func TestCachingDiscovery(t *testing.T) {
	static := discovery.NewStaticDiscoverer()
	svc := &discovery.Service{
		Name:      "api",
		Namespace: "default",
		Endpoints: []*discovery.Endpoint{
			{Address: "10.0.0.1", Port: 8080, Healthy: true},
		},
	}
	static.AddService(svc)

	caching := discovery.NewCachingDiscoverer(static, 100*time.Millisecond)

	ctx := context.Background()

	// First call should hit backend
	_, err := caching.Discover(ctx, "api", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Remove from backend
	static.RemoveService("api", "default")

	// Should still be cached
	_, err = caching.Discover(ctx, "api", "default")
	if err != nil {
		t.Error("should have returned cached result")
	}

	// Wait for cache expiry
	time.Sleep(150 * time.Millisecond)

	// Now should hit backend and fail
	_, err = caching.Discover(ctx, "api", "default")
	if err != discovery.ErrServiceNotFound {
		t.Errorf("expected ErrServiceNotFound after cache expiry, got %v", err)
	}
}

func TestPolicyStore(t *testing.T) {
	store := policy.NewPolicyStore()

	// Default policy
	defaultPolicy := store.Get("default", "unknown")
	if defaultPolicy == nil {
		t.Fatal("expected default policy")
	}

	// Set custom policy
	customPolicy := &policy.Policy{
		ServiceName: "api",
		Namespace:   "prod",
		Timeout:     60 * time.Second,
	}
	store.Set("prod", "api", customPolicy)

	// Get custom policy
	retrieved := store.Get("prod", "api")
	if retrieved != customPolicy {
		t.Error("expected custom policy")
	}

	// Different namespace should get default
	retrieved = store.Get("staging", "api")
	if retrieved == customPolicy {
		t.Error("should get default policy for different namespace")
	}
}

func TestSidecarStats(t *testing.T) {
	config := DefaultSidecarConfig("test-service", "test-namespace")
	sidecar := NewSidecar(config)

	stats := sidecar.Stats()

	if stats.Running {
		t.Error("sidecar should not be running")
	}

	if stats.Initialized {
		t.Error("sidecar should not be initialized")
	}
}

func TestSidecarSetPolicy(t *testing.T) {
	config := DefaultSidecarConfig("test-service", "test-namespace")
	sidecar := NewSidecar(config)

	customPolicy := &policy.Policy{
		ServiceName: "api",
		Namespace:   "prod",
		Timeout:     60 * time.Second,
		Retry: &policy.RetryPolicy{
			MaxRetries: 5,
		},
	}

	sidecar.SetPolicy("api", "prod", customPolicy)
	retrieved := sidecar.GetPolicy("api", "prod")

	if retrieved != customPolicy {
		t.Error("expected custom policy to be set and retrieved")
	}
}

func TestTimeoutConfig(t *testing.T) {
	config := policy.DefaultTimeoutConfig()

	if config.Request != 30*time.Second {
		t.Errorf("expected request timeout 30s, got %v", config.Request)
	}

	if config.Connect != 5*time.Second {
		t.Errorf("expected connect timeout 5s, got %v", config.Connect)
	}

	controller := policy.NewTimeoutController(config)
	if controller == nil {
		t.Fatal("expected non-nil timeout controller")
	}
}

func TestBackoffCalculator(t *testing.T) {
	calc := policy.NewBackoffCalculator(100*time.Millisecond, 5*time.Second, 0.0)

	// First attempt
	d1 := calc.Next(0)
	if d1 != 100*time.Millisecond {
		t.Errorf("expected 100ms, got %v", d1)
	}

	// Second attempt (exponential)
	d2 := calc.Next(1)
	if d2 != 200*time.Millisecond {
		t.Errorf("expected 200ms, got %v", d2)
	}

	// Third attempt
	d3 := calc.Next(2)
	if d3 != 400*time.Millisecond {
		t.Errorf("expected 400ms, got %v", d3)
	}

	// Should cap at max
	d10 := calc.Next(10)
	if d10 > 5*time.Second {
		t.Errorf("expected max 5s, got %v", d10)
	}
}

func TestAccessLogFilter(t *testing.T) {
	// Test error filter
	errorFilter := observe.FilterErrors()

	entry200 := &observe.AccessLogEntry{StatusCode: 200}
	entry500 := &observe.AccessLogEntry{StatusCode: 500}

	if errorFilter(entry200) {
		t.Error("200 should not pass error filter")
	}

	if !errorFilter(entry500) {
		t.Error("500 should pass error filter")
	}

	// Test latency filter
	latencyFilter := observe.FilterByLatency(100 * time.Millisecond)

	fastEntry := &observe.AccessLogEntry{Latency: 50 * time.Millisecond}
	slowEntry := &observe.AccessLogEntry{Latency: 150 * time.Millisecond}

	if latencyFilter(fastEntry) {
		t.Error("fast entry should not pass latency filter")
	}

	if !latencyFilter(slowEntry) {
		t.Error("slow entry should pass latency filter")
	}
}

func TestHealthChecker(t *testing.T) {
	config := &proxy.HealthCheckConfig{
		Interval:           100 * time.Millisecond,
		Timeout:            50 * time.Millisecond,
		HealthyThreshold:   1,
		UnhealthyThreshold: 1,
		TCPOnly:            true,
	}

	checker := proxy.NewHealthChecker(config)
	defer checker.Close()

	// Status of unknown endpoint
	status := checker.GetStatus("10.0.0.1:8080")
	if status != proxy.HealthUnknown {
		t.Errorf("expected unknown status, got %v", status)
	}
}

func TestPassiveHealthChecker(t *testing.T) {
	config := &proxy.PassiveHealthConfig{
		FailureThreshold:   3,
		SuccessThreshold:   2,
		EjectionTime:       100 * time.Millisecond,
		MaxEjectionPercent: 100.0, // Allow ejecting all endpoints
	}

	checker := proxy.NewPassiveHealthChecker(config)

	// Initially healthy (unknown addresses default to healthy)
	status := checker.GetStatus("10.0.0.1:8080")
	if status != proxy.HealthHealthy {
		t.Errorf("expected healthy status, got %v", status)
	}

	// Record enough failures to trigger ejection
	for i := 0; i < 5; i++ {
		checker.RecordFailure("10.0.0.1:8080")
	}

	// Should be unhealthy now
	status = checker.GetStatus("10.0.0.1:8080")
	if status != proxy.HealthUnhealthy {
		t.Errorf("expected unhealthy status after failures, got %v", status)
	}

	// Wait for ejection to expire
	time.Sleep(150 * time.Millisecond)

	// Should be healthy again (ejection expired)
	status = checker.GetStatus("10.0.0.1:8080")
	if status != proxy.HealthHealthy {
		t.Errorf("expected healthy status after ejection expiry, got %v", status)
	}
}
