// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package rollback provides rollback management for deployments.
package rollback

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Common errors returned by rollback operations.
var (
	ErrNoRollbackTarget    = errors.New("no rollback target available")
	ErrRollbackInProgress  = errors.New("rollback already in progress")
	ErrRollbackFailed      = errors.New("rollback failed")
	ErrInvalidDeployment   = errors.New("invalid deployment for rollback")
	ErrDeploymentNotFound  = errors.New("deployment not found")
)

// RollbackState represents the state of a rollback.
type RollbackState string

const (
	RollbackStatePending    RollbackState = "pending"
	RollbackStateRunning    RollbackState = "running"
	RollbackStateCompleted  RollbackState = "completed"
	RollbackStateFailed     RollbackState = "failed"
	RollbackStateCancelled  RollbackState = "cancelled"
)

// RollbackReason represents why a rollback was triggered.
type RollbackReason string

const (
	RollbackReasonManual      RollbackReason = "manual"
	RollbackReasonAutomatic   RollbackReason = "automatic"
	RollbackReasonHealthCheck RollbackReason = "health_check"
	RollbackReasonTimeout     RollbackReason = "timeout"
	RollbackReasonCanaryFail  RollbackReason = "canary_failure"
)

// Rollback represents a rollback operation.
type Rollback struct {
	// ID is the unique rollback identifier
	ID string `json:"id"`

	// DeploymentID is the deployment being rolled back
	DeploymentID string `json:"deployment_id"`

	// TargetDeploymentID is the deployment to rollback to
	TargetDeploymentID string `json:"target_deployment_id"`

	// ServiceName is the service being rolled back
	ServiceName string `json:"service_name"`

	// FromVersion is the version being rolled back from
	FromVersion string `json:"from_version"`

	// ToVersion is the version being rolled back to
	ToVersion string `json:"to_version"`

	// State is the current rollback state
	State RollbackState `json:"state"`

	// Reason is why the rollback was triggered
	Reason RollbackReason `json:"reason"`

	// Message provides additional context
	Message string `json:"message,omitempty"`

	// InitiatedBy is who initiated the rollback
	InitiatedBy string `json:"initiated_by,omitempty"`

	// StartedAt is when the rollback started
	StartedAt time.Time `json:"started_at"`

	// CompletedAt is when the rollback completed
	CompletedAt time.Time `json:"completed_at,omitempty"`

	// Duration is the rollback duration
	Duration time.Duration `json:"duration,omitempty"`

	// Metrics contains rollback metrics
	Metrics *RollbackMetrics `json:"metrics,omitempty"`

	// Error contains error details if failed
	Error string `json:"error,omitempty"`
}

// RollbackMetrics contains metrics about the rollback.
type RollbackMetrics struct {
	// InstancesRolledBack is the count of instances rolled back
	InstancesRolledBack int `json:"instances_rolled_back"`

	// InstancesFailed is the count of instances that failed rollback
	InstancesFailed int `json:"instances_failed"`

	// TrafficShifts is the count of traffic shifts performed
	TrafficShifts int `json:"traffic_shifts"`

	// HealthChecks is the count of health checks performed
	HealthChecks int `json:"health_checks"`
}

// RollbackRequest represents a request to perform a rollback.
type RollbackRequest struct {
	// DeploymentID is the deployment to rollback
	DeploymentID string `json:"deployment_id"`

	// TargetDeploymentID is optional specific target
	TargetDeploymentID string `json:"target_deployment_id,omitempty"`

	// Reason for the rollback
	Reason RollbackReason `json:"reason"`

	// InitiatedBy is who initiated the rollback
	InitiatedBy string `json:"initiated_by,omitempty"`
}

// RollbackCallback is called during rollback operations.
type RollbackCallback func(ctx context.Context, rollback *Rollback) error

// Manager manages rollback operations.
type Manager struct {
	mu sync.RWMutex

	// rollbacks stores active rollbacks
	rollbacks map[string]*Rollback

	// history stores rollback history by service
	history *History

	// deploymentLookup retrieves deployment info
	deploymentLookup DeploymentLookup

	// instanceUpdater updates instances
	instanceUpdater InstanceUpdater

	// trafficShifter shifts traffic
	trafficShifter TrafficShifter

	// inProgress tracks rollbacks in progress by service
	inProgress map[string]string

	// callbacks for rollback events
	callbacks []RollbackCallback
}

// DeploymentLookup interface for looking up deployments.
type DeploymentLookup interface {
	// Get returns a deployment by ID
	Get(ctx context.Context, deploymentID string) (*DeploymentInfo, error)

	// GetPrevious returns the previous successful deployment for a service
	GetPrevious(ctx context.Context, serviceName string, beforeDeploymentID string) (*DeploymentInfo, error)

	// GetByService returns deployments for a service
	GetByService(ctx context.Context, serviceName string, limit int) ([]*DeploymentInfo, error)
}

// DeploymentInfo contains deployment information needed for rollback.
type DeploymentInfo struct {
	ID          string `json:"id"`
	ServiceName string `json:"service_name"`
	Version     string `json:"version"`
	ArtifactRef string `json:"artifact_ref"`
	State       string `json:"state"`
	Replicas    int    `json:"replicas"`
}

// InstanceUpdater interface for updating instances.
type InstanceUpdater interface {
	// Update updates instances to a new version
	Update(ctx context.Context, serviceName, version, artifactRef string, replicas int) error
}

// TrafficShifter interface for shifting traffic.
type TrafficShifter interface {
	// Shift shifts traffic between versions
	Shift(ctx context.Context, serviceName, fromVersion, toVersion string, weight int) error
}

// NewManager creates a new rollback manager.
func NewManager(lookup DeploymentLookup, updater InstanceUpdater, shifter TrafficShifter) *Manager {
	return &Manager{
		rollbacks:        make(map[string]*Rollback),
		history:          NewHistory(100),
		deploymentLookup: lookup,
		instanceUpdater:  updater,
		trafficShifter:   shifter,
		inProgress:       make(map[string]string),
	}
}

// AddCallback adds a rollback callback.
func (m *Manager) AddCallback(callback RollbackCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callbacks = append(m.callbacks, callback)
}

// Rollback initiates a rollback.
func (m *Manager) Rollback(ctx context.Context, request *RollbackRequest) (*Rollback, error) {
	// Get current deployment
	deployment, err := m.deploymentLookup.Get(ctx, request.DeploymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}

	// Check for existing rollback in progress
	m.mu.RLock()
	if existingID, ok := m.inProgress[deployment.ServiceName]; ok {
		m.mu.RUnlock()
		return nil, fmt.Errorf("%w: rollback %s in progress", ErrRollbackInProgress, existingID)
	}
	m.mu.RUnlock()

	// Get target deployment
	var targetDeployment *DeploymentInfo
	if request.TargetDeploymentID != "" {
		targetDeployment, err = m.deploymentLookup.Get(ctx, request.TargetDeploymentID)
		if err != nil {
			return nil, fmt.Errorf("failed to get target deployment: %w", err)
		}
	} else {
		// Find previous successful deployment
		targetDeployment, err = m.deploymentLookup.GetPrevious(ctx, deployment.ServiceName, deployment.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to find previous deployment: %w", err)
		}
	}

	if targetDeployment == nil {
		return nil, ErrNoRollbackTarget
	}

	// Create rollback
	rollback := &Rollback{
		ID:                 generateRollbackID(),
		DeploymentID:       deployment.ID,
		TargetDeploymentID: targetDeployment.ID,
		ServiceName:        deployment.ServiceName,
		FromVersion:        deployment.Version,
		ToVersion:          targetDeployment.Version,
		State:              RollbackStatePending,
		Reason:             request.Reason,
		InitiatedBy:        request.InitiatedBy,
		StartedAt:          time.Now(),
		Metrics:            &RollbackMetrics{},
	}

	// Store rollback
	m.mu.Lock()
	m.rollbacks[rollback.ID] = rollback
	m.inProgress[deployment.ServiceName] = rollback.ID
	m.mu.Unlock()

	// Execute rollback
	go m.executeRollback(context.Background(), rollback, targetDeployment)

	return rollback, nil
}

// executeRollback performs the rollback operation.
func (m *Manager) executeRollback(ctx context.Context, rollback *Rollback, target *DeploymentInfo) {
	defer func() {
		m.mu.Lock()
		delete(m.inProgress, rollback.ServiceName)
		m.mu.Unlock()

		// Add to history
		m.history.Add(rollback)
	}()

	// Update state
	m.updateState(rollback, RollbackStateRunning, "Starting rollback")

	// Notify callbacks
	m.notifyCallbacks(ctx, rollback)

	// Step 1: Shift traffic away from current version (if supported)
	if m.trafficShifter != nil {
		m.mu.Lock()
		rollback.Message = "Shifting traffic to previous version"
		m.mu.Unlock()
		if err := m.trafficShifter.Shift(ctx, rollback.ServiceName, rollback.FromVersion, rollback.ToVersion, 100); err != nil {
			m.failRollback(rollback, fmt.Errorf("traffic shift failed: %w", err))
			return
		}
		m.mu.Lock()
		rollback.Metrics.TrafficShifts++
		m.mu.Unlock()
	}

	// Step 2: Update instances
	m.mu.Lock()
	rollback.Message = "Updating instances to previous version"
	m.mu.Unlock()
	if err := m.instanceUpdater.Update(ctx, target.ServiceName, target.Version, target.ArtifactRef, target.Replicas); err != nil {
		m.failRollback(rollback, fmt.Errorf("instance update failed: %w", err))
		return
	}
	m.mu.Lock()
	rollback.Metrics.InstancesRolledBack = target.Replicas
	m.mu.Unlock()

	// Complete rollback
	m.mu.Lock()
	rollback.State = RollbackStateCompleted
	rollback.CompletedAt = time.Now()
	rollback.Duration = rollback.CompletedAt.Sub(rollback.StartedAt)
	rollback.Message = fmt.Sprintf("Rollback completed: %s -> %s", rollback.FromVersion, rollback.ToVersion)
	m.mu.Unlock()

	// Notify callbacks
	m.notifyCallbacks(ctx, rollback)
}

// updateState updates the rollback state.
func (m *Manager) updateState(rollback *Rollback, state RollbackState, message string) {
	m.mu.Lock()
	rollback.State = state
	rollback.Message = message
	m.mu.Unlock()
}

// failRollback marks a rollback as failed.
func (m *Manager) failRollback(rollback *Rollback, err error) {
	m.mu.Lock()
	rollback.State = RollbackStateFailed
	rollback.CompletedAt = time.Now()
	rollback.Duration = rollback.CompletedAt.Sub(rollback.StartedAt)
	rollback.Error = err.Error()
	rollback.Message = fmt.Sprintf("Rollback failed: %v", err)
	m.mu.Unlock()
}

// notifyCallbacks notifies all registered callbacks.
func (m *Manager) notifyCallbacks(ctx context.Context, rollback *Rollback) {
	m.mu.RLock()
	callbacks := make([]RollbackCallback, len(m.callbacks))
	copy(callbacks, m.callbacks)
	m.mu.RUnlock()

	for _, callback := range callbacks {
		go callback(ctx, rollback)
	}
}

// Cancel cancels an in-progress rollback.
func (m *Manager) Cancel(ctx context.Context, rollbackID string) error {
	m.mu.Lock()
	rollback, exists := m.rollbacks[rollbackID]
	if !exists {
		m.mu.Unlock()
		return ErrDeploymentNotFound
	}

	if rollback.State != RollbackStateRunning && rollback.State != RollbackStatePending {
		m.mu.Unlock()
		return fmt.Errorf("rollback cannot be cancelled in state: %s", rollback.State)
	}

	rollback.State = RollbackStateCancelled
	rollback.CompletedAt = time.Now()
	rollback.Duration = rollback.CompletedAt.Sub(rollback.StartedAt)
	rollback.Message = "Rollback cancelled"
	delete(m.inProgress, rollback.ServiceName)
	m.mu.Unlock()

	return nil
}

// Status returns the status of a rollback.
// It returns a snapshot (copy) so the caller can safely read fields without
// holding the manager's lock.
func (m *Manager) Status(ctx context.Context, rollbackID string) (*Rollback, error) {
	m.mu.RLock()
	rollback, exists := m.rollbacks[rollbackID]
	if !exists {
		m.mu.RUnlock()
		// Check history
		return m.history.Get(rollbackID)
	}
	// Return a copy so callers don't race with executeRollback.
	cp := *rollback
	if rollback.Metrics != nil {
		metricsCopy := *rollback.Metrics
		cp.Metrics = &metricsCopy
	}
	m.mu.RUnlock()

	return &cp, nil
}

// GetByService returns rollbacks for a service.
func (m *Manager) GetByService(ctx context.Context, serviceName string) ([]*Rollback, error) {
	return m.history.GetByService(serviceName)
}

// IsRollbackInProgress checks if a rollback is in progress for a service.
func (m *Manager) IsRollbackInProgress(serviceName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.inProgress[serviceName]
	return ok
}

// generateRollbackID generates a unique rollback ID.
func generateRollbackID() string {
	return fmt.Sprintf("rollback-%d", time.Now().UnixNano())
}

// AutoRollbackConfig configures automatic rollback behavior.
type AutoRollbackConfig struct {
	// Enabled enables automatic rollback
	Enabled bool `json:"enabled"`

	// HealthCheckFailures triggers rollback after this many failures
	HealthCheckFailures int `json:"health_check_failures"`

	// ErrorRateThreshold triggers rollback above this error rate
	ErrorRateThreshold float64 `json:"error_rate_threshold"`

	// LatencyThreshold triggers rollback above this p99 latency
	LatencyThreshold time.Duration `json:"latency_threshold"`

	// GracePeriod is time to wait before auto-rollback
	GracePeriod time.Duration `json:"grace_period"`
}

// DefaultAutoRollbackConfig returns default auto-rollback configuration.
func DefaultAutoRollbackConfig() *AutoRollbackConfig {
	return &AutoRollbackConfig{
		Enabled:             true,
		HealthCheckFailures: 3,
		ErrorRateThreshold:  0.05, // 5%
		LatencyThreshold:    5 * time.Second,
		GracePeriod:         30 * time.Second,
	}
}

// AutoRollbackMonitor monitors deployments for auto-rollback triggers.
type AutoRollbackMonitor struct {
	manager *Manager
	config  *AutoRollbackConfig
	mu      sync.RWMutex
	watches map[string]*deploymentWatch
	stopCh  chan struct{}
}

type deploymentWatch struct {
	deploymentID string
	serviceName  string
	failures     int
	startedAt    time.Time
}

// NewAutoRollbackMonitor creates a new auto-rollback monitor.
func NewAutoRollbackMonitor(manager *Manager, config *AutoRollbackConfig) *AutoRollbackMonitor {
	if config == nil {
		config = DefaultAutoRollbackConfig()
	}

	return &AutoRollbackMonitor{
		manager: manager,
		config:  config,
		watches: make(map[string]*deploymentWatch),
		stopCh:  make(chan struct{}),
	}
}

// Watch starts watching a deployment for auto-rollback triggers.
func (m *AutoRollbackMonitor) Watch(deploymentID, serviceName string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.watches[deploymentID] = &deploymentWatch{
		deploymentID: deploymentID,
		serviceName:  serviceName,
		startedAt:    time.Now(),
	}
}

// Unwatch stops watching a deployment.
func (m *AutoRollbackMonitor) Unwatch(deploymentID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.watches, deploymentID)
}

// RecordHealthCheckFailure records a health check failure.
func (m *AutoRollbackMonitor) RecordHealthCheckFailure(ctx context.Context, deploymentID string) {
	if !m.config.Enabled {
		return
	}

	m.mu.Lock()
	watch, exists := m.watches[deploymentID]
	if !exists {
		m.mu.Unlock()
		return
	}
	watch.failures++
	failures := watch.failures
	serviceName := watch.serviceName
	m.mu.Unlock()

	// Check if we should trigger rollback
	if failures >= m.config.HealthCheckFailures {
		// Check grace period
		if time.Since(watch.startedAt) < m.config.GracePeriod {
			return
		}

		// Trigger rollback
		m.manager.Rollback(ctx, &RollbackRequest{
			DeploymentID: deploymentID,
			Reason:       RollbackReasonHealthCheck,
			InitiatedBy:  "auto-rollback-monitor",
		})

		// Remove watch
		m.Unwatch(deploymentID)

		// Log (would use logger in production)
		fmt.Printf("Auto-rollback triggered for deployment %s (service: %s) after %d health check failures\n",
			deploymentID, serviceName, failures)
	}
}

// Stop stops the auto-rollback monitor.
func (m *AutoRollbackMonitor) Stop() {
	close(m.stopCh)
}
