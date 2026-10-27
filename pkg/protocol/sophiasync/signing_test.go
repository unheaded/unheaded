// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package sophiasync

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"

	"unheaded/pkg/crypto/pqc"
	wotanClient "unheaded/pkg/wotan-client"
)

// M-149: a dictionary delta is signed with ML-DSA-65 over its canonical
// bytes (sophia-dictionary-03 Security Considerations 1), not "signed" with
// a bare SHA-256 anyone can recompute.

// newDeltaSigner generates an ML-DSA-65 key at test time (no key material in
// the repository).
func newDeltaSigner(t *testing.T) *Signer {
	t.Helper()
	_, s, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA65)
	if err != nil {
		t.Fatal(err)
	}
	ds, err := NewSigner(s)
	if err != nil {
		t.Fatal(err)
	}
	return ds
}

func testDelta(v TableVersion) *DictionaryDelta {
	return &DictionaryDelta{
		Version:   v,
		Additions: []DictEntry{{Index: 1, Value: "svc.api"}, {Index: 2, Value: "svc.db"}},
		Removals:  []uint32{7},
	}
}

func newSignedManager(t *testing.T, require bool, trusted ...[]byte) *SyncManager {
	t.Helper()
	sm, err := NewSyncManager(&wotanClient.Client{}, "control", makeTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	sm.SetSignaturePolicy(trusted, require)
	return sm
}

func TestSignVerify_RoundTrip(t *testing.T) {
	ds := newDeltaSigner(t)
	d := testDelta(1)
	if err := ds.Sign(d); err != nil {
		t.Fatal(err)
	}
	if len(d.Signature) != mldsa65.SignatureSize || d.Algorithm != DeltaAlgorithm || d.SignedAtUnixMs == 0 ||
		!bytes.Equal(d.PublicKey, ds.PublicKey()) {
		t.Fatalf("signed delta: sig=%d alg=%q at=%d", len(d.Signature), d.Algorithm, d.SignedAtUnixMs)
	}
	if err := Verify(d, [][]byte{ds.PublicKey()}); err != nil {
		t.Fatalf("Verify = %v", err)
	}
}

func TestVerify_TamperedDeltaFails(t *testing.T) {
	ds := newDeltaSigner(t)
	trusted := [][]byte{ds.PublicKey()}
	for name, tamper := range map[string]func(d *DictionaryDelta){
		"value":     func(d *DictionaryDelta) { d.Additions[0].Value = "svc.evil" },
		"index":     func(d *DictionaryDelta) { d.Additions[1].Index = 9 },
		"removal":   func(d *DictionaryDelta) { d.Removals = nil },
		"version":   func(d *DictionaryDelta) { d.Version++ },
		"signed_at": func(d *DictionaryDelta) { d.SignedAtUnixMs++ },
		"signature": func(d *DictionaryDelta) { d.Signature[10] ^= 1 },
	} {
		d := testDelta(1)
		if err := ds.Sign(d); err != nil {
			t.Fatal(err)
		}
		tamper(d)
		if err := Verify(d, trusted); !errors.Is(err, ErrInvalidSignature) {
			t.Errorf("%s tampered: Verify = %v, want ErrInvalidSignature", name, err)
		}
	}
}

func TestVerify_WrongKeyFails(t *testing.T) {
	signer, other := newDeltaSigner(t), newDeltaSigner(t)
	d := testDelta(1)
	if err := signer.Sign(d); err != nil {
		t.Fatal(err)
	}
	if err := Verify(d, [][]byte{other.PublicKey()}); !errors.Is(err, ErrUntrustedKey) {
		t.Fatalf("Verify(untrusted) = %v, want ErrUntrustedKey", err)
	}
	if err := Verify(d, nil); !errors.Is(err, ErrNoTrustedKeys) {
		t.Fatalf("Verify(no keys) = %v, want ErrNoTrustedKeys (fail closed)", err)
	}
	// The attached key swapped for a trusted one: the signature no longer
	// verifies under it.
	d.PublicKey = other.PublicKey()
	if err := Verify(d, [][]byte{other.PublicKey()}); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("Verify(swapped key) = %v, want ErrInvalidSignature", err)
	}
	d.Algorithm = "sha-256"
	if err := Verify(d, [][]byte{other.PublicKey()}); !errors.Is(err, ErrWrongAlgorithm) {
		t.Fatalf("Verify(wrong algorithm) = %v, want ErrWrongAlgorithm", err)
	}
}

// The old "signature" was SHA-256 of the delta: anyone could make one.
func TestApplyDelta_RecomputedDigestIsNotASignature(t *testing.T) {
	ds := newDeltaSigner(t)
	sm := newSignedManager(t, false, ds.PublicKey())
	d := testDelta(1)
	digest := d.Digest()
	d.Signature = digest[:]
	if err := sm.ApplyDelta(d); err == nil {
		t.Fatal("a SHA-256 digest was accepted as a signature")
	}
}

func TestApplyDelta_SignedAccepted(t *testing.T) {
	ds := newDeltaSigner(t)
	sm := newSignedManager(t, true, ds.PublicKey())
	d := testDelta(1)
	if err := ds.Sign(d); err != nil {
		t.Fatal(err)
	}
	if err := sm.ApplyDelta(d); err != nil {
		t.Fatalf("ApplyDelta(signed) = %v", err)
	}
	if sm.CurrentVersion() != 1 {
		t.Fatalf("version = %d, want 1", sm.CurrentVersion())
	}
}

func TestApplyDelta_BadSignatureRejectedAndCounted(t *testing.T) {
	ds := newDeltaSigner(t)
	sm := newSignedManager(t, false, ds.PublicKey())
	d := testDelta(1)
	if err := ds.Sign(d); err != nil {
		t.Fatal(err)
	}
	d.Additions[0].Value = "svc.evil"
	before := deltaSignatureFailures.Value()
	if err := sm.ApplyDelta(d); err == nil {
		t.Fatal("a tampered signed delta was applied in observe mode")
	}
	if d := deltaSignatureFailures.Value() - before; d != 1 {
		t.Fatalf("signature_failures grew by %v, want 1", d)
	}
	if sm.CurrentVersion() != 0 {
		t.Fatalf("version moved to %d on a rejected delta", sm.CurrentVersion())
	}
}

// Observe mode (the default this sprint): an unsigned delta is applied,
// counted and logged. ADR-062 warned that a mandatory signature would refuse
// every delta produced today; docs/security/severity-ladder.md says when it
// flips.
func TestApplyDelta_UnsignedObserveModeAcceptedAndCounted(t *testing.T) {
	sm := newSignedManager(t, false, newDeltaSigner(t).PublicKey())
	before := deltaUnsigned.Value()
	if err := sm.ApplyDelta(testDelta(1)); err != nil {
		t.Fatalf("ApplyDelta(unsigned, observe) = %v", err)
	}
	if d := deltaUnsigned.Value() - before; d != 1 {
		t.Fatalf("unsigned_total grew by %v, want 1", d)
	}
}

func TestApplyDelta_UnsignedRequireModeRejected(t *testing.T) {
	sm := newSignedManager(t, true, newDeltaSigner(t).PublicKey())
	before := deltaUnsigned.Value()
	if err := sm.ApplyDelta(testDelta(1)); !errors.Is(err, ErrUnsignedDelta) {
		t.Fatalf("ApplyDelta(unsigned, require) = %v, want ErrUnsignedDelta", err)
	}
	if d := deltaUnsigned.Value() - before; d != 1 {
		t.Fatalf("unsigned_total grew by %v, want 1", d)
	}
	if sm.CurrentVersion() != 0 {
		t.Fatalf("version moved to %d on a rejected delta", sm.CurrentVersion())
	}
}

// sophia-dictionary-03: reject updates signed more than 5 minutes from now.
func TestApplyDelta_StaleSignatureRejected(t *testing.T) {
	ds := newDeltaSigner(t)
	sm := newSignedManager(t, true, ds.PublicKey())
	d := testDelta(1)
	if err := ds.SignAt(d, time.Now().Add(-10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := sm.ApplyDelta(d); !errors.Is(err, ErrStaleDelta) {
		t.Fatalf("ApplyDelta(10 min old) = %v, want ErrStaleDelta", err)
	}
}

// Wire format: a signed delta still serialises; the length prefixes carry an
// ML-DSA-65 signature (3,309 bytes) and public key (1,952 bytes), and
// DecodeDelta returns exactly what was encoded.
func TestEncodeDecode_SignedDeltaRoundTrips(t *testing.T) {
	ds := newDeltaSigner(t)
	d := testDelta(3)
	if err := ds.Sign(d); err != nil {
		t.Fatal(err)
	}
	data, err := encodeDelta(d)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeDelta(data)
	if err != nil {
		t.Fatalf("DecodeDelta = %v", err)
	}
	if len(got.Signature) != 3309 || len(got.PublicKey) != 1952 {
		t.Fatalf("decoded sig=%d key=%d, want 3309/1952", len(got.Signature), len(got.PublicKey))
	}
	if got.Version != d.Version || got.Algorithm != d.Algorithm || got.SignedAtUnixMs != d.SignedAtUnixMs ||
		len(got.Additions) != 2 || got.Additions[1] != d.Additions[1] || len(got.Removals) != 1 {
		t.Fatalf("decoded %+v, want %+v", got, d)
	}
	if err := Verify(got, [][]byte{ds.PublicKey()}); err != nil {
		t.Fatalf("decoded delta does not verify: %v", err)
	}
}

func TestDecodeDelta_RejectsMalformed(t *testing.T) {
	ds := newDeltaSigner(t)
	d := testDelta(1)
	if err := ds.Sign(d); err != nil {
		t.Fatal(err)
	}
	good, err := encodeDelta(d)
	if err != nil {
		t.Fatal(err)
	}
	for name, b := range map[string][]byte{
		"empty":     nil,
		"truncated": good[:len(good)-1],
		"trailing":  append(append([]byte(nil), good...), 0),
		"short":     good[:10],
	} {
		if _, err := DecodeDelta(b); err == nil {
			t.Errorf("%s: DecodeDelta accepted it", name)
		}
	}
	// A huge declared count must not allocate before it is checked.
	huge := append([]byte(nil), good[:8]...)
	huge = append(huge, 0xff, 0xff, 0xff, 0xff)
	if _, err := DecodeDelta(huge); err == nil {
		t.Error("a 4G-entry count was accepted")
	}
}

func FuzzDeserialize(f *testing.F) {
	_, s, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA65)
	if err != nil {
		f.Fatal(err)
	}
	ds, err := NewSigner(s)
	if err != nil {
		f.Fatal(err)
	}
	d := testDelta(1)
	if err := ds.Sign(d); err != nil {
		f.Fatal(err)
	}
	seed, err := encodeDelta(d)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(seed)
	unsigned, _ := encodeDelta(testDelta(2))
	f.Add(unsigned)
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, b []byte) {
		got, err := DecodeDelta(b)
		if err != nil {
			return
		}
		// Whatever decodes re-encodes to the same bytes.
		again, err := encodeDelta(got)
		if err != nil {
			t.Fatalf("re-encode: %v", err)
		}
		if !bytes.Equal(again, b) {
			t.Fatalf("decode/encode not an identity: %x != %x", again, b)
		}
		_ = Verify(got, [][]byte{ds.PublicKey()})
	})
}

// The key layout S-IRON-GATE uses for ML-DSA-65 signing keys (ADR-103):
// a 32-byte gungnir seed made by `unheaded-keygen -algo mldsa65`, mode 0400
// (0600 accepted), named by an environment variable. Unset = no signer.
func TestSignerFromEnv(t *testing.T) {
	t.Setenv(SigningKeyEnv, "")
	if s, err := SignerFromEnv(); s != nil || err != nil {
		t.Fatalf("unset: (%v, %v), want (nil, nil): signing off", s, err)
	}

	seed := make([]byte, mldsa65.SeedSize)
	for i := range seed {
		seed[i] = byte(i * 7)
	}
	path := filepath.Join(t.TempDir(), "delta-signing.key")
	if err := os.WriteFile(path, seed, 0o400); err != nil {
		t.Fatal(err)
	}
	t.Setenv(SigningKeyEnv, path)
	s, err := SignerFromEnv()
	if err != nil || s == nil {
		t.Fatalf("SignerFromEnv = (%v, %v)", s, err)
	}
	d := testDelta(1)
	if err := s.Sign(d); err != nil {
		t.Fatal(err)
	}
	if err := Verify(d, [][]byte{s.PublicKey()}); err != nil {
		t.Fatalf("Verify with the loaded key = %v", err)
	}

	loose := filepath.Join(t.TempDir(), "loose.key")
	if err := os.WriteFile(loose, seed, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(SigningKeyEnv, loose)
	if s, err := SignerFromEnv(); err == nil || s != nil {
		t.Fatal("a group/world-readable key file was accepted")
	}
	t.Setenv(SigningKeyEnv, filepath.Join(t.TempDir(), "missing.key"))
	if _, err := SignerFromEnv(); err == nil {
		t.Fatal("a named but missing key file was accepted (must fail loudly)")
	}
}

func TestRequireSignatureFromEnv(t *testing.T) {
	for v, want := range map[string]bool{"": false, "false": false, "0": false, "true": true, "1": true, " TRUE ": true} {
		t.Setenv(RequireSignatureEnv, v)
		got, err := RequireSignatureFromEnv()
		if err != nil || got != want {
			t.Errorf("%q: (%v, %v), want (%v, nil)", v, got, err, want)
		}
	}
	t.Setenv(RequireSignatureEnv, "yes please")
	if _, err := RequireSignatureFromEnv(); err == nil {
		t.Error("an unparseable value was taken as observe mode")
	}
}
