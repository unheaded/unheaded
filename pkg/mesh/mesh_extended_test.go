// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package mesh

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/mesh/discovery"
	"unheaded/pkg/mesh/proxy"
)

// ===========================================================================
// Mesh internal functions that need more coverage
// ===========================================================================

func TestMeshCleanupConnectionPoolsRemovesEmpty(t *testing.T) {
	c := DefaultConfig()
	c.DiscoveryType = DiscoveryTypeStatic
	c.InboundPort = 0
	c.OutboundPort = 0

	m, err := NewMesh(c)
	if err != nil {
		t.Fatalf("NewMesh: %v", err)
	}

	pool := NewConnectionPool("10.0.0.1:80", nil)
	m.connPoolsMu.Lock()
	m.connPools["10.0.0.1:80"] = pool
	m.connPoolsMu.Unlock()

	m.cleanupConnectionPools()

	m.connPoolsMu.RLock()
	_, exists := m.connPools["10.0.0.1:80"]
	m.connPoolsMu.RUnlock()
	if exists {
		t.Error("empty pool should have been removed during cleanup")
	}
}

func TestMeshCheckCircuitBreakersRuns(t *testing.T) {
	c := DefaultConfig()
	c.DiscoveryType = DiscoveryTypeStatic
	c.InboundPort = 0
	c.OutboundPort = 0

	m, err := NewMesh(c)
	if err != nil {
		t.Fatalf("NewMesh: %v", err)
	}

	m.getOrCreateCircuitBreaker("svc-a")
	m.getOrCreateCircuitBreaker("svc-b")

	// Should not panic and should check all circuit breakers
	m.checkCircuitBreakers()
}

func TestMeshSyncServicesRuns(t *testing.T) {
	c := DefaultConfig()
	c.DiscoveryType = DiscoveryTypeStatic
	c.InboundPort = 0
	c.OutboundPort = 0

	m, err := NewMesh(c)
	if err != nil {
		t.Fatalf("NewMesh: %v", err)
	}
	m.discovery.Start(context.Background())

	svc := &ServiceInstance{ServiceName: "api", Namespace: "default"}
	m.registry.Register(svc)

	// Should not panic even though discovery will return errors
	m.syncServices()
}

func TestMeshOnEndpointHealthChangeNoop(t *testing.T) {
	c := DefaultConfig()
	c.DiscoveryType = DiscoveryTypeStatic
	c.InboundPort = 0
	c.OutboundPort = 0

	m, err := NewMesh(c)
	if err != nil {
		t.Fatalf("NewMesh: %v", err)
	}

	// Should not panic — it's a noop
	m.onEndpointHealthChange("ep1", true)
	m.onEndpointHealthChange("ep2", false)
}

func TestMeshHandleInboundRequestNotStarted(t *testing.T) {
	c := DefaultConfig()
	c.DiscoveryType = DiscoveryTypeStatic
	c.InboundPort = 0
	c.OutboundPort = 0

	m, err := NewMesh(c)
	if err != nil {
		t.Fatalf("NewMesh: %v", err)
	}

	headers := make(http.Header)
	headers.Set("X-Mesh-Service", "api")

	req := &ProxyRequest{
		Method:  "GET",
		Path:    "/health",
		Headers: headers,
	}

	resp, err := m.handleInboundRequest(context.Background(), req)
	if err == nil {
		t.Error("expected error when mesh not started")
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("expected 502 status, got %d", resp.StatusCode)
	}
}

func TestMeshHandleInboundRequestDefaultNamespace(t *testing.T) {
	c := DefaultConfig()
	c.DiscoveryType = DiscoveryTypeStatic
	c.InboundPort = 0
	c.OutboundPort = 0
	c.Namespace = "production"

	m, err := NewMesh(c)
	if err != nil {
		t.Fatalf("NewMesh: %v", err)
	}

	// Headers without namespace
	headers := make(http.Header)
	headers.Set("X-Mesh-Service", "api")

	req := &ProxyRequest{
		Method:  "GET",
		Path:    "/test",
		Headers: headers,
	}

	resp, _ := m.handleInboundRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestMeshHandleOutboundRequestDelegatesToInbound(t *testing.T) {
	c := DefaultConfig()
	c.DiscoveryType = DiscoveryTypeStatic
	c.InboundPort = 0
	c.OutboundPort = 0

	m, err := NewMesh(c)
	if err != nil {
		t.Fatalf("NewMesh: %v", err)
	}

	headers := make(http.Header)
	headers.Set("X-Mesh-Service", "api")

	req := &ProxyRequest{
		Method:  "GET",
		Path:    "/health",
		Headers: headers,
	}

	resp, _ := m.handleOutboundRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestMeshCallWithRateLimit(t *testing.T) {
	c := DefaultConfig()
	c.DiscoveryType = DiscoveryTypeStatic
	c.InboundPort = 0
	c.OutboundPort = 0
	c.DefaultRateLimit = 0.001 // Very low to trigger rate limiting

	m, err := NewMesh(c)
	if err != nil {
		t.Fatalf("NewMesh: %v", err)
	}

	m.discovery.Start(context.Background())

	m.mu.Lock()
	m.started = true
	m.mu.Unlock()

	// Register service with rate-limit policy
	m.registry.SetPolicy("api", &ServicePolicy{
		RateLimitEnabled: true,
		RateLimit:        0.001,
	})

	// Create a rate limiter and exhaust its tokens
	rl := m.getOrCreateRateLimiter("api")
	// Exhaust all tokens
	for rl.Allow() {
	}

	_, err = m.Call(context.Background(), &Request{Service: "api", Method: "GET", Path: "/"})
	if err != ErrRateLimited {
		t.Errorf("expected ErrRateLimited, got %v", err)
	}

	stats := m.Stats()
	if stats.RateLimited == 0 {
		t.Error("expected RateLimited stat to be incremented")
	}
}

func TestMeshCallWithHTTPBackend(t *testing.T) {
	// Create a test HTTP server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer ts.Close()

	// Parse the test server address
	host, port, _ := net.SplitHostPort(ts.Listener.Addr().String())

	c := DefaultConfig()
	c.DiscoveryType = DiscoveryTypeStatic
	c.InboundPort = 0
	c.OutboundPort = 0
	c.EnableTLS = false
	c.DefaultRetries = 0
	c.RequestTimeout = 5 * time.Second

	m, err := NewMesh(c)
	if err != nil {
		t.Fatalf("NewMesh: %v", err)
	}

	m.discovery.Start(context.Background())

	m.mu.Lock()
	m.started = true
	m.mu.Unlock()

	// Register service with endpoint pointing to test server
	portInt := 80
	if p, err := net.LookupPort("tcp", port); err == nil {
		portInt = p
	}

	ep := &Endpoint{
		ID:      host + ":" + port,
		Address: host,
		Port:    portInt,
	}

	svc := &ServiceInstance{
		ServiceName: "test-backend",
		Namespace:   "default",
		Endpoints:   []*Endpoint{ep},
	}

	m.RegisterService(svc)
	m.healthChecker.AddEndpoint(ep)

	resp, err := m.Call(context.Background(), &Request{
		Service:   "test-backend",
		Method:    "GET",
		Path:      "/health",
		Namespace: "default",
		Headers:   make(http.Header),
		TraceID:   "trace-123",
	})
	if err != nil {
		t.Fatalf("Call error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if string(resp.Body) != "ok" {
		t.Errorf("expected body 'ok', got %q", string(resp.Body))
	}

	stats := m.Stats()
	if stats.SuccessfulRequests != 1 {
		t.Errorf("expected 1 successful request, got %d", stats.SuccessfulRequests)
	}
	if stats.BytesReceived == 0 {
		t.Error("expected BytesReceived > 0")
	}
}

func TestMeshCallWithTLSBackend(t *testing.T) {
	// Create a TLS test HTTP server
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("tls-ok"))
	}))
	defer ts.Close()

	host, port, _ := net.SplitHostPort(ts.Listener.Addr().String())

	c := DefaultConfig()
	c.DiscoveryType = DiscoveryTypeStatic
	c.InboundPort = 0
	c.OutboundPort = 0
	c.EnableTLS = true
	c.DefaultRetries = 0
	c.RequestTimeout = 5 * time.Second

	m, err := NewMesh(c)
	if err != nil {
		t.Fatalf("NewMesh: %v", err)
	}

	// Use the test server's TLS client
	m.httpClient = ts.Client()

	m.discovery.Start(context.Background())

	m.mu.Lock()
	m.started = true
	m.mu.Unlock()

	portInt := 80
	if p, err := net.LookupPort("tcp", port); err == nil {
		portInt = p
	}

	ep := &Endpoint{
		ID:      host + ":" + port,
		Address: host,
		Port:    portInt,
	}

	svc := &ServiceInstance{
		ServiceName: "tls-backend",
		Namespace:   "default",
		Endpoints:   []*Endpoint{ep},
	}
	m.RegisterService(svc)
	m.healthChecker.AddEndpoint(ep)

	resp, err := m.Call(context.Background(), &Request{
		Service:   "tls-backend",
		Method:    "GET",
		Path:      "/health",
		Namespace: "default",
		Headers:   make(http.Header),
	})
	if err != nil {
		t.Fatalf("Call error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestMeshCallTimeout(t *testing.T) {
	c := DefaultConfig()
	c.DiscoveryType = DiscoveryTypeStatic
	c.InboundPort = 0
	c.OutboundPort = 0
	c.EnableTLS = false
	c.DefaultRetries = 1
	c.RequestTimeout = 50 * time.Millisecond
	c.RetryBackoff = 10 * time.Millisecond
	c.MaxRetryBackoff = 20 * time.Millisecond

	m, err := NewMesh(c)
	if err != nil {
		t.Fatalf("NewMesh: %v", err)
	}

	m.discovery.Start(context.Background())
	m.mu.Lock()
	m.started = true
	m.mu.Unlock()

	// Register service with no actual backend
	ep := &Endpoint{ID: "bad:1", Address: "192.0.2.1", Port: 1}
	svc := &ServiceInstance{
		ServiceName: "timeout-svc",
		Namespace:   "default",
		Endpoints:   []*Endpoint{ep},
	}
	m.RegisterService(svc)
	m.healthChecker.AddEndpoint(ep)

	// Use short request timeout
	_, err = m.Call(context.Background(), &Request{
		Service:   "timeout-svc",
		Method:    "GET",
		Path:      "/test",
		Namespace: "default",
		Timeout:   50 * time.Millisecond,
		Headers:   make(http.Header),
	})
	if err == nil {
		t.Error("expected error for timeout")
	}
}

func TestMeshUpdateHTTPClient(t *testing.T) {
	c := DefaultConfig()
	c.DiscoveryType = DiscoveryTypeStatic
	c.InboundPort = 0
	c.OutboundPort = 0

	m, err := NewMesh(c)
	if err != nil {
		t.Fatalf("NewMesh: %v", err)
	}

	m.updateHTTPClient()
	if m.httpClient == nil {
		t.Error("expected non-nil HTTP client after update")
	}
}

func TestMeshOnCertificateRotation(t *testing.T) {
	c := DefaultConfig()
	c.DiscoveryType = DiscoveryTypeStatic
	c.InboundPort = 0
	c.OutboundPort = 0

	m, err := NewMesh(c)
	if err != nil {
		t.Fatalf("NewMesh: %v", err)
	}

	// Without provider
	m.onCertificateRotation(nil)

	// With provider set
	m.ConfigureMTLSProvider(nil)
	m.onCertificateRotation(&tls.Certificate{})
}

func TestMeshConfigureTLSSuccess(t *testing.T) {
	c := DefaultConfig()
	c.DiscoveryType = DiscoveryTypeStatic
	c.InboundPort = 0
	c.OutboundPort = 0

	m, err := NewMesh(c)
	if err != nil {
		t.Fatalf("NewMesh: %v", err)
	}

	cert, rootCAs := generateTestCert(t)

	err = m.ConfigureTLS(cert, rootCAs)
	if err != nil {
		t.Fatalf("ConfigureTLS error: %v", err)
	}

	if m.ServerTLSConfig() == nil {
		t.Error("expected non-nil server TLS config after ConfigureTLS")
	}
	if m.ClientTLSConfig() == nil {
		t.Error("expected non-nil client TLS config after ConfigureTLS")
	}
	if m.httpClient == nil {
		t.Error("expected non-nil HTTP client after ConfigureTLS")
	}
}

func TestMeshDiscoverEndpointsLocalAndRemote(t *testing.T) {
	c := DefaultConfig()
	c.DiscoveryType = DiscoveryTypeStatic
	c.InboundPort = 0
	c.OutboundPort = 0

	m, err := NewMesh(c)
	if err != nil {
		t.Fatalf("NewMesh: %v", err)
	}

	m.discovery.Start(context.Background())

	// Register locally
	ep := &Endpoint{ID: "local-ep", Address: "10.0.0.1", Port: 80}
	svc := &ServiceInstance{
		ServiceName: "local-svc",
		Namespace:   "default",
		Endpoints:   []*Endpoint{ep},
	}
	m.registry.Register(svc)

	// Should find via local registry
	eps, err := m.discoverEndpoints(context.Background(), "local-svc", "default")
	if err != nil {
		t.Fatalf("discoverEndpoints error: %v", err)
	}
	if len(eps) != 1 || eps[0].ID != "local-ep" {
		t.Error("expected to find local endpoint")
	}

	// Not in local registry, falls back to discovery
	_, err = m.discoverEndpoints(context.Background(), "remote-svc", "default")
	if err == nil {
		t.Error("expected error for non-existent remote service")
	}
}

func TestMeshGetServicePolicyWithCustomPolicy(t *testing.T) {
	c := DefaultConfig()
	c.DiscoveryType = DiscoveryTypeStatic
	c.InboundPort = 0
	c.OutboundPort = 0
	c.DefaultRateLimit = 50.0

	m, err := NewMesh(c)
	if err != nil {
		t.Fatalf("NewMesh: %v", err)
	}

	// Set a custom policy
	m.registry.SetPolicy("api", &ServicePolicy{MaxRetries: 10, RateLimitEnabled: true})

	p := m.getServicePolicy("api")
	if p.MaxRetries != 10 {
		t.Errorf("expected 10 retries from custom policy, got %d", p.MaxRetries)
	}

	// Default policy for unknown service
	p = m.getServicePolicy("unknown")
	if p.MaxRetries != c.DefaultRetries {
		t.Errorf("expected default retries, got %d", p.MaxRetries)
	}
	if !p.RateLimitEnabled {
		t.Error("expected rate limit enabled from default")
	}
}

// ===========================================================================
// Bulkhead - acquire with waiting queue
// ===========================================================================

func TestBulkheadAcquireWithWaiting(t *testing.T) {
	bh := NewBulkhead(&BulkheadConfig{
		MaxConcurrent: 1,
		MaxWaiting:    1,
		Timeout:       200 * time.Millisecond,
	})

	ctx := context.Background()

	// Acquire the single slot
	if err := bh.Acquire(ctx); err != nil {
		t.Fatalf("first Acquire failed: %v", err)
	}

	// Second acquire goes into wait queue
	done := make(chan error, 1)
	go func() {
		done <- bh.Acquire(ctx)
	}()

	// Release first slot to let waiter proceed
	time.Sleep(20 * time.Millisecond)
	bh.Release()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("waiting Acquire failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("waiting Acquire timed out")
	}

	bh.Release()
}

func TestBulkheadAcquireWaitQueueFull(t *testing.T) {
	bh := NewBulkhead(&BulkheadConfig{
		MaxConcurrent: 1,
		MaxWaiting:    0, // No waiting allowed
		Timeout:       100 * time.Millisecond,
	})

	ctx := context.Background()

	// Acquire the single slot
	if err := bh.Acquire(ctx); err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}

	// Second acquire should fail immediately (queue full)
	err := bh.Acquire(ctx)
	if err == nil {
		t.Error("expected error when wait queue is full")
	}

	m := bh.Metrics()
	if m.RejectedTotal == 0 {
		t.Error("expected rejected count > 0")
	}

	bh.Release()
}

func TestBulkheadAcquireContextTimeout(t *testing.T) {
	bh := NewBulkhead(&BulkheadConfig{
		MaxConcurrent: 1,
		MaxWaiting:    5,
		Timeout:       50 * time.Millisecond,
	})

	ctx := context.Background()
	bh.Acquire(ctx) // Take the slot

	// Try to acquire with a short context
	shortCtx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()

	err := bh.Acquire(shortCtx)
	if err == nil {
		t.Error("expected context deadline exceeded")
	}

	bh.Release()
}

// ===========================================================================
// Health checker Stop
// ===========================================================================

func TestHealthCheckerStopViaChannel(t *testing.T) {
	hc := NewHealthChecker(&HealthCheckConfig{
		Interval: 50 * time.Millisecond,
		Timeout:  25 * time.Millisecond,
	})

	ctx := context.Background()
	done := make(chan struct{})
	go func() {
		hc.Run(ctx)
		close(done)
	}()

	time.Sleep(30 * time.Millisecond)
	hc.Stop()

	select {
	case <-done:
		// OK
	case <-time.After(time.Second):
		t.Fatal("Run did not stop")
	}
}

// ===========================================================================
// Sliding window advance
// ===========================================================================

func TestSlidingWindowAdvanceBeyondCapacity(t *testing.T) {
	w := newSlidingWindow(100*time.Millisecond, 5)

	w.increment()
	w.increment()

	// Force time advancement beyond all buckets
	w.lastUpdate = time.Now().Add(-1 * time.Second)
	s := w.sum()
	// After advancing past all buckets, old data should be cleared
	if s != 0 {
		t.Errorf("expected 0 after advancing past all buckets, got %d", s)
	}
}

// ===========================================================================
// Rate limiter with jitter in calculateBackoff
// ===========================================================================

func TestRetrierCalculateBackoffWithJitter(t *testing.T) {
	r := NewRetrier(&RetryPolicy{
		MaxRetries:     3,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     10 * time.Second,
		Multiplier:     2.0,
		Jitter:         0.5,
	})

	b1 := r.calculateBackoff(1)
	if b1 <= 0 {
		t.Error("expected positive backoff")
	}

	// Backoff with high attempt to hit max
	b10 := r.calculateBackoff(20)
	if b10 > 15*time.Second {
		t.Errorf("backoff should be capped near max, got %v", b10)
	}
}

// ===========================================================================
// DNS Discovery cache and start/stop
// ===========================================================================

func TestDNSDiscoveryStartStop(t *testing.T) {
	d := NewDNSDiscovery(&DNSDiscoveryConfig{
		Domain:      "svc.local",
		RefreshRate: 100 * time.Millisecond,
	})

	ctx := context.Background()
	if err := d.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Double start is no-op
	if err := d.Start(ctx); err != nil {
		t.Fatalf("second Start failed: %v", err)
	}

	if err := d.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Double stop is no-op
	if err := d.Stop(); err != nil {
		t.Fatalf("second Stop failed: %v", err)
	}
}

func TestDNSDiscoveryWithCustomResolver(t *testing.T) {
	d := NewDNSDiscovery(&DNSDiscoveryConfig{
		Domain:      "svc.local",
		RefreshRate: 100 * time.Millisecond,
		Resolver:    "8.8.8.8:53",
	})
	if d.resolver == nil {
		t.Error("expected custom resolver to be set")
	}
}

func TestRegistryDiscoveryStartStop(t *testing.T) {
	r := NewRegistryDiscovery(&RegistryDiscoveryConfig{
		Endpoints: []string{"http://localhost:8080"},
	})

	ctx := context.Background()
	if err := r.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Double start
	if err := r.Start(ctx); err != nil {
		t.Fatalf("second Start failed: %v", err)
	}

	// Discover - will return service not found
	_, err := r.Discover(ctx, "svc", "ns")
	if err != ErrServiceNotFound {
		t.Errorf("expected ErrServiceNotFound, got %v", err)
	}

	// Stop
	if err := r.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if err := r.Stop(); err != nil {
		t.Fatalf("second Stop failed: %v", err)
	}
}

// ===========================================================================
// Proxy-related: InboundProxy and OutboundProxy start/stop
// ===========================================================================

func TestInboundProxyStartStop(t *testing.T) {
	p := NewInboundProxy(&InboundProxyConfig{
		Address:      "127.0.0.1",
		Port:         0, // Let OS pick
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  10 * time.Second,
	})

	if err := p.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Already started
	if err := p.Start(); err == nil {
		t.Error("expected error on double start")
	}

	stats := p.Stats()
	if stats.TotalRequests != 0 {
		t.Error("expected 0 requests")
	}

	if err := p.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Stop again is idempotent
	if err := p.Stop(); err != nil {
		t.Fatalf("second Stop failed: %v", err)
	}
}

func TestInboundProxyHTTPNoHandler(t *testing.T) {
	p := NewInboundProxy(&InboundProxyConfig{
		Address:      "127.0.0.1",
		Port:         0,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  10 * time.Second,
	})

	if err := p.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer p.Stop()

	addr := p.listener.Addr().String()
	resp, err := http.Get("http://" + addr + "/test")
	if err != nil {
		t.Fatalf("HTTP GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}

func TestInboundProxyHTTPWithHandler(t *testing.T) {
	p := NewInboundProxy(&InboundProxyConfig{
		Address:      "127.0.0.1",
		Port:         0,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  10 * time.Second,
		Handler: func(ctx context.Context, req *ProxyRequest) (*ProxyResponse, error) {
			return &ProxyResponse{
				StatusCode: http.StatusOK,
				Headers:    http.Header{"X-Test": []string{"value"}},
				Body:       []byte("hello"),
			}, nil
		},
	})

	if err := p.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer p.Stop()

	addr := p.listener.Addr().String()
	resp, err := http.Get("http://" + addr + "/test")
	if err != nil {
		t.Fatalf("HTTP GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if resp.Header.Get("X-Test") != "value" {
		t.Error("expected X-Test header")
	}
}

func TestInboundProxyHTTPHandlerError(t *testing.T) {
	p := NewInboundProxy(&InboundProxyConfig{
		Address:      "127.0.0.1",
		Port:         0,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  10 * time.Second,
		Handler: func(ctx context.Context, req *ProxyRequest) (*ProxyResponse, error) {
			return nil, ErrNoBackend
		},
	})

	if err := p.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer p.Stop()

	addr := p.listener.Addr().String()
	resp, err := http.Get("http://" + addr + "/test")
	if err != nil {
		t.Fatalf("HTTP GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", resp.StatusCode)
	}
}

func TestOutboundProxyStartStop(t *testing.T) {
	p := NewOutboundProxy(&OutboundProxyConfig{
		Address:         "127.0.0.1",
		Port:            0,
		ReadTimeout:     5 * time.Second,
		WriteTimeout:    5 * time.Second,
		IdleTimeout:     10 * time.Second,
		ReadBufferSize:  32 * 1024,
		WriteBufferSize: 32 * 1024,
	})

	if err := p.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Already started
	if err := p.Start(); err == nil {
		t.Error("expected error on double start")
	}

	stats := p.Stats()
	if stats.TotalRequests != 0 {
		t.Error("expected 0 requests")
	}

	if err := p.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if err := p.Stop(); err != nil {
		t.Fatalf("second Stop failed: %v", err)
	}
}

// ===========================================================================
// PoolManager
// ===========================================================================

func TestPoolManagerGetOrCreate(t *testing.T) {
	pm := NewPoolManager(nil, 10)

	pool1 := pm.getOrCreatePool("10.0.0.1:80")
	pool2 := pm.getOrCreatePool("10.0.0.1:80")
	if pool1 != pool2 {
		t.Error("expected same pool for same address")
	}

	stats := pm.Stats()
	if len(stats) != 1 {
		t.Errorf("expected 1 pool, got %d", len(stats))
	}

	pm.Cleanup()
	pm.Close()
}

// ===========================================================================
// sendHTTPError
// ===========================================================================

func TestSendHTTPError(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()

	go func() {
		sendHTTPError(server, http.StatusServiceUnavailable)
		server.Close()
	}()

	// Read the response
	buf := make([]byte, 4096)
	n, _ := client.Read(buf)
	if n == 0 {
		t.Error("expected some response data")
	}
}

// ===========================================================================
// Concurrent Mesh tests
// ===========================================================================

func TestMeshConcurrentBalancerAccess(t *testing.T) {
	c := DefaultConfig()
	c.DiscoveryType = DiscoveryTypeStatic
	c.InboundPort = 0
	c.OutboundPort = 0

	m, err := NewMesh(c)
	if err != nil {
		t.Fatalf("NewMesh: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			m.getOrCreateBalancer("svc")
			m.getOrCreateCircuitBreaker("svc")
			m.getOrCreateRateLimiter("svc")
		}(i)
	}
	wg.Wait()
}

// ===========================================================================
// Helper: generate self-signed cert for TLS tests
// ===========================================================================

// ===========================================================================
// Sidecar Initialize and full lifecycle tests
// ===========================================================================

func TestSidecarInitializeAndStartStop(t *testing.T) {
	config := DefaultSidecarConfig("test-svc", "test-ns")
	config.InboundPort = 0
	config.OutboundPort = 0

	s := NewSidecar(config)

	// Generate a CA cert for initialization
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		IsCA:                  true,
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

	if err := s.Initialize(caCert, caKey); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Double init is a no-op
	if err := s.Initialize(caCert, caKey); err != nil {
		t.Fatalf("second Initialize failed: %v", err)
	}

	// Start not initialized (already initialized, so Start should work)
	if err := s.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Double start
	if err := s.Start(); err != ErrAlreadyRunning {
		t.Errorf("expected ErrAlreadyRunning, got %v", err)
	}

	// Stats while running
	stats := s.Stats()
	if !stats.Running {
		t.Error("expected running=true")
	}
	if !stats.Initialized {
		t.Error("expected initialized=true")
	}

	// SPIFFEID
	spiffe := s.SPIFFEID()
	if spiffe == "" {
		t.Error("expected non-empty SPIFFE ID")
	}

	// Metrics
	m := s.Metrics()
	if m == nil {
		t.Error("expected non-nil metrics")
	}

	// Stop
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Double stop is no-op
	if err := s.Stop(); err != nil {
		t.Fatalf("second Stop failed: %v", err)
	}
}

func TestSidecarInitializeWithBundle(t *testing.T) {
	config := DefaultSidecarConfig("bundle-svc", "test-ns")
	config.InboundPort = 0
	config.OutboundPort = 0

	s := NewSidecar(config)

	// Generate a cert and key for bundle
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "bundle-test"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}

	rootCAs := x509.NewCertPool()
	rootCAs.AddCert(cert)

	if err := s.InitializeWithBundle(cert, key, rootCAs); err != nil {
		t.Fatalf("InitializeWithBundle failed: %v", err)
	}

	// Double init is a no-op
	if err := s.InitializeWithBundle(cert, key, rootCAs); err != nil {
		t.Fatalf("second InitializeWithBundle failed: %v", err)
	}
}

func TestSidecarStartNotInitialized(t *testing.T) {
	s := NewSidecar(nil)

	if err := s.Start(); err != ErrNotInitialized {
		t.Errorf("expected ErrNotInitialized, got %v", err)
	}
}

func TestSidecarInitDiscoveryBranches(t *testing.T) {
	// DNS branch
	config := DefaultSidecarConfig("dns-svc", "ns")
	config.DiscoveryType = "dns"
	config.InboundPort = 0
	config.OutboundPort = 0
	s := NewSidecar(config)
	if err := s.initDiscovery(); err != nil {
		t.Errorf("initDiscovery(dns) failed: %v", err)
	}

	// Registry branch
	config2 := DefaultSidecarConfig("reg-svc", "ns")
	config2.DiscoveryType = "registry"
	config2.RegistryURL = "http://localhost:8500"
	config2.InboundPort = 0
	config2.OutboundPort = 0
	s2 := NewSidecar(config2)
	if err := s2.initDiscovery(); err != nil {
		t.Errorf("initDiscovery(registry) failed: %v", err)
	}

	// Static branch
	config3 := DefaultSidecarConfig("static-svc", "ns")
	config3.DiscoveryType = "static"
	config3.InboundPort = 0
	config3.OutboundPort = 0
	s3 := NewSidecar(config3)
	if err := s3.initDiscovery(); err != nil {
		t.Errorf("initDiscovery(static) failed: %v", err)
	}

	// Unknown type
	config4 := DefaultSidecarConfig("unknown-svc", "ns")
	config4.DiscoveryType = "invalid"
	config4.InboundPort = 0
	config4.OutboundPort = 0
	s4 := NewSidecar(config4)
	if err := s4.initDiscovery(); err == nil {
		t.Error("expected error for unknown discovery type")
	}
}

func TestSidecarInitObservability(t *testing.T) {
	// All enabled
	config := DefaultSidecarConfig("obs-svc", "ns")
	config.MetricsEnabled = true
	config.TracingEnabled = true
	config.AccessLogEnabled = true
	s := NewSidecar(config)
	s.initObservability()

	if s.metrics == nil {
		t.Error("expected metrics to be initialized")
	}
	if s.tracer == nil {
		t.Error("expected tracer to be initialized")
	}
	if s.accessLogger == nil {
		t.Error("expected accessLogger to be initialized")
	}

	// All disabled
	config2 := DefaultSidecarConfig("no-obs-svc", "ns")
	config2.MetricsEnabled = false
	config2.TracingEnabled = false
	config2.AccessLogEnabled = false
	s2 := NewSidecar(config2)
	s2.initObservability()

	if s2.metrics != nil {
		t.Error("metrics should be nil when disabled")
	}
	if s2.tracer != nil {
		t.Error("tracer should be nil when disabled")
	}
	if s2.accessLogger != nil {
		t.Error("accessLogger should be nil when disabled")
	}
}

func TestSidecarSelectBackend(t *testing.T) {
	s := NewSidecar(nil)
	_, err := s.selectBackend(context.Background())
	if err != ErrNotInitialized {
		t.Errorf("expected ErrNotInitialized, got %v", err)
	}
}

func TestSidecarGetOrCreateEndpointPoolBranches(t *testing.T) {
	// round-robin (default)
	config := DefaultSidecarConfig("rr-svc", "ns")
	config.DefaultLB = "round-robin"
	s := NewSidecar(config)
	pool1 := s.getOrCreateEndpointPool("svc-a", "ns")
	if pool1 == nil {
		t.Fatal("expected non-nil pool")
	}

	// Same key returns same pool
	pool2 := s.getOrCreateEndpointPool("svc-a", "ns")
	if pool1 != pool2 {
		t.Error("expected same pool for same key")
	}

	// least-conn
	config2 := DefaultSidecarConfig("lc-svc", "ns")
	config2.DefaultLB = "least-conn"
	s2 := NewSidecar(config2)
	pool3 := s2.getOrCreateEndpointPool("svc-b", "ns")
	if pool3 == nil {
		t.Fatal("expected non-nil pool for least-conn")
	}

	// weighted
	config3 := DefaultSidecarConfig("w-svc", "ns")
	config3.DefaultLB = "weighted"
	s3 := NewSidecar(config3)
	pool4 := s3.getOrCreateEndpointPool("svc-c", "ns")
	if pool4 == nil {
		t.Fatal("expected non-nil pool for weighted")
	}
}

func TestSidecarConvertEndpoints(t *testing.T) {
	s := NewSidecar(nil)

	eps := s.convertEndpoints([]*discovery.Endpoint{
		{Address: "10.0.0.1", Port: 8080, Weight: 5, Healthy: true, Tags: map[string]string{"zone": "us-east"}},
		{Address: "10.0.0.2", Port: 8081, Weight: 3, Healthy: false},
	})

	if len(eps) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(eps))
	}

	if eps[0].Address != "10.0.0.1" || eps[0].Port != 8080 || eps[0].Weight != 5 || !eps[0].Healthy {
		t.Error("first endpoint conversion incorrect")
	}
	if eps[1].Address != "10.0.0.2" || eps[1].Healthy {
		t.Error("second endpoint conversion incorrect")
	}
}

func TestSidecarOnRequestAndOnResponse(t *testing.T) {
	config := DefaultSidecarConfig("req-svc", "ns")
	config.MetricsEnabled = true
	config.TracingEnabled = true
	config.AccessLogEnabled = true
	s := NewSidecar(config)
	s.initObservability()
	s.passiveHealthChecker = proxy.NewPassiveHealthChecker(nil)

	// onRequest - should not panic
	s.onRequest(&proxy.Request{
		Method: "GET",
		Path:   "/health",
	})

	// onResponse with success
	s.onResponse(&proxy.Response{
		StatusCode: 200,
		Duration:   10 * time.Millisecond,
		Backend:    "10.0.0.1:8080",
	})

	// onResponse with error
	s.onResponse(&proxy.Response{
		StatusCode: 500,
		Duration:   50 * time.Millisecond,
		Backend:    "10.0.0.2:8080",
		Error:      context.DeadlineExceeded,
	})

	// onResponse without backend (no passive health update)
	s.onResponse(&proxy.Response{
		StatusCode: 200,
		Duration:   5 * time.Millisecond,
	})

	// onRequest/onResponse without observability
	s2 := NewSidecar(nil)
	s2.onRequest(&proxy.Request{Method: "GET"})
	s2.onResponse(&proxy.Response{StatusCode: 200})
}

func TestSidecarAddStaticEndpoint(t *testing.T) {
	config := DefaultSidecarConfig("static-ep-svc", "ns")
	config.DiscoveryType = "static"
	s := NewSidecar(config)
	s.initDiscovery()

	s.AddStaticEndpoint("api", "default", "10.0.0.1", 8080)

	// Verify it was added by discovering
	svc, err := s.discoverer.Discover(context.Background(), "api", "default")
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}
	if len(svc.Endpoints) != 1 {
		t.Errorf("expected 1 endpoint, got %d", len(svc.Endpoints))
	}
}

func TestSidecarAddStaticEndpointNonStatic(t *testing.T) {
	config := DefaultSidecarConfig("non-static-svc", "ns")
	config.DiscoveryType = "dns"
	s := NewSidecar(config)
	s.initDiscovery()

	// Should be a no-op because discoverer is not StaticDiscoverer
	s.AddStaticEndpoint("api", "default", "10.0.0.1", 8080)
}

func TestSidecarSPIFFEIDWithoutMTLS(t *testing.T) {
	s := NewSidecar(nil)
	if s.SPIFFEID() != "" {
		t.Error("expected empty SPIFFE ID without mTLS manager")
	}
}

func TestSidecarStatsWithComponents(t *testing.T) {
	config := DefaultSidecarConfig("full-stats-svc", "ns")
	config.InboundPort = 0
	config.OutboundPort = 0

	s := NewSidecar(config)

	// Generate CA cert
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "stats-ca"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}

	caCertDER, _ := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	caCert, _ := x509.ParseCertificate(caCertDER)

	if err := s.Initialize(caCert, caKey); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// Record some metrics so stats have data
	s.metrics.RecordRequest("test", "ns", 200, 10*time.Millisecond, nil)

	// Get circuit breaker for a service to populate circuit breaker stats
	s.circuitBreakers.Get("svc-a")

	stats := s.Stats()
	if stats.CircuitBreakerStats == nil {
		t.Error("expected non-nil circuit breaker stats")
	}
	if stats.SuccessRate == 0 {
		t.Error("expected non-zero success rate after recording a request")
	}
}

func TestSidecarSelectBackendForService(t *testing.T) {
	config := DefaultSidecarConfig("select-svc", "ns")
	config.DiscoveryType = "static"
	config.InboundPort = 0
	config.OutboundPort = 0

	s := NewSidecar(config)

	// Generate CA cert
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "select-ca"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}

	caCertDER, _ := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	caCert, _ := x509.ParseCertificate(caCertDER)

	if err := s.Initialize(caCert, caKey); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// Add static endpoint
	s.AddStaticEndpoint("backend", "default", "10.0.0.1", 8080)

	addr, err := s.SelectBackendForService(context.Background(), "backend", "default")
	if err != nil {
		t.Fatalf("SelectBackendForService: %v", err)
	}
	if addr == "" {
		t.Error("expected non-empty backend address")
	}
}

func TestSidecarSelectBackendForServiceNotFound(t *testing.T) {
	config := DefaultSidecarConfig("miss-svc", "ns")
	config.DiscoveryType = "static"
	config.InboundPort = 0
	config.OutboundPort = 0

	s := NewSidecar(config)
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "miss-ca"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	caCertDER, _ := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	caCert, _ := x509.ParseCertificate(caCertDER)

	if err := s.Initialize(caCert, caKey); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	_, err := s.SelectBackendForService(context.Background(), "nonexistent", "default")
	if err == nil {
		t.Error("expected error for non-existent service")
	}
}

// ===========================================================================
// ConnectionPool Get/Put with real TCP connections
// ===========================================================================

func TestConnectionPoolGetPutWithListener(t *testing.T) {
	// Start a local listener to connect to
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()

	// Accept connections in background
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	pool := NewConnectionPool(ln.Addr().String(), &ConnectionPoolConfig{
		MaxSize:     5,
		MinSize:     1,
		MaxLifetime: 30 * time.Minute,
		MaxIdleTime: 90 * time.Second,
		DialTimeout: 5 * time.Second,
	})
	defer pool.Close()

	// Get a connection
	conn, err := pool.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	// Put it back
	pool.Put(conn)

	// Get again - should hit pool
	conn2, err := pool.Get(context.Background())
	if err != nil {
		t.Fatalf("Get after Put: %v", err)
	}
	pool.Put(conn2)

	// Put a non-pooled conn (should just close it)
	server, client := net.Pipe()
	go func() { server.Close() }()
	pool.Put(client)

	// Verify stats
	stats := pool.Stats()
	if stats.TotalConns == 0 {
		t.Error("expected TotalConns > 0")
	}

	hitRate := stats.HitRate()
	_ = hitRate // just exercise the path

	size := pool.Size()
	_ = size
}

func TestConnectionPoolGetClosed(t *testing.T) {
	pool := NewConnectionPool("127.0.0.1:1", nil)
	pool.Close()

	_, err := pool.Get(context.Background())
	if err != ErrProxyClosed {
		t.Errorf("expected ErrProxyClosed, got %v", err)
	}
}

func TestConnectionPoolPutClosed(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	pool := NewConnectionPool(ln.Addr().String(), nil)
	conn, err := pool.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	pool.Close()

	// Put after close should just close the conn
	pool2 := NewConnectionPool(ln.Addr().String(), nil)
	conn2, err := pool2.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	pool2.Close()
	// Put to a closed pool
	pool2.Put(conn2)
	_ = conn
}

func TestConnectionPoolCleanup(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			// Keep connections alive
			go func() {
				buf := make([]byte, 1)
				conn.Read(buf)
				conn.Close()
			}()
		}
	}()

	pool := NewConnectionPool(ln.Addr().String(), &ConnectionPoolConfig{
		MaxSize:     5,
		MinSize:     0,
		MaxLifetime: 30 * time.Minute,
		MaxIdleTime: 1 * time.Millisecond, // Very short idle time
		DialTimeout: 5 * time.Second,
	})
	defer pool.Close()

	// Get and put to add a connection to the pool
	conn, err := pool.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	pool.Put(conn)

	// Wait for idle time to expire
	time.Sleep(5 * time.Millisecond)

	// Cleanup should remove the idle connection
	pool.Cleanup()
}

// ===========================================================================
// TransparentProxy Start/Stop
// ===========================================================================

func TestTransparentProxyStartStop(t *testing.T) {
	tp := NewTransparentProxy(&TransparentProxyConfig{
		Address:        "127.0.0.1",
		Port:           0,
		ConnectTimeout: 5 * time.Second,
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   5 * time.Second,
		BufferSize:     32 * 1024,
	})

	if err := tp.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Double start
	if err := tp.Start(); err == nil {
		t.Error("expected error on double start")
	}

	stats := tp.Stats()
	if stats.TotalConnections != 0 {
		t.Error("expected 0 connections")
	}

	if err := tp.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Double stop
	if err := tp.Stop(); err != nil {
		t.Fatalf("second Stop failed: %v", err)
	}
}

func TestTransparentProxyNilConfig(t *testing.T) {
	tp := NewTransparentProxy(nil)
	if tp.config == nil {
		t.Error("expected default config when nil is passed")
	}
}

// ===========================================================================
// OutboundProxy proxyHTTP via actual TCP connection
// ===========================================================================

func TestOutboundProxyHTTPProxying(t *testing.T) {
	p := NewOutboundProxy(&OutboundProxyConfig{
		Address:         "127.0.0.1",
		Port:            0,
		ReadTimeout:     5 * time.Second,
		WriteTimeout:    5 * time.Second,
		IdleTimeout:     10 * time.Second,
		ReadBufferSize:  32 * 1024,
		WriteBufferSize: 32 * 1024,
		Handler: func(ctx context.Context, req *ProxyRequest) (*ProxyResponse, error) {
			return &ProxyResponse{
				StatusCode: http.StatusOK,
				Headers:    http.Header{"Content-Type": []string{"text/plain"}},
				Body:       []byte("proxied"),
			}, nil
		},
	})

	if err := p.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer p.Stop()

	addr := p.listener.Addr().String()

	// Send an HTTP request directly via TCP
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(5 * time.Second))
	_, err = conn.Write([]byte("GET /test HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	buf := make([]byte, 4096)
	n, _ := conn.Read(buf)
	if n == 0 {
		t.Error("expected response data")
	}
}

func TestOutboundProxyHTTPNoHandler(t *testing.T) {
	p := NewOutboundProxy(&OutboundProxyConfig{
		Address:         "127.0.0.1",
		Port:            0,
		ReadTimeout:     5 * time.Second,
		WriteTimeout:    5 * time.Second,
		IdleTimeout:     10 * time.Second,
		ReadBufferSize:  32 * 1024,
		WriteBufferSize: 32 * 1024,
		// No Handler
	})

	if err := p.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer p.Stop()

	addr := p.listener.Addr().String()

	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(5 * time.Second))
	conn.Write([]byte("GET /test HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"))

	buf := make([]byte, 4096)
	n, _ := conn.Read(buf)
	if n == 0 {
		t.Error("expected error response data")
	}
}

func TestOutboundProxyL4Connection(t *testing.T) {
	p := NewOutboundProxy(&OutboundProxyConfig{
		Address:         "127.0.0.1",
		Port:            0,
		ReadTimeout:     1 * time.Second,
		WriteTimeout:    1 * time.Second,
		IdleTimeout:     2 * time.Second,
		ReadBufferSize:  32 * 1024,
		WriteBufferSize: 32 * 1024,
	})

	if err := p.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer p.Stop()

	addr := p.listener.Addr().String()

	// Send non-HTTP data (L4)
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	conn.SetDeadline(time.Now().Add(time.Second))
	conn.Write([]byte{0x00, 0x01, 0x02, 0x03}) // non-HTTP bytes
	conn.Close()

	// Give the proxy time to handle it
	time.Sleep(50 * time.Millisecond)
}

// ===========================================================================
// PoolManager Get and Put with real connections
// ===========================================================================

func TestPoolManagerGetPutWithListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	pm := NewPoolManager(&ConnectionPoolConfig{
		MaxSize:     5,
		MinSize:     1,
		MaxLifetime: 30 * time.Minute,
		MaxIdleTime: 90 * time.Second,
		DialTimeout: 5 * time.Second,
	}, 10)
	defer pm.Close()

	// Get
	conn, err := pm.Get(context.Background(), ln.Addr().String())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	// Put
	pm.Put(conn)

	// Put a non-pooled conn
	server, client := net.Pipe()
	go func() { server.Close() }()
	pm.Put(client)
}

// ===========================================================================
// Discovery: CachingDiscovery Start/Stop/Watch, KubernetesDiscovery, RegistryWatch
// ===========================================================================

func TestCachingDiscoveryStartStop(t *testing.T) {
	static := NewStaticDiscovery()
	cd := NewCachingDiscovery(static, 100*time.Millisecond)

	ctx := context.Background()
	if err := cd.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if err := cd.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestCachingDiscoveryWatch(t *testing.T) {
	static := NewStaticDiscovery()
	static.Start(context.Background())
	defer static.Stop()

	static.AddService("watch-svc", "default", []*Endpoint{
		{ID: "ep1", Address: "10.0.0.1", Port: 80},
	})

	cd := NewCachingDiscovery(static, 100*time.Millisecond)
	cd.Start(context.Background())
	defer cd.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	ch, err := cd.Watch(ctx, "watch-svc", "default")
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	// Should receive initial endpoints
	select {
	case eps := <-ch:
		if len(eps) != 1 {
			t.Errorf("expected 1 endpoint, got %d", len(eps))
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("timed out waiting for Watch update")
	}
}

func TestKubernetesDiscoveryStartStop(t *testing.T) {
	k := NewKubernetesDiscovery(&KubernetesDiscoveryConfig{
		Namespace:   "default",
		RefreshRate: 100 * time.Millisecond,
	})

	ctx := context.Background()
	if err := k.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Double start
	if err := k.Start(ctx); err != nil {
		t.Fatalf("second Start failed: %v", err)
	}

	// Stop
	if err := k.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Double stop
	if err := k.Stop(); err != nil {
		t.Fatalf("second Stop failed: %v", err)
	}
}

func TestRegistryDiscoveryWatch(t *testing.T) {
	r := NewRegistryDiscovery(&RegistryDiscoveryConfig{
		Endpoints:   []string{"http://localhost:8080"},
		RefreshRate: 50 * time.Millisecond,
	})

	ctx := context.Background()
	r.Start(ctx)
	defer r.Stop()

	watchCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	ch, err := r.Watch(watchCtx, "svc", "ns")
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	// Wait for context cancellation (Watch goroutine should stop)
	<-watchCtx.Done()
	time.Sleep(20 * time.Millisecond)

	// Channel should eventually close
	select {
	case _, ok := <-ch:
		if ok {
			// Received data, that's fine
		}
	case <-time.After(500 * time.Millisecond):
		// Watch goroutine may still be cleaning up
	}
}

func TestRegistryDiscoveryWatchNotStarted(t *testing.T) {
	r := NewRegistryDiscovery(&RegistryDiscoveryConfig{
		Endpoints: []string{"http://localhost:8080"},
	})

	_, err := r.Watch(context.Background(), "svc", "ns")
	if err != ErrDiscoveryNotStarted {
		t.Errorf("expected ErrDiscoveryNotStarted, got %v", err)
	}
}

func TestCompositeDiscoveryStart(t *testing.T) {
	s1 := NewStaticDiscovery()
	s2 := NewStaticDiscovery()

	cd := NewCompositeDiscovery(s1, s2)

	ctx := context.Background()
	if err := cd.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Double start
	if err := cd.Start(ctx); err != nil {
		t.Fatalf("second Start should be no-op, got: %v", err)
	}

	if err := cd.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

// ===========================================================================
// HealthChecker RemoveService
// ===========================================================================

func TestHealthCheckerRemoveServiceNoop(t *testing.T) {
	hc := NewHealthChecker(&HealthCheckConfig{
		Interval: 50 * time.Millisecond,
		Timeout:  25 * time.Millisecond,
	})

	// Add an endpoint
	hc.AddEndpoint(&Endpoint{ID: "ep1", Address: "10.0.0.1", Port: 80})

	// RemoveService is a placeholder noop - just verify it doesn't panic
	hc.RemoveService("any-service")
}

// ===========================================================================
// DNS Discovery Watch and Discover (via cache)
// ===========================================================================

func TestDNSDiscoveryWatch(t *testing.T) {
	d := NewDNSDiscovery(&DNSDiscoveryConfig{
		Domain:      "localhost",
		RefreshRate: 50 * time.Millisecond,
	})

	ctx := context.Background()
	d.Start(ctx)
	defer d.Stop()

	watchCtx, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	defer cancel()

	ch, err := d.Watch(watchCtx, "test", "default")
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	// Watch should eventually return or context times out
	select {
	case <-ch:
		// Received endpoints or channel closed
	case <-time.After(500 * time.Millisecond):
		// Acceptable - DNS resolution may fail
	}
}

func TestDNSDiscoveryWatchNotStarted(t *testing.T) {
	d := NewDNSDiscovery(&DNSDiscoveryConfig{
		Domain:      "localhost",
		RefreshRate: 50 * time.Millisecond,
	})

	_, err := d.Watch(context.Background(), "test", "default")
	if err != ErrDiscoveryNotStarted {
		t.Errorf("expected ErrDiscoveryNotStarted, got %v", err)
	}
}

// ===========================================================================
// Concurrent Sidecar access
// ===========================================================================

func TestSidecarConcurrentEndpointPoolAccess(t *testing.T) {
	s := NewSidecar(nil)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s.getOrCreateEndpointPool("svc", "ns")
		}(i)
	}
	wg.Wait()
}

func generateTestCert(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IsCA:         true,
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}

	pool := x509.NewCertPool()
	pool.AddCert(cert)

	tlsCert := tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
		Leaf:        cert,
	}

	return tlsCert, pool
}
