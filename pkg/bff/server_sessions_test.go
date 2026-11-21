// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package bff_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"

	"unheaded/pkg/auth"
	"unheaded/pkg/bff"
	"unheaded/pkg/bff/bfftest"
	"unheaded/pkg/bff/sessions"
)

// Server-side sessions (ADR-114 N5, decision pack 2026-11-18 row 44): the
// session is a record in a sessions.Store, extended while in use, capped at
// a maximum age, revocable from outside the process. These tests run on
// sessions.Mem, which answers as the Well's table does (the store suite in
// pkg/bff/sessions holds both to one contract).

// serverApp is an app whose BFF keeps server-side sessions in mem.
func serverApp(t *testing.T, mem *sessions.Mem, mod func(*bff.Config)) *app {
	t.Helper()
	return newApp(t, func(c *bff.Config) {
		c.SessionStore = bff.SessionStoreWell
		c.Sessions = mem.Front(c.ServiceName)
		if mod != nil {
			mod(c)
		}
	})
}

func (a *app) get(c *http.Cookie) int {
	return a.do(http.MethodGet, "/api/v1/thing", "", withCookie(c)).Code
}

// ---- lifetimes ----

// The complaint that started this: a login asked for again every 15
// minutes, when the issuer's token ran out.
func TestServerSession_OutlivesTheIssuersToken(t *testing.T) {
	a := serverApp(t, sessions.NewMem(), nil)
	c := a.login("dev", "operator")
	a.clk.Advance(16 * time.Minute)
	if code := a.get(c); code != http.StatusOK {
		t.Fatalf("16 minutes after login, past the token's life: %d, want 200", code)
	}
}

func TestServerSession_EndsWhenIdle(t *testing.T) {
	a := serverApp(t, sessions.NewMem(), nil)
	c := a.login("dev", "operator")
	a.clk.Advance(bff.DefaultSessionIdle - time.Second)
	if code := a.get(c); code != http.StatusOK {
		t.Fatalf("just inside the idle time: %d", code)
	}
	a.clk.Advance(bff.DefaultSessionIdle)
	if code := a.get(c); code != http.StatusUnauthorized {
		t.Fatalf("unused for the idle time: %d, want 401", code)
	}
	if code := a.do(http.MethodGet, "/auth/me", "", withCookie(c)).Code; code != http.StatusUnauthorized {
		t.Fatalf("/auth/me for an expired session: %d, want 401 (the page then shows the login)", code)
	}
	// Ended is ended: using it again does not bring it back.
	a.clk.Advance(time.Second)
	if code := a.get(c); code != http.StatusUnauthorized {
		t.Fatalf("an expired session came back: %d", code)
	}
}

func TestServerSession_ExtendedWhileInUseButNeverPastItsMaximumAge(t *testing.T) {
	a := serverApp(t, sessions.NewMem(), nil)
	c := a.login("dev", "operator")
	start := a.clk.Now()
	step := 20 * time.Minute
	for a.clk.Now().Add(step).Before(start.Add(bff.DefaultSessionMaxAge)) {
		a.clk.Advance(step)
		if code := a.get(c); code != http.StatusOK {
			t.Fatalf("in use every %v, refused %v after login: %d", step, a.clk.Now().Sub(start), code)
		}
	}
	if used := a.clk.Now().Sub(start); used < 11*time.Hour {
		t.Fatalf("the loop only covered %v", used)
	}
	a.clk.Advance(step) // now past the maximum age, 20 minutes after the last use
	if code := a.get(c); code != http.StatusUnauthorized {
		t.Fatalf("%v after login, in use throughout: %d, want 401 at the maximum age", a.clk.Now().Sub(start), code)
	}
}

func TestServerSession_LimitsAreConfigurable(t *testing.T) {
	a := serverApp(t, sessions.NewMem(), func(c *bff.Config) {
		c.SessionIdle, c.SessionMaxAge = 2*time.Minute, 5*time.Minute
	})
	c := a.login("dev", "operator")
	a.clk.Advance(90 * time.Second)
	if code := a.get(c); code != http.StatusOK {
		t.Fatalf("inside a 2-minute idle time: %d", code)
	}
	a.clk.Advance(90 * time.Second)
	if code := a.get(c); code != http.StatusOK {
		t.Fatalf("used 90 s ago: %d", code)
	}
	a.clk.Advance(90 * time.Second)
	if code := a.get(c); code != http.StatusOK {
		t.Fatalf("4.5 minutes in, used throughout: %d", code)
	}
	a.clk.Advance(31 * time.Second)
	if code := a.get(c); code != http.StatusUnauthorized {
		t.Fatalf("past a 5-minute maximum age: %d, want 401", code)
	}

	b := serverApp(t, sessions.NewMem(), func(c *bff.Config) { c.SessionIdle, c.SessionMaxAge = 2*time.Minute, 5*time.Minute })
	c2 := b.login("dev", "operator")
	b.clk.Advance(2 * time.Minute)
	if code := b.get(c2); code != http.StatusUnauthorized {
		t.Fatalf("idle for the configured 2 minutes: %d, want 401", code)
	}
}

// /auth/me tells the page both ends.
func TestServerSession_MeReportsBothLimits(t *testing.T) {
	a := serverApp(t, sessions.NewMem(), nil)
	c := a.login("dev", "operator")
	start := a.clk.Now()
	rec := a.do(http.MethodGet, "/auth/me", "", withCookie(c))
	var me struct {
		Sub      string   `json:"sub"`
		Roles    []string `json:"roles"`
		Expires  string   `json:"expires_at"`
		Absolute string   `json:"absolute_expires_at"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("/auth/me: %d %s", rec.Code, rec.Body)
	}
	exp, _ := time.Parse(time.RFC3339, me.Expires)
	abs, _ := time.Parse(time.RFC3339, me.Absolute)
	if me.Sub != "dev" || fmt.Sprint(me.Roles) != "[operator]" ||
		exp.Sub(start).Round(time.Minute) != bff.DefaultSessionIdle || abs.Sub(start).Round(time.Minute) != bff.DefaultSessionMaxAge {
		t.Fatalf("/auth/me: %+v", me)
	}
}

// ---- the cookie ----

func TestServerSession_CookieIsHardenedAndNotPersistent(t *testing.T) {
	a := serverApp(t, sessions.NewMem(), nil)
	pw := a.is.AddUser(t, "dev", "operator")
	rec := a.do(http.MethodPost, "/auth/login", loginBody("dev", pw), withXRW())
	if rec.Code != http.StatusOK {
		t.Fatalf("login: %d %s", rec.Code, rec.Body)
	}
	c := sessionCookie(t, rec)
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode || c.Path != "/" || c.Domain != "" || !strings.HasPrefix(c.Name, "__Host-") {
		t.Fatalf("cookie must be HttpOnly, Secure, SameSite=Strict, Path=/, no Domain, __Host-: %+v", c)
	}
	// The server ends the session; the browser is not asked to keep the
	// secret on disk for it (NIST SP 800-63B-4: not persistent, and cookie
	// expiry never relied on).
	if c.MaxAge != 0 || !c.Expires.IsZero() {
		t.Fatalf("the session cookie is persistent: Max-Age %d, Expires %v", c.MaxAge, c.Expires)
	}
	raw := strings.Join(rec.Header().Values("Set-Cookie"), "\n")
	for _, want := range []string{"HttpOnly", "Secure", "SameSite=Strict"} {
		if !strings.Contains(raw, want) {
			t.Fatalf("Set-Cookie %q lacks %s", raw, want)
		}
	}
	if strings.Contains(rec.Body.String(), c.Value) || strings.Contains(strings.ToLower(rec.Body.String()), "token") {
		t.Fatalf("the login answer carries the session id or a token: %s", rec.Body)
	}

	plain := newApp(t, func(c *bff.Config) { c.SessionStore = bff.SessionStoreDevMemory; c.DevPlaintext = true })
	pc := plain.login("dev", "operator")
	if !pc.HttpOnly || pc.Secure || pc.SameSite != http.SameSiteStrictMode || strings.HasPrefix(pc.Name, "__Host-") {
		t.Fatalf("DevPlaintext drops Secure and the prefix, nothing else: %+v", pc)
	}

	// Logout clears it with the same attributes.
	out := a.do(http.MethodPost, "/auth/logout", "", withCookie(c), withXRW())
	cleared := sessionCookie(t, out)
	if out.Code != http.StatusNoContent || cleared.MaxAge >= 0 || cleared.Value != "" || !cleared.HttpOnly || !cleared.Secure || cleared.SameSite != http.SameSiteStrictMode {
		t.Fatalf("logout: %d %+v", out.Code, cleared)
	}
}

// ---- what the store is given ----

// recording wraps a Store and notes every key it is handed.
type recording struct {
	sessions.Store
	mu      sync.Mutex
	keys    []string
	created []sessions.Session
	limits  []sessions.CreateLimits
	// ctxErrs and deadlines are what each call's context looked like.
	ctxErrs   []error
	deadlines []bool
	touches   atomic.Int64
	gets      atomic.Int64
}

func (r *recording) note(k string) { r.mu.Lock(); r.keys = append(r.keys, k); r.mu.Unlock() }
func (r *recording) Create(ctx context.Context, s sessions.Session, lim sessions.CreateLimits) (int, error) {
	r.note(s.KeyHash)
	r.mu.Lock()
	r.created = append(r.created, s)
	r.limits = append(r.limits, lim)
	r.ctxErrs = append(r.ctxErrs, ctx.Err())
	r.mu.Unlock()
	return r.Store.Create(ctx, s, lim)
}
func (r *recording) Get(ctx context.Context, k string) (sessions.Session, bool, error) {
	r.note(k)
	r.gets.Add(1)
	_, hasDeadline := ctx.Deadline()
	r.mu.Lock()
	r.ctxErrs = append(r.ctxErrs, ctx.Err())
	r.deadlines = append(r.deadlines, hasDeadline)
	r.mu.Unlock()
	return r.Store.Get(ctx, k)
}
func (r *recording) Touch(ctx context.Context, k string, now, exp time.Time) (bool, error) {
	r.note(k)
	r.touches.Add(1)
	return r.Store.Touch(ctx, k, now, exp)
}
func (r *recording) Revoke(ctx context.Context, k string, now time.Time, by string) error {
	r.note(k)
	return r.Store.Revoke(ctx, k, now, by)
}

// The store never sees the cookie value or a token: only the SHA-256 of
// the value, and who, which front and when.
func TestServerSession_StoreHoldsOnlyTheHash(t *testing.T) {
	mem := sessions.NewMem()
	rec := &recording{Store: mem.Front(svc)}
	a := newApp(t, func(c *bff.Config) { c.SessionStore = bff.SessionStoreWell; c.Sessions = rec })
	c := a.login("dev", "operator", "viewer")
	a.get(c)
	a.clk.Advance(5 * time.Minute)
	a.get(c)
	a.do(http.MethodPost, "/auth/logout", "", withCookie(c), withXRW())

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.keys) < 4 || len(rec.created) != 1 {
		t.Fatalf("store calls: %d keys, %d creates", len(rec.keys), len(rec.created))
	}
	want := sessions.KeyHash(c.Value)
	for _, k := range rec.keys {
		if k == c.Value || strings.Contains(k, c.Value) {
			t.Fatal("the store was handed the cookie value")
		}
		if k != want {
			t.Fatalf("store key %q is not the SHA-256 of the cookie value", k)
		}
	}
	s := rec.created[0]
	if s.Front != svc || s.Subject != "dev" || fmt.Sprint(s.Roles) != "[operator viewer]" || s.RevokedAt != nil {
		t.Fatalf("stored session: %+v", s)
	}
	if !s.ExpiresAt.Equal(s.CreatedAt.Add(bff.DefaultSessionIdle)) || !s.AbsoluteExpiresAt.Equal(s.CreatedAt.Add(bff.DefaultSessionMaxAge)) {
		t.Fatalf("stored limits: expires %v, absolute %v, created %v", s.ExpiresAt, s.AbsoluteExpiresAt, s.CreatedAt)
	}
	// Nothing in the record is, or contains, a JWS.
	for _, field := range append([]string{s.KeyHash, s.Front, s.Subject}, s.Roles...) {
		if strings.Count(field, ".") >= 2 || strings.HasPrefix(field, "eyJ") {
			t.Fatalf("a stored field looks like a token: %q", field)
		}
	}
}

// A session in use is not written on every request.
func TestServerSession_UseIsRecordedAtMostOnceAMinute(t *testing.T) {
	rec := &recording{Store: sessions.NewMem().Front(svc)}
	a := newApp(t, func(c *bff.Config) { c.SessionStore = bff.SessionStoreWell; c.Sessions = rec })
	c := a.login("dev", "operator")
	for i := 0; i < 20; i++ {
		a.clk.Advance(time.Second)
		a.get(c)
	}
	if n := rec.touches.Load(); n != 0 {
		t.Fatalf("%d writes in the first 20 seconds, want none", n)
	}
	if n := rec.gets.Load(); n != 20 {
		t.Fatalf("%d reads for 20 requests: every request must ask the store", n)
	}
	a.clk.Advance(time.Minute)
	a.get(c)
	a.get(c)
	if n := rec.touches.Load(); n != 1 {
		t.Fatalf("%d writes after a minute, want 1", n)
	}
}

// ---- fixation ----

func TestServerSession_LoginAlwaysIssuesANewIDAndEndsThePresentedOne(t *testing.T) {
	mem := sessions.NewMem()
	a := serverApp(t, mem, nil)

	// A planted id that was never a session does not become one.
	planted := &http.Cookie{Name: a.b.CookieName(), Value: base64.RawURLEncoding.EncodeToString([]byte("attacker-chosen-session-id-32byt"))}
	pw := a.is.AddUser(t, "victim", "operator")
	rec := a.do(http.MethodPost, "/auth/login", loginBody("victim", pw), withXRW(), withCookie(planted))
	if rec.Code != http.StatusOK {
		t.Fatalf("login: %d %s", rec.Code, rec.Body)
	}
	fresh := sessionCookie(t, rec)
	if fresh.Value == planted.Value {
		t.Fatal("login kept the planted session id (fixation)")
	}
	if code := a.get(planted); code != http.StatusUnauthorized {
		t.Fatalf("the planted id is a session after the victim's login: %d", code)
	}
	if code := a.get(fresh); code != http.StatusOK {
		t.Fatalf("the fresh session: %d", code)
	}

	// A live session presented at login is ended, not carried over.
	pw = a.is.AddUser(t, "victim", "operator")
	rec = a.do(http.MethodPost, "/auth/login", loginBody("victim", pw), withXRW(), withCookie(fresh))
	second := sessionCookie(t, rec)
	if second.Value == fresh.Value {
		t.Fatal("login reused the presented session id")
	}
	if code := a.get(fresh); code != http.StatusUnauthorized {
		t.Fatalf("the session presented at login still works: %d", code)
	}
	old, err := mem.List(context.Background(), sessions.Filter{Revoked: true})
	if err != nil || len(old) != 2 {
		t.Fatalf("sessions on record: %d %v", len(old), err)
	}
	for _, s := range old {
		if s.KeyHash == sessions.KeyHash(fresh.Value) && (s.RevokedAt == nil || s.RevokedBy != sessions.ByLogin) {
			t.Fatalf("the replaced session was not revoked in the store as replaced by a login: %+v", s)
		}
	}

	// Two logins never share an id.
	seen := map[string]bool{}
	for i := 0; i < 20; i++ {
		v := a.login(fmt.Sprintf("u%d", i), "viewer").Value
		if seen[v] || len(v) != 43 {
			t.Fatalf("session id %d: repeated or not 32 random bytes", i)
		}
		seen[v] = true
	}
}

// ---- one front's session is not another's ----

func twoFronts(t *testing.T, mem *sessions.Mem, storeFor func(front string) sessions.Store) (dash, board *bff.BFF, is *bfftest.Issuer) {
	t.Helper()
	is = bfftest.NewIssuer(t, "dashboard-backend", "kanban-app")
	mk := func(name string) *bff.BFF {
		b, err := bff.New(bff.Config{ServiceName: name, IssuerURL: is.URL, SessionStore: bff.SessionStoreWell, Sessions: storeFor(name)})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(b.Close)
		return b
	}
	return mk("dashboard-backend"), mk("kanban-app"), is
}

func loginAt(t *testing.T, b *bff.BFF, is *bfftest.Issuer, user string, roles ...string) *http.Cookie {
	t.Helper()
	pw := is.AddUser(t, user, roles...)
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(loginBody(user, pw)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(bff.HeaderRequestedWith, bff.RequestedWithValue)
	rec := httptest.NewRecorder()
	b.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login at %s: %d %s", b.CookieName(), rec.Code, rec.Body)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == b.CookieName() {
			return c
		}
	}
	t.Fatal("no session cookie")
	return nil
}

func authAs(b *bff.BFF, value string) error {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/thing", nil)
	req.AddCookie(&http.Cookie{Name: b.CookieName(), Value: value})
	_, err := b.Authenticator().Authenticate(context.Background(), req)
	return err
}

func TestServerSession_OneFrontsSessionIsRefusedByAnother(t *testing.T) {
	mem := sessions.NewMem() // one table, as in the Well
	dash, board, is := twoFronts(t, mem, mem.Front)
	c := loginAt(t, dash, is, "dev", "admin")
	if err := authAs(dash, c.Value); err != nil {
		t.Fatalf("the dashboard's own session: %v", err)
	}
	// The same id, sent to the board under the board's cookie name.
	if err := authAs(board, c.Value); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("the board accepted the dashboard's session: %v", err)
	}
	if dash.CookieName() == board.CookieName() {
		t.Fatal("two fronts share a cookie name")
	}
}

// leaky is a store that answers for any front: a bug, or a database role
// that sees too much. The BFF checks what it is handed.
type leaky struct {
	sessions.Store
	all *sessions.Mem
}

func (l leaky) Get(ctx context.Context, k string) (sessions.Session, bool, error) {
	list, _ := l.all.List(ctx, sessions.Filter{Revoked: true})
	for _, s := range list {
		if s.KeyHash == k {
			return s, true, nil
		}
	}
	return sessions.Session{}, false, nil
}

func TestServerSession_ASessionRecordOfAnotherFrontIsNotBelieved(t *testing.T) {
	mem := sessions.NewMem()
	dash, board, is := twoFronts(t, mem, func(front string) sessions.Store { return leaky{Store: mem.Front(front), all: mem} })
	c := loginAt(t, dash, is, "dev", "admin")
	if err := authAs(dash, c.Value); err != nil {
		t.Fatalf("the dashboard's own session through the leaky store: %v", err)
	}
	if err := authAs(board, c.Value); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("the board believed a record that names the dashboard: %v", err)
	}
}

// wrongKey answers every lookup with one fixed session, whatever was asked.
type wrongKey struct {
	sessions.Store
	answer sessions.Session
}

func (w wrongKey) Get(context.Context, string) (sessions.Session, bool, error) {
	return w.answer, true, nil
}

func TestServerSession_ARecordUnderAnotherKeyIsNotBelieved(t *testing.T) {
	mem := sessions.NewMem()
	now := time.Now()
	other := sessions.Session{
		KeyHash: sessions.KeyHash("someone-else"), Front: svc, Subject: "admin", Roles: []string{"admin"},
		CreatedAt: now, LastUsedAt: now, ExpiresAt: now.Add(time.Hour), AbsoluteExpiresAt: now.Add(2 * time.Hour),
	}
	a := newApp(t, func(c *bff.Config) {
		c.SessionStore = bff.SessionStoreWell
		c.Sessions = wrongKey{Store: mem.Front(svc), answer: other}
	})
	forged := &http.Cookie{Name: a.b.CookieName(), Value: base64.RawURLEncoding.EncodeToString(make([]byte, 32))}
	if code := a.get(forged); code != http.StatusUnauthorized {
		t.Fatalf("a record whose key is not the cookie's hash was believed: %d", code)
	}
}

// ---- revocation ----

func TestServerSession_RevocationTakesEffectOnTheNextRequest(t *testing.T) {
	mem := sessions.NewMem()
	a := serverApp(t, mem, nil)
	ctx := context.Background()
	one := a.login("dev", "operator")
	two := a.login("dev", "operator")
	other := a.login("alice", "operator")
	for _, c := range []*http.Cookie{one, two, other} {
		if code := a.get(c); code != http.StatusOK {
			t.Fatalf("before any revocation: %d", code)
		}
	}
	// One session, by the id a listing shows.
	if _, err := mem.Revoke(ctx, sessions.KeyHash(one.Value)[:16], a.clk.Now(), "cli:tester@host"); err != nil {
		t.Fatal(err)
	}
	if code := a.get(one); code != http.StatusUnauthorized {
		t.Fatalf("the very next request of a revoked session: %d, want 401", code)
	}
	if code := a.get(two); code != http.StatusOK {
		t.Fatalf("dev's other session: %d", code)
	}
	// Every session of a subject.
	if n, err := mem.RevokeSubject(ctx, "dev", "", a.clk.Now(), "cli:tester@host"); n != 1 || err != nil {
		t.Fatalf("revoke subject: %d %v", n, err)
	}
	if code := a.get(two); code != http.StatusUnauthorized {
		t.Fatalf("after revoking the subject: %d, want 401", code)
	}
	if code := a.do(http.MethodPost, "/api/v1/thing", "{}", withCookie(two), withXRW()).Code; code != http.StatusUnauthorized {
		t.Fatalf("a write with a revoked session: %d, want 401", code)
	}
	if code := a.get(other); code != http.StatusOK {
		t.Fatalf("another subject's session: %d", code)
	}
	// Using a revoked session does not bring it back, now or later.
	a.clk.Advance(2 * time.Minute)
	if code := a.get(one); code != http.StatusUnauthorized {
		t.Fatalf("a revoked session came back: %d", code)
	}
}

func TestServerSession_LogoutRevokesInTheStore(t *testing.T) {
	mem := sessions.NewMem()
	a := serverApp(t, mem, nil)
	c := a.login("dev", "operator")
	if code := a.do(http.MethodPost, "/auth/logout", "", withCookie(c)).Code; code != http.StatusForbidden {
		t.Fatalf("logout without X-Requested-With: %d, want 403", code)
	}
	if code := a.get(c); code != http.StatusOK {
		t.Fatal("a refused logout ended the session")
	}
	if code := a.do(http.MethodPost, "/auth/logout", "", withCookie(c), withXRW()).Code; code != http.StatusNoContent {
		t.Fatalf("logout: %d", code)
	}
	if code := a.get(c); code != http.StatusUnauthorized {
		t.Fatalf("after logout: %d", code)
	}
	list, _ := mem.List(context.Background(), sessions.Filter{Revoked: true})
	if len(list) != 1 || list[0].RevokedAt == nil || list[0].RevokedBy != sessions.ByLogout {
		t.Fatalf("the store after logout: %+v", list)
	}
}

// What changes with this mode, pinned so it is a decision and not an
// accident: the session no longer ends with the issuer's token or signing
// key. Ending one early is a revocation.
func TestServerSession_DoesNotEndWithTheIssuersKey(t *testing.T) {
	mem := sessions.NewMem()
	a := serverApp(t, mem, nil)
	c := a.login("dev", "operator")
	a.is.RotateKey(t)
	a.clk.Advance(20 * time.Minute)
	if code := a.get(c); code != http.StatusOK {
		t.Fatalf("after the issuer rotated its key: %d, want 200 (revoke to end it)", code)
	}
	if n, _ := mem.RevokeSubject(context.Background(), "dev", "", a.clk.Now(), "cli:tester@host"); n != 1 {
		t.Fatal("revocation found no session")
	}
	if code := a.get(c); code != http.StatusUnauthorized {
		t.Fatalf("after revocation: %d", code)
	}
}

// ---- the Well being down ----

// outage is a Store that can be switched off: every call then answers as
// the Well's gate does while it has no connection.
type outage struct {
	sessions.Store
	down      atomic.Bool
	touchFail atomic.Bool
}

func (o *outage) Create(ctx context.Context, s sessions.Session, lim sessions.CreateLimits) (int, error) {
	if o.down.Load() {
		return 0, sessions.ErrUnavailable
	}
	return o.Store.Create(ctx, s, lim)
}
func (o *outage) Get(ctx context.Context, k string) (sessions.Session, bool, error) {
	if o.down.Load() {
		return sessions.Session{}, false, sessions.ErrUnavailable
	}
	return o.Store.Get(ctx, k)
}
func (o *outage) Touch(ctx context.Context, k string, now, exp time.Time) (bool, error) {
	if o.down.Load() || o.touchFail.Load() {
		return false, sessions.ErrUnavailable
	}
	return o.Store.Touch(ctx, k, now, exp)
}
func (o *outage) Revoke(ctx context.Context, k string, now time.Time, by string) error {
	if o.down.Load() {
		return sessions.ErrUnavailable
	}
	return o.Store.Revoke(ctx, k, now, by)
}
func (o *outage) Sweep(ctx context.Context, before time.Time, limit int) (int, error) {
	if o.down.Load() {
		return 0, sessions.ErrUnavailable
	}
	return o.Store.Sweep(ctx, before, limit)
}

func TestServerSession_WellDownFailsClosed(t *testing.T) {
	o := &outage{Store: sessions.NewMem().Front(svc)}
	a := newApp(t, func(c *bff.Config) { c.SessionStore = bff.SessionStoreWell; c.Sessions = o })
	c := a.login("dev", "operator")
	pw := a.is.AddUser(t, "alice", "operator")

	o.down.Store(true)
	// No new login: 503, and no cookie that could later pass for a session.
	rec := a.do(http.MethodPost, "/auth/login", loginBody("alice", pw), withXRW())
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "sessions_unavailable") {
		t.Fatalf("login with the Well down: %d %s, want 503 sessions_unavailable", rec.Code, rec.Body)
	}
	for _, sc := range rec.Result().Cookies() {
		if sc.Name == a.b.CookieName() && sc.Value != "" {
			t.Fatal("a session cookie was set by a login that stored no session")
		}
	}
	// Existing sessions are not accepted on faith either, for reads or
	// writes, and neither is anything that merely looks like a session.
	if code := a.get(c); code != http.StatusUnauthorized {
		t.Fatalf("an existing session with the Well down: %d, want 401", code)
	}
	if code := a.do(http.MethodPost, "/api/v1/thing", "{}", withCookie(c), withXRW()).Code; code != http.StatusUnauthorized {
		t.Fatalf("a write with the Well down: %d, want 401", code)
	}
	if code := a.do(http.MethodGet, "/auth/me", "", withCookie(c)).Code; code != http.StatusUnauthorized {
		t.Fatalf("/auth/me with the Well down: %d", code)
	}
	wellFormed := &http.Cookie{Name: a.b.CookieName(), Value: base64.RawURLEncoding.EncodeToString(make([]byte, 32))}
	if code := a.get(wellFormed); code != http.StatusUnauthorized {
		t.Fatalf("a made-up session id with the Well down: %d: an outage must not log everyone in", code)
	}
	if code := a.do(http.MethodGet, "/api/v1/thing", "").Code; code != http.StatusUnauthorized {
		t.Fatalf("no cookie with the Well down: %d", code)
	}
	// The sweep fails quietly and deletes nothing.
	if n := a.b.Evict(context.Background()); n != 0 {
		t.Fatalf("evicted %d with the Well down", n)
	}

	// The Well comes back: the session is where it was, if still in time.
	o.down.Store(false)
	if code := a.get(c); code != http.StatusOK {
		t.Fatalf("the session after the Well returned: %d", code)
	}
	if code := a.do(http.MethodPost, "/auth/login", loginBody("alice", pw), withXRW()).Code; code != http.StatusOK {
		t.Fatalf("login after the Well returned: %d", code)
	}
}

// An outage does not stop the clock: a session that ran out during it is
// refused when the Well is back.
func TestServerSession_AnOutageDoesNotExtendASession(t *testing.T) {
	o := &outage{Store: sessions.NewMem().Front(svc)}
	a := newApp(t, func(c *bff.Config) { c.SessionStore = bff.SessionStoreWell; c.Sessions = o })
	c := a.login("dev", "operator")
	o.down.Store(true)
	a.clk.Advance(bff.DefaultSessionIdle + time.Minute)
	a.get(c)
	o.down.Store(false)
	if code := a.get(c); code != http.StatusUnauthorized {
		t.Fatalf("a session idle through an outage: %d, want 401", code)
	}
}

// A use that cannot be recorded costs the session nothing it had: the
// request is served on the record just read, and the session still ends
// when that record says, never later.
func TestServerSession_AFailedTouchNeitherRefusesNorExtends(t *testing.T) {
	o := &outage{Store: sessions.NewMem().Front(svc)}
	a := newApp(t, func(c *bff.Config) { c.SessionStore = bff.SessionStoreWell; c.Sessions = o })
	c := a.login("dev", "operator")
	o.touchFail.Store(true)
	a.clk.Advance(20 * time.Minute)
	if code := a.get(c); code != http.StatusOK {
		t.Fatalf("a live session whose use could not be recorded: %d", code)
	}
	a.clk.Advance(11 * time.Minute) // 31 minutes after login, 11 after the unrecorded use
	if code := a.get(c); code != http.StatusUnauthorized {
		t.Fatalf("the unrecorded use extended the session: %d", code)
	}
}

// ended makes Touch report that the session is gone: it was revoked or ran
// out between the read and the write.
type ended struct{ sessions.Store }

func (ended) Touch(context.Context, string, time.Time, time.Time) (bool, error) { return false, nil }

func TestServerSession_ASessionThatEndsBetweenReadAndWriteIsRefused(t *testing.T) {
	a := newApp(t, func(c *bff.Config) {
		c.SessionStore = bff.SessionStoreWell
		c.Sessions = ended{sessions.NewMem().Front(c.ServiceName)}
	})
	c := a.login("dev", "operator")
	if code := a.get(c); code != http.StatusOK {
		t.Fatalf("no write is due yet: %d", code)
	}
	a.clk.Advance(2 * time.Minute)
	if code := a.get(c); code != http.StatusUnauthorized {
		t.Fatalf("the store said the session had ended: %d, want 401", code)
	}
}

// Parallel requests on a session whose use is due to be recorded: every
// one of them is served. (The second write to land used to match no row,
// and its request was answered 401 on a live session.)
func TestServerSession_ParallelRequestsAtTheTouchBoundaryAreAllServed(t *testing.T) {
	mem := sessions.NewMem()
	rec := &recording{Store: mem.Front(svc)}
	a := newApp(t, func(c *bff.Config) { c.SessionStore = bff.SessionStoreWell; c.Sessions = rec })
	c := a.login("dev", "operator")
	for round := 0; round < 20; round++ {
		a.clk.Advance(2 * time.Minute) // a write is due
		var wg sync.WaitGroup
		codes := make([]int, 8)
		for i := range codes {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				codes[i] = a.get(c)
			}(i)
		}
		wg.Wait()
		for i, code := range codes {
			if code != http.StatusOK {
				t.Fatalf("round %d, request %d of 8 in parallel: %d, want 200 for a live session", round, i, code)
			}
		}
	}
	if rec.touches.Load() < 20 {
		t.Fatalf("%d writes over 20 rounds: the rounds did not reach the write", rec.touches.Load())
	}
}

// lateWriter makes every second Touch carry a time eight seconds in the
// past: the write of a request that read its clock first and reached the
// store last, after another request's later write.
type lateWriter struct {
	sessions.Store
	n atomic.Int64
}

func (l *lateWriter) Touch(ctx context.Context, k string, now, exp time.Time) (bool, error) {
	if l.n.Add(1)%2 == 0 {
		now, exp = now.Add(-8*time.Second), exp.Add(-8*time.Second)
	}
	return l.Store.Touch(ctx, k, now, exp)
}

// The regression: a write that lands second, carrying a time before the one
// already recorded, used to match no row and end the request with 401.
func TestServerSession_AWriteThatLandsSecondDoesNotEndTheSession(t *testing.T) {
	lw := &lateWriter{Store: sessions.NewMem().Front(svc)}
	a := newApp(t, func(c *bff.Config) {
		c.SessionStore = bff.SessionStoreWell
		c.Sessions = lw
		c.SessionIdle, c.SessionMaxAge = time.Minute, time.Hour // a use is recorded every 6 s
	})
	c := a.login("dev", "operator")
	a.clk.Advance(10 * time.Second)
	if code := a.get(c); code != http.StatusOK { // records a use at +10 s
		t.Fatalf("first write: %d", code)
	}
	a.clk.Advance(7 * time.Second)
	// Due again at +17 s; this write reaches the store stamped +9 s,
	// before the +10 s already on record.
	if code := a.get(c); code != http.StatusOK {
		t.Fatalf("a write carrying an earlier time than the record's: %d, want 200 for a live session", code)
	}
	if lw.n.Load() != 2 {
		t.Fatalf("%d writes, want 2: the late write was not exercised", lw.n.Load())
	}
	// The record was not moved back: the session still ends 60 s after +10 s.
	a.clk.Advance(52 * time.Second) // +69 s
	if code := a.get(c); code != http.StatusOK {
		t.Fatalf("at +69 s, inside the minute after the +10 s use: %d", code)
	}
}

// ---- bounds ----

func TestServerSession_FullStoreRefusesLogin(t *testing.T) {
	a := serverApp(t, sessions.NewMem(), func(c *bff.Config) { c.MaxSessions = 2 })
	a.login("u1", "viewer")
	a.login("u2", "viewer")
	pw := a.is.AddUser(t, "u3", "viewer")
	if code := a.do(http.MethodPost, "/auth/login", loginBody("u3", pw), withXRW()).Code; code != http.StatusServiceUnavailable {
		t.Fatalf("login past the cap: %d, want 503", code)
	}
	a.clk.Advance(bff.DefaultSessionIdle)
	if code := a.do(http.MethodPost, "/auth/login", loginBody("u3", pw), withXRW()).Code; code != http.StatusOK {
		t.Fatalf("login after the others expired: %d", code)
	}
}

// Ended sessions are kept for the retention time, for the record, and then
// deleted; live ones never are.
func TestServerSession_SweepDeletesEndedSessionsAfterTheRetention(t *testing.T) {
	mem := sessions.NewMem()
	a := serverApp(t, mem, func(c *bff.Config) { c.SessionRetention = time.Hour })
	gone := a.login("u1", "viewer")
	a.do(http.MethodPost, "/auth/logout", "", withCookie(gone), withXRW())
	a.login("u2", "viewer") // will expire idle
	if n := a.b.Evict(context.Background()); n != 0 || mem.Len() != 2 {
		t.Fatalf("swept %d at once; %d left", n, mem.Len())
	}
	a.clk.Advance(bff.DefaultSessionIdle + 59*time.Minute)
	keep := a.login("u3", "viewer")
	if n := a.b.Evict(context.Background()); n != 1 || mem.Len() != 2 {
		t.Fatalf("89 minutes on: swept %d (want 1, the logout), %d left", n, mem.Len())
	}
	a.clk.Advance(2 * time.Minute)
	if n := a.b.Evict(context.Background()); n != 1 || mem.Len() != 1 {
		t.Fatalf("an hour past the idle expiry: swept %d, %d left", n, mem.Len())
	}
	if code := a.get(keep); code != http.StatusOK {
		t.Fatalf("the live session after the sweeps: %d", code)
	}
}

// ---- roles and CSRF, as in the token mode ----

func TestServerSession_RolesAndCSRF(t *testing.T) {
	a := serverApp(t, sessions.NewMem(), nil)
	op := a.login("dev", "operator")
	if c := a.do(http.MethodPost, "/api/v1/thing", "{}", withCookie(op)).Code; c != http.StatusForbidden {
		t.Fatalf("cookie write without X-Requested-With (CSRF): %d, want 403", c)
	}
	rec := a.do(http.MethodPost, "/api/v1/thing", "{}", withCookie(op), withXRW())
	if rec.Code != http.StatusOK || rec.Header().Get("X-Sub") != "dev" || rec.Header().Get("X-Method") != "session" {
		t.Fatalf("operator write: %d sub %q method %q", rec.Code, rec.Header().Get("X-Sub"), rec.Header().Get("X-Method"))
	}
	viewer := a.login("guest1", "viewer")
	if c := a.do(http.MethodPost, "/api/v1/thing", "{}", withCookie(viewer), withXRW()).Code; c != http.StatusForbidden {
		t.Fatalf("viewer write: %d, want 403", c)
	}
	if c := a.get(viewer); c != http.StatusOK {
		t.Fatalf("viewer read: %d", c)
	}
	for name, v := range map[string]string{
		"not a session id": "not-a-session",
		"unknown id":       base64.RawURLEncoding.EncodeToString(make([]byte, 32)),
		"empty":            "",
		"too long":         strings.Repeat("A", 4096),
	} {
		if c := a.get(&http.Cookie{Name: a.b.CookieName(), Value: v}); c != http.StatusUnauthorized {
			t.Errorf("%s: %d, want 401", name, c)
		}
	}
}

// A role the store cannot hold is dropped (a role not held grants
// nothing); a login that would lose track of its roles is refused.
func TestServerSession_RolesTheStoreCannotHold(t *testing.T) {
	mem := sessions.NewMem()
	a := serverApp(t, mem, nil)
	c := a.login("dev", "operator", "Team Leads", "viewer,admin")
	rec := a.do(http.MethodGet, "/auth/me", "", withCookie(c))
	if !strings.Contains(rec.Body.String(), `"roles":["operator"]`) {
		t.Fatalf("/auth/me: %s, want the one representable role", rec.Body)
	}
	if code := a.do(http.MethodPost, "/api/v1/thing", "{}", withCookie(c), withXRW()).Code; code != http.StatusOK {
		t.Fatalf("operator write: %d", code)
	}
	many := make([]string, sessions.MaxRoles+1)
	for i := range many {
		many[i] = fmt.Sprintf("role%d", i)
	}
	pw := a.is.AddUser(t, "crowd", many...)
	if code := a.do(http.MethodPost, "/auth/login", loginBody("crowd", pw), withXRW()).Code; code != http.StatusBadGateway {
		t.Fatalf("login with more roles than a session holds: %d, want 502", code)
	}
}

// ---- configuration ----

func TestServerSession_Config(t *testing.T) {
	is := bfftest.NewIssuer(t, svc)
	base := func() bff.Config { return bff.Config{ServiceName: svc, IssuerURL: is.URL} }
	mem := sessions.NewMem()

	bad := map[string]func(*bff.Config){
		"an unknown store":               func(c *bff.Config) { c.SessionStore = "redis"; c.Sessions = mem.Front(svc) },
		"memory spelled without dev":     func(c *bff.Config) { c.SessionStore = "memory"; c.Sessions = mem.Front(svc) },
		"limits without a store":         func(c *bff.Config) { c.SessionIdle = 10 * time.Minute },
		"a maximum age without a store":  func(c *bff.Config) { c.SessionMaxAge = time.Hour },
		"retention without a store":      func(c *bff.Config) { c.SessionRetention = time.Hour },
		"a per-user cap without a store": func(c *bff.Config) { c.SessionMaxPerUser = 3 },
		"a clock bound without a store":  func(c *bff.Config) { c.SessionMaxClockSkew = time.Minute },
		"a miss budget without a store":  func(c *bff.Config) { c.SessionMissBudget = 5 },
		"a negative per-user cap":        func(c *bff.Config) { c.SessionStore = bff.SessionStoreDevMemory; c.SessionMaxPerUser = -1 },
		"a per-user cap over the front's": func(c *bff.Config) {
			c.SessionStore = bff.SessionStoreDevMemory
			c.MaxSessions, c.SessionMaxPerUser = 5, 6
		},
		"a clock bound under a second": func(c *bff.Config) {
			c.SessionStore = bff.SessionStoreDevMemory
			c.SessionMaxClockSkew = time.Millisecond
		},
		"a clock bound over an hour": func(c *bff.Config) { c.SessionStore = bff.SessionStoreDevMemory; c.SessionMaxClockSkew = 2 * time.Hour },
		"a negative miss budget":     func(c *bff.Config) { c.SessionStore = bff.SessionStoreDevMemory; c.SessionMissBudget = -1 },
		"the Well with the plaintext switch": func(c *bff.Config) {
			c.SessionStore = bff.SessionStoreWell
			c.Sessions = mem.Front(svc)
			c.DevPlaintext = true
		},
		"a store handed in without a mode": func(c *bff.Config) { c.Sessions = mem.Front(svc) },
		"idle under a minute":              func(c *bff.Config) { c.SessionStore = bff.SessionStoreDevMemory; c.SessionIdle = 30 * time.Second },
		"idle over the maximum age": func(c *bff.Config) {
			c.SessionStore = bff.SessionStoreDevMemory
			c.SessionIdle, c.SessionMaxAge = 2*time.Hour, time.Hour
		},
		"a maximum age over the bound": func(c *bff.Config) {
			c.SessionStore = bff.SessionStoreDevMemory
			c.SessionMaxAge = sessions.MaxLifetime + time.Hour
		},
		"a negative idle time":        func(c *bff.Config) { c.SessionStore = bff.SessionStoreDevMemory; c.SessionIdle = -time.Minute },
		"a negative retention":        func(c *bff.Config) { c.SessionStore = bff.SessionStoreDevMemory; c.SessionRetention = -time.Minute },
		"both an old and a new store": func(c *bff.Config) { c.SessionStore = bff.SessionStoreDevMemory; c.Store = bff.NewMemStore(1, nil) },
		"a front name the Well cannot hold": func(c *bff.Config) {
			c.ServiceName = "dash_board"
			c.SessionStore = bff.SessionStoreWell
			c.Sessions = mem.Front("dash_board")
		},
	}
	for name, mod := range bad {
		c := base()
		mod(&c)
		if b, err := bff.New(c); err == nil {
			b.Close()
			t.Errorf("%s: accepted", name)
		}
	}

	c := base()
	c.SessionStore = bff.SessionStoreDevMemory
	b, err := bff.New(c)
	if err != nil {
		t.Fatalf("dev-memory: %v", err)
	}
	b.Close()
}

// "well" means the Well. With no way to reach it the front does not start;
// it never quietly keeps sessions somewhere else.
func TestServerSession_WellWithoutCredentialsRefusesToStart(t *testing.T) {
	is := bfftest.NewIssuer(t, svc)
	t.Setenv("AUTH_SESSION_WELL_PASSWORD", "")
	t.Setenv("WELL_HOST", "127.0.0.1")
	t.Setenv("WELL_PORT", "1")
	b, err := bff.New(bff.Config{ServiceName: svc, IssuerURL: is.URL, SessionStore: bff.SessionStoreWell})
	if err == nil {
		b.Close()
		t.Fatal("AUTH_SESSION_STORE=well started without a Well login")
	}
	if !strings.Contains(err.Error(), "AUTH_SESSION_WELL_PASSWORD") {
		t.Fatalf("the error does not say what to set: %v", err)
	}
}

// With the Well configured and not answering, the front starts and accepts
// nothing: no login, no session.
func TestServerSession_WellUnreachableStartsClosed(t *testing.T) {
	is := bfftest.NewIssuer(t, "unheaded", svc)
	t.Setenv("AUTH_SESSION_WELL_PASSWORD", "not-a-real-password")
	t.Setenv("WELL_HOST", "127.0.0.1")
	t.Setenv("WELL_PORT", "1")                       // nothing listens on port 1
	t.Setenv("AUTH_SESSION_WELL_SSLMODE", "disable") // this host only
	b, err := bff.New(bff.Config{ServiceName: svc, IssuerURL: is.URL, SessionStore: bff.SessionStoreWell})
	if err != nil {
		t.Fatalf("a Well that is down stopped the front from starting: %v", err)
	}
	defer b.Close()
	pw := is.AddUser(t, "dev", "operator")
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(loginBody("dev", pw)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(bff.HeaderRequestedWith, bff.RequestedWithValue)
	rec := httptest.NewRecorder()
	b.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("login with the Well unreachable: %d %s, want 503", rec.Code, rec.Body)
	}
	if err := authAs(b, base64.RawURLEncoding.EncodeToString(make([]byte, 32))); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("a session id with the Well unreachable: %v", err)
	}
}

func TestServerSession_ConfigFromEnv(t *testing.T) {
	t.Setenv("AUTH_ISSUER_URL", "https://auth.example")
	for _, k := range []string{bff.EnvSessionStore, bff.EnvSessionIdle, bff.EnvSessionMaxAge, bff.EnvSessionRetention,
		bff.EnvSessionMaxPerUser, bff.EnvSessionMaxClockSkew, bff.EnvSessionMissBudget} {
		t.Setenv(k, "")
	}
	c := bff.ConfigFromEnv(svc)
	if c.SessionStore != "" || c.SessionIdle != 0 || c.SessionMaxAge != 0 || c.SessionRetention != 0 ||
		c.SessionMaxPerUser != 0 || c.SessionMaxClockSkew != 0 || c.SessionMissBudget != 0 {
		t.Fatalf("unset keys: %+v", c)
	}
	t.Setenv(bff.EnvSessionMaxPerUser, "4")
	t.Setenv(bff.EnvSessionMaxClockSkew, "30s")
	t.Setenv(bff.EnvSessionMissBudget, " 12 ")
	if c = bff.ConfigFromEnv(svc); c.SessionMaxPerUser != 4 || c.SessionMaxClockSkew != 30*time.Second || c.SessionMissBudget != 12 {
		t.Fatalf("the three bounds from the environment: %+v", c)
	}
	t.Setenv(bff.EnvSessionStore, " dev-memory ")
	t.Setenv(bff.EnvSessionIdle, "45m")
	t.Setenv(bff.EnvSessionMaxAge, "8h")
	t.Setenv(bff.EnvSessionRetention, "72h")
	c = bff.ConfigFromEnv(svc)
	if c.SessionStore != bff.SessionStoreDevMemory || c.SessionIdle != 45*time.Minute || c.SessionMaxAge != 8*time.Hour || c.SessionRetention != 72*time.Hour {
		t.Fatalf("keys from the environment: %+v", c)
	}
	// A value that cannot be read is not replaced by the default.
	for _, tc := range [][2]string{
		{bff.EnvSessionIdle, "thirty minutes"}, {bff.EnvSessionIdle, "30"}, {bff.EnvSessionMaxAge, "12 hours"}, {bff.EnvSessionRetention, "1d"},
		{bff.EnvSessionMaxPerUser, "ten"}, {bff.EnvSessionMaxPerUser, "0"}, {bff.EnvSessionMaxClockSkew, "2"}, {bff.EnvSessionMissBudget, "1.5"},
	} {
		t.Setenv(bff.EnvSessionIdle, "45m")
		t.Setenv(bff.EnvSessionMaxAge, "8h")
		t.Setenv(bff.EnvSessionRetention, "72h")
		t.Setenv(bff.EnvSessionMaxPerUser, "4")
		t.Setenv(bff.EnvSessionMaxClockSkew, "30s")
		t.Setenv(bff.EnvSessionMissBudget, "12")
		t.Setenv(tc[0], tc[1])
		b, err := bff.New(bff.ConfigFromEnv(svc))
		if err == nil {
			b.Close()
			t.Errorf("%s=%q accepted", tc[0], tc[1])
		} else if !strings.Contains(err.Error(), tc[0]) {
			t.Errorf("%s=%q: error %q does not name the key", tc[0], tc[1], err)
		}
	}
}

// Unset, nothing changes: sessions are in memory and end with the token.
func TestServerSession_UnsetKeepsTheTokenBoundSessions(t *testing.T) {
	a := newApp(t, nil)
	c := a.login("dev", "operator")
	if c.MaxAge != 900 {
		t.Fatalf("token-bound cookie Max-Age %d, want the token's 900", c.MaxAge)
	}
	a.clk.Advance(16 * time.Minute)
	if code := a.get(c); code != http.StatusUnauthorized {
		t.Fatalf("a token-bound session after 16 minutes: %d, want 401", code)
	}
}

// ---- review of 2026-11-21: floods, caps, clocks, logout, plaintext ----

func unknownCookie(a *app, n int) *http.Cookie {
	raw := make([]byte, 32)
	raw[0], raw[1] = byte(n), byte(n>>8)
	return &http.Cookie{Name: a.b.CookieName(), Value: base64.RawURLEncoding.EncodeToString(raw)}
}

// M2. A flood of well-formed ids nobody holds costs the store one read
// each, until the address runs out its budget; after that such ids are
// refused unread, and the people who are logged in are still read.
func TestServerSession_AFloodOfUnknownIDsStopsReachingTheStore(t *testing.T) {
	rec := &recording{Store: sessions.NewMem().Front(svc)}
	a := newApp(t, func(c *bff.Config) {
		c.SessionStore = bff.SessionStoreWell
		c.Sessions = rec
		c.SessionMissBudget = 5
	})
	const flooder, other = "198.51.100.7:4000", "198.51.100.8:4000"
	alice := sessionCookie(t, a.do(http.MethodPost, "/auth/login", loginBody("alice", a.is.AddUser(t, "alice", "operator")), withXRW(), fromAddr(flooder)))
	if code := a.do(http.MethodGet, "/api/v1/thing", "", withCookie(alice), fromAddr(flooder)).Code; code != http.StatusOK {
		t.Fatalf("alice before the flood: %d", code)
	}
	before := rec.gets.Load()
	for i := 0; i < 200; i++ {
		if code := a.do(http.MethodGet, "/api/v1/thing", "", withCookie(unknownCookie(a, i)), fromAddr(flooder)).Code; code != http.StatusUnauthorized {
			t.Fatalf("unknown id %d: %d, want 401", i, code)
		}
	}
	if reads := rec.gets.Load() - before; reads != 5 {
		t.Fatalf("200 unknown ids from one address cost the store %d reads, want the budget of 5", reads)
	}
	// Alice shares the flooder's address (one office, one proxy). Her
	// session is one this front has seen live: it is still read, and works.
	before = rec.gets.Load()
	if code := a.do(http.MethodGet, "/api/v1/thing", "", withCookie(alice), fromAddr(flooder)).Code; code != http.StatusOK {
		t.Fatalf("alice during the flood, from the flooder's address: %d, want 200", code)
	}
	if rec.gets.Load() == before {
		t.Fatal("alice's session was accepted without asking the store: the memory of ids seen live must not stand in for it")
	}
	// A login from that address works too, and its session is used at once.
	bob := sessionCookie(t, a.do(http.MethodPost, "/auth/login", loginBody("bob", a.is.AddUser(t, "bob", "operator")), withXRW(), fromAddr(flooder)))
	if code := a.do(http.MethodGet, "/api/v1/thing", "", withCookie(bob), fromAddr(flooder)).Code; code != http.StatusOK {
		t.Fatalf("a session made during the flood: %d, want 200", code)
	}
	// Another address has its own budget.
	before = rec.gets.Load()
	a.do(http.MethodGet, "/api/v1/thing", "", withCookie(unknownCookie(a, 9000)), fromAddr(other))
	if rec.gets.Load() != before+1 {
		t.Fatal("another address was refused unread because of the flooder")
	}
	// The budget is per minute.
	a.clk.Advance(time.Minute)
	before = rec.gets.Load()
	a.do(http.MethodGet, "/api/v1/thing", "", withCookie(unknownCookie(a, 9001)), fromAddr(flooder))
	if rec.gets.Load() != before+1 {
		t.Fatal("a minute later the flooder's address is still refused unread")
	}
}

// A session this front has seen live is not waved through by that memory:
// revoke it and the next request is refused, and it is forgotten.
func TestServerSession_TheMemoryOfLiveIDsGrantsNothing(t *testing.T) {
	mem := sessions.NewMem()
	rec := &recording{Store: mem.Front(svc)}
	a := newApp(t, func(c *bff.Config) { c.SessionStore = bff.SessionStoreWell; c.Sessions = rec; c.SessionMissBudget = 1 })
	c := a.login("dev", "operator")
	if code := a.get(c); code != http.StatusOK {
		t.Fatal(code)
	}
	if n, err := mem.RevokeSubject(context.Background(), "dev", "", a.clk.Now(), "cli:tester@host"); n != 1 || err != nil {
		t.Fatal(n, err)
	}
	if code := a.get(c); code != http.StatusUnauthorized { // read, found revoked: one miss
		t.Fatalf("a revoked session the front had seen live: %d, want 401", code)
	}
	before := rec.gets.Load()
	if code := a.get(c); code != http.StatusUnauthorized {
		t.Fatalf("again: %d", code)
	}
	if rec.gets.Load() != before {
		t.Fatal("a revoked id was still treated as known after the store said it had ended")
	}
}

// M2. A client that hangs up cannot cancel the store's work: the read runs
// under its own deadline, not the request's.
func TestServerSession_AClientThatHangsUpDoesNotCancelTheStore(t *testing.T) {
	rec := &recording{Store: sessions.NewMem().Front(svc)}
	a := newApp(t, func(c *bff.Config) { c.SessionStore = bff.SessionStoreWell; c.Sessions = rec })
	c := a.login("dev", "operator")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the client is already gone
	req := httptest.NewRequest(http.MethodGet, "/api/v1/thing", nil).WithContext(ctx)
	req.AddCookie(c)
	a.h.ServeHTTP(httptest.NewRecorder(), req)
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.ctxErrs) < 2 {
		t.Fatalf("store calls seen: %d", len(rec.ctxErrs))
	}
	for i, err := range rec.ctxErrs {
		if err != nil {
			t.Fatalf("store call %d ran under a cancelled context: %v", i, err)
		}
	}
	for i, has := range rec.deadlines {
		if !has {
			t.Fatalf("store read %d ran with no deadline of its own", i)
		}
	}
}

// M3. One account cannot fill the front: past its own cap its oldest
// session is ended, the login succeeds, and other people still get in.
func TestServerSession_OneAccountCannotFillTheFront(t *testing.T) {
	mem := sessions.NewMem()
	a := serverApp(t, mem, func(c *bff.Config) { c.MaxSessions, c.SessionMaxPerUser = 5, 2 })
	pw := a.is.AddUser(t, "greedy", "viewer")
	var cookies []*http.Cookie
	for i := 0; i < 8; i++ {
		a.clk.Advance(7 * time.Second) // each login a little later (and under the login rate limit)
		rec := a.do(http.MethodPost, "/auth/login", loginBody("greedy", pw), withXRW())
		if rec.Code != http.StatusOK {
			t.Fatalf("login %d of one account at a per-user cap of 2 and a front cap of 5: %d %s", i+1, rec.Code, rec.Body)
		}
		cookies = append(cookies, sessionCookie(t, rec))
	}
	for i, c := range cookies {
		want := http.StatusUnauthorized
		if i >= len(cookies)-2 {
			want = http.StatusOK
		}
		if code := a.get(c); code != want {
			t.Fatalf("session %d of 8: %d, want %d (only the newest two live)", i+1, code, want)
		}
	}
	// Everyone else still logs in: the account holds 2 of 5.
	for _, who := range []string{"alice", "bob", "carol"} {
		a.login(who, "viewer")
	}
	ended, _ := mem.List(context.Background(), sessions.Filter{Subject: "greedy", Revoked: true})
	byLimit := 0
	for _, s := range ended {
		if s.RevokedBy == sessions.ByLimit {
			byLimit++
		}
	}
	if byLimit != 6 {
		t.Fatalf("%d of the account's sessions ended by the cap, want 6", byLimit)
	}
}

// L1. A logout the store could not be told about is not a 204: the
// browser's cookie is cleared, the answer says the session is still on
// record, and it is.
func TestServerSession_LogoutDuringAnOutageSaysTheSessionWasNotEnded(t *testing.T) {
	mem := sessions.NewMem()
	o := &outage{Store: mem.Front(svc)}
	a := newApp(t, func(c *bff.Config) { c.SessionStore = bff.SessionStoreWell; c.Sessions = o })
	c := a.login("dev", "operator")
	o.down.Store(true)
	rec := a.do(http.MethodPost, "/auth/logout", "", withCookie(c), withXRW())
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "logout_incomplete") {
		t.Fatalf("logout with the store down: %d %s, want 503 logout_incomplete", rec.Code, rec.Body)
	}
	cleared := sessionCookie(t, rec)
	if cleared.MaxAge >= 0 || cleared.Value != "" || !cleared.HttpOnly || !cleared.Secure {
		t.Fatalf("the browser's cookie must still be cleared: %+v", cleared)
	}
	o.down.Store(false)
	if code := a.get(c); code != http.StatusOK {
		t.Fatalf("the session after the failed logout: %d; the test's premise is that it is still live", code)
	}
	// With the store back, logout ends it and says so.
	rec = a.do(http.MethodPost, "/auth/logout", "", withCookie(c), withXRW())
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout with the store back: %d", rec.Code)
	}
	if code := a.get(c); code != http.StatusUnauthorized {
		t.Fatalf("after the logout that worked: %d", code)
	}
	// A logout with no session at all is still a plain 204.
	if code := a.do(http.MethodPost, "/auth/logout", "", withXRW()).Code; code != http.StatusNoContent {
		t.Fatalf("logout without a session: %d", code)
	}
}

// L2. The front's clock judges expiry, so a front whose clock is far from
// the store's starts no sessions; the browser is told only that sessions
// are unavailable, the log says clock skew.
func TestServerSession_AFrontWithAWrongClockStartsNoSessions(t *testing.T) {
	mem := sessions.NewMem()
	storeTime := time.Now()
	mem.SetClock(func() time.Time { return storeTime })
	logs := captureLog(t)
	a := serverApp(t, mem, func(c *bff.Config) { c.SessionMaxClockSkew = 30 * time.Second })
	a.login("dev", "operator") // the clocks agree: fine

	a.clk.Advance(10 * time.Minute) // the front's clock runs ahead; the store's did not move
	pw := a.is.AddUser(t, "late", "operator")
	rec := a.do(http.MethodPost, "/auth/login", loginBody("late", pw), withXRW())
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "sessions_unavailable") {
		t.Fatalf("login with the front's clock ten minutes ahead, bound 30 s: %d %s, want 503", rec.Code, rec.Body)
	}
	if strings.Contains(strings.ToLower(rec.Body.String()), "clock") || strings.Contains(strings.ToLower(rec.Body.String()), "skew") {
		t.Fatalf("the browser was told about the clock: %s", rec.Body)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == a.b.CookieName() && c.Value != "" {
			t.Fatal("a session cookie was set by a refused login")
		}
	}
	if !strings.Contains(logs.String(), "clock skew") || !strings.Contains(logs.String(), bff.EnvSessionMaxClockSkew) {
		t.Fatalf("the log does not say clock skew and name the key:\n%s", logs.String())
	}
	if left, _ := mem.List(context.Background(), sessions.Filter{Subject: "late", Revoked: true}); len(left) != 0 {
		t.Fatal("the refused login left a session on record")
	}
	storeTime = storeTime.Add(10 * time.Minute) // the clocks agree again
	if code := a.do(http.MethodPost, "/auth/login", loginBody("late", pw), withXRW()).Code; code != http.StatusOK {
		t.Fatalf("login once the clocks agree: %d", code)
	}
}

// L4. The plaintext switch beside TLS, or beside sessions in the Well,
// used to be a warning. It is a refusal.
func TestNew_RefusesThePlaintextSwitchBesideTLS(t *testing.T) {
	is := bfftest.NewIssuer(t, svc)
	t.Setenv("UNHEADED_TLS_ENABLED", "")
	b, err := bff.New(bff.Config{ServiceName: svc, IssuerURL: is.URL, DevPlaintext: true})
	if err != nil {
		t.Fatalf("the plaintext switch without TLS (a development stack): %v", err)
	}
	b.Close()

	for name, cfg := range map[string]bff.Config{
		"token-bound sessions": {ServiceName: svc, IssuerURL: is.URL, DevPlaintext: true},
		"memory sessions":      {ServiceName: svc, IssuerURL: is.URL, DevPlaintext: true, SessionStore: bff.SessionStoreDevMemory},
	} {
		t.Setenv("UNHEADED_TLS_ENABLED", "true")
		b, err := bff.New(cfg)
		if err == nil {
			b.Close()
			t.Errorf("%s: AUTH_BFF_DEV_PLAINTEXT accepted with UNHEADED_TLS_ENABLED=true", name)
		} else if !strings.Contains(err.Error(), "UNHEADED_TLS_ENABLED") || !strings.Contains(err.Error(), "AUTH_BFF_DEV_PLAINTEXT") {
			// Another refusal (a missing CA, say) is not this one.
			t.Errorf("%s: the error does not name the two keys: %v", name, err)
		}
	}
	t.Setenv("UNHEADED_TLS_ENABLED", "")
	b, err = bff.New(bff.Config{ServiceName: svc, IssuerURL: is.URL, DevPlaintext: true,
		SessionStore: bff.SessionStoreWell, Sessions: sessions.NewMem().Front(svc)})
	if err == nil {
		b.Close()
		t.Fatal("AUTH_BFF_DEV_PLAINTEXT accepted with AUTH_SESSION_STORE=well: an hours-long session cookie without Secure")
	}
	if !strings.Contains(err.Error(), "AUTH_BFF_DEV_PLAINTEXT") || !strings.Contains(err.Error(), bff.SessionStoreDevMemory) {
		t.Fatalf("the error does not say what to use instead: %v", err)
	}
}

// The limits a login passes to the store are the configured ones.
func TestServerSession_LimitsReachTheStore(t *testing.T) {
	rec := &recording{Store: sessions.NewMem().Front(svc)}
	a := newApp(t, func(c *bff.Config) {
		c.SessionStore = bff.SessionStoreWell
		c.Sessions = rec
		c.MaxSessions, c.SessionMaxPerUser, c.SessionMaxClockSkew = 50, 3, 45*time.Second
	})
	a.login("dev", "operator")
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.limits) != 1 || rec.limits[0] != (sessions.CreateLimits{MaxLive: 50, MaxPerSubject: 3, MaxSkew: 45 * time.Second}) {
		t.Fatalf("limits handed to the store: %+v", rec.limits)
	}
	def := &recording{Store: sessions.NewMem().Front(svc)}
	b := newApp(t, func(c *bff.Config) { c.SessionStore = bff.SessionStoreWell; c.Sessions = def })
	b.login("dev", "operator")
	def.mu.Lock()
	defer def.mu.Unlock()
	want := sessions.CreateLimits{MaxLive: 10000, MaxPerSubject: bff.DefaultSessionMaxPerUser, MaxSkew: bff.DefaultSessionMaxClockSkew}
	if len(def.limits) != 1 || def.limits[0] != want {
		t.Fatalf("default limits handed to the store: %+v, want %+v", def.limits, want)
	}
}

// ---- what reaches the log ----

// syncBuffer is a bytes.Buffer several goroutines may write.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// captureLog sends the process's log to a buffer for the test.
func captureLog(t *testing.T) *syncBuffer {
	t.Helper()
	buf := &syncBuffer{}
	prev := zlog.Logger
	zlog.Logger = zerolog.New(buf).Level(zerolog.DebugLevel)
	t.Cleanup(func() { zlog.Logger = prev })
	return buf
}

// tokenTap notes every access token the issuer hands the front.
type tokenTap struct {
	next   http.RoundTripper
	mu     sync.Mutex
	tokens []string
}

func (tt *tokenTap) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := tt.next.RoundTrip(r)
	if err != nil || !strings.HasSuffix(r.URL.Path, "/login") {
		return resp, err
	}
	body, rerr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if rerr != nil {
		return nil, rerr
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if json.Unmarshal(body, &out) == nil && out.AccessToken != "" {
		tt.mu.Lock()
		tt.tokens = append(tt.tokens, out.AccessToken)
		tt.mu.Unlock()
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return resp, nil
}

// Nothing a browser or an attacker could present is ever logged: not the
// session id (the cookie value), not the issuer's token, not the password.
// A login, a replaced login, a login past the per-user cap, a miss, a spent
// miss budget, a logout, a revocation seen, a store outage and a refused
// login are each driven with the log captured.
func TestServerSession_NoSecretReachesTheLog(t *testing.T) {
	logs := captureLog(t)
	mem := sessions.NewMem()
	o := &outage{Store: mem.Front(svc)}
	tap := &tokenTap{next: http.DefaultTransport}
	a := newApp(t, func(c *bff.Config) {
		c.SessionStore = bff.SessionStoreWell
		c.Sessions = o
		c.SessionMaxPerUser = 2
		c.SessionMissBudget = 3
		c.HTTPClient = &http.Client{Transport: tap, Timeout: 10 * time.Second}
	})
	var secrets []string
	login := func(user string, present *http.Cookie) *http.Cookie {
		t.Helper()
		pw := a.is.AddUser(t, user, "operator")
		secrets = append(secrets, pw)
		opts := []reqOpt{withXRW()}
		if present != nil {
			opts = append(opts, withCookie(present))
		}
		rec := a.do(http.MethodPost, "/auth/login", loginBody(user, pw), opts...)
		if rec.Code != http.StatusOK {
			t.Fatalf("login %s: %d %s", user, rec.Code, rec.Body)
		}
		c := sessionCookie(t, rec)
		secrets = append(secrets, c.Value)
		return c
	}

	first := login("dev", nil)
	a.get(first)
	second := login("dev", first) // replaces the presented session
	third := login("dev", nil)    // dev now has two
	fourth := login("dev", nil)   // past the per-user cap: the oldest is ended
	a.get(second)                 // ended by the cap: a miss
	a.get(third)
	for i := 0; i < 6; i++ { // unknown ids, past the miss budget
		u := unknownCookie(a, 100+i)
		secrets = append(secrets, u.Value)
		a.get(u)
	}
	a.do(http.MethodPost, "/auth/logout", "", withCookie(third), withXRW())
	if _, err := mem.Revoke(context.Background(), sessions.KeyHash(fourth.Value)[:16], a.clk.Now(), "cli:tester@host"); err != nil {
		t.Fatal(err)
	}
	a.get(fourth) // revoked from outside

	fifth := login("alice", nil)
	o.down.Store(true)
	a.get(fifth)                                                            // the store cannot be read
	a.clk.Advance(61 * time.Second)                                         // store errors are logged once a minute
	a.do(http.MethodPost, "/auth/logout", "", withCookie(fifth), withXRW()) // nor told of a logout
	pw := a.is.AddUser(t, "bob", "operator")
	secrets = append(secrets, pw)
	a.clk.Advance(61 * time.Second)
	a.do(http.MethodPost, "/auth/login", loginBody("bob", pw), withXRW()) // nor given a new session
	a.do(http.MethodPost, "/auth/login", loginBody("bob", "wrong-password-typed-by-bob"), withXRW())
	secrets = append(secrets, "wrong-password-typed-by-bob")
	o.down.Store(false)

	tap.mu.Lock()
	if len(tap.tokens) < 5 {
		t.Fatalf("the tap saw %d tokens: the test no longer sees what the issuer hands over", len(tap.tokens))
	}
	secrets = append(secrets, tap.tokens...)
	tap.mu.Unlock()

	out := logs.String()
	for _, want := range []string{"bff: login", "bff: logout", "ended_oldest", "budget of unknown session ids", "cannot read a session", "cannot end a session at logout", "cannot store a new session", "login refused"} {
		if !strings.Contains(out, want) {
			t.Errorf("the log lacks %q: this test no longer drives that path\n%s", want, out)
		}
	}
	for i, secret := range secrets {
		if len(secret) < 12 {
			t.Fatalf("secret %d is too short to search for", i)
		}
		if strings.Contains(out, secret) {
			t.Errorf("the log carries a session id, a token or a password (secret %d of %d)", i+1, len(secrets))
		}
		// A token's parts, and the id with a character dropped, would be as bad.
		for _, part := range strings.Split(secret, ".") {
			if len(part) >= 20 && strings.Contains(out, part) {
				t.Errorf("the log carries part of secret %d", i+1)
			}
		}
	}
	// What is logged of a session is the first 16 characters of its hash,
	// never the whole hash.
	for _, c := range []*http.Cookie{first, second, third, fourth, fifth} {
		h := sessions.KeyHash(c.Value)
		if strings.Contains(out, h) {
			t.Error("the log carries a session's whole hash")
		}
	}
	if !strings.Contains(out, sessions.KeyHash(first.Value)[:16]) {
		t.Error("the login line no longer names the session by its short id")
	}
}
