// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package scraper

import (
	"math"
	"testing"
	"time"
)

var t0 = time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)

func req(status string, v float64) MetricSample {
	return MetricSample{Name: requestsFamily, Value: v, Labels: map[string]string{"method": "GET", "path": "/x", "status": status}}
}

func bucket(le string, v float64) MetricSample {
	return MetricSample{Name: durationBucket, Value: v, Labels: map[string]string{"method": "GET", "path": "/x", "le": le}}
}

func scrape(at time.Time, ms ...MetricSample) *ScrapeResult {
	return &ScrapeResult{Timestamp: at, Metrics: ms}
}

func near(t *testing.T, what string, got *float64, want float64) {
	t.Helper()
	if got == nil {
		t.Errorf("%s = nil, want %v", what, want)
		return
	}
	if math.Abs(*got-want) > 1e-9 {
		t.Errorf("%s = %v, want %v", what, *got, want)
	}
}

func TestSummarize_RateAndErrorsFromDeltasNotTotals(t *testing.T) {
	prev := map[string]*ScrapeResult{"a": scrape(t0, req("200", 1000), req("503", 4))}
	cur := map[string]*ScrapeResult{"a": scrape(t0.Add(10*time.Second), req("200", 1054), req("503", 10))}

	s := summarize(cur, prev, t0)
	// 60 new requests in 10s, 6 of them 5xx. The old code showed 1064/5.
	near(t, "request_rate", s.RequestRate, 6)
	near(t, "error_rate", s.ErrorRate, 10)
	if s.ServicesReporting != 1 || s.WindowSeconds != 10 {
		t.Errorf("reporting %d window %v", s.ServicesReporting, s.WindowSeconds)
	}
}

func TestSummarize_SumsServicesEachOverItsOwnWindow(t *testing.T) {
	prev := map[string]*ScrapeResult{
		"a": scrape(t0, req("200", 0)),
		"b": scrape(t0, req("200", 0)),
	}
	cur := map[string]*ScrapeResult{
		"a": scrape(t0.Add(10*time.Second), req("200", 30)), // 3/s
		"b": scrape(t0.Add(20*time.Second), req("200", 20)), // 1/s
	}
	s := summarize(cur, prev, t0)
	near(t, "request_rate", s.RequestRate, 4)
	if s.WindowSeconds != 15 {
		t.Errorf("window = %v, want mean 15", s.WindowSeconds)
	}
}

func TestSummarize_CounterResetAndNewSeries(t *testing.T) {
	prev := map[string]*ScrapeResult{"a": scrape(t0, req("200", 1000))}
	// Restarted (counter back to 30) and a 500 series born in the window.
	cur := map[string]*ScrapeResult{"a": scrape(t0.Add(10*time.Second), req("200", 30), req("500", 10))}
	s := summarize(cur, prev, t0)
	near(t, "request_rate", s.RequestRate, 4) // (30 + 10) / 10, never negative
	near(t, "error_rate", s.ErrorRate, 25)
}

func TestSummarize_NilMeansNoDataNotZero(t *testing.T) {
	start := MetricSample{Name: processStart, Value: float64(t0.Unix())}
	// One scrape only: nothing derivable but uptime.
	s := summarize(map[string]*ScrapeResult{"a": scrape(t0.Add(90*time.Second), start, req("200", 5))}, map[string]*ScrapeResult{}, t0)
	if s.RequestRate != nil || s.ErrorRate != nil || s.Latency.P50 != nil {
		t.Errorf("derived values from a single scrape: %+v", s)
	}
	near(t, "uptime", s.UptimeSeconds, 90)
	if s.ServicesScraped != 1 || s.ServicesReporting != 0 {
		t.Errorf("scraped %d reporting %d", s.ServicesScraped, s.ServicesReporting)
	}

	// Two scrapes, no traffic: the rate is a real 0; error % and latency
	// have nothing to be a percentage or quantile of.
	idle := summarize(
		map[string]*ScrapeResult{"a": scrape(t0.Add(10*time.Second), req("200", 5), bucket("+Inf", 5))},
		map[string]*ScrapeResult{"a": scrape(t0, req("200", 5), bucket("+Inf", 5))}, t0)
	near(t, "idle request_rate", idle.RequestRate, 0)
	if idle.ErrorRate != nil || idle.Latency.P99 != nil {
		t.Errorf("idle window invented error/latency: %+v", idle)
	}
}

func TestSummarize_UptimeIsLongestRunning(t *testing.T) {
	cur := map[string]*ScrapeResult{
		"a": scrape(t0.Add(100*time.Second), MetricSample{Name: processStart, Value: float64(t0.Unix())}),
		"b": scrape(t0.Add(100*time.Second), MetricSample{Name: processStart, Value: float64(t0.Add(60 * time.Second).Unix())}),
	}
	near(t, "uptime", summarize(cur, nil, t0).UptimeSeconds, 100)
}

func TestQuantileMS(t *testing.T) {
	b := map[float64]float64{0.005: 50, 0.01: 80, 0.1: 100, math.Inf(1): 100}
	near(t, "p50", quantileMS(0.50, b), 5)    // 0 + 5ms * 50/50
	near(t, "p95", quantileMS(0.95, b), 77.5) // 10 + 90 * 15/20
	near(t, "p99", quantileMS(0.99, b), 95.5) // 10 + 90 * 19/20

	// Rank in +Inf: report the largest finite bound.
	near(t, "+Inf", quantileMS(0.5, map[float64]float64{0.1: 10, math.Inf(1): 100}), 100)
	if quantileMS(0.5, map[float64]float64{0.1: 0, math.Inf(1): 0}) != nil {
		t.Error("quantile of nothing")
	}
	if quantileMS(0.5, map[float64]float64{0.1: 5}) != nil {
		t.Error("histogram without +Inf accepted")
	}
}

func TestSummarize_LatencyFromBucketIncreasesAcrossServices(t *testing.T) {
	prev := map[string]*ScrapeResult{
		"a": scrape(t0, req("200", 0), bucket("0.005", 1000), bucket("0.01", 1000), bucket("0.1", 1000), bucket("+Inf", 1000)),
		"b": scrape(t0, req("200", 0)),
	}
	cur := map[string]*ScrapeResult{
		// History (1000 fast requests before the window) must not count.
		"a": scrape(t0.Add(10*time.Second), req("200", 60), bucket("0.005", 1030), bucket("0.01", 1040), bucket("0.1", 1060), bucket("+Inf", 1060)),
		// b's series are new in the window.
		"b": scrape(t0.Add(10*time.Second), req("200", 40), bucket("0.005", 20), bucket("0.01", 40), bucket("0.1", 40), bucket("+Inf", 40)),
	}
	s := summarize(cur, prev, t0)
	// Summed increases: le .005:50, .01:80, .1:100, +Inf:100 — TestQuantileMS's histogram.
	near(t, "p50", s.Latency.P50, 5)
	near(t, "p95", s.Latency.P95, 77.5)
	near(t, "p99", s.Latency.P99, 95.5)
}

func TestScraper_SummaryPairsLastTwoGoodScrapes(t *testing.T) {
	s := &Scraper{results: map[string]*ScrapeResult{}, good: map[string]*ScrapeResult{}, prevGood: map[string]*ScrapeResult{}}
	s.storeResult(&ScrapeResult{Service: "a", Timestamp: t0, Metrics: []MetricSample{req("200", 0)}})
	s.storeResult(&ScrapeResult{Service: "a", Timestamp: t0.Add(5 * time.Second), Error: "connection refused"})
	s.storeResult(&ScrapeResult{Service: "a", Timestamp: t0.Add(10 * time.Second), Metrics: []MetricSample{req("200", 50)}})

	// The failed scrape in between neither breaks nor shortens the window.
	near(t, "request_rate", s.Summary().RequestRate, 5)
}
