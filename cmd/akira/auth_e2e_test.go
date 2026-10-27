// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"net/http"
	"testing"

	"unheaded/pkg/auth/authtest"
	"unheaded/pkg/health"
)

// fakeAkira answers the API's reads.
type fakeAkira struct{ fakeSweeper }

func (fakeAkira) GetStates() map[string]health.Verdict { return map[string]health.Verdict{} }

// S-IRON-GATE Phase 6 recipe for Akira: auth on, /health /ready /metrics
// and the consensus read /api/v1/status open (runbooks and game days curl
// it; health bands, no user data), every other route and every write needs
// a token. No service calls Akira's API, so AUTH_ALLOW_CALLERS is empty and
// a trusted service token is 403.
func TestAuthE2E_Akira(t *testing.T) {
	f := authtest.New(t, "akira", "dashboard-backend", "unheaded-cli")
	h, err := apiHandler(fakeAkira{1}, "west", f.Config("akira", "unheaded-cli"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/health", "/ready", "/metrics", "/api/v1/status"} {
		if c := authtest.Code(h, http.MethodGet, p, "", ""); c != http.StatusOK {
			t.Errorf("GET %s = %d, want 200 without a token", p, c)
		}
	}
	if c := authtest.Code(h, http.MethodPost, "/api/v1/status", "{}", ""); c != http.StatusUnauthorized {
		t.Errorf("anonymous write = %d, want 401", c)
	}
	if c := authtest.Code(h, http.MethodPost, "/api/v1/status", "{}", f.StrangerBearer("unheaded-cli", "akira")); c != http.StatusUnauthorized {
		t.Errorf("untrusted kid = %d, want 401", c)
	}
	if c := authtest.Code(h, http.MethodPost, "/api/v1/status", "{}", f.Bearer("dashboard-backend", "akira")); c != http.StatusForbidden {
		t.Errorf("trusted, not allowlisted = %d, want 403", c)
	}
	if c := authtest.Code(h, http.MethodPost, "/api/v1/status", "{}", f.Bearer("unheaded-cli", "akira")); !authtest.Authorized(c) {
		t.Errorf("allowlisted service token = %d, want past auth", c)
	}
	if c := authtest.Code(h, http.MethodGet, "/api/v1/other", "", ""); c != http.StatusUnauthorized {
		t.Errorf("anonymous read outside /api/v1/status = %d, want 401", c)
	}

	// Off: the API as before.
	off := f.Config("akira")
	off.Enabled = false
	h, err = apiHandler(fakeAkira{1}, "west", off)
	if err != nil {
		t.Fatal(err)
	}
	if c := authtest.Code(h, http.MethodPost, "/api/v1/status", "{}", ""); !authtest.Authorized(c) {
		t.Errorf("auth off: write = %d, want past auth", c)
	}
}
