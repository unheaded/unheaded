// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package scraper

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The dashboard's summary panel used to derive its numbers in the browser
// from single scrapes: "request rate" was a cumulative total divided by the
// poll interval, errors and latency read metric names no service emits, and
// "p50/p95/p99" were percentiles taken across per-service values. Summary
// derives them here instead, from each service's last two successful scrapes
// of the metrics CLAUDE.md requires (unheaded_http_*), the way Prometheus's
// rate() and histogram_quantile() would.

const (
	requestsFamily = "unheaded_http_requests_total"
	durationBucket = "unheaded_http_request_duration_seconds_bucket"
	processStart   = "process_start_time_seconds"
)

// LatencyMS holds request-latency quantiles in milliseconds; nil when no
// request was observed in the window.
type LatencyMS struct {
	P50 *float64 `json:"p50"`
	P95 *float64 `json:"p95"`
	P99 *float64 `json:"p99"`
}

// PanelSummary is what the dashboard summary panel shows. Every field that
// needs two scrapes is nil until there are two; nil means "no data", never 0.
type PanelSummary struct {
	Timestamp time.Time `json:"timestamp"`
	// RequestRate is requests/s over the window, summed across services.
	RequestRate *float64 `json:"request_rate"`
	// ErrorRate is the percentage of those requests answered 5xx (the
	// definition the kingdom alert uses).
	ErrorRate *float64  `json:"error_rate"`
	Latency   LatencyMS `json:"latency_ms"`
	// UptimeSeconds is the longest-running service's process uptime.
	UptimeSeconds *float64 `json:"uptime_seconds"`
	// ServicesReporting counts services with two scrapes of the request
	// family; ServicesScraped counts every service with a good scrape.
	ServicesReporting int `json:"services_reporting"`
	ServicesScraped   int `json:"services_scraped"`
	// WindowSeconds is the mean interval between the scrape pairs used.
	WindowSeconds float64 `json:"window_seconds"`
}

// Summary computes the panel summary from the stored scrapes.
func (s *Scraper) Summary() *PanelSummary {
	s.resultsMu.RLock()
	defer s.resultsMu.RUnlock()
	return summarize(s.good, s.prevGood, time.Now())
}

type windowTotals struct {
	requests, errors float64
	buckets          map[float64]float64 // upper bound -> increase
	rate             float64
	reporting        int
	windowSum        float64
}

func summarize(cur, prev map[string]*ScrapeResult, now time.Time) *PanelSummary {
	out := &PanelSummary{Timestamp: now, ServicesScraped: len(cur)}
	t := windowTotals{buckets: map[float64]float64{}}

	var uptime float64
	haveUptime := false
	for svc, c := range cur {
		for _, sm := range c.Metrics {
			if sm.Name == processStart && sm.Value > 0 {
				if u := c.Timestamp.Sub(time.Unix(0, int64(sm.Value*1e9))).Seconds(); !haveUptime || u > uptime {
					uptime, haveUptime = u, true
				}
			}
		}
		p, ok := prev[svc]
		if !ok {
			continue
		}
		dt := c.Timestamp.Sub(p.Timestamp).Seconds()
		if dt <= 0 {
			continue
		}
		addWindow(&t, p.Metrics, c.Metrics, dt)
	}

	if haveUptime {
		out.UptimeSeconds = &uptime
	}
	if t.reporting == 0 {
		return out
	}
	out.ServicesReporting = t.reporting
	out.WindowSeconds = t.windowSum / float64(t.reporting)
	rate := t.rate
	out.RequestRate = &rate
	if t.requests > 0 {
		pct := 100 * t.errors / t.requests
		out.ErrorRate = &pct
	}
	out.Latency = LatencyMS{
		P50: quantileMS(0.50, t.buckets),
		P95: quantileMS(0.95, t.buckets),
		P99: quantileMS(0.99, t.buckets),
	}
	return out
}

// addWindow folds one service's scrape pair into the totals.
func addWindow(t *windowTotals, prev, cur []MetricSample, dt float64) {
	before := make(map[string]float64, len(prev))
	for _, sm := range prev {
		if sm.Name == requestsFamily || sm.Name == durationBucket {
			before[seriesKey(sm.Name, sm.Labels)] = sm.Value
		}
	}

	var svcRequests float64
	sawFamily := false
	for _, sm := range cur {
		switch sm.Name {
		case requestsFamily:
			sawFamily = true
			d := increase(before, sm)
			svcRequests += d
			if strings.HasPrefix(sm.Labels["status"], "5") {
				t.errors += d
			}
		case durationBucket:
			le, err := strconv.ParseFloat(sm.Labels["le"], 64)
			if err != nil {
				continue
			}
			t.buckets[le] += increase(before, sm)
		}
	}
	if !sawFamily {
		return
	}
	t.reporting++
	t.windowSum += dt
	t.requests += svcRequests
	t.rate += svcRequests / dt
}

// increase is a counter's growth between two scrapes. A drop means the
// process restarted and the counter began again at 0, so everything it now
// holds is new (Prometheus's reset rule). A series absent before was created
// by its first increment, inside the window.
func increase(before map[string]float64, sm MetricSample) float64 {
	p, ok := before[seriesKey(sm.Name, sm.Labels)]
	if !ok || sm.Value < p {
		return sm.Value
	}
	return sm.Value - p
}

// quantileMS is histogram_quantile over cumulative bucket increases: linear
// interpolation inside the bucket the rank falls in. A rank in the +Inf
// bucket reports the largest finite bound, as Prometheus does.
func quantileMS(q float64, buckets map[float64]float64) *float64 {
	bounds := make([]float64, 0, len(buckets))
	for le := range buckets {
		bounds = append(bounds, le)
	}
	sort.Float64s(bounds)
	if len(bounds) == 0 || !math.IsInf(bounds[len(bounds)-1], 1) {
		return nil
	}
	total := buckets[bounds[len(bounds)-1]]
	if total <= 0 {
		return nil
	}
	rank := q * total
	lower, below := 0.0, 0.0
	for _, le := range bounds {
		count := buckets[le]
		if count >= rank {
			var v float64
			switch {
			case math.IsInf(le, 1):
				v = lower
			case count == below:
				v = le
			default:
				v = lower + (le-lower)*(rank-below)/(count-below)
			}
			ms := v * 1000
			return &ms
		}
		if !math.IsInf(le, 1) {
			lower = le
		}
		below = count
	}
	return nil
}
