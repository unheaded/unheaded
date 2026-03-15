// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package routing provides traffic routing for the service mesh.
// THE HAUBERK - Traffic routing for the Unheaded Kingdom.
package routing

import (
	"context"
	"errors"
	"sync"
)

var (
	// ErrNoMatchingRoute is returned when no route matches.
	ErrNoMatchingRoute = errors.New("no matching route")
	// ErrRouteNotFound is returned when a route is not found.
	ErrRouteNotFound = errors.New("route not found")
)

// Router manages traffic routing rules.
type Router struct {
	mu     sync.RWMutex
	rules  []*Rule
	byName map[string]*Rule
}

// NewRouter creates a new router.
func NewRouter() *Router {
	return &Router{
		rules:  make([]*Rule, 0),
		byName: make(map[string]*Rule),
	}
}

// AddRule adds a routing rule.
func (r *Router) AddRule(rule *Rule) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if rule.Name != "" {
		if _, exists := r.byName[rule.Name]; exists {
			// Update existing rule
			for i, existing := range r.rules {
				if existing.Name == rule.Name {
					r.rules[i] = rule
					r.byName[rule.Name] = rule
					return nil
				}
			}
		}
		r.byName[rule.Name] = rule
	}

	// Insert in priority order (higher priority first)
	inserted := false
	for i, existing := range r.rules {
		if rule.Priority > existing.Priority {
			r.rules = append(r.rules[:i], append([]*Rule{rule}, r.rules[i:]...)...)
			inserted = true
			break
		}
	}

	if !inserted {
		r.rules = append(r.rules, rule)
	}

	return nil
}

// RemoveRule removes a routing rule by name.
func (r *Router) RemoveRule(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.byName[name]; !exists {
		return ErrRouteNotFound
	}

	delete(r.byName, name)

	for i, rule := range r.rules {
		if rule.Name == name {
			r.rules = append(r.rules[:i], r.rules[i+1:]...)
			return nil
		}
	}

	return nil
}

// GetRule gets a rule by name.
func (r *Router) GetRule(name string) (*Rule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rule, exists := r.byName[name]
	if !exists {
		return nil, ErrRouteNotFound
	}

	return rule, nil
}

// Route finds the matching route for a request.
func (r *Router) Route(ctx context.Context, req *Request) (*RouteMatch, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, rule := range r.rules {
		if match := rule.Match(req); match != nil {
			return match, nil
		}
	}

	return nil, ErrNoMatchingRoute
}

// ListRules returns all rules.
func (r *Router) ListRules() []*Rule {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rules := make([]*Rule, len(r.rules))
	copy(rules, r.rules)
	return rules
}

// Request represents an incoming request.
type Request struct {
	// Method is the HTTP method (GET, POST, etc.).
	Method string
	// Path is the request path.
	Path string
	// Host is the request host.
	Host string
	// Headers are the request headers.
	Headers map[string]string
	// SourceService is the calling service.
	SourceService string
	// SourceNamespace is the caller's namespace.
	SourceNamespace string
	// Metadata holds additional request information.
	Metadata map[string]string
}

// Response represents a routed response.
type Response struct {
	// Service is the destination service.
	Service string
	// Endpoint is the selected endpoint address.
	Endpoint string
	// Headers are response headers to add.
	Headers map[string]string
	// StatusCode is the response status.
	StatusCode int
	// Body is the response body.
	Body []byte
}

// RouteMatch represents a successful route match.
type RouteMatch struct {
	// Rule is the matched rule.
	Rule *Rule
	// Destination is the selected destination.
	Destination *Destination
	// Headers are headers to modify.
	Headers map[string]string
	// Metadata is extracted from the match.
	Metadata map[string]string
}

// Destination represents a routing destination.
type Destination struct {
	// Service is the destination service name.
	Service string
	// Subset is the service subset (for traffic splitting).
	Subset string
	// Weight is the weight for traffic splitting.
	Weight int
	// Port is the destination port.
	Port int
}
