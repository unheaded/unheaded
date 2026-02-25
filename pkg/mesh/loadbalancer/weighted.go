// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package loadbalancer provides load balancing algorithms for the service mesh.
// THE HAUBERK - Weighted random load balancing implementation.
package loadbalancer

import (
	"math/rand"
	"sync"
	"time"
)

// Weighted implements weighted random load balancing.
type Weighted struct {
	mu          sync.RWMutex
	endpoints   []*Endpoint
	totalWeight int
	rng         *rand.Rand
}

// NewWeighted creates a new weighted random balancer.
func NewWeighted(endpoints []*Endpoint) *Weighted {
	w := &Weighted{
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	w.Update(endpoints)
	return w
}

// Next returns an endpoint based on weighted random selection.
func (w *Weighted) Next() (*Endpoint, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if len(w.endpoints) == 0 {
		return nil, ErrNoEndpoints
	}

	if w.totalWeight <= 0 {
		// Fallback to uniform random if no weights
		return w.endpoints[w.rng.Intn(len(w.endpoints))], nil
	}

	// Weighted random selection
	target := w.rng.Intn(w.totalWeight)
	cumulative := 0

	for _, ep := range w.endpoints {
		weight := ep.Weight
		if weight <= 0 {
			weight = 1
		}
		cumulative += weight
		if target < cumulative {
			return ep, nil
		}
	}

	// Should not reach here, but return last endpoint as fallback
	return w.endpoints[len(w.endpoints)-1], nil
}

// Update updates the list of endpoints.
func (w *Weighted) Update(endpoints []*Endpoint) {
	w.mu.Lock()
	defer w.mu.Unlock()

	healthy := filterHealthy(endpoints)
	w.endpoints = healthy

	// Calculate total weight
	w.totalWeight = 0
	for _, ep := range healthy {
		weight := ep.Weight
		if weight <= 0 {
			weight = 1
		}
		w.totalWeight += weight
	}
}

// Algorithm returns the algorithm type.
func (w *Weighted) Algorithm() Algorithm {
	return WeightedAlgorithm
}

// Count returns the number of healthy endpoints.
func (w *Weighted) Count() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.endpoints)
}

// TotalWeight returns the total weight of all endpoints.
func (w *Weighted) TotalWeight() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.totalWeight
}

// Random implements simple random load balancing.
type Random struct {
	mu        sync.RWMutex
	endpoints []*Endpoint
	rng       *rand.Rand
}

// NewRandom creates a new random balancer.
func NewRandom(endpoints []*Endpoint) *Random {
	return &Random{
		endpoints: filterHealthy(endpoints),
		rng:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Next returns a random endpoint.
func (r *Random) Next() (*Endpoint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.endpoints) == 0 {
		return nil, ErrNoEndpoints
	}

	return r.endpoints[r.rng.Intn(len(r.endpoints))], nil
}

// Update updates the list of endpoints.
func (r *Random) Update(endpoints []*Endpoint) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.endpoints = filterHealthy(endpoints)
}

// Algorithm returns the algorithm type.
func (r *Random) Algorithm() Algorithm {
	return RandomAlgorithm
}

// Count returns the number of healthy endpoints.
func (r *Random) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.endpoints)
}
