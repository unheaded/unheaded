// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package pqc

import (
	"bytes"
	"errors"
	"testing"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"
)

// A signer built from a key loaded elsewhere (a seed file) signs exactly as
// the key does: same public key, and its signatures verify.
func TestNewMLDSASignerFromPrivateKey(t *testing.T) {
	var seed [mldsa65.SeedSize]byte
	seed[0] = 7
	pub, priv := mldsa65.NewKeyFromSeed(&seed)

	s, err := NewMLDSASignerFromPrivateKey(priv)
	if err != nil {
		t.Fatalf("NewMLDSASignerFromPrivateKey: %v", err)
	}
	if s.ParameterSetID() != MLDSA65 {
		t.Fatalf("parameter set = %v, want MLDSA65", s.ParameterSetID())
	}
	if !bytes.Equal(s.PublicKey(), pub.Bytes()) {
		t.Fatal("public key differs from the private key's")
	}
	msg := []byte("drift.detected.node-1")
	sig, err := s.Sign(msg)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	v, _ := NewMLDSAVerifier(MLDSA65)
	ok, err := v.Verify(msg, sig, pub.Bytes())
	if err != nil || !ok {
		t.Fatalf("verify: ok=%v err=%v", ok, err)
	}
}

func TestNewMLDSASignerFromPrivateKey_Nil(t *testing.T) {
	if _, err := NewMLDSASignerFromPrivateKey(nil); !errors.Is(err, ErrNilPrivateKey) {
		t.Fatalf("nil key: err = %v, want ErrNilPrivateKey", err)
	}
}
