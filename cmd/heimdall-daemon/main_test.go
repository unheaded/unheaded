// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"

	"unheaded/pkg/enkrateia"
	"unheaded/pkg/gungnir"
)

// ─── LoadManifest ────────────────────────────────────────────────────────────

func TestLoadManifest_ParsesValidBaselineManifest(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "mjolnir.yaml")
	body := `apiVersion: v1
kind: BaselineManifest
metadata:
  name: kingdom-baseline
  version: "0.1.0"
  created: 2026-05-09
  signed_by: heimdall
spec:
  base_image:
    distro: nixos
    release: "25.05"
    digest: sha256:abc
  files:
    - path: /etc/nixos/configuration.nix
      sha256: deadbeef
      mode: "0644"
      owner: root
  packages:
    - name: bash
      version: "5.2"
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadManifest(path, Trust{AllowUnsigned: true})
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if m.Kind != "BaselineManifest" {
		t.Errorf("Kind = %q, want BaselineManifest", m.Kind)
	}
	if m.Metadata.Name != "kingdom-baseline" {
		t.Errorf("Metadata.Name = %q", m.Metadata.Name)
	}
	if len(m.Spec.Files) != 1 || m.Spec.Files[0].SHA256 != "deadbeef" {
		t.Errorf("Spec.Files unexpected: %+v", m.Spec.Files)
	}
	if len(m.Spec.Packages) != 1 || m.Spec.Packages[0].Name != "bash" {
		t.Errorf("Spec.Packages unexpected: %+v", m.Spec.Packages)
	}
}

func TestLoadManifest_RejectsWrongKind(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	body := `apiVersion: v1
kind: NotABaselineManifest
metadata:
  name: x
spec:
  base_image: {}
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadManifest(path, Trust{AllowUnsigned: true})
	if err == nil {
		t.Fatal("expected error for wrong kind, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected kind") {
		t.Errorf("expected 'unexpected kind' in error, got: %v", err)
	}
}

func TestLoadManifest_MissingFileWraps(t *testing.T) {
	t.Parallel()
	_, err := LoadManifest("/nonexistent/mjolnir.yaml", Trust{AllowUnsigned: true})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "read manifest") {
		t.Errorf("expected wrapped 'read manifest' error, got: %v", err)
	}
}

func TestLoadManifest_InvalidYAMLWraps(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.yaml")
	if err := os.WriteFile(path, []byte("not: [valid"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadManifest(path, Trust{AllowUnsigned: true})
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	if !strings.Contains(err.Error(), "parse manifest") {
		t.Errorf("expected wrapped 'parse manifest' error, got: %v", err)
	}
}

// ─── hashFile ────────────────────────────────────────────────────────────────

func TestHashFile_ReturnsLowercaseHexSHA256(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "data.bin")
	content := []byte("kingdom marches as one")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := hashFile(path)
	if err != nil {
		t.Fatalf("hashFile: %v", err)
	}

	expected := sha256.Sum256(content)
	want := hex.EncodeToString(expected[:])
	if got != want {
		t.Errorf("hashFile mismatch:\n  got:  %s\n  want: %s", got, want)
	}
	if len(got) != 64 {
		t.Errorf("expected 64-char hex digest, got %d chars", len(got))
	}
	if strings.ToLower(got) != got {
		t.Errorf("expected lowercase hex, got %q", got)
	}
}

func TestHashFile_EmptyFileMatchesEmptySHA256(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.bin")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := hashFile(path)
	if err != nil {
		t.Fatalf("hashFile: %v", err)
	}
	const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got != emptySHA256 {
		t.Errorf("hashFile(empty) = %s, want %s", got, emptySHA256)
	}
}

func TestHashFile_MissingFileErrors(t *testing.T) {
	t.Parallel()
	_, err := hashFile("/nonexistent/file.bin")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// ─── Seal verification (ADR-043) ──────────────────────────────────────────────

const sealedManifest = `apiVersion: unheaded.org/v1
kind: BaselineManifest
metadata:
  name: test
  version: "1"
spec:
  files:
    - path: /etc/hostname
      sha256: "00"
`

// keyPair writes a 0600 seed file and returns its path and public key.
func keyPair(t *testing.T, dir string, b byte) (string, *mldsa65.PublicKey) {
	t.Helper()
	var seed [mldsa65.SeedSize]byte
	for i := range seed {
		seed[i] = b
	}
	pub, _ := mldsa65.NewKeyFromSeed(&seed)
	path := filepath.Join(dir, "seed")
	if err := os.WriteFile(path, seed[:], 0o600); err != nil {
		t.Fatal(err)
	}
	return path, pub
}

func TestLoadManifest_Seal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mjolnir.yaml")
	if err := os.WriteFile(path, []byte(sealedManifest), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadManifest(path, Trust{}); !errors.Is(err, ErrUnverified) {
		t.Fatalf("no key and no --allow-unsigned must refuse: %v", err)
	}

	seed, pub := keyPair(t, dir, 7)
	if _, err := LoadManifest(path, Trust{PubKey: pub}); err == nil {
		t.Fatal("a manifest without a seal file was trusted")
	}

	if err := signManifest(path, seed, time.Hour); err != nil {
		t.Fatal(err)
	}
	m, err := LoadManifest(path, Trust{PubKey: pub})
	if err != nil || m.Metadata.Name != "test" {
		t.Fatalf("signed manifest refused: %v", err)
	}

	// Another key's public half does not verify it.
	_, otherPub := keyPair(t, t.TempDir(), 9)
	if _, err := LoadManifest(path, Trust{PubKey: otherPub}); !errors.Is(err, gungnir.ErrBadSignature) {
		t.Fatalf("wrong key: %v", err)
	}

	// One changed byte (a swapped file hash) breaks it.
	tampered := strings.Replace(sealedManifest, `sha256: "00"`, `sha256: "01"`, 1)
	if err := os.WriteFile(path, []byte(tampered), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(path, Trust{PubKey: pub}); !errors.Is(err, gungnir.ErrPayloadHash) {
		t.Fatalf("tampered manifest: %v", err)
	}

	// An expired seal is refused, and moving the expiry forward breaks the
	// signature (it is signed, since gungnir's seal v2).
	if err := os.WriteFile(path, []byte(sealedManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := signManifest(path, seed, time.Hour); err != nil {
		t.Fatal(err)
	}
	editSeal := func(f func(*gungnir.GungnirSeal)) {
		raw, err := os.ReadFile(SealPath(path))
		if err != nil {
			t.Fatal(err)
		}
		var seal gungnir.GungnirSeal
		if err := json.Unmarshal(raw, &seal); err != nil {
			t.Fatal(err)
		}
		f(&seal)
		out, _ := json.Marshal(seal)
		if err := os.WriteFile(SealPath(path), out, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	editSeal(func(g *gungnir.GungnirSeal) { g.ExpiresAtUnix += 365 * 24 * 3600 })
	if _, err := LoadManifest(path, Trust{PubKey: pub}); !errors.Is(err, gungnir.ErrBadSignature) {
		t.Fatalf("seal with its expiry moved forward: %v", err)
	}
	editSeal(func(g *gungnir.GungnirSeal) { g.ExpiresAtUnix = 1 })
	if _, err := LoadManifest(path, Trust{PubKey: pub}); !errors.Is(err, gungnir.ErrSealExpired) {
		t.Fatalf("expired seal: %v", err)
	}
}

func TestSignManifest_RefusesReadableKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mjolnir.yaml")
	_ = os.WriteFile(path, []byte(sealedManifest), 0o644)
	seed, _ := keyPair(t, dir, 3)
	if err := os.Chmod(seed, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := signManifest(path, seed, time.Hour); !errors.Is(err, gungnir.ErrKeyFilePerms) {
		t.Fatalf("world-readable key accepted: %v", err)
	}
}

// TestRun_StopsWhenSealStopsVerifying: the seal is re-checked every scan, so
// a manifest swapped after startup, or a seal that expires while the daemon
// runs, stops it instead of being trusted from the startup check.
func TestRun_StopsWhenSealStopsVerifying(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mjolnir.yaml")
	if err := os.WriteFile(path, []byte(sealedManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	seed, pub := keyPair(t, dir, 7)
	run := func(ttl time.Duration, during func()) error {
		if err := os.WriteFile(path, []byte(sealedManifest), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := signManifest(path, seed, ttl); err != nil {
			t.Fatal(err)
		}
		trust := Trust{PubKey: pub}
		m, err := LoadManifest(path, trust)
		if err != nil {
			t.Fatal(err)
		}
		h := &Heimdall{nodeID: "t", manifestPath: path, trust: trust, manifest: m, aggregator: enkrateia.NewAggregator(16)}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		during()
		return h.Run(ctx, 20*time.Millisecond)
	}

	err := run(time.Hour, func() {
		tampered := strings.Replace(sealedManifest, `sha256: "00"`, `sha256: "01"`, 1)
		if err := os.WriteFile(path, []byte(tampered), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	if !errors.Is(err, gungnir.ErrPayloadHash) {
		t.Fatalf("manifest swapped after startup: %v", err)
	}

	if err := run(time.Second, func() {}); !errors.Is(err, gungnir.ErrSealExpired) {
		t.Fatalf("seal expiring while running: %v", err)
	}

	if err := run(time.Hour, func() {}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("a valid seal must keep running: %v", err)
	}
}
