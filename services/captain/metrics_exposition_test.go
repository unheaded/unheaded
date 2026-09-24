// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package captain

import (
	"net/http/httptest"
	"strings"
	"testing"

	"unheaded/pkg/metrics/metricstest"
)

// captain's /metrics was hand-written with two untyped series and nothing
// checked it. This is the check.
func TestMetricsHandler_LintsCleanAndTypesEverySeries(t *testing.T) {
	svc, hs := createTestServer(t)
	defer svc.Close()
	rec := httptest.NewRecorder()
	hs.server.Handler.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	body := rec.Body.String()

	for _, p := range metricstest.Lint(body) {
		t.Error(p)
	}
	for _, name := range []string{"captain_http_requests_total", "captain_http_requests_success", "captain_http_requests_error"} {
		if !strings.Contains(body, "# TYPE "+name+" counter") {
			t.Errorf("%s is not declared a counter", name)
		}
	}
	if !strings.Contains(body, "# TYPE go_goroutines gauge") {
		t.Error("default registry not served")
	}
}
