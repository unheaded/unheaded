// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package lichtools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CampaignState is the ADR-062 lifecycle of a Lich campaign: a campaign is
// opened when its code lands and closed when its exit criteria are met. A run
// moves it through triaging to either cleared (zero net-new for the budget) or
// blocked (a HIGH+ finding).
type CampaignState string

const (
	// StateOpened — scaffolded, not yet run.
	StateOpened CampaignState = "opened"
	// StateRunning — a harness is executing in a sandbox.
	StateRunning CampaignState = "running"
	// StateTriaging — crashes/divergences found, under triage.
	StateTriaging CampaignState = "triaging"
	// StateCleared — ran clean for the configured budget; exit criteria met.
	StateCleared CampaignState = "cleared"
	// StateBlocked — a HIGH+ finding holds the campaign (and gates a release).
	StateBlocked CampaignState = "blocked"
)

// ErrIllegalTransition is returned by Transition for a move not on the graph.
type illegalTransition struct {
	from, to CampaignState
}

func (e illegalTransition) Error() string {
	return fmt.Sprintf("lichtools: illegal campaign transition %s -> %s", e.from, e.to)
}

// lifecycleEdges is the legal transition graph. A terminal-ish state can still
// re-open (a re-run trigger) or move to blocked (a late finding), matching
// ADR-062's weekly replay.
var lifecycleEdges = map[CampaignState]map[CampaignState]bool{
	StateOpened:   {StateRunning: true},
	StateRunning:  {StateTriaging: true, StateCleared: true, StateBlocked: true},
	StateTriaging: {StateBlocked: true, StateCleared: true, StateRunning: true},
	StateCleared:  {StateRunning: true, StateBlocked: true},
	StateBlocked:  {StateRunning: true, StateTriaging: true, StateCleared: true},
}

// CanTransition reports whether from->to is a legal lifecycle move.
func (s CampaignState) CanTransition(to CampaignState) bool {
	return lifecycleEdges[s][to]
}

// Transition returns the new state or an error if the move is illegal.
func (s CampaignState) Transition(to CampaignState) (CampaignState, error) {
	if !s.CanTransition(to) {
		return s, illegalTransition{from: s, to: to}
	}
	return to, nil
}

// severityRank orders finding severities; HIGH and above block a campaign.
var severityRank = map[string]int{"info": 0, "low": 1, "medium": 2, "high": 3, "critical": 4}

// RunOutcome summarises one campaign run for the state machine.
type RunOutcome struct {
	Crashes         int
	Divergences     int
	HighestSeverity string // "", info, low, medium, high, critical
	// BudgetMet is true when the run completed the configured clean budget
	// (duration/execs) with zero net-new crashes or divergences.
	BudgetMet bool
}

// NextState derives the campaign state after a run from the current state.
// A HIGH+ finding blocks; any crash/divergence moves to triaging; a clean
// budget-met run clears; otherwise it stays running.
func NextState(cur CampaignState, o RunOutcome) CampaignState {
	if severityRank[o.HighestSeverity] >= severityRank["high"] {
		if cur.CanTransition(StateBlocked) {
			return StateBlocked
		}
		return cur
	}
	if o.Crashes > 0 || o.Divergences > 0 {
		if cur.CanTransition(StateTriaging) {
			return StateTriaging
		}
		return cur // e.g. opened: caller runs open->running before a run
	}
	if o.BudgetMet && cur.CanTransition(StateCleared) {
		return StateCleared
	}
	return cur
}

// RunSummary is the standardised ADR-062 results doc for one run.
type RunSummary struct {
	CampaignID  string
	HeadSHA     string
	Duration    time.Duration
	Execs       int64
	Crashes     int
	Divergences int
	Verdict     string // CLEAN | TRIAGE | BLOCKED
}

// WriteRunSummary writes results/<ts>/summary.md and ensures a crashes/ dir,
// the standardised shape every campaign run produces (ADR-062 Step 200). The
// directory must already be inside a results tree the caller controls; this
// writes only non-secret run metadata.
func WriteRunSummary(runDir string, s RunSummary) error {
	if err := os.MkdirAll(filepath.Join(runDir, "crashes"), 0o750); err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s run summary\n\n", s.CampaignID)
	fmt.Fprintf(&b, "- campaign: %s\n", s.CampaignID)
	fmt.Fprintf(&b, "- HEAD: %s\n", s.HeadSHA)
	fmt.Fprintf(&b, "- duration: %s\n", s.Duration)
	fmt.Fprintf(&b, "- execs: %d\n", s.Execs)
	fmt.Fprintf(&b, "- crashes: %d\n", s.Crashes)
	fmt.Fprintf(&b, "- divergences: %d\n", s.Divergences)
	fmt.Fprintf(&b, "- verdict: **%s**\n", s.Verdict)
	// #nosec G306 -- non-secret run metadata in the results tree
	return os.WriteFile(filepath.Join(runDir, "summary.md"), []byte(b.String()), 0o644)
}
