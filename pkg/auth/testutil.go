// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package auth

import (
	"time"
)

// testSigningKeyForTestUtil is the shared secret for test token generation.
var testSigningKeyForTestUtil = []byte("test-hmac-secret-key-for-unheaded-kingdom-256bit")

// AdminToken generates a valid admin JWT token for testing.
// The token is valid for 1 hour and includes admin, operator, and observer roles.
func AdminToken() string {
	token, err := SignToken(testSigningKeyForTestUtil, &JWTClaims{
		Sub:   "admin-test@unheaded.dev",
		Iss:   "https://auth.unheaded.dev",
		Aud:   "unheaded-api",
		Exp:   time.Now().Add(1 * time.Hour).Unix(),
		Iat:   time.Now().Unix(),
		Roles: []string{RoleAdmin, RoleOperator, RoleObserver},
		Extra: map[string]string{"tier": "admin"},
	})
	if err != nil {
		panic("failed to create admin token: " + err.Error())
	}
	return token
}

// ObserverToken generates a valid observer JWT token for testing.
// The token is valid for 1 hour and includes the observer role.
func ObserverToken() string {
	token, err := SignToken(testSigningKeyForTestUtil, &JWTClaims{
		Sub:   "observer-test@unheaded.dev",
		Iss:   "https://auth.unheaded.dev",
		Aud:   "unheaded-api",
		Exp:   time.Now().Add(1 * time.Hour).Unix(),
		Iat:   time.Now().Unix(),
		Roles: []string{RoleObserver},
		Extra: map[string]string{"tier": "observer"},
	})
	if err != nil {
		panic("failed to create observer token: " + err.Error())
	}
	return token
}

// OperatorToken generates a valid operator JWT token for testing.
// The token is valid for 1 hour and includes operator and observer roles.
func OperatorToken() string {
	token, err := SignToken(testSigningKeyForTestUtil, &JWTClaims{
		Sub:   "operator-test@unheaded.dev",
		Iss:   "https://auth.unheaded.dev",
		Aud:   "unheaded-api",
		Exp:   time.Now().Add(1 * time.Hour).Unix(),
		Iat:   time.Now().Unix(),
		Roles: []string{RoleOperator, RoleObserver},
		Extra: map[string]string{"tier": "operator"},
	})
	if err != nil {
		panic("failed to create operator token: " + err.Error())
	}
	return token
}

// ServiceToken generates a valid service-to-service JWT token for testing.
// The token is valid for 5 minutes and includes the service role.
// The name parameter specifies the calling service name.
func ServiceToken(name string) string {
	token, err := SignToken(testSigningKeyForTestUtil, &JWTClaims{
		Sub:   name,
		Iss:   ServiceTokenIssuer,
		Aud:   "destination",
		Exp:   time.Now().Add(5 * time.Minute).Unix(),
		Iat:   time.Now().Unix(),
		Roles: []string{RoleService},
		Extra: map[string]string{
			"auth_method": "service_token",
			"source":      name,
			"target":      "destination",
		},
	})
	if err != nil {
		panic("failed to create service token: " + err.Error())
	}
	return token
}

// InvalidToken generates an expired JWT token for testing auth failure.
func InvalidToken() string {
	token, err := SignToken(testSigningKeyForTestUtil, &JWTClaims{
		Sub:   "invalid-user@unheaded.dev",
		Iss:   "https://auth.unheaded.dev",
		Aud:   "unheaded-api",
		Exp:   time.Now().Add(-1 * time.Hour).Unix(), // Already expired
		Iat:   time.Now().Add(-2 * time.Hour).Unix(),
		Roles: []string{RoleAdmin},
	})
	if err != nil {
		panic("failed to create invalid token: " + err.Error())
	}
	return token
}

// MalformedToken returns a deliberately malformed JWT string for testing.
func MalformedToken() string {
	return "eyJhbGciOiJub25lIn0.invalid.fake"
}

// WrongSignatureToken generates a token signed with a different key.
func WrongSignatureToken() string {
	wrongKey := []byte("completely-different-secret-not-the-real-key")
	token, err := SignToken(wrongKey, &JWTClaims{
		Sub:   "hacker@example.com",
		Iss:   "https://auth.unheaded.dev",
		Aud:   "unheaded-api",
		Exp:   time.Now().Add(1 * time.Hour).Unix(),
		Roles: []string{RoleAdmin},
	})
	if err != nil {
		panic("failed to create wrong-sig token: " + err.Error())
	}
	return token
}
