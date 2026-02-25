// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package pipeline provides the deployment pipeline for orchestrating deployment stages.
package pipeline

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// TestDeploymentStrategyConstants verifies deployment strategy constants.
func TestDeploymentStrategyConstants(t *testing.T) {
	tests := []struct {
		strategy DeploymentStrategy
		expected string
	}{
		{StrategyRolling, "rolling"},
		{StrategyBlueGreen, "blue_green"},
		{StrategyCanary, "canary"},
		{StrategyRecreate, "recreate"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if string(tt.strategy) != tt.expected {
				t.Errorf("expected strategy %q, got %q", tt.expected, tt.strategy)
			}
		})
	}
}

// TestStrategyExecutionStateConstants verifies execution state constants.
func TestStrategyExecutionStateConstants(t *testing.T) {
	tests := []struct {
		state    StrategyExecutionState
		expected string
	}{
		{ExecutionStatePending, "pending"},
		{ExecutionStateRunning, "running"},
		{ExecutionStatePaused, "paused"},
		{ExecutionStateAnalyzing, "analyzing"},
		{ExecutionStatePromoting, "promoting"},
		{ExecutionStateCompleted, "completed"},
		{ExecutionStateFailed, "failed"},
		{ExecutionStateAborted, "aborted"},
		{ExecutionStateRollingBack, "rolling_back"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if string(tt.state) != tt.expected {
				t.Errorf("expected state %q, got %q", tt.expected, tt.state)
			}
		})
	}
}

// TestStrategyErrors verifies strategy error constants.
func TestStrategyErrors(t *testing.T) {
	tests := []struct {
		err      error
		expected string
	}{
		{ErrStrategyNotConfigured, "deployment strategy not configured"},
		{ErrStrategyExecutionFail, "strategy execution failed"},
		{ErrTrafficShiftFailed, "traffic shift failed"},
		{ErrAnalysisFailed, "canary analysis failed"},
		{ErrPromotionFailed, "promotion failed"},
		{ErrHealthGateFailed, "health gate failed"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.err.Error() != tt.expected {
				t.Errorf("expected error %q, got %q", tt.expected, tt.err.Error())
			}
		})
	}
}

// MockStrategyTrafficManager implements TrafficManager for testing.
type MockStrategyTrafficManager struct {
	mu           sync.Mutex
	weights      map[string]int
	shiftHistory []strategyTrafficShiftRecord
	shiftErr     error
}

type strategyTrafficShiftRecord struct {
	service     string
	fromVersion string
	toVersion   string
	weight      int
}

func NewMockStrategyTrafficManager() *MockStrategyTrafficManager {
	return &MockStrategyTrafficManager{
		weights: make(map[string]int),
	}
}

func (m *MockStrategyTrafficManager) ShiftTraffic(ctx context.Context, service, fromVersion, toVersion string, weight int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shiftErr != nil {
		return m.shiftErr
	}

	m.shiftHistory = append(m.shiftHistory, strategyTrafficShiftRecord{
		service:     service,
		fromVersion: fromVersion,
		toVersion:   toVersion,
		weight:      weight,
	})

	m.weights[service+":"+toVersion] = weight
	m.weights[service+":"+fromVersion] = 100 - weight

	return nil
}

func (m *MockStrategyTrafficManager) GetTrafficWeight(ctx context.Context, service, version string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.weights[service+":"+version], nil
}

func (m *MockStrategyTrafficManager) GetShiftHistory() []strategyTrafficShiftRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]strategyTrafficShiftRecord{}, m.shiftHistory...)
}

// MockStrategyHealthChecker implements HealthChecker for testing.
type MockStrategyHealthChecker struct {
	mu         sync.Mutex
	healthy    bool
	checkErr   error
	checkCount int
}

func (m *MockStrategyHealthChecker) CheckHealth(ctx context.Context, service string, endpoints []HealthEndpoint) (*HealthResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkCount++

	if m.checkErr != nil {
		return nil, m.checkErr
	}

	return &HealthResult{
		Healthy:      m.healthy,
		HealthyCount: 1,
		TotalCount:   1,
		CheckResults: []HealthCheckResult{
			{Name: "test", Healthy: m.healthy},
		},
	}, nil
}

func (m *MockStrategyHealthChecker) CheckCustom(ctx context.Context, check *CustomHealthCheck) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.checkErr != nil {
		return m.checkErr
	}
	return nil
}

// MockStrategyMetricsProvider implements MetricsProvider for testing.
type MockStrategyMetricsProvider struct {
	mu         sync.Mutex
	metrics    *AnalysisMetrics
	metricsErr error
	queryVal   float64
	queryErr   error
}

func (m *MockStrategyMetricsProvider) GetMetrics(ctx context.Context, service, version string, duration time.Duration) (*AnalysisMetrics, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.metricsErr != nil {
		return nil, m.metricsErr
	}

	if m.metrics == nil {
		return &AnalysisMetrics{
			ErrorRate:    0.01,
			SuccessRate:  0.99,
			LatencyP50:   10 * time.Millisecond,
			LatencyP95:   50 * time.Millisecond,
			LatencyP99:   100 * time.Millisecond,
			RequestCount: 1000,
			ErrorCount:   10,
			SampleSize:   1000,
		}, nil
	}

	return m.metrics, nil
}

func (m *MockStrategyMetricsProvider) ExecuteQuery(ctx context.Context, query string, params map[string]string) (float64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.queryErr != nil {
		return 0, m.queryErr
	}

	return m.queryVal, nil
}

// MockStrategyNotifier implements Notifier for testing.
type MockStrategyNotifier struct {
	mu            sync.Mutex
	notifications []*StrategyEvent
	notifyErr     error
}

func (m *MockStrategyNotifier) Notify(ctx context.Context, channel *NotificationChannel, event *StrategyEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.notifyErr != nil {
		return m.notifyErr
	}

	m.notifications = append(m.notifications, event)
	return nil
}

func (m *MockStrategyNotifier) GetNotifications() []*StrategyEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]*StrategyEvent{}, m.notifications...)
}

// MockStrategyInstanceManager implements InstanceManager for testing.
type MockStrategyInstanceManager struct {
	mu              sync.Mutex
	instances       map[string][]InstanceInfo
	createErr       error
	deleteErr       error
	createCalls     int
	deleteCalls     int
	createdVersions []string
	deletedVersions []string
}

func NewMockStrategyInstanceManager() *MockStrategyInstanceManager {
	return &MockStrategyInstanceManager{
		instances: make(map[string][]InstanceInfo),
	}
}

func (m *MockStrategyInstanceManager) CreateInstances(ctx context.Context, service, version string, count int, labels map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.createErr != nil {
		return m.createErr
	}

	m.createCalls++
	m.createdVersions = append(m.createdVersions, version)

	key := service + ":" + version
	for i := 0; i < count; i++ {
		m.instances[key] = append(m.instances[key], InstanceInfo{
			ID:      "instance-" + version + "-" + string(rune('a'+i)),
			Service: service,
			Version: version,
			State:   "running",
			Healthy: true,
			Labels:  labels,
		})
	}

	return nil
}

func (m *MockStrategyInstanceManager) DeleteInstances(ctx context.Context, service, version string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.deleteErr != nil {
		return m.deleteErr
	}

	m.deleteCalls++
	m.deletedVersions = append(m.deletedVersions, version)

	key := service + ":" + version
	delete(m.instances, key)

	return nil
}

func (m *MockStrategyInstanceManager) GetInstances(ctx context.Context, service, version string) ([]InstanceInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := service + ":" + version
	return m.instances[key], nil
}

func (m *MockStrategyInstanceManager) UpdateInstance(ctx context.Context, instanceID, version string) error {
	return nil
}

// TestNewStrategyExecutor verifies StrategyExecutor creation.
func TestNewStrategyExecutor(t *testing.T) {
	traffic := NewMockStrategyTrafficManager()
	health := &MockStrategyHealthChecker{healthy: true}
	metrics := &MockStrategyMetricsProvider{}
	notifier := &MockStrategyNotifier{}
	instances := NewMockStrategyInstanceManager()

	executor := NewStrategyExecutor(traffic, health, metrics, notifier, instances)

	if executor == nil {
		t.Fatal("expected non-nil executor")
	}

	if executor.trafficManager == nil {
		t.Error("expected traffic manager to be set")
	}

	if executor.healthChecker == nil {
		t.Error("expected health checker to be set")
	}

	if executor.metricsProvider == nil {
		t.Error("expected metrics provider to be set")
	}

	if executor.notifier == nil {
		t.Error("expected notifier to be set")
	}

	if executor.instanceManager == nil {
		t.Error("expected instance manager to be set")
	}
}

// TestStrategyExecutorSetEventCallback verifies event callback setting.
func TestStrategyExecutorSetEventCallback(t *testing.T) {
	executor := NewStrategyExecutor(nil, nil, nil, nil, nil)

	eventCh := make(chan *StrategyEvent, 1)
	callback := func(event *StrategyEvent) {
		eventCh <- event
	}

	executor.SetEventCallback(callback)

	executor.mu.RLock()
	callbackSet := executor.eventCallback != nil
	executor.mu.RUnlock()

	if !callbackSet {
		t.Error("expected event callback to be set")
	}

	// Verify callback is called
	execution := &StrategyExecution{
		ID:           "test-exec",
		DeploymentID: "deploy-1",
		State:        ExecutionStateRunning,
		Progress:     &StrategyProgress{},
	}

	executor.emitEvent(execution, "test_event", nil)

	select {
	case receivedEvent := <-eventCh:
		if receivedEvent == nil {
			t.Error("expected non-nil event to be received")
		}
	case <-time.After(time.Second):
		t.Error("expected event to be received")
	}
}

// TestExecuteWithNilConfig verifies error handling for nil config.
func TestExecuteWithNilConfig(t *testing.T) {
	executor := NewStrategyExecutor(nil, nil, nil, nil, nil)

	_, err := executor.Execute(context.Background(), "deploy-1", nil, &ExecutionParams{})

	if !errors.Is(err, ErrStrategyNotConfigured) {
		t.Errorf("expected ErrStrategyNotConfigured, got %v", err)
	}
}

// TestDefaultStrategyConfig verifies default strategy configuration.
func TestDefaultStrategyConfig(t *testing.T) {
	tests := []struct {
		name         string
		strategyType DeploymentStrategy
		verify       func(t *testing.T, config *StrategyConfig)
	}{
		{
			name:         "rolling",
			strategyType: StrategyRolling,
			verify: func(t *testing.T, config *StrategyConfig) {
				if config.Rolling == nil {
					t.Fatal("expected rolling config")
				}
				if config.Rolling.MaxSurge != 1 {
					t.Errorf("expected MaxSurge 1, got %d", config.Rolling.MaxSurge)
				}
				if config.Rolling.MaxUnavailable != 0 {
					t.Errorf("expected MaxUnavailable 0, got %d", config.Rolling.MaxUnavailable)
				}
				if config.Rolling.ProgressDeadline != 10*time.Minute {
					t.Errorf("expected ProgressDeadline 10m, got %v", config.Rolling.ProgressDeadline)
				}
				if config.Rolling.MinReadySeconds != 30 {
					t.Errorf("expected MinReadySeconds 30, got %d", config.Rolling.MinReadySeconds)
				}
			},
		},
		{
			name:         "blue-green",
			strategyType: StrategyBlueGreen,
			verify: func(t *testing.T, config *StrategyConfig) {
				if config.BlueGreen == nil {
					t.Fatal("expected blue-green config")
				}
				if config.BlueGreen.SwitchTimeout != 30*time.Second {
					t.Errorf("expected SwitchTimeout 30s, got %v", config.BlueGreen.SwitchTimeout)
				}
				if config.BlueGreen.TestDuration != 5*time.Minute {
					t.Errorf("expected TestDuration 5m, got %v", config.BlueGreen.TestDuration)
				}
				if config.BlueGreen.AutoPromote {
					t.Error("expected AutoPromote to be false")
				}
			},
		},
		{
			name:         "canary",
			strategyType: StrategyCanary,
			verify: func(t *testing.T, config *StrategyConfig) {
				if config.Canary == nil {
					t.Fatal("expected canary config")
				}
				if len(config.Canary.Steps) != 4 {
					t.Errorf("expected 4 canary steps, got %d", len(config.Canary.Steps))
				}
				if config.Canary.Analysis == nil {
					t.Fatal("expected canary analysis config")
				}
				if config.Canary.Analysis.ErrorRateThreshold != 0.01 {
					t.Errorf("expected ErrorRateThreshold 0.01, got %f", config.Canary.Analysis.ErrorRateThreshold)
				}
				if config.Canary.CanaryReplicas != 1 {
					t.Errorf("expected CanaryReplicas 1, got %d", config.Canary.CanaryReplicas)
				}
			},
		},
		{
			name:         "recreate",
			strategyType: StrategyRecreate,
			verify: func(t *testing.T, config *StrategyConfig) {
				// Recreate strategy has no specific config
				if config.Type != StrategyRecreate {
					t.Errorf("expected strategy type 'recreate', got %q", config.Type)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultStrategyConfig(tt.strategyType)

			if config == nil {
				t.Fatal("expected non-nil config")
			}

			if config.Type != tt.strategyType {
				t.Errorf("expected strategy type %q, got %q", tt.strategyType, config.Type)
			}

			if config.Timeout != 1*time.Hour {
				t.Errorf("expected timeout 1h, got %v", config.Timeout)
			}

			if config.HealthGate == nil {
				t.Fatal("expected health gate config")
			}

			if !config.HealthGate.Enabled {
				t.Error("expected health gate to be enabled")
			}

			tt.verify(t, config)
		})
	}
}

// TestRollingConfig verifies rolling deployment configuration.
func TestRollingConfig(t *testing.T) {
	config := &RollingConfig{
		MaxSurge:         2,
		MaxUnavailable:   1,
		ProgressDeadline: 15 * time.Minute,
		MinReadySeconds:  60,
		BatchSize:        5,
		PauseAfterBatch:  true,
	}

	if config.MaxSurge != 2 {
		t.Errorf("expected MaxSurge 2, got %d", config.MaxSurge)
	}

	if config.MaxUnavailable != 1 {
		t.Errorf("expected MaxUnavailable 1, got %d", config.MaxUnavailable)
	}

	if config.BatchSize != 5 {
		t.Errorf("expected BatchSize 5, got %d", config.BatchSize)
	}

	if !config.PauseAfterBatch {
		t.Error("expected PauseAfterBatch to be true")
	}
}

// TestBlueGreenConfig verifies blue-green deployment configuration.
func TestBlueGreenConfig(t *testing.T) {
	config := &BlueGreenConfig{
		SwitchTimeout:     1 * time.Minute,
		TestDuration:      10 * time.Minute,
		PreviewEndpoint:   "https://preview.example.com",
		AutoPromote:       true,
		KeepBlueInstances: true,
		KeepBlueDuration:  30 * time.Minute,
	}

	if config.SwitchTimeout != 1*time.Minute {
		t.Errorf("expected SwitchTimeout 1m, got %v", config.SwitchTimeout)
	}

	if config.TestDuration != 10*time.Minute {
		t.Errorf("expected TestDuration 10m, got %v", config.TestDuration)
	}

	if config.PreviewEndpoint != "https://preview.example.com" {
		t.Errorf("expected PreviewEndpoint 'https://preview.example.com', got %q", config.PreviewEndpoint)
	}

	if !config.AutoPromote {
		t.Error("expected AutoPromote to be true")
	}

	if !config.KeepBlueInstances {
		t.Error("expected KeepBlueInstances to be true")
	}
}

// TestCanaryConfig verifies canary deployment configuration.
func TestCanaryConfig(t *testing.T) {
	config := &CanaryConfig{
		Steps: []CanaryStepConfig{
			{Weight: 5, Duration: 2 * time.Minute, Analysis: true, Pause: false},
			{Weight: 25, Duration: 5 * time.Minute, Analysis: true, Pause: false},
			{Weight: 50, Duration: 10 * time.Minute, Analysis: true, Pause: true},
			{Weight: 100, Duration: 0, Analysis: false, Pause: false},
		},
		Analysis: &CanaryAnalysisConfig{
			Interval:             1 * time.Minute,
			ErrorRateThreshold:   0.02,
			LatencyP99Threshold:  500 * time.Millisecond,
			LatencyP95Threshold:  200 * time.Millisecond,
			SuccessRateThreshold: 0.98,
			MinSampleSize:        200,
			FailureThreshold:     2,
		},
		AutoPromote:    true,
		PromoteTimeout: 2 * time.Hour,
		AbortOnFailure: true,
		CanaryReplicas: 2,
	}

	if len(config.Steps) != 4 {
		t.Errorf("expected 4 steps, got %d", len(config.Steps))
	}

	// Verify steps progression
	for i := 1; i < len(config.Steps); i++ {
		if config.Steps[i].Weight <= config.Steps[i-1].Weight {
			t.Errorf("expected weight to increase: step %d has weight %d, step %d has weight %d",
				i-1, config.Steps[i-1].Weight, i, config.Steps[i].Weight)
		}
	}

	if config.Steps[len(config.Steps)-1].Weight != 100 {
		t.Errorf("expected final step to have weight 100, got %d", config.Steps[len(config.Steps)-1].Weight)
	}

	if config.Analysis.ErrorRateThreshold != 0.02 {
		t.Errorf("expected ErrorRateThreshold 0.02, got %f", config.Analysis.ErrorRateThreshold)
	}

	if !config.AutoPromote {
		t.Error("expected AutoPromote to be true")
	}

	if !config.AbortOnFailure {
		t.Error("expected AbortOnFailure to be true")
	}
}

// TestCanaryAnalysisConfig verifies canary analysis configuration.
func TestCanaryAnalysisConfig(t *testing.T) {
	config := &CanaryAnalysisConfig{
		Interval:             30 * time.Second,
		ErrorRateThreshold:   0.01,
		LatencyP99Threshold:  1 * time.Second,
		LatencyP95Threshold:  500 * time.Millisecond,
		SuccessRateThreshold: 0.99,
		MinSampleSize:        100,
		FailureThreshold:     3,
		Queries: []AnalysisQuery{
			{
				Name:      "cpu_usage",
				Query:     "avg(cpu_usage{service=\"test\"})",
				Operator:  "<",
				Threshold: 80.0,
				Weight:    0.2,
			},
			{
				Name:      "memory_usage",
				Query:     "avg(memory_usage{service=\"test\"})",
				Operator:  "<",
				Threshold: 90.0,
				Weight:    0.1,
			},
		},
	}

	if config.Interval != 30*time.Second {
		t.Errorf("expected Interval 30s, got %v", config.Interval)
	}

	if len(config.Queries) != 2 {
		t.Errorf("expected 2 queries, got %d", len(config.Queries))
	}

	if config.Queries[0].Operator != "<" {
		t.Errorf("expected operator '<', got %q", config.Queries[0].Operator)
	}
}

// TestAnalysisQuery verifies analysis query configuration.
func TestAnalysisQuery(t *testing.T) {
	query := AnalysisQuery{
		Name:      "test_metric",
		Query:     "sum(rate(requests_total[5m]))",
		Operator:  ">=",
		Threshold: 1000.0,
		Weight:    0.5,
	}

	if query.Name != "test_metric" {
		t.Errorf("expected name 'test_metric', got %q", query.Name)
	}

	if query.Threshold != 1000.0 {
		t.Errorf("expected threshold 1000.0, got %f", query.Threshold)
	}

	if query.Weight != 0.5 {
		t.Errorf("expected weight 0.5, got %f", query.Weight)
	}
}

// TestHealthGateConfig verifies health gate configuration.
func TestHealthGateConfig(t *testing.T) {
	config := &HealthGateConfig{
		Enabled:          true,
		InitialDelay:     15 * time.Second,
		Timeout:          10 * time.Minute,
		CheckInterval:    10 * time.Second,
		SuccessThreshold: 5,
		FailureThreshold: 2,
		Endpoints: []HealthEndpoint{
			{
				Name:           "health",
				URL:            "http://localhost:8080/health",
				Method:         "GET",
				ExpectedStatus: 200,
				Timeout:        5 * time.Second,
				Headers:        map[string]string{"Authorization": "Bearer token"},
			},
		},
		CustomChecks: []CustomHealthCheck{
			{
				Name:    "db_connection",
				Type:    "tcp",
				Address: "localhost:5432",
				Timeout: 3 * time.Second,
			},
		},
	}

	if !config.Enabled {
		t.Error("expected health gate to be enabled")
	}

	if config.SuccessThreshold != 5 {
		t.Errorf("expected SuccessThreshold 5, got %d", config.SuccessThreshold)
	}

	if len(config.Endpoints) != 1 {
		t.Errorf("expected 1 endpoint, got %d", len(config.Endpoints))
	}

	if config.Endpoints[0].Name != "health" {
		t.Errorf("expected endpoint name 'health', got %q", config.Endpoints[0].Name)
	}

	if len(config.CustomChecks) != 1 {
		t.Errorf("expected 1 custom check, got %d", len(config.CustomChecks))
	}

	if config.CustomChecks[0].Type != "tcp" {
		t.Errorf("expected custom check type 'tcp', got %q", config.CustomChecks[0].Type)
	}
}

// TestHealthEndpoint verifies health endpoint configuration.
func TestHealthEndpoint(t *testing.T) {
	endpoint := HealthEndpoint{
		Name:           "readiness",
		URL:            "http://service:8080/ready",
		Method:         "POST",
		ExpectedStatus: 204,
		ExpectedBody:   "OK",
		Timeout:        10 * time.Second,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"X-Custom":     "value",
		},
	}

	if endpoint.Method != "POST" {
		t.Errorf("expected method 'POST', got %q", endpoint.Method)
	}

	if endpoint.ExpectedStatus != 204 {
		t.Errorf("expected status 204, got %d", endpoint.ExpectedStatus)
	}

	if len(endpoint.Headers) != 2 {
		t.Errorf("expected 2 headers, got %d", len(endpoint.Headers))
	}
}

// TestCustomHealthCheck verifies custom health check configuration.
func TestCustomHealthCheck(t *testing.T) {
	tests := []struct {
		name  string
		check CustomHealthCheck
	}{
		{
			name: "exec check",
			check: CustomHealthCheck{
				Name:    "script_check",
				Type:    "exec",
				Command: "/opt/scripts/health.sh",
				Timeout: 30 * time.Second,
			},
		},
		{
			name: "tcp check",
			check: CustomHealthCheck{
				Name:    "db_check",
				Type:    "tcp",
				Address: "db.example.com:3306",
				Timeout: 5 * time.Second,
			},
		},
		{
			name: "grpc check",
			check: CustomHealthCheck{
				Name:    "grpc_check",
				Type:    "grpc",
				Address: "grpc.example.com:9090",
				Timeout: 5 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.check.Name == "" {
				t.Error("expected non-empty name")
			}
			if tt.check.Type == "" {
				t.Error("expected non-empty type")
			}
		})
	}
}

// TestNotificationConfig verifies notification configuration.
func TestNotificationConfig(t *testing.T) {
	config := &NotificationConfig{
		Enabled:    true,
		OnStart:    true,
		OnSuccess:  true,
		OnFailure:  true,
		OnRollback: true,
		OnPause:    true,
		Channels: []NotificationChannel{
			{
				Type:     "webhook",
				Target:   "https://hooks.example.com/deploy",
				Template: "Deployment {{.State}} for {{.Service}}",
				Severity: []string{"warning", "critical"},
			},
			{
				Type:   "slack",
				Target: "#deployments",
			},
		},
	}

	if !config.Enabled {
		t.Error("expected notifications to be enabled")
	}

	if !config.OnStart {
		t.Error("expected OnStart to be true")
	}

	if !config.OnFailure {
		t.Error("expected OnFailure to be true")
	}

	if len(config.Channels) != 2 {
		t.Errorf("expected 2 channels, got %d", len(config.Channels))
	}

	if config.Channels[0].Type != "webhook" {
		t.Errorf("expected channel type 'webhook', got %q", config.Channels[0].Type)
	}
}

// TestNotificationChannel verifies notification channel configuration.
func TestNotificationChannel(t *testing.T) {
	channel := NotificationChannel{
		Type:     "wotan",
		Target:   "deployment.events",
		Template: "",
		Severity: []string{"info", "warning", "critical"},
	}

	if channel.Type != "wotan" {
		t.Errorf("expected type 'wotan', got %q", channel.Type)
	}

	if channel.Target != "deployment.events" {
		t.Errorf("expected target 'deployment.events', got %q", channel.Target)
	}

	if len(channel.Severity) != 3 {
		t.Errorf("expected 3 severity levels, got %d", len(channel.Severity))
	}
}

// TestStrategyProgress verifies progress tracking.
func TestStrategyProgress(t *testing.T) {
	progress := &StrategyProgress{
		Phase:              "deploy",
		Message:            "Deploying instances",
		Percentage:         45,
		InstancesUpdated:   3,
		InstancesTotal:     10,
		TrafficWeight:      25,
		HealthyInstances:   7,
		UnhealthyInstances: 0,
	}

	if progress.Phase != "deploy" {
		t.Errorf("expected phase 'deploy', got %q", progress.Phase)
	}

	if progress.Percentage != 45 {
		t.Errorf("expected percentage 45, got %d", progress.Percentage)
	}

	if progress.InstancesUpdated != 3 {
		t.Errorf("expected instances updated 3, got %d", progress.InstancesUpdated)
	}

	if progress.TrafficWeight != 25 {
		t.Errorf("expected traffic weight 25, got %d", progress.TrafficWeight)
	}
}

// TestAnalysisResult verifies analysis result structure.
func TestAnalysisResult(t *testing.T) {
	result := &AnalysisResult{
		Timestamp: time.Now(),
		Pass:      true,
		Score:     95.5,
		Metrics: &AnalysisMetrics{
			ErrorRate:    0.01,
			SuccessRate:  0.99,
			LatencyP50:   5 * time.Millisecond,
			LatencyP95:   20 * time.Millisecond,
			LatencyP99:   50 * time.Millisecond,
			RequestCount: 10000,
			ErrorCount:   100,
			SampleSize:   10000,
		},
		Message: "Analysis passed",
		QueryResults: map[string]float64{
			"cpu_usage":    45.2,
			"memory_usage": 60.5,
		},
	}

	if !result.Pass {
		t.Error("expected pass to be true")
	}

	if result.Score != 95.5 {
		t.Errorf("expected score 95.5, got %f", result.Score)
	}

	if result.Metrics.ErrorRate != 0.01 {
		t.Errorf("expected error rate 0.01, got %f", result.Metrics.ErrorRate)
	}

	if len(result.QueryResults) != 2 {
		t.Errorf("expected 2 query results, got %d", len(result.QueryResults))
	}
}

// TestAnalysisMetrics verifies analysis metrics structure.
func TestAnalysisMetrics(t *testing.T) {
	metrics := &AnalysisMetrics{
		ErrorRate:    0.02,
		SuccessRate:  0.98,
		LatencyP50:   10 * time.Millisecond,
		LatencyP95:   50 * time.Millisecond,
		LatencyP99:   100 * time.Millisecond,
		RequestCount: 50000,
		ErrorCount:   1000,
		SampleSize:   50000,
	}

	if metrics.ErrorRate != 0.02 {
		t.Errorf("expected error rate 0.02, got %f", metrics.ErrorRate)
	}

	if metrics.SuccessRate != 0.98 {
		t.Errorf("expected success rate 0.98, got %f", metrics.SuccessRate)
	}

	if metrics.LatencyP99 != 100*time.Millisecond {
		t.Errorf("expected p99 latency 100ms, got %v", metrics.LatencyP99)
	}

	if metrics.RequestCount != 50000 {
		t.Errorf("expected request count 50000, got %d", metrics.RequestCount)
	}
}

// TestStrategyEvent verifies strategy event structure.
func TestStrategyEvent(t *testing.T) {
	event := &StrategyEvent{
		Type:         "traffic_shifted",
		ExecutionID:  "exec-123",
		DeploymentID: "deploy-456",
		State:        ExecutionStateRunning,
		Progress: &StrategyProgress{
			Phase:         "canary_step_2",
			Percentage:    50,
			TrafficWeight: 25,
		},
		Message:   "Traffic shifted to 25%",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"step":   2,
			"weight": 25,
		},
	}

	if event.Type != "traffic_shifted" {
		t.Errorf("expected type 'traffic_shifted', got %q", event.Type)
	}

	if event.State != ExecutionStateRunning {
		t.Errorf("expected state 'running', got %q", event.State)
	}

	if event.Progress.TrafficWeight != 25 {
		t.Errorf("expected traffic weight 25, got %d", event.Progress.TrafficWeight)
	}

	if event.Data["weight"] != 25 {
		t.Errorf("expected weight 25 in data, got %v", event.Data["weight"])
	}
}

// TestInstanceInfo verifies instance info structure.
func TestInstanceInfo(t *testing.T) {
	info := InstanceInfo{
		ID:      "instance-abc123",
		Service: "api-service",
		Version: "v1.2.3",
		State:   "running",
		Healthy: true,
		Address: "10.0.0.10:8080",
		Labels: map[string]string{
			"deployment": "deploy-001",
			"role":       "canary",
		},
	}

	if info.ID != "instance-abc123" {
		t.Errorf("expected ID 'instance-abc123', got %q", info.ID)
	}

	if info.Service != "api-service" {
		t.Errorf("expected service 'api-service', got %q", info.Service)
	}

	if !info.Healthy {
		t.Error("expected healthy to be true")
	}

	if info.Labels["role"] != "canary" {
		t.Errorf("expected role label 'canary', got %q", info.Labels["role"])
	}
}

// TestHealthResult verifies health result structure.
func TestHealthResult(t *testing.T) {
	result := &HealthResult{
		Healthy:      true,
		HealthyCount: 3,
		TotalCount:   3,
		CheckResults: []HealthCheckResult{
			{Name: "http", Healthy: true, StatusCode: 200, Latency: 10 * time.Millisecond},
			{Name: "tcp", Healthy: true, Latency: 2 * time.Millisecond},
			{Name: "grpc", Healthy: true, Latency: 5 * time.Millisecond},
		},
	}

	if !result.Healthy {
		t.Error("expected healthy to be true")
	}

	if result.HealthyCount != 3 {
		t.Errorf("expected healthy count 3, got %d", result.HealthyCount)
	}

	if len(result.CheckResults) != 3 {
		t.Errorf("expected 3 check results, got %d", len(result.CheckResults))
	}

	for _, check := range result.CheckResults {
		if !check.Healthy {
			t.Errorf("expected check %q to be healthy", check.Name)
		}
	}
}

// TestHealthCheckResult verifies health check result structure.
func TestHealthCheckResult(t *testing.T) {
	result := HealthCheckResult{
		Name:       "api_health",
		Healthy:    false,
		StatusCode: 503,
		Latency:    5 * time.Second,
		Error:      "service unavailable",
	}

	if result.Healthy {
		t.Error("expected healthy to be false")
	}

	if result.StatusCode != 503 {
		t.Errorf("expected status code 503, got %d", result.StatusCode)
	}

	if result.Error != "service unavailable" {
		t.Errorf("expected error 'service unavailable', got %q", result.Error)
	}
}

// TestExecutionParams verifies execution parameters.
func TestExecutionParams(t *testing.T) {
	params := &ExecutionParams{
		ServiceName:    "my-service",
		CurrentVersion: "v1.0.0",
		TargetVersion:  "v2.0.0",
		Replicas:       5,
		ArtifactRef:    "artifacts/my-service:v2.0.0",
	}

	if params.ServiceName != "my-service" {
		t.Errorf("expected service name 'my-service', got %q", params.ServiceName)
	}

	if params.CurrentVersion != "v1.0.0" {
		t.Errorf("expected current version 'v1.0.0', got %q", params.CurrentVersion)
	}

	if params.TargetVersion != "v2.0.0" {
		t.Errorf("expected target version 'v2.0.0', got %q", params.TargetVersion)
	}

	if params.Replicas != 5 {
		t.Errorf("expected replicas 5, got %d", params.Replicas)
	}
}

// TestGenerateExecutionID verifies execution ID generation.
func TestGenerateExecutionID(t *testing.T) {
	id1 := generateExecutionID()
	time.Sleep(time.Nanosecond * 100)
	id2 := generateExecutionID()

	if id1 == "" {
		t.Error("expected non-empty execution ID")
	}

	if id1 == id2 {
		t.Errorf("expected unique execution IDs, got same: %q", id1)
	}

	// Should have prefix
	if len(id1) < 5 || id1[:5] != "exec-" {
		t.Errorf("expected execution ID to start with 'exec-', got %q", id1)
	}
}

// TestConcurrentStrategyAccess verifies thread-safe access to strategy executor.
func TestConcurrentStrategyAccess(t *testing.T) {
	traffic := NewMockStrategyTrafficManager()
	health := &MockStrategyHealthChecker{healthy: true}
	metrics := &MockStrategyMetricsProvider{}
	notifier := &MockStrategyNotifier{}
	instances := NewMockStrategyInstanceManager()

	executor := NewStrategyExecutor(traffic, health, metrics, notifier, instances)

	var wg sync.WaitGroup
	errorsChan := make(chan error, 100)

	// Concurrent executions
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			config := &StrategyConfig{
				Type:    StrategyRolling,
				Timeout: 10 * time.Second,
				Rolling: &RollingConfig{
					MaxSurge:       1,
					MaxUnavailable: 0,
				},
			}

			params := &ExecutionParams{
				ServiceName:    "service-" + string(rune('a'+id)),
				CurrentVersion: "v1.0.0",
				TargetVersion:  "v2.0.0",
				Replicas:       3,
			}

			execution, err := executor.Execute(context.Background(), "deploy-"+string(rune('a'+id)), config, params)
			if err != nil {
				errorsChan <- err
				return
			}

			if execution == nil {
				errorsChan <- errors.New("nil execution returned")
				return
			}
		}(i)
	}

	wg.Wait()
	close(errorsChan)

	for err := range errorsChan {
		t.Errorf("concurrent access error: %v", err)
	}
}

// BenchmarkDefaultStrategyConfig benchmarks default strategy config creation.
func BenchmarkDefaultStrategyConfig(b *testing.B) {
	for i := 0; i < b.N; i++ {
		DefaultStrategyConfig(StrategyCanary)
	}
}

// BenchmarkGenerateExecutionID benchmarks execution ID generation.
func BenchmarkGenerateExecutionID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		generateExecutionID()
	}
}
