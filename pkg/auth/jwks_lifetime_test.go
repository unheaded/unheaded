// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package auth

import (
	"context"
	"testing"
	"time"
)

func (fi *fakeIssuer) tokenFor(t *testing.T, kid, aud string, life time.Duration) string {
	t.Helper()
	fi.mu.Lock()
	priv := fi.keys[kid]
	fi.mu.Unlock()
	now := time.Now()
	tok, err := SignEdDSA(priv, kid, &JWTClaims{
		Sub: "stevie", Iss: fi.srv.URL, Aud: aud, Iat: now.Unix(), Exp: now.Add(life).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestJWKSVerifier_DefaultMaxLifetimeRefusesLongTokens(t *testing.T) {
	fi := newFakeIssuer(t)
	kid := fi.addKey(t, true)
	v := newJWKS(t, fi)
	if _, err := v.Authenticate(context.Background(), bearerReq(fi.tokenFor(t, kid, "cuirass", 50*time.Minute))); err != nil {
		t.Fatalf("50-minute token refused: %v", err)
	}
	if _, err := v.Authenticate(context.Background(), bearerReq(fi.tokenFor(t, kid, "cuirass", 24*time.Hour))); err == nil {
		t.Fatal("a day-long user token was accepted; want the default cap to refuse it")
	}
}

func TestJWKSVerifier_SetMaxLifetime(t *testing.T) {
	fi := newFakeIssuer(t)
	kid := fi.addKey(t, true)
	v := newJWKS(t, fi)
	v.SetMaxLifetime(3 * time.Hour)
	if _, err := v.Authenticate(context.Background(), bearerReq(fi.tokenFor(t, kid, "cuirass", 2*time.Hour))); err != nil {
		t.Fatalf("2-hour token under a 3-hour cap refused: %v", err)
	}
	v.SetMaxLifetime(10 * time.Minute)
	if _, err := v.Authenticate(context.Background(), bearerReq(fi.tokenFor(t, kid, "cuirass", 15*time.Minute))); err == nil {
		t.Fatal("15-minute token under a 10-minute cap accepted")
	}
}

func TestLoadServiceAuthConfig_OIDCMaxLifetime(t *testing.T) {
	t.Setenv("AUTH_OIDC_MAX_LIFETIME", "30m")
	if got := LoadServiceAuthConfig("cuirass").OIDCMaxLifetime; got != 30*time.Minute {
		t.Errorf("OIDCMaxLifetime = %v, want 30m", got)
	}
	t.Setenv("AUTH_OIDC_MAX_LIFETIME", "forever")
	cfg := LoadServiceAuthConfig("cuirass")
	cfg.Enabled = true
	cfg.OIDCIssuerURL = "https://auth.example.com"
	if _, err := NewServiceMiddleware(cfg); err == nil {
		t.Error("an unparseable AUTH_OIDC_MAX_LIFETIME started; want a refusal")
	}
}
