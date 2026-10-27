// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package auth

import "testing"

// S-IRON-GATE Phase 8: the JWKS verifier behind AUTH_OIDC_ISSUER_URL trusts
// the deployment CA under UNHEADED_TLS_ENABLED, so TLS without a CA is a
// startup refusal, not a verifier that can never fetch keys.
func TestOIDCVerifier_TLSWithoutCARefusesStart(t *testing.T) {
	t.Setenv("UNHEADED_TLS_ENABLED", "true")
	t.Setenv("UNHEADED_TLS_CA_CERT", "")
	cfg := ServiceAuthConfig{Enabled: true, ServiceName: "cuirass", OIDCIssuerURL: "https://auth:19020"}
	if _, err := NewServiceMiddleware(cfg); err == nil {
		t.Fatal("UNHEADED_TLS_ENABLED without a CA: the OIDC verifier started")
	}
}
