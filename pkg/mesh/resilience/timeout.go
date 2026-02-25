// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package resilience provides resilience patterns for the service mesh.
// THE HAUBERK - Timeout management for request handling.
package resilience

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	// ErrTimeout is returned when a request times out.
	ErrTimeout = errors.New("request timeout")
)

// TimeoutConfig configures timeout behavior.
type TimeoutConfig struct {
	// Default is the default timeout for requests.
	Default time.Duration
	// Read is the timeout for reading request bodies.
	Read time.Duration
	// Write is the timeout for writing responses.
	Write time.Duration
	// Idle is the idle connection timeout.
	Idle time.Duration
	// OnTimeout is called when a timeout occurs.
	OnTimeout func(operation string, duration time.Duration)
}

// DefaultTimeoutConfig returns a default timeout configuration.
func DefaultTimeoutConfig() TimeoutConfig {
	return TimeoutConfig{
		Default: 30 * time.Second,
		Read:    10 * time.Second,
		Write:   10 * time.Second,
		Idle:    60 * time.Second,
	}
}

// TimeoutManager manages timeouts for various operations.
type TimeoutManager struct {
	config TimeoutConfig
	mu     sync.RWMutex

	// Service-specific timeouts
	serviceTimeouts map[string]time.Duration
}

// NewTimeoutManager creates a new timeout manager.
func NewTimeoutManager(config TimeoutConfig) *TimeoutManager {
	if config.Default <= 0 {
		config.Default = 30 * time.Second
	}
	if config.Read <= 0 {
		config.Read = 10 * time.Second
	}
	if config.Write <= 0 {
		config.Write = 10 * time.Second
	}
	if config.Idle <= 0 {
		config.Idle = 60 * time.Second
	}

	return &TimeoutManager{
		config:          config,
		serviceTimeouts: make(map[string]time.Duration),
	}
}

// SetServiceTimeout sets a custom timeout for a specific service.
func (tm *TimeoutManager) SetServiceTimeout(service string, timeout time.Duration) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.serviceTimeouts[service] = timeout
}

// GetServiceTimeout returns the timeout for a service.
func (tm *TimeoutManager) GetServiceTimeout(service string) time.Duration {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if timeout, ok := tm.serviceTimeouts[service]; ok {
		return timeout
	}
	return tm.config.Default
}

// WithTimeout executes a function with a timeout context.
func (tm *TimeoutManager) WithTimeout(ctx context.Context, timeout time.Duration, fn func(context.Context) error) error {
	if timeout <= 0 {
		timeout = tm.config.Default
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	done := make(chan error, 1)

	go func() {
		done <- fn(ctx)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		if tm.config.OnTimeout != nil {
			tm.config.OnTimeout("request", timeout)
		}
		return ErrTimeout
	}
}

// WithServiceTimeout executes a function with the service-specific timeout.
func (tm *TimeoutManager) WithServiceTimeout(ctx context.Context, service string, fn func(context.Context) error) error {
	timeout := tm.GetServiceTimeout(service)
	return tm.WithTimeout(ctx, timeout, fn)
}

// ReadTimeout returns the read timeout.
func (tm *TimeoutManager) ReadTimeout() time.Duration {
	return tm.config.Read
}

// WriteTimeout returns the write timeout.
func (tm *TimeoutManager) WriteTimeout() time.Duration {
	return tm.config.Write
}

// IdleTimeout returns the idle timeout.
func (tm *TimeoutManager) IdleTimeout() time.Duration {
	return tm.config.Idle
}

// TimeoutContext creates a context with the default timeout.
func (tm *TimeoutManager) TimeoutContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, tm.config.Default)
}

// Deadline wraps a function call with deadline enforcement.
type Deadline struct {
	timeout time.Duration
	timer   *time.Timer
}

// NewDeadline creates a new deadline.
func NewDeadline(timeout time.Duration) *Deadline {
	return &Deadline{
		timeout: timeout,
	}
}

// Start starts the deadline timer.
func (d *Deadline) Start() <-chan time.Time {
	d.timer = time.NewTimer(d.timeout)
	return d.timer.C
}

// Stop stops the deadline timer.
func (d *Deadline) Stop() {
	if d.timer != nil {
		d.timer.Stop()
	}
}

// Reset resets the deadline timer.
func (d *Deadline) Reset() {
	if d.timer != nil {
		d.timer.Reset(d.timeout)
	}
}
