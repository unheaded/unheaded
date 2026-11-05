// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package gateway

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"unheaded/pkg/logger"
	"unheaded/services/gateway/config"
)

// loggedClient sends one request through the whole middleware chain and
// returns the request log line.
func loggedClient(t *testing.T, trusted []string, peer, xff, xri string) string {
	t.Helper()
	var out bytes.Buffer
	cfg := config.DefaultConfig()
	cfg.Auth.Enabled = false
	cfg.Wotan.Enabled = false
	cfg.RateLimit.TrustedProxies = trusted
	gw, err := New(cfg, logger.New(&out))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/nowhere", nil)
	req.RemoteAddr = peer
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	if xri != "" {
		req.Header.Set("X-Real-IP", xri)
	}
	gw.buildHandler().ServeHTTP(httptest.NewRecorder(), req)
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.Contains(line, "HTTP request") {
			return line
		}
	}
	t.Fatalf("no request log line in %q", out.String())
	return ""
}

// L8: the log names the client the limiter and the backends name. A peer that
// is not a trusted proxy is logged as itself, whatever it forwards.
func TestRequestLog_UntrustedPeerIsThePeer(t *testing.T) {
	for name, c := range map[string]struct{ xff, xri string }{
		"forwarded for": {"1.2.3.4", ""},
		"real ip":       {"", "1.2.3.4"},
		"not an ip":     {"made-up-text", ""},
	} {
		t.Run(name, func(t *testing.T) {
			line := loggedClient(t, nil, "203.0.113.9:4000", c.xff, c.xri)
			if !strings.Contains(line, "203.0.113.9") {
				t.Fatalf("log line lacks the peer: %s", line)
			}
			if strings.Contains(line, "1.2.3.4") || strings.Contains(line, "made-up-text") {
				t.Fatalf("log line carries the peer's forwarded address: %s", line)
			}
		})
	}
}

// A trusted proxy's word stands for the hop it saw; a client's entry left of
// that is not logged as the client.
func TestRequestLog_TrustedPeerNamesTheClient(t *testing.T) {
	line := loggedClient(t, []string{"192.0.2.1"}, "192.0.2.1:4000", "6.6.6.6, 1.2.3.4", "")
	if !strings.Contains(line, "1.2.3.4") {
		t.Fatalf("log line lacks the forwarded client: %s", line)
	}
	if strings.Contains(line, "6.6.6.6") {
		t.Fatalf("log line carries the client's own entry: %s", line)
	}
}
