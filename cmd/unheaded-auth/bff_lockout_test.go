// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

// bffPairIssuer is the real issuer behind two real BFFs on one address, as
// the dashboard and the board are on a host.
func bffPairIssuer(t *testing.T) (http.Handler, http.Handler) {
	t.Helper()
	ts, _ := serve(t, newFixture(t), func(c *issuerConfig) { c.Audiences = []string{"dashboard-backend", "kanban"} })
	var hs [2]http.Handler
	for i, name := range []string{"dashboard-backend", "kanban"} {
		b, err := bff.New(bff.Config{ServiceName: name, IssuerURL: ts.URL, DevPlaintext: true})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(b.Close)
		hs[i] = b.Handler()
	}
	return hs[0], hs[1]
}

// The BFF's budgets only protect anyone while every backend sharing an
// address at the issuer, spent together, stays under these limits.
func TestBFF_BudgetsStayBelowTheIssuerLockout(t *testing.T) {
	if got := bff.IssuerAddressBackends * bff.ClientUserLoginFailures; got >= maxFailuresPerPair {
		t.Errorf("per-user budgets total %d, must be under the issuer's %d", got, maxFailuresPerPair)
	}
	if got := bff.IssuerAddressBackends * bff.ClientLoginFailures; got >= maxFailuresPerSource {
		t.Errorf("per-client budgets total %d, must be under the issuer's %d", got, maxFailuresPerSource)
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

// M7: both backends on a host reach the issuer from one address, and each
// keeps its own budget, so one client spending both must still fall short.
func TestBFF_OneClientOnBothBackendsCannotLockOutAUser(t *testing.T) {
	dash, board := bffPairIssuer(t)
	for i := 0; i < 2*maxFailuresPerPair; i++ {
		bffLogin(t, dash, "192.0.2.7:4000", "dev", "wrong")
		bffLogin(t, board, "192.0.2.7:4000", "dev", "wrong")
	}
	if got := bffLogin(t, dash, "192.0.2.8:4000", "dev", "correct horse battery"); got != http.StatusOK {
		t.Fatalf("another client's login = %d, want 200", got)
	}
}

func TestBFF_OneClientOnBothBackendsCannotLockOutEveryone(t *testing.T) {
	dash, board := bffPairIssuer(t)
	for i := 0; i < 2*maxFailuresPerSource; i++ {
		bffLogin(t, dash, "192.0.2.7:4000", fmt.Sprintf("nobody-%d", i), "wrong")
		bffLogin(t, board, "192.0.2.7:4000", fmt.Sprintf("nobody-%d", i), "wrong")
	}
	if got := bffLogin(t, dash, "192.0.2.8:4000", "dev", "correct horse battery"); got != http.StatusOK {
		t.Fatalf("another client's login = %d, want 200", got)
	}
}

// The budgets are sized for IssuerAddressBackends. A further unit logging in
// through the issuer from the same host must resize them, not quietly void
// the sum above.
func TestBFF_ShippedUnitsFitTheBackendCount(t *testing.T) {
	units, err := filepath.Glob("../../deploy/systemd/*.service")
	if err != nil || len(units) == 0 {
		t.Fatalf("no units found: %v", err)
	}
	var backends []string
	for _, u := range units {
		raw, err := os.ReadFile(u) // #nosec G304 -- a path from the Glob above
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "Environment=AUTH_ISSUER_URL=") {
			backends = append(backends, filepath.Base(u))
		}
	}
	if len(backends) == 0 || len(backends) > bff.IssuerAddressBackends {
		t.Fatalf("units with an issuer URL %v, budgets sized for %d", backends, bff.IssuerAddressBackends)
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
