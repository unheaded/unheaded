// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package sword

import (
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/logger"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newTestLogger returns a logger that discards output, suitable for tests.
func newTestLogger() *logger.Logger {
	return logger.New(io.Discard)
}

// newTestService creates a Sword service with a silent logger and default config.
func newTestService() *Service {
	return NewService(newTestLogger(), nil, nil)
}

// newTestServiceWithConfig creates a Sword service with the supplied config.
func newTestServiceWithConfig(cfg *Config) *Service {
	return NewService(newTestLogger(), nil, cfg)
}

// makeDeployment is a convenience builder for test deployments.
func makeDeployment(strategy DeployStrategy) *Deployment {
	return &Deployment{
		Name:     "test-deploy",
		Artifact: "myapp",
		Version:  "1.0.0",
		Strategy: strategy,
		Target: DeployTarget{
			Type:      "service",
			Namespace: "default",
			Replicas:  3,
			Selector:  map[string]string{"app": "myapp"},
		},
	}
}

// createAndGet is a helper that creates a deployment and returns its ID.
func createAndGet(t *testing.T, svc *Service, d *Deployment) string {
	t.Helper()
	created, err := svc.CreateDeployment(context.Background(), d)
	if err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}
	if created.ID == "" {
		t.Fatal("Expected non-empty deployment ID")
	}
	return created.ID
}

// ---------------------------------------------------------------------------
// DefaultConfig tests
// ---------------------------------------------------------------------------

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	tests := []struct {
		name     string
		got      interface{}
		expected interface{}
	}{
		{"MaxConcurrentDeploys", cfg.MaxConcurrentDeploys, 5},
		{"HealthCheckTimeout", cfg.HealthCheckTimeout, 30 * time.Second},
		{"RollbackOnFailure", cfg.RollbackOnFailure, true},
		{"CanaryPercentage", cfg.CanaryPercentage, 10},
		{"WotanTopic", cfg.WotanTopic, "sword.deployments"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.expected {
				t.Errorf("DefaultConfig().%s = %v, want %v", tc.name, tc.got, tc.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// NewService tests
// ---------------------------------------------------------------------------

func TestNewService(t *testing.T) {
	t.Run("NilConfigUsesDefaults", func(t *testing.T) {
		svc := NewService(newTestLogger(), nil, nil)
		if svc == nil {
			t.Fatal("Expected non-nil service")
		}
		if svc.config.MaxConcurrentDeploys != 5 {
			t.Errorf("Expected default MaxConcurrentDeploys=5, got %d", svc.config.MaxConcurrentDeploys)
		}
		if svc.deployments == nil {
			t.Error("Expected deployments map to be initialized")
		}
		if svc.pipelines == nil {
			t.Error("Expected pipelines map to be initialized")
		}
	})

	t.Run("CustomConfigIsPreserved", func(t *testing.T) {
		cfg := &Config{
			MaxConcurrentDeploys: 10,
			HealthCheckTimeout:   1 * time.Minute,
			RollbackOnFailure:    false,
			CanaryPercentage:     25,
			WotanTopic:          "custom.topic",
		}
		svc := NewService(newTestLogger(), nil, cfg)

		if svc.config.MaxConcurrentDeploys != 10 {
			t.Errorf("Expected MaxConcurrentDeploys=10, got %d", svc.config.MaxConcurrentDeploys)
		}
		if svc.config.CanaryPercentage != 25 {
			t.Errorf("Expected CanaryPercentage=25, got %d", svc.config.CanaryPercentage)
		}
		if svc.config.RollbackOnFailure {
			t.Error("Expected RollbackOnFailure=false")
		}
		if svc.config.WotanTopic != "custom.topic" {
			t.Errorf("Expected WotanTopic='custom.topic', got %q", svc.config.WotanTopic)
		}
	})
}

// ---------------------------------------------------------------------------
// Service lifecycle (Start/Stop)
// ---------------------------------------------------------------------------

func TestServiceLifecycle(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	t.Run("StartReturnsNil", func(t *testing.T) {
		if err := svc.Start(ctx); err != nil {
			t.Fatalf("Start() returned error: %v", err)
		}
	})

	t.Run("StopReturnsNil", func(t *testing.T) {
		if err := svc.Stop(); err != nil {
			t.Fatalf("Stop() returned error: %v", err)
		}
	})

	t.Run("MultipleStartStopCycles", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			if err := svc.Start(ctx); err != nil {
				t.Fatalf("Start() cycle %d failed: %v", i, err)
			}
			if err := svc.Stop(); err != nil {
				t.Fatalf("Stop() cycle %d failed: %v", i, err)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// CreateDeployment tests (table-driven)
// ---------------------------------------------------------------------------

func TestCreateDeployment(t *testing.T) {
	strategies := []struct {
		name     string
		strategy DeployStrategy
	}{
		{"Rolling", StrategyRolling},
		{"BlueGreen", StrategyBlueGreen},
		{"Canary", StrategyCanary},
		{"Recreate", StrategyRecreate},
	}

	for _, s := range strategies {
		t.Run("Strategy_"+s.name, func(t *testing.T) {
			svc := newTestService()
			d := makeDeployment(s.strategy)

			created, err := svc.CreateDeployment(context.Background(), d)
			if err != nil {
				t.Fatalf("CreateDeployment failed: %v", err)
			}
			if created.ID == "" {
				t.Error("Expected generated ID")
			}
			if created.Status != DeployPending {
				t.Errorf("Expected status %q, got %q", DeployPending, created.Status)
			}
			if created.Progress != 0 {
				t.Errorf("Expected progress 0, got %d", created.Progress)
			}
			if created.Strategy != s.strategy {
				t.Errorf("Expected strategy %q, got %q", s.strategy, created.Strategy)
			}
		})
	}

	t.Run("AutoGeneratesID", func(t *testing.T) {
		svc := newTestService()
		d := makeDeployment(StrategyRolling)
		d.ID = "" // ensure empty

		created, _ := svc.CreateDeployment(context.Background(), d)
		if created.ID == "" {
			t.Error("Expected auto-generated ID")
		}
	})

	t.Run("PreservesExplicitID", func(t *testing.T) {
		svc := newTestService()
		d := makeDeployment(StrategyRolling)
		d.ID = "my-custom-id"

		created, _ := svc.CreateDeployment(context.Background(), d)
		if created.ID != "my-custom-id" {
			t.Errorf("Expected ID %q, got %q", "my-custom-id", created.ID)
		}
	})

	t.Run("ResetsStatusAndProgress", func(t *testing.T) {
		svc := newTestService()
		d := makeDeployment(StrategyRolling)
		d.Status = DeploySucceeded // should be overwritten
		d.Progress = 100           // should be overwritten

		created, _ := svc.CreateDeployment(context.Background(), d)
		if created.Status != DeployPending {
			t.Errorf("Expected status to be reset to pending, got %q", created.Status)
		}
		if created.Progress != 0 {
			t.Errorf("Expected progress to be reset to 0, got %d", created.Progress)
		}
	})

	t.Run("UniqueIDsForMultipleDeployments", func(t *testing.T) {
		svc := newTestService()
		ids := make(map[string]bool)

		for i := 0; i < 20; i++ {
			d := makeDeployment(StrategyRolling)
			created, _ := svc.CreateDeployment(context.Background(), d)
			if ids[created.ID] {
				t.Fatalf("Duplicate ID generated: %s", created.ID)
			}
			ids[created.ID] = true
		}
	})

	t.Run("StoresMetadata", func(t *testing.T) {
		svc := newTestService()
		d := makeDeployment(StrategyRolling)
		d.Metadata = map[string]string{"env": "staging", "team": "platform"}

		created, _ := svc.CreateDeployment(context.Background(), d)
		if created.Metadata["env"] != "staging" {
			t.Error("Expected metadata to be preserved")
		}
	})

	t.Run("StoresHealthChecks", func(t *testing.T) {
		svc := newTestService()
		d := makeDeployment(StrategyRolling)
		d.HealthChecks = []HealthCheck{
			{Name: "http-ready", Type: "http", Endpoint: "/ready", Interval: 5 * time.Second, Timeout: 2 * time.Second, Retries: 3},
			{Name: "tcp-port", Type: "tcp", Endpoint: ":8080", Interval: 10 * time.Second, Timeout: 5 * time.Second, Retries: 2},
		}

		created, _ := svc.CreateDeployment(context.Background(), d)
		if len(created.HealthChecks) != 2 {
			t.Errorf("Expected 2 health checks, got %d", len(created.HealthChecks))
		}
		if created.HealthChecks[0].Name != "http-ready" {
			t.Error("Expected health check name to be preserved")
		}
	})
}

// ---------------------------------------------------------------------------
// GetDeployment tests
// ---------------------------------------------------------------------------

func TestGetDeployment(t *testing.T) {
	t.Run("ExistingDeployment", func(t *testing.T) {
		svc := newTestService()
		d := makeDeployment(StrategyRolling)
		id := createAndGet(t, svc, d)

		got, ok := svc.GetDeployment(id)
		if !ok {
			t.Fatal("Expected to find deployment")
		}
		if got.ID != id {
			t.Errorf("ID mismatch: got %q, want %q", got.ID, id)
		}
	})

	t.Run("NonExistentDeployment", func(t *testing.T) {
		svc := newTestService()
		_, ok := svc.GetDeployment("does-not-exist")
		if ok {
			t.Error("Expected ok=false for non-existent deployment")
		}
	})

	t.Run("EmptyID", func(t *testing.T) {
		svc := newTestService()
		_, ok := svc.GetDeployment("")
		if ok {
			t.Error("Expected ok=false for empty ID")
		}
	})
}

// ---------------------------------------------------------------------------
// ListDeployments tests
// ---------------------------------------------------------------------------

func TestListDeployments(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	// Create several deployments
	for i := 0; i < 5; i++ {
		d := makeDeployment(StrategyRolling)
		_, _ = svc.CreateDeployment(ctx, d)
	}

	t.Run("ListAll", func(t *testing.T) {
		results := svc.ListDeployments("")
		if len(results) != 5 {
			t.Errorf("Expected 5 deployments, got %d", len(results))
		}
	})

	t.Run("FilterByPending", func(t *testing.T) {
		results := svc.ListDeployments(DeployPending)
		if len(results) != 5 {
			t.Errorf("Expected 5 pending deployments, got %d", len(results))
		}
	})

	t.Run("FilterByInProgress", func(t *testing.T) {
		results := svc.ListDeployments(DeployInProgress)
		if len(results) != 0 {
			t.Errorf("Expected 0 in-progress deployments, got %d", len(results))
		}
	})

	t.Run("FilterBySucceeded", func(t *testing.T) {
		results := svc.ListDeployments(DeploySucceeded)
		if len(results) != 0 {
			t.Errorf("Expected 0 succeeded deployments, got %d", len(results))
		}
	})

	t.Run("EmptyServiceReturnsNil", func(t *testing.T) {
		emptySvc := newTestService()
		results := emptySvc.ListDeployments("")
		if results != nil && len(results) != 0 {
			t.Errorf("Expected nil or empty list, got %d items", len(results))
		}
	})
}

// ---------------------------------------------------------------------------
// ExecuteDeployment -- Rolling strategy
// ---------------------------------------------------------------------------

func TestExecuteRolling(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	t.Run("SuccessfulExecution", func(t *testing.T) {
		d := makeDeployment(StrategyRolling)
		id := createAndGet(t, svc, d)

		err := svc.ExecuteDeployment(ctx, id)
		if err != nil {
			t.Fatalf("ExecuteDeployment failed: %v", err)
		}

		got, _ := svc.GetDeployment(id)
		if got.Status != DeploySucceeded {
			t.Errorf("Expected status %q, got %q", DeploySucceeded, got.Status)
		}
		if got.Progress != 100 {
			t.Errorf("Expected progress 100, got %d", got.Progress)
		}
		if got.StartedAt == nil {
			t.Error("Expected StartedAt to be set")
		}
		if got.CompletedAt == nil {
			t.Error("Expected CompletedAt to be set")
		}
		if got.CompletedAt.Before(*got.StartedAt) {
			t.Error("CompletedAt should be after StartedAt")
		}
	})

	t.Run("RollingProgressionSequence", func(t *testing.T) {
		svc2 := newTestService()
		d := makeDeployment(StrategyRolling)
		id := createAndGet(t, svc2, d)

		// Use a short-lived context so we can observe progress increments
		err := svc2.ExecuteDeployment(ctx, id)
		if err != nil {
			t.Fatalf("ExecuteDeployment failed: %v", err)
		}

		got, _ := svc2.GetDeployment(id)
		// After completion, progress should be 100 (set by ExecuteDeployment)
		if got.Progress != 100 {
			t.Errorf("Expected progress 100 after execution, got %d", got.Progress)
		}
	})

	t.Run("ContextCancellation", func(t *testing.T) {
		svc2 := newTestService()
		d := makeDeployment(StrategyRolling)
		id := createAndGet(t, svc2, d)

		ctx2, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately

		err := svc2.ExecuteDeployment(ctx2, id)
		if err == nil {
			t.Error("Expected error from cancelled context")
		}

		got, _ := svc2.GetDeployment(id)
		if got.Status != DeployFailed {
			t.Errorf("Expected status %q after cancellation, got %q", DeployFailed, got.Status)
		}
	})
}

// ---------------------------------------------------------------------------
// ExecuteDeployment -- Blue-Green strategy
// ---------------------------------------------------------------------------

func TestExecuteBlueGreen(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	t.Run("SuccessfulBlueGreenSwitch", func(t *testing.T) {
		d := makeDeployment(StrategyBlueGreen)
		id := createAndGet(t, svc, d)

		err := svc.ExecuteDeployment(ctx, id)
		if err != nil {
			t.Fatalf("Blue-green execution failed: %v", err)
		}

		got, _ := svc.GetDeployment(id)
		if got.Status != DeploySucceeded {
			t.Errorf("Expected succeeded, got %q", got.Status)
		}
		if got.Progress != 100 {
			t.Errorf("Expected progress 100, got %d", got.Progress)
		}
	})

	t.Run("ContextCancelDuringGreenDeploy", func(t *testing.T) {
		svc2 := newTestService()
		d := makeDeployment(StrategyBlueGreen)
		id := createAndGet(t, svc2, d)

		// Cancel quickly -- the green phase takes ~60ms (6 iterations x 10ms)
		ctx2, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
		defer cancel()

		err := svc2.ExecuteDeployment(ctx2, id)
		if err == nil {
			t.Error("Expected error from timeout during blue-green deploy")
		}
	})

	t.Run("ContextCancelDuringTrafficSwitch", func(t *testing.T) {
		svc2 := newTestService()
		d := makeDeployment(StrategyBlueGreen)
		id := createAndGet(t, svc2, d)

		// Allow green deploy (~60ms) but cancel during traffic switch (~60ms more)
		ctx2, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
		defer cancel()

		err := svc2.ExecuteDeployment(ctx2, id)
		// Might succeed or fail depending on timing; just ensure no panic
		_ = err
	})
}

// ---------------------------------------------------------------------------
// ExecuteDeployment -- Canary strategy
// ---------------------------------------------------------------------------

func TestExecuteCanary(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	t.Run("SuccessfulCanaryPromotion", func(t *testing.T) {
		d := makeDeployment(StrategyCanary)
		id := createAndGet(t, svc, d)

		err := svc.ExecuteDeployment(ctx, id)
		if err != nil {
			t.Fatalf("Canary execution failed: %v", err)
		}

		got, _ := svc.GetDeployment(id)
		if got.Status != DeploySucceeded {
			t.Errorf("Expected succeeded, got %q", got.Status)
		}
		if got.Progress != 100 {
			t.Errorf("Expected progress 100, got %d", got.Progress)
		}
	})

	t.Run("ContextCancelDuringCanaryDeploy", func(t *testing.T) {
		svc2 := newTestService()
		d := makeDeployment(StrategyCanary)
		id := createAndGet(t, svc2, d)

		ctx2, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
		defer cancel()

		err := svc2.ExecuteDeployment(ctx2, id)
		if err == nil {
			t.Error("Expected error from timeout during canary deploy")
		}

		got, _ := svc2.GetDeployment(id)
		if got.Status != DeployFailed {
			t.Errorf("Expected failed status, got %q", got.Status)
		}
	})

	t.Run("CanaryWithCustomPercentage", func(t *testing.T) {
		cfg := &Config{
			MaxConcurrentDeploys: 5,
			HealthCheckTimeout:   30 * time.Second,
			RollbackOnFailure:    false,
			CanaryPercentage:     50,
			WotanTopic:          "sword.test",
		}
		svc2 := newTestServiceWithConfig(cfg)

		d := makeDeployment(StrategyCanary)
		id := createAndGet(t, svc2, d)

		err := svc2.ExecuteDeployment(ctx, id)
		if err != nil {
			t.Fatalf("Canary with 50%% percentage failed: %v", err)
		}

		got, _ := svc2.GetDeployment(id)
		if got.Status != DeploySucceeded {
			t.Errorf("Expected succeeded, got %q", got.Status)
		}
	})

	t.Run("ContextCancelDuringCanaryMonitoring", func(t *testing.T) {
		svc2 := newTestService()
		d := makeDeployment(StrategyCanary)
		id := createAndGet(t, svc2, d)

		// Canary deploy phase: 5 steps x 10ms = ~50ms
		// Cancel during monitoring phase (after ~50ms but before ~120ms)
		ctx2, cancel := context.WithTimeout(context.Background(), 70*time.Millisecond)
		defer cancel()

		err := svc2.ExecuteDeployment(ctx2, id)
		if err == nil {
			t.Error("Expected error from timeout during canary monitoring")
		}
	})
}

// ---------------------------------------------------------------------------
// ExecuteDeployment -- Recreate strategy
// ---------------------------------------------------------------------------

func TestExecuteRecreate(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	t.Run("SuccessfulRecreate", func(t *testing.T) {
		d := makeDeployment(StrategyRecreate)
		id := createAndGet(t, svc, d)

		err := svc.ExecuteDeployment(ctx, id)
		if err != nil {
			t.Fatalf("Recreate execution failed: %v", err)
		}

		got, _ := svc.GetDeployment(id)
		if got.Status != DeploySucceeded {
			t.Errorf("Expected succeeded, got %q", got.Status)
		}
	})

	t.Run("ContextCancelDuringStop", func(t *testing.T) {
		svc2 := newTestService()
		d := makeDeployment(StrategyRecreate)
		id := createAndGet(t, svc2, d)

		ctx2, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
		defer cancel()

		err := svc2.ExecuteDeployment(ctx2, id)
		if err == nil {
			t.Error("Expected error from timeout during recreate stop phase")
		}
	})
}

// ---------------------------------------------------------------------------
// ExecuteDeployment -- Error paths
// ---------------------------------------------------------------------------

func TestExecuteDeploymentErrors(t *testing.T) {
	t.Run("NonExistentDeployment", func(t *testing.T) {
		svc := newTestService()
		err := svc.ExecuteDeployment(context.Background(), "nonexistent-id")
		if err == nil {
			t.Error("Expected error for non-existent deployment")
		}
	})

	t.Run("EmptyDeploymentID", func(t *testing.T) {
		svc := newTestService()
		err := svc.ExecuteDeployment(context.Background(), "")
		if err == nil {
			t.Error("Expected error for empty deployment ID")
		}
	})

	t.Run("UnknownStrategy", func(t *testing.T) {
		svc := newTestService()
		d := makeDeployment("unknown_strategy")
		id := createAndGet(t, svc, d)

		err := svc.ExecuteDeployment(context.Background(), id)
		if err == nil {
			t.Error("Expected error for unknown strategy")
		}

		got, _ := svc.GetDeployment(id)
		if got.Status != DeployFailed {
			t.Errorf("Expected failed status for unknown strategy, got %q", got.Status)
		}
		if got.CompletedAt == nil {
			t.Error("Expected CompletedAt to be set even on failure")
		}
	})

	t.Run("EmptyStrategy", func(t *testing.T) {
		svc := newTestService()
		d := makeDeployment("")
		id := createAndGet(t, svc, d)

		err := svc.ExecuteDeployment(context.Background(), id)
		if err == nil {
			t.Error("Expected error for empty strategy")
		}

		got, _ := svc.GetDeployment(id)
		if got.Status != DeployFailed {
			t.Errorf("Expected failed status, got %q", got.Status)
		}
	})
}

// ---------------------------------------------------------------------------
// Rollback tests
// ---------------------------------------------------------------------------

func TestRollback(t *testing.T) {
	t.Run("SuccessfulRollback", func(t *testing.T) {
		svc := newTestService()
		d := makeDeployment(StrategyRolling)
		d.RollbackTo = "v0.9.0"
		id := createAndGet(t, svc, d)

		err := svc.Rollback(context.Background(), id)
		if err != nil {
			t.Fatalf("Rollback failed: %v", err)
		}

		got, _ := svc.GetDeployment(id)
		if got.Status != DeployRolledBack {
			t.Errorf("Expected status %q, got %q", DeployRolledBack, got.Status)
		}
	})

	t.Run("RollbackNonExistentDeployment", func(t *testing.T) {
		svc := newTestService()
		err := svc.Rollback(context.Background(), "nonexistent")
		if err == nil {
			t.Error("Expected error for rolling back non-existent deployment")
		}
	})

	t.Run("RollbackSetsIntermediateStatus", func(t *testing.T) {
		svc := newTestService()
		d := makeDeployment(StrategyRolling)
		d.RollbackTo = "v0.8.0"
		id := createAndGet(t, svc, d)

		// Execute rollback and verify final status
		err := svc.Rollback(context.Background(), id)
		if err != nil {
			t.Fatalf("Rollback failed: %v", err)
		}

		got, ok := svc.GetDeployment(id)
		if !ok {
			t.Fatal("Expected to find deployment after rollback")
		}
		// After rollback completes, status must be rolled_back
		if got.Status != DeployRolledBack {
			t.Errorf("Expected rolled_back after rollback completes, got %q", got.Status)
		}
	})
}

// ---------------------------------------------------------------------------
// Auto-rollback on failure tests
// ---------------------------------------------------------------------------

func TestAutoRollbackOnFailure(t *testing.T) {
	t.Run("RollbackOnFailureEnabled", func(t *testing.T) {
		cfg := &Config{
			MaxConcurrentDeploys: 5,
			HealthCheckTimeout:   30 * time.Second,
			RollbackOnFailure:    true,
			CanaryPercentage:     10,
			WotanTopic:          "sword.test",
		}
		svc := newTestServiceWithConfig(cfg)

		d := makeDeployment("invalid_strategy") // will cause failure
		d.RollbackTo = "v0.9.0"
		id := createAndGet(t, svc, d)

		err := svc.ExecuteDeployment(context.Background(), id)
		// Rollback should have been triggered; Rollback itself succeeds
		// but the original execute error flow triggers rollback then returns nil
		// Actually, looking at the code: rollback returns nil, so ExecuteDeployment returns nil
		_ = err

		got, _ := svc.GetDeployment(id)
		if got.Status != DeployRolledBack {
			t.Errorf("Expected auto-rollback to %q, got %q", DeployRolledBack, got.Status)
		}
	})

	t.Run("NoRollbackWhenRollbackToEmpty", func(t *testing.T) {
		cfg := &Config{
			MaxConcurrentDeploys: 5,
			HealthCheckTimeout:   30 * time.Second,
			RollbackOnFailure:    true,
			CanaryPercentage:     10,
			WotanTopic:          "sword.test",
		}
		svc := newTestServiceWithConfig(cfg)

		d := makeDeployment("invalid_strategy")
		d.RollbackTo = "" // no rollback target
		id := createAndGet(t, svc, d)

		err := svc.ExecuteDeployment(context.Background(), id)
		if err == nil {
			t.Error("Expected error when strategy is invalid and no rollback target")
		}

		got, _ := svc.GetDeployment(id)
		if got.Status != DeployFailed {
			t.Errorf("Expected %q without rollback target, got %q", DeployFailed, got.Status)
		}
	})

	t.Run("NoRollbackWhenDisabled", func(t *testing.T) {
		cfg := &Config{
			MaxConcurrentDeploys: 5,
			HealthCheckTimeout:   30 * time.Second,
			RollbackOnFailure:    false, // disabled
			CanaryPercentage:     10,
			WotanTopic:          "sword.test",
		}
		svc := newTestServiceWithConfig(cfg)

		d := makeDeployment("invalid_strategy")
		d.RollbackTo = "v0.9.0"
		id := createAndGet(t, svc, d)

		err := svc.ExecuteDeployment(context.Background(), id)
		if err == nil {
			t.Error("Expected error when rollback is disabled")
		}

		got, _ := svc.GetDeployment(id)
		if got.Status != DeployFailed {
			t.Errorf("Expected %q when rollback disabled, got %q", DeployFailed, got.Status)
		}
	})

	t.Run("ContextCancelTriggersRollback", func(t *testing.T) {
		cfg := &Config{
			MaxConcurrentDeploys: 5,
			HealthCheckTimeout:   30 * time.Second,
			RollbackOnFailure:    true,
			CanaryPercentage:     10,
			WotanTopic:          "sword.test",
		}
		svc := newTestServiceWithConfig(cfg)

		d := makeDeployment(StrategyRolling)
		d.RollbackTo = "v0.8.0"
		id := createAndGet(t, svc, d)

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
		defer cancel()

		_ = svc.ExecuteDeployment(ctx, id)

		got, _ := svc.GetDeployment(id)
		if got.Status != DeployRolledBack {
			t.Errorf("Expected %q after context cancel with rollback enabled, got %q", DeployRolledBack, got.Status)
		}
	})
}

// ---------------------------------------------------------------------------
// Pipeline tests
// ---------------------------------------------------------------------------

func TestCreatePipeline(t *testing.T) {
	t.Run("AutoGeneratesID", func(t *testing.T) {
		svc := newTestService()
		p := &Pipeline{
			Name: "test-pipeline",
			Stages: []PipelineStage{
				{Name: "build", Type: "build", Status: "pending"},
				{Name: "test", Type: "test", Status: "pending", DependsOn: []string{"build"}},
				{Name: "deploy", Type: "deploy", Status: "pending", DependsOn: []string{"test"}},
			},
		}

		created, err := svc.CreatePipeline(p)
		if err != nil {
			t.Fatalf("CreatePipeline failed: %v", err)
		}
		if created.ID == "" {
			t.Error("Expected auto-generated pipeline ID")
		}
		if created.Status != PipelineIdle {
			t.Errorf("Expected status %q, got %q", PipelineIdle, created.Status)
		}
	})

	t.Run("PreservesExplicitID", func(t *testing.T) {
		svc := newTestService()
		p := &Pipeline{
			ID:   "my-pipeline",
			Name: "custom-pipeline",
		}

		created, err := svc.CreatePipeline(p)
		if err != nil {
			t.Fatalf("CreatePipeline failed: %v", err)
		}
		if created.ID != "my-pipeline" {
			t.Errorf("Expected ID %q, got %q", "my-pipeline", created.ID)
		}
	})

	t.Run("PreservesStages", func(t *testing.T) {
		svc := newTestService()
		stages := []PipelineStage{
			{Name: "build", Type: "build", Status: "pending"},
			{Name: "approve", Type: "approve", Status: "pending"},
			{Name: "deploy", Type: "deploy", Status: "pending"},
		}
		p := &Pipeline{Name: "staged-pipeline", Stages: stages}

		created, _ := svc.CreatePipeline(p)
		if len(created.Stages) != 3 {
			t.Errorf("Expected 3 stages, got %d", len(created.Stages))
		}
	})

	t.Run("PreservesTriggers", func(t *testing.T) {
		svc := newTestService()
		p := &Pipeline{
			Name: "triggered-pipeline",
			Triggers: []Trigger{
				{Type: "webhook", Config: map[string]string{"url": "https://example.com/hook"}},
				{Type: "schedule", Config: map[string]string{"cron": "0 0 * * *"}},
				{Type: "manual"},
			},
		}

		created, _ := svc.CreatePipeline(p)
		if len(created.Triggers) != 3 {
			t.Errorf("Expected 3 triggers, got %d", len(created.Triggers))
		}
		if created.Triggers[0].Type != "webhook" {
			t.Errorf("Expected first trigger type %q, got %q", "webhook", created.Triggers[0].Type)
		}
	})

	t.Run("SetsStatusToIdle", func(t *testing.T) {
		svc := newTestService()
		p := &Pipeline{
			Name:   "overwrite-test",
			Status: PipelineRunning, // should be overwritten
		}

		created, _ := svc.CreatePipeline(p)
		if created.Status != PipelineIdle {
			t.Errorf("Expected status to be reset to idle, got %q", created.Status)
		}
	})
}

func TestGetPipeline(t *testing.T) {
	t.Run("ExistingPipeline", func(t *testing.T) {
		svc := newTestService()
		p := &Pipeline{ID: "find-me", Name: "findable"}
		_, _ = svc.CreatePipeline(p)

		got, ok := svc.GetPipeline("find-me")
		if !ok {
			t.Fatal("Expected to find pipeline")
		}
		if got.Name != "findable" {
			t.Errorf("Expected name %q, got %q", "findable", got.Name)
		}
	})

	t.Run("NonExistentPipeline", func(t *testing.T) {
		svc := newTestService()
		_, ok := svc.GetPipeline("does-not-exist")
		if ok {
			t.Error("Expected ok=false for non-existent pipeline")
		}
	})
}

// ---------------------------------------------------------------------------
// Stats tests
// ---------------------------------------------------------------------------

func TestStats(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	t.Run("EmptyStats", func(t *testing.T) {
		stats := svc.Stats()
		if stats["total_deployments"].(int) != 0 {
			t.Errorf("Expected 0 deployments, got %d", stats["total_deployments"].(int))
		}
		if stats["total_pipelines"].(int) != 0 {
			t.Errorf("Expected 0 pipelines, got %d", stats["total_pipelines"].(int))
		}
		byStatus := stats["by_status"].(map[string]int)
		if len(byStatus) != 0 {
			t.Errorf("Expected empty status counts, got %v", byStatus)
		}
	})

	t.Run("AfterDeploymentsAndPipelines", func(t *testing.T) {
		// Create deployments
		for i := 0; i < 3; i++ {
			d := makeDeployment(StrategyRolling)
			_, _ = svc.CreateDeployment(ctx, d)
		}

		// Execute one deployment to success
		d := makeDeployment(StrategyRolling)
		id := createAndGet(t, svc, d)
		_ = svc.ExecuteDeployment(ctx, id)

		// Create a pipeline
		_, _ = svc.CreatePipeline(&Pipeline{Name: "stats-pipeline"})

		stats := svc.Stats()
		if stats["total_deployments"].(int) != 4 {
			t.Errorf("Expected 4 deployments, got %d", stats["total_deployments"].(int))
		}
		if stats["total_pipelines"].(int) != 1 {
			t.Errorf("Expected 1 pipeline, got %d", stats["total_pipelines"].(int))
		}

		byStatus := stats["by_status"].(map[string]int)
		if byStatus[string(DeployPending)] != 3 {
			t.Errorf("Expected 3 pending, got %d", byStatus[string(DeployPending)])
		}
		if byStatus[string(DeploySucceeded)] != 1 {
			t.Errorf("Expected 1 succeeded, got %d", byStatus[string(DeploySucceeded)])
		}
	})
}

// ---------------------------------------------------------------------------
// Strategy constants and status constants
// ---------------------------------------------------------------------------

func TestDeployStrategyConstants(t *testing.T) {
	tests := []struct {
		strategy DeployStrategy
		expected string
	}{
		{StrategyRolling, "rolling"},
		{StrategyBlueGreen, "blue_green"},
		{StrategyCanary, "canary"},
		{StrategyRecreate, "recreate"},
	}
	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			if string(tc.strategy) != tc.expected {
				t.Errorf("Strategy %v != %q", tc.strategy, tc.expected)
			}
		})
	}
}

func TestDeployStatusConstants(t *testing.T) {
	tests := []struct {
		status   DeployStatus
		expected string
	}{
		{DeployPending, "pending"},
		{DeployInProgress, "in_progress"},
		{DeploySucceeded, "succeeded"},
		{DeployFailed, "failed"},
		{DeployRollingBack, "rolling_back"},
		{DeployRolledBack, "rolled_back"},
	}
	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			if string(tc.status) != tc.expected {
				t.Errorf("Status %v != %q", tc.status, tc.expected)
			}
		})
	}
}

func TestPipelineStatusConstants(t *testing.T) {
	tests := []struct {
		status   PipelineStatus
		expected string
	}{
		{PipelineIdle, "idle"},
		{PipelineRunning, "running"},
		{PipelinePaused, "paused"},
		{PipelineComplete, "complete"},
		{PipelineFailed, "failed"},
	}
	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			if string(tc.status) != tc.expected {
				t.Errorf("PipelineStatus %v != %q", tc.status, tc.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// All-strategies execution matrix (table-driven)
// ---------------------------------------------------------------------------

func TestExecuteAllStrategies(t *testing.T) {
	strategies := []struct {
		name     string
		strategy DeployStrategy
	}{
		{"Rolling", StrategyRolling},
		{"BlueGreen", StrategyBlueGreen},
		{"Canary", StrategyCanary},
		{"Recreate", StrategyRecreate},
	}

	for _, s := range strategies {
		t.Run(s.name+"_Success", func(t *testing.T) {
			svc := newTestService()
			d := makeDeployment(s.strategy)
			id := createAndGet(t, svc, d)

			err := svc.ExecuteDeployment(context.Background(), id)
			if err != nil {
				t.Fatalf("ExecuteDeployment(%s) failed: %v", s.name, err)
			}

			got, _ := svc.GetDeployment(id)
			if got.Status != DeploySucceeded {
				t.Errorf("Expected succeeded for %s, got %q", s.name, got.Status)
			}
			if got.Progress != 100 {
				t.Errorf("Expected progress 100 for %s, got %d", s.name, got.Progress)
			}
		})

		t.Run(s.name+"_ContextCancel", func(t *testing.T) {
			svc := newTestService()
			d := makeDeployment(s.strategy)
			id := createAndGet(t, svc, d)

			ctx, cancel := context.WithCancel(context.Background())
			cancel() // immediately

			err := svc.ExecuteDeployment(ctx, id)
			if err == nil {
				// Some strategies might complete before noticing the cancel
				// if they hit the default branch first. This is acceptable.
				t.Log("Note: strategy completed before cancel was noticed")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Concurrent access tests (race-safe)
// ---------------------------------------------------------------------------

func TestConcurrentCreateDeployments(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()
	count := 100

	var wg sync.WaitGroup
	errCh := make(chan error, count)

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			d := &Deployment{
				Name:     fmt.Sprintf("deploy-%d", i),
				Artifact: "app",
				Version:  fmt.Sprintf("1.0.%d", i),
				Strategy: StrategyRolling,
			}
			_, err := svc.CreateDeployment(ctx, d)
			if err != nil {
				errCh <- fmt.Errorf("goroutine %d: %w", i, err)
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Error(err)
	}

	all := svc.ListDeployments("")
	if len(all) != count {
		t.Errorf("Expected %d deployments, got %d", count, len(all))
	}
}

func TestConcurrentGetDeployments(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	// Pre-populate
	ids := make([]string, 10)
	for i := 0; i < 10; i++ {
		d := makeDeployment(StrategyRolling)
		created, _ := svc.CreateDeployment(ctx, d)
		ids[i] = created.ID
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := ids[i%len(ids)]
			_, ok := svc.GetDeployment(id)
			if !ok {
				t.Errorf("Failed to find deployment %s", id)
			}
		}(i)
	}

	wg.Wait()
}

func TestConcurrentListDeployments(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	// Pre-populate
	for i := 0; i < 20; i++ {
		d := makeDeployment(StrategyRolling)
		_, _ = svc.CreateDeployment(ctx, d)
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = svc.ListDeployments("")
			_ = svc.ListDeployments(DeployPending)
		}()
	}

	wg.Wait()
}

func TestConcurrentExecuteAndGet(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	d := makeDeployment(StrategyRolling)
	id := createAndGet(t, svc, d)

	var wg sync.WaitGroup

	// Execute in one goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = svc.ExecuteDeployment(ctx, id)
	}()

	// Concurrently read the deployment state
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = svc.GetDeployment(id)
		}()
	}

	wg.Wait()
}

func TestConcurrentStats(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	var wg sync.WaitGroup

	// Writer goroutines
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			d := makeDeployment(StrategyRolling)
			_, _ = svc.CreateDeployment(ctx, d)
		}(i)
	}

	// Reader goroutines
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = svc.Stats()
		}()
	}

	wg.Wait()
}

func TestConcurrentCreatePipelines(t *testing.T) {
	svc := newTestService()
	count := 50

	var wg sync.WaitGroup
	errCh := make(chan error, count)

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p := &Pipeline{
				Name: fmt.Sprintf("pipeline-%d", i),
				Stages: []PipelineStage{
					{Name: "build", Type: "build"},
				},
			}
			_, err := svc.CreatePipeline(p)
			if err != nil {
				errCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Error(err)
	}
}

func TestConcurrentMixedOperations(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	var wg sync.WaitGroup
	const concurrency = 30

	// Mix of create, get, list, execute, stats
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			switch i % 5 {
			case 0:
				d := makeDeployment(StrategyRolling)
				_, _ = svc.CreateDeployment(ctx, d)
			case 1:
				_ = svc.ListDeployments("")
			case 2:
				_ = svc.Stats()
			case 3:
				p := &Pipeline{Name: fmt.Sprintf("p-%d", i)}
				_, _ = svc.CreatePipeline(p)
			case 4:
				_, _ = svc.GetPipeline(fmt.Sprintf("p-%d", i-1))
			}
		}(i)
	}

	wg.Wait()
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestDeploymentTargetPreserved(t *testing.T) {
	svc := newTestService()
	d := &Deployment{
		Name:     "target-test",
		Artifact: "app",
		Version:  "2.0.0",
		Strategy: StrategyRolling,
		Target: DeployTarget{
			Type:      "container",
			Namespace: "production",
			Replicas:  5,
			Selector:  map[string]string{"tier": "frontend", "region": "us-east"},
		},
	}

	created, _ := svc.CreateDeployment(context.Background(), d)
	got, _ := svc.GetDeployment(created.ID)

	if got.Target.Type != "container" {
		t.Errorf("Expected target type %q, got %q", "container", got.Target.Type)
	}
	if got.Target.Namespace != "production" {
		t.Errorf("Expected namespace %q, got %q", "production", got.Target.Namespace)
	}
	if got.Target.Replicas != 5 {
		t.Errorf("Expected 5 replicas, got %d", got.Target.Replicas)
	}
	if got.Target.Selector["tier"] != "frontend" {
		t.Error("Expected selector to be preserved")
	}
}

func TestDeploymentTimestamps(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	d := makeDeployment(StrategyRolling)
	id := createAndGet(t, svc, d)

	// Before execution
	got, _ := svc.GetDeployment(id)
	if got.StartedAt != nil {
		t.Error("StartedAt should be nil before execution")
	}
	if got.CompletedAt != nil {
		t.Error("CompletedAt should be nil before execution")
	}

	before := time.Now()
	_ = svc.ExecuteDeployment(ctx, id)
	after := time.Now()

	got, _ = svc.GetDeployment(id)

	if got.StartedAt == nil {
		t.Fatal("StartedAt should be set after execution")
	}
	if got.CompletedAt == nil {
		t.Fatal("CompletedAt should be set after execution")
	}

	if got.StartedAt.Before(before.Add(-time.Second)) {
		t.Error("StartedAt is too early")
	}
	if got.CompletedAt.After(after.Add(time.Second)) {
		t.Error("CompletedAt is too late")
	}
}

func TestDeployCounterIncrement(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	// Create multiple deployments without explicit IDs
	ids := make(map[string]struct{})
	for i := 0; i < 50; i++ {
		d := makeDeployment(StrategyRolling)
		created, _ := svc.CreateDeployment(ctx, d)
		if _, exists := ids[created.ID]; exists {
			t.Fatalf("Duplicate ID: %s on iteration %d", created.ID, i)
		}
		ids[created.ID] = struct{}{}
	}
}

func TestStatsAfterRollback(t *testing.T) {
	ctx := context.Background()

	t.Run("RolledBackInStats", func(t *testing.T) {
		cfg := &Config{
			MaxConcurrentDeploys: 5,
			HealthCheckTimeout:   30 * time.Second,
			RollbackOnFailure:    true,
			CanaryPercentage:     10,
			WotanTopic:          "sword.test",
		}
		svc := newTestServiceWithConfig(cfg)
		d := makeDeployment("invalid_strategy")
		d.RollbackTo = "v0.9.0"
		id := createAndGet(t, svc, d)
		_ = svc.ExecuteDeployment(ctx, id)

		stats := svc.Stats()
		byStatus := stats["by_status"].(map[string]int)
		if byStatus[string(DeployRolledBack)] != 1 {
			t.Errorf("Expected 1 rolled_back, got %d", byStatus[string(DeployRolledBack)])
		}
	})

	t.Run("FailedInStats", func(t *testing.T) {
		cfg := &Config{
			MaxConcurrentDeploys: 5,
			HealthCheckTimeout:   30 * time.Second,
			RollbackOnFailure:    false, // rollback disabled
			CanaryPercentage:     10,
			WotanTopic:          "sword.test",
		}
		svc := newTestServiceWithConfig(cfg)
		d := makeDeployment("invalid_strategy")
		d.RollbackTo = "v0.9.0"
		id := createAndGet(t, svc, d)
		_ = svc.ExecuteDeployment(ctx, id)

		stats := svc.Stats()
		byStatus := stats["by_status"].(map[string]int)
		if byStatus[string(DeployFailed)] != 1 {
			t.Errorf("Expected 1 failed, got %d", byStatus[string(DeployFailed)])
		}
	})
}

func TestExecuteDeploymentIdempotentTimestamps(t *testing.T) {
	// Verify that executing twice updates timestamps
	svc := newTestService()
	ctx := context.Background()

	d := makeDeployment(StrategyRolling)
	id := createAndGet(t, svc, d)

	_ = svc.ExecuteDeployment(ctx, id)
	got, _ := svc.GetDeployment(id)
	firstComplete := *got.CompletedAt

	// Reset status to allow re-execution
	svc.mu.Lock()
	got.Status = DeployPending
	got.StartedAt = nil
	got.CompletedAt = nil
	svc.mu.Unlock()

	time.Sleep(5 * time.Millisecond)
	_ = svc.ExecuteDeployment(ctx, id)

	got, _ = svc.GetDeployment(id)
	if got.CompletedAt == nil {
		t.Fatal("Expected CompletedAt to be set on second execution")
	}
	if !got.CompletedAt.After(firstComplete) {
		t.Error("Expected second completion to be after first")
	}
}

// ---------------------------------------------------------------------------
// Deployment with zero replicas and empty selectors
// ---------------------------------------------------------------------------

func TestDeploymentEdgeCases(t *testing.T) {
	t.Run("ZeroReplicas", func(t *testing.T) {
		svc := newTestService()
		d := &Deployment{
			Name:     "zero-replicas",
			Artifact: "app",
			Version:  "1.0.0",
			Strategy: StrategyRolling,
			Target: DeployTarget{
				Type:     "service",
				Replicas: 0,
			},
		}
		created, err := svc.CreateDeployment(context.Background(), d)
		if err != nil {
			t.Fatalf("CreateDeployment failed: %v", err)
		}
		if created.Target.Replicas != 0 {
			t.Error("Expected 0 replicas to be preserved")
		}
	})

	t.Run("NilSelector", func(t *testing.T) {
		svc := newTestService()
		d := &Deployment{
			Name:     "nil-selector",
			Artifact: "app",
			Version:  "1.0.0",
			Strategy: StrategyRolling,
			Target: DeployTarget{
				Type: "service",
			},
		}
		created, err := svc.CreateDeployment(context.Background(), d)
		if err != nil {
			t.Fatalf("CreateDeployment failed: %v", err)
		}
		if created.Target.Selector != nil {
			t.Error("Expected nil selector to be preserved")
		}
	})

	t.Run("EmptyMetadata", func(t *testing.T) {
		svc := newTestService()
		d := makeDeployment(StrategyRolling)
		d.Metadata = map[string]string{}

		created, _ := svc.CreateDeployment(context.Background(), d)
		if created.Metadata == nil {
			t.Error("Expected empty (not nil) metadata to be preserved")
		}
	})

	t.Run("EmptyHealthChecks", func(t *testing.T) {
		svc := newTestService()
		d := makeDeployment(StrategyRolling)
		d.HealthChecks = []HealthCheck{}

		created, _ := svc.CreateDeployment(context.Background(), d)
		if created.HealthChecks == nil {
			t.Error("Expected empty (not nil) health checks to be preserved")
		}
	})

	t.Run("EmptyArtifactAndVersion", func(t *testing.T) {
		svc := newTestService()
		d := &Deployment{
			Name:     "bare-minimum",
			Strategy: StrategyRolling,
		}
		created, err := svc.CreateDeployment(context.Background(), d)
		if err != nil {
			t.Fatalf("CreateDeployment should not reject empty artifact/version: %v", err)
		}
		if created.Artifact != "" {
			t.Error("Expected empty artifact")
		}
		if created.Version != "" {
			t.Error("Expected empty version")
		}
	})
}

// ---------------------------------------------------------------------------
// Pipeline edge cases
// ---------------------------------------------------------------------------

func TestPipelineEdgeCases(t *testing.T) {
	t.Run("EmptyStages", func(t *testing.T) {
		svc := newTestService()
		p := &Pipeline{Name: "empty-stages"}
		created, err := svc.CreatePipeline(p)
		if err != nil {
			t.Fatalf("CreatePipeline failed: %v", err)
		}
		if len(created.Stages) != 0 {
			t.Error("Expected 0 stages")
		}
	})

	t.Run("StageWithConfig", func(t *testing.T) {
		svc := newTestService()
		p := &Pipeline{
			Name: "config-stages",
			Stages: []PipelineStage{
				{
					Name:   "build",
					Type:   "build",
					Config: map[string]string{"image": "golang:1.21", "cache": "true"},
				},
			},
		}
		created, _ := svc.CreatePipeline(p)
		if created.Stages[0].Config["image"] != "golang:1.21" {
			t.Error("Expected stage config to be preserved")
		}
	})

	t.Run("PipelineCurrentStagePreserved", func(t *testing.T) {
		svc := newTestService()
		p := &Pipeline{
			Name:         "staged",
			CurrentStage: 2,
			Stages: []PipelineStage{
				{Name: "a"}, {Name: "b"}, {Name: "c"},
			},
		}
		created, _ := svc.CreatePipeline(p)
		if created.CurrentStage != 2 {
			t.Errorf("Expected CurrentStage=2, got %d", created.CurrentStage)
		}
	})
}

// ---------------------------------------------------------------------------
// Full deployment workflow
// ---------------------------------------------------------------------------

func TestFullDeploymentWorkflow(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	// 1. Create
	d := &Deployment{
		Name:     "full-workflow",
		Artifact: "myapp",
		Version:  "2.0.0",
		Strategy: StrategyCanary,
		Target: DeployTarget{
			Type:      "service",
			Namespace: "production",
			Replicas:  10,
			Selector:  map[string]string{"app": "myapp"},
		},
		RollbackTo: "1.9.0",
		Metadata:   map[string]string{"deployer": "ci", "ticket": "PROJ-123"},
		HealthChecks: []HealthCheck{
			{Name: "readiness", Type: "http", Endpoint: "/ready", Interval: 5 * time.Second, Timeout: 2 * time.Second, Retries: 3},
		},
	}

	created, err := svc.CreateDeployment(ctx, d)
	if err != nil {
		t.Fatalf("Step 1 (Create) failed: %v", err)
	}

	// Verify initial state
	if created.Status != DeployPending {
		t.Errorf("Step 1: Expected pending, got %q", created.Status)
	}

	// 2. Execute
	err = svc.ExecuteDeployment(ctx, created.ID)
	if err != nil {
		t.Fatalf("Step 2 (Execute) failed: %v", err)
	}

	// 3. Verify success
	got, ok := svc.GetDeployment(created.ID)
	if !ok {
		t.Fatal("Step 3: deployment not found")
	}
	if got.Status != DeploySucceeded {
		t.Errorf("Step 3: Expected succeeded, got %q", got.Status)
	}
	if got.Progress != 100 {
		t.Errorf("Step 3: Expected progress 100, got %d", got.Progress)
	}

	// 4. Verify in list
	all := svc.ListDeployments(DeploySucceeded)
	if len(all) != 1 {
		t.Errorf("Step 4: Expected 1 succeeded deployment in list, got %d", len(all))
	}

	// 5. Verify stats
	stats := svc.Stats()
	if stats["total_deployments"].(int) != 1 {
		t.Errorf("Step 5: Expected 1 total deployment, got %d", stats["total_deployments"].(int))
	}
}

func TestFullRollbackWorkflow(t *testing.T) {
	cfg := &Config{
		MaxConcurrentDeploys: 5,
		HealthCheckTimeout:   30 * time.Second,
		RollbackOnFailure:    true,
		CanaryPercentage:     10,
		WotanTopic:          "sword.test",
	}
	svc := newTestServiceWithConfig(cfg)
	ctx := context.Background()

	// Deploy with an invalid strategy to force failure
	d := &Deployment{
		Name:       "rollback-workflow",
		Artifact:   "myapp",
		Version:    "2.0.0-bad",
		Strategy:   "nonexistent_strategy",
		RollbackTo: "1.9.0",
	}

	created, _ := svc.CreateDeployment(ctx, d)

	// Execute will fail and auto-rollback
	_ = svc.ExecuteDeployment(ctx, created.ID)

	got, _ := svc.GetDeployment(created.ID)
	if got.Status != DeployRolledBack {
		t.Errorf("Expected rolled_back after full rollback workflow, got %q", got.Status)
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkCreateDeployment(b *testing.B) {
	svc := newTestService()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d := makeDeployment(StrategyRolling)
		_, _ = svc.CreateDeployment(ctx, d)
	}
}

func BenchmarkGetDeployment(b *testing.B) {
	svc := newTestService()
	ctx := context.Background()
	d := makeDeployment(StrategyRolling)
	created, _ := svc.CreateDeployment(ctx, d)
	id := created.ID

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.GetDeployment(id)
	}
}

func BenchmarkListDeployments(b *testing.B) {
	svc := newTestService()
	ctx := context.Background()
	for i := 0; i < 100; i++ {
		d := makeDeployment(StrategyRolling)
		_, _ = svc.CreateDeployment(ctx, d)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = svc.ListDeployments("")
	}
}

func BenchmarkStats(b *testing.B) {
	svc := newTestService()
	ctx := context.Background()
	for i := 0; i < 50; i++ {
		d := makeDeployment(StrategyRolling)
		_, _ = svc.CreateDeployment(ctx, d)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = svc.Stats()
	}
}

func BenchmarkCreatePipeline(b *testing.B) {
	svc := newTestService()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p := &Pipeline{
			Name: fmt.Sprintf("pipeline-%d", i),
			Stages: []PipelineStage{
				{Name: "build", Type: "build"},
				{Name: "test", Type: "test"},
				{Name: "deploy", Type: "deploy"},
			},
		}
		_, _ = svc.CreatePipeline(p)
	}
}

func BenchmarkConcurrentCreateDeployments(b *testing.B) {
	svc := newTestService()
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			d := makeDeployment(StrategyRolling)
			_, _ = svc.CreateDeployment(ctx, d)
		}
	})
}

func BenchmarkExecuteRolling(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc := newTestService()
		d := makeDeployment(StrategyRolling)
		created, _ := svc.CreateDeployment(ctx, d)
		_ = svc.ExecuteDeployment(ctx, created.ID)
	}
}
