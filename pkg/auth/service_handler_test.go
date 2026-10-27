// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package auth

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// serviceHandlerFixture: keys for "svc" (the service), "caller" (trusted and
// allowlisted) and "other" (trusted, not allowlisted).
func serviceHandlerFixture(t *testing.T) (cfg ServiceAuthConfig, bearer func(name string) string) {
	t.Helper()
	keys, trust := t.TempDir(), t.TempDir()
	for _, n := range []string{"svc", "caller", "other"} {
		if _, err := GenerateKeyPair(keys, n); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(keys, n+".pub"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(trust, n+".pub"), b, 0o444); err != nil {
			t.Fatal(err)
		}
	}
	cfg = ServiceAuthConfig{Enabled: true, ServiceName: "svc", TrustDir: trust,
		KeyFile: filepath.Join(keys, "svc.key"), AllowCallers: []string{"caller"}}
	bearer = func(name string) string {
		m, err := LoadServiceTokenMinter(name, filepath.Join(keys, name+".key"))
		if err != nil {
			t.Fatal(err)
		}
		h, err := m.AuthorizationHeader("svc")
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	return cfg, bearer
}

func doReq(h http.Handler, method, target, authz string) int {
	r := httptest.NewRequest(method, target, nil)
	if authz != "" {
		r.Header.Set("Authorization", authz)
	}
	return codeOf(h, r)
}

func TestNewServiceHandler_Off(t *testing.T) {
	h, err := NewServiceHandler(okHandler(), ServiceAuthConfig{ServiceName: "svc"}, ServiceRoutes{})
	if err != nil {
		t.Fatal(err)
	}
	if c := doReq(h, http.MethodPost, "/api/v1/x", ""); c != http.StatusOK {
		t.Errorf("auth off: %d, want 200", c)
	}
}

func TestNewServiceHandler_RefusesBadConfig(t *testing.T) {
	_, err := NewServiceHandler(okHandler(), ServiceAuthConfig{Enabled: true, ServiceName: "svc"}, ServiceRoutes{})
	if err == nil {
		t.Fatal("auth on with no credentials: no error, want one so the service refuses to start")
	}
}

func TestNewServiceHandler_WritesAndReads(t *testing.T) {
	cfg, bearer := serviceHandlerFixture(t)
	h, err := NewServiceHandler(okHandler(), cfg, ServiceRoutes{})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/health", "/ready", "/metrics"} {
		if c := doReq(h, http.MethodGet, p, ""); c != http.StatusOK {
			t.Errorf("GET %s anonymous: %d, want 200", p, c)
		}
	}
	cases := []struct {
		method, path, authz string
		want                int
	}{
		{http.MethodPost, "/api/v1/x", "", http.StatusUnauthorized},
		{http.MethodGet, "/api/v1/x", "", http.StatusUnauthorized},
		{http.MethodPost, "/api/v1/x", bearer("caller"), http.StatusOK},
		{http.MethodGet, "/api/v1/x", bearer("caller"), http.StatusOK},
		{http.MethodPost, "/api/v1/x", bearer("other"), http.StatusForbidden},
		// A write outside /api/v1/ still needs authentication and a write role.
		{http.MethodPost, "/timeline", bearer("caller"), http.StatusOK},
		{http.MethodDelete, "/timeline", "", http.StatusUnauthorized},
	}
	for _, c := range cases {
		if got := doReq(h, c.method, c.path, c.authz); got != c.want {
			t.Errorf("%s %s (authz=%t): %d, want %d", c.method, c.path, c.authz != "", got, c.want)
		}
	}
}

// An observer (read-only) identity may read but not write, also outside
// /api/v1/, where DefaultServicePolicies has no write rule of its own.
func TestNewServiceHandler_ObserverCannotWriteAnywhere(t *testing.T) {
	path := filepath.Join(t.TempDir(), "apikeys.jsonl")
	store, err := NewFileAPIKeyStore(path)
	if err != nil {
		t.Fatal(err)
	}
	key, _, err := store.Create(t.Context(), "watcher", RoleObserver, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	h, err := NewServiceHandler(okHandler(), ServiceAuthConfig{Enabled: true, ServiceName: "svc", APIKeyStore: "file:" + path}, ServiceRoutes{})
	if err != nil {
		t.Fatal(err)
	}
	try := func(method, p string) int {
		r := httptest.NewRequest(method, p, nil)
		r.Header.Set("X-API-Key", key)
		return codeOf(h, r)
	}
	if c := try(http.MethodGet, "/timeline"); c != http.StatusOK {
		t.Errorf("observer GET /timeline: %d, want 200", c)
	}
	if c := try(http.MethodPost, "/timeline"); c != http.StatusForbidden {
		t.Errorf("observer POST /timeline: %d, want 403", c)
	}
	if c := try(http.MethodPost, "/api/v1/x"); c != http.StatusForbidden {
		t.Errorf("observer POST /api/v1/x: %d, want 403", c)
	}
}

func TestNewServiceHandler_OpenReadsAndPaths(t *testing.T) {
	cfg, _ := serviceHandlerFixture(t)
	h, err := NewServiceHandler(okHandler(), cfg, ServiceRoutes{
		OpenReads: []string{"/api/v1/tasks"},
		OpenPaths: []string{"/events"},
	})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/api/v1/tasks", http.StatusOK},
		{http.MethodHead, "/api/v1/tasks/1", http.StatusOK},
		{http.MethodPost, "/api/v1/tasks", http.StatusUnauthorized},
		{http.MethodPut, "/api/v1/tasks/1", http.StatusUnauthorized},
		{http.MethodGet, "/api/v1/tasksx", http.StatusUnauthorized},
		{http.MethodGet, "/api/v1/tasks/../admin", http.StatusUnauthorized},
		{http.MethodPost, "/events", http.StatusOK},
		{http.MethodGet, "/api/v1/other", http.StatusUnauthorized},
	}
	for _, c := range cases {
		if got := doReq(h, c.method, c.path, ""); got != c.want {
			t.Errorf("%s %s anonymous: %d, want %d", c.method, c.path, got, c.want)
		}
	}
}

func TestNewServiceHandler_AllReadsOpen(t *testing.T) {
	cfg, _ := serviceHandlerFixture(t)
	h, err := NewServiceHandler(okHandler(), cfg, ServiceRoutes{OpenReads: []string{"/"}})
	if err != nil {
		t.Fatal(err)
	}
	if c := doReq(h, http.MethodGet, "/wiki/Home", ""); c != http.StatusOK {
		t.Errorf("GET anonymous with every read open: %d, want 200", c)
	}
	if c := doReq(h, http.MethodPost, "/wiki/Home", ""); c != http.StatusUnauthorized {
		t.Errorf("POST anonymous with every read open: %d, want 401", c)
	}
}

// RBAC replaces the defaults: here a service token may use /api/v1/deploy,
// which DefaultServicePolicies keeps for admin and operator.
func TestNewServiceHandler_CustomPolicies(t *testing.T) {
	cfg, bearer := serviceHandlerFixture(t)
	def, err := NewServiceHandler(okHandler(), cfg, ServiceRoutes{})
	if err != nil {
		t.Fatal(err)
	}
	if c := doReq(def, http.MethodPost, "/api/v1/deploy", bearer("caller")); c != http.StatusForbidden {
		t.Fatalf("default policies, service token on /api/v1/deploy: %d, want 403", c)
	}
	rbac := DefaultServicePolicies("svc")
	rbac.Policies = append([]EndpointPolicy{{PathPrefix: "/api/v1/deploy", RequiredRoles: []string{RoleAdmin, RoleOperator, RoleService}}}, rbac.Policies...)
	h, err := NewServiceHandler(okHandler(), cfg, ServiceRoutes{RBAC: &rbac})
	if err != nil {
		t.Fatal(err)
	}
	if c := doReq(h, http.MethodPost, "/api/v1/deploy", bearer("caller")); c != http.StatusOK {
		t.Errorf("custom policies, service token on /api/v1/deploy: %d, want 200", c)
	}
}
