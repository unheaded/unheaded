// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package proxy

import (
	"errors"
	"math/rand"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"unheaded/pkg/logger"
)

// ErrNoAvailableBackends is returned when no backends are available.
var ErrNoAvailableBackends = errors.New("no available backends")

// LoadBalancer defines the interface for load balancers.
type LoadBalancer interface {
	// Next returns the next backend to use.
	Next() (string, error)
	// MarkUnhealthy marks a backend as unhealthy.
	MarkUnhealthy(backend string)
	// MarkHealthy marks a backend as healthy.
	MarkHealthy(backend string)
	// GetHealthy returns all healthy backends.
	GetHealthy() []string
}

// BackendState tracks the state of a backend.
type BackendState struct {
	URL          string
	Healthy      bool
	Connections  int64
	LastChecked  time.Time
	FailureCount int
}

// BaseLoadBalancer provides common functionality for load balancers.
type BaseLoadBalancer struct {
	backends map[string]*BackendState
	healthy  []string
	mu       sync.RWMutex
	log      *logger.Logger
}

// NewBaseLoadBalancer creates a new base load balancer.
func NewBaseLoadBalancer(backends []string, log *logger.Logger) *BaseLoadBalancer {
	lb := &BaseLoadBalancer{
		backends: make(map[string]*BackendState),
		healthy:  make([]string, 0, len(backends)),
		log:      log,
	}

	for _, b := range backends {
		lb.backends[b] = &BackendState{
			URL:     b,
			Healthy: true,
		}
		lb.healthy = append(lb.healthy, b)
	}

	return lb
}

// MarkUnhealthy marks a backend as unhealthy.
func (lb *BaseLoadBalancer) MarkUnhealthy(backend string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	if state, ok := lb.backends[backend]; ok {
		state.Healthy = false
		state.FailureCount++
		lb.rebuildHealthy()

		lb.log.Warn().
			Str("backend", backend).
			Int("failure_count", state.FailureCount).
			Msg("Backend marked unhealthy")
	}
}

// MarkHealthy marks a backend as healthy.
func (lb *BaseLoadBalancer) MarkHealthy(backend string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	if state, ok := lb.backends[backend]; ok {
		state.Healthy = true
		state.FailureCount = 0
		state.LastChecked = time.Now()
		lb.rebuildHealthy()

		lb.log.Info().
			Str("backend", backend).
			Msg("Backend marked healthy")
	}
}

// GetHealthy returns all healthy backends.
func (lb *BaseLoadBalancer) GetHealthy() []string {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	result := make([]string, len(lb.healthy))
	copy(result, lb.healthy)
	return result
}

// rebuildHealthy rebuilds the healthy backend list.
func (lb *BaseLoadBalancer) rebuildHealthy() {
	lb.healthy = lb.healthy[:0]
	for url, state := range lb.backends {
		if state.Healthy {
			lb.healthy = append(lb.healthy, url)
		}
	}
}

// RoundRobinLoadBalancer implements round-robin load balancing.
type RoundRobinLoadBalancer struct {
	*BaseLoadBalancer
	current uint64
}

// NewRoundRobinLoadBalancer creates a new round-robin load balancer.
func NewRoundRobinLoadBalancer(backends []string, log *logger.Logger) *RoundRobinLoadBalancer {
	return &RoundRobinLoadBalancer{
		BaseLoadBalancer: NewBaseLoadBalancer(backends, log),
	}
}

// Next returns the next backend using round-robin.
func (lb *RoundRobinLoadBalancer) Next() (string, error) {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	if len(lb.healthy) == 0 {
		return "", ErrNoAvailableBackends
	}

	idx := atomic.AddUint64(&lb.current, 1) % uint64(len(lb.healthy))
	return lb.healthy[idx], nil
}

// RandomLoadBalancer implements random load balancing.
type RandomLoadBalancer struct {
	*BaseLoadBalancer
	rng *rand.Rand
}

// NewRandomLoadBalancer creates a new random load balancer.
func NewRandomLoadBalancer(backends []string, log *logger.Logger) *RandomLoadBalancer {
	return &RandomLoadBalancer{
		BaseLoadBalancer: NewBaseLoadBalancer(backends, log),
		rng:              rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Next returns a random backend.
func (lb *RandomLoadBalancer) Next() (string, error) {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	if len(lb.healthy) == 0 {
		return "", ErrNoAvailableBackends
	}

	idx := lb.rng.Intn(len(lb.healthy))
	return lb.healthy[idx], nil
}

// LeastConnectionsLoadBalancer implements least-connections load balancing.
type LeastConnectionsLoadBalancer struct {
	*BaseLoadBalancer
}

// NewLeastConnectionsLoadBalancer creates a new least-connections load balancer.
func NewLeastConnectionsLoadBalancer(backends []string, log *logger.Logger) *LeastConnectionsLoadBalancer {
	return &LeastConnectionsLoadBalancer{
		BaseLoadBalancer: NewBaseLoadBalancer(backends, log),
	}
}

// Next returns the backend with the least connections.
func (lb *LeastConnectionsLoadBalancer) Next() (string, error) {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	if len(lb.healthy) == 0 {
		return "", ErrNoAvailableBackends
	}

	var selected string
	var minConns int64 = -1

	for _, url := range lb.healthy {
		state := lb.backends[url]
		conns := atomic.LoadInt64(&state.Connections)
		if minConns < 0 || conns < minConns {
			minConns = conns
			selected = url
		}
	}

	// Increment connection count
	atomic.AddInt64(&lb.backends[selected].Connections, 1)

	return selected, nil
}

// ReleaseConnection decrements the connection count for a backend.
func (lb *LeastConnectionsLoadBalancer) ReleaseConnection(backend string) {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	if state, ok := lb.backends[backend]; ok {
		atomic.AddInt64(&state.Connections, -1)
	}
}

// NewLoadBalancer creates a load balancer based on the algorithm name.
func NewLoadBalancer(algorithm string, backends []string, log *logger.Logger) LoadBalancer {
	switch algorithm {
	case "random":
		return NewRandomLoadBalancer(backends, log)
	case "least_conn":
		return NewLeastConnectionsLoadBalancer(backends, log)
	default:
		return NewRoundRobinLoadBalancer(backends, log)
	}
}

// HealthChecker periodically checks backend health.
type HealthChecker struct {
	loadBalancers map[string]LoadBalancer
	routes        map[string]string // route name -> health check URL
	interval      time.Duration
	timeout       time.Duration
	client        *http.Client
	log           *logger.Logger
	stopCh        chan struct{}
	wg            sync.WaitGroup
	mu            sync.RWMutex
}

// NewHealthChecker creates a new health checker.
func NewHealthChecker(interval, timeout time.Duration, log *logger.Logger) *HealthChecker {
	return &HealthChecker{
		loadBalancers: make(map[string]LoadBalancer),
		routes:        make(map[string]string),
		interval:      interval,
		timeout:       timeout,
		client: &http.Client{
			Timeout: timeout,
		},
		log:    log,
		stopCh: make(chan struct{}),
	}
}

// RegisterRoute registers a route for health checking.
func (hc *HealthChecker) RegisterRoute(name string, healthCheckURL string, lb LoadBalancer) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.loadBalancers[name] = lb
	hc.routes[name] = healthCheckURL
}

// Start starts the health checker.
func (hc *HealthChecker) Start() {
	hc.wg.Add(1)
	go hc.run()
}

// Stop stops the health checker.
func (hc *HealthChecker) Stop() {
	close(hc.stopCh)
	hc.wg.Wait()
}

// run periodically checks backend health.
func (hc *HealthChecker) run() {
	defer hc.wg.Done()

	ticker := time.NewTicker(hc.interval)
	defer ticker.Stop()

	// Initial check
	hc.checkAll()

	for {
		select {
		case <-ticker.C:
			hc.checkAll()
		case <-hc.stopCh:
			return
		}
	}
}

// checkAll checks all backends.
func (hc *HealthChecker) checkAll() {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	for name, lb := range hc.loadBalancers {
		healthCheckURL := hc.routes[name]
		if healthCheckURL == "" {
			continue
		}

		// Check each backend
		for _, backend := range getAllBackends(lb) {
			go hc.checkBackend(name, backend, healthCheckURL, lb)
		}
	}
}

// checkBackend checks a single backend.
func (hc *HealthChecker) checkBackend(routeName, backend, healthCheckPath string, lb LoadBalancer) {
	url := backend + healthCheckPath

	resp, err := hc.client.Get(url)
	if err != nil {
		hc.log.Debug().
			Str("route", routeName).
			Str("backend", backend).
			Str("error", err.Error()).
			Msg("Health check failed")
		lb.MarkUnhealthy(backend)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		lb.MarkHealthy(backend)
	} else {
		hc.log.Debug().
			Str("route", routeName).
			Str("backend", backend).
			Int("status", resp.StatusCode).
			Msg("Health check returned non-2xx")
		lb.MarkUnhealthy(backend)
	}
}

// getAllBackends returns all backends from a load balancer (including unhealthy).
func getAllBackends(lb LoadBalancer) []string {
	switch l := lb.(type) {
	case *RoundRobinLoadBalancer:
		return getBackendsFromBase(l.BaseLoadBalancer)
	case *RandomLoadBalancer:
		return getBackendsFromBase(l.BaseLoadBalancer)
	case *LeastConnectionsLoadBalancer:
		return getBackendsFromBase(l.BaseLoadBalancer)
	default:
		return lb.GetHealthy()
	}
}

// getBackendsFromBase returns all backends from a BaseLoadBalancer.
func getBackendsFromBase(lb *BaseLoadBalancer) []string {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	backends := make([]string, 0, len(lb.backends))
	for url := range lb.backends {
		backends = append(backends, url)
	}
	return backends
}
