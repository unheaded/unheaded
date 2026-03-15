// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package pipeline provides the deployment pipeline for orchestrating deployment stages.
package pipeline

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestHealthCheckTypeConstants verifies health check type constants.
func TestHealthCheckTypeConstants(t *testing.T) {
	tests := []struct {
		checkType HealthCheckType
		expected  string
	}{
		{HealthCheckTypeHTTP, "http"},
		{HealthCheckTypeTCP, "tcp"},
		{HealthCheckTypeGRPC, "grpc"},
		{HealthCheckTypeExec, "exec"},
		{HealthCheckTypeCustom, "custom"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if string(tt.checkType) != tt.expected {
				t.Errorf("expected check type %q, got %q", tt.expected, tt.checkType)
			}
		})
	}
}

// TestGateStateConstants verifies gate state constants.
func TestGateStateConstants(t *testing.T) {
	tests := []struct {
		state    GateState
		expected string
	}{
		{GateStatePending, "pending"},
		{GateStateWaiting, "waiting"},
		{GateStateChecking, "checking"},
		{GateStatePassed, "passed"},
		{GateStateFailed, "failed"},
		{GateStateAborted, "aborted"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if string(tt.state) != tt.expected {
				t.Errorf("expected state %q, got %q", tt.expected, tt.state)
			}
		})
	}
}

// TestHealthGateErrors verifies health gate error constants.
func TestHealthGateErrors(t *testing.T) {
	tests := []struct {
		err      error
		expected string
	}{
		{ErrHealthGateTimeout, "health gate timeout"},
		{ErrHealthCheckFailed, "health check failed"},
		{ErrAllChecksFailed, "all health checks failed"},
		{ErrInsufficientHealthy, "insufficient healthy instances"},
		{ErrHealthGateAborted, "health gate aborted"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.err.Error() != tt.expected {
				t.Errorf("expected error %q, got %q", tt.expected, tt.err.Error())
			}
		})
	}
}

// TestNewHealthGate verifies HealthGate creation.
func TestNewHealthGate(t *testing.T) {
	gate := NewHealthGate()

	if gate == nil {
		t.Fatal("expected non-nil health gate")
	}

	if gate.httpClient == nil {
		t.Error("expected http client to be set")
	}

	if gate.activeGates == nil {
		t.Error("expected active gates map to be initialized")
	}

	if gate.defaultConfig == nil {
		t.Error("expected default config to be set")
	}
}

// TestDefaultHealthGateDefinition verifies default health gate definition.
func TestDefaultHealthGateDefinition(t *testing.T) {
	definition := DefaultHealthGateDefinition()

	if definition == nil {
		t.Fatal("expected non-nil definition")
	}

	if definition.Name != "default" {
		t.Errorf("expected name 'default', got %q", definition.Name)
	}

	if !definition.Enabled {
		t.Error("expected gate to be enabled")
	}

	if definition.InitialDelay != 10*time.Second {
		t.Errorf("expected initial delay 10s, got %v", definition.InitialDelay)
	}

	if definition.Timeout != 5*time.Minute {
		t.Errorf("expected timeout 5m, got %v", definition.Timeout)
	}

	if definition.Interval != 5*time.Second {
		t.Errorf("expected interval 5s, got %v", definition.Interval)
	}

	if definition.SuccessThreshold != 3 {
		t.Errorf("expected success threshold 3, got %d", definition.SuccessThreshold)
	}

	if definition.FailureThreshold != 3 {
		t.Errorf("expected failure threshold 3, got %d", definition.FailureThreshold)
	}

	if definition.HealthyPercentage != 100 {
		t.Errorf("expected healthy percentage 100, got %d", definition.HealthyPercentage)
	}

	if !definition.AbortOnFailure {
		t.Error("expected AbortOnFailure to be true")
	}

	if !definition.NotifyOnFailure {
		t.Error("expected NotifyOnFailure to be true")
	}

	if len(definition.Checks) != 1 {
		t.Errorf("expected 1 check, got %d", len(definition.Checks))
	}

	if definition.Checks[0].Name != "readiness" {
		t.Errorf("expected check name 'readiness', got %q", definition.Checks[0].Name)
	}

	if definition.Checks[0].Type != HealthCheckTypeHTTP {
		t.Errorf("expected check type 'http', got %q", definition.Checks[0].Type)
	}
}

// TestSetEventCallback verifies event callback setting.
func TestHealthGateSetEventCallback(t *testing.T) {
	gate := NewHealthGate()

	eventCh := make(chan *HealthGateEvent, 1)
	callback := func(event *HealthGateEvent) {
		eventCh <- event
	}

	gate.SetEventCallback(callback)

	gate.mu.RLock()
	callbackSet := gate.eventCallback != nil
	gate.mu.RUnlock()

	if !callbackSet {
		t.Error("expected event callback to be set")
	}

	// Emit an event to verify callback works
	execution := &GateExecution{
		ID:           "test-gate",
		DeploymentID: "deploy-1",
		State:        GateStatePending,
		Progress:     &GateProgress{},
	}

	gate.emitEvent(execution, "test_event", nil)

	select {
	case received := <-eventCh:
		if received == nil {
			t.Error("expected non-nil event")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event callback")
	}
}

// TestHTTPHealthCheckConfig verifies HTTP health check configuration.
func TestHTTPHealthCheckConfig(t *testing.T) {
	config := &HTTPHealthCheck{
		URL:               "http://localhost:8080/health",
		Method:            "GET",
		Headers:           map[string]string{"Authorization": "Bearer token"},
		Body:              "",
		ExpectedStatus:    200,
		ExpectedBody:      "OK",
		ExpectedJSON:      map[string]interface{}{"status": "healthy"},
		FollowRedirects:   false,
		InsecureSkipVerify: false,
	}

	if config.URL != "http://localhost:8080/health" {
		t.Errorf("expected URL 'http://localhost:8080/health', got %q", config.URL)
	}

	if config.Method != "GET" {
		t.Errorf("expected method 'GET', got %q", config.Method)
	}

	if config.ExpectedStatus != 200 {
		t.Errorf("expected status 200, got %d", config.ExpectedStatus)
	}

	if len(config.Headers) != 1 {
		t.Errorf("expected 1 header, got %d", len(config.Headers))
	}
}

// TestTCPHealthCheckConfig verifies TCP health check configuration.
func TestTCPHealthCheckConfig(t *testing.T) {
	config := &TCPHealthCheck{
		Address: "localhost:5432",
		Send:    "PING",
		Expect:  "PONG",
	}

	if config.Address != "localhost:5432" {
		t.Errorf("expected address 'localhost:5432', got %q", config.Address)
	}

	if config.Send != "PING" {
		t.Errorf("expected send 'PING', got %q", config.Send)
	}

	if config.Expect != "PONG" {
		t.Errorf("expected expect 'PONG', got %q", config.Expect)
	}
}

// TestGRPCHealthCheckConfig verifies gRPC health check configuration.
func TestGRPCHealthCheckConfig(t *testing.T) {
	config := &GRPCHealthCheck{
		Address:  "localhost:9090",
		Service:  "my.service.v1",
		Insecure: true,
	}

	if config.Address != "localhost:9090" {
		t.Errorf("expected address 'localhost:9090', got %q", config.Address)
	}

	if config.Service != "my.service.v1" {
		t.Errorf("expected service 'my.service.v1', got %q", config.Service)
	}

	if !config.Insecure {
		t.Error("expected Insecure to be true")
	}
}

// TestExecHealthCheckConfig verifies exec health check configuration.
func TestExecHealthCheckConfig(t *testing.T) {
	config := &ExecHealthCheck{
		Command:          "/bin/healthcheck",
		Args:             []string{"--service", "api"},
		ExpectedExitCode: 0,
		ExpectedOutput:   "healthy",
	}

	if config.Command != "/bin/healthcheck" {
		t.Errorf("expected command '/bin/healthcheck', got %q", config.Command)
	}

	if len(config.Args) != 2 {
		t.Errorf("expected 2 args, got %d", len(config.Args))
	}

	if config.ExpectedExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", config.ExpectedExitCode)
	}
}

// TestHealthCheck verifies HealthCheck structure.
func TestHealthCheck(t *testing.T) {
	check := &HealthCheck{
		Name:             "api-health",
		Type:             HealthCheckTypeHTTP,
		Enabled:          true,
		Weight:           0.5,
		Timeout:          10 * time.Second,
		SuccessThreshold: 2,
		FailureThreshold: 3,
		HTTP: &HTTPHealthCheck{
			URL:            "http://api:8080/health",
			ExpectedStatus: 200,
		},
	}

	if check.Name != "api-health" {
		t.Errorf("expected name 'api-health', got %q", check.Name)
	}

	if check.Type != HealthCheckTypeHTTP {
		t.Errorf("expected type 'http', got %q", check.Type)
	}

	if !check.Enabled {
		t.Error("expected check to be enabled")
	}

	if check.Weight != 0.5 {
		t.Errorf("expected weight 0.5, got %f", check.Weight)
	}

	if check.Timeout != 10*time.Second {
		t.Errorf("expected timeout 10s, got %v", check.Timeout)
	}
}

// TestHealthCondition verifies HealthCondition structure.
func TestHealthCondition(t *testing.T) {
	condition := &HealthCondition{
		Name:     "cpu_usage",
		Type:     "metric",
		Query:    "avg(cpu_usage{service=\"api\"})",
		Operator: "<",
		Value:    80.0,
		Weight:   0.3,
	}

	if condition.Name != "cpu_usage" {
		t.Errorf("expected name 'cpu_usage', got %q", condition.Name)
	}

	if condition.Type != "metric" {
		t.Errorf("expected type 'metric', got %q", condition.Type)
	}

	if condition.Operator != "<" {
		t.Errorf("expected operator '<', got %q", condition.Operator)
	}

	if condition.Value != 80.0 {
		t.Errorf("expected value 80.0, got %f", condition.Value)
	}
}

// TestGateProgress verifies GateProgress structure.
func TestGateProgress(t *testing.T) {
	progress := &GateProgress{
		Phase:              "checking",
		Message:            "Running health checks",
		ChecksTotal:        5,
		ChecksPassed:       3,
		ChecksFailed:       1,
		ConsecutiveSuccess: 2,
		ConsecutiveFailure: 0,
		HealthScore:        80.0,
		TimeRemaining:      2 * time.Minute,
	}

	if progress.Phase != "checking" {
		t.Errorf("expected phase 'checking', got %q", progress.Phase)
	}

	if progress.ChecksTotal != 5 {
		t.Errorf("expected checks total 5, got %d", progress.ChecksTotal)
	}

	if progress.ChecksPassed != 3 {
		t.Errorf("expected checks passed 3, got %d", progress.ChecksPassed)
	}

	if progress.HealthScore != 80.0 {
		t.Errorf("expected health score 80.0, got %f", progress.HealthScore)
	}

	if progress.ConsecutiveSuccess != 2 {
		t.Errorf("expected consecutive success 2, got %d", progress.ConsecutiveSuccess)
	}
}

// TestCheckResult verifies CheckResult structure.
func TestCheckResult(t *testing.T) {
	result := &CheckResult{
		Check: &HealthCheck{
			Name: "test-check",
			Type: HealthCheckTypeHTTP,
		},
		Passed:     true,
		StatusCode: 200,
		Latency:    50 * time.Millisecond,
		Output:     "OK",
		Timestamp:  time.Now(),
	}

	if !result.Passed {
		t.Error("expected check to pass")
	}

	if result.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", result.StatusCode)
	}

	if result.Latency != 50*time.Millisecond {
		t.Errorf("expected latency 50ms, got %v", result.Latency)
	}
}

// TestHealthGateEvent verifies HealthGateEvent structure.
func TestHealthGateEvent(t *testing.T) {
	event := &HealthGateEvent{
		Type:         "gate_passed",
		GateID:       "gate-123",
		DeploymentID: "deploy-456",
		State:        GateStatePassed,
		Progress: &GateProgress{
			Phase:       "completed",
			HealthScore: 100.0,
		},
		Timestamp: time.Now(),
		Data:      map[string]interface{}{"checks": 5},
	}

	if event.Type != "gate_passed" {
		t.Errorf("expected type 'gate_passed', got %q", event.Type)
	}

	if event.GateID != "gate-123" {
		t.Errorf("expected gate ID 'gate-123', got %q", event.GateID)
	}

	if event.State != GateStatePassed {
		t.Errorf("expected state 'passed', got %q", event.State)
	}
}

// TestHealthGateDefinition verifies HealthGateDefinition structure.
func TestHealthGateDefinition(t *testing.T) {
	definition := &HealthGateDefinition{
		Name:              "deployment-gate",
		Enabled:           true,
		InitialDelay:      15 * time.Second,
		Timeout:           10 * time.Minute,
		Interval:          3 * time.Second,
		SuccessThreshold:  5,
		FailureThreshold:  2,
		HealthyPercentage: 80,
		Checks: []*HealthCheck{
			{Name: "check1", Type: HealthCheckTypeHTTP, Enabled: true},
			{Name: "check2", Type: HealthCheckTypeTCP, Enabled: true},
		},
		Conditions: []*HealthCondition{
			{Name: "metric1", Type: "metric", Operator: "<", Value: 50},
		},
		ProgressiveRollout: true,
		AbortOnFailure:     true,
		NotifyOnFailure:    true,
	}

	if definition.Name != "deployment-gate" {
		t.Errorf("expected name 'deployment-gate', got %q", definition.Name)
	}

	if definition.SuccessThreshold != 5 {
		t.Errorf("expected success threshold 5, got %d", definition.SuccessThreshold)
	}

	if definition.HealthyPercentage != 80 {
		t.Errorf("expected healthy percentage 80, got %d", definition.HealthyPercentage)
	}

	if len(definition.Checks) != 2 {
		t.Errorf("expected 2 checks, got %d", len(definition.Checks))
	}

	if len(definition.Conditions) != 1 {
		t.Errorf("expected 1 condition, got %d", len(definition.Conditions))
	}

	if !definition.ProgressiveRollout {
		t.Error("expected ProgressiveRollout to be true")
	}
}

// TestExecuteWithDisabledGate verifies that disabled gates are skipped.
func TestExecuteWithDisabledGate(t *testing.T) {
	gate := NewHealthGate()

	definition := &HealthGateDefinition{
		Name:    "disabled-gate",
		Enabled: false,
	}

	err := gate.Execute(context.Background(), "deploy-1", "test-service", definition)

	if err != nil {
		t.Errorf("expected no error for disabled gate, got %v", err)
	}
}

// TestExecuteWithNilDefinition verifies default config is used for nil definition.
func TestExecuteWithNilDefinition(t *testing.T) {
	gate := NewHealthGate()

	// Disable the default config to avoid actual health checks
	gate.defaultConfig.Enabled = false

	err := gate.Execute(context.Background(), "deploy-1", "test-service", nil)

	if err != nil {
		t.Errorf("expected no error with disabled default config, got %v", err)
	}
}

// TestHTTPHealthCheck verifies HTTP health check execution.
func TestHTTPHealthCheckExecution(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	gate := NewHealthGate()

	httpConfig := &HTTPHealthCheck{
		URL:            server.URL + "/health",
		Method:         "GET",
		ExpectedStatus: 200,
	}

	result := &CheckResult{
		Check:     &HealthCheck{Name: "http-check", Type: HealthCheckTypeHTTP},
		Timestamp: time.Now(),
	}

	err := gate.runHTTPCheck(context.Background(), httpConfig, result)

	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	if result.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", result.StatusCode)
	}
}

// TestHTTPHealthCheckWithExpectedBody verifies HTTP health check with body check.
func TestHTTPHealthCheckWithExpectedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "healthy", "version": "1.0.0"}`))
	}))
	defer server.Close()

	gate := NewHealthGate()

	httpConfig := &HTTPHealthCheck{
		URL:            server.URL,
		Method:         "GET",
		ExpectedStatus: 200,
		ExpectedBody:   "healthy",
	}

	result := &CheckResult{
		Check:     &HealthCheck{Name: "http-body-check", Type: HealthCheckTypeHTTP},
		Timestamp: time.Now(),
	}

	err := gate.runHTTPCheck(context.Background(), httpConfig, result)

	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

// TestHTTPHealthCheckFailure verifies HTTP health check failure handling.
func TestHTTPHealthCheckFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	gate := NewHealthGate()

	httpConfig := &HTTPHealthCheck{
		URL:            server.URL,
		Method:         "GET",
		ExpectedStatus: 200,
	}

	result := &CheckResult{
		Check:     &HealthCheck{Name: "http-fail-check", Type: HealthCheckTypeHTTP},
		Timestamp: time.Now(),
	}

	err := gate.runHTTPCheck(context.Background(), httpConfig, result)

	if err == nil {
		t.Error("expected error for status mismatch")
	}

	if result.StatusCode != 503 {
		t.Errorf("expected status 503, got %d", result.StatusCode)
	}
}

// TestCalculateHealthScore verifies health score calculation.
func TestCalculateHealthScore(t *testing.T) {
	gate := NewHealthGate()

	checks := []*HealthCheck{
		{Name: "check1", Weight: 1.0},
		{Name: "check2", Weight: 1.0},
		{Name: "check3", Weight: 2.0},
	}

	tests := []struct {
		name     string
		results  []*CheckResult
		expected float64
	}{
		{
			name: "all pass",
			results: []*CheckResult{
				{Check: &HealthCheck{Name: "check1"}, Passed: true},
				{Check: &HealthCheck{Name: "check2"}, Passed: true},
				{Check: &HealthCheck{Name: "check3"}, Passed: true},
			},
			expected: 100.0,
		},
		{
			name: "all fail",
			results: []*CheckResult{
				{Check: &HealthCheck{Name: "check1"}, Passed: false},
				{Check: &HealthCheck{Name: "check2"}, Passed: false},
				{Check: &HealthCheck{Name: "check3"}, Passed: false},
			},
			expected: 0.0,
		},
		{
			name: "weighted 50%",
			results: []*CheckResult{
				{Check: &HealthCheck{Name: "check1"}, Passed: true},
				{Check: &HealthCheck{Name: "check2"}, Passed: true},
				{Check: &HealthCheck{Name: "check3"}, Passed: false},
			},
			expected: 50.0, // (1+1+0) / (1+1+2) * 100 = 50
		},
		{
			name:     "empty results",
			results:  []*CheckResult{},
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := gate.calculateHealthScore(tt.results, checks)
			if score != tt.expected {
				t.Errorf("expected score %f, got %f", tt.expected, score)
			}
		})
	}
}

// TestHealthGateBuilder verifies HealthGateBuilder functionality.
func TestHealthGateBuilder(t *testing.T) {
	definition := NewHealthGateBuilder("test-gate").
		WithInitialDelay(5 * time.Second).
		WithTimeout(1 * time.Minute).
		WithInterval(2 * time.Second).
		WithSuccessThreshold(2).
		WithFailureThreshold(4).
		WithHealthyPercentage(90).
		WithHTTPCheck("api", "http://api:8080/health", 200).
		WithTCPCheck("db", "db:5432").
		WithAbortOnFailure(true).
		WithNotifyOnFailure(true).
		Build()

	if definition.Name != "test-gate" {
		t.Errorf("expected name 'test-gate', got %q", definition.Name)
	}

	if definition.InitialDelay != 5*time.Second {
		t.Errorf("expected initial delay 5s, got %v", definition.InitialDelay)
	}

	if definition.Timeout != 1*time.Minute {
		t.Errorf("expected timeout 1m, got %v", definition.Timeout)
	}

	if definition.Interval != 2*time.Second {
		t.Errorf("expected interval 2s, got %v", definition.Interval)
	}

	if definition.SuccessThreshold != 2 {
		t.Errorf("expected success threshold 2, got %d", definition.SuccessThreshold)
	}

	if definition.FailureThreshold != 4 {
		t.Errorf("expected failure threshold 4, got %d", definition.FailureThreshold)
	}

	if definition.HealthyPercentage != 90 {
		t.Errorf("expected healthy percentage 90, got %d", definition.HealthyPercentage)
	}

	if len(definition.Checks) != 2 {
		t.Errorf("expected 2 checks, got %d", len(definition.Checks))
	}

	if definition.Checks[0].Type != HealthCheckTypeHTTP {
		t.Errorf("expected first check type 'http', got %q", definition.Checks[0].Type)
	}

	if definition.Checks[1].Type != HealthCheckTypeTCP {
		t.Errorf("expected second check type 'tcp', got %q", definition.Checks[1].Type)
	}

	if !definition.AbortOnFailure {
		t.Error("expected AbortOnFailure to be true")
	}
}

// TestQuickHealthGate verifies QuickHealthGate helper.
func TestQuickHealthGate(t *testing.T) {
	definition := QuickHealthGate("quick-gate", "http://service/health")

	if definition.Name != "quick-gate" {
		t.Errorf("expected name 'quick-gate', got %q", definition.Name)
	}

	if definition.Timeout != 5*time.Minute {
		t.Errorf("expected timeout 5m, got %v", definition.Timeout)
	}

	if len(definition.Checks) != 1 {
		t.Errorf("expected 1 check, got %d", len(definition.Checks))
	}

	if definition.Checks[0].HTTP.URL != "http://service/health" {
		t.Errorf("expected URL 'http://service/health', got %q", definition.Checks[0].HTTP.URL)
	}
}

// TestReadinessGate verifies ReadinessGate helper.
func TestReadinessGate(t *testing.T) {
	definition := ReadinessGate("readiness-gate", "http://service/ready")

	if definition.Name != "readiness-gate" {
		t.Errorf("expected name 'readiness-gate', got %q", definition.Name)
	}

	if definition.InitialDelay != 10*time.Second {
		t.Errorf("expected initial delay 10s, got %v", definition.InitialDelay)
	}

	if definition.Timeout != 3*time.Minute {
		t.Errorf("expected timeout 3m, got %v", definition.Timeout)
	}

	if definition.SuccessThreshold != 2 {
		t.Errorf("expected success threshold 2, got %d", definition.SuccessThreshold)
	}

	if definition.FailureThreshold != 5 {
		t.Errorf("expected failure threshold 5, got %d", definition.FailureThreshold)
	}
}

// TestLivenessGate verifies LivenessGate helper.
func TestLivenessGate(t *testing.T) {
	definition := LivenessGate("liveness-gate", "http://service/live")

	if definition.Name != "liveness-gate" {
		t.Errorf("expected name 'liveness-gate', got %q", definition.Name)
	}

	if definition.Timeout != 2*time.Minute {
		t.Errorf("expected timeout 2m, got %v", definition.Timeout)
	}

	if definition.Interval != 10*time.Second {
		t.Errorf("expected interval 10s, got %v", definition.Interval)
	}

	if definition.SuccessThreshold != 1 {
		t.Errorf("expected success threshold 1, got %d", definition.SuccessThreshold)
	}

	if definition.AbortOnFailure {
		// LivenessGate should NOT abort on failure
		t.Error("expected AbortOnFailure to be false for liveness gate")
	}
}

// TestGenerateGateID verifies gate ID generation.
func TestGenerateGateID(t *testing.T) {
	id1 := generateGateID()
	time.Sleep(time.Nanosecond * 100)
	id2 := generateGateID()

	if id1 == "" {
		t.Error("expected non-empty gate ID")
	}

	if id1 == id2 {
		t.Errorf("expected unique gate IDs, got same: %q", id1)
	}

	if len(id1) < 5 || id1[:5] != "gate-" {
		t.Errorf("expected gate ID to start with 'gate-', got %q", id1)
	}
}

// TestGateExecution verifies GateExecution structure.
func TestGateExecution(t *testing.T) {
	execution := &GateExecution{
		ID:           "gate-123",
		DeploymentID: "deploy-456",
		ServiceName:  "test-service",
		Definition:   DefaultHealthGateDefinition(),
		State:        GateStateChecking,
		Progress: &GateProgress{
			Phase:       "checking",
			HealthScore: 75.0,
		},
		Results:   []*CheckResult{},
		StartedAt: time.Now(),
	}

	if execution.ID != "gate-123" {
		t.Errorf("expected ID 'gate-123', got %q", execution.ID)
	}

	if execution.ServiceName != "test-service" {
		t.Errorf("expected service name 'test-service', got %q", execution.ServiceName)
	}

	if execution.State != GateStateChecking {
		t.Errorf("expected state 'checking', got %q", execution.State)
	}
}

// TestListActive verifies ListActive functionality.
func TestListActive(t *testing.T) {
	gate := NewHealthGate()

	// Initially no active gates
	active := gate.ListActive()
	if len(active) != 0 {
		t.Errorf("expected 0 active gates, got %d", len(active))
	}

	// Add some executions manually
	gate.mu.Lock()
	gate.activeGates["gate-1"] = &GateExecution{ID: "gate-1", State: GateStateChecking}
	gate.activeGates["gate-2"] = &GateExecution{ID: "gate-2", State: GateStateWaiting}
	gate.mu.Unlock()

	active = gate.ListActive()
	if len(active) != 2 {
		t.Errorf("expected 2 active gates, got %d", len(active))
	}
}

// TestAbort verifies gate abort functionality.
func TestAbort(t *testing.T) {
	gate := NewHealthGate()

	// Add an execution
	ctx, cancel := context.WithCancel(context.Background())
	execution := &GateExecution{
		ID:     "gate-abort",
		State:  GateStateChecking,
		cancel: cancel,
	}

	gate.mu.Lock()
	gate.activeGates["gate-abort"] = execution
	gate.mu.Unlock()

	// Abort it
	err := gate.Abort("gate-abort")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if execution.State != GateStateAborted {
		t.Errorf("expected state 'aborted', got %q", execution.State)
	}

	// Verify context was cancelled
	select {
	case <-ctx.Done():
		// Expected
	default:
		t.Error("expected context to be cancelled")
	}
}

// TestAbortNotFound verifies abort with non-existent gate.
func TestAbortNotFound(t *testing.T) {
	gate := NewHealthGate()

	err := gate.Abort("nonexistent")

	if err == nil {
		t.Error("expected error for nonexistent gate")
	}
}

// TestStatus verifies status retrieval.
func TestStatus(t *testing.T) {
	gate := NewHealthGate()

	execution := &GateExecution{
		ID:          "gate-status",
		ServiceName: "test-service",
		State:       GateStatePassed,
	}

	gate.mu.Lock()
	gate.activeGates["gate-status"] = execution
	gate.mu.Unlock()

	status, err := gate.Status("gate-status")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if status.ID != "gate-status" {
		t.Errorf("expected ID 'gate-status', got %q", status.ID)
	}

	if status.ServiceName != "test-service" {
		t.Errorf("expected service name 'test-service', got %q", status.ServiceName)
	}
}

// TestStatusNotFound verifies status with non-existent gate.
func TestStatusNotFound(t *testing.T) {
	gate := NewHealthGate()

	_, err := gate.Status("nonexistent")

	if err == nil {
		t.Error("expected error for nonexistent gate")
	}
}

// TestConcurrentGateAccess verifies thread-safe access to health gate.
func TestConcurrentGateAccess(t *testing.T) {
	gate := NewHealthGate()

	var wg sync.WaitGroup
	errorsChan := make(chan error, 100)

	// Concurrent operations
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			gateID := "gate-" + string(rune('a'+id))

			// Add gate
			gate.mu.Lock()
			gate.activeGates[gateID] = &GateExecution{
				ID:    gateID,
				State: GateStateChecking,
			}
			gate.mu.Unlock()

			// List active
			gate.ListActive()

			// Get status
			gate.Status(gateID)

			// Remove gate
			gate.mu.Lock()
			delete(gate.activeGates, gateID)
			gate.mu.Unlock()
		}(i)
	}

	wg.Wait()
	close(errorsChan)

	for err := range errorsChan {
		t.Errorf("concurrent access error: %v", err)
	}
}

// BenchmarkCalculateHealthScore benchmarks health score calculation.
func BenchmarkCalculateHealthScore(b *testing.B) {
	gate := NewHealthGate()

	checks := []*HealthCheck{
		{Name: "check1", Weight: 1.0},
		{Name: "check2", Weight: 1.0},
		{Name: "check3", Weight: 2.0},
		{Name: "check4", Weight: 1.0},
		{Name: "check5", Weight: 3.0},
	}

	results := []*CheckResult{
		{Check: &HealthCheck{Name: "check1"}, Passed: true},
		{Check: &HealthCheck{Name: "check2"}, Passed: true},
		{Check: &HealthCheck{Name: "check3"}, Passed: false},
		{Check: &HealthCheck{Name: "check4"}, Passed: true},
		{Check: &HealthCheck{Name: "check5"}, Passed: true},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gate.calculateHealthScore(results, checks)
	}
}

// BenchmarkGenerateGateID benchmarks gate ID generation.
func BenchmarkGenerateGateID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		generateGateID()
	}
}
