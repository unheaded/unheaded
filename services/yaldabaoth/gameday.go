// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package yaldabaoth implements the chaos engineering subsystem for
// the Unheaded Kingdom. Named after the Gnostic Demiurge, Yaldabaoth
// creates controlled disorder to test system resilience.
//
// This file is the GameDay scheduler: a scenario is a list of stages, and
// each stage's experiments run one after another through the Service, so
// every fault of a game day passes the same safety envelope as any other
// (D-16). A stage fails when one of its experiments is refused or does not
// succeed; the scenario then stops. Nothing needs undoing here: each
// experiment reverts its own fault through the journal.
package yaldabaoth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"unheaded/pkg/logger"
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

// FaultRule is the older src/dst service fault rule (DROP, DELAY, CORRUPT,
// DUPLICATE). It runs as the experiment RuleExperiment makes of it, which
// names no allowlisted target and is therefore refused: there is no packet
// injector for service pairs (packet chaos is Phase 13, on lab interfaces).
type FaultRule struct {
	SrcServiceID uint8         `json:"src_service_id"`
	DstServiceID uint8         `json:"dst_service_id"`
	FaultType    string        `json:"fault_type"` // DROP, DELAY, CORRUPT, DUPLICATE
	DropRatePPM  uint32        `json:"drop_rate_ppm"`
	DelayUS      uint64        `json:"delay_us"`
	Duration     time.Duration `json:"duration_ns"`
}

// RuleExperiment translates a FaultRule into an experiment.
func RuleExperiment(r FaultRule) *Experiment {
	e := &Experiment{
		Name:     fmt.Sprintf("rule %s %d->%d", r.FaultType, r.SrcServiceID, r.DstServiceID),
		Target:   Target{Kind: KindPacket, ID: fmt.Sprintf("svc%d-svc%d", r.SrcServiceID, r.DstServiceID)},
		Schedule: &Schedule{Duration: r.Duration},
	}
	switch r.FaultType {
	case "DROP":
		e.Type = ExpTypeNetworkLoss
		e.Parameters = Params{"loss_percent": float64(r.DropRatePPM) / 10000}
	case "DELAY":
		e.Type = ExpTypeNetworkDelay
		e.Parameters = Params{"delay_ms": float64(r.DelayUS) / 1000}
	default:
		e.Type = ExpTypeCustom
	}
	return e
}

// Stage represents a single stage in a GameDay scenario.
type Stage struct {
	Name        string        `json:"name"`
	Delay       time.Duration `json:"delay_ns"`
	Experiments []Experiment  `json:"experiments,omitempty"`
	Rules       []FaultRule   `json:"rules,omitempty"`
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
	Experiments  []string     `json:"experiments,omitempty"`
	Error        string       `json:"error,omitempty"`
}

// Runner runs one experiment to its end (the Service).
type Runner interface {
	RunAndWait(ctx context.Context, exp *Experiment) (*Experiment, error)
}

// ── GameDayScheduler ─────────────────────────────────────────────────────

// GameDayScheduler runs one GameDay scenario at a time.
type GameDayScheduler struct {
	log    *logger.Logger
	runner Runner
	pub    Publisher

	mu           sync.RWMutex
	state        GameDayState
	scenario     *GameDayScenario
	currentStage int
	startedAt    *time.Time
	completedAt  *time.Time
	lastError    string
	experiments  []string

	cancelFn context.CancelFunc
	done     chan struct{}
}

// NewGameDayScheduler creates a scheduler that runs experiments through r.
func NewGameDayScheduler(log *logger.Logger, r Runner, pub Publisher) *GameDayScheduler {
	if log == nil {
		log = logger.New(io.Discard)
	}
	return &GameDayScheduler{log: log, runner: r, pub: pub, state: StateIdle}
}

func validateScenario(sc GameDayScenario) error {
	if sc.Name == "" {
		return fmt.Errorf("scenario name is required")
	}
	if len(sc.Stages) == 0 {
		return fmt.Errorf("scenario must have at least one stage")
	}
	if sc.Duration <= 0 {
		return fmt.Errorf("scenario duration must be positive")
	}
	for i, st := range sc.Stages {
		if st.Name == "" {
			return fmt.Errorf("stage %d: name is required", i)
		}
		if len(st.Experiments) == 0 && len(st.Rules) == 0 {
			return fmt.Errorf("stage %d (%s): must have at least one experiment", i, st.Name)
		}
	}
	return nil
}

// Schedule validates and starts a scenario.
func (g *GameDayScheduler) Schedule(sc GameDayScenario) error {
	if err := validateScenario(sc); err != nil {
		return err
	}
	if g.runner == nil {
		return fmt.Errorf("no chaos engine to run the game day")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.state == StateRunning {
		return fmt.Errorf("GameDay already running: %s", g.scenario.Name)
	}
	now := time.Now().UTC()
	g.scenario, g.state, g.currentStage = &sc, StateRunning, 0
	g.startedAt, g.completedAt, g.lastError, g.experiments = &now, nil, "", nil
	ctx, cancel := context.WithTimeout(context.Background(), sc.Duration)
	g.cancelFn = cancel
	g.done = make(chan struct{})
	g.publish("gameday.started", map[string]any{"scenario": sc.Name, "stages": len(sc.Stages)})
	go g.runScenario(ctx, sc, g.done)
	return nil
}

func (g *GameDayScheduler) runScenario(ctx context.Context, sc GameDayScenario, done chan struct{}) {
	defer close(done)
	fail := func(msg string) {
		g.mu.Lock()
		g.state, g.lastError = StateAborted, msg
		now := time.Now().UTC()
		g.completedAt = &now
		g.mu.Unlock()
		g.publish("gameday.stage.failed", map[string]any{"scenario": sc.Name, "error": msg})
	}
	for i, st := range sc.Stages {
		g.mu.Lock()
		g.currentStage = i
		g.mu.Unlock()
		if st.Delay > 0 {
			t := time.NewTimer(st.Delay)
			select {
			case <-ctx.Done():
				t.Stop()
				fail("stopped before stage " + st.Name)
				return
			case <-t.C:
			}
		}
		exps := make([]*Experiment, 0, len(st.Experiments)+len(st.Rules))
		for j := range st.Experiments {
			exps = append(exps, &st.Experiments[j])
		}
		for _, r := range st.Rules {
			exps = append(exps, RuleExperiment(r))
		}
		for _, e := range exps {
			if ctx.Err() != nil {
				fail("stopped in stage " + st.Name)
				return
			}
			got, err := g.runner.RunAndWait(ctx, e)
			if got != nil {
				g.mu.Lock()
				g.experiments = append(g.experiments, got.ID)
				g.mu.Unlock()
			}
			switch {
			case err != nil:
				fail(fmt.Sprintf("stage %s: %v", st.Name, err))
				return
			case ctx.Err() != nil:
				fail("stopped in stage " + st.Name)
				return
			case got.Results == nil || !got.Results.Success:
				reason := string(got.Status)
				if got.Results != nil && got.Results.AbortReason != "" {
					reason += ": " + got.Results.AbortReason
				}
				fail(fmt.Sprintf("stage %s: experiment %s did not succeed (%s)", st.Name, got.ID, reason))
				return
			}
		}
		g.publish("gameday.stage.completed", map[string]any{"scenario": sc.Name, "stage": st.Name, "index": i})
	}
	g.mu.Lock()
	g.state = StateCompleted
	now := time.Now().UTC()
	g.completedAt = &now
	g.mu.Unlock()
	g.publish("gameday.finished", map[string]any{"scenario": sc.Name})
}

// Stop cancels the running scenario; its running experiment is aborted
// and reverted by the Service.
func (g *GameDayScheduler) Stop() {
	g.mu.Lock()
	cancel, done := g.cancelFn, g.done
	g.cancelFn = nil
	g.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

// Status returns the current GameDay status.
func (g *GameDayScheduler) Status() GameDayStatus {
	g.mu.RLock()
	defer g.mu.RUnlock()
	st := GameDayStatus{State: g.state, StartedAt: g.startedAt, CompletedAt: g.completedAt, Error: g.lastError,
		Experiments: append([]string(nil), g.experiments...)}
	if g.scenario != nil {
		st.ScenarioName, st.TotalStages, st.CurrentStage = g.scenario.Name, len(g.scenario.Stages), g.currentStage
	}
	return st
}

func (g *GameDayScheduler) publish(event string, data map[string]any) {
	if g.pub == nil {
		return
	}
	data["event_type"] = event
	data["timestamp"] = time.Now().UnixMilli()
	b, err := json.Marshal(data)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := g.pub.Publish(ctx, "chaos.schedules", b); err != nil {
		g.log.Warn().Err(err).Msg("GameDay event not published")
	}
}
