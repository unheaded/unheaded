// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package auth

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// FuzzEdDSAParse: any presented token never panics and never verifies.
// The trusted key is generated at run time and never signs a seed, so no
// input can verify short of forging Ed25519.
func FuzzEdDSAParse(f *testing.F) {
	keys, trust := f.TempDir(), f.TempDir()
	kid, err := GenerateKeyPair(keys, "cuirass")
	if err != nil {
		f.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(keys, "cuirass.pub"))
	if err != nil {
		f.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(trust, "cuirass.pub"), b, 0o444); err != nil {
		f.Fatal(err)
	}
	td, err := NewTrustDir(trust)
	if err != nil {
		f.Fatal(err)
	}
	v, err := NewEdDSAVerifier(EdDSAVerifierConfig{Trust: td, Issuer: EdDSAServiceIssuer, Audience: "wotan", AllowCallers: []string{"cuirass"}})
	if err != nil {
		f.Fatal(err)
	}

	// Seeds: tokens of the right shape signed by a key nobody trusts.
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	now := time.Now()
	claims, _ := json.Marshal(&JWTClaims{Sub: "cuirass", Iss: EdDSAServiceIssuer, Aud: "wotan", Iat: now.Unix(), Exp: now.Add(time.Minute).Unix()})
	for _, hdr := range []string{
		`{"alg":"EdDSA","typ":"JWT","kid":"` + kid + `"}`,
		`{"alg":"none","kid":"` + kid + `"}`,
		`{"alg":"HS256","kid":"` + kid + `"}`,
		`{"alg":"EdDSA","kid":"` + kid + `","crit":["x"]}`,
		`{"alg":"EdDSA"}`,
		`[]`,
	} {
		in := base64.RawURLEncoding.EncodeToString([]byte(hdr)) + "." + base64.RawURLEncoding.EncodeToString(claims)
		f.Add(in + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(other, []byte(in))))
	}
	f.Add("")
	f.Add("..")
	f.Add("a.b.c.d")

	f.Fuzz(func(t *testing.T, tok string) {
		r := bearerReq(tok)
		if id, err := v.Authenticate(context.Background(), r); err == nil || id != nil {
			t.Fatalf("fuzzed token verified: %q", tok)
		}
		// The raw parser too, with a lookup that knows every kid.
		pub, _ := td.Lookup(kid)
		if _, _, _, err := verifyEdDSA(tok, func(string) (ed25519.PublicKey, string, bool) { return pub, "cuirass", true }); err == nil {
			t.Fatalf("verifyEdDSA accepted %q", tok)
		}
	})
}

// FuzzJWKSDocument: any JWKS body never panics; oversize is refused.
func FuzzJWKSDocument(f *testing.F) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	x := base64.RawURLEncoding.EncodeToString(pub)
	f.Add([]byte(`{"keys":[{"kty":"OKP","crv":"Ed25519","kid":"k1","x":"` + x + `"}]}`))
	f.Add([]byte(`{"keys":[{"kty":"RSA","kid":"r","n":"AQAB","e":"AQAB"}]}`))
	f.Add([]byte(`{"keys":[{"kty":"OKP","crv":"Ed25519","kid":"k1","x":"` + x + `"},{"kty":"OKP","crv":"Ed25519","kid":"k1","x":"` + x + `"}]}`))
	f.Add([]byte(`{"keys":null}`))
	f.Add([]byte(`{"keys":[1,"a",null,{}]}`))
	f.Add([]byte(`not json`))

	f.Fuzz(func(t *testing.T, doc []byte) {
		body, err := readJSONDoc(bytes.NewReader(doc))
		if len(doc) > maxJWKSBytes {
			if err == nil {
				t.Fatalf("a %d-byte document was read", len(doc))
			}
			return
		}
		if err != nil {
			t.Fatalf("readJSONDoc on %d bytes: %v", len(doc), err)
		}
		var set struct {
			Keys []json.RawMessage `json:"keys"`
		}
		if json.Unmarshal(body, &set) != nil {
			return
		}
		keys, err := parseJWKS(set.Keys)
		if err != nil {
			return
		}
		for kid, k := range keys {
			if kid == "" || len(k) != ed25519.PublicKeySize {
				t.Fatalf("parseJWKS kept kid %q with a %d-byte key", kid, len(k))
			}
		}
	})
}

// FuzzTrustDirFilenames: any file name in trust.d never panics, and a key
// it loads is always bound to a valid service name.
func FuzzTrustDirFilenames(f *testing.F) {
	keys := f.TempDir()
	if _, err := GenerateKeyPair(keys, "seed"); err != nil {
		f.Fatal(err)
	}
	pub, err := os.ReadFile(filepath.Join(keys, "seed.pub"))
	if err != nil {
		f.Fatal(err)
	}
	for _, n := range []string{"a.pub", "A.pub", ".pub", "..pub", "a.b.pub", "a.key", "-a.pub", "a-.pub", "x.pub.key", "é.pub", "a b.pub"} {
		f.Add(n, true)
	}
	f.Add("junk.txt", false)

	f.Fuzz(func(t *testing.T, name string, validPub bool) {
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\x00") || len(name) > 200 {
			return
		}
		dir := t.TempDir()
		body := []byte("not a key")
		if validPub {
			body = pub
		}
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o444); err != nil {
			return // a name the filesystem refuses
		}
		td, err := NewTrustDir(dir)
		if err != nil {
			return
		}
		for _, kid := range td.Kids() {
			if _, owner, ok := td.LookupName(kid); !ok || !ValidKeyName(owner) {
				t.Fatalf("file %q bound kid %s to %q", name, kid, owner)
			}
		}
	})
}
