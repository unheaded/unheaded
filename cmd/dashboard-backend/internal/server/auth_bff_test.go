// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// S-WEST-GROUND Phase 3 (ADR-147 decision 16): with AUTH_ANONYMOUS_FROM set,
// /metrics needs a credential from outside those networks. The UI's own pages
// stay open, and nothing changes when the variable is unset.
func TestMetricsNeedsCredentialFromOutside(t *testing.T) {
	t.Setenv("AUTH_ANONYMOUS_FROM", "127.0.0.0/8,172.16.0.0/12")
	e := newBFFEnv(t, nil)
	from := func(target, remote, authz string) int {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.RemoteAddr = remote
		if authz != "" {
			req.Header.Set("Authorization", authz)
		}
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, req)
		return rec.Code
	}
	cases := []struct {
		target, remote string
		want           int
	}{
		{"/metrics", "192.168.69.50:1", http.StatusUnauthorized},
		{"/metrics", "172.28.2.5:1", http.StatusOK},
		{"/metrics", "127.0.0.1:1", http.StatusOK},
		{"/metrics/", "192.168.69.50:1", http.StatusUnauthorized},
		{"/health", "192.168.69.50:1", http.StatusOK},
	}
	for _, c := range cases {
		if got := from(c.target, c.remote, ""); got != c.want {
			t.Errorf("GET %s from %s: %d, want %d", c.target, c.remote, got, c.want)
		}
	}
	if got := from("/metrics", "192.168.69.50:1", e.f.Bearer("unheaded-cli", "dashboard-backend")); got != http.StatusOK {
		t.Errorf("GET /metrics from the LAN with a token: %d, want 200", got)
	}
}

func TestMetricsOpenWhenAnonymousFromUnset(t *testing.T) {
	t.Setenv("AUTH_ANONYMOUS_FROM", "")
	e := newBFFEnv(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "192.168.69.50:1"
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("GET /metrics from the LAN, AUTH_ANONYMOUS_FROM unset: %d, want 200", rec.Code)
	}
}
