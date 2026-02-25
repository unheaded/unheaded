// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package discovery provides Wotan-based service discovery for the Unheaded
// platform. Services announce themselves via the "system.discovery" Wotan topic
// and are automatically discovered by all other participants.
//
// Announcements are re-published every 15 seconds. A service that has not
// re-announced within 30 seconds (TTL) is considered stale and removed from the
// registry.
package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

const (
	// DiscoveryTopic is the Wotan topic used for service announcements.
	DiscoveryTopic = "system.discovery"

	// AnnounceInterval is how often a registered service re-announces itself.
	AnnounceInterval = 15 * time.Second

	// DefaultTTL is the time-to-live for a service entry. If not re-announced
	// within this window the entry is considered stale and removed.
	DefaultTTL = 30 * time.Second

	// ReapInterval is how often the registry scans for stale entries.
	ReapInterval = 5 * time.Second
)

// Sentinel errors.
var (
	ErrServiceNotFound = errors.New("service not found")
	ErrAlreadyStopped  = errors.New("registry already stopped")
)

// WotanClient is the minimal interface required for publishing and subscribing
// to Wotan topics. Both the real Wotan client and test mocks satisfy this.
type WotanClient interface {
	Publish(topic string, data []byte) error
	Subscribe(topic string, handler func([]byte)) error
}

// ServiceEntry represents a discovered service in the mesh.
type ServiceEntry struct {
	Name     string            `json:"name"`
	Address  string            `json:"address"`
	Port     int               `json:"port"`
	Health   string            `json:"health"` // "healthy", "degraded", "unhealthy"
	LastSeen time.Time         `json:"last_seen"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Registry manages service discovery via Wotan announcements. It maintains a
// local in-memory view of all services that have announced themselves.
type Registry struct {
	services map[string]*ServiceEntry
	mu       sync.RWMutex

	wotan    WotanClient
	self     *ServiceEntry // the entry this registry announces (nil if not registered)
	watchers []func(ServiceEntry)
	watchMu  sync.RWMutex

	ttl  time.Duration
	done chan struct{}
	once sync.Once
}

// New creates a new service discovery registry backed by the given Wotan client.
func New(wotan WotanClient) *Registry {
	return &Registry{
		services: make(map[string]*ServiceEntry),
		wotan:    wotan,
		ttl:      DefaultTTL,
		done:     make(chan struct{}),
	}
}

// Register announces this service to the mesh by publishing to the discovery
// topic. The entry is re-announced every AnnounceInterval. The TTL for the
// entry is DefaultTTL; if not re-announced within that window other registries
// will consider the service stale.
func (r *Registry) Register(entry ServiceEntry) error {
	entry.LastSeen = time.Now()
	if entry.Health == "" {
		entry.Health = "healthy"
	}

	r.mu.Lock()
	r.self = &entry
	r.services[entry.Name] = &entry
	r.mu.Unlock()

	// Notify local watchers.
	r.notifyWatchers(entry)

	// Publish the initial announcement.
	return r.announce(entry)
}

// Resolve looks up a service by name. It returns ErrServiceNotFound if the
// service has not been announced or has expired.
func (r *Registry) Resolve(name string) (*ServiceEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.services[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrServiceNotFound, name)
	}
	// Return a copy to prevent callers from mutating registry state.
	cp := *entry
	return &cp, nil
}

// ResolveAll returns all services currently considered healthy.
func (r *Registry) ResolveAll() []ServiceEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]ServiceEntry, 0, len(r.services))
	for _, entry := range r.services {
		if entry.Health == "healthy" {
			result = append(result, *entry)
		}
	}
	return result
}

// Watch registers a callback that fires whenever a new or updated service
// entry is received.
func (r *Registry) Watch(handler func(ServiceEntry)) {
	r.watchMu.Lock()
	defer r.watchMu.Unlock()
	r.watchers = append(r.watchers, handler)
}

// Start begins the discovery loop. It subscribes to the discovery topic for
// incoming announcements, periodically re-announces the local service (if
// registered), and reaps stale entries.
func (r *Registry) Start(ctx context.Context) error {
	// Subscribe to discovery announcements.
	if err := r.wotan.Subscribe(DiscoveryTopic, r.handleAnnouncement); err != nil {
		return fmt.Errorf("subscribe to %s: %w", DiscoveryTopic, err)
	}

	// Announce loop.
	go r.announceLoop(ctx)

	// Reaper loop.
	go r.reapLoop(ctx)

	return nil
}

// Stop gracefully shuts down the registry. It is safe to call multiple times.
func (r *Registry) Stop() {
	r.once.Do(func() {
		close(r.done)
	})
}

// ----------------------------------------------------------------------------
// Internal helpers
// ----------------------------------------------------------------------------

// announce publishes a ServiceEntry to the discovery topic.
func (r *Registry) announce(entry ServiceEntry) error {
	entry.LastSeen = time.Now()

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal announcement: %w", err)
	}
	return r.wotan.Publish(DiscoveryTopic, data)
}

// announceLoop periodically re-announces the local service.
func (r *Registry) announceLoop(ctx context.Context) {
	ticker := time.NewTicker(AnnounceInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.done:
			return
		case <-ticker.C:
			r.mu.RLock()
			self := r.self
			r.mu.RUnlock()

			if self != nil {
				// Best-effort re-announce; errors are not fatal.
				_ = r.announce(*self)
			}
		}
	}
}

// reapLoop periodically removes stale service entries.
func (r *Registry) reapLoop(ctx context.Context) {
	ticker := time.NewTicker(ReapInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.done:
			return
		case <-ticker.C:
			r.reapStale()
		}
	}
}

// reapStale removes entries whose LastSeen is older than the TTL.
func (r *Registry) reapStale() {
	now := time.Now()

	r.mu.Lock()
	defer r.mu.Unlock()

	for name, entry := range r.services {
		// Never reap the local service (we control its lifecycle).
		if r.self != nil && name == r.self.Name {
			continue
		}
		if now.Sub(entry.LastSeen) > r.ttl {
			delete(r.services, name)
		}
	}
}

// handleAnnouncement processes an incoming service announcement from Wotan.
func (r *Registry) handleAnnouncement(data []byte) {
	var entry ServiceEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return // silently drop malformed announcements
	}
	if entry.Name == "" {
		return
	}

	entry.LastSeen = time.Now()

	r.mu.Lock()
	r.services[entry.Name] = &entry
	r.mu.Unlock()

	r.notifyWatchers(entry)
}

// notifyWatchers invokes all registered watch callbacks with the given entry.
func (r *Registry) notifyWatchers(entry ServiceEntry) {
	r.watchMu.RLock()
	defer r.watchMu.RUnlock()

	for _, fn := range r.watchers {
		fn(entry)
	}
}
