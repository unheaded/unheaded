// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package bff

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The issuer's lockout sees only the backend's address, so its locks fall on
// every client behind it. These budgets are the client's own, kept here where
// the client's address is known. Each login front keeps its own count and all
// the fronts on a host share one address at the issuer, so what matters is the
// sum over the fronts: kept under the issuer's limits, one client address runs
// out of attempts on every front before it can trip the issuer's lock on
// anyone else. Several client addresses together can still reach that lock.
//
// The fronts are counted, not listed: cmd/unheaded-auth's
// TestLoginFronts_BudgetsFitTheIssuerLockout reads every binary that builds a
// BFF from the source (bfftest.GoFronts), adds the Zhen web UI's own budgets
// (raft/zhen_login.py) and fails, naming the fronts and the sums, when they
// reach the issuer's limits. Four fronts on 2026-11-21 (dashboard-backend,
// kanban-app, wiki-server, the Zhen app): 2+2+2+3 = 9 and 6+6+6+5 = 23, under
// the issuer's 10 and 25 (decision pack 2026-11-18, row 28).
const (
	// LoginFailureWindow is how long a failed login counts.
	LoginFailureWindow = 10 * time.Minute
	// DefaultClientUserLoginFailures bounds failures per client address and
	// username on one front, unless EnvClientUserLoginFailures says otherwise.
	DefaultClientUserLoginFailures = 2
	// DefaultClientLoginFailures bounds failures per client address across
	// usernames on one front, unless EnvClientLoginFailures says otherwise.
	DefaultClientLoginFailures = 6
	// EnvClientUserLoginFailures and EnvClientLoginFailures set one front's
	// budgets. Whoever raises one keeps the sum over the host's fronts under
	// the issuer's limits (wiki/Service-Auth.md, "Login budgets").
	EnvClientUserLoginFailures = "AUTH_BFF_USER_LOGIN_FAILURES"
	EnvClientLoginFailures     = "AUTH_BFF_CLIENT_LOGIN_FAILURES"
	// maxLoginFailureBudget refuses a budget that is a typing mistake.
	maxLoginFailureBudget = 1000
)

// loginBudgets resolves a Config's two budgets: zero means the default.
func loginBudgets(perUser, perClient int) (int, int, error) {
	if perUser == 0 {
		perUser = DefaultClientUserLoginFailures
	}
	if perClient == 0 {
		perClient = DefaultClientLoginFailures
	}
	if perUser < 1 || perClient < perUser || perClient > maxLoginFailureBudget {
		return 0, 0, fmt.Errorf("bff: login failure budgets %d per username and %d per client: want 1 <= per username <= per client <= %d (%s, %s)",
			perUser, perClient, maxLoginFailureBudget, EnvClientUserLoginFailures, EnvClientLoginFailures)
	}
	return perUser, perClient, nil
}

// envBudget reads one budget from the environment: unset or empty is 0 (the
// default), anything that is not a positive integer is an error.
func envBudget(key string) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("bff: %s=%q: want a positive integer", key, raw)
	}
	return n, nil
}

// loginFailures charges an attempt before the issuer answers, so concurrent
// attempts cannot outrun the count, and gives the charge back when the
// answer was not a credential failure. Safe for concurrent use.
type loginFailures struct {
	mu        sync.Mutex
	now       func() time.Time
	perUser   int                    // per client address and username
	perClient int                    // per client address across usernames
	fail      map[string][]time.Time // key -> charges in the window, oldest first
}

func newLoginFailures(now func() time.Time, perUser, perClient int) *loginFailures {
	return &loginFailures{now: now, perUser: perUser, perClient: perClient, fail: make(map[string][]time.Time)}
}

func clientFailureKey(client string) string     { return "c|" + client }
func userFailureKey(client, user string) string { return "u|" + client + "|" + user }

// recent drops charges older than the window and returns the rest. Caller
// holds mu.
func (l *loginFailures) recent(key string, now time.Time) []time.Time {
	ts := l.fail[key]
	i := 0
	for i < len(ts) && now.Sub(ts[i]) >= LoginFailureWindow {
		i++
	}
	ts = ts[i:]
	if len(ts) == 0 {
		delete(l.fail, key)
		return nil
	}
	l.fail[key] = ts
	return ts
}

// charge takes one attempt from both budgets. With either spent it takes
// nothing and returns how long until an attempt is free.
func (l *loginFailures) charge(client, user string) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if len(l.fail) >= maxLimiterKeys {
		l.evictLocked(now)
		if len(l.fail) >= maxLimiterKeys {
			return time.Minute, false
		}
	}
	budgets := []struct {
		key string
		max int
	}{{userFailureKey(client, user), l.perUser}, {clientFailureKey(client), l.perClient}}
	var wait time.Duration
	for _, b := range budgets {
		if ts := l.recent(b.key, now); len(ts) >= b.max {
			if until := ts[len(ts)-b.max].Add(LoginFailureWindow).Sub(now); until > wait {
				wait = until
			}
		}
	}
	if wait > 0 {
		return wait, false
	}
	for _, b := range budgets {
		l.fail[b.key] = append(l.fail[b.key], now)
	}
	return 0, true
}

// refund gives back the charge of an attempt that was no credential failure.
func (l *loginFailures) refund(client, user string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.refundLocked(client, user)
}

func (l *loginFailures) refundLocked(client, user string) {
	for _, key := range []string{userFailureKey(client, user), clientFailureKey(client)} {
		ts := l.fail[key]
		switch len(ts) {
		case 0:
		case 1:
			delete(l.fail, key)
		default:
			l.fail[key] = ts[:len(ts)-1]
		}
	}
}

// success refunds the attempt and clears the username's budget. The client's
// stays spent: one valid account must not reset a count across usernames.
func (l *loginFailures) success(client, user string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.refundLocked(client, user)
	delete(l.fail, userFailureKey(client, user))
}

// evict drops whatever has aged out of the window.
func (l *loginFailures) evict() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.evictLocked(l.now())
}

func (l *loginFailures) evictLocked(now time.Time) {
	for k := range l.fail {
		l.recent(k, now)
	}
}
