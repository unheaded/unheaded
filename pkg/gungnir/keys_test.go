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

// LICH-012 L12a (2026-10-30): the seed is checked on the open file, not the
// path, so what is read is what was checked; and a seed path that is not a
// regular file (a FIFO would block forever) or is huge is refused.
func TestLoadSeedFileRefusesNonRegularAndOversize(t *testing.T) {
	dir := t.TempDir()

	fifo := filepath.Join(dir, "fifo")
	if err := mkfifo(fifo, 0o600); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	done := make(chan error, 1)
	go func() { _, err := LoadSeedFile(fifo); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Error("FIFO seed accepted")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("LoadSeedFile blocked on a FIFO")
	}

	sub := filepath.Join(dir, "d")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSeedFile(sub); err == nil {
		t.Error("directory accepted as a seed")
	}

	big := filepath.Join(dir, "big")
	if err := os.WriteFile(big, make([]byte, 1<<20), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSeedFile(big); err == nil {
		t.Error("1 MiB seed accepted")
	}
}

// A 0600 symlink target is fine (operators may link key files); the check
// applies to the file actually opened, so a link to a group-readable file is
// refused even though the link itself is 0777.
func TestLoadSeedFileChecksTheOpenedFile(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good")
	if err := os.WriteFile(good, make([]byte, mldsa65.SeedSize), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(good, link); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSeedFile(link); err != nil {
		t.Errorf("symlink to a 0600 seed: %v", err)
	}
	if err := os.Chmod(good, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSeedFile(link); !errors.Is(err, ErrKeyFilePerms) {
		t.Errorf("symlink to a 0644 seed: %v", err)
	}
}
