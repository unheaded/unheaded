// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package eventbus

import (
	"strings"
	"sync"
)

// TopicRouter manages topic-based routing with wildcard support.
// Supports patterns like:
//   - "users.created" - exact match
//   - "users.*" - matches any single segment after "users."
//   - "*.created" - matches any topic ending with ".created"
//   - "*" - matches all topics
type TopicRouter struct {
	mu       sync.RWMutex
	exact    map[string][]string // exact topic -> subscriber IDs
	prefix   map[string][]string // prefix patterns (e.g., "users.*")
	suffix   map[string][]string // suffix patterns (e.g., "*.created")
	wildcard []string            // global wildcard subscribers
	topics   map[string]struct{} // all registered topic patterns
}

// NewTopicRouter creates a new topic router.
func NewTopicRouter() *TopicRouter {
	return &TopicRouter{
		exact:  make(map[string][]string),
		prefix: make(map[string][]string),
		suffix: make(map[string][]string),
		topics: make(map[string]struct{}),
	}
}

// Register adds a subscriber to a topic pattern.
func (r *TopicRouter) Register(pattern, subID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.topics[pattern] = struct{}{}

	switch {
	case pattern == "*":
		r.wildcard = append(r.wildcard, subID)
	case strings.HasSuffix(pattern, ".*"):
		prefix := strings.TrimSuffix(pattern, ".*")
		r.prefix[prefix] = append(r.prefix[prefix], subID)
	case strings.HasPrefix(pattern, "*."):
		suffix := strings.TrimPrefix(pattern, "*.")
		r.suffix[suffix] = append(r.suffix[suffix], subID)
	default:
		r.exact[pattern] = append(r.exact[pattern], subID)
	}
}

// Unregister removes a subscriber from a topic pattern.
func (r *TopicRouter) Unregister(pattern, subID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	switch {
	case pattern == "*":
		r.wildcard = removeFromSlice(r.wildcard, subID)
	case strings.HasSuffix(pattern, ".*"):
		prefix := strings.TrimSuffix(pattern, ".*")
		r.prefix[prefix] = removeFromSlice(r.prefix[prefix], subID)
		if len(r.prefix[prefix]) == 0 {
			delete(r.prefix, prefix)
		}
	case strings.HasPrefix(pattern, "*."):
		suffix := strings.TrimPrefix(pattern, "*.")
		r.suffix[suffix] = removeFromSlice(r.suffix[suffix], subID)
		if len(r.suffix[suffix]) == 0 {
			delete(r.suffix, suffix)
		}
	default:
		r.exact[pattern] = removeFromSlice(r.exact[pattern], subID)
		if len(r.exact[pattern]) == 0 {
			delete(r.exact, pattern)
		}
	}

	// Clean up topics map if no subscribers remain
	if !r.hasSubscribers(pattern) {
		delete(r.topics, pattern)
	}
}

// Match returns all subscriber IDs that match the given topic.
func (r *TopicRouter) Match(topic string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []string
	seen := make(map[string]struct{})

	// Add exact matches
	for _, subID := range r.exact[topic] {
		if _, ok := seen[subID]; !ok {
			result = append(result, subID)
			seen[subID] = struct{}{}
		}
	}

	// Add prefix matches (e.g., "users.*" matches "users.created")
	// The pattern "users.*" requires at least one segment after "users."
	parts := strings.Split(topic, ".")
	if len(parts) >= 2 {
		// Only check prefixes if topic has at least 2 segments
		// because pattern "prefix.*" requires something after the dot
		for i := 1; i < len(parts); i++ {
			prefix := strings.Join(parts[:i], ".")
			for _, subID := range r.prefix[prefix] {
				if _, ok := seen[subID]; !ok {
					result = append(result, subID)
					seen[subID] = struct{}{}
				}
			}
		}
	}

	// Add suffix matches (e.g., "*.created" matches "users.created")
	for i := 0; i < len(parts); i++ {
		suffix := strings.Join(parts[i:], ".")
		for _, subID := range r.suffix[suffix] {
			if _, ok := seen[subID]; !ok {
				result = append(result, subID)
				seen[subID] = struct{}{}
			}
		}
	}

	// Add wildcard matches
	for _, subID := range r.wildcard {
		if _, ok := seen[subID]; !ok {
			result = append(result, subID)
			seen[subID] = struct{}{}
		}
	}

	return result
}

// Count returns the number of registered topic patterns.
func (r *TopicRouter) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.topics)
}

// Topics returns all registered topic patterns.
func (r *TopicRouter) Topics() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	topics := make([]string, 0, len(r.topics))
	for topic := range r.topics {
		topics = append(topics, topic)
	}
	return topics
}

// MatchPattern checks if a topic matches a pattern.
func MatchPattern(pattern, topic string) bool {
	switch {
	case pattern == "*":
		return true
	case pattern == topic:
		return true
	case strings.HasSuffix(pattern, ".*"):
		prefix := strings.TrimSuffix(pattern, ".*")
		return strings.HasPrefix(topic, prefix+".")
	case strings.HasPrefix(pattern, "*."):
		suffix := strings.TrimPrefix(pattern, "*.")
		return strings.HasSuffix(topic, "."+suffix) || topic == suffix
	default:
		return false
	}
}

// hasSubscribers checks if a pattern has any subscribers.
func (r *TopicRouter) hasSubscribers(pattern string) bool {
	switch {
	case pattern == "*":
		return len(r.wildcard) > 0
	case strings.HasSuffix(pattern, ".*"):
		prefix := strings.TrimSuffix(pattern, ".*")
		return len(r.prefix[prefix]) > 0
	case strings.HasPrefix(pattern, "*."):
		suffix := strings.TrimPrefix(pattern, "*.")
		return len(r.suffix[suffix]) > 0
	default:
		return len(r.exact[pattern]) > 0
	}
}

// removeFromSlice removes an element from a slice.
func removeFromSlice(slice []string, elem string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != elem {
			result = append(result, s)
		}
	}
	return result
}

// TopicMatcher provides advanced topic matching capabilities.
type TopicMatcher struct {
	segments []patternSegment
}

type patternSegment struct {
	value    string
	wildcard bool
}

// NewTopicMatcher creates a matcher from a pattern.
func NewTopicMatcher(pattern string) *TopicMatcher {
	parts := strings.Split(pattern, ".")
	segments := make([]patternSegment, len(parts))

	for i, part := range parts {
		segments[i] = patternSegment{
			value:    part,
			wildcard: part == "*",
		}
	}

	return &TopicMatcher{segments: segments}
}

// Matches checks if a topic matches the pattern.
func (m *TopicMatcher) Matches(topic string) bool {
	parts := strings.Split(topic, ".")

	if len(parts) != len(m.segments) {
		// Special case: trailing wildcard matches any number of segments
		if len(m.segments) > 0 && m.segments[len(m.segments)-1].wildcard {
			if len(parts) >= len(m.segments)-1 {
				// Check all non-wildcard segments match
				for i := 0; i < len(m.segments)-1; i++ {
					if !m.segments[i].wildcard && m.segments[i].value != parts[i] {
						return false
					}
				}
				return true
			}
		}
		return false
	}

	for i, seg := range m.segments {
		if !seg.wildcard && seg.value != parts[i] {
			return false
		}
	}

	return true
}
