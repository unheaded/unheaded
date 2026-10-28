// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"unheaded/pkg/auth"
	"unheaded/pkg/httputil"
	"unheaded/pkg/metrics/auto"
	"unheaded/pkg/metrics/prom"
)

const (
	serviceName = "unheaded-auth"
	// tokenLifetime is the access-token lifetime (ADR-103 §3).
	tokenLifetime = 15 * time.Minute
	// defaultAudience is a token's "aud" unless the login asks for another
	// listed audience.
	defaultAudience = "unheaded"
	// maxLoginBody caps a login request body.
	maxLoginBody = 4096
	// maxConcurrentHashes bounds argon2id work in flight (64 MiB each).
	maxConcurrentHashes = 2
)

var (
	loginsTotal = auto.NewCounterVec(prom.CounterOpts{
		Name: "unheaded_auth_logins_total",
		Help: "Login attempts at the built-in issuer by result (ok, invalid, locked, bad_request, busy)",
	}, []string{"result"})
	tokensIssued = auto.NewCounter(prom.CounterOpts{
		Name: "unheaded_auth_tokens_issued_total",
		Help: "Access tokens minted by the built-in issuer",
	})
)

// issuerConfig configures the issuer.
type issuerConfig struct {
	// IssuerURL is the external URL: the token's "iss" and the base of
	// the discovery document. https, or http to loopback (any host with
	// DevPlaintext).
	IssuerURL string
	// KeyFile is the issuer's Ed25519 key (unheaded-keygen), 0400.
	KeyFile string
	// UsersFile is the users file, 0600.
	UsersFile string
	// DevPlaintext serves login, JWKS and discovery over plain HTTP.
	DevPlaintext bool
	// Audiences a login may ask for; default ["unheaded"]. The first is
	// the default audience.
	Audiences []string
	// Clock is the time source (tests); nil means time.Now.
	Clock func() time.Time
}

// issuer is the built-in OIDC-shaped issuer (ADR-103 §3): password login,
// EdDSA access tokens, JWKS and discovery.
type issuer struct {
	cfg       issuerConfig
	priv      ed25519.PrivateKey
	kid       string
	now       func() time.Time
	lock      *lockout
	hashSlots chan struct{}

	mu    sync.RWMutex
	users map[string]user
	dummy parsedHash // verified for unknown users, so they cost the same
}

func validateIssuerURL(raw string, dev bool) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || raw == "" {
		return "", fmt.Errorf("issuer URL %q: not a URL", raw)
	}
	if u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return "", fmt.Errorf("issuer URL %q: want scheme://host[:port][/path], no credentials, query or fragment", raw)
	}
	switch u.Scheme {
	case "https":
	case "http":
		h := u.Hostname()
		ip := net.ParseIP(h)
		if !dev && h != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return "", fmt.Errorf("issuer URL %q: plain http only to loopback (or with -dev-plaintext)", raw)
		}
	default:
		return "", fmt.Errorf("issuer URL %q: scheme must be https", raw)
	}
	return strings.TrimRight(raw, "/"), nil
}

func newIssuer(cfg issuerConfig) (*issuer, error) {
	iu, err := validateIssuerURL(cfg.IssuerURL, cfg.DevPlaintext)
	if err != nil {
		return nil, err
	}
	cfg.IssuerURL = iu
	if len(cfg.Audiences) == 0 {
		cfg.Audiences = []string{defaultAudience}
	}
	for _, a := range cfg.Audiences {
		if !auth.ValidKeyName(a) {
			return nil, fmt.Errorf("audience %q: want a service name ([a-z0-9][a-z0-9_-]*)", a)
		}
	}
	priv, kid, err := auth.LoadPrivateKey(cfg.KeyFile)
	if err != nil {
		return nil, err
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	s := &issuer{
		cfg:       cfg,
		priv:      priv,
		kid:       kid,
		now:       cfg.Clock,
		lock:      newLockout(cfg.Clock),
		hashSlots: make(chan struct{}, maxConcurrentHashes),
	}
	if err := s.ReloadUsers(); err != nil {
		return nil, err
	}
	return s, nil
}

// ReloadUsers re-reads the users file. On error the users already loaded
// stay in force.
func (s *issuer) ReloadUsers() error {
	users, err := loadUsers(s.cfg.UsersFile)
	if err != nil {
		return err
	}
	// The dummy costs what a real user costs: the params of any loaded
	// user, else the defaults.
	p := defaultParams
	for _, u := range users {
		p = u.hash.p
		break
	}
	dh, err := hashPassword("unheaded-auth-dummy", p)
	if err != nil {
		return err
	}
	dummy, err := parseHash(dh)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.users, s.dummy = users, dummy
	s.mu.Unlock()
	return nil
}

// Handler is the issuer's HTTP surface.
func (s *issuer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": serviceName})
	})
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.RLock()
		n := len(s.users)
		s.mu.RUnlock()
		writeJSON(w, http.StatusOK, map[string]any{"status": "ready", "users": n})
	})
	mux.Handle("GET /metrics", prom.Handler())
	mux.HandleFunc("GET /.well-known/openid-configuration", s.discovery)
	mux.HandleFunc("GET /jwks.json", s.jwks)
	mux.HandleFunc("POST /login", s.login)

	m := httputil.NewServiceMetrics(serviceName)
	return securityHeaders(m.Instrument(mux, s.requireTLS(mux)))
}

// requireTLS refuses everything but the probes over plain HTTP unless
// DevPlaintext: a password or a key set must not cross the network in
// clear.
func (s *issuer) requireTLS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil && !s.cfg.DevPlaintext {
			switch r.URL.Path {
			case "/health", "/ready", "/metrics":
			default:
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "tls_required"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// securityHeaders is the shared S-IRON-GATE Phase 11 header set
// (httputil.SecurityHeaders) with a CSP that allows nothing, since the
// issuer serves only JSON, plus Cache-Control: no-store on every response:
// /login answers carry tokens.
func securityHeaders(next http.Handler) http.Handler {
	noStore := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
	return httputil.SecurityHeaders(noStore, httputil.WithCSP(issuerCSP))
}

const issuerCSP = "default-src 'none'; frame-ancestors 'none'; base-uri 'none'"

func (s *issuer) discovery(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                s.cfg.IssuerURL,
		"jwks_uri":                              s.cfg.IssuerURL + "/jwks.json",
		"token_endpoint":                        s.cfg.IssuerURL + "/login",
		"response_types_supported":              []string{"token"},
		"grant_types_supported":                 []string{"password"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{auth.AlgEdDSA},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"claims_supported":                      []string{"sub", "iss", "aud", "exp", "iat", "nbf", "roles"},
	})
}

func (s *issuer) jwks(w http.ResponseWriter, _ *http.Request) {
	pub, ok := s.priv.Public().(ed25519.PublicKey)
	if !ok { // cannot happen for an ed25519.PrivateKey; refuse rather than serve nothing
		http.Error(w, "signing key is not Ed25519", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": []map[string]string{{
		"kty": "OKP",
		"crv": "Ed25519",
		"kid": s.kid,
		"x":   base64.RawURLEncoding.EncodeToString(pub),
		"use": "sig",
		"alg": auth.AlgEdDSA,
	}}})
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"` // #nosec G117 -- request field, never logged or stored
	Audience string `json:"audience,omitempty"`
}

// decodeLogin reads one strict JSON login object of at most maxLoginBody
// bytes. It returns the HTTP status to answer with on failure.
func decodeLogin(w http.ResponseWriter, r *http.Request) (loginRequest, int, string) {
	var req loginRequest
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "application/json" {
		return req, http.StatusUnsupportedMediaType, "content type must be application/json"
	}
	body := http.MaxBytesReader(w, r.Body, maxLoginBody)
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return req, http.StatusRequestEntityTooLarge, "request body too large"
		}
		return req, http.StatusBadRequest, "malformed request"
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return req, http.StatusRequestEntityTooLarge, "request body too large"
		}
		return req, http.StatusBadRequest, "malformed request"
	}
	if req.Username == "" || req.Password == "" || len(req.Username) > maxUsernameLen || len(req.Password) > maxPasswordLen {
		return req, http.StatusBadRequest, "username and password required"
	}
	return req, 0, ""
}

// sourceAddr is the peer address: never a forwarded header, which any
// client could set to dodge the lockout.
func sourceAddr(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *issuer) login(w http.ResponseWriter, r *http.Request) {
	req, status, msg := decodeLogin(w, r)
	if status != 0 {
		loginsTotal.WithLabelValues("bad_request").Inc()
		writeJSON(w, status, map[string]string{"error": "invalid_request", "error_description": msg})
		return
	}
	aud := s.cfg.Audiences[0]
	if req.Audience != "" {
		ok := false
		for _, a := range s.cfg.Audiences {
			ok = ok || a == req.Audience
		}
		if !ok {
			loginsTotal.WithLabelValues("bad_request").Inc()
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request", "error_description": "audience not offered by this issuer"})
			return
		}
		aud = req.Audience
	}
	src := sourceAddr(r)
	if locked, wait := s.lock.locked(req.Username, src); locked {
		loginsTotal.WithLabelValues("locked").Inc()
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds()+0.999)))
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "temporarily_locked"})
		return
	}

	select {
	case s.hashSlots <- struct{}{}:
	case <-r.Context().Done():
		loginsTotal.WithLabelValues("busy").Inc()
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "busy"})
		return
	}
	s.mu.RLock()
	u, known := s.users[req.Username]
	dummy := s.dummy
	s.mu.RUnlock()
	var ok bool
	if known {
		ok = u.hash.verify(req.Password)
	} else {
		_ = dummy.verify(req.Password)
	}
	<-s.hashSlots

	if !ok {
		s.lock.failure(req.Username, src)
		loginsTotal.WithLabelValues("invalid").Inc()
		// Only a known name is logged: a password typed into the username
		// field must not land in the log.
		logged := "(unknown)"
		if known {
			logged = u.name
		}
		log.Warn().Str("service", serviceName).Str("user", logged).Str("source", src).Msg("login failed")
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_credentials"})
		return
	}
	s.lock.success(req.Username, src)

	now := s.now()
	claims := &auth.JWTClaims{
		Sub:   u.name,
		Iss:   s.cfg.IssuerURL,
		Aud:   aud,
		Iat:   now.Unix(),
		Nbf:   now.Unix(),
		Exp:   now.Add(tokenLifetime).Unix(),
		Roles: u.roles,
	}
	tok, err := auth.SignEdDSA(s.priv, s.kid, claims)
	if err != nil {
		log.Error().Err(err).Str("service", serviceName).Msg("sign token")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server_error"})
		return
	}
	loginsTotal.WithLabelValues("ok").Inc()
	tokensIssued.Inc()
	log.Info().Str("service", serviceName).Str("user", u.name).Str("aud", aud).Str("source", src).Msg("token issued")
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": tok,
		"token_type":   "Bearer",
		"expires_in":   int(tokenLifetime.Seconds()),
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
