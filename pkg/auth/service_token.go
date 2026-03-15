// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package auth

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	// DefaultServiceTokenTTL is the default TTL for service tokens (5 minutes).
	DefaultServiceTokenTTL = 5 * time.Minute

	// ServiceTokenIssuer is the issuer claim for service tokens.
	ServiceTokenIssuer = "unheaded-service-mesh"
)

// ServiceTokenConfig configures service-to-service token generation/validation.
type ServiceTokenConfig struct {
	// SigningKey is the shared HMAC-SHA256 key for signing service tokens.
	// If empty, SigningKeyFile is used.
	SigningKey []byte

	// SigningKeyFile is the path to a file containing the signing key.
	SigningKeyFile string

	// ServiceName is the name of this service (used as "sub" in tokens).
	// If empty, reads from UNHEADED_SERVICE_NAME env var.
	ServiceName string

	// TTL is how long generated tokens are valid. Defaults to 5 minutes.
	TTL time.Duration
}

// ServiceTokenManager generates and validates service-to-service JWT tokens.
type ServiceTokenManager struct {
	signingKey  []byte
	serviceName string
	ttl         time.Duration
}

// NewServiceTokenManager creates a new service token manager.
func NewServiceTokenManager(cfg ServiceTokenConfig) (*ServiceTokenManager, error) {
	// Resolve signing key
	var signingKey []byte
	if len(cfg.SigningKey) > 0 {
		signingKey = cfg.SigningKey
	} else if cfg.SigningKeyFile != "" {
		data, err := os.ReadFile(cfg.SigningKeyFile)
		if err != nil {
			return nil, fmt.Errorf("read signing key file: %w", err)
		}
		signingKey = []byte(strings.TrimSpace(string(data)))
	}

	if len(signingKey) == 0 {
		return nil, fmt.Errorf("no signing key configured")
	}

	// Resolve service name
	serviceName := cfg.ServiceName
	if serviceName == "" {
		serviceName = os.Getenv("UNHEADED_SERVICE_NAME")
	}
	if serviceName == "" {
		return nil, fmt.Errorf("service name not configured (set ServiceName or UNHEADED_SERVICE_NAME)")
	}

	ttl := cfg.TTL
	if ttl == 0 {
		ttl = DefaultServiceTokenTTL
	}

	return &ServiceTokenManager{
		signingKey:  signingKey,
		serviceName: serviceName,
		ttl:         ttl,
	}, nil
}

// GenerateToken creates a short-lived JWT for calling another service.
func (m *ServiceTokenManager) GenerateToken(targetService string) (string, error) {
	now := time.Now()
	claims := &JWTClaims{
		Sub:   m.serviceName,
		Iss:   ServiceTokenIssuer,
		Aud:   targetService,
		Iat:   now.Unix(),
		Exp:   now.Add(m.ttl).Unix(),
		Roles: []string{RoleService},
		Extra: map[string]string{
			"auth_method": "service_token",
			"source":      m.serviceName,
			"target":      targetService,
		},
	}

	return SignToken(m.signingKey, claims)
}

// NewValidator returns a JWTAuthenticator configured to validate service tokens
// for the current service.
func (m *ServiceTokenManager) NewValidator() *JWTAuthenticator {
	return NewJWTAuthenticator(JWTConfig{
		SigningKey:    m.signingKey,
		Issuer:       ServiceTokenIssuer,
		Audience:     m.serviceName,
		AllowedRoles: []string{RoleService},
	})
}
