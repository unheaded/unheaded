// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package loadbalancer

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// BackendState represents the health state of a backend.
type BackendState int32

const (
	// BackendStateUnknown is the initial state before health checks.
	BackendStateUnknown BackendState = iota
	// BackendStateHealthy indicates the backend is healthy.
	BackendStateHealthy
	// BackendStateUnhealthy indicates the backend is unhealthy.
	BackendStateUnhealthy
	// BackendStateDraining indicates the backend is draining connections.
	BackendStateDraining
	// BackendStateDisabled indicates the backend is administratively disabled.
	BackendStateDisabled
)

// String returns the string representation of BackendState.
func (s BackendState) String() string {
	switch s {
	case BackendStateUnknown:
		return "unknown"
	case BackendStateHealthy:
		return "healthy"
	case BackendStateUnhealthy:
		return "unhealthy"
	case BackendStateDraining:
		return "draining"
	case BackendStateDisabled:
		return "disabled"
	default:
		return "invalid"
	}
}

// Backend represents a backend server with its state.
type Backend struct {
	// Config is the backend configuration.
	Config BackendConfig

	// state is the current health state (atomic).
	state int32

	// activeConnections is the current connection count (atomic).
	activeConnections int64

	// totalConnections is the total connections handled (atomic).
	totalConnections int64

	// totalRequests is the total requests handled (atomic).
	totalRequests int64

	// totalErrors is the total errors encountered (atomic).
	totalErrors int64

	// totalBytes is the total bytes transferred (atomic).
	totalBytes int64

	// lastHealthCheck is the time of the last health check.
	lastHealthCheck atomic.Value // time.Time

	// lastError is the last error encountered.
	lastError atomic.Value // error

	// consecutiveFailures counts consecutive health check failures.
	consecutiveFailures int32

	// consecutiveSuccesses counts consecutive health check successes.
	consecutiveSuccesses int32

	// mu protects non-atomic fields.
	mu sync.RWMutex

	// errors is a sliding window of recent errors for passive health.
	errors []errorEntry

	// tlsConfig is the TLS configuration for backend connections.
	tlsConfig *tls.Config

	// metrics for this backend.
	metrics *BackendMetrics
}

// errorEntry represents an error in the sliding window.
type errorEntry struct {
	time time.Time
	err  error
}

// BackendMetrics holds metrics for a backend.
type BackendMetrics struct {
	// ResponseTimes is a histogram of response times.
	ResponseTimes *ResponseTimeHistogram

	// RequestsPerSecond tracks request rate.
	RequestsPerSecond *RateCounter

	// ErrorsPerSecond tracks error rate.
	ErrorsPerSecond *RateCounter
}

// ResponseTimeHistogram tracks response time distribution.
type ResponseTimeHistogram struct {
	mu      sync.Mutex
	buckets []int64 // counts for each bucket
	bounds  []time.Duration
	sum     int64
	count   int64
}

// NewResponseTimeHistogram creates a new response time histogram.
func NewResponseTimeHistogram() *ResponseTimeHistogram {
	return &ResponseTimeHistogram{
		bounds: []time.Duration{
			1 * time.Millisecond,
			5 * time.Millisecond,
			10 * time.Millisecond,
			25 * time.Millisecond,
			50 * time.Millisecond,
			100 * time.Millisecond,
			250 * time.Millisecond,
			500 * time.Millisecond,
			1 * time.Second,
			5 * time.Second,
		},
		buckets: make([]int64, 11), // one extra for > max
	}
}

// Observe records a response time.
func (h *ResponseTimeHistogram) Observe(d time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.sum += int64(d)
	h.count++

	for i, bound := range h.bounds {
		if d <= bound {
			h.buckets[i]++
			return
		}
	}
	h.buckets[len(h.buckets)-1]++
}

// Stats returns histogram statistics.
func (h *ResponseTimeHistogram) Stats() (avg time.Duration, p50, p95, p99 time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.count == 0 {
		return 0, 0, 0, 0
	}

	avg = time.Duration(h.sum / h.count)

	// Calculate percentiles from buckets
	p50 = h.percentile(0.50)
	p95 = h.percentile(0.95)
	p99 = h.percentile(0.99)

	return
}

func (h *ResponseTimeHistogram) percentile(p float64) time.Duration {
	target := int64(float64(h.count) * p)
	var cumulative int64
	for i, count := range h.buckets {
		cumulative += count
		if cumulative >= target {
			if i < len(h.bounds) {
				return h.bounds[i]
			}
			return h.bounds[len(h.bounds)-1] * 2
		}
	}
	return 0
}

// RateCounter tracks rate per second.
type RateCounter struct {
	mu      sync.Mutex
	samples []rateSample
	window  time.Duration
}

type rateSample struct {
	time  time.Time
	count int64
}

// NewRateCounter creates a new rate counter.
func NewRateCounter(window time.Duration) *RateCounter {
	return &RateCounter{
		window:  window,
		samples: make([]rateSample, 0, 100),
	}
}

// Add increments the counter.
func (r *RateCounter) Add(n int64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	r.samples = append(r.samples, rateSample{time: now, count: n})
	r.cleanup(now)
}

// Rate returns the rate per second.
func (r *RateCounter) Rate() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	r.cleanup(now)

	var total int64
	for _, s := range r.samples {
		total += s.count
	}

	seconds := r.window.Seconds()
	if seconds == 0 {
		return 0
	}
	return float64(total) / seconds
}

func (r *RateCounter) cleanup(now time.Time) {
	cutoff := now.Add(-r.window)
	newStart := 0
	for i, s := range r.samples {
		if s.time.After(cutoff) {
			newStart = i
			break
		}
	}
	if newStart > 0 {
		r.samples = r.samples[newStart:]
	}
}

// NewBackend creates a new backend from configuration.
func NewBackend(config BackendConfig) (*Backend, error) {
	b := &Backend{
		Config: config,
		errors: make([]errorEntry, 0, 100),
		metrics: &BackendMetrics{
			ResponseTimes:     NewResponseTimeHistogram(),
			RequestsPerSecond: NewRateCounter(time.Minute),
			ErrorsPerSecond:   NewRateCounter(time.Minute),
		},
	}

	if config.Enabled {
		b.SetState(BackendStateUnknown)
	} else {
		b.SetState(BackendStateDisabled)
	}

	// Build TLS config if needed
	if config.TLS != nil && config.TLS.Enabled {
		tlsConfig, err := config.TLS.BuildTLSConfig()
		if err != nil {
			return nil, fmt.Errorf("build backend TLS config: %w", err)
		}
		b.tlsConfig = tlsConfig
	}

	return b, nil
}

// State returns the current backend state.
func (b *Backend) State() BackendState {
	return BackendState(atomic.LoadInt32(&b.state))
}

// SetState sets the backend state.
func (b *Backend) SetState(state BackendState) {
	atomic.StoreInt32(&b.state, int32(state))
}

// IsHealthy returns true if the backend can accept connections.
func (b *Backend) IsHealthy() bool {
	state := b.State()
	return state == BackendStateHealthy || state == BackendStateUnknown
}

// IsAvailable returns true if the backend is available for new connections.
func (b *Backend) IsAvailable() bool {
	if !b.IsHealthy() {
		return false
	}

	// Check connection limit
	if b.Config.MaxConnections > 0 {
		if b.ActiveConnections() >= int64(b.Config.MaxConnections) {
			return false
		}
	}

	return true
}

// ActiveConnections returns the current active connection count.
func (b *Backend) ActiveConnections() int64 {
	return atomic.LoadInt64(&b.activeConnections)
}

// IncrementConnections increments the active connection count.
func (b *Backend) IncrementConnections() {
	atomic.AddInt64(&b.activeConnections, 1)
	atomic.AddInt64(&b.totalConnections, 1)
}

// DecrementConnections decrements the active connection count.
func (b *Backend) DecrementConnections() {
	atomic.AddInt64(&b.activeConnections, -1)
}

// TotalConnections returns the total connection count.
func (b *Backend) TotalConnections() int64 {
	return atomic.LoadInt64(&b.totalConnections)
}

// TotalRequests returns the total request count.
func (b *Backend) TotalRequests() int64 {
	return atomic.LoadInt64(&b.totalRequests)
}

// IncrementRequests increments the request counter.
func (b *Backend) IncrementRequests() {
	atomic.AddInt64(&b.totalRequests, 1)
	b.metrics.RequestsPerSecond.Add(1)
}

// TotalErrors returns the total error count.
func (b *Backend) TotalErrors() int64 {
	return atomic.LoadInt64(&b.totalErrors)
}

// AddBytes adds to the total bytes transferred.
func (b *Backend) AddBytes(n int64) {
	atomic.AddInt64(&b.totalBytes, n)
}

// TotalBytes returns the total bytes transferred.
func (b *Backend) TotalBytes() int64 {
	return atomic.LoadInt64(&b.totalBytes)
}

// RecordResponseTime records a response time.
func (b *Backend) RecordResponseTime(d time.Duration) {
	b.metrics.ResponseTimes.Observe(d)
}

// RecordError records an error for passive health checking.
func (b *Backend) RecordError(err error) {
	atomic.AddInt64(&b.totalErrors, 1)
	b.metrics.ErrorsPerSecond.Add(1)
	b.lastError.Store(err)

	b.mu.Lock()
	defer b.mu.Unlock()

	b.errors = append(b.errors, errorEntry{
		time: time.Now(),
		err:  err,
	})
}

// ErrorCount returns the error count within the window.
func (b *Backend) ErrorCount(window time.Duration) int {
	b.mu.Lock()
	defer b.mu.Unlock()

	cutoff := time.Now().Add(-window)
	count := 0
	for _, e := range b.errors {
		if e.time.After(cutoff) {
			count++
		}
	}
	return count
}

// CleanupErrors removes old errors from the sliding window.
func (b *Backend) CleanupErrors(window time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()

	cutoff := time.Now().Add(-window)
	newStart := 0
	for i, e := range b.errors {
		if e.time.After(cutoff) {
			newStart = i
			break
		}
	}
	if newStart > 0 {
		b.errors = b.errors[newStart:]
	}
}

// SetHealthCheckResult records a health check result.
func (b *Backend) SetHealthCheckResult(healthy bool, healthConfig HealthCheckConfig) {
	b.lastHealthCheck.Store(time.Now())

	if healthy {
		atomic.StoreInt32(&b.consecutiveFailures, 0)
		newSuccesses := atomic.AddInt32(&b.consecutiveSuccesses, 1)

		// Check if we should mark as healthy
		if int(newSuccesses) >= healthConfig.SuccessThreshold {
			if b.State() != BackendStateDraining {
				b.SetState(BackendStateHealthy)
			}
		}
	} else {
		atomic.StoreInt32(&b.consecutiveSuccesses, 0)
		newFailures := atomic.AddInt32(&b.consecutiveFailures, 1)

		// Check if we should mark as unhealthy
		if int(newFailures) >= healthConfig.Retries {
			if b.State() != BackendStateDraining && b.State() != BackendStateDisabled {
				b.SetState(BackendStateUnhealthy)
			}
		}
	}
}

// LastHealthCheck returns the time of the last health check.
func (b *Backend) LastHealthCheck() time.Time {
	if v := b.lastHealthCheck.Load(); v != nil {
		return v.(time.Time)
	}
	return time.Time{}
}

// LastError returns the last error.
func (b *Backend) LastError() error {
	if v := b.lastError.Load(); v != nil {
		return v.(error)
	}
	return nil
}

// ConsecutiveFailures returns the consecutive failure count.
func (b *Backend) ConsecutiveFailures() int {
	return int(atomic.LoadInt32(&b.consecutiveFailures))
}

// ConsecutiveSuccesses returns the consecutive success count.
func (b *Backend) ConsecutiveSuccesses() int {
	return int(atomic.LoadInt32(&b.consecutiveSuccesses))
}

// Drain starts draining connections from this backend.
func (b *Backend) Drain() {
	b.SetState(BackendStateDraining)
}

// IsDrained returns true if all connections have drained.
func (b *Backend) IsDrained() bool {
	return b.State() == BackendStateDraining && b.ActiveConnections() == 0
}

// Enable enables the backend.
func (b *Backend) Enable() {
	state := b.State()
	if state == BackendStateDisabled || state == BackendStateDraining {
		b.SetState(BackendStateUnknown)
	}
}

// Disable disables the backend.
func (b *Backend) Disable() {
	b.SetState(BackendStateDisabled)
}

// Stats returns backend statistics.
func (b *Backend) Stats() BackendStats {
	avg, p50, p95, p99 := b.metrics.ResponseTimes.Stats()
	return BackendStats{
		Name:                 b.Config.Name,
		Address:              b.Config.Address,
		State:                b.State(),
		ActiveConnections:    b.ActiveConnections(),
		TotalConnections:     b.TotalConnections(),
		TotalRequests:        b.TotalRequests(),
		TotalErrors:          b.TotalErrors(),
		TotalBytes:           b.TotalBytes(),
		ConsecutiveFailures:  b.ConsecutiveFailures(),
		ConsecutiveSuccesses: b.ConsecutiveSuccesses(),
		LastHealthCheck:      b.LastHealthCheck(),
		AvgResponseTime:      avg,
		P50ResponseTime:      p50,
		P95ResponseTime:      p95,
		P99ResponseTime:      p99,
		RequestsPerSecond:    b.metrics.RequestsPerSecond.Rate(),
		ErrorsPerSecond:      b.metrics.ErrorsPerSecond.Rate(),
	}
}

// BackendStats holds statistics for a backend.
type BackendStats struct {
	Name                 string
	Address              string
	State                BackendState
	ActiveConnections    int64
	TotalConnections     int64
	TotalRequests        int64
	TotalErrors          int64
	TotalBytes           int64
	ConsecutiveFailures  int
	ConsecutiveSuccesses int
	LastHealthCheck      time.Time
	AvgResponseTime      time.Duration
	P50ResponseTime      time.Duration
	P95ResponseTime      time.Duration
	P99ResponseTime      time.Duration
	RequestsPerSecond    float64
	ErrorsPerSecond      float64
}

// BackendPool manages a pool of backends.
type BackendPool struct {
	mu       sync.RWMutex
	backends []*Backend
	byName   map[string]*Backend

	// algorithm is the load balancing algorithm state.
	algorithm AlgorithmState

	// config is the pool configuration.
	config *Config
}

// NewBackendPool creates a new backend pool from configuration.
func NewBackendPool(config *Config) (*BackendPool, error) {
	pool := &BackendPool{
		backends: make([]*Backend, 0, len(config.Backends)),
		byName:   make(map[string]*Backend),
		config:   config,
	}

	// Create backends
	for _, bc := range config.Backends {
		backend, err := NewBackend(bc)
		if err != nil {
			return nil, fmt.Errorf("create backend %s: %w", bc.Name, err)
		}
		pool.backends = append(pool.backends, backend)
		pool.byName[bc.Name] = backend
	}

	// Initialize algorithm state
	pool.algorithm = NewAlgorithmState(config.Algorithm, pool.backends)

	return pool, nil
}

// Select selects a backend using the configured algorithm.
func (p *BackendPool) Select(ctx context.Context, key string) (*Backend, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Get healthy backends
	healthy := p.healthyBackends()
	if len(healthy) == 0 {
		return nil, ErrNoHealthyBackends
	}

	return p.algorithm.Select(ctx, key, healthy)
}

// healthyBackends returns backends that can accept connections.
func (p *BackendPool) healthyBackends() []*Backend {
	result := make([]*Backend, 0, len(p.backends))
	for _, b := range p.backends {
		if b.IsAvailable() {
			result = append(result, b)
		}
	}
	return result
}

// Get returns a backend by name.
func (p *BackendPool) Get(name string) (*Backend, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	b, ok := p.byName[name]
	return b, ok
}

// All returns all backends.
func (p *BackendPool) All() []*Backend {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make([]*Backend, len(p.backends))
	copy(result, p.backends)
	return result
}

// Healthy returns all healthy backends.
func (p *BackendPool) Healthy() []*Backend {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.healthyBackends()
}

// Add adds a new backend to the pool.
func (p *BackendPool) Add(config BackendConfig) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check for duplicate name
	if _, exists := p.byName[config.Name]; exists {
		return fmt.Errorf("backend %s already exists", config.Name)
	}

	backend, err := NewBackend(config)
	if err != nil {
		return fmt.Errorf("create backend: %w", err)
	}

	p.backends = append(p.backends, backend)
	p.byName[config.Name] = backend

	// Update algorithm state
	p.algorithm = NewAlgorithmState(p.config.Algorithm, p.backends)

	return nil
}

// Remove removes a backend from the pool.
func (p *BackendPool) Remove(name string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	backend, ok := p.byName[name]
	if !ok {
		return fmt.Errorf("backend %s not found", name)
	}

	// Start draining
	backend.Drain()

	// Remove from slice
	for i, b := range p.backends {
		if b == backend {
			p.backends = append(p.backends[:i], p.backends[i+1:]...)
			break
		}
	}
	delete(p.byName, name)

	// Update algorithm state
	p.algorithm = NewAlgorithmState(p.config.Algorithm, p.backends)

	return nil
}

// UpdateWeight updates a backend's weight.
func (p *BackendPool) UpdateWeight(name string, weight int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	backend, ok := p.byName[name]
	if !ok {
		return fmt.Errorf("backend %s not found", name)
	}

	if weight < 0 || weight > 100 {
		return errors.New("weight must be between 0 and 100")
	}

	backend.Config.Weight = weight

	// Update algorithm state
	p.algorithm = NewAlgorithmState(p.config.Algorithm, p.backends)

	return nil
}

// Stats returns pool statistics.
func (p *BackendPool) Stats() PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := PoolStats{
		TotalBackends:  len(p.backends),
		Backends:       make([]BackendStats, len(p.backends)),
		TotalRequests:  0,
		TotalErrors:    0,
		TotalBytes:     0,
		ActiveConns:    0,
		RequestsPerSec: 0,
	}

	for i, b := range p.backends {
		bStats := b.Stats()
		stats.Backends[i] = bStats

		stats.TotalRequests += bStats.TotalRequests
		stats.TotalErrors += bStats.TotalErrors
		stats.TotalBytes += bStats.TotalBytes
		stats.ActiveConns += bStats.ActiveConnections
		stats.RequestsPerSec += bStats.RequestsPerSecond

		switch bStats.State {
		case BackendStateHealthy:
			stats.HealthyBackends++
		case BackendStateUnhealthy:
			stats.UnhealthyBackends++
		case BackendStateDraining:
			stats.DrainingBackends++
		}
	}

	return stats
}

// PoolStats holds statistics for the entire pool.
type PoolStats struct {
	TotalBackends     int
	HealthyBackends   int
	UnhealthyBackends int
	DrainingBackends  int
	TotalRequests     int64
	TotalErrors       int64
	TotalBytes        int64
	ActiveConns       int64
	RequestsPerSec    float64
	Backends          []BackendStats
}

// ConnectionPool manages pooled connections to a backend.
type ConnectionPool struct {
	mu          sync.Mutex
	connections []net.Conn
	maxIdle     int
	idleTimeout time.Duration
	backend     *Backend
	created     []time.Time
}

// NewConnectionPool creates a new connection pool.
func NewConnectionPool(backend *Backend, maxIdle int, idleTimeout time.Duration) *ConnectionPool {
	return &ConnectionPool{
		connections: make([]net.Conn, 0, maxIdle),
		created:     make([]time.Time, 0, maxIdle),
		maxIdle:     maxIdle,
		idleTimeout: idleTimeout,
		backend:     backend,
	}
}

// Get retrieves a connection from the pool or creates a new one.
func (p *ConnectionPool) Get(ctx context.Context) (net.Conn, error) {
	p.mu.Lock()

	// Try to get from pool
	now := time.Now()
	for len(p.connections) > 0 {
		conn := p.connections[len(p.connections)-1]
		createdAt := p.created[len(p.created)-1]
		p.connections = p.connections[:len(p.connections)-1]
		p.created = p.created[:len(p.created)-1]

		// Check if connection is still valid
		if now.Sub(createdAt) > p.idleTimeout {
			conn.Close()
			continue
		}

		p.mu.Unlock()
		return conn, nil
	}
	p.mu.Unlock()

	// Create new connection
	return p.dial(ctx)
}

// Put returns a connection to the pool.
func (p *ConnectionPool) Put(conn net.Conn) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.connections) >= p.maxIdle {
		conn.Close()
		return
	}

	p.connections = append(p.connections, conn)
	p.created = append(p.created, time.Now())
}

// dial creates a new connection to the backend.
func (p *ConnectionPool) dial(ctx context.Context) (net.Conn, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", p.backend.Config.Address)
	if err != nil {
		return nil, err
	}

	// Upgrade to TLS if configured
	if p.backend.tlsConfig != nil {
		tlsConn := tls.Client(conn, p.backend.tlsConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			conn.Close()
			return nil, err
		}
		return tlsConn, nil
	}

	return conn, nil
}

// Close closes all connections in the pool.
func (p *ConnectionPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, conn := range p.connections {
		conn.Close()
	}
	p.connections = p.connections[:0]
	p.created = p.created[:0]
}

// Size returns the current pool size.
func (p *ConnectionPool) Size() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.connections)
}

// Cleanup removes expired connections.
func (p *ConnectionPool) Cleanup() {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	newConns := make([]net.Conn, 0, len(p.connections))
	newCreated := make([]time.Time, 0, len(p.created))

	for i, conn := range p.connections {
		if now.Sub(p.created[i]) > p.idleTimeout {
			conn.Close()
		} else {
			newConns = append(newConns, conn)
			newCreated = append(newCreated, p.created[i])
		}
	}

	p.connections = newConns
	p.created = newCreated
}

// ErrNoHealthyBackends is returned when no healthy backends are available.
var ErrNoHealthyBackends = errors.New("no healthy backends available")

// ErrBackendNotFound is returned when a backend is not found.
var ErrBackendNotFound = errors.New("backend not found")

// ErrPoolExhausted is returned when the connection pool is exhausted.
var ErrPoolExhausted = errors.New("connection pool exhausted")
