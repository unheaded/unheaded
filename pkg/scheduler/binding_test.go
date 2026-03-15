// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package scheduler

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// =============================================================================
// Helper Functions
// =============================================================================

func createTestNodeRegistry() *NodeRegistry {
	registry := NewNodeRegistry()
	return registry
}

func createTestNodeForBinding(id string, cpu, memory, disk int64) *Node {
	capacity := Resources{CPU: cpu, Memory: memory, Disk: disk}
	return NewNode(id, "test-node-"+id, capacity)
}

func createTestWorkloadForBinding(id string, cpu, memory, disk int64) *Workload {
	return &Workload{
		ID:    id,
		Name:  "test-workload-" + id,
		State: WorkloadStatePending,
		Resources: ResourceRequirements{
			Requests: Resources{CPU: cpu, Memory: memory, Disk: disk},
			Limits:   Resources{CPU: cpu * 2, Memory: memory * 2, Disk: disk * 2},
		},
	}
}

// =============================================================================
// Binder Tests
// =============================================================================

func TestNewBinder(t *testing.T) {
	registry := createTestNodeRegistry()
	binder := NewBinder(registry)

	if binder == nil {
		t.Fatal("NewBinder returned nil")
	}
	if binder.nodes != registry {
		t.Error("binder.nodes should reference the registry")
	}
	if binder.bindings == nil {
		t.Error("binder.bindings should be initialized")
	}
	if binder.history == nil {
		t.Error("binder.history should be initialized")
	}
}

func TestBinderBind(t *testing.T) {
	registry := createTestNodeRegistry()
	node := createTestNodeForBinding("node-1", 8000, 16384, 100000)
	_ = registry.Register(node)

	binder := NewBinder(registry)

	workload := createTestWorkloadForBinding("workload-1", 2000, 4096, 20000)

	err := binder.Bind(workload, node)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify binding was created
	binding, err := binder.GetBinding("workload-1")
	if err != nil {
		t.Errorf("failed to get binding: %v", err)
	}
	if binding.WorkloadID != "workload-1" {
		t.Errorf("expected workload ID 'workload-1', got '%s'", binding.WorkloadID)
	}
	if binding.NodeID != "node-1" {
		t.Errorf("expected node ID 'node-1', got '%s'", binding.NodeID)
	}
	if binding.Status != BindingStatusBound {
		t.Errorf("expected status Bound, got %s", binding.Status)
	}

	// Verify workload was updated
	if workload.BoundNode != "node-1" {
		t.Errorf("expected workload BoundNode 'node-1', got '%s'", workload.BoundNode)
	}
	if workload.State != WorkloadStateScheduled {
		t.Errorf("expected workload state Scheduled, got %s", workload.State)
	}

	// Verify node resources were allocated
	retrievedNode, _ := registry.Get("node-1")
	if retrievedNode.Allocated.CPU != 2000 {
		t.Errorf("expected node Allocated.CPU 2000, got %d", retrievedNode.Allocated.CPU)
	}
}

func TestBinderBindAlreadyBound(t *testing.T) {
	registry := createTestNodeRegistry()
	node := createTestNodeForBinding("node-1", 8000, 16384, 100000)
	_ = registry.Register(node)

	binder := NewBinder(registry)

	workload := createTestWorkloadForBinding("workload-1", 2000, 4096, 20000)

	// First bind
	_ = binder.Bind(workload, node)

	// Try to bind again
	err := binder.Bind(workload, node)
	if err != ErrAlreadyBound {
		t.Errorf("expected ErrAlreadyBound, got %v", err)
	}
}

func TestBinderBindNodeNotFound(t *testing.T) {
	registry := createTestNodeRegistry()
	binder := NewBinder(registry)

	workload := createTestWorkloadForBinding("workload-1", 2000, 4096, 20000)
	node := createTestNodeForBinding("nonexistent", 8000, 16384, 100000)

	err := binder.Bind(workload, node)
	if err != ErrNodeNotFound {
		t.Errorf("expected ErrNodeNotFound, got %v", err)
	}
}

func TestBinderBindNodeNotReady(t *testing.T) {
	registry := createTestNodeRegistry()
	node := createTestNodeForBinding("node-1", 8000, 16384, 100000)
	node.State = NodeStateNotReady
	_ = registry.Register(node)

	binder := NewBinder(registry)

	workload := createTestWorkloadForBinding("workload-1", 2000, 4096, 20000)

	err := binder.Bind(workload, node)
	if err == nil {
		t.Error("expected error for not ready node")
	}
}

func TestBinderBindInsufficientResources(t *testing.T) {
	registry := createTestNodeRegistry()
	node := createTestNodeForBinding("node-1", 4000, 8192, 50000)
	_ = registry.Register(node)

	binder := NewBinder(registry)

	// Workload requires more than node has
	workload := createTestWorkloadForBinding("workload-1", 5000, 4096, 20000)

	err := binder.Bind(workload, node)
	if err != ErrInsufficientResources {
		t.Errorf("expected ErrInsufficientResources, got %v", err)
	}
}

func TestBinderUnbind(t *testing.T) {
	registry := createTestNodeRegistry()
	node := createTestNodeForBinding("node-1", 8000, 16384, 100000)
	_ = registry.Register(node)

	binder := NewBinder(registry)

	workload := createTestWorkloadForBinding("workload-1", 2000, 4096, 20000)
	_ = binder.Bind(workload, node)

	err := binder.Unbind(workload)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify binding was removed
	_, err = binder.GetBinding("workload-1")
	if err != ErrWorkloadNotFound {
		t.Errorf("expected ErrWorkloadNotFound, got %v", err)
	}

	// Verify workload was updated
	if workload.BoundNode != "" {
		t.Errorf("expected empty BoundNode, got '%s'", workload.BoundNode)
	}

	// Verify node resources were released
	retrievedNode, _ := registry.Get("node-1")
	if retrievedNode.Allocated.CPU != 0 {
		t.Errorf("expected node Allocated.CPU 0, got %d", retrievedNode.Allocated.CPU)
	}
}

func TestBinderUnbindNotFound(t *testing.T) {
	registry := createTestNodeRegistry()
	binder := NewBinder(registry)

	workload := createTestWorkloadForBinding("nonexistent", 2000, 4096, 20000)

	err := binder.Unbind(workload)
	if err != ErrWorkloadNotFound {
		t.Errorf("expected ErrWorkloadNotFound, got %v", err)
	}
}

func TestBinderGetNodeBindings(t *testing.T) {
	registry := createTestNodeRegistry()
	node1 := createTestNodeForBinding("node-1", 8000, 16384, 100000)
	node2 := createTestNodeForBinding("node-2", 8000, 16384, 100000)
	_ = registry.Register(node1)
	_ = registry.Register(node2)

	binder := NewBinder(registry)

	// Bind 2 workloads to node-1
	workload1 := createTestWorkloadForBinding("workload-1", 1000, 2048, 10000)
	workload2 := createTestWorkloadForBinding("workload-2", 1000, 2048, 10000)
	_ = binder.Bind(workload1, node1)
	_ = binder.Bind(workload2, node1)

	// Bind 1 workload to node-2
	workload3 := createTestWorkloadForBinding("workload-3", 1000, 2048, 10000)
	_ = binder.Bind(workload3, node2)

	node1Bindings := binder.GetNodeBindings("node-1")
	if len(node1Bindings) != 2 {
		t.Errorf("expected 2 bindings for node-1, got %d", len(node1Bindings))
	}

	node2Bindings := binder.GetNodeBindings("node-2")
	if len(node2Bindings) != 1 {
		t.Errorf("expected 1 binding for node-2, got %d", len(node2Bindings))
	}
}

func TestBinderListBindings(t *testing.T) {
	registry := createTestNodeRegistry()
	node := createTestNodeForBinding("node-1", 8000, 16384, 100000)
	_ = registry.Register(node)

	binder := NewBinder(registry)

	// Bind multiple workloads
	for i := 0; i < 5; i++ {
		workload := createTestWorkloadForBinding(string(rune('a'+i)), 500, 1024, 5000)
		_ = binder.Bind(workload, node)
	}

	bindings := binder.ListBindings()
	if len(bindings) != 5 {
		t.Errorf("expected 5 bindings, got %d", len(bindings))
	}
}

func TestBinderUpdateBinding(t *testing.T) {
	registry := createTestNodeRegistry()
	node := createTestNodeForBinding("node-1", 8000, 16384, 100000)
	_ = registry.Register(node)

	binder := NewBinder(registry)

	workload := createTestWorkloadForBinding("workload-1", 2000, 4096, 20000)
	_ = binder.Bind(workload, node)

	annotations := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}
	err := binder.UpdateBinding("workload-1", annotations)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	binding, _ := binder.GetBinding("workload-1")
	if binding.Annotations["key1"] != "value1" {
		t.Errorf("expected annotation key1=value1, got %s", binding.Annotations["key1"])
	}
	if binding.Annotations["key2"] != "value2" {
		t.Errorf("expected annotation key2=value2, got %s", binding.Annotations["key2"])
	}
}

func TestBinderUpdateBindingNotFound(t *testing.T) {
	registry := createTestNodeRegistry()
	binder := NewBinder(registry)

	err := binder.UpdateBinding("nonexistent", map[string]string{})
	if err != ErrWorkloadNotFound {
		t.Errorf("expected ErrWorkloadNotFound, got %v", err)
	}
}

func TestBinderRelocateBinding(t *testing.T) {
	registry := createTestNodeRegistry()
	node1 := createTestNodeForBinding("node-1", 8000, 16384, 100000)
	node2 := createTestNodeForBinding("node-2", 8000, 16384, 100000)
	_ = registry.Register(node1)
	_ = registry.Register(node2)

	binder := NewBinder(registry)

	workload := createTestWorkloadForBinding("workload-1", 2000, 4096, 20000)
	_ = binder.Bind(workload, node1)

	err := binder.RelocateBinding(workload, node2)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify binding was updated
	binding, _ := binder.GetBinding("workload-1")
	if binding.NodeID != "node-2" {
		t.Errorf("expected binding NodeID 'node-2', got '%s'", binding.NodeID)
	}

	// Verify workload was updated
	if workload.BoundNode != "node-2" {
		t.Errorf("expected workload BoundNode 'node-2', got '%s'", workload.BoundNode)
	}

	// Verify old node resources were released
	retrievedNode1, _ := registry.Get("node-1")
	if retrievedNode1.Allocated.CPU != 0 {
		t.Errorf("expected node-1 Allocated.CPU 0, got %d", retrievedNode1.Allocated.CPU)
	}

	// Verify new node resources were allocated
	retrievedNode2, _ := registry.Get("node-2")
	if retrievedNode2.Allocated.CPU != 2000 {
		t.Errorf("expected node-2 Allocated.CPU 2000, got %d", retrievedNode2.Allocated.CPU)
	}
}

func TestBinderRelocateBindingNotFound(t *testing.T) {
	registry := createTestNodeRegistry()
	node := createTestNodeForBinding("node-1", 8000, 16384, 100000)
	_ = registry.Register(node)

	binder := NewBinder(registry)

	workload := createTestWorkloadForBinding("nonexistent", 2000, 4096, 20000)

	err := binder.RelocateBinding(workload, node)
	if err != ErrWorkloadNotFound {
		t.Errorf("expected ErrWorkloadNotFound, got %v", err)
	}
}

func TestBinderRelocateBindingInsufficientResources(t *testing.T) {
	registry := createTestNodeRegistry()
	node1 := createTestNodeForBinding("node-1", 8000, 16384, 100000)
	node2 := createTestNodeForBinding("node-2", 1000, 2048, 10000) // Small node
	_ = registry.Register(node1)
	_ = registry.Register(node2)

	binder := NewBinder(registry)

	workload := createTestWorkloadForBinding("workload-1", 4000, 8192, 40000)
	_ = binder.Bind(workload, node1)

	// Try to relocate to smaller node
	err := binder.RelocateBinding(workload, node2)
	if err == nil {
		t.Error("expected error for insufficient resources")
	}

	// Verify binding was not changed
	binding, _ := binder.GetBinding("workload-1")
	if binding.NodeID != "node-1" {
		t.Errorf("expected binding NodeID 'node-1' (unchanged), got '%s'", binding.NodeID)
	}
}

func TestBinderGetStats(t *testing.T) {
	registry := createTestNodeRegistry()
	node := createTestNodeForBinding("node-1", 8000, 16384, 100000)
	_ = registry.Register(node)

	binder := NewBinder(registry)

	// Bind multiple workloads
	for i := 0; i < 3; i++ {
		workload := createTestWorkloadForBinding(string(rune('a'+i)), 1000, 2048, 10000)
		_ = binder.Bind(workload, node)
	}

	stats := binder.GetStats()

	if stats.TotalBindings != 3 {
		t.Errorf("expected TotalBindings 3, got %d", stats.TotalBindings)
	}
	if stats.ActiveBindings != 3 {
		t.Errorf("expected ActiveBindings 3, got %d", stats.ActiveBindings)
	}
	if stats.BindingsByNode["node-1"] != 3 {
		t.Errorf("expected 3 bindings for node-1, got %d", stats.BindingsByNode["node-1"])
	}
	if stats.TotalResourcesBound.CPU != 3000 {
		t.Errorf("expected TotalResourcesBound.CPU 3000, got %d", stats.TotalResourcesBound.CPU)
	}
}

func TestBinderConcurrentAccess(t *testing.T) {
	registry := createTestNodeRegistry()
	node := createTestNodeForBinding("node-1", 100000, 200000, 1000000)
	_ = registry.Register(node)

	binder := NewBinder(registry)

	var wg sync.WaitGroup
	numGoroutines := 50

	// Concurrent binds
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			workload := createTestWorkloadForBinding(string(rune(id)), 100, 200, 1000)
			_ = binder.Bind(workload, node)
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = binder.ListBindings()
			_ = binder.GetNodeBindings("node-1")
			_ = binder.GetStats()
		}()
	}

	wg.Wait()

	// Verify some bindings were created
	bindings := binder.ListBindings()
	if len(bindings) == 0 {
		t.Error("expected some bindings to be created")
	}
}

// =============================================================================
// BindingHistory Tests
// =============================================================================

func TestNewBindingHistory(t *testing.T) {
	history := NewBindingHistory(100)

	if history == nil {
		t.Fatal("NewBindingHistory returned nil")
	}
	if history.maxSize != 100 {
		t.Errorf("expected maxSize 100, got %d", history.maxSize)
	}
}

func TestBindingHistoryRecordBinding(t *testing.T) {
	history := NewBindingHistory(100)

	binding := &Binding{
		WorkloadID: "workload-1",
		NodeID:     "node-1",
		Resources:  Resources{CPU: 1000, Memory: 2048},
	}

	history.RecordBinding(binding)

	events := history.GetRecent(10)
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != BindingEventBind {
		t.Errorf("expected event type Bind, got %s", events[0].Type)
	}
	if events[0].WorkloadID != "workload-1" {
		t.Errorf("expected workload ID 'workload-1', got '%s'", events[0].WorkloadID)
	}
}

func TestBindingHistoryRecordUnbinding(t *testing.T) {
	history := NewBindingHistory(100)

	binding := &Binding{
		WorkloadID: "workload-1",
		NodeID:     "node-1",
		Resources:  Resources{CPU: 1000, Memory: 2048},
	}

	history.RecordUnbinding(binding)

	events := history.GetRecent(10)
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != BindingEventUnbind {
		t.Errorf("expected event type Unbind, got %s", events[0].Type)
	}
}

func TestBindingHistoryRecordRelocation(t *testing.T) {
	history := NewBindingHistory(100)

	binding := &Binding{
		WorkloadID: "workload-1",
		NodeID:     "node-2",
		Resources:  Resources{CPU: 1000, Memory: 2048},
	}

	history.RecordRelocation(binding, "node-1")

	events := history.GetRecent(10)
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != BindingEventRelocate {
		t.Errorf("expected event type Relocate, got %s", events[0].Type)
	}
	if events[0].OldNodeID != "node-1" {
		t.Errorf("expected old node ID 'node-1', got '%s'", events[0].OldNodeID)
	}
	if events[0].NodeID != "node-2" {
		t.Errorf("expected new node ID 'node-2', got '%s'", events[0].NodeID)
	}
}

func TestBindingHistoryMaxSize(t *testing.T) {
	maxSize := 5
	history := NewBindingHistory(maxSize)

	// Add more events than maxSize
	for i := 0; i < 10; i++ {
		binding := &Binding{
			WorkloadID: string(rune('a' + i)),
			NodeID:     "node-1",
		}
		history.RecordBinding(binding)
	}

	events := history.GetRecent(100)
	if len(events) != maxSize {
		t.Errorf("expected %d events (maxSize), got %d", maxSize, len(events))
	}

	// Should have the most recent events
	if events[0].WorkloadID != "f" {
		t.Errorf("expected oldest event to be 'f', got '%s'", events[0].WorkloadID)
	}
}

func TestBindingHistoryGetRecent(t *testing.T) {
	history := NewBindingHistory(100)

	for i := 0; i < 10; i++ {
		binding := &Binding{
			WorkloadID: string(rune('a' + i)),
			NodeID:     "node-1",
		}
		history.RecordBinding(binding)
	}

	// Get fewer than available
	events := history.GetRecent(3)
	if len(events) != 3 {
		t.Errorf("expected 3 events, got %d", len(events))
	}

	// Get more than available
	events = history.GetRecent(100)
	if len(events) != 10 {
		t.Errorf("expected 10 events, got %d", len(events))
	}
}

func TestBindingHistoryGetByWorkload(t *testing.T) {
	history := NewBindingHistory(100)

	// Record multiple events for different workloads
	binding1 := &Binding{WorkloadID: "workload-1", NodeID: "node-1"}
	binding2 := &Binding{WorkloadID: "workload-2", NodeID: "node-1"}

	history.RecordBinding(binding1)
	history.RecordBinding(binding2)
	history.RecordUnbinding(binding1)
	history.RecordBinding(binding1)

	events := history.GetByWorkload("workload-1")
	if len(events) != 3 {
		t.Errorf("expected 3 events for workload-1, got %d", len(events))
	}

	events = history.GetByWorkload("workload-2")
	if len(events) != 1 {
		t.Errorf("expected 1 event for workload-2, got %d", len(events))
	}
}

func TestBindingHistoryGetByNode(t *testing.T) {
	history := NewBindingHistory(100)

	binding1 := &Binding{WorkloadID: "workload-1", NodeID: "node-1"}
	binding2 := &Binding{WorkloadID: "workload-2", NodeID: "node-2"}
	binding3 := &Binding{WorkloadID: "workload-3", NodeID: "node-2"}

	history.RecordBinding(binding1)
	history.RecordBinding(binding2)
	history.RecordBinding(binding3)

	// Also test relocation includes old node
	binding3.NodeID = "node-1"
	history.RecordRelocation(binding3, "node-2")

	eventsNode1 := history.GetByNode("node-1")
	if len(eventsNode1) != 2 { // binding1 + relocation
		t.Errorf("expected 2 events for node-1, got %d", len(eventsNode1))
	}

	eventsNode2 := history.GetByNode("node-2")
	if len(eventsNode2) != 3 { // binding2 + binding3 + relocation (as old node)
		t.Errorf("expected 3 events for node-2, got %d", len(eventsNode2))
	}
}

func TestBindingHistoryConcurrentAccess(t *testing.T) {
	history := NewBindingHistory(1000)

	var wg sync.WaitGroup
	numGoroutines := 50

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			binding := &Binding{
				WorkloadID: string(rune(id)),
				NodeID:     "node-1",
			}
			history.RecordBinding(binding)
			history.RecordUnbinding(binding)
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = history.GetRecent(10)
			_ = history.GetByWorkload("workload-1")
			_ = history.GetByNode("node-1")
		}()
	}

	wg.Wait()
}

// =============================================================================
// BindingVerifier Tests
// =============================================================================

func TestNewBindingVerifier(t *testing.T) {
	registry := createTestNodeRegistry()
	binder := NewBinder(registry)

	verifier := NewBindingVerifier(binder, registry)

	if verifier == nil {
		t.Fatal("NewBindingVerifier returned nil")
	}
	if verifier.binder != binder {
		t.Error("verifier.binder should reference the binder")
	}
}

func TestBindingVerifierVerifyValid(t *testing.T) {
	registry := createTestNodeRegistry()
	node := createTestNodeForBinding("node-1", 8000, 16384, 100000)
	_ = registry.Register(node)

	binder := NewBinder(registry)

	workload := createTestWorkloadForBinding("workload-1", 2000, 4096, 20000)
	_ = binder.Bind(workload, node)

	verifier := NewBindingVerifier(binder, registry)
	result := verifier.Verify()

	if !result.Valid {
		t.Errorf("expected valid result, got invalid with errors: %v", result.Errors)
	}
	if len(result.OrphanedBindings) != 0 {
		t.Errorf("expected no orphaned bindings, got %d", len(result.OrphanedBindings))
	}
}

func TestBindingVerifierVerifyOrphanedBinding(t *testing.T) {
	registry := createTestNodeRegistry()
	node := createTestNodeForBinding("node-1", 8000, 16384, 100000)
	_ = registry.Register(node)

	binder := NewBinder(registry)

	workload := createTestWorkloadForBinding("workload-1", 2000, 4096, 20000)
	_ = binder.Bind(workload, node)

	// Remove the node to create an orphaned binding
	_ = registry.Unregister("node-1")

	verifier := NewBindingVerifier(binder, registry)
	result := verifier.Verify()

	if result.Valid {
		t.Error("expected invalid result due to orphaned binding")
	}
	if len(result.OrphanedBindings) != 1 {
		t.Errorf("expected 1 orphaned binding, got %d", len(result.OrphanedBindings))
	}
	if result.OrphanedBindings[0] != "workload-1" {
		t.Errorf("expected orphaned binding 'workload-1', got '%s'", result.OrphanedBindings[0])
	}
}

// =============================================================================
// BindingReconciler Tests
// =============================================================================

func TestNewBindingReconciler(t *testing.T) {
	registry := createTestNodeRegistry()
	binder := NewBinder(registry)

	reconciler := NewBindingReconciler(binder, registry)

	if reconciler == nil {
		t.Fatal("NewBindingReconciler returned nil")
	}
}

func TestBindingReconcilerReconcile(t *testing.T) {
	registry := createTestNodeRegistry()
	node := createTestNodeForBinding("node-1", 8000, 16384, 100000)
	_ = registry.Register(node)

	binder := NewBinder(registry)

	workload := createTestWorkloadForBinding("workload-1", 2000, 4096, 20000)
	_ = binder.Bind(workload, node)

	reconciler := NewBindingReconciler(binder, registry)
	result := reconciler.Reconcile()

	if len(result.Errors) != 0 {
		t.Errorf("unexpected errors: %v", result.Errors)
	}
}

func TestBindingReconcilerCleansOrphanedBindings(t *testing.T) {
	registry := createTestNodeRegistry()
	node := createTestNodeForBinding("node-1", 8000, 16384, 100000)
	_ = registry.Register(node)

	binder := NewBinder(registry)

	workload := createTestWorkloadForBinding("workload-1", 2000, 4096, 20000)
	_ = binder.Bind(workload, node)

	// Remove the node
	_ = registry.Unregister("node-1")

	reconciler := NewBindingReconciler(binder, registry)
	result := reconciler.Reconcile()

	if result.CleanedBindings != 1 {
		t.Errorf("expected 1 cleaned binding, got %d", result.CleanedBindings)
	}

	// Verify binding was removed
	_, err := binder.GetBinding("workload-1")
	if err != ErrWorkloadNotFound {
		t.Errorf("expected binding to be removed, got err: %v", err)
	}
}

func TestBindingReconcilerRepairsNodeAllocation(t *testing.T) {
	registry := createTestNodeRegistry()
	node := createTestNodeForBinding("node-1", 8000, 16384, 100000)
	_ = registry.Register(node)

	binder := NewBinder(registry)

	workload := createTestWorkloadForBinding("workload-1", 2000, 4096, 20000)
	_ = binder.Bind(workload, node)

	// Manually corrupt node allocation
	node.Allocated = Resources{CPU: 9999, Memory: 9999, Disk: 9999}

	reconciler := NewBindingReconciler(binder, registry)
	result := reconciler.Reconcile()

	if result.RepairedNodes != 1 {
		t.Errorf("expected 1 repaired node, got %d", result.RepairedNodes)
	}

	// Verify node allocation was fixed
	if node.Allocated.CPU != 2000 {
		t.Errorf("expected node Allocated.CPU 2000, got %d", node.Allocated.CPU)
	}
}

// =============================================================================
// BatchBinder Tests
// =============================================================================

func TestNewBatchBinder(t *testing.T) {
	registry := createTestNodeRegistry()
	binder := NewBinder(registry)

	batchBinder := NewBatchBinder(binder)

	if batchBinder == nil {
		t.Fatal("NewBatchBinder returned nil")
	}
}

func TestBatchBinderBatchBind(t *testing.T) {
	registry := createTestNodeRegistry()
	node := createTestNodeForBinding("node-1", 8000, 16384, 100000)
	_ = registry.Register(node)

	binder := NewBinder(registry)
	batchBinder := NewBatchBinder(binder)

	requests := []BatchBindRequest{
		{Workload: createTestWorkloadForBinding("w1", 1000, 2048, 10000), Node: node},
		{Workload: createTestWorkloadForBinding("w2", 1000, 2048, 10000), Node: node},
		{Workload: createTestWorkloadForBinding("w3", 1000, 2048, 10000), Node: node},
	}

	result := batchBinder.BatchBind(requests)

	if len(result.Successful) != 3 {
		t.Errorf("expected 3 successful bindings, got %d", len(result.Successful))
	}
	if len(result.Failed) != 0 {
		t.Errorf("expected 0 failed bindings, got %d", len(result.Failed))
	}
}

func TestBatchBinderBatchBindWithFailures(t *testing.T) {
	registry := createTestNodeRegistry()
	node := createTestNodeForBinding("node-1", 3000, 6144, 30000) // Small capacity
	_ = registry.Register(node)

	binder := NewBinder(registry)
	batchBinder := NewBatchBinder(binder)

	requests := []BatchBindRequest{
		{Workload: createTestWorkloadForBinding("w1", 1000, 2048, 10000), Node: node},
		{Workload: createTestWorkloadForBinding("w2", 1000, 2048, 10000), Node: node},
		{Workload: createTestWorkloadForBinding("w3", 2000, 4096, 20000), Node: node}, // Will fail
	}

	result := batchBinder.BatchBind(requests)

	if len(result.Successful) != 2 {
		t.Errorf("expected 2 successful bindings, got %d", len(result.Successful))
	}
	if len(result.Failed) != 1 {
		t.Errorf("expected 1 failed binding, got %d", len(result.Failed))
	}
	if _, exists := result.Failed["w3"]; !exists {
		t.Error("expected w3 to be in failed bindings")
	}
}

func TestBatchBinderBatchUnbind(t *testing.T) {
	registry := createTestNodeRegistry()
	node := createTestNodeForBinding("node-1", 8000, 16384, 100000)
	_ = registry.Register(node)

	binder := NewBinder(registry)
	batchBinder := NewBatchBinder(binder)

	// First bind some workloads
	workloads := []*Workload{
		createTestWorkloadForBinding("w1", 1000, 2048, 10000),
		createTestWorkloadForBinding("w2", 1000, 2048, 10000),
		createTestWorkloadForBinding("w3", 1000, 2048, 10000),
	}
	for _, w := range workloads {
		_ = binder.Bind(w, node)
	}

	// Unbind all
	result := batchBinder.BatchUnbind(workloads)

	if len(result.Successful) != 3 {
		t.Errorf("expected 3 successful unbindings, got %d", len(result.Successful))
	}
	if len(result.Failed) != 0 {
		t.Errorf("expected 0 failed unbindings, got %d", len(result.Failed))
	}
}

func TestBatchBinderBatchUnbindWithFailures(t *testing.T) {
	registry := createTestNodeRegistry()
	node := createTestNodeForBinding("node-1", 8000, 16384, 100000)
	_ = registry.Register(node)

	binder := NewBinder(registry)
	batchBinder := NewBatchBinder(binder)

	// Bind only 2 workloads
	w1 := createTestWorkloadForBinding("w1", 1000, 2048, 10000)
	w2 := createTestWorkloadForBinding("w2", 1000, 2048, 10000)
	_ = binder.Bind(w1, node)
	_ = binder.Bind(w2, node)

	// Try to unbind 3 (one doesn't exist)
	workloads := []*Workload{w1, w2, createTestWorkloadForBinding("w3", 1000, 2048, 10000)}
	result := batchBinder.BatchUnbind(workloads)

	if len(result.Successful) != 2 {
		t.Errorf("expected 2 successful unbindings, got %d", len(result.Successful))
	}
	if len(result.Failed) != 1 {
		t.Errorf("expected 1 failed unbinding, got %d", len(result.Failed))
	}
}

// =============================================================================
// BindingExtender Tests
// =============================================================================

func TestNewBindingExtender(t *testing.T) {
	extender := NewBindingExtender()

	if extender == nil {
		t.Fatal("NewBindingExtender returned nil")
	}
	if len(extender.preBindHooks) != 0 {
		t.Error("expected empty preBindHooks")
	}
	if len(extender.postBindHooks) != 0 {
		t.Error("expected empty postBindHooks")
	}
}

func TestBindingExtenderAddPreBindHook(t *testing.T) {
	extender := NewBindingExtender()

	hookCalled := false
	extender.AddPreBindHook(func(workload *Workload, node *Node) error {
		hookCalled = true
		return nil
	})

	if len(extender.preBindHooks) != 1 {
		t.Errorf("expected 1 preBindHook, got %d", len(extender.preBindHooks))
	}

	// Run hooks
	workload := &Workload{ID: "test"}
	node := &Node{ID: "test"}
	_ = extender.RunPreBindHooks(workload, node)

	if !hookCalled {
		t.Error("expected hook to be called")
	}
}

func TestBindingExtenderPreBindHookError(t *testing.T) {
	extender := NewBindingExtender()

	expectedErr := errors.New("hook error")
	extender.AddPreBindHook(func(workload *Workload, node *Node) error {
		return expectedErr
	})

	workload := &Workload{ID: "test"}
	node := &Node{ID: "test"}
	err := extender.RunPreBindHooks(workload, node)

	if err != expectedErr {
		t.Errorf("expected error '%v', got '%v'", expectedErr, err)
	}
}

func TestBindingExtenderAddPostBindHook(t *testing.T) {
	extender := NewBindingExtender()

	hookCalled := false
	extender.AddPostBindHook(func(workload *Workload, node *Node) {
		hookCalled = true
	})

	if len(extender.postBindHooks) != 1 {
		t.Errorf("expected 1 postBindHook, got %d", len(extender.postBindHooks))
	}

	// Run hooks
	workload := &Workload{ID: "test"}
	node := &Node{ID: "test"}
	extender.RunPostBindHooks(workload, node)

	if !hookCalled {
		t.Error("expected hook to be called")
	}
}

func TestBindingExtenderMultipleHooks(t *testing.T) {
	extender := NewBindingExtender()

	callOrder := []int{}

	extender.AddPreBindHook(func(workload *Workload, node *Node) error {
		callOrder = append(callOrder, 1)
		return nil
	})
	extender.AddPreBindHook(func(workload *Workload, node *Node) error {
		callOrder = append(callOrder, 2)
		return nil
	})
	extender.AddPreBindHook(func(workload *Workload, node *Node) error {
		callOrder = append(callOrder, 3)
		return nil
	})

	workload := &Workload{ID: "test"}
	node := &Node{ID: "test"}
	_ = extender.RunPreBindHooks(workload, node)

	if len(callOrder) != 3 {
		t.Errorf("expected 3 hooks called, got %d", len(callOrder))
	}
	for i, v := range callOrder {
		if v != i+1 {
			t.Errorf("hooks called out of order")
			break
		}
	}
}

func TestBindingExtenderPreBindHookStopsOnError(t *testing.T) {
	extender := NewBindingExtender()

	hooksCalled := 0
	extender.AddPreBindHook(func(workload *Workload, node *Node) error {
		hooksCalled++
		return nil
	})
	extender.AddPreBindHook(func(workload *Workload, node *Node) error {
		hooksCalled++
		return errors.New("error")
	})
	extender.AddPreBindHook(func(workload *Workload, node *Node) error {
		hooksCalled++
		return nil
	})

	workload := &Workload{ID: "test"}
	node := &Node{ID: "test"}
	_ = extender.RunPreBindHooks(workload, node)

	if hooksCalled != 2 {
		t.Errorf("expected 2 hooks called (stop on error), got %d", hooksCalled)
	}
}

// =============================================================================
// Edge Case Tests
// =============================================================================

func TestBinderEmptyRegistry(t *testing.T) {
	registry := createTestNodeRegistry()
	binder := NewBinder(registry)

	// No nodes registered
	bindings := binder.ListBindings()
	if len(bindings) != 0 {
		t.Errorf("expected 0 bindings, got %d", len(bindings))
	}

	stats := binder.GetStats()
	if stats.TotalBindings != 0 {
		t.Errorf("expected 0 total bindings, got %d", stats.TotalBindings)
	}
}

func TestBinderMultipleWorkloadsSameNode(t *testing.T) {
	registry := createTestNodeRegistry()
	node := createTestNodeForBinding("node-1", 8000, 16384, 100000)
	_ = registry.Register(node)

	binder := NewBinder(registry)

	// Bind multiple workloads to same node
	for i := 0; i < 5; i++ {
		workload := createTestWorkloadForBinding(string(rune('a'+i)), 1000, 2048, 10000)
		err := binder.Bind(workload, node)
		if err != nil {
			t.Errorf("failed to bind workload %d: %v", i, err)
		}
	}

	// Verify all bindings exist
	bindings := binder.ListBindings()
	if len(bindings) != 5 {
		t.Errorf("expected 5 bindings, got %d", len(bindings))
	}

	// Verify node resources
	retrievedNode, _ := registry.Get("node-1")
	if retrievedNode.Allocated.CPU != 5000 {
		t.Errorf("expected node Allocated.CPU 5000, got %d", retrievedNode.Allocated.CPU)
	}
}

func TestBindingHistoryEmpty(t *testing.T) {
	history := NewBindingHistory(100)

	events := history.GetRecent(10)
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}

	events = history.GetByWorkload("nonexistent")
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}

	events = history.GetByNode("nonexistent")
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

func TestBindingStatusTransitions(t *testing.T) {
	registry := createTestNodeRegistry()
	node := createTestNodeForBinding("node-1", 8000, 16384, 100000)
	_ = registry.Register(node)

	binder := NewBinder(registry)

	workload := createTestWorkloadForBinding("workload-1", 2000, 4096, 20000)

	// Initially no binding
	_, err := binder.GetBinding("workload-1")
	if err != ErrWorkloadNotFound {
		t.Errorf("expected ErrWorkloadNotFound, got %v", err)
	}

	// Bind -> status should be Bound
	_ = binder.Bind(workload, node)
	binding, _ := binder.GetBinding("workload-1")
	if binding.Status != BindingStatusBound {
		t.Errorf("expected status Bound, got %s", binding.Status)
	}

	// Unbind removes the binding
	_ = binder.Unbind(workload)
	_, err = binder.GetBinding("workload-1")
	if err != ErrWorkloadNotFound {
		t.Errorf("expected ErrWorkloadNotFound after unbind, got %v", err)
	}
}

func TestBindingWithAnnotations(t *testing.T) {
	registry := createTestNodeRegistry()
	node := createTestNodeForBinding("node-1", 8000, 16384, 100000)
	_ = registry.Register(node)

	binder := NewBinder(registry)

	workload := createTestWorkloadForBinding("workload-1", 2000, 4096, 20000)
	_ = binder.Bind(workload, node)

	// Add annotations
	_ = binder.UpdateBinding("workload-1", map[string]string{
		"owner":   "team-a",
		"project": "demo",
	})

	// Add more annotations
	_ = binder.UpdateBinding("workload-1", map[string]string{
		"version": "v1.0",
	})

	binding, _ := binder.GetBinding("workload-1")
	if len(binding.Annotations) != 3 {
		t.Errorf("expected 3 annotations, got %d", len(binding.Annotations))
	}
	if binding.Annotations["owner"] != "team-a" {
		t.Errorf("expected owner=team-a, got %s", binding.Annotations["owner"])
	}
}

func TestBindingTimestamps(t *testing.T) {
	registry := createTestNodeRegistry()
	node := createTestNodeForBinding("node-1", 8000, 16384, 100000)
	_ = registry.Register(node)

	binder := NewBinder(registry)

	beforeBind := time.Now()
	time.Sleep(1 * time.Millisecond)

	workload := createTestWorkloadForBinding("workload-1", 2000, 4096, 20000)
	_ = binder.Bind(workload, node)

	time.Sleep(1 * time.Millisecond)
	afterBind := time.Now()

	binding, _ := binder.GetBinding("workload-1")

	if binding.CreatedAt.Before(beforeBind) || binding.CreatedAt.After(afterBind) {
		t.Error("CreatedAt should be between beforeBind and afterBind")
	}
	if binding.UpdatedAt.Before(beforeBind) || binding.UpdatedAt.After(afterBind) {
		t.Error("UpdatedAt should be between beforeBind and afterBind")
	}

	// Update binding and check UpdatedAt changes
	time.Sleep(10 * time.Millisecond)
	_ = binder.UpdateBinding("workload-1", map[string]string{"key": "value"})

	updatedBinding, _ := binder.GetBinding("workload-1")
	// Allow for equal time (same millisecond) or after
	if updatedBinding.UpdatedAt.Before(binding.UpdatedAt) {
		t.Error("UpdatedAt should not be before original binding time after modification")
	}
}
