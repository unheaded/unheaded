// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// eddsaFixture is a trust dir with service keys generated at run time.
type eddsaFixture struct {
	trust *TrustDir
	priv  map[string]ed25519.PrivateKey
	kid   map[string]string
}

func newEdDSAFixture(t *testing.T, names ...string) *eddsaFixture {
	t.Helper()
	keys, trust := t.TempDir(), t.TempDir()
	f := &eddsaFixture{priv: map[string]ed25519.PrivateKey{}, kid: map[string]string{}}
	for _, n := range names {
		kid, err := GenerateKeyPair(keys, n)
		if err != nil {
			t.Fatal(err)
		}
		priv, lkid, err := LoadPrivateKey(filepath.Join(keys, n+".key"))
		if err != nil || lkid != kid {
			t.Fatalf("load %s: %v", n, err)
		}
		copyFile(t, filepath.Join(keys, n+".pub"), filepath.Join(trust, n+".pub"))
		f.priv[n], f.kid[n] = priv, kid
	}
	td, err := NewTrustDir(trust)
	if err != nil {
		t.Fatal(err)
	}
	f.trust = td
	return f
}

func (f *eddsaFixture) sign(t *testing.T, name string, c *JWTClaims) string {
	t.Helper()
	tok, err := SignEdDSA(f.priv[name], f.kid[name], c)
	if err != nil {
		t.Fatalf("SignEdDSA: %v", err)
	}
	return tok
}

func (f *eddsaFixture) verifier(t *testing.T, audience string, callers ...string) *EdDSAVerifier {
	t.Helper()
	v, err := NewEdDSAVerifier(EdDSAVerifierConfig{
		Trust:        f.trust,
		Issuer:       EdDSAServiceIssuer,
		Audience:     audience,
		AllowCallers: callers,
	})
	if err != nil {
		t.Fatalf("NewEdDSAVerifier: %v", err)
	}
	return v
}

func svcClaims(sub, aud string) *JWTClaims {
	now := time.Now()
	return &JWTClaims{Sub: sub, Iss: EdDSAServiceIssuer, Aud: aud, Iat: now.Unix(), Exp: now.Add(5 * time.Minute).Unix()}
}

func bearerReq(tok string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/x", nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	return r
}

// craft builds a token from a raw header and claims with the given
// signature bytes, for tests that need a token SignEdDSA would never make.
func craft(t *testing.T, header map[string]any, claims *JWTClaims, sig func(input []byte) []byte) string {
	t.Helper()
	h, err := json.Marshal(header)
	if err != nil {
		t.Fatal(err)
	}
	p, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	in := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(p)
	return in + "." + base64.RawURLEncoding.EncodeToString(sig([]byte(in)))
}

func TestSignEdDSA_HeaderHasKidAndAlg(t *testing.T) {
	f := newEdDSAFixture(t, "cuirass")
	tok := f.sign(t, "cuirass", svcClaims("cuirass", "wotan"))
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts", len(parts))
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	var h map[string]string
	if err := json.Unmarshal(raw, &h); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"alg": "EdDSA", "typ": "JWT", "kid": f.kid["cuirass"]}
	if len(h) != len(want) {
		t.Errorf("header %v, want exactly %v", h, want)
	}
	for k, v := range want {
		if h[k] != v {
			t.Errorf("header %s = %q, want %q", k, h[k], v)
		}
	}
}

func TestSignEdDSA_RefusesBadInput(t *testing.T) {
	f := newEdDSAFixture(t, "a")
	if _, err := SignEdDSA(f.priv["a"][:10], f.kid["a"], svcClaims("a", "b")); err == nil {
		t.Error("short private key accepted")
	}
	if _, err := SignEdDSA(f.priv["a"], "", svcClaims("a", "b")); err == nil {
		t.Error("empty kid accepted")
	}
	if _, err := SignEdDSA(f.priv["a"], f.kid["a"], nil); err == nil {
		t.Error("nil claims accepted")
	}
}

func TestVerifyEdDSA_RoundTrip(t *testing.T) {
	f := newEdDSAFixture(t, "cuirass")
	v := f.verifier(t, "wotan", "cuirass")
	c := svcClaims("cuirass", "wotan")
	c.Roles = []string{RoleAdmin} // self-asserted by the caller's key: not granted
	id, err := v.Authenticate(context.Background(), bearerReq(f.sign(t, "cuirass", c)))
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if id.Subject != "cuirass" || !id.HasRole(RoleService) || id.HasRole(RoleAdmin) {
		t.Errorf("identity %+v", id)
	}
	if id.Extra["kid"] != f.kid["cuirass"] || id.Extra["auth_method"] != "eddsa" {
		t.Errorf("identity extra %v", id.Extra)
	}
}

func TestVerifyEdDSA_UnknownKid(t *testing.T) {
	f := newEdDSAFixture(t, "cuirass")
	other := newEdDSAFixture(t, "cuirass") // same name, a key wotan never saw
	v := f.verifier(t, "wotan", "cuirass")
	_, err := v.Authenticate(context.Background(), bearerReq(other.sign(t, "cuirass", svcClaims("cuirass", "wotan"))))
	if !errors.Is(err, ErrUnknownKey) || !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("err %v, want ErrUnknownKey (401)", err)
	}
}

// A trusted key only speaks for the service its file names. Without this
// check any service could mint sub=<anyone> with its own key.
func TestVerifyEdDSA_SubjectMustMatchKeyOwner(t *testing.T) {
	f := newEdDSAFixture(t, "sword", "cuirass")
	v := f.verifier(t, "wotan", "sword", "cuirass")
	_, err := v.Authenticate(context.Background(), bearerReq(f.sign(t, "sword", svcClaims("cuirass", "wotan"))))
	if !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("sword's key minting sub=cuirass: err %v, want ErrUnauthenticated", err)
	}
}

func TestVerifyEdDSA_AlgConfusion(t *testing.T) {
	AllowHS256ForTests() // HS256 on, as a test or misconfiguration might leave it
	f := newEdDSAFixture(t, "cuirass")
	v := f.verifier(t, "wotan", "cuirass")
	c := svcClaims("cuirass", "wotan")
	pub, _ := f.trust.Lookup(f.kid["cuirass"])

	cases := map[string]string{
		// The classic confusion: HMAC keyed with the verifier's public key.
		"HS256 keyed with the public key": craft(t, map[string]any{"alg": "HS256", "typ": "JWT", "kid": f.kid["cuirass"]}, c,
			func(in []byte) []byte { return computeHMACSHA256(pub, in) }),
		"none":    craft(t, map[string]any{"alg": "none", "typ": "JWT", "kid": f.kid["cuirass"]}, c, func([]byte) []byte { return nil }),
		"None":    craft(t, map[string]any{"alg": "None", "kid": f.kid["cuirass"]}, c, func([]byte) []byte { return nil }),
		"empty":   craft(t, map[string]any{"alg": "", "kid": f.kid["cuirass"]}, c, func(in []byte) []byte { return ed25519.Sign(f.priv["cuirass"], in) }),
		"missing": craft(t, map[string]any{"kid": f.kid["cuirass"]}, c, func(in []byte) []byte { return ed25519.Sign(f.priv["cuirass"], in) }),
		"lowercase": craft(t, map[string]any{"alg": "eddsa", "kid": f.kid["cuirass"]}, c,
			func(in []byte) []byte { return ed25519.Sign(f.priv["cuirass"], in) }),
		"ES256": craft(t, map[string]any{"alg": "ES256", "kid": f.kid["cuirass"]}, c, func(in []byte) []byte { return ed25519.Sign(f.priv["cuirass"], in) }),
		"ML-DSA-65": craft(t, map[string]any{"alg": "ML-DSA-65", "kid": f.kid["cuirass"]}, c,
			func(in []byte) []byte { return ed25519.Sign(f.priv["cuirass"], in) }),
	}
	for name, tok := range cases {
		_, err := v.Authenticate(context.Background(), bearerReq(tok))
		if !errors.Is(err, ErrAlgMismatch) {
			t.Errorf("%s: err %v, want ErrAlgMismatch", name, err)
		}
	}
}

func TestVerifyEdDSA_CritHeaderRefused(t *testing.T) {
	f := newEdDSAFixture(t, "cuirass")
	v := f.verifier(t, "wotan", "cuirass")
	tok := craft(t, map[string]any{"alg": "EdDSA", "kid": f.kid["cuirass"], "crit": []string{"b64"}, "b64": false},
		svcClaims("cuirass", "wotan"), func(in []byte) []byte { return ed25519.Sign(f.priv["cuirass"], in) })
	if _, err := v.Authenticate(context.Background(), bearerReq(tok)); err == nil {
		t.Fatal("a token with a crit header was accepted")
	}
}

func TestVerifyEdDSA_TamperedAndMalformed(t *testing.T) {
	f := newEdDSAFixture(t, "cuirass")
	v := f.verifier(t, "wotan", "cuirass")
	good := f.sign(t, "cuirass", svcClaims("cuirass", "wotan"))
	parts := strings.Split(good, ".")

	other := svcClaims("cuirass", "wotan")
	other.Roles = []string{RoleAdmin}
	p, _ := json.Marshal(other)
	swapped := parts[0] + "." + base64.RawURLEncoding.EncodeToString(p) + "." + parts[2]

	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	sig[0] ^= 1
	flipped := parts[0] + "." + parts[1] + "." + base64.RawURLEncoding.EncodeToString(sig)

	for name, tok := range map[string]string{
		"payload swapped":   swapped,
		"signature flipped": flipped,
		"two parts":         parts[0] + "." + parts[1],
		"four parts":        good + ".x",
		"empty signature":   parts[0] + "." + parts[1] + ".",
		"short signature":   parts[0] + "." + parts[1] + "." + base64.RawURLEncoding.EncodeToString(sig[:63]),
		"padded signature":  parts[0] + "." + parts[1] + "." + base64.URLEncoding.EncodeToString(sig),
		"header not json":   base64.RawURLEncoding.EncodeToString([]byte("{")) + "." + parts[1] + "." + parts[2],
		"oversize":          strings.Repeat("a", maxTokenBytes+1),
	} {
		if _, err := v.Authenticate(context.Background(), bearerReq(tok)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := v.Authenticate(context.Background(), httptest.NewRequest(http.MethodGet, "/", nil)); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("no header: %v", err)
	}
}

func TestVerifyEdDSA_AudienceAndExpiry(t *testing.T) {
	f := newEdDSAFixture(t, "cuirass")
	v := f.verifier(t, "wotan", "cuirass")
	now := time.Now()

	wrongAud := svcClaims("cuirass", "sophia")
	if _, err := v.Authenticate(context.Background(), bearerReq(f.sign(t, "cuirass", wrongAud))); !errors.Is(err, ErrForbidden) {
		t.Errorf("wrong aud: err %v, want ErrForbidden", err)
	}

	expired := svcClaims("cuirass", "wotan")
	expired.Iat, expired.Exp = now.Add(-10*time.Minute).Unix(), now.Add(-time.Minute).Unix()
	if _, err := v.Authenticate(context.Background(), bearerReq(f.sign(t, "cuirass", expired))); !errors.Is(err, ErrTokenExpired) {
		t.Errorf("expired: err %v, want ErrTokenExpired", err)
	}

	noExp := svcClaims("cuirass", "wotan")
	noExp.Exp = 0
	if _, err := v.Authenticate(context.Background(), bearerReq(f.sign(t, "cuirass", noExp))); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("missing exp: err %v, want ErrUnauthenticated", err)
	}

	wrongIss := svcClaims("cuirass", "wotan")
	wrongIss.Iss = "someone-else"
	if _, err := v.Authenticate(context.Background(), bearerReq(f.sign(t, "cuirass", wrongIss))); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("wrong iss: err %v, want ErrUnauthenticated", err)
	}

	// A key holder cannot mint a token that outlives MaxLifetime.
	long := svcClaims("cuirass", "wotan")
	long.Exp = now.Add(24 * time.Hour).Unix()
	if _, err := v.Authenticate(context.Background(), bearerReq(f.sign(t, "cuirass", long))); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("24h token: err %v, want ErrUnauthenticated", err)
	}
}

func TestNewEdDSAVerifier_RefusesOpenConfig(t *testing.T) {
	f := newEdDSAFixture(t, "a")
	for name, cfg := range map[string]EdDSAVerifierConfig{
		"no trust":    {Issuer: EdDSAServiceIssuer, Audience: "x"},
		"no audience": {Trust: f.trust, Issuer: EdDSAServiceIssuer},
		"no issuer":   {Trust: f.trust, Audience: "x"},
	} {
		if _, err := NewEdDSAVerifier(cfg); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestVerifyEdDSA_RevocationByReload(t *testing.T) {
	f := newEdDSAFixture(t, "cuirass")
	v := f.verifier(t, "wotan", "cuirass")
	tok := f.sign(t, "cuirass", svcClaims("cuirass", "wotan"))
	if _, err := v.Authenticate(context.Background(), bearerReq(tok)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(f.trust.Path(), "cuirass.pub")); err != nil {
		t.Fatal(err)
	}
	if err := f.trust.Reload(); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Authenticate(context.Background(), bearerReq(tok)); !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("revoked key: err %v, want ErrUnknownKey", err)
	}
}

// HS256 verification is off in production builds; only tests turn it on.
func TestVerify_HS256OnlyUnderTestBuildTag(t *testing.T) {
	prev := hs256Allowed.Swap(false)
	defer hs256Allowed.Store(prev)

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	tok, err := SignToken(key, &JWTClaims{Sub: "x", Exp: time.Now().Add(time.Minute).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	j := NewJWTAuthenticator(JWTConfig{SigningKey: key})
	if _, err := j.Authenticate(context.Background(), bearerReq(tok)); !errors.Is(err, ErrAlgMismatch) {
		t.Fatalf("HS256 without the test opt-in: err %v, want ErrAlgMismatch", err)
	}

	AllowHS256ForTests()
	if _, err := j.Authenticate(context.Background(), bearerReq(tok)); err != nil {
		t.Fatalf("HS256 with the test opt-in: %v", err)
	}
}

// AllowHS256ForTests turns a security gate off, so only test files may
// call it. This walks the module for any other caller.
func TestAllowHS256ForTests_OnlyCalledFromTests(t *testing.T) {
	root := filepath.Join("..", "..")
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Skip("module root not found")
	}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "vendor", "target", ".agent-scratch", ".claude":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(p) // #nosec G304 -- walking our own module in a test
		if err != nil {
			return err
		}
		if strings.Contains(string(b), "AllowHS256ForTests()") && p != filepath.Join(root, "pkg", "auth", "jwt.go") {
			t.Errorf("%s calls AllowHS256ForTests outside a _test.go file", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
