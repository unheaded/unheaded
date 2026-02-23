// Package auth provides authentication and authorization primitives for the
// Unheaded Kingdom.
//
// Three authentication mechanisms are implemented:
//   - JWT validation (HMAC-SHA256) for external API clients
//   - API key validation for programmatic access
//   - Service-to-service tokens for inter-service communication
//   - mTLS certificate verification (stub — planned for Beta)
//
// All share the common Authenticator interface so callers don't need to know
// which backend is active.
package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

// Common errors returned by auth implementations.
var (
	ErrUnauthenticated = errors.New("auth: unauthenticated")
	ErrForbidden       = errors.New("auth: forbidden")
	ErrTokenExpired    = errors.New("auth: token expired")
	ErrInvalidCert     = errors.New("auth: invalid client certificate")
	ErrRateLimited     = errors.New("auth: rate limited")
)

// Identity represents a verified caller identity.
type Identity struct {
	// Subject is the unique identifier for the caller (e.g. service name,
	// user email, or CN from an mTLS certificate).
	Subject string

	// Roles lists the authorization roles granted to this identity.
	Roles []string

	// Extra holds implementation-specific claims (e.g. JWT custom claims).
	Extra map[string]string
}

// HasRole returns true if the identity has the given role.
func (id *Identity) HasRole(role string) bool {
	for _, r := range id.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// Authenticator verifies incoming requests and returns the caller's identity.
type Authenticator interface {
	// Authenticate inspects the request (headers, TLS state, etc.) and
	// returns an Identity if the caller is authenticated.
	// Returns ErrUnauthenticated if credentials are missing or invalid.
	Authenticate(ctx context.Context, r *http.Request) (*Identity, error)
}

// JWTValidator validates JSON Web Tokens.
// Stub -- real implementation needs:
//   - JWKS endpoint or public key configuration
//   - Token parsing & signature verification (e.g. golang-jwt/jwt)
//   - Claims extraction (sub, roles, exp)
//   - Token refresh/rotation support
type JWTValidator struct {
	// Issuer is the expected "iss" claim.
	Issuer string
	// Audience is the expected "aud" claim.
	Audience string
}

// Authenticate validates the Bearer token from the Authorization header.
// STUB: always returns ErrUnauthenticated until implemented.
func (j *JWTValidator) Authenticate(_ context.Context, r *http.Request) (*Identity, error) {
	// TODO: Implement JWT validation
	// 1. Extract Bearer token from Authorization header
	// 2. Parse and verify signature against JWKS / public key
	// 3. Validate exp, iss, aud claims
	// 4. Extract subject and roles from claims
	_ = r
	return nil, ErrUnauthenticated
}

// MTLSValidator validates client certificates presented during TLS handshake.
// Stub -- real implementation needs:
//   - CA certificate pool configuration
//   - Certificate chain verification
//   - CN/SAN extraction for identity mapping
//   - CRL/OCSP revocation checking
type MTLSValidator struct {
	// ServiceName is the expected CN or SAN of the calling service.
	ServiceName string
}

// Authenticate validates the client certificate from the TLS connection state.
// STUB: always returns ErrUnauthenticated until implemented.
func (m *MTLSValidator) Authenticate(_ context.Context, r *http.Request) (*Identity, error) {
	// TODO: Implement mTLS validation
	// 1. Check r.TLS != nil and r.TLS.PeerCertificates is non-empty
	// 2. Verify certificate chain against configured CA pool
	// 3. Extract CN and SANs for identity
	// 4. Check CRL/OCSP status
	_ = r
	return nil, ErrUnauthenticated
}

// Middleware returns an HTTP middleware that authenticates requests using the
// provided Authenticator. On success, the Identity is stored in the request
// context. On failure, the appropriate HTTP status code is returned:
//   - 401 for missing/invalid credentials or expired tokens
//   - 403 for forbidden (wrong audience, insufficient role)
//   - 429 for rate limited
func Middleware(auth Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity, err := auth.Authenticate(r.Context(), r)
			if err != nil {
				switch {
				case errors.Is(err, ErrRateLimited):
					http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				case errors.Is(err, ErrForbidden):
					http.Error(w, "Forbidden", http.StatusForbidden)
				default:
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
				}
				return
			}
			ctx := context.WithValue(r.Context(), identityKey, identity)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// MultiAuthenticator tries multiple authenticators in order.
// The first one that succeeds wins. If all fail, returns the last error.
type MultiAuthenticator struct {
	Authenticators []Authenticator
}

// Authenticate tries each authenticator in order.
func (m *MultiAuthenticator) Authenticate(ctx context.Context, r *http.Request) (*Identity, error) {
	var lastErr error
	for _, a := range m.Authenticators {
		identity, err := a.Authenticate(ctx, r)
		if err == nil {
			return identity, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, ErrUnauthenticated
}

// SkipAuthPaths returns a middleware that skips auth for the given path prefixes.
// Used to keep /health, /ready, and /metrics unauthenticated for Prometheus scraping.
func SkipAuthPaths(authMiddleware func(http.Handler) http.Handler, skipPrefixes ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		authed := authMiddleware(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, prefix := range skipPrefixes {
				if r.URL.Path == prefix || strings.HasPrefix(r.URL.Path, prefix+"/") {
					next.ServeHTTP(w, r)
					return
				}
			}
			authed.ServeHTTP(w, r)
		})
	}
}

// identityKeyType is an unexported type for the context key, preventing collisions.
type identityKeyType struct{}

var identityKey = identityKeyType{}

// IdentityFromContext extracts the authenticated Identity from a request context.
// Returns nil if the request was not authenticated.
func IdentityFromContext(ctx context.Context) *Identity {
	if id, ok := ctx.Value(identityKey).(*Identity); ok {
		return id
	}
	return nil
}
