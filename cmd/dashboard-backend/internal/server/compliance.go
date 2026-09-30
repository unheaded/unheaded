// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package server

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"time"

	"unheaded/pkg/compliance/crosswalk"
)

// complianceSource serves the compliance crosswalk (ADR-097): the catalog
// from root/compliance/catalog, evidence from evidencePath (written by
// cmd/compliance-evidence). Both are read per request; the page is a human
// view and the files are small.
type complianceSource struct {
	root         string
	evidencePath string
}

type complianceEvidence struct {
	crosswalk.Source
	Latest *crosswalk.Record `json:"latest,omitempty"`
}

type complianceControl struct {
	*crosswalk.Control
	Status   crosswalk.Status     `json:"status"`
	Evidence []complianceEvidence `json:"evidence"`
}

type complianceFrameworkCounts struct {
	ID          string                      `json:"id"`
	Name        string                      `json:"name"`
	Version     string                      `json:"version"`
	Granularity string                      `json:"granularity"`
	Source      string                      `json:"source"`
	DerivedFrom string                      `json:"derived_from,omitempty"`
	Total       int                         `json:"total"`
	Counts      map[crosswalk.ReqStatus]int `json:"counts"`
}

type complianceSummary struct {
	At                time.Time                   `json:"at"`
	EvidenceAvailable bool                        `json:"evidence_available"`
	EvidenceRecords   int                         `json:"evidence_records"`
	Controls          []complianceControl         `json:"controls"`
	Frameworks        []complianceFrameworkCounts `json:"frameworks"`
}

// load returns the catalog, the evidence records, and whether an evidence
// file existed. A missing evidence file is not an error (every control
// reads NOT_ASSESSED); an unreadable or corrupt one is.
func (c *complianceSource) load() (*crosswalk.Catalog, []crosswalk.Record, bool, int, error) {
	cat, err := crosswalk.LoadFS(os.DirFS(c.root), "compliance/catalog/frameworks", "compliance/catalog/controls.yaml")
	if err != nil {
		return nil, nil, false, http.StatusServiceUnavailable, err
	}
	f, err := os.Open(c.evidencePath)
	if errors.Is(err, fs.ErrNotExist) {
		return cat, nil, false, 0, nil
	}
	if err != nil {
		return nil, nil, false, http.StatusInternalServerError, err
	}
	defer f.Close()
	recs, err := crosswalk.ReadEvidence(f)
	if err != nil {
		return nil, nil, false, http.StatusInternalServerError, err
	}
	return cat, recs, true, 0, nil
}

func (c *complianceSource) handleSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if c == nil {
		http.Error(w, "compliance catalog not configured", http.StatusServiceUnavailable)
		return
	}
	cat, recs, have, code, err := c.load()
	if err != nil {
		http.Error(w, "compliance: "+err.Error(), code)
		return
	}
	now := time.Now()
	sum := crosswalk.Summarize(cat, recs, now)

	latest := make(map[string]crosswalk.Record)
	for _, rec := range recs {
		k := rec.Source.Key()
		if cur, ok := latest[k]; (!ok || rec.ObservedAt.After(cur.ObservedAt)) && !rec.ObservedAt.After(now) {
			latest[k] = rec
		}
	}
	out := complianceSummary{At: now, EvidenceAvailable: have, EvidenceRecords: len(recs)}
	for _, ctl := range cat.Controls {
		cc := complianceControl{Control: ctl, Status: sum.Controls[ctl.ID]}
		for _, s := range ctl.Evidence {
			e := complianceEvidence{Source: s}
			if rec, ok := latest[s.Key()]; ok {
				rec := rec
				e.Latest = &rec
			}
			cc.Evidence = append(cc.Evidence, e)
		}
		out.Controls = append(out.Controls, cc)
	}
	for i, fw := range sum.Frameworks {
		out.Frameworks = append(out.Frameworks, complianceFrameworkCounts{
			ID: fw.ID, Name: fw.Name, Version: fw.Version, Granularity: fw.Granularity, Source: fw.Source,
			DerivedFrom: cat.Frameworks[i].DerivedFrom, Total: fw.Total, Counts: fw.Counts,
		})
	}
	writeComplianceJSON(w, out)
}

func (c *complianceSource) handleFramework(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if c == nil {
		http.Error(w, "compliance catalog not configured", http.StatusServiceUnavailable)
		return
	}
	cat, recs, _, code, err := c.load()
	if err != nil {
		http.Error(w, "compliance: "+err.Error(), code)
		return
	}
	id := r.PathValue("id")
	for _, fw := range crosswalk.Summarize(cat, recs, time.Now()).Frameworks {
		if fw.ID == id {
			writeComplianceJSON(w, fw)
			return
		}
	}
	http.Error(w, "unknown framework", http.StatusNotFound)
}

// writeComplianceJSON encodes before writing the status, so a failed encode
// is a 500 rather than a 200 with an empty body.
func writeComplianceJSON(w http.ResponseWriter, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		http.Error(w, "encode", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body) // #nosec G104 -- status committed; a failed write means the client left
}
