// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package deploy provides the Deployment Engine for the Unheaded Kingdom.
// This is THE SWORD - offensive operations, CI/CD, and deployment automation.
//
// The Deployment Engine supports:
//   - Multiple deployment strategies (rolling, blue-green, canary)
//   - Pipeline with stages and hooks
//   - Artifact management
//   - Rollback capability
//   - Health check integration
//   - Traffic shifting for canary deployments
package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"unheaded/pkg/container"
	"unheaded/pkg/events"
	"unheaded/pkg/logger"
	"unheaded/pkg/metrics"
)

// deploymentIDCounter is used for generating unique deployment IDs
var deploymentIDCounter uint64

// Common errors returned by deployment operations.
var (
	ErrDeploymentNotFound    = errors.New("deployment not found")
	ErrDeploymentInProgress  = errors.New("deployment already in progress")
	ErrDeploymentFailed      = errors.New("deployment failed")
	ErrRollbackFailed        = errors.New("rollback failed")
	ErrNoRollbackAvailable   = errors.New("no rollback available")
	ErrInvalidSpec           = errors.New("invalid deployment specification")
	ErrHealthCheckFailed     = errors.New("health check failed")
	ErrStrategyNotSupported  = errors.New("deployment strategy not supported")
	ErrArtifactNotFound      = errors.New("artifact not found")
	ErrPipelineAborted       = errors.New("pipeline aborted")
	ErrTrafficShiftFailed    = errors.New("traffic shift failed")
)

// DeploymentState represents the current state of a deployment.
type DeploymentState string

const (
	StatePending     DeploymentState = "pending"
	StateBuilding    DeploymentState = "building"
	StateValidating  DeploymentState = "validating"
	StateDeploying   DeploymentState = "deploying"
	StateVerifying   DeploymentState = "verifying"
	StatePromoting   DeploymentState = "promoting"
	StateCompleted   DeploymentState = "completed"
	StateFailed      DeploymentState = "failed"
	StateRolledBack  DeploymentState = "rolled_back"
	StateCancelled   DeploymentState = "cancelled"
	StateRunning     DeploymentState = "running"
	StateRollingBack DeploymentState = "rolling_back"

	// Aliases for backward compatibility with tests
	DeploymentStatePending     = StatePending
	DeploymentStateBuilding    = StateBuilding
	DeploymentStateValidating  = StateValidating
	DeploymentStateDeploying   = StateDeploying
	DeploymentStateVerifying   = StateVerifying
	DeploymentStatePromoting   = StatePromoting
	DeploymentStateCompleted   = StateCompleted
	DeploymentStateFailed      = StateFailed
	DeploymentStateRolledBack  = StateRolledBack
	DeploymentStateCancelled   = StateCancelled
	DeploymentStateRunning     = StateRunning
	DeploymentStateRollingBack = StateRollingBack
)

// StrategyType represents the deployment strategy.
type StrategyType string

const (
	StrategyRolling   StrategyType = "rolling"
	StrategyBlueGreen StrategyType = "blue_green"
	StrategyCanary    StrategyType = "canary"
	StrategyRecreate  StrategyType = "recreate"
)

// DeploymentSpec defines the specification for a deployment.
type DeploymentSpec struct {
	// ServiceName is the name of the service being deployed
	ServiceName string `json:"service_name"`

	// Version is the target version to deploy
	Version string `json:"version"`

	// ArtifactRef references the artifact to deploy
	ArtifactRef string `json:"artifact_ref"`

	// Strategy defines the deployment strategy
	Strategy StrategyType `json:"strategy"`

	// StrategyConfig contains strategy-specific configuration
	StrategyConfig *StrategyConfig `json:"strategy_config,omitempty"`

	// Replicas is the desired number of instances
	Replicas int `json:"replicas"`

	// ContainerSpec is the container specification
	ContainerSpec *container.ContainerSpec `json:"container_spec,omitempty"`

	// HealthCheck defines health check configuration
	HealthCheck *HealthCheckSpec `json:"health_check,omitempty"`

	// Timeout is the maximum deployment duration
	Timeout time.Duration `json:"timeout,omitempty"`

	// Labels for the deployment
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations for additional metadata
	Annotations map[string]string `json:"annotations,omitempty"`

	// Environment variables for the deployment
	Environment map[string]string `json:"environment,omitempty"`

	// Hooks define pre/post deployment actions
	Hooks *HookSpec `json:"hooks,omitempty"`

	// AutoRollback enables automatic rollback on failure
	AutoRollback bool `json:"auto_rollback,omitempty"`

	// Owner is the entity initiating the deployment
	Owner string `json:"owner,omitempty"`
}

// StrategyConfig contains strategy-specific configuration.
type StrategyConfig struct {
	// Rolling strategy config
	MaxSurge       int `json:"max_surge,omitempty"`       // Max instances above desired
	MaxUnavailable int `json:"max_unavailable,omitempty"` // Max unavailable instances

	// Blue-green strategy config
	SwitchTimeout time.Duration `json:"switch_timeout,omitempty"` // Timeout for traffic switch

	// Canary strategy config
	CanarySteps    []CanaryStep  `json:"canary_steps,omitempty"`    // Traffic shift steps
	CanaryAnalysis *CanaryAnalysis `json:"canary_analysis,omitempty"` // Analysis config
}

// CanaryStep defines a canary traffic shift step.
type CanaryStep struct {
	Weight   int           `json:"weight"`   // Traffic percentage (0-100)
	Duration time.Duration `json:"duration"` // Time to hold at this weight
}

// CanaryAnalysis defines canary analysis configuration.
type CanaryAnalysis struct {
	// ErrorRateThreshold is the max error rate (0-1)
	ErrorRateThreshold float64 `json:"error_rate_threshold"`

	// LatencyThreshold is the max p99 latency
	LatencyThreshold time.Duration `json:"latency_threshold"`

	// MinRequests is minimum requests before analysis
	MinRequests int `json:"min_requests"`
}

// HealthCheckSpec defines deployment health check configuration.
type HealthCheckSpec struct {
	// Type is the health check type (http, tcp, exec)
	Type string `json:"type"`

	// Target is the health check endpoint or command
	Target string `json:"target"`

	// Interval between health checks
	Interval time.Duration `json:"interval,omitempty"`

	// Timeout for each health check
	Timeout time.Duration `json:"timeout,omitempty"`

	// SuccessThreshold is consecutive successes for healthy
	SuccessThreshold int `json:"success_threshold,omitempty"`

	// FailureThreshold is consecutive failures for unhealthy
	FailureThreshold int `json:"failure_threshold,omitempty"`

	// InitialDelay before starting health checks
	InitialDelay time.Duration `json:"initial_delay,omitempty"`
}

// HookSpec defines pre/post deployment hooks.
type HookSpec struct {
	PreBuild   []HookAction `json:"pre_build,omitempty"`
	PostBuild  []HookAction `json:"post_build,omitempty"`
	PreDeploy  []HookAction `json:"pre_deploy,omitempty"`
	PostDeploy []HookAction `json:"post_deploy,omitempty"`
	PreVerify  []HookAction `json:"pre_verify,omitempty"`
	PostVerify []HookAction `json:"post_verify,omitempty"`
}

// HookAction defines a single hook action.
type HookAction struct {
	Name    string            `json:"name"`
	Type    string            `json:"type"` // exec, http, wotan
	Command string            `json:"command,omitempty"`
	URL     string            `json:"url,omitempty"`
	Topic   string            `json:"topic,omitempty"`
	Payload map[string]string `json:"payload,omitempty"`
	Timeout time.Duration     `json:"timeout,omitempty"`
	OnError string            `json:"on_error,omitempty"` // continue, abort
}

// Deployment represents an active or completed deployment.
type Deployment struct {
	// ID is the unique deployment identifier
	ID string `json:"id"`

	// ServiceName is the name of the service being deployed (convenience field)
	ServiceName string `json:"service_name,omitempty"`

	// Version is the target version (convenience field)
	Version string `json:"version,omitempty"`

	// Spec is the deployment specification
	Spec *DeploymentSpec `json:"spec"`

	// State is the current deployment state
	State DeploymentState `json:"state"`

	// Message provides additional state context
	Message string `json:"message,omitempty"`

	// PreviousVersion is the version before this deployment
	PreviousVersion string `json:"previous_version,omitempty"`

	// StartedAt is when the deployment started
	StartedAt time.Time `json:"started_at"`

	// CompletedAt is when the deployment completed (if completed)
	CompletedAt time.Time `json:"completed_at,omitempty"`

	// Duration is the total deployment duration
	Duration time.Duration `json:"duration,omitempty"`

	// Stages contains stage execution details
	Stages []StageResult `json:"stages,omitempty"`

	// Metrics contains deployment metrics
	Metrics *DeploymentMetrics `json:"metrics,omitempty"`

	// RollbackID references a rollback deployment if this was rolled back
	RollbackID string `json:"rollback_id,omitempty"`

	// Error contains error details if deployment failed
	Error string `json:"error,omitempty"`
}

// DeploymentStatusInfo provides status information for a deployment.
type DeploymentStatusInfo struct {
	DeploymentID   string          `json:"deployment_id"`
	State          DeploymentState `json:"state"`
	InstancesReady int             `json:"instances_ready"`
	Message        string          `json:"message,omitempty"`
}

// Status returns the current status of the deployment.
func (d *Deployment) Status() *DeploymentStatusInfo {
	instancesReady := 0
	if d.Metrics != nil {
		instancesReady = d.Metrics.HealthChecksPassed
	}
	return &DeploymentStatusInfo{
		DeploymentID:   d.ID,
		State:          d.State,
		InstancesReady: instancesReady,
		Message:        d.Message,
	}
}

// StageResult contains the result of a pipeline stage.
type StageResult struct {
	Name      string          `json:"name"`
	State     DeploymentState `json:"state"`
	StartedAt time.Time       `json:"started_at"`
	EndedAt   time.Time       `json:"ended_at,omitempty"`
	Duration  time.Duration   `json:"duration,omitempty"`
	Message   string          `json:"message,omitempty"`
	Error     string          `json:"error,omitempty"`
}

// DeploymentMetrics contains metrics collected during deployment.
type DeploymentMetrics struct {
	InstancesCreated   int     `json:"instances_created"`
	InstancesDeleted   int     `json:"instances_deleted"`
	InstancesUpdated   int     `json:"instances_updated"`
	InstancesFailed    int     `json:"instances_failed"`
	HealthChecksTotal  int     `json:"health_checks_total"`
	HealthChecksPassed int     `json:"health_checks_passed"`
	HealthChecksFailed int     `json:"health_checks_failed"`
	TrafficShifts      int     `json:"traffic_shifts,omitempty"`
	RollbackCount      int     `json:"rollback_count,omitempty"`
	ErrorRate          float64 `json:"error_rate,omitempty"`
}

// DeploymentStatus provides detailed deployment status.
type DeploymentStatus struct {
	Deployment       *Deployment `json:"deployment"`
	CurrentReplicas  int         `json:"current_replicas"`
	DesiredReplicas  int         `json:"desired_replicas"`
	ReadyReplicas    int         `json:"ready_replicas"`
	UpdatedReplicas  int         `json:"updated_replicas"`
	AvailableReplicas int        `json:"available_replicas"`
	TrafficWeight    int         `json:"traffic_weight,omitempty"`
	Conditions       []Condition `json:"conditions,omitempty"`
}

// Condition represents a deployment condition.
type Condition struct {
	Type    string    `json:"type"`
	Status  string    `json:"status"`
	Reason  string    `json:"reason,omitempty"`
	Message string    `json:"message,omitempty"`
	Updated time.Time `json:"updated"`
}

// Deployer is the main deployment interface.
type Deployer interface {
	// Deploy initiates a deployment
	Deploy(ctx context.Context, spec *DeploymentSpec) (*Deployment, error)

	// Rollback rolls back a deployment
	Rollback(ctx context.Context, deploymentID string) error

	// Status returns the deployment status
	Status(ctx context.Context, deploymentID string) (*DeploymentStatus, error)

	// History returns deployment history for a service
	History(ctx context.Context, service string) ([]*Deployment, error)

	// Cancel cancels an in-progress deployment
	Cancel(ctx context.Context, deploymentID string) error

	// Promote promotes a canary deployment to full traffic
	Promote(ctx context.Context, deploymentID string) error
}

// WotanPublisher interface for publishing deployment events.
type WotanPublisher interface {
	Publish(ctx context.Context, topic string, payload []byte) error
}

// Prometheus metrics for deployments.
var (
	deploymentsTotal = metrics.NewCounterVec(
		"deploy_deployments_total",
		"Total number of deployments",
		nil,
		[]string{"service", "strategy", "status"},
	)

	deploymentDuration = metrics.NewHistogramVec(
		metrics.HistogramOpts{
			Name:    "deploy_deployment_duration_seconds",
			Help:    "Duration of deployments in seconds",
			Buckets: []float64{10, 30, 60, 120, 300, 600, 1200, 3600},
		},
		[]string{"service", "strategy"},
	)

	deploymentsInProgress = metrics.NewGaugeVec(
		"deploy_deployments_in_progress",
		"Number of deployments currently in progress",
		nil,
		[]string{"service"},
	)

	rollbacksTotal = metrics.NewCounterVec(
		"deploy_rollbacks_total",
		"Total number of rollbacks",
		nil,
		[]string{"service", "reason"},
	)
)

// Engine is the deployment engine implementation.
type Engine struct {
	// runtime is the container runtime
	runtime container.Runtime

	// wotan for event publishing
	wotan WotanPublisher

	// logger for logging
	log *logger.Logger

	// deployments stores active deployments
	deployments map[string]*Deployment
	deployMu    sync.RWMutex

	// history stores deployment history by service
	history map[string][]*Deployment
	histMu  sync.RWMutex

	// inProgress tracks deployments in progress per service
	inProgress map[string]string // service -> deploymentID
	progMu     sync.RWMutex

	// config is the engine configuration
	config *EngineConfig
}

// EngineConfig contains engine configuration.
type EngineConfig struct {
	// DefaultTimeout for deployments
	DefaultTimeout time.Duration `json:"default_timeout"`

	// HistoryLimit is max history entries per service
	HistoryLimit int `json:"history_limit"`

	// HealthCheckRetries is number of health check retries
	HealthCheckRetries int `json:"health_check_retries"`

	// DefaultStrategy is the default deployment strategy
	DefaultStrategy StrategyType `json:"default_strategy"`

	// ParallelUpdates is max parallel instance updates
	ParallelUpdates int `json:"parallel_updates"`

	// DefaultReplicas is the default number of replicas
	DefaultReplicas int `json:"default_replicas"`
}

// DefaultEngineConfig returns sensible default configuration.
func DefaultEngineConfig() *EngineConfig {
	return &EngineConfig{
		DefaultTimeout:     30 * time.Minute,
		HistoryLimit:       100,
		HealthCheckRetries: 3,
		DefaultStrategy:    StrategyRolling,
		ParallelUpdates:    5,
	}
}

// NewEngine creates a new deployment engine.
func NewEngine(runtime container.Runtime, wotan WotanPublisher, config *EngineConfig) *Engine {
	if config == nil {
		config = DefaultEngineConfig()
	}

	log := logger.New(nil).With().Str("component", "deploy-engine").Logger()

	return &Engine{
		runtime:     runtime,
		wotan:      wotan,
		log:         log,
		deployments: make(map[string]*Deployment),
		history:     make(map[string][]*Deployment),
		inProgress:  make(map[string]string),
		config:      config,
	}
}

// Deploy initiates a new deployment.
func (e *Engine) Deploy(ctx context.Context, spec *DeploymentSpec) (*Deployment, error) {
	if err := e.validateSpec(spec); err != nil {
		return nil, fmt.Errorf("validate spec: %w", err)
	}

	// Check for existing deployment in progress
	e.progMu.RLock()
	if existingID, ok := e.inProgress[spec.ServiceName]; ok {
		e.progMu.RUnlock()
		return nil, fmt.Errorf("%w: deployment %s already in progress", ErrDeploymentInProgress, existingID)
	}
	e.progMu.RUnlock()

	// Create deployment
	deployment := &Deployment{
		ID:        generateDeploymentID(),
		Spec:      spec,
		State:     StatePending,
		StartedAt: time.Now(),
		Metrics:   &DeploymentMetrics{},
	}

	// Store deployment
	e.deployMu.Lock()
	e.deployments[deployment.ID] = deployment
	e.deployMu.Unlock()

	// Mark as in progress
	e.progMu.Lock()
	e.inProgress[spec.ServiceName] = deployment.ID
	e.progMu.Unlock()

	// Update metrics
	deploymentsInProgress.WithLabels(metrics.Labels{"service": spec.ServiceName}).Inc()

	// Emit deployment started event
	e.emitEvent(ctx, events.TopicStateReconcileStarted, map[string]interface{}{
		"deployment_id": deployment.ID,
		"service":       spec.ServiceName,
		"version":       spec.Version,
		"strategy":      spec.Strategy,
	})

	// Start deployment in background
	go e.runDeployment(context.Background(), deployment)

	return deployment, nil
}

// Rollback rolls back a deployment.
func (e *Engine) Rollback(ctx context.Context, deploymentID string) error {
	e.deployMu.RLock()
	deployment, ok := e.deployments[deploymentID]
	e.deployMu.RUnlock()

	if !ok {
		return ErrDeploymentNotFound
	}

	// Get previous version from history
	e.histMu.RLock()
	history := e.history[deployment.Spec.ServiceName]
	e.histMu.RUnlock()

	if len(history) < 2 {
		return ErrNoRollbackAvailable
	}

	// Find previous successful deployment
	var previousDeployment *Deployment
	for i := len(history) - 2; i >= 0; i-- {
		if history[i].State == StateCompleted {
			previousDeployment = history[i]
			break
		}
	}

	if previousDeployment == nil {
		return ErrNoRollbackAvailable
	}

	e.log.Info().
		Str("deployment_id", deploymentID).
		Str("previous_version", previousDeployment.Spec.Version).
		Msg("initiating rollback")

	// Create rollback deployment spec
	rollbackSpec := &DeploymentSpec{
		ServiceName:    deployment.Spec.ServiceName,
		Version:        previousDeployment.Spec.Version,
		ArtifactRef:    previousDeployment.Spec.ArtifactRef,
		Strategy:       deployment.Spec.Strategy,
		Replicas:       deployment.Spec.Replicas,
		ContainerSpec:  previousDeployment.Spec.ContainerSpec,
		HealthCheck:    deployment.Spec.HealthCheck,
		Timeout:        deployment.Spec.Timeout,
		Owner:          "rollback:" + deployment.ID,
	}

	// Remove in-progress marker to allow rollback deployment
	e.progMu.Lock()
	delete(e.inProgress, deployment.Spec.ServiceName)
	e.progMu.Unlock()

	// Deploy the rollback
	rollbackDeployment, err := e.Deploy(ctx, rollbackSpec)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRollbackFailed, err)
	}

	// Update original deployment with rollback reference
	e.deployMu.Lock()
	deployment.RollbackID = rollbackDeployment.ID
	deployment.State = StateRolledBack
	e.deployMu.Unlock()

	// Update metrics
	rollbacksTotal.WithLabels(metrics.Labels{
		"service": deployment.Spec.ServiceName,
		"reason":  "manual",
	}).Inc()

	return nil
}

// Status returns the deployment status.
func (e *Engine) Status(ctx context.Context, deploymentID string) (*DeploymentStatus, error) {
	e.deployMu.RLock()
	deployment, ok := e.deployments[deploymentID]
	e.deployMu.RUnlock()

	if !ok {
		return nil, ErrDeploymentNotFound
	}

	status := &DeploymentStatus{
		Deployment:      deployment,
		DesiredReplicas: deployment.Spec.Replicas,
	}

	// Get actual replica counts from runtime
	if e.runtime != nil {
		filter := &container.Filter{
			Labels: map[string]string{
				"deployment": deploymentID,
			},
		}
		containers, err := e.runtime.List(ctx, filter)
		if err == nil {
			for _, c := range containers {
				status.CurrentReplicas++
				if c.State == container.StateRunning {
					status.ReadyReplicas++
				}
			}
		}
	}

	// Build conditions
	status.Conditions = e.buildConditions(deployment)

	return status, nil
}

// History returns deployment history for a service.
func (e *Engine) History(ctx context.Context, service string) ([]*Deployment, error) {
	e.histMu.RLock()
	defer e.histMu.RUnlock()

	history, ok := e.history[service]
	if !ok {
		return []*Deployment{}, nil
	}

	// Return a copy to prevent external modification
	result := make([]*Deployment, len(history))
	copy(result, history)
	return result, nil
}

// Cancel cancels an in-progress deployment.
func (e *Engine) Cancel(ctx context.Context, deploymentID string) error {
	e.deployMu.Lock()
	deployment, ok := e.deployments[deploymentID]
	if !ok {
		e.deployMu.Unlock()
		return ErrDeploymentNotFound
	}

	if deployment.State == StateCompleted || deployment.State == StateFailed {
		e.deployMu.Unlock()
		return fmt.Errorf("deployment already completed with state: %s", deployment.State)
	}

	deployment.State = StateCancelled
	deployment.CompletedAt = time.Now()
	deployment.Duration = deployment.CompletedAt.Sub(deployment.StartedAt)
	deployment.Message = "Deployment cancelled by user"
	e.deployMu.Unlock()

	// Remove from in-progress
	e.progMu.Lock()
	delete(e.inProgress, deployment.Spec.ServiceName)
	e.progMu.Unlock()

	// Update metrics
	deploymentsInProgress.WithLabels(metrics.Labels{"service": deployment.Spec.ServiceName}).Dec()

	e.log.Info().
		Str("deployment_id", deploymentID).
		Msg("deployment cancelled")

	return nil
}

// Promote promotes a canary deployment to full traffic.
func (e *Engine) Promote(ctx context.Context, deploymentID string) error {
	e.deployMu.Lock()
	deployment, ok := e.deployments[deploymentID]
	if !ok {
		e.deployMu.Unlock()
		return ErrDeploymentNotFound
	}

	if deployment.Spec.Strategy != StrategyCanary {
		e.deployMu.Unlock()
		return fmt.Errorf("promote only available for canary deployments")
	}

	// Mark for promotion - the deployment goroutine will pick this up
	deployment.State = StatePromoting
	e.deployMu.Unlock()

	e.log.Info().
		Str("deployment_id", deploymentID).
		Msg("canary deployment promoted to full traffic")

	return nil
}

// validateSpec validates a deployment specification.
func (e *Engine) validateSpec(spec *DeploymentSpec) error {
	if spec == nil {
		return ErrInvalidSpec
	}
	if spec.ServiceName == "" {
		return fmt.Errorf("%w: service name is required", ErrInvalidSpec)
	}
	if spec.Version == "" {
		return fmt.Errorf("%w: version is required", ErrInvalidSpec)
	}
	if spec.Replicas < 1 {
		spec.Replicas = 1
	}
	if spec.Strategy == "" {
		spec.Strategy = e.config.DefaultStrategy
	}
	if spec.Timeout == 0 {
		spec.Timeout = e.config.DefaultTimeout
	}
	return nil
}

// runDeployment executes the deployment pipeline.
func (e *Engine) runDeployment(ctx context.Context, deployment *Deployment) {
	defer func() {
		// Clean up in-progress marker
		e.progMu.Lock()
		delete(e.inProgress, deployment.Spec.ServiceName)
		e.progMu.Unlock()

		// Update metrics
		deploymentsInProgress.WithLabels(metrics.Labels{
			"service": deployment.Spec.ServiceName,
		}).Dec()

		// Add to history
		e.addToHistory(deployment)

		// Record final metrics
		deploymentDuration.WithLabels(metrics.Labels{
			"service":  deployment.Spec.ServiceName,
			"strategy": string(deployment.Spec.Strategy),
		}).Observe(deployment.Duration.Seconds())

		deploymentsTotal.WithLabels(metrics.Labels{
			"service":  deployment.Spec.ServiceName,
			"strategy": string(deployment.Spec.Strategy),
			"status":   string(deployment.State),
		}).Inc()
	}()

	// Create timeout context
	ctx, cancel := context.WithTimeout(ctx, deployment.Spec.Timeout)
	defer cancel()

	e.log.Info().
		Str("deployment_id", deployment.ID).
		Str("service", deployment.Spec.ServiceName).
		Str("version", deployment.Spec.Version).
		Str("strategy", string(deployment.Spec.Strategy)).
		Msg("starting deployment")

	// Execute pipeline stages
	stages := []struct {
		name string
		fn   func(context.Context, *Deployment) error
	}{
		{"validate", e.stageValidate},
		{"deploy", e.stageDeploy},
		{"verify", e.stageVerify},
		{"promote", e.stagePromote},
	}

	for _, stage := range stages {
		result := StageResult{
			Name:      stage.name,
			State:     StateDeploying,
			StartedAt: time.Now(),
		}

		e.updateDeploymentState(deployment, DeploymentState("stage_"+stage.name))

		err := stage.fn(ctx, deployment)
		result.EndedAt = time.Now()
		result.Duration = result.EndedAt.Sub(result.StartedAt)

		if err != nil {
			result.State = StateFailed
			result.Error = err.Error()
			deployment.Stages = append(deployment.Stages, result)

			e.handleDeploymentFailure(ctx, deployment, err)
			return
		}

		result.State = StateCompleted
		deployment.Stages = append(deployment.Stages, result)
	}

	// Mark as completed
	e.deployMu.Lock()
	deployment.State = StateCompleted
	deployment.CompletedAt = time.Now()
	deployment.Duration = deployment.CompletedAt.Sub(deployment.StartedAt)
	deployment.Message = "Deployment completed successfully"
	e.deployMu.Unlock()

	e.emitEvent(ctx, events.TopicStateReconcileComplete, map[string]interface{}{
		"deployment_id": deployment.ID,
		"service":       deployment.Spec.ServiceName,
		"version":       deployment.Spec.Version,
		"duration":      deployment.Duration.String(),
	})

	e.log.Info().
		Str("deployment_id", deployment.ID).
		Dur("duration", deployment.Duration).
		Msg("deployment completed successfully")
}

// stageValidate validates the deployment configuration and artifact.
func (e *Engine) stageValidate(ctx context.Context, deployment *Deployment) error {
	e.log.Debug().Str("deployment_id", deployment.ID).Msg("validating deployment")

	// Run pre-deploy hooks
	if deployment.Spec.Hooks != nil {
		for _, hook := range deployment.Spec.Hooks.PreDeploy {
			if err := e.runHook(ctx, hook); err != nil {
				return fmt.Errorf("pre-deploy hook %s failed: %w", hook.Name, err)
			}
		}
	}

	// Validate container spec if provided
	if deployment.Spec.ContainerSpec != nil {
		if err := deployment.Spec.ContainerSpec.Validate(); err != nil {
			return fmt.Errorf("invalid container spec: %w", err)
		}
	}

	return nil
}

// stageDeploy performs the actual deployment based on strategy.
func (e *Engine) stageDeploy(ctx context.Context, deployment *Deployment) error {
	e.log.Debug().
		Str("deployment_id", deployment.ID).
		Str("strategy", string(deployment.Spec.Strategy)).
		Msg("deploying")

	e.updateDeploymentState(deployment, StateDeploying)

	// Strategy execution is handled by strategy package
	// Here we provide a simplified implementation
	switch deployment.Spec.Strategy {
	case StrategyRolling:
		return e.deployRolling(ctx, deployment)
	case StrategyBlueGreen:
		return e.deployBlueGreen(ctx, deployment)
	case StrategyCanary:
		return e.deployCanary(ctx, deployment)
	case StrategyRecreate:
		return e.deployRecreate(ctx, deployment)
	default:
		return ErrStrategyNotSupported
	}
}

// stageVerify verifies the deployment is healthy.
func (e *Engine) stageVerify(ctx context.Context, deployment *Deployment) error {
	e.log.Debug().Str("deployment_id", deployment.ID).Msg("verifying deployment")

	e.updateDeploymentState(deployment, StateVerifying)

	// Run health checks
	if deployment.Spec.HealthCheck != nil {
		for i := 0; i < e.config.HealthCheckRetries; i++ {
			if err := e.runHealthCheck(ctx, deployment); err != nil {
				e.log.Warn().
					Str("deployment_id", deployment.ID).
					Int("attempt", i+1).
					Err(err).
					Msg("health check failed, retrying")

				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(5 * time.Second):
					continue
				}
			}
			deployment.Metrics.HealthChecksPassed++
			return nil
		}
		return ErrHealthCheckFailed
	}

	// Run post-deploy hooks
	if deployment.Spec.Hooks != nil {
		for _, hook := range deployment.Spec.Hooks.PostDeploy {
			if err := e.runHook(ctx, hook); err != nil {
				return fmt.Errorf("post-deploy hook %s failed: %w", hook.Name, err)
			}
		}
	}

	return nil
}

// stagePromote promotes the deployment to full traffic.
func (e *Engine) stagePromote(ctx context.Context, deployment *Deployment) error {
	e.log.Debug().Str("deployment_id", deployment.ID).Msg("promoting deployment")

	e.updateDeploymentState(deployment, StatePromoting)

	// For canary, shift traffic to 100%
	if deployment.Spec.Strategy == StrategyCanary {
		deployment.Metrics.TrafficShifts++
	}

	return nil
}

// deployRolling performs a rolling update deployment.
func (e *Engine) deployRolling(ctx context.Context, deployment *Deployment) error {
	// Simplified rolling update - actual implementation would interact with runtime
	maxSurge := 1
	if deployment.Spec.StrategyConfig != nil && deployment.Spec.StrategyConfig.MaxSurge > 0 {
		maxSurge = deployment.Spec.StrategyConfig.MaxSurge
	}

	e.log.Info().
		Str("deployment_id", deployment.ID).
		Int("replicas", deployment.Spec.Replicas).
		Int("max_surge", maxSurge).
		Msg("performing rolling update")

	for i := 0; i < deployment.Spec.Replicas; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Simulate instance update
			deployment.Metrics.InstancesUpdated++
			time.Sleep(100 * time.Millisecond) // Simulated delay
		}
	}

	return nil
}

// deployBlueGreen performs a blue-green deployment.
func (e *Engine) deployBlueGreen(ctx context.Context, deployment *Deployment) error {
	e.log.Info().
		Str("deployment_id", deployment.ID).
		Int("replicas", deployment.Spec.Replicas).
		Msg("performing blue-green deployment")

	// 1. Deploy green environment
	for i := 0; i < deployment.Spec.Replicas; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			deployment.Metrics.InstancesUpdated++
		}
	}

	// 2. Switch traffic (instant cutover)
	deployment.Metrics.TrafficShifts++

	return nil
}

// deployCanary performs a canary deployment.
func (e *Engine) deployCanary(ctx context.Context, deployment *Deployment) error {
	steps := []CanaryStep{
		{Weight: 1, Duration: 30 * time.Second},
		{Weight: 10, Duration: 60 * time.Second},
		{Weight: 50, Duration: 120 * time.Second},
		{Weight: 100, Duration: 0},
	}

	if deployment.Spec.StrategyConfig != nil && len(deployment.Spec.StrategyConfig.CanarySteps) > 0 {
		steps = deployment.Spec.StrategyConfig.CanarySteps
	}

	e.log.Info().
		Str("deployment_id", deployment.ID).
		Int("steps", len(steps)).
		Msg("performing canary deployment")

	for _, step := range steps {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			e.log.Debug().
				Str("deployment_id", deployment.ID).
				Int("weight", step.Weight).
				Msg("shifting traffic")

			deployment.Metrics.TrafficShifts++

			// Check if promotion was requested
			e.deployMu.RLock()
			if deployment.State == StatePromoting {
				e.deployMu.RUnlock()
				return nil // Skip remaining steps
			}
			e.deployMu.RUnlock()

			if step.Duration > 0 {
				time.Sleep(step.Duration)
			}
		}
	}

	return nil
}

// deployRecreate performs a recreate deployment (stop all, then start all).
func (e *Engine) deployRecreate(ctx context.Context, deployment *Deployment) error {
	e.log.Info().
		Str("deployment_id", deployment.ID).
		Int("replicas", deployment.Spec.Replicas).
		Msg("performing recreate deployment")

	// 1. Stop all instances
	// 2. Start all new instances
	for i := 0; i < deployment.Spec.Replicas; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			deployment.Metrics.InstancesUpdated++
		}
	}

	return nil
}

// runHealthCheck runs health checks on deployed instances.
func (e *Engine) runHealthCheck(ctx context.Context, deployment *Deployment) error {
	deployment.Metrics.HealthChecksTotal++

	// Simplified health check - actual implementation would use health package
	if deployment.Spec.HealthCheck == nil {
		return nil
	}

	// Wait for initial delay
	if deployment.Spec.HealthCheck.InitialDelay > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(deployment.Spec.HealthCheck.InitialDelay):
		}
	}

	// Run health check
	// In real implementation, this would use the health package
	return nil
}

// runHook executes a deployment hook.
func (e *Engine) runHook(ctx context.Context, hook HookAction) error {
	timeout := hook.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	e.log.Debug().
		Str("hook", hook.Name).
		Str("type", hook.Type).
		Msg("running hook")

	switch hook.Type {
	case "exec":
		// Execute command hook - would use exec package
		return nil
	case "http":
		// Execute HTTP hook
		return nil
	case "wotan":
		// Publish to Wotan topic
		if e.wotan == nil {
			e.log.Warn().Str("topic", hook.Topic).Msg("wotan unavailable — hook event dropped (degraded mode)")
			return nil
		}
		payload, _ := json.Marshal(hook.Payload)
		return e.wotan.Publish(ctx, hook.Topic, payload)
	default:
		return fmt.Errorf("unknown hook type: %s", hook.Type)
	}
}

// handleDeploymentFailure handles a deployment failure.
func (e *Engine) handleDeploymentFailure(ctx context.Context, deployment *Deployment, err error) {
	e.deployMu.Lock()
	deployment.State = StateFailed
	deployment.CompletedAt = time.Now()
	deployment.Duration = deployment.CompletedAt.Sub(deployment.StartedAt)
	deployment.Error = err.Error()
	deployment.Message = fmt.Sprintf("Deployment failed: %v", err)
	e.deployMu.Unlock()

	e.log.Error().
		Str("deployment_id", deployment.ID).
		Err(err).
		Msg("deployment failed")

	// Emit failure event
	e.emitEvent(ctx, events.TopicAlertsCritical, map[string]interface{}{
		"deployment_id": deployment.ID,
		"service":       deployment.Spec.ServiceName,
		"error":         err.Error(),
	})

	// Auto-rollback if enabled
	if deployment.Spec.AutoRollback {
		e.log.Info().
			Str("deployment_id", deployment.ID).
			Msg("initiating auto-rollback")

		if rollbackErr := e.Rollback(ctx, deployment.ID); rollbackErr != nil {
			e.log.Error().
				Str("deployment_id", deployment.ID).
				Err(rollbackErr).
				Msg("auto-rollback failed")
		}
	}
}

// updateDeploymentState updates the deployment state.
func (e *Engine) updateDeploymentState(deployment *Deployment, state DeploymentState) {
	e.deployMu.Lock()
	deployment.State = state
	e.deployMu.Unlock()
}

// addToHistory adds a deployment to history.
func (e *Engine) addToHistory(deployment *Deployment) {
	e.histMu.Lock()
	defer e.histMu.Unlock()

	history := e.history[deployment.Spec.ServiceName]
	history = append(history, deployment)

	// Trim to limit
	if len(history) > e.config.HistoryLimit {
		history = history[len(history)-e.config.HistoryLimit:]
	}

	e.history[deployment.Spec.ServiceName] = history
}

// buildConditions builds deployment conditions.
func (e *Engine) buildConditions(deployment *Deployment) []Condition {
	conditions := []Condition{}

	switch deployment.State {
	case StateCompleted:
		conditions = append(conditions, Condition{
			Type:    "Available",
			Status:  "True",
			Reason:  "DeploymentCompleted",
			Message: "Deployment completed successfully",
			Updated: deployment.CompletedAt,
		})
	case StateFailed:
		conditions = append(conditions, Condition{
			Type:    "Available",
			Status:  "False",
			Reason:  "DeploymentFailed",
			Message: deployment.Error,
			Updated: deployment.CompletedAt,
		})
	case StateDeploying, StateVerifying, StatePromoting:
		conditions = append(conditions, Condition{
			Type:    "Progressing",
			Status:  "True",
			Reason:  string(deployment.State),
			Message: "Deployment in progress",
			Updated: time.Now(),
		})
	}

	return conditions
}

// emitEvent emits a deployment event.
func (e *Engine) emitEvent(ctx context.Context, topic string, data map[string]interface{}) {
	if e.wotan == nil {
		e.log.Warn().Str("topic", topic).Msg("wotan unavailable — deploy event dropped (degraded mode)")
		return
	}

	payload, err := json.Marshal(data)
	if err != nil {
		e.log.Error().Err(err).Msg("failed to marshal event")
		return
	}

	if err := e.wotan.Publish(ctx, topic, payload); err != nil {
		e.log.Error().Err(err).Str("topic", topic).Msg("failed to publish event")
	}
}

// generateDeploymentID generates a unique deployment ID.
// It uses an atomic counter combined with the timestamp to ensure uniqueness
// even when called concurrently.
func generateDeploymentID() string {
	counter := atomic.AddUint64(&deploymentIDCounter, 1)
	return fmt.Sprintf("deploy-%d-%d", time.Now().UnixNano(), counter)
}

// ValidateSpec validates a deployment specification.
func ValidateSpec(spec *DeploymentSpec) error {
	if spec == nil {
		return ErrInvalidSpec
	}
	if spec.ServiceName == "" {
		return fmt.Errorf("%w: service name is required", ErrInvalidSpec)
	}
	if spec.Version == "" {
		return fmt.Errorf("%w: version is required", ErrInvalidSpec)
	}
	return nil
}
