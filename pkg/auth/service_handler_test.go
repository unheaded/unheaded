// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package auth

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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

// L13: a method the policies do not name is judged as a write, so a
// read-only identity cannot use one to reach a handler that does not check
// its method.
func TestNewServiceHandler_UnknownMethodIsAWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys.jsonl")
	store, err := NewFileAPIKeyStore(path)
	if err != nil {
		t.Fatal(err)
	}
	observer, _, err := store.Create(t.Context(), "watcher", RoleObserver, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	operator, _, err := store.Create(t.Context(), "hand", RoleOperator, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	h, err := NewServiceHandler(okHandler(), ServiceAuthConfig{Enabled: true, ServiceName: "svc", APIKeyStore: "file:" + path}, ServiceRoutes{})
	if err != nil {
		t.Fatal(err)
	}
	do := func(method, target, key string) int {
		req := httptest.NewRequest(method, target, nil)
		req.Header.Set("X-API-Key", key)
		return codeOf(h, req)
	}
	for _, target := range []string{"/timeline", "/api/v1/timeline"} {
		for _, method := range []string{"PROPFIND", "MERGE", "post", "CONNECT"} {
			if got := do(method, target, observer); got != http.StatusForbidden {
				t.Errorf("observer %s %s = %d, want 403", method, target, got)
			}
			if got := do(method, target, operator); got != http.StatusOK {
				t.Errorf("operator %s %s = %d, want 200", method, target, got)
			}
		}
		if got := do(http.MethodGet, target, observer); got != http.StatusOK {
			t.Errorf("observer GET %s = %d, want 200", target, got)
		}
	}
}

// goodBearer accepts exactly "Authorization: Bearer good" as an observer.
type goodBearer struct{}

func (goodBearer) Authenticate(_ context.Context, r *http.Request) (*Identity, error) {
	if r.Header.Get("Authorization") == "Bearer good" {
		return &Identity{Subject: "tester", Roles: []string{RoleObserver}}, nil
	}
	return nil, ErrUnauthenticated
}

// S-WEST-GROUND Phase 3 (ADR-147 decision 16): routes named in SourceGated are
// open only to callers whose RemoteAddr is in AnonymousFrom; anyone else needs
// a credential. Empty AnonymousFrom changes nothing.
func TestSourceGatedOpenRoutes(t *testing.T) {
	inside := []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("172.16.0.0/12")}
	cases := []struct {
		name      string
		anonymous []netip.Prefix
		gated     []string
		openReads []string
		target    string
		remote    string
		authz     string
		xff       string
		want      int
	}{
		{"unset: /metrics stays open to the LAN", nil, []string{"/metrics"}, nil, "/metrics", "192.168.69.50:1234", "", "", http.StatusOK},
		{"a bridge address reads /metrics", inside, []string{"/metrics"}, nil, "/metrics", "172.28.1.9:40000", "", "", http.StatusOK},
		{"the LAN needs a credential for /metrics", inside, []string{"/metrics"}, nil, "/metrics", "192.168.69.50:1234", "", "", http.StatusUnauthorized},
		{"the LAN with a credential reads /metrics", inside, []string{"/metrics"}, nil, "/metrics", "192.168.69.50:1234", "Bearer good", "", http.StatusOK},
		{"a wrong credential from the LAN", inside, []string{"/metrics"}, nil, "/metrics", "192.168.69.50:1234", "Bearer bad", "", http.StatusUnauthorized},
		{"/health is not gated", inside, []string{"/metrics"}, nil, "/health", "192.168.69.50:1234", "", "", http.StatusOK},
		{"the LAN needs a credential for the task list", inside, []string{"/api"}, []string{"/"}, "/api/v1/tasks", "192.168.69.50:1234", "", "", http.StatusUnauthorized},
		{"the page loads so a person can log in", inside, []string{"/api"}, []string{"/"}, "/", "192.168.69.50:1234", "", "", http.StatusOK},
		{"loopback reads the task list", inside, []string{"/api"}, []string{"/"}, "/api/v1/tasks", "127.0.0.1:5555", "", "", http.StatusOK},
		{"a mapped IPv4 address is unwrapped", inside, []string{"/metrics"}, nil, "/metrics", "[::ffff:172.28.1.9]:40000", "", "", http.StatusOK},
		{"an address that does not parse is outside", inside, []string{"/metrics"}, nil, "/metrics", "not-an-address", "", "", http.StatusUnauthorized},
		{"X-Forwarded-For is never read", inside, []string{"/metrics"}, nil, "/metrics", "192.168.69.50:1234", "", "127.0.0.1", http.StatusUnauthorized},
		{"a path below a gated prefix is gated", inside, []string{"/api"}, []string{"/"}, "/api", "192.168.69.50:1234", "", "", http.StatusUnauthorized},
		{"a prefix match is per segment", inside, []string{"/api"}, []string{"/"}, "/apix", "192.168.69.50:1234", "", "", http.StatusOK},
		{"a dot segment never opens a gated path", inside, []string{"/metrics"}, nil, "/x/../metrics", "192.168.69.50:1234", "", "", http.StatusUnauthorized},
		{"a trailing slash is still gated", inside, []string{"/metrics"}, nil, "/metrics/", "192.168.69.50:1234", "", "", http.StatusUnauthorized},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := ServiceAuthConfig{Enabled: true, ServiceName: "svc"}
			h, err := NewServiceHandler(okHandler(), cfg, ServiceRoutes{
				OpenReads:      c.openReads,
				AnonymousFrom:  c.anonymous,
				SourceGated:    c.gated,
				Authenticators: []Authenticator{goodBearer{}},
			})
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest(http.MethodGet, c.target, nil)
			r.RemoteAddr = c.remote
			if c.authz != "" {
				r.Header.Set("Authorization", c.authz)
			}
			if c.xff != "" {
				r.Header.Set("X-Forwarded-For", c.xff)
			}
			if got := codeOf(h, r); got != c.want {
				t.Errorf("GET %s from %s: %d, want %d", c.target, c.remote, got, c.want)
			}
		})
	}
}

func TestParseAnonymousFrom(t *testing.T) {
	got, err := ParseAnonymousFrom(" 127.0.0.0/8 , fd00::/8 ")
	if err != nil || len(got) != 2 || got[0] != netip.MustParsePrefix("127.0.0.0/8") || got[1] != netip.MustParsePrefix("fd00::/8") {
		t.Errorf("two prefixes: %v, %v", got, err)
	}
	if got, err := ParseAnonymousFrom(""); err != nil || got != nil {
		t.Errorf("empty: %v, %v; want nil, nil", got, err)
	}
	for _, bad := range []string{"10.0.0.0/33", "127.0.0.1", "127.0.0.0/8,,::1/128", "not-a-prefix/8", "127.0.0.0/8,"} {
		if _, err := ParseAnonymousFrom(bad); err == nil {
			t.Errorf("%q: no error", bad)
		}
	}
}

// A bad AUTH_ANONYMOUS_FROM refuses to start, like any bad auth setting.
func TestAnonymousFromBadValueRefusesToStart(t *testing.T) {
	t.Setenv("AUTH_ENABLED", "true")
	t.Setenv("AUTH_ANONYMOUS_FROM", "192.168.0.0/99")
	cfg := LoadServiceAuthConfig("svc")
	if _, err := NewServiceChain(cfg, ServiceRoutes{Authenticators: []Authenticator{goodBearer{}}}); err == nil {
		t.Fatal("AUTH_ANONYMOUS_FROM that does not parse: no error, want the service to refuse to start")
	}
	t.Setenv("AUTH_ANONYMOUS_FROM", "127.0.0.0/8,::1/128")
	cfg = LoadServiceAuthConfig("svc")
	if len(cfg.AnonymousFrom) != 2 {
		t.Fatalf("AnonymousFrom = %v, want two prefixes", cfg.AnonymousFrom)
	}
	if _, err := NewServiceChain(cfg, ServiceRoutes{Authenticators: []Authenticator{goodBearer{}}}); err != nil {
		t.Fatalf("a good value: %v", err)
	}
}

// With auth off nothing is gated, so AUTH_ANONYMOUS_FROM does nothing; an
// operator who set it must hear that at startup (review 2026-11-15, REV-3).
// Not parallel: it swaps the package-wide logger.
func TestAnonymousFromWithAuthOffWarns(t *testing.T) {
	var buf bytes.Buffer
	saved := log.Logger
	log.Logger = zerolog.New(&buf)
	t.Cleanup(func() { log.Logger = saved })

	for _, tc := range []struct {
		name, value string
		warn        bool
	}{
		{"unset: silent", "", false},
		{"set: warns", "127.0.0.0/8", true},
		{"set and malformed: warns", "192.168.0.0/99", true},
	} {
		buf.Reset()
		t.Setenv("AUTH_ENABLED", "false")
		t.Setenv("AUTH_ANONYMOUS_FROM", tc.value)
		mw, err := NewServiceChain(LoadServiceAuthConfig("svc"), ServiceRoutes{SourceGated: []string{"/metrics"}})
		if mw != nil || err != nil {
			t.Fatalf("%s: auth off: got (%v, %v), want (nil, nil)", tc.name, mw != nil, err)
		}
		out := buf.String()
		got := strings.Contains(out, `"level":"warn"`) && strings.Contains(out, "AUTH_ANONYMOUS_FROM") && strings.Contains(out, `"service":"svc"`)
		if got != tc.warn {
			t.Errorf("%s: warning logged = %v, want %v; log: %q", tc.name, got, tc.warn, out)
		}
	}

	// routes.AnonymousFrom set by the caller with auth off: the same warning.
	buf.Reset()
	t.Setenv("AUTH_ANONYMOUS_FROM", "")
	routes := ServiceRoutes{AnonymousFrom: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}, SourceGated: []string{"/metrics"}}
	if mw, err := NewServiceChain(LoadServiceAuthConfig("svc"), routes); mw != nil || err != nil {
		t.Fatalf("routes: got (%v, %v), want (nil, nil)", mw != nil, err)
	}
	if !strings.Contains(buf.String(), "AUTH_ANONYMOUS_FROM") {
		t.Errorf("routes.AnonymousFrom with auth off: no warning; log: %q", buf.String())
	}

	// Auth on: no such warning.
	buf.Reset()
	t.Setenv("AUTH_ENABLED", "true")
	t.Setenv("AUTH_ANONYMOUS_FROM", "127.0.0.0/8")
	if _, err := NewServiceChain(LoadServiceAuthConfig("svc"), ServiceRoutes{Authenticators: []Authenticator{goodBearer{}}}); err != nil {
		t.Fatalf("auth on: %v", err)
	}
	if strings.Contains(buf.String(), "has no effect") {
		t.Errorf("auth on: warned anyway; log: %q", buf.String())
	}
}
