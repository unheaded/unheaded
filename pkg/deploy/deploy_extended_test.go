// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/container"
	"unheaded/pkg/logger"
)

// mockWotanPublisher implements WotanPublisher for testing.
type mockWotanPublisher struct {
	mu       sync.Mutex
	messages []mockMessage
	err      error
}

type mockMessage struct {
	topic   string
	payload []byte
}

func (m *mockWotanPublisher) Publish(_ context.Context, topic string, payload []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.messages = append(m.messages, mockMessage{topic: topic, payload: payload})
	return nil
}

func (m *mockWotanPublisher) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.messages)
}

// newTestEngineWithWotan creates an Engine with a working mock WotanPublisher.
func newTestEngineWithWotan(cfg *EngineConfig) (*Engine, *mockWotanPublisher) {
	pub := &mockWotanPublisher{}
	engine := NewEngine(nil, pub, cfg)
	engine.log = logger.New(io.Discard).With().Str("component", "test").Logger()
	return engine, pub
}

// newTestEngineWithRuntime creates an Engine with a mock container runtime.
func newTestEngineWithRuntime(cfg *EngineConfig) (*Engine, *container.TestableRuntime) {
	rt := container.NewTestableRuntime("docker")
	engine := NewEngine(rt, nil, cfg)
	engine.log = logger.New(io.Discard).With().Str("component", "test").Logger()
	return engine, rt
}

// --- emitEvent with working Wotan ---

func TestEmitEventWithWotan(t *testing.T) {
	engine, pub := newTestEngineWithWotan(nil)
	engine.emitEvent(context.Background(), "test.topic", map[string]interface{}{
		"key": "value",
	})
	if pub.count() != 1 {
		t.Fatalf("expected 1 message, got %d", pub.count())
	}
}

func TestEmitEventWotanPublishError(t *testing.T) {
	engine, pub := newTestEngineWithWotan(nil)
	pub.err = errors.New("publish error")
	// Should not panic; error is logged
	engine.emitEvent(context.Background(), "test.topic", map[string]interface{}{
		"key": "value",
	})
}

// --- handleDeploymentFailure ---

func TestHandleDeploymentFailure(t *testing.T) {
	engine := newTestEngine(nil)
	deployment := &Deployment{
		ID:        "d-fail",
		State:     StateDeploying,
		StartedAt: time.Now().Add(-time.Minute),
		Spec:      &DeploymentSpec{ServiceName: "svc", Version: "v1", Strategy: StrategyRolling},
		Metrics:   &DeploymentMetrics{},
	}
	engine.deployMu.Lock()
	engine.deployments[deployment.ID] = deployment
	engine.deployMu.Unlock()

	engine.handleDeploymentFailure(context.Background(), deployment, errors.New("test failure"))

	engine.deployMu.RLock()
	defer engine.deployMu.RUnlock()
	if deployment.State != StateFailed {
		t.Fatalf("expected failed state, got %s", deployment.State)
	}
	if deployment.Error != "test failure" {
		t.Fatalf("expected 'test failure', got %s", deployment.Error)
	}
	if !strings.Contains(deployment.Message, "test failure") {
		t.Fatalf("expected message to contain error, got %s", deployment.Message)
	}
	if deployment.Duration == 0 {
		t.Fatal("expected non-zero duration")
	}
}

func TestHandleDeploymentFailureAutoRollback(t *testing.T) {
	engine := newTestEngine(nil)

	// Create a previous successful deployment in history
	prevDeployment := &Deployment{
		ID:    "d-prev",
		State: StateCompleted,
		Spec:  &DeploymentSpec{ServiceName: "svc", Version: "v1", ArtifactRef: "art:v1", Strategy: StrategyRolling, Replicas: 1},
	}
	engine.histMu.Lock()
	engine.history["svc"] = []*Deployment{prevDeployment}
	engine.histMu.Unlock()

	// Create the current deployment with AutoRollback
	deployment := &Deployment{
		ID:        "d-current",
		State:     StateDeploying,
		StartedAt: time.Now().Add(-time.Minute),
		Spec: &DeploymentSpec{
			ServiceName:  "svc",
			Version:      "v2",
			Strategy:     StrategyRolling,
			Replicas:     1,
			AutoRollback: true,
		},
		Metrics: &DeploymentMetrics{},
	}
	engine.deployMu.Lock()
	engine.deployments[deployment.ID] = deployment
	engine.deployMu.Unlock()

	// Add current to history so rollback has >= 2 entries
	engine.histMu.Lock()
	engine.history["svc"] = append(engine.history["svc"], deployment)
	engine.histMu.Unlock()

	engine.handleDeploymentFailure(context.Background(), deployment, errors.New("deploy broke"))

	// After auto-rollback, deployment should be marked as rolled back
	engine.deployMu.RLock()
	state := deployment.State
	rollbackID := deployment.RollbackID
	engine.deployMu.RUnlock()
	if state != StateRolledBack {
		t.Fatalf("expected rolled_back state, got %s", state)
	}
	if rollbackID == "" {
		t.Fatal("expected rollback ID to be set")
	}
}

func TestHandleDeploymentFailureAutoRollbackNoHistory(t *testing.T) {
	engine := newTestEngine(nil)
	deployment := &Deployment{
		ID:        "d-nohistory",
		State:     StateDeploying,
		StartedAt: time.Now().Add(-time.Minute),
		Spec: &DeploymentSpec{
			ServiceName:  "svc",
			Version:      "v1",
			Strategy:     StrategyRolling,
			Replicas:     1,
			AutoRollback: true,
		},
		Metrics: &DeploymentMetrics{},
	}
	engine.deployMu.Lock()
	engine.deployments[deployment.ID] = deployment
	engine.deployMu.Unlock()

	// Should not panic even with no rollback available
	engine.handleDeploymentFailure(context.Background(), deployment, errors.New("fail"))
}

// --- deployBlueGreen ---

func TestDeployBlueGreen(t *testing.T) {
	engine := newTestEngine(nil)
	deployment := &Deployment{
		ID:   "d-bg",
		Spec: &DeploymentSpec{ServiceName: "svc", Version: "v1", Replicas: 3, Strategy: StrategyBlueGreen},
		Metrics: &DeploymentMetrics{},
	}

	err := engine.deployBlueGreen(context.Background(), deployment)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deployment.Metrics.InstancesUpdated != 3 {
		t.Fatalf("expected 3 instances updated, got %d", deployment.Metrics.InstancesUpdated)
	}
	if deployment.Metrics.TrafficShifts != 1 {
		t.Fatalf("expected 1 traffic shift, got %d", deployment.Metrics.TrafficShifts)
	}
}

func TestDeployBlueGreenCancelled(t *testing.T) {
	engine := newTestEngine(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	deployment := &Deployment{
		ID:      "d-bg-cancel",
		Spec:    &DeploymentSpec{ServiceName: "svc", Version: "v1", Replicas: 3, Strategy: StrategyBlueGreen},
		Metrics: &DeploymentMetrics{},
	}

	err := engine.deployBlueGreen(ctx, deployment)
	if err == nil {
		t.Fatal("expected context error")
	}
}

// --- deployRecreate ---

func TestDeployRecreate(t *testing.T) {
	engine := newTestEngine(nil)
	deployment := &Deployment{
		ID:      "d-recreate",
		Spec:    &DeploymentSpec{ServiceName: "svc", Version: "v1", Replicas: 5, Strategy: StrategyRecreate},
		Metrics: &DeploymentMetrics{},
	}

	err := engine.deployRecreate(context.Background(), deployment)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deployment.Metrics.InstancesUpdated != 5 {
		t.Fatalf("expected 5 instances updated, got %d", deployment.Metrics.InstancesUpdated)
	}
}

func TestDeployRecreateCancelled(t *testing.T) {
	engine := newTestEngine(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	deployment := &Deployment{
		ID:      "d-recreate-cancel",
		Spec:    &DeploymentSpec{ServiceName: "svc", Version: "v1", Replicas: 3, Strategy: StrategyRecreate},
		Metrics: &DeploymentMetrics{},
	}

	err := engine.deployRecreate(ctx, deployment)
	if err == nil {
		t.Fatal("expected context error")
	}
}

// --- deployCanary ---

func TestDeployCanaryCustomSteps(t *testing.T) {
	engine := newTestEngine(nil)
	deployment := &Deployment{
		ID: "d-canary",
		Spec: &DeploymentSpec{
			ServiceName: "svc",
			Version:     "v1",
			Replicas:    1,
			Strategy:    StrategyCanary,
			StrategyConfig: &StrategyConfig{
				CanarySteps: []CanaryStep{
					{Weight: 50, Duration: 0},
					{Weight: 100, Duration: 0},
				},
			},
		},
		Metrics: &DeploymentMetrics{},
	}

	err := engine.deployCanary(context.Background(), deployment)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deployment.Metrics.TrafficShifts != 2 {
		t.Fatalf("expected 2 traffic shifts, got %d", deployment.Metrics.TrafficShifts)
	}
}

func TestDeployCanaryPromotionSkipsRemaining(t *testing.T) {
	engine := newTestEngine(nil)
	deployment := &Deployment{
		ID:    "d-canary-promote",
		State: StatePromoting, // pre-set to promoting
		Spec: &DeploymentSpec{
			ServiceName: "svc",
			Version:     "v1",
			Replicas:    1,
			Strategy:    StrategyCanary,
			StrategyConfig: &StrategyConfig{
				CanarySteps: []CanaryStep{
					{Weight: 10, Duration: 0},
					{Weight: 50, Duration: 0},
					{Weight: 100, Duration: 0},
				},
			},
		},
		Metrics: &DeploymentMetrics{},
	}
	engine.deployMu.Lock()
	engine.deployments[deployment.ID] = deployment
	engine.deployMu.Unlock()

	err := engine.deployCanary(context.Background(), deployment)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should have shifted once then exited due to promoting state
	if deployment.Metrics.TrafficShifts != 1 {
		t.Fatalf("expected 1 traffic shift (promoted early), got %d", deployment.Metrics.TrafficShifts)
	}
}

func TestDeployCanaryCancelled(t *testing.T) {
	engine := newTestEngine(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	deployment := &Deployment{
		ID: "d-canary-cancel",
		Spec: &DeploymentSpec{
			ServiceName: "svc",
			Version:     "v1",
			Replicas:    1,
			Strategy:    StrategyCanary,
			StrategyConfig: &StrategyConfig{
				CanarySteps: []CanaryStep{
					{Weight: 50, Duration: 0},
				},
			},
		},
		Metrics: &DeploymentMetrics{},
	}

	err := engine.deployCanary(ctx, deployment)
	if err == nil {
		t.Fatal("expected context error")
	}
}

// --- runHealthCheck ---

func TestRunHealthCheckNilSpec(t *testing.T) {
	engine := newTestEngine(nil)
	deployment := &Deployment{
		ID:      "d-hc-nil",
		Spec:    &DeploymentSpec{ServiceName: "svc"},
		Metrics: &DeploymentMetrics{},
	}

	err := engine.runHealthCheck(context.Background(), deployment)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deployment.Metrics.HealthChecksTotal != 1 {
		t.Fatalf("expected 1 total health check, got %d", deployment.Metrics.HealthChecksTotal)
	}
}

func TestRunHealthCheckWithInitialDelay(t *testing.T) {
	engine := newTestEngine(nil)
	deployment := &Deployment{
		ID: "d-hc-delay",
		Spec: &DeploymentSpec{
			ServiceName: "svc",
			HealthCheck: &HealthCheckSpec{
				Type:         "http",
				Target:       "/health",
				InitialDelay: 1 * time.Millisecond,
			},
		},
		Metrics: &DeploymentMetrics{},
	}

	err := engine.runHealthCheck(context.Background(), deployment)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deployment.Metrics.HealthChecksTotal != 1 {
		t.Fatalf("expected 1 health check, got %d", deployment.Metrics.HealthChecksTotal)
	}
}

func TestRunHealthCheckContextCancelled(t *testing.T) {
	engine := newTestEngine(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	deployment := &Deployment{
		ID: "d-hc-cancel",
		Spec: &DeploymentSpec{
			ServiceName: "svc",
			HealthCheck: &HealthCheckSpec{
				Type:         "http",
				Target:       "/health",
				InitialDelay: 1 * time.Hour, // long delay
			},
		},
		Metrics: &DeploymentMetrics{},
	}

	err := engine.runHealthCheck(ctx, deployment)
	if err == nil {
		t.Fatal("expected context error")
	}
}

// --- runHook ---

func TestRunHookHTTP(t *testing.T) {
	engine := newTestEngine(nil)
	err := engine.runHook(context.Background(), HookAction{
		Name: "test-http",
		Type: "http",
		URL:  "http://localhost/hook",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunHookWotanWithPublisher(t *testing.T) {
	engine, pub := newTestEngineWithWotan(nil)
	err := engine.runHook(context.Background(), HookAction{
		Name:    "test-wotan",
		Type:    "wotan",
		Topic:   "deploy.hooks",
		Payload: map[string]string{"key": "val"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pub.count() != 1 {
		t.Fatalf("expected 1 published message, got %d", pub.count())
	}
}

func TestRunHookWotanPublishError(t *testing.T) {
	engine, pub := newTestEngineWithWotan(nil)
	pub.err = errors.New("publish failed")
	err := engine.runHook(context.Background(), HookAction{
		Name:  "test-wotan-err",
		Type:  "wotan",
		Topic: "deploy.hooks",
	})
	if err == nil {
		t.Fatal("expected publish error")
	}
}

func TestRunHookCustomTimeout(t *testing.T) {
	engine := newTestEngine(nil)
	err := engine.runHook(context.Background(), HookAction{
		Name:    "test-timeout",
		Type:    "exec",
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- stageValidate ---

func TestStageValidateWithPreDeployHooks(t *testing.T) {
	engine := newTestEngine(nil)
	deployment := &Deployment{
		ID: "d-validate-hooks",
		Spec: &DeploymentSpec{
			ServiceName: "svc",
			Version:     "v1",
			Hooks: &HookSpec{
				PreDeploy: []HookAction{
					{Name: "pre1", Type: "exec"},
					{Name: "pre2", Type: "http"},
				},
			},
		},
	}

	err := engine.stageValidate(context.Background(), deployment)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStageValidatePreDeployHookFails(t *testing.T) {
	engine := newTestEngine(nil)
	deployment := &Deployment{
		ID: "d-validate-hook-fail",
		Spec: &DeploymentSpec{
			ServiceName: "svc",
			Version:     "v1",
			Hooks: &HookSpec{
				PreDeploy: []HookAction{
					{Name: "bad-hook", Type: "unknown_type"},
				},
			},
		},
	}

	err := engine.stageValidate(context.Background(), deployment)
	if err == nil {
		t.Fatal("expected error from failing hook")
	}
	if !strings.Contains(err.Error(), "pre-deploy hook") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStageValidateWithContainerSpec(t *testing.T) {
	engine := newTestEngine(nil)
	deployment := &Deployment{
		ID: "d-validate-cspec",
		Spec: &DeploymentSpec{
			ServiceName: "svc",
			Version:     "v1",
			ContainerSpec: &container.ContainerSpec{
				Name:  "test-container",
				Image: "test:latest",
			},
		},
	}

	err := engine.stageValidate(context.Background(), deployment)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStageValidateInvalidContainerSpec(t *testing.T) {
	engine := newTestEngine(nil)
	deployment := &Deployment{
		ID: "d-validate-bad-cspec",
		Spec: &DeploymentSpec{
			ServiceName: "svc",
			Version:     "v1",
			ContainerSpec: &container.ContainerSpec{
				Name: "", // invalid: empty name
			},
		},
	}

	err := engine.stageValidate(context.Background(), deployment)
	if err == nil {
		t.Fatal("expected error for invalid container spec")
	}
	if !strings.Contains(err.Error(), "invalid container spec") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- stageDeploy ---

func TestStageDeployStrategies(t *testing.T) {
	strategies := []struct {
		name     string
		strategy StrategyType
	}{
		{"rolling", StrategyRolling},
		{"blue_green", StrategyBlueGreen},
		{"recreate", StrategyRecreate},
	}

	for _, tt := range strategies {
		t.Run(tt.name, func(t *testing.T) {
			engine := newTestEngine(nil)
			deployment := &Deployment{
				ID:      fmt.Sprintf("d-stage-%s", tt.name),
				Spec:    &DeploymentSpec{ServiceName: "svc", Version: "v1", Replicas: 1, Strategy: tt.strategy},
				Metrics: &DeploymentMetrics{},
			}

			err := engine.stageDeploy(context.Background(), deployment)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestStageDeployCanary(t *testing.T) {
	engine := newTestEngine(nil)
	deployment := &Deployment{
		ID: "d-stage-canary",
		Spec: &DeploymentSpec{
			ServiceName: "svc",
			Version:     "v1",
			Replicas:    1,
			Strategy:    StrategyCanary,
			StrategyConfig: &StrategyConfig{
				CanarySteps: []CanaryStep{
					{Weight: 100, Duration: 0},
				},
			},
		},
		Metrics: &DeploymentMetrics{},
	}

	err := engine.stageDeploy(context.Background(), deployment)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStageDeployUnsupportedStrategy(t *testing.T) {
	engine := newTestEngine(nil)
	deployment := &Deployment{
		ID:      "d-stage-unsupported",
		Spec:    &DeploymentSpec{ServiceName: "svc", Version: "v1", Replicas: 1, Strategy: "magical"},
		Metrics: &DeploymentMetrics{},
	}

	err := engine.stageDeploy(context.Background(), deployment)
	if !errors.Is(err, ErrStrategyNotSupported) {
		t.Fatalf("expected ErrStrategyNotSupported, got %v", err)
	}
}

// --- stageVerify ---

func TestStageVerifyNoHealthCheck(t *testing.T) {
	engine := newTestEngine(nil)
	deployment := &Deployment{
		ID:      "d-verify-nohc",
		Spec:    &DeploymentSpec{ServiceName: "svc", Version: "v1"},
		Metrics: &DeploymentMetrics{},
	}

	err := engine.stageVerify(context.Background(), deployment)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStageVerifyWithPostDeployHooks(t *testing.T) {
	engine := newTestEngine(nil)
	deployment := &Deployment{
		ID: "d-verify-hooks",
		Spec: &DeploymentSpec{
			ServiceName: "svc",
			Version:     "v1",
			Hooks: &HookSpec{
				PostDeploy: []HookAction{
					{Name: "post1", Type: "exec"},
				},
			},
		},
		Metrics: &DeploymentMetrics{},
	}

	err := engine.stageVerify(context.Background(), deployment)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStageVerifyPostDeployHookFails(t *testing.T) {
	engine := newTestEngine(nil)
	deployment := &Deployment{
		ID: "d-verify-hook-fail",
		Spec: &DeploymentSpec{
			ServiceName: "svc",
			Version:     "v1",
			Hooks: &HookSpec{
				PostDeploy: []HookAction{
					{Name: "bad-hook", Type: "unknown_type"},
				},
			},
		},
		Metrics: &DeploymentMetrics{},
	}

	err := engine.stageVerify(context.Background(), deployment)
	if err == nil {
		t.Fatal("expected error from failing post-deploy hook")
	}
	if !strings.Contains(err.Error(), "post-deploy hook") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStageVerifyHealthCheckPasses(t *testing.T) {
	engine := newTestEngine(nil)
	deployment := &Deployment{
		ID: "d-verify-hc-pass",
		Spec: &DeploymentSpec{
			ServiceName: "svc",
			Version:     "v1",
			HealthCheck: &HealthCheckSpec{
				Type:   "http",
				Target: "/health",
			},
		},
		Metrics: &DeploymentMetrics{},
	}

	err := engine.stageVerify(context.Background(), deployment)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deployment.Metrics.HealthChecksPassed != 1 {
		t.Fatalf("expected 1 passed health check, got %d", deployment.Metrics.HealthChecksPassed)
	}
}

// --- stagePromote ---

func TestStagePromoteCanary(t *testing.T) {
	engine := newTestEngine(nil)
	deployment := &Deployment{
		ID:      "d-promote-canary",
		Spec:    &DeploymentSpec{ServiceName: "svc", Version: "v1", Strategy: StrategyCanary},
		Metrics: &DeploymentMetrics{},
	}

	err := engine.stagePromote(context.Background(), deployment)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deployment.Metrics.TrafficShifts != 1 {
		t.Fatalf("expected 1 traffic shift, got %d", deployment.Metrics.TrafficShifts)
	}
}

func TestStagePromoteNonCanary(t *testing.T) {
	engine := newTestEngine(nil)
	deployment := &Deployment{
		ID:      "d-promote-rolling",
		Spec:    &DeploymentSpec{ServiceName: "svc", Version: "v1", Strategy: StrategyRolling},
		Metrics: &DeploymentMetrics{},
	}

	err := engine.stagePromote(context.Background(), deployment)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deployment.Metrics.TrafficShifts != 0 {
		t.Fatalf("expected 0 traffic shifts for non-canary, got %d", deployment.Metrics.TrafficShifts)
	}
}

// --- Rollback with history ---

func TestRollbackSuccessful(t *testing.T) {
	engine := newTestEngine(nil)

	prevDeploy := &Deployment{
		ID:    "d-prev",
		State: StateCompleted,
		Spec: &DeploymentSpec{
			ServiceName: "svc",
			Version:     "v1",
			ArtifactRef: "art:v1",
			Strategy:    StrategyRolling,
			Replicas:    1,
		},
	}
	currentDeploy := &Deployment{
		ID:    "d-current",
		State: StateDeploying,
		Spec: &DeploymentSpec{
			ServiceName: "svc",
			Version:     "v2",
			ArtifactRef: "art:v2",
			Strategy:    StrategyRolling,
			Replicas:    1,
		},
	}

	engine.deployMu.Lock()
	engine.deployments[currentDeploy.ID] = currentDeploy
	engine.deployMu.Unlock()

	engine.histMu.Lock()
	engine.history["svc"] = []*Deployment{prevDeploy, currentDeploy}
	engine.histMu.Unlock()

	err := engine.Rollback(context.Background(), currentDeploy.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	engine.deployMu.RLock()
	state := currentDeploy.State
	rollbackID := currentDeploy.RollbackID
	engine.deployMu.RUnlock()

	if state != StateRolledBack {
		t.Fatalf("expected rolled_back, got %s", state)
	}
	if rollbackID == "" {
		t.Fatal("expected rollback deployment ID to be set")
	}
}

func TestRollbackHistoryLessThanTwo(t *testing.T) {
	engine := newTestEngine(nil)
	d := &Deployment{
		ID:   "d1",
		Spec: &DeploymentSpec{ServiceName: "svc", Version: "v2"},
	}
	engine.deployMu.Lock()
	engine.deployments["d1"] = d
	engine.deployMu.Unlock()

	engine.histMu.Lock()
	engine.history["svc"] = []*Deployment{d}
	engine.histMu.Unlock()

	err := engine.Rollback(context.Background(), "d1")
	if !errors.Is(err, ErrNoRollbackAvailable) {
		t.Fatalf("expected ErrNoRollbackAvailable, got %v", err)
	}
}

func TestRollbackNoPreviousCompleted(t *testing.T) {
	engine := newTestEngine(nil)
	d1 := &Deployment{
		ID:    "d1",
		State: StateFailed,
		Spec:  &DeploymentSpec{ServiceName: "svc", Version: "v1"},
	}
	d2 := &Deployment{
		ID:    "d2",
		State: StateFailed,
		Spec:  &DeploymentSpec{ServiceName: "svc", Version: "v2"},
	}
	engine.deployMu.Lock()
	engine.deployments["d2"] = d2
	engine.deployMu.Unlock()

	engine.histMu.Lock()
	engine.history["svc"] = []*Deployment{d1, d2}
	engine.histMu.Unlock()

	err := engine.Rollback(context.Background(), "d2")
	if !errors.Is(err, ErrNoRollbackAvailable) {
		t.Fatalf("expected ErrNoRollbackAvailable, got %v", err)
	}
}

// --- Status with runtime ---

func TestStatusWithRuntime(t *testing.T) {
	engine, rt := newTestEngineWithRuntime(nil)

	// Create a container in the runtime with the right label
	spec := &container.ContainerSpec{
		Name:  "test-container",
		Image: "test:latest",
		Labels: map[string]string{
			"deployment": "d-runtime",
		},
	}
	c, err := rt.Create(context.Background(), spec)
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}
	// Start it so it's running
	if err := rt.Start(context.Background(), c.ID); err != nil {
		t.Fatalf("failed to start container: %v", err)
	}

	deployment := &Deployment{
		ID:    "d-runtime",
		State: StateDeploying,
		Spec:  &DeploymentSpec{ServiceName: "svc", Replicas: 1},
	}
	engine.deployMu.Lock()
	engine.deployments["d-runtime"] = deployment
	engine.deployMu.Unlock()

	status, err := engine.Status(context.Background(), "d-runtime")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.CurrentReplicas != 1 {
		t.Fatalf("expected 1 current replica, got %d", status.CurrentReplicas)
	}
	if status.ReadyReplicas != 1 {
		t.Fatalf("expected 1 ready replica, got %d", status.ReadyReplicas)
	}
}

// --- Engine.Cancel for failed state ---

func TestCancelFailedDeployment(t *testing.T) {
	engine := newTestEngine(nil)
	engine.deployMu.Lock()
	engine.deployments["d1"] = &Deployment{
		ID:    "d1",
		State: StateFailed,
		Spec:  &DeploymentSpec{ServiceName: "svc"},
	}
	engine.deployMu.Unlock()

	err := engine.Cancel(context.Background(), "d1")
	if err == nil {
		t.Fatal("expected error cancelling failed deployment")
	}
	if !strings.Contains(err.Error(), "already completed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- deployRolling with strategy config ---

func TestDeployRollingWithMaxSurge(t *testing.T) {
	engine := newTestEngine(nil)
	deployment := &Deployment{
		ID: "d-rolling-surge",
		Spec: &DeploymentSpec{
			ServiceName: "svc",
			Version:     "v1",
			Replicas:    2,
			Strategy:    StrategyRolling,
			StrategyConfig: &StrategyConfig{
				MaxSurge: 3,
			},
		},
		Metrics: &DeploymentMetrics{},
	}

	err := engine.deployRolling(context.Background(), deployment)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deployment.Metrics.InstancesUpdated != 2 {
		t.Fatalf("expected 2 instances updated, got %d", deployment.Metrics.InstancesUpdated)
	}
}

func TestDeployRollingCancelled(t *testing.T) {
	engine := newTestEngine(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	deployment := &Deployment{
		ID:      "d-rolling-cancel",
		Spec:    &DeploymentSpec{ServiceName: "svc", Version: "v1", Replicas: 3, Strategy: StrategyRolling},
		Metrics: &DeploymentMetrics{},
	}

	err := engine.deployRolling(ctx, deployment)
	if err == nil {
		t.Fatal("expected context error")
	}
}

// --- runDeployment integration test ---

func TestRunDeploymentFullPipeline(t *testing.T) {
	engine, pub := newTestEngineWithWotan(nil)
	spec := &DeploymentSpec{
		ServiceName: "svc-pipeline",
		Version:     "v1",
		Replicas:    1,
		Strategy:    StrategyRecreate,
		Timeout:     30 * time.Second,
	}
	deployment := &Deployment{
		ID:        "d-pipeline",
		Spec:      spec,
		State:     StatePending,
		StartedAt: time.Now(),
		Metrics:   &DeploymentMetrics{},
	}
	engine.deployMu.Lock()
	engine.deployments[deployment.ID] = deployment
	engine.deployMu.Unlock()
	engine.progMu.Lock()
	engine.inProgress[spec.ServiceName] = deployment.ID
	engine.progMu.Unlock()

	engine.runDeployment(context.Background(), deployment)

	engine.deployMu.RLock()
	state := deployment.State
	msg := deployment.Message
	dur := deployment.Duration
	stages := deployment.Stages
	engine.deployMu.RUnlock()

	if state != StateCompleted {
		t.Fatalf("expected completed, got %s (message: %s)", state, msg)
	}
	if dur == 0 {
		t.Fatal("expected non-zero duration")
	}
	if len(stages) != 4 {
		t.Fatalf("expected 4 stages, got %d", len(stages))
	}
	for _, s := range stages {
		if s.State != StateCompleted {
			t.Fatalf("stage %s not completed: %s", s.Name, s.State)
		}
	}

	// Verify completion event was published (start event is emitted by Deploy, not runDeployment)
	if pub.count() < 1 {
		t.Fatalf("expected at least 1 event (complete), got %d", pub.count())
	}

	// Verify added to history
	engine.histMu.RLock()
	history := engine.history[spec.ServiceName]
	engine.histMu.RUnlock()
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}

	// Verify in-progress cleared
	engine.progMu.RLock()
	_, exists := engine.inProgress[spec.ServiceName]
	engine.progMu.RUnlock()
	if exists {
		t.Fatal("expected inProgress to be cleared")
	}
}

func TestRunDeploymentUnsupportedStrategy(t *testing.T) {
	engine := newTestEngine(nil)
	spec := &DeploymentSpec{
		ServiceName: "svc-bad-strategy",
		Version:     "v1",
		Replicas:    1,
		Strategy:    "imaginary",
		Timeout:     5 * time.Second,
	}
	deployment := &Deployment{
		ID:        "d-bad-strategy",
		Spec:      spec,
		State:     StatePending,
		StartedAt: time.Now(),
		Metrics:   &DeploymentMetrics{},
	}
	engine.deployMu.Lock()
	engine.deployments[deployment.ID] = deployment
	engine.deployMu.Unlock()
	engine.progMu.Lock()
	engine.inProgress[spec.ServiceName] = deployment.ID
	engine.progMu.Unlock()

	engine.runDeployment(context.Background(), deployment)

	engine.deployMu.RLock()
	state := deployment.State
	engine.deployMu.RUnlock()

	if state != StateFailed {
		t.Fatalf("expected failed, got %s", state)
	}
}

// --- updateDeploymentState ---

func TestUpdateDeploymentState(t *testing.T) {
	engine := newTestEngine(nil)
	d := &Deployment{State: StatePending}
	engine.updateDeploymentState(d, StateDeploying)
	if d.State != StateDeploying {
		t.Fatalf("expected deploying, got %s", d.State)
	}
}

// --- Deploy with Wotan events ---

func TestDeployEmitsStartEvent(t *testing.T) {
	engine, pub := newTestEngineWithWotan(nil)
	spec := &DeploymentSpec{
		ServiceName: "svc-events",
		Version:     "v1",
		Replicas:    1,
	}

	deployment, err := engine.Deploy(context.Background(), spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deployment == nil {
		t.Fatal("deployment is nil")
	}

	// The start event should have been published synchronously before the goroutine
	// Give the background goroutine a moment to not interfere
	time.Sleep(50 * time.Millisecond)

	if pub.count() < 1 {
		t.Fatal("expected at least 1 event (deployment started)")
	}
}

// --- handleDeploymentFailure with Wotan ---

func TestHandleDeploymentFailureEmitsEvent(t *testing.T) {
	engine, pub := newTestEngineWithWotan(nil)
	deployment := &Deployment{
		ID:        "d-fail-event",
		State:     StateDeploying,
		StartedAt: time.Now(),
		Spec:      &DeploymentSpec{ServiceName: "svc", Version: "v1"},
		Metrics:   &DeploymentMetrics{},
	}
	engine.deployMu.Lock()
	engine.deployments[deployment.ID] = deployment
	engine.deployMu.Unlock()

	engine.handleDeploymentFailure(context.Background(), deployment, errors.New("boom"))

	if pub.count() != 1 {
		t.Fatalf("expected 1 alert event, got %d", pub.count())
	}
}
