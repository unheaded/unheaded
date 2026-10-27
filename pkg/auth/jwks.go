// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package auth

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// User tokens (ADR-103 §3): services verify them against the issuer's JWKS,
// found through OIDC discovery. The built-in issuer (unheaded-auth) and any
// OIDC provider configured for EdDSA plug in the same way.

const (
	// maxJWKSBytes caps the discovery document and the JWKS.
	maxJWKSBytes = 64 << 10
	// maxJWKSKeys caps the keys taken from one JWKS.
	maxJWKSKeys = 64
	// jwksFetchTimeout bounds each discovery or JWKS request.
	jwksFetchTimeout = 5 * time.Second
	// defaultJWKSTTL is how long fetched keys are used before a refetch.
	defaultJWKSTTL = 10 * time.Minute
	// jwksUnknownKidInterval is the least time between refetches caused by
	// an unknown kid, so a flood of made-up kids cannot hammer the issuer.
	jwksUnknownKidInterval = 30 * time.Second
)

// JWKSVerifier authenticates user tokens signed by an OIDC issuer's EdDSA
// keys. Keys are fetched lazily (a service starts while the issuer is
// down), cached for the TTL, refetched once for an unknown kid (at most
// every 30 s), and kept when a refetch fails. Safe for concurrent use.
type JWKSVerifier struct {
	issuer   string
	audience string
	client   *http.Client
	ttl      time.Duration
	skew     time.Duration
	maxLife  time.Duration
	now      func() time.Time

	fetchMu sync.Mutex // serialises fetches

	mu          sync.RWMutex
	keys        map[string]ed25519.PublicKey
	fetchedAt   time.Time
	lastUnknown time.Time
	lastErr     error
}

// NewJWKSVerifier returns a verifier for tokens from issuerURL (the token's
// "iss" and the discovery base) with aud == audience. The issuer must be
// https, or http on loopback only. A nil httpClient gets a 5 s timeout; a
// TTL of 0 means 10 minutes.
func NewJWKSVerifier(issuerURL, audience string, httpClient *http.Client, ttl time.Duration) (*JWKSVerifier, error) {
	if audience == "" {
		return nil, errors.New("auth: JWKS verifier needs an audience")
	}
	if err := checkFetchURL(issuerURL); err != nil {
		return nil, fmt.Errorf("auth: OIDC issuer: %w", err)
	}
	c := &http.Client{Timeout: jwksFetchTimeout}
	if httpClient != nil {
		cp := *httpClient
		c = &cp
	}
	// Redirects must meet the same rule, or https could be bounced to http.
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("too many redirects")
		}
		return checkFetchURL(req.URL.String())
	}
	if ttl <= 0 {
		ttl = defaultJWKSTTL
	}
	return &JWKSVerifier{
		issuer:   strings.TrimRight(issuerURL, "/"),
		audience: audience,
		client:   c,
		ttl:      ttl,
		skew:     5 * time.Second,
		maxLife:  DefaultMaxUserTokenLifetime,
		now:      time.Now,
	}, nil
}

// DefaultMaxUserTokenLifetime bounds exp - now for a user token unless
// SetMaxLifetime says otherwise. The built-in issuer mints 15-minute tokens
// (ADR-103 §3); an hour leaves room for other OIDC providers' defaults while
// a mis-set issuer still cannot hand out day-long tokens.
const DefaultMaxUserTokenLifetime = time.Hour

// SetMaxLifetime replaces DefaultMaxUserTokenLifetime (AUTH_OIDC_MAX_LIFETIME).
// A value <= 0 keeps the current cap. Call it before the verifier is in use.
func (v *JWKSVerifier) SetMaxLifetime(d time.Duration) {
	if d > 0 {
		v.maxLife = d
	}
}

// checkFetchURL allows https anywhere and http only to loopback, with no
// userinfo.
func checkFetchURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Host == "" || u.Hostname() == "" {
		return errors.New("no host")
	}
	if u.User != nil {
		return errors.New("credentials in the URL")
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		h := u.Hostname()
		if h == "localhost" {
			return nil
		}
		if ip := net.ParseIP(h); ip != nil && ip.IsLoopback() {
			return nil
		}
		return errors.New("plain http is allowed to loopback only")
	default:
		return fmt.Errorf("scheme %q not allowed", u.Scheme)
	}
}

// Authenticate implements Authenticator.
func (v *JWKSVerifier) Authenticate(ctx context.Context, r *http.Request) (*Identity, error) {
	tok, err := bearerToken(r)
	if err != nil {
		return nil, err
	}
	lookup := func(kid string) (ed25519.PublicKey, string, bool) {
		pub, ok := v.keyFor(ctx, kid)
		return pub, "", ok
	}
	payload, kid, _, err := verifyEdDSA(tok, lookup)
	if err != nil {
		if errors.Is(err, ErrUnknownKey) {
			v.mu.RLock()
			lastErr := v.lastErr
			v.mu.RUnlock()
			if lastErr != nil {
				return nil, fmt.Errorf("%w (issuer keys: %v)", err, lastErr)
			}
		}
		return nil, err
	}
	var oc oidcClaims
	if err := json.Unmarshal(payload, &oc); err != nil {
		return nil, fmt.Errorf("%w: invalid payload JSON", ErrUnauthenticated)
	}
	if !oc.Aud.has(v.audience) {
		return nil, fmt.Errorf("%w: audience mismatch: want %q", ErrForbidden, v.audience)
	}
	claims := oc.JWTClaims
	claims.Aud = v.audience
	now := v.now()
	if err := claims.Validate(now, v.issuer, v.audience, v.skew); err != nil {
		return nil, err
	}
	if err := checkLifetime(&claims, now, v.maxLife, v.skew); err != nil {
		return nil, err
	}
	return &Identity{
		Subject: claims.Sub,
		Roles:   claims.Roles,
		Extra:   map[string]string{"auth_method": "jwks", "iss": claims.Iss, "kid": kid},
	}, nil
}

// oidcClaims reads "aud" as a string or an array (RFC 7519 §4.1.3).
type oidcClaims struct {
	JWTClaims
	Aud audience `json:"aud"`
}

type audience []string

func (a *audience) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*a = audience{s}
		return nil
	}
	var l []string
	if err := json.Unmarshal(b, &l); err != nil {
		return err
	}
	*a = l
	return nil
}

func (a audience) has(want string) bool {
	for _, s := range a {
		if s == want {
			return true
		}
	}
	return false
}

// keyFor returns the key for kid, refreshing when the cache is empty or
// older than the TTL, and once (rate-limited) for an unknown kid.
func (v *JWKSVerifier) keyFor(ctx context.Context, kid string) (ed25519.PublicKey, bool) {
	now := v.now()
	v.mu.RLock()
	stale := v.keys == nil || now.Sub(v.fetchedAt) >= v.ttl
	v.mu.RUnlock()
	if stale {
		v.refresh(ctx, false)
	}
	if pub, ok := v.cached(kid); ok {
		return pub, true
	}
	v.mu.Lock()
	allowed := v.lastUnknown.IsZero() || now.Sub(v.lastUnknown) >= jwksUnknownKidInterval
	if allowed {
		v.lastUnknown = now
	}
	v.mu.Unlock()
	if !allowed {
		return nil, false
	}
	v.refresh(ctx, true)
	return v.cached(kid)
}

func (v *JWKSVerifier) cached(kid string) (ed25519.PublicKey, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	pub, ok := v.keys[kid]
	return pub, ok
}

// refresh fetches the keys unless (without force) another caller already
// refreshed them within the TTL. A failed fetch keeps the old keys.
func (v *JWKSVerifier) refresh(ctx context.Context, force bool) {
	v.fetchMu.Lock()
	defer v.fetchMu.Unlock()
	if !force {
		v.mu.RLock()
		fresh := v.keys != nil && v.now().Sub(v.fetchedAt) < v.ttl
		v.mu.RUnlock()
		if fresh {
			return
		}
	}
	keys, err := v.fetch(ctx)
	v.mu.Lock()
	defer v.mu.Unlock()
	if err != nil {
		v.lastErr = err
		log.Warn().Err(err).Str("issuer", v.issuer).Msg("auth: JWKS fetch failed; keeping the keys already held")
		return
	}
	v.keys, v.fetchedAt, v.lastErr = keys, v.now(), nil
}

func (v *JWKSVerifier) fetch(ctx context.Context) (map[string]ed25519.PublicKey, error) {
	var disc struct {
		Issuer  string `json:"issuer"`
		JWKSURI string `json:"jwks_uri"`
	}
	if err := v.getJSON(ctx, v.issuer+"/.well-known/openid-configuration", &disc); err != nil {
		return nil, fmt.Errorf("discovery: %w", err)
	}
	if strings.TrimRight(disc.Issuer, "/") != v.issuer {
		return nil, fmt.Errorf("discovery names issuer %q, configured %q", disc.Issuer, v.issuer)
	}
	if err := checkFetchURL(disc.JWKSURI); err != nil {
		return nil, fmt.Errorf("jwks_uri: %w", err)
	}
	var set struct {
		Keys []json.RawMessage `json:"keys"`
	}
	if err := v.getJSON(ctx, disc.JWKSURI, &set); err != nil {
		return nil, fmt.Errorf("jwks: %w", err)
	}
	return parseJWKS(set.Keys)
}

// jwk is the subset of an RFC 7517 / RFC 8037 key we read.
type jwk struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	Kid string `json:"kid"`
	X   string `json:"x"`
	Use string `json:"use"`
	Alg string `json:"alg"`
}

// parseJWKS keeps the Ed25519 signing keys and skips the rest (an IdP may
// publish RSA keys too). It fails when no usable key remains.
func parseJWKS(raw []json.RawMessage) (map[string]ed25519.PublicKey, error) {
	if len(raw) > maxJWKSKeys {
		return nil, fmt.Errorf("more than %d keys", maxJWKSKeys)
	}
	keys := make(map[string]ed25519.PublicKey)
	for _, r := range raw {
		var k jwk
		if err := json.Unmarshal(r, &k); err != nil {
			continue
		}
		if k.Kty != "OKP" || k.Crv != "Ed25519" || k.Kid == "" {
			continue
		}
		if (k.Use != "" && k.Use != "sig") || (k.Alg != "" && k.Alg != AlgEdDSA) {
			continue
		}
		x, err := base64.RawURLEncoding.Strict().DecodeString(k.X)
		if err != nil || len(x) != ed25519.PublicKeySize {
			continue
		}
		if _, dup := keys[k.Kid]; dup {
			return nil, fmt.Errorf("duplicate kid %q", k.Kid)
		}
		keys[k.Kid] = ed25519.PublicKey(x)
	}
	if len(keys) == 0 {
		return nil, errors.New("no Ed25519 signing keys")
	}
	return keys, nil
}

func (v *JWKSVerifier) getJSON(ctx context.Context, u string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, jwksFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := v.client.Do(req) // #nosec G107 -- issuer URL from configuration, scheme/host checked by checkFetchURL
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := readJSONDoc(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

// readJSONDoc reads at most maxJWKSBytes and refuses anything larger.
func readJSONDoc(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxJWKSBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxJWKSBytes {
		return nil, fmt.Errorf("document larger than %d bytes", maxJWKSBytes)
	}
	return b, nil
}
