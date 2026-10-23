// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package claudebridge

import (
	"context"
	"testing"
)

func TestAutoRouter_Classification(t *testing.T) {
	cases := []struct {
		name   string
		prompt string
		want   Weight
	}{
		{"seed generation", "generate 50 mutated MBC seeds for LICH-004", WeightMinor},
		{"mutation batch", "mutate these monad-wire inputs with boundary values", WeightMinor},
		{"status lookup", "list the open campaigns and show LICH-012 status", WeightMinor},
		{"harness variant", "generate harness input variants for the parser", WeightMinor},
		{"triage", "triage this crash and draft a repro", WeightHeavy},
		{"synthesis", "synthesize a finding from these three crashes", WeightHeavy},
		{"correlation", "correlate the crashes across runs and write up root cause", WeightHeavy},
		{"novel harness", "author a new harness for the WAL integrity path", WeightHeavy},
		{"empty/uncertain", "please proceed", WeightMinor},
		{"heavy wins over corpus", "synthesize findings over the seed corpus", WeightHeavy},
	}
	r := AutoRouter{}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := r.Route(context.Background(), Task{Prompt: c.prompt})
			if err != nil {
				t.Fatalf("Route: %v", err)
			}
			if got != c.want {
				t.Fatalf("Route(%q) = %q, want %q", c.prompt, got, c.want)
			}
		})
	}
}

func TestAutoRouter_HintWinsOverPrompt(t *testing.T) {
	r := AutoRouter{}
	// A prompt that reads minor, but a hint forces heavy.
	got, err := r.Route(context.Background(), Task{
		Prompt: "generate seeds",
		Hints:  map[string]string{"weight": "heavy"},
	})
	if err != nil || got != WeightHeavy {
		t.Fatalf("hint-forced route = (%q, %v), want heavy", got, err)
	}
	// And the reverse.
	got, err = r.Route(context.Background(), Task{
		Prompt: "triage this crash",
		Hints:  map[string]string{"weight": "minor"},
	})
	if err != nil || got != WeightMinor {
		t.Fatalf("hint-forced route = (%q, %v), want minor", got, err)
	}
	// An unknown hint value falls back to prompt heuristics (heavy here).
	got, _ = r.Route(context.Background(), Task{Prompt: "triage this crash", Hints: map[string]string{"weight": "garbage"}})
	if got != WeightHeavy {
		t.Fatalf("unknown hint should fall back to prompt heuristics, got %q", got)
	}
}

func TestModelForWeight(t *testing.T) {
	const haiku, opus = "claude-haiku-5-5", "claude-opus-5-5"
	if m := ModelForWeight(WeightMinor, haiku, opus); m != haiku {
		t.Fatalf("minor -> %q, want haiku", m)
	}
	if m := ModelForWeight(WeightHeavy, haiku, opus); m != opus {
		t.Fatalf("heavy -> %q, want opus", m)
	}
	// Half-configured: heavy with no opus falls back to haiku, not "".
	if m := ModelForWeight(WeightHeavy, haiku, ""); m != haiku {
		t.Fatalf("heavy with no opus -> %q, want haiku fallback", m)
	}
}
