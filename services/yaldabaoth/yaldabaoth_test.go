// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package yaldabaoth

import (
	"context"
	"testing"
	"time"
)

// ============================================================================
// Configuration Tests
// ============================================================================

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.MaxConcurrentExperiments != 5 {
		t.Errorf("expected MaxConcurrentExperiments 5, got %d", cfg.MaxConcurrentExperiments)
	}
	if cfg.DefaultDuration != 5*time.Minute {
		t.Errorf("expected DefaultDuration 5m, got %v", cfg.DefaultDuration)
	}
	if cfg.SafetyCheckInterval != 10*time.Second {
		t.Errorf("expected SafetyCheckInterval 10s, got %v", cfg.SafetyCheckInterval)
	}
	if !cfg.EnableAutoRollback {
		t.Error("expected EnableAutoRollback true")
	}
	if cfg.DryRunMode {
		t.Error("expected DryRunMode false by default")
	}
}

func TestNewService(t *testing.T) {
	svc := NewService(nil, nil, nil)
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
	if svc.experiments == nil {
		t.Error("experiments map not initialized")
	}
	if svc.injectors == nil {
		t.Error("injectors map not initialized")
	}
	if svc.runningExps == nil {
		t.Error("runningExps map not initialized")
	}
}

// ============================================================================
// Experiment Creation Tests
// ============================================================================

func TestCreateExperiment_Basic(t *testing.T) {
	svc := NewService(nil, nil, nil)

	exp := &Experiment{
		Name:        "network-delay-test",
		Description: "Test network delay handling",
		Type:        ExpTypeNetworkDelay,
		Target: Target{
			Type:     TargetService,
			Selector: map[string]string{"app": "test-service"},
		},
		Parameters: map[string]interface{}{
			"delay_ms": 100,
		},
	}

	result, err := svc.CreateExperiment(context.Background(), exp)
	if err != nil {
		t.Fatalf("CreateExperiment failed: %v", err)
	}

	if result.ID == "" {
		t.Error("expected ID to be generated")
	}
	if result.Status != StatusDraft {
		t.Errorf("expected status Draft, got %s", result.Status)
	}
	if result.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
	if result.Rollback == nil {
		t.Error("expected Rollback to be initialized")
	}
}

func TestCreateExperiment_WithSchedule(t *testing.T) {
	svc := NewService(nil, nil, nil)

	startTime := time.Now().Add(1 * time.Hour)
	exp := &Experiment{
		Name: "scheduled-test",
		Type: ExpTypeProcessKill,
		Target: Target{
			Type:     TargetContainer,
			Selector: map[string]string{"name": "test-container"},
		},
		Schedule: &Schedule{
			Type:      ScheduleOnce,
			StartTime: &startTime,
			Duration:  5 * time.Minute,
		},
	}

	result, _ := svc.CreateExperiment(context.Background(), exp)

	if result.Schedule == nil {
		t.Error("expected Schedule to be preserved")
	}
	if result.Schedule.Duration != 5*time.Minute {
		t.Errorf("expected duration 5m, got %v", result.Schedule.Duration)
	}
}

func TestCreateExperiment_WithSteadyState(t *testing.T) {
	svc := NewService(nil, nil, nil)

	exp := &Experiment{
		Name: "steady-state-test",
		Type: ExpTypeHTTPError,
		Target: Target{
			Type: TargetService,
		},
		SteadyState: &SteadyState{
			Hypothesis: "Service remains responsive during HTTP error injection",
			Probes: []Probe{
				{
					Name:     "health-check",
					Type:     ProbeHTTP,
					Provider: "http",
					Config:   map[string]interface{}{"url": "/health", "expected_status": 200},
				},
			},
			Tolerance: 5.0,
			Timeout:   30 * time.Second,
		},
	}

	result, _ := svc.CreateExperiment(context.Background(), exp)

	if result.SteadyState == nil {
		t.Error("expected SteadyState to be preserved")
	}
	if len(result.SteadyState.Probes) != 1 {
		t.Errorf("expected 1 probe, got %d", len(result.SteadyState.Probes))
	}
}

// ============================================================================
// Experiment Retrieval Tests
// ============================================================================

func TestGetExperiment(t *testing.T) {
	svc := NewService(nil, nil, nil)

	exp := &Experiment{
		Name:   "get-test",
		Type:   ExpTypeNetworkDelay,
		Target: Target{Type: TargetService},
	}

	created, _ := svc.CreateExperiment(context.Background(), exp)

	retrieved, ok := svc.GetExperiment(created.ID)
	if !ok {
		t.Error("expected to find experiment")
	}
	if retrieved.Name != "get-test" {
		t.Errorf("expected name 'get-test', got %s", retrieved.Name)
	}
}

func TestGetExperiment_NotFound(t *testing.T) {
	svc := NewService(nil, nil, nil)

	_, ok := svc.GetExperiment("nonexistent")
	if ok {
		t.Error("expected not to find experiment")
	}
}

func TestListExperiments(t *testing.T) {
	svc := NewService(nil, nil, nil)
	ctx := context.Background()

	// Create experiments with different types
	svc.CreateExperiment(ctx, &Experiment{Name: "exp1", Type: ExpTypeNetworkDelay, Target: Target{Type: TargetService}})
	svc.CreateExperiment(ctx, &Experiment{Name: "exp2", Type: ExpTypeProcessKill, Target: Target{Type: TargetContainer}})
	svc.CreateExperiment(ctx, &Experiment{Name: "exp3", Type: ExpTypeHTTPError, Target: Target{Type: TargetService}})

	// List all
	all := svc.ListExperiments("")
	if len(all) != 3 {
		t.Errorf("expected 3 experiments, got %d", len(all))
	}

	// List by status
	drafts := svc.ListExperiments(StatusDraft)
	if len(drafts) != 3 {
		t.Errorf("expected 3 draft experiments, got %d", len(drafts))
	}
}

// ============================================================================
// Injector Registration Tests
// ============================================================================

func TestRegisterInjector(t *testing.T) {
	svc := NewService(nil, nil, nil)

	svc.RegisterInjector(ExpTypeCustom,
		func(ctx context.Context, target Target, params map[string]interface{}) error {
			return nil
		},
		func(ctx context.Context, target Target, state map[string]interface{}) error {
			return nil
		},
	)

	// Verify injector was registered
	svc.mu.RLock()
	_, hasInjector := svc.injectors[ExpTypeCustom]
	_, hasReverser := svc.reversers[ExpTypeCustom]
	svc.mu.RUnlock()

	if !hasInjector {
		t.Error("expected injector to be registered")
	}
	if !hasReverser {
		t.Error("expected reverser to be registered")
	}
}

// ============================================================================
// Run Experiment Tests
// ============================================================================

func TestRunExperiment_DryRun(t *testing.T) {
	cfg := &Config{
		DryRunMode:               true,
		DefaultDuration:          100 * time.Millisecond,
		SafetyCheckInterval:      50 * time.Millisecond,
		EnableAutoRollback:       true,
		MaxConcurrentExperiments: 5,
	}
	svc := NewService(nil, nil, cfg)
	ctx := context.Background()

	// Register built-in injectors
	svc.Start(ctx)

	exp := &Experiment{
		Name: "dry-run-test",
		Type: ExpTypeNetworkDelay,
		Target: Target{
			Type:     TargetService,
			Selector: map[string]string{"app": "test"},
		},
		Parameters: map[string]interface{}{
			"delay_ms": 100,
		},
		Schedule: &Schedule{
			Duration: 50 * time.Millisecond,
		},
	}

	created, _ := svc.CreateExperiment(ctx, exp)

	err := svc.RunExperiment(ctx, created.ID)
	if err != nil {
		t.Fatalf("RunExperiment failed: %v", err)
	}

	// Wait for experiment to complete
	time.Sleep(200 * time.Millisecond)

	result, _ := svc.GetExperiment(created.ID)
	if result.Status != StatusCompleted {
		t.Errorf("expected status Completed, got %s", result.Status)
	}
	if result.Results == nil {
		t.Error("expected Results to be set")
	}
}

func TestRunExperiment_NotFound(t *testing.T) {
	svc := NewService(nil, nil, nil)

	err := svc.RunExperiment(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent experiment")
	}
}

func TestRunExperiment_AlreadyRunning(t *testing.T) {
	cfg := &Config{
		DryRunMode:               true,
		DefaultDuration:          1 * time.Second,
		SafetyCheckInterval:      50 * time.Millisecond,
		MaxConcurrentExperiments: 5,
	}
	svc := NewService(nil, nil, cfg)
	svc.Start(context.Background())

	exp := &Experiment{
		Name:   "already-running-test",
		Type:   ExpTypeNetworkDelay,
		Target: Target{Type: TargetService},
	}

	created, _ := svc.CreateExperiment(context.Background(), exp)

	// Start experiment
	svc.RunExperiment(context.Background(), created.ID)

	// Try to start again
	err := svc.RunExperiment(context.Background(), created.ID)
	if err == nil {
		t.Error("expected error for already running experiment")
	}
}

func TestRunExperiment_MaxConcurrent(t *testing.T) {
	cfg := &Config{
		DryRunMode:               true,
		DefaultDuration:          1 * time.Second,
		SafetyCheckInterval:      50 * time.Millisecond,
		MaxConcurrentExperiments: 1,
	}
	svc := NewService(nil, nil, cfg)
	svc.Start(context.Background())
	ctx := context.Background()

	// Create two experiments
	exp1 := &Experiment{Name: "exp1", Type: ExpTypeNetworkDelay, Target: Target{Type: TargetService}}
	exp2 := &Experiment{Name: "exp2", Type: ExpTypeNetworkDelay, Target: Target{Type: TargetService}}

	created1, _ := svc.CreateExperiment(ctx, exp1)
	created2, _ := svc.CreateExperiment(ctx, exp2)

	// Start first
	svc.RunExperiment(ctx, created1.ID)

	// Second should fail due to max concurrent
	err := svc.RunExperiment(ctx, created2.ID)
	if err == nil {
		t.Error("expected error when max concurrent experiments reached")
	}
}

func TestRunExperiment_NoInjector(t *testing.T) {
	svc := NewService(nil, nil, nil)
	// Don't call Start() - no injectors registered

	exp := &Experiment{
		Name:   "no-injector-test",
		Type:   ExpTypeCustom, // No injector registered for this
		Target: Target{Type: TargetService},
	}

	created, _ := svc.CreateExperiment(context.Background(), exp)

	err := svc.RunExperiment(context.Background(), created.ID)
	if err == nil {
		t.Error("expected error when no injector registered")
	}
}

// ============================================================================
// Stop Experiment Tests
// ============================================================================

func TestStopExperiment(t *testing.T) {
	cfg := &Config{
		DryRunMode:               true,
		DefaultDuration:          10 * time.Second, // Long duration
		SafetyCheckInterval:      50 * time.Millisecond,
		MaxConcurrentExperiments: 5,
	}
	svc := NewService(nil, nil, cfg)
	svc.Start(context.Background())

	exp := &Experiment{
		Name:   "stop-test",
		Type:   ExpTypeNetworkDelay,
		Target: Target{Type: TargetService},
	}

	created, _ := svc.CreateExperiment(context.Background(), exp)
	svc.RunExperiment(context.Background(), created.ID)

	// Give it time to start
	time.Sleep(50 * time.Millisecond)

	err := svc.StopExperiment(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("StopExperiment failed: %v", err)
	}

	// Wait for abort to process
	time.Sleep(100 * time.Millisecond)

	result, _ := svc.GetExperiment(created.ID)
	if result.Status != StatusAborted {
		t.Errorf("expected status Aborted, got %s", result.Status)
	}
}

func TestStopExperiment_NotRunning(t *testing.T) {
	svc := NewService(nil, nil, nil)

	exp := &Experiment{
		Name:   "not-running-test",
		Type:   ExpTypeNetworkDelay,
		Target: Target{Type: TargetService},
	}

	created, _ := svc.CreateExperiment(context.Background(), exp)

	err := svc.StopExperiment(context.Background(), created.ID)
	if err == nil {
		t.Error("expected error for not running experiment")
	}
}

// ============================================================================
// Safety Check Tests
// ============================================================================

func TestExperiment_WithSafetyChecks(t *testing.T) {
	cfg := &Config{
		DryRunMode:               true,
		DefaultDuration:          100 * time.Millisecond,
		SafetyCheckInterval:      20 * time.Millisecond,
		MaxConcurrentExperiments: 5,
	}
	svc := NewService(nil, nil, cfg)
	svc.Start(context.Background())

	exp := &Experiment{
		Name:   "safety-check-test",
		Type:   ExpTypeNetworkDelay,
		Target: Target{Type: TargetService},
		SafetyChecks: []SafetyCheck{
			{
				Name:        "error-rate",
				Description: "Abort if error rate exceeds threshold",
				Condition:   "error_rate > threshold",
				Threshold:   0.1,
				Action:      ActionAbort,
			},
		},
	}

	created, _ := svc.CreateExperiment(context.Background(), exp)

	if len(created.SafetyChecks) != 1 {
		t.Errorf("expected 1 safety check, got %d", len(created.SafetyChecks))
	}
}

// ============================================================================
// Game Day Tests
// ============================================================================

func TestCreateGameDay(t *testing.T) {
	svc := NewService(nil, nil, nil)
	ctx := context.Background()

	// Create some experiments first
	exp1, _ := svc.CreateExperiment(ctx, &Experiment{Name: "gd-exp1", Type: ExpTypeNetworkDelay, Target: Target{Type: TargetService}})
	exp2, _ := svc.CreateExperiment(ctx, &Experiment{Name: "gd-exp2", Type: ExpTypeProcessKill, Target: Target{Type: TargetContainer}})

	gd := &GameDay{
		Name:        "Q1 Resilience Test",
		Description: "Test system resilience under various failure conditions",
		Experiments: []string{exp1.ID, exp2.ID},
		Schedule:    time.Now().Add(24 * time.Hour),
		Duration:    2 * time.Hour,
	}

	result, err := svc.CreateGameDay(ctx, gd)
	if err != nil {
		t.Fatalf("CreateGameDay failed: %v", err)
	}

	if result.ID == "" {
		t.Error("expected ID to be generated")
	}
	if result.Status != "scheduled" {
		t.Errorf("expected status 'scheduled', got %s", result.Status)
	}
	if len(result.Experiments) != 2 {
		t.Errorf("expected 2 experiments, got %d", len(result.Experiments))
	}
}

// ============================================================================
// Stats Tests
// ============================================================================

func TestStats(t *testing.T) {
	svc := NewService(nil, nil, &Config{DryRunMode: true})
	svc.Start(context.Background())
	ctx := context.Background()

	// Create experiments
	svc.CreateExperiment(ctx, &Experiment{Name: "stats1", Type: ExpTypeNetworkDelay, Target: Target{Type: TargetService}})
	svc.CreateExperiment(ctx, &Experiment{Name: "stats2", Type: ExpTypeProcessKill, Target: Target{Type: TargetContainer}})

	stats := svc.Stats()

	if stats["total_experiments"].(int) != 2 {
		t.Errorf("expected 2 experiments, got %v", stats["total_experiments"])
	}
	if stats["dry_run_mode"].(bool) != true {
		t.Error("expected dry_run_mode true")
	}

	byStatus := stats["by_status"].(map[string]int)
	if byStatus["draft"] != 2 {
		t.Errorf("expected 2 draft experiments, got %d", byStatus["draft"])
	}
}

// ============================================================================
// Concurrent Access Tests
// ============================================================================

func TestConcurrentCreateExperiment(t *testing.T) {
	svc := NewService(nil, nil, nil)
	ctx := context.Background()

	done := make(chan bool, 50)

	for i := 0; i < 50; i++ {
		go func(i int) {
			exp := &Experiment{
				Name:   "concurrent-exp",
				Type:   ExpTypeNetworkDelay,
				Target: Target{Type: TargetService},
			}
			_, _ = svc.CreateExperiment(ctx, exp)
			done <- true
		}(i)
	}

	for i := 0; i < 50; i++ {
		<-done
	}

	all := svc.ListExperiments("")
	if len(all) != 50 {
		t.Errorf("expected 50 experiments, got %d", len(all))
	}
}

// ============================================================================
// Service Lifecycle Tests
// ============================================================================

func TestServiceStop(t *testing.T) {
	cfg := &Config{
		DryRunMode:               true,
		DefaultDuration:          10 * time.Second,
		SafetyCheckInterval:      50 * time.Millisecond,
		MaxConcurrentExperiments: 5,
	}
	svc := NewService(nil, nil, cfg)
	svc.Start(context.Background())
	ctx := context.Background()

	// Create and run an experiment
	exp := &Experiment{
		Name:   "stop-lifecycle",
		Type:   ExpTypeNetworkDelay,
		Target: Target{Type: TargetService},
	}
	created, _ := svc.CreateExperiment(ctx, exp)
	_ = svc.RunExperiment(ctx, created.ID)
	time.Sleep(50 * time.Millisecond)

	// Stop should cancel running experiments
	err := svc.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Verify experiment was cancelled
	result, _ := svc.GetExperiment(created.ID)
	if result.Status != StatusAborted {
		t.Errorf("expected status Aborted after stop, got %s", result.Status)
	}
}

func TestServiceStopNoRunning(t *testing.T) {
	svc := NewService(nil, nil, nil)
	err := svc.Stop()
	if err != nil {
		t.Fatalf("Stop with no running experiments failed: %v", err)
	}
}

func TestWotanDrops(t *testing.T) {
	svc := NewService(nil, nil, nil)

	// Initial drops should be 0
	if svc.WotanDrops() != 0 {
		t.Errorf("expected 0 drops initially, got %d", svc.WotanDrops())
	}

	// publishEvent with nil wotan should increment drops
	svc.publishEvent(context.Background(), "test.event", map[string]interface{}{"key": "value"})

	if svc.WotanDrops() != 1 {
		t.Errorf("expected 1 drop after publish with nil wotan, got %d", svc.WotanDrops())
	}
}

func TestRunExperimentWithSteadyState(t *testing.T) {
	cfg := &Config{
		DryRunMode:               true,
		DefaultDuration:          50 * time.Millisecond,
		SafetyCheckInterval:      20 * time.Millisecond,
		MaxConcurrentExperiments: 5,
	}
	svc := NewService(nil, nil, cfg)
	svc.Start(context.Background())
	ctx := context.Background()

	exp := &Experiment{
		Name:   "steady-state-run",
		Type:   ExpTypeNetworkDelay,
		Target: Target{Type: TargetService},
		SteadyState: &SteadyState{
			Hypothesis: "System stays healthy",
			Probes: []Probe{
				{Name: "health", Type: ProbeHTTP, Provider: "http"},
			},
			Tolerance: 5.0,
			Timeout:   1 * time.Second,
		},
		Schedule: &Schedule{
			Duration: 30 * time.Millisecond,
		},
	}

	created, _ := svc.CreateExperiment(ctx, exp)
	err := svc.RunExperiment(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	result, _ := svc.GetExperiment(created.ID)
	if result.Status != StatusCompleted {
		t.Errorf("expected completed, got %s", result.Status)
	}
	if result.Results == nil {
		t.Fatal("expected results to be set")
	}
	if !result.Results.HypothesisValid {
		t.Error("expected hypothesis to be valid")
	}
	if len(result.Results.SteadyStateProbes) == 0 {
		t.Error("expected steady state probes in results")
	}
}

func TestRunExperimentWithRollback(t *testing.T) {
	cfg := &Config{
		DryRunMode:               true,
		DefaultDuration:          50 * time.Millisecond,
		SafetyCheckInterval:      20 * time.Millisecond,
		EnableAutoRollback:       true,
		MaxConcurrentExperiments: 5,
	}
	svc := NewService(nil, nil, cfg)
	svc.Start(context.Background())
	ctx := context.Background()

	exp := &Experiment{
		Name:   "rollback-test",
		Type:   ExpTypeNetworkDelay,
		Target: Target{Type: TargetService},
		Rollback: &Rollback{
			Automatic: true,
			State:     map[string]interface{}{"original": "state"},
		},
		Schedule: &Schedule{
			Duration: 30 * time.Millisecond,
		},
	}

	created, _ := svc.CreateExperiment(ctx, exp)
	err := svc.RunExperiment(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	result, _ := svc.GetExperiment(created.ID)
	if result.Status != StatusCompleted {
		t.Errorf("expected completed, got %s", result.Status)
	}
}

func TestStatsWithResults(t *testing.T) {
	cfg := &Config{
		DryRunMode:               true,
		DefaultDuration:          50 * time.Millisecond,
		SafetyCheckInterval:      20 * time.Millisecond,
		MaxConcurrentExperiments: 5,
	}
	svc := NewService(nil, nil, cfg)
	svc.Start(context.Background())
	ctx := context.Background()

	exp := &Experiment{
		Name:     "stats-result",
		Type:     ExpTypeNetworkDelay,
		Target:   Target{Type: TargetService},
		Schedule: &Schedule{Duration: 20 * time.Millisecond},
	}

	created, _ := svc.CreateExperiment(ctx, exp)
	_ = svc.RunExperiment(ctx, created.ID)
	time.Sleep(150 * time.Millisecond)

	stats := svc.Stats()
	successful := stats["successful"].(int)
	if successful < 1 {
		t.Errorf("expected at least 1 successful, got %d", successful)
	}
}

func TestCreateExperimentWithExplicitID(t *testing.T) {
	svc := NewService(nil, nil, nil)
	exp := &Experiment{
		ID:     "custom-id",
		Name:   "custom",
		Type:   ExpTypeNetworkDelay,
		Target: Target{Type: TargetService},
	}
	created, _ := svc.CreateExperiment(context.Background(), exp)
	if created.ID != "custom-id" {
		t.Errorf("expected custom-id, got %s", created.ID)
	}
}

func TestExperimentTypeConstants(t *testing.T) {
	types := []ExperimentType{
		ExpTypeNetworkDelay, ExpTypeNetworkLoss, ExpTypeNetworkPartition,
		ExpTypeCPUStress, ExpTypeMemoryStress, ExpTypeDiskStress,
		ExpTypeProcessKill, ExpTypeContainerKill, ExpTypeNodeDrain,
		ExpTypeDNSFailure, ExpTypeTimeSkew, ExpTypeHTTPError,
		ExpTypeLatencySpike, ExpTypeResourceExhaust, ExpTypeCustom,
	}
	for _, et := range types {
		if et == "" {
			t.Error("experiment type should not be empty")
		}
	}
}

func TestExperimentStatusConstants(t *testing.T) {
	statuses := []ExperimentStatus{
		StatusDraft, StatusScheduled, StatusRunning, StatusPaused,
		StatusCompleted, StatusAborted, StatusFailed,
	}
	for _, s := range statuses {
		if s == "" {
			t.Error("status should not be empty")
		}
	}
}

func TestTargetTypeConstants(t *testing.T) {
	types := []TargetType{TargetService, TargetContainer, TargetNode, TargetNetwork, TargetPod}
	for _, tt := range types {
		if tt == "" {
			t.Error("target type should not be empty")
		}
	}
}

func TestProbeTypeConstants(t *testing.T) {
	types := []ProbeType{ProbeHTTP, ProbeMetric, ProbeProcess, ProbeCustom}
	for _, pt := range types {
		if pt == "" {
			t.Error("probe type should not be empty")
		}
	}
}

func TestSafetyActionConstants(t *testing.T) {
	actions := []SafetyAction{ActionAbort, ActionPause, ActionAlert, ActionRollback}
	for _, a := range actions {
		if a == "" {
			t.Error("safety action should not be empty")
		}
	}
}

func TestFindingSeverityConstants(t *testing.T) {
	severities := []FindingSeverity{SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow, SeverityInfo}
	for _, s := range severities {
		if s == "" {
			t.Error("finding severity should not be empty")
		}
	}
}

func TestScheduleTypeConstants(t *testing.T) {
	types := []ScheduleType{ScheduleOnce, ScheduleRecurring, ScheduleContinuous, ScheduleGameDay}
	for _, st := range types {
		if st == "" {
			t.Error("schedule type should not be empty")
		}
	}
}

// ============================================================================
// Benchmarks
// ============================================================================

func BenchmarkCreateExperiment(b *testing.B) {
	svc := NewService(nil, nil, nil)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		exp := &Experiment{
			Name:   "bench-exp",
			Type:   ExpTypeNetworkDelay,
			Target: Target{Type: TargetService},
		}
		_, _ = svc.CreateExperiment(ctx, exp)
	}
}

func BenchmarkGetExperiment(b *testing.B) {
	svc := NewService(nil, nil, nil)
	ctx := context.Background()

	exp := &Experiment{
		Name:   "bench-get",
		Type:   ExpTypeNetworkDelay,
		Target: Target{Type: TargetService},
	}
	created, _ := svc.CreateExperiment(ctx, exp)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = svc.GetExperiment(created.ID)
	}
}

func BenchmarkListExperiments(b *testing.B) {
	svc := NewService(nil, nil, nil)
	ctx := context.Background()

	// Create 100 experiments
	for i := 0; i < 100; i++ {
		exp := &Experiment{
			Name:   "bench-list",
			Type:   ExpTypeNetworkDelay,
			Target: Target{Type: TargetService},
		}
		svc.CreateExperiment(ctx, exp)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = svc.ListExperiments("")
	}
}
