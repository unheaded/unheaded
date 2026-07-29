// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package loadbalance

import (
	"math/rand"
	"sync"
	"time"
)

// Weighted implements weighted random load balancing.
type Weighted struct {
	mu     sync.RWMutex
	rng    *rand.Rand
	total  int
	ranges []weightRange
}

type weightRange struct {
	endpoint *Endpoint
	start    int
	end      int
}

// NewWeighted creates a new weighted random balancer.
func NewWeighted() *Weighted {
	return &Weighted{
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())), // #nosec G404 -- weighted backend selection, not security-bearing
		ranges: make([]weightRange, 0),
	}
}

// Pick selects an endpoint based on weight probabilities.
func (w *Weighted) Pick(endpoints []*Endpoint) *Endpoint {
	if len(endpoints) == 0 {
		return nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Rebuild ranges for the current healthy endpoints
	w.buildRanges(endpoints)

	if w.total == 0 {
		return nil
	}

	// Generate random number in [0, total)
	r := w.rng.Intn(w.total)

	// Binary search for the endpoint
	for _, wr := range w.ranges {
		if r >= wr.start && r < wr.end {
			return wr.endpoint
		}
	}

	// Fallback to first endpoint
	return endpoints[0]
}

// Name returns the balancer name.
func (w *Weighted) Name() string {
	return "weighted"
}

// Update is called when endpoints change.
func (w *Weighted) Update(endpoints []*Endpoint) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buildRanges(endpoints)
}

func (w *Weighted) buildRanges(endpoints []*Endpoint) {
	w.ranges = make([]weightRange, 0, len(endpoints))
	w.total = 0

	for _, ep := range endpoints {
		weight := ep.Weight
		if weight <= 0 {
			weight = 1 // Default weight
		}

		w.ranges = append(w.ranges, weightRange{
			endpoint: ep,
			start:    w.total,
			end:      w.total + weight,
		})
		w.total += weight
	}
}

// SmoothWeighted implements smooth weighted round-robin for more even distribution.
type SmoothWeighted struct {
	mu      sync.Mutex
	weights map[string]*smoothWeight
}

type smoothWeight struct {
	endpoint        *Endpoint
	weight          int
	currentWeight   int
	effectiveWeight int
}

// NewSmoothWeighted creates a new smooth weighted balancer.
func NewSmoothWeighted() *SmoothWeighted {
	return &SmoothWeighted{
		weights: make(map[string]*smoothWeight),
	}
}

// Pick selects an endpoint using smooth weighted round-robin.
func (sw *SmoothWeighted) Pick(endpoints []*Endpoint) *Endpoint {
	if len(endpoints) == 0 {
		return nil
	}

	sw.mu.Lock()
	defer sw.mu.Unlock()

	// Update weights for current endpoints
	sw.updateWeights(endpoints)

	var total int
	var best *smoothWeight

	for _, ep := range endpoints {
		key := endpointKey(ep)
		w := sw.weights[key]
		if w == nil {
			continue
		}

		w.currentWeight += w.effectiveWeight
		total += w.effectiveWeight

		if best == nil || w.currentWeight > best.currentWeight {
			best = w
		}
	}

	if best == nil {
		return endpoints[0]
	}

	best.currentWeight -= total
	return best.endpoint
}

// Name returns the balancer name.
func (sw *SmoothWeighted) Name() string {
	return "smooth-weighted"
}

// Update is called when endpoints change.
func (sw *SmoothWeighted) Update(endpoints []*Endpoint) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.updateWeights(endpoints)
}

func (sw *SmoothWeighted) updateWeights(endpoints []*Endpoint) {
	current := make(map[string]bool)

	for _, ep := range endpoints {
		key := endpointKey(ep)
		current[key] = true

		weight := ep.Weight
		if weight <= 0 {
			weight = 1
		}

		if existing, ok := sw.weights[key]; ok {
			// Update existing
			existing.endpoint = ep
			existing.weight = weight
			existing.effectiveWeight = weight
		} else {
			// Add new
			sw.weights[key] = &smoothWeight{
				endpoint:        ep,
				weight:          weight,
				currentWeight:   0,
				effectiveWeight: weight,
			}
		}
	}

	// Remove stale entries
	for key := range sw.weights {
		if !current[key] {
			delete(sw.weights, key)
		}
	}
}
