// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"io"
	"net/http"
	"testing"

	"unheaded/pkg/auth"
	"unheaded/pkg/auth/authtest"
	"unheaded/pkg/logger"
	"unheaded/services/sophia"
)

// S-IRON-GATE Phase 6 (Steps 179-182, sophia): unheaded-cli and
// dashboard-backend may call; /health, /ready and /metrics stay open.
func TestAuthE2E_Sophia(t *testing.T) {
	f := authtest.New(t, "sophia", "unheaded-cli", "dashboard-backend", "sword")
	svc := sophia.NewService(logger.New(io.Discard), nil, nil)
	hs, err := newHTTPServer(svc, nil, logger.New(io.Discard), ":0", f.Config("sophia", "unheaded-cli", "dashboard-backend"))
	if err != nil {
		t.Fatal(err)
	}
	h := hs.server.Handler
	const write, body = "/api/v1/knowledge", `{"key":"auth-e2e","value":"v"}`

	for _, p := range []string{"/health", "/ready", "/metrics"} {
		if c := authtest.Code(h, http.MethodGet, p, "", ""); !authtest.Authorized(c) {
			t.Errorf("GET %s anonymous: %d, want open", p, c)
		}
	}
	if c := authtest.Code(h, http.MethodGet, "/api/v1/stats", "", ""); c != http.StatusUnauthorized {
		t.Errorf("GET /api/v1/stats anonymous: %d, want 401", c)
	}
	if c := authtest.Code(h, http.MethodPost, write, body, ""); c != http.StatusUnauthorized {
		t.Errorf("POST %s anonymous: %d, want 401", write, c)
	}
	for _, caller := range []string{"unheaded-cli", "dashboard-backend"} {
		if c := authtest.Code(h, http.MethodPost, write, body, f.Bearer(caller, "sophia")); !authtest.Authorized(c) {
			t.Errorf("POST %s as %s: %d, want past auth", write, caller, c)
		}
	}
	if c := authtest.Code(h, http.MethodPost, write, body, f.StrangerBearer("unheaded-cli", "sophia")); c != http.StatusUnauthorized {
		t.Errorf("POST %s with a key trust.d lacks: %d, want 401", write, c)
	}
	if c := authtest.Code(h, http.MethodPost, write, body, f.Bearer("sword", "sophia")); c != http.StatusForbidden {
		t.Errorf("POST %s as trusted, not allowlisted sword: %d, want 403", write, c)
	}
}

func TestAuthE2E_SophiaRefusesBadConfig(t *testing.T) {
	svc := sophia.NewService(logger.New(io.Discard), nil, nil)
	if _, err := newHTTPServer(svc, nil, logger.New(io.Discard), ":0", auth.ServiceAuthConfig{Enabled: true, ServiceName: "sophia"}); err == nil {
		t.Error("auth on with nothing to verify against: no error, want refusal to start")
	}
}
