// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package cmd

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// checkServiceHealth was flagged unused by golangci-lint — it's a real CLI
// helper waiting for its consuming subcommand. These tests pin its contract
// so it isn't accidentally deleted.

func TestCheckServiceHealth_HTTPOK(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	healthy, err := checkServiceHealth(srv.URL)
	if err != nil {
		t.Fatalf("checkServiceHealth: %v", err)
	}
	if !healthy {
		t.Errorf("expected healthy=true for 200 response")
	}
}

func TestCheckServiceHealth_Non2xxReportsUnhealthy(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	healthy, err := checkServiceHealth(srv.URL)
	if err != nil {
		t.Errorf("non-200 should not error, got: %v", err)
	}
	if healthy {
		t.Errorf("500 should report unhealthy=false")
	}
}

func TestCheckServiceHealth_UnreachableErrors(t *testing.T) {
	t.Parallel()
	// 127.0.0.1:1 is the canonical unreachable port (assigned but reserved).
	healthy, err := checkServiceHealth("http://127.0.0.1:1")
	if err == nil {
		t.Errorf("expected error for unreachable endpoint")
	}
	if healthy {
		t.Errorf("unreachable should report unhealthy=false")
	}
}
