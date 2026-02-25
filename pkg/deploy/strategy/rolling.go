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

// RollingStrategy implements a rolling update deployment strategy.
// It replaces instances one by one (or in batches) to minimize downtime.
type RollingStrategy struct {
	mu sync.RWMutex

	// deployments tracks active rolling deployments
	deployments map[string]*rollingDeployment
}

type rollingDeployment struct {
	params   *ExecuteParams
	progress *Progress
	result   *Result
	cancel   context.CancelFunc
	done     chan struct{}
}

// NewRollingStrategy creates a new rolling strategy.
func NewRollingStrategy() *RollingStrategy {
	return &RollingStrategy{
		deployments: make(map[string]*rollingDeployment),
	}
}

// Type returns the strategy type.
func (s *RollingStrategy) Type() Type {
	return TypeRolling
}

// Validate validates the strategy configuration.
func (s *RollingStrategy) Validate(config *Config) error {
	if config == nil {
		return ErrInvalidConfig
	}
	if config.Type != TypeRolling {
		return fmt.Errorf("expected rolling strategy config, got %s", config.Type)
	}
	if config.MaxSurge < 0 {
		return fmt.Errorf("max_surge must be non-negative")
	}
	if config.MaxUnavailable < 0 {
		return fmt.Errorf("max_unavailable must be non-negative")
	}
	if config.MaxSurge == 0 && config.MaxUnavailable == 0 {
		return fmt.Errorf("either max_surge or max_unavailable must be positive")
	}
	return nil
}

// Execute executes the rolling update strategy.
func (s *RollingStrategy) Execute(ctx context.Context, params *ExecuteParams) (*Result, error) {
	if err := s.Validate(params.Config); err != nil {
		return nil, err
	}

	// Create cancellable context
	ctx, cancel := context.WithCancel(ctx)

	// Create deployment tracking
	deployment := &rollingDeployment{
		params: params,
		progress: &Progress{
			Phase:          "initializing",
			Message:        "Starting rolling update",
			Percentage:     0,
			TotalInstances: params.Replicas,
		},
		result: &Result{
			Metrics: &ResultMetrics{},
		},
		cancel: cancel,
		done:   make(chan struct{}),
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

	// Get configuration values
	maxSurge := params.Config.MaxSurge
	if maxSurge == 0 {
		maxSurge = 1
	}
	maxUnavailable := params.Config.MaxUnavailable

	// Separate old and new instances
	var oldInstances, newInstances []Instance
	for _, inst := range params.Instances {
		if inst.Version == params.TargetVersion {
			newInstances = append(newInstances, inst)
		} else {
			oldInstances = append(oldInstances, inst)
		}
	}

	// Calculate how many instances need to be created/updated
	currentNew := len(newInstances)
	currentOld := len(oldInstances)
	targetNew := params.Replicas

	deployment.progress.Phase = "rolling"
	deployment.progress.Message = fmt.Sprintf("Rolling update: %d -> %d instances", currentOld, targetNew)

	// Rolling update loop with progress deadline to avoid infinite loops
	maxIterations := params.Replicas * 100 // generous upper bound
	if maxIterations < 100 {
		maxIterations = 100
	}
	iteration := 0
	lastProgress := 0

	for currentNew < targetNew || currentOld > 0 {
		iteration++
		currentProgress := currentNew*100 + (len(params.Instances)-currentOld)*10
		if currentProgress != lastProgress {
			lastProgress = currentProgress
			iteration = 0 // reset stall counter on progress
		}
		if iteration > maxIterations {
			deployment.result.Success = false
			deployment.result.Message = "Rolling update stalled: no progress detected"
			deployment.result.Duration = time.Since(startTime)
			deployment.result.Instances = newInstances
			return deployment.result, nil
		}
		select {
		case <-ctx.Done():
			deployment.result.Success = false
			deployment.result.Message = "Deployment cancelled"
			deployment.result.Duration = time.Since(startTime)
			return deployment.result, ctx.Err()
		default:
		}

		// Calculate how many new instances we can create
		total := currentNew + currentOld
		maxTotal := params.Replicas + maxSurge
		canCreate := maxTotal - total
		if canCreate < 0 {
			canCreate = 0
		}
		needCreate := targetNew - currentNew
		toCreate := min(canCreate, needCreate)

		// Calculate how many old instances we can delete
		minAvailable := params.Replicas - maxUnavailable
		if minAvailable < 0 {
			minAvailable = 0
		}
		// Count healthy instances
		healthyNew := 0
		for _, inst := range newInstances {
			if inst.Healthy {
				healthyNew++
			}
		}
		healthyOld := 0
		for _, inst := range oldInstances {
			if inst.Healthy {
				healthyOld++
			}
		}
		availableAfterDelete := healthyNew + healthyOld - 1
		canDelete := 0
		if availableAfterDelete >= minAvailable && currentOld > 0 {
			canDelete = 1
		}

		// Create new instances
		for i := 0; i < toCreate; i++ {
			if params.Callbacks != nil && params.Callbacks.OnInstanceCreate != nil {
				inst, err := params.Callbacks.OnInstanceCreate(ctx, &InstanceParams{
					ServiceName: params.ServiceName,
					Version:     params.TargetVersion,
					Labels: map[string]string{
						"deployment": params.DeploymentID,
						"version":    params.TargetVersion,
					},
				})
				if err != nil {
					deployment.result.Metrics.InstancesFailed++
					continue
				}
				newInstances = append(newInstances, *inst)
				deployment.result.Metrics.InstancesCreated++
			} else {
				// Simulate instance creation
				newInstances = append(newInstances, Instance{
					ID:      fmt.Sprintf("inst-%s-%d", params.TargetVersion, len(newInstances)),
					Version: params.TargetVersion,
					State:   InstanceRunning,
					Healthy: false,
				})
				deployment.result.Metrics.InstancesCreated++
			}
			currentNew++
		}

		// Wait for new instances to become healthy
		for i := range newInstances {
			if !newInstances[i].Healthy {
				healthy := true
				if params.Callbacks != nil && params.Callbacks.OnHealthCheck != nil {
					var err error
					healthy, err = params.Callbacks.OnHealthCheck(ctx, newInstances[i].ID)
					if err != nil {
						healthy = false
					}
				}
				newInstances[i].Healthy = healthy
				deployment.result.Metrics.HealthChecks++
				if healthy {
					healthyNew++
				}
			}
		}

		// Delete old instances
		for i := 0; i < canDelete && len(oldInstances) > 0; i++ {
			inst := oldInstances[0]
			oldInstances = oldInstances[1:]

			if params.Callbacks != nil && params.Callbacks.OnInstanceDelete != nil {
				if err := params.Callbacks.OnInstanceDelete(ctx, inst.ID); err != nil {
					// Failed to delete, add back to list
					oldInstances = append(oldInstances, inst)
					continue
				}
			}
			deployment.result.Metrics.InstancesDeleted++
			currentOld--
		}

		// Update progress
		deployment.progress.UpdatedInstances = currentNew
		deployment.progress.Percentage = (currentNew * 100) / targetNew
		deployment.progress.Message = fmt.Sprintf("Rolling update: %d/%d instances updated", currentNew, targetNew)

		if params.Callbacks != nil && params.Callbacks.OnProgress != nil {
			params.Callbacks.OnProgress(ctx, deployment.progress)
		}

		// Small delay to avoid tight loop
		time.Sleep(100 * time.Millisecond)
	}

	// Final health verification
	deployment.progress.Phase = "verifying"
	deployment.progress.Message = "Verifying deployment health"

	allHealthy := true
	for i := range newInstances {
		if params.Callbacks != nil && params.Callbacks.OnHealthCheck != nil {
			healthy, err := params.Callbacks.OnHealthCheck(ctx, newInstances[i].ID)
			if err != nil || !healthy {
				allHealthy = false
				newInstances[i].Healthy = false
			} else {
				newInstances[i].Healthy = true
			}
			deployment.result.Metrics.HealthChecks++
		}
	}

	// Build result
	deployment.result.Duration = time.Since(startTime)
	deployment.result.Instances = newInstances

	if allHealthy {
		deployment.result.Success = true
		deployment.result.Message = fmt.Sprintf("Rolling update completed: %d instances updated", len(newInstances))
		deployment.progress.Phase = "completed"
		deployment.progress.Percentage = 100
	} else {
		deployment.result.Success = false
		deployment.result.Message = "Rolling update completed with unhealthy instances"
		deployment.progress.Phase = "degraded"
	}

	if params.Callbacks != nil && params.Callbacks.OnProgress != nil {
		params.Callbacks.OnProgress(ctx, deployment.progress)
	}

	return deployment.result, nil
}

// Rollback performs a rollback of the rolling deployment.
func (s *RollingStrategy) Rollback(ctx context.Context, params *RollbackParams) error {
	// Cancel any in-progress deployment
	s.mu.RLock()
	deployment, exists := s.deployments[params.DeploymentID]
	s.mu.RUnlock()

	if exists && deployment.cancel != nil {
		deployment.cancel()
		<-deployment.done
	}

	// Execute rollback as another rolling update to the target version
	executeParams := &ExecuteParams{
		DeploymentID:   params.DeploymentID + "-rollback",
		ServiceName:    params.Instances[0].Labels["service"],
		CurrentVersion: params.Instances[0].Version,
		TargetVersion:  params.TargetVersion,
		Replicas:       len(params.Instances),
		Config:         DefaultConfig(TypeRolling),
		Instances:      params.Instances,
		Callbacks:      params.Callbacks,
	}

	result, err := s.Execute(ctx, executeParams)
	if err != nil {
		return fmt.Errorf("rollback failed: %w", err)
	}
	if !result.Success {
		return fmt.Errorf("rollback failed: %s", result.Message)
	}

	return nil
}

// Status returns the current strategy execution status.
func (s *RollingStrategy) Status(ctx context.Context, deploymentID string) (*Status, error) {
	s.mu.RLock()
	deployment, exists := s.deployments[deploymentID]
	s.mu.RUnlock()

	if !exists {
		return nil, ErrStrategyNotFound
	}

	status := &Status{
		Phase:    deployment.progress.Phase,
		Progress: deployment.progress,
	}

	// Count healthy/unhealthy instances
	if deployment.result != nil {
		for _, inst := range deployment.result.Instances {
			if inst.Healthy {
				status.Healthy++
			} else {
				status.Unhealthy++
			}
		}
		status.Instances = deployment.result.Instances
	}

	return status, nil
}

// min returns the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func init() {
	RegisterStrategy(NewRollingStrategy())
}
