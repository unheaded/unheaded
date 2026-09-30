// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package server

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/compliance/crosswalk"
	"unheaded/pkg/metrics"
)

func writeEvidence(t *testing.T, path string, recs []crosswalk.Record) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := crosswalk.WriteEvidence(f, recs); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func gather(t *testing.T, reg *metrics.Registry) string {
	t.Helper()
	var buf bytes.Buffer
	if err := reg.Gather(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestComplianceMetrics(t *testing.T) {
	src := complianceFixture(t, false)
	reg := metrics.NewRegistry()
	m := newComplianceMetrics(src)
	for _, c := range m.collectors() {
		reg.MustRegister(c)
	}
	now := time.Now()
	scan := crosswalk.Source{Kind: crosswalk.KindGitHubJob, Ref: "W/scan"}
	observed := now.Add(-time.Hour).Truncate(time.Second)
	writeEvidence(t, src.evidencePath, []crosswalk.Record{{Source: scan, Verdict: crosswalk.VerdictFail, ObservedAt: observed}})

	m.refresh(now)
	out := gather(t, reg)
	for _, want := range []string{
		`unheaded_compliance_findings_open{severity="untriaged"} 1`,
		`unheaded_compliance_findings_open{severity="critical"} 0`, // every severity exported, so no alert needs absent()
		`unheaded_compliance_finding{host="",key="github-job:W/scan",severity="untriaged"} 1`,
		`unheaded_compliance_findings_overdue 0`,
		`unheaded_compliance_zero_claimable 0`,
		`unheaded_compliance_report_ok 1`,
		"unheaded_compliance_evidence_newest_timestamp_seconds " + formatInt(observed.Unix()),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}

	// The check passes: its series disappears rather than lingering at 1.
	writeEvidence(t, src.evidencePath, []crosswalk.Record{
		{Source: scan, Verdict: crosswalk.VerdictFail, ObservedAt: observed},
		{Source: scan, Verdict: crosswalk.VerdictPass, ObservedAt: observed.Add(time.Minute)},
	})
	m.refresh(now)
	out = gather(t, reg)
	if strings.Contains(out, `key="github-job:W/scan"`) {
		t.Errorf("resolved finding still exported:\n%s", out)
	}
	if !strings.Contains(out, `unheaded_compliance_findings_open{severity="untriaged"} 0`) {
		t.Errorf("open count not reset:\n%s", out)
	}

	// A broken register is an alarm (report_ok 0), not a quiet zero.
	regPath := filepath.Join(src.root, "compliance/findings/register.yaml")
	if err := os.MkdirAll(filepath.Dir(regPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(regPath, []byte("findings: [{source: nope}]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.refresh(now)
	if out = gather(t, reg); !strings.Contains(out, "unheaded_compliance_report_ok 0") {
		t.Errorf("invalid register not reported:\n%s", out)
	}
}

func TestComplianceMetrics_NoEvidence(t *testing.T) {
	src := complianceFixture(t, false)
	reg := metrics.NewRegistry()
	m := newComplianceMetrics(src)
	for _, c := range m.collectors() {
		reg.MustRegister(c)
	}
	m.refresh(time.Now())
	out := gather(t, reg)
	if !strings.Contains(out, "unheaded_compliance_evidence_newest_timestamp_seconds 0") ||
		!strings.Contains(out, "unheaded_compliance_sources_unobserved 1") ||
		!strings.Contains(out, "unheaded_compliance_zero_claimable 0") {
		t.Errorf("no evidence:\n%s", out)
	}
}

// formatInt renders n as pkg/metrics does (%g, shortest exact form).
func formatInt(n int64) string { return fmt.Sprintf("%g", float64(n)) }
