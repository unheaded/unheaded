// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package mesh provides THE HAUBERK - the service mesh implementation for the Unheaded Kingdom.
// It provides service discovery, load balancing, circuit breaking, retry logic,
// timeout handling, mTLS, traffic shaping, and health-aware routing.
package mesh

import (
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"unheaded/pkg/certs"
	"unheaded/pkg/mesh/mtls"
)

// Version is the mesh version.
const Version = "1.0.0"

var (
	// ErrMeshClosed indicates the mesh has been closed.
	ErrMeshClosed = errors.New("mesh closed")
	// ErrMeshNotStarted indicates the mesh has not been started.
	ErrMeshNotStarted = errors.New("mesh not started")
	// ErrServiceNotFound indicates a service was not found.
	ErrServiceNotFound = errors.New("service not found")
	// ErrNoHealthyBackends indicates no healthy backends are available.
	ErrNoHealthyBackends = errors.New("no healthy backends available")
	// ErrCircuitOpen indicates the circuit breaker is open.
	ErrCircuitOpen = errors.New("circuit breaker open")
	// ErrTimeout indicates a request timed out.
	ErrTimeout = errors.New("request timed out")
	// ErrRetryExhausted indicates all retries have been exhausted.
	ErrRetryExhausted = errors.New("all retries exhausted")
	// ErrRateLimited indicates the request was rate limited.
	ErrRateLimited = errors.New("rate limited")
)

// Mesh is the core service mesh manager.
// It coordinates all mesh components: discovery, load balancing, circuit breaking,
// retries, timeouts, mTLS, and traffic shaping.
type Mesh struct {
	config *Config

	// Service registry and discovery
	registry  *ServiceRegistry
	discovery Discovery

	// Load balancing
	balancers   map[string]LoadBalancer
	balancersMu sync.RWMutex

	// Circuit breakers
	circuits   map[string]*CircuitBreaker
	circuitsMu sync.RWMutex

	// Rate limiters
	rateLimiters   map[string]*RateLimiter
	rateLimitersMu sync.RWMutex

	// Health checking
	healthChecker *HealthChecker

	// mTLS
	tlsConfig       *tls.Config
	clientTLSConfig *tls.Config
	rootCAs         *x509.CertPool

	// mTLS Provider (zero-trust networking)
	mtlsProvider    *mtls.Provider
	identityMgr     *mtls.IdentityManager
	zeroTrustEnforcer *mtls.ZeroTrustEnforcer

	// HTTP client for outbound requests
	httpClient *http.Client

	// Connection pools
	connPools   map[string]*ConnectionPool
	connPoolsMu sync.RWMutex

	// Proxy components
	inboundProxy  *InboundProxy
	outboundProxy *OutboundProxy

	// State
	mu      sync.RWMutex
	started bool
	closed  bool

	// Shutdown
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	closedCh chan struct{}

	// Metrics
	stats *MeshStats
}

// MeshStats holds mesh-wide statistics.
type MeshStats struct {
	TotalRequests       uint64
	SuccessfulRequests  uint64
	FailedRequests      uint64
	CircuitBreakerTrips uint64
	Retries             uint64
	Timeouts            uint64
	RateLimited         uint64
	BytesSent           uint64
	BytesReceived       uint64
	ActiveConnections   int64
	TotalLatencyNs      uint64
	RequestCount        uint64
}

// NewMesh creates a new service mesh with the given configuration.
func NewMesh(config *Config) (*Mesh, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	m := &Mesh{
		config:       config,
		registry:     NewServiceRegistry(),
		balancers:    make(map[string]LoadBalancer),
		circuits:     make(map[string]*CircuitBreaker),
		rateLimiters: make(map[string]*RateLimiter),
		connPools:    make(map[string]*ConnectionPool),
		ctx:          ctx,
		cancel:       cancel,
		closedCh:     make(chan struct{}),
		stats:        &MeshStats{},
	}

	// Initialize discovery based on config
	if err := m.initDiscovery(); err != nil {
		cancel()
		return nil, fmt.Errorf("discovery init failed: %w", err)
	}

	// Initialize health checker
	m.healthChecker = NewHealthChecker(&HealthCheckConfig{
		Interval:            config.HealthCheckInterval,
		Timeout:             config.HealthCheckTimeout,
		HealthyThreshold:    config.HealthyThreshold,
		UnhealthyThreshold:  config.UnhealthyThreshold,
		OnHealthChange:      m.onEndpointHealthChange,
	})

	return m, nil
}

// initDiscovery initializes the discovery backend.
func (m *Mesh) initDiscovery() error {
	switch m.config.DiscoveryType {
	case DiscoveryTypeDNS:
		m.discovery = NewDNSDiscovery(&DNSDiscoveryConfig{
			Domain:      m.config.DNSDomain,
			RefreshRate: m.config.DiscoveryRefreshRate,
			Resolver:    m.config.DNSResolver,
		})
	case DiscoveryTypeRegistry:
		m.discovery = NewRegistryDiscovery(&RegistryDiscoveryConfig{
			Endpoints:   m.config.RegistryEndpoints,
			RefreshRate: m.config.DiscoveryRefreshRate,
		})
	case DiscoveryTypeStatic:
		m.discovery = NewStaticDiscovery()
	case DiscoveryTypeKubernetes:
		m.discovery = NewKubernetesDiscovery(&KubernetesDiscoveryConfig{
			Namespace:   m.config.Namespace,
			RefreshRate: m.config.DiscoveryRefreshRate,
		})
	default:
		return fmt.Errorf("unknown discovery type: %s", m.config.DiscoveryType)
	}
	return nil
}

// ConfigureTLS configures mTLS for the mesh.
func (m *Mesh) ConfigureTLS(cert tls.Certificate, rootCAs *x509.CertPool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return ErrMeshClosed
	}

	// Server TLS config (for inbound connections)
	m.tlsConfig = &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    rootCAs,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		},
	}

	// Client TLS config (for outbound connections)
	m.clientTLSConfig = &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      rootCAs,
		MinVersion:   tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		},
	}

	m.rootCAs = rootCAs

	// Create HTTP client with mTLS
	m.httpClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig:     m.clientTLSConfig,
			MaxIdleConns:        m.config.MaxIdleConns,
			MaxIdleConnsPerHost: m.config.MaxIdleConnsPerHost,
			IdleConnTimeout:     m.config.IdleConnTimeout,
			DialContext: (&net.Dialer{
				Timeout:   m.config.ConnectTimeout,
				KeepAlive: m.config.KeepAliveInterval,
			}).DialContext,
		},
		Timeout: m.config.RequestTimeout,
	}

	return nil
}

// Start starts the mesh.
func (m *Mesh) Start() error {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return errors.New("mesh already started")
	}
	if m.closed {
		m.mu.Unlock()
		return ErrMeshClosed
	}
	m.started = true
	m.mu.Unlock()

	// Start discovery
	if err := m.discovery.Start(m.ctx); err != nil {
		return fmt.Errorf("discovery start failed: %w", err)
	}

	// Start health checker
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.healthChecker.Run(m.ctx)
	}()

	// Start inbound proxy if configured
	if m.config.InboundPort > 0 {
		m.inboundProxy = NewInboundProxy(&InboundProxyConfig{
			Address:   m.config.InboundAddress,
			Port:      m.config.InboundPort,
			TLSConfig: m.tlsConfig,
			Handler:   m.handleInboundRequest,
		})
		if err := m.inboundProxy.Start(); err != nil {
			return fmt.Errorf("inbound proxy start failed: %w", err)
		}
	}

	// Start outbound proxy if configured
	if m.config.OutboundPort > 0 {
		m.outboundProxy = NewOutboundProxy(&OutboundProxyConfig{
			Address:   m.config.OutboundAddress,
			Port:      m.config.OutboundPort,
			TLSConfig: m.clientTLSConfig,
			Handler:   m.handleOutboundRequest,
		})
		if err := m.outboundProxy.Start(); err != nil {
			return fmt.Errorf("outbound proxy start failed: %w", err)
		}
	}

	// Start background goroutines
	m.wg.Add(1)
	go m.discoverySync()

	m.wg.Add(1)
	go m.connectionPoolMaintenance()

	m.wg.Add(1)
	go m.circuitBreakerMaintenance()

	return nil
}

// Stop stops the mesh gracefully.
func (m *Mesh) Stop() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	m.started = false
	m.mu.Unlock()

	// Signal shutdown
	m.cancel()
	close(m.closedCh)

	// Stop proxies
	if m.inboundProxy != nil {
		m.inboundProxy.Stop()
	}
	if m.outboundProxy != nil {
		m.outboundProxy.Stop()
	}

	// Stop discovery
	if m.discovery != nil {
		m.discovery.Stop()
	}

	// Close connection pools
	m.connPoolsMu.Lock()
	for _, pool := range m.connPools {
		pool.Close()
	}
	m.connPoolsMu.Unlock()

	// Wait for goroutines
	m.wg.Wait()

	return nil
}

// RegisterService registers a local service with the mesh.
func (m *Mesh) RegisterService(svc *ServiceInstance) error {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return ErrMeshClosed
	}
	m.mu.RUnlock()

	m.registry.Register(svc)

	// Create load balancer for this service
	m.getOrCreateBalancer(svc.ServiceName)

	// Create circuit breaker
	m.getOrCreateCircuitBreaker(svc.ServiceName)

	// Create rate limiter if configured
	if m.config.DefaultRateLimit > 0 {
		m.getOrCreateRateLimiter(svc.ServiceName)
	}

	return nil
}

// DeregisterService removes a service from the mesh.
func (m *Mesh) DeregisterService(serviceName string) error {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return ErrMeshClosed
	}
	m.mu.RUnlock()

	m.registry.Deregister(serviceName)

	// Remove health checks for this service's endpoints
	m.healthChecker.RemoveService(serviceName)

	return nil
}

// Call makes a request to a service with full mesh features.
// It handles discovery, load balancing, circuit breaking, retries, and timeouts.
func (m *Mesh) Call(ctx context.Context, req *Request) (*Response, error) {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return nil, ErrMeshClosed
	}
	if !m.started {
		m.mu.RUnlock()
		return nil, ErrMeshNotStarted
	}
	m.mu.RUnlock()

	atomic.AddUint64(&m.stats.TotalRequests, 1)
	startTime := time.Now()

	// Apply timeout from context or config
	timeout := m.config.RequestTimeout
	if req.Timeout > 0 {
		timeout = req.Timeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Get service policy
	policy := m.getServicePolicy(req.Service)

	// Check rate limit
	if policy.RateLimitEnabled {
		limiter := m.getOrCreateRateLimiter(req.Service)
		if !limiter.Allow() {
			atomic.AddUint64(&m.stats.RateLimited, 1)
			atomic.AddUint64(&m.stats.FailedRequests, 1)
			return nil, ErrRateLimited
		}
	}

	// Execute with retry logic
	var lastErr error
	maxRetries := policy.MaxRetries
	if maxRetries == 0 {
		maxRetries = m.config.DefaultRetries
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			atomic.AddUint64(&m.stats.Retries, 1)
			// Exponential backoff
			backoff := m.config.RetryBackoff * time.Duration(1<<uint(attempt-1))
			if backoff > m.config.MaxRetryBackoff {
				backoff = m.config.MaxRetryBackoff
			}
			select {
			case <-ctx.Done():
				atomic.AddUint64(&m.stats.Timeouts, 1)
				atomic.AddUint64(&m.stats.FailedRequests, 1)
				return nil, ErrTimeout
			case <-time.After(backoff):
			}
		}

		resp, err := m.doCall(ctx, req)
		if err == nil {
			atomic.AddUint64(&m.stats.SuccessfulRequests, 1)
			latency := time.Since(startTime).Nanoseconds()
			atomic.AddUint64(&m.stats.TotalLatencyNs, uint64(latency))
			atomic.AddUint64(&m.stats.RequestCount, 1)
			return resp, nil
		}

		lastErr = err

		// Check if we should retry
		if !m.shouldRetry(err, attempt, maxRetries) {
			break
		}
	}

	atomic.AddUint64(&m.stats.FailedRequests, 1)
	if lastErr == ErrCircuitOpen {
		atomic.AddUint64(&m.stats.CircuitBreakerTrips, 1)
	}

	return nil, lastErr
}

// doCall performs a single call attempt.
func (m *Mesh) doCall(ctx context.Context, req *Request) (*Response, error) {
	// Check circuit breaker
	cb := m.getOrCreateCircuitBreaker(req.Service)
	if !cb.Allow() {
		return nil, ErrCircuitOpen
	}

	// Discover endpoints
	endpoints, err := m.discoverEndpoints(ctx, req.Service, req.Namespace)
	if err != nil {
		cb.RecordFailure()
		return nil, err
	}

	// Filter healthy endpoints
	healthyEndpoints := m.filterHealthyEndpoints(endpoints)
	if len(healthyEndpoints) == 0 {
		cb.RecordFailure()
		return nil, ErrNoHealthyBackends
	}

	// Select endpoint using load balancer
	lb := m.getOrCreateBalancer(req.Service)
	endpoint := lb.Select(healthyEndpoints)
	if endpoint == nil {
		cb.RecordFailure()
		return nil, ErrNoHealthyBackends
	}

	// Make the actual call
	resp, err := m.doRequest(ctx, endpoint, req)
	if err != nil {
		cb.RecordFailure()
		// Record passive health check failure
		m.healthChecker.RecordFailure(endpoint.ID)
		return nil, err
	}

	// Record success
	cb.RecordSuccess()
	m.healthChecker.RecordSuccess(endpoint.ID)

	return resp, nil
}

// doRequest performs the actual HTTP/gRPC request.
func (m *Mesh) doRequest(ctx context.Context, endpoint *Endpoint, req *Request) (*Response, error) {
	// Build URL
	address := fmt.Sprintf("%s:%d", endpoint.Address, endpoint.Port)
	var url string
	if m.config.EnableTLS {
		url = fmt.Sprintf("https://%s%s", address, req.Path)
	} else {
		url = fmt.Sprintf("http://%s%s", address, req.Path)
	}

	// Create HTTP request
	var body io.Reader
	if req.Body != nil {
		body = req.Body
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, url, body)
	if err != nil {
		return nil, err
	}

	// Copy headers
	for k, v := range req.Headers {
		for _, vv := range v {
			httpReq.Header.Add(k, vv)
		}
	}

	// Add mesh headers
	httpReq.Header.Set("X-Mesh-Service", req.Service)
	httpReq.Header.Set("X-Mesh-Namespace", req.Namespace)
	if req.TraceID != "" {
		httpReq.Header.Set("X-Trace-ID", req.TraceID)
	}

	// Get or create HTTP client
	client := m.httpClient
	if client == nil {
		client = http.DefaultClient
	}

	// Execute request
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, err
	}

	atomic.AddUint64(&m.stats.BytesReceived, uint64(len(respBody)))

	return &Response{
		StatusCode: httpResp.StatusCode,
		Headers:    httpResp.Header,
		Body:       respBody,
		Endpoint:   endpoint,
	}, nil
}

// discoverEndpoints discovers endpoints for a service.
func (m *Mesh) discoverEndpoints(ctx context.Context, service, namespace string) ([]*Endpoint, error) {
	// Try local registry first
	if svc, ok := m.registry.Get(service); ok {
		return svc.Endpoints, nil
	}

	// Use discovery backend
	return m.discovery.Discover(ctx, service, namespace)
}

// filterHealthyEndpoints filters out unhealthy endpoints.
func (m *Mesh) filterHealthyEndpoints(endpoints []*Endpoint) []*Endpoint {
	healthy := make([]*Endpoint, 0, len(endpoints))
	for _, ep := range endpoints {
		if m.healthChecker.IsHealthy(ep.ID) {
			healthy = append(healthy, ep)
		}
	}
	return healthy
}

// getServicePolicy returns the policy for a service.
func (m *Mesh) getServicePolicy(service string) *ServicePolicy {
	// Check for service-specific policy
	if policy, ok := m.registry.GetPolicy(service); ok {
		return policy
	}
	// Return default policy
	return &ServicePolicy{
		MaxRetries:       m.config.DefaultRetries,
		Timeout:          m.config.RequestTimeout,
		RateLimitEnabled: m.config.DefaultRateLimit > 0,
		RateLimit:        m.config.DefaultRateLimit,
	}
}

// shouldRetry determines if a request should be retried.
func (m *Mesh) shouldRetry(err error, attempt, maxRetries int) bool {
	if attempt >= maxRetries {
		return false
	}

	// Don't retry circuit breaker errors
	if errors.Is(err, ErrCircuitOpen) {
		return false
	}

	// Don't retry rate limiting
	if errors.Is(err, ErrRateLimited) {
		return false
	}

	// Don't retry context cancellation
	if errors.Is(err, context.Canceled) {
		return false
	}

	// Retry network errors and timeouts
	return true
}

// getOrCreateBalancer gets or creates a load balancer for a service.
func (m *Mesh) getOrCreateBalancer(service string) LoadBalancer {
	m.balancersMu.RLock()
	lb, ok := m.balancers[service]
	m.balancersMu.RUnlock()

	if ok {
		return lb
	}

	m.balancersMu.Lock()
	defer m.balancersMu.Unlock()

	// Double-check
	if lb, ok := m.balancers[service]; ok {
		return lb
	}

	// Create new balancer based on config
	switch m.config.LoadBalancerType {
	case LoadBalancerLeastConn:
		lb = NewLeastConnBalancer()
	case LoadBalancerWeighted:
		lb = NewWeightedBalancer()
	case LoadBalancerRandom:
		lb = NewRandomBalancer()
	case LoadBalancerIPHash:
		lb = NewIPHashBalancer()
	default:
		lb = NewRoundRobinBalancer()
	}

	m.balancers[service] = lb
	return lb
}

// getOrCreateCircuitBreaker gets or creates a circuit breaker for a service.
func (m *Mesh) getOrCreateCircuitBreaker(service string) *CircuitBreaker {
	m.circuitsMu.RLock()
	cb, ok := m.circuits[service]
	m.circuitsMu.RUnlock()

	if ok {
		return cb
	}

	m.circuitsMu.Lock()
	defer m.circuitsMu.Unlock()

	// Double-check
	if cb, ok := m.circuits[service]; ok {
		return cb
	}

	cb = NewCircuitBreaker(&CircuitBreakerConfig{
		Name:                service,
		FailureThreshold:    m.config.CircuitBreakerFailureThreshold,
		SuccessThreshold:    m.config.CircuitBreakerSuccessThreshold,
		Timeout:             m.config.CircuitBreakerTimeout,
		HalfOpenMaxRequests: m.config.CircuitBreakerHalfOpenRequests,
		OnStateChange:       m.onCircuitStateChange,
	})

	m.circuits[service] = cb
	return cb
}

// getOrCreateRateLimiter gets or creates a rate limiter for a service.
func (m *Mesh) getOrCreateRateLimiter(service string) *RateLimiter {
	m.rateLimitersMu.RLock()
	rl, ok := m.rateLimiters[service]
	m.rateLimitersMu.RUnlock()

	if ok {
		return rl
	}

	m.rateLimitersMu.Lock()
	defer m.rateLimitersMu.Unlock()

	// Double-check
	if rl, ok := m.rateLimiters[service]; ok {
		return rl
	}

	rl = NewRateLimiter(&RateLimiterConfig{
		Rate:      m.config.DefaultRateLimit,
		Burst:     m.config.DefaultRateBurst,
		Algorithm: m.config.RateLimitAlgorithm,
	})

	m.rateLimiters[service] = rl
	return rl
}

// onCircuitStateChange handles circuit breaker state changes.
func (m *Mesh) onCircuitStateChange(name string, from, to CircuitState) {
	// Log or emit metrics
	if to == CircuitStateOpen {
		atomic.AddUint64(&m.stats.CircuitBreakerTrips, 1)
	}
}

// onEndpointHealthChange handles endpoint health changes.
func (m *Mesh) onEndpointHealthChange(endpointID string, healthy bool) {
	// Could emit metrics or trigger rebalancing
}

// discoverySync synchronizes service discovery.
func (m *Mesh) discoverySync() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.DiscoveryRefreshRate)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.syncServices()
		}
	}
}

// syncServices synchronizes all registered services.
func (m *Mesh) syncServices() {
	services := m.registry.List()
	for _, svc := range services {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		endpoints, err := m.discovery.Discover(ctx, svc.ServiceName, svc.Namespace)
		cancel()

		if err != nil {
			continue
		}

		// Update endpoints in registry
		m.registry.UpdateEndpoints(svc.ServiceName, endpoints)

		// Register endpoints for health checking
		for _, ep := range endpoints {
			m.healthChecker.AddEndpoint(ep)
		}
	}
}

// connectionPoolMaintenance maintains connection pools.
func (m *Mesh) connectionPoolMaintenance() {
	defer m.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.cleanupConnectionPools()
		}
	}
}

// cleanupConnectionPools removes idle connections.
func (m *Mesh) cleanupConnectionPools() {
	m.connPoolsMu.Lock()
	defer m.connPoolsMu.Unlock()

	for addr, pool := range m.connPools {
		pool.Cleanup()
		if pool.Size() == 0 {
			pool.Close()
			delete(m.connPools, addr)
		}
	}
}

// circuitBreakerMaintenance maintains circuit breakers.
func (m *Mesh) circuitBreakerMaintenance() {
	defer m.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.checkCircuitBreakers()
		}
	}
}

// checkCircuitBreakers checks circuit breaker states.
func (m *Mesh) checkCircuitBreakers() {
	m.circuitsMu.RLock()
	defer m.circuitsMu.RUnlock()

	for _, cb := range m.circuits {
		cb.CheckState()
	}
}

// handleInboundRequest handles inbound proxy requests.
func (m *Mesh) handleInboundRequest(ctx context.Context, req *ProxyRequest) (*ProxyResponse, error) {
	// Extract service info from request
	service := req.Headers.Get("X-Mesh-Service")
	namespace := req.Headers.Get("X-Mesh-Namespace")
	if namespace == "" {
		namespace = m.config.Namespace
	}

	// Build mesh request
	meshReq := &Request{
		Method:    req.Method,
		Path:      req.Path,
		Service:   service,
		Namespace: namespace,
		Headers:   req.Headers,
		Body:      req.Body,
		TraceID:   req.Headers.Get("X-Trace-ID"),
	}

	// Call through the mesh
	resp, err := m.Call(ctx, meshReq)
	if err != nil {
		return &ProxyResponse{
			StatusCode: http.StatusBadGateway,
			Headers:    make(http.Header),
		}, err
	}

	return &ProxyResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Headers,
		Body:       resp.Body,
	}, nil
}

// handleOutboundRequest handles outbound proxy requests.
func (m *Mesh) handleOutboundRequest(ctx context.Context, req *ProxyRequest) (*ProxyResponse, error) {
	// Same as inbound but for egress traffic
	return m.handleInboundRequest(ctx, req)
}

// Stats returns mesh statistics.
func (m *Mesh) Stats() MeshStats {
	return MeshStats{
		TotalRequests:       atomic.LoadUint64(&m.stats.TotalRequests),
		SuccessfulRequests:  atomic.LoadUint64(&m.stats.SuccessfulRequests),
		FailedRequests:      atomic.LoadUint64(&m.stats.FailedRequests),
		CircuitBreakerTrips: atomic.LoadUint64(&m.stats.CircuitBreakerTrips),
		Retries:             atomic.LoadUint64(&m.stats.Retries),
		Timeouts:            atomic.LoadUint64(&m.stats.Timeouts),
		RateLimited:         atomic.LoadUint64(&m.stats.RateLimited),
		BytesSent:           atomic.LoadUint64(&m.stats.BytesSent),
		BytesReceived:       atomic.LoadUint64(&m.stats.BytesReceived),
		ActiveConnections:   atomic.LoadInt64(&m.stats.ActiveConnections),
		TotalLatencyNs:      atomic.LoadUint64(&m.stats.TotalLatencyNs),
		RequestCount:        atomic.LoadUint64(&m.stats.RequestCount),
	}
}

// AverageLatency returns the average latency in milliseconds.
func (m *Mesh) AverageLatency() float64 {
	count := atomic.LoadUint64(&m.stats.RequestCount)
	if count == 0 {
		return 0
	}
	totalNs := atomic.LoadUint64(&m.stats.TotalLatencyNs)
	return float64(totalNs) / float64(count) / 1e6
}

// SuccessRate returns the success rate as a percentage.
func (m *Mesh) SuccessRate() float64 {
	total := atomic.LoadUint64(&m.stats.TotalRequests)
	if total == 0 {
		return 100.0
	}
	success := atomic.LoadUint64(&m.stats.SuccessfulRequests)
	return float64(success) / float64(total) * 100
}

// GetCircuitBreakerState returns the circuit breaker state for a service.
func (m *Mesh) GetCircuitBreakerState(service string) CircuitState {
	m.circuitsMu.RLock()
	cb, ok := m.circuits[service]
	m.circuitsMu.RUnlock()

	if !ok {
		return CircuitStateClosed
	}
	return cb.State()
}

// ResetCircuitBreaker resets the circuit breaker for a service.
func (m *Mesh) ResetCircuitBreaker(service string) {
	m.circuitsMu.RLock()
	cb, ok := m.circuits[service]
	m.circuitsMu.RUnlock()

	if ok {
		cb.Reset()
	}
}

// SetServicePolicy sets the policy for a service.
func (m *Mesh) SetServicePolicy(service string, policy *ServicePolicy) {
	m.registry.SetPolicy(service, policy)
}

// GetServicePolicy returns the policy for a service.
func (m *Mesh) GetServicePolicy(service string) *ServicePolicy {
	if policy, ok := m.registry.GetPolicy(service); ok {
		return policy
	}
	return nil
}

// ListServices returns all registered services.
func (m *Mesh) ListServices() []*ServiceInstance {
	return m.registry.List()
}

// GetService returns a service by name.
func (m *Mesh) GetService(name string) (*ServiceInstance, bool) {
	return m.registry.Get(name)
}

// GetEndpoints returns endpoints for a service.
func (m *Mesh) GetEndpoints(ctx context.Context, service, namespace string) ([]*Endpoint, error) {
	return m.discoverEndpoints(ctx, service, namespace)
}

// GetHealthyEndpoints returns healthy endpoints for a service.
func (m *Mesh) GetHealthyEndpoints(ctx context.Context, service, namespace string) ([]*Endpoint, error) {
	endpoints, err := m.discoverEndpoints(ctx, service, namespace)
	if err != nil {
		return nil, err
	}
	return m.filterHealthyEndpoints(endpoints), nil
}

// =============================================================================
// mTLS Provider Integration - Zero-Trust Networking
// =============================================================================

// ConfigureMTLSProvider initializes the zero-trust mTLS provider.
// This provides advanced certificate management with automatic rotation.
func (m *Mesh) ConfigureMTLSProvider(providerConfig *mtls.ProviderConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return ErrMeshClosed
	}

	if providerConfig == nil {
		providerConfig = mtls.DefaultProviderConfig(
			m.config.TrustDomain,
			m.config.ServiceName,
			m.config.Namespace,
		)
	}

	m.mtlsProvider = mtls.NewProvider(providerConfig)

	// Set up identity manager
	identityConfig := mtls.DefaultIdentityConfig(m.config.ServiceName, m.config.Namespace)
	identityConfig.TrustDomain = m.config.TrustDomain
	m.identityMgr = mtls.NewIdentityManager(identityConfig)

	// Set up zero-trust enforcer
	m.zeroTrustEnforcer = mtls.NewZeroTrustEnforcer([]string{m.config.TrustDomain})

	return nil
}

// InitializeMTLSWithCA initializes mTLS with a CA for self-signed certificates.
func (m *Mesh) InitializeMTLSWithCA(caCert *x509.Certificate, caKey crypto.Signer) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return ErrMeshClosed
	}

	if m.mtlsProvider == nil {
		providerConfig := mtls.DefaultProviderConfig(
			m.config.TrustDomain,
			m.config.ServiceName,
			m.config.Namespace,
		)
		m.mtlsProvider = mtls.NewProvider(providerConfig)
	}

	if err := m.mtlsProvider.InitializeWithCA(caCert, caKey); err != nil {
		return fmt.Errorf("mTLS initialization failed: %w", err)
	}

	// Update TLS configs from provider
	m.tlsConfig = m.mtlsProvider.ServerTLSConfig()
	m.clientTLSConfig = m.mtlsProvider.ClientTLSConfig()
	m.rootCAs = m.mtlsProvider.RootCAs()

	// Update HTTP client
	m.updateHTTPClient()

	// Set up rotation callback
	m.mtlsProvider.OnRotation(func(cert *tls.Certificate) {
		m.onCertificateRotation(cert)
	})

	return nil
}

// InitializeMTLSFromFiles initializes mTLS from certificate files.
func (m *Mesh) InitializeMTLSFromFiles(certFile, keyFile, caFile string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return ErrMeshClosed
	}

	if m.mtlsProvider == nil {
		providerConfig := mtls.DefaultProviderConfig(
			m.config.TrustDomain,
			m.config.ServiceName,
			m.config.Namespace,
		)
		m.mtlsProvider = mtls.NewProvider(providerConfig)
	}

	if err := m.mtlsProvider.InitializeWithFiles(certFile, keyFile, caFile); err != nil {
		return fmt.Errorf("mTLS initialization failed: %w", err)
	}

	// Update TLS configs from provider
	m.tlsConfig = m.mtlsProvider.ServerTLSConfig()
	m.clientTLSConfig = m.mtlsProvider.ClientTLSConfig()
	m.rootCAs = m.mtlsProvider.RootCAs()

	// Update HTTP client
	m.updateHTTPClient()

	// Set up rotation callback
	m.mtlsProvider.OnRotation(func(cert *tls.Certificate) {
		m.onCertificateRotation(cert)
	})

	return nil
}

// InitializeMTLSWithCertManager initializes mTLS using the centralized cert manager.
func (m *Mesh) InitializeMTLSWithCertManager(certMgr certs.CertManager) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return ErrMeshClosed
	}

	if m.mtlsProvider == nil {
		providerConfig := mtls.DefaultProviderConfig(
			m.config.TrustDomain,
			m.config.ServiceName,
			m.config.Namespace,
		)
		m.mtlsProvider = mtls.NewProvider(providerConfig)
	}

	if err := m.mtlsProvider.InitializeWithCertManager(certMgr); err != nil {
		return fmt.Errorf("mTLS initialization failed: %w", err)
	}

	// Update TLS configs from provider
	m.tlsConfig = m.mtlsProvider.ServerTLSConfig()
	m.clientTLSConfig = m.mtlsProvider.ClientTLSConfig()
	m.rootCAs = m.mtlsProvider.RootCAs()

	// Update HTTP client
	m.updateHTTPClient()

	// Set up rotation callback
	m.mtlsProvider.OnRotation(func(cert *tls.Certificate) {
		m.onCertificateRotation(cert)
	})

	return nil
}

// onCertificateRotation handles certificate rotation events.
func (m *Mesh) onCertificateRotation(cert *tls.Certificate) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Update TLS configs
	if m.mtlsProvider != nil {
		m.tlsConfig = m.mtlsProvider.ServerTLSConfig()
		m.clientTLSConfig = m.mtlsProvider.ClientTLSConfig()
	}

	// Update HTTP client with new TLS config
	m.updateHTTPClient()
}

// updateHTTPClient updates the HTTP client with current TLS config.
func (m *Mesh) updateHTTPClient() {
	m.httpClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig:     m.clientTLSConfig,
			MaxIdleConns:        m.config.MaxIdleConns,
			MaxIdleConnsPerHost: m.config.MaxIdleConnsPerHost,
			IdleConnTimeout:     m.config.IdleConnTimeout,
			DialContext: (&net.Dialer{
				Timeout:   m.config.ConnectTimeout,
				KeepAlive: m.config.KeepAliveInterval,
			}).DialContext,
		},
		Timeout: m.config.RequestTimeout,
	}
}

// SetAuthorizationPolicy sets an authorization policy for zero-trust enforcement.
func (m *Mesh) SetAuthorizationPolicy(service string, policy *mtls.AuthorizationPolicy) {
	m.mu.RLock()
	enforcer := m.zeroTrustEnforcer
	identityMgr := m.identityMgr
	m.mu.RUnlock()

	if enforcer != nil {
		enforcer.SetPolicy(service, policy)
	}
	if identityMgr != nil {
		identityMgr.SetPolicy(service, policy)
	}
}

// EnforceZeroTrust validates and authorizes a TLS connection.
func (m *Mesh) EnforceZeroTrust(conn *tls.Conn, targetService string) error {
	m.mu.RLock()
	enforcer := m.zeroTrustEnforcer
	m.mu.RUnlock()

	if enforcer == nil {
		return nil // Zero-trust not configured
	}

	return enforcer.Enforce(conn, targetService)
}

// GetPeerIdentity extracts the peer's SPIFFE identity from a connection.
func (m *Mesh) GetPeerIdentity(conn *tls.Conn) (*mtls.SPIFFEID, error) {
	m.mu.RLock()
	identityMgr := m.identityMgr
	m.mu.RUnlock()

	if identityMgr == nil {
		// Fall back to basic extraction
		info, err := mtls.ExtractPeerInfo(conn)
		if err != nil {
			return nil, err
		}
		return info.SPIFFEID, nil
	}

	return identityMgr.ValidatePeer(conn)
}

// SPIFFEID returns the mesh's SPIFFE identity.
func (m *Mesh) SPIFFEID() *mtls.SPIFFEID {
	m.mu.RLock()
	provider := m.mtlsProvider
	m.mu.RUnlock()

	if provider != nil {
		return provider.SPIFFEID()
	}
	return nil
}

// MTLSProvider returns the mTLS provider.
func (m *Mesh) MTLSProvider() *mtls.Provider {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mtlsProvider
}

// ServerTLSConfig returns the server TLS configuration.
func (m *Mesh) ServerTLSConfig() *tls.Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.tlsConfig == nil {
		return nil
	}
	return m.tlsConfig.Clone()
}

// ClientTLSConfig returns the client TLS configuration.
func (m *Mesh) ClientTLSConfig() *tls.Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.clientTLSConfig == nil {
		return nil
	}
	return m.clientTLSConfig.Clone()
}

// RotateCertificate forces a certificate rotation.
func (m *Mesh) RotateCertificate() error {
	m.mu.RLock()
	provider := m.mtlsProvider
	m.mu.RUnlock()

	if provider == nil {
		return errors.New("mTLS provider not configured")
	}

	return provider.Rotate()
}

// SaveCertificates saves current certificates to files.
func (m *Mesh) SaveCertificates(certDir string) error {
	m.mu.RLock()
	provider := m.mtlsProvider
	m.mu.RUnlock()

	if provider == nil {
		return errors.New("mTLS provider not configured")
	}

	return provider.SaveToFiles(certDir)
}

// ZeroTrustStats returns zero-trust enforcement statistics.
func (m *Mesh) ZeroTrustStats() (allowed, denied uint64) {
	m.mu.RLock()
	enforcer := m.zeroTrustEnforcer
	m.mu.RUnlock()

	if enforcer != nil {
		return enforcer.Stats()
	}
	return 0, 0
}
