// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package dns provides DNS-based service discovery with health checking.
package dns

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// EnhancedHealthChecker performs health checks on service endpoints
type EnhancedHealthChecker struct {
	config     *ServiceDiscoveryConfig
	sd         *EnhancedServiceDiscovery
	httpClient *http.Client

	mu       sync.RWMutex
	services map[string]*serviceHealthState
	done     chan struct{}
	running  atomic.Bool
	wg       sync.WaitGroup
}

type serviceHealthState struct {
	service   *ServiceDefinition
	endpoints map[string]*endpointHealthState
	stopCh    chan struct{}
}

type endpointHealthState struct {
	endpoint         *ServiceEndpoint
	health           HealthState   // local copy, safe under hc.mu
	responseTime     time.Duration // local copy, safe under hc.mu
	consecutiveOK    int
	consecutiveFail  int
	lastCheck        time.Time
	lastError        error
	circuitState     CircuitState
	circuitOpenUntil time.Time
	halfOpenCount    int
}

// CircuitState represents circuit breaker state
type CircuitState int

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// NewEnhancedHealthChecker creates a new health checker
func NewEnhancedHealthChecker(config *ServiceDiscoveryConfig, sd *EnhancedServiceDiscovery) *EnhancedHealthChecker {
	return &EnhancedHealthChecker{
		config: config,
		sd:     sd,
		httpClient: &http.Client{
			Timeout: config.HealthCheckTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     30 * time.Second,
				DisableKeepAlives:   true,
			},
		},
		services: make(map[string]*serviceHealthState),
		done:     make(chan struct{}),
	}
}

// Start starts the health checker
func (hc *EnhancedHealthChecker) Start() error {
	if !hc.running.CompareAndSwap(false, true) {
		return fmt.Errorf("health checker already running")
	}
	return nil
}

// Stop stops all health checks
func (hc *EnhancedHealthChecker) Stop() {
	if !hc.running.CompareAndSwap(true, false) {
		return
	}

	close(hc.done)

	hc.mu.Lock()
	for _, state := range hc.services {
		close(state.stopCh)
	}
	hc.services = make(map[string]*serviceHealthState)
	hc.mu.Unlock()

	hc.wg.Wait()
}

// AddService starts health checking for a service
func (hc *EnhancedHealthChecker) AddService(svc *ServiceDefinition) {
	if svc.HealthCheck == nil {
		return
	}

	hc.mu.Lock()
	defer hc.mu.Unlock()

	if _, exists := hc.services[svc.Name]; exists {
		return
	}

	state := &serviceHealthState{
		service:   svc,
		endpoints: make(map[string]*endpointHealthState),
		stopCh:    make(chan struct{}),
	}

	for _, ep := range svc.Endpoints {
		state.endpoints[ep.ID] = &endpointHealthState{
			endpoint:     ep,
			health:       ep.Health,
			responseTime: ep.ResponseTime,
			circuitState: CircuitClosed,
		}
	}

	hc.services[svc.Name] = state

	hc.wg.Add(1)
	go hc.checkLoop(state)
}

// RemoveService stops health checking for a service
func (hc *EnhancedHealthChecker) RemoveService(name string) {
	hc.mu.Lock()
	state, exists := hc.services[name]
	if exists {
		close(state.stopCh)
		delete(hc.services, name)
	}
	hc.mu.Unlock()
}

// checkLoop runs health checks for a service
func (hc *EnhancedHealthChecker) checkLoop(state *serviceHealthState) {
	defer hc.wg.Done()

	interval := state.service.HealthCheck.Interval
	if interval == 0 {
		interval = hc.config.HealthCheckInterval
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Initial check
	hc.checkService(state)

	for {
		select {
		case <-state.stopCh:
			return
		case <-hc.done:
			return
		case <-ticker.C:
			hc.checkService(state)
		}
	}
}

// checkService checks all endpoints of a service
func (hc *EnhancedHealthChecker) checkService(state *serviceHealthState) {
	hc.mu.RLock()
	endpoints := make([]*endpointHealthState, 0, len(state.endpoints))
	for _, ep := range state.endpoints {
		endpoints = append(endpoints, ep)
	}
	hc.mu.RUnlock()

	var wg sync.WaitGroup
	for _, epState := range endpoints {
		wg.Add(1)
		go func(es *endpointHealthState) {
			defer wg.Done()
			hc.checkEndpoint(state.service, es)
		}(epState)
	}
	wg.Wait()
}

// checkEndpoint performs a health check on a single endpoint
func (hc *EnhancedHealthChecker) checkEndpoint(svc *ServiceDefinition, epState *endpointHealthState) {
	ep := epState.endpoint
	config := svc.HealthCheck

	// Check circuit breaker
	if svc.CircuitBreaker != nil && svc.CircuitBreaker.Enabled {
		if !hc.allowCheck(epState, svc.CircuitBreaker) {
			return
		}
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = hc.config.HealthCheckTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var err error
	start := time.Now()

	switch config.Type {
	case "tcp":
		err = hc.tcpCheck(ctx, ep)
	case "http", "https":
		err = hc.httpCheck(ctx, ep, config)
	case "grpc":
		err = hc.grpcCheck(ctx, ep)
	default:
		err = hc.tcpCheck(ctx, ep)
	}

	responseTime := time.Since(start)

	hc.mu.Lock()
	epState.lastCheck = time.Now()
	epState.lastError = err

	// Update response time (exponential moving average) on local copy
	if epState.responseTime == 0 {
		epState.responseTime = responseTime
	} else {
		epState.responseTime = (epState.responseTime*7 + responseTime*3) / 10
	}

	newHealth := hc.determineHealth(epState, err, config)
	epState.health = newHealth

	// Update circuit breaker state
	if svc.CircuitBreaker != nil && svc.CircuitBreaker.Enabled {
		hc.updateCircuitBreaker(epState, err, svc.CircuitBreaker)
	}

	hc.mu.Unlock()

	// Update health through service discovery (uses sd.mu exclusively for ep fields).
	// UpdateEndpointHealth no-ops when health hasn't changed.
	_ = hc.sd.UpdateEndpointHealth(svc.Name, ep.ID, newHealth)
}

// determineHealth determines the health state based on check results
func (hc *EnhancedHealthChecker) determineHealth(epState *endpointHealthState, err error, config *ServiceHealthCheck) HealthState {
	healthyThreshold := config.HealthyThreshold
	unhealthyThreshold := config.UnhealthyThreshold
	if healthyThreshold == 0 {
		healthyThreshold = hc.config.HealthyThreshold
	}
	if unhealthyThreshold == 0 {
		unhealthyThreshold = hc.config.UnhealthyThreshold
	}

	if err == nil {
		epState.consecutiveOK++
		epState.consecutiveFail = 0

		if epState.consecutiveOK >= healthyThreshold {
			return HealthStateHealthy
		}
		if epState.health == HealthStateUnhealthy {
			return HealthStateDegraded
		}
		return epState.health
	}

	epState.consecutiveFail++
	epState.consecutiveOK = 0

	if epState.consecutiveFail >= unhealthyThreshold {
		return HealthStateUnhealthy
	}
	if epState.health == HealthStateHealthy {
		return HealthStateDegraded
	}
	return epState.health
}

// allowCheck checks if a health check is allowed by the circuit breaker
func (hc *EnhancedHealthChecker) allowCheck(epState *endpointHealthState, config *CircuitBreakerConfig) bool {
	switch epState.circuitState {
	case CircuitOpen:
		if time.Now().After(epState.circuitOpenUntil) {
			epState.circuitState = CircuitHalfOpen
			epState.halfOpenCount = 0
			return true
		}
		return false
	case CircuitHalfOpen:
		if epState.halfOpenCount < config.HalfOpenRequests {
			epState.halfOpenCount++
			return true
		}
		return false
	default:
		return true
	}
}

// updateCircuitBreaker updates circuit breaker state after a check
func (hc *EnhancedHealthChecker) updateCircuitBreaker(epState *endpointHealthState, err error, config *CircuitBreakerConfig) {
	switch epState.circuitState {
	case CircuitClosed:
		if err != nil && epState.consecutiveFail >= config.ErrorThreshold {
			epState.circuitState = CircuitOpen
			epState.circuitOpenUntil = time.Now().Add(config.OpenDuration)
		}
	case CircuitHalfOpen:
		if err == nil {
			epState.circuitState = CircuitClosed
		} else {
			epState.circuitState = CircuitOpen
			epState.circuitOpenUntil = time.Now().Add(config.OpenDuration)
		}
	}
}

// tcpCheck performs a TCP health check
func (hc *EnhancedHealthChecker) tcpCheck(ctx context.Context, ep *ServiceEndpoint) error {
	var addr string
	if ep.Address.To4() != nil {
		addr = fmt.Sprintf("%s:%d", ep.Address.String(), ep.Port)
	} else {
		addr = fmt.Sprintf("[%s]:%d", ep.Address.String(), ep.Port)
	}

	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("tcp connect: %w", err)
	}
	_ = conn.Close()
	return nil
}

// httpCheck performs an HTTP health check
func (hc *EnhancedHealthChecker) httpCheck(ctx context.Context, ep *ServiceEndpoint, config *ServiceHealthCheck) error {
	scheme := "http"
	if config.Type == "https" {
		scheme = "https"
	}

	path := config.Path
	if path == "" {
		path = "/health"
	}

	var host string
	if ep.Address.To4() != nil {
		host = fmt.Sprintf("%s:%d", ep.Address.String(), ep.Port)
	} else {
		host = fmt.Sprintf("[%s]:%d", ep.Address.String(), ep.Port)
	}

	url := fmt.Sprintf("%s://%s%s", scheme, host, path)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := hc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	// Drain body
	_, _ = io.Copy(io.Discard, resp.Body)

	expectedStatus := config.ExpectedStatus
	if expectedStatus == 0 {
		expectedStatus = http.StatusOK
	}

	if resp.StatusCode != expectedStatus {
		return fmt.Errorf("unexpected status: %d (expected %d)", resp.StatusCode, expectedStatus)
	}

	return nil
}

// grpcCheck performs a gRPC health check (basic TCP for now)
func (hc *EnhancedHealthChecker) grpcCheck(ctx context.Context, ep *ServiceEndpoint) error {
	// Basic TCP check - full gRPC health check would need grpc package
	return hc.tcpCheck(ctx, ep)
}

// GetEndpointHealth returns detailed health info for an endpoint
func (hc *EnhancedHealthChecker) GetEndpointHealth(serviceName, endpointID string) (*EndpointHealthInfo, error) {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	state, exists := hc.services[serviceName]
	if !exists {
		return nil, ErrServiceNotFound
	}

	epState, exists := state.endpoints[endpointID]
	if !exists {
		return nil, fmt.Errorf("endpoint %s not found", endpointID)
	}

	return &EndpointHealthInfo{
		ID:              endpointID,
		Health:          epState.health,
		ConsecutiveOK:   epState.consecutiveOK,
		ConsecutiveFail: epState.consecutiveFail,
		LastCheck:       epState.lastCheck,
		LastError:       epState.lastError,
		CircuitState:    epState.circuitState,
		ResponseTime:    epState.responseTime,
	}, nil
}

// EndpointHealthInfo contains detailed health information
type EndpointHealthInfo struct {
	ID              string
	Health          HealthState
	ConsecutiveOK   int
	ConsecutiveFail int
	LastCheck       time.Time
	LastError       error
	CircuitState    CircuitState
	ResponseTime    time.Duration
}

// CheckNow performs an immediate health check on all endpoints of a service
func (hc *EnhancedHealthChecker) CheckNow(serviceName string) error {
	hc.mu.RLock()
	state, exists := hc.services[serviceName]
	hc.mu.RUnlock()

	if !exists {
		return ErrServiceNotFound
	}

	hc.checkService(state)
	return nil
}

// HealthStats returns health statistics for all services
func (hc *EnhancedHealthChecker) HealthStats() map[string]*ServiceHealthStats {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	stats := make(map[string]*ServiceHealthStats)

	for name, state := range hc.services {
		svcStats := &ServiceHealthStats{
			Name:      name,
			Endpoints: make(map[string]*EndpointHealthStats),
		}

		for epID, epState := range state.endpoints {
			svcStats.Endpoints[epID] = &EndpointHealthStats{
				ID:           epID,
				Health:       epState.health,
				LastCheck:    epState.lastCheck,
				CircuitState: epState.circuitState,
				ResponseTime: epState.responseTime,
			}
			svcStats.TotalEndpoints++
			if epState.health == HealthStateHealthy || epState.health == HealthStateDegraded {
				svcStats.HealthyEndpoints++
			}
		}

		stats[name] = svcStats
	}

	return stats
}

// ServiceHealthStats contains health statistics for a service
type ServiceHealthStats struct {
	Name             string
	TotalEndpoints   int
	HealthyEndpoints int
	Endpoints        map[string]*EndpointHealthStats
}

// EndpointHealthStats contains health statistics for an endpoint
type EndpointHealthStats struct {
	ID           string
	Health       HealthState
	LastCheck    time.Time
	CircuitState CircuitState
	ResponseTime time.Duration
}
