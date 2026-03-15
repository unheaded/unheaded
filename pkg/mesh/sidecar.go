// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package mesh provides THE HAUBERK - the service mesh sidecar for the Unheaded Kingdom.
package mesh

import (
	"context"
	"crypto"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"unheaded/pkg/mesh/discovery"
	"unheaded/pkg/mesh/loadbalance"
	"unheaded/pkg/mesh/mtls"
	"unheaded/pkg/mesh/observe"
	"unheaded/pkg/mesh/policy"
	"unheaded/pkg/mesh/proxy"
)

var (
	// ErrNotInitialized indicates the sidecar is not initialized.
	ErrNotInitialized = errors.New("sidecar not initialized")
	// ErrAlreadyRunning indicates the sidecar is already running.
	ErrAlreadyRunning = errors.New("sidecar already running")
)

// Sidecar is the main service mesh sidecar component.
type Sidecar struct {
	config *SidecarConfig

	// Core components
	proxy       *proxy.Proxy
	mtlsMgr     *mtls.Manager
	discoverer  discovery.Discoverer
	policyStore *policy.PolicyStore

	// Load balancing
	balancerRegistry map[string]loadbalance.Balancer
	endpointPools    map[string]*loadbalance.EndpointPool
	balancerMu       sync.RWMutex

	// Circuit breakers
	circuitBreakers *policy.CircuitBreakerRegistry

	// Health checking
	healthChecker       *proxy.HealthChecker
	passiveHealthChecker *proxy.PassiveHealthChecker

	// Observability
	metrics      *observe.Metrics
	tracer       *observe.Tracer
	accessLogger *observe.AccessLogger

	// State
	mu          sync.RWMutex
	running     bool
	initialized bool

	// Shutdown
	ctx    context.Context
	cancel context.CancelFunc
}

// SidecarConfig configures the sidecar.
type SidecarConfig struct {
	// Service identity
	ServiceName string
	Namespace   string
	TrustDomain string

	// Proxy settings
	InboundPort  int
	OutboundPort int

	// mTLS settings
	CertValidity   time.Duration
	RotationBuffer time.Duration

	// Discovery settings
	DiscoveryType    string // dns, registry, static
	RegistryURL      string
	DNSDomain        string
	DiscoveryCacheTTL time.Duration

	// Load balancing
	DefaultLB string // round-robin, least-conn, weighted

	// Policy defaults
	DefaultTimeout        time.Duration
	DefaultRetries        int
	DefaultCircuitBreaker *policy.CircuitBreakerConfig

	// Health check settings
	HealthCheckInterval time.Duration
	HealthCheckTimeout  time.Duration

	// Observability
	MetricsEnabled bool
	TracingEnabled bool
	TraceSampleRate float64
	AccessLogEnabled bool
}

// DefaultSidecarConfig returns a default sidecar configuration.
func DefaultSidecarConfig(serviceName, namespace string) *SidecarConfig {
	return &SidecarConfig{
		ServiceName:       serviceName,
		Namespace:         namespace,
		TrustDomain:       mtls.DefaultTrustDomain,
		InboundPort:       15001,
		OutboundPort:      15006,
		CertValidity:      time.Hour,
		RotationBuffer:    10 * time.Minute,
		DiscoveryType:     "dns",
		DNSDomain:         "svc.unheaded.local",
		DiscoveryCacheTTL: 30 * time.Second,
		DefaultLB:         "round-robin",
		DefaultTimeout:    30 * time.Second,
		DefaultRetries:    3,
		DefaultCircuitBreaker: &policy.CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 5,
			SuccessThreshold: 3,
			HalfOpenRequests: 1,
			Timeout:          30 * time.Second,
		},
		HealthCheckInterval: 10 * time.Second,
		HealthCheckTimeout:  5 * time.Second,
		MetricsEnabled:      true,
		TracingEnabled:      true,
		TraceSampleRate:     0.1,
		AccessLogEnabled:    true,
	}
}

// NewSidecar creates a new sidecar.
func NewSidecar(config *SidecarConfig) *Sidecar {
	if config == nil {
		config = DefaultSidecarConfig("unknown", "default")
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Sidecar{
		config:           config,
		balancerRegistry: make(map[string]loadbalance.Balancer),
		endpointPools:    make(map[string]*loadbalance.EndpointPool),
		policyStore:      policy.NewPolicyStore(),
		ctx:              ctx,
		cancel:           cancel,
	}
}

// Initialize initializes the sidecar with CA credentials.
func (s *Sidecar) Initialize(caCert *x509.Certificate, caKey crypto.Signer) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.initialized {
		return nil
	}

	// Initialize mTLS
	mtlsConfig := mtls.DefaultConfig(s.config.TrustDomain, s.config.ServiceName)
	mtlsConfig.Namespace = s.config.Namespace
	mtlsConfig.CertValidity = s.config.CertValidity
	mtlsConfig.RotationBuffer = s.config.RotationBuffer

	s.mtlsMgr = mtls.NewManager(mtlsConfig)
	if err := s.mtlsMgr.Initialize(caCert, caKey); err != nil {
		return fmt.Errorf("mTLS initialization failed: %w", err)
	}

	// Initialize discovery
	if err := s.initDiscovery(); err != nil {
		return fmt.Errorf("discovery initialization failed: %w", err)
	}

	// Initialize proxy
	if err := s.initProxy(); err != nil {
		return fmt.Errorf("proxy initialization failed: %w", err)
	}

	// Initialize circuit breakers
	s.circuitBreakers = policy.NewCircuitBreakerRegistry(s.config.DefaultCircuitBreaker)

	// Initialize health checking
	s.healthChecker = proxy.NewHealthChecker(&proxy.HealthCheckConfig{
		Interval: s.config.HealthCheckInterval,
		Timeout:  s.config.HealthCheckTimeout,
	})
	s.passiveHealthChecker = proxy.NewPassiveHealthChecker(nil)

	// Initialize observability
	s.initObservability()

	s.initialized = true
	return nil
}

// InitializeWithBundle initializes with existing certificates.
func (s *Sidecar) InitializeWithBundle(cert *x509.Certificate, key crypto.Signer, rootCAs *x509.CertPool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.initialized {
		return nil
	}

	// Initialize mTLS with existing cert
	mtlsConfig := mtls.DefaultConfig(s.config.TrustDomain, s.config.ServiceName)
	mtlsConfig.Namespace = s.config.Namespace
	mtlsConfig.RootCAs = rootCAs

	s.mtlsMgr = mtls.NewManager(mtlsConfig)
	if err := s.mtlsMgr.InitializeWithBundle(cert, key, rootCAs); err != nil {
		return fmt.Errorf("mTLS initialization failed: %w", err)
	}

	// Initialize other components
	if err := s.initDiscovery(); err != nil {
		return fmt.Errorf("discovery initialization failed: %w", err)
	}

	if err := s.initProxy(); err != nil {
		return fmt.Errorf("proxy initialization failed: %w", err)
	}

	s.circuitBreakers = policy.NewCircuitBreakerRegistry(s.config.DefaultCircuitBreaker)
	s.healthChecker = proxy.NewHealthChecker(&proxy.HealthCheckConfig{
		Interval: s.config.HealthCheckInterval,
		Timeout:  s.config.HealthCheckTimeout,
	})
	s.passiveHealthChecker = proxy.NewPassiveHealthChecker(nil)
	s.initObservability()

	s.initialized = true
	return nil
}

func (s *Sidecar) initDiscovery() error {
	switch s.config.DiscoveryType {
	case "dns":
		dnsConfig := &discovery.DNSConfig{
			Domain:      s.config.DNSDomain,
			DefaultPort: s.config.InboundPort,
			RefreshRate: s.config.DiscoveryCacheTTL,
		}
		s.discoverer = discovery.NewCachingDiscoverer(
			discovery.NewDNSDiscoverer(dnsConfig),
			s.config.DiscoveryCacheTTL,
		)

	case "registry":
		registryConfig := &discovery.RegistryConfig{
			BaseURL:         s.config.RegistryURL,
			Timeout:         10 * time.Second,
			RefreshInterval: s.config.DiscoveryCacheTTL,
		}
		s.discoverer = discovery.NewCachingDiscoverer(
			discovery.NewRegistryClient(registryConfig),
			s.config.DiscoveryCacheTTL,
		)

	case "static":
		s.discoverer = discovery.NewStaticDiscoverer()

	default:
		return fmt.Errorf("unknown discovery type: %s", s.config.DiscoveryType)
	}

	return nil
}

func (s *Sidecar) initProxy() error {
	proxyConfig := &proxy.Config{
		InboundAddr:     "127.0.0.1",
		InboundPort:     s.config.InboundPort,
		OutboundAddr:    "127.0.0.1",
		OutboundPort:    s.config.OutboundPort,
		ConnectTimeout:  5 * time.Second,
		ReadTimeout:     s.config.DefaultTimeout,
		WriteTimeout:    s.config.DefaultTimeout,
		IdleTimeout:     90 * time.Second,
		ReadBufferSize:  32 * 1024,
		WriteBufferSize: 32 * 1024,
		MaxIdleConns:    100,
		MaxConnsPerHost: 10,
		TLSConfig:       s.mtlsMgr.ServerTLSConfig(),
		ClientTLSConfig: s.mtlsMgr.ClientTLSConfig(),
	}

	s.proxy = proxy.NewProxy(proxyConfig)

	// Set up backend selector
	s.proxy.SetBackendSelector(s.selectBackend)

	// Set up request/response callbacks
	s.proxy.OnRequest(s.onRequest)
	s.proxy.OnResponse(s.onResponse)

	return nil
}

func (s *Sidecar) initObservability() {
	// Metrics
	if s.config.MetricsEnabled {
		s.metrics = observe.NewMetrics()
	}

	// Tracing
	if s.config.TracingEnabled {
		exporter := observe.NewInMemoryExporter()
		s.tracer = observe.NewTracer(s.config.ServiceName, s.config.TraceSampleRate, exporter)
	}

	// Access logging
	if s.config.AccessLogEnabled {
		s.accessLogger = observe.NewAccessLogger(nil)
	}
}

// Start starts the sidecar.
func (s *Sidecar) Start() error {
	s.mu.Lock()
	if !s.initialized {
		s.mu.Unlock()
		return ErrNotInitialized
	}
	if s.running {
		s.mu.Unlock()
		return ErrAlreadyRunning
	}
	s.running = true
	s.mu.Unlock()

	// Start the proxy
	if err := s.proxy.Start(); err != nil {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		return err
	}

	return nil
}

// Stop stops the sidecar.
func (s *Sidecar) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	s.mu.Unlock()

	// Cancel context
	s.cancel()

	// Stop proxy
	if s.proxy != nil {
		s.proxy.Stop()
	}

	// Stop health checker
	if s.healthChecker != nil {
		s.healthChecker.Close()
	}

	// Close mTLS
	if s.mtlsMgr != nil {
		s.mtlsMgr.Close()
	}

	// Close discoverer
	if s.discoverer != nil {
		s.discoverer.Close()
	}

	// Close access logger
	if s.accessLogger != nil {
		s.accessLogger.Close()
	}

	return nil
}

// selectBackend selects a backend for the request.
func (s *Sidecar) selectBackend(ctx context.Context) (string, error) {
	// This would be called with service context from the request
	// For now, return an error indicating manual configuration needed
	return "", ErrNotInitialized
}

// SelectBackendForService selects a backend for a specific service.
func (s *Sidecar) SelectBackendForService(ctx context.Context, serviceName, namespace string) (string, error) {
	// Get or create endpoint pool
	pool := s.getOrCreateEndpointPool(serviceName, namespace)

	// Discover endpoints if needed
	svc, err := s.discoverer.Discover(ctx, serviceName, namespace)
	if err != nil {
		return "", err
	}

	// Update pool with discovered endpoints
	endpoints := s.convertEndpoints(svc.Endpoints)
	pool.SetEndpoints(endpoints)

	// Pick an endpoint
	endpoint := pool.Pick()
	if endpoint == nil {
		return "", discovery.ErrNoEndpoints
	}

	// Check circuit breaker
	cb := s.circuitBreakers.Get(serviceName)
	if cb.State() == policy.StateOpen {
		return "", policy.ErrCircuitOpen
	}

	address := net.JoinHostPort(endpoint.Address, itoa(endpoint.Port))
	return address, nil
}

func (s *Sidecar) getOrCreateEndpointPool(serviceName, namespace string) *loadbalance.EndpointPool {
	key := namespace + "/" + serviceName

	s.balancerMu.RLock()
	pool, ok := s.endpointPools[key]
	s.balancerMu.RUnlock()

	if ok {
		return pool
	}

	s.balancerMu.Lock()
	defer s.balancerMu.Unlock()

	// Double-check
	if pool, ok := s.endpointPools[key]; ok {
		return pool
	}

	// Create balancer
	var balancer loadbalance.Balancer
	switch s.config.DefaultLB {
	case "least-conn":
		balancer = loadbalance.NewLeastConn()
	case "weighted":
		balancer = loadbalance.NewWeighted()
	default:
		balancer = loadbalance.NewRoundRobin()
	}

	pool = loadbalance.NewEndpointPool(balancer)
	s.endpointPools[key] = pool

	return pool
}

func (s *Sidecar) convertEndpoints(eps []*discovery.Endpoint) []*loadbalance.Endpoint {
	result := make([]*loadbalance.Endpoint, len(eps))
	for i, ep := range eps {
		result[i] = &loadbalance.Endpoint{
			Address: ep.Address,
			Port:    ep.Port,
			Weight:  ep.Weight,
			Healthy: ep.Healthy,
			Meta:    ep.Tags,
		}
	}
	return result
}

func (s *Sidecar) onRequest(req *proxy.Request) {
	if s.metrics != nil {
		// Record start of request
	}

	if s.tracer != nil {
		// Start span
	}
}

func (s *Sidecar) onResponse(resp *proxy.Response) {
	if s.metrics != nil {
		// Parse service from response
		s.metrics.RecordRequest("", "", resp.StatusCode, resp.Duration, resp.Error)
	}

	if s.accessLogger != nil {
		entry := &observe.AccessLogEntry{
			Timestamp:       time.Now(),
			StatusCode:      resp.StatusCode,
			Latency:         resp.Duration,
			UpstreamAddress: resp.Backend,
		}
		if resp.Error != nil {
			entry.Error = resp.Error.Error()
		}
		s.accessLogger.Log(entry)
	}

	// Update passive health checker
	if s.passiveHealthChecker != nil && resp.Backend != "" {
		if resp.Error != nil || resp.StatusCode >= 500 {
			s.passiveHealthChecker.RecordFailure(resp.Backend)
		} else {
			s.passiveHealthChecker.RecordSuccess(resp.Backend)
		}
	}
}

// SetPolicy sets a traffic policy for a service.
func (s *Sidecar) SetPolicy(serviceName, namespace string, p *policy.Policy) {
	s.policyStore.Set(namespace, serviceName, p)
}

// GetPolicy gets the traffic policy for a service.
func (s *Sidecar) GetPolicy(serviceName, namespace string) *policy.Policy {
	return s.policyStore.Get(namespace, serviceName)
}

// AddStaticEndpoint adds a static endpoint for a service.
func (s *Sidecar) AddStaticEndpoint(serviceName, namespace, address string, port int) {
	if static, ok := s.discoverer.(*discovery.StaticDiscoverer); ok {
		svc := &discovery.Service{
			Name:      serviceName,
			Namespace: namespace,
			Endpoints: []*discovery.Endpoint{
				{
					Address: address,
					Port:    port,
					Weight:  1,
					Healthy: true,
				},
			},
		}
		static.AddService(svc)
	}
}

// SPIFFEID returns the sidecar's SPIFFE ID.
func (s *Sidecar) SPIFFEID() string {
	if s.mtlsMgr != nil {
		return s.mtlsMgr.SPIFFEID().String()
	}
	return ""
}

// Metrics returns the metrics collector.
func (s *Sidecar) Metrics() *observe.Metrics {
	return s.metrics
}

// Stats returns sidecar statistics.
func (s *Sidecar) Stats() SidecarStats {
	stats := SidecarStats{
		Running:     s.running,
		Initialized: s.initialized,
	}

	if s.proxy != nil {
		proxyStats := s.proxy.Stats()
		stats.ActiveConnections = proxyStats.ActiveConnections
		stats.TotalConnections = proxyStats.TotalConnections
		stats.TotalRequests = proxyStats.TotalRequests
	}

	if s.metrics != nil {
		snapshot := s.metrics.Snapshot()
		stats.SuccessRate = snapshot.SuccessRate()
		stats.P50Latency = snapshot.LatencyPercentile(50)
		stats.P99Latency = snapshot.LatencyPercentile(99)
	}

	if s.circuitBreakers != nil {
		cbStats := s.circuitBreakers.Stats()
		stats.CircuitBreakerStats = make(map[string]string)
		for svc, cb := range cbStats {
			stats.CircuitBreakerStats[svc] = cb.State.String()
		}
	}

	return stats
}

// SidecarStats holds sidecar statistics.
type SidecarStats struct {
	Running             bool
	Initialized         bool
	ActiveConnections   int64
	TotalConnections    uint64
	TotalRequests       uint64
	SuccessRate         float64
	P50Latency          float64
	P99Latency          float64
	CircuitBreakerStats map[string]string
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}
