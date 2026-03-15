// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package loadbalancer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"unheaded/pkg/metrics"
)

// Balancer is the main load balancer that orchestrates all components.
type Balancer struct {
	config *Config
	pool   *BackendPool
	health *HealthAggregator

	// Proxies
	l4Proxy *L4Proxy
	l7Proxy *L7Proxy

	// State
	running  int32
	startTime time.Time
	mu       sync.RWMutex

	// Metrics
	metricsRegistry *metrics.Registry
	requestsTotal   *metrics.CounterVec
	requestDuration *metrics.HistogramVec
	backendHealth   *metrics.GaugeVec
	activeConns     *metrics.Gauge
	bytesTotal      *metrics.CounterVec

	// Event callbacks
	onStart      func()
	onStop       func()
	onBackendAdd func(backend *Backend)
	onBackendRemove func(backend *Backend)
	onBackendHealthChange func(backend *Backend, healthy bool)
}

// New creates a new load balancer from configuration.
func New(config *Config) (*Balancer, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Create backend pool
	pool, err := NewBackendPool(config)
	if err != nil {
		return nil, fmt.Errorf("create backend pool: %w", err)
	}

	// Create health checker
	health := NewHealthAggregator(config.HealthCheck, pool)

	b := &Balancer{
		config: config,
		pool:   pool,
		health: health,
	}

	// Setup metrics if enabled
	if config.MetricsEnabled {
		b.setupMetrics()
	}

	// Setup health callbacks
	health.OnHealthy(func(backend *Backend) {
		if b.onBackendHealthChange != nil {
			b.onBackendHealthChange(backend, true)
		}
		if b.backendHealth != nil {
			b.backendHealth.WithLabels(metrics.Labels{
				"backend": backend.Config.Name,
			}).Set(1)
		}
	})

	health.OnUnhealthy(func(backend *Backend, err error) {
		if b.onBackendHealthChange != nil {
			b.onBackendHealthChange(backend, false)
		}
		if b.backendHealth != nil {
			b.backendHealth.WithLabels(metrics.Labels{
				"backend": backend.Config.Name,
			}).Set(0)
		}
	})

	// Create appropriate proxy based on protocol
	switch config.Protocol {
	case ProtocolTCP, ProtocolUDP:
		l4Proxy, err := NewL4Proxy(config, pool, health)
		if err != nil {
			return nil, fmt.Errorf("create L4 proxy: %w", err)
		}
		b.l4Proxy = l4Proxy
		b.setupL4Callbacks()

	case ProtocolHTTP, ProtocolHTTPS:
		l7Proxy, err := NewL7Proxy(config, pool, health)
		if err != nil {
			return nil, fmt.Errorf("create L7 proxy: %w", err)
		}
		b.l7Proxy = l7Proxy
		b.setupL7Callbacks()
	}

	return b, nil
}

// setupMetrics initializes metrics collectors.
func (b *Balancer) setupMetrics() {
	b.metricsRegistry = metrics.NewRegistry()

	b.requestsTotal = metrics.NewCounterVec(
		"loadbalancer_requests_total",
		"Total number of requests processed",
		metrics.Labels{"balancer": b.config.Name},
		[]string{"backend", "status"},
	)

	b.requestDuration = metrics.NewHistogramVec(
		metrics.HistogramOpts{
			Name:    "loadbalancer_request_duration_seconds",
			Help:    "Request duration in seconds",
			Buckets: metrics.DefaultBuckets,
			ConstLabels: metrics.Labels{"balancer": b.config.Name},
		},
		[]string{"backend"},
	)

	b.backendHealth = metrics.NewGaugeVec(
		"loadbalancer_backend_health",
		"Backend health status (1=healthy, 0=unhealthy)",
		metrics.Labels{"balancer": b.config.Name},
		[]string{"backend"},
	)

	b.activeConns = metrics.NewGauge(
		"loadbalancer_active_connections",
		"Number of active connections",
		metrics.Labels{"balancer": b.config.Name},
	)

	b.bytesTotal = metrics.NewCounterVec(
		"loadbalancer_bytes_total",
		"Total bytes transferred",
		metrics.Labels{"balancer": b.config.Name},
		[]string{"direction"},
	)

	b.metricsRegistry.MustRegister(b.requestsTotal)
	b.metricsRegistry.MustRegister(b.requestDuration)
	b.metricsRegistry.MustRegister(b.backendHealth)
	b.metricsRegistry.MustRegister(b.activeConns)
	b.metricsRegistry.MustRegister(b.bytesTotal)

	// Initialize backend health gauges
	for _, backend := range b.pool.All() {
		b.backendHealth.WithLabels(metrics.Labels{
			"backend": backend.Config.Name,
		}).Set(0) // Unknown initially
	}
}

// setupL4Callbacks configures callbacks for L4 proxy.
func (b *Balancer) setupL4Callbacks() {
	if b.l4Proxy == nil {
		return
	}

	b.l4Proxy.OnConnect(func(clientAddr, backendAddr string) {
		if b.activeConns != nil {
			b.activeConns.Inc()
		}
	})

	b.l4Proxy.OnDisconnect(func(clientAddr, backendAddr string, duration time.Duration, bytesIn, bytesOut int64) {
		if b.activeConns != nil {
			b.activeConns.Dec()
		}
		if b.bytesTotal != nil {
			b.bytesTotal.WithLabels(metrics.Labels{"direction": "in"}).Add(float64(bytesIn))
			b.bytesTotal.WithLabels(metrics.Labels{"direction": "out"}).Add(float64(bytesOut))
		}
		if b.requestDuration != nil {
			// Find backend name from address
			for _, backend := range b.pool.All() {
				if backend.Config.Address == backendAddr {
					b.requestDuration.WithLabels(metrics.Labels{
						"backend": backend.Config.Name,
					}).Observe(duration.Seconds())
					break
				}
			}
		}
	})

	b.l4Proxy.OnError(func(clientAddr string, err error) {
		if b.requestsTotal != nil {
			b.requestsTotal.WithLabels(metrics.Labels{
				"backend": "error",
				"status":  "error",
			}).Inc()
		}
	})
}

// setupL7Callbacks configures callbacks for L7 proxy.
func (b *Balancer) setupL7Callbacks() {
	if b.l7Proxy == nil {
		return
	}

	b.l7Proxy.OnRequest(func(r *http.Request, backend *Backend) {
		if b.activeConns != nil {
			b.activeConns.Inc()
		}
	})

	b.l7Proxy.OnResponse(func(r *http.Request, backend *Backend, status int, duration time.Duration) {
		if b.activeConns != nil {
			b.activeConns.Dec()
		}
		if b.requestsTotal != nil {
			statusStr := fmt.Sprintf("%dxx", status/100)
			b.requestsTotal.WithLabels(metrics.Labels{
				"backend": backend.Config.Name,
				"status":  statusStr,
			}).Inc()
		}
		if b.requestDuration != nil {
			b.requestDuration.WithLabels(metrics.Labels{
				"backend": backend.Config.Name,
			}).Observe(duration.Seconds())
		}
	})

	b.l7Proxy.OnError(func(r *http.Request, err error) {
		if b.requestsTotal != nil {
			b.requestsTotal.WithLabels(metrics.Labels{
				"backend": "error",
				"status":  "error",
			}).Inc()
		}
	})
}

// Start starts the load balancer.
func (b *Balancer) Start() error {
	if !atomic.CompareAndSwapInt32(&b.running, 0, 1) {
		return errors.New("balancer already running")
	}

	b.startTime = time.Now()

	// Start health checker
	if err := b.health.Start(); err != nil {
		atomic.StoreInt32(&b.running, 0)
		return fmt.Errorf("start health checker: %w", err)
	}

	// Start proxy
	var err error
	switch b.config.Protocol {
	case ProtocolTCP, ProtocolUDP:
		err = b.l4Proxy.Start()
	case ProtocolHTTP, ProtocolHTTPS:
		err = b.l7Proxy.Start()
	}

	if err != nil {
		b.health.Stop()
		atomic.StoreInt32(&b.running, 0)
		return fmt.Errorf("start proxy: %w", err)
	}

	if b.onStart != nil {
		b.onStart()
	}

	return nil
}

// Stop stops the load balancer gracefully.
func (b *Balancer) Stop() error {
	if !atomic.CompareAndSwapInt32(&b.running, 1, 0) {
		return errors.New("balancer not running")
	}

	var errs []error

	// Stop proxy first to stop accepting new connections
	switch b.config.Protocol {
	case ProtocolTCP, ProtocolUDP:
		if err := b.l4Proxy.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("stop L4 proxy: %w", err))
		}
	case ProtocolHTTP, ProtocolHTTPS:
		if err := b.l7Proxy.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("stop L7 proxy: %w", err))
		}
	}

	// Stop health checker
	if err := b.health.Stop(); err != nil {
		errs = append(errs, fmt.Errorf("stop health checker: %w", err))
	}

	if b.onStop != nil {
		b.onStop()
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// IsRunning returns true if the balancer is running.
func (b *Balancer) IsRunning() bool {
	return atomic.LoadInt32(&b.running) == 1
}

// Uptime returns how long the balancer has been running.
func (b *Balancer) Uptime() time.Duration {
	if !b.IsRunning() {
		return 0
	}
	return time.Since(b.startTime)
}

// AddBackend adds a new backend to the pool.
func (b *Balancer) AddBackend(config BackendConfig) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if err := b.pool.Add(config); err != nil {
		return err
	}

	backend, _ := b.pool.Get(config.Name)

	// Initialize health metric
	if b.backendHealth != nil {
		b.backendHealth.WithLabels(metrics.Labels{
			"backend": config.Name,
		}).Set(0)
	}

	if b.onBackendAdd != nil {
		b.onBackendAdd(backend)
	}

	return nil
}

// RemoveBackend removes a backend from the pool.
func (b *Balancer) RemoveBackend(name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	backend, ok := b.pool.Get(name)
	if !ok {
		return ErrBackendNotFound
	}

	if err := b.pool.Remove(name); err != nil {
		return err
	}

	if b.onBackendRemove != nil {
		b.onBackendRemove(backend)
	}

	return nil
}

// GetBackend returns a backend by name.
func (b *Balancer) GetBackend(name string) (*Backend, bool) {
	return b.pool.Get(name)
}

// Backends returns all backends.
func (b *Balancer) Backends() []*Backend {
	return b.pool.All()
}

// HealthyBackends returns all healthy backends.
func (b *Balancer) HealthyBackends() []*Backend {
	return b.pool.Healthy()
}

// Stats returns comprehensive statistics.
func (b *Balancer) Stats() *BalancerStats {
	stats := &BalancerStats{
		Name:      b.config.Name,
		Running:   b.IsRunning(),
		Uptime:    b.Uptime(),
		Protocol:  b.config.Protocol,
		Algorithm: b.config.Algorithm,
		Pool:      b.pool.Stats(),
	}

	switch b.config.Protocol {
	case ProtocolTCP, ProtocolUDP:
		if b.l4Proxy != nil {
			stats.L4Stats = b.l4Proxy.Stats()
		}
	case ProtocolHTTP, ProtocolHTTPS:
		if b.l7Proxy != nil {
			stats.L7Stats = b.l7Proxy.Stats()
		}
	}

	return stats
}

// BalancerStats holds comprehensive balancer statistics.
type BalancerStats struct {
	Name      string
	Running   bool
	Uptime    time.Duration
	Protocol  Protocol
	Algorithm Algorithm
	Pool      PoolStats
	L4Stats   L4ProxyStats
	L7Stats   L7ProxyStats
}

// Config returns the balancer configuration.
func (b *Balancer) Config() *Config {
	return b.config
}

// SetAlgorithm changes the load balancing algorithm.
func (b *Balancer) SetAlgorithm(algorithm Algorithm) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch algorithm {
	case AlgorithmRoundRobin, AlgorithmLeastConnections, AlgorithmIPHash,
		AlgorithmWeighted, AlgorithmWeightedLeastConn, AlgorithmRandom:
		// valid
	default:
		return fmt.Errorf("invalid algorithm: %s", algorithm)
	}

	b.config.Algorithm = algorithm
	// Pool will use new algorithm on next selection

	return nil
}

// UpdateBackendWeight updates a backend's weight.
func (b *Balancer) UpdateBackendWeight(name string, weight int) error {
	return b.pool.UpdateWeight(name, weight)
}

// DrainBackend starts draining a backend.
func (b *Balancer) DrainBackend(name string) error {
	backend, ok := b.pool.Get(name)
	if !ok {
		return ErrBackendNotFound
	}

	backend.Drain()
	return nil
}

// EnableBackend enables a backend.
func (b *Balancer) EnableBackend(name string) error {
	backend, ok := b.pool.Get(name)
	if !ok {
		return ErrBackendNotFound
	}

	backend.Enable()
	return nil
}

// DisableBackend disables a backend.
func (b *Balancer) DisableBackend(name string) error {
	backend, ok := b.pool.Get(name)
	if !ok {
		return ErrBackendNotFound
	}

	backend.Disable()
	return nil
}

// MetricsHandler returns an HTTP handler for metrics.
func (b *Balancer) MetricsHandler() http.Handler {
	if b.metricsRegistry == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Metrics not enabled", http.StatusServiceUnavailable)
		})
	}
	return b.metricsRegistry.Handler()
}

// HealthHandler returns an HTTP handler for health endpoints.
func (b *Balancer) HealthHandler() http.Handler {
	return NewHealthEndpoint(b.pool)
}

// APIHandler returns an HTTP handler for the management API.
func (b *Balancer) APIHandler() http.Handler {
	return NewAPIHandler(b)
}

// OnStart sets the callback for when the balancer starts.
func (b *Balancer) OnStart(fn func()) {
	b.onStart = fn
}

// OnStop sets the callback for when the balancer stops.
func (b *Balancer) OnStop(fn func()) {
	b.onStop = fn
}

// OnBackendAdd sets the callback for when a backend is added.
func (b *Balancer) OnBackendAdd(fn func(backend *Backend)) {
	b.onBackendAdd = fn
}

// OnBackendRemove sets the callback for when a backend is removed.
func (b *Balancer) OnBackendRemove(fn func(backend *Backend)) {
	b.onBackendRemove = fn
}

// OnBackendHealthChange sets the callback for backend health changes.
func (b *Balancer) OnBackendHealthChange(fn func(backend *Backend, healthy bool)) {
	b.onBackendHealthChange = fn
}

// APIHandler provides a REST API for managing the load balancer.
type APIHandler struct {
	balancer *Balancer
}

// NewAPIHandler creates a new API handler.
func NewAPIHandler(balancer *Balancer) *APIHandler {
	return &APIHandler{balancer: balancer}
}

// ServeHTTP handles API requests.
func (h *APIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/stats":
		h.handleStats(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/backends":
		h.handleListBackends(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/backends":
		h.handleAddBackend(w, r)
	case r.Method == http.MethodDelete && r.URL.Path[:13] == "/api/backends/":
		h.handleRemoveBackend(w, r)
	case r.Method == http.MethodPost && r.URL.Path[:13] == "/api/backends/" && len(r.URL.Path) > 13:
		h.handleBackendAction(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/config":
		h.handleGetConfig(w, r)
	default:
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
	}
}

func (h *APIHandler) handleStats(w http.ResponseWriter, r *http.Request) {
	stats := h.balancer.Stats()
	json.NewEncoder(w).Encode(stats)
}

func (h *APIHandler) handleListBackends(w http.ResponseWriter, r *http.Request) {
	backends := h.balancer.Backends()
	result := make([]BackendStats, len(backends))
	for i, b := range backends {
		result[i] = b.Stats()
	}
	json.NewEncoder(w).Encode(result)
}

func (h *APIHandler) handleAddBackend(w http.ResponseWriter, r *http.Request) {
	var config BackendConfig
	// SECURITY: Enforce body size limit to prevent denial-of-service
	if err := json.NewDecoder(io.LimitReader(r.Body, 1*1024*1024)).Decode(&config); err != nil {
		// SECURITY: Use json.Encode for error messages to prevent JSON injection
		// via crafted error strings (e.g., containing quotes).
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if err := h.balancer.AddBackend(config); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "created"})
}

func (h *APIHandler) handleRemoveBackend(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Path[13:] // Remove "/api/backends/" prefix
	if err := h.balancer.RemoveBackend(name); err != nil {
		// SECURITY: Use json.Encode for error messages to prevent JSON injection
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "removed"})
}

func (h *APIHandler) handleBackendAction(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[13:] // Remove "/api/backends/" prefix
	parts := splitPath(path)
	if len(parts) != 2 {
		http.Error(w, `{"error":"invalid path"}`, http.StatusBadRequest)
		return
	}

	name, action := parts[0], parts[1]

	var err error
	switch action {
	case "drain":
		err = h.balancer.DrainBackend(name)
	case "enable":
		err = h.balancer.EnableBackend(name)
	case "disable":
		err = h.balancer.DisableBackend(name)
	default:
		http.Error(w, `{"error":"unknown action"}`, http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": action})
}

func (h *APIHandler) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(h.balancer.Config())
}

// splitPath splits a path into parts.
func splitPath(path string) []string {
	var parts []string
	var current string
	for _, c := range path {
		if c == '/' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

// BalancerGroup manages multiple load balancers.
type BalancerGroup struct {
	mu        sync.RWMutex
	balancers map[string]*Balancer
}

// NewBalancerGroup creates a new balancer group.
func NewBalancerGroup() *BalancerGroup {
	return &BalancerGroup{
		balancers: make(map[string]*Balancer),
	}
}

// Add adds a balancer to the group.
func (g *BalancerGroup) Add(balancer *Balancer) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	name := balancer.Config().Name
	if _, exists := g.balancers[name]; exists {
		return fmt.Errorf("balancer %s already exists", name)
	}

	g.balancers[name] = balancer
	return nil
}

// Remove removes a balancer from the group.
func (g *BalancerGroup) Remove(name string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	balancer, ok := g.balancers[name]
	if !ok {
		return fmt.Errorf("balancer %s not found", name)
	}

	if balancer.IsRunning() {
		if err := balancer.Stop(); err != nil {
			return err
		}
	}

	delete(g.balancers, name)
	return nil
}

// Get returns a balancer by name.
func (g *BalancerGroup) Get(name string) (*Balancer, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	b, ok := g.balancers[name]
	return b, ok
}

// All returns all balancers.
func (g *BalancerGroup) All() []*Balancer {
	g.mu.RLock()
	defer g.mu.RUnlock()

	result := make([]*Balancer, 0, len(g.balancers))
	for _, b := range g.balancers {
		result = append(result, b)
	}
	return result
}

// StartAll starts all balancers.
func (g *BalancerGroup) StartAll() error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	for name, b := range g.balancers {
		if err := b.Start(); err != nil {
			return fmt.Errorf("start %s: %w", name, err)
		}
	}
	return nil
}

// StopAll stops all balancers.
func (g *BalancerGroup) StopAll() error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var errs []error
	for name, b := range g.balancers {
		if err := b.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("stop %s: %w", name, err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// Stats returns statistics for all balancers.
func (g *BalancerGroup) Stats() map[string]*BalancerStats {
	g.mu.RLock()
	defer g.mu.RUnlock()

	stats := make(map[string]*BalancerStats)
	for name, b := range g.balancers {
		stats[name] = b.Stats()
	}
	return stats
}

// Builder provides a fluent API for building load balancers.
type Builder struct {
	config *Config
	errors []error
}

// NewBuilder creates a new builder with default configuration.
func NewBuilder(name string) *Builder {
	config := DefaultConfig()
	config.Name = name
	return &Builder{config: config}
}

// ListenAddress sets the listen address.
func (b *Builder) ListenAddress(addr string) *Builder {
	b.config.ListenAddress = addr
	return b
}

// Protocol sets the protocol.
func (b *Builder) Protocol(protocol Protocol) *Builder {
	b.config.Protocol = protocol
	return b
}

// Algorithm sets the load balancing algorithm.
func (b *Builder) Algorithm(algorithm Algorithm) *Builder {
	b.config.Algorithm = algorithm
	return b
}

// AddBackend adds a backend.
func (b *Builder) AddBackend(name, address string, weight int) *Builder {
	b.config.Backends = append(b.config.Backends, BackendConfig{
		Name:    name,
		Address: address,
		Weight:  weight,
		Enabled: true,
	})
	return b
}

// HealthCheck configures health checking.
func (b *Builder) HealthCheck(checkType HealthCheckType, interval, timeout time.Duration) *Builder {
	b.config.HealthCheck.Enabled = true
	b.config.HealthCheck.Type = checkType
	b.config.HealthCheck.Interval = interval
	b.config.HealthCheck.Timeout = timeout
	return b
}

// SessionPersistence configures session persistence.
func (b *Builder) SessionPersistence(persistenceType SessionPersistenceType, ttl time.Duration) *Builder {
	b.config.SessionPersistence.Type = persistenceType
	b.config.SessionPersistence.TTL = ttl
	return b
}

// TLS configures TLS termination.
func (b *Builder) TLS(certFile, keyFile string) *Builder {
	b.config.TLS = &TLSConfig{
		CertFile:   certFile,
		KeyFile:    keyFile,
		MinVersion: "1.2",
	}
	return b
}

// Timeouts configures timeouts.
func (b *Builder) Timeouts(connect, read, write time.Duration) *Builder {
	b.config.Timeouts.Connect = connect
	b.config.Timeouts.Read = read
	b.config.Timeouts.Write = write
	return b
}

// Metrics enables metrics collection.
func (b *Builder) Metrics(enabled bool) *Builder {
	b.config.MetricsEnabled = enabled
	return b
}

// Build creates the load balancer.
func (b *Builder) Build() (*Balancer, error) {
	if len(b.errors) > 0 {
		return nil, errors.Join(b.errors...)
	}
	return New(b.config)
}

// ConfigWatcher watches for configuration changes and applies them.
type ConfigWatcher struct {
	balancer     *Balancer
	configPath   string
	pollInterval time.Duration
	lastModified time.Time
	stopCh       chan struct{}
	running      int32
}

// NewConfigWatcher creates a new configuration watcher.
func NewConfigWatcher(balancer *Balancer, configPath string, pollInterval time.Duration) *ConfigWatcher {
	return &ConfigWatcher{
		balancer:     balancer,
		configPath:   configPath,
		pollInterval: pollInterval,
		stopCh:       make(chan struct{}),
	}
}

// Start starts watching for configuration changes.
func (cw *ConfigWatcher) Start() error {
	if !atomic.CompareAndSwapInt32(&cw.running, 0, 1) {
		return errors.New("watcher already running")
	}

	go cw.watch()
	return nil
}

// Stop stops watching.
func (cw *ConfigWatcher) Stop() error {
	if !atomic.CompareAndSwapInt32(&cw.running, 1, 0) {
		return errors.New("watcher not running")
	}

	close(cw.stopCh)
	return nil
}

func (cw *ConfigWatcher) watch() {
	ticker := time.NewTicker(cw.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-cw.stopCh:
			return
		case <-ticker.C:
			cw.checkConfig()
		}
	}
}

func (cw *ConfigWatcher) checkConfig() {
	// In production, would check file modification time and reload if changed
	// This is a placeholder implementation
}

// Reload reloads the configuration.
func (cw *ConfigWatcher) Reload() error {
	config, err := LoadConfig(cw.configPath)
	if err != nil {
		return err
	}

	// Apply changes to existing balancer
	// Note: Some changes require restart (listen address, protocol, TLS)
	// Other changes can be applied live (backends, algorithm, timeouts)

	// Update backends
	existingBackends := make(map[string]bool)
	for _, b := range cw.balancer.Backends() {
		existingBackends[b.Config.Name] = true
	}

	for _, bc := range config.Backends {
		if !existingBackends[bc.Name] {
			cw.balancer.AddBackend(bc)
		}
		delete(existingBackends, bc.Name)
	}

	// Remove backends that are no longer in config
	for name := range existingBackends {
		cw.balancer.RemoveBackend(name)
	}

	// Update algorithm
	if config.Algorithm != cw.balancer.Config().Algorithm {
		cw.balancer.SetAlgorithm(config.Algorithm)
	}

	return nil
}

// WriteMetrics writes metrics to a writer.
func (b *Balancer) WriteMetrics(w io.Writer) error {
	if b.metricsRegistry == nil {
		return errors.New("metrics not enabled")
	}
	return b.metricsRegistry.Gather(w)
}

// Select selects a backend using the current algorithm.
func (b *Balancer) Select(ctx context.Context, key string) (*Backend, error) {
	return b.pool.Select(ctx, key)
}

// RecordError records an error for a backend.
func (b *Balancer) RecordError(backend *Backend, err error) {
	b.health.RecordError(backend, err)
}
