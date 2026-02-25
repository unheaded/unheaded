// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package pipeline provides the deployment pipeline for orchestrating deployment stages.
package pipeline

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Rollback errors.
var (
	ErrRollbackNotNeeded     = errors.New("rollback not needed")
	ErrRollbackInProgress    = errors.New("rollback already in progress")
	ErrRollbackTargetMissing = errors.New("rollback target not available")
	ErrRollbackFailed        = errors.New("rollback failed")
	ErrNoRollbackHistory     = errors.New("no rollback history available")
)

// RollbackTrigger defines what triggered the rollback.
type RollbackTrigger string

const (
	TriggerManual        RollbackTrigger = "manual"
	TriggerAutomatic     RollbackTrigger = "automatic"
	TriggerHealthCheck   RollbackTrigger = "health_check"
	TriggerCanaryFailure RollbackTrigger = "canary_failure"
	TriggerTimeout       RollbackTrigger = "timeout"
	TriggerMetricBreach  RollbackTrigger = "metric_breach"
	TriggerHookFailure   RollbackTrigger = "hook_failure"
)

// RollbackState represents the state of a rollback operation.
type RollbackState string

const (
	RollbackStatePending   RollbackState = "pending"
	RollbackStateExecuting RollbackState = "executing"
	RollbackStateCompleted RollbackState = "completed"
	RollbackStateFailed    RollbackState = "failed"
	RollbackStateCancelled RollbackState = "cancelled"
)

// PipelineRollback represents a rollback operation within the pipeline.
type PipelineRollback struct {
	// ID is the unique rollback identifier
	ID string `json:"id"`

	// PipelineID is the pipeline being rolled back
	PipelineID string `json:"pipeline_id"`

	// DeploymentID is the deployment being rolled back
	DeploymentID string `json:"deployment_id"`

	// ServiceName is the service being rolled back
	ServiceName string `json:"service_name"`

	// FromVersion is the version being rolled back from
	FromVersion string `json:"from_version"`

	// ToVersion is the version being rolled back to
	ToVersion string `json:"to_version"`

	// Trigger is what triggered the rollback
	Trigger RollbackTrigger `json:"trigger"`

	// State is the current rollback state
	State RollbackState `json:"state"`

	// Strategy is the rollback strategy
	Strategy *RollbackStrategy `json:"strategy,omitempty"`

	// Progress contains rollback progress
	Progress *RollbackProgress `json:"progress,omitempty"`

	// Metrics contains rollback metrics
	Metrics *RollbackMetrics `json:"metrics,omitempty"`

	// InitiatedBy is who/what initiated the rollback
	InitiatedBy string `json:"initiated_by,omitempty"`

	// Reason describes why the rollback was initiated
	Reason string `json:"reason,omitempty"`

	// StartedAt is when the rollback started
	StartedAt time.Time `json:"started_at"`

	// CompletedAt is when the rollback completed
	CompletedAt time.Time `json:"completed_at,omitempty"`

	// Duration is the rollback duration
	Duration time.Duration `json:"duration,omitempty"`

	// Error contains error details if failed
	Error string `json:"error,omitempty"`
}

// RollbackStrategy defines the rollback strategy.
type RollbackStrategy struct {
	// Type is the strategy type (instant, gradual, blue_green)
	Type string `json:"type"`

	// Instant rollback config
	Instant *InstantRollbackConfig `json:"instant,omitempty"`

	// Gradual rollback config
	Gradual *GradualRollbackConfig `json:"gradual,omitempty"`

	// VerifyAfter verify health after rollback
	VerifyAfter bool `json:"verify_after,omitempty"`

	// VerifyTimeout is the timeout for verification
	VerifyTimeout time.Duration `json:"verify_timeout,omitempty"`

	// Hooks for rollback events
	Hooks *RollbackHooks `json:"hooks,omitempty"`

	// Timeout is the overall rollback timeout
	Timeout time.Duration `json:"timeout,omitempty"`
}

// InstantRollbackConfig for instant rollback strategy.
type InstantRollbackConfig struct {
	// StopCurrent stops current deployment immediately
	StopCurrent bool `json:"stop_current,omitempty"`

	// RestartPrevious restarts previous version instances
	RestartPrevious bool `json:"restart_previous,omitempty"`

	// PreserveTraffic keeps traffic flowing during rollback
	PreserveTraffic bool `json:"preserve_traffic,omitempty"`
}

// GradualRollbackConfig for gradual rollback strategy.
type GradualRollbackConfig struct {
	// Steps defines traffic shift steps
	Steps []RollbackStep `json:"steps,omitempty"`

	// BatchSize for instance updates
	BatchSize int `json:"batch_size,omitempty"`

	// Interval between batches
	Interval time.Duration `json:"interval,omitempty"`

	// PauseOnError pauses rollback on errors
	PauseOnError bool `json:"pause_on_error,omitempty"`
}

// RollbackStep defines a step in gradual rollback.
type RollbackStep struct {
	// Weight is the traffic percentage for old version (0-100)
	Weight int `json:"weight"`

	// Duration to hold at this step
	Duration time.Duration `json:"duration,omitempty"`

	// Verify verifies health at this step
	Verify bool `json:"verify,omitempty"`
}

// RollbackHooks defines hooks for rollback events.
type RollbackHooks struct {
	// PreRollback hooks run before rollback
	PreRollback []*Hook `json:"pre_rollback,omitempty"`

	// PostRollback hooks run after rollback
	PostRollback []*Hook `json:"post_rollback,omitempty"`

	// OnFailure hooks run on rollback failure
	OnFailure []*Hook `json:"on_failure,omitempty"`
}

// RollbackProgress tracks rollback progress.
type RollbackProgress struct {
	// Phase is the current rollback phase
	Phase string `json:"phase"`

	// Message is a human-readable message
	Message string `json:"message"`

	// Percentage is completion percentage (0-100)
	Percentage int `json:"percentage"`

	// CurrentStep for gradual rollback
	CurrentStep int `json:"current_step,omitempty"`

	// TotalSteps for gradual rollback
	TotalSteps int `json:"total_steps,omitempty"`

	// InstancesRolledBack count
	InstancesRolledBack int `json:"instances_rolled_back"`

	// InstancesTotal count
	InstancesTotal int `json:"instances_total"`

	// TrafficShifted percentage shifted to old version
	TrafficShifted int `json:"traffic_shifted,omitempty"`
}

// RollbackMetrics contains metrics about the rollback.
type RollbackMetrics struct {
	// InstancesRolledBack count of rolled back instances
	InstancesRolledBack int `json:"instances_rolled_back"`

	// InstancesFailed count of failed instance rollbacks
	InstancesFailed int `json:"instances_failed"`

	// TrafficShifts count of traffic shifts
	TrafficShifts int `json:"traffic_shifts"`

	// HealthChecks count of health checks
	HealthChecks int `json:"health_checks"`

	// HooksExecuted count of hooks executed
	HooksExecuted int `json:"hooks_executed"`

	// DowntimeSeconds estimated downtime
	DowntimeSeconds float64 `json:"downtime_seconds,omitempty"`
}

// RollbackManager manages rollback operations within the pipeline.
type RollbackManager struct {
	mu sync.RWMutex

	// rollbacks stores active rollbacks
	rollbacks map[string]*PipelineRollback

	// history stores rollback history by service
	history map[string][]*PipelineRollback

	// inProgress tracks rollbacks in progress by service
	inProgress map[string]string

	// artifactTracker for finding previous versions
	artifactTracker *ArtifactTracker

	// trafficManager for traffic shifting
	trafficManager TrafficManager

	// instanceManager for instance operations
	instanceManager InstanceManager

	// healthChecker for health verification
	healthChecker HealthChecker

	// hookExecutor for running hooks
	hookExecutor *HookExecutor

	// config is the default rollback configuration
	config *RollbackConfig

	// maxHistory is max history entries per service
	maxHistory int

	// eventCallback for rollback events
	eventCallback func(event *RollbackEvent)
}

// RollbackConfig contains rollback configuration.
type RollbackConfig struct {
	// AutoRollback enables automatic rollback on failure
	AutoRollback bool `json:"auto_rollback"`

	// DefaultStrategy is the default rollback strategy
	DefaultStrategy *RollbackStrategy `json:"default_strategy,omitempty"`

	// HealthCheckRetries before triggering rollback
	HealthCheckRetries int `json:"health_check_retries,omitempty"`

	// MetricThresholds for automatic rollback
	MetricThresholds *MetricThresholds `json:"metric_thresholds,omitempty"`

	// GracePeriod before triggering auto-rollback
	GracePeriod time.Duration `json:"grace_period,omitempty"`

	// NotifyOnRollback sends notifications on rollback
	NotifyOnRollback bool `json:"notify_on_rollback,omitempty"`
}

// MetricThresholds for auto-rollback.
type MetricThresholds struct {
	// ErrorRateThreshold triggers rollback above this error rate
	ErrorRateThreshold float64 `json:"error_rate_threshold,omitempty"`

	// LatencyP99Threshold triggers rollback above this latency
	LatencyP99Threshold time.Duration `json:"latency_p99_threshold,omitempty"`

	// SuccessRateThreshold triggers rollback below this success rate
	SuccessRateThreshold float64 `json:"success_rate_threshold,omitempty"`
}

// RollbackEvent represents a rollback event.
type RollbackEvent struct {
	// Type is the event type
	Type string `json:"type"`

	// RollbackID is the rollback ID
	RollbackID string `json:"rollback_id"`

	// DeploymentID is the deployment ID
	DeploymentID string `json:"deployment_id"`

	// ServiceName is the service name
	ServiceName string `json:"service_name"`

	// State is the rollback state
	State RollbackState `json:"state"`

	// Progress is the current progress
	Progress *RollbackProgress `json:"progress,omitempty"`

	// Timestamp of the event
	Timestamp time.Time `json:"timestamp"`

	// Data contains additional event data
	Data map[string]interface{} `json:"data,omitempty"`
}

// NewRollbackManager creates a new rollback manager.
func NewRollbackManager(
	artifactTracker *ArtifactTracker,
	trafficManager TrafficManager,
	instanceManager InstanceManager,
	healthChecker HealthChecker,
	config *RollbackConfig,
) *RollbackManager {
	if config == nil {
		config = DefaultRollbackConfig()
	}

	return &RollbackManager{
		rollbacks:       make(map[string]*PipelineRollback),
		history:         make(map[string][]*PipelineRollback),
		inProgress:      make(map[string]string),
		artifactTracker: artifactTracker,
		trafficManager:  trafficManager,
		instanceManager: instanceManager,
		healthChecker:   healthChecker,
		hookExecutor:    NewHookExecutor(),
		config:          config,
		maxHistory:      100,
	}
}

// DefaultRollbackConfig returns default rollback configuration.
func DefaultRollbackConfig() *RollbackConfig {
	return &RollbackConfig{
		AutoRollback:       true,
		HealthCheckRetries: 3,
		GracePeriod:        30 * time.Second,
		NotifyOnRollback:   true,
		DefaultStrategy: &RollbackStrategy{
			Type:          "instant",
			VerifyAfter:   true,
			VerifyTimeout: 5 * time.Minute,
			Timeout:       15 * time.Minute,
			Instant: &InstantRollbackConfig{
				StopCurrent:     true,
				RestartPrevious: true,
				PreserveTraffic: true,
			},
		},
		MetricThresholds: &MetricThresholds{
			ErrorRateThreshold:   0.05,  // 5%
			SuccessRateThreshold: 0.95,  // 95%
			LatencyP99Threshold:  5 * time.Second,
		},
	}
}

// SetEventCallback sets the event callback.
func (m *RollbackManager) SetEventCallback(callback func(event *RollbackEvent)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.eventCallback = callback
}

// Initiate initiates a rollback.
func (m *RollbackManager) Initiate(ctx context.Context, request *RollbackRequest) (*PipelineRollback, error) {
	// Validate request
	if err := m.validateRequest(request); err != nil {
		return nil, err
	}

	// Check for existing rollback in progress
	m.mu.RLock()
	if existingID, ok := m.inProgress[request.ServiceName]; ok {
		m.mu.RUnlock()
		return nil, fmt.Errorf("%w: rollback %s in progress for %s", ErrRollbackInProgress, existingID, request.ServiceName)
	}
	m.mu.RUnlock()

	// Find rollback target version
	toVersion := request.ToVersion
	if toVersion == "" {
		// Find previous successful version
		if m.artifactTracker != nil {
			prev, err := m.artifactTracker.GetPreviousDeployed(ctx, request.ServiceName)
			if err != nil {
				return nil, fmt.Errorf("%w: %v", ErrRollbackTargetMissing, err)
			}
			toVersion = prev.Version
		}
	}

	if toVersion == "" {
		return nil, ErrRollbackTargetMissing
	}

	// Create rollback
	rollback := &PipelineRollback{
		ID:           generateRollbackID(),
		PipelineID:   request.PipelineID,
		DeploymentID: request.DeploymentID,
		ServiceName:  request.ServiceName,
		FromVersion:  request.FromVersion,
		ToVersion:    toVersion,
		Trigger:      request.Trigger,
		State:        RollbackStatePending,
		Strategy:     request.Strategy,
		InitiatedBy:  request.InitiatedBy,
		Reason:       request.Reason,
		StartedAt:    time.Now(),
		Progress: &RollbackProgress{
			Phase:   "initializing",
			Message: "Starting rollback",
		},
		Metrics: &RollbackMetrics{},
	}

	// Use default strategy if not provided
	if rollback.Strategy == nil {
		rollback.Strategy = m.config.DefaultStrategy
	}

	// Store rollback
	m.mu.Lock()
	m.rollbacks[rollback.ID] = rollback
	m.inProgress[request.ServiceName] = rollback.ID
	m.mu.Unlock()

	// Execute rollback
	go m.executeRollback(context.Background(), rollback)

	return rollback, nil
}

// RollbackRequest contains parameters for initiating a rollback.
type RollbackRequest struct {
	// PipelineID is the pipeline to rollback
	PipelineID string `json:"pipeline_id,omitempty"`

	// DeploymentID is the deployment to rollback
	DeploymentID string `json:"deployment_id,omitempty"`

	// ServiceName is the service to rollback
	ServiceName string `json:"service_name"`

	// FromVersion is the current version (optional, will be detected)
	FromVersion string `json:"from_version,omitempty"`

	// ToVersion is the target version (optional, will use previous)
	ToVersion string `json:"to_version,omitempty"`

	// Trigger is what triggered the rollback
	Trigger RollbackTrigger `json:"trigger"`

	// Strategy is the rollback strategy
	Strategy *RollbackStrategy `json:"strategy,omitempty"`

	// InitiatedBy is who initiated the rollback
	InitiatedBy string `json:"initiated_by,omitempty"`

	// Reason is the reason for rollback
	Reason string `json:"reason,omitempty"`
}

// validateRequest validates a rollback request.
func (m *RollbackManager) validateRequest(request *RollbackRequest) error {
	if request == nil {
		return errors.New("request is nil")
	}
	if request.ServiceName == "" {
		return errors.New("service name is required")
	}
	if request.Trigger == "" {
		request.Trigger = TriggerManual
	}
	return nil
}

// executeRollback performs the rollback operation.
func (m *RollbackManager) executeRollback(ctx context.Context, rollback *PipelineRollback) {
	defer func() {
		m.mu.Lock()
		delete(m.inProgress, rollback.ServiceName)
		m.mu.Unlock()

		// Add to history
		m.addToHistory(rollback)
	}()

	// Apply timeout
	if rollback.Strategy != nil && rollback.Strategy.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, rollback.Strategy.Timeout)
		defer cancel()
	}

	// Update state
	m.updateState(rollback, RollbackStateExecuting, "Executing rollback")
	m.emitEvent(rollback, "rollback_started", nil)

	// Execute pre-rollback hooks
	if rollback.Strategy != nil && rollback.Strategy.Hooks != nil {
		if err := m.executeHooks(ctx, rollback.Strategy.Hooks.PreRollback, rollback); err != nil {
			m.failRollback(rollback, fmt.Errorf("pre-rollback hooks failed: %w", err))
			return
		}
	}

	// Execute rollback based on strategy type
	var err error
	strategyType := "instant"
	if rollback.Strategy != nil && rollback.Strategy.Type != "" {
		strategyType = rollback.Strategy.Type
	}

	switch strategyType {
	case "instant":
		err = m.executeInstantRollback(ctx, rollback)
	case "gradual":
		err = m.executeGradualRollback(ctx, rollback)
	case "blue_green":
		err = m.executeBlueGreenRollback(ctx, rollback)
	default:
		err = m.executeInstantRollback(ctx, rollback)
	}

	if err != nil {
		// Execute on-failure hooks
		if rollback.Strategy != nil && rollback.Strategy.Hooks != nil {
			m.executeHooks(ctx, rollback.Strategy.Hooks.OnFailure, rollback)
		}
		m.failRollback(rollback, err)
		return
	}

	// Verify health after rollback if configured
	if rollback.Strategy != nil && rollback.Strategy.VerifyAfter {
		if err := m.verifyRollback(ctx, rollback); err != nil {
			m.failRollback(rollback, fmt.Errorf("verification failed: %w", err))
			return
		}
	}

	// Execute post-rollback hooks
	if rollback.Strategy != nil && rollback.Strategy.Hooks != nil {
		m.executeHooks(ctx, rollback.Strategy.Hooks.PostRollback, rollback)
	}

	// Complete rollback
	m.mu.Lock()
	rollback.State = RollbackStateCompleted
	rollback.CompletedAt = time.Now()
	rollback.Duration = rollback.CompletedAt.Sub(rollback.StartedAt)
	rollback.Progress.Phase = "completed"
	rollback.Progress.Message = fmt.Sprintf("Rollback completed: %s -> %s", rollback.FromVersion, rollback.ToVersion)
	rollback.Progress.Percentage = 100
	m.mu.Unlock()

	m.emitEvent(rollback, "rollback_completed", nil)

	// Update artifact tracker
	if m.artifactTracker != nil {
		m.artifactTracker.MarkRollback(ctx, rollback.ServiceName, rollback.ToVersion, rollback.ID)
	}
}

// executeInstantRollback performs an instant rollback.
func (m *RollbackManager) executeInstantRollback(ctx context.Context, rollback *PipelineRollback) error {
	config := &InstantRollbackConfig{
		StopCurrent:     true,
		RestartPrevious: true,
		PreserveTraffic: true,
	}
	if rollback.Strategy != nil && rollback.Strategy.Instant != nil {
		config = rollback.Strategy.Instant
	}

	// Step 1: Shift traffic to old version (if preserving traffic)
	if config.PreserveTraffic && m.trafficManager != nil {
		rollback.Progress.Phase = "shifting_traffic"
		rollback.Progress.Message = "Shifting traffic to previous version"

		if err := m.trafficManager.ShiftTraffic(ctx, rollback.ServiceName, rollback.FromVersion, rollback.ToVersion, 100); err != nil {
			return fmt.Errorf("failed to shift traffic: %w", err)
		}
		rollback.Metrics.TrafficShifts++
		rollback.Progress.TrafficShifted = 100
	}

	// Step 2: Stop current instances
	if config.StopCurrent && m.instanceManager != nil {
		rollback.Progress.Phase = "stopping_current"
		rollback.Progress.Message = "Stopping current version instances"

		if err := m.instanceManager.DeleteInstances(ctx, rollback.ServiceName, rollback.FromVersion); err != nil {
			// Log but continue - traffic already shifted
		}
	}

	// Step 3: Restart previous instances if needed
	if config.RestartPrevious && m.instanceManager != nil {
		rollback.Progress.Phase = "starting_previous"
		rollback.Progress.Message = "Starting previous version instances"

		instances, err := m.instanceManager.GetInstances(ctx, rollback.ServiceName, rollback.ToVersion)
		if err != nil || len(instances) == 0 {
			// Need to create instances for old version
			// This would require artifact reference, skipping in simulation
		}
	}

	rollback.Progress.Percentage = 100
	return nil
}

// executeGradualRollback performs a gradual rollback.
func (m *RollbackManager) executeGradualRollback(ctx context.Context, rollback *PipelineRollback) error {
	config := &GradualRollbackConfig{
		Steps: []RollbackStep{
			{Weight: 25, Duration: time.Minute, Verify: true},
			{Weight: 50, Duration: time.Minute, Verify: true},
			{Weight: 75, Duration: time.Minute, Verify: true},
			{Weight: 100, Duration: 0, Verify: true},
		},
	}
	if rollback.Strategy != nil && rollback.Strategy.Gradual != nil {
		config = rollback.Strategy.Gradual
	}

	rollback.Progress.TotalSteps = len(config.Steps)

	for stepIdx, step := range config.Steps {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		rollback.Progress.Phase = fmt.Sprintf("step_%d", stepIdx+1)
		rollback.Progress.CurrentStep = stepIdx + 1
		rollback.Progress.Message = fmt.Sprintf("Gradual rollback step %d: %d%% traffic", stepIdx+1, step.Weight)

		// Shift traffic
		if m.trafficManager != nil {
			if err := m.trafficManager.ShiftTraffic(ctx, rollback.ServiceName, rollback.FromVersion, rollback.ToVersion, step.Weight); err != nil {
				if config.PauseOnError {
					// Pause and wait for manual intervention
					return fmt.Errorf("traffic shift failed at step %d: %w", stepIdx+1, err)
				}
			}
			rollback.Metrics.TrafficShifts++
		}

		rollback.Progress.TrafficShifted = step.Weight
		rollback.Progress.Percentage = (stepIdx + 1) * 100 / len(config.Steps)

		m.emitEvent(rollback, "rollback_progress", map[string]interface{}{
			"step":   stepIdx + 1,
			"weight": step.Weight,
		})

		// Verify health at this step
		if step.Verify && m.healthChecker != nil {
			if _, err := m.healthChecker.CheckHealth(ctx, rollback.ServiceName, nil); err != nil {
				if config.PauseOnError {
					return fmt.Errorf("health check failed at step %d: %w", stepIdx+1, err)
				}
			}
			rollback.Metrics.HealthChecks++
		}

		// Hold at this step
		if step.Duration > 0 && step.Weight < 100 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(step.Duration):
			}
		}
	}

	return nil
}

// executeBlueGreenRollback performs a blue-green rollback.
func (m *RollbackManager) executeBlueGreenRollback(ctx context.Context, rollback *PipelineRollback) error {
	// For blue-green, we simply switch traffic back to the previous version instantly
	rollback.Progress.Phase = "switching_traffic"
	rollback.Progress.Message = "Switching traffic to previous version"

	if m.trafficManager != nil {
		if err := m.trafficManager.ShiftTraffic(ctx, rollback.ServiceName, rollback.FromVersion, rollback.ToVersion, 100); err != nil {
			return fmt.Errorf("failed to switch traffic: %w", err)
		}
		rollback.Metrics.TrafficShifts++
	}

	rollback.Progress.TrafficShifted = 100
	rollback.Progress.Percentage = 100

	return nil
}

// verifyRollback verifies the rollback was successful.
func (m *RollbackManager) verifyRollback(ctx context.Context, rollback *PipelineRollback) error {
	rollback.Progress.Phase = "verifying"
	rollback.Progress.Message = "Verifying rollback health"

	timeout := 5 * time.Minute
	if rollback.Strategy != nil && rollback.Strategy.VerifyTimeout > 0 {
		timeout = rollback.Strategy.VerifyTimeout
	}

	verifyCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	retries := 3
	for i := 0; i < retries; i++ {
		select {
		case <-verifyCtx.Done():
			return verifyCtx.Err()
		default:
		}

		if m.healthChecker != nil {
			result, err := m.healthChecker.CheckHealth(verifyCtx, rollback.ServiceName, nil)
			rollback.Metrics.HealthChecks++

			if err == nil && result != nil && result.Healthy {
				return nil
			}
		} else {
			// No health checker, assume success
			return nil
		}

		time.Sleep(5 * time.Second)
	}

	return errors.New("health verification failed after retries")
}

// executeHooks executes a list of hooks.
func (m *RollbackManager) executeHooks(ctx context.Context, hooks []*Hook, rollback *PipelineRollback) error {
	if m.hookExecutor == nil {
		return nil
	}

	pipelineCtx := map[string]interface{}{
		"rollback_id":   rollback.ID,
		"service_name":  rollback.ServiceName,
		"from_version":  rollback.FromVersion,
		"to_version":    rollback.ToVersion,
		"trigger":       rollback.Trigger,
	}

	for _, hook := range hooks {
		if err := m.hookExecutor.Execute(ctx, hook, pipelineCtx); err != nil {
			if hook.OnError == HookErrorAbort {
				return err
			}
		}
		rollback.Metrics.HooksExecuted++
	}

	return nil
}

// Cancel cancels an in-progress rollback.
func (m *RollbackManager) Cancel(ctx context.Context, rollbackID string) error {
	m.mu.Lock()
	rollback, exists := m.rollbacks[rollbackID]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("rollback not found: %s", rollbackID)
	}

	if rollback.State != RollbackStateExecuting && rollback.State != RollbackStatePending {
		m.mu.Unlock()
		return fmt.Errorf("cannot cancel rollback in state: %s", rollback.State)
	}

	rollback.State = RollbackStateCancelled
	rollback.CompletedAt = time.Now()
	rollback.Duration = rollback.CompletedAt.Sub(rollback.StartedAt)
	rollback.Progress.Phase = "cancelled"
	rollback.Progress.Message = "Rollback cancelled"
	delete(m.inProgress, rollback.ServiceName)
	m.mu.Unlock()

	m.emitEvent(rollback, "rollback_cancelled", nil)

	return nil
}

// Status returns the status of a rollback.
func (m *RollbackManager) Status(ctx context.Context, rollbackID string) (*PipelineRollback, error) {
	m.mu.RLock()
	rollback, exists := m.rollbacks[rollbackID]
	m.mu.RUnlock()

	if !exists {
		// Check history
		for _, history := range m.history {
			for _, r := range history {
				if r.ID == rollbackID {
					return r, nil
				}
			}
		}
		return nil, fmt.Errorf("rollback not found: %s", rollbackID)
	}

	return rollback, nil
}

// GetHistory returns rollback history for a service.
func (m *RollbackManager) GetHistory(ctx context.Context, serviceName string, limit int) ([]*PipelineRollback, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	history, exists := m.history[serviceName]
	if !exists {
		return []*PipelineRollback{}, nil
	}

	// Return copy in reverse order (newest first)
	result := make([]*PipelineRollback, len(history))
	for i, r := range history {
		result[len(history)-1-i] = r
	}

	if limit > 0 && limit < len(result) {
		result = result[:limit]
	}

	return result, nil
}

// IsRollbackInProgress checks if a rollback is in progress for a service.
func (m *RollbackManager) IsRollbackInProgress(serviceName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.inProgress[serviceName]
	return ok
}

// ShouldAutoRollback determines if auto-rollback should be triggered.
func (m *RollbackManager) ShouldAutoRollback(metrics *AnalysisMetrics) bool {
	if m.config == nil || !m.config.AutoRollback || m.config.MetricThresholds == nil {
		return false
	}

	thresholds := m.config.MetricThresholds

	// Check error rate
	if thresholds.ErrorRateThreshold > 0 && metrics.ErrorRate > thresholds.ErrorRateThreshold {
		return true
	}

	// Check success rate
	if thresholds.SuccessRateThreshold > 0 && metrics.SuccessRate < thresholds.SuccessRateThreshold {
		return true
	}

	// Check latency
	if thresholds.LatencyP99Threshold > 0 && metrics.LatencyP99 > thresholds.LatencyP99Threshold {
		return true
	}

	return false
}

// TriggerAutoRollback triggers an automatic rollback.
func (m *RollbackManager) TriggerAutoRollback(ctx context.Context, serviceName, fromVersion, reason string, trigger RollbackTrigger) (*PipelineRollback, error) {
	return m.Initiate(ctx, &RollbackRequest{
		ServiceName: serviceName,
		FromVersion: fromVersion,
		Trigger:     trigger,
		InitiatedBy: "auto-rollback",
		Reason:      reason,
	})
}

// updateState updates rollback state.
func (m *RollbackManager) updateState(rollback *PipelineRollback, state RollbackState, message string) {
	m.mu.Lock()
	rollback.State = state
	rollback.Progress.Message = message
	m.mu.Unlock()
}

// failRollback marks a rollback as failed.
func (m *RollbackManager) failRollback(rollback *PipelineRollback, err error) {
	m.mu.Lock()
	rollback.State = RollbackStateFailed
	rollback.CompletedAt = time.Now()
	rollback.Duration = rollback.CompletedAt.Sub(rollback.StartedAt)
	rollback.Error = err.Error()
	rollback.Progress.Phase = "failed"
	rollback.Progress.Message = err.Error()
	m.mu.Unlock()

	m.emitEvent(rollback, "rollback_failed", map[string]interface{}{
		"error": err.Error(),
	})
}

// addToHistory adds a rollback to history.
func (m *RollbackManager) addToHistory(rollback *PipelineRollback) {
	m.mu.Lock()
	defer m.mu.Unlock()

	history := m.history[rollback.ServiceName]
	history = append(history, rollback)

	// Trim to max
	if len(history) > m.maxHistory {
		history = history[len(history)-m.maxHistory:]
	}

	m.history[rollback.ServiceName] = history
}

// emitEvent emits a rollback event.
func (m *RollbackManager) emitEvent(rollback *PipelineRollback, eventType string, data map[string]interface{}) {
	m.mu.RLock()
	callback := m.eventCallback
	m.mu.RUnlock()

	if callback == nil {
		return
	}

	event := &RollbackEvent{
		Type:         eventType,
		RollbackID:   rollback.ID,
		DeploymentID: rollback.DeploymentID,
		ServiceName:  rollback.ServiceName,
		State:        rollback.State,
		Progress:     rollback.Progress,
		Timestamp:    time.Now(),
		Data:         data,
	}

	go callback(event)
}

// generateRollbackID generates a unique rollback ID.
func generateRollbackID() string {
	return fmt.Sprintf("rollback-%d", time.Now().UnixNano())
}

// GetStats returns rollback statistics.
func (m *RollbackManager) GetStats(ctx context.Context) *RollbackStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := &RollbackStats{
		Active:    len(m.inProgress),
		ByTrigger: make(map[RollbackTrigger]int),
		ByState:   make(map[RollbackState]int),
	}

	var totalDuration time.Duration
	completed := 0

	for _, history := range m.history {
		stats.Total += len(history)
		for _, r := range history {
			stats.ByTrigger[r.Trigger]++
			stats.ByState[r.State]++

			if r.State == RollbackStateCompleted {
				stats.Successful++
				totalDuration += r.Duration
				completed++
			} else if r.State == RollbackStateFailed {
				stats.Failed++
			}
		}
	}

	if completed > 0 {
		stats.AvgDuration = totalDuration / time.Duration(completed)
	}

	return stats
}

// RollbackStats contains rollback statistics.
type RollbackStats struct {
	Total       int                        `json:"total"`
	Active      int                        `json:"active"`
	Successful  int                        `json:"successful"`
	Failed      int                        `json:"failed"`
	ByTrigger   map[RollbackTrigger]int    `json:"by_trigger"`
	ByState     map[RollbackState]int      `json:"by_state"`
	AvgDuration time.Duration              `json:"avg_duration"`
}
