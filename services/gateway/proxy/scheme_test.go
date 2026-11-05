// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package proxy

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"unheaded/services/gateway/config"
)

// schemeForwarded proxies one request and returns the X-Forwarded-Proto the
// backend was told. Each element of lines is one header line.
func schemeForwarded(t *testing.T, trusted []string, peer string, viaTLS bool, lines ...string) string {
	t.Helper()
	var seen string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("X-Forwarded-Proto")
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()
	p := NewReverseProxy(&config.RouteConfig{Name: "scheme", Timeout: 5 * time.Second}, testLogger(), nil,
		NewRoundRobinLoadBalancer([]string{backend.URL}, testLogger()), nil)
	defer p.Close()
	p.SetTrustedProxies(trusted)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = peer
	if viaTLS {
		req.TLS = &tls.ConnectionState{}
	}
	for _, l := range lines {
		req.Header.Add("X-Forwarded-Proto", l)
	}
	p.ServeHTTP(httptest.NewRecorder(), req)
	return seen
}

// L8: a peer that is not a trusted proxy does not choose the scheme the
// backend is told.
func TestForwardedProto_UntrustedPeerIsNotBelieved(t *testing.T) {
	if got := schemeForwarded(t, nil, "203.0.113.9:4000", false, "https"); got != "http" {
		t.Fatalf("plain request claiming https: backend told %q, want http", got)
	}
	if got := schemeForwarded(t, []string{"192.0.2.1"}, "203.0.113.9:4000", false, "https"); got != "http" {
		t.Fatalf("peer outside the trusted list: backend told %q, want http", got)
	}
}

// A trusted proxy's word stands for the hop it saw, by the rule ClientIP
// follows: the nearest entry decides, and a line or entry further out is the
// client's.
func TestForwardedProto_TrustedPeer(t *testing.T) {
	const proxyAddr = "192.0.2.1"
	for _, c := range []struct {
		name  string
		lines []string
		want  string
	}{
		{"https", []string{"https"}, "https"},
		{"http", []string{"http"}, "http"},
		{"any case", []string{"HTTPS"}, "https"},
		{"nothing forwarded", nil, "http"},
		{"client line before the proxy's", []string{"https", "http"}, "http"},
		{"client entry before the proxy's", []string{"https, http"}, "http"},
		{"not a scheme", []string{"gopher"}, "http"},
		{"not a scheme after a good one", []string{"https, gopher"}, "http"},
		{"empty nearest entry", []string{"https,"}, "http"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := schemeForwarded(t, []string{proxyAddr}, proxyAddr+":4000", false, c.lines...); got != c.want {
				t.Fatalf("backend told %q, want %q", got, c.want)
			}
		})
	}
}

// TLS the gateway ended itself needs nobody's word.
func TestForwardedProto_OwnTLSWins(t *testing.T) {
	const proxyAddr = "192.0.2.1"
	if got := schemeForwarded(t, []string{proxyAddr}, proxyAddr+":4000", true, "http"); got != "https" {
		t.Fatalf("backend told %q, want https", got)
	}
}

// The backend hears about the client from the gateway alone: a forwarding
// header the gateway does not state itself does not get through, from any peer.
func TestForwardingHeaders_OnlyTheGatewaysOwnReachTheBackend(t *testing.T) {
	const proxyAddr = "192.0.2.1"
	for _, c := range []struct {
		name    string
		trusted []string
		peer    string
	}{
		{"untrusted peer", nil, "203.0.113.9:4000"},
		{"trusted proxy", []string{proxyAddr}, proxyAddr + ":4000"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var seen http.Header
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen = r.Header.Clone()
				w.WriteHeader(http.StatusOK)
			}))
			defer backend.Close()
			p := NewReverseProxy(&config.RouteConfig{Name: "forwarding", Timeout: 5 * time.Second}, testLogger(), nil,
				NewRoundRobinLoadBalancer([]string{backend.URL}, testLogger()), nil)
			defer p.Close()
			p.SetTrustedProxies(c.trusted)

			req := httptest.NewRequest(http.MethodGet, "http://gateway.test/", nil)
			req.RemoteAddr = c.peer
			req.Header.Set("Forwarded", "for=192.0.2.60;proto=https")
			req.Header.Set("X-Forwarded-Port", "8443")
			req.Header.Set("X-Forwarded-Prefix", "/made-up")
			req.Header.Set("X-Keep", "this one is the client's own")
			p.ServeHTTP(httptest.NewRecorder(), req)

			for _, h := range []string{"Forwarded", "X-Forwarded-Port", "X-Forwarded-Prefix"} {
				if got := seen.Values(h); len(got) != 0 {
					t.Errorf("%s reached the backend: %q", h, got)
				}
			}
			for _, h := range []string{"X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Keep"} {
				if seen.Get(h) == "" {
					t.Errorf("%s is missing at the backend", h)
				}
			}
		})
	}
}
