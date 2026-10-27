// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package bff_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/auth"
	"unheaded/pkg/auth/authtest"
	"unheaded/pkg/bff"
	"unheaded/pkg/bff/bfftest"
)

const svc = "dashboard-backend"

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type app struct {
	t   *testing.T
	is  *bfftest.Issuer
	b   *bff.BFF
	h   http.Handler
	clk *clock
}

func newApp(t *testing.T, mod func(*bff.Config)) *app {
	t.Helper()
	is := bfftest.NewIssuer(t, "unheaded", svc)
	clk := &clock{t: time.Now()}
	cfg := bff.Config{ServiceName: svc, IssuerURL: is.URL, Now: clk.Now}
	if mod != nil {
		mod(&cfg)
	}
	b, err := bff.New(cfg)
	if err != nil {
		t.Fatalf("bff.New: %v", err)
	}
	t.Cleanup(b.Close)
	mux := http.NewServeMux()
	mux.Handle("/auth/", b.Handler())
	mux.HandleFunc("/api/v1/thing", func(w http.ResponseWriter, r *http.Request) {
		id := auth.IdentityFromContext(r.Context())
		if id != nil {
			w.Header().Set("X-Sub", id.Subject)
			w.Header().Set("X-Method", id.Extra["auth_method"])
		}
		w.WriteHeader(http.StatusOK)
	})
	chain, err := auth.NewServiceChain(auth.ServiceAuthConfig{Enabled: true, ServiceName: svc},
		auth.ServiceRoutes{OpenPaths: []string{"/auth"}, Authenticators: []auth.Authenticator{b.Authenticator()}})
	if err != nil {
		t.Fatalf("NewServiceChain: %v", err)
	}
	return &app{t: t, is: is, b: b, h: chain(mux), clk: clk}
}

type reqOpt func(*http.Request)

func withCookie(c *http.Cookie) reqOpt { return func(r *http.Request) { r.AddCookie(c) } }
func withXRW() reqOpt {
	return func(r *http.Request) { r.Header.Set(bff.HeaderRequestedWith, bff.RequestedWithValue) }
}
func fromAddr(a string) reqOpt { return func(r *http.Request) { r.RemoteAddr = a } }

func (a *app) do(method, target, body string, opts ...reqOpt) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, o := range opts {
		o(req)
	}
	rec := httptest.NewRecorder()
	a.h.ServeHTTP(rec, req)
	return rec
}

func loginBody(user, pw string) string {
	b, _ := json.Marshal(map[string]string{"username": user, "password": pw})
	return string(b)
}

func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == bff.DefaultCookieName {
			return c
		}
	}
	t.Fatalf("no %s cookie in %v", bff.DefaultCookieName, rec.Header().Values("Set-Cookie"))
	return nil
}

func (a *app) login(user string, roles ...string) *http.Cookie {
	a.t.Helper()
	pw := a.is.AddUser(a.t, user, roles...)
	rec := a.do(http.MethodPost, "/auth/login", loginBody(user, pw), withXRW())
	if rec.Code != http.StatusOK {
		a.t.Fatalf("login %s: %d %s", user, rec.Code, rec.Body)
	}
	return sessionCookie(a.t, rec)
}

func TestLogin_SetsHardenedCookieAndKeepsTokenServerSide(t *testing.T) {
	a := newApp(t, nil)
	pw := a.is.AddUser(t, "dev", "operator")
	rec := a.do(http.MethodPost, "/auth/login", loginBody("dev", pw), withXRW())
	if rec.Code != http.StatusOK {
		t.Fatalf("login: %d %s", rec.Code, rec.Body)
	}
	c := sessionCookie(t, rec)
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode || c.Path != "/" {
		t.Fatalf("cookie attributes: HttpOnly=%v Secure=%v SameSite=%v Path=%q", c.HttpOnly, c.Secure, c.SameSite, c.Path)
	}
	raw, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil || len(raw) != 32 {
		t.Fatalf("session id is not 32 random bytes: %q", c.Value)
	}
	if c.MaxAge <= 0 || c.MaxAge > 15*60 {
		t.Fatalf("cookie Max-Age %d, want the token's remaining life", c.MaxAge)
	}
	body := rec.Body.String()
	if strings.Contains(body, "access_token") || strings.Contains(body, "eyJ") {
		t.Fatalf("login response carries the token: %s", body)
	}
	var me struct {
		Sub   string   `json:"sub"`
		Roles []string `json:"roles"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil || me.Sub != "dev" || len(me.Roles) != 1 || me.Roles[0] != "operator" {
		t.Fatalf("login body %s", body)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control %q, want no-store", got)
	}
}

func TestLogin_DevPlaintextDropsSecureOnly(t *testing.T) {
	a := newApp(t, func(c *bff.Config) { c.DevPlaintext = true })
	c := a.login("dev", "operator")
	if c.Secure {
		t.Fatal("Secure set under DevPlaintext")
	}
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode {
		t.Fatal("DevPlaintext must only drop Secure")
	}
}

func TestLogin_Refusals(t *testing.T) {
	a := newApp(t, nil)
	pw := a.is.AddUser(t, "dev", "operator")
	cases := []struct {
		name   string
		method string
		body   string
		opts   []reqOpt
		want   int
	}{
		{"no X-Requested-With", http.MethodPost, loginBody("dev", pw), nil, http.StatusForbidden},
		{"wrong X-Requested-With", http.MethodPost, loginBody("dev", pw), []reqOpt{func(r *http.Request) { r.Header.Set(bff.HeaderRequestedWith, "XMLHttpRequest") }}, http.StatusForbidden},
		{"GET", http.MethodGet, "", []reqOpt{withXRW()}, http.StatusMethodNotAllowed},
		{"wrong password", http.MethodPost, loginBody("dev", pw+"x"), []reqOpt{withXRW()}, http.StatusUnauthorized},
		{"unknown user", http.MethodPost, loginBody("nobody", pw), []reqOpt{withXRW()}, http.StatusUnauthorized},
		{"empty password", http.MethodPost, loginBody("dev", ""), []reqOpt{withXRW()}, http.StatusBadRequest},
		{"unknown field", http.MethodPost, `{"username":"dev","password":"x","audience":"sword"}`, []reqOpt{withXRW()}, http.StatusBadRequest},
		{"oversized", http.MethodPost, `{"username":"dev","password":"` + strings.Repeat("a", 8192) + `"}`, []reqOpt{withXRW()}, http.StatusRequestEntityTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := a.do(tc.method, "/auth/login", tc.body, tc.opts...)
			if rec.Code != tc.want {
				t.Fatalf("got %d, want %d (%s)", rec.Code, tc.want, rec.Body)
			}
			if len(rec.Result().Cookies()) != 0 {
				t.Fatal("a refused login set a cookie")
			}
		})
	}
	before := a.is.Logins.Load()
	a.do(http.MethodPost, "/auth/login", loginBody("dev", pw))
	if a.is.Logins.Load() != before {
		t.Fatal("a login without X-Requested-With reached the issuer")
	}
	// Content type other than JSON.
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(loginBody("dev", pw)))
	req.Header.Set("Content-Type", "text/plain")
	withXRW()(req)
	rec := httptest.NewRecorder()
	a.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("text/plain login: %d, want 415", rec.Code)
	}
}

func TestLogin_IssuerStatuses(t *testing.T) {
	a := newApp(t, nil)
	pw := a.is.AddUser(t, "dev", "operator")
	for st, want := range map[int]int{
		http.StatusTooManyRequests:     http.StatusTooManyRequests,
		http.StatusInternalServerError: http.StatusBadGateway,
		http.StatusServiceUnavailable:  http.StatusBadGateway,
	} {
		a.is.ForceStatus.Store(int64(st))
		rec := a.do(http.MethodPost, "/auth/login", loginBody("dev", pw), withXRW(), fromAddr("192.0.2.1:1"))
		if rec.Code != want {
			t.Errorf("issuer %d: got %d, want %d", st, rec.Code, want)
		}
	}
}

func TestLogin_IssuerDownIs502(t *testing.T) {
	is := bfftest.NewIssuer(t, svc)
	b, err := bff.New(bff.Config{ServiceName: svc, IssuerURL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	pw := is.AddUser(t, "dev", "operator")
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(loginBody("dev", pw)))
	req.Header.Set("Content-Type", "application/json")
	withXRW()(req)
	rec := httptest.NewRecorder()
	b.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("issuer down: %d, want 502", rec.Code)
	}
}

func TestLogin_RateLimitedPerClient(t *testing.T) {
	a := newApp(t, nil)
	pw := a.is.AddUser(t, "dev", "operator")
	var last int
	for i := 0; i < bff.LoginAttemptsPerMinute+1; i++ {
		last = a.do(http.MethodPost, "/auth/login", loginBody("dev", pw+"x"), withXRW(), fromAddr("192.0.2.7:4000")).Code
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("attempt %d from one client: %d, want 429", bff.LoginAttemptsPerMinute+1, last)
	}
	// Another client is unaffected; the window resets.
	if c := a.do(http.MethodPost, "/auth/login", loginBody("dev", pw), withXRW(), fromAddr("192.0.2.8:4000")).Code; c != http.StatusOK {
		t.Fatalf("other client: %d", c)
	}
	a.clk.Advance(61 * time.Second)
	if c := a.do(http.MethodPost, "/auth/login", loginBody("dev", pw), withXRW(), fromAddr("192.0.2.7:4000")).Code; c != http.StatusOK {
		t.Fatalf("after the window: %d", c)
	}
}

func TestMeAndLogout(t *testing.T) {
	a := newApp(t, nil)
	if c := a.do(http.MethodGet, "/auth/me", "").Code; c != http.StatusUnauthorized {
		t.Fatalf("/auth/me without a session: %d", c)
	}
	c := a.login("dev", "operator", "viewer")
	rec := a.do(http.MethodGet, "/auth/me", "", withCookie(c))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"sub":"dev"`) || !strings.Contains(rec.Body.String(), "operator") {
		t.Fatalf("/auth/me: %d %s", rec.Code, rec.Body)
	}
	if c := a.do(http.MethodPost, "/auth/logout", "", withCookie(c)).Code; c != http.StatusForbidden {
		t.Fatalf("logout without X-Requested-With: %d, want 403", c)
	}
	rec = a.do(http.MethodPost, "/auth/logout", "", withCookie(c), withXRW())
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout: %d", rec.Code)
	}
	cleared := sessionCookie(t, rec)
	if cleared.MaxAge >= 0 || cleared.Value != "" {
		t.Fatalf("logout did not clear the cookie: %+v", cleared)
	}
	if code := a.do(http.MethodGet, "/auth/me", "", withCookie(c)).Code; code != http.StatusUnauthorized {
		t.Fatalf("/auth/me after logout: %d", code)
	}
	if code := a.do(http.MethodGet, "/api/v1/thing", "", withCookie(c)).Code; code != http.StatusUnauthorized {
		t.Fatalf("API with a logged-out cookie: %d", code)
	}
}

func TestSessionAuthorizesWritesByRole(t *testing.T) {
	a := newApp(t, nil)
	if c := a.do(http.MethodPost, "/api/v1/thing", "{}", withXRW()).Code; c != http.StatusUnauthorized {
		t.Fatalf("write without a session: %d, want 401", c)
	}
	op := a.login("dev", "operator")
	if c := a.do(http.MethodPost, "/api/v1/thing", "{}", withCookie(op)).Code; c != http.StatusForbidden {
		t.Fatalf("cookie write without X-Requested-With (CSRF): %d, want 403", c)
	}
	rec := a.do(http.MethodPost, "/api/v1/thing", "{}", withCookie(op), withXRW())
	if rec.Code != http.StatusOK || rec.Header().Get("X-Sub") != "dev" || rec.Header().Get("X-Method") != "session" {
		t.Fatalf("operator session write: %d sub %q method %q", rec.Code, rec.Header().Get("X-Sub"), rec.Header().Get("X-Method"))
	}
	viewer := a.login("guest1", "viewer")
	if c := a.do(http.MethodPost, "/api/v1/thing", "{}", withCookie(viewer), withXRW()).Code; c != http.StatusForbidden {
		t.Fatalf("viewer session write: %d, want 403", c)
	}
	if c := a.do(http.MethodGet, "/api/v1/thing", "", withCookie(viewer)).Code; c != http.StatusOK {
		t.Fatalf("viewer session read: %d, want 200", c)
	}
	bogus := &http.Cookie{Name: bff.DefaultCookieName, Value: "not-a-session"}
	if c := a.do(http.MethodGet, "/api/v1/thing", "", withCookie(bogus)).Code; c != http.StatusUnauthorized {
		t.Fatalf("made-up cookie: %d, want 401", c)
	}
	forged := &http.Cookie{Name: bff.DefaultCookieName, Value: base64.RawURLEncoding.EncodeToString(make([]byte, 32))}
	if c := a.do(http.MethodGet, "/api/v1/thing", "", withCookie(forged)).Code; c != http.StatusUnauthorized {
		t.Fatalf("well-formed unknown cookie: %d, want 401", c)
	}
}

func TestLoginReplacesTheOldSession(t *testing.T) {
	a := newApp(t, nil)
	old := a.login("dev", "operator")
	pw := a.is.AddUser(t, "dev", "operator")
	rec := a.do(http.MethodPost, "/auth/login", loginBody("dev", pw), withXRW(), withCookie(old))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Code)
	}
	if sessionCookie(t, rec).Value == old.Value {
		t.Fatal("login reused the presented session id (fixation)")
	}
	if c := a.do(http.MethodGet, "/auth/me", "", withCookie(old)).Code; c != http.StatusUnauthorized {
		t.Fatalf("the replaced session still works: %d", c)
	}
}

func TestSessionsExpireWithTheTokenAndAreEvicted(t *testing.T) {
	var store *bff.MemStore
	a := newApp(t, func(c *bff.Config) {
		store = bff.NewMemStore(0, c.Now)
		c.Store = store
	})
	c := a.login("dev", "operator")
	if store.Len() != 1 {
		t.Fatalf("store holds %d sessions", store.Len())
	}
	a.clk.Advance(16 * time.Minute)
	if code := a.do(http.MethodGet, "/api/v1/thing", "", withCookie(c)).Code; code != http.StatusUnauthorized {
		t.Fatalf("expired session: %d, want 401", code)
	}
	a.login("dev2", "operator")
	a.clk.Advance(16 * time.Minute)
	if n := a.b.Evict(context.Background()); n != 1 || store.Len() != 0 {
		t.Fatalf("evicted %d, %d left", n, store.Len())
	}
}

func TestSessionEndsWhenTheIssuerDropsItsKey(t *testing.T) {
	a := newApp(t, nil)
	c := a.login("dev", "operator")
	if code := a.do(http.MethodGet, "/api/v1/thing", "", withCookie(c)).Code; code != http.StatusOK {
		t.Fatalf("fresh session: %d", code)
	}
	a.is.RotateKey(t)
	// The verifier refetches keys at its TTL; a fresh BFF shows the
	// outcome without waiting: the stored token no longer verifies.
	b2, err := bff.New(bff.Config{ServiceName: svc, IssuerURL: a.is.URL, Store: a.b.Store()})
	if err != nil {
		t.Fatal(err)
	}
	defer b2.Close()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/thing", nil)
	req.AddCookie(c)
	if _, err := b2.Authenticator().Authenticate(context.Background(), req); err == nil {
		t.Fatal("a session whose token the issuer no longer vouches for still authenticates")
	}
}

func TestStoreFullRefusesLogin(t *testing.T) {
	a := newApp(t, func(c *bff.Config) { c.Store = bff.NewMemStore(1, c.Now) })
	a.login("dev", "operator")
	pw := a.is.AddUser(t, "dev2", "operator")
	if code := a.do(http.MethodPost, "/auth/login", loginBody("dev2", pw), withXRW()).Code; code != http.StatusServiceUnavailable {
		t.Fatalf("login with a full store: %d, want 503", code)
	}
}

func TestNew_Config(t *testing.T) {
	if _, err := bff.New(bff.Config{ServiceName: svc, IssuerURL: "http://auth:19020"}); err == nil {
		t.Fatal("plain http to a non-loopback issuer accepted without DevPlaintext")
	}
	b, err := bff.New(bff.Config{ServiceName: svc, IssuerURL: "http://auth:19020", DevPlaintext: true})
	if err != nil {
		t.Fatalf("DevPlaintext: %v", err)
	}
	b.Close()
	if _, err := bff.New(bff.Config{IssuerURL: "https://auth.example"}); err == nil {
		t.Fatal("no service name accepted")
	}
	if _, err := bff.New(bff.Config{ServiceName: svc}); err == nil {
		t.Fatal("no issuer accepted")
	}
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("AUTH_ISSUER_URL", "http://auth:19020")
	t.Setenv("AUTH_BFF_DEV_PLAINTEXT", "true")
	c := bff.ConfigFromEnv("kanban-app")
	if c.ServiceName != "kanban-app" || c.IssuerURL != "http://auth:19020" || !c.DevPlaintext || !c.Enabled() {
		t.Fatalf("%+v", c)
	}
	t.Setenv("AUTH_ISSUER_URL", "")
	if bff.ConfigFromEnv("kanban-app").Enabled() {
		t.Fatal("enabled without an issuer")
	}
}

func TestAuthJSServed(t *testing.T) {
	a := newApp(t, nil)
	rec := a.do(http.MethodGet, "/auth/auth.js", "")
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/javascript") {
		t.Fatalf("auth.js: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	js := rec.Body.String()
	for _, banned := range []string{"access_token", "localStorage", "sessionStorage", "document.cookie"} {
		if strings.Contains(js, banned) {
			t.Errorf("auth.js mentions %s: no token or cookie may touch JS", banned)
		}
	}
	if !strings.Contains(js, bff.RequestedWithValue) {
		t.Error("auth.js does not send X-Requested-With")
	}
}

func TestAuthorizeOutgoing(t *testing.T) {
	f := authtest.New(t, "sword", svc)
	m := f.Minter(svc)
	td, err := auth.NewTrustDir(f.Trust)
	if err != nil {
		t.Fatal(err)
	}
	v, err := auth.NewEdDSAVerifier(auth.EdDSAVerifierConfig{Trust: td, Issuer: auth.EdDSAServiceIssuer, Audience: "sword", AllowCallers: []string{svc}})
	if err != nil {
		t.Fatal(err)
	}
	a := newApp(t, nil)
	c := a.login("dev", "operator")
	var got *http.Request
	a2 := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = r })
	// Run through the chain so the session identity is in the context.
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", a2)
	chain, _ := auth.NewServiceChain(auth.ServiceAuthConfig{Enabled: true, ServiceName: svc},
		auth.ServiceRoutes{Authenticators: []auth.Authenticator{a.b.Authenticator()}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/x", nil)
	req.AddCookie(c)
	withXRW()(req)
	chain(mux).ServeHTTP(httptest.NewRecorder(), req)
	if got == nil {
		t.Fatal("request did not reach the handler")
	}
	out := httptest.NewRequest(http.MethodPost, "http://sword:19011/api/v1/deploy", nil)
	if err := bff.AuthorizeOutgoing(out, got, m, "sword"); err != nil {
		t.Fatal(err)
	}
	if out.Header.Get(bff.HeaderOnBehalfOf) != "dev" {
		t.Fatalf("%s = %q", bff.HeaderOnBehalfOf, out.Header.Get(bff.HeaderOnBehalfOf))
	}
	id, err := v.Authenticate(context.Background(), out)
	if err != nil || id.Subject != svc || id.Extra["act"] != "dev" {
		t.Fatalf("outgoing token: %v %+v", err, id)
	}
	// No user: a plain service token, no on-behalf-of header.
	plain := httptest.NewRequest(http.MethodPost, "http://sword:19011/api/v1/deploy", nil)
	if err := bff.AuthorizeOutgoing(plain, httptest.NewRequest(http.MethodGet, "/", nil), m, "sword"); err != nil {
		t.Fatal(err)
	}
	if plain.Header.Get(bff.HeaderOnBehalfOf) != "" {
		t.Fatal("on-behalf-of set with no user")
	}
	if id, err := v.Authenticate(context.Background(), plain); err != nil || id.Extra["act"] != "" {
		t.Fatalf("plain outgoing token: %v %+v", err, id)
	}
}
