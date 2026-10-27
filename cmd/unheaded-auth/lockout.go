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

// locked reports whether user from src is locked out and, if so, for how
// long.
func (l *lockout) locked(user, src string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	var wait time.Duration
	for _, k := range []struct {
		key string
		max int
	}{{pairKey(user, src), maxFailuresPerPair}, {sourceKey(src), maxFailuresPerSource}} {
		ts := l.recent(k.key, now)
		if len(ts) >= k.max {
			// Unlocked once enough of the oldest failures have aged out.
			until := ts[len(ts)-k.max].Add(lockoutWindow).Sub(now)
			if until > wait {
				wait = until
			}
		}
	}
	return wait > 0, wait
}

// failure records a failed login.
func (l *lockout) failure(user, src string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if len(l.fail) >= maxTrackedKeys {
		l.evict(now)
	}
	for _, k := range []struct {
		key string
		max int
	}{{pairKey(user, src), maxFailuresPerPair}, {sourceKey(src), maxFailuresPerSource}} {
		ts := append(l.recent(k.key, now), now)
		if len(ts) > k.max {
			ts = ts[len(ts)-k.max:]
		}
		l.fail[k.key] = ts
	}
}

// success clears the pair (not the source: an attacker with one valid
// account must not reset its spraying count by logging in).
func (l *lockout) success(user, src string) {
	l.mu.Lock()
	delete(l.fail, pairKey(user, src))
	l.mu.Unlock()
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
