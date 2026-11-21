// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package sessions

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Mem keeps sessions in this process's memory, with the semantics of the
// Well's table: for tests, and for a development stack that has no Well
// (pkg/bff, AUTH_SESSION_STORE=dev-memory). Sessions do not survive a
// restart, and another process, the command line included, cannot see or
// revoke them. Mem is an Admin; Front returns one front's Store.
type Mem struct {
	mu sync.Mutex
	m  map[string]Session // by KeyHash
}

// NewMem returns an empty Mem.
func NewMem() *Mem { return &Mem{m: make(map[string]Session)} }

// Front returns the Store of one front over m.
func (m *Mem) Front(front string) Store { return &memFront{m: m, front: front} }

// Len is the number of sessions held, ended or not.
func (m *Mem) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.m)
}

// Close does nothing.
func (m *Mem) Close() error { return nil }

func copySession(s Session) Session {
	s.Roles = append([]string(nil), s.Roles...)
	if s.RevokedAt != nil {
		t := *s.RevokedAt
		s.RevokedAt = &t
	}
	return s
}

// List implements Admin.
func (m *Mem) List(_ context.Context, f Filter, now time.Time) ([]Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Session
	for _, s := range m.m {
		if (f.Subject != "" && s.Subject != f.Subject) || (f.Front != "" && s.Front != f.Front) {
			continue
		}
		if !f.Ended && !s.Live(now) {
			continue
		}
		out = append(out, copySession(s))
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].KeyHash < out[j].KeyHash
	})
	if len(out) > ListLimit {
		out = out[:ListLimit]
	}
	return out, nil
}

// Revoke implements Admin.
func (m *Mem) Revoke(_ context.Context, prefix string, now time.Time) (Session, error) {
	if err := checkPrefix(prefix); err != nil {
		return Session{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var hit []string
	for k, s := range m.m {
		if strings.HasPrefix(k, prefix) && s.Live(now) {
			hit = append(hit, k)
		}
	}
	switch len(hit) {
	case 0:
		return Session{}, fmt.Errorf("%w: no live session with id %s", ErrNotFound, prefix)
	case 1:
	default:
		return Session{}, fmt.Errorf("%w: %s matches %d live sessions; give more characters", ErrAmbiguous, prefix, len(hit))
	}
	return copySession(m.revokeLocked(hit[0], now)), nil
}

// RevokeSubject implements Admin.
func (m *Mem) RevokeSubject(_ context.Context, subject, front string, now time.Time) (int, error) {
	if !ValidSubject(subject) {
		return 0, fmt.Errorf("%w: subject", ErrInvalid)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for k, s := range m.m {
		if s.Subject == subject && (front == "" || s.Front == front) && s.Live(now) {
			m.revokeLocked(k, now)
			n++
		}
	}
	return n, nil
}

// revokeLocked sets RevokedAt once, never before the session was created.
func (m *Mem) revokeLocked(key string, now time.Time) Session {
	s := m.m[key]
	if s.RevokedAt == nil {
		at := now
		if at.Before(s.CreatedAt) {
			at = s.CreatedAt
		}
		s.RevokedAt = &at
		m.m[key] = s
	}
	return s
}

type memFront struct {
	m     *Mem
	front string
}

func (f *memFront) Create(_ context.Context, s Session, maxLive int) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if s.Front != f.front {
		return fmt.Errorf("%w: a session of %q offered to the store of %q", ErrInvalid, s.Front, f.front)
	}
	f.m.mu.Lock()
	defer f.m.mu.Unlock()
	if _, exists := f.m.m[s.KeyHash]; exists {
		return fmt.Errorf("%w: key already in use", ErrInvalid)
	}
	live := 0
	for _, o := range f.m.m {
		if o.Front == f.front && o.Live(s.CreatedAt) {
			live++
		}
	}
	if live >= maxLive {
		return ErrFull
	}
	f.m.m[s.KeyHash] = copySession(s)
	return nil
}

func (f *memFront) Get(_ context.Context, keyHash string) (Session, bool, error) {
	f.m.mu.Lock()
	defer f.m.mu.Unlock()
	s, ok := f.m.m[keyHash]
	if !ok || s.Front != f.front {
		return Session{}, false, nil
	}
	return copySession(s), true, nil
}

func (f *memFront) Touch(_ context.Context, keyHash string, now, expires time.Time) (bool, error) {
	f.m.mu.Lock()
	defer f.m.mu.Unlock()
	s, ok := f.m.m[keyHash]
	if !ok || s.Front != f.front || !s.Live(now) || now.Before(s.LastUsedAt) {
		return false, nil
	}
	if expires.After(s.AbsoluteExpiresAt) {
		expires = s.AbsoluteExpiresAt
	}
	s.LastUsedAt, s.ExpiresAt = now, expires
	f.m.m[keyHash] = s
	return true, nil
}

func (f *memFront) Revoke(_ context.Context, keyHash string, now time.Time) error {
	f.m.mu.Lock()
	defer f.m.mu.Unlock()
	if s, ok := f.m.m[keyHash]; ok && s.Front == f.front {
		f.m.revokeLocked(keyHash, now)
	}
	return nil
}

func (f *memFront) Sweep(_ context.Context, endedBefore time.Time, limit int) (int, error) {
	f.m.mu.Lock()
	defer f.m.mu.Unlock()
	n := 0
	for k, s := range f.m.m {
		if n >= limit {
			break
		}
		if s.Front != f.front {
			continue
		}
		if s.ExpiresAt.Before(endedBefore) || (s.RevokedAt != nil && s.RevokedAt.Before(endedBefore)) {
			delete(f.m.m, k)
			n++
		}
	}
	return n, nil
}

func (f *memFront) Close() error { return nil }
