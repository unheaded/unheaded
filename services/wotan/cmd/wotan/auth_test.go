// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"errors"
	"net/http"
	"testing"

	"unheaded/pkg/auth"
	"unheaded/pkg/auth/authtest"
)

func TestAuthE2E_WotanAuthWiring(t *testing.T) {
	mux := http.NewServeMux()
	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
	mux.HandleFunc("/health", ok)
	mux.HandleFunc("/ready", ok)
	mux.HandleFunc("/metrics", ok)
	mux.HandleFunc("/api/v1/", ok)

	// Off: the handler unchanged, no gRPC options.
	h, opts, err := wotanAuth(mux, auth.ServiceAuthConfig{})
	if err != nil || h != http.Handler(mux) || opts != nil {
		t.Fatalf("auth off: (%v, %v, %v), want the handler as is", h, opts, err)
	}

	f := authtest.New(t, "wotan", "timeguru")
	h, opts, err = wotanAuth(mux, f.Config("wotan", "timeguru"))
	if err != nil || len(opts) != 2 {
		t.Fatalf("auth on: opts=%d err=%v, want the unary and stream interceptors", len(opts), err)
	}
	for _, p := range []string{"/health", "/ready", "/metrics"} {
		if c := authtest.Code(h, http.MethodGet, p, "", ""); c != http.StatusOK {
			t.Errorf("%s = %d, want 200 without a token", p, c)
		}
	}
	if c := authtest.Code(h, http.MethodGet, "/api/v1/topics", "", ""); c != http.StatusUnauthorized {
		t.Errorf("anonymous read = %d, want 401 (topics carry every service's logs)", c)
	}
	if c := authtest.Code(h, http.MethodPost, "/api/v1/topics/x/publish", "{}", ""); c != http.StatusUnauthorized {
		t.Errorf("anonymous publish = %d, want 401", c)
	}
	if c := authtest.Code(h, http.MethodPost, "/api/v1/topics/x/publish", "{}", f.Bearer("timeguru", "wotan")); c != http.StatusOK {
		t.Errorf("tokened publish = %d, want 200", c)
	}
	if c := authtest.Code(h, http.MethodGet, "/api/v1/admin/pending", "", f.Bearer("timeguru", "wotan")); c != http.StatusForbidden {
		t.Errorf("service token on the admin API = %d, want 403 (admin role)", c)
	}

	// On with nothing to verify against: refuse to start, never serve open.
	_, _, err = wotanAuth(mux, auth.ServiceAuthConfig{Enabled: true, ServiceName: "wotan"})
	if !errors.Is(err, auth.ErrNoCredentials) {
		t.Fatalf("auth on without credentials: %v, want ErrNoCredentials", err)
	}
}
