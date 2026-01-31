// Package loadbalancer provides load balancing algorithms for the service mesh.
// THE HAUBERK - Round-robin load balancing implementation.
package loadbalancer

import (
	"sync"
	"sync/atomic"
)

// RoundRobin implements round-robin load balancing.
type RoundRobin struct {
	mu        sync.RWMutex
	endpoints []*Endpoint
	index     uint64
}

// NewRoundRobin creates a new round-robin balancer.
func NewRoundRobin(endpoints []*Endpoint) *RoundRobin {
	return &RoundRobin{
		endpoints: filterHealthy(endpoints),
		index:     0,
	}
}

// Next returns the next endpoint in round-robin order.
func (rr *RoundRobin) Next() (*Endpoint, error) {
	rr.mu.RLock()
	defer rr.mu.RUnlock()

	if len(rr.endpoints) == 0 {
		return nil, ErrNoEndpoints
	}

	idx := atomic.AddUint64(&rr.index, 1) - 1
	return rr.endpoints[idx%uint64(len(rr.endpoints))], nil
}

// Update updates the list of endpoints.
func (rr *RoundRobin) Update(endpoints []*Endpoint) {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	rr.endpoints = filterHealthy(endpoints)
}

// Algorithm returns the algorithm type.
func (rr *RoundRobin) Algorithm() Algorithm {
	return RoundRobinAlgorithm
}

// Count returns the number of healthy endpoints.
func (rr *RoundRobin) Count() int {
	rr.mu.RLock()
	defer rr.mu.RUnlock()
	return len(rr.endpoints)
}
