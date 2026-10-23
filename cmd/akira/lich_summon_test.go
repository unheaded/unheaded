// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"errors"
	"testing"
	"time"
)

var ctfNow = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

// TestCTFSummonInertWhenUnactivated: the only reachable state this sprint. With
// no clean-soak data (the real world today — LICH-012 is OPEN), every summon
// returns ErrCTFNotActivated and produces no round, for every eligibility set.
func TestCTFSummonInertWhenUnactivated(t *testing.T) {
	cases := []CTFActivation{
		{}, // nothing clean
		{CleanSince: map[string]time.Time{"LICH-001": ctfNow.Add(-8 * 24 * time.Hour)}},                                                              // only one soaked
		{CleanSince: map[string]time.Time{"LICH-001": ctfNow.Add(-8 * 24 * time.Hour), "LICH-002": ctfNow.Add(-8 * 24 * time.Hour)}},                 // missing a 012/013
		{CleanSince: map[string]time.Time{"LICH-001": ctfNow, "LICH-002": ctfNow, "LICH-012": ctfNow}},                                               // soak too short (0 days)
		{CleanSince: map[string]time.Time{"LICH-001": ctfNow.Add(-8 * 24 * time.Hour), "LICH-002": ctfNow.Add(-8 * 24 * time.Hour), "LICH-012": {}}}, // 012 clean streak zeroed
	}
	// Even with a fully-eligible config, the activation gate keeps it inert.
	eligible := map[string]bool{"wotan": true, "kanban": true}
	for i, a := range cases {
		s := NewLichSummoner(a, eligible)
		round, err := s.Summon(ctfNow, "wotan", 30)
		if !errors.Is(err, ErrCTFNotActivated) {
			t.Fatalf("case %d: err = %v, want ErrCTFNotActivated", i, err)
		}
		if round != nil {
			t.Fatalf("case %d: a round was created while unactivated: %+v", i, round)
		}
	}
}

// TestCTFActivationRuleBoundary proves the gate opens ONLY when all three
// conditions soak past 7 days — never on a flag. (Exercised with synthetic
// data; the real data source is the future activation commit's job.)
func TestCTFActivationRuleBoundary(t *testing.T) {
	old := ctfNow.Add(-8 * 24 * time.Hour)
	full := CTFActivation{CleanSince: map[string]time.Time{"LICH-001": old, "LICH-002": old, "LICH-013": old}}
	if !full.Activated(ctfNow) {
		t.Fatalf("rule should be met with all three soaked > 7 days")
	}
	// Exactly 7 days is the boundary (>=).
	at7 := ctfNow.Add(-ctfActivationSoak)
	full7 := CTFActivation{CleanSince: map[string]time.Time{"LICH-001": at7, "LICH-002": at7, "LICH-012": at7}}
	if !full7.Activated(ctfNow) {
		t.Fatalf("rule should be met at exactly 7 days")
	}
	// One day short fails.
	short := ctfNow.Add(-6 * 24 * time.Hour)
	full6 := CTFActivation{CleanSince: map[string]time.Time{"LICH-001": short, "LICH-002": short, "LICH-012": short}}
	if full6.Activated(ctfNow) {
		t.Fatalf("rule must NOT be met at 6 days")
	}
}

// TestCTFHardHalt unit-tests the Marshal HARD-HALT conditions against a fake round.
func TestCTFHardHalt(t *testing.T) {
	cases := []struct {
		name string
		st   RoundStatus
		halt bool
	}{
		{"healthy", RoundStatus{HealthThreshold: 60 * time.Second, HealthNon200For: 5 * time.Second}, false},
		{"health past budget", RoundStatus{HealthThreshold: 60 * time.Second, HealthNon200For: 61 * time.Second}, true},
		{"lateral movement", RoundStatus{LateralMovement: true}, true},
		{"abort topic", RoundStatus{AbortTopicFired: true}, true},
		{"heartbeat lost", RoundStatus{AkiraHeartbeatLost: true}, true},
	}
	for _, tc := range cases {
		got, reason := tc.st.ShouldHardHalt()
		if got != tc.halt {
			t.Errorf("%s: ShouldHardHalt()=%v want %v (reason %q)", tc.name, got, tc.halt, reason)
		}
		if got && reason == "" {
			t.Errorf("%s: halt with no reason", tc.name)
		}
	}
}
