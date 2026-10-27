// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package auth

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"
)

// Auth modes (AUTH_MODE).
const (
	// ModeEdDSA is the default with AUTH_ENABLED=true: service tokens are
	// EdDSA, verified against trust.d (ADR-103).
	ModeEdDSA = "eddsa"
	// ModeHS256Test is the pre-ADR-103 shared-key mode. It works only in
	// tests (AllowHS256ForTests); a binary refuses it.
	ModeHS256Test = "hs256-test"

	// DefaultKeyDir holds each service's private key, <service>.key.
	DefaultKeyDir = "/etc/unheaded/keys"
	// DefaultTrustDir holds every service's public key, <service>.pub.
	DefaultTrustDir = "/etc/unheaded/trust.d"
)

// Configuration errors from NewServiceMiddleware.
var (
	// ErrSharedKeyForbidden: AUTH_JWT_KEY (the shared HMAC key) is set with
	// auth on. ADR-103 retired it; any holder could mint any token.
	ErrSharedKeyForbidden = errors.New("auth: AUTH_JWT_KEY is forbidden with AUTH_ENABLED=true (ADR-103: the shared HMAC key is retired; use per-service Ed25519 keys and trust.d)")
	// ErrNoCredentials: auth is on but nothing to verify a caller against.
	ErrNoCredentials = errors.New("auth: AUTH_ENABLED=true but no trust directory, OIDC issuer or API key is configured")
)

// ServiceAuthConfig is the configuration for a service's auth setup.
// Read from environment variables to maintain the "configuration from env" pattern.
type ServiceAuthConfig struct {
	// Enabled controls whether auth middleware is active.
	// Read from AUTH_ENABLED env var (default: false for backward compatibility).
	Enabled bool

	// JWTSigningKey is the HMAC-SHA256 key for JWT validation.
	// Read from AUTH_JWT_KEY env var.
	JWTSigningKey []byte

	// APIKeys is the list of valid API keys.
	// Read from AUTH_API_KEYS env var (comma-separated) or AUTH_API_KEY_FILE.
	APIKeys []string // #nosec G117 -- field intentionally named APIKeys; populated at runtime, not committed.

	// APIKeyFile is the path to a file containing API keys.
	APIKeyFile string

	// ServiceName is the name of this service.
	ServiceName string

	// Issuer is the expected JWT issuer.
	Issuer string

	// Audience is the expected JWT audience.
	Audience string

	// Mode is AUTH_MODE: "" or ModeEdDSA, or ModeHS256Test (tests only).
	Mode string

	// KeyFile is this service's private key (AUTH_KEY_FILE, default
	// /etc/unheaded/keys/<service>.key), used to mint its own tokens.
	KeyFile string

	// TrustDir is the directory of trusted service public keys
	// (AUTH_TRUST_DIR, default /etc/unheaded/trust.d). Empty: no service
	// tokens are accepted. A missing directory is tolerated (hosts before
	// unheaded-keygen); a broken one is an error.
	TrustDir string

	// AllowCallers lists the services allowed to call this one
	// (AUTH_ALLOW_CALLERS, comma-separated). Empty denies every service
	// token (403).
	AllowCallers []string

	// OIDCIssuerURL is the issuer whose JWKS verifies user tokens
	// (AUTH_OIDC_ISSUER_URL). Empty: no user tokens.
	OIDCIssuerURL string

	// OIDCMaxLifetime caps a user token's exp - now (AUTH_OIDC_MAX_LIFETIME,
	// a Go duration). Zero: DefaultMaxUserTokenLifetime. Negative: the
	// variable did not parse, and NewServiceMiddleware refuses to start.
	OIDCMaxLifetime time.Duration

	// APIKeyStore names where uhk_ API keys live (AUTH_APIKEY_STORE):
	// "well" (unheaded_app.api_keys, as AUTH_WELL_USER, default app_auth)
	// or "file:<absolute path>". Empty: no uhk_ keys. ADR-103 §5.
	APIKeyStore string
}

// LoadServiceAuthConfig loads auth configuration from environment variables.
func LoadServiceAuthConfig(serviceName string) ServiceAuthConfig {
	cfg := ServiceAuthConfig{
		Enabled:       os.Getenv("AUTH_ENABLED") == "true",
		ServiceName:   serviceName,
		Issuer:        getEnvDefault("AUTH_JWT_ISSUER", "https://auth.unheaded.dev"),
		Audience:      getEnvDefault("AUTH_JWT_AUDIENCE", "unheaded-api"),
		APIKeyFile:    os.Getenv("AUTH_API_KEY_FILE"),
		Mode:          os.Getenv("AUTH_MODE"),
		KeyFile:       getEnvDefault("AUTH_KEY_FILE", filepath.Join(DefaultKeyDir, serviceName+".key")),
		TrustDir:      getEnvDefault("AUTH_TRUST_DIR", DefaultTrustDir),
		OIDCIssuerURL: os.Getenv("AUTH_OIDC_ISSUER_URL"),
		APIKeyStore:   os.Getenv("AUTH_APIKEY_STORE"),
	}
	if v := os.Getenv("AUTH_OIDC_MAX_LIFETIME"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			d = -1
		}
		cfg.OIDCMaxLifetime = d
	}
	if callers := os.Getenv("AUTH_ALLOW_CALLERS"); callers != "" {
		cfg.AllowCallers = splitNonEmpty(callers, ",")
	}

	if key := os.Getenv("AUTH_JWT_KEY"); key != "" {
		cfg.JWTSigningKey = []byte(key)
	}

	if keys := os.Getenv("AUTH_API_KEYS"); keys != "" {
		cfg.APIKeys = append(cfg.APIKeys, splitNonEmpty(keys, ",")...)
	}

	return cfg
}

// SetupMiddleware creates an auth middleware from the config, or returns nil
// if auth is disabled. Nil means "no auth middleware needed."
//
// Enabled never yields nil: with a configuration NewServiceMiddleware
// refuses (no credentials, AUTH_JWT_KEY set, a broken trust dir or key
// file) it logs the error and the middleware refuses every request outside
// /health, /ready and /metrics. Returning nil there would serve the service
// with no auth at all while its operator, and callers keyed on Enabled
// (Cuirass opens its container write API), take it as protected. Callers
// that can exit should use NewServiceMiddleware and refuse to start.
//
// Internal endpoints (/health, /ready, /metrics) are always skipped.
func SetupMiddleware(cfg ServiceAuthConfig) func(http.Handler) http.Handler {
	if !cfg.Enabled {
		return nil
	}
	mw, err := NewServiceMiddleware(cfg)
	if err != nil {
		log.Error().Err(err).Str("service", cfg.ServiceName).
			Msg("auth: refusing every request except /health /ready /metrics (see ADR-103)")
		// An empty MultiAuthenticator refuses every request (401).
		return SkipAuthPaths(Middleware(&MultiAuthenticator{}), "/health", "/ready", "/metrics")
	}
	return mw
}

// NewServiceMiddleware is SetupMiddleware for callers that refuse to start
// on a bad configuration: it returns (nil, nil) with auth off, the
// middleware with auth on, or an error (ErrSharedKeyForbidden,
// ErrNoCredentials, or why a trust dir, OIDC issuer, API key file or API
// key store is unusable). /health, /ready and /metrics are skipped.
func NewServiceMiddleware(cfg ServiceAuthConfig) (func(http.Handler) http.Handler, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	a, err := NewServiceAuthenticator(cfg)
	if err != nil {
		return nil, err
	}
	return SkipAuthPaths(Middleware(a), "/health", "/ready", "/metrics"), nil
}

// NewServiceAuthenticator builds what an enabled service accepts. In
// ModeEdDSA (the default): service tokens verified against TrustDir with
// the AllowCallers allowlist, user tokens from OIDCIssuerURL's JWKS, and
// API keys; AUTH_JWT_KEY is refused. In ModeHS256Test (tests only): the
// pre-ADR-103 HS256 user and service tokens, and API keys.
func NewServiceAuthenticator(cfg ServiceAuthConfig) (Authenticator, error) {
	var authenticators []Authenticator
	switch cfg.Mode {
	case "", ModeEdDSA:
		if len(cfg.JWTSigningKey) > 0 {
			return nil, ErrSharedKeyForbidden
		}
		a, err := eddsaAuthenticators(cfg)
		if err != nil {
			return nil, err
		}
		authenticators = a
	case ModeHS256Test:
		if !hs256Allowed.Load() {
			return nil, fmt.Errorf("auth: AUTH_MODE=%s is for tests only (ADR-103)", ModeHS256Test)
		}
		authenticators = hs256Authenticators(cfg)
	default:
		return nil, fmt.Errorf("auth: unknown AUTH_MODE %q (want %q)", cfg.Mode, ModeEdDSA)
	}

	if len(cfg.APIKeys) > 0 || cfg.APIKeyFile != "" || cfg.APIKeyStore != "" {
		keyCfg := APIKeyConfig{Keys: cfg.APIKeys, KeyFile: cfg.APIKeyFile}
		if cfg.APIKeyStore != "" {
			store, err := OpenAPIKeyStore(context.Background(), cfg.APIKeyStore)
			if err != nil {
				return nil, fmt.Errorf("auth: API key store unusable: %w", err)
			}
			keyCfg.Store = store
		}
		apiKeyAuth, err := NewAPIKeyAuthenticator(keyCfg)
		if err != nil {
			return nil, fmt.Errorf("auth: API keys unusable: %w", err)
		}
		authenticators = append(authenticators, apiKeyAuth)
	}
	if len(authenticators) == 0 {
		return nil, ErrNoCredentials
	}
	return &MultiAuthenticator{Authenticators: authenticators}, nil
}

func eddsaAuthenticators(cfg ServiceAuthConfig) ([]Authenticator, error) {
	var out []Authenticator
	if cfg.TrustDir != "" {
		td, err := NewTrustDir(cfg.TrustDir)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			log.Warn().Str("service", cfg.ServiceName).Str("trust_dir", cfg.TrustDir).
				Msg("auth: no trust directory; service tokens are not accepted (run unheaded-keygen)")
		case err != nil:
			return nil, err
		default:
			v, err := NewEdDSAVerifier(EdDSAVerifierConfig{
				Trust:        td,
				Issuer:       EdDSAServiceIssuer,
				Audience:     cfg.ServiceName,
				AllowCallers: cfg.AllowCallers,
			})
			if err != nil {
				return nil, err
			}
			if len(cfg.AllowCallers) == 0 {
				log.Warn().Str("service", cfg.ServiceName).
					Msg("auth: AUTH_ALLOW_CALLERS is empty; every service token gets 403")
			}
			out = append(out, v)
		}
	}
	if cfg.OIDCIssuerURL != "" {
		if cfg.OIDCMaxLifetime < 0 {
			return nil, errors.New("auth: AUTH_OIDC_MAX_LIFETIME is not a positive Go duration (e.g. 30m)")
		}
		v, err := NewJWKSVerifier(cfg.OIDCIssuerURL, cfg.ServiceName, nil, 0)
		if err != nil {
			return nil, err
		}
		v.SetMaxLifetime(cfg.OIDCMaxLifetime)
		out = append(out, v)
	}
	return out, nil
}

// hs256Authenticators is the pre-ADR-103 shared-key setup, kept for tests.
func hs256Authenticators(cfg ServiceAuthConfig) []Authenticator {
	var out []Authenticator
	if len(cfg.JWTSigningKey) > 0 {
		out = append(out, NewJWTAuthenticator(JWTConfig{
			SigningKey: cfg.JWTSigningKey,
			Issuer:     cfg.Issuer,
			Audience:   cfg.Audience,
		}))
		if cfg.ServiceName != "" {
			out = append(out, NewJWTAuthenticator(JWTConfig{
				SigningKey:   cfg.JWTSigningKey,
				Issuer:       ServiceTokenIssuer,
				Audience:     cfg.ServiceName,
				AllowedRoles: []string{RoleService},
			}))
		}
	}
	return out
}

// ServiceTokenMinter loads this service's key (KeyFile) for minting its own
// tokens to the services it calls.
func (c ServiceAuthConfig) ServiceTokenMinter() (*ServiceTokenMinter, error) {
	return LoadServiceTokenMinter(c.ServiceName, c.KeyFile)
}

// WrapHandler applies the auth middleware to a handler if the middleware is non-nil.
// This is the simplest integration point for services.
func WrapHandler(handler http.Handler, authMiddleware func(http.Handler) http.Handler) http.Handler {
	if authMiddleware == nil {
		return handler
	}
	return authMiddleware(handler)
}

func getEnvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func splitNonEmpty(s, sep string) []string {
	var result []string
	for _, part := range splitString(s, sep) {
		trimmed := trimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// splitString splits s by sep without importing strings in auth.go.
func splitString(s, sep string) []string {
	var parts []string
	for {
		i := indexOf(s, sep)
		if i < 0 {
			parts = append(parts, s)
			break
		}
		parts = append(parts, s[:i])
		s = s[i+len(sep):]
	}
	return parts
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\n' || s[0] == '\r') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
