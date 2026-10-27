// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

// Command gen writes the Lich auth seeds for pkg/auth's verifiers
// (tomb/lich/IRON-GATE-HOOKS.md): ten malformed EdDSA service tokens into
// tomb/lich/seeds/auth/tokens/ (gitignored), and with -jwks the five hostile JWKS
// documents (jwks-*.json, committed).
//
//	go run ./tomb/lich/seeds/auth/gen           # tokens/token-*.txt
//	go run ./tomb/lich/seeds/auth/gen -jwks     # also rewrite jwks-*.json
//
// The tokens are built at run time, not committed: they have the shape of
// real JWTs, so gitleaks' jwt rule would flag them, and a fixture that looks
// like a credential is built when it is needed, never committed. Every
// signature comes from an Ed25519 key drawn at run time and dropped when the
// program exits: no seed is signed by a key any deployment trusts, and no
// private key is written anywhere. Re-running gives fresh signatures; the
// shapes stay the same.
package main

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var b64 = base64.RawURLEncoding

func kidOf(pub ed25519.PublicKey) string {
	s := sha256.Sum256(pub)
	return hex.EncodeToString(s[:])[:16]
}

func claims(sub, aud string, life time.Duration, extra map[string]any) string {
	now := time.Now().Unix()
	c := map[string]any{"iss": "unheaded", "sub": sub, "aud": aud, "iat": now, "exp": now + int64(life/time.Second)}
	for k, v := range extra {
		c[k] = v
	}
	b, err := json.Marshal(c)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func signed(priv ed25519.PrivateKey, header, payload string) string {
	in := b64.EncodeToString([]byte(header)) + "." + b64.EncodeToString([]byte(payload))
	return in + "." + b64.EncodeToString(ed25519.Sign(priv, []byte(in)))
}

func main() {
	withJWKS := flag.Bool("jwks", false, "also rewrite the committed jwks-*.json documents")
	flag.Parse()
	_, self, _, _ := runtime.Caller(0)
	dir := filepath.Dir(filepath.Dir(self)) // tomb/lich/seeds/auth
	tokDir := filepath.Join(dir, "tokens")
	if err := os.MkdirAll(tokDir, 0o755); err != nil { // #nosec G301 -- public test seeds
		panic(err)
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	kid := kidOf(pub)
	ok := claims("cuirass", "wotan", 5*time.Minute, nil)
	hdr := `{"alg":"EdDSA","typ":"JWT","kid":"` + kid + `"}`

	// HS256 keyed with the Ed25519 public key: the classic asymmetric ->
	// symmetric confusion. A verifier that let the header pick the algorithm
	// and used the trusted public key as the HMAC secret would accept it.
	hsIn := b64.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT","kid":"`+kid+`"}`)) + "." + b64.EncodeToString([]byte(ok))
	mac := hmac.New(sha256.New, pub)
	mac.Write([]byte(hsIn))
	hs := hsIn + "." + b64.EncodeToString(mac.Sum(nil))

	good := signed(priv, hdr, ok)
	parts := strings.Split(good, ".")

	tokens := []struct{ name, tok string }{
		{"alg-none", b64.EncodeToString([]byte(`{"alg":"none","typ":"JWT","kid":"`+kid+`"}`)) + "." + b64.EncodeToString([]byte(ok)) + "."},
		{"alg-hs256-pubkey-as-secret", hs},
		{"alg-lowercase-eddsa", signed(priv, `{"alg":"eddsa","typ":"JWT","kid":"`+kid+`"}`, ok)},
		{"alg-duplicate-key", signed(priv, `{"alg":"EdDSA","alg":"none","typ":"JWT","kid":"`+kid+`"}`, ok)},
		{"kid-path-traversal", signed(priv, `{"alg":"EdDSA","typ":"JWT","kid":"../../../etc/unheaded/keys/wotan"}`, ok)},
		{"kid-missing", signed(priv, `{"alg":"EdDSA","typ":"JWT"}`, ok)},
		{"crit-header", signed(priv, `{"alg":"EdDSA","typ":"JWT","kid":"`+kid+`","crit":["exp"]}`, ok)},
		{"aud-array-and-long-life", signed(priv, hdr, claims("cuirass", "", 24*time.Hour, map[string]any{"aud": []string{"wotan", "unheaded"}}))},
		{"sub-not-kid-owner", signed(priv, hdr, claims("unheaded-auth", "wotan", 5*time.Minute, map[string]any{"roles": []string{"admin"}}))},
		{"four-segments-padded", parts[0] + "=." + parts[1] + "." + parts[2] + "." + parts[2]},
	}
	for i, t := range tokens {
		write(tokDir, fmt.Sprintf("token-%02d-%s.txt", i+1, t.name), t.tok)
	}

	fmt.Printf("%d tokens in %s\n", len(tokens), tokDir)
	if !*withJWKS {
		return
	}

	x := b64.EncodeToString(pub)
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	short := b64.EncodeToString(pub[:31])
	jwks := []struct{ name, doc string }{
		{"wrong-curve-x25519", `{"keys":[{"kty":"OKP","crv":"X25519","kid":"` + kid + `","x":"` + x + `"}]}`},
		{"duplicate-kid-two-keys", `{"keys":[{"kty":"OKP","crv":"Ed25519","kid":"` + kid + `","x":"` + x + `"},{"kty":"OKP","crv":"Ed25519","kid":"` + kid + `","x":"` + b64.EncodeToString(other) + `"}]}`},
		{"short-key", `{"keys":[{"kty":"OKP","crv":"Ed25519","kid":"` + kid + `","x":"` + short + `"}]}`},
		{"symmetric-oct-key", `{"keys":[{"kty":"oct","alg":"HS256","kid":"` + kid + `","k":"` + x + `"}]}`},
		{"keys-not-array-nested", `{"keys":{"keys":[[[[[[[[{"kty":"OKP"}]]]]]]]]},"jwks_uri":"http://169.254.169.254/latest/meta-data/"}`},
	}
	for i, j := range jwks {
		if !json.Valid([]byte(j.doc)) {
			panic("invalid seed json: " + j.name)
		}
		write(dir, fmt.Sprintf("jwks-%02d-%s.json", i+1, j.name), j.doc)
	}
	fmt.Printf("%d JWKS documents in %s\n", len(jwks), dir)
}

func write(dir, name, body string) {
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil { // #nosec G306 -- public test seeds, no secrets
		panic(err)
	}
}
