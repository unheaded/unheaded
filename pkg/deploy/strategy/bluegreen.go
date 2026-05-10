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
// It deploys a complete parallel environment (green) and then switches traffic
// from the existing environment (blue) to the new one instantly.
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

// Execute executes the blue-green deployment strategy.
func (s *BlueGreenStrategy) Execute(ctx context.Context, params *ExecuteParams) (*Result, error) {
	if err := s.Validate(params.Config); err != nil {
		return nil, err
	}

	// Create cancellable context
	ctx, cancel := context.WithCancel(ctx)

	// Create deployment tracking
	deployment := &blueGreenDeployment{
		params: params,
		progress: &Progress{
			Phase:          "initializing",
			Message:        "Starting blue-green deployment",
			Percentage:     0,
			TotalInstances: params.Replicas,
		},
		result: &Result{
			Metrics: &ResultMetrics{},
		},
		blueInstances:  params.Instances, // Current instances are "blue"
		greenInstances: []Instance{},
		cancel:         cancel,
		done:           make(chan struct{}),
		trafficState:   "blue",
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

	// Phase 1: Deploy green environment
	deployment.progress.Phase = "deploying_green"
	deployment.progress.Message = "Deploying green environment"

	if params.Callbacks != nil && params.Callbacks.OnProgress != nil {
		params.Callbacks.OnProgress(ctx, deployment.progress)
	}

	for i := 0; i < params.Replicas; i++ {
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
					"deployment":  params.DeploymentID,
					"version":     params.TargetVersion,
					"environment": "green",
				},
			})
			if err != nil {
				deployment.result.Metrics.InstancesFailed++
				// For blue-green, a failed instance creation is critical
				deployment.result.Success = false
				deployment.result.Message = fmt.Sprintf("Failed to create green instance: %v", err)
				deployment.result.Duration = time.Since(startTime)
				return deployment.result, ErrInstanceUpdateFailed
			}
		} else {
			// Simulate instance creation
			inst = &Instance{
				ID:      fmt.Sprintf("green-%s-%d", params.TargetVersion, i),
				Version: params.TargetVersion,
				State:   InstanceRunning,
				Healthy: false,
				Labels: map[string]string{
					"environment": "green",
				},
			}
		}

		deployment.greenInstances = append(deployment.greenInstances, *inst)
		deployment.result.Metrics.InstancesCreated++

		deployment.progress.UpdatedInstances = len(deployment.greenInstances)
		deployment.progress.Percentage = (len(deployment.greenInstances) * 30) / params.Replicas
	}

	// Phase 2: Wait for green instances to become healthy
	deployment.progress.Phase = "health_check"
	deployment.progress.Message = "Waiting for green instances to become healthy"
	deployment.progress.Percentage = 30

	if params.Callbacks != nil && params.Callbacks.OnProgress != nil {
		params.Callbacks.OnProgress(ctx, deployment.progress)
	}

	healthyCount := 0
	maxRetries := 30 // Wait up to 30 iterations
	for attempt := 0; attempt < maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return s.buildCancelledResult(deployment, startTime), ctx.Err()
		default:
		}

		healthyCount = 0
		for i := range deployment.greenInstances {
			if deployment.greenInstances[i].Healthy {
				healthyCount++
				continue
			}

			healthy := true
			if params.Callbacks != nil && params.Callbacks.OnHealthCheck != nil {
				var err error
				healthy, err = params.Callbacks.OnHealthCheck(ctx, deployment.greenInstances[i].ID)
				if err != nil {
					healthy = false
				}
			}
			deployment.greenInstances[i].Healthy = healthy
			deployment.result.Metrics.HealthChecks++

			if healthy {
				healthyCount++
			}
		}

		deployment.progress.Percentage = 30 + (healthyCount * 30 / params.Replicas)
		deployment.progress.Message = fmt.Sprintf("Green instances healthy: %d/%d", healthyCount, params.Replicas)

		if params.Callbacks != nil && params.Callbacks.OnProgress != nil {
			params.Callbacks.OnProgress(ctx, deployment.progress)
		}

		if healthyCount == params.Replicas {
			break
		}

		time.Sleep(1 * time.Second)
	}

	if healthyCount < params.Replicas {
		deployment.result.Success = false
		deployment.result.Message = fmt.Sprintf("Green environment not healthy: %d/%d healthy", healthyCount, params.Replicas)
		deployment.result.Duration = time.Since(startTime)
		deployment.result.Instances = deployment.greenInstances
		return deployment.result, ErrHealthCheckTimeout
	}

	// Phase 3: Test green environment (optional)
	if params.Config.TestDuration > 0 {
		deployment.progress.Phase = "testing"
		deployment.progress.Message = "Testing green environment"
		deployment.progress.Percentage = 60

		if params.Callbacks != nil && params.Callbacks.OnProgress != nil {
			params.Callbacks.OnProgress(ctx, deployment.progress)
		}

		select {
		case <-ctx.Done():
			return s.buildCancelledResult(deployment, startTime), ctx.Err()
		case <-time.After(params.Config.TestDuration):
		}
	}

	// Phase 4: Switch traffic to green
	deployment.progress.Phase = "switching"
	deployment.progress.Message = "Switching traffic to green environment"
	deployment.progress.Percentage = 70

	if params.Callbacks != nil && params.Callbacks.OnProgress != nil {
		params.Callbacks.OnProgress(ctx, deployment.progress)
	}

	if params.Callbacks != nil && params.Callbacks.OnTrafficShift != nil {
		err := params.Callbacks.OnTrafficShift(ctx, &TrafficShiftParams{
			OldVersion: params.CurrentVersion,
			NewVersion: params.TargetVersion,
			Weight:     100, // Full traffic to green
		})
		if err != nil {
			deployment.result.Success = false
			deployment.result.Message = fmt.Sprintf("Failed to switch traffic: %v", err)
			deployment.result.Duration = time.Since(startTime)
			deployment.result.Instances = deployment.greenInstances
			return deployment.result, ErrTrafficShiftFailed
		}
	}

	deployment.trafficState = "green"
	deployment.result.Metrics.TrafficShifts++
	deployment.progress.Percentage = 80

	// Phase 5: Terminate blue environment
	deployment.progress.Phase = "terminating_blue"
	deployment.progress.Message = "Terminating blue environment"

	if params.Callbacks != nil && params.Callbacks.OnProgress != nil {
		params.Callbacks.OnProgress(ctx, deployment.progress)
	}

	for _, inst := range deployment.blueInstances {
		select {
		case <-ctx.Done():
			// Even if cancelled, traffic is on green - continue cleanup
		default:
		}

		if params.Callbacks != nil && params.Callbacks.OnInstanceDelete != nil {
			if err := params.Callbacks.OnInstanceDelete(ctx, inst.ID); err != nil {
				// Log but continue - blue instances are no longer serving traffic
				continue
			}
		}
		deployment.result.Metrics.InstancesDeleted++
	}

	deployment.blueInstances = nil

	// Build final result
	deployment.result.Success = true
	deployment.result.Message = fmt.Sprintf("Blue-green deployment completed: %d instances deployed", len(deployment.greenInstances))
	deployment.result.Duration = time.Since(startTime)
	deployment.result.Instances = deployment.greenInstances

	deployment.progress.Phase = "completed"
	deployment.progress.Percentage = 100
	deployment.progress.Message = "Deployment completed successfully"

	if params.Callbacks != nil && params.Callbacks.OnProgress != nil {
		params.Callbacks.OnProgress(ctx, deployment.progress)
	}

	return deployment.result, nil
}

// Rollback performs a rollback of the blue-green deployment.
func (s *BlueGreenStrategy) Rollback(ctx context.Context, params *RollbackParams) error {
	s.mu.RLock()
	deployment, exists := s.deployments[params.DeploymentID]
	s.mu.RUnlock()

	if exists {
		// If deployment is still in progress and blue is available
		if len(deployment.blueInstances) > 0 && deployment.trafficState == "green" {
			// Switch traffic back to blue
			if params.Callbacks != nil && params.Callbacks.OnTrafficShift != nil {
				err := params.Callbacks.OnTrafficShift(ctx, &TrafficShiftParams{
					OldVersion: deployment.params.TargetVersion,
					NewVersion: deployment.params.CurrentVersion,
					Weight:     100,
				})
				if err != nil {
					return fmt.Errorf("failed to rollback traffic: %w", err)
				}
			}
			deployment.trafficState = "blue"

			// Terminate green instances. We surface any callback errors to
			// stderr but don't fail rollback — the user's callback may
			// legitimately report partial failure (some instances already
			// gone) and we want to keep tearing down the rest.
			for _, inst := range deployment.greenInstances {
				if params.Callbacks != nil && params.Callbacks.OnInstanceDelete != nil {
					if err := params.Callbacks.OnInstanceDelete(ctx, inst.ID); err != nil {
						fmt.Fprintf(os.Stderr, "bluegreen: rollback delete %s: %v\n", inst.ID, err)
					}
				}
			}

			// Cancel deployment
			if deployment.cancel != nil {
				deployment.cancel()
			}

			return nil
		}
	}

	// If no in-progress deployment or blue not available, do a new blue-green deployment
	executeParams := &ExecuteParams{
		DeploymentID:   params.DeploymentID + "-rollback",
		ServiceName:    params.Instances[0].Labels["service"],
		CurrentVersion: params.Instances[0].Version,
		TargetVersion:  params.TargetVersion,
		Replicas:       len(params.Instances),
		Config:         DefaultConfig(TypeBlueGreen),
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
func (s *BlueGreenStrategy) Status(ctx context.Context, deploymentID string) (*Status, error) {
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

	// Count healthy/unhealthy in active environment
	activeInstances := deployment.greenInstances
	if deployment.trafficState == "blue" {
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

// buildCancelledResult builds a result for a cancelled deployment.
func (s *BlueGreenStrategy) buildCancelledResult(deployment *blueGreenDeployment, startTime time.Time) *Result {
	deployment.result.Success = false
	deployment.result.Message = "Deployment cancelled"
	deployment.result.Duration = time.Since(startTime)
	deployment.result.Instances = deployment.greenInstances
	return deployment.result
}

func init() {
	RegisterStrategy(NewBlueGreenStrategy())
}
