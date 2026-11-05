// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"unheaded/services/gateway/config"
)

// forwardedTo proxies one request and returns what the backend was told.
func forwardedTo(t *testing.T, trusted []string, peer, xff, xri string) (string, string) {
	t.Helper()
	var seen http.Header
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()
	p := NewReverseProxy(&config.RouteConfig{Name: "ip", Timeout: 5 * time.Second}, testLogger(), nil,
		NewRoundRobinLoadBalancer([]string{backend.URL}, testLogger()), nil)
	defer p.Close()
	setTrusted(p, trusted)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = peer
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	if xri != "" {
		req.Header.Set("X-Real-IP", xri)
	}
	p.ServeHTTP(httptest.NewRecorder(), req)
	return seen.Get("X-Forwarded-For"), seen.Get("X-Real-IP")
}

// L8: a client's own forwarding headers never reach the backend as its
// address.
func TestForwardedAddress_UntrustedPeerIsThePeer(t *testing.T) {
	for _, peer := range []string{"203.0.113.9:4000", "203.0.113.9"} {
		xff, xri := forwardedTo(t, nil, peer, "1.2.3.4", "1.2.3.5")
		if xff != "203.0.113.9" || xri != "203.0.113.9" {
			t.Fatalf("peer %q: backend told %q and %q, want 203.0.113.9", peer, xff, xri)
		}
	}
}

// A trusted proxy's word stands for the hop it saw. Anything left of that
// came from the client.
func TestForwardedAddress_TrustedPeer(t *testing.T) {
	const proxyAddr = "192.0.2.1"
	trusted := []string{proxyAddr, "192.0.2.2"}
	for _, c := range []struct{ name, xff, xri, want string }{
		{"one hop", "1.2.3.4", "", "1.2.3.4"},
		{"client prepended an entry", "6.6.6.6, 1.2.3.4", "", "1.2.3.4"},
		{"two trusted hops", "1.2.3.4, 192.0.2.2", "", "1.2.3.4"},
		{"unparsable nearest entry", "6.6.6.6, not-an-ip", "", proxyAddr},
		{"X-Real-IP only", "", "1.2.3.4", "1.2.3.4"},
		{"nothing forwarded", "", "", proxyAddr},
	} {
		t.Run(c.name, func(t *testing.T) {
			xff, xri := forwardedTo(t, trusted, proxyAddr+":4000", c.xff, c.xri)
			if xff != c.want || xri != c.want {
				t.Fatalf("backend told %q and %q, want %q", xff, xri, c.want)
			}
		})
	}
}

func setTrusted(p *ReverseProxy, trusted []string) { p.SetTrustedProxies(trusted) }
