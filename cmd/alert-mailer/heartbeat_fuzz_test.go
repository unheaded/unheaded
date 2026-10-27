// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// FuzzHeartbeatRequest drives the heartbeat listener (the one alert-mailer
// endpoint east reaches) with arbitrary method, host path segment, token and
// body. Invariants: no panic; only POST with the exact token and a watched
// host gets 204; nothing else moves a last-seen time or adds a host label.
// S-WELL-SPRING Step 286: go test ./cmd/alert-mailer/ -run '^$' -fuzz=FuzzHeartbeatRequest -fuzztime=60s
func FuzzHeartbeatRequest(f *testing.F) {
	f.Add("POST", "east", testToken(), "")
	f.Add("POST", "west", testToken()+" ", "x")
	f.Add("GET", "east", testToken(), "")
	f.Add("POST", "north", testToken(), "")
	f.Add("POST", "east", "", "")
	f.Add("POST", "../metrics", testToken(), "")
	f.Add("POST", "east%00", strings.Repeat("A", 4096), strings.Repeat("b", 2048))
	f.Fuzz(func(t *testing.T, method, host, token, body string) {
		h, c := newHeartbeatTest(t)
		before := lastSeen(t, h, "east")
		target := "/heartbeat/" + url.PathEscape(host)
		req, err := http.NewRequest(method, "http://alert-mailer"+target, strings.NewReader(body))
		if err != nil {
			return // not a request a client could send
		}
		if token != "" {
			req.Header.Set(heartbeatHeader, token)
		}
		c.t = c.t.Add(60e9)
		rec := httptest.NewRecorder()
		h.Handler().ServeHTTP(rec, req)
		ok := method == http.MethodPost && token == testToken() && (host == "east" || host == "west")
		if (rec.Code == http.StatusNoContent) != ok {
			t.Fatalf("%s %q token=%v: status %d", method, host, token == testToken(), rec.Code)
		}
		if !ok && lastSeen(t, h, "east") != before {
			t.Fatal("a refused request moved east's last-seen time")
		}
		var buf strings.Builder
		if err := h.Gauge().Write(&buf); err != nil {
			t.Fatal(err)
		}
		if n := strings.Count(buf.String(), "unheaded_heartbeat_last_seen_timestamp_seconds{"); n != 2 {
			t.Fatalf("%d host series, want 2 (a request created one)", n)
		}
	})
}
