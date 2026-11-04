// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package pqc

// VerifySignature checks signature over message under publicKey with the named
// algorithm and parameter set. It routes to MLDSAVerifier or SLHDSAVerifier.
// Any other algo, the withdrawn 0x03 included, returns
// ErrAlgorithmNotAvailable. A set outside the algorithm's family returns
// ErrInvalidParameterSet: parameter ids mean nothing without their family.
//
// A false with a nil error is a signature that does not verify; ok is false
// whenever err is not nil.
func VerifySignature(algo AlgoID, params ParameterSet, message, signature, publicKey []byte) (bool, error) {
	var (
		v   Verifier
		err error
	)
	switch algo {
	case AlgoMLDSA:
		v, err = NewMLDSAVerifier(params)
	case AlgoSLHDSA:
		v, err = NewSLHDSAVerifier(params)
	default:
		return false, ErrAlgorithmNotAvailable
	}
	if err != nil {
		return false, err
	}
	return v.Verify(message, signature, publicKey)
}
