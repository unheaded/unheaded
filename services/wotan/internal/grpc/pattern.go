// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package grpc

import "strings"

// MatchTopic checks if a topic name matches a pattern.
// Patterns:
//   - Exact: "tasks.created" matches only "tasks.created"
//   - Single wildcard: "tasks.*" matches "tasks.created", "tasks.updated" (one segment)
//   - Multi wildcard: "tasks.#" matches "tasks.created", "tasks.a.b.c" (any depth)
//   - Global: "*" matches everything
//
// Topics and patterns are dot-separated. Empty strings never match.
//
// Rules:
//   - Empty pattern or empty topic → false
//   - "*" alone → matches any non-empty topic
//   - "#" alone → matches any non-empty topic
//   - Segment "*" matches exactly one segment
//   - Segment "#" matches one or more remaining segments (must be last segment)
//   - Everything else is literal comparison
//   - Case-sensitive
func MatchTopic(pattern, topic string) bool {
	// Empty pattern or empty topic never match
	if pattern == "" || topic == "" {
		return false
	}

	// Special case: "*" alone matches any non-empty topic
	if pattern == "*" {
		return true
	}

	// Special case: "#" alone matches any non-empty topic
	if pattern == "#" {
		return true
	}

	patternSegments := strings.Split(pattern, ".")
	topicSegments := strings.Split(topic, ".")

	patternIdx := 0
	topicIdx := 0

	for patternIdx < len(patternSegments) {
		patternSeg := patternSegments[patternIdx]

		// Handle multi-level wildcard "#"
		if patternSeg == "#" {
			// "#" must be the last segment in the pattern
			if patternIdx != len(patternSegments)-1 {
				return false
			}
			// "#" matches one or more remaining segments
			// Ensure there is at least one topic segment remaining
			return topicIdx < len(topicSegments)
		}

		// If we've exhausted topic segments but pattern has more, no match
		if topicIdx >= len(topicSegments) {
			return false
		}

		topicSeg := topicSegments[topicIdx]

		// Handle single-level wildcard "*"
		if patternSeg == "*" {
			// "*" matches exactly one segment (which we already have)
			patternIdx++
			topicIdx++
			continue
		}

		// Literal comparison
		if patternSeg != topicSeg {
			return false
		}

		patternIdx++
		topicIdx++
	}

	// Both pattern and topic must be fully consumed
	return patternIdx == len(patternSegments) && topicIdx == len(topicSegments)
}

// FilterTopics returns all topics from the list that match the pattern.
func FilterTopics(pattern string, topics []string) []string {
	if pattern == "" {
		return []string{}
	}

	var result []string
	for _, topic := range topics {
		if MatchTopic(pattern, topic) {
			result = append(result, topic)
		}
	}
	return result
}
