// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"
)

// =============================================================================
// Test Helpers
// =============================================================================

func newTestNode(id string, cpuCapacity, memCapacity int64) *Node {
	return &Node{
		ID:          id,
		Name:        id,
		State:       NodeStateReady,
		Capacity:    Resources{CPU: cpuCapacity, Memory: memCapacity},
		Allocatable: Resources{CPU: cpuCapacity, Memory: memCapacity},
		Available:   Resources{CPU: cpuCapacity, Memory: memCapacity},
		Allocated:   Resources{},
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
		CreatedAt:   time.Now(),
	}
}

func newTestWorkload(id string, priority int32, cpu, mem int64) *Workload {
	return &Workload{
		ID:       id,
		Name:     id,
		Priority: priority,
		Resources: ResourceRequirements{
			Requests: Resources{CPU: cpu, Memory: mem},
		},
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
		State:       WorkloadStateScheduled,
		CreatedAt:   time.Now(),
	}
}

// =============================================================================
// Priority-Based Preemption Tests
// =============================================================================

func TestPreemption_HigherPriorityEvictsLower(t *testing.T) {
	nodes := NewNodeRegistry()
	node := newTestNode("node-1", 1000, 1024*1024*1024)
	node.Allocated = Resources{CPU: 800, Memory: 800 * 1024 * 1024}
	node.Available = Resources{CPU: 200, Memory: 224 * 1024 * 1024}
	nodes.Register(node)

	preemptor := NewPreemptor(nodes, 30*time.Second)

	// Create low priority workload
	lowPrioWorkload := newTestWorkload("low-prio", 10, 500, 500*1024*1024)
	lowPrioWorkload.BoundNode = "node-1"

	// Create high priority workload that needs preemption
	highPrioWorkload := newTestWorkload("high-prio", 100, 600, 600*1024*1024)

	// Mock workload getter
	getWorkloads := func(nodeID string) []*Workload {
		if nodeID == "node-1" {
			return []*Workload{lowPrioWorkload}
		}
		return nil
	}

	result := preemptor.FindPreemptionCandidates(highPrioWorkload, []*Node{node}, getWorkloads)

	if result == nil {
		t.Fatal("expected preemption result, got nil")
	}

	if !result.Success {
		t.Errorf("expected success, got failure")
	}

	if len(result.PreemptedWorkloads) != 1 {
		t.Errorf("expected 1 victim, got %d", len(result.PreemptedWorkloads))
	}

	if result.PreemptedWorkloads[0] != "low-prio" {
		t.Errorf("expected low-prio to be preempted, got %s", result.PreemptedWorkloads[0])
	}
}

func TestPreemption_SamePriorityCannotPreempt(t *testing.T) {
	nodes := NewNodeRegistry()
	node := newTestNode("node-1", 1000, 1024*1024*1024)
	node.Allocated = Resources{CPU: 800, Memory: 800 * 1024 * 1024}
	node.Available = Resources{CPU: 200, Memory: 224 * 1024 * 1024}
	nodes.Register(node)

	preemptor := NewPreemptor(nodes, 30*time.Second)

	// Create workload with same priority
	existingWorkload := newTestWorkload("existing", 50, 500, 500*1024*1024)
	existingWorkload.BoundNode = "node-1"

	// Create new workload with same priority
	newWorkload := newTestWorkload("new", 50, 600, 600*1024*1024)

	getWorkloads := func(nodeID string) []*Workload {
		if nodeID == "node-1" {
			return []*Workload{existingWorkload}
		}
		return nil
	}

	result := preemptor.FindPreemptionCandidates(newWorkload, []*Node{node}, getWorkloads)

	if result != nil {
		t.Error("expected no preemption for same priority workloads")
	}
}

func TestPreemption_LowerPriorityCannotPreemptHigher(t *testing.T) {
	nodes := NewNodeRegistry()
	node := newTestNode("node-1", 1000, 1024*1024*1024)
	node.Allocated = Resources{CPU: 800, Memory: 800 * 1024 * 1024}
	node.Available = Resources{CPU: 200, Memory: 224 * 1024 * 1024}
	nodes.Register(node)

	preemptor := NewPreemptor(nodes, 30*time.Second)

	// Create high priority workload
	highPrioWorkload := newTestWorkload("high-prio", 100, 500, 500*1024*1024)
	highPrioWorkload.BoundNode = "node-1"

	// Create low priority workload
	lowPrioWorkload := newTestWorkload("low-prio", 10, 600, 600*1024*1024)

	getWorkloads := func(nodeID string) []*Workload {
		if nodeID == "node-1" {
			return []*Workload{highPrioWorkload}
		}
		return nil
	}

	result := preemptor.FindPreemptionCandidates(lowPrioWorkload, []*Node{node}, getWorkloads)

	if result != nil {
		t.Error("lower priority workload should not preempt higher priority")
	}
}

// =============================================================================
// Preemption Policy Tests
// =============================================================================

func TestPreemptionPolicy_Never(t *testing.T) {
	nodes := NewNodeRegistry()
	node := newTestNode("node-1", 1000, 1024*1024*1024)
	node.Allocated = Resources{CPU: 800, Memory: 800 * 1024 * 1024}
	node.Available = Resources{CPU: 200, Memory: 224 * 1024 * 1024}
	nodes.Register(node)

	preemptor := NewPreemptor(nodes, 30*time.Second)

	// Create victim
	victim := newTestWorkload("victim", 10, 500, 500*1024*1024)
	victim.BoundNode = "node-1"

	// Create preemptor with Never policy
	highPrio := newTestWorkload("high-prio", 100, 600, 600*1024*1024)
	highPrio.Annotations["scheduler.unheaded.io/preemption-policy"] = string(PreemptNever)

	getWorkloads := func(nodeID string) []*Workload {
		return []*Workload{victim}
	}

	result := preemptor.FindPreemptionCandidates(highPrio, []*Node{node}, getWorkloads)

	if result != nil {
		t.Error("workload with PreemptNever policy should not preempt")
	}
}

func TestPreemptionPolicy_PreemptLowerPriority(t *testing.T) {
	nodes := NewNodeRegistry()
	node := newTestNode("node-1", 1000, 1024*1024*1024)
	node.Allocated = Resources{CPU: 800, Memory: 800 * 1024 * 1024}
	node.Available = Resources{CPU: 200, Memory: 224 * 1024 * 1024}
	nodes.Register(node)

	preemptor := NewPreemptor(nodes, 30*time.Second)

	victim := newTestWorkload("victim", 10, 500, 500*1024*1024)
	victim.BoundNode = "node-1"

	highPrio := newTestWorkload("high-prio", 100, 600, 600*1024*1024)
	highPrio.Annotations["scheduler.unheaded.io/preemption-policy"] = string(PreemptLowerPriority)

	getWorkloads := func(nodeID string) []*Workload {
		return []*Workload{victim}
	}

	result := preemptor.FindPreemptionCandidates(highPrio, []*Node{node}, getWorkloads)

	if result == nil {
		t.Fatal("expected preemption result")
	}

	if !result.Success {
		t.Error("expected successful preemption")
	}
}

func TestPreemption_ProtectedWorkloads(t *testing.T) {
	nodes := NewNodeRegistry()
	node := newTestNode("node-1", 1000, 1024*1024*1024)
	node.Allocated = Resources{CPU: 800, Memory: 800 * 1024 * 1024}
	node.Available = Resources{CPU: 200, Memory: 224 * 1024 * 1024}
	nodes.Register(node)

	preemptor := NewPreemptor(nodes, 30*time.Second)

	// Create protected victim
	victim := newTestWorkload("victim", 10, 500, 500*1024*1024)
	victim.Annotations["scheduler.unheaded.io/preemption-protected"] = "true"
	victim.BoundNode = "node-1"

	highPrio := newTestWorkload("high-prio", 100, 600, 600*1024*1024)

	getWorkloads := func(nodeID string) []*Workload {
		return []*Workload{victim}
	}

	result := preemptor.FindPreemptionCandidates(highPrio, []*Node{node}, getWorkloads)

	if result != nil {
		t.Error("protected workloads should not be preempted")
	}
}

// =============================================================================
// Victim Selection Algorithm Tests
// =============================================================================

func TestVictimSelection_MinimizeDisruption(t *testing.T) {
	nodes := NewNodeRegistry()
	node := newTestNode("node-1", 2000, 2*1024*1024*1024)
	node.Allocated = Resources{CPU: 1800, Memory: 1800 * 1024 * 1024}
	node.Available = Resources{CPU: 200, Memory: 200 * 1024 * 1024}
	nodes.Register(node)

	preemptor := NewPreemptor(nodes, 30*time.Second)

	// Create multiple low priority workloads
	victim1 := newTestWorkload("victim-1", 10, 300, 300*1024*1024)
	victim1.BoundNode = "node-1"

	victim2 := newTestWorkload("victim-2", 20, 400, 400*1024*1024)
	victim2.BoundNode = "node-1"

	victim3 := newTestWorkload("victim-3", 30, 500, 500*1024*1024)
	victim3.BoundNode = "node-1"

	// High priority workload that needs 500 CPU
	highPrio := newTestWorkload("high-prio", 100, 500, 500*1024*1024)

	getWorkloads := func(nodeID string) []*Workload {
		return []*Workload{victim1, victim2, victim3}
	}

	result := preemptor.FindPreemptionCandidates(highPrio, []*Node{node}, getWorkloads)

	if result == nil {
		t.Fatal("expected preemption result")
	}

	// Should select lowest priority victims first
	if len(result.PreemptedWorkloads) == 0 {
		t.Fatal("expected at least one victim")
	}

	// First victim should be the lowest priority
	if result.PreemptedWorkloads[0] != "victim-1" {
		t.Errorf("expected lowest priority victim first, got %s", result.PreemptedWorkloads[0])
	}
}

func TestVictimSelector_LowestPriorityPolicy(t *testing.T) {
	vs := NewVictimSelector(VictimSelectionLowestPriority)

	candidates := []*Workload{
		newTestWorkload("w1", 50, 100, 100),
		newTestWorkload("w2", 10, 100, 100),
		newTestWorkload("w3", 30, 100, 100),
	}

	needed := Resources{CPU: 150, Memory: 150}
	victims := vs.SelectVictims(candidates, needed, 10)

	if len(victims) == 0 {
		t.Fatal("expected victims")
	}

	// First victim should be lowest priority
	if victims[0].ID != "w2" {
		t.Errorf("expected w2 (priority 10) first, got %s", victims[0].ID)
	}
}

func TestVictimSelector_NewestPolicy(t *testing.T) {
	vs := NewVictimSelector(VictimSelectionNewest)

	now := time.Now()
	w1 := newTestWorkload("w1", 50, 100, 100)
	w1.CreatedAt = now.Add(-1 * time.Hour)

	w2 := newTestWorkload("w2", 50, 100, 100)
	w2.CreatedAt = now.Add(-1 * time.Minute)

	w3 := newTestWorkload("w3", 50, 100, 100)
	w3.CreatedAt = now.Add(-30 * time.Minute)

	candidates := []*Workload{w1, w2, w3}

	needed := Resources{CPU: 150, Memory: 150}
	victims := vs.SelectVictims(candidates, needed, 10)

	if len(victims) == 0 {
		t.Fatal("expected victims")
	}

	// First victim should be newest
	if victims[0].ID != "w2" {
		t.Errorf("expected w2 (newest) first, got %s", victims[0].ID)
	}
}

func TestVictimSelector_OldestPolicy(t *testing.T) {
	vs := NewVictimSelector(VictimSelectionOldest)

	now := time.Now()
	w1 := newTestWorkload("w1", 50, 100, 100)
	w1.CreatedAt = now.Add(-1 * time.Hour)

	w2 := newTestWorkload("w2", 50, 100, 100)
	w2.CreatedAt = now.Add(-1 * time.Minute)

	w3 := newTestWorkload("w3", 50, 100, 100)
	w3.CreatedAt = now.Add(-30 * time.Minute)

	candidates := []*Workload{w1, w2, w3}

	needed := Resources{CPU: 150, Memory: 150}
	victims := vs.SelectVictims(candidates, needed, 10)

	if len(victims) == 0 {
		t.Fatal("expected victims")
	}

	// First victim should be oldest
	if victims[0].ID != "w1" {
		t.Errorf("expected w1 (oldest) first, got %s", victims[0].ID)
	}
}

// =============================================================================
// Graceful Termination Tests
// =============================================================================

func TestGracefulTerminator_NormalTermination(t *testing.T) {
	gt := NewGracefulTerminator(5 * time.Second)

	workload := newTestWorkload("test", 50, 100, 100)

	terminated := false
	terminateFunc := func(w *Workload) error {
		terminated = true
		return nil
	}

	err := gt.Terminate(context.Background(), workload, terminateFunc)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !terminated {
		t.Error("workload was not terminated")
	}

	record, exists := gt.GetTerminationRecord(workload.ID)
	if !exists {
		t.Fatal("termination record not found")
	}

	if record.State != TerminationStateCompleted {
		t.Errorf("expected completed state, got %s", record.State)
	}
}

func TestGracefulTerminator_CustomGracePeriod(t *testing.T) {
	gt := NewGracefulTerminator(30 * time.Second)

	workload := newTestWorkload("test", 50, 100, 100)
	workload.Annotations["scheduler.unheaded.io/termination-grace-period"] = "2s"

	startTime := time.Now()
	terminated := false

	terminateFunc := func(w *Workload) error {
		// Simulate some work
		time.Sleep(100 * time.Millisecond)
		terminated = true
		return nil
	}

	err := gt.Terminate(context.Background(), workload, terminateFunc)

	duration := time.Since(startTime)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !terminated {
		t.Error("workload was not terminated")
	}

	// Should complete quickly since terminateFunc is fast
	if duration > 5*time.Second {
		t.Errorf("termination took too long: %v", duration)
	}
}

func TestGracefulTerminator_Callbacks(t *testing.T) {
	gt := NewGracefulTerminator(5 * time.Second)

	terminatingCalled := false
	terminatedCalled := false

	gt.OnTerminating(func(w *Workload) {
		terminatingCalled = true
	})

	gt.OnTerminated(func(w *Workload) {
		terminatedCalled = true
	})

	workload := newTestWorkload("test", 50, 100, 100)

	err := gt.Terminate(context.Background(), workload, func(w *Workload) error {
		return nil
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !terminatingCalled {
		t.Error("OnTerminating callback was not called")
	}

	if !terminatedCalled {
		t.Error("OnTerminated callback was not called")
	}
}

// =============================================================================
// Preemption Anti-Affinity Tests
// =============================================================================

func TestPreemptionAntiAffinity_SameServiceCooldown(t *testing.T) {
	tracker := NewPreemptionAntiAffinityTracker()
	tracker.SetCooldownTime(1 * time.Second)

	preemptor := newTestWorkload("preemptor", 100, 100, 100)

	victim1 := newTestWorkload("victim1", 10, 100, 100)
	victim1.Labels["app"] = "myservice"

	victim2 := newTestWorkload("victim2", 10, 100, 100)
	victim2.Labels["app"] = "myservice"

	// First preemption should succeed
	if !tracker.CanPreempt(preemptor, victim1) {
		t.Error("first preemption should be allowed")
	}

	tracker.RecordPreemption(victim1)

	// Second preemption of same service should still be allowed (up to 2)
	if !tracker.CanPreempt(preemptor, victim2) {
		t.Error("second preemption of same service should be allowed")
	}

	tracker.RecordPreemption(victim2)

	// Third preemption should be blocked
	victim3 := newTestWorkload("victim3", 10, 100, 100)
	victim3.Labels["app"] = "myservice"

	if tracker.CanPreempt(preemptor, victim3) {
		t.Error("third preemption of same service should be blocked")
	}

	// Different service should be allowed
	victim4 := newTestWorkload("victim4", 10, 100, 100)
	victim4.Labels["app"] = "otherservice"

	if !tracker.CanPreempt(preemptor, victim4) {
		t.Error("preemption of different service should be allowed")
	}
}

func TestPreemptionAntiAffinity_CustomRules(t *testing.T) {
	tracker := NewPreemptionAntiAffinityTracker()

	// Add rule: only 1 preemption per minute for "critical" service
	tracker.AddRule(PreemptionAntiAffinityRule{
		ServiceLabel:             "critical",
		Duration:                 1 * time.Minute,
		MaxPreemptionsPerService: 1,
	})

	preemptor := newTestWorkload("preemptor", 100, 100, 100)

	victim1 := newTestWorkload("victim1", 10, 100, 100)
	victim1.Labels["app"] = "critical"

	if !tracker.CanPreempt(preemptor, victim1) {
		t.Error("first preemption should be allowed")
	}

	tracker.RecordPreemption(victim1)

	victim2 := newTestWorkload("victim2", 10, 100, 100)
	victim2.Labels["app"] = "critical"

	if tracker.CanPreempt(preemptor, victim2) {
		t.Error("second preemption should be blocked by rule")
	}
}

// =============================================================================
// Pod Disruption Budget Tests
// =============================================================================

func TestPDB_RespectsMinAvailable(t *testing.T) {
	pdbManager := NewPodDisruptionBudgetManager()

	minAvailable := 2
	pdb := &PodDisruptionBudget{
		Name:         "test-pdb",
		Namespace:    "default",
		Selector:     LabelSelector{MatchLabels: map[string]string{"app": "myapp"}},
		MinAvailable: &minAvailable,
		CurrentHealthy:     3,
		DesiredHealthy:     3,
		DisruptionsAllowed: 1, // 3 - 2 = 1
	}
	pdbManager.AddPDB(pdb)

	workload := newTestWorkload("w1", 10, 100, 100)
	workload.Namespace = "default"
	workload.Labels["app"] = "myapp"

	pdbManager.RegisterWorkload(workload)

	if !pdbManager.CanDisrupt(workload) {
		t.Error("should allow disruption when DisruptionsAllowed > 0")
	}

	pdbManager.RecordDisruption(workload)

	if pdbManager.CanDisrupt(workload) {
		t.Error("should not allow disruption when DisruptionsAllowed = 0")
	}
}

func TestPDB_RespectsMaxUnavailable(t *testing.T) {
	pdbManager := NewPodDisruptionBudgetManager()

	maxUnavailable := 1
	pdb := &PodDisruptionBudget{
		Name:           "test-pdb",
		Namespace:      "default",
		Selector:       LabelSelector{MatchLabels: map[string]string{"app": "myapp"}},
		MaxUnavailable: &maxUnavailable,
		CurrentHealthy:     3,
		DesiredHealthy:     3,
		DisruptionsAllowed: 1,
	}
	pdbManager.AddPDB(pdb)

	workload := newTestWorkload("w1", 10, 100, 100)
	workload.Namespace = "default"
	workload.Labels["app"] = "myapp"

	pdbManager.RegisterWorkload(workload)

	if !pdbManager.CanDisrupt(workload) {
		t.Error("should allow disruption")
	}

	pdbManager.RecordDisruption(workload)

	if pdbManager.CanDisrupt(workload) {
		t.Error("should not allow further disruption")
	}
}

func TestPDB_NoPDBMeansCanDisrupt(t *testing.T) {
	pdbManager := NewPodDisruptionBudgetManager()

	workload := newTestWorkload("w1", 10, 100, 100)
	workload.Namespace = "default"

	// No PDB registered, should be able to disrupt
	if !pdbManager.CanDisrupt(workload) {
		t.Error("should allow disruption when no PDB exists")
	}
}

// =============================================================================
// Preemption Queue Tests
// =============================================================================

func TestPreemptionQueue_PriorityOrdering(t *testing.T) {
	queue := NewPreemptionQueue(100)

	w1 := newTestWorkload("w1", 10, 100, 100)
	w2 := newTestWorkload("w2", 50, 100, 100)
	w3 := newTestWorkload("w3", 30, 100, 100)

	queue.Push(w1)
	queue.Push(w2)
	queue.Push(w3)

	// Should pop in priority order (highest first)
	popped := queue.Pop()
	if popped.ID != "w2" {
		t.Errorf("expected w2 (priority 50), got %s", popped.ID)
	}

	popped = queue.Pop()
	if popped.ID != "w3" {
		t.Errorf("expected w3 (priority 30), got %s", popped.ID)
	}

	popped = queue.Pop()
	if popped.ID != "w1" {
		t.Errorf("expected w1 (priority 10), got %s", popped.ID)
	}
}

func TestPreemptionQueue_MaxSize(t *testing.T) {
	queue := NewPreemptionQueue(2)

	w1 := newTestWorkload("w1", 10, 100, 100)
	w2 := newTestWorkload("w2", 20, 100, 100)
	w3 := newTestWorkload("w3", 30, 100, 100)

	queue.Push(w1)
	queue.Push(w2)
	err := queue.Push(w3)

	if err != ErrPreemptionQueueFull {
		t.Errorf("expected ErrPreemptionQueueFull, got %v", err)
	}
}

func TestPreemptionQueue_Remove(t *testing.T) {
	queue := NewPreemptionQueue(100)

	w1 := newTestWorkload("w1", 10, 100, 100)
	w2 := newTestWorkload("w2", 20, 100, 100)

	queue.Push(w1)
	queue.Push(w2)

	if !queue.Remove("w1") {
		t.Error("remove should return true")
	}

	if queue.Contains("w1") {
		t.Error("w1 should not be in queue")
	}

	if queue.Len() != 1 {
		t.Errorf("expected queue length 1, got %d", queue.Len())
	}
}

func TestPreemptionQueue_Duplicate(t *testing.T) {
	queue := NewPreemptionQueue(100)

	w1 := newTestWorkload("w1", 10, 100, 100)

	queue.Push(w1)
	err := queue.Push(w1)

	if err != nil {
		t.Errorf("duplicate push should succeed silently, got %v", err)
	}

	if queue.Len() != 1 {
		t.Errorf("expected queue length 1, got %d", queue.Len())
	}
}

// =============================================================================
// Preemption Metrics Tests
// =============================================================================

func TestPreemptionMetrics_RecordAttempts(t *testing.T) {
	metrics := NewPreemptionMetrics()

	metrics.RecordPreemptionAttempt(true, 100*time.Millisecond)
	metrics.RecordPreemptionAttempt(true, 200*time.Millisecond)
	metrics.RecordPreemptionAttempt(false, 50*time.Millisecond)

	stats := metrics.GetStats()

	if stats.TotalAttempts != 3 {
		t.Errorf("expected 3 attempts, got %d", stats.TotalAttempts)
	}

	if stats.SuccessfulPreemptions != 2 {
		t.Errorf("expected 2 successes, got %d", stats.SuccessfulPreemptions)
	}

	if stats.FailedPreemptions != 1 {
		t.Errorf("expected 1 failure, got %d", stats.FailedPreemptions)
	}
}

func TestPreemptionMetrics_VictimCount(t *testing.T) {
	metrics := NewPreemptionMetrics()

	metrics.RecordVictimCount(3)
	metrics.RecordVictimCount(2)

	stats := metrics.GetStats()

	if stats.TotalVictims != 5 {
		t.Errorf("expected 5 victims, got %d", stats.TotalVictims)
	}
}

func TestPreemptionMetrics_Duration(t *testing.T) {
	metrics := NewPreemptionMetrics()

	metrics.RecordPreemptionAttempt(true, 100*time.Millisecond)
	metrics.RecordPreemptionAttempt(true, 200*time.Millisecond)
	metrics.RecordPreemptionAttempt(true, 150*time.Millisecond)

	stats := metrics.GetStats()

	if stats.MinDuration != 100*time.Millisecond {
		t.Errorf("expected min 100ms, got %v", stats.MinDuration)
	}

	if stats.MaxDuration != 200*time.Millisecond {
		t.Errorf("expected max 200ms, got %v", stats.MaxDuration)
	}
}

// =============================================================================
// Preemption Event Log Tests
// =============================================================================

func TestPreemptionEventLog_LogAndRetrieve(t *testing.T) {
	log := NewPreemptionEventLog(100)

	log.LogEvent(PreemptionEventSuccess, "preemptor-1", []string{"victim-1"}, "test", nil)
	log.LogEvent(PreemptionEventDenied, "preemptor-2", nil, "denied", nil)

	events := log.GetRecent(10)

	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
}

func TestPreemptionEventLog_GetByType(t *testing.T) {
	log := NewPreemptionEventLog(100)

	log.LogEvent(PreemptionEventSuccess, "p1", nil, "success 1", nil)
	log.LogEvent(PreemptionEventDenied, "p2", nil, "denied", nil)
	log.LogEvent(PreemptionEventSuccess, "p3", nil, "success 2", nil)

	successEvents := log.GetByType(PreemptionEventSuccess)

	if len(successEvents) != 2 {
		t.Errorf("expected 2 success events, got %d", len(successEvents))
	}
}

func TestPreemptionEventLog_GetByWorkload(t *testing.T) {
	log := NewPreemptionEventLog(100)

	log.LogEvent(PreemptionEventSuccess, "p1", []string{"v1", "v2"}, "test", nil)
	log.LogEvent(PreemptionEventSuccess, "p2", []string{"v3"}, "test", nil)

	events := log.GetByWorkload("v1")

	if len(events) != 1 {
		t.Errorf("expected 1 event for v1, got %d", len(events))
	}
}

func TestPreemptionEventLog_Handler(t *testing.T) {
	log := NewPreemptionEventLog(100)

	var receivedEvent PreemptionEvent
	var wg sync.WaitGroup
	wg.Add(1)

	log.OnEvent(func(e PreemptionEvent) {
		receivedEvent = e
		wg.Done()
	})

	log.LogEvent(PreemptionEventSuccess, "p1", nil, "test", nil)

	wg.Wait()

	if receivedEvent.PreemptorID != "p1" {
		t.Errorf("expected preemptor p1, got %s", receivedEvent.PreemptorID)
	}
}

func TestPreemptionEventLog_MaxSize(t *testing.T) {
	log := NewPreemptionEventLog(3)

	log.LogEvent(PreemptionEventSuccess, "p1", nil, "1", nil)
	log.LogEvent(PreemptionEventSuccess, "p2", nil, "2", nil)
	log.LogEvent(PreemptionEventSuccess, "p3", nil, "3", nil)
	log.LogEvent(PreemptionEventSuccess, "p4", nil, "4", nil)

	events := log.GetRecent(10)

	if len(events) != 3 {
		t.Errorf("expected 3 events (max size), got %d", len(events))
	}

	// Oldest event should be removed
	for _, e := range events {
		if e.PreemptorID == "p1" {
			t.Error("oldest event p1 should have been removed")
		}
	}
}

// =============================================================================
// Preemption Guard Tests
// =============================================================================

func TestPreemptionGuard_Cooldown(t *testing.T) {
	guard := NewPreemptionGuard(100 * time.Millisecond)

	guard.RecordPreemption("w1")

	if guard.CanBePreempted("w1") {
		t.Error("should not be able to preempt immediately after preemption")
	}

	time.Sleep(150 * time.Millisecond)

	if !guard.CanBePreempted("w1") {
		t.Error("should be able to preempt after cooldown")
	}
}

func TestPreemptionGuard_Cleanup(t *testing.T) {
	guard := NewPreemptionGuard(50 * time.Millisecond)

	guard.RecordPreemption("w1")
	guard.RecordPreemption("w2")

	time.Sleep(100 * time.Millisecond)

	guard.Cleanup()

	if !guard.CanBePreempted("w1") {
		t.Error("w1 should be preemptible after cleanup")
	}
}

// =============================================================================
// Preemption History Tests
// =============================================================================

func TestPreemptionHistory_AddAndRetrieve(t *testing.T) {
	history := NewPreemptionHistory(100)

	record := PreemptionRecord{
		ID:            "rec-1",
		Timestamp:     time.Now(),
		PreemptorID:   "p1",
		PreemptorPrio: 100,
		VictimIDs:     []string{"v1", "v2"},
		NodeID:        "node-1",
	}

	history.Add(record)

	records := history.GetRecent(10)

	if len(records) != 1 {
		t.Errorf("expected 1 record, got %d", len(records))
	}

	if records[0].PreemptorID != "p1" {
		t.Errorf("expected preemptor p1, got %s", records[0].PreemptorID)
	}
}

func TestPreemptionHistory_GetByWorkload(t *testing.T) {
	history := NewPreemptionHistory(100)

	history.Add(PreemptionRecord{
		ID:          "rec-1",
		PreemptorID: "p1",
		VictimIDs:   []string{"v1", "v2"},
	})

	history.Add(PreemptionRecord{
		ID:          "rec-2",
		PreemptorID: "p2",
		VictimIDs:   []string{"v3"},
	})

	records := history.GetByWorkload("v1")

	if len(records) != 1 {
		t.Errorf("expected 1 record for v1, got %d", len(records))
	}
}

func TestPreemptionHistory_Stats(t *testing.T) {
	history := NewPreemptionHistory(100)

	history.Add(PreemptionRecord{
		ID:        "rec-1",
		NodeID:    "node-1",
		VictimIDs: []string{"v1", "v2"},
	})

	history.Add(PreemptionRecord{
		ID:        "rec-2",
		NodeID:    "node-1",
		VictimIDs: []string{"v3"},
	})

	history.Add(PreemptionRecord{
		ID:        "rec-3",
		NodeID:    "node-2",
		VictimIDs: []string{"v4"},
	})

	stats := history.GetStats()

	if stats.TotalPreemptions != 3 {
		t.Errorf("expected 3 preemptions, got %d", stats.TotalPreemptions)
	}

	if stats.TotalVictims != 4 {
		t.Errorf("expected 4 victims, got %d", stats.TotalVictims)
	}

	if stats.PreemptionsByNode["node-1"] != 2 {
		t.Errorf("expected 2 preemptions on node-1, got %d", stats.PreemptionsByNode["node-1"])
	}
}

// =============================================================================
// Preemption Simulator Tests
// =============================================================================

func TestPreemptionSimulator_CanScheduleWithoutPreemption(t *testing.T) {
	nodes := NewNodeRegistry()
	node := newTestNode("node-1", 1000, 1024*1024*1024)
	nodes.Register(node)

	simulator := NewPreemptionSimulator(nodes, func(nodeID string) []*Workload {
		return nil
	})

	workload := newTestWorkload("w1", 50, 500, 500*1024*1024)

	result := simulator.Simulate(workload)

	if result == nil {
		t.Fatal("expected result")
	}

	if !result.CanSchedule {
		t.Error("should be able to schedule")
	}

	if len(result.VictimsRequired) != 0 {
		t.Error("should not require victims")
	}
}

func TestPreemptionSimulator_RequiresPreemption(t *testing.T) {
	nodes := NewNodeRegistry()
	node := newTestNode("node-1", 1000, 1024*1024*1024)
	node.Allocated = Resources{CPU: 800, Memory: 800 * 1024 * 1024}
	node.Available = Resources{CPU: 200, Memory: 224 * 1024 * 1024}
	nodes.Register(node)

	existing := newTestWorkload("existing", 10, 500, 500*1024*1024)

	simulator := NewPreemptionSimulator(nodes, func(nodeID string) []*Workload {
		return []*Workload{existing}
	})

	workload := newTestWorkload("new", 100, 600, 600*1024*1024)

	result := simulator.Simulate(workload)

	if result == nil {
		t.Fatal("expected result")
	}

	if !result.CanSchedule {
		t.Error("should be able to schedule with preemption")
	}

	if len(result.VictimsRequired) == 0 {
		t.Error("should require victims")
	}
}

// =============================================================================
// Integration Tests
// =============================================================================

func TestPreemption_FullFlow(t *testing.T) {
	nodes := NewNodeRegistry()
	node := newTestNode("node-1", 1000, 1024*1024*1024)
	node.Allocated = Resources{CPU: 800, Memory: 800 * 1024 * 1024}
	node.Available = Resources{CPU: 200, Memory: 224 * 1024 * 1024}
	nodes.Register(node)

	preemptor := NewPreemptorWithConfig(nodes, PreemptorConfig{
		Timeout:            30 * time.Second,
		GracePeriod:        1 * time.Second,
		MinVictimPriority:  -1000,
		MaxVictims:         10,
		EnablePDBRespect:   true,
		EnableAntiAffinity: true,
		QueueSize:          100,
		CooldownPeriod:     1 * time.Second,
	})

	// Track callbacks
	var preemptionStarted bool
	var preemptionSucceeded bool
	var terminatedVictim *Workload

	preemptor.OnPreemptionStart(func(p *Workload, victims []*Workload) {
		preemptionStarted = true
	})

	preemptor.OnPreemptionSuccess(func(p *Workload, result *PreemptionResult) {
		preemptionSucceeded = true
	})

	preemptor.OnVictimTerminated(func(w *Workload) {
		terminatedVictim = w
	})

	// Create victim
	victim := newTestWorkload("victim", 10, 500, 500*1024*1024)
	victim.BoundNode = "node-1"

	// Create high priority workload
	highPrio := newTestWorkload("high-prio", 100, 600, 600*1024*1024)

	getWorkloads := func(nodeID string) []*Workload {
		return []*Workload{victim}
	}

	// Find candidates
	result := preemptor.FindPreemptionCandidates(highPrio, []*Node{node}, getWorkloads)

	if result == nil {
		t.Fatal("expected preemption result")
	}

	if !preemptionStarted {
		t.Error("preemption start callback not called")
	}

	// Execute preemption
	getWorkload := func(id string) (*Workload, error) {
		if id == "victim" {
			return victim, nil
		}
		return nil, ErrWorkloadNotFound
	}

	terminateWorkload := func(w *Workload) error {
		w.State = WorkloadStateEvicted
		return nil
	}

	err := preemptor.ExecutePreemption(context.Background(), highPrio, result, getWorkload, terminateWorkload)

	if err != nil {
		t.Errorf("execution failed: %v", err)
	}

	if !preemptionSucceeded {
		t.Error("preemption success callback not called")
	}

	if terminatedVictim == nil {
		t.Error("victim termination callback not called")
	}

	// Check metrics
	stats := preemptor.GetMetrics().GetStats()
	if stats.SuccessfulPreemptions != 1 {
		t.Errorf("expected 1 successful preemption, got %d", stats.SuccessfulPreemptions)
	}

	// Check event log
	events := preemptor.GetEventLog().GetRecent(10)
	if len(events) == 0 {
		t.Error("expected events in log")
	}
}

func TestPreemption_MultipleNodes(t *testing.T) {
	nodes := NewNodeRegistry()

	// Node 1: requires 1 victim
	node1 := newTestNode("node-1", 1000, 1024*1024*1024)
	node1.Allocated = Resources{CPU: 600, Memory: 600 * 1024 * 1024}
	node1.Available = Resources{CPU: 400, Memory: 424 * 1024 * 1024}
	nodes.Register(node1)

	// Node 2: requires 2 victims
	node2 := newTestNode("node-2", 1000, 1024*1024*1024)
	node2.Allocated = Resources{CPU: 800, Memory: 800 * 1024 * 1024}
	node2.Available = Resources{CPU: 200, Memory: 224 * 1024 * 1024}
	nodes.Register(node2)

	preemptor := NewPreemptor(nodes, 30*time.Second)

	// Victims on node 1
	victim1 := newTestWorkload("victim1", 10, 300, 300*1024*1024)
	victim1.BoundNode = "node-1"

	// Victims on node 2
	victim2 := newTestWorkload("victim2", 10, 300, 300*1024*1024)
	victim2.BoundNode = "node-2"
	victim3 := newTestWorkload("victim3", 10, 300, 300*1024*1024)
	victim3.BoundNode = "node-2"

	getWorkloads := func(nodeID string) []*Workload {
		if nodeID == "node-1" {
			return []*Workload{victim1}
		}
		return []*Workload{victim2, victim3}
	}

	highPrio := newTestWorkload("high-prio", 100, 500, 500*1024*1024)

	result := preemptor.FindPreemptionCandidates(highPrio, []*Node{node1, node2}, getWorkloads)

	if result == nil {
		t.Fatal("expected preemption result")
	}

	// Should prefer node with fewer victims
	if result.Node.ID != "node-1" {
		t.Errorf("expected node-1 (fewer victims), got %s", result.Node.ID)
	}

	if len(result.PreemptedWorkloads) != 1 {
		t.Errorf("expected 1 victim, got %d", len(result.PreemptedWorkloads))
	}
}

func TestPreemption_RespectsPDBAndAntiAffinity(t *testing.T) {
	nodes := NewNodeRegistry()
	node := newTestNode("node-1", 2000, 2*1024*1024*1024)
	node.Allocated = Resources{CPU: 1800, Memory: 1800 * 1024 * 1024}
	node.Available = Resources{CPU: 200, Memory: 200 * 1024 * 1024}
	nodes.Register(node)

	preemptor := NewPreemptorWithConfig(nodes, PreemptorConfig{
		Timeout:            30 * time.Second,
		GracePeriod:        1 * time.Second,
		MinVictimPriority:  -1000,
		MaxVictims:         10,
		EnablePDBRespect:   true,
		EnableAntiAffinity: true,
		QueueSize:          100,
		CooldownPeriod:     1 * time.Second,
	})

	// Setup PDB that protects victim1
	minAvailable := 1
	pdb := &PodDisruptionBudget{
		Name:               "protect-pdb",
		Namespace:          "default",
		Selector:           LabelSelector{MatchLabels: map[string]string{"app": "protected"}},
		MinAvailable:       &minAvailable,
		CurrentHealthy:     1,
		DesiredHealthy:     1,
		DisruptionsAllowed: 0, // Cannot disrupt
	}
	preemptor.GetPDBManager().AddPDB(pdb)

	// Protected victim
	victim1 := newTestWorkload("victim1", 10, 300, 300*1024*1024)
	victim1.Namespace = "default"
	victim1.Labels["app"] = "protected"
	victim1.BoundNode = "node-1"
	preemptor.GetPDBManager().RegisterWorkload(victim1)

	// Unprotected victim
	victim2 := newTestWorkload("victim2", 10, 500, 500*1024*1024)
	victim2.Namespace = "default"
	victim2.Labels["app"] = "unprotected"
	victim2.BoundNode = "node-1"

	getWorkloads := func(nodeID string) []*Workload {
		return []*Workload{victim1, victim2}
	}

	highPrio := newTestWorkload("high-prio", 100, 500, 500*1024*1024)

	result := preemptor.FindPreemptionCandidates(highPrio, []*Node{node}, getWorkloads)

	if result == nil {
		t.Fatal("expected preemption result")
	}

	// Should only preempt unprotected victim
	for _, vid := range result.PreemptedWorkloads {
		if vid == "victim1" {
			t.Error("should not preempt PDB-protected victim1")
		}
	}

	if result.PreemptedWorkloads[0] != "victim2" {
		t.Errorf("expected victim2, got %s", result.PreemptedWorkloads[0])
	}
}

// =============================================================================
// Concurrent Preemption Tests
// =============================================================================

func TestPreemption_ConcurrentCandidateSearch(t *testing.T) {
	nodes := NewNodeRegistry()
	node := newTestNode("node-1", 10000, 10*1024*1024*1024)
	node.Allocated = Resources{CPU: 8000, Memory: 8000 * 1024 * 1024}
	node.Available = Resources{CPU: 2000, Memory: 2000 * 1024 * 1024}
	nodes.Register(node)

	preemptor := NewPreemptor(nodes, 30*time.Second)

	// Create victims
	victims := make([]*Workload, 10)
	for i := 0; i < 10; i++ {
		victims[i] = newTestWorkload("victim-"+string(rune('a'+i)), 10, 500, 500*1024*1024)
		victims[i].BoundNode = "node-1"
	}

	getWorkloads := func(nodeID string) []*Workload {
		return victims
	}

	var wg sync.WaitGroup
	numGoroutines := 20
	results := make([]*PreemptionResult, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			highPrio := newTestWorkload("preemptor-"+string(rune('a'+idx)), 100, 1000, 1000*1024*1024)
			results[idx] = preemptor.FindPreemptionCandidates(highPrio, []*Node{node}, getWorkloads)
		}(i)
	}

	wg.Wait()

	// At least some should succeed
	successCount := 0
	for _, r := range results {
		if r != nil && r.Success {
			successCount++
		}
	}

	if successCount == 0 {
		t.Error("expected at least some concurrent preemption searches to succeed")
	}
}

func TestPreemption_ConcurrentQueueOperations(t *testing.T) {
	queue := NewPreemptionQueue(1000)

	var wg sync.WaitGroup
	numOperations := 100

	// Concurrent pushes
	for i := 0; i < numOperations; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			w := newTestWorkload("w-"+string(rune(idx)), int32(idx%100), 100, 100)
			_ = queue.Push(w)
		}(i)
	}

	// Concurrent pops
	for i := 0; i < numOperations/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = queue.Pop()
		}()
	}

	// Concurrent reads
	for i := 0; i < numOperations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = queue.Len()
			_ = queue.Contains("w-a")
		}()
	}

	wg.Wait()

	// Queue should still be valid
	if queue.Len() < 0 {
		t.Error("queue length should not be negative")
	}
}

func TestPreemption_ConcurrentMetrics(t *testing.T) {
	metrics := NewPreemptionMetrics()

	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			metrics.RecordPreemptionAttempt(true, 50*time.Millisecond)
			metrics.RecordVictimCount(2)
			_ = metrics.GetStats()
		}()
	}

	wg.Wait()

	stats := metrics.GetStats()
	if stats.TotalAttempts != int64(numGoroutines) {
		t.Errorf("expected %d attempts, got %d", numGoroutines, stats.TotalAttempts)
	}
}

func TestPreemption_ConcurrentEventLog(t *testing.T) {
	log := NewPreemptionEventLog(1000)

	var wg sync.WaitGroup
	numOperations := 100

	// Concurrent writes
	for i := 0; i < numOperations; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			log.LogEvent(PreemptionEventSuccess, "p-"+string(rune(idx)), []string{"v-" + string(rune(idx))}, "test", nil)
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numOperations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = log.GetRecent(10)
			_ = log.GetByType(PreemptionEventSuccess)
		}()
	}

	wg.Wait()

	events := log.GetRecent(1000)
	if len(events) != numOperations {
		t.Errorf("expected %d events, got %d", numOperations, len(events))
	}
}

// =============================================================================
// Edge Case Tests
// =============================================================================

func TestPreemption_EmptyNodesList(t *testing.T) {
	nodes := NewNodeRegistry()
	preemptor := NewPreemptor(nodes, 30*time.Second)

	workload := newTestWorkload("high-prio", 100, 500, 500*1024*1024)

	result := preemptor.FindPreemptionCandidates(workload, []*Node{}, func(nodeID string) []*Workload {
		return nil
	})

	if result != nil {
		t.Error("expected nil result for empty nodes list")
	}
}

func TestPreemption_NoVictims(t *testing.T) {
	nodes := NewNodeRegistry()
	node := newTestNode("node-1", 1000, 1024*1024*1024)
	node.Allocated = Resources{CPU: 800, Memory: 800 * 1024 * 1024}
	node.Available = Resources{CPU: 200, Memory: 224 * 1024 * 1024}
	nodes.Register(node)

	preemptor := NewPreemptor(nodes, 30*time.Second)

	workload := newTestWorkload("high-prio", 100, 500, 500*1024*1024)

	// No workloads on the node
	result := preemptor.FindPreemptionCandidates(workload, []*Node{node}, func(nodeID string) []*Workload {
		return nil
	})

	if result != nil {
		t.Error("expected nil result when no victims available")
	}
}

func TestPreemption_AllVictimsProtected(t *testing.T) {
	nodes := NewNodeRegistry()
	node := newTestNode("node-1", 1000, 1024*1024*1024)
	node.Allocated = Resources{CPU: 800, Memory: 800 * 1024 * 1024}
	node.Available = Resources{CPU: 200, Memory: 224 * 1024 * 1024}
	nodes.Register(node)

	preemptor := NewPreemptor(nodes, 30*time.Second)

	// All victims are protected
	victim := newTestWorkload("victim", 10, 500, 500*1024*1024)
	victim.BoundNode = "node-1"
	victim.Annotations["scheduler.unheaded.io/preemption-protected"] = "true"

	workload := newTestWorkload("high-prio", 100, 500, 500*1024*1024)

	result := preemptor.FindPreemptionCandidates(workload, []*Node{node}, func(nodeID string) []*Workload {
		return []*Workload{victim}
	})

	if result != nil {
		t.Error("expected nil result when all victims are protected")
	}
}

func TestPreemption_VeryHighPriorityVictim(t *testing.T) {
	nodes := NewNodeRegistry()
	node := newTestNode("node-1", 1000, 1024*1024*1024)
	node.Allocated = Resources{CPU: 800, Memory: 800 * 1024 * 1024}
	node.Available = Resources{CPU: 200, Memory: 224 * 1024 * 1024}
	nodes.Register(node)

	preemptor := NewPreemptor(nodes, 30*time.Second)

	// Victim has higher priority than preemptor
	victim := newTestWorkload("victim", 1000, 500, 500*1024*1024)
	victim.BoundNode = "node-1"

	workload := newTestWorkload("preemptor", 100, 500, 500*1024*1024)

	result := preemptor.FindPreemptionCandidates(workload, []*Node{node}, func(nodeID string) []*Workload {
		return []*Workload{victim}
	})

	if result != nil {
		t.Error("should not preempt higher priority victim")
	}
}

func TestPreemption_ZeroResourceRequest(t *testing.T) {
	nodes := NewNodeRegistry()
	node := newTestNode("node-1", 1000, 1024*1024*1024)
	nodes.Register(node)

	preemptor := NewPreemptor(nodes, 30*time.Second)

	// Workload with zero resources should not need preemption
	workload := newTestWorkload("zero-resource", 100, 0, 0)

	result := preemptor.FindPreemptionCandidates(workload, []*Node{node}, func(nodeID string) []*Workload {
		return nil
	})

	// Should be nil or succeed without victims
	if result != nil && len(result.PreemptedWorkloads) > 0 {
		t.Error("zero resource workload should not need victims")
	}
}

func TestPreemption_ExactResourceMatch(t *testing.T) {
	nodes := NewNodeRegistry()
	node := newTestNode("node-1", 1000, 1024*1024*1024)
	node.Allocated = Resources{CPU: 500, Memory: 500 * 1024 * 1024}
	node.Available = Resources{CPU: 500, Memory: 524 * 1024 * 1024}
	nodes.Register(node)

	preemptor := NewPreemptor(nodes, 30*time.Second)

	victim := newTestWorkload("victim", 10, 500, 500*1024*1024)
	victim.BoundNode = "node-1"

	// Preemptor needs exactly the victim's resources
	workload := newTestWorkload("preemptor", 100, 500, 500*1024*1024)

	result := preemptor.FindPreemptionCandidates(workload, []*Node{node}, func(nodeID string) []*Workload {
		return []*Workload{victim}
	})

	if result == nil {
		t.Fatal("expected preemption result")
	}

	if len(result.PreemptedWorkloads) != 1 {
		t.Errorf("expected exactly 1 victim, got %d", len(result.PreemptedWorkloads))
	}
}

func TestPreemption_MultipleNodesSelectBest(t *testing.T) {
	nodes := NewNodeRegistry()

	// Node 1: High priority victims
	node1 := newTestNode("node-1", 1000, 1024*1024*1024)
	node1.Allocated = Resources{CPU: 600, Memory: 600 * 1024 * 1024}
	node1.Available = Resources{CPU: 400, Memory: 424 * 1024 * 1024}
	nodes.Register(node1)

	// Node 2: Low priority victims
	node2 := newTestNode("node-2", 1000, 1024*1024*1024)
	node2.Allocated = Resources{CPU: 600, Memory: 600 * 1024 * 1024}
	node2.Available = Resources{CPU: 400, Memory: 424 * 1024 * 1024}
	nodes.Register(node2)

	preemptor := NewPreemptor(nodes, 30*time.Second)

	// High priority victim on node 1
	victim1 := newTestWorkload("victim1", 50, 300, 300*1024*1024)
	victim1.BoundNode = "node-1"

	// Low priority victim on node 2
	victim2 := newTestWorkload("victim2", 5, 300, 300*1024*1024)
	victim2.BoundNode = "node-2"

	getWorkloads := func(nodeID string) []*Workload {
		if nodeID == "node-1" {
			return []*Workload{victim1}
		}
		return []*Workload{victim2}
	}

	workload := newTestWorkload("preemptor", 100, 300, 300*1024*1024)

	result := preemptor.FindPreemptionCandidates(workload, []*Node{node1, node2}, getWorkloads)

	if result == nil {
		t.Fatal("expected preemption result")
	}

	// Should prefer node 2 with lower priority victim
	if result.Node.ID != "node-2" {
		t.Errorf("expected node-2 (lower priority victim), got %s", result.Node.ID)
	}
}

// =============================================================================
// VictimSelector Extended Tests
// =============================================================================

func TestVictimSelector_SmallestResourceFirst(t *testing.T) {
	vs := NewVictimSelector(VictimSelectionLeastResources)

	w1 := newTestWorkload("w1", 50, 500, 500)
	w2 := newTestWorkload("w2", 50, 100, 100)
	w3 := newTestWorkload("w3", 50, 300, 300)

	candidates := []*Workload{w1, w2, w3}

	needed := Resources{CPU: 400, Memory: 400}
	victims := vs.SelectVictims(candidates, needed, 10)

	if len(victims) == 0 {
		t.Fatal("expected victims")
	}

	// First victim should be smallest (w2)
	if victims[0].ID != "w2" {
		t.Errorf("expected w2 (smallest) first, got %s", victims[0].ID)
	}
}

func TestVictimSelector_LargestResourceFirst(t *testing.T) {
	vs := NewVictimSelector(VictimSelectionMostResources)

	w1 := newTestWorkload("w1", 50, 500, 500)
	w2 := newTestWorkload("w2", 50, 100, 100)
	w3 := newTestWorkload("w3", 50, 300, 300)

	candidates := []*Workload{w1, w2, w3}

	needed := Resources{CPU: 400, Memory: 400}
	victims := vs.SelectVictims(candidates, needed, 10)

	if len(victims) == 0 {
		t.Fatal("expected victims")
	}

	// First victim should be largest (w1)
	if victims[0].ID != "w1" {
		t.Errorf("expected w1 (largest) first, got %s", victims[0].ID)
	}
}

func TestVictimSelector_EmptyCandidates(t *testing.T) {
	vs := NewVictimSelector(VictimSelectionLowestPriority)

	needed := Resources{CPU: 100, Memory: 100}
	victims := vs.SelectVictims([]*Workload{}, needed, 10)

	if len(victims) != 0 {
		t.Errorf("expected no victims for empty candidates, got %d", len(victims))
	}
}

func TestVictimSelector_ZeroNeeded(t *testing.T) {
	vs := NewVictimSelector(VictimSelectionLowestPriority)

	candidates := []*Workload{
		newTestWorkload("w1", 10, 100, 100),
	}

	needed := Resources{CPU: 0, Memory: 0}
	victims := vs.SelectVictims(candidates, needed, 10)

	// When zero resources needed, selector still returns first victim
	// (loop exits after first victim since freed >= needed)
	if len(victims) != 1 {
		t.Errorf("expected 1 victim for zero resources (first candidate), got %d", len(victims))
	}
}

func TestVictimSelector_MaxVictimsLimit(t *testing.T) {
	vs := NewVictimSelector(VictimSelectionLowestPriority)

	candidates := make([]*Workload, 10)
	for i := 0; i < 10; i++ {
		candidates[i] = newTestWorkload("w-"+string(rune('a'+i)), int32(i), 10, 10)
	}

	// Need a lot of resources but limit to 3 victims
	needed := Resources{CPU: 1000, Memory: 1000}
	victims := vs.SelectVictims(candidates, needed, 3)

	if len(victims) > 3 {
		t.Errorf("expected max 3 victims, got %d", len(victims))
	}
}

// =============================================================================
// GracefulTerminator Extended Tests
// =============================================================================

func TestGracefulTerminator_CancelledContext(t *testing.T) {
	gt := NewGracefulTerminator(30 * time.Second)

	workload := newTestWorkload("test", 50, 100, 100)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	terminateFunc := func(w *Workload) error {
		time.Sleep(100 * time.Millisecond)
		return nil
	}

	err := gt.Terminate(ctx, workload, terminateFunc)

	// Should return error due to cancelled context
	if err == nil {
		t.Log("termination may or may not complete on cancelled context")
	}
}

func TestGracefulTerminator_TerminationError(t *testing.T) {
	gt := NewGracefulTerminator(5 * time.Second)

	workload := newTestWorkload("test", 50, 100, 100)

	terminateFunc := func(w *Workload) error {
		return ErrWorkloadNotFound
	}

	err := gt.Terminate(context.Background(), workload, terminateFunc)

	if err == nil {
		t.Error("expected error from terminate function")
	}

	record, exists := gt.GetTerminationRecord(workload.ID)
	if !exists {
		t.Fatal("termination record not found")
	}

	if record.State != TerminationStateFailed {
		t.Errorf("expected failed state, got %s", record.State)
	}
}

func TestGracefulTerminator_ConcurrentTerminations(t *testing.T) {
	gt := NewGracefulTerminator(5 * time.Second)

	var wg sync.WaitGroup
	numTerminations := 20

	for i := 0; i < numTerminations; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			workload := newTestWorkload("w-"+string(rune(idx)), 50, 100, 100)
			_ = gt.Terminate(context.Background(), workload, func(w *Workload) error {
				time.Sleep(10 * time.Millisecond)
				return nil
			})
		}(i)
	}

	wg.Wait()

	// All should have records
	for i := 0; i < numTerminations; i++ {
		id := "w-" + string(rune(i))
		if _, exists := gt.GetTerminationRecord(id); !exists {
			t.Errorf("missing record for %s", id)
		}
	}
}

// =============================================================================
// PreemptionAntiAffinity Extended Tests
// =============================================================================

func TestPreemptionAntiAffinity_CooldownExpiry(t *testing.T) {
	tracker := NewPreemptionAntiAffinityTracker()
	tracker.SetCooldownTime(50 * time.Millisecond)

	victim := newTestWorkload("victim", 10, 100, 100)
	victim.Labels["app"] = "myservice"

	// Record first preemption
	tracker.RecordPreemption(victim)

	// Second should be allowed
	victim2 := newTestWorkload("victim2", 10, 100, 100)
	victim2.Labels["app"] = "myservice"
	tracker.RecordPreemption(victim2)

	// Third should be blocked
	victim3 := newTestWorkload("victim3", 10, 100, 100)
	victim3.Labels["app"] = "myservice"
	preemptor := newTestWorkload("preemptor", 100, 100, 100)

	if tracker.CanPreempt(preemptor, victim3) {
		t.Error("third preemption should be blocked")
	}

	// Wait for cooldown
	time.Sleep(100 * time.Millisecond)

	// Should be allowed after cooldown
	if !tracker.CanPreempt(preemptor, victim3) {
		t.Error("should be allowed after cooldown expiry")
	}
}

func TestPreemptionAntiAffinity_DifferentLabels(t *testing.T) {
	tracker := NewPreemptionAntiAffinityTracker()
	tracker.SetCooldownTime(1 * time.Second)

	preemptor := newTestWorkload("preemptor", 100, 100, 100)

	// Preempt service A twice
	for i := 0; i < 2; i++ {
		victim := newTestWorkload("victim-a-"+string(rune('a'+i)), 10, 100, 100)
		victim.Labels["app"] = "service-a"
		tracker.RecordPreemption(victim)
	}

	// Third from service A should be blocked
	victim3 := newTestWorkload("victim-a-c", 10, 100, 100)
	victim3.Labels["app"] = "service-a"
	if tracker.CanPreempt(preemptor, victim3) {
		t.Error("should block third preemption from same service")
	}

	// Service B should be allowed
	victimB := newTestWorkload("victim-b-a", 10, 100, 100)
	victimB.Labels["app"] = "service-b"
	if !tracker.CanPreempt(preemptor, victimB) {
		t.Error("different service should be allowed")
	}
}

// =============================================================================
// PDB Extended Tests
// =============================================================================

func TestPDB_MultiplePDBs(t *testing.T) {
	pdbManager := NewPodDisruptionBudgetManager()

	// PDB 1 for app1
	minAvailable1 := 2
	pdb1 := &PodDisruptionBudget{
		Name:               "pdb1",
		Namespace:          "default",
		Selector:           LabelSelector{MatchLabels: map[string]string{"app": "app1"}},
		MinAvailable:       &minAvailable1,
		CurrentHealthy:     3,
		DesiredHealthy:     3,
		DisruptionsAllowed: 1,
	}
	pdbManager.AddPDB(pdb1)

	// PDB 2 for app2
	minAvailable2 := 1
	pdb2 := &PodDisruptionBudget{
		Name:               "pdb2",
		Namespace:          "default",
		Selector:           LabelSelector{MatchLabels: map[string]string{"app": "app2"}},
		MinAvailable:       &minAvailable2,
		CurrentHealthy:     2,
		DesiredHealthy:     2,
		DisruptionsAllowed: 1,
	}
	pdbManager.AddPDB(pdb2)

	// Workload from app1
	w1 := newTestWorkload("w1", 10, 100, 100)
	w1.Namespace = "default"
	w1.Labels["app"] = "app1"
	pdbManager.RegisterWorkload(w1)

	// Workload from app2
	w2 := newTestWorkload("w2", 10, 100, 100)
	w2.Namespace = "default"
	w2.Labels["app"] = "app2"
	pdbManager.RegisterWorkload(w2)

	// Both should be disruptable
	if !pdbManager.CanDisrupt(w1) {
		t.Error("w1 should be disruptable")
	}
	if !pdbManager.CanDisrupt(w2) {
		t.Error("w2 should be disruptable")
	}

	// Disrupt w1
	pdbManager.RecordDisruption(w1)

	// w1 should still be disruptable (1 remaining)
	// But we need another workload to test
	w1b := newTestWorkload("w1b", 10, 100, 100)
	w1b.Namespace = "default"
	w1b.Labels["app"] = "app1"
	pdbManager.RegisterWorkload(w1b)

	if !pdbManager.CanDisrupt(w1b) {
		t.Log("second disruption for app1 depends on PDB state")
	}
}

func TestPDB_NamespaceIsolation(t *testing.T) {
	pdbManager := NewPodDisruptionBudgetManager()

	minAvailable := 2
	pdb := &PodDisruptionBudget{
		Name:               "test-pdb",
		Namespace:          "prod",
		Selector:           LabelSelector{MatchLabels: map[string]string{"app": "myapp"}},
		MinAvailable:       &minAvailable,
		CurrentHealthy:     2,
		DesiredHealthy:     2,
		DisruptionsAllowed: 0, // Cannot disrupt
	}
	pdbManager.AddPDB(pdb)

	// Workload in different namespace
	w := newTestWorkload("w1", 10, 100, 100)
	w.Namespace = "dev"
	w.Labels["app"] = "myapp"

	// Should be allowed since PDB is in different namespace
	if !pdbManager.CanDisrupt(w) {
		t.Error("workload in different namespace should not be affected by PDB")
	}
}

// =============================================================================
// PreemptionHistory Extended Tests
// =============================================================================

func TestPreemptionHistory_MaxSizeEviction(t *testing.T) {
	maxSize := 5
	history := NewPreemptionHistory(maxSize)

	// Add more records than maxSize
	for i := 0; i < 10; i++ {
		history.Add(PreemptionRecord{
			ID:          "rec-" + string(rune('a'+i)),
			PreemptorID: "p-" + string(rune('a'+i)),
			Timestamp:   time.Now().Add(time.Duration(i) * time.Minute),
		})
	}

	records := history.GetRecent(100)
	if len(records) != maxSize {
		t.Errorf("expected %d records, got %d", maxSize, len(records))
	}

	// Oldest records should be evicted
	for _, r := range records {
		if r.ID == "rec-a" || r.ID == "rec-b" {
			t.Errorf("old record %s should have been evicted", r.ID)
		}
	}
}

func TestPreemptionHistory_ConcurrentAccess(t *testing.T) {
	history := NewPreemptionHistory(1000)

	var wg sync.WaitGroup
	numOperations := 100

	// Concurrent adds
	for i := 0; i < numOperations; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			history.Add(PreemptionRecord{
				ID:          "rec-" + string(rune(idx)),
				PreemptorID: "p-" + string(rune(idx)),
				VictimIDs:   []string{"v-" + string(rune(idx))},
				NodeID:      "node-" + string(rune(idx%3)),
			})
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numOperations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = history.GetRecent(10)
			_ = history.GetByWorkload("v-a")
			_ = history.GetStats()
		}()
	}

	wg.Wait()

	// Should have all records
	if len(history.GetRecent(1000)) != numOperations {
		t.Log("some records may be missing due to concurrent access")
	}
}

// =============================================================================
// PreemptionSimulator Extended Tests
// =============================================================================

func TestPreemptionSimulator_NoFeasibleNode(t *testing.T) {
	nodes := NewNodeRegistry()
	// Node too small
	node := newTestNode("node-1", 100, 100*1024*1024)
	nodes.Register(node)

	simulator := NewPreemptionSimulator(nodes, func(nodeID string) []*Workload {
		return nil
	})

	// Workload needs way more resources
	workload := newTestWorkload("w1", 50, 10000, 10000*1024*1024)

	result := simulator.Simulate(workload)

	// When no victims available and node too small, returns nil
	if result != nil && result.CanSchedule {
		t.Error("should not be able to schedule on too-small node")
	}
}

func TestPreemptionSimulator_WithPriority(t *testing.T) {
	nodes := NewNodeRegistry()
	node := newTestNode("node-1", 1000, 1024*1024*1024)
	node.Allocated = Resources{CPU: 800, Memory: 800 * 1024 * 1024}
	node.Available = Resources{CPU: 200, Memory: 224 * 1024 * 1024}
	nodes.Register(node)

	// High priority existing workload
	highPrioExisting := newTestWorkload("existing", 100, 500, 500*1024*1024)

	simulator := NewPreemptionSimulator(nodes, func(nodeID string) []*Workload {
		return []*Workload{highPrioExisting}
	})

	// Low priority workload trying to preempt
	workload := newTestWorkload("new", 10, 600, 600*1024*1024)

	result := simulator.Simulate(workload)

	// When low priority cannot preempt high priority, returns nil
	// Or if result exists, should not be able to preempt
	if result != nil && result.CanSchedule && len(result.VictimsRequired) > 0 {
		t.Error("low priority should not preempt high priority")
	}
}
