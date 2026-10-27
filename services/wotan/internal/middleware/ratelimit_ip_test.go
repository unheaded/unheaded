// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// The limiter keys on the client IP, not IP:port: every new connection
// from one client has a fresh source port, so keying on RemoteAddr gave each
// connection its own bucket and a client could dodge the limit by
// reconnecting.
func TestRateLimiter_KeysOnClientIP(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(0.001), 1, time.Hour)
	h := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	do := func(remote string) int {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/topics", nil)
		req.RemoteAddr = remote
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	if got := do("1.2.3.4:1000"); got != http.StatusOK {
		t.Fatalf("first request: %d, want 200", got)
	}
	if got := do("1.2.3.4:1001"); got != http.StatusTooManyRequests {
		t.Errorf("same IP, new port: %d, want 429 (one bucket per IP)", got)
	}
	if got := do("5.6.7.8:1000"); got != http.StatusOK {
		t.Errorf("other IP: %d, want 200", got)
	}
	if got := do("[2001:db8::1]:1000"); got != http.StatusOK {
		t.Errorf("IPv6 first: %d, want 200", got)
	}
	if got := do("[2001:db8::1]:2000"); got != http.StatusTooManyRequests {
		t.Errorf("IPv6 same IP, new port: %d, want 429", got)
	}
}

func TestClientIP(t *testing.T) {
	for in, want := range map[string]string{
		"1.2.3.4:1000":        "1.2.3.4",
		"[2001:db8::1]:443":   "2001:db8::1",
		"not-a-host-port":     "not-a-host-port", // fall back to the raw string
		"":                    "",
		"@/run/wotan.sock:0":  "@/run/wotan.sock",
		"unix-socket-no-port": "unix-socket-no-port",
	} {
		if got := clientIP(in); got != want {
			t.Errorf("clientIP(%q) = %q, want %q", in, got, want)
		}
	}
}
