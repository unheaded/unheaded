// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package yaldabaoth

import (
	"testing"
	"time"
)

func TestNewGameDayScheduler(t *testing.T) {
	gds := NewGameDayScheduler(testLogger(), nil)
	if gds == nil {
		t.Fatal("expected non-nil scheduler")
	}
	if gds.state != StateIdle {
		t.Errorf("expected initial state IDLE, got %s", gds.state)
	}
}

func TestGameDaySchedulerScheduleValidation(t *testing.T) {
	tests := []struct {
		name     string
		scenario GameDayScenario
		wantErr  bool
	}{
		{
			name:     "empty name",
			scenario: GameDayScenario{Duration: time.Second, Stages: []Stage{{Name: "s1", Rules: []FaultRule{{FaultType: "DROP"}}}}},
			wantErr:  true,
		},
		{
			name:     "no stages",
			scenario: GameDayScenario{Name: "test", Duration: time.Second},
			wantErr:  true,
		},
		{
			name:     "zero duration",
			scenario: GameDayScenario{Name: "test", Stages: []Stage{{Name: "s1", Rules: []FaultRule{{FaultType: "DROP"}}}}},
			wantErr:  true,
		},
		{
			name: "empty stage name",
			scenario: GameDayScenario{
				Name:     "test",
				Duration: time.Second,
				Stages:   []Stage{{Name: "", Rules: []FaultRule{{FaultType: "DROP"}}}},
			},
			wantErr: true,
		},
		{
			name: "stage with no rules",
			scenario: GameDayScenario{
				Name:     "test",
				Duration: time.Second,
				Stages:   []Stage{{Name: "s1"}},
			},
			wantErr: true,
		},
		{
			name: "valid scenario",
			scenario: GameDayScenario{
				Name:     "test",
				Duration: 100 * time.Millisecond,
				Stages: []Stage{
					{
						Name:  "stage-1",
						Rules: []FaultRule{{FaultType: "DROP", SrcServiceID: 1, DstServiceID: 2, Duration: 10 * time.Millisecond}},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset scheduler between tests
			sched := NewGameDayScheduler(testLogger(), nil)
			err := sched.Schedule(tt.scenario)
			if tt.wantErr && err == nil {
				t.Error("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.wantErr {
				// Wait for completion
				time.Sleep(200 * time.Millisecond)
			}
		})
	}
}

func TestGameDaySchedulerAlreadyRunning(t *testing.T) {
	gds := NewGameDayScheduler(testLogger(), nil)

	scenario := GameDayScenario{
		Name:     "long-running",
		Duration: 5 * time.Second,
		Stages: []Stage{
			{
				Name:  "s1",
				Rules: []FaultRule{{FaultType: "DROP", Duration: 1 * time.Second}},
			},
		},
	}

	err := gds.Schedule(scenario)
	if err != nil {
		t.Fatal(err)
	}

	// Try scheduling while running
	err = gds.Schedule(scenario)
	if err == nil {
		t.Error("expected error when already running")
	}

	gds.Stop()
	time.Sleep(50 * time.Millisecond)
}

func TestGameDaySchedulerRunAndComplete(t *testing.T) {
	gds := NewGameDayScheduler(testLogger(), nil)

	scenario := GameDayScenario{
		Name:     "quick-test",
		Duration: 500 * time.Millisecond,
		Stages: []Stage{
			{
				Name:  "stage-1",
				Rules: []FaultRule{{FaultType: "DROP", Duration: 10 * time.Millisecond}},
			},
			{
				Name:  "stage-2",
				Rules: []FaultRule{{FaultType: "DELAY", DelayUS: 100, Duration: 10 * time.Millisecond}},
			},
		},
	}

	err := gds.Schedule(scenario)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for completion
	time.Sleep(600 * time.Millisecond)

	status := gds.Status()
	if status.State != StateCompleted {
		t.Errorf("expected state COMPLETED, got %s (error: %s)", status.State, status.Error)
	}
	if status.ScenarioName != "quick-test" {
		t.Errorf("expected scenario name quick-test, got %s", status.ScenarioName)
	}
	if status.TotalStages != 2 {
		t.Errorf("expected 2 total stages, got %d", status.TotalStages)
	}
}

func TestGameDaySchedulerStop(t *testing.T) {
	gds := NewGameDayScheduler(testLogger(), nil)

	scenario := GameDayScenario{
		Name:     "stoppable",
		Duration: 10 * time.Second,
		Stages: []Stage{
			{
				Name:  "long-stage",
				Rules: []FaultRule{{FaultType: "DROP", Duration: 5 * time.Second}},
			},
		},
	}

	_ = gds.Schedule(scenario)
	time.Sleep(50 * time.Millisecond)

	gds.Stop()
	time.Sleep(100 * time.Millisecond)

	status := gds.Status()
	if status.State != StateAborted {
		t.Errorf("expected state ABORTED, got %s", status.State)
	}
}

func TestGameDaySchedulerStopWhenIdle(t *testing.T) {
	gds := NewGameDayScheduler(testLogger(), nil)
	// Should not panic
	gds.Stop()

	status := gds.Status()
	if status.State != StateIdle {
		t.Errorf("expected state IDLE, got %s", status.State)
	}
}

func TestGameDaySchedulerStatus(t *testing.T) {
	gds := NewGameDayScheduler(testLogger(), nil)

	t.Run("idle status", func(t *testing.T) {
		status := gds.Status()
		if status.State != StateIdle {
			t.Errorf("expected IDLE, got %s", status.State)
		}
		if status.ScenarioName != "" {
			t.Error("expected empty scenario name when idle")
		}
	})

	t.Run("running status", func(t *testing.T) {
		scenario := GameDayScenario{
			Name:     "status-test",
			Duration: 5 * time.Second,
			Stages: []Stage{
				{
					Name:  "s1",
					Rules: []FaultRule{{FaultType: "DROP", Duration: 1 * time.Second}},
				},
			},
		}
		_ = gds.Schedule(scenario)
		time.Sleep(20 * time.Millisecond)

		status := gds.Status()
		if status.ScenarioName != "status-test" {
			t.Errorf("expected scenario name status-test, got %s", status.ScenarioName)
		}
		if status.StartedAt == nil {
			t.Error("expected StartedAt to be set")
		}

		gds.Stop()
		time.Sleep(50 * time.Millisecond)
	})
}

func TestGameDaySchedulerStageWithDelay(t *testing.T) {
	gds := NewGameDayScheduler(testLogger(), nil)

	scenario := GameDayScenario{
		Name:     "delayed",
		Duration: 1 * time.Second,
		Stages: []Stage{
			{
				Name:  "delayed-stage",
				Delay: 10 * time.Millisecond,
				Rules: []FaultRule{{FaultType: "DELAY", Duration: 10 * time.Millisecond}},
			},
		},
	}

	err := gds.Schedule(scenario)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	status := gds.Status()
	if status.State != StateCompleted {
		t.Errorf("expected COMPLETED, got %s (error: %s)", status.State, status.Error)
	}
}

func TestMaxRuleDuration(t *testing.T) {
	gds := NewGameDayScheduler(testLogger(), nil)

	t.Run("empty rules", func(t *testing.T) {
		d := gds.maxRuleDuration(nil)
		if d != 0 {
			t.Errorf("expected 0, got %v", d)
		}
	})

	t.Run("finds max", func(t *testing.T) {
		rules := []FaultRule{
			{Duration: 10 * time.Millisecond},
			{Duration: 50 * time.Millisecond},
			{Duration: 30 * time.Millisecond},
		}
		d := gds.maxRuleDuration(rules)
		if d != 50*time.Millisecond {
			t.Errorf("expected 50ms, got %v", d)
		}
	})
}

func TestGameDayStateConstants(t *testing.T) {
	states := []GameDayState{StateIdle, StateRunning, StateCompleted, StateAborted}
	for _, s := range states {
		if s == "" {
			t.Error("state constant should not be empty")
		}
	}
}
