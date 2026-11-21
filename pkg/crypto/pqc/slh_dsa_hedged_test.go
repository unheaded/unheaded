// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package pqc

import (
	"bytes"
	"errors"
	"testing"

	"github.com/cloudflare/circl/sign/slhdsa"
)

// signDeterministic is FIPS 205's deterministic variant (opt_rand = PK.seed),
// kept for known-answer comparisons only: it lives in a test file so that no
// production caller can reach it.
func (s *SLHDSASigner) signDeterministic(message []byte) ([]byte, error) {
	return slhdsa.SignDeterministic(s.sk, slhdsa.NewMessage(message), nil)
}

type failingReader struct{}

var errNoEntropy = errors.New("no entropy")

func (failingReader) Read([]byte) (int, error) { return 0, errNoEntropy }

func mustVerifySLH(t *testing.T, params ParameterSet, msg, sig, pub []byte, what string) {
	t.Helper()
	v, err := NewSLHDSAVerifier(params)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := v.Verify(msg, sig, pub)
	if err != nil || !ok {
		t.Fatalf("%s does not verify: ok=%v err=%v", what, ok, err)
	}
}

// Sign is FIPS 205's hedged variant: fresh randomness goes into every
// signature, so two over one message differ, and both verify. The
// deterministic variant repeats itself and is neither of them.
func TestSLHDSASign_IsHedged(t *testing.T) {
	for _, params := range []ParameterSet{SLHDSA_SHA2_128f, SLHDSA_SHAKE_128f} {
		t.Run(slhdsaParamInfo[params].Name, func(t *testing.T) {
			kp, signer, err := GenerateSLHDSAKeyPair(params)
			if err != nil {
				t.Fatal(err)
			}
			msg := []byte("one message, signed twice")

			a, err := signer.Sign(msg)
			if err != nil {
				t.Fatal(err)
			}
			b, err := signer.Sign(msg)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Equal(a, b) {
				t.Fatal("two signatures over one message are identical: Sign is deterministic, not hedged")
			}
			mustVerifySLH(t, params, msg, a, kp.PublicKey, "the first signature")
			mustVerifySLH(t, params, msg, b, kp.PublicKey, "the second signature")

			d1, err := signer.signDeterministic(msg)
			if err != nil {
				t.Fatal(err)
			}
			d2, err := signer.signDeterministic(msg)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(d1, d2) {
				t.Fatal("the deterministic variant did not repeat itself")
			}
			if bytes.Equal(a, d1) || bytes.Equal(b, d1) {
				t.Fatal("a hedged signature equals the deterministic one")
			}
			mustVerifySLH(t, params, msg, d1, kp.PublicKey, "the deterministic signature")
		})
	}
}

// With no randomness there is no signature: Sign must not fall back to the
// deterministic variant.
func TestSLHDSASign_FailsWithoutRandomness(t *testing.T) {
	_, signer, err := GenerateSLHDSAKeyPair(SLHDSA_SHA2_128f)
	if err != nil {
		t.Fatal(err)
	}
	signer.random = failingReader{}
	sig, err := signer.Sign([]byte("message"))
	if !errors.Is(err, errNoEntropy) {
		t.Fatalf("err = %v, want the reader's error", err)
	}
	if sig != nil {
		t.Fatalf("a signature came back with no randomness: %d bytes", len(sig))
	}
}

// The randomness read is the parameter set's n bytes and no more.
func TestSLHDSASign_ReadsNBytes(t *testing.T) {
	_, signer, err := GenerateSLHDSAKeyPair(SLHDSA_SHA2_128f)
	if err != nil {
		t.Fatal(err)
	}
	r := &countingReader{}
	signer.random = r
	if _, err := signer.Sign([]byte("message")); err != nil {
		t.Fatal(err)
	}
	if r.n != 16 { // n for the 128-bit sets
		t.Errorf("read %d random bytes, want 16", r.n)
	}
}

type countingReader struct{ n int }

func (c *countingReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(c.n + i + 1)
	}
	c.n += len(p)
	return len(p), nil
}
