package scheduler

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Preemption-related errors.
var (
	ErrPreemptionDisabled      = errors.New("preemption is disabled")
	ErrNoPreemptionCandidates  = errors.New("no valid preemption candidates found")
	ErrPreemptionPolicyDenied  = errors.New("preemption policy does not allow preemption")
	ErrPDBViolation            = errors.New("preemption would violate pod disruption budget")
	ErrAntiAffinityViolation   = errors.New("preemption violates anti-affinity rules")
	ErrGracePeriodNotExpired   = errors.New("graceful termination period not expired")
	ErrWorkloadNotPreemptible  = errors.New("workload is not preemptible")
	ErrPreemptionQueueFull     = errors.New("preemption queue is full")
)

// PreemptionPolicy defines the preemption behavior for a workload.
type PreemptionPolicy string

const (
	// PreemptNever indicates the workload should never preempt others.
	PreemptNever PreemptionPolicy = "Never"
	// PreemptLowerPriority allows preemption of lower priority workloads.
	PreemptLowerPriority PreemptionPolicy = "PreemptLowerPriority"
)

// PreemptionResult contains the result of a preemption attempt.
type PreemptionResult struct {
	Node               *Node
	PreemptedWorkloads []string
	Resources          Resources
	Success            bool
	Error              error
	Duration           time.Duration
	Timestamp          time.Time
}

// PreemptionCandidate represents a potential preemption target.
type PreemptionCandidate struct {
	NodeID            string
	VictimWorkloads   []*Workload
	FreedResources    Resources
	PrioritySum       int32
	NumVictims        int
	HighestVictimPrio int32
	DisruptionScore   float64
	PDBSafe           bool
	AntiAffinitySafe  bool
}

// PreemptorConfig configures the preemption behavior.
type PreemptorConfig struct {
	// Timeout is the maximum time to wait for preemption.
	Timeout time.Duration
	// GracePeriod is the time to allow for graceful termination.
	GracePeriod time.Duration
	// MinVictimPriority is the minimum priority for victims.
	MinVictimPriority int32
	// MaxVictims is the maximum number of victims per preemption.
	MaxVictims int
	// EnablePDBRespect enables pod disruption budget respect.
	EnablePDBRespect bool
	// EnableAntiAffinity enables preemption anti-affinity.
	EnableAntiAffinity bool
	// QueueSize is the size of the preemption queue.
	QueueSize int
	// CooldownPeriod is the time to wait before a workload can be preempted again.
	CooldownPeriod time.Duration
}

// DefaultPreemptorConfig returns a default configuration.
func DefaultPreemptorConfig() PreemptorConfig {
	return PreemptorConfig{
		Timeout:            30 * time.Second,
		GracePeriod:        30 * time.Second,
		MinVictimPriority:  -1000,
		MaxVictims:         10,
		EnablePDBRespect:   true,
		EnableAntiAffinity: true,
		QueueSize:          1000,
		CooldownPeriod:     5 * time.Minute,
	}
}

// Preemptor handles workload preemption logic.
type Preemptor struct {
	mu     sync.RWMutex
	config PreemptorConfig
	nodes  *NodeRegistry

	// Preemption queue
	queue *PreemptionQueue

	// Pod Disruption Budgets
	pdbManager *PodDisruptionBudgetManager

	// Anti-affinity tracker
	antiAffinityTracker *PreemptionAntiAffinityTracker

	// Metrics
	metrics *PreemptionMetrics

	// Event logger
	eventLog *PreemptionEventLog

	// Preemption guard
	guard *PreemptionGuard

	// History
	history *PreemptionHistory

	// Graceful termination manager
	terminator *GracefulTerminator

	// Callbacks
	onPreemptionStart   func(*Workload, []*Workload)
	onPreemptionSuccess func(*Workload, *PreemptionResult)
	onPreemptionFailure func(*Workload, error)
	onVictimTerminated  func(*Workload)
}

// NewPreemptor creates a new preemptor with the given configuration.
func NewPreemptor(nodes *NodeRegistry, timeout time.Duration) *Preemptor {
	config := DefaultPreemptorConfig()
	config.Timeout = timeout

	return NewPreemptorWithConfig(nodes, config)
}

// NewPreemptorWithConfig creates a new preemptor with full configuration.
func NewPreemptorWithConfig(nodes *NodeRegistry, config PreemptorConfig) *Preemptor {
	return &Preemptor{
		config:              config,
		nodes:               nodes,
		queue:               NewPreemptionQueue(config.QueueSize),
		pdbManager:          NewPodDisruptionBudgetManager(),
		antiAffinityTracker: NewPreemptionAntiAffinityTracker(),
		metrics:             NewPreemptionMetrics(),
		eventLog:            NewPreemptionEventLog(1000),
		guard:               NewPreemptionGuard(config.CooldownPeriod),
		history:             NewPreemptionHistory(1000),
		terminator:          NewGracefulTerminator(config.GracePeriod),
	}
}

// SetGracePeriod sets the preemption grace period.
func (p *Preemptor) SetGracePeriod(d time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.config.GracePeriod = d
	p.terminator.SetGracePeriod(d)
}

// SetMinVictimPriority sets minimum priority for victims.
func (p *Preemptor) SetMinVictimPriority(priority int32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.config.MinVictimPriority = priority
}

// SetMaxVictims sets maximum number of victims per preemption.
func (p *Preemptor) SetMaxVictims(max int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.config.MaxVictims = max
}

// SetPDBRespect enables or disables PDB respect.
func (p *Preemptor) SetPDBRespect(enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.config.EnablePDBRespect = enabled
}

// SetAntiAffinity enables or disables anti-affinity respect.
func (p *Preemptor) SetAntiAffinity(enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.config.EnableAntiAffinity = enabled
}

// OnPreemptionStart sets a callback for preemption start.
func (p *Preemptor) OnPreemptionStart(fn func(*Workload, []*Workload)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onPreemptionStart = fn
}

// OnPreemptionSuccess sets a callback for preemption success.
func (p *Preemptor) OnPreemptionSuccess(fn func(*Workload, *PreemptionResult)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onPreemptionSuccess = fn
}

// OnPreemptionFailure sets a callback for preemption failure.
func (p *Preemptor) OnPreemptionFailure(fn func(*Workload, error)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onPreemptionFailure = fn
}

// OnVictimTerminated sets a callback for victim termination.
func (p *Preemptor) OnVictimTerminated(fn func(*Workload)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onVictimTerminated = fn
}

// WorkloadGetter retrieves workloads for a node.
type WorkloadGetter func(nodeID string) []*Workload

// CanPreempt checks if a workload is allowed to preempt others.
func (p *Preemptor) CanPreempt(workload *Workload) bool {
	policy := p.GetPreemptionPolicy(workload)
	return policy == PreemptLowerPriority
}

// GetPreemptionPolicy returns the preemption policy for a workload.
func (p *Preemptor) GetPreemptionPolicy(workload *Workload) PreemptionPolicy {
	if workload.Annotations != nil {
		if policy, ok := workload.Annotations["scheduler.unheaded.io/preemption-policy"]; ok {
			switch PreemptionPolicy(policy) {
			case PreemptNever:
				return PreemptNever
			case PreemptLowerPriority:
				return PreemptLowerPriority
			}
		}
	}
	// Default to PreemptLowerPriority
	return PreemptLowerPriority
}

// IsPreemptible checks if a workload can be preempted.
func (p *Preemptor) IsPreemptible(workload *Workload, preemptorPriority int32) bool {
	p.mu.RLock()
	minPrio := p.config.MinVictimPriority
	p.mu.RUnlock()

	// Check priority
	if workload.Priority >= preemptorPriority {
		return false
	}

	if workload.Priority < minPrio {
		return false
	}

	// Check if workload has preemption protection
	if workload.Annotations != nil {
		if protected, ok := workload.Annotations["scheduler.unheaded.io/preemption-protected"]; ok && protected == "true" {
			return false
		}
	}

	// Check cooldown
	if !p.guard.CanBePreempted(workload.ID) {
		return false
	}

	return true
}

// FindPreemptionCandidates finds preemption candidates for a workload.
func (p *Preemptor) FindPreemptionCandidates(
	preemptor *Workload,
	nodes []*Node,
	getWorkloads WorkloadGetter,
) *PreemptionResult {
	start := time.Now()

	// Check if preemptor is allowed to preempt
	if !p.CanPreempt(preemptor) {
		p.metrics.RecordPreemptionAttempt(false, time.Since(start))
		p.eventLog.LogEvent(PreemptionEventDenied, preemptor.ID, nil, "preemption policy denies preemption", nil)
		return nil
	}

	p.mu.RLock()
	config := p.config
	p.mu.RUnlock()

	var candidates []PreemptionCandidate

	for _, node := range nodes {
		workloads := getWorkloads(node.ID)
		candidate := p.evaluateNode(preemptor, node, workloads, config)
		if candidate != nil {
			candidates = append(candidates, *candidate)
		}
	}

	if len(candidates) == 0 {
		p.metrics.RecordPreemptionAttempt(false, time.Since(start))
		p.eventLog.LogEvent(PreemptionEventNoCandidates, preemptor.ID, nil, "no valid candidates found", nil)
		return nil
	}

	// Select best candidate using victim selection algorithm
	best := p.selectBestCandidate(candidates, preemptor)

	victimIDs := make([]string, len(best.VictimWorkloads))
	for i, v := range best.VictimWorkloads {
		victimIDs[i] = v.ID
	}

	node, _ := p.nodes.Get(best.NodeID)

	// Fire preemption start callback
	p.mu.RLock()
	startCallback := p.onPreemptionStart
	p.mu.RUnlock()
	if startCallback != nil {
		startCallback(preemptor, best.VictimWorkloads)
	}

	result := &PreemptionResult{
		Node:               node,
		PreemptedWorkloads: victimIDs,
		Resources:          best.FreedResources,
		Success:            true,
		Duration:           time.Since(start),
		Timestamp:          time.Now(),
	}

	// Record in history
	p.history.Add(PreemptionRecord{
		ID:             fmt.Sprintf("%s-%d", preemptor.ID, time.Now().UnixNano()),
		Timestamp:      time.Now(),
		PreemptorID:    preemptor.ID,
		PreemptorPrio:  preemptor.Priority,
		VictimIDs:      victimIDs,
		NodeID:         node.ID,
		ResourcesFreed: best.FreedResources,
		Reason:         "scheduled preemption",
	})

	// Record in guard
	for _, victimID := range victimIDs {
		p.guard.RecordPreemption(victimID)
	}

	p.metrics.RecordPreemptionAttempt(true, time.Since(start))
	p.metrics.RecordVictimCount(len(victimIDs))
	p.eventLog.LogEvent(PreemptionEventSuccess, preemptor.ID, victimIDs, "preemption successful", node)

	return result
}

// evaluateNode evaluates a node for preemption potential.
func (p *Preemptor) evaluateNode(
	preemptor *Workload,
	node *Node,
	workloads []*Workload,
	config PreemptorConfig,
) *PreemptionCandidate {
	// Filter eligible victims
	var eligible []*Workload
	for _, w := range workloads {
		if p.IsPreemptible(w, preemptor.Priority) {
			eligible = append(eligible, w)
		}
	}

	if len(eligible) == 0 {
		return nil
	}

	// Sort by priority (lowest first) for minimal disruption
	sort.Slice(eligible, func(i, j int) bool {
		if eligible[i].Priority != eligible[j].Priority {
			return eligible[i].Priority < eligible[j].Priority
		}
		// Secondary: oldest first
		return eligible[i].CreatedAt.Before(eligible[j].CreatedAt)
	})

	// Find minimum set of victims
	var victims []*Workload
	freed := Resources{}

	for _, w := range eligible {
		if len(victims) >= config.MaxVictims {
			break
		}

		// Check PDB if enabled
		if config.EnablePDBRespect {
			if !p.pdbManager.CanDisrupt(w) {
				continue
			}
		}

		// Check anti-affinity if enabled
		if config.EnableAntiAffinity {
			if !p.antiAffinityTracker.CanPreempt(preemptor, w) {
				continue
			}
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
	disruptionScore := float64(0)

	for _, v := range victims {
		prioritySum += v.Priority
		if v.Priority > highestPrio {
			highestPrio = v.Priority
		}
		// Factor in resource size and priority
		disruptionScore += float64(v.Priority) * float64(v.Resources.Requests.CPU+v.Resources.Requests.Memory/1024/1024)
	}

	// Check PDB safety for all victims
	pdbSafe := true
	if config.EnablePDBRespect {
		for _, v := range victims {
			if !p.pdbManager.CanDisrupt(v) {
				pdbSafe = false
				break
			}
		}
	}

	// Check anti-affinity safety
	antiAffinitySafe := true
	if config.EnableAntiAffinity {
		for _, v := range victims {
			if !p.antiAffinityTracker.CanPreempt(preemptor, v) {
				antiAffinitySafe = false
				break
			}
		}
	}

	return &PreemptionCandidate{
		NodeID:            node.ID,
		VictimWorkloads:   victims,
		FreedResources:    freed,
		PrioritySum:       prioritySum,
		NumVictims:        len(victims),
		HighestVictimPrio: highestPrio,
		DisruptionScore:   disruptionScore,
		PDBSafe:           pdbSafe,
		AntiAffinitySafe:  antiAffinitySafe,
	}
}

// selectBestCandidate selects the best preemption candidate using
// a sophisticated victim selection algorithm that minimizes disruption.
func (p *Preemptor) selectBestCandidate(candidates []PreemptionCandidate, preemptor *Workload) *PreemptionCandidate {
	// Filter candidates that are PDB and anti-affinity safe first
	var safeCandidates []PreemptionCandidate
	for _, c := range candidates {
		if c.PDBSafe && c.AntiAffinitySafe {
			safeCandidates = append(safeCandidates, c)
		}
	}

	// Fall back to all candidates if no safe ones
	if len(safeCandidates) == 0 {
		safeCandidates = candidates
	}

	// Sort candidates by preference (minimize disruption):
	// 1. Prefer PDB-safe candidates
	// 2. Prefer anti-affinity safe candidates
	// 3. Fewer victims
	// 4. Lower disruption score
	// 5. Lower priority sum
	// 6. Lower highest victim priority
	sort.Slice(safeCandidates, func(i, j int) bool {
		// PDB safety
		if safeCandidates[i].PDBSafe != safeCandidates[j].PDBSafe {
			return safeCandidates[i].PDBSafe
		}
		// Anti-affinity safety
		if safeCandidates[i].AntiAffinitySafe != safeCandidates[j].AntiAffinitySafe {
			return safeCandidates[i].AntiAffinitySafe
		}
		// Fewer victims
		if safeCandidates[i].NumVictims != safeCandidates[j].NumVictims {
			return safeCandidates[i].NumVictims < safeCandidates[j].NumVictims
		}
		// Lower disruption score
		if safeCandidates[i].DisruptionScore != safeCandidates[j].DisruptionScore {
			return safeCandidates[i].DisruptionScore < safeCandidates[j].DisruptionScore
		}
		// Lower priority sum
		if safeCandidates[i].PrioritySum != safeCandidates[j].PrioritySum {
			return safeCandidates[i].PrioritySum < safeCandidates[j].PrioritySum
		}
		// Lower highest victim priority
		return safeCandidates[i].HighestVictimPrio < safeCandidates[j].HighestVictimPrio
	})

	return &safeCandidates[0]
}

// ExecutePreemption executes the preemption with graceful termination.
func (p *Preemptor) ExecutePreemption(
	ctx context.Context,
	preemptor *Workload,
	result *PreemptionResult,
	getWorkload func(string) (*Workload, error),
	terminateWorkload func(*Workload) error,
) error {
	if result == nil || !result.Success {
		return ErrNoPreemptionCandidates
	}

	p.eventLog.LogEvent(PreemptionEventStartTermination, preemptor.ID, result.PreemptedWorkloads, "starting graceful termination", result.Node)

	// Terminate victims gracefully
	var wg sync.WaitGroup
	errChan := make(chan error, len(result.PreemptedWorkloads))

	for _, victimID := range result.PreemptedWorkloads {
		wg.Add(1)
		go func(vid string) {
			defer wg.Done()

			victim, err := getWorkload(vid)
			if err != nil {
				errChan <- fmt.Errorf("get workload %s: %w", vid, err)
				return
			}

			// Execute graceful termination
			err = p.terminator.Terminate(ctx, victim, terminateWorkload)
			if err != nil {
				errChan <- fmt.Errorf("terminate workload %s: %w", vid, err)
				return
			}

			// Notify termination callback
			p.mu.RLock()
			callback := p.onVictimTerminated
			p.mu.RUnlock()
			if callback != nil {
				callback(victim)
			}

			p.eventLog.LogEvent(PreemptionEventVictimTerminated, preemptor.ID, []string{vid}, "victim terminated", nil)
		}(victimID)
	}

	wg.Wait()
	close(errChan)

	// Collect errors
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		p.mu.RLock()
		failCallback := p.onPreemptionFailure
		p.mu.RUnlock()
		if failCallback != nil {
			failCallback(preemptor, errs[0])
		}
		return fmt.Errorf("preemption errors: %v", errs)
	}

	// Record success
	p.mu.RLock()
	successCallback := p.onPreemptionSuccess
	p.mu.RUnlock()
	if successCallback != nil {
		successCallback(preemptor, result)
	}

	p.eventLog.LogEvent(PreemptionEventCompleted, preemptor.ID, result.PreemptedWorkloads, "preemption completed", result.Node)

	return nil
}

// QueuePreemption adds a preemption request to the queue.
func (p *Preemptor) QueuePreemption(workload *Workload) error {
	return p.queue.Push(workload)
}

// DequeuePreemption removes the next preemption request from the queue.
func (p *Preemptor) DequeuePreemption() *Workload {
	return p.queue.Pop()
}

// GetPDBManager returns the PDB manager.
func (p *Preemptor) GetPDBManager() *PodDisruptionBudgetManager {
	return p.pdbManager
}

// GetAntiAffinityTracker returns the anti-affinity tracker.
func (p *Preemptor) GetAntiAffinityTracker() *PreemptionAntiAffinityTracker {
	return p.antiAffinityTracker
}

// GetMetrics returns the preemption metrics.
func (p *Preemptor) GetMetrics() *PreemptionMetrics {
	return p.metrics
}

// GetEventLog returns the event log.
func (p *Preemptor) GetEventLog() *PreemptionEventLog {
	return p.eventLog
}

// GetHistory returns the preemption history.
func (p *Preemptor) GetHistory() *PreemptionHistory {
	return p.history
}

// GetQueueLength returns the current queue length.
func (p *Preemptor) GetQueueLength() int {
	return p.queue.Len()
}

// Cleanup performs cleanup of expired entries.
func (p *Preemptor) Cleanup() {
	p.guard.Cleanup()
	p.antiAffinityTracker.Cleanup()
}

// =============================================================================
// Pod Disruption Budget (PDB) Support
// =============================================================================

// PodDisruptionBudget defines disruption constraints.
type PodDisruptionBudget struct {
	Name         string
	Namespace    string
	Selector     LabelSelector
	MinAvailable *int
	MaxUnavailable *int
	CurrentHealthy int
	DesiredHealthy int
	DisruptionsAllowed int
	ObservedGeneration int64
	CreatedAt    time.Time
}

// PodDisruptionBudgetManager manages PDBs.
type PodDisruptionBudgetManager struct {
	mu     sync.RWMutex
	pdbs   map[string]*PodDisruptionBudget // key: namespace/name
	workloadPDBs map[string]string // workloadID -> pdb key
}

// NewPodDisruptionBudgetManager creates a new PDB manager.
func NewPodDisruptionBudgetManager() *PodDisruptionBudgetManager {
	return &PodDisruptionBudgetManager{
		pdbs:         make(map[string]*PodDisruptionBudget),
		workloadPDBs: make(map[string]string),
	}
}

// AddPDB adds a pod disruption budget.
func (m *PodDisruptionBudgetManager) AddPDB(pdb *PodDisruptionBudget) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s/%s", pdb.Namespace, pdb.Name)
	m.pdbs[key] = pdb
}

// RemovePDB removes a pod disruption budget.
func (m *PodDisruptionBudgetManager) RemovePDB(namespace, name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s/%s", namespace, name)
	delete(m.pdbs, key)
}

// GetPDB returns a PDB by namespace and name.
func (m *PodDisruptionBudgetManager) GetPDB(namespace, name string) (*PodDisruptionBudget, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := fmt.Sprintf("%s/%s", namespace, name)
	pdb, exists := m.pdbs[key]
	return pdb, exists
}

// ListPDBs returns all PDBs.
func (m *PodDisruptionBudgetManager) ListPDBs() []*PodDisruptionBudget {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*PodDisruptionBudget, 0, len(m.pdbs))
	for _, pdb := range m.pdbs {
		result = append(result, pdb)
	}
	return result
}

// RegisterWorkload registers a workload with its matching PDB.
func (m *PodDisruptionBudgetManager) RegisterWorkload(workload *Workload) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for key, pdb := range m.pdbs {
		if pdb.Namespace != workload.Namespace {
			continue
		}
		if m.matchesSelector(workload, pdb.Selector) {
			m.workloadPDBs[workload.ID] = key
			return
		}
	}
}

// UnregisterWorkload removes a workload registration.
func (m *PodDisruptionBudgetManager) UnregisterWorkload(workloadID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.workloadPDBs, workloadID)
}

// CanDisrupt checks if a workload can be disrupted.
func (m *PodDisruptionBudgetManager) CanDisrupt(workload *Workload) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pdbKey, exists := m.workloadPDBs[workload.ID]
	if !exists {
		// No PDB means can disrupt
		return true
	}

	pdb, exists := m.pdbs[pdbKey]
	if !exists {
		return true
	}

	return pdb.DisruptionsAllowed > 0
}

// RecordDisruption records a disruption.
func (m *PodDisruptionBudgetManager) RecordDisruption(workload *Workload) {
	m.mu.Lock()
	defer m.mu.Unlock()

	pdbKey, exists := m.workloadPDBs[workload.ID]
	if !exists {
		return
	}

	pdb, exists := m.pdbs[pdbKey]
	if !exists {
		return
	}

	if pdb.DisruptionsAllowed > 0 {
		pdb.DisruptionsAllowed--
		pdb.CurrentHealthy--
	}
}

// RecordRecovery records a workload recovery.
func (m *PodDisruptionBudgetManager) RecordRecovery(workload *Workload) {
	m.mu.Lock()
	defer m.mu.Unlock()

	pdbKey, exists := m.workloadPDBs[workload.ID]
	if !exists {
		return
	}

	pdb, exists := m.pdbs[pdbKey]
	if !exists {
		return
	}

	pdb.CurrentHealthy++
	m.recalculateDisruptionsAllowed(pdb)
}

// recalculateDisruptionsAllowed recalculates allowed disruptions.
func (m *PodDisruptionBudgetManager) recalculateDisruptionsAllowed(pdb *PodDisruptionBudget) {
	if pdb.MinAvailable != nil {
		pdb.DisruptionsAllowed = pdb.CurrentHealthy - *pdb.MinAvailable
		if pdb.DisruptionsAllowed < 0 {
			pdb.DisruptionsAllowed = 0
		}
	} else if pdb.MaxUnavailable != nil {
		unavailable := pdb.DesiredHealthy - pdb.CurrentHealthy
		pdb.DisruptionsAllowed = *pdb.MaxUnavailable - unavailable
		if pdb.DisruptionsAllowed < 0 {
			pdb.DisruptionsAllowed = 0
		}
	}
}

// matchesSelector checks if workload matches selector.
func (m *PodDisruptionBudgetManager) matchesSelector(workload *Workload, selector LabelSelector) bool {
	for k, v := range selector.MatchLabels {
		if workload.Labels[k] != v {
			return false
		}
	}
	return true
}

// UpdatePDBStatus updates PDB status.
func (m *PodDisruptionBudgetManager) UpdatePDBStatus(namespace, name string, currentHealthy, desiredHealthy int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s/%s", namespace, name)
	pdb, exists := m.pdbs[key]
	if !exists {
		return
	}

	pdb.CurrentHealthy = currentHealthy
	pdb.DesiredHealthy = desiredHealthy
	m.recalculateDisruptionsAllowed(pdb)
}

// =============================================================================
// Preemption Anti-Affinity
// =============================================================================

// PreemptionAntiAffinityRule defines anti-affinity for preemption.
type PreemptionAntiAffinityRule struct {
	ServiceLabel string
	Duration     time.Duration
	MaxPreemptionsPerService int
}

// PreemptionAntiAffinityTracker tracks preemption anti-affinity.
type PreemptionAntiAffinityTracker struct {
	mu           sync.RWMutex
	rules        []PreemptionAntiAffinityRule
	preemptions  map[string][]time.Time // service label -> preemption times
	cooldownTime time.Duration
}

// NewPreemptionAntiAffinityTracker creates a new tracker.
func NewPreemptionAntiAffinityTracker() *PreemptionAntiAffinityTracker {
	return &PreemptionAntiAffinityTracker{
		rules:        []PreemptionAntiAffinityRule{},
		preemptions:  make(map[string][]time.Time),
		cooldownTime: 5 * time.Minute,
	}
}

// AddRule adds an anti-affinity rule.
func (t *PreemptionAntiAffinityTracker) AddRule(rule PreemptionAntiAffinityRule) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.rules = append(t.rules, rule)
}

// RemoveRule removes an anti-affinity rule.
func (t *PreemptionAntiAffinityTracker) RemoveRule(serviceLabel string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for i, r := range t.rules {
		if r.ServiceLabel == serviceLabel {
			t.rules = append(t.rules[:i], t.rules[i+1:]...)
			return
		}
	}
}

// SetCooldownTime sets the default cooldown time.
func (t *PreemptionAntiAffinityTracker) SetCooldownTime(d time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cooldownTime = d
}

// CanPreempt checks if preemption is allowed considering anti-affinity.
func (t *PreemptionAntiAffinityTracker) CanPreempt(preemptor, victim *Workload) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Get victim's service label
	serviceLabel := t.getServiceLabel(victim)
	if serviceLabel == "" {
		return true // No service label means no anti-affinity
	}

	// Check rules
	for _, rule := range t.rules {
		if rule.ServiceLabel == serviceLabel {
			preemptTimes := t.preemptions[serviceLabel]
			recentCount := 0
			now := time.Now()

			for _, pt := range preemptTimes {
				if now.Sub(pt) < rule.Duration {
					recentCount++
				}
			}

			if recentCount >= rule.MaxPreemptionsPerService {
				return false
			}
		}
	}

	// Check default cooldown
	preemptTimes := t.preemptions[serviceLabel]
	if len(preemptTimes) > 0 {
		lastPreemption := preemptTimes[len(preemptTimes)-1]
		if time.Since(lastPreemption) < t.cooldownTime {
			// Check if same service is being preempted too frequently
			recentCount := 0
			for _, pt := range preemptTimes {
				if time.Since(pt) < t.cooldownTime {
					recentCount++
				}
			}
			// Allow up to 2 preemptions per service within cooldown
			if recentCount >= 2 {
				return false
			}
		}
	}

	return true
}

// RecordPreemption records a preemption for anti-affinity tracking.
func (t *PreemptionAntiAffinityTracker) RecordPreemption(victim *Workload) {
	t.mu.Lock()
	defer t.mu.Unlock()

	serviceLabel := t.getServiceLabel(victim)
	if serviceLabel == "" {
		return
	}

	t.preemptions[serviceLabel] = append(t.preemptions[serviceLabel], time.Now())
}

// getServiceLabel extracts service label from workload.
func (t *PreemptionAntiAffinityTracker) getServiceLabel(workload *Workload) string {
	if workload.Labels == nil {
		return ""
	}

	// Try common service label keys
	for _, key := range []string{"app", "service", "app.kubernetes.io/name"} {
		if v, ok := workload.Labels[key]; ok {
			return v
		}
	}

	return ""
}

// Cleanup removes old entries.
func (t *PreemptionAntiAffinityTracker) Cleanup() {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	for service, times := range t.preemptions {
		var validTimes []time.Time
		for _, pt := range times {
			if now.Sub(pt) < 24*time.Hour {
				validTimes = append(validTimes, pt)
			}
		}
		if len(validTimes) > 0 {
			t.preemptions[service] = validTimes
		} else {
			delete(t.preemptions, service)
		}
	}
}

// GetServicePreemptionCount returns preemption count for a service.
func (t *PreemptionAntiAffinityTracker) GetServicePreemptionCount(serviceLabel string, duration time.Duration) int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	count := 0
	now := time.Now()
	for _, pt := range t.preemptions[serviceLabel] {
		if now.Sub(pt) < duration {
			count++
		}
	}
	return count
}

// =============================================================================
// Preemption Queue
// =============================================================================

// PreemptionQueueItem wraps a workload for queue ordering.
type PreemptionQueueItem struct {
	workload    *Workload
	enqueueTime time.Time
	index       int
}

// preemptionQueueHeap implements heap.Interface.
type preemptionQueueHeap []*PreemptionQueueItem

func (h preemptionQueueHeap) Len() int { return len(h) }

func (h preemptionQueueHeap) Less(i, j int) bool {
	// Higher priority first
	if h[i].workload.Priority != h[j].workload.Priority {
		return h[i].workload.Priority > h[j].workload.Priority
	}
	// Earlier enqueue time first
	return h[i].enqueueTime.Before(h[j].enqueueTime)
}

func (h preemptionQueueHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *preemptionQueueHeap) Push(x interface{}) {
	item := x.(*PreemptionQueueItem)
	item.index = len(*h)
	*h = append(*h, item)
}

func (h *preemptionQueueHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*h = old[0 : n-1]
	return item
}

// PreemptionQueue manages workloads waiting for preemption.
type PreemptionQueue struct {
	mu       sync.Mutex
	items    preemptionQueueHeap
	lookup   map[string]*PreemptionQueueItem
	maxSize  int
	cond     *sync.Cond
}

// NewPreemptionQueue creates a new preemption queue.
func NewPreemptionQueue(maxSize int) *PreemptionQueue {
	pq := &PreemptionQueue{
		items:   make(preemptionQueueHeap, 0),
		lookup:  make(map[string]*PreemptionQueueItem),
		maxSize: maxSize,
	}
	pq.cond = sync.NewCond(&pq.mu)
	heap.Init(&pq.items)
	return pq
}

// Push adds a workload to the queue.
func (q *PreemptionQueue) Push(workload *Workload) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if _, exists := q.lookup[workload.ID]; exists {
		return nil // Already in queue
	}

	if q.maxSize > 0 && len(q.items) >= q.maxSize {
		return ErrPreemptionQueueFull
	}

	item := &PreemptionQueueItem{
		workload:    workload,
		enqueueTime: time.Now(),
	}

	heap.Push(&q.items, item)
	q.lookup[workload.ID] = item
	q.cond.Signal()

	return nil
}

// Pop removes and returns the highest priority workload.
func (q *PreemptionQueue) Pop() *Workload {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) == 0 {
		return nil
	}

	item := heap.Pop(&q.items).(*PreemptionQueueItem)
	delete(q.lookup, item.workload.ID)

	return item.workload
}

// PopWait blocks until a workload is available.
func (q *PreemptionQueue) PopWait() *Workload {
	q.mu.Lock()
	defer q.mu.Unlock()

	for len(q.items) == 0 {
		q.cond.Wait()
	}

	item := heap.Pop(&q.items).(*PreemptionQueueItem)
	delete(q.lookup, item.workload.ID)

	return item.workload
}

// Remove removes a specific workload from the queue.
func (q *PreemptionQueue) Remove(workloadID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	item, exists := q.lookup[workloadID]
	if !exists {
		return false
	}

	heap.Remove(&q.items, item.index)
	delete(q.lookup, workloadID)

	return true
}

// Len returns the queue length.
func (q *PreemptionQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

// Contains checks if a workload is in the queue.
func (q *PreemptionQueue) Contains(workloadID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	_, exists := q.lookup[workloadID]
	return exists
}

// Clear empties the queue.
func (q *PreemptionQueue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = make(preemptionQueueHeap, 0)
	q.lookup = make(map[string]*PreemptionQueueItem)
	heap.Init(&q.items)
}

// List returns all workloads in the queue.
func (q *PreemptionQueue) List() []*Workload {
	q.mu.Lock()
	defer q.mu.Unlock()

	result := make([]*Workload, len(q.items))
	for i, item := range q.items {
		result[i] = item.workload
	}
	return result
}

// =============================================================================
// Preemption Metrics
// =============================================================================

// PreemptionMetrics tracks preemption statistics.
type PreemptionMetrics struct {
	mu sync.RWMutex

	// Counters
	totalAttempts     int64
	successfulPreemptions int64
	failedPreemptions int64
	totalVictims      int64

	// Latency tracking
	totalDuration     time.Duration
	maxDuration       time.Duration
	minDuration       time.Duration

	// Per-node stats
	preemptionsByNode map[string]int64

	// Per-reason stats
	failureReasons map[string]int64

	// Time series (last hour)
	hourlyPreemptions []int64
	lastHourUpdate    time.Time
}

// NewPreemptionMetrics creates a new metrics tracker.
func NewPreemptionMetrics() *PreemptionMetrics {
	return &PreemptionMetrics{
		preemptionsByNode: make(map[string]int64),
		failureReasons:    make(map[string]int64),
		hourlyPreemptions: make([]int64, 60),
		lastHourUpdate:    time.Now(),
		minDuration:       time.Duration(1<<63 - 1),
	}
}

// RecordPreemptionAttempt records a preemption attempt.
func (m *PreemptionMetrics) RecordPreemptionAttempt(success bool, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	atomic.AddInt64(&m.totalAttempts, 1)
	m.totalDuration += duration

	if duration > m.maxDuration {
		m.maxDuration = duration
	}
	if duration < m.minDuration {
		m.minDuration = duration
	}

	if success {
		atomic.AddInt64(&m.successfulPreemptions, 1)
		m.updateHourlyStats()
	} else {
		atomic.AddInt64(&m.failedPreemptions, 1)
	}
}

// RecordVictimCount records the number of victims.
func (m *PreemptionMetrics) RecordVictimCount(count int) {
	atomic.AddInt64(&m.totalVictims, int64(count))
}

// RecordNodePreemption records a preemption on a node.
func (m *PreemptionMetrics) RecordNodePreemption(nodeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.preemptionsByNode[nodeID]++
}

// RecordFailureReason records a failure reason.
func (m *PreemptionMetrics) RecordFailureReason(reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failureReasons[reason]++
}

// updateHourlyStats updates hourly statistics.
func (m *PreemptionMetrics) updateHourlyStats() {
	now := time.Now()
	minute := now.Minute()

	// Reset if hour changed
	if now.Sub(m.lastHourUpdate) > time.Hour {
		m.hourlyPreemptions = make([]int64, 60)
	}

	m.hourlyPreemptions[minute]++
	m.lastHourUpdate = now
}

// GetStats returns current metrics.
func (m *PreemptionMetrics) GetStats() PreemptionMetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	avgDuration := time.Duration(0)
	if m.totalAttempts > 0 {
		avgDuration = m.totalDuration / time.Duration(m.totalAttempts)
	}

	nodeStats := make(map[string]int64)
	for k, v := range m.preemptionsByNode {
		nodeStats[k] = v
	}

	failureStats := make(map[string]int64)
	for k, v := range m.failureReasons {
		failureStats[k] = v
	}

	// Calculate hourly rate
	var hourlyTotal int64
	for _, v := range m.hourlyPreemptions {
		hourlyTotal += v
	}

	return PreemptionMetricsSnapshot{
		TotalAttempts:         atomic.LoadInt64(&m.totalAttempts),
		SuccessfulPreemptions: atomic.LoadInt64(&m.successfulPreemptions),
		FailedPreemptions:     atomic.LoadInt64(&m.failedPreemptions),
		TotalVictims:          atomic.LoadInt64(&m.totalVictims),
		AverageDuration:       avgDuration,
		MaxDuration:           m.maxDuration,
		MinDuration:           m.minDuration,
		PreemptionsByNode:     nodeStats,
		FailureReasons:        failureStats,
		HourlyRate:            hourlyTotal,
	}
}

// PreemptionMetricsSnapshot is a point-in-time snapshot of metrics.
type PreemptionMetricsSnapshot struct {
	TotalAttempts         int64
	SuccessfulPreemptions int64
	FailedPreemptions     int64
	TotalVictims          int64
	AverageDuration       time.Duration
	MaxDuration           time.Duration
	MinDuration           time.Duration
	PreemptionsByNode     map[string]int64
	FailureReasons        map[string]int64
	HourlyRate            int64
}

// =============================================================================
// Preemption Event Log
// =============================================================================

// PreemptionEventType represents the type of preemption event.
type PreemptionEventType string

const (
	PreemptionEventDenied           PreemptionEventType = "Denied"
	PreemptionEventNoCandidates     PreemptionEventType = "NoCandidates"
	PreemptionEventSuccess          PreemptionEventType = "Success"
	PreemptionEventStartTermination PreemptionEventType = "StartTermination"
	PreemptionEventVictimTerminated PreemptionEventType = "VictimTerminated"
	PreemptionEventCompleted        PreemptionEventType = "Completed"
	PreemptionEventFailed           PreemptionEventType = "Failed"
	PreemptionEventPDBViolation     PreemptionEventType = "PDBViolation"
	PreemptionEventAntiAffinityViolation PreemptionEventType = "AntiAffinityViolation"
)

// PreemptionEvent represents a preemption event.
type PreemptionEvent struct {
	Type        PreemptionEventType
	Timestamp   time.Time
	PreemptorID string
	VictimIDs   []string
	NodeID      string
	Message     string
}

// PreemptionEventLog maintains a log of preemption events.
type PreemptionEventLog struct {
	mu       sync.RWMutex
	events   []PreemptionEvent
	maxSize  int
	handlers []func(PreemptionEvent)
}

// NewPreemptionEventLog creates a new event log.
func NewPreemptionEventLog(maxSize int) *PreemptionEventLog {
	return &PreemptionEventLog{
		events:   make([]PreemptionEvent, 0),
		maxSize:  maxSize,
		handlers: []func(PreemptionEvent){},
	}
}

// LogEvent logs a preemption event.
func (l *PreemptionEventLog) LogEvent(eventType PreemptionEventType, preemptorID string, victimIDs []string, message string, node *Node) {
	l.mu.Lock()
	defer l.mu.Unlock()

	nodeID := ""
	if node != nil {
		nodeID = node.ID
	}

	event := PreemptionEvent{
		Type:        eventType,
		Timestamp:   time.Now(),
		PreemptorID: preemptorID,
		VictimIDs:   victimIDs,
		NodeID:      nodeID,
		Message:     message,
	}

	l.events = append(l.events, event)
	if len(l.events) > l.maxSize {
		l.events = l.events[1:]
	}

	// Notify handlers
	for _, handler := range l.handlers {
		go handler(event)
	}
}

// OnEvent registers an event handler.
func (l *PreemptionEventLog) OnEvent(handler func(PreemptionEvent)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.handlers = append(l.handlers, handler)
}

// GetRecent returns recent events.
func (l *PreemptionEventLog) GetRecent(count int) []PreemptionEvent {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if count > len(l.events) {
		count = len(l.events)
	}

	result := make([]PreemptionEvent, count)
	start := len(l.events) - count
	copy(result, l.events[start:])
	return result
}

// GetByType returns events of a specific type.
func (l *PreemptionEventLog) GetByType(eventType PreemptionEventType) []PreemptionEvent {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var result []PreemptionEvent
	for _, e := range l.events {
		if e.Type == eventType {
			result = append(result, e)
		}
	}
	return result
}

// GetByWorkload returns events for a workload.
func (l *PreemptionEventLog) GetByWorkload(workloadID string) []PreemptionEvent {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var result []PreemptionEvent
	for _, e := range l.events {
		if e.PreemptorID == workloadID {
			result = append(result, e)
			continue
		}
		for _, vid := range e.VictimIDs {
			if vid == workloadID {
				result = append(result, e)
				break
			}
		}
	}
	return result
}

// Clear clears all events.
func (l *PreemptionEventLog) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = make([]PreemptionEvent, 0)
}

// =============================================================================
// Preemption History (retained from original)
// =============================================================================

// PreemptionRecord tracks preemption history.
type PreemptionRecord struct {
	ID             string
	Timestamp      time.Time
	PreemptorID    string
	PreemptorPrio  int32
	VictimIDs      []string
	NodeID         string
	ResourcesFreed Resources
	Reason         string
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
		TotalPreemptions:  len(h.records),
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

// =============================================================================
// Preemption Guard
// =============================================================================

// PreemptionGuard prevents cascading preemptions.
type PreemptionGuard struct {
	mu                sync.RWMutex
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

// SetCooldownPeriod sets the cooldown period.
func (pg *PreemptionGuard) SetCooldownPeriod(d time.Duration) {
	pg.mu.Lock()
	defer pg.mu.Unlock()
	pg.cooldownPeriod = d
}

// =============================================================================
// Graceful Terminator
// =============================================================================

// TerminationState represents the state of a termination.
type TerminationState string

const (
	TerminationStatePending   TerminationState = "Pending"
	TerminationStateGraceful  TerminationState = "Graceful"
	TerminationStateForced    TerminationState = "Forced"
	TerminationStateCompleted TerminationState = "Completed"
	TerminationStateFailed    TerminationState = "Failed"
)

// TerminationRecord tracks a termination.
type TerminationRecord struct {
	WorkloadID   string
	State        TerminationState
	StartTime    time.Time
	EndTime      time.Time
	GracePeriod  time.Duration
	Error        error
}

// GracefulTerminator handles graceful termination of workloads.
type GracefulTerminator struct {
	mu            sync.RWMutex
	gracePeriod   time.Duration
	terminations  map[string]*TerminationRecord
	onTerminating func(*Workload)
	onTerminated  func(*Workload)
}

// NewGracefulTerminator creates a new graceful terminator.
func NewGracefulTerminator(gracePeriod time.Duration) *GracefulTerminator {
	return &GracefulTerminator{
		gracePeriod:  gracePeriod,
		terminations: make(map[string]*TerminationRecord),
	}
}

// SetGracePeriod sets the grace period.
func (gt *GracefulTerminator) SetGracePeriod(d time.Duration) {
	gt.mu.Lock()
	defer gt.mu.Unlock()
	gt.gracePeriod = d
}

// OnTerminating sets a callback for when termination starts.
func (gt *GracefulTerminator) OnTerminating(fn func(*Workload)) {
	gt.mu.Lock()
	defer gt.mu.Unlock()
	gt.onTerminating = fn
}

// OnTerminated sets a callback for when termination completes.
func (gt *GracefulTerminator) OnTerminated(fn func(*Workload)) {
	gt.mu.Lock()
	defer gt.mu.Unlock()
	gt.onTerminated = fn
}

// Terminate gracefully terminates a workload.
func (gt *GracefulTerminator) Terminate(ctx context.Context, workload *Workload, terminateFunc func(*Workload) error) error {
	gt.mu.Lock()
	gracePeriod := gt.gracePeriod
	onTerminating := gt.onTerminating
	onTerminated := gt.onTerminated

	// Check for workload-specific grace period
	if workload.Annotations != nil {
		if gpStr, ok := workload.Annotations["scheduler.unheaded.io/termination-grace-period"]; ok {
			if gp, err := time.ParseDuration(gpStr); err == nil {
				gracePeriod = gp
			}
		}
	}

	record := &TerminationRecord{
		WorkloadID:  workload.ID,
		State:       TerminationStatePending,
		StartTime:   time.Now(),
		GracePeriod: gracePeriod,
	}
	gt.terminations[workload.ID] = record
	gt.mu.Unlock()

	// Notify terminating callback
	if onTerminating != nil {
		onTerminating(workload)
	}

	// Create context with grace period timeout
	ctx, cancel := context.WithTimeout(ctx, gracePeriod)
	defer cancel()

	// Start graceful termination
	gt.mu.Lock()
	record.State = TerminationStateGraceful
	gt.mu.Unlock()

	// Execute termination
	errChan := make(chan error, 1)
	go func() {
		errChan <- terminateFunc(workload)
	}()

	select {
	case err := <-errChan:
		gt.mu.Lock()
		if err != nil {
			record.State = TerminationStateFailed
			record.Error = err
		} else {
			record.State = TerminationStateCompleted
		}
		record.EndTime = time.Now()
		gt.mu.Unlock()

		if err != nil {
			return err
		}

	case <-ctx.Done():
		// Grace period expired, force terminate
		gt.mu.Lock()
		record.State = TerminationStateForced
		gt.mu.Unlock()

		// Wait for the termination to complete even after timeout
		err := <-errChan
		gt.mu.Lock()
		if err != nil {
			record.State = TerminationStateFailed
			record.Error = err
		} else {
			record.State = TerminationStateCompleted
		}
		record.EndTime = time.Now()
		gt.mu.Unlock()

		if err != nil {
			return fmt.Errorf("forced termination after grace period: %w", err)
		}
	}

	// Notify terminated callback
	if onTerminated != nil {
		onTerminated(workload)
	}

	return nil
}

// GetTerminationRecord returns the termination record for a workload.
func (gt *GracefulTerminator) GetTerminationRecord(workloadID string) (*TerminationRecord, bool) {
	gt.mu.RLock()
	defer gt.mu.RUnlock()
	record, exists := gt.terminations[workloadID]
	return record, exists
}

// ListPendingTerminations returns all pending terminations.
func (gt *GracefulTerminator) ListPendingTerminations() []*TerminationRecord {
	gt.mu.RLock()
	defer gt.mu.RUnlock()

	var result []*TerminationRecord
	for _, record := range gt.terminations {
		if record.State == TerminationStatePending || record.State == TerminationStateGraceful {
			result = append(result, record)
		}
	}
	return result
}

// CleanupCompleted removes completed termination records.
func (gt *GracefulTerminator) CleanupCompleted() {
	gt.mu.Lock()
	defer gt.mu.Unlock()

	for id, record := range gt.terminations {
		if record.State == TerminationStateCompleted || record.State == TerminationStateFailed {
			if time.Since(record.EndTime) > 5*time.Minute {
				delete(gt.terminations, id)
			}
		}
	}
}

// =============================================================================
// Victim Selector
// =============================================================================

// VictimSelectionPolicy defines victim selection behavior.
type VictimSelectionPolicy string

const (
	VictimSelectionLowestPriority VictimSelectionPolicy = "LowestPriority"
	VictimSelectionNewest         VictimSelectionPolicy = "Newest"
	VictimSelectionOldest         VictimSelectionPolicy = "Oldest"
	VictimSelectionLeastResources VictimSelectionPolicy = "LeastResources"
	VictimSelectionMostResources  VictimSelectionPolicy = "MostResources"
)

// VictimSelector selects victims for preemption.
type VictimSelector struct {
	mu              sync.RWMutex
	selectionPolicy VictimSelectionPolicy
}

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

// GetPolicy returns the current selection policy.
func (vs *VictimSelector) GetPolicy() VictimSelectionPolicy {
	vs.mu.RLock()
	defer vs.mu.RUnlock()
	return vs.selectionPolicy
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
	case VictimSelectionLeastResources:
		sort.Slice(sorted, func(i, j int) bool {
			ri := sorted[i].Resources.Requests.CPU + sorted[i].Resources.Requests.Memory
			rj := sorted[j].Resources.Requests.CPU + sorted[j].Resources.Requests.Memory
			return ri < rj
		})
	case VictimSelectionMostResources:
		sort.Slice(sorted, func(i, j int) bool {
			ri := sorted[i].Resources.Requests.CPU + sorted[i].Resources.Requests.Memory
			rj := sorted[j].Resources.Requests.CPU + sorted[j].Resources.Requests.Memory
			return ri > rj
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

// =============================================================================
// Preemption Simulator
// =============================================================================

// PreemptionSimulator simulates preemption outcomes.
type PreemptionSimulator struct {
	nodes        *NodeRegistry
	getWorkloads WorkloadGetter
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
	CanSchedule     bool
	BestNode        *Node
	VictimsRequired []*Workload
	ResourcesFreed  Resources
	ImpactScore     float64
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
