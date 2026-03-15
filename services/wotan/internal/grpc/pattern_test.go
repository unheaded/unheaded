// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package grpc

import (
	"strings"
	"testing"
)

func TestMatchTopic(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		topic   string
		want    bool
	}{
		// Exact matches
		{name: "exact match simple", pattern: "tasks.created", topic: "tasks.created", want: true},
		{name: "exact match single segment", pattern: "tasks", topic: "tasks", want: true},
		{name: "exact match complex", pattern: "a.b.c.d.e", topic: "a.b.c.d.e", want: true},
		{name: "exact no match different", pattern: "tasks.created", topic: "tasks.updated", want: false},
		{name: "exact no match different length", pattern: "tasks.created", topic: "tasks", want: false},
		{name: "exact no match extra segment", pattern: "tasks", topic: "tasks.created", want: false},

		// Single wildcard "*"
		{name: "single wildcard one segment", pattern: "tasks.*", topic: "tasks.created", want: true},
		{name: "single wildcard different segment", pattern: "tasks.*", topic: "tasks.updated", want: true},
		{name: "single wildcard too many segments", pattern: "tasks.*", topic: "tasks.created.sub", want: false},
		{name: "single wildcard not enough segments", pattern: "tasks.*", topic: "tasks", want: false},
		{name: "single wildcard in middle", pattern: "a.*.c", topic: "a.b.c", want: true},
		{name: "single wildcard in middle no match", pattern: "a.*.c", topic: "a.b.d", want: false},
		{name: "multiple single wildcards", pattern: "a.*.c.*", topic: "a.b.c.d", want: true},
		{name: "multiple single wildcards no match", pattern: "a.*.c.*", topic: "a.b.d.e", want: false},

		// Multi-level wildcard "#"
		{name: "hash wildcard one segment", pattern: "tasks.#", topic: "tasks.created", want: true},
		{name: "hash wildcard multiple segments", pattern: "tasks.#", topic: "tasks.a.b.c", want: true},
		{name: "hash wildcard deep nesting", pattern: "a.#", topic: "a.b.c.d.e.f.g", want: true},
		{name: "hash wildcard not enough segments", pattern: "tasks.#", topic: "tasks", want: false},
		{name: "hash wildcard alone matches single", pattern: "#", topic: "tasks", want: true},
		{name: "hash wildcard alone matches multiple", pattern: "#", topic: "a.b.c", want: true},
		{name: "hash no match prefix", pattern: "tasks.#", topic: "timeline.created", want: false},

		// Global wildcard "*" alone
		{name: "global wildcard single segment", pattern: "*", topic: "anything", want: true},
		{name: "global wildcard multiple segments", pattern: "*", topic: "a.b.c", want: true},
		{name: "global wildcard long string", pattern: "*", topic: "very.long.topic.name.here", want: true},

		// Edge cases: empty strings
		{name: "empty pattern", pattern: "", topic: "tasks.created", want: false},
		{name: "empty topic", pattern: "tasks.created", topic: "", want: false},
		{name: "both empty", pattern: "", topic: "", want: false},
		{name: "empty pattern wildcard", pattern: "", topic: "*", want: false},

		// Edge cases: dots and special patterns
		{name: "pattern dots only", pattern: ".", topic: "a", want: false},
		{name: "topic dots only", pattern: "a", topic: ".", want: false},
		{name: "pattern multiple dots", pattern: "..", topic: "a.b", want: false},
		{name: "consecutive dots in pattern", pattern: "a..b", topic: "a..b", want: true},
		{name: "consecutive dots no match", pattern: "a..b", topic: "a.x.b", want: false},

		// Single segment patterns and topics
		{name: "single segment exact", pattern: "test", topic: "test", want: true},
		{name: "single segment no match", pattern: "test", topic: "other", want: false},
		{name: "single segment wildcard", pattern: "*", topic: "test", want: true},
		{name: "single segment hash", pattern: "#", topic: "test", want: true},

		// Complex patterns
		{name: "complex with mixed wildcards", pattern: "a.*.c.#", topic: "a.b.c.d.e.f", want: true},
		{name: "complex with mixed wildcards no match", pattern: "a.*.c.#", topic: "a.b.d.e.f", want: false},
		{name: "pattern hash not at end", pattern: "a.#.c", topic: "a.b.c", want: false},

		// Case sensitivity
		{name: "case sensitive match", pattern: "Tasks.Created", topic: "Tasks.Created", want: true},
		{name: "case sensitive no match", pattern: "tasks.created", topic: "Tasks.Created", want: false},

		// Real-world examples
		{name: "real world order event", pattern: "orders.#", topic: "orders.created", want: true},
		{name: "real world order event nested", pattern: "orders.#", topic: "orders.payment.completed", want: true},
		{name: "real world user wildcard", pattern: "user.*", topic: "user.login", want: true},
		{name: "real world user wildcard no match", pattern: "user.*", topic: "user.login.attempt", want: false},
		{name: "real world global", pattern: "*", topic: "system.heartbeat", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchTopic(tt.pattern, tt.topic)
			if got != tt.want {
				t.Errorf("MatchTopic(%q, %q) = %v, want %v", tt.pattern, tt.topic, got, tt.want)
			}
		})
	}
}

func TestFilterTopics(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		topics  []string
		want    []string
	}{
		{
			name:    "exact match filtering",
			pattern: "tasks.created",
			topics:  []string{"tasks.created", "tasks.updated", "timeline.created"},
			want:    []string{"tasks.created"},
		},
		{
			name:    "single wildcard filtering",
			pattern: "tasks.*",
			topics:  []string{"tasks.created", "tasks.updated", "timeline.created", "tasks.deleted"},
			want:    []string{"tasks.created", "tasks.updated", "tasks.deleted"},
		},
		{
			name:    "hash wildcard filtering",
			pattern: "order.#",
			topics:  []string{"order.created", "order.payment.completed", "order.shipping.started", "user.created"},
			want:    []string{"order.created", "order.payment.completed", "order.shipping.started"},
		},
		{
			name:    "global wildcard filtering",
			pattern: "*",
			topics:  []string{"a", "b.c", "d.e.f"},
			want:    []string{"a", "b.c", "d.e.f"},
		},
		{
			name:    "no matches",
			pattern: "tasks.#",
			topics:  []string{"user.created", "user.deleted"},
			want:    []string{},
		},
		{
			name:    "empty topic list",
			pattern: "tasks.*",
			topics:  []string{},
			want:    []string{},
		},
		{
			name:    "empty pattern returns empty",
			pattern: "",
			topics:  []string{"a", "b"},
			want:    []string{},
		},
		{
			name:    "filter with empty topics in list",
			pattern: "tasks.*",
			topics:  []string{"tasks.created", "", "tasks.updated"},
			want:    []string{"tasks.created", "tasks.updated"},
		},
		{
			name:    "complex pattern filtering",
			pattern: "a.*.c.#",
			topics:  []string{"a.x.c.d", "a.x.c.d.e.f", "a.x.d.d", "a.b.c"},
			want:    []string{"a.x.c.d", "a.x.c.d.e.f"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterTopics(tt.pattern, tt.topics)
			if !slicesEqual(got, tt.want) {
				t.Errorf("FilterTopics(%q, %v) = %v, want %v", tt.pattern, tt.topics, got, tt.want)
			}
		})
	}
}

// slicesEqual compares two string slices for equality (order matters)
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Benchmarks

func BenchmarkMatchTopic_Exact(b *testing.B) {
	pattern := "very.long.exact.topic.name.with.many.segments"
	topic := "very.long.exact.topic.name.with.many.segments"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = MatchTopic(pattern, topic)
	}
}

func BenchmarkMatchTopic_SingleWildcard(b *testing.B) {
	pattern := "tasks.*"
	topic := "tasks.created"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = MatchTopic(pattern, topic)
	}
}

func BenchmarkMatchTopic_MultiWildcard(b *testing.B) {
	pattern := "tasks.#"
	topic := "tasks.a.b.c.d.e.f"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = MatchTopic(pattern, topic)
	}
}

func BenchmarkMatchTopic_GlobalWildcard(b *testing.B) {
	pattern := "*"
	topic := "anything.goes.here"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = MatchTopic(pattern, topic)
	}
}

func BenchmarkMatchTopic_ComplexPattern(b *testing.B) {
	pattern := "a.*.c.*.e.#"
	topic := "a.x.c.y.e.f.g.h.i"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = MatchTopic(pattern, topic)
	}
}

func BenchmarkFilterTopics(b *testing.B) {
	pattern := "tasks.*"
	topics := []string{
		"tasks.created", "tasks.updated", "tasks.deleted", "tasks.assigned",
		"user.created", "user.updated",
		"order.created", "order.shipped",
		"timeline.post", "timeline.comment",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = FilterTopics(pattern, topics)
	}
}

func BenchmarkFilterTopics_Large(b *testing.B) {
	pattern := "order.#"
	// Create a large list of diverse topics
	topics := make([]string, 0, 1000)
	for i := 0; i < 100; i++ {
		topics = append(topics, strings.Join([]string{"order", "payment", "status", "confirmed"}, "."))
		topics = append(topics, strings.Join([]string{"order", "shipping", "tracking", "id", "12345"}, "."))
		topics = append(topics, strings.Join([]string{"user", "login", "attempt"}, "."))
		topics = append(topics, strings.Join([]string{"inventory", "stock", "updated"}, "."))
		topics = append(topics, strings.Join([]string{"notification", "sent"}, "."))
		topics = append(topics, strings.Join([]string{"analytics", "event", "recorded"}, "."))
		topics = append(topics, strings.Join([]string{"error", "exception", "logged"}, "."))
		topics = append(topics, strings.Join([]string{"cache", "invalidated"}, "."))
		topics = append(topics, strings.Join([]string{"database", "transaction", "committed"}, "."))
		topics = append(topics, strings.Join([]string{"auth", "token", "generated"}, "."))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = FilterTopics(pattern, topics)
	}
}
