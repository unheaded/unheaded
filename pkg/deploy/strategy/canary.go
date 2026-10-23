// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package strategy provides deployment strategy implementations.
package strategy

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// CanaryStrategy implements a canary deployment strategy.
// It starts canary instances next to the stable ones, walks the traffic
// steps with analysis at each step that asks for it, and promotes the
// canary only if every analysis passed. A failed analysis puts traffic back
// on stable, deletes the canary instances and reports RolledBack.
type CanaryStrategy struct {
	mu sync.RWMutex

	// deployments tracks active canary deployments
	deployments map[string]*canaryDeployment
}

type canaryDeployment struct {
	params          *ExecuteParams
	progress        *Progress
	result          *Result
	stableInstances []Instance
	canaryInstances []Instance
	cancel          context.CancelFunc
	done            chan struct{}
	currentWeight   int
	currentStep     int
	promoted        bool
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
		if config.Analysis.Interval < 0 {
			return fmt.Errorf("analysis interval must be non-negative")
		}
	}

	return nil
}

// Execute executes the canary deployment strategy.
func (s *CanaryStrategy) Execute(ctx context.Context, params *ExecuteParams) (*Result, error) {
	if params == nil {
		return nil, ErrInvalidConfig
	}
	if err := s.Validate(params.Config); err != nil {
		return nil, err
	}
	// A canary without analysis would promote whatever it was given.
	if err := requireDriver(TypeCanary, params.Callbacks, true, true); err != nil {
		return nil, err
	}
	if params.Replicas < 1 {
		return nil, fmt.Errorf("%w: replicas must be at least 1", ErrInvalidConfig)
	}
	cb := params.Callbacks

	ctx, cancel := context.WithTimeout(ctx, progressDeadline(params.Config))
	defer cancel()

	deployment := &canaryDeployment{
		params: params,
		progress: &Progress{
			Phase:          "initializing",
			Message:        "Starting canary deployment",
			TotalInstances: params.Replicas,
		},
		result:          &Result{Metrics: &ResultMetrics{}},
		stableInstances: params.Instances,
		cancel:          cancel,
		done:            make(chan struct{}),
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
	res := deployment.result
	steps := params.Config.Steps
	interval := pollInterval(params.Config)
	progress := func(phase, msg string, pct int) {
		deployment.progress.Phase = phase
		deployment.progress.Message = msg
		deployment.progress.Percentage = pct
		if cb.OnProgress != nil {
			cb.OnProgress(ctx, deployment.progress)
		}
	}
	// rollBack puts traffic on stable and deletes the canary instances.
	// It runs on a context the deployment's cancellation cannot stop.
	rollBack := func(msg string, err error) (*Result, error) {
		cctx := context.WithoutCancel(ctx)
		if serr := cb.OnTrafficShift(cctx, &TrafficShiftParams{
			OldVersion: params.TargetVersion,
			NewVersion: params.CurrentVersion,
			Weight:     100,
		}); serr != nil {
			msg += fmt.Sprintf("; traffic back to stable failed: %v", serr)
		} else {
			res.Metrics.TrafficShifts++
		}
		if failed := deleteAll(cctx, cb, deployment.canaryInstances, res.Metrics, "canary"); failed > 0 {
			msg += fmt.Sprintf("; %d canary instances could not be deleted", failed)
		}
		s.mu.Lock()
		deployment.currentWeight = 0
		s.mu.Unlock()
		res.Success = false
		res.RolledBack = true
		res.Message = msg
		res.Duration = time.Since(startTime)
		res.Instances = deployment.stableInstances
		deployment.progress.Phase = "rolled_back"
		return res, err
	}

	// Phase 1: canary instances (at least one).
	progress("deploying_canary", "Deploying canary instances", 0)
	canaryCount := max((params.Replicas*steps[0].Weight)/100, 1)
	for i := 0; i < canaryCount; i++ {
		inst, err := cb.OnInstanceCreate(ctx, &InstanceParams{
			ServiceName: params.ServiceName,
			Version:     params.TargetVersion,
			Labels: map[string]string{
				"deployment": params.DeploymentID,
				"version":    params.TargetVersion,
				"role":       "canary",
			},
		})
		if err != nil || inst == nil {
			res.Metrics.InstancesFailed++
			return rollBack(fmt.Sprintf("Failed to create canary instance: %v", err), ErrInstanceUpdateFailed)
		}
		inst.Healthy = false
		deployment.canaryInstances = append(deployment.canaryInstances, *inst)
		res.Metrics.InstancesCreated++
	}
	if err := waitHealthy(ctx, cb, deployment.canaryInstances, interval, &res.Metrics.HealthChecks); err != nil {
		return rollBack(fmt.Sprintf("Canary never became healthy: %v", err), err)
	}

	// Phase 2: steps.
	for stepIdx, step := range steps {
		if err := ctx.Err(); err != nil {
			return rollBack("Deployment cancelled", err)
		}
		s.mu.RLock()
		promoted := deployment.promoted
		s.mu.RUnlock()
		if promoted {
			break
		}
		s.mu.Lock()
		deployment.currentStep = stepIdx
		deployment.currentWeight = step.Weight
		s.mu.Unlock()
		deployment.progress.CurrentWeight = step.Weight
		progress(fmt.Sprintf("canary_step_%d", stepIdx+1),
			fmt.Sprintf("Canary step %d: shifting %d%% traffic", stepIdx+1, step.Weight), (stepIdx*100)/len(steps))

		if err := cb.OnTrafficShift(ctx, &TrafficShiftParams{
			OldVersion: params.CurrentVersion,
			NewVersion: params.TargetVersion,
			Weight:     step.Weight,
		}); err != nil {
			return rollBack(fmt.Sprintf("Failed to shift traffic: %v", err), ErrTrafficShiftFailed)
		}
		res.Metrics.TrafficShifts++

		if step.Analysis {
			analysis, err := s.runAnalysis(ctx, deployment, params, stepIdx, step)
			res.Metrics.AnalysisRuns++
			if err != nil {
				return rollBack(fmt.Sprintf("Analysis failed at step %d: %v", stepIdx+1, err), fmt.Errorf("%w: %w", ErrRollbackRequired, err))
			}
			deployment.analysisHistory = append(deployment.analysisHistory, analysis)
			res.Analysis = analysis
			if !analysis.Pass || analysis.ShouldRollback {
				return rollBack(fmt.Sprintf("Analysis failed at step %d: %s", stepIdx+1, analysis.Message), ErrRollbackRequired)
			}
		}

		if step.Pause > 0 && step.Weight < 100 {
			if err := sleepCtx(ctx, step.Pause); err != nil {
				return rollBack("Deployment cancelled during pause", err)
			}
		}
	}

	// Phase 3: promote. Scale the new version to Replicas, prove it
	// healthy, put all traffic on it, then delete stable.
	progress("promoting", "Promoting canary to production", 90)
	failures := 0
	for len(deployment.canaryInstances) < params.Replicas {
		if err := ctx.Err(); err != nil {
			return rollBack("Deployment cancelled during promotion", err)
		}
		inst, err := cb.OnInstanceCreate(ctx, &InstanceParams{
			ServiceName: params.ServiceName,
			Version:     params.TargetVersion,
			Labels: map[string]string{
				"deployment": params.DeploymentID,
				"version":    params.TargetVersion,
				"role":       "stable",
			},
		})
		if err != nil || inst == nil {
			res.Metrics.InstancesFailed++
			failures++
			if failures > params.Replicas {
				return rollBack(fmt.Sprintf("Promotion failed: %d instance creations failed (last: %v)", failures, err), ErrInstanceUpdateFailed)
			}
			if err := sleepCtx(ctx, interval); err != nil {
				return rollBack("Deployment cancelled during promotion", err)
			}
			continue
		}
		inst.Healthy = false
		deployment.canaryInstances = append(deployment.canaryInstances, *inst)
		res.Metrics.InstancesCreated++
	}
	if err := waitHealthy(ctx, cb, deployment.canaryInstances, interval, &res.Metrics.HealthChecks); err != nil {
		return rollBack(fmt.Sprintf("Promoted instances not healthy: %v", err), err)
	}
	if err := cb.OnTrafficShift(ctx, &TrafficShiftParams{
		OldVersion: params.CurrentVersion,
		NewVersion: params.TargetVersion,
		Weight:     100,
	}); err != nil {
		return rollBack(fmt.Sprintf("Failed to shift all traffic to the new version: %v", err), ErrTrafficShiftFailed)
	}
	res.Metrics.TrafficShifts++

	failed := deleteAll(context.WithoutCancel(ctx), cb, deployment.stableInstances, res.Metrics, "canary")
	deployment.stableInstances = nil

	res.Success = true
	res.Message = fmt.Sprintf("Canary deployment completed: %d instances deployed through %d steps", len(deployment.canaryInstances), len(steps))
	if failed > 0 {
		res.Message += fmt.Sprintf(" (%d stable instances could not be deleted)", failed)
	}
	res.Duration = time.Since(startTime)
	res.Instances = deployment.canaryInstances
	deployment.progress.CurrentWeight = 100
	progress("completed", "Deployment completed successfully", 100)
	return res, nil
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

// Rollback performs a rollback of the canary deployment. In flight, the
// deployment is cancelled, which rolls it back itself (traffic to stable,
// canary deleted). Otherwise the target version is redeployed with a
// rolling update (a canary of an old, known version needs no analysis).
func (s *CanaryStrategy) Rollback(ctx context.Context, params *RollbackParams) error {
	if params == nil {
		return ErrInvalidConfig
	}
	s.mu.RLock()
	deployment, exists := s.deployments[params.DeploymentID]
	s.mu.RUnlock()
	if exists {
		if deployment.cancel != nil {
			deployment.cancel()
		}
		<-deployment.done
		return nil
	}
	return NewRollingStrategy().Rollback(ctx, params)
}

// Status returns the current strategy execution status.
func (s *CanaryStrategy) Status(ctx context.Context, deploymentID string) (*Status, error) {
	s.mu.RLock()
	deployment, exists := s.deployments[deploymentID]
	if !exists {
		s.mu.RUnlock()
		return nil, ErrStrategyNotFound
	}
	weight := deployment.currentWeight
	s.mu.RUnlock()

	status := &Status{
		Phase:         deployment.progress.Phase,
		Progress:      deployment.progress,
		TrafficWeight: weight,
	}
	for _, inst := range deployment.canaryInstances {
		if inst.Healthy {
			status.Healthy++
		} else {
			status.Unhealthy++
		}
	}
	status.Instances = deployment.canaryInstances
	if len(deployment.analysisHistory) > 0 {
		status.Analysis = deployment.analysisHistory[len(deployment.analysisHistory)-1]
	}
	return status, nil
}

var errNoAnalysisResult = errors.New("analysis returned no result")

// runAnalysis waits one analysis interval for samples to accumulate, asks
// OnAnalysis to measure the canary against stable, and applies the
// configured thresholds to what it measured.
func (s *CanaryStrategy) runAnalysis(ctx context.Context, deployment *canaryDeployment, params *ExecuteParams, stepIdx int, step CanaryStep) (*AnalysisResult, error) {
	config := params.Config.Analysis
	if config != nil && config.Interval > 0 {
		if err := sleepCtx(ctx, config.Interval); err != nil {
			return nil, err
		}
	}
	result, err := params.Callbacks.OnAnalysis(ctx, &AnalysisRequest{
		DeploymentID:  params.DeploymentID,
		ServiceName:   params.ServiceName,
		StableVersion: params.CurrentVersion,
		CanaryVersion: params.TargetVersion,
		Step:          stepIdx + 1,
		Weight:        step.Weight,
		Canary:        append([]Instance(nil), deployment.canaryInstances...),
		Stable:        append([]Instance(nil), deployment.stableInstances...),
		Config:        config,
	})
	if err != nil {
		return nil, fmt.Errorf("analysis callback failed: %w", err)
	}
	if result == nil {
		return nil, errNoAnalysisResult
	}
	if config != nil && result.Metrics != nil {
		m := result.Metrics
		if config.ErrorRateThreshold > 0 && m.ErrorRate > config.ErrorRateThreshold {
			result.Pass, result.ShouldRollback = false, true
			result.Message = fmt.Sprintf("Error rate %.2f%% exceeds threshold %.2f%%", m.ErrorRate*100, config.ErrorRateThreshold*100)
		}
		if config.SuccessRateThreshold > 0 && m.SuccessRate < config.SuccessRateThreshold {
			result.Pass, result.ShouldRollback = false, true
			result.Message = fmt.Sprintf("Success rate %.2f%% below threshold %.2f%%", m.SuccessRate*100, config.SuccessRateThreshold*100)
		}
		if config.LatencyP99Threshold > 0 && m.LatencyP99 > config.LatencyP99Threshold {
			result.Pass, result.ShouldRollback = false, true
			result.Message = fmt.Sprintf("P99 latency %v exceeds threshold %v", m.LatencyP99, config.LatencyP99Threshold)
		}
	}
	return result, nil
}

func init() {
	RegisterStrategy(NewCanaryStrategy())
}
