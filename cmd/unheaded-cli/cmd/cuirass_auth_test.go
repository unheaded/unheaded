// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"unheaded/pkg/auth"
)

// S-IRON-GATE Step 166: Cuirass calls carry a service token the CLI mints
// from its own key (sub unheaded-cli, aud unheaded-daemon); UNHEADED_TOKEN
// (a uhk_ key or a token) wins when set.
func TestCallCuirass_MintsServiceToken(t *testing.T) {
	keys, trust := t.TempDir(), t.TempDir()
	if _, err := auth.GenerateKeyPair(keys, "unheaded-cli"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(keys, "unheaded-cli.pub"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(trust, "unheaded-cli.pub"), b, 0o444); err != nil {
		t.Fatal(err)
	}
	mw, err := auth.NewServiceMiddleware(auth.ServiceAuthConfig{
		Enabled: true, ServiceName: "unheaded-daemon", TrustDir: trust, AllowCallers: []string{"unheaded-cli"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var seen *auth.Identity
	srv := httptest.NewServer(mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = auth.IdentityFromContext(r.Context())
		_, _ = w.Write([]byte(`{}`))
	})))
	defer srv.Close()

	t.Setenv("UNHEADED_TOKEN", "")
	t.Setenv("AUTH_KEY_FILE", filepath.Join(keys, "unheaded-cli.key"))
	if err := callCuirass(http.MethodPut, srv.URL+"/api/v1/containers/web", map[string]string{"name": "web"}, nil); err != nil {
		t.Fatalf("callCuirass with the CLI key: %v", err)
	}
	if seen == nil || seen.Subject != "unheaded-cli" {
		t.Fatalf("server saw identity %+v, want subject unheaded-cli", seen)
	}

	t.Setenv("UNHEADED_TOKEN", "not-a-valid-token")
	if err := callCuirass(http.MethodPut, srv.URL+"/api/v1/containers/web", map[string]string{}, nil); !isHTTPStatus(err, http.StatusUnauthorized) {
		t.Errorf("UNHEADED_TOKEN should win over the key file: err %v, want HTTP 401", err)
	}
}

func TestCallCuirass_BadKeyFileFailsLoud(t *testing.T) {
	t.Setenv("UNHEADED_TOKEN", "")
	t.Setenv("AUTH_KEY_FILE", filepath.Join(t.TempDir(), "missing.key"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{}`)) }))
	defer srv.Close()
	if err := callCuirass(http.MethodGet, srv.URL+"/api/v1/containers", nil, nil); err == nil {
		t.Error("an AUTH_KEY_FILE that cannot be read must fail the call, not send it anonymously")
	}
}

func TestCallCuirass_NoKeyIsAnonymous(t *testing.T) {
	t.Setenv("UNHEADED_TOKEN", "")
	t.Setenv("AUTH_KEY_FILE", "")
	t.Setenv("HOME", t.TempDir())
	var authz string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authz = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	old := cliHostKeyFile
	cliHostKeyFile = filepath.Join(t.TempDir(), "none.key")
	defer func() { cliHostKeyFile = old }()
	if err := callCuirass(http.MethodGet, srv.URL+"/api/v1/state", nil, nil); err != nil {
		t.Fatal(err)
	}
	if authz != "" {
		t.Errorf("Authorization %q with no key anywhere, want none", authz)
	}
}
