// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package mesh provides THE HAUBERK - the service mesh implementation for the Unheaded Kingdom.
// This file implements mesh configuration and validation.
package mesh

import (
	"errors"
	"sync"
	"time"
)

// Config holds the complete mesh configuration.
type Config struct {
	// Service Identity
	ServiceName string // Name of the service using this mesh
	Namespace   string // Namespace for service isolation
	TrustDomain string // SPIFFE trust domain

	// Discovery Configuration
	DiscoveryType         string        // dns, registry, static, kubernetes
	DiscoveryRefreshRate  time.Duration // How often to refresh service discovery
	DNSDomain             string        // Base domain for DNS discovery
	DNSResolver           string        // Custom DNS resolver (empty = system default)
	RegistryEndpoints     []string      // Registry service endpoints
	StaticEndpoints       map[string][]EndpointConfig

	// Proxy Configuration
	InboundPort     int    // Port for inbound proxy (0 = disabled)
	InboundAddress  string // Address for inbound proxy
	OutboundPort    int    // Port for outbound proxy (0 = disabled)
	OutboundAddress string // Address for outbound proxy
	EnableTLS       bool   // Enable mTLS

	// Timeouts
	ConnectTimeout    time.Duration // Connection timeout
	RequestTimeout    time.Duration // Request timeout
	IdleConnTimeout   time.Duration // Idle connection timeout
	KeepAliveInterval time.Duration // Keep-alive interval

	// Connection Pool
	MaxIdleConns        int // Maximum idle connections total
	MaxIdleConnsPerHost int // Maximum idle connections per host
	MaxConnsPerHost     int // Maximum connections per host

	// Load Balancing
	LoadBalancerType string // round-robin, least-conn, weighted, random, ip-hash, consistent-hash
	LocalZone        string // Local zone for zone-aware load balancing
	SubsetSize       int    // Subset size for subset balancing

	// Circuit Breaker
	CircuitBreakerEnabled          bool          // Enable circuit breakers
	CircuitBreakerFailureThreshold int           // Failures before opening
	CircuitBreakerSuccessThreshold int           // Successes to close from half-open
	CircuitBreakerTimeout          time.Duration // How long circuit stays open
	CircuitBreakerHalfOpenRequests int           // Max requests in half-open state
	CircuitBreakerWindowDuration   time.Duration // Sliding window for counting failures

	// Retry
	DefaultRetries  int           // Default retry attempts
	RetryBackoff    time.Duration // Initial retry backoff
	MaxRetryBackoff time.Duration // Maximum retry backoff

	// Rate Limiting
	DefaultRateLimit   float64 // Requests per second (0 = disabled)
	DefaultRateBurst   int     // Maximum burst size
	RateLimitAlgorithm string  // token-bucket or sliding-window

	// Health Checking
	HealthCheckEnabled     bool          // Enable health checking
	HealthCheckInterval    time.Duration // Health check interval
	HealthCheckTimeout     time.Duration // Health check timeout
	HealthyThreshold       int           // Healthy threshold
	UnhealthyThreshold     int           // Unhealthy threshold
	PassiveHealthChecking  bool          // Enable passive health checking

	// Observability
	MetricsEnabled   bool    // Enable metrics collection
	TracingEnabled   bool    // Enable distributed tracing
	TraceSampleRate  float64 // Trace sampling rate (0.0 to 1.0)
	AccessLogEnabled bool    // Enable access logging

	// Advanced
	EnableHTTP2           bool          // Enable HTTP/2
	EnableGRPC            bool          // Enable gRPC support
	MaxRequestBodySize    int64         // Maximum request body size
	MaxResponseHeaderSize int           // Maximum response header size
	GracefulShutdown      time.Duration // Graceful shutdown timeout
}

// EndpointConfig configures a static endpoint.
type EndpointConfig struct {
	Address  string
	Port     int
	Weight   int
	Zone     string
	Tags     map[string]string
	Protocol string
	TLS      bool
}

// DefaultConfig returns a default mesh configuration.
func DefaultConfig() *Config {
	return &Config{
		ServiceName: "unknown",
		Namespace:   "default",
		TrustDomain: "cluster.local",

		// Discovery
		DiscoveryType:        DiscoveryTypeDNS,
		DiscoveryRefreshRate: 30 * time.Second,
		DNSDomain:            "svc.cluster.local",

		// Proxy
		InboundPort:     15001,
		InboundAddress:  "127.0.0.1",
		OutboundPort:    15006,
		OutboundAddress: "127.0.0.1",
		EnableTLS:       true,

		// Timeouts
		ConnectTimeout:    5 * time.Second,
		RequestTimeout:    30 * time.Second,
		IdleConnTimeout:   90 * time.Second,
		KeepAliveInterval: 30 * time.Second,

		// Connection Pool
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		MaxConnsPerHost:     50,

		// Load Balancing
		LoadBalancerType: LoadBalancerRoundRobin,

		// Circuit Breaker
		CircuitBreakerEnabled:          true,
		CircuitBreakerFailureThreshold: 5,
		CircuitBreakerSuccessThreshold: 3,
		CircuitBreakerTimeout:          30 * time.Second,
		CircuitBreakerHalfOpenRequests: 3,
		CircuitBreakerWindowDuration:   60 * time.Second,

		// Retry
		DefaultRetries:  3,
		RetryBackoff:    100 * time.Millisecond,
		MaxRetryBackoff: 10 * time.Second,

		// Rate Limiting
		DefaultRateLimit:   0, // Disabled by default
		DefaultRateBurst:   10,
		RateLimitAlgorithm: "token-bucket",

		// Health Checking
		HealthCheckEnabled:    true,
		HealthCheckInterval:   10 * time.Second,
		HealthCheckTimeout:    5 * time.Second,
		HealthyThreshold:      2,
		UnhealthyThreshold:    3,
		PassiveHealthChecking: true,

		// Observability
		MetricsEnabled:   true,
		TracingEnabled:   true,
		TraceSampleRate:  0.1,
		AccessLogEnabled: true,

		// Advanced
		EnableHTTP2:           true,
		EnableGRPC:            true,
		MaxRequestBodySize:    10 * 1024 * 1024, // 10MB
		MaxResponseHeaderSize: 1 * 1024 * 1024,  // 1MB
		GracefulShutdown:      30 * time.Second,
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.ServiceName == "" {
		return errors.New("service name is required")
	}

	if c.Namespace == "" {
		return errors.New("namespace is required")
	}

	// Validate discovery type
	switch c.DiscoveryType {
	case DiscoveryTypeDNS, DiscoveryTypeRegistry, DiscoveryTypeStatic, DiscoveryTypeKubernetes:
		// Valid
	default:
		return errors.New("invalid discovery type")
	}

	// Validate load balancer type
	switch c.LoadBalancerType {
	case LoadBalancerRoundRobin, LoadBalancerLeastConn, LoadBalancerWeighted,
		LoadBalancerRandom, LoadBalancerIPHash, LoadBalancerConsistent:
		// Valid
	default:
		return errors.New("invalid load balancer type")
	}

	// Validate timeouts
	if c.ConnectTimeout <= 0 {
		return errors.New("connect timeout must be positive")
	}

	if c.RequestTimeout <= 0 {
		return errors.New("request timeout must be positive")
	}

	// Validate circuit breaker
	if c.CircuitBreakerEnabled {
		if c.CircuitBreakerFailureThreshold <= 0 {
			return errors.New("circuit breaker failure threshold must be positive")
		}
		if c.CircuitBreakerSuccessThreshold <= 0 {
			return errors.New("circuit breaker success threshold must be positive")
		}
		if c.CircuitBreakerTimeout <= 0 {
			return errors.New("circuit breaker timeout must be positive")
		}
	}

	// Validate retries
	if c.DefaultRetries < 0 {
		return errors.New("default retries cannot be negative")
	}

	// Validate rate limiting
	if c.DefaultRateLimit < 0 {
		return errors.New("rate limit cannot be negative")
	}

	// Validate health checking
	if c.HealthCheckEnabled {
		if c.HealthCheckInterval <= 0 {
			return errors.New("health check interval must be positive")
		}
		if c.HealthCheckTimeout <= 0 {
			return errors.New("health check timeout must be positive")
		}
		if c.HealthCheckTimeout >= c.HealthCheckInterval {
			return errors.New("health check timeout must be less than interval")
		}
	}

	// Validate observability
	if c.TraceSampleRate < 0 || c.TraceSampleRate > 1 {
		return errors.New("trace sample rate must be between 0 and 1")
	}

	return nil
}

// Clone creates a deep copy of the configuration.
func (c *Config) Clone() *Config {
	clone := *c

	// Deep copy slices and maps
	if c.RegistryEndpoints != nil {
		clone.RegistryEndpoints = make([]string, len(c.RegistryEndpoints))
		copy(clone.RegistryEndpoints, c.RegistryEndpoints)
	}

	if c.StaticEndpoints != nil {
		clone.StaticEndpoints = make(map[string][]EndpointConfig)
		for k, v := range c.StaticEndpoints {
			endpoints := make([]EndpointConfig, len(v))
			copy(endpoints, v)
			clone.StaticEndpoints[k] = endpoints
		}
	}

	return &clone
}

// ConfigBuilder provides a fluent interface for building configurations.
type ConfigBuilder struct {
	config *Config
	err    error
}

// NewConfigBuilder creates a new configuration builder.
func NewConfigBuilder() *ConfigBuilder {
	return &ConfigBuilder{
		config: DefaultConfig(),
	}
}

// WithServiceName sets the service name.
func (b *ConfigBuilder) WithServiceName(name string) *ConfigBuilder {
	b.config.ServiceName = name
	return b
}

// WithNamespace sets the namespace.
func (b *ConfigBuilder) WithNamespace(namespace string) *ConfigBuilder {
	b.config.Namespace = namespace
	return b
}

// WithTrustDomain sets the trust domain.
func (b *ConfigBuilder) WithTrustDomain(domain string) *ConfigBuilder {
	b.config.TrustDomain = domain
	return b
}

// WithDiscovery sets discovery configuration.
func (b *ConfigBuilder) WithDiscovery(discoveryType string, refreshRate time.Duration) *ConfigBuilder {
	b.config.DiscoveryType = discoveryType
	b.config.DiscoveryRefreshRate = refreshRate
	return b
}

// WithDNSDiscovery configures DNS-based discovery.
func (b *ConfigBuilder) WithDNSDiscovery(domain, resolver string) *ConfigBuilder {
	b.config.DiscoveryType = DiscoveryTypeDNS
	b.config.DNSDomain = domain
	b.config.DNSResolver = resolver
	return b
}

// WithRegistryDiscovery configures registry-based discovery.
func (b *ConfigBuilder) WithRegistryDiscovery(endpoints []string) *ConfigBuilder {
	b.config.DiscoveryType = DiscoveryTypeRegistry
	b.config.RegistryEndpoints = endpoints
	return b
}

// WithStaticEndpoints adds static endpoints for a service.
func (b *ConfigBuilder) WithStaticEndpoints(service string, endpoints []EndpointConfig) *ConfigBuilder {
	if b.config.StaticEndpoints == nil {
		b.config.StaticEndpoints = make(map[string][]EndpointConfig)
	}
	b.config.StaticEndpoints[service] = endpoints
	return b
}

// WithInboundProxy configures the inbound proxy.
func (b *ConfigBuilder) WithInboundProxy(address string, port int) *ConfigBuilder {
	b.config.InboundAddress = address
	b.config.InboundPort = port
	return b
}

// WithOutboundProxy configures the outbound proxy.
func (b *ConfigBuilder) WithOutboundProxy(address string, port int) *ConfigBuilder {
	b.config.OutboundAddress = address
	b.config.OutboundPort = port
	return b
}

// WithTLS enables or disables TLS.
func (b *ConfigBuilder) WithTLS(enabled bool) *ConfigBuilder {
	b.config.EnableTLS = enabled
	return b
}

// WithTimeouts sets timeout configuration.
func (b *ConfigBuilder) WithTimeouts(connect, request, idle time.Duration) *ConfigBuilder {
	b.config.ConnectTimeout = connect
	b.config.RequestTimeout = request
	b.config.IdleConnTimeout = idle
	return b
}

// WithConnectionPool configures the connection pool.
func (b *ConfigBuilder) WithConnectionPool(maxIdle, maxIdlePerHost, maxPerHost int) *ConfigBuilder {
	b.config.MaxIdleConns = maxIdle
	b.config.MaxIdleConnsPerHost = maxIdlePerHost
	b.config.MaxConnsPerHost = maxPerHost
	return b
}

// WithLoadBalancer sets the load balancer type.
func (b *ConfigBuilder) WithLoadBalancer(lbType string) *ConfigBuilder {
	b.config.LoadBalancerType = lbType
	return b
}

// WithZoneAwareLoadBalancing configures zone-aware load balancing.
func (b *ConfigBuilder) WithZoneAwareLoadBalancing(localZone string) *ConfigBuilder {
	b.config.LocalZone = localZone
	return b
}

// WithCircuitBreaker configures the circuit breaker.
func (b *ConfigBuilder) WithCircuitBreaker(failureThreshold, successThreshold int, timeout time.Duration) *ConfigBuilder {
	b.config.CircuitBreakerEnabled = true
	b.config.CircuitBreakerFailureThreshold = failureThreshold
	b.config.CircuitBreakerSuccessThreshold = successThreshold
	b.config.CircuitBreakerTimeout = timeout
	return b
}

// WithoutCircuitBreaker disables circuit breaking.
func (b *ConfigBuilder) WithoutCircuitBreaker() *ConfigBuilder {
	b.config.CircuitBreakerEnabled = false
	return b
}

// WithRetry configures retry behavior.
func (b *ConfigBuilder) WithRetry(maxRetries int, initialBackoff, maxBackoff time.Duration) *ConfigBuilder {
	b.config.DefaultRetries = maxRetries
	b.config.RetryBackoff = initialBackoff
	b.config.MaxRetryBackoff = maxBackoff
	return b
}

// WithRateLimit configures rate limiting.
func (b *ConfigBuilder) WithRateLimit(rps float64, burst int) *ConfigBuilder {
	b.config.DefaultRateLimit = rps
	b.config.DefaultRateBurst = burst
	return b
}

// WithHealthCheck configures health checking.
func (b *ConfigBuilder) WithHealthCheck(interval, timeout time.Duration, healthyThreshold, unhealthyThreshold int) *ConfigBuilder {
	b.config.HealthCheckEnabled = true
	b.config.HealthCheckInterval = interval
	b.config.HealthCheckTimeout = timeout
	b.config.HealthyThreshold = healthyThreshold
	b.config.UnhealthyThreshold = unhealthyThreshold
	return b
}

// WithoutHealthCheck disables health checking.
func (b *ConfigBuilder) WithoutHealthCheck() *ConfigBuilder {
	b.config.HealthCheckEnabled = false
	return b
}

// WithMetrics enables or disables metrics.
func (b *ConfigBuilder) WithMetrics(enabled bool) *ConfigBuilder {
	b.config.MetricsEnabled = enabled
	return b
}

// WithTracing configures distributed tracing.
func (b *ConfigBuilder) WithTracing(enabled bool, sampleRate float64) *ConfigBuilder {
	b.config.TracingEnabled = enabled
	b.config.TraceSampleRate = sampleRate
	return b
}

// WithAccessLog enables or disables access logging.
func (b *ConfigBuilder) WithAccessLog(enabled bool) *ConfigBuilder {
	b.config.AccessLogEnabled = enabled
	return b
}

// WithHTTP2 enables or disables HTTP/2.
func (b *ConfigBuilder) WithHTTP2(enabled bool) *ConfigBuilder {
	b.config.EnableHTTP2 = enabled
	return b
}

// WithGRPC enables or disables gRPC support.
func (b *ConfigBuilder) WithGRPC(enabled bool) *ConfigBuilder {
	b.config.EnableGRPC = enabled
	return b
}

// WithGracefulShutdown sets the graceful shutdown timeout.
func (b *ConfigBuilder) WithGracefulShutdown(timeout time.Duration) *ConfigBuilder {
	b.config.GracefulShutdown = timeout
	return b
}

// Build builds and validates the configuration.
func (b *ConfigBuilder) Build() (*Config, error) {
	if b.err != nil {
		return nil, b.err
	}

	if err := b.config.Validate(); err != nil {
		return nil, err
	}

	return b.config.Clone(), nil
}

// MustBuild builds the configuration, panicking on error.
func (b *ConfigBuilder) MustBuild() *Config {
	config, err := b.Build()
	if err != nil {
		panic(err)
	}
	return config
}

// ServiceConfig holds per-service configuration overrides.
type ServiceConfig struct {
	// Service identity
	ServiceName string
	Namespace   string

	// Timeouts (override global)
	ConnectTimeout *time.Duration
	RequestTimeout *time.Duration

	// Load Balancing (override global)
	LoadBalancerType *string

	// Circuit Breaker (override global)
	CircuitBreakerEnabled          *bool
	CircuitBreakerFailureThreshold *int
	CircuitBreakerSuccessThreshold *int
	CircuitBreakerTimeout          *time.Duration

	// Retry (override global)
	MaxRetries  *int
	RetryBackoff *time.Duration

	// Rate Limiting (override global)
	RateLimit *float64
	RateBurst *int

	// Health Checking (override global)
	HealthCheckInterval *time.Duration
	HealthCheckTimeout  *time.Duration
	HealthyThreshold    *int
	UnhealthyThreshold  *int

	// Routing
	TrafficPolicy *TrafficPolicy
}

// TrafficPolicy defines traffic routing rules.
type TrafficPolicy struct {
	// Weighted routing
	WeightedBackends []WeightedBackend

	// Header-based routing
	HeaderRoutes []HeaderRoute

	// Retry policy
	RetryOn []string // HTTP status codes to retry on

	// Fault injection (for testing)
	FaultInjection *FaultInjection
}

// WeightedBackend defines a weighted backend for traffic splitting.
type WeightedBackend struct {
	Service   string
	Namespace string
	Weight    int
	Version   string
}

// HeaderRoute defines a header-based routing rule.
type HeaderRoute struct {
	Header  string
	Match   string // exact, prefix, regex
	Value   string
	Backend string
}

// FaultInjection configures fault injection for testing.
type FaultInjection struct {
	// Delay injection
	DelayEnabled    bool
	DelayDuration   time.Duration
	DelayPercentage float64

	// Abort injection
	AbortEnabled    bool
	AbortStatusCode int
	AbortPercentage float64
}

// Apply applies service-specific configuration to a base config.
func (sc *ServiceConfig) Apply(base *Config) *Config {
	config := base.Clone()

	if sc.ConnectTimeout != nil {
		config.ConnectTimeout = *sc.ConnectTimeout
	}
	if sc.RequestTimeout != nil {
		config.RequestTimeout = *sc.RequestTimeout
	}
	if sc.LoadBalancerType != nil {
		config.LoadBalancerType = *sc.LoadBalancerType
	}
	if sc.CircuitBreakerEnabled != nil {
		config.CircuitBreakerEnabled = *sc.CircuitBreakerEnabled
	}
	if sc.CircuitBreakerFailureThreshold != nil {
		config.CircuitBreakerFailureThreshold = *sc.CircuitBreakerFailureThreshold
	}
	if sc.CircuitBreakerSuccessThreshold != nil {
		config.CircuitBreakerSuccessThreshold = *sc.CircuitBreakerSuccessThreshold
	}
	if sc.CircuitBreakerTimeout != nil {
		config.CircuitBreakerTimeout = *sc.CircuitBreakerTimeout
	}
	if sc.MaxRetries != nil {
		config.DefaultRetries = *sc.MaxRetries
	}
	if sc.RetryBackoff != nil {
		config.RetryBackoff = *sc.RetryBackoff
	}
	if sc.RateLimit != nil {
		config.DefaultRateLimit = *sc.RateLimit
	}
	if sc.RateBurst != nil {
		config.DefaultRateBurst = *sc.RateBurst
	}
	if sc.HealthCheckInterval != nil {
		config.HealthCheckInterval = *sc.HealthCheckInterval
	}
	if sc.HealthCheckTimeout != nil {
		config.HealthCheckTimeout = *sc.HealthCheckTimeout
	}
	if sc.HealthyThreshold != nil {
		config.HealthyThreshold = *sc.HealthyThreshold
	}
	if sc.UnhealthyThreshold != nil {
		config.UnhealthyThreshold = *sc.UnhealthyThreshold
	}

	return config
}

// ConfigStore manages service-specific configurations.
type ConfigStore struct {
	mu       sync.RWMutex
	global   *Config
	services map[string]*ServiceConfig
}

// NewConfigStore creates a new configuration store.
func NewConfigStore(global *Config) *ConfigStore {
	if global == nil {
		global = DefaultConfig()
	}

	return &ConfigStore{
		global:   global,
		services: make(map[string]*ServiceConfig),
	}
}

// GetGlobal returns the global configuration.
func (s *ConfigStore) GetGlobal() *Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.global.Clone()
}

// SetGlobal sets the global configuration.
func (s *ConfigStore) SetGlobal(config *Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.global = config.Clone()
}

// GetService returns the effective configuration for a service.
func (s *ConfigStore) GetService(serviceName string) *Config {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sc, ok := s.services[serviceName]
	if !ok {
		return s.global.Clone()
	}

	return sc.Apply(s.global)
}

// SetService sets service-specific configuration.
func (s *ConfigStore) SetService(serviceName string, config *ServiceConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.services[serviceName] = config
}

// RemoveService removes service-specific configuration.
func (s *ConfigStore) RemoveService(serviceName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.services, serviceName)
}

// ListServices returns all services with custom configurations.
func (s *ConfigStore) ListServices() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	services := make([]string, 0, len(s.services))
	for svc := range s.services {
		services = append(services, svc)
	}
	return services
}

// Environment-based configuration helpers.

// ConfigFromEnv creates a configuration from environment variables.
// This is a placeholder - in production, it would read from environment.
func ConfigFromEnv() (*Config, error) {
	config := DefaultConfig()

	// In production, this would read from environment variables:
	// MESH_SERVICE_NAME, MESH_NAMESPACE, MESH_DISCOVERY_TYPE, etc.

	return config, nil
}

// ConfigFromFile loads configuration from a file.
// This is a placeholder - in production, it would support YAML, JSON, TOML.
func ConfigFromFile(path string) (*Config, error) {
	config := DefaultConfig()

	// In production, this would:
	// 1. Detect file format from extension
	// 2. Parse the file
	// 3. Populate the config struct

	_ = path // Unused for now

	return config, nil
}
