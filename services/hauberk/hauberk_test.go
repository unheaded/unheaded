package hauberk

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"math/big"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/logger"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newTestLogger returns a logger that writes to io.Discard so tests are quiet.
func newTestLogger() *logger.Logger {
	return logger.New(io.Discard)
}

// newTestService creates a Service with sensible test defaults and no busboy
// dependency. The busboy field is left nil; none of the methods under test
// call it directly.
func newTestService(opts ...func(*Config)) *Service {
	cfg := DefaultConfig()
	for _, o := range opts {
		o(cfg)
	}
	return NewService(newTestLogger(), nil, cfg)
}

// registerN registers n endpoints for the given service name and returns
// the created endpoints.
func registerN(t *testing.T, svc *Service, serviceName string, n int) []*Endpoint {
	t.Helper()
	eps := make([]*Endpoint, n)
	for i := 0; i < n; i++ {
		ep := &Endpoint{
			ServiceName: serviceName,
			Address:     fmt.Sprintf("10.0.0.%d", i+1),
			Port:        8080 + i,
		}
		if err := svc.RegisterEndpoint(ep); err != nil {
			t.Fatalf("register endpoint %d: %v", i, err)
		}
		eps[i] = ep
	}
	return eps
}

// ---------------------------------------------------------------------------
// DefaultConfig tests
// ---------------------------------------------------------------------------

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	tests := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"DefaultTimeout", cfg.DefaultTimeout, 10 * time.Second},
		{"DefaultRetries", cfg.DefaultRetries, 3},
		{"CBThreshold", cfg.CBThreshold, 5},
		{"CBTimeout", cfg.CBTimeout, 30 * time.Second},
		{"MTLSEnabled", cfg.MTLSEnabled, true},
		{"CertRotationInterval", cfg.CertRotationInterval, 24 * time.Hour},
		{"BusboyTopic", cfg.BusboyTopic, "hauberk.mesh"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("got %v, want %v", tc.got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// NewService tests
// ---------------------------------------------------------------------------

func TestNewService(t *testing.T) {
	t.Run("nil config uses defaults", func(t *testing.T) {
		svc := NewService(newTestLogger(), nil, nil)
		if svc.config.DefaultTimeout != 10*time.Second {
			t.Errorf("expected default timeout 10s, got %v", svc.config.DefaultTimeout)
		}
		if svc.config.CBThreshold != 5 {
			t.Errorf("expected CB threshold 5, got %d", svc.config.CBThreshold)
		}
	})

	t.Run("custom config is preserved", func(t *testing.T) {
		cfg := &Config{
			DefaultTimeout: 5 * time.Second,
			CBThreshold:    3,
			CBTimeout:      10 * time.Second,
			MTLSEnabled:    false,
			BusboyTopic:    "custom.topic",
		}
		svc := NewService(newTestLogger(), nil, cfg)
		if svc.config.DefaultTimeout != 5*time.Second {
			t.Errorf("expected 5s timeout, got %v", svc.config.DefaultTimeout)
		}
		if svc.config.CBThreshold != 3 {
			t.Errorf("expected CB threshold 3, got %d", svc.config.CBThreshold)
		}
		if svc.config.MTLSEnabled {
			t.Error("expected MTLSEnabled false")
		}
		if svc.config.BusboyTopic != "custom.topic" {
			t.Errorf("expected custom.topic, got %s", svc.config.BusboyTopic)
		}
	})

	t.Run("maps are initialized", func(t *testing.T) {
		svc := newTestService()
		if svc.endpoints == nil {
			t.Error("endpoints map is nil")
		}
		if svc.circuitBreakers == nil {
			t.Error("circuitBreakers map is nil")
		}
		if svc.policies == nil {
			t.Error("policies map is nil")
		}
		if svc.certificates == nil {
			t.Error("certificates map is nil")
		}
	})
}

// ---------------------------------------------------------------------------
// Start / Stop tests
// ---------------------------------------------------------------------------

func TestStartStop(t *testing.T) {
	svc := newTestService()
	ctx, cancel := context.WithCancel(context.Background())

	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// Let the circuit breaker loop tick at least once.
	time.Sleep(20 * time.Millisecond)

	cancel() // stops the circuit breaker loop goroutine

	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop() error: %v", err)
	}
}

func TestStartContextCancellation(t *testing.T) {
	svc := newTestService()
	ctx, cancel := context.WithCancel(context.Background())

	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// Cancel immediately -- the background goroutine should exit cleanly.
	cancel()
	// Give it a moment to exit.
	time.Sleep(20 * time.Millisecond)
}

// ---------------------------------------------------------------------------
// Service Discovery: RegisterEndpoint
// ---------------------------------------------------------------------------

func TestRegisterEndpoint(t *testing.T) {
	tests := []struct {
		name        string
		endpoint    *Endpoint
		wantID      string
		wantHealthy bool
	}{
		{
			name: "auto-generates ID when empty",
			endpoint: &Endpoint{
				ServiceName: "api-gateway",
				Address:     "10.0.0.1",
				Port:        8080,
			},
			wantID:      "api-gateway-10.0.0.1-8080",
			wantHealthy: true,
		},
		{
			name: "preserves explicit ID",
			endpoint: &Endpoint{
				ID:          "custom-id-123",
				ServiceName: "api-gateway",
				Address:     "10.0.0.2",
				Port:        9090,
			},
			wantID:      "custom-id-123",
			wantHealthy: true,
		},
		{
			name: "endpoint with labels",
			endpoint: &Endpoint{
				ServiceName: "worker",
				Address:     "10.0.0.5",
				Port:        3000,
				Labels:      map[string]string{"zone": "us-east-1", "version": "v2"},
			},
			wantID:      "worker-10.0.0.5-3000",
			wantHealthy: true,
		},
		{
			name: "endpoint with TLS enabled",
			endpoint: &Endpoint{
				ServiceName: "secure-svc",
				Address:     "10.0.0.6",
				Port:        443,
				TLSEnabled:  true,
			},
			wantID:      "secure-svc-10.0.0.6-443",
			wantHealthy: true,
		},
		{
			name: "endpoint with weight",
			endpoint: &Endpoint{
				ServiceName: "weighted",
				Address:     "10.0.0.7",
				Port:        8080,
				Weight:      100,
			},
			wantID:      "weighted-10.0.0.7-8080",
			wantHealthy: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService()
			err := svc.RegisterEndpoint(tc.endpoint)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.endpoint.ID != tc.wantID {
				t.Errorf("ID = %q, want %q", tc.endpoint.ID, tc.wantID)
			}
			if tc.endpoint.Healthy != tc.wantHealthy {
				t.Errorf("Healthy = %v, want %v", tc.endpoint.Healthy, tc.wantHealthy)
			}
			if tc.endpoint.LastUpdated.IsZero() {
				t.Error("LastUpdated should be set")
			}
		})
	}
}

func TestRegisterEndpoint_InitializesCircuitBreaker(t *testing.T) {
	svc := newTestService()
	ep := &Endpoint{ServiceName: "test-svc", Address: "10.0.0.1", Port: 8080}

	if err := svc.RegisterEndpoint(ep); err != nil {
		t.Fatal(err)
	}

	cb, ok := svc.GetCircuitBreaker("test-svc")
	if !ok {
		t.Fatal("circuit breaker not initialized")
	}
	if cb.State != CircuitClosed {
		t.Errorf("initial state = %v, want %v", cb.State, CircuitClosed)
	}
	if cb.ServiceName != "test-svc" {
		t.Errorf("service name = %q, want %q", cb.ServiceName, "test-svc")
	}
}

func TestRegisterEndpoint_MultipleEndpointsSameService(t *testing.T) {
	svc := newTestService()
	eps := registerN(t, svc, "multi-svc", 5)

	got := svc.GetEndpoints("multi-svc")
	if len(got) != 5 {
		t.Fatalf("got %d endpoints, want 5", len(got))
	}

	// Verify all endpoints are present.
	for i, ep := range eps {
		if got[i].ID != ep.ID {
			t.Errorf("endpoint[%d].ID = %q, want %q", i, got[i].ID, ep.ID)
		}
	}
}

func TestRegisterEndpoint_DoesNotReinitializeCircuitBreaker(t *testing.T) {
	svc := newTestService()

	// Register first endpoint, trip the circuit breaker.
	ep1 := &Endpoint{ServiceName: "cb-svc", Address: "10.0.0.1", Port: 8080}
	if err := svc.RegisterEndpoint(ep1); err != nil {
		t.Fatal(err)
	}

	// Record failures to open the circuit.
	for i := 0; i < svc.config.CBThreshold; i++ {
		svc.RecordFailure("cb-svc")
	}

	cb, _ := svc.GetCircuitBreaker("cb-svc")
	if cb.State != CircuitOpen {
		t.Fatalf("expected circuit to be open, got %v", cb.State)
	}

	// Register a second endpoint for the same service.
	ep2 := &Endpoint{ServiceName: "cb-svc", Address: "10.0.0.2", Port: 8081}
	if err := svc.RegisterEndpoint(ep2); err != nil {
		t.Fatal(err)
	}

	// Circuit breaker should still be open (not reinitialized).
	cb, _ = svc.GetCircuitBreaker("cb-svc")
	if cb.State != CircuitOpen {
		t.Errorf("circuit breaker was reinitialized on second register, state = %v", cb.State)
	}
}

// ---------------------------------------------------------------------------
// Service Discovery: DeregisterEndpoint
// ---------------------------------------------------------------------------

func TestDeregisterEndpoint(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*Service) string // returns endpoint ID to deregister
		wantError bool
		errSubstr string
	}{
		{
			name: "removes existing endpoint",
			setup: func(svc *Service) string {
				eps := registerN(t, svc, "svc-a", 3)
				return eps[1].ID // remove the middle one
			},
		},
		{
			name: "returns error for unknown ID",
			setup: func(svc *Service) string {
				return "nonexistent-id"
			},
			wantError: true,
			errSubstr: "endpoint not found",
		},
		{
			name: "removes first endpoint",
			setup: func(svc *Service) string {
				eps := registerN(t, svc, "svc-b", 2)
				return eps[0].ID
			},
		},
		{
			name: "removes last endpoint",
			setup: func(svc *Service) string {
				eps := registerN(t, svc, "svc-c", 2)
				return eps[1].ID
			},
		},
		{
			name: "removes only endpoint",
			setup: func(svc *Service) string {
				eps := registerN(t, svc, "svc-d", 1)
				return eps[0].ID
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService()
			id := tc.setup(svc)

			err := svc.DeregisterEndpoint(id)

			if tc.wantError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tc.errSubstr != "" && !strings.Contains(err.Error(), tc.errSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.errSubstr)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestDeregisterEndpoint_ReducesCount(t *testing.T) {
	svc := newTestService()
	eps := registerN(t, svc, "count-svc", 3)

	if err := svc.DeregisterEndpoint(eps[1].ID); err != nil {
		t.Fatal(err)
	}

	got := svc.GetEndpoints("count-svc")
	if len(got) != 2 {
		t.Errorf("got %d endpoints after deregister, want 2", len(got))
	}
}

func TestDeregisterEndpoint_DoubleDeregister(t *testing.T) {
	svc := newTestService()
	eps := registerN(t, svc, "double-svc", 1)

	if err := svc.DeregisterEndpoint(eps[0].ID); err != nil {
		t.Fatal(err)
	}

	err := svc.DeregisterEndpoint(eps[0].ID)
	if err == nil {
		t.Error("second deregister should return error")
	}
}

// ---------------------------------------------------------------------------
// Service Discovery: GetEndpoints
// ---------------------------------------------------------------------------

func TestGetEndpoints(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(*Service)
		serviceName string
		wantCount   int
	}{
		{
			name:        "returns nil for unknown service",
			setup:       func(svc *Service) {},
			serviceName: "unknown",
			wantCount:   0,
		},
		{
			name: "returns only healthy endpoints",
			setup: func(svc *Service) {
				registerN(t, svc, "mixed", 3)
				// Mark one as unhealthy directly.
				svc.mu.Lock()
				svc.endpoints["mixed"][1].Healthy = false
				svc.mu.Unlock()
			},
			serviceName: "mixed",
			wantCount:   2,
		},
		{
			name: "returns all when all healthy",
			setup: func(svc *Service) {
				registerN(t, svc, "all-healthy", 4)
			},
			serviceName: "all-healthy",
			wantCount:   4,
		},
		{
			name: "returns none when all unhealthy",
			setup: func(svc *Service) {
				registerN(t, svc, "all-sick", 2)
				svc.mu.Lock()
				for _, ep := range svc.endpoints["all-sick"] {
					ep.Healthy = false
				}
				svc.mu.Unlock()
			},
			serviceName: "all-sick",
			wantCount:   0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService()
			tc.setup(svc)
			got := svc.GetEndpoints(tc.serviceName)
			if len(got) != tc.wantCount {
				t.Errorf("got %d endpoints, want %d", len(got), tc.wantCount)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Service Discovery: SelectEndpoint
// ---------------------------------------------------------------------------

func TestSelectEndpoint(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*Service)
		service   string
		wantErr   bool
		errSubstr string
	}{
		{
			name: "selects first healthy endpoint",
			setup: func(svc *Service) {
				registerN(t, svc, "select-svc", 3)
			},
			service: "select-svc",
		},
		{
			name:      "errors when no endpoints registered",
			setup:     func(svc *Service) {},
			service:   "no-such-svc",
			wantErr:   true,
			errSubstr: "no healthy endpoints",
		},
		{
			name: "errors when circuit is open",
			setup: func(svc *Service) {
				registerN(t, svc, "open-cb", 2)
				for i := 0; i < 5; i++ {
					svc.RecordFailure("open-cb")
				}
			},
			service:   "open-cb",
			wantErr:   true,
			errSubstr: "circuit open",
		},
		{
			name: "errors when all endpoints unhealthy",
			setup: func(svc *Service) {
				registerN(t, svc, "sick-svc", 2)
				svc.mu.Lock()
				for _, ep := range svc.endpoints["sick-svc"] {
					ep.Healthy = false
				}
				svc.mu.Unlock()
			},
			service:   "sick-svc",
			wantErr:   true,
			errSubstr: "no healthy endpoints",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService()
			tc.setup(svc)

			ep, err := svc.SelectEndpoint(tc.service)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tc.errSubstr != "" && !strings.Contains(err.Error(), tc.errSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.errSubstr)
				}
				if ep != nil {
					t.Error("expected nil endpoint on error")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if ep == nil {
					t.Fatal("expected non-nil endpoint")
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Circuit Breaker: State Transitions
// ---------------------------------------------------------------------------

func TestCircuitBreaker_ClosedToOpen(t *testing.T) {
	threshold := 3
	svc := newTestService(func(c *Config) {
		c.CBThreshold = threshold
	})
	registerN(t, svc, "cb-test", 1)

	// Record failures below threshold -- should remain closed.
	for i := 0; i < threshold-1; i++ {
		svc.RecordFailure("cb-test")
		cb, _ := svc.GetCircuitBreaker("cb-test")
		if cb.State != CircuitClosed {
			t.Fatalf("expected closed after %d failures, got %v", i+1, cb.State)
		}
	}

	// One more failure should trip the breaker.
	svc.RecordFailure("cb-test")
	cb, _ := svc.GetCircuitBreaker("cb-test")
	if cb.State != CircuitOpen {
		t.Fatalf("expected open after %d failures, got %v", threshold, cb.State)
	}
	if cb.OpenedAt == nil {
		t.Error("OpenedAt should be set when circuit opens")
	}
	if cb.Failures != threshold {
		t.Errorf("Failures = %d, want %d", cb.Failures, threshold)
	}
}

func TestCircuitBreaker_OpenToHalfOpen(t *testing.T) {
	cbTimeout := 50 * time.Millisecond
	svc := newTestService(func(c *Config) {
		c.CBThreshold = 1
		c.CBTimeout = cbTimeout
	})
	registerN(t, svc, "ho-test", 1)

	// Trip the breaker.
	svc.RecordFailure("ho-test")
	cb, _ := svc.GetCircuitBreaker("ho-test")
	if cb.State != CircuitOpen {
		t.Fatalf("expected open, got %v", cb.State)
	}

	// Wait for the CB timeout to elapse.
	time.Sleep(cbTimeout + 20*time.Millisecond)

	// Manually trigger the check (simulates what the background loop does).
	svc.checkCircuitBreakers()

	cb, _ = svc.GetCircuitBreaker("ho-test")
	if cb.State != CircuitHalfOpen {
		t.Fatalf("expected half_open after timeout, got %v", cb.State)
	}
}

func TestCircuitBreaker_HalfOpenToClosed(t *testing.T) {
	svc := newTestService(func(c *Config) {
		c.CBThreshold = 1
		c.CBTimeout = 1 * time.Millisecond
	})
	registerN(t, svc, "close-test", 1)

	// Trip the breaker.
	svc.RecordFailure("close-test")

	// Transition to half-open.
	time.Sleep(5 * time.Millisecond)
	svc.checkCircuitBreakers()

	cb, _ := svc.GetCircuitBreaker("close-test")
	if cb.State != CircuitHalfOpen {
		t.Fatalf("expected half_open, got %v", cb.State)
	}

	// A success in half-open closes the circuit.
	svc.RecordSuccess("close-test")

	cb, _ = svc.GetCircuitBreaker("close-test")
	if cb.State != CircuitClosed {
		t.Fatalf("expected closed after success in half_open, got %v", cb.State)
	}
	if cb.Failures != 0 {
		t.Errorf("failures should be reset to 0, got %d", cb.Failures)
	}
}

func TestCircuitBreaker_SuccessInClosedState(t *testing.T) {
	svc := newTestService()
	registerN(t, svc, "success-svc", 1)

	svc.RecordSuccess("success-svc")

	cb, _ := svc.GetCircuitBreaker("success-svc")
	if cb.State != CircuitClosed {
		t.Errorf("expected closed, got %v", cb.State)
	}
	if cb.Successes != 1 {
		t.Errorf("Successes = %d, want 1", cb.Successes)
	}
	if cb.LastSuccess == nil {
		t.Error("LastSuccess should be set")
	}
}

func TestCircuitBreaker_FailureBeyondThreshold(t *testing.T) {
	svc := newTestService(func(c *Config) {
		c.CBThreshold = 2
	})
	registerN(t, svc, "extra-fail", 1)

	// Exceed the threshold by several more failures.
	for i := 0; i < 10; i++ {
		svc.RecordFailure("extra-fail")
	}

	cb, _ := svc.GetCircuitBreaker("extra-fail")
	if cb.State != CircuitOpen {
		t.Errorf("expected open, got %v", cb.State)
	}
	if cb.Failures != 10 {
		t.Errorf("Failures = %d, want 10", cb.Failures)
	}
}

func TestCircuitBreaker_RecordOnUnknownService(t *testing.T) {
	svc := newTestService()

	// Should not panic; no-ops for unknown services.
	svc.RecordSuccess("ghost-service")
	svc.RecordFailure("ghost-service")

	_, ok := svc.GetCircuitBreaker("ghost-service")
	if ok {
		t.Error("should not have a circuit breaker for unregistered service")
	}
}

func TestCircuitBreaker_CheckDoesNotTransitionClosed(t *testing.T) {
	svc := newTestService()
	registerN(t, svc, "closed-check", 1)

	// Circuit is closed; checkCircuitBreakers should be a no-op.
	svc.checkCircuitBreakers()

	cb, _ := svc.GetCircuitBreaker("closed-check")
	if cb.State != CircuitClosed {
		t.Errorf("expected closed, got %v", cb.State)
	}
}

func TestCircuitBreaker_OpenBeforeTimeoutStaysOpen(t *testing.T) {
	svc := newTestService(func(c *Config) {
		c.CBThreshold = 1
		c.CBTimeout = 1 * time.Hour // very long
	})
	registerN(t, svc, "long-open", 1)

	svc.RecordFailure("long-open")

	cb, _ := svc.GetCircuitBreaker("long-open")
	if cb.State != CircuitOpen {
		t.Fatalf("expected open, got %v", cb.State)
	}

	// Check breakers -- should still be open because timeout hasn't elapsed.
	svc.checkCircuitBreakers()

	cb, _ = svc.GetCircuitBreaker("long-open")
	if cb.State != CircuitOpen {
		t.Errorf("expected circuit to stay open before timeout, got %v", cb.State)
	}
}

func TestGetCircuitBreaker_NotFound(t *testing.T) {
	svc := newTestService()
	_, ok := svc.GetCircuitBreaker("does-not-exist")
	if ok {
		t.Error("expected ok=false for non-existent circuit breaker")
	}
}

// ---------------------------------------------------------------------------
// Circuit Breaker Background Loop
// ---------------------------------------------------------------------------

func TestCircuitBreakerLoop_TransitionsOnTimeout(t *testing.T) {
	cbTimeout := 50 * time.Millisecond
	svc := newTestService(func(c *Config) {
		c.CBThreshold = 1
		c.CBTimeout = cbTimeout
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	registerN(t, svc, "loop-svc", 1)
	svc.RecordFailure("loop-svc")

	// Start the service which launches the loop.
	if err := svc.Start(ctx); err != nil {
		t.Fatal(err)
	}

	// The default ticker is 5 seconds, which is too slow for tests.
	// Instead, we test checkCircuitBreakers directly.
	time.Sleep(cbTimeout + 10*time.Millisecond)
	svc.checkCircuitBreakers()

	cb, _ := svc.GetCircuitBreaker("loop-svc")
	if cb.State != CircuitHalfOpen {
		t.Errorf("expected half_open, got %v", cb.State)
	}
}

// ---------------------------------------------------------------------------
// Traffic Policies
// ---------------------------------------------------------------------------

func TestSetPolicy(t *testing.T) {
	tests := []struct {
		name   string
		policy *TrafficPolicy
		wantID string
	}{
		{
			name: "auto-generates ID when empty",
			policy: &TrafficPolicy{
				ServiceName: "policy-svc",
				Timeout:     5 * time.Second,
			},
			wantID: "policy-policy-svc",
		},
		{
			name: "preserves explicit ID",
			policy: &TrafficPolicy{
				ID:          "my-policy",
				ServiceName: "policy-svc-2",
				Timeout:     10 * time.Second,
			},
			wantID: "my-policy",
		},
		{
			name: "with retry policy",
			policy: &TrafficPolicy{
				ServiceName: "retry-svc",
				Retries: RetryPolicy{
					Attempts:      3,
					PerTryTimeout: 2 * time.Second,
					RetryOn:       []string{"5xx", "reset"},
				},
			},
			wantID: "policy-retry-svc",
		},
		{
			name: "with circuit breaker policy",
			policy: &TrafficPolicy{
				ServiceName: "cb-policy-svc",
				CircuitBreaker: CBPolicy{
					Threshold:        5,
					Timeout:          30 * time.Second,
					HalfOpenRequests: 3,
				},
			},
			wantID: "policy-cb-policy-svc",
		},
		{
			name: "with load balancer policy",
			policy: &TrafficPolicy{
				ServiceName: "lb-svc",
				LoadBalancer: LBPolicy{
					Algorithm: "round_robin",
				},
			},
			wantID: "policy-lb-svc",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService()

			if err := svc.SetPolicy(tc.policy); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.policy.ID != tc.wantID {
				t.Errorf("ID = %q, want %q", tc.policy.ID, tc.wantID)
			}

			got, ok := svc.GetPolicy(tc.policy.ServiceName)
			if !ok {
				t.Fatal("policy not found after set")
			}
			if got.ServiceName != tc.policy.ServiceName {
				t.Errorf("service name = %q, want %q", got.ServiceName, tc.policy.ServiceName)
			}
		})
	}
}

func TestSetPolicy_OverwritesExisting(t *testing.T) {
	svc := newTestService()

	p1 := &TrafficPolicy{ServiceName: "overwrite-svc", Timeout: 5 * time.Second}
	if err := svc.SetPolicy(p1); err != nil {
		t.Fatal(err)
	}

	p2 := &TrafficPolicy{ServiceName: "overwrite-svc", Timeout: 10 * time.Second}
	if err := svc.SetPolicy(p2); err != nil {
		t.Fatal(err)
	}

	got, ok := svc.GetPolicy("overwrite-svc")
	if !ok {
		t.Fatal("policy not found")
	}
	if got.Timeout != 10*time.Second {
		t.Errorf("Timeout = %v, want 10s", got.Timeout)
	}
}

func TestGetPolicy_NotFound(t *testing.T) {
	svc := newTestService()
	_, ok := svc.GetPolicy("nonexistent")
	if ok {
		t.Error("expected ok=false for missing policy")
	}
}

// ---------------------------------------------------------------------------
// Config Propagation
// ---------------------------------------------------------------------------

func TestConfigPropagation(t *testing.T) {
	tests := []struct {
		name     string
		modifier func(*Config)
		verify   func(*testing.T, *Service)
	}{
		{
			name: "custom CB threshold propagates to RecordFailure",
			modifier: func(c *Config) {
				c.CBThreshold = 2
			},
			verify: func(t *testing.T, svc *Service) {
				registerN(t, svc, "prop-svc", 1)
				svc.RecordFailure("prop-svc")
				cb, _ := svc.GetCircuitBreaker("prop-svc")
				if cb.State != CircuitClosed {
					t.Error("should still be closed after 1 failure with threshold=2")
				}
				svc.RecordFailure("prop-svc")
				cb, _ = svc.GetCircuitBreaker("prop-svc")
				if cb.State != CircuitOpen {
					t.Error("should be open after 2 failures with threshold=2")
				}
			},
		},
		{
			name: "custom CB timeout propagates to checkCircuitBreakers",
			modifier: func(c *Config) {
				c.CBThreshold = 1
				c.CBTimeout = 1 * time.Millisecond
			},
			verify: func(t *testing.T, svc *Service) {
				registerN(t, svc, "timeout-svc", 1)
				svc.RecordFailure("timeout-svc")
				time.Sleep(5 * time.Millisecond)
				svc.checkCircuitBreakers()
				cb, _ := svc.GetCircuitBreaker("timeout-svc")
				if cb.State != CircuitHalfOpen {
					t.Errorf("expected half_open, got %v", cb.State)
				}
			},
		},
		{
			name: "cert rotation interval propagates to IssueCertificate",
			modifier: func(c *Config) {
				c.CertRotationInterval = 48 * time.Hour
			},
			verify: func(t *testing.T, svc *Service) {
				cert, err := svc.IssueCertificate("cert-svc")
				if err != nil {
					t.Fatal(err)
				}
				duration := cert.NotAfter.Sub(cert.NotBefore)
				if duration != 48*time.Hour {
					t.Errorf("cert validity = %v, want 48h", duration)
				}
			},
		},
		{
			name: "mTLS enabled flag in stats",
			modifier: func(c *Config) {
				c.MTLSEnabled = false
			},
			verify: func(t *testing.T, svc *Service) {
				stats := svc.Stats()
				if stats["mtls_enabled"].(bool) != false {
					t.Error("expected mtls_enabled=false")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(tc.modifier)
			tc.verify(t, svc)
		})
	}
}

// ---------------------------------------------------------------------------
// mTLS Certificate Management
// ---------------------------------------------------------------------------

func TestIssueCertificate(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		wantCN      string
	}{
		{
			name:        "basic certificate issuance",
			serviceName: "web-api",
			wantCN:      "web-api.mesh.local",
		},
		{
			name:        "certificate for service with dashes",
			serviceName: "user-auth-service",
			wantCN:      "user-auth-service.mesh.local",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService()

			cert, err := svc.IssueCertificate(tc.serviceName)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if cert.CommonName != tc.wantCN {
				t.Errorf("CN = %q, want %q", cert.CommonName, tc.wantCN)
			}
			if cert.ServiceName != tc.serviceName {
				t.Errorf("ServiceName = %q, want %q", cert.ServiceName, tc.serviceName)
			}
			if cert.IsCA {
				t.Error("cert should not be CA")
			}
			if cert.ID == "" {
				t.Error("cert ID should not be empty")
			}
			if !strings.HasPrefix(cert.ID, "cert-"+tc.serviceName) {
				t.Errorf("cert ID %q should start with cert-%s", cert.ID, tc.serviceName)
			}
			if cert.NotBefore.IsZero() {
				t.Error("NotBefore should be set")
			}
			if cert.NotAfter.IsZero() {
				t.Error("NotAfter should be set")
			}
			if !cert.NotAfter.After(cert.NotBefore) {
				t.Error("NotAfter should be after NotBefore")
			}

			// Verify cert is stored.
			stored, ok := svc.GetCertificate(tc.serviceName)
			if !ok {
				t.Fatal("certificate not stored")
			}
			if stored.ID != cert.ID {
				t.Errorf("stored cert ID = %q, want %q", stored.ID, cert.ID)
			}
		})
	}
}

func TestIssueCertificate_OverwritesPrevious(t *testing.T) {
	svc := newTestService()

	cert1, _ := svc.IssueCertificate("overwrite-cert")
	// Small sleep to ensure different timestamp/nanos.
	time.Sleep(1 * time.Millisecond)
	cert2, _ := svc.IssueCertificate("overwrite-cert")

	if cert1.ID == cert2.ID {
		t.Error("reissued cert should have a different ID")
	}

	stored, ok := svc.GetCertificate("overwrite-cert")
	if !ok {
		t.Fatal("cert not found")
	}
	if stored.ID != cert2.ID {
		t.Error("stored cert should be the latest")
	}
}

func TestIssueCertificate_ValidityDuration(t *testing.T) {
	rotationInterval := 12 * time.Hour
	svc := newTestService(func(c *Config) {
		c.CertRotationInterval = rotationInterval
	})

	cert, err := svc.IssueCertificate("validity-svc")
	if err != nil {
		t.Fatal(err)
	}

	duration := cert.NotAfter.Sub(cert.NotBefore)
	if duration != rotationInterval {
		t.Errorf("validity = %v, want %v", duration, rotationInterval)
	}
}

func TestGetCertificate_NotFound(t *testing.T) {
	svc := newTestService()
	_, ok := svc.GetCertificate("nonexistent")
	if ok {
		t.Error("expected ok=false for missing certificate")
	}
}

// ---------------------------------------------------------------------------
// mTLS Handshake Mocking
// ---------------------------------------------------------------------------

func TestMTLSHandshake_SelfSignedCertificates(t *testing.T) {
	// Generate a self-signed CA cert.
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-mesh-ca"},
		NotBefore:    time.Now().Add(-1 * time.Minute),
		NotAfter:     time.Now().Add(1 * time.Hour),
		IsCA:         true,
		KeyUsage:     x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create CA cert: %v", err)
	}

	caCert, err := x509.ParseCertificate(caCertDER)
	if err != nil {
		t.Fatalf("parse CA cert: %v", err)
	}

	// Generate server cert signed by CA.
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate server key: %v", err)
	}

	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "server.mesh.local"},
		NotBefore:    time.Now().Add(-1 * time.Minute),
		NotAfter:     time.Now().Add(1 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
	}

	serverCertDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create server cert: %v", err)
	}

	serverCert, err := x509.ParseCertificate(serverCertDER)
	if err != nil {
		t.Fatalf("parse server cert: %v", err)
	}

	// Generate client cert signed by CA.
	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate client key: %v", err)
	}

	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "client.mesh.local"},
		NotBefore:    time.Now().Add(-1 * time.Minute),
		NotAfter:     time.Now().Add(1 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	clientCertDER, err := x509.CreateCertificate(rand.Reader, clientTemplate, caCert, &clientKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create client cert: %v", err)
	}

	// Build CA pool.
	caPool := x509.NewCertPool()
	caPool.AddCert(caCert)

	// Server TLS config: require client cert.
	serverTLS := &tls.Config{
		Certificates: []tls.Certificate{
			{
				Certificate: [][]byte{serverCertDER},
				PrivateKey:  serverKey,
				Leaf:        serverCert,
			},
		},
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  caPool,
		MinVersion: tls.VersionTLS13,
	}

	// Client TLS config: present client cert, verify server cert.
	clientTLS := &tls.Config{
		Certificates: []tls.Certificate{
			{
				Certificate: [][]byte{clientCertDER},
				PrivateKey:  clientKey,
			},
		},
		RootCAs:    caPool,
		MinVersion: tls.VersionTLS13,
	}

	// Set up a TLS listener.
	ln, err := tls.Listen("tcp", "127.0.0.1:0", serverTLS)
	if err != nil {
		t.Fatalf("TLS listen: %v", err)
	}
	defer ln.Close()

	// Accept one connection in a goroutine.
	serverDone := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			serverDone <- fmt.Errorf("accept: %w", err)
			return
		}
		defer conn.Close()

		tlsConn := conn.(*tls.Conn)
		if err := tlsConn.Handshake(); err != nil {
			serverDone <- fmt.Errorf("server handshake: %w", err)
			return
		}

		state := tlsConn.ConnectionState()
		if len(state.PeerCertificates) == 0 {
			serverDone <- fmt.Errorf("no client certificate received")
			return
		}
		if state.PeerCertificates[0].Subject.CommonName != "client.mesh.local" {
			serverDone <- fmt.Errorf("unexpected client CN: %s", state.PeerCertificates[0].Subject.CommonName)
			return
		}

		serverDone <- nil
	}()

	// Connect as client.
	clientConn, err := tls.Dial("tcp", ln.Addr().String(), clientTLS)
	if err != nil {
		t.Fatalf("client dial: %v", err)
	}
	defer clientConn.Close()

	state := clientConn.ConnectionState()
	if !state.HandshakeComplete {
		t.Error("handshake not complete")
	}
	if len(state.PeerCertificates) == 0 {
		t.Fatal("no server certificate received")
	}
	if state.PeerCertificates[0].Subject.CommonName != "server.mesh.local" {
		t.Errorf("server CN = %q, want server.mesh.local", state.PeerCertificates[0].Subject.CommonName)
	}

	// Wait for server to verify client.
	if err := <-serverDone; err != nil {
		t.Fatalf("server-side error: %v", err)
	}
}

func TestMTLSHandshake_RejectsUntrustedClient(t *testing.T) {
	// Generate CA.
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "trusted-ca"},
		NotBefore:             time.Now().Add(-1 * time.Minute),
		NotAfter:              time.Now().Add(1 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	caCertDER, _ := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	caCert, _ := x509.ParseCertificate(caCertDER)

	caPool := x509.NewCertPool()
	caPool.AddCert(caCert)

	// Server cert signed by CA.
	srvKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	srvTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "server.mesh.local"},
		NotBefore:    time.Now().Add(-1 * time.Minute),
		NotAfter:     time.Now().Add(1 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	srvCertDER, _ := x509.CreateCertificate(rand.Reader, srvTemplate, caCert, &srvKey.PublicKey, caKey)

	serverTLS := &tls.Config{
		Certificates: []tls.Certificate{
			{Certificate: [][]byte{srvCertDER}, PrivateKey: srvKey},
		},
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  caPool,
		MinVersion: tls.VersionTLS13,
	}

	ln, err := tls.Listen("tcp", "127.0.0.1:0", serverTLS)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	// Accept connection (will fail handshake).
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		tlsConn := conn.(*tls.Conn)
		// This should fail because client cert is not trusted.
		_ = tlsConn.Handshake()
	}()

	// Generate an untrusted client cert (self-signed, not by our CA).
	untrustedKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	untrustedTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(99),
		Subject:      pkix.Name{CommonName: "rogue-client"},
		NotBefore:    time.Now().Add(-1 * time.Minute),
		NotAfter:     time.Now().Add(1 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	untrustedCertDER, _ := x509.CreateCertificate(rand.Reader, untrustedTemplate, untrustedTemplate, &untrustedKey.PublicKey, untrustedKey)

	rogueClientTLS := &tls.Config{
		Certificates: []tls.Certificate{
			{Certificate: [][]byte{untrustedCertDER}, PrivateKey: untrustedKey},
		},
		RootCAs:            caPool,
		InsecureSkipVerify: true, // skip server verification for this test
		MinVersion:         tls.VersionTLS13,
	}

	conn, err := tls.Dial("tcp", ln.Addr().String(), rogueClientTLS)
	if err != nil {
		// Dial itself failed -- untrusted cert was rejected. This is expected.
		return
	}
	defer conn.Close()

	// With TLS 1.3 the client handshake may appear to succeed initially
	// because client cert verification happens asynchronously on the
	// server. The rejection arrives as an alert when we try to read.
	_, writeErr := conn.Write([]byte("hello"))
	if writeErr != nil {
		return // rejected on write, expected
	}

	// Read should surface the server's certificate_required / bad_certificate alert.
	buf := make([]byte, 128)
	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, readErr := conn.Read(buf)
	if readErr == nil {
		t.Error("expected mTLS handshake to reject untrusted client certificate")
	}
}

func TestMTLSHandshake_ExpiredCertRejected(t *testing.T) {
	// Generate CA.
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-2 * time.Hour),
		NotAfter:              time.Now().Add(1 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	caCertDER, _ := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	caCert, _ := x509.ParseCertificate(caCertDER)

	caPool := x509.NewCertPool()
	caPool.AddCert(caCert)

	// Server cert (valid).
	srvKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	srvTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "server.mesh.local"},
		NotBefore:    time.Now().Add(-1 * time.Minute),
		NotAfter:     time.Now().Add(1 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	srvCertDER, _ := x509.CreateCertificate(rand.Reader, srvTemplate, caCert, &srvKey.PublicKey, caKey)

	serverTLS := &tls.Config{
		Certificates: []tls.Certificate{
			{Certificate: [][]byte{srvCertDER}, PrivateKey: srvKey},
		},
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  caPool,
		MinVersion: tls.VersionTLS13,
	}

	ln, err := tls.Listen("tcp", "127.0.0.1:0", serverTLS)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		tlsConn := conn.(*tls.Conn)
		_ = tlsConn.Handshake()
	}()

	// Expired client cert signed by the same CA.
	clientKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	expiredTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "expired-client.mesh.local"},
		NotBefore:    time.Now().Add(-2 * time.Hour),
		NotAfter:     time.Now().Add(-1 * time.Hour), // already expired
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	expiredCertDER, _ := x509.CreateCertificate(rand.Reader, expiredTemplate, caCert, &clientKey.PublicKey, caKey)

	clientTLS := &tls.Config{
		Certificates: []tls.Certificate{
			{Certificate: [][]byte{expiredCertDER}, PrivateKey: clientKey},
		},
		RootCAs:    caPool,
		MinVersion: tls.VersionTLS13,
	}

	conn, err := tls.Dial("tcp", ln.Addr().String(), clientTLS)
	if err != nil {
		// Dial failed -- expired cert rejected at handshake. Expected.
		return
	}
	defer conn.Close()

	// Write may succeed before server processes the expired cert.
	_, writeErr := conn.Write([]byte("hello"))
	if writeErr != nil {
		return // rejected on write, expected
	}

	// Read should surface the server's certificate rejection alert.
	buf := make([]byte, 128)
	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, readErr := conn.Read(buf)
	if readErr == nil {
		t.Error("expected expired client certificate to be rejected")
	}
}

// ---------------------------------------------------------------------------
// Stats
// ---------------------------------------------------------------------------

func TestStats(t *testing.T) {
	svc := newTestService(func(c *Config) {
		c.MTLSEnabled = true
	})

	// Empty state.
	stats := svc.Stats()
	if stats["total_services"].(int) != 0 {
		t.Errorf("total_services = %v, want 0", stats["total_services"])
	}
	if stats["total_endpoints"].(int) != 0 {
		t.Errorf("total_endpoints = %v, want 0", stats["total_endpoints"])
	}
	if stats["total_policies"].(int) != 0 {
		t.Errorf("total_policies = %v, want 0", stats["total_policies"])
	}
	if stats["open_circuits"].(int) != 0 {
		t.Errorf("open_circuits = %v, want 0", stats["open_circuits"])
	}
	if stats["certificates"].(int) != 0 {
		t.Errorf("certificates = %v, want 0", stats["certificates"])
	}
	if stats["mtls_enabled"].(bool) != true {
		t.Error("mtls_enabled should be true")
	}

	// Populate some state.
	registerN(t, svc, "svc-a", 3)
	registerN(t, svc, "svc-b", 2)
	svc.SetPolicy(&TrafficPolicy{ServiceName: "svc-a"})
	svc.IssueCertificate("svc-a")
	svc.IssueCertificate("svc-b")

	// Trip one circuit breaker.
	for i := 0; i < svc.config.CBThreshold; i++ {
		svc.RecordFailure("svc-a")
	}

	stats = svc.Stats()
	if stats["total_services"].(int) != 2 {
		t.Errorf("total_services = %v, want 2", stats["total_services"])
	}
	if stats["total_endpoints"].(int) != 5 {
		t.Errorf("total_endpoints = %v, want 5", stats["total_endpoints"])
	}
	if stats["total_policies"].(int) != 1 {
		t.Errorf("total_policies = %v, want 1", stats["total_policies"])
	}
	if stats["open_circuits"].(int) != 1 {
		t.Errorf("open_circuits = %v, want 1", stats["open_circuits"])
	}
	if stats["certificates"].(int) != 2 {
		t.Errorf("certificates = %v, want 2", stats["certificates"])
	}
}

func TestStats_MTLSDisabled(t *testing.T) {
	svc := newTestService(func(c *Config) {
		c.MTLSEnabled = false
	})
	stats := svc.Stats()
	if stats["mtls_enabled"].(bool) != false {
		t.Error("expected mtls_enabled=false")
	}
}

// ---------------------------------------------------------------------------
// CircuitBreakerState constants
// ---------------------------------------------------------------------------

func TestCircuitBreakerStateValues(t *testing.T) {
	tests := []struct {
		state CircuitBreakerState
		want  string
	}{
		{CircuitClosed, "closed"},
		{CircuitOpen, "open"},
		{CircuitHalfOpen, "half_open"},
	}

	for _, tc := range tests {
		t.Run(string(tc.state), func(t *testing.T) {
			if string(tc.state) != tc.want {
				t.Errorf("got %q, want %q", string(tc.state), tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Edge Cases and Error Paths
// ---------------------------------------------------------------------------

func TestRegisterEndpoint_EmptyServiceName(t *testing.T) {
	svc := newTestService()
	ep := &Endpoint{
		ServiceName: "",
		Address:     "10.0.0.1",
		Port:        8080,
	}
	// Should still work -- ID is auto-generated with empty service name.
	err := svc.RegisterEndpoint(ep)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.ID != "-10.0.0.1-8080" {
		t.Errorf("ID = %q, want %q", ep.ID, "-10.0.0.1-8080")
	}
}

func TestSelectEndpoint_HalfOpenAllowsSelection(t *testing.T) {
	svc := newTestService(func(c *Config) {
		c.CBThreshold = 1
		c.CBTimeout = 1 * time.Millisecond
	})
	registerN(t, svc, "halfopen-select", 1)

	svc.RecordFailure("halfopen-select")
	time.Sleep(5 * time.Millisecond)
	svc.checkCircuitBreakers()

	cb, _ := svc.GetCircuitBreaker("halfopen-select")
	if cb.State != CircuitHalfOpen {
		t.Fatalf("expected half_open, got %v", cb.State)
	}

	// Half-open should allow selection (only CircuitOpen blocks).
	ep, err := svc.SelectEndpoint("halfopen-select")
	if err != nil {
		t.Fatalf("unexpected error selecting in half-open state: %v", err)
	}
	if ep == nil {
		t.Error("expected an endpoint in half-open state")
	}
}

func TestRecordFailure_DoesNotReopenAlreadyOpen(t *testing.T) {
	svc := newTestService(func(c *Config) {
		c.CBThreshold = 2
	})
	registerN(t, svc, "already-open", 1)

	// Open the circuit.
	svc.RecordFailure("already-open")
	svc.RecordFailure("already-open")

	cb, _ := svc.GetCircuitBreaker("already-open")
	if cb.State != CircuitOpen {
		t.Fatalf("expected open, got %v", cb.State)
	}
	openedAt := cb.OpenedAt

	// More failures should not reset OpenedAt.
	svc.RecordFailure("already-open")
	cb, _ = svc.GetCircuitBreaker("already-open")
	if cb.OpenedAt != openedAt {
		t.Error("OpenedAt should not change once circuit is open")
	}
}

func TestDeregisterEndpoint_AcrossMultipleServices(t *testing.T) {
	svc := newTestService()
	epsA := registerN(t, svc, "svc-x", 2)
	epsB := registerN(t, svc, "svc-y", 2)

	// Deregister from svc-y.
	if err := svc.DeregisterEndpoint(epsB[0].ID); err != nil {
		t.Fatal(err)
	}

	// svc-x should be unaffected.
	if len(svc.GetEndpoints("svc-x")) != 2 {
		t.Error("svc-x endpoints should be unaffected")
	}

	// svc-y should have one endpoint.
	if len(svc.GetEndpoints("svc-y")) != 1 {
		t.Error("svc-y should have 1 endpoint after deregister")
	}

	_ = epsA // used above
}

func TestIssueCertificate_UniqueIDs(t *testing.T) {
	svc := newTestService()
	seen := make(map[string]bool)

	for i := 0; i < 50; i++ {
		cert, err := svc.IssueCertificate(fmt.Sprintf("svc-%d", i))
		if err != nil {
			t.Fatal(err)
		}
		if seen[cert.ID] {
			t.Fatalf("duplicate cert ID: %s", cert.ID)
		}
		seen[cert.ID] = true
	}
}

// ---------------------------------------------------------------------------
// Concurrent (Race-Safe) Tests
// ---------------------------------------------------------------------------

func TestConcurrent_RegisterAndGetEndpoints(t *testing.T) {
	svc := newTestService()
	const goroutines = 20
	const perGoroutine = 10

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				ep := &Endpoint{
					ServiceName: "concurrent-svc",
					Address:     fmt.Sprintf("10.0.%d.%d", g, i),
					Port:        8080,
				}
				if err := svc.RegisterEndpoint(ep); err != nil {
					t.Errorf("register error: %v", err)
				}
			}
		}(g)
	}

	// Concurrent reads while writing.
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			select {
			case <-readDone:
				return
			default:
				_ = svc.GetEndpoints("concurrent-svc")
			}
		}
	}()

	wg.Wait()
	// Signal read goroutine to stop by replacing its loop condition.
	// (The channel is already used, so just let it run and it will stop
	// when the test ends.)

	got := svc.GetEndpoints("concurrent-svc")
	expected := goroutines * perGoroutine
	if len(got) != expected {
		t.Errorf("got %d endpoints, want %d", len(got), expected)
	}
}

func TestConcurrent_RecordFailureAndSuccess(t *testing.T) {
	svc := newTestService(func(c *Config) {
		c.CBThreshold = 100 // high threshold to prevent circuit opening during test
	})
	registerN(t, svc, "race-svc", 1)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			svc.RecordFailure("race-svc")
		}()
		go func() {
			defer wg.Done()
			svc.RecordSuccess("race-svc")
		}()
	}

	wg.Wait()

	cb, ok := svc.GetCircuitBreaker("race-svc")
	if !ok {
		t.Fatal("circuit breaker not found")
	}
	// Total operations = goroutines failures + goroutines successes.
	totalOps := cb.Failures + cb.Successes
	if totalOps != goroutines*2 {
		t.Errorf("total operations = %d, want %d", totalOps, goroutines*2)
	}
}

func TestConcurrent_SelectEndpoint(t *testing.T) {
	svc := newTestService()
	registerN(t, svc, "select-race", 5)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			ep, err := svc.SelectEndpoint("select-race")
			if err != nil {
				t.Errorf("select error: %v", err)
			}
			if ep == nil {
				t.Error("got nil endpoint")
			}
		}()
	}

	wg.Wait()
}

func TestConcurrent_PolicySetAndGet(t *testing.T) {
	svc := newTestService()

	const goroutines = 30
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			svc.SetPolicy(&TrafficPolicy{
				ServiceName: fmt.Sprintf("policy-race-%d", i%5),
				Timeout:     time.Duration(i) * time.Second,
			})
		}(i)
		go func(i int) {
			defer wg.Done()
			svc.GetPolicy(fmt.Sprintf("policy-race-%d", i%5))
		}(i)
	}

	wg.Wait()
}

func TestConcurrent_CertificateIssueAndGet(t *testing.T) {
	svc := newTestService()

	const goroutines = 30
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			_, err := svc.IssueCertificate(fmt.Sprintf("cert-race-%d", i%5))
			if err != nil {
				t.Errorf("issue cert error: %v", err)
			}
		}(i)
		go func(i int) {
			defer wg.Done()
			svc.GetCertificate(fmt.Sprintf("cert-race-%d", i%5))
		}(i)
	}

	wg.Wait()
}

func TestConcurrent_StatsWhileMutating(t *testing.T) {
	svc := newTestService()

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines * 3)

	// Writers.
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			svc.RegisterEndpoint(&Endpoint{
				ServiceName: fmt.Sprintf("stats-svc-%d", i%3),
				Address:     fmt.Sprintf("10.0.0.%d", i),
				Port:        8080,
			})
		}(i)
		go func(i int) {
			defer wg.Done()
			svc.SetPolicy(&TrafficPolicy{
				ServiceName: fmt.Sprintf("stats-svc-%d", i%3),
			})
		}(i)
	}

	// Readers (stats).
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			stats := svc.Stats()
			// Just verify it returns valid data.
			_ = stats["total_services"]
			_ = stats["total_endpoints"]
		}()
	}

	wg.Wait()
}

func TestConcurrent_RegisterAndDeregister(t *testing.T) {
	svc := newTestService()

	// Pre-register some endpoints.
	eps := registerN(t, svc, "dereg-race", 20)

	var wg sync.WaitGroup

	// Deregister in parallel.
	wg.Add(len(eps))
	for _, ep := range eps {
		go func(id string) {
			defer wg.Done()
			_ = svc.DeregisterEndpoint(id)
		}(ep.ID)
	}

	// Also register new ones concurrently.
	wg.Add(10)
	for i := 0; i < 10; i++ {
		go func(i int) {
			defer wg.Done()
			svc.RegisterEndpoint(&Endpoint{
				ServiceName: "dereg-race",
				Address:     fmt.Sprintf("10.1.0.%d", i),
				Port:        9090,
			})
		}(i)
	}

	wg.Wait()
}

func TestConcurrent_CircuitBreakerCheckWhileRecording(t *testing.T) {
	svc := newTestService(func(c *Config) {
		c.CBThreshold = 5
		c.CBTimeout = 1 * time.Millisecond
	})
	registerN(t, svc, "check-race", 1)

	var wg sync.WaitGroup
	wg.Add(30)

	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			svc.RecordFailure("check-race")
		}()
		go func() {
			defer wg.Done()
			svc.RecordSuccess("check-race")
		}()
		go func() {
			defer wg.Done()
			svc.checkCircuitBreakers()
		}()
	}

	wg.Wait()
}

// ---------------------------------------------------------------------------
// Integration-style test: Full lifecycle
// ---------------------------------------------------------------------------

func TestFullLifecycle(t *testing.T) {
	svc := newTestService(func(c *Config) {
		c.CBThreshold = 3
		c.CBTimeout = 10 * time.Millisecond
		c.CertRotationInterval = 1 * time.Hour
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Start the service.
	if err := svc.Start(ctx); err != nil {
		t.Fatal(err)
	}

	// 2. Register endpoints.
	for i := 0; i < 3; i++ {
		ep := &Endpoint{
			ServiceName: "payment-api",
			Address:     fmt.Sprintf("10.0.1.%d", i),
			Port:        8080,
		}
		if err := svc.RegisterEndpoint(ep); err != nil {
			t.Fatal(err)
		}
	}

	// 3. Set a traffic policy.
	if err := svc.SetPolicy(&TrafficPolicy{
		ServiceName: "payment-api",
		Timeout:     5 * time.Second,
		Retries:     RetryPolicy{Attempts: 3, PerTryTimeout: 1 * time.Second},
		LoadBalancer: LBPolicy{Algorithm: "round_robin"},
	}); err != nil {
		t.Fatal(err)
	}

	// 4. Issue a certificate.
	cert, err := svc.IssueCertificate("payment-api")
	if err != nil {
		t.Fatal(err)
	}
	if cert.CommonName != "payment-api.mesh.local" {
		t.Errorf("CN = %q", cert.CommonName)
	}

	// 5. Select an endpoint (should work -- circuit is closed).
	ep, err := svc.SelectEndpoint("payment-api")
	if err != nil {
		t.Fatal(err)
	}
	if ep == nil {
		t.Fatal("expected endpoint")
	}

	// 6. Record some successes.
	for i := 0; i < 5; i++ {
		svc.RecordSuccess("payment-api")
	}

	// 7. Trip the circuit breaker.
	for i := 0; i < 3; i++ {
		svc.RecordFailure("payment-api")
	}

	cb, _ := svc.GetCircuitBreaker("payment-api")
	if cb.State != CircuitOpen {
		t.Fatalf("expected open, got %v", cb.State)
	}

	// 8. Selection should fail when circuit is open.
	_, err = svc.SelectEndpoint("payment-api")
	if err == nil {
		t.Fatal("expected error when circuit is open")
	}

	// 9. Wait for half-open transition.
	time.Sleep(15 * time.Millisecond)
	svc.checkCircuitBreakers()

	cb, _ = svc.GetCircuitBreaker("payment-api")
	if cb.State != CircuitHalfOpen {
		t.Fatalf("expected half_open, got %v", cb.State)
	}

	// 10. Record success to close the circuit.
	svc.RecordSuccess("payment-api")

	cb, _ = svc.GetCircuitBreaker("payment-api")
	if cb.State != CircuitClosed {
		t.Fatalf("expected closed, got %v", cb.State)
	}

	// 11. Deregister an endpoint.
	if err := svc.DeregisterEndpoint(ep.ID); err != nil {
		t.Fatal(err)
	}

	// 12. Check stats.
	stats := svc.Stats()
	if stats["total_services"].(int) != 1 {
		t.Errorf("total_services = %v", stats["total_services"])
	}
	if stats["total_endpoints"].(int) != 2 { // 3 registered, 1 removed
		t.Errorf("total_endpoints = %v, want 2", stats["total_endpoints"])
	}

	// 13. Stop the service.
	cancel()
	if err := svc.Stop(); err != nil {
		t.Fatal(err)
	}
}
