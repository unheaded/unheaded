// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package claudebridge

import (
	"context"
	"errors"
	"testing"
)

// Both routers answer the same question through one interface (ADR-106
// Q7, S-CHAMPION-HARDENING Step 112): AutoRouter by weight class,
// ThresholdRouter by the eval's per-family route.
func TestRouterInterface_BothSatisfy(t *testing.T) {
	routers := map[string]Router{
		"auto":      AutoRouter{},
		"threshold": &ThresholdRouter{Config: testRouting()},
	}
	for name, r := range routers {
		if _, err := r.Route(context.Background(), Task{Prompt: "triage this crash", Hints: map[string]string{"family": "hard"}}); err != nil {
			t.Errorf("%s: Route: %v", name, err)
		}
	}
}

func TestThresholdRouter_RouteByFamily(t *testing.T) {
	r := &ThresholdRouter{Config: testRouting()}
	cases := []struct {
		family string
		want   Weight
		err    error
	}{
		{"textbook-local", WeightMinor, nil}, // local_only
		{"repo-task", WeightMinor, nil},      // local_first: the first hop is local
		{"hard", WeightHeavy, nil},           // claude_direct
		{"", "", ErrNoFamilyRoute},           // no family hint
		{"never-measured", "", ErrNoFamilyRoute},
	}
	for _, c := range cases {
		var hints map[string]string
		if c.family != "" {
			hints = map[string]string{"family": c.family}
		}
		got, err := r.Route(context.Background(), Task{Prompt: "anything", Hints: hints})
		if !errors.Is(err, c.err) || got != c.want {
			t.Errorf("family %q: got (%q, %v), want (%q, %v)", c.family, got, err, c.want, c.err)
		}
	}
	var nilCfg ThresholdRouter
	if _, err := nilCfg.Route(context.Background(), Task{Hints: map[string]string{"family": "hard"}}); !errors.Is(err, ErrNoFamilyRoute) {
		t.Errorf("nil config: err = %v, want ErrNoFamilyRoute", err)
	}
}

// A task the AutoRouter cannot classify (no weight hint, no heavy or minor
// signal in the prompt) defers to the fallback; with no fallback, or a
// fallback that has no route for the family, it stays minor (local).
func TestAutoRouter_FallbackToThreshold(t *testing.T) {
	ctx := context.Background()
	th := &ThresholdRouter{Config: testRouting()}
	unclassifiable := Task{Prompt: "please proceed", Hints: map[string]string{"family": "hard"}}

	if w, err := (AutoRouter{Fallback: th}).Route(ctx, unclassifiable); err != nil || w != WeightHeavy {
		t.Fatalf("with fallback: (%q, %v), want heavy (family hard is claude_direct)", w, err)
	}
	if w, err := (AutoRouter{}).Route(ctx, unclassifiable); err != nil || w != WeightMinor {
		t.Fatalf("without fallback: (%q, %v), want minor", w, err)
	}
	unknown := Task{Prompt: "please proceed", Hints: map[string]string{"family": "never-measured"}}
	if w, err := (AutoRouter{Fallback: th}).Route(ctx, unknown); err != nil || w != WeightMinor {
		t.Fatalf("unknown family: (%q, %v), want minor (the fallback has no route)", w, err)
	}

	// The AutoRouter's own signals win: the fallback is never consulted.
	spy := &spyRouter{w: WeightHeavy}
	for _, task := range []Task{
		{Prompt: "generate 50 mutated seeds", Hints: map[string]string{"family": "hard"}},
		{Prompt: "please proceed", Hints: map[string]string{"weight": "minor", "family": "hard"}},
	} {
		if w, err := (AutoRouter{Fallback: spy}).Route(ctx, task); err != nil || w != WeightMinor {
			t.Fatalf("%q: (%q, %v), want minor", task.Prompt, w, err)
		}
	}
	if spy.calls != 0 {
		t.Fatalf("fallback consulted %d time(s) for classifiable tasks", spy.calls)
	}

	// Any other fallback error is the caller's to see.
	boom := errors.New("boom")
	if _, err := (AutoRouter{Fallback: &spyRouter{err: boom}}).Route(ctx, unclassifiable); !errors.Is(err, boom) {
		t.Fatalf("fallback error = %v, want boom", err)
	}
}

type spyRouter struct {
	w     Weight
	err   error
	calls int
}

func (s *spyRouter) Route(context.Context, Task) (Weight, error) {
	s.calls++
	return s.w, s.err
}
