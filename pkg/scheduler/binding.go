// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package scheduler

import (
	"fmt"
	"sync"
	"time"
)

// Binder handles binding workloads to nodes.
type Binder struct {
	mu       sync.RWMutex
	nodes    *NodeRegistry
	bindings map[string]*Binding
	history  *BindingHistory
}

// Binding represents a workload-to-node binding.
type Binding struct {
	WorkloadID  string
	NodeID      string
	Resources   Resources
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Status      BindingStatus
	Annotations map[string]string
}

// BindingStatus represents the status of a binding.
type BindingStatus string

const (
	BindingStatusPending   BindingStatus = "Pending"
	BindingStatusBound     BindingStatus = "Bound"
	BindingStatusUnbinding BindingStatus = "Unbinding"
	BindingStatusFailed    BindingStatus = "Failed"
)

// NewBinder creates a new binder.
func NewBinder(nodes *NodeRegistry) *Binder {
	return &Binder{
		nodes:    nodes,
		bindings: make(map[string]*Binding),
		history:  NewBindingHistory(1000),
	}
}

// Bind binds a workload to a node.
func (b *Binder) Bind(workload *Workload, node *Node) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Check if already bound
	if existing, exists := b.bindings[workload.ID]; exists {
		if existing.Status == BindingStatusBound {
			return ErrAlreadyBound
		}
	}

	// Verify node exists and is ready
	n, err := b.nodes.Get(node.ID)
	if err != nil {
		return err
	}
	if !n.IsReady() {
		return fmt.Errorf("node %s is not ready", node.ID)
	}

	// Allocate resources on node (Allocate does its own locking and checks)
	if err := n.Allocate(workload.Resources.Requests); err != nil {
		return err
	}

	// Create binding
	binding := &Binding{
		WorkloadID:  workload.ID,
		NodeID:      node.ID,
		Resources:   workload.Resources.Requests,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Status:      BindingStatusBound,
		Annotations: make(map[string]string),
	}

	b.bindings[workload.ID] = binding

	// Update workload
	workload.BoundNode = node.ID
	workload.State = WorkloadStateScheduled
	workload.ScheduledAt = time.Now()

	// Record in history
	b.history.RecordBinding(binding)

	return nil
}

// Unbind removes a workload binding.
func (b *Binder) Unbind(workload *Workload) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	binding, exists := b.bindings[workload.ID]
	if !exists {
		return ErrWorkloadNotFound
	}

	// Release resources on node
	node, err := b.nodes.Get(binding.NodeID)
	if err == nil {
		node.Release(binding.Resources)
	}

	// Update binding status
	binding.Status = BindingStatusUnbinding
	binding.UpdatedAt = time.Now()

	// Record unbinding
	b.history.RecordUnbinding(binding)

	// Remove binding
	delete(b.bindings, workload.ID)

	// Clear workload binding
	workload.BoundNode = ""

	return nil
}

// GetBinding returns a binding by workload ID.
func (b *Binder) GetBinding(workloadID string) (*Binding, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	binding, exists := b.bindings[workloadID]
	if !exists {
		return nil, ErrWorkloadNotFound
	}

	return binding, nil
}

// GetNodeBindings returns all bindings for a node.
func (b *Binder) GetNodeBindings(nodeID string) []*Binding {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var bindings []*Binding
	for _, binding := range b.bindings {
		if binding.NodeID == nodeID {
			bindings = append(bindings, binding)
		}
	}
	return bindings
}

// ListBindings returns all bindings.
func (b *Binder) ListBindings() []*Binding {
	b.mu.RLock()
	defer b.mu.RUnlock()

	bindings := make([]*Binding, 0, len(b.bindings))
	for _, binding := range b.bindings {
		bindings = append(bindings, binding)
	}
	return bindings
}

// UpdateBinding updates a binding's annotations.
func (b *Binder) UpdateBinding(workloadID string, annotations map[string]string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	binding, exists := b.bindings[workloadID]
	if !exists {
		return ErrWorkloadNotFound
	}

	for k, v := range annotations {
		binding.Annotations[k] = v
	}
	binding.UpdatedAt = time.Now()

	return nil
}

// RelocateBinding moves a workload to a new node.
func (b *Binder) RelocateBinding(workload *Workload, newNode *Node) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	binding, exists := b.bindings[workload.ID]
	if !exists {
		return ErrWorkloadNotFound
	}

	// Release from old node
	oldNode, err := b.nodes.Get(binding.NodeID)
	if err == nil {
		oldNode.Release(binding.Resources)
	}

	// Allocate on new node
	if err := newNode.Allocate(workload.Resources.Requests); err != nil {
		// Try to restore old binding
		if oldNode != nil {
			_ = oldNode.Allocate(binding.Resources)
		}
		return err
	}

	// Update binding
	oldNodeID := binding.NodeID
	binding.NodeID = newNode.ID
	binding.Resources = workload.Resources.Requests
	binding.UpdatedAt = time.Now()

	// Update workload
	workload.BoundNode = newNode.ID

	// Record relocation
	b.history.RecordRelocation(binding, oldNodeID)

	return nil
}

// BindingHistory tracks binding events.
type BindingHistory struct {
	mu      sync.RWMutex
	events  []BindingEvent
	maxSize int
}

// BindingEvent represents a binding event.
type BindingEvent struct {
	Type       BindingEventType
	WorkloadID string
	NodeID     string
	OldNodeID  string // For relocations
	Timestamp  time.Time
	Resources  Resources
}

// BindingEventType represents the type of binding event.
type BindingEventType string

const (
	BindingEventBind     BindingEventType = "Bind"
	BindingEventUnbind   BindingEventType = "Unbind"
	BindingEventRelocate BindingEventType = "Relocate"
)

// NewBindingHistory creates a new binding history.
func NewBindingHistory(maxSize int) *BindingHistory {
	return &BindingHistory{
		events:  make([]BindingEvent, 0),
		maxSize: maxSize,
	}
}

// RecordBinding records a binding event.
func (h *BindingHistory) RecordBinding(binding *Binding) {
	h.mu.Lock()
	defer h.mu.Unlock()

	event := BindingEvent{
		Type:       BindingEventBind,
		WorkloadID: binding.WorkloadID,
		NodeID:     binding.NodeID,
		Timestamp:  time.Now(),
		Resources:  binding.Resources,
	}

	h.addEvent(event)
}

// RecordUnbinding records an unbinding event.
func (h *BindingHistory) RecordUnbinding(binding *Binding) {
	h.mu.Lock()
	defer h.mu.Unlock()

	event := BindingEvent{
		Type:       BindingEventUnbind,
		WorkloadID: binding.WorkloadID,
		NodeID:     binding.NodeID,
		Timestamp:  time.Now(),
		Resources:  binding.Resources,
	}

	h.addEvent(event)
}

// RecordRelocation records a relocation event.
func (h *BindingHistory) RecordRelocation(binding *Binding, oldNodeID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	event := BindingEvent{
		Type:       BindingEventRelocate,
		WorkloadID: binding.WorkloadID,
		NodeID:     binding.NodeID,
		OldNodeID:  oldNodeID,
		Timestamp:  time.Now(),
		Resources:  binding.Resources,
	}

	h.addEvent(event)
}

// addEvent adds an event to history.
func (h *BindingHistory) addEvent(event BindingEvent) {
	h.events = append(h.events, event)
	if len(h.events) > h.maxSize {
		h.events = h.events[1:]
	}
}

// GetRecent returns recent binding events.
func (h *BindingHistory) GetRecent(count int) []BindingEvent {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if count > len(h.events) {
		count = len(h.events)
	}

	result := make([]BindingEvent, count)
	start := len(h.events) - count
	copy(result, h.events[start:])
	return result
}

// GetByWorkload returns events for a workload.
func (h *BindingHistory) GetByWorkload(workloadID string) []BindingEvent {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var result []BindingEvent
	for _, e := range h.events {
		if e.WorkloadID == workloadID {
			result = append(result, e)
		}
	}
	return result
}

// GetByNode returns events for a node.
func (h *BindingHistory) GetByNode(nodeID string) []BindingEvent {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var result []BindingEvent
	for _, e := range h.events {
		if e.NodeID == nodeID || e.OldNodeID == nodeID {
			result = append(result, e)
		}
	}
	return result
}

// BindingVerifier verifies binding integrity.
type BindingVerifier struct {
	binder *Binder
	nodes  *NodeRegistry
}

// NewBindingVerifier creates a new binding verifier.
func NewBindingVerifier(binder *Binder, nodes *NodeRegistry) *BindingVerifier {
	return &BindingVerifier{
		binder: binder,
		nodes:  nodes,
	}
}

// VerifyResult contains verification results.
type VerifyResult struct {
	Valid              bool
	Errors             []string
	OrphanedBindings   []string
	OverallocatedNodes []string
}

// Verify checks binding integrity.
func (v *BindingVerifier) Verify() *VerifyResult {
	result := &VerifyResult{
		Valid:              true,
		Errors:             []string{},
		OrphanedBindings:   []string{},
		OverallocatedNodes: []string{},
	}

	bindings := v.binder.ListBindings()
	nodes := v.nodes.List()

	// Build node lookup
	nodeMap := make(map[string]*Node)
	for _, n := range nodes {
		nodeMap[n.ID] = n
	}

	// Check each binding
	for _, binding := range bindings {
		node, exists := nodeMap[binding.NodeID]
		if !exists {
			result.Valid = false
			result.OrphanedBindings = append(result.OrphanedBindings, binding.WorkloadID)
			result.Errors = append(result.Errors, fmt.Sprintf(
				"binding %s references non-existent node %s",
				binding.WorkloadID, binding.NodeID))
			continue
		}

		// Verify node has this binding recorded
		if !v.nodeHasBinding(node, binding) {
			result.Errors = append(result.Errors, fmt.Sprintf(
				"node %s doesn't reflect binding for %s",
				node.ID, binding.WorkloadID))
		}
	}

	// Check node allocations
	nodeAllocations := make(map[string]Resources)
	for _, binding := range bindings {
		if binding.Status == BindingStatusBound {
			nodeAllocations[binding.NodeID] = nodeAllocations[binding.NodeID].Add(binding.Resources)
		}
	}

	for nodeID, allocated := range nodeAllocations {
		node, exists := nodeMap[nodeID]
		if !exists {
			continue
		}

		if allocated.CPU > node.Allocatable.CPU ||
			allocated.Memory > node.Allocatable.Memory {
			result.Valid = false
			result.OverallocatedNodes = append(result.OverallocatedNodes, nodeID)
			result.Errors = append(result.Errors, fmt.Sprintf(
				"node %s is overallocated", nodeID))
		}
	}

	return result
}

// nodeHasBinding checks if a node properly reflects a binding.
func (v *BindingVerifier) nodeHasBinding(node *Node, binding *Binding) bool {
	// This would check node's actual allocated resources
	// For now, we assume they match
	return true
}

// BindingReconciler reconciles binding state.
type BindingReconciler struct {
	binder *Binder
	nodes  *NodeRegistry
	mu     sync.Mutex
}

// NewBindingReconciler creates a new reconciler.
func NewBindingReconciler(binder *Binder, nodes *NodeRegistry) *BindingReconciler {
	return &BindingReconciler{
		binder: binder,
		nodes:  nodes,
	}
}

// ReconcileResult contains reconciliation results.
type ReconcileResult struct {
	CleanedBindings int
	RepairedNodes   int
	Errors          []string
}

// Reconcile reconciles binding state.
func (r *BindingReconciler) Reconcile() *ReconcileResult {
	r.mu.Lock()
	defer r.mu.Unlock()

	result := &ReconcileResult{
		Errors: []string{},
	}

	// Get current state
	bindings := r.binder.ListBindings()
	nodes := r.nodes.List()

	nodeMap := make(map[string]*Node)
	for _, n := range nodes {
		nodeMap[n.ID] = n
	}

	// Clean up orphaned bindings
	for _, binding := range bindings {
		if _, exists := nodeMap[binding.NodeID]; !exists {
			// Remove orphaned binding
			r.binder.mu.Lock()
			delete(r.binder.bindings, binding.WorkloadID)
			r.binder.mu.Unlock()
			result.CleanedBindings++
		}
	}

	// Recalculate node allocations
	for _, node := range nodes {
		nodeBindings := r.binder.GetNodeBindings(node.ID)

		expectedAlloc := Resources{}
		for _, b := range nodeBindings {
			if b.Status == BindingStatusBound {
				expectedAlloc = expectedAlloc.Add(b.Resources)
			}
		}

		// Compare with actual allocation
		if expectedAlloc.CPU != node.Allocated.CPU ||
			expectedAlloc.Memory != node.Allocated.Memory {
			// Fix allocation
			node.Allocated = expectedAlloc
			node.UpdateAvailable()
			result.RepairedNodes++
		}
	}

	return result
}

// BatchBinder handles batch binding operations.
type BatchBinder struct {
	binder *Binder
}

// NewBatchBinder creates a new batch binder.
func NewBatchBinder(binder *Binder) *BatchBinder {
	return &BatchBinder{
		binder: binder,
	}
}

// BatchBindRequest represents a batch bind request.
type BatchBindRequest struct {
	Workload *Workload
	Node     *Node
}

// BatchBindResult contains batch binding results.
type BatchBindResult struct {
	Successful []string
	Failed     map[string]error
}

// BatchBind binds multiple workloads.
func (bb *BatchBinder) BatchBind(requests []BatchBindRequest) *BatchBindResult {
	result := &BatchBindResult{
		Successful: []string{},
		Failed:     make(map[string]error),
	}

	for _, req := range requests {
		err := bb.binder.Bind(req.Workload, req.Node)
		if err != nil {
			result.Failed[req.Workload.ID] = err
		} else {
			result.Successful = append(result.Successful, req.Workload.ID)
		}
	}

	return result
}

// BatchUnbind unbinds multiple workloads.
func (bb *BatchBinder) BatchUnbind(workloads []*Workload) *BatchBindResult {
	result := &BatchBindResult{
		Successful: []string{},
		Failed:     make(map[string]error),
	}

	for _, w := range workloads {
		err := bb.binder.Unbind(w)
		if err != nil {
			result.Failed[w.ID] = err
		} else {
			result.Successful = append(result.Successful, w.ID)
		}
	}

	return result
}

// BindingExtender extends binding functionality.
type BindingExtender struct {
	preBindHooks  []PreBindHook
	postBindHooks []PostBindHook
}

// PreBindHook is called before binding.
type PreBindHook func(workload *Workload, node *Node) error

// PostBindHook is called after binding.
type PostBindHook func(workload *Workload, node *Node)

// NewBindingExtender creates a new binding extender.
func NewBindingExtender() *BindingExtender {
	return &BindingExtender{
		preBindHooks:  []PreBindHook{},
		postBindHooks: []PostBindHook{},
	}
}

// AddPreBindHook adds a pre-bind hook.
func (e *BindingExtender) AddPreBindHook(hook PreBindHook) {
	e.preBindHooks = append(e.preBindHooks, hook)
}

// AddPostBindHook adds a post-bind hook.
func (e *BindingExtender) AddPostBindHook(hook PostBindHook) {
	e.postBindHooks = append(e.postBindHooks, hook)
}

// RunPreBindHooks runs all pre-bind hooks.
func (e *BindingExtender) RunPreBindHooks(workload *Workload, node *Node) error {
	for _, hook := range e.preBindHooks {
		if err := hook(workload, node); err != nil {
			return err
		}
	}
	return nil
}

// RunPostBindHooks runs all post-bind hooks.
func (e *BindingExtender) RunPostBindHooks(workload *Workload, node *Node) {
	for _, hook := range e.postBindHooks {
		hook(workload, node)
	}
}

// BindingStats contains binding statistics.
type BindingStats struct {
	TotalBindings       int
	ActiveBindings      int
	BindingsByNode      map[string]int
	AverageBindingAge   time.Duration
	TotalResourcesBound Resources
}

// GetStats returns binding statistics.
func (b *Binder) GetStats() *BindingStats {
	b.mu.RLock()
	defer b.mu.RUnlock()

	stats := &BindingStats{
		TotalBindings:  len(b.bindings),
		BindingsByNode: make(map[string]int),
	}

	var totalAge time.Duration
	now := time.Now()

	for _, binding := range b.bindings {
		if binding.Status == BindingStatusBound {
			stats.ActiveBindings++
			stats.BindingsByNode[binding.NodeID]++
			stats.TotalResourcesBound = stats.TotalResourcesBound.Add(binding.Resources)
			totalAge += now.Sub(binding.CreatedAt)
		}
	}

	if stats.ActiveBindings > 0 {
		stats.AverageBindingAge = totalAge / time.Duration(stats.ActiveBindings)
	}

	return stats
}
