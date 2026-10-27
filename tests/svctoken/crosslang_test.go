// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// Package svctoken checks that the service tokens minted outside Go are the
// tokens pkg/auth mints (ADR-103 §2): the Rust crate crates/svc-token (used
// by cmd/trace-collector and cmd/ebpf-collector) and the Python module
// scripts/svc_token.py (used by scripts/run-runbook.py). Each test makes a
// key pair with auth.GenerateKeyPair at run time (no key is ever
// committed), has the foreign minter sign a token with it, and requires
// that token to be byte-identical to auth.SignEdDSA's for the same claims
// (Ed25519 is deterministic) and to pass auth.EdDSAVerifier against a
// trust directory, as Wotan verifies it.
package svctoken

import (
	"context"
	"crypto/ed25519"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/auth"
)

const iat = 1_800_000_000

// repoRoot is the repository root (this file is tests/svctoken/).
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// keyPair makes name's key in a temp key dir and its .pub in a temp trust
// dir, the layout unheaded-keygen writes.
func keyPair(t *testing.T, name string) (keyFile, trustDir string, priv ed25519.PrivateKey, kid string) {
	t.Helper()
	dir := t.TempDir()
	keys := filepath.Join(dir, "keys")
	trustDir = filepath.Join(dir, "trust.d")
	for _, d := range []string{keys, trustDir} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	kid, err := auth.GenerateKeyPair(keys, name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(keys, name+".pub"), filepath.Join(trustDir, name+".pub")); err != nil {
		t.Fatal(err)
	}
	keyFile = filepath.Join(keys, name+".key")
	priv, gotKid, err := auth.LoadPrivateKey(keyFile)
	if err != nil {
		t.Fatal(err)
	}
	if gotKid != kid {
		t.Fatalf("kid %s != %s", gotKid, kid)
	}
	return keyFile, trustDir, priv, kid
}

// checkToken requires tok to equal pkg/auth's token for the same inputs and
// to pass an EdDSA verifier for aud that allows sub, and a tampered copy
// to fail it.
func checkToken(t *testing.T, tok, sub, aud, trustDir string, priv ed25519.PrivateKey, kid string) {
	t.Helper()
	want, err := auth.SignEdDSA(priv, kid, &auth.JWTClaims{
		Sub:   sub,
		Iss:   auth.EdDSAServiceIssuer,
		Aud:   aud,
		Iat:   iat,
		Exp:   iat + int64(auth.EdDSAServiceTokenTTL/time.Second),
		Roles: []string{auth.RoleService},
	})
	if err != nil {
		t.Fatal(err)
	}
	if tok != want {
		t.Errorf("token differs from pkg/auth's\n got %s\nwant %s", tok, want)
	}
	trust, err := auth.NewTrustDir(trustDir)
	if err != nil {
		t.Fatal(err)
	}
	v, err := auth.NewEdDSAVerifier(auth.EdDSAVerifierConfig{
		Trust:        trust,
		Issuer:       auth.EdDSAServiceIssuer,
		Audience:     aud,
		AllowCallers: []string{sub},
		Now:          func() time.Time { return time.Unix(iat+60, 0) },
	})
	if err != nil {
		t.Fatal(err)
	}
	authn := func(tok string) (*auth.Identity, error) {
		r, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://wotan/api/v1/topics", nil)
		r.Header.Set("Authorization", "Bearer "+tok)
		return v.Authenticate(context.Background(), r)
	}
	id, err := authn(tok)
	if err != nil {
		t.Fatalf("pkg/auth refused the token: %v", err)
	}
	if id.Subject != sub || id.Extra["kid"] != kid {
		t.Errorf("identity %+v, want subject %s kid %s", id, sub, kid)
	}
	// One flipped claim byte breaks the signature.
	parts := strings.Split(tok, ".")
	claims := []byte(parts[1])
	claims[len(claims)/2] ^= 0x01
	if _, err := authn(parts[0] + "." + string(claims) + "." + parts[2]); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Errorf("tampered token: err = %v, want ErrUnauthenticated", err)
	}
}

func TestRustMinterMatchesPkgAuth(t *testing.T) {
	if testing.Short() {
		t.Skip("-short: builds crates/svc-token with cargo")
	}
	cargo, err := exec.LookPath("cargo")
	if err != nil {
		t.Skip("cargo not installed")
	}
	root := repoRoot(t)
	keyFile, trustDir, priv, kid := keyPair(t, "trace-collector")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	// #nosec G204 -- fixed tool and repo-relative manifest; the other args are test values
	cmd := exec.CommandContext(ctx, cargo, "run", "--quiet", "--locked",
		"--manifest-path", filepath.Join(root, "crates", "svc-token", "Cargo.toml"),
		"--example", "mint", "--", keyFile, "trace-collector", "wotan", strconv.Itoa(iat))
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			t.Fatalf("cargo run mint: %v\n%s", err, ee.Stderr)
		}
		t.Fatalf("cargo run mint: %v", err)
	}
	checkToken(t, strings.TrimSpace(string(out)), "trace-collector", "wotan", trustDir, priv, kid)
}

func TestPythonMinterMatchesPkgAuth(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not installed")
	}
	// The minter needs the cryptography package (in the Zhen venv pins,
	// sbom/python-requirements.txt, and Debian's python3-cryptography).
	if err := exec.Command(py, "-I", "-c", "import cryptography.hazmat.primitives.asymmetric.ed25519").Run(); err != nil { // #nosec G204 -- fixed interpreter and code
		t.Skip("python3 has no cryptography package")
	}
	root := repoRoot(t)
	for _, sub := range []string{"zhen-agentd", "unheaded-cli"} {
		keyFile, trustDir, priv, kid := keyPair(t, sub)
		// #nosec G204 -- fixed interpreter and repo script; the other args are test values
		out, err := exec.Command(py, "-I", filepath.Join(root, "scripts", "svc_token.py"),
			"mint", keyFile, sub, "wotan", strconv.Itoa(iat)).Output()
		if err != nil {
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				t.Fatalf("svc_token.py mint: %v\n%s", err, ee.Stderr)
			}
			t.Fatalf("svc_token.py mint: %v", err)
		}
		checkToken(t, strings.TrimSpace(string(out)), sub, "wotan", trustDir, priv, kid)
	}
}
