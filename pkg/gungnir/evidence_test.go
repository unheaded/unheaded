// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package gungnir

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"
)

func evidenceSigner(t *testing.T) *Signer {
	t.Helper()
	s, err := NewSignerWithID(GenerateDevKey())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestEvidence_RoundTrip(t *testing.T) {
	s := evidenceSigner(t)
	payload := []byte("schema_version: \"1.0.0\"\n")
	seal, err := s.SignEvidence(payload, "manifest")
	if err != nil {
		t.Fatal(err)
	}
	if seal.Format != EvidenceFormat || seal.Purpose != "manifest" || seal.Algorithm != Algorithm {
		t.Errorf("seal header: %+v", seal)
	}
	if seal.KeyId != KeyID(s.pub) {
		t.Errorf("key id %q, want %q", seal.KeyId, KeyID(s.pub))
	}
	if err := VerifyEvidence(payload, seal, s.pub, "manifest"); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

// Evidence is audited months after the build: a seal must verify long after
// any config-seal TTL would have run out.
func TestEvidence_DoesNotExpire(t *testing.T) {
	s := evidenceSigner(t)
	payload := []byte("old build")
	issued := time.Now().Add(-400 * 24 * time.Hour)
	seal, err := s.signEvidenceAt(payload, "manifest", issued)
	if err != nil {
		t.Fatal(err)
	}
	if seal.IssuedAtUnix != issued.Unix() {
		t.Fatalf("issued %d, want %d", seal.IssuedAtUnix, issued.Unix())
	}
	if err := VerifyEvidence(payload, seal, s.pub, "manifest"); err != nil {
		t.Fatalf("400-day-old evidence seal: %v", err)
	}
}

// A config seal and an evidence seal from the same key, over the same payload,
// must never stand in for each other (different ML-DSA contexts and messages).
func TestEvidence_DomainSeparation(t *testing.T) {
	s := evidenceSigner(t)
	payload := []byte("same bytes")

	cfg, err := s.SignFor(payload, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	asEvidence := &EvidenceSeal{
		Format:        EvidenceFormat,
		Purpose:       "manifest",
		PayloadSha256: cfg.PayloadSha256,
		KeyId:         cfg.KeyId,
		IssuedAtUnix:  cfg.IssuedAtUnix,
		Algorithm:     cfg.Algorithm,
		Mldsa65Sig:    cfg.Mldsa65Sig,
	}
	if err := VerifyEvidence(payload, asEvidence, s.pub, "manifest"); !errors.Is(err, ErrBadSignature) {
		t.Errorf("config seal accepted as evidence: %v", err)
	}

	ev, err := s.SignEvidence(payload, "manifest")
	if err != nil {
		t.Fatal(err)
	}
	asConfig := &GungnirSeal{
		PayloadSha256: ev.PayloadSha256,
		Mldsa65Sig:    ev.Mldsa65Sig,
		KeyId:         ev.KeyId,
		IssuedAtUnix:  ev.IssuedAtUnix,
		ExpiresAtUnix: time.Now().Add(time.Hour).Unix(),
		Algorithm:     ev.Algorithm,
	}
	if err := Verify(payload, asConfig, s.pub); !errors.Is(err, ErrBadSignature) {
		t.Errorf("evidence seal accepted as config seal: %v", err)
	}
	asConfig.ExpiresAtUnix = 0 // what a naive conversion of "no expiry" gives
	if err := Verify(payload, asConfig, s.pub); err == nil {
		t.Error("evidence seal with zero expiry accepted by Verify")
	}
	// The raw signature under the config context must not verify either.
	if mldsa65.Verify(s.pub, evidenceMessage(ev), []byte(sealContext), ev.Mldsa65Sig) {
		t.Error("evidence signature verifies under the config-seal context")
	}
}

func TestEvidence_Refusals(t *testing.T) {
	s := evidenceSigner(t)
	other := evidenceSigner(t)
	payload := []byte("manifest bytes")
	fresh := func() *EvidenceSeal {
		seal, err := s.SignEvidence(payload, "manifest")
		if err != nil {
			t.Fatal(err)
		}
		return seal
	}

	cases := []struct {
		name    string
		payload []byte
		edit    func(*EvidenceSeal)
		pub     *mldsa65.PublicKey
		purpose string
		want    error
	}{
		{"tampered payload", []byte("manifest byteZ"), nil, s.pub, "manifest", ErrPayloadHash},
		{"wrong key", payload, nil, other.pub, "manifest", ErrKeyID},
		{"wrong key, key id edited to match", payload, func(e *EvidenceSeal) { e.KeyId = KeyID(other.pub) }, other.pub, "manifest", ErrBadSignature},
		{"wrong purpose asked", payload, nil, s.pub, "iso-sha256", ErrPurpose},
		{"purpose edited to match", payload, func(e *EvidenceSeal) { e.Purpose = "iso-sha256" }, s.pub, "iso-sha256", ErrBadSignature},
		{"algorithm edited", payload, func(e *EvidenceSeal) { e.Algorithm = "ed25519" }, s.pub, "manifest", ErrWrongAlgo},
		{"key id not KeyID(pub)", payload, func(e *EvidenceSeal) { e.KeyId = "0123456789abcdef" }, s.pub, "manifest", ErrKeyID},
		{"format edited", payload, func(e *EvidenceSeal) { e.Format = "gungnir-evidence/v2" }, s.pub, "manifest", ErrFormat},
		{"format empty", payload, func(e *EvidenceSeal) { e.Format = "" }, s.pub, "manifest", ErrFormat},
		{"issued moved", payload, func(e *EvidenceSeal) { e.IssuedAtUnix -= 60 }, s.pub, "manifest", ErrBadSignature},
		{"issued zero", payload, func(e *EvidenceSeal) { e.IssuedAtUnix = 0 }, s.pub, "manifest", ErrBadSignature},
		{"hash replaced to match a new payload", []byte("evil"), func(e *EvidenceSeal) { e.PayloadSha256 = sha256Of([]byte("evil")) }, s.pub, "manifest", ErrBadSignature},
		{"hash truncated", payload, func(e *EvidenceSeal) { e.PayloadSha256 = e.PayloadSha256[:31] }, s.pub, "manifest", ErrPayloadHash},
		{"signature flipped", payload, func(e *EvidenceSeal) { e.Mldsa65Sig[100] ^= 1 }, s.pub, "manifest", ErrBadSignature},
		{"signature truncated", payload, func(e *EvidenceSeal) { e.Mldsa65Sig = e.Mldsa65Sig[:10] }, s.pub, "manifest", ErrBadSignature},
		{"signature empty", payload, func(e *EvidenceSeal) { e.Mldsa65Sig = nil }, s.pub, "manifest", ErrBadSignature},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seal := fresh()
			if tc.edit != nil {
				tc.edit(seal)
			}
			if err := VerifyEvidence(tc.payload, seal, tc.pub, tc.purpose); !errors.Is(err, tc.want) {
				t.Errorf("got %v, want %v", err, tc.want)
			}
		})
	}

	if err := VerifyEvidence(payload, nil, s.pub, "manifest"); !errors.Is(err, ErrNilSeal) {
		t.Errorf("nil seal: %v", err)
	}
	if err := VerifyEvidence(payload, fresh(), nil, "manifest"); err == nil {
		t.Error("nil public key accepted")
	}
	if err := VerifyEvidence(payload, fresh(), s.pub, ""); !errors.Is(err, ErrPurpose) {
		t.Errorf("empty expected purpose: %v", err)
	}
}

func TestEvidence_SignRefusals(t *testing.T) {
	s := evidenceSigner(t)
	for _, p := range []string{"", "Manifest", "has space", "a/b", strings.Repeat("a", 65), "manifest\n"} {
		if _, err := s.SignEvidence([]byte("x"), p); !errors.Is(err, ErrPurpose) {
			t.Errorf("purpose %q: %v", p, err)
		}
	}
	dev, err := NewSigner(GenerateDevKey())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dev.SignEvidence([]byte("x"), "manifest"); !errors.Is(err, ErrKeyID) {
		t.Errorf("dev-spike key id accepted for evidence: %v", err)
	}
	var nilSigner *Signer
	if _, err := nilSigner.SignEvidence([]byte("x"), "manifest"); err == nil {
		t.Error("nil signer accepted")
	}
}

// The JSON envelope is what ships in a pack: its field names are part of the
// format and must not drift.
func TestEvidence_JSONStable(t *testing.T) {
	s := evidenceSigner(t)
	payload := []byte("iso hash line\n")
	seal, err := s.SignEvidence(payload, "iso-sha256")
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(seal)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	want := []string{"algorithm", "format", "issued_at_unix", "key_id", "mldsa65_sig", "payload_sha256", "purpose"}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Errorf("JSON keys %v, want %v", keys, want)
	}
	back, err := ParseEvidenceSeal(b)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyEvidence(payload, back, s.pub, "iso-sha256"); err != nil {
		t.Fatalf("after JSON round trip: %v", err)
	}
	b2, err := json.Marshal(back)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, b2) {
		t.Error("JSON round trip is not byte-stable")
	}
}

func TestParseEvidenceSeal_Refusals(t *testing.T) {
	s := evidenceSigner(t)
	seal, err := s.SignEvidence([]byte("p"), "manifest")
	if err != nil {
		t.Fatal(err)
	}
	good, err := json.Marshal(seal)
	if err != nil {
		t.Fatal(err)
	}
	for name, in := range map[string][]byte{
		"empty":          nil,
		"not json":       []byte("not json"),
		"unknown field":  bytes.Replace(good, []byte(`{`), []byte(`{"expires_at_unix":1,`), 1),
		"trailing data":  append(append([]byte{}, good...), []byte(` {}`)...),
		"array":          []byte(`[]`),
		"oversized":      bytes.Repeat([]byte(" "), MaxEvidenceSealBytes+1),
		"config seal":    []byte(`{"PayloadSha256":"AA==","Mldsa65Sig":"AA==","KeyId":"x","IssuedAtUnix":1,"ExpiresAtUnix":2,"Algorithm":"ml-dsa-65"}`),
		"duplicate keys": bytes.Replace(good, []byte(`{`), []byte(`{"purpose":"custody-entry",`), 1),
	} {
		if _, err := ParseEvidenceSeal(in); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestEvidenceDetached(t *testing.T) {
	s := evidenceSigner(t)
	other := evidenceSigner(t)
	payload := []byte("custody entry canonical bytes")
	sig, err := s.SignEvidenceDetached(payload, "custody-entry")
	if err != nil {
		t.Fatal(err)
	}
	if len(sig) != mldsa65.SignatureSize {
		t.Fatalf("sig length %d", len(sig))
	}
	if err := VerifyEvidenceDetached(payload, sig, s.pub, "custody-entry"); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := VerifyEvidenceDetached([]byte("custody entry canonical byteZ"), sig, s.pub, "custody-entry"); !errors.Is(err, ErrBadSignature) {
		t.Errorf("tampered payload: %v", err)
	}
	if err := VerifyEvidenceDetached(payload, sig, s.pub, "manifest"); !errors.Is(err, ErrBadSignature) {
		t.Errorf("other purpose: %v", err)
	}
	if err := VerifyEvidenceDetached(payload, sig, other.pub, "custody-entry"); !errors.Is(err, ErrBadSignature) {
		t.Errorf("other key: %v", err)
	}
	if err := VerifyEvidenceDetached(payload, sig[:100], s.pub, "custody-entry"); !errors.Is(err, ErrBadSignature) {
		t.Errorf("short sig: %v", err)
	}
	if err := VerifyEvidenceDetached(payload, sig, nil, "custody-entry"); err == nil {
		t.Error("nil key accepted")
	}
	// A seal-shaped signature (with a real issue time) is not a detached one.
	seal, err := s.SignEvidence(payload, "custody-entry")
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyEvidenceDetached(payload, seal.Mldsa65Sig, s.pub, "custody-entry"); !errors.Is(err, ErrBadSignature) {
		t.Errorf("timed seal accepted as detached: %v", err)
	}
	if _, err := s.SignEvidenceDetached(payload, ""); !errors.Is(err, ErrPurpose) {
		t.Errorf("empty purpose: %v", err)
	}
}

func TestPublicKeyFromSeed(t *testing.T) {
	dir := t.TempDir()
	seed := make([]byte, mldsa65.SeedSize)
	if _, err := rand.Read(seed); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "seed")
	if err := os.WriteFile(p, seed, 0o600); err != nil {
		t.Fatal(err)
	}
	pub, err := PublicKeyFromSeed(p)
	if err != nil {
		t.Fatal(err)
	}
	priv, err := LoadSeedFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSignerWithID(priv)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pub.Bytes(), s.PublicKey()) {
		t.Error("derived public key differs from the signer's")
	}
	if err := os.Chmod(p, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := PublicKeyFromSeed(p); !errors.Is(err, ErrKeyFilePerms) {
		t.Errorf("world-readable seed: %v", err)
	}
}

func FuzzParseEvidenceSeal(f *testing.F) {
	f.Add([]byte(`{"format":"gungnir-evidence/v1","purpose":"manifest","payload_sha256":"AA==","key_id":"x","issued_at_unix":1,"algorithm":"ml-dsa-65","mldsa65_sig":"AA=="}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`null`))
	pub, _, err := mldsa65.GenerateKey(nil)
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, in []byte) {
		seal, err := ParseEvidenceSeal(in)
		if err != nil {
			return
		}
		// Whatever parses must be refused, never panic: no seal here was signed.
		if err := VerifyEvidence([]byte("payload"), seal, pub, "manifest"); err == nil {
			t.Fatal("unsigned fuzz input verified")
		}
	})
}
