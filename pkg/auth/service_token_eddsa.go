// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package auth

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"sync"
	"time"
)

const (
	// EdDSAServiceTokenTTL is the lifetime of a minted service token.
	EdDSAServiceTokenTTL = 5 * time.Minute
	// serviceTokenRefresh is how long before exp a cached token is replaced.
	serviceTokenRefresh = 30 * time.Second
	// maxMinterTargets bounds the per-target cache.
	maxMinterTargets = 256
)

// ServiceTokenMinter mints this service's own EdDSA tokens for the services
// it calls (ADR-103 §2): sub = this service, aud = the target, iss =
// EdDSAServiceIssuer, 5-minute lifetime. Tokens are cached per target and
// replaced 30 s before they expire. No token service sits in the request
// path. Safe for concurrent use.
type ServiceTokenMinter struct {
	name  string
	priv  ed25519.PrivateKey
	kid   string
	clock func() time.Time

	mu    sync.Mutex
	cache map[string]mintedToken
}

type mintedToken struct {
	token string
	exp   time.Time
}

// NewServiceTokenMinter returns a minter for service name with its private
// key and that key's kid. A nil clock means time.Now.
func NewServiceTokenMinter(name string, priv ed25519.PrivateKey, kid string, clock func() time.Time) (*ServiceTokenMinter, error) {
	if !ValidKeyName(name) {
		return nil, fmt.Errorf("%w: %q", ErrBadKeyName, name)
	}
	if len(priv) != ed25519.PrivateKeySize {
		return nil, errors.New("auth: service token minter needs an Ed25519 private key")
	}
	pub, ok := priv.Public().(ed25519.PublicKey)
	if !ok || KeyID(pub) != kid {
		return nil, errors.New("auth: service token minter: kid does not name this key")
	}
	if clock == nil {
		clock = time.Now
	}
	return &ServiceTokenMinter{name: name, priv: priv, kid: kid, clock: clock, cache: make(map[string]mintedToken)}, nil
}

// LoadServiceTokenMinter reads the service's key file (LoadPrivateKey, so a
// group/other-readable file is refused) and returns its minter.
func LoadServiceTokenMinter(name, keyFile string) (*ServiceTokenMinter, error) {
	priv, kid, err := LoadPrivateKey(keyFile)
	if err != nil {
		return nil, err
	}
	return NewServiceTokenMinter(name, priv, kid, nil)
}

// Name returns the service name tokens are minted for (their sub).
func (m *ServiceTokenMinter) Name() string { return m.name }

// Mint returns a token for target, from the cache while it has more than
// 30 s left.
func (m *ServiceTokenMinter) Mint(target string) (string, error) {
	if target == "" {
		return "", errors.New("auth: Mint: empty target")
	}
	now := m.clock()
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.cache[target]; ok && now.Before(c.exp.Add(-serviceTokenRefresh)) {
		return c.token, nil
	}
	exp := now.Add(EdDSAServiceTokenTTL)
	tok, err := SignEdDSA(m.priv, m.kid, &JWTClaims{
		Sub:   m.name,
		Iss:   EdDSAServiceIssuer,
		Aud:   target,
		Iat:   now.Unix(),
		Exp:   exp.Unix(),
		Roles: []string{RoleService},
	})
	if err != nil {
		return "", err
	}
	if len(m.cache) >= maxMinterTargets {
		for k, c := range m.cache {
			if !now.Before(c.exp) {
				delete(m.cache, k)
			}
		}
		if len(m.cache) >= maxMinterTargets {
			m.cache = make(map[string]mintedToken)
		}
	}
	m.cache[target] = mintedToken{token: tok, exp: exp}
	return tok, nil
}

// AuthorizationHeader returns "Bearer <token>" for target.
func (m *ServiceTokenMinter) AuthorizationHeader(target string) (string, error) {
	tok, err := m.Mint(target)
	if err != nil {
		return "", err
	}
	return "Bearer " + tok, nil
}
