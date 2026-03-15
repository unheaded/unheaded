// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package loadbalancer provides load balancing algorithms for the service mesh.
// THE HAUBERK - Least connections load balancing implementation.
package loadbalancer

import (
	"sync"
	"sync/atomic"
)

// endpointConn tracks connection count for an endpoint.
type endpointConn struct {
	endpoint *Endpoint
	conns    int64
}

// LeastConn implements least-connections load balancing.
type LeastConn struct {
	mu        sync.RWMutex
	endpoints []*endpointConn
}

// NewLeastConn creates a new least-connections balancer.
func NewLeastConn(endpoints []*Endpoint) *LeastConn {
	healthy := filterHealthy(endpoints)
	epConns := make([]*endpointConn, len(healthy))
	for i, ep := range healthy {
		epConns[i] = &endpointConn{
			endpoint: ep,
			conns:    0,
		}
	}

	return &LeastConn{
		endpoints: epConns,
	}
}

// Next returns the endpoint with the fewest connections.
func (lc *LeastConn) Next() (*Endpoint, error) {
	lc.mu.RLock()
	defer lc.mu.RUnlock()

	if len(lc.endpoints) == 0 {
		return nil, ErrNoEndpoints
	}

	// Find endpoint with least connections
	var minIdx int
	minConns := atomic.LoadInt64(&lc.endpoints[0].conns)

	for i := 1; i < len(lc.endpoints); i++ {
		conns := atomic.LoadInt64(&lc.endpoints[i].conns)
		if conns < minConns {
			minConns = conns
			minIdx = i
		}
	}

	// Increment connection count
	atomic.AddInt64(&lc.endpoints[minIdx].conns, 1)

	return lc.endpoints[minIdx].endpoint, nil
}

// Update updates the list of endpoints.
func (lc *LeastConn) Update(endpoints []*Endpoint) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	healthy := filterHealthy(endpoints)

	// Create map of existing connection counts
	connMap := make(map[string]int64)
	for _, ec := range lc.endpoints {
		connMap[ec.endpoint.ID] = atomic.LoadInt64(&ec.conns)
	}

	// Update endpoints, preserving connection counts
	epConns := make([]*endpointConn, len(healthy))
	for i, ep := range healthy {
		conns := connMap[ep.ID] // 0 if not found
		epConns[i] = &endpointConn{
			endpoint: ep,
			conns:    conns,
		}
	}

	lc.endpoints = epConns
}

// Algorithm returns the algorithm type.
func (lc *LeastConn) Algorithm() Algorithm {
	return LeastConnAlgorithm
}

// Release decrements the connection count for an endpoint.
func (lc *LeastConn) Release(id string) {
	lc.mu.RLock()
	defer lc.mu.RUnlock()

	for _, ec := range lc.endpoints {
		if ec.endpoint.ID == id {
			atomic.AddInt64(&ec.conns, -1)
			return
		}
	}
}

// Connections returns the connection count for an endpoint.
func (lc *LeastConn) Connections(id string) int64 {
	lc.mu.RLock()
	defer lc.mu.RUnlock()

	for _, ec := range lc.endpoints {
		if ec.endpoint.ID == id {
			return atomic.LoadInt64(&ec.conns)
		}
	}
	return 0
}

// TotalConnections returns the total connections across all endpoints.
func (lc *LeastConn) TotalConnections() int64 {
	lc.mu.RLock()
	defer lc.mu.RUnlock()

	var total int64
	for _, ec := range lc.endpoints {
		total += atomic.LoadInt64(&ec.conns)
	}
	return total
}

// Count returns the number of healthy endpoints.
func (lc *LeastConn) Count() int {
	lc.mu.RLock()
	defer lc.mu.RUnlock()
	return len(lc.endpoints)
}
