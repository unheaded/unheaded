// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"unheaded/pkg/auth"
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

// With auth off the dashboard never builds the chain, so it must say itself
// that AUTH_ANONYMOUS_FROM limits nothing (review 2026-11-15, REV-3).
// Not parallel: it swaps the package-wide logger.
func TestAnonymousFromWithAuthOffWarns(t *testing.T) {
	var buf bytes.Buffer
	saved := log.Logger
	log.Logger = zerolog.New(&buf)
	t.Cleanup(func() { log.Logger = saved })
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	cfg := auth.ServiceAuthConfig{ServiceName: "dashboard-backend", AnonymousFrom: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}}
	if _, err := (&Server{}).wrapAuth(cfg, next); err != nil {
		t.Fatalf("auth off: %v", err)
	}
	if !strings.Contains(buf.String(), `"level":"warn"`) || !strings.Contains(buf.String(), "AUTH_ANONYMOUS_FROM") {
		t.Errorf("auth off with AUTH_ANONYMOUS_FROM: no warning; log: %q", buf.String())
	}
	buf.Reset()
	cfg.AnonymousFrom = nil
	if _, err := (&Server{}).wrapAuth(cfg, next); err != nil {
		t.Fatalf("auth off: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("auth off, AUTH_ANONYMOUS_FROM unset: logged %q, want nothing", buf.String())
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
