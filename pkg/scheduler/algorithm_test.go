// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package scheduler

import (
	"sync"
	"testing"
	"time"
)

// =============================================================================
// Helper Functions for Algorithm Tests
// =============================================================================

func createTestNode(id string, cpu, mem int64) *Node {
	return &Node{
		ID:          id,
		Name:        id,
		State:       NodeStateReady,
		Capacity:    Resources{CPU: cpu, Memory: mem},
		Allocatable: Resources{CPU: cpu, Memory: mem},
		Available:   Resources{CPU: cpu, Memory: mem},
		Allocated:   Resources{},
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
		CreatedAt:   time.Now(),
	}
}

func createTestWorkload(id string, cpu, mem int64) *Workload {
	return &Workload{
		ID:        id,
		Name:      id,
		Namespace: "default",
		Priority:  50,
		Resources: ResourceRequirements{
			Requests: Resources{CPU: cpu, Memory: mem},
		},
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
		State:       WorkloadStatePending,
		CreatedAt:   time.Now(),
	}
}

// =============================================================================
// BinPack Algorithm Tests
// =============================================================================

func TestBinPackAlgorithm_Name(t *testing.T) {
	algo := NewBinPackAlgorithm()
	if algo.Name() != "BinPack" {
		t.Errorf("expected name BinPack, got %s", algo.Name())
	}
}

func TestBinPackAlgorithm_SelectNode_PrefersHighUtilization(t *testing.T) {
	algo := NewBinPackAlgorithm()

	// Node 1: 80% utilized
	node1 := createTestNode("node-1", 1000, 1000*1024*1024)
	node1.Allocated = Resources{CPU: 800, Memory: 800 * 1024 * 1024}
	node1.UpdateAvailable()

	// Node 2: 20% utilized
	node2 := createTestNode("node-2", 1000, 1000*1024*1024)
	node2.Allocated = Resources{CPU: 200, Memory: 200 * 1024 * 1024}
	node2.UpdateAvailable()

	workload := createTestWorkload("w1", 100, 100*1024*1024)

	selected := algo.SelectNode(workload, []*Node{node1, node2})

	// BinPack should prefer node1 (higher utilization)
	if selected == nil {
		t.Fatal("expected a node to be selected")
	}
	if selected.ID != "node-1" {
		t.Errorf("expected node-1 (higher utilization), got %s", selected.ID)
	}
}

func TestBinPackAlgorithm_SelectNode_NoFeasibleNodes(t *testing.T) {
	algo := NewBinPackAlgorithm()

	// Node with insufficient resources
	node := createTestNode("node-1", 100, 100*1024*1024)
	node.Available = Resources{CPU: 50, Memory: 50 * 1024 * 1024}

	// Workload requires more resources
	workload := createTestWorkload("w1", 200, 200*1024*1024)

	selected := algo.SelectNode(workload, []*Node{node})

	if selected != nil {
		t.Error("expected nil when no feasible nodes")
	}
}

func TestBinPackAlgorithm_ScoreNodes(t *testing.T) {
	algo := NewBinPackAlgorithm()

	node1 := createTestNode("node-1", 1000, 1000*1024*1024)
	node1.Allocated = Resources{CPU: 500, Memory: 500 * 1024 * 1024}
	node1.UpdateAvailable()

	node2 := createTestNode("node-2", 1000, 1000*1024*1024)
	node2.Allocated = Resources{CPU: 200, Memory: 200 * 1024 * 1024}
	node2.UpdateAvailable()

	workload := createTestWorkload("w1", 100, 100*1024*1024)

	scores := algo.ScoreNodes(workload, []*Node{node1, node2})

	if len(scores) != 2 {
		t.Fatalf("expected 2 scores, got %d", len(scores))
	}

	// Find scores by node
	var score1, score2 int64
	for _, s := range scores {
		switch s.Node.ID {
		case "node-1":
			score1 = s.Score
		case "node-2":
			score2 = s.Score
		}
	}

	// node-1 should have higher score (higher utilization after scheduling)
	if score1 <= score2 {
		t.Errorf("expected node-1 score (%d) > node-2 score (%d)", score1, score2)
	}
}

func TestBinPackAlgorithm_SetWeights(t *testing.T) {
	algo := NewBinPackAlgorithm()

	// Set custom weights
	algo.SetWeights(2.0, 1.0, 0.0)

	node := createTestNode("node-1", 1000, 1000*1024*1024)
	workload := createTestWorkload("w1", 100, 100*1024*1024)

	scores := algo.ScoreNodes(workload, []*Node{node})
	if len(scores) != 1 {
		t.Errorf("expected 1 score, got %d", len(scores))
	}
}

func TestBinPackAlgorithm_DiskWeight(t *testing.T) {
	algo := NewBinPackAlgorithm()
	algo.SetWeights(1.0, 1.0, 1.0)

	node := createTestNode("node-1", 1000, 1000*1024*1024)
	node.Allocatable.Disk = 100 * 1024 * 1024 * 1024 // 100GB
	node.Available.Disk = 50 * 1024 * 1024 * 1024    // 50GB

	workload := createTestWorkload("w1", 100, 100*1024*1024)
	workload.Resources.Requests.Disk = 10 * 1024 * 1024 * 1024 // 10GB

	scores := algo.ScoreNodes(workload, []*Node{node})
	if len(scores) != 1 {
		t.Errorf("expected 1 score, got %d", len(scores))
	}
}

func TestBinPackAlgorithm_EmptyNodes(t *testing.T) {
	algo := NewBinPackAlgorithm()
	workload := createTestWorkload("w1", 100, 100*1024*1024)

	selected := algo.SelectNode(workload, []*Node{})
	if selected != nil {
		t.Error("expected nil for empty nodes list")
	}
}

// =============================================================================
// Spread Algorithm Tests
// =============================================================================

func TestSpreadAlgorithm_Name(t *testing.T) {
	algo := NewSpreadAlgorithm()
	if algo.Name() != "Spread" {
		t.Errorf("expected name Spread, got %s", algo.Name())
	}
}

func TestSpreadAlgorithm_SelectNode_PrefersLowUtilization(t *testing.T) {
	algo := NewSpreadAlgorithm()

	// Node 1: 80% utilized
	node1 := createTestNode("node-1", 1000, 1000*1024*1024)
	node1.Allocated = Resources{CPU: 800, Memory: 800 * 1024 * 1024}
	node1.UpdateAvailable()

	// Node 2: 20% utilized
	node2 := createTestNode("node-2", 1000, 1000*1024*1024)
	node2.Allocated = Resources{CPU: 200, Memory: 200 * 1024 * 1024}
	node2.UpdateAvailable()

	workload := createTestWorkload("w1", 100, 100*1024*1024)

	selected := algo.SelectNode(workload, []*Node{node1, node2})

	// Spread should prefer node2 (lower utilization)
	if selected == nil {
		t.Fatal("expected a node to be selected")
	}
	if selected.ID != "node-2" {
		t.Errorf("expected node-2 (lower utilization), got %s", selected.ID)
	}
}

func TestSpreadAlgorithm_ScoreNodes(t *testing.T) {
	algo := NewSpreadAlgorithm()

	node1 := createTestNode("node-1", 1000, 1000*1024*1024)
	node1.Allocated = Resources{CPU: 800, Memory: 800 * 1024 * 1024}
	node1.UpdateAvailable()

	node2 := createTestNode("node-2", 1000, 1000*1024*1024)
	node2.Allocated = Resources{CPU: 200, Memory: 200 * 1024 * 1024}
	node2.UpdateAvailable()

	workload := createTestWorkload("w1", 100, 100*1024*1024)

	scores := algo.ScoreNodes(workload, []*Node{node1, node2})

	// Find scores by node
	var score1, score2 int64
	for _, s := range scores {
		switch s.Node.ID {
		case "node-1":
			score1 = s.Score
		case "node-2":
			score2 = s.Score
		}
	}

	// node-2 should have higher score (lower utilization)
	if score2 <= score1 {
		t.Errorf("expected node-2 score (%d) > node-1 score (%d)", score2, score1)
	}
}

func TestSpreadAlgorithm_SetWeights(t *testing.T) {
	algo := NewSpreadAlgorithm()
	algo.SetWeights(2.0, 0.5)

	node := createTestNode("node-1", 1000, 1000*1024*1024)
	workload := createTestWorkload("w1", 100, 100*1024*1024)

	scores := algo.ScoreNodes(workload, []*Node{node})
	if len(scores) != 1 {
		t.Errorf("expected 1 score, got %d", len(scores))
	}
}

func TestSpreadAlgorithm_SetZoneSpread(t *testing.T) {
	algo := NewSpreadAlgorithm()
	algo.SetZoneSpread(false)

	// Should still work with zone spread disabled
	node := createTestNode("node-1", 1000, 1000*1024*1024)
	workload := createTestWorkload("w1", 100, 100*1024*1024)

	selected := algo.SelectNode(workload, []*Node{node})
	if selected == nil {
		t.Error("expected node to be selected")
	}
}

func TestSpreadAlgorithm_NoFeasibleNodes(t *testing.T) {
	algo := NewSpreadAlgorithm()

	// Node with insufficient resources
	node := createTestNode("node-1", 100, 100*1024*1024)
	node.Available = Resources{CPU: 50, Memory: 50 * 1024 * 1024}

	workload := createTestWorkload("w1", 200, 200*1024*1024)

	selected := algo.SelectNode(workload, []*Node{node})
	if selected != nil {
		t.Error("expected nil when no feasible nodes")
	}
}

// =============================================================================
// Random Algorithm Tests
// =============================================================================

func TestRandomAlgorithm_Name(t *testing.T) {
	algo := NewRandomAlgorithm()
	if algo.Name() != "Random" {
		t.Errorf("expected name Random, got %s", algo.Name())
	}
}

func TestRandomAlgorithm_SelectNode_SelectsFeasible(t *testing.T) {
	algo := NewRandomAlgorithm()

	// Create multiple feasible nodes
	node1 := createTestNode("node-1", 1000, 1000*1024*1024)
	node2 := createTestNode("node-2", 1000, 1000*1024*1024)
	node3 := createTestNode("node-3", 1000, 1000*1024*1024)

	workload := createTestWorkload("w1", 100, 100*1024*1024)

	// Run multiple times to verify randomness
	selectedCounts := make(map[string]int)
	for i := 0; i < 100; i++ {
		selected := algo.SelectNode(workload, []*Node{node1, node2, node3})
		if selected != nil {
			selectedCounts[selected.ID]++
		}
	}

	// Verify at least 2 different nodes were selected (with high probability)
	if len(selectedCounts) < 2 {
		t.Log("Random algorithm might not be very random, only selected:", selectedCounts)
	}
}

func TestRandomAlgorithm_SelectNode_FiltersFeasible(t *testing.T) {
	algo := NewRandomAlgorithm()

	// Node 1: feasible
	node1 := createTestNode("node-1", 1000, 1000*1024*1024)

	// Node 2: not feasible
	node2 := createTestNode("node-2", 100, 100*1024*1024)
	node2.Available = Resources{CPU: 10, Memory: 10 * 1024 * 1024}

	workload := createTestWorkload("w1", 500, 500*1024*1024)

	// Run multiple times - should always select node1
	for i := 0; i < 10; i++ {
		selected := algo.SelectNode(workload, []*Node{node1, node2})
		if selected == nil {
			t.Fatal("expected a node to be selected")
		}
		if selected.ID != "node-1" {
			t.Errorf("expected node-1 (only feasible), got %s", selected.ID)
		}
	}
}

func TestRandomAlgorithm_ScoreNodes(t *testing.T) {
	algo := NewRandomAlgorithm()

	node1 := createTestNode("node-1", 1000, 1000*1024*1024)
	node2 := createTestNode("node-2", 1000, 1000*1024*1024)

	workload := createTestWorkload("w1", 100, 100*1024*1024)

	scores := algo.ScoreNodes(workload, []*Node{node1, node2})

	if len(scores) != 2 {
		t.Fatalf("expected 2 scores, got %d", len(scores))
	}

	// Verify scores are random (0-99)
	for _, s := range scores {
		if s.Score < 0 || s.Score > 99 {
			t.Errorf("expected score in range 0-99, got %d", s.Score)
		}
	}
}

func TestRandomAlgorithm_NoFeasibleNodes(t *testing.T) {
	algo := NewRandomAlgorithm()

	node := createTestNode("node-1", 100, 100*1024*1024)
	node.Available = Resources{CPU: 10, Memory: 10 * 1024 * 1024}

	workload := createTestWorkload("w1", 500, 500*1024*1024)

	selected := algo.SelectNode(workload, []*Node{node})
	if selected != nil {
		t.Error("expected nil when no feasible nodes")
	}
}

// =============================================================================
// Balanced Algorithm Tests
// =============================================================================

func TestBalancedAlgorithm_Name(t *testing.T) {
	algo := NewBalancedAlgorithm(0.5)
	if algo.Name() != "Balanced" {
		t.Errorf("expected name Balanced, got %s", algo.Name())
	}
}

func TestBalancedAlgorithm_RatioClamping(t *testing.T) {
	// Test ratio < 0
	algo1 := NewBalancedAlgorithm(-0.5)
	// Test ratio > 1
	algo2 := NewBalancedAlgorithm(1.5)

	// Both should create valid algorithms
	if algo1 == nil || algo2 == nil {
		t.Error("expected valid algorithms")
	}
}

func TestBalancedAlgorithm_FullBinPack(t *testing.T) {
	// ratio = 1.0 means full bin pack
	algo := NewBalancedAlgorithm(1.0)

	node1 := createTestNode("node-1", 1000, 1000*1024*1024)
	node1.Allocated = Resources{CPU: 800, Memory: 800 * 1024 * 1024}
	node1.UpdateAvailable()

	node2 := createTestNode("node-2", 1000, 1000*1024*1024)
	node2.Allocated = Resources{CPU: 200, Memory: 200 * 1024 * 1024}
	node2.UpdateAvailable()

	workload := createTestWorkload("w1", 100, 100*1024*1024)

	selected := algo.SelectNode(workload, []*Node{node1, node2})

	// Should behave like bin pack (prefer higher utilization)
	if selected.ID != "node-1" {
		t.Errorf("expected node-1 with ratio=1.0, got %s", selected.ID)
	}
}

func TestBalancedAlgorithm_FullSpread(t *testing.T) {
	// ratio = 0.0 means full spread
	algo := NewBalancedAlgorithm(0.0)

	node1 := createTestNode("node-1", 1000, 1000*1024*1024)
	node1.Allocated = Resources{CPU: 800, Memory: 800 * 1024 * 1024}
	node1.UpdateAvailable()

	node2 := createTestNode("node-2", 1000, 1000*1024*1024)
	node2.Allocated = Resources{CPU: 200, Memory: 200 * 1024 * 1024}
	node2.UpdateAvailable()

	workload := createTestWorkload("w1", 100, 100*1024*1024)

	selected := algo.SelectNode(workload, []*Node{node1, node2})

	// Should behave like spread (prefer lower utilization)
	if selected.ID != "node-2" {
		t.Errorf("expected node-2 with ratio=0.0, got %s", selected.ID)
	}
}

func TestBalancedAlgorithm_SetRatio(t *testing.T) {
	algo := NewBalancedAlgorithm(0.5)

	// Change to full bin pack
	algo.SetRatio(1.0)

	node1 := createTestNode("node-1", 1000, 1000*1024*1024)
	node1.Allocated = Resources{CPU: 800, Memory: 800 * 1024 * 1024}
	node1.UpdateAvailable()

	node2 := createTestNode("node-2", 1000, 1000*1024*1024)
	node2.Allocated = Resources{CPU: 200, Memory: 200 * 1024 * 1024}
	node2.UpdateAvailable()

	workload := createTestWorkload("w1", 100, 100*1024*1024)

	selected := algo.SelectNode(workload, []*Node{node1, node2})
	if selected.ID != "node-1" {
		t.Errorf("expected node-1 after SetRatio(1.0), got %s", selected.ID)
	}
}

func TestBalancedAlgorithm_ScoreNodes(t *testing.T) {
	algo := NewBalancedAlgorithm(0.5)

	node1 := createTestNode("node-1", 1000, 1000*1024*1024)
	node2 := createTestNode("node-2", 1000, 1000*1024*1024)

	workload := createTestWorkload("w1", 100, 100*1024*1024)

	scores := algo.ScoreNodes(workload, []*Node{node1, node2})

	if len(scores) != 2 {
		t.Fatalf("expected 2 scores, got %d", len(scores))
	}
}

// =============================================================================
// Resource Fit Algorithm Tests
// =============================================================================

func TestResourceFitAlgorithm_Name(t *testing.T) {
	algo := NewResourceFitAlgorithm(true)
	if algo.Name() != "ResourceFit" {
		t.Errorf("expected name ResourceFit, got %s", algo.Name())
	}
}

func TestResourceFitAlgorithm_PreferLarger(t *testing.T) {
	algo := NewResourceFitAlgorithm(true) // prefer larger

	// Node 1: More remaining resources
	node1 := createTestNode("node-1", 2000, 2000*1024*1024)
	node1.Allocated = Resources{CPU: 500, Memory: 500 * 1024 * 1024}
	node1.UpdateAvailable()

	// Node 2: Less remaining resources
	node2 := createTestNode("node-2", 1000, 1000*1024*1024)
	node2.Allocated = Resources{CPU: 500, Memory: 500 * 1024 * 1024}
	node2.UpdateAvailable()

	workload := createTestWorkload("w1", 100, 100*1024*1024)

	selected := algo.SelectNode(workload, []*Node{node1, node2})

	if selected.ID != "node-1" {
		t.Errorf("expected node-1 (more remaining), got %s", selected.ID)
	}
}

func TestResourceFitAlgorithm_PreferTightFit(t *testing.T) {
	algo := NewResourceFitAlgorithm(false) // prefer tight fit

	// Node 1: Lots of extra resources
	node1 := createTestNode("node-1", 2000, 2000*1024*1024)

	// Node 2: Tight fit
	node2 := createTestNode("node-2", 150, 150*1024*1024)

	workload := createTestWorkload("w1", 100, 100*1024*1024)

	selected := algo.SelectNode(workload, []*Node{node1, node2})

	if selected.ID != "node-2" {
		t.Errorf("expected node-2 (tighter fit), got %s", selected.ID)
	}
}

func TestResourceFitAlgorithm_NoFeasibleNodes(t *testing.T) {
	algo := NewResourceFitAlgorithm(true)

	node := createTestNode("node-1", 100, 100*1024*1024)
	node.Available = Resources{CPU: 10, Memory: 10 * 1024 * 1024}

	workload := createTestWorkload("w1", 500, 500*1024*1024)

	selected := algo.SelectNode(workload, []*Node{node})
	if selected != nil {
		t.Error("expected nil when no feasible nodes")
	}
}

// =============================================================================
// Priority Algorithm Tests
// =============================================================================

func TestPriorityAlgorithm_Name(t *testing.T) {
	base := NewBinPackAlgorithm()
	algo := NewPriorityAlgorithm(base)

	expected := "Priority(BinPack)"
	if algo.Name() != expected {
		t.Errorf("expected name %s, got %s", expected, algo.Name())
	}
}

func TestPriorityAlgorithm_SetPriorityBoost(t *testing.T) {
	base := NewBinPackAlgorithm()
	algo := NewPriorityAlgorithm(base)

	// Set boost for high priority
	algo.SetPriorityBoost(100, 500)

	node := createTestNode("node-1", 1000, 1000*1024*1024)

	workload := createTestWorkload("w1", 100, 100*1024*1024)
	workload.Priority = 100

	scores := algo.ScoreNodes(workload, []*Node{node})

	if len(scores) != 1 {
		t.Fatalf("expected 1 score, got %d", len(scores))
	}

	// Verify score includes boost
	baseAlgo := NewBinPackAlgorithm()
	baseScores := baseAlgo.ScoreNodes(workload, []*Node{node})
	baseScore := baseScores[0].Score

	if scores[0].Score != baseScore+500 {
		t.Errorf("expected score %d (base %d + boost 500), got %d",
			baseScore+500, baseScore, scores[0].Score)
	}
}

func TestPriorityAlgorithm_SelectNode(t *testing.T) {
	base := NewBinPackAlgorithm()
	algo := NewPriorityAlgorithm(base)

	node := createTestNode("node-1", 1000, 1000*1024*1024)
	workload := createTestWorkload("w1", 100, 100*1024*1024)

	// Should use base algorithm for selection
	selected := algo.SelectNode(workload, []*Node{node})
	if selected == nil {
		t.Error("expected a node to be selected")
	}
}

// =============================================================================
// Topology Spread Algorithm Tests
// =============================================================================

func TestTopologySpreadAlgorithm_Name(t *testing.T) {
	algo := NewTopologySpreadAlgorithm("zone", 1)
	if algo.Name() != "TopologySpread" {
		t.Errorf("expected name TopologySpread, got %s", algo.Name())
	}
}

func TestTopologySpreadAlgorithm_SpreadAcrossZones(t *testing.T) {
	algo := NewTopologySpreadAlgorithm("topology.kubernetes.io/zone", 1)

	// Node 1: zone-a
	node1 := createTestNode("node-1", 1000, 1000*1024*1024)
	node1.Labels["topology.kubernetes.io/zone"] = "zone-a"

	// Node 2: zone-b
	node2 := createTestNode("node-2", 1000, 1000*1024*1024)
	node2.Labels["topology.kubernetes.io/zone"] = "zone-b"

	workload := createTestWorkload("w1", 100, 100*1024*1024)

	scores := algo.ScoreNodes(workload, []*Node{node1, node2})

	if len(scores) != 2 {
		t.Fatalf("expected 2 scores, got %d", len(scores))
	}
}

func TestTopologySpreadAlgorithm_NoFeasibleNodes(t *testing.T) {
	algo := NewTopologySpreadAlgorithm("zone", 1)

	node := createTestNode("node-1", 100, 100*1024*1024)
	node.Available = Resources{CPU: 10, Memory: 10 * 1024 * 1024}
	node.Labels["zone"] = "zone-a"

	workload := createTestWorkload("w1", 500, 500*1024*1024)

	selected := algo.SelectNode(workload, []*Node{node})
	if selected != nil {
		t.Error("expected nil when no feasible nodes")
	}
}

// =============================================================================
// Composite Algorithm Tests
// =============================================================================

func TestCompositeAlgorithm_Name(t *testing.T) {
	algo := NewCompositeAlgorithm()
	if algo.Name() != "Composite" {
		t.Errorf("expected name Composite, got %s", algo.Name())
	}
}

func TestCompositeAlgorithm_CombinesScores(t *testing.T) {
	algo := NewCompositeAlgorithm()

	// Add bin pack with weight 0.5
	algo.AddAlgorithm(NewBinPackAlgorithm(), 0.5)
	// Add spread with weight 0.5
	algo.AddAlgorithm(NewSpreadAlgorithm(), 0.5)

	node1 := createTestNode("node-1", 1000, 1000*1024*1024)
	node1.Allocated = Resources{CPU: 500, Memory: 500 * 1024 * 1024}
	node1.UpdateAvailable()

	node2 := createTestNode("node-2", 1000, 1000*1024*1024)
	node2.Allocated = Resources{CPU: 800, Memory: 800 * 1024 * 1024}
	node2.UpdateAvailable()

	workload := createTestWorkload("w1", 100, 100*1024*1024)

	scores := algo.ScoreNodes(workload, []*Node{node1, node2})

	if len(scores) != 2 {
		t.Fatalf("expected 2 scores, got %d", len(scores))
	}
}

func TestCompositeAlgorithm_NoAlgorithms(t *testing.T) {
	algo := NewCompositeAlgorithm()
	// Don't add any algorithms

	node := createTestNode("node-1", 1000, 1000*1024*1024)
	workload := createTestWorkload("w1", 100, 100*1024*1024)

	scores := algo.ScoreNodes(workload, []*Node{node})
	if scores != nil {
		t.Errorf("expected nil scores with no algorithms, got %d", len(scores))
	}
}

func TestCompositeAlgorithm_SelectNode(t *testing.T) {
	algo := NewCompositeAlgorithm()
	algo.AddAlgorithm(NewBinPackAlgorithm(), 1.0)

	node := createTestNode("node-1", 1000, 1000*1024*1024)
	workload := createTestWorkload("w1", 100, 100*1024*1024)

	selected := algo.SelectNode(workload, []*Node{node})
	if selected == nil {
		t.Error("expected a node to be selected")
	}
}

// =============================================================================
// Least Requested Algorithm Tests
// =============================================================================

func TestLeastRequestedAlgorithm_Name(t *testing.T) {
	algo := NewLeastRequestedAlgorithm()
	if algo.Name() != "LeastRequested" {
		t.Errorf("expected name LeastRequested, got %s", algo.Name())
	}
}

func TestLeastRequestedAlgorithm_PrefersMoreAvailable(t *testing.T) {
	algo := NewLeastRequestedAlgorithm()

	// Node 1: 80% utilized (less available)
	node1 := createTestNode("node-1", 1000, 1000*1024*1024)
	node1.Allocated = Resources{CPU: 800, Memory: 800 * 1024 * 1024}
	node1.UpdateAvailable()

	// Node 2: 20% utilized (more available)
	node2 := createTestNode("node-2", 1000, 1000*1024*1024)
	node2.Allocated = Resources{CPU: 200, Memory: 200 * 1024 * 1024}
	node2.UpdateAvailable()

	workload := createTestWorkload("w1", 100, 100*1024*1024)

	selected := algo.SelectNode(workload, []*Node{node1, node2})

	if selected.ID != "node-2" {
		t.Errorf("expected node-2 (more available), got %s", selected.ID)
	}
}

func TestLeastRequestedAlgorithm_ScoreNodes(t *testing.T) {
	algo := NewLeastRequestedAlgorithm()

	node1 := createTestNode("node-1", 1000, 1000*1024*1024)
	node1.Allocated = Resources{CPU: 800, Memory: 800 * 1024 * 1024}
	node1.UpdateAvailable()

	node2 := createTestNode("node-2", 1000, 1000*1024*1024)
	node2.Allocated = Resources{CPU: 200, Memory: 200 * 1024 * 1024}
	node2.UpdateAvailable()

	workload := createTestWorkload("w1", 100, 100*1024*1024)

	scores := algo.ScoreNodes(workload, []*Node{node1, node2})

	var score1, score2 int64
	for _, s := range scores {
		if s.Node.ID == "node-1" {
			score1 = s.Score
		} else {
			score2 = s.Score
		}
	}

	if score2 <= score1 {
		t.Errorf("expected node-2 score (%d) > node-1 score (%d)", score2, score1)
	}
}

func TestLeastRequestedAlgorithm_ZeroAllocatable(t *testing.T) {
	algo := NewLeastRequestedAlgorithm()

	node := createTestNode("node-1", 0, 0)
	workload := createTestWorkload("w1", 0, 0)
	workload.Resources.Requests = Resources{}

	scores := algo.ScoreNodes(workload, []*Node{node})
	// Should handle zero allocatable gracefully
	if len(scores) != 1 {
		t.Errorf("expected 1 score, got %d", len(scores))
	}
}

// =============================================================================
// Most Requested Algorithm Tests
// =============================================================================

func TestMostRequestedAlgorithm_Name(t *testing.T) {
	algo := NewMostRequestedAlgorithm()
	if algo.Name() != "MostRequested" {
		t.Errorf("expected name MostRequested, got %s", algo.Name())
	}
}

func TestMostRequestedAlgorithm_PrefersHigherUtilization(t *testing.T) {
	algo := NewMostRequestedAlgorithm()

	// Node 1: 80% utilized (higher)
	node1 := createTestNode("node-1", 1000, 1000*1024*1024)
	node1.Allocated = Resources{CPU: 800, Memory: 800 * 1024 * 1024}
	node1.UpdateAvailable()

	// Node 2: 20% utilized (lower)
	node2 := createTestNode("node-2", 1000, 1000*1024*1024)
	node2.Allocated = Resources{CPU: 200, Memory: 200 * 1024 * 1024}
	node2.UpdateAvailable()

	workload := createTestWorkload("w1", 100, 100*1024*1024)

	selected := algo.SelectNode(workload, []*Node{node1, node2})

	if selected.ID != "node-1" {
		t.Errorf("expected node-1 (higher utilization), got %s", selected.ID)
	}
}

func TestMostRequestedAlgorithm_ScoreNodes(t *testing.T) {
	algo := NewMostRequestedAlgorithm()

	node1 := createTestNode("node-1", 1000, 1000*1024*1024)
	node1.Allocated = Resources{CPU: 800, Memory: 800 * 1024 * 1024}
	node1.UpdateAvailable()

	node2 := createTestNode("node-2", 1000, 1000*1024*1024)
	node2.Allocated = Resources{CPU: 200, Memory: 200 * 1024 * 1024}
	node2.UpdateAvailable()

	workload := createTestWorkload("w1", 100, 100*1024*1024)

	scores := algo.ScoreNodes(workload, []*Node{node1, node2})

	var score1, score2 int64
	for _, s := range scores {
		if s.Node.ID == "node-1" {
			score1 = s.Score
		} else {
			score2 = s.Score
		}
	}

	if score1 <= score2 {
		t.Errorf("expected node-1 score (%d) > node-2 score (%d)", score1, score2)
	}
}

func TestMostRequestedAlgorithm_ZeroAllocatable(t *testing.T) {
	algo := NewMostRequestedAlgorithm()

	node := createTestNode("node-1", 0, 0)
	workload := createTestWorkload("w1", 0, 0)
	workload.Resources.Requests = Resources{}

	scores := algo.ScoreNodes(workload, []*Node{node})
	// Should handle zero allocatable gracefully
	if len(scores) != 1 {
		t.Errorf("expected 1 score, got %d", len(scores))
	}
}

// =============================================================================
// Concurrent Access Tests
// =============================================================================

func TestBinPackAlgorithm_ConcurrentAccess(t *testing.T) {
	algo := NewBinPackAlgorithm()

	nodes := []*Node{
		createTestNode("node-1", 1000, 1000*1024*1024),
		createTestNode("node-2", 1000, 1000*1024*1024),
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			workload := createTestWorkload("w", 100, 100*1024*1024)

			// Concurrent SetWeights
			if i%2 == 0 {
				algo.SetWeights(float64(i%3), float64(i%3), float64(i%3))
			}

			// Concurrent SelectNode
			algo.SelectNode(workload, nodes)

			// Concurrent ScoreNodes
			algo.ScoreNodes(workload, nodes)
		}(i)
	}

	wg.Wait()
}

func TestSpreadAlgorithm_ConcurrentAccess(t *testing.T) {
	algo := NewSpreadAlgorithm()

	nodes := []*Node{
		createTestNode("node-1", 1000, 1000*1024*1024),
		createTestNode("node-2", 1000, 1000*1024*1024),
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			workload := createTestWorkload("w", 100, 100*1024*1024)

			// Concurrent SetWeights and SetZoneSpread
			if i%2 == 0 {
				algo.SetWeights(float64(i%3), float64(i%3))
				algo.SetZoneSpread(i%3 == 0)
			}

			algo.SelectNode(workload, nodes)
			algo.ScoreNodes(workload, nodes)
		}(i)
	}

	wg.Wait()
}

func TestBalancedAlgorithm_ConcurrentAccess(t *testing.T) {
	algo := NewBalancedAlgorithm(0.5)

	nodes := []*Node{
		createTestNode("node-1", 1000, 1000*1024*1024),
		createTestNode("node-2", 1000, 1000*1024*1024),
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			workload := createTestWorkload("w", 100, 100*1024*1024)

			// Concurrent SetRatio
			if i%2 == 0 {
				algo.SetRatio(float64(i%100) / 100.0)
			}

			algo.SelectNode(workload, nodes)
			algo.ScoreNodes(workload, nodes)
		}(i)
	}

	wg.Wait()
}

func TestCompositeAlgorithm_ConcurrentAccess(t *testing.T) {
	algo := NewCompositeAlgorithm()
	algo.AddAlgorithm(NewBinPackAlgorithm(), 1.0)

	nodes := []*Node{
		createTestNode("node-1", 1000, 1000*1024*1024),
		createTestNode("node-2", 1000, 1000*1024*1024),
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			workload := createTestWorkload("w", 100, 100*1024*1024)

			// Concurrent AddAlgorithm
			if i%10 == 0 {
				algo.AddAlgorithm(NewSpreadAlgorithm(), 0.5)
			}

			algo.SelectNode(workload, nodes)
			algo.ScoreNodes(workload, nodes)
		}(i)
	}

	wg.Wait()
}

// =============================================================================
// Edge Cases
// =============================================================================

func TestAlgorithms_NilWorkload(t *testing.T) {
	// This tests defensive coding - algorithms should handle edge cases
	algorithms := []Algorithm{
		NewBinPackAlgorithm(),
		NewSpreadAlgorithm(),
		NewRandomAlgorithm(),
		NewBalancedAlgorithm(0.5),
		NewLeastRequestedAlgorithm(),
		NewMostRequestedAlgorithm(),
	}

	node := createTestNode("node-1", 1000, 1000*1024*1024)
	nodes := []*Node{node}

	for _, algo := range algorithms {
		// Test with very small workload
		smallWorkload := createTestWorkload("w1", 0, 0)
		smallWorkload.Resources.Requests = Resources{}

		// These should not panic
		algo.SelectNode(smallWorkload, nodes)
		algo.ScoreNodes(smallWorkload, nodes)
	}
}

func TestAlgorithms_LargeCluster(t *testing.T) {
	algo := NewBinPackAlgorithm()

	// Create 1000 nodes
	nodes := make([]*Node, 1000)
	for i := 0; i < 1000; i++ {
		nodes[i] = createTestNode(
			"node-"+string(rune(i)),
			int64(1000+i),
			int64((1000+i)*1024*1024),
		)
		nodes[i].Allocated = Resources{
			CPU:    int64(i % 500),
			Memory: int64((i % 500) * 1024 * 1024),
		}
		nodes[i].UpdateAvailable()
	}

	workload := createTestWorkload("w1", 100, 100*1024*1024)

	start := time.Now()
	selected := algo.SelectNode(workload, nodes)
	duration := time.Since(start)

	if selected == nil {
		t.Error("expected a node to be selected")
	}

	// Should complete in reasonable time
	if duration > 1*time.Second {
		t.Errorf("selection took too long: %v", duration)
	}
}

// =============================================================================
// Algorithm Interface Compliance Tests
// =============================================================================

func TestAllAlgorithmsImplementInterface(t *testing.T) {
	algorithms := []Algorithm{
		NewBinPackAlgorithm(),
		NewSpreadAlgorithm(),
		NewRandomAlgorithm(),
		NewBalancedAlgorithm(0.5),
		NewResourceFitAlgorithm(true),
		NewPriorityAlgorithm(NewBinPackAlgorithm()),
		NewTopologySpreadAlgorithm("zone", 1),
		NewCompositeAlgorithm(),
		NewLeastRequestedAlgorithm(),
		NewMostRequestedAlgorithm(),
	}

	for _, algo := range algorithms {
		// Verify Name() returns non-empty string
		if algo.Name() == "" {
			t.Errorf("algorithm returned empty name")
		}

		// Verify SelectNode and ScoreNodes don't panic on empty input
		algo.SelectNode(createTestWorkload("w1", 100, 100*1024*1024), []*Node{})
		algo.ScoreNodes(createTestWorkload("w1", 100, 100*1024*1024), []*Node{})
	}
}
