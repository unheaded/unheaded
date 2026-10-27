// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/metrics"
)

// testToken is built at run time so no key-shaped literal sits in the tree
// (gitleaks).
func testToken() string { return strings.Repeat("hb", 24) }

func writeToken(t *testing.T, mode os.FileMode, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "heartbeat-token")
	if err := os.WriteFile(p, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
	return p
}

func newHeartbeatTest(t *testing.T) (*Heartbeats, *clock) {
	t.Helper()
	c := &clock{time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)}
	env := map[string]string{
		"HEARTBEAT_HOSTS":      "east,west",
		"HEARTBEAT_TOKEN_FILE": writeToken(t, 0o400, testToken()+"\n"),
	}
	h, err := heartbeatFromEnv(func(k string) string { return env[k] }, c.now)
	if err != nil || h == nil {
		t.Fatalf("heartbeatFromEnv: %v, %v", h, err)
	}
	return h, c
}

func beat(h *Heartbeats, method, host, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/heartbeat/"+host, nil)
	if token != "" {
		req.Header.Set("X-Heartbeat-Token", token)
	}
	rec := httptest.NewRecorder()
	h.Handler().ServeHTTP(rec, req)
	return rec
}

func lastSeen(t *testing.T, h *Heartbeats, host string) float64 {
	t.Helper()
	var buf bytes.Buffer
	if err := h.Gauge().Write(&buf); err != nil {
		t.Fatal(err)
	}
	prefix := `unheaded_heartbeat_last_seen_timestamp_seconds{host="` + host + `"} `
	for _, line := range strings.Split(buf.String(), "\n") {
		if v, ok := strings.CutPrefix(line, prefix); ok {
			var f float64
			if _, err := fmt.Sscan(v, &f); err != nil {
				t.Fatalf("parse %q: %v", line, err)
			}
			return f
		}
	}
	t.Fatalf("no series for host %q in:\n%s", host, buf.String())
	return 0
}

// A configured host starts at the process start time, so a host that never
// beats is missing one full window after a restart, not never.
func TestHeartbeatStartsAtProcessStart(t *testing.T) {
	h, c := newHeartbeatTest(t)
	for _, host := range []string{"east", "west"} {
		if got := lastSeen(t, h, host); got != float64(c.t.Unix()) {
			t.Fatalf("%s: last seen %v at start, want %v", host, got, c.t.Unix())
		}
	}
}

func TestHeartbeatRightTokenUpdatesGauge(t *testing.T) {
	h, c := newHeartbeatTest(t)
	c.t = c.t.Add(5 * time.Minute)
	if rec := beat(h, http.MethodPost, "east", testToken()); rec.Code != http.StatusNoContent {
		t.Fatalf("POST with the right token: %d %s, want 204", rec.Code, rec.Body)
	}
	if got := lastSeen(t, h, "east"); got != float64(c.t.Unix()) {
		t.Fatalf("east last seen %v, want %v", got, c.t.Unix())
	}
}

func TestHeartbeatWrongTokenRefused(t *testing.T) {
	h, c := newHeartbeatTest(t)
	start := float64(c.t.Unix())
	c.t = c.t.Add(5 * time.Minute)
	for name, tok := range map[string]string{"wrong": strings.Repeat("x", 48), "missing": "", "prefix": testToken()[:10]} {
		if rec := beat(h, http.MethodPost, "east", tok); rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s token: %d, want 401", name, rec.Code)
		}
	}
	if got := lastSeen(t, h, "east"); got != start {
		t.Fatalf("a refused heartbeat moved east to %v", got)
	}
}

func TestHeartbeatUnknownHost(t *testing.T) {
	h, _ := newHeartbeatTest(t)
	if rec := beat(h, http.MethodPost, "north", testToken()); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown host: %d, want 404", rec.Code)
	}
	var buf bytes.Buffer
	_ = h.Gauge().Write(&buf)
	if strings.Contains(buf.String(), `host="north"`) {
		t.Fatalf("an unknown host got a series:\n%s", buf.String())
	}
}

func TestHeartbeatGetNotAllowed(t *testing.T) {
	h, _ := newHeartbeatTest(t)
	if rec := beat(h, http.MethodGet, "east", testToken()); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: %d, want 405", rec.Code)
	}
}

// The heartbeat listener serves the heartbeat and nothing else: east can
// reach it, so it must not reach the Alertmanager webhooks or the outbox.
func TestHeartbeatListenerServesNothingElse(t *testing.T) {
	h, _ := newHeartbeatTest(t)
	for _, p := range []string{"/v1/alerts/page", "/v1/alerts/ticket", "/v1/outbox", "/v1/status", "/metrics"} {
		req := httptest.NewRequest(http.MethodPost, p, strings.NewReader("{}"))
		req.Header.Set("X-Heartbeat-Token", testToken())
		rec := httptest.NewRecorder()
		h.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s on the heartbeat listener: %d, want 404", p, rec.Code)
		}
	}
}

func TestHeartbeatFromEnv(t *testing.T) {
	now := func() time.Time { return time.Unix(0, 0) }
	good := writeToken(t, 0o400, testToken())
	cases := []struct {
		name  string
		env   map[string]string
		off   bool
		wants string
	}{
		{"off when no hosts", map[string]string{}, true, ""},
		{"hosts without a token file", map[string]string{"HEARTBEAT_HOSTS": "east"}, false, "HEARTBEAT_TOKEN_FILE"},
		{"token file missing", map[string]string{"HEARTBEAT_HOSTS": "east", "HEARTBEAT_TOKEN_FILE": "/nonexistent/heartbeat-token"}, false, "heartbeat token"},
		{"token too short", map[string]string{"HEARTBEAT_HOSTS": "east", "HEARTBEAT_TOKEN_FILE": writeToken(t, 0o400, "short")}, false, "at least 32"},
		{"token world readable", map[string]string{"HEARTBEAT_HOSTS": "east", "HEARTBEAT_TOKEN_FILE": writeToken(t, 0o604, testToken())}, false, "readable by others"},
		{"bad host name", map[string]string{"HEARTBEAT_HOSTS": "east,../x", "HEARTBEAT_TOKEN_FILE": good}, false, "HEARTBEAT_HOSTS"},
		{"ok", map[string]string{"HEARTBEAT_HOSTS": " east ", "HEARTBEAT_TOKEN_FILE": good}, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h, err := heartbeatFromEnv(func(k string) string { return c.env[k] }, now)
			switch {
			case c.wants != "":
				if err == nil || !strings.Contains(err.Error(), c.wants) {
					t.Fatalf("want an error containing %q, got %v", c.wants, err)
				}
			case err != nil:
				t.Fatalf("unexpected error: %v", err)
			case c.off && h != nil:
				t.Fatal("heartbeat on with no hosts configured")
			case !c.off && h == nil:
				t.Fatal("heartbeat off with hosts configured")
			}
		})
	}
}

// The gauge registers on the mailer's registry like its other metrics.
func TestHeartbeatGaugeRegisters(t *testing.T) {
	h, _ := newHeartbeatTest(t)
	reg := metrics.NewRegistry()
	reg.MustRegister(h.Gauge())
}
