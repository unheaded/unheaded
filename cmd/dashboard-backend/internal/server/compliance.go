// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package server

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"unheaded/pkg/compliance/crosswalk"
	"unheaded/pkg/compliance/well"
)

// complianceSource serves the compliance crosswalk (ADR-097): the catalog
// from root/compliance/catalog, evidence from evidencePath (written by
// cmd/compliance-evidence). Both are read per request; the page is a human
// view and the files are small.
//
// With ops set, evidence and findings triage come from The Well
// (unheaded_ops, read-only role) when it holds them, and from the files
// otherwise; every response says which (ADR-099 amended: The Well first,
// labelled fallback).
type complianceSource struct {
	root         string
	evidencePath string
	ops          *well.Lazy // unheaded_ops as ops_reader
	config       *well.Lazy // unheaded_config as config_reader
	admin        *well.Lazy // unheaded_config as config_admin: the profile switch only

	// writeStore replaces the config_admin store (tests).
	writeStore func(ctx context.Context) (well.Store, error)

	mu   sync.Mutex
	last complianceDataSource
}

// Where a response's data came from.
const (
	srcWell     = "well"
	srcFile     = "file-fallback"
	srcYAML     = "yaml-fallback"
	wellTimeout = 5 * time.Second
)

// complianceDataSource labels each response, so fallback data is never
// shown as live.
type complianceDataSource struct {
	Evidence string `json:"evidence"`
	Register string `json:"register"`
	Reason   string `json:"reason,omitempty"`
}

// opsStore returns The Well's ops store, or why there is none.
func (c *complianceSource) opsStore(ctx context.Context) (*well.OpsStore, string) {
	if c.ops == nil {
		return nil, "The Well is not configured"
	}
	db, err := c.ops.DB(ctx)
	if err != nil {
		return nil, err.Error()
	}
	return well.NewOpsStore(db), ""
}

func (c *complianceSource) remember(src complianceDataSource) {
	c.mu.Lock()
	c.last = src
	c.mu.Unlock()
}

// lastSource is the source of the latest load, for the metrics.
func (c *complianceSource) lastSource() complianceDataSource {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.last
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
	Source            complianceDataSource        `json:"source"`
}

// complianceFindings is the findings report with its source.
type complianceFindings struct {
	*crosswalk.FindingsReport
	Source complianceDataSource `json:"source"`
}

// load returns the catalog, the evidence records, whether any evidence
// exists, and where it came from. The Well is used when it holds evidence;
// otherwise the file. A missing file is not an error (every control reads
// NOT_ASSESSED); an unreadable or corrupt one is.
func (c *complianceSource) load(ctx context.Context) (*crosswalk.Catalog, []crosswalk.Record, bool, complianceDataSource, int, error) {
	src := complianceDataSource{Evidence: srcFile}
	cat, err := crosswalk.LoadFS(os.DirFS(c.root), "compliance/catalog/frameworks", "compliance/catalog/controls.yaml")
	if err != nil {
		return nil, nil, false, src, http.StatusServiceUnavailable, err
	}
	ctx, cancel := context.WithTimeout(ctx, wellTimeout)
	defer cancel()
	if store, why := c.opsStore(ctx); store != nil {
		recs, err := store.LatestEvidence(ctx)
		switch {
		case err != nil:
			src.Reason = "evidence: " + err.Error()
		case len(recs) == 0:
			src.Reason = "The Well holds no evidence yet; the collector writes " + c.evidencePath
		default:
			src.Evidence = srcWell
			return cat, recs, true, src, 0, nil
		}
	} else {
		src.Reason = why
	}
	f, err := os.Open(c.evidencePath)
	if errors.Is(err, fs.ErrNotExist) {
		return cat, nil, false, src, 0, nil
	}
	if err != nil {
		return nil, nil, false, src, http.StatusInternalServerError, err
	}
	defer f.Close()
	recs, err := crosswalk.ReadEvidence(f)
	if err != nil {
		return nil, nil, false, src, http.StatusInternalServerError, err
	}
	return cat, recs, true, src, 0, nil
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
	cat, recs, have, src, code, err := c.load(r.Context())
	if err != nil {
		http.Error(w, "compliance: "+err.Error(), code)
		return
	}
	src.Register = "" // the summary does not use the register
	c.remember(src)
	now := time.Now()
	sum := crosswalk.Summarize(cat, recs, now)

	latest := latestBySource(recs, now)
	out := complianceSummary{At: now, EvidenceAvailable: have, EvidenceRecords: len(recs), Source: src}
	for _, ctl := range cat.Controls {
		cc := complianceControl{Control: ctl, Status: sum.Controls[ctl.ID]}
		for _, s := range ctl.Evidence {
			e := complianceEvidence{Source: s.Source}
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

// handleFindings serves the findings register (ADR-098): open findings from
// evidence, triage from compliance/findings/register.yaml. A missing register
// leaves every finding untriaged; an invalid one is an error, never ignored.
func (c *complianceSource) handleFindings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if c == nil {
		http.Error(w, "compliance catalog not configured", http.StatusServiceUnavailable)
		return
	}
	rep, _, src, code, err := c.findings(r.Context(), time.Now())
	if err != nil {
		http.Error(w, "compliance: "+err.Error(), code)
		return
	}
	writeComplianceJSON(w, complianceFindings{FindingsReport: rep, Source: src})
}

// findings loads catalog, evidence and register and builds the report.
// Triage comes from The Well when it holds any, else from register.yaml. A
// missing register leaves every finding untriaged; an invalid one, from
// either source, is an error.
func (c *complianceSource) findings(ctx context.Context, now time.Time) (*crosswalk.FindingsReport, []crosswalk.Record, complianceDataSource, int, error) {
	cat, recs, _, src, code, err := c.load(ctx)
	if err != nil {
		return nil, nil, src, code, err
	}
	src.Register = srcYAML
	ctx, cancel := context.WithTimeout(ctx, wellTimeout)
	defer cancel()
	if store, why := c.opsStore(ctx); store != nil {
		reg, err := store.Register(ctx, cat)
		switch {
		case err != nil:
			return nil, nil, src, http.StatusInternalServerError, fmt.Errorf("register in The Well: %w", err)
		case len(reg.Entries) > 0:
			src.Register = srcWell
			c.remember(src)
			return crosswalk.Findings(cat, recs, reg, now), recs, src, 0, nil
		default:
			src.Reason = joinReason(src.Reason, "The Well holds no findings triage yet (run compliance-evidence well sync)")
		}
	} else if src.Reason == "" {
		src.Reason = why
	}
	var reg *crosswalk.Register
	raw, err := fs.ReadFile(os.DirFS(c.root), "compliance/findings/register.yaml")
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, nil, src, http.StatusInternalServerError, fmt.Errorf("register: %w", err)
	default:
		if reg, err = crosswalk.LoadRegister(raw, cat); err != nil {
			return nil, nil, src, http.StatusInternalServerError, err
		}
	}
	c.remember(src)
	return crosswalk.Findings(cat, recs, reg, now), recs, src, 0, nil
}

func joinReason(a, b string) string {
	if a == "" || a == b {
		return b
	}
	return a + "; " + b
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
	cat, recs, _, _, code, err := c.load(r.Context())
	if err != nil {
		http.Error(w, "compliance: "+err.Error(), code)
		return
	}
	id := r.PathValue("id")
	now := time.Now()
	sum := crosswalk.Summarize(cat, recs, now)
	for _, fw := range sum.Frameworks {
		if fw.ID != id {
			continue
		}
		if r.URL.Query().Get("format") == "csv" {
			writeFrameworkCSV(w, cat, sum, fw, latestBySource(recs, now))
			return
		}
		writeComplianceJSON(w, fw)
		return
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

// latestBySource keeps each source's newest record at or before now.
func latestBySource(recs []crosswalk.Record, now time.Time) map[string]crosswalk.Record {
	latest := make(map[string]crosswalk.Record)
	for _, rec := range recs {
		k := rec.Source.Key()
		if cur, ok := latest[k]; (!ok || rec.ObservedAt.After(cur.ObservedAt)) && !rec.ObservedAt.After(now) {
			latest[k] = rec
		}
	}
	return latest
}

// csvCell stops a cell from being read as a spreadsheet formula (CSV
// injection): evidence details come from tool output and job URLs.
func csvCell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

// writeFrameworkCSV is the auditor export: one row per requirement with its
// status, the controls mapped to it and every piece of their evidence.
func writeFrameworkCSV(w http.ResponseWriter, cat *crosswalk.Catalog, sum *crosswalk.Summary,
	fw *crosswalk.FrameworkSummary, latest map[string]crosswalk.Record) {
	byID := make(map[string]*crosswalk.Control, len(cat.Controls))
	for _, c := range cat.Controls {
		byID[c.ID] = c
	}
	var buf bytes.Buffer
	cw := csv.NewWriter(&buf)
	_ = cw.Write([]string{"requirement", "title", "status", "controls", "evidence", "framework", "version", "exported_at"})
	for _, req := range fw.Requirements {
		var ctls, ev []string
		for _, id := range req.Controls {
			ctls = append(ctls, id+"="+string(sum.Controls[id]))
			for _, src := range byID[id].Evidence {
				line := id + ": " + src.Kind + " " + src.Ref
				if rec, ok := latest[src.Key()]; ok {
					line += fmt.Sprintf(" %s %s %s %s", rec.Verdict, rec.ObservedAt.UTC().Format(time.RFC3339), rec.Commit, rec.Detail)
				} else {
					line += " no evidence"
				}
				ev = append(ev, line)
			}
		}
		for _, key := range req.Sources { // mapped per source: that source's evidence alone
			line := "source " + key
			if rec, ok := latest[key]; ok {
				line += fmt.Sprintf(" %s %s %s %s", rec.Verdict, rec.ObservedAt.UTC().Format(time.RFC3339), rec.Commit, rec.Detail)
			} else {
				line += " no evidence"
			}
			ev = append(ev, line)
		}
		controls := strings.Join(ctls, " ") // "UH-VULN-01=PASS UH-SAST-01=FAIL"
		_ = cw.Write([]string{csvCell(req.ID), csvCell(req.Title), string(req.Status), csvCell(controls),
			csvCell(strings.Join(ev, " | ")), csvCell(fw.Name), csvCell(fw.Version), sum.At.UTC().Format(time.RFC3339)})
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		http.Error(w, "encode csv", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="compliance-`+fw.ID+`.csv"`)
	_, _ = w.Write(buf.Bytes()) // #nosec G104 -- status committed; a failed write means the client left
}
