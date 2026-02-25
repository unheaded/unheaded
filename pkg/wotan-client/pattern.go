// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package wotanClient

import "strings"

// MatchTopic checks if a topic name matches a pattern.
//
// Patterns use dot-separated segments with wildcard support:
//   - Exact match: "tasks.created" matches only "tasks.created"
//   - Single-level wildcard: "tasks.*" matches "tasks.created" but NOT "tasks.created.bulk"
//   - Multi-level wildcard: "tasks.#" matches "tasks.created" AND "tasks.created.bulk"
//   - Global wildcard: "*" matches any non-empty topic
//
// Rules:
//   - Empty pattern or empty topic → false
//   - "*" alone → matches any non-empty topic
//   - "#" alone → matches any non-empty topic
//   - Segment "*" matches exactly one segment
//   - Segment "#" matches one or more remaining segments (must be last segment)
//   - Everything else is literal case-sensitive comparison
//
// This is a pure function with zero allocations on the hot path (no regex, no
// intermediate slices when possible). For patterns without wildcards, it falls
// through to a simple string equality check.
func MatchTopic(pattern, topic string) bool {
	if pattern == "" || topic == "" {
		return false
	}

	// Fast path: global wildcard
	if pattern == "*" || pattern == "#" {
		return true
	}

	// Fast path: exact match (no wildcard characters at all)
	if !strings.ContainsAny(pattern, "*#") {
		return pattern == topic
	}

	// Segment-by-segment matching using index walking to avoid
	// allocating a []string from strings.Split on the hot path.
	pRem := pattern
	tRem := topic

	for pRem != "" {
		// Extract next pattern segment
		var pSeg string
		if i := strings.IndexByte(pRem, '.'); i >= 0 {
			pSeg = pRem[:i]
			pRem = pRem[i+1:]
		} else {
			pSeg = pRem
			pRem = ""
		}

		// Multi-level wildcard "#" — must be last segment
		if pSeg == "#" {
			// "#" is only valid as the final segment
			if pRem != "" {
				return false
			}
			// Matches one or more remaining topic segments
			return tRem != ""
		}

		// If topic is exhausted but pattern has more segments, no match
		if tRem == "" {
			return false
		}

		// Extract next topic segment
		var tSeg string
		if i := strings.IndexByte(tRem, '.'); i >= 0 {
			tSeg = tRem[:i]
			tRem = tRem[i+1:]
		} else {
			tSeg = tRem
			tRem = ""
		}

		// Single-level wildcard "*" matches exactly one segment
		if pSeg == "*" {
			continue
		}

		// Literal comparison
		if pSeg != tSeg {
			return false
		}
	}

	// Both pattern and topic must be fully consumed
	return tRem == ""
}

// FilterTopics returns all topics from the list that match the given pattern.
func FilterTopics(pattern string, topics []string) []string {
	if pattern == "" {
		return nil
	}

	var result []string
	for _, topic := range topics {
		if MatchTopic(pattern, topic) {
			result = append(result, topic)
		}
	}
	return result
}
