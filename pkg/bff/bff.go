// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

// Package bff is the backend-for-frontend login a browser-facing service
// (dashboard-backend, kanban-app) puts in front of its API (ADR-103 §4,
// S-IRON-GATE Phase 6).
//
// The browser never holds a bearer token. POST /auth/login sends the
// username and password to the OIDC issuer (unheaded-auth, AUTH_ISSUER_URL)
// asking for a token whose audience is this service, verifies that token
// against the issuer's JWKS, keeps it server-side, and gives the browser
// only a random session id in a cookie: HttpOnly; Secure; SameSite=Strict;
// Path=/. An XSS bug can then use the session while the page is open but
// cannot carry a token away.
//
// Every request presenting the cookie is authenticated by re-verifying the
// stored token (cached JWKS), so a session ends with its token, and when
// the issuer stops publishing the key that signed it.
//
// CSRF: SameSite=Strict keeps the cookie off cross-site requests in current
// browsers; as a second, independent guard every state-changing request
// authenticated by the cookie, and login and logout themselves, must carry
// X-Requested-With: unheaded. A cross-site form cannot set a custom header,
// and a cross-site fetch that sets one needs a CORS preflight these
// services never grant.
package bff

import (
	"bytes"
	"context"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"unheaded/pkg/auth"
)

const (
	// DefaultCookieName is the session cookie.
	DefaultCookieName = "unheaded_session"
	// HeaderRequestedWith must be RequestedWithValue on login, logout and
	// every state-changing request a session cookie authenticates.
	HeaderRequestedWith = "X-Requested-With"
	// RequestedWithValue is the value auth.js sends.
	RequestedWithValue = "unheaded"
	// HeaderOnBehalfOf names the user on an outgoing service call
	// (AuthorizeOutgoing); the signed "act" claim is what counts.
	HeaderOnBehalfOf = "X-Unheaded-On-Behalf-Of"

	// LoginAttemptsPerMinute bounds logins per client address and
	// username, in front of the issuer's own lockout (which sees only this
	// backend's address).
	LoginAttemptsPerMinute = 10
	// ClientLoginAttemptsPerMinute bounds logins per client address.
	ClientLoginAttemptsPerMinute = 60

	maxLoginBody     = 4 << 10
	maxIssuerReply   = 16 << 10
	issuerTimeout    = 15 * time.Second // argon2id is slow on purpose
	evictionInterval = time.Minute
	sessionIDBytes   = 32
	maxLimiterKeys   = 10000
)

//go:embed static/auth.js
var staticFS embed.FS

// Config configures a BFF.
type Config struct {
	// ServiceName is this service's identity: the audience it asks the
	// issuer for and verifies (unheaded-auth -audiences must list it).
	ServiceName string
	// IssuerURL is the OIDC issuer (AUTH_ISSUER_URL). Empty: no BFF.
	IssuerURL string
	// CookieName defaults to DefaultCookieName.
	CookieName string
	// DevPlaintext (AUTH_BFF_DEV_PLAINTEXT=true) is for the compose stack
	// before internal TLS: the cookie drops Secure, and the issuer may be
	// plain http on any host. Nothing else changes.
	DevPlaintext bool
	// MaxSessions caps the default in-memory store (<= 0: 10000).
	MaxSessions int
	// Store defaults to an in-memory store.
	Store Store
	// HTTPClient talks to the issuer (default: 15 s timeout).
	HTTPClient *http.Client
	// Now defaults to time.Now.
	Now func() time.Time
}

// ConfigFromEnv reads AUTH_ISSUER_URL and AUTH_BFF_DEV_PLAINTEXT.
func ConfigFromEnv(serviceName string) Config {
	return Config{
		ServiceName:  serviceName,
		IssuerURL:    strings.TrimSpace(os.Getenv("AUTH_ISSUER_URL")),
		DevPlaintext: os.Getenv("AUTH_BFF_DEV_PLAINTEXT") == "true",
	}
}

// Enabled reports whether an issuer is configured.
func (c Config) Enabled() bool { return c.IssuerURL != "" }

// BFF holds the login routes, the session store and the cookie
// authenticator. Safe for concurrent use.
type BFF struct {
	cfg      Config
	issuer   string
	verifier *auth.JWKSVerifier
	client   *http.Client
	store    Store
	now      func() time.Time

	limitMu sync.Mutex
	limits  map[string]*window

	stop     chan struct{}
	stopOnce sync.Once
}

type window struct {
	start time.Time
	n     int
}

// New builds a BFF and starts its eviction loop (Close stops it).
func New(cfg Config) (*BFF, error) {
	if !auth.ValidKeyName(cfg.ServiceName) {
		return nil, fmt.Errorf("bff: service name %q is not an identity name", cfg.ServiceName)
	}
	if cfg.IssuerURL == "" {
		return nil, errors.New("bff: no issuer URL (AUTH_ISSUER_URL)")
	}
	if cfg.CookieName == "" {
		cfg.CookieName = DefaultCookieName
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	var opts []auth.JWKSOption
	if cfg.DevPlaintext {
		opts = append(opts, auth.WithPlainHTTPIssuer())
	}
	v, err := auth.NewJWKSVerifier(cfg.IssuerURL, cfg.ServiceName, cfg.HTTPClient, 0, opts...)
	if err != nil {
		return nil, fmt.Errorf("bff: %w", err)
	}
	client := &http.Client{Timeout: issuerTimeout}
	if cfg.HTTPClient != nil {
		cp := *cfg.HTTPClient
		client = &cp
	}
	// The login carries a password: never follow a redirect with it.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	store := cfg.Store
	if store == nil {
		store = NewMemStore(cfg.MaxSessions, cfg.Now)
	}
	if cfg.DevPlaintext {
		log.Warn().Str("service", cfg.ServiceName).Str("issuer", cfg.IssuerURL).
			Msg("bff: AUTH_BFF_DEV_PLAINTEXT: session cookie without Secure, issuer over plain http (compose before TLS only)")
	}
	b := &BFF{
		cfg:      cfg,
		issuer:   strings.TrimRight(cfg.IssuerURL, "/"),
		verifier: v,
		client:   client,
		store:    store,
		now:      cfg.Now,
		limits:   make(map[string]*window),
		stop:     make(chan struct{}),
	}
	go b.evictLoop()
	return b, nil
}

// Close stops the eviction loop.
func (b *BFF) Close() { b.stopOnce.Do(func() { close(b.stop) }) }

// Store is the session store in use.
func (b *BFF) Store() Store { return b.store }

func (b *BFF) evictLoop() {
	t := time.NewTicker(evictionInterval)
	defer t.Stop()
	for {
		select {
		case <-b.stop:
			return
		case <-t.C:
			b.Evict(context.Background())
		}
	}
}

// Evict drops expired sessions and stale rate-limit windows now, returning
// how many sessions went.
func (b *BFF) Evict(ctx context.Context) int {
	n, err := b.store.Evict(ctx, b.now())
	if err != nil {
		log.Warn().Err(err).Str("service", b.cfg.ServiceName).Msg("bff: session eviction failed")
	}
	b.limitMu.Lock()
	now := b.now()
	for k, w := range b.limits {
		if now.Sub(w.start) >= time.Minute {
			delete(b.limits, k)
		}
	}
	b.limitMu.Unlock()
	return n
}

// Handler serves /auth/login (POST), /auth/me (GET), /auth/logout (POST)
// and /auth/auth.js. Mount it at "/auth/" outside the auth chain: each
// route checks what it needs itself.
func (b *BFF) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/login", b.login)
	mux.HandleFunc("/auth/me", b.me)
	mux.HandleFunc("/auth/logout", b.logout)
	mux.HandleFunc("/auth/auth.js", ServeAuthJS)
	return mux
}

// ServeAuthJS serves auth.js, the login panel and fetch wrapper. A
// service mounts it at /auth/auth.js even with auth off, so its pages can
// load it unconditionally.
func ServeAuthJS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	js, err := staticFS.ReadFile("static/auth.js")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(js) // #nosec G104 -- status committed; a failed write means the client left
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func requestedWith(r *http.Request) bool {
	return r.Header.Get(HeaderRequestedWith) == RequestedWithValue
}

// clientAddr is the peer address, never a forwarded header a client could
// set to dodge the limit.
func clientAddr(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// allowLogin counts one attempt for the client and for client+username.
func (b *BFF) allowLogin(client, user string) bool {
	b.limitMu.Lock()
	defer b.limitMu.Unlock()
	now := b.now()
	if len(b.limits) >= maxLimiterKeys {
		for k, w := range b.limits {
			if now.Sub(w.start) >= time.Minute {
				delete(b.limits, k)
			}
		}
		if len(b.limits) >= maxLimiterKeys {
			return false
		}
	}
	hit := func(key string, max int) bool {
		w, ok := b.limits[key]
		if !ok || now.Sub(w.start) >= time.Minute {
			w = &window{start: now}
			b.limits[key] = w
		}
		w.n++
		return w.n <= max
	}
	okClient := hit("c|"+client, ClientLoginAttemptsPerMinute)
	okUser := hit("u|"+client+"|"+user, LoginAttemptsPerMinute)
	return okClient && okUser
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"` // #nosec G117 -- request field, forwarded to the issuer, never logged or stored
}

func decodeLogin(w http.ResponseWriter, r *http.Request) (loginRequest, int) {
	var req loginRequest
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "application/json" {
		return req, http.StatusUnsupportedMediaType
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxLoginBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return req, http.StatusRequestEntityTooLarge
		}
		return req, http.StatusBadRequest
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return req, http.StatusBadRequest
	}
	if req.Username == "" || req.Password == "" {
		return req, http.StatusBadRequest
	}
	return req, 0
}

func (b *BFF) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if !requestedWith(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "missing " + HeaderRequestedWith + ": " + RequestedWithValue})
		return
	}
	req, status := decodeLogin(w, r)
	if status != 0 {
		writeJSON(w, status, map[string]string{"error": "invalid_request"})
		return
	}
	client := clientAddr(r)
	if !b.allowLogin(client, req.Username) {
		w.Header().Set("Retry-After", "60")
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too_many_attempts"})
		return
	}

	tok, status, retry := b.issuerLogin(r.Context(), req)
	if status != http.StatusOK {
		if retry != "" {
			w.Header().Set("Retry-After", retry)
		}
		msg := map[int]string{
			http.StatusUnauthorized:    "invalid_credentials",
			http.StatusTooManyRequests: "temporarily_locked",
			http.StatusBadRequest:      "invalid_request",
		}[status]
		if msg == "" {
			msg = "issuer_unavailable"
		}
		log.Warn().Str("service", b.cfg.ServiceName).Str("source", client).Int("status", status).Msg("bff: login refused")
		writeJSON(w, status, map[string]string{"error": msg})
		return
	}

	id, err := b.verify(r.Context(), tok)
	if err != nil {
		log.Error().Err(err).Str("service", b.cfg.ServiceName).Msg("bff: the issuer's token did not verify")
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "issuer_token_invalid"})
		return
	}
	life, err := tokenLife(tok, b.now())
	if err != nil || life <= 0 {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "issuer_token_invalid"})
		return
	}

	// A login always gets a fresh id; a presented one is dropped, so a
	// planted cookie cannot become someone's session (fixation).
	if c, err := r.Cookie(b.cfg.CookieName); err == nil && c.Value != "" {
		_ = b.store.Delete(r.Context(), storeKey(c.Value))
	}
	sid, err := newSessionID()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server_error"})
		return
	}
	expires := b.now().Add(life)
	if err := b.store.Put(r.Context(), storeKey(sid), Session{Token: tok, Subject: id.Subject, Roles: id.Roles, Expires: expires}); err != nil {
		log.Error().Err(err).Str("service", b.cfg.ServiceName).Msg("bff: cannot store session")
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sessions_unavailable"})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     b.cfg.CookieName,
		Value:    sid,
		Path:     "/",
		MaxAge:   int(life / time.Second),
		HttpOnly: true,
		Secure:   !b.cfg.DevPlaintext,
		SameSite: http.SameSiteStrictMode,
	})
	log.Info().Str("service", b.cfg.ServiceName).Str("user", id.Subject).Strs("roles", id.Roles).Str("source", client).Msg("bff: login")
	writeJSON(w, http.StatusOK, meBody(id.Subject, id.Roles, expires))
}

func meBody(sub string, roles []string, exp time.Time) map[string]any {
	if roles == nil {
		roles = []string{}
	}
	return map[string]any{"sub": sub, "roles": roles, "expires_at": exp.UTC().Format(time.RFC3339)}
}

// issuerLogin posts the credentials to the issuer and returns the access
// token, or the status to answer with (401, 429, 400, else 502) and any
// Retry-After.
func (b *BFF) issuerLogin(ctx context.Context, req loginRequest) (string, int, string) {
	body, err := json.Marshal(map[string]string{"username": req.Username, "password": req.Password, "audience": b.cfg.ServiceName})
	if err != nil {
		return "", http.StatusBadGateway, ""
	}
	ctx, cancel := context.WithTimeout(ctx, issuerTimeout)
	defer cancel()
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, b.issuer+"/login", bytes.NewReader(body))
	if err != nil {
		return "", http.StatusBadGateway, ""
	}
	hr.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(hr) // #nosec G107,G704 -- issuer URL from configuration, scheme/host checked at New
	if err != nil {
		log.Warn().Err(err).Str("service", b.cfg.ServiceName).Str("issuer", b.issuer).Msg("bff: issuer unreachable")
		return "", http.StatusBadGateway, ""
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusBadRequest:
		return "", resp.StatusCode, ""
	case http.StatusTooManyRequests:
		return "", resp.StatusCode, resp.Header.Get("Retry-After")
	default:
		return "", http.StatusBadGateway, ""
	}
	var out struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxIssuerReply)).Decode(&out); err != nil ||
		out.AccessToken == "" || !strings.EqualFold(out.TokenType, "bearer") {
		return "", http.StatusBadGateway, ""
	}
	return out.AccessToken, http.StatusOK, ""
}

// verify checks tok against the issuer's JWKS for this service's audience.
func (b *BFF) verify(ctx context.Context, tok string) (*auth.Identity, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	return b.verifier.Authenticate(ctx, req)
}

// tokenLife is exp - iat of an already verified token (exp - now without
// iat): the session's life on this host's clock, whatever the issuer's.
func tokenLife(tok string, now time.Time) (time.Duration, error) {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return 0, errors.New("not a compact JWS")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, err
	}
	var c struct {
		Exp int64 `json:"exp"`
		Iat int64 `json:"iat"`
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return 0, err
	}
	if c.Iat > 0 && c.Exp > c.Iat {
		return time.Duration(c.Exp-c.Iat) * time.Second, nil
	}
	return time.Unix(c.Exp, 0).Sub(now), nil
}

func newSessionID() (string, error) {
	b := make([]byte, sessionIDBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// validSessionID is the shape newSessionID makes; anything else is not
// looked up.
func validSessionID(s string) bool {
	if len(s) != base64.RawURLEncoding.EncodedLen(sessionIDBytes) {
		return false
	}
	_, err := base64.RawURLEncoding.Strict().DecodeString(s)
	return err == nil
}

// session returns the live session the request's cookie names.
func (b *BFF) session(r *http.Request) (string, Session, bool) {
	c, err := r.Cookie(b.cfg.CookieName)
	if err != nil || !validSessionID(c.Value) {
		return "", Session{}, false
	}
	key := storeKey(c.Value)
	s, ok, err := b.store.Get(r.Context(), key)
	if err != nil || !ok {
		return "", Session{}, false
	}
	if !b.now().Before(s.Expires) {
		_ = b.store.Delete(r.Context(), key)
		return "", Session{}, false
	}
	return key, s, true
}

func (b *BFF) me(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	id, s, err := b.authenticate(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "no_session"})
		return
	}
	writeJSON(w, http.StatusOK, meBody(id.Subject, id.Roles, s.Expires))
}

func (b *BFF) logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if !requestedWith(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "missing " + HeaderRequestedWith + ": " + RequestedWithValue})
		return
	}
	if key, s, ok := b.session(r); ok {
		_ = b.store.Delete(r.Context(), key)
		log.Info().Str("service", b.cfg.ServiceName).Str("user", s.Subject).Msg("bff: logout")
	}
	http.SetCookie(w, &http.Cookie{
		Name: b.cfg.CookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: !b.cfg.DevPlaintext, SameSite: http.SameSiteStrictMode,
	})
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

// authenticate resolves the cookie to a session and re-verifies its token.
// A token that no longer verifies ends the session.
func (b *BFF) authenticate(r *http.Request) (*auth.Identity, Session, error) {
	key, s, ok := b.session(r)
	if !ok {
		return nil, Session{}, fmt.Errorf("%w: no session", auth.ErrUnauthenticated)
	}
	id, err := b.verify(r.Context(), s.Token)
	if err != nil {
		_ = b.store.Delete(r.Context(), key)
		return nil, Session{}, fmt.Errorf("%w: session token: %v", auth.ErrUnauthenticated, err)
	}
	if id.Extra == nil {
		id.Extra = map[string]string{}
	}
	id.Extra["auth_method"] = "session"
	return id, s, nil
}

// Authenticator is the session cookie as an auth.Authenticator, for
// auth.ServiceRoutes.Authenticators. Without the cookie it answers
// ErrUnauthenticated so other credentials are tried; a state-changing
// request authenticated by the cookie without X-Requested-With is
// ErrForbidden (CSRF).
func (b *BFF) Authenticator() auth.Authenticator { return sessionAuthenticator{b} }

type sessionAuthenticator struct{ b *BFF }

func (a sessionAuthenticator) Authenticate(_ context.Context, r *http.Request) (*auth.Identity, error) {
	id, _, err := a.b.authenticate(r)
	if err != nil {
		return nil, err
	}
	if !safeMethod(r.Method) && !requestedWith(r) {
		return nil, fmt.Errorf("%w: a session-authenticated %s needs %s: %s", auth.ErrForbidden, r.Method, HeaderRequestedWith, RequestedWithValue)
	}
	return id, nil
}

func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// AuthorizeOutgoing sets out's Authorization to the minter's service token
// for target. When in was authenticated by a session (a logged-in user),
// the token carries that user as its signed "act" claim and out gets
// X-Unheaded-On-Behalf-Of: the receiver records who asked without ever
// seeing the user's token (ADR-103 §4).
func AuthorizeOutgoing(out, in *http.Request, m *auth.ServiceTokenMinter, target string) error {
	var opts []auth.MintOption
	user := ""
	if id := auth.IdentityFromContext(in.Context()); id != nil && id.Extra["auth_method"] == "session" {
		user = id.Subject
		opts = append(opts, auth.WithActor(user))
	}
	h, err := m.AuthorizationHeader(target, opts...)
	if err != nil {
		return err
	}
	out.Header.Set("Authorization", h)
	if user != "" {
		out.Header.Set(HeaderOnBehalfOf, url.PathEscape(user))
	} else {
		out.Header.Del(HeaderOnBehalfOf)
	}
	return nil
}
