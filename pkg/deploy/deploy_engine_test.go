// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/logger"
)

// newTestEngine creates an Engine with a logger that writes to io.Discard,
// preventing nil-writer panics in tests that trigger logging.
func newTestEngine(cfg *EngineConfig) *Engine {
	engine := NewEngine(nil, nil, cfg)
	engine.log = logger.New(io.Discard).With().Str("component", "test").Logger()
	return engine
}

// --- DefaultEngineConfig tests ---

func TestDefaultEngineConfig(t *testing.T) {
	cfg := DefaultEngineConfig()
	if cfg == nil {
		t.Fatal("DefaultEngineConfig returned nil")
	}
	if cfg.DefaultTimeout != 30*time.Minute {
		t.Fatalf("expected 30m timeout, got %v", cfg.DefaultTimeout)
	}
	if cfg.HistoryLimit != 100 {
		t.Fatalf("expected history limit 100, got %d", cfg.HistoryLimit)
	}
	if cfg.HealthCheckRetries != 3 {
		t.Fatalf("expected 3 retries, got %d", cfg.HealthCheckRetries)
	}
	if cfg.DefaultStrategy != StrategyRolling {
		t.Fatalf("expected rolling strategy, got %s", cfg.DefaultStrategy)
	}
	if cfg.ParallelUpdates != 5 {
		t.Fatalf("expected 5 parallel updates, got %d", cfg.ParallelUpdates)
	}
}

// --- NewEngine tests ---

func TestNewEngineNilConfig(t *testing.T) {
	engine := NewEngine(nil, nil, nil)
	if engine == nil {
		t.Fatal("NewEngine returned nil")
	}
	if engine.config == nil {
		t.Fatal("engine config is nil")
	}
	if engine.config.DefaultTimeout != 30*time.Minute {
		t.Fatalf("expected default timeout, got %v", engine.config.DefaultTimeout)
	}
	if engine.deployments == nil {
		t.Fatal("deployments map not initialized")
	}
	if engine.history == nil {
		t.Fatal("history map not initialized")
	}
	if engine.inProgress == nil {
		t.Fatal("inProgress map not initialized")
	}
}

func TestNewEngineCustomConfig(t *testing.T) {
	cfg := &EngineConfig{
		DefaultTimeout:     10 * time.Minute,
		HistoryLimit:       50,
		HealthCheckRetries: 5,
		DefaultStrategy:    StrategyBlueGreen,
		ParallelUpdates:    3,
	}
	engine := newTestEngine(cfg)
	if engine.config.DefaultTimeout != 10*time.Minute {
		t.Fatalf("expected 10m timeout, got %v", engine.config.DefaultTimeout)
	}
	if engine.config.DefaultStrategy != StrategyBlueGreen {
		t.Fatalf("expected blue-green strategy, got %s", engine.config.DefaultStrategy)
	}
}

// --- ValidateSpec (exported) tests ---

func TestValidateSpecNil(t *testing.T) {
	err := ValidateSpec(nil)
	if !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("expected ErrInvalidSpec, got %v", err)
	}
}

func TestValidateSpecEmptyServiceName(t *testing.T) {
	err := ValidateSpec(&DeploymentSpec{Version: "v1"})
	if !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("expected ErrInvalidSpec, got %v", err)
	}
	if !strings.Contains(err.Error(), "service name") {
		t.Fatalf("expected 'service name' in error, got %v", err)
	}
}

func TestValidateSpecEmptyVersion(t *testing.T) {
	err := ValidateSpec(&DeploymentSpec{ServiceName: "svc"})
	if !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("expected ErrInvalidSpec, got %v", err)
	}
	if !strings.Contains(err.Error(), "version") {
		t.Fatalf("expected 'version' in error, got %v", err)
	}
}

func TestValidateSpecValid(t *testing.T) {
	err := ValidateSpec(&DeploymentSpec{ServiceName: "svc", Version: "v1"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// --- Engine.validateSpec (internal, via Deploy) tests ---

func TestEngineValidateSpecSetsDefaults(t *testing.T) {
	engine := newTestEngine(nil)

	spec := &DeploymentSpec{
		ServiceName: "svc",
		Version:     "v1",
		// Replicas 0 should default to 1
		// Strategy "" should default to rolling
		// Timeout 0 should default to 30m
	}

	err := engine.validateSpec(spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Replicas != 1 {
		t.Fatalf("expected replicas 1, got %d", spec.Replicas)
	}
	if spec.Strategy != StrategyRolling {
		t.Fatalf("expected rolling strategy, got %s", spec.Strategy)
	}
	if spec.Timeout != 30*time.Minute {
		t.Fatalf("expected 30m timeout, got %v", spec.Timeout)
	}
}

func TestEngineValidateSpecPreservesExplicit(t *testing.T) {
	engine := newTestEngine(nil)
	spec := &DeploymentSpec{
		ServiceName: "svc",
		Version:     "v1",
		Replicas:    5,
		Strategy:    StrategyCanary,
		Timeout:     10 * time.Minute,
	}
	err := engine.validateSpec(spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Replicas != 5 {
		t.Fatalf("replicas changed to %d", spec.Replicas)
	}
	if spec.Strategy != StrategyCanary {
		t.Fatalf("strategy changed to %s", spec.Strategy)
	}
	if spec.Timeout != 10*time.Minute {
		t.Fatalf("timeout changed to %v", spec.Timeout)
	}
}

// --- Deployment.Status tests ---

func TestDeploymentStatusNilMetrics(t *testing.T) {
	d := &Deployment{
		ID:      "d1",
		State:   StatePending,
		Message: "waiting",
	}
	status := d.Status()
	if status.DeploymentID != "d1" {
		t.Fatalf("expected d1, got %s", status.DeploymentID)
	}
	if status.InstancesReady != 0 {
		t.Fatalf("expected 0 instances ready, got %d", status.InstancesReady)
	}
	if status.State != StatePending {
		t.Fatalf("expected pending, got %s", status.State)
	}
	if status.Message != "waiting" {
		t.Fatalf("expected 'waiting', got %s", status.Message)
	}
}

func TestDeploymentStatusWithMetrics(t *testing.T) {
	d := &Deployment{
		ID:    "d2",
		State: StateRunning,
		Metrics: &DeploymentMetrics{
			HealthChecksPassed: 7,
		},
	}
	status := d.Status()
	if status.InstancesReady != 7 {
		t.Fatalf("expected 7 instances ready, got %d", status.InstancesReady)
	}
}

// --- Engine.buildConditions tests ---

func TestBuildConditionsCompleted(t *testing.T) {
	engine := newTestEngine(nil)
	now := time.Now()
	d := &Deployment{
		State:       StateCompleted,
		CompletedAt: now,
	}
	conditions := engine.buildConditions(d)
	if len(conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(conditions))
	}
	if conditions[0].Type != "Available" {
		t.Fatalf("expected Available, got %s", conditions[0].Type)
	}
	if conditions[0].Status != "True" {
		t.Fatalf("expected True, got %s", conditions[0].Status)
	}
	if conditions[0].Reason != "DeploymentCompleted" {
		t.Fatalf("expected DeploymentCompleted, got %s", conditions[0].Reason)
	}
}

func TestBuildConditionsFailed(t *testing.T) {
	engine := newTestEngine(nil)
	d := &Deployment{
		State: StateFailed,
		Error: "out of memory",
	}
	conditions := engine.buildConditions(d)
	if len(conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(conditions))
	}
	if conditions[0].Status != "False" {
		t.Fatalf("expected False, got %s", conditions[0].Status)
	}
	if conditions[0].Reason != "DeploymentFailed" {
		t.Fatalf("expected DeploymentFailed, got %s", conditions[0].Reason)
	}
	if conditions[0].Message != "out of memory" {
		t.Fatalf("expected error message, got %s", conditions[0].Message)
	}
}

func TestBuildConditionsDeploying(t *testing.T) {
	engine := newTestEngine(nil)
	for _, state := range []DeploymentState{StateDeploying, StateVerifying, StatePromoting} {
		d := &Deployment{State: state}
		conditions := engine.buildConditions(d)
		if len(conditions) != 1 {
			t.Fatalf("state %s: expected 1 condition, got %d", state, len(conditions))
		}
		if conditions[0].Type != "Progressing" {
			t.Fatalf("state %s: expected Progressing, got %s", state, conditions[0].Type)
		}
		if conditions[0].Reason != string(state) {
			t.Fatalf("state %s: expected reason %s, got %s", state, state, conditions[0].Reason)
		}
	}
}

func TestBuildConditionsPending(t *testing.T) {
	engine := newTestEngine(nil)
	d := &Deployment{State: StatePending}
	conditions := engine.buildConditions(d)
	if len(conditions) != 0 {
		t.Fatalf("expected 0 conditions for pending, got %d", len(conditions))
	}
}

// --- Engine.History tests ---

func TestEngineHistoryEmpty(t *testing.T) {
	engine := newTestEngine(nil)
	ctx := context.Background()

	history, err := engine.History(ctx, "nosvc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(history) != 0 {
		t.Fatalf("expected empty history, got %d", len(history))
	}
}

func TestEngineHistoryReturnsCopy(t *testing.T) {
	engine := newTestEngine(nil)
	ctx := context.Background()

	// Manually add to history
	engine.histMu.Lock()
	engine.history["svc"] = []*Deployment{
		{ID: "d1", Spec: &DeploymentSpec{ServiceName: "svc"}},
		{ID: "d2", Spec: &DeploymentSpec{ServiceName: "svc"}},
	}
	engine.histMu.Unlock()

	history, err := engine.History(ctx, "svc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2, got %d", len(history))
	}

	// Modify returned slice; original should be unaffected.
	history[0] = nil
	engine.histMu.RLock()
	if engine.history["svc"][0] == nil {
		t.Fatal("History returned reference, not copy")
	}
	engine.histMu.RUnlock()
}

// --- Engine.Cancel tests ---

func TestEngineCancelNotFound(t *testing.T) {
	engine := newTestEngine(nil)
	err := engine.Cancel(context.Background(), "missing")
	if !errors.Is(err, ErrDeploymentNotFound) {
		t.Fatalf("expected ErrDeploymentNotFound, got %v", err)
	}
}

func TestEngineCancelCompleted(t *testing.T) {
	engine := newTestEngine(nil)
	engine.deployMu.Lock()
	engine.deployments["d1"] = &Deployment{
		ID:    "d1",
		State: StateCompleted,
		Spec:  &DeploymentSpec{ServiceName: "svc"},
	}
	engine.deployMu.Unlock()

	err := engine.Cancel(context.Background(), "d1")
	if err == nil {
		t.Fatal("expected error cancelling completed deployment")
	}
	if !strings.Contains(err.Error(), "already completed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEngineCancelSuccess(t *testing.T) {
	engine := newTestEngine(nil)
	engine.deployMu.Lock()
	engine.deployments["d1"] = &Deployment{
		ID:        "d1",
		State:     StateDeploying,
		Spec:      &DeploymentSpec{ServiceName: "svc"},
		StartedAt: time.Now().Add(-time.Minute),
	}
	engine.deployMu.Unlock()
	engine.progMu.Lock()
	engine.inProgress["svc"] = "d1"
	engine.progMu.Unlock()

	err := engine.Cancel(context.Background(), "d1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	engine.deployMu.RLock()
	d := engine.deployments["d1"]
	engine.deployMu.RUnlock()
	if d.State != StateCancelled {
		t.Fatalf("expected cancelled, got %s", d.State)
	}
	if d.Message != "Deployment cancelled by user" {
		t.Fatalf("unexpected message: %s", d.Message)
	}
	if d.Duration == 0 {
		t.Fatal("expected non-zero duration")
	}

	// inProgress should be cleared
	engine.progMu.RLock()
	_, exists := engine.inProgress["svc"]
	engine.progMu.RUnlock()
	if exists {
		t.Fatal("expected inProgress to be cleared")
	}
}

// --- Engine.Promote tests ---

func TestEnginePromoteNotFound(t *testing.T) {
	engine := newTestEngine(nil)
	err := engine.Promote(context.Background(), "missing")
	if !errors.Is(err, ErrDeploymentNotFound) {
		t.Fatalf("expected ErrDeploymentNotFound, got %v", err)
	}
}

func TestEnginePromoteNonCanary(t *testing.T) {
	engine := newTestEngine(nil)
	engine.deployMu.Lock()
	engine.deployments["d1"] = &Deployment{
		ID:   "d1",
		Spec: &DeploymentSpec{ServiceName: "svc", Strategy: StrategyRolling},
	}
	engine.deployMu.Unlock()

	err := engine.Promote(context.Background(), "d1")
	if err == nil {
		t.Fatal("expected error promoting non-canary deployment")
	}
	if !strings.Contains(err.Error(), "canary") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnginePromoteCanary(t *testing.T) {
	engine := newTestEngine(nil)
	engine.deployMu.Lock()
	engine.deployments["d1"] = &Deployment{
		ID:   "d1",
		State: StateVerifying,
		Spec: &DeploymentSpec{ServiceName: "svc", Strategy: StrategyCanary},
	}
	engine.deployMu.Unlock()

	err := engine.Promote(context.Background(), "d1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	engine.deployMu.RLock()
	d := engine.deployments["d1"]
	engine.deployMu.RUnlock()
	if d.State != StatePromoting {
		t.Fatalf("expected promoting, got %s", d.State)
	}
}

// --- generateDeploymentID tests ---

func TestGenerateDeploymentIDUnique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := generateDeploymentID()
		if seen[id] {
			t.Fatalf("duplicate ID: %s", id)
		}
		seen[id] = true
		if !strings.HasPrefix(id, "deploy-") {
			t.Fatalf("expected deploy- prefix, got %s", id)
		}
	}
}

// --- Engine.Status tests ---

func TestEngineStatusNotFound(t *testing.T) {
	engine := newTestEngine(nil)
	_, err := engine.Status(context.Background(), "missing")
	if !errors.Is(err, ErrDeploymentNotFound) {
		t.Fatalf("expected ErrDeploymentNotFound, got %v", err)
	}
}

func TestEngineStatusFound(t *testing.T) {
	engine := newTestEngine(nil)
	engine.deployMu.Lock()
	engine.deployments["d1"] = &Deployment{
		ID:    "d1",
		State: StateDeploying,
		Spec:  &DeploymentSpec{ServiceName: "svc", Replicas: 3},
	}
	engine.deployMu.Unlock()

	status, err := engine.Status(context.Background(), "d1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Deployment.ID != "d1" {
		t.Fatalf("expected d1, got %s", status.Deployment.ID)
	}
	if status.DesiredReplicas != 3 {
		t.Fatalf("expected 3 desired replicas, got %d", status.DesiredReplicas)
	}
	// Conditions should be present for deploying state
	if len(status.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(status.Conditions))
	}
}

// --- Engine.addToHistory tests ---

func TestAddToHistoryTrimsToLimit(t *testing.T) {
	cfg := &EngineConfig{
		DefaultTimeout:  30 * time.Minute,
		HistoryLimit:    3,
		DefaultStrategy: StrategyRolling,
	}
	engine := newTestEngine(cfg)

	for i := 0; i < 5; i++ {
		d := &Deployment{
			ID:   fmt.Sprintf("d%d", i),
			Spec: &DeploymentSpec{ServiceName: "svc"},
		}
		engine.addToHistory(d)
	}

	engine.histMu.RLock()
	history := engine.history["svc"]
	engine.histMu.RUnlock()

	if len(history) != 3 {
		t.Fatalf("expected 3, got %d", len(history))
	}
	// Should keep the last 3
	if history[0].ID != "d2" {
		t.Fatalf("expected d2 first, got %s", history[0].ID)
	}
}

// --- Engine.Rollback tests ---

func TestEngineRollbackNotFound(t *testing.T) {
	engine := newTestEngine(nil)
	err := engine.Rollback(context.Background(), "missing")
	if !errors.Is(err, ErrDeploymentNotFound) {
		t.Fatalf("expected ErrDeploymentNotFound, got %v", err)
	}
}

func TestEngineRollbackNoHistory(t *testing.T) {
	engine := newTestEngine(nil)
	engine.deployMu.Lock()
	engine.deployments["d1"] = &Deployment{
		ID:   "d1",
		Spec: &DeploymentSpec{ServiceName: "svc", Version: "v2"},
	}
	engine.deployMu.Unlock()

	err := engine.Rollback(context.Background(), "d1")
	if !errors.Is(err, ErrNoRollbackAvailable) {
		t.Fatalf("expected ErrNoRollbackAvailable, got %v", err)
	}
}

// --- Error sentinel tests ---

func TestErrorSentinels(t *testing.T) {
	sentinels := []error{
		ErrDeploymentNotFound,
		ErrDeploymentInProgress,
		ErrDeploymentFailed,
		ErrRollbackFailed,
		ErrNoRollbackAvailable,
		ErrInvalidSpec,
		ErrHealthCheckFailed,
		ErrStrategyNotSupported,
		ErrArtifactNotFound,
		ErrPipelineAborted,
		ErrTrafficShiftFailed,
	}
	for _, err := range sentinels {
		if err == nil {
			t.Fatal("sentinel error should not be nil")
		}
		if err.Error() == "" {
			t.Fatal("sentinel error should have message")
		}
	}
}

// --- State and Strategy constants ---

func TestStateConstants(t *testing.T) {
	states := []DeploymentState{
		StatePending, StateBuilding, StateValidating, StateDeploying,
		StateVerifying, StatePromoting, StateCompleted, StateFailed,
		StateRolledBack, StateCancelled, StateRunning, StateRollingBack,
	}
	for _, s := range states {
		if s == "" {
			t.Fatal("state constant should not be empty")
		}
	}
}

func TestStrategyConstants(t *testing.T) {
	strategies := []StrategyType{
		StrategyRolling, StrategyBlueGreen, StrategyCanary, StrategyRecreate,
	}
	for _, s := range strategies {
		if s == "" {
			t.Fatal("strategy constant should not be empty")
		}
	}
}

// --- Engine.Deploy tests (integration) ---

func TestEngineDeployInvalidSpec(t *testing.T) {
	engine := newTestEngine(nil)
	_, err := engine.Deploy(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil spec")
	}
}

func TestEngineDeployDuplicateService(t *testing.T) {
	engine := newTestEngine(nil)
	engine.progMu.Lock()
	engine.inProgress["svc"] = "d-existing"
	engine.progMu.Unlock()

	_, err := engine.Deploy(context.Background(), &DeploymentSpec{
		ServiceName: "svc",
		Version:     "v1",
	})
	if !errors.Is(err, ErrDeploymentInProgress) {
		t.Fatalf("expected ErrDeploymentInProgress, got %v", err)
	}
}

func TestEngineDeploySuccess(t *testing.T) {
	engine := newTestEngine(nil)
	spec := &DeploymentSpec{
		ServiceName: "svc",
		Version:     "v1",
	}
	deployment, err := engine.Deploy(context.Background(), spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deployment == nil {
		t.Fatal("deployment is nil")
	}
	// ID and Spec are set before the goroutine starts, safe to read.
	if deployment.ID == "" {
		t.Fatal("deployment ID is empty")
	}
	if deployment.Spec != spec {
		t.Fatal("spec mismatch")
	}

	// All state reads must be under lock since Deploy spawns a background goroutine.
	engine.deployMu.RLock()
	state := deployment.State
	engine.deployMu.RUnlock()
	_ = state
}

// --- Engine.runHook tests ---

func TestEngineRunHookUnknownType(t *testing.T) {
	engine := newTestEngine(nil)
	err := engine.runHook(context.Background(), HookAction{
		Name: "test",
		Type: "unknown_type",
	})
	if err == nil {
		t.Fatal("expected error for unknown hook type")
	}
	if !strings.Contains(err.Error(), "unknown hook type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEngineRunHookExec(t *testing.T) {
	engine := newTestEngine(nil)
	err := engine.runHook(context.Background(), HookAction{
		Name: "test",
		Type: "exec",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEngineRunHookWotanNilPublisher(t *testing.T) {
	engine := newTestEngine(nil)
	err := engine.runHook(context.Background(), HookAction{
		Name:  "test",
		Type:  "wotan",
		Topic: "test.topic",
	})
	// Should not error - degraded mode
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- DeploymentStatusInfo struct tests ---

func TestDeploymentStatusInfoJSON(t *testing.T) {
	info := &DeploymentStatusInfo{
		DeploymentID:   "d1",
		State:          StateRunning,
		InstancesReady: 3,
		Message:        "deploying",
	}
	if info.DeploymentID != "d1" {
		t.Fatal("field mismatch")
	}
}

// --- emitEvent tests ---

func TestEmitEventNilWotan(t *testing.T) {
	engine := newTestEngine(nil)
	// Should not panic
	engine.emitEvent(context.Background(), "test.topic", map[string]interface{}{"key": "value"})
}
