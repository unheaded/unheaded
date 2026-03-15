// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package pipeline provides the deployment pipeline for orchestrating deployment stages.
package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Health gate errors.
var (
	ErrHealthGateTimeout    = errors.New("health gate timeout")
	ErrHealthCheckFailed    = errors.New("health check failed")
	ErrAllChecksFailed      = errors.New("all health checks failed")
	ErrInsufficientHealthy  = errors.New("insufficient healthy instances")
	ErrHealthGateAborted    = errors.New("health gate aborted")
)

// HealthCheckType represents the type of health check.
type HealthCheckType string

const (
	HealthCheckTypeHTTP   HealthCheckType = "http"
	HealthCheckTypeTCP    HealthCheckType = "tcp"
	HealthCheckTypeGRPC   HealthCheckType = "grpc"
	HealthCheckTypeExec   HealthCheckType = "exec"
	HealthCheckTypeCustom HealthCheckType = "custom"
)

// HealthGate manages health gates for deployments.
type HealthGate struct {
	mu sync.RWMutex

	// httpClient for HTTP health checks
	httpClient *http.Client

	// activeGates tracks active health gates
	activeGates map[string]*GateExecution

	// defaultConfig is the default health gate configuration
	defaultConfig *HealthGateDefinition

	// eventCallback for health gate events
	eventCallback func(event *HealthGateEvent)
}

// HealthGateDefinition defines a health gate configuration.
type HealthGateDefinition struct {
	// Name is the gate name
	Name string `json:"name"`

	// Enabled indicates if the gate is enabled
	Enabled bool `json:"enabled"`

	// InitialDelay before starting health checks
	InitialDelay time.Duration `json:"initial_delay,omitempty"`

	// Timeout is the overall gate timeout
	Timeout time.Duration `json:"timeout,omitempty"`

	// Interval between health checks
	Interval time.Duration `json:"interval,omitempty"`

	// SuccessThreshold consecutive successes to pass
	SuccessThreshold int `json:"success_threshold,omitempty"`

	// FailureThreshold consecutive failures to fail
	FailureThreshold int `json:"failure_threshold,omitempty"`

	// HealthyPercentage minimum percentage of instances that must be healthy
	HealthyPercentage int `json:"healthy_percentage,omitempty"`

	// Checks are the health checks to perform
	Checks []*HealthCheck `json:"checks,omitempty"`

	// Conditions are additional conditions to evaluate
	Conditions []*HealthCondition `json:"conditions,omitempty"`

	// ProgressiveRollout waits for progressive health during canary
	ProgressiveRollout bool `json:"progressive_rollout,omitempty"`

	// AbortOnFailure aborts deployment on gate failure
	AbortOnFailure bool `json:"abort_on_failure,omitempty"`

	// NotifyOnFailure sends notification on gate failure
	NotifyOnFailure bool `json:"notify_on_failure,omitempty"`
}

// HealthCheck defines a single health check.
type HealthCheck struct {
	// Name is the check name
	Name string `json:"name"`

	// Type is the check type
	Type HealthCheckType `json:"type"`

	// Enabled indicates if the check is enabled
	Enabled bool `json:"enabled"`

	// Weight is the check weight in overall health (0-1)
	Weight float64 `json:"weight,omitempty"`

	// HTTP check configuration
	HTTP *HTTPHealthCheck `json:"http,omitempty"`

	// TCP check configuration
	TCP *TCPHealthCheck `json:"tcp,omitempty"`

	// GRPC check configuration
	GRPC *GRPCHealthCheck `json:"grpc,omitempty"`

	// Exec check configuration
	Exec *ExecHealthCheck `json:"exec,omitempty"`

	// Timeout for this check
	Timeout time.Duration `json:"timeout,omitempty"`

	// SuccessThreshold for this check
	SuccessThreshold int `json:"success_threshold,omitempty"`

	// FailureThreshold for this check
	FailureThreshold int `json:"failure_threshold,omitempty"`
}

// HTTPHealthCheck defines HTTP health check configuration.
type HTTPHealthCheck struct {
	// URL is the health check URL
	URL string `json:"url"`

	// Method is the HTTP method (GET, POST, etc)
	Method string `json:"method,omitempty"`

	// Headers to send with the request
	Headers map[string]string `json:"headers,omitempty"`

	// Body to send with the request
	Body string `json:"body,omitempty"`

	// ExpectedStatus is the expected HTTP status code
	ExpectedStatus int `json:"expected_status,omitempty"`

	// ExpectedBody is expected response body (substring match)
	ExpectedBody string `json:"expected_body,omitempty"`

	// ExpectedJSON is expected JSON path values
	ExpectedJSON map[string]interface{} `json:"expected_json,omitempty"`

	// FollowRedirects allows following redirects
	FollowRedirects bool `json:"follow_redirects,omitempty"`

	// InsecureSkipVerify skips TLS verification
	InsecureSkipVerify bool `json:"insecure_skip_verify,omitempty"`
}

// TCPHealthCheck defines TCP health check configuration.
type TCPHealthCheck struct {
	// Address is the TCP address (host:port)
	Address string `json:"address"`

	// Send is optional data to send
	Send string `json:"send,omitempty"`

	// Expect is expected response (substring match)
	Expect string `json:"expect,omitempty"`
}

// GRPCHealthCheck defines gRPC health check configuration.
type GRPCHealthCheck struct {
	// Address is the gRPC address
	Address string `json:"address"`

	// Service is the service name for health check
	Service string `json:"service,omitempty"`

	// Insecure uses insecure connection
	Insecure bool `json:"insecure,omitempty"`
}

// ExecHealthCheck defines command execution health check configuration.
type ExecHealthCheck struct {
	// Command is the command to execute
	Command string `json:"command"`

	// Args are command arguments
	Args []string `json:"args,omitempty"`

	// ExpectedExitCode is the expected exit code (default 0)
	ExpectedExitCode int `json:"expected_exit_code,omitempty"`

	// ExpectedOutput is expected stdout (substring match)
	ExpectedOutput string `json:"expected_output,omitempty"`
}

// HealthCondition defines an additional health condition.
type HealthCondition struct {
	// Name is the condition name
	Name string `json:"name"`

	// Type is the condition type (metric, ready_pods, etc)
	Type string `json:"type"`

	// Query is the condition query
	Query string `json:"query,omitempty"`

	// Operator for comparison (>, <, >=, <=, ==, !=)
	Operator string `json:"operator"`

	// Value for comparison
	Value float64 `json:"value"`

	// Weight in overall health
	Weight float64 `json:"weight,omitempty"`
}

// GateExecution represents an active health gate execution.
type GateExecution struct {
	// ID is the execution ID
	ID string `json:"id"`

	// DeploymentID is the deployment this gate is for
	DeploymentID string `json:"deployment_id"`

	// ServiceName is the service name
	ServiceName string `json:"service_name"`

	// Definition is the gate definition
	Definition *HealthGateDefinition `json:"definition"`

	// State is the current gate state
	State GateState `json:"state"`

	// Progress contains execution progress
	Progress *GateProgress `json:"progress"`

	// Results contains check results
	Results []*CheckResult `json:"results"`

	// StartedAt is when the gate started
	StartedAt time.Time `json:"started_at"`

	// CompletedAt is when the gate completed
	CompletedAt time.Time `json:"completed_at,omitempty"`

	// Error contains error if failed
	Error string `json:"error,omitempty"`

	// cancel function
	cancel context.CancelFunc

	// done channel
	done chan struct{}
}

// GateState represents the state of a health gate.
type GateState string

const (
	GateStatePending   GateState = "pending"
	GateStateWaiting   GateState = "waiting"
	GateStateChecking  GateState = "checking"
	GateStatePassed    GateState = "passed"
	GateStateFailed    GateState = "failed"
	GateStateAborted   GateState = "aborted"
)

// GateProgress tracks gate execution progress.
type GateProgress struct {
	// Phase is the current phase
	Phase string `json:"phase"`

	// Message is a human-readable message
	Message string `json:"message"`

	// ChecksTotal is total checks
	ChecksTotal int `json:"checks_total"`

	// ChecksPassed is passed checks
	ChecksPassed int `json:"checks_passed"`

	// ChecksFailed is failed checks
	ChecksFailed int `json:"checks_failed"`

	// ConsecutiveSuccess count
	ConsecutiveSuccess int `json:"consecutive_success"`

	// ConsecutiveFailure count
	ConsecutiveFailure int `json:"consecutive_failure"`

	// HealthScore is overall health score (0-100)
	HealthScore float64 `json:"health_score"`

	// TimeRemaining until timeout
	TimeRemaining time.Duration `json:"time_remaining,omitempty"`
}

// CheckResult contains the result of a health check.
type CheckResult struct {
	// Check is the check that was executed
	Check *HealthCheck `json:"check"`

	// Passed indicates if the check passed
	Passed bool `json:"passed"`

	// StatusCode for HTTP checks
	StatusCode int `json:"status_code,omitempty"`

	// Latency is the check latency
	Latency time.Duration `json:"latency"`

	// Output contains check output
	Output string `json:"output,omitempty"`

	// Error contains error message if failed
	Error string `json:"error,omitempty"`

	// Timestamp of the check
	Timestamp time.Time `json:"timestamp"`
}

// HealthGateEvent represents a health gate event.
type HealthGateEvent struct {
	// Type is the event type
	Type string `json:"type"`

	// GateID is the gate ID
	GateID string `json:"gate_id"`

	// DeploymentID is the deployment ID
	DeploymentID string `json:"deployment_id"`

	// State is the gate state
	State GateState `json:"state"`

	// Progress is the gate progress
	Progress *GateProgress `json:"progress,omitempty"`

	// Timestamp of the event
	Timestamp time.Time `json:"timestamp"`

	// Data contains additional event data
	Data map[string]interface{} `json:"data,omitempty"`
}

// NewHealthGate creates a new health gate manager.
func NewHealthGate() *HealthGate {
	return &HealthGate{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse // Don't follow redirects by default
			},
		},
		activeGates:   make(map[string]*GateExecution),
		defaultConfig: DefaultHealthGateDefinition(),
	}
}

// DefaultHealthGateDefinition returns a default health gate definition.
func DefaultHealthGateDefinition() *HealthGateDefinition {
	return &HealthGateDefinition{
		Name:              "default",
		Enabled:           true,
		InitialDelay:      10 * time.Second,
		Timeout:           5 * time.Minute,
		Interval:          5 * time.Second,
		SuccessThreshold:  3,
		FailureThreshold:  3,
		HealthyPercentage: 100,
		AbortOnFailure:    true,
		NotifyOnFailure:   true,
		Checks: []*HealthCheck{
			{
				Name:    "readiness",
				Type:    HealthCheckTypeHTTP,
				Enabled: true,
				Weight:  1.0,
				HTTP: &HTTPHealthCheck{
					URL:            "/ready",
					Method:         "GET",
					ExpectedStatus: 200,
				},
				Timeout: 5 * time.Second,
			},
		},
	}
}

// SetEventCallback sets the event callback.
func (g *HealthGate) SetEventCallback(callback func(event *HealthGateEvent)) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.eventCallback = callback
}

// Execute executes a health gate.
func (g *HealthGate) Execute(ctx context.Context, deploymentID, serviceName string, definition *HealthGateDefinition) error {
	if definition == nil {
		definition = g.defaultConfig
	}

	if !definition.Enabled {
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	if definition.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, definition.Timeout)
	}

	execution := &GateExecution{
		ID:           generateGateID(),
		DeploymentID: deploymentID,
		ServiceName:  serviceName,
		Definition:   definition,
		State:        GateStatePending,
		Progress: &GateProgress{
			Phase:       "initializing",
			Message:     "Starting health gate",
			ChecksTotal: len(definition.Checks),
		},
		Results:   []*CheckResult{},
		StartedAt: time.Now(),
		cancel:    cancel,
		done:      make(chan struct{}),
	}

	g.mu.Lock()
	g.activeGates[execution.ID] = execution
	g.mu.Unlock()

	defer func() {
		close(execution.done)
		g.mu.Lock()
		delete(g.activeGates, execution.ID)
		g.mu.Unlock()
	}()

	g.emitEvent(execution, "gate_started", nil)

	// Initial delay
	if definition.InitialDelay > 0 {
		execution.State = GateStateWaiting
		execution.Progress.Phase = "waiting"
		execution.Progress.Message = fmt.Sprintf("Waiting %v before health checks", definition.InitialDelay)

		select {
		case <-ctx.Done():
			return g.handleTimeout(execution)
		case <-time.After(definition.InitialDelay):
		}
	}

	// Run health check loop
	execution.State = GateStateChecking
	err := g.runHealthCheckLoop(ctx, execution)

	if err != nil {
		execution.State = GateStateFailed
		execution.Error = err.Error()
		execution.CompletedAt = time.Now()
		g.emitEvent(execution, "gate_failed", map[string]interface{}{"error": err.Error()})
		return err
	}

	execution.State = GateStatePassed
	execution.CompletedAt = time.Now()
	execution.Progress.Phase = "completed"
	execution.Progress.Message = "Health gate passed"
	g.emitEvent(execution, "gate_passed", nil)

	return nil
}

// runHealthCheckLoop runs the health check loop until success or failure.
func (g *HealthGate) runHealthCheckLoop(ctx context.Context, execution *GateExecution) error {
	definition := execution.Definition
	interval := definition.Interval
	if interval == 0 {
		interval = 5 * time.Second
	}

	successThreshold := definition.SuccessThreshold
	if successThreshold == 0 {
		successThreshold = 3
	}

	failureThreshold := definition.FailureThreshold
	if failureThreshold == 0 {
		failureThreshold = 3
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return g.handleTimeout(execution)
		case <-ticker.C:
		}

		// Run all checks
		execution.Progress.Phase = "checking"
		results := g.runAllChecks(ctx, execution)

		// Calculate health score
		healthScore := g.calculateHealthScore(results, definition.Checks)
		execution.Progress.HealthScore = healthScore

		// Count passed/failed
		passed := 0
		failed := 0
		for _, result := range results {
			if result.Passed {
				passed++
			} else {
				failed++
			}
		}

		execution.Progress.ChecksPassed = passed
		execution.Progress.ChecksFailed = failed

		// Determine if overall check passed
		checkPassed := healthScore >= float64(definition.HealthyPercentage)

		if checkPassed {
			execution.Progress.ConsecutiveSuccess++
			execution.Progress.ConsecutiveFailure = 0
			execution.Progress.Message = fmt.Sprintf("Health check passed (%d/%d)", execution.Progress.ConsecutiveSuccess, successThreshold)
		} else {
			execution.Progress.ConsecutiveFailure++
			execution.Progress.ConsecutiveSuccess = 0
			execution.Progress.Message = fmt.Sprintf("Health check failed (%d/%d)", execution.Progress.ConsecutiveFailure, failureThreshold)
		}

		g.emitEvent(execution, "check_completed", map[string]interface{}{
			"health_score":        healthScore,
			"consecutive_success": execution.Progress.ConsecutiveSuccess,
			"consecutive_failure": execution.Progress.ConsecutiveFailure,
		})

		// Check thresholds
		if execution.Progress.ConsecutiveSuccess >= successThreshold {
			return nil // Success!
		}

		if execution.Progress.ConsecutiveFailure >= failureThreshold {
			return fmt.Errorf("%w: %d consecutive failures", ErrHealthCheckFailed, failureThreshold)
		}
	}
}

// runAllChecks runs all health checks.
func (g *HealthGate) runAllChecks(ctx context.Context, execution *GateExecution) []*CheckResult {
	results := make([]*CheckResult, 0, len(execution.Definition.Checks))

	for _, check := range execution.Definition.Checks {
		if !check.Enabled {
			continue
		}

		result := g.runCheck(ctx, check)
		results = append(results, result)
		execution.Results = append(execution.Results, result)
	}

	return results
}

// runCheck runs a single health check.
func (g *HealthGate) runCheck(ctx context.Context, check *HealthCheck) *CheckResult {
	result := &CheckResult{
		Check:     check,
		Timestamp: time.Now(),
	}

	timeout := check.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	startTime := time.Now()

	var err error
	switch check.Type {
	case HealthCheckTypeHTTP:
		err = g.runHTTPCheck(checkCtx, check.HTTP, result)
	case HealthCheckTypeTCP:
		err = g.runTCPCheck(checkCtx, check.TCP, result)
	case HealthCheckTypeGRPC:
		err = g.runGRPCCheck(checkCtx, check.GRPC, result)
	case HealthCheckTypeExec:
		err = g.runExecCheck(checkCtx, check.Exec, result)
	default:
		err = fmt.Errorf("unknown check type: %s", check.Type)
	}

	result.Latency = time.Since(startTime)

	if err != nil {
		result.Passed = false
		result.Error = err.Error()
	} else {
		result.Passed = true
	}

	return result
}

// runHTTPCheck runs an HTTP health check.
func (g *HealthGate) runHTTPCheck(ctx context.Context, config *HTTPHealthCheck, result *CheckResult) error {
	if config == nil {
		return errors.New("HTTP check configuration is nil")
	}

	method := config.Method
	if method == "" {
		method = "GET"
	}

	var bodyReader io.Reader
	if config.Body != "" {
		bodyReader = bytes.NewReader([]byte(config.Body))
	}

	req, err := http.NewRequestWithContext(ctx, method, config.URL, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	for key, value := range config.Headers {
		req.Header.Set(key, value)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode

	// Check expected status
	expectedStatus := config.ExpectedStatus
	if expectedStatus == 0 {
		expectedStatus = 200
	}

	if resp.StatusCode != expectedStatus {
		return fmt.Errorf("unexpected status code: %d (expected %d)", resp.StatusCode, expectedStatus)
	}

	// Check expected body if configured
	if config.ExpectedBody != "" {
		buf := new(bytes.Buffer)
		buf.ReadFrom(resp.Body)
		result.Output = buf.String()

		if !strings.Contains(result.Output, config.ExpectedBody) {
			return fmt.Errorf("response body does not contain expected string")
		}
	}

	// Check expected JSON if configured
	if config.ExpectedJSON != nil {
		buf := new(bytes.Buffer)
		buf.ReadFrom(resp.Body)

		var responseJSON map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &responseJSON); err != nil {
			return fmt.Errorf("failed to parse JSON response: %w", err)
		}

		for key, expectedValue := range config.ExpectedJSON {
			actualValue, exists := responseJSON[key]
			if !exists {
				return fmt.Errorf("expected JSON key not found: %s", key)
			}
			if actualValue != expectedValue {
				return fmt.Errorf("JSON value mismatch for key %s: expected %v, got %v", key, expectedValue, actualValue)
			}
		}
	}

	return nil
}

// runTCPCheck runs a TCP health check.
func (g *HealthGate) runTCPCheck(ctx context.Context, config *TCPHealthCheck, result *CheckResult) error {
	if config == nil {
		return errors.New("TCP check configuration is nil")
	}

	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", config.Address)
	if err != nil {
		return fmt.Errorf("TCP connection failed: %w", err)
	}
	defer conn.Close()

	// Send data if configured
	if config.Send != "" {
		_, err = conn.Write([]byte(config.Send))
		if err != nil {
			return fmt.Errorf("failed to send data: %w", err)
		}
	}

	// Check expected response if configured
	if config.Expect != "" {
		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			return fmt.Errorf("failed to read response: %w", err)
		}
		result.Output = string(buf[:n])

		if !strings.Contains(result.Output, config.Expect) {
			return fmt.Errorf("response does not contain expected string")
		}
	}

	return nil
}

// runGRPCCheck runs a gRPC health check.
func (g *HealthGate) runGRPCCheck(ctx context.Context, config *GRPCHealthCheck, result *CheckResult) error {
	if config == nil {
		return errors.New("gRPC check configuration is nil")
	}

	// For gRPC health checks, we would typically use grpc-health-probe
	// Here we do a simple TCP connection check as a fallback
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", config.Address)
	if err != nil {
		return fmt.Errorf("gRPC connection failed: %w", err)
	}
	defer conn.Close()

	// In a real implementation, we would use the gRPC health checking protocol
	// For now, a successful TCP connection indicates the service is reachable

	return nil
}

// runExecCheck runs an exec health check.
func (g *HealthGate) runExecCheck(ctx context.Context, config *ExecHealthCheck, result *CheckResult) error {
	if config == nil {
		return errors.New("Exec check configuration is nil")
	}

	cmd := exec.CommandContext(ctx, config.Command, config.Args...)

	output, err := cmd.Output()
	result.Output = string(output)

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode := exitErr.ExitCode()
			if exitCode != config.ExpectedExitCode {
				return fmt.Errorf("unexpected exit code: %d (expected %d)", exitCode, config.ExpectedExitCode)
			}
		} else {
			return fmt.Errorf("command execution failed: %w", err)
		}
	}

	// Check expected output if configured
	if config.ExpectedOutput != "" {
		if !strings.Contains(result.Output, config.ExpectedOutput) {
			return fmt.Errorf("output does not contain expected string")
		}
	}

	return nil
}

// calculateHealthScore calculates the overall health score.
func (g *HealthGate) calculateHealthScore(results []*CheckResult, checks []*HealthCheck) float64 {
	if len(results) == 0 {
		return 0
	}

	totalWeight := 0.0
	weightedScore := 0.0

	checkWeights := make(map[string]float64)
	for _, check := range checks {
		weight := check.Weight
		if weight == 0 {
			weight = 1.0
		}
		checkWeights[check.Name] = weight
	}

	for _, result := range results {
		weight := checkWeights[result.Check.Name]
		if weight == 0 {
			weight = 1.0
		}
		totalWeight += weight
		if result.Passed {
			weightedScore += weight
		}
	}

	if totalWeight == 0 {
		return 0
	}

	return (weightedScore / totalWeight) * 100
}

// handleTimeout handles a gate timeout.
func (g *HealthGate) handleTimeout(execution *GateExecution) error {
	execution.State = GateStateFailed
	execution.Error = "health gate timeout"
	execution.CompletedAt = time.Now()
	g.emitEvent(execution, "gate_timeout", nil)
	return ErrHealthGateTimeout
}

// Abort aborts a running health gate.
func (g *HealthGate) Abort(gateID string) error {
	g.mu.Lock()
	execution, exists := g.activeGates[gateID]
	if !exists {
		g.mu.Unlock()
		return fmt.Errorf("gate not found: %s", gateID)
	}

	execution.State = GateStateAborted
	execution.CompletedAt = time.Now()
	g.mu.Unlock()

	if execution.cancel != nil {
		execution.cancel()
	}

	g.emitEvent(execution, "gate_aborted", nil)
	return nil
}

// Status returns the status of a health gate.
func (g *HealthGate) Status(gateID string) (*GateExecution, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	execution, exists := g.activeGates[gateID]
	if !exists {
		return nil, fmt.Errorf("gate not found: %s", gateID)
	}

	return execution, nil
}

// ListActive returns all active health gates.
func (g *HealthGate) ListActive() []*GateExecution {
	g.mu.RLock()
	defer g.mu.RUnlock()

	gates := make([]*GateExecution, 0, len(g.activeGates))
	for _, gate := range g.activeGates {
		gates = append(gates, gate)
	}
	return gates
}

// emitEvent emits a health gate event.
func (g *HealthGate) emitEvent(execution *GateExecution, eventType string, data map[string]interface{}) {
	g.mu.RLock()
	callback := g.eventCallback
	g.mu.RUnlock()

	if callback == nil {
		return
	}

	event := &HealthGateEvent{
		Type:         eventType,
		GateID:       execution.ID,
		DeploymentID: execution.DeploymentID,
		State:        execution.State,
		Progress:     execution.Progress,
		Timestamp:    time.Now(),
		Data:         data,
	}

	go callback(event)
}

// generateGateID generates a unique gate ID.
func generateGateID() string {
	return fmt.Sprintf("gate-%d", time.Now().UnixNano())
}

// HealthGateBuilder helps build health gate definitions.
type HealthGateBuilder struct {
	definition *HealthGateDefinition
}

// NewHealthGateBuilder creates a new health gate builder.
func NewHealthGateBuilder(name string) *HealthGateBuilder {
	return &HealthGateBuilder{
		definition: &HealthGateDefinition{
			Name:             name,
			Enabled:          true,
			SuccessThreshold: 3,
			FailureThreshold: 3,
			Checks:           []*HealthCheck{},
			Conditions:       []*HealthCondition{},
		},
	}
}

// WithInitialDelay sets the initial delay.
func (b *HealthGateBuilder) WithInitialDelay(delay time.Duration) *HealthGateBuilder {
	b.definition.InitialDelay = delay
	return b
}

// WithTimeout sets the timeout.
func (b *HealthGateBuilder) WithTimeout(timeout time.Duration) *HealthGateBuilder {
	b.definition.Timeout = timeout
	return b
}

// WithInterval sets the check interval.
func (b *HealthGateBuilder) WithInterval(interval time.Duration) *HealthGateBuilder {
	b.definition.Interval = interval
	return b
}

// WithSuccessThreshold sets the success threshold.
func (b *HealthGateBuilder) WithSuccessThreshold(threshold int) *HealthGateBuilder {
	b.definition.SuccessThreshold = threshold
	return b
}

// WithFailureThreshold sets the failure threshold.
func (b *HealthGateBuilder) WithFailureThreshold(threshold int) *HealthGateBuilder {
	b.definition.FailureThreshold = threshold
	return b
}

// WithHealthyPercentage sets the healthy percentage.
func (b *HealthGateBuilder) WithHealthyPercentage(percentage int) *HealthGateBuilder {
	b.definition.HealthyPercentage = percentage
	return b
}

// WithHTTPCheck adds an HTTP health check.
func (b *HealthGateBuilder) WithHTTPCheck(name, url string, expectedStatus int) *HealthGateBuilder {
	b.definition.Checks = append(b.definition.Checks, &HealthCheck{
		Name:    name,
		Type:    HealthCheckTypeHTTP,
		Enabled: true,
		Weight:  1.0,
		HTTP: &HTTPHealthCheck{
			URL:            url,
			Method:         "GET",
			ExpectedStatus: expectedStatus,
		},
		Timeout: 10 * time.Second,
	})
	return b
}

// WithTCPCheck adds a TCP health check.
func (b *HealthGateBuilder) WithTCPCheck(name, address string) *HealthGateBuilder {
	b.definition.Checks = append(b.definition.Checks, &HealthCheck{
		Name:    name,
		Type:    HealthCheckTypeTCP,
		Enabled: true,
		Weight:  1.0,
		TCP: &TCPHealthCheck{
			Address: address,
		},
		Timeout: 10 * time.Second,
	})
	return b
}

// WithExecCheck adds an exec health check.
func (b *HealthGateBuilder) WithExecCheck(name, command string, args ...string) *HealthGateBuilder {
	b.definition.Checks = append(b.definition.Checks, &HealthCheck{
		Name:    name,
		Type:    HealthCheckTypeExec,
		Enabled: true,
		Weight:  1.0,
		Exec: &ExecHealthCheck{
			Command:          command,
			Args:             args,
			ExpectedExitCode: 0,
		},
		Timeout: 30 * time.Second,
	})
	return b
}

// WithAbortOnFailure sets abort on failure.
func (b *HealthGateBuilder) WithAbortOnFailure(abort bool) *HealthGateBuilder {
	b.definition.AbortOnFailure = abort
	return b
}

// WithNotifyOnFailure sets notify on failure.
func (b *HealthGateBuilder) WithNotifyOnFailure(notify bool) *HealthGateBuilder {
	b.definition.NotifyOnFailure = notify
	return b
}

// Build returns the built health gate definition.
func (b *HealthGateBuilder) Build() *HealthGateDefinition {
	return b.definition
}

// QuickHealthGate creates a simple health gate with default settings.
func QuickHealthGate(name, healthURL string) *HealthGateDefinition {
	return NewHealthGateBuilder(name).
		WithTimeout(5 * time.Minute).
		WithInterval(5 * time.Second).
		WithSuccessThreshold(3).
		WithFailureThreshold(3).
		WithHTTPCheck("health", healthURL, 200).
		WithAbortOnFailure(true).
		Build()
}

// ReadinessGate creates a readiness-focused health gate.
func ReadinessGate(name, readyURL string) *HealthGateDefinition {
	return NewHealthGateBuilder(name).
		WithInitialDelay(10 * time.Second).
		WithTimeout(3 * time.Minute).
		WithInterval(3 * time.Second).
		WithSuccessThreshold(2).
		WithFailureThreshold(5).
		WithHTTPCheck("ready", readyURL, 200).
		WithAbortOnFailure(true).
		Build()
}

// LivenessGate creates a liveness-focused health gate.
func LivenessGate(name, livenessURL string) *HealthGateDefinition {
	return NewHealthGateBuilder(name).
		WithTimeout(2 * time.Minute).
		WithInterval(10 * time.Second).
		WithSuccessThreshold(1).
		WithFailureThreshold(3).
		WithHTTPCheck("live", livenessURL, 200).
		WithAbortOnFailure(false).
		Build()
}
