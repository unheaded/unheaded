// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package loadbalancer

import (
	"context"
	"hash/fnv"
	"math/rand"
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// AlgorithmState represents the state for a load balancing algorithm.
type AlgorithmState interface {
	// Select selects a backend from the list of healthy backends.
	Select(ctx context.Context, key string, backends []*Backend) (*Backend, error)
}

// NewAlgorithmState creates a new algorithm state for the given algorithm.
func NewAlgorithmState(algorithm Algorithm, backends []*Backend) AlgorithmState {
	switch algorithm {
	case AlgorithmRoundRobin:
		return NewRoundRobinState()
	case AlgorithmLeastConnections:
		return NewLeastConnectionsState()
	case AlgorithmIPHash:
		return NewIPHashState()
	case AlgorithmWeighted:
		return NewWeightedState(backends)
	case AlgorithmWeightedLeastConn:
		return NewWeightedLeastConnState(backends)
	case AlgorithmRandom:
		return NewRandomState()
	default:
		return NewRoundRobinState()
	}
}

// RoundRobinState implements round-robin load balancing.
type RoundRobinState struct {
	current uint64
}

// NewRoundRobinState creates a new round-robin state.
func NewRoundRobinState() *RoundRobinState {
	return &RoundRobinState{}
}

// Select implements AlgorithmState.
func (r *RoundRobinState) Select(ctx context.Context, key string, backends []*Backend) (*Backend, error) {
	if len(backends) == 0 {
		return nil, ErrNoHealthyBackends
	}

	// Atomic increment and wrap around
	idx := atomic.AddUint64(&r.current, 1)
	return backends[idx%uint64(len(backends))], nil
}

// LeastConnectionsState implements least-connections load balancing.
type LeastConnectionsState struct{}

// NewLeastConnectionsState creates a new least-connections state.
func NewLeastConnectionsState() *LeastConnectionsState {
	return &LeastConnectionsState{}
}

// Select implements AlgorithmState.
func (l *LeastConnectionsState) Select(ctx context.Context, key string, backends []*Backend) (*Backend, error) {
	if len(backends) == 0 {
		return nil, ErrNoHealthyBackends
	}

	var selected *Backend
	minConns := int64(^uint64(0) >> 1) // max int64

	for _, b := range backends {
		conns := b.ActiveConnections()
		if conns < minConns {
			minConns = conns
			selected = b
		}
	}

	if selected == nil {
		selected = backends[0]
	}

	return selected, nil
}

// IPHashState implements IP-hash load balancing for session persistence.
type IPHashState struct{}

// NewIPHashState creates a new IP-hash state.
func NewIPHashState() *IPHashState {
	return &IPHashState{}
}

// Select implements AlgorithmState.
// The key should be the client IP address.
func (i *IPHashState) Select(ctx context.Context, key string, backends []*Backend) (*Backend, error) {
	if len(backends) == 0 {
		return nil, ErrNoHealthyBackends
	}

	// Extract IP from address if it contains port
	ip := key
	if host, _, err := net.SplitHostPort(key); err == nil {
		ip = host
	}

	// Hash the IP
	h := fnv.New32a()
	h.Write([]byte(ip))
	hash := h.Sum32()

	// Select backend based on hash
	idx := int(hash) % len(backends)
	return backends[idx], nil
}

// WeightedState implements weighted round-robin load balancing.
type WeightedState struct {
	mu           sync.Mutex
	weights      []int
	gcd          int
	maxWeight    int
	currentIndex int
	currentWeight int
}

// NewWeightedState creates a new weighted state.
func NewWeightedState(backends []*Backend) *WeightedState {
	w := &WeightedState{
		weights: make([]int, len(backends)),
	}

	// Initialize weights
	for i, b := range backends {
		weight := b.Config.Weight
		if weight <= 0 {
			weight = 1
		}
		w.weights[i] = weight
		if weight > w.maxWeight {
			w.maxWeight = weight
		}
	}

	// Calculate GCD of all weights
	w.gcd = w.calculateGCD()
	w.currentIndex = -1
	w.currentWeight = 0

	return w
}

// calculateGCD calculates the GCD of all weights.
func (w *WeightedState) calculateGCD() int {
	if len(w.weights) == 0 {
		return 1
	}

	result := w.weights[0]
	for i := 1; i < len(w.weights); i++ {
		result = gcd(result, w.weights[i])
	}
	return result
}

// gcd calculates the greatest common divisor.
func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// Select implements AlgorithmState using weighted round-robin.
func (w *WeightedState) Select(ctx context.Context, key string, backends []*Backend) (*Backend, error) {
	if len(backends) == 0 {
		return nil, ErrNoHealthyBackends
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Rebuild weights if backend count changed
	if len(w.weights) != len(backends) {
		w.weights = make([]int, len(backends))
		w.maxWeight = 0
		for i, b := range backends {
			weight := b.Config.Weight
			if weight <= 0 {
				weight = 1
			}
			w.weights[i] = weight
			if weight > w.maxWeight {
				w.maxWeight = weight
			}
		}
		w.gcd = w.calculateGCD()
		w.currentIndex = -1
		w.currentWeight = 0
	}

	// Weighted round-robin selection (Nginx-style)
	for {
		w.currentIndex = (w.currentIndex + 1) % len(backends)
		if w.currentIndex == 0 {
			w.currentWeight -= w.gcd
			if w.currentWeight <= 0 {
				w.currentWeight = w.maxWeight
				if w.currentWeight == 0 {
					return backends[0], nil
				}
			}
		}

		if w.weights[w.currentIndex] >= w.currentWeight {
			return backends[w.currentIndex], nil
		}
	}
}

// WeightedLeastConnState combines weights with connection count.
type WeightedLeastConnState struct {
	weights []int
}

// NewWeightedLeastConnState creates a new weighted least-connections state.
func NewWeightedLeastConnState(backends []*Backend) *WeightedLeastConnState {
	w := &WeightedLeastConnState{
		weights: make([]int, len(backends)),
	}

	for i, b := range backends {
		weight := b.Config.Weight
		if weight <= 0 {
			weight = 1
		}
		w.weights[i] = weight
	}

	return w
}

// Select implements AlgorithmState.
func (w *WeightedLeastConnState) Select(ctx context.Context, key string, backends []*Backend) (*Backend, error) {
	if len(backends) == 0 {
		return nil, ErrNoHealthyBackends
	}

	// Rebuild weights if backend count changed
	if len(w.weights) != len(backends) {
		w.weights = make([]int, len(backends))
		for i, b := range backends {
			weight := b.Config.Weight
			if weight <= 0 {
				weight = 1
			}
			w.weights[i] = weight
		}
	}

	var selected *Backend
	minScore := float64(^uint64(0) >> 1) // max float64

	for i, b := range backends {
		weight := w.weights[i]
		if weight == 0 {
			weight = 1
		}

		// Score = connections / weight (lower is better)
		score := float64(b.ActiveConnections()) / float64(weight)
		if score < minScore {
			minScore = score
			selected = b
		}
	}

	if selected == nil {
		selected = backends[0]
	}

	return selected, nil
}

// RandomState implements random load balancing.
type RandomState struct {
	rng *rand.Rand
	mu  sync.Mutex
}

// NewRandomState creates a new random state.
func NewRandomState() *RandomState {
	return &RandomState{
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Select implements AlgorithmState.
func (r *RandomState) Select(ctx context.Context, key string, backends []*Backend) (*Backend, error) {
	if len(backends) == 0 {
		return nil, ErrNoHealthyBackends
	}

	r.mu.Lock()
	idx := r.rng.Intn(len(backends))
	r.mu.Unlock()

	return backends[idx], nil
}

// ConsistentHashState implements consistent hashing for better session persistence.
type ConsistentHashState struct {
	mu       sync.RWMutex
	ring     []hashNode
	replicas int
}

type hashNode struct {
	hash    uint32
	backend *Backend
}

// NewConsistentHashState creates a new consistent hash state.
func NewConsistentHashState(backends []*Backend, replicas int) *ConsistentHashState {
	if replicas <= 0 {
		replicas = 150
	}

	ch := &ConsistentHashState{
		replicas: replicas,
	}
	ch.rebuild(backends)
	return ch
}

// rebuild rebuilds the hash ring.
func (ch *ConsistentHashState) rebuild(backends []*Backend) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	ch.ring = make([]hashNode, 0, len(backends)*ch.replicas)

	for _, b := range backends {
		for i := 0; i < ch.replicas; i++ {
			key := b.Config.Name + ":" + string(rune(i))
			h := fnv.New32a()
			h.Write([]byte(key))
			ch.ring = append(ch.ring, hashNode{
				hash:    h.Sum32(),
				backend: b,
			})
		}
	}

	// Sort ring by hash
	sort.Slice(ch.ring, func(i, j int) bool {
		return ch.ring[i].hash < ch.ring[j].hash
	})
}

// Select implements AlgorithmState using consistent hashing.
func (ch *ConsistentHashState) Select(ctx context.Context, key string, backends []*Backend) (*Backend, error) {
	if len(backends) == 0 {
		return nil, ErrNoHealthyBackends
	}

	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if len(ch.ring) == 0 {
		return backends[0], nil
	}

	// Hash the key
	h := fnv.New32a()
	h.Write([]byte(key))
	hash := h.Sum32()

	// Binary search for the first node with hash >= key hash
	idx := sort.Search(len(ch.ring), func(i int) bool {
		return ch.ring[i].hash >= hash
	})

	// Wrap around if necessary
	if idx >= len(ch.ring) {
		idx = 0
	}

	// Find first healthy backend
	for i := 0; i < len(ch.ring); i++ {
		node := ch.ring[(idx+i)%len(ch.ring)]
		if node.backend.IsHealthy() {
			return node.backend, nil
		}
	}

	// Fallback to first backend
	return backends[0], nil
}

// PriorityGroupState implements priority-based load balancing with failover.
type PriorityGroupState struct {
	mu     sync.RWMutex
	groups map[int][]*Backend
	inner  AlgorithmState
}

// NewPriorityGroupState creates a new priority group state.
func NewPriorityGroupState(backends []*Backend, innerAlgorithm Algorithm) *PriorityGroupState {
	pg := &PriorityGroupState{
		groups: make(map[int][]*Backend),
	}

	// Group backends by priority
	for _, b := range backends {
		priority := b.Config.Priority
		pg.groups[priority] = append(pg.groups[priority], b)
	}

	// Create inner algorithm for selection within a priority group
	pg.inner = NewAlgorithmState(innerAlgorithm, backends)

	return pg
}

// Select implements AlgorithmState with priority groups.
func (pg *PriorityGroupState) Select(ctx context.Context, key string, backends []*Backend) (*Backend, error) {
	if len(backends) == 0 {
		return nil, ErrNoHealthyBackends
	}

	pg.mu.RLock()
	defer pg.mu.RUnlock()

	// Get sorted priorities
	priorities := make([]int, 0, len(pg.groups))
	for p := range pg.groups {
		priorities = append(priorities, p)
	}
	sort.Ints(priorities)

	// Find highest priority group with healthy backends
	for _, priority := range priorities {
		group := pg.groups[priority]
		healthy := make([]*Backend, 0)
		for _, b := range group {
			if b.IsHealthy() {
				healthy = append(healthy, b)
			}
		}
		if len(healthy) > 0 {
			return pg.inner.Select(ctx, key, healthy)
		}
	}

	return nil, ErrNoHealthyBackends
}

// SmoothWeightedRRState implements smooth weighted round-robin (NGINX-style).
type SmoothWeightedRRState struct {
	mu      sync.Mutex
	weights map[*Backend]*smoothWeight
}

type smoothWeight struct {
	weight          int
	currentWeight   int
	effectiveWeight int
}

// NewSmoothWeightedRRState creates a new smooth weighted round-robin state.
func NewSmoothWeightedRRState(backends []*Backend) *SmoothWeightedRRState {
	sw := &SmoothWeightedRRState{
		weights: make(map[*Backend]*smoothWeight),
	}

	for _, b := range backends {
		weight := b.Config.Weight
		if weight <= 0 {
			weight = 1
		}
		sw.weights[b] = &smoothWeight{
			weight:          weight,
			effectiveWeight: weight,
			currentWeight:   0,
		}
	}

	return sw
}

// Select implements AlgorithmState using smooth weighted round-robin.
func (sw *SmoothWeightedRRState) Select(ctx context.Context, key string, backends []*Backend) (*Backend, error) {
	if len(backends) == 0 {
		return nil, ErrNoHealthyBackends
	}

	sw.mu.Lock()
	defer sw.mu.Unlock()

	// Ensure all backends have weights
	for _, b := range backends {
		if _, ok := sw.weights[b]; !ok {
			weight := b.Config.Weight
			if weight <= 0 {
				weight = 1
			}
			sw.weights[b] = &smoothWeight{
				weight:          weight,
				effectiveWeight: weight,
				currentWeight:   0,
			}
		}
	}

	var selected *Backend
	totalWeight := 0

	// Calculate total weight and find max current weight
	for _, b := range backends {
		w := sw.weights[b]
		w.currentWeight += w.effectiveWeight
		totalWeight += w.effectiveWeight

		if selected == nil || w.currentWeight > sw.weights[selected].currentWeight {
			selected = b
		}
	}

	if selected == nil {
		return backends[0], nil
	}

	// Reduce current weight of selected backend
	sw.weights[selected].currentWeight -= totalWeight

	return selected, nil
}

// LeastResponseTimeState selects based on response time.
type LeastResponseTimeState struct{}

// NewLeastResponseTimeState creates a new least response time state.
func NewLeastResponseTimeState() *LeastResponseTimeState {
	return &LeastResponseTimeState{}
}

// Select implements AlgorithmState.
func (l *LeastResponseTimeState) Select(ctx context.Context, key string, backends []*Backend) (*Backend, error) {
	if len(backends) == 0 {
		return nil, ErrNoHealthyBackends
	}

	var selected *Backend
	minResponseTime := time.Duration(^uint64(0) >> 1)

	for _, b := range backends {
		avg, _, _, _ := b.metrics.ResponseTimes.Stats()
		if avg == 0 {
			// No data yet, use this backend
			return b, nil
		}
		if avg < minResponseTime {
			minResponseTime = avg
			selected = b
		}
	}

	if selected == nil {
		selected = backends[0]
	}

	return selected, nil
}

// ResourceBasedState selects based on backend resource utilization.
type ResourceBasedState struct {
	mu     sync.RWMutex
	scores map[*Backend]float64
}

// NewResourceBasedState creates a new resource-based state.
func NewResourceBasedState() *ResourceBasedState {
	return &ResourceBasedState{
		scores: make(map[*Backend]float64),
	}
}

// UpdateScore updates the resource score for a backend.
func (r *ResourceBasedState) UpdateScore(backend *Backend, score float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.scores[backend] = score
}

// Select implements AlgorithmState based on resource scores.
func (r *ResourceBasedState) Select(ctx context.Context, key string, backends []*Backend) (*Backend, error) {
	if len(backends) == 0 {
		return nil, ErrNoHealthyBackends
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var selected *Backend
	minScore := float64(^uint64(0) >> 1)

	for _, b := range backends {
		score, ok := r.scores[b]
		if !ok {
			// No score, use connection count as fallback
			score = float64(b.ActiveConnections())
		}
		if score < minScore {
			minScore = score
			selected = b
		}
	}

	if selected == nil {
		selected = backends[0]
	}

	return selected, nil
}

// TwoRandomChoicesState implements power of two random choices.
// It selects two random backends and picks the one with fewer connections.
type TwoRandomChoicesState struct {
	rng *rand.Rand
	mu  sync.Mutex
}

// NewTwoRandomChoicesState creates a new two random choices state.
func NewTwoRandomChoicesState() *TwoRandomChoicesState {
	return &TwoRandomChoicesState{
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Select implements AlgorithmState using power of two random choices.
func (t *TwoRandomChoicesState) Select(ctx context.Context, key string, backends []*Backend) (*Backend, error) {
	if len(backends) == 0 {
		return nil, ErrNoHealthyBackends
	}

	if len(backends) == 1 {
		return backends[0], nil
	}

	t.mu.Lock()
	idx1 := t.rng.Intn(len(backends))
	idx2 := t.rng.Intn(len(backends))
	for idx2 == idx1 {
		idx2 = t.rng.Intn(len(backends))
	}
	t.mu.Unlock()

	b1, b2 := backends[idx1], backends[idx2]

	// Pick the one with fewer connections
	if b1.ActiveConnections() <= b2.ActiveConnections() {
		return b1, nil
	}
	return b2, nil
}

// SourcePortHashState uses source port for hashing (useful for UDP).
type SourcePortHashState struct{}

// NewSourcePortHashState creates a new source port hash state.
func NewSourcePortHashState() *SourcePortHashState {
	return &SourcePortHashState{}
}

// Select implements AlgorithmState using source port hash.
func (s *SourcePortHashState) Select(ctx context.Context, key string, backends []*Backend) (*Backend, error) {
	if len(backends) == 0 {
		return nil, ErrNoHealthyBackends
	}

	// Extract port from address
	port := key
	if _, p, err := net.SplitHostPort(key); err == nil {
		port = p
	}

	// Hash the port
	h := fnv.New32a()
	h.Write([]byte(port))
	hash := h.Sum32()

	return backends[int(hash)%len(backends)], nil
}

// MaglevHashState implements Maglev consistent hashing.
// https://research.google/pubs/pub44824/
type MaglevHashState struct {
	mu          sync.RWMutex
	table       []*Backend
	tableSize   int
	permutation [][]int
	backends    []*Backend
}

// NewMaglevHashState creates a new Maglev hash state.
func NewMaglevHashState(backends []*Backend, tableSize int) *MaglevHashState {
	if tableSize <= 0 {
		tableSize = 65537 // Prime number
	}

	m := &MaglevHashState{
		tableSize: tableSize,
		backends:  backends,
	}
	m.rebuild()
	return m
}

func (m *MaglevHashState) rebuild() {
	m.mu.Lock()
	defer m.mu.Unlock()

	n := len(m.backends)
	if n == 0 {
		m.table = nil
		return
	}

	// Generate permutation table
	m.permutation = make([][]int, n)
	for i, b := range m.backends {
		m.permutation[i] = m.generatePermutation(b.Config.Name)
	}

	// Populate lookup table
	m.table = make([]*Backend, m.tableSize)
	next := make([]int, n)

	for filled := 0; filled < m.tableSize; {
		for i := 0; i < n; i++ {
			c := m.permutation[i][next[i]]
			for m.table[c] != nil {
				next[i]++
				c = m.permutation[i][next[i]]
			}
			m.table[c] = m.backends[i]
			next[i]++
			filled++
			if filled >= m.tableSize {
				break
			}
		}
	}
}

func (m *MaglevHashState) generatePermutation(name string) []int {
	h1 := fnv.New64()
	h1.Write([]byte(name))
	offset := h1.Sum64() % uint64(m.tableSize)

	h2 := fnv.New64a()
	h2.Write([]byte(name))
	skip := h2.Sum64()%(uint64(m.tableSize)-1) + 1

	permutation := make([]int, m.tableSize)
	for i := 0; i < m.tableSize; i++ {
		permutation[i] = int((offset + uint64(i)*skip) % uint64(m.tableSize))
	}
	return permutation
}

// Select implements AlgorithmState using Maglev hashing.
func (m *MaglevHashState) Select(ctx context.Context, key string, backends []*Backend) (*Backend, error) {
	if len(backends) == 0 {
		return nil, ErrNoHealthyBackends
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.table == nil || len(m.table) == 0 {
		return backends[0], nil
	}

	h := fnv.New64a()
	h.Write([]byte(key))
	idx := h.Sum64() % uint64(m.tableSize)

	// Find first healthy backend starting from idx
	for i := 0; i < m.tableSize; i++ {
		backend := m.table[(int(idx)+i)%m.tableSize]
		if backend != nil && backend.IsHealthy() {
			return backend, nil
		}
	}

	return backends[0], nil
}

// ChainedAlgorithm combines multiple algorithms with fallback.
type ChainedAlgorithm struct {
	algorithms []AlgorithmState
}

// NewChainedAlgorithm creates a chained algorithm.
func NewChainedAlgorithm(algorithms ...AlgorithmState) *ChainedAlgorithm {
	return &ChainedAlgorithm{
		algorithms: algorithms,
	}
}

// Select tries each algorithm in order until one succeeds.
func (c *ChainedAlgorithm) Select(ctx context.Context, key string, backends []*Backend) (*Backend, error) {
	for _, alg := range c.algorithms {
		backend, err := alg.Select(ctx, key, backends)
		if err == nil && backend != nil {
			return backend, nil
		}
	}
	return nil, ErrNoHealthyBackends
}
