// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package pipeline provides the deployment pipeline for orchestrating deployment stages.
package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Common errors returned by pipeline operations.
var (
	ErrPipelineNotFound    = errors.New("pipeline not found")
	ErrPipelineRunning     = errors.New("pipeline already running")
	ErrPipelineAborted     = errors.New("pipeline aborted")
	ErrStageNotFound       = errors.New("stage not found")
	ErrStageFailed         = errors.New("stage failed")
	ErrHookFailed          = errors.New("hook failed")
	ErrInvalidStage        = errors.New("invalid stage configuration")
)

// PipelineState represents the current state of a pipeline.
type PipelineState string

const (
	StatePending   PipelineState = "pending"
	StateRunning   PipelineState = "running"
	StateCompleted PipelineState = "completed"
	StateFailed    PipelineState = "failed"
	StateAborted   PipelineState = "aborted"
)

// Pipeline represents a deployment pipeline.
type Pipeline struct {
	// ID is the unique pipeline identifier
	ID string `json:"id"`

	// Name is the pipeline name
	Name string `json:"name"`

	// DeploymentID references the parent deployment
	DeploymentID string `json:"deployment_id"`

	// Stages are the pipeline stages in execution order
	Stages []*Stage `json:"stages"`

	// State is the current pipeline state
	State PipelineState `json:"state"`

	// CurrentStage is the index of the currently executing stage
	CurrentStage int `json:"current_stage"`

	// Context holds pipeline context data
	Context map[string]interface{} `json:"context,omitempty"`

	// StartedAt is when the pipeline started
	StartedAt time.Time `json:"started_at"`

	// CompletedAt is when the pipeline completed
	CompletedAt time.Time `json:"completed_at,omitempty"`

	// Duration is the total pipeline duration
	Duration time.Duration `json:"duration,omitempty"`

	// Error contains error details if failed
	Error string `json:"error,omitempty"`
}

// Stage represents a single stage in the pipeline.
type Stage struct {
	// Name is the stage name
	Name string `json:"name"`

	// Type is the stage type
	Type StageType `json:"type"`

	// Config contains stage-specific configuration
	Config *StageConfig `json:"config,omitempty"`

	// PreHooks are hooks to run before the stage
	PreHooks []*Hook `json:"pre_hooks,omitempty"`

	// PostHooks are hooks to run after the stage
	PostHooks []*Hook `json:"post_hooks,omitempty"`

	// State is the current stage state
	State PipelineState `json:"state"`

	// StartedAt is when the stage started
	StartedAt time.Time `json:"started_at,omitempty"`

	// CompletedAt is when the stage completed
	CompletedAt time.Time `json:"completed_at,omitempty"`

	// Duration is the stage duration
	Duration time.Duration `json:"duration,omitempty"`

	// Output contains stage execution output
	Output map[string]interface{} `json:"output,omitempty"`

	// Error contains error details if failed
	Error string `json:"error,omitempty"`

	// Retries is the number of retries for this stage
	Retries int `json:"retries,omitempty"`

	// RetryCount is current retry attempt
	RetryCount int `json:"retry_count,omitempty"`

	// OnFailure specifies behavior on failure
	OnFailure FailureAction `json:"on_failure,omitempty"`

	// Timeout for stage execution
	Timeout time.Duration `json:"timeout,omitempty"`
}

// StageType represents the type of stage.
type StageType string

const (
	StageBuild    StageType = "build"
	StageValidate StageType = "validate"
	StageDeploy   StageType = "deploy"
	StageVerify   StageType = "verify"
	StagePromote  StageType = "promote"
	StageCleanup  StageType = "cleanup"
	StageCustom   StageType = "custom"
)

// FailureAction specifies what to do on stage failure.
type FailureAction string

const (
	FailureAbort    FailureAction = "abort"
	FailureRollback FailureAction = "rollback"
	FailureContinue FailureAction = "continue"
	FailureRetry    FailureAction = "retry"
)

// StageConfig contains stage-specific configuration.
type StageConfig struct {
	// Build stage config
	BuildImage    string   `json:"build_image,omitempty"`
	BuildCommands []string `json:"build_commands,omitempty"`

	// Validate stage config
	ValidateRules []string `json:"validate_rules,omitempty"`

	// Deploy stage config
	DeployStrategy string `json:"deploy_strategy,omitempty"`
	DeployParams   map[string]interface{} `json:"deploy_params,omitempty"`

	// Verify stage config
	VerifyEndpoint string        `json:"verify_endpoint,omitempty"`
	VerifyTimeout  time.Duration `json:"verify_timeout,omitempty"`

	// Custom stage config
	CustomHandler string `json:"custom_handler,omitempty"`
	CustomParams  map[string]interface{} `json:"custom_params,omitempty"`
}

// PipelineSpec defines the specification for creating a pipeline.
type PipelineSpec struct {
	// Name is the pipeline name
	Name string `json:"name"`

	// Stages define the pipeline stages
	Stages []*StageSpec `json:"stages"`

	// Context provides initial context data
	Context map[string]interface{} `json:"context,omitempty"`

	// Timeout is the overall pipeline timeout
	Timeout time.Duration `json:"timeout,omitempty"`

	// Hooks are pipeline-level hooks
	Hooks *PipelineHooks `json:"hooks,omitempty"`
}

// StageSpec defines the specification for a stage.
type StageSpec struct {
	Name      string        `json:"name"`
	Type      StageType     `json:"type"`
	Config    *StageConfig  `json:"config,omitempty"`
	PreHooks  []*HookSpec   `json:"pre_hooks,omitempty"`
	PostHooks []*HookSpec   `json:"post_hooks,omitempty"`
	OnFailure FailureAction `json:"on_failure,omitempty"`
	Timeout   time.Duration `json:"timeout,omitempty"`
	Retries   int           `json:"retries,omitempty"`
}

// PipelineHooks defines pipeline-level hooks.
type PipelineHooks struct {
	OnStart    []*HookSpec `json:"on_start,omitempty"`
	OnSuccess  []*HookSpec `json:"on_success,omitempty"`
	OnFailure  []*HookSpec `json:"on_failure,omitempty"`
	OnComplete []*HookSpec `json:"on_complete,omitempty"`
}

// Executor executes pipelines.
type Executor struct {
	mu sync.RWMutex

	// pipelines tracks running pipelines
	pipelines map[string]*pipelineRun

	// handlers registers custom stage handlers
	handlers map[StageType]StageHandler

	// hookExecutor executes hooks
	hookExecutor *HookExecutor

	// eventCallback for pipeline events
	eventCallback func(event *PipelineEvent)
}

type pipelineRun struct {
	pipeline *Pipeline
	cancel   context.CancelFunc
	done     chan struct{}
}

// StageHandler is a function that handles stage execution.
type StageHandler func(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error

// PipelineEvent represents a pipeline event.
type PipelineEvent struct {
	Type       string                 `json:"type"`
	PipelineID string                 `json:"pipeline_id"`
	StageName  string                 `json:"stage_name,omitempty"`
	State      PipelineState          `json:"state"`
	Message    string                 `json:"message,omitempty"`
	Timestamp  time.Time              `json:"timestamp"`
	Data       map[string]interface{} `json:"data,omitempty"`
}

// NewExecutor creates a new pipeline executor.
func NewExecutor() *Executor {
	return &Executor{
		pipelines:    make(map[string]*pipelineRun),
		handlers:     make(map[StageType]StageHandler),
		hookExecutor: NewHookExecutor(),
	}
}

// RegisterHandler registers a custom stage handler.
func (e *Executor) RegisterHandler(stageType StageType, handler StageHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.handlers[stageType] = handler
}

// SetEventCallback sets the event callback.
func (e *Executor) SetEventCallback(callback func(event *PipelineEvent)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.eventCallback = callback
}

// Execute executes a pipeline from specification.
func (e *Executor) Execute(ctx context.Context, deploymentID string, spec *PipelineSpec) (*Pipeline, error) {
	// Build pipeline from spec
	pipeline := &Pipeline{
		ID:           generatePipelineID(),
		Name:         spec.Name,
		DeploymentID: deploymentID,
		State:        StatePending,
		Context:      spec.Context,
		Stages:       make([]*Stage, len(spec.Stages)),
	}

	for i, stageSpec := range spec.Stages {
		stage := &Stage{
			Name:      stageSpec.Name,
			Type:      stageSpec.Type,
			Config:    stageSpec.Config,
			State:     StatePending,
			OnFailure: stageSpec.OnFailure,
			Timeout:   stageSpec.Timeout,
			Retries:   stageSpec.Retries,
		}

		// Convert hook specs to hooks
		for _, hookSpec := range stageSpec.PreHooks {
			stage.PreHooks = append(stage.PreHooks, hookFromSpec(hookSpec))
		}
		for _, hookSpec := range stageSpec.PostHooks {
			stage.PostHooks = append(stage.PostHooks, hookFromSpec(hookSpec))
		}

		if stage.OnFailure == "" {
			stage.OnFailure = FailureAbort
		}

		pipeline.Stages[i] = stage
	}

	return e.Run(ctx, pipeline)
}

// Run runs a pipeline.
func (e *Executor) Run(ctx context.Context, pipeline *Pipeline) (*Pipeline, error) {
	// Check for existing run
	e.mu.RLock()
	_, exists := e.pipelines[pipeline.ID]
	e.mu.RUnlock()

	if exists {
		return nil, ErrPipelineRunning
	}

	// Create cancellable context
	ctx, cancel := context.WithCancel(ctx)

	run := &pipelineRun{
		pipeline: pipeline,
		cancel:   cancel,
		done:     make(chan struct{}),
	}

	e.mu.Lock()
	e.pipelines[pipeline.ID] = run
	e.mu.Unlock()

	// Execute in background
	go e.executePipeline(ctx, run)

	// Wait for completion or context cancellation
	select {
	case <-run.done:
		return pipeline, nil
	case <-ctx.Done():
		cancel()
		<-run.done
		return pipeline, ctx.Err()
	}
}

// executePipeline executes the pipeline stages.
func (e *Executor) executePipeline(ctx context.Context, run *pipelineRun) {
	defer func() {
		close(run.done)
		e.mu.Lock()
		delete(e.pipelines, run.pipeline.ID)
		e.mu.Unlock()
	}()

	pipeline := run.pipeline
	pipeline.State = StateRunning
	pipeline.StartedAt = time.Now()

	e.emitEvent(&PipelineEvent{
		Type:       "pipeline_started",
		PipelineID: pipeline.ID,
		State:      StateRunning,
		Message:    "Pipeline started",
		Timestamp:  time.Now(),
	})

	// Execute stages
	for i, stage := range pipeline.Stages {
		select {
		case <-ctx.Done():
			pipeline.State = StateAborted
			pipeline.Error = "Pipeline aborted"
			e.emitEvent(&PipelineEvent{
				Type:       "pipeline_aborted",
				PipelineID: pipeline.ID,
				State:      StateAborted,
				Message:    "Pipeline aborted",
				Timestamp:  time.Now(),
			})
			return
		default:
		}

		pipeline.CurrentStage = i

		if err := e.executeStage(ctx, pipeline, stage); err != nil {
			switch stage.OnFailure {
			case FailureAbort:
				pipeline.State = StateFailed
				pipeline.Error = fmt.Sprintf("Stage %s failed: %v", stage.Name, err)
				e.emitEvent(&PipelineEvent{
					Type:       "pipeline_failed",
					PipelineID: pipeline.ID,
					StageName:  stage.Name,
					State:      StateFailed,
					Message:    pipeline.Error,
					Timestamp:  time.Now(),
				})
				return

			case FailureRollback:
				// Trigger rollback - handled by caller
				pipeline.State = StateFailed
				pipeline.Error = fmt.Sprintf("Stage %s failed, triggering rollback: %v", stage.Name, err)
				e.emitEvent(&PipelineEvent{
					Type:       "pipeline_rollback",
					PipelineID: pipeline.ID,
					StageName:  stage.Name,
					State:      StateFailed,
					Message:    pipeline.Error,
					Timestamp:  time.Now(),
					Data:       map[string]interface{}{"rollback": true},
				})
				return

			case FailureContinue:
				// Log and continue to next stage
				stage.Error = err.Error()
				continue

			case FailureRetry:
				// Retry logic is handled in executeStage
				if stage.RetryCount >= stage.Retries {
					pipeline.State = StateFailed
					pipeline.Error = fmt.Sprintf("Stage %s failed after %d retries: %v", stage.Name, stage.Retries, err)
					return
				}
			}
		}
	}

	// Pipeline completed successfully
	pipeline.State = StateCompleted
	pipeline.CompletedAt = time.Now()
	pipeline.Duration = pipeline.CompletedAt.Sub(pipeline.StartedAt)

	e.emitEvent(&PipelineEvent{
		Type:       "pipeline_completed",
		PipelineID: pipeline.ID,
		State:      StateCompleted,
		Message:    "Pipeline completed successfully",
		Timestamp:  time.Now(),
		Data: map[string]interface{}{
			"duration": pipeline.Duration.String(),
		},
	})
}

// executeStage executes a single stage.
func (e *Executor) executeStage(ctx context.Context, pipeline *Pipeline, stage *Stage) error {
	stage.State = StateRunning
	stage.StartedAt = time.Now()

	e.emitEvent(&PipelineEvent{
		Type:       "stage_started",
		PipelineID: pipeline.ID,
		StageName:  stage.Name,
		State:      StateRunning,
		Message:    fmt.Sprintf("Stage %s started", stage.Name),
		Timestamp:  time.Now(),
	})

	// Create stage context with timeout
	stageCtx := ctx
	if stage.Timeout > 0 {
		var cancel context.CancelFunc
		stageCtx, cancel = context.WithTimeout(ctx, stage.Timeout)
		defer cancel()
	}

	// Execute pre-hooks
	for _, hook := range stage.PreHooks {
		if err := e.hookExecutor.Execute(stageCtx, hook, pipeline.Context); err != nil {
			if hook.OnError != HookErrorContinue {
				stage.State = StateFailed
				stage.Error = fmt.Sprintf("Pre-hook %s failed: %v", hook.Name, err)
				return ErrHookFailed
			}
		}
	}

	// Execute stage with retry support
	var lastErr error
	maxAttempts := stage.Retries + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		stage.RetryCount = attempt

		lastErr = e.executeStageHandler(stageCtx, stage, pipeline.Context)
		if lastErr == nil {
			break
		}

		if attempt < maxAttempts-1 {
			// Wait before retry
			select {
			case <-stageCtx.Done():
				return stageCtx.Err()
			case <-time.After(time.Second * time.Duration(attempt+1)):
			}
		}
	}

	if lastErr != nil {
		stage.State = StateFailed
		stage.Error = lastErr.Error()
		stage.CompletedAt = time.Now()
		stage.Duration = stage.CompletedAt.Sub(stage.StartedAt)

		e.emitEvent(&PipelineEvent{
			Type:       "stage_failed",
			PipelineID: pipeline.ID,
			StageName:  stage.Name,
			State:      StateFailed,
			Message:    fmt.Sprintf("Stage %s failed: %v", stage.Name, lastErr),
			Timestamp:  time.Now(),
		})

		return lastErr
	}

	// Execute post-hooks
	for _, hook := range stage.PostHooks {
		if err := e.hookExecutor.Execute(stageCtx, hook, pipeline.Context); err != nil {
			if hook.OnError != HookErrorContinue {
				stage.State = StateFailed
				stage.Error = fmt.Sprintf("Post-hook %s failed: %v", hook.Name, err)
				return ErrHookFailed
			}
		}
	}

	stage.State = StateCompleted
	stage.CompletedAt = time.Now()
	stage.Duration = stage.CompletedAt.Sub(stage.StartedAt)

	e.emitEvent(&PipelineEvent{
		Type:       "stage_completed",
		PipelineID: pipeline.ID,
		StageName:  stage.Name,
		State:      StateCompleted,
		Message:    fmt.Sprintf("Stage %s completed", stage.Name),
		Timestamp:  time.Now(),
		Data: map[string]interface{}{
			"duration": stage.Duration.String(),
		},
	})

	return nil
}

// executeStageHandler executes the appropriate handler for the stage.
func (e *Executor) executeStageHandler(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error {
	e.mu.RLock()
	handler, exists := e.handlers[stage.Type]
	e.mu.RUnlock()

	if exists {
		return handler(ctx, stage, pipelineCtx)
	}

	// Default handlers
	switch stage.Type {
	case StageBuild:
		return e.handleBuildStage(ctx, stage, pipelineCtx)
	case StageValidate:
		return e.handleValidateStage(ctx, stage, pipelineCtx)
	case StageDeploy:
		return e.handleDeployStage(ctx, stage, pipelineCtx)
	case StageVerify:
		return e.handleVerifyStage(ctx, stage, pipelineCtx)
	case StagePromote:
		return e.handlePromoteStage(ctx, stage, pipelineCtx)
	case StageCleanup:
		return e.handleCleanupStage(ctx, stage, pipelineCtx)
	case StageCustom:
		return e.handleCustomStage(ctx, stage, pipelineCtx)
	default:
		return fmt.Errorf("unknown stage type: %s", stage.Type)
	}
}

// Default stage handlers (simplified implementations)

func (e *Executor) handleBuildStage(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error {
	// Build stage implementation would build artifacts
	stage.Output = map[string]interface{}{
		"build_status": "success",
	}
	return nil
}

func (e *Executor) handleValidateStage(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error {
	// Validate stage implementation would validate configuration
	stage.Output = map[string]interface{}{
		"validation_status": "passed",
	}
	return nil
}

func (e *Executor) handleDeployStage(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error {
	// Deploy stage implementation would perform the deployment
	stage.Output = map[string]interface{}{
		"deploy_status": "deployed",
	}
	return nil
}

func (e *Executor) handleVerifyStage(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error {
	// Verify stage implementation would verify the deployment
	stage.Output = map[string]interface{}{
		"verify_status": "healthy",
	}
	return nil
}

func (e *Executor) handlePromoteStage(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error {
	// Promote stage implementation would promote traffic
	stage.Output = map[string]interface{}{
		"promote_status": "promoted",
	}
	return nil
}

func (e *Executor) handleCleanupStage(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error {
	// Cleanup stage implementation would clean up resources
	stage.Output = map[string]interface{}{
		"cleanup_status": "cleaned",
	}
	return nil
}

func (e *Executor) handleCustomStage(ctx context.Context, stage *Stage, pipelineCtx map[string]interface{}) error {
	// Custom stage requires a registered handler
	if stage.Config == nil || stage.Config.CustomHandler == "" {
		return fmt.Errorf("custom stage requires a handler")
	}
	return nil
}

// Abort aborts a running pipeline.
func (e *Executor) Abort(pipelineID string) error {
	e.mu.RLock()
	run, exists := e.pipelines[pipelineID]
	e.mu.RUnlock()

	if !exists {
		return ErrPipelineNotFound
	}

	run.cancel()
	return nil
}

// Status returns the status of a pipeline.
func (e *Executor) Status(pipelineID string) (*Pipeline, error) {
	e.mu.RLock()
	run, exists := e.pipelines[pipelineID]
	e.mu.RUnlock()

	if !exists {
		return nil, ErrPipelineNotFound
	}

	return run.pipeline, nil
}

// emitEvent emits a pipeline event.
func (e *Executor) emitEvent(event *PipelineEvent) {
	e.mu.RLock()
	callback := e.eventCallback
	e.mu.RUnlock()

	if callback != nil {
		go callback(event)
	}
}

// generatePipelineID generates a unique pipeline ID.
func generatePipelineID() string {
	return fmt.Sprintf("pipeline-%d", time.Now().UnixNano())
}

// hookFromSpec creates a Hook from HookSpec.
func hookFromSpec(spec *HookSpec) *Hook {
	return &Hook{
		Name:    spec.Name,
		Type:    spec.Type,
		Config:  spec.Config,
		Timeout: spec.Timeout,
		OnError: spec.OnError,
	}
}

// MarshalJSON implements json.Marshaler for Pipeline.
func (p *Pipeline) MarshalJSON() ([]byte, error) {
	type Alias Pipeline
	return json.Marshal(&struct {
		*Alias
		Duration string `json:"duration,omitempty"`
	}{
		Alias:    (*Alias)(p),
		Duration: p.Duration.String(),
	})
}

// DefaultPipelineSpec returns a default deployment pipeline specification.
func DefaultPipelineSpec() *PipelineSpec {
	return &PipelineSpec{
		Name: "default-deployment-pipeline",
		Stages: []*StageSpec{
			{
				Name:      "validate",
				Type:      StageValidate,
				OnFailure: FailureAbort,
				Timeout:   5 * time.Minute,
			},
			{
				Name:      "deploy",
				Type:      StageDeploy,
				OnFailure: FailureRollback,
				Timeout:   30 * time.Minute,
				Retries:   2,
			},
			{
				Name:      "verify",
				Type:      StageVerify,
				OnFailure: FailureRollback,
				Timeout:   10 * time.Minute,
				Retries:   3,
			},
			{
				Name:      "promote",
				Type:      StagePromote,
				OnFailure: FailureRollback,
				Timeout:   5 * time.Minute,
			},
		},
		Timeout: 1 * time.Hour,
	}
}
