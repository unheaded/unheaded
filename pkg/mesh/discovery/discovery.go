// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package discovery provides service discovery for THE HAUBERK.
package discovery

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	// ErrServiceNotFound indicates the service was not found.
	ErrServiceNotFound = errors.New("service not found")
	// ErrNoEndpoints indicates no endpoints are available.
	ErrNoEndpoints = errors.New("no endpoints available")
	// ErrDiscoveryUnavailable indicates discovery is unavailable.
	ErrDiscoveryUnavailable = errors.New("discovery service unavailable")
)

// Endpoint represents a service endpoint.
type Endpoint struct {
	Address   string
	Port      int
	Weight    int
	Healthy   bool
	Zone      string
	Tags      map[string]string
	LastSeen  time.Time
	ServiceID string
}

// Service represents a discovered service.
type Service struct {
	Name      string
	Namespace string
	Endpoints []*Endpoint
	Protocol  string
	Version   string
	Meta      map[string]string
	UpdatedAt time.Time
}

// Discoverer is the interface for service discovery backends.
type Discoverer interface {
	// Discover returns endpoints for a service.
	Discover(ctx context.Context, service, namespace string) (*Service, error)
	// Watch returns a channel that receives updates for a service.
	Watch(ctx context.Context, service, namespace string) (<-chan *Service, error)
	// Close shuts down the discoverer.
	Close() error
}

// CachingDiscoverer wraps a Discoverer with caching.
type CachingDiscoverer struct {
	backend  Discoverer
	mu       sync.RWMutex
	cache    map[string]*cacheEntry
	ttl      time.Duration
	negative time.Duration // TTL for negative (not found) cache
}

type cacheEntry struct {
	service   *Service
	err       error
	expiresAt time.Time
}

// NewCachingDiscoverer creates a new caching discoverer.
func NewCachingDiscoverer(backend Discoverer, ttl time.Duration) *CachingDiscoverer {
	return &CachingDiscoverer{
		backend:  backend,
		cache:    make(map[string]*cacheEntry),
		ttl:      ttl,
		negative: ttl / 4, // Shorter negative cache
	}
}

// Discover returns endpoints with caching.
func (c *CachingDiscoverer) Discover(ctx context.Context, service, namespace string) (*Service, error) {
	key := namespace + "/" + service

	// Check cache
	c.mu.RLock()
	entry, ok := c.cache[key]
	c.mu.RUnlock()

	if ok && time.Now().Before(entry.expiresAt) {
		if entry.err != nil {
			return nil, entry.err
		}
		return entry.service, nil
	}

	// Cache miss or expired, fetch from backend
	svc, err := c.backend.Discover(ctx, service, namespace)

	// Update cache
	c.mu.Lock()
	if err != nil {
		c.cache[key] = &cacheEntry{
			err:       err,
			expiresAt: time.Now().Add(c.negative),
		}
	} else {
		c.cache[key] = &cacheEntry{
			service:   svc,
			expiresAt: time.Now().Add(c.ttl),
		}
	}
	c.mu.Unlock()

	return svc, err
}

// Watch delegates to the backend.
func (c *CachingDiscoverer) Watch(ctx context.Context, service, namespace string) (<-chan *Service, error) {
	return c.backend.Watch(ctx, service, namespace)
}

// Close shuts down the discoverer.
func (c *CachingDiscoverer) Close() error {
	return c.backend.Close()
}

// Invalidate removes a service from the cache.
func (c *CachingDiscoverer) Invalidate(service, namespace string) {
	key := namespace + "/" + service
	c.mu.Lock()
	delete(c.cache, key)
	c.mu.Unlock()
}

// CompositeDiscoverer tries multiple discoverers in order.
type CompositeDiscoverer struct {
	discoverers []Discoverer
}

// NewCompositeDiscoverer creates a new composite discoverer.
func NewCompositeDiscoverer(discoverers ...Discoverer) *CompositeDiscoverer {
	return &CompositeDiscoverer{
		discoverers: discoverers,
	}
}

// Discover tries each discoverer until one succeeds.
func (c *CompositeDiscoverer) Discover(ctx context.Context, service, namespace string) (*Service, error) {
	var lastErr error

	for _, d := range c.discoverers {
		svc, err := d.Discover(ctx, service, namespace)
		if err == nil {
			return svc, nil
		}
		lastErr = err
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, ErrServiceNotFound
}

// Watch uses the first available discoverer.
func (c *CompositeDiscoverer) Watch(ctx context.Context, service, namespace string) (<-chan *Service, error) {
	for _, d := range c.discoverers {
		ch, err := d.Watch(ctx, service, namespace)
		if err == nil {
			return ch, nil
		}
	}
	return nil, ErrDiscoveryUnavailable
}

// Close shuts down all discoverers.
func (c *CompositeDiscoverer) Close() error {
	var errs []error
	for _, d := range c.discoverers {
		if err := d.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// StaticDiscoverer provides static endpoint configuration.
type StaticDiscoverer struct {
	mu       sync.RWMutex
	services map[string]*Service
}

// NewStaticDiscoverer creates a new static discoverer.
func NewStaticDiscoverer() *StaticDiscoverer {
	return &StaticDiscoverer{
		services: make(map[string]*Service),
	}
}

// AddService adds a service to the static configuration.
func (s *StaticDiscoverer) AddService(svc *Service) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := svc.Namespace + "/" + svc.Name
	s.services[key] = svc
}

// RemoveService removes a service.
func (s *StaticDiscoverer) RemoveService(name, namespace string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := namespace + "/" + name
	delete(s.services, key)
}

// Discover returns the static service configuration.
func (s *StaticDiscoverer) Discover(ctx context.Context, service, namespace string) (*Service, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := namespace + "/" + service
	if svc, ok := s.services[key]; ok {
		return svc, nil
	}
	return nil, ErrServiceNotFound
}

// Watch returns a channel (static doesn't support real-time updates).
func (s *StaticDiscoverer) Watch(ctx context.Context, service, namespace string) (<-chan *Service, error) {
	ch := make(chan *Service, 1)

	// Send current state
	svc, err := s.Discover(ctx, service, namespace)
	if err != nil {
		close(ch)
		return ch, err
	}
	ch <- svc

	// Keep channel open but no more updates
	go func() {
		<-ctx.Done()
		close(ch)
	}()

	return ch, nil
}

// Close is a no-op for static discoverer.
func (s *StaticDiscoverer) Close() error {
	return nil
}

// ServiceRegistry maintains local service state.
type ServiceRegistry struct {
	mu       sync.RWMutex
	services map[string]*Service
	watchers map[string][]chan<- *Service
}

// NewServiceRegistry creates a new service registry.
func NewServiceRegistry() *ServiceRegistry {
	return &ServiceRegistry{
		services: make(map[string]*Service),
		watchers: make(map[string][]chan<- *Service),
	}
}

// Register registers or updates a service.
func (r *ServiceRegistry) Register(svc *Service) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := svc.Namespace + "/" + svc.Name
	svc.UpdatedAt = time.Now()
	r.services[key] = svc

	// Notify watchers
	if watchers, ok := r.watchers[key]; ok {
		for _, ch := range watchers {
			select {
			case ch <- svc:
			default:
				// Channel full, skip
			}
		}
	}
}

// Deregister removes a service.
func (r *ServiceRegistry) Deregister(name, namespace string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := namespace + "/" + name
	delete(r.services, key)
}

// Get retrieves a service.
func (r *ServiceRegistry) Get(name, namespace string) (*Service, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := namespace + "/" + name
	svc, ok := r.services[key]
	return svc, ok
}

// List returns all services.
func (r *ServiceRegistry) List() []*Service {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Service, 0, len(r.services))
	for _, svc := range r.services {
		result = append(result, svc)
	}
	return result
}

// Watch registers a watcher for service updates.
func (r *ServiceRegistry) Watch(name, namespace string) <-chan *Service {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := namespace + "/" + name
	ch := make(chan *Service, 10)
	r.watchers[key] = append(r.watchers[key], ch)

	// Send current state if exists
	if svc, ok := r.services[key]; ok {
		ch <- svc
	}

	return ch
}

// Unwatch removes a watcher by closing the channel.
func (r *ServiceRegistry) Unwatch(name, namespace string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := namespace + "/" + name
	if watchers, ok := r.watchers[key]; ok {
		for _, w := range watchers {
			close(w)
		}
		delete(r.watchers, key)
	}
}
