// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package response

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRedirectToHTTPS_HostHeaderOpenRedirect is the regression test for a real
// open redirect: RedirectToHTTPS/WWW/NonWWW interpolated r.Host — the
// client-supplied Host header — straight into the Location header. A request
// carrying `Host: evil.com` produced `301 Location: https://evil.com/...`,
// served by the WAF.
func TestRedirectToHTTPS_HostHeaderOpenRedirect(t *testing.T) {
	rh := NewRedirectHandler("https://kingdom.internal/")
	rh.SetAllowedHosts([]string{"kingdom.internal"})

	tests := []struct {
		name       string
		host       string
		wantAttack bool // Location points at the attacker host
	}{
		{"attacker host", "evil.com", false},
		{"lookalike suffix", "kingdom.internal.evil.com", false},
		{"legitimate host", "kingdom.internal", true},
		{"legitimate subdomain", "api.kingdom.internal", true},
		{"legitimate host with port", "kingdom.internal:8443", true},
		{"empty host", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://placeholder/path?a=1", nil)
			req.Host = tt.host
			w := httptest.NewRecorder()

			rh.RedirectToHTTPS(w, req)

			loc := w.Result().Header.Get("Location")
			if tt.wantAttack {
				if !strings.Contains(loc, tt.host) && !strings.Contains(loc, strings.Split(tt.host, ":")[0]) {
					t.Errorf("legitimate host %q should be honoured, got Location %q", tt.host, loc)
				}
				return
			}
			if strings.Contains(loc, "evil.com") {
				t.Errorf("OPEN REDIRECT: Host %q produced Location %q", tt.host, loc)
			}
			if loc != "https://kingdom.internal/" {
				t.Errorf("untrusted Host should fall back to defaultURL, got %q", loc)
			}
		})
	}
}

// TestHostDerivedRedirects_FailClosed asserts the deliberate default: with no
// allowlist configured, NO Host is trusted. Failing closed matters more than
// convenience here — an unconfigured handler must not be exploitable.
func TestHostDerivedRedirects_FailClosed(t *testing.T) {
	rh := NewRedirectHandler("https://safe.example/")

	for _, fn := range []struct {
		name string
		call func(http.ResponseWriter, *http.Request)
	}{
		{"RedirectToHTTPS", rh.RedirectToHTTPS},
		{"RedirectToWWW", rh.RedirectToWWW},
		{"RedirectToNonWWW", rh.RedirectToNonWWW},
	} {
		t.Run(fn.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://placeholder/p", nil)
			req.Host = "attacker.test"
			w := httptest.NewRecorder()

			fn.call(w, req)

			if loc := w.Result().Header.Get("Location"); strings.Contains(loc, "attacker.test") {
				t.Errorf("OPEN REDIRECT with no allowlist configured: Location %q", loc)
			}
		})
	}
}
