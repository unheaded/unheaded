// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package loadbalance

import (
	"sync"
)

// LeastConn implements least-connections load balancing.
type LeastConn struct {
	mu          sync.RWMutex
	connections map[string]int64 // address:port -> connection count
}

// NewLeastConn creates a new least-connections balancer.
func NewLeastConn() *LeastConn {
	return &LeastConn{
		connections: make(map[string]int64),
	}
}

// Pick selects the endpoint with the fewest active connections.
func (l *LeastConn) Pick(endpoints []*Endpoint) *Endpoint {
	if len(endpoints) == 0 {
		return nil
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	var selected *Endpoint
	minConns := int64(-1)

	for _, ep := range endpoints {
		key := endpointKey(ep)
		conns := l.connections[key]

		if minConns < 0 || conns < minConns {
			minConns = conns
			selected = ep
		}
	}

	return selected
}

// Name returns the balancer name.
func (l *LeastConn) Name() string {
	return "least-conn"
}

// Update is called when endpoints change.
func (l *LeastConn) Update(endpoints []*Endpoint) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Build set of current endpoints
	current := make(map[string]bool)
	for _, ep := range endpoints {
		key := endpointKey(ep)
		current[key] = true
		// Initialize new endpoints with 0 connections
		if _, exists := l.connections[key]; !exists {
			l.connections[key] = 0
		}
	}

	// Remove stale endpoints
	for key := range l.connections {
		if !current[key] {
			delete(l.connections, key)
		}
	}
}

// IncrementConnections increases the connection count for an endpoint.
func (l *LeastConn) IncrementConnections(ep *Endpoint) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.connections[endpointKey(ep)]++
}

// DecrementConnections decreases the connection count for an endpoint.
func (l *LeastConn) DecrementConnections(ep *Endpoint) {
	l.mu.Lock()
	defer l.mu.Unlock()
	key := endpointKey(ep)
	if l.connections[key] > 0 {
		l.connections[key]--
	}
}

// GetConnections returns the current connection count for an endpoint.
func (l *LeastConn) GetConnections(ep *Endpoint) int64 {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.connections[endpointKey(ep)]
}

func endpointKey(ep *Endpoint) string {
	return ep.Address + ":" + itoa(ep.Port)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}
