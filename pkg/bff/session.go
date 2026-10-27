// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package bff

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// Session is what the backend holds for one browser login. The access token
// never leaves the server (ADR-103 §4).
type Session struct {
	Token   string // the issuer's access token
	Subject string
	Roles   []string
	Expires time.Time // the token's exp; the session ends with it
}

// ErrStoreFull refuses a new session while the store is at its cap.
var ErrStoreFull = errors.New("bff: session store full")

// Store keeps sessions by the SHA-256 of their cookie value, so a dump of
// the store (or a future Well table) holds nothing a browser can present.
// An implementation must be safe for concurrent use.
type Store interface {
	Put(ctx context.Context, key string, s Session) error
	Get(ctx context.Context, key string) (Session, bool, error)
	Delete(ctx context.Context, key string) error
	// Evict removes sessions expired at now and returns how many.
	Evict(ctx context.Context, now time.Time) (int, error)
}

// storeKey is the key a cookie value is stored under.
func storeKey(cookie string) string {
	sum := sha256.Sum256([]byte(cookie))
	return hex.EncodeToString(sum[:])
}

// MemStore is an in-memory Store with a cap. Sessions do not survive a
// restart: users log in again.
type MemStore struct {
	max int
	now func() time.Time

	mu sync.Mutex
	m  map[string]Session
}

// NewMemStore returns a store holding at most max sessions (<= 0: 10000).
// A nil now means time.Now.
func NewMemStore(max int, now func() time.Time) *MemStore {
	if max <= 0 {
		max = 10000
	}
	if now == nil {
		now = time.Now
	}
	return &MemStore{max: max, now: now, m: make(map[string]Session)}
}

// Put stores s, evicting expired sessions first when at the cap.
func (s *MemStore) Put(_ context.Context, key string, sess Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.m[key]; !exists && len(s.m) >= s.max {
		s.evictLocked(s.now())
		if len(s.m) >= s.max {
			return ErrStoreFull
		}
	}
	sess.Roles = append([]string(nil), sess.Roles...)
	s.m[key] = sess
	return nil
}

// Get returns the session under key; an expired one is removed and not
// returned.
func (s *MemStore) Get(_ context.Context, key string) (Session, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.m[key]
	if !ok {
		return Session{}, false, nil
	}
	if !s.now().Before(sess.Expires) {
		delete(s.m, key)
		return Session{}, false, nil
	}
	sess.Roles = append([]string(nil), sess.Roles...)
	return sess, true, nil
}

// Delete removes the session under key, if any.
func (s *MemStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
	return nil
}

// Evict removes every session expired at now.
func (s *MemStore) Evict(_ context.Context, now time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.evictLocked(now), nil
}

func (s *MemStore) evictLocked(now time.Time) int {
	n := 0
	for k, sess := range s.m {
		if !now.Before(sess.Expires) {
			delete(s.m, k)
			n++
		}
	}
	return n
}

// Len is the number of sessions held, expired or not.
func (s *MemStore) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.m)
}
