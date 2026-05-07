// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package pqc

import (
	"fmt"

	"github.com/cloudflare/circl/kem/mlkem/mlkem1024"
	"github.com/cloudflare/circl/kem/mlkem/mlkem512"
	"github.com/cloudflare/circl/kem/mlkem/mlkem768"
)

// MLKEMEncapsulator implements the KEMEncapsulator interface for ML-KEM
// (FIPS 203).
//
// ML-KEM is the standardized name for CRYSTALS-Kyber. It supports three
// parameter sets:
//   - ML-KEM-512: NIST security level 1
//   - ML-KEM-768: NIST security level 3
//   - ML-KEM-1024: NIST security level 5
type MLKEMEncapsulator struct {
	params ParameterSet
}

// NewMLKEMEncapsulator creates a new ML-KEM encapsulator for the given
// parameter set.
func NewMLKEMEncapsulator(params ParameterSet) (*MLKEMEncapsulator, error) {
	if _, ok := mlkemParamInfo[params]; !ok {
		return nil, ErrInvalidParameterSet
	}
	return &MLKEMEncapsulator{params: params}, nil
}

// Encapsulate generates a shared secret and ciphertext for the given public
// key. The ciphertext should be sent to the private key holder who can recover
// the shared secret via Decapsulate.
func (e *MLKEMEncapsulator) Encapsulate(publicKey []byte) (ciphertext, sharedSecret []byte, err error) {
	switch e.params {
	case MLKEM512:
		if len(publicKey) != mlkem512.PublicKeySize {
			return nil, nil, ErrInvalidKeySize
		}
		var pk mlkem512.PublicKey
		if uerr := pk.Unpack(publicKey); uerr != nil {
			return nil, nil, fmt.Errorf("pqc: ML-KEM-512 unmarshal public key: %w", uerr)
		}
		ct := make([]byte, mlkem512.CiphertextSize)
		ss := make([]byte, mlkem512.SharedKeySize)
		pk.EncapsulateTo(ct, ss, nil)
		return ct, ss, nil
	case MLKEM768:
		if len(publicKey) != mlkem768.PublicKeySize {
			return nil, nil, ErrInvalidKeySize
		}
		var pk mlkem768.PublicKey
		if uerr := pk.Unpack(publicKey); uerr != nil {
			return nil, nil, fmt.Errorf("pqc: ML-KEM-768 unmarshal public key: %w", uerr)
		}
		ct := make([]byte, mlkem768.CiphertextSize)
		ss := make([]byte, mlkem768.SharedKeySize)
		pk.EncapsulateTo(ct, ss, nil)
		return ct, ss, nil
	case MLKEM1024:
		if len(publicKey) != mlkem1024.PublicKeySize {
			return nil, nil, ErrInvalidKeySize
		}
		var pk mlkem1024.PublicKey
		if uerr := pk.Unpack(publicKey); uerr != nil {
			return nil, nil, fmt.Errorf("pqc: ML-KEM-1024 unmarshal public key: %w", uerr)
		}
		ct := make([]byte, mlkem1024.CiphertextSize)
		ss := make([]byte, mlkem1024.SharedKeySize)
		pk.EncapsulateTo(ct, ss, nil)
		return ct, ss, nil
	default:
		return nil, nil, ErrInvalidParameterSet
	}
}

// AlgoID returns AlgoMLKEM.
func (e *MLKEMEncapsulator) AlgoID() AlgoID {
	return AlgoMLKEM
}

// MLKEMDecapsulator implements the KEMDecapsulator interface for ML-KEM
// (FIPS 203).
type MLKEMDecapsulator struct {
	params ParameterSet
}

// NewMLKEMDecapsulator creates a new ML-KEM decapsulator for the given
// parameter set.
func NewMLKEMDecapsulator(params ParameterSet) (*MLKEMDecapsulator, error) {
	if _, ok := mlkemParamInfo[params]; !ok {
		return nil, ErrInvalidParameterSet
	}
	return &MLKEMDecapsulator{params: params}, nil
}

// Decapsulate recovers the shared secret from ciphertext using the provided
// secret key.
func (d *MLKEMDecapsulator) Decapsulate(ciphertext, secretKey []byte) (sharedSecret []byte, err error) {
	switch d.params {
	case MLKEM512:
		if len(secretKey) != mlkem512.PrivateKeySize {
			return nil, ErrInvalidKeySize
		}
		if len(ciphertext) != mlkem512.CiphertextSize {
			return nil, ErrInvalidCiphertextSize
		}
		var sk mlkem512.PrivateKey
		if uerr := sk.Unpack(secretKey); uerr != nil {
			return nil, fmt.Errorf("pqc: ML-KEM-512 unmarshal private key: %w", uerr)
		}
		ss := make([]byte, mlkem512.SharedKeySize)
		sk.DecapsulateTo(ss, ciphertext)
		return ss, nil
	case MLKEM768:
		if len(secretKey) != mlkem768.PrivateKeySize {
			return nil, ErrInvalidKeySize
		}
		if len(ciphertext) != mlkem768.CiphertextSize {
			return nil, ErrInvalidCiphertextSize
		}
		var sk mlkem768.PrivateKey
		if uerr := sk.Unpack(secretKey); uerr != nil {
			return nil, fmt.Errorf("pqc: ML-KEM-768 unmarshal private key: %w", uerr)
		}
		ss := make([]byte, mlkem768.SharedKeySize)
		sk.DecapsulateTo(ss, ciphertext)
		return ss, nil
	case MLKEM1024:
		if len(secretKey) != mlkem1024.PrivateKeySize {
			return nil, ErrInvalidKeySize
		}
		if len(ciphertext) != mlkem1024.CiphertextSize {
			return nil, ErrInvalidCiphertextSize
		}
		var sk mlkem1024.PrivateKey
		if uerr := sk.Unpack(secretKey); uerr != nil {
			return nil, fmt.Errorf("pqc: ML-KEM-1024 unmarshal private key: %w", uerr)
		}
		ss := make([]byte, mlkem1024.SharedKeySize)
		sk.DecapsulateTo(ss, ciphertext)
		return ss, nil
	default:
		return nil, ErrInvalidParameterSet
	}
}

// AlgoID returns AlgoMLKEM.
func (d *MLKEMDecapsulator) AlgoID() AlgoID {
	return AlgoMLKEM
}

// GenerateMLKEMKeyPair generates an ML-KEM key pair for the specified
// parameter set.
func GenerateMLKEMKeyPair(params ParameterSet) (*KeyPair, error) {
	switch params {
	case MLKEM512:
		pk, sk, err := mlkem512.GenerateKeyPair(nil)
		if err != nil {
			return nil, fmt.Errorf("pqc: generate ML-KEM-512 key: %w", err)
		}
		pkBytes := make([]byte, mlkem512.PublicKeySize)
		pk.Pack(pkBytes)
		skBytes := make([]byte, mlkem512.PrivateKeySize)
		sk.Pack(skBytes)
		return &KeyPair{
			AlgoID:    AlgoMLKEM,
			PublicKey: pkBytes,
			SecretKey: skBytes,
		}, nil
	case MLKEM768:
		pk, sk, err := mlkem768.GenerateKeyPair(nil)
		if err != nil {
			return nil, fmt.Errorf("pqc: generate ML-KEM-768 key: %w", err)
		}
		pkBytes := make([]byte, mlkem768.PublicKeySize)
		pk.Pack(pkBytes)
		skBytes := make([]byte, mlkem768.PrivateKeySize)
		sk.Pack(skBytes)
		return &KeyPair{
			AlgoID:    AlgoMLKEM,
			PublicKey: pkBytes,
			SecretKey: skBytes,
		}, nil
	case MLKEM1024:
		pk, sk, err := mlkem1024.GenerateKeyPair(nil)
		if err != nil {
			return nil, fmt.Errorf("pqc: generate ML-KEM-1024 key: %w", err)
		}
		pkBytes := make([]byte, mlkem1024.PublicKeySize)
		pk.Pack(pkBytes)
		skBytes := make([]byte, mlkem1024.PrivateKeySize)
		sk.Pack(skBytes)
		return &KeyPair{
			AlgoID:    AlgoMLKEM,
			PublicKey: pkBytes,
			SecretKey: skBytes,
		}, nil
	default:
		return nil, ErrInvalidParameterSet
	}
}
