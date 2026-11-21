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
	mu  sync.Mutex
	m   map[string]Session // by KeyHash
	now func() time.Time   // the store's own clock, for CreateLimits.MaxSkew
}

// NewMem returns an empty Mem.
func NewMem() *Mem { return &Mem{m: make(map[string]Session)} }

// SetClock gives the store a clock of its own, as the Well has one: Create
// then holds a session's CreatedAt to CreateLimits.MaxSkew of it. Without
// one there is no second clock to disagree with, and MaxSkew is not
// checked.
func (m *Mem) SetClock(now func() time.Time) {
	m.mu.Lock()
	m.now = now
	m.mu.Unlock()
}

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

// newestFirst orders sessions as a listing shows them.
func newestFirst(out []Session) {
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].KeyHash < out[j].KeyHash
	})
}

// List implements Admin.
func (m *Mem) List(_ context.Context, f Filter) ([]Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Session
	for _, s := range m.m {
		if (f.Subject != "" && s.Subject != f.Subject) || (f.Front != "" && s.Front != f.Front) {
			continue
		}
		if !f.Revoked && s.RevokedAt != nil {
			continue
		}
		out = append(out, copySession(s))
	}
	newestFirst(out)
	if len(out) > ListLimit {
		out = out[:ListLimit]
	}
	return out, nil
}

// Revoke implements Admin.
func (m *Mem) Revoke(_ context.Context, prefix string, now time.Time, by string) (Session, error) {
	if err := checkPrefix(prefix); err != nil {
		return Session{}, err
	}
	if err := checkActor(by); err != nil {
		return Session{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var hit []string
	for k, s := range m.m {
		if strings.HasPrefix(k, prefix) && s.RevokedAt == nil {
			hit = append(hit, k)
		}
	}
	switch len(hit) {
	case 0:
		return Session{}, fmt.Errorf("%w: no session with id %s that is not already revoked", ErrNotFound, prefix)
	case 1:
	default:
		return Session{}, fmt.Errorf("%w: %s matches %d sessions; give more characters", ErrAmbiguous, prefix, len(hit))
	}
	return copySession(m.revokeLocked(hit[0], now, by)), nil
}

// RevokeSubject implements Admin.
func (m *Mem) RevokeSubject(_ context.Context, subject, front string, now time.Time, by string) (int, error) {
	if !ValidSubject(subject) {
		return 0, fmt.Errorf("%w: subject", ErrInvalid)
	}
	if err := checkActor(by); err != nil {
		return 0, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for k, s := range m.m {
		if s.Subject == subject && (front == "" || s.Front == front) && s.RevokedAt == nil {
			m.revokeLocked(k, now, by)
			n++
		}
	}
	return n, nil
}

// revokeLocked sets RevokedAt and RevokedBy once, the time never before the
// session was created.
func (m *Mem) revokeLocked(key string, now time.Time, by string) Session {
	s := m.m[key]
	if s.RevokedAt == nil {
		at := now
		if at.Before(s.CreatedAt) {
			at = s.CreatedAt
		}
		s.RevokedAt, s.RevokedBy = &at, by
		m.m[key] = s
	}
	return s
}

type memFront struct {
	m     *Mem
	front string
}

func (f *memFront) Create(_ context.Context, s Session, lim CreateLimits) (int, error) {
	if err := s.Validate(); err != nil {
		return 0, err
	}
	if s.Front != f.front {
		return 0, fmt.Errorf("%w: a session of %q offered to the store of %q", ErrInvalid, s.Front, f.front)
	}
	f.m.mu.Lock()
	defer f.m.mu.Unlock()
	if lim.MaxSkew > 0 && f.m.now != nil {
		if d := s.CreatedAt.Sub(f.m.now()); d > lim.MaxSkew || d < -lim.MaxSkew {
			return 0, fmt.Errorf("%w: the session's creation time is %v from the store's clock, bound %v", ErrClockSkew, d.Round(time.Second), lim.MaxSkew)
		}
	}
	if _, exists := f.m.m[s.KeyHash]; exists {
		return 0, fmt.Errorf("%w: key already in use", ErrInvalid)
	}
	live := 0
	var own []Session // the subject's live sessions on this front
	for _, o := range f.m.m {
		if o.Front != f.front || !o.Live(s.CreatedAt) {
			continue
		}
		live++
		if o.Subject == s.Subject {
			own = append(own, o)
		}
	}
	// The subject's oldest go first, until the new session fits its cap.
	var drop []Session
	if lim.MaxPerSubject > 0 && len(own) >= lim.MaxPerSubject {
		newestFirst(own)
		drop = own[lim.MaxPerSubject-1:]
	}
	if live-len(drop) >= lim.MaxLive {
		return 0, ErrFull // and nothing was revoked
	}
	for _, o := range drop {
		f.m.revokeLocked(o.KeyHash, s.CreatedAt, ByLimit)
	}
	f.m.m[s.KeyHash] = copySession(s)
	return len(drop), nil
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
	if !ok || s.Front != f.front || !s.Live(now) {
		return false, nil
	}
	if expires.After(s.AbsoluteExpiresAt) {
		expires = s.AbsoluteExpiresAt
	}
	// Forward only: a use recorded late does not undo a later one.
	if now.After(s.LastUsedAt) {
		s.LastUsedAt = now
	}
	if expires.After(s.ExpiresAt) {
		s.ExpiresAt = expires
	}
	f.m.m[keyHash] = s
	return true, nil
}

func (f *memFront) Revoke(_ context.Context, keyHash string, now time.Time, by string) error {
	if err := checkActor(by); err != nil {
		return err
	}
	f.m.mu.Lock()
	defer f.m.mu.Unlock()
	if s, ok := f.m.m[keyHash]; ok && s.Front == f.front {
		f.m.revokeLocked(keyHash, now, by)
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
