// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"net/http"
	"testing"

	"unheaded/pkg/auth"
	"unheaded/pkg/auth/authtest"
)

// S-IRON-GATE Phase 6 (Steps 179-182, wiki-server): the wiki is read by
// browsers, so every read stays open; it has no write route, and with auth
// on a write is refused for everyone: 401 without a token, 403 for any
// service token (the allowlist is empty until a user login exists).
func TestAuthE2E_WikiServer(t *testing.T) {
	f := authtest.New(t, "wiki-server", "unheaded-cli")
	h, err := wrapAuth(setupMux(setupTestWiki(t)), f.Config("wiki-server"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/health", "/ready", "/metrics", "/wiki/", "/wiki/Home"} {
		if c := authtest.Code(h, http.MethodGet, p, "", ""); !authtest.Authorized(c) {
			t.Errorf("GET %s anonymous: %d, want readable", p, c)
		}
	}
	if c := authtest.Code(h, http.MethodPost, "/wiki/Home", "x", ""); c != http.StatusUnauthorized {
		t.Errorf("POST /wiki/Home anonymous: %d, want 401", c)
	}
	if c := authtest.Code(h, http.MethodPost, "/wiki/Home", "x", f.Bearer("unheaded-cli", "wiki-server")); c != http.StatusForbidden {
		t.Errorf("POST /wiki/Home as unheaded-cli (no caller allowlisted): %d, want 403", c)
	}
	if c := authtest.Code(h, http.MethodPost, "/wiki/Home", "x", f.StrangerBearer("unheaded-cli", "wiki-server")); c != http.StatusUnauthorized {
		t.Errorf("POST /wiki/Home with a key trust.d lacks: %d, want 401", c)
	}
}

func TestAuthE2E_WikiServerRefusesBadConfig(t *testing.T) {
	if _, err := wrapAuth(http.NewServeMux(), auth.ServiceAuthConfig{Enabled: true, ServiceName: "wiki-server"}); err == nil {
		t.Error("auth on with nothing to verify against: no error, want refusal to start")
	}
}
