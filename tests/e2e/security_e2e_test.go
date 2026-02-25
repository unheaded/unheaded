// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package e2e provides security-focused E2E tests for the Unheaded platform.
//
// Tests verify:
//   - Unauthenticated requests are rejected (401)
//   - Invalid tokens are rejected (401)
//   - API key rate limiting returns 429
//   - mTLS certificate verification
//
// All tests are self-contained using httptest.Server and in-process mocks.
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/auth"
	"unheaded/pkg/health"
	utls "unheaded/pkg/tls"
)

// ============================================================================
// SECURITY E2E TEST HARNESS
// ============================================================================

// securityTestHarness provides a server with auth middleware configured.
type securityTestHarness struct {
	t          *testing.T
	server     *httptest.Server
	signingKey []byte
}

// newSecurityTestHarness creates a test server with auth middleware.
func newSecurityTestHarness(t *testing.T) *securityTestHarness {
	t.Helper()

	signingKey := []byte("security-e2e-test-key-must-be-32b!")

	jwtAuth := auth.NewJWTAuthenticator(auth.JWTConfig{
		SigningKey: signingKey,
		Issuer:    "unheaded-security-test",
	})

	mux := http.NewServeMux()

	// Unauthenticated endpoints.
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		health.WriteHealthy(w, "security-test", "0.1.0")
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
	})

	// Protected endpoints with auth middleware.
	authMiddleware := auth.SkipAuthPaths(
		auth.Middleware(jwtAuth),
		"/health", "/ready", "/metrics",
	)

	protectedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity := auth.IdentityFromContext(r.Context())
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"path":    r.URL.Path,
			"subject": identity.Subject,
			"roles":   identity.Roles,
		}
		json.NewEncoder(w).Encode(resp)
	})

	mux.Handle("/api/", authMiddleware(protectedHandler))

	server := httptest.NewServer(mux)

	return &securityTestHarness{
		t:          t,
		server:     server,
		signingKey: signingKey,
	}
}

// generateValidToken creates a valid JWT with the given subject and roles.
func (h *securityTestHarness) generateValidToken(subject string, roles []string) string {
	h.t.Helper()
	claims := &auth.JWTClaims{
		Sub:   subject,
		Iss:   "unheaded-security-test",
		Iat:   time.Now().Unix(),
		Exp:   time.Now().Add(5 * time.Minute).Unix(),
		Roles: roles,
	}
	token, err := auth.SignToken(h.signingKey, claims)
	if err != nil {
		h.t.Fatalf("Failed to sign token: %v", err)
	}
	return token
}

// generateExpiredToken creates a JWT that has already expired.
func (h *securityTestHarness) generateExpiredToken(subject string) string {
	h.t.Helper()
	claims := &auth.JWTClaims{
		Sub:   subject,
		Iss:   "unheaded-security-test",
		Iat:   time.Now().Add(-10 * time.Minute).Unix(),
		Exp:   time.Now().Add(-5 * time.Minute).Unix(),
		Roles: []string{auth.RoleAdmin},
	}
	token, err := auth.SignToken(h.signingKey, claims)
	if err != nil {
		h.t.Fatalf("Failed to sign expired token: %v", err)
	}
	return token
}

// Cleanup tears down test resources.
func (h *securityTestHarness) Cleanup() {
	h.server.Close()
}

// ============================================================================
// SECURITY E2E TESTS
// ============================================================================

// TestSecurity_UnauthenticatedRejected verifies that requests without any
// authentication credentials receive 401 Unauthorized.
func TestSecurity_UnauthenticatedRejected(t *testing.T) {
	h := newSecurityTestHarness(t)
	defer h.Cleanup()

	paths := []string{
		"/api/v1/status",
		"/api/v1/traces",
		"/api/v1/services",
		"/api/v1/config",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			resp, err := http.Get(h.server.URL + path)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("Expected 401 for unauthenticated request to %s, got %d", path, resp.StatusCode)
			}
		})
	}
}

// TestSecurity_InvalidTokenRejected verifies that requests with an invalid
// or malformed JWT token receive 401 Unauthorized.
func TestSecurity_InvalidTokenRejected(t *testing.T) {
	h := newSecurityTestHarness(t)
	defer h.Cleanup()

	testCases := []struct {
		name      string
		authValue string
	}{
		{"empty_bearer", "Bearer "},
		{"malformed_jwt", "Bearer not.a.jwt"},
		{"wrong_prefix", "Basic dXNlcjpwYXNz"},
		{"garbage", "Bearer " + strings.Repeat("x", 100)},
		{"too_few_parts", "Bearer aaa.bbb"},
		{"too_many_parts", "Bearer aaa.bbb.ccc.ddd"},
		{"wrong_signing_key", func() string {
			wrongKey := []byte("wrong-key-that-does-not-match-32!")
			claims := &auth.JWTClaims{
				Sub:   "attacker",
				Iss:   "unheaded-security-test",
				Iat:   time.Now().Unix(),
				Exp:   time.Now().Add(5 * time.Minute).Unix(),
				Roles: []string{auth.RoleAdmin},
			}
			token, _ := auth.SignToken(wrongKey, claims)
			return "Bearer " + token
		}()},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, h.server.URL+"/api/v1/status", nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Header.Set("Authorization", tc.authValue)

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("Expected 401 for %s, got %d", tc.name, resp.StatusCode)
			}
		})
	}
}

// TestSecurity_ExpiredTokenRejected verifies that expired JWT tokens are rejected.
func TestSecurity_ExpiredTokenRejected(t *testing.T) {
	h := newSecurityTestHarness(t)
	defer h.Cleanup()

	token := h.generateExpiredToken("expired-user")

	req, err := http.NewRequest(http.MethodGet, h.server.URL+"/api/v1/status", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected 401 for expired token, got %d", resp.StatusCode)
	}
}

// TestSecurity_ValidTokenAccepted verifies that a valid JWT token grants access.
func TestSecurity_ValidTokenAccepted(t *testing.T) {
	h := newSecurityTestHarness(t)
	defer h.Cleanup()

	token := h.generateValidToken("valid-user", []string{auth.RoleAdmin})

	req, err := http.NewRequest(http.MethodGet, h.server.URL+"/api/v1/status", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 for valid token, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)

	if body["subject"] != "valid-user" {
		t.Errorf("Expected subject 'valid-user', got %v", body["subject"])
	}
}

// TestSecurity_HealthEndpointsNoAuth verifies that health/ready/metrics
// endpoints are accessible without authentication.
func TestSecurity_HealthEndpointsNoAuth(t *testing.T) {
	h := newSecurityTestHarness(t)
	defer h.Cleanup()

	paths := []string{"/health", "/ready"}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			resp, err := http.Get(h.server.URL + path)
			if err != nil {
				t.Fatalf("Request to %s failed: %v", path, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected 200 for %s without auth, got %d", path, resp.StatusCode)
			}
		})
	}
}

// TestSecurity_RateLimiting verifies that the API key authenticator returns 429
// after exceeding the rate limit threshold.
func TestSecurity_RateLimiting(t *testing.T) {
	const rateLimit = 5 // requests per minute

	// Create API key authenticator with rate limiting.
	apiKeyAuth, err := auth.NewAPIKeyAuthenticator(auth.APIKeyConfig{
		Keys:               []string{"test-api-key-12345"},
		RateLimitPerMinute: rateLimit,
	})
	if err != nil {
		t.Fatalf("Failed to create API key authenticator: %v", err)
	}

	mux := http.NewServeMux()
	authMiddleware := auth.Middleware(apiKeyAuth)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.Handle("/api/", authMiddleware(handler))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := &http.Client{}
	successCount := 0
	rateLimitedCount := 0

	// Send rateLimit + 5 requests to exceed the limit.
	for i := 0; i < rateLimit+5; i++ {
		req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/data", nil)
		req.Header.Set("X-API-Key", "test-api-key-12345")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request %d failed: %v", i, err)
		}
		resp.Body.Close()

		switch resp.StatusCode {
		case http.StatusOK:
			successCount++
		case http.StatusTooManyRequests:
			rateLimitedCount++
		default:
			t.Errorf("Request %d: unexpected status %d", i, resp.StatusCode)
		}
	}

	if successCount == 0 {
		t.Error("Expected at least some successful requests")
	}
	if rateLimitedCount == 0 {
		t.Error("Expected some requests to be rate limited (429)")
	}

	t.Logf("Rate limiting: %d successful, %d rate-limited (limit=%d/min)",
		successCount, rateLimitedCount, rateLimit)
}

// TestSecurity_mTLS_RequiredBetweenServices verifies that connections fail
// when a server requires mTLS but the client does not present a certificate.
func TestSecurity_mTLS_RequiredBetweenServices(t *testing.T) {
	// Create a CA and issue certs using the Unheaded TLS package.
	ca, err := utls.NewCA()
	if err != nil {
		t.Fatalf("Failed to create CA: %v", err)
	}

	// Issue a server certificate.
	serverCert, err := ca.IssueCert(utls.CertRequest{
		ServiceName: "test-server",
		TTL:         1 * time.Hour,
	})
	if err != nil {
		t.Fatalf("Failed to issue server cert: %v", err)
	}

	// Issue a client certificate.
	clientCert, err := ca.IssueCert(utls.CertRequest{
		ServiceName: "test-client",
		TTL:         1 * time.Hour,
	})
	if err != nil {
		t.Fatalf("Failed to issue client cert: %v", err)
	}

	// Verify certs through the CA.
	if err := ca.VerifyCert(serverCert.CertPEM); err != nil {
		t.Fatalf("Server cert verification failed: %v", err)
	}
	if err := ca.VerifyCert(clientCert.CertPEM); err != nil {
		t.Fatalf("Client cert verification failed: %v", err)
	}

	// Create an mTLS server using the Unheaded TLS config builder.
	serverTLSConfig, err := utls.NewServerTLSConfig(utls.ServerConfig{
		CertPEM:           serverCert.CertPEM,
		KeyPEM:            serverCert.KeyPEM,
		CAPEM:             ca.CertPEM(),
		RequireClientCert: true,
	})
	if err != nil {
		t.Fatalf("Failed to build server TLS config: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/data", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	server := httptest.NewUnstartedServer(mux)
	server.TLS = serverTLSConfig
	server.StartTLS()
	defer server.Close()

	// Test 1: Client WITHOUT certificate -- should fail.
	t.Run("no_client_cert", func(t *testing.T) {
		noClientCert, err := utls.NewTLSHTTPClient(utls.ClientConfig{
			CAPEM:   ca.CertPEM(),
			Timeout: 5 * time.Second,
		})
		if err != nil {
			t.Fatalf("Failed to create no-cert client: %v", err)
		}

		_, err = noClientCert.Get(server.URL + "/api/v1/data")
		if err == nil {
			t.Error("Expected connection to fail without client certificate")
		} else {
			t.Logf("Correctly rejected (no client cert): %v", err)
		}
	})

	// Test 2: Client WITH valid certificate -- should succeed.
	t.Run("valid_client_cert", func(t *testing.T) {
		validClient, err := utls.NewTLSHTTPClient(utls.ClientConfig{
			CertPEM: clientCert.CertPEM,
			KeyPEM:  clientCert.KeyPEM,
			CAPEM:   ca.CertPEM(),
			Timeout: 5 * time.Second,
		})
		if err != nil {
			t.Fatalf("Failed to create mTLS client: %v", err)
		}

		resp, err := validClient.Get(server.URL + "/api/v1/data")
		if err != nil {
			t.Fatalf("mTLS request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}
	})

	// Test 3: Client with cert signed by wrong CA -- should fail.
	t.Run("wrong_ca_cert", func(t *testing.T) {
		wrongCA, err := utls.NewCA()
		if err != nil {
			t.Fatalf("Failed to create wrong CA: %v", err)
		}

		wrongCert, err := wrongCA.IssueCert(utls.CertRequest{
			ServiceName: "attacker-client",
			TTL:         1 * time.Hour,
		})
		if err != nil {
			t.Fatalf("Failed to issue wrong cert: %v", err)
		}

		wrongClient, err := utls.NewTLSHTTPClient(utls.ClientConfig{
			CertPEM: wrongCert.CertPEM,
			KeyPEM:  wrongCert.KeyPEM,
			CAPEM:   ca.CertPEM(), // Trust the right CA to verify server
			Timeout: 5 * time.Second,
		})
		if err != nil {
			t.Fatalf("Failed to create wrong-CA client: %v", err)
		}

		_, err = wrongClient.Get(server.URL + "/api/v1/data")
		if err == nil {
			t.Error("Expected connection to fail with cert from wrong CA")
		} else {
			t.Logf("Correctly rejected (wrong CA cert): %v", err)
		}
	})
}

// TestSecurity_AuthMiddlewareChain verifies that multiple authenticators can be
// chained together with the MultiAuthenticator.
func TestSecurity_AuthMiddlewareChain(t *testing.T) {
	signingKey := []byte("multi-auth-chain-test-key-32bits!")

	// Create JWT authenticator.
	jwtAuth := auth.NewJWTAuthenticator(auth.JWTConfig{
		SigningKey: signingKey,
		Issuer:    "unheaded-multi-auth-test",
	})

	// Create API key authenticator.
	apiKeyAuth, err := auth.NewAPIKeyAuthenticator(auth.APIKeyConfig{
		Keys: []string{"multi-auth-test-key"},
	})
	if err != nil {
		t.Fatalf("Failed to create API key auth: %v", err)
	}

	// Create multi-authenticator.
	multiAuth := &auth.MultiAuthenticator{
		Authenticators: []auth.Authenticator{jwtAuth, apiKeyAuth},
	}

	mux := http.NewServeMux()
	authMiddleware := auth.Middleware(multiAuth)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity := auth.IdentityFromContext(r.Context())
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"subject":     identity.Subject,
			"auth_method": identity.Extra["auth_method"],
		})
	})

	mux.Handle("/api/", authMiddleware(handler))
	server := httptest.NewServer(mux)
	defer server.Close()

	client := &http.Client{}

	// Test: JWT auth.
	t.Run("jwt_auth", func(t *testing.T) {
		claims := &auth.JWTClaims{
			Sub: "jwt-user", Iss: "unheaded-multi-auth-test",
			Iat: time.Now().Unix(), Exp: time.Now().Add(5 * time.Minute).Unix(),
			Roles: []string{auth.RoleAdmin},
		}
		token, _ := auth.SignToken(signingKey, claims)

		req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/data", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}
	})

	// Test: API key auth.
	t.Run("apikey_auth", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/data", nil)
		req.Header.Set("X-API-Key", "multi-auth-test-key")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}
	})

	// Test: No auth -- should fail.
	t.Run("no_auth", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/data", nil)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected 401, got %d", resp.StatusCode)
		}
	})
}

// TestSecurity_ServiceTokenGeneration verifies that service-to-service tokens
// can be generated and validated.
func TestSecurity_ServiceTokenGeneration(t *testing.T) {
	signingKey := []byte("service-token-test-key-32-bytes!!")

	// Create a service token manager for "timeguru".
	manager, err := auth.NewServiceTokenManager(auth.ServiceTokenConfig{
		SigningKey:   signingKey,
		ServiceName: "timeguru",
		TTL:         5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("Failed to create service token manager: %v", err)
	}

	// Generate a token for calling "captain".
	token, err := manager.GenerateToken("captain")
	if err != nil {
		t.Fatalf("Failed to generate service token: %v", err)
	}

	if token == "" {
		t.Fatal("Token should not be empty")
	}

	// Validate with a validator configured for "captain".
	captainManager, err := auth.NewServiceTokenManager(auth.ServiceTokenConfig{
		SigningKey:   signingKey,
		ServiceName: "captain",
	})
	if err != nil {
		t.Fatalf("Failed to create captain manager: %v", err)
	}

	validator := captainManager.NewValidator()

	// Create a request with the token.
	req, _ := http.NewRequest(http.MethodGet, "http://captain:8001/api/v1/status", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	identity, err := validator.Authenticate(context.Background(), req)
	if err != nil {
		t.Fatalf("Token validation failed: %v", err)
	}

	if identity.Subject != "timeguru" {
		t.Errorf("Expected subject 'timeguru', got %q", identity.Subject)
	}
	if !identity.HasRole(auth.RoleService) {
		t.Error("Expected service role")
	}

	t.Logf("Service token validated: %s -> captain (roles: %v)", identity.Subject, identity.Roles)
}

// TestSecurity_ConcurrentAuthRequests verifies that the auth middleware
// handles concurrent requests without races.
func TestSecurity_ConcurrentAuthRequests(t *testing.T) {
	h := newSecurityTestHarness(t)
	defer h.Cleanup()

	token := h.generateValidToken("concurrent-user", []string{auth.RoleAdmin})

	const numRequests = 100
	errCh := make(chan error, numRequests)

	var wg sync.WaitGroup
	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			req, _ := http.NewRequest(http.MethodGet,
				h.server.URL+fmt.Sprintf("/api/v1/resource/%d", idx), nil)
			req.Header.Set("Authorization", "Bearer "+token)

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				errCh <- fmt.Errorf("request %d: %w", idx, err)
				return
			}
			resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				errCh <- fmt.Errorf("request %d: expected 200, got %d", idx, resp.StatusCode)
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Error(err)
	}

	t.Logf("Completed %d concurrent authenticated requests", numRequests)
}
