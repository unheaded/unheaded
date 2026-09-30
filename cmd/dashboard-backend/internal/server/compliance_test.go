// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"unheaded/pkg/compliance/crosswalk"
)

func complianceFixture(t *testing.T, withEvidence bool) *complianceSource {
	t.Helper()
	root := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(root, "compliance/catalog/frameworks"), 0o755))
	must(os.WriteFile(filepath.Join(root, "compliance/catalog/frameworks/fw.yaml"), []byte(`
frameworks:
  - id: base
    name: Base
    requirements: [{id: RA-5, title: Vulnerability Monitoring}, {id: SI-2}, {id: AC-2}]
  - id: low
    name: Low
    derived_from: base
    requirements: [{id: RA-5}, {id: AC-2}]
`), 0o644))
	must(os.WriteFile(filepath.Join(root, "compliance/catalog/controls.yaml"), []byte(`
controls:
  - id: UH-VULN-01
    title: Vulnerable dependencies fail the build
    statement: s
    freshness_days: 7
    evidence: [{kind: github-job, ref: "W/scan"}]
    mappings: {base: [RA-5, SI-2]}
`), 0o644))
	ev := filepath.Join(root, "evidence.json")
	if withEvidence {
		f, err := os.Create(ev)
		must(err)
		must(crosswalk.WriteEvidence(f, []crosswalk.Record{{
			Source:  crosswalk.Source{Kind: crosswalk.KindGitHubJob, Ref: "W/scan"},
			Verdict: crosswalk.VerdictPass, ObservedAt: time.Now().Add(-time.Hour), Commit: "abc", Detail: "https://x",
		}}))
		must(f.Close())
	}
	return &complianceSource{root: root, evidencePath: ev}
}

func get(t *testing.T, h http.HandlerFunc, url string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, url, nil))
	var body map[string]any
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("body not JSON: %v\n%s", err, rec.Body.String())
		}
	}
	return rec, body
}

func TestComplianceSummary(t *testing.T) {
	src := complianceFixture(t, true)
	rec, body := get(t, src.handleSummary, "/api/v1/compliance/summary")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if body["evidence_available"] != true {
		t.Errorf("evidence_available = %v", body["evidence_available"])
	}
	ctls := body["controls"].([]any)
	c0 := ctls[0].(map[string]any)
	if c0["status"] != "PASS" {
		t.Errorf("control status = %v", c0["status"])
	}
	ev := c0["evidence"].([]any)[0].(map[string]any)
	if ev["latest"].(map[string]any)["detail"] != "https://x" {
		t.Errorf("latest evidence = %v", ev["latest"])
	}
	fws := body["frameworks"].([]any)
	low := fws[1].(map[string]any)
	if low["id"] != "low" || low["total"].(float64) != 2 {
		t.Fatalf("low = %v", low)
	}
	counts := low["counts"].(map[string]any)
	if counts["EVIDENCED"].(float64) != 1 || counts["UNMAPPED"].(float64) != 1 {
		t.Errorf("low counts = %v (RA-5 evidenced via the base mapping, AC-2 unmapped)", counts)
	}
	if _, ok := low["requirements"]; ok {
		t.Error("summary carries requirement lists; those belong to the framework endpoint")
	}
}

func TestComplianceSummary_NoEvidenceClaimsNothing(t *testing.T) {
	src := complianceFixture(t, false)
	rec, body := get(t, src.handleSummary, "/api/v1/compliance/summary")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if body["evidence_available"] != false {
		t.Errorf("evidence_available = %v", body["evidence_available"])
	}
	if c := body["controls"].([]any)[0].(map[string]any); c["status"] != "NOT_ASSESSED" {
		t.Errorf("status without evidence = %v", c["status"])
	}
}

func TestComplianceFramework(t *testing.T) {
	src := complianceFixture(t, true)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/compliance/frameworks/{id}", src.handleFramework)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/compliance/frameworks/base", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var fw crosswalk.FrameworkSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &fw); err != nil {
		t.Fatal(err)
	}
	if fw.Total != 3 || len(fw.Requirements) != 3 || fw.Requirements[0].Title != "Vulnerability Monitoring" {
		t.Fatalf("framework = %+v", fw)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/compliance/frameworks/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown framework: status %d", rec.Code)
	}
}

func TestCompliance_Errors(t *testing.T) {
	src := &complianceSource{root: t.TempDir(), evidencePath: "/nonexistent"}
	if rec, _ := get(t, src.handleSummary, "/api/v1/compliance/summary"); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("no catalog: status %d, want 503", rec.Code)
	}
	var nilSrc *complianceSource
	if rec, _ := get(t, nilSrc.handleSummary, "/api/v1/compliance/summary"); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("not configured: status %d, want 503", rec.Code)
	}
	good := complianceFixture(t, true)
	rec := httptest.NewRecorder()
	good.handleSummary(rec, httptest.NewRequest(http.MethodPost, "/api/v1/compliance/summary", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: status %d", rec.Code)
	}
	if err := os.WriteFile(good.evidencePath, []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	if rec, _ := get(t, good.handleSummary, "/api/v1/compliance/summary"); rec.Code != http.StatusInternalServerError {
		t.Errorf("corrupt evidence: status %d, want 500 (never silently empty)", rec.Code)
	}
}
