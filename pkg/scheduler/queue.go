// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package scheduler

import (
	"container/heap"
	"sync"
	"time"
)

// PriorityQueue manages pending workloads in priority order.
type PriorityQueue struct {
	mu       sync.Mutex
	items    workloadHeap
	lookup   map[string]*queueItem
	maxSize  int
	cond     *sync.Cond
}

// queueItem wraps a workload with queue metadata.
type queueItem struct {
	workload    *Workload
	index       int
	enqueueTime time.Time
	retryCount  int
}

// workloadHeap implements heap.Interface for priority-based scheduling.
type workloadHeap []*queueItem

func (h workloadHeap) Len() int { return len(h) }

func (h workloadHeap) Less(i, j int) bool {
	// Higher priority first
	if h[i].workload.Priority != h[j].workload.Priority {
		return h[i].workload.Priority > h[j].workload.Priority
	}
	// Then by enqueue time (FIFO for same priority)
	return h[i].enqueueTime.Before(h[j].enqueueTime)
}

func (h workloadHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *workloadHeap) Push(x interface{}) {
	item := x.(*queueItem)
	item.index = len(*h)
	*h = append(*h, item)
}

func (h *workloadHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*h = old[0 : n-1]
	return item
}

// NewPriorityQueue creates a new priority queue.
func NewPriorityQueue(maxSize int) *PriorityQueue {
	pq := &PriorityQueue{
		items:   make(workloadHeap, 0),
		lookup:  make(map[string]*queueItem),
		maxSize: maxSize,
	}
	pq.cond = sync.NewCond(&pq.mu)
	heap.Init(&pq.items)
	return pq
}

// Push adds a workload to the queue.
func (pq *PriorityQueue) Push(workload *Workload) bool {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	// Check if already in queue
	if _, exists := pq.lookup[workload.ID]; exists {
		return false
	}

	// Check capacity
	if pq.maxSize > 0 && len(pq.items) >= pq.maxSize {
		return false
	}

	item := &queueItem{
		workload:    workload,
		enqueueTime: time.Now(),
	}

	heap.Push(&pq.items, item)
	pq.lookup[workload.ID] = item

	pq.cond.Signal()
	return true
}

// Pop removes and returns the highest priority workload.
func (pq *PriorityQueue) Pop() *Workload {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if len(pq.items) == 0 {
		return nil
	}

	item := heap.Pop(&pq.items).(*queueItem)
	delete(pq.lookup, item.workload.ID)

	return item.workload
}

// PopWait removes and returns the highest priority workload, blocking if empty.
func (pq *PriorityQueue) PopWait() *Workload {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	for len(pq.items) == 0 {
		pq.cond.Wait()
	}

	item := heap.Pop(&pq.items).(*queueItem)
	delete(pq.lookup, item.workload.ID)

	return item.workload
}

// Peek returns the highest priority workload without removing it.
func (pq *PriorityQueue) Peek() *Workload {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if len(pq.items) == 0 {
		return nil
	}

	return pq.items[0].workload
}

// Remove removes a specific workload from the queue.
func (pq *PriorityQueue) Remove(workloadID string) bool {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	item, exists := pq.lookup[workloadID]
	if !exists {
		return false
	}

	heap.Remove(&pq.items, item.index)
	delete(pq.lookup, workloadID)

	return true
}

// Update updates a workload's priority in the queue.
func (pq *PriorityQueue) Update(workload *Workload) bool {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	item, exists := pq.lookup[workload.ID]
	if !exists {
		return false
	}

	item.workload = workload
	heap.Fix(&pq.items, item.index)

	return true
}

// Len returns the number of items in the queue.
func (pq *PriorityQueue) Len() int {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	return len(pq.items)
}

// Contains checks if a workload is in the queue.
func (pq *PriorityQueue) Contains(workloadID string) bool {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	_, exists := pq.lookup[workloadID]
	return exists
}

// Clear removes all items from the queue.
func (pq *PriorityQueue) Clear() {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	pq.items = make(workloadHeap, 0)
	pq.lookup = make(map[string]*queueItem)
	heap.Init(&pq.items)
}

// List returns all workloads in the queue.
func (pq *PriorityQueue) List() []*Workload {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	workloads := make([]*Workload, len(pq.items))
	for i, item := range pq.items {
		workloads[i] = item.workload
	}
	return workloads
}

// GetByPriority returns workloads grouped by priority.
func (pq *PriorityQueue) GetByPriority() map[int32][]*Workload {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	result := make(map[int32][]*Workload)
	for _, item := range pq.items {
		priority := item.workload.Priority
		result[priority] = append(result[priority], item.workload)
	}
	return result
}

// GetByNamespace returns workloads grouped by namespace.
func (pq *PriorityQueue) GetByNamespace() map[string][]*Workload {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	result := make(map[string][]*Workload)
	for _, item := range pq.items {
		ns := item.workload.Namespace
		result[ns] = append(result[ns], item.workload)
	}
	return result
}

// Signal wakes up one waiting goroutine.
func (pq *PriorityQueue) Signal() {
	pq.cond.Signal()
}

// Broadcast wakes up all waiting goroutines.
func (pq *PriorityQueue) Broadcast() {
	pq.cond.Broadcast()
}

// ActiveQueue manages the scheduling pipeline with multiple stages.
type ActiveQueue struct {
	mu              sync.Mutex
	activeQ         *PriorityQueue      // Ready for scheduling
	backoffQ        *BackoffQueue       // Waiting for retry
	unschedulableQ  map[string]*Workload // Unschedulable workloads
}

// NewActiveQueue creates a new active queue.
func NewActiveQueue(maxSize int) *ActiveQueue {
	return &ActiveQueue{
		activeQ:        NewPriorityQueue(maxSize),
		backoffQ:       NewBackoffQueue(),
		unschedulableQ: make(map[string]*Workload),
	}
}

// Add adds a workload to the active queue.
func (aq *ActiveQueue) Add(workload *Workload) bool {
	return aq.activeQ.Push(workload)
}

// AddBackoff adds a workload to the backoff queue.
func (aq *ActiveQueue) AddBackoff(workload *Workload, backoffDuration time.Duration) {
	aq.backoffQ.Add(workload, backoffDuration)
}

// AddUnschedulable marks a workload as unschedulable.
func (aq *ActiveQueue) AddUnschedulable(workload *Workload) {
	aq.mu.Lock()
	defer aq.mu.Unlock()
	aq.unschedulableQ[workload.ID] = workload
}

// Pop returns the next workload to schedule.
func (aq *ActiveQueue) Pop() *Workload {
	// First check backoff queue for ready items
	if ready := aq.backoffQ.PopReady(); ready != nil {
		return ready
	}
	return aq.activeQ.Pop()
}

// MoveToActive moves a workload from unschedulable to active.
func (aq *ActiveQueue) MoveToActive(workloadID string) bool {
	aq.mu.Lock()
	workload, exists := aq.unschedulableQ[workloadID]
	if !exists {
		aq.mu.Unlock()
		return false
	}
	delete(aq.unschedulableQ, workloadID)
	aq.mu.Unlock()

	return aq.activeQ.Push(workload)
}

// FlushBackoff moves all ready items from backoff to active queue.
func (aq *ActiveQueue) FlushBackoff() int {
	count := 0
	for {
		workload := aq.backoffQ.PopReady()
		if workload == nil {
			break
		}
		aq.activeQ.Push(workload)
		count++
	}
	return count
}

// BackoffQueue manages workloads waiting for retry.
type BackoffQueue struct {
	mu    sync.Mutex
	items map[string]*backoffItem
}

type backoffItem struct {
	workload  *Workload
	readyTime time.Time
}

// NewBackoffQueue creates a new backoff queue.
func NewBackoffQueue() *BackoffQueue {
	return &BackoffQueue{
		items: make(map[string]*backoffItem),
	}
}

// Add adds a workload with a backoff duration.
func (bq *BackoffQueue) Add(workload *Workload, backoff time.Duration) {
	bq.mu.Lock()
	defer bq.mu.Unlock()

	bq.items[workload.ID] = &backoffItem{
		workload:  workload,
		readyTime: time.Now().Add(backoff),
	}
}

// PopReady returns a workload if its backoff period has passed.
func (bq *BackoffQueue) PopReady() *Workload {
	bq.mu.Lock()
	defer bq.mu.Unlock()

	now := time.Now()
	for id, item := range bq.items {
		if item.readyTime.Before(now) || item.readyTime.Equal(now) {
			delete(bq.items, id)
			return item.workload
		}
	}
	return nil
}

// Len returns the number of items in backoff.
func (bq *BackoffQueue) Len() int {
	bq.mu.Lock()
	defer bq.mu.Unlock()
	return len(bq.items)
}

// SchedulingCycle represents a single scheduling cycle.
type SchedulingCycle struct {
	mu          sync.Mutex
	ID          int64
	StartTime   time.Time
	EndTime     time.Time
	Scheduled   int
	Failed      int
	Preempted   int
	NodeScores  map[string]int64
}

// NewSchedulingCycle creates a new scheduling cycle.
func NewSchedulingCycle(id int64) *SchedulingCycle {
	return &SchedulingCycle{
		ID:         id,
		StartTime:  time.Now(),
		NodeScores: make(map[string]int64),
	}
}

// Complete marks the cycle as complete.
func (sc *SchedulingCycle) Complete() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.EndTime = time.Now()
}

// Duration returns the cycle duration.
func (sc *SchedulingCycle) Duration() time.Duration {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if sc.EndTime.IsZero() {
		return time.Since(sc.StartTime)
	}
	return sc.EndTime.Sub(sc.StartTime)
}

// RecordScheduled increments the scheduled count.
func (sc *SchedulingCycle) RecordScheduled() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.Scheduled++
}

// RecordFailed increments the failed count.
func (sc *SchedulingCycle) RecordFailed() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.Failed++
}

// RecordPreempted increments the preempted count.
func (sc *SchedulingCycle) RecordPreempted() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.Preempted++
}

// SetNodeScore records a node's score.
func (sc *SchedulingCycle) SetNodeScore(nodeID string, score int64) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.NodeScores[nodeID] = score
}

// NominatedWorkloads tracks workloads nominated for specific nodes.
type NominatedWorkloads struct {
	mu         sync.RWMutex
	nominated  map[string][]string // nodeID -> workloadIDs
}

// NewNominatedWorkloads creates a new nominated workloads tracker.
func NewNominatedWorkloads() *NominatedWorkloads {
	return &NominatedWorkloads{
		nominated: make(map[string][]string),
	}
}

// Add nominates a workload for a node.
func (nw *NominatedWorkloads) Add(nodeID, workloadID string) {
	nw.mu.Lock()
	defer nw.mu.Unlock()

	nw.nominated[nodeID] = append(nw.nominated[nodeID], workloadID)
}

// Remove removes a nomination.
func (nw *NominatedWorkloads) Remove(nodeID, workloadID string) {
	nw.mu.Lock()
	defer nw.mu.Unlock()

	workloads := nw.nominated[nodeID]
	for i, id := range workloads {
		if id == workloadID {
			nw.nominated[nodeID] = append(workloads[:i], workloads[i+1:]...)
			break
		}
	}
}

// GetForNode returns all nominated workloads for a node.
func (nw *NominatedWorkloads) GetForNode(nodeID string) []string {
	nw.mu.RLock()
	defer nw.mu.RUnlock()

	result := make([]string, len(nw.nominated[nodeID]))
	copy(result, nw.nominated[nodeID])
	return result
}

// ClearNode removes all nominations for a node.
func (nw *NominatedWorkloads) ClearNode(nodeID string) {
	nw.mu.Lock()
	defer nw.mu.Unlock()
	delete(nw.nominated, nodeID)
}
