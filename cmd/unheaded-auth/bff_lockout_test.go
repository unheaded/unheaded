// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"unheaded/pkg/bff"
)

// bffIssuer is the real issuer behind a real BFF: to the issuer, every
// browser is the one peer address of the backend in front of it.
func bffIssuer(t *testing.T) http.Handler {
	t.Helper()
	ts, _ := serve(t, newFixture(t), func(c *issuerConfig) { c.Audiences = []string{"dashboard-backend"} })
	b, err := bff.New(bff.Config{ServiceName: "dashboard-backend", IssuerURL: ts.URL, DevPlaintext: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Close)
	return b.Handler()
}

func bffLogin(t *testing.T, h http.Handler, addr, user, password string) int {
	t.Helper()
	body, err := json.Marshal(map[string]string{"username": user, "password": password})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(bff.HeaderRequestedWith, bff.RequestedWithValue)
	req.RemoteAddr = addr
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

// The BFF's budgets only protect anyone while they stay under these limits.
func TestBFF_BudgetsStayBelowTheIssuerLockout(t *testing.T) {
	if bff.ClientUserLoginFailures >= maxFailuresPerPair {
		t.Errorf("per-user budget %d must be under the issuer's %d", bff.ClientUserLoginFailures, maxFailuresPerPair)
	}
	if bff.ClientLoginFailures >= maxFailuresPerSource {
		t.Errorf("per-client budget %d must be under the issuer's %d", bff.ClientLoginFailures, maxFailuresPerSource)
	}
	if bff.LoginFailureWindow < lockoutWindow {
		t.Errorf("window %v must cover the issuer's %v", bff.LoginFailureWindow, lockoutWindow)
	}
}

// M7: however often one client fails, it cannot reach the issuer's lock on
// the backend's address, so nobody else loses their login.
func TestBFF_OneClientCannotLockOutAUser(t *testing.T) {
	h := bffIssuer(t)
	for i := 0; i < 2*maxFailuresPerPair; i++ {
		bffLogin(t, h, "192.0.2.7:4000", "dev", "wrong")
	}
	if got := bffLogin(t, h, "192.0.2.8:4000", "dev", "correct horse battery"); got != http.StatusOK {
		t.Fatalf("another client's login = %d, want 200", got)
	}
}

func TestBFF_OneClientCannotLockOutEveryone(t *testing.T) {
	h := bffIssuer(t)
	for i := 0; i < 2*maxFailuresPerSource; i++ {
		bffLogin(t, h, "192.0.2.7:4000", fmt.Sprintf("nobody-%d", i), "wrong")
	}
	if got := bffLogin(t, h, "192.0.2.8:4000", "dev", "correct horse battery"); got != http.StatusOK {
		t.Fatalf("another client's login = %d, want 200", got)
	}
}
