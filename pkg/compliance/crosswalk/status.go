// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package crosswalk

import "time"

// Verdict is what one collector run observed for one evidence source.
type Verdict string

const (
	VerdictPass Verdict = "pass"
	VerdictFail Verdict = "fail"
)

// Record is one observation of one evidence source. Detail holds the raw
// observation (exit code, job conclusion, URL): the fact, not a judgement.
type Record struct {
	Source     Source    `json:"source"`
	Verdict    Verdict   `json:"verdict"`
	ObservedAt time.Time `json:"observed_at"`
	Commit     string    `json:"commit,omitempty"`
	Detail     string    `json:"detail,omitempty"`
}

// Status of a control, derived from its sources' latest records.
type Status string

const (
	StatusPass        Status = "PASS"
	StatusFail        Status = "FAIL"
	StatusStale       Status = "STALE"
	StatusNotAssessed Status = "NOT_ASSESSED"
)

// ReqStatus of a framework requirement, derived from the controls mapped to it.
type ReqStatus string

const (
	// ReqEvidenced: every mapped control has current passing evidence. It
	// says the evidence supports the requirement, not that an auditor would
	// find it satisfied: a mapping covers part of a requirement at most.
	ReqEvidenced ReqStatus = "EVIDENCED"
	// ReqFailing: at least one mapped control fails.
	ReqFailing ReqStatus = "FAILING"
	// ReqIncomplete: mapped, nothing failing, but some mapped control is
	// unassessed or stale.
	ReqIncomplete ReqStatus = "INCOMPLETE"
	// ReqUnmapped: no control maps to it. Counted, never dropped.
	ReqUnmapped ReqStatus = "UNMAPPED"
)

// ControlStatus derives a control's status from evidence records (any
// order, any sources). Per source, the latest record observed at or before
// now counts; a future-dated or unrecognised record is not evidence.
// FAIL beats everything, including staleness: an old failure is not
// evidence of a fix. Otherwise a source without a record makes the control
// NOT_ASSESSED, and a latest pass older than FreshnessDays makes it STALE.
func ControlStatus(ctl *Control, recs []Record, now time.Time) Status {
	latest := make(map[string]Record, len(ctl.Evidence))
	want := make(map[string]bool, len(ctl.Evidence))
	for _, s := range ctl.Evidence {
		want[s.Key()] = true
	}
	for _, r := range recs {
		k := r.Source.Key()
		if !want[k] || r.ObservedAt.After(now) {
			continue
		}
		if r.Verdict != VerdictPass && r.Verdict != VerdictFail {
			continue
		}
		if cur, ok := latest[k]; !ok || r.ObservedAt.After(cur.ObservedAt) {
			latest[k] = r
		}
	}

	window := time.Duration(ctl.FreshnessDays) * 24 * time.Hour
	missing, stale := false, false
	for _, s := range ctl.Evidence {
		r, ok := latest[s.Key()]
		switch {
		case !ok:
			missing = true
		case r.Verdict == VerdictFail:
			return StatusFail
		case now.Sub(r.ObservedAt) > window:
			stale = true
		}
	}
	switch {
	case missing:
		return StatusNotAssessed
	case stale:
		return StatusStale
	default:
		return StatusPass
	}
}

// RequirementResult is one requirement and why it has its status.
type RequirementResult struct {
	ID       string    `json:"id"`
	Title    string    `json:"title,omitempty"`
	Status   ReqStatus `json:"status"`
	Controls []string  `json:"controls,omitempty"`
	Sources  []string  `json:"sources,omitempty"` // mapped per source (EvidenceSource.Maps)
}

// FrameworkSummary is a framework's requirements with the full denominator.
type FrameworkSummary struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	Version      string              `json:"version"`
	Granularity  string              `json:"granularity"`
	Source       string              `json:"source"`
	Total        int                 `json:"total"`
	Counts       map[ReqStatus]int   `json:"counts"`
	Requirements []RequirementResult `json:"requirements"`
}

// Summary is the whole crosswalk evaluated at one instant.
type Summary struct {
	At         time.Time           `json:"at"`
	Controls   map[string]Status   `json:"controls"`
	Frameworks []*FrameworkSummary `json:"frameworks"`
}

// Summarize evaluates every control, then every requirement of every
// framework in catalog order. A requirement's contributors are the controls
// mapping it and the individual sources mapping it; it is EVIDENCED only
// when every contributor passes.
func Summarize(c *Catalog, recs []Record, now time.Time) *Summary {
	s := &Summary{At: now, Controls: make(map[string]Status, len(c.Controls))}
	mappedBy := make(map[string]map[string][]string)  // framework -> requirement -> controls
	mappedSrc := make(map[string]map[string][]string) // framework -> requirement -> source keys
	srcStatus := make(map[string]Status)              // source key -> status (per owning control's window)
	add := func(m map[string]map[string][]string, fw, req, v string) {
		if m[fw] == nil {
			m[fw] = make(map[string][]string)
		}
		m[fw][req] = append(m[fw][req], v)
	}
	for _, ctl := range c.Controls {
		s.Controls[ctl.ID] = ControlStatus(ctl, recs, now)
		for fw, reqs := range ctl.Mappings {
			for _, r := range reqs {
				add(mappedBy, fw, r, ctl.ID)
			}
		}
		for _, src := range ctl.Evidence {
			if len(src.Maps) == 0 {
				continue
			}
			one := &Control{ID: ctl.ID, FreshnessDays: ctl.FreshnessDays, Evidence: []EvidenceSource{{Source: src.Source}}}
			st := ControlStatus(one, recs, now)
			if prev, ok := srcStatus[src.Key()]; !ok || worse(st, prev) {
				srcStatus[src.Key()] = st
			}
			for fw, reqs := range src.Maps {
				for _, r := range reqs {
					add(mappedSrc, fw, r, src.Key())
				}
			}
		}
	}

	for _, fw := range c.Frameworks {
		fs := &FrameworkSummary{
			ID: fw.ID, Name: fw.Name, Version: fw.Version, Granularity: fw.Granularity, Source: fw.Source,
			Total:  len(fw.Requirements),
			Counts: map[ReqStatus]int{ReqEvidenced: 0, ReqFailing: 0, ReqIncomplete: 0, ReqUnmapped: 0},
		}
		for _, req := range fw.Requirements {
			ctls := mappedBy[fw.ID][req.ID]
			srcs := mappedSrc[fw.ID][req.ID]
			var sts []Status
			for _, id := range ctls {
				sts = append(sts, s.Controls[id])
			}
			for _, k := range srcs {
				sts = append(sts, srcStatus[k])
			}
			st := requirementStatus(sts)
			fs.Counts[st]++
			fs.Requirements = append(fs.Requirements, RequirementResult{ID: req.ID, Title: req.Title, Status: st, Controls: ctls, Sources: srcs})
		}
		s.Frameworks = append(s.Frameworks, fs)
	}
	return s
}

// worse orders statuses for a source listed by several controls with
// different windows: FAIL, then NOT_ASSESSED, STALE, PASS.
func worse(a, b Status) bool {
	rank := map[Status]int{StatusFail: 0, StatusNotAssessed: 1, StatusStale: 2, StatusPass: 3}
	return rank[a] < rank[b]
}

func requirementStatus(sts []Status) ReqStatus {
	if len(sts) == 0 {
		return ReqUnmapped
	}
	all := true
	for _, st := range sts {
		switch st {
		case StatusFail:
			return ReqFailing
		case StatusPass:
		default:
			all = false
		}
	}
	if all {
		return ReqEvidenced
	}
	return ReqIncomplete
}
