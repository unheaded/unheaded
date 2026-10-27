// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package claudebridge

import (
	"context"
	"errors"
	"regexp"
	"strings"
)

// AutoRouter is the ADR-053 weight classifier for the Lich drivers
// (S-LICH-AWAKENS Phase 4, Step 109). It decides whether a task is MINOR
// churn — seed and mutation generation, harness-variant drafting, status and
// lookup — which the free local model handles, or HEAVY work — crash triage,
// finding synthesis, novel-harness authoring, multi-crash correlation — which
// goes to Claude.
//
// It never sends anything itself; it only classifies. The dollar cap, the
// Champion gate, and the external-data (privacy) filter on the heavy path are
// enforced by the Bridge the driver sends through — classification can never
// bypass them.
//
// A Hint wins over the prompt text (a workflow template or Kanban label is more
// reliable than guessing). When nothing matches it asks Fallback, if one is
// set; otherwise, or when Fallback has no route for the task
// (ErrNoFamilyRoute), it returns WeightMinor: the cheaper, local, on-host
// default, so an ambiguous task never spends dollars or leaves the host
// unless the operator wired a fallback whose measurement says it should.
//
// The two routers stay separate (ADR-106 Q7, S-CHAMPION-HARDENING Step 112):
// AutoRouter answers "how heavy is this work" for the Lich drivers;
// ThresholdRouter answers "has the eval shown the local model can do this
// family". Folding them would couple the Lich's weight classes to the
// eval's per-family bars. The one seam between them is Fallback: set it to
// a *ThresholdRouter and an unclassifiable task carrying a "family" hint
// takes that family's measured route.
type AutoRouter struct {
	// Fallback decides tasks AutoRouter cannot classify. nil: minor.
	Fallback Router
}

// Route implements Router.
func (a AutoRouter) Route(ctx context.Context, t Task) (Weight, error) {
	if h, ok := t.Hints["weight"]; ok {
		switch strings.ToLower(strings.TrimSpace(h)) {
		case string(WeightHeavy):
			return WeightHeavy, nil
		case string(WeightMinor):
			return WeightMinor, nil
		}
	}
	if w, ok := classifyWeight(t.Prompt); ok {
		return w, nil
	}
	if a.Fallback == nil {
		return WeightMinor, nil
	}
	w, err := a.Fallback.Route(ctx, t)
	if errors.Is(err, ErrNoFamilyRoute) {
		return WeightMinor, nil
	}
	if err != nil {
		return "", err
	}
	return w, nil
}

// heavyWeightRE matches work that benefits from Claude: triage, synthesis,
// authoring a new harness, correlating across crashes, root-cause/exploit
// reasoning, drafting a finding or repro.
var heavyWeightRE = regexp.MustCompile(`(?i)\b(triage|triaging|synthesi[sz]e|synthesis|correlat\w*|root[\s-]?cause|exploitab\w*|exploit\b|author\w*\s+(a\s+)?(new\s+)?harness|novel\s+harness|write[\s-]?up|draft\s+(a\s+)?(finding|repro|report)|finding\s+synthesis|multi[\s-]?crash|classif\w*\s+(the\s+)?crash)`)

// minorWeightRE matches high-volume churn the local model handles: seed and
// mutation generation, harness-input variants, and plain status/lookup.
var minorWeightRE = regexp.MustCompile(`(?i)\b(seed\w*|mutat\w*|fuzz\s+input|input\s+variant|variant\w*|generate\b|corpus|status\b|list\b|look\s?up|show\b)`)

// ClassifyWeight returns the weight a prompt's text implies. A heavy signal
// wins over a minor one (synthesising findings over a corpus is heavy even
// though it mentions "corpus"); with no signal at all it is minor.
func ClassifyWeight(prompt string) Weight {
	if w, ok := classifyWeight(prompt); ok {
		return w
	}
	return WeightMinor
}

// classifyWeight reports the weight a prompt's text implies and whether it
// implied one at all.
func classifyWeight(prompt string) (Weight, bool) {
	if heavyWeightRE.MatchString(prompt) {
		return WeightHeavy, true
	}
	if minorWeightRE.MatchString(prompt) {
		return WeightMinor, true
	}
	return "", false
}

// ModelForWeight picks the model a weight routes to: Haiku (volume) for MINOR
// drafting, Opus (triage/synthesis) for HEAVY. An empty id for either falls
// back to the other so a half-configured driver still runs.
func ModelForWeight(w Weight, haiku, opus string) string {
	switch w {
	case WeightHeavy:
		if opus != "" {
			return opus
		}
		return haiku
	default:
		if haiku != "" {
			return haiku
		}
		return opus
	}
}

// compile-time assertion that AutoRouter is a Router.
var _ Router = AutoRouter{}
