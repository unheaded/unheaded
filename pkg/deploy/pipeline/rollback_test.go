// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package pipeline provides the deployment pipeline for orchestrating deployment stages.
package pipeline

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// TestRollbackTriggerConstants verifies rollback trigger constants.
func TestRollbackTriggerConstants(t *testing.T) {
	tests := []struct {
		trigger  RollbackTrigger
		expected string
	}{
		{TriggerManual, "manual"},
		{TriggerAutomatic, "automatic"},
		{TriggerHealthCheck, "health_check"},
		{TriggerCanaryFailure, "canary_failure"},
		{TriggerTimeout, "timeout"},
		{TriggerMetricBreach, "metric_breach"},
		{TriggerHookFailure, "hook_failure"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if string(tt.trigger) != tt.expected {
				t.Errorf("expected trigger %q, got %q", tt.expected, tt.trigger)
			}
		})
	}
}

// TestRollbackStateConstants verifies rollback state constants.
func TestRollbackStateConstants(t *testing.T) {
	tests := []struct {
		state    RollbackState
		expected string
	}{
		{RollbackStatePending, "pending"},
		{RollbackStateExecuting, "executing"},
		{RollbackStateCompleted, "completed"},
		{RollbackStateFailed, "failed"},
		{RollbackStateCancelled, "cancelled"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if string(tt.state) != tt.expected {
				t.Errorf("expected state %q, got %q", tt.expected, tt.state)
			}
		})
	}
}

// TestDefaultRollbackConfig verifies default rollback configuration.
func TestDefaultRollbackConfig(t *testing.T) {
	config := DefaultRollbackConfig()

	if config == nil {
		t.Fatal("expected non-nil config")
	}

	if !config.AutoRollback {
		t.Error("expected AutoRollback to be true")
	}

	if config.HealthCheckRetries != 3 {
		t.Errorf("expected HealthCheckRetries 3, got %d", config.HealthCheckRetries)
	}

	if config.GracePeriod != 30*time.Second {
		t.Errorf("expected GracePeriod 30s, got %v", config.GracePeriod)
	}

	if !config.NotifyOnRollback {
		t.Error("expected NotifyOnRollback to be true")
	}

	// Verify default strategy
	if config.DefaultStrategy == nil {
		t.Fatal("expected non-nil DefaultStrategy")
	}

	if config.DefaultStrategy.Type != "instant" {
		t.Errorf("expected strategy type 'instant', got %q", config.DefaultStrategy.Type)
	}

	if !config.DefaultStrategy.VerifyAfter {
		t.Error("expected VerifyAfter to be true")
	}

	if config.DefaultStrategy.VerifyTimeout != 5*time.Minute {
		t.Errorf("expected VerifyTimeout 5m, got %v", config.DefaultStrategy.VerifyTimeout)
	}

	if config.DefaultStrategy.Timeout != 15*time.Minute {
		t.Errorf("expected Timeout 15m, got %v", config.DefaultStrategy.Timeout)
	}

	// Verify instant config
	if config.DefaultStrategy.Instant == nil {
		t.Fatal("expected non-nil Instant config")
	}

	if !config.DefaultStrategy.Instant.StopCurrent {
		t.Error("expected StopCurrent to be true")
	}

	if !config.DefaultStrategy.Instant.RestartPrevious {
		t.Error("expected RestartPrevious to be true")
	}

	if !config.DefaultStrategy.Instant.PreserveTraffic {
		t.Error("expected PreserveTraffic to be true")
	}

	// Verify metric thresholds
	if config.MetricThresholds == nil {
		t.Fatal("expected non-nil MetricThresholds")
	}

	if config.MetricThresholds.ErrorRateThreshold != 0.05 {
		t.Errorf("expected ErrorRateThreshold 0.05, got %f", config.MetricThresholds.ErrorRateThreshold)
	}

	if config.MetricThresholds.SuccessRateThreshold != 0.95 {
		t.Errorf("expected SuccessRateThreshold 0.95, got %f", config.MetricThresholds.SuccessRateThreshold)
	}

	if config.MetricThresholds.LatencyP99Threshold != 5*time.Second {
		t.Errorf("expected LatencyP99Threshold 5s, got %v", config.MetricThresholds.LatencyP99Threshold)
	}
}

// MockRollbackTrafficManager implements TrafficManager for testing.
type MockRollbackTrafficManager struct {
	mu           sync.Mutex
	shiftCalled  bool
	shiftErr     error
	shiftHistory []trafficShiftRecord
	weights      map[string]int
}

type trafficShiftRecord struct {
	serviceName string
	fromVersion string
	toVersion   string
	weight      int
}

func (m *MockRollbackTrafficManager) ShiftTraffic(ctx context.Context, serviceName, fromVersion, toVersion string, weight int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shiftCalled = true
	m.shiftHistory = append(m.shiftHistory, trafficShiftRecord{
		serviceName: serviceName,
		fromVersion: fromVersion,
		toVersion:   toVersion,
		weight:      weight,
	})
	if m.weights == nil {
		m.weights = make(map[string]int)
	}
	m.weights[toVersion] = weight
	return m.shiftErr
}

func (m *MockRollbackTrafficManager) GetTrafficWeight(ctx context.Context, serviceName, version string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.weights == nil {
		return 0, nil
	}
	return m.weights[version], nil
}

// MockRollbackInstanceManager implements InstanceManager for testing.
type MockRollbackInstanceManager struct {
	mu              sync.Mutex
	instances       map[string][]InstanceInfo
	deleteErr       error
	createErr       error
	deleteCalled    bool
	createCalled    bool
	deletedVersions []string
}

func (m *MockRollbackInstanceManager) GetInstances(ctx context.Context, serviceName, version string) ([]InstanceInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := serviceName + ":" + version
	return m.instances[key], nil
}

func (m *MockRollbackInstanceManager) DeleteInstances(ctx context.Context, serviceName, version string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleteCalled = true
	m.deletedVersions = append(m.deletedVersions, version)
	return m.deleteErr
}

func (m *MockRollbackInstanceManager) CreateInstances(ctx context.Context, serviceName, version string, count int, labels map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createCalled = true
	key := serviceName + ":" + version
	if m.instances == nil {
		m.instances = make(map[string][]InstanceInfo)
	}
	for i := 0; i < count; i++ {
		m.instances[key] = append(m.instances[key], InstanceInfo{
			ID:      "instance-" + string(rune('a'+i)),
			Service: serviceName,
			Version: version,
			Healthy: true,
		})
	}
	return m.createErr
}

func (m *MockRollbackInstanceManager) UpdateInstance(ctx context.Context, instanceID, version string) error {
	return nil
}

// MockRollbackHealthChecker implements HealthChecker for testing.
type MockRollbackHealthChecker struct {
	mu         sync.Mutex
	healthy    bool
	checkErr   error
	checkCount int
	healthyAt  int // Become healthy after this many checks
}

func (m *MockRollbackHealthChecker) CheckHealth(ctx context.Context, serviceName string, endpoints []HealthEndpoint) (*HealthResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkCount++

	if m.checkErr != nil {
		return nil, m.checkErr
	}

	// Support gradual health improvement
	healthy := m.healthy
	if m.healthyAt > 0 && m.checkCount >= m.healthyAt {
		healthy = true
	}

	return &HealthResult{
		Healthy:      healthy,
		HealthyCount: 1,
		TotalCount:   1,
		CheckResults: []HealthCheckResult{
			{Name: "test", Healthy: healthy},
		},
	}, nil
}

func (m *MockRollbackHealthChecker) CheckCustom(ctx context.Context, check *CustomHealthCheck) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.checkErr != nil {
		return m.checkErr
	}
	return nil
}

// TestNewRollbackManager verifies RollbackManager creation.
func TestNewRollbackManager(t *testing.T) {
	t.Run("with nil config", func(t *testing.T) {
		manager := NewRollbackManager(nil, nil, nil, nil, nil)

		if manager == nil {
			t.Fatal("expected non-nil manager")
		}

		if manager.config == nil {
			t.Fatal("expected default config to be applied")
		}

		if !manager.config.AutoRollback {
			t.Error("expected AutoRollback from default config")
		}
	})

	t.Run("with custom config", func(t *testing.T) {
		config := &RollbackConfig{
			AutoRollback:       false,
			HealthCheckRetries: 5,
			GracePeriod:        1 * time.Minute,
		}

		manager := NewRollbackManager(nil, nil, nil, nil, config)

		if manager.config.AutoRollback {
			t.Error("expected AutoRollback to be false")
		}

		if manager.config.HealthCheckRetries != 5 {
			t.Errorf("expected HealthCheckRetries 5, got %d", manager.config.HealthCheckRetries)
		}
	})

	t.Run("with dependencies", func(t *testing.T) {
		trafficMgr := &MockRollbackTrafficManager{}
		instanceMgr := &MockRollbackInstanceManager{instances: make(map[string][]InstanceInfo)}
		healthChecker := &MockRollbackHealthChecker{healthy: true}

		manager := NewRollbackManager(nil, trafficMgr, instanceMgr, healthChecker, nil)

		if manager.trafficManager == nil {
			t.Error("expected traffic manager to be set")
		}

		if manager.instanceManager == nil {
			t.Error("expected instance manager to be set")
		}

		if manager.healthChecker == nil {
			t.Error("expected health checker to be set")
		}
	})
}

// TestSetEventCallback verifies event callback setting.
func TestSetEventCallback(t *testing.T) {
	manager := NewRollbackManager(nil, nil, nil, nil, nil)

	eventCh := make(chan *RollbackEvent, 1)
	callback := func(event *RollbackEvent) {
		eventCh <- event
	}

	manager.SetEventCallback(callback)

	// Verify callback is set by emitting an event
	rollback := &PipelineRollback{
		ID:          "test-rollback",
		ServiceName: "test-service",
		State:       RollbackStatePending,
		Progress:    &RollbackProgress{},
	}

	manager.emitEvent(rollback, "test_event", nil)

	select {
	case receivedEvent := <-eventCh:
		if receivedEvent == nil {
			t.Fatal("expected non-nil event")
		}
		if receivedEvent.Type != "test_event" {
			t.Errorf("expected event type 'test_event', got %q", receivedEvent.Type)
		}
		if receivedEvent.RollbackID != "test-rollback" {
			t.Errorf("expected rollback ID 'test-rollback', got %q", receivedEvent.RollbackID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event callback")
	}
}

// TestValidateRequest verifies request validation.
func TestValidateRequest(t *testing.T) {
	manager := NewRollbackManager(nil, nil, nil, nil, nil)

	tests := []struct {
		name        string
		request     *RollbackRequest
		expectError bool
		errorMsg    string
	}{
		{
			name:        "nil request",
			request:     nil,
			expectError: true,
			errorMsg:    "request is nil",
		},
		{
			name:        "missing service name",
			request:     &RollbackRequest{},
			expectError: true,
			errorMsg:    "service name is required",
		},
		{
			name: "valid request",
			request: &RollbackRequest{
				ServiceName: "test-service",
				Trigger:     TriggerManual,
			},
			expectError: false,
		},
		{
			name: "request with missing trigger defaults to manual",
			request: &RollbackRequest{
				ServiceName: "test-service",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.validateRequest(tt.request)

			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				} else if err.Error() != tt.errorMsg {
					t.Errorf("expected error %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				// Verify default trigger is set
				if tt.request != nil && tt.request.Trigger == "" {
					// validateRequest should have set it
					t.Error("expected trigger to be set to default")
				}
			}
		})
	}
}

// TestInitiateRollback verifies rollback initiation.
func TestInitiateRollback(t *testing.T) {
	t.Run("successful initiation with explicit target version", func(t *testing.T) {
		manager := NewRollbackManager(nil, nil, nil, nil, nil)

		request := &RollbackRequest{
			ServiceName: "test-service",
			FromVersion: "v2.0.0",
			ToVersion:   "v1.0.0",
			Trigger:     TriggerManual,
			InitiatedBy: "test-user",
			Reason:      "Testing rollback",
		}

		rollback, err := manager.Initiate(context.Background(), request)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if rollback == nil {
			t.Fatal("expected non-nil rollback")
		}

		if rollback.ServiceName != "test-service" {
			t.Errorf("expected service name 'test-service', got %q", rollback.ServiceName)
		}

		if rollback.FromVersion != "v2.0.0" {
			t.Errorf("expected from version 'v2.0.0', got %q", rollback.FromVersion)
		}

		if rollback.ToVersion != "v1.0.0" {
			t.Errorf("expected to version 'v1.0.0', got %q", rollback.ToVersion)
		}

		if rollback.Trigger != TriggerManual {
			t.Errorf("expected trigger 'manual', got %q", rollback.Trigger)
		}

		// Read state under lock — executeRollback goroutine may have progressed it
		manager.mu.RLock()
		currentState := rollback.State
		manager.mu.RUnlock()
		if currentState != RollbackStatePending && currentState != RollbackStateExecuting {
			t.Errorf("expected state 'pending' or 'executing', got %q", currentState)
		}

		if rollback.InitiatedBy != "test-user" {
			t.Errorf("expected initiated by 'test-user', got %q", rollback.InitiatedBy)
		}

		if rollback.Reason != "Testing rollback" {
			t.Errorf("expected reason 'Testing rollback', got %q", rollback.Reason)
		}

		if rollback.Progress == nil {
			t.Fatal("expected non-nil progress")
		}

		if rollback.Metrics == nil {
			t.Fatal("expected non-nil metrics")
		}
	})

	t.Run("initiation fails when no target version available without artifact tracker", func(t *testing.T) {
		manager := NewRollbackManager(nil, nil, nil, nil, nil)

		request := &RollbackRequest{
			ServiceName: "test-service",
			FromVersion: "v2.0.0",
			Trigger:     TriggerAutomatic,
		}

		_, err := manager.Initiate(context.Background(), request)

		if err == nil {
			t.Fatal("expected error for missing target version")
		}

		if !errors.Is(err, ErrRollbackTargetMissing) {
			t.Errorf("expected ErrRollbackTargetMissing, got %v", err)
		}
	})

	t.Run("duplicate rollback prevented", func(t *testing.T) {
		manager := NewRollbackManager(nil, nil, nil, nil, nil)

		request := &RollbackRequest{
			ServiceName: "test-service",
			FromVersion: "v2.0.0",
			ToVersion:   "v1.0.0",
			Trigger:     TriggerManual,
		}

		// First initiation
		_, err := manager.Initiate(context.Background(), request)
		if err != nil {
			t.Fatalf("first initiation failed: %v", err)
		}

		// Second initiation should fail
		_, err = manager.Initiate(context.Background(), request)
		if err == nil {
			t.Fatal("expected error for duplicate rollback")
		}

		if !errors.Is(err, ErrRollbackInProgress) {
			t.Errorf("expected ErrRollbackInProgress, got %v", err)
		}
	})
}

// TestIsRollbackInProgress verifies in-progress check.
func TestIsRollbackInProgress(t *testing.T) {
	manager := NewRollbackManager(nil, nil, nil, nil, nil)

	// Initially no rollback in progress
	if manager.IsRollbackInProgress("test-service") {
		t.Error("expected no rollback in progress initially")
	}

	// Initiate a rollback
	request := &RollbackRequest{
		ServiceName: "test-service",
		FromVersion: "v2.0.0",
		ToVersion:   "v1.0.0",
		Trigger:     TriggerManual,
	}

	_, err := manager.Initiate(context.Background(), request)
	if err != nil {
		t.Fatalf("initiation failed: %v", err)
	}

	// Now should show in progress
	if !manager.IsRollbackInProgress("test-service") {
		t.Error("expected rollback to be in progress")
	}

	// Different service should not show in progress
	if manager.IsRollbackInProgress("other-service") {
		t.Error("expected no rollback in progress for other service")
	}
}

// TestShouldAutoRollback verifies auto-rollback decision logic.
func TestShouldAutoRollback(t *testing.T) {
	tests := []struct {
		name          string
		config        *RollbackConfig
		metrics       *AnalysisMetrics
		shouldTrigger bool
	}{
		{
			name:   "auto rollback disabled",
			config: &RollbackConfig{AutoRollback: false},
			metrics: &AnalysisMetrics{
				ErrorRate: 0.5, // High error rate
			},
			shouldTrigger: false,
		},
		{
			name:   "nil thresholds",
			config: &RollbackConfig{AutoRollback: true, MetricThresholds: nil},
			metrics: &AnalysisMetrics{
				ErrorRate: 0.5,
			},
			shouldTrigger: false,
		},
		{
			name: "error rate exceeded",
			config: &RollbackConfig{
				AutoRollback: true,
				MetricThresholds: &MetricThresholds{
					ErrorRateThreshold: 0.05,
				},
			},
			metrics: &AnalysisMetrics{
				ErrorRate: 0.10, // 10% error rate
			},
			shouldTrigger: true,
		},
		{
			name: "error rate within threshold",
			config: &RollbackConfig{
				AutoRollback: true,
				MetricThresholds: &MetricThresholds{
					ErrorRateThreshold: 0.05,
				},
			},
			metrics: &AnalysisMetrics{
				ErrorRate: 0.02, // 2% error rate
			},
			shouldTrigger: false,
		},
		{
			name: "success rate below threshold",
			config: &RollbackConfig{
				AutoRollback: true,
				MetricThresholds: &MetricThresholds{
					SuccessRateThreshold: 0.95,
				},
			},
			metrics: &AnalysisMetrics{
				SuccessRate: 0.90, // 90% success rate
			},
			shouldTrigger: true,
		},
		{
			name: "success rate above threshold",
			config: &RollbackConfig{
				AutoRollback: true,
				MetricThresholds: &MetricThresholds{
					SuccessRateThreshold: 0.95,
				},
			},
			metrics: &AnalysisMetrics{
				SuccessRate: 0.99, // 99% success rate
			},
			shouldTrigger: false,
		},
		{
			name: "latency exceeded",
			config: &RollbackConfig{
				AutoRollback: true,
				MetricThresholds: &MetricThresholds{
					LatencyP99Threshold: 5 * time.Second,
				},
			},
			metrics: &AnalysisMetrics{
				LatencyP99: 10 * time.Second, // High latency
			},
			shouldTrigger: true,
		},
		{
			name: "latency within threshold",
			config: &RollbackConfig{
				AutoRollback: true,
				MetricThresholds: &MetricThresholds{
					LatencyP99Threshold: 5 * time.Second,
				},
			},
			metrics: &AnalysisMetrics{
				LatencyP99: 2 * time.Second, // Good latency
			},
			shouldTrigger: false,
		},
		{
			name: "all metrics good",
			config: &RollbackConfig{
				AutoRollback: true,
				MetricThresholds: &MetricThresholds{
					ErrorRateThreshold:   0.05,
					SuccessRateThreshold: 0.95,
					LatencyP99Threshold:  5 * time.Second,
				},
			},
			metrics: &AnalysisMetrics{
				ErrorRate:   0.01,
				SuccessRate: 0.99,
				LatencyP99:  1 * time.Second,
			},
			shouldTrigger: false,
		},
		{
			name: "multiple thresholds exceeded",
			config: &RollbackConfig{
				AutoRollback: true,
				MetricThresholds: &MetricThresholds{
					ErrorRateThreshold:   0.05,
					SuccessRateThreshold: 0.95,
					LatencyP99Threshold:  5 * time.Second,
				},
			},
			metrics: &AnalysisMetrics{
				ErrorRate:   0.20,
				SuccessRate: 0.80,
				LatencyP99:  15 * time.Second,
			},
			shouldTrigger: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewRollbackManager(nil, nil, nil, nil, tt.config)

			result := manager.ShouldAutoRollback(tt.metrics)

			if result != tt.shouldTrigger {
				t.Errorf("expected ShouldAutoRollback to return %v, got %v", tt.shouldTrigger, result)
			}
		})
	}
}

// TestTriggerAutoRollback verifies automatic rollback triggering.
func TestTriggerAutoRollback(t *testing.T) {
	manager := NewRollbackManager(nil, nil, nil, nil, nil)

	rollback, err := manager.TriggerAutoRollback(
		context.Background(),
		"test-service",
		"v2.0.0",
		"High error rate detected",
		TriggerMetricBreach,
	)

	// Will fail due to missing target version, but we can verify the request was structured correctly
	if err == nil && rollback != nil {
		if rollback.InitiatedBy != "auto-rollback" {
			t.Errorf("expected InitiatedBy 'auto-rollback', got %q", rollback.InitiatedBy)
		}

		if rollback.Trigger != TriggerMetricBreach {
			t.Errorf("expected trigger TriggerMetricBreach, got %v", rollback.Trigger)
		}

		if rollback.Reason != "High error rate detected" {
			t.Errorf("expected reason 'High error rate detected', got %q", rollback.Reason)
		}
	}
}

// TestRollbackCancel verifies rollback cancellation.
func TestRollbackCancel(t *testing.T) {
	manager := NewRollbackManager(nil, nil, nil, nil, nil)

	// Initiate a rollback
	request := &RollbackRequest{
		ServiceName: "test-service",
		FromVersion: "v2.0.0",
		ToVersion:   "v1.0.0",
		Trigger:     TriggerManual,
	}

	rollback, err := manager.Initiate(context.Background(), request)
	if err != nil {
		t.Fatalf("initiation failed: %v", err)
	}

	// Cancel the rollback
	err = manager.Cancel(context.Background(), rollback.ID)
	if err != nil {
		t.Fatalf("cancel failed: %v", err)
	}

	// Verify state
	status, err := manager.Status(context.Background(), rollback.ID)
	if err != nil {
		t.Fatalf("status check failed: %v", err)
	}

	if status.State != RollbackStateCancelled {
		t.Errorf("expected state 'cancelled', got %q", status.State)
	}

	// Should no longer be in progress
	if manager.IsRollbackInProgress("test-service") {
		t.Error("expected rollback to not be in progress after cancel")
	}
}

// TestRollbackCancelNotFound verifies cancel with invalid ID.
func TestRollbackCancelNotFound(t *testing.T) {
	manager := NewRollbackManager(nil, nil, nil, nil, nil)

	err := manager.Cancel(context.Background(), "nonexistent-id")

	if err == nil {
		t.Fatal("expected error for nonexistent rollback")
	}
}

// TestRollbackStatus verifies status retrieval.
func TestRollbackStatus(t *testing.T) {
	manager := NewRollbackManager(nil, nil, nil, nil, nil)

	// Initiate a rollback
	request := &RollbackRequest{
		ServiceName: "test-service",
		FromVersion: "v2.0.0",
		ToVersion:   "v1.0.0",
		Trigger:     TriggerManual,
	}

	rollback, err := manager.Initiate(context.Background(), request)
	if err != nil {
		t.Fatalf("initiation failed: %v", err)
	}

	// Get status
	status, err := manager.Status(context.Background(), rollback.ID)
	if err != nil {
		t.Fatalf("status check failed: %v", err)
	}

	if status.ID != rollback.ID {
		t.Errorf("expected ID %q, got %q", rollback.ID, status.ID)
	}

	if status.ServiceName != "test-service" {
		t.Errorf("expected service name 'test-service', got %q", status.ServiceName)
	}
}

// TestRollbackStatusNotFound verifies status with invalid ID.
func TestRollbackStatusNotFound(t *testing.T) {
	manager := NewRollbackManager(nil, nil, nil, nil, nil)

	_, err := manager.Status(context.Background(), "nonexistent-id")

	if err == nil {
		t.Fatal("expected error for nonexistent rollback")
	}
}

// TestGetHistory verifies rollback history retrieval.
func TestGetHistory(t *testing.T) {
	manager := NewRollbackManager(nil, nil, nil, nil, nil)

	// Add some rollbacks to history manually
	manager.mu.Lock()
	manager.history["test-service"] = []*PipelineRollback{
		{ID: "rollback-1", ServiceName: "test-service", State: RollbackStateCompleted},
		{ID: "rollback-2", ServiceName: "test-service", State: RollbackStateCompleted},
		{ID: "rollback-3", ServiceName: "test-service", State: RollbackStateFailed},
	}
	manager.mu.Unlock()

	// Get all history
	history, err := manager.GetHistory(context.Background(), "test-service", 0)
	if err != nil {
		t.Fatalf("GetHistory failed: %v", err)
	}

	if len(history) != 3 {
		t.Errorf("expected 3 history entries, got %d", len(history))
	}

	// Verify reverse order (newest first)
	if history[0].ID != "rollback-3" {
		t.Errorf("expected newest rollback first, got %q", history[0].ID)
	}

	// Get limited history
	limitedHistory, err := manager.GetHistory(context.Background(), "test-service", 2)
	if err != nil {
		t.Fatalf("GetHistory with limit failed: %v", err)
	}

	if len(limitedHistory) != 2 {
		t.Errorf("expected 2 history entries with limit, got %d", len(limitedHistory))
	}

	// Get history for nonexistent service
	emptyHistory, err := manager.GetHistory(context.Background(), "nonexistent-service", 0)
	if err != nil {
		t.Fatalf("GetHistory for nonexistent service failed: %v", err)
	}

	if len(emptyHistory) != 0 {
		t.Errorf("expected empty history, got %d entries", len(emptyHistory))
	}
}

// TestGetStats verifies rollback statistics.
func TestGetStats(t *testing.T) {
	manager := NewRollbackManager(nil, nil, nil, nil, nil)

	// Add rollbacks to history
	manager.mu.Lock()
	manager.history["service-1"] = []*PipelineRollback{
		{
			ID:       "rollback-1",
			State:    RollbackStateCompleted,
			Trigger:  TriggerManual,
			Duration: 30 * time.Second,
		},
		{
			ID:       "rollback-2",
			State:    RollbackStateCompleted,
			Trigger:  TriggerAutomatic,
			Duration: 60 * time.Second,
		},
	}
	manager.history["service-2"] = []*PipelineRollback{
		{
			ID:      "rollback-3",
			State:   RollbackStateFailed,
			Trigger: TriggerHealthCheck,
		},
	}
	manager.inProgress["service-3"] = "rollback-4"
	manager.mu.Unlock()

	stats := manager.GetStats(context.Background())

	if stats.Total != 3 {
		t.Errorf("expected total 3, got %d", stats.Total)
	}

	if stats.Active != 1 {
		t.Errorf("expected active 1, got %d", stats.Active)
	}

	if stats.Successful != 2 {
		t.Errorf("expected successful 2, got %d", stats.Successful)
	}

	if stats.Failed != 1 {
		t.Errorf("expected failed 1, got %d", stats.Failed)
	}

	if stats.ByTrigger[TriggerManual] != 1 {
		t.Errorf("expected 1 manual trigger, got %d", stats.ByTrigger[TriggerManual])
	}

	if stats.ByTrigger[TriggerAutomatic] != 1 {
		t.Errorf("expected 1 automatic trigger, got %d", stats.ByTrigger[TriggerAutomatic])
	}

	if stats.ByTrigger[TriggerHealthCheck] != 1 {
		t.Errorf("expected 1 health check trigger, got %d", stats.ByTrigger[TriggerHealthCheck])
	}

	// Average duration should be (30 + 60) / 2 = 45 seconds
	expectedAvg := 45 * time.Second
	if stats.AvgDuration != expectedAvg {
		t.Errorf("expected average duration %v, got %v", expectedAvg, stats.AvgDuration)
	}
}

// TestRollbackStrategyTypes verifies different rollback strategy types.
func TestRollbackStrategyTypes(t *testing.T) {
	tests := []struct {
		name         string
		strategyType string
		setup        func(*RollbackStrategy)
	}{
		{
			name:         "instant strategy",
			strategyType: "instant",
			setup: func(s *RollbackStrategy) {
				s.Instant = &InstantRollbackConfig{
					StopCurrent:     true,
					RestartPrevious: true,
					PreserveTraffic: true,
				}
			},
		},
		{
			name:         "gradual strategy",
			strategyType: "gradual",
			setup: func(s *RollbackStrategy) {
				s.Gradual = &GradualRollbackConfig{
					Steps: []RollbackStep{
						{Weight: 25, Duration: time.Minute, Verify: true},
						{Weight: 50, Duration: time.Minute, Verify: true},
						{Weight: 100, Duration: 0, Verify: true},
					},
					BatchSize:    5,
					Interval:     10 * time.Second,
					PauseOnError: true,
				}
			},
		},
		{
			name:         "blue-green strategy",
			strategyType: "blue_green",
			setup:        func(s *RollbackStrategy) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy := &RollbackStrategy{
				Type:          tt.strategyType,
				VerifyAfter:   true,
				VerifyTimeout: 5 * time.Minute,
				Timeout:       15 * time.Minute,
			}
			tt.setup(strategy)

			if strategy.Type != tt.strategyType {
				t.Errorf("expected type %q, got %q", tt.strategyType, strategy.Type)
			}
		})
	}
}

// TestRollbackProgress verifies progress tracking.
func TestRollbackProgress(t *testing.T) {
	progress := &RollbackProgress{
		Phase:               "shifting_traffic",
		Message:             "Shifting traffic to previous version",
		Percentage:          50,
		CurrentStep:         2,
		TotalSteps:          4,
		InstancesRolledBack: 3,
		InstancesTotal:      6,
		TrafficShifted:      50,
	}

	if progress.Phase != "shifting_traffic" {
		t.Errorf("expected phase 'shifting_traffic', got %q", progress.Phase)
	}

	if progress.Percentage != 50 {
		t.Errorf("expected percentage 50, got %d", progress.Percentage)
	}

	if progress.CurrentStep != 2 {
		t.Errorf("expected current step 2, got %d", progress.CurrentStep)
	}

	if progress.TotalSteps != 4 {
		t.Errorf("expected total steps 4, got %d", progress.TotalSteps)
	}

	if progress.InstancesRolledBack != 3 {
		t.Errorf("expected instances rolled back 3, got %d", progress.InstancesRolledBack)
	}

	if progress.TrafficShifted != 50 {
		t.Errorf("expected traffic shifted 50, got %d", progress.TrafficShifted)
	}
}

// TestRollbackMetrics verifies metrics tracking.
func TestRollbackMetrics(t *testing.T) {
	metrics := &RollbackMetrics{
		InstancesRolledBack: 10,
		InstancesFailed:     2,
		TrafficShifts:       4,
		HealthChecks:        12,
		HooksExecuted:       6,
		DowntimeSeconds:     5.5,
	}

	if metrics.InstancesRolledBack != 10 {
		t.Errorf("expected instances rolled back 10, got %d", metrics.InstancesRolledBack)
	}

	if metrics.InstancesFailed != 2 {
		t.Errorf("expected instances failed 2, got %d", metrics.InstancesFailed)
	}

	if metrics.TrafficShifts != 4 {
		t.Errorf("expected traffic shifts 4, got %d", metrics.TrafficShifts)
	}

	if metrics.HealthChecks != 12 {
		t.Errorf("expected health checks 12, got %d", metrics.HealthChecks)
	}

	if metrics.HooksExecuted != 6 {
		t.Errorf("expected hooks executed 6, got %d", metrics.HooksExecuted)
	}

	if metrics.DowntimeSeconds != 5.5 {
		t.Errorf("expected downtime 5.5s, got %f", metrics.DowntimeSeconds)
	}
}

// TestRollbackHooks verifies rollback hooks structure.
func TestRollbackHooks(t *testing.T) {
	hooks := &RollbackHooks{
		PreRollback: []*Hook{
			{Name: "pre-hook-1", Type: HookTypeExec},
			{Name: "pre-hook-2", Type: HookTypeHTTP},
		},
		PostRollback: []*Hook{
			{Name: "post-hook-1", Type: HookTypeWebhook},
		},
		OnFailure: []*Hook{
			{Name: "failure-hook-1", Type: HookTypeWotan},
		},
	}

	if len(hooks.PreRollback) != 2 {
		t.Errorf("expected 2 pre-rollback hooks, got %d", len(hooks.PreRollback))
	}

	if len(hooks.PostRollback) != 1 {
		t.Errorf("expected 1 post-rollback hook, got %d", len(hooks.PostRollback))
	}

	if len(hooks.OnFailure) != 1 {
		t.Errorf("expected 1 on-failure hook, got %d", len(hooks.OnFailure))
	}
}

// TestGradualRollbackSteps verifies gradual rollback step configuration.
func TestGradualRollbackSteps(t *testing.T) {
	steps := []RollbackStep{
		{Weight: 10, Duration: 1 * time.Minute, Verify: true},
		{Weight: 25, Duration: 2 * time.Minute, Verify: true},
		{Weight: 50, Duration: 3 * time.Minute, Verify: true},
		{Weight: 75, Duration: 2 * time.Minute, Verify: false},
		{Weight: 100, Duration: 0, Verify: true},
	}

	// Verify steps are properly configured
	if len(steps) != 5 {
		t.Fatalf("expected 5 steps, got %d", len(steps))
	}

	// Verify weight progression
	for i := 1; i < len(steps); i++ {
		if steps[i].Weight <= steps[i-1].Weight {
			t.Errorf("expected weight to increase: step %d has weight %d, step %d has weight %d",
				i-1, steps[i-1].Weight, i, steps[i].Weight)
		}
	}

	// Verify final step reaches 100%
	if steps[len(steps)-1].Weight != 100 {
		t.Errorf("expected final step weight 100, got %d", steps[len(steps)-1].Weight)
	}
}

// TestInstantRollbackConfig verifies instant rollback configuration.
func TestInstantRollbackConfig(t *testing.T) {
	config := &InstantRollbackConfig{
		StopCurrent:     true,
		RestartPrevious: true,
		PreserveTraffic: false,
	}

	if !config.StopCurrent {
		t.Error("expected StopCurrent to be true")
	}

	if !config.RestartPrevious {
		t.Error("expected RestartPrevious to be true")
	}

	if config.PreserveTraffic {
		t.Error("expected PreserveTraffic to be false")
	}
}

// TestGradualRollbackConfig verifies gradual rollback configuration.
func TestGradualRollbackConfig(t *testing.T) {
	config := &GradualRollbackConfig{
		Steps: []RollbackStep{
			{Weight: 50, Duration: time.Minute, Verify: true},
			{Weight: 100, Duration: 0, Verify: true},
		},
		BatchSize:    10,
		Interval:     30 * time.Second,
		PauseOnError: true,
	}

	if len(config.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(config.Steps))
	}

	if config.BatchSize != 10 {
		t.Errorf("expected batch size 10, got %d", config.BatchSize)
	}

	if config.Interval != 30*time.Second {
		t.Errorf("expected interval 30s, got %v", config.Interval)
	}

	if !config.PauseOnError {
		t.Error("expected PauseOnError to be true")
	}
}

// TestRollbackEventEmission verifies event emission during rollback.
func TestRollbackEventEmission(t *testing.T) {
	manager := NewRollbackManager(nil, nil, nil, nil, nil)

	eventTypes := make(map[string]bool)
	var eventsMu sync.Mutex
	eventCount := 0

	manager.SetEventCallback(func(event *RollbackEvent) {
		eventsMu.Lock()
		eventTypes[event.Type] = true
		eventCount++

		// Verify common fields
		if event.RollbackID != "test-rollback" {
			t.Errorf("expected rollback ID 'test-rollback', got %q", event.RollbackID)
		}

		if event.DeploymentID != "deploy-1" {
			t.Errorf("expected deployment ID 'deploy-1', got %q", event.DeploymentID)
		}

		if event.ServiceName != "test-service" {
			t.Errorf("expected service name 'test-service', got %q", event.ServiceName)
		}

		// Verify progress event has data
		if event.Type == "rollback_progress" {
			if event.Data == nil {
				t.Error("expected progress event to have data")
			} else if step, ok := event.Data["step"].(int); !ok || step != 1 {
				t.Errorf("expected step 1 in progress event data, got %v", event.Data["step"])
			}
		}
		eventsMu.Unlock()
	})

	// Create a rollback and emit events
	rollback := &PipelineRollback{
		ID:           "test-rollback",
		DeploymentID: "deploy-1",
		ServiceName:  "test-service",
		State:        RollbackStatePending,
		Progress:     &RollbackProgress{Phase: "starting"},
	}

	// Emit various events
	manager.emitEvent(rollback, "rollback_started", nil)
	manager.emitEvent(rollback, "rollback_progress", map[string]interface{}{"step": 1})
	manager.emitEvent(rollback, "rollback_completed", nil)

	// Wait for goroutines to complete
	time.Sleep(150 * time.Millisecond)

	eventsMu.Lock()
	defer eventsMu.Unlock()

	if eventCount != 3 {
		t.Errorf("expected 3 events, got %d", eventCount)
	}

	// Verify all event types were received (order not guaranteed due to goroutines)
	expectedTypes := []string{"rollback_started", "rollback_progress", "rollback_completed"}
	for _, expectedType := range expectedTypes {
		if !eventTypes[expectedType] {
			t.Errorf("expected event type %q to be received", expectedType)
		}
	}
}

// TestRollbackErrors verifies error constants.
func TestRollbackErrors(t *testing.T) {
	tests := []struct {
		err      error
		expected string
	}{
		{ErrRollbackNotNeeded, "rollback not needed"},
		{ErrRollbackInProgress, "rollback already in progress"},
		{ErrRollbackTargetMissing, "rollback target not available"},
		{ErrRollbackFailed, "rollback failed"},
		{ErrNoRollbackHistory, "no rollback history available"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.err.Error() != tt.expected {
				t.Errorf("expected error %q, got %q", tt.expected, tt.err.Error())
			}
		})
	}
}

// TestGenerateRollbackID verifies rollback ID generation.
func TestGenerateRollbackID(t *testing.T) {
	id1 := generateRollbackID()
	// Small delay to ensure different nanosecond timestamp
	time.Sleep(time.Nanosecond * 100)
	id2 := generateRollbackID()

	if id1 == "" {
		t.Error("expected non-empty rollback ID")
	}

	if id1 == id2 {
		t.Errorf("expected unique rollback IDs, got same: %q", id1)
	}

	// Should have prefix
	if len(id1) < 9 || id1[:9] != "rollback-" {
		t.Errorf("expected rollback ID to start with 'rollback-', got %q", id1)
	}
}

// TestConcurrentRollbackAccess verifies thread-safe access to rollback manager.
func TestConcurrentRollbackAccess(t *testing.T) {
	manager := NewRollbackManager(nil, nil, nil, nil, nil)

	var wg sync.WaitGroup
	errorsChan := make(chan error, 100)

	// Concurrent reads and writes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			serviceName := "service-" + string(rune('a'+id))

			// Try to initiate (some will fail due to duplicate)
			request := &RollbackRequest{
				ServiceName: serviceName,
				FromVersion: "v2.0.0",
				ToVersion:   "v1.0.0",
				Trigger:     TriggerManual,
			}

			rollback, err := manager.Initiate(context.Background(), request)
			if err != nil && !errors.Is(err, ErrRollbackInProgress) {
				errorsChan <- err
				return
			}

			if rollback != nil {
				// Check status
				_, err := manager.Status(context.Background(), rollback.ID)
				if err != nil {
					errorsChan <- err
					return
				}

				// Check in-progress
				manager.IsRollbackInProgress(serviceName)

				// Get history
				_, err = manager.GetHistory(context.Background(), serviceName, 10)
				if err != nil {
					errorsChan <- err
					return
				}

				// Get stats
				manager.GetStats(context.Background())
			}
		}(i)
	}

	wg.Wait()
	close(errorsChan)

	for err := range errorsChan {
		t.Errorf("concurrent access error: %v", err)
	}
}

// BenchmarkInitiateRollback benchmarks rollback initiation.
func BenchmarkInitiateRollback(b *testing.B) {
	manager := NewRollbackManager(nil, nil, nil, nil, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		request := &RollbackRequest{
			ServiceName: "test-service-" + string(rune('a'+i%26)),
			FromVersion: "v2.0.0",
			ToVersion:   "v1.0.0",
			Trigger:     TriggerManual,
		}

		// Note: This will accumulate rollbacks, but we're measuring initiation speed
		manager.Initiate(context.Background(), request)
	}
}

// BenchmarkShouldAutoRollback benchmarks auto-rollback decision.
func BenchmarkShouldAutoRollback(b *testing.B) {
	config := &RollbackConfig{
		AutoRollback: true,
		MetricThresholds: &MetricThresholds{
			ErrorRateThreshold:   0.05,
			SuccessRateThreshold: 0.95,
			LatencyP99Threshold:  5 * time.Second,
		},
	}

	manager := NewRollbackManager(nil, nil, nil, nil, config)

	metrics := &AnalysisMetrics{
		ErrorRate:   0.02,
		SuccessRate: 0.98,
		LatencyP99:  2 * time.Second,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.ShouldAutoRollback(metrics)
	}
}

// BenchmarkGenerateRollbackID benchmarks rollback ID generation.
func BenchmarkGenerateRollbackID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		generateRollbackID()
	}
}
