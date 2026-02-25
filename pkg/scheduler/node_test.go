// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package scheduler

import (
	"sync"
	"testing"
	"time"
)

// =============================================================================
// Node Tests
// =============================================================================

func TestNewNode(t *testing.T) {
	capacity := Resources{CPU: 8000, Memory: 16384, Disk: 100000}
	node := NewNode("node-1", "test-node", capacity)

	if node.ID != "node-1" {
		t.Errorf("expected ID 'node-1', got '%s'", node.ID)
	}
	if node.Name != "test-node" {
		t.Errorf("expected Name 'test-node', got '%s'", node.Name)
	}
	if node.State != NodeStateReady {
		t.Errorf("expected State NodeStateReady, got '%s'", node.State)
	}
	if node.Capacity.CPU != 8000 {
		t.Errorf("expected Capacity.CPU 8000, got %d", node.Capacity.CPU)
	}
	if node.Allocatable.CPU != 8000 {
		t.Errorf("expected Allocatable.CPU 8000, got %d", node.Allocatable.CPU)
	}
	if node.Available.CPU != 8000 {
		t.Errorf("expected Available.CPU 8000, got %d", node.Available.CPU)
	}
	if node.Allocated.CPU != 0 {
		t.Errorf("expected Allocated.CPU 0, got %d", node.Allocated.CPU)
	}
	if node.Labels == nil {
		t.Error("expected Labels map to be initialized")
	}
	if node.Annotations == nil {
		t.Error("expected Annotations map to be initialized")
	}
}

func TestNodeAllocate(t *testing.T) {
	capacity := Resources{CPU: 8000, Memory: 16384, Disk: 100000}
	node := NewNode("node-1", "test-node", capacity)

	// Allocate some resources
	resources := Resources{CPU: 2000, Memory: 4096, Disk: 20000}
	err := node.Allocate(resources)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if node.Allocated.CPU != 2000 {
		t.Errorf("expected Allocated.CPU 2000, got %d", node.Allocated.CPU)
	}
	if node.Available.CPU != 6000 {
		t.Errorf("expected Available.CPU 6000, got %d", node.Available.CPU)
	}

	// Allocate more resources
	err = node.Allocate(resources)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if node.Allocated.CPU != 4000 {
		t.Errorf("expected Allocated.CPU 4000, got %d", node.Allocated.CPU)
	}
	if node.Available.CPU != 4000 {
		t.Errorf("expected Available.CPU 4000, got %d", node.Available.CPU)
	}
}

func TestNodeAllocateInsufficientResources(t *testing.T) {
	capacity := Resources{CPU: 4000, Memory: 8192, Disk: 50000}
	node := NewNode("node-1", "test-node", capacity)

	// Try to allocate more than available
	resources := Resources{CPU: 5000, Memory: 4096, Disk: 20000}
	err := node.Allocate(resources)
	if err != ErrInsufficientResources {
		t.Errorf("expected ErrInsufficientResources, got %v", err)
	}

	// Verify nothing was allocated
	if node.Allocated.CPU != 0 {
		t.Errorf("expected Allocated.CPU 0, got %d", node.Allocated.CPU)
	}
}

func TestNodeRelease(t *testing.T) {
	capacity := Resources{CPU: 8000, Memory: 16384, Disk: 100000}
	node := NewNode("node-1", "test-node", capacity)

	// Allocate resources first
	resources := Resources{CPU: 4000, Memory: 8192, Disk: 50000}
	_ = node.Allocate(resources)

	// Release some resources
	releaseResources := Resources{CPU: 2000, Memory: 4096, Disk: 25000}
	node.Release(releaseResources)

	if node.Allocated.CPU != 2000 {
		t.Errorf("expected Allocated.CPU 2000, got %d", node.Allocated.CPU)
	}
	if node.Available.CPU != 6000 {
		t.Errorf("expected Available.CPU 6000, got %d", node.Available.CPU)
	}
}

func TestNodeReleaseNoNegative(t *testing.T) {
	capacity := Resources{CPU: 8000, Memory: 16384, Disk: 100000}
	node := NewNode("node-1", "test-node", capacity)

	// Allocate small amount
	resources := Resources{CPU: 1000, Memory: 2048, Disk: 10000}
	_ = node.Allocate(resources)

	// Release more than allocated - should not go negative
	releaseResources := Resources{CPU: 5000, Memory: 10000, Disk: 50000}
	node.Release(releaseResources)

	if node.Allocated.CPU < 0 {
		t.Errorf("Allocated.CPU should not be negative, got %d", node.Allocated.CPU)
	}
	if node.Allocated.Memory < 0 {
		t.Errorf("Allocated.Memory should not be negative, got %d", node.Allocated.Memory)
	}
	if node.Allocated.Disk < 0 {
		t.Errorf("Allocated.Disk should not be negative, got %d", node.Allocated.Disk)
	}
}

func TestNodeIsReady(t *testing.T) {
	capacity := Resources{CPU: 8000, Memory: 16384, Disk: 100000}
	node := NewNode("node-1", "test-node", capacity)

	// New node should be ready
	if !node.IsReady() {
		t.Error("new node should be ready")
	}

	// Set condition to Ready=True, should still be ready
	node.SetCondition(NodeCondition{
		Type:   NodeConditionReady,
		Status: ConditionTrue,
	})
	if !node.IsReady() {
		t.Error("node with Ready=True should be ready")
	}

	// Set condition to Ready=False, should not be ready
	node.SetCondition(NodeCondition{
		Type:   NodeConditionReady,
		Status: ConditionFalse,
	})
	if node.IsReady() {
		t.Error("node with Ready=False should not be ready")
	}
}

func TestNodeIsReadyWithNotReadyState(t *testing.T) {
	capacity := Resources{CPU: 8000, Memory: 16384, Disk: 100000}
	node := NewNode("node-1", "test-node", capacity)

	// Change state to NotReady
	node.State = NodeStateNotReady
	if node.IsReady() {
		t.Error("node with NotReady state should not be ready")
	}

	// Change to Maintenance
	node.State = NodeStateMaintenance
	if node.IsReady() {
		t.Error("node in Maintenance should not be ready")
	}

	// Change to Draining
	node.State = NodeStateDraining
	if node.IsReady() {
		t.Error("node Draining should not be ready")
	}
}

func TestNodeTaints(t *testing.T) {
	capacity := Resources{CPU: 8000, Memory: 16384, Disk: 100000}
	node := NewNode("node-1", "test-node", capacity)

	// Initially no taints
	if node.HasTaint("gpu", TaintEffectNoSchedule) {
		t.Error("node should not have gpu taint initially")
	}

	// Add taint
	taint := Taint{
		Key:       "gpu",
		Value:     "true",
		Effect:    TaintEffectNoSchedule,
		TimeAdded: time.Now(),
	}
	node.AddTaint(taint)

	if !node.HasTaint("gpu", TaintEffectNoSchedule) {
		t.Error("node should have gpu taint")
	}

	// Add same taint again (should update)
	taint2 := Taint{
		Key:       "gpu",
		Value:     "v100",
		Effect:    TaintEffectNoSchedule,
		TimeAdded: time.Now(),
	}
	node.AddTaint(taint2)

	// Should still be only one taint
	if len(node.Taints) != 1 {
		t.Errorf("expected 1 taint, got %d", len(node.Taints))
	}
	if node.Taints[0].Value != "v100" {
		t.Errorf("expected taint value 'v100', got '%s'", node.Taints[0].Value)
	}

	// Remove taint
	removed := node.RemoveTaint("gpu", TaintEffectNoSchedule)
	if !removed {
		t.Error("taint should have been removed")
	}
	if node.HasTaint("gpu", TaintEffectNoSchedule) {
		t.Error("node should not have gpu taint after removal")
	}

	// Remove non-existent taint
	removed = node.RemoveTaint("nonexistent", TaintEffectNoSchedule)
	if removed {
		t.Error("removing non-existent taint should return false")
	}
}

func TestNodeConditions(t *testing.T) {
	capacity := Resources{CPU: 8000, Memory: 16384, Disk: 100000}
	node := NewNode("node-1", "test-node", capacity)

	// Initially no conditions
	cond := node.GetCondition(NodeConditionReady)
	if cond != nil {
		t.Error("should have no Ready condition initially")
	}

	// Set condition
	node.SetCondition(NodeCondition{
		Type:    NodeConditionReady,
		Status:  ConditionTrue,
		Reason:  "NodeReady",
		Message: "Node is ready",
	})

	cond = node.GetCondition(NodeConditionReady)
	if cond == nil {
		t.Fatal("should have Ready condition")
	}
	if cond.Status != ConditionTrue {
		t.Errorf("expected ConditionTrue, got %s", cond.Status)
	}

	// Update condition
	node.SetCondition(NodeCondition{
		Type:    NodeConditionReady,
		Status:  ConditionFalse,
		Reason:  "NotReady",
		Message: "Node is not ready",
	})

	cond = node.GetCondition(NodeConditionReady)
	if cond.Status != ConditionFalse {
		t.Errorf("expected ConditionFalse, got %s", cond.Status)
	}

	// Should still only have one condition
	if len(node.Conditions) != 1 {
		t.Errorf("expected 1 condition, got %d", len(node.Conditions))
	}
}

func TestNodeUtilization(t *testing.T) {
	capacity := Resources{CPU: 8000, Memory: 16384, Disk: 100000}
	node := NewNode("node-1", "test-node", capacity)

	// Initially 0 utilization
	if node.UtilizationRatio() != 0 {
		t.Errorf("expected 0 utilization, got %f", node.UtilizationRatio())
	}
	if node.CPUUtilization() != 0 {
		t.Errorf("expected 0 CPU utilization, got %f", node.CPUUtilization())
	}
	if node.MemoryUtilization() != 0 {
		t.Errorf("expected 0 memory utilization, got %f", node.MemoryUtilization())
	}

	// Allocate 50% of resources
	resources := Resources{CPU: 4000, Memory: 8192, Disk: 50000}
	_ = node.Allocate(resources)

	cpuUtil := node.CPUUtilization()
	if cpuUtil != 0.5 {
		t.Errorf("expected 0.5 CPU utilization, got %f", cpuUtil)
	}

	memUtil := node.MemoryUtilization()
	if memUtil != 0.5 {
		t.Errorf("expected 0.5 memory utilization, got %f", memUtil)
	}

	overallUtil := node.UtilizationRatio()
	if overallUtil != 0.5 {
		t.Errorf("expected 0.5 overall utilization, got %f", overallUtil)
	}
}

func TestNodeUtilizationZeroCapacity(t *testing.T) {
	capacity := Resources{CPU: 0, Memory: 0, Disk: 0}
	node := NewNode("node-1", "test-node", capacity)

	// Should handle zero capacity without division by zero
	if node.UtilizationRatio() != 0 {
		t.Errorf("expected 0 utilization for zero capacity, got %f", node.UtilizationRatio())
	}
	if node.CPUUtilization() != 0 {
		t.Errorf("expected 0 CPU utilization for zero capacity, got %f", node.CPUUtilization())
	}
	if node.MemoryUtilization() != 0 {
		t.Errorf("expected 0 memory utilization for zero capacity, got %f", node.MemoryUtilization())
	}
}

// =============================================================================
// Toleration Tests
// =============================================================================

func TestTolerationMatches(t *testing.T) {
	tests := []struct {
		name       string
		toleration Toleration
		taint      Taint
		expected   bool
	}{
		{
			name: "exact match with Equal operator",
			toleration: Toleration{
				Key:      "gpu",
				Operator: TolerationOpEqual,
				Value:    "true",
				Effect:   TaintEffectNoSchedule,
			},
			taint: Taint{
				Key:    "gpu",
				Value:  "true",
				Effect: TaintEffectNoSchedule,
			},
			expected: true,
		},
		{
			name: "key match with Exists operator",
			toleration: Toleration{
				Key:      "gpu",
				Operator: TolerationOpExists,
				Effect:   TaintEffectNoSchedule,
			},
			taint: Taint{
				Key:    "gpu",
				Value:  "any-value",
				Effect: TaintEffectNoSchedule,
			},
			expected: true,
		},
		{
			name: "wildcard toleration (empty key with Exists)",
			toleration: Toleration{
				Key:      "",
				Operator: TolerationOpExists,
			},
			taint: Taint{
				Key:    "any-key",
				Value:  "any-value",
				Effect: TaintEffectNoSchedule,
			},
			expected: true,
		},
		{
			name: "key mismatch",
			toleration: Toleration{
				Key:      "different-key",
				Operator: TolerationOpEqual,
				Value:    "true",
				Effect:   TaintEffectNoSchedule,
			},
			taint: Taint{
				Key:    "gpu",
				Value:  "true",
				Effect: TaintEffectNoSchedule,
			},
			expected: false,
		},
		{
			name: "value mismatch with Equal operator",
			toleration: Toleration{
				Key:      "gpu",
				Operator: TolerationOpEqual,
				Value:    "false",
				Effect:   TaintEffectNoSchedule,
			},
			taint: Taint{
				Key:    "gpu",
				Value:  "true",
				Effect: TaintEffectNoSchedule,
			},
			expected: false,
		},
		{
			name: "effect mismatch",
			toleration: Toleration{
				Key:      "gpu",
				Operator: TolerationOpEqual,
				Value:    "true",
				Effect:   TaintEffectNoExecute,
			},
			taint: Taint{
				Key:    "gpu",
				Value:  "true",
				Effect: TaintEffectNoSchedule,
			},
			expected: false,
		},
		{
			name: "empty effect tolerates any effect",
			toleration: Toleration{
				Key:      "gpu",
				Operator: TolerationOpEqual,
				Value:    "true",
				Effect:   "",
			},
			taint: Taint{
				Key:    "gpu",
				Value:  "true",
				Effect: TaintEffectNoSchedule,
			},
			expected: true,
		},
		{
			name: "default operator (Equal) with value mismatch",
			toleration: Toleration{
				Key:    "gpu",
				Value:  "different",
				Effect: TaintEffectNoSchedule,
			},
			taint: Taint{
				Key:    "gpu",
				Value:  "true",
				Effect: TaintEffectNoSchedule,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.toleration.Matches(tt.taint)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// =============================================================================
// NodeRegistry Tests
// =============================================================================

func TestNewNodeRegistry(t *testing.T) {
	registry := NewNodeRegistry()

	if registry.Count() != 0 {
		t.Errorf("expected 0 nodes, got %d", registry.Count())
	}
}

func TestNodeRegistryRegister(t *testing.T) {
	registry := NewNodeRegistry()

	capacity := Resources{CPU: 8000, Memory: 16384, Disk: 100000}
	node := NewNode("node-1", "test-node", capacity)

	err := registry.Register(node)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if registry.Count() != 1 {
		t.Errorf("expected 1 node, got %d", registry.Count())
	}
}

func TestNodeRegistryRegisterDuplicate(t *testing.T) {
	registry := NewNodeRegistry()

	capacity := Resources{CPU: 8000, Memory: 16384, Disk: 100000}
	node := NewNode("node-1", "test-node", capacity)

	_ = registry.Register(node)

	// Register again with same ID
	err := registry.Register(node)
	if err == nil {
		t.Error("expected error for duplicate registration")
	}
}

func TestNodeRegistryRegisterEmptyID(t *testing.T) {
	registry := NewNodeRegistry()

	node := &Node{
		ID:   "",
		Name: "test-node",
	}

	err := registry.Register(node)
	if err == nil {
		t.Error("expected error for empty node ID")
	}
}

func TestNodeRegistryUnregister(t *testing.T) {
	registry := NewNodeRegistry()

	capacity := Resources{CPU: 8000, Memory: 16384, Disk: 100000}
	node := NewNode("node-1", "test-node", capacity)
	_ = registry.Register(node)

	err := registry.Unregister("node-1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if registry.Count() != 0 {
		t.Errorf("expected 0 nodes, got %d", registry.Count())
	}
}

func TestNodeRegistryUnregisterNotFound(t *testing.T) {
	registry := NewNodeRegistry()

	err := registry.Unregister("nonexistent")
	if err != ErrNodeNotFound {
		t.Errorf("expected ErrNodeNotFound, got %v", err)
	}
}

func TestNodeRegistryGet(t *testing.T) {
	registry := NewNodeRegistry()

	capacity := Resources{CPU: 8000, Memory: 16384, Disk: 100000}
	node := NewNode("node-1", "test-node", capacity)
	_ = registry.Register(node)

	retrieved, err := registry.Get("node-1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if retrieved.ID != "node-1" {
		t.Errorf("expected node ID 'node-1', got '%s'", retrieved.ID)
	}
}

func TestNodeRegistryGetNotFound(t *testing.T) {
	registry := NewNodeRegistry()

	_, err := registry.Get("nonexistent")
	if err != ErrNodeNotFound {
		t.Errorf("expected ErrNodeNotFound, got %v", err)
	}
}

func TestNodeRegistryUpdate(t *testing.T) {
	registry := NewNodeRegistry()

	capacity := Resources{CPU: 8000, Memory: 16384, Disk: 100000}
	node := NewNode("node-1", "test-node", capacity)
	_ = registry.Register(node)

	// Update node
	node.State = NodeStateNotReady
	err := registry.Update(node)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	retrieved, _ := registry.Get("node-1")
	if retrieved.State != NodeStateNotReady {
		t.Errorf("expected state NotReady, got %s", retrieved.State)
	}
}

func TestNodeRegistryUpdateNotFound(t *testing.T) {
	registry := NewNodeRegistry()

	node := &Node{ID: "nonexistent"}
	err := registry.Update(node)
	if err != ErrNodeNotFound {
		t.Errorf("expected ErrNodeNotFound, got %v", err)
	}
}

func TestNodeRegistryList(t *testing.T) {
	registry := NewNodeRegistry()

	capacity := Resources{CPU: 8000, Memory: 16384, Disk: 100000}
	for i := 0; i < 5; i++ {
		node := NewNode(string(rune('a'+i)), "node", capacity)
		_ = registry.Register(node)
	}

	nodes := registry.List()
	if len(nodes) != 5 {
		t.Errorf("expected 5 nodes, got %d", len(nodes))
	}
}

func TestNodeRegistryListReady(t *testing.T) {
	registry := NewNodeRegistry()

	capacity := Resources{CPU: 8000, Memory: 16384, Disk: 100000}

	// Create 3 ready nodes
	for i := 0; i < 3; i++ {
		node := NewNode(string(rune('a'+i)), "ready-node", capacity)
		_ = registry.Register(node)
	}

	// Create 2 not-ready nodes
	for i := 3; i < 5; i++ {
		node := NewNode(string(rune('a'+i)), "not-ready-node", capacity)
		node.State = NodeStateNotReady
		_ = registry.Register(node)
	}

	readyNodes := registry.ListReady()
	if len(readyNodes) != 3 {
		t.Errorf("expected 3 ready nodes, got %d", len(readyNodes))
	}
}

func TestNodeRegistryListByLabel(t *testing.T) {
	registry := NewNodeRegistry()

	capacity := Resources{CPU: 8000, Memory: 16384, Disk: 100000}

	// Create nodes with different labels
	for i := 0; i < 3; i++ {
		node := NewNode(string(rune('a'+i)), "gpu-node", capacity)
		node.Labels["gpu"] = "true"
		_ = registry.Register(node)
	}
	for i := 3; i < 5; i++ {
		node := NewNode(string(rune('a'+i)), "cpu-node", capacity)
		node.Labels["gpu"] = "false"
		_ = registry.Register(node)
	}

	gpuNodes := registry.ListByLabel("gpu", "true")
	if len(gpuNodes) != 3 {
		t.Errorf("expected 3 gpu nodes, got %d", len(gpuNodes))
	}
}

func TestNodeRegistryListByZone(t *testing.T) {
	registry := NewNodeRegistry()

	capacity := Resources{CPU: 8000, Memory: 16384, Disk: 100000}

	// Create nodes in different zones
	for i := 0; i < 3; i++ {
		node := NewNode(string(rune('a'+i)), "zone-a-node", capacity)
		node.Zone = "zone-a"
		_ = registry.Register(node)
	}
	for i := 3; i < 5; i++ {
		node := NewNode(string(rune('a'+i)), "zone-b-node", capacity)
		node.Zone = "zone-b"
		_ = registry.Register(node)
	}

	zoneANodes := registry.ListByZone("zone-a")
	if len(zoneANodes) != 3 {
		t.Errorf("expected 3 zone-a nodes, got %d", len(zoneANodes))
	}
}

func TestNodeRegistryCountReady(t *testing.T) {
	registry := NewNodeRegistry()

	capacity := Resources{CPU: 8000, Memory: 16384, Disk: 100000}

	// Create 3 ready nodes
	for i := 0; i < 3; i++ {
		node := NewNode(string(rune('a'+i)), "ready-node", capacity)
		_ = registry.Register(node)
	}

	// Create 2 not-ready nodes
	for i := 3; i < 5; i++ {
		node := NewNode(string(rune('a'+i)), "not-ready-node", capacity)
		node.State = NodeStateNotReady
		_ = registry.Register(node)
	}

	if registry.CountReady() != 3 {
		t.Errorf("expected 3 ready nodes, got %d", registry.CountReady())
	}
}

func TestNodeRegistryGetTotalCapacity(t *testing.T) {
	registry := NewNodeRegistry()

	capacity := Resources{CPU: 8000, Memory: 16384, Disk: 100000}

	for i := 0; i < 3; i++ {
		node := NewNode(string(rune('a'+i)), "node", capacity)
		_ = registry.Register(node)
	}

	total := registry.GetTotalCapacity()
	if total.CPU != 24000 {
		t.Errorf("expected total CPU 24000, got %d", total.CPU)
	}
	if total.Memory != 49152 {
		t.Errorf("expected total Memory 49152, got %d", total.Memory)
	}
}

func TestNodeRegistryGetTotalAllocated(t *testing.T) {
	registry := NewNodeRegistry()

	capacity := Resources{CPU: 8000, Memory: 16384, Disk: 100000}

	for i := 0; i < 3; i++ {
		node := NewNode(string(rune('a'+i)), "node", capacity)
		_ = node.Allocate(Resources{CPU: 2000, Memory: 4096, Disk: 20000})
		_ = registry.Register(node)
	}

	total := registry.GetTotalAllocated()
	if total.CPU != 6000 {
		t.Errorf("expected total allocated CPU 6000, got %d", total.CPU)
	}
}

func TestNodeRegistryGetTotalAvailable(t *testing.T) {
	registry := NewNodeRegistry()

	capacity := Resources{CPU: 8000, Memory: 16384, Disk: 100000}

	for i := 0; i < 3; i++ {
		node := NewNode(string(rune('a'+i)), "node", capacity)
		_ = node.Allocate(Resources{CPU: 2000, Memory: 4096, Disk: 20000})
		_ = registry.Register(node)
	}

	total := registry.GetTotalAvailable()
	if total.CPU != 18000 {
		t.Errorf("expected total available CPU 18000, got %d", total.CPU)
	}
}

func TestNodeRegistryUpdateHeartbeat(t *testing.T) {
	registry := NewNodeRegistry()

	capacity := Resources{CPU: 8000, Memory: 16384, Disk: 100000}
	node := NewNode("node-1", "test-node", capacity)
	_ = registry.Register(node)

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	err := registry.UpdateHeartbeat("node-1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	retrieved, _ := registry.Get("node-1")
	if retrieved.LastHeartbeat.Before(node.CreatedAt) {
		t.Error("heartbeat should have been updated")
	}
}

func TestNodeRegistryUpdateHeartbeatNotFound(t *testing.T) {
	registry := NewNodeRegistry()

	err := registry.UpdateHeartbeat("nonexistent")
	if err != ErrNodeNotFound {
		t.Errorf("expected ErrNodeNotFound, got %v", err)
	}
}

func TestNodeRegistryMarkNotReady(t *testing.T) {
	registry := NewNodeRegistry()

	capacity := Resources{CPU: 8000, Memory: 16384, Disk: 100000}

	// Create node with old heartbeat
	node := NewNode("node-1", "test-node", capacity)
	node.LastHeartbeat = time.Now().Add(-10 * time.Minute)
	_ = registry.Register(node)

	// Create node with recent heartbeat
	node2 := NewNode("node-2", "test-node-2", capacity)
	_ = registry.Register(node2)

	// Mark nodes not ready if heartbeat older than 5 minutes
	marked := registry.MarkNotReady(5 * time.Minute)

	if len(marked) != 1 {
		t.Errorf("expected 1 marked node, got %d", len(marked))
	}
	if marked[0] != "node-1" {
		t.Errorf("expected node-1 to be marked, got %s", marked[0])
	}

	retrieved, _ := registry.Get("node-1")
	if retrieved.State != NodeStateNotReady {
		t.Errorf("expected state NotReady, got %s", retrieved.State)
	}
}

func TestNodeRegistryConcurrentAccess(t *testing.T) {
	registry := NewNodeRegistry()

	var wg sync.WaitGroup
	numGoroutines := 100

	// Concurrent registration
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			capacity := Resources{CPU: 8000, Memory: 16384, Disk: 100000}
			node := NewNode(string(rune('a'+id%26))+string(rune('a'+id/26)), "node", capacity)
			_ = registry.Register(node)
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = registry.List()
			_ = registry.ListReady()
			_ = registry.Count()
		}()
	}

	wg.Wait()

	// Verify some operations succeeded (not all will due to duplicates)
	if registry.Count() == 0 {
		t.Error("expected some nodes to be registered")
	}
}

// =============================================================================
// NodeScorer Tests
// =============================================================================

func TestNewNodeScorer(t *testing.T) {
	scorer := NewNodeScorer()

	if scorer.weights["cpu"] != 1.0 {
		t.Errorf("expected cpu weight 1.0, got %f", scorer.weights["cpu"])
	}
	if scorer.weights["memory"] != 1.0 {
		t.Errorf("expected memory weight 1.0, got %f", scorer.weights["memory"])
	}
	if scorer.weights["spread"] != 1.0 {
		t.Errorf("expected spread weight 1.0, got %f", scorer.weights["spread"])
	}
}

func TestNodeScorerSetWeight(t *testing.T) {
	scorer := NewNodeScorer()

	scorer.SetWeight("cpu", 2.0)
	if scorer.weights["cpu"] != 2.0 {
		t.Errorf("expected cpu weight 2.0, got %f", scorer.weights["cpu"])
	}

	scorer.SetWeight("custom", 0.5)
	if scorer.weights["custom"] != 0.5 {
		t.Errorf("expected custom weight 0.5, got %f", scorer.weights["custom"])
	}
}

func TestNodeScorerScoreNodes(t *testing.T) {
	scorer := NewNodeScorer()

	workload := &Workload{
		ID: "workload-1",
		Resources: ResourceRequirements{
			Requests: Resources{CPU: 1000, Memory: 2048, Disk: 10000},
		},
	}

	capacity := Resources{CPU: 8000, Memory: 16384, Disk: 100000}
	nodes := []*Node{
		NewNode("node-1", "node-1", capacity),
		NewNode("node-2", "node-2", capacity),
	}

	// Allocate some resources on node-1 to make it less desirable for spread
	_ = nodes[0].Allocate(Resources{CPU: 4000, Memory: 8192, Disk: 50000})

	scores := scorer.ScoreNodes(workload, nodes)

	if len(scores) != 2 {
		t.Errorf("expected 2 scores, got %d", len(scores))
	}

	// Node-2 should have higher score (more available resources)
	if scores["node-2"] <= scores["node-1"] {
		t.Errorf("expected node-2 to have higher score than node-1")
	}
}

func TestNodeScorerScoreNodesEmpty(t *testing.T) {
	scorer := NewNodeScorer()

	workload := &Workload{
		ID: "workload-1",
		Resources: ResourceRequirements{
			Requests: Resources{CPU: 1000, Memory: 2048, Disk: 10000},
		},
	}

	scores := scorer.ScoreNodes(workload, []*Node{})

	if len(scores) != 0 {
		t.Errorf("expected 0 scores, got %d", len(scores))
	}
}

func TestNodeScorerScoreNodesWithWeights(t *testing.T) {
	scorer := NewNodeScorer()
	scorer.SetWeight("cpu", 2.0)
	scorer.SetWeight("memory", 0.5)
	scorer.SetWeight("spread", 0.0)

	workload := &Workload{
		ID: "workload-1",
		Resources: ResourceRequirements{
			Requests: Resources{CPU: 1000, Memory: 2048, Disk: 10000},
		},
	}

	// Node with more CPU available
	nodeMoreCPU := NewNode("node-cpu", "node-cpu", Resources{CPU: 16000, Memory: 8192, Disk: 100000})

	// Node with more memory available
	nodeMoreMem := NewNode("node-mem", "node-mem", Resources{CPU: 4000, Memory: 32768, Disk: 100000})

	nodes := []*Node{nodeMoreCPU, nodeMoreMem}

	scores := scorer.ScoreNodes(workload, nodes)

	// With higher CPU weight, node-cpu should score higher
	if scores["node-cpu"] <= scores["node-mem"] {
		t.Errorf("expected node-cpu to have higher score with CPU weight=2.0")
	}
}

// =============================================================================
// NodePool Tests
// =============================================================================

func TestNewNodePoolManager(t *testing.T) {
	manager := NewNodePoolManager()

	pools := manager.ListPools()
	if len(pools) != 0 {
		t.Errorf("expected 0 pools, got %d", len(pools))
	}
}

func TestNodePoolManagerCreatePool(t *testing.T) {
	manager := NewNodePoolManager()

	pool := &NodePool{
		Name:     "gpu-pool",
		Labels:   map[string]string{"gpu": "true"},
		MinNodes: 1,
		MaxNodes: 10,
	}

	err := manager.CreatePool(pool)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	pools := manager.ListPools()
	if len(pools) != 1 {
		t.Errorf("expected 1 pool, got %d", len(pools))
	}
}

func TestNodePoolManagerCreatePoolDuplicate(t *testing.T) {
	manager := NewNodePoolManager()

	pool := &NodePool{Name: "gpu-pool"}
	_ = manager.CreatePool(pool)

	err := manager.CreatePool(pool)
	if err == nil {
		t.Error("expected error for duplicate pool")
	}
}

func TestNodePoolManagerDeletePool(t *testing.T) {
	manager := NewNodePoolManager()

	pool := &NodePool{Name: "gpu-pool"}
	_ = manager.CreatePool(pool)

	err := manager.DeletePool("gpu-pool")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	pools := manager.ListPools()
	if len(pools) != 0 {
		t.Errorf("expected 0 pools, got %d", len(pools))
	}
}

func TestNodePoolManagerDeletePoolNotFound(t *testing.T) {
	manager := NewNodePoolManager()

	err := manager.DeletePool("nonexistent")
	if err == nil {
		t.Error("expected error for non-existent pool")
	}
}

func TestNodePoolManagerGetPool(t *testing.T) {
	manager := NewNodePoolManager()

	pool := &NodePool{
		Name:     "gpu-pool",
		Labels:   map[string]string{"gpu": "true"},
		MinNodes: 1,
		MaxNodes: 10,
	}
	_ = manager.CreatePool(pool)

	retrieved, err := manager.GetPool("gpu-pool")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if retrieved.Name != "gpu-pool" {
		t.Errorf("expected pool name 'gpu-pool', got '%s'", retrieved.Name)
	}
	if retrieved.MinNodes != 1 {
		t.Errorf("expected MinNodes 1, got %d", retrieved.MinNodes)
	}
}

func TestNodePoolManagerGetPoolNotFound(t *testing.T) {
	manager := NewNodePoolManager()

	_, err := manager.GetPool("nonexistent")
	if err == nil {
		t.Error("expected error for non-existent pool")
	}
}

func TestNodePoolManagerAddNodeToPool(t *testing.T) {
	manager := NewNodePoolManager()

	pool := &NodePool{Name: "gpu-pool"}
	_ = manager.CreatePool(pool)

	err := manager.AddNodeToPool("gpu-pool", "node-1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	retrieved, _ := manager.GetPool("gpu-pool")
	if len(retrieved.NodeIDs) != 1 {
		t.Errorf("expected 1 node in pool, got %d", len(retrieved.NodeIDs))
	}
	if retrieved.NodeIDs[0] != "node-1" {
		t.Errorf("expected node-1, got %s", retrieved.NodeIDs[0])
	}
}

func TestNodePoolManagerAddNodeToPoolDuplicate(t *testing.T) {
	manager := NewNodePoolManager()

	pool := &NodePool{Name: "gpu-pool"}
	_ = manager.CreatePool(pool)

	_ = manager.AddNodeToPool("gpu-pool", "node-1")
	err := manager.AddNodeToPool("gpu-pool", "node-1")
	if err == nil {
		t.Error("expected error for duplicate node in pool")
	}
}

func TestNodePoolManagerAddNodeToPoolNotFound(t *testing.T) {
	manager := NewNodePoolManager()

	err := manager.AddNodeToPool("nonexistent", "node-1")
	if err == nil {
		t.Error("expected error for non-existent pool")
	}
}

func TestNodePoolManagerRemoveNodeFromPool(t *testing.T) {
	manager := NewNodePoolManager()

	pool := &NodePool{Name: "gpu-pool"}
	_ = manager.CreatePool(pool)
	_ = manager.AddNodeToPool("gpu-pool", "node-1")
	_ = manager.AddNodeToPool("gpu-pool", "node-2")

	err := manager.RemoveNodeFromPool("gpu-pool", "node-1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	retrieved, _ := manager.GetPool("gpu-pool")
	if len(retrieved.NodeIDs) != 1 {
		t.Errorf("expected 1 node in pool, got %d", len(retrieved.NodeIDs))
	}
	if retrieved.NodeIDs[0] != "node-2" {
		t.Errorf("expected node-2, got %s", retrieved.NodeIDs[0])
	}
}

func TestNodePoolManagerRemoveNodeFromPoolNotFound(t *testing.T) {
	manager := NewNodePoolManager()

	pool := &NodePool{Name: "gpu-pool"}
	_ = manager.CreatePool(pool)

	err := manager.RemoveNodeFromPool("gpu-pool", "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent node")
	}
}

func TestNodePoolManagerRemoveNodeFromPoolPoolNotFound(t *testing.T) {
	manager := NewNodePoolManager()

	err := manager.RemoveNodeFromPool("nonexistent", "node-1")
	if err == nil {
		t.Error("expected error for non-existent pool")
	}
}

func TestNodePoolManagerConcurrentAccess(t *testing.T) {
	manager := NewNodePoolManager()

	var wg sync.WaitGroup
	numGoroutines := 50

	// Create initial pool
	_ = manager.CreatePool(&NodePool{Name: "test-pool"})

	// Concurrent adds and removes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			nodeID := string(rune('a' + id%26))
			_ = manager.AddNodeToPool("test-pool", nodeID)
			_ = manager.RemoveNodeFromPool("test-pool", nodeID)
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = manager.GetPool("test-pool")
			_ = manager.ListPools()
		}()
	}

	wg.Wait()
}

// =============================================================================
// CreateSnapshot Tests
// =============================================================================

func TestCreateSnapshot(t *testing.T) {
	capacity := Resources{CPU: 8000, Memory: 16384, Disk: 100000}
	node := NewNode("node-1", "test-node", capacity)
	_ = node.Allocate(Resources{CPU: 4000, Memory: 8192, Disk: 50000})

	workloadIDs := []string{"workload-1", "workload-2", "workload-3"}

	snapshot := CreateSnapshot(node, workloadIDs)

	if snapshot.NodeID != "node-1" {
		t.Errorf("expected NodeID 'node-1', got '%s'", snapshot.NodeID)
	}
	if snapshot.State != NodeStateReady {
		t.Errorf("expected state Ready, got %s", snapshot.State)
	}
	if snapshot.Allocated.CPU != 4000 {
		t.Errorf("expected Allocated.CPU 4000, got %d", snapshot.Allocated.CPU)
	}
	if snapshot.Available.CPU != 4000 {
		t.Errorf("expected Available.CPU 4000, got %d", snapshot.Available.CPU)
	}
	if snapshot.Utilization != 0.5 {
		t.Errorf("expected Utilization 0.5, got %f", snapshot.Utilization)
	}
	if len(snapshot.WorkloadIDs) != 3 {
		t.Errorf("expected 3 workload IDs, got %d", len(snapshot.WorkloadIDs))
	}
}

// =============================================================================
// Edge Case Tests
// =============================================================================

func TestNodeWithGPUsAndEphemeral(t *testing.T) {
	capacity := Resources{
		CPU:       8000,
		Memory:    16384,
		Disk:      100000,
		GPUs:      4,
		Ephemeral: 50000,
	}
	node := NewNode("node-1", "gpu-node", capacity)

	// Allocate GPU resources
	resources := Resources{
		CPU:       2000,
		Memory:    4096,
		Disk:      20000,
		GPUs:      2,
		Ephemeral: 10000,
	}
	err := node.Allocate(resources)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if node.Allocated.GPUs != 2 {
		t.Errorf("expected Allocated.GPUs 2, got %d", node.Allocated.GPUs)
	}
	if node.Available.GPUs != 2 {
		t.Errorf("expected Available.GPUs 2, got %d", node.Available.GPUs)
	}
	if node.Allocated.Ephemeral != 10000 {
		t.Errorf("expected Allocated.Ephemeral 10000, got %d", node.Allocated.Ephemeral)
	}
}

func TestNodeReleaseWithGPUsAndEphemeral(t *testing.T) {
	capacity := Resources{
		CPU:       8000,
		Memory:    16384,
		Disk:      100000,
		GPUs:      4,
		Ephemeral: 50000,
	}
	node := NewNode("node-1", "gpu-node", capacity)

	// Allocate and release
	resources := Resources{
		CPU:       2000,
		Memory:    4096,
		Disk:      20000,
		GPUs:      2,
		Ephemeral: 10000,
	}
	_ = node.Allocate(resources)
	node.Release(resources)

	if node.Allocated.GPUs != 0 {
		t.Errorf("expected Allocated.GPUs 0, got %d", node.Allocated.GPUs)
	}
	if node.Allocated.Ephemeral != 0 {
		t.Errorf("expected Allocated.Ephemeral 0, got %d", node.Allocated.Ephemeral)
	}

	// Test releasing more than allocated (shouldn't go negative)
	node.Release(resources)
	if node.Allocated.GPUs < 0 {
		t.Errorf("Allocated.GPUs should not be negative, got %d", node.Allocated.GPUs)
	}
	if node.Allocated.Ephemeral < 0 {
		t.Errorf("Allocated.Ephemeral should not be negative, got %d", node.Allocated.Ephemeral)
	}
}

func TestNodeMultipleTaints(t *testing.T) {
	capacity := Resources{CPU: 8000, Memory: 16384, Disk: 100000}
	node := NewNode("node-1", "test-node", capacity)

	// Add multiple taints
	node.AddTaint(Taint{Key: "gpu", Value: "true", Effect: TaintEffectNoSchedule})
	node.AddTaint(Taint{Key: "dedicated", Value: "ml", Effect: TaintEffectNoSchedule})
	node.AddTaint(Taint{Key: "maintenance", Effect: TaintEffectNoExecute})

	if len(node.Taints) != 3 {
		t.Errorf("expected 3 taints, got %d", len(node.Taints))
	}

	if !node.HasTaint("gpu", TaintEffectNoSchedule) {
		t.Error("expected gpu taint")
	}
	if !node.HasTaint("dedicated", TaintEffectNoSchedule) {
		t.Error("expected dedicated taint")
	}
	if !node.HasTaint("maintenance", TaintEffectNoExecute) {
		t.Error("expected maintenance taint")
	}

	// Check for non-existent taint
	if node.HasTaint("nonexistent", TaintEffectNoSchedule) {
		t.Error("should not have nonexistent taint")
	}

	// Check for taint with wrong effect
	if node.HasTaint("gpu", TaintEffectNoExecute) {
		t.Error("should not have gpu taint with NoExecute effect")
	}
}

func TestNodeMultipleConditions(t *testing.T) {
	capacity := Resources{CPU: 8000, Memory: 16384, Disk: 100000}
	node := NewNode("node-1", "test-node", capacity)

	// Set multiple conditions
	node.SetCondition(NodeCondition{
		Type:    NodeConditionReady,
		Status:  ConditionTrue,
		Reason:  "Ready",
		Message: "Node is ready",
	})
	node.SetCondition(NodeCondition{
		Type:    NodeConditionMemoryPressure,
		Status:  ConditionFalse,
		Reason:  "NoMemoryPressure",
		Message: "Memory is available",
	})
	node.SetCondition(NodeCondition{
		Type:    NodeConditionDiskPressure,
		Status:  ConditionFalse,
		Reason:  "NoDiskPressure",
		Message: "Disk space is available",
	})

	if len(node.Conditions) != 3 {
		t.Errorf("expected 3 conditions, got %d", len(node.Conditions))
	}

	readyCond := node.GetCondition(NodeConditionReady)
	if readyCond == nil || readyCond.Status != ConditionTrue {
		t.Error("expected Ready condition with True status")
	}

	memCond := node.GetCondition(NodeConditionMemoryPressure)
	if memCond == nil || memCond.Status != ConditionFalse {
		t.Error("expected MemoryPressure condition with False status")
	}

	// Non-existent condition
	pidCond := node.GetCondition(NodeConditionPIDPressure)
	if pidCond != nil {
		t.Error("should not have PIDPressure condition")
	}
}

func TestNodeRegistryLargeScale(t *testing.T) {
	registry := NewNodeRegistry()
	numNodes := 1000

	capacity := Resources{CPU: 8000, Memory: 16384, Disk: 100000}

	// Register many nodes
	for i := 0; i < numNodes; i++ {
		node := NewNode(string(rune(i)), "node", capacity)
		node.Zone = "zone-" + string(rune(i%3))
		if i%2 == 0 {
			node.Labels["type"] = "compute"
		} else {
			node.Labels["type"] = "storage"
		}
		_ = registry.Register(node)
	}

	if registry.Count() != numNodes {
		t.Errorf("expected %d nodes, got %d", numNodes, registry.Count())
	}

	// Test list operations
	all := registry.List()
	if len(all) != numNodes {
		t.Errorf("expected %d nodes in list, got %d", numNodes, len(all))
	}

	// Test label filtering
	computeNodes := registry.ListByLabel("type", "compute")
	if len(computeNodes) != numNodes/2 {
		t.Errorf("expected %d compute nodes, got %d", numNodes/2, len(computeNodes))
	}
}
