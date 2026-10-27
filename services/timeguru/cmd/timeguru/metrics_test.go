// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"unheaded/pkg/logagg"
	"unheaded/pkg/metrics/metricstest"
)

// /metrics serves the log publisher's counters from a registry (S-WELL-SPRING
// Step 188) with the default registry's go_* and process_*: one HELP each,
// a well-formed page, GET only.
func TestMetricsHandler_LogPublisherCountersFromRegistry(t *testing.T) {
	p := logagg.NewPublisher("timeguru", nil)
	defer p.Close()
	h, err := newMetricsHandler(p)
	if err != nil {
		t.Fatalf("newMetricsHandler: %v", err)
	}

	w := httptest.NewRecorder()
	h(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{
		"# HELP unheaded_logagg_entries_dropped_total ",
		`unheaded_logagg_entries_dropped_total{service="timeguru"} 0`,
		`unheaded_logagg_entries_published_total{service="timeguru"} 0`,
		`unheaded_logagg_publish_errors_total{service="timeguru"} 0`,
		"\ngo_goroutines ",
	} {
		if n := strings.Count(body, want); n != 1 {
			t.Errorf("%q appears %d times in /metrics, want 1", want, n)
		}
	}
	for _, p := range metricstest.Lint(body) {
		t.Error(p)
	}

	w = httptest.NewRecorder()
	h(w, httptest.NewRequest(http.MethodPost, "/metrics", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /metrics: %d, want 405", w.Code)
	}
}
