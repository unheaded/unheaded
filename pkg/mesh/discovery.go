// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package mesh provides THE HAUBERK - the service mesh implementation for the Unheaded Kingdom.
// This file implements service registry and discovery mechanisms.
package mesh

import (
	"context"
	"errors"
	"net"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Discovery type constants.
const (
	DiscoveryTypeDNS        = "dns"
	DiscoveryTypeRegistry   = "registry"
	DiscoveryTypeStatic     = "static"
	DiscoveryTypeKubernetes = "kubernetes"
)

// Discovery errors.
var (
	ErrDiscoveryNotStarted = errors.New("discovery not started")
	ErrDiscoveryStopped    = errors.New("discovery stopped")
	ErrEmptyServiceName    = errors.New("service name cannot be empty")
)

// Endpoint represents a service endpoint.
type Endpoint struct {
	ID        string            // Unique identifier
	Address   string            // IP address or hostname
	Port      int               // Port number
	Weight    int               // Weight for weighted load balancing
	Zone      string            // Availability zone
	Region    string            // Region
	Tags      map[string]string // Metadata tags
	Protocol  string            // Protocol (http, grpc, tcp)
	TLS       bool              // Whether TLS is enabled
	LastSeen  time.Time         // Last time endpoint was seen
	CreatedAt time.Time         // When endpoint was created
}

// ServiceInstance represents a registered service instance.
type ServiceInstance struct {
	ServiceName string            // Service name
	Namespace   string            // Namespace
	Version     string            // Service version
	Endpoints   []*Endpoint       // Service endpoints
	Protocol    string            // Protocol (http, grpc, tcp)
	Meta        map[string]string // Service metadata
	UpdatedAt   time.Time         // Last update time
}

// ServicePolicy defines traffic policy for a service.
type ServicePolicy struct {
	MaxRetries       int           // Maximum retry attempts
	Timeout          time.Duration // Request timeout
	RateLimitEnabled bool          // Whether rate limiting is enabled
	RateLimit        float64       // Requests per second
	CircuitBreaker   bool          // Whether circuit breaker is enabled
	LoadBalancer     string        // Load balancer type
	HealthCheck      *HealthCheckPolicy
}

// HealthCheckPolicy defines health check configuration.
type HealthCheckPolicy struct {
	Interval           time.Duration
	Timeout            time.Duration
	HealthyThreshold   int
	UnhealthyThreshold int
	Path               string // For HTTP health checks
	Port               int    // Health check port (0 = use service port)
}

// Discovery is the interface for service discovery backends.
type Discovery interface {
	// Discover returns endpoints for a service.
	Discover(ctx context.Context, service, namespace string) ([]*Endpoint, error)
	// Watch returns a channel that receives endpoint updates.
	Watch(ctx context.Context, service, namespace string) (<-chan []*Endpoint, error)
	// Start starts the discovery backend.
	Start(ctx context.Context) error
	// Stop stops the discovery backend.
	Stop() error
}

// ServiceRegistry maintains local service state and policies.
type ServiceRegistry struct {
	mu       sync.RWMutex
	services map[string]*ServiceInstance
	policies map[string]*ServicePolicy
	watchers map[string][]chan<- *ServiceInstance
}

// NewServiceRegistry creates a new service registry.
func NewServiceRegistry() *ServiceRegistry {
	return &ServiceRegistry{
		services: make(map[string]*ServiceInstance),
		policies: make(map[string]*ServicePolicy),
		watchers: make(map[string][]chan<- *ServiceInstance),
	}
}

// Register registers a service instance.
func (r *ServiceRegistry) Register(svc *ServiceInstance) {
	r.mu.Lock()
	defer r.mu.Unlock()

	svc.UpdatedAt = time.Now()
	r.services[svc.ServiceName] = svc

	// Notify watchers
	if watchers, ok := r.watchers[svc.ServiceName]; ok {
		for _, ch := range watchers {
			select {
			case ch <- svc:
			default:
				// Channel full, skip
			}
		}
	}
}

// Deregister removes a service.
func (r *ServiceRegistry) Deregister(serviceName string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.services, serviceName)
	delete(r.policies, serviceName)

	// Close watcher channels
	if watchers, ok := r.watchers[serviceName]; ok {
		for _, ch := range watchers {
			close(ch)
		}
		delete(r.watchers, serviceName)
	}
}

// Get retrieves a service by name.
func (r *ServiceRegistry) Get(serviceName string) (*ServiceInstance, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	svc, ok := r.services[serviceName]
	return svc, ok
}

// List returns all registered services.
func (r *ServiceRegistry) List() []*ServiceInstance {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*ServiceInstance, 0, len(r.services))
	for _, svc := range r.services {
		result = append(result, svc)
	}
	return result
}

// UpdateEndpoints updates endpoints for a service.
func (r *ServiceRegistry) UpdateEndpoints(serviceName string, endpoints []*Endpoint) {
	r.mu.Lock()
	defer r.mu.Unlock()

	svc, ok := r.services[serviceName]
	if !ok {
		return
	}

	svc.Endpoints = endpoints
	svc.UpdatedAt = time.Now()

	// Notify watchers
	if watchers, ok := r.watchers[serviceName]; ok {
		for _, ch := range watchers {
			select {
			case ch <- svc:
			default:
			}
		}
	}
}

// SetPolicy sets the policy for a service.
func (r *ServiceRegistry) SetPolicy(serviceName string, policy *ServicePolicy) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.policies[serviceName] = policy
}

// GetPolicy returns the policy for a service.
func (r *ServiceRegistry) GetPolicy(serviceName string) (*ServicePolicy, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	policy, ok := r.policies[serviceName]
	return policy, ok
}

// Watch registers a watcher for service updates.
func (r *ServiceRegistry) Watch(serviceName string) <-chan *ServiceInstance {
	r.mu.Lock()
	defer r.mu.Unlock()

	ch := make(chan *ServiceInstance, 10)
	r.watchers[serviceName] = append(r.watchers[serviceName], ch)

	// Send current state if exists
	if svc, ok := r.services[serviceName]; ok {
		ch <- svc
	}

	return ch
}

// DNSDiscoveryConfig configures DNS-based discovery.
type DNSDiscoveryConfig struct {
	Domain      string        // Base domain for service discovery
	RefreshRate time.Duration // How often to refresh DNS records
	Resolver    string        // Custom DNS resolver (empty = system default)
	SRV         bool          // Use SRV records
}

// DNSDiscovery implements Discovery using DNS.
type DNSDiscovery struct {
	config   *DNSDiscoveryConfig
	resolver *net.Resolver
	cache    map[string]*dnsCache
	cacheMu  sync.RWMutex
	started  int32
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

type dnsCache struct {
	endpoints []*Endpoint
	expiresAt time.Time
}

// NewDNSDiscovery creates a new DNS discovery backend.
func NewDNSDiscovery(config *DNSDiscoveryConfig) *DNSDiscovery {
	if config.RefreshRate == 0 {
		config.RefreshRate = 30 * time.Second
	}

	var resolver *net.Resolver
	if config.Resolver != "" {
		resolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{
					Timeout: 5 * time.Second,
				}
				return d.DialContext(ctx, network, config.Resolver)
			},
		}
	}

	return &DNSDiscovery{
		config:   config,
		resolver: resolver,
		cache:    make(map[string]*dnsCache),
	}
}

// Start starts the DNS discovery.
func (d *DNSDiscovery) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&d.started, 0, 1) {
		return nil
	}

	d.ctx, d.cancel = context.WithCancel(ctx)
	return nil
}

// Stop stops the DNS discovery.
func (d *DNSDiscovery) Stop() error {
	if atomic.CompareAndSwapInt32(&d.started, 1, 0) {
		d.cancel()
		d.wg.Wait()
	}
	return nil
}

// Discover returns endpoints for a service using DNS.
func (d *DNSDiscovery) Discover(ctx context.Context, service, namespace string) ([]*Endpoint, error) {
	if atomic.LoadInt32(&d.started) == 0 {
		return nil, ErrDiscoveryNotStarted
	}

	if service == "" {
		return nil, ErrEmptyServiceName
	}

	// Build DNS name
	dnsName := service
	if namespace != "" {
		dnsName = service + "." + namespace
	}
	if d.config.Domain != "" {
		dnsName = dnsName + "." + d.config.Domain
	}

	// Check cache
	cacheKey := namespace + "/" + service
	d.cacheMu.RLock()
	if entry, ok := d.cache[cacheKey]; ok && time.Now().Before(entry.expiresAt) {
		endpoints := entry.endpoints
		d.cacheMu.RUnlock()
		return endpoints, nil
	}
	d.cacheMu.RUnlock()

	// Resolve DNS
	var endpoints []*Endpoint
	var err error

	if d.config.SRV {
		endpoints, err = d.resolveSRV(ctx, dnsName)
	} else {
		endpoints, err = d.resolveA(ctx, dnsName)
	}

	if err != nil {
		return nil, err
	}

	// Update cache
	d.cacheMu.Lock()
	d.cache[cacheKey] = &dnsCache{
		endpoints: endpoints,
		expiresAt: time.Now().Add(d.config.RefreshRate),
	}
	d.cacheMu.Unlock()

	return endpoints, nil
}

// resolveSRV resolves SRV records.
func (d *DNSDiscovery) resolveSRV(ctx context.Context, name string) ([]*Endpoint, error) {
	resolver := d.resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}

	_, srvs, err := resolver.LookupSRV(ctx, "", "", name)
	if err != nil {
		return nil, err
	}

	endpoints := make([]*Endpoint, 0, len(srvs))
	for _, srv := range srvs {
		// Resolve the target hostname
		addrs, err := resolver.LookupHost(ctx, srv.Target)
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			endpoints = append(endpoints, &Endpoint{
				ID:        addr + ":" + strconv.Itoa(int(srv.Port)),
				Address:   addr,
				Port:      int(srv.Port),
				Weight:    int(srv.Weight),
				LastSeen:  time.Now(),
				CreatedAt: time.Now(),
			})
		}
	}

	return endpoints, nil
}

// resolveA resolves A/AAAA records.
func (d *DNSDiscovery) resolveA(ctx context.Context, name string) ([]*Endpoint, error) {
	resolver := d.resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}

	addrs, err := resolver.LookupHost(ctx, name)
	if err != nil {
		return nil, err
	}

	endpoints := make([]*Endpoint, 0, len(addrs))
	for _, addr := range addrs {
		endpoints = append(endpoints, &Endpoint{
			ID:        addr + ":80",
			Address:   addr,
			Port:      80, // Default port
			Weight:    1,
			LastSeen:  time.Now(),
			CreatedAt: time.Now(),
		})
	}

	return endpoints, nil
}

// Watch returns a channel for endpoint updates.
func (d *DNSDiscovery) Watch(ctx context.Context, service, namespace string) (<-chan []*Endpoint, error) {
	if atomic.LoadInt32(&d.started) == 0 {
		return nil, ErrDiscoveryNotStarted
	}

	ch := make(chan []*Endpoint, 10)

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer close(ch)

		ticker := time.NewTicker(d.config.RefreshRate)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-d.ctx.Done():
				return
			case <-ticker.C:
				endpoints, err := d.Discover(ctx, service, namespace)
				if err == nil {
					select {
					case ch <- endpoints:
					default:
					}
				}
			}
		}
	}()

	return ch, nil
}

// RegistryDiscoveryConfig configures registry-based discovery.
type RegistryDiscoveryConfig struct {
	Endpoints   []string      // Registry endpoints
	RefreshRate time.Duration // How often to refresh
	Timeout     time.Duration // Request timeout
}

// RegistryDiscovery implements Discovery using a service registry.
type RegistryDiscovery struct {
	config    *RegistryDiscoveryConfig
	cache     map[string]*registryCache
	cacheMu   sync.RWMutex
	started   int32
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	endpoints []string
	epIndex   uint32
}

type registryCache struct {
	endpoints []*Endpoint
	expiresAt time.Time
}

// NewRegistryDiscovery creates a new registry discovery backend.
func NewRegistryDiscovery(config *RegistryDiscoveryConfig) *RegistryDiscovery {
	if config.RefreshRate == 0 {
		config.RefreshRate = 30 * time.Second
	}
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}

	return &RegistryDiscovery{
		config:    config,
		cache:     make(map[string]*registryCache),
		endpoints: config.Endpoints,
	}
}

// Start starts the registry discovery.
func (r *RegistryDiscovery) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&r.started, 0, 1) {
		return nil
	}

	r.ctx, r.cancel = context.WithCancel(ctx)
	return nil
}

// Stop stops the registry discovery.
func (r *RegistryDiscovery) Stop() error {
	if atomic.CompareAndSwapInt32(&r.started, 1, 0) {
		r.cancel()
		r.wg.Wait()
	}
	return nil
}

// Discover returns endpoints for a service from the registry.
func (r *RegistryDiscovery) Discover(ctx context.Context, service, namespace string) ([]*Endpoint, error) {
	if atomic.LoadInt32(&r.started) == 0 {
		return nil, ErrDiscoveryNotStarted
	}

	if service == "" {
		return nil, ErrEmptyServiceName
	}

	// Check cache
	cacheKey := namespace + "/" + service
	r.cacheMu.RLock()
	if entry, ok := r.cache[cacheKey]; ok && time.Now().Before(entry.expiresAt) {
		endpoints := entry.endpoints
		r.cacheMu.RUnlock()
		return endpoints, nil
	}
	r.cacheMu.RUnlock()

	// Fetch from registry (simplified - in production would make HTTP/gRPC call)
	// For now, return empty with error indicating registry not available
	// This would be replaced with actual registry client logic

	return nil, ErrServiceNotFound
}

// Watch returns a channel for endpoint updates from registry.
func (r *RegistryDiscovery) Watch(ctx context.Context, service, namespace string) (<-chan []*Endpoint, error) {
	if atomic.LoadInt32(&r.started) == 0 {
		return nil, ErrDiscoveryNotStarted
	}

	ch := make(chan []*Endpoint, 10)

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		defer close(ch)

		ticker := time.NewTicker(r.config.RefreshRate)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-r.ctx.Done():
				return
			case <-ticker.C:
				endpoints, err := r.Discover(ctx, service, namespace)
				if err == nil {
					select {
					case ch <- endpoints:
					default:
					}
				}
			}
		}
	}()

	return ch, nil
}

// StaticDiscovery implements Discovery with static configuration.
type StaticDiscovery struct {
	mu       sync.RWMutex
	services map[string][]*Endpoint
	watchers map[string][]chan []*Endpoint
	started  int32
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewStaticDiscovery creates a new static discovery backend.
func NewStaticDiscovery() *StaticDiscovery {
	return &StaticDiscovery{
		services: make(map[string][]*Endpoint),
		watchers: make(map[string][]chan []*Endpoint),
	}
}

// Start starts the static discovery.
func (s *StaticDiscovery) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&s.started, 0, 1) {
		return nil
	}

	s.ctx, s.cancel = context.WithCancel(ctx)
	return nil
}

// Stop stops the static discovery.
func (s *StaticDiscovery) Stop() error {
	if atomic.CompareAndSwapInt32(&s.started, 1, 0) {
		s.cancel()

		s.mu.Lock()
		for _, watchers := range s.watchers {
			for _, ch := range watchers {
				close(ch)
			}
		}
		s.watchers = make(map[string][]chan []*Endpoint)
		s.mu.Unlock()
	}
	return nil
}

// AddService adds endpoints for a service.
func (s *StaticDiscovery) AddService(service, namespace string, endpoints []*Endpoint) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := namespace + "/" + service
	s.services[key] = endpoints

	// Notify watchers
	if watchers, ok := s.watchers[key]; ok {
		for _, ch := range watchers {
			select {
			case ch <- endpoints:
			default:
			}
		}
	}
}

// RemoveService removes a service.
func (s *StaticDiscovery) RemoveService(service, namespace string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := namespace + "/" + service
	delete(s.services, key)
}

// Discover returns endpoints for a service.
func (s *StaticDiscovery) Discover(ctx context.Context, service, namespace string) ([]*Endpoint, error) {
	if atomic.LoadInt32(&s.started) == 0 {
		return nil, ErrDiscoveryNotStarted
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	key := namespace + "/" + service
	if endpoints, ok := s.services[key]; ok {
		return endpoints, nil
	}
	return nil, ErrServiceNotFound
}

// Watch returns a channel for endpoint updates.
func (s *StaticDiscovery) Watch(ctx context.Context, service, namespace string) (<-chan []*Endpoint, error) {
	if atomic.LoadInt32(&s.started) == 0 {
		return nil, ErrDiscoveryNotStarted
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := namespace + "/" + service
	ch := make(chan []*Endpoint, 10)
	s.watchers[key] = append(s.watchers[key], ch)

	// Send current state
	if endpoints, ok := s.services[key]; ok {
		ch <- endpoints
	}

	return ch, nil
}

// KubernetesDiscoveryConfig configures Kubernetes-based discovery.
type KubernetesDiscoveryConfig struct {
	Namespace   string        // Kubernetes namespace
	RefreshRate time.Duration // How often to refresh
	LabelSelect string        // Label selector
}

// KubernetesDiscovery implements Discovery using Kubernetes API.
type KubernetesDiscovery struct {
	config  *KubernetesDiscoveryConfig
	cache   map[string]*k8sCache
	cacheMu sync.RWMutex
	started int32
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

type k8sCache struct {
	endpoints []*Endpoint
	expiresAt time.Time
}

// NewKubernetesDiscovery creates a new Kubernetes discovery backend.
func NewKubernetesDiscovery(config *KubernetesDiscoveryConfig) *KubernetesDiscovery {
	if config.RefreshRate == 0 {
		config.RefreshRate = 30 * time.Second
	}

	return &KubernetesDiscovery{
		config: config,
		cache:  make(map[string]*k8sCache),
	}
}

// Start starts the Kubernetes discovery.
func (k *KubernetesDiscovery) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&k.started, 0, 1) {
		return nil
	}

	k.ctx, k.cancel = context.WithCancel(ctx)
	return nil
}

// Stop stops the Kubernetes discovery.
func (k *KubernetesDiscovery) Stop() error {
	if atomic.CompareAndSwapInt32(&k.started, 1, 0) {
		k.cancel()
		k.wg.Wait()
	}
	return nil
}

// Discover returns endpoints for a Kubernetes service.
func (k *KubernetesDiscovery) Discover(ctx context.Context, service, namespace string) ([]*Endpoint, error) {
	if atomic.LoadInt32(&k.started) == 0 {
		return nil, ErrDiscoveryNotStarted
	}

	if namespace == "" {
		namespace = k.config.Namespace
	}

	// Check cache
	cacheKey := namespace + "/" + service
	k.cacheMu.RLock()
	if entry, ok := k.cache[cacheKey]; ok && time.Now().Before(entry.expiresAt) {
		endpoints := entry.endpoints
		k.cacheMu.RUnlock()
		return endpoints, nil
	}
	k.cacheMu.RUnlock()

	// In production, this would use the Kubernetes API
	// For now, fall back to DNS-based discovery (Kubernetes internal DNS)
	dnsName := service + "." + namespace + ".svc.cluster.local"

	addrs, err := net.LookupHost(dnsName)
	if err != nil {
		return nil, err
	}

	endpoints := make([]*Endpoint, 0, len(addrs))
	for _, addr := range addrs {
		endpoints = append(endpoints, &Endpoint{
			ID:        addr + ":80",
			Address:   addr,
			Port:      80,
			Weight:    1,
			LastSeen:  time.Now(),
			CreatedAt: time.Now(),
		})
	}

	// Update cache
	k.cacheMu.Lock()
	k.cache[cacheKey] = &k8sCache{
		endpoints: endpoints,
		expiresAt: time.Now().Add(k.config.RefreshRate),
	}
	k.cacheMu.Unlock()

	return endpoints, nil
}

// Watch returns a channel for endpoint updates.
func (k *KubernetesDiscovery) Watch(ctx context.Context, service, namespace string) (<-chan []*Endpoint, error) {
	if atomic.LoadInt32(&k.started) == 0 {
		return nil, ErrDiscoveryNotStarted
	}

	ch := make(chan []*Endpoint, 10)

	k.wg.Add(1)
	go func() {
		defer k.wg.Done()
		defer close(ch)

		ticker := time.NewTicker(k.config.RefreshRate)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-k.ctx.Done():
				return
			case <-ticker.C:
				endpoints, err := k.Discover(ctx, service, namespace)
				if err == nil {
					select {
					case ch <- endpoints:
					default:
					}
				}
			}
		}
	}()

	return ch, nil
}

// CachingDiscovery wraps a Discovery with caching.
type CachingDiscovery struct {
	backend     Discovery
	cache       map[string]*cacheEntry
	cacheMu     sync.RWMutex
	ttl         time.Duration
	negativeTTL time.Duration // TTL for negative cache entries
}

type cacheEntry struct {
	endpoints []*Endpoint
	err       error
	expiresAt time.Time
}

// NewCachingDiscovery creates a new caching discovery wrapper.
func NewCachingDiscovery(backend Discovery, ttl time.Duration) *CachingDiscovery {
	return &CachingDiscovery{
		backend:     backend,
		cache:       make(map[string]*cacheEntry),
		ttl:         ttl,
		negativeTTL: ttl / 4, // Shorter TTL for negative entries
	}
}

// Start starts the underlying discovery.
func (c *CachingDiscovery) Start(ctx context.Context) error {
	return c.backend.Start(ctx)
}

// Stop stops the underlying discovery.
func (c *CachingDiscovery) Stop() error {
	return c.backend.Stop()
}

// Discover returns cached endpoints or fetches from backend.
func (c *CachingDiscovery) Discover(ctx context.Context, service, namespace string) ([]*Endpoint, error) {
	key := namespace + "/" + service

	// Check cache
	c.cacheMu.RLock()
	entry, ok := c.cache[key]
	c.cacheMu.RUnlock()

	if ok && time.Now().Before(entry.expiresAt) {
		if entry.err != nil {
			return nil, entry.err
		}
		return entry.endpoints, nil
	}

	// Cache miss, fetch from backend
	endpoints, err := c.backend.Discover(ctx, service, namespace)

	// Update cache
	c.cacheMu.Lock()
	if err != nil {
		c.cache[key] = &cacheEntry{
			err:       err,
			expiresAt: time.Now().Add(c.negativeTTL),
		}
	} else {
		c.cache[key] = &cacheEntry{
			endpoints: endpoints,
			expiresAt: time.Now().Add(c.ttl),
		}
	}
	c.cacheMu.Unlock()

	return endpoints, err
}

// Watch delegates to the backend.
func (c *CachingDiscovery) Watch(ctx context.Context, service, namespace string) (<-chan []*Endpoint, error) {
	return c.backend.Watch(ctx, service, namespace)
}

// Invalidate removes a service from the cache.
func (c *CachingDiscovery) Invalidate(service, namespace string) {
	key := namespace + "/" + service
	c.cacheMu.Lock()
	delete(c.cache, key)
	c.cacheMu.Unlock()
}

// CompositeDiscovery tries multiple discovery backends.
type CompositeDiscovery struct {
	backends []Discovery
	started  int32
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewCompositeDiscovery creates a new composite discovery.
func NewCompositeDiscovery(backends ...Discovery) *CompositeDiscovery {
	return &CompositeDiscovery{
		backends: backends,
	}
}

// Start starts all backends.
func (c *CompositeDiscovery) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&c.started, 0, 1) {
		return nil
	}

	c.ctx, c.cancel = context.WithCancel(ctx)

	for _, backend := range c.backends {
		if err := backend.Start(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Stop stops all backends.
func (c *CompositeDiscovery) Stop() error {
	if atomic.CompareAndSwapInt32(&c.started, 1, 0) {
		c.cancel()

		var lastErr error
		for _, backend := range c.backends {
			if err := backend.Stop(); err != nil {
				lastErr = err
			}
		}
		return lastErr
	}
	return nil
}

// Discover tries each backend until one succeeds.
func (c *CompositeDiscovery) Discover(ctx context.Context, service, namespace string) ([]*Endpoint, error) {
	if atomic.LoadInt32(&c.started) == 0 {
		return nil, ErrDiscoveryNotStarted
	}

	var lastErr error
	for _, backend := range c.backends {
		endpoints, err := backend.Discover(ctx, service, namespace)
		if err == nil {
			return endpoints, nil
		}
		lastErr = err
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, ErrServiceNotFound
}

// Watch uses the first available backend.
func (c *CompositeDiscovery) Watch(ctx context.Context, service, namespace string) (<-chan []*Endpoint, error) {
	for _, backend := range c.backends {
		ch, err := backend.Watch(ctx, service, namespace)
		if err == nil {
			return ch, nil
		}
	}
	return nil, ErrDiscoveryStopped
}

// EndpointSorter provides utilities for sorting endpoints.
type EndpointSorter struct {
	endpoints []*Endpoint
	by        func(e1, e2 *Endpoint) bool
}

// Len returns the number of endpoints.
func (s *EndpointSorter) Len() int {
	return len(s.endpoints)
}

// Swap swaps two endpoints.
func (s *EndpointSorter) Swap(i, j int) {
	s.endpoints[i], s.endpoints[j] = s.endpoints[j], s.endpoints[i]
}

// Less compares two endpoints.
func (s *EndpointSorter) Less(i, j int) bool {
	return s.by(s.endpoints[i], s.endpoints[j])
}

// SortByWeight sorts endpoints by weight (descending).
func SortByWeight(endpoints []*Endpoint) {
	sort.Sort(&EndpointSorter{
		endpoints: endpoints,
		by: func(e1, e2 *Endpoint) bool {
			return e1.Weight > e2.Weight
		},
	})
}

// SortByLastSeen sorts endpoints by last seen time (most recent first).
func SortByLastSeen(endpoints []*Endpoint) {
	sort.Sort(&EndpointSorter{
		endpoints: endpoints,
		by: func(e1, e2 *Endpoint) bool {
			return e1.LastSeen.After(e2.LastSeen)
		},
	})
}

// SortByAddress sorts endpoints by address.
func SortByAddress(endpoints []*Endpoint) {
	sort.Sort(&EndpointSorter{
		endpoints: endpoints,
		by: func(e1, e2 *Endpoint) bool {
			return e1.Address < e2.Address
		},
	})
}

// FilterByZone filters endpoints by zone.
func FilterByZone(endpoints []*Endpoint, zone string) []*Endpoint {
	result := make([]*Endpoint, 0, len(endpoints))
	for _, ep := range endpoints {
		if ep.Zone == zone {
			result = append(result, ep)
		}
	}
	return result
}

// FilterByTag filters endpoints by a tag.
func FilterByTag(endpoints []*Endpoint, key, value string) []*Endpoint {
	result := make([]*Endpoint, 0, len(endpoints))
	for _, ep := range endpoints {
		if ep.Tags != nil && ep.Tags[key] == value {
			result = append(result, ep)
		}
	}
	return result
}

