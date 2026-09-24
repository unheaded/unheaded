// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// sophiaSeries is every series this service publishes.
//
// Four of them — requests_success, requests_error, knowledge_learned,
// knowledge_queried — were emitted by the old hand-written exposition with NO
// # TYPE line, so a scraper treated them as untyped. Verified on the live
// endpoint before the change: 7 types declared, 4 samples undeclared. That is
// the defect class hand-assembling the wire format produces.
var sophiaSeries = []string{
	"sophia_http_requests_total",
	"sophia_http_requests_success",
	"sophia_http_requests_error",
	"sophia_knowledge_total",
	"sophia_knowledge_learned",
	"sophia_knowledge_queried",
	"sophia_insights_total",
	"sophia_decisions_total",
}

func scrapeSophia(t *testing.T) string {
	t.Helper()
	hs := &HTTPServer{metrics: &HTTPMetrics{}}
	hs.initMetrics()

	w := httptest.NewRecorder()
	hs.metricsHandler(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	return w.Body.String()
}

func TestSophiaMetrics_EverySeriesHasATypeAndASample(t *testing.T) {
	body := scrapeSophia(t)

	for _, name := range sophiaSeries {
		if !strings.Contains(body, "# TYPE "+name+" ") {
			t.Errorf("%s has no # TYPE — a scraper will treat it as untyped", name)
		}
		if !strings.Contains(body, "\n"+name+" ") && !strings.HasPrefix(body, name+" ") {
			t.Errorf("%s has no sample line", name)
		}
	}
}

func TestSophiaMetrics_NoDuplicateHelpOrType(t *testing.T) {
	seen := map[string]int{}
	for _, line := range strings.Split(scrapeSophia(t), "\n") {
		if strings.HasPrefix(line, "# HELP ") || strings.HasPrefix(line, "# TYPE ") {
			seen[line]++
		}
	}
	for line, n := range seen {
		if n != 1 {
			t.Errorf("%q appears %d times; the text format allows one", line, n)
		}
	}
}

// No sample may appear without a TYPE declared for it. This is the assertion
// the old implementation failed for four series.
func TestSophiaMetrics_NoUntypedSamples(t *testing.T) {
	declared := map[string]bool{}
	var samples []string

	for _, line := range strings.Split(scrapeSophia(t), "\n") {
		switch {
		case strings.HasPrefix(line, "# TYPE "):
			if f := strings.Fields(line); len(f) == 4 {
				declared[f[2]] = true
			}
		case line == "" || strings.HasPrefix(line, "#"):
		default:
			samples = append(samples, strings.SplitN(strings.Fields(line)[0], "{", 2)[0])
		}
	}

	for _, s := range samples {
		if !declared[s] {
			t.Errorf("sample %s has no declared # TYPE", s)
		}
	}
}
