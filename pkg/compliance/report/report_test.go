// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package report

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/compliance/controls"
	"unheaded/pkg/compliance/standards"
)

// ---------- helpers ----------

func makeControl(id, name string, cat controls.ControlCategory, sev controls.Severity) *controls.Control {
	return controls.NewControl(id, "TEST-STD", name, "desc for "+id).
		WithCategory(cat).
		WithSeverity(sev)
}

func makeAssessment(results ...*controls.ControlResult) *standards.AssessmentResult {
	a := &standards.AssessmentResult{
		Standard:      "TEST-STD",
		Version:       "1.0",
		TotalControls: len(results),
		Results:       results,
	}
	for _, r := range results {
		switch r.Status {
		case controls.StatusCompliant:
			a.Compliant++
		case controls.StatusNonCompliant:
			a.NonCompliant++
		case controls.StatusPartial:
			a.Partial++
		case controls.StatusNotAssessed:
			a.NotAssessed++
		case controls.StatusNotApplicable:
			a.NotApplicable++
		}
	}
	assessable := a.TotalControls - a.NotApplicable - a.NotAssessed
	if assessable > 0 {
		a.ComplianceRate = float64(a.Compliant) / float64(assessable)
	}
	return a
}

func makeResult(controlID string, status controls.ControlStatus, score float64, findings ...controls.Finding) *controls.ControlResult {
	return &controls.ControlResult{
		ControlID: controlID,
		Status:    status,
		Score:     score,
		Findings:  findings,
		TestedAt:  time.Now(),
		Notes:     "test notes",
	}
}

func makeFinding(id, title string, sev controls.Severity) controls.Finding {
	return controls.Finding{
		ID:          id,
		Title:       title,
		Description: "finding description",
		Severity:    sev,
		Resource:    "test-resource",
		Remediation: "fix it",
		DetectedAt:  time.Now(),
	}
}

// ---------- NewReportGenerator ----------

func TestNewReportGenerator(t *testing.T) {
	g := NewReportGenerator()
	if g == nil {
		t.Fatal("expected non-nil generator")
	}
	if g.assessments == nil || g.controls == nil || g.evidence == nil {
		t.Fatal("expected maps to be initialized")
	}
}

// ---------- AddAssessment / AddControl / AddEvidence ----------

func TestAddAssessment(t *testing.T) {
	g := NewReportGenerator()
	a := makeAssessment()
	g.AddAssessment("a1", a)
	if got := g.assessments["a1"]; got != a {
		t.Fatalf("expected stored assessment, got %v", got)
	}
}

func TestAddControl(t *testing.T) {
	g := NewReportGenerator()
	c := makeControl("C-1", "Ctrl 1", controls.CategoryAccessControl, controls.SeverityHigh)
	g.AddControl(c)
	if got := g.controls["C-1"]; got != c {
		t.Fatalf("expected stored control, got %v", got)
	}
}

func TestAddEvidence(t *testing.T) {
	g := NewReportGenerator()
	ev := []*controls.Evidence{{ID: "EV-1"}, {ID: "EV-2"}}
	g.AddEvidence("C-1", ev)
	if len(g.evidence["C-1"]) != 2 {
		t.Fatalf("expected 2 evidence items, got %d", len(g.evidence["C-1"]))
	}
	// append semantics
	g.AddEvidence("C-1", []*controls.Evidence{{ID: "EV-3"}})
	if len(g.evidence["C-1"]) != 3 {
		t.Fatalf("expected 3 evidence items after append, got %d", len(g.evidence["C-1"]))
	}
}

// ---------- GenerateAssessmentReport ----------

func TestGenerateAssessmentReport_NotFound(t *testing.T) {
	g := NewReportGenerator()
	_, err := g.GenerateAssessmentReport(context.Background(), "missing", "title")
	if err == nil || !strings.Contains(err.Error(), "assessment not found") {
		t.Fatalf("expected 'assessment not found' error, got %v", err)
	}
}

func TestGenerateAssessmentReport_Basic(t *testing.T) {
	g := NewReportGenerator()

	c1 := makeControl("C-1", "Access Control", controls.CategoryAccessControl, controls.SeverityHigh)
	c2 := makeControl("C-2", "Audit Logging", controls.CategoryAuditLogging, controls.SeverityMedium)
	g.AddControl(c1)
	g.AddControl(c2)

	g.AddEvidence("C-1", []*controls.Evidence{{ID: "EV-1"}, {ID: "EV-2"}})

	r1 := makeResult("C-1", controls.StatusCompliant, 1.0)
	r2 := makeResult("C-2", controls.StatusNonCompliant, 0.2,
		makeFinding("F-1", "Missing audit logs", controls.SeverityHigh),
		makeFinding("F-2", "Incomplete retention", controls.SeverityMedium),
	)
	assessment := makeAssessment(r1, r2)
	g.AddAssessment("a1", assessment)

	rpt, err := g.GenerateAssessmentReport(context.Background(), "a1", "Test Report")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Basic fields
	if rpt.Type != ReportTypeAssessment {
		t.Errorf("expected type %q, got %q", ReportTypeAssessment, rpt.Type)
	}
	if rpt.Title != "Test Report" {
		t.Errorf("expected title 'Test Report', got %q", rpt.Title)
	}
	if rpt.Standard != "TEST-STD" {
		t.Errorf("expected standard TEST-STD, got %q", rpt.Standard)
	}
	if !strings.HasPrefix(rpt.ID, "RPT-") {
		t.Errorf("expected ID prefix RPT-, got %q", rpt.ID)
	}

	// Summary
	if rpt.Summary.TotalControls != 2 {
		t.Errorf("expected 2 total controls, got %d", rpt.Summary.TotalControls)
	}
	if rpt.Summary.Compliant != 1 {
		t.Errorf("expected 1 compliant, got %d", rpt.Summary.Compliant)
	}
	if rpt.Summary.NonCompliant != 1 {
		t.Errorf("expected 1 non-compliant, got %d", rpt.Summary.NonCompliant)
	}

	// Findings severity counts
	if rpt.Summary.HighFindings != 1 {
		t.Errorf("expected 1 high finding, got %d", rpt.Summary.HighFindings)
	}
	if rpt.Summary.MediumFindings != 1 {
		t.Errorf("expected 1 medium finding, got %d", rpt.Summary.MediumFindings)
	}

	// Control results
	if len(rpt.ControlResults) != 2 {
		t.Fatalf("expected 2 control results, got %d", len(rpt.ControlResults))
	}

	// Evidence count for C-1
	for _, cr := range rpt.ControlResults {
		if cr.ControlID == "C-1" && cr.EvidenceCount != 2 {
			t.Errorf("expected 2 evidence for C-1, got %d", cr.EvidenceCount)
		}
	}

	// Findings
	if len(rpt.Findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(rpt.Findings))
	}
	for _, f := range rpt.Findings {
		if f.Status != "open" {
			t.Errorf("expected finding status 'open', got %q", f.Status)
		}
	}
}

func TestGenerateAssessmentReport_AllSeverities(t *testing.T) {
	g := NewReportGenerator()
	c := makeControl("C-1", "Ctrl", controls.CategoryAccessControl, controls.SeverityCritical)
	g.AddControl(c)

	r := makeResult("C-1", controls.StatusNonCompliant, 0.0,
		makeFinding("F-1", "crit", controls.SeverityCritical),
		makeFinding("F-2", "high", controls.SeverityHigh),
		makeFinding("F-3", "med", controls.SeverityMedium),
		makeFinding("F-4", "low", controls.SeverityLow),
		makeFinding("F-5", "info", controls.SeverityInfo), // not counted
	)
	g.AddAssessment("a1", makeAssessment(r))

	rpt, err := g.GenerateAssessmentReport(context.Background(), "a1", "Sev Test")
	if err != nil {
		t.Fatal(err)
	}

	if rpt.Summary.CriticalFindings != 1 {
		t.Errorf("expected 1 critical, got %d", rpt.Summary.CriticalFindings)
	}
	if rpt.Summary.HighFindings != 1 {
		t.Errorf("expected 1 high, got %d", rpt.Summary.HighFindings)
	}
	if rpt.Summary.MediumFindings != 1 {
		t.Errorf("expected 1 medium, got %d", rpt.Summary.MediumFindings)
	}
	if rpt.Summary.LowFindings != 1 {
		t.Errorf("expected 1 low, got %d", rpt.Summary.LowFindings)
	}
}

func TestGenerateAssessmentReport_SkipsUnknownControls(t *testing.T) {
	g := NewReportGenerator()
	// Don't add control definition — result should be skipped
	r := makeResult("C-UNKNOWN", controls.StatusCompliant, 1.0)
	g.AddAssessment("a1", makeAssessment(r))

	rpt, err := g.GenerateAssessmentReport(context.Background(), "a1", "Skip Test")
	if err != nil {
		t.Fatal(err)
	}
	if len(rpt.ControlResults) != 0 {
		t.Errorf("expected 0 control results, got %d", len(rpt.ControlResults))
	}
}

// ---------- calculateRiskScore ----------

func TestCalculateRiskScore(t *testing.T) {
	g := NewReportGenerator()

	tests := []struct {
		name    string
		summary ReportSummary
		want    float64
	}{
		{"zero", ReportSummary{}, 0.0},
		{"critical only", ReportSummary{CriticalFindings: 5}, 50.0},
		{"capped at 100", ReportSummary{CriticalFindings: 20}, 100.0},
		{"mixed", ReportSummary{
			CriticalFindings: 1,
			HighFindings:     2,
			MediumFindings:   3,
			LowFindings:      4,
		}, 1*10.0 + 2*5.0 + 3*2.0 + 4*0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := g.calculateRiskScore(tt.summary)
			if got != tt.want {
				t.Errorf("got %.2f, want %.2f", got, tt.want)
			}
		})
	}
}

// ---------- GenerateGapAnalysisReport ----------

func TestGenerateGapAnalysisReport_NotFound(t *testing.T) {
	g := NewReportGenerator()
	_, err := g.GenerateGapAnalysisReport(context.Background(), "missing", "title")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGenerateGapAnalysisReport(t *testing.T) {
	g := NewReportGenerator()

	c1 := makeControl("C-1", "AC Policy", controls.CategoryAccessControl, controls.SeverityCritical)
	c2 := makeControl("C-2", "Logging", controls.CategoryAuditLogging, controls.SeverityHigh)
	c3 := makeControl("C-3", "Encryption", controls.CategoryDataProtection, controls.SeverityMedium)
	g.AddControl(c1)
	g.AddControl(c2)
	g.AddControl(c3)

	r1 := makeResult("C-1", controls.StatusNonCompliant, 0.1)
	r2 := makeResult("C-2", controls.StatusPartial, 0.5)
	r3 := makeResult("C-3", controls.StatusCompliant, 1.0)
	g.AddAssessment("a1", makeAssessment(r1, r2, r3))

	rpt, err := g.GenerateGapAnalysisReport(context.Background(), "a1", "Gap Report")
	if err != nil {
		t.Fatal(err)
	}

	if rpt.Type != ReportTypeGapAnalysis {
		t.Errorf("expected type gap_analysis, got %q", rpt.Type)
	}

	ga := rpt.GapAnalysis
	if ga == nil {
		t.Fatal("expected gap analysis to be non-nil")
	}

	if ga.TotalGaps != 2 {
		t.Errorf("expected 2 gaps, got %d", ga.TotalGaps)
	}
	if ga.CriticalGaps != 1 {
		t.Errorf("expected 1 critical gap, got %d", ga.CriticalGaps)
	}
	if ga.HighPriorityGaps != 1 {
		t.Errorf("expected 1 high priority gap, got %d", ga.HighPriorityGaps)
	}

	// Categories
	if len(ga.Categories) != 3 {
		t.Errorf("expected 3 categories, got %d", len(ga.Categories))
	}

	acCat := ga.Categories[string(controls.CategoryAccessControl)]
	if acCat == nil {
		t.Fatal("expected access_control category")
	}
	if acCat.ComplianceRate != 0.0 {
		t.Errorf("expected 0%% compliance for access_control, got %.2f", acCat.ComplianceRate)
	}

	dpCat := ga.Categories[string(controls.CategoryDataProtection)]
	if dpCat == nil {
		t.Fatal("expected data_protection category")
	}
	if dpCat.ComplianceRate != 1.0 {
		t.Errorf("expected 100%% compliance for data_protection, got %.2f", dpCat.ComplianceRate)
	}

	// Recommendations should include critical and high mentions
	recsText := strings.Join(ga.Recommendations, " ")
	if !strings.Contains(recsText, "critical") {
		t.Error("expected recommendations to mention critical gaps")
	}
	if !strings.Contains(recsText, "high priority") {
		t.Error("expected recommendations to mention high priority gaps")
	}

	// Gap detail
	for _, gap := range ga.Gaps {
		if gap.TargetState != "compliant" {
			t.Errorf("expected target state 'compliant', got %q", gap.TargetState)
		}
		if len(gap.Recommendations) == 0 {
			t.Errorf("expected non-empty recommendations for gap %s", gap.ControlID)
		}
		if gap.EstimatedEffort == "" {
			t.Errorf("expected non-empty effort estimate for gap %s", gap.ControlID)
		}
	}
}

// ---------- generateRecommendations ----------

func TestGenerateRecommendations_AllCategories(t *testing.T) {
	g := NewReportGenerator()

	categories := []controls.ControlCategory{
		controls.CategoryAccessControl,
		controls.CategoryAuditLogging,
		controls.CategoryDataProtection,
		controls.CategoryIncidentResponse,
		controls.CategoryRiskManagement, // default case
	}

	for _, cat := range categories {
		item := &ControlReportItem{Category: cat}
		recs := g.generateRecommendations(item)
		if len(recs) == 0 {
			t.Errorf("expected recommendations for category %q", cat)
		}
	}
}

// ---------- estimateEffort ----------

func TestEstimateEffort(t *testing.T) {
	g := NewReportGenerator()

	tests := []struct {
		sev  controls.Severity
		want string
	}{
		{controls.SeverityCritical, "1-2 weeks"},
		{controls.SeverityHigh, "2-4 weeks"},
		{controls.SeverityMedium, "1-2 months"},
		{controls.SeverityLow, "2-3 months"},
		{controls.SeverityInfo, "To be determined"},
	}

	for _, tt := range tests {
		got := g.estimateEffort(tt.sev)
		if got != tt.want {
			t.Errorf("estimateEffort(%q) = %q, want %q", tt.sev, got, tt.want)
		}
	}
}

// ---------- generateOverallRecommendations ----------

func TestGenerateOverallRecommendations_WeakCategories(t *testing.T) {
	g := NewReportGenerator()
	ga := &GapAnalysis{
		Categories: map[string]*CategoryGap{
			"weak": {ComplianceRate: 0.3},
			"ok":   {ComplianceRate: 0.8},
		},
	}
	recs := g.generateOverallRecommendations(ga)
	joined := strings.Join(recs, " ")
	if !strings.Contains(joined, "weak") {
		t.Error("expected recommendations to mention weak category")
	}
	// Always includes the standard recs
	if !strings.Contains(joined, "compliance review cadence") {
		t.Error("expected standard recommendation about review cadence")
	}
	if !strings.Contains(joined, "continuous monitoring") {
		t.Error("expected standard recommendation about continuous monitoring")
	}
}

// ---------- ExporterRegistry ----------

func TestNewExporterRegistry_DefaultExporters(t *testing.T) {
	reg := NewExporterRegistry()

	formats := []ExportFormat{FormatJSON, FormatHTML, FormatCSV, FormatMarkdown}
	for _, f := range formats {
		exp, ok := reg.Get(f)
		if !ok {
			t.Errorf("expected default exporter for format %q", f)
		}
		if exp.Format() != f {
			t.Errorf("exporter format mismatch: got %q, want %q", exp.Format(), f)
		}
	}

	// PDF not registered by default
	_, ok := reg.Get(FormatPDF)
	if ok {
		t.Error("did not expect PDF exporter by default")
	}
}

func TestExporterRegistry_Register(t *testing.T) {
	reg := NewExporterRegistry()

	// Custom exporter overrides existing
	custom := &JSONExporter{} // reuse type, just testing registration
	reg.Register(custom)

	exp, ok := reg.Get(FormatJSON)
	if !ok || exp != custom {
		t.Error("expected custom exporter to override default")
	}
}

func TestExporterRegistry_ExportUnsupported(t *testing.T) {
	reg := NewExporterRegistry()
	err := reg.Export(context.Background(), &Report{}, FormatPDF, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "unsupported export format") {
		t.Fatalf("expected unsupported format error, got %v", err)
	}
}

// ---------- JSONExporter ----------

func TestJSONExporter(t *testing.T) {
	rpt := &Report{
		ID:       "RPT-1",
		Type:     ReportTypeAssessment,
		Title:    "JSON Test",
		Standard: "STD",
		Version:  "1.0",
		Metadata: map[string]interface{}{"key": "value"},
	}

	var buf bytes.Buffer
	exp := NewJSONExporter()
	if exp.Format() != FormatJSON {
		t.Errorf("expected format json, got %q", exp.Format())
	}

	err := exp.Export(context.Background(), rpt, &buf)
	if err != nil {
		t.Fatal(err)
	}

	var decoded Report
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if decoded.Title != "JSON Test" {
		t.Errorf("expected title 'JSON Test', got %q", decoded.Title)
	}
}

// ---------- HTMLExporter ----------

func TestHTMLExporter(t *testing.T) {
	rpt := &Report{
		ID:          "RPT-1",
		Type:        ReportTypeAssessment,
		Title:       "HTML Test",
		Standard:    "STD",
		Version:     "1.0",
		GeneratedAt: time.Now(),
		Period: ReportPeriod{
			Start: time.Now().AddDate(0, -1, 0),
			End:   time.Now(),
		},
		Summary: ReportSummary{
			TotalControls:  5,
			Compliant:      3,
			NonCompliant:   1,
			Partial:        1,
			ComplianceRate: 0.75,
			RiskScore:      15.0,
		},
		ControlResults: []*ControlReportItem{
			{
				ControlID:   "C-1",
				ControlName: "Access Control",
				Status:      controls.StatusCompliant,
				Score:       1.0,
			},
		},
		Findings: []*FindingReportItem{
			{
				FindingID:   "F-1",
				Title:       "Test Finding",
				ControlID:   "C-1",
				Severity:    controls.SeverityHigh,
				Description: "desc",
				Remediation: "fix",
			},
		},
		Metadata: map[string]interface{}{},
	}

	var buf bytes.Buffer
	exp := NewHTMLExporter()
	if exp.Format() != FormatHTML {
		t.Errorf("expected format html, got %q", exp.Format())
	}

	err := exp.Export(context.Background(), rpt, &buf)
	if err != nil {
		t.Fatal(err)
	}

	html := buf.String()
	if !strings.Contains(html, "<title>HTML Test</title>") {
		t.Error("expected title in HTML output")
	}
	if !strings.Contains(html, "Access Control") {
		t.Error("expected control name in HTML output")
	}
	if !strings.Contains(html, "Test Finding") {
		t.Error("expected finding title in HTML output")
	}
}

func TestHTMLExporter_WithGapAnalysis(t *testing.T) {
	rpt := &Report{
		Title:       "Gap HTML",
		GeneratedAt: time.Now(),
		Period:      ReportPeriod{Start: time.Now(), End: time.Now()},
		Summary:     ReportSummary{},
		GapAnalysis: &GapAnalysis{
			TotalGaps:       3,
			CriticalGaps:    1,
			HighPriorityGaps: 2,
			Recommendations: []string{"Fix critical gaps", "Review policies"},
		},
		Metadata: map[string]interface{}{},
	}

	var buf bytes.Buffer
	err := NewHTMLExporter().Export(context.Background(), rpt, &buf)
	if err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if !strings.Contains(html, "Gap Analysis") {
		t.Error("expected Gap Analysis section in HTML")
	}
	if !strings.Contains(html, "Fix critical gaps") {
		t.Error("expected recommendation in HTML")
	}
}

// ---------- CSVExporter ----------

func TestCSVExporter(t *testing.T) {
	rpt := &Report{
		ControlResults: []*ControlReportItem{
			{
				ControlID:     "C-1",
				ControlName:   "Access Control",
				Category:      controls.CategoryAccessControl,
				Status:        controls.StatusCompliant,
				Score:         0.95,
				Severity:      controls.SeverityHigh,
				FindingsCount: 2,
				TestedAt:      time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
				Notes:         "all good",
			},
			{
				ControlID:     "C-2",
				ControlName:   "Data, \"Protection\"",
				Category:      controls.CategoryDataProtection,
				Status:        controls.StatusNonCompliant,
				Score:         0.30,
				Severity:      controls.SeverityCritical,
				FindingsCount: 5,
				TestedAt:      time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
				Notes:         "needs work, urgent",
			},
		},
	}

	var buf bytes.Buffer
	exp := NewCSVExporter()
	if exp.Format() != FormatCSV {
		t.Errorf("expected format csv, got %q", exp.Format())
	}

	err := exp.Export(context.Background(), rpt, &buf)
	if err != nil {
		t.Fatal(err)
	}

	csv := buf.String()
	lines := strings.Split(strings.TrimSpace(csv), "\n")
	if len(lines) != 3 { // header + 2 rows
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}

	// Header
	if !strings.HasPrefix(lines[0], "Control ID,") {
		t.Error("expected CSV header")
	}

	// First row should contain C-1
	if !strings.Contains(lines[1], "C-1") {
		t.Error("expected C-1 in first data row")
	}

	// Second row should have escaped values (comma and quotes in name)
	if !strings.Contains(lines[2], "\"Data, \"\"Protection\"\"\"") {
		t.Errorf("expected properly escaped CSV name, got line: %s", lines[2])
	}
	// Notes with comma should also be escaped
	if !strings.Contains(lines[2], "\"needs work, urgent\"") {
		t.Errorf("expected properly escaped CSV notes, got line: %s", lines[2])
	}
}

// ---------- MarkdownExporter ----------

func TestMarkdownExporter(t *testing.T) {
	rpt := &Report{
		Title:       "MD Report",
		Standard:    "TEST",
		Version:     "2.0",
		GeneratedAt: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		Period: ReportPeriod{
			Start: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		},
		Summary: ReportSummary{
			TotalControls:    10,
			Compliant:        7,
			NonCompliant:     2,
			Partial:          1,
			ComplianceRate:   0.7,
			RiskScore:        25.0,
			CriticalFindings: 1,
			HighFindings:     2,
			MediumFindings:   3,
			LowFindings:      4,
		},
		ControlResults: []*ControlReportItem{
			{ControlID: "C-1", ControlName: "Short Name", Status: controls.StatusCompliant, Score: 1.0, FindingsCount: 0},
		},
		Findings: []*FindingReportItem{
			{
				Title:       "Critical Issue",
				ControlID:   "C-2",
				Severity:    controls.SeverityCritical,
				Description: "bad thing",
				Remediation: "fix it",
			},
		},
		GapAnalysis: &GapAnalysis{
			TotalGaps:        2,
			CriticalGaps:     1,
			HighPriorityGaps: 1,
			Recommendations:  []string{"Address critical gaps"},
		},
		Metadata: map[string]interface{}{},
	}

	var buf bytes.Buffer
	exp := NewMarkdownExporter()
	if exp.Format() != FormatMarkdown {
		t.Errorf("expected format markdown, got %q", exp.Format())
	}

	err := exp.Export(context.Background(), rpt, &buf)
	if err != nil {
		t.Fatal(err)
	}

	md := buf.String()

	checks := []string{
		"# MD Report",
		"**Standard:** TEST (2.0)",
		"**Compliance Rate:** 70.0%",
		"**Total Controls:** 10",
		"**Risk Score:** 25.0",
		"| Critical | 1 |",
		"## Control Results",
		"C-1",
		"## Findings",
		"Critical Issue",
		"## Gap Analysis",
		"Address critical gaps",
	}

	for _, check := range checks {
		if !strings.Contains(md, check) {
			t.Errorf("expected markdown to contain %q", check)
		}
	}
}

func TestMarkdownExporter_NoFindingsNoGaps(t *testing.T) {
	rpt := &Report{
		Title:       "Empty",
		Standard:    "S",
		Version:     "1",
		GeneratedAt: time.Now(),
		Period:      ReportPeriod{Start: time.Now(), End: time.Now()},
		Summary:     ReportSummary{},
		Metadata:    map[string]interface{}{},
	}

	var buf bytes.Buffer
	err := NewMarkdownExporter().Export(context.Background(), rpt, &buf)
	if err != nil {
		t.Fatal(err)
	}

	md := buf.String()
	// The summary always contains "### Findings Summary", but the detail
	// section "## Findings" (level 2) should not appear when there are none.
	// Count exact "## Findings\n" occurrences (not "### Findings Summary").
	detailCount := strings.Count(md, "\n## Findings\n")
	if detailCount > 0 {
		t.Error("did not expect findings detail section when no findings")
	}
	if strings.Contains(md, "## Gap Analysis") {
		t.Error("did not expect gap analysis section when nil")
	}
}

// ---------- escape helper ----------

func TestEscape(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{"has,comma", "\"has,comma\""},
		{"has\"quote", "\"has\"\"quote\""},
		{"has\nnewline", "\"has\nnewline\""},
		{"", ""},
	}

	for _, tt := range tests {
		got := escape(tt.input)
		if got != tt.want {
			t.Errorf("escape(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ---------- truncate helper ----------

func TestTruncate(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"short", 10, "short"},
		{"exactly10c", 10, "exactly10c"},
		{"this is a longer string", 10, "this is..."},
	}

	for _, tt := range tests {
		got := truncate(tt.input, tt.max)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
		}
	}
}

// ---------- ExporterRegistry.Export integration ----------

func TestExporterRegistry_ExportJSON(t *testing.T) {
	reg := NewExporterRegistry()
	rpt := &Report{
		ID:       "RPT-INT",
		Title:    "Integration",
		Metadata: map[string]interface{}{},
	}

	var buf bytes.Buffer
	err := reg.Export(context.Background(), rpt, FormatJSON, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Integration") {
		t.Error("expected report title in JSON output")
	}
}

// ---------- ReportType constants ----------

func TestReportTypeConstants(t *testing.T) {
	types := map[ReportType]string{
		ReportTypeAssessment:  "assessment",
		ReportTypeGapAnalysis: "gap_analysis",
		ReportTypeRemediation: "remediation",
		ReportTypeExecutive:   "executive",
		ReportTypeAudit:       "audit",
	}
	for rt, want := range types {
		if string(rt) != want {
			t.Errorf("ReportType %v = %q, want %q", rt, string(rt), want)
		}
	}
}

// ---------- ExportFormat constants ----------

func TestExportFormatConstants(t *testing.T) {
	formats := map[ExportFormat]string{
		FormatJSON:     "json",
		FormatPDF:      "pdf",
		FormatHTML:     "html",
		FormatCSV:      "csv",
		FormatMarkdown: "markdown",
	}
	for f, want := range formats {
		if string(f) != want {
			t.Errorf("ExportFormat %v = %q, want %q", f, string(f), want)
		}
	}
}
