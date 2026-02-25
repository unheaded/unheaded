// Package auth — JWT authentication with HMAC-SHA256.
// No external JWT libraries — uses crypto/hmac + crypto/sha256 from stdlib.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// JWTConfig configures the JWT validator.
type JWTConfig struct {
	// SigningKey is the HMAC-SHA256 shared secret.
	SigningKey []byte
	// Issuer is the expected "iss" claim. Empty = skip check.
	Issuer string
	// Audience is the expected "aud" claim. Empty = skip check.
	Audience string
	// AllowedRoles restricts which roles are accepted. Empty = allow all.
	AllowedRoles []string
}

// JWTHeader represents the JOSE header of a JWT.
type JWTHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

// JWTClaims represents the JWT payload claims.
type JWTClaims struct {
	// Registered claims
	Sub string `json:"sub"`           // Subject
	Iss string `json:"iss,omitempty"` // Issuer
	Aud string `json:"aud,omitempty"` // Audience
	Exp int64  `json:"exp,omitempty"` // Expiration (Unix seconds)
	Iat int64  `json:"iat,omitempty"` // Issued at (Unix seconds)
	Nbf int64  `json:"nbf,omitempty"` // Not before (Unix seconds)

	// Custom claims
	Roles []string          `json:"roles,omitempty"`
	Extra map[string]string `json:"extra,omitempty"`
}

// Validate checks that the claims are valid at the given time.
func (c *JWTClaims) Validate(now time.Time, issuer, audience string) error {
	unixNow := now.Unix()

	// Check expiration
	if c.Exp > 0 && unixNow > c.Exp {
		return ErrTokenExpired
	}

	// Check not-before
	if c.Nbf > 0 && unixNow < c.Nbf {
		return fmt.Errorf("%w: token not yet valid", ErrUnauthenticated)
	}

	// Check issuer
	if issuer != "" && c.Iss != issuer {
		return fmt.Errorf("%w: issuer mismatch: got %q, want %q", ErrUnauthenticated, c.Iss, issuer)
	}

	// Check audience
	if audience != "" && c.Aud != audience {
		return fmt.Errorf("%w: audience mismatch: got %q, want %q", ErrForbidden, c.Aud, audience)
	}

	// Subject is required
	if c.Sub == "" {
		return fmt.Errorf("%w: missing subject", ErrUnauthenticated)
	}

	return nil
}

// NewJWTAuthenticator creates a new JWT authenticator from config.
func NewJWTAuthenticator(cfg JWTConfig) *JWTAuthenticator {
	return &JWTAuthenticator{
		signingKey:   cfg.SigningKey,
		issuer:       cfg.Issuer,
		audience:     cfg.Audience,
		allowedRoles: cfg.AllowedRoles,
		nowFunc:      time.Now,
	}
}

// JWTAuthenticator validates JWT tokens using HMAC-SHA256.
type JWTAuthenticator struct {
	signingKey   []byte
	issuer       string
	audience     string
	allowedRoles []string
	nowFunc      func() time.Time // injectable for testing
}

// Authenticate extracts and validates a JWT from the Authorization: Bearer header.
func (j *JWTAuthenticator) Authenticate(_ context.Context, r *http.Request) (*Identity, error) {
	// 1. Extract token from Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, ErrUnauthenticated
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return nil, fmt.Errorf("%w: invalid Authorization header format", ErrUnauthenticated)
	}

	tokenStr := strings.TrimSpace(parts[1])
	if tokenStr == "" {
		return nil, fmt.Errorf("%w: empty token", ErrUnauthenticated)
	}

	// 2. Parse and verify token
	claims, err := j.parseAndVerify(tokenStr)
	if err != nil {
		return nil, err
	}

	// 3. Validate claims
	now := j.nowFunc()
	if err := claims.Validate(now, j.issuer, j.audience); err != nil {
		return nil, err
	}

	// 4. Check role restrictions
	if len(j.allowedRoles) > 0 {
		if !hasAnyRole(claims.Roles, j.allowedRoles) {
			return nil, fmt.Errorf("%w: insufficient role permissions", ErrForbidden)
		}
	}

	// 5. Build identity
	return &Identity{
		Subject: claims.Sub,
		Roles:   claims.Roles,
		Extra:   claims.Extra,
	}, nil
}

// parseAndVerify parses a JWT string and verifies the HMAC-SHA256 signature.
func (j *JWTAuthenticator) parseAndVerify(tokenStr string) (*JWTClaims, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("%w: malformed JWT", ErrUnauthenticated)
	}

	headerB64 := parts[0]
	payloadB64 := parts[1]
	signatureB64 := parts[2]

	// Decode and verify header
	headerBytes, err := base64.RawURLEncoding.DecodeString(headerB64)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid header encoding", ErrUnauthenticated)
	}

	var header JWTHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("%w: invalid header JSON", ErrUnauthenticated)
	}

	if header.Alg != "HS256" {
		return nil, fmt.Errorf("%w: unsupported algorithm %q", ErrUnauthenticated, header.Alg)
	}

	// Verify signature
	signingInput := headerB64 + "." + payloadB64
	expectedSig := computeHMACSHA256(j.signingKey, []byte(signingInput))

	actualSig, err := base64.RawURLEncoding.DecodeString(signatureB64)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid signature encoding", ErrUnauthenticated)
	}

	if !hmac.Equal(expectedSig, actualSig) {
		return nil, fmt.Errorf("%w: invalid signature", ErrUnauthenticated)
	}

	// Decode payload
	payloadBytes, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid payload encoding", ErrUnauthenticated)
	}

	var claims JWTClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("%w: invalid payload JSON", ErrUnauthenticated)
	}

	return &claims, nil
}

// SignToken creates a signed JWT from the given claims.
func SignToken(signingKey []byte, claims *JWTClaims) (string, error) {
	// Encode header
	header := JWTHeader{Alg: "HS256", Typ: "JWT"}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("marshal header: %w", err)
	}
	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)

	// Encode payload
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)

	// Create signature
	signingInput := headerB64 + "." + payloadB64
	signature := computeHMACSHA256(signingKey, []byte(signingInput))
	signatureB64 := base64.RawURLEncoding.EncodeToString(signature)

	return signingInput + "." + signatureB64, nil
}

// computeHMACSHA256 computes HMAC-SHA256 of data with the given key.
func computeHMACSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

// hasAnyRole returns true if any of the identity roles are in the allowed set.
func hasAnyRole(identityRoles, allowedRoles []string) bool {
	for _, ir := range identityRoles {
		for _, ar := range allowedRoles {
			if ir == ar {
				return true
			}
		}
	}
	return false
}
