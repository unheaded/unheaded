// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package scheduler

import (
	"sync"
	"testing"
	"time"
)

// =============================================================================
// PriorityQueue Tests
// =============================================================================

func TestPriorityQueue_NewPriorityQueue(t *testing.T) {
	pq := NewPriorityQueue(100)
	if pq == nil {
		t.Fatal("expected non-nil priority queue")
	}
	if pq.Len() != 0 {
		t.Errorf("expected empty queue, got length %d", pq.Len())
	}
}

func TestPriorityQueue_Push(t *testing.T) {
	pq := NewPriorityQueue(100)

	w1 := createTestWorkload("w1", 100, 100*1024*1024)
	w1.Priority = 50

	if !pq.Push(w1) {
		t.Error("push should succeed")
	}

	if pq.Len() != 1 {
		t.Errorf("expected length 1, got %d", pq.Len())
	}
}

func TestPriorityQueue_Push_Duplicate(t *testing.T) {
	pq := NewPriorityQueue(100)

	w1 := createTestWorkload("w1", 100, 100*1024*1024)

	pq.Push(w1)
	if pq.Push(w1) {
		t.Error("push of duplicate should fail")
	}

	if pq.Len() != 1 {
		t.Errorf("expected length 1, got %d", pq.Len())
	}
}

func TestPriorityQueue_Push_MaxSize(t *testing.T) {
	pq := NewPriorityQueue(2)

	w1 := createTestWorkload("w1", 100, 100*1024*1024)
	w2 := createTestWorkload("w2", 100, 100*1024*1024)
	w3 := createTestWorkload("w3", 100, 100*1024*1024)

	pq.Push(w1)
	pq.Push(w2)

	if pq.Push(w3) {
		t.Error("push should fail when queue is full")
	}

	if pq.Len() != 2 {
		t.Errorf("expected length 2, got %d", pq.Len())
	}
}

func TestPriorityQueue_Push_UnlimitedSize(t *testing.T) {
	pq := NewPriorityQueue(0) // 0 means unlimited

	for i := 0; i < 100; i++ {
		w := createTestWorkload("w"+string(rune(i)), 100, 100*1024*1024)
		if !pq.Push(w) {
			t.Errorf("push %d should succeed with unlimited size", i)
		}
	}
}

func TestPriorityQueue_Pop(t *testing.T) {
	pq := NewPriorityQueue(100)

	w1 := createTestWorkload("w1", 100, 100*1024*1024)
	w1.Priority = 50

	pq.Push(w1)
	popped := pq.Pop()

	if popped == nil {
		t.Fatal("pop should return workload")
	}
	if popped.ID != "w1" {
		t.Errorf("expected w1, got %s", popped.ID)
	}
	if pq.Len() != 0 {
		t.Errorf("expected empty queue after pop")
	}
}

func TestPriorityQueue_Pop_Empty(t *testing.T) {
	pq := NewPriorityQueue(100)

	popped := pq.Pop()
	if popped != nil {
		t.Error("pop on empty queue should return nil")
	}
}

func TestPriorityQueue_Pop_PriorityOrder(t *testing.T) {
	pq := NewPriorityQueue(100)

	w1 := createTestWorkload("w1", 100, 100*1024*1024)
	w1.Priority = 10

	w2 := createTestWorkload("w2", 100, 100*1024*1024)
	w2.Priority = 50

	w3 := createTestWorkload("w3", 100, 100*1024*1024)
	w3.Priority = 30

	// Push in random order
	pq.Push(w1)
	pq.Push(w2)
	pq.Push(w3)

	// Should pop in priority order (highest first)
	p1 := pq.Pop()
	if p1.ID != "w2" {
		t.Errorf("expected w2 (priority 50), got %s (priority %d)", p1.ID, p1.Priority)
	}

	p2 := pq.Pop()
	if p2.ID != "w3" {
		t.Errorf("expected w3 (priority 30), got %s (priority %d)", p2.ID, p2.Priority)
	}

	p3 := pq.Pop()
	if p3.ID != "w1" {
		t.Errorf("expected w1 (priority 10), got %s (priority %d)", p3.ID, p3.Priority)
	}
}

func TestPriorityQueue_Pop_FIFOForSamePriority(t *testing.T) {
	pq := NewPriorityQueue(100)

	// All same priority
	w1 := createTestWorkload("w1", 100, 100*1024*1024)
	w1.Priority = 50

	w2 := createTestWorkload("w2", 100, 100*1024*1024)
	w2.Priority = 50

	w3 := createTestWorkload("w3", 100, 100*1024*1024)
	w3.Priority = 50

	// Push with small delay to ensure different enqueue times
	pq.Push(w1)
	time.Sleep(1 * time.Millisecond)
	pq.Push(w2)
	time.Sleep(1 * time.Millisecond)
	pq.Push(w3)

	// Should pop in FIFO order for same priority
	p1 := pq.Pop()
	if p1.ID != "w1" {
		t.Errorf("expected w1 (first pushed), got %s", p1.ID)
	}

	p2 := pq.Pop()
	if p2.ID != "w2" {
		t.Errorf("expected w2 (second pushed), got %s", p2.ID)
	}

	p3 := pq.Pop()
	if p3.ID != "w3" {
		t.Errorf("expected w3 (third pushed), got %s", p3.ID)
	}
}

func TestPriorityQueue_Peek(t *testing.T) {
	pq := NewPriorityQueue(100)

	w1 := createTestWorkload("w1", 100, 100*1024*1024)
	w1.Priority = 50

	pq.Push(w1)

	peeked := pq.Peek()
	if peeked == nil {
		t.Fatal("peek should return workload")
	}
	if peeked.ID != "w1" {
		t.Errorf("expected w1, got %s", peeked.ID)
	}

	// Length should not change
	if pq.Len() != 1 {
		t.Errorf("peek should not change length")
	}
}

func TestPriorityQueue_Peek_Empty(t *testing.T) {
	pq := NewPriorityQueue(100)

	peeked := pq.Peek()
	if peeked != nil {
		t.Error("peek on empty queue should return nil")
	}
}

func TestPriorityQueue_Remove(t *testing.T) {
	pq := NewPriorityQueue(100)

	w1 := createTestWorkload("w1", 100, 100*1024*1024)
	w2 := createTestWorkload("w2", 100, 100*1024*1024)

	pq.Push(w1)
	pq.Push(w2)

	if !pq.Remove("w1") {
		t.Error("remove should succeed")
	}

	if pq.Len() != 1 {
		t.Errorf("expected length 1, got %d", pq.Len())
	}

	if pq.Contains("w1") {
		t.Error("w1 should no longer be in queue")
	}
}

func TestPriorityQueue_Remove_NotFound(t *testing.T) {
	pq := NewPriorityQueue(100)

	if pq.Remove("nonexistent") {
		t.Error("remove of nonexistent should return false")
	}
}

func TestPriorityQueue_Update(t *testing.T) {
	pq := NewPriorityQueue(100)

	w1 := createTestWorkload("w1", 100, 100*1024*1024)
	w1.Priority = 10

	w2 := createTestWorkload("w2", 100, 100*1024*1024)
	w2.Priority = 50

	pq.Push(w1)
	pq.Push(w2)

	// Update w1's priority to be higher
	w1Updated := createTestWorkload("w1", 100, 100*1024*1024)
	w1Updated.Priority = 100

	if !pq.Update(w1Updated) {
		t.Error("update should succeed")
	}

	// Now w1 should be popped first
	popped := pq.Pop()
	if popped.ID != "w1" {
		t.Errorf("expected w1 (updated priority 100), got %s", popped.ID)
	}
}

func TestPriorityQueue_Update_NotFound(t *testing.T) {
	pq := NewPriorityQueue(100)

	w := createTestWorkload("nonexistent", 100, 100*1024*1024)
	if pq.Update(w) {
		t.Error("update of nonexistent should return false")
	}
}

func TestPriorityQueue_Contains(t *testing.T) {
	pq := NewPriorityQueue(100)

	w1 := createTestWorkload("w1", 100, 100*1024*1024)
	pq.Push(w1)

	if !pq.Contains("w1") {
		t.Error("should contain w1")
	}

	if pq.Contains("w2") {
		t.Error("should not contain w2")
	}
}

func TestPriorityQueue_Clear(t *testing.T) {
	pq := NewPriorityQueue(100)

	for i := 0; i < 10; i++ {
		w := createTestWorkload("w"+string(rune(i)), 100, 100*1024*1024)
		pq.Push(w)
	}

	pq.Clear()

	if pq.Len() != 0 {
		t.Errorf("expected empty queue after clear, got %d", pq.Len())
	}
}

func TestPriorityQueue_List(t *testing.T) {
	pq := NewPriorityQueue(100)

	w1 := createTestWorkload("w1", 100, 100*1024*1024)
	w2 := createTestWorkload("w2", 100, 100*1024*1024)
	w3 := createTestWorkload("w3", 100, 100*1024*1024)

	pq.Push(w1)
	pq.Push(w2)
	pq.Push(w3)

	list := pq.List()
	if len(list) != 3 {
		t.Errorf("expected 3 workloads, got %d", len(list))
	}
}

func TestPriorityQueue_GetByPriority(t *testing.T) {
	pq := NewPriorityQueue(100)

	w1 := createTestWorkload("w1", 100, 100*1024*1024)
	w1.Priority = 10

	w2 := createTestWorkload("w2", 100, 100*1024*1024)
	w2.Priority = 50

	w3 := createTestWorkload("w3", 100, 100*1024*1024)
	w3.Priority = 10

	pq.Push(w1)
	pq.Push(w2)
	pq.Push(w3)

	byPriority := pq.GetByPriority()

	if len(byPriority[10]) != 2 {
		t.Errorf("expected 2 workloads with priority 10, got %d", len(byPriority[10]))
	}

	if len(byPriority[50]) != 1 {
		t.Errorf("expected 1 workload with priority 50, got %d", len(byPriority[50]))
	}
}

func TestPriorityQueue_GetByNamespace(t *testing.T) {
	pq := NewPriorityQueue(100)

	w1 := createTestWorkload("w1", 100, 100*1024*1024)
	w1.Namespace = "ns1"

	w2 := createTestWorkload("w2", 100, 100*1024*1024)
	w2.Namespace = "ns2"

	w3 := createTestWorkload("w3", 100, 100*1024*1024)
	w3.Namespace = "ns1"

	pq.Push(w1)
	pq.Push(w2)
	pq.Push(w3)

	byNamespace := pq.GetByNamespace()

	if len(byNamespace["ns1"]) != 2 {
		t.Errorf("expected 2 workloads in ns1, got %d", len(byNamespace["ns1"]))
	}

	if len(byNamespace["ns2"]) != 1 {
		t.Errorf("expected 1 workload in ns2, got %d", len(byNamespace["ns2"]))
	}
}

func TestPriorityQueue_Signal(t *testing.T) {
	pq := NewPriorityQueue(100)

	// Should not panic
	pq.Signal()
}

func TestPriorityQueue_Broadcast(t *testing.T) {
	pq := NewPriorityQueue(100)

	// Should not panic
	pq.Broadcast()
}

func TestPriorityQueue_ConcurrentAccess(t *testing.T) {
	pq := NewPriorityQueue(0) // unlimited

	var wg sync.WaitGroup

	// Concurrent pushes
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w := createTestWorkload("w-push-"+string(rune(i)), 100, 100*1024*1024)
			w.Priority = int32(i % 100)
			pq.Push(w)
		}(i)
	}

	wg.Wait()

	// Concurrent pops
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pq.Pop()
		}()
	}

	wg.Wait()

	// Concurrent mixed operations
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			switch i % 4 {
			case 0:
				w := createTestWorkload("w-mixed-"+string(rune(i)), 100, 100*1024*1024)
				pq.Push(w)
			case 1:
				pq.Pop()
			case 2:
				pq.Peek()
			case 3:
				pq.Len()
			}
		}(i)
	}

	wg.Wait()
}

// =============================================================================
// BackoffQueue Tests
// =============================================================================

func TestBackoffQueue_NewBackoffQueue(t *testing.T) {
	bq := NewBackoffQueue()
	if bq == nil {
		t.Fatal("expected non-nil backoff queue")
	}
}

func TestBackoffQueue_Add(t *testing.T) {
	bq := NewBackoffQueue()

	w := createTestWorkload("w1", 100, 100*1024*1024)
	bq.Add(w, 100*time.Millisecond)

	if bq.Len() != 1 {
		t.Errorf("expected length 1, got %d", bq.Len())
	}
}

func TestBackoffQueue_PopReady_NotReady(t *testing.T) {
	bq := NewBackoffQueue()

	w := createTestWorkload("w1", 100, 100*1024*1024)
	bq.Add(w, 1*time.Hour) // Long backoff

	ready := bq.PopReady()
	if ready != nil {
		t.Error("expected nil when not ready")
	}
}

func TestBackoffQueue_PopReady_Ready(t *testing.T) {
	bq := NewBackoffQueue()

	w := createTestWorkload("w1", 100, 100*1024*1024)
	bq.Add(w, 1*time.Millisecond)

	// Wait for backoff
	time.Sleep(10 * time.Millisecond)

	ready := bq.PopReady()
	if ready == nil {
		t.Error("expected workload to be ready")
	}
	if ready.ID != "w1" {
		t.Errorf("expected w1, got %s", ready.ID)
	}

	if bq.Len() != 0 {
		t.Errorf("expected empty queue after pop")
	}
}

func TestBackoffQueue_PopReady_MultipleItems(t *testing.T) {
	bq := NewBackoffQueue()

	w1 := createTestWorkload("w1", 100, 100*1024*1024)
	w2 := createTestWorkload("w2", 100, 100*1024*1024)

	bq.Add(w1, 1*time.Millisecond)
	bq.Add(w2, 1*time.Hour)

	time.Sleep(10 * time.Millisecond)

	// Only w1 should be ready
	ready := bq.PopReady()
	if ready == nil || ready.ID != "w1" {
		t.Error("expected w1 to be ready")
	}

	// w2 should still be waiting
	if bq.Len() != 1 {
		t.Errorf("expected 1 item remaining, got %d", bq.Len())
	}

	ready2 := bq.PopReady()
	if ready2 != nil {
		t.Error("w2 should not be ready yet")
	}
}

func TestBackoffQueue_ConcurrentAccess(t *testing.T) {
	bq := NewBackoffQueue()

	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			w := createTestWorkload("w-"+string(rune(i)), 100, 100*1024*1024)
			bq.Add(w, time.Duration(i)*time.Millisecond)

			bq.PopReady()
			bq.Len()
		}(i)
	}

	wg.Wait()
}

// =============================================================================
// ActiveQueue Tests
// =============================================================================

func TestActiveQueue_NewActiveQueue(t *testing.T) {
	aq := NewActiveQueue(100)
	if aq == nil {
		t.Fatal("expected non-nil active queue")
	}
}

func TestActiveQueue_Add(t *testing.T) {
	aq := NewActiveQueue(100)

	w := createTestWorkload("w1", 100, 100*1024*1024)
	if !aq.Add(w) {
		t.Error("add should succeed")
	}
}

func TestActiveQueue_AddBackoff(t *testing.T) {
	aq := NewActiveQueue(100)

	w := createTestWorkload("w1", 100, 100*1024*1024)
	aq.AddBackoff(w, 100*time.Millisecond)

	// Should not be in active queue
	popped := aq.Pop()
	if popped != nil {
		t.Error("expected nil when backoff not expired")
	}
}

func TestActiveQueue_AddUnschedulable(t *testing.T) {
	aq := NewActiveQueue(100)

	w := createTestWorkload("w1", 100, 100*1024*1024)
	aq.AddUnschedulable(w)

	// Should not be in active queue
	popped := aq.Pop()
	if popped != nil {
		t.Error("expected nil for unschedulable workload")
	}
}

func TestActiveQueue_Pop(t *testing.T) {
	aq := NewActiveQueue(100)

	w := createTestWorkload("w1", 100, 100*1024*1024)
	aq.Add(w)

	popped := aq.Pop()
	if popped == nil {
		t.Error("expected workload")
	}
	if popped.ID != "w1" {
		t.Errorf("expected w1, got %s", popped.ID)
	}
}

func TestActiveQueue_Pop_PrefersBackoffReady(t *testing.T) {
	aq := NewActiveQueue(100)

	w1 := createTestWorkload("w1", 100, 100*1024*1024)
	w1.Priority = 10

	w2 := createTestWorkload("w2", 100, 100*1024*1024)
	w2.Priority = 50

	// Add w1 to backoff with short duration
	aq.AddBackoff(w1, 1*time.Millisecond)

	// Add w2 to active queue
	aq.Add(w2)

	// Wait for backoff
	time.Sleep(10 * time.Millisecond)

	// Should prefer backoff ready item
	popped := aq.Pop()
	if popped == nil {
		t.Fatal("expected workload")
	}
	if popped.ID != "w1" {
		t.Errorf("expected w1 (from backoff), got %s", popped.ID)
	}
}

func TestActiveQueue_MoveToActive(t *testing.T) {
	aq := NewActiveQueue(100)

	w := createTestWorkload("w1", 100, 100*1024*1024)
	aq.AddUnschedulable(w)

	// Move back to active
	if !aq.MoveToActive("w1") {
		t.Error("move should succeed")
	}

	// Now should be poppable
	popped := aq.Pop()
	if popped == nil || popped.ID != "w1" {
		t.Error("expected w1 to be in active queue")
	}
}

func TestActiveQueue_MoveToActive_NotFound(t *testing.T) {
	aq := NewActiveQueue(100)

	if aq.MoveToActive("nonexistent") {
		t.Error("move of nonexistent should return false")
	}
}

func TestActiveQueue_FlushBackoff(t *testing.T) {
	aq := NewActiveQueue(100)

	w1 := createTestWorkload("w1", 100, 100*1024*1024)
	w2 := createTestWorkload("w2", 100, 100*1024*1024)

	aq.AddBackoff(w1, 1*time.Millisecond)
	aq.AddBackoff(w2, 1*time.Millisecond)

	time.Sleep(10 * time.Millisecond)

	count := aq.FlushBackoff()
	if count != 2 {
		t.Errorf("expected 2 flushed, got %d", count)
	}

	// Both should now be in active queue
	p1 := aq.Pop()
	p2 := aq.Pop()

	if p1 == nil || p2 == nil {
		t.Error("expected both workloads in active queue")
	}
}

// =============================================================================
// SchedulingCycle Tests
// =============================================================================

func TestSchedulingCycle_NewSchedulingCycle(t *testing.T) {
	sc := NewSchedulingCycle(1)
	if sc == nil {
		t.Fatal("expected non-nil scheduling cycle")
	}
	if sc.ID != 1 {
		t.Errorf("expected ID 1, got %d", sc.ID)
	}
}

func TestSchedulingCycle_Complete(t *testing.T) {
	sc := NewSchedulingCycle(1)

	// Should have zero end time initially
	if !sc.EndTime.IsZero() {
		t.Error("expected zero end time initially")
	}

	sc.Complete()

	if sc.EndTime.IsZero() {
		t.Error("expected non-zero end time after complete")
	}
}

func TestSchedulingCycle_Duration(t *testing.T) {
	sc := NewSchedulingCycle(1)

	// Duration before complete
	d1 := sc.Duration()
	if d1 == 0 {
		t.Error("expected non-zero duration")
	}

	time.Sleep(10 * time.Millisecond)

	// Duration should increase
	d2 := sc.Duration()
	if d2 <= d1 {
		t.Error("duration should increase over time")
	}

	sc.Complete()

	// Duration after complete should be fixed
	d3 := sc.Duration()
	time.Sleep(10 * time.Millisecond)
	d4 := sc.Duration()

	if d3 != d4 {
		t.Error("duration should be fixed after complete")
	}
}

func TestSchedulingCycle_RecordScheduled(t *testing.T) {
	sc := NewSchedulingCycle(1)

	sc.RecordScheduled()
	sc.RecordScheduled()

	if sc.Scheduled != 2 {
		t.Errorf("expected 2 scheduled, got %d", sc.Scheduled)
	}
}

func TestSchedulingCycle_RecordFailed(t *testing.T) {
	sc := NewSchedulingCycle(1)

	sc.RecordFailed()
	sc.RecordFailed()
	sc.RecordFailed()

	if sc.Failed != 3 {
		t.Errorf("expected 3 failed, got %d", sc.Failed)
	}
}

func TestSchedulingCycle_RecordPreempted(t *testing.T) {
	sc := NewSchedulingCycle(1)

	sc.RecordPreempted()

	if sc.Preempted != 1 {
		t.Errorf("expected 1 preempted, got %d", sc.Preempted)
	}
}

func TestSchedulingCycle_SetNodeScore(t *testing.T) {
	sc := NewSchedulingCycle(1)

	sc.SetNodeScore("node-1", 100)
	sc.SetNodeScore("node-2", 200)

	if sc.NodeScores["node-1"] != 100 {
		t.Errorf("expected node-1 score 100, got %d", sc.NodeScores["node-1"])
	}
	if sc.NodeScores["node-2"] != 200 {
		t.Errorf("expected node-2 score 200, got %d", sc.NodeScores["node-2"])
	}
}

func TestSchedulingCycle_ConcurrentAccess(t *testing.T) {
	sc := NewSchedulingCycle(1)

	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			switch i % 4 {
			case 0:
				sc.RecordScheduled()
			case 1:
				sc.RecordFailed()
			case 2:
				sc.RecordPreempted()
			case 3:
				sc.SetNodeScore("node-"+string(rune(i)), int64(i))
			}

			sc.Duration()
		}(i)
	}

	wg.Wait()
	sc.Complete()
}

// =============================================================================
// NominatedWorkloads Tests
// =============================================================================

func TestNominatedWorkloads_NewNominatedWorkloads(t *testing.T) {
	nw := NewNominatedWorkloads()
	if nw == nil {
		t.Fatal("expected non-nil nominated workloads")
	}
}

func TestNominatedWorkloads_Add(t *testing.T) {
	nw := NewNominatedWorkloads()

	nw.Add("node-1", "w1")
	nw.Add("node-1", "w2")

	workloads := nw.GetForNode("node-1")
	if len(workloads) != 2 {
		t.Errorf("expected 2 workloads, got %d", len(workloads))
	}
}

func TestNominatedWorkloads_Remove(t *testing.T) {
	nw := NewNominatedWorkloads()

	nw.Add("node-1", "w1")
	nw.Add("node-1", "w2")

	nw.Remove("node-1", "w1")

	workloads := nw.GetForNode("node-1")
	if len(workloads) != 1 {
		t.Errorf("expected 1 workload, got %d", len(workloads))
	}
	if workloads[0] != "w2" {
		t.Errorf("expected w2, got %s", workloads[0])
	}
}

func TestNominatedWorkloads_Remove_NotFound(t *testing.T) {
	nw := NewNominatedWorkloads()

	nw.Add("node-1", "w1")

	// Remove nonexistent - should not panic
	nw.Remove("node-1", "nonexistent")
	nw.Remove("nonexistent", "w1")

	workloads := nw.GetForNode("node-1")
	if len(workloads) != 1 {
		t.Errorf("expected 1 workload, got %d", len(workloads))
	}
}

func TestNominatedWorkloads_GetForNode(t *testing.T) {
	nw := NewNominatedWorkloads()

	nw.Add("node-1", "w1")
	nw.Add("node-2", "w2")

	workloads1 := nw.GetForNode("node-1")
	workloads2 := nw.GetForNode("node-2")
	workloads3 := nw.GetForNode("nonexistent")

	if len(workloads1) != 1 {
		t.Errorf("expected 1 workload for node-1, got %d", len(workloads1))
	}
	if len(workloads2) != 1 {
		t.Errorf("expected 1 workload for node-2, got %d", len(workloads2))
	}
	if len(workloads3) != 0 {
		t.Errorf("expected 0 workloads for nonexistent, got %d", len(workloads3))
	}
}

func TestNominatedWorkloads_ClearNode(t *testing.T) {
	nw := NewNominatedWorkloads()

	nw.Add("node-1", "w1")
	nw.Add("node-1", "w2")
	nw.Add("node-2", "w3")

	nw.ClearNode("node-1")

	workloads1 := nw.GetForNode("node-1")
	workloads2 := nw.GetForNode("node-2")

	if len(workloads1) != 0 {
		t.Errorf("expected 0 workloads after clear, got %d", len(workloads1))
	}
	if len(workloads2) != 1 {
		t.Errorf("node-2 should be unaffected, got %d", len(workloads2))
	}
}

func TestNominatedWorkloads_ConcurrentAccess(t *testing.T) {
	nw := NewNominatedWorkloads()

	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			nodeID := "node-" + string(rune(i%5))
			workloadID := "w-" + string(rune(i))

			switch i % 3 {
			case 0:
				nw.Add(nodeID, workloadID)
			case 1:
				nw.Remove(nodeID, workloadID)
			case 2:
				nw.GetForNode(nodeID)
			}
		}(i)
	}

	wg.Wait()
}

// =============================================================================
// Heap Operations Tests
// =============================================================================

func TestWorkloadHeap_Operations(t *testing.T) {
	// Test the heap operations directly
	pq := NewPriorityQueue(100)

	// Push many items
	for i := 0; i < 100; i++ {
		w := createTestWorkload("w-"+string(rune(i)), 100, 100*1024*1024)
		w.Priority = int32(100 - i)
		pq.Push(w)
	}

	// Pop should return in priority order
	lastPriority := int32(1000)
	for pq.Len() > 0 {
		w := pq.Pop()
		if w.Priority > lastPriority {
			t.Errorf("priority order violated: %d > %d", w.Priority, lastPriority)
		}
		lastPriority = w.Priority
	}
}

func TestWorkloadHeap_RemoveMiddle(t *testing.T) {
	pq := NewPriorityQueue(100)

	// Push items
	for i := 0; i < 10; i++ {
		w := createTestWorkload("w"+string(rune('0'+i)), 100, 100*1024*1024)
		w.Priority = int32(i * 10)
		pq.Push(w)
	}

	// Remove middle item
	pq.Remove("w5")

	// Pop should still work correctly
	count := 0
	for pq.Len() > 0 {
		w := pq.Pop()
		if w.ID == "w5" {
			t.Error("w5 should have been removed")
		}
		count++
	}

	if count != 9 {
		t.Errorf("expected 9 items, got %d", count)
	}
}

// =============================================================================
// PopWait Tests (require goroutine coordination)
// =============================================================================

func TestPriorityQueue_PopWait(t *testing.T) {
	pq := NewPriorityQueue(100)

	var received *Workload
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		received = pq.PopWait()
	}()

	// Give the goroutine time to start waiting
	time.Sleep(50 * time.Millisecond)

	// Push a workload
	w := createTestWorkload("w1", 100, 100*1024*1024)
	pq.Push(w)

	wg.Wait()

	if received == nil {
		t.Error("expected to receive workload")
	}
	if received.ID != "w1" {
		t.Errorf("expected w1, got %s", received.ID)
	}
}

func TestPriorityQueue_PopWait_ImmediateReturn(t *testing.T) {
	pq := NewPriorityQueue(100)

	// Push first
	w := createTestWorkload("w1", 100, 100*1024*1024)
	pq.Push(w)

	// PopWait should return immediately
	done := make(chan *Workload)
	go func() {
		done <- pq.PopWait()
	}()

	select {
	case received := <-done:
		if received == nil || received.ID != "w1" {
			t.Error("expected w1")
		}
	case <-time.After(1 * time.Second):
		t.Error("PopWait should return immediately when queue is not empty")
	}
}
