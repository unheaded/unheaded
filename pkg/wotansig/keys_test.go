// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package wotansig

import (
	"bytes"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"
)

// writeSeed writes a fresh ML-DSA-65 seed (generated now, never committed)
// with mode and returns its path and the public key it makes.
func writeSeed(t *testing.T, mode os.FileMode) (string, []byte) {
	t.Helper()
	var seed [mldsa65.SeedSize]byte
	if _, err := rand.Read(seed[:]); err != nil {
		t.Fatal(err)
	}
	pub, _ := mldsa65.NewKeyFromSeed(&seed)
	p := filepath.Join(t.TempDir(), "health-signing.key")
	if err := os.WriteFile(p, seed[:], 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
	b, err := pub.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	return p, b
}

func TestLoadSignerFile_SignsWithTheSeedsKey(t *testing.T) {
	p, pub := writeSeed(t, 0o400)
	s, err := LoadSignerFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(s.PublicKey(), pub) {
		t.Fatal("signer's public key is not the seed's")
	}
	sg, err := s.Sign("system.health.consensus", "akira", []byte("p"))
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyEvent("system.health.consensus", "akira", []byte("p"), sg, [][]byte{pub}); err != nil {
		t.Fatal(err)
	}
}

// Same owner rules as the service keys (S-IRON-GATE Phase 2): a key file
// group or others can read is refused.
func TestLoadSignerFile_Refusals(t *testing.T) {
	for _, mode := range []os.FileMode{0o440, 0o404, 0o644, 0o666} {
		p, _ := writeSeed(t, mode)
		if _, err := LoadSignerFile(p); err == nil {
			t.Errorf("mode %04o accepted", mode)
		}
	}
	short := filepath.Join(t.TempDir(), "short.key")
	if err := os.WriteFile(short, []byte("not a seed"), 0o400); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSignerFile(short); err == nil {
		t.Error("a file that is not a 32-byte seed accepted")
	}
	if _, err := LoadSignerFile(filepath.Join(t.TempDir(), "missing.key")); err == nil {
		t.Error("missing file accepted")
	}
	if _, err := LoadSignerFile(""); err == nil {
		t.Error("empty path accepted")
	}
}

func TestSignerFromEnv(t *testing.T) {
	t.Setenv(HealthKeyEnv, "")
	s, err := SignerFromEnv(HealthKeyEnv)
	if err != nil || s != nil {
		t.Fatalf("unset: %v, %v; want nil, nil (the producer then refuses signed topics)", s, err)
	}
	p, pub := writeSeed(t, 0o400)
	t.Setenv(HealthKeyEnv, p)
	s, err = SignerFromEnv(HealthKeyEnv)
	if err != nil || !bytes.Equal(s.PublicKey(), pub) {
		t.Fatalf("set: %v", err)
	}
	bad, _ := writeSeed(t, 0o644)
	t.Setenv(AuditKeyEnv, bad)
	if _, err := SignerFromEnv(AuditKeyEnv); err == nil {
		t.Fatal("a readable key named by the env was accepted")
	}
}

func TestCovers(t *testing.T) {
	for _, c := range []struct {
		prefix, topic string
		want          bool
	}{
		{"alerts.critical.", "alerts.critical", true},
		{"alerts.critical.", "alerts.critical.disk", true},
		{"alerts.critical.", "alerts.criticality", false},
		{"alerts.critical.", "alerts.warning", false},
		{"drift.", "drift.detected.n1", true},
		{"drift.", "drift", true},
		{"drift.", "driftwood.x", false},
		{"", "anything", false},
		{".", "", false},
	} {
		if got := Covers(c.prefix, c.topic); got != c.want {
			t.Errorf("Covers(%q, %q) = %v, want %v", c.prefix, c.topic, got, c.want)
		}
	}
}

// The producer-side list: a first-party publisher never sends these
// unsigned, whatever Wotan's config says.
func TestRequiresSignature_ProducerList(t *testing.T) {
	for topic, want := range map[string]bool{
		"config.kingdom":          true,
		"drift.detected.west":     true,
		"audit.login":             true,
		"alerts.critical":         true,
		"system.health.consensus": true,
		"system.health.remediate": true,
		"system.health.reports":   false,
		"alerts.warning":          false,
		"zhen.audit.toolcalls":    false,
		"logs.akira.info":         false,
	} {
		if got := RequiresSignature(topic); got != want {
			t.Errorf("RequiresSignature(%q) = %v, want %v", topic, got, want)
		}
	}
	if err := RefuseUnsigned("alerts.critical"); !errors.Is(err, ErrSignatureRequired) {
		t.Errorf("RefuseUnsigned(alerts.critical) = %v", err)
	}
	if err := RefuseUnsigned("system.health.reports"); err != nil {
		t.Errorf("RefuseUnsigned(system.health.reports) = %v", err)
	}
}
