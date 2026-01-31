package scheduler

import (
	"sort"
	"sync"
	"time"
)

// PreemptionResult contains the result of a preemption attempt.
type PreemptionResult struct {
	Node               *Node
	PreemptedWorkloads []string
	Resources          Resources
}

// PreemptionCandidate represents a potential preemption target.
type PreemptionCandidate struct {
	NodeID            string
	VictimWorkloads   []*Workload
	FreedResources    Resources
	PrioritySum       int32
	NumVictims        int
	HighestVictimPrio int32
}

// Preemptor handles workload preemption logic.
type Preemptor struct {
	mu                sync.RWMutex
	nodes             *NodeRegistry
	timeout           time.Duration
	gracePeriod       time.Duration
	minVictimPriority int32
	maxVictims        int
}

// NewPreemptor creates a new preemptor.
func NewPreemptor(nodes *NodeRegistry, timeout time.Duration) *Preemptor {
	return &Preemptor{
		nodes:             nodes,
		timeout:           timeout,
		gracePeriod:       30 * time.Second,
		minVictimPriority: -1000,
		maxVictims:        10,
	}
}

// SetGracePeriod sets the preemption grace period.
func (p *Preemptor) SetGracePeriod(d time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.gracePeriod = d
}

// SetMinVictimPriority sets minimum priority for victims.
func (p *Preemptor) SetMinVictimPriority(priority int32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.minVictimPriority = priority
}

// SetMaxVictims sets maximum number of victims per preemption.
func (p *Preemptor) SetMaxVictims(max int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.maxVictims = max
}

// WorkloadGetter retrieves workloads for a node.
type WorkloadGetter func(nodeID string) []*Workload

// FindPreemptionCandidates finds preemption candidates for a workload.
func (p *Preemptor) FindPreemptionCandidates(
	preemptor *Workload,
	nodes []*Node,
	getWorkloads WorkloadGetter,
) *PreemptionResult {
	p.mu.RLock()
	minPrio := p.minVictimPriority
	maxVictims := p.maxVictims
	p.mu.RUnlock()

	var candidates []PreemptionCandidate

	for _, node := range nodes {
		workloads := getWorkloads(node.ID)
		candidate := p.evaluateNode(preemptor, node, workloads, minPrio, maxVictims)
		if candidate != nil {
			candidates = append(candidates, *candidate)
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	// Select best candidate
	best := p.selectBestCandidate(candidates)

	victimIDs := make([]string, len(best.VictimWorkloads))
	for i, v := range best.VictimWorkloads {
		victimIDs[i] = v.ID
	}

	node, _ := p.nodes.Get(best.NodeID)

	return &PreemptionResult{
		Node:               node,
		PreemptedWorkloads: victimIDs,
		Resources:          best.FreedResources,
	}
}

// evaluateNode evaluates a node for preemption potential.
func (p *Preemptor) evaluateNode(
	preemptor *Workload,
	node *Node,
	workloads []*Workload,
	minPriority int32,
	maxVictims int,
) *PreemptionCandidate {
	// Filter eligible victims
	var eligible []*Workload
	for _, w := range workloads {
		if w.Priority < preemptor.Priority && w.Priority >= minPriority {
			eligible = append(eligible, w)
		}
	}

	if len(eligible) == 0 {
		return nil
	}

	// Sort by priority (lowest first)
	sort.Slice(eligible, func(i, j int) bool {
		return eligible[i].Priority < eligible[j].Priority
	})

	// Find minimum set of victims
	var victims []*Workload
	freed := Resources{}
	_ = preemptor.Resources.Requests.Sub(node.Available) // Calculate needed resources

	for _, w := range eligible {
		if len(victims) >= maxVictims {
			break
		}

		victims = append(victims, w)
		freed = freed.Add(w.Resources.Requests)

		// Check if we have enough resources now
		potential := node.Available.Add(freed)
		if preemptor.Resources.Requests.FitsIn(potential) {
			break
		}
	}

	// Verify we have enough resources
	potential := node.Available.Add(freed)
	if !preemptor.Resources.Requests.FitsIn(potential) {
		return nil
	}

	// Calculate stats
	var prioritySum int32
	var highestPrio int32 = -32768
	for _, v := range victims {
		prioritySum += v.Priority
		if v.Priority > highestPrio {
			highestPrio = v.Priority
		}
	}

	return &PreemptionCandidate{
		NodeID:            node.ID,
		VictimWorkloads:   victims,
		FreedResources:    freed,
		PrioritySum:       prioritySum,
		NumVictims:        len(victims),
		HighestVictimPrio: highestPrio,
	}
}

// selectBestCandidate selects the best preemption candidate.
func (p *Preemptor) selectBestCandidate(candidates []PreemptionCandidate) *PreemptionCandidate {
	// Sort candidates by preference:
	// 1. Fewer victims
	// 2. Lower priority sum
	// 3. Lower highest victim priority
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].NumVictims != candidates[j].NumVictims {
			return candidates[i].NumVictims < candidates[j].NumVictims
		}
		if candidates[i].PrioritySum != candidates[j].PrioritySum {
			return candidates[i].PrioritySum < candidates[j].PrioritySum
		}
		return candidates[i].HighestVictimPrio < candidates[j].HighestVictimPrio
	})

	return &candidates[0]
}

// PreemptionPolicy defines preemption behavior.
type PreemptionPolicy struct {
	Name              string
	Enabled           bool
	PriorityThreshold int32
	GracePeriod       time.Duration
	MaxVictimsPerNode int
	PreferSamePod     bool
	AvoidSystemPods   bool
}

// DefaultPreemptionPolicy returns the default policy.
func DefaultPreemptionPolicy() PreemptionPolicy {
	return PreemptionPolicy{
		Name:              "default",
		Enabled:           true,
		PriorityThreshold: -1000,
		GracePeriod:       30 * time.Second,
		MaxVictimsPerNode: 10,
		PreferSamePod:     false,
		AvoidSystemPods:   true,
	}
}

// PreemptionPolicyManager manages preemption policies.
type PreemptionPolicyManager struct {
	mu       sync.RWMutex
	policies map[string]PreemptionPolicy
	default_ string
}

// NewPreemptionPolicyManager creates a new manager.
func NewPreemptionPolicyManager() *PreemptionPolicyManager {
	mgr := &PreemptionPolicyManager{
		policies: make(map[string]PreemptionPolicy),
		default_: "default",
	}
	mgr.policies["default"] = DefaultPreemptionPolicy()
	return mgr
}

// SetPolicy sets a preemption policy.
func (m *PreemptionPolicyManager) SetPolicy(policy PreemptionPolicy) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.policies[policy.Name] = policy
}

// GetPolicy returns a policy by name.
func (m *PreemptionPolicyManager) GetPolicy(name string) (PreemptionPolicy, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	policy, exists := m.policies[name]
	return policy, exists
}

// GetDefaultPolicy returns the default policy.
func (m *PreemptionPolicyManager) GetDefaultPolicy() PreemptionPolicy {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.policies[m.default_]
}

// SetDefault sets the default policy name.
func (m *PreemptionPolicyManager) SetDefault(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.policies[name]; !exists {
		return ErrWorkloadNotFound
	}
	m.default_ = name
	return nil
}

// DeletePolicy removes a policy.
func (m *PreemptionPolicyManager) DeletePolicy(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if name != "default" {
		delete(m.policies, name)
	}
}

// PreemptionRecord tracks preemption history.
type PreemptionRecord struct {
	ID              string
	Timestamp       time.Time
	PreemptorID     string
	PreemptorPrio   int32
	VictimIDs       []string
	NodeID          string
	ResourcesFreed  Resources
	Reason          string
}

// PreemptionHistory tracks preemption events.
type PreemptionHistory struct {
	mu      sync.RWMutex
	records []PreemptionRecord
	maxSize int
}

// NewPreemptionHistory creates a new history tracker.
func NewPreemptionHistory(maxSize int) *PreemptionHistory {
	return &PreemptionHistory{
		records: make([]PreemptionRecord, 0),
		maxSize: maxSize,
	}
}

// Add adds a preemption record.
func (h *PreemptionHistory) Add(record PreemptionRecord) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.records = append(h.records, record)
	if len(h.records) > h.maxSize {
		h.records = h.records[1:]
	}
}

// GetRecent returns recent preemption records.
func (h *PreemptionHistory) GetRecent(count int) []PreemptionRecord {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if count > len(h.records) {
		count = len(h.records)
	}

	result := make([]PreemptionRecord, count)
	start := len(h.records) - count
	copy(result, h.records[start:])
	return result
}

// GetByWorkload returns records for a workload.
func (h *PreemptionHistory) GetByWorkload(workloadID string) []PreemptionRecord {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var result []PreemptionRecord
	for _, r := range h.records {
		if r.PreemptorID == workloadID {
			result = append(result, r)
			continue
		}
		for _, vid := range r.VictimIDs {
			if vid == workloadID {
				result = append(result, r)
				break
			}
		}
	}
	return result
}

// GetByNode returns records for a node.
func (h *PreemptionHistory) GetByNode(nodeID string) []PreemptionRecord {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var result []PreemptionRecord
	for _, r := range h.records {
		if r.NodeID == nodeID {
			result = append(result, r)
		}
	}
	return result
}

// GetStats returns preemption statistics.
func (h *PreemptionHistory) GetStats() PreemptionStats {
	h.mu.RLock()
	defer h.mu.RUnlock()

	stats := PreemptionStats{
		TotalPreemptions: len(h.records),
		PreemptionsByNode: make(map[string]int),
	}

	for _, r := range h.records {
		stats.TotalVictims += len(r.VictimIDs)
		stats.PreemptionsByNode[r.NodeID]++
	}

	return stats
}

// PreemptionStats contains preemption statistics.
type PreemptionStats struct {
	TotalPreemptions  int
	TotalVictims      int
	PreemptionsByNode map[string]int
}

// VictimSelector selects victims for preemption.
type VictimSelector struct {
	mu              sync.RWMutex
	selectionPolicy VictimSelectionPolicy
}

// VictimSelectionPolicy defines victim selection behavior.
type VictimSelectionPolicy string

const (
	VictimSelectionLowestPriority VictimSelectionPolicy = "LowestPriority"
	VictimSelectionNewest         VictimSelectionPolicy = "Newest"
	VictimSelectionOldest         VictimSelectionPolicy = "Oldest"
	VictimSelectionRandom         VictimSelectionPolicy = "Random"
)

// NewVictimSelector creates a new victim selector.
func NewVictimSelector(policy VictimSelectionPolicy) *VictimSelector {
	return &VictimSelector{
		selectionPolicy: policy,
	}
}

// SetPolicy sets the selection policy.
func (vs *VictimSelector) SetPolicy(policy VictimSelectionPolicy) {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	vs.selectionPolicy = policy
}

// SelectVictims selects victims from candidates.
func (vs *VictimSelector) SelectVictims(
	candidates []*Workload,
	needed Resources,
	maxVictims int,
) []*Workload {
	vs.mu.RLock()
	policy := vs.selectionPolicy
	vs.mu.RUnlock()

	if len(candidates) == 0 {
		return nil
	}

	// Sort based on policy
	sorted := make([]*Workload, len(candidates))
	copy(sorted, candidates)

	switch policy {
	case VictimSelectionLowestPriority:
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Priority < sorted[j].Priority
		})
	case VictimSelectionNewest:
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].CreatedAt.After(sorted[j].CreatedAt)
		})
	case VictimSelectionOldest:
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].CreatedAt.Before(sorted[j].CreatedAt)
		})
	}

	// Select minimum victims needed
	var victims []*Workload
	freed := Resources{}

	for _, w := range sorted {
		if len(victims) >= maxVictims {
			break
		}

		victims = append(victims, w)
		freed = freed.Add(w.Resources.Requests)

		if freed.CPU >= needed.CPU && freed.Memory >= needed.Memory {
			break
		}
	}

	return victims
}

// PreemptionSimulator simulates preemption outcomes.
type PreemptionSimulator struct {
	nodes         *NodeRegistry
	getWorkloads  WorkloadGetter
}

// NewPreemptionSimulator creates a new simulator.
func NewPreemptionSimulator(nodes *NodeRegistry, getWorkloads WorkloadGetter) *PreemptionSimulator {
	return &PreemptionSimulator{
		nodes:        nodes,
		getWorkloads: getWorkloads,
	}
}

// SimulationResult contains simulation results.
type SimulationResult struct {
	CanSchedule      bool
	BestNode         *Node
	VictimsRequired  []*Workload
	ResourcesFreed   Resources
	ImpactScore      float64
}

// Simulate simulates preemption for a workload.
func (ps *PreemptionSimulator) Simulate(preemptor *Workload) *SimulationResult {
	nodes := ps.nodes.ListReady()

	// First check if we can schedule without preemption
	for _, node := range nodes {
		if preemptor.Resources.Requests.FitsIn(node.Available) {
			return &SimulationResult{
				CanSchedule: true,
				BestNode:    node,
			}
		}
	}

	// Find best preemption option
	var bestResult *SimulationResult
	bestScore := float64(1000000)

	for _, node := range nodes {
		workloads := ps.getWorkloads(node.ID)
		result := ps.simulateNode(preemptor, node, workloads)
		if result != nil && result.ImpactScore < bestScore {
			bestResult = result
			bestScore = result.ImpactScore
		}
	}

	return bestResult
}

// simulateNode simulates preemption on a single node.
func (ps *PreemptionSimulator) simulateNode(
	preemptor *Workload,
	node *Node,
	workloads []*Workload,
) *SimulationResult {
	// Find eligible victims
	var eligible []*Workload
	for _, w := range workloads {
		if w.Priority < preemptor.Priority {
			eligible = append(eligible, w)
		}
	}

	if len(eligible) == 0 {
		return nil
	}

	// Sort by priority
	sort.Slice(eligible, func(i, j int) bool {
		return eligible[i].Priority < eligible[j].Priority
	})

	// Find minimum victims
	var victims []*Workload
	freed := Resources{}

	for _, w := range eligible {
		victims = append(victims, w)
		freed = freed.Add(w.Resources.Requests)

		potential := node.Available.Add(freed)
		if preemptor.Resources.Requests.FitsIn(potential) {
			break
		}
	}

	// Verify feasibility
	potential := node.Available.Add(freed)
	if !preemptor.Resources.Requests.FitsIn(potential) {
		return nil
	}

	// Calculate impact score
	impactScore := float64(len(victims)) * 10
	for _, v := range victims {
		impactScore += float64(v.Priority)
	}

	return &SimulationResult{
		CanSchedule:     true,
		BestNode:        node,
		VictimsRequired: victims,
		ResourcesFreed:  freed,
		ImpactScore:     impactScore,
	}
}

// PreemptionGuard prevents cascading preemptions.
type PreemptionGuard struct {
	mu              sync.RWMutex
	recentlyPreempted map[string]time.Time
	cooldownPeriod    time.Duration
}

// NewPreemptionGuard creates a new preemption guard.
func NewPreemptionGuard(cooldown time.Duration) *PreemptionGuard {
	return &PreemptionGuard{
		recentlyPreempted: make(map[string]time.Time),
		cooldownPeriod:    cooldown,
	}
}

// RecordPreemption records that a workload was preempted.
func (pg *PreemptionGuard) RecordPreemption(workloadID string) {
	pg.mu.Lock()
	defer pg.mu.Unlock()
	pg.recentlyPreempted[workloadID] = time.Now()
}

// CanBePreempted checks if a workload can be preempted.
func (pg *PreemptionGuard) CanBePreempted(workloadID string) bool {
	pg.mu.RLock()
	defer pg.mu.RUnlock()

	lastPreemption, exists := pg.recentlyPreempted[workloadID]
	if !exists {
		return true
	}

	return time.Since(lastPreemption) > pg.cooldownPeriod
}

// Cleanup removes expired entries.
func (pg *PreemptionGuard) Cleanup() {
	pg.mu.Lock()
	defer pg.mu.Unlock()

	now := time.Now()
	for id, t := range pg.recentlyPreempted {
		if now.Sub(t) > pg.cooldownPeriod {
			delete(pg.recentlyPreempted, id)
		}
	}
}
