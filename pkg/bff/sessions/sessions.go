// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

// Package sessions is the server-side record of browser logins (ADR-114 N5,
// decision pack 2026-11-18 row 44): who logged in to which front, when the
// session was last used, when it ends, and whether it was revoked and by
// what.
//
// A session is known by the SHA-256 of its cookie value. Nothing here ever
// holds the cookie value or a token, so a dump of a store, in memory or in
// the Well, is nothing a browser can present.
//
// Two lifetimes bound a session: it ends when unused for the idle time
// (ExpiresAt moves forward with use) and, used or not, at its maximum age
// (AbsoluteExpiresAt, fixed at login). Ending is by clock or by revocation;
// an ended session is refused and later swept.
//
// A front (pkg/bff) holds a Store, bound to that one front. The command line
// holds an Admin, which lists and revokes across fronts and cannot create a
// session. Mem is both, for tests and for a development stack without a
// Well; the Well's are in well.go.
package sessions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
)

var (
	// ErrFull refuses a new session while the front is at its cap.
	ErrFull = errors.New("sessions: store full")
	// ErrClockSkew refuses a new session whose creation time is further
	// from the store's own clock than the caller allows. The front's clock
	// is what later judges the session's expiry, so a front whose clock is
	// wrong must not start sessions.
	ErrClockSkew = errors.New("sessions: clock skew")
	// ErrUnavailable is every call while the Well is not connected. A caller
	// treats it like any other store error: no session, no login.
	ErrUnavailable = errors.New("sessions: the Well is unavailable")
	// ErrNotFound: no session matches what Admin was given.
	ErrNotFound = errors.New("sessions: no such session")
	// ErrAmbiguous: a prefix given to Admin matches more than one session.
	ErrAmbiguous = errors.New("sessions: prefix matches more than one session")
	// ErrInvalid: a session or an argument is not one a store will hold.
	ErrInvalid = errors.New("sessions: invalid")
)

const (
	// MaxLifetime is the longest maximum age a session may be created with.
	// The Well's table carries the same bound (migration 056).
	MaxLifetime = 31 * 24 * time.Hour
	// MaxRoles and MaxSubjectLen bound what a session records.
	MaxRoles      = 16
	MaxSubjectLen = 255
	// MinPrefix is the shortest id prefix Admin revokes by.
	MinPrefix = 8
	// ListLimit bounds Admin.List.
	ListLimit = 1000
)

// What ended a session, as RevokedBy records it. A front writes the first
// three; the command line writes "cli:" and who ran it (CLIActor). The
// Well refuses a front that writes a "cli:" actor (migration 056).
const (
	// ByLogout: the user logged out.
	ByLogout = "logout"
	// ByLogin: the browser presented this session at a new login, which
	// replaced it.
	ByLogin = "login"
	// ByLimit: its user logged in past the per-user cap and this was the
	// oldest.
	ByLimit = "limit"
	// cliActorPrefix starts the actor of a revocation from the command
	// line.
	cliActorPrefix = "cli:"
	// maxActorWho bounds the name after the prefix (the table's CHECK).
	maxActorWho = 128
)

var (
	keyHashRE = regexp.MustCompile(`^[0-9a-f]{64}$`)
	prefixRE  = regexp.MustCompile(`^[0-9a-f]{8,64}$`)
	// frontRE is a service identity without an underscore, short enough
	// that the Well role named after it (WellRole) fits a role name.
	frontRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,58}$`)
	roleRE  = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
	// actorRE is what RevokedBy may hold; the table has the same CHECK.
	actorRE    = regexp.MustCompile(`^[a-z]{1,16}(:[A-Za-z0-9._@-]{1,128})?$`)
	notActorRE = regexp.MustCompile(`[^A-Za-z0-9._@-]`)
)

// Session is one browser login.
type Session struct {
	// KeyHash is KeyHash(cookie value): the session's name everywhere.
	KeyHash string
	// Front is the service that logged the user in. A session belongs to
	// one front and is no session to any other.
	Front   string
	Subject string
	Roles   []string

	CreatedAt  time.Time
	LastUsedAt time.Time
	// ExpiresAt is when the session ends unless it is used first.
	ExpiresAt time.Time
	// AbsoluteExpiresAt is when it ends whatever its use.
	AbsoluteExpiresAt time.Time
	// RevokedAt is set once, with RevokedBy: by a logout, a new login, the
	// per-user cap, or the command line.
	RevokedAt *time.Time
	// RevokedBy says what revoked it: ByLogout, ByLogin, ByLimit, or
	// "cli:" and who. Empty while RevokedAt is nil.
	RevokedBy string
}

// KeyHash is the name a cookie value is stored and looked up under.
func KeyHash(cookieValue string) string {
	sum := sha256.Sum256([]byte(cookieValue))
	return hex.EncodeToString(sum[:])
}

// ValidKeyHash reports whether s is the shape KeyHash returns.
func ValidKeyHash(s string) bool { return keyHashRE.MatchString(s) }

// ValidFront reports whether name can own sessions.
func ValidFront(name string) bool { return frontRE.MatchString(name) }

// ValidRole reports whether r is a role a session can record.
func ValidRole(r string) bool { return roleRE.MatchString(r) }

// ValidActor reports whether by can be recorded as what revoked a session.
func ValidActor(by string) bool { return actorRE.MatchString(by) }

// CLIActor is the actor of a revocation from the command line: "cli:" and
// who (an operating-system user at a host, say), with anything outside
// [A-Za-z0-9._@-] turned into "_" and the whole held to what the record
// takes. An empty who is "unknown".
func CLIActor(who string) string {
	who = notActorRE.ReplaceAllString(who, "_")
	if who == "" {
		who = "unknown"
	}
	if len(who) > maxActorWho {
		who = who[:maxActorWho]
	}
	return cliActorPrefix + who
}

// ValidSubject reports whether s is a subject a session can record: 1 to
// MaxSubjectLen bytes of UTF-8 with no space or control character.
func ValidSubject(s string) bool {
	if s == "" || len(s) > MaxSubjectLen {
		return false
	}
	// Invalid UTF-8 ranges as the replacement character.
	return !strings.ContainsFunc(s, func(r rune) bool {
		return r == unicode.ReplacementChar || unicode.IsSpace(r) || unicode.IsControl(r)
	})
}

// Live reports whether the session is neither revoked nor expired at now.
func (s Session) Live(now time.Time) bool {
	return s.RevokedAt == nil && now.Before(s.ExpiresAt) && now.Before(s.AbsoluteExpiresAt)
}

// State is "active", "revoked" or "expired" at now.
func (s Session) State(now time.Time) string {
	switch {
	case s.RevokedAt != nil:
		return "revoked"
	case !s.Live(now):
		return "expired"
	}
	return "active"
}

// EndedAt is when a session that is not live ended.
func (s Session) EndedAt() time.Time {
	end := s.ExpiresAt
	if s.RevokedAt != nil && s.RevokedAt.Before(end) {
		end = *s.RevokedAt
	}
	return end
}

// ID is the first 16 characters of the hash: what a listing shows and what
// Admin.Revoke takes. It is not the cookie and cannot be turned into it.
func (s Session) ID() string {
	if len(s.KeyHash) < 16 {
		return s.KeyHash
	}
	return s.KeyHash[:16]
}

// Validate checks a session about to be created.
func (s Session) Validate() error {
	switch {
	case !ValidKeyHash(s.KeyHash):
		return fmt.Errorf("%w: key is not a SHA-256 in hex", ErrInvalid)
	case !ValidFront(s.Front):
		return fmt.Errorf("%w: front %q (want [a-z0-9][a-z0-9-]*, at most 59 characters, no underscore)", ErrInvalid, s.Front)
	case !ValidSubject(s.Subject):
		return fmt.Errorf("%w: subject (1 to %d bytes, no space or control character)", ErrInvalid, MaxSubjectLen)
	case len(s.Roles) > MaxRoles:
		return fmt.Errorf("%w: %d roles, at most %d", ErrInvalid, len(s.Roles), MaxRoles)
	case s.CreatedAt.IsZero() || s.LastUsedAt.Before(s.CreatedAt):
		return fmt.Errorf("%w: created and last-used times", ErrInvalid)
	case !s.ExpiresAt.After(s.CreatedAt) || s.ExpiresAt.After(s.AbsoluteExpiresAt):
		return fmt.Errorf("%w: expiry must be after creation and not after the maximum age", ErrInvalid)
	case s.AbsoluteExpiresAt.Sub(s.CreatedAt) > MaxLifetime:
		return fmt.Errorf("%w: maximum age over %v", ErrInvalid, MaxLifetime)
	case s.RevokedAt != nil || s.RevokedBy != "":
		return fmt.Errorf("%w: a new session is not revoked", ErrInvalid)
	}
	for _, r := range s.Roles {
		if !ValidRole(r) {
			return fmt.Errorf("%w: role %q", ErrInvalid, r)
		}
	}
	return nil
}

// CreateLimits bound what Create writes.
type CreateLimits struct {
	// MaxLive is the front's cap of live sessions: past it Create is
	// ErrFull.
	MaxLive int
	// MaxPerSubject is one subject's cap of live sessions on the front.
	// A login past it is not refused: the subject's oldest live sessions
	// are revoked (ByLimit) until the new one fits. So a user can always
	// log in, and no one account can fill the front. Zero or less: no cap.
	MaxPerSubject int
	// MaxSkew is how far the session's CreatedAt may be from the store's
	// own clock, either way: past it Create is ErrClockSkew. Zero or less:
	// not checked.
	MaxSkew time.Duration
}

// Store is one front's sessions. Every method sees that front's sessions
// only. An implementation must be safe for concurrent use.
type Store interface {
	// Create stores s, whose Front must be the store's, within lim, and
	// returns how many of the subject's older sessions it revoked to keep
	// MaxPerSubject. A Create that fails changes nothing.
	Create(ctx context.Context, s Session, lim CreateLimits) (int, error)
	// Get returns the session under keyHash, live or ended: the caller
	// judges. False when the front has none.
	Get(ctx context.Context, keyHash string) (Session, bool, error)
	// Touch records a use at now on a session live at now, and reports
	// whether there was one: false means the session has ended or never
	// was. Last-used and the idle expiry only move forward (expires is
	// held to the maximum age), so of two uses recorded out of order the
	// later one stands and both report true.
	Touch(ctx context.Context, keyHash string, now, expires time.Time) (bool, error)
	// Revoke ends the session under keyHash at now and records by as what
	// ended it. Revoking twice, or revoking nothing, is no error; the
	// first revocation stands.
	Revoke(ctx context.Context, keyHash string, now time.Time, by string) error
	// Sweep deletes at most limit sessions that ended before endedBefore
	// and returns how many.
	Sweep(ctx context.Context, endedBefore time.Time, limit int) (int, error)
	// Close releases what the store holds.
	Close() error
}

// Filter narrows Admin.List. Empty fields match everything.
type Filter struct {
	Subject string
	Front   string
	// Revoked also lists revoked sessions not yet swept.
	Revoked bool
}

// Admin lists and revokes sessions of every front, for the command line.
// It cannot create one.
//
// Admin never judges expiry. The clock that decides whether a session has
// expired is its front's, and the caller's may differ: List returns every
// session not revoked, expired by the caller's clock or not, and the
// revocations act on every session not yet revoked. Revoking an expired
// session is harmless; hiding or skipping a live one that looked expired
// from here is not.
type Admin interface {
	// List returns at most ListLimit sessions, newest first.
	List(ctx context.Context, f Filter) ([]Session, error)
	// Revoke ends the one session not yet revoked whose hash starts with
	// prefix (at least MinPrefix hex characters) and records by:
	// ErrNotFound for none, ErrAmbiguous for several. now only stamps it.
	Revoke(ctx context.Context, prefix string, now time.Time, by string) (Session, error)
	// RevokeSubject ends every session of subject not yet revoked, on one
	// front or (front "") on all, records by, and returns how many.
	RevokeSubject(ctx context.Context, subject, front string, now time.Time, by string) (int, error)
	Close() error
}

func checkPrefix(prefix string) error {
	if !prefixRE.MatchString(prefix) {
		return fmt.Errorf("%w: a session id is %d to 64 hex characters (see the ID column of the listing)", ErrInvalid, MinPrefix)
	}
	return nil
}

func checkActor(by string) error {
	if !ValidActor(by) {
		return fmt.Errorf("%w: revocation actor %q", ErrInvalid, by)
	}
	return nil
}
