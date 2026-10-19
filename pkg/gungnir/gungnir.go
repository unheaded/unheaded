// SPDX-License-Identifier: GPL-3.0-or-later
// Package gungnir wraps cloudflare/circl ML-DSA-65 for signing and verifying
// GungnirSeal payloads in the Mímir's Law / Gleipnir Phase 0 PoC.
//
// Hard condition: ML-DSA-65 ONLY (no algorithm agility — blocks downgrade).
// Hard condition: HSM-grade key storage (this PoC uses a dev key file with 0600 perms;
// production must use TPM or sealed equivalent — see ADR-043 §Decision condition #4).
package gungnir

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"
)

const Algorithm = "ml-dsa-65"

var (
	ErrNilSeal      = errors.New("gungnir: nil seal")
	ErrWrongAlgo    = errors.New("gungnir: wrong algorithm (only ml-dsa-65 accepted)")
	ErrSealExpired  = errors.New("gungnir: seal expired")
	ErrPayloadHash  = errors.New("gungnir: payload hash mismatch")
	ErrBadSignature = errors.New("gungnir: signature verification failed")
)

// GungnirSeal is the ML-DSA-65 signature wrapper for Mjölnir manifests
// and config deltas. Frozen field layout for protobuf compatibility.
type GungnirSeal struct {
	PayloadSha256 []byte // hash of the signed payload
	Mldsa65Sig    []byte // ML-DSA-65 signature bytes
	KeyId         string // identifier of signing key
	IssuedAtUnix  int64  // signing timestamp
	ExpiresAtUnix int64  // expiry (forces freshness)
	Algorithm     string // "ml-dsa-65" — explicit, blocks downgrade
}

// Signer holds an ML-DSA-65 private key and signs payloads.
type Signer struct {
	priv  *mldsa65.PrivateKey
	pub   *mldsa65.PublicKey
	keyID string
}

// GenerateDevKey creates an ephemeral ML-DSA-65 key for spike testing.
// Production must load from HSM/TPM/sealed storage per ADR-043 #4.
func GenerateDevKey() *mldsa65.PrivateKey {
	_, priv, err := mldsa65.GenerateKey(nil)
	if err != nil {
		panic(err)
	}
	return priv
}

// NewSigner creates a Signer from a private key.
func NewSigner(priv *mldsa65.PrivateKey) (*Signer, error) {
	if priv == nil {
		return nil, errors.New("gungnir: nil private key")
	}
	pub, ok := priv.Public().(*mldsa65.PublicKey)
	if !ok {
		return nil, fmt.Errorf("gungnir: private key Public() did not return *mldsa65.PublicKey (got %T)", priv.Public())
	}
	return &Signer{
		priv:  priv,
		pub:   pub,
		keyID: "dev-spike",
	}, nil
}

// sealContext is ML-DSA's context string (FIPS 204): it keeps these
// signatures apart from anything else the same key might sign.
const sealContext = "unheaded/gungnir/seal/v2"

// signedMessage is what a seal's signature covers: every field Verify acts
// on, each length-prefixed. The first version signed only the payload hash,
// so expiry, issue time, key ID and algorithm could be edited on a valid
// seal: a stale seal could be made fresh again by rewriting ExpiresAtUnix.
func signedMessage(seal *GungnirSeal) []byte {
	var b bytes.Buffer
	field := func(p []byte) {
		_ = binary.Write(&b, binary.BigEndian, uint32(len(p))) // #nosec G115 -- fields are a hash, a short name and a key id
		b.Write(p)
	}
	field([]byte(seal.Algorithm))
	field([]byte(seal.KeyId))
	_ = binary.Write(&b, binary.BigEndian, seal.IssuedAtUnix)
	_ = binary.Write(&b, binary.BigEndian, seal.ExpiresAtUnix)
	field(seal.PayloadSha256)
	return b.Bytes()
}

// Sign produces a GungnirSeal over the given payload, valid for one hour.
func (s *Signer) Sign(payload []byte) (*GungnirSeal, error) {
	return s.sign(payload, time.Hour)
}

func (s *Signer) sign(payload []byte, ttl time.Duration) (*GungnirSeal, error) {
	h := sha256.Sum256(payload)
	now := time.Now().Unix()
	seal := &GungnirSeal{
		PayloadSha256: h[:],
		KeyId:         s.keyID,
		IssuedAtUnix:  now,
		ExpiresAtUnix: now + int64(ttl/time.Second),
		Algorithm:     Algorithm,
	}
	sig := make([]byte, mldsa65.SignatureSize)
	if err := mldsa65.SignTo(s.priv, signedMessage(seal), []byte(sealContext), false, sig); err != nil {
		return nil, err
	}
	seal.Mldsa65Sig = sig
	return seal, nil
}

// PublicKey returns the signer's public key bytes.
func (s *Signer) PublicKey() []byte {
	return s.pub.Bytes()
}

// Verify checks a seal against a payload using the embedded signature.
// Rejects: tampered payload, wrong algorithm, expired, missing fields.
func Verify(payload []byte, seal *GungnirSeal, pubKey *mldsa65.PublicKey) error {
	if seal == nil {
		return ErrNilSeal
	}
	if seal.Algorithm != Algorithm {
		return ErrWrongAlgo
	}
	if seal.ExpiresAtUnix < time.Now().Unix() {
		return ErrSealExpired
	}
	h := sha256.Sum256(payload)
	if string(h[:]) != string(seal.PayloadSha256) {
		return ErrPayloadHash
	}
	if !mldsa65.Verify(pubKey, signedMessage(seal), []byte(sealContext), seal.Mldsa65Sig) {
		return ErrBadSignature
	}
	return nil
}
