// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package loadbalance provides load balancing strategies for THE HAUBERK.
package loadbalance

import (
	"sync"
)

// Endpoint represents a backend service endpoint.
type Endpoint struct {
	Address string
	Port    int
	Weight  int
	Healthy bool
	Meta    map[string]string
}

// Balancer is the interface for load balancing strategies.
type Balancer interface {
	// Pick selects an endpoint from the pool.
	Pick(endpoints []*Endpoint) *Endpoint
	// Name returns the balancer name.
	Name() string
	// Update notifies the balancer of endpoint changes.
	Update(endpoints []*Endpoint)
}

// EndpointPool manages a pool of endpoints with health status.
type EndpointPool struct {
	mu        sync.RWMutex
	endpoints []*Endpoint
	balancer  Balancer
}

// NewEndpointPool creates a new endpoint pool with the specified balancer.
func NewEndpointPool(balancer Balancer) *EndpointPool {
	return &EndpointPool{
		endpoints: make([]*Endpoint, 0),
		balancer:  balancer,
	}
}

// SetEndpoints replaces all endpoints in the pool.
func (p *EndpointPool) SetEndpoints(endpoints []*Endpoint) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.endpoints = endpoints
	p.balancer.Update(endpoints)
}

// Pick selects a healthy endpoint using the configured balancer.
func (p *EndpointPool) Pick() *Endpoint {
	p.mu.RLock()
	defer p.mu.RUnlock()

	healthy := make([]*Endpoint, 0, len(p.endpoints))
	for _, ep := range p.endpoints {
		if ep.Healthy {
			healthy = append(healthy, ep)
		}
	}

	if len(healthy) == 0 {
		return nil
	}

	return p.balancer.Pick(healthy)
}

// MarkHealthy sets the health status of an endpoint.
func (p *EndpointPool) MarkHealthy(address string, port int, healthy bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, ep := range p.endpoints {
		if ep.Address == address && ep.Port == port {
			ep.Healthy = healthy
			break
		}
	}
}

// GetEndpoints returns a copy of all endpoints.
func (p *EndpointPool) GetEndpoints() []*Endpoint {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make([]*Endpoint, len(p.endpoints))
	copy(result, p.endpoints)
	return result
}

// HealthyCount returns the number of healthy endpoints.
func (p *EndpointPool) HealthyCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	count := 0
	for _, ep := range p.endpoints {
		if ep.Healthy {
			count++
		}
	}
	return count
}
