// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package bff_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/bff"
)

const failingClient = "192.0.2.7:4000"

func (a *app) failLogin(user string) int {
	return a.do(http.MethodPost, "/auth/login", loginBody(user, "wrong"), withXRW(), fromAddr(failingClient)).Code
}

func TestFailureBudget_PerUsernameStopsAtTheBFF(t *testing.T) {
	a := newApp(t, nil)
	pw := a.is.AddUser(t, "dev", "operator")
	for i := 0; i < bff.DefaultClientUserLoginFailures; i++ {
		if got := a.failLogin("dev"); got != http.StatusUnauthorized {
			t.Fatalf("failure %d = %d, want 401", i+1, got)
		}
	}
	seen := a.is.Logins.Load()
	rec := a.do(http.MethodPost, "/auth/login", loginBody("dev", pw), withXRW(), fromAddr(failingClient))
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("spent budget = %d, Retry-After %q; want 429 with a wait", rec.Code, rec.Header().Get("Retry-After"))
	}
	if got := a.is.Logins.Load(); got != seen {
		t.Fatalf("issuer saw %d more logins past the budget", got-seen)
	}
	// Only this client and name are spent.
	if c := a.do(http.MethodPost, "/auth/login", loginBody("dev", pw), withXRW(), fromAddr("192.0.2.8:4000")).Code; c != http.StatusOK {
		t.Fatalf("other client: %d, want 200", c)
	}
	if got := a.failLogin("somebody-else"); got != http.StatusUnauthorized {
		t.Fatalf("same client, other name = %d, want 401", got)
	}
	a.clk.Advance(bff.LoginFailureWindow)
	if c := a.do(http.MethodPost, "/auth/login", loginBody("dev", pw), withXRW(), fromAddr(failingClient)).Code; c != http.StatusOK {
		t.Fatalf("after the window: %d, want 200", c)
	}
}

func TestFailureBudget_AcrossUsernames(t *testing.T) {
	a := newApp(t, nil)
	for i := 0; i < bff.DefaultClientLoginFailures; i++ {
		if got := a.failLogin(fmt.Sprintf("name-%d", i)); got != http.StatusUnauthorized {
			t.Fatalf("failure %d = %d, want 401", i+1, got)
		}
	}
	if got := a.failLogin("one-more"); got != http.StatusTooManyRequests {
		t.Fatalf("past the client budget = %d, want 429", got)
	}
}

// Attempts in flight count, or a burst outruns the issuer's answers.
func TestFailureBudget_ConcurrentAttemptsCannotOutrunIt(t *testing.T) {
	a := newApp(t, nil)
	var wg sync.WaitGroup
	for i := 0; i < bff.LoginAttemptsPerMinute; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a.failLogin("dev")
		}()
	}
	wg.Wait()
	if got := a.is.Logins.Load(); got > bff.DefaultClientUserLoginFailures {
		t.Fatalf("issuer saw %d concurrent attempts, budget is %d", got, bff.DefaultClientUserLoginFailures)
	}
}

// The issuer counts an attempt once it has it, whether or not the client
// stays for the answer.
func TestFailureBudget_HangingUpKeepsTheCharge(t *testing.T) {
	a := newApp(t, nil)
	for i := 0; i < bff.DefaultClientUserLoginFailures; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(loginBody("dev", "wrong"))).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		withXRW()(req)
		fromAddr(failingClient)(req)
		a.h.ServeHTTP(httptest.NewRecorder(), req)
	}
	if got := a.failLogin("dev"); got != http.StatusTooManyRequests {
		t.Fatalf("after %d abandoned attempts = %d, want 429", bff.DefaultClientUserLoginFailures, got)
	}
}

// Only a credential failure spends the budget.
func TestFailureBudget_OtherAnswersDoNotSpendIt(t *testing.T) {
	a := newApp(t, nil)
	pw := a.is.AddUser(t, "dev", "operator")
	for _, status := range []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadRequest} {
		a.is.ForceStatus.Store(int64(status))
		for i := 0; i < bff.DefaultClientUserLoginFailures; i++ {
			a.failLogin("dev")
		}
		a.clk.Advance(time.Minute) // past the per-minute rate limit
	}
	a.is.ForceStatus.Store(0)
	if c := a.do(http.MethodPost, "/auth/login", loginBody("dev", pw), withXRW(), fromAddr(failingClient)).Code; c != http.StatusOK {
		t.Fatalf("after refused answers: %d, want 200", c)
	}
}

// A good login clears the username's count, not the client's.
func TestFailureBudget_SuccessClearsOnlyTheUsername(t *testing.T) {
	a := newApp(t, nil)
	pw := a.is.AddUser(t, "dev", "operator")
	for i := 0; i < bff.DefaultClientUserLoginFailures-1; i++ {
		a.failLogin("dev")
	}
	if c := a.do(http.MethodPost, "/auth/login", loginBody("dev", pw), withXRW(), fromAddr(failingClient)).Code; c != http.StatusOK {
		t.Fatalf("good login: %d, want 200", c)
	}
	for i := 0; i < bff.DefaultClientUserLoginFailures-1; i++ {
		if got := a.failLogin("dev"); got != http.StatusUnauthorized {
			t.Fatalf("failure %d after a good login = %d, want 401", i+1, got)
		}
	}
	for i := 0; i < bff.DefaultClientLoginFailures-2*(bff.DefaultClientUserLoginFailures-1); i++ {
		a.failLogin(fmt.Sprintf("name-%d", i))
	}
	if got := a.failLogin("one-more"); got != http.StatusTooManyRequests {
		t.Fatalf("client budget after a good login = %d, want 429: it must not have reset", got)
	}
}

// The budgets are configuration keys (decision pack 2026-11-18, row 28).
func TestFailureBudget_ReadFromTheEnvironment(t *testing.T) {
	t.Setenv("AUTH_ISSUER_URL", "https://auth.example")
	c := bff.ConfigFromEnv(svc)
	if c.ClientUserLoginFailures != 0 || c.ClientLoginFailures != 0 {
		t.Fatalf("unset keys must leave the defaults to New: %d, %d", c.ClientUserLoginFailures, c.ClientLoginFailures)
	}
	t.Setenv(bff.EnvClientUserLoginFailures, "1")
	t.Setenv(bff.EnvClientLoginFailures, " 4 ")
	c = bff.ConfigFromEnv(svc)
	if c.ClientUserLoginFailures != 1 || c.ClientLoginFailures != 4 {
		t.Fatalf("budgets from the environment: %d, %d, want 1, 4", c.ClientUserLoginFailures, c.ClientLoginFailures)
	}
}

func TestFailureBudget_DefaultsAndConfiguredValuesAreInForce(t *testing.T) {
	a := newApp(t, nil)
	if u, c := a.b.LoginFailureBudgets(); u != bff.DefaultClientUserLoginFailures || c != bff.DefaultClientLoginFailures {
		t.Fatalf("default budgets in force: %d, %d", u, c)
	}
	a = newApp(t, func(c *bff.Config) { c.ClientUserLoginFailures, c.ClientLoginFailures = 1, 3 })
	if got := a.failLogin("dev"); got != http.StatusUnauthorized {
		t.Fatalf("first failure = %d, want 401", got)
	}
	if got := a.failLogin("dev"); got != http.StatusTooManyRequests {
		t.Fatalf("second failure with a budget of 1 = %d, want 429", got)
	}
	a.failLogin("name-1")
	a.failLogin("name-2")
	if got := a.failLogin("name-3"); got != http.StatusTooManyRequests {
		t.Fatalf("fourth name with a client budget of 3 = %d, want 429", got)
	}
}

// A budget that cannot be read is never replaced by a default: the front
// refuses to start.
func TestFailureBudget_BadValuesRefuseToStart(t *testing.T) {
	is := "https://auth.example"
	for _, tc := range []struct{ user, client string }{
		{"many", ""}, {"0", ""}, {"-1", ""}, {"", "0"}, {"", "1.5"}, {"7", "6"}, {"", "100000"},
	} {
		t.Setenv("AUTH_ISSUER_URL", is)
		t.Setenv(bff.EnvClientUserLoginFailures, tc.user)
		t.Setenv(bff.EnvClientLoginFailures, tc.client)
		b, err := bff.New(bff.ConfigFromEnv(svc))
		if err == nil {
			b.Close()
			t.Errorf("%s=%q %s=%q accepted", bff.EnvClientUserLoginFailures, tc.user, bff.EnvClientLoginFailures, tc.client)
			continue
		}
		if !strings.Contains(err.Error(), "LOGIN_FAILURES") {
			t.Errorf("%q, %q: error %q does not name the key", tc.user, tc.client, err)
		}
	}
	if _, err := bff.New(bff.Config{ServiceName: svc, IssuerURL: is, ClientUserLoginFailures: -2}); err == nil {
		t.Error("a negative budget in Config accepted")
	}
}
