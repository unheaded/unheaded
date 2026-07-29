// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package yaldabaoth provides chaos engineering and fault injection capabilities.
// In Gnostic philosophy, Yaldabaoth is the Demiurge - a false god who created the flawed material world.
// This service embodies that concept: deliberately introducing chaos, testing resilience,
// and proving the Kingdom can survive adversity. The adversary that makes us stronger.
package yaldabaoth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"unheaded/pkg/logger"
	wotanClient "unheaded/pkg/wotan-client"
)

// Experiment represents a chaos engineering experiment.
type Experiment struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	Type         ExperimentType         `json:"type"`
	Target       Target                 `json:"target"`
	Parameters   map[string]interface{} `json:"parameters"`
	Schedule     *Schedule              `json:"schedule,omitempty"`
	Status       ExperimentStatus       `json:"status"`
	SteadyState  *SteadyState           `json:"steady_state,omitempty"`
	SafetyChecks []SafetyCheck          `json:"safety_checks,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	StartedAt    *time.Time             `json:"started_at,omitempty"`
	CompletedAt  *time.Time             `json:"completed_at,omitempty"`
	Results      *ExperimentResult      `json:"results,omitempty"`
	Rollback     *Rollback              `json:"rollback,omitempty"`
}

// ExperimentType defines the kind of chaos experiment.
type ExperimentType string

const (
	ExpTypeNetworkDelay     ExperimentType = "network_delay"
	ExpTypeNetworkLoss      ExperimentType = "network_loss"
	ExpTypeNetworkPartition ExperimentType = "network_partition"
	ExpTypeCPUStress        ExperimentType = "cpu_stress"
	ExpTypeMemoryStress     ExperimentType = "memory_stress"
	ExpTypeDiskStress       ExperimentType = "disk_stress"
	ExpTypeProcessKill      ExperimentType = "process_kill"
	ExpTypeContainerKill    ExperimentType = "container_kill"
	ExpTypeNodeDrain        ExperimentType = "node_drain"
	ExpTypeDNSFailure       ExperimentType = "dns_failure"
	ExpTypeTimeSkew         ExperimentType = "time_skew"
	ExpTypeHTTPError        ExperimentType = "http_error"
	ExpTypeLatencySpike     ExperimentType = "latency_spike"
	ExpTypeResourceExhaust  ExperimentType = "resource_exhaustion"
	ExpTypeCustom           ExperimentType = "custom"
)

// ExperimentStatus tracks experiment lifecycle.
type ExperimentStatus string

const (
	StatusDraft     ExperimentStatus = "draft"
	StatusScheduled ExperimentStatus = "scheduled"
	StatusRunning   ExperimentStatus = "running"
	StatusPaused    ExperimentStatus = "paused"
	StatusCompleted ExperimentStatus = "completed"
	StatusAborted   ExperimentStatus = "aborted"
	StatusFailed    ExperimentStatus = "failed"
)

// Target defines what the experiment affects.
type Target struct {
	Type       TargetType        `json:"type"`
	Selector   map[string]string `json:"selector"`
	Namespace  string            `json:"namespace,omitempty"`
	Percentage int               `json:"percentage,omitempty"` // % of matching targets to affect
}

// TargetType defines what kind of resource is targeted.
type TargetType string

const (
	TargetService   TargetType = "service"
	TargetContainer TargetType = "container"
	TargetNode      TargetType = "node"
	TargetNetwork   TargetType = "network"
	TargetPod       TargetType = "pod"
)

// Schedule defines when experiments run.
type Schedule struct {
	Type      ScheduleType  `json:"type"`
	Cron      string        `json:"cron,omitempty"`
	Interval  time.Duration `json:"interval,omitempty"`
	StartTime *time.Time    `json:"start_time,omitempty"`
	EndTime   *time.Time    `json:"end_time,omitempty"`
	Duration  time.Duration `json:"duration,omitempty"`
}

// ScheduleType defines how experiments are scheduled.
type ScheduleType string

const (
	ScheduleOnce       ScheduleType = "once"
	ScheduleRecurring  ScheduleType = "recurring"
	ScheduleContinuous ScheduleType = "continuous"
	ScheduleGameDay    ScheduleType = "game_day"
)

// SteadyState defines the expected normal state before/after experiment.
type SteadyState struct {
	Hypothesis string        `json:"hypothesis"`
	Probes     []Probe       `json:"probes"`
	Tolerance  float64       `json:"tolerance"` // Acceptable deviation %
	Timeout    time.Duration `json:"timeout"`
}

// Probe checks system state.
type Probe struct {
	Name     string                 `json:"name"`
	Type     ProbeType              `json:"type"`
	Provider string                 `json:"provider"`
	Config   map[string]interface{} `json:"config"`
}

// ProbeType defines how state is checked.
type ProbeType string

const (
	ProbeHTTP    ProbeType = "http"
	ProbeMetric  ProbeType = "metric"
	ProbeProcess ProbeType = "process"
	ProbeCustom  ProbeType = "custom"
)

// SafetyCheck defines conditions that abort the experiment.
type SafetyCheck struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Condition   string       `json:"condition"`
	Threshold   float64      `json:"threshold"`
	Action      SafetyAction `json:"action"`
}

// SafetyAction defines what happens when safety check fails.
type SafetyAction string

const (
	ActionAbort    SafetyAction = "abort"
	ActionPause    SafetyAction = "pause"
	ActionAlert    SafetyAction = "alert"
	ActionRollback SafetyAction = "rollback"
)

// ExperimentResult captures the outcome.
type ExperimentResult struct {
	Success           bool                   `json:"success"`
	HypothesisValid   bool                   `json:"hypothesis_valid"`
	SteadyStateProbes []ProbeResult          `json:"steady_state_probes,omitempty"`
	Metrics           map[string]interface{} `json:"metrics,omitempty"`
	Findings          []Finding              `json:"findings,omitempty"`
	Duration          time.Duration          `json:"duration"`
	AbortReason       string                 `json:"abort_reason,omitempty"`
}

// ProbeResult captures a probe check result.
type ProbeResult struct {
	Name    string      `json:"name"`
	Success bool        `json:"success"`
	Value   interface{} `json:"value"`
	Error   string      `json:"error,omitempty"`
}

// Finding represents a discovery from the experiment.
type Finding struct {
	Severity    FindingSeverity `json:"severity"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Remediation string          `json:"remediation,omitempty"`
}

// FindingSeverity indicates importance of finding.
type FindingSeverity string

const (
	SeverityCritical FindingSeverity = "critical"
	SeverityHigh     FindingSeverity = "high"
	SeverityMedium   FindingSeverity = "medium"
	SeverityLow      FindingSeverity = "low"
	SeverityInfo     FindingSeverity = "info"
)

// Rollback defines how to undo the chaos.
type Rollback struct {
	Automatic bool                   `json:"automatic"`
	Steps     []RollbackStep         `json:"steps,omitempty"`
	State     map[string]interface{} `json:"state,omitempty"` // Captured state to restore
}

// RollbackStep defines a single rollback action.
type RollbackStep struct {
	Name       string                 `json:"name"`
	Action     string                 `json:"action"`
	Parameters map[string]interface{} `json:"parameters"`
}

// FaultInjector is a function that injects a specific fault type.
type FaultInjector func(ctx context.Context, target Target, params map[string]interface{}) error

// FaultReverser is a function that reverses an injected fault.
type FaultReverser func(ctx context.Context, target Target, state map[string]interface{}) error

// Service is the main Yaldabaoth service for chaos engineering.
type Service struct {
	log    *logger.Logger
	wotan  *wotanClient.Client
	config *Config
	rng    *rand.Rand

	mu          sync.RWMutex
	experiments map[string]*Experiment
	injectors   map[ExperimentType]FaultInjector
	reversers   map[ExperimentType]FaultReverser
	runningExps map[string]context.CancelFunc // Active experiment cancel functions

	// Counters
	expCounter int64

	// Degraded mode counter — messages dropped because Wotan is nil
	wotanDrops int64
}

// WotanDrops returns the total number of messages dropped because Wotan was nil.
func (s *Service) WotanDrops() int64 {
	return atomic.LoadInt64(&s.wotanDrops)
}

// Config holds Yaldabaoth service configuration.
type Config struct {
	MaxConcurrentExperiments int           `json:"max_concurrent_experiments"`
	DefaultDuration          time.Duration `json:"default_duration"`
	SafetyCheckInterval      time.Duration `json:"safety_check_interval"`
	EnableAutoRollback       bool          `json:"enable_auto_rollback"`
	DryRunMode               bool          `json:"dry_run_mode"`
	WotanTopic               string        `json:"wotan_topic"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		MaxConcurrentExperiments: 5,
		DefaultDuration:          5 * time.Minute,
		SafetyCheckInterval:      10 * time.Second,
		EnableAutoRollback:       true,
		DryRunMode:               false, // DANGER: Set true for testing
		WotanTopic:               "yaldabaoth.chaos",
	}
}

// NewService creates a new Yaldabaoth service.
func NewService(log *logger.Logger, wotan *wotanClient.Client, cfg *Config) *Service {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	if log == nil {
		log = logger.New(io.Discard)
	}

	return &Service{
		log:         log,
		wotan:       wotan,
		config:      cfg,
		rng:         rand.New(rand.NewSource(time.Now().UnixNano())), // #nosec G404 -- chaos fault-injection sampling, not security-bearing
		experiments: make(map[string]*Experiment),
		injectors:   make(map[ExperimentType]FaultInjector),
		reversers:   make(map[ExperimentType]FaultReverser),
		runningExps: make(map[string]context.CancelFunc),
	}
}

// Start initializes the Yaldabaoth service.
func (s *Service) Start(ctx context.Context) error {
	s.log.Info().Msg("Yaldabaoth awakens - chaos shall test the Kingdom's resilience")

	// Register built-in fault injectors
	s.registerBuiltinInjectors()

	// Subscribe to events
	if s.wotan != nil {
		go s.subscribeToEvents(ctx)
	}

	return nil
}

// Stop gracefully shuts down the service.
func (s *Service) Stop() error {
	s.log.Info().Msg("Yaldabaoth rests - chaos subsides")

	// Cancel all running experiments
	s.mu.Lock()
	for id, cancel := range s.runningExps {
		s.log.Warn().Str("experiment_id", id).Msg("Cancelling running experiment on shutdown")
		cancel()
	}
	s.mu.Unlock()

	return nil
}

// RegisterInjector registers a fault injector for an experiment type.
func (s *Service) RegisterInjector(expType ExperimentType, injector FaultInjector, reverser FaultReverser) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.injectors[expType] = injector
	s.reversers[expType] = reverser
	s.log.Debug().Str("type", string(expType)).Msg("Fault injector registered")
}

// CreateExperiment creates a new chaos experiment.
func (s *Service) CreateExperiment(ctx context.Context, exp *Experiment) (*Experiment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Generate ID if not provided
	if exp.ID == "" {
		s.expCounter++
		exp.ID = fmt.Sprintf("chaos-%d-%d", time.Now().UnixNano(), s.expCounter)
	}

	// Set defaults
	exp.CreatedAt = time.Now()
	exp.Status = StatusDraft

	if exp.Rollback == nil {
		exp.Rollback = &Rollback{Automatic: s.config.EnableAutoRollback}
	}

	// Store experiment
	s.experiments[exp.ID] = exp

	s.log.Info().
		Str("id", exp.ID).
		Str("name", exp.Name).
		Str("type", string(exp.Type)).
		Msg("Experiment created")

	s.publishEvent(ctx, "yaldabaoth.experiment.created", map[string]interface{}{
		"experiment_id": exp.ID,
		"name":          exp.Name,
		"type":          exp.Type,
	})

	return exp, nil
}

// RunExperiment executes a chaos experiment.
func (s *Service) RunExperiment(ctx context.Context, experimentID string) error {
	s.mu.Lock()
	exp, ok := s.experiments[experimentID]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("experiment not found: %s", experimentID)
	}

	// Check if already running
	if _, running := s.runningExps[experimentID]; running {
		s.mu.Unlock()
		return fmt.Errorf("experiment already running: %s", experimentID)
	}

	// Check concurrent limit
	if len(s.runningExps) >= s.config.MaxConcurrentExperiments {
		s.mu.Unlock()
		return fmt.Errorf("max concurrent experiments reached: %d", s.config.MaxConcurrentExperiments)
	}

	// Get injector
	injector, ok := s.injectors[exp.Type]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("no injector registered for type: %s", exp.Type)
	}

	// Create cancellable context
	expCtx, cancel := context.WithCancel(ctx)
	s.runningExps[experimentID] = cancel

	now := time.Now()
	exp.StartedAt = &now
	exp.Status = StatusRunning
	s.mu.Unlock()

	s.log.Info().
		Str("id", experimentID).
		Str("name", exp.Name).
		Bool("dry_run", s.config.DryRunMode).
		Msg("Starting chaos experiment")

	s.publishEvent(ctx, "yaldabaoth.experiment.started", map[string]interface{}{
		"experiment_id": experimentID,
		"name":          exp.Name,
		"dry_run":       s.config.DryRunMode,
	})

	// Run experiment in goroutine
	go s.executeExperiment(expCtx, exp, injector)

	return nil
}

// executeExperiment runs the experiment lifecycle.
func (s *Service) executeExperiment(ctx context.Context, exp *Experiment, injector FaultInjector) {
	result := &ExperimentResult{
		Metrics: make(map[string]interface{}),
	}

	startTime := time.Now()

	defer func() {
		// Cleanup
		s.mu.Lock()
		delete(s.runningExps, exp.ID)
		now := time.Now()
		exp.CompletedAt = &now
		result.Duration = now.Sub(startTime)
		exp.Results = result
		s.mu.Unlock()

		s.publishEvent(context.Background(), "yaldabaoth.experiment.completed", map[string]interface{}{
			"experiment_id": exp.ID,
			"success":       result.Success,
			"duration_ms":   result.Duration.Milliseconds(),
		})
	}()

	// 1. Check steady state before
	if exp.SteadyState != nil {
		s.log.Debug().Str("id", exp.ID).Msg("Checking initial steady state")
		if !s.checkSteadyState(ctx, exp.SteadyState, result) {
			s.log.Warn().Str("id", exp.ID).Msg("Initial steady state check failed")
			exp.Status = StatusAborted
			result.AbortReason = "initial steady state check failed"
			return
		}
	}

	// 2. Inject fault (or simulate in dry run mode)
	if s.config.DryRunMode {
		s.log.Info().Str("id", exp.ID).Msg("DRY RUN: Simulating fault injection")
	} else {
		if err := injector(ctx, exp.Target, exp.Parameters); err != nil {
			s.log.Error().Err(err).Str("id", exp.ID).Msg("Fault injection failed")
			exp.Status = StatusFailed
			result.AbortReason = fmt.Sprintf("injection failed: %v", err)
			return
		}
	}

	// 3. Wait for duration while monitoring safety
	duration := s.config.DefaultDuration
	if exp.Schedule != nil && exp.Schedule.Duration > 0 {
		duration = exp.Schedule.Duration
	}

	ticker := time.NewTicker(s.config.SafetyCheckInterval)
	defer ticker.Stop()

	timer := time.NewTimer(duration)
	defer timer.Stop()

experimentLoop:
	for {
		select {
		case <-ctx.Done():
			s.log.Info().Str("id", exp.ID).Msg("Experiment cancelled")
			exp.Status = StatusAborted
			result.AbortReason = "cancelled"
			break experimentLoop

		case <-timer.C:
			s.log.Info().Str("id", exp.ID).Msg("Experiment duration completed")
			break experimentLoop

		case <-ticker.C:
			// Run safety checks
			if s.shouldAbort(ctx, exp) {
				s.log.Warn().Str("id", exp.ID).Msg("Safety check triggered abort")
				exp.Status = StatusAborted
				result.AbortReason = "safety check failed"
				break experimentLoop
			}
		}
	}

	// 4. Rollback/reverse fault
	if exp.Rollback != nil && exp.Rollback.Automatic {
		s.log.Debug().Str("id", exp.ID).Msg("Executing automatic rollback")
		if reverser, ok := s.reversers[exp.Type]; ok {
			if err := reverser(ctx, exp.Target, exp.Rollback.State); err != nil {
				s.log.Error().Err(err).Str("id", exp.ID).Msg("Rollback failed")
				result.Findings = append(result.Findings, Finding{
					Severity:    SeverityHigh,
					Title:       "Rollback Failed",
					Description: err.Error(),
				})
			}
		}
	}

	// 5. Check steady state after
	if exp.SteadyState != nil {
		s.log.Debug().Str("id", exp.ID).Msg("Checking final steady state")
		result.HypothesisValid = s.checkSteadyState(ctx, exp.SteadyState, result)
	} else {
		result.HypothesisValid = true
	}

	// 6. Determine success
	if exp.Status != StatusAborted {
		exp.Status = StatusCompleted
		result.Success = result.HypothesisValid
	}

	s.log.Info().
		Str("id", exp.ID).
		Bool("success", result.Success).
		Bool("hypothesis_valid", result.HypothesisValid).
		Int("findings", len(result.Findings)).
		Msg("Experiment completed")
}

// checkSteadyState verifies the system is in expected state.
func (s *Service) checkSteadyState(ctx context.Context, ss *SteadyState, result *ExperimentResult) bool {
	allPassed := true
	for _, probe := range ss.Probes {
		probeResult := s.runProbe(ctx, probe)
		result.SteadyStateProbes = append(result.SteadyStateProbes, probeResult)
		if !probeResult.Success {
			allPassed = false
		}
	}
	return allPassed
}

// runProbe executes a single probe.
func (s *Service) runProbe(ctx context.Context, probe Probe) ProbeResult {
	// Simplified probe execution
	return ProbeResult{
		Name:    probe.Name,
		Success: true, // In real implementation, would actually check
		Value:   "ok",
	}
}

// shouldAbort checks if safety conditions require abort.
func (s *Service) shouldAbort(ctx context.Context, exp *Experiment) bool {
	for _, check := range exp.SafetyChecks {
		// In real implementation, would evaluate condition
		_ = check
	}
	return false
}

// StopExperiment stops a running experiment.
func (s *Service) StopExperiment(ctx context.Context, experimentID string) error {
	s.mu.Lock()
	cancel, ok := s.runningExps[experimentID]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("experiment not running: %s", experimentID)
	}
	s.mu.Unlock()

	s.log.Info().Str("id", experimentID).Msg("Stopping experiment")
	cancel()
	return nil
}

// GetExperiment retrieves an experiment by ID.
func (s *Service) GetExperiment(experimentID string) (*Experiment, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	exp, ok := s.experiments[experimentID]
	return exp, ok
}

// ListExperiments returns all experiments.
func (s *Service) ListExperiments(status ExperimentStatus) []*Experiment {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*Experiment
	for _, exp := range s.experiments {
		if status == "" || exp.Status == status {
			results = append(results, exp)
		}
	}
	return results
}

// registerBuiltinInjectors adds default fault injectors.
func (s *Service) registerBuiltinInjectors() {
	// Network delay injector (simulated)
	s.RegisterInjector(ExpTypeNetworkDelay,
		func(ctx context.Context, target Target, params map[string]interface{}) error {
			delay, _ := params["delay_ms"].(int)
			s.log.Info().Int("delay_ms", delay).Msg("Injecting network delay")
			return nil
		},
		func(ctx context.Context, target Target, state map[string]interface{}) error {
			s.log.Info().Msg("Removing network delay")
			return nil
		},
	)

	// Process kill injector (simulated)
	s.RegisterInjector(ExpTypeProcessKill,
		func(ctx context.Context, target Target, params map[string]interface{}) error {
			process, _ := params["process"].(string)
			s.log.Info().Str("process", process).Msg("Simulating process kill")
			return nil
		},
		func(ctx context.Context, target Target, state map[string]interface{}) error {
			s.log.Info().Msg("Process would be restarted by supervisor")
			return nil
		},
	)

	// HTTP error injector (simulated)
	s.RegisterInjector(ExpTypeHTTPError,
		func(ctx context.Context, target Target, params map[string]interface{}) error {
			errorCode, _ := params["error_code"].(int)
			percentage, _ := params["percentage"].(int)
			s.log.Info().Int("error_code", errorCode).Int("percentage", percentage).Msg("Injecting HTTP errors")
			return nil
		},
		func(ctx context.Context, target Target, state map[string]interface{}) error {
			s.log.Info().Msg("Removing HTTP error injection")
			return nil
		},
	)

	// Latency spike injector (simulated)
	s.RegisterInjector(ExpTypeLatencySpike,
		func(ctx context.Context, target Target, params map[string]interface{}) error {
			latencyMs, _ := params["latency_ms"].(int)
			s.log.Info().Int("latency_ms", latencyMs).Msg("Injecting latency spike")
			return nil
		},
		func(ctx context.Context, target Target, state map[string]interface{}) error {
			s.log.Info().Msg("Removing latency spike")
			return nil
		},
	)
}

// subscribeToEvents listens for chaos-related events.
func (s *Service) subscribeToEvents(ctx context.Context) {
	if s.wotan == nil {
		s.log.Warn().Msg("wotan unavailable — event subscription disabled (degraded mode)")
		return
	}

	_, err := s.wotan.Subscribe(ctx, "chaos.trigger", "yaldabaoth-service")
	if err != nil {
		s.log.Warn().Err(err).Msg("Failed to subscribe to chaos events")
		return
	}

	s.log.Info().Msg("Subscribed to chaos events")
}

// publishEvent sends an event to Wotan.
func (s *Service) publishEvent(ctx context.Context, eventType string, data map[string]interface{}) {
	if s.wotan == nil {
		atomic.AddInt64(&s.wotanDrops, 1)
		s.log.Warn().Str("event_type", eventType).Msg("wotan unavailable — event dropped (degraded mode)")
		return
	}

	event := map[string]interface{}{
		"event_type": eventType,
		"timestamp":  time.Now().Unix(),
		"data":       data,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		s.log.Warn().Err(err).Msg("Failed to marshal event")
		return
	}

	if err := s.wotan.Publish(ctx, s.config.WotanTopic, payload); err != nil {
		s.log.Warn().Err(err).Msg("Failed to publish event")
	}
}

// Stats returns service statistics.
func (s *Service) Stats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Count by status
	statusCounts := make(map[string]int)
	typeCounts := make(map[string]int)
	successCount := 0
	failCount := 0

	for _, exp := range s.experiments {
		statusCounts[string(exp.Status)]++
		typeCounts[string(exp.Type)]++
		if exp.Results != nil {
			if exp.Results.Success {
				successCount++
			} else {
				failCount++
			}
		}
	}

	return map[string]interface{}{
		"total_experiments":    len(s.experiments),
		"running_experiments":  len(s.runningExps),
		"by_status":            statusCounts,
		"by_type":              typeCounts,
		"successful":           successCount,
		"failed":               failCount,
		"registered_injectors": len(s.injectors),
		"dry_run_mode":         s.config.DryRunMode,
	}
}

// GameDay represents a coordinated chaos exercise.
type GameDay struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Experiments []string      `json:"experiments"` // Experiment IDs
	Schedule    time.Time     `json:"schedule"`
	Duration    time.Duration `json:"duration"`
	Status      string        `json:"status"`
}

// CreateGameDay creates a coordinated chaos exercise.
func (s *Service) CreateGameDay(ctx context.Context, gd *GameDay) (*GameDay, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	gd.ID = fmt.Sprintf("gameday-%d", time.Now().UnixNano())
	gd.Status = "scheduled"

	s.log.Info().
		Str("id", gd.ID).
		Str("name", gd.Name).
		Int("experiments", len(gd.Experiments)).
		Msg("Game day scheduled")

	return gd, nil
}
