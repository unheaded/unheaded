// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package pqc

import (
	"fmt"

	"github.com/cloudflare/circl/sign/mldsa/mldsa44"
	"github.com/cloudflare/circl/sign/mldsa/mldsa65"
	"github.com/cloudflare/circl/sign/mldsa/mldsa87"
)

// MLDSASigner implements the Signer interface for ML-DSA (FIPS 204).
//
// ML-DSA is the standardized name for the CRYSTALS-Dilithium lattice-based
// digital signature scheme. It supports three parameter sets:
//   - ML-DSA-44: NIST security level 2
//   - ML-DSA-65: NIST security level 3
//   - ML-DSA-87: NIST security level 5
type MLDSASigner struct {
	params ParameterSet

	// Internal key storage. Exactly one of these will be non-nil based on
	// the parameter set.
	sk44 *mldsa44.PrivateKey
	sk65 *mldsa65.PrivateKey
	sk87 *mldsa87.PrivateKey

	pk44 *mldsa44.PublicKey
	pk65 *mldsa65.PublicKey
	pk87 *mldsa87.PublicKey
}

// Sign produces an ML-DSA signature over message.
func (s *MLDSASigner) Sign(message []byte) ([]byte, error) {
	switch s.params {
	case MLDSA44:
		sig := make([]byte, mldsa44.SignatureSize)
		if err := mldsa44.SignTo(s.sk44, message, nil, false, sig); err != nil {
			return nil, fmt.Errorf("pqc: ML-DSA-44 sign: %w", err)
		}
		return sig, nil
	case MLDSA65:
		sig := make([]byte, mldsa65.SignatureSize)
		if err := mldsa65.SignTo(s.sk65, message, nil, false, sig); err != nil {
			return nil, fmt.Errorf("pqc: ML-DSA-65 sign: %w", err)
		}
		return sig, nil
	case MLDSA87:
		sig := make([]byte, mldsa87.SignatureSize)
		if err := mldsa87.SignTo(s.sk87, message, nil, false, sig); err != nil {
			return nil, fmt.Errorf("pqc: ML-DSA-87 sign: %w", err)
		}
		return sig, nil
	default:
		return nil, ErrInvalidParameterSet
	}
}

// PublicKey returns the packed public key bytes.
func (s *MLDSASigner) PublicKey() []byte {
	switch s.params {
	case MLDSA44:
		return s.pk44.Bytes()
	case MLDSA65:
		return s.pk65.Bytes()
	case MLDSA87:
		return s.pk87.Bytes()
	default:
		return nil
	}
}

// AlgoID returns AlgoMLDSA.
func (s *MLDSASigner) AlgoID() AlgoID {
	return AlgoMLDSA
}

// ParameterSetID returns the parameter set used by this signer.
func (s *MLDSASigner) ParameterSetID() ParameterSet {
	return s.params
}

// MLDSAVerifier implements the Verifier interface for ML-DSA (FIPS 204).
type MLDSAVerifier struct {
	params ParameterSet
}

// NewMLDSAVerifier creates a new ML-DSA verifier for the given parameter set.
func NewMLDSAVerifier(params ParameterSet) (*MLDSAVerifier, error) {
	if _, ok := mldsaParamInfo[params]; !ok {
		return nil, ErrInvalidParameterSet
	}
	return &MLDSAVerifier{params: params}, nil
}

// Verify checks whether signature is valid for message under publicKey.
func (v *MLDSAVerifier) Verify(message, signature, publicKey []byte) (bool, error) {
	switch v.params {
	case MLDSA44:
		if len(publicKey) != mldsa44.PublicKeySize {
			return false, ErrInvalidKeySize
		}
		if len(signature) != mldsa44.SignatureSize {
			return false, ErrInvalidSignatureSize
		}
		var pk mldsa44.PublicKey
		if err := pk.UnmarshalBinary(publicKey); err != nil {
			return false, fmt.Errorf("pqc: ML-DSA-44 unmarshal public key: %w", err)
		}
		return mldsa44.Verify(&pk, message, nil, signature), nil
	case MLDSA65:
		if len(publicKey) != mldsa65.PublicKeySize {
			return false, ErrInvalidKeySize
		}
		if len(signature) != mldsa65.SignatureSize {
			return false, ErrInvalidSignatureSize
		}
		var pk mldsa65.PublicKey
		if err := pk.UnmarshalBinary(publicKey); err != nil {
			return false, fmt.Errorf("pqc: ML-DSA-65 unmarshal public key: %w", err)
		}
		return mldsa65.Verify(&pk, message, nil, signature), nil
	case MLDSA87:
		if len(publicKey) != mldsa87.PublicKeySize {
			return false, ErrInvalidKeySize
		}
		if len(signature) != mldsa87.SignatureSize {
			return false, ErrInvalidSignatureSize
		}
		var pk mldsa87.PublicKey
		if err := pk.UnmarshalBinary(publicKey); err != nil {
			return false, fmt.Errorf("pqc: ML-DSA-87 unmarshal public key: %w", err)
		}
		return mldsa87.Verify(&pk, message, nil, signature), nil
	default:
		return false, ErrInvalidParameterSet
	}
}

// AlgoID returns AlgoMLDSA.
func (v *MLDSAVerifier) AlgoID() AlgoID {
	return AlgoMLDSA
}

// GenerateMLDSAKeyPair generates an ML-DSA key pair for the specified
// parameter set.
func GenerateMLDSAKeyPair(params ParameterSet) (*KeyPair, *MLDSASigner, error) {
	switch params {
	case MLDSA44:
		pk, sk, err := mldsa44.GenerateKey(nil)
		if err != nil {
			return nil, nil, fmt.Errorf("pqc: generate ML-DSA-44 key: %w", err)
		}
		signer := &MLDSASigner{
			params: MLDSA44,
			sk44:   sk,
			pk44:   pk,
		}
		kp := &KeyPair{
			AlgoID:    AlgoMLDSA,
			PublicKey: pk.Bytes(),
			SecretKey: sk.Bytes(),
		}
		return kp, signer, nil
	case MLDSA65:
		pk, sk, err := mldsa65.GenerateKey(nil)
		if err != nil {
			return nil, nil, fmt.Errorf("pqc: generate ML-DSA-65 key: %w", err)
		}
		signer := &MLDSASigner{
			params: MLDSA65,
			sk65:   sk,
			pk65:   pk,
		}
		kp := &KeyPair{
			AlgoID:    AlgoMLDSA,
			PublicKey: pk.Bytes(),
			SecretKey: sk.Bytes(),
		}
		return kp, signer, nil
	case MLDSA87:
		pk, sk, err := mldsa87.GenerateKey(nil)
		if err != nil {
			return nil, nil, fmt.Errorf("pqc: generate ML-DSA-87 key: %w", err)
		}
		signer := &MLDSASigner{
			params: MLDSA87,
			sk87:   sk,
			pk87:   pk,
		}
		kp := &KeyPair{
			AlgoID:    AlgoMLDSA,
			PublicKey: pk.Bytes(),
			SecretKey: sk.Bytes(),
		}
		return kp, signer, nil
	default:
		return nil, nil, ErrInvalidParameterSet
	}
}
