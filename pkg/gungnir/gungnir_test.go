// SPDX-License-Identifier: GPL-3.0-or-later
package gungnir

import (
	"testing"
)

func TestSignVerifyRoundTrip(t *testing.T) {
	priv := GenerateDevKey()
	s, err := NewSigner(priv)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	payload := []byte("hello mímir")
	seal, err := s.Sign(payload)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := Verify(payload, seal, s.pub); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestVerifyRejectsTamperedPayload(t *testing.T) {
	s, _ := NewSigner(GenerateDevKey())
	seal, _ := s.Sign([]byte("original"))
	if err := Verify([]byte("tampered"), seal, s.pub); err == nil {
		t.Fatal("expected verification failure on tampered payload")
	}
}

func TestVerifyRejectsWrongAlgorithm(t *testing.T) {
	s, _ := NewSigner(GenerateDevKey())
	seal, _ := s.Sign([]byte("payload"))
	seal.Algorithm = "hmac-sha256"
	if err := Verify([]byte("payload"), seal, s.pub); err != ErrWrongAlgo {
		t.Fatalf("expected ErrWrongAlgo, got %v", err)
	}
}

func TestVerifyRejectsExpiredSeal(t *testing.T) {
	s, _ := NewSigner(GenerateDevKey())
	seal, _ := s.Sign([]byte("payload"))
	seal.ExpiresAtUnix = 1
	if err := Verify([]byte("payload"), seal, s.pub); err != ErrSealExpired {
		t.Fatalf("expected ErrSealExpired, got %v", err)
	}
}
