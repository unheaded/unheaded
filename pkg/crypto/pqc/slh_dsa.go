// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package pqc

import (
	"fmt"

	"github.com/cloudflare/circl/sign/slhdsa"
)

// SLH-DSA (FIPS 205) - Stateless Hash-Based Digital Signature Algorithm.
//
// SLH-DSA is a conservative, hash-based signature scheme that relies only on
// the security of hash functions (no lattice assumptions). It produces larger
// signatures (up to ~49KB) but offers the strongest security guarantees against
// quantum computers.
//
// Implemented via cloudflare/circl (sign/slhdsa). All 12 parameter sets are
// wrapped, in SHA2 and SHAKE at each of:
//
//   - 128s: NIST security level 1 (small signatures)
//   - 128f: NIST security level 1 (fast signing)
//   - 192s: NIST security level 3 (small signatures)
//   - 192f: NIST security level 3 (fast signing)
//   - 256s: NIST security level 5 (small signatures)
//   - 256f: NIST security level 5 (fast signing)
//
// SLH-DSA is BPF-safe as it requires no floating point arithmetic.

// SLH-DSA parameter set constants. These map onto circl slhdsa.ID values.
// The SHAKE sets use PQC-00's ids; the SHA2 sets predate them and stay at
// 0x10-0x15 until one table owns the values (RFC desk item 8).
const (
	SLHDSA_SHAKE_128s ParameterSet = 0x07
	SLHDSA_SHAKE_128f ParameterSet = 0x08
	SLHDSA_SHAKE_192s ParameterSet = 0x09
	SLHDSA_SHAKE_192f ParameterSet = 0x0A
	SLHDSA_SHAKE_256s ParameterSet = 0x0B
	SLHDSA_SHAKE_256f ParameterSet = 0x0C

	SLHDSA_SHA2_128s ParameterSet = 0x10
	SLHDSA_SHA2_128f ParameterSet = 0x11
	SLHDSA_SHA2_192s ParameterSet = 0x12
	SLHDSA_SHA2_192f ParameterSet = 0x13
	SLHDSA_SHA2_256s ParameterSet = 0x14
	SLHDSA_SHA2_256f ParameterSet = 0x15
)

// slhdsaParamToID maps our ParameterSet constants to circl slhdsa.ID values.
var slhdsaParamToID = map[ParameterSet]slhdsa.ID{
	SLHDSA_SHAKE_128s: slhdsa.SHAKE_128s,
	SLHDSA_SHAKE_128f: slhdsa.SHAKE_128f,
	SLHDSA_SHAKE_192s: slhdsa.SHAKE_192s,
	SLHDSA_SHAKE_192f: slhdsa.SHAKE_192f,
	SLHDSA_SHAKE_256s: slhdsa.SHAKE_256s,
	SLHDSA_SHAKE_256f: slhdsa.SHAKE_256f,

	SLHDSA_SHA2_128s: slhdsa.SHA2_128s,
	SLHDSA_SHA2_128f: slhdsa.SHA2_128f,
	SLHDSA_SHA2_192s: slhdsa.SHA2_192s,
	SLHDSA_SHA2_192f: slhdsa.SHA2_192f,
	SLHDSA_SHA2_256s: slhdsa.SHA2_256s,
	SLHDSA_SHA2_256f: slhdsa.SHA2_256f,
}

// slhdsaParamInfo holds parameter-set-specific sizes for SLH-DSA.
var slhdsaParamInfo = map[ParameterSet]struct {
	Name           string
	SignatureSize  int
	PublicKeySize  int
	PrivateKeySize int
}{
	SLHDSA_SHAKE_128s: {Name: "SLH-DSA-SHAKE-128s", SignatureSize: 7856, PublicKeySize: 32, PrivateKeySize: 64},
	SLHDSA_SHAKE_128f: {Name: "SLH-DSA-SHAKE-128f", SignatureSize: 17088, PublicKeySize: 32, PrivateKeySize: 64},
	SLHDSA_SHAKE_192s: {Name: "SLH-DSA-SHAKE-192s", SignatureSize: 16224, PublicKeySize: 48, PrivateKeySize: 96},
	SLHDSA_SHAKE_192f: {Name: "SLH-DSA-SHAKE-192f", SignatureSize: 35664, PublicKeySize: 48, PrivateKeySize: 96},
	SLHDSA_SHAKE_256s: {Name: "SLH-DSA-SHAKE-256s", SignatureSize: 29792, PublicKeySize: 64, PrivateKeySize: 128},
	SLHDSA_SHAKE_256f: {Name: "SLH-DSA-SHAKE-256f", SignatureSize: 49856, PublicKeySize: 64, PrivateKeySize: 128},

	SLHDSA_SHA2_128s: {Name: "SLH-DSA-SHA2-128s", SignatureSize: 7856, PublicKeySize: 32, PrivateKeySize: 64},
	SLHDSA_SHA2_128f: {Name: "SLH-DSA-SHA2-128f", SignatureSize: 17088, PublicKeySize: 32, PrivateKeySize: 64},
	SLHDSA_SHA2_192s: {Name: "SLH-DSA-SHA2-192s", SignatureSize: 16224, PublicKeySize: 48, PrivateKeySize: 96},
	SLHDSA_SHA2_192f: {Name: "SLH-DSA-SHA2-192f", SignatureSize: 35664, PublicKeySize: 48, PrivateKeySize: 96},
	SLHDSA_SHA2_256s: {Name: "SLH-DSA-SHA2-256s", SignatureSize: 29792, PublicKeySize: 64, PrivateKeySize: 128},
	SLHDSA_SHA2_256f: {Name: "SLH-DSA-SHA2-256f", SignatureSize: 49856, PublicKeySize: 64, PrivateKeySize: 128},
}

// SLHDSASigner implements the Signer interface for SLH-DSA (FIPS 205).
type SLHDSASigner struct {
	params ParameterSet
	sk     *slhdsa.PrivateKey
	pk     *slhdsa.PublicKey
}

// Sign produces an SLH-DSA signature over message using deterministic signing.
func (s *SLHDSASigner) Sign(message []byte) ([]byte, error) {
	if s.sk == nil {
		return nil, ErrAlgorithmNotAvailable
	}
	msg := slhdsa.NewMessage(message)
	sig, err := slhdsa.SignDeterministic(s.sk, msg, nil)
	if err != nil {
		return nil, fmt.Errorf("pqc: SLH-DSA sign: %w", err)
	}
	return sig, nil
}

// PublicKey returns the packed public key bytes.
func (s *SLHDSASigner) PublicKey() []byte {
	if s.pk == nil {
		return nil
	}
	b, err := s.pk.MarshalBinary()
	if err != nil {
		return nil
	}
	return b
}

// AlgoID returns AlgoSLHDSA.
func (s *SLHDSASigner) AlgoID() AlgoID {
	return AlgoSLHDSA
}

// ParameterSetID returns the parameter set used by this signer.
func (s *SLHDSASigner) ParameterSetID() ParameterSet {
	return s.params
}

// SLHDSAVerifier implements the Verifier interface for SLH-DSA (FIPS 205).
type SLHDSAVerifier struct {
	params ParameterSet
	id     slhdsa.ID
}

// NewSLHDSAVerifier creates a new SLH-DSA verifier for the given parameter set.
func NewSLHDSAVerifier(params ParameterSet) (*SLHDSAVerifier, error) {
	id, ok := slhdsaParamToID[params]
	if !ok {
		return nil, ErrInvalidParameterSet
	}
	return &SLHDSAVerifier{params: params, id: id}, nil
}

// Verify checks whether signature is valid for message under publicKey.
func (v *SLHDSAVerifier) Verify(message, signature, publicKey []byte) (bool, error) {
	info, ok := slhdsaParamInfo[v.params]
	if !ok {
		return false, ErrInvalidParameterSet
	}
	if len(publicKey) != info.PublicKeySize {
		return false, ErrInvalidKeySize
	}
	if len(signature) != info.SignatureSize {
		return false, ErrInvalidSignatureSize
	}

	pk := slhdsa.PublicKey{ID: v.id}
	if err := pk.UnmarshalBinary(publicKey); err != nil {
		return false, fmt.Errorf("pqc: SLH-DSA unmarshal public key: %w", err)
	}

	msg := slhdsa.NewMessage(message)
	return slhdsa.Verify(&pk, msg, signature, nil), nil
}

// AlgoID returns AlgoSLHDSA.
func (v *SLHDSAVerifier) AlgoID() AlgoID {
	return AlgoSLHDSA
}

// GenerateSLHDSAKeyPair generates an SLH-DSA key pair for the specified
// parameter set.
func GenerateSLHDSAKeyPair(params ParameterSet) (*KeyPair, *SLHDSASigner, error) {
	id, ok := slhdsaParamToID[params]
	if !ok {
		return nil, nil, ErrInvalidParameterSet
	}

	pk, sk, err := slhdsa.GenerateKey(nil, id)
	if err != nil {
		return nil, nil, fmt.Errorf("pqc: generate SLH-DSA key: %w", err)
	}

	pkBytes, err := pk.MarshalBinary()
	if err != nil {
		return nil, nil, fmt.Errorf("pqc: marshal SLH-DSA public key: %w", err)
	}
	skBytes, err := sk.MarshalBinary()
	if err != nil {
		return nil, nil, fmt.Errorf("pqc: marshal SLH-DSA private key: %w", err)
	}

	signer := &SLHDSASigner{
		params: params,
		sk:     &sk,
		pk:     &pk,
	}
	kp := &KeyPair{
		AlgoID:    AlgoSLHDSA,
		PublicKey: pkBytes,
		SecretKey: skBytes,
	}
	return kp, signer, nil
}

// SLHDSAParamInfo returns the parameter set info for a given SLH-DSA parameter
// set. Returns false if the parameter set is unknown.
func SLHDSAParamInfo(ps ParameterSet) (name string, sigSize, pkSize, skSize int, ok bool) {
	info, found := slhdsaParamInfo[ps]
	if !found {
		return "", 0, 0, 0, false
	}
	return info.Name, info.SignatureSize, info.PublicKeySize, info.PrivateKeySize, true
}
