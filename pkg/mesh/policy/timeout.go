package policy

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

var (
	// ErrTimeout indicates an operation timed out.
	ErrTimeout = errors.New("operation timed out")
	// ErrDeadlineExceeded indicates a deadline was exceeded.
	ErrDeadlineExceeded = errors.New("deadline exceeded")
)

// TimeoutConfig holds timeout settings for different operations.
type TimeoutConfig struct {
	Request    time.Duration // total request timeout
	Connect    time.Duration // connection establishment timeout
	Idle       time.Duration // idle connection timeout
	Read       time.Duration // read operation timeout
	Write      time.Duration // write operation timeout
	TLSShake   time.Duration // TLS handshake timeout
	KeepAlive  time.Duration // keepalive interval
}

// DefaultTimeoutConfig returns sensible timeout defaults.
func DefaultTimeoutConfig() *TimeoutConfig {
	return &TimeoutConfig{
		Request:   30 * time.Second,
		Connect:   5 * time.Second,
		Idle:      90 * time.Second,
		Read:      30 * time.Second,
		Write:     30 * time.Second,
		TLSShake:  10 * time.Second,
		KeepAlive: 30 * time.Second,
	}
}

// TimeoutController manages request timeouts.
type TimeoutController struct {
	config *TimeoutConfig
}

// NewTimeoutController creates a new timeout controller.
func NewTimeoutController(config *TimeoutConfig) *TimeoutController {
	if config == nil {
		config = DefaultTimeoutConfig()
	}
	return &TimeoutController{config: config}
}

// WithRequestTimeout wraps a context with request timeout.
func (tc *TimeoutController) WithRequestTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, tc.config.Request)
}

// WithConnectTimeout wraps a context with connect timeout.
func (tc *TimeoutController) WithConnectTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, tc.config.Connect)
}

// Execute runs an operation with the configured timeout.
func (tc *TimeoutController) Execute(ctx context.Context, op func(ctx context.Context) error) error {
	ctx, cancel := tc.WithRequestTimeout(ctx)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- op(ctx)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ErrTimeout
	}
}

// TimeoutConn wraps a net.Conn with timeout support.
type TimeoutConn struct {
	net.Conn
	readTimeout  time.Duration
	writeTimeout time.Duration
}

// NewTimeoutConn wraps a connection with timeout support.
func NewTimeoutConn(conn net.Conn, readTimeout, writeTimeout time.Duration) *TimeoutConn {
	return &TimeoutConn{
		Conn:         conn,
		readTimeout:  readTimeout,
		writeTimeout: writeTimeout,
	}
}

// Read reads data with timeout.
func (tc *TimeoutConn) Read(b []byte) (int, error) {
	if tc.readTimeout > 0 {
		if err := tc.Conn.SetReadDeadline(time.Now().Add(tc.readTimeout)); err != nil {
			return 0, err
		}
	}
	return tc.Conn.Read(b)
}

// Write writes data with timeout.
func (tc *TimeoutConn) Write(b []byte) (int, error) {
	if tc.writeTimeout > 0 {
		if err := tc.Conn.SetWriteDeadline(time.Now().Add(tc.writeTimeout)); err != nil {
			return 0, err
		}
	}
	return tc.Conn.Write(b)
}

// SetTimeouts updates the read and write timeouts.
func (tc *TimeoutConn) SetTimeouts(read, write time.Duration) {
	tc.readTimeout = read
	tc.writeTimeout = write
}

// DeadlineManager tracks and enforces request deadlines.
type DeadlineManager struct {
	mu        sync.RWMutex
	deadlines map[string]time.Time
	cleanup   *time.Ticker
	done      chan struct{}
}

// NewDeadlineManager creates a new deadline manager.
func NewDeadlineManager() *DeadlineManager {
	dm := &DeadlineManager{
		deadlines: make(map[string]time.Time),
		cleanup:   time.NewTicker(time.Minute),
		done:      make(chan struct{}),
	}
	go dm.cleanupLoop()
	return dm
}

// Set sets a deadline for a request.
func (dm *DeadlineManager) Set(requestID string, deadline time.Time) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.deadlines[requestID] = deadline
}

// Get retrieves the deadline for a request.
func (dm *DeadlineManager) Get(requestID string) (time.Time, bool) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	d, ok := dm.deadlines[requestID]
	return d, ok
}

// IsExpired checks if a request's deadline has passed.
func (dm *DeadlineManager) IsExpired(requestID string) bool {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	if d, ok := dm.deadlines[requestID]; ok {
		return time.Now().After(d)
	}
	return false
}

// Remove removes a deadline.
func (dm *DeadlineManager) Remove(requestID string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	delete(dm.deadlines, requestID)
}

// RemainingTime returns the remaining time until deadline.
func (dm *DeadlineManager) RemainingTime(requestID string) time.Duration {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	if d, ok := dm.deadlines[requestID]; ok {
		remaining := time.Until(d)
		if remaining < 0 {
			return 0
		}
		return remaining
	}
	return 0
}

// Stop stops the deadline manager.
func (dm *DeadlineManager) Stop() {
	close(dm.done)
	dm.cleanup.Stop()
}

func (dm *DeadlineManager) cleanupLoop() {
	for {
		select {
		case <-dm.done:
			return
		case <-dm.cleanup.C:
			dm.mu.Lock()
			now := time.Now()
			for id, deadline := range dm.deadlines {
				if now.After(deadline) {
					delete(dm.deadlines, id)
				}
			}
			dm.mu.Unlock()
		}
	}
}

// IdleTracker tracks idle connections for timeout.
type IdleTracker struct {
	mu         sync.RWMutex
	lastActive map[string]time.Time
	timeout    time.Duration
}

// NewIdleTracker creates a new idle tracker.
func NewIdleTracker(timeout time.Duration) *IdleTracker {
	return &IdleTracker{
		lastActive: make(map[string]time.Time),
		timeout:    timeout,
	}
}

// Touch updates the last active time for a connection.
func (it *IdleTracker) Touch(connID string) {
	it.mu.Lock()
	defer it.mu.Unlock()
	it.lastActive[connID] = time.Now()
}

// IsIdle checks if a connection is idle.
func (it *IdleTracker) IsIdle(connID string) bool {
	it.mu.RLock()
	defer it.mu.RUnlock()
	if last, ok := it.lastActive[connID]; ok {
		return time.Since(last) > it.timeout
	}
	return true
}

// Remove removes a connection from tracking.
func (it *IdleTracker) Remove(connID string) {
	it.mu.Lock()
	defer it.mu.Unlock()
	delete(it.lastActive, connID)
}

// GetIdleConnections returns all idle connection IDs.
func (it *IdleTracker) GetIdleConnections() []string {
	it.mu.RLock()
	defer it.mu.RUnlock()

	var idle []string
	now := time.Now()
	for id, last := range it.lastActive {
		if now.Sub(last) > it.timeout {
			idle = append(idle, id)
		}
	}
	return idle
}

// TimeoutReader wraps an io.Reader with timeout.
type TimeoutReader struct {
	r       io.Reader
	timeout time.Duration
	cancel  context.CancelFunc
}

// NewTimeoutReader creates a new timeout reader.
func NewTimeoutReader(r io.Reader, timeout time.Duration) *TimeoutReader {
	return &TimeoutReader{
		r:       r,
		timeout: timeout,
	}
}

// Read reads with timeout.
func (tr *TimeoutReader) Read(p []byte) (int, error) {
	if tr.timeout <= 0 {
		return tr.r.Read(p)
	}

	type readResult struct {
		n   int
		err error
	}

	result := make(chan readResult, 1)
	go func() {
		n, err := tr.r.Read(p)
		result <- readResult{n, err}
	}()

	select {
	case r := <-result:
		return r.n, r.err
	case <-time.After(tr.timeout):
		return 0, ErrTimeout
	}
}
