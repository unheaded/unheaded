// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package crosswalk

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const smFrameworks = `
frameworks:
  - id: cis
    name: CIS
    requirements: [{id: "2.1.1"}, {id: "2.1.2"}, {id: "2.1.3"}]
  - id: cis-l1
    name: CIS L1
    derived_from: cis
    requirements: [{id: "2.1.1"}, {id: "2.1.3"}]
  - id: nist
    name: NIST
    requirements: [{id: CM-7}]
`

const smControls = `
controls:
  - id: UH-SVC-01
    title: t
    statement: s
    freshness_days: 1
    evidence:
      - {kind: host-probe, ref: "west:svc-a", maps: {cis: ["2.1.1"]}}
      - {kind: host-probe, ref: "west:svc-b", maps: {cis: ["2.1.2"]}}
      - {kind: host-probe, ref: "west:svc-c", maps: {cis: ["2.1.3"]}}
    mappings: {nist: [CM-7]}
`

func TestSourceMaps_RequirementFollowsItsOwnSources(t *testing.T) {
	c, err := Load([]byte(smFrameworks), []byte(smControls))
	if err != nil {
		t.Fatal(err)
	}
	src := func(r string) Source { return Source{Kind: KindHostProbe, Ref: r} }
	day := 24 * time.Hour
	recs := []Record{
		rec(src("west:svc-a"), VerdictPass, time.Hour),
		rec(src("west:svc-b"), VerdictFail, time.Hour),
		rec(src("west:svc-c"), VerdictPass, 2*day), // older than freshness_days 1
	}
	s := Summarize(c, recs, now)
	if s.Controls["UH-SVC-01"] != StatusFail {
		t.Errorf("control = %s", s.Controls["UH-SVC-01"])
	}
	got := map[string]map[string]ReqStatus{}
	for _, fw := range s.Frameworks {
		got[fw.ID] = map[string]ReqStatus{}
		for _, r := range fw.Requirements {
			got[fw.ID][r.ID] = r.Status
		}
	}
	want := map[string]map[string]ReqStatus{
		// one failing service no longer fails its siblings
		"cis":    {"2.1.1": ReqEvidenced, "2.1.2": ReqFailing, "2.1.3": ReqIncomplete},
		"cis-l1": {"2.1.1": ReqEvidenced, "2.1.3": ReqIncomplete},
		// control-level mappings keep control-level semantics
		"nist": {"CM-7": ReqFailing},
	}
	for fw, reqs := range want {
		for id, st := range reqs {
			if got[fw][id] != st {
				t.Errorf("%s %s = %s, want %s", fw, id, got[fw][id], st)
			}
		}
	}
	for _, fw := range s.Frameworks {
		if fw.ID != "cis" {
			continue
		}
		if r := fw.Requirements[1]; strings.Join(r.Sources, ",") != "host-probe:west:svc-b" || len(r.Controls) != 0 {
			t.Errorf("2.1.2 contributors: controls %v sources %v", r.Controls, r.Sources)
		}
	}
	if ControlStatus(c.Controls[0], nil, now) != StatusNotAssessed {
		t.Error("ControlStatus changed for a control with source maps")
	}
}

func TestSourceMaps_Rejects(t *testing.T) {
	for name, ctl := range map[string]string{
		"unknown framework":           `{kind: host-probe, ref: "west:a", maps: {nope: ["2.1.1"]}}`,
		"unknown requirement":         `{kind: host-probe, ref: "west:a", maps: {cis: ["9.9.9"]}}`,
		"derived framework":           `{kind: host-probe, ref: "west:a", maps: {cis-l1: ["2.1.1"]}}`,
		"both levels, same framework": `{kind: host-probe, ref: "west:a", maps: {nist: [CM-7]}}`,
	} {
		ctls := "controls:\n  - id: UH-SVC-01\n    title: t\n    statement: s\n    freshness_days: 1\n    evidence:\n      - " + ctl + "\n    mappings: {nist: [CM-7]}\n"
		if _, err := Load([]byte(smFrameworks), []byte(ctls)); !errors.Is(err, ErrInvalidCatalog) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	// Source-level mappings alone are enough for a control.
	only := "controls:\n  - id: UH-SVC-01\n    title: t\n    statement: s\n    freshness_days: 1\n    evidence:\n      - {kind: host-probe, ref: \"west:a\", maps: {cis: [\"2.1.1\"]}}\n"
	if _, err := Load([]byte(smFrameworks), []byte(only)); err != nil {
		t.Errorf("source maps only: %v", err)
	}
}
