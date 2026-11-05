// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"sync"
	"time"
)

// Login throttling. A (username, source address) pair that failed
// maxFailuresPerPair times within lockoutWindow gets 429 until the oldest
// of those failures leaves the window; so does a source address with
// maxFailuresPerSource failures across any usernames (spraying). There is
// no per-username lock across addresses: anyone could then lock any user
// out. Locked attempts never reach argon2id, so they cost nothing.
//
// Behind a backend-for-frontend the source is that backend, so these locks
// fall on all its clients together. pkg/bff holds each client to smaller
// budgets, which keeps any one client from reaching them. The issuer takes
// no forwarded address.
const (
	lockoutWindow        = 10 * time.Minute
	maxFailuresPerPair   = 5
	maxFailuresPerSource = 20
	// maxTrackedKeys bounds memory under a flood of distinct addresses.
	maxTrackedKeys = 10000
)

type lockout struct {
	mu   sync.Mutex
	now  func() time.Time
	fail map[string][]time.Time // key -> failure times within the window, oldest first
}

func newLockout(now func() time.Time) *lockout {
	return &lockout{now: now, fail: make(map[string][]time.Time)}
}

func pairKey(user, src string) string { return "p\x00" + user + "\x00" + src }
func sourceKey(src string) string     { return "s\x00" + src }

// recent drops failures older than the window and returns what is left.
// Caller holds mu.
func (l *lockout) recent(key string, now time.Time) []time.Time {
	ts := l.fail[key]
	i := 0
	for i < len(ts) && now.Sub(ts[i]) >= lockoutWindow {
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

// begin admits an attempt or reports the lockout and how long it lasts. An
// admitted attempt is charged as a failure now, before the slow hash, so
// attempts in flight count and a burst gets no more guesses than a sequence
// would. success or abandon takes the charge back.
func (l *lockout) begin(user, src string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	limits := []struct {
		key string
		max int
	}{{pairKey(user, src), maxFailuresPerPair}, {sourceKey(src), maxFailuresPerSource}}
	var wait time.Duration
	for _, k := range limits {
		ts := l.recent(k.key, now)
		if len(ts) >= k.max {
			// Unlocked once enough of the oldest failures have aged out.
			until := ts[len(ts)-k.max].Add(lockoutWindow).Sub(now)
			if until > wait {
				wait = until
			}
		}
	}
	if wait > 0 {
		return true, wait
	}
	if len(l.fail) >= maxTrackedKeys {
		l.evict(now)
	}
	for _, k := range limits {
		l.fail[k.key] = append(l.recent(k.key, now), now)
	}
	return false, 0
}

// dropNewest takes back one charge. Caller holds mu.
func (l *lockout) dropNewest(key string) {
	switch ts := l.fail[key]; len(ts) {
	case 0:
	case 1:
		delete(l.fail, key)
	default:
		l.fail[key] = ts[:len(ts)-1]
	}
}

// success clears the pair and takes back this attempt's charge on the
// source, but no more: an attacker with one valid account must not reset its
// spraying count by logging in.
func (l *lockout) success(user, src string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.fail, pairKey(user, src))
	l.dropNewest(sourceKey(src))
}

// abandon takes back the charge of an attempt that checked no password.
func (l *lockout) abandon(user, src string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.dropNewest(pairKey(user, src))
	l.dropNewest(sourceKey(src))
}

// evict drops expired entries, then, if still full, half of the rest.
// Caller holds mu.
func (l *lockout) evict(now time.Time) {
	for k := range l.fail {
		l.recent(k, now)
	}
	if len(l.fail) < maxTrackedKeys {
		return
	}
	n := len(l.fail) / 2
	for k := range l.fail {
		if n == 0 {
			break
		}
		delete(l.fail, k)
		n--
	}
}
