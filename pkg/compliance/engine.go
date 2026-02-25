// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

/*
 * This file is part of the Unheaded distributed system platform.
 *
 * Unheaded is licensed under the Business Source License 1.1 (BSL 1.1).
 * See the LICENSE file in the root directory for the full license text.
 *
 * For protocol specifications, see LICENSE-PROTOCOLS.
 * For GPL 2.0-licensed components (DOOM engine), see doom/LICENSE.
 */

// Package compliance provides a comprehensive compliance engine for managing
// regulatory compliance across multiple standards including SOC2, NIST, PCI-DSS,
// HIPAA, and GDPR.
package compliance

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"unheaded/pkg/compliance/audit"
	"unheaded/pkg/compliance/controls"
	"unheaded/pkg/compliance/report"
	"unheaded/pkg/compliance/standards"
)

// Assessment represents a compliance assessment.
type Assessment struct {
	ID             string                     `json:"id"`
	Standard       string                     `json:"standard"`
	Version        string                     `json:"version"`
	Status         AssessmentStatus           `json:"status"`
	StartedAt      time.Time                  `json:"started_at"`
	CompletedAt    *time.Time                 `json:"completed_at,omitempty"`
	Results        []*controls.ControlResult  `json:"results"`
	Summary        *AssessmentSummary         `json:"summary"`
	Metadata       map[string]interface{}     `json:"metadata"`
}

// AssessmentStatus represents the status of an assessment.
type AssessmentStatus string

const (
	AssessmentPending    AssessmentStatus = "pending"
	AssessmentInProgress AssessmentStatus = "in_progress"
	AssessmentCompleted  AssessmentStatus = "completed"
	AssessmentFailed     AssessmentStatus = "failed"
)

// AssessmentSummary provides a summary of the assessment results.
type AssessmentSummary struct {
	TotalControls  int     `json:"total_controls"`
	Compliant      int     `json:"compliant"`
	NonCompliant   int     `json:"non_compliant"`
	Partial        int     `json:"partial"`
	NotAssessed    int     `json:"not_assessed"`
	NotApplicable  int     `json:"not_applicable"`
	ComplianceRate float64 `json:"compliance_rate"`
	RiskScore      float64 `json:"risk_score"`
}

// Control represents a compliance control exposed by the engine.
type Control = controls.Control

// Report represents a compliance report exposed by the engine.
type Report = report.Report

// Evidence represents compliance evidence exposed by the engine.
type Evidence = controls.Evidence

// ComplianceEngine defines the interface for the compliance engine.
type ComplianceEngine interface {
	// RunAssessment runs a compliance assessment for a standard.
	RunAssessment(ctx context.Context, standard string) (*Assessment, error)

	// GetControl retrieves a control by ID.
	GetControl(ctx context.Context, controlID string) (*Control, error)

	// ListControls lists all controls for a standard.
	ListControls(ctx context.Context, standard string) ([]*Control, error)

	// GenerateReport generates a compliance report for an assessment.
	GenerateReport(ctx context.Context, assessmentID string) (*Report, error)

	// GetEvidence retrieves evidence for a control.
	GetEvidence(ctx context.Context, controlID string) ([]*Evidence, error)

	// GetAssessment retrieves an assessment by ID.
	GetAssessment(ctx context.Context, assessmentID string) (*Assessment, error)

	// ListStandards lists all available compliance standards.
	ListStandards(ctx context.Context) ([]string, error)

	// GetGapAnalysis generates a gap analysis for an assessment.
	GetGapAnalysis(ctx context.Context, assessmentID string) (*report.GapAnalysis, error)
}

// Engine implements the ComplianceEngine interface.
type Engine struct {
	mu              sync.RWMutex
	standardsReg    *standards.StandardRegistry
	checkRegistry   *controls.CheckRegistry
	checkRunner     *controls.CheckRunner
	evidenceStore   *controls.EvidenceStore
	evidenceCollectors *controls.EvidenceCollectorRegistry
	reportGenerator *report.ReportGenerator
	auditLogger     audit.AuditLogger
	auditTrail      *audit.AuditTrail
	auditor         *audit.ComplianceAuditor
	assessments     map[string]*Assessment
}

// EngineConfig holds configuration for the compliance engine.
type EngineConfig struct {
	Concurrency     int
	CheckTimeout    time.Duration
	AuditLogSize    int
	EnableAuditTrail bool
}

// DefaultEngineConfig returns the default engine configuration.
func DefaultEngineConfig() *EngineConfig {
	return &EngineConfig{
		Concurrency:      10,
		CheckTimeout:     30 * time.Second,
		AuditLogSize:     10000,
		EnableAuditTrail: true,
	}
}

// NewEngine creates a new compliance engine with the provided configuration.
func NewEngine(config *EngineConfig) *Engine {
	if config == nil {
		config = DefaultEngineConfig()
	}

	checkRegistry := controls.NewCheckRegistry()
	auditLogger := audit.NewInMemoryAuditLogger(config.AuditLogSize)

	engine := &Engine{
		standardsReg:       standards.NewStandardRegistry(),
		checkRegistry:      checkRegistry,
		checkRunner:        controls.NewCheckRunner(checkRegistry).WithConcurrency(config.Concurrency).WithTimeout(config.CheckTimeout),
		evidenceStore:      controls.NewEvidenceStore(),
		evidenceCollectors: controls.NewEvidenceCollectorRegistry(),
		reportGenerator:    report.NewReportGenerator(),
		auditLogger:        auditLogger,
		auditor:            audit.NewComplianceAuditor(auditLogger),
		assessments:        make(map[string]*Assessment),
	}

	if config.EnableAuditTrail {
		engine.auditTrail = audit.NewAuditTrail()
	}

	// Register default standards
	engine.registerDefaultStandards()

	return engine
}

func (e *Engine) registerDefaultStandards() {
	e.standardsReg.Register(standards.NewSOC2Standard())
	e.standardsReg.Register(standards.NewNISTStandard())
	e.standardsReg.Register(standards.NewPCIStandard())
	e.standardsReg.Register(standards.NewHIPAAStandard())
	e.standardsReg.Register(standards.NewGDPRStandard())
}

// RegisterStandard registers a custom compliance standard.
func (e *Engine) RegisterStandard(s standards.Standard) {
	e.standardsReg.Register(s)
}

// RegisterChecker registers a control checker.
func (e *Engine) RegisterChecker(pattern string, checker controls.Checker) {
	e.checkRegistry.Register(pattern, checker)
}

// RegisterEvidenceCollector registers an evidence collector.
func (e *Engine) RegisterEvidenceCollector(name string, collector controls.EvidenceCollector) {
	e.evidenceCollectors.Register(name, collector)
}

// RunAssessment runs a compliance assessment for the specified standard.
func (e *Engine) RunAssessment(ctx context.Context, standard string) (*Assessment, error) {
	std, ok := e.standardsReg.Get(standard)
	if !ok {
		return nil, fmt.Errorf("unknown standard: %s", standard)
	}

	assessmentID := fmt.Sprintf("ASMT-%d", time.Now().UnixNano())

	// Log assessment start
	correlationID := fmt.Sprintf("COR-%d", time.Now().UnixNano())
	auditor := e.auditor.WithCorrelation(correlationID).WithActor("system")
	_ = auditor.LogAssessmentStart(ctx, standard, assessmentID)

	assessment := &Assessment{
		ID:        assessmentID,
		Standard:  std.ID(),
		Version:   std.Version(),
		Status:    AssessmentInProgress,
		StartedAt: time.Now(),
		Metadata:  make(map[string]interface{}),
	}

	// Store assessment
	e.mu.Lock()
	e.assessments[assessmentID] = assessment
	e.mu.Unlock()

	// Get all controls for the standard
	allControls := std.Controls()

	// Add controls to report generator
	for _, ctrl := range allControls {
		e.reportGenerator.AddControl(ctrl)
	}

	// Run checks
	results, err := e.checkRunner.RunChecks(ctx, allControls)
	if err != nil {
		assessment.Status = AssessmentFailed
		_ = auditor.LogError(ctx, "assessment", assessmentID, err)
		return assessment, err
	}

	// Update assessment with results
	assessment.Results = results

	// Calculate summary
	summary := e.calculateSummary(results)
	assessment.Summary = summary

	// Collect evidence for each control
	for _, ctrl := range allControls {
		evidence, err := e.evidenceCollectors.CollectAll(ctx, ctrl)
		if err == nil && len(evidence) > 0 {
			for _, ev := range evidence {
				_ = e.evidenceStore.Store(ctx, ev)
				_ = auditor.LogEvidenceCollection(ctx, ctrl.ID, ev.ID, string(ev.Type))
			}
			e.reportGenerator.AddEvidence(ctrl.ID, evidence)
		}
	}

	// Log control checks
	for _, result := range results {
		if result != nil {
			_ = auditor.LogControlCheck(ctx, result.ControlID, string(result.Status), result.Score)
		}
	}

	// Mark assessment as completed
	now := time.Now()
	assessment.CompletedAt = &now
	assessment.Status = AssessmentCompleted

	// Store assessment result for report generation
	assessmentResult := &standards.AssessmentResult{
		Standard:       std.ID(),
		Version:        std.Version(),
		TotalControls:  summary.TotalControls,
		Compliant:      summary.Compliant,
		NonCompliant:   summary.NonCompliant,
		Partial:        summary.Partial,
		NotAssessed:    summary.NotAssessed,
		NotApplicable:  summary.NotApplicable,
		ComplianceRate: summary.ComplianceRate,
		Results:        results,
	}
	e.reportGenerator.AddAssessment(assessmentID, assessmentResult)

	// Log completion
	_ = auditor.LogAssessmentComplete(ctx, assessmentID, map[string]interface{}{
		"compliance_rate": summary.ComplianceRate,
		"total_controls":  summary.TotalControls,
		"compliant":       summary.Compliant,
		"non_compliant":   summary.NonCompliant,
	})

	// Add to audit trail if enabled
	if e.auditTrail != nil {
		event := audit.NewAuditEvent(audit.EventAssessmentCompleted).
			WithActor("system").
			WithResource("assessment", assessmentID).
			WithDescription(fmt.Sprintf("Completed assessment for %s", standard)).
			WithMetadata("compliance_rate", summary.ComplianceRate).
			Build()
		_, _ = e.auditTrail.Append(ctx, event)
	}

	return assessment, nil
}

// GetControl retrieves a control by ID.
func (e *Engine) GetControl(ctx context.Context, controlID string) (*Control, error) {
	// Search across all standards
	for _, std := range e.standardsReg.List() {
		if ctrl, ok := std.GetControl(controlID); ok {
			return ctrl, nil
		}
	}
	return nil, fmt.Errorf("control not found: %s", controlID)
}

// ListControls lists all controls for a standard.
func (e *Engine) ListControls(ctx context.Context, standard string) ([]*Control, error) {
	std, ok := e.standardsReg.Get(standard)
	if !ok {
		return nil, fmt.Errorf("unknown standard: %s", standard)
	}
	return std.Controls(), nil
}

// GenerateReport generates a compliance report for an assessment.
func (e *Engine) GenerateReport(ctx context.Context, assessmentID string) (*Report, error) {
	e.mu.RLock()
	assessment, ok := e.assessments[assessmentID]
	e.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("assessment not found: %s", assessmentID)
	}

	title := fmt.Sprintf("%s Compliance Assessment Report", assessment.Standard)
	rpt, err := e.reportGenerator.GenerateAssessmentReport(ctx, assessmentID, title)
	if err != nil {
		return nil, err
	}

	// Log report generation
	auditor := e.auditor.WithActor("system")
	_ = auditor.LogReportGeneration(ctx, rpt.ID, string(rpt.Type))

	return rpt, nil
}

// GetEvidence retrieves evidence for a control.
func (e *Engine) GetEvidence(ctx context.Context, controlID string) ([]*Evidence, error) {
	return e.evidenceStore.GetByControl(ctx, controlID)
}

// GetAssessment retrieves an assessment by ID.
func (e *Engine) GetAssessment(ctx context.Context, assessmentID string) (*Assessment, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	assessment, ok := e.assessments[assessmentID]
	if !ok {
		return nil, fmt.Errorf("assessment not found: %s", assessmentID)
	}

	return assessment, nil
}

// ListStandards lists all available compliance standards.
func (e *Engine) ListStandards(ctx context.Context) ([]string, error) {
	stds := e.standardsReg.List()
	result := make([]string, len(stds))
	for i, s := range stds {
		result[i] = s.ID()
	}
	return result, nil
}

// GetGapAnalysis generates a gap analysis for an assessment.
func (e *Engine) GetGapAnalysis(ctx context.Context, assessmentID string) (*report.GapAnalysis, error) {
	e.mu.RLock()
	assessment, ok := e.assessments[assessmentID]
	e.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("assessment not found: %s", assessmentID)
	}

	title := fmt.Sprintf("%s Gap Analysis Report", assessment.Standard)
	rpt, err := e.reportGenerator.GenerateGapAnalysisReport(ctx, assessmentID, title)
	if err != nil {
		return nil, err
	}

	return rpt.GapAnalysis, nil
}

// ExportReport exports a report in the specified format.
func (e *Engine) ExportReport(ctx context.Context, assessmentID string, format report.ExportFormat) ([]byte, error) {
	rpt, err := e.GenerateReport(ctx, assessmentID)
	if err != nil {
		return nil, err
	}

	registry := report.NewExporterRegistry()

	var buf bytes.Buffer
	if err := registry.Export(ctx, rpt, format, &buf); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// GetAuditTrail returns the audit trail.
func (e *Engine) GetAuditTrail() *audit.AuditTrail {
	return e.auditTrail
}

// GetAuditLogger returns the audit logger.
func (e *Engine) GetAuditLogger() audit.AuditLogger {
	return e.auditLogger
}

// VerifyAuditTrail verifies the integrity of the audit trail.
func (e *Engine) VerifyAuditTrail() (*audit.VerificationResult, error) {
	if e.auditTrail == nil {
		return nil, fmt.Errorf("audit trail not enabled")
	}
	return e.auditTrail.Verify()
}

func (e *Engine) calculateSummary(results []*controls.ControlResult) *AssessmentSummary {
	summary := &AssessmentSummary{
		TotalControls: len(results),
	}

	criticalFindings := 0
	highFindings := 0

	for _, r := range results {
		if r == nil {
			summary.NotAssessed++
			continue
		}
		switch r.Status {
		case controls.StatusCompliant:
			summary.Compliant++
		case controls.StatusNonCompliant:
			summary.NonCompliant++
		case controls.StatusPartial:
			summary.Partial++
		case controls.StatusNotAssessed:
			summary.NotAssessed++
		case controls.StatusNotApplicable:
			summary.NotApplicable++
		}

		// Count findings by severity
		for _, f := range r.Findings {
			switch f.Severity {
			case controls.SeverityCritical:
				criticalFindings++
			case controls.SeverityHigh:
				highFindings++
			}
		}
	}

	// Calculate compliance rate
	assessable := summary.TotalControls - summary.NotApplicable - summary.NotAssessed
	if assessable > 0 {
		summary.ComplianceRate = float64(summary.Compliant) / float64(assessable)
	}

	// Calculate risk score
	summary.RiskScore = float64(criticalFindings)*10.0 + float64(highFindings)*5.0 +
		float64(summary.NonCompliant)*3.0 + float64(summary.Partial)*1.0

	if summary.RiskScore > 100 {
		summary.RiskScore = 100
	}

	return summary
}

