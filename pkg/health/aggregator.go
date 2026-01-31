// Package health provides a health check aggregator for the Unheaded Kingdom infrastructure.
// It supports multiple check types (HTTP, TCP, gRPC, exec), concurrent execution,
// circuit breakers, and integrates with Busboy for event emission.
package health

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"sort"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// Common errors
var (
	ErrCheckNotFound      = errors.New("health check not found")
	ErrCheckAlreadyExists = errors.New("health check already exists")
	ErrInvalidCheckType   = errors.New("invalid check type")
	ErrCheckTimeout       = errors.New("health check timed out")
	ErrCircuitOpen        = errors.New("circuit breaker is open")
	ErrAggregatorStopped  = errors.New("aggregator has been stopped")
)

// HealthStatus represents the health state of a service
type HealthStatus string

const (
	StatusHealthy   HealthStatus = "healthy"
	StatusDegraded  HealthStatus = "degraded"
	StatusUnhealthy HealthStatus = "unhealthy"
	StatusUnknown   HealthStatus = "unknown"
)

// CheckType represents the type of health check
type CheckType string

const (
	CheckTypeHTTP   CheckType = "http"
	CheckTypeTCP    CheckType = "tcp"
	CheckTypeGRPC   CheckType = "grpc"
	CheckTypeExec   CheckType = "exec"
	CheckTypeBusboy CheckType = "busboy"
)

// HealthCheck defines a health check configuration
type HealthCheck struct {
	// Name is a unique identifier for the check
	Name string `json:"name"`

	// Type specifies the check type (http, tcp, grpc, exec, busboy)
	Type CheckType `json:"type"`

	// Target is the endpoint or command to check
	// For HTTP: URL (e.g., "http://localhost:8080/health")
	// For TCP: address (e.g., "localhost:5432")
	// For gRPC: address (e.g., "localhost:9090")
	// For exec: command (e.g., "/usr/bin/check-service.sh")
	// For busboy: address (e.g., "localhost:9090")
	Target string `json:"target"`

	// Timeout for the check (default: 5s)
	Timeout time.Duration `json:"timeout"`

	// Interval between checks (default: 30s)
	Interval time.Duration `json:"interval"`

	// SuccessThreshold is number of consecutive successes to be healthy (default: 1)
	SuccessThreshold int `json:"success_threshold"`

	// FailureThreshold is number of consecutive failures to be unhealthy (default: 3)
	FailureThreshold int `json:"failure_threshold"`

	// Tags for categorizing checks
	Tags []string `json:"tags,omitempty"`

	// Metadata for additional check-specific info
	Metadata map[string]string `json:"metadata,omitempty"`

	// HTTPConfig contains HTTP-specific configuration
	HTTPConfig *HTTPCheckConfig `json:"http_config,omitempty"`

	// GRPCConfig contains gRPC-specific configuration
	GRPCConfig *GRPCCheckConfig `json:"grpc_config,omitempty"`

	// ExecConfig contains exec-specific configuration
	ExecConfig *ExecCheckConfig `json:"exec_config,omitempty"`
}

// HTTPCheckConfig contains HTTP-specific check configuration
type HTTPCheckConfig struct {
	// Method is the HTTP method (default: GET)
	Method string `json:"method"`

	// Headers to include in the request
	Headers map[string]string `json:"headers,omitempty"`

	// ExpectedStatus is the expected HTTP status code (default: 200)
	ExpectedStatus int `json:"expected_status"`

	// ExpectedBody is an optional string to check in response body
	ExpectedBody string `json:"expected_body,omitempty"`

	// FollowRedirects determines if redirects should be followed (default: false)
	FollowRedirects bool `json:"follow_redirects"`
}

// GRPCCheckConfig contains gRPC-specific check configuration
type GRPCCheckConfig struct {
	// Service is the gRPC service name (optional)
	Service string `json:"service,omitempty"`

	// UseTLS determines if TLS should be used
	UseTLS bool `json:"use_tls"`
}

// ExecCheckConfig contains exec-specific check configuration
type ExecCheckConfig struct {
	// Args are command arguments
	Args []string `json:"args,omitempty"`

	// Env are environment variables
	Env []string `json:"env,omitempty"`

	// ExpectedExitCode is the expected exit code (default: 0)
	ExpectedExitCode int `json:"expected_exit_code"`
}

// HealthResult represents the result of a health check execution
type HealthResult struct {
	// Name of the check
	Name string `json:"name"`

	// Status is the current health status
	Status HealthStatus `json:"status"`

	// Message provides additional context
	Message string `json:"message,omitempty"`

	// Duration is how long the check took
	Duration time.Duration `json:"duration"`

	// Timestamp is when the check was performed
	Timestamp time.Time `json:"timestamp"`

	// ConsecutiveSuccesses counts consecutive successful checks
	ConsecutiveSuccesses int `json:"consecutive_successes"`

	// ConsecutiveFailures counts consecutive failed checks
	ConsecutiveFailures int `json:"consecutive_failures"`

	// Error contains error details if check failed
	Error string `json:"error,omitempty"`

	// Metadata contains check-specific result data
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// HealthHistory stores historical health results for a check
type HealthHistory struct {
	// Name of the check
	Name string `json:"name"`

	// Results are historical check results (most recent first)
	Results []HealthResult `json:"results"`

	// MaxSize is the maximum number of results to keep
	MaxSize int `json:"max_size"`

	mu sync.RWMutex
}

// Add adds a result to the history
func (h *HealthHistory) Add(result HealthResult) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.Results = append([]HealthResult{result}, h.Results...)
	if len(h.Results) > h.MaxSize {
		h.Results = h.Results[:h.MaxSize]
	}
}

// GetSince returns results since a given time
func (h *HealthHistory) GetSince(since time.Time) []HealthResult {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var results []HealthResult
	for _, r := range h.Results {
		if r.Timestamp.After(since) {
			results = append(results, r)
		}
	}
	return results
}

// GetDuration returns results within a duration
func (h *HealthHistory) GetDuration(duration time.Duration) []HealthResult {
	return h.GetSince(time.Now().Add(-duration))
}

// CircuitBreaker implements circuit breaker pattern for health checks
type CircuitBreaker struct {
	// Name of the circuit
	Name string

	// State: closed (normal), open (tripped), half-open (testing)
	State string

	// FailureThreshold is failures before opening
	FailureThreshold int

	// SuccessThreshold is successes before closing (from half-open)
	SuccessThreshold int

	// ResetTimeout is time to wait before trying again
	ResetTimeout time.Duration

	// Failures counts consecutive failures
	Failures int

	// Successes counts consecutive successes (in half-open)
	Successes int

	// LastFailure is when the last failure occurred
	LastFailure time.Time

	// OpenedAt is when the circuit was opened
	OpenedAt time.Time

	mu sync.RWMutex
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(name string, failureThreshold, successThreshold int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		Name:             name,
		State:            "closed",
		FailureThreshold: failureThreshold,
		SuccessThreshold: successThreshold,
		ResetTimeout:     resetTimeout,
	}
}

// Allow checks if a request should be allowed
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.State {
	case "closed":
		return true
	case "open":
		if time.Since(cb.OpenedAt) > cb.ResetTimeout {
			cb.State = "half-open"
			cb.Successes = 0
			return true
		}
		return false
	case "half-open":
		return true
	default:
		return true
	}
}

// RecordSuccess records a successful operation
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.State {
	case "half-open":
		cb.Successes++
		if cb.Successes >= cb.SuccessThreshold {
			cb.State = "closed"
			cb.Failures = 0
		}
	case "closed":
		cb.Failures = 0
	}
}

// RecordFailure records a failed operation
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.LastFailure = time.Now()

	switch cb.State {
	case "half-open":
		cb.State = "open"
		cb.OpenedAt = time.Now()
	case "closed":
		cb.Failures++
		if cb.Failures >= cb.FailureThreshold {
			cb.State = "open"
			cb.OpenedAt = time.Now()
		}
	}
}

// GetState returns the current circuit state
func (cb *CircuitBreaker) GetState() string {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.State
}

// SystemHealth represents the overall system health
type SystemHealth struct {
	// Status is the aggregate health status
	Status HealthStatus `json:"status"`

	// Checks contains individual check results
	Checks map[string]*HealthResult `json:"checks"`

	// HealthyCount is number of healthy checks
	HealthyCount int `json:"healthy_count"`

	// DegradedCount is number of degraded checks
	DegradedCount int `json:"degraded_count"`

	// UnhealthyCount is number of unhealthy checks
	UnhealthyCount int `json:"unhealthy_count"`

	// TotalCount is total number of checks
	TotalCount int `json:"total_count"`

	// Timestamp is when the aggregate was computed
	Timestamp time.Time `json:"timestamp"`

	// Message provides summary information
	Message string `json:"message,omitempty"`
}

// HealthEvent represents a health state change event
type HealthEvent struct {
	// Type is the event type (state_change, check_registered, check_unregistered)
	Type string `json:"type"`

	// CheckName is the affected check
	CheckName string `json:"check_name"`

	// OldStatus is the previous status
	OldStatus HealthStatus `json:"old_status,omitempty"`

	// NewStatus is the new status
	NewStatus HealthStatus `json:"new_status"`

	// Timestamp is when the event occurred
	Timestamp time.Time `json:"timestamp"`

	// Message provides additional context
	Message string `json:"message,omitempty"`
}

// BusboyPublisher interface for publishing events to Busboy
type BusboyPublisher interface {
	Publish(ctx context.Context, topic string, payload []byte) error
}

// AggregatorConfig contains configuration for the Aggregator
type AggregatorConfig struct {
	// DefaultTimeout is the default check timeout
	DefaultTimeout time.Duration

	// DefaultInterval is the default check interval
	DefaultInterval time.Duration

	// HistorySize is the max history entries per check
	HistorySize int

	// CircuitBreakerFailures is failures before circuit opens
	CircuitBreakerFailures int

	// CircuitBreakerSuccesses is successes before circuit closes
	CircuitBreakerSuccesses int

	// CircuitBreakerResetTimeout is time before circuit tries again
	CircuitBreakerResetTimeout time.Duration

	// BusboyTopic is the topic for health events
	BusboyTopic string

	// ServiceName is the name of this aggregator service
	ServiceName string
}

// DefaultConfig returns sensible default configuration
func DefaultConfig() *AggregatorConfig {
	return &AggregatorConfig{
		DefaultTimeout:             5 * time.Second,
		DefaultInterval:            30 * time.Second,
		HistorySize:               1000,
		CircuitBreakerFailures:    3,
		CircuitBreakerSuccesses:   2,
		CircuitBreakerResetTimeout: 30 * time.Second,
		BusboyTopic:               "health.events",
		ServiceName:               "health-aggregator",
	}
}

// Prometheus metrics
var (
	healthCheckDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "health_check_duration_seconds",
			Help:    "Duration of health checks in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"name", "type", "status"},
	)

	healthCheckStatus = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "health_check_status",
			Help: "Current health check status (1=healthy, 0.5=degraded, 0=unhealthy)",
		},
		[]string{"name", "type"},
	)

	healthCheckTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "health_check_total",
			Help: "Total number of health checks performed",
		},
		[]string{"name", "type", "status"},
	)

	systemHealthStatus = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "system_health_status",
			Help: "Overall system health status (1=healthy, 0.5=degraded, 0=unhealthy)",
		},
	)

	circuitBreakerState = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "health_circuit_breaker_state",
			Help: "Circuit breaker state (0=closed, 1=open, 0.5=half-open)",
		},
		[]string{"name"},
	)
)

// Aggregator aggregates health checks from multiple services
type Aggregator struct {
	config *AggregatorConfig

	// checks stores registered health checks
	checks map[string]*HealthCheck
	checksMu sync.RWMutex

	// results stores latest results
	results map[string]*HealthResult
	resultsMu sync.RWMutex

	// history stores historical results
	history map[string]*HealthHistory
	historyMu sync.RWMutex

	// circuitBreakers for each check
	circuitBreakers map[string]*CircuitBreaker
	cbMu sync.RWMutex

	// busboy for publishing events
	busboy BusboyPublisher

	// httpClient for HTTP checks
	httpClient *http.Client

	// running state
	running bool
	stopCh  chan struct{}
	wg      sync.WaitGroup
	runMu   sync.RWMutex

	// event listeners
	listeners []func(HealthEvent)
	listenersMu sync.RWMutex
}

// NewAggregator creates a new health check aggregator
func NewAggregator(config *AggregatorConfig, busboy BusboyPublisher) *Aggregator {
	if config == nil {
		config = DefaultConfig()
	}

	return &Aggregator{
		config:          config,
		checks:          make(map[string]*HealthCheck),
		results:         make(map[string]*HealthResult),
		history:         make(map[string]*HealthHistory),
		circuitBreakers: make(map[string]*CircuitBreaker),
		busboy:          busboy,
		httpClient: &http.Client{
			Timeout: config.DefaultTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse // Don't follow redirects by default
			},
		},
		stopCh: make(chan struct{}),
	}
}

// RegisterCheck adds a new health check
func (a *Aggregator) RegisterCheck(check HealthCheck) error {
	if check.Name == "" {
		return errors.New("check name is required")
	}

	// Apply defaults
	if check.Timeout == 0 {
		check.Timeout = a.config.DefaultTimeout
	}
	if check.Interval == 0 {
		check.Interval = a.config.DefaultInterval
	}
	if check.SuccessThreshold == 0 {
		check.SuccessThreshold = 1
	}
	if check.FailureThreshold == 0 {
		check.FailureThreshold = 3
	}

	// Apply type-specific defaults
	if check.Type == CheckTypeHTTP && check.HTTPConfig == nil {
		check.HTTPConfig = &HTTPCheckConfig{
			Method:         "GET",
			ExpectedStatus: 200,
		}
	}
	if check.Type == CheckTypeGRPC && check.GRPCConfig == nil {
		check.GRPCConfig = &GRPCCheckConfig{}
	}
	if check.Type == CheckTypeExec && check.ExecConfig == nil {
		check.ExecConfig = &ExecCheckConfig{
			ExpectedExitCode: 0,
		}
	}

	a.checksMu.Lock()
	if _, exists := a.checks[check.Name]; exists {
		a.checksMu.Unlock()
		return ErrCheckAlreadyExists
	}
	a.checks[check.Name] = &check
	a.checksMu.Unlock()

	// Initialize history
	a.historyMu.Lock()
	a.history[check.Name] = &HealthHistory{
		Name:    check.Name,
		MaxSize: a.config.HistorySize,
	}
	a.historyMu.Unlock()

	// Initialize circuit breaker
	a.cbMu.Lock()
	a.circuitBreakers[check.Name] = NewCircuitBreaker(
		check.Name,
		a.config.CircuitBreakerFailures,
		a.config.CircuitBreakerSuccesses,
		a.config.CircuitBreakerResetTimeout,
	)
	a.cbMu.Unlock()

	// Initialize result as unknown
	a.resultsMu.Lock()
	a.results[check.Name] = &HealthResult{
		Name:      check.Name,
		Status:    StatusUnknown,
		Timestamp: time.Now(),
	}
	a.resultsMu.Unlock()

	// Emit event
	a.emitEvent(HealthEvent{
		Type:      "check_registered",
		CheckName: check.Name,
		NewStatus: StatusUnknown,
		Timestamp: time.Now(),
		Message:   fmt.Sprintf("Registered %s check for %s", check.Type, check.Target),
	})

	return nil
}

// UnregisterCheck removes a health check
func (a *Aggregator) UnregisterCheck(name string) error {
	a.checksMu.Lock()
	check, exists := a.checks[name]
	if !exists {
		a.checksMu.Unlock()
		return ErrCheckNotFound
	}
	delete(a.checks, name)
	a.checksMu.Unlock()

	// Get last status before removing
	a.resultsMu.Lock()
	lastResult := a.results[name]
	delete(a.results, name)
	a.resultsMu.Unlock()

	// Remove history
	a.historyMu.Lock()
	delete(a.history, name)
	a.historyMu.Unlock()

	// Remove circuit breaker
	a.cbMu.Lock()
	delete(a.circuitBreakers, name)
	a.cbMu.Unlock()

	// Emit event
	var lastStatus HealthStatus = StatusUnknown
	if lastResult != nil {
		lastStatus = lastResult.Status
	}

	a.emitEvent(HealthEvent{
		Type:      "check_unregistered",
		CheckName: name,
		OldStatus: lastStatus,
		NewStatus: StatusUnknown,
		Timestamp: time.Now(),
		Message:   fmt.Sprintf("Unregistered %s check for %s", check.Type, check.Target),
	})

	return nil
}

// RunChecks executes all registered health checks concurrently
func (a *Aggregator) RunChecks(ctx context.Context) map[string]*HealthResult {
	a.checksMu.RLock()
	checks := make([]*HealthCheck, 0, len(a.checks))
	for _, check := range a.checks {
		checks = append(checks, check)
	}
	a.checksMu.RUnlock()

	results := make(map[string]*HealthResult)
	var resultsMu sync.Mutex
	var wg sync.WaitGroup

	for _, check := range checks {
		wg.Add(1)
		go func(c *HealthCheck) {
			defer wg.Done()
			result := a.runSingleCheck(ctx, c)
			resultsMu.Lock()
			results[c.Name] = result
			resultsMu.Unlock()
		}(check)
	}

	wg.Wait()
	return results
}

// runSingleCheck executes a single health check
func (a *Aggregator) runSingleCheck(ctx context.Context, check *HealthCheck) *HealthResult {
	// Check circuit breaker
	a.cbMu.RLock()
	cb := a.circuitBreakers[check.Name]
	a.cbMu.RUnlock()

	if cb != nil && !cb.Allow() {
		return &HealthResult{
			Name:      check.Name,
			Status:    StatusUnhealthy,
			Message:   "Circuit breaker is open",
			Timestamp: time.Now(),
			Error:     ErrCircuitOpen.Error(),
		}
	}

	// Create timeout context
	checkCtx, cancel := context.WithTimeout(ctx, check.Timeout)
	defer cancel()

	start := time.Now()
	var result *HealthResult

	switch check.Type {
	case CheckTypeHTTP:
		result = a.runHTTPCheck(checkCtx, check)
	case CheckTypeTCP:
		result = a.runTCPCheck(checkCtx, check)
	case CheckTypeGRPC:
		result = a.runGRPCCheck(checkCtx, check)
	case CheckTypeExec:
		result = a.runExecCheck(checkCtx, check)
	case CheckTypeBusboy:
		result = a.runBusboyCheck(checkCtx, check)
	default:
		result = &HealthResult{
			Name:      check.Name,
			Status:    StatusUnhealthy,
			Message:   fmt.Sprintf("Unknown check type: %s", check.Type),
			Timestamp: time.Now(),
			Error:     ErrInvalidCheckType.Error(),
		}
	}

	result.Duration = time.Since(start)
	result.Timestamp = time.Now()

	// Update circuit breaker
	if cb != nil {
		if result.Status == StatusHealthy {
			cb.RecordSuccess()
		} else {
			cb.RecordFailure()
		}
		a.updateCircuitBreakerMetrics(check.Name, cb)
	}

	// Update consecutive counts
	a.updateConsecutiveCounts(check, result)

	// Determine final status based on thresholds
	a.applyThresholds(check, result)

	// Record metrics
	a.recordMetrics(check, result)

	// Store result and check for state change
	a.storeResultAndCheckStateChange(check, result)

	return result
}

// runHTTPCheck performs an HTTP health check
func (a *Aggregator) runHTTPCheck(ctx context.Context, check *HealthCheck) *HealthResult {
	config := check.HTTPConfig
	if config == nil {
		config = &HTTPCheckConfig{Method: "GET", ExpectedStatus: 200}
	}

	method := config.Method
	if method == "" {
		method = "GET"
	}

	req, err := http.NewRequestWithContext(ctx, method, check.Target, nil)
	if err != nil {
		return &HealthResult{
			Name:    check.Name,
			Status:  StatusUnhealthy,
			Message: "Failed to create request",
			Error:   err.Error(),
		}
	}

	for key, value := range config.Headers {
		req.Header.Set(key, value)
	}

	// Create client that respects follow redirects config
	client := a.httpClient
	if config.FollowRedirects {
		client = &http.Client{
			Timeout: check.Timeout,
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return &HealthResult{
			Name:    check.Name,
			Status:  StatusUnhealthy,
			Message: "Request failed",
			Error:   err.Error(),
		}
	}
	defer resp.Body.Close()

	expectedStatus := config.ExpectedStatus
	if expectedStatus == 0 {
		expectedStatus = 200
	}

	if resp.StatusCode != expectedStatus {
		return &HealthResult{
			Name:    check.Name,
			Status:  StatusUnhealthy,
			Message: fmt.Sprintf("Unexpected status: %d (expected %d)", resp.StatusCode, expectedStatus),
			Metadata: map[string]interface{}{
				"status_code": resp.StatusCode,
			},
		}
	}

	// Check expected body if configured
	if config.ExpectedBody != "" {
		buf := new(bytes.Buffer)
		_, err := buf.ReadFrom(resp.Body)
		if err != nil {
			return &HealthResult{
				Name:    check.Name,
				Status:  StatusUnhealthy,
				Message: "Failed to read response body",
				Error:   err.Error(),
			}
		}
		body := buf.String()
		if !bytes.Contains([]byte(body), []byte(config.ExpectedBody)) {
			return &HealthResult{
				Name:    check.Name,
				Status:  StatusUnhealthy,
				Message: "Response body does not contain expected content",
			}
		}
	}

	return &HealthResult{
		Name:    check.Name,
		Status:  StatusHealthy,
		Message: fmt.Sprintf("HTTP %d OK", resp.StatusCode),
		Metadata: map[string]interface{}{
			"status_code": resp.StatusCode,
		},
	}
}

// runTCPCheck performs a TCP connectivity check
func (a *Aggregator) runTCPCheck(ctx context.Context, check *HealthCheck) *HealthResult {
	dialer := net.Dialer{
		Timeout: check.Timeout,
	}

	conn, err := dialer.DialContext(ctx, "tcp", check.Target)
	if err != nil {
		return &HealthResult{
			Name:    check.Name,
			Status:  StatusUnhealthy,
			Message: "TCP connection failed",
			Error:   err.Error(),
		}
	}
	defer conn.Close()

	return &HealthResult{
		Name:    check.Name,
		Status:  StatusHealthy,
		Message: "TCP connection successful",
		Metadata: map[string]interface{}{
			"remote_addr": conn.RemoteAddr().String(),
		},
	}
}

// runGRPCCheck performs a gRPC health check using the standard health protocol
func (a *Aggregator) runGRPCCheck(ctx context.Context, check *HealthCheck) *HealthResult {
	config := check.GRPCConfig
	if config == nil {
		config = &GRPCCheckConfig{}
	}

	opts := []grpc.DialOption{
		grpc.WithBlock(),
	}

	if !config.UseTLS {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.DialContext(ctx, check.Target, opts...)
	if err != nil {
		return &HealthResult{
			Name:    check.Name,
			Status:  StatusUnhealthy,
			Message: "gRPC connection failed",
			Error:   err.Error(),
		}
	}
	defer conn.Close()

	client := healthpb.NewHealthClient(conn)
	req := &healthpb.HealthCheckRequest{
		Service: config.Service,
	}

	resp, err := client.Check(ctx, req)
	if err != nil {
		return &HealthResult{
			Name:    check.Name,
			Status:  StatusUnhealthy,
			Message: "gRPC health check failed",
			Error:   err.Error(),
		}
	}

	switch resp.Status {
	case healthpb.HealthCheckResponse_SERVING:
		return &HealthResult{
			Name:    check.Name,
			Status:  StatusHealthy,
			Message: "gRPC service is serving",
			Metadata: map[string]interface{}{
				"grpc_status": "SERVING",
			},
		}
	case healthpb.HealthCheckResponse_NOT_SERVING:
		return &HealthResult{
			Name:    check.Name,
			Status:  StatusUnhealthy,
			Message: "gRPC service is not serving",
			Metadata: map[string]interface{}{
				"grpc_status": "NOT_SERVING",
			},
		}
	default:
		return &HealthResult{
			Name:    check.Name,
			Status:  StatusUnknown,
			Message: fmt.Sprintf("Unknown gRPC health status: %v", resp.Status),
			Metadata: map[string]interface{}{
				"grpc_status": resp.Status.String(),
			},
		}
	}
}

// runExecCheck performs an exec-based health check
func (a *Aggregator) runExecCheck(ctx context.Context, check *HealthCheck) *HealthResult {
	config := check.ExecConfig
	if config == nil {
		config = &ExecCheckConfig{ExpectedExitCode: 0}
	}

	cmd := exec.CommandContext(ctx, check.Target, config.Args...)
	if len(config.Env) > 0 {
		cmd.Env = config.Env
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return &HealthResult{
				Name:    check.Name,
				Status:  StatusUnhealthy,
				Message: "Command execution failed",
				Error:   err.Error(),
			}
		}
	}

	if exitCode != config.ExpectedExitCode {
		return &HealthResult{
			Name:    check.Name,
			Status:  StatusUnhealthy,
			Message: fmt.Sprintf("Unexpected exit code: %d (expected %d)", exitCode, config.ExpectedExitCode),
			Metadata: map[string]interface{}{
				"exit_code": exitCode,
				"stdout":    stdout.String(),
				"stderr":    stderr.String(),
			},
		}
	}

	return &HealthResult{
		Name:    check.Name,
		Status:  StatusHealthy,
		Message: fmt.Sprintf("Command exited with code %d", exitCode),
		Metadata: map[string]interface{}{
			"exit_code": exitCode,
			"stdout":    stdout.String(),
		},
	}
}

// runBusboyCheck performs a Busboy connectivity check
func (a *Aggregator) runBusboyCheck(ctx context.Context, check *HealthCheck) *HealthResult {
	// First, check TCP connectivity
	dialer := net.Dialer{
		Timeout: check.Timeout,
	}

	conn, err := dialer.DialContext(ctx, "tcp", check.Target)
	if err != nil {
		return &HealthResult{
			Name:    check.Name,
			Status:  StatusUnhealthy,
			Message: "Busboy TCP connection failed",
			Error:   err.Error(),
		}
	}
	conn.Close()

	// Then try HTTP health endpoint
	healthURL := fmt.Sprintf("http://%s/health", check.Target)
	req, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
	if err != nil {
		// If we can't create request, at least TCP is working
		return &HealthResult{
			Name:    check.Name,
			Status:  StatusDegraded,
			Message: "Busboy TCP OK, but health check unavailable",
		}
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return &HealthResult{
			Name:    check.Name,
			Status:  StatusDegraded,
			Message: "Busboy TCP OK, HTTP health check failed",
			Error:   err.Error(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &HealthResult{
			Name:    check.Name,
			Status:  StatusDegraded,
			Message: fmt.Sprintf("Busboy health returned status %d", resp.StatusCode),
			Metadata: map[string]interface{}{
				"status_code": resp.StatusCode,
			},
		}
	}

	return &HealthResult{
		Name:    check.Name,
		Status:  StatusHealthy,
		Message: "Busboy is healthy",
	}
}

// updateConsecutiveCounts updates consecutive success/failure counts
func (a *Aggregator) updateConsecutiveCounts(check *HealthCheck, result *HealthResult) {
	a.resultsMu.Lock()
	defer a.resultsMu.Unlock()

	prev, exists := a.results[check.Name]
	if !exists {
		if result.Status == StatusHealthy {
			result.ConsecutiveSuccesses = 1
		} else {
			result.ConsecutiveFailures = 1
		}
		return
	}

	if result.Status == StatusHealthy {
		result.ConsecutiveSuccesses = prev.ConsecutiveSuccesses + 1
		result.ConsecutiveFailures = 0
	} else {
		result.ConsecutiveFailures = prev.ConsecutiveFailures + 1
		result.ConsecutiveSuccesses = 0
	}
}

// applyThresholds applies success/failure thresholds to determine final status
func (a *Aggregator) applyThresholds(check *HealthCheck, result *HealthResult) {
	// If check is healthy but hasn't reached success threshold yet, mark as degraded
	if result.Status == StatusHealthy && result.ConsecutiveSuccesses < check.SuccessThreshold {
		result.Status = StatusDegraded
		result.Message = fmt.Sprintf("Recovering (%d/%d successes)", result.ConsecutiveSuccesses, check.SuccessThreshold)
	}

	// If check has failures but hasn't reached failure threshold, mark as degraded
	if result.ConsecutiveFailures > 0 && result.ConsecutiveFailures < check.FailureThreshold {
		result.Status = StatusDegraded
		result.Message = fmt.Sprintf("Degraded (%d/%d failures)", result.ConsecutiveFailures, check.FailureThreshold)
	}
}

// storeResultAndCheckStateChange stores the result and checks for state changes
func (a *Aggregator) storeResultAndCheckStateChange(check *HealthCheck, result *HealthResult) {
	// Get previous result
	a.resultsMu.RLock()
	prev, exists := a.results[check.Name]
	var prevStatus HealthStatus = StatusUnknown
	if exists && prev != nil {
		prevStatus = prev.Status
	}
	a.resultsMu.RUnlock()

	// Store new result
	a.resultsMu.Lock()
	a.results[check.Name] = result
	a.resultsMu.Unlock()

	// Add to history
	a.historyMu.Lock()
	if hist, ok := a.history[check.Name]; ok {
		hist.Add(*result)
	}
	a.historyMu.Unlock()

	// Check for state change
	if result.Status != prevStatus {
		a.emitEvent(HealthEvent{
			Type:      "state_change",
			CheckName: check.Name,
			OldStatus: prevStatus,
			NewStatus: result.Status,
			Timestamp: time.Now(),
			Message:   result.Message,
		})
	}
}

// recordMetrics records Prometheus metrics for a check result
func (a *Aggregator) recordMetrics(check *HealthCheck, result *HealthResult) {
	// Record duration
	healthCheckDuration.WithLabelValues(
		check.Name,
		string(check.Type),
		string(result.Status),
	).Observe(result.Duration.Seconds())

	// Record status gauge
	var statusValue float64
	switch result.Status {
	case StatusHealthy:
		statusValue = 1.0
	case StatusDegraded:
		statusValue = 0.5
	default:
		statusValue = 0.0
	}
	healthCheckStatus.WithLabelValues(check.Name, string(check.Type)).Set(statusValue)

	// Increment total counter
	healthCheckTotal.WithLabelValues(
		check.Name,
		string(check.Type),
		string(result.Status),
	).Inc()
}

// updateCircuitBreakerMetrics updates circuit breaker metrics
func (a *Aggregator) updateCircuitBreakerMetrics(name string, cb *CircuitBreaker) {
	var stateValue float64
	switch cb.GetState() {
	case "closed":
		stateValue = 0.0
	case "half-open":
		stateValue = 0.5
	case "open":
		stateValue = 1.0
	}
	circuitBreakerState.WithLabelValues(name).Set(stateValue)
}

// emitEvent emits a health event to listeners and Busboy
func (a *Aggregator) emitEvent(event HealthEvent) {
	// Notify local listeners
	a.listenersMu.RLock()
	listeners := make([]func(HealthEvent), len(a.listeners))
	copy(listeners, a.listeners)
	a.listenersMu.RUnlock()

	for _, listener := range listeners {
		go listener(event)
	}

	// Publish to Busboy if configured
	if a.busboy != nil {
		payload, err := json.Marshal(event)
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = a.busboy.Publish(ctx, a.config.BusboyTopic, payload)
			cancel()
		}
	}
}

// AddListener adds an event listener
func (a *Aggregator) AddListener(listener func(HealthEvent)) {
	a.listenersMu.Lock()
	defer a.listenersMu.Unlock()
	a.listeners = append(a.listeners, listener)
}

// GetHealth returns the current health of a specific check
func (a *Aggregator) GetHealth(name string) (*HealthResult, error) {
	a.resultsMu.RLock()
	defer a.resultsMu.RUnlock()

	result, exists := a.results[name]
	if !exists {
		return nil, ErrCheckNotFound
	}

	return result, nil
}

// GetSystemHealth returns the aggregate health of all checks
func (a *Aggregator) GetSystemHealth() *SystemHealth {
	a.resultsMu.RLock()
	defer a.resultsMu.RUnlock()

	health := &SystemHealth{
		Checks:    make(map[string]*HealthResult),
		Timestamp: time.Now(),
	}

	for name, result := range a.results {
		health.Checks[name] = result
		health.TotalCount++

		switch result.Status {
		case StatusHealthy:
			health.HealthyCount++
		case StatusDegraded:
			health.DegradedCount++
		case StatusUnhealthy, StatusUnknown:
			health.UnhealthyCount++
		}
	}

	// Determine overall status
	if health.UnhealthyCount > 0 {
		health.Status = StatusUnhealthy
		health.Message = fmt.Sprintf("%d/%d checks unhealthy", health.UnhealthyCount, health.TotalCount)
	} else if health.DegradedCount > 0 {
		health.Status = StatusDegraded
		health.Message = fmt.Sprintf("%d/%d checks degraded", health.DegradedCount, health.TotalCount)
	} else if health.HealthyCount == health.TotalCount && health.TotalCount > 0 {
		health.Status = StatusHealthy
		health.Message = fmt.Sprintf("All %d checks healthy", health.TotalCount)
	} else {
		health.Status = StatusUnknown
		health.Message = "No checks registered"
	}

	// Update system health metric
	var statusValue float64
	switch health.Status {
	case StatusHealthy:
		statusValue = 1.0
	case StatusDegraded:
		statusValue = 0.5
	default:
		statusValue = 0.0
	}
	systemHealthStatus.Set(statusValue)

	return health
}

// GetHealthHistory returns historical results for a check
func (a *Aggregator) GetHealthHistory(name string, duration time.Duration) ([]HealthResult, error) {
	a.historyMu.RLock()
	defer a.historyMu.RUnlock()

	hist, exists := a.history[name]
	if !exists {
		return nil, ErrCheckNotFound
	}

	return hist.GetDuration(duration), nil
}

// StartMonitoring starts continuous health monitoring
func (a *Aggregator) StartMonitoring(ctx context.Context, interval time.Duration) error {
	a.runMu.Lock()
	if a.running {
		a.runMu.Unlock()
		return errors.New("monitoring already running")
	}
	a.running = true
	a.stopCh = make(chan struct{})
	a.runMu.Unlock()

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Run immediately on start
		a.RunChecks(ctx)

		for {
			select {
			case <-ctx.Done():
				return
			case <-a.stopCh:
				return
			case <-ticker.C:
				a.RunChecks(ctx)
			}
		}
	}()

	return nil
}

// StopMonitoring stops continuous health monitoring
func (a *Aggregator) StopMonitoring() {
	a.runMu.Lock()
	if !a.running {
		a.runMu.Unlock()
		return
	}
	a.running = false
	close(a.stopCh)
	a.runMu.Unlock()

	a.wg.Wait()
}

// IsRunning returns whether monitoring is active
func (a *Aggregator) IsRunning() bool {
	a.runMu.RLock()
	defer a.runMu.RUnlock()
	return a.running
}

// GetChecks returns all registered checks
func (a *Aggregator) GetChecks() []*HealthCheck {
	a.checksMu.RLock()
	defer a.checksMu.RUnlock()

	checks := make([]*HealthCheck, 0, len(a.checks))
	for _, check := range a.checks {
		checks = append(checks, check)
	}

	// Sort by name for consistent ordering
	sort.Slice(checks, func(i, j int) bool {
		return checks[i].Name < checks[j].Name
	})

	return checks
}

// GetCircuitBreakerState returns the circuit breaker state for a check
func (a *Aggregator) GetCircuitBreakerState(name string) (string, error) {
	a.cbMu.RLock()
	defer a.cbMu.RUnlock()

	cb, exists := a.circuitBreakers[name]
	if !exists {
		return "", ErrCheckNotFound
	}

	return cb.GetState(), nil
}

// HTTPHandler returns an HTTP handler for health queries
func (a *Aggregator) HTTPHandler() http.Handler {
	mux := http.NewServeMux()

	// GET /health - overall system health
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		health := a.GetSystemHealth()
		w.Header().Set("Content-Type", "application/json")

		switch health.Status {
		case StatusHealthy:
			w.WriteHeader(http.StatusOK)
		case StatusDegraded:
			w.WriteHeader(http.StatusOK) // Still operational
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		json.NewEncoder(w).Encode(health)
	})

	// GET /health/{name} - specific check health
	mux.HandleFunc("/health/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		name := r.URL.Path[len("/health/"):]
		if name == "" {
			http.Error(w, "Check name required", http.StatusBadRequest)
			return
		}

		result, err := a.GetHealth(name)
		if err != nil {
			if errors.Is(err, ErrCheckNotFound) {
				http.Error(w, "Check not found", http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	// GET /checks - list all checks
	mux.HandleFunc("/checks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		checks := a.GetChecks()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"checks": checks,
			"count":  len(checks),
		})
	})

	// GET /history/{name}?duration=1h - historical results
	mux.HandleFunc("/history/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		name := r.URL.Path[len("/history/"):]
		if name == "" {
			http.Error(w, "Check name required", http.StatusBadRequest)
			return
		}

		durationStr := r.URL.Query().Get("duration")
		if durationStr == "" {
			durationStr = "1h"
		}

		duration, err := time.ParseDuration(durationStr)
		if err != nil {
			http.Error(w, "Invalid duration", http.StatusBadRequest)
			return
		}

		history, err := a.GetHealthHistory(name, duration)
		if err != nil {
			if errors.Is(err, ErrCheckNotFound) {
				http.Error(w, "Check not found", http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"name":     name,
			"duration": duration.String(),
			"results":  history,
			"count":    len(history),
		})
	})

	// POST /checks - register a new check
	mux.HandleFunc("/checks/register", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var check HealthCheck
		if err := json.NewDecoder(r.Body).Decode(&check); err != nil {
			http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
			return
		}

		if err := a.RegisterCheck(check); err != nil {
			if errors.Is(err, ErrCheckAlreadyExists) {
				http.Error(w, "Check already exists", http.StatusConflict)
			} else {
				http.Error(w, err.Error(), http.StatusBadRequest)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "registered",
			"name":    check.Name,
			"message": fmt.Sprintf("Check %s registered successfully", check.Name),
		})
	})

	// DELETE /checks/{name} - unregister a check
	mux.HandleFunc("/checks/", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Path[len("/checks/"):]
		if name == "" || name == "register" {
			http.Error(w, "Check name required", http.StatusBadRequest)
			return
		}

		if r.Method != http.MethodDelete {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if err := a.UnregisterCheck(name); err != nil {
			if errors.Is(err, ErrCheckNotFound) {
				http.Error(w, "Check not found", http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "unregistered",
			"name":    name,
			"message": fmt.Sprintf("Check %s unregistered successfully", name),
		})
	})

	// POST /run - trigger all checks immediately
	mux.HandleFunc("/run", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		results := a.RunChecks(r.Context())
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"results": results,
			"count":   len(results),
		})
	})

	// GET /ready - readiness probe
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		health := a.GetSystemHealth()
		if health.Status == StatusUnhealthy {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{
				"status": "not ready",
				"reason": health.Message,
			})
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "ready",
		})
	})

	// GET /live - liveness probe
	mux.HandleFunc("/live", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "alive",
		})
	})

	return mux
}

// Close gracefully shuts down the aggregator
func (a *Aggregator) Close() error {
	a.StopMonitoring()
	return nil
}
