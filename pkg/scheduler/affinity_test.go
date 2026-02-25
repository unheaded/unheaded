// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package scheduler

import (
	"sync"
	"testing"
	"time"
)

// =============================================================================
// AffinityChecker Tests
// =============================================================================

func TestAffinityChecker_NewAffinityChecker(t *testing.T) {
	ac := NewAffinityChecker()
	if ac == nil {
		t.Fatal("expected non-nil affinity checker")
	}
}

func TestAffinityChecker_AddWorkload(t *testing.T) {
	ac := NewAffinityChecker()

	w := createTestWorkload("w1", 100, 100*1024*1024)
	w.Labels["app"] = "myapp"
	w.Labels["version"] = "v1"

	ac.AddWorkload(w, "node-1")

	workloads := ac.GetWorkloadsOnNode("node-1")
	if len(workloads) != 1 {
		t.Errorf("expected 1 workload, got %d", len(workloads))
	}

	// Verify label index
	labeledWorkloads := ac.GetWorkloadsByLabel("app", "myapp")
	if len(labeledWorkloads) != 1 {
		t.Errorf("expected 1 workload with label, got %d", len(labeledWorkloads))
	}
}

func TestAffinityChecker_RemoveWorkload(t *testing.T) {
	ac := NewAffinityChecker()

	w := createTestWorkload("w1", 100, 100*1024*1024)
	w.Labels["app"] = "myapp"

	ac.AddWorkload(w, "node-1")
	ac.RemoveWorkload(w, "node-1")

	workloads := ac.GetWorkloadsOnNode("node-1")
	if len(workloads) != 0 {
		t.Errorf("expected 0 workloads after remove, got %d", len(workloads))
	}

	// Verify label index is cleaned
	labeledWorkloads := ac.GetWorkloadsByLabel("app", "myapp")
	if len(labeledWorkloads) != 0 {
		t.Errorf("expected 0 workloads with label, got %d", len(labeledWorkloads))
	}
}

func TestAffinityChecker_CheckNodeAffinity_NoAffinity(t *testing.T) {
	ac := NewAffinityChecker()

	w := createTestWorkload("w1", 100, 100*1024*1024)
	w.Affinity = nil

	node := createTestNode("node-1", 1000, 1000*1024*1024)

	satisfied, score := ac.CheckNodeAffinity(w, node)
	if !satisfied {
		t.Error("expected satisfied when no affinity")
	}
	if score != 0 {
		t.Errorf("expected score 0, got %d", score)
	}
}

func TestAffinityChecker_CheckNodeAffinity_RequiredSatisfied(t *testing.T) {
	ac := NewAffinityChecker()

	w := createTestWorkload("w1", 100, 100*1024*1024)
	w.Affinity = &AffinitySpec{
		NodeAffinity: &NodeAffinity{
			RequiredDuringScheduling: []NodeSelectorTerm{
				{
					MatchExpressions: []LabelSelectorRequirement{
						{
							Key:      "zone",
							Operator: LabelSelectorOpIn,
							Values:   []string{"zone-a", "zone-b"},
						},
					},
				},
			},
		},
	}

	node := createTestNode("node-1", 1000, 1000*1024*1024)
	node.Labels["zone"] = "zone-a"

	satisfied, _ := ac.CheckNodeAffinity(w, node)
	if !satisfied {
		t.Error("expected satisfied when node matches required affinity")
	}
}

func TestAffinityChecker_CheckNodeAffinity_RequiredNotSatisfied(t *testing.T) {
	ac := NewAffinityChecker()

	w := createTestWorkload("w1", 100, 100*1024*1024)
	w.Affinity = &AffinitySpec{
		NodeAffinity: &NodeAffinity{
			RequiredDuringScheduling: []NodeSelectorTerm{
				{
					MatchExpressions: []LabelSelectorRequirement{
						{
							Key:      "zone",
							Operator: LabelSelectorOpIn,
							Values:   []string{"zone-a", "zone-b"},
						},
					},
				},
			},
		},
	}

	node := createTestNode("node-1", 1000, 1000*1024*1024)
	node.Labels["zone"] = "zone-c" // Not in allowed zones

	satisfied, _ := ac.CheckNodeAffinity(w, node)
	if satisfied {
		t.Error("expected not satisfied when node doesn't match required affinity")
	}
}

func TestAffinityChecker_CheckNodeAffinity_PreferredScore(t *testing.T) {
	ac := NewAffinityChecker()

	w := createTestWorkload("w1", 100, 100*1024*1024)
	w.Affinity = &AffinitySpec{
		NodeAffinity: &NodeAffinity{
			PreferredDuringScheduling: []PreferredSchedulingTerm{
				{
					Weight: 50,
					Preference: NodeSelectorTerm{
						MatchExpressions: []LabelSelectorRequirement{
							{
								Key:      "zone",
								Operator: LabelSelectorOpIn,
								Values:   []string{"zone-a"},
							},
						},
					},
				},
				{
					Weight: 30,
					Preference: NodeSelectorTerm{
						MatchExpressions: []LabelSelectorRequirement{
							{
								Key:      "tier",
								Operator: LabelSelectorOpIn,
								Values:   []string{"premium"},
							},
						},
					},
				},
			},
		},
	}

	node := createTestNode("node-1", 1000, 1000*1024*1024)
	node.Labels["zone"] = "zone-a"
	node.Labels["tier"] = "premium"

	satisfied, score := ac.CheckNodeAffinity(w, node)
	if !satisfied {
		t.Error("expected satisfied")
	}
	if score != 80 { // 50 + 30
		t.Errorf("expected score 80, got %d", score)
	}
}

func TestAffinityChecker_CheckNodeAffinity_Operators(t *testing.T) {
	ac := NewAffinityChecker()

	tests := []struct {
		name     string
		operator LabelSelectorOperator
		values   []string
		nodeVal  string
		nodeHas  bool
		expected bool
	}{
		{"In_Match", LabelSelectorOpIn, []string{"a", "b"}, "a", true, true},
		{"In_NoMatch", LabelSelectorOpIn, []string{"a", "b"}, "c", true, false},
		{"In_NoLabel", LabelSelectorOpIn, []string{"a", "b"}, "", false, false},
		{"NotIn_Match", LabelSelectorOpNotIn, []string{"a", "b"}, "c", true, true},
		{"NotIn_NoMatch", LabelSelectorOpNotIn, []string{"a", "b"}, "a", true, false},
		{"NotIn_NoLabel", LabelSelectorOpNotIn, []string{"a", "b"}, "", false, true},
		{"Exists_HasLabel", LabelSelectorOpExists, nil, "any", true, true},
		{"Exists_NoLabel", LabelSelectorOpExists, nil, "", false, false},
		{"DoesNotExist_HasLabel", LabelSelectorOpDoesNotExist, nil, "any", true, false},
		{"DoesNotExist_NoLabel", LabelSelectorOpDoesNotExist, nil, "", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := createTestWorkload("w1", 100, 100*1024*1024)
			w.Affinity = &AffinitySpec{
				NodeAffinity: &NodeAffinity{
					RequiredDuringScheduling: []NodeSelectorTerm{
						{
							MatchExpressions: []LabelSelectorRequirement{
								{
									Key:      "key",
									Operator: tt.operator,
									Values:   tt.values,
								},
							},
						},
					},
				},
			}

			node := createTestNode("node-1", 1000, 1000*1024*1024)
			if tt.nodeHas {
				node.Labels["key"] = tt.nodeVal
			}

			satisfied, _ := ac.CheckNodeAffinity(w, node)
			if satisfied != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, satisfied)
			}
		})
	}
}

func TestAffinityChecker_CheckNodeAffinity_FieldSelectors(t *testing.T) {
	ac := NewAffinityChecker()

	w := createTestWorkload("w1", 100, 100*1024*1024)
	w.Affinity = &AffinitySpec{
		NodeAffinity: &NodeAffinity{
			RequiredDuringScheduling: []NodeSelectorTerm{
				{
					MatchFields: []FieldSelectorRequirement{
						{
							Key:      "metadata.name",
							Operator: "=",
							Value:    "node-1",
						},
					},
				},
			},
		},
	}

	node1 := createTestNode("node-1", 1000, 1000*1024*1024)
	node1.Name = "node-1"

	node2 := createTestNode("node-2", 1000, 1000*1024*1024)
	node2.Name = "node-2"

	satisfied1, _ := ac.CheckNodeAffinity(w, node1)
	satisfied2, _ := ac.CheckNodeAffinity(w, node2)

	if !satisfied1 {
		t.Error("node-1 should satisfy field selector")
	}
	if satisfied2 {
		t.Error("node-2 should not satisfy field selector")
	}
}

func TestAffinityChecker_CheckWorkloadAffinity_NoAffinity(t *testing.T) {
	ac := NewAffinityChecker()

	w := createTestWorkload("w1", 100, 100*1024*1024)
	w.Affinity = nil

	node := createTestNode("node-1", 1000, 1000*1024*1024)

	satisfied, score := ac.CheckWorkloadAffinity(w, node)
	if !satisfied {
		t.Error("expected satisfied when no affinity")
	}
	if score != 0 {
		t.Errorf("expected score 0, got %d", score)
	}
}

func TestAffinityChecker_CheckWorkloadAffinity_Required(t *testing.T) {
	ac := NewAffinityChecker()

	// Add existing workload to node
	existing := createTestWorkload("existing", 100, 100*1024*1024)
	existing.Labels["app"] = "cache"
	existing.Namespace = "default"
	ac.AddWorkload(existing, "node-1")

	// New workload requires co-location with cache
	w := createTestWorkload("w1", 100, 100*1024*1024)
	w.Namespace = "default"
	w.Affinity = &AffinitySpec{
		WorkloadAffinity: &WorkloadAffinity{
			RequiredDuringScheduling: []WorkloadAffinityTerm{
				{
					LabelSelector: LabelSelector{
						MatchLabels: map[string]string{"app": "cache"},
					},
					TopologyKey: "zone",
				},
			},
		},
	}

	node := createTestNode("node-1", 1000, 1000*1024*1024)

	satisfied, _ := ac.CheckWorkloadAffinity(w, node)
	if !satisfied {
		t.Error("expected satisfied when required workload exists")
	}
}

func TestAffinityChecker_CheckWorkloadAffinity_RequiredNotSatisfied(t *testing.T) {
	ac := NewAffinityChecker()

	// New workload requires co-location with cache, but no cache exists
	w := createTestWorkload("w1", 100, 100*1024*1024)
	w.Namespace = "default"
	w.Affinity = &AffinitySpec{
		WorkloadAffinity: &WorkloadAffinity{
			RequiredDuringScheduling: []WorkloadAffinityTerm{
				{
					LabelSelector: LabelSelector{
						MatchLabels: map[string]string{"app": "cache"},
					},
				},
			},
		},
	}

	node := createTestNode("node-1", 1000, 1000*1024*1024)

	satisfied, _ := ac.CheckWorkloadAffinity(w, node)
	if satisfied {
		t.Error("expected not satisfied when required workload doesn't exist")
	}
}

func TestAffinityChecker_CheckWorkloadAntiAffinity_NoAffinity(t *testing.T) {
	ac := NewAffinityChecker()

	w := createTestWorkload("w1", 100, 100*1024*1024)
	w.Affinity = nil

	node := createTestNode("node-1", 1000, 1000*1024*1024)

	satisfied, score := ac.CheckWorkloadAntiAffinity(w, node)
	if !satisfied {
		t.Error("expected satisfied when no anti-affinity")
	}
	if score != 0 {
		t.Errorf("expected score 0, got %d", score)
	}
}

func TestAffinityChecker_CheckWorkloadAntiAffinity_Required(t *testing.T) {
	ac := NewAffinityChecker()

	// Add existing workload to node
	existing := createTestWorkload("existing", 100, 100*1024*1024)
	existing.Labels["app"] = "myapp"
	existing.Namespace = "default"
	ac.AddWorkload(existing, "node-1")

	// New workload has anti-affinity with myapp
	w := createTestWorkload("w1", 100, 100*1024*1024)
	w.Namespace = "default"
	w.Affinity = &AffinitySpec{
		WorkloadAntiAffinity: &WorkloadAntiAffinity{
			RequiredDuringScheduling: []WorkloadAffinityTerm{
				{
					LabelSelector: LabelSelector{
						MatchLabels: map[string]string{"app": "myapp"},
					},
				},
			},
		},
	}

	node := createTestNode("node-1", 1000, 1000*1024*1024)

	satisfied, _ := ac.CheckWorkloadAntiAffinity(w, node)
	if satisfied {
		t.Error("expected not satisfied when conflicting workload exists")
	}
}

func TestAffinityChecker_CheckWorkloadAntiAffinity_RequiredSatisfied(t *testing.T) {
	ac := NewAffinityChecker()

	// New workload has anti-affinity, but no conflicting workloads exist
	w := createTestWorkload("w1", 100, 100*1024*1024)
	w.Namespace = "default"
	w.Affinity = &AffinitySpec{
		WorkloadAntiAffinity: &WorkloadAntiAffinity{
			RequiredDuringScheduling: []WorkloadAffinityTerm{
				{
					LabelSelector: LabelSelector{
						MatchLabels: map[string]string{"app": "myapp"},
					},
				},
			},
		},
	}

	node := createTestNode("node-1", 1000, 1000*1024*1024)

	satisfied, _ := ac.CheckWorkloadAntiAffinity(w, node)
	if !satisfied {
		t.Error("expected satisfied when no conflicting workloads")
	}
}

func TestAffinityChecker_CheckWorkloadAntiAffinity_Preferred(t *testing.T) {
	ac := NewAffinityChecker()

	// New workload prefers anti-affinity with myapp
	w := createTestWorkload("w1", 100, 100*1024*1024)
	w.Namespace = "default"
	w.Affinity = &AffinitySpec{
		WorkloadAntiAffinity: &WorkloadAntiAffinity{
			PreferredDuringScheduling: []WeightedWorkloadAffinityTerm{
				{
					Weight: 50,
					AffinityTerm: WorkloadAffinityTerm{
						LabelSelector: LabelSelector{
							MatchLabels: map[string]string{"app": "myapp"},
						},
					},
				},
			},
		},
	}

	node := createTestNode("node-1", 1000, 1000*1024*1024)

	satisfied, score := ac.CheckWorkloadAntiAffinity(w, node)
	if !satisfied {
		t.Error("preferred anti-affinity should always be satisfied")
	}
	if score != 50 { // Score added when no matching workloads
		t.Errorf("expected score 50, got %d", score)
	}
}

func TestAffinityChecker_NamespaceFiltering(t *testing.T) {
	ac := NewAffinityChecker()

	// Add workload in namespace-a
	existing := createTestWorkload("existing", 100, 100*1024*1024)
	existing.Labels["app"] = "cache"
	existing.Namespace = "namespace-a"
	ac.AddWorkload(existing, "node-1")

	// New workload in namespace-b requires cache
	w := createTestWorkload("w1", 100, 100*1024*1024)
	w.Namespace = "namespace-b"
	w.Affinity = &AffinitySpec{
		WorkloadAffinity: &WorkloadAffinity{
			RequiredDuringScheduling: []WorkloadAffinityTerm{
				{
					LabelSelector: LabelSelector{
						MatchLabels: map[string]string{"app": "cache"},
					},
					// No namespaces specified - defaults to same namespace
				},
			},
		},
	}

	node := createTestNode("node-1", 1000, 1000*1024*1024)

	satisfied, _ := ac.CheckWorkloadAffinity(w, node)
	if satisfied {
		t.Error("expected not satisfied - different namespace")
	}
}

func TestAffinityChecker_ExplicitNamespaces(t *testing.T) {
	ac := NewAffinityChecker()

	// Add workload in namespace-a
	existing := createTestWorkload("existing", 100, 100*1024*1024)
	existing.Labels["app"] = "cache"
	existing.Namespace = "namespace-a"
	ac.AddWorkload(existing, "node-1")

	// New workload in namespace-b, but explicitly looks in namespace-a
	w := createTestWorkload("w1", 100, 100*1024*1024)
	w.Namespace = "namespace-b"
	w.Affinity = &AffinitySpec{
		WorkloadAffinity: &WorkloadAffinity{
			RequiredDuringScheduling: []WorkloadAffinityTerm{
				{
					LabelSelector: LabelSelector{
						MatchLabels: map[string]string{"app": "cache"},
					},
					Namespaces: []string{"namespace-a"},
				},
			},
		},
	}

	node := createTestNode("node-1", 1000, 1000*1024*1024)

	satisfied, _ := ac.CheckWorkloadAffinity(w, node)
	if !satisfied {
		t.Error("expected satisfied - explicitly checking namespace-a")
	}
}

func TestAffinityChecker_ConcurrentAccess(t *testing.T) {
	ac := NewAffinityChecker()

	var wg sync.WaitGroup

	// Concurrent adds and removes
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			w := createTestWorkload("w-"+string(rune(i)), 100, 100*1024*1024)
			w.Labels["app"] = "myapp"
			nodeID := "node-" + string(rune(i%5))

			ac.AddWorkload(w, nodeID)
			ac.GetWorkloadsOnNode(nodeID)
			ac.GetWorkloadsByLabel("app", "myapp")
			ac.RemoveWorkload(w, nodeID)
		}(i)
	}

	wg.Wait()
}

// =============================================================================
// TopologySpreadChecker Tests
// =============================================================================

func TestTopologySpreadChecker_NewTopologySpreadChecker(t *testing.T) {
	tc := NewTopologySpreadChecker()
	if tc == nil {
		t.Fatal("expected non-nil topology spread checker")
	}
}

func TestTopologySpreadChecker_RegisterNode(t *testing.T) {
	tc := NewTopologySpreadChecker()

	node := createTestNode("node-1", 1000, 1000*1024*1024)
	node.Labels["zone"] = "zone-a"
	node.Labels["rack"] = "rack-1"

	tc.RegisterNode(node)

	// Should not panic and should store topology info
}

func TestTopologySpreadChecker_UnregisterNode(t *testing.T) {
	tc := NewTopologySpreadChecker()

	node := createTestNode("node-1", 1000, 1000*1024*1024)
	tc.RegisterNode(node)
	tc.UnregisterNode("node-1")

	// Should not panic
}

func TestTopologySpreadChecker_RecordWorkloadPlacement(t *testing.T) {
	tc := NewTopologySpreadChecker()

	node := createTestNode("node-1", 1000, 1000*1024*1024)
	node.Labels["zone"] = "zone-a"
	tc.RegisterNode(node)

	w := createTestWorkload("w1", 100, 100*1024*1024)
	tc.RecordWorkloadPlacement(w, "node-1")

	// Should not panic and should update topology counts
}

func TestTopologySpreadChecker_RemoveWorkloadPlacement(t *testing.T) {
	tc := NewTopologySpreadChecker()

	node := createTestNode("node-1", 1000, 1000*1024*1024)
	node.Labels["zone"] = "zone-a"
	tc.RegisterNode(node)

	w := createTestWorkload("w1", 100, 100*1024*1024)
	tc.RecordWorkloadPlacement(w, "node-1")
	tc.RemoveWorkloadPlacement(w, "node-1")

	// Should not panic
}

func TestTopologySpreadChecker_CheckConstraints_NoConstraints(t *testing.T) {
	tc := NewTopologySpreadChecker()

	w := createTestWorkload("w1", 100, 100*1024*1024)
	node := createTestNode("node-1", 1000, 1000*1024*1024)

	satisfied, _ := tc.CheckConstraints(w, node, []TopologySpreadConstraint{})
	if !satisfied {
		t.Error("expected satisfied with no constraints")
	}
}

func TestTopologySpreadChecker_CheckConstraints_DoNotSchedule(t *testing.T) {
	tc := NewTopologySpreadChecker()

	node := createTestNode("node-1", 1000, 1000*1024*1024)
	node.Labels["zone"] = "zone-a"
	tc.RegisterNode(node)

	w := createTestWorkload("w1", 100, 100*1024*1024)

	constraints := []TopologySpreadConstraint{
		{
			TopologyKey:       "zone",
			MaxSkew:           1,
			WhenUnsatisfiable: DoNotSchedule,
		},
	}

	satisfied, _ := tc.CheckConstraints(w, node, constraints)
	if !satisfied {
		t.Error("expected satisfied with first workload (no skew)")
	}
}

func TestTopologySpreadChecker_CheckConstraints_NoTopologyKey(t *testing.T) {
	tc := NewTopologySpreadChecker()

	node := createTestNode("node-1", 1000, 1000*1024*1024)
	// Node has no zone label
	tc.RegisterNode(node)

	w := createTestWorkload("w1", 100, 100*1024*1024)

	constraints := []TopologySpreadConstraint{
		{
			TopologyKey:       "zone",
			MaxSkew:           1,
			WhenUnsatisfiable: DoNotSchedule,
		},
	}

	satisfied, _ := tc.CheckConstraints(w, node, constraints)
	if satisfied {
		t.Error("expected not satisfied when node doesn't have topology key")
	}
}

func TestTopologySpreadChecker_CheckConstraints_ScheduleAnyway(t *testing.T) {
	tc := NewTopologySpreadChecker()

	node := createTestNode("node-1", 1000, 1000*1024*1024)
	// Node has no zone label
	tc.RegisterNode(node)

	w := createTestWorkload("w1", 100, 100*1024*1024)

	constraints := []TopologySpreadConstraint{
		{
			TopologyKey:       "zone",
			MaxSkew:           1,
			WhenUnsatisfiable: ScheduleAnyway,
		},
	}

	satisfied, _ := tc.CheckConstraints(w, node, constraints)
	if !satisfied {
		t.Error("expected satisfied with ScheduleAnyway")
	}
}

func TestTopologySpreadChecker_ConcurrentAccess(t *testing.T) {
	tc := NewTopologySpreadChecker()

	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			node := createTestNode("node-"+string(rune(i)), 1000, 1000*1024*1024)
			node.Labels["zone"] = "zone-" + string(rune(i%3))

			tc.RegisterNode(node)

			w := createTestWorkload("w-"+string(rune(i)), 100, 100*1024*1024)
			tc.RecordWorkloadPlacement(w, node.ID)

			tc.CheckConstraints(w, node, []TopologySpreadConstraint{
				{TopologyKey: "zone", MaxSkew: 1, WhenUnsatisfiable: DoNotSchedule},
			})

			tc.RemoveWorkloadPlacement(w, node.ID)
			tc.UnregisterNode(node.ID)
		}(i)
	}

	wg.Wait()
}

// =============================================================================
// InterWorkloadAffinityManager Tests
// =============================================================================

func TestInterWorkloadAffinityManager_NewInterWorkloadAffinityManager(t *testing.T) {
	m := NewInterWorkloadAffinityManager()
	if m == nil {
		t.Fatal("expected non-nil manager")
	}
}

func TestInterWorkloadAffinityManager_AddAffinity(t *testing.T) {
	m := NewInterWorkloadAffinityManager()

	affinity := InterWorkloadAffinity{
		Source: LabelSelector{
			MatchLabels: map[string]string{"app": "frontend"},
		},
		Target: LabelSelector{
			MatchLabels: map[string]string{"app": "backend"},
		},
		Type:        AffinityTypeRequired,
		TopologyKey: "zone",
	}

	m.AddAffinity(affinity)

	affinities := m.GetAffinities()
	if len(affinities) != 1 {
		t.Errorf("expected 1 affinity, got %d", len(affinities))
	}
}

func TestInterWorkloadAffinityManager_RemoveAffinity(t *testing.T) {
	m := NewInterWorkloadAffinityManager()

	m.AddAffinity(InterWorkloadAffinity{TopologyKey: "zone-a"})
	m.AddAffinity(InterWorkloadAffinity{TopologyKey: "zone-b"})

	m.RemoveAffinity(0)

	affinities := m.GetAffinities()
	if len(affinities) != 1 {
		t.Errorf("expected 1 affinity after remove, got %d", len(affinities))
	}
	if affinities[0].TopologyKey != "zone-b" {
		t.Error("wrong affinity removed")
	}
}

func TestInterWorkloadAffinityManager_RemoveAffinity_OutOfBounds(t *testing.T) {
	m := NewInterWorkloadAffinityManager()

	m.AddAffinity(InterWorkloadAffinity{TopologyKey: "zone-a"})

	// Should not panic
	m.RemoveAffinity(-1)
	m.RemoveAffinity(5)

	affinities := m.GetAffinities()
	if len(affinities) != 1 {
		t.Errorf("expected 1 affinity (unchanged), got %d", len(affinities))
	}
}

func TestInterWorkloadAffinityManager_ConcurrentAccess(t *testing.T) {
	m := NewInterWorkloadAffinityManager()

	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			if i%2 == 0 {
				m.AddAffinity(InterWorkloadAffinity{
					TopologyKey: "zone-" + string(rune(i)),
				})
			} else {
				m.GetAffinities()
			}
		}(i)
	}

	wg.Wait()
}

// =============================================================================
// NodeAffinityBuilder Tests
// =============================================================================

func TestNodeAffinityBuilder_RequireMatchLabels(t *testing.T) {
	builder := NewNodeAffinityBuilder()

	affinity := builder.
		RequireMatchLabels(map[string]string{
			"zone": "zone-a",
			"tier": "premium",
		}).
		Build()

	if len(affinity.RequiredDuringScheduling) != 1 {
		t.Fatal("expected 1 required term")
	}

	term := affinity.RequiredDuringScheduling[0]
	if len(term.MatchExpressions) != 2 {
		t.Errorf("expected 2 expressions, got %d", len(term.MatchExpressions))
	}
}

func TestNodeAffinityBuilder_RequireMatchExpression(t *testing.T) {
	builder := NewNodeAffinityBuilder()

	affinity := builder.
		RequireMatchExpression("zone", LabelSelectorOpIn, "zone-a", "zone-b").
		RequireMatchExpression("tier", LabelSelectorOpExists).
		Build()

	if len(affinity.RequiredDuringScheduling) != 1 {
		t.Fatal("expected 1 required term")
	}

	term := affinity.RequiredDuringScheduling[0]
	if len(term.MatchExpressions) != 2 {
		t.Errorf("expected 2 expressions, got %d", len(term.MatchExpressions))
	}
}

func TestNodeAffinityBuilder_PreferMatchLabels(t *testing.T) {
	builder := NewNodeAffinityBuilder()

	affinity := builder.
		PreferMatchLabels(map[string]string{"zone": "zone-a"}, 50).
		PreferMatchLabels(map[string]string{"tier": "premium"}, 30).
		Build()

	if len(affinity.PreferredDuringScheduling) != 2 {
		t.Errorf("expected 2 preferred terms, got %d", len(affinity.PreferredDuringScheduling))
	}

	if affinity.PreferredDuringScheduling[0].Weight != 50 {
		t.Errorf("expected weight 50, got %d", affinity.PreferredDuringScheduling[0].Weight)
	}
}

// =============================================================================
// AffinitySpecBuilder Tests
// =============================================================================

func TestAffinitySpecBuilder_Complete(t *testing.T) {
	nodeAffinity := NewNodeAffinityBuilder().
		RequireMatchLabels(map[string]string{"zone": "zone-a"}).
		Build()

	workloadAffinity := &WorkloadAffinity{
		RequiredDuringScheduling: []WorkloadAffinityTerm{
			{
				LabelSelector: LabelSelector{
					MatchLabels: map[string]string{"app": "cache"},
				},
			},
		},
	}

	workloadAntiAffinity := &WorkloadAntiAffinity{
		RequiredDuringScheduling: []WorkloadAffinityTerm{
			{
				LabelSelector: LabelSelector{
					MatchLabels: map[string]string{"app": "noisy"},
				},
			},
		},
	}

	spec := NewAffinitySpecBuilder().
		WithNodeAffinity(nodeAffinity).
		WithWorkloadAffinity(workloadAffinity).
		WithWorkloadAntiAffinity(workloadAntiAffinity).
		Build()

	if spec.NodeAffinity == nil {
		t.Error("expected node affinity")
	}
	if spec.WorkloadAffinity == nil {
		t.Error("expected workload affinity")
	}
	if spec.WorkloadAntiAffinity == nil {
		t.Error("expected workload anti-affinity")
	}
}

// =============================================================================
// Label Matching Tests
// =============================================================================

func TestLabelMatching_MatchLabels(t *testing.T) {
	ac := NewAffinityChecker()

	w := createTestWorkload("w1", 100, 100*1024*1024)
	w.Labels = map[string]string{
		"app":     "myapp",
		"version": "v1",
		"extra":   "label",
	}

	selector := LabelSelector{
		MatchLabels: map[string]string{
			"app":     "myapp",
			"version": "v1",
		},
	}

	if !ac.matchesLabelSelector(w.Labels, selector) {
		t.Error("expected labels to match")
	}
}

func TestLabelMatching_MatchLabels_Missing(t *testing.T) {
	ac := NewAffinityChecker()

	w := createTestWorkload("w1", 100, 100*1024*1024)
	w.Labels = map[string]string{
		"app": "myapp",
	}

	selector := LabelSelector{
		MatchLabels: map[string]string{
			"app":     "myapp",
			"version": "v1", // Missing from workload
		},
	}

	if ac.matchesLabelSelector(w.Labels, selector) {
		t.Error("expected labels not to match")
	}
}

func TestLabelMatching_MatchExpressions(t *testing.T) {
	ac := NewAffinityChecker()

	w := createTestWorkload("w1", 100, 100*1024*1024)
	w.Labels = map[string]string{
		"app":     "myapp",
		"version": "v1",
	}

	selector := LabelSelector{
		MatchExpressions: []LabelSelectorRequirement{
			{
				Key:      "app",
				Operator: LabelSelectorOpIn,
				Values:   []string{"myapp", "otherapp"},
			},
			{
				Key:      "version",
				Operator: LabelSelectorOpExists,
			},
		},
	}

	if !ac.matchesLabelSelector(w.Labels, selector) {
		t.Error("expected labels to match expressions")
	}
}

func TestLabelMatching_Combined(t *testing.T) {
	ac := NewAffinityChecker()

	w := createTestWorkload("w1", 100, 100*1024*1024)
	w.Labels = map[string]string{
		"app":     "myapp",
		"version": "v1",
		"tier":    "frontend",
	}

	selector := LabelSelector{
		MatchLabels: map[string]string{
			"app": "myapp",
		},
		MatchExpressions: []LabelSelectorRequirement{
			{
				Key:      "tier",
				Operator: LabelSelectorOpIn,
				Values:   []string{"frontend", "backend"},
			},
		},
	}

	if !ac.matchesLabelSelector(w.Labels, selector) {
		t.Error("expected combined selector to match")
	}
}

// =============================================================================
// Field Selector Tests
// =============================================================================

func TestFieldSelector_Zone(t *testing.T) {
	ac := NewAffinityChecker()

	node := createTestNode("node-1", 1000, 1000*1024*1024)
	node.Zone = "us-east-1a"

	req := FieldSelectorRequirement{
		Key:      "spec.zone",
		Operator: "=",
		Value:    "us-east-1a",
	}

	if !ac.matchesFieldRequirement(node, req) {
		t.Error("expected zone to match")
	}
}

func TestFieldSelector_Region(t *testing.T) {
	ac := NewAffinityChecker()

	node := createTestNode("node-1", 1000, 1000*1024*1024)
	node.Region = "us-east-1"

	req := FieldSelectorRequirement{
		Key:      "spec.region",
		Operator: "==",
		Value:    "us-east-1",
	}

	if !ac.matchesFieldRequirement(node, req) {
		t.Error("expected region to match")
	}
}

func TestFieldSelector_NodeType(t *testing.T) {
	ac := NewAffinityChecker()

	node := createTestNode("node-1", 1000, 1000*1024*1024)
	node.NodeType = "gpu"

	req := FieldSelectorRequirement{
		Key:      "spec.nodeType",
		Operator: "!=",
		Value:    "cpu",
	}

	if !ac.matchesFieldRequirement(node, req) {
		t.Error("expected nodeType != cpu to match")
	}
}

func TestFieldSelector_UnknownField(t *testing.T) {
	ac := NewAffinityChecker()

	node := createTestNode("node-1", 1000, 1000*1024*1024)

	req := FieldSelectorRequirement{
		Key:      "spec.unknown",
		Operator: "=",
		Value:    "value",
	}

	if ac.matchesFieldRequirement(node, req) {
		t.Error("expected unknown field to not match")
	}
}

func TestFieldSelector_UnknownOperator(t *testing.T) {
	ac := NewAffinityChecker()

	node := createTestNode("node-1", 1000, 1000*1024*1024)
	node.Zone = "zone-a"

	req := FieldSelectorRequirement{
		Key:      "spec.zone",
		Operator: "~=", // Unknown operator
		Value:    "zone-a",
	}

	if ac.matchesFieldRequirement(node, req) {
		t.Error("expected unknown operator to not match")
	}
}

// =============================================================================
// Edge Cases
// =============================================================================

func TestAffinity_EmptyLabels(t *testing.T) {
	ac := NewAffinityChecker()

	w := createTestWorkload("w1", 100, 100*1024*1024)
	w.Labels = nil // No labels

	selector := LabelSelector{
		MatchLabels: map[string]string{},
	}

	// Empty match should succeed
	if !ac.matchesLabelSelector(w.Labels, selector) {
		t.Error("expected empty match to succeed")
	}
}

func TestAffinity_NilAffinitySpec(t *testing.T) {
	ac := NewAffinityChecker()

	w := createTestWorkload("w1", 100, 100*1024*1024)
	w.Affinity = &AffinitySpec{
		NodeAffinity:         nil,
		WorkloadAffinity:     nil,
		WorkloadAntiAffinity: nil,
	}

	node := createTestNode("node-1", 1000, 1000*1024*1024)

	satisfied, score := ac.CheckNodeAffinity(w, node)
	if !satisfied {
		t.Error("expected satisfied with nil node affinity")
	}
	if score != 0 {
		t.Errorf("expected score 0, got %d", score)
	}
}

func TestAffinity_MultipleRequiredTerms_OR(t *testing.T) {
	ac := NewAffinityChecker()

	// Multiple required terms are OR'd together
	w := createTestWorkload("w1", 100, 100*1024*1024)
	w.Affinity = &AffinitySpec{
		NodeAffinity: &NodeAffinity{
			RequiredDuringScheduling: []NodeSelectorTerm{
				{
					MatchExpressions: []LabelSelectorRequirement{
						{Key: "zone", Operator: LabelSelectorOpIn, Values: []string{"zone-a"}},
					},
				},
				{
					MatchExpressions: []LabelSelectorRequirement{
						{Key: "zone", Operator: LabelSelectorOpIn, Values: []string{"zone-b"}},
					},
				},
			},
		},
	}

	// Node in zone-b should match (second term)
	node := createTestNode("node-1", 1000, 1000*1024*1024)
	node.Labels["zone"] = "zone-b"

	satisfied, _ := ac.CheckNodeAffinity(w, node)
	if !satisfied {
		t.Error("expected satisfied - should match second term")
	}
}

func TestAffinity_MultipleExpressions_AND(t *testing.T) {
	ac := NewAffinityChecker()

	// Multiple expressions within a term are AND'd together
	w := createTestWorkload("w1", 100, 100*1024*1024)
	w.Affinity = &AffinitySpec{
		NodeAffinity: &NodeAffinity{
			RequiredDuringScheduling: []NodeSelectorTerm{
				{
					MatchExpressions: []LabelSelectorRequirement{
						{Key: "zone", Operator: LabelSelectorOpIn, Values: []string{"zone-a"}},
						{Key: "tier", Operator: LabelSelectorOpIn, Values: []string{"premium"}},
					},
				},
			},
		},
	}

	// Node only has zone-a but not premium tier
	node := createTestNode("node-1", 1000, 1000*1024*1024)
	node.Labels["zone"] = "zone-a"
	node.Labels["tier"] = "standard"

	satisfied, _ := ac.CheckNodeAffinity(w, node)
	if satisfied {
		t.Error("expected not satisfied - both expressions must match")
	}
}

// =============================================================================
// Performance Test
// =============================================================================

func TestAffinity_LargeScale(t *testing.T) {
	ac := NewAffinityChecker()

	// Add many workloads
	for i := 0; i < 1000; i++ {
		w := createTestWorkload("w-"+string(rune(i)), 100, 100*1024*1024)
		w.Labels["app"] = "myapp"
		w.Labels["version"] = "v" + string(rune(i%10))
		w.Namespace = "ns-" + string(rune(i%5))
		ac.AddWorkload(w, "node-"+string(rune(i%10)))
	}

	// Create workload with complex affinity
	w := createTestWorkload("test", 100, 100*1024*1024)
	w.Namespace = "ns-1"
	w.Affinity = &AffinitySpec{
		NodeAffinity: &NodeAffinity{
			RequiredDuringScheduling: []NodeSelectorTerm{
				{
					MatchExpressions: []LabelSelectorRequirement{
						{Key: "zone", Operator: LabelSelectorOpIn, Values: []string{"zone-a", "zone-b"}},
					},
				},
			},
			PreferredDuringScheduling: []PreferredSchedulingTerm{
				{Weight: 50, Preference: NodeSelectorTerm{
					MatchExpressions: []LabelSelectorRequirement{
						{Key: "tier", Operator: LabelSelectorOpIn, Values: []string{"premium"}},
					},
				}},
			},
		},
		WorkloadAffinity: &WorkloadAffinity{
			PreferredDuringScheduling: []WeightedWorkloadAffinityTerm{
				{Weight: 30, AffinityTerm: WorkloadAffinityTerm{
					LabelSelector: LabelSelector{MatchLabels: map[string]string{"app": "myapp"}},
				}},
			},
		},
	}

	node := createTestNode("node-1", 1000, 1000*1024*1024)
	node.Labels["zone"] = "zone-a"
	node.Labels["tier"] = "premium"

	start := time.Now()

	// Check all affinity types
	ac.CheckNodeAffinity(w, node)
	ac.CheckWorkloadAffinity(w, node)
	ac.CheckWorkloadAntiAffinity(w, node)

	duration := time.Since(start)

	// Should complete quickly even with many workloads
	if duration > 100*time.Millisecond {
		t.Errorf("affinity checking took too long: %v", duration)
	}
}
