// SPDX-License-Identifier: GPL-3.0-or-later
package gungnir

import (
	"testing"
	"time"
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

// TestVerify_MetadataIsSigned: a seal's expiry, issue time and key id are
// covered by the signature. The first version signed only the payload
// hash, so a stale seal could be made fresh by editing ExpiresAtUnix.
func TestVerify_MetadataIsSigned(t *testing.T) {
	s, err := NewSigner(GenerateDevKey())
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("baseline")
	fresh := func() *GungnirSeal {
		seal, err := s.SignFor(payload, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		return seal
	}
	if err := Verify(payload, fresh(), s.pub); err != nil {
		t.Fatalf("untouched seal: %v", err)
	}
	for name, edit := range map[string]func(*GungnirSeal){
		"expiry extended": func(g *GungnirSeal) { g.ExpiresAtUnix += 365 * 24 * 3600 },
		"issued moved":    func(g *GungnirSeal) { g.IssuedAtUnix -= 60 },
		"key id swapped":  func(g *GungnirSeal) { g.KeyId = "someone-else" },
	} {
		seal := fresh()
		edit(seal)
		if err := Verify(payload, seal, s.pub); err != ErrBadSignature {
			t.Errorf("%s: want ErrBadSignature, got %v", name, err)
		}
	}
}
