// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package scheduler

import (
	"math/rand"
	"sort"
	"sync"
	"time"
)

// AlgorithmType represents the type of scheduling algorithm.
type AlgorithmType string

const (
	AlgorithmBinPack  AlgorithmType = "BinPack"
	AlgorithmSpread   AlgorithmType = "Spread"
	AlgorithmRandom   AlgorithmType = "Random"
	AlgorithmBalanced AlgorithmType = "Balanced"
)

// Algorithm is the interface for scheduling algorithms.
type Algorithm interface {
	// Name returns the algorithm name.
	Name() string
	// SelectNode selects the best node for a workload.
	SelectNode(workload *Workload, nodes []*Node) *Node
	// ScoreNodes scores all nodes for a workload.
	ScoreNodes(workload *Workload, nodes []*Node) []NodeScore
}

// NodeScore represents a node's score.
type NodeScore struct {
	Node  *Node
	Score int64
}

// BinPackAlgorithm implements bin packing scheduling.
// It tries to pack workloads onto as few nodes as possible.
type BinPackAlgorithm struct {
	mu           sync.Mutex
	cpuWeight    float64
	memoryWeight float64
	diskWeight   float64
}

// NewBinPackAlgorithm creates a new bin pack algorithm.
func NewBinPackAlgorithm() *BinPackAlgorithm {
	return &BinPackAlgorithm{
		cpuWeight:    1.0,
		memoryWeight: 1.0,
		diskWeight:   0.5,
	}
}

// Name returns the algorithm name.
func (a *BinPackAlgorithm) Name() string {
	return string(AlgorithmBinPack)
}

// SetWeights sets the resource weights.
func (a *BinPackAlgorithm) SetWeights(cpu, memory, disk float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cpuWeight = cpu
	a.memoryWeight = memory
	a.diskWeight = disk
}

// SelectNode selects the node with highest utilization that fits.
func (a *BinPackAlgorithm) SelectNode(workload *Workload, nodes []*Node) *Node {
	scores := a.ScoreNodes(workload, nodes)
	if len(scores) == 0 {
		return nil
	}

	// Sort by score descending
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})

	return scores[0].Node
}

// ScoreNodes calculates bin pack scores for all nodes.
func (a *BinPackAlgorithm) ScoreNodes(workload *Workload, nodes []*Node) []NodeScore {
	a.mu.Lock()
	cpuW := a.cpuWeight
	memW := a.memoryWeight
	diskW := a.diskWeight
	a.mu.Unlock()

	var scores []NodeScore

	for _, node := range nodes {
		if !workload.Resources.Requests.FitsIn(node.Available) {
			continue
		}

		// Calculate utilization after scheduling
		afterCPU := node.Allocated.CPU + workload.Resources.Requests.CPU
		afterMem := node.Allocated.Memory + workload.Resources.Requests.Memory
		afterDisk := node.Allocated.Disk + workload.Resources.Requests.Disk

		cpuUtil := float64(afterCPU) / float64(node.Allocatable.CPU)
		memUtil := float64(afterMem) / float64(node.Allocatable.Memory)
		diskUtil := float64(0)
		if node.Allocatable.Disk > 0 {
			diskUtil = float64(afterDisk) / float64(node.Allocatable.Disk)
		}

		// Higher utilization = higher score for bin packing
		score := int64((cpuUtil*cpuW + memUtil*memW + diskUtil*diskW) * 100)

		scores = append(scores, NodeScore{
			Node:  node,
			Score: score,
		})
	}

	return scores
}

// SpreadAlgorithm implements spread scheduling.
// It tries to spread workloads across nodes evenly.
type SpreadAlgorithm struct {
	mu           sync.Mutex
	cpuWeight    float64
	memoryWeight float64
	zoneSpread   bool
}

// NewSpreadAlgorithm creates a new spread algorithm.
func NewSpreadAlgorithm() *SpreadAlgorithm {
	return &SpreadAlgorithm{
		cpuWeight:    1.0,
		memoryWeight: 1.0,
		zoneSpread:   true,
	}
}

// Name returns the algorithm name.
func (a *SpreadAlgorithm) Name() string {
	return string(AlgorithmSpread)
}

// SetWeights sets the resource weights.
func (a *SpreadAlgorithm) SetWeights(cpu, memory float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cpuWeight = cpu
	a.memoryWeight = memory
}

// SetZoneSpread enables or disables zone spreading.
func (a *SpreadAlgorithm) SetZoneSpread(enabled bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.zoneSpread = enabled
}

// SelectNode selects the node with lowest utilization.
func (a *SpreadAlgorithm) SelectNode(workload *Workload, nodes []*Node) *Node {
	scores := a.ScoreNodes(workload, nodes)
	if len(scores) == 0 {
		return nil
	}

	// Sort by score descending
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})

	return scores[0].Node
}

// ScoreNodes calculates spread scores for all nodes.
func (a *SpreadAlgorithm) ScoreNodes(workload *Workload, nodes []*Node) []NodeScore {
	a.mu.Lock()
	cpuW := a.cpuWeight
	memW := a.memoryWeight
	a.mu.Unlock()

	var scores []NodeScore

	for _, node := range nodes {
		if !workload.Resources.Requests.FitsIn(node.Available) {
			continue
		}

		// Lower utilization = higher score for spreading
		cpuUtil := float64(node.Allocated.CPU) / float64(node.Allocatable.CPU)
		memUtil := float64(node.Allocated.Memory) / float64(node.Allocatable.Memory)

		// Invert utilization for spread
		score := int64(((1-cpuUtil)*cpuW + (1-memUtil)*memW) * 100)

		scores = append(scores, NodeScore{
			Node:  node,
			Score: score,
		})
	}

	return scores
}

// RandomAlgorithm implements random scheduling.
type RandomAlgorithm struct {
	rng *rand.Rand
	mu  sync.Mutex
}

// NewRandomAlgorithm creates a new random algorithm.
func NewRandomAlgorithm() *RandomAlgorithm {
	return &RandomAlgorithm{
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Name returns the algorithm name.
func (a *RandomAlgorithm) Name() string {
	return string(AlgorithmRandom)
}

// SelectNode randomly selects a feasible node.
func (a *RandomAlgorithm) SelectNode(workload *Workload, nodes []*Node) *Node {
	var feasible []*Node
	for _, node := range nodes {
		if workload.Resources.Requests.FitsIn(node.Available) {
			feasible = append(feasible, node)
		}
	}

	if len(feasible) == 0 {
		return nil
	}

	a.mu.Lock()
	idx := a.rng.Intn(len(feasible))
	a.mu.Unlock()

	return feasible[idx]
}

// ScoreNodes returns random scores for nodes.
func (a *RandomAlgorithm) ScoreNodes(workload *Workload, nodes []*Node) []NodeScore {
	a.mu.Lock()
	defer a.mu.Unlock()

	var scores []NodeScore
	for _, node := range nodes {
		if workload.Resources.Requests.FitsIn(node.Available) {
			scores = append(scores, NodeScore{
				Node:  node,
				Score: int64(a.rng.Intn(100)),
			})
		}
	}
	return scores
}

// BalancedAlgorithm balances between bin packing and spreading.
type BalancedAlgorithm struct {
	mu           sync.Mutex
	binPackRatio float64 // 0.0 = full spread, 1.0 = full bin pack
}

// NewBalancedAlgorithm creates a new balanced algorithm.
func NewBalancedAlgorithm(ratio float64) *BalancedAlgorithm {
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	return &BalancedAlgorithm{
		binPackRatio: ratio,
	}
}

// Name returns the algorithm name.
func (a *BalancedAlgorithm) Name() string {
	return string(AlgorithmBalanced)
}

// SetRatio sets the bin pack ratio.
func (a *BalancedAlgorithm) SetRatio(ratio float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	a.binPackRatio = ratio
}

// SelectNode selects a node balancing bin pack and spread.
func (a *BalancedAlgorithm) SelectNode(workload *Workload, nodes []*Node) *Node {
	scores := a.ScoreNodes(workload, nodes)
	if len(scores) == 0 {
		return nil
	}

	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})

	return scores[0].Node
}

// ScoreNodes calculates balanced scores.
func (a *BalancedAlgorithm) ScoreNodes(workload *Workload, nodes []*Node) []NodeScore {
	a.mu.Lock()
	ratio := a.binPackRatio
	a.mu.Unlock()

	binPack := NewBinPackAlgorithm()
	spread := NewSpreadAlgorithm()

	binPackScores := binPack.ScoreNodes(workload, nodes)
	spreadScores := spread.ScoreNodes(workload, nodes)

	// Create lookup for spread scores
	spreadLookup := make(map[string]int64)
	for _, s := range spreadScores {
		spreadLookup[s.Node.ID] = s.Score
	}

	var scores []NodeScore
	for _, s := range binPackScores {
		spreadScore := spreadLookup[s.Node.ID]
		balanced := int64(float64(s.Score)*ratio + float64(spreadScore)*(1-ratio))
		scores = append(scores, NodeScore{
			Node:  s.Node,
			Score: balanced,
		})
	}

	return scores
}

// ResourceFitAlgorithm selects based on best resource fit.
type ResourceFitAlgorithm struct {
	mu           sync.Mutex
	preferLarger bool
}

// NewResourceFitAlgorithm creates a new resource fit algorithm.
func NewResourceFitAlgorithm(preferLarger bool) *ResourceFitAlgorithm {
	return &ResourceFitAlgorithm{
		preferLarger: preferLarger,
	}
}

// Name returns the algorithm name.
func (a *ResourceFitAlgorithm) Name() string {
	return "ResourceFit"
}

// SelectNode selects the node with best resource fit.
func (a *ResourceFitAlgorithm) SelectNode(workload *Workload, nodes []*Node) *Node {
	scores := a.ScoreNodes(workload, nodes)
	if len(scores) == 0 {
		return nil
	}

	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})

	return scores[0].Node
}

// ScoreNodes calculates resource fit scores.
func (a *ResourceFitAlgorithm) ScoreNodes(workload *Workload, nodes []*Node) []NodeScore {
	a.mu.Lock()
	preferLarger := a.preferLarger
	a.mu.Unlock()

	var scores []NodeScore
	requested := workload.Resources.Requests

	for _, node := range nodes {
		if !requested.FitsIn(node.Available) {
			continue
		}

		// Calculate how well the resources fit
		cpuDiff := node.Available.CPU - requested.CPU
		memDiff := node.Available.Memory - requested.Memory

		var score int64
		if preferLarger {
			// Prefer nodes with more remaining resources
			score = cpuDiff + memDiff/1024/1024 // Normalize memory
		} else {
			// Prefer tight fit (minimize waste)
			score = 10000 - (cpuDiff + memDiff/1024/1024)
			if score < 0 {
				score = 0
			}
		}

		scores = append(scores, NodeScore{
			Node:  node,
			Score: score,
		})
	}

	return scores
}

// PriorityAlgorithm wraps another algorithm with priority handling.
type PriorityAlgorithm struct {
	base          Algorithm
	priorityBoost map[int32]int64
	mu            sync.RWMutex
}

// NewPriorityAlgorithm creates a priority-aware algorithm.
func NewPriorityAlgorithm(base Algorithm) *PriorityAlgorithm {
	return &PriorityAlgorithm{
		base:          base,
		priorityBoost: make(map[int32]int64),
	}
}

// Name returns the algorithm name.
func (a *PriorityAlgorithm) Name() string {
	return "Priority(" + a.base.Name() + ")"
}

// SetPriorityBoost sets the score boost for a priority level.
func (a *PriorityAlgorithm) SetPriorityBoost(priority int32, boost int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.priorityBoost[priority] = boost
}

// SelectNode selects a node considering priority.
func (a *PriorityAlgorithm) SelectNode(workload *Workload, nodes []*Node) *Node {
	return a.base.SelectNode(workload, nodes)
}

// ScoreNodes calculates scores with priority boost.
func (a *PriorityAlgorithm) ScoreNodes(workload *Workload, nodes []*Node) []NodeScore {
	scores := a.base.ScoreNodes(workload, nodes)

	a.mu.RLock()
	boost := a.priorityBoost[workload.Priority]
	a.mu.RUnlock()

	for i := range scores {
		scores[i].Score += boost
	}

	return scores
}

// TopologySpreadAlgorithm spreads workloads across topology domains.
type TopologySpreadAlgorithm struct {
	mu                sync.RWMutex
	topologyKey       string
	maxSkew           int
	whenUnsatisfiable string
}

// NewTopologySpreadAlgorithm creates a topology spread algorithm.
func NewTopologySpreadAlgorithm(topologyKey string, maxSkew int) *TopologySpreadAlgorithm {
	return &TopologySpreadAlgorithm{
		topologyKey:       topologyKey,
		maxSkew:           maxSkew,
		whenUnsatisfiable: "DoNotSchedule",
	}
}

// Name returns the algorithm name.
func (a *TopologySpreadAlgorithm) Name() string {
	return "TopologySpread"
}

// SelectNode selects a node respecting topology spread.
func (a *TopologySpreadAlgorithm) SelectNode(workload *Workload, nodes []*Node) *Node {
	scores := a.ScoreNodes(workload, nodes)
	if len(scores) == 0 {
		return nil
	}

	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})

	return scores[0].Node
}

// ScoreNodes calculates topology spread scores.
func (a *TopologySpreadAlgorithm) ScoreNodes(workload *Workload, nodes []*Node) []NodeScore {
	a.mu.RLock()
	topologyKey := a.topologyKey
	maxSkew := a.maxSkew
	a.mu.RUnlock()

	// Count workloads per topology domain
	domainCounts := make(map[string]int)
	for _, node := range nodes {
		domain := node.Labels[topologyKey]
		domainCounts[domain] = 0
	}

	// Find min count
	minCount := int(^uint(0) >> 1)
	for _, count := range domainCounts {
		if count < minCount {
			minCount = count
		}
	}

	var scores []NodeScore
	for _, node := range nodes {
		if !workload.Resources.Requests.FitsIn(node.Available) {
			continue
		}

		domain := node.Labels[topologyKey]
		count := domainCounts[domain]

		// Check skew constraint
		if count-minCount >= maxSkew {
			continue
		}

		// Score based on inverse domain count
		score := int64(100 - count*10)
		if score < 0 {
			score = 0
		}

		scores = append(scores, NodeScore{
			Node:  node,
			Score: score,
		})
	}

	return scores
}

// CompositeAlgorithm combines multiple algorithms.
type CompositeAlgorithm struct {
	mu         sync.RWMutex
	algorithms []weightedAlgorithm
}

type weightedAlgorithm struct {
	algorithm Algorithm
	weight    float64
}

// NewCompositeAlgorithm creates a composite algorithm.
func NewCompositeAlgorithm() *CompositeAlgorithm {
	return &CompositeAlgorithm{
		algorithms: []weightedAlgorithm{},
	}
}

// Name returns the algorithm name.
func (a *CompositeAlgorithm) Name() string {
	return "Composite"
}

// AddAlgorithm adds an algorithm with a weight.
func (a *CompositeAlgorithm) AddAlgorithm(algo Algorithm, weight float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.algorithms = append(a.algorithms, weightedAlgorithm{
		algorithm: algo,
		weight:    weight,
	})
}

// SelectNode selects a node using combined scores.
func (a *CompositeAlgorithm) SelectNode(workload *Workload, nodes []*Node) *Node {
	scores := a.ScoreNodes(workload, nodes)
	if len(scores) == 0 {
		return nil
	}

	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})

	return scores[0].Node
}

// ScoreNodes calculates combined scores from all algorithms.
func (a *CompositeAlgorithm) ScoreNodes(workload *Workload, nodes []*Node) []NodeScore {
	a.mu.RLock()
	algorithms := make([]weightedAlgorithm, len(a.algorithms))
	copy(algorithms, a.algorithms)
	a.mu.RUnlock()

	if len(algorithms) == 0 {
		return nil
	}

	// Collect scores from all algorithms
	nodeScores := make(map[string]float64)
	for _, node := range nodes {
		nodeScores[node.ID] = 0
	}

	totalWeight := float64(0)
	for _, wa := range algorithms {
		scores := wa.algorithm.ScoreNodes(workload, nodes)
		for _, s := range scores {
			nodeScores[s.Node.ID] += float64(s.Score) * wa.weight
		}
		totalWeight += wa.weight
	}

	// Normalize and convert back
	var scores []NodeScore
	for _, node := range nodes {
		score := nodeScores[node.ID]
		if totalWeight > 0 {
			score /= totalWeight
		}
		if score > 0 || workload.Resources.Requests.FitsIn(node.Available) {
			scores = append(scores, NodeScore{
				Node:  node,
				Score: int64(score),
			})
		}
	}

	return scores
}

// LeastRequestedAlgorithm favors nodes with the most available resources.
type LeastRequestedAlgorithm struct{}

// NewLeastRequestedAlgorithm creates a least requested algorithm.
func NewLeastRequestedAlgorithm() *LeastRequestedAlgorithm {
	return &LeastRequestedAlgorithm{}
}

// Name returns the algorithm name.
func (a *LeastRequestedAlgorithm) Name() string {
	return "LeastRequested"
}

// SelectNode selects the node with most available resources.
func (a *LeastRequestedAlgorithm) SelectNode(workload *Workload, nodes []*Node) *Node {
	scores := a.ScoreNodes(workload, nodes)
	if len(scores) == 0 {
		return nil
	}

	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})

	return scores[0].Node
}

// ScoreNodes calculates least requested scores.
func (a *LeastRequestedAlgorithm) ScoreNodes(workload *Workload, nodes []*Node) []NodeScore {
	var scores []NodeScore

	for _, node := range nodes {
		if !workload.Resources.Requests.FitsIn(node.Available) {
			continue
		}

		// Calculate available resource ratio
		cpuAvail := float64(0)
		memAvail := float64(0)

		if node.Allocatable.CPU > 0 {
			cpuAvail = float64(node.Available.CPU) / float64(node.Allocatable.CPU)
		}
		if node.Allocatable.Memory > 0 {
			memAvail = float64(node.Available.Memory) / float64(node.Allocatable.Memory)
		}

		score := int64((cpuAvail + memAvail) * 50)

		scores = append(scores, NodeScore{
			Node:  node,
			Score: score,
		})
	}

	return scores
}

// MostRequestedAlgorithm favors nodes with highest utilization.
type MostRequestedAlgorithm struct{}

// NewMostRequestedAlgorithm creates a most requested algorithm.
func NewMostRequestedAlgorithm() *MostRequestedAlgorithm {
	return &MostRequestedAlgorithm{}
}

// Name returns the algorithm name.
func (a *MostRequestedAlgorithm) Name() string {
	return "MostRequested"
}

// SelectNode selects the node with highest utilization.
func (a *MostRequestedAlgorithm) SelectNode(workload *Workload, nodes []*Node) *Node {
	scores := a.ScoreNodes(workload, nodes)
	if len(scores) == 0 {
		return nil
	}

	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})

	return scores[0].Node
}

// ScoreNodes calculates most requested scores.
func (a *MostRequestedAlgorithm) ScoreNodes(workload *Workload, nodes []*Node) []NodeScore {
	var scores []NodeScore

	for _, node := range nodes {
		if !workload.Resources.Requests.FitsIn(node.Available) {
			continue
		}

		// Calculate utilization ratio
		cpuUtil := float64(0)
		memUtil := float64(0)

		if node.Allocatable.CPU > 0 {
			cpuUtil = float64(node.Allocated.CPU) / float64(node.Allocatable.CPU)
		}
		if node.Allocatable.Memory > 0 {
			memUtil = float64(node.Allocated.Memory) / float64(node.Allocatable.Memory)
		}

		score := int64((cpuUtil + memUtil) * 50)

		scores = append(scores, NodeScore{
			Node:  node,
			Score: score,
		})
	}

	return scores
}
