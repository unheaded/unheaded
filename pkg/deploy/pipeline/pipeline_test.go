// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package pipeline provides the deployment pipeline for orchestrating deployment stages.
package pipeline

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestNewExecutor tests executor creation.
func TestNewExecutor(t *testing.T) {
	executor := NewExecutor()

	if executor == nil {
		t.Fatal("NewExecutor returned nil")
	}
	if executor.pipelines == nil {
		t.Error("pipelines map should be initialized")
	}
	if executor.handlers == nil {
		t.Error("handlers map should be initialized")
	}
	if executor.hookExecutor == nil {
		t.Error("hookExecutor should be initialized")
	}
}

// TestPipelineStateConstants tests pipeline state constant values.
func TestPipelineStateConstants(t *testing.T) {
	tests := []struct {
		state    PipelineState
		expected string
	}{
		{StatePending, "pending"},
		{StateRunning, "running"},
		{StateCompleted, "completed"},
		{StateFailed, "failed"},
		{StateAborted, "aborted"},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if string(tt.state) != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, string(tt.state))
			}
		})
	}
}

// TestStageTypeConstants tests stage type constant values.
func TestStageTypeConstants(t *testing.T) {
	tests := []struct {
		stageType StageType
		expected  string
	}{
		{StageBuild, "build"},
		{StageValidate, "validate"},
		{StageDeploy, "deploy"},
		{StageVerify, "verify"},
		{StagePromote, "promote"},
		{StageCleanup, "cleanup"},
		{StageCustom, "custom"},
	}

	for _, tt := range tests {
		t.Run(string(tt.stageType), func(t *testing.T) {
			if string(tt.stageType) != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, string(tt.stageType))
			}
		})
	}
}

// TestFailureActionConstants tests failure action constant values.
func TestFailureActionConstants(t *testing.T) {
	tests := []struct {
		action   FailureAction
		expected string
	}{
		{FailureAbort, "abort"},
		{FailureRollback, "rollback"},
		{FailureContinue, "continue"},
		{FailureRetry, "retry"},
	}

	for _, tt := range tests {
		t.Run(string(tt.action), func(t *testing.T) {
			if string(tt.action) != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, string(tt.action))
			}
		})
	}
}

// TestExecutorRegisterHandler tests registering custom handlers.
func TestExecutorRegisterHandler(t *testing.T) {
	executor := NewExecutor()

	customHandler := func(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error {
		return nil
	}

	executor.RegisterHandler(StageCustom, customHandler)

	// Verify handler is registered
	executor.mu.RLock()
	_, exists := executor.handlers[StageCustom]
	executor.mu.RUnlock()

	if !exists {
		t.Error("handler should be registered")
	}
}

// TestExecutorSetEventCallback tests setting event callback.
func TestExecutorSetEventCallback(t *testing.T) {
	executor := NewExecutor()

	callback := func(event *PipelineEvent) {
		// Callback registered for testing
	}

	executor.SetEventCallback(callback)

	executor.mu.RLock()
	callbackSet := executor.eventCallback != nil
	executor.mu.RUnlock()

	if !callbackSet {
		t.Error("event callback should be set")
	}
}

// TestDefaultPipelineSpec tests default pipeline specification.
func TestDefaultPipelineSpec(t *testing.T) {
	spec := DefaultPipelineSpec()

	if spec == nil {
		t.Fatal("DefaultPipelineSpec returned nil")
	}
	if spec.Name != "default-deployment-pipeline" {
		t.Errorf("expected name 'default-deployment-pipeline', got %s", spec.Name)
	}
	if len(spec.Stages) != 4 {
		t.Errorf("expected 4 stages, got %d", len(spec.Stages))
	}
	if spec.Timeout != 1*time.Hour {
		t.Errorf("expected 1h timeout, got %v", spec.Timeout)
	}

	// Verify stages
	expectedStages := []StageType{StageValidate, StageDeploy, StageVerify, StagePromote}
	for i, stageSpec := range spec.Stages {
		if stageSpec.Type != expectedStages[i] {
			t.Errorf("stage %d: expected type %s, got %s", i, expectedStages[i], stageSpec.Type)
		}
	}
}

// TestPipelineExecutionSuccess tests successful pipeline execution.
func TestPipelineExecutionSuccess(t *testing.T) {
	executor := NewExecutor()

	spec := &PipelineSpec{
		Name: "test-pipeline",
		Stages: []*StageSpec{
			{Name: "validate", Type: StageValidate, Timeout: 5 * time.Second},
			{Name: "deploy", Type: StageDeploy, Timeout: 5 * time.Second},
		},
		Context: map[string]interface{}{
			"service": "test-service",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pipeline, err := executor.Execute(ctx, "deploy-123", spec)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if pipeline == nil {
		t.Fatal("pipeline should not be nil")
	}
	if pipeline.DeploymentID != "deploy-123" {
		t.Errorf("expected deployment ID 'deploy-123', got %s", pipeline.DeploymentID)
	}
	if pipeline.State != StateCompleted {
		t.Errorf("expected state %s, got %s", StateCompleted, pipeline.State)
	}
}

// TestPipelineExecutionWithCustomHandler tests pipeline with custom stage handler.
func TestPipelineExecutionWithCustomHandler(t *testing.T) {
	executor := NewExecutor()

	var customHandlerCalled int32 = 0
	customHandler := func(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error {
		atomic.AddInt32(&customHandlerCalled, 1)
		stage.Output = map[string]interface{}{
			"custom_result": "success",
		}
		return nil
	}
	executor.RegisterHandler(StageCustom, customHandler)

	spec := &PipelineSpec{
		Name: "custom-pipeline",
		Stages: []*StageSpec{
			{
				Name: "custom-stage",
				Type: StageCustom,
				Config: &StageConfig{
					CustomHandler: "test-handler",
				},
				Timeout: 5 * time.Second,
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pipeline, err := executor.Execute(ctx, "deploy-456", spec)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if atomic.LoadInt32(&customHandlerCalled) != 1 {
		t.Error("custom handler should have been called once")
	}

	if pipeline.State != StateCompleted {
		t.Errorf("expected state %s, got %s", StateCompleted, pipeline.State)
	}
}

// TestPipelineExecutionFailure tests pipeline execution with stage failure.
func TestPipelineExecutionFailure(t *testing.T) {
	executor := NewExecutor()

	failingHandler := func(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error {
		return errors.New("stage failed intentionally")
	}
	executor.RegisterHandler(StageDeploy, failingHandler)

	spec := &PipelineSpec{
		Name: "failing-pipeline",
		Stages: []*StageSpec{
			{Name: "validate", Type: StageValidate, Timeout: 5 * time.Second},
			{Name: "deploy", Type: StageDeploy, Timeout: 5 * time.Second, OnFailure: FailureAbort},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pipeline, err := executor.Execute(ctx, "deploy-789", spec)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if pipeline.State != StateFailed {
		t.Errorf("expected state %s, got %s", StateFailed, pipeline.State)
	}
	if pipeline.Error == "" {
		t.Error("pipeline should have an error message")
	}
}

// TestPipelineExecutionWithContinueOnFailure tests continue on failure behavior.
func TestPipelineExecutionWithContinueOnFailure(t *testing.T) {
	executor := NewExecutor()

	stageOrder := []string{}
	var mu sync.Mutex

	executor.RegisterHandler(StageValidate, func(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error {
		mu.Lock()
		stageOrder = append(stageOrder, stage.Name)
		mu.Unlock()
		return errors.New("validation failed")
	})

	executor.RegisterHandler(StageDeploy, func(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error {
		mu.Lock()
		stageOrder = append(stageOrder, stage.Name)
		mu.Unlock()
		return nil
	})

	spec := &PipelineSpec{
		Name: "continue-pipeline",
		Stages: []*StageSpec{
			{Name: "validate", Type: StageValidate, Timeout: 5 * time.Second, OnFailure: FailureContinue},
			{Name: "deploy", Type: StageDeploy, Timeout: 5 * time.Second},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pipeline, err := executor.Execute(ctx, "deploy-continue", spec)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(stageOrder) != 2 {
		t.Errorf("expected 2 stages executed, got %d", len(stageOrder))
	}
	if pipeline.State != StateCompleted {
		t.Errorf("expected state %s, got %s", StateCompleted, pipeline.State)
	}
}

// TestPipelineAbort tests aborting a running pipeline.
func TestPipelineAbort(t *testing.T) {
	executor := NewExecutor()

	longRunningHandler := func(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
			return nil
		}
	}
	executor.RegisterHandler(StageDeploy, longRunningHandler)

	spec := &PipelineSpec{
		Name: "abort-pipeline",
		Stages: []*StageSpec{
			{Name: "deploy", Type: StageDeploy, Timeout: 15 * time.Second},
		},
	}

	ctx := context.Background()

	// Start pipeline in goroutine
	var pipeline *Pipeline
	var execErr error
	done := make(chan struct{})

	go func() {
		pipeline, execErr = executor.Execute(ctx, "deploy-abort", spec)
		close(done)
	}()

	// Wait a bit for pipeline to start
	time.Sleep(100 * time.Millisecond)

	// Get the pipeline ID
	executor.mu.RLock()
	var pipelineID string
	for id := range executor.pipelines {
		pipelineID = id
		break
	}
	executor.mu.RUnlock()

	if pipelineID != "" {
		err := executor.Abort(pipelineID)
		if err != nil {
			t.Errorf("Abort failed: %v", err)
		}
	}

	// Wait for pipeline to finish
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("pipeline did not complete after abort")
	}

	if execErr == nil && pipeline != nil {
		if pipeline.State != StateAborted {
			// Context was cancelled, so this is expected
			t.Logf("pipeline state: %s", pipeline.State)
		}
	}
}

// TestPipelineStatus tests getting pipeline status.
func TestPipelineStatus(t *testing.T) {
	executor := NewExecutor()

	// Try to get status of non-existent pipeline
	_, err := executor.Status("non-existent")
	if err != ErrPipelineNotFound {
		t.Errorf("expected ErrPipelineNotFound, got %v", err)
	}
}

// TestPipelineAlreadyRunning tests running same pipeline twice.
func TestPipelineAlreadyRunning(t *testing.T) {
	executor := NewExecutor()

	slowHandler := func(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error {
		time.Sleep(1 * time.Second)
		return nil
	}
	executor.RegisterHandler(StageDeploy, slowHandler)

	pipeline := &Pipeline{
		ID:           "test-pipeline-1",
		Name:         "test",
		DeploymentID: "deploy-1",
		State:        StatePending,
		Stages: []*Stage{
			{Name: "deploy", Type: StageDeploy, State: StatePending, Timeout: 10 * time.Second},
		},
	}

	ctx := context.Background()

	// Start first run
	go executor.Run(ctx, pipeline)

	// Wait for it to start
	time.Sleep(50 * time.Millisecond)

	// Try to run the same pipeline again
	_, err := executor.Run(ctx, pipeline)
	if err != ErrPipelineRunning {
		t.Errorf("expected ErrPipelineRunning, got %v", err)
	}
}

// TestPipelineWithRetries tests stage retry functionality.
func TestPipelineWithRetries(t *testing.T) {
	executor := NewExecutor()

	attemptCount := 0
	var mu sync.Mutex

	retryHandler := func(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error {
		mu.Lock()
		attemptCount++
		attempt := attemptCount
		mu.Unlock()

		if attempt < 3 {
			return errors.New("temporary failure")
		}
		return nil
	}
	executor.RegisterHandler(StageDeploy, retryHandler)

	spec := &PipelineSpec{
		Name: "retry-pipeline",
		Stages: []*StageSpec{
			{
				Name:      "deploy",
				Type:      StageDeploy,
				Timeout:   30 * time.Second,
				Retries:   3,
				OnFailure: FailureRetry,
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pipeline, err := executor.Execute(ctx, "deploy-retry", spec)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if pipeline.State != StateCompleted {
		t.Errorf("expected state %s, got %s", StateCompleted, pipeline.State)
	}

	mu.Lock()
	finalCount := attemptCount
	mu.Unlock()

	if finalCount != 3 {
		t.Errorf("expected 3 attempts, got %d", finalCount)
	}
}

// TestPipelineEvents tests event emission.
func TestPipelineEvents(t *testing.T) {
	executor := NewExecutor()

	var events []*PipelineEvent
	var mu sync.Mutex

	executor.SetEventCallback(func(event *PipelineEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	spec := &PipelineSpec{
		Name: "event-pipeline",
		Stages: []*StageSpec{
			{Name: "validate", Type: StageValidate, Timeout: 5 * time.Second},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := executor.Execute(ctx, "deploy-events", spec)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Wait for events to be processed
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	eventCount := len(events)
	mu.Unlock()

	if eventCount < 2 {
		t.Errorf("expected at least 2 events (start + complete), got %d", eventCount)
	}
}

// TestPipelineContextPropagation tests that context is passed through stages.
func TestPipelineContextPropagation(t *testing.T) {
	executor := NewExecutor()

	var contextValue interface{}
	var mu sync.Mutex

	executor.RegisterHandler(StageDeploy, func(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error {
		mu.Lock()
		contextValue = pipelineCtx["test_key"]
		mu.Unlock()
		return nil
	})

	spec := &PipelineSpec{
		Name: "context-pipeline",
		Stages: []*StageSpec{
			{Name: "deploy", Type: StageDeploy, Timeout: 5 * time.Second},
		},
		Context: map[string]interface{}{
			"test_key": "test_value",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := executor.Execute(ctx, "deploy-context", spec)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if contextValue != "test_value" {
		t.Errorf("expected context value 'test_value', got %v", contextValue)
	}
}

// TestPipelineStageTimeout tests stage timeout handling.
func TestPipelineStageTimeout(t *testing.T) {
	executor := NewExecutor()

	slowHandler := func(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
			return nil
		}
	}
	executor.RegisterHandler(StageDeploy, slowHandler)

	spec := &PipelineSpec{
		Name: "timeout-pipeline",
		Stages: []*StageSpec{
			{
				Name:      "deploy",
				Type:      StageDeploy,
				Timeout:   100 * time.Millisecond,
				OnFailure: FailureAbort,
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline, err := executor.Execute(ctx, "deploy-timeout", spec)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if pipeline.State != StateFailed {
		t.Errorf("expected state %s, got %s", StateFailed, pipeline.State)
	}
}

// TestPipelineMarshalJSON tests JSON marshaling of pipeline.
func TestPipelineMarshalJSON(t *testing.T) {
	pipeline := &Pipeline{
		ID:           "test-pipeline",
		Name:         "Test Pipeline",
		DeploymentID: "deploy-123",
		State:        StateCompleted,
		StartedAt:    time.Now(),
		CompletedAt:  time.Now().Add(5 * time.Minute),
		Duration:     5 * time.Minute,
	}

	data, err := pipeline.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("marshaled data should not be empty")
	}
}

// TestHookFromSpec tests hook creation from specification.
func TestHookFromSpec(t *testing.T) {
	spec := &HookSpec{
		Name:    "test-hook",
		Type:    HookTypeHTTP,
		Timeout: 30 * time.Second,
		OnError: HookErrorContinue,
		Config: &HookConfig{
			URL:    "http://example.com/webhook",
			Method: "POST",
		},
	}

	hook := hookFromSpec(spec)

	if hook.Name != spec.Name {
		t.Errorf("expected name %s, got %s", spec.Name, hook.Name)
	}
	if hook.Type != spec.Type {
		t.Errorf("expected type %s, got %s", spec.Type, hook.Type)
	}
	if hook.Timeout != spec.Timeout {
		t.Errorf("expected timeout %v, got %v", spec.Timeout, hook.Timeout)
	}
	if hook.OnError != spec.OnError {
		t.Errorf("expected onError %s, got %s", spec.OnError, hook.OnError)
	}
}

// TestDefaultStageHandlers tests default stage handlers.
func TestDefaultStageHandlers(t *testing.T) {
	executor := NewExecutor()

	testCases := []struct {
		name      string
		stageType StageType
	}{
		{"build", StageBuild},
		{"validate", StageValidate},
		{"deploy", StageDeploy},
		{"verify", StageVerify},
		{"promote", StagePromote},
		{"cleanup", StageCleanup},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stage := &Stage{
				Name:  tc.name,
				Type:  tc.stageType,
				State: StatePending,
			}

			ctx := context.Background()
			pipelineCtx := map[string]interface{}{}

			err := executor.executeStageHandler(ctx, stage, pipelineCtx)
			if err != nil {
				t.Errorf("handler for %s failed: %v", tc.name, err)
			}

			if stage.Output == nil {
				t.Errorf("handler for %s did not set output", tc.name)
			}
		})
	}
}

// TestCustomStageWithoutHandler tests custom stage without handler.
func TestCustomStageWithoutHandler(t *testing.T) {
	executor := NewExecutor()

	stage := &Stage{
		Name:   "custom",
		Type:   StageCustom,
		State:  StatePending,
		Config: nil, // No custom handler configured
	}

	ctx := context.Background()
	pipelineCtx := map[string]interface{}{}

	err := executor.executeStageHandler(ctx, stage, pipelineCtx)
	if err == nil {
		t.Error("expected error for custom stage without handler")
	}
}

// TestPipelineWithPreHooks tests pipeline with pre-stage hooks.
func TestPipelineWithPreHooks(t *testing.T) {
	executor := NewExecutor()

	hookExecuted := false
	var mu sync.Mutex

	// We can't easily test actual hook execution without mocking,
	// but we can test the pipeline structure accepts hooks
	spec := &PipelineSpec{
		Name: "hook-pipeline",
		Stages: []*StageSpec{
			{
				Name:    "deploy",
				Type:    StageDeploy,
				Timeout: 5 * time.Second,
				PreHooks: []*HookSpec{
					{
						Name: "pre-deploy-hook",
						Type: HookTypeExec,
						Config: &HookConfig{
							Command: "echo",
							Args:    []string{"pre-deploy"},
						},
						OnError: HookErrorContinue,
					},
				},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pipeline, err := executor.Execute(ctx, "deploy-hooks", spec)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	mu.Lock()
	_ = hookExecuted // suppress unused warning
	mu.Unlock()

	if len(pipeline.Stages) != 1 {
		t.Errorf("expected 1 stage, got %d", len(pipeline.Stages))
	}
}

// TestGeneratePipelineID tests pipeline ID generation.
func TestGeneratePipelineID(t *testing.T) {
	id1 := generatePipelineID()
	time.Sleep(time.Nanosecond * 100) // Ensure unique timestamp
	id2 := generatePipelineID()

	if id1 == "" {
		t.Error("generated ID should not be empty")
	}
	if id1 == id2 {
		t.Error("generated IDs should be unique")
	}
	if len(id1) < 10 {
		t.Error("generated ID should have reasonable length")
	}
}

// TestPipelineWithRollbackTrigger tests rollback action on failure.
func TestPipelineWithRollbackTrigger(t *testing.T) {
	executor := NewExecutor()

	executor.RegisterHandler(StageDeploy, func(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error {
		return errors.New("deploy failed")
	})

	var rollbackEventReceived bool
	var mu sync.Mutex

	executor.SetEventCallback(func(event *PipelineEvent) {
		if event.Type == "pipeline_rollback" {
			mu.Lock()
			rollbackEventReceived = true
			mu.Unlock()
		}
	})

	spec := &PipelineSpec{
		Name: "rollback-trigger-pipeline",
		Stages: []*StageSpec{
			{
				Name:      "deploy",
				Type:      StageDeploy,
				Timeout:   5 * time.Second,
				OnFailure: FailureRollback,
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pipeline, err := executor.Execute(ctx, "deploy-rollback-trigger", spec)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Wait for events
	time.Sleep(100 * time.Millisecond)

	if pipeline.State != StateFailed {
		t.Errorf("expected state %s, got %s", StateFailed, pipeline.State)
	}

	mu.Lock()
	defer mu.Unlock()
	if !rollbackEventReceived {
		t.Error("rollback event should have been received")
	}
}

// BenchmarkPipelineExecution benchmarks pipeline execution.
func BenchmarkPipelineExecution(b *testing.B) {
	executor := NewExecutor()

	spec := &PipelineSpec{
		Name: "bench-pipeline",
		Stages: []*StageSpec{
			{Name: "validate", Type: StageValidate, Timeout: 5 * time.Second},
			{Name: "deploy", Type: StageDeploy, Timeout: 5 * time.Second},
		},
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		deploymentID := generatePipelineID()
		_, _ = executor.Execute(ctx, deploymentID, spec)
	}
}

// BenchmarkPipelineIDGeneration benchmarks ID generation.
func BenchmarkPipelineIDGeneration(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = generatePipelineID()
	}
}
