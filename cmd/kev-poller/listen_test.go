// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"unheaded/pkg/auth"
	"unheaded/pkg/auth/authtest"
	"unheaded/pkg/threatfeed/kev"
)

// L11: with no flags the poller binds loopback; --port keeps meaning the
// port, and --listen names addresses.
func TestListenList(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{nil, "127.0.0.1:19120"},
		{[]string{"--port", "19121"}, "127.0.0.1:19121"},
		{[]string{"--listen", ":19120"}, ":19120"},
	} {
		c, err := parseFlags(tc.args, io.Discard, io.Discard)
		if err != nil {
			t.Fatal(err)
		}
		if got := c.listenList(); got != tc.want {
			t.Errorf("%v: listen = %q, want %q", tc.args, got, tc.want)
		}
	}
}

// L11: a listener off loopback needs auth on.
func TestOpenListeners_RefusesOpenBindWithAuthOff(t *testing.T) {
	if _, err := openListeners(":0", auth.ServiceAuthConfig{}); !errors.Is(err, auth.ErrOpenListener) {
		t.Fatalf("every-interface bind with auth off: %v, want ErrOpenListener", err)
	}
	lns, err := openListeners("127.0.0.1:0", auth.ServiceAuthConfig{})
	if err != nil {
		t.Fatal(err)
	}
	for _, ln := range lns {
		_ = ln.Close()
	}
}

// L11: the probes and the Prometheus scrape stay open; the API needs a
// credential, and stays closed on a host with no trust directory.
func TestHandler_Auth(t *testing.T) {
	f := authtest.New(t, "kev-poller", "unheaded-cli")
	serve := func(cfg auth.ServiceAuthConfig) http.Handler {
		d := &daemon{poller: &kev.Poller{Store: kev.Store{Dir: t.TempDir()}}, metrics: newPollMetrics(), auth: auth.SetupMiddleware(cfg)}
		return d.handler()
	}
	h := serve(f.Config("kev-poller", "unheaded-cli"))
	if c := authtest.Code(h, http.MethodGet, "/health", "", ""); c != http.StatusOK {
		t.Errorf("GET /health anonymous: %d, want 200", c)
	}
	if c := authtest.Code(h, http.MethodGet, "/metrics", "", ""); c != http.StatusOK {
		t.Errorf("GET /metrics anonymous: %d, want 200", c)
	}
	if c := authtest.Code(h, http.MethodGet, "/api/v1/kev", "", ""); c != http.StatusUnauthorized {
		t.Errorf("GET /api/v1/kev anonymous: %d, want 401", c)
	}
	if c := authtest.Code(h, http.MethodGet, "/api/v1/kev", "", f.Bearer("unheaded-cli", "kev-poller")); c != http.StatusOK {
		t.Errorf("GET /api/v1/kev with a token: %d, want 200", c)
	}

	bad := f.Config("kev-poller", "unheaded-cli")
	bad.TrustDir = ""
	h = serve(bad)
	if c := authtest.Code(h, http.MethodGet, "/api/v1/kev", "", ""); c != http.StatusUnauthorized {
		t.Errorf("GET /api/v1/kev, auth on and nothing to verify against: %d, want 401", c)
	}
	if c := authtest.Code(h, http.MethodGet, "/metrics", "", ""); c != http.StatusOK {
		t.Errorf("GET /metrics, auth on and nothing to verify against: %d, want 200", c)
	}
}

// L11: the shipped unit keeps the scrape's bind and turns auth on.
func TestUnit_AuthOn(t *testing.T) {
	raw, err := os.ReadFile("../../deploy/systemd/unheaded-kev-poller.service")
	if err != nil {
		t.Fatal(err)
	}
	unit := string(raw)
	for _, want := range []string{"\nEnvironment=AUTH_ENABLED=true\n", "\nEnvironment=KEV_LISTEN=:19120\n", "--listen ${KEV_LISTEN}"} {
		if !strings.Contains(unit, want) {
			t.Errorf("unit lacks %q", want)
		}
	}
}
