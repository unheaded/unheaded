// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package routing provides traffic routing for the service mesh.
// THE HAUBERK - Routing rule definitions for the Unheaded Kingdom.
package routing

import (
	"math/rand"
	"time"
)

// Rule defines a routing rule.
type Rule struct {
	// Name is the rule name.
	Name string
	// Priority determines evaluation order (higher = first).
	Priority int
	// Matches are the conditions to match.
	Matches []*Match
	// Destinations are the possible destinations.
	Destinations []*Destination
	// Headers to add/modify on match.
	Headers *HeaderOperation
	// Retry configuration for this route.
	Retry *RetryConfig
	// Timeout for this route.
	Timeout time.Duration
	// Mirror configuration for traffic mirroring.
	Mirror *MirrorConfig
	// Fault injection configuration.
	Fault *FaultConfig
}

// Match checks if a request matches this rule.
func (r *Rule) Match(req *Request) *RouteMatch {
	// If no matches defined, always match
	if len(r.Matches) == 0 {
		return r.selectDestination(req)
	}

	// Any match condition must match
	for _, m := range r.Matches {
		if m.Matches(req) {
			return r.selectDestination(req)
		}
	}

	return nil
}

// selectDestination selects a destination based on weights.
func (r *Rule) selectDestination(req *Request) *RouteMatch {
	if len(r.Destinations) == 0 {
		return nil
	}

	// Calculate total weight
	totalWeight := 0
	for _, d := range r.Destinations {
		weight := d.Weight
		if weight <= 0 {
			weight = 1
		}
		totalWeight += weight
	}

	// Weighted random selection
	target := rand.Intn(totalWeight) // #nosec G404 -- weighted route selection, not security-bearing
	cumulative := 0

	var selected *Destination
	for _, d := range r.Destinations {
		weight := d.Weight
		if weight <= 0 {
			weight = 1
		}
		cumulative += weight
		if target < cumulative {
			selected = d
			break
		}
	}

	if selected == nil {
		selected = r.Destinations[0]
	}

	match := &RouteMatch{
		Rule:        r,
		Destination: selected,
		Metadata:    make(map[string]string),
	}

	if r.Headers != nil {
		match.Headers = r.Headers.Apply(req.Headers)
	}

	return match
}

// RetryConfig configures retry behavior for a route.
type RetryConfig struct {
	// Attempts is the number of retry attempts.
	Attempts int
	// PerTryTimeout is the timeout per attempt.
	PerTryTimeout time.Duration
	// RetryOn specifies which conditions trigger retry.
	RetryOn []string
}

// MirrorConfig configures traffic mirroring.
type MirrorConfig struct {
	// Service to mirror traffic to.
	Service string
	// Subset of the mirror service.
	Subset string
	// Percentage of traffic to mirror (0-100).
	Percentage int
}

// FaultConfig configures fault injection.
type FaultConfig struct {
	// Delay configuration.
	Delay *DelayFault
	// Abort configuration.
	Abort *AbortFault
}

// DelayFault injects delays.
type DelayFault struct {
	// Duration is the delay duration.
	Duration time.Duration
	// Percentage of requests to delay (0-100).
	Percentage int
}

// AbortFault injects failures.
type AbortFault struct {
	// StatusCode to return.
	StatusCode int
	// Percentage of requests to abort (0-100).
	Percentage int
}

// HeaderOperation defines header modifications.
type HeaderOperation struct {
	// Set headers (overwrites existing).
	Set map[string]string
	// Add headers (appends to existing).
	Add map[string]string
	// Remove headers.
	Remove []string
}

// Apply applies header operations to headers.
func (h *HeaderOperation) Apply(headers map[string]string) map[string]string {
	result := make(map[string]string)

	// Copy existing headers
	for k, v := range headers {
		result[k] = v
	}

	// Remove headers
	for _, k := range h.Remove {
		delete(result, k)
	}

	// Set headers
	for k, v := range h.Set {
		result[k] = v
	}

	// Add headers (only if not exists)
	for k, v := range h.Add {
		if _, exists := result[k]; !exists {
			result[k] = v
		}
	}

	return result
}

// RuleBuilder helps build routing rules.
type RuleBuilder struct {
	rule *Rule
}

// NewRuleBuilder creates a new rule builder.
func NewRuleBuilder(name string) *RuleBuilder {
	return &RuleBuilder{
		rule: &Rule{
			Name:         name,
			Matches:      make([]*Match, 0),
			Destinations: make([]*Destination, 0),
		},
	}
}

// Priority sets the rule priority.
func (b *RuleBuilder) Priority(p int) *RuleBuilder {
	b.rule.Priority = p
	return b
}

// AddMatch adds a match condition.
func (b *RuleBuilder) AddMatch(m *Match) *RuleBuilder {
	b.rule.Matches = append(b.rule.Matches, m)
	return b
}

// AddDestination adds a destination.
func (b *RuleBuilder) AddDestination(service string, weight int) *RuleBuilder {
	b.rule.Destinations = append(b.rule.Destinations, &Destination{
		Service: service,
		Weight:  weight,
	})
	return b
}

// Timeout sets the route timeout.
func (b *RuleBuilder) Timeout(t time.Duration) *RuleBuilder {
	b.rule.Timeout = t
	return b
}

// Retry configures retry behavior.
func (b *RuleBuilder) Retry(attempts int, perTryTimeout time.Duration) *RuleBuilder {
	b.rule.Retry = &RetryConfig{
		Attempts:      attempts,
		PerTryTimeout: perTryTimeout,
	}
	return b
}

// Build returns the built rule.
func (b *RuleBuilder) Build() *Rule {
	return b.rule
}
