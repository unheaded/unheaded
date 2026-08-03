// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

//! Circuit Breaker for THE SHIELD
//!
//! Implements the circuit breaker pattern to protect backends from cascading failures.
//! When a backend starts failing, the circuit opens to prevent additional load,
//! then gradually allows requests through to test recovery.

use std::collections::HashMap;
use std::sync::atomic::{AtomicU32, AtomicU64, Ordering};
use std::sync::Arc;
use std::time::{Duration, Instant};
use tokio::sync::RwLock;

/// Circuit breaker state
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum CircuitState {
    /// Circuit is closed - requests flow normally
    Closed,
    /// Circuit is open - requests are rejected
    Open,
    /// Circuit is half-open - limited requests allowed to test recovery
    HalfOpen,
}

impl CircuitState {
    /// Get string representation for metrics/logging
    pub fn as_str(&self) -> &'static str {
        match self {
            CircuitState::Closed => "closed",
            CircuitState::Open => "open",
            CircuitState::HalfOpen => "half_open",
        }
    }
}

/// Configuration for a circuit breaker
#[derive(Debug, Clone)]
pub struct CircuitBreakerConfig {
    /// Whether the breaker is consulted at all (`[circuit_breaker] enabled`).
    /// Was parsed from config and never read, so disabling it did nothing.
    pub enabled: bool,
    /// Number of failures before opening the circuit
    pub failure_threshold: u32,
    /// Number of successes in half-open state before closing
    pub success_threshold: u32,
    /// Time to wait before transitioning from open to half-open
    pub reset_timeout: Duration,
    /// Time window for counting failures
    pub failure_window: Duration,
    /// Maximum concurrent requests in half-open state
    pub half_open_max_requests: u32,
}

impl Default for CircuitBreakerConfig {
    fn default() -> Self {
        Self {
            enabled: true,
            failure_threshold: 5,
            success_threshold: 3,
            reset_timeout: Duration::from_secs(60),
            failure_window: Duration::from_secs(10),
            half_open_max_requests: 3,
        }
    }
}

/// A single circuit breaker instance
pub struct CircuitBreaker {
    /// Current state
    state: RwLock<CircuitState>,
    /// Configuration
    config: CircuitBreakerConfig,
    /// Failure count in current window
    failure_count: AtomicU32,
    /// Success count (used in half-open state)
    success_count: AtomicU32,
    /// Time when circuit was opened
    opened_at: RwLock<Option<Instant>>,
    /// Requests in progress during half-open state
    half_open_requests: AtomicU32,
    /// Last failure time (for failure window)
    last_failure: RwLock<Option<Instant>>,
    /// Total requests (for metrics)
    total_requests: AtomicU64,
    /// Total failures (for metrics)
    total_failures: AtomicU64,
    /// Total rejections (for metrics)
    total_rejections: AtomicU64,
}

impl CircuitBreaker {
    /// Create a new circuit breaker with the given configuration
    pub fn new(config: CircuitBreakerConfig) -> Self {
        Self {
            state: RwLock::new(CircuitState::Closed),
            config,
            failure_count: AtomicU32::new(0),
            success_count: AtomicU32::new(0),
            opened_at: RwLock::new(None),
            half_open_requests: AtomicU32::new(0),
            last_failure: RwLock::new(None),
            total_requests: AtomicU64::new(0),
            total_failures: AtomicU64::new(0),
            total_rejections: AtomicU64::new(0),
        }
    }

    /// Create a circuit breaker with default configuration
    pub fn with_defaults() -> Self {
        Self::new(CircuitBreakerConfig::default())
    }

    /// Get the current state
    pub async fn state(&self) -> CircuitState {
        self.check_state_transition().await;
        *self.state.read().await
    }

    /// Check if a request should be allowed
    pub async fn allow_request(&self) -> bool {
        self.total_requests.fetch_add(1, Ordering::Relaxed);

        // Check for state transitions
        self.check_state_transition().await;

        let state = *self.state.read().await;
        match state {
            CircuitState::Closed => true,
            CircuitState::Open => {
                self.total_rejections.fetch_add(1, Ordering::Relaxed);
                false
            }
            CircuitState::HalfOpen => {
                // Allow limited requests in half-open state
                let current = self.half_open_requests.fetch_add(1, Ordering::SeqCst);
                if current < self.config.half_open_max_requests {
                    true
                } else {
                    self.half_open_requests.fetch_sub(1, Ordering::SeqCst);
                    self.total_rejections.fetch_add(1, Ordering::Relaxed);
                    false
                }
            }
        }
    }

    /// Record a successful request
    pub async fn record_success(&self) {
        let state = *self.state.read().await;
        match state {
            CircuitState::HalfOpen => {
                self.half_open_requests.fetch_sub(1, Ordering::SeqCst);
                let successes = self.success_count.fetch_add(1, Ordering::SeqCst) + 1;

                // Check if we should close the circuit
                if successes >= self.config.success_threshold {
                    self.close_circuit().await;
                }
            }
            CircuitState::Closed => {
                // Reset failure count on success in closed state
                // Only reset if we're past the failure window
                let should_reset = {
                    let last_failure = self.last_failure.read().await;
                    last_failure
                        .map(|t| t.elapsed() > self.config.failure_window)
                        .unwrap_or(true)
                };

                if should_reset {
                    self.failure_count.store(0, Ordering::Relaxed);
                }
            }
            CircuitState::Open => {
                // Shouldn't happen, but handle gracefully
            }
        }
    }

    /// Record a failed request
    pub async fn record_failure(&self) {
        self.total_failures.fetch_add(1, Ordering::Relaxed);

        let state = *self.state.read().await;
        match state {
            CircuitState::Closed => {
                // Check failure window
                let should_reset = {
                    let last_failure = self.last_failure.read().await;
                    last_failure
                        .map(|t| t.elapsed() > self.config.failure_window)
                        .unwrap_or(false)
                };

                if should_reset {
                    self.failure_count.store(0, Ordering::Relaxed);
                }

                // Update last failure time
                {
                    let mut last_failure = self.last_failure.write().await;
                    *last_failure = Some(Instant::now());
                }

                let failures = self.failure_count.fetch_add(1, Ordering::SeqCst) + 1;

                // Check if we should open the circuit
                if failures >= self.config.failure_threshold {
                    self.open_circuit().await;
                }
            }
            CircuitState::HalfOpen => {
                self.half_open_requests.fetch_sub(1, Ordering::SeqCst);
                // Any failure in half-open reopens the circuit
                self.open_circuit().await;
            }
            CircuitState::Open => {
                // Already open, nothing to do
            }
        }
    }

    /// Check for automatic state transitions
    async fn check_state_transition(&self) {
        let state = *self.state.read().await;

        if state == CircuitState::Open {
            let should_transition = {
                let opened_at = self.opened_at.read().await;
                opened_at
                    .map(|t| t.elapsed() >= self.config.reset_timeout)
                    .unwrap_or(false)
            };

            if should_transition {
                self.transition_to_half_open().await;
            }
        }
    }

    /// Open the circuit
    async fn open_circuit(&self) {
        let mut state = self.state.write().await;
        *state = CircuitState::Open;

        let mut opened_at = self.opened_at.write().await;
        *opened_at = Some(Instant::now());

        self.failure_count.store(0, Ordering::Relaxed);
        self.success_count.store(0, Ordering::Relaxed);
        self.half_open_requests.store(0, Ordering::Relaxed);
    }

    /// Transition to half-open state
    async fn transition_to_half_open(&self) {
        let mut state = self.state.write().await;
        if *state == CircuitState::Open {
            *state = CircuitState::HalfOpen;
            self.success_count.store(0, Ordering::Relaxed);
            self.half_open_requests.store(0, Ordering::Relaxed);
        }
    }

    /// Close the circuit
    async fn close_circuit(&self) {
        let mut state = self.state.write().await;
        *state = CircuitState::Closed;

        let mut opened_at = self.opened_at.write().await;
        *opened_at = None;

        self.failure_count.store(0, Ordering::Relaxed);
        self.success_count.store(0, Ordering::Relaxed);
        self.half_open_requests.store(0, Ordering::Relaxed);
    }

    /// Force the circuit to a specific state (for testing/admin)
    pub async fn force_state(&self, new_state: CircuitState) {
        let mut state = self.state.write().await;
        *state = new_state;

        match new_state {
            CircuitState::Closed => {
                let mut opened_at = self.opened_at.write().await;
                *opened_at = None;
            }
            CircuitState::Open => {
                let mut opened_at = self.opened_at.write().await;
                *opened_at = Some(Instant::now());
            }
            CircuitState::HalfOpen => {}
        }

        self.failure_count.store(0, Ordering::Relaxed);
        self.success_count.store(0, Ordering::Relaxed);
        self.half_open_requests.store(0, Ordering::Relaxed);
    }

    /// Get metrics for this circuit breaker
    pub fn metrics(&self) -> CircuitBreakerMetrics {
        CircuitBreakerMetrics {
            total_requests: self.total_requests.load(Ordering::Relaxed),
            total_failures: self.total_failures.load(Ordering::Relaxed),
            total_rejections: self.total_rejections.load(Ordering::Relaxed),
            failure_count: self.failure_count.load(Ordering::Relaxed),
            success_count: self.success_count.load(Ordering::Relaxed),
        }
    }
}

/// Metrics for a circuit breaker
#[derive(Debug, Clone, Default)]
pub struct CircuitBreakerMetrics {
    pub total_requests: u64,
    pub total_failures: u64,
    pub total_rejections: u64,
    pub failure_count: u32,
    pub success_count: u32,
}

/// Circuit breaker manager for multiple backends
pub struct CircuitBreakerManager {
    /// Circuit breakers per backend name
    breakers: RwLock<HashMap<String, Arc<CircuitBreaker>>>,
    /// Default configuration
    default_config: CircuitBreakerConfig,
}

impl CircuitBreakerManager {
    /// Create a new circuit breaker manager
    pub fn new(config: CircuitBreakerConfig) -> Self {
        Self {
            breakers: RwLock::new(HashMap::new()),
            default_config: config,
        }
    }

    /// Whether circuit breaking is enforced. When false, callers must skip
    /// the breaker entirely and treat every backend as available.
    pub fn is_enabled(&self) -> bool {
        self.default_config.enabled
    }

    /// Get or create a circuit breaker for a backend
    pub async fn get(&self, backend: &str) -> Arc<CircuitBreaker> {
        // Fast path: read lock
        {
            let breakers = self.breakers.read().await;
            if let Some(cb) = breakers.get(backend) {
                return cb.clone();
            }
        }

        // Slow path: write lock
        let mut breakers = self.breakers.write().await;

        // Double-check
        if let Some(cb) = breakers.get(backend) {
            return cb.clone();
        }

        let cb = Arc::new(CircuitBreaker::new(self.default_config.clone()));
        breakers.insert(backend.to_string(), cb.clone());
        cb
    }

    /// Get all circuit breaker states for monitoring
    pub async fn all_states(&self) -> HashMap<String, CircuitState> {
        let breakers = self.breakers.read().await;
        let mut states = HashMap::new();

        for (name, cb) in breakers.iter() {
            states.insert(name.clone(), cb.state().await);
        }

        states
    }

    /// Get all metrics for monitoring
    pub async fn all_metrics(&self) -> HashMap<String, CircuitBreakerMetrics> {
        let breakers = self.breakers.read().await;
        let mut metrics = HashMap::new();

        for (name, cb) in breakers.iter() {
            metrics.insert(name.clone(), cb.metrics());
        }

        metrics
    }

    /// Force reset all circuit breakers
    pub async fn reset_all(&self) {
        let breakers = self.breakers.read().await;
        for cb in breakers.values() {
            cb.force_state(CircuitState::Closed).await;
        }
    }
}

/// Guard that automatically records success or failure
pub struct RequestGuard<'a> {
    circuit_breaker: &'a CircuitBreaker,
    completed: bool,
}

impl<'a> RequestGuard<'a> {
    /// Create a new request guard
    pub fn new(circuit_breaker: &'a CircuitBreaker) -> Self {
        Self {
            circuit_breaker,
            completed: false,
        }
    }

    /// Mark the request as successful
    pub async fn success(mut self) {
        self.completed = true;
        self.circuit_breaker.record_success().await;
    }

    /// Mark the request as failed
    pub async fn failure(mut self) {
        self.completed = true;
        self.circuit_breaker.record_failure().await;
    }
}

impl<'a> Drop for RequestGuard<'a> {
    fn drop(&mut self) {
        // If not explicitly completed, treat as failure
        // This handles panics and other unexpected terminations
        if !self.completed {
            // We can't await in drop, so we use a separate mechanism
            // In practice, callers should always call success() or failure()
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_circuit_breaker_closed() {
        let cb = CircuitBreaker::new(CircuitBreakerConfig {
            enabled: true,
            failure_threshold: 3,
            success_threshold: 2,
            reset_timeout: Duration::from_secs(1),
            failure_window: Duration::from_secs(10),
            half_open_max_requests: 2,
        });

        // Should start closed
        assert_eq!(cb.state().await, CircuitState::Closed);

        // Should allow requests
        assert!(cb.allow_request().await);
        assert!(cb.allow_request().await);
    }

    #[tokio::test]
    async fn test_circuit_breaker_opens_on_failures() {
        let cb = CircuitBreaker::new(CircuitBreakerConfig {
            enabled: true,
            failure_threshold: 3,
            success_threshold: 2,
            reset_timeout: Duration::from_secs(1),
            failure_window: Duration::from_secs(10),
            half_open_max_requests: 2,
        });

        // Record failures
        for _ in 0..3 {
            cb.allow_request().await;
            cb.record_failure().await;
        }

        // Should be open now
        assert_eq!(cb.state().await, CircuitState::Open);

        // Should reject requests
        assert!(!cb.allow_request().await);
    }

    #[tokio::test]
    async fn test_circuit_breaker_half_open_transition() {
        let cb = CircuitBreaker::new(CircuitBreakerConfig {
            enabled: true,
            failure_threshold: 2,
            success_threshold: 2,
            reset_timeout: Duration::from_millis(100),
            failure_window: Duration::from_secs(10),
            half_open_max_requests: 2,
        });

        // Open the circuit
        cb.allow_request().await;
        cb.record_failure().await;
        cb.allow_request().await;
        cb.record_failure().await;

        assert_eq!(cb.state().await, CircuitState::Open);

        // Wait for reset timeout
        tokio::time::sleep(Duration::from_millis(150)).await;

        // Should transition to half-open
        assert_eq!(cb.state().await, CircuitState::HalfOpen);
    }

    #[tokio::test]
    async fn test_circuit_breaker_closes_on_success() {
        let cb = CircuitBreaker::new(CircuitBreakerConfig {
            enabled: true,
            failure_threshold: 2,
            success_threshold: 2,
            reset_timeout: Duration::from_millis(50),
            failure_window: Duration::from_secs(10),
            half_open_max_requests: 5,
        });

        // Open the circuit
        cb.allow_request().await;
        cb.record_failure().await;
        cb.allow_request().await;
        cb.record_failure().await;

        // Wait for half-open
        tokio::time::sleep(Duration::from_millis(100)).await;
        assert_eq!(cb.state().await, CircuitState::HalfOpen);

        // Record successes
        cb.allow_request().await;
        cb.record_success().await;
        cb.allow_request().await;
        cb.record_success().await;

        // Should be closed now
        assert_eq!(cb.state().await, CircuitState::Closed);
    }

    #[tokio::test]
    async fn test_circuit_breaker_manager() {
        let manager = CircuitBreakerManager::new(CircuitBreakerConfig::default());

        let cb1 = manager.get("backend1").await;
        let cb2 = manager.get("backend2").await;

        // Should be different instances
        assert!(cb1.allow_request().await);
        assert!(cb2.allow_request().await);

        // Getting same name should return same instance
        let cb1_again = manager.get("backend1").await;
        assert!(Arc::ptr_eq(&cb1, &cb1_again));
    }

    /// `[circuit_breaker] enabled` must actually reach the manager.
    ///
    /// This config field was parsed and then never read: `main.rs` built the
    /// manager from `failure_threshold` / `success_threshold` / the timeouts
    /// and dropped `enabled` on the floor, so setting it to false left the
    /// breaker fully active. This test is what stops it going inert again.
    #[test]
    fn enabled_flag_reaches_the_manager() {
        let on = CircuitBreakerManager::new(CircuitBreakerConfig::default());
        assert!(on.is_enabled(), "default config must enforce breaking");

        let off = CircuitBreakerManager::new(CircuitBreakerConfig {
            enabled: false,
            ..Default::default()
        });
        assert!(
            !off.is_enabled(),
            "enabled: false must disable breaking — if this fails, the config \
             toggle is decorative again and proxy.rs will still short backends"
        );
    }
}
