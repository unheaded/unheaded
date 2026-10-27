// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"unheaded/pkg/gungnir"
	"unheaded/pkg/wotansig"
)

// pubB64 is the base64 public key line keygen prints for wotan.yaml.
func pubB64(t *testing.T, stdout string) []byte {
	t.Helper()
	for _, line := range strings.Split(stdout, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "public "); ok {
			b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(v))
			if err != nil {
				t.Fatalf("public key line %q: %v", line, err)
			}
			return b
		}
	}
	t.Fatalf("no \"public <base64>\" line in %q", stdout)
	return nil
}

// S-IRON-GATE Step 279: drift and signing-prefix keys come from the same
// tool with the same ownership rules: a gungnir seed (0400) whose public key
// round-trips seed -> PublicKeyFromSeed -> base64 for wotan.yaml.
func TestCreateMLDSA65(t *testing.T) {
	keys, trust := dirs(t)
	r := runKeygen(t, "-algo", "mldsa65", "-dir", keys, "-trust-dir", trust, "heimdall-drift")
	if r.code != 0 {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
	keyPath := filepath.Join(keys, "heimdall-drift.key")
	if got := mode(t, keyPath); got != 0o400 {
		t.Errorf("key mode %04o, want 0400", got)
	}
	if got := mode(t, filepath.Join(keys, "heimdall-drift.pub")); got != 0o444 {
		t.Errorf("pub mode %04o, want 0444", got)
	}
	// An ML-DSA key never goes in trust.d: that directory holds Ed25519
	// service keys, and Wotan takes signing keys from its own config.
	if _, err := os.Stat(filepath.Join(trust, "heimdall-drift.pub")); !os.IsNotExist(err) {
		t.Errorf("ML-DSA public key in trust.d: %v", err)
	}

	pub, err := gungnir.PublicKeyFromSeed(keyPath)
	if err != nil {
		t.Fatalf("seed does not load as a gungnir seed: %v", err)
	}
	printed := pubB64(t, r.stdout)
	if !bytes.Equal(printed, pub.Bytes()) {
		t.Fatal("printed public key is not the seed's")
	}
	if !wotansig.ValidPublicKey(printed) {
		t.Fatal("printed key is not a valid ML-DSA-65 public key for wotan.yaml")
	}
	if !strings.Contains(r.stdout, gungnir.KeyID(pub)) {
		t.Errorf("stdout %q does not name the key id", r.stdout)
	}
	filePub, err := gungnir.LoadPublicKeyFile(filepath.Join(keys, "heimdall-drift.pub"))
	if err != nil || !bytes.Equal(filePub.Bytes(), pub.Bytes()) {
		t.Fatalf(".pub file: %v", err)
	}
	// The signer a producer builds from it (HEALTH_/AUDIT_SIGNING_KEY_FILE,
	// --drift-sign-key) signs with that same key.
	s, err := wotansig.LoadSignerFile(keyPath)
	if err != nil || !bytes.Equal(s.PublicKey(), printed) {
		t.Fatalf("LoadSignerFile: %v", err)
	}
	// The private seed is never printed.
	seed, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(r.stdout+r.stderr, base64.StdEncoding.EncodeToString(seed)) {
		t.Fatal("the seed was printed")
	}

	// -print-kid prints the id and the public key again, for enrolment.
	r = runKeygen(t, "-algo", "mldsa65", "-dir", keys, "-trust-dir", trust, "-print-kid", "heimdall-drift")
	if r.code != 0 || !strings.Contains(r.stdout, gungnir.KeyID(pub)) || !bytes.Equal(pubB64(t, r.stdout), printed) {
		t.Fatalf("-print-kid: exit %d stdout %q stderr %q", r.code, r.stdout, r.stderr)
	}
}

func TestCreateMLDSA65Refusals(t *testing.T) {
	keys, trust := dirs(t)
	if r := runKeygen(t, "-algo", "mldsa65", "-dir", keys, "-trust-dir", trust, "akira-health"); r.code != 0 {
		t.Fatalf("create: %q", r.stderr)
	}
	if r := runKeygen(t, "-algo", "mldsa65", "-dir", keys, "-trust-dir", trust, "akira-health"); r.code != exitRefused {
		t.Errorf("second create: exit %d, want %d (an existing key is never replaced)", r.code, exitRefused)
	}
	if r := runKeygen(t, "-algo", "mldsa65", "-rotate", "-dir", keys, "-trust-dir", trust, "akira-health"); r.code != exitRefused {
		t.Errorf("-rotate: exit %d, want %d", r.code, exitRefused)
	}
	if r := runKeygen(t, "-algo", "rsa", "-dir", keys, "-trust-dir", trust, "x"); r.code != exitRefused {
		t.Errorf("-algo rsa: exit %d, want %d", r.code, exitRefused)
	}
	// The default stays Ed25519.
	if r := runKeygen(t, "-algo", "ed25519", "-dir", keys, "-trust-dir", trust, "akira"); r.code != 0 {
		t.Errorf("-algo ed25519: %q", r.stderr)
	}
}
