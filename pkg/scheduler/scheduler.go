// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package scheduler implements a container scheduler for Unheaded Kingdom.
// It provides node management, resource tracking, scheduling algorithms,
// affinity/anti-affinity rules, taints/tolerations, priority classes,
// queue management, binding, and preemption support.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Common errors returned by the scheduler.
var (
	ErrNoNodesAvailable    = errors.New("no nodes available for scheduling")
	ErrInsufficientResources = errors.New("insufficient resources on all nodes")
	ErrWorkloadNotFound    = errors.New("workload not found")
	ErrNodeNotFound        = errors.New("node not found")
	ErrAlreadyBound        = errors.New("workload already bound to a node")
	ErrSchedulerStopped    = errors.New("scheduler has been stopped")
	ErrQuotaExceeded       = errors.New("resource quota exceeded")
	ErrAffinityNotSatisfied = errors.New("affinity requirements not satisfied")
	ErrTaintNotTolerated   = errors.New("node taint not tolerated by workload")
)

// WorkloadState represents the current state of a workload.
type WorkloadState string

const (
	WorkloadStatePending   WorkloadState = "Pending"
	WorkloadStateScheduled WorkloadState = "Scheduled"
	WorkloadStateRunning   WorkloadState = "Running"
	WorkloadStateFailed    WorkloadState = "Failed"
	WorkloadStateEvicted   WorkloadState = "Evicted"
)

// Workload represents a container workload to be scheduled.
type Workload struct {
	ID          string
	Name        string
	Namespace   string
	Priority    int32
	Resources   ResourceRequirements
	Affinity    *AffinitySpec
	Tolerations []Toleration
	State       WorkloadState
	BoundNode   string
	CreatedAt   time.Time
	ScheduledAt time.Time
	Labels      map[string]string
	Annotations map[string]string
}

// ResourceRequirements specifies the resource requests and limits for a workload.
type ResourceRequirements struct {
	Requests Resources
	Limits   Resources
}

// Resources represents compute resources.
type Resources struct {
	CPU       int64 // millicores
	Memory    int64 // bytes
	Disk      int64 // bytes
	GPUs      int32
	Ephemeral int64 // ephemeral storage in bytes
}

// Add returns a new Resources with values added.
func (r Resources) Add(other Resources) Resources {
	return Resources{
		CPU:       r.CPU + other.CPU,
		Memory:    r.Memory + other.Memory,
		Disk:      r.Disk + other.Disk,
		GPUs:      r.GPUs + other.GPUs,
		Ephemeral: r.Ephemeral + other.Ephemeral,
	}
}

// Sub returns a new Resources with values subtracted.
func (r Resources) Sub(other Resources) Resources {
	return Resources{
		CPU:       r.CPU - other.CPU,
		Memory:    r.Memory - other.Memory,
		Disk:      r.Disk - other.Disk,
		GPUs:      r.GPUs - other.GPUs,
		Ephemeral: r.Ephemeral - other.Ephemeral,
	}
}

// FitsIn checks if resources fit within available resources.
func (r Resources) FitsIn(available Resources) bool {
	return r.CPU <= available.CPU &&
		r.Memory <= available.Memory &&
		r.Disk <= available.Disk &&
		r.GPUs <= available.GPUs &&
		r.Ephemeral <= available.Ephemeral
}

// IsZero checks if all resource values are zero.
func (r Resources) IsZero() bool {
	return r.CPU == 0 && r.Memory == 0 && r.Disk == 0 && r.GPUs == 0 && r.Ephemeral == 0
}

// SchedulerConfig holds configuration for the scheduler.
type SchedulerConfig struct {
	// SchedulingInterval is how often the scheduler runs.
	SchedulingInterval time.Duration
	// Algorithm specifies the scheduling algorithm to use.
	Algorithm AlgorithmType
	// EnablePreemption enables workload preemption.
	EnablePreemption bool
	// PreemptionTimeout is how long to wait before preemption.
	PreemptionTimeout time.Duration
	// MaxRetries is the maximum number of scheduling retries.
	MaxRetries int
	// RetryBackoff is the backoff duration between retries.
	RetryBackoff time.Duration
	// QueueSize is the maximum size of the scheduling queue.
	QueueSize int
	// WorkerCount is the number of parallel scheduling workers.
	WorkerCount int
}

// DefaultConfig returns a default scheduler configuration.
func DefaultConfig() SchedulerConfig {
	return SchedulerConfig{
		SchedulingInterval: 1 * time.Second,
		Algorithm:          AlgorithmBinPack,
		EnablePreemption:   true,
		PreemptionTimeout:  30 * time.Second,
		MaxRetries:         3,
		RetryBackoff:       5 * time.Second,
		QueueSize:          10000,
		WorkerCount:        4,
	}
}

// SchedulerMetrics holds metrics for the scheduler.
type SchedulerMetrics struct {
	mu                    sync.RWMutex
	SchedulingAttempts    int64
	SuccessfulSchedulings int64
	FailedSchedulings     int64
	PreemptionCount       int64
	AverageSchedulingTime time.Duration
	QueueDepth            int
	NodesCount            int
	TotalSchedulingTime   time.Duration
}

// RecordSchedulingAttempt records a scheduling attempt.
func (m *SchedulerMetrics) RecordSchedulingAttempt(success bool, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SchedulingAttempts++
	m.TotalSchedulingTime += duration
	if success {
		m.SuccessfulSchedulings++
	} else {
		m.FailedSchedulings++
	}
	if m.SchedulingAttempts > 0 {
		m.AverageSchedulingTime = m.TotalSchedulingTime / time.Duration(m.SchedulingAttempts)
	}
}

// RecordPreemption records a preemption event.
func (m *SchedulerMetrics) RecordPreemption() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PreemptionCount++
}

// GetMetrics returns a copy of the current metrics.
func (m *SchedulerMetrics) GetMetrics() SchedulerMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return SchedulerMetrics{
		SchedulingAttempts:    m.SchedulingAttempts,
		SuccessfulSchedulings: m.SuccessfulSchedulings,
		FailedSchedulings:     m.FailedSchedulings,
		PreemptionCount:       m.PreemptionCount,
		AverageSchedulingTime: m.AverageSchedulingTime,
		QueueDepth:            m.QueueDepth,
		NodesCount:            m.NodesCount,
	}
}

// SchedulingResult represents the result of a scheduling decision.
type SchedulingResult struct {
	Workload      *Workload
	Node          *Node
	Success       bool
	Error         error
	PreemptedPods []string
	Duration      time.Duration
}

// Scheduler is the main container scheduler.
type Scheduler struct {
	config     SchedulerConfig
	nodes      *NodeRegistry
	queue      *PriorityQueue
	quotas     *QuotaManager
	algorithm  Algorithm
	preemptor  *Preemptor
	binder     *Binder
	metrics    *SchedulerMetrics

	mu         sync.RWMutex
	workloads  map[string]*Workload
	scheduleMu sync.Mutex // serializes filter-select-bind in scheduleWorkload

	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	stopOnce   sync.Once
	stopped    bool

	// Callbacks
	onScheduled  func(*Workload, *Node)
	onFailed     func(*Workload, error)
	onPreempted  func(*Workload, *Node)
}

// NewScheduler creates a new scheduler with the given configuration.
func NewScheduler(config SchedulerConfig) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())

	nodes := NewNodeRegistry()
	queue := NewPriorityQueue(config.QueueSize)
	quotas := NewQuotaManager()
	binder := NewBinder(nodes)
	preemptor := NewPreemptor(nodes, config.PreemptionTimeout)

	var algo Algorithm
	switch config.Algorithm {
	case AlgorithmBinPack:
		algo = NewBinPackAlgorithm()
	case AlgorithmSpread:
		algo = NewSpreadAlgorithm()
	case AlgorithmRandom:
		algo = NewRandomAlgorithm()
	default:
		algo = NewBinPackAlgorithm()
	}

	return &Scheduler{
		config:    config,
		nodes:     nodes,
		queue:     queue,
		quotas:    quotas,
		algorithm: algo,
		preemptor: preemptor,
		binder:    binder,
		metrics:   &SchedulerMetrics{},
		workloads: make(map[string]*Workload),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Start begins the scheduler loop.
func (s *Scheduler) Start() error {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return ErrSchedulerStopped
	}
	s.mu.Unlock()

	// Start worker goroutines
	for i := 0; i < s.config.WorkerCount; i++ {
		s.wg.Add(1)
		go s.schedulingWorker(i)
	}

	// Start the main scheduling loop
	s.wg.Add(1)
	go s.schedulingLoop()

	return nil
}

// Stop gracefully stops the scheduler.
func (s *Scheduler) Stop() {
	s.stopOnce.Do(func() {
		s.mu.Lock()
		s.stopped = true
		s.mu.Unlock()

		s.cancel()
		s.wg.Wait()
	})
}

// schedulingLoop is the main scheduling loop.
func (s *Scheduler) schedulingLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.SchedulingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.processQueue()
		}
	}
}

// schedulingWorker processes workloads from the queue.
func (s *Scheduler) schedulingWorker(id int) {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			workload := s.queue.Pop()
			if workload == nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			result := s.scheduleWorkload(workload)
			s.handleSchedulingResult(result)
		}
	}
}

// processQueue triggers queue processing.
func (s *Scheduler) processQueue() {
	s.metrics.mu.Lock()
	s.metrics.QueueDepth = s.queue.Len()
	s.metrics.NodesCount = s.nodes.Count()
	s.metrics.mu.Unlock()
}

// Submit adds a workload to the scheduling queue.
func (s *Scheduler) Submit(workload *Workload) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		return ErrSchedulerStopped
	}

	// Validate workload
	if workload.ID == "" {
		return fmt.Errorf("workload ID is required")
	}

	// Check if already exists
	if _, exists := s.workloads[workload.ID]; exists {
		return fmt.Errorf("workload %s already exists", workload.ID)
	}

	// Check namespace quota
	if err := s.quotas.CheckQuota(workload.Namespace, workload.Resources.Requests); err != nil {
		return err
	}

	// Initialize workload state
	workload.State = WorkloadStatePending
	workload.CreatedAt = time.Now()

	// Store workload
	s.workloads[workload.ID] = workload

	// Add to queue
	s.queue.Push(workload)

	return nil
}

// Cancel removes a workload from the scheduler.
func (s *Scheduler) Cancel(workloadID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	workload, exists := s.workloads[workloadID]
	if !exists {
		return ErrWorkloadNotFound
	}

	// Remove from queue if pending
	if workload.State == WorkloadStatePending {
		s.queue.Remove(workloadID)
	}

	// Unbind if scheduled
	if workload.BoundNode != "" {
		s.binder.Unbind(workload)
		s.quotas.ReleaseQuota(workload.Namespace, workload.Resources.Requests)
	}

	delete(s.workloads, workloadID)
	return nil
}

// scheduleWorkload attempts to schedule a single workload.
func (s *Scheduler) scheduleWorkload(workload *Workload) *SchedulingResult {
	// Serialize filter-select-bind to prevent races on node resource fields
	s.scheduleMu.Lock()
	defer s.scheduleMu.Unlock()

	start := time.Now()
	result := &SchedulingResult{
		Workload: workload,
	}

	// Get all available nodes
	nodes := s.nodes.ListReady()
	if len(nodes) == 0 {
		result.Error = ErrNoNodesAvailable
		result.Duration = time.Since(start)
		return result
	}

	// Filter nodes based on constraints
	feasibleNodes := s.filterNodes(workload, nodes)
	if len(feasibleNodes) == 0 {
		// Try preemption if enabled
		if s.config.EnablePreemption {
			preemptResult := s.attemptPreemption(workload, nodes)
			if preemptResult != nil {
				result.Node = preemptResult.Node
				result.PreemptedPods = preemptResult.PreemptedWorkloads
				result.Success = true
				result.Duration = time.Since(start)
				return result
			}
		}
		result.Error = ErrInsufficientResources
		result.Duration = time.Since(start)
		return result
	}

	// Score and select the best node
	selectedNode := s.algorithm.SelectNode(workload, feasibleNodes)
	if selectedNode == nil {
		result.Error = ErrNoNodesAvailable
		result.Duration = time.Since(start)
		return result
	}

	// Bind workload to node
	if err := s.binder.Bind(workload, selectedNode); err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result
	}

	// Reserve quota
	s.quotas.ReserveQuota(workload.Namespace, workload.Resources.Requests)

	result.Node = selectedNode
	result.Success = true
	result.Duration = time.Since(start)
	return result
}

// filterNodes filters nodes based on workload constraints.
func (s *Scheduler) filterNodes(workload *Workload, nodes []*Node) []*Node {
	var feasible []*Node

	for _, node := range nodes {
		// Check if node has sufficient resources (thread-safe read)
		if !workload.Resources.Requests.FitsIn(node.GetAvailable()) {
			continue
		}

		// Check taints and tolerations
		if !s.toleratesTaints(workload, node) {
			continue
		}

		// Check affinity rules
		if workload.Affinity != nil {
			if !s.checkNodeAffinity(workload, node) {
				continue
			}
		}

		feasible = append(feasible, node)
	}

	return feasible
}

// toleratesTaints checks if workload tolerates all node taints.
func (s *Scheduler) toleratesTaints(workload *Workload, node *Node) bool {
	for _, taint := range node.Taints {
		if taint.Effect == TaintEffectNoSchedule || taint.Effect == TaintEffectNoExecute {
			tolerated := false
			for _, toleration := range workload.Tolerations {
				if toleration.Matches(taint) {
					tolerated = true
					break
				}
			}
			if !tolerated {
				return false
			}
		}
	}
	return true
}

// checkNodeAffinity checks node affinity requirements.
func (s *Scheduler) checkNodeAffinity(workload *Workload, node *Node) bool {
	if workload.Affinity == nil {
		return true
	}

	// Check required node affinity
	if workload.Affinity.NodeAffinity != nil {
		required := workload.Affinity.NodeAffinity.RequiredDuringScheduling
		if len(required) > 0 {
			if !matchesNodeSelector(node, required) {
				return false
			}
		}
	}

	return true
}

// attemptPreemption tries to preempt lower priority workloads.
func (s *Scheduler) attemptPreemption(workload *Workload, nodes []*Node) *PreemptionResult {
	result := s.preemptor.FindPreemptionCandidates(workload, nodes, s.getNodeWorkloads)
	if result == nil {
		return nil
	}

	// Execute preemption
	for _, victimID := range result.PreemptedWorkloads {
		s.mu.Lock()
		victim, exists := s.workloads[victimID]
		if exists {
			victim.State = WorkloadStateEvicted
			s.binder.Unbind(victim)
			s.quotas.ReleaseQuota(victim.Namespace, victim.Resources.Requests)

			if s.onPreempted != nil {
				s.onPreempted(victim, result.Node)
			}
		}
		s.mu.Unlock()
	}

	s.metrics.RecordPreemption()
	return result
}

// getNodeWorkloads returns workloads running on a node.
func (s *Scheduler) getNodeWorkloads(nodeID string) []*Workload {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var workloads []*Workload
	for _, w := range s.workloads {
		if w.BoundNode == nodeID && w.State == WorkloadStateScheduled {
			workloads = append(workloads, w)
		}
	}
	return workloads
}

// handleSchedulingResult processes the result of a scheduling attempt.
func (s *Scheduler) handleSchedulingResult(result *SchedulingResult) {
	s.metrics.RecordSchedulingAttempt(result.Success, result.Duration)

	s.mu.Lock()
	defer s.mu.Unlock()

	if result.Success {
		result.Workload.State = WorkloadStateScheduled
		result.Workload.ScheduledAt = time.Now()
		result.Workload.BoundNode = result.Node.ID

		if s.onScheduled != nil {
			s.onScheduled(result.Workload, result.Node)
		}
	} else {
		// Check retry count
		retries := 0
		if result.Workload.Annotations == nil {
			result.Workload.Annotations = make(map[string]string)
		}
		if r, ok := result.Workload.Annotations["scheduler/retries"]; ok {
			fmt.Sscanf(r, "%d", &retries)
		}

		if retries < s.config.MaxRetries {
			// Re-queue for retry
			retries++
			result.Workload.Annotations["scheduler/retries"] = fmt.Sprintf("%d", retries)
			s.queue.Push(result.Workload)
		} else {
			result.Workload.State = WorkloadStateFailed
			if s.onFailed != nil {
				s.onFailed(result.Workload, result.Error)
			}
		}
	}
}

// RegisterNode adds a new node to the scheduler.
func (s *Scheduler) RegisterNode(node *Node) error {
	return s.nodes.Register(node)
}

// UnregisterNode removes a node from the scheduler.
func (s *Scheduler) UnregisterNode(nodeID string) error {
	// First, reschedule all workloads on this node
	workloads := s.getNodeWorkloads(nodeID)
	for _, w := range workloads {
		s.mu.Lock()
		w.State = WorkloadStatePending
		w.BoundNode = ""
		s.queue.Push(w)
		s.mu.Unlock()
	}

	return s.nodes.Unregister(nodeID)
}

// UpdateNode updates a node's state.
func (s *Scheduler) UpdateNode(node *Node) error {
	return s.nodes.Update(node)
}

// SetQuota sets a resource quota for a namespace.
func (s *Scheduler) SetQuota(namespace string, quota *ResourceQuota) {
	s.quotas.SetQuota(namespace, quota)
}

// GetQuota returns the quota for a namespace.
func (s *Scheduler) GetQuota(namespace string) *ResourceQuota {
	return s.quotas.GetQuota(namespace)
}

// SetAlgorithm changes the scheduling algorithm.
func (s *Scheduler) SetAlgorithm(algo AlgorithmType) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch algo {
	case AlgorithmBinPack:
		s.algorithm = NewBinPackAlgorithm()
	case AlgorithmSpread:
		s.algorithm = NewSpreadAlgorithm()
	case AlgorithmRandom:
		s.algorithm = NewRandomAlgorithm()
	}
	s.config.Algorithm = algo
}

// GetWorkload returns a workload by ID.
func (s *Scheduler) GetWorkload(id string) (*Workload, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	workload, exists := s.workloads[id]
	if !exists {
		return nil, ErrWorkloadNotFound
	}
	return workload, nil
}

// ListWorkloads returns all workloads.
func (s *Scheduler) ListWorkloads() []*Workload {
	s.mu.RLock()
	defer s.mu.RUnlock()

	workloads := make([]*Workload, 0, len(s.workloads))
	for _, w := range s.workloads {
		workloads = append(workloads, w)
	}
	return workloads
}

// ListPendingWorkloads returns all pending workloads.
func (s *Scheduler) ListPendingWorkloads() []*Workload {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var pending []*Workload
	for _, w := range s.workloads {
		if w.State == WorkloadStatePending {
			pending = append(pending, w)
		}
	}
	return pending
}

// GetMetrics returns current scheduler metrics.
func (s *Scheduler) GetMetrics() SchedulerMetrics {
	return s.metrics.GetMetrics()
}

// OnScheduled sets a callback for successful scheduling.
func (s *Scheduler) OnScheduled(fn func(*Workload, *Node)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onScheduled = fn
}

// OnFailed sets a callback for scheduling failures.
func (s *Scheduler) OnFailed(fn func(*Workload, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onFailed = fn
}

// OnPreempted sets a callback for workload preemption.
func (s *Scheduler) OnPreempted(fn func(*Workload, *Node)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onPreempted = fn
}

// ScheduleSync synchronously schedules a workload.
func (s *Scheduler) ScheduleSync(workload *Workload) (*SchedulingResult, error) {
	if err := s.Submit(workload); err != nil {
		return nil, err
	}

	// Wait for scheduling
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return nil, fmt.Errorf("scheduling timeout for workload %s", workload.ID)
		case <-ticker.C:
			s.mu.RLock()
			w, exists := s.workloads[workload.ID]
			if !exists {
				s.mu.RUnlock()
				return nil, ErrWorkloadNotFound
			}

			if w.State == WorkloadStateScheduled {
				node, _ := s.nodes.Get(w.BoundNode)
				s.mu.RUnlock()
				return &SchedulingResult{
					Workload: w,
					Node:     node,
					Success:  true,
				}, nil
			}

			if w.State == WorkloadStateFailed {
				s.mu.RUnlock()
				return &SchedulingResult{
					Workload: w,
					Success:  false,
					Error:    fmt.Errorf("scheduling failed"),
				}, nil
			}
			s.mu.RUnlock()
		}
	}
}

// Reschedule forces rescheduling of a workload.
func (s *Scheduler) Reschedule(workloadID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	workload, exists := s.workloads[workloadID]
	if !exists {
		return ErrWorkloadNotFound
	}

	// Unbind if currently bound
	if workload.BoundNode != "" {
		s.binder.Unbind(workload)
		s.quotas.ReleaseQuota(workload.Namespace, workload.Resources.Requests)
	}

	workload.State = WorkloadStatePending
	workload.BoundNode = ""
	s.queue.Push(workload)

	return nil
}

// matchesNodeSelector checks if a node matches a selector.
func matchesNodeSelector(node *Node, selectors []NodeSelectorTerm) bool {
	for _, term := range selectors {
		if matchesNodeSelectorTerm(node, term) {
			return true
		}
	}
	return false
}

// matchesNodeSelectorTerm checks if a node matches a single selector term.
func matchesNodeSelectorTerm(node *Node, term NodeSelectorTerm) bool {
	for _, req := range term.MatchExpressions {
		if !matchesLabelRequirement(node.Labels, req) {
			return false
		}
	}
	return true
}

// matchesLabelRequirement checks if labels match a requirement.
func matchesLabelRequirement(labels map[string]string, req LabelSelectorRequirement) bool {
	value, exists := labels[req.Key]

	switch req.Operator {
	case LabelSelectorOpIn:
		if !exists {
			return false
		}
		for _, v := range req.Values {
			if v == value {
				return true
			}
		}
		return false
	case LabelSelectorOpNotIn:
		if !exists {
			return true
		}
		for _, v := range req.Values {
			if v == value {
				return false
			}
		}
		return true
	case LabelSelectorOpExists:
		return exists
	case LabelSelectorOpDoesNotExist:
		return !exists
	}

	return false
}
