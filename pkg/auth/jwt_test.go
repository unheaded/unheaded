package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// testSigningKey is a fixed key for deterministic tests.
var testSigningKey = []byte("test-hmac-secret-key-for-unheaded-kingdom-256bit")

func makeTestToken(t *testing.T, claims *JWTClaims) string {
	t.Helper()
	token, err := SignToken(testSigningKey, claims)
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}
	return token
}

func newJWTAuth() *JWTAuthenticator {
	return NewJWTAuthenticator(JWTConfig{
		SigningKey: testSigningKey,
		Issuer:    "https://auth.unheaded.dev",
		Audience:  "unheaded-api",
	})
}

// --- JWTAuthenticator Tests ---

func TestJWTAuthenticator_ValidToken(t *testing.T) {
	auth := newJWTAuth()
	token := makeTestToken(t, &JWTClaims{
		Sub:   "admin@unheaded.dev",
		Iss:   "https://auth.unheaded.dev",
		Aud:   "unheaded-api",
		Exp:   time.Now().Add(1 * time.Hour).Unix(),
		Iat:   time.Now().Unix(),
		Roles: []string{"admin", "operator"},
		Extra: map[string]string{"tier": "alpha"},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	identity, err := auth.Authenticate(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if identity.Subject != "admin@unheaded.dev" {
		t.Errorf("Subject = %q, want %q", identity.Subject, "admin@unheaded.dev")
	}
	if !identity.HasRole("admin") {
		t.Error("expected admin role")
	}
	if !identity.HasRole("operator") {
		t.Error("expected operator role")
	}
	if identity.Extra["tier"] != "alpha" {
		t.Errorf("Extra[tier] = %q, want %q", identity.Extra["tier"], "alpha")
	}
}

func TestJWTAuthenticator_ExpiredToken(t *testing.T) {
	auth := newJWTAuth()
	token := makeTestToken(t, &JWTClaims{
		Sub:   "user@unheaded.dev",
		Iss:   "https://auth.unheaded.dev",
		Aud:   "unheaded-api",
		Exp:   time.Now().Add(-1 * time.Hour).Unix(),
		Roles: []string{"admin"},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	_, err := auth.Authenticate(context.Background(), req)
	if !errors.Is(err, ErrTokenExpired) {
		t.Errorf("expected ErrTokenExpired, got: %v", err)
	}
}

func TestJWTAuthenticator_MissingToken(t *testing.T) {
	auth := newJWTAuth()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline", nil)
	// No Authorization header

	_, err := auth.Authenticate(context.Background(), req)
	if !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("expected ErrUnauthenticated, got: %v", err)
	}
}

func TestJWTAuthenticator_InvalidSignature(t *testing.T) {
	// Sign with a different key
	otherKey := []byte("wrong-key-not-the-real-one-at-all")
	token, err := SignToken(otherKey, &JWTClaims{
		Sub:   "user@unheaded.dev",
		Iss:   "https://auth.unheaded.dev",
		Aud:   "unheaded-api",
		Exp:   time.Now().Add(1 * time.Hour).Unix(),
		Roles: []string{"admin"},
	})
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}

	auth := newJWTAuth()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	_, err = auth.Authenticate(context.Background(), req)
	if !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("expected ErrUnauthenticated, got: %v", err)
	}
}

func TestJWTAuthenticator_WrongAudience(t *testing.T) {
	auth := newJWTAuth()
	token := makeTestToken(t, &JWTClaims{
		Sub:   "user@unheaded.dev",
		Iss:   "https://auth.unheaded.dev",
		Aud:   "wrong-audience",
		Exp:   time.Now().Add(1 * time.Hour).Unix(),
		Roles: []string{"admin"},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	_, err := auth.Authenticate(context.Background(), req)
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("expected ErrForbidden, got: %v", err)
	}
}

func TestJWTAuthenticator_WrongIssuer(t *testing.T) {
	auth := newJWTAuth()
	token := makeTestToken(t, &JWTClaims{
		Sub:   "user@unheaded.dev",
		Iss:   "https://evil.example.com",
		Aud:   "unheaded-api",
		Exp:   time.Now().Add(1 * time.Hour).Unix(),
		Roles: []string{"admin"},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	_, err := auth.Authenticate(context.Background(), req)
	if !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("expected ErrUnauthenticated, got: %v", err)
	}
}

func TestJWTAuthenticator_RoleEscalation(t *testing.T) {
	// Authenticator only allows "readonly" role
	auth := NewJWTAuthenticator(JWTConfig{
		SigningKey:    testSigningKey,
		Issuer:       "https://auth.unheaded.dev",
		Audience:     "unheaded-api",
		AllowedRoles: []string{"readonly"},
	})

	token := makeTestToken(t, &JWTClaims{
		Sub:   "user@unheaded.dev",
		Iss:   "https://auth.unheaded.dev",
		Aud:   "unheaded-api",
		Exp:   time.Now().Add(1 * time.Hour).Unix(),
		Roles: []string{"admin"}, // user has admin, but endpoint only allows readonly
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	_, err := auth.Authenticate(context.Background(), req)
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("expected ErrForbidden for role escalation, got: %v", err)
	}
}

func TestJWTAuthenticator_MalformedToken(t *testing.T) {
	auth := newJWTAuth()

	tests := []struct {
		name  string
		token string
	}{
		{"empty string", ""},
		{"no dots", "notajwt"},
		{"one dot", "a.b"},
		{"four dots", "a.b.c.d"},
		{"bad base64 header", "!!!.abc.def"},
		{"bad base64 payload", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.!!!.abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline", nil)
			req.Header.Set("Authorization", "Bearer "+tt.token)

			_, err := auth.Authenticate(context.Background(), req)
			if !errors.Is(err, ErrUnauthenticated) {
				t.Errorf("expected ErrUnauthenticated, got: %v", err)
			}
		})
	}
}

func TestJWTAuthenticator_MissingSubject(t *testing.T) {
	auth := newJWTAuth()
	token := makeTestToken(t, &JWTClaims{
		Sub:   "", // empty subject
		Iss:   "https://auth.unheaded.dev",
		Aud:   "unheaded-api",
		Exp:   time.Now().Add(1 * time.Hour).Unix(),
		Roles: []string{"admin"},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	_, err := auth.Authenticate(context.Background(), req)
	if !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("expected ErrUnauthenticated for missing subject, got: %v", err)
	}
}

func TestJWTAuthenticator_NotBefore(t *testing.T) {
	auth := newJWTAuth()
	// Token that is not valid yet (NBF in the future)
	token := makeTestToken(t, &JWTClaims{
		Sub:   "user@unheaded.dev",
		Iss:   "https://auth.unheaded.dev",
		Aud:   "unheaded-api",
		Exp:   time.Now().Add(2 * time.Hour).Unix(),
		Nbf:   time.Now().Add(1 * time.Hour).Unix(),
		Roles: []string{"admin"},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	_, err := auth.Authenticate(context.Background(), req)
	if !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("expected ErrUnauthenticated for NBF, got: %v", err)
	}
}

func TestJWTAuthenticator_InvalidAuthorizationFormat(t *testing.T) {
	auth := newJWTAuth()

	tests := []struct {
		name   string
		header string
	}{
		{"basic auth", "Basic dXNlcjpwYXNz"},
		{"no scheme", "sometoken"},
		{"empty bearer", "Bearer "},
		{"bearer no space", "Bearertoken"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline", nil)
			req.Header.Set("Authorization", tt.header)

			_, err := auth.Authenticate(context.Background(), req)
			if !errors.Is(err, ErrUnauthenticated) {
				t.Errorf("expected ErrUnauthenticated, got: %v", err)
			}
		})
	}
}

func TestJWTAuthenticator_CaseInsensitiveBearer(t *testing.T) {
	auth := newJWTAuth()
	token := makeTestToken(t, &JWTClaims{
		Sub:   "admin@unheaded.dev",
		Iss:   "https://auth.unheaded.dev",
		Aud:   "unheaded-api",
		Exp:   time.Now().Add(1 * time.Hour).Unix(),
		Roles: []string{"admin"},
	})

	// "bearer" lowercase
	req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline", nil)
	req.Header.Set("Authorization", "bearer "+token)

	identity, err := auth.Authenticate(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error with lowercase bearer, got: %v", err)
	}
	if identity.Subject != "admin@unheaded.dev" {
		t.Errorf("Subject = %q, want %q", identity.Subject, "admin@unheaded.dev")
	}
}

func TestJWTAuthenticator_NoExpiration(t *testing.T) {
	// Token without expiration should be accepted (exp=0 means no expiration check)
	auth := newJWTAuth()
	token := makeTestToken(t, &JWTClaims{
		Sub:   "admin@unheaded.dev",
		Iss:   "https://auth.unheaded.dev",
		Aud:   "unheaded-api",
		Roles: []string{"admin"},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	identity, err := auth.Authenticate(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error for token without exp, got: %v", err)
	}
	if identity.Subject != "admin@unheaded.dev" {
		t.Errorf("Subject = %q, want %q", identity.Subject, "admin@unheaded.dev")
	}
}

func TestSignToken_Roundtrip(t *testing.T) {
	key := []byte("roundtrip-test-key-256-bits-long!")

	original := &JWTClaims{
		Sub:   "timeguru",
		Iss:   "https://auth.unheaded.dev",
		Aud:   "captain",
		Exp:   time.Now().Add(5 * time.Minute).Unix(),
		Iat:   time.Now().Unix(),
		Roles: []string{"service"},
		Extra: map[string]string{"instance": "alpha-01"},
	}

	tokenStr, err := SignToken(key, original)
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}

	// Parse it back
	auth := NewJWTAuthenticator(JWTConfig{
		SigningKey: key,
		Issuer:    "https://auth.unheaded.dev",
		Audience:  "captain",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)

	identity, err := auth.Authenticate(context.Background(), req)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}

	if identity.Subject != original.Sub {
		t.Errorf("Subject = %q, want %q", identity.Subject, original.Sub)
	}
	if !identity.HasRole("service") {
		t.Error("expected service role")
	}
	if identity.Extra["instance"] != "alpha-01" {
		t.Errorf("Extra[instance] = %q, want %q", identity.Extra["instance"], "alpha-01")
	}
}

func TestJWTAuthenticator_UnsupportedAlgorithm(t *testing.T) {
	// Manually craft a token with "alg": "none"
	auth := newJWTAuth()

	// This token is not HS256, it should be rejected
	req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline", nil)
	// base64url of {"alg":"none","typ":"JWT"} = eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0
	// base64url of {"sub":"hacker"} = eyJzdWIiOiJoYWNrZXIifQ
	req.Header.Set("Authorization", "Bearer eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJzdWIiOiJoYWNrZXIifQ.")

	_, err := auth.Authenticate(context.Background(), req)
	if !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("expected ErrUnauthenticated for alg:none, got: %v", err)
	}
}

// --- Middleware Integration Tests ---

func TestMiddleware_JWT_Valid(t *testing.T) {
	auth := newJWTAuth()
	token := makeTestToken(t, &JWTClaims{
		Sub:   "admin@unheaded.dev",
		Iss:   "https://auth.unheaded.dev",
		Aud:   "unheaded-api",
		Exp:   time.Now().Add(1 * time.Hour).Unix(),
		Roles: []string{"admin"},
	})

	var gotIdentity *Identity
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIdentity = IdentityFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := Middleware(auth)(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if gotIdentity == nil {
		t.Fatal("expected identity in context")
	}
	if gotIdentity.Subject != "admin@unheaded.dev" {
		t.Errorf("Subject = %q, want %q", gotIdentity.Subject, "admin@unheaded.dev")
	}
}

func TestMiddleware_JWT_Expired(t *testing.T) {
	auth := newJWTAuth()
	token := makeTestToken(t, &JWTClaims{
		Sub:   "user@unheaded.dev",
		Iss:   "https://auth.unheaded.dev",
		Aud:   "unheaded-api",
		Exp:   time.Now().Add(-1 * time.Hour).Unix(),
		Roles: []string{"admin"},
	})

	handler := Middleware(auth)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for expired token")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}
