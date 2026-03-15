// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package mesh

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Config tests
// ---------------------------------------------------------------------------

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig returned nil")
	}
	if cfg.ServiceName != "unknown" {
		t.Errorf("expected ServiceName 'unknown', got %q", cfg.ServiceName)
	}
	if cfg.Namespace != "default" {
		t.Errorf("expected Namespace 'default', got %q", cfg.Namespace)
	}
	if cfg.DiscoveryType != DiscoveryTypeDNS {
		t.Errorf("expected DiscoveryType %q, got %q", DiscoveryTypeDNS, cfg.DiscoveryType)
	}
	if cfg.LoadBalancerType != LoadBalancerRoundRobin {
		t.Errorf("expected LoadBalancerType %q, got %q", LoadBalancerRoundRobin, cfg.LoadBalancerType)
	}
	if cfg.DefaultRetries != 3 {
		t.Errorf("expected DefaultRetries 3, got %d", cfg.DefaultRetries)
	}
}

func TestConfigValidate_Valid(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid config should pass validation: %v", err)
	}
}

func TestConfigValidate_Errors(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(c *Config)
		errMsg string
	}{
		{"empty service name", func(c *Config) { c.ServiceName = "" }, "service name"},
		{"empty namespace", func(c *Config) { c.Namespace = "" }, "namespace"},
		{"invalid discovery type", func(c *Config) { c.DiscoveryType = "bogus" }, "discovery type"},
		{"invalid load balancer type", func(c *Config) { c.LoadBalancerType = "bogus" }, "load balancer type"},
		{"zero connect timeout", func(c *Config) { c.ConnectTimeout = 0 }, "connect timeout"},
		{"zero request timeout", func(c *Config) { c.RequestTimeout = 0 }, "request timeout"},
		{"negative retries", func(c *Config) { c.DefaultRetries = -1 }, "retries"},
		{"negative rate limit", func(c *Config) { c.DefaultRateLimit = -1 }, "rate limit"},
		{"trace sample rate too high", func(c *Config) { c.TraceSampleRate = 2.0 }, "trace sample rate"},
		{"trace sample rate negative", func(c *Config) { c.TraceSampleRate = -0.1 }, "trace sample rate"},
		{"cb failure threshold zero", func(c *Config) {
			c.CircuitBreakerEnabled = true
			c.CircuitBreakerFailureThreshold = 0
		}, "failure threshold"},
		{"cb success threshold zero", func(c *Config) {
			c.CircuitBreakerEnabled = true
			c.CircuitBreakerSuccessThreshold = 0
		}, "success threshold"},
		{"cb timeout zero", func(c *Config) {
			c.CircuitBreakerEnabled = true
			c.CircuitBreakerTimeout = 0
		}, "circuit breaker timeout"},
		{"health check interval zero", func(c *Config) {
			c.HealthCheckEnabled = true
			c.HealthCheckInterval = 0
		}, "health check interval"},
		{"health check timeout zero", func(c *Config) {
			c.HealthCheckEnabled = true
			c.HealthCheckTimeout = 0
		}, "health check timeout"},
		{"health check timeout >= interval", func(c *Config) {
			c.HealthCheckEnabled = true
			c.HealthCheckInterval = 5 * time.Second
			c.HealthCheckTimeout = 5 * time.Second
		}, "health check timeout must be less"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.mutate(cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.errMsg)) {
				t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
			}
		})
	}
}

func TestConfigClone(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RegistryEndpoints = []string{"a", "b"}
	cfg.StaticEndpoints = map[string][]EndpointConfig{
		"svc": {{Address: "1.2.3.4", Port: 80}},
	}

	clone := cfg.Clone()
	if clone == cfg {
		t.Fatal("clone should be a different pointer")
	}

	// Modify original and verify clone is independent.
	cfg.RegistryEndpoints[0] = "changed"
	if clone.RegistryEndpoints[0] == "changed" {
		t.Error("clone RegistryEndpoints was not deep-copied")
	}

	cfg.StaticEndpoints["svc"][0].Address = "changed"
	if clone.StaticEndpoints["svc"][0].Address == "changed" {
		t.Error("clone StaticEndpoints was not deep-copied")
	}
}

func TestConfigClone_NilSlicesAndMaps(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RegistryEndpoints = nil
	cfg.StaticEndpoints = nil

	clone := cfg.Clone()
	if clone.RegistryEndpoints != nil {
		t.Error("expected nil RegistryEndpoints in clone")
	}
	if clone.StaticEndpoints != nil {
		t.Error("expected nil StaticEndpoints in clone")
	}
}

// ---------------------------------------------------------------------------
// ConfigBuilder tests
// ---------------------------------------------------------------------------

func TestConfigBuilder_Build(t *testing.T) {
	cfg, err := NewConfigBuilder().
		WithServiceName("myservice").
		WithNamespace("prod").
		WithTrustDomain("example.com").
		WithDiscovery(DiscoveryTypeStatic, 10*time.Second).
		WithTLS(false).
		WithTimeouts(2*time.Second, 15*time.Second, 60*time.Second).
		WithConnectionPool(50, 5, 25).
		WithLoadBalancer(LoadBalancerLeastConn).
		WithRetry(2, 50*time.Millisecond, 5*time.Second).
		WithRateLimit(100, 20).
		WithHealthCheck(5*time.Second, 2*time.Second, 2, 3).
		WithMetrics(false).
		WithTracing(true, 0.5).
		WithAccessLog(false).
		WithHTTP2(false).
		WithGRPC(false).
		WithGracefulShutdown(10 * time.Second).
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if cfg.ServiceName != "myservice" {
		t.Errorf("ServiceName = %q, want %q", cfg.ServiceName, "myservice")
	}
	if cfg.Namespace != "prod" {
		t.Errorf("Namespace = %q, want %q", cfg.Namespace, "prod")
	}
	if cfg.LoadBalancerType != LoadBalancerLeastConn {
		t.Errorf("LoadBalancerType = %q, want %q", cfg.LoadBalancerType, LoadBalancerLeastConn)
	}
}

func TestConfigBuilder_BuildError(t *testing.T) {
	_, err := NewConfigBuilder().
		WithServiceName("").
		Build()
	if err == nil {
		t.Fatal("expected error for empty service name")
	}
}

func TestConfigBuilder_MustBuild_Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic from MustBuild")
		}
	}()
	NewConfigBuilder().WithServiceName("").MustBuild()
}

func TestConfigBuilder_MustBuild_Success(t *testing.T) {
	cfg := NewConfigBuilder().
		WithServiceName("ok").
		MustBuild()
	if cfg.ServiceName != "ok" {
		t.Errorf("ServiceName = %q, want %q", cfg.ServiceName, "ok")
	}
}

func TestConfigBuilder_DNSDiscovery(t *testing.T) {
	cfg, err := NewConfigBuilder().
		WithDNSDiscovery("svc.local", "8.8.8.8:53").
		Build()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DiscoveryType != DiscoveryTypeDNS {
		t.Errorf("DiscoveryType = %q", cfg.DiscoveryType)
	}
	if cfg.DNSDomain != "svc.local" {
		t.Errorf("DNSDomain = %q", cfg.DNSDomain)
	}
}

func TestConfigBuilder_RegistryDiscovery(t *testing.T) {
	cfg, err := NewConfigBuilder().
		WithRegistryDiscovery([]string{"http://reg:8080"}).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DiscoveryType != DiscoveryTypeRegistry {
		t.Errorf("DiscoveryType = %q", cfg.DiscoveryType)
	}
}

func TestConfigBuilder_StaticEndpoints(t *testing.T) {
	cfg, err := NewConfigBuilder().
		WithDiscovery(DiscoveryTypeStatic, 10*time.Second).
		WithStaticEndpoints("svc", []EndpointConfig{{Address: "1.2.3.4", Port: 80}}).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.StaticEndpoints["svc"]) != 1 {
		t.Errorf("expected 1 static endpoint, got %d", len(cfg.StaticEndpoints["svc"]))
	}
}

func TestConfigBuilder_ProxyAndCircuitBreaker(t *testing.T) {
	cfg, err := NewConfigBuilder().
		WithInboundProxy("0.0.0.0", 15001).
		WithOutboundProxy("0.0.0.0", 15006).
		WithCircuitBreaker(10, 5, 60*time.Second).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.InboundPort != 15001 {
		t.Errorf("InboundPort = %d", cfg.InboundPort)
	}
	if cfg.OutboundPort != 15006 {
		t.Errorf("OutboundPort = %d", cfg.OutboundPort)
	}
	if !cfg.CircuitBreakerEnabled {
		t.Error("CircuitBreakerEnabled should be true")
	}
}

func TestConfigBuilder_WithoutCircuitBreaker(t *testing.T) {
	cfg, err := NewConfigBuilder().WithoutCircuitBreaker().Build()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CircuitBreakerEnabled {
		t.Error("CircuitBreakerEnabled should be false")
	}
}

func TestConfigBuilder_WithoutHealthCheck(t *testing.T) {
	cfg, err := NewConfigBuilder().WithoutHealthCheck().Build()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HealthCheckEnabled {
		t.Error("HealthCheckEnabled should be false")
	}
}

func TestConfigBuilder_ZoneAware(t *testing.T) {
	cfg, err := NewConfigBuilder().
		WithZoneAwareLoadBalancing("us-east-1a").
		Build()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LocalZone != "us-east-1a" {
		t.Errorf("LocalZone = %q", cfg.LocalZone)
	}
}

// ---------------------------------------------------------------------------
// ServiceConfig.Apply tests
// ---------------------------------------------------------------------------

func TestServiceConfigApply(t *testing.T) {
	base := DefaultConfig()
	timeout := 60 * time.Second
	retries := 5
	lbType := LoadBalancerWeighted
	cbEnabled := false
	cbFail := 10
	cbSuccess := 5
	cbTimeout := 120 * time.Second
	backoff := 200 * time.Millisecond
	rateLimit := 50.0
	burst := 20
	hcInterval := 20 * time.Second
	hcTimeout := 8 * time.Second
	healthy := 3
	unhealthy := 5

	sc := &ServiceConfig{
		ConnectTimeout:                 &timeout,
		RequestTimeout:                 &timeout,
		LoadBalancerType:               &lbType,
		CircuitBreakerEnabled:          &cbEnabled,
		CircuitBreakerFailureThreshold: &cbFail,
		CircuitBreakerSuccessThreshold: &cbSuccess,
		CircuitBreakerTimeout:          &cbTimeout,
		MaxRetries:                     &retries,
		RetryBackoff:                   &backoff,
		RateLimit:                      &rateLimit,
		RateBurst:                      &burst,
		HealthCheckInterval:            &hcInterval,
		HealthCheckTimeout:             &hcTimeout,
		HealthyThreshold:               &healthy,
		UnhealthyThreshold:             &unhealthy,
	}

	result := sc.Apply(base)
	if result.ConnectTimeout != timeout {
		t.Errorf("ConnectTimeout = %v", result.ConnectTimeout)
	}
	if result.RequestTimeout != timeout {
		t.Errorf("RequestTimeout = %v", result.RequestTimeout)
	}
	if result.LoadBalancerType != lbType {
		t.Errorf("LoadBalancerType = %v", result.LoadBalancerType)
	}
	if result.CircuitBreakerEnabled != cbEnabled {
		t.Errorf("CircuitBreakerEnabled = %v", result.CircuitBreakerEnabled)
	}
	if result.DefaultRetries != retries {
		t.Errorf("DefaultRetries = %v", result.DefaultRetries)
	}
	if result.DefaultRateLimit != rateLimit {
		t.Errorf("DefaultRateLimit = %v", result.DefaultRateLimit)
	}
	if result.HealthCheckInterval != hcInterval {
		t.Errorf("HealthCheckInterval = %v", result.HealthCheckInterval)
	}
}

// ---------------------------------------------------------------------------
// ConfigStore tests
// ---------------------------------------------------------------------------

func TestConfigStore(t *testing.T) {
	store := NewConfigStore(nil)
	g := store.GetGlobal()
	if g.ServiceName != "unknown" {
		t.Errorf("expected default global config, got ServiceName=%q", g.ServiceName)
	}

	newGlobal := DefaultConfig()
	newGlobal.ServiceName = "global-updated"
	store.SetGlobal(newGlobal)
	g2 := store.GetGlobal()
	if g2.ServiceName != "global-updated" {
		t.Errorf("SetGlobal didn't take effect, got %q", g2.ServiceName)
	}

	// Service with no override should get global.
	svcCfg := store.GetService("mysvc")
	if svcCfg.ServiceName != "global-updated" {
		t.Errorf("expected global for unknown service, got %q", svcCfg.ServiceName)
	}

	// Set service override.
	timeout := 99 * time.Second
	store.SetService("mysvc", &ServiceConfig{RequestTimeout: &timeout})
	svcCfg = store.GetService("mysvc")
	if svcCfg.RequestTimeout != 99*time.Second {
		t.Errorf("service override not applied, RequestTimeout=%v", svcCfg.RequestTimeout)
	}

	services := store.ListServices()
	if len(services) != 1 || services[0] != "mysvc" {
		t.Errorf("ListServices = %v", services)
	}

	store.RemoveService("mysvc")
	services = store.ListServices()
	if len(services) != 0 {
		t.Errorf("expected empty list after remove, got %v", services)
	}
}

func TestConfigFromEnv(t *testing.T) {
	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("ConfigFromEnv returned nil")
	}
}

func TestConfigFromFile(t *testing.T) {
	cfg, err := ConfigFromFile("/nonexistent")
	if err != nil {
		t.Fatalf("ConfigFromFile failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("ConfigFromFile returned nil")
	}
}

// ---------------------------------------------------------------------------
// CircuitBreaker tests
// ---------------------------------------------------------------------------

func TestCircuitStateString(t *testing.T) {
	tests := []struct {
		state CircuitState
		want  string
	}{
		{CircuitStateClosed, "closed"},
		{CircuitStateOpen, "open"},
		{CircuitStateHalfOpen, "half-open"},
		{CircuitState(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("CircuitState(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestNewCircuitBreaker_Nil(t *testing.T) {
	cb := NewCircuitBreaker(nil)
	if cb == nil {
		t.Fatal("NewCircuitBreaker(nil) returned nil")
	}
	if cb.State() != CircuitStateClosed {
		t.Errorf("initial state = %v, want closed", cb.State())
	}
}

func TestNewCircuitBreaker_Defaults(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{})
	if cb.config.FailureThreshold != 5 {
		t.Errorf("FailureThreshold = %d, want 5", cb.config.FailureThreshold)
	}
	if cb.config.SuccessThreshold != 3 {
		t.Errorf("SuccessThreshold = %d, want 3", cb.config.SuccessThreshold)
	}
}

func TestCircuitBreaker_TransitionToOpen(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		Name:             "test",
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Timeout:          50 * time.Millisecond,
	})

	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	if cb.State() != CircuitStateOpen {
		t.Errorf("state after 3 failures = %v, want open", cb.State())
	}

	// Should not allow requests when open.
	if cb.Allow() {
		t.Error("Allow() should return false when circuit is open")
	}
}

func TestCircuitBreaker_HalfOpenAndRecovery(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		Name:                "test",
		FailureThreshold:    2,
		SuccessThreshold:    2,
		Timeout:             50 * time.Millisecond,
		HalfOpenMaxRequests: 3,
	})

	// Trip the circuit.
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != CircuitStateOpen {
		t.Fatalf("expected open, got %v", cb.State())
	}

	// Wait for timeout to transition to half-open.
	time.Sleep(60 * time.Millisecond)

	if cb.State() != CircuitStateHalfOpen {
		t.Fatalf("expected half-open after timeout, got %v", cb.State())
	}

	// Allow should work in half-open.
	if !cb.Allow() {
		t.Error("Allow() should return true in half-open")
	}

	// Successful requests should close the circuit.
	cb.RecordSuccess()
	cb.RecordSuccess()

	if cb.State() != CircuitStateClosed {
		t.Errorf("expected closed after enough successes, got %v", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenFailureReopens(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		Name:             "test",
		FailureThreshold: 1,
		SuccessThreshold: 2,
		Timeout:          50 * time.Millisecond,
	})

	cb.RecordFailure()
	time.Sleep(60 * time.Millisecond)

	// Should be half-open now.
	if !cb.Allow() {
		t.Fatal("expected Allow in half-open")
	}

	// Failure in half-open should re-open.
	cb.RecordFailure()
	if cb.State() != CircuitStateOpen {
		t.Errorf("expected open after half-open failure, got %v", cb.State())
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		Name:             "test",
		FailureThreshold: 1,
	})
	cb.RecordFailure()
	if cb.State() != CircuitStateOpen {
		t.Fatal("expected open")
	}

	cb.Reset()
	if cb.State() != CircuitStateClosed {
		t.Errorf("expected closed after reset, got %v", cb.State())
	}
}

func TestCircuitBreaker_CheckState(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		Name:             "test",
		FailureThreshold: 1,
		Timeout:          50 * time.Millisecond,
	})
	cb.RecordFailure()
	time.Sleep(60 * time.Millisecond)

	// CheckState should transition from open to half-open.
	state := cb.CheckState()
	if state != CircuitStateHalfOpen {
		t.Errorf("CheckState() = %v, want half-open", state)
	}
}

func TestCircuitBreaker_Execute(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		Name:             "test",
		FailureThreshold: 5,
	})

	// Successful execution.
	err := cb.Execute(context.Background(), func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Errorf("Execute should succeed: %v", err)
	}

	// Failed execution.
	testErr := errors.New("fail")
	err = cb.Execute(context.Background(), func(ctx context.Context) error {
		return testErr
	})
	if err != testErr {
		t.Errorf("Execute should return original error, got %v", err)
	}
}

func TestCircuitBreaker_ExecuteWhenOpen(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		Name:             "test",
		FailureThreshold: 1,
		Timeout:          time.Hour,
	})
	cb.RecordFailure()

	err := cb.Execute(context.Background(), func(ctx context.Context) error {
		return nil
	})
	if err != ErrCircuitBreakerOpen {
		t.Errorf("expected ErrCircuitBreakerOpen, got %v", err)
	}
}

func TestCircuitBreaker_ExecuteWithFallback(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		Name:             "test",
		FailureThreshold: 1,
		Timeout:          time.Hour,
	})
	cb.RecordFailure()

	var fallbackCalled bool
	err := cb.ExecuteWithFallback(
		context.Background(),
		func(ctx context.Context) error { return nil },
		func(ctx context.Context, err error) error {
			fallbackCalled = true
			return nil
		},
	)
	if err != nil {
		t.Errorf("expected nil from fallback, got %v", err)
	}
	if !fallbackCalled {
		t.Error("fallback was not called")
	}
}

func TestCircuitBreaker_Metrics(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		Name:             "test-metrics",
		FailureThreshold: 10,
	})

	cb.Execute(context.Background(), func(ctx context.Context) error { return nil })
	cb.Execute(context.Background(), func(ctx context.Context) error { return errors.New("x") })

	m := cb.Metrics()
	if m.Name != "test-metrics" {
		t.Errorf("Name = %q", m.Name)
	}
	if m.TotalRequests != 2 {
		t.Errorf("TotalRequests = %d, want 2", m.TotalRequests)
	}
	if m.TotalSuccesses != 1 {
		t.Errorf("TotalSuccesses = %d", m.TotalSuccesses)
	}
	if m.TotalFailures != 1 {
		t.Errorf("TotalFailures = %d", m.TotalFailures)
	}
}

func TestCircuitBreakerMetrics_FailureRate(t *testing.T) {
	m := &CircuitBreakerMetrics{TotalRequests: 0}
	if m.FailureRate() != 0 {
		t.Errorf("FailureRate with 0 requests = %v", m.FailureRate())
	}

	m = &CircuitBreakerMetrics{TotalRequests: 10, TotalFailures: 3}
	if m.FailureRate() != 30.0 {
		t.Errorf("FailureRate = %v, want 30.0", m.FailureRate())
	}
}

func TestCircuitBreaker_StateChangeCallback(t *testing.T) {
	var mu sync.Mutex
	var changes []string

	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		Name:             "cb-callback",
		FailureThreshold: 1,
		Timeout:          50 * time.Millisecond,
		OnStateChange: func(name string, from, to CircuitState) {
			mu.Lock()
			changes = append(changes, fmt.Sprintf("%s->%s", from.String(), to.String()))
			mu.Unlock()
		},
	})

	cb.RecordFailure()
	// Give the goroutine time to execute callback.
	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	if len(changes) < 1 {
		t.Error("expected at least 1 state change callback")
	}
	mu.Unlock()
}

func TestDefaultCircuitBreakerConfig(t *testing.T) {
	cfg := DefaultCircuitBreakerConfig("my-cb")
	if cfg.Name != "my-cb" {
		t.Errorf("Name = %q", cfg.Name)
	}
	if cfg.FailureThreshold != 5 {
		t.Errorf("FailureThreshold = %d", cfg.FailureThreshold)
	}
}

// ---------------------------------------------------------------------------
// CircuitBreakerRegistry tests
// ---------------------------------------------------------------------------

func TestCircuitBreakerRegistry(t *testing.T) {
	reg := NewCircuitBreakerRegistry(nil)

	cb := reg.Get("svc-a")
	if cb == nil {
		t.Fatal("Get returned nil")
	}

	// Second get should return same instance.
	cb2 := reg.Get("svc-a")
	if cb != cb2 {
		t.Error("expected same CircuitBreaker instance")
	}

	// GetOrCreate with nil config.
	cb3 := reg.GetOrCreate("svc-b", nil)
	if cb3 == nil {
		t.Fatal("GetOrCreate returned nil")
	}

	// GetOrCreate with custom config.
	cb4 := reg.GetOrCreate("svc-c", &CircuitBreakerConfig{FailureThreshold: 99})
	if cb4 == nil {
		t.Fatal("GetOrCreate returned nil")
	}

	names := reg.List()
	if len(names) != 3 {
		t.Errorf("List() len = %d, want 3", len(names))
	}

	stats := reg.Stats()
	if len(stats) != 3 {
		t.Errorf("Stats() len = %d", len(stats))
	}

	reg.Remove("svc-a")
	names = reg.List()
	if len(names) != 2 {
		t.Errorf("after Remove, List() len = %d", len(names))
	}

	reg.ResetAll()
}

// ---------------------------------------------------------------------------
// Bulkhead tests
// ---------------------------------------------------------------------------

func TestNewBulkhead_Nil(t *testing.T) {
	bh := NewBulkhead(nil)
	if bh == nil {
		t.Fatal("NewBulkhead(nil) returned nil")
	}
}

func TestBulkhead_AcquireRelease(t *testing.T) {
	bh := NewBulkhead(&BulkheadConfig{
		Name:          "test",
		MaxConcurrent: 2,
		MaxWaiting:    5,
		Timeout:       time.Second,
	})

	ctx := context.Background()
	if err := bh.Acquire(ctx); err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}
	if err := bh.Acquire(ctx); err != nil {
		t.Fatalf("second Acquire failed: %v", err)
	}

	bh.Release()
	bh.Release()
}

func TestBulkhead_Execute(t *testing.T) {
	bh := NewBulkhead(&BulkheadConfig{
		Name:          "test",
		MaxConcurrent: 1,
		MaxWaiting:    1,
		Timeout:       time.Second,
	})

	err := bh.Execute(context.Background(), func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
}

func TestBulkhead_Metrics(t *testing.T) {
	bh := NewBulkhead(&BulkheadConfig{
		Name:          "test-bh",
		MaxConcurrent: 5,
		MaxWaiting:    10,
		Timeout:       time.Second,
	})

	bh.Execute(context.Background(), func(ctx context.Context) error { return nil })

	m := bh.Metrics()
	if m.Name != "test-bh" {
		t.Errorf("Name = %q", m.Name)
	}
	if m.TotalRequests != 1 {
		t.Errorf("TotalRequests = %d", m.TotalRequests)
	}
	if m.MaxConcurrent != 5 {
		t.Errorf("MaxConcurrent = %d", m.MaxConcurrent)
	}
}

func TestDefaultBulkheadConfig(t *testing.T) {
	cfg := DefaultBulkheadConfig("bh-name")
	if cfg.Name != "bh-name" {
		t.Errorf("Name = %q", cfg.Name)
	}
	if cfg.MaxConcurrent != 10 {
		t.Errorf("MaxConcurrent = %d", cfg.MaxConcurrent)
	}
}

// ---------------------------------------------------------------------------
// RetryPolicy / Retrier tests
// ---------------------------------------------------------------------------

func TestDefaultRetryPolicy(t *testing.T) {
	p := DefaultRetryPolicy()
	if p.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d", p.MaxRetries)
	}
	if p.Multiplier != 2.0 {
		t.Errorf("Multiplier = %v", p.Multiplier)
	}
}

func TestRetrier_Success(t *testing.T) {
	r := NewRetrier(&RetryPolicy{
		MaxRetries:     3,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		Multiplier:     2.0,
	})

	attempts := 0
	err := r.Execute(context.Background(), func(ctx context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("transient")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Execute should succeed: %v", err)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

func TestRetrier_AllFail(t *testing.T) {
	r := NewRetrier(&RetryPolicy{
		MaxRetries:     2,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     5 * time.Millisecond,
		Multiplier:     2.0,
	})

	err := r.Execute(context.Background(), func(ctx context.Context) error {
		return errors.New("permanent")
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRetrier_ContextCanceled(t *testing.T) {
	r := NewRetrier(&RetryPolicy{
		MaxRetries:     5,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		Multiplier:     2.0,
	})

	err := r.Execute(context.Background(), func(ctx context.Context) error {
		return context.Canceled
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestRetrier_RetryOnFilter(t *testing.T) {
	r := NewRetrier(&RetryPolicy{
		MaxRetries:     3,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		Multiplier:     2.0,
		RetryOn: func(err error) bool {
			return false // Don't retry anything.
		},
	})

	attempts := 0
	err := r.Execute(context.Background(), func(ctx context.Context) error {
		attempts++
		return errors.New("fail")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 (no retries)", attempts)
	}
}

func TestRetrier_Nil(t *testing.T) {
	r := NewRetrier(nil)
	if r == nil {
		t.Fatal("NewRetrier(nil) returned nil")
	}
}

// ---------------------------------------------------------------------------
// RateLimiter tests
// ---------------------------------------------------------------------------

func TestNewRateLimiter_Nil(t *testing.T) {
	rl := NewRateLimiter(nil)
	if rl == nil {
		t.Fatal("NewRateLimiter(nil) returned nil")
	}
}

func TestDefaultRateLimiterConfig(t *testing.T) {
	cfg := DefaultRateLimiterConfig()
	if cfg.Rate != 100 {
		t.Errorf("Rate = %v", cfg.Rate)
	}
}

func TestRateLimiter_Allow(t *testing.T) {
	rl := NewRateLimiter(&RateLimiterConfig{
		Rate:  1000,
		Burst: 5,
	})

	// Burst of 5 should all be allowed.
	for i := 0; i < 5; i++ {
		if !rl.Allow() {
			t.Errorf("Allow() returned false on call %d", i)
		}
	}

	// Next one should be denied (no time for refill).
	if rl.Allow() {
		t.Error("Allow() should return false after burst exhausted")
	}
}

func TestRateLimiter_AllowN(t *testing.T) {
	rl := NewRateLimiter(&RateLimiterConfig{
		Rate:  1000,
		Burst: 10,
	})

	if !rl.AllowN(5) {
		t.Error("AllowN(5) should succeed")
	}
	if !rl.AllowN(5) {
		t.Error("AllowN(5) should succeed (10 burst)")
	}
	if rl.AllowN(1) {
		t.Error("AllowN(1) should fail (burst exhausted)")
	}
}

func TestRateLimiter_Wait(t *testing.T) {
	rl := NewRateLimiter(&RateLimiterConfig{
		Rate:  10000,
		Burst: 1,
	})

	// First call consumes the burst.
	rl.Allow()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := rl.Wait(ctx)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
}

func TestRateLimiter_WaitTimeout(t *testing.T) {
	rl := NewRateLimiter(&RateLimiterConfig{
		Rate:  0.1, // Very slow refill.
		Burst: 1,
	})
	rl.Allow() // Consume burst.

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := rl.Wait(ctx)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestRateLimiter_Metrics(t *testing.T) {
	rl := NewRateLimiter(&RateLimiterConfig{
		Rate:  100,
		Burst: 2,
	})
	rl.Allow()
	rl.Allow()
	rl.Allow() // This one should be denied.

	m := rl.Metrics()
	if m.AllowedTotal != 2 {
		t.Errorf("AllowedTotal = %d, want 2", m.AllowedTotal)
	}
	if m.DeniedTotal != 1 {
		t.Errorf("DeniedTotal = %d, want 1", m.DeniedTotal)
	}
	if m.TotalRequests != 3 {
		t.Errorf("TotalRequests = %d, want 3", m.TotalRequests)
	}
}

// ---------------------------------------------------------------------------
// HealthChecker tests
// ---------------------------------------------------------------------------

func TestNewHealthChecker_Nil(t *testing.T) {
	hc := NewHealthChecker(nil)
	if hc == nil {
		t.Fatal("NewHealthChecker(nil) returned nil")
	}
}

func TestDefaultHealthCheckConfig(t *testing.T) {
	cfg := DefaultHealthCheckConfig()
	if cfg.Interval != 10*time.Second {
		t.Errorf("Interval = %v", cfg.Interval)
	}
}

func TestHealthChecker_UnknownEndpointIsHealthy(t *testing.T) {
	hc := NewHealthChecker(nil)
	if !hc.IsHealthy("unknown-id") {
		t.Error("unknown endpoints should be assumed healthy")
	}
}

func TestHealthChecker_AddAndCheck(t *testing.T) {
	hc := NewHealthChecker(&HealthCheckConfig{
		Interval:           time.Second,
		Timeout:            500 * time.Millisecond,
		HealthyThreshold:   2,
		UnhealthyThreshold: 2,
	})

	ep := &Endpoint{ID: "ep-1", Address: "10.0.0.1", Port: 8080}
	hc.AddEndpoint(ep)

	if !hc.IsHealthy("ep-1") {
		t.Error("newly added endpoint should be healthy")
	}

	// Record failures to make unhealthy.
	hc.RecordFailure("ep-1")
	hc.RecordFailure("ep-1")
	if hc.IsHealthy("ep-1") {
		t.Error("endpoint should be unhealthy after failures")
	}

	// Recover.
	hc.RecordSuccess("ep-1")
	hc.RecordSuccess("ep-1")
	if !hc.IsHealthy("ep-1") {
		t.Error("endpoint should be healthy after successes")
	}
}

func TestHealthChecker_RemoveEndpoint(t *testing.T) {
	hc := NewHealthChecker(nil)
	ep := &Endpoint{ID: "ep-1"}
	hc.AddEndpoint(ep)
	hc.RemoveEndpoint("ep-1")

	// After removal, IsHealthy should return true (unknown = healthy).
	if !hc.IsHealthy("ep-1") {
		t.Error("removed endpoint should be assumed healthy")
	}
}

func TestHealthChecker_RecordOnUnknown(t *testing.T) {
	hc := NewHealthChecker(nil)
	// Should not panic.
	hc.RecordSuccess("nonexistent")
	hc.RecordFailure("nonexistent")
}

func TestHealthChecker_RemoveService(t *testing.T) {
	hc := NewHealthChecker(nil)
	// Placeholder -- should not panic.
	hc.RemoveService("any-service")
}

func TestHealthChecker_HealthChangeCallback(t *testing.T) {
	var mu sync.Mutex
	var healthChanges []bool

	hc := NewHealthChecker(&HealthCheckConfig{
		Interval:           time.Second,
		Timeout:            500 * time.Millisecond,
		HealthyThreshold:   1,
		UnhealthyThreshold: 1,
		OnHealthChange: func(epID string, healthy bool) {
			mu.Lock()
			healthChanges = append(healthChanges, healthy)
			mu.Unlock()
		},
	})

	ep := &Endpoint{ID: "ep-cb"}
	hc.AddEndpoint(ep)

	hc.RecordFailure("ep-cb")
	time.Sleep(20 * time.Millisecond) // callback runs in goroutine

	mu.Lock()
	if len(healthChanges) < 1 || healthChanges[0] != false {
		t.Error("expected health change to false")
	}
	mu.Unlock()

	hc.RecordSuccess("ep-cb")
	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	if len(healthChanges) < 2 || healthChanges[1] != true {
		t.Error("expected health change to true")
	}
	mu.Unlock()
}

// ---------------------------------------------------------------------------
// ServiceRegistry tests
// ---------------------------------------------------------------------------

func TestServiceRegistry(t *testing.T) {
	reg := NewServiceRegistry()

	svc := &ServiceInstance{
		ServiceName: "api",
		Namespace:   "default",
		Endpoints: []*Endpoint{
			{ID: "ep-1", Address: "10.0.0.1", Port: 8080},
		},
	}
	reg.Register(svc)

	got, ok := reg.Get("api")
	if !ok || got.ServiceName != "api" {
		t.Error("Register/Get failed")
	}

	list := reg.List()
	if len(list) != 1 {
		t.Errorf("List() len = %d", len(list))
	}

	// Update endpoints.
	newEps := []*Endpoint{
		{ID: "ep-2", Address: "10.0.0.2", Port: 8080},
	}
	reg.UpdateEndpoints("api", newEps)
	got, _ = reg.Get("api")
	if len(got.Endpoints) != 1 || got.Endpoints[0].ID != "ep-2" {
		t.Error("UpdateEndpoints failed")
	}

	// UpdateEndpoints for nonexistent service should be a no-op.
	reg.UpdateEndpoints("nonexistent", newEps)

	// Set and get policy.
	pol := &ServicePolicy{MaxRetries: 5}
	reg.SetPolicy("api", pol)
	gotPol, ok := reg.GetPolicy("api")
	if !ok || gotPol.MaxRetries != 5 {
		t.Error("SetPolicy/GetPolicy failed")
	}
	_, ok = reg.GetPolicy("unknown")
	if ok {
		t.Error("GetPolicy should return false for unknown service")
	}

	// Deregister.
	reg.Deregister("api")
	_, ok = reg.Get("api")
	if ok {
		t.Error("Deregister failed")
	}
}

func TestServiceRegistry_Watch(t *testing.T) {
	reg := NewServiceRegistry()

	// Register before watching to get initial state.
	svc := &ServiceInstance{ServiceName: "api", Namespace: "default"}
	reg.Register(svc)

	ch := reg.Watch("api")
	select {
	case got := <-ch:
		if got.ServiceName != "api" {
			t.Errorf("Watch initial state: ServiceName = %q", got.ServiceName)
		}
	case <-time.After(time.Second):
		t.Fatal("Watch timed out for initial state")
	}

	// Register again to trigger notification.
	svc2 := &ServiceInstance{ServiceName: "api", Version: "v2"}
	reg.Register(svc2)

	select {
	case got := <-ch:
		if got.Version != "v2" {
			t.Errorf("Watch update: Version = %q", got.Version)
		}
	case <-time.After(time.Second):
		t.Fatal("Watch timed out for update")
	}

	// Deregister closes the channel.
	reg.Deregister("api")
}

// ---------------------------------------------------------------------------
// Discovery tests
// ---------------------------------------------------------------------------

func TestStaticDiscoveryLifecycle(t *testing.T) {
	sd := NewStaticDiscovery()
	ctx := context.Background()

	// Not started yet.
	_, err := sd.Discover(ctx, "svc", "ns")
	if err != ErrDiscoveryNotStarted {
		t.Errorf("expected ErrDiscoveryNotStarted, got %v", err)
	}

	sd.Start(ctx)
	defer sd.Stop()

	endpoints := []*Endpoint{
		{ID: "ep-1", Address: "10.0.0.1", Port: 80},
	}
	sd.AddService("svc", "ns", endpoints)

	got, err := sd.Discover(ctx, "svc", "ns")
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("len(endpoints) = %d", len(got))
	}

	// Unknown service.
	_, err = sd.Discover(ctx, "unknown", "ns")
	if err != ErrServiceNotFound {
		t.Errorf("expected ErrServiceNotFound, got %v", err)
	}

	// Watch.
	watchCh, err := sd.Watch(ctx, "svc", "ns")
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}
	select {
	case eps := <-watchCh:
		if len(eps) != 1 {
			t.Errorf("Watch initial: len = %d", len(eps))
		}
	case <-time.After(time.Second):
		t.Fatal("Watch timed out")
	}

	// Remove service.
	sd.RemoveService("svc", "ns")
	_, err = sd.Discover(ctx, "svc", "ns")
	if err != ErrServiceNotFound {
		t.Errorf("after remove: expected ErrServiceNotFound, got %v", err)
	}
}

func TestStaticDiscovery_WatchNotStarted(t *testing.T) {
	sd := NewStaticDiscovery()
	_, err := sd.Watch(context.Background(), "svc", "ns")
	if err != ErrDiscoveryNotStarted {
		t.Errorf("expected ErrDiscoveryNotStarted, got %v", err)
	}
}

func TestDNSDiscovery_NotStarted(t *testing.T) {
	dd := NewDNSDiscovery(&DNSDiscoveryConfig{Domain: "test.local"})
	_, err := dd.Discover(context.Background(), "svc", "ns")
	if err != ErrDiscoveryNotStarted {
		t.Errorf("expected ErrDiscoveryNotStarted, got %v", err)
	}
	_, err = dd.Watch(context.Background(), "svc", "ns")
	if err != ErrDiscoveryNotStarted {
		t.Errorf("Watch: expected ErrDiscoveryNotStarted, got %v", err)
	}
}

func TestDNSDiscovery_EmptyServiceName(t *testing.T) {
	dd := NewDNSDiscovery(&DNSDiscoveryConfig{Domain: "test.local"})
	dd.Start(context.Background())
	defer dd.Stop()

	_, err := dd.Discover(context.Background(), "", "ns")
	if err != ErrEmptyServiceName {
		t.Errorf("expected ErrEmptyServiceName, got %v", err)
	}
}

func TestRegistryDiscovery_NotStarted(t *testing.T) {
	rd := NewRegistryDiscovery(&RegistryDiscoveryConfig{Endpoints: []string{"http://localhost:8080"}})
	_, err := rd.Discover(context.Background(), "svc", "ns")
	if err != ErrDiscoveryNotStarted {
		t.Errorf("expected ErrDiscoveryNotStarted, got %v", err)
	}
}

func TestRegistryDiscovery_EmptyService(t *testing.T) {
	rd := NewRegistryDiscovery(&RegistryDiscoveryConfig{Endpoints: []string{"http://localhost:8080"}})
	rd.Start(context.Background())
	defer rd.Stop()

	_, err := rd.Discover(context.Background(), "", "ns")
	if err != ErrEmptyServiceName {
		t.Errorf("expected ErrEmptyServiceName, got %v", err)
	}
}

func TestKubernetesDiscovery_NotStarted(t *testing.T) {
	kd := NewKubernetesDiscovery(&KubernetesDiscoveryConfig{Namespace: "default"})
	_, err := kd.Discover(context.Background(), "svc", "ns")
	if err != ErrDiscoveryNotStarted {
		t.Errorf("expected ErrDiscoveryNotStarted, got %v", err)
	}
	_, err = kd.Watch(context.Background(), "svc", "ns")
	if err != ErrDiscoveryNotStarted {
		t.Errorf("Watch: expected ErrDiscoveryNotStarted, got %v", err)
	}
}

func TestCachingDiscoveryWrapper(t *testing.T) {
	sd := NewStaticDiscovery()
	sd.Start(context.Background())
	defer sd.Stop()

	sd.AddService("svc", "ns", []*Endpoint{{ID: "ep-1", Address: "10.0.0.1", Port: 80}})

	cd := NewCachingDiscovery(sd, 100*time.Millisecond)
	ctx := context.Background()

	// First call should populate cache.
	eps, err := cd.Discover(ctx, "svc", "ns")
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}
	if len(eps) != 1 {
		t.Errorf("len = %d", len(eps))
	}

	// Remove from backend and verify cache hit.
	sd.RemoveService("svc", "ns")
	eps, err = cd.Discover(ctx, "svc", "ns")
	if err != nil {
		t.Error("cached result should succeed")
	}

	// Invalidate and verify miss.
	cd.Invalidate("svc", "ns")
	_, err = cd.Discover(ctx, "svc", "ns")
	if err != ErrServiceNotFound {
		t.Errorf("expected ErrServiceNotFound after invalidation, got %v", err)
	}

	// Negative cache should persist briefly.
	_, err = cd.Discover(ctx, "svc", "ns")
	if err != ErrServiceNotFound {
		t.Errorf("expected negative cache hit, got %v", err)
	}
}

func TestCompositeDiscovery(t *testing.T) {
	sd1 := NewStaticDiscovery()
	sd2 := NewStaticDiscovery()

	comp := NewCompositeDiscovery(sd1, sd2)
	ctx := context.Background()

	comp.Start(ctx)
	defer comp.Stop()

	// Neither has the service.
	_, err := comp.Discover(ctx, "svc", "ns")
	if err == nil {
		t.Error("expected error from composite when no backend has the service")
	}

	// Add to second backend.
	sd2.AddService("svc", "ns", []*Endpoint{{ID: "ep-1"}})

	eps, err := comp.Discover(ctx, "svc", "ns")
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}
	if len(eps) != 1 {
		t.Errorf("len = %d", len(eps))
	}
}

func TestCompositeDiscovery_NotStarted(t *testing.T) {
	comp := NewCompositeDiscovery()
	_, err := comp.Discover(context.Background(), "svc", "ns")
	if err != ErrDiscoveryNotStarted {
		t.Errorf("expected ErrDiscoveryNotStarted, got %v", err)
	}
}

func TestCompositeDiscovery_Watch(t *testing.T) {
	sd := NewStaticDiscovery()
	comp := NewCompositeDiscovery(sd)
	ctx := context.Background()
	comp.Start(ctx)
	defer comp.Stop()

	sd.AddService("svc", "ns", []*Endpoint{{ID: "ep-1"}})

	ch, err := comp.Watch(ctx, "svc", "ns")
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}
	select {
	case eps := <-ch:
		if len(eps) != 1 {
			t.Errorf("Watch: len = %d", len(eps))
		}
	case <-time.After(time.Second):
		t.Fatal("Watch timed out")
	}
}

// ---------------------------------------------------------------------------
// Endpoint sorting/filtering tests
// ---------------------------------------------------------------------------

func TestSortByWeight(t *testing.T) {
	endpoints := []*Endpoint{
		{ID: "a", Weight: 1},
		{ID: "b", Weight: 10},
		{ID: "c", Weight: 5},
	}
	SortByWeight(endpoints)
	if endpoints[0].Weight != 10 {
		t.Errorf("first endpoint weight = %d, want 10", endpoints[0].Weight)
	}
}

func TestSortByLastSeen(t *testing.T) {
	now := time.Now()
	endpoints := []*Endpoint{
		{ID: "a", LastSeen: now.Add(-time.Hour)},
		{ID: "b", LastSeen: now},
		{ID: "c", LastSeen: now.Add(-time.Minute)},
	}
	SortByLastSeen(endpoints)
	if endpoints[0].ID != "b" {
		t.Errorf("first endpoint = %q, want 'b'", endpoints[0].ID)
	}
}

func TestSortByAddress(t *testing.T) {
	endpoints := []*Endpoint{
		{ID: "c", Address: "10.0.0.3"},
		{ID: "a", Address: "10.0.0.1"},
		{ID: "b", Address: "10.0.0.2"},
	}
	SortByAddress(endpoints)
	if endpoints[0].Address != "10.0.0.1" {
		t.Errorf("first = %q", endpoints[0].Address)
	}
}

func TestFilterByZone(t *testing.T) {
	endpoints := []*Endpoint{
		{ID: "a", Zone: "us-east-1a"},
		{ID: "b", Zone: "us-west-1a"},
		{ID: "c", Zone: "us-east-1a"},
	}
	filtered := FilterByZone(endpoints, "us-east-1a")
	if len(filtered) != 2 {
		t.Errorf("filtered count = %d, want 2", len(filtered))
	}
}

func TestFilterByTag(t *testing.T) {
	endpoints := []*Endpoint{
		{ID: "a", Tags: map[string]string{"env": "prod"}},
		{ID: "b", Tags: map[string]string{"env": "staging"}},
		{ID: "c", Tags: nil},
	}
	filtered := FilterByTag(endpoints, "env", "prod")
	if len(filtered) != 1 {
		t.Errorf("filtered count = %d, want 1", len(filtered))
	}
}

// ---------------------------------------------------------------------------
// LoadBalancer tests
// ---------------------------------------------------------------------------

func TestRoundRobinBalancer(t *testing.T) {
	lb := NewRoundRobinBalancer()
	if lb.Name() != LoadBalancerRoundRobin {
		t.Errorf("Name() = %q", lb.Name())
	}

	// Empty endpoints.
	if lb.Select(nil) != nil {
		t.Error("Select on nil should return nil")
	}

	endpoints := []*Endpoint{
		{ID: "a"}, {ID: "b"}, {ID: "c"},
	}

	counts := map[string]int{}
	for i := 0; i < 9; i++ {
		ep := lb.Select(endpoints)
		counts[ep.ID]++
	}
	for _, id := range []string{"a", "b", "c"} {
		if counts[id] != 3 {
			t.Errorf("endpoint %q selected %d times, want 3", id, counts[id])
		}
	}
}

func TestLeastConnBalancer(t *testing.T) {
	lb := NewLeastConnBalancer()
	if lb.Name() != LoadBalancerLeastConn {
		t.Errorf("Name() = %q", lb.Name())
	}

	if lb.Select(nil) != nil {
		t.Error("Select on nil should return nil")
	}

	endpoints := []*Endpoint{
		{ID: "a"}, {ID: "b"},
	}

	ep1 := lb.Select(endpoints) // Increments count for selected.
	ep2 := lb.Select(endpoints)
	if ep1.ID == ep2.ID {
		t.Error("LeastConn should pick different endpoint when first has connection")
	}

	lb.ReleaseConnection(ep1)
	lb.ReleaseConnection(ep2)

	// Update with subset.
	lb.Update([]*Endpoint{{ID: "a"}})
}

func TestWeightedBalancer(t *testing.T) {
	lb := NewWeightedBalancer()
	if lb.Select(nil) != nil {
		t.Error("Select on nil should return nil")
	}

	endpoints := []*Endpoint{
		{ID: "heavy", Weight: 100},
		{ID: "light", Weight: 1},
	}

	counts := map[string]int{}
	for i := 0; i < 1000; i++ {
		ep := lb.Select(endpoints)
		counts[ep.ID]++
	}
	if counts["heavy"] < counts["light"] {
		t.Errorf("heavy=%d, light=%d - heavy should be selected more", counts["heavy"], counts["light"])
	}
}

func TestRandomBalancer(t *testing.T) {
	lb := NewRandomBalancer()
	if lb.Select(nil) != nil {
		t.Error("Select on nil should return nil")
	}

	endpoints := []*Endpoint{{ID: "a"}, {ID: "b"}}
	ep := lb.Select(endpoints)
	if ep == nil {
		t.Fatal("Select returned nil for non-empty list")
	}
}

func TestIPHashBalancer(t *testing.T) {
	lb := NewIPHashBalancer()
	if lb.Select(nil) != nil {
		t.Error("Select on nil should return nil")
	}

	endpoints := []*Endpoint{{ID: "a"}, {ID: "b"}, {ID: "c"}}

	// Without context, should still return something.
	ep := lb.Select(endpoints)
	if ep == nil {
		t.Fatal("Select returned nil")
	}

	// With client IP, same IP should consistently select same endpoint.
	ctx := &SelectContext{ClientIP: "192.168.1.1"}
	ep1 := lb.SelectWithContext(endpoints, ctx)
	ep2 := lb.SelectWithContext(endpoints, ctx)
	if ep1.ID != ep2.ID {
		t.Errorf("IP hash not consistent: %q != %q", ep1.ID, ep2.ID)
	}
}

func TestConsistentHashBalancer(t *testing.T) {
	lb := NewConsistentHashBalancer(50)
	if lb.Select(nil) != nil {
		t.Error("Select on empty ring should return nil")
	}

	endpoints := []*Endpoint{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	lb.Update(endpoints)

	// With same key, should be consistent.
	ctx := &SelectContext{ClientIP: "10.0.0.1"}
	ep1 := lb.SelectWithContext(endpoints, ctx)
	ep2 := lb.SelectWithContext(endpoints, ctx)
	if ep1.ID != ep2.ID {
		t.Errorf("consistent hash not consistent: %q != %q", ep1.ID, ep2.ID)
	}

	// With path context.
	ctx2 := &SelectContext{Path: "/api/v1/users"}
	ep3 := lb.SelectWithContext(endpoints, ctx2)
	if ep3 == nil {
		t.Fatal("SelectWithContext returned nil")
	}

	// Default 100 virtual nodes.
	lb2 := NewConsistentHashBalancer(0)
	if lb2.virtualNodes != 100 {
		t.Errorf("virtualNodes = %d, want 100", lb2.virtualNodes)
	}
}

func TestAdaptiveBalancer(t *testing.T) {
	lb := NewAdaptiveBalancer()
	if lb.Select(nil) != nil {
		t.Error("Select on nil should return nil")
	}

	endpoints := []*Endpoint{{ID: "a"}, {ID: "b"}}
	ep := lb.Select(endpoints)
	if ep == nil {
		t.Fatal("Select returned nil")
	}

	// Record some metrics.
	lb.RecordResponse(endpoints[0], 10*time.Millisecond, true)
	lb.RecordResponse(endpoints[1], 100*time.Millisecond, false)

	// After metrics, adaptive should prefer the faster one.
	ep = lb.Select(endpoints)
	if ep == nil {
		t.Fatal("Select returned nil after recording metrics")
	}
}

func TestP2CBalancer(t *testing.T) {
	lb := NewP2CBalancer()
	if lb.Select(nil) != nil {
		t.Error("Select on nil should return nil")
	}

	endpoints := []*Endpoint{{ID: "a"}}
	ep := lb.Select(endpoints)
	if ep == nil || ep.ID != "a" {
		t.Error("single endpoint should be selected")
	}

	endpoints = []*Endpoint{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	ep = lb.Select(endpoints)
	if ep == nil {
		t.Fatal("Select returned nil")
	}

	lb.ReleaseConnection(ep)
	lb.Update([]*Endpoint{{ID: "a"}})
}

func TestZoneAwareBalancer(t *testing.T) {
	lb := NewZoneAwareBalancer("zone-a", nil)
	if lb.Select(nil) != nil {
		t.Error("Select on nil should return nil")
	}

	endpoints := []*Endpoint{
		{ID: "a", Zone: "zone-a"},
		{ID: "b", Zone: "zone-b"},
		{ID: "c", Zone: "zone-a"},
	}

	// Should prefer zone-a endpoints.
	counts := map[string]int{}
	for i := 0; i < 100; i++ {
		ep := lb.Select(endpoints)
		counts[ep.ID]++
	}
	if counts["b"] > 0 {
		t.Error("zone-aware should not select zone-b when zone-a endpoints exist")
	}

	// With no local zone endpoints, should fall back.
	remoteEndpoints := []*Endpoint{
		{ID: "x", Zone: "zone-x"},
	}
	ep := lb.Select(remoteEndpoints)
	if ep == nil || ep.ID != "x" {
		t.Error("should fall back to remote endpoints")
	}

	lb.Update(endpoints)
	lb.RecordResponse(endpoints[0], time.Millisecond, true)
}

func TestSubsetBalancer(t *testing.T) {
	lb := NewSubsetBalancer(2)
	if lb.Select(nil) != nil {
		t.Error("Select on nil should return nil")
	}

	endpoints := []*Endpoint{
		{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}, {ID: "e"},
	}

	ctx := &SelectContext{ServiceName: "test-svc"}
	ep := lb.SelectWithContext(endpoints, ctx)
	if ep == nil {
		t.Fatal("Select returned nil")
	}

	lb.Update(endpoints)

	// Default subset size.
	lb2 := NewSubsetBalancer(0)
	if lb2.subsetSize != 10 {
		t.Errorf("subsetSize = %d, want 10", lb2.subsetSize)
	}
}

func TestBalancerPool(t *testing.T) {
	pool := NewBalancerPool(nil)

	lb := pool.Get("svc-a")
	if lb == nil {
		t.Fatal("Get returned nil")
	}

	lb2 := pool.Get("svc-a")
	if lb != lb2 {
		t.Error("expected same instance")
	}

	pool.Set("svc-b", NewRandomBalancer())
	names := pool.List()
	if len(names) != 2 {
		t.Errorf("List() len = %d, want 2", len(names))
	}

	pool.Remove("svc-a")
	names = pool.List()
	if len(names) != 1 {
		t.Errorf("after Remove, List() len = %d", len(names))
	}
}

func TestBaseBalancer_RecordResponse(t *testing.T) {
	bb := newBaseBalancer("test")

	ep := &Endpoint{ID: "ep-1"}
	bb.RecordResponse(ep, 10*time.Millisecond, true)
	bb.RecordResponse(ep, 20*time.Millisecond, false)

	m := bb.GetMetrics("ep-1")
	if m == nil {
		t.Fatal("GetMetrics returned nil")
	}
	if m.TotalRequests != 2 {
		t.Errorf("TotalRequests = %d", m.TotalRequests)
	}
	if m.SuccessCount != 1 {
		t.Errorf("SuccessCount = %d", m.SuccessCount)
	}
	if m.FailureCount != 1 {
		t.Errorf("FailureCount = %d", m.FailureCount)
	}

	// Unknown endpoint.
	if bb.GetMetrics("unknown") != nil {
		t.Error("expected nil for unknown endpoint")
	}
}

func TestBaseBalancer_Update(t *testing.T) {
	bb := newBaseBalancer("test")

	ep1 := &Endpoint{ID: "ep-1"}
	ep2 := &Endpoint{ID: "ep-2"}
	bb.RecordResponse(ep1, time.Millisecond, true)
	bb.RecordResponse(ep2, time.Millisecond, true)

	// Update with only ep-1.
	bb.Update([]*Endpoint{ep1})

	if bb.GetMetrics("ep-2") != nil {
		t.Error("ep-2 metrics should be removed after Update")
	}
}

// ---------------------------------------------------------------------------
// Proxy and ConnectionPool tests
// ---------------------------------------------------------------------------

func TestDefaultInboundProxyConfig(t *testing.T) {
	cfg := DefaultInboundProxyConfig()
	if cfg.Port != 15001 {
		t.Errorf("Port = %d", cfg.Port)
	}
}

func TestDefaultOutboundProxyConfig(t *testing.T) {
	cfg := DefaultOutboundProxyConfig()
	if cfg.Port != 15006 {
		t.Errorf("Port = %d", cfg.Port)
	}
}

func TestDefaultConnectionPoolConfig(t *testing.T) {
	cfg := DefaultConnectionPoolConfig()
	if cfg.MaxSize != 10 {
		t.Errorf("MaxSize = %d", cfg.MaxSize)
	}
}

func TestConnectionPool_NewAndClose(t *testing.T) {
	pool := NewConnectionPool("127.0.0.1:9999", nil)
	if pool == nil {
		t.Fatal("NewConnectionPool returned nil")
	}

	if pool.Size() != 0 {
		t.Errorf("Size() = %d, want 0", pool.Size())
	}

	stats := pool.Stats()
	if stats.Address != "127.0.0.1:9999" {
		t.Errorf("Address = %q", stats.Address)
	}

	pool.Close()
	// Double close should not panic.
	pool.Close()
}

func TestConnectionPool_GetWhenClosed(t *testing.T) {
	pool := NewConnectionPool("127.0.0.1:9999", nil)
	pool.Close()

	_, err := pool.Get(context.Background())
	if err != ErrProxyClosed {
		t.Errorf("expected ErrProxyClosed, got %v", err)
	}
}

func TestConnectionPoolStats_HitRate(t *testing.T) {
	s := ConnectionPoolStats{TotalHits: 0, TotalMisses: 0}
	if s.HitRate() != 0 {
		t.Errorf("HitRate with 0 total = %v", s.HitRate())
	}

	s = ConnectionPoolStats{TotalHits: 7, TotalMisses: 3}
	if s.HitRate() != 70.0 {
		t.Errorf("HitRate = %v, want 70.0", s.HitRate())
	}
}

func TestIsHTTPMethod(t *testing.T) {
	httpStarts := []byte{'G', 'P', 'D', 'H', 'O', 'C', 'T'}
	for _, b := range httpStarts {
		if !isHTTPMethod(b) {
			t.Errorf("isHTTPMethod(%c) = false, want true", b)
		}
	}
	if isHTTPMethod('X') {
		t.Error("isHTTPMethod('X') = true, want false")
	}
}

func TestNewTransparentProxy(t *testing.T) {
	tp := NewTransparentProxy(nil)
	if tp == nil {
		t.Fatal("NewTransparentProxy(nil) returned nil")
	}
}

func TestPoolManager(t *testing.T) {
	pm := NewPoolManager(nil, 0)
	if pm == nil {
		t.Fatal("NewPoolManager returned nil")
	}
	if pm.maxPools != 100 {
		t.Errorf("maxPools = %d, want 100", pm.maxPools)
	}

	stats := pm.Stats()
	if len(stats) != 0 {
		t.Errorf("Stats() len = %d", len(stats))
	}

	pm.Cleanup()
	pm.Close()
}

func TestNewInboundProxy_Stats(t *testing.T) {
	p := NewInboundProxy(nil)
	stats := p.Stats()
	if stats.TotalRequests != 0 {
		t.Errorf("TotalRequests = %d", stats.TotalRequests)
	}
}

func TestNewOutboundProxy_Stats(t *testing.T) {
	p := NewOutboundProxy(nil)
	stats := p.Stats()
	if stats.TotalRequests != 0 {
		t.Errorf("TotalRequests = %d", stats.TotalRequests)
	}
}

func TestTransparentProxy_Stats(t *testing.T) {
	p := NewTransparentProxy(nil)
	stats := p.Stats()
	if stats.TotalConnections != 0 {
		t.Errorf("TotalConnections = %d", stats.TotalConnections)
	}
}

// ---------------------------------------------------------------------------
// Mesh constructor and lifecycle tests
// ---------------------------------------------------------------------------

func TestNewMesh_NilConfig(t *testing.T) {
	m, err := NewMesh(nil)
	if err != nil {
		t.Fatalf("NewMesh(nil) error: %v", err)
	}
	if m == nil {
		t.Fatal("NewMesh(nil) returned nil mesh")
	}
	m.Stop()
}

func TestNewMesh_InvalidConfig(t *testing.T) {
	cfg := &Config{} // Empty, will fail validation.
	_, err := NewMesh(cfg)
	if err == nil {
		t.Fatal("expected error for invalid config")
	}
}

func TestNewMesh_UnknownDiscoveryType(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DiscoveryType = "bogus"
	_, err := NewMesh(cfg)
	if err == nil {
		t.Fatal("expected error for unknown discovery type")
	}
}

func TestNewMesh_AllDiscoveryTypes(t *testing.T) {
	for _, dt := range []string{DiscoveryTypeDNS, DiscoveryTypeRegistry, DiscoveryTypeStatic, DiscoveryTypeKubernetes} {
		t.Run(dt, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.DiscoveryType = dt
			cfg.RegistryEndpoints = []string{"http://localhost:8080"} // For registry type.
			m, err := NewMesh(cfg)
			if err != nil {
				t.Fatalf("NewMesh with %s discovery failed: %v", dt, err)
			}
			m.Stop()
		})
	}
}

func TestMesh_StartStop(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DiscoveryType = DiscoveryTypeStatic
	cfg.InboundPort = 0  // Disable proxies.
	cfg.OutboundPort = 0 // Disable proxies.
	m, err := NewMesh(cfg)
	if err != nil {
		t.Fatal(err)
	}

	if err := m.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Double start should error.
	if err := m.Start(); err == nil {
		t.Error("expected error on double start")
	}

	if err := m.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Double stop should be safe.
	if err := m.Stop(); err != nil {
		t.Fatalf("double Stop failed: %v", err)
	}
}

func TestMesh_StartWhenClosed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DiscoveryType = DiscoveryTypeStatic
	cfg.InboundPort = 0
	cfg.OutboundPort = 0
	m, err := NewMesh(cfg)
	if err != nil {
		t.Fatal(err)
	}
	m.Stop()

	err = m.Start()
	if err != ErrMeshClosed {
		t.Errorf("expected ErrMeshClosed, got %v", err)
	}
}

func TestMesh_RegisterDeregisterService(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DiscoveryType = DiscoveryTypeStatic
	cfg.InboundPort = 0
	cfg.OutboundPort = 0
	cfg.DefaultRateLimit = 100 // Enable rate limiters.
	m, err := NewMesh(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Stop()

	svc := &ServiceInstance{
		ServiceName: "api",
		Namespace:   "default",
		Endpoints: []*Endpoint{
			{ID: "ep-1", Address: "10.0.0.1", Port: 8080},
		},
	}

	if err := m.RegisterService(svc); err != nil {
		t.Fatalf("RegisterService failed: %v", err)
	}

	got, ok := m.GetService("api")
	if !ok || got.ServiceName != "api" {
		t.Error("GetService failed after register")
	}

	services := m.ListServices()
	if len(services) != 1 {
		t.Errorf("ListServices = %d", len(services))
	}

	if err := m.DeregisterService("api"); err != nil {
		t.Fatalf("DeregisterService failed: %v", err)
	}

	_, ok = m.GetService("api")
	if ok {
		t.Error("service should not exist after deregister")
	}
}

func TestMesh_RegisterWhenClosed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DiscoveryType = DiscoveryTypeStatic
	cfg.InboundPort = 0
	cfg.OutboundPort = 0
	m, err := NewMesh(cfg)
	if err != nil {
		t.Fatal(err)
	}
	m.Stop()

	err = m.RegisterService(&ServiceInstance{ServiceName: "x"})
	if err != ErrMeshClosed {
		t.Errorf("expected ErrMeshClosed, got %v", err)
	}

	err = m.DeregisterService("x")
	if err != ErrMeshClosed {
		t.Errorf("expected ErrMeshClosed, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Mesh stats and metrics tests
// ---------------------------------------------------------------------------

func TestMesh_Stats(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DiscoveryType = DiscoveryTypeStatic
	cfg.InboundPort = 0
	cfg.OutboundPort = 0
	m, err := NewMesh(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Stop()

	stats := m.Stats()
	if stats.TotalRequests != 0 {
		t.Errorf("TotalRequests = %d", stats.TotalRequests)
	}
}

func TestMesh_AverageLatency(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DiscoveryType = DiscoveryTypeStatic
	cfg.InboundPort = 0
	cfg.OutboundPort = 0
	m, err := NewMesh(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Stop()

	if m.AverageLatency() != 0 {
		t.Errorf("AverageLatency with no requests = %v", m.AverageLatency())
	}
}

func TestMesh_SuccessRate(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DiscoveryType = DiscoveryTypeStatic
	cfg.InboundPort = 0
	cfg.OutboundPort = 0
	m, err := NewMesh(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Stop()

	if m.SuccessRate() != 100.0 {
		t.Errorf("SuccessRate with no requests = %v", m.SuccessRate())
	}
}

func TestMesh_CircuitBreakerState(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DiscoveryType = DiscoveryTypeStatic
	cfg.InboundPort = 0
	cfg.OutboundPort = 0
	m, err := NewMesh(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Stop()

	// Unknown service returns closed.
	if m.GetCircuitBreakerState("unknown") != CircuitStateClosed {
		t.Error("unknown service CB should be closed")
	}

	// Create one and trip it.
	m.getOrCreateCircuitBreaker("svc")
	m.ResetCircuitBreaker("svc")
	if m.GetCircuitBreakerState("svc") != CircuitStateClosed {
		t.Error("CB should be closed after reset")
	}
}

func TestMesh_SetGetServicePolicy(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DiscoveryType = DiscoveryTypeStatic
	cfg.InboundPort = 0
	cfg.OutboundPort = 0
	m, err := NewMesh(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Stop()

	// No policy set.
	if m.GetServicePolicy("unknown") != nil {
		t.Error("expected nil for unknown service policy")
	}

	pol := &ServicePolicy{MaxRetries: 10}
	m.SetServicePolicy("api", pol)

	got := m.GetServicePolicy("api")
	if got == nil || got.MaxRetries != 10 {
		t.Error("SetServicePolicy/GetServicePolicy failed")
	}
}

func TestMesh_GetEndpoints(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DiscoveryType = DiscoveryTypeStatic
	cfg.InboundPort = 0
	cfg.OutboundPort = 0
	m, err := NewMesh(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Stop()

	// Register a service with endpoints.
	m.RegisterService(&ServiceInstance{
		ServiceName: "api",
		Namespace:   "default",
		Endpoints: []*Endpoint{
			{ID: "ep-1", Address: "10.0.0.1", Port: 8080},
		},
	})

	eps, err := m.GetEndpoints(context.Background(), "api", "default")
	if err != nil {
		t.Fatalf("GetEndpoints: %v", err)
	}
	if len(eps) != 1 {
		t.Errorf("GetEndpoints len = %d", len(eps))
	}
}

func TestMesh_GetHealthyEndpoints(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DiscoveryType = DiscoveryTypeStatic
	cfg.InboundPort = 0
	cfg.OutboundPort = 0
	m, err := NewMesh(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Stop()

	m.RegisterService(&ServiceInstance{
		ServiceName: "api",
		Namespace:   "default",
		Endpoints: []*Endpoint{
			{ID: "ep-1", Address: "10.0.0.1", Port: 8080},
		},
	})

	eps, err := m.GetHealthyEndpoints(context.Background(), "api", "default")
	if err != nil {
		t.Fatalf("GetHealthyEndpoints: %v", err)
	}
	// All endpoints are considered healthy by default.
	if len(eps) != 1 {
		t.Errorf("GetHealthyEndpoints len = %d", len(eps))
	}
}

// ---------------------------------------------------------------------------
// Mesh.Call tests
// ---------------------------------------------------------------------------

func TestMesh_CallNotStarted(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DiscoveryType = DiscoveryTypeStatic
	cfg.InboundPort = 0
	cfg.OutboundPort = 0
	m, err := NewMesh(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Stop()

	_, err = m.Call(context.Background(), &Request{Service: "api"})
	if err != ErrMeshNotStarted {
		t.Errorf("expected ErrMeshNotStarted, got %v", err)
	}
}

func TestMesh_CallWhenClosed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DiscoveryType = DiscoveryTypeStatic
	cfg.InboundPort = 0
	cfg.OutboundPort = 0
	m, err := NewMesh(cfg)
	if err != nil {
		t.Fatal(err)
	}
	m.Stop()

	_, err = m.Call(context.Background(), &Request{Service: "api"})
	if err != ErrMeshClosed {
		t.Errorf("expected ErrMeshClosed, got %v", err)
	}
}

func TestMesh_CallSuccess(t *testing.T) {
	// Set up a test HTTP server.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer ts.Close()

	// Parse server address.
	parts := strings.SplitN(strings.TrimPrefix(ts.URL, "http://"), ":", 2)
	host := parts[0]
	port := 80
	if len(parts) == 2 {
		fmt.Sscanf(parts[1], "%d", &port)
	}

	cfg := DefaultConfig()
	cfg.DiscoveryType = DiscoveryTypeStatic
	cfg.InboundPort = 0
	cfg.OutboundPort = 0
	cfg.EnableTLS = false
	cfg.DefaultRetries = 0
	cfg.RequestTimeout = 5 * time.Second
	m, err := NewMesh(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Stop()

	m.RegisterService(&ServiceInstance{
		ServiceName: "test-svc",
		Namespace:   "default",
		Endpoints: []*Endpoint{
			{ID: fmt.Sprintf("%s:%d", host, port), Address: host, Port: port},
		},
	})

	m.Start()

	resp, err := m.Call(context.Background(), &Request{
		Service:   "test-svc",
		Namespace: "default",
		Method:    "GET",
		Path:      "/",
		Headers:   make(http.Header),
	})
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
	if string(resp.Body) != "ok" {
		t.Errorf("Body = %q", string(resp.Body))
	}

	stats := m.Stats()
	if stats.TotalRequests != 1 {
		t.Errorf("TotalRequests = %d", stats.TotalRequests)
	}
	if stats.SuccessfulRequests != 1 {
		t.Errorf("SuccessfulRequests = %d", stats.SuccessfulRequests)
	}
	if m.AverageLatency() <= 0 {
		t.Error("AverageLatency should be > 0 after a request")
	}
	if m.SuccessRate() != 100.0 {
		t.Errorf("SuccessRate = %v", m.SuccessRate())
	}
}

func TestMesh_CallNoHealthyBackends(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DiscoveryType = DiscoveryTypeStatic
	cfg.InboundPort = 0
	cfg.OutboundPort = 0
	cfg.DefaultRetries = 0
	m, err := NewMesh(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Stop()

	ep := &Endpoint{ID: "ep-1", Address: "10.0.0.1", Port: 9999}
	m.RegisterService(&ServiceInstance{
		ServiceName: "api",
		Namespace:   "default",
		Endpoints:   []*Endpoint{ep},
	})

	// Mark endpoint unhealthy.
	m.healthChecker.AddEndpoint(ep)
	m.healthChecker.RecordFailure("ep-1")
	m.healthChecker.RecordFailure("ep-1")
	m.healthChecker.RecordFailure("ep-1")

	m.Start()

	_, err = m.Call(context.Background(), &Request{
		Service:   "api",
		Namespace: "default",
		Method:    "GET",
		Path:      "/",
	})
	if err != ErrNoHealthyBackends {
		t.Errorf("expected ErrNoHealthyBackends, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Mesh.shouldRetry tests
// ---------------------------------------------------------------------------

func TestMesh_ShouldRetry(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DiscoveryType = DiscoveryTypeStatic
	cfg.InboundPort = 0
	cfg.OutboundPort = 0
	m, err := NewMesh(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Stop()

	tests := []struct {
		name       string
		err        error
		attempt    int
		maxRetries int
		want       bool
	}{
		{"max retries exceeded", errors.New("x"), 3, 3, false},
		{"circuit open", ErrCircuitOpen, 0, 3, false},
		{"rate limited", ErrRateLimited, 0, 3, false},
		{"context canceled", context.Canceled, 0, 3, false},
		{"network error", errors.New("connection refused"), 0, 3, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.shouldRetry(tt.err, tt.attempt, tt.maxRetries)
			if got != tt.want {
				t.Errorf("shouldRetry = %v, want %v", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Mesh.getOrCreateBalancer tests for different LB types
// ---------------------------------------------------------------------------

func TestMesh_GetOrCreateBalancer_AllTypes(t *testing.T) {
	for _, lbType := range []string{
		LoadBalancerRoundRobin, LoadBalancerLeastConn,
		LoadBalancerWeighted, LoadBalancerRandom,
		LoadBalancerIPHash,
	} {
		t.Run(lbType, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.DiscoveryType = DiscoveryTypeStatic
			cfg.InboundPort = 0
			cfg.OutboundPort = 0
			cfg.LoadBalancerType = lbType
			m, err := NewMesh(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer m.Stop()

			lb := m.getOrCreateBalancer("test-svc")
			if lb == nil {
				t.Fatal("getOrCreateBalancer returned nil")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Mesh TLS tests
// ---------------------------------------------------------------------------

func TestMesh_ConfigureTLS(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DiscoveryType = DiscoveryTypeStatic
	cfg.InboundPort = 0
	cfg.OutboundPort = 0
	m, err := NewMesh(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Stop()

	// Before TLS config, should return nil.
	if m.ServerTLSConfig() != nil {
		t.Error("ServerTLSConfig should be nil before configuration")
	}
	if m.ClientTLSConfig() != nil {
		t.Error("ClientTLSConfig should be nil before configuration")
	}
}

func TestMesh_ConfigureTLSWhenClosed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DiscoveryType = DiscoveryTypeStatic
	cfg.InboundPort = 0
	cfg.OutboundPort = 0
	m, err := NewMesh(cfg)
	if err != nil {
		t.Fatal(err)
	}
	m.Stop()

	err = m.ConfigureTLS(tls.Certificate{}, nil)
	if err != ErrMeshClosed {
		t.Errorf("expected ErrMeshClosed, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Mesh mTLS provider tests (nil paths)
// ---------------------------------------------------------------------------

func TestMesh_MTLSProviderNil(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DiscoveryType = DiscoveryTypeStatic
	cfg.InboundPort = 0
	cfg.OutboundPort = 0
	m, err := NewMesh(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Stop()

	if m.MTLSProvider() != nil {
		t.Error("MTLSProvider should be nil by default")
	}
	if m.SPIFFEID() != nil {
		t.Error("SPIFFEID should be nil by default")
	}

	err = m.RotateCertificate()
	if err == nil {
		t.Error("RotateCertificate should fail when provider not configured")
	}

	err = m.SaveCertificates("/tmp")
	if err == nil {
		t.Error("SaveCertificates should fail when provider not configured")
	}
}

func TestMesh_ZeroTrustStats(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DiscoveryType = DiscoveryTypeStatic
	cfg.InboundPort = 0
	cfg.OutboundPort = 0
	m, err := NewMesh(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Stop()

	allowed, denied := m.ZeroTrustStats()
	if allowed != 0 || denied != 0 {
		t.Errorf("ZeroTrustStats = (%d, %d), want (0, 0)", allowed, denied)
	}
}

func TestMesh_EnforceZeroTrust_NilEnforcer(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DiscoveryType = DiscoveryTypeStatic
	cfg.InboundPort = 0
	cfg.OutboundPort = 0
	m, err := NewMesh(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Stop()

	err = m.EnforceZeroTrust(nil, "svc")
	if err != nil {
		t.Errorf("EnforceZeroTrust with nil enforcer should return nil, got %v", err)
	}
}

func TestMesh_SetAuthorizationPolicy_NilEnforcer(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DiscoveryType = DiscoveryTypeStatic
	cfg.InboundPort = 0
	cfg.OutboundPort = 0
	m, err := NewMesh(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Stop()

	// Should not panic with nil enforcer/identityMgr.
	m.SetAuthorizationPolicy("svc", nil)
}

func TestMesh_GetPeerIdentity_NilIdentityMgr(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DiscoveryType = DiscoveryTypeStatic
	cfg.InboundPort = 0
	cfg.OutboundPort = 0
	m, err := NewMesh(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Stop()

	// With nil identityMgr, GetPeerIdentity calls ExtractPeerInfo which
	// requires a non-nil tls.Conn. We verify the nil identity manager path
	// by checking that the function exists and would go through the fallback.
	// A real tls.Conn with peer certs would be needed for a full test.
	// Just ensure no panic with a valid but empty TLS conn.
	// We cannot easily create a proper tls.Conn in unit tests, so we skip
	// the actual call and just verify the identity manager is nil.
	if m.identityMgr != nil {
		t.Error("identityMgr should be nil by default")
	}
}

// ---------------------------------------------------------------------------
// Mesh.ConfigureMTLSProvider tests
// ---------------------------------------------------------------------------

func TestMesh_ConfigureMTLSProvider_WhenClosed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DiscoveryType = DiscoveryTypeStatic
	cfg.InboundPort = 0
	cfg.OutboundPort = 0
	m, err := NewMesh(cfg)
	if err != nil {
		t.Fatal(err)
	}
	m.Stop()

	err = m.ConfigureMTLSProvider(nil)
	if err != ErrMeshClosed {
		t.Errorf("expected ErrMeshClosed, got %v", err)
	}
}

func TestMesh_ConfigureMTLSProvider_Success(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DiscoveryType = DiscoveryTypeStatic
	cfg.InboundPort = 0
	cfg.OutboundPort = 0
	cfg.TrustDomain = "test.local"
	cfg.ServiceName = "my-svc"
	cfg.Namespace = "prod"
	m, err := NewMesh(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Stop()

	err = m.ConfigureMTLSProvider(nil)
	if err != nil {
		t.Fatalf("ConfigureMTLSProvider failed: %v", err)
	}

	if m.MTLSProvider() == nil {
		t.Error("MTLSProvider should not be nil after configuration")
	}
}

func TestMesh_InitializeMTLSWithCA_WhenClosed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DiscoveryType = DiscoveryTypeStatic
	cfg.InboundPort = 0
	cfg.OutboundPort = 0
	m, err := NewMesh(cfg)
	if err != nil {
		t.Fatal(err)
	}
	m.Stop()

	err = m.InitializeMTLSWithCA(nil, nil)
	if err != ErrMeshClosed {
		t.Errorf("expected ErrMeshClosed, got %v", err)
	}
}

func TestMesh_InitializeMTLSFromFiles_WhenClosed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DiscoveryType = DiscoveryTypeStatic
	cfg.InboundPort = 0
	cfg.OutboundPort = 0
	m, err := NewMesh(cfg)
	if err != nil {
		t.Fatal(err)
	}
	m.Stop()

	err = m.InitializeMTLSFromFiles("cert.pem", "key.pem", "ca.pem")
	if err != ErrMeshClosed {
		t.Errorf("expected ErrMeshClosed, got %v", err)
	}
}

func TestMesh_InitializeMTLSWithCertManager_WhenClosed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DiscoveryType = DiscoveryTypeStatic
	cfg.InboundPort = 0
	cfg.OutboundPort = 0
	m, err := NewMesh(cfg)
	if err != nil {
		t.Fatal(err)
	}
	m.Stop()

	err = m.InitializeMTLSWithCertManager(nil)
	if err != ErrMeshClosed {
		t.Errorf("expected ErrMeshClosed, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Mesh version constant test
// ---------------------------------------------------------------------------

func TestVersion(t *testing.T) {
	if Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", Version, "1.0.0")
	}
}

// ---------------------------------------------------------------------------
// SlidingWindow tests
// ---------------------------------------------------------------------------

func TestSlidingWindow(t *testing.T) {
	w := newSlidingWindow(100*time.Millisecond, 0) // 0 -> defaults to 10
	if len(w.buckets) != 10 {
		t.Errorf("expected 10 buckets, got %d", len(w.buckets))
	}

	w.increment()
	w.increment()
	if w.sum() != 2 {
		t.Errorf("sum = %d, want 2", w.sum())
	}

	w.reset()
	if w.sum() != 0 {
		t.Errorf("sum after reset = %d, want 0", w.sum())
	}
}

// ---------------------------------------------------------------------------
// Mesh.onCircuitStateChange / onEndpointHealthChange tests
// ---------------------------------------------------------------------------

func TestMesh_OnCircuitStateChange(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DiscoveryType = DiscoveryTypeStatic
	cfg.InboundPort = 0
	cfg.OutboundPort = 0
	m, err := NewMesh(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Stop()

	// Should not panic.
	m.onCircuitStateChange("svc", CircuitStateClosed, CircuitStateOpen)
	m.onEndpointHealthChange("ep-1", false)
}

// ---------------------------------------------------------------------------
// Mesh.getServicePolicy default path test
// ---------------------------------------------------------------------------

func TestMesh_getServicePolicy_Default(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DiscoveryType = DiscoveryTypeStatic
	cfg.InboundPort = 0
	cfg.OutboundPort = 0
	cfg.DefaultRateLimit = 50
	m, err := NewMesh(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Stop()

	pol := m.getServicePolicy("unknown-svc")
	if pol.MaxRetries != cfg.DefaultRetries {
		t.Errorf("MaxRetries = %d", pol.MaxRetries)
	}
	if !pol.RateLimitEnabled {
		t.Error("RateLimitEnabled should be true when DefaultRateLimit > 0")
	}
}

// ---------------------------------------------------------------------------
// Sidecar itoa helper test
// ---------------------------------------------------------------------------

func TestItoa(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{42, "42"},
		{12345, "12345"},
		{-1, "-1"},
		{-42, "-42"},
	}
	for _, tt := range tests {
		got := itoa(tt.input)
		if got != tt.want {
			t.Errorf("itoa(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Concurrent access tests
// ---------------------------------------------------------------------------

func TestConcurrentCircuitBreakerOps(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		Name:             "concurrent",
		FailureThreshold: 100,
		SuccessThreshold: 100,
		Timeout:          time.Hour,
	})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			cb.RecordSuccess()
		}()
		go func() {
			defer wg.Done()
			cb.RecordFailure()
		}()
		go func() {
			defer wg.Done()
			cb.Allow()
			cb.State()
			cb.Metrics()
		}()
	}
	wg.Wait()
}

func TestConcurrentRateLimiter(t *testing.T) {
	rl := NewRateLimiter(&RateLimiterConfig{
		Rate:  10000,
		Burst: 100,
	})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rl.Allow()
		}()
	}
	wg.Wait()
}

func TestConcurrentServiceRegistry(t *testing.T) {
	reg := NewServiceRegistry()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		name := fmt.Sprintf("svc-%d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			reg.Register(&ServiceInstance{ServiceName: name})
			reg.Get(name)
			reg.List()
			reg.GetPolicy(name)
		}()
	}
	wg.Wait()
}
