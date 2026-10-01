// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package gungnir

import (
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"
)

func TestKeyFilesRoundTripAndSignFor(t *testing.T) {
	dir := t.TempDir()
	seed := make([]byte, mldsa65.SeedSize)
	if _, err := rand.Read(seed); err != nil {
		t.Fatal(err)
	}
	seedPath := filepath.Join(dir, "seed")
	if err := os.WriteFile(seedPath, seed, 0o600); err != nil {
		t.Fatal(err)
	}
	priv, err := LoadSeedFile(seedPath)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSignerWithID(priv)
	if err != nil {
		t.Fatal(err)
	}
	pubPath := filepath.Join(dir, "pub")
	if err := os.WriteFile(pubPath, s.PublicKey(), 0o644); err != nil {
		t.Fatal(err)
	}
	pub, err := LoadPublicKeyFile(pubPath)
	if err != nil {
		t.Fatal(err)
	}
	seal, err := s.SignFor([]byte("payload"), 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if seal.KeyId != KeyID(pub) || seal.KeyId == "dev-spike" {
		t.Errorf("key id %q", seal.KeyId)
	}
	if got := seal.ExpiresAtUnix - seal.IssuedAtUnix; got != 7*24*3600 {
		t.Errorf("ttl %d", got)
	}
	if err := Verify([]byte("payload"), seal, pub); err != nil {
		t.Fatal(err)
	}
	if err := Verify([]byte("payloaD"), seal, pub); !errors.Is(err, ErrPayloadHash) {
		t.Errorf("tampered payload: %v", err)
	}
}

func TestKeyFileRefusals(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "seed")
	if err := os.WriteFile(p, make([]byte, mldsa65.SeedSize), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSeedFile(p); !errors.Is(err, ErrKeyFilePerms) {
		t.Errorf("group-readable seed: %v", err)
	}
	if err := os.WriteFile(p, []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSeedFile(p); err == nil {
		t.Error("short seed accepted")
	}
	if err := os.WriteFile(p, []byte("not a key"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPublicKeyFile(p); err == nil {
		t.Error("garbage public key accepted")
	}
	s, _ := NewSignerWithID(GenerateDevKey())
	for _, ttl := range []time.Duration{0, -time.Hour, 32 * 24 * time.Hour} {
		if _, err := s.SignFor(nil, ttl); err == nil {
			t.Errorf("ttl %s accepted", ttl)
		}
	}
}
