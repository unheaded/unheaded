// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package topicpattern

import "testing"

func TestCovers(t *testing.T) {
	cases := []struct {
		allow, pattern string
		want           bool
	}{
		// one topic
		{"tasks.created", "tasks.created", true},
		{"tasks.created", "tasks.updated", false},
		{"tasks.created", "tasks.*", false},
		{"tasks.created", "tasks.#", false},
		{"tasks.created", "tasks", false},
		{"tasks.created", "tasks.created.x", false},
		// one level
		{"tasks.*", "tasks.created", true},
		{"tasks.*", "tasks.*", true},
		{"tasks.*", "tasks.#", false},
		{"tasks.*", "tasks", false},
		{"tasks.*", "tasks.a.b", false},
		{"tasks.*", "other.created", false},
		{"logs.*.*", "logs.svc.info", true},
		{"logs.*.*", "logs.*.error", true},
		{"logs.*.*", "logs.svc", false},
		{"logs.*.*", "logs.#", false},
		// many levels
		{"tasks.#", "tasks.created", true},
		{"tasks.#", "tasks.a.b.c", true},
		{"tasks.#", "tasks.*", true},
		{"tasks.#", "tasks.#", true},
		{"tasks.#", "tasks.*.#", true},
		{"tasks.#", "tasks", false},
		{"tasks.#", "task.created", false},
		{"system.health.#", "system.health.reports", true},
		{"system.health.#", "system.#", false},
		{"system.health.#", "#", false},
		// everything
		{"#", "tasks.created", true},
		{"#", "#", true},
		{"#", "*", true},
		{"*", "tasks.created", true},
		{"*", "#", true},
		// a "#" not last in the allowance covers nothing (Match never takes it)
		{"a.#.b", "a.x.b", false},
		// invalid or empty strings
		{"", "tasks.created", false},
		{"tasks.*", "", false},
		{"tasks.*", "tasks.cre ated", false},
		{"tasks.*", "../x", false},
	}
	for _, c := range cases {
		if got := Covers(c.allow, c.pattern); got != c.want {
			t.Errorf("Covers(%q, %q) = %v, want %v", c.allow, c.pattern, got, c.want)
		}
	}
}

// Whatever Covers says yes to, Match agrees with on sample topics: a topic
// the pattern matches is matched by the allowance too.
func TestCovers_AgreesWithMatch(t *testing.T) {
	allows := []string{"tasks.created", "tasks.*", "tasks.#", "logs.*.*", "system.health.#", "#", "*"}
	patterns := []string{"tasks.created", "tasks.*", "tasks.#", "tasks.*.#", "logs.svc.info", "logs.*.error", "logs.#", "system.health.reports", "system.#", "#"}
	topics := []string{"tasks.created", "tasks.updated", "tasks.a.b", "tasks", "logs.svc.info", "logs.svc.error", "logs.svc.a.b", "system.health.reports", "system.health.a.b", "system.discovery", "x"}
	for _, a := range allows {
		for _, p := range patterns {
			if !Covers(a, p) {
				continue
			}
			for _, topic := range topics {
				if Match(p, topic) && !Match(a, topic) {
					t.Errorf("Covers(%q, %q) is true, but topic %q matches the pattern and not the allowance", a, p, topic)
				}
			}
		}
	}
}
