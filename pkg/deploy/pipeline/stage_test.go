// Package pipeline provides the deployment pipeline for orchestrating deployment stages.
package pipeline

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// Note: TestStageTypeConstants and TestFailureActionConstants are defined in pipeline_test.go

// TestNewStageExecutor verifies StageExecutor creation.
func TestNewStageExecutor(t *testing.T) {
	executor := NewStageExecutor()

	if executor == nil {
		t.Fatal("expected non-nil stage executor")
	}

	if executor.handlers == nil {
		t.Error("expected handlers map to be initialized")
	}

	if executor.validators == nil {
		t.Error("expected validators map to be initialized")
	}

	// Verify default handlers are registered
	expectedHandlers := []StageType{StageBuild, StageValidate, StageDeploy, StageVerify, StagePromote, StageCleanup}
	for _, st := range expectedHandlers {
		if _, ok := executor.handlers[st]; !ok {
			t.Errorf("expected handler for stage type %q to be registered", st)
		}
	}

	// Verify default validators are registered
	for _, st := range expectedHandlers {
		if _, ok := executor.validators[st]; !ok {
			t.Errorf("expected validator for stage type %q to be registered", st)
		}
	}
}

// TestStageExecutorRegisterHandler verifies handler registration.
func TestStageExecutorRegisterHandler(t *testing.T) {
	executor := NewStageExecutor()

	customCalled := false
	customHandler := func(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error {
		customCalled = true
		return nil
	}

	executor.RegisterHandler(StageCustom, customHandler)

	// Verify handler is registered
	if _, ok := executor.handlers[StageCustom]; !ok {
		t.Error("expected custom handler to be registered")
	}

	// Execute to verify it works
	stage := &Stage{Name: "custom-stage", Type: StageCustom}
	_, err := executor.Execute(context.Background(), stage, nil)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !customCalled {
		t.Error("expected custom handler to be called")
	}
}

// TestStageExecutorRegisterValidator verifies validator registration.
func TestStageExecutorRegisterValidator(t *testing.T) {
	executor := NewStageExecutor()

	validatorCalled := false
	customValidator := func(stage *Stage) error {
		validatorCalled = true
		return nil
	}

	executor.RegisterValidator(StageCustom, customValidator)

	// Verify validator is registered
	if _, ok := executor.validators[StageCustom]; !ok {
		t.Error("expected custom validator to be registered")
	}

	// Register a handler so execution can proceed
	executor.RegisterHandler(StageCustom, func(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error {
		return nil
	})

	// Execute to verify validator is called
	stage := &Stage{Name: "custom-stage", Type: StageCustom}
	_, err := executor.Execute(context.Background(), stage, nil)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !validatorCalled {
		t.Error("expected custom validator to be called")
	}
}

// TestStageExecutorValidate verifies stage validation.
func TestStageExecutorValidate(t *testing.T) {
	executor := NewStageExecutor()

	tests := []struct {
		name        string
		stage       *Stage
		expectError bool
		errorContains string
	}{
		{
			name:        "nil stage",
			stage:       nil,
			expectError: true,
		},
		{
			name:        "empty name",
			stage:       &Stage{Name: "", Type: StageBuild},
			expectError: true,
			errorContains: "stage name is required",
		},
		{
			name:        "empty type",
			stage:       &Stage{Name: "test-stage", Type: ""},
			expectError: true,
			errorContains: "stage type is required",
		},
		{
			name:        "valid build stage",
			stage:       &Stage{Name: "build-stage", Type: StageBuild},
			expectError: false,
		},
		{
			name:        "valid validate stage",
			stage:       &Stage{Name: "validate-stage", Type: StageValidate},
			expectError: false,
		},
		{
			name:        "valid deploy stage",
			stage:       &Stage{Name: "deploy-stage", Type: StageDeploy},
			expectError: false,
		},
		{
			name:        "valid verify stage",
			stage:       &Stage{Name: "verify-stage", Type: StageVerify},
			expectError: false,
		},
		{
			name:        "valid promote stage",
			stage:       &Stage{Name: "promote-stage", Type: StagePromote},
			expectError: false,
		},
		{
			name:        "valid cleanup stage",
			stage:       &Stage{Name: "cleanup-stage", Type: StageCleanup},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := executor.Validate(tt.stage)

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.errorContains != "" && err != nil {
				if !errors.Is(err, ErrInvalidStage) {
					t.Errorf("expected error to wrap ErrInvalidStage")
				}
			}
		})
	}
}

// TestStageExecutorValidateWithCustomValidator verifies custom validator execution.
func TestStageExecutorValidateWithCustomValidator(t *testing.T) {
	executor := NewStageExecutor()

	expectedErr := errors.New("custom validation failed")
	executor.RegisterValidator(StageCustom, func(stage *Stage) error {
		return expectedErr
	})

	stage := &Stage{Name: "custom-stage", Type: StageCustom}
	err := executor.Validate(stage)

	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

// TestStageExecutorExecute verifies stage execution.
func TestStageExecutorExecute(t *testing.T) {
	executor := NewStageExecutor()

	tests := []struct {
		name           string
		stage          *Stage
		pipelineCtx    map[string]interface{}
		expectError    bool
		expectSuccess  bool
	}{
		{
			name:          "successful build stage",
			stage:         &Stage{Name: "build", Type: StageBuild},
			pipelineCtx:   map[string]interface{}{"service_name": "test"},
			expectError:   false,
			expectSuccess: true,
		},
		{
			name:          "successful validate stage",
			stage:         &Stage{Name: "validate", Type: StageValidate},
			pipelineCtx:   map[string]interface{}{"service_name": "test", "version": "1.0"},
			expectError:   false,
			expectSuccess: true,
		},
		{
			name:          "successful deploy stage",
			stage:         &Stage{Name: "deploy", Type: StageDeploy},
			pipelineCtx:   map[string]interface{}{},
			expectError:   false,
			expectSuccess: true,
		},
		{
			name:          "successful verify stage",
			stage:         &Stage{Name: "verify", Type: StageVerify},
			pipelineCtx:   map[string]interface{}{},
			expectError:   false,
			expectSuccess: true,
		},
		{
			name:          "successful promote stage",
			stage:         &Stage{Name: "promote", Type: StagePromote},
			pipelineCtx:   map[string]interface{}{},
			expectError:   false,
			expectSuccess: true,
		},
		{
			name:          "successful cleanup stage",
			stage:         &Stage{Name: "cleanup", Type: StageCleanup},
			pipelineCtx:   map[string]interface{}{},
			expectError:   false,
			expectSuccess: true,
		},
		{
			name:          "invalid stage",
			stage:         &Stage{Name: "", Type: StageBuild},
			pipelineCtx:   map[string]interface{}{},
			expectError:   true,
			expectSuccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := executor.Execute(context.Background(), tt.stage, tt.pipelineCtx)

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("expected non-nil result")
			}

			if result.Success != tt.expectSuccess {
				t.Errorf("expected success=%v, got %v", tt.expectSuccess, result.Success)
			}

			// Verify timestamps are set
			if result.StartedAt.IsZero() {
				t.Error("expected StartedAt to be set")
			}
			if result.CompletedAt.IsZero() {
				t.Error("expected CompletedAt to be set")
			}
			if result.Duration < 0 {
				t.Error("expected non-negative duration")
			}
		})
	}
}

// TestStageExecutorExecuteWithTimeout verifies stage timeout handling.
func TestStageExecutorExecuteWithTimeout(t *testing.T) {
	executor := NewStageExecutor()

	// Register a slow handler
	executor.RegisterHandler(StageCustom, func(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return nil
		}
	})

	stage := &Stage{
		Name:    "slow-stage",
		Type:    StageCustom,
		Timeout: 100 * time.Millisecond,
	}

	result, err := executor.Execute(context.Background(), stage, nil)

	if err == nil {
		t.Error("expected timeout error")
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded error, got: %v", err)
	}

	if result.Success {
		t.Error("expected result to be unsuccessful")
	}
}

// TestStageExecutorExecuteWithRetries verifies stage retry logic.
func TestStageExecutorExecuteWithRetries(t *testing.T) {
	executor := NewStageExecutor()

	attemptCount := 0
	executor.RegisterHandler(StageCustom, func(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error {
		attemptCount++
		if attemptCount < 3 {
			return errors.New("temporary failure")
		}
		return nil
	})

	stage := &Stage{
		Name:    "retry-stage",
		Type:    StageCustom,
		Retries: 3, // 4 total attempts
	}

	result, err := executor.Execute(context.Background(), stage, nil)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Error("expected result to be successful after retries")
	}

	if attemptCount != 3 {
		t.Errorf("expected 3 attempts, got %d", attemptCount)
	}

	if result.Retries != 2 { // 0-indexed
		t.Errorf("expected retries count 2, got %d", result.Retries)
	}
}

// TestStageExecutorExecuteAllRetriesFail verifies behavior when all retries fail.
func TestStageExecutorExecuteAllRetriesFail(t *testing.T) {
	executor := NewStageExecutor()

	attemptCount := 0
	executor.RegisterHandler(StageCustom, func(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error {
		attemptCount++
		return errors.New("persistent failure")
	})

	stage := &Stage{
		Name:    "fail-stage",
		Type:    StageCustom,
		Retries: 2, // 3 total attempts
	}

	result, err := executor.Execute(context.Background(), stage, nil)

	if err == nil {
		t.Error("expected error after all retries fail")
	}

	if result.Success {
		t.Error("expected result to be unsuccessful")
	}

	if attemptCount != 3 {
		t.Errorf("expected 3 attempts, got %d", attemptCount)
	}
}

// TestStageExecutorExecuteNoHandler verifies error when no handler exists.
func TestStageExecutorExecuteNoHandler(t *testing.T) {
	executor := &StageExecutor{
		handlers:   make(map[StageType]StageHandler),
		validators: make(map[StageType]StageValidator),
	}

	stage := &Stage{Name: "orphan-stage", Type: StageCustom}
	result, err := executor.Execute(context.Background(), stage, nil)

	if err == nil {
		t.Error("expected error for missing handler")
	}

	if result.Success {
		t.Error("expected result to be unsuccessful")
	}

	expectedErrMsg := "no handler for stage type: custom"
	if result.Error != expectedErrMsg {
		t.Errorf("expected error message %q, got %q", expectedErrMsg, result.Error)
	}
}

// TestStageExecutorExecuteWithConfig verifies stage execution with config.
func TestStageExecutorExecuteWithConfig(t *testing.T) {
	executor := NewStageExecutor()

	stage := &Stage{
		Name: "build-with-config",
		Type: StageBuild,
		Config: &StageConfig{
			BuildImage:    "golang:1.21",
			BuildCommands: []string{"go build", "go test"},
		},
	}

	result, err := executor.Execute(context.Background(), stage, nil)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Error("expected result to be successful")
	}

	// Verify output contains build info
	if stage.Output["build_image"] != "golang:1.21" {
		t.Errorf("expected build_image in output")
	}
}

// TestStageExecutorExecuteDeployStrategy verifies deploy stage uses strategy.
func TestStageExecutorExecuteDeployStrategy(t *testing.T) {
	executor := NewStageExecutor()

	stage := &Stage{
		Name: "deploy-canary",
		Type: StageDeploy,
		Config: &StageConfig{
			DeployStrategy: "canary",
		},
	}

	result, err := executor.Execute(context.Background(), stage, nil)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Error("expected result to be successful")
	}

	if stage.Output["strategy"] != "canary" {
		t.Errorf("expected strategy 'canary' in output, got %v", stage.Output["strategy"])
	}
}

// TestStageExecutorExecuteVerifyEndpoint verifies verify stage uses endpoint.
func TestStageExecutorExecuteVerifyEndpoint(t *testing.T) {
	executor := NewStageExecutor()

	stage := &Stage{
		Name: "verify-health",
		Type: StageVerify,
		Config: &StageConfig{
			VerifyEndpoint: "/api/health",
		},
	}

	result, err := executor.Execute(context.Background(), stage, nil)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Error("expected result to be successful")
	}

	if stage.Output["endpoint"] != "/api/health" {
		t.Errorf("expected endpoint '/api/health' in output, got %v", stage.Output["endpoint"])
	}
}

// TestStageExecutorContextCancellation verifies context cancellation handling.
func TestStageExecutorContextCancellation(t *testing.T) {
	executor := NewStageExecutor()

	// Register a handler that blocks
	executor.RegisterHandler(StageCustom, func(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
			return nil
		}
	})

	ctx, cancel := context.WithCancel(context.Background())

	stage := &Stage{Name: "cancellable-stage", Type: StageCustom}

	// Cancel after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	result, err := executor.Execute(ctx, stage, nil)

	if err == nil {
		t.Error("expected cancellation error")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got: %v", err)
	}

	if result.Success {
		t.Error("expected result to be unsuccessful")
	}
}

// TestStageResult verifies StageResult structure.
func TestStageResult(t *testing.T) {
	stage := &Stage{Name: "test-stage", Type: StageBuild}
	startedAt := time.Now()
	completedAt := startedAt.Add(100 * time.Millisecond)

	result := &StageResult{
		Success:     true,
		Stage:       stage,
		Output:      map[string]interface{}{"key": "value"},
		StartedAt:   startedAt,
		CompletedAt: completedAt,
		Duration:    100 * time.Millisecond,
		Retries:     2,
	}

	if !result.Success {
		t.Error("expected Success to be true")
	}

	if result.Stage.Name != "test-stage" {
		t.Errorf("expected stage name 'test-stage', got %q", result.Stage.Name)
	}

	if result.Output["key"] != "value" {
		t.Errorf("expected output key 'value', got %v", result.Output["key"])
	}

	if result.Duration != 100*time.Millisecond {
		t.Errorf("expected duration 100ms, got %v", result.Duration)
	}

	if result.Retries != 2 {
		t.Errorf("expected retries 2, got %d", result.Retries)
	}
}

// TestStageResultWithError verifies StageResult with error.
func TestStageResultWithError(t *testing.T) {
	result := &StageResult{
		Success: false,
		Error:   "stage execution failed",
	}

	if result.Success {
		t.Error("expected Success to be false")
	}

	if result.Error != "stage execution failed" {
		t.Errorf("expected error message, got %q", result.Error)
	}
}

// TestNewStageBuilder verifies StageBuilder creation.
func TestNewStageBuilder(t *testing.T) {
	builder := NewStageBuilder("test-stage", StageDeploy)

	if builder == nil {
		t.Fatal("expected non-nil builder")
	}

	if builder.stage.Name != "test-stage" {
		t.Errorf("expected stage name 'test-stage', got %q", builder.stage.Name)
	}

	if builder.stage.Type != StageDeploy {
		t.Errorf("expected stage type 'deploy', got %q", builder.stage.Type)
	}

	if builder.stage.State != StatePending {
		t.Errorf("expected state 'pending', got %q", builder.stage.State)
	}

	if builder.stage.OnFailure != FailureAbort {
		t.Errorf("expected OnFailure 'abort', got %q", builder.stage.OnFailure)
	}
}

// TestStageBuilderWithConfig verifies WithConfig method.
func TestStageBuilderWithConfig(t *testing.T) {
	config := &StageConfig{
		BuildImage:    "golang:1.21",
		BuildCommands: []string{"go build"},
	}

	stage := NewStageBuilder("build-stage", StageBuild).
		WithConfig(config).
		Build()

	if stage.Config == nil {
		t.Fatal("expected config to be set")
	}

	if stage.Config.BuildImage != "golang:1.21" {
		t.Errorf("expected build image 'golang:1.21', got %q", stage.Config.BuildImage)
	}
}

// TestStageBuilderWithTimeout verifies WithTimeout method.
func TestStageBuilderWithTimeout(t *testing.T) {
	stage := NewStageBuilder("timed-stage", StageBuild).
		WithTimeout(5 * time.Minute).
		Build()

	if stage.Timeout != 5*time.Minute {
		t.Errorf("expected timeout 5m, got %v", stage.Timeout)
	}
}

// TestStageBuilderWithRetries verifies WithRetries method.
func TestStageBuilderWithRetries(t *testing.T) {
	stage := NewStageBuilder("retry-stage", StageDeploy).
		WithRetries(3).
		Build()

	if stage.Retries != 3 {
		t.Errorf("expected retries 3, got %d", stage.Retries)
	}
}

// TestStageBuilderWithOnFailure verifies WithOnFailure method.
func TestStageBuilderWithOnFailure(t *testing.T) {
	stage := NewStageBuilder("fail-stage", StageDeploy).
		WithOnFailure(FailureRollback).
		Build()

	if stage.OnFailure != FailureRollback {
		t.Errorf("expected OnFailure 'rollback', got %q", stage.OnFailure)
	}
}

// TestStageBuilderWithPreHook verifies WithPreHook method.
func TestStageBuilderWithPreHook(t *testing.T) {
	hook := &Hook{
		Name: "pre-deploy",
		Type: HookTypeScript,
		Config: &HookConfig{
			Script: "echo 'Starting deploy'",
		},
	}

	stage := NewStageBuilder("deploy-stage", StageDeploy).
		WithPreHook(hook).
		Build()

	if len(stage.PreHooks) != 1 {
		t.Errorf("expected 1 pre-hook, got %d", len(stage.PreHooks))
	}

	if stage.PreHooks[0].Name != "pre-deploy" {
		t.Errorf("expected pre-hook name 'pre-deploy', got %q", stage.PreHooks[0].Name)
	}
}

// TestStageBuilderWithPostHook verifies WithPostHook method.
func TestStageBuilderWithPostHook(t *testing.T) {
	hook := &Hook{
		Name: "post-deploy",
		Type: HookTypeWebhook,
		Config: &HookConfig{
			URL: "http://notify.example.com",
		},
	}

	stage := NewStageBuilder("deploy-stage", StageDeploy).
		WithPostHook(hook).
		Build()

	if len(stage.PostHooks) != 1 {
		t.Errorf("expected 1 post-hook, got %d", len(stage.PostHooks))
	}

	if stage.PostHooks[0].Name != "post-deploy" {
		t.Errorf("expected post-hook name 'post-deploy', got %q", stage.PostHooks[0].Name)
	}
}

// TestStageBuilderChaining verifies builder method chaining.
func TestStageBuilderChaining(t *testing.T) {
	config := &StageConfig{DeployStrategy: "canary"}
	preHook := &Hook{Name: "pre-hook", Type: HookTypeScript}
	postHook := &Hook{Name: "post-hook", Type: HookTypeWebhook}

	stage := NewStageBuilder("complex-stage", StageDeploy).
		WithConfig(config).
		WithTimeout(10 * time.Minute).
		WithRetries(2).
		WithOnFailure(FailureRollback).
		WithPreHook(preHook).
		WithPostHook(postHook).
		Build()

	if stage.Name != "complex-stage" {
		t.Errorf("expected name 'complex-stage', got %q", stage.Name)
	}

	if stage.Type != StageDeploy {
		t.Errorf("expected type 'deploy', got %q", stage.Type)
	}

	if stage.Config.DeployStrategy != "canary" {
		t.Errorf("expected strategy 'canary', got %q", stage.Config.DeployStrategy)
	}

	if stage.Timeout != 10*time.Minute {
		t.Errorf("expected timeout 10m, got %v", stage.Timeout)
	}

	if stage.Retries != 2 {
		t.Errorf("expected retries 2, got %d", stage.Retries)
	}

	if stage.OnFailure != FailureRollback {
		t.Errorf("expected OnFailure 'rollback', got %q", stage.OnFailure)
	}

	if len(stage.PreHooks) != 1 {
		t.Errorf("expected 1 pre-hook, got %d", len(stage.PreHooks))
	}

	if len(stage.PostHooks) != 1 {
		t.Errorf("expected 1 post-hook, got %d", len(stage.PostHooks))
	}
}

// TestBuildStageHelper verifies BuildStage helper function.
func TestBuildStageHelper(t *testing.T) {
	stage := BuildStage("my-build", "golang:1.21", []string{"go build", "go test"})

	if stage.Name != "my-build" {
		t.Errorf("expected name 'my-build', got %q", stage.Name)
	}

	if stage.Type != StageBuild {
		t.Errorf("expected type 'build', got %q", stage.Type)
	}

	if stage.Config.BuildImage != "golang:1.21" {
		t.Errorf("expected build image 'golang:1.21', got %q", stage.Config.BuildImage)
	}

	if len(stage.Config.BuildCommands) != 2 {
		t.Errorf("expected 2 build commands, got %d", len(stage.Config.BuildCommands))
	}

	if stage.Timeout != 30*time.Minute {
		t.Errorf("expected timeout 30m, got %v", stage.Timeout)
	}
}

// TestValidateStageHelper verifies ValidateStage helper function.
func TestValidateStageHelper(t *testing.T) {
	stage := ValidateStage("my-validate", []string{"rule1", "rule2", "rule3"})

	if stage.Name != "my-validate" {
		t.Errorf("expected name 'my-validate', got %q", stage.Name)
	}

	if stage.Type != StageValidate {
		t.Errorf("expected type 'validate', got %q", stage.Type)
	}

	if len(stage.Config.ValidateRules) != 3 {
		t.Errorf("expected 3 validate rules, got %d", len(stage.Config.ValidateRules))
	}

	if stage.Timeout != 5*time.Minute {
		t.Errorf("expected timeout 5m, got %v", stage.Timeout)
	}
}

// TestDeployStageHelper verifies DeployStage helper function.
func TestDeployStageHelper(t *testing.T) {
	params := map[string]interface{}{
		"replicas": 3,
		"region":   "us-east-1",
	}

	stage := DeployStage("my-deploy", "blue-green", params)

	if stage.Name != "my-deploy" {
		t.Errorf("expected name 'my-deploy', got %q", stage.Name)
	}

	if stage.Type != StageDeploy {
		t.Errorf("expected type 'deploy', got %q", stage.Type)
	}

	if stage.Config.DeployStrategy != "blue-green" {
		t.Errorf("expected strategy 'blue-green', got %q", stage.Config.DeployStrategy)
	}

	if stage.Config.DeployParams["replicas"] != 3 {
		t.Errorf("expected replicas 3, got %v", stage.Config.DeployParams["replicas"])
	}

	if stage.Timeout != 30*time.Minute {
		t.Errorf("expected timeout 30m, got %v", stage.Timeout)
	}

	if stage.Retries != 2 {
		t.Errorf("expected retries 2, got %d", stage.Retries)
	}

	if stage.OnFailure != FailureRollback {
		t.Errorf("expected OnFailure 'rollback', got %q", stage.OnFailure)
	}
}

// TestVerifyStageHelper verifies VerifyStage helper function.
func TestVerifyStageHelper(t *testing.T) {
	stage := VerifyStage("my-verify", "/api/health", 2*time.Minute)

	if stage.Name != "my-verify" {
		t.Errorf("expected name 'my-verify', got %q", stage.Name)
	}

	if stage.Type != StageVerify {
		t.Errorf("expected type 'verify', got %q", stage.Type)
	}

	if stage.Config.VerifyEndpoint != "/api/health" {
		t.Errorf("expected endpoint '/api/health', got %q", stage.Config.VerifyEndpoint)
	}

	if stage.Config.VerifyTimeout != 2*time.Minute {
		t.Errorf("expected verify timeout 2m, got %v", stage.Config.VerifyTimeout)
	}

	if stage.Timeout != 2*time.Minute {
		t.Errorf("expected stage timeout 2m, got %v", stage.Timeout)
	}

	if stage.Retries != 3 {
		t.Errorf("expected retries 3, got %d", stage.Retries)
	}

	if stage.OnFailure != FailureRollback {
		t.Errorf("expected OnFailure 'rollback', got %q", stage.OnFailure)
	}
}

// TestPromoteStageHelper verifies PromoteStage helper function.
func TestPromoteStageHelper(t *testing.T) {
	stage := PromoteStage("my-promote")

	if stage.Name != "my-promote" {
		t.Errorf("expected name 'my-promote', got %q", stage.Name)
	}

	if stage.Type != StagePromote {
		t.Errorf("expected type 'promote', got %q", stage.Type)
	}

	if stage.Timeout != 5*time.Minute {
		t.Errorf("expected timeout 5m, got %v", stage.Timeout)
	}

	if stage.OnFailure != FailureRollback {
		t.Errorf("expected OnFailure 'rollback', got %q", stage.OnFailure)
	}
}

// TestCleanupStageHelper verifies CleanupStage helper function.
func TestCleanupStageHelper(t *testing.T) {
	stage := CleanupStage("my-cleanup")

	if stage.Name != "my-cleanup" {
		t.Errorf("expected name 'my-cleanup', got %q", stage.Name)
	}

	if stage.Type != StageCleanup {
		t.Errorf("expected type 'cleanup', got %q", stage.Type)
	}

	if stage.Timeout != 10*time.Minute {
		t.Errorf("expected timeout 10m, got %v", stage.Timeout)
	}

	if stage.OnFailure != FailureContinue {
		t.Errorf("expected OnFailure 'continue', got %q", stage.OnFailure)
	}
}

// TestDefaultBuildHandler verifies default build handler.
func TestDefaultBuildHandler(t *testing.T) {
	t.Run("without config", func(t *testing.T) {
		stage := &Stage{Name: "build", Type: StageBuild}
		err := defaultBuildHandler(context.Background(), stage, nil)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if stage.Output["message"] != "No build configuration, skipping build" {
			t.Errorf("expected skip message, got %v", stage.Output)
		}
	})

	t.Run("with config", func(t *testing.T) {
		stage := &Stage{
			Name: "build",
			Type: StageBuild,
			Config: &StageConfig{
				BuildImage: "node:18",
			},
		}
		err := defaultBuildHandler(context.Background(), stage, nil)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if stage.Output["build_image"] != "node:18" {
			t.Errorf("expected build_image 'node:18', got %v", stage.Output["build_image"])
		}

		if stage.Output["build_status"] != "success" {
			t.Errorf("expected build_status 'success', got %v", stage.Output["build_status"])
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		stage := &Stage{
			Name:   "build",
			Type:   StageBuild,
			Config: &StageConfig{BuildImage: "test"},
		}

		err := defaultBuildHandler(ctx, stage, nil)

		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	})
}

// TestDefaultValidateHandler verifies default validate handler.
func TestDefaultValidateHandler(t *testing.T) {
	t.Run("with required keys present", func(t *testing.T) {
		stage := &Stage{Name: "validate", Type: StageValidate}
		pipelineCtx := map[string]interface{}{
			"service_name": "my-service",
			"version":      "1.0.0",
		}

		err := defaultValidateHandler(context.Background(), stage, pipelineCtx)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		results := stage.Output["validation_results"].(map[string]string)
		if results["service_name"] != "present" {
			t.Errorf("expected service_name to be present")
		}
		if results["version"] != "present" {
			t.Errorf("expected version to be present")
		}
	})

	t.Run("with required keys missing", func(t *testing.T) {
		stage := &Stage{Name: "validate", Type: StageValidate}
		pipelineCtx := map[string]interface{}{}

		err := defaultValidateHandler(context.Background(), stage, pipelineCtx)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		results := stage.Output["validation_results"].(map[string]string)
		if results["service_name"] != "missing" {
			t.Errorf("expected service_name to be missing")
		}
	})

	t.Run("with validation rules", func(t *testing.T) {
		stage := &Stage{
			Name: "validate",
			Type: StageValidate,
			Config: &StageConfig{
				ValidateRules: []string{"check-health", "check-deps"},
			},
		}

		err := defaultValidateHandler(context.Background(), stage, nil)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		results := stage.Output["validation_results"].(map[string]string)
		if results["check-health"] != "passed" {
			t.Errorf("expected check-health to pass")
		}
		if results["check-deps"] != "passed" {
			t.Errorf("expected check-deps to pass")
		}
	})
}

// TestDefaultDeployHandler verifies default deploy handler.
func TestDefaultDeployHandler(t *testing.T) {
	t.Run("default strategy", func(t *testing.T) {
		stage := &Stage{Name: "deploy", Type: StageDeploy}
		err := defaultDeployHandler(context.Background(), stage, nil)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if stage.Output["strategy"] != "rolling" {
			t.Errorf("expected default strategy 'rolling', got %v", stage.Output["strategy"])
		}

		if stage.Output["deploy_status"] != "deployed" {
			t.Errorf("expected deploy_status 'deployed', got %v", stage.Output["deploy_status"])
		}
	})

	t.Run("custom strategy", func(t *testing.T) {
		stage := &Stage{
			Name: "deploy",
			Type: StageDeploy,
			Config: &StageConfig{
				DeployStrategy: "canary",
			},
		}

		err := defaultDeployHandler(context.Background(), stage, nil)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if stage.Output["strategy"] != "canary" {
			t.Errorf("expected strategy 'canary', got %v", stage.Output["strategy"])
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		stage := &Stage{Name: "deploy", Type: StageDeploy}
		err := defaultDeployHandler(ctx, stage, nil)

		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	})
}

// TestDefaultVerifyHandler verifies default verify handler.
func TestDefaultVerifyHandler(t *testing.T) {
	t.Run("default endpoint", func(t *testing.T) {
		stage := &Stage{Name: "verify", Type: StageVerify}
		err := defaultVerifyHandler(context.Background(), stage, nil)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if stage.Output["endpoint"] != "/health" {
			t.Errorf("expected default endpoint '/health', got %v", stage.Output["endpoint"])
		}

		if stage.Output["verify_status"] != "healthy" {
			t.Errorf("expected verify_status 'healthy', got %v", stage.Output["verify_status"])
		}
	})

	t.Run("custom endpoint", func(t *testing.T) {
		stage := &Stage{
			Name: "verify",
			Type: StageVerify,
			Config: &StageConfig{
				VerifyEndpoint: "/api/ready",
			},
		}

		err := defaultVerifyHandler(context.Background(), stage, nil)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if stage.Output["endpoint"] != "/api/ready" {
			t.Errorf("expected endpoint '/api/ready', got %v", stage.Output["endpoint"])
		}
	})
}

// TestDefaultPromoteHandler verifies default promote handler.
func TestDefaultPromoteHandler(t *testing.T) {
	stage := &Stage{Name: "promote", Type: StagePromote}
	err := defaultPromoteHandler(context.Background(), stage, nil)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if stage.Output["promote_status"] != "promoted" {
		t.Errorf("expected promote_status 'promoted', got %v", stage.Output["promote_status"])
	}

	if stage.Output["traffic_weight"] != 100 {
		t.Errorf("expected traffic_weight 100, got %v", stage.Output["traffic_weight"])
	}
}

// TestDefaultCleanupHandler verifies default cleanup handler.
func TestDefaultCleanupHandler(t *testing.T) {
	stage := &Stage{Name: "cleanup", Type: StageCleanup}
	err := defaultCleanupHandler(context.Background(), stage, nil)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if stage.Output["cleanup_status"] != "completed" {
		t.Errorf("expected cleanup_status 'completed', got %v", stage.Output["cleanup_status"])
	}

	if stage.Output["resources_freed"] != 3 {
		t.Errorf("expected resources_freed 3, got %v", stage.Output["resources_freed"])
	}
}

// TestStageValidators verifies default stage validators.
func TestStageValidators(t *testing.T) {
	t.Run("build validator", func(t *testing.T) {
		err := validateBuildStage(&Stage{Name: "build", Type: StageBuild})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("validate validator", func(t *testing.T) {
		err := validateValidateStage(&Stage{Name: "validate", Type: StageValidate})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("deploy validator sets default timeout", func(t *testing.T) {
		stage := &Stage{Name: "deploy", Type: StageDeploy}
		err := validateDeployStage(stage)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if stage.Timeout != 30*time.Minute {
			t.Errorf("expected default timeout 30m, got %v", stage.Timeout)
		}
	})

	t.Run("deploy validator preserves custom timeout", func(t *testing.T) {
		stage := &Stage{Name: "deploy", Type: StageDeploy, Timeout: 1 * time.Hour}
		err := validateDeployStage(stage)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if stage.Timeout != 1*time.Hour {
			t.Errorf("expected timeout 1h, got %v", stage.Timeout)
		}
	})

	t.Run("verify validator sets default timeout", func(t *testing.T) {
		stage := &Stage{Name: "verify", Type: StageVerify}
		err := validateVerifyStage(stage)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if stage.Timeout != 10*time.Minute {
			t.Errorf("expected default timeout 10m, got %v", stage.Timeout)
		}
	})

	t.Run("promote validator sets default timeout", func(t *testing.T) {
		stage := &Stage{Name: "promote", Type: StagePromote}
		err := validatePromoteStage(stage)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if stage.Timeout != 5*time.Minute {
			t.Errorf("expected default timeout 5m, got %v", stage.Timeout)
		}
	})

	t.Run("cleanup validator", func(t *testing.T) {
		err := validateCleanupStage(&Stage{Name: "cleanup", Type: StageCleanup})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

// TestStageConfig verifies StageConfig structure.
func TestStageConfig(t *testing.T) {
	config := &StageConfig{
		BuildImage:     "golang:1.21",
		BuildCommands:  []string{"go build", "go test"},
		ValidateRules:  []string{"lint", "security"},
		DeployStrategy: "canary",
		DeployParams:   map[string]interface{}{"replicas": 3},
		VerifyEndpoint: "/health",
		VerifyTimeout:  1 * time.Minute,
		CustomHandler:  "my-handler",
		CustomParams:   map[string]interface{}{"key": "value"},
	}

	if config.BuildImage != "golang:1.21" {
		t.Errorf("expected BuildImage 'golang:1.21', got %q", config.BuildImage)
	}

	if len(config.BuildCommands) != 2 {
		t.Errorf("expected 2 BuildCommands, got %d", len(config.BuildCommands))
	}

	if len(config.ValidateRules) != 2 {
		t.Errorf("expected 2 ValidateRules, got %d", len(config.ValidateRules))
	}

	if config.DeployStrategy != "canary" {
		t.Errorf("expected DeployStrategy 'canary', got %q", config.DeployStrategy)
	}

	if config.DeployParams["replicas"] != 3 {
		t.Errorf("expected DeployParams replicas 3, got %v", config.DeployParams["replicas"])
	}

	if config.VerifyEndpoint != "/health" {
		t.Errorf("expected VerifyEndpoint '/health', got %q", config.VerifyEndpoint)
	}

	if config.VerifyTimeout != 1*time.Minute {
		t.Errorf("expected VerifyTimeout 1m, got %v", config.VerifyTimeout)
	}

	if config.CustomHandler != "my-handler" {
		t.Errorf("expected CustomHandler 'my-handler', got %q", config.CustomHandler)
	}
}

// TestConcurrentStageExecution verifies thread-safe stage execution.
func TestConcurrentStageExecution(t *testing.T) {
	executor := NewStageExecutor()

	var wg sync.WaitGroup
	errChan := make(chan error, 20)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			stage := &Stage{
				Name: "concurrent-stage",
				Type: StageBuild,
			}

			result, err := executor.Execute(context.Background(), stage, nil)
			if err != nil {
				errChan <- err
				return
			}

			if !result.Success {
				errChan <- errors.New("stage execution failed")
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		t.Errorf("concurrent execution error: %v", err)
	}
}

// TestStageOutputPreserved verifies stage output is preserved in result.
func TestStageOutputPreserved(t *testing.T) {
	executor := NewStageExecutor()

	executor.RegisterHandler(StageCustom, func(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error {
		stage.Output = map[string]interface{}{
			"custom_key": "custom_value",
			"count":      42,
		}
		return nil
	})

	stage := &Stage{Name: "output-stage", Type: StageCustom}
	result, err := executor.Execute(context.Background(), stage, nil)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if result.Output["custom_key"] != "custom_value" {
		t.Errorf("expected custom_key 'custom_value', got %v", result.Output["custom_key"])
	}

	if result.Output["count"] != 42 {
		t.Errorf("expected count 42, got %v", result.Output["count"])
	}
}

// BenchmarkStageExecution benchmarks stage execution.
func BenchmarkStageExecution(b *testing.B) {
	executor := NewStageExecutor()

	executor.RegisterHandler(StageCustom, func(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error {
		return nil
	})

	stage := &Stage{Name: "bench-stage", Type: StageCustom}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		executor.Execute(context.Background(), stage, nil)
	}
}

// BenchmarkStageValidation benchmarks stage validation.
func BenchmarkStageValidation(b *testing.B) {
	executor := NewStageExecutor()
	stage := &Stage{Name: "bench-stage", Type: StageBuild}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		executor.Validate(stage)
	}
}

// BenchmarkStageBuilder benchmarks stage builder.
func BenchmarkStageBuilder(b *testing.B) {
	config := &StageConfig{DeployStrategy: "canary"}
	hook := &Hook{Name: "hook", Type: HookTypeScript}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewStageBuilder("bench-stage", StageDeploy).
			WithConfig(config).
			WithTimeout(5 * time.Minute).
			WithRetries(2).
			WithOnFailure(FailureRollback).
			WithPreHook(hook).
			WithPostHook(hook).
			Build()
	}
}
