// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package server

import (
	"context"
	"time"

	"unheaded/pkg/compliance/crosswalk"
	"unheaded/pkg/metrics"
)

// complianceMetrics exports the findings register (ADR-098) so Prometheus
// can alert on it: open findings per severity, one series per open finding,
// and the newest evidence time, which is how a collector that has stopped
// running shows up. report_ok drops to 0 when the catalog, register or
// evidence cannot be read, so a broken register alarms instead of reading
// as zero findings.
type complianceMetrics struct {
	src         *complianceSource
	open        *metrics.GaugeVec
	finding     *metrics.GaugeVec
	overdue     *metrics.Gauge
	expired     *metrics.Gauge
	unobserved  *metrics.Gauge
	stale       *metrics.Gauge
	zero        *metrics.Gauge
	newest      *metrics.Gauge
	reportOK    *metrics.Gauge
	exported    []metrics.Labels // finding series set by the last refresh
	refreshTick time.Duration
}

var findingSeverities = []crosswalk.Severity{crosswalk.SeverityUntriaged, crosswalk.SeverityCritical,
	crosswalk.SeverityHigh, crosswalk.SeverityMedium, crosswalk.SeverityLow}

func newComplianceMetrics(src *complianceSource) *complianceMetrics {
	g := func(name, help string) *metrics.Gauge { return metrics.NewGauge(name, help, nil) }
	return &complianceMetrics{
		src: src,
		open: metrics.NewGaugeVec("unheaded_compliance_findings_open",
			"Open findings in the ADR-098 register, by severity (accepted ones included)", nil, []string{"severity"}),
		finding: metrics.NewGaugeVec("unheaded_compliance_finding",
			"1 per open finding; the series goes away when its check passes", nil, []string{"key", "host", "severity"}),
		overdue:     g("unheaded_compliance_findings_overdue", "Open findings past their due date"),
		expired:     g("unheaded_compliance_acceptances_expired", "Open findings whose risk acceptance has expired"),
		unobserved:  g("unheaded_compliance_sources_unobserved", "Catalog evidence sources never observed (NOT_ASSESSED)"),
		stale:       g("unheaded_compliance_sources_stale", "Catalog evidence sources whose latest pass is past its freshness window"),
		zero:        g("unheaded_compliance_zero_claimable", "1 when nothing is open, unobserved or stale"),
		newest:      g("unheaded_compliance_evidence_newest_timestamp_seconds", "Newest evidence record (unix seconds, 0 if none)"),
		reportOK:    g("unheaded_compliance_report_ok", "1 if catalog, register and evidence were read at the last refresh"),
		refreshTick: time.Minute,
	}
}

func (m *complianceMetrics) collectors() []metrics.Collector {
	return []metrics.Collector{m.open, m.finding, m.overdue, m.expired, m.unobserved, m.stale, m.zero, m.newest, m.reportOK}
}

// refresh recomputes every series. On error only report_ok changes: the
// last good values stay, and the alarm is report_ok == 0.
func (m *complianceMetrics) refresh(now time.Time) {
	rep, recs, _, err := m.src.findings(now)
	if err != nil {
		m.reportOK.Set(0)
		return
	}
	for _, s := range findingSeverities {
		m.open.WithLabelValues(string(s)).Set(float64(rep.Counts[s]))
	}
	for _, l := range m.exported {
		m.finding.DeletePartialMatch(l)
	}
	m.exported = m.exported[:0]
	var expired int
	for _, f := range rep.Open {
		l := metrics.Labels{"key": f.Key, "host": f.Host, "severity": string(f.Severity)}
		m.finding.WithLabels(l).Set(1)
		m.exported = append(m.exported, l)
		if f.AcceptanceExpired {
			expired++
		}
	}
	m.overdue.Set(float64(rep.Overdue))
	m.expired.Set(float64(expired))
	m.unobserved.Set(float64(len(rep.Unobserved)))
	m.stale.Set(float64(len(rep.Stale)))
	if rep.ZeroClaimable {
		m.zero.Set(1)
	} else {
		m.zero.Set(0)
	}
	var newest time.Time
	for _, r := range recs {
		if r.ObservedAt.After(newest) && !r.ObservedAt.After(now) {
			newest = r.ObservedAt
		}
	}
	if newest.IsZero() {
		m.newest.Set(0)
	} else {
		m.newest.Set(float64(newest.Unix()))
	}
	m.reportOK.Set(1)
}

func (s *Server) refreshComplianceMetrics(ctx context.Context) {
	defer s.wg.Done()
	m := s.complianceMetrics
	m.refresh(time.Now())
	ticker := time.NewTicker(m.refreshTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.shutdown:
			return
		case <-ticker.C:
			m.refresh(time.Now())
		}
	}
}
