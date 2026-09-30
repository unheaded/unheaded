// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package server

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestComplianceFrameworkCSV(t *testing.T) {
	src := complianceFixture(t, true)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/compliance/frameworks/{id}", src.handleFramework)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/compliance/frameworks/low?format=csv", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("content-type %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "low") {
		t.Errorf("content-disposition %q", cd)
	}
	rows, err := csv.NewReader(rec.Body).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0][0] != "requirement" { // header + RA-5 + AC-2
		t.Fatalf("rows = %v", rows)
	}
	ra5 := rows[1]
	if ra5[0] != "RA-5" || ra5[2] != "EVIDENCED" || ra5[3] != "UH-VULN-01=PASS" || !strings.Contains(ra5[4], "github-job W/scan pass") ||
		!strings.Contains(ra5[4], "abc") {
		t.Errorf("RA-5 row = %v", ra5)
	}
	if rows[2][0] != "AC-2" || rows[2][2] != "UNMAPPED" || rows[2][3] != "" {
		t.Errorf("AC-2 row = %v", rows[2])
	}
}

func TestComplianceFrameworkCSV_NeutralisesFormulas(t *testing.T) {
	if got := csvCell("=HYPERLINK(\"x\")"); !strings.HasPrefix(got, "'") {
		t.Errorf("csvCell = %q: a leading = must not reach a spreadsheet as a formula", got)
	}
	if got := csvCell("RA-5"); got != "RA-5" {
		t.Errorf("csvCell(RA-5) = %q", got)
	}
}

func TestComplianceFindings(t *testing.T) {
	src := complianceFixture(t, false)
	f, err := os.Create(src.evidencePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := crosswalk.WriteEvidence(f, []crosswalk.Record{{
		Source:  crosswalk.Source{Kind: crosswalk.KindGitHubJob, Ref: "W/scan"},
		Verdict: crosswalk.VerdictFail, ObservedAt: time.Now().Add(-time.Hour), Detail: "https://x",
	}}); err != nil {
		t.Fatal(err)
	}
	f.Close()

	// No register: the finding is still open, untriaged.
	rec, body := get(t, src.handleFindings, "/api/v1/compliance/findings")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	open := body["open"].([]any)
	if len(open) != 1 || open[0].(map[string]any)["severity"] != "untriaged" || body["zero_claimable"] != false {
		t.Fatalf("no register: %v", body)
	}

	reg := filepath.Join(src.root, "compliance/findings/register.yaml")
	if err := os.MkdirAll(filepath.Dir(reg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(reg, []byte(`findings:
  - {source: "github-job:W/scan", severity: high, class: ci, opened: 2026-09-30, lockout: none, step: 1, plan: fix it}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, body = get(t, src.handleFindings, "/api/v1/compliance/findings")
	f0 := body["open"].([]any)[0].(map[string]any)
	if f0["severity"] != "high" || f0["entry"].(map[string]any)["plan"] != "fix it" {
		t.Errorf("triaged finding = %v", f0)
	}

	if err := os.WriteFile(reg, []byte("findings:\n  - {source: \"github-job:gone\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if rec, _ := get(t, src.handleFindings, "/api/v1/compliance/findings"); rec.Code != http.StatusInternalServerError {
		t.Errorf("invalid register: status %d, want 500 (never silently untriaged)", rec.Code)
	}
	var nilSrc *complianceSource
	if rec, _ := get(t, nilSrc.handleFindings, "/api/v1/compliance/findings"); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("not configured: status %d", rec.Code)
	}
}

func TestComplianceFrameworkCSV_SourceMapped(t *testing.T) {
	src := complianceFixture(t, false)
	if err := os.WriteFile(filepath.Join(src.root, "compliance/catalog/controls.yaml"), []byte(`
controls:
  - id: UH-VULN-01
    title: t
    statement: s
    freshness_days: 7
    evidence:
      - {kind: github-job, ref: "W/scan", maps: {base: [RA-5]}}
      - {kind: github-job, ref: "W/other", maps: {base: [SI-2]}}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	writeEvidence(t, src.evidencePath, []crosswalk.Record{
		{Source: crosswalk.Source{Kind: crosswalk.KindGitHubJob, Ref: "W/scan"}, Verdict: crosswalk.VerdictPass, ObservedAt: time.Now().Add(-time.Hour), Detail: "ok"},
		{Source: crosswalk.Source{Kind: crosswalk.KindGitHubJob, Ref: "W/other"}, Verdict: crosswalk.VerdictFail, ObservedAt: time.Now().Add(-time.Hour), Detail: "red"},
	})
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/compliance/frameworks/{id}", src.handleFramework)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/compliance/frameworks/base?format=csv", nil))
	rows, err := csv.NewReader(rec.Body).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string][]string{}
	for _, r := range rows[1:] {
		byID[r[0]] = r
	}
	if r := byID["RA-5"]; r[2] != "EVIDENCED" || !strings.Contains(r[4], "source github-job:W/scan pass") {
		t.Errorf("RA-5 = %v (its own source passes; the sibling's failure must not leak in)", r)
	}
	if r := byID["SI-2"]; r[2] != "FAILING" || !strings.Contains(r[4], "source github-job:W/other fail") {
		t.Errorf("SI-2 = %v", r)
	}
}
