// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package strategy provides deployment strategy implementations.
package strategy

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// CanaryStrategy implements a canary deployment strategy.
// It gradually shifts traffic from the current version to the new version,
// with analysis at each step to detect issues before full rollout.
type CanaryStrategy struct {
	mu sync.RWMutex

	// deployments tracks active canary deployments
	deployments map[string]*canaryDeployment
}

type canaryDeployment struct {
	params         *ExecuteParams
	progress       *Progress
	result         *Result
	stableInstances []Instance
	canaryInstances []Instance
	cancel         context.CancelFunc
	done           chan struct{}
	currentWeight  int
	currentStep    int
	promoted       bool
	analysisHistory []*AnalysisResult
}

// NewCanaryStrategy creates a new canary strategy.
func NewCanaryStrategy() *CanaryStrategy {
	return &CanaryStrategy{
		deployments: make(map[string]*canaryDeployment),
	}
}

// Type returns the strategy type.
func (s *CanaryStrategy) Type() Type {
	return TypeCanary
}

// Validate validates the strategy configuration.
func (s *CanaryStrategy) Validate(config *Config) error {
	if config == nil {
		return ErrInvalidConfig
	}
	if config.Type != TypeCanary {
		return fmt.Errorf("expected canary strategy config, got %s", config.Type)
	}
	if len(config.Steps) == 0 {
		return fmt.Errorf("canary strategy requires at least one step")
	}

	lastWeight := -1
	for i, step := range config.Steps {
		if step.Weight < 0 || step.Weight > 100 {
			return fmt.Errorf("canary step weight must be 0-100, got %d", step.Weight)
		}
		if step.Weight <= lastWeight {
			return fmt.Errorf("canary step weights must be increasing")
		}
		lastWeight = step.Weight

		if i == len(config.Steps)-1 && step.Weight != 100 {
			return fmt.Errorf("final canary step must have weight 100")
		}
	}

	if config.Analysis != nil {
		if config.Analysis.ErrorRateThreshold < 0 || config.Analysis.ErrorRateThreshold > 1 {
			return fmt.Errorf("error_rate_threshold must be 0-1")
		}
		if config.Analysis.SuccessRateThreshold < 0 || config.Analysis.SuccessRateThreshold > 1 {
			return fmt.Errorf("success_rate_threshold must be 0-1")
		}
	}

	return nil
}

// Execute executes the canary deployment strategy.
func (s *CanaryStrategy) Execute(ctx context.Context, params *ExecuteParams) (*Result, error) {
	if err := s.Validate(params.Config); err != nil {
		return nil, err
	}

	// Create cancellable context
	ctx, cancel := context.WithCancel(ctx)

	// Create deployment tracking
	deployment := &canaryDeployment{
		params: params,
		progress: &Progress{
			Phase:          "initializing",
			Message:        "Starting canary deployment",
			Percentage:     0,
			TotalInstances: params.Replicas,
		},
		result: &Result{
			Metrics: &ResultMetrics{},
		},
		stableInstances: params.Instances,
		canaryInstances: []Instance{},
		cancel:          cancel,
		done:            make(chan struct{}),
		currentWeight:   0,
		currentStep:     0,
		analysisHistory: []*AnalysisResult{},
	}

	s.mu.Lock()
	s.deployments[params.DeploymentID] = deployment
	s.mu.Unlock()

	defer func() {
		close(deployment.done)
		s.mu.Lock()
		delete(s.deployments, params.DeploymentID)
		s.mu.Unlock()
	}()

	startTime := time.Now()
	steps := params.Config.Steps

	// Phase 1: Deploy canary instances
	deployment.progress.Phase = "deploying_canary"
	deployment.progress.Message = "Deploying canary instances"

	if params.Callbacks != nil && params.Callbacks.OnProgress != nil {
		params.Callbacks.OnProgress(ctx, deployment.progress)
	}

	// Calculate initial canary size (based on first step weight)
	initialWeight := steps[0].Weight
	canaryCount := (params.Replicas * initialWeight) / 100
	if canaryCount < 1 {
		canaryCount = 1 // At least one canary instance
	}

	for i := 0; i < canaryCount; i++ {
		select {
		case <-ctx.Done():
			return s.buildCancelledResult(deployment, startTime), ctx.Err()
		default:
		}

		var inst *Instance
		if params.Callbacks != nil && params.Callbacks.OnInstanceCreate != nil {
			var err error
			inst, err = params.Callbacks.OnInstanceCreate(ctx, &InstanceParams{
				ServiceName: params.ServiceName,
				Version:     params.TargetVersion,
				Labels: map[string]string{
					"deployment": params.DeploymentID,
					"version":    params.TargetVersion,
					"role":       "canary",
				},
			})
			if err != nil {
				deployment.result.Metrics.InstancesFailed++
				deployment.result.Success = false
				deployment.result.Message = fmt.Sprintf("Failed to create canary instance: %v", err)
				deployment.result.Duration = time.Since(startTime)
				return deployment.result, ErrInstanceUpdateFailed
			}
		} else {
			inst = &Instance{
				ID:      fmt.Sprintf("canary-%s-%d", params.TargetVersion, i),
				Version: params.TargetVersion,
				State:   InstanceRunning,
				Healthy: false,
				Labels: map[string]string{
					"role": "canary",
				},
			}
		}

		deployment.canaryInstances = append(deployment.canaryInstances, *inst)
		deployment.result.Metrics.InstancesCreated++
	}

	// Wait for canary instances to become healthy
	if err := s.waitForHealthy(ctx, deployment, params); err != nil {
		return s.buildFailedResult(deployment, startTime, err)
	}

	// Phase 2: Execute canary steps
	for stepIdx, step := range steps {
		select {
		case <-ctx.Done():
			return s.buildCancelledResult(deployment, startTime), ctx.Err()
		default:
		}

		// Check if manually promoted
		s.mu.RLock()
		promoted := deployment.promoted
		s.mu.RUnlock()
		if promoted {
			// Skip to final promotion
			break
		}

		deployment.currentStep = stepIdx
		deployment.currentWeight = step.Weight

		deployment.progress.Phase = fmt.Sprintf("canary_step_%d", stepIdx+1)
		deployment.progress.Message = fmt.Sprintf("Canary step %d: shifting %d%% traffic", stepIdx+1, step.Weight)
		deployment.progress.CurrentWeight = step.Weight
		deployment.progress.Percentage = (stepIdx * 100) / len(steps)

		if params.Callbacks != nil && params.Callbacks.OnProgress != nil {
			params.Callbacks.OnProgress(ctx, deployment.progress)
		}

		// Shift traffic
		if params.Callbacks != nil && params.Callbacks.OnTrafficShift != nil {
			err := params.Callbacks.OnTrafficShift(ctx, &TrafficShiftParams{
				OldVersion: params.CurrentVersion,
				NewVersion: params.TargetVersion,
				Weight:     step.Weight,
			})
			if err != nil {
				// Rollback traffic
				s.rollbackTraffic(ctx, deployment, params)
				deployment.result.Success = false
				deployment.result.Message = fmt.Sprintf("Failed to shift traffic: %v", err)
				deployment.result.Duration = time.Since(startTime)
				return deployment.result, ErrTrafficShiftFailed
			}
		}
		deployment.result.Metrics.TrafficShifts++

		// Run analysis if configured for this step
		if step.Analysis && params.Config.Analysis != nil {
			analysisResult, err := s.runAnalysis(ctx, deployment, params)
			if err != nil {
				// Analysis error - rollback
				s.rollbackTraffic(ctx, deployment, params)
				deployment.result.Success = false
				deployment.result.Message = fmt.Sprintf("Analysis failed: %v", err)
				deployment.result.Duration = time.Since(startTime)
				return deployment.result, err
			}

			deployment.analysisHistory = append(deployment.analysisHistory, analysisResult)
			deployment.result.Metrics.AnalysisRuns++

			if !analysisResult.Pass || analysisResult.ShouldRollback {
				// Analysis failed - rollback
				s.rollbackTraffic(ctx, deployment, params)
				deployment.result.Success = false
				deployment.result.Message = fmt.Sprintf("Analysis failed at step %d: %s", stepIdx+1, analysisResult.Message)
				deployment.result.Duration = time.Since(startTime)
				deployment.result.Instances = deployment.canaryInstances
				return deployment.result, ErrRollbackRequired
			}
		}

		// Pause at this step
		if step.Pause > 0 && step.Weight < 100 {
			select {
			case <-ctx.Done():
				return s.buildCancelledResult(deployment, startTime), ctx.Err()
			case <-time.After(step.Pause):
			}
		}
	}

	// Phase 3: Full promotion - replace stable instances with canary
	deployment.progress.Phase = "promoting"
	deployment.progress.Message = "Promoting canary to production"
	deployment.progress.Percentage = 90

	if params.Callbacks != nil && params.Callbacks.OnProgress != nil {
		params.Callbacks.OnProgress(ctx, deployment.progress)
	}

	// Scale canary to full replica count
	for len(deployment.canaryInstances) < params.Replicas {
		select {
		case <-ctx.Done():
			return s.buildCancelledResult(deployment, startTime), ctx.Err()
		default:
		}

		var inst *Instance
		if params.Callbacks != nil && params.Callbacks.OnInstanceCreate != nil {
			var err error
			inst, err = params.Callbacks.OnInstanceCreate(ctx, &InstanceParams{
				ServiceName: params.ServiceName,
				Version:     params.TargetVersion,
				Labels: map[string]string{
					"deployment": params.DeploymentID,
					"version":    params.TargetVersion,
					"role":       "stable", // Now promoted to stable
				},
			})
			if err != nil {
				deployment.result.Metrics.InstancesFailed++
				continue
			}
		} else {
			inst = &Instance{
				ID:      fmt.Sprintf("promoted-%s-%d", params.TargetVersion, len(deployment.canaryInstances)),
				Version: params.TargetVersion,
				State:   InstanceRunning,
				Healthy: true,
			}
		}

		deployment.canaryInstances = append(deployment.canaryInstances, *inst)
		deployment.result.Metrics.InstancesCreated++
	}

	// Wait for all canary instances to be healthy
	if err := s.waitForHealthy(ctx, deployment, params); err != nil {
		return s.buildFailedResult(deployment, startTime, err)
	}

	// Shift 100% traffic to canary
	if params.Callbacks != nil && params.Callbacks.OnTrafficShift != nil {
		params.Callbacks.OnTrafficShift(ctx, &TrafficShiftParams{
			OldVersion: params.CurrentVersion,
			NewVersion: params.TargetVersion,
			Weight:     100,
		})
		deployment.result.Metrics.TrafficShifts++
	}

	// Terminate stable instances
	for _, inst := range deployment.stableInstances {
		if params.Callbacks != nil && params.Callbacks.OnInstanceDelete != nil {
			params.Callbacks.OnInstanceDelete(ctx, inst.ID)
		}
		deployment.result.Metrics.InstancesDeleted++
	}
	deployment.stableInstances = nil

	// Build final result
	deployment.result.Success = true
	deployment.result.Message = fmt.Sprintf("Canary deployment completed: %d instances deployed through %d steps", len(deployment.canaryInstances), len(steps))
	deployment.result.Duration = time.Since(startTime)
	deployment.result.Instances = deployment.canaryInstances

	deployment.progress.Phase = "completed"
	deployment.progress.Percentage = 100
	deployment.progress.CurrentWeight = 100
	deployment.progress.Message = "Deployment completed successfully"

	if params.Callbacks != nil && params.Callbacks.OnProgress != nil {
		params.Callbacks.OnProgress(ctx, deployment.progress)
	}

	return deployment.result, nil
}

// Promote marks a canary deployment for immediate promotion to 100%.
func (s *CanaryStrategy) Promote(deploymentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	deployment, exists := s.deployments[deploymentID]
	if !exists {
		return ErrStrategyNotFound
	}

	deployment.promoted = true
	return nil
}

// Rollback performs a rollback of the canary deployment.
func (s *CanaryStrategy) Rollback(ctx context.Context, params *RollbackParams) error {
	s.mu.RLock()
	deployment, exists := s.deployments[params.DeploymentID]
	s.mu.RUnlock()

	if exists {
		// Cancel the deployment
		if deployment.cancel != nil {
			deployment.cancel()
		}

		// Rollback traffic to stable
		s.rollbackTraffic(ctx, deployment, &ExecuteParams{
			CurrentVersion: deployment.params.TargetVersion,
			TargetVersion:  deployment.params.CurrentVersion,
			Callbacks:      params.Callbacks,
		})

		// Terminate canary instances
		for _, inst := range deployment.canaryInstances {
			if params.Callbacks != nil && params.Callbacks.OnInstanceDelete != nil {
				params.Callbacks.OnInstanceDelete(ctx, inst.ID)
			}
		}

		return nil
	}

	// If no active deployment, perform a fresh rollback deployment
	executeParams := &ExecuteParams{
		DeploymentID:   params.DeploymentID + "-rollback",
		ServiceName:    params.Instances[0].Labels["service"],
		CurrentVersion: params.Instances[0].Version,
		TargetVersion:  params.TargetVersion,
		Replicas:       len(params.Instances),
		Config:         DefaultConfig(TypeCanary),
		Instances:      params.Instances,
		Callbacks:      params.Callbacks,
	}

	result, err := s.Execute(ctx, executeParams)
	if err != nil {
		return fmt.Errorf("rollback deployment failed: %w", err)
	}
	if !result.Success {
		return fmt.Errorf("rollback deployment failed: %s", result.Message)
	}

	return nil
}

// Status returns the current strategy execution status.
func (s *CanaryStrategy) Status(ctx context.Context, deploymentID string) (*Status, error) {
	s.mu.RLock()
	deployment, exists := s.deployments[deploymentID]
	s.mu.RUnlock()

	if !exists {
		return nil, ErrStrategyNotFound
	}

	status := &Status{
		Phase:         deployment.progress.Phase,
		Progress:      deployment.progress,
		TrafficWeight: deployment.currentWeight,
	}

	// Count healthy/unhealthy
	for _, inst := range deployment.canaryInstances {
		if inst.Healthy {
			status.Healthy++
		} else {
			status.Unhealthy++
		}
	}
	status.Instances = deployment.canaryInstances

	// Include latest analysis result
	if len(deployment.analysisHistory) > 0 {
		status.Analysis = deployment.analysisHistory[len(deployment.analysisHistory)-1]
	}

	return status, nil
}

// waitForHealthy waits for all canary instances to become healthy.
func (s *CanaryStrategy) waitForHealthy(ctx context.Context, deployment *canaryDeployment, params *ExecuteParams) error {
	maxRetries := 30
	for attempt := 0; attempt < maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		healthyCount := 0
		for i := range deployment.canaryInstances {
			if deployment.canaryInstances[i].Healthy {
				healthyCount++
				continue
			}

			healthy := true
			if params.Callbacks != nil && params.Callbacks.OnHealthCheck != nil {
				var err error
				healthy, err = params.Callbacks.OnHealthCheck(ctx, deployment.canaryInstances[i].ID)
				if err != nil {
					healthy = false
				}
			}
			deployment.canaryInstances[i].Healthy = healthy
			deployment.result.Metrics.HealthChecks++

			if healthy {
				healthyCount++
			}
		}

		if healthyCount == len(deployment.canaryInstances) {
			return nil
		}

		time.Sleep(1 * time.Second)
	}

	return ErrHealthCheckTimeout
}

// runAnalysis runs canary analysis.
func (s *CanaryStrategy) runAnalysis(ctx context.Context, deployment *canaryDeployment, params *ExecuteParams) (*AnalysisResult, error) {
	config := params.Config.Analysis
	if config == nil {
		return &AnalysisResult{Pass: true, Message: "No analysis configured"}, nil
	}

	// Wait for minimum sample size
	time.Sleep(config.Interval)

	if params.Callbacks == nil || params.Callbacks.OnAnalysis == nil {
		// No analysis callback - assume pass
		return &AnalysisResult{
			Pass:    true,
			Message: "No analysis callback configured, assuming pass",
		}, nil
	}

	// Gather metrics
	metrics := &AnalysisMetrics{} // Would be gathered from metrics system
	result, err := params.Callbacks.OnAnalysis(ctx, metrics)
	if err != nil {
		return nil, fmt.Errorf("analysis callback failed: %w", err)
	}

	// Evaluate thresholds
	if result.Metrics != nil {
		if config.ErrorRateThreshold > 0 && result.Metrics.ErrorRate > config.ErrorRateThreshold {
			result.Pass = false
			result.ShouldRollback = true
			result.Message = fmt.Sprintf("Error rate %.2f%% exceeds threshold %.2f%%",
				result.Metrics.ErrorRate*100, config.ErrorRateThreshold*100)
		}

		if config.SuccessRateThreshold > 0 && result.Metrics.SuccessRate < config.SuccessRateThreshold {
			result.Pass = false
			result.ShouldRollback = true
			result.Message = fmt.Sprintf("Success rate %.2f%% below threshold %.2f%%",
				result.Metrics.SuccessRate*100, config.SuccessRateThreshold*100)
		}

		if config.LatencyP99Threshold > 0 && result.Metrics.LatencyP99 > config.LatencyP99Threshold {
			result.Pass = false
			result.ShouldRollback = true
			result.Message = fmt.Sprintf("P99 latency %v exceeds threshold %v",
				result.Metrics.LatencyP99, config.LatencyP99Threshold)
		}
	}

	return result, nil
}

// rollbackTraffic shifts traffic back to stable.
func (s *CanaryStrategy) rollbackTraffic(ctx context.Context, deployment *canaryDeployment, params *ExecuteParams) {
	if params.Callbacks != nil && params.Callbacks.OnTrafficShift != nil {
		params.Callbacks.OnTrafficShift(ctx, &TrafficShiftParams{
			OldVersion: params.TargetVersion,
			NewVersion: params.CurrentVersion,
			Weight:     100,
		})
	}
	deployment.currentWeight = 0
}

// buildCancelledResult builds a result for a cancelled deployment.
func (s *CanaryStrategy) buildCancelledResult(deployment *canaryDeployment, startTime time.Time) *Result {
	deployment.result.Success = false
	deployment.result.Message = "Deployment cancelled"
	deployment.result.Duration = time.Since(startTime)
	deployment.result.Instances = deployment.canaryInstances
	return deployment.result
}

// buildFailedResult builds a result for a failed deployment.
func (s *CanaryStrategy) buildFailedResult(deployment *canaryDeployment, startTime time.Time, err error) (*Result, error) {
	deployment.result.Success = false
	deployment.result.Message = fmt.Sprintf("Deployment failed: %v", err)
	deployment.result.Duration = time.Since(startTime)
	deployment.result.Instances = deployment.canaryInstances
	return deployment.result, err
}

func init() {
	RegisterStrategy(NewCanaryStrategy())
}
