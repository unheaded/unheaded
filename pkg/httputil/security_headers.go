// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package httputil

import (
	"net/http"
	"strings"
)

// DefaultCSP is the Content-Security-Policy SecurityHeaders sets unless a
// service passes WithCSP: same-origin everything, no framing, no <base>.
const DefaultCSP = "default-src 'self'; frame-ancestors 'none'; base-uri 'none'"

type securityHeadersConfig struct {
	csp string
}

// SecurityHeadersOption adjusts SecurityHeaders for one service.
type SecurityHeadersOption func(*securityHeadersConfig)

// WithCSP replaces DefaultCSP, for a UI that needs more than 'self' (for
// example inline styles). Keep frame-ancestors 'none' and base-uri 'none'.
func WithCSP(csp string) SecurityHeadersOption {
	return func(c *securityHeadersConfig) { c.csp = csp }
}

// SecurityHeaders sets the browser-facing security headers on every
// response (S-IRON-GATE Phase 11, M-056):
//
//   - X-Content-Type-Options: nosniff
//   - X-Frame-Options: DENY
//   - Referrer-Policy: no-referrer
//   - Permissions-Policy: camera=(), microphone=(), geolocation=()
//   - Content-Security-Policy: DefaultCSP, or the WithCSP value
//   - Cache-Control: no-store under /api and /auth
//   - Strict-Transport-Security: max-age=31536000, only when the request
//     arrived over TLS (HSTS sent over plain HTTP is ignored by browsers and
//     misleads a reader of the response)
//
// The headers are set before next runs, so a handler can still override one.
// Content-Type is never touched, so /metrics keeps its exposition type. A
// WebSocket upgrade passes through untouched.
func SecurityHeaders(next http.Handler, opts ...SecurityHeadersOption) http.Handler {
	cfg := securityHeadersConfig{csp: DefaultCSP}
	for _, o := range opts {
		o(&cfg)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isWebSocketUpgrade(r) {
			next.ServeHTTP(w, r)
			return
		}
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.Set("Content-Security-Policy", cfg.csp)
		if underPrefix(r.URL.Path, "/api") || underPrefix(r.URL.Path, "/auth") {
			h.Set("Cache-Control", "no-store")
		}
		if r.TLS != nil {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(w, r)
	})
}

// underPrefix reports whether p is prefix itself or below it ("/api",
// "/api/x"), not a sibling that merely starts with it ("/apix").
func underPrefix(p, prefix string) bool {
	return p == prefix || strings.HasPrefix(p, prefix+"/")
}

func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		headerHasToken(r.Header.Get("Connection"), "upgrade")
}

func headerHasToken(v, token string) bool {
	for _, part := range strings.Split(v, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}
