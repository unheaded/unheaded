// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

/*
 * This file is part of the Unheaded distributed system platform.
 *
 * Unheaded is licensed under the MIT License.
 * See the LICENSE file in the root directory for the full license text.
 *
 * For protocol specifications, see LICENSE-PROTOCOLS.
 * For GPL 2.0-licensed components (DOOM engine), see doom/LICENSE.
 */

// Package report provides compliance reporting capabilities.
package report

import (
	"context"
	"fmt"
	"sort"
	"time"

	"unheaded/pkg/compliance/controls"
	"unheaded/pkg/compliance/standards"
)

// ReportType represents the type of compliance report.
type ReportType string

const (
	ReportTypeAssessment  ReportType = "assessment"
	ReportTypeGapAnalysis ReportType = "gap_analysis"
	ReportTypeRemediation ReportType = "remediation"
	ReportTypeExecutive   ReportType = "executive"
	ReportTypeAudit       ReportType = "audit"
)

// Report represents a compliance report.
type Report struct {
	ID             string                   `json:"id"`
	Type           ReportType               `json:"type"`
	Title          string                   `json:"title"`
	Standard       string                   `json:"standard"`
	Version        string                   `json:"version"`
	GeneratedAt    time.Time                `json:"generated_at"`
	GeneratedBy    string                   `json:"generated_by"`
	Period         ReportPeriod             `json:"period"`
	Summary        ReportSummary            `json:"summary"`
	ControlResults []*ControlReportItem     `json:"control_results"`
	Findings       []*FindingReportItem     `json:"findings"`
	Remediations   []*RemediationReportItem `json:"remediations"`
	GapAnalysis    *GapAnalysis             `json:"gap_analysis,omitempty"`
	Metadata       map[string]interface{}   `json:"metadata"`
}

// ReportPeriod represents the time period covered by the report.
type ReportPeriod struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// ReportSummary provides a high-level summary of compliance status.
type ReportSummary struct {
	TotalControls       int     `json:"total_controls"`
	Compliant           int     `json:"compliant"`
	NonCompliant        int     `json:"non_compliant"`
	Partial             int     `json:"partial"`
	NotAssessed         int     `json:"not_assessed"`
	NotApplicable       int     `json:"not_applicable"`
	ComplianceRate      float64 `json:"compliance_rate"`
	RiskScore           float64 `json:"risk_score"`
	CriticalFindings    int     `json:"critical_findings"`
	HighFindings        int     `json:"high_findings"`
	MediumFindings      int     `json:"medium_findings"`
	LowFindings         int     `json:"low_findings"`
	OpenRemediations    int     `json:"open_remediations"`
	OverdueRemediations int     `json:"overdue_remediations"`
}

// ControlReportItem represents a control result in the report.
type ControlReportItem struct {
	ControlID     string                   `json:"control_id"`
	ControlName   string                   `json:"control_name"`
	Category      controls.ControlCategory `json:"category"`
	Status        controls.ControlStatus   `json:"status"`
	Score         float64                  `json:"score"`
	Severity      controls.Severity        `json:"severity"`
	FindingsCount int                      `json:"findings_count"`
	EvidenceCount int                      `json:"evidence_count"`
	TestedAt      time.Time                `json:"tested_at"`
	Notes         string                   `json:"notes,omitempty"`
}

// FindingReportItem represents a finding in the report.
type FindingReportItem struct {
	FindingID   string            `json:"finding_id"`
	ControlID   string            `json:"control_id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Severity    controls.Severity `json:"severity"`
	Resource    string            `json:"resource"`
	Remediation string            `json:"remediation"`
	Status      string            `json:"status"`
	DetectedAt  time.Time         `json:"detected_at"`
}

// RemediationReportItem represents a remediation item in the report.
type RemediationReportItem struct {
	ID          string                     `json:"id"`
	ControlID   string                     `json:"control_id"`
	FindingID   string                     `json:"finding_id"`
	Title       string                     `json:"title"`
	Status      controls.RemediationStatus `json:"status"`
	Priority    controls.Severity          `json:"priority"`
	Owner       string                     `json:"owner"`
	DueDate     time.Time                  `json:"due_date"`
	IsOverdue   bool                       `json:"is_overdue"`
	DaysOverdue int                        `json:"days_overdue,omitempty"`
}

// GapAnalysis provides gap analysis results.
type GapAnalysis struct {
	Gaps             []*Gap                  `json:"gaps"`
	Categories       map[string]*CategoryGap `json:"categories"`
	TotalGaps        int                     `json:"total_gaps"`
	CriticalGaps     int                     `json:"critical_gaps"`
	HighPriorityGaps int                     `json:"high_priority_gaps"`
	Recommendations  []string                `json:"recommendations"`
}

// Gap represents a compliance gap.
type Gap struct {
	ControlID       string                   `json:"control_id"`
	ControlName     string                   `json:"control_name"`
	Category        controls.ControlCategory `json:"category"`
	Severity        controls.Severity        `json:"severity"`
	CurrentState    string                   `json:"current_state"`
	TargetState     string                   `json:"target_state"`
	GapDescription  string                   `json:"gap_description"`
	Recommendations []string                 `json:"recommendations"`
	EstimatedEffort string                   `json:"estimated_effort"`
}

// CategoryGap represents gaps by category.
type CategoryGap struct {
	Category       controls.ControlCategory `json:"category"`
	TotalControls  int                      `json:"total_controls"`
	Compliant      int                      `json:"compliant"`
	Gaps           int                      `json:"gaps"`
	ComplianceRate float64                  `json:"compliance_rate"`
}

// ReportGenerator generates compliance reports.
type ReportGenerator struct {
	assessments map[string]*standards.AssessmentResult
	controls    map[string]*controls.Control
	evidence    map[string][]*controls.Evidence
}

// NewReportGenerator creates a new report generator.
func NewReportGenerator() *ReportGenerator {
	return &ReportGenerator{
		assessments: make(map[string]*standards.AssessmentResult),
		controls:    make(map[string]*controls.Control),
		evidence:    make(map[string][]*controls.Evidence),
	}
}

// AddAssessment adds an assessment result to the generator.
func (g *ReportGenerator) AddAssessment(id string, result *standards.AssessmentResult) {
	g.assessments[id] = result
}

// AddControl adds a control definition.
func (g *ReportGenerator) AddControl(c *controls.Control) {
	g.controls[c.ID] = c
}

// AddEvidence adds evidence for a control.
func (g *ReportGenerator) AddEvidence(controlID string, ev []*controls.Evidence) {
	g.evidence[controlID] = append(g.evidence[controlID], ev...)
}

// GenerateAssessmentReport generates an assessment report.
func (g *ReportGenerator) GenerateAssessmentReport(ctx context.Context, assessmentID, title string) (*Report, error) {
	assessment, ok := g.assessments[assessmentID]
	if !ok {
		return nil, fmt.Errorf("assessment not found: %s", assessmentID)
	}

	report := &Report{
		ID:          fmt.Sprintf("RPT-%d", time.Now().UnixNano()),
		Type:        ReportTypeAssessment,
		Title:       title,
		Standard:    assessment.Standard,
		Version:     assessment.Version,
		GeneratedAt: time.Now(),
		GeneratedBy: "compliance-engine",
		Period: ReportPeriod{
			Start: time.Now().AddDate(0, -1, 0),
			End:   time.Now(),
		},
		Metadata: make(map[string]interface{}),
	}

	// Build summary
	summary := ReportSummary{
		TotalControls:  assessment.TotalControls,
		Compliant:      assessment.Compliant,
		NonCompliant:   assessment.NonCompliant,
		Partial:        assessment.Partial,
		NotAssessed:    assessment.NotAssessed,
		NotApplicable:  assessment.NotApplicable,
		ComplianceRate: assessment.ComplianceRate,
	}

	// Build control results and findings
	var controlItems []*ControlReportItem
	var findingItems []*FindingReportItem

	for _, result := range assessment.Results {
		ctrl := g.controls[result.ControlID]
		if ctrl == nil {
			continue
		}

		item := &ControlReportItem{
			ControlID:     result.ControlID,
			ControlName:   ctrl.Name,
			Category:      ctrl.Category,
			Status:        result.Status,
			Score:         result.Score,
			Severity:      ctrl.Severity,
			FindingsCount: len(result.Findings),
			EvidenceCount: len(g.evidence[result.ControlID]),
			TestedAt:      result.TestedAt,
			Notes:         result.Notes,
		}
		controlItems = append(controlItems, item)

		// Add findings
		for _, finding := range result.Findings {
			findingItem := &FindingReportItem{
				FindingID:   finding.ID,
				ControlID:   result.ControlID,
				Title:       finding.Title,
				Description: finding.Description,
				Severity:    finding.Severity,
				Resource:    finding.Resource,
				Remediation: finding.Remediation,
				Status:      "open",
				DetectedAt:  finding.DetectedAt,
			}
			findingItems = append(findingItems, findingItem)

			// Update summary counts
			switch finding.Severity {
			case controls.SeverityCritical:
				summary.CriticalFindings++
			case controls.SeverityHigh:
				summary.HighFindings++
			case controls.SeverityMedium:
				summary.MediumFindings++
			case controls.SeverityLow:
				summary.LowFindings++
			}
		}
	}

	// Calculate risk score
	summary.RiskScore = g.calculateRiskScore(summary)

	report.Summary = summary
	report.ControlResults = controlItems
	report.Findings = findingItems

	return report, nil
}

// GenerateGapAnalysisReport generates a gap analysis report.
func (g *ReportGenerator) GenerateGapAnalysisReport(ctx context.Context, assessmentID, title string) (*Report, error) {
	report, err := g.GenerateAssessmentReport(ctx, assessmentID, title)
	if err != nil {
		return nil, err
	}

	report.Type = ReportTypeGapAnalysis

	// Build gap analysis
	gapAnalysis := &GapAnalysis{
		Categories: make(map[string]*CategoryGap),
	}

	categoryStats := make(map[controls.ControlCategory]*CategoryGap)

	for _, item := range report.ControlResults {
		// Initialize category if needed
		if _, ok := categoryStats[item.Category]; !ok {
			categoryStats[item.Category] = &CategoryGap{
				Category: item.Category,
			}
		}
		categoryStats[item.Category].TotalControls++

		switch item.Status {
		case controls.StatusCompliant:
			categoryStats[item.Category].Compliant++
		case controls.StatusNonCompliant, controls.StatusPartial:
			categoryStats[item.Category].Gaps++

			gap := &Gap{
				ControlID:      item.ControlID,
				ControlName:    item.ControlName,
				Category:       item.Category,
				Severity:       item.Severity,
				CurrentState:   string(item.Status),
				TargetState:    "compliant",
				GapDescription: fmt.Sprintf("Control %s is %s", item.ControlID, item.Status),
			}

			// Add recommendations based on severity
			gap.Recommendations = g.generateRecommendations(item)
			gap.EstimatedEffort = g.estimateEffort(item.Severity)

			gapAnalysis.Gaps = append(gapAnalysis.Gaps, gap)
			gapAnalysis.TotalGaps++

			switch item.Severity {
			case controls.SeverityCritical:
				gapAnalysis.CriticalGaps++
			case controls.SeverityHigh:
				gapAnalysis.HighPriorityGaps++
			}
		}
	}

	// Calculate category compliance rates
	for cat, stats := range categoryStats {
		if stats.TotalControls > 0 {
			stats.ComplianceRate = float64(stats.Compliant) / float64(stats.TotalControls)
		}
		gapAnalysis.Categories[string(cat)] = stats
	}

	// Generate overall recommendations
	gapAnalysis.Recommendations = g.generateOverallRecommendations(gapAnalysis)

	report.GapAnalysis = gapAnalysis

	return report, nil
}

func (g *ReportGenerator) calculateRiskScore(summary ReportSummary) float64 {
	// Risk score is a weighted calculation based on findings
	score := 0.0
	score += float64(summary.CriticalFindings) * 10.0
	score += float64(summary.HighFindings) * 5.0
	score += float64(summary.MediumFindings) * 2.0
	score += float64(summary.LowFindings) * 0.5

	// Normalize to 0-100 scale
	if score > 100 {
		score = 100
	}

	return score
}

func (g *ReportGenerator) generateRecommendations(item *ControlReportItem) []string {
	var recs []string

	switch item.Category {
	case controls.CategoryAccessControl:
		recs = append(recs, "Review and update access control policies")
		recs = append(recs, "Implement role-based access control")
	case controls.CategoryAuditLogging:
		recs = append(recs, "Enable comprehensive audit logging")
		recs = append(recs, "Implement log retention policies")
	case controls.CategoryDataProtection:
		recs = append(recs, "Implement encryption for data at rest and in transit")
		recs = append(recs, "Review data classification procedures")
	case controls.CategoryIncidentResponse:
		recs = append(recs, "Develop incident response procedures")
		recs = append(recs, "Conduct incident response training")
	default:
		recs = append(recs, "Review and address control requirements")
	}

	return recs
}

func (g *ReportGenerator) estimateEffort(severity controls.Severity) string {
	switch severity {
	case controls.SeverityCritical:
		return "1-2 weeks"
	case controls.SeverityHigh:
		return "2-4 weeks"
	case controls.SeverityMedium:
		return "1-2 months"
	case controls.SeverityLow:
		return "2-3 months"
	default:
		return "To be determined"
	}
}

func (g *ReportGenerator) generateOverallRecommendations(analysis *GapAnalysis) []string {
	var recs []string

	if analysis.CriticalGaps > 0 {
		recs = append(recs, fmt.Sprintf("Prioritize remediation of %d critical gaps immediately", analysis.CriticalGaps))
	}

	if analysis.HighPriorityGaps > 0 {
		recs = append(recs, fmt.Sprintf("Address %d high priority gaps within 30 days", analysis.HighPriorityGaps))
	}

	// Identify weakest categories
	var weakCategories []string
	for name, cat := range analysis.Categories {
		if cat.ComplianceRate < 0.5 {
			weakCategories = append(weakCategories, name)
		}
	}

	if len(weakCategories) > 0 {
		sort.Strings(weakCategories)
		recs = append(recs, fmt.Sprintf("Focus on improving controls in: %v", weakCategories))
	}

	recs = append(recs, "Establish regular compliance review cadence")
	recs = append(recs, "Implement continuous monitoring for automated controls")

	return recs
}
