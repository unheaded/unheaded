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
	for i := 0; i < bff.ClientUserLoginFailures; i++ {
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
	for i := 0; i < bff.ClientLoginFailures; i++ {
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
	if got := a.is.Logins.Load(); got > bff.ClientUserLoginFailures {
		t.Fatalf("issuer saw %d concurrent attempts, budget is %d", got, bff.ClientUserLoginFailures)
	}
}

// The issuer counts an attempt once it has it, whether or not the client
// stays for the answer.
func TestFailureBudget_HangingUpKeepsTheCharge(t *testing.T) {
	a := newApp(t, nil)
	for i := 0; i < bff.ClientUserLoginFailures; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(loginBody("dev", "wrong"))).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		withXRW()(req)
		fromAddr(failingClient)(req)
		a.h.ServeHTTP(httptest.NewRecorder(), req)
	}
	if got := a.failLogin("dev"); got != http.StatusTooManyRequests {
		t.Fatalf("after %d abandoned attempts = %d, want 429", bff.ClientUserLoginFailures, got)
	}
}

// Only a credential failure spends the budget.
func TestFailureBudget_OtherAnswersDoNotSpendIt(t *testing.T) {
	a := newApp(t, nil)
	pw := a.is.AddUser(t, "dev", "operator")
	for _, status := range []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadRequest} {
		a.is.ForceStatus.Store(int64(status))
		for i := 0; i < bff.ClientUserLoginFailures; i++ {
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
	for i := 0; i < bff.ClientUserLoginFailures-1; i++ {
		a.failLogin("dev")
	}
	if c := a.do(http.MethodPost, "/auth/login", loginBody("dev", pw), withXRW(), fromAddr(failingClient)).Code; c != http.StatusOK {
		t.Fatalf("good login: %d, want 200", c)
	}
	for i := 0; i < bff.ClientUserLoginFailures-1; i++ {
		if got := a.failLogin("dev"); got != http.StatusUnauthorized {
			t.Fatalf("failure %d after a good login = %d, want 401", i+1, got)
		}
	}
	for i := 0; i < bff.ClientLoginFailures-2*(bff.ClientUserLoginFailures-1); i++ {
		a.failLogin(fmt.Sprintf("name-%d", i))
	}
	if got := a.failLogin("one-more"); got != http.StatusTooManyRequests {
		t.Fatalf("client budget after a good login = %d, want 429: it must not have reset", got)
	}
}
