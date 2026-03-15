// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package strategy provides deployment strategy implementations for the Unheaded Kingdom.
// Strategies define HOW instances are updated during a deployment.
package strategy

import (
	"context"
	"errors"
	"time"
)

// Common errors returned by strategy operations.
var (
	ErrStrategyNotFound    = errors.New("strategy not found")
	ErrInvalidConfig       = errors.New("invalid strategy configuration")
	ErrInstanceUpdateFailed = errors.New("instance update failed")
	ErrRollbackRequired    = errors.New("rollback required")
	ErrTrafficShiftFailed  = errors.New("traffic shift failed")
	ErrHealthCheckTimeout  = errors.New("health check timeout")
)

// Type represents the type of deployment strategy.
type Type string

const (
	TypeRolling   Type = "rolling"
	TypeBlueGreen Type = "blue_green"
	TypeCanary    Type = "canary"
	TypeRecreate  Type = "recreate"
)

// Strategy defines the interface for deployment strategies.
type Strategy interface {
	// Type returns the strategy type
	Type() Type

	// Execute executes the deployment strategy
	Execute(ctx context.Context, params *ExecuteParams) (*Result, error)

	// Rollback performs a rollback of the deployment
	Rollback(ctx context.Context, params *RollbackParams) error

	// Status returns the current strategy execution status
	Status(ctx context.Context, deploymentID string) (*Status, error)

	// Validate validates the strategy configuration
	Validate(config *Config) error
}

// Config contains strategy-specific configuration.
type Config struct {
	// Type is the strategy type
	Type Type `json:"type"`

	// Rolling config
	MaxSurge       int `json:"max_surge,omitempty"`
	MaxUnavailable int `json:"max_unavailable,omitempty"`

	// Blue-green config
	SwitchTimeout time.Duration `json:"switch_timeout,omitempty"`
	TestDuration  time.Duration `json:"test_duration,omitempty"`

	// Canary config
	Steps          []CanaryStep    `json:"steps,omitempty"`
	CanarySteps    []CanaryStep    `json:"canary_steps,omitempty"` // Alias for Steps
	Analysis       *AnalysisConfig `json:"analysis,omitempty"`
	AutoPromote    bool            `json:"auto_promote,omitempty"`
	PromoteTimeout time.Duration   `json:"promote_timeout,omitempty"`

	// Common
	HealthCheck      *HealthCheckConfig `json:"health_check,omitempty"`
	ProgressDeadline time.Duration      `json:"progress_deadline,omitempty"`
}

// CanaryStep defines a step in canary traffic shifting.
type CanaryStep struct {
	// Weight is the percentage of traffic (0-100)
	Weight int `json:"weight"`

	// Pause is optional duration to pause before next step
	Pause time.Duration `json:"pause,omitempty"`

	// Analysis enables analysis during this step
	Analysis bool `json:"analysis,omitempty"`
}

// AnalysisConfig defines canary analysis configuration.
type AnalysisConfig struct {
	// ErrorRateThreshold is max error rate (0.0-1.0) before rollback
	ErrorRateThreshold float64 `json:"error_rate_threshold"`

	// LatencyP99Threshold is max p99 latency before rollback
	LatencyP99Threshold time.Duration `json:"latency_p99_threshold"`

	// SuccessRateThreshold is min success rate (0.0-1.0)
	SuccessRateThreshold float64 `json:"success_rate_threshold"`

	// SampleSize is the number of requests to sample for analysis
	SampleSize int `json:"sample_size"`

	// MinSampleSize is minimum requests before analysis
	MinSampleSize int `json:"min_sample_size"`

	// Interval is analysis check interval
	Interval time.Duration `json:"interval"`
}

// HealthCheckConfig defines health check configuration for strategy.
type HealthCheckConfig struct {
	// Type is the check type (http, tcp, exec)
	Type string `json:"type"`

	// Endpoint is the health check endpoint
	Endpoint string `json:"endpoint"`

	// Path is the health check path (for http checks)
	Path string `json:"path"`

	// Port is the health check port
	Port int `json:"port"`

	// Interval between checks
	Interval time.Duration `json:"interval"`

	// Timeout for each check
	Timeout time.Duration `json:"timeout"`

	// SuccessThreshold for healthy
	SuccessThreshold int `json:"success_threshold"`

	// FailureThreshold for unhealthy
	FailureThreshold int `json:"failure_threshold"`
}

// ExecuteParams contains parameters for strategy execution.
type ExecuteParams struct {
	// DeploymentID is the unique deployment identifier
	DeploymentID string

	// ServiceName is the name of the service being deployed
	ServiceName string

	// CurrentVersion is the current running version
	CurrentVersion string

	// TargetVersion is the version being deployed
	TargetVersion string

	// Replicas is the desired number of instances
	Replicas int

	// Config is the strategy configuration
	Config *Config

	// Instances contains current instance information
	Instances []Instance

	// Callbacks for strategy events
	Callbacks *Callbacks
}

// Instance represents a service instance.
type Instance struct {
	// ID is the unique instance identifier
	ID string `json:"id"`

	// Version is the current version running on this instance
	Version string `json:"version"`

	// State is the current instance state
	State InstanceState `json:"state"`

	// Healthy indicates if the instance is healthy
	Healthy bool `json:"healthy"`

	// Address is the instance network address
	Address string `json:"address"`

	// Labels contain instance metadata
	Labels map[string]string `json:"labels,omitempty"`
}

// InstanceState represents the state of an instance.
type InstanceState string

const (
	InstanceRunning     InstanceState = "running"
	InstanceStarting    InstanceState = "starting"
	InstanceStopping    InstanceState = "stopping"
	InstanceStopped     InstanceState = "stopped"
	InstanceFailed      InstanceState = "failed"
	InstanceTerminating InstanceState = "terminating"
)

// Callbacks for strategy events.
type Callbacks struct {
	// OnInstanceCreate is called when an instance needs to be created
	OnInstanceCreate func(ctx context.Context, params *InstanceParams) (*Instance, error)

	// OnInstanceDelete is called when an instance needs to be deleted
	OnInstanceDelete func(ctx context.Context, instanceID string) error

	// OnInstanceUpdate is called when an instance needs to be updated
	OnInstanceUpdate func(ctx context.Context, instanceID string, version string) error

	// OnHealthCheck is called to check instance health
	OnHealthCheck func(ctx context.Context, instanceID string) (bool, error)

	// OnTrafficShift is called to shift traffic
	OnTrafficShift func(ctx context.Context, params *TrafficShiftParams) error

	// OnProgress is called to report progress
	OnProgress func(ctx context.Context, progress *Progress)

	// OnAnalysis is called during canary analysis
	OnAnalysis func(ctx context.Context, metrics *AnalysisMetrics) (*AnalysisResult, error)
}

// InstanceParams contains parameters for creating an instance.
type InstanceParams struct {
	ServiceName string
	Version     string
	Labels      map[string]string
}

// TrafficShiftParams contains parameters for traffic shifting.
type TrafficShiftParams struct {
	// OldVersion is the version to shift traffic from
	OldVersion string

	// NewVersion is the version to shift traffic to
	NewVersion string

	// Weight is the percentage of traffic for new version (0-100)
	Weight int
}

// Progress represents deployment progress.
type Progress struct {
	// Phase is the current deployment phase
	Phase string `json:"phase"`

	// Message is a human-readable progress message
	Message string `json:"message"`

	// Percentage is the completion percentage (0-100)
	Percentage int `json:"percentage"`

	// UpdatedInstances is the count of updated instances
	UpdatedInstances int `json:"updated_instances"`

	// TotalInstances is the total instance count
	TotalInstances int `json:"total_instances"`

	// CurrentWeight is the current traffic weight (for canary)
	CurrentWeight int `json:"current_weight,omitempty"`
}

// AnalysisMetrics contains metrics for canary analysis.
type AnalysisMetrics struct {
	// ErrorRate is the current error rate (0.0-1.0)
	ErrorRate float64 `json:"error_rate"`

	// LatencyP50 is the p50 latency
	LatencyP50 time.Duration `json:"latency_p50"`

	// LatencyP99 is the p99 latency
	LatencyP99 time.Duration `json:"latency_p99"`

	// SuccessRate is the success rate (0.0-1.0)
	SuccessRate float64 `json:"success_rate"`

	// RequestCount is the total request count
	RequestCount int `json:"request_count"`

	// ErrorCount is the total error count
	ErrorCount int `json:"error_count"`
}

// AnalysisResult contains the result of canary analysis.
type AnalysisResult struct {
	// Pass indicates if analysis passed
	Pass bool `json:"pass"`

	// Message is a human-readable result message
	Message string `json:"message"`

	// Metrics contains the analyzed metrics
	Metrics *AnalysisMetrics `json:"metrics"`

	// ShouldRollback indicates if rollback is recommended
	ShouldRollback bool `json:"should_rollback"`
}

// RollbackParams contains parameters for rollback.
type RollbackParams struct {
	// DeploymentID is the deployment to rollback
	DeploymentID string

	// TargetVersion is the version to rollback to
	TargetVersion string

	// Instances is the current instances
	Instances []Instance

	// Callbacks for rollback events
	Callbacks *Callbacks
}

// Result contains the result of strategy execution.
type Result struct {
	// Success indicates if the deployment succeeded
	Success bool `json:"success"`

	// Message is a human-readable result message
	Message string `json:"message"`

	// Instances contains the final instance states
	Instances []Instance `json:"instances"`

	// Metrics contains deployment metrics
	Metrics *ResultMetrics `json:"metrics"`

	// Duration is the total deployment duration
	Duration time.Duration `json:"duration"`
}

// ResultMetrics contains deployment result metrics.
type ResultMetrics struct {
	// InstancesCreated is the count of instances created
	InstancesCreated int `json:"instances_created"`

	// InstancesDeleted is the count of instances deleted
	InstancesDeleted int `json:"instances_deleted"`

	// InstancesUpdated is the count of instances updated
	InstancesUpdated int `json:"instances_updated"`

	// InstancesFailed is the count of instances that failed
	InstancesFailed int `json:"instances_failed"`

	// HealthChecks is the count of health checks performed
	HealthChecks int `json:"health_checks"`

	// TrafficShifts is the count of traffic shifts (for canary)
	TrafficShifts int `json:"traffic_shifts,omitempty"`

	// AnalysisRuns is the count of analysis runs (for canary)
	AnalysisRuns int `json:"analysis_runs,omitempty"`
}

// Status represents the current status of strategy execution.
type Status struct {
	// Phase is the current phase
	Phase string `json:"phase"`

	// Progress is the current progress
	Progress *Progress `json:"progress"`

	// Instances contains current instance states
	Instances []Instance `json:"instances"`

	// Healthy is the count of healthy instances
	Healthy int `json:"healthy"`

	// Unhealthy is the count of unhealthy instances
	Unhealthy int `json:"unhealthy"`

	// TrafficWeight is current traffic weight (for canary)
	TrafficWeight int `json:"traffic_weight,omitempty"`

	// Analysis contains latest analysis result (for canary)
	Analysis *AnalysisResult `json:"analysis,omitempty"`
}

// DefaultConfig returns default configuration for a strategy type.
func DefaultConfig(t Type) *Config {
	switch t {
	case TypeRolling:
		return &Config{
			Type:             TypeRolling,
			MaxSurge:         1,
			MaxUnavailable:   0,
			ProgressDeadline: 10 * time.Minute,
			HealthCheck: &HealthCheckConfig{
				Type:             "http",
				Interval:         10 * time.Second,
				Timeout:          5 * time.Second,
				SuccessThreshold: 1,
				FailureThreshold: 3,
			},
		}

	case TypeBlueGreen:
		return &Config{
			Type:             TypeBlueGreen,
			SwitchTimeout:    30 * time.Second,
			TestDuration:     5 * time.Minute,
			ProgressDeadline: 15 * time.Minute,
			HealthCheck: &HealthCheckConfig{
				Type:             "http",
				Interval:         10 * time.Second,
				Timeout:          5 * time.Second,
				SuccessThreshold: 1,
				FailureThreshold: 3,
			},
		}

	case TypeCanary:
		return &Config{
			Type:       TypeCanary,
			AutoPromote: false,
			PromoteTimeout: 1 * time.Hour,
			Steps: []CanaryStep{
				{Weight: 1, Pause: 1 * time.Minute, Analysis: true},
				{Weight: 10, Pause: 5 * time.Minute, Analysis: true},
				{Weight: 50, Pause: 10 * time.Minute, Analysis: true},
				{Weight: 100, Pause: 0, Analysis: false},
			},
			Analysis: &AnalysisConfig{
				ErrorRateThreshold:   0.01,  // 1% error rate
				LatencyP99Threshold:  500 * time.Millisecond,
				SuccessRateThreshold: 0.99,  // 99% success rate
				MinSampleSize:        100,
				Interval:             30 * time.Second,
			},
			ProgressDeadline: 2 * time.Hour,
			HealthCheck: &HealthCheckConfig{
				Type:             "http",
				Interval:         10 * time.Second,
				Timeout:          5 * time.Second,
				SuccessThreshold: 1,
				FailureThreshold: 3,
			},
		}

	case TypeRecreate:
		return &Config{
			Type:             TypeRecreate,
			ProgressDeadline: 10 * time.Minute,
			HealthCheck: &HealthCheckConfig{
				Type:             "http",
				Interval:         10 * time.Second,
				Timeout:          5 * time.Second,
				SuccessThreshold: 1,
				FailureThreshold: 3,
			},
		}

	default:
		return &Config{Type: t}
	}
}

// ValidateConfig validates a strategy configuration.
func ValidateConfig(config *Config) error {
	if config == nil {
		return ErrInvalidConfig
	}

	switch config.Type {
	case TypeRolling:
		if config.MaxSurge < 0 {
			return errors.New("max_surge must be non-negative")
		}
		if config.MaxUnavailable < 0 {
			return errors.New("max_unavailable must be non-negative")
		}
		if config.MaxSurge == 0 && config.MaxUnavailable == 0 {
			return errors.New("either max_surge or max_unavailable must be positive")
		}

	case TypeBlueGreen:
		if config.SwitchTimeout < 0 {
			return errors.New("switch_timeout must be non-negative")
		}

	case TypeCanary:
		if len(config.Steps) == 0 {
			return errors.New("canary strategy requires at least one step")
		}
		lastWeight := -1
		for i, step := range config.Steps {
			if step.Weight < 0 || step.Weight > 100 {
				return errors.New("canary step weight must be 0-100")
			}
			if step.Weight <= lastWeight {
				return errors.New("canary step weights must be increasing")
			}
			lastWeight = step.Weight
			if i == len(config.Steps)-1 && step.Weight != 100 {
				return errors.New("final canary step must have weight 100")
			}
		}

	case TypeRecreate:
		// No specific validation needed

	default:
		return errors.New("unknown strategy type")
	}

	return nil
}

// Registry holds registered strategies.
type Registry struct {
	strategies map[Type]Strategy
}

// NewRegistry creates a new strategy registry.
func NewRegistry() *Registry {
	return &Registry{
		strategies: make(map[Type]Strategy),
	}
}

// Register registers a strategy.
func (r *Registry) Register(strategy Strategy) {
	r.strategies[strategy.Type()] = strategy
}

// Get returns a strategy by type.
func (r *Registry) Get(t Type) (Strategy, error) {
	strategy, ok := r.strategies[t]
	if !ok {
		return nil, ErrStrategyNotFound
	}
	return strategy, nil
}

// DefaultRegistry is the default strategy registry.
var DefaultRegistry = NewRegistry()

// RegisterStrategy registers a strategy with the default registry.
func RegisterStrategy(strategy Strategy) {
	DefaultRegistry.Register(strategy)
}

// GetStrategy returns a strategy from the default registry.
func GetStrategy(t Type) (Strategy, error) {
	return DefaultRegistry.Get(t)
}
