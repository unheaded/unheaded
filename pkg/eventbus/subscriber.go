// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package eventbus

import (
	"sync"
	"time"
)

// Subscription represents a registered event handler.
type Subscription struct {
	ID      string
	Pattern string
	Handler Handler
	Filter  FilterFunc
	Created time.Time
}

// SubscriberRegistry manages active subscriptions.
type SubscriberRegistry struct {
	mu            sync.RWMutex
	subscriptions map[string]*Subscription
}

// NewSubscriberRegistry creates a new subscriber registry.
func NewSubscriberRegistry() *SubscriberRegistry {
	return &SubscriberRegistry{
		subscriptions: make(map[string]*Subscription),
	}
}

// Add registers a new subscription.
func (r *SubscriberRegistry) Add(sub *Subscription) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subscriptions[sub.ID] = sub
}

// Remove unregisters a subscription by ID.
func (r *SubscriberRegistry) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.subscriptions, id)
}

// Get retrieves a subscription by ID.
func (r *SubscriberRegistry) Get(id string) *Subscription {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.subscriptions[id]
}

// Count returns the number of active subscriptions.
func (r *SubscriberRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.subscriptions)
}

// All returns all active subscriptions.
func (r *SubscriberRegistry) All() []*Subscription {
	r.mu.RLock()
	defer r.mu.RUnlock()

	subs := make([]*Subscription, 0, len(r.subscriptions))
	for _, sub := range r.subscriptions {
		subs = append(subs, sub)
	}
	return subs
}

// ByPattern returns all subscriptions matching a pattern.
func (r *SubscriberRegistry) ByPattern(pattern string) []*Subscription {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var subs []*Subscription
	for _, sub := range r.subscriptions {
		if sub.Pattern == pattern {
			subs = append(subs, sub)
		}
	}
	return subs
}

// Clear removes all subscriptions.
func (r *SubscriberRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subscriptions = make(map[string]*Subscription)
}

// SubscriptionInfo provides read-only subscription details.
type SubscriptionInfo struct {
	ID      string
	Pattern string
	Created time.Time
}

// Info returns read-only information about a subscription.
func (s *Subscription) Info() SubscriptionInfo {
	return SubscriptionInfo{
		ID:      s.ID,
		Pattern: s.Pattern,
		Created: s.Created,
	}
}

// ListSubscriptions returns info about all active subscriptions.
func (r *SubscriberRegistry) ListSubscriptions() []SubscriptionInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	infos := make([]SubscriptionInfo, 0, len(r.subscriptions))
	for _, sub := range r.subscriptions {
		infos = append(infos, sub.Info())
	}
	return infos
}
