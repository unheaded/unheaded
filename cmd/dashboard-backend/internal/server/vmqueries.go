// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package server

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"

	"unheaded/pkg/httputil"
	"unheaded/pkg/vmquery"
)

// Every PromQL the dashboard runs (ADR-147 decision 9: it keeps no time
// series of its own; every graph reads VictoriaMetrics). One constant per
// field the page reads (Step 207's dash-fields.txt), plus the per-service
// rows /api/v1/metrics serves. HTTP series come from pkg/httputil's
// middleware (unheaded_http_*, a const service label); process series carry
// the scrape job.
const (
	// rateWindowSeconds is the request-rate window; the summary reports it.
	rateWindowSeconds = 60

	// Summary panel (/api/v1/metrics/summary).
	qRequestRate       = `sum(rate(unheaded_http_requests_total[1m]))`
	qErrorPercent      = `100 * sum(rate(unheaded_http_requests_total{status=~"5.."}[1m])) / sum(rate(unheaded_http_requests_total[1m]))`
	qLatencyQuantile   = `histogram_quantile(%s, sum by (le) (rate(unheaded_http_request_duration_seconds_bucket[5m])))`
	qServicesReporting = `count(sum by (service) (rate(unheaded_http_requests_total[1m])) > 0)`
	qTargetsScraped    = `count(up)`
	qUptime            = `max(time() - process_start_time_seconds)`

	// Per service (/api/v1/metrics, /api/v1/latency without BPF).
	qSvcRequestRate = `sum by (service) (rate(unheaded_http_requests_total[1m]))`
	qSvcErrorRate   = `sum by (service) (rate(unheaded_http_requests_total{status=~"5.."}[1m]))`
	qSvcQuantile    = `histogram_quantile(%s, sum by (service, le) (rate(unheaded_http_request_duration_seconds_bucket[5m])))`
	qSvcMeanSeconds = `sum by (service) (rate(unheaded_http_request_duration_seconds_sum[5m])) / sum by (service) (rate(unheaded_http_request_duration_seconds_count[5m]))`
	qSvcSamples     = `sum by (service) (increase(unheaded_http_request_duration_seconds_count[5m]))`
	qJobMemory      = `sum by (job) (process_resident_memory_bytes)`
	qJobCPU         = `sum by (job) (rate(process_cpu_seconds_total[1m]))`
)

// errVictoria is what every metric endpoint answers when VictoriaMetrics is
// down or not configured: a 503 the page can see, never a 200 with zeros.
var errVictoria = errors.New("victoriametrics unavailable")

// quantile fills a quantile template.
func quantile(tmpl string, q float64) string {
	return strings.Replace(tmpl, "%s", strconv.FormatFloat(q, 'f', -1, 64), 1)
}

// writeVictoriaDown answers 503 {"error":"victoriametrics unavailable"}.
func writeVictoriaDown(w http.ResponseWriter) {
	httputil.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": errVictoria.Error()})
}

// vmScalar runs a query whose answer is one number; nil when the answer is
// empty or not a finite number ("no data yet", shown as "--").
func (s *Server) vmScalar(ctx context.Context, promql string) (*float64, error) {
	if s.vm == nil {
		return nil, errVictoria
	}
	samples, err := s.vm.Instant(ctx, promql)
	if err != nil {
		s.log.Debug().Err(err).Str("query", promql).Msg("victoriametrics query failed")
		return nil, errVictoria
	}
	if len(samples) == 0 {
		return nil, nil
	}
	return finite(samples[0].Value), nil
}

// vmBy runs a query grouped by one label: label value -> number (finite
// values only).
func (s *Server) vmBy(ctx context.Context, promql, label string) (map[string]float64, error) {
	if s.vm == nil {
		return nil, errVictoria
	}
	samples, err := s.vm.Instant(ctx, promql)
	if err != nil {
		s.log.Debug().Err(err).Str("query", promql).Msg("victoriametrics query failed")
		return nil, errVictoria
	}
	out := make(map[string]float64, len(samples))
	for _, sm := range samples {
		key := sm.Metric[label]
		if v := finite(sm.Value); key != "" && v != nil {
			out[key] = *v
		}
	}
	return out, nil
}

func finite(v float64) *float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return &v
}

// scaled multiplies a reading (seconds to milliseconds, and so on).
func scaled(v *float64, by float64) *float64 {
	if v == nil {
		return nil
	}
	x := *v * by
	return &x
}

// latencyMS is the summary's latency_ms object.
type latencyMS struct {
	P50 *float64 `json:"p50"`
	P95 *float64 `json:"p95"`
	P99 *float64 `json:"p99"`
}

// panelSummary is /api/v1/metrics/summary: the fields the page's summary
// cards read, from VictoriaMetrics. A null is "no data yet" ("--").
type panelSummary struct {
	RequestRate       *float64  `json:"request_rate"`
	ErrorRate         *float64  `json:"error_rate"`
	Latency           latencyMS `json:"latency_ms"`
	UptimeSeconds     *float64  `json:"uptime_seconds"`
	ServicesReporting int       `json:"services_reporting"`
	ServicesScraped   int       `json:"services_scraped"`
	WindowSeconds     float64   `json:"window_seconds"`
	DashboardClients  int       `json:"dashboard_clients"`
}

func (s *Server) vmSummary(ctx context.Context) (*panelSummary, error) {
	out := &panelSummary{WindowSeconds: rateWindowSeconds}
	var err error
	for _, f := range []struct {
		q   string
		dst **float64
	}{
		{qRequestRate, &out.RequestRate},
		{qErrorPercent, &out.ErrorRate},
		{qUptime, &out.UptimeSeconds},
	} {
		if *f.dst, err = s.vmScalar(ctx, f.q); err != nil {
			return nil, err
		}
	}
	for _, f := range []struct {
		q   float64
		dst **float64
	}{{0.5, &out.Latency.P50}, {0.95, &out.Latency.P95}, {0.99, &out.Latency.P99}} {
		v, err := s.vmScalar(ctx, quantile(qLatencyQuantile, f.q))
		if err != nil {
			return nil, err
		}
		*f.dst = scaled(v, 1e3)
	}
	for _, f := range []struct {
		q   string
		dst *int
	}{{qServicesReporting, &out.ServicesReporting}, {qTargetsScraped, &out.ServicesScraped}} {
		v, err := s.vmScalar(ctx, f.q)
		if err != nil {
			return nil, err
		}
		if v != nil {
			*f.dst = int(*v)
		}
	}
	return out, nil
}

// serviceRow is one service's line in /api/v1/metrics.
type serviceRow struct {
	RequestRate *float64 `json:"request_rate"`
	ErrorRate   *float64 `json:"error_rate"`
	P50MS       *float64 `json:"p50_ms"`
	P99MS       *float64 `json:"p99_ms"`
	MemoryBytes *float64 `json:"memory_bytes"`
	CPUCores    *float64 `json:"cpu_cores"`
}

// vmServiceRows reads every service's request rate, 5xx rate, p50 and p99
// (by the middleware's service label) and memory and CPU (by scrape job).
func (s *Server) vmServiceRows(ctx context.Context) (map[string]*serviceRow, error) {
	rows := map[string]*serviceRow{}
	row := func(name string) *serviceRow {
		if rows[name] == nil {
			rows[name] = &serviceRow{}
		}
		return rows[name]
	}
	for _, f := range []struct {
		q, label string
		scale    float64
		set      func(*serviceRow, *float64)
	}{
		{qSvcRequestRate, "service", 1, func(r *serviceRow, v *float64) { r.RequestRate = v }},
		{qSvcErrorRate, "service", 1, func(r *serviceRow, v *float64) { r.ErrorRate = v }},
		{quantile(qSvcQuantile, 0.5), "service", 1e3, func(r *serviceRow, v *float64) { r.P50MS = v }},
		{quantile(qSvcQuantile, 0.99), "service", 1e3, func(r *serviceRow, v *float64) { r.P99MS = v }},
		{qJobMemory, "job", 1, func(r *serviceRow, v *float64) { r.MemoryBytes = v }},
		{qJobCPU, "job", 1, func(r *serviceRow, v *float64) { r.CPUCores = v }},
	} {
		vals, err := s.vmBy(ctx, f.q, f.label)
		if err != nil {
			return nil, err
		}
		for name, v := range vals {
			f.set(row(name), scaled(&v, f.scale))
		}
	}
	return rows, nil
}

// vmLatencyPercentiles is /api/v1/latency without BPF: per service, the
// HTTP request latency the middleware records, as one window in the shape
// the page reads ({sample_count, p50_ns, p90_ns, p99_ns, mean_ns}), under
// the operation name http_<service>. A service with no requests in the
// window is left out.
func (s *Server) vmLatencyPercentiles(ctx context.Context) (map[string][]map[string]float64, error) {
	samples, err := s.vmBy(ctx, qSvcSamples, "service")
	if err != nil {
		return nil, err
	}
	out := map[string][]map[string]float64{}
	fields := map[string]map[string]float64{}
	for svc, n := range samples {
		if n > 0 {
			fields[svc] = map[string]float64{"sample_count": math.Round(n)}
		}
	}
	for _, f := range []struct {
		q   string
		key string
	}{
		{quantile(qSvcQuantile, 0.5), "p50_ns"},
		{quantile(qSvcQuantile, 0.9), "p90_ns"},
		{quantile(qSvcQuantile, 0.99), "p99_ns"},
		{qSvcMeanSeconds, "mean_ns"},
	} {
		vals, err := s.vmBy(ctx, f.q, "service")
		if err != nil {
			return nil, err
		}
		for svc, v := range vals {
			if m := fields[svc]; m != nil {
				m[f.key] = v * 1e9
			}
		}
	}
	for svc, m := range fields {
		out["http_"+svc] = []map[string]float64{m}
	}
	return out, nil
}

// newVMClient builds the VictoriaMetrics reader; nil (every metric endpoint
// answers 503) when no URL is set.
func newVMClient(url string) (*vmquery.Client, error) {
	if url == "" {
		return nil, nil
	}
	return vmquery.New(url, nil)
}
