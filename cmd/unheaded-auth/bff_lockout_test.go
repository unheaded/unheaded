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
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

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

// zhenAppUnit is the one shipped unit that logs people in at the issuer
// without pkg/bff: the Zhen web UI's app keeps its own budgets
// (raft/zhen_login.py), sized with the two backends' on 2026-11-09.
const zhenAppUnit = "unheaded-zhen-app.service"

// zhenBudget reads one of the Zhen app's budget constants from its source, as
// raft/tests/test_zhen_login.py reads this side's.
func zhenBudget(t *testing.T, name string) int {
	t.Helper()
	raw, err := os.ReadFile("../../raft/zhen_login.py")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^` + name + `\s*=\s*(\d+)\s*$`).FindSubmatch(raw)
	if m == nil {
		t.Fatalf("%s not found in raft/zhen_login.py", name)
	}
	n, err := strconv.Atoi(string(m[1]))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// Every shipped unit that names an issuer spends failures from the host's one
// address at the issuer. Each is accounted for here: a pkg/bff backend counts
// against IssuerAddressBackends, the Zhen app brings its own budgets, and all
// of them added together stay under the issuer's locks. A further unit must
// resize the budgets, not quietly void the sum.
func TestBFF_ShippedUnitsFitTheBackendCount(t *testing.T) {
	units, err := filepath.Glob("../../deploy/systemd/*.service")
	if err != nil || len(units) == 0 {
		t.Fatalf("no units found: %v", err)
	}
	var backends []string
	zhen := false
	for _, u := range units {
		raw, err := os.ReadFile(u) // #nosec G304 -- a path from the Glob above
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), "Environment=AUTH_ISSUER_URL=") {
			continue
		}
		if filepath.Base(u) == zhenAppUnit {
			zhen = true
			continue
		}
		backends = append(backends, filepath.Base(u))
	}
	if len(backends) == 0 || len(backends) > bff.IssuerAddressBackends {
		t.Fatalf("pkg/bff units with an issuer URL %v, budgets sized for %d", backends, bff.IssuerAddressBackends)
	}
	if !zhen {
		t.Fatalf("%s no longer names an issuer; take its budgets out of this sum", zhenAppUnit)
	}
	perUser := bff.IssuerAddressBackends*bff.ClientUserLoginFailures + zhenBudget(t, "CLIENT_USER_FAILURES")
	if perUser >= maxFailuresPerPair {
		t.Errorf("per-user budgets of %v and %s total %d, must be under the issuer's %d", backends, zhenAppUnit, perUser, maxFailuresPerPair)
	}
	perClient := bff.IssuerAddressBackends*bff.ClientLoginFailures + zhenBudget(t, "CLIENT_FAILURES")
	if perClient >= maxFailuresPerSource {
		t.Errorf("per-client budgets of %v and %s total %d, must be under the issuer's %d", backends, zhenAppUnit, perClient, maxFailuresPerSource)
	}
	if window := time.Duration(zhenBudget(t, "FAILURE_WINDOW")) * time.Second; window < lockoutWindow {
		t.Errorf("%s counts a failure for %v, must cover the issuer's %v", zhenAppUnit, window, lockoutWindow)
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
