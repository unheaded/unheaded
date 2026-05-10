// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package loadbalancer

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// HealthCheckType constants for additional probe types.
const (
	// HealthCheckGRPC performs gRPC health check.
	HealthCheckGRPC HealthCheckType = "grpc"
)

// GRPCHealthCheckConfig configures gRPC health checks.
type GRPCHealthCheckConfig struct {
	// ServiceName is the gRPC service to check.
	ServiceName string `json:"service_name,omitempty" yaml:"service_name,omitempty"`
	// Authority is the :authority header value for the request.
	Authority string `json:"authority,omitempty" yaml:"authority,omitempty"`
}

// EnhancedHealthChecker extends HealthChecker with additional probe types and features.
type EnhancedHealthChecker struct {
	*HealthChecker

	// gRPC health check config
	grpcConfig GRPCHealthCheckConfig

	// Metrics
	metrics *HealthCheckMetrics

	// Logger interface for health check events
	logger HealthCheckLogger

	// Circuit breakers per backend
	circuitBreakers   map[string]*CircuitBreaker
	circuitBreakersMu sync.RWMutex

	// Per-backend health state history for trend analysis
	healthHistory   map[string]*HealthHistory
	healthHistoryMu sync.RWMutex
}

// HealthCheckLogger defines the interface for health check logging.
type HealthCheckLogger interface {
	Debug(msg string, args ...interface{})
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}

// DefaultHealthCheckLogger is a no-op logger.
type DefaultHealthCheckLogger struct{}

func (d *DefaultHealthCheckLogger) Debug(msg string, args ...interface{}) {}
func (d *DefaultHealthCheckLogger) Info(msg string, args ...interface{})  {}
func (d *DefaultHealthCheckLogger) Warn(msg string, args ...interface{})  {}
func (d *DefaultHealthCheckLogger) Error(msg string, args ...interface{}) {}

// HealthCheckMetrics holds metrics for health checks.
type HealthCheckMetrics struct {
	mu sync.RWMutex

	// Total checks performed per backend
	TotalChecks map[string]int64

	// Failed checks per backend
	FailedChecks map[string]int64

	// Check latencies per backend (rolling window)
	Latencies map[string]*LatencyWindow

	// State transitions per backend
	StateTransitions map[string][]StateTransition

	// Circuit breaker trips
	CircuitBreakerTrips map[string]int64
}

// LatencyWindow tracks latencies over a rolling window.
type LatencyWindow struct {
	mu       sync.Mutex
	samples  []time.Duration
	maxSize  int
	position int
	filled   bool
}

// NewLatencyWindow creates a new latency window.
func NewLatencyWindow(size int) *LatencyWindow {
	return &LatencyWindow{
		samples: make([]time.Duration, size),
		maxSize: size,
	}
}

// Add adds a latency sample.
func (lw *LatencyWindow) Add(d time.Duration) {
	lw.mu.Lock()
	defer lw.mu.Unlock()

	lw.samples[lw.position] = d
	lw.position = (lw.position + 1) % lw.maxSize
	if lw.position == 0 {
		lw.filled = true
	}
}

// Stats returns latency statistics.
func (lw *LatencyWindow) Stats() (avg, min, max, p50, p95, p99 time.Duration) {
	lw.mu.Lock()
	defer lw.mu.Unlock()

	count := lw.maxSize
	if !lw.filled {
		count = lw.position
	}
	if count == 0 {
		return
	}

	// Copy samples for sorting
	sorted := make([]time.Duration, count)
	copy(sorted, lw.samples[:count])
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	// Calculate stats
	var sum time.Duration
	for _, d := range sorted {
		sum += d
	}
	avg = sum / time.Duration(count)
	min = sorted[0]
	max = sorted[count-1]
	p50 = sorted[count*50/100]
	p95 = sorted[count*95/100]
	if count > 0 {
		p99 = sorted[count*99/100]
	}

	return
}

// StateTransition records a state transition.
type StateTransition struct {
	Time      time.Time
	FromState BackendState
	ToState   BackendState
	Reason    string
}

// NewHealthCheckMetrics creates new health check metrics.
func NewHealthCheckMetrics() *HealthCheckMetrics {
	return &HealthCheckMetrics{
		TotalChecks:         make(map[string]int64),
		FailedChecks:        make(map[string]int64),
		Latencies:           make(map[string]*LatencyWindow),
		StateTransitions:    make(map[string][]StateTransition),
		CircuitBreakerTrips: make(map[string]int64),
	}
}

// RecordCheck records a health check.
func (m *HealthCheckMetrics) RecordCheck(backend string, success bool, latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.TotalChecks[backend]++
	if !success {
		m.FailedChecks[backend]++
	}

	if _, ok := m.Latencies[backend]; !ok {
		m.Latencies[backend] = NewLatencyWindow(100)
	}
	m.Latencies[backend].Add(latency)
}

// RecordStateTransition records a state transition.
func (m *HealthCheckMetrics) RecordStateTransition(backend string, from, to BackendState, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	transition := StateTransition{
		Time:      time.Now(),
		FromState: from,
		ToState:   to,
		Reason:    reason,
	}

	m.StateTransitions[backend] = append(m.StateTransitions[backend], transition)

	// Keep only last 100 transitions
	if len(m.StateTransitions[backend]) > 100 {
		m.StateTransitions[backend] = m.StateTransitions[backend][1:]
	}
}

// RecordCircuitBreakerTrip records a circuit breaker trip.
func (m *HealthCheckMetrics) RecordCircuitBreakerTrip(backend string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CircuitBreakerTrips[backend]++
}

// GetStats returns metrics for a backend.
func (m *HealthCheckMetrics) GetStats(backend string) HealthCheckBackendMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := HealthCheckBackendMetrics{
		TotalChecks:         m.TotalChecks[backend],
		FailedChecks:        m.FailedChecks[backend],
		CircuitBreakerTrips: m.CircuitBreakerTrips[backend],
	}

	if lw, ok := m.Latencies[backend]; ok {
		stats.AvgLatency, stats.MinLatency, stats.MaxLatency, stats.P50Latency, stats.P95Latency, stats.P99Latency = lw.Stats()
	}

	if transitions, ok := m.StateTransitions[backend]; ok {
		stats.StateTransitions = make([]StateTransition, len(transitions))
		copy(stats.StateTransitions, transitions)
	}

	return stats
}

// HealthCheckBackendMetrics holds metrics for a single backend.
type HealthCheckBackendMetrics struct {
	TotalChecks         int64
	FailedChecks        int64
	AvgLatency          time.Duration
	MinLatency          time.Duration
	MaxLatency          time.Duration
	P50Latency          time.Duration
	P95Latency          time.Duration
	P99Latency          time.Duration
	CircuitBreakerTrips int64
	StateTransitions    []StateTransition
}

// HealthHistory tracks health check history for trend analysis.
type HealthHistory struct {
	mu      sync.Mutex
	results []healthHistoryEntry
	maxSize int
}

type healthHistoryEntry struct {
	timestamp time.Time
	healthy   bool
	latency   time.Duration
}

// NewHealthHistory creates a new health history tracker.
func NewHealthHistory(maxSize int) *HealthHistory {
	return &HealthHistory{
		results: make([]healthHistoryEntry, 0, maxSize),
		maxSize: maxSize,
	}
}

// Add adds a health check result.
func (h *HealthHistory) Add(healthy bool, latency time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()

	entry := healthHistoryEntry{
		timestamp: time.Now(),
		healthy:   healthy,
		latency:   latency,
	}

	if len(h.results) >= h.maxSize {
		h.results = h.results[1:]
	}
	h.results = append(h.results, entry)
}

// Trend returns the health trend (positive = improving, negative = degrading).
func (h *HealthHistory) Trend() float64 {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.results) < 10 {
		return 0
	}

	// Compare recent 25% vs older 25%
	quarter := len(h.results) / 4
	if quarter < 2 {
		quarter = 2
	}

	recentFailures := 0
	olderFailures := 0

	for i := len(h.results) - quarter; i < len(h.results); i++ {
		if !h.results[i].healthy {
			recentFailures++
		}
	}

	for i := 0; i < quarter; i++ {
		if !h.results[i].healthy {
			olderFailures++
		}
	}

	// Positive = improving (fewer recent failures)
	// Negative = degrading (more recent failures)
	return float64(olderFailures-recentFailures) / float64(quarter)
}

// SuccessRate returns the success rate over the window.
func (h *HealthHistory) SuccessRate() float64 {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.results) == 0 {
		return 1.0
	}

	successes := 0
	for _, r := range h.results {
		if r.healthy {
			successes++
		}
	}
	return float64(successes) / float64(len(h.results))
}

// NewEnhancedHealthChecker creates a new enhanced health checker.
func NewEnhancedHealthChecker(config HealthCheckConfig, pool *BackendPool) *EnhancedHealthChecker {
	base := NewHealthChecker(config, pool)

	ehc := &EnhancedHealthChecker{
		HealthChecker:   base,
		metrics:         NewHealthCheckMetrics(),
		logger:          &DefaultHealthCheckLogger{},
		circuitBreakers: make(map[string]*CircuitBreaker),
		healthHistory:   make(map[string]*HealthHistory),
	}

	// Initialize circuit breakers and history for each backend
	for _, b := range pool.All() {
		ehc.circuitBreakers[b.Config.Name] = NewCircuitBreaker(
			config.Retries,          // threshold
			config.Interval*3,       // timeout (3x check interval)
			config.SuccessThreshold, // half-open max
		)
		ehc.healthHistory[b.Config.Name] = NewHealthHistory(100)
	}

	return ehc
}

// SetLogger sets the logger for health check events.
func (ehc *EnhancedHealthChecker) SetLogger(logger HealthCheckLogger) {
	ehc.logger = logger
}

// SetGRPCConfig sets the gRPC health check configuration.
func (ehc *EnhancedHealthChecker) SetGRPCConfig(config GRPCHealthCheckConfig) {
	ehc.grpcConfig = config
}

// CheckWithCircuitBreaker performs a health check with circuit breaker.
func (ehc *EnhancedHealthChecker) CheckWithCircuitBreaker(backend *Backend) error {
	ehc.circuitBreakersMu.RLock()
	cb, ok := ehc.circuitBreakers[backend.Config.Name]
	ehc.circuitBreakersMu.RUnlock()

	if !ok {
		// No circuit breaker, just do the check
		ehc.check(backend)
		return nil
	}

	// Check if circuit breaker allows the request
	if !cb.Allow() {
		ehc.logger.Warn("Circuit breaker open for backend",
			"backend", backend.Config.Name,
			"state", cb.State())
		return fmt.Errorf("circuit breaker open for %s", backend.Config.Name)
	}

	// Perform the check
	start := time.Now()
	ehc.check(backend)
	duration := time.Since(start)

	// Get the result from the health checker
	result, ok := ehc.LastResult(backend.Config.Name)
	if !ok {
		return nil
	}

	// Update circuit breaker and metrics
	if result.healthy {
		cb.RecordSuccess()
	} else {
		cb.RecordFailure()
		if cb.State() == "open" {
			ehc.metrics.RecordCircuitBreakerTrip(backend.Config.Name)
			ehc.logger.Warn("Circuit breaker tripped",
				"backend", backend.Config.Name)
		}
	}

	// Record metrics
	ehc.metrics.RecordCheck(backend.Config.Name, result.healthy, duration)

	// Update history
	ehc.healthHistoryMu.Lock()
	if h, ok := ehc.healthHistory[backend.Config.Name]; ok {
		h.Add(result.healthy, duration)
	}
	ehc.healthHistoryMu.Unlock()

	return nil
}

// (Note: a half-baked checkGRPC + makeHTTP2SettingsFrame helper used to live
// here, but they were never wired into the dispatch on the embedded
// HealthChecker — health.go's switch on HealthCheckType has cases only for
// TCP/HTTP/HTTPS/Exec. The grpc-health-probe path needs a structural change
// to either move the dispatch onto EnhancedHealthChecker or split the
// underlying HealthChecker, which is out of scope for the current cleanup.
// HealthCheckGRPC + GRPCHealthCheckConfig are kept as the public API surface
// for that future wiring.)

// GetBackendHealth returns comprehensive health info for a backend.
func (ehc *EnhancedHealthChecker) GetBackendHealth(name string) BackendHealthInfo {
	info := BackendHealthInfo{
		Name: name,
	}

	// Get last result
	if result, ok := ehc.LastResult(name); ok {
		info.Healthy = result.healthy
		info.LastCheck = result.timestamp
		info.LastError = result.err
		info.LastLatency = result.duration
	}

	// Get circuit breaker state
	ehc.circuitBreakersMu.RLock()
	if cb, ok := ehc.circuitBreakers[name]; ok {
		info.CircuitBreakerState = cb.State()
	}
	ehc.circuitBreakersMu.RUnlock()

	// Get health history trend
	ehc.healthHistoryMu.RLock()
	if h, ok := ehc.healthHistory[name]; ok {
		info.Trend = h.Trend()
		info.SuccessRate = h.SuccessRate()
	}
	ehc.healthHistoryMu.RUnlock()

	// Get metrics
	info.Metrics = ehc.metrics.GetStats(name)

	return info
}

// BackendHealthInfo contains comprehensive health information.
type BackendHealthInfo struct {
	Name                string
	Healthy             bool
	LastCheck           time.Time
	LastError           error
	LastLatency         time.Duration
	CircuitBreakerState string
	Trend               float64 // positive = improving, negative = degrading
	SuccessRate         float64
	Metrics             HealthCheckBackendMetrics
}

// EnhancedPassiveHealthChecker extends passive health checking.
type EnhancedPassiveHealthChecker struct {
	*PassiveHealthChecker

	// Configurable error types to track
	errorTypes   map[string]bool
	errorTypesMu sync.RWMutex

	// Per-backend error tracking with categorization
	errorsByType   map[string]map[string]int
	errorsByTypeMu sync.RWMutex

	// Callbacks
	onBackendDegraded  func(backend *Backend, errorRate float64)
	onBackendRecovered func(backend *Backend)

	// Metrics
	metrics *PassiveHealthMetrics
}

// PassiveHealthMetrics tracks passive health check metrics.
type PassiveHealthMetrics struct {
	mu sync.RWMutex

	// Errors by type per backend
	ErrorsByType map[string]map[string]int64

	// Error rate per backend (errors/minute)
	ErrorRates map[string]float64

	// Last error time per backend
	LastErrorTime map[string]time.Time
}

// NewPassiveHealthMetrics creates new passive health metrics.
func NewPassiveHealthMetrics() *PassiveHealthMetrics {
	return &PassiveHealthMetrics{
		ErrorsByType:  make(map[string]map[string]int64),
		ErrorRates:    make(map[string]float64),
		LastErrorTime: make(map[string]time.Time),
	}
}

// ErrorType constants for categorizing errors.
const (
	ErrorTypeConnection = "connection"
	ErrorTypeTimeout    = "timeout"
	ErrorTypeHTTP5xx    = "http_5xx"
	ErrorTypeHTTP4xx    = "http_4xx"
	ErrorTypeProtocol   = "protocol"
	ErrorTypeOther      = "other"
)

// NewEnhancedPassiveHealthChecker creates a new enhanced passive health checker.
func NewEnhancedPassiveHealthChecker(config HealthCheckConfig, pool *BackendPool) *EnhancedPassiveHealthChecker {
	base := NewPassiveHealthChecker(config, pool)

	ephc := &EnhancedPassiveHealthChecker{
		PassiveHealthChecker: base,
		errorTypes: map[string]bool{
			ErrorTypeConnection: true,
			ErrorTypeTimeout:    true,
			ErrorTypeHTTP5xx:    true,
			ErrorTypeProtocol:   true,
		},
		errorsByType: make(map[string]map[string]int),
		metrics:      NewPassiveHealthMetrics(),
	}

	return ephc
}

// SetErrorTypes sets which error types to track.
func (ephc *EnhancedPassiveHealthChecker) SetErrorTypes(types []string) {
	ephc.errorTypesMu.Lock()
	defer ephc.errorTypesMu.Unlock()

	ephc.errorTypes = make(map[string]bool)
	for _, t := range types {
		ephc.errorTypes[t] = true
	}
}

// OnBackendDegraded sets callback for backend degradation.
func (ephc *EnhancedPassiveHealthChecker) OnBackendDegraded(fn func(backend *Backend, errorRate float64)) {
	ephc.onBackendDegraded = fn
}

// OnBackendRecovered sets callback for backend recovery.
func (ephc *EnhancedPassiveHealthChecker) OnBackendRecovered(fn func(backend *Backend)) {
	ephc.onBackendRecovered = fn
}

// RecordErrorWithType records an error with type categorization.
func (ephc *EnhancedPassiveHealthChecker) RecordErrorWithType(backend *Backend, err error, errType string) {
	// Check if we should track this error type
	ephc.errorTypesMu.RLock()
	shouldTrack := ephc.errorTypes[errType]
	ephc.errorTypesMu.RUnlock()

	if !shouldTrack {
		return
	}

	// Record the error
	ephc.RecordError(backend, err)

	// Track by type
	ephc.errorsByTypeMu.Lock()
	if ephc.errorsByType[backend.Config.Name] == nil {
		ephc.errorsByType[backend.Config.Name] = make(map[string]int)
	}
	ephc.errorsByType[backend.Config.Name][errType]++
	ephc.errorsByTypeMu.Unlock()

	// Update metrics
	ephc.metrics.mu.Lock()
	if ephc.metrics.ErrorsByType[backend.Config.Name] == nil {
		ephc.metrics.ErrorsByType[backend.Config.Name] = make(map[string]int64)
	}
	ephc.metrics.ErrorsByType[backend.Config.Name][errType]++
	ephc.metrics.LastErrorTime[backend.Config.Name] = time.Now()
	ephc.metrics.mu.Unlock()
}

// ClassifyError classifies an error into a type.
func ClassifyError(err error) string {
	if err == nil {
		return ""
	}

	errStr := err.Error()

	// Connection errors
	if strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "connection reset") {
		return ErrorTypeConnection
	}

	// Timeout errors
	if strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "deadline exceeded") {
		return ErrorTypeTimeout
	}

	// HTTP 5xx errors
	if strings.Contains(errStr, "5") && strings.Contains(errStr, "status") {
		return ErrorTypeHTTP5xx
	}

	// HTTP 4xx errors
	if strings.Contains(errStr, "4") && strings.Contains(errStr, "status") {
		return ErrorTypeHTTP4xx
	}

	// Protocol errors
	if strings.Contains(errStr, "protocol") ||
		strings.Contains(errStr, "malformed") {
		return ErrorTypeProtocol
	}

	return ErrorTypeOther
}

// GetErrorStats returns error statistics for a backend.
func (ephc *EnhancedPassiveHealthChecker) GetErrorStats(backend string) map[string]int {
	ephc.errorsByTypeMu.RLock()
	defer ephc.errorsByTypeMu.RUnlock()

	result := make(map[string]int)
	if stats, ok := ephc.errorsByType[backend]; ok {
		for k, v := range stats {
			result[k] = v
		}
	}
	return result
}

// HealthCheckHTTPResponse represents an HTTP health check response for detailed inspection.
type HealthCheckHTTPResponse struct {
	StatusCode     int
	Headers        http.Header
	Body           []byte
	Latency        time.Duration
	TLSVersion     uint16
	TLSCipherSuite uint16
}

// AdvancedHTTPHealthCheck performs an advanced HTTP health check with detailed response inspection.
func AdvancedHTTPHealthCheck(ctx context.Context, url string, config HealthCheckConfig) (*HealthCheckHTTPResponse, error) {
	req, err := http.NewRequestWithContext(ctx, config.HTTPMethod(), url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Add custom headers
	for k, v := range config.HTTPHeaders {
		req.Header.Set(k, v)
	}

	start := time.Now()

	client := &http.Client{
		Timeout: config.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Don't follow redirects for health checks by default
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	latency := time.Since(start)

	// Read body (limited)
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	result := &HealthCheckHTTPResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       body,
		Latency:    latency,
	}

	if resp.TLS != nil {
		result.TLSVersion = resp.TLS.Version
		result.TLSCipherSuite = resp.TLS.CipherSuite
	}

	return result, nil
}

// HTTPMethod returns the HTTP method for health checks (default GET).
func (h *HealthCheckConfig) HTTPMethod() string {
	// Could be extended to support configurable methods
	return http.MethodGet
}

// TCPHealthCheckWithBanner performs a TCP check and reads the server banner.
type TCPHealthCheckWithBanner struct {
	ExpectedBanner string
	Timeout        time.Duration
}

// Check performs the TCP check with banner verification.
func (t *TCPHealthCheckWithBanner) Check(ctx context.Context, address string) (string, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", address)
	if err != nil {
		return "", fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	// Set read deadline
	conn.SetReadDeadline(time.Now().Add(t.Timeout))

	// Read banner
	reader := bufio.NewReader(conn)
	banner, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read banner: %w", err)
	}

	banner = strings.TrimSpace(banner)

	// Verify banner if expected
	if t.ExpectedBanner != "" && !strings.Contains(banner, t.ExpectedBanner) {
		return banner, fmt.Errorf("unexpected banner: got %q, expected to contain %q", banner, t.ExpectedBanner)
	}

	return banner, nil
}

// HealthCheckWithRetry performs a health check with configurable retry logic.
type HealthCheckWithRetry struct {
	MaxRetries     int
	RetryDelay     time.Duration
	RetryBackoff   float64 // Multiplier for exponential backoff
	RetryableTypes []string
}

// NewHealthCheckWithRetry creates a new health check with retry.
func NewHealthCheckWithRetry(maxRetries int, retryDelay time.Duration) *HealthCheckWithRetry {
	return &HealthCheckWithRetry{
		MaxRetries:   maxRetries,
		RetryDelay:   retryDelay,
		RetryBackoff: 2.0,
		RetryableTypes: []string{
			ErrorTypeConnection,
			ErrorTypeTimeout,
		},
	}
}

// IsRetryable checks if an error should be retried.
func (h *HealthCheckWithRetry) IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	errType := ClassifyError(err)
	for _, t := range h.RetryableTypes {
		if t == errType {
			return true
		}
	}
	return false
}

// GetDelay returns the delay for a retry attempt.
func (h *HealthCheckWithRetry) GetDelay(attempt int) time.Duration {
	if attempt == 0 {
		return h.RetryDelay
	}
	multiplier := 1.0
	for i := 0; i < attempt; i++ {
		multiplier *= h.RetryBackoff
	}
	return time.Duration(float64(h.RetryDelay) * multiplier)
}

// HealthCheckJitter adds jitter to health check intervals to prevent thundering herd.
type HealthCheckJitter struct {
	BaseInterval time.Duration
	JitterFactor float64 // 0.0 to 1.0
}

// NewHealthCheckJitter creates a new jitter configuration.
func NewHealthCheckJitter(baseInterval time.Duration, jitterFactor float64) *HealthCheckJitter {
	if jitterFactor < 0 {
		jitterFactor = 0
	}
	if jitterFactor > 1 {
		jitterFactor = 1
	}
	return &HealthCheckJitter{
		BaseInterval: baseInterval,
		JitterFactor: jitterFactor,
	}
}

// GetInterval returns the interval with jitter applied.
func (h *HealthCheckJitter) GetInterval() time.Duration {
	if h.JitterFactor == 0 {
		return h.BaseInterval
	}

	// Random jitter in range [1-jitter, 1+jitter] * baseInterval
	// Use simple deterministic "random" based on time for vanilla Go
	jitter := float64(time.Now().UnixNano()%1000) / 1000.0 // 0.0 to 1.0
	jitter = (jitter*2 - 1) * h.JitterFactor               // -jitter to +jitter

	return time.Duration(float64(h.BaseInterval) * (1 + jitter))
}

// HealthCheckCoordinator coordinates health checks across multiple checkers.
type HealthCheckCoordinator struct {
	mu       sync.RWMutex
	checkers map[string]*EnhancedHealthChecker
	running  int32
	stopCh   chan struct{}
}

// NewHealthCheckCoordinator creates a new health check coordinator.
func NewHealthCheckCoordinator() *HealthCheckCoordinator {
	return &HealthCheckCoordinator{
		checkers: make(map[string]*EnhancedHealthChecker),
		stopCh:   make(chan struct{}),
	}
}

// AddChecker adds a health checker for a pool.
func (hcc *HealthCheckCoordinator) AddChecker(name string, checker *EnhancedHealthChecker) {
	hcc.mu.Lock()
	defer hcc.mu.Unlock()
	hcc.checkers[name] = checker
}

// RemoveChecker removes a health checker.
func (hcc *HealthCheckCoordinator) RemoveChecker(name string) {
	hcc.mu.Lock()
	defer hcc.mu.Unlock()
	delete(hcc.checkers, name)
}

// Start starts all health checkers.
func (hcc *HealthCheckCoordinator) Start() error {
	if !atomic.CompareAndSwapInt32(&hcc.running, 0, 1) {
		return errors.New("coordinator already running")
	}

	hcc.mu.RLock()
	defer hcc.mu.RUnlock()

	for _, checker := range hcc.checkers {
		if err := checker.Start(); err != nil {
			return err
		}
	}

	return nil
}

// Stop stops all health checkers.
func (hcc *HealthCheckCoordinator) Stop() error {
	if !atomic.CompareAndSwapInt32(&hcc.running, 1, 0) {
		return errors.New("coordinator not running")
	}

	close(hcc.stopCh)

	hcc.mu.RLock()
	defer hcc.mu.RUnlock()

	var errs []error
	for _, checker := range hcc.checkers {
		if err := checker.Stop(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// GetAllHealth returns health info for all backends across all pools.
func (hcc *HealthCheckCoordinator) GetAllHealth() map[string][]BackendHealthInfo {
	hcc.mu.RLock()
	defer hcc.mu.RUnlock()

	result := make(map[string][]BackendHealthInfo)

	for poolName, checker := range hcc.checkers {
		backends := checker.pool.All()
		healthInfos := make([]BackendHealthInfo, len(backends))
		for i, b := range backends {
			healthInfos[i] = checker.GetBackendHealth(b.Config.Name)
		}
		result[poolName] = healthInfos
	}

	return result
}
