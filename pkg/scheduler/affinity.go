// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package scheduler

import (
	"sync"
)

// AffinitySpec defines affinity and anti-affinity rules.
type AffinitySpec struct {
	NodeAffinity         *NodeAffinity
	WorkloadAffinity     *WorkloadAffinity
	WorkloadAntiAffinity *WorkloadAntiAffinity
}

// NodeAffinity defines node affinity rules.
type NodeAffinity struct {
	RequiredDuringScheduling  []NodeSelectorTerm
	PreferredDuringScheduling []PreferredSchedulingTerm
}

// NodeSelectorTerm defines a node selector term.
type NodeSelectorTerm struct {
	MatchExpressions []LabelSelectorRequirement
	MatchFields      []FieldSelectorRequirement
}

// PreferredSchedulingTerm defines a weighted preference.
type PreferredSchedulingTerm struct {
	Weight     int32
	Preference NodeSelectorTerm
}

// LabelSelectorRequirement defines a label requirement.
type LabelSelectorRequirement struct {
	Key      string
	Operator LabelSelectorOperator
	Values   []string
}

// LabelSelectorOperator defines label selector operators.
type LabelSelectorOperator string

const (
	LabelSelectorOpIn           LabelSelectorOperator = "In"
	LabelSelectorOpNotIn        LabelSelectorOperator = "NotIn"
	LabelSelectorOpExists       LabelSelectorOperator = "Exists"
	LabelSelectorOpDoesNotExist LabelSelectorOperator = "DoesNotExist"
)

// FieldSelectorRequirement defines a field requirement.
type FieldSelectorRequirement struct {
	Key      string
	Operator string
	Value    string
}

// WorkloadAffinity defines workload affinity rules.
type WorkloadAffinity struct {
	RequiredDuringScheduling  []WorkloadAffinityTerm
	PreferredDuringScheduling []WeightedWorkloadAffinityTerm
}

// WorkloadAntiAffinity defines workload anti-affinity rules.
type WorkloadAntiAffinity struct {
	RequiredDuringScheduling  []WorkloadAffinityTerm
	PreferredDuringScheduling []WeightedWorkloadAffinityTerm
}

// WorkloadAffinityTerm defines a workload affinity term.
type WorkloadAffinityTerm struct {
	LabelSelector LabelSelector
	Namespaces    []string
	TopologyKey   string
}

// WeightedWorkloadAffinityTerm adds weight to affinity term.
type WeightedWorkloadAffinityTerm struct {
	Weight       int32
	AffinityTerm WorkloadAffinityTerm
}

// LabelSelector selects workloads by labels.
type LabelSelector struct {
	MatchLabels      map[string]string
	MatchExpressions []LabelSelectorRequirement
}

// AffinityChecker evaluates affinity rules for scheduling.
type AffinityChecker struct {
	mu                sync.RWMutex
	nodeWorkloads     map[string][]*Workload
	workloadsByLabel  map[string]map[string][]*Workload // label key -> value -> workloads
}

// NewAffinityChecker creates a new affinity checker.
func NewAffinityChecker() *AffinityChecker {
	return &AffinityChecker{
		nodeWorkloads:    make(map[string][]*Workload),
		workloadsByLabel: make(map[string]map[string][]*Workload),
	}
}

// AddWorkload adds a workload to the tracker.
func (ac *AffinityChecker) AddWorkload(workload *Workload, nodeID string) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	ac.nodeWorkloads[nodeID] = append(ac.nodeWorkloads[nodeID], workload)

	for k, v := range workload.Labels {
		if ac.workloadsByLabel[k] == nil {
			ac.workloadsByLabel[k] = make(map[string][]*Workload)
		}
		ac.workloadsByLabel[k][v] = append(ac.workloadsByLabel[k][v], workload)
	}
}

// RemoveWorkload removes a workload from the tracker.
func (ac *AffinityChecker) RemoveWorkload(workload *Workload, nodeID string) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	// Remove from node workloads
	workloads := ac.nodeWorkloads[nodeID]
	for i, w := range workloads {
		if w.ID == workload.ID {
			ac.nodeWorkloads[nodeID] = append(workloads[:i], workloads[i+1:]...)
			break
		}
	}

	// Remove from label index
	for k, v := range workload.Labels {
		if ac.workloadsByLabel[k] != nil {
			labelWorkloads := ac.workloadsByLabel[k][v]
			for i, w := range labelWorkloads {
				if w.ID == workload.ID {
					ac.workloadsByLabel[k][v] = append(labelWorkloads[:i], labelWorkloads[i+1:]...)
					break
				}
			}
		}
	}
}

// CheckNodeAffinity checks if a node satisfies affinity requirements.
func (ac *AffinityChecker) CheckNodeAffinity(workload *Workload, node *Node) (bool, int64) {
	if workload.Affinity == nil || workload.Affinity.NodeAffinity == nil {
		return true, 0
	}

	na := workload.Affinity.NodeAffinity

	// Check required terms
	if len(na.RequiredDuringScheduling) > 0 {
		matched := false
		for _, term := range na.RequiredDuringScheduling {
			if ac.matchesNodeSelectorTerm(node, term) {
				matched = true
				break
			}
		}
		if !matched {
			return false, 0
		}
	}

	// Calculate preferred score
	var score int64
	for _, pref := range na.PreferredDuringScheduling {
		if ac.matchesNodeSelectorTerm(node, pref.Preference) {
			score += int64(pref.Weight)
		}
	}

	return true, score
}

// matchesNodeSelectorTerm checks if a node matches a selector term.
func (ac *AffinityChecker) matchesNodeSelectorTerm(node *Node, term NodeSelectorTerm) bool {
	// Check match expressions
	for _, req := range term.MatchExpressions {
		if !ac.matchesLabelRequirement(node.Labels, req) {
			return false
		}
	}

	// Check field requirements
	for _, req := range term.MatchFields {
		if !ac.matchesFieldRequirement(node, req) {
			return false
		}
	}

	return true
}

// matchesLabelRequirement checks if labels match a requirement.
func (ac *AffinityChecker) matchesLabelRequirement(labels map[string]string, req LabelSelectorRequirement) bool {
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

	default:
		return false
	}
}

// matchesFieldRequirement checks if a node field matches.
func (ac *AffinityChecker) matchesFieldRequirement(node *Node, req FieldSelectorRequirement) bool {
	var fieldValue string

	switch req.Key {
	case "metadata.name":
		fieldValue = node.Name
	case "spec.zone":
		fieldValue = node.Zone
	case "spec.region":
		fieldValue = node.Region
	case "spec.nodeType":
		fieldValue = node.NodeType
	default:
		return false
	}

	switch req.Operator {
	case "=", "==":
		return fieldValue == req.Value
	case "!=":
		return fieldValue != req.Value
	default:
		return false
	}
}

// CheckWorkloadAffinity checks workload affinity rules.
func (ac *AffinityChecker) CheckWorkloadAffinity(workload *Workload, node *Node) (bool, int64) {
	if workload.Affinity == nil || workload.Affinity.WorkloadAffinity == nil {
		return true, 0
	}

	wa := workload.Affinity.WorkloadAffinity

	ac.mu.RLock()
	nodeWorkloads := ac.nodeWorkloads[node.ID]
	ac.mu.RUnlock()

	// Check required terms
	for _, term := range wa.RequiredDuringScheduling {
		if !ac.hasMatchingWorkload(nodeWorkloads, term, workload.Namespace) {
			return false, 0
		}
	}

	// Calculate preferred score
	var score int64
	for _, pref := range wa.PreferredDuringScheduling {
		if ac.hasMatchingWorkload(nodeWorkloads, pref.AffinityTerm, workload.Namespace) {
			score += int64(pref.Weight)
		}
	}

	return true, score
}

// CheckWorkloadAntiAffinity checks workload anti-affinity rules.
func (ac *AffinityChecker) CheckWorkloadAntiAffinity(workload *Workload, node *Node) (bool, int64) {
	if workload.Affinity == nil || workload.Affinity.WorkloadAntiAffinity == nil {
		return true, 0
	}

	waa := workload.Affinity.WorkloadAntiAffinity

	ac.mu.RLock()
	nodeWorkloads := ac.nodeWorkloads[node.ID]
	ac.mu.RUnlock()

	// Check required terms - must NOT have matching workloads
	for _, term := range waa.RequiredDuringScheduling {
		if ac.hasMatchingWorkload(nodeWorkloads, term, workload.Namespace) {
			return false, 0
		}
	}

	// Calculate preferred score - higher if no matching workloads
	var score int64
	for _, pref := range waa.PreferredDuringScheduling {
		if !ac.hasMatchingWorkload(nodeWorkloads, pref.AffinityTerm, workload.Namespace) {
			score += int64(pref.Weight)
		}
	}

	return true, score
}

// hasMatchingWorkload checks if any workload matches the affinity term.
func (ac *AffinityChecker) hasMatchingWorkload(workloads []*Workload, term WorkloadAffinityTerm, namespace string) bool {
	for _, w := range workloads {
		// Check namespace
		if len(term.Namespaces) > 0 {
			found := false
			for _, ns := range term.Namespaces {
				if ns == w.Namespace {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		} else if w.Namespace != namespace {
			continue
		}

		// Check label selector
		if ac.matchesLabelSelector(w.Labels, term.LabelSelector) {
			return true
		}
	}
	return false
}

// matchesLabelSelector checks if labels match a selector.
func (ac *AffinityChecker) matchesLabelSelector(labels map[string]string, selector LabelSelector) bool {
	// Check match labels
	for k, v := range selector.MatchLabels {
		if labels[k] != v {
			return false
		}
	}

	// Check match expressions
	for _, req := range selector.MatchExpressions {
		if !ac.matchesLabelRequirement(labels, req) {
			return false
		}
	}

	return true
}

// GetWorkloadsOnNode returns all workloads on a node.
func (ac *AffinityChecker) GetWorkloadsOnNode(nodeID string) []*Workload {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	result := make([]*Workload, len(ac.nodeWorkloads[nodeID]))
	copy(result, ac.nodeWorkloads[nodeID])
	return result
}

// GetWorkloadsByLabel returns workloads matching a label.
func (ac *AffinityChecker) GetWorkloadsByLabel(key, value string) []*Workload {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	if ac.workloadsByLabel[key] == nil {
		return nil
	}

	result := make([]*Workload, len(ac.workloadsByLabel[key][value]))
	copy(result, ac.workloadsByLabel[key][value])
	return result
}

// TopologySpreadConstraint defines workload spreading across topology.
type TopologySpreadConstraint struct {
	MaxSkew           int32
	TopologyKey       string
	WhenUnsatisfiable UnsatisfiableAction
	LabelSelector     LabelSelector
}

// UnsatisfiableAction defines behavior when constraints can't be met.
type UnsatisfiableAction string

const (
	DoNotSchedule  UnsatisfiableAction = "DoNotSchedule"
	ScheduleAnyway UnsatisfiableAction = "ScheduleAnyway"
)

// TopologySpreadChecker evaluates topology spread constraints.
type TopologySpreadChecker struct {
	mu                sync.RWMutex
	nodeTopology      map[string]map[string]string // nodeID -> topologyKey -> value
	topologyWorkloads map[string]map[string]int    // topologyKey -> value -> count
}

// NewTopologySpreadChecker creates a new topology spread checker.
func NewTopologySpreadChecker() *TopologySpreadChecker {
	return &TopologySpreadChecker{
		nodeTopology:      make(map[string]map[string]string),
		topologyWorkloads: make(map[string]map[string]int),
	}
}

// RegisterNode registers a node's topology.
func (tc *TopologySpreadChecker) RegisterNode(node *Node) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.nodeTopology[node.ID] = make(map[string]string)
	for k, v := range node.Labels {
		tc.nodeTopology[node.ID][k] = v
	}
}

// UnregisterNode removes a node.
func (tc *TopologySpreadChecker) UnregisterNode(nodeID string) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	delete(tc.nodeTopology, nodeID)
}

// RecordWorkloadPlacement records a workload placement.
func (tc *TopologySpreadChecker) RecordWorkloadPlacement(workload *Workload, nodeID string) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	topology := tc.nodeTopology[nodeID]
	for key, value := range topology {
		if tc.topologyWorkloads[key] == nil {
			tc.topologyWorkloads[key] = make(map[string]int)
		}
		tc.topologyWorkloads[key][value]++
	}
}

// RemoveWorkloadPlacement removes a workload placement record.
func (tc *TopologySpreadChecker) RemoveWorkloadPlacement(workload *Workload, nodeID string) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	topology := tc.nodeTopology[nodeID]
	for key, value := range topology {
		if tc.topologyWorkloads[key] != nil {
			tc.topologyWorkloads[key][value]--
			if tc.topologyWorkloads[key][value] <= 0 {
				delete(tc.topologyWorkloads[key], value)
			}
		}
	}
}

// CheckConstraints evaluates topology spread constraints.
func (tc *TopologySpreadChecker) CheckConstraints(workload *Workload, node *Node, constraints []TopologySpreadConstraint) (bool, int64) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	var totalScore int64
	topology := tc.nodeTopology[node.ID]

	for _, constraint := range constraints {
		value := topology[constraint.TopologyKey]
		if value == "" {
			if constraint.WhenUnsatisfiable == DoNotSchedule {
				return false, 0
			}
			continue
		}

		counts := tc.topologyWorkloads[constraint.TopologyKey]
		if counts == nil {
			counts = make(map[string]int)
		}

		// Find min count
		minCount := int(^uint(0) >> 1)
		for _, c := range counts {
			if c < minCount {
				minCount = c
			}
		}
		if minCount == int(^uint(0)>>1) {
			minCount = 0
		}

		currentCount := counts[value]
		skew := int32(currentCount - minCount)

		if skew >= constraint.MaxSkew {
			if constraint.WhenUnsatisfiable == DoNotSchedule {
				return false, 0
			}
		}

		// Score based on inverse skew
		score := int64(100 - skew*10)
		if score < 0 {
			score = 0
		}
		totalScore += score
	}

	return true, totalScore
}

// InterWorkloadAffinity represents affinity between two workloads.
type InterWorkloadAffinity struct {
	Source      LabelSelector
	Target      LabelSelector
	Type        AffinityType
	TopologyKey string
}

// AffinityType defines the type of affinity.
type AffinityType string

const (
	AffinityTypePreferred AffinityType = "Preferred"
	AffinityTypeRequired  AffinityType = "Required"
)

// InterWorkloadAffinityManager manages inter-workload affinities.
type InterWorkloadAffinityManager struct {
	mu        sync.RWMutex
	affinities []InterWorkloadAffinity
}

// NewInterWorkloadAffinityManager creates a new manager.
func NewInterWorkloadAffinityManager() *InterWorkloadAffinityManager {
	return &InterWorkloadAffinityManager{
		affinities: []InterWorkloadAffinity{},
	}
}

// AddAffinity adds an inter-workload affinity rule.
func (m *InterWorkloadAffinityManager) AddAffinity(affinity InterWorkloadAffinity) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.affinities = append(m.affinities, affinity)
}

// RemoveAffinity removes an affinity rule.
func (m *InterWorkloadAffinityManager) RemoveAffinity(index int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if index >= 0 && index < len(m.affinities) {
		m.affinities = append(m.affinities[:index], m.affinities[index+1:]...)
	}
}

// GetAffinities returns all affinity rules.
func (m *InterWorkloadAffinityManager) GetAffinities() []InterWorkloadAffinity {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]InterWorkloadAffinity, len(m.affinities))
	copy(result, m.affinities)
	return result
}

// NodeAffinityBuilder helps build node affinity specs.
type NodeAffinityBuilder struct {
	affinity *NodeAffinity
}

// NewNodeAffinityBuilder creates a new builder.
func NewNodeAffinityBuilder() *NodeAffinityBuilder {
	return &NodeAffinityBuilder{
		affinity: &NodeAffinity{},
	}
}

// RequireMatchLabels adds required label matching.
func (b *NodeAffinityBuilder) RequireMatchLabels(labels map[string]string) *NodeAffinityBuilder {
	var expressions []LabelSelectorRequirement
	for k, v := range labels {
		expressions = append(expressions, LabelSelectorRequirement{
			Key:      k,
			Operator: LabelSelectorOpIn,
			Values:   []string{v},
		})
	}
	b.affinity.RequiredDuringScheduling = append(b.affinity.RequiredDuringScheduling, NodeSelectorTerm{
		MatchExpressions: expressions,
	})
	return b
}

// RequireMatchExpression adds a required expression.
func (b *NodeAffinityBuilder) RequireMatchExpression(key string, op LabelSelectorOperator, values ...string) *NodeAffinityBuilder {
	req := LabelSelectorRequirement{
		Key:      key,
		Operator: op,
		Values:   values,
	}
	if len(b.affinity.RequiredDuringScheduling) == 0 {
		b.affinity.RequiredDuringScheduling = append(b.affinity.RequiredDuringScheduling, NodeSelectorTerm{})
	}
	last := len(b.affinity.RequiredDuringScheduling) - 1
	b.affinity.RequiredDuringScheduling[last].MatchExpressions = append(
		b.affinity.RequiredDuringScheduling[last].MatchExpressions, req)
	return b
}

// PreferMatchLabels adds preferred label matching.
func (b *NodeAffinityBuilder) PreferMatchLabels(labels map[string]string, weight int32) *NodeAffinityBuilder {
	var expressions []LabelSelectorRequirement
	for k, v := range labels {
		expressions = append(expressions, LabelSelectorRequirement{
			Key:      k,
			Operator: LabelSelectorOpIn,
			Values:   []string{v},
		})
	}
	b.affinity.PreferredDuringScheduling = append(b.affinity.PreferredDuringScheduling, PreferredSchedulingTerm{
		Weight: weight,
		Preference: NodeSelectorTerm{
			MatchExpressions: expressions,
		},
	})
	return b
}

// Build returns the built node affinity.
func (b *NodeAffinityBuilder) Build() *NodeAffinity {
	return b.affinity
}

// AffinitySpecBuilder helps build complete affinity specs.
type AffinitySpecBuilder struct {
	spec *AffinitySpec
}

// NewAffinitySpecBuilder creates a new builder.
func NewAffinitySpecBuilder() *AffinitySpecBuilder {
	return &AffinitySpecBuilder{
		spec: &AffinitySpec{},
	}
}

// WithNodeAffinity sets node affinity.
func (b *AffinitySpecBuilder) WithNodeAffinity(na *NodeAffinity) *AffinitySpecBuilder {
	b.spec.NodeAffinity = na
	return b
}

// WithWorkloadAffinity sets workload affinity.
func (b *AffinitySpecBuilder) WithWorkloadAffinity(wa *WorkloadAffinity) *AffinitySpecBuilder {
	b.spec.WorkloadAffinity = wa
	return b
}

// WithWorkloadAntiAffinity sets workload anti-affinity.
func (b *AffinitySpecBuilder) WithWorkloadAntiAffinity(waa *WorkloadAntiAffinity) *AffinitySpecBuilder {
	b.spec.WorkloadAntiAffinity = waa
	return b
}

// Build returns the built affinity spec.
func (b *AffinitySpecBuilder) Build() *AffinitySpec {
	return b.spec
}
