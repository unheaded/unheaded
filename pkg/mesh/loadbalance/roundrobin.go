// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package loadbalance

import (
	"sync/atomic"
)

// RoundRobin implements round-robin load balancing.
type RoundRobin struct {
	counter uint64
}

// NewRoundRobin creates a new round-robin balancer.
func NewRoundRobin() *RoundRobin {
	return &RoundRobin{}
}

// Pick selects the next endpoint in round-robin order.
func (r *RoundRobin) Pick(endpoints []*Endpoint) *Endpoint {
	if len(endpoints) == 0 {
		return nil
	}

	idx := atomic.AddUint64(&r.counter, 1) - 1
	return endpoints[idx%uint64(len(endpoints))]
}

// Name returns the balancer name.
func (r *RoundRobin) Name() string {
	return "round-robin"
}

// Update is called when endpoints change. Round-robin doesn't need special handling.
func (r *RoundRobin) Update(endpoints []*Endpoint) {
	// No special action needed for round-robin
}
