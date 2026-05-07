// SPDX-License-Identifier: GPL-3.0-or-later
// Package gungnir wraps cloudflare/circl ML-DSA-65 for signing and verifying
// GungnirSeal payloads in the Mímir's Law / Gleipnir Phase 0 PoC.
//
// Hard condition: ML-DSA-65 ONLY (no algorithm agility — blocks downgrade).
// Hard condition: HSM-grade key storage (this PoC uses a dev key file with 0600 perms;
// production must use TPM or sealed equivalent — see ADR-043 §Decision condition #4).
package gungnir

import (
	"crypto/sha256"
	"errors"
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
	return &Signer{
		priv:  priv,
		pub:   priv.Public().(*mldsa65.PublicKey),
		keyID: "dev-spike",
	}, nil
}

// Sign produces a GungnirSeal over the given payload.
func (s *Signer) Sign(payload []byte) (*GungnirSeal, error) {
	h := sha256.Sum256(payload)
	sig := make([]byte, mldsa65.SignatureSize)
	if err := mldsa65.SignTo(s.priv, h[:], nil, false, sig); err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	return &GungnirSeal{
		PayloadSha256: h[:],
		Mldsa65Sig:    sig,
		KeyId:         s.keyID,
		IssuedAtUnix:  now,
		ExpiresAtUnix: now + 3600, // 1 hour for dev
		Algorithm:     Algorithm,
	}, nil
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
	if !mldsa65.Verify(pubKey, h[:], nil, seal.Mldsa65Sig) {
		return ErrBadSignature
	}
	return nil
}
