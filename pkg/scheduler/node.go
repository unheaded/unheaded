package scheduler

import (
	"fmt"
	"sync"
	"time"
)

// NodeState represents the current state of a node.
type NodeState string

const (
	NodeStateReady      NodeState = "Ready"
	NodeStateNotReady   NodeState = "NotReady"
	NodeStateMaintenance NodeState = "Maintenance"
	NodeStateDraining   NodeState = "Draining"
	NodeStateUnknown    NodeState = "Unknown"
)

// Node represents a compute node in the cluster.
type Node struct {
	mu           sync.RWMutex
	ID           string
	Name         string
	State        NodeState
	Capacity     Resources
	Allocatable  Resources
	Allocated    Resources
	Available    Resources
	Labels       map[string]string
	Annotations  map[string]string
	Taints       []Taint
	Conditions   []NodeCondition
	CreatedAt    time.Time
	LastHeartbeat time.Time
	Zone         string
	Region       string
	NodeType     string
}

// NodeCondition represents a condition of a node.
type NodeCondition struct {
	Type               NodeConditionType
	Status             ConditionStatus
	LastTransitionTime time.Time
	Reason             string
	Message            string
}

// NodeConditionType is the type of node condition.
type NodeConditionType string

const (
	NodeConditionReady              NodeConditionType = "Ready"
	NodeConditionMemoryPressure     NodeConditionType = "MemoryPressure"
	NodeConditionDiskPressure       NodeConditionType = "DiskPressure"
	NodeConditionPIDPressure        NodeConditionType = "PIDPressure"
	NodeConditionNetworkUnavailable NodeConditionType = "NetworkUnavailable"
)

// ConditionStatus represents the status of a condition.
type ConditionStatus string

const (
	ConditionTrue    ConditionStatus = "True"
	ConditionFalse   ConditionStatus = "False"
	ConditionUnknown ConditionStatus = "Unknown"
)

// Taint represents a node taint.
type Taint struct {
	Key       string
	Value     string
	Effect    TaintEffect
	TimeAdded time.Time
}

// TaintEffect is the effect of a taint.
type TaintEffect string

const (
	TaintEffectNoSchedule       TaintEffect = "NoSchedule"
	TaintEffectPreferNoSchedule TaintEffect = "PreferNoSchedule"
	TaintEffectNoExecute        TaintEffect = "NoExecute"
)

// Toleration allows a workload to tolerate a taint.
type Toleration struct {
	Key               string
	Operator          TolerationOperator
	Value             string
	Effect            TaintEffect
	TolerationSeconds *int64
}

// TolerationOperator is the operator for a toleration.
type TolerationOperator string

const (
	TolerationOpEqual  TolerationOperator = "Equal"
	TolerationOpExists TolerationOperator = "Exists"
)

// Matches checks if a toleration matches a taint.
func (t Toleration) Matches(taint Taint) bool {
	if t.Effect != "" && t.Effect != taint.Effect {
		return false
	}

	if t.Key == "" {
		return t.Operator == TolerationOpExists
	}

	if t.Key != taint.Key {
		return false
	}

	switch t.Operator {
	case TolerationOpExists:
		return true
	case TolerationOpEqual:
		return t.Value == taint.Value
	default:
		return t.Value == taint.Value
	}
}

// NewNode creates a new node with the given capacity.
func NewNode(id, name string, capacity Resources) *Node {
	return &Node{
		ID:           id,
		Name:         name,
		State:        NodeStateReady,
		Capacity:     capacity,
		Allocatable:  capacity,
		Allocated:    Resources{},
		Available:    capacity,
		Labels:       make(map[string]string),
		Annotations:  make(map[string]string),
		Taints:       []Taint{},
		Conditions:   []NodeCondition{},
		CreatedAt:    time.Now(),
		LastHeartbeat: time.Now(),
	}
}

// UpdateAvailable recalculates available resources.
// Caller must hold n.mu for writing.
func (n *Node) UpdateAvailable() {
	n.Available = n.Allocatable.Sub(n.Allocated)
}

// GetAvailable returns a snapshot of the available resources (thread-safe).
func (n *Node) GetAvailable() Resources {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.Available
}

// Allocate reserves resources on the node.
func (n *Node) Allocate(resources Resources) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if !resources.FitsIn(n.Available) {
		return ErrInsufficientResources
	}

	n.Allocated = n.Allocated.Add(resources)
	n.UpdateAvailable()
	return nil
}

// Release frees resources on the node.
func (n *Node) Release(resources Resources) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.Allocated = n.Allocated.Sub(resources)
	// Ensure we don't go negative
	if n.Allocated.CPU < 0 {
		n.Allocated.CPU = 0
	}
	if n.Allocated.Memory < 0 {
		n.Allocated.Memory = 0
	}
	if n.Allocated.Disk < 0 {
		n.Allocated.Disk = 0
	}
	if n.Allocated.GPUs < 0 {
		n.Allocated.GPUs = 0
	}
	if n.Allocated.Ephemeral < 0 {
		n.Allocated.Ephemeral = 0
	}
	n.UpdateAvailable()
}

// IsReady returns true if the node is ready for scheduling.
func (n *Node) IsReady() bool {
	if n.State != NodeStateReady {
		return false
	}

	for _, cond := range n.Conditions {
		if cond.Type == NodeConditionReady && cond.Status != ConditionTrue {
			return false
		}
	}

	return true
}

// HasTaint checks if the node has a specific taint.
func (n *Node) HasTaint(key string, effect TaintEffect) bool {
	for _, t := range n.Taints {
		if t.Key == key && t.Effect == effect {
			return true
		}
	}
	return false
}

// AddTaint adds a taint to the node.
func (n *Node) AddTaint(taint Taint) {
	for i, t := range n.Taints {
		if t.Key == taint.Key && t.Effect == taint.Effect {
			n.Taints[i] = taint
			return
		}
	}
	n.Taints = append(n.Taints, taint)
}

// RemoveTaint removes a taint from the node.
func (n *Node) RemoveTaint(key string, effect TaintEffect) bool {
	for i, t := range n.Taints {
		if t.Key == key && t.Effect == effect {
			n.Taints = append(n.Taints[:i], n.Taints[i+1:]...)
			return true
		}
	}
	return false
}

// SetCondition sets a node condition.
func (n *Node) SetCondition(condition NodeCondition) {
	for i, c := range n.Conditions {
		if c.Type == condition.Type {
			n.Conditions[i] = condition
			return
		}
	}
	n.Conditions = append(n.Conditions, condition)
}

// GetCondition returns a node condition by type.
func (n *Node) GetCondition(condType NodeConditionType) *NodeCondition {
	for _, c := range n.Conditions {
		if c.Type == condType {
			return &c
		}
	}
	return nil
}

// UtilizationRatio returns the resource utilization ratio.
func (n *Node) UtilizationRatio() float64 {
	if n.Allocatable.CPU == 0 && n.Allocatable.Memory == 0 {
		return 0
	}

	cpuUtil := float64(0)
	memUtil := float64(0)

	if n.Allocatable.CPU > 0 {
		cpuUtil = float64(n.Allocated.CPU) / float64(n.Allocatable.CPU)
	}
	if n.Allocatable.Memory > 0 {
		memUtil = float64(n.Allocated.Memory) / float64(n.Allocatable.Memory)
	}

	return (cpuUtil + memUtil) / 2
}

// CPUUtilization returns the CPU utilization ratio.
func (n *Node) CPUUtilization() float64 {
	if n.Allocatable.CPU == 0 {
		return 0
	}
	return float64(n.Allocated.CPU) / float64(n.Allocatable.CPU)
}

// MemoryUtilization returns the memory utilization ratio.
func (n *Node) MemoryUtilization() float64 {
	if n.Allocatable.Memory == 0 {
		return 0
	}
	return float64(n.Allocated.Memory) / float64(n.Allocatable.Memory)
}

// NodeRegistry manages all nodes in the cluster.
type NodeRegistry struct {
	mu    sync.RWMutex
	nodes map[string]*Node
}

// NewNodeRegistry creates a new node registry.
func NewNodeRegistry() *NodeRegistry {
	return &NodeRegistry{
		nodes: make(map[string]*Node),
	}
}

// Register adds a node to the registry.
func (r *NodeRegistry) Register(node *Node) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if node.ID == "" {
		return fmt.Errorf("node ID is required")
	}

	if _, exists := r.nodes[node.ID]; exists {
		return fmt.Errorf("node %s already registered", node.ID)
	}

	r.nodes[node.ID] = node
	return nil
}

// Unregister removes a node from the registry.
func (r *NodeRegistry) Unregister(nodeID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.nodes[nodeID]; !exists {
		return ErrNodeNotFound
	}

	delete(r.nodes, nodeID)
	return nil
}

// Get returns a node by ID.
func (r *NodeRegistry) Get(nodeID string) (*Node, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	node, exists := r.nodes[nodeID]
	if !exists {
		return nil, ErrNodeNotFound
	}

	return node, nil
}

// Update updates a node in the registry.
func (r *NodeRegistry) Update(node *Node) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.nodes[node.ID]; !exists {
		return ErrNodeNotFound
	}

	r.nodes[node.ID] = node
	return nil
}

// List returns all nodes.
func (r *NodeRegistry) List() []*Node {
	r.mu.RLock()
	defer r.mu.RUnlock()

	nodes := make([]*Node, 0, len(r.nodes))
	for _, n := range r.nodes {
		nodes = append(nodes, n)
	}
	return nodes
}

// ListReady returns all ready nodes.
func (r *NodeRegistry) ListReady() []*Node {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var ready []*Node
	for _, n := range r.nodes {
		if n.IsReady() {
			ready = append(ready, n)
		}
	}
	return ready
}

// ListByLabel returns nodes matching a label selector.
func (r *NodeRegistry) ListByLabel(key, value string) []*Node {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matched []*Node
	for _, n := range r.nodes {
		if v, ok := n.Labels[key]; ok && v == value {
			matched = append(matched, n)
		}
	}
	return matched
}

// ListByZone returns nodes in a specific zone.
func (r *NodeRegistry) ListByZone(zone string) []*Node {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matched []*Node
	for _, n := range r.nodes {
		if n.Zone == zone {
			matched = append(matched, n)
		}
	}
	return matched
}

// Count returns the number of registered nodes.
func (r *NodeRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.nodes)
}

// CountReady returns the number of ready nodes.
func (r *NodeRegistry) CountReady() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, n := range r.nodes {
		if n.IsReady() {
			count++
		}
	}
	return count
}

// GetTotalCapacity returns the total capacity of all nodes.
func (r *NodeRegistry) GetTotalCapacity() Resources {
	r.mu.RLock()
	defer r.mu.RUnlock()

	total := Resources{}
	for _, n := range r.nodes {
		total = total.Add(n.Capacity)
	}
	return total
}

// GetTotalAllocated returns the total allocated resources.
func (r *NodeRegistry) GetTotalAllocated() Resources {
	r.mu.RLock()
	defer r.mu.RUnlock()

	total := Resources{}
	for _, n := range r.nodes {
		total = total.Add(n.Allocated)
	}
	return total
}

// GetTotalAvailable returns the total available resources.
func (r *NodeRegistry) GetTotalAvailable() Resources {
	r.mu.RLock()
	defer r.mu.RUnlock()

	total := Resources{}
	for _, n := range r.nodes {
		total = total.Add(n.Available)
	}
	return total
}

// UpdateHeartbeat updates a node's last heartbeat time.
func (r *NodeRegistry) UpdateHeartbeat(nodeID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	node, exists := r.nodes[nodeID]
	if !exists {
		return ErrNodeNotFound
	}

	node.LastHeartbeat = time.Now()
	return nil
}

// MarkNotReady marks nodes that haven't sent a heartbeat as not ready.
func (r *NodeRegistry) MarkNotReady(threshold time.Duration) []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	var marked []string
	now := time.Now()

	for _, n := range r.nodes {
		if n.State == NodeStateReady && now.Sub(n.LastHeartbeat) > threshold {
			n.State = NodeStateNotReady
			n.SetCondition(NodeCondition{
				Type:               NodeConditionReady,
				Status:             ConditionFalse,
				LastTransitionTime: now,
				Reason:             "HeartbeatTimeout",
				Message:            "Node has not sent heartbeat",
			})
			marked = append(marked, n.ID)
		}
	}

	return marked
}

// NodeScorer calculates scores for nodes.
type NodeScorer struct {
	weights map[string]float64
}

// NewNodeScorer creates a new node scorer.
func NewNodeScorer() *NodeScorer {
	return &NodeScorer{
		weights: map[string]float64{
			"cpu":    1.0,
			"memory": 1.0,
			"spread": 1.0,
		},
	}
}

// SetWeight sets a scoring weight.
func (s *NodeScorer) SetWeight(name string, weight float64) {
	s.weights[name] = weight
}

// ScoreNodes scores a list of nodes for a workload.
func (s *NodeScorer) ScoreNodes(workload *Workload, nodes []*Node) map[string]int64 {
	scores := make(map[string]int64)

	for _, node := range nodes {
		score := s.scoreNode(workload, node)
		scores[node.ID] = score
	}

	return scores
}

// scoreNode calculates the score for a single node.
func (s *NodeScorer) scoreNode(workload *Workload, node *Node) int64 {
	var totalScore float64

	// Resource fit score (higher remaining resources = higher score)
	remaining := node.Available.Sub(workload.Resources.Requests)

	cpuScore := float64(0)
	if node.Allocatable.CPU > 0 {
		cpuScore = float64(remaining.CPU) / float64(node.Allocatable.CPU) * 100
	}

	memScore := float64(0)
	if node.Allocatable.Memory > 0 {
		memScore = float64(remaining.Memory) / float64(node.Allocatable.Memory) * 100
	}

	totalScore += cpuScore * s.weights["cpu"]
	totalScore += memScore * s.weights["memory"]

	// Utilization balance score
	utilization := node.UtilizationRatio()
	balanceScore := (1 - utilization) * 100 * s.weights["spread"]
	totalScore += balanceScore

	return int64(totalScore)
}

// NodePool represents a group of nodes with similar characteristics.
type NodePool struct {
	Name       string
	Labels     map[string]string
	Taints     []Taint
	MinNodes   int
	MaxNodes   int
	NodeIDs    []string
}

// NodePoolManager manages node pools.
type NodePoolManager struct {
	mu    sync.RWMutex
	pools map[string]*NodePool
}

// NewNodePoolManager creates a new node pool manager.
func NewNodePoolManager() *NodePoolManager {
	return &NodePoolManager{
		pools: make(map[string]*NodePool),
	}
}

// CreatePool creates a new node pool.
func (m *NodePoolManager) CreatePool(pool *NodePool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.pools[pool.Name]; exists {
		return fmt.Errorf("pool %s already exists", pool.Name)
	}

	m.pools[pool.Name] = pool
	return nil
}

// DeletePool removes a node pool.
func (m *NodePoolManager) DeletePool(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.pools[name]; !exists {
		return fmt.Errorf("pool %s not found", name)
	}

	delete(m.pools, name)
	return nil
}

// GetPool returns a node pool by name.
func (m *NodePoolManager) GetPool(name string) (*NodePool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pool, exists := m.pools[name]
	if !exists {
		return nil, fmt.Errorf("pool %s not found", name)
	}

	return pool, nil
}

// ListPools returns all node pools.
func (m *NodePoolManager) ListPools() []*NodePool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pools := make([]*NodePool, 0, len(m.pools))
	for _, p := range m.pools {
		pools = append(pools, p)
	}
	return pools
}

// AddNodeToPool adds a node to a pool.
func (m *NodePoolManager) AddNodeToPool(poolName, nodeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	pool, exists := m.pools[poolName]
	if !exists {
		return fmt.Errorf("pool %s not found", poolName)
	}

	for _, id := range pool.NodeIDs {
		if id == nodeID {
			return fmt.Errorf("node %s already in pool", nodeID)
		}
	}

	pool.NodeIDs = append(pool.NodeIDs, nodeID)
	return nil
}

// RemoveNodeFromPool removes a node from a pool.
func (m *NodePoolManager) RemoveNodeFromPool(poolName, nodeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	pool, exists := m.pools[poolName]
	if !exists {
		return fmt.Errorf("pool %s not found", poolName)
	}

	for i, id := range pool.NodeIDs {
		if id == nodeID {
			pool.NodeIDs = append(pool.NodeIDs[:i], pool.NodeIDs[i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("node %s not in pool", nodeID)
}

// NodeInfoSnapshot captures node state at a point in time.
type NodeInfoSnapshot struct {
	NodeID      string
	Timestamp   time.Time
	State       NodeState
	Allocated   Resources
	Available   Resources
	Utilization float64
	WorkloadIDs []string
}

// CreateSnapshot creates a snapshot of a node's state.
func CreateSnapshot(node *Node, workloadIDs []string) *NodeInfoSnapshot {
	return &NodeInfoSnapshot{
		NodeID:      node.ID,
		Timestamp:   time.Now(),
		State:       node.State,
		Allocated:   node.Allocated,
		Available:   node.Available,
		Utilization: node.UtilizationRatio(),
		WorkloadIDs: workloadIDs,
	}
}
