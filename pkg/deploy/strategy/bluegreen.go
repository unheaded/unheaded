// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package strategy provides deployment strategy implementations.
package strategy

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"
)

// BlueGreenStrategy implements a blue-green deployment strategy.
// It deploys a complete parallel environment (green), waits until every
// green instance is healthy, switches traffic once, and only then deletes
// the old environment (blue). A green environment that never becomes
// healthy is deleted and blue keeps serving.
type BlueGreenStrategy struct {
	mu sync.RWMutex

	// deployments tracks active blue-green deployments
	deployments map[string]*blueGreenDeployment
}

type blueGreenDeployment struct {
	params         *ExecuteParams
	progress       *Progress
	result         *Result
	blueInstances  []Instance
	greenInstances []Instance
	cancel         context.CancelFunc
	done           chan struct{}
	trafficState   string // "blue", "green"
}

// NewBlueGreenStrategy creates a new blue-green strategy.
func NewBlueGreenStrategy() *BlueGreenStrategy {
	return &BlueGreenStrategy{
		deployments: make(map[string]*blueGreenDeployment),
	}
}

// Type returns the strategy type.
func (s *BlueGreenStrategy) Type() Type {
	return TypeBlueGreen
}

// Validate validates the strategy configuration.
func (s *BlueGreenStrategy) Validate(config *Config) error {
	if config == nil {
		return ErrInvalidConfig
	}
	if config.Type != TypeBlueGreen {
		return fmt.Errorf("expected blue-green strategy config, got %s", config.Type)
	}
	if config.SwitchTimeout < 0 {
		return fmt.Errorf("switch_timeout must be non-negative")
	}
	return nil
}

// deleteAll deletes insts through the driver, best effort, and returns how
// many deletions failed.
func deleteAll(ctx context.Context, cb *Callbacks, insts []Instance, m *ResultMetrics, who string) int {
	failed := 0
	for _, inst := range insts {
		if err := cb.OnInstanceDelete(ctx, inst.ID); err != nil {
			failed++
			m.InstancesFailed++
			fmt.Fprintf(os.Stderr, "%s: delete %s: %v\n", who, inst.ID, err)
			continue
		}
		m.InstancesDeleted++
	}
	return failed
}

// Execute executes the blue-green deployment strategy.
func (s *BlueGreenStrategy) Execute(ctx context.Context, params *ExecuteParams) (*Result, error) {
	if params == nil {
		return nil, ErrInvalidConfig
	}
	if err := s.Validate(params.Config); err != nil {
		return nil, err
	}
	if err := requireDriver(TypeBlueGreen, params.Callbacks, true, false); err != nil {
		return nil, err
	}
	if params.Replicas < 1 {
		return nil, fmt.Errorf("%w: replicas must be at least 1", ErrInvalidConfig)
	}
	cb := params.Callbacks

	ctx, cancel := context.WithTimeout(ctx, progressDeadline(params.Config))
	defer cancel()

	deployment := &blueGreenDeployment{
		params: params,
		progress: &Progress{
			Phase:          "initializing",
			Message:        "Starting blue-green deployment",
			TotalInstances: params.Replicas,
		},
		result:        &Result{Metrics: &ResultMetrics{}},
		blueInstances: params.Instances,
		cancel:        cancel,
		done:          make(chan struct{}),
		trafficState:  "blue",
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
	progress := func(phase, msg string, pct int) {
		deployment.progress.Phase = phase
		deployment.progress.Message = msg
		deployment.progress.Percentage = pct
		if cb.OnProgress != nil {
			cb.OnProgress(ctx, deployment.progress)
		}
	}
	// abandonGreen deletes green (blue still serves) and fails the result.
	abandonGreen := func(msg string, err error) (*Result, error) {
		// A cancelled ctx must not stop the cleanup.
		deleteAll(context.WithoutCancel(ctx), cb, deployment.greenInstances, res.Metrics, "bluegreen")
		res.Success = false
		res.Message = msg
		res.Duration = time.Since(startTime)
		res.Instances = nil
		deployment.progress.Phase = "failed"
		return res, err
	}

	// Phase 1: deploy green.
	progress("deploying_green", "Deploying green environment", 0)
	for i := 0; i < params.Replicas; i++ {
		if err := ctx.Err(); err != nil {
			return abandonGreen("Deployment cancelled", err)
		}
		inst, err := cb.OnInstanceCreate(ctx, &InstanceParams{
			ServiceName: params.ServiceName,
			Version:     params.TargetVersion,
			Labels: map[string]string{
				"deployment":  params.DeploymentID,
				"version":     params.TargetVersion,
				"environment": "green",
			},
		})
		if err != nil || inst == nil {
			res.Metrics.InstancesFailed++
			return abandonGreen(fmt.Sprintf("Failed to create green instance: %v", err), ErrInstanceUpdateFailed)
		}
		inst.Healthy = false // green proves its health below, whatever the driver said
		deployment.greenInstances = append(deployment.greenInstances, *inst)
		res.Metrics.InstancesCreated++
		deployment.progress.UpdatedInstances = len(deployment.greenInstances)
	}

	// Phase 2: every green instance healthy.
	progress("health_check", "Waiting for green instances to become healthy", 30)
	if err := waitHealthy(ctx, cb, deployment.greenInstances, pollInterval(params.Config), &res.Metrics.HealthChecks); err != nil {
		return abandonGreen(fmt.Sprintf("Green environment not healthy: %v", err), ErrHealthCheckTimeout)
	}

	// Phase 3: optional soak.
	if params.Config.TestDuration > 0 {
		progress("testing", "Testing green environment", 60)
		if err := sleepCtx(ctx, params.Config.TestDuration); err != nil {
			return abandonGreen("Deployment cancelled during test", err)
		}
	}

	// Phase 4: switch traffic, once.
	progress("switching", "Switching traffic to green environment", 70)
	if err := cb.OnTrafficShift(ctx, &TrafficShiftParams{
		OldVersion: params.CurrentVersion,
		NewVersion: params.TargetVersion,
		Weight:     100,
	}); err != nil {
		return abandonGreen(fmt.Sprintf("Failed to switch traffic: %v", err), ErrTrafficShiftFailed)
	}
	s.mu.Lock()
	deployment.trafficState = "green"
	s.mu.Unlock()
	res.Metrics.TrafficShifts++

	// Phase 5: delete blue (traffic is on green, so finish even if ctx ends).
	progress("terminating_blue", "Terminating blue environment", 80)
	failed := deleteAll(context.WithoutCancel(ctx), cb, deployment.blueInstances, res.Metrics, "bluegreen")
	deployment.blueInstances = nil

	res.Success = true
	res.Message = fmt.Sprintf("Blue-green deployment completed: %d instances deployed", len(deployment.greenInstances))
	if failed > 0 {
		res.Message += fmt.Sprintf(" (%d blue instances could not be deleted)", failed)
	}
	res.Duration = time.Since(startTime)
	res.Instances = deployment.greenInstances
	progress("completed", "Deployment completed successfully", 100)
	return res, nil
}

// Rollback performs a rollback of the blue-green deployment: a fresh
// blue-green deployment of the target version.
func (s *BlueGreenStrategy) Rollback(ctx context.Context, params *RollbackParams) error {
	if params == nil {
		return ErrInvalidConfig
	}
	svc, err := rollbackServiceName(params)
	if err != nil {
		return err
	}
	replicas, err := rollbackReplicas(params)
	if err != nil {
		return err
	}
	s.mu.RLock()
	deployment, exists := s.deployments[params.DeploymentID]
	s.mu.RUnlock()
	if exists && deployment.cancel != nil {
		// In flight: cancelling abandons green (blue was never deleted).
		deployment.cancel()
		<-deployment.done
	}

	cfg := DefaultConfig(TypeBlueGreen)
	cfg.TestDuration = 0
	result, err := s.Execute(ctx, &ExecuteParams{
		DeploymentID:   params.DeploymentID + "-rollback",
		ServiceName:    svc,
		CurrentVersion: currentVersionOf(params.Instances, params.TargetVersion),
		TargetVersion:  params.TargetVersion,
		Replicas:       replicas,
		Config:         cfg,
		Instances:      params.Instances,
		Callbacks:      params.Callbacks,
	})
	if err != nil {
		return fmt.Errorf("rollback deployment failed: %w", err)
	}
	if !result.Success {
		return fmt.Errorf("rollback deployment failed: %s", result.Message)
	}
	return nil
}

// Status returns the current strategy execution status.
func (s *BlueGreenStrategy) Status(ctx context.Context, deploymentID string) (*Status, error) {
	s.mu.RLock()
	deployment, exists := s.deployments[deploymentID]
	if !exists {
		s.mu.RUnlock()
		return nil, ErrStrategyNotFound
	}
	state := deployment.trafficState
	s.mu.RUnlock()

	status := &Status{
		Phase:    deployment.progress.Phase,
		Progress: deployment.progress,
	}
	activeInstances := deployment.greenInstances
	if state == "blue" {
		activeInstances = deployment.blueInstances
	}
	for _, inst := range activeInstances {
		if inst.Healthy {
			status.Healthy++
		} else {
			status.Unhealthy++
		}
	}
	status.Instances = activeInstances
	return status, nil
}

func init() {
	RegisterStrategy(NewBlueGreenStrategy())
}
