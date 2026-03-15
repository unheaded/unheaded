// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package events - Integration Tests for Cross-Service Communication
//
// These tests verify that the event routing works correctly between
// all Kingdom services through the Fae Chamber.
package events

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// INTEGRATION TEST: FULL KINGDOM EVENT FLOW
// ============================================================================

// TestFullKingdomEventFlow simulates the complete event flow across all services
func TestFullKingdomEventFlow(t *testing.T) {
	// Create the router (simulates the Fae Chamber)
	router := NewRouter(&RouterConfig{DefaultTimeout: 5 * time.Second})
	defer router.Close()

	// Track events received by each service
	var (
		timelineTasksReceived     int32
		captainMilestonesReceived int32
		captainAlertsReceived     int32
		micromanagerDecisions     int32
		architectDecisions        int32
		kanbanTasksReceived       int32
	)

	// =========================================================================
	// REGISTER SERVICE HANDLERS
	// =========================================================================

	// Timeguru: listens for task completions to update timeline
	err := TimeguruRoutes(router, func(ctx context.Context, event *TaskEvent) error {
		atomic.AddInt32(&timelineTasksReceived, 1)
		t.Logf("[Timeguru] Task completed: %s - %s", event.TaskID, event.Title)
		return nil
	})
	if err != nil {
		t.Fatalf("timeguru routes: %v", err)
	}

	// Captain: listens for milestones and critical alerts
	err = CaptainRoutes(router, func(ctx context.Context, event *MilestoneHitEvent) error {
		atomic.AddInt32(&captainMilestonesReceived, 1)
		t.Logf("[Captain] 🎉 Milestone achieved: %s", event.MilestoneName)
		return nil
	})
	if err != nil {
		t.Fatalf("captain routes: %v", err)
	}

	err = router.Register(TopicAlertsCritical, func(ctx context.Context, topic string, payload []byte) error {
		atomic.AddInt32(&captainAlertsReceived, 1)
		t.Logf("[Captain] ⚠️ Critical alert received")
		return nil
	})
	if err != nil {
		t.Fatalf("captain alerts: %v", err)
	}

	// Micromanager: listens for approved decisions to create tasks
	err = MicromanagerRoutes(router, func(ctx context.Context, event *DecisionEvent) error {
		atomic.AddInt32(&micromanagerDecisions, 1)
		t.Logf("[Micromanager] Decision approved: %s - creating tasks", event.Title)
		return nil
	})
	if err != nil {
		t.Fatalf("micromanager routes: %v", err)
	}

	// Architect: listens for approved decisions to create ADRs
	err = ArchitectRoutes(router, func(ctx context.Context, event *DecisionEvent) error {
		atomic.AddInt32(&architectDecisions, 1)
		t.Logf("[Architect] Decision approved: %s - creating ADR", event.Title)
		return nil
	})
	if err != nil {
		t.Fatalf("architect routes: %v", err)
	}

	// Kanban: listens for all task events
	err = router.Register(TopicTasksAll, func(ctx context.Context, topic string, payload []byte) error {
		atomic.AddInt32(&kanbanTasksReceived, 1)
		t.Logf("[Kanban] Task event on topic: %s", topic)
		return nil
	})
	if err != nil {
		t.Fatalf("kanban routes: %v", err)
	}

	// =========================================================================
	// SIMULATE EVENT FLOW
	// =========================================================================
	ctx := context.Background()

	// 1. Captain makes a decision
	decision := &DecisionEvent{
		DecisionID: "decision-001",
		Title:      "Adopt BGP for all networking",
		Content:    "All services will use BGP for routing",
		Owner:      "captain",
		Priority:   PriorityHigh,
		Status:     "approved",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	decisionPayload, _ := json.Marshal(decision)
	router.Route(ctx, TopicDecisionsApproved, decisionPayload)

	// 2. Micromanager creates and completes a task
	task := &TaskEvent{
		TaskID:    "task-001",
		Title:     "Implement BGP configuration",
		Status:    "done",
		Priority:  PriorityHigh,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	taskPayload, _ := json.Marshal(task)
	router.Route(ctx, TopicTasksCreated, taskPayload)
	router.Route(ctx, TopicTasksCompleted, taskPayload)

	// 3. Timeguru detects milestone achieved
	milestone := &MilestoneHitEvent{
		MilestoneID:   "milestone-001",
		MilestoneName: "Phase 1 Complete",
		Phase:         "Age 1",
		CompletedAt:   time.Now(),
	}
	milestonePayload, _ := json.Marshal(milestone)
	router.Route(ctx, TopicTimelineMilestoneHit, milestonePayload)

	// 4. Vambraces detects anomaly and sends critical alert
	alert := &AlertEvent{
		AlertID:   "alert-001",
		Severity:  SeverityCritical,
		Source:    "vambraces",
		Title:     "Container health check failed",
		CreatedAt: time.Now(),
	}
	alertPayload, _ := json.Marshal(alert)
	router.Route(ctx, TopicAlertsCritical, alertPayload)

	// =========================================================================
	// VERIFY RESULTS
	// =========================================================================

	// Decision should be received by both Micromanager and Architect
	if atomic.LoadInt32(&micromanagerDecisions) != 1 {
		t.Errorf("micromanager should receive 1 decision, got %d", micromanagerDecisions)
	}
	if atomic.LoadInt32(&architectDecisions) != 1 {
		t.Errorf("architect should receive 1 decision, got %d", architectDecisions)
	}

	// Timeguru should receive the task completion
	if atomic.LoadInt32(&timelineTasksReceived) != 1 {
		t.Errorf("timeguru should receive 1 task completion, got %d", timelineTasksReceived)
	}

	// Captain should receive milestone and alert
	if atomic.LoadInt32(&captainMilestonesReceived) != 1 {
		t.Errorf("captain should receive 1 milestone, got %d", captainMilestonesReceived)
	}
	if atomic.LoadInt32(&captainAlertsReceived) != 1 {
		t.Errorf("captain should receive 1 alert, got %d", captainAlertsReceived)
	}

	// Kanban should receive both task events (created + completed)
	if atomic.LoadInt32(&kanbanTasksReceived) != 2 {
		t.Errorf("kanban should receive 2 task events, got %d", kanbanTasksReceived)
	}

	// Check stats
	stats := router.Stats()
	t.Logf("Router stats: %+v", stats)

	if stats["messages_routed"].(int64) < 5 {
		t.Errorf("expected at least 5 routed messages, got %d", stats["messages_routed"])
	}
}

// ============================================================================
// TEST: CONCURRENT EVENT HANDLING
// ============================================================================

func TestConcurrentEventHandling(t *testing.T) {
	router := NewRouter(&RouterConfig{DefaultTimeout: 10 * time.Second})
	defer router.Close()

	var eventsReceived int64
	var wg sync.WaitGroup

	// Register handler
	router.Register(TopicTasksAll, func(ctx context.Context, topic string, payload []byte) error {
		atomic.AddInt64(&eventsReceived, 1)
		// Simulate some work
		time.Sleep(time.Millisecond)
		return nil
	})

	// Send many events concurrently
	numEvents := 100
	for i := 0; i < numEvents; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			task := &TaskEvent{
				TaskID: "task-" + string(rune(id)),
				Title:  "Concurrent Task",
				Status: "in-progress",
			}
			payload, _ := json.Marshal(task)
			router.Route(context.Background(), TopicTasksCreated, payload)
		}(i)
	}

	wg.Wait()

	// All events should be received
	if atomic.LoadInt64(&eventsReceived) != int64(numEvents) {
		t.Errorf("expected %d events, got %d", numEvents, eventsReceived)
	}
}

// ============================================================================
// TEST: EVENT CHAIN (CASCADE)
// ============================================================================

func TestEventChain(t *testing.T) {
	router := NewRouter(nil)
	defer router.Close()

	var chainComplete bool

	// Set up a chain: decision -> task -> completion -> milestone
	// Each step triggers the next

	// Step 1: Decision approved triggers task creation
	router.Register(TopicDecisionsApproved, func(ctx context.Context, topic string, payload []byte) error {
		t.Log("Chain step 1: Decision approved")
		task := &TaskEvent{
			TaskID: "chain-task",
			Title:  "Chain Task",
			Status: "created",
		}
		taskPayload, _ := json.Marshal(task)
		return router.Route(ctx, TopicTasksCreated, taskPayload)
	})

	// Step 2: Task created triggers completion (simulate fast execution)
	router.Register(TopicTasksCreated, func(ctx context.Context, topic string, payload []byte) error {
		t.Log("Chain step 2: Task created")
		var task TaskEvent
		json.Unmarshal(payload, &task)
		task.Status = "done"
		completedPayload, _ := json.Marshal(task)
		return router.Route(ctx, TopicTasksCompleted, completedPayload)
	})

	// Step 3: Task completed triggers milestone
	router.Register(TopicTasksCompleted, func(ctx context.Context, topic string, payload []byte) error {
		t.Log("Chain step 3: Task completed")
		milestone := &MilestoneHitEvent{
			MilestoneID:   "chain-milestone",
			MilestoneName: "Chain Complete",
		}
		milestonePayload, _ := json.Marshal(milestone)
		return router.Route(ctx, TopicTimelineMilestoneHit, milestonePayload)
	})

	// Step 4: Milestone triggers chain complete
	router.Register(TopicTimelineMilestoneHit, func(ctx context.Context, topic string, payload []byte) error {
		t.Log("Chain step 4: Milestone hit - CHAIN COMPLETE!")
		chainComplete = true
		return nil
	})

	// Trigger the chain with initial decision
	decision := &DecisionEvent{
		DecisionID: "chain-decision",
		Title:      "Start the chain",
		Owner:      "test",
		Status:     "approved",
	}
	payload, _ := json.Marshal(decision)
	err := router.Route(context.Background(), TopicDecisionsApproved, payload)
	if err != nil {
		t.Errorf("chain route failed: %v", err)
	}

	if !chainComplete {
		t.Error("event chain did not complete")
	}
}

// ============================================================================
// TEST: ALERT ESCALATION
// ============================================================================

func TestAlertEscalation(t *testing.T) {
	router := NewRouter(nil)
	defer router.Close()

	var infoReceived, warningReceived, criticalReceived bool

	// Register handlers for each severity
	router.Register(TopicAlertsInfo, func(ctx context.Context, topic string, payload []byte) error {
		infoReceived = true
		t.Log("Info alert received")
		return nil
	})

	router.Register(TopicAlertsWarning, func(ctx context.Context, topic string, payload []byte) error {
		warningReceived = true
		t.Log("Warning alert received")
		return nil
	})

	router.Register(TopicAlertsCritical, func(ctx context.Context, topic string, payload []byte) error {
		criticalReceived = true
		t.Log("CRITICAL alert received!")
		return nil
	})

	// Send alerts of different severities
	alerts := []struct {
		severity AlertSeverity
		topic    string
	}{
		{SeverityInfo, TopicAlertsInfo},
		{SeverityWarning, TopicAlertsWarning},
		{SeverityCritical, TopicAlertsCritical},
	}

	for _, a := range alerts {
		alert := &AlertEvent{
			AlertID:   "alert-" + string(a.severity),
			Severity:  a.severity,
			Source:    "test",
			Title:     "Test " + string(a.severity),
			CreatedAt: time.Now(),
		}
		payload, _ := json.Marshal(alert)
		router.Route(context.Background(), a.topic, payload)
	}

	if !infoReceived {
		t.Error("info alert not received")
	}
	if !warningReceived {
		t.Error("warning alert not received")
	}
	if !criticalReceived {
		t.Error("critical alert not received")
	}
}

// ============================================================================
// TEST: STATE DRIFT DETECTION
// ============================================================================

func TestStateDriftDetection(t *testing.T) {
	router := NewRouter(nil)
	defer router.Close()

	var driftDetected, driftResolved bool

	// Cuirass detects drift
	router.Register(TopicStateDriftDetected, func(ctx context.Context, topic string, payload []byte) error {
		var drift DriftEvent
		json.Unmarshal(payload, &drift)
		t.Logf("Drift detected: %s (type: %s)", drift.DriftID, drift.DriftType)
		driftDetected = true

		// Simulate automatic resolution
		drift.ResolvedAt = time.Now()
		resolvedPayload, _ := json.Marshal(drift)
		return router.Route(ctx, TopicStateDriftResolved, resolvedPayload)
	})

	router.Register(TopicStateDriftResolved, func(ctx context.Context, topic string, payload []byte) error {
		var drift DriftEvent
		json.Unmarshal(payload, &drift)
		t.Logf("Drift resolved: %s", drift.DriftID)
		driftResolved = true
		return nil
	})

	// Simulate drift detection
	drift := &DriftEvent{
		DriftID:      "drift-001",
		ContainerID:  "container-123",
		DriftType:    "missing",
		Severity:     "high",
		DesiredState: "running",
		ActualState:  "not_found",
		DetectedAt:   time.Now(),
	}
	payload, _ := json.Marshal(drift)
	router.Route(context.Background(), TopicStateDriftDetected, payload)

	if !driftDetected {
		t.Error("drift not detected")
	}
	if !driftResolved {
		t.Error("drift not resolved")
	}
}

// ============================================================================
// BENCHMARK: EVENT ROUTING
// ============================================================================

func BenchmarkEventRouting(b *testing.B) {
	router := NewRouter(nil)
	defer router.Close()

	router.Register(TopicTasksCreated, func(ctx context.Context, topic string, payload []byte) error {
		return nil
	})

	task := &TaskEvent{
		TaskID: "bench-task",
		Title:  "Benchmark Task",
		Status: "created",
	}
	payload, _ := json.Marshal(task)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.Route(context.Background(), TopicTasksCreated, payload)
	}
}

func BenchmarkWildcardRouting(b *testing.B) {
	router := NewRouter(nil)
	defer router.Close()

	router.Register(TopicTasksAll, func(ctx context.Context, topic string, payload []byte) error {
		return nil
	})

	task := &TaskEvent{
		TaskID: "bench-task",
		Title:  "Benchmark Task",
		Status: "created",
	}
	payload, _ := json.Marshal(task)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.Route(context.Background(), TopicTasksCreated, payload)
	}
}

func BenchmarkConcurrentRouting(b *testing.B) {
	router := NewRouter(nil)
	defer router.Close()

	router.Register(TopicTasksAll, func(ctx context.Context, topic string, payload []byte) error {
		return nil
	})

	task := &TaskEvent{
		TaskID: "bench-task",
		Title:  "Benchmark Task",
		Status: "created",
	}
	payload, _ := json.Marshal(task)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			router.Route(context.Background(), TopicTasksCreated, payload)
		}
	})
}
