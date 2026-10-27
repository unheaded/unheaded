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
	"unheaded/services/monad"
)

// S-IRON-GATE Phase 6 (Steps 179-182, monad): unheaded-cli and
// dashboard-backend may call; /health, /ready and /metrics stay open.
func TestAuthE2E_Monad(t *testing.T) {
	f := authtest.New(t, "monad", "unheaded-cli", "dashboard-backend", "sword")
	svc := monad.NewService(logger.New(io.Discard), nil)
	hs, err := newHTTPServer(svc, logger.New(io.Discard), ":0", f.Config("monad", "unheaded-cli", "dashboard-backend"))
	if err != nil {
		t.Fatal(err)
	}
	h := hs.server.Handler
	const write, body = "/api/v1/operations", `{"type":"noop"}`

	for _, p := range []string{"/health", "/ready", "/metrics"} {
		if c := authtest.Code(h, http.MethodGet, p, "", ""); c != http.StatusOK {
			t.Errorf("GET %s anonymous: %d, want 200", p, c)
		}
	}
	if c := authtest.Code(h, http.MethodGet, "/api/v1/stats", "", ""); c != http.StatusUnauthorized {
		t.Errorf("GET /api/v1/stats anonymous: %d, want 401", c)
	}
	if c := authtest.Code(h, http.MethodPost, write, body, ""); c != http.StatusUnauthorized {
		t.Errorf("POST %s anonymous: %d, want 401", write, c)
	}
	for _, caller := range []string{"unheaded-cli", "dashboard-backend"} {
		if c := authtest.Code(h, http.MethodPost, write, body, f.Bearer(caller, "monad")); !authtest.Authorized(c) {
			t.Errorf("POST %s as %s: %d, want past auth", write, caller, c)
		}
	}
	if c := authtest.Code(h, http.MethodPost, write, body, f.StrangerBearer("unheaded-cli", "monad")); c != http.StatusUnauthorized {
		t.Errorf("POST %s with a key trust.d lacks: %d, want 401", write, c)
	}
	if c := authtest.Code(h, http.MethodPost, write, body, f.Bearer("sword", "monad")); c != http.StatusForbidden {
		t.Errorf("POST %s as trusted, not allowlisted sword: %d, want 403", write, c)
	}
}

func TestAuthE2E_MonadRefusesBadConfig(t *testing.T) {
	svc := monad.NewService(logger.New(io.Discard), nil)
	if _, err := newHTTPServer(svc, logger.New(io.Discard), ":0", auth.ServiceAuthConfig{Enabled: true, ServiceName: "monad"}); err == nil {
		t.Error("auth on with nothing to verify against: no error, want refusal to start")
	}
}
