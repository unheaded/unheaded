// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package loadbalancer provides load balancing algorithms for the service mesh.
// THE HAUBERK - Load balancer interface and management for the Unheaded Kingdom.
package loadbalancer

import (
	"errors"
	"sync"
)

var (
	// ErrNoEndpoints is returned when no endpoints are available.
	ErrNoEndpoints = errors.New("no endpoints available")
	// ErrUnknownAlgorithm is returned when an unknown algorithm is requested.
	ErrUnknownAlgorithm = errors.New("unknown load balancing algorithm")
)

// Algorithm represents a load balancing algorithm type.
type Algorithm string

const (
	// RoundRobinAlgorithm distributes requests evenly in order.
	RoundRobinAlgorithm Algorithm = "round-robin"
	// LeastConnAlgorithm sends to the endpoint with fewest connections.
	LeastConnAlgorithm Algorithm = "least-conn"
	// WeightedAlgorithm distributes based on endpoint weights.
	WeightedAlgorithm Algorithm = "weighted"
	// RandomAlgorithm distributes randomly.
	RandomAlgorithm Algorithm = "random"
)

// Endpoint represents a service endpoint.
type Endpoint struct {
	// ID is the unique identifier for this endpoint.
	ID string
	// Address is the network address (host:port).
	Address string
	// Weight is used for weighted load balancing.
	Weight int
	// Healthy indicates if the endpoint is healthy.
	Healthy bool
	// Metadata holds additional endpoint information.
	Metadata map[string]string
}

// Balancer is the interface for load balancers.
type Balancer interface {
	// Next returns the next endpoint to use.
	Next() (*Endpoint, error)
	// Update updates the list of endpoints.
	Update(endpoints []*Endpoint)
	// Algorithm returns the algorithm type.
	Algorithm() Algorithm
}

// LoadBalancer manages load balancing for services.
type LoadBalancer struct {
	mu        sync.RWMutex
	balancers map[string]Balancer
	algorithm Algorithm
}

// New creates a new load balancer manager.
func New(defaultAlgorithm Algorithm) *LoadBalancer {
	if defaultAlgorithm == "" {
		defaultAlgorithm = RoundRobinAlgorithm
	}

	return &LoadBalancer{
		balancers: make(map[string]Balancer),
		algorithm: defaultAlgorithm,
	}
}

// Register registers endpoints for a service.
func (lb *LoadBalancer) Register(service string, endpoints []*Endpoint) error {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	balancer, ok := lb.balancers[service]
	if !ok {
		var err error
		balancer, err = lb.createBalancer(lb.algorithm, endpoints)
		if err != nil {
			return err
		}
		lb.balancers[service] = balancer
	} else {
		balancer.Update(endpoints)
	}

	return nil
}

// RegisterWithAlgorithm registers endpoints with a specific algorithm.
func (lb *LoadBalancer) RegisterWithAlgorithm(service string, algorithm Algorithm, endpoints []*Endpoint) error {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	balancer, err := lb.createBalancer(algorithm, endpoints)
	if err != nil {
		return err
	}

	lb.balancers[service] = balancer
	return nil
}

// createBalancer creates a new balancer for the given algorithm.
func (lb *LoadBalancer) createBalancer(algorithm Algorithm, endpoints []*Endpoint) (Balancer, error) {
	switch algorithm {
	case RoundRobinAlgorithm:
		return NewRoundRobin(endpoints), nil
	case LeastConnAlgorithm:
		return NewLeastConn(endpoints), nil
	case WeightedAlgorithm:
		return NewWeighted(endpoints), nil
	case RandomAlgorithm:
		return NewRandom(endpoints), nil
	default:
		return nil, ErrUnknownAlgorithm
	}
}

// Next returns the next endpoint for a service.
func (lb *LoadBalancer) Next(service string) (*Endpoint, error) {
	lb.mu.RLock()
	balancer, ok := lb.balancers[service]
	lb.mu.RUnlock()

	if !ok {
		return nil, ErrNoEndpoints
	}

	return balancer.Next()
}

// Update updates the endpoints for a service.
func (lb *LoadBalancer) Update(service string, endpoints []*Endpoint) error {
	lb.mu.RLock()
	balancer, ok := lb.balancers[service]
	lb.mu.RUnlock()

	if !ok {
		return lb.Register(service, endpoints)
	}

	balancer.Update(endpoints)
	return nil
}

// Remove removes a service from the load balancer.
func (lb *LoadBalancer) Remove(service string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	delete(lb.balancers, service)
}

// Services returns all registered services.
func (lb *LoadBalancer) Services() []string {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	services := make([]string, 0, len(lb.balancers))
	for svc := range lb.balancers {
		services = append(services, svc)
	}
	return services
}

// GetAlgorithm returns the algorithm for a service.
func (lb *LoadBalancer) GetAlgorithm(service string) (Algorithm, error) {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	balancer, ok := lb.balancers[service]
	if !ok {
		return "", ErrNoEndpoints
	}

	return balancer.Algorithm(), nil
}

// filterHealthy filters endpoints to only healthy ones.
func filterHealthy(endpoints []*Endpoint) []*Endpoint {
	healthy := make([]*Endpoint, 0, len(endpoints))
	for _, ep := range endpoints {
		if ep.Healthy {
			healthy = append(healthy, ep)
		}
	}
	return healthy
}
