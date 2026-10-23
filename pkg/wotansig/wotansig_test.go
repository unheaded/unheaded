// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package wotansig

import (
	"bytes"
	"encoding/hex"
	"errors"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/crypto/pqc"
)

// goldenCanonical is the same vector services/wotan/internal/signing pins:
// the canonical form moved here byte for byte (ADR-105).
const goldenCanonical = "cba2c4b17a7c4b2bb29c36b2bd58d6d460070835c3c4dc8b1360ef42d6182b76"

func TestGoldenCanonical(t *testing.T) {
	got := hex.EncodeToString(Canonicalize("config.x", "sender-1", 1700000000000, []byte("payload")))
	if got != goldenCanonical {
		t.Fatalf("canonical form changed:\n got  %s\n want %s", got, goldenCanonical)
	}
}

// Each field is separated, so moving bytes between topic and sender (or
// sender and payload) changes the digest.
func TestCanonicalize_FieldsAreSeparated(t *testing.T) {
	a := Canonicalize("drift.a", "b", 1, []byte("c"))
	for name, other := range map[string][]byte{
		"topic/sender shift":  Canonicalize("drift.", "ab", 1, []byte("c")),
		"sender/payload":      Canonicalize("drift.a", "", 1, []byte("bc")),
		"time":                Canonicalize("drift.a", "b", 2, []byte("c")),
		"topic":               Canonicalize("config.a", "b", 1, []byte("c")),
		"payload appended 00": Canonicalize("drift.a", "b", 1, []byte("c\x00")),
	} {
		if bytes.Equal(a, other) {
			t.Errorf("%s: same digest", name)
		}
	}
}

func newSigner(t testing.TB) *TopicSigner {
	t.Helper()
	_, s, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA65)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	return NewTopicSigner(s)
}

func TestSignVerifyEvent_RoundTrip(t *testing.T) {
	ts := newSigner(t)
	sg, err := ts.Sign("drift.detected.n1", "heimdall-n1", []byte(`{"path":"/etc/x"}`))
	if err != nil {
		t.Fatal(err)
	}
	if sg.Algorithm != Algorithm || sg.SignedAtUnixMs == 0 || !bytes.Equal(sg.PublicKey, ts.PublicKey()) {
		t.Fatalf("unexpected Signed: alg=%q at=%d", sg.Algorithm, sg.SignedAtUnixMs)
	}
	if err := VerifyEvent("drift.detected.n1", "heimdall-n1", []byte(`{"path":"/etc/x"}`), sg, [][]byte{ts.PublicKey()}); err != nil {
		t.Fatalf("VerifyEvent: %v", err)
	}
}

// Subscribers re-verify long after Wotan accepted a message, so VerifyEvent
// has no clock: an old signature still proves who sent it.
func TestVerifyEvent_OldSignatureStillVerifies(t *testing.T) {
	ts := newSigner(t)
	at := time.Now().Add(-400 * 24 * time.Hour)
	sg, err := ts.SignAt("drift.detected.n1", "s", []byte("p"), at)
	if err != nil {
		t.Fatal(err)
	}
	if sg.SignedAtUnixMs != at.UnixMilli() {
		t.Fatalf("signed_at = %d, want %d", sg.SignedAtUnixMs, at.UnixMilli())
	}
	if err := VerifyEvent("drift.detected.n1", "s", []byte("p"), sg, [][]byte{ts.PublicKey()}); err != nil {
		t.Fatalf("old signature: %v", err)
	}
}

func TestVerifyEvent_Refusals(t *testing.T) {
	ts := newSigner(t)
	attacker := newSigner(t)
	topic, sender, payload := "drift.detected.n1", "heimdall-n1", []byte("body")
	good, err := ts.Sign(topic, sender, payload)
	if err != nil {
		t.Fatal(err)
	}
	forged, err := attacker.Sign(topic, sender, payload)
	if err != nil {
		t.Fatal(err)
	}
	trusted := [][]byte{ts.PublicKey()}

	mutate := func(f func(*Signed)) Signed {
		c := good
		c.Signature = bytes.Clone(good.Signature)
		c.PublicKey = bytes.Clone(good.PublicKey)
		f(&c)
		return c
	}
	cases := []struct {
		name    string
		topic   string
		sender  string
		payload []byte
		sg      Signed
		trusted [][]byte
		want    error
	}{
		{"wrong algorithm", topic, sender, payload, mutate(func(s *Signed) { s.Algorithm = "ed25519" }), trusted, ErrWrongAlgorithm},
		{"empty algorithm", topic, sender, payload, mutate(func(s *Signed) { s.Algorithm = "" }), trusted, ErrWrongAlgorithm},
		{"no trusted keys", topic, sender, payload, good, nil, ErrNoTrustedKeys},
		{"attacker key attached", topic, sender, payload, forged, trusted, ErrUntrustedKey},
		{"attacker sig, trusted key attached", topic, sender, payload, mutate(func(s *Signed) { s.Signature = forged.Signature }), trusted, ErrInvalidSignature},
		{"tampered payload", topic, sender, []byte("bodY"), good, trusted, ErrInvalidSignature},
		{"other topic", "config.kingdom", sender, payload, good, trusted, ErrInvalidSignature},
		{"other node topic", "drift.detected.n2", sender, payload, good, trusted, ErrInvalidSignature},
		{"other sender", topic, "attacker", payload, good, trusted, ErrInvalidSignature},
		{"re-dated", topic, sender, payload, mutate(func(s *Signed) { s.SignedAtUnixMs++ }), trusted, ErrInvalidSignature},
		{"flipped signature bit", topic, sender, payload, mutate(func(s *Signed) { s.Signature[10] ^= 1 }), trusted, ErrInvalidSignature},
		{"truncated signature", topic, sender, payload, mutate(func(s *Signed) { s.Signature = s.Signature[:100] }), trusted, ErrInvalidSignature},
		{"empty signature", topic, sender, payload, mutate(func(s *Signed) { s.Signature = nil }), trusted, ErrInvalidSignature},
		{"empty public key", topic, sender, payload, mutate(func(s *Signed) { s.PublicKey = nil }), trusted, ErrUntrustedKey},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := VerifyEvent(tc.topic, tc.sender, tc.payload, tc.sg, tc.trusted)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

// A trusted key that is garbage (wrong length) never verifies anything and
// never panics.
func TestVerifyEvent_MalformedTrustedKey(t *testing.T) {
	ts := newSigner(t)
	sg, _ := ts.Sign("drift.x", "s", []byte("p"))
	sg.PublicKey = []byte{1, 2, 3}
	if err := VerifyEvent("drift.x", "s", []byte("p"), sg, [][]byte{{1, 2, 3}}); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("err = %v, want ErrInvalidSignature", err)
	}
}

func TestCheckKey(t *testing.T) {
	ts := newSigner(t)
	sg, _ := ts.Sign("drift.x", "s", []byte("p"))
	if err := CheckKey(sg, [][]byte{ts.PublicKey()}); err != nil {
		t.Fatalf("trusted: %v", err)
	}
	if err := CheckKey(sg, [][]byte{newSigner(t).PublicKey()}); !errors.Is(err, ErrUntrustedKey) {
		t.Fatalf("untrusted: %v", err)
	}
}

func TestValidPublicKey(t *testing.T) {
	if !ValidPublicKey(newSigner(t).PublicKey()) {
		t.Fatal("a real key is not valid")
	}
	for _, b := range [][]byte{nil, {}, []byte("short"), make([]byte, 1953)} {
		if ValidPublicKey(b) {
			t.Errorf("accepted %d bytes", len(b))
		}
	}
}

func TestTopicSigner_Nil(t *testing.T) {
	var ts *TopicSigner
	if _, err := ts.Sign("drift.x", "s", []byte("p")); !errors.Is(err, ErrNoSigner) {
		t.Fatalf("nil signer: %v", err)
	}
	if NewTopicSigner(nil) != nil {
		t.Fatal("NewTopicSigner(nil) should be nil")
	}
	if ts.PublicKey() != nil {
		t.Fatal("nil signer has a public key")
	}
}

func TestTopicSigner_RefusesOtherParameterSets(t *testing.T) {
	_, s44, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA44)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewTopicSigner(s44).Sign("drift.x", "s", []byte("p")); !errors.Is(err, ErrWrongAlgorithm) {
		t.Fatalf("ML-DSA-44 signer: %v, want ErrWrongAlgorithm", err)
	}
}

// PublicKey hands out a copy: a caller editing it cannot change what the
// signer attaches to later signatures.
func TestTopicSigner_PublicKeyIsACopy(t *testing.T) {
	ts := newSigner(t)
	k := ts.PublicKey()
	k[0] ^= 0xff
	sg, _ := ts.Sign("drift.x", "s", []byte("p"))
	if bytes.Equal(k, sg.PublicKey) {
		t.Fatal("PublicKey returned the signer's own slice")
	}
}

func TestSignAndVerify_Concurrent(t *testing.T) {
	ts := newSigner(t)
	trusted := [][]byte{ts.PublicKey()}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sg, err := ts.Sign("drift.detected.n1", "s", []byte("p"))
			if err != nil {
				t.Error(err)
				return
			}
			if err := VerifyEvent("drift.detected.n1", "s", []byte("p"), sg, trusted); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}

// FuzzVerifyEvent: hostile signature fields never panic and never verify
// against a key that did not sign them.
func FuzzVerifyEvent(f *testing.F) {
	ts := newSigner(f)
	trusted := [][]byte{ts.PublicKey()}
	good, err := ts.SignAt("drift.detected.n1", "s", []byte("p"), time.UnixMilli(1700000000000))
	if err != nil {
		f.Fatal(err)
	}
	f.Add("drift.detected.n1", "s", []byte("p"), good.Signature, good.PublicKey, Algorithm, good.SignedAtUnixMs)
	f.Add("", "", []byte{}, []byte{}, []byte{}, "", int64(0))
	f.Add("drift.detected.n1", "s", []byte("p"), good.Signature[:10], good.PublicKey, Algorithm, int64(-1))
	f.Fuzz(func(t *testing.T, topic, sender string, payload, sig, pub []byte, alg string, at int64) {
		sg := Signed{Signature: sig, PublicKey: pub, Algorithm: alg, SignedAtUnixMs: at}
		err := VerifyEvent(topic, sender, payload, sg, trusted)
		if err != nil {
			return
		}
		// Accepted: it must be exactly the one message the trusted key signed.
		if topic != "drift.detected.n1" || sender != "s" || string(payload) != "p" ||
			at != good.SignedAtUnixMs || !bytes.Equal(pub, ts.PublicKey()) {
			t.Fatalf("accepted a message the key never signed: %q %q %q at=%d", topic, sender, payload, at)
		}
	})
}
