// Package pipeline provides the deployment pipeline for orchestrating deployment stages.
package pipeline

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Strategy-related errors.
var (
	ErrStrategyNotConfigured = errors.New("deployment strategy not configured")
	ErrStrategyExecutionFail = errors.New("strategy execution failed")
	ErrTrafficShiftFailed    = errors.New("traffic shift failed")
	ErrAnalysisFailed        = errors.New("canary analysis failed")
	ErrPromotionFailed       = errors.New("promotion failed")
	ErrHealthGateFailed      = errors.New("health gate failed")
)

// DeploymentStrategy represents a deployment strategy type.
type DeploymentStrategy string

const (
	StrategyRolling   DeploymentStrategy = "rolling"
	StrategyBlueGreen DeploymentStrategy = "blue_green"
	StrategyCanary    DeploymentStrategy = "canary"
	StrategyRecreate  DeploymentStrategy = "recreate"
)

// StrategyConfig contains comprehensive strategy configuration.
type StrategyConfig struct {
	// Type is the deployment strategy type
	Type DeploymentStrategy `json:"type"`

	// Rolling strategy settings
	Rolling *RollingConfig `json:"rolling,omitempty"`

	// BlueGreen strategy settings
	BlueGreen *BlueGreenConfig `json:"blue_green,omitempty"`

	// Canary strategy settings
	Canary *CanaryConfig `json:"canary,omitempty"`

	// HealthGate configuration
	HealthGate *HealthGateConfig `json:"health_gate,omitempty"`

	// Timeout for entire strategy execution
	Timeout time.Duration `json:"timeout,omitempty"`

	// Notifications for strategy events
	Notifications *NotificationConfig `json:"notifications,omitempty"`
}

// RollingConfig contains rolling deployment configuration.
type RollingConfig struct {
	// MaxSurge is the maximum number of pods above desired
	MaxSurge int `json:"max_surge"`

	// MaxUnavailable is the maximum unavailable pods
	MaxUnavailable int `json:"max_unavailable"`

	// ProgressDeadline is the time to wait for progress
	ProgressDeadline time.Duration `json:"progress_deadline,omitempty"`

	// MinReadySeconds is minimum time a pod should be ready
	MinReadySeconds int `json:"min_ready_seconds,omitempty"`

	// BatchSize for batch updates (0 = automatic)
	BatchSize int `json:"batch_size,omitempty"`

	// PauseAfterBatch pauses after each batch for manual approval
	PauseAfterBatch bool `json:"pause_after_batch,omitempty"`
}

// BlueGreenConfig contains blue-green deployment configuration.
type BlueGreenConfig struct {
	// SwitchTimeout is the timeout for traffic switch
	SwitchTimeout time.Duration `json:"switch_timeout,omitempty"`

	// TestDuration is how long to test green before switching
	TestDuration time.Duration `json:"test_duration,omitempty"`

	// PreviewEndpoint for testing green environment
	PreviewEndpoint string `json:"preview_endpoint,omitempty"`

	// AutoPromote automatically promotes after successful test
	AutoPromote bool `json:"auto_promote,omitempty"`

	// KeepBlueInstances keeps blue instances after switch
	KeepBlueInstances bool `json:"keep_blue_instances,omitempty"`

	// KeepBlueDuration is how long to keep blue after switch
	KeepBlueDuration time.Duration `json:"keep_blue_duration,omitempty"`
}

// CanaryConfig contains canary deployment configuration.
type CanaryConfig struct {
	// Steps defines the traffic shift steps
	Steps []CanaryStepConfig `json:"steps,omitempty"`

	// Analysis configuration for canary analysis
	Analysis *CanaryAnalysisConfig `json:"analysis,omitempty"`

	// AutoPromote automatically promotes on analysis success
	AutoPromote bool `json:"auto_promote,omitempty"`

	// PromoteTimeout is the timeout for manual promotion
	PromoteTimeout time.Duration `json:"promote_timeout,omitempty"`

	// AbortOnFailure automatically aborts on analysis failure
	AbortOnFailure bool `json:"abort_on_failure,omitempty"`

	// CanaryReplicas is the number of canary replicas
	CanaryReplicas int `json:"canary_replicas,omitempty"`
}

// CanaryStepConfig defines a canary traffic shift step.
type CanaryStepConfig struct {
	// Weight is the traffic percentage (0-100)
	Weight int `json:"weight"`

	// Duration is how long to hold at this weight
	Duration time.Duration `json:"duration,omitempty"`

	// Analysis enables analysis during this step
	Analysis bool `json:"analysis,omitempty"`

	// Pause for manual approval
	Pause bool `json:"pause,omitempty"`
}

// CanaryAnalysisConfig defines canary analysis parameters.
type CanaryAnalysisConfig struct {
	// Interval between analysis runs
	Interval time.Duration `json:"interval,omitempty"`

	// ErrorRateThreshold is max allowed error rate (0-1)
	ErrorRateThreshold float64 `json:"error_rate_threshold,omitempty"`

	// LatencyP99Threshold is max p99 latency
	LatencyP99Threshold time.Duration `json:"latency_p99_threshold,omitempty"`

	// LatencyP95Threshold is max p95 latency
	LatencyP95Threshold time.Duration `json:"latency_p95_threshold,omitempty"`

	// SuccessRateThreshold is minimum success rate (0-1)
	SuccessRateThreshold float64 `json:"success_rate_threshold,omitempty"`

	// MinSampleSize is minimum requests for analysis
	MinSampleSize int `json:"min_sample_size,omitempty"`

	// FailureThreshold consecutive failures before abort
	FailureThreshold int `json:"failure_threshold,omitempty"`

	// Queries for custom metrics analysis
	Queries []AnalysisQuery `json:"queries,omitempty"`
}

// AnalysisQuery defines a custom analysis query.
type AnalysisQuery struct {
	// Name is the query name
	Name string `json:"name"`

	// Query is the PromQL or similar query
	Query string `json:"query"`

	// Operator for comparison (>, <, >=, <=, ==)
	Operator string `json:"operator"`

	// Threshold is the comparison value
	Threshold float64 `json:"threshold"`

	// Weight in overall analysis (0-1)
	Weight float64 `json:"weight,omitempty"`
}

// HealthGateConfig defines health gate configuration.
type HealthGateConfig struct {
	// Enabled enables health gate
	Enabled bool `json:"enabled"`

	// InitialDelay before starting health checks
	InitialDelay time.Duration `json:"initial_delay,omitempty"`

	// Timeout for health gate to pass
	Timeout time.Duration `json:"timeout,omitempty"`

	// CheckInterval between health checks
	CheckInterval time.Duration `json:"check_interval,omitempty"`

	// SuccessThreshold consecutive successes required
	SuccessThreshold int `json:"success_threshold,omitempty"`

	// FailureThreshold consecutive failures for abort
	FailureThreshold int `json:"failure_threshold,omitempty"`

	// Endpoints to check for health
	Endpoints []HealthEndpoint `json:"endpoints,omitempty"`

	// CustomChecks for additional health verification
	CustomChecks []CustomHealthCheck `json:"custom_checks,omitempty"`
}

// HealthEndpoint defines a health check endpoint.
type HealthEndpoint struct {
	// Name is the endpoint name
	Name string `json:"name"`

	// URL is the health check URL
	URL string `json:"url"`

	// Method is the HTTP method (GET, POST)
	Method string `json:"method,omitempty"`

	// ExpectedStatus is the expected HTTP status
	ExpectedStatus int `json:"expected_status,omitempty"`

	// ExpectedBody is optional expected body content
	ExpectedBody string `json:"expected_body,omitempty"`

	// Timeout for this endpoint
	Timeout time.Duration `json:"timeout,omitempty"`

	// Headers to send with request
	Headers map[string]string `json:"headers,omitempty"`
}

// CustomHealthCheck defines a custom health check.
type CustomHealthCheck struct {
	// Name is the check name
	Name string `json:"name"`

	// Type is the check type (exec, tcp, grpc)
	Type string `json:"type"`

	// Command for exec type
	Command string `json:"command,omitempty"`

	// Address for tcp/grpc type
	Address string `json:"address,omitempty"`

	// Timeout for the check
	Timeout time.Duration `json:"timeout,omitempty"`
}

// NotificationConfig defines notification settings for strategy events.
type NotificationConfig struct {
	// Enabled enables notifications
	Enabled bool `json:"enabled"`

	// OnStart notifies on strategy start
	OnStart bool `json:"on_start,omitempty"`

	// OnSuccess notifies on success
	OnSuccess bool `json:"on_success,omitempty"`

	// OnFailure notifies on failure
	OnFailure bool `json:"on_failure,omitempty"`

	// OnRollback notifies on rollback
	OnRollback bool `json:"on_rollback,omitempty"`

	// OnPause notifies when paused for approval
	OnPause bool `json:"on_pause,omitempty"`

	// Channels to notify
	Channels []NotificationChannel `json:"channels,omitempty"`
}

// NotificationChannel defines a notification channel.
type NotificationChannel struct {
	// Type is the channel type (webhook, wotan, slack)
	Type string `json:"type"`

	// Target is the channel target (URL, topic, channel name)
	Target string `json:"target"`

	// Template is optional message template
	Template string `json:"template,omitempty"`

	// Severity filters (info, warning, critical)
	Severity []string `json:"severity,omitempty"`
}

// StrategyExecutor executes deployment strategies.
type StrategyExecutor struct {
	mu sync.RWMutex

	// executions tracks active executions
	executions map[string]*StrategyExecution

	// trafficManager manages traffic shifting
	trafficManager TrafficManager

	// healthChecker performs health checks
	healthChecker HealthChecker

	// metricsProvider provides metrics for analysis
	metricsProvider MetricsProvider

	// notifier sends notifications
	notifier Notifier

	// instanceManager manages instances
	instanceManager InstanceManager

	// eventCallback for execution events
	eventCallback func(event *StrategyEvent)
}

// StrategyExecution represents an active strategy execution.
type StrategyExecution struct {
	// ID is the execution ID
	ID string `json:"id"`

	// DeploymentID is the parent deployment
	DeploymentID string `json:"deployment_id"`

	// Config is the strategy configuration
	Config *StrategyConfig `json:"config"`

	// State is the current execution state
	State StrategyExecutionState `json:"state"`

	// CurrentStep for canary deployments
	CurrentStep int `json:"current_step,omitempty"`

	// CurrentWeight for traffic percentage
	CurrentWeight int `json:"current_weight,omitempty"`

	// Progress contains execution progress
	Progress *StrategyProgress `json:"progress"`

	// StartedAt is when execution started
	StartedAt time.Time `json:"started_at"`

	// CompletedAt is when execution completed
	CompletedAt time.Time `json:"completed_at,omitempty"`

	// Error contains error if failed
	Error string `json:"error,omitempty"`

	// AnalysisResults from canary analysis
	AnalysisResults []*AnalysisResult `json:"analysis_results,omitempty"`

	// cancel function for execution
	cancel context.CancelFunc

	// done channel
	done chan struct{}
}

// StrategyExecutionState represents the state of strategy execution.
type StrategyExecutionState string

const (
	ExecutionStatePending    StrategyExecutionState = "pending"
	ExecutionStateRunning    StrategyExecutionState = "running"
	ExecutionStatePaused     StrategyExecutionState = "paused"
	ExecutionStateAnalyzing  StrategyExecutionState = "analyzing"
	ExecutionStatePromoting  StrategyExecutionState = "promoting"
	ExecutionStateCompleted  StrategyExecutionState = "completed"
	ExecutionStateFailed     StrategyExecutionState = "failed"
	ExecutionStateAborted    StrategyExecutionState = "aborted"
	ExecutionStateRollingBack StrategyExecutionState = "rolling_back"
)

// StrategyProgress tracks execution progress.
type StrategyProgress struct {
	// Phase is the current phase
	Phase string `json:"phase"`

	// Message is a human-readable message
	Message string `json:"message"`

	// Percentage is completion percentage (0-100)
	Percentage int `json:"percentage"`

	// InstancesUpdated is count of updated instances
	InstancesUpdated int `json:"instances_updated"`

	// InstancesTotal is total instances
	InstancesTotal int `json:"instances_total"`

	// TrafficWeight for canary/blue-green
	TrafficWeight int `json:"traffic_weight,omitempty"`

	// HealthyInstances count
	HealthyInstances int `json:"healthy_instances"`

	// UnhealthyInstances count
	UnhealthyInstances int `json:"unhealthy_instances"`
}

// AnalysisResult contains canary analysis results.
type AnalysisResult struct {
	// Timestamp of analysis
	Timestamp time.Time `json:"timestamp"`

	// Pass indicates if analysis passed
	Pass bool `json:"pass"`

	// Score is the overall score (0-100)
	Score float64 `json:"score"`

	// Metrics contains the analyzed metrics
	Metrics *AnalysisMetrics `json:"metrics"`

	// Message describes the result
	Message string `json:"message"`

	// QueryResults for custom queries
	QueryResults map[string]float64 `json:"query_results,omitempty"`
}

// AnalysisMetrics contains metrics for analysis.
type AnalysisMetrics struct {
	// ErrorRate is the error rate (0-1)
	ErrorRate float64 `json:"error_rate"`

	// SuccessRate is the success rate (0-1)
	SuccessRate float64 `json:"success_rate"`

	// LatencyP50 is the p50 latency
	LatencyP50 time.Duration `json:"latency_p50"`

	// LatencyP95 is the p95 latency
	LatencyP95 time.Duration `json:"latency_p95"`

	// LatencyP99 is the p99 latency
	LatencyP99 time.Duration `json:"latency_p99"`

	// RequestCount is total requests
	RequestCount int64 `json:"request_count"`

	// ErrorCount is total errors
	ErrorCount int64 `json:"error_count"`

	// SampleSize is the analysis sample size
	SampleSize int `json:"sample_size"`
}

// StrategyEvent represents a strategy execution event.
type StrategyEvent struct {
	// Type is the event type
	Type string `json:"type"`

	// ExecutionID is the execution ID
	ExecutionID string `json:"execution_id"`

	// DeploymentID is the deployment ID
	DeploymentID string `json:"deployment_id"`

	// State is the current state
	State StrategyExecutionState `json:"state"`

	// Progress is the current progress
	Progress *StrategyProgress `json:"progress,omitempty"`

	// Message is a human-readable message
	Message string `json:"message,omitempty"`

	// Timestamp of the event
	Timestamp time.Time `json:"timestamp"`

	// Data contains additional event data
	Data map[string]interface{} `json:"data,omitempty"`
}

// TrafficManager interface for managing traffic.
type TrafficManager interface {
	// ShiftTraffic shifts traffic between versions
	ShiftTraffic(ctx context.Context, service, fromVersion, toVersion string, weight int) error

	// GetTrafficWeight returns current traffic weight for a version
	GetTrafficWeight(ctx context.Context, service, version string) (int, error)
}

// HealthChecker interface for health checks.
type HealthChecker interface {
	// CheckHealth checks health of instances
	CheckHealth(ctx context.Context, service string, endpoints []HealthEndpoint) (*HealthResult, error)

	// CheckCustom runs custom health checks
	CheckCustom(ctx context.Context, check *CustomHealthCheck) error
}

// HealthResult contains health check results.
type HealthResult struct {
	// Healthy is true if all checks passed
	Healthy bool `json:"healthy"`

	// CheckResults contains individual check results
	CheckResults []HealthCheckResult `json:"check_results"`

	// HealthyCount is count of healthy endpoints
	HealthyCount int `json:"healthy_count"`

	// TotalCount is total endpoints checked
	TotalCount int `json:"total_count"`
}

// HealthCheckResult contains a single health check result.
type HealthCheckResult struct {
	// Name is the endpoint name
	Name string `json:"name"`

	// Healthy is true if check passed
	Healthy bool `json:"healthy"`

	// StatusCode is the HTTP status code
	StatusCode int `json:"status_code,omitempty"`

	// Latency is the check latency
	Latency time.Duration `json:"latency"`

	// Error contains error message if failed
	Error string `json:"error,omitempty"`
}

// MetricsProvider interface for metrics.
type MetricsProvider interface {
	// GetMetrics retrieves metrics for analysis
	GetMetrics(ctx context.Context, service, version string, duration time.Duration) (*AnalysisMetrics, error)

	// ExecuteQuery executes a custom query
	ExecuteQuery(ctx context.Context, query string, params map[string]string) (float64, error)
}

// Notifier interface for notifications.
type Notifier interface {
	// Notify sends a notification
	Notify(ctx context.Context, channel *NotificationChannel, event *StrategyEvent) error
}

// InstanceManager interface for instance management.
type InstanceManager interface {
	// CreateInstances creates new instances
	CreateInstances(ctx context.Context, service, version string, count int, labels map[string]string) error

	// DeleteInstances deletes instances
	DeleteInstances(ctx context.Context, service, version string) error

	// GetInstances returns instance information
	GetInstances(ctx context.Context, service, version string) ([]InstanceInfo, error)

	// UpdateInstance updates an instance
	UpdateInstance(ctx context.Context, instanceID, version string) error
}

// InstanceInfo contains instance information.
type InstanceInfo struct {
	ID      string            `json:"id"`
	Service string            `json:"service"`
	Version string            `json:"version"`
	State   string            `json:"state"`
	Healthy bool              `json:"healthy"`
	Address string            `json:"address"`
	Labels  map[string]string `json:"labels,omitempty"`
}

// NewStrategyExecutor creates a new strategy executor.
func NewStrategyExecutor(
	traffic TrafficManager,
	health HealthChecker,
	metrics MetricsProvider,
	notifier Notifier,
	instances InstanceManager,
) *StrategyExecutor {
	return &StrategyExecutor{
		executions:      make(map[string]*StrategyExecution),
		trafficManager:  traffic,
		healthChecker:   health,
		metricsProvider: metrics,
		notifier:        notifier,
		instanceManager: instances,
	}
}

// SetEventCallback sets the event callback.
func (e *StrategyExecutor) SetEventCallback(callback func(event *StrategyEvent)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.eventCallback = callback
}

// Execute executes a deployment strategy.
func (e *StrategyExecutor) Execute(ctx context.Context, deploymentID string, config *StrategyConfig, params *ExecutionParams) (*StrategyExecution, error) {
	if config == nil {
		return nil, ErrStrategyNotConfigured
	}

	ctx, cancel := context.WithCancel(ctx)
	if config.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, config.Timeout)
	}

	execution := &StrategyExecution{
		ID:           generateExecutionID(),
		DeploymentID: deploymentID,
		Config:       config,
		State:        ExecutionStatePending,
		Progress: &StrategyProgress{
			Phase:          "initializing",
			Message:        "Starting deployment",
			InstancesTotal: params.Replicas,
		},
		StartedAt:       time.Now(),
		AnalysisResults: []*AnalysisResult{},
		cancel:          cancel,
		done:            make(chan struct{}),
	}

	e.mu.Lock()
	e.executions[execution.ID] = execution
	e.mu.Unlock()

	// Start execution
	go e.executeStrategy(ctx, execution, params)

	return execution, nil
}

// ExecutionParams contains parameters for strategy execution.
type ExecutionParams struct {
	// ServiceName is the service being deployed
	ServiceName string

	// CurrentVersion is the current running version
	CurrentVersion string

	// TargetVersion is the version being deployed
	TargetVersion string

	// Replicas is the desired replica count
	Replicas int

	// ArtifactRef is the artifact reference
	ArtifactRef string
}

// executeStrategy runs the strategy execution.
func (e *StrategyExecutor) executeStrategy(ctx context.Context, execution *StrategyExecution, params *ExecutionParams) {
	defer func() {
		close(execution.done)
		e.mu.Lock()
		delete(e.executions, execution.ID)
		e.mu.Unlock()
	}()

	e.updateState(execution, ExecutionStateRunning, "Executing deployment strategy")
	e.emitEvent(execution, "strategy_started", nil)

	// Notify on start
	e.sendNotifications(ctx, execution, "start")

	var err error
	switch execution.Config.Type {
	case StrategyRolling:
		err = e.executeRolling(ctx, execution, params)
	case StrategyBlueGreen:
		err = e.executeBlueGreen(ctx, execution, params)
	case StrategyCanary:
		err = e.executeCanary(ctx, execution, params)
	case StrategyRecreate:
		err = e.executeRecreate(ctx, execution, params)
	default:
		err = fmt.Errorf("unsupported strategy type: %s", execution.Config.Type)
	}

	if err != nil {
		e.failExecution(execution, err)
		e.sendNotifications(ctx, execution, "failure")
		return
	}

	// Run health gate if configured
	if execution.Config.HealthGate != nil && execution.Config.HealthGate.Enabled {
		if err := e.runHealthGate(ctx, execution, params); err != nil {
			e.failExecution(execution, err)
			e.sendNotifications(ctx, execution, "failure")
			return
		}
	}

	// Complete execution
	e.mu.Lock()
	execution.State = ExecutionStateCompleted
	execution.CompletedAt = time.Now()
	execution.Progress.Phase = "completed"
	execution.Progress.Message = "Deployment completed successfully"
	execution.Progress.Percentage = 100
	e.mu.Unlock()

	e.emitEvent(execution, "strategy_completed", nil)
	e.sendNotifications(ctx, execution, "success")
}

// executeRolling executes a rolling deployment strategy.
func (e *StrategyExecutor) executeRolling(ctx context.Context, execution *StrategyExecution, params *ExecutionParams) error {
	config := execution.Config.Rolling
	if config == nil {
		config = &RollingConfig{MaxSurge: 1, MaxUnavailable: 0}
	}

	execution.Progress.Phase = "rolling_update"
	execution.Progress.Message = "Performing rolling update"

	batchSize := config.BatchSize
	if batchSize == 0 {
		batchSize = config.MaxSurge
		if batchSize == 0 {
			batchSize = 1
		}
	}

	updated := 0
	for updated < params.Replicas {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Update a batch
		toUpdate := batchSize
		if updated+toUpdate > params.Replicas {
			toUpdate = params.Replicas - updated
		}

		// Create new instances
		if e.instanceManager != nil {
			err := e.instanceManager.CreateInstances(ctx, params.ServiceName, params.TargetVersion, toUpdate, map[string]string{
				"deployment": execution.DeploymentID,
			})
			if err != nil {
				return fmt.Errorf("failed to create instances: %w", err)
			}
		}

		updated += toUpdate

		// Update progress
		e.mu.Lock()
		execution.Progress.InstancesUpdated = updated
		execution.Progress.Percentage = (updated * 100) / params.Replicas
		execution.Progress.Message = fmt.Sprintf("Rolling update: %d/%d instances updated", updated, params.Replicas)
		e.mu.Unlock()

		e.emitEvent(execution, "progress", map[string]interface{}{
			"updated": updated,
			"total":   params.Replicas,
		})

		// Pause after batch if configured
		if config.PauseAfterBatch && updated < params.Replicas {
			e.updateState(execution, ExecutionStatePaused, "Paused for approval")
			e.sendNotifications(ctx, execution, "pause")
			// Wait for resume (in production, this would be a channel wait)
			time.Sleep(100 * time.Millisecond)
			e.updateState(execution, ExecutionStateRunning, "Resumed rolling update")
		}

		// Small delay between batches
		time.Sleep(100 * time.Millisecond)
	}

	return nil
}

// executeBlueGreen executes a blue-green deployment strategy.
func (e *StrategyExecutor) executeBlueGreen(ctx context.Context, execution *StrategyExecution, params *ExecutionParams) error {
	config := execution.Config.BlueGreen
	if config == nil {
		config = &BlueGreenConfig{}
	}

	// Phase 1: Deploy green environment
	execution.Progress.Phase = "deploy_green"
	execution.Progress.Message = "Deploying green environment"

	if e.instanceManager != nil {
		err := e.instanceManager.CreateInstances(ctx, params.ServiceName, params.TargetVersion, params.Replicas, map[string]string{
			"deployment":  execution.DeploymentID,
			"environment": "green",
		})
		if err != nil {
			return fmt.Errorf("failed to deploy green: %w", err)
		}
	}

	execution.Progress.InstancesUpdated = params.Replicas
	execution.Progress.Percentage = 30

	// Phase 2: Wait for green to be healthy
	execution.Progress.Phase = "verify_green"
	execution.Progress.Message = "Verifying green environment health"

	if e.healthChecker != nil && execution.Config.HealthGate != nil {
		result, err := e.healthChecker.CheckHealth(ctx, params.ServiceName, execution.Config.HealthGate.Endpoints)
		if err != nil || !result.Healthy {
			return fmt.Errorf("green environment not healthy: %v", err)
		}
	}

	execution.Progress.Percentage = 50

	// Phase 3: Test green (optional)
	if config.TestDuration > 0 {
		execution.Progress.Phase = "test_green"
		execution.Progress.Message = "Testing green environment"

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(config.TestDuration):
		}
	}

	execution.Progress.Percentage = 70

	// Phase 4: Switch traffic
	execution.Progress.Phase = "switch_traffic"
	execution.Progress.Message = "Switching traffic to green"

	if e.trafficManager != nil {
		err := e.trafficManager.ShiftTraffic(ctx, params.ServiceName, params.CurrentVersion, params.TargetVersion, 100)
		if err != nil {
			return fmt.Errorf("failed to switch traffic: %w", err)
		}
	}

	execution.CurrentWeight = 100
	execution.Progress.TrafficWeight = 100
	execution.Progress.Percentage = 85

	// Phase 5: Terminate blue (unless keeping)
	if !config.KeepBlueInstances {
		execution.Progress.Phase = "terminate_blue"
		execution.Progress.Message = "Terminating blue environment"

		if e.instanceManager != nil {
			e.instanceManager.DeleteInstances(ctx, params.ServiceName, params.CurrentVersion)
		}
	}

	execution.Progress.Percentage = 100
	return nil
}

// executeCanary executes a canary deployment strategy.
func (e *StrategyExecutor) executeCanary(ctx context.Context, execution *StrategyExecution, params *ExecutionParams) error {
	config := execution.Config.Canary
	if config == nil {
		config = &CanaryConfig{}
	}

	// Default steps if not configured
	steps := config.Steps
	if len(steps) == 0 {
		steps = []CanaryStepConfig{
			{Weight: 1, Duration: time.Minute, Analysis: true},
			{Weight: 10, Duration: 5 * time.Minute, Analysis: true},
			{Weight: 50, Duration: 10 * time.Minute, Analysis: true},
			{Weight: 100, Duration: 0, Analysis: false},
		}
	}

	// Phase 1: Deploy canary instances
	canaryReplicas := config.CanaryReplicas
	if canaryReplicas == 0 {
		canaryReplicas = 1
	}

	execution.Progress.Phase = "deploy_canary"
	execution.Progress.Message = "Deploying canary instances"

	if e.instanceManager != nil {
		err := e.instanceManager.CreateInstances(ctx, params.ServiceName, params.TargetVersion, canaryReplicas, map[string]string{
			"deployment": execution.DeploymentID,
			"role":       "canary",
		})
		if err != nil {
			return fmt.Errorf("failed to deploy canary: %w", err)
		}
	}

	execution.Progress.InstancesUpdated = canaryReplicas

	// Phase 2: Execute canary steps
	for stepIdx, step := range steps {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		execution.CurrentStep = stepIdx
		execution.CurrentWeight = step.Weight
		execution.Progress.Phase = fmt.Sprintf("canary_step_%d", stepIdx+1)
		execution.Progress.Message = fmt.Sprintf("Canary step %d: %d%% traffic", stepIdx+1, step.Weight)
		execution.Progress.TrafficWeight = step.Weight
		execution.Progress.Percentage = (stepIdx * 100) / len(steps)

		// Shift traffic
		if e.trafficManager != nil {
			err := e.trafficManager.ShiftTraffic(ctx, params.ServiceName, params.CurrentVersion, params.TargetVersion, step.Weight)
			if err != nil {
				return fmt.Errorf("failed to shift traffic to %d%%: %w", step.Weight, err)
			}
		}

		e.emitEvent(execution, "traffic_shifted", map[string]interface{}{
			"step":   stepIdx + 1,
			"weight": step.Weight,
		})

		// Run analysis if enabled
		if step.Analysis && config.Analysis != nil {
			e.updateState(execution, ExecutionStateAnalyzing, fmt.Sprintf("Analyzing at %d%% traffic", step.Weight))

			result, err := e.runAnalysis(ctx, execution, params, config.Analysis)
			if err != nil {
				return fmt.Errorf("analysis failed: %w", err)
			}

			execution.AnalysisResults = append(execution.AnalysisResults, result)

			if !result.Pass {
				if config.AbortOnFailure {
					// Rollback traffic
					if e.trafficManager != nil {
						e.trafficManager.ShiftTraffic(ctx, params.ServiceName, params.TargetVersion, params.CurrentVersion, 100)
					}
					return fmt.Errorf("%w: %s", ErrAnalysisFailed, result.Message)
				}
			}

			e.updateState(execution, ExecutionStateRunning, "Continuing canary deployment")
		}

		// Pause if configured
		if step.Pause {
			e.updateState(execution, ExecutionStatePaused, "Paused for approval")
			e.sendNotifications(ctx, execution, "pause")
			// In production, would wait for approval
			time.Sleep(100 * time.Millisecond)
			e.updateState(execution, ExecutionStateRunning, "Resumed canary deployment")
		}

		// Hold at this weight
		if step.Duration > 0 && step.Weight < 100 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(step.Duration):
			}
		}
	}

	// Phase 3: Full promotion
	execution.Progress.Phase = "promoting"
	execution.Progress.Message = "Promoting canary to production"
	e.updateState(execution, ExecutionStatePromoting, "Promoting to full traffic")

	// Scale canary to full replicas
	if e.instanceManager != nil {
		for execution.Progress.InstancesUpdated < params.Replicas {
			remaining := params.Replicas - execution.Progress.InstancesUpdated
			e.instanceManager.CreateInstances(ctx, params.ServiceName, params.TargetVersion, remaining, map[string]string{
				"deployment": execution.DeploymentID,
				"role":       "stable",
			})
			execution.Progress.InstancesUpdated = params.Replicas
		}
	}

	// Remove old instances
	if e.instanceManager != nil {
		e.instanceManager.DeleteInstances(ctx, params.ServiceName, params.CurrentVersion)
	}

	execution.Progress.Percentage = 100
	return nil
}

// executeRecreate executes a recreate deployment strategy.
func (e *StrategyExecutor) executeRecreate(ctx context.Context, execution *StrategyExecution, params *ExecutionParams) error {
	// Phase 1: Stop all old instances
	execution.Progress.Phase = "stopping"
	execution.Progress.Message = "Stopping old instances"

	if e.instanceManager != nil {
		e.instanceManager.DeleteInstances(ctx, params.ServiceName, params.CurrentVersion)
	}

	execution.Progress.Percentage = 30

	// Phase 2: Start new instances
	execution.Progress.Phase = "starting"
	execution.Progress.Message = "Starting new instances"

	if e.instanceManager != nil {
		err := e.instanceManager.CreateInstances(ctx, params.ServiceName, params.TargetVersion, params.Replicas, map[string]string{
			"deployment": execution.DeploymentID,
		})
		if err != nil {
			return fmt.Errorf("failed to create instances: %w", err)
		}
	}

	execution.Progress.InstancesUpdated = params.Replicas
	execution.Progress.Percentage = 100

	return nil
}

// runAnalysis runs canary analysis.
func (e *StrategyExecutor) runAnalysis(ctx context.Context, execution *StrategyExecution, params *ExecutionParams, config *CanaryAnalysisConfig) (*AnalysisResult, error) {
	result := &AnalysisResult{
		Timestamp:    time.Now(),
		QueryResults: make(map[string]float64),
	}

	// Get metrics
	if e.metricsProvider == nil {
		result.Pass = true
		result.Message = "No metrics provider, assuming pass"
		return result, nil
	}

	interval := config.Interval
	if interval == 0 {
		interval = 30 * time.Second
	}

	metrics, err := e.metricsProvider.GetMetrics(ctx, params.ServiceName, params.TargetVersion, interval)
	if err != nil {
		return nil, fmt.Errorf("failed to get metrics: %w", err)
	}

	result.Metrics = metrics

	// Check sample size
	if config.MinSampleSize > 0 && metrics.SampleSize < config.MinSampleSize {
		result.Pass = true
		result.Message = fmt.Sprintf("Insufficient sample size: %d < %d", metrics.SampleSize, config.MinSampleSize)
		return result, nil
	}

	// Evaluate thresholds
	score := 100.0
	issues := []string{}

	if config.ErrorRateThreshold > 0 && metrics.ErrorRate > config.ErrorRateThreshold {
		score -= 30
		issues = append(issues, fmt.Sprintf("error rate %.2f%% > %.2f%%", metrics.ErrorRate*100, config.ErrorRateThreshold*100))
	}

	if config.SuccessRateThreshold > 0 && metrics.SuccessRate < config.SuccessRateThreshold {
		score -= 30
		issues = append(issues, fmt.Sprintf("success rate %.2f%% < %.2f%%", metrics.SuccessRate*100, config.SuccessRateThreshold*100))
	}

	if config.LatencyP99Threshold > 0 && metrics.LatencyP99 > config.LatencyP99Threshold {
		score -= 20
		issues = append(issues, fmt.Sprintf("p99 latency %v > %v", metrics.LatencyP99, config.LatencyP99Threshold))
	}

	if config.LatencyP95Threshold > 0 && metrics.LatencyP95 > config.LatencyP95Threshold {
		score -= 20
		issues = append(issues, fmt.Sprintf("p95 latency %v > %v", metrics.LatencyP95, config.LatencyP95Threshold))
	}

	// Execute custom queries
	for _, query := range config.Queries {
		value, err := e.metricsProvider.ExecuteQuery(ctx, query.Query, nil)
		if err != nil {
			continue
		}
		result.QueryResults[query.Name] = value

		passed := false
		switch query.Operator {
		case ">":
			passed = value > query.Threshold
		case ">=":
			passed = value >= query.Threshold
		case "<":
			passed = value < query.Threshold
		case "<=":
			passed = value <= query.Threshold
		case "==":
			passed = value == query.Threshold
		}

		if !passed {
			weight := query.Weight
			if weight == 0 {
				weight = 0.1
			}
			score -= weight * 100
			issues = append(issues, fmt.Sprintf("%s: %v %s %v", query.Name, value, query.Operator, query.Threshold))
		}
	}

	result.Score = score
	result.Pass = score >= 70 // Pass threshold

	if len(issues) > 0 {
		result.Message = fmt.Sprintf("Analysis issues: %v", issues)
	} else {
		result.Message = "Analysis passed"
	}

	return result, nil
}

// runHealthGate runs the health gate.
func (e *StrategyExecutor) runHealthGate(ctx context.Context, execution *StrategyExecution, params *ExecutionParams) error {
	config := execution.Config.HealthGate
	if config == nil || !config.Enabled {
		return nil
	}

	execution.Progress.Phase = "health_gate"
	execution.Progress.Message = "Running health gate checks"

	// Initial delay
	if config.InitialDelay > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(config.InitialDelay):
		}
	}

	// Create timeout context
	gateCtx := ctx
	if config.Timeout > 0 {
		var cancel context.CancelFunc
		gateCtx, cancel = context.WithTimeout(ctx, config.Timeout)
		defer cancel()
	}

	interval := config.CheckInterval
	if interval == 0 {
		interval = 5 * time.Second
	}

	successThreshold := config.SuccessThreshold
	if successThreshold == 0 {
		successThreshold = 3
	}

	failureThreshold := config.FailureThreshold
	if failureThreshold == 0 {
		failureThreshold = 3
	}

	consecutiveSuccess := 0
	consecutiveFailure := 0

	for {
		select {
		case <-gateCtx.Done():
			return fmt.Errorf("%w: timeout waiting for health gate", ErrHealthGateFailed)
		default:
		}

		// Check health
		healthy := true
		if e.healthChecker != nil && len(config.Endpoints) > 0 {
			result, err := e.healthChecker.CheckHealth(gateCtx, params.ServiceName, config.Endpoints)
			if err != nil || !result.Healthy {
				healthy = false
			}
		}

		// Check custom checks
		for _, check := range config.CustomChecks {
			if e.healthChecker != nil {
				if err := e.healthChecker.CheckCustom(gateCtx, &check); err != nil {
					healthy = false
					break
				}
			}
		}

		if healthy {
			consecutiveSuccess++
			consecutiveFailure = 0

			if consecutiveSuccess >= successThreshold {
				return nil
			}
		} else {
			consecutiveFailure++
			consecutiveSuccess = 0

			if consecutiveFailure >= failureThreshold {
				return fmt.Errorf("%w: %d consecutive failures", ErrHealthGateFailed, consecutiveFailure)
			}
		}

		time.Sleep(interval)
	}
}

// Abort aborts a running execution.
func (e *StrategyExecutor) Abort(executionID string) error {
	e.mu.Lock()
	execution, exists := e.executions[executionID]
	if !exists {
		e.mu.Unlock()
		return fmt.Errorf("execution not found: %s", executionID)
	}

	execution.State = ExecutionStateAborted
	execution.CompletedAt = time.Now()
	e.mu.Unlock()

	if execution.cancel != nil {
		execution.cancel()
	}

	return nil
}

// Resume resumes a paused execution.
func (e *StrategyExecutor) Resume(executionID string) error {
	e.mu.Lock()
	execution, exists := e.executions[executionID]
	if !exists {
		e.mu.Unlock()
		return fmt.Errorf("execution not found: %s", executionID)
	}

	if execution.State != ExecutionStatePaused {
		e.mu.Unlock()
		return fmt.Errorf("execution is not paused: %s", execution.State)
	}

	execution.State = ExecutionStateRunning
	e.mu.Unlock()

	return nil
}

// Promote promotes a canary execution to 100%.
func (e *StrategyExecutor) Promote(executionID string) error {
	e.mu.Lock()
	execution, exists := e.executions[executionID]
	if !exists {
		e.mu.Unlock()
		return fmt.Errorf("execution not found: %s", executionID)
	}

	if execution.Config.Type != StrategyCanary {
		e.mu.Unlock()
		return fmt.Errorf("promote only available for canary deployments")
	}

	execution.State = ExecutionStatePromoting
	e.mu.Unlock()

	return nil
}

// Status returns the status of an execution.
func (e *StrategyExecutor) Status(executionID string) (*StrategyExecution, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	execution, exists := e.executions[executionID]
	if !exists {
		return nil, fmt.Errorf("execution not found: %s", executionID)
	}

	return execution, nil
}

// updateState updates execution state.
func (e *StrategyExecutor) updateState(execution *StrategyExecution, state StrategyExecutionState, message string) {
	e.mu.Lock()
	execution.State = state
	execution.Progress.Message = message
	e.mu.Unlock()

	e.emitEvent(execution, "state_changed", map[string]interface{}{
		"state":   state,
		"message": message,
	})
}

// failExecution marks an execution as failed.
func (e *StrategyExecutor) failExecution(execution *StrategyExecution, err error) {
	e.mu.Lock()
	execution.State = ExecutionStateFailed
	execution.CompletedAt = time.Now()
	execution.Error = err.Error()
	execution.Progress.Phase = "failed"
	execution.Progress.Message = err.Error()
	e.mu.Unlock()

	e.emitEvent(execution, "strategy_failed", map[string]interface{}{
		"error": err.Error(),
	})
}

// sendNotifications sends notifications for an event.
func (e *StrategyExecutor) sendNotifications(ctx context.Context, execution *StrategyExecution, eventType string) {
	if execution.Config.Notifications == nil || !execution.Config.Notifications.Enabled {
		return
	}

	config := execution.Config.Notifications
	shouldNotify := false

	switch eventType {
	case "start":
		shouldNotify = config.OnStart
	case "success":
		shouldNotify = config.OnSuccess
	case "failure":
		shouldNotify = config.OnFailure
	case "rollback":
		shouldNotify = config.OnRollback
	case "pause":
		shouldNotify = config.OnPause
	}

	if !shouldNotify || e.notifier == nil {
		return
	}

	event := &StrategyEvent{
		Type:         eventType,
		ExecutionID:  execution.ID,
		DeploymentID: execution.DeploymentID,
		State:        execution.State,
		Progress:     execution.Progress,
		Timestamp:    time.Now(),
	}

	for _, channel := range config.Channels {
		go e.notifier.Notify(ctx, &channel, event)
	}
}

// emitEvent emits a strategy event.
func (e *StrategyExecutor) emitEvent(execution *StrategyExecution, eventType string, data map[string]interface{}) {
	e.mu.RLock()
	callback := e.eventCallback
	e.mu.RUnlock()

	if callback == nil {
		return
	}

	event := &StrategyEvent{
		Type:         eventType,
		ExecutionID:  execution.ID,
		DeploymentID: execution.DeploymentID,
		State:        execution.State,
		Progress:     execution.Progress,
		Timestamp:    time.Now(),
		Data:         data,
	}

	go callback(event)
}

// generateExecutionID generates a unique execution ID.
func generateExecutionID() string {
	return fmt.Sprintf("exec-%d", time.Now().UnixNano())
}

// DefaultStrategyConfig returns a default strategy configuration.
func DefaultStrategyConfig(strategyType DeploymentStrategy) *StrategyConfig {
	config := &StrategyConfig{
		Type:    strategyType,
		Timeout: 1 * time.Hour,
	}

	switch strategyType {
	case StrategyRolling:
		config.Rolling = &RollingConfig{
			MaxSurge:         1,
			MaxUnavailable:   0,
			ProgressDeadline: 10 * time.Minute,
			MinReadySeconds:  30,
		}

	case StrategyBlueGreen:
		config.BlueGreen = &BlueGreenConfig{
			SwitchTimeout: 30 * time.Second,
			TestDuration:  5 * time.Minute,
			AutoPromote:   false,
		}

	case StrategyCanary:
		config.Canary = &CanaryConfig{
			Steps: []CanaryStepConfig{
				{Weight: 1, Duration: time.Minute, Analysis: true},
				{Weight: 10, Duration: 5 * time.Minute, Analysis: true},
				{Weight: 50, Duration: 10 * time.Minute, Analysis: true},
				{Weight: 100, Duration: 0, Analysis: false},
			},
			Analysis: &CanaryAnalysisConfig{
				Interval:             30 * time.Second,
				ErrorRateThreshold:   0.01,
				SuccessRateThreshold: 0.99,
				MinSampleSize:        100,
				FailureThreshold:     3,
			},
			AutoPromote:    false,
			PromoteTimeout: time.Hour,
			AbortOnFailure: true,
			CanaryReplicas: 1,
		}

	case StrategyRecreate:
		// No specific config needed
	}

	config.HealthGate = &HealthGateConfig{
		Enabled:          true,
		InitialDelay:     10 * time.Second,
		Timeout:          5 * time.Minute,
		CheckInterval:    5 * time.Second,
		SuccessThreshold: 3,
		FailureThreshold: 3,
	}

	return config
}
