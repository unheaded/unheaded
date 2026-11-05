// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"errors"
	"flag"
	"net/http"
	"os"
	"strings"
	"testing"

	"unheaded/pkg/auth"
	"unheaded/pkg/auth/authtest"
	"unheaded/pkg/ports"
)

// M10: with no flags the collector binds loopback, in both modes.
func TestDefaultBindIsLoopback(t *testing.T) {
	for name, port := range map[string]int{
		"http-addr":    ports.TraceCollectorHTTP,
		"metrics-addr": ports.TraceCollectorMetrics,
	} {
		if got, want := flag.Lookup(name).DefValue, ports.LoopbackAddr(port); got != want {
			t.Errorf("--%s default = %q, want %q", name, got, want)
		}
	}
}

// M10: a listener that reaches a network needs auth on.
func TestOpenListeners_RefusesOpenBindWithAuthOff(t *testing.T) {
	if _, err := openListeners("127.0.0.1:0,0.0.0.0:0", auth.ServiceAuthConfig{}); !errors.Is(err, auth.ErrOpenListener) {
		t.Fatalf("every-interface bind with auth off: %v, want ErrOpenListener", err)
	}
	lns, err := openListeners("127.0.0.1:0", auth.ServiceAuthConfig{})
	if err != nil {
		t.Fatalf("loopback with auth off: %v", err)
	}
	for _, ln := range lns {
		_ = ln.Close()
	}
}

// M10: flow data needs a credential; the probes and the scrape do not.
func TestAPIHandler_RequiresAuth(t *testing.T) {
	mux := http.NewServeMux()
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	for _, p := range []string{"/health", "/ready", "/metrics", "/api/v1/traces", "/api/v1/stats"} {
		mux.Handle(p, ok)
	}
	f := authtest.New(t, "trace-collector", "unheaded-cli")
	h := apiHandler(mux, f.Config("trace-collector", "unheaded-cli"))

	for _, p := range []string{"/health", "/ready", "/metrics"} {
		if c := authtest.Code(h, http.MethodGet, p, "", ""); c != http.StatusOK {
			t.Errorf("GET %s anonymous: %d, want 200", p, c)
		}
	}
	for _, p := range []string{"/api/v1/traces", "/api/v1/stats"} {
		if c := authtest.Code(h, http.MethodGet, p, "", ""); c != http.StatusUnauthorized {
			t.Errorf("GET %s anonymous: %d, want 401", p, c)
		}
		if c := authtest.Code(h, http.MethodGet, p, "", f.Bearer("unheaded-cli", "trace-collector")); c != http.StatusOK {
			t.Errorf("GET %s with a token: %d, want 200", p, c)
		}
	}

	// A host with no keys yet: the collector keeps running and the flow
	// data stays closed, not open.
	bad := f.Config("trace-collector", "unheaded-cli")
	bad.TrustDir = ""
	h = apiHandler(mux, bad)
	if c := authtest.Code(h, http.MethodGet, "/api/v1/traces", "", ""); c != http.StatusUnauthorized {
		t.Errorf("GET /api/v1/traces with auth on and nothing to verify against: %d, want 401", c)
	}
	if c := authtest.Code(h, http.MethodGet, "/metrics", "", ""); c != http.StatusOK {
		t.Errorf("GET /metrics with auth on and nothing to verify against: %d, want 200", c)
	}
}

// M10: the shipped unit binds loopback unless the host's env file names its
// internal address, and turns auth on.
func TestUnit_InternalBindAndAuth(t *testing.T) {
	raw, err := os.ReadFile("../../deploy/systemd/unheaded-trace-collector.service")
	if err != nil {
		t.Fatal(err)
	}
	unit := string(raw)
	for _, want := range []string{
		"\nEnvironment=AUTH_ENABLED=true\n",
		"\nEnvironment=TRACE_METRICS_ADDR=127.0.0.1:16671\n",
		"--metrics-addr ${TRACE_METRICS_ADDR}",
	} {
		if !strings.Contains(unit, want) {
			t.Errorf("unit lacks %q", want)
		}
	}
}
