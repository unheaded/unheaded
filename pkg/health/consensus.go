// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package health

import (
	"sort"
	"sync"
	"time"
)

// Severity is the Kingdom's consensus severity for a service: which band the
// share of reporters seeing it fail falls in (CLAUDE.md, Cross-Service Health
// Monitoring).
type Severity string

// The five bands.
const (
	SeverityOK       Severity = "OK"       // 0 - 12.49 %
	SeverityWarn     Severity = "WARN"     // 12.50 - 37.49 %
	SeverityError    Severity = "ERROR"    // 37.50 - 62.49 %
	SeverityCritical Severity = "CRITICAL" // 62.50 - 87.49 %
	SeverityPanic    Severity = "PANIC"    // 87.50 - 100 %
)

// SeverityFor maps a failure rate (0..1) to its band.
func SeverityFor(rate float64) Severity {
	switch {
	case rate < 0.125:
		return SeverityOK
	case rate < 0.375:
		return SeverityWarn
	case rate < 0.625:
		return SeverityError
	case rate < 0.875:
		return SeverityCritical
	default:
		return SeverityPanic
	}
}

// ReportFreshness is how long a reporter's latest report counts as its vote:
// three sweeps. A reporter that stops reporting stops voting.
const ReportFreshness = 3 * HealthCheckInterval

// Verdict is the consensus on one service.
type Verdict struct {
	Service     string   `json:"service"`
	Reporters   int      `json:"reporters"` // distinct reporters with a fresh report
	Failing     int      `json:"failing"`
	FailureRate float64  `json:"failure_rate"`
	Severity    Severity `json:"severity"`
	// Remediate: FailureRate has reached ConsensusThreshold (two-thirds,
	// ADR-029), inside the CRITICAL band.
	Remediate        bool     `json:"remediate"`
	FailingReporters []string `json:"failing_reporters,omitempty"`
}

// Ballot holds each reporter's latest report per service. Consensus is the
// share of distinct reporters whose vote is "failing"; one reporter's
// repeated checks are one vote.
type Ballot struct {
	mu     sync.Mutex
	latest map[string]map[string]vote // service -> reporter -> vote
	now    func() time.Time           // arrival clock; time.Now outside tests
}

// vote is a report and the time it counts from: its own timestamp, or its
// arrival if that is earlier. A reporter's clock can run ahead of ours, and a
// report can carry any timestamp at all. Judged by its timestamp alone, a
// report dated an hour ahead stayed a fresh vote for an hour after its
// reporter went quiet, and shut out that reporter's real reports meanwhile.
type vote struct {
	HealthReport
	at time.Time
}

// NewBallot returns an empty ballot.
func NewBallot() *Ballot {
	return &Ballot{latest: map[string]map[string]vote{}, now: time.Now}
}

// Record casts a report as its reporter's vote on the service, unless the
// reporter already voted with a newer report.
func (b *Ballot) Record(r HealthReport) {
	if r.Service == "" || r.Reporter == "" {
		return
	}
	at := r.Timestamp
	if arrived := b.now(); arrived.Before(at) {
		at = arrived
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	votes := b.latest[r.Service]
	if votes == nil {
		votes = map[string]vote{}
		b.latest[r.Service] = votes
	}
	if prev, ok := votes[r.Reporter]; ok && prev.at.After(at) {
		return
	}
	votes[r.Reporter] = vote{HealthReport: r, at: at}
}

// Tally returns a verdict per service with at least one fresh vote, sorted
// by service. Stale votes are dropped.
func (b *Ballot) Tally(now time.Time) []Verdict {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []Verdict
	for svc, votes := range b.latest {
		v := Verdict{Service: svc}
		for reporter, r := range votes {
			if now.Sub(r.at) > ReportFreshness {
				delete(votes, reporter)
				continue
			}
			v.Reporters++
			if !r.Healthy {
				v.Failing++
				v.FailingReporters = append(v.FailingReporters, reporter)
			}
		}
		if v.Reporters == 0 {
			delete(b.latest, svc)
			continue
		}
		sort.Strings(v.FailingReporters)
		v.FailureRate = float64(v.Failing) / float64(v.Reporters)
		v.Severity = SeverityFor(v.FailureRate)
		v.Remediate = v.FailureRate >= ConsensusThreshold
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Service < out[j].Service })
	return out
}
