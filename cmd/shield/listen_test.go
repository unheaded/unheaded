// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"unheaded/pkg/auth"
	"unheaded/pkg/auth/authtest"
)

// L6: with no flags shield binds loopback.
func TestListenAddr_DefaultsToLoopback(t *testing.T) {
	if got := listenAddr("", 19009); got != "127.0.0.1:19009" {
		t.Fatalf("default = %q", got)
	}
	if got := listenAddr("[fd00::1]:19009", 19009); got != "[fd00::1]:19009" {
		t.Fatalf("explicit = %q", got)
	}
}

// L6: off loopback needs auth on; the routes then need a credential.
func TestNewHandler_Auth(t *testing.T) {
	mux, _ := newTestMux(t)
	if _, err := newHandler(mux, ":19009", auth.ServiceAuthConfig{}); !errors.Is(err, auth.ErrOpenListener) {
		t.Fatalf("every-interface bind with auth off: %v, want ErrOpenListener", err)
	}
	if h, err := newHandler(mux, "127.0.0.1:19009", auth.ServiceAuthConfig{}); err != nil || h == nil {
		t.Fatalf("loopback with auth off: %v", err)
	}

	f := authtest.New(t, "shield", "unheaded-cli")
	h, err := newHandler(mux, ":19009", f.Config("shield", "unheaded-cli"))
	if err != nil {
		t.Fatal(err)
	}
	rule := `{"id":"r1","name":"n"}`
	if c := authtest.Code(h, http.MethodGet, "/api/v1/rules", "", ""); c != http.StatusUnauthorized {
		t.Errorf("GET rules anonymous: %d, want 401", c)
	}
	if c := authtest.Code(h, http.MethodPost, "/api/v1/rules", rule, ""); c != http.StatusUnauthorized {
		t.Errorf("POST rules anonymous: %d, want 401", c)
	}
	if c := authtest.Code(h, http.MethodDelete, "/api/v1/rules/r1", "", ""); c != http.StatusUnauthorized {
		t.Errorf("DELETE rule anonymous: %d, want 401", c)
	}
	if c := authtest.Code(h, http.MethodGet, "/api/v1/rules", "", f.Bearer("unheaded-cli", "shield")); !authtest.Authorized(c) {
		t.Errorf("GET rules with a token: %d, want past auth", c)
	}

	bad := f.Config("shield", "unheaded-cli")
	bad.TrustDir = ""
	if _, err := newHandler(mux, ":19009", bad); err == nil {
		t.Error("auth on with nothing to verify against: no error, want refusal to start")
	}
}

// L6: rule and evaluate bodies are bounded.
func TestControlAPI_BodyLimit(t *testing.T) {
	mux, _ := newTestMux(t)
	big := `{"name":"` + strings.Repeat("a", maxBodyBytes+1) + `"}`
	for _, p := range []string{"/api/v1/rules", "/api/v1/evaluate"} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, p, strings.NewReader(big)))
		if w.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("POST %s over the limit: %d, want 413", p, w.Code)
		}
	}
}
