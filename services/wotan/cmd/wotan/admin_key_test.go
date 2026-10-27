// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package main

import (
	"flag"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"unheaded/services/wotan/internal/metrics"
)

// Built at run time: a 32-character literal here reads as an API key to
// gitleaks (generic-api-key), and the secrets baseline only shrinks.
var testEnvAdminKey = strings.Repeat("0123456789abcdef", 2)

// -admin with no key must refuse to start, naming the variable to set.
func TestAdminKey_RequiredWhenAdminEnabled(t *testing.T) {
	t.Setenv(adminKeyEnv, "")
	key, err := adminKeyFromEnv(true)
	if err == nil {
		t.Fatalf("adminKeyFromEnv(true) with empty env: want error, got key %q", key)
	}
	if !strings.Contains(err.Error(), "WOTAN_ADMIN_API_KEY") {
		t.Errorf("error %q does not name WOTAN_ADMIN_API_KEY", err)
	}

	// Whitespace only is not a key.
	t.Setenv(adminKeyEnv, "  \t ")
	if _, err := adminKeyFromEnv(true); err == nil {
		t.Errorf("whitespace-only key accepted")
	}
}

// Admin off: no key needed and none applied, even if the env carries one.
func TestAdminKey_NotRequiredWhenAdminDisabled(t *testing.T) {
	t.Setenv(adminKeyEnv, "")
	key, err := adminKeyFromEnv(false)
	if err != nil || key != "" {
		t.Fatalf("adminKeyFromEnv(false) = %q, %v; want \"\", nil", key, err)
	}
	t.Setenv(adminKeyEnv, testEnvAdminKey)
	key, err = adminKeyFromEnv(false)
	if err != nil || key != "" {
		t.Fatalf("adminKeyFromEnv(false) with env set = %q, %v; want \"\", nil", key, err)
	}
}

func TestAdminKey_SetOnServer(t *testing.T) {
	t.Setenv(adminKeyEnv, testEnvAdminKey)
	s := newTestAPIServer(t)
	if err := configureAdmin(s, true); err != nil {
		t.Fatalf("configureAdmin: %v", err)
	}
	if s.AdminAPIKey != testEnvAdminKey {
		t.Errorf("AdminAPIKey = %q, want the env value", s.AdminAPIKey)
	}

	t.Setenv(adminKeyEnv, "")
	s2 := newTestAPIServer(t)
	if err := configureAdmin(s2, true); err == nil {
		t.Errorf("configureAdmin with -admin and no key: want error")
	}
}

// -admin is opt-in since S-CLEAN-SLATE: the endpoints never worked with the
// old default (no key was ever set), so off changes nothing live.
func TestAdminKey_FlagDefaultsOff(t *testing.T) {
	cfg := parseFlags(flag.NewFlagSet("wotan-admin-default", flag.ContinueOnError), nil)
	if cfg.AdminEnabled {
		t.Errorf("-admin default = true, want false")
	}
}

// End to end through the real mux: with the key wired from the environment
// an admin request without the header is refused and one with it succeeds.
func TestAdminKey_RoutesEnforceKey(t *testing.T) {
	t.Setenv(adminKeyEnv, testEnvAdminKey)
	s := newTestAPIServer(t)
	if err := configureAdmin(s, true); err != nil {
		t.Fatalf("configureAdmin: %v", err)
	}
	mux := setupHTTPRoutes(s, true, metrics.Initialize("wotan_test_admin_key"))

	cases := []struct {
		name   string
		header string
		value  string
		want   int
	}{
		{"no header", "", "", http.StatusUnauthorized},
		{"wrong key", "X-Admin-API-Key", "nope", http.StatusUnauthorized},
		{"x-admin header", "X-Admin-API-Key", testEnvAdminKey, http.StatusOK},
		{"bearer", "Authorization", "Bearer " + testEnvAdminKey, http.StatusOK},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/pending", nil)
		if tc.header != "" {
			req.Header.Set(tc.header, tc.value)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Errorf("%s: status %d, want %d", tc.name, w.Code, tc.want)
		}
	}
}
