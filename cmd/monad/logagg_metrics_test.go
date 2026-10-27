// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"strings"
	"testing"

	"unheaded/pkg/logagg"
	"unheaded/pkg/metrics/metricstest"
)

// The log publisher's counters live in the registry this service serves
// (S-WELL-SPRING Step 188), not in text appended after it: one HELP, one
// sample, a well-formed page, and a publisher swap does not duplicate them.
func TestMetrics_LogPublisherCountersInRegistry(t *testing.T) {
	hs := newMetricsTestServer(t)
	for i := 0; i < 2; i++ { // the second call swaps publishers
		p := logagg.NewPublisher("monad", nil)
		defer p.Close()
		if err := hs.SetLogPublisher(p); err != nil {
			t.Fatalf("SetLogPublisher #%d: %v", i+1, err)
		}
	}

	var buf bytes.Buffer
	if err := hs.registry.Gather(&buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `unheaded_logagg_entries_dropped_total{service="monad"} 0`) {
		t.Fatalf("registry lacks the logagg counters:\n%s", buf.String())
	}

	body := scrape(t, hs)
	for _, want := range []string{
		"# HELP unheaded_logagg_entries_dropped_total ",
		`unheaded_logagg_entries_dropped_total{service="monad"} 0`,
		`unheaded_logagg_entries_published_total{service="monad"} 0`,
		`unheaded_logagg_publish_errors_total{service="monad"} 0`,
	} {
		if n := strings.Count(body, want); n != 1 {
			t.Errorf("%q appears %d times in /metrics, want 1", want, n)
		}
	}
	for _, p := range metricstest.Lint(body) {
		t.Error(p)
	}
}
