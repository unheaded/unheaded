// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package health

import (
	"fmt"
	"testing"
	"time"
)

// The bands are CLAUDE.md's (Cross-Service Health Monitoring).
func TestSeverityFor(t *testing.T) {
	for _, tc := range []struct {
		rate float64
		want Severity
	}{
		{0, SeverityOK}, {0.1249, SeverityOK},
		{0.125, SeverityWarn}, {0.3749, SeverityWarn},
		{0.375, SeverityError}, {0.6249, SeverityError},
		{0.625, SeverityCritical}, {0.8749, SeverityCritical},
		{0.875, SeverityPanic}, {1, SeverityPanic},
	} {
		if got := SeverityFor(tc.rate); got != tc.want {
			t.Errorf("SeverityFor(%v) = %s, want %s", tc.rate, got, tc.want)
		}
	}
}

func report(svc, reporter string, healthy bool, at time.Time) HealthReport {
	return HealthReport{Service: svc, Reporter: reporter, Healthy: healthy, Timestamp: at}
}

// Consensus is the share of distinct reporters whose latest fresh report
// says failing, not the share of one reporter's recent checks (which is what
// Akira computed and called TotalReporters).
func TestBallot_TallyIsAShareOfReporters(t *testing.T) {
	now := time.Now()
	b := NewBallot()
	// Three reporters on "wotan": two see it failing.
	b.Record(report("wotan", "west", false, now))
	b.Record(report("wotan", "east", false, now))
	b.Record(report("wotan", "north", true, now))
	// One reporter repeating itself does not become three votes.
	for i := 0; i < 5; i++ {
		b.Record(report("kanban", "west", false, now.Add(-time.Duration(i)*time.Second)))
	}
	b.Record(report("kanban", "east", true, now))

	got := map[string]Verdict{}
	for _, v := range b.Tally(now) {
		got[v.Service] = v
	}
	w := got["wotan"]
	if w.Reporters != 3 || w.Failing != 2 || w.Severity != SeverityCritical || !w.Remediate {
		t.Errorf("wotan = %+v, want 2 of 3 failing, CRITICAL, remediate (2/3 >= threshold)", w)
	}
	if fmt.Sprint(w.FailingReporters) != "[east west]" {
		t.Errorf("wotan failing reporters = %v, want [east west]", w.FailingReporters)
	}
	k := got["kanban"]
	if k.Reporters != 2 || k.Failing != 1 || k.Severity != SeverityError || k.Remediate {
		t.Errorf("kanban = %+v, want 1 of 2 failing, ERROR, no remediation", k)
	}
}

// A reporter's latest report is its vote; an older one arriving late does
// not overwrite it, and a vote expires after ReportFreshness.
func TestBallot_LatestFreshReportCounts(t *testing.T) {
	now := time.Now()
	b := NewBallot()
	b.Record(report("sophia", "west", true, now))
	b.Record(report("sophia", "west", false, now.Add(-time.Minute))) // late, older
	b.Record(report("sophia", "east", false, now.Add(-ReportFreshness-time.Second)))
	vs := b.Tally(now)
	if len(vs) != 1 || vs[0].Reporters != 1 || vs[0].Failing != 0 || vs[0].Severity != SeverityOK {
		t.Fatalf("sophia = %+v, want 1 fresh healthy reporter", vs)
	}
	// Once every vote is stale the service has no verdict at all.
	if vs := b.Tally(now.Add(ReportFreshness + time.Second)); len(vs) != 0 {
		t.Errorf("all stale: %+v, want no verdicts", vs)
	}
	// Reports without a service or reporter are not votes.
	b.Record(report("y", "west", true, now))
	b.Record(report("", "west", false, now))
	b.Record(report("x", "", false, now))
	if vs := b.Tally(now); len(vs) != 1 || vs[0].Service != "y" {
		t.Errorf("anonymous reports counted: %+v", vs)
	}
}
