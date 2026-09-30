// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package crosswalk

import (
	"testing"
	"time"
)

var now = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

func rec(src Source, v Verdict, age time.Duration) Record {
	return Record{Source: src, Verdict: v, ObservedAt: now.Add(-age)}
}

func TestControlStatus(t *testing.T) {
	a := Source{Kind: KindGitHubJob, Ref: "W/a"}
	b := Source{Kind: KindGateScript, Ref: "scripts/b.sh"}
	ctl := &Control{ID: "UH-TEST-01", FreshnessDays: 7, Evidence: []Source{a, b}}
	day := 24 * time.Hour

	for _, tc := range []struct {
		name string
		recs []Record
		want Status
	}{
		{"no evidence at all", nil, StatusNotAssessed},
		{"one source missing", []Record{rec(a, VerdictPass, day)}, StatusNotAssessed},
		{"all fresh pass", []Record{rec(a, VerdictPass, day), rec(b, VerdictPass, day)}, StatusPass},
		{"one stale", []Record{rec(a, VerdictPass, day), rec(b, VerdictPass, 8*day)}, StatusStale},
		{"exactly at the window is fresh", []Record{rec(a, VerdictPass, 7*day), rec(b, VerdictPass, 0)}, StatusPass},
		{"a fail wins over pass", []Record{rec(a, VerdictFail, day), rec(b, VerdictPass, day)}, StatusFail},
		{"a fail wins over missing", []Record{rec(a, VerdictFail, day)}, StatusFail},
		{"a stale fail is still a fail", []Record{rec(a, VerdictFail, 30*day), rec(b, VerdictPass, day)}, StatusFail},
		{"latest record per source wins", []Record{
			rec(a, VerdictFail, 3*day), rec(a, VerdictPass, day), rec(b, VerdictPass, day)}, StatusPass},
		{"latest fail after an older pass", []Record{
			rec(a, VerdictPass, 3*day), rec(a, VerdictFail, day), rec(b, VerdictPass, day)}, StatusFail},
		{"records for other sources ignored", []Record{
			rec(Source{Kind: KindGitHubJob, Ref: "W/other"}, VerdictFail, day),
			rec(a, VerdictPass, day), rec(b, VerdictPass, day)}, StatusPass},
		{"future-dated record is not evidence", []Record{
			rec(a, VerdictPass, -time.Hour), rec(b, VerdictPass, day)}, StatusNotAssessed},
		{"unknown verdict is not evidence", []Record{
			rec(a, Verdict("maybe"), day), rec(b, VerdictPass, day)}, StatusNotAssessed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ControlStatus(ctl, tc.recs, now); got != tc.want {
				t.Errorf("ControlStatus = %s, want %s", got, tc.want)
			}
		})
	}
}

func catalogForSummary(t *testing.T) *Catalog {
	t.Helper()
	c, err := Load([]byte(`
frameworks:
  - id: fw
    name: FW
    requirements: [{id: R1}, {id: R2}, {id: R3}, {id: R4}, {id: R5}]
`), []byte(`
controls:
  - id: UH-A-01
    title: A
    statement: a
    freshness_days: 7
    evidence: [{kind: gate-script, ref: a.sh}]
    mappings: {fw: [R1, R2]}
  - id: UH-B-01
    title: B
    statement: b
    freshness_days: 7
    evidence: [{kind: gate-script, ref: b.sh}]
    mappings: {fw: [R2, R3]}
  - id: UH-C-01
    title: C
    statement: c
    freshness_days: 7
    evidence: [{kind: gate-script, ref: c.sh}]
    mappings: {fw: [R4]}
`))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestSummarize(t *testing.T) {
	c := catalogForSummary(t)
	src := func(ref string) Source { return Source{Kind: KindGateScript, Ref: ref} }
	// A passes, B fails, C has no evidence.
	recs := []Record{rec(src("a.sh"), VerdictPass, time.Hour), rec(src("b.sh"), VerdictFail, time.Hour)}

	s := Summarize(c, recs, now)
	if len(s.Frameworks) != 1 {
		t.Fatalf("frameworks = %d", len(s.Frameworks))
	}
	fw := s.Frameworks[0]
	want := map[string]ReqStatus{
		"R1": ReqEvidenced,  // only A, which passes
		"R2": ReqFailing,    // A passes but B fails: one blocker is a blocker
		"R3": ReqFailing,    // B
		"R4": ReqIncomplete, // C not assessed: mapped, but no evidence
		"R5": ReqUnmapped,   // nothing maps to it; still in the denominator
	}
	for _, r := range fw.Requirements {
		if r.Status != want[r.ID] {
			t.Errorf("%s = %s, want %s", r.ID, r.Status, want[r.ID])
		}
	}
	if fw.Total != 5 || fw.Counts[ReqEvidenced] != 1 || fw.Counts[ReqFailing] != 2 ||
		fw.Counts[ReqIncomplete] != 1 || fw.Counts[ReqUnmapped] != 1 {
		t.Errorf("counts = %v of %d", fw.Counts, fw.Total)
	}
	var sum int
	for _, n := range fw.Counts {
		sum += n
	}
	if sum != fw.Total {
		t.Errorf("counts sum to %d, total %d: a requirement was dropped or double-counted", sum, fw.Total)
	}
	if got := s.Controls["UH-B-01"]; got != StatusFail {
		t.Errorf("UH-B-01 = %s", got)
	}
}

// Mapping is not satisfaction: two controls mapped to one requirement, one
// passing and one unassessed, must not read as evidenced.
func TestSummarize_PartialEvidenceIsNotEvidenced(t *testing.T) {
	c := catalogForSummary(t)
	recs := []Record{rec(Source{Kind: KindGateScript, Ref: "a.sh"}, VerdictPass, time.Hour)}
	for _, r := range Summarize(c, recs, now).Frameworks[0].Requirements {
		if r.ID == "R2" && r.Status != ReqIncomplete {
			t.Fatalf("R2 (A pass, B unassessed) = %s, want %s", r.Status, ReqIncomplete)
		}
	}
}

// No evidence anywhere: nothing may read as evidenced or failing.
func TestSummarize_NoEvidenceClaimsNothing(t *testing.T) {
	fw := Summarize(catalogForSummary(t), nil, now).Frameworks[0]
	if fw.Counts[ReqEvidenced] != 0 || fw.Counts[ReqFailing] != 0 {
		t.Fatalf("counts with no evidence = %v", fw.Counts)
	}
	if fw.Counts[ReqIncomplete] != 4 || fw.Counts[ReqUnmapped] != 1 {
		t.Fatalf("counts = %v", fw.Counts)
	}
}
