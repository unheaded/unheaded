// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package loadbalancer

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// HealthChecker manages health checks for backends.
type HealthChecker struct {
	config  HealthCheckConfig
	pool    *BackendPool
	client  *http.Client
	running int32
	stopCh  chan struct{}
	wg      sync.WaitGroup

	// Callbacks for health state changes.
	onHealthy   func(backend *Backend)
	onUnhealthy func(backend *Backend, err error)

	// mu protects per-backend check state.
	mu          sync.RWMutex
	lastResults map[string]*healthCheckResult
	checkCounts map[string]int64
	errorCounts map[string]int64
}

// healthCheckResult stores the result of a health check.
type healthCheckResult struct {
	healthy   bool
	err       error
	duration  time.Duration
	timestamp time.Time
}

// NewHealthChecker creates a new health checker.
func NewHealthChecker(config HealthCheckConfig, pool *BackendPool) *HealthChecker {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		TLSClientConfig: &tls.Config{
			// Health probes target backends inside the kingdom mesh — these
			// may legitimately serve self-signed or short-lived certs. mTLS
			// for actual proxy traffic is enforced upstream via pkg/mesh/mtls.
			// Health probes do NOT carry user data; the trade-off is acceptable.
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: true, // #nosec G402 -- health-probe-only path; see comment above
		},
		DialContext: (&net.Dialer{
			Timeout:   config.Timeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}

	return &HealthChecker{
		config: config,
		pool:   pool,
		client: &http.Client{
			Transport: transport,
			Timeout:   config.Timeout,
		},
		stopCh:      make(chan struct{}),
		lastResults: make(map[string]*healthCheckResult),
		checkCounts: make(map[string]int64),
		errorCounts: make(map[string]int64),
	}
}

// OnHealthy sets the callback for when a backend becomes healthy.
func (hc *HealthChecker) OnHealthy(fn func(backend *Backend)) {
	hc.onHealthy = fn
}

// OnUnhealthy sets the callback for when a backend becomes unhealthy.
func (hc *HealthChecker) OnUnhealthy(fn func(backend *Backend, err error)) {
	hc.onUnhealthy = fn
}

// Start starts the health checker.
func (hc *HealthChecker) Start() error {
	if !atomic.CompareAndSwapInt32(&hc.running, 0, 1) {
		return errors.New("health checker already running")
	}

	hc.wg.Add(1)
	go hc.run()

	return nil
}

// Stop stops the health checker.
func (hc *HealthChecker) Stop() error {
	if !atomic.CompareAndSwapInt32(&hc.running, 1, 0) {
		return errors.New("health checker not running")
	}

	close(hc.stopCh)
	hc.wg.Wait()

	return nil
}

// run is the main health check loop.
func (hc *HealthChecker) run() {
	defer hc.wg.Done()

	ticker := time.NewTicker(hc.config.Interval)
	defer ticker.Stop()

	// Initial check
	hc.checkAll()

	for {
		select {
		case <-hc.stopCh:
			return
		case <-ticker.C:
			hc.checkAll()
		}
	}
}

// checkAll performs health checks on all backends.
func (hc *HealthChecker) checkAll() {
	backends := hc.pool.All()

	var wg sync.WaitGroup
	for _, backend := range backends {
		// Skip disabled backends
		if backend.State() == BackendStateDisabled {
			continue
		}

		wg.Add(1)
		go func(b *Backend) {
			defer wg.Done()
			hc.check(b)
		}(backend)
	}
	wg.Wait()
}

// check performs a health check on a single backend.
func (hc *HealthChecker) check(backend *Backend) {
	// Get effective config (per-backend override or global)
	config := hc.config
	if backend.Config.HealthCheckOverride != nil {
		config = *backend.Config.HealthCheckOverride
	}

	start := time.Now()
	var err error

	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	switch config.Type {
	case HealthCheckTCP:
		err = hc.checkTCP(ctx, backend)
	case HealthCheckHTTP:
		err = hc.checkHTTP(ctx, backend, config)
	case HealthCheckHTTPS:
		err = hc.checkHTTPS(ctx, backend, config)
	case HealthCheckExec:
		err = hc.checkExec(ctx, backend, config)
	default:
		err = fmt.Errorf("unknown health check type: %s", config.Type)
	}

	duration := time.Since(start)
	healthy := err == nil

	// Store result
	hc.mu.Lock()
	hc.lastResults[backend.Config.Name] = &healthCheckResult{
		healthy:   healthy,
		err:       err,
		duration:  duration,
		timestamp: time.Now(),
	}
	hc.checkCounts[backend.Config.Name]++
	if !healthy {
		hc.errorCounts[backend.Config.Name]++
	}
	hc.mu.Unlock()

	// Update backend state
	prevState := backend.State()
	backend.SetHealthCheckResult(healthy, config)
	newState := backend.State()

	// Call callbacks on state change
	if prevState != newState {
		if newState == BackendStateHealthy && hc.onHealthy != nil {
			hc.onHealthy(backend)
		} else if newState == BackendStateUnhealthy && hc.onUnhealthy != nil {
			hc.onUnhealthy(backend, err)
		}
	}
}

// checkTCP performs a TCP connection health check.
func (hc *HealthChecker) checkTCP(ctx context.Context, backend *Backend) error {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", backend.Config.Address)
	if err != nil {
		return fmt.Errorf("tcp connect: %w", err)
	}
	_ = conn.Close()
	return nil
}

// checkHTTP performs an HTTP health check.
func (hc *HealthChecker) checkHTTP(ctx context.Context, backend *Backend, config HealthCheckConfig) error {
	url := fmt.Sprintf("http://%s%s", backend.Config.Address, config.HTTPPath)
	return hc.doHTTPCheck(ctx, url, config)
}

// checkHTTPS performs an HTTPS health check.
func (hc *HealthChecker) checkHTTPS(ctx context.Context, backend *Backend, config HealthCheckConfig) error {
	url := fmt.Sprintf("https://%s%s", backend.Config.Address, config.HTTPPath)
	return hc.doHTTPCheck(ctx, url, config)
}

// doHTTPCheck performs the actual HTTP health check.
func (hc *HealthChecker) doHTTPCheck(ctx context.Context, url string, config HealthCheckConfig) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	// Add custom headers
	for k, v := range config.HTTPHeaders {
		req.Header.Set(k, v)
	}

	resp, err := hc.client.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if len(config.HTTPExpectedCodes) > 0 {
		found := false
		for _, code := range config.HTTPExpectedCodes {
			if resp.StatusCode == code {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unexpected status code: %d (expected one of %v)",
				resp.StatusCode, config.HTTPExpectedCodes)
		}
	} else if resp.StatusCode >= 400 {
		return fmt.Errorf("unhealthy status code: %d", resp.StatusCode)
	}

	// Check response body if configured
	if config.HTTPExpectedBody != "" {
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024)) // 1MB limit
		if err != nil {
			return fmt.Errorf("read body: %w", err)
		}
		if !bytes.Contains(body, []byte(config.HTTPExpectedBody)) {
			return fmt.Errorf("body does not contain expected string: %s", config.HTTPExpectedBody)
		}
	}

	return nil
}

// checkExec performs a command execution health check.
func (hc *HealthChecker) checkExec(ctx context.Context, backend *Backend, config HealthCheckConfig) error {
	if len(config.ExecCommand) == 0 {
		return errors.New("no exec command configured")
	}

	cmd := exec.CommandContext(ctx, config.ExecCommand[0], config.ExecCommand[1:]...) //nolint:gosec // operator-configured exec health check

	// Set environment variables with backend info
	cmd.Env = append(cmd.Env,
		fmt.Sprintf("BACKEND_NAME=%s", backend.Config.Name),
		fmt.Sprintf("BACKEND_ADDRESS=%s", backend.Config.Address),
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("exec failed: %w (output: %s)", err, strings.TrimSpace(string(output)))
	}

	return nil
}

// CheckNow performs an immediate health check on a backend.
func (hc *HealthChecker) CheckNow(backend *Backend) error {
	hc.check(backend)
	return nil
}

// LastResult returns the last health check result for a backend.
func (hc *HealthChecker) LastResult(name string) (*healthCheckResult, bool) {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	result, ok := hc.lastResults[name]
	return result, ok
}

// Stats returns health check statistics.
func (hc *HealthChecker) Stats() HealthCheckStats {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	stats := HealthCheckStats{
		Results: make(map[string]BackendHealthStats),
	}

	for name, result := range hc.lastResults {
		stats.Results[name] = BackendHealthStats{
			Healthy:     result.healthy,
			LastError:   result.err,
			Duration:    result.duration,
			LastCheck:   result.timestamp,
			TotalChecks: hc.checkCounts[name],
			TotalErrors: hc.errorCounts[name],
		}
	}

	return stats
}

// HealthCheckStats holds health check statistics.
type HealthCheckStats struct {
	Results map[string]BackendHealthStats
}

// BackendHealthStats holds health stats for a single backend.
type BackendHealthStats struct {
	Healthy     bool
	LastError   error
	Duration    time.Duration
	LastCheck   time.Time
	TotalChecks int64
	TotalErrors int64
}

// PassiveHealthChecker monitors backends for errors and marks them unhealthy.
type PassiveHealthChecker struct {
	config  HealthCheckConfig
	pool    *BackendPool
	running int32
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

// NewPassiveHealthChecker creates a new passive health checker.
func NewPassiveHealthChecker(config HealthCheckConfig, pool *BackendPool) *PassiveHealthChecker {
	return &PassiveHealthChecker{
		config: config,
		pool:   pool,
		stopCh: make(chan struct{}),
	}
}

// Start starts the passive health checker.
func (phc *PassiveHealthChecker) Start() error {
	if !atomic.CompareAndSwapInt32(&phc.running, 0, 1) {
		return errors.New("passive health checker already running")
	}

	phc.wg.Add(1)
	go phc.run()

	return nil
}

// Stop stops the passive health checker.
func (phc *PassiveHealthChecker) Stop() error {
	if !atomic.CompareAndSwapInt32(&phc.running, 1, 0) {
		return errors.New("passive health checker not running")
	}

	close(phc.stopCh)
	phc.wg.Wait()

	return nil
}

// run monitors error rates.
func (phc *PassiveHealthChecker) run() {
	defer phc.wg.Done()

	ticker := time.NewTicker(phc.config.PassiveErrorWindow / 10)
	defer ticker.Stop()

	for {
		select {
		case <-phc.stopCh:
			return
		case <-ticker.C:
			phc.checkErrorRates()
		}
	}
}

// checkErrorRates checks error rates for all backends.
func (phc *PassiveHealthChecker) checkErrorRates() {
	backends := phc.pool.All()

	for _, backend := range backends {
		// Skip non-healthy backends
		if backend.State() != BackendStateHealthy {
			continue
		}

		// Check error count in window
		errors := backend.ErrorCount(phc.config.PassiveErrorWindow)
		if errors >= phc.config.PassiveErrorThreshold {
			backend.SetState(BackendStateUnhealthy)
		}

		// Cleanup old errors
		backend.CleanupErrors(phc.config.PassiveErrorWindow)
	}
}

// RecordError records an error for passive health checking.
func (phc *PassiveHealthChecker) RecordError(backend *Backend, err error) {
	backend.RecordError(err)

	// Immediate check if threshold reached
	errors := backend.ErrorCount(phc.config.PassiveErrorWindow)
	if errors >= phc.config.PassiveErrorThreshold {
		backend.SetState(BackendStateUnhealthy)
	}
}

// HealthAggregator combines multiple health checkers.
type HealthAggregator struct {
	active  *HealthChecker
	passive *PassiveHealthChecker
}

// NewHealthAggregator creates a new health aggregator.
func NewHealthAggregator(config HealthCheckConfig, pool *BackendPool) *HealthAggregator {
	ha := &HealthAggregator{}

	if config.Enabled {
		ha.active = NewHealthChecker(config, pool)
	}

	if config.PassiveEnabled {
		ha.passive = NewPassiveHealthChecker(config, pool)
	}

	return ha
}

// Start starts all health checkers.
func (ha *HealthAggregator) Start() error {
	if ha.active != nil {
		if err := ha.active.Start(); err != nil {
			return fmt.Errorf("start active health checker: %w", err)
		}
	}

	if ha.passive != nil {
		if err := ha.passive.Start(); err != nil {
			if ha.active != nil {
				_ = ha.active.Stop()
			}
			return fmt.Errorf("start passive health checker: %w", err)
		}
	}

	return nil
}

// Stop stops all health checkers.
func (ha *HealthAggregator) Stop() error {
	var errs []error

	if ha.active != nil {
		if err := ha.active.Stop(); err != nil {
			errs = append(errs, err)
		}
	}

	if ha.passive != nil {
		if err := ha.passive.Stop(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// RecordError records an error for passive health checking.
func (ha *HealthAggregator) RecordError(backend *Backend, err error) {
	if ha.passive != nil {
		ha.passive.RecordError(backend, err)
	}
}

// OnHealthy sets the callback for when a backend becomes healthy.
func (ha *HealthAggregator) OnHealthy(fn func(backend *Backend)) {
	if ha.active != nil {
		ha.active.OnHealthy(fn)
	}
}

// OnUnhealthy sets the callback for when a backend becomes unhealthy.
func (ha *HealthAggregator) OnUnhealthy(fn func(backend *Backend, err error)) {
	if ha.active != nil {
		ha.active.OnUnhealthy(fn)
	}
}

// HealthEndpoint provides HTTP health check endpoints.
type HealthEndpoint struct {
	pool *BackendPool
}

// NewHealthEndpoint creates a new health endpoint.
func NewHealthEndpoint(pool *BackendPool) *HealthEndpoint {
	return &HealthEndpoint{pool: pool}
}

// ServeHTTP handles health check requests.
func (he *HealthEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/health":
		he.handleHealth(w, r)
	case "/ready":
		he.handleReady(w, r)
	case "/backends":
		he.handleBackends(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (he *HealthEndpoint) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Health returns 200 if load balancer is running
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func (he *HealthEndpoint) handleReady(w http.ResponseWriter, r *http.Request) {
	// Ready returns 200 if at least one backend is healthy
	healthy := he.pool.Healthy()
	if len(healthy) == 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("No healthy backends"))
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK (%d healthy backends)", len(healthy))
}

func (he *HealthEndpoint) handleBackends(w http.ResponseWriter, r *http.Request) {
	stats := he.pool.Stats()

	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "Total: %d, Healthy: %d, Unhealthy: %d, Draining: %d\n\n",
		stats.TotalBackends, stats.HealthyBackends,
		stats.UnhealthyBackends, stats.DrainingBackends)

	for _, bs := range stats.Backends {
		fmt.Fprintf(w, "%s (%s): %s\n", bs.Name, bs.Address, bs.State)
		fmt.Fprintf(w, "  Connections: %d active, %d total\n",
			bs.ActiveConnections, bs.TotalConnections)
		fmt.Fprintf(w, "  Requests: %d total, %.2f/s\n",
			bs.TotalRequests, bs.RequestsPerSecond)
		fmt.Fprintf(w, "  Errors: %d total, %.2f/s\n",
			bs.TotalErrors, bs.ErrorsPerSecond)
		fmt.Fprintf(w, "  Response times: avg=%v, p50=%v, p95=%v, p99=%v\n",
			bs.AvgResponseTime, bs.P50ResponseTime, bs.P95ResponseTime, bs.P99ResponseTime)
		fmt.Fprintf(w, "  Last health check: %v\n", bs.LastHealthCheck)
		fmt.Fprintln(w)
	}
}

// HealthCheckScheduler allows custom scheduling of health checks.
type HealthCheckScheduler struct {
	checker  *HealthChecker
	schedule map[string]time.Duration
	mu       sync.RWMutex
	stopCh   chan struct{}
	wg       sync.WaitGroup
	running  int32
}

// NewHealthCheckScheduler creates a scheduler with custom intervals per backend.
func NewHealthCheckScheduler(checker *HealthChecker) *HealthCheckScheduler {
	return &HealthCheckScheduler{
		checker:  checker,
		schedule: make(map[string]time.Duration),
		stopCh:   make(chan struct{}),
	}
}

// SetInterval sets a custom check interval for a backend.
func (hcs *HealthCheckScheduler) SetInterval(backendName string, interval time.Duration) {
	hcs.mu.Lock()
	defer hcs.mu.Unlock()
	hcs.schedule[backendName] = interval
}

// Start starts the custom scheduler.
func (hcs *HealthCheckScheduler) Start() error {
	if !atomic.CompareAndSwapInt32(&hcs.running, 0, 1) {
		return errors.New("scheduler already running")
	}

	hcs.mu.RLock()
	backends := hcs.checker.pool.All()
	hcs.mu.RUnlock()

	for _, b := range backends {
		hcs.wg.Add(1)
		go hcs.runBackend(b)
	}

	return nil
}

func (hcs *HealthCheckScheduler) runBackend(backend *Backend) {
	defer hcs.wg.Done()

	hcs.mu.RLock()
	interval := hcs.schedule[backend.Config.Name]
	if interval == 0 {
		interval = hcs.checker.config.Interval
	}
	hcs.mu.RUnlock()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-hcs.stopCh:
			return
		case <-ticker.C:
			hcs.checker.check(backend)
		}
	}
}

// Stop stops the custom scheduler.
func (hcs *HealthCheckScheduler) Stop() error {
	if !atomic.CompareAndSwapInt32(&hcs.running, 1, 0) {
		return errors.New("scheduler not running")
	}

	close(hcs.stopCh)
	hcs.wg.Wait()
	return nil
}
