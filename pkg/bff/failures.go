// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package bff

import (
	"sync"
	"time"
)

// The issuer's lockout sees only the backend's address, so its locks fall on
// every client behind it. These budgets are the client's own, kept here where
// the client's address is known. Each backend keeps its own count and
// IssuerAddressBackends of them share an address at the issuer, so the sum
// stays below the issuer's limits (TestBFF_BudgetsStayBelowTheIssuerLockout):
// one client address runs out of attempts on every backend before it can trip
// the issuer's lock on anyone else. Several client addresses together can
// still reach that lock.
const (
	// LoginFailureWindow is how long a failed login counts.
	LoginFailureWindow = 10 * time.Minute
	// IssuerAddressBackends is how many backends reach the issuer from one
	// source address: the dashboard and the board on one host. The
	// budgets below are per backend, so this many of them add up at the
	// issuer.
	IssuerAddressBackends = 2
	// ClientUserLoginFailures bounds failures per client and username.
	ClientUserLoginFailures = 2
	// ClientLoginFailures bounds failures per client across usernames.
	ClientLoginFailures = 9
)

// loginFailures charges an attempt before the issuer answers, so concurrent
// attempts cannot outrun the count, and gives the charge back when the
// answer was not a credential failure. Safe for concurrent use.
type loginFailures struct {
	mu   sync.Mutex
	now  func() time.Time
	fail map[string][]time.Time // key -> charges in the window, oldest first
}

func newLoginFailures(now func() time.Time) *loginFailures {
	return &loginFailures{now: now, fail: make(map[string][]time.Time)}
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
	}{{userFailureKey(client, user), ClientUserLoginFailures}, {clientFailureKey(client), ClientLoginFailures}}
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
