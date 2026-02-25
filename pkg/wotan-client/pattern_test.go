// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package wotanClient

import (
	"testing"
)

func TestMatchTopic(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		topic   string
		want    bool
	}{
		// === Exact match ===
		{"exact match", "tasks.created", "tasks.created", true},
		{"exact no match", "tasks.created", "tasks.updated", false},
		{"exact single segment", "tasks", "tasks", true},
		{"exact single no match", "tasks", "events", false},
		{"exact multi segment", "a.b.c.d", "a.b.c.d", true},
		{"exact multi no match", "a.b.c.d", "a.b.c.e", false},
		{"exact different depth", "a.b", "a.b.c", false},
		{"exact shorter topic", "a.b.c", "a.b", false},

		// === Single-level wildcard (*) ===
		{"star matches one", "tasks.*", "tasks.created", true},
		{"star matches another", "tasks.*", "tasks.updated", true},
		{"star matches deleted", "tasks.*", "tasks.deleted", true},
		{"star no match deeper", "tasks.*", "tasks.created.bulk", false},
		{"star middle", "a.*.c", "a.b.c", true},
		{"star middle no match", "a.*.c", "a.b.d", false},
		{"star middle wrong depth", "a.*.c", "a.b.c.d", false},
		{"star first", "*.tasks", "system.tasks", true},
		{"star first no match depth", "*.tasks", "system.tasks.extra", false},
		{"multiple stars", "*.*", "a.b", true},
		{"multiple stars no match", "*.*", "a", false},
		{"multiple stars three", "*.*.*", "a.b.c", true},
		{"multiple stars three fail", "*.*.*", "a.b", false},

		// === Multi-level wildcard (#) ===
		{"hash matches one", "tasks.#", "tasks.created", true},
		{"hash matches two", "tasks.#", "tasks.created.bulk", true},
		{"hash matches deep", "tasks.#", "tasks.a.b.c.d.e", true},
		{"hash no match empty", "tasks.#", "tasks", false},
		{"hash with prefix", "system.alerts.#", "system.alerts.critical", true},
		{"hash with prefix deep", "system.alerts.#", "system.alerts.critical.database", true},
		{"hash only", "#", "anything", true},
		{"hash only deep", "#", "a.b.c.d", true},

		// === Global wildcard ===
		{"global star", "*", "tasks.created", true},
		{"global star single", "*", "anything", true},
		{"global star deep", "*", "a.b.c.d.e.f", true},

		// === Empty inputs ===
		{"empty pattern", "", "tasks.created", false},
		{"empty topic", "tasks.*", "", false},
		{"both empty", "", "", false},

		// === Edge cases ===
		{"trailing dot pattern", "tasks.", "tasks.", true},
		{"trailing dot topic", "tasks.*", "tasks.", false}, // empty segment is not a valid match for *
		{"double dots pattern", "tasks..created", "tasks..created", true},
		{"single segment star", "*", "x", true},
		{"hash not last invalid", "tasks.#.more", "tasks.a.more", false},
		{"same segments different order", "a.b", "b.a", false},

		// === eBPF topic patterns (real-world usage) ===
		{"ebpf all events", "ebpf.#", "ebpf.packet.events", true},
		{"ebpf all events deep", "ebpf.#", "ebpf.flow.events.detailed", true},
		{"ebpf single level", "ebpf.*", "ebpf.packet", true},
		{"ebpf single level no match", "ebpf.*", "ebpf.packet.events", false},
		{"ebpf exact", "ebpf.packet.events", "ebpf.packet.events", true},
		{"ebpf exact no match", "ebpf.packet.events", "ebpf.flow.events", false},

		// === Task topics (Kanban usage) ===
		{"tasks wildcard", "tasks.*", "tasks.created", true},
		{"tasks wildcard update", "tasks.*", "tasks.updated", true},
		{"tasks wildcard delete", "tasks.*", "tasks.deleted", true},
		{"timeline exact", "timeline.updates", "timeline.updates", true},
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
	topics := []string{
		"tasks.created",
		"tasks.updated",
		"tasks.deleted",
		"tasks.created.bulk",
		"timeline.updates",
		"ebpf.packet.events",
		"ebpf.flow.events",
		"system.alerts.critical",
	}

	tests := []struct {
		name    string
		pattern string
		want    int
	}{
		{"tasks star", "tasks.*", 3},       // created, updated, deleted
		{"tasks hash", "tasks.#", 4},       // all four tasks.*
		{"ebpf hash", "ebpf.#", 2},        // packet.events, flow.events
		{"global", "*", 8},                 // everything
		{"exact", "timeline.updates", 1},   // just timeline.updates
		{"no match", "nonexistent", 0},     // nothing
		{"empty pattern", "", 0},           // nothing
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterTopics(tt.pattern, topics)
			if len(result) != tt.want {
				t.Errorf("FilterTopics(%q) returned %d topics, want %d: %v",
					tt.pattern, len(result), tt.want, result)
			}
		})
	}
}

// BenchmarkMatchTopic_Exact benchmarks exact match (most common case).
func BenchmarkMatchTopic_Exact(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		MatchTopic("tasks.created", "tasks.created")
	}
}

// BenchmarkMatchTopic_SingleWildcard benchmarks single-level wildcard.
func BenchmarkMatchTopic_SingleWildcard(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		MatchTopic("tasks.*", "tasks.created")
	}
}

// BenchmarkMatchTopic_MultiWildcard benchmarks multi-level wildcard.
func BenchmarkMatchTopic_MultiWildcard(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		MatchTopic("ebpf.#", "ebpf.packet.events.detailed")
	}
}

// BenchmarkMatchTopic_Global benchmarks global wildcard.
func BenchmarkMatchTopic_Global(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		MatchTopic("*", "ebpf.packet.events.detailed.trace")
	}
}

// BenchmarkMatchTopic_NoMatch benchmarks a non-matching pattern.
func BenchmarkMatchTopic_NoMatch(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		MatchTopic("tasks.created", "events.processed")
	}
}

// BenchmarkMatchTopic_DeepSegments benchmarks deeply nested topics.
func BenchmarkMatchTopic_DeepSegments(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		MatchTopic("a.b.*.d.*.f", "a.b.c.d.e.f")
	}
}
