// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package lichtools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLifecycleTransitions(t *testing.T) {
	legal := []struct{ from, to CampaignState }{
		{StateOpened, StateRunning},
		{StateRunning, StateTriaging},
		{StateRunning, StateCleared},
		{StateRunning, StateBlocked},
		{StateTriaging, StateBlocked},
		{StateTriaging, StateCleared},
		{StateBlocked, StateRunning}, // re-run trigger
		{StateCleared, StateRunning}, // weekly replay re-arms
	}
	for _, tc := range legal {
		if _, err := tc.from.Transition(tc.to); err != nil {
			t.Errorf("expected legal %s->%s: %v", tc.from, tc.to, err)
		}
	}
	illegal := []struct{ from, to CampaignState }{
		{StateOpened, StateCleared},  // cannot clear without running
		{StateOpened, StateTriaging}, // must run first
		{StateCleared, StateTriaging},
	}
	for _, tc := range illegal {
		if _, err := tc.from.Transition(tc.to); err == nil {
			t.Errorf("expected illegal %s->%s to error", tc.from, tc.to)
		}
	}
}

func TestNextStateDerivation(t *testing.T) {
	cases := []struct {
		name string
		cur  CampaignState
		out  RunOutcome
		want CampaignState
	}{
		{"clean budget-met run clears", StateRunning, RunOutcome{BudgetMet: true}, StateCleared},
		{"a crash moves to triaging", StateRunning, RunOutcome{Crashes: 1}, StateTriaging},
		{"a divergence moves to triaging", StateRunning, RunOutcome{Divergences: 1}, StateTriaging},
		{"a HIGH finding blocks", StateRunning, RunOutcome{Crashes: 1, HighestSeverity: "high"}, StateBlocked},
		{"a CRITICAL finding blocks", StateTriaging, RunOutcome{HighestSeverity: "critical"}, StateBlocked},
		{"a LOW finding does not block", StateRunning, RunOutcome{Crashes: 1, HighestSeverity: "low"}, StateTriaging},
		{"clean-but-budget-not-met stays running", StateRunning, RunOutcome{}, StateRunning},
	}
	for _, tc := range cases {
		if got := NextState(tc.cur, tc.out); got != tc.want {
			t.Errorf("%s: NextState(%s,%+v)=%s want %s", tc.name, tc.cur, tc.out, got, tc.want)
		}
	}
}

func TestWriteRunSummary(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "20261023_000000")
	err := WriteRunSummary(dir, RunSummary{
		CampaignID: "LICH-015", HeadSHA: "deadbeef", Duration: 5 * time.Minute,
		Execs: 50000, Crashes: 0, Divergences: 0, Verdict: "CLEAN",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "crashes")); err != nil {
		t.Errorf("crashes/ dir not created: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "summary.md")) // #nosec G304 -- test temp
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{"LICH-015", "deadbeef", "5m0s", "50000", "**CLEAN**"} {
		if !strings.Contains(s, want) {
			t.Errorf("summary.md missing %q:\n%s", want, s)
		}
	}
}

func TestCampaignSeedSlug(t *testing.T) {
	cases := map[string]string{
		"LICH-015":  "lich_015",
		"lich-016":  "lich_016",
		"LICH-015a": "lich_015a",
		"":          "",
		"../etc":    "etc", // dots/slashes dropped — cannot escape
	}
	for in, want := range cases {
		if got := campaignSeedSlug(in); got != want {
			t.Errorf("campaignSeedSlug(%q)=%q want %q", in, got, want)
		}
	}
}
