// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package pipeline provides the deployment pipeline for orchestrating deployment stages.
package pipeline

import (
	"context"
	"fmt"
	"time"
)

// StageExecutor is responsible for executing individual pipeline stages.
type StageExecutor struct {
	// handlers maps stage types to their handlers
	handlers map[StageType]StageHandler

	// validators maps stage types to their validators
	validators map[StageType]StageValidator
}

// StageValidator validates stage configuration.
type StageValidator func(stage *Stage) error

// StageResult contains the result of stage execution.
type StageResult struct {
	// Success indicates if the stage succeeded
	Success bool `json:"success"`

	// Stage is the executed stage
	Stage *Stage `json:"stage"`

	// Output contains stage output data
	Output map[string]interface{} `json:"output,omitempty"`

	// Error contains error message if failed
	Error string `json:"error,omitempty"`

	// StartedAt is when execution started
	StartedAt time.Time `json:"started_at"`

	// CompletedAt is when execution completed
	CompletedAt time.Time `json:"completed_at"`

	// Duration is the execution duration
	Duration time.Duration `json:"duration"`

	// Retries is the number of retries performed
	Retries int `json:"retries,omitempty"`
}

// NewStageExecutor creates a new stage executor.
func NewStageExecutor() *StageExecutor {
	executor := &StageExecutor{
		handlers:   make(map[StageType]StageHandler),
		validators: make(map[StageType]StageValidator),
	}

	// Register default handlers
	executor.RegisterHandler(StageBuild, defaultBuildHandler)
	executor.RegisterHandler(StageValidate, defaultValidateHandler)
	executor.RegisterHandler(StageDeploy, defaultDeployHandler)
	executor.RegisterHandler(StageVerify, defaultVerifyHandler)
	executor.RegisterHandler(StagePromote, defaultPromoteHandler)
	executor.RegisterHandler(StageCleanup, defaultCleanupHandler)

	// Register default validators
	executor.RegisterValidator(StageBuild, validateBuildStage)
	executor.RegisterValidator(StageValidate, validateValidateStage)
	executor.RegisterValidator(StageDeploy, validateDeployStage)
	executor.RegisterValidator(StageVerify, validateVerifyStage)
	executor.RegisterValidator(StagePromote, validatePromoteStage)
	executor.RegisterValidator(StageCleanup, validateCleanupStage)

	return executor
}

// RegisterHandler registers a stage handler.
func (e *StageExecutor) RegisterHandler(stageType StageType, handler StageHandler) {
	e.handlers[stageType] = handler
}

// RegisterValidator registers a stage validator.
func (e *StageExecutor) RegisterValidator(stageType StageType, validator StageValidator) {
	e.validators[stageType] = validator
}

// Validate validates a stage configuration.
func (e *StageExecutor) Validate(stage *Stage) error {
	if stage == nil {
		return ErrInvalidStage
	}
	if stage.Name == "" {
		return fmt.Errorf("%w: stage name is required", ErrInvalidStage)
	}
	if stage.Type == "" {
		return fmt.Errorf("%w: stage type is required", ErrInvalidStage)
	}

	// Run type-specific validation
	if validator, ok := e.validators[stage.Type]; ok {
		return validator(stage)
	}

	return nil
}

// Execute executes a stage.
func (e *StageExecutor) Execute(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) (*StageResult, error) {
	result := &StageResult{
		Stage:     stage,
		StartedAt: time.Now(),
		Output:    make(map[string]interface{}),
	}

	// Validate stage
	if err := e.Validate(stage); err != nil {
		result.Success = false
		result.Error = err.Error()
		result.CompletedAt = time.Now()
		result.Duration = result.CompletedAt.Sub(result.StartedAt)
		return result, err
	}

	// Get handler
	handler, ok := e.handlers[stage.Type]
	if !ok {
		result.Success = false
		result.Error = fmt.Sprintf("no handler for stage type: %s", stage.Type)
		result.CompletedAt = time.Now()
		result.Duration = result.CompletedAt.Sub(result.StartedAt)
		return result, fmt.Errorf("no handler for stage type: %s", stage.Type)
	}

	// Create stage context with timeout
	stageCtx := ctx
	if stage.Timeout > 0 {
		var cancel context.CancelFunc
		stageCtx, cancel = context.WithTimeout(ctx, stage.Timeout)
		defer cancel()
	}

	// Execute with retries
	maxAttempts := stage.Retries + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		result.Retries = attempt

		lastErr = handler(stageCtx, stage, pipelineCtx)
		if lastErr == nil {
			break
		}

		// Wait before retry (exponential backoff)
		if attempt < maxAttempts-1 {
			backoff := time.Duration(1<<uint(attempt)) * time.Second
			select {
			case <-stageCtx.Done():
				lastErr = stageCtx.Err()
				break
			case <-time.After(backoff):
			}
		}
	}

	result.CompletedAt = time.Now()
	result.Duration = result.CompletedAt.Sub(result.StartedAt)

	if lastErr != nil {
		result.Success = false
		result.Error = lastErr.Error()
		return result, lastErr
	}

	result.Success = true
	if stage.Output != nil {
		result.Output = stage.Output
	}

	return result, nil
}

// Default stage handlers

func defaultBuildHandler(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error {
	// Simplified build handler
	// In production, this would build container images, compile code, etc.

	if stage.Config == nil {
		stage.Output = map[string]interface{}{
			"message": "No build configuration, skipping build",
		}
		return nil
	}

	// Simulate build process
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(100 * time.Millisecond):
	}

	stage.Output = map[string]interface{}{
		"build_image":  stage.Config.BuildImage,
		"build_status": "success",
		"artifact_id":  fmt.Sprintf("artifact-%d", time.Now().Unix()),
	}

	return nil
}

func defaultValidateHandler(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error {
	// Simplified validation handler
	// In production, this would validate configuration, check dependencies, etc.

	validationResults := make(map[string]string)

	// Check required context values
	requiredKeys := []string{"service_name", "version"}
	for _, key := range requiredKeys {
		if _, ok := pipelineCtx[key]; ok {
			validationResults[key] = "present"
		} else {
			validationResults[key] = "missing"
		}
	}

	// Run custom validation rules
	if stage.Config != nil && len(stage.Config.ValidateRules) > 0 {
		for _, rule := range stage.Config.ValidateRules {
			// Simplified rule checking
			validationResults[rule] = "passed"
		}
	}

	stage.Output = map[string]interface{}{
		"validation_results": validationResults,
		"status":             "passed",
	}

	return nil
}

func defaultDeployHandler(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error {
	// Simplified deploy handler
	// In production, this would interact with container runtime, orchestrator, etc.

	strategy := "rolling"
	if stage.Config != nil && stage.Config.DeployStrategy != "" {
		strategy = stage.Config.DeployStrategy
	}

	// Simulate deployment
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(200 * time.Millisecond):
	}

	stage.Output = map[string]interface{}{
		"strategy":      strategy,
		"deploy_status": "deployed",
		"instances":     3,
	}

	return nil
}

func defaultVerifyHandler(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error {
	// Simplified verify handler
	// In production, this would run health checks, smoke tests, etc.

	endpoint := "/health"
	if stage.Config != nil && stage.Config.VerifyEndpoint != "" {
		endpoint = stage.Config.VerifyEndpoint
	}

	// Simulate verification
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(100 * time.Millisecond):
	}

	stage.Output = map[string]interface{}{
		"endpoint":      endpoint,
		"verify_status": "healthy",
		"checks_passed": 5,
		"checks_total":  5,
	}

	return nil
}

func defaultPromoteHandler(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error {
	// Simplified promote handler
	// In production, this would shift traffic, update routing, etc.

	// Simulate promotion
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(50 * time.Millisecond):
	}

	stage.Output = map[string]interface{}{
		"promote_status": "promoted",
		"traffic_weight": 100,
	}

	return nil
}

func defaultCleanupHandler(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error {
	// Simplified cleanup handler
	// In production, this would remove old instances, clean up resources, etc.

	// Simulate cleanup
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(50 * time.Millisecond):
	}

	stage.Output = map[string]interface{}{
		"cleanup_status":   "completed",
		"resources_freed":  3,
	}

	return nil
}

// Default stage validators

func validateBuildStage(stage *Stage) error {
	// Build stage validation is optional
	return nil
}

func validateValidateStage(stage *Stage) error {
	// Validate stage validation is optional
	return nil
}

func validateDeployStage(stage *Stage) error {
	// Deploy stage requires at minimum a timeout
	if stage.Timeout == 0 {
		stage.Timeout = 30 * time.Minute // Default timeout
	}
	return nil
}

func validateVerifyStage(stage *Stage) error {
	// Verify stage validation
	if stage.Timeout == 0 {
		stage.Timeout = 10 * time.Minute // Default timeout
	}
	return nil
}

func validatePromoteStage(stage *Stage) error {
	// Promote stage validation
	if stage.Timeout == 0 {
		stage.Timeout = 5 * time.Minute // Default timeout
	}
	return nil
}

func validateCleanupStage(stage *Stage) error {
	// Cleanup stage validation is optional
	return nil
}

// StageBuilder helps build stage configurations.
type StageBuilder struct {
	stage *Stage
}

// NewStageBuilder creates a new stage builder.
func NewStageBuilder(name string, stageType StageType) *StageBuilder {
	return &StageBuilder{
		stage: &Stage{
			Name:      name,
			Type:      stageType,
			State:     StatePending,
			OnFailure: FailureAbort,
		},
	}
}

// WithConfig sets the stage configuration.
func (b *StageBuilder) WithConfig(config *StageConfig) *StageBuilder {
	b.stage.Config = config
	return b
}

// WithTimeout sets the stage timeout.
func (b *StageBuilder) WithTimeout(timeout time.Duration) *StageBuilder {
	b.stage.Timeout = timeout
	return b
}

// WithRetries sets the number of retries.
func (b *StageBuilder) WithRetries(retries int) *StageBuilder {
	b.stage.Retries = retries
	return b
}

// WithOnFailure sets the failure action.
func (b *StageBuilder) WithOnFailure(action FailureAction) *StageBuilder {
	b.stage.OnFailure = action
	return b
}

// WithPreHook adds a pre-execution hook.
func (b *StageBuilder) WithPreHook(hook *Hook) *StageBuilder {
	b.stage.PreHooks = append(b.stage.PreHooks, hook)
	return b
}

// WithPostHook adds a post-execution hook.
func (b *StageBuilder) WithPostHook(hook *Hook) *StageBuilder {
	b.stage.PostHooks = append(b.stage.PostHooks, hook)
	return b
}

// Build returns the built stage.
func (b *StageBuilder) Build() *Stage {
	return b.stage
}

// BuildStage creates a build stage.
func BuildStage(name string, image string, commands []string) *Stage {
	return NewStageBuilder(name, StageBuild).
		WithConfig(&StageConfig{
			BuildImage:    image,
			BuildCommands: commands,
		}).
		WithTimeout(30 * time.Minute).
		Build()
}

// ValidateStage creates a validation stage.
func ValidateStage(name string, rules []string) *Stage {
	return NewStageBuilder(name, StageValidate).
		WithConfig(&StageConfig{
			ValidateRules: rules,
		}).
		WithTimeout(5 * time.Minute).
		Build()
}

// DeployStage creates a deploy stage.
func DeployStage(name string, strategy string, params map[string]interface{}) *Stage {
	return NewStageBuilder(name, StageDeploy).
		WithConfig(&StageConfig{
			DeployStrategy: strategy,
			DeployParams:   params,
		}).
		WithTimeout(30 * time.Minute).
		WithRetries(2).
		WithOnFailure(FailureRollback).
		Build()
}

// VerifyStage creates a verification stage.
func VerifyStage(name string, endpoint string, timeout time.Duration) *Stage {
	return NewStageBuilder(name, StageVerify).
		WithConfig(&StageConfig{
			VerifyEndpoint: endpoint,
			VerifyTimeout:  timeout,
		}).
		WithTimeout(timeout).
		WithRetries(3).
		WithOnFailure(FailureRollback).
		Build()
}

// PromoteStage creates a promotion stage.
func PromoteStage(name string) *Stage {
	return NewStageBuilder(name, StagePromote).
		WithTimeout(5 * time.Minute).
		WithOnFailure(FailureRollback).
		Build()
}

// CleanupStage creates a cleanup stage.
func CleanupStage(name string) *Stage {
	return NewStageBuilder(name, StageCleanup).
		WithTimeout(10 * time.Minute).
		WithOnFailure(FailureContinue). // Cleanup failures shouldn't block
		Build()
}
