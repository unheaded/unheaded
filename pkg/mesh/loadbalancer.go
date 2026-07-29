// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package mesh provides THE HAUBERK - the service mesh implementation for the Unheaded Kingdom.
// This file implements load balancing strategies: round-robin, least-connections, weighted, and more.
package mesh

import (
	"hash/fnv"
	"math/rand"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Load balancer type constants.
const (
	LoadBalancerRoundRobin = "round-robin"
	LoadBalancerLeastConn  = "least-conn"
	LoadBalancerWeighted   = "weighted"
	LoadBalancerRandom     = "random"
	LoadBalancerIPHash     = "ip-hash"
	LoadBalancerConsistent = "consistent-hash"
)

// LoadBalancer is the interface for load balancing strategies.
type LoadBalancer interface {
	// Select selects an endpoint from the list.
	Select(endpoints []*Endpoint) *Endpoint
	// SelectWithContext selects an endpoint with additional context.
	SelectWithContext(endpoints []*Endpoint, ctx *SelectContext) *Endpoint
	// Name returns the load balancer name.
	Name() string
	// Update notifies the balancer of endpoint changes.
	Update(endpoints []*Endpoint)
	// RecordResponse records response metrics for adaptive load balancing.
	RecordResponse(endpoint *Endpoint, latency time.Duration, success bool)
}

// SelectContext provides additional context for load balancer selection.
type SelectContext struct {
	ClientIP    string            // Client IP for IP hash
	Headers     map[string]string // Request headers for header-based routing
	Path        string            // Request path for path-based routing
	ServiceName string            // Service name
	Retry       int               // Retry attempt number
}

// BaseBalancer provides common functionality for load balancers.
type BaseBalancer struct {
	name     string
	mu       sync.RWMutex
	metrics  map[string]*EndpointMetrics
	lastUsed map[string]time.Time
}

// EndpointMetrics tracks metrics for an endpoint.
type EndpointMetrics struct {
	ActiveConnections int64
	TotalRequests     uint64
	SuccessCount      uint64
	FailureCount      uint64
	TotalLatencyNs    uint64
	LastResponseTime  time.Time
	MovingAvgLatency  float64
	WeightedScore     float64
}

// newBaseBalancer creates a new base balancer.
func newBaseBalancer(name string) *BaseBalancer {
	return &BaseBalancer{
		name:     name,
		metrics:  make(map[string]*EndpointMetrics),
		lastUsed: make(map[string]time.Time),
	}
}

// Name returns the balancer name.
func (b *BaseBalancer) Name() string {
	return b.name
}

// Update updates the endpoint list.
func (b *BaseBalancer) Update(endpoints []*Endpoint) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Initialize metrics for new endpoints
	existing := make(map[string]bool)
	for _, ep := range endpoints {
		existing[ep.ID] = true
		if _, ok := b.metrics[ep.ID]; !ok {
			b.metrics[ep.ID] = &EndpointMetrics{}
		}
	}

	// Remove metrics for removed endpoints
	for id := range b.metrics {
		if !existing[id] {
			delete(b.metrics, id)
			delete(b.lastUsed, id)
		}
	}
}

// RecordResponse records response metrics.
func (b *BaseBalancer) RecordResponse(endpoint *Endpoint, latency time.Duration, success bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	m, ok := b.metrics[endpoint.ID]
	if !ok {
		m = &EndpointMetrics{}
		b.metrics[endpoint.ID] = m
	}

	atomic.AddUint64(&m.TotalRequests, 1)
	atomic.AddUint64(&m.TotalLatencyNs, uint64(latency.Nanoseconds())) // #nosec G115 -- bounded by construction; see the surrounding guard
	m.LastResponseTime = time.Now()

	if success {
		atomic.AddUint64(&m.SuccessCount, 1)
	} else {
		atomic.AddUint64(&m.FailureCount, 1)
	}

	// Update moving average latency (exponential moving average)
	alpha := 0.3
	m.MovingAvgLatency = alpha*float64(latency.Nanoseconds())/1e6 + (1-alpha)*m.MovingAvgLatency
}

// GetMetrics returns metrics for an endpoint.
func (b *BaseBalancer) GetMetrics(endpointID string) *EndpointMetrics {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.metrics[endpointID]
}

// RoundRobinBalancer implements round-robin load balancing.
type RoundRobinBalancer struct {
	*BaseBalancer
	counter uint64
}

// NewRoundRobinBalancer creates a new round-robin balancer.
func NewRoundRobinBalancer() *RoundRobinBalancer {
	return &RoundRobinBalancer{
		BaseBalancer: newBaseBalancer(LoadBalancerRoundRobin),
	}
}

// Select selects an endpoint using round-robin.
func (r *RoundRobinBalancer) Select(endpoints []*Endpoint) *Endpoint {
	return r.SelectWithContext(endpoints, nil)
}

// SelectWithContext selects an endpoint with context.
func (r *RoundRobinBalancer) SelectWithContext(endpoints []*Endpoint, ctx *SelectContext) *Endpoint {
	n := len(endpoints)
	if n == 0 {
		return nil
	}

	// Atomic increment and wrap
	idx := atomic.AddUint64(&r.counter, 1) - 1
	return endpoints[idx%uint64(n)]
}

// LeastConnBalancer implements least-connections load balancing.
type LeastConnBalancer struct {
	*BaseBalancer
	connections map[string]*int64
	connMu      sync.RWMutex
}

// NewLeastConnBalancer creates a new least-connections balancer.
func NewLeastConnBalancer() *LeastConnBalancer {
	return &LeastConnBalancer{
		BaseBalancer: newBaseBalancer(LoadBalancerLeastConn),
		connections:  make(map[string]*int64),
	}
}

// Select selects the endpoint with the least active connections.
func (l *LeastConnBalancer) Select(endpoints []*Endpoint) *Endpoint {
	return l.SelectWithContext(endpoints, nil)
}

// SelectWithContext selects an endpoint with context.
func (l *LeastConnBalancer) SelectWithContext(endpoints []*Endpoint, ctx *SelectContext) *Endpoint {
	n := len(endpoints)
	if n == 0 {
		return nil
	}

	l.connMu.Lock()
	// Ensure all endpoints have connection counters
	for _, ep := range endpoints {
		if _, ok := l.connections[ep.ID]; !ok {
			var zero int64
			l.connections[ep.ID] = &zero
		}
	}
	l.connMu.Unlock()

	l.connMu.RLock()
	defer l.connMu.RUnlock()

	// Find endpoint with least connections
	var minConn int64 = -1
	var selected *Endpoint

	for _, ep := range endpoints {
		conn := atomic.LoadInt64(l.connections[ep.ID])
		if minConn == -1 || conn < minConn {
			minConn = conn
			selected = ep
		}
	}

	// Increment connection count for selected endpoint
	if selected != nil {
		atomic.AddInt64(l.connections[selected.ID], 1)
	}

	return selected
}

// ReleaseConnection decrements the connection count.
func (l *LeastConnBalancer) ReleaseConnection(endpoint *Endpoint) {
	l.connMu.RLock()
	defer l.connMu.RUnlock()

	if counter, ok := l.connections[endpoint.ID]; ok {
		atomic.AddInt64(counter, -1)
	}
}

// Update updates endpoints and cleans up counters.
func (l *LeastConnBalancer) Update(endpoints []*Endpoint) {
	l.BaseBalancer.Update(endpoints)

	l.connMu.Lock()
	defer l.connMu.Unlock()

	// Build set of valid endpoint IDs
	valid := make(map[string]bool)
	for _, ep := range endpoints {
		valid[ep.ID] = true
	}

	// Remove counters for removed endpoints
	for id := range l.connections {
		if !valid[id] {
			delete(l.connections, id)
		}
	}
}

// WeightedBalancer implements weighted load balancing.
type WeightedBalancer struct {
	*BaseBalancer
	rng     *rand.Rand
	rngLock sync.Mutex
}

// NewWeightedBalancer creates a new weighted balancer.
func NewWeightedBalancer() *WeightedBalancer {
	return &WeightedBalancer{
		BaseBalancer: newBaseBalancer(LoadBalancerWeighted),
		rng:          rand.New(rand.NewSource(time.Now().UnixNano())), // #nosec G404 -- backend selection, not security-bearing
	}
}

// Select selects an endpoint based on weights.
func (w *WeightedBalancer) Select(endpoints []*Endpoint) *Endpoint {
	return w.SelectWithContext(endpoints, nil)
}

// SelectWithContext selects an endpoint with context.
func (w *WeightedBalancer) SelectWithContext(endpoints []*Endpoint, ctx *SelectContext) *Endpoint {
	n := len(endpoints)
	if n == 0 {
		return nil
	}

	// Calculate total weight
	totalWeight := 0
	for _, ep := range endpoints {
		weight := ep.Weight
		if weight <= 0 {
			weight = 1
		}
		totalWeight += weight
	}

	if totalWeight == 0 {
		return endpoints[0]
	}

	// Select random weight
	w.rngLock.Lock()
	target := w.rng.Intn(totalWeight)
	w.rngLock.Unlock()

	// Find endpoint at target weight
	current := 0
	for _, ep := range endpoints {
		weight := ep.Weight
		if weight <= 0 {
			weight = 1
		}
		current += weight
		if target < current {
			return ep
		}
	}

	return endpoints[n-1]
}

// RandomBalancer implements random load balancing.
type RandomBalancer struct {
	*BaseBalancer
	rng     *rand.Rand
	rngLock sync.Mutex
}

// NewRandomBalancer creates a new random balancer.
func NewRandomBalancer() *RandomBalancer {
	return &RandomBalancer{
		BaseBalancer: newBaseBalancer(LoadBalancerRandom),
		rng:          rand.New(rand.NewSource(time.Now().UnixNano())), // #nosec G404 -- backend selection, not security-bearing
	}
}

// Select selects a random endpoint.
func (r *RandomBalancer) Select(endpoints []*Endpoint) *Endpoint {
	return r.SelectWithContext(endpoints, nil)
}

// SelectWithContext selects an endpoint with context.
func (r *RandomBalancer) SelectWithContext(endpoints []*Endpoint, ctx *SelectContext) *Endpoint {
	n := len(endpoints)
	if n == 0 {
		return nil
	}

	r.rngLock.Lock()
	idx := r.rng.Intn(n)
	r.rngLock.Unlock()

	return endpoints[idx]
}

// IPHashBalancer implements IP-based consistent hashing.
type IPHashBalancer struct {
	*BaseBalancer
}

// NewIPHashBalancer creates a new IP hash balancer.
func NewIPHashBalancer() *IPHashBalancer {
	return &IPHashBalancer{
		BaseBalancer: newBaseBalancer(LoadBalancerIPHash),
	}
}

// Select selects an endpoint - requires context with ClientIP.
func (h *IPHashBalancer) Select(endpoints []*Endpoint) *Endpoint {
	// Without context, fall back to round-robin behavior
	return h.SelectWithContext(endpoints, nil)
}

// SelectWithContext selects an endpoint based on client IP hash.
func (h *IPHashBalancer) SelectWithContext(endpoints []*Endpoint, ctx *SelectContext) *Endpoint {
	n := len(endpoints)
	if n == 0 {
		return nil
	}

	var key string
	if ctx != nil && ctx.ClientIP != "" {
		key = ctx.ClientIP
	} else {
		// Fall back to random selection
		return endpoints[time.Now().UnixNano()%int64(n)]
	}

	// FNV-1a hash
	hasher := fnv.New32a()
	hasher.Write([]byte(key)) // #nosec G104 -- hash.Write never returns an error (documented in hash.Hash)
	hash := hasher.Sum32()

	return endpoints[hash%uint32(n)] // #nosec G115 -- bounded by construction; see the surrounding guard
}

// ConsistentHashBalancer implements consistent hashing with virtual nodes.
type ConsistentHashBalancer struct {
	*BaseBalancer
	virtualNodes int
	ring         []uint32
	ringMap      map[uint32]*Endpoint
	ringMu       sync.RWMutex
}

// NewConsistentHashBalancer creates a new consistent hash balancer.
func NewConsistentHashBalancer(virtualNodes int) *ConsistentHashBalancer {
	if virtualNodes <= 0 {
		virtualNodes = 100
	}

	return &ConsistentHashBalancer{
		BaseBalancer: newBaseBalancer(LoadBalancerConsistent),
		virtualNodes: virtualNodes,
		ringMap:      make(map[uint32]*Endpoint),
	}
}

// Update rebuilds the hash ring.
func (c *ConsistentHashBalancer) Update(endpoints []*Endpoint) {
	c.BaseBalancer.Update(endpoints)

	c.ringMu.Lock()
	defer c.ringMu.Unlock()

	// Rebuild ring
	c.ring = make([]uint32, 0, len(endpoints)*c.virtualNodes)
	c.ringMap = make(map[uint32]*Endpoint)

	for _, ep := range endpoints {
		for i := 0; i < c.virtualNodes; i++ {
			key := ep.ID + ":" + strconv.Itoa(i)
			hash := c.hash(key)
			c.ring = append(c.ring, hash)
			c.ringMap[hash] = ep
		}
	}

	// Sort ring
	sort.Slice(c.ring, func(i, j int) bool {
		return c.ring[i] < c.ring[j]
	})
}

// Select selects an endpoint - requires context.
func (c *ConsistentHashBalancer) Select(endpoints []*Endpoint) *Endpoint {
	return c.SelectWithContext(endpoints, nil)
}

// SelectWithContext selects an endpoint based on consistent hash.
func (c *ConsistentHashBalancer) SelectWithContext(endpoints []*Endpoint, ctx *SelectContext) *Endpoint {
	c.ringMu.RLock()
	defer c.ringMu.RUnlock()

	if len(c.ring) == 0 {
		return nil
	}

	var key string
	if ctx != nil {
		if ctx.ClientIP != "" {
			key = ctx.ClientIP
		} else if ctx.Path != "" {
			key = ctx.Path
		}
	}

	if key == "" {
		key = strconv.Itoa(int(time.Now().UnixNano()))
	}

	hash := c.hash(key)

	// Binary search for the first hash >= target
	idx := sort.Search(len(c.ring), func(i int) bool {
		return c.ring[i] >= hash
	})

	// Wrap around
	if idx == len(c.ring) {
		idx = 0
	}

	return c.ringMap[c.ring[idx]]
}

// hash computes FNV-1a hash.
func (c *ConsistentHashBalancer) hash(key string) uint32 {
	hasher := fnv.New32a()
	hasher.Write([]byte(key)) // #nosec G104 -- hash.Write never returns an error (documented in hash.Hash)
	return hasher.Sum32()
}

// AdaptiveBalancer implements adaptive load balancing based on response times.
type AdaptiveBalancer struct {
	*BaseBalancer
	fallback LoadBalancer
	alpha    float64 // Smoothing factor for EMA
}

// NewAdaptiveBalancer creates a new adaptive balancer.
func NewAdaptiveBalancer() *AdaptiveBalancer {
	return &AdaptiveBalancer{
		BaseBalancer: newBaseBalancer("adaptive"),
		fallback:     NewRoundRobinBalancer(),
		alpha:        0.3,
	}
}

// Select selects the endpoint with the best performance score.
func (a *AdaptiveBalancer) Select(endpoints []*Endpoint) *Endpoint {
	return a.SelectWithContext(endpoints, nil)
}

// SelectWithContext selects an endpoint with context.
func (a *AdaptiveBalancer) SelectWithContext(endpoints []*Endpoint, ctx *SelectContext) *Endpoint {
	n := len(endpoints)
	if n == 0 {
		return nil
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	// Calculate scores for each endpoint
	type scored struct {
		endpoint *Endpoint
		score    float64
	}

	scores := make([]scored, 0, n)

	for _, ep := range endpoints {
		m := a.metrics[ep.ID]
		if m == nil || m.TotalRequests == 0 {
			// No metrics yet, use fallback
			scores = append(scores, scored{ep, 0})
			continue
		}

		// Score based on:
		// - Success rate (higher is better)
		// - Latency (lower is better)
		// - Active connections (lower is better)
		successRate := float64(m.SuccessCount) / float64(m.TotalRequests)
		latencyScore := 1.0 / (m.MovingAvgLatency + 1) // Avoid division by zero
		connScore := 1.0 / (float64(m.ActiveConnections) + 1)

		score := successRate*0.5 + latencyScore*0.3 + connScore*0.2
		scores = append(scores, scored{ep, score})
	}

	// Sort by score (descending)
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})

	// Select highest scoring endpoint
	return scores[0].endpoint
}

// P2CBalancer implements Power of Two Choices load balancing.
// It randomly selects two endpoints and picks the one with fewer connections.
type P2CBalancer struct {
	*BaseBalancer
	connections map[string]*int64
	connMu      sync.RWMutex
	rng         *rand.Rand
	rngLock     sync.Mutex
}

// NewP2CBalancer creates a new Power of Two Choices balancer.
func NewP2CBalancer() *P2CBalancer {
	return &P2CBalancer{
		BaseBalancer: newBaseBalancer("p2c"),
		connections:  make(map[string]*int64),
		rng:          rand.New(rand.NewSource(time.Now().UnixNano())), // #nosec G404 -- backend selection, not security-bearing
	}
}

// Select selects an endpoint using P2C algorithm.
func (p *P2CBalancer) Select(endpoints []*Endpoint) *Endpoint {
	return p.SelectWithContext(endpoints, nil)
}

// SelectWithContext selects an endpoint with context.
func (p *P2CBalancer) SelectWithContext(endpoints []*Endpoint, ctx *SelectContext) *Endpoint {
	n := len(endpoints)
	if n == 0 {
		return nil
	}

	if n == 1 {
		return endpoints[0]
	}

	p.connMu.Lock()
	// Ensure all endpoints have connection counters
	for _, ep := range endpoints {
		if _, ok := p.connections[ep.ID]; !ok {
			var zero int64
			p.connections[ep.ID] = &zero
		}
	}
	p.connMu.Unlock()

	// Pick two random endpoints
	p.rngLock.Lock()
	idx1 := p.rng.Intn(n)
	idx2 := p.rng.Intn(n)
	// Ensure we pick different endpoints if possible
	for idx2 == idx1 && n > 1 {
		idx2 = p.rng.Intn(n)
	}
	p.rngLock.Unlock()

	ep1 := endpoints[idx1]
	ep2 := endpoints[idx2]

	// Pick the one with fewer connections
	p.connMu.RLock()
	defer p.connMu.RUnlock()

	conn1 := atomic.LoadInt64(p.connections[ep1.ID])
	conn2 := atomic.LoadInt64(p.connections[ep2.ID])

	if conn1 <= conn2 {
		atomic.AddInt64(p.connections[ep1.ID], 1)
		return ep1
	}
	atomic.AddInt64(p.connections[ep2.ID], 1)
	return ep2
}

// ReleaseConnection decrements the connection count.
func (p *P2CBalancer) ReleaseConnection(endpoint *Endpoint) {
	p.connMu.RLock()
	defer p.connMu.RUnlock()

	if counter, ok := p.connections[endpoint.ID]; ok {
		atomic.AddInt64(counter, -1)
	}
}

// Update updates endpoints.
func (p *P2CBalancer) Update(endpoints []*Endpoint) {
	p.BaseBalancer.Update(endpoints)

	p.connMu.Lock()
	defer p.connMu.Unlock()

	valid := make(map[string]bool)
	for _, ep := range endpoints {
		valid[ep.ID] = true
	}

	for id := range p.connections {
		if !valid[id] {
			delete(p.connections, id)
		}
	}
}

// ZoneAwareBalancer implements zone-aware load balancing.
// It prefers endpoints in the same zone as the caller.
type ZoneAwareBalancer struct {
	*BaseBalancer
	localZone string
	fallback  LoadBalancer
}

// NewZoneAwareBalancer creates a new zone-aware balancer.
func NewZoneAwareBalancer(localZone string, fallback LoadBalancer) *ZoneAwareBalancer {
	if fallback == nil {
		fallback = NewRoundRobinBalancer()
	}

	return &ZoneAwareBalancer{
		BaseBalancer: newBaseBalancer("zone-aware"),
		localZone:    localZone,
		fallback:     fallback,
	}
}

// Select selects an endpoint preferring local zone.
func (z *ZoneAwareBalancer) Select(endpoints []*Endpoint) *Endpoint {
	return z.SelectWithContext(endpoints, nil)
}

// SelectWithContext selects an endpoint with context.
func (z *ZoneAwareBalancer) SelectWithContext(endpoints []*Endpoint, ctx *SelectContext) *Endpoint {
	n := len(endpoints)
	if n == 0 {
		return nil
	}

	// Filter endpoints by local zone
	localEndpoints := make([]*Endpoint, 0, n)
	for _, ep := range endpoints {
		if ep.Zone == z.localZone {
			localEndpoints = append(localEndpoints, ep)
		}
	}

	// Use local endpoints if available
	if len(localEndpoints) > 0 {
		return z.fallback.SelectWithContext(localEndpoints, ctx)
	}

	// Fall back to all endpoints
	return z.fallback.SelectWithContext(endpoints, ctx)
}

// Update updates the balancer and fallback.
func (z *ZoneAwareBalancer) Update(endpoints []*Endpoint) {
	z.BaseBalancer.Update(endpoints)
	z.fallback.Update(endpoints)
}

// RecordResponse records response for both balancers.
func (z *ZoneAwareBalancer) RecordResponse(endpoint *Endpoint, latency time.Duration, success bool) {
	z.BaseBalancer.RecordResponse(endpoint, latency, success)
	z.fallback.RecordResponse(endpoint, latency, success)
}

// BalancerPool manages load balancers for multiple services.
type BalancerPool struct {
	mu        sync.RWMutex
	balancers map[string]LoadBalancer
	factory   func(string) LoadBalancer
}

// NewBalancerPool creates a new balancer pool.
func NewBalancerPool(factory func(string) LoadBalancer) *BalancerPool {
	if factory == nil {
		factory = func(service string) LoadBalancer {
			return NewRoundRobinBalancer()
		}
	}

	return &BalancerPool{
		balancers: make(map[string]LoadBalancer),
		factory:   factory,
	}
}

// Get returns the balancer for a service, creating one if needed.
func (p *BalancerPool) Get(service string) LoadBalancer {
	p.mu.RLock()
	lb, ok := p.balancers[service]
	p.mu.RUnlock()

	if ok {
		return lb
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check
	if lb, ok := p.balancers[service]; ok {
		return lb
	}

	lb = p.factory(service)
	p.balancers[service] = lb
	return lb
}

// Set sets the balancer for a service.
func (p *BalancerPool) Set(service string, lb LoadBalancer) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.balancers[service] = lb
}

// Remove removes the balancer for a service.
func (p *BalancerPool) Remove(service string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.balancers, service)
}

// List returns all services with balancers.
func (p *BalancerPool) List() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	services := make([]string, 0, len(p.balancers))
	for svc := range p.balancers {
		services = append(services, svc)
	}
	return services
}

// SubsetBalancer implements subset-based load balancing for large clusters.
// It limits the number of endpoints each client connects to.
type SubsetBalancer struct {
	*BaseBalancer
	subsetSize int
	subsets    map[string][]*Endpoint
	subsetsMu  sync.RWMutex
	fallback   LoadBalancer
	rng        *rand.Rand
	rngLock    sync.Mutex
}

// NewSubsetBalancer creates a new subset balancer.
func NewSubsetBalancer(subsetSize int) *SubsetBalancer {
	if subsetSize <= 0 {
		subsetSize = 10
	}

	return &SubsetBalancer{
		BaseBalancer: newBaseBalancer("subset"),
		subsetSize:   subsetSize,
		subsets:      make(map[string][]*Endpoint),
		fallback:     NewRoundRobinBalancer(),
		rng:          rand.New(rand.NewSource(time.Now().UnixNano())), // #nosec G404 -- backend selection, not security-bearing
	}
}

// Select selects from the service's subset.
func (s *SubsetBalancer) Select(endpoints []*Endpoint) *Endpoint {
	return s.SelectWithContext(endpoints, nil)
}

// SelectWithContext selects from subset based on service name.
func (s *SubsetBalancer) SelectWithContext(endpoints []*Endpoint, ctx *SelectContext) *Endpoint {
	n := len(endpoints)
	if n == 0 {
		return nil
	}

	service := "default"
	if ctx != nil && ctx.ServiceName != "" {
		service = ctx.ServiceName
	}

	// Get or create subset
	subset := s.getOrCreateSubset(service, endpoints)

	return s.fallback.SelectWithContext(subset, ctx)
}

// getOrCreateSubset returns the subset for a service.
func (s *SubsetBalancer) getOrCreateSubset(service string, endpoints []*Endpoint) []*Endpoint {
	s.subsetsMu.RLock()
	subset, ok := s.subsets[service]
	s.subsetsMu.RUnlock()

	if ok && len(subset) > 0 {
		// Validate subset still contains valid endpoints
		valid := make([]*Endpoint, 0, len(subset))
		endpointSet := make(map[string]bool)
		for _, ep := range endpoints {
			endpointSet[ep.ID] = true
		}
		for _, ep := range subset {
			if endpointSet[ep.ID] {
				valid = append(valid, ep)
			}
		}
		if len(valid) > 0 {
			return valid
		}
	}

	s.subsetsMu.Lock()
	defer s.subsetsMu.Unlock()

	// Build new subset
	n := len(endpoints)
	subsetSize := s.subsetSize
	if subsetSize > n {
		subsetSize = n
	}

	// Shuffle and take first subsetSize elements
	perm := make([]int, n)
	for i := 0; i < n; i++ {
		perm[i] = i
	}

	s.rngLock.Lock()
	s.rng.Shuffle(n, func(i, j int) {
		perm[i], perm[j] = perm[j], perm[i]
	})
	s.rngLock.Unlock()

	subset = make([]*Endpoint, subsetSize)
	for i := 0; i < subsetSize; i++ {
		subset[i] = endpoints[perm[i]]
	}

	s.subsets[service] = subset
	return subset
}

// Update clears subsets to force recomputation.
func (s *SubsetBalancer) Update(endpoints []*Endpoint) {
	s.BaseBalancer.Update(endpoints)

	s.subsetsMu.Lock()
	// Clear all subsets to force recomputation
	s.subsets = make(map[string][]*Endpoint)
	s.subsetsMu.Unlock()
}
