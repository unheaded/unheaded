// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.
//
// CTF mode (ADR-063) — DESIGNED, GATED, NOT ACTIVATED. S-LICH-AWAKENS Phase 10.
//
// Akira may "summon the Lich" against a live eligible service for a timed CTF
// round. This is the whole mechanism, built to one-sentence-activation
// readiness — but it is HARD-GATED OFF behind the ADR-063 activation rule:
// LICH-001, LICH-002 and one of LICH-012/013 clean for 7 consecutive days. The
// gate reads that clean-soak condition, NEVER a boolean flag, so no flag flip
// (and no prompt) can turn it on. Today the rule is unmet (LICH-012 is OPEN),
// so every Summon returns ErrCTFNotActivated and nothing touches a live service.
//
// When the day comes, activation is a single commit that supplies a real
// clean-soak source to CTFActivation; the guard below then opens on its own.

package main

import (
	"errors"
	"time"
)

// ErrCTFNotActivated is returned by every summon path while the ADR-063
// activation rule is unmet. It is the sprint's only reachable state.
var ErrCTFNotActivated = errors.New("akira: CTF mode not activated (ADR-063 rule unmet: LICH-001+LICH-002+one of LICH-012/013 must be clean 7 consecutive days)")

// ctfActivationSoak is the ADR-063 clean-soak requirement.
const ctfActivationSoak = 7 * 24 * time.Hour

// CTFActivation reads campaign clean-soak state. CleanSince[id] is the start of
// a campaign's current uninterrupted clean streak; a zero/absent value means
// the campaign is not currently clean. This is DATA, never a flag — a future
// activation commit wires a real source (the ADR-098 evidence store) into it.
type CTFActivation struct {
	CleanSince map[string]time.Time
}

// Activated reports whether the ADR-063 rule is met at `now`.
func (a CTFActivation) Activated(now time.Time) bool {
	soaked := func(id string) bool {
		t, ok := a.CleanSince[id]
		return ok && !t.IsZero() && now.Sub(t) >= ctfActivationSoak
	}
	return soaked("LICH-001") && soaked("LICH-002") && (soaked("LICH-012") || soaked("LICH-013"))
}

// RoundStatus is the live state a CTF round reports to the HARD-HALT watcher.
type RoundStatus struct {
	HealthNon200For    time.Duration // how long the target /health has been non-200
	LateralMovement    bool          // a non-eligible service was implicated
	AbortTopicFired    bool          // someone published lich.ctf.abort
	AkiraHeartbeatLost bool          // the summoner lost its heartbeat
	HealthThreshold    time.Duration // configured non-200 budget (e.g. 60s)
}

// ShouldHardHalt applies the ADR-063 Marshal HARD-HALT conditions. Any one trips
// the round: it reuses the Phase 2 kill switch to SIGTERM the round and journal
// ABORTED. Pure, so it is unit-testable without a live round.
func (s RoundStatus) ShouldHardHalt() (bool, string) {
	switch {
	case s.HealthThreshold > 0 && s.HealthNon200For >= s.HealthThreshold:
		return true, "target /health non-200 past the budget"
	case s.LateralMovement:
		return true, "lateral movement: a non-eligible service was implicated"
	case s.AbortTopicFired:
		return true, "lich.ctf.abort published"
	case s.AkiraHeartbeatLost:
		return true, "Akira heartbeat lost"
	default:
		return false, ""
	}
}

// CTFRound is what a summon would produce once activated (design-only shape).
type CTFRound struct {
	Target          string
	DurationMinutes int
	StartedAt       time.Time
}

// LichSummoner holds the gate and the eligibility list. It POSTs rounds to the
// confined Lich MCP surface (never a raw fuzzer against a live service without
// the gate) — but only once activated.
type LichSummoner struct {
	activation CTFActivation
	// eligible is the set of services allowed as targets AND marked
	// eligible:true in configs/lich-ctf-eligible.yaml. Empty today.
	eligible map[string]bool
}

// NewLichSummoner builds a summoner. Even with a non-empty eligibility set, the
// activation gate keeps it inert until the 7-day rule is met.
func NewLichSummoner(activation CTFActivation, eligible map[string]bool) *LichSummoner {
	return &LichSummoner{activation: activation, eligible: eligible}
}

// Summon attempts to open a CTF round against target at `now`. While the ADR-063
// rule is unmet it returns ErrCTFNotActivated and does nothing — the only
// reachable behaviour this sprint. The eligibility check is a second gate for
// when activation lands.
func (s *LichSummoner) Summon(now time.Time, target string, durationMinutes int) (*CTFRound, error) {
	// GATE 1 (hard): the activation rule, read from clean-soak data, never a flag.
	if !s.activation.Activated(now) {
		return nil, ErrCTFNotActivated
	}
	// GATE 2: the target must be explicitly eligible in the config.
	if !s.eligible[target] {
		return nil, errors.New("akira: target not eligible for CTF mode")
	}
	return &CTFRound{Target: target, DurationMinutes: durationMinutes, StartedAt: now}, nil
}
