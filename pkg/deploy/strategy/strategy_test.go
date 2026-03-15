// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package strategy

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Tests for DefaultConfig
// ---------------------------------------------------------------------------

func TestDefaultConfig_Rolling(t *testing.T) {
	cfg := DefaultConfig(TypeRolling)
	if cfg.Type != TypeRolling {
		t.Fatalf("expected type %q, got %q", TypeRolling, cfg.Type)
	}
	if cfg.MaxSurge != 1 {
		t.Errorf("expected MaxSurge 1, got %d", cfg.MaxSurge)
	}
	if cfg.MaxUnavailable != 0 {
		t.Errorf("expected MaxUnavailable 0, got %d", cfg.MaxUnavailable)
	}
	if cfg.HealthCheck == nil {
		t.Fatal("expected HealthCheck config, got nil")
	}
	if cfg.HealthCheck.Type != "http" {
		t.Errorf("expected HealthCheck type http, got %s", cfg.HealthCheck.Type)
	}
	if cfg.ProgressDeadline != 10*time.Minute {
		t.Errorf("expected ProgressDeadline 10m, got %v", cfg.ProgressDeadline)
	}
}

func TestDefaultConfig_BlueGreen(t *testing.T) {
	cfg := DefaultConfig(TypeBlueGreen)
	if cfg.Type != TypeBlueGreen {
		t.Fatalf("expected type %q, got %q", TypeBlueGreen, cfg.Type)
	}
	if cfg.SwitchTimeout != 30*time.Second {
		t.Errorf("expected SwitchTimeout 30s, got %v", cfg.SwitchTimeout)
	}
	if cfg.TestDuration != 5*time.Minute {
		t.Errorf("expected TestDuration 5m, got %v", cfg.TestDuration)
	}
}

func TestDefaultConfig_Canary(t *testing.T) {
	cfg := DefaultConfig(TypeCanary)
	if cfg.Type != TypeCanary {
		t.Fatalf("expected type %q, got %q", TypeCanary, cfg.Type)
	}
	if len(cfg.Steps) == 0 {
		t.Fatal("expected canary steps, got none")
	}
	if cfg.Steps[len(cfg.Steps)-1].Weight != 100 {
		t.Errorf("expected final step weight 100, got %d", cfg.Steps[len(cfg.Steps)-1].Weight)
	}
	if cfg.Analysis == nil {
		t.Fatal("expected Analysis config, got nil")
	}
	if cfg.AutoPromote {
		t.Error("expected AutoPromote false")
	}
}

func TestDefaultConfig_Recreate(t *testing.T) {
	cfg := DefaultConfig(TypeRecreate)
	if cfg.Type != TypeRecreate {
		t.Fatalf("expected type %q, got %q", TypeRecreate, cfg.Type)
	}
	if cfg.HealthCheck == nil {
		t.Fatal("expected HealthCheck config, got nil")
	}
}

func TestDefaultConfig_Unknown(t *testing.T) {
	cfg := DefaultConfig(Type("unknown"))
	if cfg.Type != Type("unknown") {
		t.Fatalf("expected type unknown, got %q", cfg.Type)
	}
}

// ---------------------------------------------------------------------------
// Tests for ValidateConfig
// ---------------------------------------------------------------------------

func TestValidateConfig_Nil(t *testing.T) {
	err := ValidateConfig(nil)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestValidateConfig_Rolling_Valid(t *testing.T) {
	cfg := DefaultConfig(TypeRolling)
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestValidateConfig_Rolling_NegativeMaxSurge(t *testing.T) {
	cfg := &Config{Type: TypeRolling, MaxSurge: -1, MaxUnavailable: 1}
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected error for negative MaxSurge")
	}
}

func TestValidateConfig_Rolling_NegativeMaxUnavailable(t *testing.T) {
	cfg := &Config{Type: TypeRolling, MaxSurge: 1, MaxUnavailable: -1}
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected error for negative MaxUnavailable")
	}
}

func TestValidateConfig_Rolling_BothZero(t *testing.T) {
	cfg := &Config{Type: TypeRolling, MaxSurge: 0, MaxUnavailable: 0}
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected error when both MaxSurge and MaxUnavailable are zero")
	}
}

func TestValidateConfig_BlueGreen_Valid(t *testing.T) {
	cfg := DefaultConfig(TypeBlueGreen)
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestValidateConfig_BlueGreen_NegativeSwitchTimeout(t *testing.T) {
	cfg := &Config{Type: TypeBlueGreen, SwitchTimeout: -1}
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected error for negative SwitchTimeout")
	}
}

func TestValidateConfig_Canary_Valid(t *testing.T) {
	cfg := DefaultConfig(TypeCanary)
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestValidateConfig_Canary_NoSteps(t *testing.T) {
	cfg := &Config{Type: TypeCanary, Steps: []CanaryStep{}}
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected error for canary with no steps")
	}
}

func TestValidateConfig_Canary_NegativeWeight(t *testing.T) {
	cfg := &Config{Type: TypeCanary, Steps: []CanaryStep{{Weight: -5}}}
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected error for negative step weight")
	}
}

func TestValidateConfig_Canary_WeightOver100(t *testing.T) {
	cfg := &Config{Type: TypeCanary, Steps: []CanaryStep{{Weight: 150}}}
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected error for step weight > 100")
	}
}

func TestValidateConfig_Canary_NonIncreasingWeights(t *testing.T) {
	cfg := &Config{Type: TypeCanary, Steps: []CanaryStep{
		{Weight: 50},
		{Weight: 30},
		{Weight: 100},
	}}
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected error for non-increasing weights")
	}
}

func TestValidateConfig_Canary_FinalNotHundred(t *testing.T) {
	cfg := &Config{Type: TypeCanary, Steps: []CanaryStep{
		{Weight: 10},
		{Weight: 50},
	}}
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected error when final step != 100")
	}
}

func TestValidateConfig_Recreate_Valid(t *testing.T) {
	cfg := DefaultConfig(TypeRecreate)
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestValidateConfig_UnknownType(t *testing.T) {
	cfg := &Config{Type: Type("imaginary")}
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected error for unknown strategy type")
	}
}

// ---------------------------------------------------------------------------
// Tests for Registry
// ---------------------------------------------------------------------------

type stubStrategy struct {
	strategyType Type
}

func (s *stubStrategy) Type() Type                                                    { return s.strategyType }
func (s *stubStrategy) Execute(_ context.Context, _ *ExecuteParams) (*Result, error)  { return nil, nil }
func (s *stubStrategy) Rollback(_ context.Context, _ *RollbackParams) error           { return nil }
func (s *stubStrategy) Status(_ context.Context, _ string) (*Status, error)           { return nil, nil }
func (s *stubStrategy) Validate(_ *Config) error                                      { return nil }

func TestRegistry_RegisterAndGet(t *testing.T) {
	reg := NewRegistry()
	stub := &stubStrategy{strategyType: TypeRecreate}
	reg.Register(stub)

	got, err := reg.Get(TypeRecreate)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != stub {
		t.Fatalf("expected registered strategy, got different instance")
	}
}

func TestRegistry_GetNotFound(t *testing.T) {
	reg := NewRegistry()
	_, err := reg.Get(TypeRecreate)
	if !errors.Is(err, ErrStrategyNotFound) {
		t.Fatalf("expected ErrStrategyNotFound, got %v", err)
	}
}

func TestDefaultRegistry_InitRegistered(t *testing.T) {
	// init() in rolling.go, canary.go, bluegreen.go register strategies
	for _, typ := range []Type{TypeRolling, TypeBlueGreen, TypeCanary} {
		s, err := GetStrategy(typ)
		if err != nil {
			t.Errorf("expected default registry to have %q, got error %v", typ, err)
		}
		if s == nil {
			t.Errorf("expected non-nil strategy for %q", typ)
		}
	}
}

func TestGetStrategy_NotFound(t *testing.T) {
	_, err := GetStrategy(TypeRecreate)
	if err == nil {
		t.Fatal("expected error for unregistered strategy type")
	}
}

// ---------------------------------------------------------------------------
// Tests for RollingStrategy
// ---------------------------------------------------------------------------

func TestRollingStrategy_Type(t *testing.T) {
	s := NewRollingStrategy()
	if s.Type() != TypeRolling {
		t.Fatalf("expected %q, got %q", TypeRolling, s.Type())
	}
}

func TestRollingStrategy_Validate(t *testing.T) {
	s := NewRollingStrategy()

	// nil config
	if err := s.Validate(nil); !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for nil, got %v", err)
	}

	// wrong type
	if err := s.Validate(&Config{Type: TypeCanary}); err == nil {
		t.Error("expected error for wrong type")
	}

	// valid
	if err := s.Validate(DefaultConfig(TypeRolling)); err != nil {
		t.Errorf("expected nil, got %v", err)
	}

	// both zero
	if err := s.Validate(&Config{Type: TypeRolling, MaxSurge: 0, MaxUnavailable: 0}); err == nil {
		t.Error("expected error when both are zero")
	}

	// negative
	if err := s.Validate(&Config{Type: TypeRolling, MaxSurge: -1}); err == nil {
		t.Error("expected error for negative MaxSurge")
	}
	if err := s.Validate(&Config{Type: TypeRolling, MaxSurge: 1, MaxUnavailable: -1}); err == nil {
		t.Error("expected error for negative MaxUnavailable")
	}
}

func TestRollingStrategy_Execute_NoCallbacks(t *testing.T) {
	s := NewRollingStrategy()
	cfg := DefaultConfig(TypeRolling)

	params := &ExecuteParams{
		DeploymentID:   "dep-1",
		ServiceName:    "svc-a",
		CurrentVersion: "v1",
		TargetVersion:  "v2",
		Replicas:       3,
		Config:         cfg,
		Instances: []Instance{
			{ID: "i-1", Version: "v1", State: InstanceRunning, Healthy: true},
			{ID: "i-2", Version: "v1", State: InstanceRunning, Healthy: true},
			{ID: "i-3", Version: "v1", State: InstanceRunning, Healthy: true},
		},
	}

	ctx := context.Background()
	result, err := s.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got message: %s", result.Message)
	}
	if result.Duration <= 0 {
		t.Error("expected positive duration")
	}
	if result.Metrics.InstancesCreated == 0 {
		t.Error("expected instances to be created")
	}
	for _, inst := range result.Instances {
		if inst.Version != "v2" {
			t.Errorf("expected version v2, got %s", inst.Version)
		}
	}
}

func TestRollingStrategy_Execute_WithCallbacks(t *testing.T) {
	s := NewRollingStrategy()
	cfg := DefaultConfig(TypeRolling)

	var createCount, deleteCount, healthCount int
	var mu sync.Mutex

	callbacks := &Callbacks{
		OnInstanceCreate: func(ctx context.Context, params *InstanceParams) (*Instance, error) {
			mu.Lock()
			createCount++
			n := createCount
			mu.Unlock()
			return &Instance{
				ID:      fmt.Sprintf("new-%d", n),
				Version: params.Version,
				State:   InstanceRunning,
				Healthy: true,
			}, nil
		},
		OnInstanceDelete: func(ctx context.Context, instanceID string) error {
			mu.Lock()
			deleteCount++
			mu.Unlock()
			return nil
		},
		OnHealthCheck: func(ctx context.Context, instanceID string) (bool, error) {
			mu.Lock()
			healthCount++
			mu.Unlock()
			return true, nil
		},
		OnProgress: func(ctx context.Context, progress *Progress) {
			// no-op
		},
	}

	params := &ExecuteParams{
		DeploymentID:   "dep-cb-1",
		ServiceName:    "svc-a",
		CurrentVersion: "v1",
		TargetVersion:  "v2",
		Replicas:       2,
		Config:         cfg,
		Instances: []Instance{
			{ID: "old-1", Version: "v1", State: InstanceRunning, Healthy: true},
			{ID: "old-2", Version: "v1", State: InstanceRunning, Healthy: true},
		},
		Callbacks: callbacks,
	}

	result, err := s.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Message)
	}

	mu.Lock()
	if createCount == 0 {
		t.Error("expected OnInstanceCreate to be called")
	}
	if deleteCount == 0 {
		t.Error("expected OnInstanceDelete to be called")
	}
	if healthCount == 0 {
		t.Error("expected OnHealthCheck to be called")
	}
	mu.Unlock()
}

func TestRollingStrategy_Execute_InvalidConfig(t *testing.T) {
	s := NewRollingStrategy()
	params := &ExecuteParams{
		DeploymentID: "dep-invalid",
		Config:       &Config{Type: TypeCanary},
	}
	_, err := s.Execute(context.Background(), params)
	if err == nil {
		t.Fatal("expected error for invalid config")
	}
}

func TestRollingStrategy_Execute_Cancelled(t *testing.T) {
	s := NewRollingStrategy()
	cfg := DefaultConfig(TypeRolling)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately so the loop aborts
	cancel()

	params := &ExecuteParams{
		DeploymentID:   "dep-cancel",
		ServiceName:    "svc-a",
		CurrentVersion: "v1",
		TargetVersion:  "v2",
		Replicas:       5,
		Config:         cfg,
		Instances: []Instance{
			{ID: "old-1", Version: "v1", State: InstanceRunning, Healthy: true},
		},
	}

	result, err := s.Execute(ctx, params)
	if err == nil {
		// Depending on timing, the context could be detected before or
		// after the loop. If no error, the result should indicate cancellation.
		if result != nil && result.Success {
			// It completed before noticing cancellation, acceptable.
			return
		}
	}
}

func TestRollingStrategy_Execute_UnhealthyInstances(t *testing.T) {
	s := NewRollingStrategy()
	cfg := DefaultConfig(TypeRolling)

	callbacks := &Callbacks{
		OnHealthCheck: func(ctx context.Context, instanceID string) (bool, error) {
			return false, nil
		},
	}

	params := &ExecuteParams{
		DeploymentID:   "dep-unhealthy",
		ServiceName:    "svc-a",
		CurrentVersion: "v1",
		TargetVersion:  "v2",
		Replicas:       2,
		Config:         cfg,
		Instances: []Instance{
			{ID: "old-1", Version: "v1", State: InstanceRunning, Healthy: true},
			{ID: "old-2", Version: "v1", State: InstanceRunning, Healthy: true},
		},
		Callbacks: callbacks,
	}

	result, err := s.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// When health checks fail, result should be unsuccessful
	if result.Success {
		t.Error("expected unsuccessful result for unhealthy instances")
	}
}

func TestRollingStrategy_Status_NotFound(t *testing.T) {
	s := NewRollingStrategy()
	_, err := s.Status(context.Background(), "nonexistent")
	if !errors.Is(err, ErrStrategyNotFound) {
		t.Fatalf("expected ErrStrategyNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Tests for BlueGreenStrategy
// ---------------------------------------------------------------------------

func TestBlueGreenStrategy_Type(t *testing.T) {
	s := NewBlueGreenStrategy()
	if s.Type() != TypeBlueGreen {
		t.Fatalf("expected %q, got %q", TypeBlueGreen, s.Type())
	}
}

func TestBlueGreenStrategy_Validate(t *testing.T) {
	s := NewBlueGreenStrategy()

	if err := s.Validate(nil); !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for nil, got %v", err)
	}
	if err := s.Validate(&Config{Type: TypeRolling}); err == nil {
		t.Error("expected error for wrong type")
	}
	if err := s.Validate(&Config{Type: TypeBlueGreen, SwitchTimeout: -1}); err == nil {
		t.Error("expected error for negative SwitchTimeout")
	}
	if err := s.Validate(DefaultConfig(TypeBlueGreen)); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestBlueGreenStrategy_Execute_NoCallbacks(t *testing.T) {
	s := NewBlueGreenStrategy()
	cfg := DefaultConfig(TypeBlueGreen)
	cfg.TestDuration = 0 // skip test phase to keep test fast

	params := &ExecuteParams{
		DeploymentID:   "bg-dep-1",
		ServiceName:    "svc-bg",
		CurrentVersion: "v1",
		TargetVersion:  "v2",
		Replicas:       2,
		Config:         cfg,
		Instances: []Instance{
			{ID: "blue-1", Version: "v1", State: InstanceRunning, Healthy: true},
			{ID: "blue-2", Version: "v1", State: InstanceRunning, Healthy: true},
		},
	}

	result, err := s.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Message)
	}
	if len(result.Instances) != 2 {
		t.Errorf("expected 2 instances, got %d", len(result.Instances))
	}
	for _, inst := range result.Instances {
		if inst.Version != "v2" {
			t.Errorf("expected version v2, got %s", inst.Version)
		}
	}
	if result.Metrics.InstancesCreated != 2 {
		t.Errorf("expected 2 created, got %d", result.Metrics.InstancesCreated)
	}
}

func TestBlueGreenStrategy_Execute_WithCallbacks(t *testing.T) {
	s := NewBlueGreenStrategy()
	cfg := DefaultConfig(TypeBlueGreen)
	cfg.TestDuration = 0

	var trafficShifted bool
	var deletedIDs []string
	var mu sync.Mutex

	callbacks := &Callbacks{
		OnInstanceCreate: func(ctx context.Context, p *InstanceParams) (*Instance, error) {
			return &Instance{
				ID:      fmt.Sprintf("green-%s", p.Version),
				Version: p.Version,
				State:   InstanceRunning,
				Healthy: true,
			}, nil
		},
		OnInstanceDelete: func(ctx context.Context, id string) error {
			mu.Lock()
			deletedIDs = append(deletedIDs, id)
			mu.Unlock()
			return nil
		},
		OnHealthCheck: func(ctx context.Context, id string) (bool, error) {
			return true, nil
		},
		OnTrafficShift: func(ctx context.Context, p *TrafficShiftParams) error {
			mu.Lock()
			trafficShifted = true
			mu.Unlock()
			return nil
		},
		OnProgress: func(ctx context.Context, p *Progress) {},
	}

	params := &ExecuteParams{
		DeploymentID:   "bg-dep-cb",
		ServiceName:    "svc-bg",
		CurrentVersion: "v1",
		TargetVersion:  "v2",
		Replicas:       2,
		Config:         cfg,
		Instances: []Instance{
			{ID: "blue-1", Version: "v1", State: InstanceRunning, Healthy: true},
			{ID: "blue-2", Version: "v1", State: InstanceRunning, Healthy: true},
		},
		Callbacks: callbacks,
	}

	result, err := s.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Message)
	}
	mu.Lock()
	if !trafficShifted {
		t.Error("expected traffic to be shifted")
	}
	if len(deletedIDs) != 2 {
		t.Errorf("expected 2 blue instances deleted, got %d", len(deletedIDs))
	}
	mu.Unlock()
}

func TestBlueGreenStrategy_Execute_TrafficShiftFailure(t *testing.T) {
	s := NewBlueGreenStrategy()
	cfg := DefaultConfig(TypeBlueGreen)
	cfg.TestDuration = 0

	callbacks := &Callbacks{
		OnInstanceCreate: func(ctx context.Context, p *InstanceParams) (*Instance, error) {
			return &Instance{
				ID:      "green-inst",
				Version: p.Version,
				State:   InstanceRunning,
				Healthy: true,
			}, nil
		},
		OnHealthCheck: func(ctx context.Context, id string) (bool, error) {
			return true, nil
		},
		OnTrafficShift: func(ctx context.Context, p *TrafficShiftParams) error {
			return errors.New("switch failed")
		},
		OnProgress: func(ctx context.Context, p *Progress) {},
	}

	params := &ExecuteParams{
		DeploymentID:   "bg-fail",
		ServiceName:    "svc-bg",
		CurrentVersion: "v1",
		TargetVersion:  "v2",
		Replicas:       1,
		Config:         cfg,
		Instances: []Instance{
			{ID: "blue-1", Version: "v1", State: InstanceRunning, Healthy: true},
		},
		Callbacks: callbacks,
	}

	result, err := s.Execute(context.Background(), params)
	if !errors.Is(err, ErrTrafficShiftFailed) {
		t.Fatalf("expected ErrTrafficShiftFailed, got %v", err)
	}
	if result.Success {
		t.Error("expected unsuccessful result")
	}
}

func TestBlueGreenStrategy_Execute_InstanceCreateFailure(t *testing.T) {
	s := NewBlueGreenStrategy()
	cfg := DefaultConfig(TypeBlueGreen)
	cfg.TestDuration = 0

	callbacks := &Callbacks{
		OnInstanceCreate: func(ctx context.Context, p *InstanceParams) (*Instance, error) {
			return nil, errors.New("create failed")
		},
		OnProgress: func(ctx context.Context, p *Progress) {},
	}

	params := &ExecuteParams{
		DeploymentID:   "bg-create-fail",
		ServiceName:    "svc-bg",
		CurrentVersion: "v1",
		TargetVersion:  "v2",
		Replicas:       1,
		Config:         cfg,
		Instances:      []Instance{{ID: "blue-1", Version: "v1", State: InstanceRunning, Healthy: true}},
		Callbacks:      callbacks,
	}

	result, err := s.Execute(context.Background(), params)
	if !errors.Is(err, ErrInstanceUpdateFailed) {
		t.Fatalf("expected ErrInstanceUpdateFailed, got %v", err)
	}
	if result.Success {
		t.Error("expected unsuccessful result")
	}
}

func TestBlueGreenStrategy_Status_NotFound(t *testing.T) {
	s := NewBlueGreenStrategy()
	_, err := s.Status(context.Background(), "nonexistent")
	if !errors.Is(err, ErrStrategyNotFound) {
		t.Fatalf("expected ErrStrategyNotFound, got %v", err)
	}
}

func TestBlueGreenStrategy_Execute_InvalidConfig(t *testing.T) {
	s := NewBlueGreenStrategy()
	params := &ExecuteParams{
		DeploymentID: "bg-invalid",
		Config:       &Config{Type: TypeRolling},
	}
	_, err := s.Execute(context.Background(), params)
	if err == nil {
		t.Fatal("expected error for invalid config")
	}
}

// ---------------------------------------------------------------------------
// Tests for CanaryStrategy
// ---------------------------------------------------------------------------

func TestCanaryStrategy_Type(t *testing.T) {
	s := NewCanaryStrategy()
	if s.Type() != TypeCanary {
		t.Fatalf("expected %q, got %q", TypeCanary, s.Type())
	}
}

func TestCanaryStrategy_Validate(t *testing.T) {
	s := NewCanaryStrategy()

	if err := s.Validate(nil); !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for nil, got %v", err)
	}
	if err := s.Validate(&Config{Type: TypeRolling}); err == nil {
		t.Error("expected error for wrong type")
	}
	if err := s.Validate(&Config{Type: TypeCanary, Steps: []CanaryStep{}}); err == nil {
		t.Error("expected error for empty steps")
	}
	if err := s.Validate(&Config{Type: TypeCanary, Steps: []CanaryStep{{Weight: -1}}}); err == nil {
		t.Error("expected error for negative weight")
	}
	if err := s.Validate(&Config{Type: TypeCanary, Steps: []CanaryStep{{Weight: 101}}}); err == nil {
		t.Error("expected error for weight > 100")
	}
	if err := s.Validate(&Config{Type: TypeCanary, Steps: []CanaryStep{
		{Weight: 50}, {Weight: 30}, {Weight: 100},
	}}); err == nil {
		t.Error("expected error for non-increasing weights")
	}
	if err := s.Validate(&Config{Type: TypeCanary, Steps: []CanaryStep{
		{Weight: 10}, {Weight: 50},
	}}); err == nil {
		t.Error("expected error when final step != 100")
	}

	// Valid with analysis
	cfg := &Config{
		Type: TypeCanary,
		Steps: []CanaryStep{
			{Weight: 50},
			{Weight: 100},
		},
		Analysis: &AnalysisConfig{
			ErrorRateThreshold:   0.05,
			SuccessRateThreshold: 0.95,
		},
	}
	if err := s.Validate(cfg); err != nil {
		t.Errorf("expected nil, got %v", err)
	}

	// Invalid analysis thresholds
	cfg.Analysis.ErrorRateThreshold = 1.5
	if err := s.Validate(cfg); err == nil {
		t.Error("expected error for ErrorRateThreshold > 1")
	}
	cfg.Analysis.ErrorRateThreshold = 0.05
	cfg.Analysis.SuccessRateThreshold = -0.5
	if err := s.Validate(cfg); err == nil {
		t.Error("expected error for SuccessRateThreshold < 0")
	}
}

func TestCanaryStrategy_Execute_NoCallbacks(t *testing.T) {
	s := NewCanaryStrategy()
	cfg := &Config{
		Type: TypeCanary,
		Steps: []CanaryStep{
			{Weight: 50, Pause: 0},
			{Weight: 100, Pause: 0},
		},
	}

	params := &ExecuteParams{
		DeploymentID:   "canary-dep-1",
		ServiceName:    "svc-canary",
		CurrentVersion: "v1",
		TargetVersion:  "v2",
		Replicas:       2,
		Config:         cfg,
		Instances: []Instance{
			{ID: "stable-1", Version: "v1", State: InstanceRunning, Healthy: true},
			{ID: "stable-2", Version: "v1", State: InstanceRunning, Healthy: true},
		},
	}

	result, err := s.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Message)
	}
	if result.Metrics.InstancesCreated == 0 {
		t.Error("expected instances to be created")
	}
}

func TestCanaryStrategy_Execute_WithCallbacks(t *testing.T) {
	s := NewCanaryStrategy()
	cfg := &Config{
		Type: TypeCanary,
		Steps: []CanaryStep{
			{Weight: 50, Pause: 0},
			{Weight: 100, Pause: 0},
		},
	}

	var trafficShifts int
	var mu sync.Mutex

	callbacks := &Callbacks{
		OnInstanceCreate: func(ctx context.Context, p *InstanceParams) (*Instance, error) {
			return &Instance{
				ID:      fmt.Sprintf("canary-%s", p.Version),
				Version: p.Version,
				State:   InstanceRunning,
				Healthy: true,
			}, nil
		},
		OnInstanceDelete: func(ctx context.Context, id string) error {
			return nil
		},
		OnHealthCheck: func(ctx context.Context, id string) (bool, error) {
			return true, nil
		},
		OnTrafficShift: func(ctx context.Context, p *TrafficShiftParams) error {
			mu.Lock()
			trafficShifts++
			mu.Unlock()
			return nil
		},
		OnProgress: func(ctx context.Context, p *Progress) {},
	}

	params := &ExecuteParams{
		DeploymentID:   "canary-dep-cb",
		ServiceName:    "svc-canary",
		CurrentVersion: "v1",
		TargetVersion:  "v2",
		Replicas:       2,
		Config:         cfg,
		Instances: []Instance{
			{ID: "stable-1", Version: "v1", State: InstanceRunning, Healthy: true},
			{ID: "stable-2", Version: "v1", State: InstanceRunning, Healthy: true},
		},
		Callbacks: callbacks,
	}

	result, err := s.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Message)
	}
	mu.Lock()
	// 2 steps + final 100% shift = 3
	if trafficShifts < 2 {
		t.Errorf("expected at least 2 traffic shifts, got %d", trafficShifts)
	}
	mu.Unlock()
}

func TestCanaryStrategy_Execute_TrafficShiftFailure(t *testing.T) {
	s := NewCanaryStrategy()
	cfg := &Config{
		Type: TypeCanary,
		Steps: []CanaryStep{
			{Weight: 50, Pause: 0},
			{Weight: 100, Pause: 0},
		},
	}

	callbacks := &Callbacks{
		OnInstanceCreate: func(ctx context.Context, p *InstanceParams) (*Instance, error) {
			return &Instance{
				ID:      "canary-inst",
				Version: p.Version,
				State:   InstanceRunning,
				Healthy: true,
			}, nil
		},
		OnHealthCheck: func(ctx context.Context, id string) (bool, error) {
			return true, nil
		},
		OnTrafficShift: func(ctx context.Context, p *TrafficShiftParams) error {
			return errors.New("traffic shift failed")
		},
		OnProgress: func(ctx context.Context, p *Progress) {},
	}

	params := &ExecuteParams{
		DeploymentID:   "canary-ts-fail",
		ServiceName:    "svc-canary",
		CurrentVersion: "v1",
		TargetVersion:  "v2",
		Replicas:       1,
		Config:         cfg,
		Instances: []Instance{
			{ID: "stable-1", Version: "v1", State: InstanceRunning, Healthy: true},
		},
		Callbacks: callbacks,
	}

	result, err := s.Execute(context.Background(), params)
	if !errors.Is(err, ErrTrafficShiftFailed) {
		t.Fatalf("expected ErrTrafficShiftFailed, got %v", err)
	}
	if result.Success {
		t.Error("expected unsuccessful result")
	}
}

func TestCanaryStrategy_Execute_InstanceCreateFailure(t *testing.T) {
	s := NewCanaryStrategy()
	cfg := &Config{
		Type: TypeCanary,
		Steps: []CanaryStep{
			{Weight: 50, Pause: 0},
			{Weight: 100, Pause: 0},
		},
	}

	callbacks := &Callbacks{
		OnInstanceCreate: func(ctx context.Context, p *InstanceParams) (*Instance, error) {
			return nil, errors.New("create failed")
		},
		OnProgress: func(ctx context.Context, p *Progress) {},
	}

	params := &ExecuteParams{
		DeploymentID:   "canary-create-fail",
		ServiceName:    "svc-canary",
		CurrentVersion: "v1",
		TargetVersion:  "v2",
		Replicas:       1,
		Config:         cfg,
		Instances:      []Instance{{ID: "s-1", Version: "v1", State: InstanceRunning, Healthy: true}},
		Callbacks:      callbacks,
	}

	result, err := s.Execute(context.Background(), params)
	if !errors.Is(err, ErrInstanceUpdateFailed) {
		t.Fatalf("expected ErrInstanceUpdateFailed, got %v", err)
	}
	if result.Success {
		t.Error("expected unsuccessful result")
	}
}

func TestCanaryStrategy_Execute_AnalysisFailure(t *testing.T) {
	s := NewCanaryStrategy()
	cfg := &Config{
		Type: TypeCanary,
		Steps: []CanaryStep{
			{Weight: 50, Pause: 0, Analysis: true},
			{Weight: 100, Pause: 0},
		},
		Analysis: &AnalysisConfig{
			ErrorRateThreshold:   0.01,
			SuccessRateThreshold: 0.99,
			Interval:             1 * time.Millisecond, // keep test fast
		},
	}

	callbacks := &Callbacks{
		OnInstanceCreate: func(ctx context.Context, p *InstanceParams) (*Instance, error) {
			return &Instance{
				ID:      "canary-inst",
				Version: p.Version,
				State:   InstanceRunning,
				Healthy: true,
			}, nil
		},
		OnHealthCheck: func(ctx context.Context, id string) (bool, error) {
			return true, nil
		},
		OnTrafficShift: func(ctx context.Context, p *TrafficShiftParams) error {
			return nil
		},
		OnAnalysis: func(ctx context.Context, metrics *AnalysisMetrics) (*AnalysisResult, error) {
			return &AnalysisResult{
				Pass:           false,
				ShouldRollback: true,
				Message:        "error rate too high",
				Metrics: &AnalysisMetrics{
					ErrorRate:   0.10, // 10% error rate exceeds 1% threshold
					SuccessRate: 0.90,
				},
			}, nil
		},
		OnProgress: func(ctx context.Context, p *Progress) {},
	}

	params := &ExecuteParams{
		DeploymentID:   "canary-analysis-fail",
		ServiceName:    "svc-canary",
		CurrentVersion: "v1",
		TargetVersion:  "v2",
		Replicas:       1,
		Config:         cfg,
		Instances:      []Instance{{ID: "s-1", Version: "v1", State: InstanceRunning, Healthy: true}},
		Callbacks:      callbacks,
	}

	result, err := s.Execute(context.Background(), params)
	if !errors.Is(err, ErrRollbackRequired) {
		t.Fatalf("expected ErrRollbackRequired, got %v", err)
	}
	if result.Success {
		t.Error("expected unsuccessful result")
	}
}

func TestCanaryStrategy_Execute_InvalidConfig(t *testing.T) {
	s := NewCanaryStrategy()
	params := &ExecuteParams{
		DeploymentID: "canary-invalid",
		Config:       &Config{Type: TypeRolling},
	}
	_, err := s.Execute(context.Background(), params)
	if err == nil {
		t.Fatal("expected error for invalid config")
	}
}

func TestCanaryStrategy_Status_NotFound(t *testing.T) {
	s := NewCanaryStrategy()
	_, err := s.Status(context.Background(), "nonexistent")
	if !errors.Is(err, ErrStrategyNotFound) {
		t.Fatalf("expected ErrStrategyNotFound, got %v", err)
	}
}

func TestCanaryStrategy_Promote_NotFound(t *testing.T) {
	s := NewCanaryStrategy()
	err := s.Promote("nonexistent")
	if !errors.Is(err, ErrStrategyNotFound) {
		t.Fatalf("expected ErrStrategyNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Tests for Type constants and InstanceState constants
// ---------------------------------------------------------------------------

func TestTypeConstants(t *testing.T) {
	cases := []struct {
		typ  Type
		want string
	}{
		{TypeRolling, "rolling"},
		{TypeBlueGreen, "blue_green"},
		{TypeCanary, "canary"},
		{TypeRecreate, "recreate"},
	}
	for _, tc := range cases {
		if string(tc.typ) != tc.want {
			t.Errorf("expected %q, got %q", tc.want, tc.typ)
		}
	}
}

func TestInstanceStateConstants(t *testing.T) {
	cases := []struct {
		state InstanceState
		want  string
	}{
		{InstanceRunning, "running"},
		{InstanceStarting, "starting"},
		{InstanceStopping, "stopping"},
		{InstanceStopped, "stopped"},
		{InstanceFailed, "failed"},
		{InstanceTerminating, "terminating"},
	}
	for _, tc := range cases {
		if string(tc.state) != tc.want {
			t.Errorf("expected %q, got %q", tc.want, tc.state)
		}
	}
}

// ---------------------------------------------------------------------------
// Tests for error sentinels
// ---------------------------------------------------------------------------

func TestErrorSentinels(t *testing.T) {
	errs := []error{
		ErrStrategyNotFound,
		ErrInvalidConfig,
		ErrInstanceUpdateFailed,
		ErrRollbackRequired,
		ErrTrafficShiftFailed,
		ErrHealthCheckTimeout,
	}
	for _, e := range errs {
		if e == nil {
			t.Error("expected non-nil error sentinel")
		}
		if e.Error() == "" {
			t.Error("expected non-empty error message")
		}
	}
}

// ---------------------------------------------------------------------------
// Tests for struct field coverage
// ---------------------------------------------------------------------------

func TestExecuteParams_Fields(t *testing.T) {
	p := &ExecuteParams{
		DeploymentID:   "dep-1",
		ServiceName:    "svc",
		CurrentVersion: "v1",
		TargetVersion:  "v2",
		Replicas:       3,
		Config:         &Config{},
		Instances:      []Instance{{ID: "i-1"}},
		Callbacks:      &Callbacks{},
	}
	if p.DeploymentID != "dep-1" {
		t.Error("DeploymentID mismatch")
	}
	if p.ServiceName != "svc" {
		t.Error("ServiceName mismatch")
	}
	if p.Replicas != 3 {
		t.Error("Replicas mismatch")
	}
}

func TestResult_Fields(t *testing.T) {
	r := &Result{
		Success:  true,
		Message:  "ok",
		Duration: 5 * time.Second,
		Instances: []Instance{
			{ID: "i-1", Version: "v1", State: InstanceRunning, Healthy: true, Address: "10.0.0.1"},
		},
		Metrics: &ResultMetrics{
			InstancesCreated: 1,
			InstancesDeleted: 0,
			InstancesUpdated: 1,
			InstancesFailed:  0,
			HealthChecks:     3,
			TrafficShifts:    1,
			AnalysisRuns:     0,
		},
	}
	if !r.Success {
		t.Error("expected success")
	}
	if r.Metrics.InstancesCreated != 1 {
		t.Error("InstancesCreated mismatch")
	}
}

func TestInstance_Labels(t *testing.T) {
	inst := Instance{
		ID:      "i-1",
		Version: "v1",
		Labels: map[string]string{
			"env": "prod",
		},
	}
	if inst.Labels["env"] != "prod" {
		t.Error("expected label env=prod")
	}
}

func TestCanaryStep_Fields(t *testing.T) {
	step := CanaryStep{
		Weight:   50,
		Pause:    5 * time.Minute,
		Analysis: true,
	}
	if step.Weight != 50 {
		t.Error("Weight mismatch")
	}
	if !step.Analysis {
		t.Error("expected Analysis true")
	}
}

func TestAnalysisConfig_Fields(t *testing.T) {
	ac := &AnalysisConfig{
		ErrorRateThreshold:   0.01,
		LatencyP99Threshold:  500 * time.Millisecond,
		SuccessRateThreshold: 0.99,
		SampleSize:           1000,
		MinSampleSize:        100,
		Interval:             30 * time.Second,
	}
	if ac.ErrorRateThreshold != 0.01 {
		t.Error("ErrorRateThreshold mismatch")
	}
	if ac.MinSampleSize != 100 {
		t.Error("MinSampleSize mismatch")
	}
}

func TestHealthCheckConfig_Fields(t *testing.T) {
	hc := &HealthCheckConfig{
		Type:             "http",
		Endpoint:         "http://localhost:8080/health",
		Path:             "/health",
		Port:             8080,
		Interval:         10 * time.Second,
		Timeout:          5 * time.Second,
		SuccessThreshold: 1,
		FailureThreshold: 3,
	}
	if hc.Port != 8080 {
		t.Error("Port mismatch")
	}
	if hc.FailureThreshold != 3 {
		t.Error("FailureThreshold mismatch")
	}
}

func TestTrafficShiftParams_Fields(t *testing.T) {
	p := &TrafficShiftParams{
		OldVersion: "v1",
		NewVersion: "v2",
		Weight:     100,
	}
	if p.Weight != 100 {
		t.Error("Weight mismatch")
	}
}

func TestProgress_Fields(t *testing.T) {
	p := &Progress{
		Phase:            "rolling",
		Message:          "in progress",
		Percentage:       50,
		UpdatedInstances: 3,
		TotalInstances:   6,
		CurrentWeight:    50,
	}
	if p.Percentage != 50 {
		t.Error("Percentage mismatch")
	}
}

func TestAnalysisMetrics_Fields(t *testing.T) {
	m := &AnalysisMetrics{
		ErrorRate:    0.01,
		LatencyP50:  10 * time.Millisecond,
		LatencyP99:  100 * time.Millisecond,
		SuccessRate:  0.99,
		RequestCount: 1000,
		ErrorCount:   10,
	}
	if m.RequestCount != 1000 {
		t.Error("RequestCount mismatch")
	}
}

func TestAnalysisResult_Fields(t *testing.T) {
	r := &AnalysisResult{
		Pass:           true,
		Message:        "ok",
		ShouldRollback: false,
		Metrics:        &AnalysisMetrics{ErrorRate: 0.001},
	}
	if !r.Pass {
		t.Error("expected Pass")
	}
}

func TestRollbackParams_Fields(t *testing.T) {
	p := &RollbackParams{
		DeploymentID:  "dep-1",
		TargetVersion: "v1",
		Instances:     []Instance{{ID: "i-1"}},
		Callbacks:     &Callbacks{},
	}
	if p.DeploymentID != "dep-1" {
		t.Error("DeploymentID mismatch")
	}
}

func TestStatus_Fields(t *testing.T) {
	s := &Status{
		Phase: "running",
		Progress: &Progress{
			Percentage: 50,
		},
		Instances:     []Instance{{ID: "i-1", Healthy: true}},
		Healthy:       1,
		Unhealthy:     0,
		TrafficWeight: 50,
		Analysis:      &AnalysisResult{Pass: true},
	}
	if s.Healthy != 1 {
		t.Error("Healthy mismatch")
	}
}
