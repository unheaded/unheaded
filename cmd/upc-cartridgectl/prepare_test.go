// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"

	"unheaded/pkg/gungnir"
)

// testImage builds a valid one-instruction XDP image (HALT).
func testImage(t *testing.T, word uint32) []byte {
	t.Helper()
	b := []byte{'M', 'B', 'C', 'C', 0, 0}
	b = binary.LittleEndian.AppendUint16(b, 64)
	b = binary.LittleEndian.AppendUint16(b, 1)
	b = binary.LittleEndian.AppendUint16(b, 0)
	b = binary.LittleEndian.AppendUint16(b, 0)
	b = binary.LittleEndian.AppendUint16(b, 0)
	b = binary.LittleEndian.AppendUint16(b, 0)
	b = binary.LittleEndian.AppendUint16(b, 0)
	body := binary.LittleEndian.AppendUint32(nil, word)
	h := sha256.New()
	h.Write(b[:20])
	h.Write(body)
	b = append(b, h.Sum(nil)...)
	return append(b, body...)
}

func testKey(t *testing.T, seedByte byte) (*gungnir.Signer, *mldsa65.PublicKey) {
	t.Helper()
	var seed [mldsa65.SeedSize]byte
	for i := range seed {
		seed[i] = seedByte
	}
	pub, priv := mldsa65.NewKeyFromSeed(&seed)
	s, err := gungnir.NewSignerWithID(priv)
	if err != nil {
		t.Fatal(err)
	}
	return s, pub
}

func sealOf(t *testing.T, s *gungnir.Signer, payload []byte, ttl time.Duration) []byte {
	t.Helper()
	seal, err := s.SignFor(payload, ttl)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(seal)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func okValidator([]byte) error { return nil }

func TestPrepare_SignedAndValid(t *testing.T) {
	t.Parallel()
	s, pub := testKey(t, 1)
	raw := testImage(t, 0xFF000000)
	var validated []byte
	p, err := Prepare(raw, sealOf(t, s, raw, time.Hour), pub, false, func(b []byte) error { validated = b; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if p.Lab || p.KeyID != gungnir.KeyID(pub) || p.Image == nil || len(p.Image.ROM) != 1 {
		t.Fatalf("prepared %+v", p)
	}
	// TOCTOU: the validator saw, and the image came from, the very bytes the
	// seal covers — the same backing array, not a re-read.
	if &validated[0] != &raw[0] || &p.Raw[0] != &raw[0] {
		t.Fatal("validator or image did not use the verified in-memory bytes")
	}
}

func TestPrepare_Refusals(t *testing.T) {
	t.Parallel()
	s, pub := testKey(t, 1)
	other, _ := testKey(t, 2)
	raw := testImage(t, 0xFF000000)
	otherRaw := testImage(t, 0x00000000)
	cases := []struct {
		name    string
		seal    []byte
		pub     *mldsa65.PublicKey
		lab     bool
		val     func([]byte) error
		wantErr error
	}{
		{"unsigned", nil, pub, false, okValidator, ErrUnsigned},
		{"seal for other bytes", sealOf(t, s, otherRaw, time.Hour), pub, false, okValidator, gungnir.ErrPayloadHash},
		{"wrong key", sealOf(t, other, raw, time.Hour), pub, false, okValidator, gungnir.ErrBadSignature},
		{"no pinned key", sealOf(t, s, raw, time.Hour), nil, false, okValidator, ErrNoKey},
		{"garbage seal", []byte("{not json"), pub, false, okValidator, ErrSeal},
		{"validator reject", sealOf(t, s, raw, time.Hour), pub, false, func([]byte) error { return errors.New("pc 0: opcode 0x04 not allowed") }, ErrInvalid},
	}
	for _, c := range cases {
		if _, err := Prepare(raw, c.seal, c.pub, c.lab, c.val); !errors.Is(err, c.wantErr) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.wantErr)
		}
	}
	// An expired seal: sign with a 1s TTL and move the clock past it.
	sealNow := sealOf(t, s, raw, time.Second)
	var g gungnir.GungnirSeal
	_ = json.Unmarshal(sealNow, &g)
	g.ExpiresAtUnix = time.Now().Add(-time.Minute).Unix() // tampering also breaks the signature; either way refused
	expired, _ := json.Marshal(g)
	if _, err := Prepare(raw, expired, pub, false, okValidator); err == nil {
		t.Error("expired seal accepted")
	}
	// Bytes that are not an image never reach the maps even when signed.
	junk := []byte("not a cartridge")
	if _, err := Prepare(junk, sealOf(t, s, junk, time.Hour), pub, false, okValidator); !errors.Is(err, ErrInvalid) {
		t.Errorf("junk: %v", err)
	}
}

func TestPrepare_LabAllowsUnsigned(t *testing.T) {
	t.Parallel()
	raw := testImage(t, 0xFF000000)
	p, err := Prepare(raw, nil, nil, true, okValidator)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Lab || p.KeyID != "" {
		t.Fatalf("lab load not marked: %+v", p)
	}
	// --lab never skips the validator.
	if _, err := Prepare(raw, nil, nil, true, func([]byte) error { return errors.New("bad") }); !errors.Is(err, ErrInvalid) {
		t.Fatalf("lab skipped validation: %v", err)
	}
}
