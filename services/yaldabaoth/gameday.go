// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package yaldabaoth implements the chaos engineering subsystem for
// the Unheaded Kingdom. Named after the Gnostic Demiurge, Yaldabaoth
// creates controlled disorder to test system resilience.
//
// This package provides the GameDay scheduler, audit trail, and shared
// types used by the chaos-controller binary (cmd/chaos-controller).
package yaldabaoth

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"unheaded/pkg/logger"
	wotanClient "unheaded/pkg/wotan-client"
)

// ── State machine ────────────────────────────────────────────────────────

// GameDayState represents the current state of a GameDay scenario.
type GameDayState string

const (
	StateIdle      GameDayState = "IDLE"
	StateRunning   GameDayState = "RUNNING"
	StateCompleted GameDayState = "COMPLETED"
	StateAborted   GameDayState = "ABORTED"
)

// ── Types ────────────────────────────────────────────────────────────────

// FaultRule defines a chaos fault rule used within GameDay stages.
type FaultRule struct {
	SrcServiceID uint8         `json:"src_service_id"`
	DstServiceID uint8         `json:"dst_service_id"`
	FaultType    string        `json:"fault_type"` // DROP, DELAY, CORRUPT, DUPLICATE
	DropRatePPM  uint32        `json:"drop_rate_ppm"`
	DelayUS      uint64        `json:"delay_us"`
	Duration     time.Duration `json:"duration_ns"`
}

// Stage represents a single stage in a GameDay scenario.
type Stage struct {
	Name              string        `json:"name"`
	Delay             time.Duration `json:"delay_ns"`
	Rules             []FaultRule   `json:"rules"`
	RollbackOnFailure bool          `json:"rollback_on_failure"`
}

// GameDayScenario defines a complete GameDay scenario with multiple stages.
type GameDayScenario struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Duration    time.Duration `json:"duration_ns"`
	Stages      []Stage       `json:"stages"`
}

// GameDayStatus captures the current state of a running GameDay.
type GameDayStatus struct {
	State        GameDayState `json:"state"`
	ScenarioName string       `json:"scenario_name,omitempty"`
	CurrentStage int          `json:"current_stage"`
	TotalStages  int          `json:"total_stages"`
	StartedAt    *time.Time   `json:"started_at,omitempty"`
	CompletedAt  *time.Time   `json:"completed_at,omitempty"`
	Error        string       `json:"error,omitempty"`
}

// ── GameDayScheduler ─────────────────────────────────────────────────────

// GameDayScheduler manages the lifecycle of GameDay chaos scenarios.
// It implements a state machine: IDLE -> RUNNING -> stage transitions ->
// COMPLETED/ABORTED.
type GameDayScheduler struct {
	log    *logger.Logger
	wotan  *wotanClient.Client

	mu           sync.RWMutex
	state        GameDayState
	scenario     *GameDayScenario
	currentStage int
	startedAt    *time.Time
	completedAt  *time.Time
	lastError    string

	cancelFn context.CancelFunc
}

// NewGameDayScheduler creates a new GameDayScheduler.
func NewGameDayScheduler(log *logger.Logger, wotan *wotanClient.Client) *GameDayScheduler {
	return &GameDayScheduler{
		log:   log,
		wotan: wotan,
		state: StateIdle,
	}
}

// Schedule validates and starts a GameDay scenario. Returns an error if
// a scenario is already running or the scenario is invalid.
func (gds *GameDayScheduler) Schedule(scenario GameDayScenario) error {
	gds.mu.Lock()
	defer gds.mu.Unlock()

	if gds.state == StateRunning {
		return fmt.Errorf("GameDay already running: %s", gds.scenario.Name)
	}

	// Validate scenario.
	if scenario.Name == "" {
		return fmt.Errorf("scenario name is required")
	}
	if len(scenario.Stages) == 0 {
		return fmt.Errorf("scenario must have at least one stage")
	}
	if scenario.Duration <= 0 {
		return fmt.Errorf("scenario duration must be positive")
	}
	for i, stage := range scenario.Stages {
		if stage.Name == "" {
			return fmt.Errorf("stage %d: name is required", i)
		}
		if len(stage.Rules) == 0 {
			return fmt.Errorf("stage %d (%s): must have at least one rule", i, stage.Name)
		}
	}

	now := time.Now()
	gds.scenario = &scenario
	gds.state = StateRunning
	gds.currentStage = 0
	gds.startedAt = &now
	gds.completedAt = nil
	gds.lastError = ""

	ctx, cancel := context.WithTimeout(context.Background(), scenario.Duration)
	gds.cancelFn = cancel

	// Publish schedule event.
	gds.publishEvent(ctx, "chaos.schedules", map[string]interface{}{
		"event_type": "gameday.started",
		"scenario":   scenario.Name,
		"stages":     len(scenario.Stages),
		"duration":   scenario.Duration.Milliseconds(),
		"timestamp":  now.UnixMilli(),
	})

	gds.log.Info().
		Str("scenario", scenario.Name).
		Int("stages", len(scenario.Stages)).
		Dur("duration", scenario.Duration).
		Msg("GameDay scenario scheduled")

	// Run stages in background.
	go gds.runScenario(ctx, scenario)

	return nil
}

// runScenario executes all stages sequentially.
func (gds *GameDayScheduler) runScenario(ctx context.Context, scenario GameDayScenario) {
	defer func() {
		gds.mu.Lock()
		if gds.state == StateRunning {
			gds.state = StateCompleted
			now := time.Now()
			gds.completedAt = &now
		}
		gds.mu.Unlock()

		gds.publishEvent(context.Background(), "chaos.schedules", map[string]interface{}{
			"event_type": "gameday.finished",
			"scenario":   scenario.Name,
			"state":      gds.state,
			"timestamp":  time.Now().UnixMilli(),
		})

		gds.log.Info().
			Str("scenario", scenario.Name).
			Str("state", string(gds.state)).
			Msg("GameDay scenario finished")
	}()

	for i, stage := range scenario.Stages {
		// Check if scenario was aborted.
		select {
		case <-ctx.Done():
			gds.mu.Lock()
			gds.state = StateAborted
			gds.lastError = "context cancelled"
			gds.mu.Unlock()
			return
		default:
		}

		gds.mu.Lock()
		gds.currentStage = i
		gds.mu.Unlock()

		gds.log.Info().
			Str("scenario", scenario.Name).
			Str("stage", stage.Name).
			Int("stage_index", i).
			Msg("executing GameDay stage")

		if err := gds.executeStage(ctx, stage); err != nil {
			gds.log.Error().
				Err(err).
				Str("stage", stage.Name).
				Msg("GameDay stage failed")

			if stage.RollbackOnFailure {
				gds.log.Warn().Str("stage", stage.Name).Msg("rolling back stage rules")
				// In a real implementation, we would undo the injected rules here.
			}

			gds.mu.Lock()
			gds.state = StateAborted
			gds.lastError = fmt.Sprintf("stage %s failed: %v", stage.Name, err)
			gds.mu.Unlock()

			gds.publishEvent(context.Background(), "chaos.schedules", map[string]interface{}{
				"event_type": "gameday.stage.failed",
				"scenario":   scenario.Name,
				"stage":      stage.Name,
				"error":      err.Error(),
				"timestamp":  time.Now().UnixMilli(),
			})
			return
		}

		gds.publishEvent(ctx, "chaos.schedules", map[string]interface{}{
			"event_type": "gameday.stage.completed",
			"scenario":   scenario.Name,
			"stage":      stage.Name,
			"index":      i,
			"timestamp":  time.Now().UnixMilli(),
		})
	}
}

// executeStage applies the fault rules for a single stage with the
// configured delay before activation.
func (gds *GameDayScheduler) executeStage(ctx context.Context, stage Stage) error {
	// Wait for the stage's delay before applying rules.
	if stage.Delay > 0 {
		gds.log.Info().
			Str("stage", stage.Name).
			Dur("delay", stage.Delay).
			Msg("waiting before stage activation")

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(stage.Delay):
		}
	}

	// Apply each rule in the stage.
	for i, rule := range stage.Rules {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		gds.log.Info().
			Str("stage", stage.Name).
			Int("rule_index", i).
			Str("fault_type", rule.FaultType).
			Uint8("src_svc", rule.SrcServiceID).
			Uint8("dst_svc", rule.DstServiceID).
			Msg("applying fault rule")

		// Publish the rule as a chaos injection event.
		gds.publishEvent(ctx, "chaos.injections", map[string]interface{}{
			"event_type":     "chaos.rule.injected",
			"stage":          stage.Name,
			"src_service_id": rule.SrcServiceID,
			"dst_service_id": rule.DstServiceID,
			"fault_type":     rule.FaultType,
			"drop_rate_ppm":  rule.DropRatePPM,
			"delay_us":       rule.DelayUS,
			"duration_ns":    rule.Duration.Nanoseconds(),
			"timestamp":      time.Now().UnixMilli(),
		})
	}

	// Wait for the maximum rule duration in this stage.
	maxDuration := gds.maxRuleDuration(stage.Rules)
	if maxDuration > 0 {
		gds.log.Info().
			Str("stage", stage.Name).
			Dur("rule_duration", maxDuration).
			Msg("waiting for stage rules to complete")

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(maxDuration):
		}
	}

	return nil
}

// maxRuleDuration returns the longest duration among a set of rules.
func (gds *GameDayScheduler) maxRuleDuration(rules []FaultRule) time.Duration {
	var max time.Duration
	for _, r := range rules {
		if r.Duration > max {
			max = r.Duration
		}
	}
	return max
}

// Stop cancels the running scenario and cleans up.
func (gds *GameDayScheduler) Stop() {
	gds.mu.Lock()
	defer gds.mu.Unlock()

	if gds.cancelFn != nil {
		gds.cancelFn()
		gds.cancelFn = nil
	}

	if gds.state == StateRunning {
		gds.state = StateAborted
		now := time.Now()
		gds.completedAt = &now
		gds.lastError = "stopped by operator"
		gds.log.Warn().Str("scenario", gds.scenario.Name).Msg("GameDay stopped by operator")
	}
}

// Status returns the current GameDay status.
func (gds *GameDayScheduler) Status() GameDayStatus {
	gds.mu.RLock()
	defer gds.mu.RUnlock()

	status := GameDayStatus{
		State:       gds.state,
		StartedAt:   gds.startedAt,
		CompletedAt: gds.completedAt,
		Error:       gds.lastError,
	}

	if gds.scenario != nil {
		status.ScenarioName = gds.scenario.Name
		status.TotalStages = len(gds.scenario.Stages)
		status.CurrentStage = gds.currentStage
	}

	return status
}

// publishEvent publishes an event to a Wotan topic (best-effort).
func (gds *GameDayScheduler) publishEvent(ctx context.Context, topic string, event interface{}) {
	if gds.wotan == nil {
		return
	}

	payload, err := json.Marshal(event)
	if err != nil {
		gds.log.Warn().Err(err).Msg("failed to marshal GameDay event")
		return
	}

	pubCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := gds.wotan.Publish(pubCtx, topic, payload); err != nil {
		gds.log.Warn().Err(err).Str("topic", topic).Msg("failed to publish GameDay event")
	}
}
