// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package wotanClient

import (
	"testing"
)

// FuzzMatchTopic feeds random pattern/topic pairs to MatchTopic.
// The function must never panic. This is the hot path for message routing.
func FuzzMatchTopic(f *testing.F) {
	// Seed corpus: real-world patterns from Unheaded.
	f.Add("tasks.created", "tasks.created")
	f.Add("tasks.*", "tasks.created")
	f.Add("tasks.#", "tasks.created.bulk")
	f.Add("*", "anything.at.all")
	f.Add("#", "deep.nested.topic.path")
	f.Add("ebpf.#", "ebpf.packet.events")
	f.Add("ebpf.*", "ebpf.packet")
	f.Add("a.*.c", "a.b.c")
	f.Add("system.alerts.#", "system.alerts.critical.database")

	// Edge cases.
	f.Add("", "")
	f.Add("", "topic")
	f.Add("pattern", "")
	f.Add("*", "")
	f.Add("#", "")
	f.Add(".", ".")
	f.Add("..", "..")
	f.Add("a..b", "a..b")
	f.Add("tasks.#.more", "tasks.a.more")

	// Adversarial inputs.
	f.Add("*.*.*.*.*.*.*.*.*.*", "a.b.c.d.e.f.g.h.i.j")
	f.Add("#", "a.b.c.d.e.f.g.h.i.j.k.l.m.n.o.p.q.r.s.t")

	f.Fuzz(func(t *testing.T, pattern, topic string) {
		// MatchTopic must never panic.
		result := MatchTopic(pattern, topic)

		// Invariant: empty pattern always returns false.
		if pattern == "" && result {
			t.Errorf("MatchTopic(%q, %q) = true, empty pattern should always be false", pattern, topic)
		}

		// Invariant: empty topic always returns false.
		if topic == "" && result {
			t.Errorf("MatchTopic(%q, %q) = true, empty topic should always be false", pattern, topic)
		}

		// Invariant: exact match (no wildcards) should equal string comparison.
		if pattern != "" && topic != "" &&
			!containsWildcard(pattern) {
			expected := (pattern == topic)
			if result != expected {
				t.Errorf("MatchTopic(%q, %q) = %v, want %v (exact match, no wildcards)",
					pattern, topic, result, expected)
			}
		}
	})
}

// containsWildcard checks if a pattern contains * or # characters.
func containsWildcard(pattern string) bool {
	for _, c := range pattern {
		if c == '*' || c == '#' {
			return true
		}
	}
	return false
}

// FuzzFilterTopics feeds random patterns and topic lists to FilterTopics.
func FuzzFilterTopics(f *testing.F) {
	f.Add("tasks.*", "tasks.created\ntasks.updated\ntimeline.updates")
	f.Add("#", "a\nb\nc")
	f.Add("", "a\nb")

	f.Fuzz(func(t *testing.T, pattern, topicsRaw string) {
		// Split raw string into topic list.
		var topics []string
		start := 0
		for i := 0; i < len(topicsRaw); i++ {
			if topicsRaw[i] == '\n' {
				if i > start {
					topics = append(topics, topicsRaw[start:i])
				}
				start = i + 1
			}
		}
		if start < len(topicsRaw) {
			topics = append(topics, topicsRaw[start:])
		}

		result := FilterTopics(pattern, topics)

		// Invariant: result length should not exceed input length.
		if len(result) > len(topics) {
			t.Errorf("FilterTopics returned %d results from %d topics", len(result), len(topics))
		}

		// Invariant: every result should match the pattern.
		for _, r := range result {
			if !MatchTopic(pattern, r) {
				t.Errorf("FilterTopics returned %q which does not match pattern %q", r, pattern)
			}
		}

		// Invariant: empty pattern returns nil/empty.
		if pattern == "" && len(result) > 0 {
			t.Errorf("FilterTopics with empty pattern returned %d results", len(result))
		}
	})
}
