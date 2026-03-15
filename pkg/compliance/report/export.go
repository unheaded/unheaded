// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

/*
 * This file is part of the Unheaded distributed system platform.
 *
 * Unheaded is licensed under the MIT License.
 * See the LICENSE file in the root directory for the full license text.
 *
 * For protocol specifications, see LICENSE-PROTOCOLS.
 * For GPL 2.0-licensed components (DOOM engine), see doom/LICENSE.
 */

package report

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/template"
	"time"
)

// ExportFormat represents the export format for reports.
type ExportFormat string

const (
	FormatJSON     ExportFormat = "json"
	FormatPDF      ExportFormat = "pdf"
	FormatHTML     ExportFormat = "html"
	FormatCSV      ExportFormat = "csv"
	FormatMarkdown ExportFormat = "markdown"
)

// Exporter defines the interface for report exporters.
type Exporter interface {
	Export(ctx context.Context, report *Report, w io.Writer) error
	Format() ExportFormat
}

// ExporterRegistry manages report exporters.
type ExporterRegistry struct {
	exporters map[ExportFormat]Exporter
}

// NewExporterRegistry creates a new exporter registry.
func NewExporterRegistry() *ExporterRegistry {
	r := &ExporterRegistry{
		exporters: make(map[ExportFormat]Exporter),
	}

	// Register default exporters
	r.Register(NewJSONExporter())
	r.Register(NewHTMLExporter())
	r.Register(NewCSVExporter())
	r.Register(NewMarkdownExporter())

	return r
}

// Register registers an exporter.
func (r *ExporterRegistry) Register(e Exporter) {
	r.exporters[e.Format()] = e
}

// Get returns an exporter by format.
func (r *ExporterRegistry) Get(format ExportFormat) (Exporter, bool) {
	e, ok := r.exporters[format]
	return e, ok
}

// Export exports a report in the specified format.
func (r *ExporterRegistry) Export(ctx context.Context, report *Report, format ExportFormat, w io.Writer) error {
	exporter, ok := r.Get(format)
	if !ok {
		return fmt.Errorf("unsupported export format: %s", format)
	}
	return exporter.Export(ctx, report, w)
}

// JSONExporter exports reports to JSON format.
type JSONExporter struct{}

// NewJSONExporter creates a new JSON exporter.
func NewJSONExporter() *JSONExporter {
	return &JSONExporter{}
}

func (e *JSONExporter) Format() ExportFormat {
	return FormatJSON
}

func (e *JSONExporter) Export(ctx context.Context, report *Report, w io.Writer) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

// HTMLExporter exports reports to HTML format.
type HTMLExporter struct {
	template *template.Template
}

// NewHTMLExporter creates a new HTML exporter.
func NewHTMLExporter() *HTMLExporter {
	tmpl := template.Must(template.New("report").Funcs(template.FuncMap{
		"formatDate": func(t time.Time) string {
			return t.Format("January 2, 2006")
		},
		"formatPercent": func(f float64) string {
			return fmt.Sprintf("%.1f%%", f*100)
		},
		"multiply": func(a, b float64) float64 {
			return a * b
		},
		"statusClass": func(status string) string {
			switch status {
			case "compliant":
				return "status-compliant"
			case "non_compliant":
				return "status-non-compliant"
			case "partial":
				return "status-partial"
			default:
				return "status-unknown"
			}
		},
	}).Parse(htmlReportTemplate))

	return &HTMLExporter{template: tmpl}
}

func (e *HTMLExporter) Format() ExportFormat {
	return FormatHTML
}

func (e *HTMLExporter) Export(ctx context.Context, report *Report, w io.Writer) error {
	return e.template.Execute(w, report)
}

const htmlReportTemplate = `<!DOCTYPE html>
<html>
<head>
    <title>{{.Title}}</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        .header { border-bottom: 2px solid #333; padding-bottom: 20px; margin-bottom: 30px; }
        .summary { background: #f5f5f5; padding: 20px; border-radius: 8px; margin-bottom: 30px; }
        .summary-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 20px; }
        .summary-item { text-align: center; }
        .summary-value { font-size: 2em; font-weight: bold; }
        .summary-label { color: #666; }
        .status-compliant { color: #28a745; }
        .status-non-compliant { color: #dc3545; }
        .status-partial { color: #ffc107; }
        .status-unknown { color: #6c757d; }
        table { width: 100%; border-collapse: collapse; margin-bottom: 30px; }
        th, td { padding: 12px; text-align: left; border-bottom: 1px solid #ddd; }
        th { background: #333; color: white; }
        tr:hover { background: #f5f5f5; }
        .findings-section { margin-top: 30px; }
        .finding-card { border: 1px solid #ddd; padding: 15px; margin-bottom: 15px; border-radius: 8px; }
        .finding-critical { border-left: 4px solid #dc3545; }
        .finding-high { border-left: 4px solid #fd7e14; }
        .finding-medium { border-left: 4px solid #ffc107; }
        .finding-low { border-left: 4px solid #28a745; }
        .compliance-rate { font-size: 3em; font-weight: bold; color: #333; }
    </style>
</head>
<body>
    <div class="header">
        <h1>{{.Title}}</h1>
        <p>Standard: {{.Standard}} ({{.Version}})</p>
        <p>Generated: {{formatDate .GeneratedAt}}</p>
        <p>Period: {{formatDate .Period.Start}} - {{formatDate .Period.End}}</p>
    </div>

    <div class="summary">
        <h2>Executive Summary</h2>
        <div class="summary-grid">
            <div class="summary-item">
                <div class="summary-value compliance-rate">{{formatPercent .Summary.ComplianceRate}}</div>
                <div class="summary-label">Compliance Rate</div>
            </div>
            <div class="summary-item">
                <div class="summary-value status-compliant">{{.Summary.Compliant}}</div>
                <div class="summary-label">Compliant Controls</div>
            </div>
            <div class="summary-item">
                <div class="summary-value status-non-compliant">{{.Summary.NonCompliant}}</div>
                <div class="summary-label">Non-Compliant Controls</div>
            </div>
            <div class="summary-item">
                <div class="summary-value status-partial">{{.Summary.Partial}}</div>
                <div class="summary-label">Partial Controls</div>
            </div>
        </div>
        <div class="summary-grid" style="margin-top: 20px;">
            <div class="summary-item">
                <div class="summary-value">{{.Summary.TotalControls}}</div>
                <div class="summary-label">Total Controls</div>
            </div>
            <div class="summary-item">
                <div class="summary-value" style="color: #dc3545;">{{.Summary.CriticalFindings}}</div>
                <div class="summary-label">Critical Findings</div>
            </div>
            <div class="summary-item">
                <div class="summary-value" style="color: #fd7e14;">{{.Summary.HighFindings}}</div>
                <div class="summary-label">High Findings</div>
            </div>
            <div class="summary-item">
                <div class="summary-value">{{printf "%.1f" .Summary.RiskScore}}</div>
                <div class="summary-label">Risk Score</div>
            </div>
        </div>
    </div>

    <h2>Control Results</h2>
    <table>
        <thead>
            <tr>
                <th>Control ID</th>
                <th>Name</th>
                <th>Category</th>
                <th>Status</th>
                <th>Score</th>
                <th>Findings</th>
            </tr>
        </thead>
        <tbody>
            {{range .ControlResults}}
            <tr>
                <td>{{.ControlID}}</td>
                <td>{{.ControlName}}</td>
                <td>{{.Category}}</td>
                <td class="{{statusClass (printf "%s" .Status)}}">{{.Status}}</td>
                <td>{{printf "%.0f%%" (multiply .Score 100)}}</td>
                <td>{{.FindingsCount}}</td>
            </tr>
            {{end}}
        </tbody>
    </table>

    {{if .Findings}}
    <div class="findings-section">
        <h2>Findings</h2>
        {{range .Findings}}
        <div class="finding-card finding-{{.Severity}}">
            <h3>{{.Title}}</h3>
            <p><strong>Control:</strong> {{.ControlID}}</p>
            <p><strong>Severity:</strong> {{.Severity}}</p>
            <p><strong>Description:</strong> {{.Description}}</p>
            <p><strong>Remediation:</strong> {{.Remediation}}</p>
        </div>
        {{end}}
    </div>
    {{end}}

    {{if .GapAnalysis}}
    <div class="gap-analysis">
        <h2>Gap Analysis</h2>
        <p>Total Gaps: {{.GapAnalysis.TotalGaps}} | Critical: {{.GapAnalysis.CriticalGaps}} | High Priority: {{.GapAnalysis.HighPriorityGaps}}</p>
        <h3>Recommendations</h3>
        <ul>
            {{range .GapAnalysis.Recommendations}}
            <li>{{.}}</li>
            {{end}}
        </ul>
    </div>
    {{end}}
</body>
</html>`

// CSVExporter exports reports to CSV format.
type CSVExporter struct{}

// NewCSVExporter creates a new CSV exporter.
func NewCSVExporter() *CSVExporter {
	return &CSVExporter{}
}

func (e *CSVExporter) Format() ExportFormat {
	return FormatCSV
}

func (e *CSVExporter) Export(ctx context.Context, report *Report, w io.Writer) error {
	var buf bytes.Buffer

	// Write header
	buf.WriteString("Control ID,Control Name,Category,Status,Score,Severity,Findings Count,Tested At,Notes\n")

	// Write control results
	for _, item := range report.ControlResults {
		line := fmt.Sprintf("%s,%s,%s,%s,%.2f,%s,%d,%s,%s\n",
			escape(item.ControlID),
			escape(item.ControlName),
			escape(string(item.Category)),
			escape(string(item.Status)),
			item.Score,
			escape(string(item.Severity)),
			item.FindingsCount,
			item.TestedAt.Format(time.RFC3339),
			escape(item.Notes),
		)
		buf.WriteString(line)
	}

	_, err := w.Write(buf.Bytes())
	return err
}

func escape(s string) string {
	if strings.ContainsAny(s, ",\"\n") {
		s = strings.ReplaceAll(s, "\"", "\"\"")
		return fmt.Sprintf("\"%s\"", s)
	}
	return s
}

// MarkdownExporter exports reports to Markdown format.
type MarkdownExporter struct{}

// NewMarkdownExporter creates a new Markdown exporter.
func NewMarkdownExporter() *MarkdownExporter {
	return &MarkdownExporter{}
}

func (e *MarkdownExporter) Format() ExportFormat {
	return FormatMarkdown
}

func (e *MarkdownExporter) Export(ctx context.Context, report *Report, w io.Writer) error {
	var buf bytes.Buffer

	// Title and metadata
	buf.WriteString(fmt.Sprintf("# %s\n\n", report.Title))
	buf.WriteString(fmt.Sprintf("**Standard:** %s (%s)\n", report.Standard, report.Version))
	buf.WriteString(fmt.Sprintf("**Generated:** %s\n", report.GeneratedAt.Format("January 2, 2006")))
	buf.WriteString(fmt.Sprintf("**Period:** %s - %s\n\n",
		report.Period.Start.Format("January 2, 2006"),
		report.Period.End.Format("January 2, 2006")))

	// Summary
	buf.WriteString("## Executive Summary\n\n")
	buf.WriteString(fmt.Sprintf("- **Compliance Rate:** %.1f%%\n", report.Summary.ComplianceRate*100))
	buf.WriteString(fmt.Sprintf("- **Total Controls:** %d\n", report.Summary.TotalControls))
	buf.WriteString(fmt.Sprintf("- **Compliant:** %d\n", report.Summary.Compliant))
	buf.WriteString(fmt.Sprintf("- **Non-Compliant:** %d\n", report.Summary.NonCompliant))
	buf.WriteString(fmt.Sprintf("- **Partial:** %d\n", report.Summary.Partial))
	buf.WriteString(fmt.Sprintf("- **Risk Score:** %.1f\n\n", report.Summary.RiskScore))

	buf.WriteString("### Findings Summary\n\n")
	buf.WriteString(fmt.Sprintf("| Severity | Count |\n"))
	buf.WriteString("|----------|-------|\n")
	buf.WriteString(fmt.Sprintf("| Critical | %d |\n", report.Summary.CriticalFindings))
	buf.WriteString(fmt.Sprintf("| High | %d |\n", report.Summary.HighFindings))
	buf.WriteString(fmt.Sprintf("| Medium | %d |\n", report.Summary.MediumFindings))
	buf.WriteString(fmt.Sprintf("| Low | %d |\n\n", report.Summary.LowFindings))

	// Control Results
	buf.WriteString("## Control Results\n\n")
	buf.WriteString("| Control ID | Name | Status | Score | Findings |\n")
	buf.WriteString("|------------|------|--------|-------|----------|\n")
	for _, item := range report.ControlResults {
		buf.WriteString(fmt.Sprintf("| %s | %s | %s | %.0f%% | %d |\n",
			item.ControlID,
			truncate(item.ControlName, 30),
			item.Status,
			item.Score*100,
			item.FindingsCount))
	}
	buf.WriteString("\n")

	// Findings
	if len(report.Findings) > 0 {
		buf.WriteString("## Findings\n\n")
		for _, finding := range report.Findings {
			buf.WriteString(fmt.Sprintf("### %s\n\n", finding.Title))
			buf.WriteString(fmt.Sprintf("- **Control:** %s\n", finding.ControlID))
			buf.WriteString(fmt.Sprintf("- **Severity:** %s\n", finding.Severity))
			buf.WriteString(fmt.Sprintf("- **Description:** %s\n", finding.Description))
			buf.WriteString(fmt.Sprintf("- **Remediation:** %s\n\n", finding.Remediation))
		}
	}

	// Gap Analysis
	if report.GapAnalysis != nil {
		buf.WriteString("## Gap Analysis\n\n")
		buf.WriteString(fmt.Sprintf("- **Total Gaps:** %d\n", report.GapAnalysis.TotalGaps))
		buf.WriteString(fmt.Sprintf("- **Critical Gaps:** %d\n", report.GapAnalysis.CriticalGaps))
		buf.WriteString(fmt.Sprintf("- **High Priority Gaps:** %d\n\n", report.GapAnalysis.HighPriorityGaps))

		buf.WriteString("### Recommendations\n\n")
		for _, rec := range report.GapAnalysis.Recommendations {
			buf.WriteString(fmt.Sprintf("- %s\n", rec))
		}
	}

	_, err := w.Write(buf.Bytes())
	return err
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
