// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package httputil

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func serveWith(t *testing.T, h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
}

func TestSecurityHeaders_Baseline(t *testing.T) {
	w := serveWith(t, SecurityHeaders(okHandler()), httptest.NewRequest(http.MethodGet, "/", nil))
	want := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "no-referrer",
		"Permissions-Policy":      "camera=(), microphone=(), geolocation=()",
		"Content-Security-Policy": DefaultCSP,
	}
	for k, v := range want {
		if got := w.Header().Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	if got := w.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("HSTS over plain HTTP = %q, want none", got)
	}
	if got := w.Header().Get("Cache-Control"); got != "" {
		t.Errorf("Cache-Control on / = %q, want none", got)
	}
}

func TestSecurityHeaders_HSTSOnlyOverTLS(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.TLS = &tls.ConnectionState{}
	w := serveWith(t, SecurityHeaders(okHandler()), r)
	if got := w.Header().Get("Strict-Transport-Security"); got != "max-age=31536000" {
		t.Errorf("HSTS = %q", got)
	}
}

func TestSecurityHeaders_NoStoreOnAPIAndAuth(t *testing.T) {
	for _, p := range []string{"/api/v1/x", "/auth/login", "/api", "/auth"} {
		w := serveWith(t, SecurityHeaders(okHandler()), httptest.NewRequest(http.MethodGet, p, nil))
		if got := w.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s Cache-Control = %q, want no-store", p, got)
		}
	}
	for _, p := range []string{"/apix", "/static/app.js", "/authors"} {
		w := serveWith(t, SecurityHeaders(okHandler()), httptest.NewRequest(http.MethodGet, p, nil))
		if got := w.Header().Get("Cache-Control"); got != "" {
			t.Errorf("%s Cache-Control = %q, want none", p, got)
		}
	}
}

func TestSecurityHeaders_WithCSP(t *testing.T) {
	const csp = "default-src 'self'; style-src 'self' 'unsafe-inline'; frame-ancestors 'none'; base-uri 'none'"
	w := serveWith(t, SecurityHeaders(okHandler(), WithCSP(csp)), httptest.NewRequest(http.MethodGet, "/", nil))
	if got := w.Header().Get("Content-Security-Policy"); got != csp {
		t.Errorf("CSP = %q", got)
	}
}

func TestSecurityHeaders_MetricsKeepsHandlerContentType(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte("up 1\n"))
	})
	w := serveWith(t, SecurityHeaders(h), httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if got := w.Header().Get("Content-Type"); got != "text/plain; version=0.0.4" {
		t.Errorf("Content-Type = %q", got)
	}
}

func TestSecurityHeaders_HandlerMayOverride(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.WriteHeader(http.StatusOK)
	})
	w := serveWith(t, SecurityHeaders(h), httptest.NewRequest(http.MethodGet, "/", nil))
	if got := w.Header().Get("X-Frame-Options"); got != "SAMEORIGIN" {
		t.Errorf("X-Frame-Options = %q, want the handler's value", got)
	}
}

func TestSecurityHeaders_WebSocketUpgradeUntouched(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.Header.Set("Connection", "Upgrade")
	r.Header.Set("Upgrade", "websocket")
	w := serveWith(t, SecurityHeaders(okHandler()), r)
	for _, k := range []string{"X-Content-Type-Options", "X-Frame-Options", "Content-Security-Policy", "Referrer-Policy"} {
		if got := w.Header().Get(k); got != "" {
			t.Errorf("%s on upgrade = %q, want none", k, got)
		}
	}
}
