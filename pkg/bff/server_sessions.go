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
	"strconv"
	"strings"
	"sync"
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
// ends, whether it was revoked and by what. The record is found by the
// SHA-256 of the cookie value; neither the value nor a token is stored. It
// is extended while in use (AUTH_SESSION_IDLE), ends at a maximum age
// whatever its use (AUTH_SESSION_MAX_AGE), and can be revoked from the
// command line (unheaded session revoke). The issuer's token is checked
// once, at login, and then dropped: the session no longer ends with it, so
// ending one early is a revocation.
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
	// EnvSessionMaxPerUser caps one user's live sessions on this front; a
	// login past it ends that user's oldest.
	EnvSessionMaxPerUser = "AUTH_SESSION_MAX_PER_USER"
	// EnvSessionMaxClockSkew is how far this host's clock may be from the
	// Well's when a session starts.
	EnvSessionMaxClockSkew = "AUTH_SESSION_MAX_CLOCK_SKEW"
	// EnvSessionMissBudget is how many unknown or ended session ids one
	// client address may present in a minute before further unknown ones
	// are refused without asking the store.
	EnvSessionMissBudget = "AUTH_SESSION_MISS_BUDGET"

	// DefaultSessionIdle and DefaultSessionMaxAge are ADR-114's decision 5
	// (2026-11-03): idle 30 minutes, absolute 12 hours.
	DefaultSessionIdle   = 30 * time.Minute
	DefaultSessionMaxAge = 12 * time.Hour
	// DefaultSessionRetention is how long an ended session stays on record
	// (for unheaded session list --all) before the sweep deletes it.
	DefaultSessionRetention = 24 * time.Hour
	// DefaultSessionMaxPerUser: a few browsers and devices each.
	DefaultSessionMaxPerUser = 10
	// DefaultSessionMaxClockSkew: far above what a synchronised clock
	// drifts, far below the shortest idle time.
	DefaultSessionMaxClockSkew = 2 * time.Minute
	// DefaultSessionMissBudget: a page left open on an ended session asks a
	// few times a minute; a guesser asks as fast as it can.
	DefaultSessionMissBudget = 30

	minSessionIdle        = time.Minute
	maxSessionRetention   = 90 * 24 * time.Hour
	maxSessionMaxPerUser  = 1000
	minSessionClockSkew   = time.Second
	maxSessionClockSkew   = time.Hour
	maxSessionMissBudget  = 100000
	defaultMaxLiveSession = 10000
	// maxTouchInterval bounds how stale a session's last-used time may be:
	// a use is recorded when the last recorded one is this old (or a tenth
	// of the idle time, if that is shorter). A session in constant use is
	// thus never closer than this to its idle expiry than it should be.
	maxTouchInterval = time.Minute
	// sweepBatch bounds one sweep; the loop runs every evictionInterval.
	sweepBatch = 1000
	// storeTimeout bounds one request's work in the store, the wait for a
	// connection included. It is not the client's to cut short: see
	// storeCtx.
	storeTimeout = 5 * time.Second
	// quietLogEvery keeps an outage, or a flood, from filling the log.
	quietLogEvery = time.Minute
	// maxKnownSessions bounds the memory of session ids seen live.
	maxKnownSessions = 2 * defaultMaxLiveSession
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

// envCount reads one whole-number key: unset or empty is 0 (the default),
// anything that is not a positive integer is an error.
func envCount(key string) (int, error) {
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

// knownSessions remembers which session ids this process has lately seen
// live, by their hashes. It decides nothing about a session: a known id is
// still read from the store on every request. It only tells a request that
// is probably a real user's from one that is probably a guess, once an
// address has run out its budget of misses.
type knownSessions struct {
	mu  sync.Mutex
	m   map[string]time.Time // key hash -> when last seen live
	ttl time.Duration
}

func (k *knownSessions) has(key string, now time.Time) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	seen, ok := k.m[key]
	return ok && now.Sub(seen) < k.ttl
}

func (k *knownSessions) add(key string, now time.Time) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if _, ok := k.m[key]; !ok && len(k.m) >= maxKnownSessions {
		k.evictLocked(now)
		if len(k.m) >= maxKnownSessions {
			return
		}
	}
	k.m[key] = now
}

func (k *knownSessions) drop(key string) {
	k.mu.Lock()
	delete(k.m, key)
	k.mu.Unlock()
}

func (k *knownSessions) evict(now time.Time) {
	k.mu.Lock()
	k.evictLocked(now)
	k.mu.Unlock()
}

func (k *knownSessions) evictLocked(now time.Time) {
	for key, seen := range k.m {
		if now.Sub(seen) >= k.ttl {
			delete(k.m, key)
		}
	}
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
	maxPerUser int
	maxSkew    time.Duration
	missBudget int
	known      *knownSessions

	lastErrLog  atomic.Int64 // unix seconds of the last logged store error
	lastMissLog atomic.Int64 // and of the last logged spent miss budget
}

// newServerSessions resolves cfg's session mode. Nil with a nil error is
// the token-bound mode.
func newServerSessions(cfg Config) (*serverSessions, error) {
	mode := cfg.SessionStore
	switch mode {
	case "":
		if cfg.SessionIdle != 0 || cfg.SessionMaxAge != 0 || cfg.SessionRetention != 0 ||
			cfg.SessionMaxPerUser != 0 || cfg.SessionMaxClockSkew != 0 || cfg.SessionMissBudget != 0 {
			return nil, fmt.Errorf("bff: the AUTH_SESSION_* limits (%s, %s, %s, %s, %s, %s) apply to server-side sessions; set %s=%s (or leave them unset)",
				EnvSessionIdle, EnvSessionMaxAge, EnvSessionRetention, EnvSessionMaxPerUser, EnvSessionMaxClockSkew, EnvSessionMissBudget,
				EnvSessionStore, SessionStoreWell)
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
	// A session that lasts hours rides on its cookie. Sessions in the Well
	// are the deployed mode: the cookie must be Secure there, and the
	// development switch that drops Secure is refused beside it.
	if mode == SessionStoreWell && cfg.DevPlaintext {
		return nil, fmt.Errorf("bff: AUTH_BFF_DEV_PLAINTEXT with %s=%s: a session kept in the Well lasts hours and its cookie must be Secure; a stack without TLS uses %s=%s",
			EnvSessionStore, SessionStoreWell, EnvSessionStore, SessionStoreDevMemory)
	}
	s := &serverSessions{
		idle: cfg.SessionIdle, maxAge: cfg.SessionMaxAge, retention: cfg.SessionRetention, maxLive: cfg.MaxSessions,
		maxPerUser: cfg.SessionMaxPerUser, maxSkew: cfg.SessionMaxClockSkew, missBudget: cfg.SessionMissBudget,
	}
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
		s.maxLive = defaultMaxLiveSession
	}
	if s.maxPerUser == 0 {
		// The default never exceeds the front's own cap; a configured
		// value that does is refused below.
		s.maxPerUser = min(DefaultSessionMaxPerUser, s.maxLive)
	}
	if s.maxSkew == 0 {
		s.maxSkew = DefaultSessionMaxClockSkew
	}
	if s.missBudget == 0 {
		s.missBudget = DefaultSessionMissBudget
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
	case s.maxPerUser < 1 || s.maxPerUser > maxSessionMaxPerUser || s.maxPerUser > s.maxLive:
		return nil, fmt.Errorf("bff: %s %d: between 1 and %d, and no more than the front's cap of %d", EnvSessionMaxPerUser, s.maxPerUser, maxSessionMaxPerUser, s.maxLive)
	case s.maxSkew < minSessionClockSkew || s.maxSkew > maxSessionClockSkew:
		return nil, fmt.Errorf("bff: %s %v: between %v and %v", EnvSessionMaxClockSkew, s.maxSkew, minSessionClockSkew, maxSessionClockSkew)
	case s.missBudget < 1 || s.missBudget > maxSessionMissBudget:
		return nil, fmt.Errorf("bff: %s %d: between 1 and %d", EnvSessionMissBudget, s.missBudget, maxSessionMissBudget)
	}
	s.touchEvery = min(s.idle/10, maxTouchInterval)
	s.known = &knownSessions{m: make(map[string]time.Time), ttl: s.idle}

	switch {
	case cfg.Sessions != nil:
		s.store = cfg.Sessions
	case mode == SessionStoreDevMemory:
		mem := sessions.NewMem()
		mem.SetClock(cfg.Now)
		s.store, s.owned = mem.Front(cfg.ServiceName), true
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

// storeCtx is the context a request's store work runs under: the
// request's values without its cancellation, and a timeout of its own. A
// client that hangs up mid-query must not cancel the query: the driver
// would discard the connection, the pool is small, and the log would blame
// the store for what the client did.
func storeCtx(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(r.Context()), storeTimeout)
}

// quietly reports whether a line guarded by last may be logged now: at
// most one in quietLogEvery.
func (b *BFF) quietly(last *atomic.Int64) bool {
	now := b.now().Unix()
	prev := last.Load()
	if now >= prev && now-prev < int64(quietLogEvery/time.Second) {
		return false
	}
	return last.CompareAndSwap(prev, now)
}

// logStoreError logs a store failure at most once in quietLogEvery.
func (b *BFF) logStoreError(err error, what string) {
	if b.quietly(&b.sess.lastErrLog) {
		log.Error().Err(err).Str("service", b.cfg.ServiceName).
			Msgf("bff: session store: %s; no session is accepted while this lasts", what)
	}
}

// The miss budget. A request that presents a well-formed session id nobody
// holds costs a read of the store, and a flood of them from one address
// would crowd out everyone's reads. Each client address may miss
// missBudget times a minute (an unknown id, or one that has ended). Past
// that, an id this process has not lately seen live is refused without
// asking the store. Ids it has seen live, and the id of a login just made,
// are still asked for as always, so a person behind the same address as a
// flood, or behind the same proxy, keeps working.

func missKey(client string) string { return "m|" + client }

// missBudgetSpent reports whether client has run out its misses for this
// minute. With the limiter's table full of other addresses it answers yes:
// what cannot be counted is not let through.
func (b *BFF) missBudgetSpent(client string) bool {
	b.limitMu.Lock()
	defer b.limitMu.Unlock()
	now := b.now()
	w, ok := b.limits[missKey(client)]
	if !ok || now.Sub(w.start) >= time.Minute {
		return !ok && len(b.limits) >= maxLimiterKeys
	}
	return w.n >= b.sess.missBudget
}

// chargeMiss counts one miss for client.
func (b *BFF) chargeMiss(client string) {
	b.limitMu.Lock()
	defer b.limitMu.Unlock()
	now := b.now()
	key := missKey(client)
	w, ok := b.limits[key]
	if !ok || now.Sub(w.start) >= time.Minute {
		if !ok && len(b.limits) >= maxLimiterKeys {
			return
		}
		w = &window{start: now}
		b.limits[key] = w
	}
	w.n++
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
	ctx, cancel := storeCtx(r)
	defer cancel()
	now := b.now()
	// A login always gets a fresh id; a presented one is ended, so a
	// planted cookie cannot become someone's session (fixation). If the
	// store cannot be reached the create below fails too, and nobody is
	// logged in.
	if c, err := r.Cookie(b.cfg.CookieName); err == nil && validSessionID(c.Value) {
		old := storeKey(c.Value)
		if err := b.sess.store.Revoke(ctx, old, now, sessions.ByLogin); err != nil {
			b.logStoreError(err, "cannot end the session presented at login")
		}
		b.sess.known.drop(old)
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
	ended, err := b.sess.store.Create(ctx, s, sessions.CreateLimits{
		MaxLive: b.sess.maxLive, MaxPerSubject: b.sess.maxPerUser, MaxSkew: b.sess.maxSkew,
	})
	if err != nil {
		switch {
		case errors.Is(err, sessions.ErrFull):
			log.Error().Str("service", b.cfg.ServiceName).Int("cap", b.sess.maxLive).Msg("bff: session store full; login refused")
		case errors.Is(err, sessions.ErrClockSkew):
			// The browser is told only that sessions are unavailable; the
			// operator is told why.
			log.Error().Err(err).Str("service", b.cfg.ServiceName).Dur("bound", b.sess.maxSkew).
				Msgf("bff: clock skew: this host's clock and the session store's differ by more than %s; no login is accepted until the clock is right", EnvSessionMaxClockSkew)
		default:
			b.logStoreError(err, "cannot store a new session")
		}
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sessions_unavailable"})
		return
	}
	b.sess.known.add(s.KeyHash, now)
	// No Max-Age and no Expires: the server ends the session, and the
	// browser is not asked to keep the id past its own session.
	b.setSessionCookie(w, sid, 0)
	ev := log.Info().Str("service", b.cfg.ServiceName).Str("user", s.Subject).Strs("roles", s.Roles).Str("source", client).
		Str("session", s.ID())
	if ended > 0 {
		ev = ev.Int("ended_oldest", ended).Int("per_user_cap", b.sess.maxPerUser)
	}
	ev.Msg("bff: login")
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
	now := b.now()
	client := clientAddr(r)
	if !b.sess.known.has(key, now) && b.missBudgetSpent(client) {
		if b.quietly(&b.sess.lastMissLog) {
			log.Warn().Str("service", b.cfg.ServiceName).Str("source", client).Int("budget", b.sess.missBudget).
				Msg("bff: an address spent its budget of unknown session ids; further unknown ids from it are refused unread for a minute")
		}
		return none("no session")
	}
	ctx, cancel := storeCtx(r)
	defer cancel()
	s, ok, err := b.sess.store.Get(ctx, key)
	if err != nil {
		b.logStoreError(err, "cannot read a session")
		return none("sessions unavailable")
	}
	if !ok {
		b.sess.known.drop(key)
		b.chargeMiss(client)
		return none("no session")
	}
	// The store answered for this key and this front, or it is not
	// believed: a record of another front, or under another key, is no
	// session here whatever returned it.
	if subtle.ConstantTimeCompare([]byte(s.KeyHash), []byte(key)) != 1 || s.Front != b.cfg.ServiceName {
		log.Error().Str("service", b.cfg.ServiceName).Str("record_front", s.Front).
			Msg("bff: the session store returned a record that is not this front's or not under the key asked for; refused")
		b.chargeMiss(client)
		return none("no session")
	}
	ended := func() (sessions.Session, error) {
		b.sess.known.drop(key)
		b.chargeMiss(client)
		return none("session ended")
	}
	if !s.Live(now) {
		return ended()
	}
	if now.Sub(s.LastUsedAt) >= b.sess.touchEvery {
		touched, err := b.sess.store.Touch(ctx, key, now, now.Add(b.sess.idle))
		switch {
		case err != nil:
			// The record just read says the session is live; a use that
			// could not be written leaves it ending when that record says.
			b.logStoreError(err, "cannot record a session's use")
		case !touched:
			// Revoked or run out between the read and the write. (Not a
			// lost race with another request's write: the store finds a
			// live session whichever write lands first.)
			return ended()
		default:
			// Forward only, as the store keeps it: another request may
			// have recorded a later use already.
			if now.After(s.LastUsedAt) {
				s.LastUsedAt = now
			}
			exp := now.Add(b.sess.idle)
			if exp.After(s.AbsoluteExpiresAt) {
				exp = s.AbsoluteExpiresAt
			}
			if exp.After(s.ExpiresAt) {
				s.ExpiresAt = exp
			}
		}
	}
	b.sess.known.add(key, now)
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

// logoutServerSide ends the session the cookie names. An error means the
// store could not be told: the session is still on record as live, and the
// caller must say so.
func (b *BFF) logoutServerSide(r *http.Request) error {
	c, err := r.Cookie(b.cfg.CookieName)
	if err != nil || !validSessionID(c.Value) {
		return nil
	}
	key := storeKey(c.Value)
	ctx, cancel := storeCtx(r)
	defer cancel()
	if err := b.sess.store.Revoke(ctx, key, b.now(), sessions.ByLogout); err != nil {
		b.logStoreError(err, "cannot end a session at logout")
		return err
	}
	b.sess.known.drop(key)
	log.Info().Str("service", b.cfg.ServiceName).Str("session", key[:16]).Msg("bff: logout")
	return nil
}

// sweepServerSide deletes this front's sessions that ended more than the
// retention ago, at most sweepBatch a call.
func (b *BFF) sweepServerSide(ctx context.Context) int {
	b.sess.known.evict(b.now())
	n, err := b.sess.store.Sweep(ctx, b.now().Add(-b.sess.retention), sweepBatch)
	if err != nil {
		if !errors.Is(err, sessions.ErrUnavailable) {
			b.logStoreError(err, "cannot sweep ended sessions")
		}
		return 0
	}
	return n
}
