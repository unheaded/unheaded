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

// RollingStrategy implements a rolling update deployment strategy.
// It replaces instances one by one (or in batches) to minimize downtime:
// create up to MaxSurge new instances, wait until they are healthy, then
// delete old ones while at least Replicas-MaxUnavailable stay healthy.
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

func countHealthy(insts []Instance) int {
	n := 0
	for _, in := range insts {
		if in.Healthy {
			n++
		}
	}
	return n
}

// Execute executes the rolling update strategy.
func (s *RollingStrategy) Execute(ctx context.Context, params *ExecuteParams) (*Result, error) {
	if params == nil {
		return nil, ErrInvalidConfig
	}
	if err := s.Validate(params.Config); err != nil {
		return nil, err
	}
	if err := requireDriver(TypeRolling, params.Callbacks, false, false); err != nil {
		return nil, err
	}
	if params.Replicas < 1 {
		return nil, fmt.Errorf("%w: replicas must be at least 1", ErrInvalidConfig)
	}
	cb := params.Callbacks

	ctx, cancel := context.WithTimeout(ctx, progressDeadline(params.Config))
	defer cancel()

	deployment := &rollingDeployment{
		params: params,
		progress: &Progress{
			Phase:          "initializing",
			Message:        "Starting rolling update",
			TotalInstances: params.Replicas,
		},
		result: &Result{Metrics: &ResultMetrics{}},
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
	res := deployment.result
	interval := pollInterval(params.Config)
	maxSurge := params.Config.MaxSurge
	minAvailable := max(params.Replicas-params.Config.MaxUnavailable, 0)

	var oldInstances, newInstances []Instance
	for _, inst := range params.Instances {
		if inst.Version == params.TargetVersion {
			newInstances = append(newInstances, inst)
		} else {
			oldInstances = append(oldInstances, inst)
		}
	}
	fail := func(msg string, err error) (*Result, error) {
		res.Success = false
		res.Message = msg
		res.Duration = time.Since(startTime)
		res.Instances = newInstances
		deployment.progress.Phase = "failed"
		return res, err
	}

	deployment.progress.Phase = "rolling"
	createFailures := 0
	for len(newInstances) < params.Replicas || len(oldInstances) > 0 {
		if err := ctx.Err(); err != nil {
			return fail(fmt.Sprintf("Rolling update stopped: %v", err), err)
		}
		progressed := false

		// Surge: create while under Replicas+MaxSurge in total.
		for len(newInstances) < params.Replicas && len(newInstances)+len(oldInstances) < params.Replicas+maxSurge {
			inst, err := cb.OnInstanceCreate(ctx, &InstanceParams{
				ServiceName: params.ServiceName,
				Version:     params.TargetVersion,
				Labels:      map[string]string{"deployment": params.DeploymentID, "version": params.TargetVersion},
			})
			if err != nil || inst == nil {
				res.Metrics.InstancesFailed++
				createFailures++
				if createFailures > params.Replicas {
					return fail(fmt.Sprintf("Rolling update failed: %d instance creations failed (last: %v)", createFailures, err), ErrInstanceUpdateFailed)
				}
				break
			}
			newInstances = append(newInstances, *inst)
			res.Metrics.InstancesCreated++
			progressed = true
		}

		// Ask about every new instance not yet healthy, once per pass.
		for i := range newInstances {
			if newInstances[i].Healthy {
				continue
			}
			ok, err := cb.OnHealthCheck(ctx, newInstances[i].ID)
			res.Metrics.HealthChecks++
			if err == nil && ok {
				newInstances[i].Healthy = true
				progressed = true
			}
		}

		// Delete old instances while availability holds; unhealthy ones first.
		for len(oldInstances) > 0 {
			idx := 0
			for i, in := range oldInstances {
				if !in.Healthy {
					idx = i
					break
				}
			}
			victim := oldInstances[idx]
			after := countHealthy(newInstances) + countHealthy(oldInstances)
			if victim.Healthy {
				after--
			}
			if after < minAvailable && len(newInstances)+len(oldInstances) <= params.Replicas+maxSurge {
				// Only an excess (beyond Replicas+MaxSurge) may be removed
				// at the cost of availability; otherwise wait for health.
				break
			}
			if err := cb.OnInstanceDelete(ctx, victim.ID); err != nil {
				res.Metrics.InstancesFailed++
				break // retried next pass
			}
			oldInstances = append(oldInstances[:idx:idx], oldInstances[idx+1:]...)
			res.Metrics.InstancesDeleted++
			progressed = true
		}

		deployment.progress.UpdatedInstances = countHealthy(newInstances)
		deployment.progress.Percentage = min(countHealthy(newInstances)*100/params.Replicas, 100)
		deployment.progress.Message = fmt.Sprintf("Rolling update: %d/%d new healthy, %d old left",
			countHealthy(newInstances), params.Replicas, len(oldInstances))
		if cb.OnProgress != nil {
			cb.OnProgress(ctx, deployment.progress)
		}

		if !progressed {
			if err := sleepCtx(ctx, interval); err != nil {
				msg := fmt.Sprintf("Rolling update did not finish: %d/%d new instances healthy, %d old left",
					countHealthy(newInstances), params.Replicas, len(oldInstances))
				if errors.Is(err, context.DeadlineExceeded) {
					return fail(msg, ErrHealthCheckTimeout)
				}
				return fail(msg, err)
			}
		}
	}

	// Final verification: every new instance healthy now.
	deployment.progress.Phase = "verifying"
	for i := range newInstances {
		newInstances[i].Healthy = false
	}
	if err := waitHealthy(ctx, cb, newInstances, interval, &res.Metrics.HealthChecks); err != nil {
		return fail("Rolling update finished with unhealthy instances", err)
	}

	res.Success = true
	res.Message = fmt.Sprintf("Rolling update completed: %d instances updated", len(newInstances))
	res.Duration = time.Since(startTime)
	res.Instances = newInstances
	deployment.progress.Phase = "completed"
	deployment.progress.Percentage = 100
	if cb.OnProgress != nil {
		cb.OnProgress(ctx, deployment.progress)
	}
	return res, nil
}

// Rollback performs a rollback of the rolling deployment.
func (s *RollingStrategy) Rollback(ctx context.Context, params *RollbackParams) error {
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
		deployment.cancel()
		<-deployment.done
	}

	result, err := s.Execute(ctx, &ExecuteParams{
		DeploymentID:   params.DeploymentID + "-rollback",
		ServiceName:    svc,
		CurrentVersion: currentVersionOf(params.Instances, params.TargetVersion),
		TargetVersion:  params.TargetVersion,
		Replicas:       replicas,
		Config:         DefaultConfig(TypeRolling),
		Instances:      params.Instances,
		Callbacks:      params.Callbacks,
	})
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

func init() {
	RegisterStrategy(NewRollingStrategy())
}
