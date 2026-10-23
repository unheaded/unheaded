// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package strategy

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// RecreateStrategy deletes every old instance, then creates Replicas new
// ones and waits until all are healthy. It accepts downtime; it is the
// baseline for a service with nothing running yet, or one whose versions
// cannot run side by side.
type RecreateStrategy struct {
	mu          sync.RWMutex
	deployments map[string]*Progress
}

// NewRecreateStrategy creates a new recreate strategy.
func NewRecreateStrategy() *RecreateStrategy {
	return &RecreateStrategy{deployments: make(map[string]*Progress)}
}

// Type returns the strategy type.
func (s *RecreateStrategy) Type() Type { return TypeRecreate }

// Validate validates the strategy configuration.
func (s *RecreateStrategy) Validate(config *Config) error {
	if config == nil {
		return ErrInvalidConfig
	}
	if config.Type != TypeRecreate {
		return fmt.Errorf("expected recreate strategy config, got %s", config.Type)
	}
	return nil
}

// Execute executes the recreate strategy.
func (s *RecreateStrategy) Execute(ctx context.Context, params *ExecuteParams) (*Result, error) {
	if params == nil {
		return nil, ErrInvalidConfig
	}
	if err := s.Validate(params.Config); err != nil {
		return nil, err
	}
	if err := requireDriver(TypeRecreate, params.Callbacks, false, false); err != nil {
		return nil, err
	}
	if params.Replicas < 1 {
		return nil, fmt.Errorf("%w: replicas must be at least 1", ErrInvalidConfig)
	}
	cb := params.Callbacks
	ctx, cancel := context.WithTimeout(ctx, progressDeadline(params.Config))
	defer cancel()

	prog := &Progress{Phase: "terminating", Message: "Deleting old instances", TotalInstances: params.Replicas}
	s.mu.Lock()
	s.deployments[params.DeploymentID] = prog
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.deployments, params.DeploymentID)
		s.mu.Unlock()
	}()

	start := time.Now()
	res := &Result{Metrics: &ResultMetrics{}}
	fail := func(msg string, err error) (*Result, error) {
		res.Success = false
		res.Message = msg
		res.Duration = time.Since(start)
		return res, err
	}

	if failed := deleteAll(ctx, cb, params.Instances, res.Metrics, "recreate"); failed > 0 {
		return fail(fmt.Sprintf("Recreate stopped: %d old instances could not be deleted", failed), ErrInstanceUpdateFailed)
	}

	s.setPhase(prog, "creating", "Creating new instances", 0)
	var created []Instance
	for i := 0; i < params.Replicas; i++ {
		inst, err := cb.OnInstanceCreate(ctx, &InstanceParams{
			ServiceName: params.ServiceName,
			Version:     params.TargetVersion,
			Labels:      map[string]string{"deployment": params.DeploymentID, "version": params.TargetVersion},
		})
		if err != nil || inst == nil {
			res.Metrics.InstancesFailed++
			res.Instances = created
			return fail(fmt.Sprintf("Failed to create instance %d of %d: %v", i+1, params.Replicas, err), ErrInstanceUpdateFailed)
		}
		inst.Healthy = false
		created = append(created, *inst)
		res.Metrics.InstancesCreated++
	}

	s.setPhase(prog, "health_check", "Waiting for new instances to become healthy", 50)
	res.Instances = created
	if err := waitHealthy(ctx, cb, created, pollInterval(params.Config), &res.Metrics.HealthChecks); err != nil {
		return fail(fmt.Sprintf("New instances not healthy: %v", err), err)
	}

	res.Success = true
	res.Message = fmt.Sprintf("Recreate completed: %d instances running %s", len(created), params.TargetVersion)
	res.Duration = time.Since(start)
	s.setPhase(prog, "completed", res.Message, 100)
	return res, nil
}

func (s *RecreateStrategy) setPhase(p *Progress, phase, msg string, pct int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p.Phase, p.Message, p.Percentage = phase, msg, pct
}

// Rollback recreates the target version.
func (s *RecreateStrategy) Rollback(ctx context.Context, params *RollbackParams) error {
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
	res, err := s.Execute(ctx, &ExecuteParams{
		DeploymentID:   params.DeploymentID + "-rollback",
		ServiceName:    svc,
		CurrentVersion: currentVersionOf(params.Instances, params.TargetVersion),
		TargetVersion:  params.TargetVersion,
		Replicas:       replicas,
		Config:         DefaultConfig(TypeRecreate),
		Instances:      params.Instances,
		Callbacks:      params.Callbacks,
	})
	if err != nil {
		return fmt.Errorf("rollback failed: %w", err)
	}
	if !res.Success {
		return fmt.Errorf("rollback failed: %s", res.Message)
	}
	return nil
}

// Status returns the current strategy execution status.
func (s *RecreateStrategy) Status(_ context.Context, deploymentID string) (*Status, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.deployments[deploymentID]
	if !ok {
		return nil, ErrStrategyNotFound
	}
	cp := *p
	return &Status{Phase: cp.Phase, Progress: &cp}, nil
}

func init() {
	RegisterStrategy(NewRecreateStrategy())
}
