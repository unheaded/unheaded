// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package pqc

import (
	"bytes"
	"testing"
)

func TestDeriveSigningKey(t *testing.T) {
	ss := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	flowID := []byte("flow-test-001")

	key1, err := DeriveSigningKey(ss, nil, flowID, 1)
	if err != nil {
		t.Fatalf("DeriveSigningKey: %v", err)
	}

	// Same inputs produce same key.
	key2, err := DeriveSigningKey(ss, nil, flowID, 1)
	if err != nil {
		t.Fatalf("DeriveSigningKey: %v", err)
	}
	if key1 != key2 {
		t.Fatal("same inputs should produce same key")
	}

	// Different epoch produces different key.
	key3, err := DeriveSigningKey(ss, nil, flowID, 2)
	if err != nil {
		t.Fatalf("DeriveSigningKey: %v", err)
	}
	if key1 == key3 {
		t.Fatal("different epoch should produce different key")
	}

	// Different flow produces different key.
	key4, err := DeriveSigningKey(ss, nil, []byte("flow-test-002"), 1)
	if err != nil {
		t.Fatalf("DeriveSigningKey: %v", err)
	}
	if key1 == key4 {
		t.Fatal("different flow should produce different key")
	}

	// Key should not be all zeros.
	var zero [32]byte
	if key1 == zero {
		t.Fatal("derived key should not be all zeros")
	}
}

func TestDeriveSigningKey_WithSalt(t *testing.T) {
	ss := [32]byte{42}
	flowID := []byte("salted-flow")
	salt := []byte("test-salt-value")

	key1, err := DeriveSigningKey(ss, nil, flowID, 1)
	if err != nil {
		t.Fatalf("without salt: %v", err)
	}

	key2, err := DeriveSigningKey(ss, salt, flowID, 1)
	if err != nil {
		t.Fatalf("with salt: %v", err)
	}

	if key1 == key2 {
		t.Fatal("salt should change derived key")
	}
}

func TestDeriveTunnelKeys(t *testing.T) {
	ss := [32]byte{10, 20, 30, 40, 50}

	ks, err := DeriveTunnelKeys(ss, nil, "test-tunnel")
	if err != nil {
		t.Fatalf("DeriveTunnelKeys: %v", err)
	}

	// All three keys should be different.
	if ks.SigningKey == ks.AuthKey {
		t.Fatal("signing and auth keys should differ")
	}
	if bytes.Equal(ks.SigningKey[:], ks.IVKey[:]) {
		t.Fatal("signing and IV keys should differ")
	}

	// Zero should wipe all.
	ks.Zero()
	var zeroSK [32]byte
	var zeroIV [16]byte
	if ks.SigningKey != zeroSK {
		t.Fatal("signing key not zeroed")
	}
	if ks.AuthKey != zeroSK {
		t.Fatal("auth key not zeroed")
	}
	if ks.IVKey != zeroIV {
		t.Fatal("IV key not zeroed")
	}
}

func TestTunnelNegotiator_FullHandshake(t *testing.T) {
	params := []struct {
		name string
		ps   ParameterSet
	}{
		{"ML-KEM-512", MLKEM512},
		{"ML-KEM-768", MLKEM768},
		{"ML-KEM-1024", MLKEM1024},
	}

	for _, tt := range params {
		t.Run(tt.name, func(t *testing.T) {
			// Generate responder key pair.
			kp, err := GenerateMLKEMKeyPair(tt.ps)
			if err != nil {
				t.Fatalf("GenerateMLKEMKeyPair: %v", err)
			}

			negotiator, err := NewTunnelNegotiator(tt.ps)
			if err != nil {
				t.Fatalf("NewTunnelNegotiator: %v", err)
			}

			salt := []byte("handshake-salt")
			ctx := "test-context"

			// Initiator handshake.
			initResult, err := negotiator.InitiatorHandshake(kp.PublicKey, salt, ctx)
			if err != nil {
				t.Fatalf("InitiatorHandshake: %v", err)
			}
			defer initResult.Zero()

			// Responder handshake.
			respResult, err := negotiator.ResponderHandshake(initResult.Ciphertext, kp.SecretKey, salt, ctx)
			if err != nil {
				t.Fatalf("ResponderHandshake: %v", err)
			}
			defer respResult.Zero()

			// Shared secrets must match.
			if initResult.SharedSecret != respResult.SharedSecret {
				t.Fatal("shared secrets do not match")
			}

			// Derived keys must match.
			if initResult.Keys.SigningKey != respResult.Keys.SigningKey {
				t.Fatal("signing keys do not match")
			}
			if initResult.Keys.AuthKey != respResult.Keys.AuthKey {
				t.Fatal("auth keys do not match")
			}
			if initResult.Keys.IVKey != respResult.Keys.IVKey {
				t.Fatal("IV keys do not match")
			}
		})
	}
}

func TestKEMTunnelRateLimiter(t *testing.T) {
	rl := NewKEMTunnelRateLimiter(3)

	if !rl.Allow() {
		t.Fatal("first request should be allowed")
	}
	if !rl.Allow() {
		t.Fatal("second request should be allowed")
	}
	if !rl.Allow() {
		t.Fatal("third request should be allowed")
	}
	if rl.Allow() {
		t.Fatal("fourth request should be rate-limited")
	}

	if rl.Count() != 3 {
		t.Fatalf("count = %d, want 3", rl.Count())
	}
}

func TestHandshakeResult_Zero(t *testing.T) {
	hr := &HandshakeResult{
		SharedSecret: [32]byte{1, 2, 3},
		Keys: &TunnelKeySet{
			SigningKey: [32]byte{4, 5, 6},
			AuthKey:    [32]byte{7, 8, 9},
			IVKey:      [16]byte{10, 11, 12},
		},
	}

	hr.Zero()

	var zero32 [32]byte
	var zero16 [16]byte
	if hr.SharedSecret != zero32 {
		t.Fatal("shared secret not zeroed")
	}
	if hr.Keys.SigningKey != zero32 {
		t.Fatal("signing key not zeroed")
	}
	if hr.Keys.AuthKey != zero32 {
		t.Fatal("auth key not zeroed")
	}
	if hr.Keys.IVKey != zero16 {
		t.Fatal("IV key not zeroed")
	}
}
