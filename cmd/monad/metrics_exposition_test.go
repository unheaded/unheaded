// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"unheaded/pkg/logger"
	"unheaded/services/monad"

	"unheaded/pkg/metrics/metricstest"
)

// newMetricsTestServer builds just enough HTTPServer to serve /metrics.
func newMetricsTestServer(t *testing.T) *HTTPServer {
	t.Helper()
	svc := monad.NewService(logger.New(io.Discard), nil)
	hs, err := NewHTTPServer(svc, logger.New(io.Discard), ":0")
	if err != nil {
		t.Fatalf("NewHTTPServer: %v", err)
	}
	return hs
}

// scrape returns the body of GET /metrics.
func scrape(t *testing.T, hs *HTTPServer) string {
	t.Helper()
	w := httptest.NewRecorder()
	hs.metricsHandler(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	return w.Body.String()
}

// The exposition must carry every metric this service has always published.
// Named explicitly so a refactor of how the text is produced cannot silently
// drop one: a scraper that loses a series looks identical to a service that
// went quiet.
func TestMetricsHandler_PublishesEveryDocumentedSeries(t *testing.T) {
	body := scrape(t, newMetricsTestServer(t))

	for _, name := range []string{
		"monad_http_requests_total",
		"monad_http_requests_success",
		"monad_http_requests_error",
		"monad_operations_executed_total",
		"monad_transactions_total",
		"monad_transactions_executed_total",
		"monad_operations_total",
		"monad_operations_completed",
		"monad_operations_failed",
		"monad_operations_running",
	} {
		if !strings.Contains(body, "\n"+name+" ") && !strings.HasPrefix(body, name+" ") {
			t.Errorf("exposition is missing a sample line for %s", name)
		}
		if !strings.Contains(body, "# TYPE "+name+" ") {
			t.Errorf("exposition is missing # TYPE for %s", name)
		}
	}
}

// Prometheus' text format forbids a metric's HELP or TYPE appearing twice.
// Hand-rolled exposition makes this easy to get wrong — it is exactly the bug
// that shipped in pkg/logagg's first collector.
func TestMetricsHandler_NoDuplicateHelpOrType(t *testing.T) {
	body := scrape(t, newMetricsTestServer(t))

	seen := map[string]int{}
	for _, line := range strings.Split(body, "\n") {
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

// Every sample line must be preceded by a declared TYPE for its family, and
// every declared TYPE must be a real Prometheus type. The rules, including a
// summary's _sum/_count, live in one shared checker (metricstest.Lint).
func TestMetricsHandler_WellFormed(t *testing.T) {
	body := scrape(t, newMetricsTestServer(t))
	for _, p := range metricstest.Lint(body) {
		t.Error(p)
	}
}
