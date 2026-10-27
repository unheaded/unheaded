// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package auth

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// EdDSA (Ed25519) JWS for service tokens, ADR-103 §1-2. A verifier holds
// only public keys, so being able to check a token no longer means being
// able to mint one (the HS256 shared key's flaw). Hand-rolled like jwt.go:
// stdlib crypto/ed25519, no JWT library.

const (
	// AlgEdDSA is the only "alg" an EdDSA verifier accepts (RFC 8037).
	AlgEdDSA = "EdDSA"
	// EdDSAServiceIssuer is the "iss" of service tokens a service mints for
	// itself (ADR-103 token shape, S-IRON-GATE Appendix C).
	EdDSAServiceIssuer = "unheaded"
	// DefaultMaxTokenLifetime bounds exp - now for an accepted EdDSA token:
	// service tokens live 5 minutes, user tokens 15.
	DefaultMaxTokenLifetime = 15 * time.Minute
	// maxTokenBytes bounds the work spent on one presented token.
	maxTokenBytes = 8192
)

// Errors returned by EdDSA verification. Each wraps ErrUnauthenticated (401)
// or ErrForbidden (403), so Middleware and the gRPC interceptors map them.
var (
	// ErrUnknownKey: the token's kid is not a trusted key (not in trust.d,
	// or not in the issuer's JWKS).
	ErrUnknownKey = fmt.Errorf("%w: unknown signing key", ErrUnauthenticated)
	// ErrAlgMismatch: the token's alg is not the one the verifier's keys
	// are for ("none", HS256 at an EdDSA verifier, or HS256 outside tests).
	ErrAlgMismatch = fmt.Errorf("%w: token algorithm not accepted", ErrUnauthenticated)
	// ErrCallerNotAllowed: a correctly signed service token from a caller
	// that is not on this service's allowlist (AUTH_ALLOW_CALLERS). 403.
	ErrCallerNotAllowed = fmt.Errorf("%w: caller not allowed", ErrForbidden)
)

// jwsHeader is the JOSE header as verifiers read it. Crit is kept raw: any
// critical extension is one we do not implement, so its presence refuses
// the token (RFC 7515 §4.1.11).
type jwsHeader struct {
	Alg  string          `json:"alg"`
	Typ  string          `json:"typ,omitempty"`
	Kid  string          `json:"kid,omitempty"`
	Crit json.RawMessage `json:"crit,omitempty"`
}

// SignEdDSA signs claims as a compact JWS with header
// {"alg":"EdDSA","typ":"JWT","kid":kid}. kid must be the key id of priv.
func SignEdDSA(priv ed25519.PrivateKey, kid string, claims *JWTClaims) (string, error) {
	if len(priv) != ed25519.PrivateKeySize {
		return "", errors.New("auth: SignEdDSA: not an Ed25519 private key")
	}
	if claims == nil {
		return "", errors.New("auth: SignEdDSA: nil claims")
	}
	pub, ok := priv.Public().(ed25519.PublicKey)
	if !ok || kid == "" || KeyID(pub) != kid {
		return "", errors.New("auth: SignEdDSA: kid does not name this key")
	}
	hdr, err := json.Marshal(JWTHeader{Alg: AlgEdDSA, Typ: "JWT", Kid: kid})
	if err != nil {
		return "", fmt.Errorf("marshal header: %w", err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}
	input := base64.RawURLEncoding.EncodeToString(hdr) + "." + base64.RawURLEncoding.EncodeToString(payload)
	sig := ed25519.Sign(priv, []byte(input))
	return input + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// keyLookup resolves a kid to a public key and the identity it is bound to
// ("" when the source binds none, e.g. a JWKS).
type keyLookup func(kid string) (pub ed25519.PublicKey, owner string, ok bool)

// verifyEdDSA checks a compact JWS: size, shape, alg == EdDSA, no crit, a
// kid that lookup knows, and the signature. It returns the payload bytes,
// the kid and the key's owner. Nothing in the payload is read before the
// signature verifies.
func verifyEdDSA(token string, lookup keyLookup) (payload []byte, kid, owner string, err error) {
	if len(token) > maxTokenBytes {
		return nil, "", "", fmt.Errorf("%w: token larger than %d bytes", ErrUnauthenticated, maxTokenBytes)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, "", "", fmt.Errorf("%w: malformed JWT", ErrUnauthenticated)
	}
	rawHdr, err := base64.RawURLEncoding.Strict().DecodeString(parts[0])
	if err != nil {
		return nil, "", "", fmt.Errorf("%w: invalid header encoding", ErrUnauthenticated)
	}
	var hdr jwsHeader
	if err := json.Unmarshal(rawHdr, &hdr); err != nil {
		return nil, "", "", fmt.Errorf("%w: invalid header JSON", ErrUnauthenticated)
	}
	// Exact match: "none", "None", "HS256", "eddsa" and anything else are
	// refused before a key is even looked up.
	if hdr.Alg != AlgEdDSA {
		return nil, "", "", fmt.Errorf("%w: got %q, want %q", ErrAlgMismatch, hdr.Alg, AlgEdDSA)
	}
	if len(hdr.Crit) != 0 {
		return nil, "", "", fmt.Errorf("%w: unsupported critical header", ErrUnauthenticated)
	}
	if hdr.Kid == "" {
		return nil, "", "", fmt.Errorf("%w: token has no kid", ErrUnknownKey)
	}
	pub, owner, ok := lookup(hdr.Kid)
	if !ok {
		return nil, "", "", ErrUnknownKey
	}
	sig, err := base64.RawURLEncoding.Strict().DecodeString(parts[2])
	if err != nil || len(sig) != ed25519.SignatureSize {
		return nil, "", "", fmt.Errorf("%w: invalid signature encoding", ErrUnauthenticated)
	}
	if !ed25519.Verify(pub, []byte(parts[0]+"."+parts[1]), sig) {
		return nil, "", "", fmt.Errorf("%w: invalid signature", ErrUnauthenticated)
	}
	payload, err = base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil {
		return nil, "", "", fmt.Errorf("%w: invalid payload encoding", ErrUnauthenticated)
	}
	return payload, hdr.Kid, owner, nil
}

// bearerToken extracts the token from "Authorization: Bearer <token>".
func bearerToken(r *http.Request) (string, error) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", ErrUnauthenticated
	}
	scheme, tok, ok := strings.Cut(h, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", fmt.Errorf("%w: invalid Authorization header format", ErrUnauthenticated)
	}
	tok = strings.TrimSpace(tok)
	if tok == "" {
		return "", fmt.Errorf("%w: empty token", ErrUnauthenticated)
	}
	return tok, nil
}

// checkLifetime refuses a token issued in the future or valid for longer
// than maxLife from now (a key holder cannot mint a day-long token).
func checkLifetime(c *JWTClaims, now time.Time, maxLife, skew time.Duration) error {
	if c.Iat != 0 && c.Iat > now.Add(skew).Unix() {
		return fmt.Errorf("%w: token issued in the future", ErrUnauthenticated)
	}
	if maxLife > 0 && c.Exp > now.Add(maxLife+skew).Unix() {
		return fmt.Errorf("%w: token lifetime exceeds %s", ErrUnauthenticated, maxLife)
	}
	return nil
}

// EdDSAVerifierConfig configures an EdDSAVerifier.
type EdDSAVerifierConfig struct {
	// Trust is the set of service public keys (trust.d). Required.
	Trust *TrustDir
	// Issuer is the required "iss" (EdDSAServiceIssuer for service tokens).
	Issuer string
	// Audience is the required "aud": this service's name. Required, so a
	// token minted for one service cannot be replayed at another.
	Audience string
	// AllowCallers lists the subjects (caller service names) allowed to call
	// this service. Empty allows none: a valid token from anyone gets 403.
	AllowCallers []string
	// ClockSkew is the tolerance for exp/nbf/iat. Default 5 s.
	ClockSkew time.Duration
	// MaxLifetime bounds exp - now. Default DefaultMaxTokenLifetime.
	MaxLifetime time.Duration
	// Now is the clock; nil means time.Now.
	Now func() time.Time
}

// EdDSAVerifier authenticates service tokens: EdDSA-signed by a key in the
// trust directory, sub equal to the name that key is filed under, iss and
// aud as configured, unexpired, and sub on the caller allowlist.
type EdDSAVerifier struct {
	trust        *TrustDir
	issuer       string
	audience     string
	allowCallers map[string]struct{}
	skew         time.Duration
	maxLife      time.Duration
	now          func() time.Time
}

// NewEdDSAVerifier returns a verifier, refusing a config that would accept
// tokens for any audience or issuer.
func NewEdDSAVerifier(cfg EdDSAVerifierConfig) (*EdDSAVerifier, error) {
	if cfg.Trust == nil {
		return nil, errors.New("auth: EdDSA verifier needs a trust directory")
	}
	if cfg.Issuer == "" || cfg.Audience == "" {
		return nil, errors.New("auth: EdDSA verifier needs an issuer and an audience")
	}
	v := &EdDSAVerifier{
		trust:        cfg.Trust,
		issuer:       cfg.Issuer,
		audience:     cfg.Audience,
		allowCallers: make(map[string]struct{}, len(cfg.AllowCallers)),
		skew:         cfg.ClockSkew,
		maxLife:      cfg.MaxLifetime,
		now:          cfg.Now,
	}
	for _, c := range cfg.AllowCallers {
		if c = strings.TrimSpace(c); c != "" {
			v.allowCallers[c] = struct{}{}
		}
	}
	if v.skew == 0 {
		v.skew = 5 * time.Second
	}
	if v.maxLife == 0 {
		v.maxLife = DefaultMaxTokenLifetime
	}
	if v.now == nil {
		v.now = time.Now
	}
	return v, nil
}

// Authenticate implements Authenticator.
func (v *EdDSAVerifier) Authenticate(_ context.Context, r *http.Request) (*Identity, error) {
	tok, err := bearerToken(r)
	if err != nil {
		return nil, err
	}
	payload, kid, owner, err := verifyEdDSA(tok, v.trust.LookupName)
	if err != nil {
		return nil, err
	}
	var claims JWTClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("%w: invalid payload JSON", ErrUnauthenticated)
	}
	// The key proves who signed; its file name says who that is. A key
	// speaks only for its own service.
	if claims.Sub != owner {
		return nil, fmt.Errorf("%w: subject %q does not own key %s", ErrUnauthenticated, claims.Sub, kid)
	}
	now := v.now()
	if err := claims.Validate(now, v.issuer, v.audience, v.skew); err != nil {
		return nil, err
	}
	if err := checkLifetime(&claims, now, v.maxLife, v.skew); err != nil {
		return nil, err
	}
	if _, ok := v.allowCallers[claims.Sub]; !ok {
		return nil, fmt.Errorf("%w: %q", ErrCallerNotAllowed, claims.Sub)
	}
	// Roles in a service token are asserted by the caller's own key, so
	// they are not taken: a service is a service, whatever it claims.
	return &Identity{
		Subject: claims.Sub,
		Roles:   []string{RoleService},
		Extra:   map[string]string{"auth_method": "eddsa", "kid": kid},
	}, nil
}
