// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package mesh provides THE HAUBERK - the service mesh implementation for the Unheaded Kingdom.
// This file implements the circuit breaker pattern for fault tolerance.
package mesh

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// Circuit breaker state constants.
const (
	CircuitStateClosed   CircuitState = iota // Normal operation
	CircuitStateOpen                         // Blocking all requests
	CircuitStateHalfOpen                     // Testing if service recovered
)

// CircuitState represents the state of a circuit breaker.
type CircuitState int

// String returns the string representation of the circuit state.
func (s CircuitState) String() string {
	switch s {
	case CircuitStateClosed:
		return "closed"
	case CircuitStateOpen:
		return "open"
	case CircuitStateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Circuit breaker errors.
var (
	ErrCircuitBreakerOpen    = errors.New("circuit breaker is open")
	ErrTooManyHalfOpenReqs   = errors.New("too many requests in half-open state")
	ErrCircuitBreakerTimeout = errors.New("circuit breaker request timeout")
)

// CircuitBreakerConfig configures a circuit breaker.
type CircuitBreakerConfig struct {
	Name                string                                  // Circuit breaker name
	FailureThreshold    int                                     // Failures before opening
	SuccessThreshold    int                                     // Successes needed to close from half-open
	Timeout             time.Duration                           // How long circuit stays open
	HalfOpenMaxRequests int                                     // Max requests allowed in half-open state
	WindowDuration      time.Duration                           // Sliding window duration for counting failures
	OnStateChange       func(name string, from, to CircuitState) // State change callback
	IsFailure           func(err error) bool                    // Custom failure classifier
}

// DefaultCircuitBreakerConfig returns a default configuration.
func DefaultCircuitBreakerConfig(name string) *CircuitBreakerConfig {
	return &CircuitBreakerConfig{
		Name:                name,
		FailureThreshold:    5,
		SuccessThreshold:    3,
		Timeout:             30 * time.Second,
		HalfOpenMaxRequests: 3,
		WindowDuration:      60 * time.Second,
	}
}

// CircuitBreaker implements the circuit breaker pattern.
type CircuitBreaker struct {
	config *CircuitBreakerConfig

	mu               sync.RWMutex
	state            CircuitState
	failures         int
	successes        int
	halfOpenRequests int
	lastFailureTime  time.Time
	openedAt         time.Time

	// Sliding window
	window       *slidingWindow
	windowMu     sync.Mutex
	windowPeriod time.Duration

	// Metrics
	totalRequests    uint64
	totalFailures    uint64
	totalSuccesses   uint64
	consecutiveFails int
	lastStateChange  time.Time
}

// slidingWindow implements a time-based sliding window for counting events.
type slidingWindow struct {
	buckets    []int
	bucketSize time.Duration
	numBuckets int
	current    int
	lastUpdate time.Time
}

// newSlidingWindow creates a new sliding window.
func newSlidingWindow(windowDuration time.Duration, numBuckets int) *slidingWindow {
	if numBuckets <= 0 {
		numBuckets = 10
	}

	return &slidingWindow{
		buckets:    make([]int, numBuckets),
		bucketSize: windowDuration / time.Duration(numBuckets),
		numBuckets: numBuckets,
		lastUpdate: time.Now(),
	}
}

// increment increments the current bucket.
func (w *slidingWindow) increment() {
	now := time.Now()
	w.advance(now)
	w.buckets[w.current]++
}

// sum returns the sum of all buckets.
func (w *slidingWindow) sum() int {
	now := time.Now()
	w.advance(now)

	total := 0
	for _, count := range w.buckets {
		total += count
	}
	return total
}

// advance moves the window forward in time, clearing old buckets.
func (w *slidingWindow) advance(now time.Time) {
	elapsed := now.Sub(w.lastUpdate)
	bucketsToAdvance := int(elapsed / w.bucketSize)

	if bucketsToAdvance > w.numBuckets {
		bucketsToAdvance = w.numBuckets
	}

	for i := 0; i < bucketsToAdvance; i++ {
		w.current = (w.current + 1) % w.numBuckets
		w.buckets[w.current] = 0
	}

	w.lastUpdate = now
}

// reset clears all buckets.
func (w *slidingWindow) reset() {
	for i := range w.buckets {
		w.buckets[i] = 0
	}
}

// NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker(config *CircuitBreakerConfig) *CircuitBreaker {
	if config == nil {
		config = DefaultCircuitBreakerConfig("default")
	}

	// Apply defaults
	if config.FailureThreshold <= 0 {
		config.FailureThreshold = 5
	}
	if config.SuccessThreshold <= 0 {
		config.SuccessThreshold = 3
	}
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}
	if config.HalfOpenMaxRequests <= 0 {
		config.HalfOpenMaxRequests = 3
	}
	if config.WindowDuration <= 0 {
		config.WindowDuration = 60 * time.Second
	}

	cb := &CircuitBreaker{
		config:          config,
		state:           CircuitStateClosed,
		window:          newSlidingWindow(config.WindowDuration, 10),
		windowPeriod:    config.WindowDuration,
		lastStateChange: time.Now(),
	}

	return cb
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.currentState()
}

// currentState returns the current state, potentially transitioning from open to half-open.
// Must be called with at least a read lock held.
func (cb *CircuitBreaker) currentState() CircuitState {
	if cb.state == CircuitStateOpen {
		if time.Since(cb.openedAt) >= cb.config.Timeout {
			return CircuitStateHalfOpen
		}
	}
	return cb.state
}

// Allow checks if a request should be allowed.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	state := cb.currentState()

	switch state {
	case CircuitStateClosed:
		return true

	case CircuitStateOpen:
		return false

	case CircuitStateHalfOpen:
		if cb.halfOpenRequests >= cb.config.HalfOpenMaxRequests {
			return false
		}
		cb.halfOpenRequests++
		// Transition to half-open if needed
		if cb.state == CircuitStateOpen {
			cb.setState(CircuitStateHalfOpen)
		}
		return true

	default:
		return false
	}
}

// Execute runs a function with circuit breaker protection.
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func(context.Context) error) error {
	if !cb.Allow() {
		return ErrCircuitBreakerOpen
	}

	atomic.AddUint64(&cb.totalRequests, 1)

	err := fn(ctx)

	if err != nil {
		cb.RecordFailure()
	} else {
		cb.RecordSuccess()
	}

	return err
}

// ExecuteWithFallback runs a function with fallback on circuit open.
func (cb *CircuitBreaker) ExecuteWithFallback(ctx context.Context, fn func(context.Context) error, fallback func(context.Context, error) error) error {
	err := cb.Execute(ctx, fn)
	if err != nil && fallback != nil {
		return fallback(ctx, err)
	}
	return err
}

// RecordSuccess records a successful request.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	atomic.AddUint64(&cb.totalSuccesses, 1)
	cb.consecutiveFails = 0

	switch cb.currentState() {
	case CircuitStateClosed:
		cb.failures = 0
		cb.window.reset()

	case CircuitStateHalfOpen:
		cb.successes++
		if cb.successes >= cb.config.SuccessThreshold {
			cb.setState(CircuitStateClosed)
		}
	}
}

// RecordFailure records a failed request.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	atomic.AddUint64(&cb.totalFailures, 1)
	cb.lastFailureTime = time.Now()
	cb.consecutiveFails++

	cb.windowMu.Lock()
	cb.window.increment()
	failuresInWindow := cb.window.sum()
	cb.windowMu.Unlock()

	switch cb.currentState() {
	case CircuitStateClosed:
		cb.failures++
		// Check both consecutive failures and windowed failures
		if cb.failures >= cb.config.FailureThreshold || failuresInWindow >= cb.config.FailureThreshold {
			cb.setState(CircuitStateOpen)
		}

	case CircuitStateHalfOpen:
		// Any failure in half-open state opens the circuit
		cb.setState(CircuitStateOpen)
	}
}

// setState transitions to a new state.
func (cb *CircuitBreaker) setState(newState CircuitState) {
	if cb.state == newState {
		return
	}

	oldState := cb.state
	cb.state = newState
	cb.lastStateChange = time.Now()

	// Reset counters on state change
	cb.failures = 0
	cb.successes = 0
	cb.halfOpenRequests = 0

	if newState == CircuitStateOpen {
		cb.openedAt = time.Now()
	}

	if newState == CircuitStateClosed {
		cb.windowMu.Lock()
		cb.window.reset()
		cb.windowMu.Unlock()
	}

	// Notify callback
	if cb.config.OnStateChange != nil {
		go cb.config.OnStateChange(cb.config.Name, oldState, newState)
	}
}

// Reset resets the circuit breaker to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = CircuitStateClosed
	cb.failures = 0
	cb.successes = 0
	cb.halfOpenRequests = 0
	cb.consecutiveFails = 0
	cb.lastFailureTime = time.Time{}
	cb.openedAt = time.Time{}
	cb.lastStateChange = time.Now()

	cb.windowMu.Lock()
	cb.window.reset()
	cb.windowMu.Unlock()
}

// CheckState checks and potentially transitions the state.
func (cb *CircuitBreaker) CheckState() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	state := cb.currentState()
	if state == CircuitStateHalfOpen && cb.state == CircuitStateOpen {
		cb.setState(CircuitStateHalfOpen)
	}
	return cb.state
}

// Metrics returns circuit breaker metrics.
func (cb *CircuitBreaker) Metrics() CircuitBreakerMetrics {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return CircuitBreakerMetrics{
		Name:             cb.config.Name,
		State:            cb.currentState(),
		TotalRequests:    atomic.LoadUint64(&cb.totalRequests),
		TotalFailures:    atomic.LoadUint64(&cb.totalFailures),
		TotalSuccesses:   atomic.LoadUint64(&cb.totalSuccesses),
		ConsecutiveFails: cb.consecutiveFails,
		LastFailureTime:  cb.lastFailureTime,
		LastStateChange:  cb.lastStateChange,
		Failures:         cb.failures,
		Successes:        cb.successes,
		HalfOpenRequests: cb.halfOpenRequests,
	}
}

// CircuitBreakerMetrics holds metrics for a circuit breaker.
type CircuitBreakerMetrics struct {
	Name             string
	State            CircuitState
	TotalRequests    uint64
	TotalFailures    uint64
	TotalSuccesses   uint64
	ConsecutiveFails int
	LastFailureTime  time.Time
	LastStateChange  time.Time
	Failures         int
	Successes        int
	HalfOpenRequests int
}

// FailureRate returns the failure rate as a percentage.
func (m *CircuitBreakerMetrics) FailureRate() float64 {
	if m.TotalRequests == 0 {
		return 0
	}
	return float64(m.TotalFailures) / float64(m.TotalRequests) * 100
}

// CircuitBreakerRegistry manages multiple circuit breakers.
type CircuitBreakerRegistry struct {
	mu       sync.RWMutex
	breakers map[string]*CircuitBreaker
	config   *CircuitBreakerConfig // Default config template
}

// NewCircuitBreakerRegistry creates a new circuit breaker registry.
func NewCircuitBreakerRegistry(defaultConfig *CircuitBreakerConfig) *CircuitBreakerRegistry {
	if defaultConfig == nil {
		defaultConfig = DefaultCircuitBreakerConfig("default")
	}

	return &CircuitBreakerRegistry{
		breakers: make(map[string]*CircuitBreaker),
		config:   defaultConfig,
	}
}

// Get returns the circuit breaker for a service, creating one if needed.
func (r *CircuitBreakerRegistry) Get(service string) *CircuitBreaker {
	r.mu.RLock()
	cb, ok := r.breakers[service]
	r.mu.RUnlock()

	if ok {
		return cb
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check
	if cb, ok := r.breakers[service]; ok {
		return cb
	}

	// Create new circuit breaker with default config
	config := &CircuitBreakerConfig{
		Name:                service,
		FailureThreshold:    r.config.FailureThreshold,
		SuccessThreshold:    r.config.SuccessThreshold,
		Timeout:             r.config.Timeout,
		HalfOpenMaxRequests: r.config.HalfOpenMaxRequests,
		WindowDuration:      r.config.WindowDuration,
		OnStateChange:       r.config.OnStateChange,
		IsFailure:           r.config.IsFailure,
	}

	cb = NewCircuitBreaker(config)
	r.breakers[service] = cb
	return cb
}

// GetOrCreate gets or creates a circuit breaker with custom config.
func (r *CircuitBreakerRegistry) GetOrCreate(service string, config *CircuitBreakerConfig) *CircuitBreaker {
	r.mu.RLock()
	cb, ok := r.breakers[service]
	r.mu.RUnlock()

	if ok {
		return cb
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if cb, ok := r.breakers[service]; ok {
		return cb
	}

	if config == nil {
		config = r.config
	}
	config.Name = service

	cb = NewCircuitBreaker(config)
	r.breakers[service] = cb
	return cb
}

// Remove removes a circuit breaker.
func (r *CircuitBreakerRegistry) Remove(service string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.breakers, service)
}

// Reset resets all circuit breakers.
func (r *CircuitBreakerRegistry) ResetAll() {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, cb := range r.breakers {
		cb.Reset()
	}
}

// Stats returns metrics for all circuit breakers.
func (r *CircuitBreakerRegistry) Stats() map[string]CircuitBreakerMetrics {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := make(map[string]CircuitBreakerMetrics, len(r.breakers))
	for name, cb := range r.breakers {
		stats[name] = cb.Metrics()
	}
	return stats
}

// List returns all circuit breaker names.
func (r *CircuitBreakerRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.breakers))
	for name := range r.breakers {
		names = append(names, name)
	}
	return names
}

// Bulkhead implements the bulkhead pattern for isolation.
type Bulkhead struct {
	name            string
	maxConcurrent   int
	maxWaiting      int
	semaphore       chan struct{}
	waitQueue       chan struct{}
	timeout         time.Duration
	mu              sync.RWMutex
	activeRequests  int64
	waitingRequests int64
	totalRequests   uint64
	rejectedTotal   uint64
}

// BulkheadConfig configures a bulkhead.
type BulkheadConfig struct {
	Name          string
	MaxConcurrent int
	MaxWaiting    int
	Timeout       time.Duration
}

// DefaultBulkheadConfig returns default bulkhead configuration.
func DefaultBulkheadConfig(name string) *BulkheadConfig {
	return &BulkheadConfig{
		Name:          name,
		MaxConcurrent: 10,
		MaxWaiting:    100,
		Timeout:       10 * time.Second,
	}
}

// NewBulkhead creates a new bulkhead.
func NewBulkhead(config *BulkheadConfig) *Bulkhead {
	if config == nil {
		config = DefaultBulkheadConfig("default")
	}

	if config.MaxConcurrent <= 0 {
		config.MaxConcurrent = 10
	}
	if config.MaxWaiting <= 0 {
		config.MaxWaiting = 100
	}
	if config.Timeout <= 0 {
		config.Timeout = 10 * time.Second
	}

	return &Bulkhead{
		name:          config.Name,
		maxConcurrent: config.MaxConcurrent,
		maxWaiting:    config.MaxWaiting,
		semaphore:     make(chan struct{}, config.MaxConcurrent),
		waitQueue:     make(chan struct{}, config.MaxWaiting),
		timeout:       config.Timeout,
	}
}

// Acquire acquires a permit from the bulkhead.
func (b *Bulkhead) Acquire(ctx context.Context) error {
	atomic.AddUint64(&b.totalRequests, 1)
	atomic.AddInt64(&b.waitingRequests, 1)
	defer atomic.AddInt64(&b.waitingRequests, -1)

	// Try to acquire immediately
	select {
	case b.semaphore <- struct{}{}:
		atomic.AddInt64(&b.activeRequests, 1)
		return nil
	default:
	}

	// Check if we can wait
	select {
	case b.waitQueue <- struct{}{}:
	default:
		atomic.AddUint64(&b.rejectedTotal, 1)
		return errors.New("bulkhead wait queue full")
	}
	defer func() { <-b.waitQueue }()

	// Wait for permit with timeout
	ctx, cancel := context.WithTimeout(ctx, b.timeout)
	defer cancel()

	select {
	case b.semaphore <- struct{}{}:
		atomic.AddInt64(&b.activeRequests, 1)
		return nil
	case <-ctx.Done():
		atomic.AddUint64(&b.rejectedTotal, 1)
		return ctx.Err()
	}
}

// Release releases a permit back to the bulkhead.
func (b *Bulkhead) Release() {
	atomic.AddInt64(&b.activeRequests, -1)
	<-b.semaphore
}

// Execute runs a function with bulkhead protection.
func (b *Bulkhead) Execute(ctx context.Context, fn func(context.Context) error) error {
	if err := b.Acquire(ctx); err != nil {
		return err
	}
	defer b.Release()

	return fn(ctx)
}

// Metrics returns bulkhead metrics.
func (b *Bulkhead) Metrics() BulkheadMetrics {
	return BulkheadMetrics{
		Name:            b.name,
		MaxConcurrent:   b.maxConcurrent,
		MaxWaiting:      b.maxWaiting,
		ActiveRequests:  atomic.LoadInt64(&b.activeRequests),
		WaitingRequests: atomic.LoadInt64(&b.waitingRequests),
		TotalRequests:   atomic.LoadUint64(&b.totalRequests),
		RejectedTotal:   atomic.LoadUint64(&b.rejectedTotal),
	}
}

// BulkheadMetrics holds metrics for a bulkhead.
type BulkheadMetrics struct {
	Name            string
	MaxConcurrent   int
	MaxWaiting      int
	ActiveRequests  int64
	WaitingRequests int64
	TotalRequests   uint64
	RejectedTotal   uint64
}

// RetryPolicy defines retry behavior.
type RetryPolicy struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Multiplier     float64
	Jitter         float64 // 0.0 to 1.0
	RetryOn        func(error) bool
}

// DefaultRetryPolicy returns a default retry policy.
func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxRetries:     3,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     10 * time.Second,
		Multiplier:     2.0,
		Jitter:         0.1,
	}
}

// Retrier implements retry logic.
type Retrier struct {
	policy *RetryPolicy
}

// NewRetrier creates a new retrier.
func NewRetrier(policy *RetryPolicy) *Retrier {
	if policy == nil {
		policy = DefaultRetryPolicy()
	}

	return &Retrier{
		policy: policy,
	}
}

// Execute runs a function with retry logic.
func (r *Retrier) Execute(ctx context.Context, fn func(context.Context) error) error {
	var lastErr error

	for attempt := 0; attempt <= r.policy.MaxRetries; attempt++ {
		if attempt > 0 {
			// Calculate backoff
			backoff := r.calculateBackoff(attempt)

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		err := fn(ctx)
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if we should retry
		if r.policy.RetryOn != nil && !r.policy.RetryOn(err) {
			return err
		}

		// Don't retry context errors
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
	}

	return lastErr
}

// calculateBackoff calculates the backoff duration for an attempt.
func (r *Retrier) calculateBackoff(attempt int) time.Duration {
	backoff := float64(r.policy.InitialBackoff)
	for i := 1; i < attempt; i++ {
		backoff *= r.policy.Multiplier
	}

	if backoff > float64(r.policy.MaxBackoff) {
		backoff = float64(r.policy.MaxBackoff)
	}

	// Add jitter
	if r.policy.Jitter > 0 {
		jitter := backoff * r.policy.Jitter
		// Use current time nanoseconds as pseudo-random source
		jitterMult := float64(time.Now().UnixNano()%1000) / 1000
		backoff = backoff - jitter + (jitter * 2 * jitterMult)
	}

	return time.Duration(backoff)
}

// RateLimiter implements token bucket rate limiting.
type RateLimiter struct {
	config     *RateLimiterConfig
	mu         sync.Mutex
	tokens     float64
	lastRefill time.Time
	requests   uint64
	allowed    uint64
	denied     uint64
}

// RateLimiterConfig configures the rate limiter.
type RateLimiterConfig struct {
	Rate      float64 // Requests per second
	Burst     int     // Maximum burst size
	Algorithm string  // "token-bucket" or "sliding-window"
}

// DefaultRateLimiterConfig returns default rate limiter configuration.
func DefaultRateLimiterConfig() *RateLimiterConfig {
	return &RateLimiterConfig{
		Rate:      100,
		Burst:     10,
		Algorithm: "token-bucket",
	}
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(config *RateLimiterConfig) *RateLimiter {
	if config == nil {
		config = DefaultRateLimiterConfig()
	}

	if config.Rate <= 0 {
		config.Rate = 100
	}
	if config.Burst <= 0 {
		config.Burst = int(config.Rate)
	}

	return &RateLimiter{
		config:     config,
		tokens:     float64(config.Burst),
		lastRefill: time.Now(),
	}
}

// Allow checks if a request should be allowed.
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	atomic.AddUint64(&rl.requests, 1)

	// Refill tokens
	now := time.Now()
	elapsed := now.Sub(rl.lastRefill).Seconds()
	rl.tokens += elapsed * rl.config.Rate
	if rl.tokens > float64(rl.config.Burst) {
		rl.tokens = float64(rl.config.Burst)
	}
	rl.lastRefill = now

	// Try to take a token
	if rl.tokens >= 1 {
		rl.tokens--
		atomic.AddUint64(&rl.allowed, 1)
		return true
	}

	atomic.AddUint64(&rl.denied, 1)
	return false
}

// AllowN checks if n requests should be allowed.
func (rl *RateLimiter) AllowN(n int) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Refill tokens
	now := time.Now()
	elapsed := now.Sub(rl.lastRefill).Seconds()
	rl.tokens += elapsed * rl.config.Rate
	if rl.tokens > float64(rl.config.Burst) {
		rl.tokens = float64(rl.config.Burst)
	}
	rl.lastRefill = now

	// Try to take n tokens
	if rl.tokens >= float64(n) {
		rl.tokens -= float64(n)
		return true
	}

	return false
}

// Wait blocks until a request is allowed or context is cancelled.
func (rl *RateLimiter) Wait(ctx context.Context) error {
	for {
		if rl.Allow() {
			return nil
		}

		// Calculate wait time until next token
		rl.mu.Lock()
		tokensNeeded := 1 - rl.tokens
		if tokensNeeded < 0 {
			tokensNeeded = 0
		}
		waitTime := time.Duration(tokensNeeded / rl.config.Rate * float64(time.Second))
		rl.mu.Unlock()

		if waitTime < time.Millisecond {
			waitTime = time.Millisecond
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
		}
	}
}

// Metrics returns rate limiter metrics.
func (rl *RateLimiter) Metrics() RateLimiterMetrics {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	return RateLimiterMetrics{
		Rate:           rl.config.Rate,
		Burst:          rl.config.Burst,
		AvailableTokens: rl.tokens,
		TotalRequests:  atomic.LoadUint64(&rl.requests),
		AllowedTotal:   atomic.LoadUint64(&rl.allowed),
		DeniedTotal:    atomic.LoadUint64(&rl.denied),
	}
}

// RateLimiterMetrics holds rate limiter metrics.
type RateLimiterMetrics struct {
	Rate           float64
	Burst          int
	AvailableTokens float64
	TotalRequests  uint64
	AllowedTotal   uint64
	DeniedTotal    uint64
}

// HealthChecker performs active health checking on endpoints.
type HealthChecker struct {
	config   *HealthCheckConfig
	mu       sync.RWMutex
	health   map[string]*endpointHealth
	stopCh   chan struct{}
	wg       sync.WaitGroup
	running  bool
}

// HealthCheckConfig configures the health checker.
type HealthCheckConfig struct {
	Interval           time.Duration
	Timeout            time.Duration
	HealthyThreshold   int
	UnhealthyThreshold int
	OnHealthChange     func(endpointID string, healthy bool)
}

// DefaultHealthCheckConfig returns default health check configuration.
func DefaultHealthCheckConfig() *HealthCheckConfig {
	return &HealthCheckConfig{
		Interval:           10 * time.Second,
		Timeout:            5 * time.Second,
		HealthyThreshold:   2,
		UnhealthyThreshold: 3,
	}
}

type endpointHealth struct {
	endpoint          *Endpoint
	healthy           bool
	consecutiveOK     int
	consecutiveFail   int
	lastCheck         time.Time
	lastSuccess       time.Time
	lastFailure       time.Time
	totalChecks       uint64
	totalSuccesses    uint64
	totalFailures     uint64
}

// NewHealthChecker creates a new health checker.
func NewHealthChecker(config *HealthCheckConfig) *HealthChecker {
	if config == nil {
		config = DefaultHealthCheckConfig()
	}

	return &HealthChecker{
		config: config,
		health: make(map[string]*endpointHealth),
		stopCh: make(chan struct{}),
	}
}

// AddEndpoint adds an endpoint for health checking.
func (hc *HealthChecker) AddEndpoint(endpoint *Endpoint) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	if _, ok := hc.health[endpoint.ID]; !ok {
		hc.health[endpoint.ID] = &endpointHealth{
			endpoint: endpoint,
			healthy:  true, // Assume healthy until proven otherwise
		}
	}
}

// RemoveEndpoint removes an endpoint from health checking.
func (hc *HealthChecker) RemoveEndpoint(endpointID string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	delete(hc.health, endpointID)
}

// RemoveService removes all endpoints for a service.
func (hc *HealthChecker) RemoveService(serviceName string) {
	// This is a placeholder - in production would filter by service
	// For now, do nothing as we'd need service info stored per endpoint
}

// IsHealthy returns whether an endpoint is healthy.
func (hc *HealthChecker) IsHealthy(endpointID string) bool {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	if h, ok := hc.health[endpointID]; ok {
		return h.healthy
	}
	return true // Unknown endpoints are assumed healthy
}

// RecordSuccess records a successful request to an endpoint.
func (hc *HealthChecker) RecordSuccess(endpointID string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	h, ok := hc.health[endpointID]
	if !ok {
		return
	}

	h.consecutiveOK++
	h.consecutiveFail = 0
	h.lastSuccess = time.Now()
	h.totalSuccesses++

	// Mark healthy if threshold reached
	if !h.healthy && h.consecutiveOK >= hc.config.HealthyThreshold {
		h.healthy = true
		if hc.config.OnHealthChange != nil {
			go hc.config.OnHealthChange(endpointID, true)
		}
	}
}

// RecordFailure records a failed request to an endpoint.
func (hc *HealthChecker) RecordFailure(endpointID string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	h, ok := hc.health[endpointID]
	if !ok {
		return
	}

	h.consecutiveFail++
	h.consecutiveOK = 0
	h.lastFailure = time.Now()
	h.totalFailures++

	// Mark unhealthy if threshold reached
	if h.healthy && h.consecutiveFail >= hc.config.UnhealthyThreshold {
		h.healthy = false
		if hc.config.OnHealthChange != nil {
			go hc.config.OnHealthChange(endpointID, false)
		}
	}
}

// Run starts the health checker.
func (hc *HealthChecker) Run(ctx context.Context) {
	hc.mu.Lock()
	if hc.running {
		hc.mu.Unlock()
		return
	}
	hc.running = true
	hc.mu.Unlock()

	ticker := time.NewTicker(hc.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-hc.stopCh:
			return
		case <-ticker.C:
			// In production, this would perform actual health checks
			// For now, we rely on passive health checking via RecordSuccess/RecordFailure
		}
	}
}

// Stop stops the health checker.
func (hc *HealthChecker) Stop() {
	close(hc.stopCh)
}
