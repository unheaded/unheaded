// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package bff

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"

	"unheaded/pkg/auth"
	"unheaded/pkg/bff/sessions"
)

// Server-side sessions (ADR-114 N5; decision pack 2026-11-18, row 44).
//
// Unset, AUTH_SESSION_STORE leaves a front as it was: sessions in this
// process's memory, each holding the issuer's token and ending with it
// (15 minutes with the built-in issuer).
//
// AUTH_SESSION_STORE=well keeps the session as a record in the Well
// instead (pkg/bff/sessions): who, which front, when last used, when it
// ends, whether it was revoked. The record is found by the SHA-256 of the
// cookie value; neither the value nor a token is stored. It is extended
// while in use (AUTH_SESSION_IDLE), ends at a maximum age whatever its use
// (AUTH_SESSION_MAX_AGE), and can be revoked from the command line
// (unheaded session revoke). The issuer's token is checked once, at login,
// and then dropped: the session no longer ends with it, so ending one
// early is a revocation.
//
// Every request with a session cookie reads the record, so a revocation or
// an expiry takes effect on the next request; nothing is cached. If the
// Well cannot be asked, the answer is no: no login, and no session
// accepted, the ones that were live a moment ago included. There is no
// fallback to memory.
//
// AUTH_SESSION_STORE=dev-memory is the same behaviour over this process's
// memory, for a development stack that has no Well. It has to be asked for
// by name: sessions then last hours with nothing outside the process able
// to revoke them.
const (
	// SessionStoreWell and SessionStoreDevMemory are the values of
	// EnvSessionStore; the empty string is the token-bound mode.
	SessionStoreWell      = "well"
	SessionStoreDevMemory = "dev-memory"

	EnvSessionStore     = "AUTH_SESSION_STORE"
	EnvSessionIdle      = "AUTH_SESSION_IDLE"
	EnvSessionMaxAge    = "AUTH_SESSION_MAX_AGE"
	EnvSessionRetention = "AUTH_SESSION_RETENTION"

	// DefaultSessionIdle and DefaultSessionMaxAge are ADR-114's decision 5
	// (2026-11-03): idle 30 minutes, absolute 12 hours.
	DefaultSessionIdle   = 30 * time.Minute
	DefaultSessionMaxAge = 12 * time.Hour
	// DefaultSessionRetention is how long an ended session stays on record
	// (for unheaded session list --all) before the sweep deletes it.
	DefaultSessionRetention = 24 * time.Hour

	minSessionIdle      = time.Minute
	maxSessionRetention = 90 * 24 * time.Hour
	// maxTouchInterval bounds how stale a session's last-used time may be:
	// a use is recorded when the last recorded one is this old (or a tenth
	// of the idle time, if that is shorter). A session in constant use is
	// thus never closer than this to its idle expiry than it should be.
	maxTouchInterval = time.Minute
	// sweepBatch bounds one sweep; the loop runs every evictionInterval.
	sweepBatch = 1000
	// storeErrorLogEvery keeps an outage from filling the log.
	storeErrorLogEvery = time.Minute
)

// envDuration reads one duration key: unset or empty is 0 (the default),
// anything time.ParseDuration refuses, or a negative value, is an error.
func envDuration(key string) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("bff: %s=%q: want a duration such as 30m or 12h", key, raw)
	}
	return d, nil
}

// serverSessions is the server-side mode's state on a BFF.
type serverSessions struct {
	store      sessions.Store
	owned      bool // opened here, so closed here
	idle       time.Duration
	maxAge     time.Duration
	retention  time.Duration
	touchEvery time.Duration
	maxLive    int

	lastErrLog atomic.Int64 // unix seconds of the last logged store error
}

// newServerSessions resolves cfg's session mode. Nil with a nil error is
// the token-bound mode.
func newServerSessions(cfg Config) (*serverSessions, error) {
	mode := cfg.SessionStore
	switch mode {
	case "":
		if cfg.SessionIdle != 0 || cfg.SessionMaxAge != 0 || cfg.SessionRetention != 0 {
			return nil, fmt.Errorf("bff: %s, %s and %s apply to server-side sessions; set %s=%s (or leave them unset)",
				EnvSessionIdle, EnvSessionMaxAge, EnvSessionRetention, EnvSessionStore, SessionStoreWell)
		}
		if cfg.Sessions != nil {
			return nil, fmt.Errorf("bff: a session store was given without %s", EnvSessionStore)
		}
		return nil, nil
	case SessionStoreWell, SessionStoreDevMemory:
	default:
		return nil, fmt.Errorf("bff: %s=%q: want %q, %q (a development stack without a Well) or unset", EnvSessionStore, mode, SessionStoreWell, SessionStoreDevMemory)
	}
	if cfg.Store != nil {
		return nil, fmt.Errorf("bff: Config.Store is the token-bound store; with %s=%s use Config.Sessions", EnvSessionStore, mode)
	}
	if !sessions.ValidFront(cfg.ServiceName) {
		return nil, fmt.Errorf("bff: service name %q cannot keep server-side sessions (want [a-z0-9][a-z0-9-]*, at most 59 characters, no underscore)", cfg.ServiceName)
	}
	s := &serverSessions{idle: cfg.SessionIdle, maxAge: cfg.SessionMaxAge, retention: cfg.SessionRetention, maxLive: cfg.MaxSessions}
	if s.idle == 0 {
		s.idle = DefaultSessionIdle
	}
	if s.maxAge == 0 {
		s.maxAge = DefaultSessionMaxAge
	}
	if s.retention == 0 {
		s.retention = DefaultSessionRetention
	}
	if s.maxLive <= 0 {
		s.maxLive = 10000
	}
	switch {
	case s.idle < minSessionIdle:
		return nil, fmt.Errorf("bff: %s %v: at least %v", EnvSessionIdle, s.idle, minSessionIdle)
	case s.idle > s.maxAge:
		return nil, fmt.Errorf("bff: %s %v is longer than %s %v", EnvSessionIdle, s.idle, EnvSessionMaxAge, s.maxAge)
	case s.maxAge > sessions.MaxLifetime:
		return nil, fmt.Errorf("bff: %s %v: at most %v", EnvSessionMaxAge, s.maxAge, sessions.MaxLifetime)
	case s.retention < 0 || s.retention > maxSessionRetention:
		return nil, fmt.Errorf("bff: %s %v: between 0 and %v", EnvSessionRetention, s.retention, maxSessionRetention)
	}
	s.touchEvery = min(s.idle/10, maxTouchInterval)

	switch {
	case cfg.Sessions != nil:
		s.store = cfg.Sessions
	case mode == SessionStoreDevMemory:
		s.store, s.owned = sessions.NewMem().Front(cfg.ServiceName), true
		log.Warn().Str("service", cfg.ServiceName).Dur("idle", s.idle).Dur("max_age", s.maxAge).
			Msgf("bff: %s=%s: sessions are kept in this process's memory; a restart ends them and nothing outside this process can revoke one (development only)", EnvSessionStore, SessionStoreDevMemory)
	default:
		st, err := sessions.OpenWell(context.Background(), cfg.ServiceName)
		if err != nil {
			return nil, fmt.Errorf("bff: %s=%s: %w", EnvSessionStore, SessionStoreWell, err)
		}
		s.store, s.owned = st, true
		log.Info().Str("service", cfg.ServiceName).Dur("idle", s.idle).Dur("max_age", s.maxAge).
			Msg("bff: sessions are kept in the Well")
	}
	return s, nil
}

func (s *serverSessions) close() {
	if s.owned {
		_ = s.store.Close()
	}
}

// logStoreError logs a store failure at most once in storeErrorLogEvery.
func (b *BFF) logStoreError(err error, what string) {
	now := b.now().Unix()
	last := b.sess.lastErrLog.Load()
	if now-last < int64(storeErrorLogEvery/time.Second) && now >= last {
		return
	}
	if b.sess.lastErrLog.CompareAndSwap(last, now) {
		log.Error().Err(err).Str("service", b.cfg.ServiceName).
			Msgf("bff: session store: %s; no session is accepted while this lasts", what)
	}
}

// sessionRoles keeps the roles a session record can hold. One it cannot
// (not [a-z][a-z0-9_-]{0,31}) is left out: a role that is not held grants
// nothing. More roles than a record holds is an error: dropping some would
// change what the user may do in a way nobody chose.
func sessionRoles(roles []string) (kept, dropped []string, err error) {
	seen := make(map[string]bool, len(roles))
	for _, r := range roles {
		switch {
		case seen[r]:
		case sessions.ValidRole(r):
			seen[r] = true
			kept = append(kept, r)
		default:
			dropped = append(dropped, r)
		}
	}
	if len(kept) > sessions.MaxRoles {
		return nil, nil, fmt.Errorf("%d roles, a session holds at most %d", len(kept), sessions.MaxRoles)
	}
	return kept, dropped, nil
}

// loginServerSide finishes a login whose token has verified: it ends the
// session the browser presented, if any, writes a new record under a new
// id and sets the cookie. The token is not kept.
func (b *BFF) loginServerSide(w http.ResponseWriter, r *http.Request, id *auth.Identity, client string) {
	roles, dropped, err := sessionRoles(id.Roles)
	if err != nil || !sessions.ValidSubject(id.Subject) {
		log.Error().Err(err).Str("service", b.cfg.ServiceName).Str("source", client).
			Msg("bff: the issuer's token names a subject or roles a session cannot record; login refused")
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "issuer_token_invalid"})
		return
	}
	if len(dropped) > 0 {
		log.Warn().Str("service", b.cfg.ServiceName).Str("user", id.Subject).Int("dropped", len(dropped)).
			Msg("bff: roles a session cannot record were left out of this login")
	}
	now := b.now()
	// A login always gets a fresh id; a presented one is ended, so a
	// planted cookie cannot become someone's session (fixation). If the
	// store cannot be reached the create below fails too, and nobody is
	// logged in.
	if c, err := r.Cookie(b.cfg.CookieName); err == nil && validSessionID(c.Value) {
		if err := b.sess.store.Revoke(r.Context(), storeKey(c.Value), now); err != nil {
			b.logStoreError(err, "cannot end the session presented at login")
		}
	}
	sid, err := newSessionID()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server_error"})
		return
	}
	s := sessions.Session{
		KeyHash:           storeKey(sid),
		Front:             b.cfg.ServiceName,
		Subject:           id.Subject,
		Roles:             roles,
		CreatedAt:         now,
		LastUsedAt:        now,
		ExpiresAt:         now.Add(b.sess.idle),
		AbsoluteExpiresAt: now.Add(b.sess.maxAge),
	}
	if err := b.sess.store.Create(r.Context(), s, b.sess.maxLive); err != nil {
		if errors.Is(err, sessions.ErrFull) {
			log.Error().Str("service", b.cfg.ServiceName).Int("cap", b.sess.maxLive).Msg("bff: session store full; login refused")
		} else {
			b.logStoreError(err, "cannot store a new session")
		}
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sessions_unavailable"})
		return
	}
	// No Max-Age and no Expires: the server ends the session, and the
	// browser is not asked to keep the id past its own session.
	// #nosec G124 -- HttpOnly and SameSite=Strict always; Secure unless AUTH_BFF_DEV_PLAINTEXT, the documented mode for a stack without TLS (ADR-103)
	http.SetCookie(w, &http.Cookie{
		Name:     b.cfg.CookieName,
		Value:    sid,
		Path:     "/",
		HttpOnly: true,
		Secure:   !b.cfg.DevPlaintext,
		SameSite: http.SameSiteStrictMode,
	})
	log.Info().Str("service", b.cfg.ServiceName).Str("user", s.Subject).Strs("roles", s.Roles).Str("source", client).
		Str("session", s.ID()).Msg("bff: login")
	writeJSON(w, http.StatusOK, serverMeBody(s))
}

func serverMeBody(s sessions.Session) map[string]any {
	body := meBody(s.Subject, s.Roles, s.ExpiresAt)
	body["absolute_expires_at"] = s.AbsoluteExpiresAt.UTC().Format(time.RFC3339)
	return body
}

// serverSession returns the live session the request's cookie names, after
// recording the use if one is due. Every failure is "no session".
func (b *BFF) serverSession(r *http.Request) (sessions.Session, error) {
	none := func(why string) (sessions.Session, error) {
		return sessions.Session{}, fmt.Errorf("%w: %s", auth.ErrUnauthenticated, why)
	}
	c, err := r.Cookie(b.cfg.CookieName)
	if err != nil || !validSessionID(c.Value) {
		return none("no session")
	}
	key := storeKey(c.Value)
	s, ok, err := b.sess.store.Get(r.Context(), key)
	if err != nil {
		b.logStoreError(err, "cannot read a session")
		return none("sessions unavailable")
	}
	if !ok {
		return none("no session")
	}
	// The store answered for this key and this front, or it is not
	// believed: a record of another front, or under another key, is no
	// session here whatever returned it.
	if subtle.ConstantTimeCompare([]byte(s.KeyHash), []byte(key)) != 1 || s.Front != b.cfg.ServiceName {
		log.Error().Str("service", b.cfg.ServiceName).Str("record_front", s.Front).
			Msg("bff: the session store returned a record that is not this front's or not under the key asked for; refused")
		return none("no session")
	}
	now := b.now()
	if !s.Live(now) {
		return none("session ended")
	}
	if now.Sub(s.LastUsedAt) >= b.sess.touchEvery {
		touched, err := b.sess.store.Touch(r.Context(), key, now, now.Add(b.sess.idle))
		switch {
		case err != nil:
			// The record just read says the session is live; a use that
			// could not be written leaves it ending when that record says.
			b.logStoreError(err, "cannot record a session's use")
		case !touched:
			// Revoked or run out between the read and the write.
			return none("session ended")
		default:
			s.LastUsedAt = now
			if s.ExpiresAt = now.Add(b.sess.idle); s.ExpiresAt.After(s.AbsoluteExpiresAt) {
				s.ExpiresAt = s.AbsoluteExpiresAt
			}
		}
	}
	return s, nil
}

// authenticateServerSide is authenticate for the server-side mode: the
// identity comes from the session record, not from a token.
func (b *BFF) authenticateServerSide(r *http.Request) (*auth.Identity, sessions.Session, error) {
	s, err := b.serverSession(r)
	if err != nil {
		return nil, sessions.Session{}, err
	}
	return &auth.Identity{
		Subject: s.Subject,
		Roles:   append([]string(nil), s.Roles...),
		Extra:   map[string]string{"auth_method": "session"},
	}, s, nil
}

// logoutServerSide ends the session the cookie names. The caller clears
// the cookie whatever happens here.
func (b *BFF) logoutServerSide(r *http.Request) {
	c, err := r.Cookie(b.cfg.CookieName)
	if err != nil || !validSessionID(c.Value) {
		return
	}
	key := storeKey(c.Value)
	if err := b.sess.store.Revoke(r.Context(), key, b.now()); err != nil {
		b.logStoreError(err, "cannot end a session at logout")
		return
	}
	log.Info().Str("service", b.cfg.ServiceName).Str("session", key[:16]).Msg("bff: logout")
}

// sweepServerSide deletes this front's sessions that ended more than the
// retention ago, at most sweepBatch a call.
func (b *BFF) sweepServerSide(ctx context.Context) int {
	n, err := b.sess.store.Sweep(ctx, b.now().Add(-b.sess.retention), sweepBatch)
	if err != nil {
		if !errors.Is(err, sessions.ErrUnavailable) {
			b.logStoreError(err, "cannot sweep ended sessions")
		}
		return 0
	}
	return n
}
