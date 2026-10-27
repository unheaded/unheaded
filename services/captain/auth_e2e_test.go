// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package captain

import (
	"net/http"
	"testing"

	"unheaded/pkg/auth"
	"unheaded/pkg/auth/authtest"
)

// S-IRON-GATE Phase 6 (Steps 179-182, captain): architect, micromanager and
// dashboard-backend may call; /health, /ready and /metrics stay open.
func TestAuthE2E_Captain(t *testing.T) {
	f := authtest.New(t, "captain", "architect", "micromanager", "dashboard-backend", "sword")
	svc, err := NewService(Config{Storage: NewMemoryStorage(), Wotan: newMockWotan()})
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	hs, err := newHTTPServer(svc, "localhost:0", f.Config("captain", "architect", "micromanager", "dashboard-backend"))
	if err != nil {
		t.Fatal(err)
	}
	h := hs.server.Handler
	const write, body = "/api/v1/decisions", `{"title":"auth e2e","description":"d","rationale":"r"}`

	for _, p := range []string{"/health", "/ready", "/metrics"} {
		if c := authtest.Code(h, http.MethodGet, p, "", ""); c != http.StatusOK {
			t.Errorf("GET %s anonymous: %d, want 200", p, c)
		}
	}
	if c := authtest.Code(h, http.MethodGet, "/api/v1/decisions", "", ""); c != http.StatusUnauthorized {
		t.Errorf("GET /api/v1/decisions anonymous: %d, want 401", c)
	}
	if c := authtest.Code(h, http.MethodPost, write, body, ""); c != http.StatusUnauthorized {
		t.Errorf("POST %s anonymous: %d, want 401", write, c)
	}
	for _, caller := range []string{"architect", "micromanager", "dashboard-backend"} {
		if c := authtest.Code(h, http.MethodPost, write, body, f.Bearer(caller, "captain")); !authtest.Authorized(c) {
			t.Errorf("POST %s as %s: %d, want past auth", write, caller, c)
		}
	}
	if c := authtest.Code(h, http.MethodPost, write, body, f.StrangerBearer("architect", "captain")); c != http.StatusUnauthorized {
		t.Errorf("POST %s with a key trust.d lacks: %d, want 401", write, c)
	}
	if c := authtest.Code(h, http.MethodPost, write, body, f.Bearer("sword", "captain")); c != http.StatusForbidden {
		t.Errorf("POST %s as trusted, not allowlisted sword: %d, want 403", write, c)
	}
}

func TestAuthE2E_CaptainRefusesBadConfig(t *testing.T) {
	svc, err := NewService(Config{Storage: NewMemoryStorage(), Wotan: newMockWotan()})
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	if _, err := newHTTPServer(svc, "localhost:0", auth.ServiceAuthConfig{Enabled: true, ServiceName: "captain"}); err == nil {
		t.Error("auth on with nothing to verify against: no error, want refusal to start")
	}
}
